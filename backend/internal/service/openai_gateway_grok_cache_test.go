//go:build unit

package service

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func newGrokCacheTestContext(apiKeyID int64) *gin.Context {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	if apiKeyID > 0 {
		c.Set("api_key", &APIKey{ID: apiKeyID, Group: &Group{Platform: PlatformGrok}})
	}
	return c
}

func TestResolveGrokCacheIdentityStableAcrossAppendOnlyTurns(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c := newGrokCacheTestContext(101)
	round1 := []byte(`{"model":"grok","instructions":"be concise","tools":[{"type":"function","name":"lookup","parameters":{"type":"object"}}],"input":[{"role":"user","content":"first question"}]}`)
	round2 := []byte(`{"model":"grok","instructions":"be concise","tools":[{"type":"function","name":"lookup","parameters":{"type":"object"}}],"input":[{"role":"user","content":"first question"},{"role":"assistant","content":"first answer"},{"role":"user","content":"second question"}]}`)

	first := resolveGrokCacheIdentity(c, round1, "", "grok-4.5")
	second := resolveGrokCacheIdentity(c, round2, "", "grok-4.5")

	require.NotEmpty(t, first)
	require.Len(t, first, 36)
	require.Equal(t, first, second)
}

func TestResolveGrokCacheIdentityStableAcrossIndependentPromptsWithSamePrefix(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c := newGrokCacheTestContext(102)
	firstBody := []byte(`{"model":"grok","instructions":"be concise","tools":[{"type":"function","name":"lookup"}],"input":[{"role":"user","content":"Question A"}]}`)
	secondBody := []byte(`{"model":"grok","instructions":"be concise","tools":[{"type":"function","name":"lookup"}],"input":[{"role":"user","content":"Question B"}]}`)

	first := resolveGrokCacheIdentity(c, firstBody, "", "grok-4.5")
	second := resolveGrokCacheIdentity(c, secondBody, "", "grok-4.5")

	require.NotEmpty(t, first)
	require.Equal(t, first, second)
}

func TestResolveGrokCacheIdentityStablePrefixIsolation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	baseBody := []byte(`{"model":"grok","instructions":"be concise","tools":[{"type":"function","name":"lookup"}],"input":[{"role":"system","content":"System A"},{"role":"user","content":"Question A"}]}`)
	differentInstructions := []byte(`{"model":"grok","instructions":"be detailed","tools":[{"type":"function","name":"lookup"}],"input":[{"role":"system","content":"System A"},{"role":"user","content":"Question B"}]}`)
	differentSystem := []byte(`{"model":"grok","instructions":"be concise","tools":[{"type":"function","name":"lookup"}],"input":[{"role":"system","content":"System B"},{"role":"user","content":"Question B"}]}`)
	differentTools := []byte(`{"model":"grok","instructions":"be concise","tools":[{"type":"function","name":"search"}],"input":[{"role":"system","content":"System A"},{"role":"user","content":"Question B"}]}`)

	base := resolveGrokCacheIdentity(newGrokCacheTestContext(103), baseBody, "", "grok-4.5")
	require.NotEqual(t, base, resolveGrokCacheIdentity(newGrokCacheTestContext(104), baseBody, "", "grok-4.5"))
	require.NotEqual(t, base, resolveGrokCacheIdentity(newGrokCacheTestContext(103), baseBody, "", "grok-4.3"))
	require.NotEqual(t, base, resolveGrokCacheIdentity(newGrokCacheTestContext(103), differentInstructions, "", "grok-4.5"))
	require.NotEqual(t, base, resolveGrokCacheIdentity(newGrokCacheTestContext(103), differentSystem, "", "grok-4.5"))
	require.NotEqual(t, base, resolveGrokCacheIdentity(newGrokCacheTestContext(103), differentTools, "", "grok-4.5"))
}

func TestResolveGrokCacheIdentityFallsBackWhenStablePrefixIsEmpty(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c := newGrokCacheTestContext(105)
	firstBody := []byte(`{"model":"grok","tools":[],"input":"Question A"}`)
	secondBody := []byte(`{"model":"grok","tools":[],"input":"Question B"}`)

	first := resolveGrokCacheIdentity(c, firstBody, "", "grok-4.5")
	second := resolveGrokCacheIdentity(c, secondBody, "", "grok-4.5")

	require.NotEmpty(t, first)
	require.NotEmpty(t, second)
	require.NotEqual(t, first, second)
}

func TestResolveGrokCacheIdentitySkipsUnanchoredFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c := newGrokCacheTestContext(106)
	tests := [][]byte{
		[]byte(`{"model":"grok"}`),
		[]byte(`{"model":"grok","messages":[{"role":"assistant","content":"answer"}]}`),
		[]byte(`{"model":"grok","messages":[{"role":"user","content":""}]}`),
		[]byte(`{"model":"grok","input":"  "}`),
	}

	for _, body := range tests {
		require.Empty(t, resolveGrokCacheIdentity(c, body, "", "grok-4.5"))
	}
}

func TestResolveGrokCacheIdentityIsolatesAPIKeyAndMappedModel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"grok","input":"same prompt"}`)

	base := resolveGrokCacheIdentity(newGrokCacheTestContext(201), body, "", "grok-4.5")
	otherTenant := resolveGrokCacheIdentity(newGrokCacheTestContext(202), body, "", "grok-4.5")
	otherModel := resolveGrokCacheIdentity(newGrokCacheTestContext(201), body, "", "grok-4.3")

	require.NotEmpty(t, base)
	require.NotEqual(t, base, otherTenant)
	require.NotEqual(t, base, otherModel)
}

func TestResolveGrokCacheIdentityUsesAndIsolatesNativeConversationHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c := newGrokCacheTestContext(301)
	c.Request.Header.Set(grokConversationIDHeader, "raw-native-conversation")
	body1 := []byte(`{"model":"grok","input":"one"}`)
	body2 := []byte(`{"model":"grok","input":"different body that must not replace the explicit session"}`)

	first := resolveGrokCacheIdentity(c, body1, "body-cache-key", "grok-4.5")
	second := resolveGrokCacheIdentity(c, body2, "another-body-cache-key", "grok-4.5")

	require.Equal(t, "raw-native-conversation", (&OpenAIGatewayService{}).ExtractSessionID(c, body1))
	require.Equal(t, first, second)
	require.NotEqual(t, "raw-native-conversation", first)
	require.NotContains(t, first, "raw-native-conversation")
}

func TestResolveGrokCacheIdentityExplicitHeaderPriority(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"grok","prompt_cache_key":"body-key","input":"hi"}`)
	c := newGrokCacheTestContext(401)
	c.Request.Header.Set(grokConversationIDHeader, "grok-key")
	c.Request.Header.Set("conversation_id", "conversation-key")
	c.Request.Header.Set("session_id", "session-key")

	got := resolveGrokCacheIdentity(c, body, "explicit-argument", "grok-4.5")
	onlySession := newGrokCacheTestContext(401)
	onlySession.Request.Header.Set("session_id", "session-key")
	want := resolveGrokCacheIdentity(onlySession, []byte(`{"model":"grok","input":"unrelated"}`), "", "grok-4.5")

	require.Equal(t, want, got)
}

func TestResolveGrokCacheIdentityFailsClosedWithoutAPIKeyContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c := newGrokCacheTestContext(0)
	c.Request.Header.Set(grokConversationIDHeader, "native-session")

	require.Empty(t, resolveGrokCacheIdentity(c, []byte(`{"model":"grok","input":"hi"}`), "", "grok-4.5"))
	require.Empty(t, resolveGrokCacheIdentity(nil, []byte(`{"model":"grok","prompt_cache_key":"key"}`), "key", "grok-4.5"))
}

func TestGrokConversationHeaderIsScopedToGrokRequestScheduling(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"grok","prompt_cache_key":"body-session","input":"hi"}`)

	grokContext := newGrokCacheTestContext(601)
	grokContext.Request.Header.Set(grokConversationIDHeader, "native-grok-session")
	require.Equal(t, "native-grok-session", (&OpenAIGatewayService{}).ExtractSessionID(grokContext, body))

	openAIContext := newGrokCacheTestContext(601)
	openAIContext.Set("api_key", &APIKey{ID: 601, Group: &Group{Platform: PlatformOpenAI}})
	openAIContext.Request.Header.Set(grokConversationIDHeader, "must-be-ignored")
	require.Equal(t, "body-session", (&OpenAIGatewayService{}).ExtractSessionID(openAIContext, body))

	withoutGrokHeader := newGrokCacheTestContext(601)
	withoutGrokHeader.Set("api_key", &APIKey{ID: 601, Group: &Group{Platform: PlatformOpenAI}})
	require.Equal(t,
		(&OpenAIGatewayService{}).GenerateSessionHash(withoutGrokHeader, body),
		(&OpenAIGatewayService{}).GenerateSessionHash(openAIContext, body),
	)
}

func TestApplyGrokCacheIdentityWritesResponsesBodyAndHeader(t *testing.T) {
	sourceBody := []byte(`{"model":"grok-4.5","prompt_cache_key":"raw-client-key"}`)
	body, err := applyGrokResponsesCacheIdentity(sourceBody, sourceBody, "isolated-id", true)
	require.NoError(t, err)
	require.Equal(t, "isolated-id", gjson.GetBytes(body, "prompt_cache_key").String())
	require.Equal(t, "web_search", gjson.GetBytes(body, "tools.0.type").String())
	require.Equal(t, "x_search", gjson.GetBytes(body, "tools.1.type").String())
	require.Equal(t, grokFreeCacheDisabledToolChoice, gjson.GetBytes(body, "tool_choice").String())

	headers := make(http.Header)
	headers.Set(grokConversationIDHeader, "spoofed-client-value")
	applyGrokCacheHeaders(headers, "isolated-id")
	require.Equal(t, "isolated-id", headers.Get(grokConversationIDHeader))
	applyGrokCacheHeaders(headers, "")
	require.Empty(t, headers.Get(grokConversationIDHeader))

	chatBody, err := stripGrokChatPromptCacheKey(body)
	require.NoError(t, err)
	require.False(t, gjson.GetBytes(chatBody, "prompt_cache_key").Exists())

	unscopedSourceBody := []byte(`{"model":"grok","prompt_cache_key":"raw-client-key"}`)
	unscopedBody, err := applyGrokResponsesCacheIdentity(unscopedSourceBody, unscopedSourceBody, "", true)
	require.NoError(t, err)
	require.False(t, gjson.GetBytes(unscopedBody, "prompt_cache_key").Exists())
	require.False(t, gjson.GetBytes(unscopedBody, "tools").Exists())
	require.False(t, gjson.GetBytes(unscopedBody, "tool_choice").Exists())
}

func TestApplyGrokCacheIdentityAppendsNativeToolsToResponseFunctions(t *testing.T) {
	account := healthyGrokOAuthGatewayTestAccount(901, "access-token")
	account.Credentials["subscription_tier"] = " FREE "
	tests := []struct {
		name           string
		toolChoiceJSON string
		wantChoice     bool
	}{
		{name: "missing tool choice"},
		{name: "automatic tool choice", toolChoiceJSON: `,"tool_choice":"auto"`, wantChoice: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			intentBody := []byte(`{"model":"grok","tools":[{"type":"function","name":"lookup","description":"look up a value","parameters":{"type":"object"}},{"type":"function","name":"save","parameters":{"type":"object"}}]` + tt.toolChoiceJSON + `}`)
			body, err := applyGrokResponsesCacheIdentity(intentBody, intentBody, "isolated-id", true)
			require.NoError(t, err)
			body, err = applyGrokFreeMessagesFunctionToolCacheRoute(body, intentBody, account, "isolated-id")

			require.NoError(t, err)
			require.Equal(t, "isolated-id", gjson.GetBytes(body, "prompt_cache_key").String())
			tools := gjson.GetBytes(body, "tools").Array()
			require.Len(t, tools, 4)
			require.Equal(t, "function", tools[0].Get("type").String())
			require.Equal(t, "lookup", tools[0].Get("name").String())
			require.Equal(t, "function", tools[1].Get("type").String())
			require.Equal(t, "save", tools[1].Get("name").String())
			require.Equal(t, "web_search", tools[2].Get("type").String())
			require.Equal(t, "x_search", tools[3].Get("type").String())
			require.Equal(t, tt.wantChoice, gjson.GetBytes(body, "tool_choice").Exists())
			if tt.wantChoice {
				require.Equal(t, "auto", gjson.GetBytes(body, "tool_choice").String())
			}

			second, err := applyGrokResponsesCacheIdentity(body, intentBody, "isolated-id", true)
			require.NoError(t, err)
			second, err = applyGrokFreeMessagesFunctionToolCacheRoute(second, intentBody, account, "isolated-id")
			require.NoError(t, err)
			require.JSONEq(t, string(body), string(second), "native tools must not be duplicated")
			require.Len(t, gjson.GetBytes(second, "tools").Array(), 4)
		})
	}
}

func TestApplyGrokCacheIdentityRequiresPatchedFunctionTools(t *testing.T) {
	account := healthyGrokOAuthGatewayTestAccount(902, "access-token")
	account.Credentials["subscription_tier"] = "free"
	intentBody := []byte(`{"model":"grok","tools":[{"type":"function","name":"lookup"}],"tool_choice":"auto"}`)
	tests := []struct {
		name        string
		patchedBody string
	}{
		{name: "missing tools", patchedBody: `{"model":"grok-4.5"}`},
		{name: "empty tools", patchedBody: `{"model":"grok-4.5","tools":[]}`},
		{name: "native tools only", patchedBody: `{"model":"grok-4.5","tools":[{"type":"web_search"}]}`},
		{name: "unexpected patched tool", patchedBody: `{"model":"grok-4.5","tools":[{"type":"function","name":"lookup"},{"type":"mcp","name":"server"}]}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			beforeTools := gjson.Get(tt.patchedBody, "tools")
			body, err := applyGrokResponsesCacheIdentity([]byte(tt.patchedBody), intentBody, "isolated-id", true)
			require.NoError(t, err)
			body, err = applyGrokFreeMessagesFunctionToolCacheRoute(body, intentBody, account, "isolated-id")

			require.NoError(t, err)
			require.Equal(t, "isolated-id", gjson.GetBytes(body, "prompt_cache_key").String())
			afterTools := gjson.GetBytes(body, "tools")
			require.Equal(t, beforeTools.Exists(), afterTools.Exists())
			require.Equal(t, beforeTools.Raw, afterTools.Raw)
		})
	}
}

func TestGrokFreeMessagesFunctionToolCacheRouteRequiresKnownFreeTier(t *testing.T) {
	intentBody := []byte(`{"model":"grok","tools":[{"type":"function","name":"lookup"}],"tool_choice":"auto"}`)
	tests := []struct {
		name    string
		account *Account
		wantMix bool
	}{
		{
			name: "free credential tier",
			account: func() *Account {
				a := healthyGrokOAuthGatewayTestAccount(910, "access-token")
				a.Credentials["subscription_tier"] = "free"
				return a
			}(),
			wantMix: true,
		},
		{
			name: "free billing tier",
			account: func() *Account {
				a := healthyGrokOAuthGatewayTestAccount(911, "access-token")
				a.Extra = map[string]any{grokBillingExtraKey: map[string]any{"plan": "FREE"}}
				return a
			}(),
			wantMix: true,
		},
		{
			name: "free successful billing has blank plan",
			account: func() *Account {
				a := healthyGrokOAuthGatewayTestAccount(9111, "access-token")
				a.Extra = map[string]any{grokBillingExtraKey: map[string]any{
					"status_code":        http.StatusOK,
					"source":             "billing_probe",
					"monthly_updated_at": "2026-07-15T05:00:00Z",
				}}
				return a
			}(),
			wantMix: true,
		},
		{
			name: "free rolling token quota",
			account: func() *Account {
				a := healthyGrokOAuthGatewayTestAccount(9112, "access-token")
				a.Extra = map[string]any{grokQuotaSnapshotExtraKey: map[string]any{
					"headers_observed": true,
					"tokens":           map[string]any{"limit": grokFreeRolling24hTokenLimit},
				}}
				return a
			}(),
			wantMix: true,
		},
		{
			name: "supergrok remains unchanged",
			account: func() *Account {
				a := healthyGrokOAuthGatewayTestAccount(912, "access-token")
				a.Credentials["subscription_tier"] = "supergrok"
				return a
			}(),
		},
		{
			name: "paid billing overrides stale free quota",
			account: func() *Account {
				a := healthyGrokOAuthGatewayTestAccount(9121, "access-token")
				a.Extra = map[string]any{
					grokBillingExtraKey: map[string]any{"plan": "SuperGrok", "status_code": http.StatusOK},
					grokQuotaSnapshotExtraKey: map[string]any{
						"headers_observed": true,
						"tokens":           map[string]any{"limit": grokFreeRolling24hTokenLimit},
					},
				}
				return a
			}(),
		},
		{
			name: "partial billing without monthly evidence remains unknown",
			account: func() *Account {
				a := healthyGrokOAuthGatewayTestAccount(9122, "access-token")
				a.Extra = map[string]any{grokBillingExtraKey: map[string]any{
					"status_code":    http.StatusOK,
					"source":         "billing_probe",
					"partial":        true,
					"failed_windows": []string{"monthly"},
				}}
				return a
			}(),
		},
		{
			name:    "unknown tier remains unchanged",
			account: healthyGrokOAuthGatewayTestAccount(913, "access-token"),
		},
		{
			name: "api key remains unchanged",
			account: &Account{
				ID:       914,
				Platform: PlatformGrok,
				Type:     AccountTypeAPIKey,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, err := applyGrokFreeMessagesFunctionToolCacheRoute(intentBody, intentBody, tt.account, "isolated-id")

			require.NoError(t, err)
			tools := gjson.GetBytes(body, "tools").Array()
			if tt.wantMix {
				require.Len(t, tools, 3)
				require.Equal(t, "web_search", tools[1].Get("type").String())
				require.Equal(t, "x_search", tools[2].Get("type").String())
				return
			}
			require.Len(t, tools, 1)
		})
	}
}

func TestGrokFreeMessagesFunctionToolCacheRouteRequiresIdentity(t *testing.T) {
	account := healthyGrokOAuthGatewayTestAccount(915, "access-token")
	account.Credentials["subscription_tier"] = "free"
	body := []byte(`{"model":"grok","tools":[{"type":"function","name":"lookup"}],"tool_choice":"auto"}`)

	patched, err := applyGrokFreeMessagesFunctionToolCacheRoute(body, body, account, "")

	require.NoError(t, err)
	require.JSONEq(t, string(body), string(patched))
	require.Len(t, gjson.GetBytes(patched, "tools").Array(), 1)
}

func TestApplyGrokCacheIdentityPreservesIneligibleClientToolFields(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{
			name: "empty tools array",
			body: `{"model":"grok","tools":[]}`,
		},
		{
			name: "null tools",
			body: `{"model":"grok","tools":null}`,
		},
		{
			name: "tool choice only",
			body: `{"model":"grok","tool_choice":{"type":"function","name":"lookup"}}`,
		},
		{
			name: "null tool choice",
			body: `{"model":"grok","tool_choice":null}`,
		},
		{
			name: "native tool with auto choice",
			body: `{"model":"grok","tools":[{"type":"web_search"}],"tool_choice":"auto"}`,
		},
		{
			name: "function with required choice",
			body: `{"model":"grok","tools":[{"type":"function","name":"lookup"}],"tool_choice":"required"}`,
		},
		{
			name: "function with none choice",
			body: `{"model":"grok","tools":[{"type":"function","name":"lookup"}],"tool_choice":"none"}`,
		},
		{
			name: "function with specific choice",
			body: `{"model":"grok","tools":[{"type":"function","name":"lookup"}],"tool_choice":{"type":"function","name":"lookup"}}`,
		},
		{
			name: "function with object auto choice",
			body: `{"model":"grok","tools":[{"type":"function","name":"lookup"}],"tool_choice":{"type":"auto"}}`,
		},
		{
			name: "function mixed with unsupported tool",
			body: `{"model":"grok","tools":[{"type":"function","name":"lookup"},{"type":"namespace","name":"client_tools"}],"tool_choice":"auto"}`,
		},
		{
			name: "unsupported tool only",
			body: `{"model":"grok","tools":[{"type":"namespace","name":"client_tools"}]}`,
		},
		{
			name: "chat completions function shape",
			body: `{"model":"grok","tools":[{"type":"function","function":{"name":"lookup","parameters":{"type":"object"}}}],"tool_choice":"auto"}`,
		},
		{
			name: "incomplete responses function",
			body: `{"model":"grok","tools":[{"type":"function","parameters":{"type":"object"}}]}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			beforeTools := gjson.Get(tt.body, "tools")
			beforeChoice := gjson.Get(tt.body, "tool_choice")
			body, err := applyGrokResponsesCacheIdentity([]byte(tt.body), []byte(tt.body), "isolated-id", true)
			require.NoError(t, err)
			require.Equal(t, "isolated-id", gjson.GetBytes(body, "prompt_cache_key").String())
			require.Equal(t, beforeTools.Exists(), gjson.GetBytes(body, "tools").Exists())
			require.Equal(t, beforeTools.Raw, gjson.GetBytes(body, "tools").Raw)
			require.Equal(t, beforeChoice.Exists(), gjson.GetBytes(body, "tool_choice").Exists())
			require.Equal(t, beforeChoice.Raw, gjson.GetBytes(body, "tool_choice").Raw)
		})
	}
}

func TestApplyGrokCacheIdentityUsesPreSanitizationToolIntent(t *testing.T) {
	tests := []struct {
		name       string
		intentBody string
	}{
		{
			name:       "unsupported tools removed by sanitizer",
			intentBody: `{"model":"grok","tools":[{"type":"namespace","name":"client_tools"}]}`,
		},
		{
			name:       "tool choice removed with unsupported tool",
			intentBody: `{"model":"grok","tools":[{"type":"namespace","name":"client_tools"}],"tool_choice":{"type":"namespace","name":"client_tools"}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This is the shape apply receives after patchGrokResponsesBody has
			// removed unsupported tools and their associated tool_choice.
			patchedBody := []byte(`{"model":"grok-4.5","input":"hello"}`)
			body, err := applyGrokResponsesCacheIdentity(patchedBody, []byte(tt.intentBody), "isolated-id", true)

			require.NoError(t, err)
			require.Equal(t, "isolated-id", gjson.GetBytes(body, "prompt_cache_key").String())
			require.False(t, gjson.GetBytes(body, "tools").Exists())
			require.False(t, gjson.GetBytes(body, "tool_choice").Exists())
		})
	}
}

func TestApplyGrokCacheIdentityWithoutFreeTierRoutingOnlyWritesIdentity(t *testing.T) {
	sourceBody := []byte(`{"model":"grok-4.5","input":"hello"}`)
	body, err := applyGrokResponsesCacheIdentity(sourceBody, sourceBody, "isolated-id", false)

	require.NoError(t, err)
	require.Equal(t, "isolated-id", gjson.GetBytes(body, "prompt_cache_key").String())
	require.False(t, gjson.GetBytes(body, "tools").Exists())
	require.False(t, gjson.GetBytes(body, "tool_choice").Exists())
}

func TestGrokCompactRequestSkipsCacheIdentityAndNativeTools(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c := newGrokCacheTestContext(701)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses/compact", nil)
	body := []byte(`{"model":"grok","input":"compact this","prompt_cache_key":"raw-client-key"}`)

	identity := resolveGrokCacheIdentity(c, body, "", "grok-4.5")
	patched, err := applyGrokResponsesCacheIdentity(body, body, identity, true)

	require.NoError(t, err)
	require.Empty(t, identity)
	require.False(t, gjson.GetBytes(patched, "prompt_cache_key").Exists())
	require.False(t, gjson.GetBytes(patched, "tools").Exists())
	require.False(t, gjson.GetBytes(patched, "tool_choice").Exists())
}

func TestResolveGrokCacheIdentityConcurrentDeterminism(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const workers = 50
	body := []byte(`{"model":"grok","messages":[{"role":"system","content":"stable"},{"role":"user","content":"hello"}]}`)
	identities := make(chan string, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			identities <- resolveGrokCacheIdentity(newGrokCacheTestContext(501), body, "", "grok-4.5")
		}()
	}
	wg.Wait()
	close(identities)

	var first string
	for identity := range identities {
		if first == "" {
			first = identity
		}
		require.Equal(t, first, identity)
	}
	require.NotEmpty(t, first)
}
