//go:build integration

package repository

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/accountgroup"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type AccountRepoSuite struct {
	suite.Suite
	ctx    context.Context
	client *dbent.Client
	repo   *accountRepository
REDACTED

type schedulerCacheRecorder struct {
	setAccounts []*service.Account
	deleteIDs   []int64
	accounts    map[int64]*service.Account
	setCtxErr   error
REDACTED

func (s *schedulerCacheRecorder) GetSnapshot(ctx context.Context, bucket service.SchedulerBucket) ([]*service.Account, bool, error) {
	return nil, false, nil
REDACTED

func (s *schedulerCacheRecorder) CaptureBucketWriteToken(ctx context.Context, bucket service.SchedulerBucket) (service.SchedulerBucketWriteToken, error) {
	return service.SchedulerBucketWriteToken{Bucket: bucket, Epoch: 1REDACTED, nil
REDACTED

func (s *schedulerCacheRecorder) SetSnapshot(ctx context.Context, bucket service.SchedulerBucket, token service.SchedulerBucketWriteToken, accounts []service.Account) error {
	return nil
REDACTED

func (s *schedulerCacheRecorder) RetireBucket(ctx context.Context, bucket service.SchedulerBucket) error {
	return nil
REDACTED

func (s *schedulerCacheRecorder) ReopenBucket(ctx context.Context, bucket service.SchedulerBucket) (service.SchedulerBucketWriteToken, error) {
	return service.SchedulerBucketWriteToken{Bucket: bucket, Epoch: 1REDACTED, nil
REDACTED

func (s *schedulerCacheRecorder) TryAcquireGroupLifecycleLease(_ context.Context, groupID int64, _ time.Duration) (service.SchedulerGroupLifecycleLease, bool, error) {
	return service.SchedulerGroupLifecycleLease{GroupID: groupID, OwnerToken: "scheduler-cache-recorder"REDACTED, true, nil
REDACTED

func (s *schedulerCacheRecorder) ReleaseGroupLifecycleLease(context.Context, service.SchedulerGroupLifecycleLease) error {
	return nil
REDACTED

func (s *schedulerCacheRecorder) GetAccount(ctx context.Context, accountID int64) (*service.Account, error) {
	if s.accounts == nil {
		return nil, nil
REDACTED
	return s.accounts[accountID], nil
REDACTED

func (s *schedulerCacheRecorder) SetAccount(ctx context.Context, account *service.Account) error {
	s.setCtxErr = ctx.Err()
	s.setAccounts = append(s.setAccounts, account)
	if s.accounts == nil {
		s.accounts = make(map[int64]*service.Account)
REDACTED
	if account != nil {
		s.accounts[account.ID] = account
REDACTED
	return nil
REDACTED

type failAtomicSchedulerOutboxSQLExecutor struct {
	sqlExecutor
REDACTED

func (e *failAtomicSchedulerOutboxSQLExecutor) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	if strings.Contains(query, "WITH updated AS") && strings.Contains(query, "INSERT INTO scheduler_outbox") && len(args) > 0 {
		args = append([]any(nil), args...)
		args[len(args)-1] = nil // event_type is NOT NULL; the whole statement must roll back.
REDACTED
	return e.sqlExecutor.ExecContext(ctx, query, args...)
REDACTED

type cancelAfterAtomicMutationSQLExecutor struct {
	sqlExecutor
	cancel context.CancelFunc
REDACTED

func (e *cancelAfterAtomicMutationSQLExecutor) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	result, err := e.sqlExecutor.ExecContext(ctx, query, args...)
	if err == nil && strings.Contains(query, "WITH updated AS") && strings.Contains(query, "INSERT INTO scheduler_outbox") {
		e.cancel()
REDACTED
	return result, err
REDACTED

func (s *schedulerCacheRecorder) DeleteAccount(ctx context.Context, accountID int64) error {
	s.deleteIDs = append(s.deleteIDs, accountID)
	if s.accounts != nil {
		delete(s.accounts, accountID)
REDACTED
	return nil
REDACTED

func (s *schedulerCacheRecorder) UpdateLastUsed(ctx context.Context, updates map[int64]time.Time) error {
	return nil
REDACTED

func (s *schedulerCacheRecorder) TryLockBucket(ctx context.Context, bucket service.SchedulerBucket, ttl time.Duration) (bool, error) {
	return true, nil
REDACTED

func (s *schedulerCacheRecorder) UnlockBucket(ctx context.Context, bucket service.SchedulerBucket) error {
	return nil
REDACTED

func (s *schedulerCacheRecorder) ListBuckets(ctx context.Context) ([]service.SchedulerBucket, error) {
	return nil, nil
REDACTED

func (s *schedulerCacheRecorder) GetOutboxWatermark(ctx context.Context) (int64, error) {
	return 0, nil
REDACTED

func (s *schedulerCacheRecorder) SetOutboxWatermark(ctx context.Context, id int64) error {
	return nil
REDACTED

func (s *AccountRepoSuite) SetupTest() {
	s.ctx = context.Background()
	tx := testEntTx(s.T())
	s.client = tx.Client()
	s.repo = newAccountRepositoryWithSQL(s.client, tx, nil)
REDACTED

func TestAccountRepoSuite(t *testing.T) {
	suite.Run(t, new(AccountRepoSuite))
REDACTED

// --- Create / GetByID / Update / Delete ---

func (s *AccountRepoSuite) TestCreate() {
	account := &service.Account{
		Name:        "test-create",
		Platform:    service.PlatformAnthropic,
		Type:        service.AccountTypeOAuth,
		Status:      service.StatusActive,
REDACTEDREDACTED,
		Extra:       map[string]any{REDACTED,
		Concurrency: 3,
		Priority:    50,
		Schedulable: true,
REDACTED

	err := s.repo.Create(s.ctx, account)
	s.Require().NoError(err, "Create")
	s.Require().NotZero(account.ID, "expected ID to be set")

	got, err := s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err, "GetByID")
	s.Require().Equal("test-create", got.Name)
REDACTED

func (s *AccountRepoSuite) TestGetByID_NotFound() {
	_, err := s.repo.GetByID(s.ctx, 999999)
	s.Require().Error(err, "expected error for non-existent ID")
REDACTED

func (s *AccountRepoSuite) TestUpdate() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "original"REDACTED)

	account.Name = "updated"
	err := s.repo.Update(s.ctx, account)
	s.Require().NoError(err, "Update")

	got, err := s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err, "GetByID after update")
	s.Require().Equal("updated", got.Name)
REDACTED

func (s *AccountRepoSuite) TestUpdate_SyncSchedulerSnapshotOnDisabled() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "sync-update", Status: service.StatusActive, Schedulable: trueREDACTED)
	cacheRecorder := &schedulerCacheRecorder{REDACTED
	s.repo.schedulerCache = cacheRecorder

	account.Status = service.StatusDisabled
	err := s.repo.Update(s.ctx, account)
	s.Require().NoError(err, "Update")

	s.Require().Len(cacheRecorder.setAccounts, 1)
	s.Require().Equal(account.ID, cacheRecorder.setAccounts[0].ID)
	s.Require().Equal(service.StatusDisabled, cacheRecorder.setAccounts[0].Status)
REDACTED

func (s *AccountRepoSuite) TestUpdate_SyncSchedulerSnapshotOnCredentialsChange() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{
		Name:        "sync-credentials-update",
		Status:      service.StatusActive,
		Schedulable: true,
REDACTED
			"model_mapping": map[string]any{
				"gpt-5": "gpt-5.1",
		REDACTED,
	REDACTED,
REDACTED)
	cacheRecorder := &schedulerCacheRecorder{REDACTED
	s.repo.schedulerCache = cacheRecorder

	account.Credentials = map[string]any{
		"model_mapping": map[string]any{
			"gpt-5": "gpt-5.2",
	REDACTED,
REDACTED
	err := s.repo.Update(s.ctx, account)
	s.Require().NoError(err, "Update")

	s.Require().Len(cacheRecorder.setAccounts, 1)
	s.Require().Equal(account.ID, cacheRecorder.setAccounts[0].ID)
	mapping, ok := cacheRecorder.setAccounts[0].Credentials["model_mapping"].(map[string]any)
	s.Require().True(ok)
	s.Require().Equal("gpt-5.2", mapping["gpt-5"])
REDACTED

func (s *AccountRepoSuite) TestUpdateCredentials_SyncsSnapshotAndDurableOutbox() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{
		Name:        "sync-refresh-credentials",
		Status:      service.StatusActive,
		Schedulable: true,
REDACTED"access_token": "old-token"REDACTED,
REDACTED)
	cacheRecorder := &schedulerCacheRecorder{REDACTED
	s.repo.schedulerCache = cacheRecorder
	_, err := s.repo.sql.ExecContext(s.ctx, "TRUNCATE scheduler_outbox")
	s.Require().NoError(err)

	s.Require().NoError(s.repo.UpdateCredentials(s.ctx, account.ID, map[string]any{"access_token": "new-token"REDACTED))

	s.Require().Len(cacheRecorder.setAccounts, 1)
	s.Require().Equal("new-token", cacheRecorder.setAccounts[0].GetCredential("access_token"))
	var outboxCount int
	err = scanSingleRow(
		s.ctx,
		s.repo.sql,
		"SELECT COUNT(*) FROM scheduler_outbox WHERE event_type = $1 AND account_id = $2",
		[]any{service.SchedulerOutboxEventAccountChanged, account.IDREDACTED,
		&outboxCount,
	)
	s.Require().NoError(err)
	s.Require().Equal(1, outboxCount)
REDACTED

func (s *AccountRepoSuite) TestDelete() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "to-delete"REDACTED)

	err := s.repo.Delete(s.ctx, account.ID)
	s.Require().NoError(err, "Delete")

	_, err = s.repo.GetByID(s.ctx, account.ID)
	s.Require().Error(err, "expected error after delete")
REDACTED

func (s *AccountRepoSuite) TestDelete_RemovesSchedulerAccountSnapshot() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "to-delete-cache"REDACTED)
	cacheRecorder := &schedulerCacheRecorder{
		accounts: map[int64]*service.Account{
			account.ID: {
				ID:          account.ID,
				Name:        account.Name,
				Status:      service.StatusActive,
				Schedulable: true,
		REDACTED,
	REDACTED,
REDACTED
	s.repo.schedulerCache = cacheRecorder

	err := s.repo.Delete(s.ctx, account.ID)
	s.Require().NoError(err, "Delete")

	s.Require().Equal([]int64{account.IDREDACTED, cacheRecorder.deleteIDs)
	s.Require().NotContains(cacheRecorder.accounts, account.ID)
REDACTED

func (s *AccountRepoSuite) TestDelete_WithGroupBindings() {
	group := mustCreateGroup(s.T(), s.client, &service.Group{Name: "g-del"REDACTED)
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-del"REDACTED)
	mustBindAccountToGroup(s.T(), s.client, account.ID, group.ID, 1)

	err := s.repo.Delete(s.ctx, account.ID)
	s.Require().NoError(err, "Delete should cascade remove bindings")

	count, err := s.client.AccountGroup.Query().Where(accountgroup.AccountIDEQ(account.ID)).Count(s.ctx)
	s.Require().NoError(err)
	s.Require().Zero(count, "expected bindings to be removed")
REDACTED

// --- List / ListWithFilters ---

func (s *AccountRepoSuite) TestList() {
	mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc1"REDACTED)
	mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc2"REDACTED)

	accounts, page, err := s.repo.List(s.ctx, pagination.PaginationParams{Page: 1, PageSize: 10REDACTED)
	s.Require().NoError(err, "List")
	s.Require().Len(accounts, 2)
	s.Require().Equal(int64(2), page.Total)
REDACTED

func (s *AccountRepoSuite) TestListOAuthRefreshCandidatePage_GrokCursorAndExclusions() {
	now := time.Now().UTC()
	valid1 := mustCreateAccount(s.T(), s.client, &service.Account{
		Name:     "grok-oauth-page-1",
		Platform: service.PlatformGrok,
		Type:     service.AccountTypeOAuth,
		Status:   service.StatusActive,
REDACTED
			"access_token":  "access-1",
			"refresh_token": "refresh-1",
			"expires_at":    now.Add(30 * time.Minute).Format(time.RFC3339),
	REDACTED,
REDACTED)
	unschedulable := mustCreateAccount(s.T(), s.client, &service.Account{
		Name:        "grok-oauth-unschedulable-excluded",
		Platform:    service.PlatformGrok,
		Type:        service.AccountTypeOAuth,
		Status:      service.StatusActive,
REDACTED"refresh_token": "refresh-unschedulable"REDACTED,
REDACTED)
	s.Require().NoError(s.client.Account.UpdateOneID(unschedulable.ID).SetSchedulable(false).Exec(s.ctx))
	mustCreateAccount(s.T(), s.client, &service.Account{
		Name:     "grok-api-key-excluded",
		Platform: service.PlatformGrok,
		Type:     service.AccountTypeAPIKey,
		Status:   service.StatusActive,
REDACTED
			"api_key":       "api-key",
			"refresh_token": "must-not-make-api-key-eligible",
	REDACTED,
REDACTED)
	valid2 := mustCreateAccount(s.T(), s.client, &service.Account{
		Name:        "grok-oauth-page-2",
		Platform:    service.PlatformGrok,
		Type:        service.AccountTypeOAuth,
		Status:      service.StatusActive,
REDACTED"refresh_token": "refresh-2"REDACTED,
REDACTED)
	mustCreateAccount(s.T(), s.client, &service.Account{
		Name:        "grok-oauth-blank-refresh-excluded",
		Platform:    service.PlatformGrok,
		Type:        service.AccountTypeOAuth,
		Status:      service.StatusActive,
REDACTED"refresh_token": "   "REDACTED,
REDACTED)
	valid3 := mustCreateAccount(s.T(), s.client, &service.Account{
		Name:        "grok-oauth-page-3",
		Platform:    service.PlatformGrok,
		Type:        service.AccountTypeOAuth,
		Status:      service.StatusActive,
REDACTED"refresh_token": "refresh-3"REDACTED,
REDACTED)
	cooldown := mustCreateAccount(s.T(), s.client, &service.Account{
		Name:        "grok-oauth-retry-cooldown-excluded",
		Platform:    service.PlatformGrok,
		Type:        service.AccountTypeOAuth,
		Status:      service.StatusActive,
REDACTED"refresh_token": "refresh-cooldown"REDACTED,
REDACTED)
	s.Require().NoError(s.repo.SetTempUnschedulable(s.ctx, cooldown.ID, now.Add(10*time.Minute), "token refresh retry exhausted: timeout"))
	mustCreateAccount(s.T(), s.client, &service.Account{
		Name:        "openai-oauth-excluded",
		Platform:    service.PlatformOpenAI,
		Type:        service.AccountTypeOAuth,
		Status:      service.StatusActive,
REDACTED"refresh_token": "refresh-openai"REDACTED,
REDACTED)

	options := service.OAuthRefreshPageOptions{
		Platforms:            []string{service.PlatformGrokREDACTED,
		Limit:                2,
		ActiveOnly:           true,
		RequireRefreshToken:  true,
		ExcludeRetryCooldown: true,
REDACTED
	firstPage, err := s.repo.ListOAuthRefreshCandidatePage(s.ctx, options)
	s.Require().NoError(err)
	first := firstPage.Accounts
	s.Require().Len(first, 2)
	s.Require().Equal([]int64{valid1.ID, valid2.IDREDACTED, []int64{first[0].ID, first[1].IDREDACTED)
	s.Require().NotContains([]int64{first[0].ID, first[1].IDREDACTED, unschedulable.ID)

	options.AfterID = first[len(first)-1].ID
	secondPage, err := s.repo.ListOAuthRefreshCandidatePage(s.ctx, options)
	s.Require().NoError(err)
	second := secondPage.Accounts
	s.Require().Len(second, 1)
	s.Require().Equal(valid3.ID, second[0].ID)
	s.Require().NotContains([]int64{first[0].ID, first[1].IDREDACTED, second[0].ID)
REDACTED

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
REDACTED{
		{
			name: "filter_by_platform",
			setup: func(client *dbent.Client) {
				mustCreateAccount(s.T(), client, &service.Account{Name: "a1", Platform: service.PlatformAnthropicREDACTED)
				mustCreateAccount(s.T(), client, &service.Account{Name: "a2", Platform: service.PlatformOpenAIREDACTED)
		REDACTED,
			platform:  service.PlatformOpenAI,
			wantCount: 1,
			validate: func(accounts []service.Account) {
				s.Require().Equal(service.PlatformOpenAI, accounts[0].Platform)
		REDACTED,
	REDACTED,
		{
			name: "filter_by_type",
			setup: func(client *dbent.Client) {
				mustCreateAccount(s.T(), client, &service.Account{Name: "t1", Type: service.AccountTypeOAuthREDACTED)
				mustCreateAccount(s.T(), client, &service.Account{Name: "t2", Type: service.AccountTypeAPIKeyREDACTED)
		REDACTED,
			accType:   service.AccountTypeAPIKey,
			wantCount: 1,
			validate: func(accounts []service.Account) {
				s.Require().Equal(service.AccountTypeAPIKey, accounts[0].Type)
		REDACTED,
	REDACTED,
		{
			name: "filter_by_status",
			setup: func(client *dbent.Client) {
				mustCreateAccount(s.T(), client, &service.Account{Name: "s1", Status: service.StatusActiveREDACTED)
				mustCreateAccount(s.T(), client, &service.Account{Name: "s2", Status: service.StatusDisabledREDACTED)
		REDACTED,
			status:    service.StatusDisabled,
			wantCount: 1,
			validate: func(accounts []service.Account) {
				s.Require().Equal(service.StatusDisabled, accounts[0].Status)
		REDACTED,
	REDACTED,
		{
			name: "filter_by_status_active_excludes_runtime_blocked_accounts",
			setup: func(client *dbent.Client) {
				mustCreateAccount(s.T(), client, &service.Account{Name: "active-normal", Status: service.StatusActiveREDACTED)
				rateLimited := mustCreateAccount(s.T(), client, &service.Account{Name: "active-rate-limited", Status: service.StatusActiveREDACTED)
				err := client.Account.UpdateOneID(rateLimited.ID).
					SetRateLimitResetAt(time.Now().Add(10 * time.Minute)).
					Exec(context.Background())
				s.Require().NoError(err)
				tempUnsched := mustCreateAccount(s.T(), client, &service.Account{Name: "active-temp-unsched", Status: service.StatusActiveREDACTED)
				err = client.Account.UpdateOneID(tempUnsched.ID).
					SetTempUnschedulableUntil(time.Now().Add(15 * time.Minute)).
					Exec(context.Background())
				s.Require().NoError(err)
				unsched := mustCreateAccount(s.T(), client, &service.Account{Name: "active-unsched", Status: service.StatusActiveREDACTED)
				err = client.Account.UpdateOneID(unsched.ID).
					SetSchedulable(false).
					Exec(context.Background())
				s.Require().NoError(err)
		REDACTED,
			status:    service.StatusActive,
			wantCount: 1,
			validate: func(accounts []service.Account) {
				s.Require().Equal("active-normal", accounts[0].Name)
		REDACTED,
	REDACTED,
		{
			name: "filter_by_status_unschedulable_excludes_rate_limited_and_temp_unschedulable",
			setup: func(client *dbent.Client) {
				mustCreateAccount(s.T(), client, &service.Account{Name: "active-normal", Status: service.StatusActive, Schedulable: trueREDACTED)
				unsched := mustCreateAccount(s.T(), client, &service.Account{Name: "active-unsched", Status: service.StatusActiveREDACTED)
				err := client.Account.UpdateOneID(unsched.ID).
					SetSchedulable(false).
					Exec(context.Background())
				s.Require().NoError(err)
				rateLimited := mustCreateAccount(s.T(), client, &service.Account{Name: "active-rate-limited", Status: service.StatusActiveREDACTED)
				err = client.Account.UpdateOneID(rateLimited.ID).
					SetSchedulable(false).
					SetRateLimitResetAt(time.Now().Add(10 * time.Minute)).
					Exec(context.Background())
				s.Require().NoError(err)
				tempUnsched := mustCreateAccount(s.T(), client, &service.Account{Name: "active-temp-unsched", Status: service.StatusActiveREDACTED)
				err = client.Account.UpdateOneID(tempUnsched.ID).
					SetSchedulable(false).
					SetTempUnschedulableUntil(time.Now().Add(15 * time.Minute)).
					Exec(context.Background())
				s.Require().NoError(err)
		REDACTED,
			status:    "unschedulable",
			wantCount: 1,
			validate: func(accounts []service.Account) {
				s.Require().Equal("active-unsched", accounts[0].Name)
		REDACTED,
	REDACTED,
		{
			name: "filter_by_status_rate_limited_excludes_temp_unschedulable",
			setup: func(client *dbent.Client) {
				rateLimited := mustCreateAccount(s.T(), client, &service.Account{Name: "active-rate-limited", Status: service.StatusActiveREDACTED)
				err := client.Account.UpdateOneID(rateLimited.ID).
					SetRateLimitResetAt(time.Now().Add(10 * time.Minute)).
					Exec(context.Background())
				s.Require().NoError(err)
				tempUnsched := mustCreateAccount(s.T(), client, &service.Account{Name: "active-temp-unsched", Status: service.StatusActiveREDACTED)
				err = client.Account.UpdateOneID(tempUnsched.ID).
					SetRateLimitResetAt(time.Now().Add(20 * time.Minute)).
					SetTempUnschedulableUntil(time.Now().Add(15 * time.Minute)).
					Exec(context.Background())
				s.Require().NoError(err)
		REDACTED,
			status:    "rate_limited",
			wantCount: 1,
			validate: func(accounts []service.Account) {
				s.Require().Equal("active-rate-limited", accounts[0].Name)
		REDACTED,
	REDACTED,
		{
			name: "filter_by_status_temp_unschedulable_excludes_manually_unschedulable",
			setup: func(client *dbent.Client) {
				tempUnsched := mustCreateAccount(s.T(), client, &service.Account{Name: "active-temp-unsched", Status: service.StatusActive, Schedulable: trueREDACTED)
				err := client.Account.UpdateOneID(tempUnsched.ID).
					SetTempUnschedulableUntil(time.Now().Add(15 * time.Minute)).
					Exec(context.Background())
				s.Require().NoError(err)
				unsched := mustCreateAccount(s.T(), client, &service.Account{Name: "active-unsched", Status: service.StatusActiveREDACTED)
				err = client.Account.UpdateOneID(unsched.ID).
					SetSchedulable(false).
					Exec(context.Background())
				s.Require().NoError(err)
		REDACTED,
			status:    "temp_unschedulable",
			wantCount: 1,
			validate: func(accounts []service.Account) {
				s.Require().Equal("active-temp-unsched", accounts[0].Name)
		REDACTED,
	REDACTED,
		{
			name: "filter_by_search",
			setup: func(client *dbent.Client) {
				mustCreateAccount(s.T(), client, &service.Account{Name: "alpha-account"REDACTED)
				mustCreateAccount(s.T(), client, &service.Account{Name: "beta-account"REDACTED)
		REDACTED,
			search:    "alpha",
			wantCount: 1,
			validate: func(accounts []service.Account) {
				s.Require().Contains(accounts[0].Name, "alpha")
		REDACTED,
	REDACTED,
		{
			name: "filter_by_ungrouped",
			setup: func(client *dbent.Client) {
				group := mustCreateGroup(s.T(), client, &service.Group{Name: "g-ungrouped"REDACTED)
				grouped := mustCreateAccount(s.T(), client, &service.Account{Name: "grouped-account"REDACTED)
				mustCreateAccount(s.T(), client, &service.Account{Name: "ungrouped-account"REDACTED)
				mustBindAccountToGroup(s.T(), client, grouped.ID, group.ID, 1)
		REDACTED,
			groupID:   service.AccountListGroupUngrouped,
			wantCount: 1,
			validate: func(accounts []service.Account) {
				s.Require().Equal("ungrouped-account", accounts[0].Name)
				s.Require().Empty(accounts[0].GroupIDs)
		REDACTED,
	REDACTED,
		{
			name: "filter_by_privacy_mode",
			setup: func(client *dbent.Client) {
				mustCreateAccount(s.T(), client, &service.Account{Name: "privacy-ok", Extra: map[string]any{"privacy_mode": service.PrivacyModeTrainingOffREDACTEDREDACTED)
				mustCreateAccount(s.T(), client, &service.Account{Name: "privacy-fail", Extra: map[string]any{"privacy_mode": service.PrivacyModeFailedREDACTEDREDACTED)
		REDACTED,
			privacyMode: service.PrivacyModeTrainingOff,
			wantCount:   1,
			validate: func(accounts []service.Account) {
				s.Require().Equal("privacy-ok", accounts[0].Name)
		REDACTED,
	REDACTED,
		{
			name: "filter_by_privacy_mode_unset",
			setup: func(client *dbent.Client) {
				mustCreateAccount(s.T(), client, &service.Account{Name: "privacy-unset", Extra: nilREDACTED)
				mustCreateAccount(s.T(), client, &service.Account{Name: "privacy-empty", Extra: map[string]any{"privacy_mode": ""REDACTEDREDACTED)
				mustCreateAccount(s.T(), client, &service.Account{Name: "privacy-set", Extra: map[string]any{"privacy_mode": service.PrivacyModeTrainingOffREDACTEDREDACTED)
		REDACTED,
			privacyMode: service.AccountPrivacyModeUnsetFilter,
			wantCount:   2,
			validate: func(accounts []service.Account) {
				names := []string{accounts[0].Name, accounts[1].NameREDACTED
				s.ElementsMatch([]string{"privacy-unset", "privacy-empty"REDACTED, names)
		REDACTED,
	REDACTED,
REDACTED

	for _, tt := range tests {
		s.Run(tt.name, func() {
			// 每个 case 重新获取隔离资源
			tx := testEntTx(s.T())
			client := tx.Client()
			repo := newAccountRepositoryWithSQL(client, tx, nil)
			ctx := context.Background()

			tt.setup(client)

			accounts, page, err := repo.ListWithFilters(ctx, pagination.PaginationParams{Page: 1, PageSize: 10REDACTED, tt.platform, tt.accType, tt.status, tt.search, tt.groupID, tt.privacyMode)
			s.Require().NoError(err)
			s.Require().Len(accounts, tt.wantCount)
			// Regression guard for issue #3601: when the whole result set fits on a single page,
			// pagination.Total must match len(items). A mismatch means the Count query was applied
			// against different predicates than the list query — the exact symptom reported.
			s.Require().NotNil(page)
			s.Require().Equal(int64(tt.wantCount), page.Total, "total must match items on single page")
			if tt.validate != nil {
				tt.validate(accounts)
		REDACTED
	REDACTED)
REDACTED
REDACTED

// --- ListByGroup / ListActive / ListByPlatform ---

func (s *AccountRepoSuite) TestListByGroup() {
	group := mustCreateGroup(s.T(), s.client, &service.Group{Name: "g-list"REDACTED)
	acc1 := mustCreateAccount(s.T(), s.client, &service.Account{Name: "a1", Status: service.StatusActiveREDACTED)
	acc2 := mustCreateAccount(s.T(), s.client, &service.Account{Name: "a2", Status: service.StatusActiveREDACTED)
	mustBindAccountToGroup(s.T(), s.client, acc1.ID, group.ID, 2)
	mustBindAccountToGroup(s.T(), s.client, acc2.ID, group.ID, 1)

	accounts, err := s.repo.ListByGroup(s.ctx, group.ID)
	s.Require().NoError(err, "ListByGroup")
	s.Require().Len(accounts, 2)
	// Should be ordered by priority
	s.Require().Equal(acc2.ID, accounts[0].ID, "expected acc2 first (priority=1)")
REDACTED

func (s *AccountRepoSuite) TestListActive() {
	mustCreateAccount(s.T(), s.client, &service.Account{Name: "active1", Status: service.StatusActiveREDACTED)
	mustCreateAccount(s.T(), s.client, &service.Account{Name: "inactive1", Status: service.StatusDisabledREDACTED)

	accounts, err := s.repo.ListActive(s.ctx)
	s.Require().NoError(err, "ListActive")
	s.Require().Len(accounts, 1)
	s.Require().Equal("active1", accounts[0].Name)
REDACTED

func (s *AccountRepoSuite) TestListByPlatform() {
	mustCreateAccount(s.T(), s.client, &service.Account{Name: "p1", Platform: service.PlatformAnthropic, Status: service.StatusActiveREDACTED)
	mustCreateAccount(s.T(), s.client, &service.Account{Name: "p2", Platform: service.PlatformOpenAI, Status: service.StatusActiveREDACTED)

	accounts, err := s.repo.ListByPlatform(s.ctx, service.PlatformAnthropic)
	s.Require().NoError(err, "ListByPlatform")
	s.Require().Len(accounts, 1)
	s.Require().Equal(service.PlatformAnthropic, accounts[0].Platform)
REDACTED

// --- Preload and VirtualFields ---

func (s *AccountRepoSuite) TestPreload_And_VirtualFields() {
	proxy := mustCreateProxy(s.T(), s.client, &service.Proxy{Name: "p1"REDACTED)
	group := mustCreateGroup(s.T(), s.client, &service.Group{Name: "g1"REDACTED)

	account := mustCreateAccount(s.T(), s.client, &service.Account{
		Name:    "acc1",
		ProxyID: &proxy.ID,
REDACTED)
	mustBindAccountToGroup(s.T(), s.client, account.ID, group.ID, 1)

	got, err := s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err, "GetByID")
	s.Require().NotNil(got.Proxy, "expected Proxy preload")
	s.Require().Equal(proxy.ID, got.Proxy.ID)
	s.Require().Len(got.GroupIDs, 1, "expected GroupIDs to be populated")
	s.Require().Equal(group.ID, got.GroupIDs[0])
	s.Require().Len(got.Groups, 1, "expected Groups to be populated")
	s.Require().Equal(group.ID, got.Groups[0].ID)

	accounts, page, err := s.repo.ListWithFilters(s.ctx, pagination.PaginationParams{Page: 1, PageSize: 10REDACTED, "", "", "", "acc", 0, "")
	s.Require().NoError(err, "ListWithFilters")
	s.Require().Equal(int64(1), page.Total)
	s.Require().Len(accounts, 1)
	s.Require().NotNil(accounts[0].Proxy, "expected Proxy preload in list")
	s.Require().Equal(proxy.ID, accounts[0].Proxy.ID)
	s.Require().Len(accounts[0].GroupIDs, 1, "expected GroupIDs in list")
	s.Require().Equal(group.ID, accounts[0].GroupIDs[0])
REDACTED

// --- GroupBinding / AddToGroup / RemoveFromGroup / BindGroups / GetGroups ---

func (s *AccountRepoSuite) TestGroupBinding_And_BindGroups() {
	g1 := mustCreateGroup(s.T(), s.client, &service.Group{Name: "g1"REDACTED)
	g2 := mustCreateGroup(s.T(), s.client, &service.Group{Name: "g2"REDACTED)
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc"REDACTED)

	s.Require().NoError(s.repo.AddToGroup(s.ctx, account.ID, g1.ID, 10), "AddToGroup")
	groups, err := s.repo.GetGroups(s.ctx, account.ID)
	s.Require().NoError(err, "GetGroups")
	s.Require().Len(groups, 1, "expected 1 group")
	s.Require().Equal(g1.ID, groups[0].ID)

	s.Require().NoError(s.repo.RemoveFromGroup(s.ctx, account.ID, g1.ID), "RemoveFromGroup")
	groups, err = s.repo.GetGroups(s.ctx, account.ID)
	s.Require().NoError(err, "GetGroups after remove")
	s.Require().Empty(groups, "expected 0 groups after remove")

	s.Require().NoError(s.repo.BindGroups(s.ctx, account.ID, []int64{g1.ID, g2.IDREDACTED), "BindGroups")
	groups, err = s.repo.GetGroups(s.ctx, account.ID)
	s.Require().NoError(err, "GetGroups after bind")
	s.Require().Len(groups, 2, "expected 2 groups after bind")
REDACTED

func (s *AccountRepoSuite) TestBindGroups_EmptyList() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-empty"REDACTED)
	group := mustCreateGroup(s.T(), s.client, &service.Group{Name: "g-empty"REDACTED)
	mustBindAccountToGroup(s.T(), s.client, account.ID, group.ID, 1)

	s.Require().NoError(s.repo.BindGroups(s.ctx, account.ID, []int64{REDACTED), "BindGroups empty")

	groups, err := s.repo.GetGroups(s.ctx, account.ID)
	s.Require().NoError(err)
	s.Require().Empty(groups, "expected 0 groups after binding empty list")
REDACTED

// --- Schedulable ---

func (s *AccountRepoSuite) TestListSchedulable() {
	now := time.Now()
	group := mustCreateGroup(s.T(), s.client, &service.Group{Name: "g-sched"REDACTED)

	okAcc := mustCreateAccount(s.T(), s.client, &service.Account{Name: "ok", Schedulable: trueREDACTED)
	mustBindAccountToGroup(s.T(), s.client, okAcc.ID, group.ID, 1)

	future := now.Add(10 * time.Minute)
	overloaded := mustCreateAccount(s.T(), s.client, &service.Account{Name: "over", Schedulable: true, OverloadUntil: &futureREDACTED)
	mustBindAccountToGroup(s.T(), s.client, overloaded.ID, group.ID, 1)

	sched, err := s.repo.ListSchedulable(s.ctx)
	s.Require().NoError(err, "ListSchedulable")
	ids := idsOfAccounts(sched)
	s.Require().Contains(ids, okAcc.ID)
	s.Require().NotContains(ids, overloaded.ID)
REDACTED

func (s *AccountRepoSuite) TestListSchedulableByGroupID_TimeBoundaries_And_StatusUpdates() {
	now := time.Now()
	group := mustCreateGroup(s.T(), s.client, &service.Group{Name: "g-sched"REDACTED)

	okAcc := mustCreateAccount(s.T(), s.client, &service.Account{Name: "ok", Schedulable: trueREDACTED)
	mustBindAccountToGroup(s.T(), s.client, okAcc.ID, group.ID, 1)

	future := now.Add(10 * time.Minute)
	overloaded := mustCreateAccount(s.T(), s.client, &service.Account{Name: "over", Schedulable: true, OverloadUntil: &futureREDACTED)
	mustBindAccountToGroup(s.T(), s.client, overloaded.ID, group.ID, 1)

	rateLimited := mustCreateAccount(s.T(), s.client, &service.Account{Name: "rl", Schedulable: trueREDACTED)
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
REDACTED

func (s *AccountRepoSuite) TestListSchedulableByPlatform() {
	mustCreateAccount(s.T(), s.client, &service.Account{Name: "a1", Platform: service.PlatformAnthropic, Schedulable: trueREDACTED)
	mustCreateAccount(s.T(), s.client, &service.Account{Name: "a2", Platform: service.PlatformOpenAI, Schedulable: trueREDACTED)

	accounts, err := s.repo.ListSchedulableByPlatform(s.ctx, service.PlatformAnthropic)
	s.Require().NoError(err)
	s.Require().Len(accounts, 1)
	s.Require().Equal(service.PlatformAnthropic, accounts[0].Platform)
REDACTED

func (s *AccountRepoSuite) TestListSchedulableByGroupIDAndPlatform() {
	group := mustCreateGroup(s.T(), s.client, &service.Group{Name: "g-sp"REDACTED)
	a1 := mustCreateAccount(s.T(), s.client, &service.Account{Name: "a1", Platform: service.PlatformAnthropic, Schedulable: trueREDACTED)
	a2 := mustCreateAccount(s.T(), s.client, &service.Account{Name: "a2", Platform: service.PlatformOpenAI, Schedulable: trueREDACTED)
	mustBindAccountToGroup(s.T(), s.client, a1.ID, group.ID, 1)
	mustBindAccountToGroup(s.T(), s.client, a2.ID, group.ID, 2)

	accounts, err := s.repo.ListSchedulableByGroupIDAndPlatform(s.ctx, group.ID, service.PlatformAnthropic)
	s.Require().NoError(err)
	s.Require().Len(accounts, 1)
	s.Require().Equal(a1.ID, accounts[0].ID)
REDACTED

func (s *AccountRepoSuite) TestSetSchedulable() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-sched", Schedulable: trueREDACTED)
	cacheRecorder := &schedulerCacheRecorder{REDACTED
	s.repo.schedulerCache = cacheRecorder

	s.Require().NoError(s.repo.SetSchedulable(s.ctx, account.ID, false))

	got, err := s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err)
	s.Require().False(got.Schedulable)
	s.Require().Len(cacheRecorder.setAccounts, 1)
	s.Require().Equal(account.ID, cacheRecorder.setAccounts[0].ID)
REDACTED

func (s *AccountRepoSuite) TestBulkUpdate_SyncSchedulerSnapshotOnDisabled() {
	account1 := mustCreateAccount(s.T(), s.client, &service.Account{Name: "bulk-1", Status: service.StatusActive, Schedulable: trueREDACTED)
	account2 := mustCreateAccount(s.T(), s.client, &service.Account{Name: "bulk-2", Status: service.StatusActive, Schedulable: trueREDACTED)
	cacheRecorder := &schedulerCacheRecorder{REDACTED
	s.repo.schedulerCache = cacheRecorder

	disabled := service.StatusDisabled
	rows, err := s.repo.BulkUpdate(s.ctx, []int64{account1.ID, account2.IDREDACTED, service.AccountBulkUpdate{
		Status: &disabled,
REDACTED)
	s.Require().NoError(err)
	s.Require().Equal(int64(2), rows)

	s.Require().Len(cacheRecorder.setAccounts, 2)
	ids := map[int64]struct{REDACTED{REDACTED
	for _, acc := range cacheRecorder.setAccounts {
		ids[acc.ID] = struct{REDACTED{REDACTED
REDACTED
	s.Require().Contains(ids, account1.ID)
	s.Require().Contains(ids, account2.ID)
REDACTED

// --- SetOverloaded / SetRateLimited / ClearRateLimit ---

func (s *AccountRepoSuite) TestSetOverloaded() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-over"REDACTED)
	until := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
	cacheRecorder := &schedulerCacheRecorder{REDACTED
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
REDACTED

func (s *AccountRepoSuite) TestSetRateLimited() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-rl"REDACTED)
	resetAt := time.Date(2025, 6, 15, 14, 0, 0, 0, time.UTC)

	s.Require().NoError(s.repo.SetRateLimited(s.ctx, account.ID, resetAt))

	got, err := s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err)
	s.Require().NotNil(got.RateLimitedAt)
	s.Require().NotNil(got.RateLimitResetAt)
	s.Require().WithinDuration(resetAt, *got.RateLimitResetAt, time.Second)
REDACTED

func (s *AccountRepoSuite) TestSetRateLimitedIfLaterDoesNotShortenReset() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-rl-monotonic"REDACTED)
	later := time.Now().Add(30 * time.Minute).UTC().Truncate(time.Second)
	earlier := time.Now().Add(5 * time.Minute).UTC().Truncate(time.Second)
	cacheRecorder := &schedulerCacheRecorder{REDACTED
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
REDACTED

func (s *AccountRepoSuite) TestClearRateLimitIfObservedProtectsRearmed429Generation() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{
		Name:     "acc-rl-conditional-clear",
		Platform: service.PlatformGrok,
		Type:     service.AccountTypeOAuth,
REDACTED)
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
REDACTED

func (s *AccountRepoSuite) TestClearRateLimit() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-clear"REDACTED)
	until := time.Now().Add(1 * time.Hour)
	s.Require().NoError(s.repo.SetOverloaded(s.ctx, account.ID, until))
	s.Require().NoError(s.repo.SetRateLimited(s.ctx, account.ID, until))

	s.Require().NoError(s.repo.ClearRateLimit(s.ctx, account.ID))

	got, err := s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err)
	s.Require().Nil(got.RateLimitedAt)
	s.Require().Nil(got.RateLimitResetAt)
	s.Require().Nil(got.OverloadUntil)
REDACTED

func (s *AccountRepoSuite) TestTempUnschedulableFieldsLoadedByGetByIDAndGetByIDs() {
	acc1 := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-temp-1"REDACTED)
	acc2 := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-temp-2"REDACTED)

	until := time.Now().Add(15 * time.Minute).UTC().Truncate(time.Second)
	reason := `{"rule":"429","matched_keyword":"too many requests"REDACTED`
	s.Require().NoError(s.repo.SetTempUnschedulable(s.ctx, acc1.ID, until, reason))

	gotByID, err := s.repo.GetByID(s.ctx, acc1.ID)
	s.Require().NoError(err)
	s.Require().NotNil(gotByID.TempUnschedulableUntil)
	s.Require().WithinDuration(until, *gotByID.TempUnschedulableUntil, time.Second)
	s.Require().Equal(reason, gotByID.TempUnschedulableReason)

	gotByIDs, err := s.repo.GetByIDs(s.ctx, []int64{acc2.ID, acc1.IDREDACTED)
	s.Require().NoError(err)
	s.Require().Len(gotByIDs, 2)
	s.Require().Equal(acc2.ID, gotByIDs[0].ID)
	s.Require().Nil(gotByIDs[0].TempUnschedulableUntil)
	s.Require().Equal("", gotByIDs[0].TempUnschedulableReason)
	s.Require().Equal(acc1.ID, gotByIDs[1].ID)
	s.Require().NotNil(gotByIDs[1].TempUnschedulableUntil)
	s.Require().WithinDuration(until, *gotByIDs[1].TempUnschedulableUntil, time.Second)
	s.Require().Equal(reason, gotByIDs[1].TempUnschedulableReason)

	cacheRecorder := &schedulerCacheRecorder{REDACTED
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
REDACTED

func (s *AccountRepoSuite) TestSetTempUnschedulableSkipsOutboxWhenWindowDoesNotExtend() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-temp-noop"REDACTED)
	cacheRecorder := &schedulerCacheRecorder{REDACTED
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
REDACTED

func (s *AccountRepoSuite) TestClearModelRateLimits_SyncsSchedulerSnapshot() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{
		Name: "acc-clear-model-rate",
		Extra: map[string]any{
			"model_rate_limits": map[string]any{
				"claude-sonnet-4-5": map[string]any{
					"rate_limit_reset_at": "2026-06-03T10:00:00Z",
			REDACTED,
		REDACTED,
	REDACTED,
REDACTED)
	cacheRecorder := &schedulerCacheRecorder{REDACTED
	s.repo.schedulerCache = cacheRecorder

	s.Require().NoError(s.repo.ClearModelRateLimits(s.ctx, account.ID))

	got, err := s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err)
	s.Require().NotContains(got.Extra, "model_rate_limits")
	s.Require().Len(cacheRecorder.setAccounts, 1)
	s.Require().Equal(account.ID, cacheRecorder.setAccounts[0].ID)
	s.Require().NotContains(cacheRecorder.setAccounts[0].Extra, "model_rate_limits")
REDACTED

// --- UpdateLastUsed ---

func (s *AccountRepoSuite) TestUpdateLastUsed() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-used"REDACTED)
	s.Require().Nil(account.LastUsedAt)

	s.Require().NoError(s.repo.UpdateLastUsed(s.ctx, account.ID))

	got, err := s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err)
	s.Require().NotNil(got.LastUsedAt)
REDACTED

// --- SetError ---

func (s *AccountRepoSuite) TestSetError() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-err", Status: service.StatusActive, Schedulable: trueREDACTED)
	cacheRecorder := &schedulerCacheRecorder{REDACTED
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
		[]any{service.SchedulerOutboxEventAccountChanged, account.IDREDACTED,
		&outboxCount,
	)
	s.Require().NoError(err)
	s.Require().Equal(1, outboxCount)
REDACTED

func (s *AccountRepoSuite) TestSetGrokOAuthErrorIfCredentialsUnchanged_AppliesAndSyncsSchedulerState() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{
		Name:        "grok-conditional-error-applied",
		Platform:    service.PlatformGrok,
		Type:        service.AccountTypeOAuth,
		Status:      service.StatusActive,
		Schedulable: true,
REDACTED"access_token": "observed", "_token_version": int64(7)REDACTED,
REDACTED)
	observed, err := s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err)
	cacheRecorder := &schedulerCacheRecorder{REDACTED
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
		[]any{service.SchedulerOutboxEventAccountChanged, account.IDREDACTED,
		&outboxCount,
	)
	s.Require().NoError(err)
	s.Require().Equal(1, outboxCount)
REDACTED

func (s *AccountRepoSuite) TestSetGrokOAuthErrorIfCredentialsUnchanged_SkipsConcurrentReauthorization() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{
		Name:        "grok-conditional-error-reauthorized",
		Platform:    service.PlatformGrok,
		Type:        service.AccountTypeOAuth,
		Status:      service.StatusActive,
		Schedulable: true,
REDACTED"access_token": "observed", "_token_version": int64(7)REDACTED,
REDACTED)
	observed, err := s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err)
	s.Require().NoError(s.repo.UpdateCredentials(s.ctx, account.ID, map[string]any{
		"access_token":   "fresh-access",
		"refresh_token":  "fresh-refresh",
		"expires_at":     time.Now().UTC().Add(4 * time.Hour).Format(time.RFC3339),
		"_token_version": int64(8),
REDACTED))
	cacheRecorder := &schedulerCacheRecorder{REDACTED
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
		[]any{service.SchedulerOutboxEventAccountChanged, account.IDREDACTED,
		&outboxCount,
	)
	s.Require().NoError(err)
	s.Require().Zero(outboxCount, "a lost compare-and-set race must not enqueue a stale account change")
REDACTED

func (s *AccountRepoSuite) TestUpdateGrokOAuthCredentialsIfUnchanged_AppliesAndPublishesSchedulerState() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{
		Name:        "grok-refresh-success-cas-applied",
		Platform:    service.PlatformGrok,
		Type:        service.AccountTypeOAuth,
		Status:      service.StatusActive,
		Schedulable: true,
REDACTED
			"access_token":   "attempted-access",
			"refresh_token":  "attempted-refresh",
			"_token_version": int64(10),
	REDACTED,
REDACTED)
	observed, err := s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err)
	cacheRecorder := &schedulerCacheRecorder{REDACTED
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
	REDACTED,
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
		[]any{service.SchedulerOutboxEventAccountChanged, account.IDREDACTED,
		&outboxCount,
	)
	s.Require().NoError(err)
	s.Require().Equal(1, outboxCount)
REDACTED

func (s *AccountRepoSuite) TestUpdateGrokOAuthCredentialsIfUnchanged_SkipsConcurrentReauthorization() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{
		Name:        "grok-refresh-success-cas-reauthorized",
		Platform:    service.PlatformGrok,
		Type:        service.AccountTypeOAuth,
		Status:      service.StatusActive,
		Schedulable: true,
REDACTED
			"access_token":   "attempted-access",
			"refresh_token":  "attempted-refresh",
			"_token_version": int64(20),
	REDACTED,
REDACTED)
	observed, err := s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err)
	s.Require().NoError(s.repo.UpdateCredentials(s.ctx, account.ID, map[string]any{
		"access_token":   "reauthorized-access",
		"refresh_token":  "reauthorized-refresh",
		"_token_version": int64(21),
REDACTED))
	cacheRecorder := &schedulerCacheRecorder{REDACTED
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
	REDACTED,
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
		[]any{service.SchedulerOutboxEventAccountChanged, account.IDREDACTED,
		&outboxCount,
	)
	s.Require().NoError(err)
	s.Require().Zero(outboxCount)
REDACTED

func (s *AccountRepoSuite) TestGrokOAuthConditionalMutation_DetachesBoundedSnapshotSync() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{
		Name:        "grok-conditional-detached-sync",
		Platform:    service.PlatformGrok,
		Type:        service.AccountTypeOAuth,
		Status:      service.StatusActive,
		Schedulable: true,
REDACTED"access_token": "observed"REDACTED,
REDACTED)
	observed, err := s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err)
	ctx, cancel := context.WithCancel(context.Background())
	cacheRecorder := &schedulerCacheRecorder{REDACTED
	repo := newAccountRepositoryWithSQL(s.client, &cancelAfterAtomicMutationSQLExecutor{
		sqlExecutor: s.repo.sql,
		cancel:      cancel,
REDACTED, cacheRecorder)

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
REDACTED

func TestGrokOAuthConditionalMutationRollsBackWhenOutboxInsertFails(t *testing.T) {
	client := testEntClient(t)
	account := mustCreateAccount(t, client, &service.Account{
		Name:        "grok-conditional-atomic-outbox-failure",
		Platform:    service.PlatformGrok,
		Type:        service.AccountTypeOAuth,
		Status:      service.StatusActive,
		Schedulable: true,
REDACTED"access_token": "observed"REDACTED,
REDACTED)
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(context.Background(), "DELETE FROM scheduler_outbox WHERE account_id = $1", account.ID)
		_ = client.Account.DeleteOneID(account.ID).Exec(context.Background())
REDACTED)
	repo := newAccountRepositoryWithSQL(client, &failAtomicSchedulerOutboxSQLExecutor{sqlExecutor: integrationDBREDACTED, nil)

	applied, err := repo.SetGrokOAuthErrorIfCredentialsUnchanged(
		context.Background(),
		account.ID,
		account.Credentials,
		"missing refresh token",
	)

REDACTED
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
REDACTED

func (s *AccountRepoSuite) TestUpdateErrorStatusUnschedulesAccount() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-update-err", Status: service.StatusActive, Schedulable: trueREDACTED)
	account.Status = service.StatusError
	account.ErrorMessage = "token revoked"
	account.Schedulable = true

	s.Require().NoError(s.repo.Update(s.ctx, account))

	got, err := s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err)
	s.Require().Equal(service.StatusError, got.Status)
	s.Require().Equal("token revoked", got.ErrorMessage)
	s.Require().False(got.Schedulable)
REDACTED

func (s *AccountRepoSuite) TestClearError_SyncSchedulerSnapshotOnRecovery() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{
		Name:         "acc-clear-err",
		Status:       service.StatusError,
		ErrorMessage: "temporary error",
REDACTED)
	cacheRecorder := &schedulerCacheRecorder{REDACTED
	s.repo.schedulerCache = cacheRecorder

	s.Require().NoError(s.repo.ClearError(s.ctx, account.ID))

	got, err := s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err)
	s.Require().Equal(service.StatusActive, got.Status)
	s.Require().Empty(got.ErrorMessage)
	s.Require().Len(cacheRecorder.setAccounts, 1)
	s.Require().Equal(account.ID, cacheRecorder.setAccounts[0].ID)
	s.Require().Equal(service.StatusActive, cacheRecorder.setAccounts[0].Status)
REDACTED

// --- UpdateSessionWindow ---

func (s *AccountRepoSuite) TestUpdateSessionWindow() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-win"REDACTED)
	start := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)
	end := time.Date(2025, 6, 15, 15, 0, 0, 0, time.UTC)

	s.Require().NoError(s.repo.UpdateSessionWindow(s.ctx, account.ID, &start, &end, "active"))

	got, err := s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err)
	s.Require().NotNil(got.SessionWindowStart)
	s.Require().NotNil(got.SessionWindowEnd)
	s.Require().Equal("active", got.SessionWindowStatus)
REDACTED

// --- UpdateExtra ---

func (s *AccountRepoSuite) TestUpdateExtra_MergesFields() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{
		Name:  "acc-extra",
		Extra: map[string]any{"a": "1"REDACTED,
REDACTED)
	s.Require().NoError(s.repo.UpdateExtra(s.ctx, account.ID, map[string]any{"b": "2"REDACTED), "UpdateExtra")

	got, err := s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err, "GetByID")
	s.Require().Equal("1", got.Extra["a"])
	s.Require().Equal("2", got.Extra["b"])
REDACTED

func (s *AccountRepoSuite) TestUpdateExtra_EmptyUpdates() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-extra-empty"REDACTED)
	s.Require().NoError(s.repo.UpdateExtra(s.ctx, account.ID, map[string]any{REDACTED))
REDACTED

func (s *AccountRepoSuite) TestUpdateExtra_NilExtra() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-nil-extra", Extra: nilREDACTED)
	s.Require().NoError(s.repo.UpdateExtra(s.ctx, account.ID, map[string]any{"key": "val"REDACTED))

	got, err := s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err)
	s.Require().Equal("val", got.Extra["key"])
REDACTED

func (s *AccountRepoSuite) TestUpdateExtra_SchedulerNeutralSkipsOutboxAndSyncsFreshSnapshot() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{
		Name:     "acc-extra-neutral",
		Platform: service.PlatformOpenAI,
		Extra:    map[string]any{"codex_usage_updated_at": "old"REDACTED,
REDACTED)
	cacheRecorder := &schedulerCacheRecorder{
		accounts: map[int64]*service.Account{
			account.ID: {
				ID:       account.ID,
				Platform: account.Platform,
				Status:   service.StatusDisabled,
				Extra: map[string]any{
					"codex_usage_updated_at": "old",
			REDACTED,
		REDACTED,
	REDACTED,
REDACTED
	s.repo.schedulerCache = cacheRecorder

	updates := map[string]any{
		"codex_usage_updated_at":     "2026-03-11T10:00:00Z",
		"codex_5h_used_percent":      88.5,
		"session_window_utilization": 0.42,
REDACTED
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
REDACTED

func (s *AccountRepoSuite) TestUpdateExtra_ExhaustedCodexSnapshotSyncsSchedulerCache() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{
		Name:     "acc-extra-codex-exhausted",
		Platform: service.PlatformOpenAI,
		Type:     service.AccountTypeOAuth,
		Extra:    map[string]any{REDACTED,
REDACTED)
	cacheRecorder := &schedulerCacheRecorder{REDACTED
	s.repo.schedulerCache = cacheRecorder
	_, err := s.repo.sql.ExecContext(s.ctx, "TRUNCATE scheduler_outbox")
	s.Require().NoError(err)

	s.Require().NoError(s.repo.UpdateExtra(s.ctx, account.ID, map[string]any{
		"codex_7d_used_percent":        100.0,
		"codex_7d_reset_at":            "2026-03-12T13:00:00Z",
		"codex_7d_reset_after_seconds": 86400,
REDACTED))

	var count int
	err = scanSingleRow(s.ctx, s.repo.sql, "SELECT COUNT(*) FROM scheduler_outbox", nil, &count)
	s.Require().NoError(err)
	s.Require().Equal(0, count)
	s.Require().Len(cacheRecorder.setAccounts, 1)
	s.Require().Equal(account.ID, cacheRecorder.setAccounts[0].ID)
	s.Require().Equal(service.StatusActive, cacheRecorder.setAccounts[0].Status)
	s.Require().Equal(100.0, cacheRecorder.setAccounts[0].Extra["codex_7d_used_percent"])
REDACTED

func (s *AccountRepoSuite) TestUpdateExtra_SchedulerRelevantStillEnqueuesOutbox() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{
		Name:     "acc-extra-mixed",
		Platform: service.PlatformAntigravity,
		Extra:    map[string]any{REDACTED,
REDACTED)
	_, err := s.repo.sql.ExecContext(s.ctx, "TRUNCATE scheduler_outbox")
	s.Require().NoError(err)

	s.Require().NoError(s.repo.UpdateExtra(s.ctx, account.ID, map[string]any{
		"mixed_scheduling":       true,
		"codex_usage_updated_at": "2026-03-11T10:00:00Z",
REDACTED))

	var count int
	err = scanSingleRow(s.ctx, s.repo.sql, "SELECT COUNT(*) FROM scheduler_outbox", nil, &count)
	s.Require().NoError(err)
	s.Require().Equal(1, count)
REDACTED

// --- GetByCRSAccountID ---

func (s *AccountRepoSuite) TestGetByCRSAccountID() {
	crsID := "crs-12345"
	mustCreateAccount(s.T(), s.client, &service.Account{
		Name:  "acc-crs",
		Extra: map[string]any{"crs_account_id": crsIDREDACTED,
REDACTED)

	got, err := s.repo.GetByCRSAccountID(s.ctx, crsID)
	s.Require().NoError(err)
	s.Require().NotNil(got)
	s.Require().Equal("acc-crs", got.Name)
REDACTED

func (s *AccountRepoSuite) TestGetByCRSAccountID_NotFound() {
	got, err := s.repo.GetByCRSAccountID(s.ctx, "non-existent")
	s.Require().NoError(err)
	s.Require().Nil(got)
REDACTED

func (s *AccountRepoSuite) TestGetByCRSAccountID_EmptyString() {
	got, err := s.repo.GetByCRSAccountID(s.ctx, "")
	s.Require().NoError(err)
	s.Require().Nil(got)
REDACTED

// TestGetByCRSAccountID_ExcludesSparkShadow 验证外审第7轮 P1:即便 spark 影子的 Extra 被误写入
// crs_account_id,CRS 查询也绝不能命中影子(否则会被当普通账号更新而覆盖 type/credentials/proxy)。
func (s *AccountRepoSuite) TestGetByCRSAccountID_ExcludesSparkShadow() {
	crsID := "crs-shadow-only-99"
	parent := mustCreateAccount(s.T(), s.client, &service.Account{
		Name: "crs-mother", Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth,
REDACTED)
	mustCreateAccount(s.T(), s.client, &service.Account{
		Name: "crs-shadow", Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth,
		ParentAccountID: &parent.ID,
		QuotaDimension:  service.QuotaDimensionSpark,
		Extra:           map[string]any{"crs_account_id": crsIDREDACTED,
REDACTED)

	got, err := s.repo.GetByCRSAccountID(s.ctx, crsID)
	s.Require().NoError(err)
	s.Require().Nil(got, "spark 影子即便带 crs_account_id 也不应被 CRS 命中")
REDACTED

// TestListCRSAccountIDs_ExcludesSparkShadow 验证外审第7轮 P1:影子的 crs_account_id 不应进入
// CRS 同步映射(否则后续 CRS 同步会把影子当普通账号更新)。
func (s *AccountRepoSuite) TestListCRSAccountIDs_ExcludesSparkShadow() {
	parent := mustCreateAccount(s.T(), s.client, &service.Account{
		Name: "crs-list-mother", Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth,
REDACTED)
	shadowCRSID := "crs-list-shadow-77"
	mustCreateAccount(s.T(), s.client, &service.Account{
		Name: "crs-list-shadow", Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth,
		ParentAccountID: &parent.ID,
		QuotaDimension:  service.QuotaDimensionSpark,
		Extra:           map[string]any{"crs_account_id": shadowCRSIDREDACTED,
REDACTED)

	ids, err := s.repo.ListCRSAccountIDs(s.ctx)
	s.Require().NoError(err)
	_, ok := ids[shadowCRSID]
	s.Require().False(ok, "影子的 crs_account_id 不应进入 CRS 映射")
REDACTED

// --- BulkUpdate ---

func (s *AccountRepoSuite) TestBulkUpdate() {
	a1 := mustCreateAccount(s.T(), s.client, &service.Account{Name: "bulk1", Priority: 1REDACTED)
	a2 := mustCreateAccount(s.T(), s.client, &service.Account{Name: "bulk2", Priority: 1REDACTED)

	newPriority := 99
	affected, err := s.repo.BulkUpdate(s.ctx, []int64{a1.ID, a2.IDREDACTED, service.AccountBulkUpdate{
		Priority: &newPriority,
REDACTED)
	s.Require().NoError(err)
	s.Require().GreaterOrEqual(affected, int64(1), "expected at least one affected row")

	got1, _ := s.repo.GetByID(s.ctx, a1.ID)
	got2, _ := s.repo.GetByID(s.ctx, a2.ID)
	s.Require().Equal(99, got1.Priority)
	s.Require().Equal(99, got2.Priority)
REDACTED

func (s *AccountRepoSuite) TestBulkUpdate_MergeCredentials() {
	a1 := mustCreateAccount(s.T(), s.client, &service.Account{
		Name:        "bulk-cred",
REDACTED"existing": "value"REDACTED,
REDACTED)

	_, err := s.repo.BulkUpdate(s.ctx, []int64{a1.IDREDACTED, service.AccountBulkUpdate{
REDACTED"new_key": "new_value"REDACTED,
REDACTED)
	s.Require().NoError(err)

	got, _ := s.repo.GetByID(s.ctx, a1.ID)
	s.Require().Equal("value", got.Credentials["existing"])
	s.Require().Equal("new_value", got.Credentials["new_key"])
REDACTED

func (s *AccountRepoSuite) TestBulkUpdate_MergeExtra() {
	a1 := mustCreateAccount(s.T(), s.client, &service.Account{
		Name:  "bulk-extra",
		Extra: map[string]any{"existing": "val"REDACTED,
REDACTED)

	_, err := s.repo.BulkUpdate(s.ctx, []int64{a1.IDREDACTED, service.AccountBulkUpdate{
		Extra: map[string]any{"new_key": "new_val"REDACTED,
REDACTED)
	s.Require().NoError(err)

	got, _ := s.repo.GetByID(s.ctx, a1.ID)
	s.Require().Equal("val", got.Extra["existing"])
	s.Require().Equal("new_val", got.Extra["new_key"])
REDACTED

func (s *AccountRepoSuite) TestBulkUpdate_EmptyIDs() {
	affected, err := s.repo.BulkUpdate(s.ctx, []int64{REDACTED, service.AccountBulkUpdate{REDACTED)
	s.Require().NoError(err)
	s.Require().Zero(affected)
REDACTED

func (s *AccountRepoSuite) TestBulkUpdate_EmptyUpdates() {
	a1 := mustCreateAccount(s.T(), s.client, &service.Account{Name: "bulk-empty"REDACTED)

	affected, err := s.repo.BulkUpdate(s.ctx, []int64{a1.IDREDACTED, service.AccountBulkUpdate{REDACTED)
	s.Require().NoError(err)
	s.Require().Zero(affected)
REDACTED

func idsOfAccounts(accounts []service.Account) []int64 {
	out := make([]int64, 0, len(accounts))
	for i := range accounts {
		out = append(out, accounts[i].ID)
REDACTED
	return out
REDACTED
