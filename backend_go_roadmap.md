# Guide de Réalisation : Backend Essensys (Go) - v1

Ce document décrit la feuille de route pour transformer le prototype `server.go` en une application Go structurée, maintenable et prête pour la production.

## 1. Objectifs du Projet

*   **Robustesse** : Gérer les connexions concurrentes et les erreurs réseau sans crasher.
*   **Compatibilité** : Respecter scrupuleusement le protocole du client `BP_MQX_ETH` (format JSON strict, headers).
*   **Maintenabilité** : Séparer la logique HTTP, la logique métier et le stockage de données.

## 2. Architecture Proposée

Nous recommandons la structure standard des projets Go (Layout) :

```text
essensys-server/
├── cmd/
│   └── server/
│       └── main.go           # Point d'entrée (Initialisation & Démarrage)
├── internal/
│   ├── api/                  # Handlers HTTP (Contrôleurs)
│   │   ├── handlers.go
│   │   └── router.go
│   ├── core/                 # Logique Métier (Services)
│   │   ├── action_service.go # Gestion des actions & fusion bitwise
│   │   └── status_service.go # Gestion des états
│   ├── data/                 # Accès aux Données (Repository)
│   │   └── memory_store.go   # Stockage thread-safe (Mutex)
│   └── middleware/           # Auth, Logging, Recovery
│       └── auth.go
├── pkg/
│   └── protocol/             # Types et Constantes partagés
│       ├── types.go          # Structs JSON (Action, Status)
│       └── constants.go      # Indices (613, 590...)
├── go.mod
└── README.md
```

## 3. Étapes d'Implémentation

### Étape 1 : Initialisation du Projet
```bash
mkdir essensys-server
cd essensys-server
go mod init github.com/essensys-hub/essensys-server
```

### Étape 2 : Définition du Protocole (`pkg/protocol`)
Définir les structures de données qui matchent exactement le JSON du client.

*   **ActionParam** : `k` (int), `v` (string).
*   **ActionPayload** : `_de67f` (null), `actions` (array).
*   **Constantes** :
    *   `INDEX_SCENARIO = 590`
    *   `INDEX_LIGHT_START = 605`
    *   `INDEX_LIGHT_END = 622`

### Étape 3 : Couche de Données (`internal/data`)
Créer un `Store` thread-safe pour stocker :
1.  **Table d'Échange** : Map `[int]string` protégée par `sync.RWMutex`.
2.  **File d'Attente Actions** : Slice ou Channel pour les actions en attente.

### Étape 4 : Logique Métier (`internal/core`)
C'est ici que réside l'intelligence du serveur (portée du `server.go` actuel).

*   **Fusion des Actions (Bitwise OR)** :
    *   Lorsqu'une action arrive pour un indice existant (ex: 615), faire un `OR` binaire avec la valeur actuelle.
*   **Préparation du Bloc Complet** :
    *   Pour toute action sur les lumières/volets, générer automatiquement la liste des paramètres pour **tous** les indices de 605 à 622 (valeur 0 par défaut).
    *   Ajouter systématiquement `k=590, v=1`.

### Étape 5 : API HTTP (`internal/api`)
Implémenter les handlers :

*   `GET /api/serverinfos` : Renvoie la config.
*   `POST /api/mystatus` :
    *   **CRITIQUE** : Le client envoie un JSON invalide (clés sans quotes `{k:1...}`).
    *   Solution : Utiliser une regex ou un `strings.Replace` pour corriger le JSON *avant* le `json.Unmarshal`.
*   `GET /api/myactions` :
    *   Renvoie la file d'attente.
    *   Format strict : `{"_de67f":null,"actions":[...]}`.
*   `POST /api/done/{guid}` : Acquitte et supprime l'action.

### Étape 6 : Middleware & Sécurité (`internal/middleware`)
*   **Authentification** : Vérifier le header `Authorization: Basic ...`.
*   **Logging** : Tracer chaque requête pour le débogage.

## 4. Points de Vigilance Critiques

1.  **Format JSON de `myactions`** :
    *   Le client C plante si le champ `_de67f` n'est pas le premier.
    *   Le client ignore l'action si le bloc 605-622 n'est pas complet.

2.  **Performance & Concurrence** :
    *   Le client poll `/api/mystatus` et `/api/myactions` très fréquemment.
    *   Utilisez impérativement des `Mutex` pour lire/écrire dans la mémoire partagée.

3.  **Port 80** :
    *   Le serveur doit écouter sur le port 80 (standard HTTP).
    *   Sous Linux/Mac, cela nécessite les droits `root` (`sudo`) ou une configuration `setcap`.

## 5. Validation
Utiliser le script `test_chb3.py` pour valider chaque étape du développement. Il vérifie automatiquement la conformité du protocole.
