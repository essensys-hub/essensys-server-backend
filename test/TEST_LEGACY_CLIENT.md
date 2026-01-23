# Test Client Legacy IoT

Ce guide explique comment tester que le serveur Go fonctionne toujours avec le client legacy IoT embarqué.

## Vue d'Ensemble

Le test vérifie que :
1. Le serveur Go peut recevoir des requêtes du client legacy
2. Le client peut envoyer des status (`/api/mystatus`)
3. Le client peut recevoir des actions (`/api/myactions`)
4. Le protocole legacy (JSON malformé, headers spécifiques) fonctionne toujours

## Prérequis

1. **Go installé** (version 1.19+)
2. **Python 3 installé** avec le module `requests`
   ```bash
   pip3 install requests
   ```
3. **Accès au port 80** (nécessite sudo sur Linux/macOS)
4. **Aucun autre service sur le port 80** (Apache, nginx, etc.)

## Test Automatique

### Script de test complet

```bash
cd essensys-server-backend
./test/test_legacy_client.sh
```

Ce script :
1. Compile le serveur Go
2. Configure les permissions pour le port 80
3. Lance le serveur en arrière-plan
4. Attend que le serveur soit prêt
5. Exécute `test_chb3.py` qui simule le client legacy
6. Arrête le serveur
7. Affiche les logs

### Résultat attendu

```
==========================================
Test Client Legacy IoT
==========================================

1. Compilation du serveur Go...
   ✓ Serveur compilé

2. Vérification du port 80...
   ✓ Port disponible

3. Configuration des permissions port 80...
   ✓ Permissions configurées

4. Démarrage du serveur Go...
   Serveur démarré (PID: 12345)
   Logs: test/server.log

5. Attente du démarrage du serveur...
   ✓ Serveur prêt

6. Exécution du test client legacy (test_chb3.py)...

--- STEP 1: Turn ON ---
[TEST] Injecting Action: k=613, v=64
[TEST] Injection OK
[TEST] Verifying action content...
[TEST] Verification SUCCESS: JSON matches standard format.
[TEST] Waiting for client to acknowledge (queue empty)...
[TEST] Action queue empty. Done.

[TEST] Waiting 10 seconds...

--- STEP 2: Turn OFF ---
[TEST] Injecting Action: k=607, v=64
[TEST] Injection OK
[TEST] Verifying action content...
[TEST] Verification SUCCESS: JSON matches standard format.
[TEST] Waiting for client to acknowledge (queue empty)...
[TEST] Action queue empty. Done.

[TEST] Sequence Completed Successfully.

==========================================
✓ Test réussi ! Le client legacy fonctionne
==========================================
```

## Test Manuel

### Étape 1 : Lancer le serveur

```bash
cd essensys-server-backend

# Compiler
go build -o server ./cmd/server

# Configurer les permissions (Linux)
sudo setcap 'cap_net_bind_service=+ep' ./server

# Ou lancer avec sudo (macOS/Linux)
sudo ./server
```

Le serveur devrait afficher :
```
Server starting on :80
Health check available at: http://localhost:80/health
HTTP server listening (legacy-compatible mode)...
```

### Étape 2 : Vérifier que le serveur répond

Dans un autre terminal :
```bash
# Test health check
curl http://localhost/health
# Devrait retourner: {"status":"ok"}

# Test serverinfos (endpoint legacy)
curl http://localhost/api/serverinfos
# Devrait retourner: {"isconnected":true,"infos":[...],"newversion":"no"}
```

### Étape 3 : Lancer le test client

Dans un autre terminal :
```bash
cd essensys-server-backend/test
python3 test_chb3.py
```

### Étape 4 : Observer les logs

Le serveur devrait afficher des logs comme :
```
[GO] GET /api/serverinfos
[REQUEST] 2025-01-08T10:22:45+01:00 GET /api/serverinfos 127.0.0.1:54321
[RESPONSE] /api/serverinfos 200 1.2ms

[GO] POST /api/mystatus
[REQUEST] 2025-01-08T10:22:45+01:00 POST /api/mystatus 127.0.0.1:54321
[GO] Status Update (Version: V37, Items: 12)
[RESPONSE] /api/mystatus 201 2.5ms

[GO] GET /api/myactions
[REQUEST] 2025-01-08T10:22:45+01:00 GET /api/myactions 127.0.0.1:54321
[GO] Sending Actions: {"_de67f":null,"actions":[...]}
[RESPONSE] /api/myactions 200 0.8ms
```

## Ce que teste test_chb3.py

Le script `test_chb3.py` simule un client legacy et teste :

1. **Injection d'action** : Envoie une action via `/api/admin/inject`
2. **Vérification du format** : Vérifie que la réponse `/api/myactions` contient :
   - Le champ `_de67f` en premier
   - Tous les indices 605-622 présents
   - L'index 590 (scenario trigger) avec valeur "1"
   - L'index cible (613 ou 607) avec la bonne valeur
3. **Attente d'acknowledge** : Attend que la queue soit vide (action exécutée)

### Séquence de test

1. **Turn ON** : Injecte action pour allumer (index 613 = 64)
2. **Vérification** : Vérifie le format JSON de la réponse
3. **Attente** : Attend que le client acknowledge
4. **Pause** : Attend 10 secondes
5. **Turn OFF** : Injecte action pour éteindre (index 607 = 64)
6. **Vérification** : Vérifie le format JSON de la réponse
7. **Attente** : Attend que le client acknowledge

## Dépannage

### Erreur : "bind: permission denied"

**Solution** : Configurer les permissions ou utiliser sudo
```bash
# Linux
sudo setcap 'cap_net_bind_service=+ep' ./server

# macOS (pas de setcap)
sudo ./server
```

### Erreur : "bind: address already in use"

**Solution** : Arrêter le service qui utilise le port 80
```bash
# Trouver le processus
sudo lsof -i :80

# Arrêter (exemple avec Apache)
sudo systemctl stop apache2
# ou
sudo systemctl stop nginx
```

### Erreur : "Connection refused" dans test_chb3.py

**Solution** : Vérifier que le serveur est bien démarré
```bash
# Vérifier que le serveur écoute
sudo lsof -i :80

# Tester manuellement
curl http://localhost/health
```

### Erreur : "No actions found to verify"

**Solution** : Le client n'a pas récupéré l'action. Vérifier :
1. Que le serveur est bien démarré
2. Que les logs montrent l'injection
3. Que `/api/myactions` retourne bien des actions

### Le test échoue sur la vérification du format JSON

**Solution** : Vérifier que le serveur génère bien le format complet :
- Tous les indices 605-622 doivent être présents
- L'index 590 doit être présent avec valeur "1"
- Le champ `_de67f` doit être en premier

## Test avec DatabaseStore

Pour tester avec PostgreSQL au lieu de MemoryStore :

1. **Modifier main.go** temporairement :
```go
// Avant
store := data.NewMemoryStore()

// Après
db, err := database.Connect(database.Config{
    Host:     "localhost",
    Port:     5432,
    User:     "postgres",
    Password: "postgres",
    DBName:   "essensys",
    SSLMode:  "disable",
})
if err != nil {
    log.Fatalf("Failed to connect to database: %v", err)
}
defer database.Close()
store := data.NewDatabaseStore(db)
```

2. **Appliquer les migrations** :
```bash
psql -U postgres -d essensys -f migrations/001_initial_schema.up.sql
```

3. **Relancer le test** :
```bash
./test/test_legacy_client.sh
```

## Vérification de la Compatibilité

Le test vérifie que :
- ✅ Le protocole legacy fonctionne toujours
- ✅ Le format JSON est correct (champ `_de67f` en premier)
- ✅ Tous les indices 605-622 sont présents
- ✅ L'index 590 (scenario trigger) est présent
- ✅ Les actions sont bien reçues et exécutées

## Prochaines Étapes

Une fois le test réussi :
1. ✅ Le serveur Go fonctionne avec le client legacy
2. ✅ Le protocole legacy est maintenu à 100%
3. ✅ On peut continuer avec la Phase 3 (authentification)




