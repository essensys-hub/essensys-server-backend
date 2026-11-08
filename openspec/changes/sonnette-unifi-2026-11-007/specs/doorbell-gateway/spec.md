## Purpose

Détecter la pression de la sonnette UniFi, faire clignoter les lumières choisies par l'administrateur puis les restaurer, servir l'état et la vidéo sur le réseau local, et conserver les réglages dans le fichier existant.

## ADDED Requirements

### Requirement: Détection de la sonnette
La gateway SHALL écouter les événements UniFi Protect de la caméra de sonnette choisie et considérer une pression comme un événement de sonnerie. UniFi désactivé ou clé d'API absente : aucune écoute, aucune erreur au démarrage.

#### Scenario: Pression de la sonnette
- **GIVEN** la fonction activée et la console joignable (faux serveur UniFi)
- **WHEN** la console émet un événement de sonnerie de la caméra choisie
- **THEN** `GET /api/doorbell/status` renvoie `ringing=true` et `last_ring_at`

#### Scenario: Autre caméra
- **WHEN** un événement concerne une autre caméra
- **THEN** rien ne se produit

#### Scenario: UniFi désactivé
- **GIVEN** `unifi.enabled=false`
- **WHEN** la gateway démarre
- **THEN** elle fonctionne normalement et `available=false`

### Requirement: Clignotement et restauration
À chaque sonnerie acceptée, la gateway SHALL faire clignoter les seules lumières choisies `blinks` fois puis les remettre dans l'état qu'elles avaient avant la séquence. Elle MUST NOT toucher aux autres lumières ni aux volets.

#### Scenario: Séquence complète
- **GIVEN** deux lumières choisies, l'une allumée et l'autre éteinte
- **WHEN** la sonnette est pressée
- **THEN** seules ces deux lumières reçoivent des ordres, alternés `blinks` fois, puis retrouvent leur état initial

#### Scenario: Rafale
- **WHEN** la sonnette est pressée trois fois en 10 s
- **THEN** une seule séquence est jouée

#### Scenario: Commande manuelle pendant la séquence
- **WHEN** un utilisateur commande une lumière pendant qu'elle clignote
- **THEN** elle est exclue de la séquence et n'est pas restaurée

#### Scenario: Aucune lumière choisie
- **WHEN** la sonnette est pressée sans lumière choisie
- **THEN** la vidéo est signalée, aucun ordre n'est écrit

### Requirement: Réglages de l'administrateur
La gateway SHALL exposer `GET /api/admin/doorbell/settings` et `PUT /api/admin/doorbell/settings` (`{"enabled","camera_id","lights","blinks"}`) derrière `adminOnlyJSON`. Les réglages sont stockés dans la section `doorbell` du fichier désigné par `light_names.path`, sans nouvelle clé de chemin, en préservant toutes les autres sections. Valeurs par défaut : désactivé, aucune lumière, 5 clignotements. `lights` MUST contenir des identifiants du catalogue ; `blinks` MUST être compris entre 1 et 10 ; `camera_id` MUST appartenir aux caméras de la console.

#### Scenario: Enregistrement
- **WHEN** l'administrateur choisit deux lumières et 3 clignotements
- **THEN** la réponse contient les réglages et ils sont relus après redémarrage

#### Scenario: Valeurs refusées
- **WHEN** une lumière inconnue, `blinks=0` ou une caméra inconnue est envoyée
- **THEN** réponse 400 `unknown_light`, `invalid_blinks` ou `unknown_camera`

#### Scenario: Droits
- **WHEN** un `lan_user` appelle `PUT`
- **THEN** réponse 403

#### Scenario: Autres sections préservées
- **GIVEN** un fichier contenant `lights`, `shutters`, `lights_layout`, `shutters_layout`
- **WHEN** les réglages de la sonnette sont écrits
- **THEN** toutes les autres sections restent intactes

#### Scenario: Stockage indisponible
- **WHEN** `light_names.path` est absent ou non inscriptible
- **THEN** la lecture renvoie les valeurs par défaut et l'écriture répond 503 `names_unavailable`

### Requirement: État et vidéo
`GET /api/doorbell/status` et `GET /api/doorbell/stream` SHALL être accessibles à tout utilisateur authentifié (ouverts si l'IAM LAN est désactivé). Le flux MUST être servi par la gateway : la clé d'API et l'adresse de la console ne sont jamais renvoyées.

#### Scenario: Utilisateur authentifié
- **WHEN** un `lan_user` appelle l'état ou le flux
- **THEN** réponse 200

#### Scenario: Sans session
- **WHEN** aucune session n'est présente (IAM actif)
- **THEN** réponse 401

#### Scenario: Console injoignable
- **WHEN** la console ne répond pas
- **THEN** `available=false`, le flux répond 503 `doorbell_unavailable`, le reste de la gateway fonctionne, et la reconnexion est automatique

### Requirement: Essai administrateur
`POST /api/admin/doorbell/test` SHALL jouer la séquence comme une vraie pression. Avec l'en-tête `X-Essensys-Test-Mode: dry-run`, aucune écriture n'est faite dans la table d'échange.

#### Scenario: Essai à sec
- **WHEN** l'administrateur appelle l'essai avec l'en-tête `dry-run`
- **THEN** l'état passe à `ringing=true` et aucun ordre n'est écrit dans l'armoire

#### Scenario: Aucune écriture legacy
- **THEN** aucun endpoint `serverinfos`, `mystatus`, `myactions` ni `done` n'est modifié

### Requirement: Aucune donnée sensible exposée
Les journaux, réponses et tests MUST NOT contenir l'adresse de la console, la clé d'API ni un identifiant de compte.

#### Scenario: Journaux
- **WHEN** une erreur de connexion à la console survient
- **THEN** le journal n'indique ni adresse ni clé
