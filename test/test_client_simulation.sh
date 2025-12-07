#!/bin/bash

echo "=========================================="
echo "Simulating BP_MQX_ETH Client Requests"
echo "=========================================="
echo ""

# Test 1: GET /api/serverinfos
echo "1. GET /api/serverinfos"
echo "Expected: {\"isconnected\":true,\"infos\":[613,607,615,590,349,350,351,352,363,425,426,920],\"newversion\":\"no\"}"
echo "Actual:"
curl -s http://localhost:80/api/serverinfos
echo ""
echo ""

# Test 2: POST /api/mystatus with valid JSON
echo "2. POST /api/mystatus (valid JSON)"
curl -s -X POST http://localhost:80/api/mystatus \
  -H "Content-Type: application/json" \
  -d '{"version":"V37","ek":[{"k":613,"v":"1"},{"k":607,"v":"0"}]}'
echo "Expected: HTTP 201 Created"
echo ""
echo ""

# Test 3: POST /api/mystatus with malformed JSON (like C client sends)
echo "3. POST /api/mystatus (malformed JSON - unquoted keys)"
curl -s -X POST http://localhost:80/api/mystatus \
  -H "Content-Type: application/json" \
  -d '{version:"V37",ek:[{k:613,v:"1"},{k:607,v:"0"}]}'
echo "Expected: HTTP 201 Created"
echo ""
echo ""

# Test 4: GET /api/myactions
echo "4. GET /api/myactions"
echo "Expected: {\"_de67f\":null,\"actions\":[]}"
echo "Actual:"
curl -s http://localhost:80/api/myactions
echo ""
echo ""

echo "=========================================="
echo "Tests completed!"
echo "=========================================="
