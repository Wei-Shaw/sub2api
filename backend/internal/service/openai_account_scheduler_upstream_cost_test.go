package service

import (
	"context"
	"errors"
	"math"
	"strconv"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type upstreamCostTrackingConcurrencyCache struct {
	ConcurrencyCache
	loadMap       map[int64]*AccountLoadInfo
	acquireLimits map[int64][]int
	releases      map[int64]int
	rejectAcquire bool
REDACTED

func (c *upstreamCostTrackingConcurrencyCache) AcquireAccountSlot(_ context.Context, accountID int64, maxConcurrency int, _ string) (bool, error) {
	if c.acquireLimits == nil {
		c.acquireLimits = make(map[int64][]int)
REDACTED
	c.acquireLimits[accountID] = append(c.acquireLimits[accountID], maxConcurrency)
	return !c.rejectAcquire, nil
REDACTED

func (c *upstreamCostTrackingConcurrencyCache) ReleaseAccountSlot(_ context.Context, accountID int64, _ string) error {
	if c.releases == nil {
		c.releases = make(map[int64]int)
REDACTED
	c.releases[accountID]++
	return nil
REDACTED

func (c *upstreamCostTrackingConcurrencyCache) GetAccountsLoadBatch(_ context.Context, accounts []AccountWithConcurrency) (map[int64]*AccountLoadInfo, error) {
	out := make(map[int64]*AccountLoadInfo, len(accounts))
	for _, account := range accounts {
		if load := c.loadMap[account.ID]; load != nil {
			copied := *load
			out[account.ID] = &copied
	REDACTED
REDACTED
	return out, nil
REDACTED

func (c *upstreamCostTrackingConcurrencyCache) limits(accountID int64) []int {
	return append([]int(nil), c.acquireLimits[accountID]...)
REDACTED

func (c *upstreamCostTrackingConcurrencyCache) releaseCount(accountID int64) int {
	return c.releases[accountID]
REDACTED

func (c *upstreamCostTrackingConcurrencyCache) totalAcquires() int {
	total := 0
	for _, limits := range c.acquireLimits {
		total += len(limits)
REDACTED
	return total
REDACTED

type upstreamCostCountingAccountRepo struct {
	AccountRepository
	accounts map[int64]*Account
	getCalls int
REDACTED

func (r *upstreamCostCountingAccountRepo) GetByID(_ context.Context, accountID int64) (*Account, error) {
	r.getCalls++
	account := r.accounts[accountID]
	if account == nil {
		return nil, errors.New("account not found")
REDACTED
	cloned := *account
	return &cloned, nil
REDACTED

func (r *upstreamCostCountingAccountRepo) calls() int {
	return r.getCalls
REDACTED

func upstreamCostTestAccount(id int64, status string, rate float64, receivedAt time.Time, interval time.Duration) *Account {
REDACTED
		ID:       id,
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Extra: map[string]any{
			UpstreamBillingProbeExtraKey: map[string]any{
				"status": status,
				"data": map[string]any{
					"billing_scope":             "token",
					"resolved_rate_multiplier":  rate,
					"peak_rate_enabled":         false,
					"effective_rate_multiplier": rate,
			REDACTED,
				"received_at":     receivedAt.UTC().Format(time.RFC3339Nano),
				"fresh_until":     receivedAt.Add(2 * interval).UTC().Format(time.RFC3339Nano),
				"last_attempt_at": receivedAt.UTC().Format(time.RFC3339Nano),
				"next_probe_at":   receivedAt.Add(interval).UTC().Format(time.RFC3339Nano),
		REDACTED,
	REDACTED,
REDACTED
REDACTED

func upstreamCostTestOAuthAccount(id int64) *Account {
REDACTEDID: id, Platform: PlatformOpenAI, Type: AccountTypeOAuthREDACTED
REDACTED

func TestAdvancedCostSchedulerUsesTopKOverflowWhenPreferredAccountIsKnownFull(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()
	defer resetOpenAIAdvancedSchedulerSettingCacheForTest()

	now := time.Now()
	cheap := upstreamCostTestAccount(1, UpstreamBillingProbeStatusOK, 0.03, now.Add(-time.Minute), 30*time.Minute)
	expensive := upstreamCostTestAccount(2, UpstreamBillingProbeStatusOK, 0.8, now.Add(-time.Minute), 30*time.Minute)
	for _, account := range []*Account{cheap, expensiveREDACTED {
		account.Status = StatusActive
		account.Schedulable = true
		account.Concurrency = 1
REDACTED
	cache := &upstreamCostTrackingConcurrencyCache{loadMap: map[int64]*AccountLoadInfo{
		cheap.ID:     {AccountID: cheap.ID, CurrentConcurrency: 1, LoadRate: 100REDACTED,
		expensive.ID: {AccountID: expensive.IDREDACTED,
REDACTEDREDACTED
	cfg := &config.Config{REDACTED
	cfg.Gateway.OpenAIWS.LBTopK = 1
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.UpstreamCost = 1
	svc := &OpenAIGatewayService{
		accountRepo:        schedulerTestOpenAIAccountRepo{accounts: []Account{*cheap, *expensiveREDACTEDREDACTED,
		cfg:                cfg,
		rateLimitService:   newOpenAIAdvancedSchedulerRateLimitService("true"),
		concurrencyService: NewConcurrencyService(cache),
REDACTED
	groupID := int64(1)

	selection, _, err := svc.SelectAccountWithScheduler(context.Background(), &groupID, "", "", "gpt-test", nil, OpenAIUpstreamTransportAny, false)
REDACTED
	require.Equal(t, expensive.ID, selection.Account.ID)
	require.Empty(t, cache.limits(cheap.ID))
	require.Equal(t, []int{1REDACTED, cache.limits(expensive.ID))
	selection.ReleaseFunc()
REDACTED

func TestAdvancedSchedulerCapsRejectedCostOverflowAcquires(t *testing.T) {
	selectionOrder := make([]openAIAccountCandidateScore, 0, 15_000)
	for id := int64(1); id <= 15_000; id++ {
		account := &Account{ID: id, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1REDACTED
		selectionOrder = append(selectionOrder, openAIAccountCandidateScore{
			account: account, loadInfo: &AccountLoadInfo{AccountID: idREDACTED, loadKnown: false,
	REDACTED)
REDACTED
	cache := &upstreamCostTrackingConcurrencyCache{rejectAcquire: trueREDACTED
	scheduler := &defaultOpenAIAccountScheduler{service: &OpenAIGatewayService{
		concurrencyService: NewConcurrencyService(cache),
REDACTEDREDACTED

	selection, _, err := scheduler.tryAcquireOpenAISelectionOrder(
		context.Background(), OpenAIAccountScheduleRequest{Platform: PlatformOpenAIREDACTED, selectionOrder,
	)

REDACTED
	require.Nil(t, selection)
	require.Equal(t, openAIAccountSelectionProbeLimit, cache.totalAcquires())
REDACTED

func TestOpenAICostOverflowExpandedOnlyWhenCostAddsCandidates(t *testing.T) {
	candidates := []openAIAccountCandidateScore{
		{account: &Account{ID: 1, Extra: map[string]any{"openai_compact_supported": trueREDACTEDREDACTEDREDACTED,
		{account: &Account{ID: 2REDACTEDREDACTED,
REDACTED
	plan := openAIAccountLoadPlan{candidates: candidates, topK: 1, includeOverflowFallback: trueREDACTED
	require.True(t, openAICostOverflowExpanded(OpenAIAccountScheduleRequest{REDACTED, plan))
	require.False(t, openAICostOverflowExpanded(OpenAIAccountScheduleRequest{RequireCompact: trueREDACTED, plan),
		"one candidate per compact tier does not expand either tier's top-k")
	plan.topK = len(candidates)
	require.False(t, openAICostOverflowExpanded(OpenAIAccountScheduleRequest{REDACTED, plan))
	plan.includeOverflowFallback = false
	plan.topK = 1
	require.False(t, openAICostOverflowExpanded(OpenAIAccountScheduleRequest{REDACTED, plan))
REDACTED

func TestAdvancedSchedulerKnownFullOverflowStillFindsAvailableAccount(t *testing.T) {
	selectionOrder := make([]openAIAccountCandidateScore, 0, openAIAccountSelectionProbeLimit+2)
	for id := int64(1); id <= openAIAccountSelectionProbeLimit+1; id++ {
		account := &Account{ID: id, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1REDACTED
		selectionOrder = append(selectionOrder, openAIAccountCandidateScore{
			account:   account,
			loadInfo:  &AccountLoadInfo{AccountID: id, CurrentConcurrency: 1, LoadRate: 100REDACTED,
			loadKnown: true,
	REDACTED)
REDACTED
	availableID := int64(openAIAccountSelectionProbeLimit + 2)
	available := &Account{ID: availableID, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1REDACTED
	selectionOrder = append(selectionOrder, openAIAccountCandidateScore{
		account: available, loadInfo: &AccountLoadInfo{AccountID: availableIDREDACTED, loadKnown: true,
REDACTED)
	cache := &upstreamCostTrackingConcurrencyCache{REDACTED
	scheduler := &defaultOpenAIAccountScheduler{service: &OpenAIGatewayService{
		concurrencyService: NewConcurrencyService(cache),
REDACTEDREDACTED

	selection, _, err := scheduler.tryAcquireOpenAISelectionOrder(
		context.Background(), OpenAIAccountScheduleRequest{Platform: PlatformOpenAIREDACTED, selectionOrder,
	)

REDACTED
	require.NotNil(t, selection)
	require.Equal(t, availableID, selection.Account.ID)
	require.Equal(t, 1, cache.totalAcquires())
	selection.ReleaseFunc()
REDACTED

func TestAdvancedSchedulerSharesProbeBudgetWithFallbackDBRechecks(t *testing.T) {
	const size = 15_000
	latestAccounts := make(map[int64]*Account, size)
	snapshotAccounts := make(map[int64]*Account, size)
	selectionOrder := make([]openAIAccountCandidateScore, 0, size)
	for id := int64(1); id <= size; id++ {
		stale := &Account{ID: id, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1REDACTED
		latest := *stale
		latest.Status = StatusDisabled
		snapshotAccounts[id] = stale
		latestAccounts[id] = &latest
		selectionOrder = append(selectionOrder, openAIAccountCandidateScore{
			account: stale, loadInfo: &AccountLoadInfo{AccountID: idREDACTED, loadKnown: false,
	REDACTED)
REDACTED
	repo := &upstreamCostCountingAccountRepo{accounts: latestAccountsREDACTED
	cache := &upstreamCostTrackingConcurrencyCache{REDACTED
	scheduler := &defaultOpenAIAccountScheduler{service: &OpenAIGatewayService{
		accountRepo:        repo,
		schedulerSnapshot:  &SchedulerSnapshotService{cache: &openAISnapshotCacheStub{accountsByID: snapshotAccountsREDACTEDREDACTED,
		concurrencyService: NewConcurrencyService(cache),
REDACTEDREDACTED
	budget := newOpenAISelectionProbeBudget()
	budget.enableLimit()
	req := OpenAIAccountScheduleRequest{Platform: PlatformOpenAIREDACTED

	selection, _, err := scheduler.tryAcquireOpenAISelectionOrderWithBudget(context.Background(), req, selectionOrder, budget)
REDACTED
	require.Nil(t, selection)
	selection, _, _, _, err = scheduler.finishLoadBalanceSelectionFallback(
		context.Background(), req, openAIAccountLoadSelectionAttempt{selectionOrder: selectionOrderREDACTED, budget, openAISelectionFilterStats{REDACTED,
	)

REDACTED
	require.Nil(t, selection)
	require.Equal(t, openAIAccountSelectionProbeLimit, cache.totalAcquires())
	require.Equal(t, openAIAccountSelectionProbeLimit, repo.calls())
REDACTED

func TestAdvancedCostSchedulerKeepsCompactSupportedOverflowAheadOfUnknown(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()
	defer resetOpenAIAdvancedSchedulerSettingCacheForTest()

	now := time.Now()
	preferred := upstreamCostTestAccount(11, UpstreamBillingProbeStatusOK, 0.01, now.Add(-time.Minute), 30*time.Minute)
	overflow := upstreamCostTestAccount(12, UpstreamBillingProbeStatusOK, 0.1, now.Add(-time.Minute), 30*time.Minute)
	unknown := upstreamCostTestAccount(13, UpstreamBillingProbeStatusOK, 0.001, now.Add(-time.Minute), 30*time.Minute)
	preferred.Extra["openai_compact_supported"] = true
	overflow.Extra["openai_compact_supported"] = true
	for _, account := range []*Account{preferred, overflow, unknownREDACTED {
		account.Status = StatusActive
		account.Schedulable = true
		account.Concurrency = 1
REDACTED
	cache := &upstreamCostTrackingConcurrencyCache{loadMap: map[int64]*AccountLoadInfo{
		preferred.ID: {AccountID: preferred.ID, CurrentConcurrency: 1, LoadRate: 100REDACTED,
		overflow.ID:  {AccountID: overflow.IDREDACTED,
		unknown.ID:   {AccountID: unknown.IDREDACTED,
REDACTEDREDACTED
	cfg := &config.Config{REDACTED
	cfg.Gateway.OpenAIWS.LBTopK = 1
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.UpstreamCost = 1
	svc := &OpenAIGatewayService{
		accountRepo:        schedulerTestOpenAIAccountRepo{accounts: []Account{*preferred, *overflow, *unknownREDACTEDREDACTED,
		cfg:                cfg,
		rateLimitService:   newOpenAIAdvancedSchedulerRateLimitService("true"),
		concurrencyService: NewConcurrencyService(cache),
REDACTED
	groupID := int64(1)

	selection, _, err := svc.SelectAccountWithScheduler(context.Background(), &groupID, "", "", "gpt-test", nil, OpenAIUpstreamTransportAny, true)
REDACTED
	require.Equal(t, overflow.ID, selection.Account.ID)
	require.Empty(t, cache.limits(preferred.ID))
	require.Equal(t, []int{1REDACTED, cache.limits(overflow.ID))
	require.Empty(t, cache.limits(unknown.ID))
	selection.ReleaseFunc()
REDACTED

func TestAdvancedSchedulerUnknownLoadFailsOpen(t *testing.T) {
	account := &Account{ID: 21, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1REDACTED
	cache := &upstreamCostTrackingConcurrencyCache{REDACTED
	scheduler := &defaultOpenAIAccountScheduler{service: &OpenAIGatewayService{concurrencyService: NewConcurrencyService(cache)REDACTEDREDACTED

	selection, _, err := scheduler.tryAcquireOpenAISelectionOrder(context.Background(), OpenAIAccountScheduleRequest{Platform: PlatformOpenAIREDACTED, []openAIAccountCandidateScore{{
		account: account, loadInfo: &AccountLoadInfo{AccountID: account.ID, CurrentConcurrency: 99REDACTED, loadKnown: false,
REDACTEDREDACTED)
REDACTED
	require.NotNil(t, selection)
	require.Equal(t, []int{1REDACTED, cache.limits(account.ID))
	selection.ReleaseFunc()
REDACTED

func TestAdvancedSchedulerReleasesSlotWhenDBDisablesCandidate(t *testing.T) {
	stale := &Account{ID: 31, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1REDACTED
	backup := &Account{ID: 32, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1REDACTED
	disabled := *stale
	disabled.Status = StatusDisabled
	repo := &upstreamCostCountingAccountRepo{accounts: map[int64]*Account{stale.ID: &disabled, backup.ID: backupREDACTEDREDACTED
	snapshot := &openAISnapshotCacheStub{accountsByID: map[int64]*Account{stale.ID: stale, backup.ID: backupREDACTEDREDACTED
	cache := &upstreamCostTrackingConcurrencyCache{REDACTED
	scheduler := &defaultOpenAIAccountScheduler{service: &OpenAIGatewayService{
		accountRepo:        repo,
		schedulerSnapshot:  &SchedulerSnapshotService{cache: snapshotREDACTED,
		concurrencyService: NewConcurrencyService(cache),
REDACTEDREDACTED

	selection, _, err := scheduler.tryAcquireOpenAISelectionOrder(context.Background(), OpenAIAccountScheduleRequest{Platform: PlatformOpenAIREDACTED, []openAIAccountCandidateScore{
		{account: stale, loadInfo: &AccountLoadInfo{AccountID: stale.IDREDACTED, loadKnown: trueREDACTED,
		{account: backup, loadInfo: &AccountLoadInfo{AccountID: backup.IDREDACTED, loadKnown: trueREDACTED,
REDACTED)
REDACTED
	require.Equal(t, backup.ID, selection.Account.ID)
	require.Equal(t, 1, cache.releaseCount(stale.ID))
	selection.ReleaseFunc()
REDACTED

func TestAdvancedSchedulerReacquiresOnceWhenDBConcurrencyChanges(t *testing.T) {
	stale := &Account{ID: 41, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 10REDACTED
	latest := *stale
	latest.Concurrency = 1
	repo := &upstreamCostCountingAccountRepo{accounts: map[int64]*Account{stale.ID: &latestREDACTEDREDACTED
	snapshot := &openAISnapshotCacheStub{accountsByID: map[int64]*Account{stale.ID: staleREDACTEDREDACTED
	cache := &upstreamCostTrackingConcurrencyCache{REDACTED
	scheduler := &defaultOpenAIAccountScheduler{service: &OpenAIGatewayService{
		accountRepo:        repo,
		schedulerSnapshot:  &SchedulerSnapshotService{cache: snapshotREDACTED,
		concurrencyService: NewConcurrencyService(cache),
REDACTEDREDACTED

	selection, _, err := scheduler.tryAcquireOpenAISelectionOrder(context.Background(), OpenAIAccountScheduleRequest{Platform: PlatformOpenAIREDACTED, []openAIAccountCandidateScore{{
		account: stale, loadInfo: &AccountLoadInfo{AccountID: stale.IDREDACTED, loadKnown: true,
REDACTEDREDACTED)
REDACTED
	require.Equal(t, 1, selection.Account.Concurrency)
	require.Equal(t, []int{10, 1REDACTED, cache.limits(stale.ID))
	require.Equal(t, 1, cache.releaseCount(stale.ID))
	selection.ReleaseFunc()
REDACTED

func TestAdvancedSchedulerKnownFullPoolsDoNotRecheckDB(t *testing.T) {
	for _, size := range []int{100, 15_000REDACTED {
		t.Run(strconv.Itoa(size), func(t *testing.T) {
			accounts := make(map[int64]*Account, size)
			selectionOrder := make([]openAIAccountCandidateScore, 0, size)
			for i := 1; i <= size; i++ {
				account := &Account{ID: int64(i), Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1REDACTED
				accounts[account.ID] = account
				selectionOrder = append(selectionOrder, openAIAccountCandidateScore{
					account:   account,
					loadInfo:  &AccountLoadInfo{AccountID: account.ID, CurrentConcurrency: 1, LoadRate: 100REDACTED,
					loadKnown: true,
			REDACTED)
		REDACTED
			repo := &upstreamCostCountingAccountRepo{accounts: accountsREDACTED
			cache := &upstreamCostTrackingConcurrencyCache{REDACTED
			scheduler := &defaultOpenAIAccountScheduler{service: &OpenAIGatewayService{
				accountRepo:        repo,
				schedulerSnapshot:  &SchedulerSnapshotService{cache: &openAISnapshotCacheStub{accountsByID: accountsREDACTEDREDACTED,
				concurrencyService: NewConcurrencyService(cache),
		REDACTEDREDACTED

			selection, _, err := scheduler.tryAcquireOpenAISelectionOrder(context.Background(), OpenAIAccountScheduleRequest{Platform: PlatformOpenAIREDACTED, selectionOrder)
		REDACTED
			require.Nil(t, selection)
			require.Zero(t, repo.calls())
			require.Zero(t, cache.totalAcquires())
	REDACTED)
REDACTED
REDACTED

func TestOpenAIFreshUpstreamBillingRateRecomputesPeakAtSelectionTime(t *testing.T) {
	receivedAt := time.Date(2026, 7, 13, 17, 30, 0, 0, time.UTC)
	account := upstreamCostTestAccount(1, UpstreamBillingProbeStatusOK, 0.4, receivedAt, time.Hour)
	snapshot, ok := account.Extra[UpstreamBillingProbeExtraKey].(map[string]any)
	require.True(t, ok)
	snapshot["data"] = map[string]any{
		"billing_scope":             "token",
		"resolved_rate_multiplier":  0.4,
		"peak_rate_enabled":         true,
		"peak_start":                "09:00",
		"peak_end":                  "18:00",
		"peak_rate_multiplier":      2.0,
		"applied_peak_multiplier":   2.0,
		"effective_rate_multiplier": 0.8,
		"timezone":                  "UTC",
REDACTED

	duringPeak, ok := openAIFreshUpstreamBillingRate(account, time.Date(2026, 7, 13, 17, 59, 0, 0, time.UTC))
	require.True(t, ok)
	require.Equal(t, 0.8, duringPeak)

	afterPeak, ok := openAIFreshUpstreamBillingRate(account, time.Date(2026, 7, 13, 18, 1, 0, 0, time.UTC))
	require.True(t, ok)
	require.Equal(t, 0.4, afterPeak)
REDACTED

func TestOpenAIUpstreamCostFactorsSparseProbeIsNeutral(t *testing.T) {
	now := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)
	accounts := make([]*Account, 0, 10)
	accounts = append(accounts, upstreamCostTestAccount(1, UpstreamBillingProbeStatusOK, 1, now.Add(-time.Minute), 30*time.Minute))
	for id := int64(2); id <= 10; id++ {
		accounts = append(accounts, &Account{
			ID:       id,
			Platform: PlatformOpenAI,
			Type:     AccountTypeAPIKey,
			Extra: map[string]any{
				UpstreamBillingProbeExtraKey: map[string]any{
					"status":          UpstreamBillingProbeStatusFailed,
					"last_attempt_at": now.UTC().Format(time.RFC3339Nano),
					"next_probe_at":   now.Add(time.Hour).UTC().Format(time.RFC3339Nano),
			REDACTED,
		REDACTED,
	REDACTED)
REDACTED

	factors := openAIUpstreamCostFactors(accounts, now, defaultOpenAIOAuthSchedulingRateMultiplier)
	for id := int64(1); id <= 10; id++ {
		require.Equal(t, openAIUpstreamCostNeutralFactor, factors[id])
REDACTED
REDACTED

func TestOpenAIUpstreamCostFactorsCoverageShrinksSparseSignal(t *testing.T) {
	now := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)
	accounts := []*Account{
		upstreamCostTestAccount(1, UpstreamBillingProbeStatusOK, 0.03, now.Add(-time.Minute), 30*time.Minute),
		upstreamCostTestAccount(2, UpstreamBillingProbeStatusOK, 0.8, now.Add(-time.Minute), 30*time.Minute),
REDACTED
	for id := int64(3); id <= 10; id++ {
		accounts = append(accounts, &Account{ID: id, Platform: PlatformOpenAI, Type: AccountTypeAPIKeyREDACTED)
REDACTED

	factors := openAIUpstreamCostFactors(accounts, now, defaultOpenAIOAuthSchedulingRateMultiplier)
	center := math.Sqrt(0.03 * 0.8)
	require.InDelta(t, 0.5+0.2*(1/(1+0.03/center)-0.5), factors[1], 1e-12)
	require.InDelta(t, 0.5+0.2*(1/(1+0.8/center)-0.5), factors[2], 1e-12)
	require.Equal(t, openAIUpstreamCostNeutralFactor, factors[3])
REDACTED

func TestOpenAIUpstreamCostFactorsUseMedianAgainstOutlier(t *testing.T) {
	now := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)
	accounts := []*Account{
		upstreamCostTestAccount(1, UpstreamBillingProbeStatusOK, 0.1, now.Add(-time.Minute), 30*time.Minute),
		upstreamCostTestAccount(2, UpstreamBillingProbeStatusOK, 0.2, now.Add(-time.Minute), 30*time.Minute),
		upstreamCostTestAccount(3, UpstreamBillingProbeStatusOK, 100, now.Add(-time.Minute), 30*time.Minute),
REDACTED

	factors := openAIUpstreamCostFactors(accounts, now, defaultOpenAIOAuthSchedulingRateMultiplier)
	require.InDelta(t, 2.0/3.0, factors[1], 1e-12)
	require.InDelta(t, 0.5, factors[2], 1e-12)
	require.InDelta(t, 1/(1+100/0.2), factors[3], 1e-12)
REDACTED

func TestOpenAILegacyUpstreamRateOrderRequiresComparableRates(t *testing.T) {
	now := time.Now()
	oneKnown := newOpenAILegacyUpstreamRateOrder([]*Account{
		upstreamCostTestAccount(1, UpstreamBillingProbeStatusOK, 0.03, now.Add(-time.Minute), 30*time.Minute),
		{ID: 2, Platform: PlatformOpenAI, Type: AccountTypeAPIKeyREDACTED,
REDACTED, now, defaultOpenAIOAuthSchedulingRateMultiplier)
	require.False(t, oneKnown.enabled)

	allEqual := newOpenAILegacyUpstreamRateOrder([]*Account{
		upstreamCostTestAccount(1, UpstreamBillingProbeStatusOK, 0.3, now.Add(-time.Minute), 30*time.Minute),
		upstreamCostTestAccount(2, UpstreamBillingProbeStatusOK, 0.3, now.Add(-time.Minute), 30*time.Minute),
REDACTED, now, defaultOpenAIOAuthSchedulingRateMultiplier)
	require.False(t, allEqual.enabled)

	distinct := newOpenAILegacyUpstreamRateOrder([]*Account{
		upstreamCostTestAccount(1, UpstreamBillingProbeStatusOK, 0.03, now.Add(-time.Minute), 30*time.Minute),
		upstreamCostTestAccount(2, UpstreamBillingProbeStatusOK, 0.8, now.Add(-time.Minute), 30*time.Minute),
		{ID: 3, Platform: PlatformOpenAI, Type: AccountTypeAPIKeyREDACTED,
REDACTED, now, defaultOpenAIOAuthSchedulingRateMultiplier)
	require.True(t, distinct.enabled)
	require.Negative(t, distinct.compare(&Account{ID: 1REDACTED, &Account{ID: 2REDACTED))
	require.Negative(t, distinct.compare(&Account{ID: 2REDACTED, &Account{ID: 3REDACTED))
REDACTED

func TestOpenAISchedulingRatePlacesOAuthAtConfiguredReference(t *testing.T) {
	now := time.Now()
	cheap := upstreamCostTestAccount(1, UpstreamBillingProbeStatusOK, 0.02, now.Add(-time.Minute), 30*time.Minute)
	oauth := upstreamCostTestOAuthAccount(2)
	expensive := upstreamCostTestAccount(3, UpstreamBillingProbeStatusOK, 0.12, now.Add(-time.Minute), 30*time.Minute)

	order := newOpenAILegacyUpstreamRateOrder([]*Account{cheap, oauth, expensiveREDACTED, now, 0.05)
	require.True(t, order.enabled)
	require.Negative(t, order.compare(cheap, oauth))
	require.Negative(t, order.compare(oauth, expensive))

	factors := openAIUpstreamCostFactors([]*Account{cheap, oauth, expensiveREDACTED, now, 0.05)
	require.Greater(t, factors[cheap.ID], factors[oauth.ID])
	require.Greater(t, factors[oauth.ID], factors[expensive.ID])
REDACTED

func TestOpenAIGatewayServiceLegacyLowRatePriorityUsesConfiguredOAuthReference(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()
	defer resetOpenAIAdvancedSchedulerSettingCacheForTest()

	now := time.Now()
	cheap := upstreamCostTestAccount(1, UpstreamBillingProbeStatusOK, 0.02, now.Add(-time.Minute), 30*time.Minute)
	oauth := upstreamCostTestOAuthAccount(2)
	expensive := upstreamCostTestAccount(3, UpstreamBillingProbeStatusOK, 0.12, now.Add(-time.Minute), 30*time.Minute)
	for _, account := range []*Account{cheap, oauth, expensiveREDACTED {
		account.Status = StatusActive
		account.Schedulable = true
		account.Concurrency = 1
REDACTED
	cheap.Priority, oauth.Priority, expensive.Priority = 20, 10, 0

	settings := &openAIAdvancedSchedulerSettingRepoStub{values: map[string]string{
		openAIAdvancedSchedulerSettingKey:              "false",
		SettingKeyOpenAILowUpstreamRatePriorityEnabled: "true",
		SettingKeyOpenAIOAuthSchedulingRateMultiplier:  "0.05",
REDACTEDREDACTED
	cfg := &config.Config{REDACTED
	svc := &OpenAIGatewayService{
		accountRepo:      schedulerTestOpenAIAccountRepo{accounts: []Account{*cheap, *oauth, *expensiveREDACTEDREDACTED,
		cache:            &schedulerTestGatewayCache{REDACTED,
		cfg:              cfg,
		rateLimitService: &RateLimitService{settingService: NewSettingService(settings, cfg)REDACTED,
REDACTED
	groupID := int64(1)

	first, _, err := svc.SelectAccountWithScheduler(context.Background(), &groupID, "", "", "gpt-test", nil, OpenAIUpstreamTransportAny, false)
REDACTED
	require.Equal(t, cheap.ID, first.Account.ID)

	second, _, err := svc.SelectAccountWithScheduler(context.Background(), &groupID, "", "", "gpt-test", map[int64]struct{REDACTED{cheap.ID: {REDACTEDREDACTED, OpenAIUpstreamTransportAny, false)
REDACTED
	require.Equal(t, oauth.ID, second.Account.ID)
REDACTED

func TestOpenAIModelsSelectionIgnoresTokenCostSignal(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()
	defer resetOpenAIAdvancedSchedulerSettingCacheForTest()

	now := time.Now()
	cheap := upstreamCostTestAccount(51, UpstreamBillingProbeStatusOK, 0.02, now.Add(-time.Minute), 30*time.Minute)
	expensive := upstreamCostTestAccount(52, UpstreamBillingProbeStatusOK, 0.8, now.Add(-time.Minute), 30*time.Minute)
	for _, account := range []*Account{cheap, expensiveREDACTED {
		account.Status = StatusActive
		account.Schedulable = true
		account.Concurrency = 1
REDACTED
	cheap.Priority = 10
	expensive.Priority = 0
	settings := &openAIAdvancedSchedulerSettingRepoStub{values: map[string]string{
		SettingKeyOpenAILowUpstreamRatePriorityEnabled: "true",
REDACTEDREDACTED
	cfg := &config.Config{REDACTED
	svc := &OpenAIGatewayService{
		accountRepo:      schedulerTestOpenAIAccountRepo{accounts: []Account{*cheap, *expensiveREDACTEDREDACTED,
		cfg:              cfg,
		rateLimitService: &RateLimitService{settingService: NewSettingService(settings, cfg)REDACTED,
REDACTED

	account, err := svc.SelectAccountForModelWithExclusions(context.Background(), nil, "", "", nil)
REDACTED
	require.Equal(t, expensive.ID, account.ID)
REDACTED

func TestOpenAIGatewayServiceLegacyLowRatePriorityIsIndependentFromAdvancedScheduler(t *testing.T) {
	now := time.Now()
	cheap := upstreamCostTestAccount(1, UpstreamBillingProbeStatusOK, 0.03, now.Add(-time.Minute), 30*time.Minute)
	cheap.Status, cheap.Schedulable, cheap.Concurrency, cheap.Priority = StatusActive, true, 1, 10
	expensive := upstreamCostTestAccount(2, UpstreamBillingProbeStatusOK, 0.8, now.Add(-time.Minute), 30*time.Minute)
	expensive.Status, expensive.Schedulable, expensive.Concurrency, expensive.Priority = StatusActive, true, 1, 0
	accounts := []Account{*cheap, *expensiveREDACTED
	groupID := int64(1)

	tests := []struct {
		name      string
		enabled   bool
		loadBatch bool
		loadErr   error
		wantID    int64
REDACTED{
		{name: "switch off keeps priority first", loadBatch: true, wantID: 2REDACTED,
		{name: "load batch", enabled: true, loadBatch: true, wantID: 1REDACTED,
		{name: "load batch disabled", enabled: true, wantID: 1REDACTED,
		{name: "load lookup failure", enabled: true, loadBatch: true, loadErr: errors.New("load unavailable"), wantID: 1REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetOpenAIAdvancedSchedulerSettingCacheForTest()
			settings := &openAIAdvancedSchedulerSettingRepoStub{values: map[string]string{
				openAIAdvancedSchedulerSettingKey:              "false",
				SettingKeyOpenAILowUpstreamRatePriorityEnabled: strconv.FormatBool(tt.enabled),
		REDACTEDREDACTED
			cfg := &config.Config{REDACTED
			cfg.Gateway.Scheduling.LoadBatchEnabled = tt.loadBatch
			svc := &OpenAIGatewayService{
				accountRepo:      schedulerTestOpenAIAccountRepo{accounts: accountsREDACTED,
				cache:            &schedulerTestGatewayCache{REDACTED,
				cfg:              cfg,
				rateLimitService: &RateLimitService{settingService: NewSettingService(settings, cfg)REDACTED,
				concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{
					loadBatchErr: tt.loadErr,
					loadMap: map[int64]*AccountLoadInfo{
						1: {AccountID: 1, LoadRate: 90REDACTED,
						2: {AccountID: 2, LoadRate: 10REDACTED,
				REDACTED,
			REDACTED),
		REDACTED

			selection, _, err := svc.SelectAccountWithScheduler(context.Background(), &groupID, "", "", "gpt-test", nil, OpenAIUpstreamTransportAny, false)
		REDACTED
			require.Equal(t, tt.wantID, selection.Account.ID)
			if selection.ReleaseFunc != nil {
				selection.ReleaseFunc()
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestOpenAIGatewayServiceAdvancedSchedulerIgnoresLegacyLowRateSwitch(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()
	defer resetOpenAIAdvancedSchedulerSettingCacheForTest()
	now := time.Now()
	cheap := upstreamCostTestAccount(1, UpstreamBillingProbeStatusOK, 0.03, now.Add(-time.Minute), 30*time.Minute)
	cheap.Status, cheap.Schedulable, cheap.Concurrency, cheap.Priority = StatusActive, true, 1, 10
	expensive := upstreamCostTestAccount(2, UpstreamBillingProbeStatusOK, 0.8, now.Add(-time.Minute), 30*time.Minute)
	expensive.Status, expensive.Schedulable, expensive.Concurrency, expensive.Priority = StatusActive, true, 1, 0
	settings := &openAIAdvancedSchedulerSettingRepoStub{values: map[string]string{
		openAIAdvancedSchedulerSettingKey:              "true",
		SettingKeyOpenAILowUpstreamRatePriorityEnabled: "true",
REDACTEDREDACTED
	cfg := &config.Config{REDACTED
	cfg.Gateway.OpenAIWS.LBTopK = 1
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.Priority = 1
	svc := &OpenAIGatewayService{
		accountRepo:        schedulerTestOpenAIAccountRepo{accounts: []Account{*cheap, *expensiveREDACTEDREDACTED,
		cache:              &schedulerTestGatewayCache{REDACTED,
		cfg:                cfg,
		rateLimitService:   &RateLimitService{settingService: NewSettingService(settings, cfg)REDACTED,
		concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{REDACTED),
REDACTED
	groupID := int64(1)

	selection, _, err := svc.SelectAccountWithScheduler(context.Background(), &groupID, "", "", "gpt-test", nil, OpenAIUpstreamTransportAny, false)
REDACTED
	require.Equal(t, int64(2), selection.Account.ID)
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
REDACTED
REDACTED

func TestOpenAIGatewayServiceLegacyLowRatePrioritySkipsCooledDownAccount(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()
	defer resetOpenAIAdvancedSchedulerSettingCacheForTest()

	now := time.Now()
	cheap := upstreamCostTestAccount(1, UpstreamBillingProbeStatusOK, 0.03, now.Add(-time.Minute), 30*time.Minute)
	cheap.Status, cheap.Schedulable, cheap.Concurrency, cheap.Priority = StatusActive, true, 1, 10
	cooldownUntil := now.Add(time.Minute)
	cheap.TempUnschedulableUntil = &cooldownUntil
	expensive := upstreamCostTestAccount(2, UpstreamBillingProbeStatusOK, 0.8, now.Add(-time.Minute), 30*time.Minute)
	expensive.Status, expensive.Schedulable, expensive.Concurrency, expensive.Priority = StatusActive, true, 1, 0
	settings := &openAIAdvancedSchedulerSettingRepoStub{values: map[string]string{
		openAIAdvancedSchedulerSettingKey:              "false",
		SettingKeyOpenAILowUpstreamRatePriorityEnabled: "true",
REDACTEDREDACTED
	cfg := &config.Config{REDACTED
	cfg.Gateway.Scheduling.LoadBatchEnabled = true
	svc := &OpenAIGatewayService{
		accountRepo:      schedulerTestOpenAIAccountRepo{accounts: []Account{*cheap, *expensiveREDACTEDREDACTED,
		cache:            &schedulerTestGatewayCache{REDACTED,
		cfg:              cfg,
		rateLimitService: &RateLimitService{settingService: NewSettingService(settings, cfg)REDACTED,
		concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{loadMap: map[int64]*AccountLoadInfo{
			1: {AccountID: 1REDACTED,
			2: {AccountID: 2REDACTED,
	REDACTEDREDACTED),
REDACTED
	groupID := int64(1)

	selection, _, err := svc.SelectAccountWithScheduler(context.Background(), &groupID, "", "", "gpt-test", nil, OpenAIUpstreamTransportAny, false)
REDACTED
	require.Equal(t, int64(2), selection.Account.ID)
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
REDACTED
REDACTED

func TestOpenAIFreshUpstreamBillingRateUsesFreshCachedSuccessOnly(t *testing.T) {
	now := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name    string
		account *Account
		wantOK  bool
REDACTED{
		{name: "fresh", account: upstreamCostTestAccount(1, UpstreamBillingProbeStatusOK, 0.3, now.Add(-time.Minute), 30*time.Minute), wantOK: trueREDACTED,
		{name: "zero rate", account: upstreamCostTestAccount(2, UpstreamBillingProbeStatusOK, 0, now.Add(-time.Minute), 30*time.Minute), wantOK: trueREDACTED,
		{name: "transient failure with fresh cache", account: upstreamCostTestAccount(3, UpstreamBillingProbeStatusFailed, 0.3, now.Add(-time.Minute), 30*time.Minute), wantOK: trueREDACTED,
		{name: "stale", account: upstreamCostTestAccount(4, UpstreamBillingProbeStatusOK, 0.3, now.Add(-61*time.Minute), 30*time.Minute)REDACTED,
		{name: "future", account: upstreamCostTestAccount(5, UpstreamBillingProbeStatusOK, 0.3, now.Add(time.Minute), 30*time.Minute)REDACTED,
		{name: "unsupported", account: upstreamCostTestAccount(6, UpstreamBillingProbeStatusUnsupported, 0.3, now.Add(-time.Minute), 30*time.Minute)REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, ok := openAIFreshUpstreamBillingRate(tt.account, now)
			require.Equal(t, tt.wantOK, ok)
	REDACTED)
REDACTED
REDACTED

func TestBuildOpenAISelectionOrderIncludesOverflowOnlyForCostScheduling(t *testing.T) {
	scheduler := &defaultOpenAIAccountScheduler{REDACTED
	candidates := []openAIAccountCandidateScore{
		{account: &Account{ID: 1REDACTED, loadInfo: &AccountLoadInfo{REDACTED, score: 3REDACTED,
		{account: &Account{ID: 2REDACTED, loadInfo: &AccountLoadInfo{REDACTED, score: 2REDACTED,
		{account: &Account{ID: 3REDACTED, loadInfo: &AccountLoadInfo{REDACTED, score: 1REDACTED,
REDACTED

	legacy := scheduler.buildOpenAISelectionOrder(OpenAIAccountScheduleRequest{REDACTED, openAIAccountLoadPlan{
		candidates: candidates,
		topK:       1,
REDACTED)
	require.Len(t, legacy, 1)

	costAware := scheduler.buildOpenAISelectionOrder(OpenAIAccountScheduleRequest{REDACTED, openAIAccountLoadPlan{
		candidates:              candidates,
		topK:                    1,
		includeOverflowFallback: true,
REDACTED)
	require.Equal(t, []int64{1, 2, 3REDACTED, []int64{
		costAware[0].account.ID,
		costAware[1].account.ID,
		costAware[2].account.ID,
REDACTED)
REDACTED

func TestBuildOpenAIAccountLoadPlanUsesCostOnlyForTokenScope(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()
	defer resetOpenAIAdvancedSchedulerSettingCacheForTest()

	now := time.Now()
	accounts := []*Account{
		upstreamCostTestAccount(1, UpstreamBillingProbeStatusOK, 0.03, now.Add(-time.Minute), 30*time.Minute),
		upstreamCostTestOAuthAccount(2),
		upstreamCostTestAccount(3, UpstreamBillingProbeStatusOK, 0.8, now.Add(-time.Minute), 30*time.Minute),
REDACTED
	cfg := &config.Config{REDACTED
	cfg.Gateway.OpenAIWS.LBTopK = 1
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.UpstreamCost = 1.5
	settings := &openAIAdvancedSchedulerSettingRepoStub{values: map[string]string{
		SettingKeyOpenAIOAuthSchedulingRateMultiplier: "0.05",
REDACTEDREDACTED
	scheduler := &defaultOpenAIAccountScheduler{service: &OpenAIGatewayService{
		cfg:              cfg,
		rateLimitService: &RateLimitService{settingService: NewSettingService(settings, cfg)REDACTED,
REDACTEDREDACTED
	loadMap := map[int64]*AccountLoadInfo{
		1: {AccountID: 1REDACTED,
		2: {AccountID: 2REDACTED,
		3: {AccountID: 3REDACTED,
REDACTED

	tokenPlan := scheduler.buildOpenAIAccountLoadPlan(context.Background(), OpenAIAccountScheduleRequest{UseUpstreamTokenCost: trueREDACTED, accounts, loadMap)
	require.Greater(t, tokenPlan.candidates[0].score, tokenPlan.candidates[1].score)
	require.Greater(t, tokenPlan.candidates[1].score, tokenPlan.candidates[2].score)
	require.True(t, tokenPlan.includeOverflowFallback)

	otherPlan := scheduler.buildOpenAIAccountLoadPlan(context.Background(), OpenAIAccountScheduleRequest{REDACTED, accounts, loadMap)
	require.Equal(t, otherPlan.candidates[0].score, otherPlan.candidates[1].score)
	require.Equal(t, otherPlan.candidates[1].score, otherPlan.candidates[2].score)
	require.False(t, otherPlan.includeOverflowFallback)
REDACTED

func TestBuildOpenAIAccountSchedulerScoreSnapshotUpstreamCostIsExactNoOpWithoutSignal(t *testing.T) {
	accounts := []*Account{
		{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKeyREDACTED,
		{ID: 2, Platform: PlatformOpenAI, Type: AccountTypeAPIKeyREDACTED,
REDACTED
	loadMap := map[int64]*AccountLoadInfo{
		1: {AccountID: 1, LoadRate: 20REDACTED,
		2: {AccountID: 2, LoadRate: 80REDACTED,
REDACTED
	weights := GatewayOpenAIWSSchedulerScoreWeightsView{Priority: 1, Load: 1, Queue: 0.7, ErrorRate: 0.8, TTFT: 0.5REDACTED
	baseline := buildOpenAIAccountSchedulerScoreSnapshot(accounts, loadMap, weights, false, defaultOpenAIOAuthSchedulingRateMultiplier)
	weights.UpstreamCost = 1.5
	withCost := buildOpenAIAccountSchedulerScoreSnapshot(accounts, loadMap, weights, false, defaultOpenAIOAuthSchedulingRateMultiplier)

	require.Equal(t, baseline, withCost)
REDACTED

func TestBuildOpenAIAccountSchedulerScoreSnapshotUsesUpstreamCostSignal(t *testing.T) {
	now := time.Now()
	accounts := []*Account{
		upstreamCostTestAccount(1, UpstreamBillingProbeStatusOK, 0.03, now.Add(-time.Minute), 30*time.Minute),
		upstreamCostTestAccount(2, UpstreamBillingProbeStatusOK, 0.8, now.Add(-time.Minute), 30*time.Minute),
REDACTED
	weights := GatewayOpenAIWSSchedulerScoreWeightsView{UpstreamCost: 1.5REDACTED
	scores := buildOpenAIAccountSchedulerScoreSnapshot(accounts, nil, weights, false, defaultOpenAIOAuthSchedulingRateMultiplier)

	require.Greater(t, scores[1].BaseScore, scores[2].BaseScore)
REDACTED
