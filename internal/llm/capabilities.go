package llm

import (
	"encoding/json"

	"github.com/perfree/funcode/pkg/types"
)

type ToolSchemaProfile struct {
	StripUnsupportedKeywords bool
	UnsupportedKeywords      []string
}

type StreamingProfile struct {
	SupportsMultiToolCallsPerChunk bool
}

type ProviderCapabilities struct {
	ToolSchema ToolSchemaProfile
	Streaming  StreamingProfile
}

func DefaultCapabilities() ProviderCapabilities {
	return ProviderCapabilities{
		ToolSchema: ToolSchemaProfile{},
		Streaming: StreamingProfile{
			SupportsMultiToolCallsPerChunk: true,
		},
	}
}

func OpenAICompatibleCapabilities() ProviderCapabilities {
	return ProviderCapabilities{
		ToolSchema: ToolSchemaProfile{
			StripUnsupportedKeywords: true,
			UnsupportedKeywords:      []string{"anyOf", "oneOf", "allOf", "enum", "not"},
		},
		Streaming: StreamingProfile{
			SupportsMultiToolCallsPerChunk: true,
		},
	}
}

func AdaptToolDefs(caps ProviderCapabilities, defs []types.ToolDef) []types.ToolDef {
	if len(defs) == 0 {
		return nil
	}
	adapted := make([]types.ToolDef, len(defs))
	for i, def := range defs {
		adapted[i] = def
		adapted[i].Parameters = AdaptToolSchema(caps.ToolSchema, def.Parameters)
	}
	return adapted
}

func AdaptToolSchema(profile ToolSchemaProfile, schema json.RawMessage) json.RawMessage {
	if !profile.StripUnsupportedKeywords || len(profile.UnsupportedKeywords) == 0 || len(schema) == 0 {
		return schema
	}

	var decoded any
	if err := json.Unmarshal(schema, &decoded); err != nil {
		return schema
	}

	disallowed := make(map[string]bool, len(profile.UnsupportedKeywords))
	for _, keyword := range profile.UnsupportedKeywords {
		disallowed[keyword] = true
	}

	decoded = stripSchemaKeywords(decoded, disallowed)
	encoded, err := json.Marshal(decoded)
	if err != nil {
		return schema
	}
	return json.RawMessage(encoded)
}

func stripSchemaKeywords(value any, disallowed map[string]bool) any {
	switch typed := value.(type) {
	case map[string]any:
		cleaned := make(map[string]any, len(typed))
		for key, item := range typed {
			if disallowed[key] {
				continue
			}
			cleaned[key] = stripSchemaKeywords(item, disallowed)
		}
		return cleaned
	case []any:
		items := make([]any, len(typed))
		for i, item := range typed {
			items[i] = stripSchemaKeywords(item, disallowed)
		}
		return items
	default:
		return value
	}
}
