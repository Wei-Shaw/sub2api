//go:build unit

package service

import (
	"context"
	"net/http"
	"testing"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"github.com/stretchr/testify/require"
)

func TestGrokMediaGenerationEligibility(t *testing.T) {
	forbiddenBilling := &xai.BillingSummary{
		StatusCode:        http.StatusForbidden,
		WeeklyStatusCode:  http.StatusForbidden,
		MonthlyStatusCode: http.StatusForbidden,
	}
	weeklyAllowance := &xai.BillingSummary{
		PeriodType:       "weekly",
		StatusCode:       http.StatusOK,
		WeeklyStatusCode: http.StatusOK,
	}
	weeklyForbidden := &xai.BillingSummary{
		StatusCode:        http.StatusOK,
		WeeklyStatusCode:  http.StatusForbidden,
		MonthlyStatusCode: http.StatusOK,
	}
	monthlyForbidden := &xai.BillingSummary{
		StatusCode:        http.StatusOK,
		WeeklyStatusCode:  http.StatusOK,
		MonthlyStatusCode: http.StatusForbidden,
	}

	tests := []struct {
		name       string
		account    *Account
		want       bool
		wantReason string
	}{
		{name: "nil account", account: nil, want: false, wantReason: "not_grok"},
		{name: "non grok account", account: &Account{Platform: PlatformOpenAI}, want: false, wantReason: "not_grok"},
		{name: "non oauth grok account stays eligible", account: &Account{Platform: PlatformGrok, Type: AccountTypeAPIKey}, want: true, wantReason: "non_oauth"},
		{name: "unobserved oauth preserves legacy routing", account: &Account{Platform: PlatformGrok, Type: AccountTypeOAuth}, want: true, wantReason: "billing_unobserved"},
		{name: "weekly allowance is not treated as weekly subscription", account: &Account{Platform: PlatformGrok, Type: AccountTypeOAuth, Extra: map[string]any{grokBillingExtraKey: weeklyAllowance}}, want: true, wantReason: "eligible"},
		{name: "billing forbidden is rejected", account: &Account{Platform: PlatformGrok, Type: AccountTypeOAuth, Extra: map[string]any{grokBillingExtraKey: forbiddenBilling}}, want: false, wantReason: "billing_forbidden"},
		{name: "weekly billing forbidden is rejected after partial success", account: &Account{Platform: PlatformGrok, Type: AccountTypeOAuth, Extra: map[string]any{grokBillingExtraKey: weeklyForbidden}}, want: false, wantReason: "billing_forbidden"},
		{name: "monthly billing forbidden is rejected after partial success", account: &Account{Platform: PlatformGrok, Type: AccountTypeOAuth, Extra: map[string]any{grokBillingExtraKey: monthlyForbidden}}, want: false, wantReason: "billing_forbidden"},
		{name: "malformed billing observation preserves legacy routing", account: &Account{Platform: PlatformGrok, Type: AccountTypeOAuth, Extra: map[string]any{grokBillingExtraKey: make(chan int)}}, want: true, wantReason: "billing_unobserved"},
		{name: "malformed override falls back to observations", account: &Account{Platform: PlatformGrok, Type: AccountTypeOAuth, Extra: map[string]any{GrokMediaEligibleExtraKey: "false", grokBillingExtraKey: weeklyAllowance}}, want: true, wantReason: "eligible"},
		{name: "explicit disable wins", account: &Account{Platform: PlatformGrok, Type: AccountTypeOAuth, Extra: map[string]any{GrokMediaEligibleExtraKey: false}}, want: false, wantReason: "override_disabled"},
		{name: "explicit enable wins over forbidden probe", account: &Account{Platform: PlatformGrok, Type: AccountTypeOAuth, Extra: map[string]any{GrokMediaEligibleExtraKey: true, grokBillingExtraKey: forbiddenBilling}}, want: true, wantReason: "override_enabled"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, reason := tt.account.GrokMediaGenerationEligibility()
			require.Equal(t, tt.want, got)
			require.Equal(t, tt.wantReason, reason)
		})
	}
}

func TestGrokMediaCapabilityFiltersOnlyGeneration(t *testing.T) {
	account := &Account{
		ID:          1,
		Platform:    PlatformGrok,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Extra:       map[string]any{GrokMediaEligibleExtraKey: false},
	}

	require.True(t, account.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityChatCompletions))
	require.False(t, account.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityGrokMediaGeneration))
	require.False(t, isOpenAICompatibleAccountEligibleForRequest(
		context.Background(), account, PlatformGrok, "grok-imagine-video", false,
		OpenAIEndpointCapabilityGrokMediaGeneration,
	))
}

func TestNormalizeGrokMediaEligibilityExtra(t *testing.T) {
	t.Run("boolean override is accepted", func(t *testing.T) {
		extra, err := normalizeGrokMediaEligibilityExtra(PlatformGrok, map[string]any{GrokMediaEligibleExtraKey: false})

		require.NoError(t, err)
		require.Equal(t, false, extra[GrokMediaEligibleExtraKey])
	})

	t.Run("null clears override", func(t *testing.T) {
		extra, err := normalizeGrokMediaEligibilityExtra(PlatformGrok, map[string]any{GrokMediaEligibleExtraKey: nil})

		require.NoError(t, err)
		require.NotContains(t, extra, GrokMediaEligibleExtraKey)
	})

	t.Run("malformed override is rejected", func(t *testing.T) {
		_, err := normalizeGrokMediaEligibilityExtra(PlatformGrok, map[string]any{GrokMediaEligibleExtraKey: "false"})

		require.Error(t, err)
		require.Equal(t, http.StatusBadRequest, infraerrors.Code(err))
	})

	t.Run("other platforms ignore provider owned value", func(t *testing.T) {
		extra := map[string]any{GrokMediaEligibleExtraKey: "provider-owned"}
		normalized, err := normalizeGrokMediaEligibilityExtra(PlatformOpenAI, extra)

		require.NoError(t, err)
		require.Equal(t, extra, normalized)
	})
}

func TestNormalizeGrokMediaEligibilityUpdateExtra(t *testing.T) {
	account := &Account{Platform: PlatformGrok, Extra: map[string]any{GrokMediaEligibleExtraKey: false}}

	t.Run("omitted override preserves current value", func(t *testing.T) {
		input := &UpdateAccountInput{Extra: map[string]any{"quota_used": float64(1)}}
		normalized, err := normalizeGrokMediaEligibilityUpdateExtra(account, input, map[string]any{"quota_used": float64(1)})

		require.NoError(t, err)
		require.Equal(t, false, normalized[GrokMediaEligibleExtraKey])
	})

	t.Run("null removes current override", func(t *testing.T) {
		input := &UpdateAccountInput{Extra: map[string]any{GrokMediaEligibleExtraKey: nil}}
		normalized, err := normalizeGrokMediaEligibilityUpdateExtra(account, input, map[string]any{GrokMediaEligibleExtraKey: nil})

		require.NoError(t, err)
		require.NotContains(t, normalized, GrokMediaEligibleExtraKey)
		require.Contains(t, input.Extra, GrokMediaEligibleExtraKey)
	})

	t.Run("provided boolean replaces current override", func(t *testing.T) {
		input := &UpdateAccountInput{Extra: map[string]any{GrokMediaEligibleExtraKey: true}}
		normalized, err := normalizeGrokMediaEligibilityUpdateExtra(account, input, map[string]any{GrokMediaEligibleExtraKey: true})

		require.NoError(t, err)
		require.Equal(t, true, normalized[GrokMediaEligibleExtraKey])
	})

	t.Run("malformed override is rejected on update", func(t *testing.T) {
		input := &UpdateAccountInput{Extra: map[string]any{GrokMediaEligibleExtraKey: "false"}}
		_, err := normalizeGrokMediaEligibilityUpdateExtra(account, input, nil)

		require.Error(t, err)
		require.Equal(t, http.StatusBadRequest, infraerrors.Code(err))
	})

	t.Run("non grok update is unchanged", func(t *testing.T) {
		input := &UpdateAccountInput{Extra: map[string]any{GrokMediaEligibleExtraKey: "provider-owned"}}
		normalized := map[string]any{GrokMediaEligibleExtraKey: "provider-owned"}
		got, err := normalizeGrokMediaEligibilityUpdateExtra(&Account{Platform: PlatformOpenAI}, input, normalized)

		require.NoError(t, err)
		require.Equal(t, normalized, got)
	})
}
