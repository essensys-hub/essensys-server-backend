> Ticket : [essensys-hub/essensys-feature-lifecycle#55](https://github.com/essensys-hub/essensys-feature-lifecycle/issues/55). Feature ID : `affichage-objets-2026-11-006`. Sub-issue : essensys-hub/essensys-server-backend#23.
> Feature répartie sur plusieurs dépôts. Le change commun (proposal, design, contrat d'API, tâches de tous les dépôts) est dans `essensys-feature-lifecycle/openspec/changes/affichage-objets-2026-11-006`. Ce change ne décrit que le code de **essensys-server-backend** (gateway CM5 et Raspberry Pi).

## Why

Permettre à l'administrateur d'activer ou de désactiver chacune des 33 lumières et des 16 volets et stores, et de choisir leur ordre d'affichage, sur le réseau local comme dans le cloud. Jumeau de `noms-lumieres-2026-11-004` et `noms-volets-2026-11-005`, avec un document par genre au lieu d'une entrée par objet.

## What Changes

- Modèle, validation et ordre effectif de la disposition (`internal/core/object_layout.go`) : `{"order":[…],"disabled":[…],"version":n,"updated_at":"…"}`, identifiants du catalogue du genre, sans doublon, `reset`.
- Le fichier JSON des noms (clé `light_names.path`, **aucune nouvelle clé de configuration**) porte deux sections de plus, `lights_layout` et `shutters_layout`. Ce sont des **objets** : décodeur dédié et écriture `setRaw`, jamais `namesDoc.section()`. Même verrou, même écriture atomique ; chaque écriture préserve toutes les autres sections (noms, volets, autre disposition, sections inconnues).
- `GET /api/lights/layout` et `GET /api/shutters/layout` (tout utilisateur authentifié) ; `PUT /api/admin/lights/layout` et `PUT /api/admin/shutters/layout` derrière `lan_admin`, avec `reset`, audit `object_layout` et dry-run.
- Synchronisation dans les deux sens avec le cloud dans l'agent `cloudsync` : `POST /api/gateway/{lights|shutters}/layout/sync` à chaque cycle (un appel par genre, distincts de ceux des noms), fusion du document entier, synchro immédiate non bloquante, reprise après coupure, tolérance à un cloud qui répond 404.
- Aucune écriture dans l'armoire ; aucun changement des endpoints legacy (`serverinfos`, `mystatus`, `myactions`, `done`) ni des routes des noms.

## Capabilities

### New Capabilities

- `object-layout-gateway` : stockage (sections `lights_layout` et `shutters_layout`), lecture, écriture et synchronisation cloud de la disposition des lumières et des volets.
