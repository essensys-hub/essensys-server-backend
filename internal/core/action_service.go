package core

import (
	"crypto/rand"
	"fmt"
	"sort"
	"strconv"

	"github.com/essensys-hub/essensys-server-backend/internal/data"
	"github.com/essensys-hub/essensys-server-backend/pkg/protocol"
)

// generateGUID generates a unique identifier for actions
// Format: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
func generateGUID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

// ActionService handles action processing logic
type ActionService struct {
	store data.Store
}

// NewActionService creates a new ActionService instance
func NewActionService(store data.Store) *ActionService {
	return &ActionService{
		store: store,
	}
}

// AddAction adds an action to the queue with proper processing
// It applies complete block generation and bitwise fusion as needed
func (s *ActionService) AddAction(clientID string, params []protocol.ExchangeKV) (string, error) {
	// Generate complete block if needed (for light/shutter indices 605-622)
	processedParams := s.GenerateCompleteBlock(params)

	// Create action with processed parameters
	action := protocol.Action{
		GUID:   generateGUID(),
		Params: processedParams,
	}

	// Enqueue the action
	s.store.EnqueueAction(clientID, action)

	return action.GUID, nil
}

// ProcessAction applies bitwise fusion and generates complete blocks
func (s *ActionService) ProcessAction(params []protocol.ExchangeKV) []protocol.ExchangeKV {
	// TODO: Implement action processing
	return params
}

// BitwiseFusion merges multiple values for the same index using OR
// Exception: Index 590 (Scenario) is never fused
// Fallback: Non-numeric values use the most recent value
func (s *ActionService) BitwiseFusion(index int, existing, new string) string {
	// Exception: Index 590 (Scenario) is never fused - always use new value
	if index == protocol.IndexScenario {
		return new
	}

	// Try to parse both values as integers
	existingInt, existingErr := strconv.Atoi(existing)
	newInt, newErr := strconv.Atoi(new)

	// If both are numeric, apply bitwise OR
	if existingErr == nil && newErr == nil {
		result := existingInt | newInt
		return strconv.Itoa(result)
	}

	// Fallback: If either value is non-numeric, use the most recent value
	return new
}

// GenerateCompleteBlock ensures all indices 605-622 are present
// CRITICAL: This function MUST generate ALL indices from 605 to 622
// The BP_MQX_ETH client will IGNORE the action if ANY index from 605-622 is missing!
func (s *ActionService) GenerateCompleteBlock(params []protocol.ExchangeKV) []protocol.ExchangeKV {
	// Check if any parameter is in the light/shutter range (605-622)
	hasLightShutterIndex := false
	for _, param := range params {
		if param.K >= protocol.IndexLightStart && param.K <= protocol.IndexLightEnd {
			hasLightShutterIndex = true
			break
		}
	}

	// If no light/shutter indices, return params as-is
	if !hasLightShutterIndex {
		return params
	}

	// Create a map to store explicit values
	explicitValues := make(map[int]string)
	for _, param := range params {
		explicitValues[param.K] = param.V
	}

	// Build the complete block
	result := make([]protocol.ExchangeKV, 0)

	// Add index 590 (scenario trigger) with value "1" if not already present
	if _, exists := explicitValues[protocol.IndexScenario]; !exists {
		explicitValues[protocol.IndexScenario] = "1"
	}

	// Ensure all indices from 605 to 622 are present
	for i := protocol.IndexLightStart; i <= protocol.IndexLightEnd; i++ {
		if _, exists := explicitValues[i]; !exists {
			explicitValues[i] = "0"
		}
	}

	// Convert map to slice
	for k, v := range explicitValues {
		result = append(result, protocol.ExchangeKV{
			K: k,
			V: v,
		})
	}

	// Sort by ascending index number
	sort.Slice(result, func(i, j int) bool {
		return result[i].K < result[j].K
	})

	return result
}
