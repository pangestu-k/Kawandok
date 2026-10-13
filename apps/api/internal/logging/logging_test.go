package logging

import (
	"encoding/json"
	"testing"
)

func TestIsSensitive(t *testing.T) {
	tests := []struct {
		key      string
		expected bool
	}{
		{"password", true},
		{"PASSWORD", true},
		{"  Token ", true},
		{"api_key", true},
		{"Authorization", true},
		{"prompt", true},
		{"document_text", true},
		{"request_id", false},
		{"method", false},
		{"duration_ms", false},
		{"", false},
	}

	for _, tt := range tests {
		if got := IsSensitive(tt.key); got != tt.expected {
			t.Errorf("IsSensitive(%q) = %v, want %v", tt.key, got, tt.expected)
		}
	}
}

func TestScrub(t *testing.T) {
	fields := map[string]any{
		"request_id": "req-123",
		"method":     "GET",
		"password":   "hunter2",
		"nested": map[string]any{
			"token": "secret-token",
			"safe":  "value",
			"deep": map[string]any{
				"api_key": "key-value",
			},
		},
		"items": []any{
			map[string]any{"content": "secret text", "page": 3},
			"plain",
		},
	}

	got := Scrub(fields)

	if got["request_id"] != "req-123" {
		t.Errorf("request_id = %v, want req-123", got["request_id"])
	}
	if got["password"] != "[REDACTED]" {
		t.Errorf("password = %v, want [REDACTED]", got["password"])
	}

	nested, ok := got["nested"].(map[string]any)
	if !ok {
		t.Fatalf("nested missing or wrong type: %T", got["nested"])
	}
	if nested["token"] != "[REDACTED]" {
		t.Errorf("nested.token = %v, want [REDACTED]", nested["token"])
	}
	if nested["safe"] != "value" {
		t.Errorf("nested.safe = %v, want value", nested["safe"])
	}
	deep, ok := nested["deep"].(map[string]any)
	if !ok {
		t.Fatalf("nested.deep missing or wrong type: %T", nested["deep"])
	}
	if deep["api_key"] != "[REDACTED]" {
		t.Errorf("nested.deep.api_key = %v, want [REDACTED]", deep["api_key"])
	}

	items, ok := got["items"].([]any)
	if !ok {
		t.Fatalf("items missing or wrong type: %T", got["items"])
	}
	first, ok := items[0].(map[string]any)
	if !ok {
		t.Fatalf("items[0] wrong type: %T", items[0])
	}
	if first["content"] != "[REDACTED]" {
		t.Errorf("items[0].content = %v, want [REDACTED]", first["content"])
	}
	if first["page"] != 3 {
		t.Errorf("items[0].page = %v, want 3", first["page"])
	}
	if items[1] != "plain" {
		t.Errorf("items[1] = %v, want plain", items[1])
	}
}

func TestScrubDoesNotMutateInput(t *testing.T) {
	fields := map[string]any{
		"nested": map[string]any{"token": "secret"},
	}
	Scrub(fields)
	nested := fields["nested"].(map[string]any)
	if nested["token"] != "secret" {
		t.Fatalf("input map was mutated: %v", nested)
	}
}

func TestScrubOutputRemainsJSONSerializable(t *testing.T) {
	fields := map[string]any{"token": "x", "ok": []any{map[string]any{"secret": 1}}}
	out := Scrub(fields)
	if _, err := json.Marshal(out); err != nil {
		t.Fatalf("scrubbed output not JSON serializable: %v", err)
	}
}

func TestRedact(t *testing.T) {
	if Redact("anything") != "[REDACTED]" {
		t.Fatal("Redact must replace the value")
	}
}
