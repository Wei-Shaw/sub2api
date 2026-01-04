//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/suite"
)

type UsageLogRepoSuite struct {
	suite.Suite
	ctx    context.Context
	tx     *dbent.Tx
	client *dbent.Client
	repo   *usageLogRepository
REDACTED

func (s *UsageLogRepoSuite) SetupTest() {
	s.ctx = context.Background()
	tx := testEntTx(s.T())
	s.tx = tx
	s.client = tx.Client()
	s.repo = newUsageLogRepositoryWithSQL(s.client, tx)
REDACTED

func TestUsageLogRepoSuite(t *testing.T) {
	suite.Run(t, new(UsageLogRepoSuite))
REDACTED

func (s *UsageLogRepoSuite) createUsageLog(user *service.User, apiKey *service.ApiKey, account *service.Account, inputTokens, outputTokens int, cost float64, createdAt time.Time) *service.UsageLog {
	log := &service.UsageLog{
		UserID:       user.ID,
		ApiKeyID:     apiKey.ID,
		AccountID:    account.ID,
		RequestID:    uuid.New().String(), // Generate unique RequestID for each log
		Model:        "claude-3",
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
		TotalCost:    cost,
		ActualCost:   cost,
		CreatedAt:    createdAt,
REDACTED
	_, err := s.repo.Create(s.ctx, log)
	s.Require().NoError(err)
	return log
REDACTED

// --- Create / GetByID ---

func (s *UsageLogRepoSuite) TestCreate() {
	user := mustCreateUser(s.T(), s.client, &service.User{Email: "create@test.com"REDACTED)
	apiKey := mustCreateApiKey(s.T(), s.client, &service.ApiKey{UserID: user.ID, Key: "sk-create", Name: "k"REDACTED)
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-create"REDACTED)

	log := &service.UsageLog{
		UserID:       user.ID,
		ApiKeyID:     apiKey.ID,
		AccountID:    account.ID,
		Model:        "claude-3",
		InputTokens:  10,
		OutputTokens: 20,
		TotalCost:    0.5,
		ActualCost:   0.4,
REDACTED

	_, err := s.repo.Create(s.ctx, log)
	s.Require().NoError(err, "Create")
	s.Require().NotZero(log.ID)
REDACTED

func (s *UsageLogRepoSuite) TestGetByID() {
	user := mustCreateUser(s.T(), s.client, &service.User{Email: "getbyid@test.com"REDACTED)
	apiKey := mustCreateApiKey(s.T(), s.client, &service.ApiKey{UserID: user.ID, Key: "sk-getbyid", Name: "k"REDACTED)
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-getbyid"REDACTED)

	log := s.createUsageLog(user, apiKey, account, 10, 20, 0.5, time.Now())

	got, err := s.repo.GetByID(s.ctx, log.ID)
	s.Require().NoError(err, "GetByID")
	s.Require().Equal(log.ID, got.ID)
	s.Require().Equal(10, got.InputTokens)
REDACTED

func (s *UsageLogRepoSuite) TestGetByID_NotFound() {
	_, err := s.repo.GetByID(s.ctx, 999999)
	s.Require().Error(err, "expected error for non-existent ID")
REDACTED

// --- Delete ---

func (s *UsageLogRepoSuite) TestDelete() {
	user := mustCreateUser(s.T(), s.client, &service.User{Email: "delete@test.com"REDACTED)
	apiKey := mustCreateApiKey(s.T(), s.client, &service.ApiKey{UserID: user.ID, Key: "sk-delete", Name: "k"REDACTED)
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-delete"REDACTED)

	log := s.createUsageLog(user, apiKey, account, 10, 20, 0.5, time.Now())

	err := s.repo.Delete(s.ctx, log.ID)
	s.Require().NoError(err, "Delete")

	_, err = s.repo.GetByID(s.ctx, log.ID)
	s.Require().Error(err, "expected error after delete")
REDACTED

// --- ListByUser ---

func (s *UsageLogRepoSuite) TestListByUser() {
	user := mustCreateUser(s.T(), s.client, &service.User{Email: "listbyuser@test.com"REDACTED)
	apiKey := mustCreateApiKey(s.T(), s.client, &service.ApiKey{UserID: user.ID, Key: "sk-listbyuser", Name: "k"REDACTED)
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-listbyuser"REDACTED)

	s.createUsageLog(user, apiKey, account, 10, 20, 0.5, time.Now())
	s.createUsageLog(user, apiKey, account, 15, 25, 0.6, time.Now())

	logs, page, err := s.repo.ListByUser(s.ctx, user.ID, pagination.PaginationParams{Page: 1, PageSize: 10REDACTED)
	s.Require().NoError(err, "ListByUser")
	s.Require().Len(logs, 2)
	s.Require().Equal(int64(2), page.Total)
REDACTED

// --- ListByApiKey ---

func (s *UsageLogRepoSuite) TestListByApiKey() {
	user := mustCreateUser(s.T(), s.client, &service.User{Email: "listbyapikey@test.com"REDACTED)
	apiKey := mustCreateApiKey(s.T(), s.client, &service.ApiKey{UserID: user.ID, Key: "sk-listbyapikey", Name: "k"REDACTED)
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-listbyapikey"REDACTED)

	s.createUsageLog(user, apiKey, account, 10, 20, 0.5, time.Now())
	s.createUsageLog(user, apiKey, account, 15, 25, 0.6, time.Now())

	logs, page, err := s.repo.ListByApiKey(s.ctx, apiKey.ID, pagination.PaginationParams{Page: 1, PageSize: 10REDACTED)
	s.Require().NoError(err, "ListByApiKey")
	s.Require().Len(logs, 2)
	s.Require().Equal(int64(2), page.Total)
REDACTED

// --- ListByAccount ---

func (s *UsageLogRepoSuite) TestListByAccount() {
	user := mustCreateUser(s.T(), s.client, &service.User{Email: "listbyaccount@test.com"REDACTED)
	apiKey := mustCreateApiKey(s.T(), s.client, &service.ApiKey{UserID: user.ID, Key: "sk-listbyaccount", Name: "k"REDACTED)
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-listbyaccount"REDACTED)

	s.createUsageLog(user, apiKey, account, 10, 20, 0.5, time.Now())

	logs, page, err := s.repo.ListByAccount(s.ctx, account.ID, pagination.PaginationParams{Page: 1, PageSize: 10REDACTED)
	s.Require().NoError(err, "ListByAccount")
	s.Require().Len(logs, 1)
	s.Require().Equal(int64(1), page.Total)
REDACTED

// --- GetUserStats ---

func (s *UsageLogRepoSuite) TestGetUserStats() {
	user := mustCreateUser(s.T(), s.client, &service.User{Email: "userstats@test.com"REDACTED)
	apiKey := mustCreateApiKey(s.T(), s.client, &service.ApiKey{UserID: user.ID, Key: "sk-userstats", Name: "k"REDACTED)
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-userstats"REDACTED)

	base := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	s.createUsageLog(user, apiKey, account, 10, 20, 0.5, base)
	s.createUsageLog(user, apiKey, account, 15, 25, 0.6, base.Add(1*time.Hour))

	startTime := base.Add(-1 * time.Hour)
	endTime := base.Add(2 * time.Hour)
	stats, err := s.repo.GetUserStats(s.ctx, user.ID, startTime, endTime)
	s.Require().NoError(err, "GetUserStats")
	s.Require().Equal(int64(2), stats.TotalRequests)
	s.Require().Equal(int64(25), stats.InputTokens)
	s.Require().Equal(int64(45), stats.OutputTokens)
REDACTED

// --- ListWithFilters ---

func (s *UsageLogRepoSuite) TestListWithFilters() {
	user := mustCreateUser(s.T(), s.client, &service.User{Email: "filters@test.com"REDACTED)
	apiKey := mustCreateApiKey(s.T(), s.client, &service.ApiKey{UserID: user.ID, Key: "sk-filters", Name: "k"REDACTED)
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-filters"REDACTED)

	s.createUsageLog(user, apiKey, account, 10, 20, 0.5, time.Now())

	filters := usagestats.UsageLogFilters{UserID: user.IDREDACTED
	logs, page, err := s.repo.ListWithFilters(s.ctx, pagination.PaginationParams{Page: 1, PageSize: 10REDACTED, filters)
	s.Require().NoError(err, "ListWithFilters")
	s.Require().Len(logs, 1)
	s.Require().Equal(int64(1), page.Total)
REDACTED

// --- GetDashboardStats ---

func (s *UsageLogRepoSuite) TestDashboardStats_TodayTotalsAndPerformance() {
	now := time.Now()
	todayStart := timezone.Today()
	baseStats, err := s.repo.GetDashboardStats(s.ctx)
	s.Require().NoError(err, "GetDashboardStats base")

	userToday := mustCreateUser(s.T(), s.client, &service.User{
		Email:     "today@example.com",
		CreatedAt: maxTime(todayStart.Add(10*time.Second), now.Add(-10*time.Second)),
		UpdatedAt: now,
REDACTED)
	userOld := mustCreateUser(s.T(), s.client, &service.User{
		Email:     "old@example.com",
		CreatedAt: todayStart.Add(-24 * time.Hour),
		UpdatedAt: todayStart.Add(-24 * time.Hour),
REDACTED)

	group := mustCreateGroup(s.T(), s.client, &service.Group{Name: "g-ul"REDACTED)
	apiKey1 := mustCreateApiKey(s.T(), s.client, &service.ApiKey{UserID: userToday.ID, Key: "sk-ul-1", Name: "ul1"REDACTED)
	mustCreateApiKey(s.T(), s.client, &service.ApiKey{UserID: userOld.ID, Key: "sk-ul-2", Name: "ul2", Status: service.StatusDisabledREDACTED)

	resetAt := now.Add(10 * time.Minute)
	accNormal := mustCreateAccount(s.T(), s.client, &service.Account{Name: "a-normal", Schedulable: trueREDACTED)
	mustCreateAccount(s.T(), s.client, &service.Account{Name: "a-error", Status: service.StatusError, Schedulable: trueREDACTED)
	mustCreateAccount(s.T(), s.client, &service.Account{Name: "a-rl", RateLimitedAt: &now, RateLimitResetAt: &resetAt, Schedulable: trueREDACTED)
	mustCreateAccount(s.T(), s.client, &service.Account{Name: "a-ov", OverloadUntil: &resetAt, Schedulable: trueREDACTED)

	d1, d2, d3 := 100, 200, 300
	logToday := &service.UsageLog{
		UserID:              userToday.ID,
		ApiKeyID:            apiKey1.ID,
		AccountID:           accNormal.ID,
		Model:               "claude-3",
		GroupID:             &group.ID,
		InputTokens:         10,
		OutputTokens:        20,
		CacheCreationTokens: 3,
		CacheReadTokens:     4,
		TotalCost:           1.5,
		ActualCost:          1.2,
		DurationMs:          &d1,
		CreatedAt:           maxTime(todayStart.Add(2*time.Minute), now.Add(-2*time.Minute)),
REDACTED
	_, err = s.repo.Create(s.ctx, logToday)
	s.Require().NoError(err, "Create logToday")

	logOld := &service.UsageLog{
		UserID:       userOld.ID,
		ApiKeyID:     apiKey1.ID,
		AccountID:    accNormal.ID,
		Model:        "claude-3",
		InputTokens:  5,
		OutputTokens: 6,
		TotalCost:    0.7,
		ActualCost:   0.7,
		DurationMs:   &d2,
		CreatedAt:    todayStart.Add(-1 * time.Hour),
REDACTED
	_, err = s.repo.Create(s.ctx, logOld)
	s.Require().NoError(err, "Create logOld")

	logPerf := &service.UsageLog{
		UserID:       userToday.ID,
		ApiKeyID:     apiKey1.ID,
		AccountID:    accNormal.ID,
		Model:        "claude-3",
		InputTokens:  1,
		OutputTokens: 2,
		TotalCost:    0.1,
		ActualCost:   0.1,
		DurationMs:   &d3,
		CreatedAt:    now.Add(-30 * time.Second),
REDACTED
	_, err = s.repo.Create(s.ctx, logPerf)
	s.Require().NoError(err, "Create logPerf")

	stats, err := s.repo.GetDashboardStats(s.ctx)
	s.Require().NoError(err, "GetDashboardStats")

	s.Require().Equal(baseStats.TotalUsers+2, stats.TotalUsers, "TotalUsers mismatch")
	s.Require().Equal(baseStats.TodayNewUsers+1, stats.TodayNewUsers, "TodayNewUsers mismatch")
	s.Require().Equal(baseStats.ActiveUsers+1, stats.ActiveUsers, "ActiveUsers mismatch")
	s.Require().Equal(baseStats.TotalApiKeys+2, stats.TotalApiKeys, "TotalApiKeys mismatch")
	s.Require().Equal(baseStats.ActiveApiKeys+1, stats.ActiveApiKeys, "ActiveApiKeys mismatch")
	s.Require().Equal(baseStats.TotalAccounts+4, stats.TotalAccounts, "TotalAccounts mismatch")
	s.Require().Equal(baseStats.ErrorAccounts+1, stats.ErrorAccounts, "ErrorAccounts mismatch")
	s.Require().Equal(baseStats.RateLimitAccounts+1, stats.RateLimitAccounts, "RateLimitAccounts mismatch")
	s.Require().Equal(baseStats.OverloadAccounts+1, stats.OverloadAccounts, "OverloadAccounts mismatch")

	s.Require().Equal(baseStats.TotalRequests+3, stats.TotalRequests, "TotalRequests mismatch")
	s.Require().Equal(baseStats.TotalInputTokens+int64(16), stats.TotalInputTokens, "TotalInputTokens mismatch")
	s.Require().Equal(baseStats.TotalOutputTokens+int64(28), stats.TotalOutputTokens, "TotalOutputTokens mismatch")
	s.Require().Equal(baseStats.TotalCacheCreationTokens+int64(3), stats.TotalCacheCreationTokens, "TotalCacheCreationTokens mismatch")
	s.Require().Equal(baseStats.TotalCacheReadTokens+int64(4), stats.TotalCacheReadTokens, "TotalCacheReadTokens mismatch")
	s.Require().Equal(baseStats.TotalTokens+int64(51), stats.TotalTokens, "TotalTokens mismatch")
	s.Require().Equal(baseStats.TotalCost+2.3, stats.TotalCost, "TotalCost mismatch")
	s.Require().Equal(baseStats.TotalActualCost+2.0, stats.TotalActualCost, "TotalActualCost mismatch")
	s.Require().GreaterOrEqual(stats.TodayRequests, int64(1), "expected TodayRequests >= 1")
	s.Require().GreaterOrEqual(stats.TodayCost, 0.0, "expected TodayCost >= 0")

	wantRpm, wantTpm, err := s.repo.getPerformanceStats(s.ctx, 0)
	s.Require().NoError(err, "getPerformanceStats")
	s.Require().Equal(wantRpm, stats.Rpm, "Rpm mismatch")
	s.Require().Equal(wantTpm, stats.Tpm, "Tpm mismatch")
REDACTED

// --- GetUserDashboardStats ---

func (s *UsageLogRepoSuite) TestGetUserDashboardStats() {
	user := mustCreateUser(s.T(), s.client, &service.User{Email: "userdash@test.com"REDACTED)
	apiKey := mustCreateApiKey(s.T(), s.client, &service.ApiKey{UserID: user.ID, Key: "sk-userdash", Name: "k"REDACTED)
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-userdash"REDACTED)

	s.createUsageLog(user, apiKey, account, 10, 20, 0.5, time.Now())

	stats, err := s.repo.GetUserDashboardStats(s.ctx, user.ID)
	s.Require().NoError(err, "GetUserDashboardStats")
	s.Require().Equal(int64(1), stats.TotalApiKeys)
	s.Require().Equal(int64(1), stats.TotalRequests)
REDACTED

// --- GetAccountTodayStats ---

func (s *UsageLogRepoSuite) TestGetAccountTodayStats() {
	user := mustCreateUser(s.T(), s.client, &service.User{Email: "acctoday@test.com"REDACTED)
	apiKey := mustCreateApiKey(s.T(), s.client, &service.ApiKey{UserID: user.ID, Key: "sk-acctoday", Name: "k"REDACTED)
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-today"REDACTED)

	s.createUsageLog(user, apiKey, account, 10, 20, 0.5, time.Now())

	stats, err := s.repo.GetAccountTodayStats(s.ctx, account.ID)
	s.Require().NoError(err, "GetAccountTodayStats")
	s.Require().Equal(int64(1), stats.Requests)
	s.Require().Equal(int64(30), stats.Tokens)
REDACTED

// --- GetBatchUserUsageStats ---

func (s *UsageLogRepoSuite) TestGetBatchUserUsageStats() {
	user1 := mustCreateUser(s.T(), s.client, &service.User{Email: "batch1@test.com"REDACTED)
	user2 := mustCreateUser(s.T(), s.client, &service.User{Email: "batch2@test.com"REDACTED)
	apiKey1 := mustCreateApiKey(s.T(), s.client, &service.ApiKey{UserID: user1.ID, Key: "sk-batch1", Name: "k"REDACTED)
	apiKey2 := mustCreateApiKey(s.T(), s.client, &service.ApiKey{UserID: user2.ID, Key: "sk-batch2", Name: "k"REDACTED)
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-batch"REDACTED)

	s.createUsageLog(user1, apiKey1, account, 10, 20, 0.5, time.Now())
	s.createUsageLog(user2, apiKey2, account, 15, 25, 0.6, time.Now())

	stats, err := s.repo.GetBatchUserUsageStats(s.ctx, []int64{user1.ID, user2.IDREDACTED)
	s.Require().NoError(err, "GetBatchUserUsageStats")
	s.Require().Len(stats, 2)
	s.Require().NotNil(stats[user1.ID])
	s.Require().NotNil(stats[user2.ID])
REDACTED

func (s *UsageLogRepoSuite) TestGetBatchUserUsageStats_Empty() {
	stats, err := s.repo.GetBatchUserUsageStats(s.ctx, []int64{REDACTED)
	s.Require().NoError(err)
	s.Require().Empty(stats)
REDACTED

// --- GetBatchApiKeyUsageStats ---

func (s *UsageLogRepoSuite) TestGetBatchApiKeyUsageStats() {
	user := mustCreateUser(s.T(), s.client, &service.User{Email: "batchkey@test.com"REDACTED)
	apiKey1 := mustCreateApiKey(s.T(), s.client, &service.ApiKey{UserID: user.ID, Key: "sk-batchkey1", Name: "k1"REDACTED)
	apiKey2 := mustCreateApiKey(s.T(), s.client, &service.ApiKey{UserID: user.ID, Key: "sk-batchkey2", Name: "k2"REDACTED)
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-batchkey"REDACTED)

	s.createUsageLog(user, apiKey1, account, 10, 20, 0.5, time.Now())
	s.createUsageLog(user, apiKey2, account, 15, 25, 0.6, time.Now())

	stats, err := s.repo.GetBatchApiKeyUsageStats(s.ctx, []int64{apiKey1.ID, apiKey2.IDREDACTED)
	s.Require().NoError(err, "GetBatchApiKeyUsageStats")
	s.Require().Len(stats, 2)
REDACTED

func (s *UsageLogRepoSuite) TestGetBatchApiKeyUsageStats_Empty() {
	stats, err := s.repo.GetBatchApiKeyUsageStats(s.ctx, []int64{REDACTED)
	s.Require().NoError(err)
	s.Require().Empty(stats)
REDACTED

// --- GetGlobalStats ---

func (s *UsageLogRepoSuite) TestGetGlobalStats() {
	user := mustCreateUser(s.T(), s.client, &service.User{Email: "global@test.com"REDACTED)
	apiKey := mustCreateApiKey(s.T(), s.client, &service.ApiKey{UserID: user.ID, Key: "sk-global", Name: "k"REDACTED)
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-global"REDACTED)

	base := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	s.createUsageLog(user, apiKey, account, 10, 20, 0.5, base)
	s.createUsageLog(user, apiKey, account, 15, 25, 0.6, base.Add(1*time.Hour))

	stats, err := s.repo.GetGlobalStats(s.ctx, base.Add(-1*time.Hour), base.Add(2*time.Hour))
	s.Require().NoError(err, "GetGlobalStats")
	s.Require().Equal(int64(2), stats.TotalRequests)
	s.Require().Equal(int64(25), stats.TotalInputTokens)
	s.Require().Equal(int64(45), stats.TotalOutputTokens)
REDACTED

func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
REDACTED
	return b
REDACTED

// --- ListByUserAndTimeRange ---

func (s *UsageLogRepoSuite) TestListByUserAndTimeRange() {
	user := mustCreateUser(s.T(), s.client, &service.User{Email: "timerange@test.com"REDACTED)
	apiKey := mustCreateApiKey(s.T(), s.client, &service.ApiKey{UserID: user.ID, Key: "sk-timerange", Name: "k"REDACTED)
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-timerange"REDACTED)

	base := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	s.createUsageLog(user, apiKey, account, 10, 20, 0.5, base)
	s.createUsageLog(user, apiKey, account, 15, 25, 0.6, base.Add(1*time.Hour))
	s.createUsageLog(user, apiKey, account, 20, 30, 0.7, base.Add(-24*time.Hour)) // outside range

	startTime := base.Add(-1 * time.Hour)
	endTime := base.Add(2 * time.Hour)
	logs, _, err := s.repo.ListByUserAndTimeRange(s.ctx, user.ID, startTime, endTime)
	s.Require().NoError(err, "ListByUserAndTimeRange")
	s.Require().Len(logs, 2)
REDACTED

// --- ListByApiKeyAndTimeRange ---

func (s *UsageLogRepoSuite) TestListByApiKeyAndTimeRange() {
	user := mustCreateUser(s.T(), s.client, &service.User{Email: "keytimerange@test.com"REDACTED)
	apiKey := mustCreateApiKey(s.T(), s.client, &service.ApiKey{UserID: user.ID, Key: "sk-keytimerange", Name: "k"REDACTED)
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-keytimerange"REDACTED)

	base := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	s.createUsageLog(user, apiKey, account, 10, 20, 0.5, base)
	s.createUsageLog(user, apiKey, account, 15, 25, 0.6, base.Add(30*time.Minute))
	s.createUsageLog(user, apiKey, account, 20, 30, 0.7, base.Add(-24*time.Hour)) // outside range

	startTime := base.Add(-1 * time.Hour)
	endTime := base.Add(2 * time.Hour)
	logs, _, err := s.repo.ListByApiKeyAndTimeRange(s.ctx, apiKey.ID, startTime, endTime)
	s.Require().NoError(err, "ListByApiKeyAndTimeRange")
	s.Require().Len(logs, 2)
REDACTED

// --- ListByAccountAndTimeRange ---

func (s *UsageLogRepoSuite) TestListByAccountAndTimeRange() {
	user := mustCreateUser(s.T(), s.client, &service.User{Email: "acctimerange@test.com"REDACTED)
	apiKey := mustCreateApiKey(s.T(), s.client, &service.ApiKey{UserID: user.ID, Key: "sk-acctimerange", Name: "k"REDACTED)
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-acctimerange"REDACTED)

	base := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	s.createUsageLog(user, apiKey, account, 10, 20, 0.5, base)
	s.createUsageLog(user, apiKey, account, 15, 25, 0.6, base.Add(45*time.Minute))
	s.createUsageLog(user, apiKey, account, 20, 30, 0.7, base.Add(-24*time.Hour)) // outside range

	startTime := base.Add(-1 * time.Hour)
	endTime := base.Add(2 * time.Hour)
	logs, _, err := s.repo.ListByAccountAndTimeRange(s.ctx, account.ID, startTime, endTime)
	s.Require().NoError(err, "ListByAccountAndTimeRange")
	s.Require().Len(logs, 2)
REDACTED

// --- ListByModelAndTimeRange ---

func (s *UsageLogRepoSuite) TestListByModelAndTimeRange() {
	user := mustCreateUser(s.T(), s.client, &service.User{Email: "modeltimerange@test.com"REDACTED)
	apiKey := mustCreateApiKey(s.T(), s.client, &service.ApiKey{UserID: user.ID, Key: "sk-modeltimerange", Name: "k"REDACTED)
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-modeltimerange"REDACTED)

	base := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)

	// Create logs with different models
	log1 := &service.UsageLog{
		UserID:       user.ID,
		ApiKeyID:     apiKey.ID,
		AccountID:    account.ID,
		Model:        "claude-3-opus",
		InputTokens:  10,
		OutputTokens: 20,
		TotalCost:    0.5,
		ActualCost:   0.5,
		CreatedAt:    base,
REDACTED
	_, err := s.repo.Create(s.ctx, log1)
	s.Require().NoError(err)

	log2 := &service.UsageLog{
		UserID:       user.ID,
		ApiKeyID:     apiKey.ID,
		AccountID:    account.ID,
		Model:        "claude-3-opus",
		InputTokens:  15,
		OutputTokens: 25,
		TotalCost:    0.6,
		ActualCost:   0.6,
		CreatedAt:    base.Add(30 * time.Minute),
REDACTED
	_, err = s.repo.Create(s.ctx, log2)
	s.Require().NoError(err)

	log3 := &service.UsageLog{
		UserID:       user.ID,
		ApiKeyID:     apiKey.ID,
		AccountID:    account.ID,
		Model:        "claude-3-sonnet",
		InputTokens:  20,
		OutputTokens: 30,
		TotalCost:    0.7,
		ActualCost:   0.7,
		CreatedAt:    base.Add(1 * time.Hour),
REDACTED
	_, err = s.repo.Create(s.ctx, log3)
	s.Require().NoError(err)

	startTime := base.Add(-1 * time.Hour)
	endTime := base.Add(2 * time.Hour)
	logs, _, err := s.repo.ListByModelAndTimeRange(s.ctx, "claude-3-opus", startTime, endTime)
	s.Require().NoError(err, "ListByModelAndTimeRange")
	s.Require().Len(logs, 2)
REDACTED

// --- GetAccountWindowStats ---

func (s *UsageLogRepoSuite) TestGetAccountWindowStats() {
	user := mustCreateUser(s.T(), s.client, &service.User{Email: "windowstats@test.com"REDACTED)
	apiKey := mustCreateApiKey(s.T(), s.client, &service.ApiKey{UserID: user.ID, Key: "sk-windowstats", Name: "k"REDACTED)
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-windowstats"REDACTED)

	now := time.Now()
	windowStart := now.Add(-10 * time.Minute)

	s.createUsageLog(user, apiKey, account, 10, 20, 0.5, now.Add(-5*time.Minute))
	s.createUsageLog(user, apiKey, account, 15, 25, 0.6, now.Add(-3*time.Minute))
	s.createUsageLog(user, apiKey, account, 20, 30, 0.7, now.Add(-30*time.Minute)) // outside window

	stats, err := s.repo.GetAccountWindowStats(s.ctx, account.ID, windowStart)
	s.Require().NoError(err, "GetAccountWindowStats")
	s.Require().Equal(int64(2), stats.Requests)
	s.Require().Equal(int64(70), stats.Tokens) // (10+20) + (15+25)
REDACTED

// --- GetUserUsageTrendByUserID ---

func (s *UsageLogRepoSuite) TestGetUserUsageTrendByUserID() {
	user := mustCreateUser(s.T(), s.client, &service.User{Email: "usertrend@test.com"REDACTED)
	apiKey := mustCreateApiKey(s.T(), s.client, &service.ApiKey{UserID: user.ID, Key: "sk-usertrend", Name: "k"REDACTED)
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-usertrend"REDACTED)

	base := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	s.createUsageLog(user, apiKey, account, 10, 20, 0.5, base)
	s.createUsageLog(user, apiKey, account, 15, 25, 0.6, base.Add(1*time.Hour))
	s.createUsageLog(user, apiKey, account, 20, 30, 0.7, base.Add(24*time.Hour)) // next day

	startTime := base.Add(-1 * time.Hour)
	endTime := base.Add(48 * time.Hour)
	trend, err := s.repo.GetUserUsageTrendByUserID(s.ctx, user.ID, startTime, endTime, "day")
	s.Require().NoError(err, "GetUserUsageTrendByUserID")
	s.Require().Len(trend, 2) // 2 different days
REDACTED

func (s *UsageLogRepoSuite) TestGetUserUsageTrendByUserID_HourlyGranularity() {
	user := mustCreateUser(s.T(), s.client, &service.User{Email: "usertrendhourly@test.com"REDACTED)
	apiKey := mustCreateApiKey(s.T(), s.client, &service.ApiKey{UserID: user.ID, Key: "sk-usertrendhourly", Name: "k"REDACTED)
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-usertrendhourly"REDACTED)

	base := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	s.createUsageLog(user, apiKey, account, 10, 20, 0.5, base)
	s.createUsageLog(user, apiKey, account, 15, 25, 0.6, base.Add(1*time.Hour))
	s.createUsageLog(user, apiKey, account, 20, 30, 0.7, base.Add(2*time.Hour))

	startTime := base.Add(-1 * time.Hour)
	endTime := base.Add(3 * time.Hour)
	trend, err := s.repo.GetUserUsageTrendByUserID(s.ctx, user.ID, startTime, endTime, "hour")
	s.Require().NoError(err, "GetUserUsageTrendByUserID hourly")
	s.Require().Len(trend, 3) // 3 different hours
REDACTED

// --- GetUserModelStats ---

func (s *UsageLogRepoSuite) TestGetUserModelStats() {
	user := mustCreateUser(s.T(), s.client, &service.User{Email: "modelstats@test.com"REDACTED)
	apiKey := mustCreateApiKey(s.T(), s.client, &service.ApiKey{UserID: user.ID, Key: "sk-modelstats", Name: "k"REDACTED)
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-modelstats"REDACTED)

	base := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)

	// Create logs with different models
	log1 := &service.UsageLog{
		UserID:       user.ID,
		ApiKeyID:     apiKey.ID,
		AccountID:    account.ID,
		Model:        "claude-3-opus",
		InputTokens:  100,
		OutputTokens: 200,
		TotalCost:    0.5,
		ActualCost:   0.5,
		CreatedAt:    base,
REDACTED
	_, err := s.repo.Create(s.ctx, log1)
	s.Require().NoError(err)

	log2 := &service.UsageLog{
		UserID:       user.ID,
		ApiKeyID:     apiKey.ID,
		AccountID:    account.ID,
		Model:        "claude-3-sonnet",
		InputTokens:  50,
		OutputTokens: 100,
		TotalCost:    0.2,
		ActualCost:   0.2,
		CreatedAt:    base.Add(1 * time.Hour),
REDACTED
	_, err = s.repo.Create(s.ctx, log2)
	s.Require().NoError(err)

	startTime := base.Add(-1 * time.Hour)
	endTime := base.Add(2 * time.Hour)
	stats, err := s.repo.GetUserModelStats(s.ctx, user.ID, startTime, endTime)
	s.Require().NoError(err, "GetUserModelStats")
	s.Require().Len(stats, 2)

	// Should be ordered by total_tokens DESC
	s.Require().Equal("claude-3-opus", stats[0].Model)
	s.Require().Equal(int64(300), stats[0].TotalTokens)
REDACTED

// --- GetUsageTrendWithFilters ---

func (s *UsageLogRepoSuite) TestGetUsageTrendWithFilters() {
	user := mustCreateUser(s.T(), s.client, &service.User{Email: "trendfilters@test.com"REDACTED)
	apiKey := mustCreateApiKey(s.T(), s.client, &service.ApiKey{UserID: user.ID, Key: "sk-trendfilters", Name: "k"REDACTED)
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-trendfilters"REDACTED)

	base := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	s.createUsageLog(user, apiKey, account, 10, 20, 0.5, base)
	s.createUsageLog(user, apiKey, account, 15, 25, 0.6, base.Add(24*time.Hour))

	startTime := base.Add(-1 * time.Hour)
	endTime := base.Add(48 * time.Hour)

	// Test with user filter
	trend, err := s.repo.GetUsageTrendWithFilters(s.ctx, startTime, endTime, "day", user.ID, 0)
	s.Require().NoError(err, "GetUsageTrendWithFilters user filter")
	s.Require().Len(trend, 2)

	// Test with apiKey filter
	trend, err = s.repo.GetUsageTrendWithFilters(s.ctx, startTime, endTime, "day", 0, apiKey.ID)
	s.Require().NoError(err, "GetUsageTrendWithFilters apiKey filter")
	s.Require().Len(trend, 2)

	// Test with both filters
	trend, err = s.repo.GetUsageTrendWithFilters(s.ctx, startTime, endTime, "day", user.ID, apiKey.ID)
	s.Require().NoError(err, "GetUsageTrendWithFilters both filters")
	s.Require().Len(trend, 2)
REDACTED

func (s *UsageLogRepoSuite) TestGetUsageTrendWithFilters_HourlyGranularity() {
	user := mustCreateUser(s.T(), s.client, &service.User{Email: "trendfilters-h@test.com"REDACTED)
	apiKey := mustCreateApiKey(s.T(), s.client, &service.ApiKey{UserID: user.ID, Key: "sk-trendfilters-h", Name: "k"REDACTED)
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-trendfilters-h"REDACTED)

	base := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	s.createUsageLog(user, apiKey, account, 10, 20, 0.5, base)
	s.createUsageLog(user, apiKey, account, 15, 25, 0.6, base.Add(1*time.Hour))

	startTime := base.Add(-1 * time.Hour)
	endTime := base.Add(3 * time.Hour)

	trend, err := s.repo.GetUsageTrendWithFilters(s.ctx, startTime, endTime, "hour", user.ID, 0)
	s.Require().NoError(err, "GetUsageTrendWithFilters hourly")
	s.Require().Len(trend, 2)
REDACTED

// --- GetModelStatsWithFilters ---

func (s *UsageLogRepoSuite) TestGetModelStatsWithFilters() {
	user := mustCreateUser(s.T(), s.client, &service.User{Email: "modelfilters@test.com"REDACTED)
	apiKey := mustCreateApiKey(s.T(), s.client, &service.ApiKey{UserID: user.ID, Key: "sk-modelfilters", Name: "k"REDACTED)
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-modelfilters"REDACTED)

	base := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)

	log1 := &service.UsageLog{
		UserID:       user.ID,
		ApiKeyID:     apiKey.ID,
		AccountID:    account.ID,
		Model:        "claude-3-opus",
		InputTokens:  100,
		OutputTokens: 200,
		TotalCost:    0.5,
		ActualCost:   0.5,
		CreatedAt:    base,
REDACTED
	_, err := s.repo.Create(s.ctx, log1)
	s.Require().NoError(err)

	log2 := &service.UsageLog{
		UserID:       user.ID,
		ApiKeyID:     apiKey.ID,
		AccountID:    account.ID,
		Model:        "claude-3-sonnet",
		InputTokens:  50,
		OutputTokens: 100,
		TotalCost:    0.2,
		ActualCost:   0.2,
		CreatedAt:    base.Add(1 * time.Hour),
REDACTED
	_, err = s.repo.Create(s.ctx, log2)
	s.Require().NoError(err)

	startTime := base.Add(-1 * time.Hour)
	endTime := base.Add(2 * time.Hour)

	// Test with user filter
	stats, err := s.repo.GetModelStatsWithFilters(s.ctx, startTime, endTime, user.ID, 0, 0)
	s.Require().NoError(err, "GetModelStatsWithFilters user filter")
	s.Require().Len(stats, 2)

	// Test with apiKey filter
	stats, err = s.repo.GetModelStatsWithFilters(s.ctx, startTime, endTime, 0, apiKey.ID, 0)
	s.Require().NoError(err, "GetModelStatsWithFilters apiKey filter")
	s.Require().Len(stats, 2)

	// Test with account filter
	stats, err = s.repo.GetModelStatsWithFilters(s.ctx, startTime, endTime, 0, 0, account.ID)
	s.Require().NoError(err, "GetModelStatsWithFilters account filter")
	s.Require().Len(stats, 2)
REDACTED

// --- GetAccountUsageStats ---

func (s *UsageLogRepoSuite) TestGetAccountUsageStats() {
	user := mustCreateUser(s.T(), s.client, &service.User{Email: "accstats@test.com"REDACTED)
	apiKey := mustCreateApiKey(s.T(), s.client, &service.ApiKey{UserID: user.ID, Key: "sk-accstats", Name: "k"REDACTED)
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-accstats"REDACTED)

	base := time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC)

	// Create logs on different days
	log1 := &service.UsageLog{
		UserID:       user.ID,
		ApiKeyID:     apiKey.ID,
		AccountID:    account.ID,
		Model:        "claude-3-opus",
		InputTokens:  100,
		OutputTokens: 200,
		TotalCost:    0.5,
		ActualCost:   0.4,
		CreatedAt:    base.Add(12 * time.Hour),
REDACTED
	_, err := s.repo.Create(s.ctx, log1)
	s.Require().NoError(err)

	log2 := &service.UsageLog{
		UserID:       user.ID,
		ApiKeyID:     apiKey.ID,
		AccountID:    account.ID,
		Model:        "claude-3-sonnet",
		InputTokens:  50,
		OutputTokens: 100,
		TotalCost:    0.2,
		ActualCost:   0.15,
		CreatedAt:    base.Add(36 * time.Hour), // next day
REDACTED
	_, err = s.repo.Create(s.ctx, log2)
	s.Require().NoError(err)

	startTime := base
	endTime := base.Add(72 * time.Hour)

	resp, err := s.repo.GetAccountUsageStats(s.ctx, account.ID, startTime, endTime)
	s.Require().NoError(err, "GetAccountUsageStats")

	s.Require().Len(resp.History, 2, "expected 2 days of history")
	s.Require().Equal(int64(2), resp.Summary.TotalRequests)
	s.Require().Equal(int64(450), resp.Summary.TotalTokens)
	s.Require().Len(resp.Models, 2)
REDACTED

func (s *UsageLogRepoSuite) TestGetAccountUsageStats_EmptyRange() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-emptystats"REDACTED)

	base := time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC)
	startTime := base
	endTime := base.Add(72 * time.Hour)

	resp, err := s.repo.GetAccountUsageStats(s.ctx, account.ID, startTime, endTime)
	s.Require().NoError(err, "GetAccountUsageStats empty")

	s.Require().Len(resp.History, 0)
	s.Require().Equal(int64(0), resp.Summary.TotalRequests)
REDACTED

// --- GetUserUsageTrend ---

func (s *UsageLogRepoSuite) TestGetUserUsageTrend() {
	user1 := mustCreateUser(s.T(), s.client, &service.User{Email: "usertrend1@test.com"REDACTED)
	user2 := mustCreateUser(s.T(), s.client, &service.User{Email: "usertrend2@test.com"REDACTED)
	apiKey1 := mustCreateApiKey(s.T(), s.client, &service.ApiKey{UserID: user1.ID, Key: "sk-usertrend1", Name: "k1"REDACTED)
	apiKey2 := mustCreateApiKey(s.T(), s.client, &service.ApiKey{UserID: user2.ID, Key: "sk-usertrend2", Name: "k2"REDACTED)
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-usertrends"REDACTED)

	base := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	s.createUsageLog(user1, apiKey1, account, 100, 200, 1.0, base)
	s.createUsageLog(user2, apiKey2, account, 50, 100, 0.5, base)
	s.createUsageLog(user1, apiKey1, account, 100, 200, 1.0, base.Add(24*time.Hour))

	startTime := base.Add(-1 * time.Hour)
	endTime := base.Add(48 * time.Hour)

	trend, err := s.repo.GetUserUsageTrend(s.ctx, startTime, endTime, "day", 10)
	s.Require().NoError(err, "GetUserUsageTrend")
	s.Require().GreaterOrEqual(len(trend), 2)
REDACTED

// --- GetApiKeyUsageTrend ---

func (s *UsageLogRepoSuite) TestGetApiKeyUsageTrend() {
	user := mustCreateUser(s.T(), s.client, &service.User{Email: "keytrend@test.com"REDACTED)
	apiKey1 := mustCreateApiKey(s.T(), s.client, &service.ApiKey{UserID: user.ID, Key: "sk-keytrend1", Name: "k1"REDACTED)
	apiKey2 := mustCreateApiKey(s.T(), s.client, &service.ApiKey{UserID: user.ID, Key: "sk-keytrend2", Name: "k2"REDACTED)
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-keytrends"REDACTED)

	base := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	s.createUsageLog(user, apiKey1, account, 100, 200, 1.0, base)
	s.createUsageLog(user, apiKey2, account, 50, 100, 0.5, base)
	s.createUsageLog(user, apiKey1, account, 100, 200, 1.0, base.Add(24*time.Hour))

	startTime := base.Add(-1 * time.Hour)
	endTime := base.Add(48 * time.Hour)

	trend, err := s.repo.GetApiKeyUsageTrend(s.ctx, startTime, endTime, "day", 10)
	s.Require().NoError(err, "GetApiKeyUsageTrend")
	s.Require().GreaterOrEqual(len(trend), 2)
REDACTED

func (s *UsageLogRepoSuite) TestGetApiKeyUsageTrend_HourlyGranularity() {
	user := mustCreateUser(s.T(), s.client, &service.User{Email: "keytrendh@test.com"REDACTED)
	apiKey := mustCreateApiKey(s.T(), s.client, &service.ApiKey{UserID: user.ID, Key: "sk-keytrendh", Name: "k"REDACTED)
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-keytrendh"REDACTED)

	base := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	s.createUsageLog(user, apiKey, account, 100, 200, 1.0, base)
	s.createUsageLog(user, apiKey, account, 50, 100, 0.5, base.Add(1*time.Hour))

	startTime := base.Add(-1 * time.Hour)
	endTime := base.Add(3 * time.Hour)

	trend, err := s.repo.GetApiKeyUsageTrend(s.ctx, startTime, endTime, "hour", 10)
	s.Require().NoError(err, "GetApiKeyUsageTrend hourly")
	s.Require().Len(trend, 2)
REDACTED

// --- ListWithFilters (additional filter tests) ---

func (s *UsageLogRepoSuite) TestListWithFilters_ApiKeyFilter() {
	user := mustCreateUser(s.T(), s.client, &service.User{Email: "filterskey@test.com"REDACTED)
	apiKey := mustCreateApiKey(s.T(), s.client, &service.ApiKey{UserID: user.ID, Key: "sk-filterskey", Name: "k"REDACTED)
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-filterskey"REDACTED)

	s.createUsageLog(user, apiKey, account, 10, 20, 0.5, time.Now())

	filters := usagestats.UsageLogFilters{ApiKeyID: apiKey.IDREDACTED
	logs, page, err := s.repo.ListWithFilters(s.ctx, pagination.PaginationParams{Page: 1, PageSize: 10REDACTED, filters)
	s.Require().NoError(err, "ListWithFilters apiKey")
	s.Require().Len(logs, 1)
	s.Require().Equal(int64(1), page.Total)
REDACTED

func (s *UsageLogRepoSuite) TestListWithFilters_TimeRange() {
	user := mustCreateUser(s.T(), s.client, &service.User{Email: "filterstime@test.com"REDACTED)
	apiKey := mustCreateApiKey(s.T(), s.client, &service.ApiKey{UserID: user.ID, Key: "sk-filterstime", Name: "k"REDACTED)
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-filterstime"REDACTED)

	base := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	s.createUsageLog(user, apiKey, account, 10, 20, 0.5, base)
	s.createUsageLog(user, apiKey, account, 15, 25, 0.6, base.Add(1*time.Hour))
	s.createUsageLog(user, apiKey, account, 20, 30, 0.7, base.Add(-24*time.Hour)) // outside range

	startTime := base.Add(-1 * time.Hour)
	endTime := base.Add(2 * time.Hour)
	filters := usagestats.UsageLogFilters{StartTime: &startTime, EndTime: &endTimeREDACTED
	logs, page, err := s.repo.ListWithFilters(s.ctx, pagination.PaginationParams{Page: 1, PageSize: 10REDACTED, filters)
	s.Require().NoError(err, "ListWithFilters time range")
	s.Require().Len(logs, 2)
	s.Require().Equal(int64(2), page.Total)
REDACTED

func (s *UsageLogRepoSuite) TestListWithFilters_CombinedFilters() {
	user := mustCreateUser(s.T(), s.client, &service.User{Email: "filterscombined@test.com"REDACTED)
	apiKey := mustCreateApiKey(s.T(), s.client, &service.ApiKey{UserID: user.ID, Key: "sk-filterscombined", Name: "k"REDACTED)
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-filterscombined"REDACTED)

	base := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	s.createUsageLog(user, apiKey, account, 10, 20, 0.5, base)
	s.createUsageLog(user, apiKey, account, 15, 25, 0.6, base.Add(1*time.Hour))

	startTime := base.Add(-1 * time.Hour)
	endTime := base.Add(2 * time.Hour)
	filters := usagestats.UsageLogFilters{
		UserID:    user.ID,
		ApiKeyID:  apiKey.ID,
		StartTime: &startTime,
		EndTime:   &endTime,
REDACTED
	logs, page, err := s.repo.ListWithFilters(s.ctx, pagination.PaginationParams{Page: 1, PageSize: 10REDACTED, filters)
	s.Require().NoError(err, "ListWithFilters combined")
	s.Require().Len(logs, 2)
	s.Require().Equal(int64(2), page.Total)
REDACTED
