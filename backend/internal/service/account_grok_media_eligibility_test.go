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
REDACTED
	weeklyAllowance := &xai.BillingSummary{
		PeriodType:       "weekly",
		StatusCode:       http.StatusOK,
		WeeklyStatusCode: http.StatusOK,
REDACTED

	tests := []struct {
		name       string
		account    *Account
		want       bool
		wantReason string
REDACTED{
		{name: "nil account", account: nil, want: false, wantReason: "not_grok"REDACTED,
		{name: "non grok account", account: &Account{Platform: PlatformOpenAIREDACTED, want: false, wantReason: "not_grok"REDACTED,
		{name: "non oauth grok account stays eligible", account: &Account{Platform: PlatformGrok, Type: AccountTypeAPIKeyREDACTED, want: true, wantReason: "non_oauth"REDACTED,
		{name: "unobserved oauth preserves legacy routing", account: &Account{Platform: PlatformGrok, Type: AccountTypeOAuthREDACTED, want: true, wantReason: "billing_unobserved"REDACTED,
		{name: "weekly allowance is not treated as weekly subscription", account: &Account{Platform: PlatformGrok, Type: AccountTypeOAuth, Extra: map[string]any{grokBillingExtraKey: weeklyAllowanceREDACTEDREDACTED, want: true, wantReason: "eligible"REDACTED,
		{name: "billing forbidden is rejected", account: &Account{Platform: PlatformGrok, Type: AccountTypeOAuth, Extra: map[string]any{grokBillingExtraKey: forbiddenBillingREDACTEDREDACTED, want: false, wantReason: "billing_forbidden"REDACTED,
		{name: "explicit disable wins", account: &Account{Platform: PlatformGrok, Type: AccountTypeOAuth, Extra: map[string]any{GrokMediaEligibleExtraKey: falseREDACTEDREDACTED, want: false, wantReason: "override_disabled"REDACTED,
		{name: "explicit enable wins over forbidden probe", account: &Account{Platform: PlatformGrok, Type: AccountTypeOAuth, Extra: map[string]any{GrokMediaEligibleExtraKey: true, grokBillingExtraKey: forbiddenBillingREDACTEDREDACTED, want: true, wantReason: "override_enabled"REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, reason := tt.account.GrokMediaGenerationEligibility()
			require.Equal(t, tt.want, got)
			require.Equal(t, tt.wantReason, reason)
	REDACTED)
REDACTED
REDACTED

func TestGrokMediaCapabilityFiltersOnlyGeneration(t *testing.T) {
	account := &Account{
		ID:          1,
		Platform:    PlatformGrok,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Extra:       map[string]any{GrokMediaEligibleExtraKey: falseREDACTED,
REDACTED

	require.True(t, account.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityChatCompletions))
	require.False(t, account.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityGrokMediaGeneration))
	require.False(t, isOpenAICompatibleAccountEligibleForRequest(
		context.Background(), account, PlatformGrok, "grok-imagine-video", false,
		OpenAIEndpointCapabilityGrokMediaGeneration,
	))
REDACTED

func TestNormalizeGrokMediaEligibilityExtra(t *testing.T) {
	t.Run("boolean override is accepted", func(t *testing.T) {
		extra, err := normalizeGrokMediaEligibilityExtra(PlatformGrok, map[string]any{GrokMediaEligibleExtraKey: falseREDACTED)

	REDACTED
		require.Equal(t, false, extra[GrokMediaEligibleExtraKey])
REDACTED)

	t.Run("null clears override", func(t *testing.T) {
		extra, err := normalizeGrokMediaEligibilityExtra(PlatformGrok, map[string]any{GrokMediaEligibleExtraKey: nilREDACTED)

	REDACTED
		require.NotContains(t, extra, GrokMediaEligibleExtraKey)
REDACTED)

	t.Run("malformed override is rejected", func(t *testing.T) {
		_, err := normalizeGrokMediaEligibilityExtra(PlatformGrok, map[string]any{GrokMediaEligibleExtraKey: "false"REDACTED)

	REDACTED
		require.Equal(t, http.StatusBadRequest, infraerrors.Code(err))
REDACTED)

	t.Run("other platforms ignore provider owned value", func(t *testing.T) {
		extra := map[string]any{GrokMediaEligibleExtraKey: "provider-owned"REDACTED
		normalized, err := normalizeGrokMediaEligibilityExtra(PlatformOpenAI, extra)

	REDACTED
		require.Equal(t, extra, normalized)
REDACTED)
REDACTED

func TestNormalizeGrokMediaEligibilityUpdateExtra(t *testing.T) {
	account := &Account{Platform: PlatformGrok, Extra: map[string]any{GrokMediaEligibleExtraKey: falseREDACTEDREDACTED

	t.Run("omitted override preserves current value", func(t *testing.T) {
		input := &UpdateAccountInput{Extra: map[string]any{"quota_used": float64(1)REDACTEDREDACTED
		normalized, err := normalizeGrokMediaEligibilityUpdateExtra(account, input, map[string]any{"quota_used": float64(1)REDACTED)

	REDACTED
		require.Equal(t, false, normalized[GrokMediaEligibleExtraKey])
REDACTED)

	t.Run("null removes current override", func(t *testing.T) {
		input := &UpdateAccountInput{Extra: map[string]any{GrokMediaEligibleExtraKey: nilREDACTEDREDACTED
		normalized, err := normalizeGrokMediaEligibilityUpdateExtra(account, input, map[string]any{GrokMediaEligibleExtraKey: nilREDACTED)

	REDACTED
		require.NotContains(t, normalized, GrokMediaEligibleExtraKey)
		require.Contains(t, input.Extra, GrokMediaEligibleExtraKey)
REDACTED)
REDACTED
