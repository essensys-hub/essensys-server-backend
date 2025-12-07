#!/bin/bash

# Kill any running servers
pkill -f "server.sample"
pkill -f "./server"

# Start server.sample on port 8081
echo "Starting server.sample on port 8081..."
./server.sample -port 8081 > sample.log 2>&1 &
SAMPLE_PID=$!

# Start server on port 8082
echo "Starting server on port 8082..."
# Use env var to override port for testing only
export SERVER_PORT=8082
./server > server.log 2>&1 &
SERVER_PID=$!

sleep 2

# Inject action to server.sample
echo "Injecting to server.sample..."
curl -s -X POST -d '[{"k":613,"v":"1"}]' http://localhost:8081/api/admin/inject

# Inject action to server
echo "Injecting to server..."
curl -s -X POST -d '[{"k":613,"v":"1"}]' http://localhost:8082/api/admin/inject

sleep 1

# Get actions from server.sample
echo "Fetching actions from server.sample..."
SAMPLE_RESP=$(curl -s http://localhost:8081/api/myactions)
echo "Sample Response: $SAMPLE_RESP"

# Get actions from server
echo "Fetching actions from server..."
SERVER_RESP=$(curl -s http://localhost:8082/api/myactions)
echo "Server Response: $SERVER_RESP"

# Cleanup
kill $SAMPLE_PID
kill $SERVER_PID
