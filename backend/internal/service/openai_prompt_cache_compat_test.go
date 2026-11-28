package service

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestSanitizePromptCacheHintsResponsesShape(t *testing.T) {
	body := []byte(`{
		"model":"gpt-6.1-sol",
		"prompt_cache_options":{"mode":"explicit","ttl":"30m"},
		"prompt_cache_key":"keep-key",
		"input":[
			{"type":"message","role":"user","content":[
				{"type":"input_text","text":"hello","prompt_cache_breakpoint":{"mode":"explicit"}},
				{"type":"input_text","text":"world"}
			]},
			{"type":"message","role":"user","content":"plain string"}
		],
		"tools":[{"type":"function","name":"lookup","parameters":{"type":"object","properties":{"prompt_cache_breakpoint":{"type":"string"}}}}]
	}`)

	got, report, err := sanitizePromptCacheHints(body, promptCacheHintPolicy{StripBreakpoints: true, StripOptions: true})

	require.NoError(t, err)
	require.True(t, report.changed())
	require.Equal(t, 1, report.Breakpoints)
	require.True(t, report.Options)
	require.False(t, gjson.GetBytes(got, "prompt_cache_options").Exists())
	require.False(t, gjson.GetBytes(got, "input.0.content.0.prompt_cache_breakpoint").Exists())
	require.Equal(t, "hello", gjson.GetBytes(got, "input.0.content.0.text").String())
	require.Equal(t, "plain string", gjson.GetBytes(got, "input.1.content").String())
	// The cache identity the gateway injects must survive: implicit caching
	// matches through it once the explicit hints are gone.
	require.Equal(t, "keep-key", gjson.GetBytes(got, "prompt_cache_key").String())
	// A tool schema may legitimately declare a property with the same name.
	require.True(t, gjson.GetBytes(got, "tools.0.parameters.properties.prompt_cache_breakpoint").Exists())
}

func TestSanitizePromptCacheHintsChatShape(t *testing.T) {
	body := []byte(`{
		"model":"glm-4.6",
		"prompt_cache_options":{"mode":"explicit"},
		"messages":[
			{"role":"user","prompt_cache_breakpoint":{"mode":"explicit"},"content":[
				{"type":"text","text":"hello","prompt_cache_breakpoint":{"mode":"explicit"}}
			]},
			{"role":"tool","tool_call_id":"call_1","content":"{\"prompt_cache_breakpoint\":true}","function_call":{"arguments":"{\"prompt_cache_breakpoint\":true}"}}
		],
		"tools":[{"type":"function","function":{"name":"lookup","parameters":{"type":"object","properties":{"prompt_cache_breakpoint":{"type":"string"}}}}}]
	}`)

	got, report, err := sanitizePromptCacheHints(body, promptCacheHintPolicy{StripBreakpoints: true, StripOptions: true})

	require.NoError(t, err)
	require.Equal(t, 2, report.Breakpoints)
	require.False(t, gjson.GetBytes(got, "prompt_cache_options").Exists())
	require.False(t, gjson.GetBytes(got, "messages.0.prompt_cache_breakpoint").Exists())
	require.False(t, gjson.GetBytes(got, "messages.0.content.0.prompt_cache_breakpoint").Exists())
	// Argument strings that merely mention the field are content, not schema.
	require.Contains(t, gjson.GetBytes(got, "messages.1.content").String(), "prompt_cache_breakpoint")
	require.Contains(t, gjson.GetBytes(got, "messages.1.function_call.arguments").String(), "prompt_cache_breakpoint")
	require.True(t, gjson.GetBytes(got, "tools.0.function.parameters.properties.prompt_cache_breakpoint").Exists())
}

func TestSanitizePromptCacheHintsDropsExplicitOptionsWithBreakpoints(t *testing.T) {
	// Stripping only the breakpoints while leaving mode=explicit behind would
	// silently disable caching: explicit mode writes no cache without a
	// breakpoint. The options must go with them.
	body := []byte(`{"prompt_cache_options":{"mode":"explicit","ttl":"30m"},"input":[{"content":[{"type":"input_text","text":"hi","prompt_cache_breakpoint":{"mode":"explicit"}}]}]}`)

	got, report, err := sanitizePromptCacheHints(body, promptCacheHintPolicy{StripBreakpoints: true})

	require.NoError(t, err)
	require.Equal(t, 1, report.Breakpoints)
	require.True(t, report.Options)
	require.False(t, gjson.GetBytes(got, "prompt_cache_options").Exists())
}

func TestSanitizePromptCacheHintsKeepsImplicitOptions(t *testing.T) {
	body := []byte(`{"prompt_cache_options":{"mode":"implicit"},"input":"hello"}`)

	got, report, err := sanitizePromptCacheHints(body, promptCacheHintPolicy{StripBreakpoints: true})

	require.NoError(t, err)
	require.False(t, report.changed())
	require.True(t, gjson.GetBytes(got, "prompt_cache_options").Exists())
}

func TestSanitizePromptCacheHintsNoopCases(t *testing.T) {
	t.Run("policy keeps hints", func(t *testing.T) {
		body := []byte(`{"prompt_cache_options":{"mode":"explicit"},"input":[{"content":[{"prompt_cache_breakpoint":{"mode":"explicit"}}]}]}`)
		got, report, err := sanitizePromptCacheHints(body, promptCacheHintPolicy{})
		require.NoError(t, err)
		require.False(t, report.changed())
		require.Equal(t, string(body), string(got))
	})

	t.Run("invalid json", func(t *testing.T) {
		body := []byte(`{"input":`)
		got, report, err := sanitizePromptCacheHints(body, promptCacheHintPolicy{StripBreakpoints: true, StripOptions: true})
		require.NoError(t, err)
		require.False(t, report.changed())
		require.Equal(t, string(body), string(got))
	})

	t.Run("empty body", func(t *testing.T) {
		got, report, err := sanitizePromptCacheHints(nil, promptCacheHintPolicy{StripBreakpoints: true, StripOptions: true})
		require.NoError(t, err)
		require.False(t, report.changed())
		require.Nil(t, got)
	})
}

func TestResolvePromptCacheHintPolicy(t *testing.T) {
	apiKey := func(baseURL string, credentials map[string]any) *Account {
		merged := map[string]any{"api_key": "sk-test", "base_url": baseURL}
		for k, v := range credentials {
			merged[k] = v
		}
		return &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: merged}
	}

	tests := []struct {
		name          string
		account       *Account
		upstreamModel string
		wantStrip     bool
	}{
		{
			name:          "codex oauth upstream never implements explicit caching",
			account:       &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth},
			upstreamModel: "gpt-6.1-sol",
			wantStrip:     true,
		},
		{
			name:          "third party base url",
			account:       apiKey("https://compat.example", nil),
			upstreamModel: "gpt-6.1-sol",
			wantStrip:     true,
		},
		{
			name:          "official api key keeps gpt-5.6 hints",
			account:       apiKey("https://api.openai.com", nil),
			upstreamModel: "gpt-5.6-sol",
			wantStrip:     false,
		},
		{
			name:          "official api key keeps gpt-6 hints",
			account:       apiKey("https://api.openai.com", nil),
			upstreamModel: "gpt-6.1-sol",
			wantStrip:     false,
		},
		{
			name:          "official api key strips hints for models without explicit caching",
			account:       apiKey("https://api.openai.com", nil),
			upstreamModel: "gpt-5.5",
			wantStrip:     true,
		},
		{
			name:          "credential override strips on official api key",
			account:       apiKey("https://api.openai.com", map[string]any{promptCacheHintsCredentialKey: "strip"}),
			upstreamModel: "gpt-6.1-sol",
			wantStrip:     true,
		},
		{
			name:          "credential override keeps hints on third party relay",
			account:       apiKey("https://compat.example", map[string]any{promptCacheHintsCredentialKey: "allow"}),
			upstreamModel: "gpt-6.1-sol",
			wantStrip:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy := resolvePromptCacheHintPolicy(tt.account, tt.upstreamModel)
			require.Equal(t, tt.wantStrip, policy.stripsHints())
			if tt.wantStrip {
				require.True(t, policy.StripBreakpoints)
				require.True(t, policy.StripOptions)
			}
		})
	}

	t.Run("nil account strips", func(t *testing.T) {
		require.True(t, resolvePromptCacheHintPolicy(nil, "gpt-6.1-sol").stripsHints())
	})
}
