#!/bin/bash

# Script de test pour vérifier la compatibilité avec le client legacy IoT
# Ce script lance le serveur Go et teste avec test_chb3.py

set -e

# Couleurs
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo -e "${BLUE}=========================================="
echo "Test Client Legacy IoT"
echo "==========================================${NC}"
echo ""

# Vérifier que Go est installé
if ! command -v go &> /dev/null; then
    echo -e "${RED}Erreur: Go n'est pas installé${NC}"
    exit 1
fi

# Vérifier que Python est installé
if ! command -v python3 &> /dev/null; then
    echo -e "${RED}Erreur: Python3 n'est pas installé${NC}"
    exit 1
fi

# Vérifier que requests est installé
if ! python3 -c "import requests" 2>/dev/null; then
    echo -e "${YELLOW}Installation de requests...${NC}"
    pip3 install requests
fi

# Aller dans le répertoire du projet
cd "$(dirname "$0")/.."

# Compiler le serveur
echo -e "${BLUE}1. Compilation du serveur Go...${NC}"
if go build -o server ./cmd/server; then
    echo -e "${GREEN}   ✓ Serveur compilé${NC}"
else
    echo -e "${RED}   ✗ Erreur de compilation${NC}"
    exit 1
fi

# Vérifier les permissions pour le port 80
echo ""
echo -e "${BLUE}2. Vérification du port 80...${NC}"
if lsof -Pi :80 -sTCP:LISTEN -t >/dev/null 2>&1; then
    echo -e "${YELLOW}   ⚠ Port 80 déjà utilisé${NC}"
    echo "   Arrêt du processus existant..."
    sudo lsof -ti:80 | xargs sudo kill -9 2>/dev/null || true
    sleep 2
fi

# Configurer les permissions pour le port 80 (Linux)
if [[ "$OSTYPE" == "linux-gnu"* ]]; then
    echo -e "${BLUE}3. Configuration des permissions port 80...${NC}"
    sudo setcap 'cap_net_bind_service=+ep' ./server 2>/dev/null || {
        echo -e "${YELLOW}   ⚠ Impossible de configurer setcap, utilisation de sudo${NC}"
        USE_SUDO=true
    }
fi

# Lancer le serveur en arrière-plan
echo ""
echo -e "${BLUE}4. Démarrage du serveur Go...${NC}"
if [ "$USE_SUDO" = true ]; then
    sudo ./server > test/server.log 2>&1 &
else
    ./server > test/server.log 2>&1 &
fi
SERVER_PID=$!

echo "   Serveur démarré (PID: $SERVER_PID)"
echo "   Logs: test/server.log"

# Attendre que le serveur soit prêt
echo ""
echo -e "${BLUE}5. Attente du démarrage du serveur...${NC}"
for i in {1..10}; do
    if curl -s http://localhost/health > /dev/null 2>&1; then
        echo -e "${GREEN}   ✓ Serveur prêt${NC}"
        break
    fi
    if [ $i -eq 10 ]; then
        echo -e "${RED}   ✗ Serveur n'a pas démarré après 10 secondes${NC}"
        kill $SERVER_PID 2>/dev/null || true
        exit 1
    fi
    sleep 1
    echo "   Attente... ($i/10)"
done

# Lancer le test Python
echo ""
echo -e "${BLUE}6. Exécution du test client legacy (test_chb3.py)...${NC}"
echo ""

cd test
if python3 test_chb3.py; then
    echo ""
    echo -e "${GREEN}=========================================="
    echo "✓ Test réussi ! Le client legacy fonctionne"
    echo "==========================================${NC}"
    TEST_RESULT=0
else
    echo ""
    echo -e "${RED}=========================================="
    echo "✗ Test échoué"
    echo "==========================================${NC}"
    TEST_RESULT=1
fi
cd ..

# Arrêter le serveur
echo ""
echo -e "${BLUE}7. Arrêt du serveur...${NC}"
if [ "$USE_SUDO" = true ]; then
    sudo kill $SERVER_PID 2>/dev/null || true
else
    kill $SERVER_PID 2>/dev/null || true
fi

# Attendre que le serveur s'arrête
sleep 2

# Afficher les logs du serveur
echo ""
echo -e "${BLUE}8. Dernières lignes des logs du serveur:${NC}"
echo "----------------------------------------"
tail -20 test/server.log || true
echo "----------------------------------------"

# Nettoyer
rm -f server

exit $TEST_RESULT


