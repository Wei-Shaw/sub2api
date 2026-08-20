//go:build unit

package service

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestClassifyGrokUpstreamFailure_FreeUsage(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
REDACTED{
		{
			name:   "code free-usage-exhausted",
			status: http.StatusTooManyRequests,
			body:   `{"error":{"code":"subscription:free-usage-exhausted","message":"You've used all the included free usage for model grok-4.5. Usage resets over a rolling 24-hour window."REDACTEDREDACTED`,
	REDACTED,
		{
			name:   "chinese body without 429",
			status: http.StatusBadRequest,
			body:   `{"error":{"message":"模型额度用完，请稍后再试"REDACTEDREDACTED`,
	REDACTED,
		{
			name:   "token pair with free marker",
			status: http.StatusOK,
			body:   `{"error":{"message":"free usage tokens (actual / limit): 2000000 / 2000000 for model grok-4.5"REDACTEDREDACTED`,
	REDACTED,
REDACTED
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := classifyGrokUpstreamFailure(tc.status, []byte(tc.body), "grok-4.5")
			require.Equal(t, GrokFailureFreeUsage, d.Class)
			require.True(t, d.ShouldCooldown)
			require.True(t, d.ShouldFailover)
			require.False(t, d.BlockModel, "free-usage must not soft-block models")
			require.Equal(t, grokFreeUsageProbeCooldown, d.Cooldown)
	REDACTED)
REDACTED
REDACTED

func TestClassifyGrokUpstreamFailure_EmptyUpstream(t *testing.T) {
	d := classifyGrokUpstreamFailure(http.StatusBadGateway, []byte(`empty model output: no content/tool_calls`), "grok-4.5")
	require.Equal(t, GrokFailureEmptyUpstream, d.Class)
	require.True(t, d.ShouldCooldown)
	require.True(t, d.ShouldFailover)
	require.True(t, d.BlockModel)
	require.Equal(t, 4*time.Minute, d.Cooldown)
REDACTED

func TestClassifyGrokUpstreamFailure_Billing(t *testing.T) {
	d := classifyGrokUpstreamFailure(http.StatusForbidden, []byte(`{"code":"personal-team-blocked:spending-limit","error":"spending limit reached"REDACTED`), "")
	require.Equal(t, GrokFailureBilling, d.Class)
	require.True(t, d.ShouldCooldown)
	require.True(t, d.ShouldFailover)
REDACTED

func TestClassifyGrokUpstreamFailure_GrokSubscriptionRequiredIsBilling(t *testing.T) {
	d := classifyGrokUpstreamFailure(http.StatusPaymentRequired,
		[]byte(`{"error":{"message":"You have run out of credits or need a Grok subscription"REDACTEDREDACTED`), "grok-4.6")
	require.Equal(t, GrokFailureBilling, d.Class)
	require.True(t, d.ShouldFailover)
	require.True(t, d.ShouldCooldown)
REDACTED

func TestGrokRetryableOnSameAccount_CapacityAndRateLimit(t *testing.T) {
	account := &Account{ID: 9105, Platform: PlatformGrok, Type: AccountTypeOAuthREDACTED
	require.True(t, grokRetryableOnSameAccount(account, http.StatusTooManyRequests,
		[]byte(`{"error":{"message":"The model is currently at capacity due to high demand"REDACTEDREDACTED`)))
	require.False(t, grokRetryableOnSameAccount(account, http.StatusTooManyRequests,
		[]byte(`{"error":{"message":"rate limit exceeded"REDACTEDREDACTED`)))
	require.False(t, grokRetryableOnSameAccount(account, http.StatusPaymentRequired,
		[]byte(`{"error":{"message":"You have run out of credits or need a Grok subscription"REDACTEDREDACTED`)))
	poolAccount := &Account{ID: 9108, Platform: PlatformGrok, Type: AccountTypeOAuth,
REDACTED"pool_mode": trueREDACTEDREDACTED
	require.False(t, grokRetryableOnSameAccount(poolAccount, http.StatusTooManyRequests,
		[]byte(`{"error":{"code":"subscription:free-usage-exhausted"REDACTEDREDACTED`)),
		"pool free-usage must fail over instead of retrying the exhausted account")
	require.False(t, grokRetryableOnSameAccount(account, http.StatusBadRequest,
		[]byte(`{"error":{"message":"capacity field is invalid"REDACTEDREDACTED`)))
	nonGrok := &Account{ID: 9106, Platform: PlatformOpenAI, Type: AccountTypeOAuthREDACTED
	require.False(t, grokRetryableOnSameAccount(nonGrok, http.StatusTooManyRequests,
		[]byte(`{"error":{"message":"model at capacity"REDACTEDREDACTED`)))
REDACTED

func TestShouldMarkGrokTeamModelRateLimit_ExcludesCapacity(t *testing.T) {
	require.False(t, shouldMarkGrokTeamModelRateLimit(http.StatusTooManyRequests,
		[]byte(`{"error":{"message":"The model is currently at capacity due to high demand"REDACTEDREDACTED`)))
	require.True(t, shouldMarkGrokTeamModelRateLimit(http.StatusTooManyRequests,
		[]byte(`{"error":{"message":"rate limit exceeded"REDACTEDREDACTED`)))
	require.True(t, shouldMarkGrokTeamModelRateLimit(http.StatusBadRequest,
		[]byte(`{"error":{"code":"subscription:free-usage-exhausted"REDACTEDREDACTED`)))
	require.False(t, shouldMarkGrokTeamModelRateLimit(http.StatusBadRequest,
		[]byte(`{"error":{"message":"invalid request"REDACTEDREDACTED`)))
REDACTED

func TestGrokSameAccountRetryMetadata_CapacityDeadline(t *testing.T) {
	account := &Account{ID: 9107, Platform: PlatformGrok, Type: AccountTypeOAuthREDACTED
	retryable, delay, deadline := grokSameAccountRetryMetadata(account, http.StatusTooManyRequests,
		[]byte(`{"error":{"message":"model capacity exceeded"REDACTEDREDACTED`))
	require.True(t, retryable)
	require.Equal(t, 500*time.Millisecond, delay)
	require.WithinDuration(t, time.Now().Add(30*time.Second), deadline, 2*time.Second)

	retryable, delay, deadline = grokSameAccountRetryMetadata(account, http.StatusTooManyRequests,
		[]byte(`{"error":{"message":"rate limit exceeded"REDACTEDREDACTED`))
	require.False(t, retryable)
	require.Zero(t, delay)
	require.True(t, deadline.IsZero())
REDACTED

func TestClassifyGrokUpstreamFailure_ValidationNoCool(t *testing.T) {
	d := classifyGrokUpstreamFailure(http.StatusBadRequest, []byte(`{"error":{"message":"invalid tool schema"REDACTEDREDACTED`), "")
	require.Equal(t, GrokFailureNone, d.Class)
	require.False(t, d.ShouldCooldown)
	require.False(t, d.ShouldFailover)
REDACTED

func TestClassifyGrokUpstreamFailure_FreeUsageWinsOver5xx(t *testing.T) {
	// Proxy may rewrite free-usage into synthetic 502; body must win.
	d := classifyGrokUpstreamFailure(http.StatusBadGateway, []byte(`subscription:free-usage-exhausted for model grok-4.3`), "grok-4.3")
	require.Equal(t, GrokFailureFreeUsage, d.Class)
	require.NotEqual(t, GrokFailureServer, d.Class)
REDACTED

func TestClassifyGrokUpstreamFailure_CompatibilityDoesNotCooldown(t *testing.T) {
	cases := []string{
		`{"error":{"message":"Could not decode the compaction blob. Ensure it is unmodified from the compact response"REDACTEDREDACTED`,
		`{"code":"compaction_decode_error","message":"invalid response history"REDACTED`,
REDACTED
	for _, body := range cases {
		d := classifyGrokUpstreamFailure(http.StatusUnprocessableEntity, []byte(body), "grok-4.6")
		require.Equal(t, GrokFailureCompatibility, d.Class, body)
		require.True(t, d.ShouldFailover, body)
		require.False(t, d.ShouldCooldown, body)
		require.Zero(t, d.Cooldown, body)
REDACTED
REDACTED

func TestClassifyGrokUpstreamFailure_GenericShapeErrorDoesNotFailover(t *testing.T) {
	d := classifyGrokUpstreamFailure(http.StatusBadRequest,
		[]byte(`{"error":{"message":"data did not match any variant of the untagged enum content"REDACTEDREDACTED`), "grok-4.6")
	require.NotEqual(t, GrokFailureCompatibility, d.Class)
	require.False(t, d.ShouldFailover)
REDACTED

func TestShouldFailoverGrokUpstreamError_FreeUsageBody(t *testing.T) {
	svc := &OpenAIGatewayService{REDACTED
	body := []byte(`{"error":{"code":"subscription:free-usage-exhausted","message":"free usage exhausted"REDACTEDREDACTED`)
	require.True(t, svc.shouldFailoverGrokUpstreamError(http.StatusBadRequest, body))
REDACTED

func TestShouldFailoverGrokUpstreamError_CompatibilityBody(t *testing.T) {
	svc := &OpenAIGatewayService{REDACTED
	body := []byte(`{"error":{"message":"Could not decode the compaction blob"REDACTEDREDACTED`)
	require.True(t, svc.shouldFailoverGrokUpstreamError(http.StatusUnprocessableEntity, body))
REDACTED

func TestShouldFailoverGrokUpstreamError_ContentPolicyStillNoFailover(t *testing.T) {
	svc := &OpenAIGatewayService{REDACTED
	body := []byte(`{"error":{"code":"new_sensitive","message":"text is sensitive"REDACTEDREDACTED`)
	require.False(t, svc.shouldFailoverGrokUpstreamError(http.StatusForbidden, body))
REDACTED

func TestHandleGrokAccountUpstreamError_FreeUsageBodyCoolsAccount(t *testing.T) {
	repo := &grokQuotaAccountRepo{REDACTED
	svc := &OpenAIGatewayService{accountRepo: repoREDACTED
	account := &Account{ID: 9101, Platform: PlatformGrok, Type: AccountTypeOAuthREDACTED
	before := time.Now()
	body := []byte(`{"error":{"code":"subscription:free-usage-exhausted","message":"You've used all the included free usage. Usage resets over a rolling 24-hour window."REDACTEDREDACTED`)

	svc.handleGrokAccountUpstreamError(context.Background(), account, http.StatusBadRequest, nil, body)

	require.Equal(t, 1, repo.tempUnschedCalls)
	require.Equal(t, "grok free usage exhausted", repo.lastTempUnschedReason)
	// Rolling-window exhaustion must use a short probe cooldown when no
	// upstream absolute reset is available; it must not start a 24h lock here.
	require.Greater(t, repo.lastTempUnschedUntil, before.Add(grokFreeUsageProbeCooldown-time.Second))
	require.Less(t, repo.lastTempUnschedUntil, before.Add(grokFreeUsageProbeCooldown+time.Second))
REDACTED

func TestHandleGrokAccountUpstreamError_FreeUsageUsesUpstreamReset(t *testing.T) {
	repo := &grokQuotaAccountRepo{REDACTED
	svc := &OpenAIGatewayService{accountRepo: repoREDACTED
	account := &Account{ID: 9102, Platform: PlatformGrok, Type: AccountTypeOAuthREDACTED
	body := []byte(`{"error":{"code":"subscription:free-usage-exhausted","message":"free usage exhausted; rolling 24-hour window"REDACTEDREDACTED`)

	svc.handleGrokAccountUpstreamError(context.Background(), account, http.StatusTooManyRequests,
		http.Header{"Retry-After": []string{"3600"REDACTEDREDACTED, body)

	require.Zero(t, repo.tempUnschedCalls)
	require.WithinDuration(t, time.Now().Add(time.Hour), repo.lastRateLimitResetAt, 2*time.Second)
REDACTED

func TestHandleGrokAccountUpstreamError_EmptyOutputCoolsAccount(t *testing.T) {
	repo := &grokQuotaAccountRepo{REDACTED
	svc := &OpenAIGatewayService{accountRepo: repoREDACTED
	account := &Account{ID: 9102, Platform: PlatformGrok, Type: AccountTypeOAuthREDACTED
	before := time.Now()

	svc.handleGrokAccountUpstreamError(
		context.Background(), account, http.StatusBadGateway, nil,
		[]byte(`empty model output: no content/tool_calls`),
	)

	require.Equal(t, 1, repo.tempUnschedCalls)
	require.Equal(t, "grok empty model output", repo.lastTempUnschedReason)
	require.WithinDuration(t, before.Add(4*time.Minute), repo.lastTempUnschedUntil, time.Second)
REDACTED

func TestHandleGrokAccountUpstreamError_MultiAgentCapacityBlocksOnlyThatModel(t *testing.T) {
	repo := &grokQuotaAccountRepo{REDACTED
	svc := &OpenAIGatewayService{accountRepo: repoREDACTED
	account := &Account{ID: 9120, Platform: PlatformGrok, Type: AccountTypeOAuthREDACTED
	ctx := withGrokTeamRateLimitModel(context.Background(), "grok-4.20-multi-agent-0309")

	svc.handleGrokAccountUpstreamError(
		ctx, account, http.StatusBadGateway, nil,
		[]byte(`{"error":{"message":"engine_overloaded"REDACTEDREDACTED`),
	)

	require.Zero(t, repo.tempUnschedCalls)
	require.True(t, isGrokModelQuotaBlocked(account.ID, "grok-4.20-multi-agent-0309", time.Now()))
	require.False(t, isGrokModelQuotaBlocked(account.ID, "grok-4.5", time.Now()))
REDACTED

func TestHandleGrokAccountUpstreamError_CapacityNeverCoolsAccount(t *testing.T) {
	repo := &grokQuotaAccountRepo{REDACTED
	svc := &OpenAIGatewayService{accountRepo: repoREDACTED
	account := &Account{ID: 9121, Platform: PlatformGrok, Type: AccountTypeOAuthREDACTED
	ctx := withGrokTeamRateLimitModel(context.Background(), "grok-4.6")

	svc.handleGrokAccountUpstreamError(ctx, account, http.StatusTooManyRequests, nil,
		[]byte(`{"error":{"message":"The model is currently at capacity due to high demand"REDACTEDREDACTED`))

	require.Zero(t, repo.tempUnschedCalls)
	require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))
REDACTED

func TestHandleGrokAccountUpstreamError_FreeUsageDoesNotCoolPoolMode(t *testing.T) {
	repo := &grokQuotaAccountRepo{REDACTED
	svc := &OpenAIGatewayService{accountRepo: repoREDACTED
	account := &Account{
		ID:       9103,
		Platform: PlatformGrok,
		Type:     AccountTypeAPIKey,
REDACTED
			"pool_mode": true,
	REDACTED,
REDACTED
	body := []byte(`{"error":{"code":"subscription:free-usage-exhausted","message":"free usage exhausted"REDACTEDREDACTED`)

	svc.handleGrokAccountUpstreamError(context.Background(), account, http.StatusBadRequest, nil, body)

	require.Zero(t, repo.tempUnschedCalls)
	require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))
REDACTED

func TestHandleGrokAccountUpstreamError_ContentPolicyStillNoMutation(t *testing.T) {
	repo := &grokQuotaAccountRepo{REDACTED
	svc := &OpenAIGatewayService{accountRepo: repoREDACTED
	account := &Account{ID: 9104, Platform: PlatformGrok, Type: AccountTypeOAuthREDACTED
	body := []byte(`{"error":{"code":"new_sensitive","message":"text is sensitive"REDACTEDREDACTED`)

	svc.handleGrokAccountUpstreamError(context.Background(), account, http.StatusForbidden, nil, body)

	require.Zero(t, repo.tempUnschedCalls)
REDACTED

func TestHandleGrokAccountUpstreamError_Entitlement403Unchanged(t *testing.T) {
	repo := &grokQuotaAccountRepo{REDACTED
	svc := &OpenAIGatewayService{accountRepo: repoREDACTED
	account := &Account{ID: 9105, Platform: PlatformGrok, Type: AccountTypeOAuthREDACTED
	before := time.Now()

	svc.handleGrokAccountUpstreamError(
		context.Background(), account, http.StatusForbidden, nil,
		[]byte(`{"error":{"message":"subscription required"REDACTEDREDACTED`),
	)

	require.Equal(t, 1, repo.tempUnschedCalls)
	require.Equal(t, "grok access or entitlement denied", repo.lastTempUnschedReason)
	require.Greater(t, repo.lastTempUnschedUntil, before.Add(29*time.Minute))
	require.Less(t, repo.lastTempUnschedUntil, before.Add(31*time.Minute))
REDACTED
