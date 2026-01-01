# Guide Rapide : Tester le Client Legacy

## Test Rapide (1 commande)

```bash
cd essensys-server-backend
./test/test_legacy_client.sh
```

Ce script fait tout automatiquement :
- Compile le serveur
- Lance le serveur sur le port 80
- Exécute le test client legacy
- Affiche les résultats

## Test Manuel (2 terminaux)

### Terminal 1 : Serveur Go

```bash
cd essensys-server-backend

# Compiler
go build -o server ./cmd/server

# Lancer (nécessite sudo pour port 80)
sudo ./server
```

### Terminal 2 : Test Client

```bash
cd essensys-server-backend/test
python3 test_chb3.py
```

## Résultat Attendu

Le test devrait afficher :
```
--- STEP 1: Turn ON ---
[TEST] Injecting Action: k=613, v=64
[TEST] Injection OK
[TEST] Verifying action content...
[TEST] Verification SUCCESS: JSON matches standard format.
[TEST] Waiting for client to acknowledge (queue empty)...
[TEST] Action queue empty. Done.

--- STEP 2: Turn OFF ---
[TEST] Injecting Action: k=607, v=64
[TEST] Injection OK
[TEST] Verification SUCCESS: JSON matches standard format.
[TEST] Action queue empty. Done.

[TEST] Sequence Completed Successfully.
```

## Vérification

Si le test passe, cela signifie que :
- ✅ Le serveur Go fonctionne
- ✅ Le protocole legacy est compatible
- ✅ Les actions sont bien formatées (indices 605-622, index 590)
- ✅ Le client peut recevoir et exécuter les actions

## Problèmes Courants

**Port 80 occupé** :
```bash
sudo lsof -i :80  # Voir qui utilise le port
sudo kill <PID>    # Arrêter le processus
```

**Permission denied** :
```bash
# Linux
sudo setcap 'cap_net_bind_service=+ep' ./server

# macOS
sudo ./server
```

**Python requests manquant** :
```bash
pip3 install requests
```

