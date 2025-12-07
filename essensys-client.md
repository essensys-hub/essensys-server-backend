# Protocole Client Essensys

## Vue d'ensemble

Ce document décrit le protocole de communication utilisé par le firmware BP_MQX_ETH pour communiquer avec le serveur Essensys (mon.essensys.fr). Le gateway server doit implémenter une compatibilité 100% avec ce protocole.

## Architecture de communication

### Tâche Ethernet (ETH_TASK)

La communication s'effectue dans une tâche dédiée `Ethernet_task()` qui s'exécute en boucle avec les fréquences suivantes :
- **Utilisateur connecté** : 500ms
- **Pas d'utilisateur** : 2 secondes

### Séquence de connexion

1. **Vérification câble réseau** (`ipcfg_get_link_active()`)
2. **Configuration IP** (DHCP ou IP fixe)
3. **Résolution DNS** de "mon.essensys.fr"
4. **Dialogue avec serveur** (4 fonctions principales)

## API Endpoints

### 1. GET /api/serverinfos

**Fonction** : `sc_GetInformationServer()`

**En-têtes HTTP** :
```http
GET /api/serverinfos HTTP/1.1
Accept: application/json,application/xhtml1+xml,application/xml;q=0.9,*/*;q=0.8
host: mon.essensys.fr
Cache-Control: max-age=0
Accept-Charset: ISO-8859-1,utf-8;q=0.7,*;q=0.3
Authorization: Basic [matricule_crypté]
[matricule_système]
```

**Réponse attendue** :
```json
{
  "isconnected": false,
  "infos": [363, 349, 350, 351, 352, 353, 11, 920],
  "newversion": "V1"
}
```

**Champs** :
- `isconnected` : boolean - Utilisateur connecté sur interface web
- `infos` : array - Indices Table d'Échange demandés (max 30)
- `newversion` : string - "no" ou "V123" (numéro version)

### 2. POST /api/mystatus

**Fonction** : `sc_PostInformationServer()`

**Objectif** : Envoie au serveur les valeurs de la Table d'Échange demandées lors de l'étape précédente.

#### Construction de la requête

**Étape 1 - Préparation des données** :
```c
// Construction JSON des données EK
signed char sc_JsonPostServerInformation(char *pc_RxBuffer, unsigned short us_TailleBuffer, struct StructServerInformation *pst_ServerInformation)
{
    // En-tête avec version BP
    sprintf(l_c_Buffer, "{\"version\":\"V%d\",\"ek\":[", us_BP_VERSION_SERVEUR);
    strcpy(pc_RxBuffer, l_c_Buffer);
    
    // Pour chaque indice demandé par le serveur
    for(l_uc_CompteurValeurs = 0; l_uc_CompteurValeurs < pst_ServerInformation->uc_NbInfosDemandees; l_uc_CompteurValeurs++)
    {
        // Lecture valeur depuis Table d'Échange
        unsigned char valeur = uc_TableEchange_Lit_Data(pst_ServerInformation->us_InfosDemandees[l_uc_CompteurValeurs]);
        
        // Traitement spécial pour données binaires
        if(indice == Alerte || indice == EtatBP1 || indice == EtatBP2)
        {
            vd_ConvertirOctetEnChaineBinaire(valeur, chaine_binaire);
            sprintf(l_c_Buffer, "{k:%d,v:\"%s\"}", indice, chaine_binaire);
        }
        else
        {
            sprintf(l_c_Buffer, "{k:%d,v:\"%d\"}", indice, valeur);
        }
        strcat(pc_RxBuffer, l_c_Buffer);
    }
    strcat(pc_RxBuffer, "]}");
}
```

**Étape 2 - Construction requête HTTP** :
```c
strcpy(c_EthernetBufferTX, "POST /api/mystatus HTTP/1.1\r\n");
strcat(c_EthernetBufferTX, c_EnteteTrameAccept());
strcat(c_EthernetBufferTX, c_EnteteTrameContentType());
strcat(c_EthernetBufferTX, c_EnteteTrameHost());
strcat(c_EthernetBufferTX, c_EnteteTrameCache());
strcat(c_EthernetBufferTX, c_EnteteTrameAcceptCharset());

// Calcul et ajout Content-Length
l_ul_Longueur = strlen(c_EthernetBufferTX2);
sprintf(l_c_Buffer, "Content-Length: %d\r\n", l_ul_Longueur);
strcat(c_EthernetBufferTX, l_c_Buffer);

strcat(c_EthernetBufferTX, c_EnteteTrameAuthorisation());
strcat(c_EthernetBufferTX, c_EnteteTrameMatricule());
strcat(c_EthernetBufferTX, "\r\n\r\n");

// Ajout du corps JSON
strcat(c_EthernetBufferTX, c_EthernetBufferTX2);
```

#### En-têtes HTTP complets
```http
POST /api/mystatus HTTP/1.1
Accept: application/json,application/xhtml1+xml,application/xml;q=0.9,*/*;q=0.8
Content-type: application/json ;charset=UTF-8
host: mon.essensys.fr
Cache-Control: max-age=0
Accept-Charset: ISO-8859-1,utf-8;q=0.7,*;q=0.3
Content-Length: 156
Authorization: Basic MTIzNDU2Nzg5MGFiY2RlZjpmZWRjYmEwOTg3NjU0MzIx
MTIzNDU2Nzg5MGFiY2RlZjpmZWRjYmEwOTg3NjU0MzIx


```

#### Corps de la requête (exemple)
```json
{
  "version": "V125",
  "ek": [
    {"k": 363, "v": "01001100"},
    {"k": 349, "v": "25"},
    {"k": 350, "v": "1"},
    {"k": 351, "v": "0"},
    {"k": 352, "v": "22"},
    {"k": 353, "v": "1"}
  ]
}
```

#### Format des données EK

**Structure générale** :
- `k` : Indice Table d'Échange (0-999)
- `v` : Valeur (toujours string)

**Types de données** :

1. **Données normales** (valeur numérique) :
```json
{"k": 349, "v": "25"}  // Température = 25°C
{"k": 350, "v": "1"}   // Mode chauffage = 1 (Confort)
```

2. **Données binaires** (état des bits) :
```json
{"k": 363, "v": "01001100"}  // Alerte : bits d'état
```

**Conversion binaire** (fonction `vd_ConvertirOctetEnChaineBinaire`) :
```c
// Exemple : valeur 76 (0x4C) → "01001100"
void vd_ConvertirOctetEnChaineBinaire(unsigned char uc_Valeur, char *c_Chaine9Caracteres)
{
    unsigned char l_uc_Masque = 1;
    for(l_uc_Compteur = 0; l_uc_Compteur < 8; l_uc_Compteur++)
    {
        if((uc_Valeur & l_uc_Masque) == 0)  
            c_Chaine9Caracteres[l_uc_Compteur] = '0';
        else                                
            c_Chaine9Caracteres[l_uc_Compteur] = '1';
        l_uc_Masque = l_uc_Masque << 1;
    }
    c_Chaine9Caracteres[8] = 0;
}
```

#### Indices spéciaux (format binaire)

**363 (Alerte)** - Chaque bit représente un état :
- Bit 0 : Déclenchement alarme
- Bit 1 : Fuite lave-linge  
- Bit 2 : Fuite lave-vaisselle
- Bits 3-7 : Autres alertes

**EtatBP1** - État du boîtier principal :
- Bit 0 : Alarme activée
- Bit 1 : Alarme déclenchée
- Bits 2-7 : Autres états

**EtatBP2** - États système :
- Bits 0-7 : États divers du système

#### Traitement des données

**Lecture depuis Table d'Échange** :
```c
// Pour chaque indice demandé
unsigned short indice = pst_ServerInformation->us_InfosDemandees[compteur];
unsigned char valeur = uc_TableEchange_Lit_Data(indice);
```

**Validation des données** :
- Vérification que l'indice existe dans la Table d'Échange
- Conversion appropriée selon le type de donnée
- Limitation de la taille du buffer de sortie

#### Réponse serveur attendue
```http
HTTP/1.1 201 Created
Content-Type: application/json
Content-Length: 0


```

**Validation côté client** :
```c
// Vérification code retour serveur
if(strstr(c_EthernetBufferRX, "HTTP/1.1 201 Created") == NULL)
{
    l_sc_Retour = sc_ETHERNET_RETOUR_PB_DATA;
    vd_EspionRS_Printf(uc_ESPION_TACHE_ETHERNET_ACTIVITE,"PostInformationServer -> code retour serveur incorrect\n");
}
```

#### Gestion d'erreurs

**Codes de retour** :
- `sc_ETHERNET_RETOUR_OK` (0) : Succès
- `sc_ETHERNET_RETOUR_PB_DATA` (-1) : Erreur données/réponse
- `sc_ETHERNET_RETOUR_PB_RTCS` (-2) : Erreur réseau/socket

**Erreurs possibles** :
- Buffer trop petit pour les données JSON
- Erreur réseau lors de l'envoi
- Réponse serveur incorrecte
- Timeout de connexion

### 3. GET /api/myactions

**Fonction** : `sc_ActionManagment()`

**En-têtes HTTP** : Identiques à serverinfos

**Réponse attendue** :
```json
{
  "_de67f": {
    "guid": "806b4fc7-a820-4c49-9ae8-24ced8f6770f",
    "obl": "73;178;187;105;197;154;208;248;26;52;219;233;77;251;102;182"
  },
  "actions": [
    {
      "guid": "ec9026fe-25fc-4b2f-b4b0-c5402699f399",
      "params": [
        {"k": 590, "v": "1"},
        {"k": 605, "v": "0"},
        {"k": 606, "v": "0"},
        {"k": 607, "v": "0"},
        {"k": 608, "v": "0"},
        {"k": 609, "v": "0"},
        {"k": 610, "v": "0"},
        {"k": 611, "v": "0"},
        {"k": 612, "v": "0"},
        {"k": 613, "v": "64"},
        {"k": 614, "v": "0"},
        {"k": 615, "v": "0"},
        {"k": 616, "v": "0"},
        {"k": 617, "v": "0"},
        {"k": 618, "v": "0"},
        {"k": 619, "v": "0"},
        {"k": 620, "v": "0"},
        {"k": 621, "v": "0"},
        {"k": 622, "v": "0"}
      ]
    }
  ]
}
```

> [!IMPORTANT]
> **Mise à jour complète requise** : Le client attend que **tous les indices de la plage 605 à 622** soient présents dans le tableau `params`, même s'ils sont à "0". L'indice **590** (Scénario) doit également être présent avec la valeur "1" pour déclencher la prise en compte.

**Champs** :
- `_de67f` : Commande alarme cryptée (optionnel)
  - `guid` : Identifiant unique pour acquittement
  - `obl` : Données cryptées AES (ALARMEON/ALARMEOFF)
- `actions` : Tableau d'actions à exécuter
  - `guid` : Identifiant unique pour acquittement
  - `params` : Modifications Table d'Échange

### 4. POST /api/done/{guid}

**Fonction** : Acquittement d'action

**En-têtes HTTP** :
```http
POST /api/done/ec9026fe-25fc-4b2f-b4b0-c5402699f399 HTTP/1.1
Accept: application/json,application/xhtml1+xml,application/xml;q=0.9,*/*;q=0.8
Content-type: application/json
host: mon.essensys.fr
Cache-Control: max-age=0
Accept-Charset: ISO-8859-1,utf-8;q=0.7,*;q=0.3
Content-Length: 0
Authorization: Basic [matricule_crypté]
[matricule_système]
```

**Réponse attendue** : `HTTP/1.1 201 Created`

## Gestion des actions

### Actions normales

1. **Réception** : Parsing JSON des paramètres
2. **Application** : Mise à jour Table d'Échange via `uc_TableEchange_Ecrit_Data()`
3. **Gestion spéciale** : Les scénarios sont appliqués en dernier
4. **Acquittement** : POST /api/done/{guid}

### Actions alarme

1. **Décryptage AES** : Utilisation clé serveur stockée en EEPROM
2. **Analyse** : Recherche "ALARMEON" ou "ALARMEOFF"
3. **Application** : Flags `uc_DemandeServeurActiverAlarme` / `uc_DemandeServeurCouperAlarme`
4. **Acquittement** : POST /api/done/{guid}

## Authentification

### Matricule système
- **Format** : Adresse MAC + clé serveur
- **Cryptage** : Base64 de la concaténation
- **Usage** : En-tête Authorization et ligne séparée

### Clé serveur
- **Stockage** : EEPROM externe SPI
- **Taille** : 16 octets
- **Usage** : Authentification + décryptage alarmes

## Download firmware

### Déclenchement
Si `newversion` > version actuelle → téléchargement automatique

### Séquence
1. **POST /api/getversioncontent/{index}** : Téléchargement par blocs
2. **Écriture Flash** : Stockage en zone programme
3. **POST /api/endversioncontent** : Acquittement fin
4. **Redémarrage** : Reset automatique du contrôleur

## Table d'Échange

### Indices spéciaux (format binaire)
- **363 (Alerte)** :
  - Bit 0 : Déclenchement alarme
  - Bit 1 : Fuite lave-linge
  - Bit 2 : Fuite lave-vaisselle

- **EtatBP1** :
  - Bit 0 : Alarme activée
  - Bit 1 : Alarme déclenchée

### Accès données
- **Lecture** : `uc_TableEchange_Lit_Data(indice)`
- **Écriture** : `uc_TableEchange_Ecrit_Data(indice, valeur, source)`

## Gestion d'erreurs

### États réseau
- **Bit 0** : Câble réseau (0=OK, 1=HS)
- **Bit 1** : DHCP (0=OK, 1=HS)
- **Bit 2** : DNS (0=OK, 1=HS)
- **Bit 3** : Serveur (0=OK, 1=HS)

### Codes retour
- **sc_ETHERNET_RETOUR_OK** : 0
- **sc_ETHERNET_RETOUR_PB_DATA** : -1
- **sc_ETHERNET_RETOUR_PB_RTCS** : -2

## Implémentation Gateway

### Compatibilité requise
1. **Endpoints identiques** : Mêmes URLs et méthodes HTTP
2. **Format JSON** : Respect exact des structures
3. **Authentification** : Support matricule/clé serveur
4. **Gestion actions** : Traitement et acquittement
5. **Download** : Support mise à jour firmware (optionnel)

### Adaptations possibles
- **Stockage** : SQLite/Redis au lieu de Table d'Échange
- **WebSocket** : Notifications temps réel pour interface web
- **API moderne** : Endpoints supplémentaires pour nouvelles fonctionnalités
- **Sécurité** : HTTPS, authentification renforcée

## Exemple d'implémentation

```typescript
// Endpoint serverinfos
app.get('/api/serverinfos', authenticateClient, (req, res) => {
  const isConnected = websocketServer.hasConnectedClients();
  const requestedData = getRequestedDataIndices(req.clientId);
  const newVersion = checkForUpdates(req.clientVersion);
  
  res.json({
    isconnected: isConnected,
    infos: requestedData,
    newversion: newVersion || "no"
  });
});

// Endpoint mystatus
app.post('/api/mystatus', authenticateClient, (req, res) => {
  const { version, ek } = req.body;
  
  // Stocker les données reçues
  for (const data of ek) {
    await storage.updateValue(req.clientId, data.k, data.v);
  }
  
  // Notifier clients WebSocket
  websocketServer.broadcast('dataUpdate', { clientId: req.clientId, data: ek });
  
  res.status(201).send();
});
```

Ce protocole garantit une compatibilité totale avec le firmware existant tout en permettant des extensions modernes pour le gateway server.