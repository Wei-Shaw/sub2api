//go:build unit

package service

import (
	"context"
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestRateLimitService_AnthropicOAuth401CooldownDisabled(t *testing.T) {
	for _, tc := range []struct {
		name     string
		platform string
		minutes  int
		paused   bool
	}{
		{"anthropic_disabled", PlatformAnthropic, 0, false},
		{"anthropic_enabled", PlatformAnthropic, 10, true},
		{"anthropic_negative_fallback", PlatformAnthropic, -1, true},
		{"openai_unchanged", PlatformOpenAI, 0, true},
		{"gemini_unchanged", PlatformGemini, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &rateLimitAccountRepoStub{}
			invalidator := &tokenCacheInvalidatorRecorder{}
			cfg := &config.Config{RateLimit: config.RateLimitConfig{OAuth401CooldownMinutes: tc.minutes}}
			svc := NewRateLimitService(repo, nil, cfg, nil, nil)
			svc.SetTokenCacheInvalidator(invalidator)
			credentials := map[string]any{"access_token": "test-access", "refresh_token": "test-refresh", "expires_at": "1999999999"}
			account := &Account{ID: 1, Platform: tc.platform, Type: AccountTypeOAuth, Status: StatusActive, Credentials: shallowCopyMap(credentials)}
			paused := svc.HandleUpstreamError(context.Background(), account, http.StatusUnauthorized, http.Header{}, []byte(`{"error":{"type":"authentication_error","message":"Invalid authentication credentials"}}`))
			require.Equal(t, tc.paused, paused)
			if tc.paused {
				require.Equal(t, 1, repo.tempCalls)
			} else {
				require.Zero(t, repo.tempCalls)
			}
			require.Zero(t, repo.setErrorCalls)
			require.Zero(t, repo.updateCredentialsCalls)
			require.Zero(t, repo.updateExtraCalls)
			require.Equal(t, credentials, account.Credentials)
			require.Len(t, invalidator.accounts, 1)
		})
	}
}
