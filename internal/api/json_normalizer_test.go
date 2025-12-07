package api

import (
	"encoding/json"
	"testing"
)

func TestNormalizeJSON_BasicMalformedJSON(t *testing.T) {
	input := []byte(`{k:1,v:"0"}`)
	expected := `{"k":1,"v":"0"}`

	result, err := NormalizeJSON(input)
	if err != nil {
		t.Fatalf("NormalizeJSON failed: %v", err)
	}

	if string(result) != expected {
		t.Errorf("Expected %s, got %s", expected, string(result))
	}

	// Verify it's valid JSON
	var parsed interface{}
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Errorf("Result is not valid JSON: %v", err)
	}
}

func TestNormalizeJSON_PreservesQuotedStrings(t *testing.T) {
	input := []byte(`{k:1,v:"hello world"}`)
	expected := `{"k":1,"v":"hello world"}`

	result, err := NormalizeJSON(input)
	if err != nil {
		t.Fatalf("NormalizeJSON failed: %v", err)
	}

	if string(result) != expected {
		t.Errorf("Expected %s, got %s", expected, string(result))
	}
}

func TestNormalizeJSON_PreservesNumericValues(t *testing.T) {
	input := []byte(`{k:123,v:"456"}`)
	expected := `{"k":123,"v":"456"}`

	result, err := NormalizeJSON(input)
	if err != nil {
		t.Fatalf("NormalizeJSON failed: %v", err)
	}

	if string(result) != expected {
		t.Errorf("Expected %s, got %s", expected, string(result))
	}

	// Verify numeric value is preserved as number
	var parsed map[string]interface{}
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("Failed to parse result: %v", err)
	}

	if k, ok := parsed["k"].(float64); !ok || k != 123 {
		t.Errorf("Expected k to be numeric 123, got %v", parsed["k"])
	}
}

func TestNormalizeJSON_HandlesArrays(t *testing.T) {
	input := []byte(`[{k:1,v:"0"},{k:2,v:"1"}]`)
	expected := `[{"k":1,"v":"0"},{"k":2,"v":"1"}]`

	result, err := NormalizeJSON(input)
	if err != nil {
		t.Fatalf("NormalizeJSON failed: %v", err)
	}

	if string(result) != expected {
		t.Errorf("Expected %s, got %s", expected, string(result))
	}

	// Verify it's valid JSON
	var parsed interface{}
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Errorf("Result is not valid JSON: %v", err)
	}
}

func TestNormalizeJSON_HandlesNestedStructures(t *testing.T) {
	input := []byte(`{"ek":[{k:1,v:"0"},{k:2,v:"1"}]}`)

	result, err := NormalizeJSON(input)
	if err != nil {
		t.Fatalf("NormalizeJSON failed: %v", err)
	}

	// Verify it's valid JSON
	var parsed interface{}
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Errorf("Result is not valid JSON: %v", err)
	}
}

func TestNormalizeJSON_ReturnsErrorForInvalidJSON(t *testing.T) {
	input := []byte(`{invalid json that cannot be fixed}`)

	_, err := NormalizeJSON(input)
	if err == nil {
		t.Error("Expected error for invalid JSON, got nil")
	}
}

func TestNormalizeJSON_HandlesEmptyInput(t *testing.T) {
	input := []byte(``)

	_, err := NormalizeJSON(input)
	if err == nil {
		t.Error("Expected error for empty input, got nil")
	}
}

func TestNormalizeJSON_HandlesAlreadyValidJSON(t *testing.T) {
	input := []byte(`{"k":1,"v":"0"}`)

	result, err := NormalizeJSON(input)
	if err != nil {
		t.Fatalf("NormalizeJSON failed: %v", err)
	}

	// Should return valid JSON unchanged
	var parsed interface{}
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Errorf("Result is not valid JSON: %v", err)
	}
}
