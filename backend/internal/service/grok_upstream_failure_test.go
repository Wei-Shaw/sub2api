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
			require.GreaterOrEqual(t, d.Cooldown, 20*time.Minute)
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

func TestShouldFailoverGrokUpstreamError_FreeUsageBody(t *testing.T) {
	svc := &OpenAIGatewayService{REDACTED
	body := []byte(`{"error":{"code":"subscription:free-usage-exhausted","message":"free usage exhausted"REDACTEDREDACTED`)
	require.True(t, svc.shouldFailoverGrokUpstreamError(http.StatusBadRequest, body))
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
	// 24h rolling → 2h cool
	require.Greater(t, repo.lastTempUnschedUntil, before.Add(119*time.Minute))
	require.Less(t, repo.lastTempUnschedUntil, before.Add(121*time.Minute))
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
