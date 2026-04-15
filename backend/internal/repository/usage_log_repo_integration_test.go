//go:build integration

package repository

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type UsageLogRepoSuite struct {
	suite.Suite
	ctx    context.Context
	tx     *dbent.Tx
	client *dbent.Client
	repo   *usageLogRepository
}

func (s *UsageLogRepoSuite) SetupTest() {
	s.ctx = context.Background()
	tx := testEntTx(s.T())
	s.tx = tx
	s.client = tx.Client()
	s.repo = newUsageLogRepositoryWithSQL(s.client, tx)
}

func TestUsageLogRepoSuite(t *testing.T) {
	suite.Run(t, new(UsageLogRepoSuite))
}

// truncateToDayUTC 截断到 UTC 日期边界（测试辅助函数）
func truncateToDayUTC(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

func (s *UsageLogRepoSuite) createUsageLog(user *service.User, apiKey *service.APIKey, account *service.Account, inputTokens, outputTokens int, cost float64, createdAt time.Time) *service.UsageLog {
	log := &service.UsageLog{
		UserID:       user.ID,
		APIKeyID:     apiKey.ID,
		AccountID:    account.ID,
		RequestID:    uuid.New().String(), // Generate unique RequestID for each log
		Model:        "claude-3",
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
		TotalCost:    cost,
		ActualCost:   cost,
		CreatedAt:    createdAt,
	}
	_, err := s.repo.Create(s.ctx, log)
	s.Require().NoError(err)
	return log
}

// --- Create / GetByID ---

func (s *UsageLogRepoSuite) TestCreate() {
	user := mustCreateUser(s.T(), s.client, &service.User{Email: "create@test.com"})
	apiKey := mustCreateApiKey(s.T(), s.client, &service.APIKey{UserID: user.ID, Key: "sk-create", Name: "k"})
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-create"})

	log := &service.UsageLog{
		UserID:       user.ID,
		APIKeyID:     apiKey.ID,
		AccountID:    account.ID,
		Model:        "claude-3",
		InputTokens:  10,
		OutputTokens: 20,
		TotalCost:    0.5,
		ActualCost:   0.4,
	}

	_, err := s.repo.Create(s.ctx, log)
	s.Require().NoError(err, "Create")
	s.Require().NotZero(log.ID)
}

func TestUsageLogRepositoryCreate_BatchPathConcurrent(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	repo := newUsageLogRepositoryWithSQL(client, integrationDB)

	user := mustCreateUser(t, client, &service.User{Email: fmt.Sprintf("usage-batch-%d@example.com", time.Now().UnixNano())})
	apiKey := mustCreateApiKey(t, client, &service.APIKey{UserID: user.ID, Key: "sk-usage-batch-" + uuid.NewString(), Name: "k"})
	account := mustCreateAccount(t, client, &service.Account{Name: "acc-usage-batch-" + uuid.NewString()})

	const total = 16
	results := make([]bool, total)
	errs := make([]error, total)
	logs := make([]*service.UsageLog, total)

	var wg sync.WaitGroup
	wg.Add(total)
	for i := 0; i < total; i++ {
		i := i
		logs[i] = &service.UsageLog{
			UserID:       user.ID,
			APIKeyID:     apiKey.ID,
			AccountID:    account.ID,
			RequestID:    uuid.NewString(),
			Model:        "claude-3",
			InputTokens:  10 + i,
			OutputTokens: 20 + i,
			TotalCost:    0.5,
			ActualCost:   0.5,
			CreatedAt:    time.Now().UTC(),
		}
		go func() {
			defer wg.Done()
			results[i], errs[i] = repo.Create(ctx, logs[i])
		}()
	}
	wg.Wait()

	for i := 0; i < total; i++ {
		require.NoError(t, errs[i])
		require.True(t, results[i])
		require.NotZero(t, logs[i].ID)
	}

	var count int
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM usage_logs WHERE api_key_id = $1", apiKey.ID).Scan(&count))
	require.Equal(t, total, count)
}

func TestUsageLogRepositoryCreate_BatchPathDuplicateRequestID(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	repo := newUsageLogRepositoryWithSQL(client, integrationDB)

	user := mustCreateUser(t, client, &service.User{Email: fmt.Sprintf("usage-dup-%d@example.com", time.Now().UnixNano())})
	apiKey := mustCreateApiKey(t, client, &service.APIKey{UserID: user.ID, Key: "sk-usage-dup-" + uuid.NewString(), Name: "k"})
	account := mustCreateAccount(t, client, &service.Account{Name: "acc-usage-dup-" + uuid.NewString()})
	requestID := uuid.NewString()

	log1 := &service.UsageLog{
		UserID:       user.ID,
		APIKeyID:     apiKey.ID,
		AccountID:    account.ID,
		RequestID:    requestID,
		Model:        "claude-3",
		InputTokens:  10,
		OutputTokens: 20,
		TotalCost:    0.5,
		ActualCost:   0.5,
		CreatedAt:    time.Now().UTC(),
	}
	log2 := &service.UsageLog{
		UserID:       user.ID,
		APIKeyID:     apiKey.ID,
		AccountID:    account.ID,
		RequestID:    requestID,
		Model:        "claude-3",
		InputTokens:  10,
		OutputTokens: 20,
		TotalCost:    0.5,
		ActualCost:   0.5,
		CreatedAt:    time.Now().UTC(),
	}

	inserted1, err1 := repo.Create(ctx, log1)
	inserted2, err2 := repo.Create(ctx, log2)
	require.NoError(t, err1)
	require.NoError(t, err2)
	require.True(t, inserted1)
	require.False(t, inserted2)
	require.Equal(t, log1.ID, log2.ID)

	var count int
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM usage_logs WHERE request_id = $1 AND api_key_id = $2", requestID, apiKey.ID).Scan(&count))
	require.Equal(t, 1, count)
}

func TestUsageLogRepositoryFlushCreateBatch_DeduplicatesSameKeyInMemory(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	repo := newUsageLogRepositoryWithSQL(client, integrationDB)

	user := mustCreateUser(t, client, &service.User{Email: fmt.Sprintf("usage-batch-memdup-%d@example.com", time.Now().UnixNano())})
	apiKey := mustCreateApiKey(t, client, &service.APIKey{UserID: user.ID, Key: "sk-usage-batch-memdup-" + uuid.NewString(), Name: "k"})
	account := mustCreateAccount(t, client, &service.Account{Name: "acc-usage-batch-memdup-" + uuid.NewString()})
	requestID := uuid.NewString()

	const total = 8
	batch := make([]usageLogCreateRequest, 0, total)
	logs := make([]*service.UsageLog, 0, total)

	for i := 0; i < total; i++ {
		log := &service.UsageLog{
			UserID:       user.ID,
			APIKeyID:     apiKey.ID,
			AccountID:    account.ID,
			RequestID:    requestID,
			Model:        "claude-3",
			InputTokens:  10 + i,
			OutputTokens: 20 + i,
			TotalCost:    0.5,
			ActualCost:   0.5,
			CreatedAt:    time.Now().UTC(),
		}
		logs = append(logs, log)
		batch = append(batch, usageLogCreateRequest{
			log:      log,
			prepared: prepareUsageLogInsert(log),
			resultCh: make(chan usageLogCreateResult, 1),
		})
	}

	repo.flushCreateBatch(integrationDB, batch)

	insertedCount := 0
	var firstID int64
	for idx, req := range batch {
		res := <-req.resultCh
		require.NoError(t, res.err)
		if res.inserted {
			insertedCount++
		}
		require.NotZero(t, logs[idx].ID)
		if idx == 0 {
			firstID = logs[idx].ID
		} else {
			require.Equal(t, firstID, logs[idx].ID)
		}
	}

	require.Equal(t, 1, insertedCount)

	var count int
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM usage_logs WHERE request_id = $1 AND api_key_id = $2", requestID, apiKey.ID).Scan(&count))
	require.Equal(t, 1, count)
}

func TestUsageLogRepositoryCreateBestEffort_BatchPathDuplicateRequestID(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	repo := newUsageLogRepositoryWithSQL(client, integrationDB)

	user := mustCreateUser(t, client, &service.User{Email: fmt.Sprintf("usage-best-effort-dup-%d@example.com", time.Now().UnixNano())})
	apiKey := mustCreateApiKey(t, client, &service.APIKey{UserID: user.ID, Key: "sk-usage-best-effort-dup-" + uuid.NewString(), Name: "k"})
	account := mustCreateAccount(t, client, &service.Account{Name: "acc-usage-best-effort-dup-" + uuid.NewString()})
	requestID := uuid.NewString()

	log1 := &service.UsageLog{
		UserID:       user.ID,
		APIKeyID:     apiKey.ID,
		AccountID:    account.ID,
		RequestID:    requestID,
		Model:        "claude-3",
		InputTokens:  10,
		OutputTokens: 20,
		TotalCost:    0.5,
		ActualCost:   0.5,
		CreatedAt:    time.Now().UTC(),
	}
	log2 := &service.UsageLog{
		UserID:       user.ID,
		APIKeyID:     apiKey.ID,
		AccountID:    account.ID,
		RequestID:    requestID,
		Model:        "claude-3",
		InputTokens:  10,
		OutputTokens: 20,
		TotalCost:    0.5,
		ActualCost:   0.5,
		CreatedAt:    time.Now().UTC(),
	}

	require.NoError(t, repo.CreateBestEffort(ctx, log1))
	require.NoError(t, repo.CreateBestEffort(ctx, log2))

	require.Eventually(t, func() bool {
		var count int
		err := integrationDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM usage_logs WHERE request_id = $1 AND api_key_id = $2", requestID, apiKey.ID).Scan(&count)
		return err == nil && count == 1
	}, 3*time.Second, 20*time.Millisecond)
}

func TestUsageLogRepositoryCreateBestEffort_QueueFullReturnsDropped(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	repo := newUsageLogRepositoryWithSQL(client, integrationDB)
	repo.bestEffortBatchCh = make(chan usageLogBestEffortRequest, 1)
	repo.bestEffortBatchCh <- usageLogBestEffortRequest{}

	user := mustCreateUser(t, client, &service.User{Email: fmt.Sprintf("usage-best-effort-full-%d@example.com", time.Now().UnixNano())})
	apiKey := mustCreateApiKey(t, client, &service.APIKey{UserID: user.ID, Key: "sk-usage-best-effort-full-" + uuid.NewString(), Name: "k"})
	account := mustCreateAccount(t, client, &service.Account{Name: "acc-usage-best-effort-full-" + uuid.NewString()})

	err := repo.CreateBestEffort(ctx, &service.UsageLog{
		UserID:       user.ID,
		APIKeyID:     apiKey.ID,
		AccountID:    account.ID,
		RequestID:    uuid.NewString(),
		Model:        "claude-3",
		InputTokens:  10,
		OutputTokens: 20,
		TotalCost:    0.5,
		ActualCost:   0.5,
		CreatedAt:    time.Now().UTC(),
	})

	require.Error(t, err)
	require.True(t, service.IsUsageLogCreateDropped(err))
}

func TestUsageLogRepositoryCreate_BatchPathCanceledContextMarksNotPersisted(t *testing.T) {
	client := testEntClient(t)
	repo := newUsageLogRepositoryWithSQL(client, integrationDB)

	user := mustCreateUser(t, client, &service.User{Email: fmt.Sprintf("usage-cancel-%d@example.com", time.Now().UnixNano())})
	apiKey := mustCreateApiKey(t, client, &service.APIKey{UserID: user.ID, Key: "sk-usage-cancel-" + uuid.NewString(), Name: "k"})
	account := mustCreateAccount(t, client, &service.Account{Name: "acc-usage-cancel-" + uuid.NewString()})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	inserted, err := repo.Create(ctx, &service.UsageLog{
		UserID:       user.ID,
		APIKeyID:     apiKey.ID,
		AccountID:    account.ID,
		RequestID:    uuid.NewString(),
		Model:        "claude-3",
		InputTokens:  10,
		OutputTokens: 20,
		TotalCost:    0.5,
		ActualCost:   0.5,
		CreatedAt:    time.Now().UTC(),
	})

	require.False(t, inserted)
	require.Error(t, err)
	require.True(t, service.IsUsageLogCreateNotPersisted(err))
}

func TestUsageLogRepositoryCreate_BatchPathQueueFullMarksNotPersisted(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	repo := newUsageLogRepositoryWithSQL(client, integrationDB)
	repo.createBatchCh = make(chan usageLogCreateRequest, 1)
	repo.createBatchCh <- usageLogCreateRequest{}

	user := mustCreateUser(t, client, &service.User{Email: fmt.Sprintf("usage-create-full-%d@example.com", time.Now().UnixNano())})
	apiKey := mustCreateApiKey(t, client, &service.APIKey{UserID: user.ID, Key: "sk-usage-create-full-" + uuid.NewString(), Name: "k"})
	account := mustCreateAccount(t, client, &service.Account{Name: "acc-usage-create-full-" + uuid.NewString()})

	inserted, err := repo.Create(ctx, &service.UsageLog{
		UserID:       user.ID,
		APIKeyID:     apiKey.ID,
		AccountID:    account.ID,
		RequestID:    uuid.NewString(),
		Model:        "claude-3",
		InputTokens:  10,
		OutputTokens: 20,
		TotalCost:    0.5,
		ActualCost:   0.5,
		CreatedAt:    time.Now().UTC(),
	})

	require.False(t, inserted)
	require.Error(t, err)
	require.True(t, service.IsUsageLogCreateNotPersisted(err))
}

func TestUsageLogRepositoryCreate_BatchPathCanceledAfterQueueMarksNotPersisted(t *testing.T) {
	client := testEntClient(t)
	repo := newUsageLogRepositoryWithSQL(client, integrationDB)
	repo.createBatchCh = make(chan usageLogCreateRequest, 1)

	user := mustCreateUser(t, client, &service.User{Email: fmt.Sprintf("usage-cancel-queued-%d@example.com", time.Now().UnixNano())})
	apiKey := mustCreateApiKey(t, client, &service.APIKey{UserID: user.ID, Key: "sk-usage-cancel-queued-" + uuid.NewString(), Name: "k"})
	account := mustCreateAccount(t, client, &service.Account{Name: "acc-usage-cancel-queued-" + uuid.NewString()})

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)

	go func() {
		_, err := repo.createBatched(ctx, &service.UsageLog{
			UserID:       user.ID,
			APIKeyID:     apiKey.ID,
			AccountID:    account.ID,
			RequestID:    uuid.NewString(),
			Model:        "claude-3",
			InputTokens:  10,
			OutputTokens: 20,
			TotalCost:    0.5,
			ActualCost:   0.5,
			CreatedAt:    time.Now().UTC(),
		})
		errCh <- err
	}()

	req := <-repo.createBatchCh
	require.NotNil(t, req.shared)
	cancel()

	err := <-errCh
	require.Error(t, err)
	require.True(t, service.IsUsageLogCreateNotPersisted(err))
	completeUsageLogCreateRequest(req, usageLogCreateResult{inserted: false, err: service.MarkUsageLogCreateNotPersisted(context.Canceled)})
}

func TestUsageLogRepositoryFlushCreateBatch_CanceledRequestReturnsNotPersisted(t *testing.T) {
	client := testEntClient(t)
	repo := newUsageLogRepositoryWithSQL(client, integrationDB)

	user := mustCreateUser(t, client, &service.User{Email: fmt.Sprintf("usage-flush-cancel-%d@example.com", time.Now().UnixNano())})
	apiKey := mustCreateApiKey(t, client, &service.APIKey{UserID: user.ID, Key: "sk-usage-flush-cancel-" + uuid.NewString(), Name: "k"})
	account := mustCreateAccount(t, client, &service.Account{Name: "acc-usage-flush-cancel-" + uuid.NewString()})

	log := &service.UsageLog{
		UserID:       user.ID,
		APIKeyID:     apiKey.ID,
		AccountID:    account.ID,
		RequestID:    uuid.NewString(),
		Model:        "claude-3",
		InputTokens:  10,
		OutputTokens: 20,
		TotalCost:    0.5,
		ActualCost:   0.5,
		CreatedAt:    time.Now().UTC(),
	}
	req := usageLogCreateRequest{
		log:      log,
		prepared: prepareUsageLogInsert(log),
		shared:   &usageLogCreateShared{},
		resultCh: make(chan usageLogCreateResult, 1),
	}
	req.shared.state.Store(usageLogCreateStateCanceled)

	repo.flushCreateBatch(integrationDB, []usageLogCreateRequest{req})

	res := <-req.resultCh
	require.False(t, res.inserted)
	require.Error(t, res.err)
	require.True(t, service.IsUsageLogCreateNotPersisted(res.err))
}

func (s *UsageLogRepoSuite) TestGetByID() {
	user := mustCreateUser(s.T(), s.client, &service.User{Email: "getbyid@test.com"})
	apiKey := mustCreateApiKey(s.T(), s.client, &service.APIKey{UserID: user.ID, Key: "sk-getbyid", Name: "k"})
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-getbyid"})

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

func (s *UsageLogRepoSuite) TestGetByID_ReturnsAccountRateMultiplier() {
	user := mustCreateUser(s.T(), s.client, &service.User{Email: "getbyid-mult@test.com"})
	apiKey := mustCreateApiKey(s.T(), s.client, &service.APIKey{UserID: user.ID, Key: "sk-getbyid-mult", Name: "k"})
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-getbyid-mult"})

	m := 0.5
	log := &service.UsageLog{
		UserID:                user.ID,
		APIKeyID:              apiKey.ID,
		AccountID:             account.ID,
		RequestID:             uuid.New().String(),
		Model:                 "claude-3",
		InputTokens:           10,
		OutputTokens:          20,
		TotalCost:             1.0,
		ActualCost:            2.0,
		AccountRateMultiplier: &m,
		CreatedAt:             timezone.Today().Add(2 * time.Hour),
	}
	_, err := s.repo.Create(s.ctx, log)
	s.Require().NoError(err)

	got, err := s.repo.GetByID(s.ctx, log.ID)
	s.Require().NoError(err)
	s.Require().NotNil(got.AccountRateMultiplier)
	s.Require().InEpsilon(0.5, *got.AccountRateMultiplier, 0.0001)
}

func (s *UsageLogRepoSuite) TestGetByID_ReturnsOpenAIWSMode() {
	user := mustCreateUser(s.T(), s.client, &service.User{Email: "getbyid-ws@test.com"})
	apiKey := mustCreateApiKey(s.T(), s.client, &service.APIKey{UserID: user.ID, Key: "sk-getbyid-ws", Name: "k"})
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-getbyid-ws"})

	log := &service.UsageLog{
		UserID:       user.ID,
		APIKeyID:     apiKey.ID,
		AccountID:    account.ID,
		RequestID:    uuid.New().String(),
		Model:        "gpt-5.3-codex",
		InputTokens:  10,
		OutputTokens: 20,
		TotalCost:    1.0,
		ActualCost:   1.0,
		OpenAIWSMode: true,
		CreatedAt:    timezone.Today().Add(3 * time.Hour),
	}
	_, err := s.repo.Create(s.ctx, log)
	s.Require().NoError(err)

	got, err := s.repo.GetByID(s.ctx, log.ID)
	s.Require().NoError(err)
	s.Require().True(got.OpenAIWSMode)
}

func (s *UsageLogRepoSuite) TestGetByID_ReturnsRequestTypeAndLegacyFallback() {
	user := mustCreateUser(s.T(), s.client, &service.User{Email: "getbyid-request-type@test.com"})
	apiKey := mustCreateApiKey(s.T(), s.client, &service.APIKey{UserID: user.ID, Key: "sk-getbyid-request-type", Name: "k"})
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-getbyid-request-type"})

	log := &service.UsageLog{
		UserID:       user.ID,
		APIKeyID:     apiKey.ID,
		AccountID:    account.ID,
		RequestID:    uuid.New().String(),
		Model:        "gpt-5.3-codex",
		RequestType:  service.RequestTypeWSV2,
		Stream:       true,
		OpenAIWSMode: false,
		InputTokens:  10,
		OutputTokens: 20,
		TotalCost:    1.0,
		ActualCost:   1.0,
		CreatedAt:    timezone.Today().Add(4 * time.Hour),
	}
	_, err := s.repo.Create(s.ctx, log)
	s.Require().NoError(err)

	got, err := s.repo.GetByID(s.ctx, log.ID)
	s.Require().NoError(err)
	s.Require().Equal(service.RequestTypeWSV2, got.RequestType)
	s.Require().True(got.Stream)
	s.Require().True(got.OpenAIWSMode)
}

// --- Delete ---

func (s *UsageLogRepoSuite) TestDelete() {
	user := mustCreateUser(s.T(), s.client, &service.User{Email: "delete@test.com"})
	apiKey := mustCreateApiKey(s.T(), s.client, &service.APIKey{UserID: user.ID, Key: "sk-delete", Name: "k"})
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-delete"})

	log := s.createUsageLog(user, apiKey, account, 10, 20, 0.5, time.Now())

	err := s.repo.Delete(s.ctx, log.ID)
	s.Require().NoError(err, "Delete")

	_, err = s.repo.GetByID(s.ctx, log.ID)
	s.Require().Error(err, "expected error after delete")
}

// --- ListByUser ---

func (s *UsageLogRepoSuite) TestListByUser() {
	user := mustCreateUser(s.T(), s.client, &service.User{Email: "listbyuser@test.com"})
	apiKey := mustCreateApiKey(s.T(), s.client, &service.APIKey{UserID: user.ID, Key: "sk-listbyuser", Name: "k"})
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-listbyuser"})

	s.createUsageLog(user, apiKey, account, 10, 20, 0.5, time.Now())
	s.createUsageLog(user, apiKey, account, 15, 25, 0.6, time.Now())

	logs, page, err := s.repo.ListByUser(s.ctx, user.ID, pagination.PaginationParams{Page: 1, PageSize: 10})
	s.Require().NoError(err, "ListByUser")
	s.Require().Len(logs, 2)
	s.Require().Equal(int64(2), page.Total)
}

// --- ListByAPIKey ---

func (s *UsageLogRepoSuite) TestListByAPIKey() {
	user := mustCreateUser(s.T(), s.client, &service.User{Email: "listbyapikey@test.com"})
	apiKey := mustCreateApiKey(s.T(), s.client, &service.APIKey{UserID: user.ID, Key: "sk-listbyapikey", Name: "k"})
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-listbyapikey"})

	s.createUsageLog(user, apiKey, account, 10, 20, 0.5, time.Now())
	s.createUsageLog(user, apiKey, account, 15, 25, 0.6, time.Now())

	logs, page, err := s.repo.ListByAPIKey(s.ctx, apiKey.ID, pagination.PaginationParams{Page: 1, PageSize: 10})
	s.Require().NoError(err, "ListByAPIKey")
	s.Require().Len(logs, 2)
	s.Require().Equal(int64(2), page.Total)
}

// --- ListByAccount ---

func (s *UsageLogRepoSuite) TestListByAccount() {
	user := mustCreateUser(s.T(), s.client, &service.User{Email: "listbyaccount@test.com"})
	apiKey := mustCreateApiKey(s.T(), s.client, &service.APIKey{UserID: user.ID, Key: "sk-listbyaccount", Name: "k"})
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-listbyaccount"})

	s.createUsageLog(user, apiKey, account, 10, 20, 0.5, time.Now())

	logs, page, err := s.repo.ListByAccount(s.ctx, account.ID, pagination.PaginationParams{Page: 1, PageSize: 10})
	s.Require().NoError(err, "ListByAccount")
	s.Require().Len(logs, 1)
	s.Require().Equal(int64(1), page.Total)
}

// --- GetUserStats ---

func (s *UsageLogRepoSuite) TestGetUserStats() {
	user := mustCreateUser(s.T(), s.client, &service.User{Email: "userstats@test.com"})
	apiKey := mustCreateApiKey(s.T(), s.client, &service.APIKey{UserID: user.ID, Key: "sk-userstats", Name: "k"})
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-userstats"})

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
	user := mustCreateUser(s.T(), s.client, &service.User{Email: "filters@test.com"})
	apiKey := mustCreateApiKey(s.T(), s.client, &service.APIKey{UserID: user.ID, Key: "sk-filters", Name: "k"})
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-filters"})

	s.createUsageLog(user, apiKey, account, 10, 20, 0.5, time.Now())

	filters := usagestats.UsageLogFilters{UserID: user.ID}
	logs, page, err := s.repo.ListWithFilters(s.ctx, pagination.PaginationParams{Page: 1, PageSize: 10}, filters)
	s.Require().NoError(err, "ListWithFilters")
	s.Require().Len(logs, 1)
	s.Require().Equal(int64(1), page.Total)
}

// --- GetDashboardStats ---

func (s *UsageLogRepoSuite) TestDashboardStats_TodayTotalsAndPerformance() {
	now := time.Now().UTC()
	todayStart := truncateToDayUTC(now)
	baseStats, err := s.repo.GetDashboardStats(s.ctx)
	s.Require().NoError(err, "GetDashboardStats base")

	userToday := mustCreateUser(s.T(), s.client, &service.User{
		Email:     "today@example.com",
		CreatedAt: testMaxTime(todayStart.Add(10*time.Second), now.Add(-10*time.Second)),
		UpdatedAt: now,
	})
	userOld := mustCreateUser(s.T(), s.client, &service.User{
		Email:     "old@example.com",
		CreatedAt: todayStart.Add(-24 * time.Hour),
		UpdatedAt: todayStart.Add(-24 * time.Hour),
	})

	group := mustCreateGroup(s.T(), s.client, &service.Group{Name: "g-ul"})
	apiKey1 := mustCreateApiKey(s.T(), s.client, &service.APIKey{UserID: userToday.ID, Key: "sk-ul-1", Name: "ul1"})
	mustCreateApiKey(s.T(), s.client, &service.APIKey{UserID: userOld.ID, Key: "sk-ul-2", Name: "ul2", Status: service.StatusDisabled})

	resetAt := now.Add(10 * time.Minute)
	accNormal := mustCreateAccount(s.T(), s.client, &service.Account{Name: "a-normal", Schedulable: true})
	mustCreateAccount(s.T(), s.client, &service.Account{Name: "a-error", Status: service.StatusError, Schedulable: true})
	mustCreateAccount(s.T(), s.client, &service.Account{Name: "a-rl", RateLimitedAt: &now, RateLimitResetAt: &resetAt, Schedulable: true})
	mustCreateAccount(s.T(), s.client, &service.Account{Name: "a-ov", OverloadUntil: &resetAt, Schedulable: true})

	d1, d2, d3 := 100, 200, 300
	logToday := &service.UsageLog{
		UserID:              userToday.ID,
		APIKeyID:            apiKey1.ID,
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
		CreatedAt:           testMaxTime(todayStart.Add(2*time.Minute), now.Add(-2*time.Minute)),
	}
	_, err = s.repo.Create(s.ctx, logToday)
	s.Require().NoError(err, "Create logToday")

	logOld := &service.UsageLog{
		UserID:       userOld.ID,
		APIKeyID:     apiKey1.ID,
		AccountID:    accNormal.ID,
		Model:        "claude-3",
		InputTokens:  5,
		OutputTokens: 6,
		TotalCost:    0.7,
		ActualCost:   0.7,
		DurationMs:   &d2,
		CreatedAt:    todayStart.Add(-1 * time.Hour),
	}
	_, err = s.repo.Create(s.ctx, logOld)
	s.Require().NoError(err, "Create logOld")

	logPerf := &service.UsageLog{
		UserID:       userToday.ID,
		APIKeyID:     apiKey1.ID,
		AccountID:    accNormal.ID,
		Model:        "claude-3",
		InputTokens:  1,
		OutputTokens: 2,
		TotalCost:    0.1,
		ActualCost:   0.1,
		DurationMs:   &d3,
		CreatedAt:    now.Add(-30 * time.Second),
	}
	_, err = s.repo.Create(s.ctx, logPerf)
	s.Require().NoError(err, "Create logPerf")

	aggRepo := newDashboardAggregationRepositoryWithSQL(s.tx)
	aggStart := todayStart.Add(-2 * time.Hour)
	aggEnd := now.Add(2 * time.Minute)
	s.Require().NoError(aggRepo.AggregateRange(s.ctx, aggStart, aggEnd), "AggregateRange")

	stats, err := s.repo.GetDashboardStats(s.ctx)
	s.Require().NoError(err, "GetDashboardStats")

	s.Require().Equal(baseStats.TotalUsers+2, stats.TotalUsers, "TotalUsers mismatch")
	s.Require().Equal(baseStats.TodayNewUsers+1, stats.TodayNewUsers, "TodayNewUsers mismatch")
	s.Require().Equal(baseStats.ActiveUsers+1, stats.ActiveUsers, "ActiveUsers mismatch")
	s.Require().Equal(baseStats.TotalAPIKeys+2, stats.TotalAPIKeys, "TotalAPIKeys mismatch")
	s.Require().Equal(baseStats.ActiveAPIKeys+1, stats.ActiveAPIKeys, "ActiveAPIKeys mismatch")
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
	// account_cost falls back to total_cost when account_stats_cost is NULL
	s.Require().Equal(baseStats.TotalAccountCost+2.3, stats.TotalAccountCost, "TotalAccountCost mismatch")
	s.Require().GreaterOrEqual(stats.TodayRequests, int64(1), "expected TodayRequests >= 1")
	s.Require().GreaterOrEqual(stats.TodayCost, 0.0, "expected TodayCost >= 0")
	s.Require().GreaterOrEqual(stats.TodayAccountCost, 0.0, "expected TodayAccountCost >= 0")

	wantRpm, wantTpm, err := s.repo.getPerformanceStats(s.ctx, 0)
	s.Require().NoError(err, "getPerformanceStats")
	s.Require().Equal(wantRpm, stats.Rpm, "Rpm mismatch")
	s.Require().Equal(wantTpm, stats.Tpm, "Tpm mismatch")
}

func (s *UsageLogRepoSuite) TestDashboardStatsWithRange_Fallback() {
	now := time.Now().UTC()
	todayStart := truncateToDayUTC(now)
	rangeStart := todayStart.Add(-24 * time.Hour)
	rangeEnd := now.Add(1 * time.Second)

	user1 := mustCreateUser(s.T(), s.client, &service.User{Email: "range-u1@test.com"})
	user2 := mustCreateUser(s.T(), s.client, &service.User{Email: "range-u2@test.com"})
	apiKey1 := mustCreateApiKey(s.T(), s.client, &service.APIKey{UserID: user1.ID, Key: "sk-range-1", Name: "k1"})
	apiKey2 := mustCreateApiKey(s.T(), s.client, &service.APIKey{UserID: user2.ID, Key: "sk-range-2", Name: "k2"})
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-range"})

	d1, d2, d3 := 100, 200, 300
	logOutside := &service.UsageLog{
		UserID:       user1.ID,
		APIKeyID:     apiKey1.ID,
		AccountID:    account.ID,
		Model:        "claude-3",
		InputTokens:  7,
		OutputTokens: 8,
		TotalCost:    0.8,
		ActualCost:   0.7,
		DurationMs:   &d3,
		CreatedAt:    rangeStart.Add(-1 * time.Hour),
	}
	_, err := s.repo.Create(s.ctx, logOutside)
	s.Require().NoError(err)

	logRange := &service.UsageLog{
		UserID:              user1.ID,
		APIKeyID:            apiKey1.ID,
		AccountID:           account.ID,
		Model:               "claude-3",
		InputTokens:         10,
		OutputTokens:        20,
		CacheCreationTokens: 1,
		CacheReadTokens:     2,
		TotalCost:           1.0,
		ActualCost:          0.9,
		DurationMs:          &d1,
		CreatedAt:           rangeStart.Add(2 * time.Hour),
	}
	_, err = s.repo.Create(s.ctx, logRange)
	s.Require().NoError(err)

	logToday := &service.UsageLog{
		UserID:          user2.ID,
		APIKeyID:        apiKey2.ID,
		AccountID:       account.ID,
		Model:           "claude-3",
		InputTokens:     5,
		OutputTokens:    6,
		CacheReadTokens: 1,
		TotalCost:       0.5,
		ActualCost:      0.5,
		DurationMs:      &d2,
		CreatedAt:       now,
	}
	_, err = s.repo.Create(s.ctx, logToday)
	s.Require().NoError(err)

	stats, err := s.repo.GetDashboardStatsWithRange(s.ctx, rangeStart, rangeEnd)
	s.Require().NoError(err)
	s.Require().Equal(int64(2), stats.TotalRequests)
	s.Require().Equal(int64(15), stats.TotalInputTokens)
	s.Require().Equal(int64(26), stats.TotalOutputTokens)
	s.Require().Equal(int64(1), stats.TotalCacheCreationTokens)
	s.Require().Equal(int64(3), stats.TotalCacheReadTokens)
	s.Require().Equal(int64(45), stats.TotalTokens)
	s.Require().Equal(1.5, stats.TotalCost)
	s.Require().Equal(1.4, stats.TotalActualCost)
	// account_cost = COALESCE(account_stats_cost, total_cost) * COALESCE(account_rate_multiplier, 1) = total_cost
	s.Require().Equal(1.5, stats.TotalAccountCost)
	s.Require().InEpsilon(150.0, stats.AverageDurationMs, 0.0001)
}

// --- GetUserDashboardStats ---

func (s *UsageLogRepoSuite) TestGetUserDashboardStats() {
	user := mustCreateUser(s.T(), s.client, &service.User{Email: "userdash@test.com"})
	apiKey := mustCreateApiKey(s.T(), s.client, &service.APIKey{UserID: user.ID, Key: "sk-userdash", Name: "k"})
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-userdash"})

	s.createUsageLog(user, apiKey, account, 10, 20, 0.5, time.Now())

	stats, err := s.repo.GetUserDashboardStats(s.ctx, user.ID)
	s.Require().NoError(err, "GetUserDashboardStats")
	s.Require().Equal(int64(1), stats.TotalAPIKeys)
	s.Require().Equal(int64(1), stats.TotalRequests)
}

// --- GetAccountTodayStats ---

func (s *UsageLogRepoSuite) TestGetAccountTodayStats() {
	user := mustCreateUser(s.T(), s.client, &service.User{Email: "acctoday@test.com"})
	apiKey := mustCreateApiKey(s.T(), s.client, &service.APIKey{UserID: user.ID, Key: "sk-acctoday", Name: "k"})
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-today"})

	createdAt := timezone.Today().Add(1 * time.Hour)

	m1 := 1.5
	m2 := 0.0
	_, err := s.repo.Create(s.ctx, &service.UsageLog{
		UserID:                user.ID,
		APIKeyID:              apiKey.ID,
		AccountID:             account.ID,
		RequestID:             uuid.New().String(),
		Model:                 "claude-3",
		InputTokens:           10,
		OutputTokens:          20,
		TotalCost:             1.0,
		ActualCost:            2.0,
		AccountRateMultiplier: &m1,
		CreatedAt:             createdAt,
	})
	s.Require().NoError(err)
	_, err = s.repo.Create(s.ctx, &service.UsageLog{
		UserID:                user.ID,
		APIKeyID:              apiKey.ID,
		AccountID:             account.ID,
		RequestID:             uuid.New().String(),
		Model:                 "claude-3",
		InputTokens:           5,
		OutputTokens:          5,
		TotalCost:             0.5,
		ActualCost:            1.0,
		AccountRateMultiplier: &m2,
		CreatedAt:             createdAt,
	})
	s.Require().NoError(err)

	stats, err := s.repo.GetAccountTodayStats(s.ctx, account.ID)
	s.Require().NoError(err, "GetAccountTodayStats")
	s.Require().Equal(int64(2), stats.Requests)
	s.Require().Equal(int64(40), stats.Tokens)
	// account cost = SUM(total_cost * account_rate_multiplier)
	s.Require().InEpsilon(1.5, stats.Cost, 0.0001)
	// standard cost = SUM(total_cost)
	s.Require().InEpsilon(1.5, stats.StandardCost, 0.0001)
	// user cost = SUM(actual_cost)
	s.Require().InEpsilon(3.0, stats.UserCost, 0.0001)
}

func (s *UsageLogRepoSuite) TestDashboardAggregationConsistency() {
	now := time.Now().UTC().Truncate(time.Second)
	// 使用固定的时间偏移确保 hour1 和 hour2 在同一天且都在过去
	// 选择当天 02:00 和 03:00 作为测试时间点（基于 now 的日期）
	dayStart := truncateToDayUTC(now)
	hour1 := dayStart.Add(2 * time.Hour) // 当天 02:00
	hour2 := dayStart.Add(3 * time.Hour) // 当天 03:00
	// 如果当前时间早于 hour2，则使用昨天的时间
	if now.Before(hour2.Add(time.Hour)) {
		dayStart = dayStart.Add(-24 * time.Hour)
		hour1 = dayStart.Add(2 * time.Hour)
		hour2 = dayStart.Add(3 * time.Hour)
	}

	user1 := mustCreateUser(s.T(), s.client, &service.User{Email: "agg-u1@test.com"})
	user2 := mustCreateUser(s.T(), s.client, &service.User{Email: "agg-u2@test.com"})
	apiKey1 := mustCreateApiKey(s.T(), s.client, &service.APIKey{UserID: user1.ID, Key: "sk-agg-1", Name: "k1"})
	apiKey2 := mustCreateApiKey(s.T(), s.client, &service.APIKey{UserID: user2.ID, Key: "sk-agg-2", Name: "k2"})
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-agg"})

	d1, d2, d3 := 100, 200, 150
	log1 := &service.UsageLog{
		UserID:              user1.ID,
		APIKeyID:            apiKey1.ID,
		AccountID:           account.ID,
		Model:               "claude-3",
		InputTokens:         10,
		OutputTokens:        20,
		CacheCreationTokens: 2,
		CacheReadTokens:     1,
		TotalCost:           1.0,
		ActualCost:          0.9,
		DurationMs:          &d1,
		CreatedAt:           hour1.Add(5 * time.Minute),
	}
	_, err := s.repo.Create(s.ctx, log1)
	s.Require().NoError(err)

	log2 := &service.UsageLog{
		UserID:       user1.ID,
		APIKeyID:     apiKey1.ID,
		AccountID:    account.ID,
		Model:        "claude-3",
		InputTokens:  5,
		OutputTokens: 5,
		TotalCost:    0.5,
		ActualCost:   0.5,
		DurationMs:   &d2,
		CreatedAt:    hour1.Add(20 * time.Minute),
	}
	_, err = s.repo.Create(s.ctx, log2)
	s.Require().NoError(err)

	log3 := &service.UsageLog{
		UserID:       user2.ID,
		APIKeyID:     apiKey2.ID,
		AccountID:    account.ID,
		Model:        "claude-3",
		InputTokens:  7,
		OutputTokens: 8,
		TotalCost:    0.7,
		ActualCost:   0.7,
		DurationMs:   &d3,
		CreatedAt:    hour2.Add(10 * time.Minute),
	}
	_, err = s.repo.Create(s.ctx, log3)
	s.Require().NoError(err)

	aggRepo := newDashboardAggregationRepositoryWithSQL(s.tx)
	aggStart := hour1.Add(-5 * time.Minute)
	aggEnd := hour2.Add(time.Hour) // 确保覆盖 hour2 的所有数据
	s.Require().NoError(aggRepo.AggregateRange(s.ctx, aggStart, aggEnd))

	type hourlyRow struct {
		totalRequests       int64
		inputTokens         int64
		outputTokens        int64
		cacheCreationTokens int64
		cacheReadTokens     int64
		totalCost           float64
		actualCost          float64
		totalDurationMs     int64
		activeUsers         int64
	}
	fetchHourly := func(bucketStart time.Time) hourlyRow {
		var row hourlyRow
		err := scanSingleRow(s.ctx, s.tx, `
			SELECT total_requests, input_tokens, output_tokens, cache_creation_tokens, cache_read_tokens,
			       total_cost, actual_cost, total_duration_ms, active_users
			FROM usage_dashboard_hourly
			WHERE bucket_start = $1
		`, []any{bucketStart}, &row.totalRequests, &row.inputTokens, &row.outputTokens,
			&row.cacheCreationTokens, &row.cacheReadTokens, &row.totalCost, &row.actualCost,
			&row.totalDurationMs, &row.activeUsers,
		)
		s.Require().NoError(err)
		return row
	}

	hour1Row := fetchHourly(hour1)
	s.Require().Equal(int64(2), hour1Row.totalRequests)
	s.Require().Equal(int64(15), hour1Row.inputTokens)
	s.Require().Equal(int64(25), hour1Row.outputTokens)
	s.Require().Equal(int64(2), hour1Row.cacheCreationTokens)
	s.Require().Equal(int64(1), hour1Row.cacheReadTokens)
	s.Require().Equal(1.5, hour1Row.totalCost)
	s.Require().Equal(1.4, hour1Row.actualCost)
	s.Require().Equal(int64(300), hour1Row.totalDurationMs)
	s.Require().Equal(int64(1), hour1Row.activeUsers)

	hour2Row := fetchHourly(hour2)
	s.Require().Equal(int64(1), hour2Row.totalRequests)
	s.Require().Equal(int64(7), hour2Row.inputTokens)
	s.Require().Equal(int64(8), hour2Row.outputTokens)
	s.Require().Equal(int64(0), hour2Row.cacheCreationTokens)
	s.Require().Equal(int64(0), hour2Row.cacheReadTokens)
	s.Require().Equal(0.7, hour2Row.totalCost)
	s.Require().Equal(0.7, hour2Row.actualCost)
	s.Require().Equal(int64(150), hour2Row.totalDurationMs)
	s.Require().Equal(int64(1), hour2Row.activeUsers)

	var daily struct {
		totalRequests       int64
		inputTokens         int64
		outputTokens        int64
		cacheCreationTokens int64
		cacheReadTokens     int64
		totalCost           float64
		actualCost          float64
		totalDurationMs     int64
		activeUsers         int64
	}
	err = scanSingleRow(s.ctx, s.tx, `
		SELECT total_requests, input_tokens, output_tokens, cache_creation_tokens, cache_read_tokens,
		       total_cost, actual_cost, total_duration_ms, active_users
		FROM usage_dashboard_daily
		WHERE bucket_date = $1::date
	`, []any{dayStart}, &daily.totalRequests, &daily.inputTokens, &daily.outputTokens,
		&daily.cacheCreationTokens, &daily.cacheReadTokens, &daily.totalCost, &daily.actualCost,
		&daily.totalDurationMs, &daily.activeUsers,
	)
	s.Require().NoError(err)
	s.Require().Equal(int64(3), daily.totalRequests)
	s.Require().Equal(int64(22), daily.inputTokens)
	s.Require().Equal(int64(33), daily.outputTokens)
	s.Require().Equal(int64(2), daily.cacheCreationTokens)
	s.Require().Equal(int64(1), daily.cacheReadTokens)
	s.Require().Equal(2.2, daily.totalCost)
	s.Require().Equal(2.1, daily.actualCost)
	s.Require().Equal(int64(450), daily.totalDurationMs)
	s.Require().Equal(int64(2), daily.activeUsers)
}

// --- GetBatchUserUsageStats ---

func (s *UsageLogRepoSuite) TestGetBatchUserUsageStats() {
	user1 := mustCreateUser(s.T(), s.client, &service.User{Email: "batch1@test.com"})
	user2 := mustCreateUser(s.T(), s.client, &service.User{Email: "batch2@test.com"})
	apiKey1 := mustCreateApiKey(s.T(), s.client, &service.APIKey{UserID: user1.ID, Key: "sk-batch1", Name: "k"})
	apiKey2 := mustCreateApiKey(s.T(), s.client, &service.APIKey{UserID: user2.ID, Key: "sk-batch2", Name: "k"})
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-batch"})

	s.createUsageLog(user1, apiKey1, account, 10, 20, 0.5, time.Now())
	s.createUsageLog(user2, apiKey2, account, 15, 25, 0.6, time.Now())

	stats, err := s.repo.GetBatchUserUsageStats(s.ctx, []int64{user1.ID, user2.ID}, time.Time{}, time.Time{})
	s.Require().NoError(err, "GetBatchUserUsageStats")
	s.Require().Len(stats, 2)
	s.Require().NotNil(stats[user1.ID])
	s.Require().NotNil(stats[user2.ID])
}

func (s *UsageLogRepoSuite) TestGetBatchUserUsageStats_Empty() {
	stats, err := s.repo.GetBatchUserUsageStats(s.ctx, []int64{}, time.Time{}, time.Time{})
	s.Require().NoError(err)
	s.Require().Empty(stats)
}

// --- GetBatchAPIKeyUsageStats ---

func (s *UsageLogRepoSuite) TestGetBatchApiKeyUsageStats() {
	user := mustCreateUser(s.T(), s.client, &service.User{Email: "batchkey@test.com"})
	apiKey1 := mustCreateApiKey(s.T(), s.client, &service.APIKey{UserID: user.ID, Key: "sk-batchkey1", Name: "k1"})
	apiKey2 := mustCreateApiKey(s.T(), s.client, &service.APIKey{UserID: user.ID, Key: "sk-batchkey2", Name: "k2"})
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-batchkey"})

	s.createUsageLog(user, apiKey1, account, 10, 20, 0.5, time.Now())
	s.createUsageLog(user, apiKey2, account, 15, 25, 0.6, time.Now())

	stats, err := s.repo.GetBatchAPIKeyUsageStats(s.ctx, []int64{apiKey1.ID, apiKey2.ID}, time.Time{}, time.Time{})
	s.Require().NoError(err, "GetBatchAPIKeyUsageStats")
	s.Require().Len(stats, 2)
}

func (s *UsageLogRepoSuite) TestGetBatchApiKeyUsageStats_Empty() {
	stats, err := s.repo.GetBatchAPIKeyUsageStats(s.ctx, []int64{}, time.Time{}, time.Time{})
	s.Require().NoError(err)
	s.Require().Empty(stats)
}

// --- GetGlobalStats ---

func (s *UsageLogRepoSuite) TestGetGlobalStats() {
	user := mustCreateUser(s.T(), s.client, &service.User{Email: "global@test.com"})
	apiKey := mustCreateApiKey(s.T(), s.client, &service.APIKey{UserID: user.ID, Key: "sk-global", Name: "k"})
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-global"})

	base := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	s.createUsageLog(user, apiKey, account, 10, 20, 0.5, base)
	s.createUsageLog(user, apiKey, account, 15, 25, 0.6, base.Add(1*time.Hour))

	stats, err := s.repo.GetGlobalStats(s.ctx, base.Add(-1*time.Hour), base.Add(2*time.Hour))
	s.Require().NoError(err, "GetGlobalStats")
	s.Require().Equal(int64(2), stats.TotalRequests)
	s.Require().Equal(int64(25), stats.TotalInputTokens)
	s.Require().Equal(int64(45), stats.TotalOutputTokens)
}

func testMaxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

// --- ListByUserAndTimeRange ---

func (s *UsageLogRepoSuite) TestListByUserAndTimeRange() {
	user := mustCreateUser(s.T(), s.client, &service.User{Email: "timerange@test.com"})
	apiKey := mustCreateApiKey(s.T(), s.client, &service.APIKey{UserID: user.ID, Key: "sk-timerange", Name: "k"})
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-timerange"})

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

// --- ListByAPIKeyAndTimeRange ---

func (s *UsageLogRepoSuite) TestListByAPIKeyAndTimeRange() {
	user := mustCreateUser(s.T(), s.client, &service.User{Email: "keytimerange@test.com"})
	apiKey := mustCreateApiKey(s.T(), s.client, &service.APIKey{UserID: user.ID, Key: "sk-keytimerange", Name: "k"})
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-keytimerange"})

	base := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	s.createUsageLog(user, apiKey, account, 10, 20, 0.5, base)
	s.createUsageLog(user, apiKey, account, 15, 25, 0.6, base.Add(30*time.Minute))
	s.createUsageLog(user, apiKey, account, 20, 30, 0.7, base.Add(-24*time.Hour)) // outside range

	startTime := base.Add(-1 * time.Hour)
	endTime := base.Add(2 * time.Hour)
	logs, _, err := s.repo.ListByAPIKeyAndTimeRange(s.ctx, apiKey.ID, startTime, endTime)
	s.Require().NoError(err, "ListByAPIKeyAndTimeRange")
	s.Require().Len(logs, 2)
}

// --- ListByAccountAndTimeRange ---

func (s *UsageLogRepoSuite) TestListByAccountAndTimeRange() {
	user := mustCreateUser(s.T(), s.client, &service.User{Email: "acctimerange@test.com"})
	apiKey := mustCreateApiKey(s.T(), s.client, &service.APIKey{UserID: user.ID, Key: "sk-acctimerange", Name: "k"})
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-acctimerange"})

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
	user := mustCreateUser(s.T(), s.client, &service.User{Email: "modeltimerange@test.com"})
	apiKey := mustCreateApiKey(s.T(), s.client, &service.APIKey{UserID: user.ID, Key: "sk-modeltimerange", Name: "k"})
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-modeltimerange"})

	base := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)

	// Create logs with different models
	log1 := &service.UsageLog{
		UserID:       user.ID,
		APIKeyID:     apiKey.ID,
		AccountID:    account.ID,
		Model:        "claude-3-opus",
		InputTokens:  10,
		OutputTokens: 20,
		TotalCost:    0.5,
		ActualCost:   0.5,
		CreatedAt:    base,
	}
	_, err := s.repo.Create(s.ctx, log1)
	s.Require().NoError(err)

	log2 := &service.UsageLog{
		UserID:       user.ID,
		APIKeyID:     apiKey.ID,
		AccountID:    account.ID,
		Model:        "claude-3-opus",
		InputTokens:  15,
		OutputTokens: 25,
		TotalCost:    0.6,
		ActualCost:   0.6,
		CreatedAt:    base.Add(30 * time.Minute),
	}
	_, err = s.repo.Create(s.ctx, log2)
	s.Require().NoError(err)

	log3 := &service.UsageLog{
		UserID:       user.ID,
		APIKeyID:     apiKey.ID,
		AccountID:    account.ID,
		Model:        "claude-3-sonnet",
		InputTokens:  20,
		OutputTokens: 30,
		TotalCost:    0.7,
		ActualCost:   0.7,
		CreatedAt:    base.Add(1 * time.Hour),
	}
	_, err = s.repo.Create(s.ctx, log3)
	s.Require().NoError(err)

	startTime := base.Add(-1 * time.Hour)
	endTime := base.Add(2 * time.Hour)
	logs, _, err := s.repo.ListByModelAndTimeRange(s.ctx, "claude-3-opus", startTime, endTime)
	s.Require().NoError(err, "ListByModelAndTimeRange")
	s.Require().Len(logs, 2)
}

// --- GetAccountWindowStats ---

func (s *UsageLogRepoSuite) TestGetAccountWindowStats() {
	user := mustCreateUser(s.T(), s.client, &service.User{Email: "windowstats@test.com"})
	apiKey := mustCreateApiKey(s.T(), s.client, &service.APIKey{UserID: user.ID, Key: "sk-windowstats", Name: "k"})
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-windowstats"})

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
	user := mustCreateUser(s.T(), s.client, &service.User{Email: "usertrend@test.com"})
	apiKey := mustCreateApiKey(s.T(), s.client, &service.APIKey{UserID: user.ID, Key: "sk-usertrend", Name: "k"})
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-usertrend"})

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
	user := mustCreateUser(s.T(), s.client, &service.User{Email: "usertrendhourly@test.com"})
	apiKey := mustCreateApiKey(s.T(), s.client, &service.APIKey{UserID: user.ID, Key: "sk-usertrendhourly", Name: "k"})
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-usertrendhourly"})

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
	user := mustCreateUser(s.T(), s.client, &service.User{Email: "modelstats@test.com"})
	apiKey := mustCreateApiKey(s.T(), s.client, &service.APIKey{UserID: user.ID, Key: "sk-modelstats", Name: "k"})
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-modelstats"})

	base := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)

	// Create logs with different models
	log1 := &service.UsageLog{
		UserID:       user.ID,
		APIKeyID:     apiKey.ID,
		AccountID:    account.ID,
		Model:        "claude-3-opus",
		InputTokens:  100,
		OutputTokens: 200,
		TotalCost:    0.5,
		ActualCost:   0.5,
		CreatedAt:    base,
	}
	_, err := s.repo.Create(s.ctx, log1)
	s.Require().NoError(err)

	log2 := &service.UsageLog{
		UserID:       user.ID,
		APIKeyID:     apiKey.ID,
		AccountID:    account.ID,
		Model:        "claude-3-sonnet",
		InputTokens:  50,
		OutputTokens: 100,
		TotalCost:    0.2,
		ActualCost:   0.2,
		CreatedAt:    base.Add(1 * time.Hour),
	}
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
}

// --- GetUsageTrendWithFilters ---

func (s *UsageLogRepoSuite) TestGetUsageTrendWithFilters() {
	user := mustCreateUser(s.T(), s.client, &service.User{Email: "trendfilters@test.com"})
	apiKey := mustCreateApiKey(s.T(), s.client, &service.APIKey{UserID: user.ID, Key: "sk-trendfilters", Name: "k"})
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-trendfilters"})

	base := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	s.createUsageLog(user, apiKey, account, 10, 20, 0.5, base)
	s.createUsageLog(user, apiKey, account, 15, 25, 0.6, base.Add(24*time.Hour))

	startTime := base.Add(-1 * time.Hour)
	endTime := base.Add(48 * time.Hour)

	// Test with user filter
	trend, err := s.repo.GetUsageTrendWithFilters(s.ctx, startTime, endTime, "day", user.ID, 0, 0, 0, "", nil, nil, nil)
	s.Require().NoError(err, "GetUsageTrendWithFilters user filter")
	s.Require().Len(trend, 2)

	// Test with apiKey filter
	trend, err = s.repo.GetUsageTrendWithFilters(s.ctx, startTime, endTime, "day", 0, apiKey.ID, 0, 0, "", nil, nil, nil)
	s.Require().NoError(err, "GetUsageTrendWithFilters apiKey filter")
	s.Require().Len(trend, 2)

	// Test with both filters
	trend, err = s.repo.GetUsageTrendWithFilters(s.ctx, startTime, endTime, "day", user.ID, apiKey.ID, 0, 0, "", nil, nil, nil)
	s.Require().NoError(err, "GetUsageTrendWithFilters both filters")
	s.Require().Len(trend, 2)
}

func (s *UsageLogRepoSuite) TestGetUsageTrendWithFilters_HourlyGranularity() {
	user := mustCreateUser(s.T(), s.client, &service.User{Email: "trendfilters-h@test.com"})
	apiKey := mustCreateApiKey(s.T(), s.client, &service.APIKey{UserID: user.ID, Key: "sk-trendfilters-h", Name: "k"})
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-trendfilters-h"})

	base := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	s.createUsageLog(user, apiKey, account, 10, 20, 0.5, base)
	s.createUsageLog(user, apiKey, account, 15, 25, 0.6, base.Add(1*time.Hour))

	startTime := base.Add(-1 * time.Hour)
	endTime := base.Add(3 * time.Hour)

	trend, err := s.repo.GetUsageTrendWithFilters(s.ctx, startTime, endTime, "hour", user.ID, 0, 0, 0, "", nil, nil, nil)
	s.Require().NoError(err, "GetUsageTrendWithFilters hourly")
	s.Require().Len(trend, 2)
}

// --- GetModelStatsWithFilters ---

func (s *UsageLogRepoSuite) TestGetModelStatsWithFilters() {
	user := mustCreateUser(s.T(), s.client, &service.User{Email: "modelfilters@test.com"})
	apiKey := mustCreateApiKey(s.T(), s.client, &service.APIKey{UserID: user.ID, Key: "sk-modelfilters", Name: "k"})
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-modelfilters"})

	base := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)

	log1 := &service.UsageLog{
		UserID:       user.ID,
		APIKeyID:     apiKey.ID,
		AccountID:    account.ID,
		Model:        "claude-3-opus",
		InputTokens:  100,
		OutputTokens: 200,
		TotalCost:    0.5,
		ActualCost:   0.5,
		CreatedAt:    base,
	}
	_, err := s.repo.Create(s.ctx, log1)
	s.Require().NoError(err)

	log2 := &service.UsageLog{
		UserID:       user.ID,
		APIKeyID:     apiKey.ID,
		AccountID:    account.ID,
		Model:        "claude-3-sonnet",
		InputTokens:  50,
		OutputTokens: 100,
		TotalCost:    0.2,
		ActualCost:   0.2,
		CreatedAt:    base.Add(1 * time.Hour),
	}
	_, err = s.repo.Create(s.ctx, log2)
	s.Require().NoError(err)

	startTime := base.Add(-1 * time.Hour)
	endTime := base.Add(2 * time.Hour)

	// Test with user filter
	stats, err := s.repo.GetModelStatsWithFilters(s.ctx, startTime, endTime, user.ID, 0, 0, 0, nil, nil, nil)
	s.Require().NoError(err, "GetModelStatsWithFilters user filter")
	s.Require().Len(stats, 2)

	// Test with apiKey filter
	stats, err = s.repo.GetModelStatsWithFilters(s.ctx, startTime, endTime, 0, apiKey.ID, 0, 0, nil, nil, nil)
	s.Require().NoError(err, "GetModelStatsWithFilters apiKey filter")
	s.Require().Len(stats, 2)

	// Test with account filter
	stats, err = s.repo.GetModelStatsWithFilters(s.ctx, startTime, endTime, 0, 0, account.ID, 0, nil, nil, nil)
	s.Require().NoError(err, "GetModelStatsWithFilters account filter")
	s.Require().Len(stats, 2)
}

// --- GetAccountUsageStats ---

func (s *UsageLogRepoSuite) TestGetAccountUsageStats() {
	user := mustCreateUser(s.T(), s.client, &service.User{Email: "accstats@test.com"})
	apiKey := mustCreateApiKey(s.T(), s.client, &service.APIKey{UserID: user.ID, Key: "sk-accstats", Name: "k"})
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-accstats"})

	base := time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC)

	// Create logs on different days
	log1 := &service.UsageLog{
		UserID:       user.ID,
		APIKeyID:     apiKey.ID,
		AccountID:    account.ID,
		Model:        "claude-3-opus",
		InputTokens:  100,
		OutputTokens: 200,
		TotalCost:    0.5,
		ActualCost:   0.4,
		CreatedAt:    base.Add(12 * time.Hour),
	}
	_, err := s.repo.Create(s.ctx, log1)
	s.Require().NoError(err)

	log2 := &service.UsageLog{
		UserID:       user.ID,
		APIKeyID:     apiKey.ID,
		AccountID:    account.ID,
		Model:        "claude-3-sonnet",
		InputTokens:  50,
		OutputTokens: 100,
		TotalCost:    0.2,
		ActualCost:   0.15,
		CreatedAt:    base.Add(36 * time.Hour), // next day
	}
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
}

func (s *UsageLogRepoSuite) TestGetAccountUsageStats_EmptyRange() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-emptystats"})

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
	user1 := mustCreateUser(s.T(), s.client, &service.User{Email: "usertrend1@test.com"})
	user2 := mustCreateUser(s.T(), s.client, &service.User{Email: "usertrend2@test.com"})
	apiKey1 := mustCreateApiKey(s.T(), s.client, &service.APIKey{UserID: user1.ID, Key: "sk-usertrend1", Name: "k1"})
	apiKey2 := mustCreateApiKey(s.T(), s.client, &service.APIKey{UserID: user2.ID, Key: "sk-usertrend2", Name: "k2"})
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-usertrends"})

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

// --- GetAPIKeyUsageTrend ---

func (s *UsageLogRepoSuite) TestGetAPIKeyUsageTrend() {
	user := mustCreateUser(s.T(), s.client, &service.User{Email: "keytrend@test.com"})
	apiKey1 := mustCreateApiKey(s.T(), s.client, &service.APIKey{UserID: user.ID, Key: "sk-keytrend1", Name: "k1"})
	apiKey2 := mustCreateApiKey(s.T(), s.client, &service.APIKey{UserID: user.ID, Key: "sk-keytrend2", Name: "k2"})
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-keytrends"})

	base := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	s.createUsageLog(user, apiKey1, account, 100, 200, 1.0, base)
	s.createUsageLog(user, apiKey2, account, 50, 100, 0.5, base)
	s.createUsageLog(user, apiKey1, account, 100, 200, 1.0, base.Add(24*time.Hour))

	startTime := base.Add(-1 * time.Hour)
	endTime := base.Add(48 * time.Hour)

	trend, err := s.repo.GetAPIKeyUsageTrend(s.ctx, startTime, endTime, "day", 10)
	s.Require().NoError(err, "GetAPIKeyUsageTrend")
	s.Require().GreaterOrEqual(len(trend), 2)
}

func (s *UsageLogRepoSuite) TestGetAPIKeyUsageTrend_HourlyGranularity() {
	user := mustCreateUser(s.T(), s.client, &service.User{Email: "keytrendh@test.com"})
	apiKey := mustCreateApiKey(s.T(), s.client, &service.APIKey{UserID: user.ID, Key: "sk-keytrendh", Name: "k"})
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-keytrendh"})

	base := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	s.createUsageLog(user, apiKey, account, 100, 200, 1.0, base)
	s.createUsageLog(user, apiKey, account, 50, 100, 0.5, base.Add(1*time.Hour))

	startTime := base.Add(-1 * time.Hour)
	endTime := base.Add(3 * time.Hour)

	trend, err := s.repo.GetAPIKeyUsageTrend(s.ctx, startTime, endTime, "hour", 10)
	s.Require().NoError(err, "GetAPIKeyUsageTrend hourly")
	s.Require().Len(trend, 2)
}

// --- ListWithFilters (additional filter tests) ---

func (s *UsageLogRepoSuite) TestListWithFilters_ApiKeyFilter() {
	user := mustCreateUser(s.T(), s.client, &service.User{Email: "filterskey@test.com"})
	apiKey := mustCreateApiKey(s.T(), s.client, &service.APIKey{UserID: user.ID, Key: "sk-filterskey", Name: "k"})
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-filterskey"})

	s.createUsageLog(user, apiKey, account, 10, 20, 0.5, time.Now())

	filters := usagestats.UsageLogFilters{APIKeyID: apiKey.ID}
	logs, page, err := s.repo.ListWithFilters(s.ctx, pagination.PaginationParams{Page: 1, PageSize: 10}, filters)
	s.Require().NoError(err, "ListWithFilters apiKey")
	s.Require().Len(logs, 1)
	s.Require().Equal(int64(1), page.Total)
}

func (s *UsageLogRepoSuite) TestListWithFilters_TimeRange() {
	user := mustCreateUser(s.T(), s.client, &service.User{Email: "filterstime@test.com"})
	apiKey := mustCreateApiKey(s.T(), s.client, &service.APIKey{UserID: user.ID, Key: "sk-filterstime", Name: "k"})
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-filterstime"})

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
	user := mustCreateUser(s.T(), s.client, &service.User{Email: "filterscombined@test.com"})
	apiKey := mustCreateApiKey(s.T(), s.client, &service.APIKey{UserID: user.ID, Key: "sk-filterscombined", Name: "k"})
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-filterscombined"})

	base := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	s.createUsageLog(user, apiKey, account, 10, 20, 0.5, base)
	s.createUsageLog(user, apiKey, account, 15, 25, 0.6, base.Add(1*time.Hour))

	startTime := base.Add(-1 * time.Hour)
	endTime := base.Add(2 * time.Hour)
	filters := usagestats.UsageLogFilters{
		UserID:    user.ID,
		APIKeyID:  apiKey.ID,
		StartTime: &startTime,
		EndTime:   &endTime,
	}
	logs, page, err := s.repo.ListWithFilters(s.ctx, pagination.PaginationParams{Page: 1, PageSize: 10}, filters)
	s.Require().NoError(err, "ListWithFilters combined")
	s.Require().Len(logs, 2)
	s.Require().Equal(int64(2), page.Total)
}
