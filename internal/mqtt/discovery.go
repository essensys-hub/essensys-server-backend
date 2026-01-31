package mqtt

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/essensys-hub/essensys-server-backend/internal/mqtt/handlers"
)

// TableReferenceEntry represents an entry in table_reference.json
type TableReferenceEntry struct {
	Zone            string `json:"zone"`
	Piece           string `json:"piece"`
	Categorie       string `json:"categorie"`
	Keys            string `json:"keys"`
	Value           string `json:"value"`
	Attribute       string `json:"attribute"`
	Action          string `json:"action"`
	ShortDescription string `json:"shortDescription"`
	LongDescription  string `json:"longDescription"`
}

// TableReferenceData represents the structure of table_reference.json
type TableReferenceData struct {
	Entries []TableReferenceEntry `json:"entries"`
}

// DiscoveryConfig represents an MQTT Discovery configuration
type DiscoveryConfig struct {
	Name              string                 `json:"name"`
	UniqueID          string                 `json:"unique_id"`
	StateTopic        string                 `json:"state_topic"`
	CommandTopic      string                 `json:"command_topic"`
	AvailabilityTopic string                 `json:"availability_topic"`
	PayloadOn         string                 `json:"payload_on,omitempty"`
	PayloadOff        string                 `json:"payload_off,omitempty"`
	Device            map[string]interface{} `json:"device"`
	Options           []string               `json:"options,omitempty"` // For select entities
}

// PublishDiscoveryConfigs publishes MQTT Discovery configurations for all entities
func (c *Client) PublishDiscoveryConfigs() error {
	if !c.IsConnected() {
		return fmt.Errorf("MQTT client not connected")
	}

	// Try to find table_reference.json in common locations
	tableRefPath := c.findTableReferenceFile()
	if tableRefPath == "" {
		log.Println("Warning: table_reference.json not found, skipping MQTT Discovery")
		return nil
	}

	data, err := c.loadTableReference(tableRefPath)
	if err != nil {
		return fmt.Errorf("failed to load table_reference.json: %w", err)
	}

	// Build entity mappings for commands
	entityMapping := make(map[string]EntityCommand)

	// Process lights
	lights := c.groupLights(data.Entries)
	for id, light := range lights {
		config := c.createLightDiscoveryConfig(id, light)
		if err := c.publishDiscoveryConfig("light", id, config); err != nil {
			log.Printf("Failed to publish light discovery config for %s: %v", id, err)
		}

		// Add to command mapping
		if light.OnIndex > 0 {
			entityMapping["light/"+id] = EntityCommand{
				Index:    light.OnIndex,
				OnValue:  light.OnValue,
				OffValue: light.OffValue,
			}
		}
	}

	// Process covers
	covers := c.groupCovers(data.Entries)
	for id, cover := range covers {
		config := c.createCoverDiscoveryConfig(id, cover)
		if err := c.publishDiscoveryConfig("cover", id, config); err != nil {
			log.Printf("Failed to publish cover discovery config for %s: %v", id, err)
		}

		// Add to command mapping
		if cover.OpenIndex > 0 {
			entityMapping["cover/"+id] = EntityCommand{
				Index:    cover.OpenIndex,
				OnValue:  cover.OpenValue,
				OffValue: cover.CloseValue,
			}
		}
	}

	// Process select (heating)
	selects := c.groupSelects(data.Entries)
	for id, sel := range selects {
		config := c.createSelectDiscoveryConfig(id, sel)
		if err := c.publishDiscoveryConfig("select", id, config); err != nil {
			log.Printf("Failed to publish select discovery config for %s: %v", id, err)
		}

		// Add to command mapping for select
		// For select entities, we need to map the option label to the value
		// This is simplified - in practice, we'd need a more complex mapping
		if len(sel.Options) > 0 && sel.Index > 0 {
			entityMapping["select/"+id] = EntityCommand{
				Index: sel.Index,
				// OnValue/OffValue not used for select, value comes from payload
			}
		}
	}

	// Store entity mapping in client for later use
	c.mu.Lock()
	c.entityMapping = entityMapping
	c.mu.Unlock()

	log.Printf("Published MQTT Discovery configs: %d lights, %d covers, %d selects", len(lights), len(covers), len(selects))
	return nil
}

// GetEntityMapping returns the entity mapping for command handlers
func (c *Client) GetEntityMapping() map[string]EntityCommand {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.entityMapping
}

// LightEntity represents a grouped light entity
type LightEntity struct {
	Name      string
	OnIndex   int
	OffIndex  int
	OnValue   string
	OffValue  string
}

// CoverEntity represents a grouped cover entity
type CoverEntity struct {
	Name       string
	OpenIndex  int
	CloseIndex int
	OpenValue  string
	CloseValue string
}

// SelectEntity represents a grouped select entity
type SelectEntity struct {
	Name    string
	Index   int
	Options []SelectOption
}

// SelectOption represents an option for a select entity
type SelectOption struct {
	Index   int
	Value   string
	Label   string
}

func (c *Client) findTableReferenceFile() string {
	// Try common locations
	paths := []string{
		"table_reference.json",
		"../essensys-homeassitant/custom_components/essensys/table_reference.json",
		"/opt/essensys/homeassistant/config/custom_components/essensys/table_reference.json",
	}

	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	return ""
}

func (c *Client) loadTableReference(path string) (*TableReferenceData, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var refData TableReferenceData
	if err := json.Unmarshal(data, &refData); err != nil {
		return nil, err
	}

	return &refData, nil
}

func (c *Client) groupLights(entries []TableReferenceEntry) map[string]LightEntity {
	grouped := make(map[string]LightEntity)

	for _, entry := range entries {
		name := entry.ShortDescription
		if name == "" {
			continue
		}

		attr := strings.ToLower(entry.Attribute)
		keys, _ := parseInt(entry.Keys)

		if _, exists := grouped[name]; !exists {
			grouped[name] = LightEntity{
				Name: entry.LongDescription,
			}
		}

		light := grouped[name]
		if strings.Contains(attr, "on") || strings.Contains(attr, "allumé") || strings.Contains(attr, "open") {
			light.OnIndex = keys
			light.OnValue = entry.Value
		} else if strings.Contains(attr, "off") || strings.Contains(attr, "eteint") || strings.Contains(attr, "éteint") || strings.Contains(attr, "close") {
			light.OffIndex = keys
			light.OffValue = entry.Value
		}
		grouped[name] = light
	}

	// Filter to only include lights with both ON and OFF
	result := make(map[string]LightEntity)
	for name, light := range grouped {
		if light.OnIndex > 0 && light.OffIndex > 0 {
			// Use sanitized name as ID
			id := sanitizeID(name)
			result[id] = light
		}
	}

	return result
}

func (c *Client) groupCovers(entries []TableReferenceEntry) map[string]CoverEntity {
	grouped := make(map[string]CoverEntity)

	for _, entry := range entries {
		cat := strings.ToLower(entry.Categorie)
		if !strings.Contains(cat, "volets") && !strings.Contains(cat, "store") {
			continue
		}

		name := entry.ShortDescription
		if name == "" {
			continue
		}

		attr := strings.ToLower(entry.Attribute)
		keys, _ := parseInt(entry.Keys)

		if _, exists := grouped[name]; !exists {
			grouped[name] = CoverEntity{
				Name: entry.LongDescription,
			}
		}

		cover := grouped[name]
		if strings.Contains(attr, "open") {
			cover.OpenIndex = keys
			cover.OpenValue = entry.Value
		} else if strings.Contains(attr, "close") {
			cover.CloseIndex = keys
			cover.CloseValue = entry.Value
		}
		grouped[name] = cover
	}

	// Filter to only include covers with both OPEN and CLOSE
	result := make(map[string]CoverEntity)
	for name, cover := range grouped {
		if cover.OpenIndex > 0 && cover.CloseIndex > 0 {
			id := sanitizeID(name)
			result[id] = cover
		}
	}

	return result
}

func (c *Client) groupSelects(entries []TableReferenceEntry) map[string]SelectEntity {
	grouped := make(map[string]SelectEntity)

	for _, entry := range entries {
		cat := strings.ToLower(entry.Categorie)
		if !strings.Contains(cat, "chauffage") {
			continue
		}

		name := entry.ShortDescription
		if name == "" {
			continue
		}

		keys, _ := parseInt(entry.Keys)

		if _, exists := grouped[name]; !exists {
			grouped[name] = SelectEntity{
				Name:  entry.LongDescription,
				Index: keys,
			}
		}

		sel := grouped[name]
		sel.Options = append(sel.Options, SelectOption{
			Index: keys,
			Value: entry.Value,
			Label: entry.Attribute,
		})
		grouped[name] = sel
	}

	result := make(map[string]SelectEntity)
	for name, sel := range grouped {
		if len(sel.Options) > 0 {
			id := sanitizeID(name)
			result[id] = sel
		}
	}

	return result
}

func (c *Client) createLightDiscoveryConfig(id string, light LightEntity) DiscoveryConfig {
	return DiscoveryConfig{
		Name:              light.Name,
		UniqueID:          fmt.Sprintf("essensys_light_%s", id),
		StateTopic:        fmt.Sprintf("essensys/light/%s/state", id),
		CommandTopic:      fmt.Sprintf("essensys/light/%s/set", id),
		AvailabilityTopic: fmt.Sprintf("essensys/light/%s/available", id),
		PayloadOn:         "ON",
		PayloadOff:        "OFF",
		Device: map[string]interface{}{
			"identifiers":  []string{"essensys"},
			"name":         "Essensys",
			"manufacturer": "Essensys",
		},
	}
}

func (c *Client) createCoverDiscoveryConfig(id string, cover CoverEntity) DiscoveryConfig {
	return DiscoveryConfig{
		Name:              cover.Name,
		UniqueID:          fmt.Sprintf("essensys_cover_%s", id),
		StateTopic:        fmt.Sprintf("essensys/cover/%s/state", id),
		CommandTopic:      fmt.Sprintf("essensys/cover/%s/set", id),
		AvailabilityTopic: fmt.Sprintf("essensys/cover/%s/available", id),
		Device: map[string]interface{}{
			"identifiers":  []string{"essensys"},
			"name":         "Essensys",
			"manufacturer": "Essensys",
		},
	}
}

func (c *Client) createSelectDiscoveryConfig(id string, sel SelectEntity) DiscoveryConfig {
	options := make([]string, len(sel.Options))
	for i, opt := range sel.Options {
		options[i] = opt.Label
	}

	return DiscoveryConfig{
		Name:              sel.Name,
		UniqueID:          fmt.Sprintf("essensys_select_%s", id),
		StateTopic:        fmt.Sprintf("essensys/select/%s/state", id),
		CommandTopic:      fmt.Sprintf("essensys/select/%s/set", id),
		AvailabilityTopic: fmt.Sprintf("essensys/select/%s/available", id),
		Options:           options,
		Device: map[string]interface{}{
			"identifiers":  []string{"essensys"},
			"name":         "Essensys",
			"manufacturer": "Essensys",
		},
	}
}

func (c *Client) publishDiscoveryConfig(entityType, entityID string, config DiscoveryConfig) error {
	topic := fmt.Sprintf("homeassistant/%s/essensys_%s/config", entityType, entityID)

	configJSON, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal discovery config: %w", err)
	}

	return c.Publish(topic, string(configJSON), true)
}

// Helper functions
func sanitizeID(name string) string {
	// Convert to lowercase and replace spaces/special chars with underscores
	id := strings.ToLower(name)
	id = strings.ReplaceAll(id, " ", "_")
	id = strings.ReplaceAll(id, "é", "e")
	id = strings.ReplaceAll(id, "è", "e")
	id = strings.ReplaceAll(id, "ê", "e")
	id = strings.ReplaceAll(id, "à", "a")
	id = strings.ReplaceAll(id, "ç", "c")
	return id
}

func parseInt(s string) (int, error) {
	var result int
	_, err := fmt.Sscanf(s, "%d", &result)
	return result, err
}
