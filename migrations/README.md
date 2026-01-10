# Migrations de Base de Données

Ce dossier contient les migrations SQL pour PostgreSQL.

## Structure

- `001_initial_schema.up.sql` : Création du schéma initial
- `001_initial_schema.down.sql` : Rollback du schéma initial

## Utilisation

### Avec migrate (recommandé)

```bash
# Installer migrate
go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest

# Appliquer les migrations
migrate -path ./migrations -database "postgres://user:password@localhost/essensys?sslmode=disable" up

# Rollback
migrate -path ./migrations -database "postgres://user:password@localhost/essensys?sslmode=disable" down
```

### Manuellement

```bash
# Appliquer
psql -U user -d essensys -f migrations/001_initial_schema.up.sql

# Rollback
psql -U user -d essensys -f migrations/001_initial_schema.down.sql
```

## Schéma de Base de Données

### Tables Principales

- **es_user** : Utilisateurs du système
- **es_machine** : Boîtiers Essensys (terminaux)
- **es_action** : Actions à exécuter par les machines
- **es_state** : Snapshots de l'état des machines
- **es_data_index** : Référentiel des indices de données
- **es_version** : Versions de firmware disponibles
- **es_version_machine** : Suivi des téléchargements de versions

### Relations

- Un utilisateur (`es_user`) est associé à une machine (`es_machine`)
- Une machine peut avoir plusieurs utilisateurs
- Une action (`es_action`) appartient à une machine et contient plusieurs index (`es_action_index`)
- Un état (`es_state`) appartient à une machine et contient plusieurs index (`es_state_index`)
- Les index référencent le référentiel `es_data_index`

## Notes

- Les mots de passe sont stockés en SHA1 (legacy) pour la compatibilité
- Les clés privées (`pkey`) ne doivent jamais être exposées via l'API
- Les timestamps utilisent le type `TIMESTAMP` PostgreSQL
- Les booléens utilisent le type `BOOLEAN` PostgreSQL


