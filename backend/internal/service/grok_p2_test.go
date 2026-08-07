//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestGrokModelQuotaBlock_FiltersOnlyNamedModel(t *testing.T) {
	id := time.Now().UnixNano()%1_000_000 + 5000
	markGrokModelQuotaBlock(id, "grok-4.5", time.Now().Add(time.Hour))
	now := time.Now()
	require.True(t, isGrokModelQuotaBlocked(id, "grok-4.5", now))
	require.False(t, isGrokModelQuotaBlocked(id, "grok-4.3", now))

	accounts := []Account{
		{ID: id, Platform: PlatformGrok, Type: AccountTypeOAuthREDACTED,
		{ID: id + 1, Platform: PlatformGrok, Type: AccountTypeOAuthREDACTED,
REDACTED
	filtered := filterGrokModelQuotaBlockedAccounts(accounts, "grok-4.5", now)
	require.Len(t, filtered, 1)
	require.Equal(t, id+1, filtered[0].ID)
REDACTED

func TestGrokModelQuotaBlockFiltersMappedUpstreamModel(t *testing.T) {
	id := time.Now().UnixNano()%1_000_000 + 7000
	markGrokModelQuotaBlock(id, "grok-4.5", time.Now().Add(time.Hour))
	account := Account{
		ID:       id,
		Platform: PlatformGrok,
		Type:     AccountTypeOAuth,
REDACTED
			"model_mapping": map[string]any{"gpt-*": "grok-4.5"REDACTED,
	REDACTED,
REDACTED

	require.Empty(t, filterGrokModelQuotaBlockedAccounts([]Account{accountREDACTED, "gpt-5", time.Now()))
REDACTED

func TestIsGrokModelSpecificFreeUsage(t *testing.T) {
	require.True(t, isGrokModelSpecificFreeUsage(
		"you've used all the included free usage for model grok-4.5", "grok-4.5"))
	require.True(t, isGrokModelSpecificFreeUsage("模型额度用完 grok-4.3", "grok-4.3"))
	require.False(t, isGrokModelSpecificFreeUsage("free usage exhausted", "grok-4.5"))
REDACTED

func TestGrokStickyAffinitySeed_ScopesByModel(t *testing.T) {
	a := grokStickyAffinitySeed("session-1", []byte(`{"model":"grok-4.5"REDACTED`))
	b := grokStickyAffinitySeed("session-1", []byte(`{"model":"grok-4.3"REDACTED`))
	c := grokStickyAffinitySeed("session-1", []byte(`{"model":"grok-4.5"REDACTED`))
	require.NotEqual(t, a, b)
	require.Equal(t, a, c)
	require.Contains(t, a, "grok-affinity:v1:")
REDACTED

func TestGrokFreeUsageRecoveryHints(t *testing.T) {
	id := int64(9001)
	coolUntil := time.Now().Add(time.Hour)
	markGrokFreeUsageRecovery(id, coolUntil)
	require.True(t, isGrokFreeUsageCooling(id, time.Now()))
	require.False(t, isGrokFreeUsageProbePreferred(id, time.Now()))
	// After cool: probe preferred
	require.True(t, isGrokFreeUsageProbePreferred(id, coolUntil.Add(time.Minute)))
	require.False(t, isGrokFreeUsageCooling(id, coolUntil.Add(time.Minute)))
REDACTED

func TestExtractGrokModelIDsFromModelsBody(t *testing.T) {
	body := []byte(`{"object":"list","data":[{"id":"grok-4.5"REDACTED,{"id":"grok-4.3"REDACTED,{"id":"grok-4.5"REDACTED]REDACTED`)
	ids := extractGrokModelIDsFromModelsBody(body)
	require.Equal(t, []string{"grok-4.5", "grok-4.3"REDACTED, ids)
REDACTED

func TestAccountGrokNeedsReauth(t *testing.T) {
	require.False(t, accountGrokNeedsReauth(nil))
	require.True(t, accountGrokNeedsReauth(&Account{
		Extra: map[string]any{"grok_needs_reauth": trueREDACTED,
REDACTED))
	require.True(t, accountGrokNeedsReauth(&Account{
		Status:       StatusError,
		ErrorMessage: grokSpendingLimitErrorMessage,
REDACTED))
REDACTED

func TestApplyGrokUpstreamFailure_ModelSpecificFreeUsage(t *testing.T) {
	repo := &grokQuotaAccountRepo{REDACTED
	svc := &OpenAIGatewayService{accountRepo: repoREDACTED
	account := &Account{ID: 9109, Platform: PlatformGrok, Type: AccountTypeOAuthREDACTED
	body := []byte(`{"error":{"code":"subscription:free-usage-exhausted","message":"You've used all the included free usage for model grok-4.5. Usage resets over a rolling 24-hour window."REDACTEDREDACTED`)

	svc.handleGrokAccountUpstreamError(context.Background(), account, 400, nil, body)

	require.Zero(t, repo.tempUnschedCalls, "model-scoped free usage must not cool sibling models")
	require.True(t, isGrokModelQuotaBlocked(account.ID, "grok-4.5", time.Now()))
	require.False(t, isGrokModelQuotaBlocked(account.ID, "grok-4.3", time.Now()))
REDACTED

func TestApplyGrokUpstreamFailure_SpendingLimitMarksReauth(t *testing.T) {
	repo := &grokQuotaAccountRepo{REDACTED
	svc := &OpenAIGatewayService{accountRepo: repoREDACTED
	account := &Account{ID: 9110, Platform: PlatformGrok, Type: AccountTypeOAuthREDACTED
	body := []byte(`{"code":"personal-team-blocked:spending-limit","error":"spending limit reached"REDACTED`)

	svc.handleGrokAccountUpstreamError(context.Background(), account, 403, nil, body)

	require.Equal(t, 1, repo.tempUnschedCalls)
	require.Equal(t, "grok spending limit", repo.lastTempUnschedReason)
	// Long cool for spending
	require.Greater(t, repo.lastTempUnschedUntil, time.Now().Add(23*time.Hour))
REDACTED
