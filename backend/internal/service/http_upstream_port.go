package service

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
)

type httpUpstreamIsolationScopeContextKey struct{}

type httpUpstreamIsolationIdentity struct {
	Scope    string
	UserID   int64
	APIKeyID int64
}

func WithHTTPUpstreamIsolationScope(ctx context.Context, userID int64, apiKeyID int64) context.Context {
	if ctx == nil || userID <= 0 || apiKeyID <= 0 {
		return ctx
	}
	scope := fmt.Sprintf("user:%d|key:%d", userID, apiKeyID)
	return context.WithValue(ctx, httpUpstreamIsolationScopeContextKey{}, httpUpstreamIsolationIdentity{
		Scope:    scope,
		UserID:   userID,
		APIKeyID: apiKeyID,
	})
}

func HTTPUpstreamIsolationScopeFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	switch value := ctx.Value(httpUpstreamIsolationScopeContextKey{}).(type) {
	case httpUpstreamIsolationIdentity:
		return strings.TrimSpace(value.Scope)
	case string:
		// Backward compatibility for contexts created before the typed identity
		// was introduced and for narrow tests that inject the legacy value.
		return strings.TrimSpace(value)
	default:
		return ""
	}
}

// HTTPUpstreamIsolationIDsFromContext exposes the authenticated IDs without
// parsing the stable scope string used by Codex profile affinity and leases.
func HTTPUpstreamIsolationIDsFromContext(ctx context.Context) (userID, apiKeyID int64, ok bool) {
	if ctx == nil {
		return 0, 0, false
	}
	identity, ok := ctx.Value(httpUpstreamIsolationScopeContextKey{}).(httpUpstreamIsolationIdentity)
	if !ok || identity.UserID <= 0 || identity.APIKeyID <= 0 {
		return 0, 0, false
	}
	return identity.UserID, identity.APIKeyID, true
}

// HTTPUpstream 上游 HTTP 请求接口
// 用于向上游 API（Claude、OpenAI、Gemini 等）发送请求
type HTTPUpstream interface {
	// Do 执行 HTTP 请求（不启用 TLS 指纹）
	Do(req *http.Request, proxyURL string, accountID int64, accountConcurrency int) (*http.Response, error)

	// DoWithTLS 执行带 TLS 指纹伪装的 HTTP 请求
	//
	// profile 参数:
	//   - nil: 不启用 TLS 指纹，行为与 Do 方法相同
	//   - non-nil: 使用指定的 Profile 进行 TLS 指纹伪装
	//
	// Profile 由调用方通过 TLSFingerprintProfileService 解析后传入，
	// 支持按账号绑定的数据库 profile 或内置默认 profile。
	DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, profile *tlsfingerprint.Profile) (*http.Response, error)
}
