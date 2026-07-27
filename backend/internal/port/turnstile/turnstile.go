// Package turnstile contains the port interface for the Cloudflare
// Turnstile bounded context: human-verification token validation. The
// contract references only a domain DTO so the repository layer can
// implement it without importing internal/service. The service package
// keeps a type alias to the interface so existing call sites and test
// stubs continue to satisfy the contract.
package turnstile

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

// TurnstileVerifier 验证 Turnstile token 的接口。
type TurnstileVerifier interface {
	VerifyToken(ctx context.Context, secretKey, token, remoteIP string) (*domain.TurnstileVerifyResponse, error)
}
