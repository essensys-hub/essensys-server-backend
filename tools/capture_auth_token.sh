#!/bin/bash

# Script pour capturer le token d'authentification du client IoT
# Utilise tcpdump pour capturer le trafic réseau

echo "==========================================="
echo "Capture Auth Token - Essensys IoT Client"
echo "==========================================="
echo ""
echo "Ce script capture le trafic HTTP sur le port 80"
echo "et extrait le header Authorization du client IoT."
echo ""
echo "Appuyez sur Ctrl+C pour arrêter la capture."
echo "==========================================="
echo ""

# Vérifier les permissions
if [ "$EUID" -ne 0 ]; then 
    echo "⚠️  Ce script nécessite les permissions root (sudo)"
    echo "Relancez avec: sudo $0"
    exit 1
fi

# Interface réseau (ajustez si nécessaire)
INTERFACE="any"
PORT=80

echo "Capture sur l'interface: $INTERFACE, port: $PORT"
echo "En attente de requêtes du client IoT..."
echo ""

# Capturer le trafic et extraire le header Authorization
tcpdump -i "$INTERFACE" -A -s 0 "tcp port $PORT" 2>/dev/null | \
    grep --line-buffered -A 50 "Authorization" | \
    while IFS= read -r line; do
        if echo "$line" | grep -q "Authorization"; then
            echo ""
            echo "==========================================="
            echo "TOKEN D'AUTHENTIFICATION DÉTECTÉ"
            echo "==========================================="
            echo "$line"
            echo ""
            
            # Extraire le token Base64
            TOKEN=$(echo "$line" | grep -oP "Basic \K[^\s]+" || echo "")
            if [ ! -z "$TOKEN" ]; then
                echo "Token Base64: $TOKEN"
                echo ""
                
                # Décoder le token
                DECODED=$(echo "$TOKEN" | base64 -d 2>/dev/null)
                if [ ! -z "$DECODED" ]; then
                    echo "Token décodé (username:password): $DECODED"
                    
                    # Extraire username et password
                    USERNAME=$(echo "$DECODED" | cut -d: -f1)
                    PASSWORD=$(echo "$DECODED" | cut -d: -f2)
                    
                    echo ""
                    echo "--- DÉTAILS ---"
                    echo "Username (16 hex): $USERNAME"
                    echo "Password (16 hex): $PASSWORD"
                    echo "Hashed Pkey: ${USERNAME}${PASSWORD}"
                    echo ""
                    echo "--- REQUÊTE SQL ---"
                    echo "SELECT * FROM es_machine WHERE hashed_pkey = '${USERNAME}${PASSWORD}' AND is_active = true;"
                    echo "==========================================="
                    echo ""
                fi
            fi
        fi
    done


