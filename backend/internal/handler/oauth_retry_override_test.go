//go:build unit

package handler

import (
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func TestOAuthExplicitZeroDisablesSameAccountRetry(t *testing.T) {
	account := &service.Account{
		Type: service.AccountTypeOAuth, Platform: service.PlatformOpenAI,
		Credentials: map[string]any{"pool_mode_retry_count": float64(0)},
	}
	for _, status := range []int{429, 503} {
		failure := &service.UpstreamFailoverError{StatusCode: status, RetryableOnSameAccount: true}
		if status == 429 {
			failure.SameAccountRetryDeadline = time.Now().Add(time.Minute)
		}
		limit := effectiveSameAccountRetryLimit(failure, account)
		if sameAccountRetryAllowed(failure, 0, limit) {
			t.Errorf("status %d: explicit zero must disable same-account retry", status)
		}
	}
}

func TestOAuthExplicitRetryLimitCapsDeadlineRetries(t *testing.T) {
	for _, limit := range []int{1, 3, 10} {
		account := &service.Account{
			Type: service.AccountTypeOAuth, Platform: service.PlatformOpenAI,
			Credentials: map[string]any{"pool_mode_retry_count": limit},
		}
		failure := &service.UpstreamFailoverError{
			StatusCode: 429, RetryableOnSameAccount: true,
			SameAccountRetryDeadline: time.Now().Add(time.Minute),
		}
		effective := effectiveSameAccountRetryLimit(failure, account)
		if !sameAccountRetryAllowed(failure, limit-1, effective) || sameAccountRetryAllowed(failure, limit, effective) {
			t.Errorf("explicit limit %d was not enforced", limit)
		}
	}
}

func TestOAuthUnsetRetryLimitPreservesDeadlineBehavior(t *testing.T) {
	account := &service.Account{Type: service.AccountTypeOAuth, Platform: service.PlatformOpenAI}
	failure := &service.UpstreamFailoverError{
		StatusCode: 429, RetryableOnSameAccount: true,
		SameAccountRetryDeadline: time.Now().Add(time.Minute),
	}
	limit := effectiveSameAccountRetryLimit(failure, account)
	if !sameAccountRetryAllowed(failure, 100, limit) {
		t.Fatal("unset OAuth override must preserve deadline-based retries")
	}
}
