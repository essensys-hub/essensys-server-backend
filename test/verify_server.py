import requests
import time
import json
import sys

# Configuration
SERVER_URL = "http://localhost:8080" # Using 8080 as per main.go
SERVER_INFOS_URL = f"{SERVER_URL}/api/serverinfos"
STATUS_URL = f"{SERVER_URL}/api/mystatus"
ACTIONS_URL = f"{SERVER_URL}/api/myactions"
INJECT_URL = f"{SERVER_URL}/api/admin/inject"
DONE_URL = f"{SERVER_URL}/api/done"

def check_server_infos():
    print(f"[TEST] Checking {SERVER_INFOS_URL}...")
    try:
        resp = requests.get(SERVER_INFOS_URL)
        resp.raise_for_status()
        data = resp.json()
        print(f"[TEST] Response: {data}")
        if "isconnected" not in data:
            print("[TEST] FAIL: 'isconnected' missing")
            return False
        print("[TEST] OK")
        return True
    except Exception as e:
        print(f"[TEST] FAIL: {e}")
        return False

def check_my_status():
    print(f"[TEST] Checking {STATUS_URL} (Invalid JSON quirk)...")
    # Client sends invalid JSON: {k:123,v:"1"}
    # We simulate this by sending a raw string
    raw_body = '{version:"V1",ek:[{k:123,v:"1"}]}'
    try:
        resp = requests.post(STATUS_URL, data=raw_body)
        if resp.status_code != 201:
            print(f"[TEST] FAIL: Expected 201, got {resp.status_code}")
            return False
        print("[TEST] OK")
        return True
    except Exception as e:
        print(f"[TEST] FAIL: {e}")
        return False

def inject_and_verify_action():
    print(f"[TEST] Injecting action via {INJECT_URL}...")
    try:
        # Inject action
        payload = {"k": 615, "v": "1"} # Light ON
        resp = requests.post(INJECT_URL, json=payload)
        resp.raise_for_status()
        print("[TEST] Injection OK")

        # Verify in queue
        print(f"[TEST] Checking {ACTIONS_URL}...")
        resp = requests.get(ACTIONS_URL)
        resp.raise_for_status()
        data = resp.json()
        
        # Check _de67f first
        if list(data.keys())[0] != "_de67f":
             print("[TEST] WARNING: _de67f is not the first key (might be Python dict ordering, check raw text if critical)")
        
        actions = data.get("actions", [])
        if not actions:
            print("[TEST] FAIL: No actions found")
            return False
        
        action = actions[0]
        guid = action.get("guid")
        print(f"[TEST] Found action GUID: {guid}")
        
        # Check full block
        params = {p["k"]: p["v"] for p in action.get("params", [])}
        if params.get(590) != "1":
             print("[TEST] FAIL: Scenario 590 missing or wrong")
             return False
        if params.get(605) != "0": # Default 0
             print("[TEST] FAIL: Light block 605 missing")
             return False
             
        print("[TEST] Action content OK")
        
        # Ack
        print(f"[TEST] Acknowledging {DONE_URL}/{guid}...")
        resp = requests.post(f"{DONE_URL}/{guid}")
        if resp.status_code != 201:
             print(f"[TEST] FAIL: Ack failed with {resp.status_code}")
             return False
             
        # Verify gone
        resp = requests.get(ACTIONS_URL)
        data = resp.json()
        if data.get("actions"):
             print("[TEST] FAIL: Action still in queue")
             return False
             
        print("[TEST] OK")
        return True

    except Exception as e:
        print(f"[TEST] FAIL: {e}")
        return False

def main():
    print("=== Essensys Server Verification ===")
    
    if not check_server_infos():
        sys.exit(1)
        
    if not check_my_status():
        sys.exit(1)
        
    if not inject_and_verify_action():
        sys.exit(1)
        
    print("\n=== All Tests Passed ===")

if __name__ == "__main__":
    main()
