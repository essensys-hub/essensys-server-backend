---
inclusion: always
---

# Protocole Legacy Essensys - Contraintes Critiques

## Client Non-Standard HTTP

Le client Essensys est un système embarqué ancien qui **ne respecte pas les normes HTTP**:

### 1. JSON Malformé
Le client envoie du JSON avec **clés non-quotées**:
```json
// Client envoie:
{version:"1.0",ek:[{k:613,v:"1"},{k:607,v:"0"}]}

// Au lieu de:
{"version":"1.0","ek":[{"k":613,"v":"1"},{"k":607,"v":"0"}]}
```

**Solution**: Normaliser en ajoutant les quotes manquantes avant parsing:
```go
bodyStr = strings.ReplaceAll(bodyStr, "{k:", "{\"k\":")
bodyStr = strings.ReplaceAll(bodyStr, ",v:", ",\"v\":")
```

### 2. Headers HTTP Incomplets
Le client peut:
- Omettre certains headers standards
- Envoyer des headers mal formatés
- Ne pas respecter la casse des headers

**Solution**: Parser les headers de manière permissive, ignorer les erreurs non-critiques.

### 3. Content-Type Incohérent
Le serveur doit **toujours** répondre avec:
```
Content-Type: application/json ;charset=UTF-8
```
Note l'espace avant `;charset` - c'est requis par le client legacy.

## Table d'Échange Complète (605-622)

### Comportement Critique
Quand on veut allumer **une seule lumière**, le serveur doit envoyer **toute la table d'échange** (indices 605-622) avec:
- L'index cible à "1" (ON) ou valeur souhaitée
- **Tous les autres indices à "0"** (OFF/reset)

### Exemple: Allumer Lumière Escalier (index 613)
```json
{
  "_de67f": null,
  "actions": [{
    "guid": "abc-123",
    "params": [
      {"k": 605, "v": "0"},
      {"k": 606, "v": "0"},
      {"k": 607, "v": "0"},
      // ... tous les autres à 0 ...
      {"k": 613, "v": "1"},  // ← Seul index actif
      {"k": 614, "v": "0"},
      // ... jusqu'à 622 ...
      {"k": 622, "v": "0"},
      {"k": 590, "v": "1"}   // Trigger scenario
    ]
  }]
}
```

### Pourquoi?
Le client interprète l'absence d'un index comme "garder l'état actuel". Pour garantir qu'une seule lumière s'allume, il faut **explicitement éteindre toutes les autres**.

### Logique de Merge (Bitwise OR)
Quand plusieurs commandes arrivent via `/api/admin/inject`:
1. Initialiser indices 605-622 à 0
2. Pour chaque paramètre reçu: `mergedValues[k] = currentValue | newValue`
3. Envoyer la table complète au client

```go
// Initialisation
for i := 605; i <= 622; i++ {
    mergedValues[i] = 0
}

// Merge avec OR
for _, p := range params {
    valInt := parseIntFromString(p.V)
    mergedValues[p.K] = mergedValues[p.K] | valInt
}
```

## Index Spéciaux

| Index | Description | Valeur |
|-------|-------------|--------|
| 590 | Trigger Scenario | Toujours "1" |
| 613 | Lumière Escalier ON | "1" = allumer |
| 607 | Lumière Escalier OFF | "1" = éteindre |
| 615 | Lumière SDB2 ON | "1" = allumer |

**Note**: L'index 590 doit **toujours** être inclus avec valeur "1" dans chaque action.

## Champ `_de67f`

Le champ `_de67f` doit être **le premier** dans la réponse JSON de `/api/myactions`:
```json
{
  "_de67f": null,  // ← DOIT être en premier
  "actions": [...]
}
```

Le parser du client legacy lit séquentiellement et s'attend à ce champ en premier.

## Endpoints et Comportements

### GET `/api/serverinfos`
- Retourne la liste des indices à surveiller
- Pas de body, juste les headers HTTP standards

### POST `/api/mystatus`
- Reçoit les valeurs actuelles du client
- JSON malformé à normaliser
- Répondre `201 Created` (pas `200 OK`)

### GET `/api/myactions`
- Retourne les actions en attente
- Inclure la table complète 605-622
- Champ `_de67f` en premier

### POST `/api/done/{guid}`
- Acknowledge d'une action exécutée
- Répondre `201 Created`

## Timeouts et Polling

Le client poll toutes les **1 seconde**:
- `/api/mystatus` - envoie son état
- `/api/myactions` - récupère les commandes

Le serveur doit répondre rapidement (< 500ms) pour éviter les timeouts côté client.

## Historique des Valeurs

Garder les **25 dernières valeurs** de chaque index pour:
- Debugging
- Monitoring via interface web
- Détection de patterns

```go
if len(history) > 25 {
    history = history[len(history)-25:]
}
```

## Résumé des Contraintes

1. ✅ Normaliser le JSON malformé du client
2. ✅ Toujours envoyer la table complète 605-622
3. ✅ Initialiser les indices non-mentionnés à "0"
4. ✅ Inclure index 590 = "1" dans chaque action
5. ✅ Champ `_de67f` en premier dans `/api/myactions`
6. ✅ Content-Type avec espace: `application/json ;charset=UTF-8`
7. ✅ Répondre `201 Created` pour POST (pas `200 OK`)
8. ✅ Parser HTTP permissif pour headers non-standard
