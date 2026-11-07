## Purpose

Conserver une réplique locale des noms personnalisés des volets et stores dans le fichier déjà utilisé pour les lumières, la servir sur le réseau local, accepter les modifications de l'administrateur local et les synchroniser avec le cloud dans les deux sens, y compris après une coupure.

## ADDED Requirements

### Requirement: Stockage dans le fichier des noms de lumières
La gateway SHALL conserver les noms des volets dans une section `"shutters":{id:{name,version,updated_at,updated_by}}` du **même fichier JSON** que les lumières, désigné par la **même clé** `light_names.path`, à côté de la section `"lights"`. Le champ `version` du fichier reste 1 ; une section absente est vide. L'écriture d'une section MUST préserver l'autre section et toute section inconnue ; l'écriture reste atomique (fichier temporaire puis renommage) sous un seul verrou. La gateway MUST fonctionner sans PostgreSQL. Si l'implémentation introduit un autre chemin, une autre clé ou un autre fichier, les briques `raspberry-pi` et `ansible` redeviennent concernées et la matrice doit être revue par un humain.

#### Scenario: Redémarrage
- **GIVEN** un nom de volet enregistré
- **WHEN** la gateway redémarre
- **THEN** le nom est relu depuis la section `shutters`

#### Scenario: Les deux sections coexistent
- **GIVEN** un fichier contenant `lights` (dressing = « WC 3 ») et `shutters`
- **WHEN** un volet est renommé, puis une lumière
- **THEN** chaque écriture laisse l'autre section intacte, ainsi qu'une section inconnue ajoutée à la main

#### Scenario: Fichier de #41 sans section volets
- **GIVEN** un fichier produit par #41 (section `lights` seulement)
- **WHEN** la gateway lit les noms de volets puis en écrit un
- **THEN** la lecture renvoie les 16 noms d'origine, l'écriture ajoute la section sans toucher `lights`

#### Scenario: Stockage indisponible
- **GIVEN** `light_names.path` absent ou non inscriptible
- **WHEN** un client lit les noms de volets
- **THEN** les 16 noms d'origine sont renvoyés ; une écriture répond 503 `names_unavailable`

### Requirement: Lecture pour tout utilisateur
`GET /api/shutters/names` SHALL renvoyer `{"shutters":[…]}` (16 éléments : `id`, `default`, `name`, `version`, `updated_at`) à tout utilisateur authentifié du réseau local, et sans authentification quand l'IAM LAN est désactivé.

#### Scenario: Utilisateur local simple
- **WHEN** un `lan_user` lit les noms
- **THEN** la réponse est 200 avec les noms

#### Scenario: Sans session avec IAM actif
- **WHEN** une requête sans session est envoyée
- **THEN** la réponse est 401

### Requirement: Écriture réservée à l'administrateur local
`PUT /api/admin/shutters/names/{id}` SHALL enregistrer le nom dans le fichier, incrémenter la version, poser `updated_at` (UTC) et `updated_by`, auditer le changement (`shutter_name`) et tenter une synchronisation immédiate avec le cloud sans bloquer la réponse. Seul `lan_admin` MUST pouvoir écrire (ou tout le monde quand l'IAM LAN est désactivé, comme les autres routes admin).

#### Scenario: lan_admin renomme
- **WHEN** `lan_admin` envoie `{"name": "Volet baie vitrée"}` pour `volet1salon`
- **THEN** la réponse est 200, la section `shutters` du fichier est mise à jour, un événement d'audit est écrit

#### Scenario: lan_user
- **WHEN** un `lan_user` envoie la même requête
- **THEN** la réponse est 403 et le fichier est inchangé

#### Scenario: Validation
- **WHEN** le nom est invalide ou l'identifiant inconnu (y compris un identifiant de lumière)
- **THEN** la réponse est 400 `invalid_name` ou `unknown_shutter`

### Requirement: Synchronisation dans les deux sens avec le cloud
À chaque cycle de synchronisation, l'agent cloud SHALL envoyer ses 16 entrées à `POST /api/gateway/shutters/names/sync` et remplacer sa section `shutters` par l'état fusionné renvoyé. Sans connexion, la réplique locale MUST rester utilisable et la synchronisation reprendre au retour de la connexion, sans perdre une modification locale. La synchronisation des volets et celle des lumières sont indépendantes : l'échec de l'une ne bloque pas l'autre.

#### Scenario: Nom saisi dans le cloud
- **GIVEN** un nom de volet modifié sur le portail cloud
- **WHEN** le cycle de synchronisation suivant s'exécute
- **THEN** la réplique locale contient le nouveau nom (moins d'une minute)

#### Scenario: Modification locale hors ligne
- **GIVEN** la gateway sans accès au cloud
- **WHEN** `lan_admin` renomme `store`, puis la connexion revient
- **THEN** le cloud reçoit le nom et le renvoie aux autres clients

#### Scenario: Conflit
- **GIVEN** deux modifications du même nom sur la gateway et sur le cloud
- **THEN** le plus récent `(updated_at, version, updated_by)` est conservé partout

#### Scenario: Route volets indisponible côté cloud
- **GIVEN** un cloud qui répond 404 sur la route des volets (cloud pas encore déployé)
- **THEN** les lumières continuent de se synchroniser et la réplique des volets reste utilisable

### Requirement: Mode essai à blanc
Avec `X-Essensys-Test-Mode: dry-run` ou `?test_mode=dry_run`, l'écriture SHALL valider et répondre `"status": "dry_run"` sans toucher le fichier.

#### Scenario: Dry-run
- **WHEN** une écriture valide est envoyée en mode essai
- **THEN** le fichier est inchangé

### Requirement: Noms des lumières inchangés
Les routes `/api/lights/names`, `/api/admin/lights/names/{id}` et la section `lights` MUST se comporter exactement comme avant cette Feature (non-régression des tests de #41).

#### Scenario: Écriture d'une lumière après un volet
- **WHEN** un volet est renommé puis une lumière
- **THEN** les deux noms sont lisibles et synchronisés
