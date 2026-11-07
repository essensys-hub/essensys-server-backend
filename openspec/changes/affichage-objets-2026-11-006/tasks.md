## 2. Gateway : disposition des objets (essensys-server-backend, CM5) (#23)

- [x] 2.1 Ajouter au magasin de fichier les sections `lights_layout` et `shutters_layout` (objets, décodeur dédié, sans passer par `namesDoc.section()`), même fichier, même clé `light_names.path` ; vérifier par des tests (`internal/core/object_layout_test.go` : fichier de #52 relu et complété, écritures croisées sans perte des autres sections ni d'une section inconnue, relecture après redémarrage, stockage absent, section illisible non écrasée) et par la non-régression de `NR-server-backend-28` à `NR-server-backend-31`
- [x] 2.2 Ajouter le modèle, la validation et `effective` (catalogues de 33 lumières et 16 volets) ; vérifier par des tests de domaine (`internal/core/object_layout_test.go`)
- [x] 2.3 Ajouter `GET /api/lights/layout` et `GET /api/shutters/layout` ; vérifier par des tests de routeur (`internal/api/handlers_object_layout_test.go`)
- [x] 2.4 Ajouter `PUT /api/admin/lights/layout` et `PUT /api/admin/shutters/layout` derrière `adminOnlyJSON`, avec audit `object_layout`, `reset` et dry-run ; vérifier par des tests de routeur (`NR-server-backend-32`)
- [x] 2.5 Ajouter la synchronisation dans les deux sens dans l'agent cloud ; vérifier par des tests avec un faux cloud (`internal/cloudsync/object_layout_sync_test.go`, `NR-server-backend-33`)
- [x] 2.6 Documenter les sections `lights_layout` et `shutters_layout` dans `config.yaml.example` et le README ; aucune nouvelle clé de configuration (`internal/config/config.go` inchangé)
