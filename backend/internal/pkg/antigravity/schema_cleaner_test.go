package antigravity

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCleanJSONSchema_ArrayPrefixItems(t *testing.T) {
	// 模拟 Claude Code 2.1 Artifact 工具的 query.where 参数 Schema (Draft 2020-12 prefixItems 元组)
	input := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"where": map[string]any{
						"type": "array",
						"items": map[string]any{
							"type": "array",
							"prefixItems": []any{
								map[string]any{"type": "string"},
								map[string]any{"type": "string", "enum": []any{"==", "!=", ">", "<"}},
								map[string]any{},
							},
						},
						"maxItems": float64(10),
					},
				},
			},
		},
	}

	cleaned := CleanJSONSchema(input)
	require.NotNil(t, cleaned)

	query, ok := cleaned["properties"].(map[string]any)["query"].(map[string]any)
	require.True(t, ok)
	where, ok := query["properties"].(map[string]any)["where"].(map[string]any)
	require.True(t, ok)

	assert.Equal(t, "array", where["type"])
	// prefixItems 应被移除或替换
	assert.Nil(t, where["prefixItems"])

	whereItems, ok := where["items"].(map[string]any)
	require.True(t, ok, "where.items must be an object")
	assert.Equal(t, "array", whereItems["type"])

	// 关键验证：where.items.items 必须存在且有效，不能缺失导致 Gemini 400
	innerItems, ok := whereItems["items"].(map[string]any)
	require.True(t, ok, "where.items.items must be an object")
	assert.Equal(t, "string", innerItems["type"])
}

func TestCleanJSONSchema_ArrayMissingItemsFallback(t *testing.T) {
	// 针对任何缺少 items 的 array，必须兜底注入 items: {type: string}
	input := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"tags": map[string]any{
				"type": "array",
			},
			"nested_empty_array": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "array",
				},
			},
		},
	}

	cleaned := CleanJSONSchema(input)
	require.NotNil(t, cleaned)

	props := cleaned["properties"].(map[string]any)
	tags := props["tags"].(map[string]any)
	require.NotNil(t, tags["items"])
	assert.Equal(t, "string", tags["items"].(map[string]any)["type"])

	nested := props["nested_empty_array"].(map[string]any)
	nestedItems := nested["items"].(map[string]any)
	assert.Equal(t, "array", nestedItems["type"])
	require.NotNil(t, nestedItems["items"])
	assert.Equal(t, "string", nestedItems["items"].(map[string]any)["type"])
}

func TestCleanJSONSchema_ArrayExistingItemsPreserved(t *testing.T) {
	// 正常的 array items 不受影响
	input := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"numbers": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "integer",
				},
			},
		},
	}

	cleaned := CleanJSONSchema(input)
	require.NotNil(t, cleaned)

	numbers := cleaned["properties"].(map[string]any)["numbers"].(map[string]any)
	assert.Equal(t, "array", numbers["type"])
	items := numbers["items"].(map[string]any)
	assert.Equal(t, "integer", items["type"])
}
