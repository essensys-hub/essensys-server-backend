#!/bin/bash

echo "Testing /api/serverinfos endpoint..."
echo ""
echo "Expected: {\"isconnected\":true,\"infos\":[613,607,615,590,349,350,351,352,363,425,426,920],\"newversion\":\"no\"}"
echo ""
echo "Actual:"
curl -s http://localhost:80/api/serverinfos | jq .
