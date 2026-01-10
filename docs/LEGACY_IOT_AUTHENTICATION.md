# Authentification Legacy IoT - Protocole Basic Auth

## Vue d'Ensemble

Les clients IoT legacy (`client-essensys-legacy`) utilisent un protocole d'authentification **Basic Auth non-standard** qui doit être respecté à la lettre pour assurer la compatibilité avec les anciens boîtiers embarqués.

## Protocole d'Authentification

### Côté Client (BP_MQX_ETH)

Le client legacy génère ses credentials d'authentification selon le processus suivant :

1. **Lecture de la clé serveur** : Le client lit `uc_Cle_Acces_Distance` (16 octets) depuis l'EEPROM SPI
2. **Conversion en hexadécimal** : Chaque octet est converti en 2 caractères hex → 32 caractères hex au total
3. **Calcul MD5** : Le MD5 de cette chaîne hexadécimale est calculé → 16 octets (32 caractères hex)
4. **Division** : Le MD5 est divisé en deux parties :
   - `username` = 8 premiers octets (16 caractères hex)
   - `password` = 8 derniers octets (16 caractères hex)
5. **Format Basic Auth** : `username:password` (33 caractères avec le `:`)
6. **Encodage Base64** : La chaîne `username:password` est encodée en Base64
7. **Header HTTP** : `Authorization: Basic <base64_encoded_credentials>`

### Exemple de Code Client (C)

```c
// Dans Cryptage.c
void cryptage(void)
{
    unsigned char uc_CleServeur[33];
    unsigned char md5[16];
    char md5chaine[33];
    
    // 1. Conversion de la clé serveur (16 octets) en hex (32 chars)
    for(l_uc_Compteur = 0; l_uc_Compteur < 16; l_uc_Compteur++)
    {
        uc_CleServeur[(l_uc_Compteur*2)] = ((uc_Cle_Acces_Distance[l_uc_Compteur]) & 0x0F) + '0';
        uc_CleServeur[(l_uc_Compteur*2)+1] = ((uc_Cle_Acces_Distance[l_uc_Compteur] >> 4) & 0x0F) + '0';
    }
    uc_CleServeur[32] = 0;
    
    // 2. Calcul MD5
    MD5Init(&TestMD5);
    MD5Update(&TestMD5, (unsigned char *)uc_CleServeur, 32);
    MD5Final(md5, &TestMD5);
    
    // 3. Formatage: 8 premiers octets + ":" + 8 derniers octets
    for(i=0; i<8; i++)
    {
        sprintf(tempHex, "%02x", md5[i]);
        strcat(md5chaine, tempHex);
    }
    strcat(md5chaine, ":");
    for(i=8; i<16; i++)
    {
        sprintf(tempHex, "%02x", md5[i]);
        strcat(md5chaine, tempHex);
    }
    
    // 4. Encodage Base64
    base64_init_encodestate(&base64State);
    base64_encode_block(md5chaine, 33, c_MatriculeCryptee, &base64State);
    
    // 5. Envoi dans le header HTTP
    // Authorization: Basic <c_MatriculeCryptee>
}
```

### Côté Serveur (Go)

Le serveur Go valide les credentials selon le processus suivant :

1. **Extraction du header** : `Authorization: Basic <base64>`
2. **Décodage Base64** : Obtention de `username:password`
3. **Concaténation** : `hashedPkey = username + password` (32 caractères hex = MD5 complet)
4. **Recherche en base** : Recherche d'une machine avec `hashed_pkey = hashedPkey` ET `is_active = true`
5. **Validation** : Si machine trouvée, authentification réussie

### Exemple de Code Serveur (Go)

```go
// Dans internal/middleware/auth.go
func BasicAuthDB(db *sqlx.DB) func(http.Handler) http.Handler {
    machineRepo := database.NewMachineRepository(db)
    
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            // 1. Extraction et décodage Base64
            authHeader := r.Header.Get("Authorization")
            encodedCredentials := strings.TrimPrefix(authHeader, "Basic ")
            decodedBytes, _ := base64.StdEncoding.DecodeString(encodedCredentials)
            
            // 2. Parse username:password
            credentials := string(decodedBytes)
            parts := strings.SplitN(credentials, ":", 2)
            username := parts[0]  // 16 hex chars
            password := parts[1]  // 16 hex chars
            
            // 3. Concaténation = MD5 complet (32 hex chars)
            hashedPkey := username + password
            
            // 4. Recherche en base de données
            machine, err := machineRepo.GetByHashedPkey(hashedPkey)
            if err != nil || machine == nil {
                w.Header().Set("WWW-Authenticate", "Basic")
                w.WriteHeader(http.StatusUnauthorized)
                return
            }
            
            // 5. Authentification réussie
            ctx := context.WithValue(r.Context(), ClientIDKey, machine.NoSerie)
            ctx = context.WithValue(ctx, MachineIDKey, machine.ID)
            r = r.WithContext(ctx)
            
            next.ServeHTTP(w, r)
        })
    }
}
```

## Schéma de la Base de Données

### Table `es_machine`

| Colonne | Type | Description |
|---------|------|-------------|
| `id` | INT | Identifiant unique |
| `no_serie` | VARCHAR | Numéro de série du boîtier |
| `pkey` | VARCHAR(32) | Clé d'activation (32 caractères) |
| `hashed_pkey` | VARCHAR(255) | MD5 de `pkey` (32 caractères hex) |
| `is_active` | BOOLEAN | Machine active (doit être `true` pour l'authentification) |

### Relation avec le Legacy ASP.NET

Dans le système legacy ASP.NET :

```csharp
// UserService.cs
public EsMachine ValidateAPIAccess(string cryptedcode)
{
    // cryptedcode = username + password (32 hex chars = MD5 complet)
    return _repm.FindBy(m => m.HashedPkey == cryptedcode && m.IsActive);
}
```

Le serveur Go reproduit exactement cette logique.

## Configuration du Serveur

### Avec Base de Données (Recommandé)

```go
// Dans cmd/server/main.go
db, _ := sqlx.Connect("postgres", connectionString)
router := api.NewRouterWithDB(handler, db, nil, true)
```

Le serveur utilisera automatiquement `BasicAuthDB` pour l'authentification des clients legacy.

### Sans Base de Données (Mode Legacy)

```go
// Pour la compatibilité avec les tests existants
validCredentials := map[string]string{
    "client1": "password1",
    "client2": "password2",
}
router := api.NewRouter(handler, validCredentials, true)
```

Le serveur utilisera `BasicAuth` avec un map en mémoire (non compatible avec les vrais clients legacy).

## Headers HTTP Requis

### Requête du Client

```http
POST /api/mystatus HTTP/1.1
Host: mon.essensys.fr
Authorization: Basic <base64_encoded_credentials>
Content-Type: application/json
Connection: close
```

### Réponse en Cas d'Échec

```http
HTTP/1.1 401 Unauthorized
WWW-Authenticate: Basic
Connection: close
```

**Important** : Le header `WWW-Authenticate: Basic` est **requis** pour que le client legacy comprenne qu'il doit s'authentifier.

## Compatibilité avec le Legacy

### Points Critiques

1. **Format exact** : Le client envoie `username:password` où chaque partie fait exactement 16 caractères hex
2. **Concaténation** : Le serveur doit concaténer `username + password` (pas de séparation)
3. **MD5 en hexadécimal** : Le `hashed_pkey` en base est stocké en hexadécimal (32 caractères)
4. **Case sensitivity** : Les caractères hex sont en minuscules (`a-f`, `0-9`)
5. **Base64 standard** : Utilisation de l'encodage Base64 standard (RFC 4648)

### Validation

Pour valider que l'authentification fonctionne correctement :

1. **Test avec un client réel** : Utiliser un boîtier BP_MQX_ETH réel
2. **Test avec script Python** : Utiliser `test_chb3.py` qui simule le client legacy
3. **Vérification des logs** : Les requêtes authentifiées doivent avoir `clientID` et `machineID` dans le contexte

## Migration depuis le Legacy ASP.NET

### Étape 1 : Migration des Données

Les machines doivent avoir leur `hashed_pkey` migré depuis SQL Server :

```sql
-- SQL Server (legacy)
SELECT PKEY, HASHEDPKEY FROM ES_MACHINE WHERE ISACTIVE = 1

-- PostgreSQL (nouveau)
INSERT INTO es_machine (pkey, hashed_pkey, is_active, ...)
VALUES ('...', '...', true, ...);
```

### Étape 2 : Validation

Après migration, tester avec un client legacy pour vérifier que :
- L'authentification fonctionne
- Le `clientID` (NoSerie) est correctement extrait
- Le `machineID` est disponible dans le contexte

## Dépannage

### Problème : 401 Unauthorized

**Causes possibles** :
1. `hashed_pkey` incorrect en base (vérifier le format hex)
2. Machine `is_active = false`
3. Base64 mal encodé côté client
4. MD5 calculé incorrectement

**Solution** :
- Vérifier les logs du serveur
- Comparer le `hashed_pkey` en base avec le MD5 attendu
- Utiliser `tcpdump` pour capturer les requêtes HTTP

### Problème : Client ne s'authentifie pas

**Causes possibles** :
1. Header `WWW-Authenticate` manquant dans la réponse 401
2. Format Base64 incorrect
3. Connexion TCP fermée avant la réponse

**Solution** :
- Vérifier que le middleware ajoute `WWW-Authenticate: Basic`
- Capturer les paquets réseau avec `tcpdump` ou Wireshark

## Références

- [Protocole HTTP Legacy](./client-essensys-legacy/docs/protocol/http-legacy-protocol.md)
- [Architecture Dual Protocol](./ARCHITECTURE_DUAL_PROTOCOL.md)
- Code source : `client-essensys-legacy/Ethernet/Cryptage.c`
- Code serveur : `internal/middleware/auth.go` (fonction `BasicAuthDB`)


