# Tests Phase 2 : Repositories PostgreSQL

Ce dossier contient les tests pour la Phase 2 de la migration.

## Prérequis

1. **PostgreSQL installé et démarré**
   ```bash
   # Vérifier que PostgreSQL est démarré
   psql --version
   ```

2. **Base de données de test créée**
   ```bash
   createdb essensys_test
   ```

3. **Variables d'environnement (optionnel)**
   ```bash
   export DB_HOST=localhost
   export DB_PORT=5432
   export DB_USER=postgres
   export DB_PASSWORD=postgres
   export DB_NAME=essensys_test
   ```

## Tests Automatiques

### Script de test complet

```bash
./test/phase2_test.sh
```

Ce script :
1. Vérifie la connexion PostgreSQL
2. Crée la base de données de test si nécessaire
3. Applique les migrations
4. Exécute les tests Go

### Tests unitaires Go

```bash
# Tous les tests
go test ./internal/data/database/... -v

# Tests spécifiques
go test ./internal/data/database/... -v -run TestUserRepository
go test ./internal/data/database/... -v -run TestMachineRepository
go test ./internal/data/database/... -v -run TestDatabaseStore
```

## Tests Manuels

### Test manuel interactif

```bash
# Avec variables d'environnement par défaut
go run test/phase2_manual_test.go

# Avec variables d'environnement personnalisées
DB_HOST=localhost DB_USER=myuser DB_PASSWORD=mypass go run test/phase2_manual_test.go
```

Ce script teste :
1. Connexion à PostgreSQL
2. UserRepository (création, récupération, vérification)
3. MachineRepository (création, récupération)
4. DatabaseStore (interface Store)
5. ActionRepository (création, récupération, marquage)
6. StateRepository (création, récupération, vérification)

## Structure des Tests

### Tests Unitaires (`internal/data/database/store_test.go`)

- `TestDatabaseStore_StoreInterface` : Vérifie que DatabaseStore implémente Store
- `TestDatabaseStore_GetSetValue` : Test des opérations exchange table
- `TestDatabaseStore_ActionQueue` : Test de la queue d'actions
- `TestUserRepository` : Test complet du repository utilisateur
- `TestMachineRepository` : Test complet du repository machine

### Tests Manuels (`test/phase2_manual_test.go`)

Script interactif qui teste tous les repositories et affiche les résultats.

## Résultats Attendus

### Tests Unitaires

```
=== RUN   TestDatabaseStore_StoreInterface
--- PASS: TestDatabaseStore_StoreInterface (0.01s)
=== RUN   TestDatabaseStore_GetSetValue
--- PASS: TestDatabaseStore_GetSetValue (0.05s)
=== RUN   TestDatabaseStore_ActionQueue
--- PASS: TestDatabaseStore_ActionQueue (0.08s)
=== RUN   TestUserRepository
--- PASS: TestUserRepository (0.12s)
=== RUN   TestMachineRepository
--- PASS: TestMachineRepository (0.10s)
PASS
ok      github.com/essensys-hub/essensys-server-backend/internal/data/database    0.456s
```

### Tests Manuels

```
==========================================
Test Manuel Phase 2 : Repositories
==========================================

1. Connexion à PostgreSQL...
   ✓ Connexion réussie

2. Test UserRepository...
   - Utilisateur créé (ID: 1)
   - Utilisateur récupéré par email: test-manual@example.com
   - Email vérifié: existe = true
   ✓ UserRepository OK

3. Test MachineRepository...
   - Machine créée (ID: 1, NoSerie: TEST-MANUAL-001)
   - Machine récupérée par no_serie: TEST-MANUAL-001
   ✓ MachineRepository OK

4. Test DatabaseStore (interface Store)...
   - SetValue/GetValue OK (index 605 = 1)
   - GetAllValues OK (2 valeurs)
   - EnqueueAction OK (GUID: test-guid-manual-123)
   - DequeueActions OK (1 actions)
   - AcknowledgeAction OK
   ✓ DatabaseStore OK

5. Test ActionRepository...
   - Action créée (ID: 1, GUID: test-action-manual-123)
   - Actions en attente: 1
   - Action marquée comme terminée
   ✓ ActionRepository OK

6. Test StateRepository...
   - État créé (ID: 1)
   - Dernier état récupéré (ID: 1)
   - Machine rafraîchie: true
   ✓ StateRepository OK

==========================================
Tous les tests manuels sont passés !
==========================================
```

## Dépannage

### Erreur : "cannot connect to test database"

**Solution** : Vérifiez que PostgreSQL est démarré et accessible
```bash
# Vérifier le statut
sudo systemctl status postgresql  # Linux
brew services list | grep postgresql  # macOS

# Tester la connexion
psql -h localhost -U postgres -d postgres -c "SELECT 1"
```

### Erreur : "database does not exist"

**Solution** : Créez la base de données de test
```bash
createdb essensys_test
```

### Erreur : "relation does not exist"

**Solution** : Appliquez les migrations
```bash
psql -h localhost -U postgres -d essensys_test -f migrations/001_initial_schema.up.sql
```

### Erreur : "permission denied"

**Solution** : Vérifiez les permissions PostgreSQL
```bash
# Créer un utilisateur de test
createuser -s testuser
# Ou utiliser l'utilisateur postgres existant
```

## Nettoyage

Après les tests, vous pouvez nettoyer la base de données de test :

```bash
# Supprimer toutes les données
psql -h localhost -U postgres -d essensys_test -c "TRUNCATE TABLE es_state_index, es_state, es_action_index, es_action, es_user, es_machine, es_data_index CASCADE;"

# Ou supprimer complètement la base de données
dropdb essensys_test
```

## Prochaines Étapes

Une fois les tests passés, vous pouvez :
1. Intégrer le DatabaseStore dans le serveur principal
2. Configurer la connexion PostgreSQL dans la configuration
3. Passer à la Phase 3 : Authentification




