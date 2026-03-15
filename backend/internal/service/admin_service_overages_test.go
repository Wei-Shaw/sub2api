//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type updateAccountOveragesRepoStub struct {
	mockAccountRepoForGemini
	account     *Account
	updateCalls int
}

func (r *updateAccountOveragesRepoStub) GetByID(ctx context.Context, id int64) (*Account, error) {
	return r.account, nil
}

func (r *updateAccountOveragesRepoStub) Update(ctx context.Context, account *Account) error {
	r.updateCalls++
	r.account = account
	return nil
}

func TestUpdateAccount_DisableOveragesClearsAICreditsKey(t *testing.T) {
	accountID := int64(101)
	repo := &updateAccountOveragesRepoStub{
		account: &Account{
			ID:       accountID,
			Platform: PlatformAntigravity,
			Type:     AccountTypeOAuth,
			Status:   StatusActive,
			Extra: map[string]any{
				"allow_overages":   true,
				"mixed_scheduling": true,
				modelRateLimitsKey: map[string]any{
					"claude-sonnet-4-5": map[string]any{
						"rate_limited_at":     "2026-03-15T00:00:00Z",
						"rate_limit_reset_at": "2099-03-15T00:00:00Z",
					},
					creditsExhaustedKey: map[string]any{
						"rate_limited_at":     "2026-03-15T00:00:00Z",
						"rate_limit_reset_at": time.Now().Add(5 * time.Hour).UTC().Format(time.RFC3339),
					},
				},
			},
		},
	}

	svc := &adminServiceImpl{accountRepo: repo}
	updated, err := svc.UpdateAccount(context.Background(), accountID, &UpdateAccountInput{
		Extra: map[string]any{
			"mixed_scheduling": true,
			modelRateLimitsKey: map[string]any{
				"claude-sonnet-4-5": map[string]any{
					"rate_limited_at":     "2026-03-15T00:00:00Z",
					"rate_limit_reset_at": "2099-03-15T00:00:00Z",
				},
				creditsExhaustedKey: map[string]any{
					"rate_limited_at":     "2026-03-15T00:00:00Z",
					"rate_limit_reset_at": time.Now().Add(5 * time.Hour).UTC().Format(time.RFC3339),
				},
			},
		},
	})

	require.NoError(t, err)
	require.NotNil(t, updated)
	require.Equal(t, 1, repo.updateCalls)
	require.False(t, updated.IsOveragesEnabled())

	// 关闭 overages 后，AICredits key 应被清除
	rawLimits, ok := repo.account.Extra[modelRateLimitsKey].(map[string]any)
	if ok {
		_, exists := rawLimits[creditsExhaustedKey]
		require.False(t, exists, "关闭 overages 时应清除 AICredits 限流 key")
	}
	// 普通模型限流应保留
	require.True(t, ok)
	_, exists := rawLimits["claude-sonnet-4-5"]
	require.True(t, exists, "普通模型限流应保留")
}

func TestUpdateAccount_EnableOveragesClearsModelRateLimitsBeforePersist(t *testing.T) {
	accountID := int64(102)
	repo := &updateAccountOveragesRepoStub{
		account: &Account{
			ID:       accountID,
			Platform: PlatformAntigravity,
			Type:     AccountTypeOAuth,
			Status:   StatusActive,
			Extra: map[string]any{
				"mixed_scheduling": true,
				modelRateLimitsKey: map[string]any{
					"claude-sonnet-4-5": map[string]any{
						"rate_limited_at":     "2026-03-15T00:00:00Z",
						"rate_limit_reset_at": "2099-03-15T00:00:00Z",
					},
				},
			},
		},
	}

	svc := &adminServiceImpl{accountRepo: repo}
	updated, err := svc.UpdateAccount(context.Background(), accountID, &UpdateAccountInput{
		Extra: map[string]any{
			"mixed_scheduling": true,
			"allow_overages":   true,
		},
	})

	require.NoError(t, err)
	require.NotNil(t, updated)
	require.Equal(t, 1, repo.updateCalls)
	require.True(t, updated.IsOveragesEnabled())

	_, exists := repo.account.Extra[modelRateLimitsKey]
	require.False(t, exists, "开启 overages 时应在持久化前清掉旧模型限流")
}
