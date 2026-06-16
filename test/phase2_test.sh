#!/bin/bash

# Script de test pour la Phase 2 : Repositories PostgreSQL
# Ce script teste la connexion à la base de données et les repositories

set -e

echo "=========================================="
echo "Test Phase 2 : Repositories PostgreSQL"
echo "=========================================="
echo ""

# Couleurs pour l'output
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Configuration de la base de données de test
DB_HOST=${DB_HOST:-localhost}
DB_PORT=${DB_PORT:-5432}
DB_USER=${DB_USER:-nrineau}
DB_PASSWORD=${DB_PASSWORD:?Définir DB_PASSWORD avant d'exécuter ce script}
DB_NAME=${DB_NAME:-essensys_test}

echo "Configuration de la base de données de test:"
echo "  Host: $DB_HOST"
echo "  Port: $DB_PORT"
echo "  User: $DB_USER"
echo "  Database: $DB_NAME"
echo ""

# Vérifier que PostgreSQL est accessible
echo -n "Vérification de la connexion PostgreSQL... "
if PGPASSWORD=$DB_PASSWORD psql -h $DB_HOST -p $DB_PORT -U $DB_USER -d postgres -c "SELECT 1" > /dev/null 2>&1; then
    echo -e "${GREEN}OK${NC}"
else
    echo -e "${RED}ÉCHEC${NC}"
    echo "Erreur: Impossible de se connecter à PostgreSQL"
    echo "Assurez-vous que PostgreSQL est démarré et accessible"
    exit 1
fi

# Créer la base de données de test si elle n'existe pas
echo -n "Création de la base de données de test... "
PGPASSWORD=$DB_PASSWORD psql -h $DB_HOST -p $DB_PORT -U $DB_USER -d postgres -c "CREATE DATABASE $DB_NAME" 2>/dev/null || true
echo -e "${GREEN}OK${NC}"

# Appliquer les migrations
echo -n "Application des migrations... "
cd "$(dirname "$0")/.."
if PGPASSWORD=$DB_PASSWORD psql -h $DB_HOST -p $DB_PORT -U $DB_USER -d $DB_NAME -f migrations/001_initial_schema.up.sql > /dev/null 2>&1; then
    echo -e "${GREEN}OK${NC}"
else
    echo -e "${YELLOW}AVERTISSEMENT${NC}"
    echo "Les migrations ont peut-être déjà été appliquées"
fi

# Exécuter les tests Go
echo ""
echo "Exécution des tests Go..."
echo ""

export DB_HOST=$DB_HOST
export DB_PORT=$DB_PORT
export DB_USER=$DB_USER
export DB_PASSWORD=$DB_PASSWORD
export DB_NAME=$DB_NAME

if go test ./internal/data/... -v -run "TestDatabaseStore|TestUserRepository|TestMachineRepository"; then
    echo ""
    echo -e "${GREEN}=========================================="
    echo "Tous les tests sont passés !"
    echo "==========================================${NC}"
    exit 0
else
    echo ""
    echo -e "${RED}=========================================="
    echo "Certains tests ont échoué"
    echo "==========================================${NC}"
    exit 1
fi

