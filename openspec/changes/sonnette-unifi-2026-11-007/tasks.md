## 1. Gateway : sonnette UniFi (essensys-server-backend) (#25)

- [x] 1.1 (fait le 2026-11-08 : flux `ring` reçu sur la caméra réelle, instantanés JPEG 130 à 270 ms, `lastRing` en 401 donc sans repli ; cadence de l'armoire et ffmpeg restent à relever) Essai de faisabilité sur la console réelle, en lecture seule, par Nicolas avec `scripts/unifi-doorbell-probe.go` (variables d'environnement `UNIFI_BASE_URL` et `UNIFI_API_KEY`, rien d'écrit) ; consigner le compte rendu sur l'issue (événement de sonnerie exposé ou repli, cadence de l'armoire, ffmpeg) et ajuster `DoorbellPhase`
- [x] 1.2 Étendre `internal/unifi` (écoute des événements, reconnexion avec attente croissante, repli par interrogation) ; vérifier par des tests avec un faux serveur UniFi (`internal/unifi/events_test.go`)
- [x] 1.3 Ajouter le service sonnette ; vérifier par des tests de service avec un écrivain simulé (`internal/core/doorbell_test.go`)
- [x] 1.4 Ajouter le magasin des réglages (section `doorbell`) et `GET/PUT /api/admin/doorbell/settings` ; vérifier par des tests (`internal/core/doorbell_test.go`, `internal/api/handlers_doorbell_test.go`)
- [x] 1.5 Ajouter `GET /api/doorbell/status`, `GET /api/doorbell/stream` et `POST /api/admin/doorbell/test` (dry-run) ; vérifier par des tests de routeur (`internal/api/handlers_doorbell_test.go`)
- [x] 1.6 Ajouter le test de non-régression `NR-server-backend-34` (ticket `#58`), documenter dans le README ; vérifier par `go build`, `go vet`, `go test ./...`, `validate_feature_manifests.py` et `check_feature_gate.py --strict`
