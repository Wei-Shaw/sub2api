//go:build unit

package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

func ctxWithEffectivePlatform(t *testing.T, platform string) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	if platform != "" {
		req = req.WithContext(context.WithValue(req.Context(), ctxkey.EffectiveGroupPlatform, platform))
	}
	c.Request = req
	return c
}

// ctxRoutedTo 模拟中间件跨组路由命中后的 ctx：目标分组对象进 gin ctx、平台快照进 request ctx。
func ctxRoutedTo(t *testing.T, target *service.Group) *gin.Context {
	t.Helper()
	c := ctxWithEffectivePlatform(t, target.Platform)
	c.Set(string(middleware2.ContextKeyEffectiveGroup), target)
	return c
}

func anthropicKey() *service.APIKey {
	return &service.APIKey{Group: &service.Group{Platform: service.PlatformAnthropic, AllowMessagesDispatch: false}}
}

// TestAllowMessagesDispatchUsesTargetGroupFlag 跨组路由后按**目标分组**的旗子判：
// 谁的账号在服务，就按谁的策略。源组（聚合组）的旗子跟哪个平台在干活毫无关系。
func TestAllowMessagesDispatchUsesTargetGroupFlag(t *testing.T) {
	target := &service.Group{Platform: service.PlatformOpenAI, AllowMessagesDispatch: true}
	c := ctxRoutedTo(t, target)

	if !allowOpenAICompatibleMessagesDispatch(c, anthropicKey()) {
		t.Fatal("target group opted in (AllowMessagesDispatch=true) -> must be allowed regardless of the source group's flag")
	}
}

// TestAllowMessagesDispatchTargetOptOut 目标组没开旗子就必须挡住 —— 这条比正向那条更
// 重要：它守的是「用户以为在用 Claude、其实被换成别的模型」这个风险不被跨组路由绕过。
func TestAllowMessagesDispatchTargetOptOut(t *testing.T) {
	target := &service.Group{Platform: service.PlatformOpenAI, AllowMessagesDispatch: false}
	c := ctxRoutedTo(t, target)

	if allowOpenAICompatibleMessagesDispatch(c, anthropicKey()) {
		t.Fatal("target group did not opt in -> must block, even though the request was routed there")
	}
}

// TestAllowMessagesDispatchGrokStaysExempt grok 组免检是上游的既定行为
// （10e623f6 "allow grok messages compatibility"）：grok 组开箱即用支持 /v1/messages。
// 这条守住它不被「顺手清理硬编码」的重构干掉 —— 那会破坏现有 grok 用户，并在每次
// 上游同步时跟上游打架。
func TestAllowMessagesDispatchGrokStaysExempt(t *testing.T) {
	target := &service.Group{Platform: service.PlatformGrok, AllowMessagesDispatch: false}
	c := ctxRoutedTo(t, target)

	if !allowOpenAICompatibleMessagesDispatch(c, anthropicKey()) {
		t.Fatal("grok groups are exempt by upstream design (10e623f6) and must stay exempt")
	}
}

// TestAllowMessagesDispatchWithoutRouteKeepsSourceGate 未命中路由时按源组判（源组就是
// 有效组），保持原行为。
func TestAllowMessagesDispatchWithoutRouteKeepsSourceGate(t *testing.T) {
	c := ctxWithEffectivePlatform(t, "")

	if allowOpenAICompatibleMessagesDispatch(c, anthropicKey()) {
		t.Fatal("without a cross-group route the source group's AllowMessagesDispatch=false must still block")
	}
}

// TestRequestPlatformFollowsEffectiveGroup 跨组后必须按 grok 做协议转换，
// 否则请求会被按 OpenAI 协议转换后发给 xAI 上游。
func TestRequestPlatformFollowsEffectiveGroup(t *testing.T) {
	c := ctxWithEffectivePlatform(t, service.PlatformGrok)

	if got := openAICompatibleRequestPlatform(c.Request.Context(), anthropicKey()); got != service.PlatformGrok {
		t.Fatalf("want %q for a request routed to a grok group, got %q", service.PlatformGrok, got)
	}
}

// TestRequestPlatformWithoutRoute 未命中路由时回落源组平台的原行为。
func TestRequestPlatformWithoutRoute(t *testing.T) {
	c := ctxWithEffectivePlatform(t, "")
	openAIKey := &service.APIKey{Group: &service.Group{Platform: service.PlatformOpenAI}}

	if got := openAICompatibleRequestPlatform(c.Request.Context(), openAIKey); got != service.PlatformOpenAI {
		t.Fatalf("want %q without a route, got %q", service.PlatformOpenAI, got)
	}
}

func TestEffectiveGroupPlatformFallback(t *testing.T) {
	cases := []struct {
		name      string
		ctxValue  string
		apiKey    *service.APIKey
		wantValue string
	}{
		{"ctx 有值优先", service.PlatformGrok, anthropicKey(), service.PlatformGrok},
		{"ctx 无值回落 apiKey", "", anthropicKey(), service.PlatformAnthropic},
		{"apiKey 为 nil 不 panic", "", nil, ""},
		{"apiKey.Group 为 nil 不 panic", "", &service.APIKey{}, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := ctxWithEffectivePlatform(t, tc.ctxValue)
			if got := effectiveGroupPlatform(c, tc.apiKey); got != tc.wantValue {
				t.Fatalf("effectiveGroupPlatform() = %q, want %q", got, tc.wantValue)
			}
		})
	}
}
