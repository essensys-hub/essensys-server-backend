package api

import (
	"encoding/json"
	"regexp"
)

// NormalizeJSON converts malformed JSON to valid JSON
// The legacy C client sends JSON with unquoted keys: {k:123,v:"val"}
// We need to convert it to valid JSON: {"k":123,"v":"val"}
// This matches the exact behavior of the ASP.NET server
func NormalizeJSON(input []byte) ([]byte, error) {
	if len(input) == 0 {
		return nil, json.Unmarshal(input, new(interface{}))
	}

	// Convert to string for processing
	normalized := string(input)

	// Fix unquoted keys - same approach as server.sample.go
	// Pattern 1: {k: -> {"k":
	normalized = regexp.MustCompile(`\{k:`).ReplaceAllString(normalized, `{"k":`)
	
	// Pattern 2: ,v: -> ,"v":
	normalized = regexp.MustCompile(`,v:`).ReplaceAllString(normalized, `,"v":`)

	// Also handle nested objects and arrays
	// Pattern 3: [k: -> ["k": (for arrays)
	normalized = regexp.MustCompile(`\[k:`).ReplaceAllString(normalized, `[{"k":`)
	
	// Pattern 4: Handle version field if unquoted
	normalized = regexp.MustCompile(`\{version:`).ReplaceAllString(normalized, `{"version":`)
	normalized = regexp.MustCompile(`,version:`).ReplaceAllString(normalized, `,"version":`)
	
	// Pattern 5: Handle ek field if unquoted
	normalized = regexp.MustCompile(`,ek:`).ReplaceAllString(normalized, `,"ek":`)

	// Validate that the normalized JSON is valid
	var test interface{}
	if err := json.Unmarshal([]byte(normalized), &test); err != nil {
		return nil, err
	}

	return []byte(normalized), nil
}
