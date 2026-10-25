## Purpose

Fait lire, conserver et pousser par la gateway l'état des prises de sécurité (indice 440), et l'expose dans le snapshot de l'armoire.

## ADDED Requirements

### Requirement: Lecture par la gateway
La gateway SHALL demander l'indice 440 au BP dans sa rotation `serverinfos` (groupe d'indices de santé) et SHALL le conserver dans sa table, de sorte que `GET /api/admin/exchange?keys=440` le renvoie après au plus un tour de rotation.

#### Scenario: Après un tour de rotation
- **WHEN** le BP a répondu à la demande contenant 440
- **THEN** `GET /api/admin/exchange?keys=440` renvoie la valeur reçue

### Requirement: Snapshot de l'armoire
`GET /api/admin/armoire/snapshot` SHALL inclure l'état des prises de sécurité dans le bloc alarme (`security_outlets`: `cut`, `piloted` ou `unknown`).

#### Scenario: Prises coupées
- **WHEN** l'indice 440 vaut 1
- **THEN** le snapshot contient `security_outlets: "cut"`

### Requirement: Poussée vers le cloud
Un profil de synchronisation par défaut « Prises de sécurité » SHALL couvrir la plage [440, 440] avec pull et push vers le cloud, créé par migration, sans modifier les profils existants.

#### Scenario: Nouvelle installation
- **WHEN** les migrations sont appliquées sur une gateway
- **THEN** le profil « Prises de sécurité » existe et 440 fait partie des clés poussées au cloud

### Requirement: Commande MCP
Les commandes MCP « couper / remettre les prises de sécurité » SHALL passer par le même complément que l'API (`590 = "1"` ajouté), au lieu d'envoyer 623 seul.

#### Scenario: Commande MCP
- **WHEN** l'outil MCP « couper les prises de sécurité » est appelé
- **THEN** l'action mise en file contient `590 = "1"`, 605–622 à `"0"` et `623 = "1"`
