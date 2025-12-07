#!/bin/bash

echo "Testing /api/admin/inject endpoint..."
echo ""

# Test injection
echo "1. Injecting action k=613 v=64"
curl -X POST http://localhost:80/api/admin/inject \
  -H "Content-Type: application/json" \
  -d '{"k":613,"v":"64"}' \
  -v 2>&1 | grep -E "(HTTP|status|guid)"

echo ""
echo ""

# Check actions
echo "2. Checking /api/myactions"
curl -s http://localhost:80/api/myactions | jq .

echo ""
echo "Done!"
