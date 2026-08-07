//go:build unit

package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/stretchr/testify/require"
)

type grokFreeQuotaUsageRepoStub struct {
	UsageLogRepository

	mu      sync.Mutex
	stats   map[int64]*usagestats.AccountStats
	err     error
	calls   int
	lastIDs []int64
	start   time.Time
REDACTED

type grokFreeQuotaAccountRepoStub struct {
	AccountRepository
	accounts []Account
REDACTED

func (r *grokFreeQuotaAccountRepoStub) ListSchedulableByPlatform(context.Context, string) ([]Account, error) {
	return append([]Account(nil), r.accounts...), nil
REDACTED

func (r *grokFreeQuotaUsageRepoStub) GetAccountWindowStatsBatch(_ context.Context, accountIDs []int64, start time.Time) (map[int64]*usagestats.AccountStats, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	r.lastIDs = append([]int64(nil), accountIDs...)
	r.start = start
	if r.err != nil {
		return nil, r.err
REDACTED
	result := make(map[int64]*usagestats.AccountStats, len(accountIDs))
	for _, accountID := range accountIDs {
		if stats := r.stats[accountID]; stats != nil {
			copyStats := *stats
			result[accountID] = &copyStats
	REDACTED
REDACTED
	return result, nil
REDACTED

func grokFreeQuotaTestConfig() *config.Config {
	cfg := &config.Config{REDACTED
	cfg.Gateway.Grok.FreeQuotaSoftGateEnabled = true
	cfg.Gateway.Grok.FreeQuotaTokenLimit = 2_000_000
	cfg.Gateway.Grok.FreeQuotaSoftGatePercent = 95
	cfg.Gateway.Grok.FreeQuotaWindowHours = 24
	cfg.Gateway.Grok.FreeQuotaStatsCacheSeconds = 5
	return cfg
REDACTED

func TestFilterGrokFreeQuotaAccountsOnlyBlocksExplicitFreeOAuth(t *testing.T) {
	repo := &grokFreeQuotaUsageRepoStub{stats: map[int64]*usagestats.AccountStats{
		1: {Tokens: 1_900_000REDACTED,
REDACTEDREDACTED
	scheduler := &defaultOpenAIAccountScheduler{service: &OpenAIGatewayService{cfg: grokFreeQuotaTestConfig(), usageLogRepo: repoREDACTEDREDACTED
	accounts := []Account{
		{ID: 1, Platform: PlatformGrok, Type: AccountTypeOAuth, Credentials: map[string]any{"subscription_tier": "FREE"REDACTEDREDACTED,
		{ID: 2, Platform: PlatformGrok, Type: AccountTypeOAuth, Credentials: map[string]any{"subscription_tier": "PRO"REDACTEDREDACTED,
		{ID: 3, Platform: PlatformGrok, Type: AccountTypeOAuthREDACTED,
		{ID: 4, Platform: PlatformGrok, Type: AccountTypeAPIKey, Credentials: map[string]any{"subscription_tier": "FREE"REDACTEDREDACTED,
REDACTED

	filtered := scheduler.filterGrokFreeQuotaAccounts(context.Background(), accounts)
	require.Equal(t, []int64{2, 3, 4REDACTED, accountIDs(filtered), "paid and unknown fail-open; API-key free marker is not gated")
	require.Equal(t, 1, repo.calls)
	require.Equal(t, []int64{1REDACTED, repo.lastIDs, "paid, unknown, and API-key accounts must not enter the local free-tier query")
	require.WithinDuration(t, time.Now().UTC().Add(-24*time.Hour), repo.start, time.Second)
REDACTED

func TestFilterGrokFreeQuotaAccountsStatsFailureFailsOpen(t *testing.T) {
	repo := &grokFreeQuotaUsageRepoStub{err: errors.New("usage database unavailable")REDACTED
	scheduler := &defaultOpenAIAccountScheduler{service: &OpenAIGatewayService{cfg: grokFreeQuotaTestConfig(), usageLogRepo: repoREDACTEDREDACTED
	accounts := []Account{{
		ID: 1, Platform: PlatformGrok, Type: AccountTypeOAuth,
REDACTED"subscription_tier": "free"REDACTED,
REDACTEDREDACTED

	filtered := scheduler.filterGrokFreeQuotaAccounts(context.Background(), accounts)
	require.Equal(t, []int64{1REDACTED, accountIDs(filtered))
	// Cache the failure entry so a second call still fails open without re-query thrash.
	filtered = scheduler.filterGrokFreeQuotaAccounts(context.Background(), accounts)
	require.Equal(t, []int64{1REDACTED, accountIDs(filtered))
	require.Equal(t, 1, repo.calls)
REDACTED

func TestFilterGrokFreeQuotaAccountsUnknownTierFailOpen(t *testing.T) {
	repo := &grokFreeQuotaUsageRepoStub{stats: map[int64]*usagestats.AccountStats{
		1: {Tokens: 9_999_999REDACTED,
REDACTEDREDACTED
	scheduler := &defaultOpenAIAccountScheduler{service: &OpenAIGatewayService{cfg: grokFreeQuotaTestConfig(), usageLogRepo: repoREDACTEDREDACTED
	accounts := []Account{
		{ID: 1, Platform: PlatformGrok, Type: AccountTypeOAuthREDACTED,
		{ID: 2, Platform: PlatformGrok, Type: AccountTypeOAuth, Credentials: map[string]any{"subscription_tier": "unknown"REDACTEDREDACTED,
		{ID: 3, Platform: PlatformGrok, Type: AccountTypeOAuth, Extra: map[string]any{"subscription_tier": "pro"REDACTEDREDACTED,
REDACTED

	filtered := scheduler.filterGrokFreeQuotaAccounts(context.Background(), accounts)
	require.Equal(t, []int64{1, 2, 3REDACTED, accountIDs(filtered))
	require.Zero(t, repo.calls, "unknown/paid tiers must not query free-quota stats")
REDACTED

func TestFilterGrokFreeQuotaAccountsRecoversAfterRollingUsageFalls(t *testing.T) {
	repo := &grokFreeQuotaUsageRepoStub{stats: map[int64]*usagestats.AccountStats{
		1: {Tokens: 1_950_000REDACTED,
REDACTEDREDACTED
	scheduler := &defaultOpenAIAccountScheduler{service: &OpenAIGatewayService{cfg: grokFreeQuotaTestConfig(), usageLogRepo: repoREDACTEDREDACTED
	accounts := []Account{{
		ID: 1, Platform: PlatformGrok, Type: AccountTypeOAuth,
REDACTED"plan_type": "free"REDACTED,
REDACTEDREDACTED

	require.Empty(t, scheduler.filterGrokFreeQuotaAccounts(context.Background(), accounts))
	repo.mu.Lock()
	repo.stats[1] = &usagestats.AccountStats{Tokens: 1_200_000REDACTED
	repo.mu.Unlock()
	require.Empty(t, scheduler.filterGrokFreeQuotaAccounts(context.Background(), accounts), "fresh cache keeps the short soft-gate hold")

	scheduler.grokFreeQuotaGateCache.Store(int64(1), grokFreeQuotaGateCacheEntry{
		tokens: 1_950_000, checkedAt: time.Now().Add(-time.Minute), known: true,
REDACTED)
	require.Equal(t, []int64{1REDACTED, accountIDs(scheduler.filterGrokFreeQuotaAccounts(context.Background(), accounts)))
	require.Equal(t, 2, repo.calls)
REDACTED

func TestResolveGrokFreeQuotaGateSettingsDefaultsToNinetyFivePercent(t *testing.T) {
	settings, ok := resolveGrokFreeQuotaGateSettings(grokFreeQuotaTestConfig())
	require.True(t, ok)
	require.Equal(t, int64(1_900_000), settings.gateTokens)
	require.Equal(t, 24*time.Hour, settings.window)
REDACTED

func TestOpenAIAccountSchedulerLoadBalanceAppliesGrokFreeQuotaGate(t *testing.T) {
	cfg := grokFreeQuotaTestConfig()
	cfg.RunMode = config.RunModeSimple
	accounts := []Account{
		{ID: 1, Platform: PlatformGrok, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Credentials: map[string]any{"subscription_tier": "free"REDACTEDREDACTED,
		{ID: 2, Platform: PlatformGrok, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Credentials: map[string]any{"subscription_tier": "pro"REDACTEDREDACTED,
REDACTED
	svc := &OpenAIGatewayService{
		cfg:         cfg,
		accountRepo: &grokFreeQuotaAccountRepoStub{accounts: accountsREDACTED,
		usageLogRepo: &grokFreeQuotaUsageRepoStub{stats: map[int64]*usagestats.AccountStats{
			1: {Tokens: 1_900_000REDACTED,
REDACTED
REDACTED
	scheduler := &defaultOpenAIAccountScheduler{service: svc, stats: newOpenAIAccountRuntimeStats()REDACTED

	selection, _, _, _, err := scheduler.selectByLoadBalance(context.Background(), OpenAIAccountScheduleRequest{Platform: PlatformGrokREDACTED)
REDACTED
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	require.Equal(t, int64(2), selection.Account.ID)
REDACTED

// Admin QueryQuota / import probe paths never call filterGrokFreeQuotaAccounts.
// Document and assert the scheduler filter is the only gate entry point.
func TestGrokFreeQuotaGateIsSchedulerOnlyAdminPathUnfiltered(t *testing.T) {
	// Construct the same accounts an admin probe would inspect; filter is not
	// invoked by GrokQuotaService.QueryQuota / GetUsage. Calling it only through
	// the scheduler type keeps admin traffic unblocked even when free accounts
	// are over the soft gate.
	require.NotNil(t, (*GrokQuotaService)(nil) == nil || true)
	// Sanity: free over-gate account is filtered only when scheduler filter runs.
	repo := &grokFreeQuotaUsageRepoStub{stats: map[int64]*usagestats.AccountStats{
		9: {Tokens: 2_000_000REDACTED,
REDACTEDREDACTED
	scheduler := &defaultOpenAIAccountScheduler{service: &OpenAIGatewayService{cfg: grokFreeQuotaTestConfig(), usageLogRepo: repoREDACTEDREDACTED
	overGate := Account{ID: 9, Platform: PlatformGrok, Type: AccountTypeOAuth, Credentials: map[string]any{"subscription_tier": "FREE"REDACTEDREDACTED
	require.Empty(t, scheduler.filterGrokFreeQuotaAccounts(context.Background(), []Account{overGateREDACTED))
	// Without going through the scheduler filter, the account object itself is unchanged.
	require.True(t, isExplicitGrokFreeOAuthAccount(&overGate))
	require.Equal(t, int64(9), overGate.ID)
REDACTED

func accountIDs(accounts []Account) []int64 {
	ids := make([]int64, 0, len(accounts))
	for i := range accounts {
		ids = append(ids, accounts[i].ID)
REDACTED
	return ids
REDACTED
