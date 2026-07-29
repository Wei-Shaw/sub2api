//go:build unit

package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

func messagesCtx(t *testing.T, mutate func(ctx context.Context) context.Context) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	if mutate != nil {
		req = req.WithContext(mutate(req.Context()))
	}
	c.Request = req
	return c
}

func anthropicSourceKey() *service.APIKey {
	return &service.APIKey{Group: &service.Group{Platform: service.PlatformAnthropic}}
}

// TestMessagesPlatformFollowsCrossGroupRoute 是这条回归的核心守卫：
// 分发组（源组 platform=anthropic）里发 gemini-* 命中跨组路由后，本次请求必须按 gemini
// 转发。少了这一条，账号调度选出的是 gemini 账号、协议却按 anthropic 走，会拿 gemini 的
// access_token 去打 Anthropic 上游，稳定 401 invalid bearer token。
func TestMessagesPlatformFollowsCrossGroupRoute(t *testing.T) {
	c := messagesCtx(t, func(ctx context.Context) context.Context {
		return context.WithValue(ctx, ctxkey.EffectiveGroupPlatform, service.PlatformGemini)
	})

	if got := resolveMessagesRequestPlatform(c, anthropicSourceKey()); got != service.PlatformGemini {
		t.Fatalf("跨组路由到 gemini 组后应按 %q 转发，实际 %q", service.PlatformGemini, got)
	}
}

// TestMessagesPlatformWithoutRouteKeepsSourceGroup 未命中跨组路由时保持原行为：按源组平台。
func TestMessagesPlatformWithoutRouteKeepsSourceGroup(t *testing.T) {
	c := messagesCtx(t, nil)

	if got := resolveMessagesRequestPlatform(c, anthropicSourceKey()); got != service.PlatformAnthropic {
		t.Fatalf("未命中路由应回落源组 %q，实际 %q", service.PlatformAnthropic, got)
	}
}

// TestMessagesPlatformCompositeWins composite 解析结果优先于跨组路由（上游语义）。
func TestMessagesPlatformCompositeWins(t *testing.T) {
	c := messagesCtx(t, func(ctx context.Context) context.Context {
		ctx = context.WithValue(ctx, ctxkey.EffectiveGroupPlatform, service.PlatformGemini)
		return context.WithValue(ctx, ctxkey.ResolvedTargetPlatform, service.PlatformGrok)
	})

	if got := resolveMessagesRequestPlatform(c, anthropicSourceKey()); got != service.PlatformGrok {
		t.Fatalf("composite 目标平台应优先，期望 %q，实际 %q", service.PlatformGrok, got)
	}
}

// TestMessagesPlatformNilSafety apiKey / Group 为空时不 panic。
func TestMessagesPlatformNilSafety(t *testing.T) {
	c := messagesCtx(t, nil)
	if got := resolveMessagesRequestPlatform(c, nil); got != "" {
		t.Fatalf("apiKey 为 nil 应返回空串，实际 %q", got)
	}
	if got := resolveMessagesRequestPlatform(c, &service.APIKey{}); got != "" {
		t.Fatalf("Group 为 nil 应返回空串，实际 %q", got)
	}
}
