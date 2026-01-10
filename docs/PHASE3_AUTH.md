# Phase 3 : Authentification et Gestion des Utilisateurs - Complétée ✅

## Résumé

La Phase 3 de la migration a été complétée avec succès. Le système d'authentification et la gestion des utilisateurs ont été implémentés pour le frontend React, tout en maintenant la compatibilité avec le protocole legacy IoT.

## Fichiers créés

### Authentification

- `internal/auth/password.go` : Hash SHA1/MD5 (compatibilité legacy)
- `internal/auth/session.go` : Gestion des sessions utilisateur

### Services

- `internal/services/user_service.go` : Service de gestion des utilisateurs
  - `Login(email, password)`
  - `Register(user, noSerie)`
  - `ValidateAccount(guid, email, generateCode)`
  - `ForgotPassword(email)`
  - `UpdateUser(...)`
  - `CloseAccount(userID)`
  - `TestQuestion(userID, response)`

### Handlers Web

- `internal/api/handlers_web.go` : Handlers pour les endpoints web
  - `POST /api/auth/login`
  - `POST /api/auth/logout`
  - `POST /api/auth/register`
  - `GET /api/auth/validate`
  - `POST /api/auth/forgot-password`
  - `GET /api/user/me`
  - `PUT /api/user/me`
  - `POST /api/user/close-account`
  - `POST /api/user/test-question`

### Middleware

- `internal/middleware/web_auth.go` : Authentification par sessions
  - `WebAuth` : Middleware de vérification de session
  - `CORS` : Support CORS pour le frontend React
  - `SetSessionCookie` / `ClearSessionCookie` : Gestion des cookies

### Repositories

- `internal/data/database/cle_machine_repository.go` : Repository pour les clés d'activation

### Router

- `internal/api/router.go` : Mis à jour pour séparer endpoints legacy et web

## Architecture

### Séparation Legacy / Web

```
┌─────────────────────────────────────┐
│  Endpoints Legacy (IoT)             │
│  - /api/serverinfos                 │
│  - /api/mystatus                    │
│  - /api/myactions                   │
│  - /api/done/{guid}                 │
│  Auth: Basic Auth (optionnel)       │
└─────────────────────────────────────┘

┌─────────────────────────────────────┐
│  Endpoints Web (React)              │
│  - /api/auth/* (public)             │
│  - /api/user/* (authentifié)        │
│  Auth: Sessions (cookies)            │
└─────────────────────────────────────┘
```

### Hash SHA1 (Legacy Compatibility)

Le système utilise SHA1 pour les mots de passe (comme le legacy) :
- **Format** : UTF-16 (comme C# UnicodeEncoding)
- **Output** : Hex lowercase
- **Compatibilité** : 100% avec les données legacy existantes

### Sessions

- **Stockage** : En mémoire (peut être migré vers Redis)
- **Durée** : 24 heures
- **Cookies** : HttpOnly, SameSite=Lax
- **Expiration** : Nettoyage automatique des sessions expirées

## Endpoints API

### Authentification (Public)

#### POST /api/auth/login

**Request** :
```json
{
  "email": "user@example.com",
  "password": "password123"
}
```

**Response** :
```json
{
  "user": {
    "id": 1,
    "email": "user@example.com",
    "nom": "Dupont",
    "prenom": "Jean",
    "machine_id": 1
  }
}
```

**Cookies** : `session_id` est défini

#### POST /api/auth/logout

**Response** :
```json
{
  "status": "ok"
}
```

**Cookies** : `session_id` est supprimé

#### POST /api/auth/register

**Request** :
```json
{
  "email": "newuser@example.com",
  "password": "password123",
  "nom": "Martin",
  "prenom": "Pierre",
  "no_serie": "ABC123",
  "question": "What is your favorite color?",
  "reponse": "blue",
  ...
}
```

**Response** :
```json
{
  "status": "ok",
  "guid": "uuid-here",
  "message": "Account created. Please check your email to validate your account."
}
```

#### GET /api/auth/validate?guid=...&email=...

**Response** :
```json
{
  "status": "ok",
  "code": "1234  5678  9012  3456  7890  1234  5678  9012",
  "message": "Account validated. Use this code on your Essensys control panel."
}
```

#### POST /api/auth/forgot-password

**Request** :
```json
{
  "email": "user@example.com"
}
```

**Response** :
```json
{
  "status": "ok",
  "message": "A new password has been sent to your email."
}
```

### Utilisateur (Authentifié)

#### GET /api/user/me

**Response** :
```json
{
  "user": {
    "id": 1,
    "email": "user@example.com",
    "nom": "Dupont",
    "prenom": "Jean",
    "adr1": "123 Rue Example",
    "cp": "75001",
    "ville": "Paris",
    "phone": "0123456789",
    "question": "What is your favorite color?",
    "send_infos": true,
    "machine_id": 1
  }
}
```

#### PUT /api/user/me

**Request** :
```json
{
  "nom": "Martin",
  "prenom": "Pierre",
  "current_password": "oldpass",
  "new_password": "newpass",
  "current_response": "blue",
  "question": "New question?",
  "reponse": "new answer"
}
```

#### POST /api/user/close-account

**Response** :
```json
{
  "status": "ok",
  "message": "Account closed successfully"
}
```

#### POST /api/user/test-question

**Request** :
```json
{
  "response": "blue"
}
```

**Response** :
```json
{
  "response_is_ok": true
}
```

## Utilisation

### Configuration du Router

```go
// Avec base de données (recommandé pour Phase 3+)
db, err := database.Connect(database.Config{...})
sessionStore := auth.NewSessionStore()
webHandler := api.NewWebHandler(db, sessionStore)
router := api.NewRouterWithDB(handler, db, credentials, authEnabled)

// Ou sans base de données (legacy uniquement)
router := api.NewRouter(handler, credentials, authEnabled)
```

### Exemple de Login

```go
// Frontend React
const response = await fetch('http://localhost/api/auth/login', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  credentials: 'include', // Important pour les cookies
  body: JSON.stringify({
    email: 'user@example.com',
    password: 'password123'
  })
});

const data = await response.json();
// Session cookie est automatiquement défini
```

## Compatibilité Legacy

### Hash SHA1

Le hash SHA1 utilise UTF-16 (comme C#) pour garantir la compatibilité :
- Mots de passe legacy existants fonctionnent sans modification
- Nouveaux utilisateurs utilisent le même format
- Migration transparente

### Format d'Activation Code

Le code d'activation est formaté comme le legacy :
- Format : "1234  5678  9012  3456..."
- 32 caractères numériques
- Espaces tous les 4 caractères

## Sécurité

### Sessions

- **HttpOnly** : Les cookies ne sont pas accessibles via JavaScript
- **SameSite=Lax** : Protection CSRF
- **Expiration** : 24 heures
- **Nettoyage** : Sessions expirées supprimées automatiquement

### Mots de Passe

- **Hash SHA1** : Compatible legacy (à migrer vers bcrypt/argon2 plus tard)
- **Stockage** : Jamais en clair
- **Validation** : Vérification du hash

### CORS

- **Origines autorisées** : Configurables
- **Credentials** : Supporté pour les cookies de session
- **Headers** : Content-Type, Authorization

## Prochaines Étapes

La Phase 4 consiste à implémenter les services métier (Alarme, Chauffage, Arrosage, etc.).

## Notes Importantes

1. **Compatibilité Legacy** : Les hash SHA1 utilisent UTF-16 pour correspondre exactement au comportement C#
2. **Sessions en Mémoire** : Pour la production, considérer Redis ou base de données
3. **CORS** : Configurer les origines autorisées selon l'environnement
4. **Email** : L'envoi d'emails n'est pas encore implémenté (TODO)


