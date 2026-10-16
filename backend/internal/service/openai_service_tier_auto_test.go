package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestCodexClientResponseServiceTierAutoDoesNotChangeBilling(t *testing.T) {
	for _, accountType := range []string{AccountTypeOAuth, AccountTypeSetupToken, AccountTypeAPIKey} {
		for _, requested := range []string{"priority", "fast", "ultrafast", "flex", "", "default", "auto"} {
			t.Run(accountType+"/"+requested, func(t *testing.T) {
				account := &Account{Platform: PlatformOpenAI, Type: accountType}
				before := ResolveOpenAIServiceTierBilling(account, requested, "auto")
				want := "auto"
				if accountType != AccountTypeAPIKey {
					switch requested {
					case "priority", "fast":
						want = "priority"
					case "ultrafast", "flex":
						want = requested
					}
				}
				for _, event := range []string{"response.created", "response.in_progress", "response.completed"} {
					raw := []byte(fmt.Sprintf(`{"type":%q,"response":{"model":"gpt-6.1-sol","service_tier":"auto"}}`, event))
					observer := &upstreamResponseModelObserver{}
					observer.ObserveOpenAI(raw, event)
					client := normalizeOpenAIResponseServiceTier(account, requested, raw)
					require.Equal(t, want, gjson.GetBytes(client, "response.service_tier").String())
					// Existing observation ignores auto (and non-terminal tier echoes),
					// rather than recording the normalized client-facing priority.
					require.Empty(t, observer.ServiceTier())
					require.Equal(t, "auto", gjson.GetBytes(raw, "response.service_tier").String())
				}
				require.Equal(t, before, ResolveOpenAIServiceTierBilling(account, requested, "auto"))
			})
		}
	}
	// auto remains authoritative to the existing billing resolver. Only the
	// client-response helper treats it as a placeholder.
	require.False(t, codexOAuthResponseTierIsNonAuthoritative("auto"))
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	require.Equal(t, "auto", ResolveOpenAIServiceTierBilling(account, "priority", "auto").Billing)
}

func relayAutoLifecycleSSE() string {
	var wire strings.Builder
	for _, event := range []string{"response.created", "response.in_progress"} {
		fmt.Fprintf(&wire, "data: {\"type\":%q,\"response\":%s}\n\n", event, relayTierResponseBody("auto"))
	}
	_, _ = wire.WriteString("data: {\"type\":\"response.output_text.delta\",\"delta\":\"hello\"}\n\n")
	fmt.Fprintf(&wire, "data: {\"type\":\"response.completed\",\"response\":%s}\n\ndata: [DONE]\n\n", relayTierResponseBody("default"))
	return wire.String()
}

func TestOpenAIServiceTierAutoLifecycleResponsesAndChat(t *testing.T) {
	for _, accountType := range []string{AccountTypeOAuth, AccountTypeAPIKey} {
		for _, requested := range []string{"priority", ""} {
			for _, route := range []string{"responses", "passthrough", "chat"} {
				t.Run(accountType+"/"+requested+"/"+route, func(t *testing.T) {
					upstream := &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(relayAutoLifecycleSSE()))}
					var account *Account
					var result *OpenAIForwardResult
					var rec *httptest.ResponseRecorder
					if route == "chat" {
						body := []byte(`{"model":"gpt-6.1-sol","stream":true,"messages":[{"role":"user","content":"hi"}]`)
						if requested != "" {
							body = append(body, []byte(fmt.Sprintf(`,"service_tier":%q`, requested))...)
						}
						body = append(body, '}')
						rec = httptest.NewRecorder()
						c, _ := gin.CreateTestContext(rec)
						c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
						account = &Account{ID: 3, Platform: PlatformOpenAI, Type: accountType, Concurrency: 1,
							Credentials: map[string]any{"api_key": "fixture-key", "access_token": "fixture-token", "chatgpt_account_id": "fixture-account"},
							Extra:       map[string]any{"openai_responses_supported": true}}
						svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: &httpUpstreamRecorder{resp: upstream}, cache: &stubGatewayCache{}}
						var err error
						result, err = svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")
						require.NoError(t, err)
					} else {
						account, result, rec = forwardServiceTierRelayFixture(t, accountType, route == "passthrough", true, requested, upstream)
					}
					require.Equal(t, "default", result.UpstreamResponseServiceTier)
					require.Equal(t, 100, result.Usage.InputTokens)
					require.Equal(t, 10, result.Usage.OutputTokens)
					resolution := ApplyOpenAIServiceTierBillingResolution(account, result)
					if accountType == AccountTypeOAuth && requested == "priority" {
						require.Equal(t, "priority", resolution.Billing)
					}
					payloads := relayTierSSEData(t, rec.Body.String())
					seen := 0
					for _, payload := range payloads {
						path := "response.service_tier"
						if route == "chat" {
							path = "service_tier"
						} else if !gjson.GetBytes(payload, "response").IsObject() {
							continue
						}
						want := "auto"
						if gjson.GetBytes(payload, "type").String() == "response.completed" || (route == "chat" && seen >= 2) {
							want = "default"
						}
						if accountType == AccountTypeOAuth && requested == "priority" {
							want = "priority"
						}
						require.Equal(t, want, gjson.GetBytes(payload, path).String(), string(payload))
						seen++
					}
					if route == "chat" {
						require.GreaterOrEqual(t, seen, 4, "role, text, finish and usage chunks")
					} else {
						require.Equal(t, 3, seen)
					}
				})
			}
		}
	}
}
