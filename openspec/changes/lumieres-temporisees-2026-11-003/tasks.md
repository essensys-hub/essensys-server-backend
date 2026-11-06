## 2. Gateway : temporisations (essensys-server-backend, CM5 et Raspberry Pi) (#17)

- [x] 2.1 Ajouter le catalogue des six lumières, la validation et les constantes de clés ; vérifier par des tests (`internal/core/light_timers_test.go`)
- [x] 2.2 Ajouter `GET /api/admin/lights/timers` et les six clés à la rotation de lecture et au snapshot ; vérifier par des tests (`NR-server-backend-26`, `internal/api/handlers_light_timers_test.go`)
- [x] 2.3 Ajouter `PUT /api/admin/lights/timers/{id}` derrière `adminOnly`, avec file d'actions et audit ; vérifier par des tests de routeur (`NR-server-backend-27` : lan_admin accepté, lan_user 403, sans session 401, IAM LAN désactivé, hors plage 400, dry-run)
- [x] 2.4 Accepter les six clés dans le filtre de poussée vers le cloud ; vérifier par `TestPushIndicesFromProfilesList_temporisationsLumieres`
