package protocol

import (
	"bytes"
	"encoding/json"
)

// ServerInfoResponse - Response for GET /api/serverinfos
type ServerInfoResponse struct {
	IsConnected bool   `json:"isconnected"`
	Infos       []int  `json:"infos"`
	NewVersion  string `json:"newversion"`
}

// StatusRequest - Request body for POST /api/mystatus
type StatusRequest struct {
	Version string       `json:"version"`
	EK      []ExchangeKV `json:"ek"`
}

// ExchangeKV - Key-value pair in exchange table
type ExchangeKV struct {
	K int    `json:"k"` // Index
	V string `json:"v"` // Value (always string)
}

// ActionsResponse - Response for GET /api/myactions
type ActionsResponse struct {
	De67f   *AlarmCommand `json:"_de67f"` // Must be first field
	Actions []Action      `json:"actions"`
}

// MarshalJSON implements custom JSON marshaling to ensure field ordering
// _de67f must appear before actions in the JSON output
func (ar ActionsResponse) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteString("{")
	
	// Always write _de67f first
	buf.WriteString(`"_de67f":`)
	if ar.De67f == nil {
		buf.WriteString("null")
	} else {
		de67fJSON, err := json.Marshal(ar.De67f)
		if err != nil {
			return nil, err
		}
		buf.Write(de67fJSON)
	}
	
	// Then write actions
	buf.WriteString(`,"actions":`)
	actionsJSON, err := json.Marshal(ar.Actions)
	if err != nil {
		return nil, err
	}
	buf.Write(actionsJSON)
	
	buf.WriteString("}")
	return buf.Bytes(), nil
}

// Action - Single action to execute
type Action struct {
	GUID   string       `json:"guid"`
	Params []ExchangeKV `json:"params"`
}

// AlarmCommand - Encrypted alarm command (optional)
type AlarmCommand struct {
	GUID string `json:"guid"`
	OBL  string `json:"obl"` // AES encrypted data
}
