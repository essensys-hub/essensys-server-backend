# Phase 1 : Schéma PostgreSQL et Modèles Go - Complétée ✅

## Résumé

La Phase 1 de la migration a été complétée avec succès. Le schéma de base de données PostgreSQL et tous les modèles Go correspondants ont été créés.

## Fichiers créés

### Migrations SQL

- `migrations/001_initial_schema.up.sql` : Schéma complet avec toutes les tables
- `migrations/001_initial_schema.down.sql` : Script de rollback
- `migrations/README.md` : Documentation des migrations

### Modèles Go

- `internal/models/user.go` : Modèle utilisateur
- `internal/models/machine.go` : Modèle machine (boîtier Essensys)
- `internal/models/action.go` : Modèles action et action_index
- `internal/models/state.go` : Modèles state et state_index
- `internal/models/data_index.go` : Modèle référentiel des indices
- `internal/models/version.go` : Modèles version et version_machine
- `internal/models/phone.go` : Modèles phone et smssend
- `internal/models/cle_machine.go` : Modèle clé d'activation

## Structure de la Base de Données

### Tables Principales

1. **es_data_index** : Référentiel des indices (605, 613, etc.)
2. **es_machine** : Boîtiers Essensys
3. **es_user** : Utilisateurs
4. **es_cle_machine** : Clés d'activation
5. **es_phone** : Numéros de téléphone
6. **es_sms_send** : Historique des SMS
7. **es_action** : Actions à exécuter
8. **es_action_index** : Paramètres des actions
9. **es_state** : Snapshots d'état
10. **es_state_index** : Valeurs des index dans un état
11. **es_version** : Versions de firmware
12. **es_version_machine** : Suivi des téléchargements

### Relations

```
es_machine (1) ──< (N) es_user
es_machine (1) ──< (N) es_action
es_machine (1) ──< (N) es_state
es_machine (1) ──< (N) es_version_machine
es_user (1) ──< (N) es_phone
es_user (1) ──< (N) es_sms_send
es_action (1) ──< (N) es_action_index
es_state (1) ──< (N) es_state_index
es_data_index (1) ──< (N) es_action_index
es_data_index (1) ──< (N) es_state_index
```

## Caractéristiques

### Index Créés

- Index sur les clés uniques (mail, guid, no_serie, cle)
- Index sur les clés étrangères pour les performances
- Index sur les champs fréquemment recherchés (is_done, is_valid, etc.)

### Contraintes

- Clés primaires auto-incrémentées (SERIAL)
- Clés étrangères avec ON DELETE CASCADE ou SET NULL selon le cas
- Contraintes UNIQUE sur les champs critiques
- Valeurs par défaut pour les booléens et timestamps

### Sécurité

- Les mots de passe sont hashés (SHA1 legacy)
- Les clés privées (`pkey`) ne sont jamais exposées dans les modèles JSON
- Les champs sensibles utilisent `json:"-"` pour éviter la sérialisation

## Migration depuis SQL Server

Le schéma PostgreSQL est compatible avec le schéma SQL Server legacy :
- Même structure de tables
- Même nommage (en minuscules pour PostgreSQL)
- Même logique de relations
- Types de données adaptés (TIMESTAMP au lieu de DATETIME)

## Prochaines Étapes

La Phase 2 consiste à implémenter la couche de persistance Go (repositories) pour remplacer NHibernate.

## Utilisation

### Appliquer les migrations

```bash
# Avec migrate
migrate -path ./migrations -database "postgres://user:password@localhost/essensys?sslmode=disable" up

# Ou manuellement
psql -U user -d essensys -f migrations/001_initial_schema.up.sql
```

### Utiliser les modèles

```go
import "github.com/essensys-hub/essensys-server-backend/internal/models"

user := models.User{
    Mail: "user@example.com",
    Nom: "Dupont",
    Prenom: "Jean",
    // ...
}
```

## Notes Importantes

1. **Compatibilité Legacy** : Les mots de passe utilisent SHA1 (comme le système legacy) pour permettre la migration sans réinitialisation
2. **Performance** : Les index ont été créés sur tous les champs fréquemment recherchés
3. **Sécurité** : Les champs sensibles ne sont jamais sérialisés en JSON
4. **Extensibilité** : Le schéma peut être étendu avec de nouvelles migrations sans affecter les données existantes




