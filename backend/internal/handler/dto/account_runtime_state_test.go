package dto

import (
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func TestAccountFromServiceShallow_OmitsRateLimitFieldsForExhaustedOpenAI(t *testing.T) {
	t.Parallel()

	limitedAt := time.Now().Add(-5 * time.Minute)
	resetAt := time.Now().Add(30 * time.Minute)
	account := &service.Account{
		ID:               66,
		Name:             "exhausted-openai",
		Platform:         service.PlatformOpenAI,
		Type:             service.AccountTypeOAuth,
		Status:           service.StatusActive,
		Schedulable:      true,
		RateLimitedAt:    &limitedAt,
		RateLimitResetAt: &resetAt,
		Extra: map[string]any{
			"codex_7d_used_percent":      100.0,
			"codex_primary_used_percent": 100.0,
		},
	}

	mapped := AccountFromServiceShallow(account)
	if mapped == nil {
		t.Fatalf("expected mapped account")
	}
	if mapped.RateLimitedAt != nil {
		t.Fatalf("expected exhausted openai account not to expose rate_limited_at")
	}
	if mapped.RateLimitResetAt != nil {
		t.Fatalf("expected exhausted openai account not to expose rate_limit_reset_at")
	}
}

func TestAccountFromServiceShallow_KeepsRateLimitFieldsForTemporaryRateLimit(t *testing.T) {
	t.Parallel()

	limitedAt := time.Now().Add(-5 * time.Minute)
	resetAt := time.Now().Add(30 * time.Minute)
	account := &service.Account{
		ID:               64,
		Name:             "active-openai",
		Platform:         service.PlatformOpenAI,
		Type:             service.AccountTypeOAuth,
		Status:           service.StatusActive,
		Schedulable:      true,
		RateLimitedAt:    &limitedAt,
		RateLimitResetAt: &resetAt,
		Extra: map[string]any{
			"codex_7d_used_percent":      4.0,
			"codex_primary_used_percent": 34.0,
		},
	}

	mapped := AccountFromServiceShallow(account)
	if mapped == nil {
		t.Fatalf("expected mapped account")
	}
	if mapped.RateLimitedAt == nil {
		t.Fatalf("expected temporary rate-limited account to keep rate_limited_at")
	}
	if mapped.RateLimitResetAt == nil {
		t.Fatalf("expected temporary rate-limited account to keep rate_limit_reset_at")
	}
}
