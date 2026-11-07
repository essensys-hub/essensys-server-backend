## 2. Gateway : noms des volets (essensys-server-backend, CM5) (#21)

- [x] 2.1 Refactorer le magasin de fichier pour porter deux sections, `lights` et `shutters`, sous la même clé `light_names.path` ; vérifier par des tests (`internal/core/shutter_names_test.go` : fichier de #41 relu et complété, écritures croisées sans perte, section inconnue conservée, relecture après redémarrage, stockage absent) et par la non-régression de `NR-server-backend-28` et `NR-server-backend-29`
- [x] 2.2 Ajouter le catalogue des 16 volets et la validation ; vérifier par des tests (`internal/core/shutter_names_test.go`)
- [x] 2.3 Ajouter `GET /api/shutters/names` ; vérifier par des tests de routeur (`internal/api/handlers_shutter_names_test.go`)
- [x] 2.4 Ajouter `PUT /api/admin/shutters/names/{id}` derrière `adminOnlyJSON`, avec audit `shutter_name` et dry-run ; vérifier par des tests de routeur (`NR-server-backend-30`)
- [x] 2.5 Ajouter la synchronisation dans les deux sens dans l'agent cloud ; vérifier par des tests avec un faux cloud (`internal/cloudsync/shutter_names_sync_test.go`, `NR-server-backend-31`)
- [x] 2.6 Documenter la section `shutters` dans `config.yaml.example` et le README ; aucune nouvelle clé de configuration (`internal/config/config.go` inchangé)
