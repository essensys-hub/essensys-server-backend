# Architecture Dual-Protocol : Legacy IoT + Modern Web

## Vue d'Ensemble

Le backend Essensys doit gérer **deux protocoles distincts** :

1. **Protocole Legacy** : Client embarqué BP_MQX_ETH (IoT)
   - Format JSON non-standard (clés non-quotées)
   - Headers HTTP spécifiques
   - Single-packet TCP requirement
   - Endpoints : `/api/serverinfos`, `/api/mystatus`, `/api/myactions`, `/api/done/{guid}`

2. **Protocole Moderne** : Frontend React (Web)
   - JSON standard (RFC 8259)
   - Headers HTTP standards
   - REST API classique
   - Endpoints : `/api/auth/*`, `/api/user/*`, `/api/machine/*`, etc.

## Séparation des Protocoles

### Endpoints Legacy (IoT Client)

Ces endpoints **NE DOIVENT JAMAIS ÊTRE MODIFIÉS** :

```
GET  /api/serverinfos     → Client IoT uniquement
POST /api/mystatus        → Client IoT uniquement
GET  /api/myactions       → Client IoT uniquement
POST /api/done/{guid}     → Client IoT uniquement
```

**Caractéristiques** :
- ✅ Normalisation JSON malformé (clés non-quotées)
- ✅ Header `Content-Type: application/json ;charset=UTF-8` (espace avant `;`)
- ✅ Réponses en un seul paquet TCP
- ✅ Codes de statut non-standard (201 Created pour POST)
- ✅ Authentification Basic Auth (optionnelle)

### Endpoints Modernes (Web Frontend)

Ces endpoints sont pour le frontend React :

```
POST /api/auth/login              → Authentification web
POST /api/auth/logout             → Déconnexion
POST /api/auth/register           → Inscription
GET  /api/auth/validate           → Validation de compte
POST /api/auth/forgot-password    → Mot de passe oublié

GET  /api/user/me                 → Informations utilisateur
PUT  /api/user/me                  → Mise à jour utilisateur
POST /api/user/close-account       → Fermeture de compte
POST /api/user/test-question       → Test question de sécurité

GET  /api/machine/status           → État de la machine
GET  /api/machine/wait-box        → Attente de connexion
POST /api/machine/purge-actions   → Purge des actions
GET  /api/machine/wait-actions     → Attente d'exécution

POST /api/actions/do              → Exécution d'actions (depuis web)

GET  /api/version/server           → Version serveur
POST /api/version/init             → Initialisation téléchargement
GET  /api/version/wait             → Attente téléchargement
```

**Caractéristiques** :
- ✅ JSON standard (RFC 8259)
- ✅ Headers HTTP standards
- ✅ Codes de statut REST standard (200 OK, 201 Created, 400 Bad Request, etc.)
- ✅ Authentification par session (cookies) ou JWT
- ✅ Gestion d'erreurs structurée

## Architecture du Backend

### Structure des Handlers

```
internal/api/
├── handlers.go              → Handlers legacy (IoT)
├── handlers_legacy.go      → Handlers legacy (séparés pour clarté)
├── handlers_web.go         → Handlers web (React)
├── router.go               → Router avec séparation des routes
└── json_normalizer.go      → Normalisation JSON legacy uniquement
```

### Router avec Séparation

```go
// Router sépare clairement les deux protocoles
func SetupRouter(handler *Handler, webHandler *WebHandler) *http.ServeMux {
    mux := http.NewServeMux()
    
    // ============================================
    // PROTOCOLE LEGACY (Client IoT)
    // ============================================
    // Ces routes NE DOIVENT PAS être modifiées
    legacy := http.NewServeMux()
    legacy.HandleFunc("/api/serverinfos", handler.GetServerInfos)
    legacy.HandleFunc("/api/mystatus", handler.PostMyStatus)
    legacy.HandleFunc("/api/myactions", handler.GetMyActions)
    legacy.HandleFunc("/api/done/", handler.PostDone)
    
    // Middleware legacy : normalisation JSON + single-packet
    legacyHandler := legacyMiddleware(legacy)
    mux.Handle("/api/serverinfos", legacyHandler)
    mux.Handle("/api/mystatus", legacyHandler)
    mux.Handle("/api/myactions", legacyHandler)
    mux.Handle("/api/done/", legacyHandler)
    
    // ============================================
    // PROTOCOLE MODERNE (Frontend React)
    // ============================================
    web := http.NewServeMux()
    web.HandleFunc("/api/auth/login", webHandler.Login)
    web.HandleFunc("/api/auth/logout", webHandler.Logout)
    web.HandleFunc("/api/auth/register", webHandler.Register)
    // ... autres endpoints web
    
    // Middleware web : sessions, CORS, etc.
    webHandler := webMiddleware(web)
    mux.Handle("/api/auth/", webHandler)
    mux.Handle("/api/user/", webHandler)
    mux.Handle("/api/machine/", webHandler)
    mux.Handle("/api/actions/", webHandler)
    mux.Handle("/api/version/", webHandler)
    
    return mux
}
```

## Normalisation JSON

### Legacy (IoT Client)

Le client IoT envoie du JSON malformé :
```json
{version:"1.0",ek:[{k:613,v:"1"}]}
```

**Solution** : Normalisation automatique dans `json_normalizer.go`
```go
// Appliqué UNIQUEMENT aux endpoints legacy
normalizedBody, err := NormalizeJSON(body)
```

### Moderne (Web Frontend)

Le frontend React envoie du JSON standard :
```json
{"version":"1.0","ek":[{"k":613,"v":"1"}]}
```

**Solution** : Pas de normalisation nécessaire, parser JSON standard

## Headers HTTP

### Legacy (IoT Client)

```
Content-Type: application/json ;charset=UTF-8
Connection: close
```

**Points critiques** :
- Espace **avant** le point-virgule : ` ;charset`
- `Connection: close` obligatoire
- Pas de CORS (client embarqué)

### Moderne (Web Frontend)

```
Content-Type: application/json;charset=UTF-8
Access-Control-Allow-Origin: http://localhost:5173
Access-Control-Allow-Credentials: true
```

**Points standards** :
- Pas d'espace avant `;charset`
- CORS activé pour le frontend
- Cookies de session supportés

## Authentification

### Legacy (IoT Client)

**Basic Auth** (optionnel) :
```
Authorization: Basic base64(client_id:password)
```

**Identification** : Par `client_id` (identifiant machine)

### Moderne (Web Frontend)

**Sessions** (cookies) :
```
Cookie: session_id=abc123...
```

**Identification** : Par `user_id` (utilisateur web)

## Gestion des Erreurs

### Legacy (IoT Client)

**Format simple** :
```http
HTTP/1.1 400 Bad Request
Content-Type: application/json ;charset=UTF-8

{}
```

**Limitations** :
- Le client ne gère pas toujours les erreurs
- Peut se bloquer silencieusement
- Pas de retry automatique

### Moderne (Web Frontend)

**Format structuré** :
```json
{
  "error": {
    "code": "INVALID_CREDENTIALS",
    "message": "Email ou mot de passe incorrect",
    "details": {}
  }
}
```

**Avantages** :
- Gestion d'erreurs complète
- Codes d'erreur standardisés
- Messages utilisateur-friendly

## Base de Données

### Partage des Données

Les deux protocoles partagent la même base de données PostgreSQL :

```
┌─────────────────┐
│  Client IoT     │──┐
│  (Legacy)       │  │
└─────────────────┘  │
                      ├──→ PostgreSQL
┌─────────────────┐  │
│  Frontend React │  │
│  (Moderne)      │──┘
└─────────────────┘
```

**Tables partagées** :
- `es_machine` : Machines (boîtiers)
- `es_action` : Actions à exécuter
- `es_state` : États des machines
- `es_user` : Utilisateurs web

### Isolation des Données

**Client IoT** :
- Identifié par `machine_id` (no_serie)
- Actions via `/api/myactions`
- États via `/api/mystatus`

**Frontend Web** :
- Identifié par `user_id` (email)
- Actions via `/api/actions/do`
- États via `/api/machine/status`

## Migration PostgreSQL

### Impact sur le Protocole Legacy

**Aucun impact** : Le protocole legacy reste identique

**Changements internes uniquement** :
- Store en mémoire → PostgreSQL
- Même format de réponse JSON
- Mêmes headers HTTP
- Même logique de traitement

### Compatibilité

Le backend Go maintient **100% de compatibilité** avec le client IoT :
- ✅ Même format JSON de réponse
- ✅ Mêmes headers HTTP
- ✅ Même logique de traitement
- ✅ Même comportement

## Tests

### Tests Legacy

```go
// Test du protocole legacy
func TestLegacyProtocol(t *testing.T) {
    // JSON malformé
    body := `{version:"1.0",ek:[{k:613,v:"1"}]}`
    
    // Headers legacy
    req.Header.Set("Content-Type", "application/json")
    
    // Vérifier normalisation
    normalized := NormalizeJSON([]byte(body))
    
    // Vérifier réponse
    assert.Equal(t, "application/json ;charset=UTF-8", resp.Header.Get("Content-Type"))
}
```

### Tests Modernes

```go
// Test du protocole moderne
func TestModernProtocol(t *testing.T) {
    // JSON standard
    body := `{"email":"user@example.com","password":"hash"}`
    
    // Headers standards
    req.Header.Set("Content-Type", "application/json")
    
    // Vérifier réponse
    assert.Equal(t, "application/json;charset=UTF-8", resp.Header.Get("Content-Type"))
}
```

## Recommandations

### ✅ À FAIRE

1. **Séparer clairement** les handlers legacy et web
2. **Documenter** chaque endpoint avec son protocole
3. **Tester** les deux protocoles indépendamment
4. **Maintenir** la compatibilité legacy à 100%
5. **Isoler** les changements (legacy vs moderne)

### ❌ À NE PAS FAIRE

1. **Modifier** les endpoints legacy existants
2. **Changer** le format JSON des réponses legacy
3. **Modifier** les headers HTTP legacy
4. **Mélanger** la logique legacy et moderne
5. **Casser** la compatibilité avec le client IoT

## Conclusion

L'architecture dual-protocol permet de :
- ✅ Maintenir la compatibilité avec le client IoT legacy
- ✅ Offrir une API moderne pour le frontend React
- ✅ Partager la même base de données
- ✅ Évoluer indépendamment les deux protocoles

**Règle d'or** : Le protocole legacy est **sacré** et ne doit jamais être modifié.




