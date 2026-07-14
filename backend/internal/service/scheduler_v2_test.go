package service

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/stretchr/testify/require"
)

type schedulerV2CacheStub struct {
	SchedulerCache
	state              SchedulerEngineState
	legacySnapshot     []*Account
	accounts           map[int64]*Account
	candidates         map[string][]*Account
	ready              map[string]bool
	legacyReads        int
	candidateReads     int
	candidateRowsRead  int
	invalidateCalls    int
	operations         []string
	replacedAccountID  int64
	replacedAccountIDs []int64
	replacedBuckets    []SchedulerBucket
	deletedAccountIDs  []int64
	setIndexCalls      int
	onSetIndex         func(int)
}

func newSchedulerV2CacheStub() *schedulerV2CacheStub {
	return &schedulerV2CacheStub{
		state:      SchedulerEngineState{Engine: SchedulerEngineLegacy, Status: SchedulerEngineStatusDisabled},
		accounts:   map[int64]*Account{},
		candidates: map[string][]*Account{},
		ready:      map[string]bool{},
	}
}

func (c *schedulerV2CacheStub) GetSnapshot(context.Context, SchedulerBucket) ([]*Account, bool, error) {
	c.legacyReads++
	return c.legacySnapshot, true, nil
}

func (c *schedulerV2CacheStub) SetSnapshot(_ context.Context, bucket SchedulerBucket, accounts []Account) error {
	c.legacySnapshot = accountPointers(accounts)
	return nil
}

func (c *schedulerV2CacheStub) GetAccount(_ context.Context, accountID int64) (*Account, error) {
	return c.accounts[accountID], nil
}

func (c *schedulerV2CacheStub) SetAccount(_ context.Context, account *Account) error {
	if account != nil {
		copyAccount := *account
		c.accounts[account.ID] = &copyAccount
	}
	return nil
}

func (c *schedulerV2CacheStub) DeleteAccount(_ context.Context, accountID int64) error {
	delete(c.accounts, accountID)
	return nil
}

func (c *schedulerV2CacheStub) UpdateLastUsed(context.Context, map[int64]time.Time) error {
	return nil
}

func (c *schedulerV2CacheStub) TryLockBucket(context.Context, SchedulerBucket, time.Duration) (bool, error) {
	return true, nil
}

func (c *schedulerV2CacheStub) UnlockBucket(context.Context, SchedulerBucket) error {
	return nil
}

func (c *schedulerV2CacheStub) ListBuckets(context.Context) ([]SchedulerBucket, error) {
	buckets := make([]SchedulerBucket, 0, len(c.candidates))
	for raw := range c.candidates {
		if bucket, ok := ParseSchedulerBucket(raw); ok {
			buckets = append(buckets, bucket)
		}
	}
	return buckets, nil
}

func (c *schedulerV2CacheStub) GetOutboxWatermark(context.Context) (int64, error) {
	return 0, nil
}

func (c *schedulerV2CacheStub) SetOutboxWatermark(context.Context, int64) error {
	return nil
}

func (c *schedulerV2CacheStub) GetSchedulerEngineState(context.Context) (SchedulerEngineState, error) {
	state := c.state
	if ValidateSchedulerV2Limits(state.CandidateLimit, state.ScanLimit) != nil {
		state.CandidateLimit = DefaultSchedulerCandidateFetchLimit
		state.ScanLimit = DefaultSchedulerCandidateScanLimit
	}
	return state, nil
}

func (c *schedulerV2CacheStub) SetSchedulerEngineState(_ context.Context, state SchedulerEngineState) error {
	if ValidateSchedulerV2Limits(state.CandidateLimit, state.ScanLimit) != nil {
		state.CandidateLimit = c.state.CandidateLimit
		state.ScanLimit = c.state.ScanLimit
	}
	c.state = state
	c.operations = append(c.operations, "state:"+state.Engine)
	return nil
}

func (c *schedulerV2CacheStub) CompareAndSetSchedulerEngineState(_ context.Context, expectedEngine, expectedStatus string, state SchedulerEngineState) (bool, error) {
	if c.state.Engine != expectedEngine || c.state.Status != expectedStatus {
		return false, nil
	}
	if ValidateSchedulerV2Limits(state.CandidateLimit, state.ScanLimit) != nil {
		state.CandidateLimit = c.state.CandidateLimit
		state.ScanLimit = c.state.ScanLimit
	}
	c.state = state
	c.operations = append(c.operations, "state:"+state.Engine)
	return true, nil
}

func (c *schedulerV2CacheStub) SetSchedulerV2Limits(_ context.Context, candidateLimit, scanLimit int) error {
	c.state.CandidateLimit = candidateLimit
	c.state.ScanLimit = scanLimit
	return nil
}

func (c *schedulerV2CacheStub) GetCandidatePage(_ context.Context, bucket SchedulerBucket, offset int64, limit int) (SchedulerCandidatePage, bool, error) {
	c.candidateReads++
	raw := bucket.String()
	if !c.ready[raw] {
		return SchedulerCandidatePage{}, false, nil
	}
	accounts := c.candidates[raw]
	if offset >= int64(len(accounts)) {
		return SchedulerCandidatePage{Accounts: []*Account{}, NextOffset: offset, Done: true}, true, nil
	}
	end := int(offset) + limit
	if end > len(accounts) {
		end = len(accounts)
	}
	c.candidateRowsRead += end - int(offset)
	return SchedulerCandidatePage{
		Accounts:   append([]*Account(nil), accounts[offset:end]...),
		NextOffset: int64(end),
		Done:       end == len(accounts),
	}, true, nil
}

func (c *schedulerV2CacheStub) SetCandidateIndex(_ context.Context, bucket SchedulerBucket, accounts []Account) error {
	c.setIndexCalls++
	pointers := accountPointers(accounts)
	sort.SliceStable(pointers, func(i, j int) bool {
		left := SchedulerCandidateScore(*pointers[i], bucket)
		right := SchedulerCandidateScore(*pointers[j], bucket)
		if left == right {
			return pointers[i].ID < pointers[j].ID
		}
		return left < right
	})
	c.candidates[bucket.String()] = pointers
	c.ready[bucket.String()] = true
	for _, account := range pointers {
		copyAccount := *account
		c.accounts[account.ID] = &copyAccount
	}
	if c.onSetIndex != nil {
		c.onSetIndex(c.setIndexCalls)
	}
	return nil
}

func (c *schedulerV2CacheStub) ReplaceAccountCandidates(_ context.Context, account *Account, buckets []SchedulerBucket) error {
	c.replacedAccountID = account.ID
	c.replacedAccountIDs = append(c.replacedAccountIDs, account.ID)
	c.replacedBuckets = append([]SchedulerBucket(nil), buckets...)
	return c.SetAccount(context.Background(), account)
}

func (c *schedulerV2CacheStub) DeleteCandidateAccount(_ context.Context, accountID int64) error {
	c.deletedAccountIDs = append(c.deletedAccountIDs, accountID)
	delete(c.accounts, accountID)
	return nil
}

func (c *schedulerV2CacheStub) UpdateCandidateLastUsed(context.Context, map[int64]time.Time) error {
	return nil
}

func (c *schedulerV2CacheStub) InvalidateLegacySnapshots(context.Context) error {
	c.invalidateCalls++
	c.operations = append(c.operations, "invalidate")
	c.legacySnapshot = nil
	return nil
}

type schedulerV2AccountRepoStub struct {
	AccountRepository
	accounts []Account
	getCalls int
}

func (r *schedulerV2AccountRepoStub) GetByID(_ context.Context, id int64) (*Account, error) {
	r.getCalls++
	for i := range r.accounts {
		if r.accounts[i].ID == id {
			copyAccount := r.accounts[i]
			return &copyAccount, nil
		}
	}
	return nil, ErrAccountNotFound
}

func (r *schedulerV2AccountRepoStub) GetByIDs(_ context.Context, ids []int64) ([]*Account, error) {
	wanted := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		wanted[id] = struct{}{}
	}
	result := make([]*Account, 0, len(ids))
	for i := range r.accounts {
		if _, ok := wanted[r.accounts[i].ID]; ok {
			copyAccount := r.accounts[i]
			result = append(result, &copyAccount)
		}
	}
	return result, nil
}

func (r *schedulerV2AccountRepoStub) ListByGroup(_ context.Context, groupID int64) ([]Account, error) {
	result := make([]Account, 0)
	for _, account := range r.accounts {
		for _, id := range account.GroupIDs {
			if id == groupID {
				result = append(result, account)
				break
			}
		}
	}
	return result, nil
}

func (r *schedulerV2AccountRepoStub) ListActive(context.Context) ([]Account, error) {
	return append([]Account(nil), r.accounts...), nil
}

func (r *schedulerV2AccountRepoStub) ListAllWithFilters(context.Context, string, string, string, string, int64, string) ([]Account, error) {
	return append([]Account(nil), r.accounts...), nil
}

func TestSchedulerSnapshotV2_NeverReadsLegacySnapshotOrFallback(t *testing.T) {
	cache := newSchedulerV2CacheStub()
	cache.state = SchedulerEngineState{Engine: SchedulerEngineV2, Status: SchedulerEngineStatusActive}
	bucket := SchedulerBucket{Platform: PlatformOpenAI, Mode: SchedulerModeSingle}
	account := schedulerScenarioAccount(1, PlatformOpenAI, AccountTypeOAuth, 1, nil)
	require.NoError(t, cache.SetCandidateIndex(context.Background(), bucket, []Account{account}))
	repo := &schedulerV2AccountRepoStub{accounts: []Account{schedulerScenarioAccount(99, PlatformOpenAI, AccountTypeOAuth, 1, nil)}}
	svc := NewSchedulerSnapshotService(cache, nil, repo, nil, nil)

	accounts, _, err := svc.ListSchedulableAccounts(context.Background(), nil, PlatformOpenAI, false)
	require.NoError(t, err)
	require.Equal(t, []int64{1}, accountValueIDs(accounts))
	require.Zero(t, cache.legacyReads)
	require.Zero(t, repo.getCalls)
}

func TestSchedulerSnapshotV2_PaginatesPastBlockedAndExcludedCandidates(t *testing.T) {
	cache := newSchedulerV2CacheStub()
	cache.state = SchedulerEngineState{Engine: SchedulerEngineV2, Status: SchedulerEngineStatusActive}
	bucket := SchedulerBucket{Platform: PlatformAnthropic, Mode: SchedulerModeMixed}
	future := time.Now().Add(time.Hour)
	accounts := make([]Account, 0, DefaultSchedulerCandidateFetchLimit+1)
	excluded := make(map[int64]struct{}, DefaultSchedulerCandidateFetchLimit/2)
	for i := 1; i <= DefaultSchedulerCandidateFetchLimit; i++ {
		account := schedulerScenarioAccount(int64(i), PlatformAnthropic, AccountTypeOAuth, 1, nil)
		if i <= DefaultSchedulerCandidateFetchLimit/2 {
			account.TempUnschedulableUntil = &future
		} else {
			excluded[account.ID] = struct{}{}
		}
		accounts = append(accounts, account)
	}
	accounts = append(accounts, schedulerScenarioAccount(1000, PlatformAnthropic, AccountTypeSetupToken, 2, nil))
	require.NoError(t, cache.SetCandidateIndex(context.Background(), bucket, accounts))
	svc := NewSchedulerSnapshotService(cache, nil, nil, nil, nil)
	ctx := withSchedulerCandidateExclusions(context.Background(), excluded)

	selected, _, err := svc.ListSchedulableAccounts(ctx, nil, PlatformAnthropic, false)
	require.NoError(t, err)
	require.Equal(t, []int64{1000}, accountValueIDs(selected))
	require.GreaterOrEqual(t, cache.candidateReads, 2)
	require.Zero(t, cache.legacyReads)
}

func TestSchedulerSnapshotV2_ConfigurableCandidateAndScanLimits(t *testing.T) {
	cache := newSchedulerV2CacheStub()
	cache.state = SchedulerEngineState{Engine: SchedulerEngineV2, Status: SchedulerEngineStatusActive}
	bucket := SchedulerBucket{Platform: PlatformAnthropic, Mode: SchedulerModeMixed}
	future := time.Now().Add(time.Hour)
	indexed := make([]Account, 0, 8)
	for i := 1; i <= 4; i++ {
		account := schedulerScenarioAccount(int64(i), PlatformAnthropic, AccountTypeOAuth, 1, nil)
		account.TempUnschedulableUntil = &future
		indexed = append(indexed, account)
	}
	for i := 5; i <= 8; i++ {
		indexed = append(indexed, schedulerScenarioAccount(int64(i), PlatformAnthropic, AccountTypeOAuth, 1, nil))
	}
	require.NoError(t, cache.SetCandidateIndex(context.Background(), bucket, indexed))
	svc := NewSchedulerSnapshotService(cache, nil, nil, nil, nil)

	require.NoError(t, svc.ConfigureSchedulerV2Limits(context.Background(), 2, 4))
	accounts, _, err := svc.ListSchedulableAccounts(context.Background(), nil, PlatformAnthropic, false)
	require.NoError(t, err)
	require.Empty(t, accounts, "an eligible account beyond the raw scan budget must not be inspected")
	require.Equal(t, 4, cache.candidateRowsRead)

	cache.candidateRowsRead = 0
	require.NoError(t, svc.ConfigureSchedulerV2Limits(context.Background(), 2, 6))
	accounts, _, err = svc.ListSchedulableAccounts(context.Background(), nil, PlatformAnthropic, false)
	require.NoError(t, err)
	require.Equal(t, []int64{5, 6}, accountValueIDs(accounts))
	require.Equal(t, 6, cache.candidateRowsRead)
}

func TestSchedulerSnapshotV2_LimitsPropagateThroughGlobalState(t *testing.T) {
	cache := newSchedulerV2CacheStub()
	cache.state = SchedulerEngineState{Engine: SchedulerEngineV2, Status: SchedulerEngineStatusActive}
	first := NewSchedulerSnapshotService(cache, nil, nil, nil, nil)
	require.NoError(t, first.ConfigureSchedulerV2Limits(context.Background(), 24, 96))

	second := NewSchedulerSnapshotService(cache, nil, nil, nil, nil)
	state := second.SchedulerEngineState(context.Background())
	require.Equal(t, 24, state.CandidateLimit)
	require.Equal(t, 96, state.ScanLimit)
	candidateLimit, scanLimit := second.SchedulerV2Limits()
	require.Equal(t, 24, candidateLimit)
	require.Equal(t, 96, scanLimit)
}

func TestSchedulerSnapshotV2_BatchFilterPaginatesPastUnavailableWindow(t *testing.T) {
	cache := newSchedulerV2CacheStub()
	cache.state = SchedulerEngineState{Engine: SchedulerEngineV2, Status: SchedulerEngineStatusActive}
	bucket := SchedulerBucket{Platform: PlatformAnthropic, Mode: SchedulerModeMixed}
	accounts := make([]Account, 0, DefaultSchedulerCandidateFetchLimit+1)
	for i := 1; i <= DefaultSchedulerCandidateFetchLimit; i++ {
		accounts = append(accounts, schedulerScenarioAccount(int64(i), PlatformAnthropic, AccountTypeOAuth, 1, nil))
	}
	accounts = append(accounts, schedulerScenarioAccount(1000, PlatformAnthropic, AccountTypeSetupToken, 1, nil))
	require.NoError(t, cache.SetCandidateIndex(context.Background(), bucket, accounts))
	svc := NewSchedulerSnapshotService(cache, nil, nil, nil, nil)
	batchCalls := 0
	ctx := withSchedulerCandidateBatchPredicate(context.Background(), func(page []*Account) []bool {
		batchCalls++
		matches := make([]bool, len(page))
		for i, account := range page {
			matches[i] = account != nil && account.ID == 1000
		}
		return matches
	})

	selected, _, err := svc.ListSchedulableAccounts(ctx, nil, PlatformAnthropic, false)
	require.NoError(t, err)
	require.Equal(t, []int64{1000}, accountValueIDs(selected))
	require.GreaterOrEqual(t, batchCalls, 2)
	require.GreaterOrEqual(t, cache.candidateReads, 2)
}

func TestSchedulerV2_PaginatesPastModelIncompatibleWindow(t *testing.T) {
	cache := newSchedulerV2CacheStub()
	cache.state = SchedulerEngineState{Engine: SchedulerEngineV2, Status: SchedulerEngineStatusActive}
	bucket := SchedulerBucket{Platform: PlatformOpenAI, Mode: SchedulerModeSingle}
	accounts := make([]Account, 0, DefaultSchedulerCandidateFetchLimit+1)
	for i := 1; i <= DefaultSchedulerCandidateFetchLimit; i++ {
		account := schedulerScenarioAccount(int64(i), PlatformOpenAI, AccountTypeAPIKey, 1, nil)
		account.Credentials = map[string]any{"model_mapping": map[string]any{"other-model": "other-model"}}
		accounts = append(accounts, account)
	}
	compatible := schedulerScenarioAccount(1000, PlatformOpenAI, AccountTypeAPIKey, 2, nil)
	compatible.Credentials = map[string]any{"model_mapping": map[string]any{"gpt-target": "gpt-target"}}
	accounts = append(accounts, compatible)
	require.NoError(t, cache.SetCandidateIndex(context.Background(), bucket, accounts))
	snapshot := NewSchedulerSnapshotService(cache, nil, nil, nil, nil)
	gateway := &OpenAIGatewayService{schedulerSnapshot: snapshot}

	selected, err := gateway.selectAccountForModelWithExclusions(
		context.Background(), nil, PlatformOpenAI, "", "gpt-target", nil, false, 0, "",
	)
	require.NoError(t, err)
	require.NotNil(t, selected)
	require.Equal(t, int64(1000), selected.ID)
	require.GreaterOrEqual(t, cache.candidateReads, 2)
	require.Zero(t, cache.legacyReads)
}

func TestSchedulerSnapshotV2_AccountEventUsesIncrementalReplacement(t *testing.T) {
	cache := newSchedulerV2CacheStub()
	cache.state = SchedulerEngineState{Engine: SchedulerEngineV2, Status: SchedulerEngineStatusActive}
	groupID := int64(77)
	account := schedulerScenarioAccount(7, PlatformAntigravity, AccountTypeOAuth, 1, nil)
	account.Extra = map[string]any{"mixed_scheduling": true}
	account.GroupIDs = []int64{groupID}
	account.AccountGroups = []AccountGroup{{AccountID: account.ID, GroupID: groupID, Group: &Group{ID: groupID, Platform: PlatformGemini}}}
	repo := &schedulerV2AccountRepoStub{accounts: []Account{account}}
	svc := NewSchedulerSnapshotService(cache, nil, repo, nil, &config.Config{})

	require.NoError(t, svc.handleAccountEvent(context.Background(), &account.ID, nil, nil))
	require.Equal(t, account.ID, cache.replacedAccountID)
	require.Contains(t, cache.replacedBuckets, SchedulerBucket{GroupID: groupID, Platform: PlatformGemini, Mode: SchedulerModeMixed})
	require.Contains(t, cache.replacedBuckets, SchedulerBucket{GroupID: groupID, Platform: PlatformAntigravity, Mode: SchedulerModeForced})
	require.Zero(t, cache.legacyReads)
}

func TestSchedulerSnapshotV2_BulkEventIncrementallyUpdatesAndDeletes(t *testing.T) {
	cache := newSchedulerV2CacheStub()
	cache.state = SchedulerEngineState{Engine: SchedulerEngineV2, Status: SchedulerEngineStatusActive}
	account := schedulerScenarioAccount(21, PlatformGemini, AccountTypeServiceAccount, 1, nil)
	account.GroupIDs = []int64{9}
	account.AccountGroups = []AccountGroup{{AccountID: account.ID, GroupID: 9, Group: &Group{ID: 9, Platform: PlatformGemini}}}
	repo := &schedulerV2AccountRepoStub{accounts: []Account{account}}
	svc := NewSchedulerSnapshotService(cache, nil, repo, nil, &config.Config{})

	err := svc.handleBulkAccountEvent(context.Background(), map[string]any{
		"account_ids": []any{float64(account.ID), float64(22)},
		"group_ids":   []any{float64(9)},
	}, nil)
	require.NoError(t, err)
	require.Equal(t, []int64{account.ID}, cache.replacedAccountIDs)
	require.Equal(t, []int64{22}, cache.deletedAccountIDs)
	require.Zero(t, cache.legacyReads)
}

func TestSchedulerSnapshotV2_DisableInvalidatesLegacyBeforePublishingSwitch(t *testing.T) {
	cache := newSchedulerV2CacheStub()
	cache.state = SchedulerEngineState{Engine: SchedulerEngineV2, Status: SchedulerEngineStatusActive}
	svc := NewSchedulerSnapshotService(cache, nil, nil, nil, nil)

	require.NoError(t, svc.SetSchedulerV2Enabled(context.Background(), false, DefaultSchedulerCandidateFetchLimit, DefaultSchedulerCandidateScanLimit))
	require.Equal(t, []string{"invalidate", "state:legacy"}, cache.operations)
	require.Equal(t, SchedulerEngineLegacy, cache.state.Engine)
}

func TestSchedulerSnapshotV2_LateActivationCannotOverwriteDisable(t *testing.T) {
	cache := newSchedulerV2CacheStub()
	cache.state = SchedulerEngineState{Engine: SchedulerEngineV2, Status: SchedulerEngineStatusBuilding}
	cache.onSetIndex = func(call int) {
		if call == 12 {
			cache.state = SchedulerEngineState{Engine: SchedulerEngineLegacy, Status: SchedulerEngineStatusDisabled}
		}
	}
	repo := &schedulerV2AccountRepoStub{accounts: []Account{
		schedulerScenarioAccount(1, PlatformOpenAI, AccountTypeOAuth, 1, nil),
	}}
	svc := NewSchedulerSnapshotService(cache, nil, repo, nil, &config.Config{})

	svc.activateSchedulerV2("test_disable_race")

	require.Equal(t, 12, cache.setIndexCalls)
	require.Equal(t, SchedulerEngineLegacy, cache.state.Engine)
	require.Equal(t, SchedulerEngineStatusDisabled, cache.state.Status)
	require.Equal(t, SchedulerEngineLegacy, svc.SchedulerEngineState(context.Background()).Engine)
}

func TestSchedulerSnapshotV2_RecoversMissingRedisStateFromPersistedTarget(t *testing.T) {
	cache := newSchedulerV2CacheStub()
	cache.state = SchedulerEngineState{}
	accountRepo := &schedulerV2AccountRepoStub{accounts: []Account{
		schedulerScenarioAccount(1, PlatformOpenAI, AccountTypeOAuth, 1, nil),
	}}
	settingRepo := &schedulerV2SettingRepo{values: map[string]string{SettingKeySchedulerV2Enabled: "true"}}
	svc := NewSchedulerSnapshotService(cache, nil, accountRepo, nil, &config.Config{})
	svc.SetSchedulerEngineSettingRepository(settingRepo)

	state := svc.SchedulerEngineState(context.Background())
	require.True(t, state.V2Enabled(), "a missing Redis state must not send a persisted v2 deployment back to legacy")
	svc.Stop()

	state = svc.SchedulerEngineState(context.Background())
	require.Equal(t, SchedulerEngineV2, state.Engine)
	require.Equal(t, SchedulerEngineStatusActive, state.Status)
}

func TestSchedulerSnapshotV2_RejectsPartialEngineState(t *testing.T) {
	require.False(t, validSchedulerEngineState(SchedulerEngineState{Engine: SchedulerEngineV2}))
	require.False(t, validSchedulerEngineState(SchedulerEngineState{Engine: SchedulerEngineLegacy}))
	require.False(t, validSchedulerEngineState(SchedulerEngineState{Engine: SchedulerEngineV2, Status: SchedulerEngineStatusBuilding}))
	require.True(t, validSchedulerEngineState(SchedulerEngineState{
		Engine:         SchedulerEngineV2,
		Status:         SchedulerEngineStatusBuilding,
		CandidateLimit: DefaultSchedulerCandidateFetchLimit,
		ScanLimit:      DefaultSchedulerCandidateScanLimit,
	}))
	require.True(t, validSchedulerEngineState(SchedulerEngineState{
		Engine:         SchedulerEngineLegacy,
		Status:         SchedulerEngineStatusDisabled,
		CandidateLimit: DefaultSchedulerCandidateFetchLimit,
		ScanLimit:      DefaultSchedulerCandidateScanLimit,
	}))
}

func TestSchedulerV2_OldAndNewActuallySelectWeightCompliantAccounts(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	older := now.Add(-time.Hour)
	cases := []struct {
		platform    string
		accountType string
	}{
		{PlatformAnthropic, AccountTypeAPIKey},
		{PlatformAnthropic, AccountTypeOAuth},
		{PlatformAnthropic, AccountTypeSetupToken},
		{PlatformAnthropic, AccountTypeBedrock},
		{PlatformAnthropic, AccountTypeServiceAccount},
		{PlatformGemini, AccountTypeAPIKey},
		{PlatformGemini, AccountTypeOAuth},
		{PlatformGemini, AccountTypeServiceAccount},
		{PlatformAntigravity, AccountTypeAPIKey},
		{PlatformAntigravity, AccountTypeOAuth},
		{PlatformAntigravity, AccountTypeUpstream},
	}
	for _, tc := range cases {
		t.Run(tc.platform+"/"+tc.accountType, func(t *testing.T) {
			accounts := []Account{
				schedulerScenarioAccount(1, tc.platform, tc.accountType, 2, nil),
				schedulerScenarioAccount(2, tc.platform, tc.accountType, 1, &now),
				schedulerScenarioAccount(3, tc.platform, tc.accountType, 1, &older),
			}
			cache := newSchedulerV2CacheStub()
			cache.legacySnapshot = accountPointers(accounts)
			for i := range accounts {
				copyAccount := accounts[i]
				cache.accounts[copyAccount.ID] = &copyAccount
			}
			bucket := SchedulerBucket{Platform: tc.platform, Mode: SchedulerModeForced}
			require.NoError(t, cache.SetCandidateIndex(context.Background(), bucket, accounts))
			snapshot := NewSchedulerSnapshotService(cache, nil, nil, nil, nil)
			gateway := &GatewayService{schedulerSnapshot: snapshot, cfg: &config.Config{}}
			ctx := context.WithValue(context.Background(), ctxkey.ForcePlatform, tc.platform)

			cache.state = SchedulerEngineState{Engine: SchedulerEngineLegacy, Status: SchedulerEngineStatusDisabled}
			legacy, err := gateway.SelectAccountForModelWithExclusions(ctx, nil, "", "", nil)
			require.NoError(t, err)
			cache.state = SchedulerEngineState{Engine: SchedulerEngineV2, Status: SchedulerEngineStatusActive}
			v2, err := gateway.SelectAccountForModelWithExclusions(ctx, nil, "", "", nil)
			require.NoError(t, err)

			assertWeightCompliantSelection(t, legacy, 1, older)
			assertWeightCompliantSelection(t, v2, 1, older)
		})
	}
}

func TestSchedulerV2_OldAndNewOpenAICompatibleSelectionAcrossTypes(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	older := now.Add(-time.Hour)
	cases := []struct {
		platform    string
		accountType string
	}{
		{PlatformOpenAI, AccountTypeAPIKey},
		{PlatformOpenAI, AccountTypeOAuth},
		{PlatformGrok, AccountTypeAPIKey},
		{PlatformGrok, AccountTypeOAuth},
	}
	for _, tc := range cases {
		t.Run(tc.platform+"/"+tc.accountType, func(t *testing.T) {
			accounts := []Account{
				schedulerScenarioAccount(1, tc.platform, tc.accountType, 2, nil),
				schedulerScenarioAccount(2, tc.platform, tc.accountType, 1, &now),
				schedulerScenarioAccount(3, tc.platform, tc.accountType, 1, &older),
			}
			cache := newSchedulerV2CacheStub()
			cache.legacySnapshot = accountPointers(accounts)
			for i := range accounts {
				copyAccount := accounts[i]
				cache.accounts[copyAccount.ID] = &copyAccount
			}
			bucket := SchedulerBucket{Platform: tc.platform, Mode: SchedulerModeSingle}
			require.NoError(t, cache.SetCandidateIndex(context.Background(), bucket, accounts))
			snapshot := NewSchedulerSnapshotService(cache, nil, nil, nil, nil)
			gateway := &OpenAIGatewayService{schedulerSnapshot: snapshot}

			cache.state = SchedulerEngineState{Engine: SchedulerEngineLegacy, Status: SchedulerEngineStatusDisabled}
			legacy, err := gateway.selectAccountForModelWithExclusions(context.Background(), nil, tc.platform, "", "", nil, false, 0, "")
			require.NoError(t, err)
			cache.state = SchedulerEngineState{Engine: SchedulerEngineV2, Status: SchedulerEngineStatusActive}
			v2, err := gateway.selectAccountForModelWithExclusions(context.Background(), nil, tc.platform, "", "", nil, false, 0, "")
			require.NoError(t, err)

			assertWeightCompliantSelection(t, legacy, 1, older)
			assertWeightCompliantSelection(t, v2, 1, older)
		})
	}
}

func TestSchedulerV2_MixedSchedulingIncludesAntigravityAccounts(t *testing.T) {
	bucket := SchedulerBucket{GroupID: 8, Platform: PlatformGemini, Mode: SchedulerModeMixed}
	account := schedulerScenarioAccount(88, PlatformAntigravity, AccountTypeOAuth, 1, nil)
	account.Extra = map[string]any{"mixed_scheduling": true}
	account.GroupIDs = []int64{8}
	account.AccountGroups = []AccountGroup{{AccountID: 88, GroupID: 8, Group: &Group{ID: 8, Platform: PlatformGemini}}}

	require.True(t, schedulerV2AccountBelongsToBucket(&account, bucket))
	svc := NewSchedulerSnapshotService(nil, nil, nil, nil, &config.Config{})
	require.Contains(t, svc.schedulerV2BucketsForAccount(&account), bucket)
}

func schedulerScenarioAccount(id int64, platform, accountType string, priority int, lastUsedAt *time.Time) Account {
	return Account{
		ID:          id,
		Platform:    platform,
		Type:        accountType,
		Priority:    priority,
		LastUsedAt:  lastUsedAt,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
	}
}

func accountPointers(accounts []Account) []*Account {
	result := make([]*Account, 0, len(accounts))
	for i := range accounts {
		copyAccount := accounts[i]
		result = append(result, &copyAccount)
	}
	return result
}

func accountValueIDs(accounts []Account) []int64 {
	ids := make([]int64, 0, len(accounts))
	for _, account := range accounts {
		ids = append(ids, account.ID)
	}
	return ids
}

func assertWeightCompliantSelection(t *testing.T, account *Account, priority int, lastUsedAt time.Time) {
	t.Helper()
	require.NotNil(t, account)
	require.True(t, account.IsSchedulable())
	require.Equal(t, priority, account.Priority)
	require.NotNil(t, account.LastUsedAt)
	require.True(t, account.LastUsedAt.Equal(lastUsedAt))
}
