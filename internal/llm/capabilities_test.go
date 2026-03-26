package llm

import (
	"encoding/json"
	"testing"
)

func TestAdaptToolSchema_StripsUnsupportedKeywordsRecursively(t *testing.T) {
	raw := json.RawMessage(`{
		"type": "object",
		"properties": {
			"role": {
				"type": "string",
				"enum": ["developer", "architect"]
			},
			"payload": {
				"type": "object",
				"properties": {
					"value": {
						"type": "string"
					}
				},
				"not": {
					"type": "null"
				}
			}
		},
		"anyOf": [
			{"required": ["role"]},
			{"required": ["payload"]}
		]
	}`)

	adapted := AdaptToolSchema(OpenAICompatibleCapabilities().ToolSchema, raw)

	var decoded map[string]any
	if err := json.Unmarshal(adapted, &decoded); err != nil {
		t.Fatalf("unmarshal adapted schema: %v", err)
	}

	if _, exists := decoded["anyOf"]; exists {
		t.Fatalf("expected root anyOf to be stripped")
	}

	properties, ok := decoded["properties"].(map[string]any)
	if !ok {
		t.Fatalf("expected properties object")
	}

	role, ok := properties["role"].(map[string]any)
	if !ok {
		t.Fatalf("expected role property")
	}
	if _, exists := role["enum"]; exists {
		t.Fatalf("expected nested enum to be stripped")
	}

	payload, ok := properties["payload"].(map[string]any)
	if !ok {
		t.Fatalf("expected payload property")
	}
	if _, exists := payload["not"]; exists {
		t.Fatalf("expected nested not to be stripped")
	}
}

func TestAdaptToolSchema_DefaultProfileLeavesSchemaUntouched(t *testing.T) {
	raw := json.RawMessage(`{"type":"object","anyOf":[{"required":["task"]}]}`)
	adapted := AdaptToolSchema(DefaultCapabilities().ToolSchema, raw)
	if string(adapted) != string(raw) {
		t.Fatalf("expected schema to remain unchanged, got %s", string(adapted))
	}
}
