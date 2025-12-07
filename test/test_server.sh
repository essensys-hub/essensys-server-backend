#!/bin/bash

echo "Testing Essensys Backend Server..."
echo ""

# Test health endpoint
echo "1. Testing /health endpoint:"
curl -v http://localhost:80/health 2>&1 | grep -E "(HTTP|status)"
echo ""

# Test serverinfos endpoint
echo "2. Testing /api/serverinfos endpoint:"
curl -v http://localhost:80/api/serverinfos 2>&1 | grep -E "(HTTP|isconnected)"
echo ""

# Test mystatus endpoint
echo "3. Testing /api/mystatus endpoint:"
curl -X POST http://localhost:80/api/mystatus \
  -H "Content-Type: application/json" \
  -d '{"version":"V37","ek":[{"k":1,"v":"100"}]}' \
  -v 2>&1 | grep -E "(HTTP|201)"
echo ""

# Test myactions endpoint
echo "4. Testing /api/myactions endpoint:"
curl -v http://localhost:80/api/myactions 2>&1 | grep -E "(HTTP|actions)"
echo ""

echo "Tests completed!"
