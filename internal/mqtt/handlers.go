package mqtt

import (
	"log"
	"strings"

	"github.com/essensys-hub/essensys-server-backend/internal/core"
	"github.com/essensys-hub/essensys-server-backend/pkg/protocol"
)

// EntityCommand contains information about how to convert MQTT commands to actions
type EntityCommand struct {
	Index    int    // The protocol index (k)
	OnValue  string // Value to send for ON/open/etc
	OffValue string // Value to send for OFF/close/etc
}

// CommandHandler handles MQTT command messages
type CommandHandler struct {
	actionService *core.ActionService
	entityMapping map[string]EntityCommand // Maps "entityType/entityID" to command info
}

// NewCommandHandler creates a new command handler
func NewCommandHandler(actionService *core.ActionService, entityMapping map[string]EntityCommand) *CommandHandler {
	return &CommandHandler{
		actionService: actionService,
		entityMapping: entityMapping,
	}
}

// HandleCommand processes an MQTT command message
func (h *CommandHandler) HandleCommand(topic string, payload []byte) {
	// Parse topic: essensys/{type}/{id}/set
	parts := strings.Split(topic, "/")
	if len(parts) != 4 || parts[0] != "essensys" || parts[3] != "set" {
		log.Printf("Invalid MQTT command topic: %s", topic)
		return
	}

	entityType := parts[1]
	entityID := parts[2]
	commandKey := entityType + "/" + entityID

	// Get entity command mapping
	entityCmd, exists := h.entityMapping[commandKey]
	if !exists {
		log.Printf("No mapping found for entity: %s", commandKey)
		return
	}

	// Convert payload to string
	payloadStr := strings.TrimSpace(string(payload))
	
	// Determine value based on entity type and payload
	var value string
	switch entityType {
	case "light":
		if payloadStr == "ON" {
			value = entityCmd.OnValue
		} else {
			value = entityCmd.OffValue
		}

	case "cover":
		if payloadStr == "OPEN" {
			value = entityCmd.OnValue
		} else if payloadStr == "CLOSE" {
			value = entityCmd.OffValue
		} else {
			// STOP command - may need special handling
			log.Printf("STOP command not yet implemented for cover")
			return
		}

	case "select":
		// For select, the payload is the mode value directly
		value = payloadStr

	default:
		log.Printf("Unknown entity type: %s", entityType)
		return
	}

	// Create action
	params := []protocol.ExchangeKV{
		{K: entityCmd.Index, V: value},
	}

	clientID := "default" // Use default client ID for MQTT commands
	guid, err := h.actionService.AddAction(clientID, params)
	if err != nil {
		log.Printf("Failed to add action from MQTT command: %v", err)
		return
	}

	log.Printf("MQTT command processed: %s -> index %d = %s (GUID: %s)", topic, entityCmd.Index, value, guid)
}

// SubscribeToCommands subscribes to all command topics
func (c *Client) SubscribeToCommands(handler *CommandHandler) error {
	// Subscribe to wildcard topic for all commands
	topic := "essensys/+/+/set"
	return c.Subscribe(topic, func(topic string, payload []byte) {
		handler.HandleCommand(topic, payload)
	})
}
