# Debug Auth Listener - Outil de Debug pour l'Authentification IoT

## Description

Ces outils permettent d'extraire le token d'authentification envoyé par le client IoT legacy pour faciliter le débogage et la configuration de la base de données.

## Outils Disponibles

### 1. `debug_auth_listener` (Go) - Port 8080

Écoute sur un port alternatif (8080) et affiche toutes les informations de la requête.

**Avantages** :
- Facile à utiliser
- Affiche tous les détails de la requête
- Génère automatiquement la requête SQL

**Inconvénients** :
- Nécessite de modifier l'adresse du client IoT (ou utiliser un proxy)

### 2. `capture_auth_token.sh` (tcpdump) - Port 80

Capture le trafic réseau sur le port 80 en temps réel.

**Avantages** :
- Fonctionne avec le client IoT sans modification
- Capture le trafic réel même si le serveur principal tourne
- Pas besoin de modifier la configuration du client

**Inconvénients** :
- Nécessite les permissions root (sudo)
- Nécessite tcpdump installé

## Utilisation

### Option 1 : Debug Listener (Port 8080)

```bash
cd /Users/nrineau/ESSENSYS/essensys-server-backend
./tools/debug_auth_listener
```

L'outil écoute sur le port **8080**. Vous devrez :
- Soit modifier temporairement l'adresse IP du serveur dans le client IoT
- Soit utiliser un proxy/redirect pour rediriger les requêtes du port 80 vers 8080

### Option 2 : Capture Réseau (Port 80) - RECOMMANDÉ

```bash
cd /Users/nrineau/ESSENSYS/essensys-server-backend
sudo ./tools/capture_auth_token.sh
```

Cet outil capture le trafic sur le port 80 en temps réel, même si le serveur principal tourne. Il affichera automatiquement le token lorsqu'une requête est détectée.

### 3. Observer les logs

L'outil affichera :
- Tous les headers HTTP de la requête
- Le header `Authorization: Basic <base64>`
- Le token décodé (username:password)
- Le `hashed_pkey` calculé (username + password)
- Une requête SQL pour trouver la machine correspondante

### Exemple de sortie

```
===========================================
REQUEST: GET /api/myactions
From: 192.168.0.151:xxxxx
===========================================

--- Headers ---
Authorization: Basic <base64(username:password)>
...

--- AUTHENTICATION TOKEN ---
Authorization Header: Basic <base64(username:password)>
Base64 Encoded: <base64(username:password)>
Decoded (username:password): testuser:testpass

--- TOKEN BREAKDOWN ---
Username (first 16 hex): testuser
Password (last 16 hex): testpass
Hashed Pkey (concatenated): testusertestpass
Hashed Pkey Length: 20 characters
⚠️  Format unexpected: username=8 chars, password=8 chars

--- SQL QUERY TO FIND MACHINE ---
SELECT * FROM es_machine WHERE hashed_pkey = 'testusertestpass' AND is_active = true;
```

## Utilisation avec tcpdump (Alternative)

Si vous ne pouvez pas modifier l'adresse du client, vous pouvez capturer le trafic réseau :

```bash
# Capturer le trafic sur le port 80
sudo tcpdump -i any -A -s 0 'tcp port 80 and (((ip[2:2] - ((ip[0]&0xf)<<2)) - ((tcp[12]&0xf0)>>2)) != 0)' | grep -A 20 "Authorization"
```

## Notes

- L'outil répond avec `200 OK` et `{}` pour ne pas bloquer le client
- Le port 8080 est utilisé pour éviter les conflits avec le serveur principal (port 80)
- Pour tester sur le port 80, arrêtez d'abord le serveur principal

