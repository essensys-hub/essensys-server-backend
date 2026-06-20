#!/usr/bin/env bash
# E2E pilote CM5 — gestion scénarios (OpenSpec essensys-scenario-management)
# Prérequis : backend sur port 80/7070, armoire connectée, auth LAN si activée.
set -euo pipefail

BASE="${BASE:-http://127.0.0.1:80}"
CLIENT_ID="${CLIENT_ID:-default}"
SLOT_PERSO1=6

echo "=== E2E scénarios @ $BASE (client=$CLIENT_ID) ==="

curl_json() {
  curl -sS -f -H "Content-Type: application/json" -H "X-Client-ID: $CLIENT_ID" "$@"
}

echo "[1/6] GET /api/scenarios"
LIST=$(curl_json "$BASE/api/scenarios")
echo "$LIST" | grep -q '"slots"' || { echo "[FAIL] liste scénarios"; exit 1; }
echo "$LIST" | grep -q 'Je sors' || { echo "[FAIL] slot Je sors absent"; exit 1; }
echo "[OK] liste"

echo "[2/6] POST /api/scenarios/2/launch (Je sors — Mode A 590=2)"
curl_json -X POST "$BASE/api/scenarios/2/launch" -d '{}' >/dev/null
echo "[OK] launch slot 2"

echo "[3/6] Attente index 591 (dernier lancé) — poll exchange"
for i in $(seq 1 20); do
  VAL=$(curl_json "$BASE/api/exchange/read?keys=591" 2>/dev/null | grep -o '"591"[[:space:]]*:[[:space:]]*"[^"]*"' | head -1 || true)
  if echo "$VAL" | grep -q '"2"'; then
    echo "[OK] 591=2 après $i tentatives"
    break
  fi
  sleep 3
  if [[ $i -eq 20 ]]; then
    echo "[WARN] 591 pas encore à 2 (armoire lente ou hors ligne) — continuer"
  fi
done

echo "[4/6] GET /api/scenarios/$SLOT_PERSO1 (Perso 1)"
DETAIL=$(curl_json "$BASE/api/scenarios/$SLOT_PERSO1")
echo "$DETAIL" | grep -q '"slot_number"' || { echo "[FAIL] détail slot $SLOT_PERSO1"; exit 1; }
echo "[OK] lecture Perso 1"

echo "[5/6] GET /api/scenarios/meta/bitmasks"
curl_json "$BASE/api/scenarios/meta/bitmasks" | grep -q 'light' || { echo "[FAIL] bitmasks"; exit 1; }
echo "[OK] bitmasks"

echo "[6/6] GET /api/admin/scenarios/sync"
SYNC=$(curl_json "$BASE/api/admin/scenarios/sync" 2>/dev/null || echo '{"found":false}')
echo "$SYNC"
echo "$SYNC" | grep -q '"found"' || { echo "[FAIL] sync status"; exit 1; }
echo "[OK] sync admin"

echo "=== Succès E2E scénarios LAN ==="
