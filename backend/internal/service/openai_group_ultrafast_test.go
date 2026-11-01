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
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestGroupForcedUltrafastHTTPAndWebSocket(t *testing.T) {
	ctx := context.WithValue(context.Background(), ctxkey.Group, &Group{
		ID: 19, Platform: PlatformOpenAI, Status: StatusActive, Hydrated: true, ForceOpenAIUltrafast: true,
	})
	svc := newOpenAIGatewayServiceWithSettings(t, DefaultOpenAIFastPolicySettings())
	for _, accountType := range []string{AccountTypeOAuth, AccountTypeAPIKey} {
		account := &Account{Platform: PlatformOpenAI, Type: accountType}
		for _, tier := range []string{"", "default", "auto", "priority", "fast", "flex", "ultrafast", "unknown"} {
			t.Run(accountType+"/"+tier, func(t *testing.T) {
				tierField := ""
				if tier != "" {
					tierField = fmt.Sprintf(",\"service_tier\":%q", tier)
				}
				body := []byte(`{"model":"gpt-6.1-sol","input":"test"` + tierField + `}`)
				out, err := svc.applyOpenAIFastPolicyToBody(ctx, account, "gpt-6.1-sol", body)
				require.NoError(t, err)
				require.Equal(t, OpenAIFastTierUltrafast, gjson.GetBytes(out, "service_tier").String())
				require.Equal(t, "test", gjson.GetBytes(out, "input").String())
				frame := []byte(`{"type":"response.create","model":"gpt-6.1-sol","input":[]` + tierField + `}`)
				out, blocked, err := svc.applyOpenAIFastPolicyToWSResponseCreate(ctx, account, "gpt-6.1-sol", frame)
				require.NoError(t, err)
				require.Nil(t, blocked)
				require.Equal(t, OpenAIFastTierUltrafast, gjson.GetBytes(out, "service_tier").String())
			})
		}
	}
}

func TestGroupForcedUltrafastScope(t *testing.T) {
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	for _, tc := range []struct {
		name  string
		group *Group
		want  string
	}{
		{"ultrafast group", &Group{ID: 19, Platform: PlatformOpenAI, Status: StatusActive, Hydrated: true, ForceOpenAIUltrafast: true}, OpenAIFastTierUltrafast},
		{"any group ID", &Group{ID: 22, Platform: PlatformOpenAI, Status: StatusActive, Hydrated: true, ForceOpenAIUltrafast: true}, OpenAIFastTierUltrafast},
		{"composite group", &Group{ID: 3, Platform: PlatformComposite, Status: StatusActive, Hydrated: true, ForceOpenAIUltrafast: true}, OpenAIFastTierUltrafast},
		{"other group", &Group{ID: 2, Platform: PlatformOpenAI, Status: StatusActive, Hydrated: true}, ""},
		{"existing fast group", &Group{ID: 2, Platform: PlatformOpenAI, Status: StatusActive, Hydrated: true, ForceOpenAIFast: true}, OpenAIFastTierPriority},
		{"unhydrated", &Group{ID: 19, Platform: PlatformOpenAI, Status: StatusActive, ForceOpenAIUltrafast: true}, ""},
		{"other platform", &Group{ID: 19, Platform: PlatformAnthropic, Status: StatusActive, Hydrated: true, ForceOpenAIUltrafast: true}, ""},
		{"missing group", nil, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.WithValue(context.Background(), ctxkey.Group, tc.group)
			require.Equal(t, tc.want, openAIGroupForcedServiceTier(ctx, account))
		})
	}
	ctx := context.WithValue(context.Background(), ctxkey.Group, &Group{
		ID: 19, Platform: PlatformOpenAI, Status: StatusActive, Hydrated: true, ForceOpenAIUltrafast: true,
	})
	require.Empty(t, openAIGroupForcedServiceTier(nil, account))
	require.Empty(t, openAIGroupForcedServiceTier(ctx, nil))
	require.Empty(t, openAIGroupForcedServiceTier(ctx, &Account{Platform: PlatformAnthropic}))
}

func TestGroupForcedUltrafastLeavesOtherWebSocketFramesUntouched(t *testing.T) {
	ctx := context.WithValue(context.Background(), ctxkey.Group, &Group{
		ID: 19, Platform: PlatformOpenAI, Status: StatusActive, Hydrated: true, ForceOpenAIUltrafast: true,
	})
	svc := &OpenAIGatewayService{}
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	for _, raw := range []string{`{"type":"response.cancel"}`, `{"type":"conversation.item.create"}`, `{invalid`, `{}`} {
		out, blocked, err := svc.applyOpenAIFastPolicyToWSResponseCreate(ctx, account, "gpt-6.1-sol", []byte(raw))
		require.NoError(t, err)
		require.Nil(t, blocked)
		require.Equal(t, raw, string(out))
	}
}

func TestGroupForcedUltrafastRespectsGlobalPolicy(t *testing.T) {
	ctx := context.WithValue(context.Background(), ctxkey.Group, &Group{
		ID: 19, Platform: PlatformOpenAI, Status: StatusActive, Hydrated: true, ForceOpenAIUltrafast: true,
	})
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	for _, action := range []string{BetaPolicyActionFilter, BetaPolicyActionBlock, OpenAIFastPolicyActionForcePriority} {
		t.Run(action, func(t *testing.T) {
			svc := newOpenAIGatewayServiceWithSettings(t, &OpenAIFastPolicySettings{Rules: []OpenAIFastPolicyRule{{
				ServiceTier: OpenAIFastTierUltrafast, Action: action, Scope: BetaPolicyScopeAll,
			}}})
			body, err := svc.applyOpenAIFastPolicyToBody(ctx, account, "gpt-6.1-sol", []byte(`{"model":"gpt-6.1-sol"}`))
			frame, blocked, wsErr := svc.applyOpenAIFastPolicyToWSResponseCreate(ctx, account, "gpt-6.1-sol", []byte(`{"type":"response.create","model":"gpt-6.1-sol"}`))
			require.NoError(t, wsErr)
			switch action {
			case BetaPolicyActionBlock:
				require.Error(t, err)
				require.NotNil(t, blocked)
			case BetaPolicyActionFilter:
				require.NoError(t, err)
				require.Nil(t, blocked)
				require.False(t, gjson.GetBytes(body, "service_tier").Exists())
				require.False(t, gjson.GetBytes(frame, "service_tier").Exists())
			default:
				require.NoError(t, err)
				require.Nil(t, blocked)
				require.Equal(t, OpenAIFastTierPriority, gjson.GetBytes(body, "service_tier").String())
				require.Equal(t, OpenAIFastTierPriority, gjson.GetBytes(frame, "service_tier").String())
			}
		})
	}
}

func TestGroupForcedUltrafastForwardCapturesActualUpstreamRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, passthrough := range []bool{false, true} {
		for _, tier := range []string{"", "default", "priority", "flex"} {
			t.Run(fmt.Sprintf("passthrough=%v/tier=%s", passthrough, tier), func(t *testing.T) {
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(nil))
				c.Request.Header.Set("User-Agent", "codex_cli_rs/0.146.0")
				tierField := ""
				if tier != "" {
					tierField = fmt.Sprintf(",\"service_tier\":%q", tier)
				}
				body := []byte(`{"model":"gpt-6.1-sol","stream":true,"instructions":"test","input":[{"type":"text","text":"hi"}]` + tierField + `}`)
				upstream := &httpUpstreamRecorder{resp: &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
					Body:       io.NopCloser(strings.NewReader("data: [DONE]\n\n")),
				}}
				svc := newOpenAIGatewayServiceWithSettings(t, DefaultOpenAIFastPolicySettings())
				svc.cfg = &config.Config{}
				svc.httpUpstream = upstream
				account := &Account{
					ID: 123, Name: "test", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Concurrency: 1,
					Credentials: map[string]any{"access_token": "test-token", "chatgpt_account_id": "test-account"},
					Extra: map[string]any{
						"openai_passthrough":                        passthrough,
						"openai_oauth_responses_websockets_v2_mode": OpenAIWSIngressModeOff,
					},
					Status: StatusActive, Schedulable: true, RateMultiplier: f64p(1),
				}
				ctx := context.WithValue(context.Background(), ctxkey.Group, &Group{
					ID: 19, Platform: PlatformOpenAI, Status: StatusActive, Hydrated: true, ForceOpenAIUltrafast: true,
				})
				result, err := svc.Forward(ctx, c, account, body)
				require.NoError(t, err)
				require.NotNil(t, result)
				require.Equal(t, OpenAIFastTierUltrafast, gjson.GetBytes(upstream.lastBody, "service_tier").String())
				require.NotNil(t, result.ServiceTier)
				require.Equal(t, OpenAIFastTierUltrafast, *result.ServiceTier)
				require.Equal(t, "model=gpt-6.1-sol;tier=ultrafast", upstream.lastReq.Header.Get(openAICodexRoutingHintHeader))
			})
		}
	}
}
