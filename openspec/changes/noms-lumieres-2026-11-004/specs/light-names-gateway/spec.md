## Purpose

Conserver une réplique locale des noms personnalisés des lumières, la servir sur le réseau local (CM5 et Raspberry Pi), accepter les modifications de l'administrateur local et les synchroniser avec le cloud dans les deux sens, y compris après une coupure.

## ADDED Requirements

### Requirement: Stockage dans un fichier
La gateway SHALL conserver les noms dans un fichier JSON désigné par la clé de configuration `light_names.path`, écrit de façon atomique (fichier temporaire puis renommage). Elle MUST fonctionner sans PostgreSQL.

#### Scenario: Redémarrage
- **GIVEN** un nom enregistré
- **WHEN** la gateway redémarre
- **THEN** le nom est relu depuis le fichier

#### Scenario: Stockage indisponible
- **GIVEN** `light_names.path` absent ou non inscriptible
- **WHEN** un client lit les noms
- **THEN** les 33 noms d'origine sont renvoyés ; une écriture répond 503 `names_unavailable`

### Requirement: Lecture pour tout utilisateur
`GET /api/lights/names` SHALL renvoyer les 33 lumières (`id`, `default`, `name`, `version`, `updated_at`) à tout utilisateur authentifié du réseau local, et sans authentification quand l'IAM LAN est désactivé.

#### Scenario: Utilisateur local simple
- **WHEN** un `lan_user` lit les noms
- **THEN** la réponse est 200 avec les noms

#### Scenario: Sans session avec IAM actif
- **WHEN** une requête sans session est envoyée
- **THEN** la réponse est 401

### Requirement: Écriture réservée à l'administrateur local
`PUT /api/admin/lights/names/{id}` SHALL enregistrer le nom dans le fichier, incrémenter la version, poser `updated_at` (UTC) et `updated_by`, auditer le changement et tenter une synchronisation immédiate avec le cloud sans bloquer la réponse. Seul `lan_admin` MUST pouvoir écrire (ou tout le monde quand l'IAM LAN est désactivé, comme les autres routes admin).

#### Scenario: lan_admin renomme
- **WHEN** `lan_admin` envoie `{"name": "WC 3"}` pour `dressing`
- **THEN** la réponse est 200, le fichier est mis à jour, un événement d'audit est écrit

#### Scenario: lan_user
- **WHEN** un `lan_user` envoie la même requête
- **THEN** la réponse est 403 et le fichier est inchangé

#### Scenario: Validation
- **WHEN** le nom est invalide ou l'identifiant inconnu
- **THEN** la réponse est 400 `invalid_name` ou `unknown_light`

### Requirement: Synchronisation dans les deux sens avec le cloud
À chaque cycle de synchronisation, l'agent cloud SHALL envoyer ses 33 entrées à `POST /api/gateway/lights/names/sync` et remplacer la réplique par l'état fusionné renvoyé. Sans connexion, la réplique locale MUST rester utilisable et la synchronisation reprendre au retour de la connexion, sans perdre une modification locale.

#### Scenario: Nom saisi dans le cloud
- **GIVEN** un nom modifié sur le portail cloud
- **WHEN** le cycle de synchronisation suivant s'exécute
- **THEN** la réplique locale contient le nouveau nom (moins d'une minute)

#### Scenario: Modification locale hors ligne
- **GIVEN** la gateway sans accès au cloud
- **WHEN** `lan_admin` renomme `wc1`, puis la connexion revient
- **THEN** le cloud reçoit le nom et le renvoie aux autres clients

#### Scenario: Conflit
- **GIVEN** deux modifications du même nom sur la gateway et sur le cloud
- **THEN** le plus récent `(updated_at, version, updated_by)` est conservé partout

### Requirement: Mode essai à blanc
Avec `X-Essensys-Test-Mode: dry-run` ou `?test_mode=dry_run`, l'écriture SHALL valider et répondre `"status": "dry_run"` sans toucher le fichier.

#### Scenario: Dry-run
- **WHEN** une écriture valide est envoyée en mode essai
- **THEN** le fichier est inchangé
