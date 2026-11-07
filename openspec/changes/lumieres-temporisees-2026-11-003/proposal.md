> Ticket : [essensys-hub/essensys-feature-lifecycle#45](https://github.com/essensys-hub/essensys-feature-lifecycle/issues/45). Feature ID : `lumieres-temporisees-2026-11-003`. Sub-issue : essensys-hub/essensys-server-backend#17.
> Feature répartie sur plusieurs dépôts. Le change commun (proposal, design, contrat d'API, tâches de tous les dépôts) est dans `essensys-feature-lifecycle/openspec/changes/lumieres-temporisees-2026-11-003`. Ce change ne décrit que le code de **essensys-server-backend** (gateway CM5 et Raspberry Pi).

## Why

Régler, depuis les apps et les portails, le délai d'extinction automatique de six lumières (cases 521, 555, 556, 557, 561, 562 de la table d'échange, 0 à 255 minutes, 0 = jamais), réservé à l'administrateur local.

## What Changes

- Catalogue et validation des six lumières, constantes de clés (`pkg/protocol`, `internal/core/light_timers.go`).
- `GET /api/admin/lights/timers` et `PUT /api/admin/lights/timers/{id}` derrière `lan_admin`, avec file d'actions, audit et dry-run.
- Les six clés entrent dans la rotation `serverinfos` et le snapshot (`comfort.light_timers`) ; une clé jamais lue est `null`.
- Le filtre de poussée cloud accepte ces clés (profil de synchro du cloud).
- Aucun changement des endpoints legacy (`serverinfos`, `mystatus`, `myactions`, `done`).

## Capabilities

### New Capabilities

- `light-timers-gateway` : lecture, écriture, rotation, snapshot et poussée cloud des temporisations.
