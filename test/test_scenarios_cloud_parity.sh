#!/usr/bin/env bash
# Parité cloud vs armoire — après sync profil Scénarios (591–919)
# Compare un échantillon d'indices slot Perso 1 (633+) entre gateway LAN et portail OVH.
set -euo pipefail

LAN_BASE="${LAN_BASE:-http://127.0.0.1:80}"
CLOUD_BASE="${CLOUD_BASE:-https://mon.essensys.fr}"
EMAIL="${EMAIL:-}"
JWT_SECRET="${JWT_SECRET:-}"
SLOT=6
# Scenario2 base = 633 ; Perso 1 = slot 6 → base 633 + (6-2)*41 = 797
SAMPLE_KEYS="${SAMPLE_KEYS:-797 798 799 591}"

if [[ -z "$EMAIL" || -z "$JWT_SECRET" ]]; then
  echo "Usage: EMAIL=user@example.com JWT_SECRET=... $0"
  echo "Optionnel: LAN_BASE CLOUD_BASE SLOT SAMPLE_KEYS"
  exit 1
fi

GENJWT="$(dirname "$0")/../../essensys-user-portal-frontend/test/genjwt.go"
BACKEND_DIR="${BACKEND_DIR:-../../essensys-user-portal-backend}"
TOKEN=$(cd "$BACKEND_DIR" 2>/dev/null && JWT_SECRET="$JWT_SECRET" go run "$GENJWT" "$EMAIL" 2>/dev/null) || \
  TOKEN=$(JWT_SECRET="$JWT_SECRET" go run "$GENJWT" "$EMAIL")
AUTH=(-H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json")

echo "=== Parité scénarios slot $SLOT (LAN vs cloud) ==="

LAN_JSON=$(curl -sS -f -H "X-Client-ID: default" "$LAN_BASE/api/scenarios/$SLOT")
CLOUD_JSON=$(curl -sS -f "${AUTH[@]}" "$CLOUD_BASE/api/portal/scenarios/$SLOT")

mismatch=0
for k in $SAMPLE_KEYS; do
  lan_v=$(echo "$LAN_JSON" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('params',{}).get('$k','?'))" 2>/dev/null || echo "?")
  cloud_v=$(echo "$CLOUD_JSON" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('params',{}).get('$k','?'))" 2>/dev/null || echo "?")
  if [[ "$lan_v" != "$cloud_v" ]]; then
    echo "[DIFF] k=$k LAN=$lan_v CLOUD=$cloud_v"
    mismatch=$((mismatch + 1))
  else
    echo "[OK] k=$k v=$lan_v"
  fi
done

if [[ $mismatch -gt 0 ]]; then
  echo "[FAIL] $mismatch différences — relancer sync profil Scénarios ou attendre fin de run"
  exit 1
fi
echo "=== Parité OK ($SAMPLE_KEYS) ==="
