#!/bin/bash

# Ensure we are running as root
if [ "$EUID" -ne 0 ]; then 
  echo "Please run as root (sudo)"
  exit 1
fi

# Kill any running servers
echo "Stopping any existing servers..."
pkill -f "server.sample"
pkill -f "./server"

# --- Test 1: server.sample ---
echo "------------------------------------------------"
echo "Starting server.sample on port 80..."
./server.sample -port 80 > sample_server.log 2>&1 &
SAMPLE_PID=$!
echo "Server PID: $SAMPLE_PID"

# Wait for server to start
sleep 2

echo "Running test_chb3.py against server.sample..."
python3 ./test_chb3.py > test_chb3.sample.log 2>&1

echo "Stopping server.sample..."
kill $SAMPLE_PID
wait $SAMPLE_PID 2>/dev/null

# --- Test 2: server ---
echo "------------------------------------------------"
echo "Starting new server on port 80..."
./server > new_server.log 2>&1 &
SERVER_PID=$!
echo "Server PID: $SERVER_PID"

# Wait for server to start
sleep 2

echo "Running test_chb3.py against new server..."
python3 ./test_chb3.py > test_chb3.new.log 2>&1

echo "Stopping new server..."
kill $SERVER_PID
wait $SERVER_PID 2>/dev/null

echo "------------------------------------------------"
echo "Verification complete."
echo "Logs generated: test_chb3.sample.log, test_chb3.new.log"
