> Ticket : [essensys-hub/essensys-feature-lifecycle#52](https://github.com/essensys-hub/essensys-feature-lifecycle/issues/52). Feature ID : `noms-volets-2026-11-005`. Sub-issue : essensys-hub/essensys-server-backend#21.
> Feature répartie sur plusieurs dépôts. Le change commun (proposal, design, contrat d'API, tâches de tous les dépôts) est dans `essensys-feature-lifecycle/openspec/changes/noms-volets-2026-11-005`. Ce change ne décrit que le code de **essensys-server-backend** (gateway CM5 et Raspberry Pi).

## Why

Permettre à l'administrateur de renommer chacun des 16 volets et stores (« Volet 1 Salon » devient « Volet baie vitrée »), sur le réseau local comme dans le cloud, avec un retour possible au nom d'origine. Jumeau de `noms-lumieres-2026-11-004`.

## What Changes

- Catalogue des 16 volets et stores et règle du nom (`internal/core/shutter_names.go`, règle identique à celle des lumières).
- Le fichier JSON des noms (clé `light_names.path`, **aucune nouvelle clé de configuration**) porte une seconde section `shutters` à côté de `lights`. Le magasin est refactoré (`internal/core/light_names.go`) : chaque écriture préserve l'autre section et toute section inconnue ; écriture atomique et verrou unique inchangés.
- `GET /api/shutters/names` (tout utilisateur authentifié) et `PUT /api/admin/shutters/names/{id}` derrière `lan_admin`, avec audit `shutter_name` et dry-run.
- Synchronisation dans les deux sens avec le cloud dans l'agent `cloudsync` : `POST /api/gateway/shutters/names/sync` à chaque cycle (appel distinct de celui des lumières), synchro immédiate non bloquante, reprise après coupure, tolérance à un cloud qui répond 404.
- Aucune écriture dans l'armoire ; aucun changement des endpoints legacy (`serverinfos`, `mystatus`, `myactions`, `done`) ni des routes des lumières.

## Capabilities

### New Capabilities

- `shutter-names-gateway` : stockage (section `shutters`), lecture, écriture et synchronisation cloud des noms personnalisés des volets et stores.
