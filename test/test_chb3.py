import requests
import time
import json
import sys

SERVER_URL = "http://localhost:80"
INJECT_URL = f"{SERVER_URL}/api/admin/inject"
ACTIONS_URL = f"{SERVER_URL}/api/myactions"

# Indices calculated from Tbb_Donnees_Index.cs
# Scenario1 = 592
# Scenario_Allumer_CHB_LSB = Offset 21 -> Index 613
# Scenario_Eteindre_CHB_LSB = Offset 15 -> Index 607
# Value for Petite Chambre 3 = 64 (0x40)

INDEX_ON = 613
INDEX_OFF = 607
VALUE = "64"

def inject_action(k, v):
    print(f"[TEST] Injecting Action: k={k}, v={v}")
    try:
        payload = {"k": k, "v": v}
        resp = requests.post(INJECT_URL, json=payload)
        resp.raise_for_status()
        print("[TEST] Injection OK")
    except Exception as e:
        print(f"[TEST] Injection Failed: {e}")
        sys.exit(1)

def wait_for_done():
    print("[TEST] Waiting for client to acknowledge (queue empty)...")
    while True:
        try:
            resp = requests.get(ACTIONS_URL)
            resp.raise_for_status()
            data = resp.json()
            actions = data.get("actions", [])
            
            if not actions:
                print("[TEST] Action queue empty. Done.")
                return
            
            # Optional: Print what's pending
            # print(f"[TEST] Pending: {len(actions)} actions")
            
            time.sleep(0.5)
        except Exception as e:
            print(f"[TEST] Error polling actions: {e}")
            time.sleep(1)

def verify_action_content(expected_k, expected_v):
    print("[TEST] Verifying action content...")
    try:
        resp = requests.get(ACTIONS_URL)
        resp.raise_for_status()
        data = resp.json()
        actions = data.get("actions", [])
        
        if not actions:
            print("[TEST] ERROR: No actions found to verify!")
            sys.exit(1)
            
        params = actions[0].get("params", [])
        param_map = {p["k"]: p["v"] for p in params}
        
        # Check target value
        if param_map.get(expected_k) != expected_v:
            print(f"[TEST] ERROR: Expected k={expected_k} v={expected_v}, got {param_map.get(expected_k)}")
            sys.exit(1)
            
        # Check Scenario trigger
        if param_map.get(590) != "1":
            print(f"[TEST] ERROR: Expected k=590 v=1, got {param_map.get(590)}")
            sys.exit(1)
            
        # Check full block 605-622 (excluding target)
        for k in range(605, 623):
            if k == expected_k:
                continue
            if param_map.get(k) != "0":
                print(f"[TEST] ERROR: Expected k={k} v=0, got {param_map.get(k)}")
                sys.exit(1)
                
        print("[TEST] Verification SUCCESS: JSON matches standard format.")
        
    except Exception as e:
        print(f"[TEST] Verification Failed: {e}")
        sys.exit(1)

def main():
    # 1. Turn ON
    print("--- STEP 1: Turn ON ---")
    inject_action(INDEX_ON, VALUE)
    verify_action_content(INDEX_ON, VALUE)
    wait_for_done()
    
    # 2. Wait 10 seconds
    print("\n[TEST] Waiting 10 seconds...")
    time.sleep(10)
    
    # 3. Turn OFF
    print("\n--- STEP 2: Turn OFF ---")
    inject_action(INDEX_OFF, VALUE)
    verify_action_content(INDEX_OFF, VALUE)
    wait_for_done()
    
    print("\n[TEST] Sequence Completed Successfully.")

if __name__ == "__main__":
    main()
