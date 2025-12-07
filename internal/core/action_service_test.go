package core

import (
	"testing"

	"github.com/essensys-hub/essensys-server-backend/internal/data"
	"github.com/essensys-hub/essensys-server-backend/pkg/protocol"
)

func TestGenerateCompleteBlock_WithLightIndex(t *testing.T) {
	store := data.NewMemoryStore()
	service := NewActionService(store)

	// Test with a single light index (613 = bedroom 3 light)
	input := []protocol.ExchangeKV{
		{K: 613, V: "64"},
	}

	result := service.GenerateCompleteBlock(input)

	// Should have 19 parameters: 590 + (605-622)
	expectedCount := 19
	if len(result) != expectedCount {
		t.Errorf("Expected %d parameters, got %d", expectedCount, len(result))
	}

	// Verify index 590 is present with value "1"
	found590 := false
	for _, param := range result {
		if param.K == 590 {
			found590 = true
			if param.V != "1" {
				t.Errorf("Expected index 590 to have value '1', got '%s'", param.V)
			}
			break
		}
	}
	if !found590 {
		t.Error("Index 590 (scenario trigger) not found in result")
	}

	// Verify all indices 605-622 are present
	indexMap := make(map[int]string)
	for _, param := range result {
		indexMap[param.K] = param.V
	}

	for i := 605; i <= 622; i++ {
		if _, exists := indexMap[i]; !exists {
			t.Errorf("Index %d is missing from complete block", i)
		}
	}

	// Verify explicit value is preserved
	if indexMap[613] != "64" {
		t.Errorf("Expected index 613 to have value '64', got '%s'", indexMap[613])
	}

	// Verify default values for non-explicit indices
	if indexMap[605] != "0" {
		t.Errorf("Expected index 605 to have default value '0', got '%s'", indexMap[605])
	}

	// Verify parameters are sorted by ascending index
	for i := 1; i < len(result); i++ {
		if result[i-1].K >= result[i].K {
			t.Errorf("Parameters not sorted: index %d comes before %d", result[i-1].K, result[i].K)
		}
	}
}

func TestGenerateCompleteBlock_WithoutLightIndex(t *testing.T) {
	store := data.NewMemoryStore()
	service := NewActionService(store)

	// Test with indices outside the light/shutter range
	input := []protocol.ExchangeKV{
		{K: 100, V: "test"},
		{K: 200, V: "value"},
	}

	result := service.GenerateCompleteBlock(input)

	// Should return input as-is since no light/shutter indices
	if len(result) != len(input) {
		t.Errorf("Expected %d parameters, got %d", len(input), len(result))
	}
}

func TestGenerateCompleteBlock_PreservesExplicitValues(t *testing.T) {
	store := data.NewMemoryStore()
	service := NewActionService(store)

	// Test with multiple explicit values in the light range
	input := []protocol.ExchangeKV{
		{K: 605, V: "32"},
		{K: 613, V: "64"},
		{K: 622, V: "128"},
	}

	result := service.GenerateCompleteBlock(input)

	// Build map for easy lookup
	indexMap := make(map[int]string)
	for _, param := range result {
		indexMap[param.K] = param.V
	}

	// Verify all explicit values are preserved
	if indexMap[605] != "32" {
		t.Errorf("Expected index 605 to have value '32', got '%s'", indexMap[605])
	}
	if indexMap[613] != "64" {
		t.Errorf("Expected index 613 to have value '64', got '%s'", indexMap[613])
	}
	if indexMap[622] != "128" {
		t.Errorf("Expected index 622 to have value '128', got '%s'", indexMap[622])
	}

	// Verify other indices have default value "0"
	if indexMap[606] != "0" {
		t.Errorf("Expected index 606 to have default value '0', got '%s'", indexMap[606])
	}
}

func TestGenerateCompleteBlock_WithExplicitScenarioIndex(t *testing.T) {
	store := data.NewMemoryStore()
	service := NewActionService(store)

	// Test with explicit scenario index value
	input := []protocol.ExchangeKV{
		{K: 590, V: "5"}, // Explicit scenario value
		{K: 613, V: "64"},
	}

	result := service.GenerateCompleteBlock(input)

	// Build map for easy lookup
	indexMap := make(map[int]string)
	for _, param := range result {
		indexMap[param.K] = param.V
	}

	// Verify explicit scenario value is preserved
	if indexMap[590] != "5" {
		t.Errorf("Expected index 590 to preserve explicit value '5', got '%s'", indexMap[590])
	}
}

func TestGenerateCompleteBlock_ParameterOrdering(t *testing.T) {
	store := data.NewMemoryStore()
	service := NewActionService(store)

	// Test with unordered input
	input := []protocol.ExchangeKV{
		{K: 622, V: "1"},
		{K: 605, V: "2"},
		{K: 613, V: "3"},
	}

	result := service.GenerateCompleteBlock(input)

	// Verify parameters are sorted by ascending index
	for i := 1; i < len(result); i++ {
		if result[i-1].K >= result[i].K {
			t.Errorf("Parameters not sorted: index %d (value %s) comes before index %d (value %s)",
				result[i-1].K, result[i-1].V, result[i].K, result[i].V)
		}
	}

	// Verify first parameter is index 590 (lowest)
	if result[0].K != 590 {
		t.Errorf("Expected first parameter to be index 590, got %d", result[0].K)
	}
}

func TestBitwiseFusion_TwoNumericValues(t *testing.T) {
	store := data.NewMemoryStore()
	service := NewActionService(store)

	// Test bitwise OR: 64 | 128 = 192
	result := service.BitwiseFusion(613, "64", "128")
	if result != "192" {
		t.Errorf("Expected '192', got '%s'", result)
	}

	// Test bitwise OR: 1 | 2 = 3
	result = service.BitwiseFusion(605, "1", "2")
	if result != "3" {
		t.Errorf("Expected '3', got '%s'", result)
	}

	// Test bitwise OR: 15 | 240 = 255
	result = service.BitwiseFusion(610, "15", "240")
	if result != "255" {
		t.Errorf("Expected '255', got '%s'", result)
	}
}

func TestBitwiseFusion_ScenarioIndexException(t *testing.T) {
	store := data.NewMemoryStore()
	service := NewActionService(store)

	// Index 590 (Scenario) should NOT be fused - always use new value
	result := service.BitwiseFusion(590, "1", "5")
	if result != "5" {
		t.Errorf("Expected '5' (no fusion for index 590), got '%s'", result)
	}

	// Even with numeric values, index 590 should not be fused
	result = service.BitwiseFusion(590, "64", "128")
	if result != "128" {
		t.Errorf("Expected '128' (no fusion for index 590), got '%s'", result)
	}
}

func TestBitwiseFusion_NonNumericFallback(t *testing.T) {
	store := data.NewMemoryStore()
	service := NewActionService(store)

	// Non-numeric existing value - should use new value
	result := service.BitwiseFusion(613, "abc", "128")
	if result != "128" {
		t.Errorf("Expected '128' (fallback to new), got '%s'", result)
	}

	// Non-numeric new value - should use new value
	result = service.BitwiseFusion(613, "64", "xyz")
	if result != "xyz" {
		t.Errorf("Expected 'xyz' (fallback to new), got '%s'", result)
	}

	// Both non-numeric - should use new value
	result = service.BitwiseFusion(613, "abc", "xyz")
	if result != "xyz" {
		t.Errorf("Expected 'xyz' (fallback to new), got '%s'", result)
	}
}

func TestBitwiseFusion_SingleAction(t *testing.T) {
	store := data.NewMemoryStore()
	service := NewActionService(store)

	// Single action (no existing value) - should use new value
	// This is simulated by passing empty string as existing
	result := service.BitwiseFusion(613, "", "64")
	if result != "64" {
		t.Errorf("Expected '64', got '%s'", result)
	}

	// Single action with "0" as existing (default) - should apply OR
	result = service.BitwiseFusion(613, "0", "64")
	if result != "64" {
		t.Errorf("Expected '64' (0 | 64 = 64), got '%s'", result)
	}
}

func TestBitwiseFusion_Commutativity(t *testing.T) {
	store := data.NewMemoryStore()
	service := NewActionService(store)

	// Bitwise OR should be commutative: A | B = B | A
	result1 := service.BitwiseFusion(613, "64", "128")
	result2 := service.BitwiseFusion(613, "128", "64")

	if result1 != result2 {
		t.Errorf("Bitwise OR not commutative: %s != %s", result1, result2)
	}
}

func TestBitwiseFusion_ZeroValue(t *testing.T) {
	store := data.NewMemoryStore()
	service := NewActionService(store)

	// OR with 0 should return the other value
	result := service.BitwiseFusion(613, "0", "64")
	if result != "64" {
		t.Errorf("Expected '64' (0 | 64 = 64), got '%s'", result)
	}

	result = service.BitwiseFusion(613, "128", "0")
	if result != "128" {
		t.Errorf("Expected '128' (128 | 0 = 128), got '%s'", result)
	}
}
