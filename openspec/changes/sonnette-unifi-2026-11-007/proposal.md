> Ticket : [essensys-hub/essensys-feature-lifecycle#58](https://github.com/essensys-hub/essensys-feature-lifecycle/issues/58). Feature ID : `sonnette-unifi-2026-11-007`. Sub-issue : essensys-hub/essensys-server-backend#25.
> Feature répartie sur plusieurs dépôts. Le change commun (proposal, design, contrat d'API, tâches de tous les dépôts) est dans `essensys-feature-lifecycle/openspec/changes/sonnette-unifi-2026-11-007`. Ce change ne décrit que le code de **essensys-server-backend** (gateway CM5).

## Why

La CM5 sait déjà parler à UniFi Protect (clé d'API, caméras, instantané) mais ne réagit à aucun événement. Un visiteur qui sonne n'est signalé ni dans le portail local ni par les lumières de la maison.

## What Changes

- Sources d'événements de sonnerie dans `internal/unifi` derrière une même interface (`EventSource`) : écoute du flux d'événements de la console (WebSocket) avec reconnexion à attente croissante (1 s à 60 s), et repli par interrogation de la caméra (toutes les 2 s) si le flux n'est pas exposé. Aucune adresse, clé ni identifiant journalisé.
- Service sonnette (`internal/core/doorbell.go`) : état `ringing` (60 s), une séquence à la fois, anti-rafale (30 s), clignotement des seules lumières choisies puis restauration de leur état d'origine, exclusion d'une lumière commandée à la main (observateur de l'`ActionService`). Écritures par le chemin existant des commandes de lumières ; l'essai `dry-run` n'écrit rien.
- Réglages (`internal/core/doorbell_settings.go`) : section `doorbell` du **même fichier** que les noms et les dispositions (clé `light_names.path`, **aucune nouvelle clé de configuration**), sections existantes préservées.
- Routes (`internal/api/handlers_doorbell.go`) : `GET /api/doorbell/status`, `GET /api/doorbell/stream` (instantanés `multipart/x-mixed-replace`), `GET/PUT /api/admin/doorbell/settings` et `POST /api/admin/doorbell/test` (`adminOnlyJSON`, dry-run).
- Sonde de faisabilité en lecture seule : `scripts/unifi-doorbell-probe.go` (tâche 1.1, exécutée par Nicolas avec sa clé).
- Aucun changement des endpoints legacy (`serverinfos`, `mystatus`, `myactions`, `done`).

## Capabilities

### New Capabilities

- `doorbell-gateway` : détection de la sonnette, clignotement et restauration, réglages, état, vidéo et essai.
