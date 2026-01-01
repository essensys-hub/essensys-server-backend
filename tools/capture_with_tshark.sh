#!/bin/bash

# Script pour capturer le token d'authentification avec tshark
# Plus précis que tcpdump pour l'analyse HTTP

echo "==========================================="
echo "Capture Auth Token avec tshark"
echo "==========================================="
echo ""
echo "Capture du trafic HTTP sur le port 80"
echo "Appuyez sur Ctrl+C pour arrêter"
echo "==========================================="
echo ""

# Vérifier les permissions
if [ "$EUID" -ne 0 ]; then 
    echo "⚠️  Ce script nécessite les permissions root (sudo)"
    echo "Relancez avec: sudo $0"
    exit 1
fi

# Vérifier si tshark est installé
if ! command -v tshark &> /dev/null; then
    echo "❌ tshark n'est pas installé"
    echo "Installez-le avec: brew install wireshark (macOS) ou apt-get install tshark (Linux)"
    exit 1
fi

# Interface réseau
INTERFACE="any"
PORT=80

echo "Capture sur l'interface: $INTERFACE, port: $PORT"
echo "En attente de requêtes du client IoT..."
echo ""

# Capturer avec tshark et extraire le header Authorization
tshark -i "$INTERFACE" \
    -f "tcp port $PORT" \
    -Y "http.request and http.authorization" \
    -T fields \
    -e frame.time \
    -e ip.src \
    -e ip.dst \
    -e http.request.method \
    -e http.request.uri \
    -e http.authorization \
    2>/dev/null | while IFS=$'\t' read -r time src dst method uri auth; do
    if [ ! -z "$auth" ]; then
        echo ""
        echo "==========================================="
        echo "TOKEN D'AUTHENTIFICATION DÉTECTÉ"
        echo "==========================================="
        echo "Time: $time"
        echo "From: $src -> $dst"
        echo "Method: $method"
        echo "URI: $uri"
        echo "Authorization: $auth"
        echo ""
        
        # Extraire le token Base64
        TOKEN=$(echo "$auth" | grep -oP "Basic \K[^\s]+" || echo "")
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
                echo "Username (first 16 hex): $USERNAME"
                echo "Password (last 16 hex): $PASSWORD"
                echo "Hashed Pkey (concatenated): ${USERNAME}${PASSWORD}"
                echo "Hashed Pkey Length: ${#USERNAME}${#PASSWORD} characters"
                echo ""
                echo "--- REQUÊTE SQL ---"
                echo "SELECT * FROM es_machine WHERE hashed_pkey = '${USERNAME}${PASSWORD}' AND is_active = true;"
                echo ""
                echo "--- MISE À JOUR SI NÉCESSAIRE ---"
                echo "UPDATE es_machine SET hashed_pkey = '${USERNAME}${PASSWORD}' WHERE no_serie = 'ESS002SU1702280040';"
                echo "==========================================="
                echo ""
            else
                echo "⚠️  Impossible de décoder le token Base64"
            fi
        else
            echo "⚠️  Token Base64 non trouvé dans le header Authorization"
        fi
    fi
done

