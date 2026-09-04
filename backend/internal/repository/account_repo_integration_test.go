//go:build integration

package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/accountgroup"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

func TestCodexDeviceBindingConcurrentResolveIsStable(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	repo := newAccountRepositoryWithSQL(client, integrationDB, nil)
	suffix := time.Now().UnixNano()
	user := mustCreateUser(t, client, &service.User{Email: fmt.Sprintf("codex-concurrent-%d@example.com", suffix)})
	apiKey := mustCreateApiKey(t, client, &service.APIKey{UserID: user.ID, Key: fmt.Sprintf("sk-codex-concurrent-%d", suffix)})
	account := &service.Account{
		Name: fmt.Sprintf("codex-concurrent-%d", suffix), Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth,
		Credentials: map[string]any{"access_token": "test-token"}, Extra: map[string]any{},
		Status: service.StatusActive, Schedulable: true, Concurrency: 3, Priority: 50,
	}
	policy := service.CodexIdentityPolicySpec{
		Mode: service.CodexIdentityPolicyOSProfileDevicePool,
		Profiles: []service.CodexOSProfilePolicy{{
			OSClass: service.CodexOSLinux, CanonicalSurface: service.CodexSurfaceCLI,
			Architecture: service.CodexArchX8664, SlotCount: 3,
		}},
	}
	require.NoError(t, repo.ProvisionAccount(ctx, &service.AccountProvisioningSpec{
		Account: account, Identity: &policy, FinalStatus: service.StatusActive,
		Schedulable: true, ProvisioningState: service.AccountProvisioningActive,
	}))
	t.Cleanup(func() {
		_ = client.Account.DeleteOneID(account.ID).Exec(context.Background())
		_ = client.APIKey.DeleteOneID(apiKey.ID).Exec(context.Background())
		_ = client.User.DeleteOneID(user.ID).Exec(context.Background())
	})

	const workers = 12
	results := make(chan *service.CodexResolvedDeviceSlot, workers)
	errorsCh := make(chan error, workers)
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resolved, err := repo.ResolveCodexDeviceBinding(ctx, account.ID, apiKey.ID, service.CodexOSLinux, service.CodexSurfaceCLI)
			if err != nil {
				errorsCh <- err
				return
			}
			results <- resolved
		}()
	}
	wg.Wait()
	close(results)
	close(errorsCh)
	for err := range errorsCh {
		require.NoError(t, err)
	}
	var bindingID, slotID int64
	for resolved := range results {
		if bindingID == 0 {
			bindingID, slotID = resolved.BindingID, resolved.SlotID
		}
		require.Equal(t, bindingID, resolved.BindingID)
		require.Equal(t, slotID, resolved.SlotID)
	}
}

func TestProvisionAccountInvalidGroupRollsBackIdentityGraphAndOutbox(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	repo := newAccountRepositoryWithSQL(client, integrationDB, nil)
	account := &service.Account{
		Name:     fmt.Sprintf("invalid-provision-%d", time.Now().UnixNano()),
		Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth,
		Credentials: map[string]any{"access_token": "test-token"}, Extra: map[string]any{},
		Status: service.StatusActive, Schedulable: true, Concurrency: 3, Priority: 50,
	}
	policy := service.CodexIdentityPolicySpec{
		Mode: service.CodexIdentityPolicyOSProfileDevicePool,
		Profiles: []service.CodexOSProfilePolicy{{
			OSClass: service.CodexOSLinux, CanonicalSurface: service.CodexSurfaceCLI,
			Architecture: service.CodexArchX8664, SlotCount: 1,
		}},
	}
	err := repo.ProvisionAccount(ctx, &service.AccountProvisioningSpec{
		Account: account, GroupIDs: []int64{int64(1 << 60)}, Identity: &policy,
		FinalStatus: service.StatusActive, Schedulable: true,
		ProvisioningState: service.AccountProvisioningActive,
	})
	require.Error(t, err)
	require.NotZero(t, account.ID, "the failure must occur after the account insert to prove rollback")

	for table, predicate := range map[string]string{
		"accounts":                        "id=$1",
		"account_groups":                  "account_id=$1",
		"account_codex_identity_policies": "account_id=$1",
		"account_codex_profiles":          "account_id=$1",
		"account_codex_device_slots":      "account_id=$1",
		"account_codex_device_bindings":   "account_id=$1",
		"scheduler_outbox":                "account_id=$1",
	} {
		var count int
		require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table+" WHERE "+predicate, account.ID).Scan(&count))
		require.Zerof(t, count, "%s must roll back", table)
	}
}

func TestProvisionAccountIsInvisibleToConcurrentSchedulerUntilCommit(t *testing.T) {
	ctx := context.Background()
	tx, err := integrationEntClient.Tx(ctx)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	txRepo := newAccountRepositoryWithSQL(tx.Client(), tx, nil)
	rootRepo := newAccountRepositoryWithSQL(integrationEntClient, integrationDB, nil)
	account := &service.Account{
		Name:     fmt.Sprintf("provision-visibility-%d", time.Now().UnixNano()),
		Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth,
		Credentials: map[string]any{"access_token": "test-token"}, Extra: map[string]any{},
		Status: service.StatusActive, Schedulable: true, Concurrency: 3, Priority: 50,
	}
	policy := service.CodexIdentityPolicySpec{
		Mode: service.CodexIdentityPolicyOSProfileDevicePool,
		Profiles: []service.CodexOSProfilePolicy{{
			OSClass: service.CodexOSLinux, CanonicalSurface: service.CodexSurfaceCLI,
			Architecture: service.CodexArchX8664, SlotCount: 1,
		}},
	}
	require.NoError(t, txRepo.ProvisionAccount(ctx, &service.AccountProvisioningSpec{
		Account: account, Identity: &policy, FinalStatus: service.StatusActive,
		Schedulable: true, ProvisioningState: service.AccountProvisioningActive,
	}))

	beforeCommit, err := rootRepo.ListSchedulableByPlatform(ctx, service.PlatformOpenAI)
	require.NoError(t, err)
	require.NotContains(t, idsOfAccounts(beforeCommit), account.ID)
	require.NoError(t, tx.Commit())
	t.Cleanup(func() { _ = integrationEntClient.Account.DeleteOneID(account.ID).Exec(context.Background()) })

	afterCommit, err := rootRepo.ListSchedulableByPlatform(ctx, service.PlatformOpenAI)
	require.NoError(t, err)
	require.Contains(t, idsOfAccounts(afterCommit), account.ID)
}

func TestConcurrentPolicyUpdatesAllocateDistinctVersionsAndEpochs(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	repo := newAccountRepositoryWithSQL(client, integrationDB, nil)
	account := &service.Account{
		Name:     fmt.Sprintf("policy-concurrency-%d", time.Now().UnixNano()),
		Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth,
		Credentials: map[string]any{"access_token": "test-token"}, Extra: map[string]any{},
		Status: service.StatusActive, Schedulable: true, Concurrency: 3, Priority: 50,
	}
	policy := service.CodexIdentityPolicySpec{
		Mode: service.CodexIdentityPolicyOSProfileDevicePool,
		Profiles: []service.CodexOSProfilePolicy{{
			OSClass: service.CodexOSLinux, CanonicalSurface: service.CodexSurfaceCLI,
			Architecture: service.CodexArchX8664, SlotCount: 1,
		}},
	}
	require.NoError(t, repo.ProvisionAccount(ctx, &service.AccountProvisioningSpec{
		Account: account, Identity: &policy, FinalStatus: service.StatusActive,
		Schedulable: true, ProvisioningState: service.AccountProvisioningActive,
	}))
	t.Cleanup(func() { _ = client.Account.DeleteOneID(account.ID).Exec(context.Background()) })

	first, err := repo.GetByID(ctx, account.ID)
	require.NoError(t, err)
	second, err := repo.GetByID(ctx, account.ID)
	require.NoError(t, err)
	firstPolicy := first.CodexIdentityPolicy
	firstPolicy.Profiles = append([]service.CodexOSProfilePolicy(nil), firstPolicy.Profiles...)
	firstPolicy.Profiles[0].CanonicalSurface = service.CodexSurfaceDesktop
	secondPolicy := second.CodexIdentityPolicy
	secondPolicy.Profiles = append([]service.CodexOSProfilePolicy(nil), secondPolicy.Profiles...)
	secondPolicy.Profiles[0].SlotCount = 2

	start := make(chan struct{})
	errorsCh := make(chan error, 2)
	var wg sync.WaitGroup
	for _, update := range []struct {
		account *service.Account
		policy  *service.CodexIdentityPolicySpec
	}{{first, &firstPolicy}, {second, &secondPolicy}} {
		update := update
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errorsCh <- repo.UpdateProvisionedAccount(ctx, &service.AccountProvisioningSpec{
				Account: update.account, Identity: update.policy, FinalStatus: service.StatusActive,
				Schedulable: true, ProvisioningState: service.AccountProvisioningActive,
			}, nil, nil, update.account.RateMultiplier)
		}()
	}
	close(start)
	wg.Wait()
	close(errorsCh)
	for err := range errorsCh {
		require.NoError(t, err)
	}

	final, err := repo.GetByID(ctx, account.ID)
	require.NoError(t, err)
	require.EqualValues(t, 3, final.CodexIdentityPolicy.Version)
	require.Len(t, final.CodexIdentityPolicy.Profiles, 1)
	require.EqualValues(t, 3, final.CodexIdentityPolicy.Profiles[0].Epoch)
}

type AccountRepoSuite struct {
	suite.Suite
	ctx    context.Context
	client *dbent.Client
	repo   *accountRepository
}

type schedulerCacheRecorder struct {
	setAccounts []*service.Account
	deleteIDs   []int64
	accounts    map[int64]*service.Account
	setCtxErr   error
}

func (s *schedulerCacheRecorder) GetSnapshot(ctx context.Context, bucket service.SchedulerBucket) ([]*service.Account, bool, error) {
	return nil, false, nil
}

func (s *schedulerCacheRecorder) CaptureBucketWriteToken(ctx context.Context, bucket service.SchedulerBucket) (service.SchedulerBucketWriteToken, error) {
	return service.SchedulerBucketWriteToken{Bucket: bucket, Epoch: 1}, nil
}

func (s *schedulerCacheRecorder) SetSnapshot(ctx context.Context, bucket service.SchedulerBucket, token service.SchedulerBucketWriteToken, accounts []service.Account) error {
	return nil
}

func (s *schedulerCacheRecorder) RetireBucket(ctx context.Context, bucket service.SchedulerBucket) error {
	return nil
}

func (s *schedulerCacheRecorder) ReopenBucket(ctx context.Context, bucket service.SchedulerBucket) (service.SchedulerBucketWriteToken, error) {
	return service.SchedulerBucketWriteToken{Bucket: bucket, Epoch: 1}, nil
}

func (s *schedulerCacheRecorder) TryAcquireGroupLifecycleLease(_ context.Context, groupID int64, _ time.Duration) (service.SchedulerGroupLifecycleLease, bool, error) {
	return service.SchedulerGroupLifecycleLease{GroupID: groupID, OwnerToken: "scheduler-cache-recorder"}, true, nil
}

func (s *schedulerCacheRecorder) ReleaseGroupLifecycleLease(context.Context, service.SchedulerGroupLifecycleLease) error {
	return nil
}

func (s *schedulerCacheRecorder) GetAccount(ctx context.Context, accountID int64) (*service.Account, error) {
	if s.accounts == nil {
		return nil, nil
	}
	return s.accounts[accountID], nil
}

func (s *schedulerCacheRecorder) SetAccount(ctx context.Context, account *service.Account) error {
	s.setCtxErr = ctx.Err()
	s.setAccounts = append(s.setAccounts, account)
	if s.accounts == nil {
		s.accounts = make(map[int64]*service.Account)
	}
	if account != nil {
		s.accounts[account.ID] = account
	}
	return nil
}

type failAtomicSchedulerOutboxSQLExecutor struct {
	sqlExecutor
}

func (e *failAtomicSchedulerOutboxSQLExecutor) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	if strings.Contains(query, "WITH updated AS") && strings.Contains(query, "INSERT INTO scheduler_outbox") && len(args) > 0 {
		args = append([]any(nil), args...)
		args[len(args)-1] = nil // event_type is NOT NULL; the whole statement must roll back.
	}
	return e.sqlExecutor.ExecContext(ctx, query, args...)
}

type cancelAfterAtomicMutationSQLExecutor struct {
	sqlExecutor
	cancel context.CancelFunc
}

func (e *cancelAfterAtomicMutationSQLExecutor) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	result, err := e.sqlExecutor.ExecContext(ctx, query, args...)
	if err == nil && strings.Contains(query, "WITH updated AS") && strings.Contains(query, "INSERT INTO scheduler_outbox") {
		e.cancel()
	}
	return result, err
}

func (s *schedulerCacheRecorder) DeleteAccount(ctx context.Context, accountID int64) error {
	s.deleteIDs = append(s.deleteIDs, accountID)
	if s.accounts != nil {
		delete(s.accounts, accountID)
	}
	return nil
}

func (s *schedulerCacheRecorder) UpdateLastUsed(ctx context.Context, updates map[int64]time.Time) error {
	return nil
}

func (s *schedulerCacheRecorder) TryLockBucket(ctx context.Context, bucket service.SchedulerBucket, ttl time.Duration) (bool, error) {
	return true, nil
}

func (s *schedulerCacheRecorder) UnlockBucket(ctx context.Context, bucket service.SchedulerBucket) error {
	return nil
}

func (s *schedulerCacheRecorder) ListBuckets(ctx context.Context) ([]service.SchedulerBucket, error) {
	return nil, nil
}

func (s *schedulerCacheRecorder) GetOutboxWatermark(ctx context.Context) (int64, error) {
	return 0, nil
}

func (s *schedulerCacheRecorder) SetOutboxWatermark(ctx context.Context, id int64) error {
	return nil
}

func (s *AccountRepoSuite) SetupTest() {
	s.ctx = context.Background()
	tx := testEntTx(s.T())
	s.client = tx.Client()
	s.repo = newAccountRepositoryWithSQL(s.client, tx, nil)
}

func TestAccountRepoSuite(t *testing.T) {
	suite.Run(t, new(AccountRepoSuite))
}

// --- Create / GetByID / Update / Delete ---

func (s *AccountRepoSuite) TestCreate() {
	account := &service.Account{
		Name:        "test-create",
		Platform:    service.PlatformAnthropic,
		Type:        service.AccountTypeOAuth,
		Status:      service.StatusActive,
		Credentials: map[string]any{},
		Extra:       map[string]any{},
		Concurrency: 3,
		Priority:    50,
		Schedulable: true,
	}

	err := s.repo.Create(s.ctx, account)
	s.Require().NoError(err, "Create")
	s.Require().NotZero(account.ID, "expected ID to be set")

	got, err := s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err, "GetByID")
	s.Require().Equal("test-create", got.Name)
}

func (s *AccountRepoSuite) TestGetByID_NotFound() {
	_, err := s.repo.GetByID(s.ctx, 999999)
	s.Require().Error(err, "expected error for non-existent ID")
}

func (s *AccountRepoSuite) TestUpdate() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "original"})

	account.Name = "updated"
	err := s.repo.Update(s.ctx, account)
	s.Require().NoError(err, "Update")

	got, err := s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err, "GetByID after update")
	s.Require().Equal("updated", got.Name)
}

func (s *AccountRepoSuite) TestUpdate_SyncSchedulerSnapshotOnDisabled() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "sync-update", Status: service.StatusActive, Schedulable: true})
	cacheRecorder := &schedulerCacheRecorder{}
	s.repo.schedulerCache = cacheRecorder

	account.Status = service.StatusDisabled
	err := s.repo.Update(s.ctx, account)
	s.Require().NoError(err, "Update")

	s.Require().Len(cacheRecorder.setAccounts, 1)
	s.Require().Equal(account.ID, cacheRecorder.setAccounts[0].ID)
	s.Require().Equal(service.StatusDisabled, cacheRecorder.setAccounts[0].Status)
}

func (s *AccountRepoSuite) TestUpdate_SyncSchedulerSnapshotOnCredentialsChange() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{
		Name:        "sync-credentials-update",
		Status:      service.StatusActive,
		Schedulable: true,
		Credentials: map[string]any{
			"model_mapping": map[string]any{
				"gpt-5": "gpt-5.1",
			},
		},
	})
	cacheRecorder := &schedulerCacheRecorder{}
	s.repo.schedulerCache = cacheRecorder

	account.Credentials = map[string]any{
		"model_mapping": map[string]any{
			"gpt-5": "gpt-5.2",
		},
	}
	err := s.repo.Update(s.ctx, account)
	s.Require().NoError(err, "Update")

	s.Require().Len(cacheRecorder.setAccounts, 1)
	s.Require().Equal(account.ID, cacheRecorder.setAccounts[0].ID)
	mapping, ok := cacheRecorder.setAccounts[0].Credentials["model_mapping"].(map[string]any)
	s.Require().True(ok)
	s.Require().Equal("gpt-5.2", mapping["gpt-5"])
}

func (s *AccountRepoSuite) TestUpdateCredentials_SyncsSnapshotAndDurableOutbox() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{
		Name:        "sync-refresh-credentials",
		Status:      service.StatusActive,
		Schedulable: true,
		Credentials: map[string]any{"access_token": "old-token"},
	})
	cacheRecorder := &schedulerCacheRecorder{}
	s.repo.schedulerCache = cacheRecorder
	_, err := s.repo.sql.ExecContext(s.ctx, "TRUNCATE scheduler_outbox")
	s.Require().NoError(err)

	s.Require().NoError(s.repo.UpdateCredentials(s.ctx, account.ID, map[string]any{"access_token": "new-token"}))

	s.Require().Len(cacheRecorder.setAccounts, 1)
	s.Require().Equal("new-token", cacheRecorder.setAccounts[0].GetCredential("access_token"))
	var outboxCount int
	err = scanSingleRow(
		s.ctx,
		s.repo.sql,
		"SELECT COUNT(*) FROM scheduler_outbox WHERE event_type = $1 AND account_id = $2",
		[]any{service.SchedulerOutboxEventAccountChanged, account.ID},
		&outboxCount,
	)
	s.Require().NoError(err)
	s.Require().Equal(1, outboxCount)
}

func (s *AccountRepoSuite) TestDelete() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "to-delete"})

	err := s.repo.Delete(s.ctx, account.ID)
	s.Require().NoError(err, "Delete")

	_, err = s.repo.GetByID(s.ctx, account.ID)
	s.Require().Error(err, "expected error after delete")
}

func (s *AccountRepoSuite) TestDelete_RemovesSchedulerAccountSnapshot() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "to-delete-cache"})
	cacheRecorder := &schedulerCacheRecorder{
		accounts: map[int64]*service.Account{
			account.ID: {
				ID:          account.ID,
				Name:        account.Name,
				Status:      service.StatusActive,
				Schedulable: true,
			},
		},
	}
	s.repo.schedulerCache = cacheRecorder

	err := s.repo.Delete(s.ctx, account.ID)
	s.Require().NoError(err, "Delete")

	s.Require().Equal([]int64{account.ID}, cacheRecorder.deleteIDs)
	s.Require().NotContains(cacheRecorder.accounts, account.ID)
}

func (s *AccountRepoSuite) TestDelete_WithGroupBindings() {
	group := mustCreateGroup(s.T(), s.client, &service.Group{Name: "g-del"})
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-del"})
	mustBindAccountToGroup(s.T(), s.client, account.ID, group.ID, 1)

	err := s.repo.Delete(s.ctx, account.ID)
	s.Require().NoError(err, "Delete should cascade remove bindings")

	count, err := s.client.AccountGroup.Query().Where(accountgroup.AccountIDEQ(account.ID)).Count(s.ctx)
	s.Require().NoError(err)
	s.Require().Zero(count, "expected bindings to be removed")
}

func (s *AccountRepoSuite) TestDelete_RemovesCodexIdentityGraphDespiteAccountSoftDelete() {
	user := mustCreateUser(s.T(), s.client, &service.User{Email: "delete-codex-graph@example.com"})
	apiKey := mustCreateApiKey(s.T(), s.client, &service.APIKey{UserID: user.ID, Key: "sk-delete-codex-graph"})
	policy := service.CodexIdentityPolicySpec{
		Mode: service.CodexIdentityPolicyOSProfileDevicePool,
		Profiles: []service.CodexOSProfilePolicy{{
			OSClass: service.CodexOSLinux, CanonicalSurface: service.CodexSurfaceCLI,
			Architecture: service.CodexArchX8664, SlotCount: 1,
		}},
	}
	account := &service.Account{
		Name: "delete-codex-graph", Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth,
		Credentials: map[string]any{"access_token": "test-token"}, Extra: map[string]any{},
		Status: service.StatusActive, Schedulable: true, Concurrency: 3, Priority: 50,
	}
	s.Require().NoError(s.repo.ProvisionAccount(s.ctx, &service.AccountProvisioningSpec{
		Account: account, Identity: &policy, FinalStatus: service.StatusActive,
		Schedulable: true, ProvisioningState: service.AccountProvisioningActive,
	}))
	_, err := s.repo.ResolveCodexDeviceBinding(s.ctx, account.ID, apiKey.ID, service.CodexOSLinux, service.CodexSurfaceCLI)
	s.Require().NoError(err)

	s.Require().NoError(s.repo.Delete(s.ctx, account.ID))
	for table := range map[string]struct{}{
		"account_codex_identity_policies": {},
		"account_codex_profiles":          {},
		"account_codex_device_slots":      {},
		"account_codex_device_bindings":   {},
	} {
		var count int
		s.Require().NoError(scanSingleRow(s.ctx, s.repo.sql, "SELECT COUNT(*) FROM "+table+" WHERE account_id=$1", []any{account.ID}, &count))
		s.Require().Zero(count, table)
	}
}

// --- List / ListWithFilters ---

func (s *AccountRepoSuite) TestList() {
	mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc1"})
	mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc2"})

	accounts, page, err := s.repo.List(s.ctx, pagination.PaginationParams{Page: 1, PageSize: 10})
	s.Require().NoError(err, "List")
	s.Require().Len(accounts, 2)
	s.Require().Equal(int64(2), page.Total)
}

func (s *AccountRepoSuite) TestListOAuthRefreshCandidatePage_GrokCursorAndExclusions() {
	now := time.Now().UTC()
	valid1 := mustCreateAccount(s.T(), s.client, &service.Account{
		Name:     "grok-oauth-page-1",
		Platform: service.PlatformGrok,
		Type:     service.AccountTypeOAuth,
		Status:   service.StatusActive,
		Credentials: map[string]any{
			"access_token":  "access-1",
			"refresh_token": "refresh-1",
			"expires_at":    now.Add(30 * time.Minute).Format(time.RFC3339),
		},
	})
	unschedulable := mustCreateAccount(s.T(), s.client, &service.Account{
		Name:        "grok-oauth-unschedulable-excluded",
		Platform:    service.PlatformGrok,
		Type:        service.AccountTypeOAuth,
		Status:      service.StatusActive,
		Credentials: map[string]any{"refresh_token": "refresh-unschedulable"},
	})
	s.Require().NoError(s.client.Account.UpdateOneID(unschedulable.ID).SetSchedulable(false).Exec(s.ctx))
	mustCreateAccount(s.T(), s.client, &service.Account{
		Name:     "grok-api-key-excluded",
		Platform: service.PlatformGrok,
		Type:     service.AccountTypeAPIKey,
		Status:   service.StatusActive,
		Credentials: map[string]any{
			"api_key":       "api-key",
			"refresh_token": "must-not-make-api-key-eligible",
		},
	})
	valid2 := mustCreateAccount(s.T(), s.client, &service.Account{
		Name:        "grok-oauth-page-2",
		Platform:    service.PlatformGrok,
		Type:        service.AccountTypeOAuth,
		Status:      service.StatusActive,
		Credentials: map[string]any{"refresh_token": "refresh-2"},
	})
	mustCreateAccount(s.T(), s.client, &service.Account{
		Name:        "grok-oauth-blank-refresh-excluded",
		Platform:    service.PlatformGrok,
		Type:        service.AccountTypeOAuth,
		Status:      service.StatusActive,
		Credentials: map[string]any{"refresh_token": "   "},
	})
	valid3 := mustCreateAccount(s.T(), s.client, &service.Account{
		Name:        "grok-oauth-page-3",
		Platform:    service.PlatformGrok,
		Type:        service.AccountTypeOAuth,
		Status:      service.StatusActive,
		Credentials: map[string]any{"refresh_token": "refresh-3"},
	})
	cooldown := mustCreateAccount(s.T(), s.client, &service.Account{
		Name:        "grok-oauth-retry-cooldown-excluded",
		Platform:    service.PlatformGrok,
		Type:        service.AccountTypeOAuth,
		Status:      service.StatusActive,
		Credentials: map[string]any{"refresh_token": "refresh-cooldown"},
	})
	s.Require().NoError(s.repo.SetTempUnschedulable(s.ctx, cooldown.ID, now.Add(10*time.Minute), "token refresh retry exhausted: timeout"))
	mustCreateAccount(s.T(), s.client, &service.Account{
		Name:        "openai-oauth-excluded",
		Platform:    service.PlatformOpenAI,
		Type:        service.AccountTypeOAuth,
		Status:      service.StatusActive,
		Credentials: map[string]any{"refresh_token": "refresh-openai"},
	})

	options := service.OAuthRefreshPageOptions{
		Platforms:            []string{service.PlatformGrok},
		Limit:                2,
		ActiveOnly:           true,
		RequireRefreshToken:  true,
		ExcludeRetryCooldown: true,
	}
	firstPage, err := s.repo.ListOAuthRefreshCandidatePage(s.ctx, options)
	s.Require().NoError(err)
	first := firstPage.Accounts
	s.Require().Len(first, 2)
	s.Require().Equal([]int64{valid1.ID, valid2.ID}, []int64{first[0].ID, first[1].ID})
	s.Require().NotContains([]int64{first[0].ID, first[1].ID}, unschedulable.ID)

	options.AfterID = first[len(first)-1].ID
	secondPage, err := s.repo.ListOAuthRefreshCandidatePage(s.ctx, options)
	s.Require().NoError(err)
	second := secondPage.Accounts
	s.Require().Len(second, 1)
	s.Require().Equal(valid3.ID, second[0].ID)
	s.Require().NotContains([]int64{first[0].ID, first[1].ID}, second[0].ID)
}

func (s *AccountRepoSuite) TestListWithFilters() {
	tests := []struct {
		name        string
		setup       func(client *dbent.Client)
		platform    string
		accType     string
		status      string
		search      string
		groupID     int64
		privacyMode string
		wantCount   int
		validate    func(accounts []service.Account)
	}{
		{
			name: "filter_by_platform",
			setup: func(client *dbent.Client) {
				mustCreateAccount(s.T(), client, &service.Account{Name: "a1", Platform: service.PlatformAnthropic})
				mustCreateAccount(s.T(), client, &service.Account{Name: "a2", Platform: service.PlatformOpenAI})
			},
			platform:  service.PlatformOpenAI,
			wantCount: 1,
			validate: func(accounts []service.Account) {
				s.Require().Equal(service.PlatformOpenAI, accounts[0].Platform)
			},
		},
		{
			name: "filter_by_type",
			setup: func(client *dbent.Client) {
				mustCreateAccount(s.T(), client, &service.Account{Name: "t1", Type: service.AccountTypeOAuth})
				mustCreateAccount(s.T(), client, &service.Account{Name: "t2", Type: service.AccountTypeAPIKey})
			},
			accType:   service.AccountTypeAPIKey,
			wantCount: 1,
			validate: func(accounts []service.Account) {
				s.Require().Equal(service.AccountTypeAPIKey, accounts[0].Type)
			},
		},
		{
			name: "filter_by_status",
			setup: func(client *dbent.Client) {
				mustCreateAccount(s.T(), client, &service.Account{Name: "s1", Status: service.StatusActive})
				mustCreateAccount(s.T(), client, &service.Account{Name: "s2", Status: service.StatusDisabled})
			},
			status:    service.StatusDisabled,
			wantCount: 1,
			validate: func(accounts []service.Account) {
				s.Require().Equal(service.StatusDisabled, accounts[0].Status)
			},
		},
		{
			name: "filter_by_status_active_excludes_runtime_blocked_accounts",
			setup: func(client *dbent.Client) {
				mustCreateAccount(s.T(), client, &service.Account{Name: "active-normal", Status: service.StatusActive})
				rateLimited := mustCreateAccount(s.T(), client, &service.Account{Name: "active-rate-limited", Status: service.StatusActive})
				err := client.Account.UpdateOneID(rateLimited.ID).
					SetRateLimitResetAt(time.Now().Add(10 * time.Minute)).
					Exec(context.Background())
				s.Require().NoError(err)
				tempUnsched := mustCreateAccount(s.T(), client, &service.Account{Name: "active-temp-unsched", Status: service.StatusActive})
				err = client.Account.UpdateOneID(tempUnsched.ID).
					SetTempUnschedulableUntil(time.Now().Add(15 * time.Minute)).
					Exec(context.Background())
				s.Require().NoError(err)
				unsched := mustCreateAccount(s.T(), client, &service.Account{Name: "active-unsched", Status: service.StatusActive})
				err = client.Account.UpdateOneID(unsched.ID).
					SetSchedulable(false).
					Exec(context.Background())
				s.Require().NoError(err)
			},
			status:    service.StatusActive,
			wantCount: 1,
			validate: func(accounts []service.Account) {
				s.Require().Equal("active-normal", accounts[0].Name)
			},
		},
		{
			name: "filter_by_status_unschedulable_excludes_rate_limited_and_temp_unschedulable",
			setup: func(client *dbent.Client) {
				mustCreateAccount(s.T(), client, &service.Account{Name: "active-normal", Status: service.StatusActive, Schedulable: true})
				unsched := mustCreateAccount(s.T(), client, &service.Account{Name: "active-unsched", Status: service.StatusActive})
				err := client.Account.UpdateOneID(unsched.ID).
					SetSchedulable(false).
					Exec(context.Background())
				s.Require().NoError(err)
				rateLimited := mustCreateAccount(s.T(), client, &service.Account{Name: "active-rate-limited", Status: service.StatusActive})
				err = client.Account.UpdateOneID(rateLimited.ID).
					SetSchedulable(false).
					SetRateLimitResetAt(time.Now().Add(10 * time.Minute)).
					Exec(context.Background())
				s.Require().NoError(err)
				tempUnsched := mustCreateAccount(s.T(), client, &service.Account{Name: "active-temp-unsched", Status: service.StatusActive})
				err = client.Account.UpdateOneID(tempUnsched.ID).
					SetSchedulable(false).
					SetTempUnschedulableUntil(time.Now().Add(15 * time.Minute)).
					Exec(context.Background())
				s.Require().NoError(err)
			},
			status:    "unschedulable",
			wantCount: 1,
			validate: func(accounts []service.Account) {
				s.Require().Equal("active-unsched", accounts[0].Name)
			},
		},
		{
			name: "filter_by_status_rate_limited_excludes_temp_unschedulable",
			setup: func(client *dbent.Client) {
				rateLimited := mustCreateAccount(s.T(), client, &service.Account{Name: "active-rate-limited", Status: service.StatusActive})
				err := client.Account.UpdateOneID(rateLimited.ID).
					SetRateLimitResetAt(time.Now().Add(10 * time.Minute)).
					Exec(context.Background())
				s.Require().NoError(err)
				tempUnsched := mustCreateAccount(s.T(), client, &service.Account{Name: "active-temp-unsched", Status: service.StatusActive})
				err = client.Account.UpdateOneID(tempUnsched.ID).
					SetRateLimitResetAt(time.Now().Add(20 * time.Minute)).
					SetTempUnschedulableUntil(time.Now().Add(15 * time.Minute)).
					Exec(context.Background())
				s.Require().NoError(err)
			},
			status:    "rate_limited",
			wantCount: 1,
			validate: func(accounts []service.Account) {
				s.Require().Equal("active-rate-limited", accounts[0].Name)
			},
		},
		{
			name: "filter_by_status_temp_unschedulable_excludes_manually_unschedulable",
			setup: func(client *dbent.Client) {
				tempUnsched := mustCreateAccount(s.T(), client, &service.Account{Name: "active-temp-unsched", Status: service.StatusActive, Schedulable: true})
				err := client.Account.UpdateOneID(tempUnsched.ID).
					SetTempUnschedulableUntil(time.Now().Add(15 * time.Minute)).
					Exec(context.Background())
				s.Require().NoError(err)
				unsched := mustCreateAccount(s.T(), client, &service.Account{Name: "active-unsched", Status: service.StatusActive})
				err = client.Account.UpdateOneID(unsched.ID).
					SetSchedulable(false).
					Exec(context.Background())
				s.Require().NoError(err)
			},
			status:    "temp_unschedulable",
			wantCount: 1,
			validate: func(accounts []service.Account) {
				s.Require().Equal("active-temp-unsched", accounts[0].Name)
			},
		},
		{
			name: "filter_by_search",
			setup: func(client *dbent.Client) {
				mustCreateAccount(s.T(), client, &service.Account{Name: "alpha-account"})
				mustCreateAccount(s.T(), client, &service.Account{Name: "beta-account"})
			},
			search:    "alpha",
			wantCount: 1,
			validate: func(accounts []service.Account) {
				s.Require().Contains(accounts[0].Name, "alpha")
			},
		},
		{
			name: "filter_by_ungrouped",
			setup: func(client *dbent.Client) {
				group := mustCreateGroup(s.T(), client, &service.Group{Name: "g-ungrouped"})
				grouped := mustCreateAccount(s.T(), client, &service.Account{Name: "grouped-account"})
				mustCreateAccount(s.T(), client, &service.Account{Name: "ungrouped-account"})
				mustBindAccountToGroup(s.T(), client, grouped.ID, group.ID, 1)
			},
			groupID:   service.AccountListGroupUngrouped,
			wantCount: 1,
			validate: func(accounts []service.Account) {
				s.Require().Equal("ungrouped-account", accounts[0].Name)
				s.Require().Empty(accounts[0].GroupIDs)
			},
		},
		{
			name: "filter_by_privacy_mode",
			setup: func(client *dbent.Client) {
				mustCreateAccount(s.T(), client, &service.Account{Name: "privacy-ok", Extra: map[string]any{"privacy_mode": service.PrivacyModeTrainingOff}})
				mustCreateAccount(s.T(), client, &service.Account{Name: "privacy-fail", Extra: map[string]any{"privacy_mode": service.PrivacyModeFailed}})
			},
			privacyMode: service.PrivacyModeTrainingOff,
			wantCount:   1,
			validate: func(accounts []service.Account) {
				s.Require().Equal("privacy-ok", accounts[0].Name)
			},
		},
		{
			name: "filter_by_privacy_mode_unset",
			setup: func(client *dbent.Client) {
				mustCreateAccount(s.T(), client, &service.Account{Name: "privacy-unset", Extra: nil})
				mustCreateAccount(s.T(), client, &service.Account{Name: "privacy-empty", Extra: map[string]any{"privacy_mode": ""}})
				mustCreateAccount(s.T(), client, &service.Account{Name: "privacy-set", Extra: map[string]any{"privacy_mode": service.PrivacyModeTrainingOff}})
			},
			privacyMode: service.AccountPrivacyModeUnsetFilter,
			wantCount:   2,
			validate: func(accounts []service.Account) {
				names := []string{accounts[0].Name, accounts[1].Name}
				s.ElementsMatch([]string{"privacy-unset", "privacy-empty"}, names)
			},
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			// 每个 case 重新获取隔离资源
			tx := testEntTx(s.T())
			client := tx.Client()
			repo := newAccountRepositoryWithSQL(client, tx, nil)
			ctx := context.Background()

			tt.setup(client)

			accounts, page, err := repo.ListWithFilters(ctx, pagination.PaginationParams{Page: 1, PageSize: 10}, tt.platform, tt.accType, tt.status, tt.search, tt.groupID, tt.privacyMode)
			s.Require().NoError(err)
			s.Require().Len(accounts, tt.wantCount)
			// Regression guard for issue #3601: when the whole result set fits on a single page,
			// pagination.Total must match len(items). A mismatch means the Count query was applied
			// against different predicates than the list query — the exact symptom reported.
			s.Require().NotNil(page)
			s.Require().Equal(int64(tt.wantCount), page.Total, "total must match items on single page")
			if tt.validate != nil {
				tt.validate(accounts)
			}
		})
	}
}

// --- ListByGroup / ListActive / ListByPlatform ---

func (s *AccountRepoSuite) TestListByGroup() {
	group := mustCreateGroup(s.T(), s.client, &service.Group{Name: "g-list"})
	acc1 := mustCreateAccount(s.T(), s.client, &service.Account{Name: "a1", Status: service.StatusActive})
	acc2 := mustCreateAccount(s.T(), s.client, &service.Account{Name: "a2", Status: service.StatusActive})
	mustBindAccountToGroup(s.T(), s.client, acc1.ID, group.ID, 2)
	mustBindAccountToGroup(s.T(), s.client, acc2.ID, group.ID, 1)

	accounts, err := s.repo.ListByGroup(s.ctx, group.ID)
	s.Require().NoError(err, "ListByGroup")
	s.Require().Len(accounts, 2)
	// Should be ordered by priority
	s.Require().Equal(acc2.ID, accounts[0].ID, "expected acc2 first (priority=1)")
}

func (s *AccountRepoSuite) TestListActive() {
	mustCreateAccount(s.T(), s.client, &service.Account{Name: "active1", Status: service.StatusActive})
	mustCreateAccount(s.T(), s.client, &service.Account{Name: "inactive1", Status: service.StatusDisabled})

	accounts, err := s.repo.ListActive(s.ctx)
	s.Require().NoError(err, "ListActive")
	s.Require().Len(accounts, 1)
	s.Require().Equal("active1", accounts[0].Name)
}

func (s *AccountRepoSuite) TestListByPlatform() {
	mustCreateAccount(s.T(), s.client, &service.Account{Name: "p1", Platform: service.PlatformAnthropic, Status: service.StatusActive})
	mustCreateAccount(s.T(), s.client, &service.Account{Name: "p2", Platform: service.PlatformOpenAI, Status: service.StatusActive})

	accounts, err := s.repo.ListByPlatform(s.ctx, service.PlatformAnthropic)
	s.Require().NoError(err, "ListByPlatform")
	s.Require().Len(accounts, 1)
	s.Require().Equal(service.PlatformAnthropic, accounts[0].Platform)
}

// --- Preload and VirtualFields ---

func (s *AccountRepoSuite) TestPreload_And_VirtualFields() {
	proxy := mustCreateProxy(s.T(), s.client, &service.Proxy{Name: "p1"})
	group := mustCreateGroup(s.T(), s.client, &service.Group{Name: "g1"})

	account := mustCreateAccount(s.T(), s.client, &service.Account{
		Name:    "acc1",
		ProxyID: &proxy.ID,
	})
	mustBindAccountToGroup(s.T(), s.client, account.ID, group.ID, 1)

	got, err := s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err, "GetByID")
	s.Require().NotNil(got.Proxy, "expected Proxy preload")
	s.Require().Equal(proxy.ID, got.Proxy.ID)
	s.Require().Len(got.GroupIDs, 1, "expected GroupIDs to be populated")
	s.Require().Equal(group.ID, got.GroupIDs[0])
	s.Require().Len(got.Groups, 1, "expected Groups to be populated")
	s.Require().Equal(group.ID, got.Groups[0].ID)

	accounts, page, err := s.repo.ListWithFilters(s.ctx, pagination.PaginationParams{Page: 1, PageSize: 10}, "", "", "", "acc", 0, "")
	s.Require().NoError(err, "ListWithFilters")
	s.Require().Equal(int64(1), page.Total)
	s.Require().Len(accounts, 1)
	s.Require().NotNil(accounts[0].Proxy, "expected Proxy preload in list")
	s.Require().Equal(proxy.ID, accounts[0].Proxy.ID)
	s.Require().Len(accounts[0].GroupIDs, 1, "expected GroupIDs in list")
	s.Require().Equal(group.ID, accounts[0].GroupIDs[0])
}

// --- GroupBinding / AddToGroup / RemoveFromGroup / BindGroups / GetGroups ---

func (s *AccountRepoSuite) TestGroupBinding_And_BindGroups() {
	g1 := mustCreateGroup(s.T(), s.client, &service.Group{Name: "g1"})
	g2 := mustCreateGroup(s.T(), s.client, &service.Group{Name: "g2"})
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc"})

	s.Require().NoError(s.repo.AddToGroup(s.ctx, account.ID, g1.ID, 10), "AddToGroup")
	groups, err := s.repo.GetGroups(s.ctx, account.ID)
	s.Require().NoError(err, "GetGroups")
	s.Require().Len(groups, 1, "expected 1 group")
	s.Require().Equal(g1.ID, groups[0].ID)

	s.Require().NoError(s.repo.RemoveFromGroup(s.ctx, account.ID, g1.ID), "RemoveFromGroup")
	groups, err = s.repo.GetGroups(s.ctx, account.ID)
	s.Require().NoError(err, "GetGroups after remove")
	s.Require().Empty(groups, "expected 0 groups after remove")

	s.Require().NoError(s.repo.BindGroups(s.ctx, account.ID, []int64{g1.ID, g2.ID}), "BindGroups")
	groups, err = s.repo.GetGroups(s.ctx, account.ID)
	s.Require().NoError(err, "GetGroups after bind")
	s.Require().Len(groups, 2, "expected 2 groups after bind")
}

func (s *AccountRepoSuite) TestBindGroups_EmptyList() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-empty"})
	group := mustCreateGroup(s.T(), s.client, &service.Group{Name: "g-empty"})
	mustBindAccountToGroup(s.T(), s.client, account.ID, group.ID, 1)

	s.Require().NoError(s.repo.BindGroups(s.ctx, account.ID, []int64{}), "BindGroups empty")

	groups, err := s.repo.GetGroups(s.ctx, account.ID)
	s.Require().NoError(err)
	s.Require().Empty(groups, "expected 0 groups after binding empty list")
}

// --- Schedulable ---

func (s *AccountRepoSuite) TestProvisionAccount_CommitsIdentityGraphAndPublishesOnlyActiveRow() {
	group := mustCreateGroup(s.T(), s.client, &service.Group{Name: "provision-group", Platform: service.PlatformOpenAI})
	proxy := mustCreateProxy(s.T(), s.client, &service.Proxy{Name: "provision-proxy"})
	policy := service.CodexIdentityPolicySpec{
		Mode: service.CodexIdentityPolicyOSProfileDevicePool,
		Profiles: []service.CodexOSProfilePolicy{{
			OSClass:          service.CodexOSLinux,
			CanonicalSurface: service.CodexSurfaceDesktop,
			Architecture:     service.CodexArchARM64,
			SlotCount:        2,
			ProxyID:          &proxy.ID,
			Slots: []service.CodexDeviceSlotPolicy{{
				Index: 1, ClientVersionMode: service.CodexClientVersionPinned, ClientVersion: "0.200.1",
			}},
		}},
	}
	account := &service.Account{
		Name:        "provisioned-openai",
		Platform:    service.PlatformOpenAI,
		Type:        service.AccountTypeOAuth,
		Credentials: map[string]any{"access_token": "test-token"},
		Extra:       map[string]any{},
		Concurrency: 3,
		Priority:    50,
	}

	err := s.repo.ProvisionAccount(s.ctx, &service.AccountProvisioningSpec{
		Account:           account,
		GroupIDs:          []int64{group.ID},
		Identity:          &policy,
		FinalStatus:       service.StatusActive,
		Schedulable:       true,
		ProvisioningState: service.AccountProvisioningActive,
	})
	s.Require().NoError(err)
	s.Require().NotZero(account.ID)
	s.Require().Equal(service.AccountProvisioningActive, account.ProvisioningState)
	s.Require().True(account.Schedulable)
	s.Require().NotEmpty(account.Extra[service.CodexFingerprintSeedExtraKey])

	var policyCount, profileCount, slotCount, bindingCount, outboxCount int
	s.Require().NoError(scanSingleRow(s.ctx, s.repo.sql, "SELECT COUNT(*) FROM account_codex_identity_policies WHERE account_id=$1", []any{account.ID}, &policyCount))
	s.Require().NoError(scanSingleRow(s.ctx, s.repo.sql, "SELECT COUNT(*) FROM account_codex_profiles WHERE account_id=$1", []any{account.ID}, &profileCount))
	s.Require().NoError(scanSingleRow(s.ctx, s.repo.sql, "SELECT COUNT(*) FROM account_codex_device_slots WHERE account_id=$1", []any{account.ID}, &slotCount))
	s.Require().NoError(scanSingleRow(s.ctx, s.repo.sql, "SELECT COUNT(*) FROM account_codex_device_bindings WHERE account_id=$1", []any{account.ID}, &bindingCount))
	s.Require().NoError(scanSingleRow(s.ctx, s.repo.sql, "SELECT COUNT(*) FROM scheduler_outbox WHERE account_id=$1 AND event_type=$2", []any{account.ID, service.SchedulerOutboxEventAccountChanged}, &outboxCount))
	s.Require().Equal(1, policyCount)
	s.Require().Equal(1, profileCount)
	s.Require().Equal(2, slotCount)
	s.Require().Zero(bindingCount)
	s.Require().Equal(1, outboxCount)
	var slotVersionMode, slotVersion string
	s.Require().NoError(scanSingleRow(s.ctx, s.repo.sql, `
		SELECT client_version_mode, client_version
		FROM account_codex_device_slots
		WHERE account_id=$1 AND slot_index=1
	`, []any{account.ID}, &slotVersionMode, &slotVersion))
	s.Require().Equal(string(service.CodexClientVersionPinned), slotVersionMode)
	s.Require().Equal("0.200.1", slotVersion)
	resolvedSlots, err := s.repo.ListCodexDeviceSlots(s.ctx, account.ID, service.CodexOSLinux, service.CodexSurfaceDesktop, false)
	s.Require().NoError(err)
	s.Require().Len(resolvedSlots, 2)
	var pinnedSlot *service.CodexResolvedDeviceSlot
	for index := range resolvedSlots {
		if resolvedSlots[index].SlotIndex == 1 {
			pinnedSlot = &resolvedSlots[index]
			break
		}
	}
	s.Require().NotNil(pinnedSlot)
	s.Require().Equal(service.CodexClientVersionPinned, pinnedSlot.ClientVersionMode)
	s.Require().Equal("0.200.1", pinnedSlot.ClientVersion)

	schedulable, err := s.repo.ListSchedulableByGroupIDAndPlatform(s.ctx, group.ID, service.PlatformOpenAI)
	s.Require().NoError(err)
	s.Require().Len(schedulable, 1)
	s.Require().Equal(account.ID, schedulable[0].ID)
}

func (s *AccountRepoSuite) TestListSchedulableRejectsPendingProvisioningState() {
	pending := mustCreateAccount(s.T(), s.client, &service.Account{
		Name:        "pending-provisioning",
		Platform:    service.PlatformOpenAI,
		Schedulable: true,
	})
	_, err := s.client.Account.UpdateOneID(pending.ID).
		SetProvisioningState(string(service.AccountProvisioningPending)).
		Save(s.ctx)
	s.Require().NoError(err)

	accounts, err := s.repo.ListSchedulableByPlatform(s.ctx, service.PlatformOpenAI)
	s.Require().NoError(err)
	s.Require().NotContains(idsOfAccounts(accounts), pending.ID)
}

func (s *AccountRepoSuite) TestPendingProvisioningTriggerForcesUnschedulableForLegacyWriter() {
	created, err := s.client.Account.Create().
		SetName("legacy-writer-pending").
		SetPlatform(service.PlatformOpenAI).
		SetType(service.AccountTypeOAuth).
		SetCredentials(map[string]any{}).
		SetExtra(map[string]any{}).
		SetStatus(service.StatusActive).
		SetSchedulable(true).
		Save(s.ctx)
	s.Require().NoError(err)
	loaded, err := s.client.Account.Get(s.ctx, created.ID)
	s.Require().NoError(err)
	s.Require().Equal(string(service.AccountProvisioningPending), loaded.ProvisioningState)
	s.Require().False(loaded.Schedulable)
}

func (s *AccountRepoSuite) TestDatabaseRejectsLegacyIdentityModeConflict() {
	policy := service.CodexIdentityPolicySpec{
		Mode: service.CodexIdentityPolicyOSProfileDevicePool,
		Profiles: []service.CodexOSProfilePolicy{{
			OSClass: service.CodexOSLinux, CanonicalSurface: service.CodexSurfaceCLI,
			Architecture: service.CodexArchX8664, SlotCount: 1,
		}},
	}
	account := &service.Account{
		Name: "constraint-account", Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth,
		Credentials: map[string]any{"access_token": "test-token"}, Extra: map[string]any{},
		Status: service.StatusActive, Schedulable: true, Concurrency: 3, Priority: 50,
	}
	s.Require().NoError(s.repo.ProvisionAccount(s.ctx, &service.AccountProvisioningSpec{
		Account: account, Identity: &policy, FinalStatus: service.StatusActive,
		Schedulable: true, ProvisioningState: service.AccountProvisioningActive,
	}))

	conflictingExtra := copyJSONMap(account.Extra)
	conflictingExtra["codex_fingerprint_mode"] = "session"
	_, err := s.client.Account.UpdateOneID(account.ID).SetExtra(conflictingExtra).Save(s.ctx)
	s.Require().Error(err)
}

func (s *AccountRepoSuite) TestDatabaseRejectsActiveDevicePoolWithoutCredential() {
	policy := service.CodexIdentityPolicySpec{
		Mode: service.CodexIdentityPolicyOSProfileDevicePool,
		Profiles: []service.CodexOSProfilePolicy{{
			OSClass: service.CodexOSLinux, CanonicalSurface: service.CodexSurfaceCLI,
			Architecture: service.CodexArchX8664, SlotCount: 1,
		}},
	}
	account := &service.Account{
		Name: "credential-constraint-account", Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth,
		Credentials: map[string]any{"refresh_token": "test-refresh-token"}, Extra: map[string]any{},
		Status: service.StatusActive, Schedulable: true, Concurrency: 3, Priority: 50,
	}
	s.Require().NoError(s.repo.ProvisionAccount(s.ctx, &service.AccountProvisioningSpec{
		Account: account, Identity: &policy, FinalStatus: service.StatusActive,
		Schedulable: true, ProvisioningState: service.AccountProvisioningActive,
	}))
	_, err := s.client.Account.UpdateOneID(account.ID).SetCredentials(map[string]any{}).Save(s.ctx)
	s.Require().Error(err)
}

func (s *AccountRepoSuite) TestLegacyCachedAccountUpdatePreservesProvisioningAndIdentityPolicy() {
	policy := service.CodexIdentityPolicySpec{
		Mode: service.CodexIdentityPolicyOSProfileDevicePool,
		Profiles: []service.CodexOSProfilePolicy{{
			OSClass: service.CodexOSLinux, CanonicalSurface: service.CodexSurfaceCLI,
			Architecture: service.CodexArchX8664, SlotCount: 1,
		}},
	}
	account := &service.Account{
		Name: "legacy-cache-update", Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth,
		Credentials: map[string]any{"access_token": "before"}, Extra: map[string]any{},
		Status: service.StatusActive, Schedulable: true, Concurrency: 3, Priority: 50,
	}
	s.Require().NoError(s.repo.ProvisionAccount(s.ctx, &service.AccountProvisioningSpec{
		Account: account, Identity: &policy, FinalStatus: service.StatusActive,
		Schedulable: true, ProvisioningState: service.AccountProvisioningActive,
	}))

	legacyPayload, err := s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err)
	legacyPayload.ProvisioningState = ""
	legacyPayload.CodexIdentityPolicy = service.CodexIdentityPolicySpec{}
	legacyPayload.Credentials["access_token"] = "after"
	s.Require().NoError(s.repo.Update(s.ctx, legacyPayload))

	updated, err := s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err)
	s.Require().Equal(service.AccountProvisioningActive, updated.ProvisioningState)
	s.Require().Equal(service.CodexIdentityPolicyOSProfileDevicePool, updated.CodexIdentityPolicy.Mode)
	s.Require().EqualValues(1, updated.CodexIdentityPolicy.Version)
	var profileCount int
	s.Require().NoError(scanSingleRow(s.ctx, s.repo.sql, "SELECT COUNT(*) FROM account_codex_profiles WHERE account_id=$1", []any{account.ID}, &profileCount))
	s.Require().Equal(1, profileCount)
}

func (s *AccountRepoSuite) TestAccountDefaultProxyChangeRotatesOnlyInheritedProfileEpoch() {
	proxyOne := mustCreateProxy(s.T(), s.client, &service.Proxy{Name: "identity-proxy-one"})
	proxyTwo := mustCreateProxy(s.T(), s.client, &service.Proxy{Name: "identity-proxy-two"})
	user := mustCreateUser(s.T(), s.client, &service.User{Email: "identity-proxy-epoch@example.com"})
	apiKey := mustCreateApiKey(s.T(), s.client, &service.APIKey{UserID: user.ID, Key: "sk-identity-proxy-epoch"})
	policy := service.CodexIdentityPolicySpec{
		Mode: service.CodexIdentityPolicyOSProfileDevicePool,
		Profiles: []service.CodexOSProfilePolicy{
			{OSClass: service.CodexOSLinux, CanonicalSurface: service.CodexSurfaceCLI, Architecture: service.CodexArchX8664, SlotCount: 1},
			{OSClass: service.CodexOSWindows, CanonicalSurface: service.CodexSurfaceCLI, Architecture: service.CodexArchX8664, SlotCount: 1,
				Slots: []service.CodexDeviceSlotPolicy{{Index: 0, ProxyID: &proxyOne.ID}}},
		},
	}
	account := &service.Account{
		Name: "proxy-epoch-account", Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth,
		Credentials: map[string]any{"access_token": "test-token"}, Extra: map[string]any{},
		ProxyID: &proxyOne.ID, Status: service.StatusActive, Schedulable: true, Concurrency: 3, Priority: 50,
	}
	s.Require().NoError(s.repo.ProvisionAccount(s.ctx, &service.AccountProvisioningSpec{
		Account: account, Identity: &policy, FinalStatus: service.StatusActive,
		Schedulable: true, ProvisioningState: service.AccountProvisioningActive,
	}))
	oldBinding, err := s.repo.ResolveCodexDeviceBinding(s.ctx, account.ID, apiKey.ID, service.CodexOSLinux, service.CodexSurfaceCLI)
	s.Require().NoError(err)
	s.Require().NotNil(oldBinding.ProxyID)
	s.Require().Equal(proxyOne.ID, *oldBinding.ProxyID)
	requested := account.CodexIdentityPolicy
	account.ProxyID = &proxyTwo.ID
	s.Require().NoError(s.repo.UpdateProvisionedAccount(s.ctx, &service.AccountProvisioningSpec{
		Account: account, Identity: &requested, FinalStatus: service.StatusActive,
		Schedulable: true, ProvisioningState: service.AccountProvisioningActive,
	}, nil, nil, account.RateMultiplier))

	profiles := make(map[service.CodexOSClass]service.CodexOSProfilePolicy)
	for _, profile := range account.CodexIdentityPolicy.Profiles {
		profiles[profile.OSClass] = profile
	}
	s.Require().EqualValues(2, account.CodexIdentityPolicy.Version)
	s.Require().EqualValues(2, profiles[service.CodexOSLinux].Epoch)
	s.Require().EqualValues(1, profiles[service.CodexOSWindows].Epoch)
	stillOld, err := s.repo.ResolveCodexDeviceBinding(s.ctx, account.ID, apiKey.ID, service.CodexOSLinux, service.CodexSurfaceCLI)
	s.Require().NoError(err)
	s.Require().Equal(oldBinding.SlotID, stillOld.SlotID)
	s.Require().Equal("draining", stillOld.State)
	s.Require().NotNil(stillOld.ProxyID)
	s.Require().Equal(proxyOne.ID, *stillOld.ProxyID, "old epoch must retain its old effective proxy during drain")
}

func (s *AccountRepoSuite) TestCodexDeviceBindingDrainsAfterAffinitySilence() {
	user := mustCreateUser(s.T(), s.client, &service.User{Email: "codex-binding@example.com"})
	apiKey := mustCreateApiKey(s.T(), s.client, &service.APIKey{UserID: user.ID, Key: "sk-codex-binding"})
	policy := service.CodexIdentityPolicySpec{
		Mode:               service.CodexIdentityPolicyOSProfileDevicePool,
		AffinityTTLSeconds: 60,
		Profiles: []service.CodexOSProfilePolicy{{
			OSClass: service.CodexOSLinux, CanonicalSurface: service.CodexSurfaceCLI,
			Architecture: service.CodexArchX8664, SlotCount: 2,
		}},
	}
	account := &service.Account{
		Name: "binding-account", Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth,
		Credentials: map[string]any{"access_token": "test-token"}, Extra: map[string]any{},
		Status: service.StatusActive, Schedulable: true, Concurrency: 3, Priority: 50,
	}
	s.Require().NoError(s.repo.ProvisionAccount(s.ctx, &service.AccountProvisioningSpec{
		Account: account, Identity: &policy, FinalStatus: service.StatusActive,
		Schedulable: true, ProvisioningState: service.AccountProvisioningActive,
	}))

	first, err := s.repo.ResolveCodexDeviceBinding(s.ctx, account.ID, apiKey.ID, service.CodexOSLinux, service.CodexSurfaceCLI)
	s.Require().NoError(err)
	again, err := s.repo.ResolveCodexDeviceBinding(s.ctx, account.ID, apiKey.ID, service.CodexOSLinux, service.CodexSurfaceCLI)
	s.Require().NoError(err)
	s.Require().Equal(first.SlotID, again.SlotID)
	s.Require().EqualValues(1, first.Epoch)

	nextPolicy := account.CodexIdentityPolicy
	nextPolicy.Profiles = append([]service.CodexOSProfilePolicy(nil), account.CodexIdentityPolicy.Profiles...)
	nextPolicy.Profiles[0].CanonicalSurface = service.CodexSurfaceDesktop
	s.Require().NoError(s.repo.UpdateProvisionedAccount(s.ctx, &service.AccountProvisioningSpec{
		Account: account, Identity: &nextPolicy, FinalStatus: service.StatusActive,
		Schedulable: true, ProvisioningState: service.AccountProvisioningActive,
	}, nil, nil, account.RateMultiplier))

	stillOld, err := s.repo.ResolveCodexDeviceBinding(s.ctx, account.ID, apiKey.ID, service.CodexOSLinux, service.CodexSurfaceCLI)
	s.Require().NoError(err)
	s.Require().Equal(first.SlotID, stillOld.SlotID)
	s.Require().Equal("draining", stillOld.State)

	_, err = s.repo.sql.ExecContext(s.ctx, `
		UPDATE account_codex_device_bindings SET updated_at=NOW()-INTERVAL '2 minutes'
		WHERE id=$1
	`, first.BindingID)
	s.Require().NoError(err)
	migrated, err := s.repo.ResolveCodexDeviceBinding(s.ctx, account.ID, apiKey.ID, service.CodexOSLinux, service.CodexSurfaceCLI)
	s.Require().NoError(err)
	s.Require().NotEqual(first.SlotID, migrated.SlotID)
	s.Require().EqualValues(2, migrated.Epoch)
	s.Require().Equal("active", migrated.State)

	var oldSlotCount int
	s.Require().NoError(scanSingleRow(s.ctx, s.repo.sql, `
		SELECT COUNT(*) FROM account_codex_device_slots
		WHERE account_id=$1 AND epoch=1
	`, []any{account.ID}, &oldSlotCount))
	s.Require().Zero(oldSlotCount)

	removeLinux := account.CodexIdentityPolicy
	removeLinux.Profiles = []service.CodexOSProfilePolicy{{
		OSClass: service.CodexOSWindows, CanonicalSurface: service.CodexSurfaceCLI,
		Architecture: service.CodexArchX8664, SlotCount: 1,
	}}
	s.Require().NoError(s.repo.UpdateProvisionedAccount(s.ctx, &service.AccountProvisioningSpec{
		Account: account, Identity: &removeLinux, FinalStatus: service.StatusActive,
		Schedulable: true, ProvisioningState: service.AccountProvisioningActive,
	}, nil, nil, account.RateMultiplier))
	var linuxBindingCount, linuxSlotCount int
	s.Require().NoError(scanSingleRow(s.ctx, s.repo.sql, `
		SELECT COUNT(*) FROM account_codex_device_bindings
		WHERE account_id=$1 AND os_class='linux'
	`, []any{account.ID}, &linuxBindingCount))
	s.Require().NoError(scanSingleRow(s.ctx, s.repo.sql, `
		SELECT COUNT(*)
		FROM account_codex_device_slots AS slots
		JOIN account_codex_profiles AS profiles ON profiles.id=slots.profile_id
		WHERE slots.account_id=$1 AND profiles.os_class='linux'
	`, []any{account.ID}, &linuxSlotCount))
	s.Require().Zero(linuxBindingCount)
	s.Require().Zero(linuxSlotCount)
}

func (s *AccountRepoSuite) TestFinalizeRemovesAbandonedStaleDrainingBinding() {
	user := mustCreateUser(s.T(), s.client, &service.User{Email: "codex-abandoned-binding@example.com"})
	apiKey := mustCreateApiKey(s.T(), s.client, &service.APIKey{UserID: user.ID, Key: "sk-codex-abandoned-binding"})
	policy := service.CodexIdentityPolicySpec{
		Mode: service.CodexIdentityPolicyOSProfileDevicePool, AffinityTTLSeconds: 60,
		Profiles: []service.CodexOSProfilePolicy{{
			OSClass: service.CodexOSLinux, CanonicalSurface: service.CodexSurfaceCLI,
			Architecture: service.CodexArchX8664, SlotCount: 1,
		}},
	}
	account := &service.Account{
		Name: "abandoned-binding-account", Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth,
		Credentials: map[string]any{"access_token": "test-token"}, Extra: map[string]any{},
		Status: service.StatusActive, Schedulable: true, Concurrency: 3, Priority: 50,
	}
	s.Require().NoError(s.repo.ProvisionAccount(s.ctx, &service.AccountProvisioningSpec{
		Account: account, Identity: &policy, FinalStatus: service.StatusActive,
		Schedulable: true, ProvisioningState: service.AccountProvisioningActive,
	}))
	binding, err := s.repo.ResolveCodexDeviceBinding(s.ctx, account.ID, apiKey.ID, service.CodexOSLinux, service.CodexSurfaceCLI)
	s.Require().NoError(err)
	next := account.CodexIdentityPolicy
	next.Profiles = append([]service.CodexOSProfilePolicy(nil), next.Profiles...)
	next.Profiles[0].CanonicalSurface = service.CodexSurfaceDesktop
	s.Require().NoError(s.repo.UpdateProvisionedAccount(s.ctx, &service.AccountProvisioningSpec{
		Account: account, Identity: &next, FinalStatus: service.StatusActive,
		Schedulable: true, ProvisioningState: service.AccountProvisioningActive,
	}, nil, nil, account.RateMultiplier))
	_, err = s.repo.sql.ExecContext(s.ctx, "UPDATE account_codex_device_bindings SET updated_at=NOW()-INTERVAL '2 minutes' WHERE id=$1", binding.BindingID)
	s.Require().NoError(err)
	deleted, err := s.repo.FinalizeDrainedCodexDeviceSlots(s.ctx, account.ID)
	s.Require().NoError(err)
	s.Require().EqualValues(1, deleted)
	var bindingCount int
	s.Require().NoError(scanSingleRow(s.ctx, s.repo.sql, "SELECT COUNT(*) FROM account_codex_device_bindings WHERE id=$1", []any{binding.BindingID}, &bindingCount))
	s.Require().Zero(bindingCount)
}

func (s *AccountRepoSuite) TestSessionOnlyPolicyUpdateRefreshesBindingVersionWithoutEpochRotation() {
	user := mustCreateUser(s.T(), s.client, &service.User{Email: "codex-session-policy@example.com"})
	apiKey := mustCreateApiKey(s.T(), s.client, &service.APIKey{UserID: user.ID, Key: "sk-codex-session-policy"})
	policy := service.CodexIdentityPolicySpec{
		Mode: service.CodexIdentityPolicyOSProfileDevicePool,
		Profiles: []service.CodexOSProfilePolicy{{
			OSClass: service.CodexOSLinux, CanonicalSurface: service.CodexSurfaceCLI,
			Architecture: service.CodexArchX8664, SlotCount: 1,
		}},
	}
	account := &service.Account{
		Name: "session-policy-account", Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth,
		Credentials: map[string]any{"access_token": "test-token"}, Extra: map[string]any{},
		Status: service.StatusActive, Schedulable: true, Concurrency: 3, Priority: 50,
	}
	s.Require().NoError(s.repo.ProvisionAccount(s.ctx, &service.AccountProvisioningSpec{
		Account: account, Identity: &policy, FinalStatus: service.StatusActive,
		Schedulable: true, ProvisioningState: service.AccountProvisioningActive,
	}))
	binding, err := s.repo.ResolveCodexDeviceBinding(s.ctx, account.ID, apiKey.ID, service.CodexOSLinux, service.CodexSurfaceCLI)
	s.Require().NoError(err)
	next := account.CodexIdentityPolicy
	next.SessionPolicy = service.CodexSessionPolicySpec{Mode: service.CodexSessionAPIKeyShared}
	s.Require().NoError(s.repo.UpdateProvisionedAccount(s.ctx, &service.AccountProvisioningSpec{
		Account: account, Identity: &next, FinalStatus: service.StatusActive,
		Schedulable: true, ProvisioningState: service.AccountProvisioningActive,
	}, nil, nil, account.RateMultiplier))
	updatedBinding, err := s.repo.ResolveCodexDeviceBinding(s.ctx, account.ID, apiKey.ID, service.CodexOSLinux, service.CodexSurfaceCLI)
	s.Require().NoError(err)
	s.Require().Equal(binding.SlotID, updatedBinding.SlotID)
	s.Require().EqualValues(1, updatedBinding.Epoch)
	s.Require().EqualValues(2, updatedBinding.PolicyVersion)
}

func (s *AccountRepoSuite) TestListSchedulable() {
	now := time.Now()
	group := mustCreateGroup(s.T(), s.client, &service.Group{Name: "g-sched"})

	okAcc := mustCreateAccount(s.T(), s.client, &service.Account{Name: "ok", Schedulable: true})
	mustBindAccountToGroup(s.T(), s.client, okAcc.ID, group.ID, 1)

	future := now.Add(10 * time.Minute)
	overloaded := mustCreateAccount(s.T(), s.client, &service.Account{Name: "over", Schedulable: true, OverloadUntil: &future})
	mustBindAccountToGroup(s.T(), s.client, overloaded.ID, group.ID, 1)

	sched, err := s.repo.ListSchedulable(s.ctx)
	s.Require().NoError(err, "ListSchedulable")
	ids := idsOfAccounts(sched)
	s.Require().Contains(ids, okAcc.ID)
	s.Require().NotContains(ids, overloaded.ID)
}

func (s *AccountRepoSuite) TestListSchedulableByGroupID_TimeBoundaries_And_StatusUpdates() {
	now := time.Now()
	group := mustCreateGroup(s.T(), s.client, &service.Group{Name: "g-sched"})

	okAcc := mustCreateAccount(s.T(), s.client, &service.Account{Name: "ok", Schedulable: true})
	mustBindAccountToGroup(s.T(), s.client, okAcc.ID, group.ID, 1)

	future := now.Add(10 * time.Minute)
	overloaded := mustCreateAccount(s.T(), s.client, &service.Account{Name: "over", Schedulable: true, OverloadUntil: &future})
	mustBindAccountToGroup(s.T(), s.client, overloaded.ID, group.ID, 1)

	rateLimited := mustCreateAccount(s.T(), s.client, &service.Account{Name: "rl", Schedulable: true})
	mustBindAccountToGroup(s.T(), s.client, rateLimited.ID, group.ID, 1)
	s.Require().NoError(s.repo.SetRateLimited(s.ctx, rateLimited.ID, now.Add(10*time.Minute)), "SetRateLimited")

	s.Require().NoError(s.repo.SetError(s.ctx, overloaded.ID, "boom"), "SetError")

	sched, err := s.repo.ListSchedulableByGroupID(s.ctx, group.ID)
	s.Require().NoError(err, "ListSchedulableByGroupID")
	s.Require().Len(sched, 1, "expected only ok account schedulable")
	s.Require().Equal(okAcc.ID, sched[0].ID)

	s.Require().NoError(s.repo.ClearRateLimit(s.ctx, rateLimited.ID), "ClearRateLimit")
	sched2, err := s.repo.ListSchedulableByGroupID(s.ctx, group.ID)
	s.Require().NoError(err, "ListSchedulableByGroupID after ClearRateLimit")
	s.Require().Len(sched2, 2, "expected 2 schedulable accounts after ClearRateLimit")
}

func (s *AccountRepoSuite) TestListSchedulableByPlatform() {
	mustCreateAccount(s.T(), s.client, &service.Account{Name: "a1", Platform: service.PlatformAnthropic, Schedulable: true})
	mustCreateAccount(s.T(), s.client, &service.Account{Name: "a2", Platform: service.PlatformOpenAI, Schedulable: true})

	accounts, err := s.repo.ListSchedulableByPlatform(s.ctx, service.PlatformAnthropic)
	s.Require().NoError(err)
	s.Require().Len(accounts, 1)
	s.Require().Equal(service.PlatformAnthropic, accounts[0].Platform)
}

func (s *AccountRepoSuite) TestListSchedulableByGroupIDAndPlatform() {
	group := mustCreateGroup(s.T(), s.client, &service.Group{Name: "g-sp"})
	a1 := mustCreateAccount(s.T(), s.client, &service.Account{Name: "a1", Platform: service.PlatformAnthropic, Schedulable: true})
	a2 := mustCreateAccount(s.T(), s.client, &service.Account{Name: "a2", Platform: service.PlatformOpenAI, Schedulable: true})
	mustBindAccountToGroup(s.T(), s.client, a1.ID, group.ID, 1)
	mustBindAccountToGroup(s.T(), s.client, a2.ID, group.ID, 2)

	accounts, err := s.repo.ListSchedulableByGroupIDAndPlatform(s.ctx, group.ID, service.PlatformAnthropic)
	s.Require().NoError(err)
	s.Require().Len(accounts, 1)
	s.Require().Equal(a1.ID, accounts[0].ID)
}

func (s *AccountRepoSuite) TestSetSchedulable() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-sched", Schedulable: true})
	cacheRecorder := &schedulerCacheRecorder{}
	s.repo.schedulerCache = cacheRecorder

	s.Require().NoError(s.repo.SetSchedulable(s.ctx, account.ID, false))

	got, err := s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err)
	s.Require().False(got.Schedulable)
	s.Require().Len(cacheRecorder.setAccounts, 1)
	s.Require().Equal(account.ID, cacheRecorder.setAccounts[0].ID)
}

func (s *AccountRepoSuite) TestBulkUpdate_SyncSchedulerSnapshotOnDisabled() {
	account1 := mustCreateAccount(s.T(), s.client, &service.Account{Name: "bulk-1", Status: service.StatusActive, Schedulable: true})
	account2 := mustCreateAccount(s.T(), s.client, &service.Account{Name: "bulk-2", Status: service.StatusActive, Schedulable: true})
	cacheRecorder := &schedulerCacheRecorder{}
	s.repo.schedulerCache = cacheRecorder

	disabled := service.StatusDisabled
	rows, err := s.repo.BulkUpdate(s.ctx, []int64{account1.ID, account2.ID}, service.AccountBulkUpdate{
		Status: &disabled,
	})
	s.Require().NoError(err)
	s.Require().Equal(int64(2), rows)

	s.Require().Len(cacheRecorder.setAccounts, 2)
	ids := map[int64]struct{}{}
	for _, acc := range cacheRecorder.setAccounts {
		ids[acc.ID] = struct{}{}
	}
	s.Require().Contains(ids, account1.ID)
	s.Require().Contains(ids, account2.ID)
}

// --- SetOverloaded / SetRateLimited / ClearRateLimit ---

func (s *AccountRepoSuite) TestSetOverloaded() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-over"})
	until := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
	cacheRecorder := &schedulerCacheRecorder{}
	s.repo.schedulerCache = cacheRecorder

	s.Require().NoError(s.repo.SetOverloaded(s.ctx, account.ID, until))

	got, err := s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err)
	s.Require().NotNil(got.OverloadUntil)
	s.Require().WithinDuration(until, *got.OverloadUntil, time.Second)
	s.Require().Len(cacheRecorder.setAccounts, 1)
	s.Require().Equal(account.ID, cacheRecorder.setAccounts[0].ID)
	s.Require().NotNil(cacheRecorder.setAccounts[0].OverloadUntil)
	s.Require().WithinDuration(until, *cacheRecorder.setAccounts[0].OverloadUntil, time.Second)
}

func (s *AccountRepoSuite) TestSetRateLimited() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-rl"})
	resetAt := time.Date(2025, 6, 15, 14, 0, 0, 0, time.UTC)

	s.Require().NoError(s.repo.SetRateLimited(s.ctx, account.ID, resetAt))

	got, err := s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err)
	s.Require().NotNil(got.RateLimitedAt)
	s.Require().NotNil(got.RateLimitResetAt)
	s.Require().WithinDuration(resetAt, *got.RateLimitResetAt, time.Second)
}

func (s *AccountRepoSuite) TestSetRateLimitedIfLaterDoesNotShortenReset() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-rl-monotonic"})
	later := time.Now().Add(30 * time.Minute).UTC().Truncate(time.Second)
	earlier := time.Now().Add(5 * time.Minute).UTC().Truncate(time.Second)
	cacheRecorder := &schedulerCacheRecorder{}
	s.repo.schedulerCache = cacheRecorder

	s.Require().NoError(s.repo.SetRateLimitedIfLater(s.ctx, account.ID, later))
	s.Require().NoError(s.repo.SetRateLimitedIfLater(s.ctx, account.ID, earlier))

	got, err := s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err)
	s.Require().NotNil(got.RateLimitResetAt)
	s.Require().WithinDuration(later, *got.RateLimitResetAt, time.Second)
	s.Require().Len(cacheRecorder.setAccounts, 2)
	s.Require().NotNil(cacheRecorder.setAccounts[1].RateLimitResetAt)
	s.Require().WithinDuration(later, *cacheRecorder.setAccounts[1].RateLimitResetAt, time.Second)
}

func (s *AccountRepoSuite) TestClearRateLimitIfObservedProtectsRearmed429Generation() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{
		Name:     "acc-rl-conditional-clear",
		Platform: service.PlatformGrok,
		Type:     service.AccountTypeOAuth,
	})
	firstReset := time.Now().Add(30 * time.Minute).UTC().Truncate(time.Second)
	rearmedReset := time.Now().Add(5 * time.Minute).UTC().Truncate(time.Second)

	s.Require().NoError(s.repo.SetRateLimitedIfLater(s.ctx, account.ID, firstReset))
	staleGeneration, err := s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err)
	s.Require().NotNil(staleGeneration.RateLimitedAt)
	s.Require().NotNil(staleGeneration.RateLimitResetAt)
	cleared, err := s.repo.ClearRateLimitIfObserved(s.ctx, account.ID, *staleGeneration.RateLimitedAt, *staleGeneration.RateLimitResetAt)
	s.Require().NoError(err)
	s.Require().True(cleared)

	// A newer generation may legitimately re-arm a shorter boundary after the
	// first generation was cleared. The stale success must not erase it.
	s.Require().NoError(s.repo.SetRateLimitedIfLater(s.ctx, account.ID, rearmedReset))
	cleared, err = s.repo.ClearRateLimitIfObserved(s.ctx, account.ID, *staleGeneration.RateLimitedAt, *staleGeneration.RateLimitResetAt)
	s.Require().NoError(err)
	s.Require().False(cleared)

	got, err := s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err)
	s.Require().NotNil(got.RateLimitedAt)
	s.Require().NotNil(got.RateLimitResetAt)
	s.Require().WithinDuration(rearmedReset, *got.RateLimitResetAt, time.Second)

	// An admin can retype the row while the successful OAuth request is still
	// in flight. The stale OAuth recovery must not cross into API-key state even
	// when both observed timestamps still match.
	_, err = s.client.Account.UpdateOneID(account.ID).
		SetType(service.AccountTypeAPIKey).
		Save(s.ctx)
	s.Require().NoError(err)
	cleared, err = s.repo.ClearRateLimitIfObserved(s.ctx, account.ID, *got.RateLimitedAt, *got.RateLimitResetAt)
	s.Require().NoError(err)
	s.Require().False(cleared)

	retyped, err := s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err)
	s.Require().Equal(service.AccountTypeAPIKey, retyped.Type)
	s.Require().NotNil(retyped.RateLimitedAt)
	s.Require().NotNil(retyped.RateLimitResetAt)
	s.Require().WithinDuration(rearmedReset, *retyped.RateLimitResetAt, time.Second)
}

func (s *AccountRepoSuite) TestClearRateLimit() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-clear"})
	until := time.Now().Add(1 * time.Hour)
	s.Require().NoError(s.repo.SetOverloaded(s.ctx, account.ID, until))
	s.Require().NoError(s.repo.SetRateLimited(s.ctx, account.ID, until))

	s.Require().NoError(s.repo.ClearRateLimit(s.ctx, account.ID))

	got, err := s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err)
	s.Require().Nil(got.RateLimitedAt)
	s.Require().Nil(got.RateLimitResetAt)
	s.Require().Nil(got.OverloadUntil)
}

func (s *AccountRepoSuite) TestResetQuotaUsedAndClearRateLimitCooldownPreservesOtherRuntimeState() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{
		Name: "acc-reset-quota-cooldown",
		Extra: map[string]any{
			"quota_used":        12.5,
			"quota_daily_used":  5.0,
			"quota_weekly_used": 9.0,
			"model_rate_limits": map[string]any{
				"claude-sonnet-4-5": map[string]any{"rate_limit_reset_at": "2026-09-01T10:00:00Z"},
			},
		},
	})
	until := time.Now().Add(1 * time.Hour)
	s.Require().NoError(s.repo.SetOverloaded(s.ctx, account.ID, until))
	s.Require().NoError(s.repo.SetRateLimited(s.ctx, account.ID, until))
	s.Require().NoError(s.repo.SetTempUnschedulable(s.ctx, account.ID, until, "preserve-me"))

	cacheRecorder := &schedulerCacheRecorder{}
	s.repo.schedulerCache = cacheRecorder

	s.Require().NoError(s.repo.ResetQuotaUsedAndClearRateLimitCooldown(s.ctx, account.ID))

	got, err := s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err)
	s.Require().Nil(got.RateLimitedAt)
	s.Require().Nil(got.RateLimitResetAt)
	s.Require().NotNil(got.OverloadUntil)
	s.Require().WithinDuration(until, *got.OverloadUntil, time.Second)
	s.Require().NotNil(got.TempUnschedulableUntil)
	s.Require().WithinDuration(until, *got.TempUnschedulableUntil, time.Second)
	s.Require().Equal("preserve-me", got.TempUnschedulableReason)
	s.Require().Contains(got.Extra, "model_rate_limits")
	s.Require().Equal(float64(0), got.Extra["quota_used"])
	s.Require().Equal(float64(0), got.Extra["quota_daily_used"])
	s.Require().Equal(float64(0), got.Extra["quota_weekly_used"])

	var pendingEventExists bool
	s.Require().NoError(scanSingleRow(s.ctx, s.repo.sql, `
		SELECT EXISTS (
			SELECT 1 FROM scheduler_outbox
			WHERE event_type = $1 AND account_id = $2 AND dedup_key IS NOT NULL
		)`, []any{service.SchedulerOutboxEventAccountChanged, account.ID}, &pendingEventExists))
	s.Require().True(pendingEventExists)
	s.Require().Len(cacheRecorder.setAccounts, 1)
	s.Require().Equal(account.ID, cacheRecorder.setAccounts[0].ID)
}

func (s *AccountRepoSuite) TestTempUnschedulableFieldsLoadedByGetByIDAndGetByIDs() {
	acc1 := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-temp-1"})
	acc2 := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-temp-2"})

	until := time.Now().Add(15 * time.Minute).UTC().Truncate(time.Second)
	reason := `{"rule":"429","matched_keyword":"too many requests"}`
	s.Require().NoError(s.repo.SetTempUnschedulable(s.ctx, acc1.ID, until, reason))

	gotByID, err := s.repo.GetByID(s.ctx, acc1.ID)
	s.Require().NoError(err)
	s.Require().NotNil(gotByID.TempUnschedulableUntil)
	s.Require().WithinDuration(until, *gotByID.TempUnschedulableUntil, time.Second)
	s.Require().Equal(reason, gotByID.TempUnschedulableReason)

	gotByIDs, err := s.repo.GetByIDs(s.ctx, []int64{acc2.ID, acc1.ID})
	s.Require().NoError(err)
	s.Require().Len(gotByIDs, 2)
	s.Require().Equal(acc2.ID, gotByIDs[0].ID)
	s.Require().Nil(gotByIDs[0].TempUnschedulableUntil)
	s.Require().Equal("", gotByIDs[0].TempUnschedulableReason)
	s.Require().Equal(acc1.ID, gotByIDs[1].ID)
	s.Require().NotNil(gotByIDs[1].TempUnschedulableUntil)
	s.Require().WithinDuration(until, *gotByIDs[1].TempUnschedulableUntil, time.Second)
	s.Require().Equal(reason, gotByIDs[1].TempUnschedulableReason)

	cacheRecorder := &schedulerCacheRecorder{}
	s.repo.schedulerCache = cacheRecorder

	s.Require().NoError(s.repo.ClearTempUnschedulable(s.ctx, acc1.ID))
	cleared, err := s.repo.GetByID(s.ctx, acc1.ID)
	s.Require().NoError(err)
	s.Require().Nil(cleared.TempUnschedulableUntil)
	s.Require().Equal("", cleared.TempUnschedulableReason)
	s.Require().Len(cacheRecorder.setAccounts, 1)
	s.Require().Equal(acc1.ID, cacheRecorder.setAccounts[0].ID)
	s.Require().Nil(cacheRecorder.setAccounts[0].TempUnschedulableUntil)
	s.Require().Equal("", cacheRecorder.setAccounts[0].TempUnschedulableReason)
}

func (s *AccountRepoSuite) TestSetTempUnschedulableSkipsOutboxWhenWindowDoesNotExtend() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-temp-noop"})
	cacheRecorder := &schedulerCacheRecorder{}
	s.repo.schedulerCache = cacheRecorder

	_, err := s.repo.sql.ExecContext(s.ctx, "TRUNCATE scheduler_outbox")
	s.Require().NoError(err)

	until := time.Now().Add(30 * time.Minute).UTC().Truncate(time.Second)
	s.Require().NoError(s.repo.SetTempUnschedulable(s.ctx, account.ID, until, "first"))

	var count int
	err = scanSingleRow(s.ctx, s.repo.sql, "SELECT COUNT(*) FROM scheduler_outbox", nil, &count)
	s.Require().NoError(err)
	s.Require().Equal(1, count)
	s.Require().Len(cacheRecorder.setAccounts, 1)

	s.Require().NoError(s.repo.SetTempUnschedulable(s.ctx, account.ID, until.Add(-5*time.Minute), "older"))

	err = scanSingleRow(s.ctx, s.repo.sql, "SELECT COUNT(*) FROM scheduler_outbox", nil, &count)
	s.Require().NoError(err)
	s.Require().Equal(1, count)
	s.Require().Len(cacheRecorder.setAccounts, 1)

	got, err := s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err)
	s.Require().Equal("first", got.TempUnschedulableReason)
	s.Require().NotNil(got.TempUnschedulableUntil)
	s.Require().WithinDuration(until, *got.TempUnschedulableUntil, time.Second)
}

func (s *AccountRepoSuite) TestClearModelRateLimits_SyncsSchedulerSnapshot() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{
		Name: "acc-clear-model-rate",
		Extra: map[string]any{
			"model_rate_limits": map[string]any{
				"claude-sonnet-4-5": map[string]any{
					"rate_limit_reset_at": "2026-06-03T10:00:00Z",
				},
			},
		},
	})
	cacheRecorder := &schedulerCacheRecorder{}
	s.repo.schedulerCache = cacheRecorder

	s.Require().NoError(s.repo.ClearModelRateLimits(s.ctx, account.ID))

	got, err := s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err)
	s.Require().NotContains(got.Extra, "model_rate_limits")
	s.Require().Len(cacheRecorder.setAccounts, 1)
	s.Require().Equal(account.ID, cacheRecorder.setAccounts[0].ID)
	s.Require().NotContains(cacheRecorder.setAccounts[0].Extra, "model_rate_limits")
}

// --- UpdateLastUsed ---

func (s *AccountRepoSuite) TestUpdateLastUsed() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-used"})
	s.Require().Nil(account.LastUsedAt)

	s.Require().NoError(s.repo.UpdateLastUsed(s.ctx, account.ID))

	got, err := s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err)
	s.Require().NotNil(got.LastUsedAt)
}

// --- SetError ---

func (s *AccountRepoSuite) TestSetError() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-err", Status: service.StatusActive, Schedulable: true})
	cacheRecorder := &schedulerCacheRecorder{}
	s.repo.schedulerCache = cacheRecorder
	_, err := s.repo.sql.ExecContext(s.ctx, "TRUNCATE scheduler_outbox")
	s.Require().NoError(err)

	s.Require().NoError(s.repo.SetError(s.ctx, account.ID, "something went wrong"))

	got, err := s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err)
	s.Require().Equal(service.StatusError, got.Status)
	s.Require().Equal("something went wrong", got.ErrorMessage)
	s.Require().False(got.Schedulable)
	s.Require().Len(cacheRecorder.setAccounts, 1)
	s.Require().Equal(account.ID, cacheRecorder.setAccounts[0].ID)
	s.Require().Equal(service.StatusError, cacheRecorder.setAccounts[0].Status)
	s.Require().False(cacheRecorder.setAccounts[0].Schedulable)

	var outboxCount int
	err = scanSingleRow(
		s.ctx,
		s.repo.sql,
		"SELECT COUNT(*) FROM scheduler_outbox WHERE event_type = $1 AND account_id = $2",
		[]any{service.SchedulerOutboxEventAccountChanged, account.ID},
		&outboxCount,
	)
	s.Require().NoError(err)
	s.Require().Equal(1, outboxCount)
}

func (s *AccountRepoSuite) TestSetGrokOAuthErrorIfCredentialsUnchanged_AppliesAndSyncsSchedulerState() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{
		Name:        "grok-conditional-error-applied",
		Platform:    service.PlatformGrok,
		Type:        service.AccountTypeOAuth,
		Status:      service.StatusActive,
		Schedulable: true,
		Credentials: map[string]any{"access_token": "observed", "_token_version": int64(7)},
	})
	observed, err := s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err)
	cacheRecorder := &schedulerCacheRecorder{}
	s.repo.schedulerCache = cacheRecorder
	_, err = s.repo.sql.ExecContext(s.ctx, "TRUNCATE scheduler_outbox")
	s.Require().NoError(err)

	applied, err := s.repo.SetGrokOAuthErrorIfCredentialsUnchanged(
		s.ctx,
		account.ID,
		observed.Credentials,
		"missing refresh token",
	)

	s.Require().NoError(err)
	s.Require().True(applied)
	got, err := s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err)
	s.Require().Equal(service.StatusError, got.Status)
	s.Require().False(got.Schedulable)
	s.Require().Equal("missing refresh token", got.ErrorMessage)
	s.Require().Len(cacheRecorder.setAccounts, 1)
	s.Require().Equal(service.StatusError, cacheRecorder.setAccounts[0].Status)

	var outboxCount int
	err = scanSingleRow(
		s.ctx,
		s.repo.sql,
		"SELECT COUNT(*) FROM scheduler_outbox WHERE event_type = $1 AND account_id = $2",
		[]any{service.SchedulerOutboxEventAccountChanged, account.ID},
		&outboxCount,
	)
	s.Require().NoError(err)
	s.Require().Equal(1, outboxCount)
}

func (s *AccountRepoSuite) TestSetGrokOAuthErrorIfCredentialsUnchanged_SkipsConcurrentReauthorization() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{
		Name:        "grok-conditional-error-reauthorized",
		Platform:    service.PlatformGrok,
		Type:        service.AccountTypeOAuth,
		Status:      service.StatusActive,
		Schedulable: true,
		Credentials: map[string]any{"access_token": "observed", "_token_version": int64(7)},
	})
	observed, err := s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err)
	s.Require().NoError(s.repo.UpdateCredentials(s.ctx, account.ID, map[string]any{
		"access_token":   "fresh-access",
		"refresh_token":  "fresh-refresh",
		"expires_at":     time.Now().UTC().Add(4 * time.Hour).Format(time.RFC3339),
		"_token_version": int64(8),
	}))
	cacheRecorder := &schedulerCacheRecorder{}
	s.repo.schedulerCache = cacheRecorder
	_, err = s.repo.sql.ExecContext(s.ctx, "TRUNCATE scheduler_outbox")
	s.Require().NoError(err)

	applied, err := s.repo.SetGrokOAuthErrorIfCredentialsUnchanged(
		s.ctx,
		account.ID,
		observed.Credentials,
		"stale reconciliation",
	)

	s.Require().NoError(err)
	s.Require().False(applied)
	got, err := s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err)
	s.Require().Equal(service.StatusActive, got.Status)
	s.Require().True(got.Schedulable)
	s.Require().Equal("fresh-refresh", got.GetGrokRefreshToken())
	s.Require().Empty(cacheRecorder.setAccounts, "a lost compare-and-set race must not rewrite the scheduler snapshot")

	var outboxCount int
	err = scanSingleRow(
		s.ctx,
		s.repo.sql,
		"SELECT COUNT(*) FROM scheduler_outbox WHERE event_type = $1 AND account_id = $2",
		[]any{service.SchedulerOutboxEventAccountChanged, account.ID},
		&outboxCount,
	)
	s.Require().NoError(err)
	s.Require().Zero(outboxCount, "a lost compare-and-set race must not enqueue a stale account change")
}

func (s *AccountRepoSuite) TestUpdateGrokOAuthCredentialsIfUnchanged_AppliesAndPublishesSchedulerState() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{
		Name:        "grok-refresh-success-cas-applied",
		Platform:    service.PlatformGrok,
		Type:        service.AccountTypeOAuth,
		Status:      service.StatusActive,
		Schedulable: true,
		Credentials: map[string]any{
			"access_token":   "attempted-access",
			"refresh_token":  "attempted-refresh",
			"_token_version": int64(10),
		},
	})
	observed, err := s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err)
	cacheRecorder := &schedulerCacheRecorder{}
	s.repo.schedulerCache = cacheRecorder
	_, err = s.repo.sql.ExecContext(s.ctx, "TRUNCATE scheduler_outbox")
	s.Require().NoError(err)

	applied, err := s.repo.UpdateGrokOAuthCredentialsIfUnchanged(
		s.ctx,
		account.ID,
		observed.Credentials,
		observed.ProxyID,
		map[string]any{
			"access_token":   "rotated-access",
			"refresh_token":  "rotated-refresh",
			"_token_version": int64(11),
		},
	)

	s.Require().NoError(err)
	s.Require().True(applied)
	got, err := s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err)
	s.Require().Equal("rotated-refresh", got.GetGrokRefreshToken())
	s.Require().Len(cacheRecorder.setAccounts, 1)
	s.Require().Equal("rotated-refresh", cacheRecorder.setAccounts[0].GetGrokRefreshToken())
	s.Require().NoError(cacheRecorder.setCtxErr)

	var outboxCount int
	err = scanSingleRow(
		s.ctx,
		s.repo.sql,
		"SELECT COUNT(*) FROM scheduler_outbox WHERE event_type = $1 AND account_id = $2",
		[]any{service.SchedulerOutboxEventAccountChanged, account.ID},
		&outboxCount,
	)
	s.Require().NoError(err)
	s.Require().Equal(1, outboxCount)
}

func (s *AccountRepoSuite) TestUpdateGrokOAuthCredentialsIfUnchanged_SkipsConcurrentReauthorization() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{
		Name:        "grok-refresh-success-cas-reauthorized",
		Platform:    service.PlatformGrok,
		Type:        service.AccountTypeOAuth,
		Status:      service.StatusActive,
		Schedulable: true,
		Credentials: map[string]any{
			"access_token":   "attempted-access",
			"refresh_token":  "attempted-refresh",
			"_token_version": int64(20),
		},
	})
	observed, err := s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err)
	s.Require().NoError(s.repo.UpdateCredentials(s.ctx, account.ID, map[string]any{
		"access_token":   "reauthorized-access",
		"refresh_token":  "reauthorized-refresh",
		"_token_version": int64(21),
	}))
	cacheRecorder := &schedulerCacheRecorder{}
	s.repo.schedulerCache = cacheRecorder
	_, err = s.repo.sql.ExecContext(s.ctx, "TRUNCATE scheduler_outbox")
	s.Require().NoError(err)

	applied, err := s.repo.UpdateGrokOAuthCredentialsIfUnchanged(
		s.ctx,
		account.ID,
		observed.Credentials,
		observed.ProxyID,
		map[string]any{
			"access_token":   "provider-access",
			"refresh_token":  "provider-refresh",
			"_token_version": int64(22),
		},
	)

	s.Require().NoError(err)
	s.Require().False(applied)
	got, err := s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err)
	s.Require().Equal("reauthorized-refresh", got.GetGrokRefreshToken())
	s.Require().Empty(cacheRecorder.setAccounts)

	var outboxCount int
	err = scanSingleRow(
		s.ctx,
		s.repo.sql,
		"SELECT COUNT(*) FROM scheduler_outbox WHERE event_type = $1 AND account_id = $2",
		[]any{service.SchedulerOutboxEventAccountChanged, account.ID},
		&outboxCount,
	)
	s.Require().NoError(err)
	s.Require().Zero(outboxCount)
}

func (s *AccountRepoSuite) TestGrokOAuthConditionalMutation_DetachesBoundedSnapshotSync() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{
		Name:        "grok-conditional-detached-sync",
		Platform:    service.PlatformGrok,
		Type:        service.AccountTypeOAuth,
		Status:      service.StatusActive,
		Schedulable: true,
		Credentials: map[string]any{"access_token": "observed"},
	})
	observed, err := s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err)
	ctx, cancel := context.WithCancel(context.Background())
	cacheRecorder := &schedulerCacheRecorder{}
	repo := newAccountRepositoryWithSQL(s.client, &cancelAfterAtomicMutationSQLExecutor{
		sqlExecutor: s.repo.sql,
		cancel:      cancel,
	}, cacheRecorder)

	applied, err := repo.SetGrokOAuthErrorIfCredentialsUnchanged(
		ctx,
		account.ID,
		observed.Credentials,
		"missing refresh token",
	)

	s.Require().NoError(err)
	s.Require().True(applied)
	s.Require().ErrorIs(ctx.Err(), context.Canceled)
	s.Require().Len(cacheRecorder.setAccounts, 1)
	s.Require().NoError(cacheRecorder.setCtxErr, "immediate scheduler propagation must use a bounded detached context")
}

func TestGrokOAuthConditionalMutationRollsBackWhenOutboxInsertFails(t *testing.T) {
	client := testEntClient(t)
	account := mustCreateAccount(t, client, &service.Account{
		Name:        "grok-conditional-atomic-outbox-failure",
		Platform:    service.PlatformGrok,
		Type:        service.AccountTypeOAuth,
		Status:      service.StatusActive,
		Schedulable: true,
		Credentials: map[string]any{"access_token": "observed"},
	})
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(context.Background(), "DELETE FROM scheduler_outbox WHERE account_id = $1", account.ID)
		_ = client.Account.DeleteOneID(account.ID).Exec(context.Background())
	})
	repo := newAccountRepositoryWithSQL(client, &failAtomicSchedulerOutboxSQLExecutor{sqlExecutor: integrationDB}, nil)

	applied, err := repo.SetGrokOAuthErrorIfCredentialsUnchanged(
		context.Background(),
		account.ID,
		account.Credentials,
		"missing refresh token",
	)

	require.Error(t, err)
	require.False(t, applied)
	got, readErr := repo.GetByID(context.Background(), account.ID)
	require.NoError(t, readErr)
	require.Equal(t, service.StatusActive, got.Status)
	require.True(t, got.Schedulable)
	require.Empty(t, got.ErrorMessage)
	var outboxCount int
	require.NoError(t, integrationDB.QueryRowContext(
		context.Background(),
		"SELECT COUNT(*) FROM scheduler_outbox WHERE account_id = $1",
		account.ID,
	).Scan(&outboxCount))
	require.Zero(t, outboxCount)
}

func (s *AccountRepoSuite) TestUpdateErrorStatusUnschedulesAccount() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-update-err", Status: service.StatusActive, Schedulable: true})
	account.Status = service.StatusError
	account.ErrorMessage = "token revoked"
	account.Schedulable = true

	s.Require().NoError(s.repo.Update(s.ctx, account))

	got, err := s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err)
	s.Require().Equal(service.StatusError, got.Status)
	s.Require().Equal("token revoked", got.ErrorMessage)
	s.Require().False(got.Schedulable)
}

func (s *AccountRepoSuite) TestClearError_SyncSchedulerSnapshotOnRecovery() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{
		Name:         "acc-clear-err",
		Status:       service.StatusError,
		ErrorMessage: "temporary error",
	})
	cacheRecorder := &schedulerCacheRecorder{}
	s.repo.schedulerCache = cacheRecorder

	s.Require().NoError(s.repo.ClearError(s.ctx, account.ID))

	got, err := s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err)
	s.Require().Equal(service.StatusActive, got.Status)
	s.Require().Empty(got.ErrorMessage)
	s.Require().Len(cacheRecorder.setAccounts, 1)
	s.Require().Equal(account.ID, cacheRecorder.setAccounts[0].ID)
	s.Require().Equal(service.StatusActive, cacheRecorder.setAccounts[0].Status)
}

// --- UpdateSessionWindow ---

func (s *AccountRepoSuite) TestUpdateSessionWindow() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-win"})
	start := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)
	end := time.Date(2025, 6, 15, 15, 0, 0, 0, time.UTC)

	s.Require().NoError(s.repo.UpdateSessionWindow(s.ctx, account.ID, &start, &end, "active"))

	got, err := s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err)
	s.Require().NotNil(got.SessionWindowStart)
	s.Require().NotNil(got.SessionWindowEnd)
	s.Require().Equal("active", got.SessionWindowStatus)
}

// --- UpdateExtra ---

func (s *AccountRepoSuite) TestUpdateExtra_MergesFields() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{
		Name:  "acc-extra",
		Extra: map[string]any{"a": "1"},
	})
	s.Require().NoError(s.repo.UpdateExtra(s.ctx, account.ID, map[string]any{"b": "2"}), "UpdateExtra")

	got, err := s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err, "GetByID")
	s.Require().Equal("1", got.Extra["a"])
	s.Require().Equal("2", got.Extra["b"])
}

func (s *AccountRepoSuite) TestUpdateExtra_EmptyUpdates() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-extra-empty"})
	s.Require().NoError(s.repo.UpdateExtra(s.ctx, account.ID, map[string]any{}))
}

func (s *AccountRepoSuite) TestUpdateExtra_NilExtra() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-nil-extra", Extra: nil})
	s.Require().NoError(s.repo.UpdateExtra(s.ctx, account.ID, map[string]any{"key": "val"}))

	got, err := s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err)
	s.Require().Equal("val", got.Extra["key"])
}

func (s *AccountRepoSuite) TestUpdateExtra_SchedulerNeutralSkipsOutboxAndSyncsFreshSnapshot() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{
		Name:     "acc-extra-neutral",
		Platform: service.PlatformOpenAI,
		Extra:    map[string]any{"codex_usage_updated_at": "old"},
	})
	_, err := s.repo.sql.ExecContext(s.ctx, "TRUNCATE scheduler_outbox")
	s.Require().NoError(err)
	cacheRecorder := &schedulerCacheRecorder{
		accounts: map[int64]*service.Account{
			account.ID: {
				ID:       account.ID,
				Platform: account.Platform,
				Status:   service.StatusDisabled,
				Extra: map[string]any{
					"codex_usage_updated_at": "old",
				},
			},
		},
	}
	s.repo.schedulerCache = cacheRecorder

	updates := map[string]any{
		"codex_usage_updated_at":     "2026-03-11T10:00:00Z",
		"codex_5h_used_percent":      88.5,
		"session_window_utilization": 0.42,
	}
	s.Require().NoError(s.repo.UpdateExtra(s.ctx, account.ID, updates))

	got, err := s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err)
	s.Require().Equal("2026-03-11T10:00:00Z", got.Extra["codex_usage_updated_at"])
	s.Require().Equal(88.5, got.Extra["codex_5h_used_percent"])
	s.Require().Equal(0.42, got.Extra["session_window_utilization"])

	var outboxCount int
	s.Require().NoError(scanSingleRow(s.ctx, s.repo.sql, "SELECT COUNT(*) FROM scheduler_outbox", nil, &outboxCount))
	s.Require().Zero(outboxCount)
	s.Require().Len(cacheRecorder.setAccounts, 1)
	s.Require().NotNil(cacheRecorder.accounts[account.ID])
	s.Require().Equal(service.StatusActive, cacheRecorder.accounts[account.ID].Status)
	s.Require().Equal("2026-03-11T10:00:00Z", cacheRecorder.accounts[account.ID].Extra["codex_usage_updated_at"])
}

func (s *AccountRepoSuite) TestUpdateExtra_ExhaustedCodexSnapshotSyncsSchedulerCache() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{
		Name:     "acc-extra-codex-exhausted",
		Platform: service.PlatformOpenAI,
		Type:     service.AccountTypeOAuth,
		Extra:    map[string]any{},
	})
	cacheRecorder := &schedulerCacheRecorder{}
	s.repo.schedulerCache = cacheRecorder
	_, err := s.repo.sql.ExecContext(s.ctx, "TRUNCATE scheduler_outbox")
	s.Require().NoError(err)

	s.Require().NoError(s.repo.UpdateExtra(s.ctx, account.ID, map[string]any{
		"codex_7d_used_percent":        100.0,
		"codex_7d_reset_at":            "2026-03-12T13:00:00Z",
		"codex_7d_reset_after_seconds": 86400,
	}))

	var count int
	err = scanSingleRow(s.ctx, s.repo.sql, "SELECT COUNT(*) FROM scheduler_outbox", nil, &count)
	s.Require().NoError(err)
	s.Require().Equal(0, count)
	s.Require().Len(cacheRecorder.setAccounts, 1)
	s.Require().Equal(account.ID, cacheRecorder.setAccounts[0].ID)
	s.Require().Equal(service.StatusActive, cacheRecorder.setAccounts[0].Status)
	s.Require().Equal(100.0, cacheRecorder.setAccounts[0].Extra["codex_7d_used_percent"])
}

func (s *AccountRepoSuite) TestUpdateExtra_SchedulerRelevantStillEnqueuesOutbox() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{
		Name:     "acc-extra-mixed",
		Platform: service.PlatformAntigravity,
		Extra:    map[string]any{},
	})
	_, err := s.repo.sql.ExecContext(s.ctx, "TRUNCATE scheduler_outbox")
	s.Require().NoError(err)

	s.Require().NoError(s.repo.UpdateExtra(s.ctx, account.ID, map[string]any{
		"mixed_scheduling":       true,
		"codex_usage_updated_at": "2026-03-11T10:00:00Z",
	}))

	var count int
	err = scanSingleRow(s.ctx, s.repo.sql, "SELECT COUNT(*) FROM scheduler_outbox", nil, &count)
	s.Require().NoError(err)
	s.Require().Equal(1, count)
}

// --- GetByCRSAccountID ---

func (s *AccountRepoSuite) TestGetByCRSAccountID() {
	crsID := "crs-12345"
	mustCreateAccount(s.T(), s.client, &service.Account{
		Name:  "acc-crs",
		Extra: map[string]any{"crs_account_id": crsID},
	})

	got, err := s.repo.GetByCRSAccountID(s.ctx, crsID)
	s.Require().NoError(err)
	s.Require().NotNil(got)
	s.Require().Equal("acc-crs", got.Name)
}

func (s *AccountRepoSuite) TestGetByCRSAccountID_NotFound() {
	got, err := s.repo.GetByCRSAccountID(s.ctx, "non-existent")
	s.Require().NoError(err)
	s.Require().Nil(got)
}

func (s *AccountRepoSuite) TestGetByCRSAccountID_EmptyString() {
	got, err := s.repo.GetByCRSAccountID(s.ctx, "")
	s.Require().NoError(err)
	s.Require().Nil(got)
}

// TestGetByCRSAccountID_ExcludesSparkShadow 验证外审第7轮 P1:即便 spark 影子的 Extra 被误写入
// crs_account_id,CRS 查询也绝不能命中影子(否则会被当普通账号更新而覆盖 type/credentials/proxy)。
func (s *AccountRepoSuite) TestGetByCRSAccountID_ExcludesSparkShadow() {
	crsID := "crs-shadow-only-99"
	parent := mustCreateAccount(s.T(), s.client, &service.Account{
		Name: "crs-mother", Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth,
	})
	mustCreateAccount(s.T(), s.client, &service.Account{
		Name: "crs-shadow", Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth,
		ParentAccountID: &parent.ID,
		QuotaDimension:  service.QuotaDimensionSpark,
		Extra:           map[string]any{"crs_account_id": crsID},
	})

	got, err := s.repo.GetByCRSAccountID(s.ctx, crsID)
	s.Require().NoError(err)
	s.Require().Nil(got, "spark 影子即便带 crs_account_id 也不应被 CRS 命中")
}

// TestListCRSAccountIDs_ExcludesSparkShadow 验证外审第7轮 P1:影子的 crs_account_id 不应进入
// CRS 同步映射(否则后续 CRS 同步会把影子当普通账号更新)。
func (s *AccountRepoSuite) TestListCRSAccountIDs_ExcludesSparkShadow() {
	parent := mustCreateAccount(s.T(), s.client, &service.Account{
		Name: "crs-list-mother", Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth,
	})
	shadowCRSID := "crs-list-shadow-77"
	mustCreateAccount(s.T(), s.client, &service.Account{
		Name: "crs-list-shadow", Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth,
		ParentAccountID: &parent.ID,
		QuotaDimension:  service.QuotaDimensionSpark,
		Extra:           map[string]any{"crs_account_id": shadowCRSID},
	})

	ids, err := s.repo.ListCRSAccountIDs(s.ctx)
	s.Require().NoError(err)
	_, ok := ids[shadowCRSID]
	s.Require().False(ok, "影子的 crs_account_id 不应进入 CRS 映射")
}

// --- BulkUpdate ---

func (s *AccountRepoSuite) TestBulkUpdate() {
	a1 := mustCreateAccount(s.T(), s.client, &service.Account{Name: "bulk1", Priority: 1})
	a2 := mustCreateAccount(s.T(), s.client, &service.Account{Name: "bulk2", Priority: 1})

	newPriority := 99
	affected, err := s.repo.BulkUpdate(s.ctx, []int64{a1.ID, a2.ID}, service.AccountBulkUpdate{
		Priority: &newPriority,
	})
	s.Require().NoError(err)
	s.Require().GreaterOrEqual(affected, int64(1), "expected at least one affected row")

	got1, _ := s.repo.GetByID(s.ctx, a1.ID)
	got2, _ := s.repo.GetByID(s.ctx, a2.ID)
	s.Require().Equal(99, got1.Priority)
	s.Require().Equal(99, got2.Priority)
}

func (s *AccountRepoSuite) TestBulkUpdate_MergeCredentials() {
	a1 := mustCreateAccount(s.T(), s.client, &service.Account{
		Name:        "bulk-cred",
		Credentials: map[string]any{"existing": "value"},
	})

	_, err := s.repo.BulkUpdate(s.ctx, []int64{a1.ID}, service.AccountBulkUpdate{
		Credentials: map[string]any{"new_key": "new_value"},
	})
	s.Require().NoError(err)

	got, _ := s.repo.GetByID(s.ctx, a1.ID)
	s.Require().Equal("value", got.Credentials["existing"])
	s.Require().Equal("new_value", got.Credentials["new_key"])
}

func (s *AccountRepoSuite) TestBulkUpdate_MergeExtra() {
	a1 := mustCreateAccount(s.T(), s.client, &service.Account{
		Name:  "bulk-extra",
		Extra: map[string]any{"existing": "val"},
	})

	_, err := s.repo.BulkUpdate(s.ctx, []int64{a1.ID}, service.AccountBulkUpdate{
		Extra: map[string]any{"new_key": "new_val"},
	})
	s.Require().NoError(err)

	got, _ := s.repo.GetByID(s.ctx, a1.ID)
	s.Require().Equal("val", got.Extra["existing"])
	s.Require().Equal("new_val", got.Extra["new_key"])
}

func (s *AccountRepoSuite) TestBulkUpdate_EmptyIDs() {
	affected, err := s.repo.BulkUpdate(s.ctx, []int64{}, service.AccountBulkUpdate{})
	s.Require().NoError(err)
	s.Require().Zero(affected)
}

func (s *AccountRepoSuite) TestBulkUpdate_EmptyUpdates() {
	a1 := mustCreateAccount(s.T(), s.client, &service.Account{Name: "bulk-empty"})

	affected, err := s.repo.BulkUpdate(s.ctx, []int64{a1.ID}, service.AccountBulkUpdate{})
	s.Require().NoError(err)
	s.Require().Zero(affected)
}

func idsOfAccounts(accounts []service.Account) []int64 {
	out := make([]int64, 0, len(accounts))
	for i := range accounts {
		out = append(out, accounts[i].ID)
	}
	return out
}
