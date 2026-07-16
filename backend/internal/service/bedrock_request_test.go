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
		{"us.anthropic.claude-opus-4-8-v1", true},
		{"anthropic.claude-fable-5", true},
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

	betaHeader := "context-1m-2025-08-07, compact-2026-01-12"
	result, err := PrepareBedrockRequestBody([]byte(input), "us.anthropic.claude-opus-4-6-v1", betaHeader)
	require.NoError(t, err)

	// 基本字段
	assert.Equal(t, "bedrock-2023-05-31", gjson.GetBytes(result, "anthropic_version").String())
	assert.False(t, gjson.GetBytes(result, "model").Exists())
	assert.False(t, gjson.GetBytes(result, "stream").Exists())
	assert.Equal(t, int64(16384), gjson.GetBytes(result, "max_tokens").Int())

	// anthropic_beta 应包含所有 beta tokens
	betaArr := gjson.GetBytes(result, "anthropic_beta").Array()
	require.Len(t, betaArr, 2)
	assert.Equal(t, "context-1m-2025-08-07", betaArr[0].String())
	assert.Equal(t, "compact-2026-01-12", betaArr[1].String())

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
		result, err := PrepareBedrockRequestBody([]byte(input), "us.anthropic.claude-opus-4-6-v1", "context-1m-2025-08-07")
		require.NoError(t, err)
		arr := gjson.GetBytes(result, "anthropic_beta").Array()
		require.Len(t, arr, 1)
		assert.Equal(t, "context-1m-2025-08-07", arr[0].String())
	})

	t.Run("multiple beta tokens with spaces", func(t *testing.T) {
		result, err := PrepareBedrockRequestBody([]byte(input), "us.anthropic.claude-opus-4-6-v1", "context-1m-2025-08-07 , compact-2026-01-12 ")
		require.NoError(t, err)
		arr := gjson.GetBytes(result, "anthropic_beta").Array()
		require.Len(t, arr, 2)
		assert.Equal(t, "context-1m-2025-08-07", arr[0].String())
		assert.Equal(t, "compact-2026-01-12", arr[1].String())
	})

	t.Run("json array beta header", func(t *testing.T) {
		result, err := PrepareBedrockRequestBody([]byte(input), "us.anthropic.claude-opus-4-6-v1", `["context-1m-2025-08-07","compact-2026-01-12"]`)
		require.NoError(t, err)
		arr := gjson.GetBytes(result, "anthropic_beta").Array()
		require.Len(t, arr, 2)
		assert.Equal(t, "context-1m-2025-08-07", arr[0].String())
		assert.Equal(t, "compact-2026-01-12", arr[1].String())
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
		tokens := []string{"context-1m-2025-08-07", "compact-2026-01-12", "computer-use-2025-11-24"}
		result := filterBedrockBetaTokens(tokens)
		assert.Equal(t, tokens, result)
	})

	t.Run("unsupported tokens are filtered out", func(t *testing.T) {
		tokens := []string{"context-1m-2025-08-07", "output-128k-2025-02-19", "files-api-2025-04-14", "structured-outputs-2025-11-13"}
		result := filterBedrockBetaTokens(tokens)
		assert.Equal(t, []string{"context-1m-2025-08-07"}, result)
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
			"compact-2026-01-12, output-128k-2025-02-19, files-api-2025-04-14")
		require.NoError(t, err)
		arr := gjson.GetBytes(result, "anthropic_beta").Array()
		require.Len(t, arr, 1)
		assert.Equal(t, "compact-2026-01-12", arr[0].String())
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

func TestPrepareBedrockRequestBodyWithTokens_ContextManagementRequiresSupportedBeta(t *testing.T) {
	modelID := "us.anthropic.claude-opus-4-6-v1"

	t.Run("strips context_management when final tokens omit context-management beta", func(t *testing.T) {
		input := `{
			"messages":[{"role":"user","content":"hi"}],
			"max_tokens":100,
			"context_management":{"edits":[{"type":"clear_thinking_20251015","keep":"all"}]}
		}`
		betaTokens := []string{"context-1m-2025-08-07"}
		originalTokens := append([]string(nil), betaTokens...)

		result, err := PrepareBedrockRequestBodyWithTokens([]byte(input), modelID, betaTokens, false)
		require.NoError(t, err)

		assert.False(t, gjson.GetBytes(result, "context_management").Exists())
		assert.Equal(t, originalTokens, betaTokens)
		assert.Equal(t, originalTokens, bedrockAnthropicBetaNames(result))
	})

	t.Run("leaves body without context_management otherwise intact", func(t *testing.T) {
		input := `{"messages":[{"role":"user","content":"hi"}],"max_tokens":100}`

		result, err := PrepareBedrockRequestBodyWithTokens([]byte(input), modelID, nil, false)
		require.NoError(t, err)

		assert.False(t, gjson.GetBytes(result, "context_management").Exists())
		assert.False(t, gjson.GetBytes(result, "anthropic_beta").Exists())
		assert.Equal(t, "hi", gjson.GetBytes(result, "messages.0.content").String())
		assert.Equal(t, int64(100), gjson.GetBytes(result, "max_tokens").Int())
	})

	t.Run("keeps supported context-management beta and retains field", func(t *testing.T) {
		input := `{
			"messages":[{"role":"user","content":"hi"}],
			"max_tokens":100,
			"context_management":{"edits":[{"type":"clear_thinking_20251015","keep":"all"}]}
		}`

		result, err := PrepareBedrockRequestBodyWithTokens(
			[]byte(input),
			modelID,
			[]string{bedrockContextManagementBetaToken, "context-1m-2025-08-07"},
			false,
		)
		require.NoError(t, err)

		assert.True(t, gjson.GetBytes(result, "context_management").Exists())
		assert.Equal(t, []string{bedrockContextManagementBetaToken, "context-1m-2025-08-07"}, bedrockAnthropicBetaNames(result))
	})
}

func bedrockAnthropicBetaNames(body []byte) []string {
	arr := gjson.GetBytes(body, "anthropic_beta").Array()
	names := make([]string, len(arr))
	for i, token := range arr {
		names[i] = token.String()
	}
	return names
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

	t.Run("default opus 4.8 mapping uses regional Bedrock model id", func(t *testing.T) {
		account := &Account{
			Platform: PlatformAnthropic,
			Type:     AccountTypeBedrock,
			Credentials: map[string]any{
				"aws_region": "eu-west-1",
			},
		}

		modelID, ok := ResolveBedrockModelID(account, "claude-opus-4-8")
		require.True(t, ok)
		assert.Equal(t, "eu.anthropic.claude-opus-4-8-v1", modelID)
	})

	t.Run("默认 Fable 5 映射使用官方 Bedrock 模型 ID", func(t *testing.T) {
		account := &Account{
			Platform: PlatformAnthropic,
			Type:     AccountTypeBedrock,
			Credentials: map[string]any{
				"aws_region": "eu-west-1",
			},
		}

		modelID, ok := ResolveBedrockModelID(account, "claude-fable-5")
		require.True(t, ok)
		assert.Equal(t, "anthropic.claude-fable-5", modelID)
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
	t.Run("no auto-inject for thinking (interleaved-thinking not supported)", func(t *testing.T) {
		body := []byte(`{"thinking":{"type":"enabled","budget_tokens":10000},"messages":[{"role":"user","content":"hi"}]}`)
		result := autoInjectBedrockBetaTokens(nil, body, "us.anthropic.claude-opus-4-6-v1")
		// interleaved-thinking-2025-05-14 已从白名单移除，不应自动注入
		assert.Empty(t, result)
	})

	t.Run("no duplicate when already present", func(t *testing.T) {
		body := []byte(`{"thinking":{"type":"enabled","budget_tokens":10000},"messages":[{"role":"user","content":"hi"}]}`)
		result := autoInjectBedrockBetaTokens([]string{"context-1m-2025-08-07"}, body, "us.anthropic.claude-opus-4-6-v1")
		assert.Equal(t, []string{"context-1m-2025-08-07"}, result)
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
		// interleaved-thinking 不再自动注入
		assert.NotContains(t, result, "interleaved-thinking-2025-05-14")
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
		result := ResolveBedrockBetaTokens("context-1m-2025-08-07,files-api-2025-04-14", body, "us.anthropic.claude-opus-4-6-v1")
		assert.Equal(t, []string{"context-1m-2025-08-07"}, result)
	})
}

func TestPrepareBedrockRequestBody_AutoBetaInjection(t *testing.T) {
	t.Run("thinking in body does not auto-inject beta (not supported)", func(t *testing.T) {
		input := `{"messages":[{"role":"user","content":"hi"}],"max_tokens":100,"thinking":{"type":"enabled","budget_tokens":10000}}`
		result, err := PrepareBedrockRequestBody([]byte(input), "us.anthropic.claude-opus-4-6-v1", "")
		require.NoError(t, err)
		// interleaved-thinking 已从白名单移除，不应自动注入
		assert.False(t, gjson.GetBytes(result, "anthropic_beta").Exists())
	})

	t.Run("header tokens preserved without auto-injection", func(t *testing.T) {
		input := `{"messages":[{"role":"user","content":"hi"}],"max_tokens":100,"thinking":{"type":"enabled","budget_tokens":10000}}`
		result, err := PrepareBedrockRequestBody([]byte(input), "us.anthropic.claude-opus-4-6-v1", "context-1m-2025-08-07")
		require.NoError(t, err)
		arr := gjson.GetBytes(result, "anthropic_beta").Array()
		names := make([]string, len(arr))
		for i, v := range arr {
			names[i] = v.String()
		}
		assert.Contains(t, names, "context-1m-2025-08-07")
		// interleaved-thinking 不再自动注入
		assert.NotContains(t, names, "interleaved-thinking-2025-05-14")
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

func TestIsBedrockOpus47OrNewer(t *testing.T) {
	tests := []struct {
		modelID string
		expect  bool
	}{
		{"us.anthropic.claude-opus-4-8-v1", true},
		{"us.anthropic.claude-opus-4-7-v1", true},
		{"us.anthropic.claude-opus-4-6-v1", false},
		{"us.anthropic.claude-opus-4-5-20251101-v1:0", false},
		{"us.anthropic.claude-opus-5-0-v1", true},
		// Sonnet 4.7 is not Opus → false
		{"us.anthropic.claude-sonnet-4-7-v1", false},
		{"us.anthropic.claude-sonnet-4-6", false},
		// Haiku is not Opus
		{"us.anthropic.claude-haiku-4-5-20251001-v1:0", false},
		// Non-Claude models
		{"amazon.nova-pro-v1", false},
	}
	for _, tt := range tests {
		t.Run(tt.modelID, func(t *testing.T) {
			assert.Equal(t, tt.expect, isBedrockOpus47OrNewer(tt.modelID))
		})
	}
}

func TestSanitizeBedrockThinking(t *testing.T) {
	t.Run("Fable 5 将 enabled 转换为 adaptive 并移除预算", func(t *testing.T) {
		input := `{"thinking":{"type":"enabled","budget_tokens":10000},"messages":[]}`
		result := sanitizeBedrockThinking([]byte(input), "anthropic.claude-fable-5")
		assert.Equal(t, "adaptive", gjson.GetBytes(result, "thinking.type").String())
		assert.False(t, gjson.GetBytes(result, "thinking.budget_tokens").Exists())
	})

	t.Run("Fable 5 adaptive 移除预算", func(t *testing.T) {
		input := `{"thinking":{"type":"adaptive","budget_tokens":10000},"messages":[]}`
		result := sanitizeBedrockThinking([]byte(input), "claude-fable-5")
		assert.Equal(t, "adaptive", gjson.GetBytes(result, "thinking.type").String())
		assert.False(t, gjson.GetBytes(result, "thinking.budget_tokens").Exists())
	})

	t.Run("opus 4.7 converts enabled to adaptive", func(t *testing.T) {
		input := `{"thinking":{"type":"enabled","budget_tokens":10000},"messages":[]}`
		result := sanitizeBedrockThinking([]byte(input), "us.anthropic.claude-opus-4-7-v1")
		assert.Equal(t, "adaptive", gjson.GetBytes(result, "thinking.type").String())
		assert.False(t, gjson.GetBytes(result, "thinking.budget_tokens").Exists())
	})

	t.Run("opus 4.7 keeps adaptive unchanged", func(t *testing.T) {
		input := `{"thinking":{"type":"adaptive"},"messages":[]}`
		result := sanitizeBedrockThinking([]byte(input), "us.anthropic.claude-opus-4-7-v1")
		assert.Equal(t, "adaptive", gjson.GetBytes(result, "thinking.type").String())
	})

	t.Run("opus 4.6 enabled without budget_tokens gets default", func(t *testing.T) {
		input := `{"thinking":{"type":"enabled"},"messages":[]}`
		result := sanitizeBedrockThinking([]byte(input), "us.anthropic.claude-opus-4-6-v1")
		assert.Equal(t, "enabled", gjson.GetBytes(result, "thinking.type").String())
		assert.Equal(t, int64(defaultThinkingBudgetTokens), gjson.GetBytes(result, "thinking.budget_tokens").Int())
	})

	t.Run("opus 4.6 enabled with budget_tokens unchanged", func(t *testing.T) {
		input := `{"thinking":{"type":"enabled","budget_tokens":20000},"messages":[]}`
		result := sanitizeBedrockThinking([]byte(input), "us.anthropic.claude-opus-4-6-v1")
		assert.Equal(t, "enabled", gjson.GetBytes(result, "thinking.type").String())
		assert.Equal(t, int64(20000), gjson.GetBytes(result, "thinking.budget_tokens").Int())
	})

	t.Run("no thinking field unchanged", func(t *testing.T) {
		input := `{"messages":[]}`
		result := sanitizeBedrockThinking([]byte(input), "us.anthropic.claude-opus-4-7-v1")
		assert.JSONEq(t, input, string(result))
	})

	t.Run("sonnet 4.6 enabled without budget_tokens gets default", func(t *testing.T) {
		input := `{"thinking":{"type":"enabled"},"messages":[]}`
		result := sanitizeBedrockThinking([]byte(input), "us.anthropic.claude-sonnet-4-6")
		assert.Equal(t, "enabled", gjson.GetBytes(result, "thinking.type").String())
		assert.Equal(t, int64(defaultThinkingBudgetTokens), gjson.GetBytes(result, "thinking.budget_tokens").Int())
	})
}

func TestSanitizeBedrockToolUseIDs(t *testing.T) {
	t.Run("clean IDs unchanged", func(t *testing.T) {
		input := `{"messages":[{"role":"assistant","content":[{"type":"tool_use","id":"toolu_01AbCdEf","name":"bash","input":{}}]}]}`
		result := sanitizeBedrockToolUseIDs([]byte(input))
		assert.Equal(t, "toolu_01AbCdEf", gjson.GetBytes(result, "messages.0.content.0.id").String())
	})

	t.Run("dots in tool_use ID replaced with underscores", func(t *testing.T) {
		input := `{"messages":[{"role":"assistant","content":[{"type":"tool_use","id":"toolu.01.Ab","name":"bash","input":{}}]}]}`
		result := sanitizeBedrockToolUseIDs([]byte(input))
		assert.Equal(t, "toolu_01_Ab", gjson.GetBytes(result, "messages.0.content.0.id").String())
	})

	t.Run("special chars in tool_use ID sanitized", func(t *testing.T) {
		input := `{"messages":[{"role":"assistant","content":[{"type":"tool_use","id":"toolu:01@Ab#Cd","name":"bash","input":{}}]}]}`
		result := sanitizeBedrockToolUseIDs([]byte(input))
		id := gjson.GetBytes(result, "messages.0.content.0.id").String()
		assert.Regexp(t, `^[a-zA-Z0-9_-]+$`, id)
	})

	t.Run("tool_result tool_use_id sanitized", func(t *testing.T) {
		input := `{"messages":[{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu.01.Ab","content":"ok"}]}]}`
		result := sanitizeBedrockToolUseIDs([]byte(input))
		assert.Equal(t, "toolu_01_Ab", gjson.GetBytes(result, "messages.0.content.0.tool_use_id").String())
	})

	t.Run("mixed clean and dirty IDs", func(t *testing.T) {
		input := `{"messages":[
			{"role":"assistant","content":[{"type":"tool_use","id":"clean_id-123","name":"a","input":{}}]},
			{"role":"user","content":[{"type":"tool_result","tool_use_id":"dirty.id@456","content":"ok"}]},
			{"role":"assistant","content":[{"type":"tool_use","id":"also.dirty","name":"b","input":{}}]}
		]}`
		result := sanitizeBedrockToolUseIDs([]byte(input))
		assert.Equal(t, "clean_id-123", gjson.GetBytes(result, "messages.0.content.0.id").String())
		assert.Equal(t, "dirty_id_456", gjson.GetBytes(result, "messages.1.content.0.tool_use_id").String())
		assert.Equal(t, "also_dirty", gjson.GetBytes(result, "messages.2.content.0.id").String())
	})

	t.Run("no messages unchanged", func(t *testing.T) {
		input := `{"system":[{"type":"text","text":"hi"}]}`
		result := sanitizeBedrockToolUseIDs([]byte(input))
		assert.JSONEq(t, input, string(result))
	})

	t.Run("string content skipped", func(t *testing.T) {
		input := `{"messages":[{"role":"user","content":"plain text"}]}`
		result := sanitizeBedrockToolUseIDs([]byte(input))
		assert.JSONEq(t, input, string(result))
	})

	t.Run("empty ID skipped", func(t *testing.T) {
		input := `{"messages":[{"role":"assistant","content":[{"type":"tool_use","id":"","name":"a","input":{}}]}]}`
		result := sanitizeBedrockToolUseIDs([]byte(input))
		assert.Equal(t, "", gjson.GetBytes(result, "messages.0.content.0.id").String())
	})
}

func TestSanitizeBedrockThinking_EdgeCases(t *testing.T) {
	t.Run("opus 4.7 enabled without budget_tokens converts to adaptive", func(t *testing.T) {
		input := `{"thinking":{"type":"enabled"},"messages":[]}`
		result := sanitizeBedrockThinking([]byte(input), "us.anthropic.claude-opus-4-7-v1")
		assert.Equal(t, "adaptive", gjson.GetBytes(result, "thinking.type").String())
		assert.False(t, gjson.GetBytes(result, "thinking.budget_tokens").Exists())
	})

	t.Run("thinking type disabled unchanged", func(t *testing.T) {
		input := `{"thinking":{"type":"disabled"},"messages":[]}`
		result := sanitizeBedrockThinking([]byte(input), "us.anthropic.claude-opus-4-7-v1")
		assert.Equal(t, "disabled", gjson.GetBytes(result, "thinking.type").String())
	})

	t.Run("thinking type empty string unchanged", func(t *testing.T) {
		input := `{"thinking":{"type":""},"messages":[]}`
		result := sanitizeBedrockThinking([]byte(input), "us.anthropic.claude-opus-4-7-v1")
		assert.JSONEq(t, input, string(result))
	})

	t.Run("thinking is not an object unchanged", func(t *testing.T) {
		input := `{"thinking":true,"messages":[]}`
		result := sanitizeBedrockThinking([]byte(input), "us.anthropic.claude-opus-4-7-v1")
		assert.JSONEq(t, input, string(result))
	})

	t.Run("opus 4.7 adaptive with budget_tokens preserved", func(t *testing.T) {
		input := `{"thinking":{"type":"adaptive","budget_tokens":5000},"messages":[]}`
		result := sanitizeBedrockThinking([]byte(input), "us.anthropic.claude-opus-4-7-v1")
		assert.Equal(t, "adaptive", gjson.GetBytes(result, "thinking.type").String())
		assert.Equal(t, int64(5000), gjson.GetBytes(result, "thinking.budget_tokens").Int())
	})

	// Forward() passes parsed.Model (standard names like "claude-opus-4-7")
	t.Run("standard model name opus 4.7 converts enabled to adaptive", func(t *testing.T) {
		input := `{"thinking":{"type":"enabled","budget_tokens":10000},"messages":[]}`
		result := sanitizeBedrockThinking([]byte(input), "claude-opus-4-7")
		assert.Equal(t, "adaptive", gjson.GetBytes(result, "thinking.type").String())
		assert.False(t, gjson.GetBytes(result, "thinking.budget_tokens").Exists())
	})

	t.Run("standard model name opus 4.6 keeps enabled", func(t *testing.T) {
		input := `{"thinking":{"type":"enabled","budget_tokens":10000},"messages":[]}`
		result := sanitizeBedrockThinking([]byte(input), "claude-opus-4-6")
		assert.Equal(t, "enabled", gjson.GetBytes(result, "thinking.type").String())
		assert.Equal(t, int64(10000), gjson.GetBytes(result, "thinking.budget_tokens").Int())
	})
}

func TestIsBedrockOpus47OrNewer_EdgeCases(t *testing.T) {
	tests := []struct {
		modelID string
		expect  bool
	}{
		{"anthropic.claude-opus-4-8-v1", true},
		{"anthropic.claude-opus-4-7-v1", true},
		{"us.anthropic.claude-opus-4-7-20270101-v1:0", true},
		{"", false},
		// Forward() passes parsed.Model (standard names), not Bedrock IDs
		{"claude-opus-4-8", true},
		{"claude-opus-4-7", true},
		{"claude-opus-4-6", false},
		{"claude-sonnet-4-7", false},
	}
	for _, tt := range tests {
		t.Run(tt.modelID, func(t *testing.T) {
			assert.Equal(t, tt.expect, isBedrockOpus47OrNewer(tt.modelID))
		})
	}
}

func TestPrepareBedrockRequestBodyWithTokens_CCCompat(t *testing.T) {
	input := `{
		"model":"claude-opus-4-6",
		"stream":true,
		"max_tokens":16384,
		"thinking":{"type":"enabled"},
		"messages":[
			{"role":"assistant","content":[{"type":"tool_use","id":"toolu.01.Ab","name":"bash","input":{}}]},
			{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu.01.Ab","content":"ok"}]}
		]
	}`

	t.Run("ccCompat=false skips thinking and toolUseID sanitization", func(t *testing.T) {
		result, err := PrepareBedrockRequestBodyWithTokens([]byte(input), "us.anthropic.claude-opus-4-6-v1", nil, false)
		require.NoError(t, err)
		assert.Equal(t, "enabled", gjson.GetBytes(result, "thinking.type").String())
		assert.False(t, gjson.GetBytes(result, "thinking.budget_tokens").Exists())
		assert.Equal(t, "toolu.01.Ab", gjson.GetBytes(result, "messages.0.content.0.id").String())
	})

	t.Run("ccCompat=true applies thinking fix and toolUseID sanitization (opus 4.6)", func(t *testing.T) {
		result, err := PrepareBedrockRequestBodyWithTokens([]byte(input), "us.anthropic.claude-opus-4-6-v1", nil, true)
		require.NoError(t, err)
		assert.Equal(t, "enabled", gjson.GetBytes(result, "thinking.type").String())
		assert.Equal(t, int64(defaultThinkingBudgetTokens), gjson.GetBytes(result, "thinking.budget_tokens").Int())
		assert.Equal(t, "toolu_01_Ab", gjson.GetBytes(result, "messages.0.content.0.id").String())
		assert.Equal(t, "toolu_01_Ab", gjson.GetBytes(result, "messages.1.content.0.tool_use_id").String())
	})

	t.Run("ccCompat=true converts thinking to adaptive for opus 4.7", func(t *testing.T) {
		result, err := PrepareBedrockRequestBodyWithTokens([]byte(input), "us.anthropic.claude-opus-4-7-v1", nil, true)
		require.NoError(t, err)
		assert.Equal(t, "adaptive", gjson.GetBytes(result, "thinking.type").String())
		assert.False(t, gjson.GetBytes(result, "thinking.budget_tokens").Exists())
		assert.Equal(t, "toolu_01_Ab", gjson.GetBytes(result, "messages.0.content.0.id").String())
	})
}

func TestSanitizeBedrockCCFields(t *testing.T) {
	t.Run("removes service_tier and interface_geo", func(t *testing.T) {
		body := []byte(`{"model":"claude-opus-4-6","service_tier":"standard","interface_geo":"us","messages":[]}`)
		result := sanitizeBedrockCCFields(body)
		assert.False(t, gjson.GetBytes(result, "service_tier").Exists())
		assert.False(t, gjson.GetBytes(result, "interface_geo").Exists())
		assert.True(t, gjson.GetBytes(result, "messages").Exists())
	})

	t.Run("removes context_management", func(t *testing.T) {
		body := []byte(`{"model":"claude-opus-4-6","context_management":{"edits":[{"type":"clear_thinking_20251015","keep":"all"}]},"messages":[]}`)
		result := sanitizeBedrockCCFields(body)
		assert.False(t, gjson.GetBytes(result, "context_management").Exists())
		assert.True(t, gjson.GetBytes(result, "messages").Exists())
	})

	t.Run("injects max_tokens when missing", func(t *testing.T) {
		body := []byte(`{"model":"claude-opus-4-6","messages":[]}`)
		result := sanitizeBedrockCCFields(body)
		assert.Equal(t, int64(defaultCCMaxTokens), gjson.GetBytes(result, "max_tokens").Int())
	})

	t.Run("preserves existing max_tokens", func(t *testing.T) {
		body := []byte(`{"model":"claude-opus-4-6","max_tokens":4096,"messages":[]}`)
		result := sanitizeBedrockCCFields(body)
		assert.Equal(t, int64(4096), gjson.GetBytes(result, "max_tokens").Int())
	})

	t.Run("injects anthropic_version when missing", func(t *testing.T) {
		body := []byte(`{"model":"claude-opus-4-6","messages":[]}`)
		result := sanitizeBedrockCCFields(body)
		assert.Equal(t, "bedrock-2023-05-31", gjson.GetBytes(result, "anthropic_version").String())
	})

	t.Run("preserves existing anthropic_version", func(t *testing.T) {
		body := []byte(`{"model":"claude-opus-4-6","anthropic_version":"bedrock-2023-05-31","messages":[]}`)
		result := sanitizeBedrockCCFields(body)
		assert.Equal(t, "bedrock-2023-05-31", gjson.GetBytes(result, "anthropic_version").String())
	})

	t.Run("no-op when fields already clean", func(t *testing.T) {
		body := []byte(`{"model":"claude-opus-4-6","max_tokens":81920,"anthropic_version":"bedrock-2023-05-31","messages":[]}`)
		result := sanitizeBedrockCCFields(body)
		assert.Equal(t, int64(defaultCCMaxTokens), gjson.GetBytes(result, "max_tokens").Int())
		assert.Equal(t, "bedrock-2023-05-31", gjson.GetBytes(result, "anthropic_version").String())
		assert.False(t, gjson.GetBytes(result, "service_tier").Exists())
		assert.False(t, gjson.GetBytes(result, "interface_geo").Exists())
		assert.False(t, gjson.GetBytes(result, "context_management").Exists())
	})

	t.Run("full CC request sanitization", func(t *testing.T) {
		body := []byte(`{
			"model":"claude-opus-4-6",
			"service_tier":"standard",
			"interface_geo":"us",
			"context_management":{"edits":[{"type":"clear_thinking_20251015","keep":"all"}]},
			"messages":[{"role":"user","content":"hello"}],
			"thinking":{"type":"enabled"}
		}`)
		result := sanitizeBedrockCCFields(body)
		assert.False(t, gjson.GetBytes(result, "service_tier").Exists())
		assert.False(t, gjson.GetBytes(result, "interface_geo").Exists())
		assert.False(t, gjson.GetBytes(result, "context_management").Exists())
		assert.Equal(t, int64(defaultCCMaxTokens), gjson.GetBytes(result, "max_tokens").Int())
		assert.Equal(t, "bedrock-2023-05-31", gjson.GetBytes(result, "anthropic_version").String())
		assert.Equal(t, "enabled", gjson.GetBytes(result, "thinking.type").String())
	})
}

func TestSanitizeBedrockCCBetaTokens(t *testing.T) {
	t.Run("filters unsupported beta tokens", func(t *testing.T) {
		input := `{"anthropic_beta":["prompt-caching-2024-07-31","context-1m-2025-08-07","unsupported-feature"],"messages":[]}`
		result := sanitizeBedrockCCBetaTokens([]byte(input), "claude-opus-4-6")
		beta := gjson.GetBytes(result, "anthropic_beta")
		assert.True(t, beta.Exists())
		assert.True(t, beta.IsArray())
		tokens := beta.Array()
		assert.Equal(t, 1, len(tokens))
		assert.Equal(t, "context-1m-2025-08-07", tokens[0].String())
	})

	t.Run("removes anthropic_beta if all tokens filtered", func(t *testing.T) {
		input := `{"anthropic_beta":["prompt-caching-2024-07-31","unsupported-feature"],"messages":[]}`
		result := sanitizeBedrockCCBetaTokens([]byte(input), "claude-opus-4-6")
		assert.False(t, gjson.GetBytes(result, "anthropic_beta").Exists())
	})

	t.Run("thinking alone does not auto-inject beta tokens", func(t *testing.T) {
		input := `{"anthropic_beta":[],"thinking":{"type":"enabled"},"messages":[]}`
		result := sanitizeBedrockCCBetaTokens([]byte(input), "claude-opus-4-6")
		assert.False(t, gjson.GetBytes(result, "anthropic_beta").Exists())
	})

	t.Run("auto-injects computer-use beta token", func(t *testing.T) {
		input := `{"anthropic_beta":[],"tools":[{"type":"computer_20250124","name":"computer"}],"messages":[]}`
		result := sanitizeBedrockCCBetaTokens([]byte(input), "claude-opus-4-6")
		beta := gjson.GetBytes(result, "anthropic_beta")
		assert.True(t, beta.Exists())
		tokens := beta.Array()
		assert.Equal(t, 1, len(tokens))
		assert.Equal(t, "computer-use-2025-11-24", tokens[0].String())
	})

	t.Run("transforms advanced-tool-use to tool-search-tool", func(t *testing.T) {
		input := `{"anthropic_beta":["advanced-tool-use-2025-11-20"],"messages":[]}`
		result := sanitizeBedrockCCBetaTokens([]byte(input), "claude-opus-4-6")
		beta := gjson.GetBytes(result, "anthropic_beta")
		tokens := beta.Array()
		assert.Equal(t, 2, len(tokens)) // tool-search-tool + tool-examples (auto-associated)
		assert.Contains(t, []string{tokens[0].String(), tokens[1].String()}, "tool-search-tool-2025-10-19")
		assert.Contains(t, []string{tokens[0].String(), tokens[1].String()}, "tool-examples-2025-10-29")
	})

	t.Run("no-op when anthropic_beta not present", func(t *testing.T) {
		input := `{"messages":[]}`
		result := sanitizeBedrockCCBetaTokens([]byte(input), "claude-opus-4-6")
		assert.False(t, gjson.GetBytes(result, "anthropic_beta").Exists())
	})

	t.Run("preserves supported beta tokens", func(t *testing.T) {
		input := `{"anthropic_beta":["computer-use-2025-11-24","context-1m-2025-08-07"],"messages":[]}`
		result := sanitizeBedrockCCBetaTokens([]byte(input), "claude-opus-4-6")
		beta := gjson.GetBytes(result, "anthropic_beta")
		tokens := beta.Array()
		assert.Equal(t, 2, len(tokens))
		assert.Contains(t, []string{tokens[0].String(), tokens[1].String()}, "computer-use-2025-11-24")
		assert.Contains(t, []string{tokens[0].String(), tokens[1].String()}, "context-1m-2025-08-07")
	})
}
