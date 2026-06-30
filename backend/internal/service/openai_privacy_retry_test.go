//go:build unit

package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/imroc/req/v3"
	"github.com/stretchr/testify/require"
)

func TestAdminService_EnsureOpenAIPrivacy_RetriesNonSuccessModes(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		mode string
	}{
		{name: "failed", mode: PrivacyModeFailed},
		{name: "cf_blocked", mode: PrivacyModeCFBlocked},
		{name: "empty", mode: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			privacyCalls := 0
			svc := &adminServiceImpl{
				accountRepo: &mockAccountRepoForGemini{},
				privacyClientFactory: func(proxyURL string) (*req.Client, error) {
					privacyCalls++
					return nil, errors.New("factory failed")
				},
			}

			account := &Account{
				ID:       101,
				Platform: PlatformOpenAI,
				Type:     AccountTypeOAuth,
				Credentials: map[string]any{
					"access_token": "token-1",
				},
				Extra: map[string]any{
					"privacy_mode": tc.mode,
				},
			}

			got := svc.EnsureOpenAIPrivacy(context.Background(), account)

			require.Equal(t, PrivacyModeFailed, got)
			require.Equal(t, 1, privacyCalls)
		})
	}
}

func TestAdminService_ForceOpenAIPrivacy_IdempotentWhenAlreadyTrainingOff(t *testing.T) {
	t.Parallel()

	repo := &mockAccountRepoForGemini{}
	privacyCalls := 0
	svc := &adminServiceImpl{
		accountRepo: repo,
		privacyClientFactory: func(proxyURL string) (*req.Client, error) {
			privacyCalls++
			return nil, errors.New("unexpected privacy call")
		},
	}

	account := &Account{
		ID:       101,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Extra: map[string]any{
			"privacy_mode": PrivacyModeTrainingOff,
		},
	}

	got := svc.ForceOpenAIPrivacy(context.Background(), account)

	require.Equal(t, PrivacyModeTrainingOff, got)
	require.Equal(t, 0, privacyCalls)
	require.Equal(t, 0, repo.updateExtraCalls)
}

func TestAdminService_ForceAntigravityPrivacy_IdempotentWhenAlreadySet(t *testing.T) {
	t.Parallel()

	repo := &mockAccountRepoForGemini{}
	svc := &adminServiceImpl{accountRepo: repo}

	account := &Account{
		ID:       102,
		Platform: PlatformAntigravity,
		Type:     AccountTypeOAuth,
		Extra: map[string]any{
			"privacy_mode": AntigravityPrivacySet,
		},
	}

	got := svc.ForceAntigravityPrivacy(context.Background(), account)

	require.Equal(t, AntigravityPrivacySet, got)
	require.Equal(t, 0, repo.updateExtraCalls)
}

func TestTokenRefreshService_ensureOpenAIPrivacy_RetriesNonSuccessModes(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		TokenRefresh: config.TokenRefreshConfig{
			MaxRetries:          1,
			RetryBackoffSeconds: 0,
		},
	}

	for _, mode := range []string{PrivacyModeFailed, PrivacyModeCFBlocked} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()

			service := NewTokenRefreshService(&tokenRefreshAccountRepo{}, nil, nil, nil, nil, nil, nil, cfg, nil)
			privacyCalls := 0
			service.SetPrivacyDeps(func(proxyURL string) (*req.Client, error) {
				privacyCalls++
				return nil, errors.New("factory failed")
			}, nil)

			account := &Account{
				ID:       202,
				Platform: PlatformOpenAI,
				Type:     AccountTypeOAuth,
				Credentials: map[string]any{
					"access_token": "token-2",
				},
				Extra: map[string]any{
					"privacy_mode": mode,
				},
			}

			service.ensureOpenAIPrivacy(context.Background(), account)

			require.Equal(t, 1, privacyCalls)
		})
	}
}
