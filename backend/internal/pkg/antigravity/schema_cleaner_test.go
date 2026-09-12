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

	whereItems, ok := where["items"].(map[string]any)
	require.True(t, ok, "where.items must be an object")
	assert.Equal(t, "array", whereItems["type"])
	// prefixItems 应在 whereItems 中被彻底移除
	assert.Nil(t, whereItems["prefixItems"])

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

func TestCleanJSONSchema_EnumOnlySchemaInTuple(t *testing.T) {
	// 测试 enum-only schema 不会被误判为 object，也不应被注入 reason 属性
	input := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"choice": map[string]any{
				"enum": []any{"option_a", "option_b"},
			},
			"tuple_with_enum": map[string]any{
				"type": "array",
				"prefixItems": []any{
					map[string]any{
						"enum": []any{"read", "write"},
					},
				},
			},
		},
	}

	cleaned := CleanJSONSchema(input)
	require.NotNil(t, cleaned)

	props := cleaned["properties"].(map[string]any)
	choice := props["choice"].(map[string]any)
	assert.Equal(t, "string", choice["type"])
	assert.Nil(t, choice["properties"], "enum-only schema must NOT be treated as object with reason property")
	assert.Equal(t, []any{"option_a", "option_b"}, choice["enum"])

	tupleArray := props["tuple_with_enum"].(map[string]any)
	tupleItems := tupleArray["items"].(map[string]any)
	assert.Equal(t, "string", tupleItems["type"])
	assert.Nil(t, tupleItems["properties"])
	assert.Equal(t, []any{"read", "write"}, tupleItems["enum"])
}

func TestCleanJSONSchema_ConstKeywordConversion(t *testing.T) {
	// 测试 const 关键字自动转换为 Gemini 兼容的 enum: [const] 且具有合法 type
	input := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"const": "ping",
			},
		},
	}

	cleaned := CleanJSONSchema(input)
	require.NotNil(t, cleaned)

	props := cleaned["properties"].(map[string]any)
	action := props["action"].(map[string]any)
	assert.Nil(t, action["const"], "const keyword must be removed")
	assert.Equal(t, "string", action["type"])
	assert.Equal(t, []any{"ping"}, action["enum"])
}

func TestCleanJSONSchema_AnyOfNestedPrefixItems(t *testing.T) {
	// 测试 anyOf 分支合并进来的嵌套 prefixItems 也被完整深度清洗
	input := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"status": map[string]any{"type": "string"},
		},
		"anyOf": []any{
			map[string]any{
				"properties": map[string]any{
					"extra_tuple": map[string]any{
						"type": "array",
						"prefixItems": []any{
							map[string]any{"type": "integer"},
						},
					},
				},
			},
		},
	}

	cleaned := CleanJSONSchema(input)
	require.NotNil(t, cleaned)

	props := cleaned["properties"].(map[string]any)
	require.NotNil(t, props["extra_tuple"])
	extraTuple := props["extra_tuple"].(map[string]any)
	assert.Nil(t, extraTuple["prefixItems"], "nested prefixItems from anyOf merge must be cleaned")
	require.NotNil(t, extraTuple["items"])
	assert.Equal(t, "integer", extraTuple["items"].(map[string]any)["type"])
}

func TestCleanJSONSchema_EmptyPrefixItems(t *testing.T) {
	// 验证空 prefixItems: [] 也被安全删除，不遗留非法关键字
	input := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"empty_tuple": map[string]any{
				"type":        "array",
				"prefixItems": []any{},
			},
		},
	}

	cleaned := CleanJSONSchema(input)
	require.NotNil(t, cleaned)

	props := cleaned["properties"].(map[string]any)
	emptyTuple := props["empty_tuple"].(map[string]any)
	assert.Nil(t, emptyTuple["prefixItems"], "empty prefixItems array must be removed")
	require.NotNil(t, emptyTuple["items"])
	assert.Equal(t, "string", emptyTuple["items"].(map[string]any)["type"])
}

