## Context

Motivation : voir `proposal.md`. État des lieux (2026-10-26) : aucune interface ne pilote les prises ; les backends ne complètent le bloc du scénario 1 que pour 605–622 (une commande 623 seule n'est pas déclenchée) ; l'indice 440 n'est lu ni par la gateway ni par le cloud ; le cloud regroupe les actions d'éclairage consécutives (portal-action-merge-2026-10-007).

**Décisions communes aux 6 dépôts** (le détail propre à chaque dépôt suit) :
- **C1 — Commande** : les clients envoient un lot `[590 = "1", 623 = "<1|2>"]`. Les backends le complètent (605–622 à `0`), comme pour l'éclairage. Envoyer 590 côté client garde la commande valable même avant le déploiement des backends.
- **C2 — Pas de recoupure** : les commandes d'éclairage et de volets portent désormais `623 = "0"` (« ne rien faire »). Une valeur restée dans le tableau du BP ne peut donc pas être rejouée par la commande d'éclairage suivante.
- **C3 — État** : l'indice 440 est lu par la gateway (rotation `serverinfos` et snapshot), poussé au cloud par un profil de synchronisation, et affiché par les clients (« Coupées », « Pilotées » ou « État inconnu »).
- **C4 — Sécurité d'usage** : une confirmation est demandée avant de couper. La commande est désactivée quand l'armoire est hors ligne et respecte le mode test (dry-run).
- **C5 — Validation réelle** : sur le banc BP, par un humain, jamais par les tests automatiques. La feature n'est annoncée qu'après cette validation.

## Goals / Non-Goals

**Goals :** voir les specs de ce change.

**Non-Goals :** la coupure des machines à laver (indice 624, même mécanisme, à traiter séparément) ; les endpoints legacy du protocole IoT ; l'installation sans gateway pour l'état.

## Decisions

### D1 — Complément du bloc (`action_service.go`)
Même règle que le cloud dans `GenerateCompleteBlock` (déclencheur 605–623, complément 590, 605–622, 623). Tests `NR-server-backend-22` (coupure seule) et `NR-server-backend-23` (éclairage → 623 = 0).

### D2 — Lecture et snapshot de 440
440 ajouté au groupe `HealthIndices` de `pull_groups.go` (rotation `serverinfos`) et à `SnapshotIndices` ; `decode.go` gagne `securityOutletsLabel` et `snapshot.go` un champ `SecurityOutlets` dans `AlarmBlock`. Test `NR-server-backend-24` sur le décodage (1 → cut, 0 → piloted, autre → unknown).

### D3 — Poussée cloud
Migration `016_security_outlets_sync_profile.sql` dans essensys-user-portal-backend, où vivent les profils (`sync_profiles`, migrations 007 et 009) : profil « Prises de sécurité » [440, 440], pull + push, activé, idempotente (`WHERE NOT EXISTS`, les migrations étant rejouées à chaque démarrage). La gateway récupère les profils par `sync-config` et pousse 440 avec les autres clés (`pushIndicesFromProfilesList`).

### D4 — MCP
`cmd/mcp-server/main.go` réutilise le complément de `core` au lieu de sa copie locale, pour que les commandes 623 (et 624) soient déclenchées.

### D5 — Outillage
Le dépôt reçoit `openspec/`, `features/schema/feature.schema.json` et `scripts/feature_lifecycle/` (copiés de `essensys-user-portal-backend`), pour que manifest et gate fonctionnent.

## Risks / Trade-offs

- [Le BP garde 623 après exécution du scénario 1] → C2 : 623 = 0 sur chaque commande d'éclairage ou de volets.
- [Le BP garde les masques 605–622] → la commande des prises est complétée à 0 sur 605–622 par les backends.
- [Indice 440 non transmis (installation sans gateway, ancienne gateway)] → « État inconnu », commandes toujours utilisables.
- [Couper les prises par erreur] → confirmation obligatoire, rétablissement en un appui.
- [Comportement réel non vérifiable en test automatique] → validation sur le banc BP (C5).

## Migration Plan

1. Backends d'abord (cloud et gateway), car ils assurent C2 (pas de recoupure) et C3 (état). Les clients restent fonctionnels avant ce déploiement grâce à C1.
2. Puis portails et apps, une PR par dépôt avec `Feature: security-outlets-2026-10-011` et `Refs`/`Closes` vers les sub-issues.
3. Validation sur le banc BP par un humain, puis annonce aux utilisateurs.
4. Rollback : les commandes envoyées par les anciens clients ne changent pas ; retirer la carte suffit côté client.
