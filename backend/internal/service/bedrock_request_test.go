package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestPrepareBedrockRequestBody_BasicFields(t *testing.T) {
	input := `{"model":"claude-opus-4-6","stream":true,"max_tokens":1024,"messages":[{"role":"user","content":"hi"}]}`
	result, err := PrepareBedrockRequestBody([]byte(input), "us.anthropic.claude-opus-4-6-v1", "")
	require.NoError(t, err)

	// anthropic_version 应被注入
	assert.Equal(t, "bedrock-2023-05-31", gjson.GetBytes(result, "anthropic_version").String())
	// model 和 stream 应被移除
	assert.False(t, gjson.GetBytes(result, "model").Exists())
	assert.False(t, gjson.GetBytes(result, "stream").Exists())
	// max_tokens 应保留
	assert.Equal(t, int64(1024), gjson.GetBytes(result, "max_tokens").Int())
}

func TestPrepareBedrockRequestBody_OutputFormatInlineSchema(t *testing.T) {
	t.Run("schema inlined into last user message array content", func(t *testing.T) {
		input := `{"model":"claude-sonnet-4-5","output_format":{"type":"json","schema":{"name":"string"}},"messages":[{"role":"user","content":[{"type":"text","text":"hello"}]}]}`
		result, err := PrepareBedrockRequestBody([]byte(input), "us.anthropic.claude-sonnet-4-5-v1", "")
		require.NoError(t, err)

		assert.False(t, gjson.GetBytes(result, "output_format").Exists())
		// schema 应内联到最后一条 user message 的 content 数组末尾
		contentArr := gjson.GetBytes(result, "messages.0.content").Array()
		require.Len(t, contentArr, 2)
		assert.Equal(t, "text", contentArr[1].Get("type").String())
		assert.Contains(t, contentArr[1].Get("text").String(), `"name":"string"`)
	})

	t.Run("schema inlined into string content", func(t *testing.T) {
		input := `{"model":"claude-sonnet-4-5","output_format":{"type":"json","schema":{"result":"number"}},"messages":[{"role":"user","content":"compute this"}]}`
		result, err := PrepareBedrockRequestBody([]byte(input), "us.anthropic.claude-sonnet-4-5-v1", "")
		require.NoError(t, err)

		assert.False(t, gjson.GetBytes(result, "output_format").Exists())
		contentArr := gjson.GetBytes(result, "messages.0.content").Array()
		require.Len(t, contentArr, 2)
		assert.Equal(t, "compute this", contentArr[0].Get("text").String())
		assert.Contains(t, contentArr[1].Get("text").String(), `"result":"number"`)
	})

	t.Run("no schema field just removes output_format", func(t *testing.T) {
		input := `{"model":"claude-sonnet-4-5","output_format":{"type":"json"},"messages":[{"role":"user","content":"hi"}]}`
		result, err := PrepareBedrockRequestBody([]byte(input), "us.anthropic.claude-sonnet-4-5-v1", "")
		require.NoError(t, err)

		assert.False(t, gjson.GetBytes(result, "output_format").Exists())
	})

	t.Run("no messages just removes output_format", func(t *testing.T) {
		input := `{"model":"claude-sonnet-4-5","output_format":{"type":"json","schema":{"name":"string"}}}`
		result, err := PrepareBedrockRequestBody([]byte(input), "us.anthropic.claude-sonnet-4-5-v1", "")
		require.NoError(t, err)

		assert.False(t, gjson.GetBytes(result, "output_format").Exists())
	})
}

func TestPrepareBedrockRequestBody_RemoveOutputConfig(t *testing.T) {
	input := `{"model":"claude-sonnet-4-5","output_config":{"max_tokens":100},"messages":[]}`
	result, err := PrepareBedrockRequestBody([]byte(input), "us.anthropic.claude-sonnet-4-5-v1", "")
	require.NoError(t, err)

	assert.False(t, gjson.GetBytes(result, "output_config").Exists())
}

func TestRemoveCustomFieldFromTools(t *testing.T) {
	input := `{
		"tools": [
			{"name":"tool1","custom":{"defer_loading":true},"description":"desc1"},
			{"name":"tool2","description":"desc2"},
			{"name":"tool3","custom":{"defer_loading":true,"other":123},"description":"desc3"}
		]
	}`
	result := removeCustomFieldFromTools([]byte(input))

	tools := gjson.GetBytes(result, "tools").Array()
	require.Len(t, tools, 3)
	// custom 应被移除
	assert.False(t, tools[0].Get("custom").Exists())
	// name/description 应保留
	assert.Equal(t, "tool1", tools[0].Get("name").String())
	assert.Equal(t, "desc1", tools[0].Get("description").String())
	// 没有 custom 的工具不受影响
	assert.Equal(t, "tool2", tools[1].Get("name").String())
	// 第三个工具的 custom 也应被移除
	assert.False(t, tools[2].Get("custom").Exists())
	assert.Equal(t, "tool3", tools[2].Get("name").String())
}

func TestRemoveCustomFieldFromTools_NoTools(t *testing.T) {
	input := `{"messages":[{"role":"user","content":"hi"}]}`
	result := removeCustomFieldFromTools([]byte(input))
	// 无 tools 时不改变原始数据
	assert.JSONEq(t, input, string(result))
}

func TestSanitizeBedrockCacheControl_RemoveScope(t *testing.T) {
	input := `{
		"system": [{"type":"text","text":"sys","cache_control":{"type":"ephemeral","scope":"global"}}],
		"messages": [{"role":"user","content":[{"type":"text","text":"hi","cache_control":{"type":"ephemeral","scope":"global"}}]}]
	}`
	result := sanitizeBedrockCacheControl([]byte(input), "us.anthropic.claude-opus-4-6-v1")

	// scope 应被移除
	assert.False(t, gjson.GetBytes(result, "system.0.cache_control.scope").Exists())
	assert.False(t, gjson.GetBytes(result, "messages.0.content.0.cache_control.scope").Exists())
	// type 应保留
	assert.Equal(t, "ephemeral", gjson.GetBytes(result, "system.0.cache_control.type").String())
	assert.Equal(t, "ephemeral", gjson.GetBytes(result, "messages.0.content.0.cache_control.type").String())
}

func TestSanitizeBedrockCacheControl_TTL_OldModel(t *testing.T) {
	input := `{
		"system": [{"type":"text","text":"sys","cache_control":{"type":"ephemeral","ttl":"5m"}}]
	}`
	// 旧模型（Claude 3.5）不支持 ttl
	result := sanitizeBedrockCacheControl([]byte(input), "anthropic.claude-3-5-sonnet-20241022-v2:0")

	assert.False(t, gjson.GetBytes(result, "system.0.cache_control.ttl").Exists())
	assert.Equal(t, "ephemeral", gjson.GetBytes(result, "system.0.cache_control.type").String())
}

func TestSanitizeBedrockCacheControl_TTL_Claude45_Supported(t *testing.T) {
	input := `{
		"system": [{"type":"text","text":"sys","cache_control":{"type":"ephemeral","ttl":"5m"}}]
	}`
	// Claude 4.5+ 支持 "5m" 和 "1h"
	result := sanitizeBedrockCacheControl([]byte(input), "us.anthropic.claude-sonnet-4-5-20250929-v1:0")

	assert.True(t, gjson.GetBytes(result, "system.0.cache_control.ttl").Exists())
	assert.Equal(t, "5m", gjson.GetBytes(result, "system.0.cache_control.ttl").String())
}

func TestSanitizeBedrockCacheControl_TTL_Claude45_UnsupportedValue(t *testing.T) {
	input := `{
		"system": [{"type":"text","text":"sys","cache_control":{"type":"ephemeral","ttl":"10m"}}]
	}`
	// Claude 4.5 不支持 "10m"
	result := sanitizeBedrockCacheControl([]byte(input), "us.anthropic.claude-sonnet-4-5-20250929-v1:0")

	assert.False(t, gjson.GetBytes(result, "system.0.cache_control.ttl").Exists())
}

func TestSanitizeBedrockCacheControl_TTL_Claude46(t *testing.T) {
	input := `{
		"messages": [{"role":"user","content":[{"type":"text","text":"hi","cache_control":{"type":"ephemeral","ttl":"1h"}}]}]
	}`
	result := sanitizeBedrockCacheControl([]byte(input), "us.anthropic.claude-opus-4-6-v1")

	assert.True(t, gjson.GetBytes(result, "messages.0.content.0.cache_control.ttl").Exists())
	assert.Equal(t, "1h", gjson.GetBytes(result, "messages.0.content.0.cache_control.ttl").String())
}

func TestSanitizeBedrockCacheControl_NoCacheControl(t *testing.T) {
	input := `{"system":[{"type":"text","text":"sys"}],"messages":[{"role":"user","content":[{"type":"text","text":"hi"}]}]}`
	result := sanitizeBedrockCacheControl([]byte(input), "us.anthropic.claude-opus-4-6-v1")
	// 无 cache_control 时不改变原始数据
	assert.JSONEq(t, input, string(result))
}

func TestIsBedrockClaude45OrNewer(t *testing.T) {
	tests := []struct {
		modelID string
		expect  bool
	}{
		{"us.anthropic.claude-opus-4-6-v1", true},
		{"us.anthropic.claude-sonnet-4-6", true},
		{"us.anthropic.claude-sonnet-4-5-20250929-v1:0", true},
		{"us.anthropic.claude-opus-4-5-20251101-v1:0", true},
		{"us.anthropic.claude-haiku-4-5-20251001-v1:0", true},
		{"anthropic.claude-3-5-sonnet-20241022-v2:0", false},
		{"anthropic.claude-3-opus-20240229-v1:0", false},
		{"anthropic.claude-3-haiku-20240307-v1:0", false},
		// 未来版本应自动支持
		{"us.anthropic.claude-sonnet-5-0-v1", true},
		{"us.anthropic.claude-opus-4-7-v1", true},
		// 旧版本
		{"anthropic.claude-opus-4-1-v1", false},
		{"anthropic.claude-sonnet-4-0-v1", false},
		// 非 Claude 模型
		{"amazon.nova-pro-v1", false},
		{"meta.llama3-70b", false},
	}
	for _, tt := range tests {
		t.Run(tt.modelID, func(t *testing.T) {
			assert.Equal(t, tt.expect, isBedrockClaude45OrNewer(tt.modelID))
		})
	}
}

func TestPrepareBedrockRequestBody_FullIntegration(t *testing.T) {
	// 模拟一个完整的 Claude Code 请求
	input := `{
		"model": "claude-opus-4-6",
		"stream": true,
		"max_tokens": 16384,
		"output_format": {"type": "json", "schema": {"result": "string"}},
		"output_config": {"max_tokens": 100},
		"system": [{"type": "text", "text": "You are helpful", "cache_control": {"type": "ephemeral", "scope": "global", "ttl": "5m"}}],
		"messages": [
			{"role": "user", "content": [{"type": "text", "text": "hello", "cache_control": {"type": "ephemeral", "ttl": "1h"}}]}
		],
		"tools": [
			{"name": "bash", "description": "Run bash", "custom": {"defer_loading": true}, "input_schema": {"type": "object"}},
			{"name": "read", "description": "Read file", "input_schema": {"type": "object"}}
		]
	}`

	betaHeader := "interleaved-thinking-2025-05-14, context-1m-2025-08-07, compact-2026-01-12"
	result, err := PrepareBedrockRequestBody([]byte(input), "us.anthropic.claude-opus-4-6-v1", betaHeader)
	require.NoError(t, err)

	// 基本字段
	assert.Equal(t, "bedrock-2023-05-31", gjson.GetBytes(result, "anthropic_version").String())
	assert.False(t, gjson.GetBytes(result, "model").Exists())
	assert.False(t, gjson.GetBytes(result, "stream").Exists())
	assert.Equal(t, int64(16384), gjson.GetBytes(result, "max_tokens").Int())

	// anthropic_beta 应包含所有 beta tokens
	betaArr := gjson.GetBytes(result, "anthropic_beta").Array()
	require.Len(t, betaArr, 3)
	assert.Equal(t, "interleaved-thinking-2025-05-14", betaArr[0].String())
	assert.Equal(t, "context-1m-2025-08-07", betaArr[1].String())
	assert.Equal(t, "compact-2026-01-12", betaArr[2].String())

	// output_format 应被移除，schema 内联到最后一条 user message
	assert.False(t, gjson.GetBytes(result, "output_format").Exists())
	assert.False(t, gjson.GetBytes(result, "output_config").Exists())
	// content 数组：原始 text block + 内联 schema block
	contentArr := gjson.GetBytes(result, "messages.0.content").Array()
	require.Len(t, contentArr, 2)
	assert.Equal(t, "hello", contentArr[0].Get("text").String())
	assert.Contains(t, contentArr[1].Get("text").String(), `"result":"string"`)

	// tools 中的 custom 应被移除
	assert.False(t, gjson.GetBytes(result, "tools.0.custom").Exists())
	assert.Equal(t, "bash", gjson.GetBytes(result, "tools.0.name").String())
	assert.Equal(t, "read", gjson.GetBytes(result, "tools.1.name").String())

	// cache_control: scope 应被移除，ttl 在 Claude 4.6 上保留合法值
	assert.False(t, gjson.GetBytes(result, "system.0.cache_control.scope").Exists())
	assert.Equal(t, "ephemeral", gjson.GetBytes(result, "system.0.cache_control.type").String())
	assert.Equal(t, "5m", gjson.GetBytes(result, "system.0.cache_control.ttl").String())
	assert.Equal(t, "1h", gjson.GetBytes(result, "messages.0.content.0.cache_control.ttl").String())
}

func TestPrepareBedrockRequestBody_BetaHeader(t *testing.T) {
	input := `{"messages":[{"role":"user","content":"hi"}],"max_tokens":100}`

	t.Run("empty beta header", func(t *testing.T) {
		result, err := PrepareBedrockRequestBody([]byte(input), "us.anthropic.claude-opus-4-6-v1", "")
		require.NoError(t, err)
		assert.False(t, gjson.GetBytes(result, "anthropic_beta").Exists())
	})

	t.Run("single beta token", func(t *testing.T) {
		result, err := PrepareBedrockRequestBody([]byte(input), "us.anthropic.claude-opus-4-6-v1", "interleaved-thinking-2025-05-14")
		require.NoError(t, err)
		arr := gjson.GetBytes(result, "anthropic_beta").Array()
		require.Len(t, arr, 1)
		assert.Equal(t, "interleaved-thinking-2025-05-14", arr[0].String())
	})

	t.Run("multiple beta tokens with spaces", func(t *testing.T) {
		result, err := PrepareBedrockRequestBody([]byte(input), "us.anthropic.claude-opus-4-6-v1", "interleaved-thinking-2025-05-14 , context-1m-2025-08-07 ")
		require.NoError(t, err)
		arr := gjson.GetBytes(result, "anthropic_beta").Array()
		require.Len(t, arr, 2)
		assert.Equal(t, "interleaved-thinking-2025-05-14", arr[0].String())
		assert.Equal(t, "context-1m-2025-08-07", arr[1].String())
	})

	t.Run("json array beta header", func(t *testing.T) {
		result, err := PrepareBedrockRequestBody([]byte(input), "us.anthropic.claude-opus-4-6-v1", `["interleaved-thinking-2025-05-14","context-1m-2025-08-07"]`)
		require.NoError(t, err)
		arr := gjson.GetBytes(result, "anthropic_beta").Array()
		require.Len(t, arr, 2)
		assert.Equal(t, "interleaved-thinking-2025-05-14", arr[0].String())
		assert.Equal(t, "context-1m-2025-08-07", arr[1].String())
	})
}

func TestParseAnthropicBetaHeader(t *testing.T) {
	assert.Nil(t, parseAnthropicBetaHeader(""))
	assert.Equal(t, []string{"a"}, parseAnthropicBetaHeader("a"))
	assert.Equal(t, []string{"a", "b"}, parseAnthropicBetaHeader("a,b"))
	assert.Equal(t, []string{"a", "b"}, parseAnthropicBetaHeader("a , b "))
	assert.Equal(t, []string{"a", "b", "c"}, parseAnthropicBetaHeader("a,b,c"))
	assert.Equal(t, []string{"a", "b"}, parseAnthropicBetaHeader(`["a","b"]`))
}

func TestFilterBedrockBetaTokens(t *testing.T) {
	t.Run("supported tokens pass through", func(t *testing.T) {
		tokens := []string{"interleaved-thinking-2025-05-14", "context-1m-2025-08-07", "compact-2026-01-12"}
		result := filterBedrockBetaTokens(tokens)
		assert.Equal(t, tokens, result)
	})

	t.Run("unsupported tokens are filtered out", func(t *testing.T) {
		tokens := []string{"interleaved-thinking-2025-05-14", "output-128k-2025-02-19", "files-api-2025-04-14", "structured-outputs-2025-11-13"}
		result := filterBedrockBetaTokens(tokens)
		assert.Equal(t, []string{"interleaved-thinking-2025-05-14"}, result)
	})

	t.Run("advanced-tool-use transforms to tool-search-tool", func(t *testing.T) {
		tokens := []string{"advanced-tool-use-2025-11-20"}
		result := filterBedrockBetaTokens(tokens)
		assert.Contains(t, result, "tool-search-tool-2025-10-19")
		// tool-examples 自动关联
		assert.Contains(t, result, "tool-examples-2025-10-29")
	})

	t.Run("tool-search-tool auto-associates tool-examples", func(t *testing.T) {
		tokens := []string{"tool-search-tool-2025-10-19"}
		result := filterBedrockBetaTokens(tokens)
		assert.Contains(t, result, "tool-search-tool-2025-10-19")
		assert.Contains(t, result, "tool-examples-2025-10-29")
	})

	t.Run("no duplication when tool-examples already present", func(t *testing.T) {
		tokens := []string{"tool-search-tool-2025-10-19", "tool-examples-2025-10-29"}
		result := filterBedrockBetaTokens(tokens)
		count := 0
		for _, t := range result {
			if t == "tool-examples-2025-10-29" {
				count++
			}
		}
		assert.Equal(t, 1, count)
	})

	t.Run("empty input returns nil", func(t *testing.T) {
		result := filterBedrockBetaTokens(nil)
		assert.Nil(t, result)
	})

	t.Run("all unsupported returns nil", func(t *testing.T) {
		result := filterBedrockBetaTokens([]string{"output-128k-2025-02-19", "effort-2025-11-24"})
		assert.Nil(t, result)
	})

	t.Run("duplicate tokens are deduplicated", func(t *testing.T) {
		tokens := []string{"context-1m-2025-08-07", "context-1m-2025-08-07"}
		result := filterBedrockBetaTokens(tokens)
		assert.Equal(t, []string{"context-1m-2025-08-07"}, result)
	})
}

func TestPrepareBedrockRequestBody_BetaFiltering(t *testing.T) {
	input := `{"messages":[{"role":"user","content":"hi"}],"max_tokens":100}`

	t.Run("unsupported beta tokens are filtered", func(t *testing.T) {
		result, err := PrepareBedrockRequestBody([]byte(input), "us.anthropic.claude-opus-4-6-v1",
			"interleaved-thinking-2025-05-14, output-128k-2025-02-19, files-api-2025-04-14")
		require.NoError(t, err)
		arr := gjson.GetBytes(result, "anthropic_beta").Array()
		require.Len(t, arr, 1)
		assert.Equal(t, "interleaved-thinking-2025-05-14", arr[0].String())
	})

	t.Run("advanced-tool-use transformed in full pipeline", func(t *testing.T) {
		result, err := PrepareBedrockRequestBody([]byte(input), "us.anthropic.claude-opus-4-6-v1",
			"advanced-tool-use-2025-11-20")
		require.NoError(t, err)
		arr := gjson.GetBytes(result, "anthropic_beta").Array()
		require.Len(t, arr, 2)
		assert.Equal(t, "tool-search-tool-2025-10-19", arr[0].String())
		assert.Equal(t, "tool-examples-2025-10-29", arr[1].String())
	})
}

func TestBedrockCrossRegionPrefix(t *testing.T) {
	tests := []struct {
		region string
		expect string
	}{
		// US regions
		{"us-east-1", "us"},
		{"us-east-2", "us"},
		{"us-west-1", "us"},
		{"us-west-2", "us"},
		// GovCloud
		{"us-gov-east-1", "us-gov"},
		{"us-gov-west-1", "us-gov"},
		// EU regions
		{"eu-west-1", "eu"},
		{"eu-west-2", "eu"},
		{"eu-west-3", "eu"},
		{"eu-central-1", "eu"},
		{"eu-central-2", "eu"},
		{"eu-north-1", "eu"},
		{"eu-south-1", "eu"},
		// APAC regions
		{"ap-northeast-1", "jp"},
		{"ap-northeast-2", "apac"},
		{"ap-southeast-1", "apac"},
		{"ap-southeast-2", "au"},
		{"ap-south-1", "apac"},
		// Canada / South America fallback to us
		{"ca-central-1", "us"},
		{"sa-east-1", "us"},
		// Unknown defaults to us
		{"me-south-1", "us"},
	}
	for _, tt := range tests {
		t.Run(tt.region, func(t *testing.T) {
			assert.Equal(t, tt.expect, BedrockCrossRegionPrefix(tt.region))
		})
	}
}

func TestResolveBedrockModelID(t *testing.T) {
	t.Run("default alias resolves and adjusts region", func(t *testing.T) {
		account := &Account{
			Platform: PlatformAnthropic,
			Type:     AccountTypeBedrock,
			Credentials: map[string]any{
				"aws_region": "eu-west-1",
			},
		}

		modelID, ok := ResolveBedrockModelID(account, "claude-sonnet-4-5")
		require.True(t, ok)
		assert.Equal(t, "eu.anthropic.claude-sonnet-4-5-20250929-v1:0", modelID)
	})

	t.Run("custom alias mapping reuses default bedrock mapping", func(t *testing.T) {
		account := &Account{
			Platform: PlatformAnthropic,
			Type:     AccountTypeBedrock,
			Credentials: map[string]any{
				"aws_region": "ap-southeast-2",
				"model_mapping": map[string]any{
					"claude-*": "claude-opus-4-6",
				},
			},
		}

		modelID, ok := ResolveBedrockModelID(account, "claude-opus-4-6-thinking")
		require.True(t, ok)
		assert.Equal(t, "au.anthropic.claude-opus-4-6-v1", modelID)
	})

	t.Run("force global rewrites anthropic regional model id", func(t *testing.T) {
		account := &Account{
			Platform: PlatformAnthropic,
			Type:     AccountTypeBedrock,
			Credentials: map[string]any{
				"aws_region":       "us-east-1",
				"aws_force_global": "true",
				"model_mapping": map[string]any{
					"claude-sonnet-4-6": "us.anthropic.claude-sonnet-4-6",
				},
			},
		}

		modelID, ok := ResolveBedrockModelID(account, "claude-sonnet-4-6")
		require.True(t, ok)
		assert.Equal(t, "global.anthropic.claude-sonnet-4-6", modelID)
	})

	t.Run("direct bedrock model id passes through", func(t *testing.T) {
		account := &Account{
			Platform: PlatformAnthropic,
			Type:     AccountTypeBedrock,
			Credentials: map[string]any{
				"aws_region": "us-east-1",
			},
		}

		modelID, ok := ResolveBedrockModelID(account, "anthropic.claude-haiku-4-5-20251001-v1:0")
		require.True(t, ok)
		assert.Equal(t, "anthropic.claude-haiku-4-5-20251001-v1:0", modelID)
	})

	t.Run("unsupported alias returns false", func(t *testing.T) {
		account := &Account{
			Platform: PlatformAnthropic,
			Type:     AccountTypeBedrock,
			Credentials: map[string]any{
				"aws_region": "us-east-1",
			},
		}

		_, ok := ResolveBedrockModelID(account, "claude-3-5-sonnet-20241022")
		assert.False(t, ok)
	})
}

func TestAutoInjectBedrockBetaTokens(t *testing.T) {
	t.Run("inject interleaved-thinking when thinking present", func(t *testing.T) {
		body := []byte(`{"thinking":{"type":"enabled","budget_tokens":10000},"messages":[{"role":"user","content":"hi"}]}`)
		result := autoInjectBedrockBetaTokens(nil, body, "us.anthropic.claude-opus-4-6-v1")
		assert.Contains(t, result, "interleaved-thinking-2025-05-14")
	})

	t.Run("no duplicate when already present", func(t *testing.T) {
		body := []byte(`{"thinking":{"type":"enabled","budget_tokens":10000},"messages":[{"role":"user","content":"hi"}]}`)
		result := autoInjectBedrockBetaTokens([]string{"interleaved-thinking-2025-05-14"}, body, "us.anthropic.claude-opus-4-6-v1")
		count := 0
		for _, t := range result {
			if t == "interleaved-thinking-2025-05-14" {
				count++
			}
		}
		assert.Equal(t, 1, count)
	})

	t.Run("inject computer-use when computer tool present", func(t *testing.T) {
		body := []byte(`{"tools":[{"type":"computer_20250124","name":"computer","display_width_px":1024}],"messages":[{"role":"user","content":"hi"}]}`)
		result := autoInjectBedrockBetaTokens(nil, body, "us.anthropic.claude-opus-4-6-v1")
		assert.Contains(t, result, "computer-use-2025-11-24")
	})

	t.Run("inject advanced-tool-use for programmatic tool calling", func(t *testing.T) {
		body := []byte(`{"tools":[{"name":"bash","allowed_callers":["code_execution_20250825"]}],"messages":[{"role":"user","content":"hi"}]}`)
		result := autoInjectBedrockBetaTokens(nil, body, "us.anthropic.claude-opus-4-6-v1")
		assert.Contains(t, result, "advanced-tool-use-2025-11-20")
	})

	t.Run("inject advanced-tool-use for input examples", func(t *testing.T) {
		body := []byte(`{"tools":[{"name":"bash","input_examples":[{"cmd":"ls"}]}],"messages":[{"role":"user","content":"hi"}]}`)
		result := autoInjectBedrockBetaTokens(nil, body, "us.anthropic.claude-opus-4-6-v1")
		assert.Contains(t, result, "advanced-tool-use-2025-11-20")
	})

	t.Run("inject tool-search-tool directly for pure tool search (no programmatic/inputExamples)", func(t *testing.T) {
		body := []byte(`{"tools":[{"type":"tool_search_tool_regex_20251119","name":"search"}],"messages":[{"role":"user","content":"hi"}]}`)
		result := autoInjectBedrockBetaTokens(nil, body, "us.anthropic.claude-sonnet-4-6")
		// 纯 tool search 场景直接注入 Bedrock 特定头，不走 advanced-tool-use 转换
		assert.Contains(t, result, "tool-search-tool-2025-10-19")
		assert.NotContains(t, result, "advanced-tool-use-2025-11-20")
	})

	t.Run("inject advanced-tool-use when tool search combined with programmatic calling", func(t *testing.T) {
		body := []byte(`{"tools":[{"type":"tool_search_tool_regex_20251119","name":"search"},{"name":"bash","allowed_callers":["code_execution_20250825"]}],"messages":[{"role":"user","content":"hi"}]}`)
		result := autoInjectBedrockBetaTokens(nil, body, "us.anthropic.claude-sonnet-4-6")
		// 混合场景使用 advanced-tool-use（后续由 filter 转换为 tool-search-tool）
		assert.Contains(t, result, "advanced-tool-use-2025-11-20")
	})

	t.Run("do not inject tool-search beta for unsupported models", func(t *testing.T) {
		body := []byte(`{"tools":[{"type":"tool_search_tool_regex_20251119","name":"search"}],"messages":[{"role":"user","content":"hi"}]}`)
		result := autoInjectBedrockBetaTokens(nil, body, "anthropic.claude-3-5-sonnet-20241022-v2:0")
		assert.NotContains(t, result, "advanced-tool-use-2025-11-20")
		assert.NotContains(t, result, "tool-search-tool-2025-10-19")
	})

	t.Run("no injection for regular tools", func(t *testing.T) {
		body := []byte(`{"tools":[{"name":"bash","description":"run bash","input_schema":{"type":"object"}}],"messages":[{"role":"user","content":"hi"}]}`)
		result := autoInjectBedrockBetaTokens(nil, body, "us.anthropic.claude-opus-4-6-v1")
		assert.Empty(t, result)
	})

	t.Run("no injection when no features detected", func(t *testing.T) {
		body := []byte(`{"messages":[{"role":"user","content":"hi"}],"max_tokens":100}`)
		result := autoInjectBedrockBetaTokens(nil, body, "us.anthropic.claude-opus-4-6-v1")
		assert.Empty(t, result)
	})

	t.Run("preserves existing tokens", func(t *testing.T) {
		body := []byte(`{"thinking":{"type":"enabled"},"messages":[{"role":"user","content":"hi"}]}`)
		existing := []string{"context-1m-2025-08-07", "compact-2026-01-12"}
		result := autoInjectBedrockBetaTokens(existing, body, "us.anthropic.claude-opus-4-6-v1")
		assert.Contains(t, result, "context-1m-2025-08-07")
		assert.Contains(t, result, "compact-2026-01-12")
		assert.Contains(t, result, "interleaved-thinking-2025-05-14")
	})
}

func TestResolveBedrockBetaTokens(t *testing.T) {
	t.Run("body-only tool features resolve to final bedrock tokens", func(t *testing.T) {
		body := []byte(`{"tools":[{"name":"bash","allowed_callers":["code_execution_20250825"]}],"messages":[{"role":"user","content":"hi"}]}`)
		result := ResolveBedrockBetaTokens("", body, "us.anthropic.claude-opus-4-6-v1")
		assert.Contains(t, result, "tool-search-tool-2025-10-19")
		assert.Contains(t, result, "tool-examples-2025-10-29")
	})

	t.Run("unsupported client beta tokens are filtered out", func(t *testing.T) {
		body := []byte(`{"messages":[{"role":"user","content":"hi"}]}`)
		result := ResolveBedrockBetaTokens("interleaved-thinking-2025-05-14,files-api-2025-04-14", body, "us.anthropic.claude-opus-4-6-v1")
		assert.Equal(t, []string{"interleaved-thinking-2025-05-14"}, result)
	})
}

func TestPrepareBedrockRequestBody_AutoBetaInjection(t *testing.T) {
	t.Run("thinking in body auto-injects beta without header", func(t *testing.T) {
		input := `{"messages":[{"role":"user","content":"hi"}],"max_tokens":100,"thinking":{"type":"enabled","budget_tokens":10000}}`
		result, err := PrepareBedrockRequestBody([]byte(input), "us.anthropic.claude-opus-4-6-v1", "")
		require.NoError(t, err)
		arr := gjson.GetBytes(result, "anthropic_beta").Array()
		found := false
		for _, v := range arr {
			if v.String() == "interleaved-thinking-2025-05-14" {
				found = true
			}
		}
		assert.True(t, found, "interleaved-thinking should be auto-injected")
	})

	t.Run("header tokens merged with auto-injected tokens", func(t *testing.T) {
		input := `{"messages":[{"role":"user","content":"hi"}],"max_tokens":100,"thinking":{"type":"enabled","budget_tokens":10000}}`
		result, err := PrepareBedrockRequestBody([]byte(input), "us.anthropic.claude-opus-4-6-v1", "context-1m-2025-08-07")
		require.NoError(t, err)
		arr := gjson.GetBytes(result, "anthropic_beta").Array()
		names := make([]string, len(arr))
		for i, v := range arr {
			names[i] = v.String()
		}
		assert.Contains(t, names, "context-1m-2025-08-07")
		assert.Contains(t, names, "interleaved-thinking-2025-05-14")
	})
}

func TestAdjustBedrockModelRegionPrefix(t *testing.T) {
	tests := []struct {
		name    string
		modelID string
		region  string
		expect  string
	}{
		// US region — no change needed
		{"us region keeps us prefix", "us.anthropic.claude-opus-4-6-v1", "us-east-1", "us.anthropic.claude-opus-4-6-v1"},
		// EU region — replace us → eu
		{"eu region replaces prefix", "us.anthropic.claude-opus-4-6-v1", "eu-west-1", "eu.anthropic.claude-opus-4-6-v1"},
		{"eu region sonnet", "us.anthropic.claude-sonnet-4-6", "eu-central-1", "eu.anthropic.claude-sonnet-4-6"},
		// APAC region — jp and au have dedicated prefixes per AWS docs
		{"jp region (ap-northeast-1)", "us.anthropic.claude-sonnet-4-5-20250929-v1:0", "ap-northeast-1", "jp.anthropic.claude-sonnet-4-5-20250929-v1:0"},
		{"au region (ap-southeast-2)", "us.anthropic.claude-haiku-4-5-20251001-v1:0", "ap-southeast-2", "au.anthropic.claude-haiku-4-5-20251001-v1:0"},
		{"apac region (ap-southeast-1)", "us.anthropic.claude-sonnet-4-5-20250929-v1:0", "ap-southeast-1", "apac.anthropic.claude-sonnet-4-5-20250929-v1:0"},
		// eu → us (user manually set eu prefix, moved to us region)
		{"eu to us", "eu.anthropic.claude-opus-4-6-v1", "us-west-2", "us.anthropic.claude-opus-4-6-v1"},
		// global prefix — replace to match region
		{"global to eu", "global.anthropic.claude-opus-4-6-v1", "eu-west-1", "eu.anthropic.claude-opus-4-6-v1"},
		// No known prefix — leave unchanged
		{"no prefix unchanged", "anthropic.claude-3-5-sonnet-20241022-v2:0", "eu-west-1", "anthropic.claude-3-5-sonnet-20241022-v2:0"},
		// GovCloud — uses independent us-gov prefix
		{"govcloud from us", "us.anthropic.claude-opus-4-6-v1", "us-gov-east-1", "us-gov.anthropic.claude-opus-4-6-v1"},
		{"govcloud already correct", "us-gov.anthropic.claude-opus-4-6-v1", "us-gov-west-1", "us-gov.anthropic.claude-opus-4-6-v1"},
		// Force global (special region value)
		{"force global from us", "us.anthropic.claude-opus-4-6-v1", "global", "global.anthropic.claude-opus-4-6-v1"},
		{"force global from eu", "eu.anthropic.claude-sonnet-4-6", "global", "global.anthropic.claude-sonnet-4-6"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expect, AdjustBedrockModelRegionPrefix(tt.modelID, tt.region))
		})
	}
}
