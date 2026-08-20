package service

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type openAIAPIKeyHealthSettingRepo struct {
	SettingRepository
	value string
REDACTED

func (r *openAIAPIKeyHealthSettingRepo) GetValue(context.Context, string) (string, error) {
	return r.value, nil
REDACTED

type openAIAPIKeyHealthAccountRepo struct {
	AccountRepository
	setCalls int
	reason   string
REDACTED

func (r *openAIAPIKeyHealthAccountRepo) SetTempUnschedulable(_ context.Context, _ int64, _ time.Time, reason string) error {
	r.setCalls++
	r.reason = reason
	return nil
REDACTED

type openAIAPIKeyHealthCacheStub struct {
	TempUnschedCache
	recordCalls int
	resetCalls  int
	setCalls    int
	tripped     bool
REDACTED

func (c *openAIAPIKeyHealthCacheStub) RecordOpenAIAPIKeyHealthFailure(context.Context, int64, int, int) (int64, bool, error) {
	c.recordCalls++
	return 3, c.tripped, nil
REDACTED

func (c *openAIAPIKeyHealthCacheStub) ResetOpenAIAPIKeyHealthFailures(context.Context, int64) error {
	c.resetCalls++
	return nil
REDACTED

func (c *openAIAPIKeyHealthCacheStub) SetTempUnsched(context.Context, int64, *TempUnschedState) error {
	c.setCalls++
	return nil
REDACTED

type openAIAPIKeyHealthRuntimeBlocker struct{ calls int REDACTED

func (b *openAIAPIKeyHealthRuntimeBlocker) BlockAccountScheduling(*Account, time.Time, string) {
	b.calls++
REDACTED
func (*openAIAPIKeyHealthRuntimeBlocker) ClearAccountSchedulingBlock(int64) {REDACTED

func openAIHealthPoolAccount() *Account {
REDACTED
		ID:       42,
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
REDACTED
			"pool_mode": true,
	REDACTED,
REDACTED
REDACTED

func TestClassifyOpenAIAPIKeyHealthFailureExclusions(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		eligible bool
REDACTED{
		{name: "account attributed 502", err: &UpstreamFailoverError{StatusCode: http.StatusBadGatewayREDACTED, eligible: trueREDACTED,
		{name: "request scoped capacity", err: &UpstreamFailoverError{StatusCode: 529, RequestScopedTransient: trueREDACTEDREDACTED,
		{name: "provider scoped overload", err: &UpstreamFailoverError{StatusCode: 529, Scope: GatewayFailureScopeProviderREDACTEDREDACTED,
		{name: "dedicated same account retry", err: &UpstreamFailoverError{StatusCode: http.StatusTooManyRequests, RetryableOnSameAccount: trueREDACTEDREDACTED,
		{name: "credential disable path", err: &UpstreamFailoverError{StatusCode: http.StatusUnauthorized, Stage: GatewayFailureStageAccountAuth, Scope: GatewayFailureScopeAccountREDACTEDREDACTED,
		{name: "client request", err: &UpstreamFailoverError{StatusCode: http.StatusBadRequestREDACTEDREDACTED,
REDACTED
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, eligible := classifyOpenAIAPIKeyHealthFailure(tt.err)
			require.Equal(t, tt.eligible, eligible)
	REDACTED)
REDACTED
REDACTED

func TestOpenAIAPIKeyHealthBreakerDefaultDisabled(t *testing.T) {
	settings := NewSettingService(&openAIAPIKeyHealthSettingRepo{REDACTED, &config.Config{REDACTED)
	cache := &openAIAPIKeyHealthCacheStub{tripped: trueREDACTED
	svc := NewRateLimitService(&openAIAPIKeyHealthAccountRepo{REDACTED, nil, &config.Config{REDACTED, nil, cache)
	svc.SetSettingService(settings)
	svc.SetOpenAIAPIKeyHealthCache(cache)

	require.False(t, svc.ObserveOpenAIAPIKeyHealthFailure(context.Background(), openAIHealthPoolAccount(), &UpstreamFailoverError{StatusCode: http.StatusBadGatewayREDACTED))
	require.Zero(t, cache.recordCalls)
REDACTED

func TestOpenAIAPIKeyHealthBreakerTripsPersistedAndRuntimeState(t *testing.T) {
	encoded, err := json.Marshal(OpenAIAPIKeyHealthBreakerSettings{Enabled: true, WindowMinutes: 1, FailureThreshold: 3, CooldownMinutes: 5REDACTED)
REDACTED
	settings := NewSettingService(&openAIAPIKeyHealthSettingRepo{value: string(encoded)REDACTED, &config.Config{REDACTED)
	cache := &openAIAPIKeyHealthCacheStub{tripped: trueREDACTED
	repo := &openAIAPIKeyHealthAccountRepo{REDACTED
	blocker := &openAIAPIKeyHealthRuntimeBlocker{REDACTED
	svc := NewRateLimitService(repo, nil, &config.Config{REDACTED, nil, cache)
	svc.SetSettingService(settings)
	svc.SetOpenAIAPIKeyHealthCache(cache)
	svc.SetAccountRuntimeBlocker(blocker)
	account := openAIHealthPoolAccount()

	require.True(t, svc.ObserveOpenAIAPIKeyHealthFailure(context.Background(), account, &UpstreamFailoverError{StatusCode: http.StatusBadGateway, ResponseBody: []byte(`{"error":"upstream"REDACTED`)REDACTED))
	require.Equal(t, 1, cache.recordCalls)
	require.Equal(t, 1, cache.setCalls)
	require.Equal(t, 1, repo.setCalls)
	require.Equal(t, 1, blocker.calls)
	require.NotNil(t, account.TempUnschedulableUntil)
	require.Contains(t, repo.reason, openAIAPIKeyHealthBreakerReason)
REDACTED

func TestOpenAIAPIKeyHealthSuccessResetsOnlyEligiblePoolAccount(t *testing.T) {
	encoded, err := json.Marshal(OpenAIAPIKeyHealthBreakerSettings{Enabled: true, WindowMinutes: 1, FailureThreshold: 3, CooldownMinutes: 5REDACTED)
REDACTED
	settings := NewSettingService(&openAIAPIKeyHealthSettingRepo{value: string(encoded)REDACTED, &config.Config{REDACTED)
	cache := &openAIAPIKeyHealthCacheStub{REDACTED
	svc := NewRateLimitService(&openAIAPIKeyHealthAccountRepo{REDACTED, nil, &config.Config{REDACTED, nil, cache)
	svc.SetSettingService(settings)
	svc.SetOpenAIAPIKeyHealthCache(cache)

	svc.ObserveOpenAIAPIKeyHealthSuccess(context.Background(), openAIHealthPoolAccount())
	svc.ObserveOpenAIAPIKeyHealthSuccess(context.Background(), &Account{ID: 43, Platform: PlatformOpenAI, Type: AccountTypeAPIKeyREDACTED)
	require.Equal(t, 1, cache.resetCalls)
REDACTED
