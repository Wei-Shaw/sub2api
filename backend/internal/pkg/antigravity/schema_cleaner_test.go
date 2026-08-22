package antigravity

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCleanJSONSchema_ExpandsLocalDefinitions(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"item": map[string]any{
				"$ref": "#/definitions/Item",
			},
		},
		"definitions": map[string]any{
			"Item": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name": map[string]any{
						"type": "string",
					},
				},
				"required": []any{"name"},
			},
		},
	}

	cleaned := CleanJSONSchema(schema)

	require.NotNil(t, cleaned)
	encoded, err := json.Marshal(cleaned)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), `"$ref"`)
	require.NotContains(t, string(encoded), `"definitions"`)

	properties := cleaned["properties"].(map[string]any)
	item := properties["item"].(map[string]any)
	require.Equal(t, "object", item["type"])
	itemProperties := item["properties"].(map[string]any)
	require.Equal(t, "string", itemProperties["name"].(map[string]any)["type"])
	require.Equal(t, []any{"name"}, item["required"])
}

func TestCleanJSONSchema_SelfReferenceTerminatesWithoutRefs(t *testing.T) {
	schema := map[string]any{
		"$ref": "#/$defs/Node",
		"$defs": map[string]any{
			"Node": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"value": map[string]any{
						"type": "string",
					},
					"next": map[string]any{
						"$ref": "#/$defs/Node",
					},
				},
			},
		},
	}

	cleaned := CleanJSONSchema(schema)

	require.NotNil(t, cleaned)
	encoded, err := json.Marshal(cleaned)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), `"$ref"`)
	require.NotContains(t, string(encoded), `"$defs"`)
	require.Equal(t, "object", cleaned["type"])

	properties := cleaned["properties"].(map[string]any)
	require.Equal(t, "string", properties["value"].(map[string]any)["type"])
	require.Equal(t, "object", properties["next"].(map[string]any)["type"])
}

func TestCleanJSONSchema_ExternalRefDoesNotResolveCollidingLocalDefinition(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"external": map[string]any{
				"$ref": "https://example.com/schema.json#/$defs/Item",
			},
		},
		"$defs": map[string]any{
			"Item": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name": map[string]any{
						"type": "string",
					},
				},
			},
		},
	}

	cleaned := CleanJSONSchema(schema)

	require.NotNil(t, cleaned)
	encoded, err := json.Marshal(cleaned)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), `"$ref"`)

	properties := cleaned["properties"].(map[string]any)
	external := properties["external"].(map[string]any)
	require.Equal(t, "object", external["type"])
	require.NotContains(t, external["properties"], "name")
}
