package main

import (
	"bufio"
	"crypto/rand"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"
)

// --- Structures JSON ---

type ServerInfosResponse struct {
	IsConnected bool   `json:"isconnected"`
	Infos       []int  `json:"infos"`
	NewVersion  string `json:"newversion"`
}

type EkItem struct {
	K int    `json:"k"`
	V string `json:"v"`
}

type MyActionsResponse struct {
	De67f   interface{} `json:"_de67f"` // Must be first for client parser!
	Actions []Action    `json:"actions"`
}

type MyStatusRequest struct {
	Version string   `json:"version"`
	Ek      []EkItem `json:"ek"`
}

type ActionParam struct {
	K int    `json:"k"`
	V string `json:"v"`
}

type Action struct {
	Guid   string        `json:"guid"`
	Params []ActionParam `json:"params"`
}

// --- Storage (Thread-Safe) ---

type Storage struct {
	sync.RWMutex
	ExchangeTable map[int][]string // History of last 25 values
	ActionQueue   []Action
}

var store = &Storage{
	ExchangeTable: make(map[int][]string),
	ActionQueue:   make([]Action, 0),
}

func (s *Storage) UpdateValue(k int, v string) {
	s.Lock()
	defer s.Unlock()
	
	history, ok := s.ExchangeTable[k]
	if !ok {
		history = make([]string, 0)
	}
	
	// Append new value
	history = append(history, v)
	
	// Keep last 25
	if len(history) > 25 {
		history = history[len(history)-25:]
	}
	
	s.ExchangeTable[k] = history
}

func (s *Storage) AddAction(action Action) {
	s.Lock()
	defer s.Unlock()
	s.ActionQueue = append(s.ActionQueue, action)
	fmt.Printf("[STORAGE] Added action %s\n", action.Guid)
}

func (s *Storage) GetPendingActions() []Action {
	s.RLock()
	defer s.RUnlock()
	actions := make([]Action, len(s.ActionQueue))
	copy(actions, s.ActionQueue)
	return actions
}

func (s *Storage) RemoveAction(guid string) bool {
	s.Lock()
	defer s.Unlock()
	for i, action := range s.ActionQueue {
		if action.Guid == guid {
			s.ActionQueue = append(s.ActionQueue[:i], s.ActionQueue[i+1:]...)
			return true
		}
	}
	return false
}

// --- Helper ---

func generateUUID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

// --- Main Server ---

func main() {
	blinkPtr := flag.Bool("blink", false, "Enable blinking mode (10s ON / 10s OFF)")
	portPtr := flag.String("port", "80", "Port to listen on")
	flag.Parse()

	port := ":" + *portPtr
	
	if *blinkPtr {
		fmt.Println("[GO] Blinking mode ENABLED")
		go startBlinking()
	}

	listener, err := net.Listen("tcp", port)
	if err != nil {
		fmt.Printf("Error listening on %s: %v\n", port, err)
		return
	}
	defer listener.Close()
	fmt.Printf("[GO SERVER] Listening on %s\n", port)

	for {
		conn, err := listener.Accept()
		if err != nil {
			fmt.Printf("Error accepting connection: %v\n", err)
			continue
		}
		go handleConnection(conn)
	}
}

func startBlinking() {
	// Indices from Index.cshtml
	// Escalier ON: index 613, value "1"
	// Escalier OFF: index 607, value "1"
	
	for {
		// ON
		guid := generateUUID()
		store.AddAction(Action{Guid: guid, Params: []ActionParam{{K: 613, V: "1"}}})
		fmt.Println("[BLINK] Light ON (Action queued)")
		time.Sleep(10 * time.Second)

		// OFF
		guid = generateUUID()
		store.AddAction(Action{Guid: guid, Params: []ActionParam{{K: 607, V: "1"}}})
		fmt.Println("[BLINK] Light OFF (Action queued)")
		time.Sleep(10 * time.Second)
	}
}

func handleConnection(conn net.Conn) {
	defer conn.Close()
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))

	reader := bufio.NewReader(conn)
	
	// 1. Read Request Line
	requestLine, err := reader.ReadString('\n')
	if err != nil {
		return
	}
	requestLine = strings.TrimSpace(requestLine)
	parts := strings.Split(requestLine, " ")
	if len(parts) < 2 {
		return
	}
	method := parts[0]
	path := parts[1]

	fmt.Printf("[GO] %s %s (%s)\n", method, path, conn.RemoteAddr().String())

	// 2. Read Headers
	headers := make(map[string]string)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			break
		}
		line = strings.TrimSpace(line)
		if line == "" {
			break // End of headers
		}
		if idx := strings.Index(line, ":"); idx > 0 {
			key := strings.TrimSpace(line[:idx])
			val := strings.TrimSpace(line[idx+1:])
			headers[key] = val
		}
	}

	// 3. Read Body
	body := []byte{}
	if clStr, ok := headers["Content-Length"]; ok {
		var cl int
		fmt.Sscanf(clStr, "%d", &cl)
		if cl > 0 {
			body = make([]byte, cl)
			_, err := io.ReadFull(reader, body)
			if err != nil {
				fmt.Printf("[GO] Error reading body: %v\n", err)
				return
			}
		}
	}

	// 4. Routing
	if path == "/api/serverinfos" && method == "GET" {
		handleServerInfos(conn)
	} else if path == "/api/mystatus" && method == "POST" {
		handleMyStatus(conn, body)
	} else if path == "/api/myactions" && method == "GET" {
		handleMyActions(conn)
	} else if strings.HasPrefix(path, "/api/done") && method == "POST" {
		handleDone(conn, path)
	} else if path == "/api/admin/inject" && method == "POST" {
		handleAdminInject(conn, body)
	} else if path == "/api/view-status" && method == "GET" {
		handleViewStatus(conn)
	} else if path == "/" && method == "GET" {
		handleIndex(conn)
	} else {
		sendResponse(conn, 404, "Not Found", "")
	}
}

// --- Handlers ---

func handleIndex(conn net.Conn) {
	html := `<!DOCTYPE html>
<html>
<head>
    <title>Essensys Monitor</title>
    <style>
        body { font-family: sans-serif; padding: 20px; background: #f0f0f0; }
        h1 { color: #333; }
        .card { background: white; padding: 20px; border-radius: 8px; box-shadow: 0 2px 4px rgba(0,0,0,0.1); margin-bottom: 20px; }
        table { width: 100%; border-collapse: collapse; }
        th, td { text-align: left; padding: 12px; border-bottom: 1px solid #ddd; }
        th { background-color: #f8f9fa; }
        tr:hover { background-color: #f5f5f5; }
        .changed { animation: flash 1s; }
        @keyframes flash { 0% { background-color: #fff3cd; } 100% { background-color: transparent; } }
        .status-badge { padding: 4px 8px; border-radius: 4px; font-weight: bold; }
        .on { background-color: #d4edda; color: #155724; }
        .off { background-color: #f8d7da; color: #721c24; }
        .history-item { display: inline-block; width: 20px; height: 20px; margin-right: 2px; text-align: center; font-size: 10px; line-height: 20px; color: white; border-radius: 2px; }
        .hist-1 { background-color: #28a745; } /* Green */
        .hist-0 { background-color: #dc3545; } /* Red */
        .hist-other { background-color: #6c757d; } /* Grey */
    </style>
</head>
<body>
    <div class="card">
        <h1>Essensys Monitor</h1>
        <p>Status updates from client (polling every 1s) - Last 25 values</p>
        <table id="statusTable">
            <thead>
                <tr>
                    <th>Index (K)</th>
                    <th>Description</th>
                    <th>Current Value</th>
                    <th>History (Last 25)</th>
                    <th>Last Update</th>
                </tr>
            </thead>
            <tbody></tbody>
        </table>
    </div>

    <script>
        const descriptions = {
            613: "Lumière Escalier (ON)",
            607: "Lumière Escalier (OFF)",
            615: "Lumière SDB2 (ON) [Scenario1+23]",
            590: "Trigger Scenario",
            349: "Unknown 349",
            350: "Unknown 350",
            351: "Unknown 351",
            352: "Unknown 352",
            363: "Unknown 363",
            425: "Unknown 425",
            426: "Unknown 426",
            920: "Unknown 920"
        };

        const oldValues = {};

        function updateTable() {
            fetch('/api/view-status')
                .then(response => response.json())
                .then(data => {
                    const tbody = document.querySelector('#statusTable tbody');
                    
                    // Sort keys
                    const keys = Object.keys(data).map(Number).sort((a, b) => a - b);
                    
                    keys.forEach(k => {
                        const history = data[k]; // Array of strings
                        const currentVal = history[history.length - 1];
                        
                        let row = document.getElementById('row-' + k);
                        
                        if (!row) {
                            row = document.createElement('tr');
                            row.id = 'row-' + k;
                            row.innerHTML = '<td>' + k + '</td><td>' + (descriptions[k] || 'Unknown') + '</td><td class="val"></td><td class="hist"></td><td class="time"></td>';
                            tbody.appendChild(row);
                        }
                        
                        const valCell = row.querySelector('.val');
                        const histCell = row.querySelector('.hist');
                        const timeCell = row.querySelector('.time');
                        
                        // Update Value
                        if (valCell.textContent !== currentVal) {
                            valCell.textContent = currentVal;
                            timeCell.textContent = new Date().toLocaleTimeString();
                            row.classList.remove('changed');
                            void row.offsetWidth; // trigger reflow
                            row.classList.add('changed');
                        }

                        // Update History
                        histCell.innerHTML = '';
                        history.forEach(v => {
                            const span = document.createElement('span');
                            span.className = 'history-item';
                            span.textContent = v;
                            if (v === "1") span.classList.add('hist-1');
                            else if (v === "0") span.classList.add('hist-0');
                            else span.classList.add('hist-other');
                            histCell.appendChild(span);
                        });
                    });
                });
        }

        setInterval(updateTable, 1000);
        updateTable();
    </script>
</body>
</html>`

	response := fmt.Sprintf("HTTP/1.1 200 OK\r\n")
	response += "Content-Type: text/html; charset=UTF-8\r\n"
	response += "Connection: close\r\n"
	response += fmt.Sprintf("Content-Length: %d\r\n", len(html))
	response += "\r\n"
	response += html

	conn.Write([]byte(response))
}

func handleViewStatus(conn net.Conn) {
	store.RLock()
	defer store.RUnlock()
	
	jsonBytes, _ := json.Marshal(store.ExchangeTable)
	sendResponse(conn, 200, "OK", string(jsonBytes))
}

func sendResponse(conn net.Conn, statusCode int, statusText string, body string) {
	response := fmt.Sprintf("HTTP/1.1 %d %s\r\n", statusCode, statusText)
	response += "Content-Type: application/json ;charset=UTF-8\r\n"
	response += "Connection: close\r\n"
	response += fmt.Sprintf("Content-Length: %d\r\n", len(body))
	response += "\r\n"
	response += body

	conn.Write([]byte(response))
}

func handleServerInfos(conn net.Conn) {
	// Indices demandés par le serveur
	// 613: Lumière Escalier ON (identifié dans Index.cshtml)
	indices := []int{613, 607, 615, 590, 349, 350, 351, 352, 363, 425, 426, 920}
	
	resp := ServerInfosResponse{
		IsConnected: true,
		Infos:       indices,
		NewVersion:  "no",
	}
	
	jsonBytes, _ := json.Marshal(resp)
	sendResponse(conn, 200, "OK", string(jsonBytes))
}

func handleMyStatus(conn net.Conn, body []byte) {
	// Fix non-standard JSON from client (unquoted keys k and v)
	// Client sends: {k:123,v:"val"} instead of {"k":123,"v":"val"}
	bodyStr := string(body)
	bodyStr = strings.ReplaceAll(bodyStr, "{k:", "{\"k\":")
	bodyStr = strings.ReplaceAll(bodyStr, ",v:", ",\"v\":")

	var req MyStatusRequest
	if err := json.Unmarshal([]byte(bodyStr), &req); err != nil {
		fmt.Printf("[GO] JSON Error in MyStatus: %v\nBody: %s\n", err, bodyStr)
		sendResponse(conn, 400, "Bad Request", "")
		return
	}

	fmt.Printf("[GO] Status Update (Version: %s, Items: %d) from %s\n", req.Version, len(req.Ek), conn.RemoteAddr().String())
	
	for _, item := range req.Ek {
		store.UpdateValue(item.K, item.V)
	}

	sendResponse(conn, 201, "Created", "")
}

func handleMyActions(conn net.Conn) {
	actions := store.GetPendingActions()
	
	resp := MyActionsResponse{
		De67f:   nil,
		Actions: actions,
	}
	
	jsonBytes, _ := json.Marshal(resp)
	fmt.Printf("[GO] Sending Actions to %s: %s\n", conn.RemoteAddr().String(), string(jsonBytes))
	sendResponse(conn, 200, "OK", string(jsonBytes))
}

func handleDone(conn net.Conn, path string) {
	// /api/done/GUID
	parts := strings.Split(path, "/")
	if len(parts) >= 4 {
		guid := parts[3]
		if store.RemoveAction(guid) {
			fmt.Printf("[GO] Action acknowledged: %s from %s\n", guid, conn.RemoteAddr().String())
			sendResponse(conn, 201, "Created", "")
			return
		}
	}
	sendResponse(conn, 404, "Not Found", "")
}

func handleAdminInject(conn net.Conn, body []byte) {
	// Support both single object and array of objects
	var params []ActionParam
	
	// Try parsing as array first
	if err := json.Unmarshal(body, &params); err != nil {
		// If array fails, try single object
		var singleParam ActionParam
		if err2 := json.Unmarshal(body, &singleParam); err2 != nil {
			sendResponse(conn, 400, "Bad Request", "Invalid JSON: expected array or object")
			return
		}
		params = []ActionParam{singleParam}
	}
	
	// Logic to merge values (Bitwise OR) and prepare final list
	// Mimics VoletService.cs logic
	mergedValues := make(map[int]int)
	
	// 1. Initialize with "0" for the Volet/Light range (605-622) to be fully compliant with legacy server
	// VoletService.cs lines 31-48
	for i := 605; i <= 622; i++ {
		mergedValues[i] = 0
	}
	
	for _, p := range params {
		valInt := 0
		fmt.Sscanf(p.V, "%d", &valInt)
		
		if currentVal, exists := mergedValues[p.K]; exists {
			mergedValues[p.K] = currentVal | valInt
		} else {
			mergedValues[p.K] = valInt
		}
	}
	
	// Convert back to ActionParam list
	finalParams := make([]ActionParam, 0)
	for k, v := range mergedValues {
		finalParams = append(finalParams, ActionParam{
			K: k,
			V: fmt.Sprintf("%d", v),
		})
	}

	// Add Scenario=1 (Index 590) if not present, as VoletService always does this.
	// We check if 590 is already in the map.
	if _, ok := mergedValues[590]; !ok {
		finalParams = append(finalParams, ActionParam{K: 590, V: "1"})
	}
	
	action := Action{
		Guid:   generateUUID(),
		Params: finalParams,
	}
	
	store.AddAction(action)
	sendResponse(conn, 200, "OK", `{"status":"ok"}`)
}
