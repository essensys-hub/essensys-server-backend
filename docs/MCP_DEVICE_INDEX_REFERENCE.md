# Référence complète des indices MCP Essensys

Ce document liste tous les équipements disponibles via le serveur MCP Essensys avec leurs indices et valeurs correspondants.

## Outil `find_device_index`

L'outil `find_device_index` permet de rechercher un équipement par nom et retourne l'index et la valeur à utiliser.

### Utilisation

```json
{
  "name": "find_device_index",
  "arguments": {
    "device_name": "chevet chambre petit 3",
    "category": "light"  // Optionnel: "light", "shutter", "scenario", "security", "heating", "irrigation"
  }
}
```

## Catégories d'équipements

### 1. Éclairage (Light) - Indices 605-616

#### Allumer - PDV (Pièces De Vie) - Index 611
| Équipement | Valeur | Description |
|------------|--------|-------------|
| Lampe Entrée | `1` | Allumer la lampe d'entrée |
| Lampe Salon 1 | `2` | Allumer la lampe du salon (1) |
| Lampe Salon 2 | `4` | Allumer la lampe du salon (2) |
| Lampe Dressing 1 | `8` | Allumer la lampe du dressing (1) |
| Lampe Dressing 2 | `16` | Allumer la lampe du dressing (2) |

#### Allumer - PDV MSB (Variateurs) - Index 612
| Équipement | Valeur | Description |
|------------|--------|-------------|
| Variateur Bureau | `32` | Allumer le variateur du bureau |
| Variateur Salle à Manger | `64` | Allumer le variateur de la salle à manger |
| Variateur Salon | `128` | Allumer le variateur du salon |

#### Allumer - CHB (Chambres) - Index 613
| Équipement | Valeur | Description |
|------------|--------|-------------|
| Lampe Escalier | `1` | Allumer la lampe de l'escalier |
| Lampe Grande Chambre 1 | `2` | Allumer la lampe de la grande chambre (1) |
| Lampe Grande Chambre 2 | `4` | Allumer la lampe de la grande chambre (2) |
| Lampe Petite Chambre 1 (1) | `8` | Allumer la lampe de la petite chambre 1 (1) |
| Lampe Petite Chambre 1 (2) | `16` | Allumer la lampe de la petite chambre 1 (2) |
| Lampe Petite Chambre 2 | `32` | Allumer la lampe de la petite chambre 2 |
| **Lampe Petite Chambre 3** | **`64`** | **Allumer la lampe de la petite chambre 3** |
| **Chevet Petite Chambre 3** | **`64`** | **Allumer le chevet de la petite chambre 3** |

#### Allumer - CHB MSB (Variateurs) - Index 614
| Équipement | Valeur | Description |
|------------|--------|-------------|
| Variateur Petite Chambre 3 | `16` | Allumer le variateur de la petite chambre 3 |
| Variateur Petite Chambre 2 | `32` | Allumer le variateur de la petite chambre 2 |
| Variateur Petite Chambre 1 | `64` | Allumer le variateur de la petite chambre 1 |
| Variateur Grande Chambre | `128` | Allumer le variateur de la grande chambre |

#### Allumer - PDE (Pièces d'Eau) - Index 615
| Équipement | Valeur | Description |
|------------|--------|-------------|
| Lampe Cuisine 1 | `1` | Allumer la lampe de la cuisine (1) |
| Lampe Cuisine 2 | `2` | Allumer la lampe de la cuisine (2) |
| Lampe SDB 1 | `4` | Allumer la lampe de la salle de bain 1 |
| Lampe SDB 2 (1) | `8` | Allumer la lampe de la salle de bain 2 (1) |
| Lampe SDB 2 (2) | `16` | Allumer la lampe de la salle de bain 2 (2) |
| Lampe WC 1 | `32` | Allumer la lampe des WC 1 |
| Lampe WC 2 | `64` | Allumer la lampe des WC 2 |
| Lampe Service | `128` | Allumer la lampe de service |

#### Allumer - PDE MSB - Index 616
| Équipement | Valeur | Description |
|------------|--------|-------------|
| Lampe Dégagement 1 | `1` | Allumer la lampe du dégagement 1 |
| Lampe Dégagement 2 | `2` | Allumer la lampe du dégagement 2 |
| Lampe Terrasse | `4` | Allumer la lampe de la terrasse |
| Lampe Annexe 1 | `8` | Allumer la lampe de l'annexe 1 |
| Lampe Annexe 2 | `16` | Allumer la lampe de l'annexe 2 |
| Variateur SDB 1 | `128` | Allumer le variateur de la SDB 1 |

#### Éteindre - Indices 605-610
Les mêmes équipements peuvent être éteints avec les indices suivants :
- **Index 605** : Éteindre PDV LSB (mêmes valeurs que 611)
- **Index 606** : Éteindre PDV MSB (mêmes valeurs que 612)
- **Index 607** : Éteindre CHB LSB (mêmes valeurs que 613)
- **Index 608** : Éteindre CHB MSB (mêmes valeurs que 614)
- **Index 609** : Éteindre PDE LSB (mêmes valeurs que 615)
- **Index 610** : Éteindre PDE MSB (mêmes valeurs que 616)

### 2. Volets (Shutter) - Indices 617-622

#### Ouvrir - PDV - Index 617
| Équipement | Valeur | Description |
|------------|--------|-------------|
| Volet Salon 1 | `1` | Ouvrir le volet du salon 1 |
| Volet Salon 2 | `2` | Ouvrir le volet du salon 2 |
| Volet Salon 3 | `4` | Ouvrir le volet du salon 3 |
| Volet SAM 1 | `8` | Ouvrir le volet de la salle à manger 1 |
| Volet SAM 2 | `16` | Ouvrir le volet de la salle à manger 2 |
| Volet Bureau | `32` | Ouvrir le volet du bureau |

#### Ouvrir - CHB - Index 618
| Équipement | Valeur | Description |
|------------|--------|-------------|
| Volet Grande Chambre 1 | `1` | Ouvrir le volet de la grande chambre 1 |
| Volet Grande Chambre 2 | `2` | Ouvrir le volet de la grande chambre 2 |
| Volet Petite Chambre 1 | `4` | Ouvrir le volet de la petite chambre 1 |
| Volet Petite Chambre 2 | `8` | Ouvrir le volet de la petite chambre 2 |
| Volet Petite Chambre 3 | `16` | Ouvrir le volet de la petite chambre 3 |

#### Ouvrir - PDE - Index 619
| Équipement | Valeur | Description |
|------------|--------|-------------|
| Volet Cuisine 1 | `1` | Ouvrir le volet de la cuisine 1 |
| Volet Cuisine 2 | `2` | Ouvrir le volet de la cuisine 2 |
| Volet SDB 1 | `4` | Ouvrir le volet de la SDB 1 |
| Store Terrasse | `8` | Remonter le store de la terrasse |

#### Fermer - Indices 620-622
Les mêmes volets peuvent être fermés avec les indices suivants :
- **Index 620** : Fermer PDV (mêmes valeurs que 617)
- **Index 621** : Fermer CHB (mêmes valeurs que 618)
- **Index 622** : Fermer PDE (mêmes valeurs que 619)

### 3. Scénarios (Scenario) - Index 590

| Scénario | Valeur | Description |
|----------|--------|-------------|
| Scénario 1 | `1` | Réservé Internet (Zone de test) |
| Scénario 2 | `2` | Je sors (Alarme ON, Volets fermés, Sécurité) |
| Scénario 3 | `3` | Je pars en vacances (Alarme ON, Volets fermés, Chauffage Hors Gel) |
| Scénario 4 | `4` | Je rentre (Alarme OFF, Volets ouverts, Sécurité rétablie) |
| Scénario 5 | `5` | Je vais me coucher (Alarme ON, Volets fermés, Réveil armé) |
| Scénario 6 | `6` | Je me lève (Alarme OFF, Volets ouverts, Réveil désactivé) |
| Scénario 7 | `7` | Personnalisé 1 (Configurable) |
| Scénario 8 | `8` | Personnalisé 2 (Configurable) |

### 4. Sécurité (Security)

| Équipement | Index | Valeur | Description |
|------------|-------|--------|-------------|
| Mettre l'alarme | 593 | `1` | Activer l'alarme |
| Enlever l'alarme | 593 | `2` | Désactiver l'alarme |
| Couper prises sécurité | 623 | `1` | Couper les prises de sécurité |
| Remettre prises sécurité | 623 | `2` | Rétablir les prises de sécurité |
| Couper machines | 624 | `1` | Couper les machines à laver |
| Remettre machines | 624 | `2` | Rétablir les machines à laver |

### 5. Arrosage (Irrigation) - Index 363

| Mode | Valeur | Description |
|------|--------|-------------|
| OFF | `0` | Pas d'arrosage |
| Marche Forcée (minutes) | `1-254` | Durée en minutes (ex: `30` = 30 minutes) |
| Automatique | `255` | Arrosage selon planning horaire |

## Exemples d'utilisation

### Exemple 1 : Allumer le chevet de la petite chambre 3

```json
{
  "name": "send_order",
  "arguments": {
    "params_json": "[{\"k\":613,\"v\":\"64\"}]"
  }
}
```

### Exemple 2 : Ouvrir le volet de la petite chambre 3

```json
{
  "name": "send_order",
  "arguments": {
    "params_json": "[{\"k\":618,\"v\":\"16\"}]"
  }
}
```

### Exemple 3 : Déclencher le scénario "Je sors"

```json
{
  "name": "send_order",
  "arguments": {
    "params_json": "[{\"k\":590,\"v\":\"2\"}]"
  }
}
```

### Exemple 4 : Rechercher un équipement

```json
{
  "name": "find_device_index",
  "arguments": {
    "device_name": "chevet chambre petit 3",
    "category": "light"
  }
}
```

## Notes importantes

1. **Valeurs multiples** : Pour allumer/fermer plusieurs équipements simultanément, additionnez les valeurs (ex: Salon 1 + Salon 2 = `2 + 4 = 6`)

2. **Scénarios** : Les scénarios nécessitent souvent plusieurs paramètres. Le backend complète automatiquement les indices 605-622 si nécessaire.

3. **Catégories** : Utilisez le paramètre `category` dans `find_device_index` pour filtrer les résultats par type d'équipement.

4. **Recherche partielle** : La recherche accepte des termes partiels (ex: "chambre" trouvera toutes les chambres, "volet" trouvera tous les volets).

## Indices système (lecture seule)

| Index | Description |
|-------|-------------|
| 0-4 | Versions système |
| 10 | Statut global |
| 11 | Alertes |
| 12 | Informations techniques |
| 349-352 | Températures zones |
| 363 | Alerte (format binaire) |
| 920 | État Bouton Poussoir 1 |

Pour plus de détails, consultez la documentation complète dans `essensys-raspberry-install/docs/maintenance/debug.md`.
