> Ticket : [essensys-hub/essensys-feature-lifecycle#41](https://github.com/essensys-hub/essensys-feature-lifecycle/issues/41). Feature ID : `noms-lumieres-2026-11-004`. Sub-issue : essensys-hub/essensys-server-backend#19.
> Feature répartie sur plusieurs dépôts. Le change commun (proposal, design, contrat d'API, tâches de tous les dépôts) est dans `essensys-feature-lifecycle/openspec/changes/noms-lumieres-2026-11-004`. Ce change ne décrit que le code de **essensys-server-backend** (gateway CM5 et Raspberry Pi).

## Why

Permettre à l'administrateur de renommer chacune des 33 lumières (« Dressing » devient « WC 3 »), sur le réseau local comme dans le cloud, avec un retour possible au nom d'origine.

## What Changes

- Catalogue des 33 lumières et règle du nom (`internal/core/light_names.go`).
- Magasin de fichier JSON atomique, clé de configuration `light_names.path` (`config.yaml`, variable `LIGHT_NAMES_PATH`).
- `GET /api/lights/names` (tout utilisateur authentifié) et `PUT /api/admin/lights/names/{id}` derrière `lan_admin`, avec audit et dry-run.
- Synchronisation dans les deux sens avec le cloud dans l'agent `cloudsync` : `POST /api/gateway/lights/names/sync` à chaque cycle, synchro immédiate non bloquante après une écriture locale, reprise après coupure.
- Aucune écriture dans l'armoire ; aucun changement des endpoints legacy (`serverinfos`, `mystatus`, `myactions`, `done`).

## Capabilities

### New Capabilities

- `light-names-gateway` : stockage, lecture, écriture et synchronisation cloud des noms personnalisés des lumières.
