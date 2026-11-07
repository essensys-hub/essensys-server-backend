## Purpose

Conserver une réplique locale de la disposition des lumières et des volets dans le fichier déjà utilisé pour les noms, la servir sur le réseau local, accepter les modifications de l'administrateur local et les synchroniser avec le cloud dans les deux sens, y compris après une coupure.

## ADDED Requirements

### Requirement: Stockage dans le fichier des noms
La gateway SHALL conserver les dispositions dans deux sections, `"lights_layout"` et `"shutters_layout"`, du **même fichier JSON** que les noms, désigné par la **même clé** `light_names.path`. Chaque section est un objet `{"order":[…],"disabled":[…],"version":n,"updated_at":"…","updated_by":"…"}` ; une section absente est la disposition par défaut. Le champ `version` du fichier reste 1. L'écriture d'une section MUST préserver toutes les autres sections (`lights`, `shutters`, l'autre disposition, toute section inconnue) ; elle reste atomique (fichier temporaire puis renommage) sous le même verrou. Les sections de disposition MUST NOT être décodées comme des tables de noms. La gateway MUST fonctionner sans PostgreSQL. Si l'implémentation introduit un autre chemin, une autre clé ou un autre fichier, les briques `raspberry-pi` et `ansible` redeviennent concernées et la matrice doit être revue par un humain.

#### Scenario: Redémarrage
- **GIVEN** une disposition enregistrée
- **WHEN** la gateway redémarre
- **THEN** elle est relue depuis la section correspondante

#### Scenario: Toutes les sections coexistent
- **GIVEN** un fichier contenant `lights`, `shutters`, `lights_layout`, `shutters_layout` et une section inconnue ajoutée à la main
- **WHEN** un nom de lumière, un nom de volet, puis chaque disposition sont écrits
- **THEN** chaque écriture laisse toutes les autres sections intactes

#### Scenario: Fichier de #52 sans section de disposition
- **GIVEN** un fichier produit par #52
- **WHEN** la gateway lit les dispositions puis en écrit une
- **THEN** la lecture renvoie la disposition par défaut, l'écriture ajoute la section sans toucher les noms

#### Scenario: Stockage indisponible
- **GIVEN** `light_names.path` absent ou non inscriptible
- **WHEN** un client lit une disposition
- **THEN** la disposition par défaut est renvoyée ; une écriture répond 503 `names_unavailable`

#### Scenario: Aucune nouvelle clé de configuration
- **THEN** `config.yaml.example` ne reçoit aucune nouvelle clé ; sinon l'implémentation s'arrête et la matrice des briques est revue

### Requirement: Lecture pour tout utilisateur
`GET /api/lights/layout` et `GET /api/shutters/layout` SHALL renvoyer le document (sans `updated_by`) à tout utilisateur authentifié du réseau local, et sans authentification quand l'IAM LAN est désactivé.

#### Scenario: Utilisateur local simple
- **WHEN** un `lan_user` lit la disposition
- **THEN** la réponse est 200

#### Scenario: Sans session avec IAM actif
- **WHEN** une requête sans session est envoyée
- **THEN** la réponse est 401

### Requirement: Écriture réservée à l'administrateur local
`PUT /api/admin/lights/layout` et `PUT /api/admin/shutters/layout` SHALL valider (mêmes règles et mêmes erreurs que le cloud : `unknown_light` / `unknown_shutter`, `invalid_layout`), enregistrer le document entier à la version `n+1`, poser `updated_at` (UTC) et `updated_by`, auditer (`object_layout`) et tenter une synchronisation immédiate avec le cloud sans bloquer la réponse. `{"reset":true}` revient à la disposition par défaut à la version `n+1`. Seul `lan_admin` MUST pouvoir écrire (ou tout le monde quand l'IAM LAN est désactivé, comme les autres routes admin).

#### Scenario: lan_admin désactive une lumière
- **WHEN** `lan_admin` envoie `{"order":[],"disabled":["wc2"]}`
- **THEN** la réponse est 200, la section `lights_layout` est mise à jour, un événement d'audit est écrit

#### Scenario: lan_user
- **WHEN** un `lan_user` envoie la même requête
- **THEN** la réponse est 403 et le fichier est inchangé

#### Scenario: Validation
- **WHEN** l'identifiant est inconnu, en double, ou le corps illisible
- **THEN** la réponse est 400 `unknown_light`, `unknown_shutter` ou `invalid_layout`

### Requirement: Synchronisation dans les deux sens avec le cloud
À chaque cycle de synchronisation, l'agent cloud SHALL envoyer chaque document à `POST /api/gateway/{lights|shutters}/layout/sync` et remplacer sa section par l'état fusionné renvoyé. Sans connexion, la réplique locale MUST rester utilisable et la synchronisation reprendre au retour de la connexion, sans perdre une modification locale. Les synchronisations des noms de lumières, des noms de volets et des deux dispositions sont indépendantes : l'échec de l'une ne bloque pas les autres.

#### Scenario: Disposition saisie dans le cloud
- **GIVEN** une lumière désactivée sur le portail cloud
- **WHEN** le cycle de synchronisation suivant s'exécute
- **THEN** la réplique locale contient la nouvelle disposition (moins d'une minute)

#### Scenario: Modification locale hors ligne
- **GIVEN** la gateway sans accès au cloud
- **WHEN** `lan_admin` désactive `store`, puis la connexion revient
- **THEN** le cloud reçoit le document et le renvoie aux autres clients

#### Scenario: Conflit
- **GIVEN** deux modifications de la même disposition sur la gateway et sur le cloud
- **THEN** le document le plus récent `(updated_at, version, updated_by)` est conservé partout, entier

#### Scenario: Route absente côté cloud
- **GIVEN** un cloud qui répond 404 sur les routes de disposition
- **THEN** les noms continuent de se synchroniser et la réplique des dispositions reste utilisable

### Requirement: Mode essai à blanc
Avec `X-Essensys-Test-Mode: dry-run` ou `?test_mode=dry_run`, l'écriture SHALL valider et répondre `"status":"dry_run"` sans toucher le fichier.

#### Scenario: Dry-run
- **WHEN** une requête valide est envoyée avec le mode essai
- **THEN** la réponse est `dry_run` et le fichier est inchangé
