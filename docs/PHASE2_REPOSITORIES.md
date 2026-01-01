# Phase 2 : Couche de Persistance Go (Repositories) - Complétée ✅

## Résumé

La Phase 2 de la migration a été complétée avec succès. La couche de persistance Go avec repositories PostgreSQL a été créée, remplaçant NHibernate tout en maintenant la compatibilité avec le protocole legacy IoT.

## Fichiers créés

### Interface Store

- `internal/data/store.go` : Interface `Store` pour compatibilité legacy + interface étendue `DatabaseStoreInterface`

### Connection Database

- `internal/data/database/connection.go` : Gestion de la connexion PostgreSQL avec pool de connexions

### Repositories

- `internal/data/database/user_repository.go` : Repository pour les utilisateurs
- `internal/data/database/machine_repository.go` : Repository pour les machines
- `internal/data/database/action_repository.go` : Repository pour les actions
- `internal/data/database/state_repository.go` : Repository pour les états
- `internal/data/database/data_index_repository.go` : Repository pour les index de données

### Store Database

- `internal/data/database_store.go` : Implémentation de `Store` avec PostgreSQL (compatibilité legacy)

## Architecture

### Compatibilité Legacy

Le `DatabaseStore` implémente l'interface `Store` existante, permettant une transition transparente :

```go
// Avant (MemoryStore)
store := data.NewMemoryStore()

// Après (DatabaseStore) - même interface !
store := data.NewDatabaseStore(db)
```

**Aucun changement** dans les services existants (`ActionService`, `StatusService`) qui utilisent `Store`.

### Mapping ClientID → MachineID

Le protocole legacy utilise des `clientID` (strings comme "default" ou identifiants Basic Auth), tandis que la base de données utilise des `machine_id` (entiers).

Le `DatabaseStore` gère automatiquement ce mapping :
- Cache en mémoire pour les performances
- Résolution par `no_serie` (numéro de série)
- Fallback gracieux si machine non trouvée

### Repositories Pattern

Chaque repository gère une entité spécifique :

```go
// UserRepository
userRepo := database.NewUserRepository(db)
user, err := userRepo.GetByEmail("user@example.com")

// MachineRepository
machineRepo := database.NewMachineRepository(db)
machine, err := machineRepo.GetByNoSerie("ABC123")

// ActionRepository
actionRepo := database.NewActionRepository(db)
actions, err := actionRepo.GetPendingByMachineID(machineID)
```

## Fonctionnalités

### UserRepository

- `GetByID(id)` : Récupère un utilisateur par ID
- `GetByEmail(email)` : Récupère un utilisateur par email
- `GetByGuid(guid, email)` : Récupère un utilisateur par GUID (validation de compte)
- `Create(user)` : Crée un nouvel utilisateur
- `Update(user)` : Met à jour un utilisateur
- `Delete(id)` : Soft-delete (obsolete = true)
- `UpdateLastAccess(userID)` : Met à jour le dernier accès
- `CheckEmailExists(email)` : Vérifie si un email existe
- `CheckNoSerieExists(noSerie)` : Vérifie si un numéro de série existe

### MachineRepository

- `GetByID(id)` : Récupère une machine par ID
- `GetByNoSerie(noSerie)` : Récupère une machine par numéro de série
- `Create(machine)` : Crée une nouvelle machine
- `Update(machine)` : Met à jour une machine
- `GetByUserID(userID)` : Récupère la machine d'un utilisateur

### ActionRepository

- `Create(action, indexes)` : Crée une action avec ses index (transaction)
- `GetByGUID(guid)` : Récupère une action par GUID
- `GetPendingByMachineID(machineID)` : Récupère les actions en attente
- `GetIndexesByActionID(actionID)` : Récupère les index d'une action
- `MarkDone(guid)` : Marque une action comme terminée
- `DeleteByMachineID(machineID)` : Supprime toutes les actions en attente (purge)
- `GetByMachineIDAndDateRange(machineID, start, end)` : Récupère les actions dans une plage de dates

### StateRepository

- `Create(state, indexes)` : Crée un état avec ses index (transaction)
- `GetLatestByMachineID(machineID)` : Récupère le dernier état
- `GetByMachineIDAfter(machineID, after)` : Récupère les états après une date
- `GetIndexesByStateID(stateID)` : Récupère les index d'un état
- `GetLastCallTime(machineID)` : Récupère l'heure du dernier appel
- `HasRefreshed(machineID, since)` : Vérifie si la machine a rafraîchi depuis une date

### DataIndexRepository

- `GetByKey(key)` : Récupère un index par sa clé (ex: "605")
- `GetByID(id)` : Récupère un index par ID
- `GetOrCreateByKey(key)` : Récupère ou crée un index
- `GetAllActive()` : Récupère tous les index actifs

## Transactions

Les repositories utilisent des transactions pour les opérations complexes :

```go
// ActionRepository.Create utilise une transaction
tx, err := r.db.Beginx()
// Insert action
// Insert action indexes
tx.Commit()
```

## Pool de Connexions

La connexion PostgreSQL est configurée avec un pool :

```go
db.SetMaxOpenConns(25)      // Maximum 25 connexions ouvertes
db.SetMaxIdleConns(5)       // Maximum 5 connexions inactives
db.SetConnMaxLifetime(5 * time.Minute) // Durée de vie max d'une connexion
```

## Migration depuis MemoryStore

### Étape 1 : Configuration

```go
// Avant
store := data.NewMemoryStore()

// Après
db, err := database.Connect(database.Config{
    Host:     "localhost",
    Port:     5432,
    User:     "essensys",
    Password: "password",
    DBName:   "essensys",
    SSLMode:  "disable",
})
if err != nil {
    log.Fatal(err)
}
defer database.Close()

store := data.NewDatabaseStore(db)
```

### Étape 2 : Aucun changement dans les services

Les services existants continuent de fonctionner sans modification :

```go
// ActionService et StatusService utilisent l'interface Store
actionService := core.NewActionService(store)
statusService := core.NewStatusService(store)
```

### Étape 3 : Utilisation des repositories pour les nouveaux endpoints

Pour les nouveaux endpoints web, utilisez directement les repositories :

```go
userRepo := database.NewUserRepository(db)
user, err := userRepo.GetByEmail(email)
```

## Performance

### Cache ClientID → MachineID

Le `DatabaseStore` maintient un cache en mémoire pour éviter les requêtes répétées :

```go
clientCache map[string]int // clientID -> machineID
```

### Index de Base de Données

Les index créés dans la Phase 1 optimisent les requêtes :
- Index sur `machine_id` pour les actions et états
- Index sur `guid` pour les actions
- Index sur `mail` pour les utilisateurs
- Index sur `no_serie` pour les machines

## Tests

### Tests Unitaires

Les repositories peuvent être testés avec une base de données de test :

```go
func TestUserRepository(t *testing.T) {
    db := setupTestDB(t)
    defer teardownTestDB(t, db)
    
    repo := database.NewUserRepository(db)
    user, err := repo.GetByEmail("test@example.com")
    // ...
}
```

### Tests d'Intégration

Le `DatabaseStore` peut être testé avec l'interface `Store` :

```go
func TestDatabaseStore(t *testing.T) {
    db := setupTestDB(t)
    defer teardownTestDB(t, db)
    
    store := data.NewDatabaseStore(db)
    
    // Test Store interface
    store.SetValue("client1", 605, "1")
    value, exists := store.GetValue("client1", 605)
    assert.True(t, exists)
    assert.Equal(t, "1", value)
}
```

## Prochaines Étapes

La Phase 3 consiste à implémenter l'authentification et la gestion des utilisateurs pour le frontend React.

## Notes Importantes

1. **Compatibilité Legacy** : Le `DatabaseStore` maintient 100% de compatibilité avec le protocole legacy IoT
2. **Performance** : Le cache et les index optimisent les performances
3. **Transactions** : Les opérations complexes utilisent des transactions pour garantir la cohérence
4. **Extensibilité** : Les repositories peuvent être étendus avec de nouvelles méthodes sans affecter l'interface `Store`

