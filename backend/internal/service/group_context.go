package service

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
)

// GroupFromContext 返回认证中间件写入请求 ctx 的、已校验的请求分组（API Key 绑定分组）。
//
// 由 internal/server/middleware/api_key_auth.go 的 setGroupContext 写入 ctxkey.Group，
// 且仅写入通过 IsGroupContextValid 的 group（ID>0 且 Hydrated）。这里再校验一次以容错。
// 无有效分组时返回 nil —— 调用方据此回退到系统默认，保持 account > group > system 三级覆盖语义。
func GroupFromContext(ctx context.Context) *Group {
	if ctx == nil {
		return nil
	}
	if g, ok := ctx.Value(ctxkey.Group).(*Group); ok && IsGroupContextValid(g) {
		return g
	}
	return nil
}
