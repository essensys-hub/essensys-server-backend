## Purpose

Régler et lire, sur le réseau local (CM5 et Raspberry Pi), le délai d'extinction automatique de six lumières, pour l'administrateur local.

## ADDED Requirements

### Requirement: Lecture des temporisations
`GET /api/admin/lights/timers` SHALL renvoyer les six lumières (`dressing` 521, `wc1` 555, `wc2` 556, `pieceserv` 557, `ann1` 561, `ann2` 562) avec leur libellé, leur clé et leur valeur en minutes. Une clé jamais lue MUST être `null`.

#### Scenario: Valeur lue
- **GIVEN** la table contient 521 = "60"
- **WHEN** un utilisateur local appelle la route
- **THEN** `dressing` vaut 60

### Requirement: Lecture périodique
Les six clés MUST faire partie de la rotation de lecture de la gateway et du snapshot de l'armoire, comme l'état des prises de sécurité.

#### Scenario: Clé jamais remontée
- **GIVEN** le firmware n'a jamais remonté 561
- **WHEN** le snapshot est lu
- **THEN** `ann1` est inconnue, pas à 0

### Requirement: Écriture réservée à l'administrateur local
`PUT /api/admin/lights/timers/{id}` avec `{"minutes": n}` SHALL mettre en file une action d'écriture sur la clé de la lumière. Avec l'IAM LAN actif, seul `lan_admin` MUST pouvoir écrire (403 sinon, 401 sans session). Avec l'IAM LAN désactivé, le comportement est celui des autres routes de la table.

#### Scenario: Administrateur local
- **WHEN** un `lan_admin` envoie `{"minutes": 15}` pour `dressing`
- **THEN** une action `521 = "15"` est en file et l'écriture est auditée avec la valeur précédente

#### Scenario: Utilisateur local simple
- **WHEN** un `lan_user` envoie la même requête
- **THEN** la réponse est 403 et la file est vide

### Requirement: Validation
L'identifiant MUST être l'un des six connus et `minutes` un entier de 0 à 255, sinon 400 sans action.

#### Scenario: Identifiant inconnu
- **WHEN** la requête vise `salon`
- **THEN** la réponse est 400

### Requirement: Remontée vers le cloud
Les six clés MUST être acceptées par le filtre de poussée vers le cloud pour que le profil de synchro du cloud les transporte.

#### Scenario: Filtre de poussée
- **WHEN** le profil du cloud couvre ces clés
- **THEN** une valeur modifiée à l'armoire est poussée au cloud
