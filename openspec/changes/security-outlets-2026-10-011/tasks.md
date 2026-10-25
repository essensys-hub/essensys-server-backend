## 1. Outillage

- [ ] 1.1 Ajouter `openspec/`, `features/schema/feature.schema.json` et `scripts/feature_lifecycle/` ; vérifier `check_feature_gate.py --strict` sur le manifest

## 2. Commande

- [ ] 2.1 Élargir `GenerateCompleteBlock` à 623 ; vérifier par `NR-server-backend-22` et `NR-server-backend-23`
- [ ] 2.2 Faire passer les commandes MCP 623 et 624 par le même complément ; vérifier par un test du serveur MCP

## 3. État des prises

- [ ] 3.1 Ajouter 440 à la rotation `serverinfos` et au snapshot (`security_outlets`) ; vérifier par `NR-server-backend-24` (décodage) et un test du handler snapshot
- [ ] 3.2 Migration du profil de synchronisation « Prises de sécurité » [440, 440] ; vérifier qu'elle est idempotente et que 440 est poussé

## 4. Livraison

- [ ] 4.1 `/checkup` vert, déploiement sur la gateway (essensys-ansible) avant le portail local, puis validation sur le banc BP
