//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/model"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"
)

type UsageLogRepoSuite struct {
	suite.Suite
	ctx  context.Context
	db   *gorm.DB
	repo *UsageLogRepository
}

func (s *UsageLogRepoSuite) SetupTest() {
	s.ctx = context.Background()
	s.db = testTx(s.T())
	s.repo = NewUsageLogRepository(s.db)
}

func TestUsageLogRepoSuite(t *testing.T) {
	suite.Run(t, new(UsageLogRepoSuite))
}

func (s *UsageLogRepoSuite) createUsageLog(user *model.User, apiKey *model.ApiKey, account *model.Account, inputTokens, outputTokens int, cost float64, createdAt time.Time) *model.UsageLog {
	log := &model.UsageLog{
		UserID:       user.ID,
		ApiKeyID:     apiKey.ID,
		AccountID:    account.ID,
		Model:        "claude-3",
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
		TotalCost:    cost,
		ActualCost:   cost,
		CreatedAt:    createdAt,
	}
	s.Require().NoError(s.repo.Create(s.ctx, log))
	return log
}

// --- Create / GetByID ---

func (s *UsageLogRepoSuite) TestCreate() {
	user := mustCreateUser(s.T(), s.db, &model.User{Email: "create@test.com"})
	apiKey := mustCreateApiKey(s.T(), s.db, &model.ApiKey{UserID: user.ID, Key: "sk-create", Name: "k"})
	account := mustCreateAccount(s.T(), s.db, &model.Account{Name: "acc-create"})

	log := &model.UsageLog{
		UserID:       user.ID,
		ApiKeyID:     apiKey.ID,
		AccountID:    account.ID,
		Model:        "claude-3",
		InputTokens:  10,
		OutputTokens: 20,
		TotalCost:    0.5,
		ActualCost:   0.4,
	}

	err := s.repo.Create(s.ctx, log)
	s.Require().NoError(err, "Create")
	s.Require().NotZero(log.ID)
}

func (s *UsageLogRepoSuite) TestGetByID() {
	user := mustCreateUser(s.T(), s.db, &model.User{Email: "getbyid@test.com"})
	apiKey := mustCreateApiKey(s.T(), s.db, &model.ApiKey{UserID: user.ID, Key: "sk-getbyid", Name: "k"})
	account := mustCreateAccount(s.T(), s.db, &model.Account{Name: "acc-getbyid"})

	log := s.createUsageLog(user, apiKey, account, 10, 20, 0.5, time.Now())

	got, err := s.repo.GetByID(s.ctx, log.ID)
	s.Require().NoError(err, "GetByID")
	s.Require().Equal(log.ID, got.ID)
	s.Require().Equal(10, got.InputTokens)
}

func (s *UsageLogRepoSuite) TestGetByID_NotFound() {
	_, err := s.repo.GetByID(s.ctx, 999999)
	s.Require().Error(err, "expected error for non-existent ID")
}

// --- Delete ---

func (s *UsageLogRepoSuite) TestDelete() {
	user := mustCreateUser(s.T(), s.db, &model.User{Email: "delete@test.com"})
	apiKey := mustCreateApiKey(s.T(), s.db, &model.ApiKey{UserID: user.ID, Key: "sk-delete", Name: "k"})
	account := mustCreateAccount(s.T(), s.db, &model.Account{Name: "acc-delete"})

	log := s.createUsageLog(user, apiKey, account, 10, 20, 0.5, time.Now())

	err := s.repo.Delete(s.ctx, log.ID)
	s.Require().NoError(err, "Delete")

	_, err = s.repo.GetByID(s.ctx, log.ID)
	s.Require().Error(err, "expected error after delete")
}

// --- ListByUser ---

func (s *UsageLogRepoSuite) TestListByUser() {
	user := mustCreateUser(s.T(), s.db, &model.User{Email: "listbyuser@test.com"})
	apiKey := mustCreateApiKey(s.T(), s.db, &model.ApiKey{UserID: user.ID, Key: "sk-listbyuser", Name: "k"})
	account := mustCreateAccount(s.T(), s.db, &model.Account{Name: "acc-listbyuser"})

	s.createUsageLog(user, apiKey, account, 10, 20, 0.5, time.Now())
	s.createUsageLog(user, apiKey, account, 15, 25, 0.6, time.Now())

	logs, page, err := s.repo.ListByUser(s.ctx, user.ID, pagination.PaginationParams{Page: 1, PageSize: 10})
	s.Require().NoError(err, "ListByUser")
	s.Require().Len(logs, 2)
	s.Require().Equal(int64(2), page.Total)
}

// --- ListByApiKey ---

func (s *UsageLogRepoSuite) TestListByApiKey() {
	user := mustCreateUser(s.T(), s.db, &model.User{Email: "listbyapikey@test.com"})
	apiKey := mustCreateApiKey(s.T(), s.db, &model.ApiKey{UserID: user.ID, Key: "sk-listbyapikey", Name: "k"})
	account := mustCreateAccount(s.T(), s.db, &model.Account{Name: "acc-listbyapikey"})

	s.createUsageLog(user, apiKey, account, 10, 20, 0.5, time.Now())
	s.createUsageLog(user, apiKey, account, 15, 25, 0.6, time.Now())

	logs, page, err := s.repo.ListByApiKey(s.ctx, apiKey.ID, pagination.PaginationParams{Page: 1, PageSize: 10})
	s.Require().NoError(err, "ListByApiKey")
	s.Require().Len(logs, 2)
	s.Require().Equal(int64(2), page.Total)
}

// --- ListByAccount ---

func (s *UsageLogRepoSuite) TestListByAccount() {
	user := mustCreateUser(s.T(), s.db, &model.User{Email: "listbyaccount@test.com"})
	apiKey := mustCreateApiKey(s.T(), s.db, &model.ApiKey{UserID: user.ID, Key: "sk-listbyaccount", Name: "k"})
	account := mustCreateAccount(s.T(), s.db, &model.Account{Name: "acc-listbyaccount"})

	s.createUsageLog(user, apiKey, account, 10, 20, 0.5, time.Now())

	logs, page, err := s.repo.ListByAccount(s.ctx, account.ID, pagination.PaginationParams{Page: 1, PageSize: 10})
	s.Require().NoError(err, "ListByAccount")
	s.Require().Len(logs, 1)
	s.Require().Equal(int64(1), page.Total)
}

// --- GetUserStats ---

func (s *UsageLogRepoSuite) TestGetUserStats() {
	user := mustCreateUser(s.T(), s.db, &model.User{Email: "userstats@test.com"})
	apiKey := mustCreateApiKey(s.T(), s.db, &model.ApiKey{UserID: user.ID, Key: "sk-userstats", Name: "k"})
	account := mustCreateAccount(s.T(), s.db, &model.Account{Name: "acc-userstats"})

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
}

// --- ListWithFilters ---

func (s *UsageLogRepoSuite) TestListWithFilters() {
	user := mustCreateUser(s.T(), s.db, &model.User{Email: "filters@test.com"})
	apiKey := mustCreateApiKey(s.T(), s.db, &model.ApiKey{UserID: user.ID, Key: "sk-filters", Name: "k"})
	account := mustCreateAccount(s.T(), s.db, &model.Account{Name: "acc-filters"})

	s.createUsageLog(user, apiKey, account, 10, 20, 0.5, time.Now())

	filters := usagestats.UsageLogFilters{UserID: user.ID}
	logs, page, err := s.repo.ListWithFilters(s.ctx, pagination.PaginationParams{Page: 1, PageSize: 10}, filters)
	s.Require().NoError(err, "ListWithFilters")
	s.Require().Len(logs, 1)
	s.Require().Equal(int64(1), page.Total)
}

// --- GetDashboardStats ---

func (s *UsageLogRepoSuite) TestDashboardStats_TodayTotalsAndPerformance() {
	now := time.Now()
	todayStart := timezone.Today()

	userToday := mustCreateUser(s.T(), s.db, &model.User{
		Email:     "today@example.com",
		CreatedAt: maxTime(todayStart.Add(10*time.Second), now.Add(-10*time.Second)),
		UpdatedAt: now,
	})
	userOld := mustCreateUser(s.T(), s.db, &model.User{
		Email:     "old@example.com",
		CreatedAt: todayStart.Add(-24 * time.Hour),
		UpdatedAt: todayStart.Add(-24 * time.Hour),
	})

	group := mustCreateGroup(s.T(), s.db, &model.Group{Name: "g-ul"})
	apiKey1 := mustCreateApiKey(s.T(), s.db, &model.ApiKey{UserID: userToday.ID, Key: "sk-ul-1", Name: "ul1"})
	mustCreateApiKey(s.T(), s.db, &model.ApiKey{UserID: userOld.ID, Key: "sk-ul-2", Name: "ul2", Status: model.StatusDisabled})

	resetAt := now.Add(10 * time.Minute)
	accNormal := mustCreateAccount(s.T(), s.db, &model.Account{Name: "a-normal", Schedulable: true})
	mustCreateAccount(s.T(), s.db, &model.Account{Name: "a-error", Status: model.StatusError, Schedulable: true})
	mustCreateAccount(s.T(), s.db, &model.Account{Name: "a-rl", RateLimitedAt: &now, RateLimitResetAt: &resetAt, Schedulable: true})
	mustCreateAccount(s.T(), s.db, &model.Account{Name: "a-ov", OverloadUntil: &resetAt, Schedulable: true})

	d1, d2, d3 := 100, 200, 300
	logToday := &model.UsageLog{
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
	}
	s.Require().NoError(s.repo.Create(s.ctx, logToday), "Create logToday")

	logOld := &model.UsageLog{
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
	}
	s.Require().NoError(s.repo.Create(s.ctx, logOld), "Create logOld")

	logPerf := &model.UsageLog{
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
	}
	s.Require().NoError(s.repo.Create(s.ctx, logPerf), "Create logPerf")

	stats, err := s.repo.GetDashboardStats(s.ctx)
	s.Require().NoError(err, "GetDashboardStats")

	s.Require().Equal(int64(2), stats.TotalUsers, "TotalUsers mismatch")
	s.Require().Equal(int64(1), stats.TodayNewUsers, "TodayNewUsers mismatch")
	s.Require().Equal(int64(1), stats.ActiveUsers, "ActiveUsers mismatch")
	s.Require().Equal(int64(2), stats.TotalApiKeys, "TotalApiKeys mismatch")
	s.Require().Equal(int64(1), stats.ActiveApiKeys, "ActiveApiKeys mismatch")
	s.Require().Equal(int64(4), stats.TotalAccounts, "TotalAccounts mismatch")
	s.Require().Equal(int64(1), stats.ErrorAccounts, "ErrorAccounts mismatch")
	s.Require().Equal(int64(1), stats.RateLimitAccounts, "RateLimitAccounts mismatch")
	s.Require().Equal(int64(1), stats.OverloadAccounts, "OverloadAccounts mismatch")

	s.Require().Equal(int64(3), stats.TotalRequests, "TotalRequests mismatch")
	s.Require().Equal(int64(16), stats.TotalInputTokens, "TotalInputTokens mismatch")
	s.Require().Equal(int64(28), stats.TotalOutputTokens, "TotalOutputTokens mismatch")
	s.Require().Equal(int64(3), stats.TotalCacheCreationTokens, "TotalCacheCreationTokens mismatch")
	s.Require().Equal(int64(4), stats.TotalCacheReadTokens, "TotalCacheReadTokens mismatch")
	s.Require().Equal(int64(51), stats.TotalTokens, "TotalTokens mismatch")
	s.Require().Equal(2.3, stats.TotalCost, "TotalCost mismatch")
	s.Require().Equal(2.0, stats.TotalActualCost, "TotalActualCost mismatch")
	s.Require().GreaterOrEqual(stats.TodayRequests, int64(1), "expected TodayRequests >= 1")
	s.Require().GreaterOrEqual(stats.TodayCost, 0.0, "expected TodayCost >= 0")

	wantRpm, wantTpm := s.repo.getPerformanceStats(s.ctx, 0)
	s.Require().Equal(wantRpm, stats.Rpm, "Rpm mismatch")
	s.Require().Equal(wantTpm, stats.Tpm, "Tpm mismatch")
}

// --- GetUserDashboardStats ---

func (s *UsageLogRepoSuite) TestGetUserDashboardStats() {
	user := mustCreateUser(s.T(), s.db, &model.User{Email: "userdash@test.com"})
	apiKey := mustCreateApiKey(s.T(), s.db, &model.ApiKey{UserID: user.ID, Key: "sk-userdash", Name: "k"})
	account := mustCreateAccount(s.T(), s.db, &model.Account{Name: "acc-userdash"})

	s.createUsageLog(user, apiKey, account, 10, 20, 0.5, time.Now())

	stats, err := s.repo.GetUserDashboardStats(s.ctx, user.ID)
	s.Require().NoError(err, "GetUserDashboardStats")
	s.Require().Equal(int64(1), stats.TotalApiKeys)
	s.Require().Equal(int64(1), stats.TotalRequests)
}

// --- GetAccountTodayStats ---

func (s *UsageLogRepoSuite) TestGetAccountTodayStats() {
	user := mustCreateUser(s.T(), s.db, &model.User{Email: "acctoday@test.com"})
	apiKey := mustCreateApiKey(s.T(), s.db, &model.ApiKey{UserID: user.ID, Key: "sk-acctoday", Name: "k"})
	account := mustCreateAccount(s.T(), s.db, &model.Account{Name: "acc-today"})

	s.createUsageLog(user, apiKey, account, 10, 20, 0.5, time.Now())

	stats, err := s.repo.GetAccountTodayStats(s.ctx, account.ID)
	s.Require().NoError(err, "GetAccountTodayStats")
	s.Require().Equal(int64(1), stats.Requests)
	s.Require().Equal(int64(30), stats.Tokens)
}

// --- GetBatchUserUsageStats ---

func (s *UsageLogRepoSuite) TestGetBatchUserUsageStats() {
	user1 := mustCreateUser(s.T(), s.db, &model.User{Email: "batch1@test.com"})
	user2 := mustCreateUser(s.T(), s.db, &model.User{Email: "batch2@test.com"})
	apiKey1 := mustCreateApiKey(s.T(), s.db, &model.ApiKey{UserID: user1.ID, Key: "sk-batch1", Name: "k"})
	apiKey2 := mustCreateApiKey(s.T(), s.db, &model.ApiKey{UserID: user2.ID, Key: "sk-batch2", Name: "k"})
	account := mustCreateAccount(s.T(), s.db, &model.Account{Name: "acc-batch"})

	s.createUsageLog(user1, apiKey1, account, 10, 20, 0.5, time.Now())
	s.createUsageLog(user2, apiKey2, account, 15, 25, 0.6, time.Now())

	stats, err := s.repo.GetBatchUserUsageStats(s.ctx, []int64{user1.ID, user2.ID})
	s.Require().NoError(err, "GetBatchUserUsageStats")
	s.Require().Len(stats, 2)
	s.Require().NotNil(stats[user1.ID])
	s.Require().NotNil(stats[user2.ID])
}

func (s *UsageLogRepoSuite) TestGetBatchUserUsageStats_Empty() {
	stats, err := s.repo.GetBatchUserUsageStats(s.ctx, []int64{})
	s.Require().NoError(err)
	s.Require().Empty(stats)
}

// --- GetBatchApiKeyUsageStats ---

func (s *UsageLogRepoSuite) TestGetBatchApiKeyUsageStats() {
	user := mustCreateUser(s.T(), s.db, &model.User{Email: "batchkey@test.com"})
	apiKey1 := mustCreateApiKey(s.T(), s.db, &model.ApiKey{UserID: user.ID, Key: "sk-batchkey1", Name: "k1"})
	apiKey2 := mustCreateApiKey(s.T(), s.db, &model.ApiKey{UserID: user.ID, Key: "sk-batchkey2", Name: "k2"})
	account := mustCreateAccount(s.T(), s.db, &model.Account{Name: "acc-batchkey"})

	s.createUsageLog(user, apiKey1, account, 10, 20, 0.5, time.Now())
	s.createUsageLog(user, apiKey2, account, 15, 25, 0.6, time.Now())

	stats, err := s.repo.GetBatchApiKeyUsageStats(s.ctx, []int64{apiKey1.ID, apiKey2.ID})
	s.Require().NoError(err, "GetBatchApiKeyUsageStats")
	s.Require().Len(stats, 2)
}

func (s *UsageLogRepoSuite) TestGetBatchApiKeyUsageStats_Empty() {
	stats, err := s.repo.GetBatchApiKeyUsageStats(s.ctx, []int64{})
	s.Require().NoError(err)
	s.Require().Empty(stats)
}

// --- GetGlobalStats ---

func (s *UsageLogRepoSuite) TestGetGlobalStats() {
	user := mustCreateUser(s.T(), s.db, &model.User{Email: "global@test.com"})
	apiKey := mustCreateApiKey(s.T(), s.db, &model.ApiKey{UserID: user.ID, Key: "sk-global", Name: "k"})
	account := mustCreateAccount(s.T(), s.db, &model.Account{Name: "acc-global"})

	base := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	s.createUsageLog(user, apiKey, account, 10, 20, 0.5, base)
	s.createUsageLog(user, apiKey, account, 15, 25, 0.6, base.Add(1*time.Hour))

	stats, err := s.repo.GetGlobalStats(s.ctx, base.Add(-1*time.Hour), base.Add(2*time.Hour))
	s.Require().NoError(err, "GetGlobalStats")
	s.Require().Equal(int64(2), stats.TotalRequests)
	s.Require().Equal(int64(25), stats.TotalInputTokens)
	s.Require().Equal(int64(45), stats.TotalOutputTokens)
}

func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

// --- ListByUserAndTimeRange ---

func (s *UsageLogRepoSuite) TestListByUserAndTimeRange() {
	user := mustCreateUser(s.T(), s.db, &model.User{Email: "timerange@test.com"})
	apiKey := mustCreateApiKey(s.T(), s.db, &model.ApiKey{UserID: user.ID, Key: "sk-timerange", Name: "k"})
	account := mustCreateAccount(s.T(), s.db, &model.Account{Name: "acc-timerange"})

	base := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	s.createUsageLog(user, apiKey, account, 10, 20, 0.5, base)
	s.createUsageLog(user, apiKey, account, 15, 25, 0.6, base.Add(1*time.Hour))
	s.createUsageLog(user, apiKey, account, 20, 30, 0.7, base.Add(-24*time.Hour)) // outside range

	startTime := base.Add(-1 * time.Hour)
	endTime := base.Add(2 * time.Hour)
	logs, _, err := s.repo.ListByUserAndTimeRange(s.ctx, user.ID, startTime, endTime)
	s.Require().NoError(err, "ListByUserAndTimeRange")
	s.Require().Len(logs, 2)
}

// --- ListByApiKeyAndTimeRange ---

func (s *UsageLogRepoSuite) TestListByApiKeyAndTimeRange() {
	user := mustCreateUser(s.T(), s.db, &model.User{Email: "keytimerange@test.com"})
	apiKey := mustCreateApiKey(s.T(), s.db, &model.ApiKey{UserID: user.ID, Key: "sk-keytimerange", Name: "k"})
	account := mustCreateAccount(s.T(), s.db, &model.Account{Name: "acc-keytimerange"})

	base := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	s.createUsageLog(user, apiKey, account, 10, 20, 0.5, base)
	s.createUsageLog(user, apiKey, account, 15, 25, 0.6, base.Add(30*time.Minute))
	s.createUsageLog(user, apiKey, account, 20, 30, 0.7, base.Add(-24*time.Hour)) // outside range

	startTime := base.Add(-1 * time.Hour)
	endTime := base.Add(2 * time.Hour)
	logs, _, err := s.repo.ListByApiKeyAndTimeRange(s.ctx, apiKey.ID, startTime, endTime)
	s.Require().NoError(err, "ListByApiKeyAndTimeRange")
	s.Require().Len(logs, 2)
}

// --- ListByAccountAndTimeRange ---

func (s *UsageLogRepoSuite) TestListByAccountAndTimeRange() {
	user := mustCreateUser(s.T(), s.db, &model.User{Email: "acctimerange@test.com"})
	apiKey := mustCreateApiKey(s.T(), s.db, &model.ApiKey{UserID: user.ID, Key: "sk-acctimerange", Name: "k"})
	account := mustCreateAccount(s.T(), s.db, &model.Account{Name: "acc-acctimerange"})

	base := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	s.createUsageLog(user, apiKey, account, 10, 20, 0.5, base)
	s.createUsageLog(user, apiKey, account, 15, 25, 0.6, base.Add(45*time.Minute))
	s.createUsageLog(user, apiKey, account, 20, 30, 0.7, base.Add(-24*time.Hour)) // outside range

	startTime := base.Add(-1 * time.Hour)
	endTime := base.Add(2 * time.Hour)
	logs, _, err := s.repo.ListByAccountAndTimeRange(s.ctx, account.ID, startTime, endTime)
	s.Require().NoError(err, "ListByAccountAndTimeRange")
	s.Require().Len(logs, 2)
}

// --- ListByModelAndTimeRange ---

func (s *UsageLogRepoSuite) TestListByModelAndTimeRange() {
	user := mustCreateUser(s.T(), s.db, &model.User{Email: "modeltimerange@test.com"})
	apiKey := mustCreateApiKey(s.T(), s.db, &model.ApiKey{UserID: user.ID, Key: "sk-modeltimerange", Name: "k"})
	account := mustCreateAccount(s.T(), s.db, &model.Account{Name: "acc-modeltimerange"})

	base := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)

	// Create logs with different models
	log1 := &model.UsageLog{
		UserID:       user.ID,
		ApiKeyID:     apiKey.ID,
		AccountID:    account.ID,
		Model:        "claude-3-opus",
		InputTokens:  10,
		OutputTokens: 20,
		TotalCost:    0.5,
		ActualCost:   0.5,
		CreatedAt:    base,
	}
	s.Require().NoError(s.repo.Create(s.ctx, log1))

	log2 := &model.UsageLog{
		UserID:       user.ID,
		ApiKeyID:     apiKey.ID,
		AccountID:    account.ID,
		Model:        "claude-3-opus",
		InputTokens:  15,
		OutputTokens: 25,
		TotalCost:    0.6,
		ActualCost:   0.6,
		CreatedAt:    base.Add(30 * time.Minute),
	}
	s.Require().NoError(s.repo.Create(s.ctx, log2))

	log3 := &model.UsageLog{
		UserID:       user.ID,
		ApiKeyID:     apiKey.ID,
		AccountID:    account.ID,
		Model:        "claude-3-sonnet",
		InputTokens:  20,
		OutputTokens: 30,
		TotalCost:    0.7,
		ActualCost:   0.7,
		CreatedAt:    base.Add(1 * time.Hour),
	}
	s.Require().NoError(s.repo.Create(s.ctx, log3))

	startTime := base.Add(-1 * time.Hour)
	endTime := base.Add(2 * time.Hour)
	logs, _, err := s.repo.ListByModelAndTimeRange(s.ctx, "claude-3-opus", startTime, endTime)
	s.Require().NoError(err, "ListByModelAndTimeRange")
	s.Require().Len(logs, 2)
}

// --- GetAccountWindowStats ---

func (s *UsageLogRepoSuite) TestGetAccountWindowStats() {
	user := mustCreateUser(s.T(), s.db, &model.User{Email: "windowstats@test.com"})
	apiKey := mustCreateApiKey(s.T(), s.db, &model.ApiKey{UserID: user.ID, Key: "sk-windowstats", Name: "k"})
	account := mustCreateAccount(s.T(), s.db, &model.Account{Name: "acc-windowstats"})

	now := time.Now()
	windowStart := now.Add(-10 * time.Minute)

	s.createUsageLog(user, apiKey, account, 10, 20, 0.5, now.Add(-5*time.Minute))
	s.createUsageLog(user, apiKey, account, 15, 25, 0.6, now.Add(-3*time.Minute))
	s.createUsageLog(user, apiKey, account, 20, 30, 0.7, now.Add(-30*time.Minute)) // outside window

	stats, err := s.repo.GetAccountWindowStats(s.ctx, account.ID, windowStart)
	s.Require().NoError(err, "GetAccountWindowStats")
	s.Require().Equal(int64(2), stats.Requests)
	s.Require().Equal(int64(70), stats.Tokens) // (10+20) + (15+25)
}

// --- GetUserUsageTrendByUserID ---

func (s *UsageLogRepoSuite) TestGetUserUsageTrendByUserID() {
	user := mustCreateUser(s.T(), s.db, &model.User{Email: "usertrend@test.com"})
	apiKey := mustCreateApiKey(s.T(), s.db, &model.ApiKey{UserID: user.ID, Key: "sk-usertrend", Name: "k"})
	account := mustCreateAccount(s.T(), s.db, &model.Account{Name: "acc-usertrend"})

	base := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	s.createUsageLog(user, apiKey, account, 10, 20, 0.5, base)
	s.createUsageLog(user, apiKey, account, 15, 25, 0.6, base.Add(1*time.Hour))
	s.createUsageLog(user, apiKey, account, 20, 30, 0.7, base.Add(24*time.Hour)) // next day

	startTime := base.Add(-1 * time.Hour)
	endTime := base.Add(48 * time.Hour)
	trend, err := s.repo.GetUserUsageTrendByUserID(s.ctx, user.ID, startTime, endTime, "day")
	s.Require().NoError(err, "GetUserUsageTrendByUserID")
	s.Require().Len(trend, 2) // 2 different days
}

func (s *UsageLogRepoSuite) TestGetUserUsageTrendByUserID_HourlyGranularity() {
	user := mustCreateUser(s.T(), s.db, &model.User{Email: "usertrendhourly@test.com"})
	apiKey := mustCreateApiKey(s.T(), s.db, &model.ApiKey{UserID: user.ID, Key: "sk-usertrendhourly", Name: "k"})
	account := mustCreateAccount(s.T(), s.db, &model.Account{Name: "acc-usertrendhourly"})

	base := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	s.createUsageLog(user, apiKey, account, 10, 20, 0.5, base)
	s.createUsageLog(user, apiKey, account, 15, 25, 0.6, base.Add(1*time.Hour))
	s.createUsageLog(user, apiKey, account, 20, 30, 0.7, base.Add(2*time.Hour))

	startTime := base.Add(-1 * time.Hour)
	endTime := base.Add(3 * time.Hour)
	trend, err := s.repo.GetUserUsageTrendByUserID(s.ctx, user.ID, startTime, endTime, "hour")
	s.Require().NoError(err, "GetUserUsageTrendByUserID hourly")
	s.Require().Len(trend, 3) // 3 different hours
}

// --- GetUserModelStats ---

func (s *UsageLogRepoSuite) TestGetUserModelStats() {
	user := mustCreateUser(s.T(), s.db, &model.User{Email: "modelstats@test.com"})
	apiKey := mustCreateApiKey(s.T(), s.db, &model.ApiKey{UserID: user.ID, Key: "sk-modelstats", Name: "k"})
	account := mustCreateAccount(s.T(), s.db, &model.Account{Name: "acc-modelstats"})

	base := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)

	// Create logs with different models
	log1 := &model.UsageLog{
		UserID:       user.ID,
		ApiKeyID:     apiKey.ID,
		AccountID:    account.ID,
		Model:        "claude-3-opus",
		InputTokens:  100,
		OutputTokens: 200,
		TotalCost:    0.5,
		ActualCost:   0.5,
		CreatedAt:    base,
	}
	s.Require().NoError(s.repo.Create(s.ctx, log1))

	log2 := &model.UsageLog{
		UserID:       user.ID,
		ApiKeyID:     apiKey.ID,
		AccountID:    account.ID,
		Model:        "claude-3-sonnet",
		InputTokens:  50,
		OutputTokens: 100,
		TotalCost:    0.2,
		ActualCost:   0.2,
		CreatedAt:    base.Add(1 * time.Hour),
	}
	s.Require().NoError(s.repo.Create(s.ctx, log2))

	startTime := base.Add(-1 * time.Hour)
	endTime := base.Add(2 * time.Hour)
	stats, err := s.repo.GetUserModelStats(s.ctx, user.ID, startTime, endTime)
	s.Require().NoError(err, "GetUserModelStats")
	s.Require().Len(stats, 2)

	// Should be ordered by total_tokens DESC
	s.Require().Equal("claude-3-opus", stats[0].Model)
	s.Require().Equal(int64(300), stats[0].TotalTokens)
}

// --- GetUsageTrendWithFilters ---

func (s *UsageLogRepoSuite) TestGetUsageTrendWithFilters() {
	user := mustCreateUser(s.T(), s.db, &model.User{Email: "trendfilters@test.com"})
	apiKey := mustCreateApiKey(s.T(), s.db, &model.ApiKey{UserID: user.ID, Key: "sk-trendfilters", Name: "k"})
	account := mustCreateAccount(s.T(), s.db, &model.Account{Name: "acc-trendfilters"})

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
}

func (s *UsageLogRepoSuite) TestGetUsageTrendWithFilters_HourlyGranularity() {
	user := mustCreateUser(s.T(), s.db, &model.User{Email: "trendfilters-h@test.com"})
	apiKey := mustCreateApiKey(s.T(), s.db, &model.ApiKey{UserID: user.ID, Key: "sk-trendfilters-h", Name: "k"})
	account := mustCreateAccount(s.T(), s.db, &model.Account{Name: "acc-trendfilters-h"})

	base := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	s.createUsageLog(user, apiKey, account, 10, 20, 0.5, base)
	s.createUsageLog(user, apiKey, account, 15, 25, 0.6, base.Add(1*time.Hour))

	startTime := base.Add(-1 * time.Hour)
	endTime := base.Add(3 * time.Hour)

	trend, err := s.repo.GetUsageTrendWithFilters(s.ctx, startTime, endTime, "hour", user.ID, 0)
	s.Require().NoError(err, "GetUsageTrendWithFilters hourly")
	s.Require().Len(trend, 2)
}

// --- GetModelStatsWithFilters ---

func (s *UsageLogRepoSuite) TestGetModelStatsWithFilters() {
	user := mustCreateUser(s.T(), s.db, &model.User{Email: "modelfilters@test.com"})
	apiKey := mustCreateApiKey(s.T(), s.db, &model.ApiKey{UserID: user.ID, Key: "sk-modelfilters", Name: "k"})
	account := mustCreateAccount(s.T(), s.db, &model.Account{Name: "acc-modelfilters"})

	base := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)

	log1 := &model.UsageLog{
		UserID:       user.ID,
		ApiKeyID:     apiKey.ID,
		AccountID:    account.ID,
		Model:        "claude-3-opus",
		InputTokens:  100,
		OutputTokens: 200,
		TotalCost:    0.5,
		ActualCost:   0.5,
		CreatedAt:    base,
	}
	s.Require().NoError(s.repo.Create(s.ctx, log1))

	log2 := &model.UsageLog{
		UserID:       user.ID,
		ApiKeyID:     apiKey.ID,
		AccountID:    account.ID,
		Model:        "claude-3-sonnet",
		InputTokens:  50,
		OutputTokens: 100,
		TotalCost:    0.2,
		ActualCost:   0.2,
		CreatedAt:    base.Add(1 * time.Hour),
	}
	s.Require().NoError(s.repo.Create(s.ctx, log2))

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
}

// --- GetAccountUsageStats ---

func (s *UsageLogRepoSuite) TestGetAccountUsageStats() {
	user := mustCreateUser(s.T(), s.db, &model.User{Email: "accstats@test.com"})
	apiKey := mustCreateApiKey(s.T(), s.db, &model.ApiKey{UserID: user.ID, Key: "sk-accstats", Name: "k"})
	account := mustCreateAccount(s.T(), s.db, &model.Account{Name: "acc-accstats"})

	base := time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC)

	// Create logs on different days
	log1 := &model.UsageLog{
		UserID:       user.ID,
		ApiKeyID:     apiKey.ID,
		AccountID:    account.ID,
		Model:        "claude-3-opus",
		InputTokens:  100,
		OutputTokens: 200,
		TotalCost:    0.5,
		ActualCost:   0.4,
		CreatedAt:    base.Add(12 * time.Hour),
	}
	s.Require().NoError(s.repo.Create(s.ctx, log1))

	log2 := &model.UsageLog{
		UserID:       user.ID,
		ApiKeyID:     apiKey.ID,
		AccountID:    account.ID,
		Model:        "claude-3-sonnet",
		InputTokens:  50,
		OutputTokens: 100,
		TotalCost:    0.2,
		ActualCost:   0.15,
		CreatedAt:    base.Add(36 * time.Hour), // next day
	}
	s.Require().NoError(s.repo.Create(s.ctx, log2))

	startTime := base
	endTime := base.Add(72 * time.Hour)

	resp, err := s.repo.GetAccountUsageStats(s.ctx, account.ID, startTime, endTime)
	s.Require().NoError(err, "GetAccountUsageStats")

	s.Require().Len(resp.History, 2, "expected 2 days of history")
	s.Require().Equal(int64(2), resp.Summary.TotalRequests)
	s.Require().Equal(int64(450), resp.Summary.TotalTokens)
	s.Require().Len(resp.Models, 2)
}

func (s *UsageLogRepoSuite) TestGetAccountUsageStats_EmptyRange() {
	account := mustCreateAccount(s.T(), s.db, &model.Account{Name: "acc-emptystats"})

	base := time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC)
	startTime := base
	endTime := base.Add(72 * time.Hour)

	resp, err := s.repo.GetAccountUsageStats(s.ctx, account.ID, startTime, endTime)
	s.Require().NoError(err, "GetAccountUsageStats empty")

	s.Require().Len(resp.History, 0)
	s.Require().Equal(int64(0), resp.Summary.TotalRequests)
}

// --- GetUserUsageTrend ---

func (s *UsageLogRepoSuite) TestGetUserUsageTrend() {
	user1 := mustCreateUser(s.T(), s.db, &model.User{Email: "usertrend1@test.com"})
	user2 := mustCreateUser(s.T(), s.db, &model.User{Email: "usertrend2@test.com"})
	apiKey1 := mustCreateApiKey(s.T(), s.db, &model.ApiKey{UserID: user1.ID, Key: "sk-usertrend1", Name: "k1"})
	apiKey2 := mustCreateApiKey(s.T(), s.db, &model.ApiKey{UserID: user2.ID, Key: "sk-usertrend2", Name: "k2"})
	account := mustCreateAccount(s.T(), s.db, &model.Account{Name: "acc-usertrends"})

	base := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	s.createUsageLog(user1, apiKey1, account, 100, 200, 1.0, base)
	s.createUsageLog(user2, apiKey2, account, 50, 100, 0.5, base)
	s.createUsageLog(user1, apiKey1, account, 100, 200, 1.0, base.Add(24*time.Hour))

	startTime := base.Add(-1 * time.Hour)
	endTime := base.Add(48 * time.Hour)

	trend, err := s.repo.GetUserUsageTrend(s.ctx, startTime, endTime, "day", 10)
	s.Require().NoError(err, "GetUserUsageTrend")
	s.Require().GreaterOrEqual(len(trend), 2)
}

// --- GetApiKeyUsageTrend ---

func (s *UsageLogRepoSuite) TestGetApiKeyUsageTrend() {
	user := mustCreateUser(s.T(), s.db, &model.User{Email: "keytrend@test.com"})
	apiKey1 := mustCreateApiKey(s.T(), s.db, &model.ApiKey{UserID: user.ID, Key: "sk-keytrend1", Name: "k1"})
	apiKey2 := mustCreateApiKey(s.T(), s.db, &model.ApiKey{UserID: user.ID, Key: "sk-keytrend2", Name: "k2"})
	account := mustCreateAccount(s.T(), s.db, &model.Account{Name: "acc-keytrends"})

	base := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	s.createUsageLog(user, apiKey1, account, 100, 200, 1.0, base)
	s.createUsageLog(user, apiKey2, account, 50, 100, 0.5, base)
	s.createUsageLog(user, apiKey1, account, 100, 200, 1.0, base.Add(24*time.Hour))

	startTime := base.Add(-1 * time.Hour)
	endTime := base.Add(48 * time.Hour)

	trend, err := s.repo.GetApiKeyUsageTrend(s.ctx, startTime, endTime, "day", 10)
	s.Require().NoError(err, "GetApiKeyUsageTrend")
	s.Require().GreaterOrEqual(len(trend), 2)
}

func (s *UsageLogRepoSuite) TestGetApiKeyUsageTrend_HourlyGranularity() {
	user := mustCreateUser(s.T(), s.db, &model.User{Email: "keytrendh@test.com"})
	apiKey := mustCreateApiKey(s.T(), s.db, &model.ApiKey{UserID: user.ID, Key: "sk-keytrendh", Name: "k"})
	account := mustCreateAccount(s.T(), s.db, &model.Account{Name: "acc-keytrendh"})

	base := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	s.createUsageLog(user, apiKey, account, 100, 200, 1.0, base)
	s.createUsageLog(user, apiKey, account, 50, 100, 0.5, base.Add(1*time.Hour))

	startTime := base.Add(-1 * time.Hour)
	endTime := base.Add(3 * time.Hour)

	trend, err := s.repo.GetApiKeyUsageTrend(s.ctx, startTime, endTime, "hour", 10)
	s.Require().NoError(err, "GetApiKeyUsageTrend hourly")
	s.Require().Len(trend, 2)
}

// --- ListWithFilters (additional filter tests) ---

func (s *UsageLogRepoSuite) TestListWithFilters_ApiKeyFilter() {
	user := mustCreateUser(s.T(), s.db, &model.User{Email: "filterskey@test.com"})
	apiKey := mustCreateApiKey(s.T(), s.db, &model.ApiKey{UserID: user.ID, Key: "sk-filterskey", Name: "k"})
	account := mustCreateAccount(s.T(), s.db, &model.Account{Name: "acc-filterskey"})

	s.createUsageLog(user, apiKey, account, 10, 20, 0.5, time.Now())

	filters := usagestats.UsageLogFilters{ApiKeyID: apiKey.ID}
	logs, page, err := s.repo.ListWithFilters(s.ctx, pagination.PaginationParams{Page: 1, PageSize: 10}, filters)
	s.Require().NoError(err, "ListWithFilters apiKey")
	s.Require().Len(logs, 1)
	s.Require().Equal(int64(1), page.Total)
}

func (s *UsageLogRepoSuite) TestListWithFilters_TimeRange() {
	user := mustCreateUser(s.T(), s.db, &model.User{Email: "filterstime@test.com"})
	apiKey := mustCreateApiKey(s.T(), s.db, &model.ApiKey{UserID: user.ID, Key: "sk-filterstime", Name: "k"})
	account := mustCreateAccount(s.T(), s.db, &model.Account{Name: "acc-filterstime"})

	base := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	s.createUsageLog(user, apiKey, account, 10, 20, 0.5, base)
	s.createUsageLog(user, apiKey, account, 15, 25, 0.6, base.Add(1*time.Hour))
	s.createUsageLog(user, apiKey, account, 20, 30, 0.7, base.Add(-24*time.Hour)) // outside range

	startTime := base.Add(-1 * time.Hour)
	endTime := base.Add(2 * time.Hour)
	filters := usagestats.UsageLogFilters{StartTime: &startTime, EndTime: &endTime}
	logs, page, err := s.repo.ListWithFilters(s.ctx, pagination.PaginationParams{Page: 1, PageSize: 10}, filters)
	s.Require().NoError(err, "ListWithFilters time range")
	s.Require().Len(logs, 2)
	s.Require().Equal(int64(2), page.Total)
}

func (s *UsageLogRepoSuite) TestListWithFilters_CombinedFilters() {
	user := mustCreateUser(s.T(), s.db, &model.User{Email: "filterscombined@test.com"})
	apiKey := mustCreateApiKey(s.T(), s.db, &model.ApiKey{UserID: user.ID, Key: "sk-filterscombined", Name: "k"})
	account := mustCreateAccount(s.T(), s.db, &model.Account{Name: "acc-filterscombined"})

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
	}
	logs, page, err := s.repo.ListWithFilters(s.ctx, pagination.PaginationParams{Page: 1, PageSize: 10}, filters)
	s.Require().NoError(err, "ListWithFilters combined")
	s.Require().Len(logs, 2)
	s.Require().Equal(int64(2), page.Total)
}
