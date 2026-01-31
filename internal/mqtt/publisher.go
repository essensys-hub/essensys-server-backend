package mqtt

import (
	"log"
	"strings"
)

// PublishState publishes the state of an entity to MQTT
func (c *Client) PublishState(entityType, entityID, state string) error {
	if !c.IsConnected() {
		return nil // Silently fail if not connected
	}

	topic := "essensys/" + entityType + "/" + entityID + "/state"
	if err := c.Publish(topic, state, true); err != nil {
		log.Printf("Failed to publish state to %s: %v", topic, err)
		return err
	}

	// Also publish availability
	availabilityTopic := "essensys/" + entityType + "/" + entityID + "/available"
	if err := c.Publish(availabilityTopic, "online", true); err != nil {
		log.Printf("Failed to publish availability to %s: %v", availabilityTopic, err)
		return err
	}

	return nil
}

// PublishActionState publishes state based on action parameters
// This converts ExchangeKV parameters to entity states
// Implements core.MQTTPublisher interface
func (c *Client) PublishActionState(params []struct {
	K int
	V string
}, entityMapping map[int]struct {
	EntityType string
	EntityID   string
}) {
	if !c.IsConnected() {
		return
	}

	for _, param := range params {
		entityInfo, exists := entityMapping[param.K]
		if !exists {
			continue
		}

		// Convert value to state string
		state := c.convertValueToState(param.V, entityInfo.EntityType)
		if state != "" {
			c.PublishState(entityInfo.EntityType, entityInfo.EntityID, state)
		}
	}
}


// convertValueToState converts a protocol value to MQTT state string
func (c *Client) convertValueToState(value, entityType string) string {
	value = strings.TrimSpace(value)

	switch entityType {
	case "light":
		// For lights, "1" or non-zero means ON, "0" means OFF
		if value == "1" || (value != "0" && value != "") {
			return "ON"
		}
		return "OFF"

	case "cover":
		// For covers, we need to check the value
		// This is simplified - actual implementation may need more logic
		if value == "1" {
			return "open"
		} else if value == "0" {
			return "closed"
		}
		return ""

	case "select":
		// For select entities, return the value as-is (it's the mode name)
		return value

	default:
		return ""
	}
}
