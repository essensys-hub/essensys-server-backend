## 2. Gateway : noms des lumières (essensys-server-backend, CM5 et Raspberry Pi) (#19)

- [x] 2.1 Ajouter le catalogue et la validation et le magasin de fichier atomique (`light_names.path`) ; vérifier par des tests (`internal/core/light_names_test.go`, `NR-server-backend-28`)
- [x] 2.2 Ajouter `GET /api/lights/names` (utilisateur authentifié, ouvert si IAM désactivé) ; vérifier par des tests de routeur (`internal/api/handlers_light_names_test.go`)
- [x] 2.3 Ajouter `PUT /api/admin/lights/names/{id}` derrière `adminOnlyJSON`, avec audit et dry-run ; vérifier par des tests de routeur (`NR-server-backend-29` : lan_admin accepté, lan_user 403, 400 invalides, 503 sans stockage)
- [x] 2.4 Ajouter la synchronisation dans les deux sens dans l'agent cloud ; vérifier par des tests avec un faux cloud (`internal/cloudsync/light_names_sync_test.go` : cloud plus récent, gateway plus récente, hors ligne puis retour, conflit)
