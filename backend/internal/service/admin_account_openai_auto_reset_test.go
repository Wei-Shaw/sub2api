package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUpdateAccountPreservesOpenAIAutoResetExpiryAtWhileExpiryEnabled(t *testing.T) {
	accountID := int64(120)
	repo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{
		accountID: {
			ID:       accountID,
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Status:   StatusActive,
			Extra: map[string]any{
				OpenAIAutoResetCreditExpiryEnabledExtraKey:     true,
				OpenAIAutoResetCreditExpiryLeadMinutesExtraKey: float64(10),
				OpenAIAutoResetCreditStateExtraKey:             map[string]any{"status": "available"},
				OpenAIAutoResetCreditExpiryAtExtraKey:          "2026-10-04T05:34:23Z",
			},
		},
	}}
	svc := &adminServiceImpl{accountRepo: repo}

	updated, err := svc.UpdateAccount(context.Background(), accountID, &UpdateAccountInput{
		Extra: map[string]any{
			OpenAIAutoResetCreditExpiryEnabledExtraKey:     true,
			OpenAIAutoResetCreditExpiryLeadMinutesExtraKey: float64(30),
		},
	})
	require.NoError(t, err)
	require.Equal(t, "2026-10-04T05:34:23Z", updated.Extra[OpenAIAutoResetCreditExpiryAtExtraKey])
	require.Contains(t, updated.Extra, OpenAIAutoResetCreditStateExtraKey)

	updated, err = svc.UpdateAccount(context.Background(), accountID, &UpdateAccountInput{
		Extra: map[string]any{
			OpenAIAutoResetCreditExpiryEnabledExtraKey: false,
		},
	})
	require.NoError(t, err)
	require.NotContains(t, updated.Extra, OpenAIAutoResetCreditExpiryAtExtraKey)
	require.Contains(t, updated.Extra, OpenAIAutoResetCreditStateExtraKey)
}
