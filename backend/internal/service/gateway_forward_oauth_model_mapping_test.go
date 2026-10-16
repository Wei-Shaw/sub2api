//go:build unit

package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestGatewayService_Forward_AnthropicSetupTokenAppliesAccountModelMapping(t *testing.T) {
	gin.SetMode(gin.TestMode)

	for _, tc := range []struct {
		requested string
		upstream  string
	}{
		{requested: "claude-opus-4-6", upstream: "claude-opus-4-7"},
		{requested: "claude-sonnet-4-6", upstream: "claude-sonnet-4-6"},
	} {
		t.Run(tc.requested, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

			body := []byte(`{"model":"` + tc.requested + `","stream":false,"messages":[{"role":"user","content":[{"type":"text","text":"hello"}]}]}`)
			parsed, err := ParseGatewayRequest(NewRequestBodyRef(body), PlatformAnthropic)
			require.NoError(t, err)

			upstream := &anthropicHTTPUpstreamRecorder{resp: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"id":"msg_1","type":"message","role":"assistant","model":"` + tc.upstream + `","content":[{"type":"text","text":"hi"}],"usage":{"input_tokens":1,"output_tokens":1}}`)),
			}}
			svc := newForwardPartialUsageServiceForTest(upstream)
			account := newAnthropicOAuthAccountForPartialUsageTest()
			account.Type = AccountTypeSetupToken
			account.Credentials["model_mapping"] = map[string]any{
				"claude-opus-4-6":   "claude-opus-4-7",
				"claude-sonnet-4-6": "claude-sonnet-4-6",
			}

			_, err = svc.Forward(context.Background(), c, account, parsed)
			require.NoError(t, err)
			require.Equal(t, tc.upstream, gjson.GetBytes(upstream.lastBody, "model").String())
		})
	}
}

func TestNativeClaude55ValidationUsesOAuthMappedModel(t *testing.T) {
	gin.SetMode(gin.TestMode)

	for _, accountType := range []string{AccountTypeOAuth, AccountTypeSetupToken} {
		for _, endpoint := range []string{"/v1/messages", "/v1/messages/count_tokens"} {
			for _, mapping := range []struct {
				name      string
				requested string
				target    string
				reject    bool
			}{
				{"to_opus55", "claude-opus-4-8", "claude-opus-5-5", true},
				{"short_id_to_opus55", "claude-opus-4-5", "claude-opus-5-5", true},
				{"away_from_opus55", "claude-opus-5-5", "claude-opus-4-8", false},
			} {
				for _, param := range []struct {
					name string
					json string
				}{
					{"enabled_thinking", `"thinking":{"type":"enabled","budget_tokens":1024}`},
					{"disabled_thinking", `"thinking":{"type":"disabled"}`},
					{"forced_any", `"tool_choice":{"type":"any"}`},
					{"forced_tool", `"tool_choice":{"type":"tool","name":"lookup"}`},
				} {
					t.Run(accountType+endpoint+"/"+mapping.name+"/"+param.name, func(t *testing.T) {
						rec := httptest.NewRecorder()
						c, _ := gin.CreateTestContext(rec)
						c.Request = httptest.NewRequest(http.MethodPost, endpoint, nil)
						body := []byte(`{"model":"` + mapping.requested + `","max_tokens":2048,"messages":[{"role":"user","content":"hello"}],"tools":[{"name":"lookup","description":"Look up a value","input_schema":{"type":"object","properties":{}}}],` + param.json + `}`)
						parsed, err := ParseGatewayRequest(NewRequestBodyRef(body), PlatformAnthropic)
						require.NoError(t, err)

						response := `{"id":"msg_1","type":"message","role":"assistant","model":"` + mapping.target + `","content":[{"type":"text","text":"hi"}],"usage":{"input_tokens":1,"output_tokens":1}}`
						if endpoint == "/v1/messages/count_tokens" {
							response = `{"input_tokens":1}`
						}
						upstream := &anthropicHTTPUpstreamRecorder{resp: &http.Response{
							StatusCode: http.StatusOK,
							Header:     http.Header{"Content-Type": []string{"application/json"}},
							Body:       io.NopCloser(strings.NewReader(response)),
						}}
						svc := newForwardPartialUsageServiceForTest(upstream)
						account := newAnthropicOAuthAccountForPartialUsageTest()
						account.Type = accountType
						account.Credentials["model_mapping"] = map[string]any{mapping.requested: mapping.target}

						if endpoint == "/v1/messages/count_tokens" {
							err = svc.ForwardCountTokens(context.Background(), c, account, parsed)
						} else {
							_, err = svc.Forward(context.Background(), c, account, parsed)
						}
						if mapping.reject {
							require.Error(t, err)
							require.Equal(t, http.StatusBadRequest, rec.Code)
							require.Contains(t, rec.Body.String(), "invalid_request_error")
							require.Nil(t, upstream.lastReq, "reject unsupported parameters before sending upstream")
							return
						}
						require.NoError(t, err)
						require.Equal(t, http.StatusOK, rec.Code)
						require.NotNil(t, upstream.lastReq, "parameters accepted by the mapped target must be forwarded")
						require.Equal(t, claude.NormalizeModelID(mapping.target), gjson.GetBytes(upstream.lastBody, "model").String())
						for _, path := range []string{"thinking.type", "tool_choice.type"} {
							if expected := gjson.GetBytes(body, path); expected.Exists() {
								require.Equal(t, expected.String(), gjson.GetBytes(upstream.lastBody, path).String(), "preserve %s for the mapped target", path)
							}
						}
					})
				}
			}
		}
	}
}

func TestNativeOAuthMimicryUsesResolvedModelOnce(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, accountType := range []string{AccountTypeOAuth, AccountTypeSetupToken} {
		for _, endpoint := range []string{"/v1/messages", "/v1/messages/count_tokens"} {
			for _, tc := range []struct {
				name        string
				requested   string
				mapping     map[string]any
				target      string
				temperature bool
			}{
				{
					name:      "short_id_to_opus55",
					requested: "claude-opus-4-5",
					mapping:   map[string]any{"claude-opus-4-5": "claude-opus-5-5"},
					target:    "claude-opus-5-5",
				},
				{
					name:      "no_chained_mapping",
					requested: "public-opus",
					mapping:   map[string]any{"public-opus": "claude-opus-5-5", "claude-opus-5-5": "claude-opus-4-8"},
					target:    "claude-opus-5-5",
				},
				{
					name:      "identity_mapping",
					requested: "claude-opus-5-5",
					mapping:   map[string]any{"claude-opus-5-5": "claude-opus-5-5"},
					target:    "claude-opus-5-5",
				},
				{
					name:      "wildcard_mapping",
					requested: "public-opus-5",
					mapping:   map[string]any{"public-opus-*": "claude-opus-5-5"},
					target:    "claude-opus-5-5",
				},
				{
					name:        "unmapped_short_id",
					requested:   "claude-opus-4-5",
					target:      "claude-opus-4-5-20251101",
					temperature: true,
				},
				{
					name:        "normalized_id_does_not_remap",
					requested:   "claude-opus-4-5",
					mapping:     map[string]any{"claude-opus-4-5-20251101": "claude-opus-5-5"},
					target:      "claude-opus-4-5-20251101",
					temperature: true,
				},
				{
					name:        "away_from_opus55",
					requested:   "claude-opus-5-5",
					mapping:     map[string]any{"claude-opus-5-5": "claude-opus-4-8"},
					target:      "claude-opus-4-8",
					temperature: true,
				},
			} {
				t.Run(accountType+endpoint+"/"+tc.name, func(t *testing.T) {
					rec := httptest.NewRecorder()
					c, _ := gin.CreateTestContext(rec)
					c.Request = httptest.NewRequest(http.MethodPost, endpoint, nil)
					body := []byte(`{"model":"` + tc.requested + `","messages":[{"role":"user","content":"hello"}]}`)
					parsed, err := ParseGatewayRequest(NewRequestBodyRef(body), PlatformAnthropic)
					require.NoError(t, err)
					response := `{"id":"msg_1","type":"message","role":"assistant","model":"` + tc.target + `","content":[{"type":"text","text":"hi"}],"usage":{"input_tokens":1,"output_tokens":1}}`
					if endpoint == "/v1/messages/count_tokens" {
						response = `{"input_tokens":1}`
					}
					upstream := &anthropicHTTPUpstreamRecorder{resp: &http.Response{
						StatusCode: http.StatusOK,
						Header:     http.Header{"Content-Type": []string{"application/json"}},
						Body:       io.NopCloser(strings.NewReader(response)),
					}}
					svc := newForwardPartialUsageServiceForTest(upstream)
					account := newAnthropicOAuthAccountForPartialUsageTest()
					account.Type = accountType
					account.Credentials["model_mapping"] = tc.mapping
					if endpoint == "/v1/messages/count_tokens" {
						err = svc.ForwardCountTokens(context.Background(), c, account, parsed)
					} else {
						var result *ForwardResult
						result, err = svc.Forward(context.Background(), c, account, parsed)
						require.NoError(t, err)
						require.NotNil(t, result)
						require.Equal(t, tc.requested, result.Model, "billing and logging keep the client model")
						require.Equal(t, tc.target, result.UpstreamModel)
					}
					require.NoError(t, err)
					require.Equal(t, http.StatusOK, rec.Code)
					require.NotNil(t, upstream.lastReq)
					require.Equal(t, tc.target, gjson.GetBytes(upstream.lastBody, "model").String())
					temperature := gjson.GetBytes(upstream.lastBody, "temperature")
					// count_tokens strips all generation parameters, regardless of model.
					wantTemperature := tc.temperature && endpoint == "/v1/messages"
					require.Equal(t, wantTemperature, temperature.Exists(), "temperature default must follow the target model and endpoint")
					if wantTemperature {
						require.Equal(t, int64(1), temperature.Int())
					}
				})
			}
		}
	}
}

// Compatibility endpoints must map before conversion and must not chain mappings.
func TestAnthropicOAuthCompatModelMapping(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, accountType := range []string{AccountTypeOAuth, AccountTypeSetupToken} {
		for _, chat := range []bool{false, true} {
			for _, tc := range []struct {
				name, requested, target string
				mapping                 map[string]any
			}{
				{"alias", "public-claude", "claude-opus-5-5", map[string]any{"public-claude": "claude-opus-5-5", "claude-opus-5-5": "claude-opus-4-8"}},
				{"short_target", "public-claude", "claude-opus-4-5-20251101", map[string]any{"public-claude": "claude-opus-4-5"}},
				{"wildcard", "public-opus", "claude-opus-4-8", map[string]any{"public-*": "claude-opus-4-8"}},
				{"unrestricted", "claude-opus-4-5", "claude-opus-4-5-20251101", nil},
			} {
				endpoint := "/v1/responses"
				body := `{"model":"` + tc.requested + `","input":"hello","reasoning":{"effort":"high"}}`
				if chat {
					endpoint = "/v1/chat/completions"
					body = `{"model":"` + tc.requested + `","messages":[{"role":"user","content":"hello"}],"reasoning_effort":"high"}`
				}
				t.Run(accountType+endpoint+"/"+tc.name, func(t *testing.T) {
					rec := httptest.NewRecorder()
					c, _ := gin.CreateTestContext(rec)
					c.Request = httptest.NewRequest(http.MethodPost, endpoint, strings.NewReader(body))
					upstream := &anthropicHTTPUpstreamRecorder{resp: &http.Response{
						StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}},
						Body: io.NopCloser(strings.NewReader(namespaceToolAnthropicStream())),
					}}
					svc := newForwardPartialUsageServiceForTest(upstream)
					account := newAnthropicOAuthAccountForPartialUsageTest()
					account.Type = accountType
					account.Credentials["model_mapping"] = tc.mapping
					var result *ForwardResult
					var err error
					if chat {
						result, err = svc.ForwardAsChatCompletions(context.Background(), c, account, []byte(body), nil)
					} else {
						result, err = svc.ForwardAsResponses(context.Background(), c, account, []byte(body), nil)
					}
					require.NoError(t, err)
					require.Equal(t, tc.requested, result.Model)
					require.Equal(t, tc.target, result.UpstreamModel)
					require.Equal(t, tc.target, gjson.GetBytes(upstream.lastBody, "model").String())
					if tc.target == "claude-opus-5-5" {
						require.Equal(t, "adaptive", gjson.GetBytes(upstream.lastBody, "thinking.type").String())
						require.False(t, gjson.GetBytes(upstream.lastBody, "thinking.budget_tokens").Exists())
					}
				})
			}
		}
	}
}

func TestAnthropicOAuthCompatAliasUsesTargetDuringInitialConversion(t *testing.T) {
	for _, accountType := range []string{AccountTypeOAuth, AccountTypeSetupToken} {
		for _, endpoint := range []string{"/v1/chat/completions", "/v1/responses"} {
			for _, alias := range []string{"gpt-6.1-sol", "gpt-5"} {
				t.Run(accountType+endpoint+"/"+alias, func(t *testing.T) {
					body := []byte(`{"model":"` + alias + `","messages":[{"role":"user","content":"hello"}],"reasoning_effort":"none","temperature":0.4}`)
					rec := httptest.NewRecorder()
					c, _ := gin.CreateTestContext(rec)
					c.Request = httptest.NewRequest(http.MethodPost, endpoint, nil)
					upstream := &anthropicHTTPUpstreamRecorder{resp: &http.Response{
						StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}},
						Body: io.NopCloser(strings.NewReader(namespaceToolAnthropicStream())),
					}}
					svc := newForwardPartialUsageServiceForTest(upstream)
					account := newAnthropicOAuthAccountForPartialUsageTest()
					account.Type = accountType
					account.Credentials["model_mapping"] = map[string]any{alias: "claude-sonnet-4-6", "claude-sonnet-4-6": "claude-opus-5-5"}
					var result *ForwardResult
					var err error
					if endpoint == "/v1/chat/completions" {
						result, err = svc.ForwardAsChatCompletions(context.Background(), c, account, body, nil)
					} else {
						result, err = svc.ForwardAsResponses(context.Background(), c, account, body, nil)
					}
					require.NoError(t, err)
					require.Equal(t, alias, result.Model)
					require.Equal(t, "claude-sonnet-4-6", result.UpstreamModel)
					require.Equal(t, "claude-sonnet-4-6", gjson.GetBytes(upstream.lastBody, "model").String())
					require.Equal(t, 0.4, gjson.GetBytes(upstream.lastBody, "temperature").Float())
				})
			}
		}
	}
}

// Scheduling must inspect the same request name as forwarding. Only the resolved
// upstream target may be normalized; short and dated request IDs are distinct keys.
func TestAnthropicOAuthSchedulingAndForwardingUseRequestModel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cases := []struct {
		name, requested, target string
		mapping                 map[string]any
		allowed                 bool
	}{
		{
			name: "short_id_whitelist", requested: "claude-opus-4-5",
			mapping: map[string]any{"claude-opus-4-5": "claude-opus-4-5"},
			allowed: true, target: "claude-opus-4-5-20251101",
		},
		{
			name: "short_id_mapping", requested: "claude-opus-4-5",
			mapping: map[string]any{"claude-opus-4-5": "claude-sonnet-4-5"},
			allowed: true, target: "claude-sonnet-4-5-20250929",
		},
		{
			name: "long_id_mapping", requested: "claude-opus-4-5-20251101",
			mapping: map[string]any{"claude-opus-4-5-20251101": "claude-haiku-4-5"},
			allowed: true, target: "claude-haiku-4-5-20251001",
		},
		{
			name: "long_id_key_does_not_allow_short_request", requested: "claude-opus-4-5",
			mapping: map[string]any{"claude-opus-4-5-20251101": "claude-sonnet-4-6"},
		},
		{
			name: "short_id_key_does_not_allow_long_request", requested: "claude-opus-4-5-20251101",
			mapping: map[string]any{"claude-opus-4-5": "claude-sonnet-4-6"},
		},
		{
			name: "short_and_long_keys_are_independent", requested: "claude-opus-4-5",
			mapping: map[string]any{
				"claude-opus-4-5": "claude-sonnet-4-6", "claude-opus-4-5-20251101": "claude-opus-4-8",
			},
			allowed: true, target: "claude-sonnet-4-6",
		},
		{
			name: "exact_request_wins_over_wildcard", requested: "claude-opus-4-5",
			mapping: map[string]any{"claude-opus-4-5": "claude-sonnet-4-6", "claude-opus-*": "claude-opus-4-8"},
			allowed: true, target: "claude-sonnet-4-6",
		},
		{
			name: "longest_request_wildcard_wins", requested: "claude-opus-4-5",
			mapping: map[string]any{"claude-*": "claude-opus-4-8", "claude-opus-*": "claude-haiku-4-5"},
			allowed: true, target: "claude-haiku-4-5-20251001",
		},
		{
			name: "wildcard_cannot_match_normalized_request", requested: "claude-opus-4-5",
			mapping: map[string]any{"claude-opus-4-5-2025*": "claude-sonnet-4-6"},
		},
		{
			name: "target_is_not_automatically_allowed", requested: "claude-sonnet-4-6",
			mapping: map[string]any{"public-claude": "claude-sonnet-4-6"},
		},
		{
			name: "normalized_target_is_not_remapped", requested: "public-claude",
			mapping: map[string]any{
				"public-claude": "claude-opus-4-5", "claude-opus-4-5": "claude-sonnet-4-6",
				"claude-opus-4-5-20251101": "claude-opus-4-8",
			},
			allowed: true, target: "claude-opus-4-5-20251101",
		},
		{
			name: "unrestricted_short_id", requested: "claude-opus-4-5",
			allowed: true, target: "claude-opus-4-5-20251101",
		},
		{
			name: "empty_mapping_is_unrestricted", requested: "claude-sonnet-4-6",
			mapping: map[string]any{}, allowed: true, target: "claude-sonnet-4-6",
		},
	}
	for _, accountType := range []string{AccountTypeOAuth, AccountTypeSetupToken} {
		for _, endpoint := range []string{"/v1/messages", "/v1/messages/count_tokens", "/v1/chat/completions", "/v1/responses"} {
			for _, tc := range cases {
				t.Run(accountType+endpoint+"/"+tc.name, func(t *testing.T) {
					body := []byte(`{"model":"` + tc.requested + `","messages":[{"role":"user","content":"hello"}]}`)
					response := `{"id":"msg_1","type":"message","role":"assistant","model":"` + tc.target + `","content":[{"type":"text","text":"hi"}],"usage":{"input_tokens":1,"output_tokens":1}}`
					contentType := "application/json"
					switch endpoint {
					case "/v1/messages/count_tokens":
						response = `{"input_tokens":1}`
					case "/v1/responses":
						body = []byte(`{"model":"` + tc.requested + `","input":"hello"}`)
						fallthrough
					case "/v1/chat/completions":
						response = namespaceToolAnthropicStream()
						contentType = "text/event-stream"
					}
					upstream := &anthropicHTTPUpstreamRecorder{resp: &http.Response{
						StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{contentType}},
						Body: io.NopCloser(strings.NewReader(response)),
					}}
					svc := newForwardPartialUsageServiceForTest(upstream)
					account := newAnthropicOAuthAccountForPartialUsageTest()
					account.Type = accountType
					if tc.mapping != nil {
						account.Credentials["model_mapping"] = tc.mapping
					}
					ctx := context.Background()
					require.Equal(t, tc.allowed, svc.isModelSupportedByAccountWithContext(ctx, account, tc.requested))
					if !tc.allowed {
						return
					}

					rec := httptest.NewRecorder()
					c, _ := gin.CreateTestContext(rec)
					c.Request = httptest.NewRequest(http.MethodPost, endpoint, strings.NewReader(string(body)))
					parsed, err := ParseGatewayRequest(NewRequestBodyRef(body), PlatformAnthropic)
					require.NoError(t, err)
					var result *ForwardResult
					switch endpoint {
					case "/v1/messages":
						result, err = svc.Forward(ctx, c, account, parsed)
					case "/v1/messages/count_tokens":
						err = svc.ForwardCountTokens(ctx, c, account, parsed)
					case "/v1/chat/completions":
						result, err = svc.ForwardAsChatCompletions(ctx, c, account, body, parsed)
					case "/v1/responses":
						result, err = svc.ForwardAsResponses(ctx, c, account, body, parsed)
					}
					require.NoError(t, err)
					require.Equal(t, http.StatusOK, rec.Code)
					require.Equal(t, tc.target, gjson.GetBytes(upstream.lastBody, "model").String())
					if endpoint != "/v1/messages/count_tokens" {
						require.NotNil(t, result)
						require.Equal(t, tc.requested, result.Model, "billing retains the request model")
						require.Equal(t, tc.target, result.UpstreamModel)
					}
				})
			}
		}
	}
}
