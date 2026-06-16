# Test Client Legacy IoT

## Description

Le script `test_legacy_client.sh` lance le serveur Go et exécute `test_chb3.py` pour simuler un client IoT legacy avec authentification Basic Auth.

## Prérequis

1. **Base de données PostgreSQL** avec la machine de test :
   ```sql
   SELECT * FROM es_machine WHERE hashed_pkey = '5e6e0e1ffd940ee5649cf65b1d7a4df8' AND is_active = true;
   ```

2. **Configuration du serveur** (`config.yaml`) :
   ```yaml
   auth:
     enabled: true
   database:
     enabled: true
   ```

## Utilisation

```bash
cd /Users/nrineau/ESSENSYS/essensys-server-backend
./test/test_legacy_client.sh
```

## Fonctionnement

1. **Compilation** : Compile le serveur Go
2. **Démarrage** : Lance le serveur sur le port 80
3. **Tests** : Exécute `test_chb3.py` qui :
   - Teste `GET /api/serverinfos` (sans auth)
   - Teste `POST /api/mystatus` (avec auth)
   - Injecte des actions via `POST /api/admin/inject` (avec auth)
   - Récupère les actions via `GET /api/myactions` (avec auth)
   - Vérifie le format des réponses

## Authentification

Le script utilise l'authentification Basic Auth avec :
- **Hashed Pkey** : `5e6e0e1ffd940ee5649cf65b1d7a4df8` (exemple de test)
- **Token** : `Basic <base64(username:password)>` (calculé à partir du hashed_pkey)

Le token est calculé en divisant le `hashed_pkey` en deux parties :
- `username` = 16 premiers caractères hex
- `password` = 16 derniers caractères hex
- Encodage Base64 de `username:password`

## Machine de Test

Le script utilise la machine avec :
- **No Serie** : `TEST_CLIENT_001` ou `ESS002SU1702280040`
- **Hashed Pkey** : `5e6e0e1ffd940ee5649cf65b1d7a4df8`
- **Is Active** : `true`

## Logs

Les logs du serveur sont sauvegardés dans `test/server.log`.

## Dépannage

### Erreur : "401 Unauthorized"
- Vérifiez que `auth.enabled: true` dans `config.yaml`
- Vérifiez que la machine existe en base avec le bon `hashed_pkey`
- Vérifiez que `is_active = true`

### Erreur : "database does not exist"
- Vérifiez que `database.enabled: true` dans `config.yaml`
- Vérifiez que la base de données `essensys` existe
- Vérifiez les paramètres de connexion (host, port, user, password)

### Le test échoue
- Vérifiez les logs dans `test/server.log`
- Vérifiez que le serveur est bien démarré : `curl http://localhost/health`
- Vérifiez que le port 80 n'est pas utilisé par un autre processus




