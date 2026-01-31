package mqtt

import (
	"fmt"
	"log"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/essensys-hub/essensys-server-backend/internal/config"
)

// MessageHandler is a function type for handling MQTT messages
type MessageHandler func(topic string, payload []byte)

// Client wraps the MQTT client with reconnection logic
type Client struct {
	config        config.MQTTConfig
	mqttClient    mqtt.Client
	connected     bool
	mu            sync.RWMutex
	handlers      map[string]MessageHandler
	onConnect     func()
	entityMapping map[string]EntityCommand // Will be set by discovery
}

// NewClient creates a new MQTT client instance
func NewClient(cfg config.MQTTConfig) *Client {
	return &Client{
		config:   cfg,
		handlers: make(map[string]MessageHandler),
	}
}

// Connect establishes connection to the MQTT broker
func (c *Client) Connect() error {
	if !c.config.Enabled {
		log.Println("MQTT is disabled, skipping connection")
		return nil
	}

	opts := mqtt.NewClientOptions()
	opts.AddBroker(c.config.Broker)
	opts.SetClientID(c.config.ClientID)
	opts.SetUsername(c.config.Username)
	opts.SetPassword(c.config.Password)
	opts.SetAutoReconnect(true)
	opts.SetConnectRetry(true)
	opts.SetConnectRetryInterval(5 * time.Second)
	opts.SetKeepAlive(60 * time.Second)
	opts.SetPingTimeout(10 * time.Second)
	opts.SetCleanSession(true)

	// Set connection handler
	opts.OnConnect = func(client mqtt.Client) {
		log.Println("MQTT client connected")
		c.mu.Lock()
		c.connected = true
		c.mu.Unlock()

		// Resubscribe to all topics
		for topic, handler := range c.handlers {
			if token := client.Subscribe(topic, 1, func(client mqtt.Client, msg mqtt.Message) {
				handler(msg.Topic(), msg.Payload())
			}); token.Wait() && token.Error() != nil {
				log.Printf("Failed to resubscribe to %s: %v", topic, token.Error())
			}
		}

		// Call custom onConnect handler if set
		if c.onConnect != nil {
			c.onConnect()
		}
	}

	// Set connection lost handler
	opts.OnConnectionLost = func(client mqtt.Client, err error) {
		log.Printf("MQTT connection lost: %v", err)
		c.mu.Lock()
		c.connected = false
		c.mu.Unlock()
	}

	c.mqttClient = mqtt.NewClient(opts)

	if token := c.mqttClient.Connect(); token.Wait() && token.Error() != nil {
		return fmt.Errorf("failed to connect to MQTT broker: %w", token.Error())
	}

	return nil
}

// Disconnect closes the MQTT connection
func (c *Client) Disconnect() {
	if c.mqttClient != nil {
		c.mqttClient.Disconnect(250)
		c.mu.Lock()
		c.connected = false
		c.mu.Unlock()
		log.Println("MQTT client disconnected")
	}
}

// IsConnected returns whether the client is currently connected
func (c *Client) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected && c.mqttClient != nil && c.mqttClient.IsConnected()
}

// Publish publishes a message to a topic
func (c *Client) Publish(topic string, payload string, retain bool) error {
	if !c.IsConnected() {
		return fmt.Errorf("MQTT client not connected")
	}

	token := c.mqttClient.Publish(topic, 1, retain, payload)
	if token.Wait() && token.Error() != nil {
		return fmt.Errorf("failed to publish to %s: %w", topic, token.Error())
	}

	return nil
}

// Subscribe subscribes to a topic with a message handler
func (c *Client) Subscribe(topic string, handler MessageHandler) error {
	if !c.IsConnected() {
		return fmt.Errorf("MQTT client not connected")
	}

	// Store handler for reconnection
	c.mu.Lock()
	c.handlers[topic] = handler
	c.mu.Unlock()

	// Subscribe to topic
	token := c.mqttClient.Subscribe(topic, 1, func(client mqtt.Client, msg mqtt.Message) {
		handler(msg.Topic(), msg.Payload())
	})

	if token.Wait() && token.Error() != nil {
		return fmt.Errorf("failed to subscribe to %s: %w", topic, token.Error())
	}

	log.Printf("Subscribed to MQTT topic: %s", topic)
	return nil
}

// SetOnConnect sets a callback function to be called when connected
func (c *Client) SetOnConnect(fn func()) {
	c.onConnect = fn
}
