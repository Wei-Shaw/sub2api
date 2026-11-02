//go:build unit

package service

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// Every transport and token is synthetic; a cache miss cannot exchange the
// deliberately invalid Vertex private key for a real credential.
func fastPolicyForwardFixture(t *testing.T, kind string, rules ...BetaPolicyRule) (*GatewayService, *Account, *fastTestUpstream) {
	t.Helper()
	svc := fastTestService(t, rules...)
	account := fastTestAccount(kind)
	switch kind {
	case "passthrough":
		account.Type, account.Extra = AccountTypeAPIKey, map[string]any{"anthropic_passthrough": true}
	case AccountTypeServiceAccount:
		account.Credentials["service_account_json"] = `{"project_id":"test-project","client_email":"synthetic@example.invalid","private_key":"not-a-private-key"}`
		key, err := parseVertexServiceAccountKey(account)
		require.NoError(t, err)
		cache := newClaudeTokenCacheStub()
		cache.tokens[vertexServiceAccountCacheKey(account, key)] = "synthetic-token"
		svc.claudeTokenProvider = NewClaudeTokenProvider(nil, cache, nil)
	case AccountTypeBedrock:
		account.Credentials["auth_mode"] = "apikey"
		account.Credentials["region"] = "us-east-1"
		account.Credentials["model_mapping"] = map[string]any{"claude-opus-5": "us.anthropic.claude-opus-5-v1"}
	}
	upstream := &fastTestUpstream{status: http.StatusOK, headers: http.Header{}, payload: fastTestSuccess}
	svc.httpUpstream = upstream
	return svc, account, upstream
}

func TestAnthropicFastPolicyVertexForwardClientAndProviderDecisions(t *testing.T) {
	const client = "claude-sonnet-4-5-20250929"
	const provider = "claude-sonnet-4-5@20250929"
	for _, beta := range []string{claude.BetaFastMode, claude.BetaContext1M} {
		for _, tc := range []struct {
			name, model, action string
			blocked, filtered   bool
		}{
			{"client block", client, BetaPolicyActionBlock, true, false},
			{"provider block", provider, BetaPolicyActionBlock, true, false},
			{"provider filter", provider, BetaPolicyActionFilter, false, true},
			{"client filter does not replace provider decision", client, BetaPolicyActionFilter, false, false},
			{"pass", client, BetaPolicyActionPass, false, false},
		} {
			t.Run(beta+"/"+tc.name, func(t *testing.T) {
				rule := BetaPolicyRule{BetaToken: beta, Scope: BetaPolicyScopeAll, Action: tc.action, ModelWhitelist: []string{tc.model}}
				svc, account, upstream := fastPolicyForwardFixture(t, AccountTypeServiceAccount, rule)
				body := strings.Replace(fastTestBody, "claude-opus-5", client, 1)
				c := fastTestContext()
				if beta != claude.BetaFastMode {
					body = strings.Replace(body, `"speed":"fast",`, "", 1)
					c.Request.Header.Set("anthropic-beta", beta)
				}
				parsed, err := ParseGatewayRequest(NewRequestBodyRef([]byte(body)), PlatformAnthropic)
				require.NoError(t, err)
				result, err := svc.Forward(context.Background(), c, account, parsed)
				if tc.blocked {
					var blocked *BetaBlockedError
					require.ErrorAs(t, err, &blocked)
					require.Zero(t, upstream.calls)
					require.Nil(t, result)
					return
				}
				require.NoError(t, err)
				require.Equal(t, 1, upstream.calls)
				require.Contains(t, upstream.lastReq.URL.Path, provider)
				if beta == claude.BetaFastMode {
					require.Equal(t, !tc.filtered, gjson.GetBytes(upstream.lastBody, "speed").Exists())
					if tc.filtered {
						require.Empty(t, parsed.Speed)
						require.Nil(t, result.ServiceTier)
					}
				} else {
					require.Equal(t, !tc.filtered, containsBetaToken(anthropicBetaHeader(upstream.lastReq.Header), beta))
				}
			})
		}
	}
}

func TestAnthropicFastPolicyRejectsDuplicateSpeedBeforeForward(t *testing.T) {
	for _, count := range []bool{false, true} {
		for _, kind := range []string{AccountTypeAPIKey, AccountTypeOAuth, AccountTypeSetupToken, "passthrough", AccountTypeServiceAccount, AccountTypeBedrock} {
			if count && kind == AccountTypeBedrock { // Endpoint deliberately unsupported.
				continue
			}
			for _, action := range []string{BetaPolicyActionFilter, BetaPolicyActionBlock, BetaPolicyActionPass, "no-rule"} {
				for _, fields := range []string{
					`"speed":"fast","speed":"fast"`,
					`"speed":"standard","speed":"fast"`,
					`"speed":"fast","speed":"standard"`,
					`"speed":"standard","\u0073peed":"fast"`,
					`"\u0073peed":"fast","speed":"standard"`,
					`"\u0073peed":"fast","spe\u0065d":"fast"`,
				} {
					t.Run(fmt.Sprintf("count=%v/%s/%s/%s", count, kind, action, fields), func(t *testing.T) {
						var rules []BetaPolicyRule
						if action != "no-rule" {
							rules = []BetaPolicyRule{fastTestRule(action)}
						}
						svc, account, upstream := fastPolicyForwardFixture(t, kind, rules...)
						body := strings.Replace(fastTestBody, `"speed":"fast"`, fields, 1)
						parsed, err := ParseGatewayRequest(NewRequestBodyRef([]byte(body)), PlatformAnthropic)
						require.NoError(t, err)
						c := fastTestContext()
						if count {
							err = svc.ForwardCountTokens(context.Background(), c, account, parsed)
							require.Equal(t, http.StatusBadRequest, c.Writer.Status())
						} else {
							var result *ForwardResult
							result, err = svc.Forward(context.Background(), c, account, parsed)
							require.Nil(t, result, "rejected input cannot produce billable usage")
						}
						var blocked *BetaBlockedError
						require.ErrorAs(t, err, &blocked)
						require.Equal(t, "duplicate top-level speed fields are not allowed", blocked.Message)
						require.Zero(t, upstream.calls)
						require.Equal(t, body, string(parsed.Body.Bytes()), "reject before any rewrite")
					})
				}
			}
		}
	}
}

func TestAnthropicFastPolicyDuplicateValidationPrecedesCachedDecision(t *testing.T) {
	svc := fastTestService(t)
	account := fastTestAccount(AccountTypeAPIKey)
	ctx := withBetaPolicy(context.Background(), account, betaPolicyResult{})
	policy := svc.requestBetaPolicy(ctx, fastTestContext(), account, "claude-opus-5", []byte(`{"speed":"standard","speed":"fast"}`))
	require.NotNil(t, policy.blockErr)
}

func TestAnthropicFastPolicyOpaqueDuplicateKeysAndEscapedSpeed(t *testing.T) {
	const body = `{"model":"claude-opus-5","\u0073peed":"fast","max_tokens":32,"tools":[{"name":"example","input_schema":{"type":"object","properties":{"speed":{"type":"string"},"speed":{"type":"string"}}}}],"messages":[{"role":"assistant","content":[{"type":"tool_use","id":"toolu_test","name":"example","input":{"speed":"standard","speed":"fast"}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_test","content":"ok"}]}]}`
	for _, kind := range []string{AccountTypeAPIKey, "passthrough"} {
		for _, action := range []string{BetaPolicyActionFilter, BetaPolicyActionPass, "no-rule"} {
			t.Run(kind+"/"+action, func(t *testing.T) {
				var rules []BetaPolicyRule
				if action != "no-rule" {
					rules = []BetaPolicyRule{fastTestRule(action)}
				}
				svc, account, upstream := fastPolicyForwardFixture(t, kind, rules...)
				parsed, err := ParseGatewayRequest(NewRequestBodyRef([]byte(body)), PlatformAnthropic)
				require.NoError(t, err)
				result, err := svc.Forward(context.Background(), fastTestContext(), account, parsed)
				require.NoError(t, err)
				require.Equal(t, 1, upstream.calls)
				require.Contains(t, string(upstream.lastBody), `"speed":"standard","speed":"fast"`)
				require.Contains(t, string(upstream.lastBody), `"speed":{"type":"string"},"speed":{"type":"string"}`)
				require.Equal(t, action != BetaPolicyActionFilter, gjson.GetBytes(upstream.lastBody, "speed").Exists())
				if action == BetaPolicyActionFilter {
					require.Empty(t, parsed.Speed)
					require.Nil(t, result.ServiceTier, "filtered billing uses actual wire speed")
				} else {
					require.NotNil(t, result.ServiceTier)
					require.Equal(t, "fast", *result.ServiceTier)
				}
			})
		}
	}
}

func TestAnthropicFastPolicyMultilineBetaForward(t *testing.T) {
	for _, count := range []bool{false, true} {
		for _, kind := range []string{AccountTypeAPIKey, "passthrough", AccountTypeOAuth} {
			for _, action := range []string{"no-rule", BetaPolicyActionFilter, BetaPolicyActionBlock, BetaPolicyActionPass} {
				for _, override := range []bool{false, true} {
					if override && kind == AccountTypeOAuth { // Overrides are API-key-only.
						continue
					}
					for _, mixedCaseFast := range []bool{false, true} {
						t.Run(fmt.Sprintf("count=%v/%s/%s/override=%v/mixed=%v", count, kind, action, override, mixedCaseFast), func(t *testing.T) {
							var rules []BetaPolicyRule
							if action != "no-rule" {
								rules = []BetaPolicyRule{fastTestRule(action)}
							}
							svc, account, upstream := fastPolicyForwardFixture(t, kind, rules...)
							if override {
								account.Credentials["header_override_enabled"] = true
								account.Credentials["header_overrides"] = map[string]any{"ANTHROPIC-BETA": claude.BetaFastMode + ",override-beta"}
							}
							// No body Fast: only the second header line can trigger policy.
							body := strings.Replace(fastTestBody, `"speed":"fast",`, "", 1)
							parsed, err := ParseGatewayRequest(NewRequestBodyRef([]byte(body)), PlatformAnthropic)
							require.NoError(t, err)
							c := fastTestContext()
							c.Request.Header["Anthropic-Beta"] = []string{"first-beta", claude.BetaFastMode + ",second-beta"}
							c.Request.Header["aNtHrOpIc-BeTa"] = []string{"third-beta", "fourth-beta"}
							if mixedCaseFast {
								c.Request.Header["Anthropic-Beta"][1] = "second-beta"
								c.Request.Header["aNtHrOpIc-BeTa"][1] += "," + claude.BetaFastMode
							}
							c.Request.Header.Set("User-Agent", "claude-cli/2.1.280 (external, cli)")
							before := c.Request.Header.Clone()
							ctx := SetClaudeCodeClient(context.Background(), true)
							if count {
								err = svc.ForwardCountTokens(ctx, c, account, parsed)
							} else {
								_, err = svc.Forward(ctx, c, account, parsed)
							}
							require.Equal(t, before, c.Request.Header)
							if action == BetaPolicyActionBlock {
								var blocked *BetaBlockedError
								require.ErrorAs(t, err, &blocked)
								require.Zero(t, upstream.calls)
								return
							}
							require.NoError(t, err)
							require.Equal(t, 1, upstream.calls)
							beta := anthropicBetaHeader(upstream.lastReq.Header)
							require.Equal(t, action != BetaPolicyActionFilter, containsBetaToken(beta, claude.BetaFastMode))
							for _, token := range []string{"first-beta", "second-beta", "third-beta", "fourth-beta"} {
								require.Equal(t, !override, containsBetaToken(beta, token), token)
							}
							require.Equal(t, override, containsBetaToken(beta, "override-beta"))
						})
					}
				}
			}
		}
	}
}

func TestAnthropicFastPolicyCountTokensVertexKeepsLaterModelDecision(t *testing.T) {
	const client = "claude-sonnet-4-5"
	provider := claude.NormalizeModelID(client)
	require.NotEqual(t, client, provider)
	for _, beta := range []string{claude.BetaFastMode, claude.BetaContext1M} {
		for _, tc := range []struct {
			name, model, action string
			blocked, filtered   bool
		}{
			{"client block", client, BetaPolicyActionBlock, true, false},
			{"later block", provider, BetaPolicyActionBlock, true, false},
			{"later filter", provider, BetaPolicyActionFilter, false, true},
			{"client filter does not overwrite later decision", client, BetaPolicyActionFilter, false, false},
		} {
			t.Run(beta+"/"+tc.name, func(t *testing.T) {
				rule := BetaPolicyRule{BetaToken: beta, Action: tc.action, Scope: BetaPolicyScopeAll, ModelWhitelist: []string{tc.model}}
				svc, account, upstream := fastPolicyForwardFixture(t, AccountTypeServiceAccount, rule)
				body := strings.Replace(fastTestBody, "claude-opus-5", client, 1)
				c := fastTestContext()
				if beta != claude.BetaFastMode {
					body = strings.Replace(body, `"speed":"fast",`, "", 1)
					c.Request.Header.Set("anthropic-beta", beta)
				}
				parsed, err := ParseGatewayRequest(NewRequestBodyRef([]byte(body)), PlatformAnthropic)
				require.NoError(t, err)
				err = svc.ForwardCountTokens(context.Background(), c, account, parsed)
				if tc.blocked {
					var blocked *BetaBlockedError
					require.ErrorAs(t, err, &blocked)
					require.Equal(t, http.StatusBadRequest, c.Writer.Status())
					require.Zero(t, upstream.calls)
					return
				}
				require.NoError(t, err)
				require.Equal(t, 1, upstream.calls)
				require.Equal(t, provider, gjson.GetBytes(upstream.lastBody, "model").String())
				if beta == claude.BetaFastMode {
					require.Equal(t, !tc.filtered, gjson.GetBytes(upstream.lastBody, "speed").Exists())
				} else {
					require.Equal(t, !tc.filtered, containsBetaToken(anthropicBetaHeader(upstream.lastReq.Header), beta))
				}
			})
		}
	}
}

func TestAnthropicFastPolicyMessagesAndCountTokensClientModel(t *testing.T) {
	for _, kind := range []string{AccountTypeAPIKey, "passthrough", AccountTypeOAuth, AccountTypeSetupToken} {
		for _, tc := range []struct{ name, model, action, fallback, want string }{
			{"client filter", "client-alias", BetaPolicyActionFilter, BetaPolicyActionPass, BetaPolicyActionFilter},
			{"client block", "client-alias", BetaPolicyActionBlock, BetaPolicyActionPass, BetaPolicyActionBlock},
			{"client pass", "client-alias", BetaPolicyActionPass, BetaPolicyActionBlock, BetaPolicyActionPass},
			{"fallback filter", "claude-opus-5", BetaPolicyActionPass, BetaPolicyActionFilter, BetaPolicyActionFilter},
			{"fallback block", "claude-opus-5", BetaPolicyActionPass, BetaPolicyActionBlock, BetaPolicyActionBlock},
			{"fallback pass", "claude-opus-5", BetaPolicyActionBlock, BetaPolicyActionPass, BetaPolicyActionPass},
			{"default fallback", "claude-opus-5", BetaPolicyActionBlock, "", BetaPolicyActionPass},
		} {
			for _, count := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/count=%v", kind, tc.name, count), func(t *testing.T) {
					clientModel, upstreamModel := "client-alias", "claude-opus-5"
					if kind == AccountTypeOAuth || kind == AccountTypeSetupToken {
						// OAuth uses the existing short-ID normalization, not an API-key mapping.
						clientModel = "claude-sonnet-4-5"
						upstreamModel = claude.NormalizeModelID(clientModel)
					}
					whitelistModel := upstreamModel
					if tc.model == "client-alias" {
						whitelistModel = clientModel
					}
					rule := fastTestRule(tc.action)
					rule.ModelWhitelist, rule.FallbackAction = []string{whitelistModel}, tc.fallback
					svc, account, upstream := fastPolicyForwardFixture(t, kind, rule)
					if kind == AccountTypeAPIKey || kind == "passthrough" {
						account.Credentials["model_mapping"] = map[string]any{clientModel: upstreamModel}
					}
					body := strings.Replace(fastTestBody, "claude-opus-5", clientModel, 1)
					parsed, err := ParseGatewayRequest(NewRequestBodyRef([]byte(body)), PlatformAnthropic)
					require.NoError(t, err)
					c := fastTestContext()
					var result *ForwardResult
					if count {
						err = svc.ForwardCountTokens(context.Background(), c, account, parsed)
					} else {
						result, err = svc.Forward(context.Background(), c, account, parsed)
					}
					if tc.want == BetaPolicyActionBlock {
						var blocked *BetaBlockedError
						require.ErrorAs(t, err, &blocked)
						require.Zero(t, upstream.calls)
						if count {
							require.Equal(t, http.StatusBadRequest, c.Writer.Status())
						}
						return
					}
					require.NoError(t, err)
					require.Equal(t, 1, upstream.calls)
					require.Equal(t, upstreamModel, gjson.GetBytes(upstream.lastBody, "model").String())
					require.Equal(t, tc.want != BetaPolicyActionFilter, gjson.GetBytes(upstream.lastBody, "speed").Exists())
					if !count {
						// The normalized Sonnet fixture is not a Fast-billable model;
						// API-key fixtures use Opus and retain wire-based Fast billing.
						wantFastTier := tc.want != BetaPolicyActionFilter && (kind == AccountTypeAPIKey || kind == "passthrough")
						require.Equal(t, wantFastTier, result.ServiceTier != nil)
					}
				})
			}
		}
	}
}
