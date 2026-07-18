package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

type backgroundTaskAccountRepoStub struct {
	AccountRepository
	account *Account
}

func (r *backgroundTaskAccountRepoStub) GetByID(_ context.Context, id int64) (*Account, error) {
	if r.account == nil || r.account.ID != id {
		return nil, errors.New("account not found")
	}
	return r.account, nil
}

type backgroundTaskOpenAIStub struct {
	mu         sync.Mutex
	candidates []OpenAIQuotaResetCreditCandidate
	listErr    error
	listCalls  int
	reset      func(context.Context, int64, string, string) (*OpenAIQuotaResetResult, error)
}

func (s *backgroundTaskOpenAIStub) ListResetCredits(_ context.Context, _ int64) ([]OpenAIQuotaResetCreditCandidate, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.listCalls++
	return append([]OpenAIQuotaResetCreditCandidate(nil), s.candidates...), s.listErr
}

func (s *backgroundTaskOpenAIStub) ResetCreditByID(ctx context.Context, accountID int64, redeemRequestID, creditID string) (*OpenAIQuotaResetResult, error) {
	if s.reset == nil {
		return nil, errors.New("unexpected reset")
	}
	return s.reset(ctx, accountID, redeemRequestID, creditID)
}

type backgroundTaskRepoStub struct {
	BackgroundTaskRepository
	mu                 sync.Mutex
	createdInput       *CreateBackgroundTaskInput
	createdTask        *BackgroundTaskRun
	tasksByCreationKey map[string]*BackgroundTaskRun
	beginDispatches    int
	beginErr           error
}

type panickingBackgroundTaskHandler struct {
	dispatch bool
}

func (h panickingBackgroundTaskHandler) TaskType() string { return "panic_test" }

func (h panickingBackgroundTaskHandler) Execute(ctx context.Context, execution *BackgroundTaskExecution) BackgroundTaskHandlerResult {
	if h.dispatch {
		if err := execution.BeginDispatch(ctx); err != nil {
			panic(err)
		}
	}
	panic(`secret panic with {"credit_id":"private-credit"}`)
}

func (r *backgroundTaskRepoStub) Create(_ context.Context, input *CreateBackgroundTaskInput) (*BackgroundTaskRun, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	copyInput := *input
	copyInput.Payload = append(json.RawMessage(nil), input.Payload...)
	copyInput.Display = append(json.RawMessage(nil), input.Display...)
	r.createdInput = &copyInput
	if r.createdTask != nil {
		return r.createdTask, false, nil
	}
	key := "persisted-redeem-request-id"
	task := &BackgroundTaskRun{
		ID: 1, TaskType: input.TaskType, ResourceType: input.ResourceType,
		ResourceID: input.ResourceID, Payload: copyInput.Payload, Display: copyInput.Display,
		RunAt: input.RunAt, Status: BackgroundTaskStatusPending, MaxAttempts: input.MaxAttempts,
		DedupeKey: input.DedupeKey, IdempotencyKey: &key, CreatedBy: input.CreatedBy,
	}
	if input.CreationRequestKey != "" {
		creationKey := input.CreationRequestKey
		task.CreationRequestKey = &creationKey
		if r.tasksByCreationKey == nil {
			r.tasksByCreationKey = make(map[string]*BackgroundTaskRun)
		}
		r.tasksByCreationKey[creationKey] = task
	}
	r.createdTask = task
	return task, true, nil
}

func (r *backgroundTaskRepoStub) GetByCreationRequestKey(_ context.Context, key string) (*BackgroundTaskRun, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if task := r.tasksByCreationKey[key]; task != nil {
		return task, nil
	}
	return nil, ErrBackgroundTaskNotFound
}

func (r *backgroundTaskRepoStub) BeginDispatch(_ context.Context, _ int64, _ string, _ int64, _ time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.beginErr != nil {
		return r.beginErr
	}
	r.beginDispatches++
	return nil
}

func TestOpenAIQuotaResetHandlerRetriesSameLogicalRequestAfterAmbiguousConsumption(t *testing.T) {
	const (
		accountID = int64(42)
		requestID = "redeem-request-fixed"
		creditID  = "credit-fixed"
	)
	type call struct {
		requestID string
		creditID  string
	}
	var calls []call
	consumed := false
	consumptionCount := 0
	upstream := &backgroundTaskOpenAIStub{reset: func(_ context.Context, gotAccountID int64, gotRequestID, gotCreditID string) (*OpenAIQuotaResetResult, error) {
		require.Equal(t, accountID, gotAccountID)
		calls = append(calls, call{requestID: gotRequestID, creditID: gotCreditID})
		if !consumed {
			consumed = true
			consumptionCount++
			return nil, &OpenAIQuotaResetAttemptError{
				Kind: OpenAIQuotaResetErrorRequest, Retryable: true,
				Err: errors.New("response timed out after upstream consumed the credit"),
			}
		}
		if gotRequestID == requestID && gotCreditID == creditID {
			return &OpenAIQuotaResetResult{Code: "alreadyRedeemed", WindowsReset: 1}, nil
		}
		consumptionCount++
		return &OpenAIQuotaResetResult{Code: "reset", WindowsReset: 1}, nil
	}}

	expiresAt := time.Now().Add(10 * time.Minute).UTC().Truncate(time.Second)
	payload, err := json.Marshal(openAIQuotaResetTaskPayload{
		AccountID: accountID, CreditID: creditID, CreditExpiresAt: expiresAt.Format(time.RFC3339),
	})
	require.NoError(t, err)
	task := &BackgroundTaskRun{
		ID: 7, TaskType: BackgroundTaskTypeOpenAIQuotaReset, ResourceType: "openai_account", ResourceID: "42",
		DedupeKey: "account:42:credit:credit-fixed", Payload: payload,
		Status: BackgroundTaskStatusRunning, MaxAttempts: 5,
		IdempotencyKey: stringPointer(requestID), ClaimOwner: stringPointer("worker-a"), ClaimVersion: 3,
	}
	repo := &backgroundTaskRepoStub{}
	handler := &openAIQuotaResetTaskHandler{quotaService: upstream}

	first := handler.Execute(context.Background(), &BackgroundTaskExecution{repo: repo, task: task, owner: "worker-a"})
	require.NotNil(t, first.RetryAt)
	require.Equal(t, BackgroundTaskStatus(""), first.Status)
	require.Equal(t, 1, task.DispatchCount)

	second := handler.Execute(context.Background(), &BackgroundTaskExecution{repo: repo, task: task, owner: "worker-b"})
	require.Nil(t, second.RetryAt)
	require.Equal(t, BackgroundTaskStatusSucceeded, second.Status)
	require.Equal(t, 2, task.DispatchCount)
	require.Equal(t, 2, repo.beginDispatches)
	require.Len(t, calls, 2)
	require.Equal(t, 1, consumptionCount)

	requestIDs := map[string]struct{}{}
	creditIDs := map[string]struct{}{}
	for _, item := range calls {
		requestIDs[item.requestID] = struct{}{}
		creditIDs[item.creditID] = struct{}{}
	}
	require.Equal(t, map[string]struct{}{requestID: {}}, requestIDs)
	require.Equal(t, map[string]struct{}{creditID: {}}, creditIDs)
}

func TestBackgroundTaskHandlerPanicAfterDispatchIsRetryableIndeterminate(t *testing.T) {
	task := &BackgroundTaskRun{ID: 95, ClaimVersion: 1}
	result := executeBackgroundTaskHandler(
		panickingBackgroundTaskHandler{dispatch: true},
		context.Background(),
		&BackgroundTaskExecution{repo: &backgroundTaskRepoStub{}, task: task, owner: "worker"},
	)
	require.Equal(t, BackgroundTaskStatusIndeterminate, result.Status)
	require.False(t, result.ReleaseDedupeLock)
	require.NotNil(t, task.FirstDispatchAt)
	require.Equal(t, 1, task.DispatchCount)
	require.Equal(t, "background task handler panicked", result.ErrorMessage)
	require.NotContains(t, result.ErrorMessage, "private-credit")
}

func TestBackgroundTaskHandlerPanicBeforeDispatchFailsAndUnlocks(t *testing.T) {
	task := &BackgroundTaskRun{ID: 96, ClaimVersion: 1}
	result := executeBackgroundTaskHandler(
		panickingBackgroundTaskHandler{},
		context.Background(),
		&BackgroundTaskExecution{repo: &backgroundTaskRepoStub{}, task: task, owner: "worker"},
	)
	require.Equal(t, BackgroundTaskStatusFailed, result.Status)
	require.True(t, result.ReleaseDedupeLock)
	require.Nil(t, task.FirstDispatchAt)
}

func TestOpenAIQuotaResetHandlerRealHTTPTimeoutThenAlreadyRedeemed(t *testing.T) {
	const (
		accountID = int64(77)
		requestID = "real-http-request-fixed"
		creditID  = "real-http-credit-fixed"
	)
	type pair struct {
		requestID string
		creditID  string
	}
	var (
		mu               sync.Mutex
		calls            []pair
		consumed         = make(map[pair]bool)
		consumptionCount int
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/backend-api/wham/rate-limit-reset-credits/consume" {
			http.NotFound(w, r)
			return
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		logicalPair := pair{requestID: body["redeem_request_id"], creditID: body["credit_id"]}
		mu.Lock()
		calls = append(calls, logicalPair)
		firstConsumption := !consumed[logicalPair]
		if firstConsumption {
			consumed[logicalPair] = true
			consumptionCount++
		}
		mu.Unlock()

		w.Header().Set("content-type", "application/json")
		if firstConsumption {
			// The side effect is committed before the response enters the
			// client-timeout window.
			time.Sleep(100 * time.Millisecond)
			_, _ = w.Write([]byte(`{"code":"reset","windows_reset":1}`))
			return
		}
		_, _ = w.Write([]byte(`{"code":"alreadyRedeemed","windows_reset":1}`))
	}))
	defer server.Close()

	account := &Account{
		ID: accountID, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive,
		Credentials: map[string]any{"chatgpt_account_id": "account-77"},
	}
	accountRepo := &stubQuotaAccountRepo{accounts: map[int64]*Account{accountID: account}}
	tokenCache := &stubQuotaTokenCache{tokens: map[string]string{OpenAITokenCacheKey(account): "access-token"}}
	quota := NewOpenAIQuotaService(
		accountRepo,
		nil,
		NewOpenAITokenProvider(accountRepo, tokenCache, nil),
		newQuotaRedirectingFactory(server),
	)
	quota.upstreamTimeout = 25 * time.Millisecond
	expiresAt := time.Now().Add(10 * time.Minute).UTC().Truncate(time.Second)
	payload, err := json.Marshal(openAIQuotaResetTaskPayload{
		AccountID: accountID, CreditID: creditID, CreditExpiresAt: expiresAt.Format(time.RFC3339),
	})
	require.NoError(t, err)
	task := &BackgroundTaskRun{
		ID: 77, TaskType: BackgroundTaskTypeOpenAIQuotaReset, ResourceType: "openai_account", ResourceID: "77",
		DedupeKey: "account:77:credit:real-http-credit-fixed", Payload: payload,
		Status: BackgroundTaskStatusRunning, MaxAttempts: 5,
		IdempotencyKey: stringPointer(requestID), ClaimVersion: 1,
	}
	handler := &openAIQuotaResetTaskHandler{quotaService: quota}
	repo := &backgroundTaskRepoStub{}

	first := handler.Execute(context.Background(), &BackgroundTaskExecution{repo: repo, task: task, owner: "worker-a"})
	require.NotNil(t, first.RetryAt)
	second := handler.Execute(context.Background(), &BackgroundTaskExecution{repo: repo, task: task, owner: "worker-b"})
	require.Equal(t, BackgroundTaskStatusSucceeded, second.Status)

	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, 2, len(calls), "HTTP calls")
	require.Equal(t, 1, len(consumed), "unique logical request pairs")
	require.Equal(t, 1, consumptionCount, "actual credit consumptions")
	require.Equal(t, []pair{
		{requestID: requestID, creditID: creditID},
		{requestID: requestID, creditID: creditID},
	}, calls)
}

func TestOpenAIQuotaResetHandlerExhaustionBecomesIndeterminate(t *testing.T) {
	expiresAt := time.Now().Add(10 * time.Minute).UTC().Truncate(time.Second)
	payload, err := json.Marshal(openAIQuotaResetTaskPayload{
		AccountID: 9, CreditID: "credit-9", CreditExpiresAt: expiresAt.Format(time.RFC3339),
	})
	require.NoError(t, err)
	requestID := "request-9"
	task := &BackgroundTaskRun{
		ID: 9, TaskType: BackgroundTaskTypeOpenAIQuotaReset, ResourceType: "openai_account", ResourceID: "9",
		DedupeKey: "account:9:credit:credit-9", Payload: payload,
		Status: BackgroundTaskStatusRunning, DispatchCount: 4, MaxAttempts: 5,
		IdempotencyKey: &requestID, ClaimVersion: 5,
	}
	upstream := &backgroundTaskOpenAIStub{reset: func(context.Context, int64, string, string) (*OpenAIQuotaResetResult, error) {
		return nil, &OpenAIQuotaResetAttemptError{
			Kind: OpenAIQuotaResetErrorRequest, Retryable: true, Err: errors.New("timeout"),
		}
	}}

	result := (&openAIQuotaResetTaskHandler{quotaService: upstream}).Execute(
		context.Background(),
		&BackgroundTaskExecution{repo: &backgroundTaskRepoStub{}, task: task, owner: "worker"},
	)
	require.Nil(t, result.RetryAt)
	require.Equal(t, BackgroundTaskStatusIndeterminate, result.Status)
	require.Equal(t, "request", result.ErrorCode)
	require.False(t, result.ReleaseDedupeLock)
}

func TestOpenAIQuotaResetHandlerCrashReclaimCannotExceedDispatchBudget(t *testing.T) {
	expiresAt := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
	payload, err := json.Marshal(openAIQuotaResetTaskPayload{
		AccountID: 90, CreditID: "credit-90", CreditExpiresAt: expiresAt.Format(time.RFC3339),
	})
	require.NoError(t, err)
	requestID := "request-90"
	firstDispatch := time.Now().Add(-time.Minute)
	task := &BackgroundTaskRun{
		ID: 90, ResourceType: "openai_account", ResourceID: "90", DedupeKey: "account:90:credit:credit-90",
		Payload: payload, Status: BackgroundTaskStatusRunning, DispatchCount: 5, MaxAttempts: 5,
		IdempotencyKey: &requestID, FirstDispatchAt: &firstDispatch,
	}
	repo := &backgroundTaskRepoStub{}
	upstream := &backgroundTaskOpenAIStub{reset: func(context.Context, int64, string, string) (*OpenAIQuotaResetResult, error) {
		require.FailNow(t, "exhausted reclaimed task reached upstream")
		return nil, nil
	}}

	result := (&openAIQuotaResetTaskHandler{quotaService: upstream}).Execute(
		context.Background(),
		&BackgroundTaskExecution{repo: repo, task: task, owner: "worker"},
	)
	require.Equal(t, BackgroundTaskStatusIndeterminate, result.Status)
	require.Equal(t, "retry_budget_exhausted", result.ErrorCode)
	require.False(t, result.ReleaseDedupeLock)
	require.Zero(t, repo.beginDispatches)
}

func TestOpenAIQuotaResetHandlerShutdownBeforeRedispatchRequeuesSameTask(t *testing.T) {
	expiresAt := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
	payload, err := json.Marshal(openAIQuotaResetTaskPayload{
		AccountID: 99, CreditID: "credit-99", CreditExpiresAt: expiresAt.Format(time.RFC3339),
	})
	require.NoError(t, err)
	requestID := "request-99"
	firstDispatch := time.Now().Add(-time.Minute)
	task := &BackgroundTaskRun{
		ID: 99, ResourceType: "openai_account", ResourceID: "99", DedupeKey: "account:99:credit:credit-99",
		Payload: payload, Status: BackgroundTaskStatusRunning, DispatchCount: 1, MaxAttempts: 5,
		IdempotencyKey: &requestID, FirstDispatchAt: &firstDispatch,
	}
	repo := &backgroundTaskRepoStub{beginErr: context.Canceled}
	upstream := &backgroundTaskOpenAIStub{reset: func(context.Context, int64, string, string) (*OpenAIQuotaResetResult, error) {
		require.FailNow(t, "canceled redispatch reached upstream")
		return nil, nil
	}}

	result := (&openAIQuotaResetTaskHandler{quotaService: upstream}).Execute(
		context.Background(),
		&BackgroundTaskExecution{repo: repo, task: task, owner: "worker"},
	)
	require.NotNil(t, result.RetryAt)
	require.Empty(t, result.Status)
	require.Equal(t, "begin_dispatch_failed", result.ErrorCode)
	require.False(t, result.ReleaseDedupeLock)
	require.Zero(t, repo.beginDispatches)
}

func TestOpenAIQuotaResetHandlerShutdownBeforeFirstDispatchRequeuesTask(t *testing.T) {
	expiresAt := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
	payload, err := json.Marshal(openAIQuotaResetTaskPayload{
		AccountID: 100, CreditID: "credit-100", CreditExpiresAt: expiresAt.Format(time.RFC3339),
	})
	require.NoError(t, err)
	requestID := "request-100"
	task := &BackgroundTaskRun{
		ID: 100, ResourceType: "openai_account", ResourceID: "100", DedupeKey: "account:100:credit:credit-100",
		Payload: payload, Status: BackgroundTaskStatusRunning, MaxAttempts: 5, IdempotencyKey: &requestID,
	}
	repo := &backgroundTaskRepoStub{beginErr: context.Canceled}
	upstream := &backgroundTaskOpenAIStub{reset: func(context.Context, int64, string, string) (*OpenAIQuotaResetResult, error) {
		require.FailNow(t, "canceled first dispatch reached upstream")
		return nil, nil
	}}

	result := (&openAIQuotaResetTaskHandler{quotaService: upstream}).Execute(
		context.Background(),
		&BackgroundTaskExecution{repo: repo, task: task, owner: "worker"},
	)
	require.NotNil(t, result.RetryAt)
	require.Empty(t, result.Status)
	require.Zero(t, task.DispatchCount)
	require.Nil(t, task.FirstDispatchAt)
	require.Zero(t, repo.beginDispatches)
}

func TestOpenAIQuotaResetHandlerShutdownAfterDispatchSchedulesSameKeyRetry(t *testing.T) {
	expiresAt := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
	payload, err := json.Marshal(openAIQuotaResetTaskPayload{
		AccountID: 102, CreditID: "credit-102", CreditExpiresAt: expiresAt.Format(time.RFC3339),
	})
	require.NoError(t, err)
	requestID := "request-102"
	task := &BackgroundTaskRun{
		ID: 102, ResourceType: "openai_account", ResourceID: "102", DedupeKey: "account:102:credit:credit-102",
		Payload: payload, Status: BackgroundTaskStatusRunning, MaxAttempts: 5, IdempotencyKey: &requestID,
	}
	workerCtx, cancel := context.WithCancel(context.Background())
	cancel()
	upstream := &backgroundTaskOpenAIStub{reset: func(_ context.Context, _ int64, gotRequestID, gotCreditID string) (*OpenAIQuotaResetResult, error) {
		require.Equal(t, requestID, gotRequestID)
		require.Equal(t, "credit-102", gotCreditID)
		return nil, errors.New("preflight stopped before the HTTP request completed")
	}}

	result := (&openAIQuotaResetTaskHandler{quotaService: upstream}).Execute(
		workerCtx,
		&BackgroundTaskExecution{repo: &backgroundTaskRepoStub{}, task: task, owner: "worker"},
	)
	require.NotNil(t, result.RetryAt)
	require.Empty(t, result.Status)
	require.Equal(t, "worker_shutdown", result.ErrorCode)
	require.Equal(t, 1, task.DispatchCount)
	require.NotNil(t, task.FirstDispatchAt)
}

func TestOpenAIQuotaResetHandlerSanitizesPersistedUpstreamError(t *testing.T) {
	expiresAt := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
	payload, err := json.Marshal(openAIQuotaResetTaskPayload{
		AccountID: 93, CreditID: "private-credit", CreditExpiresAt: expiresAt.Format(time.RFC3339),
	})
	require.NoError(t, err)
	requestID := "private-request"
	task := &BackgroundTaskRun{
		ID: 93, ResourceType: "openai_account", ResourceID: "93", DedupeKey: "account:93:credit:private-credit",
		Payload: payload, Status: BackgroundTaskStatusRunning, MaxAttempts: 5, IdempotencyKey: &requestID,
	}
	upstream := &backgroundTaskOpenAIStub{reset: func(context.Context, int64, string, string) (*OpenAIQuotaResetResult, error) {
		return nil, &OpenAIQuotaResetAttemptError{
			Kind: OpenAIQuotaResetErrorUpstream, UpstreamStatus: http.StatusInternalServerError, Retryable: true,
			Err: errors.New(`upstream echoed {"credit_id":"private-credit","redeem_request_id":"private-request"}`),
		}
	}}

	result := (&openAIQuotaResetTaskHandler{quotaService: upstream}).Execute(
		context.Background(),
		&BackgroundTaskExecution{repo: &backgroundTaskRepoStub{}, task: task, owner: "worker"},
	)
	require.NotNil(t, result.RetryAt)
	require.Equal(t, "upstream reset returned HTTP 500", result.ErrorMessage)
	require.NotContains(t, result.ErrorMessage, "private-credit")
	require.NotContains(t, result.ErrorMessage, "private-request")
}

func TestOpenAIQuotaResetHandlerLaterDefinitiveFailurePreservesAmbiguousLock(t *testing.T) {
	expiresAt := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
	payload, err := json.Marshal(openAIQuotaResetTaskPayload{
		AccountID: 94, CreditID: "credit-94", CreditExpiresAt: expiresAt.Format(time.RFC3339),
	})
	require.NoError(t, err)
	requestID := "request-94"
	firstDispatch := time.Now().Add(-time.Minute)
	task := &BackgroundTaskRun{
		ID: 94, ResourceType: "openai_account", ResourceID: "94", DedupeKey: "account:94:credit:credit-94",
		Payload: payload, Status: BackgroundTaskStatusRunning, DispatchCount: 1, MaxAttempts: 5,
		IdempotencyKey: &requestID, FirstDispatchAt: &firstDispatch,
	}
	upstream := &backgroundTaskOpenAIStub{reset: func(context.Context, int64, string, string) (*OpenAIQuotaResetResult, error) {
		return nil, &OpenAIQuotaResetAttemptError{
			Kind: OpenAIQuotaResetErrorAuth, Retryable: false, Err: errors.New("authentication rejected"),
		}
	}}

	result := (&openAIQuotaResetTaskHandler{quotaService: upstream}).Execute(
		context.Background(),
		&BackgroundTaskExecution{repo: &backgroundTaskRepoStub{}, task: task, owner: "worker"},
	)
	require.Equal(t, BackgroundTaskStatusIndeterminate, result.Status)
	require.Equal(t, "auth", result.ErrorCode)
	require.False(t, result.ReleaseDedupeLock)
}

func TestOpenAIQuotaResetHandlerFirstUncertainHTTPFailureStaysLocked(t *testing.T) {
	expiresAt := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
	payload, err := json.Marshal(openAIQuotaResetTaskPayload{
		AccountID: 97, CreditID: "credit-97", CreditExpiresAt: expiresAt.Format(time.RFC3339),
	})
	require.NoError(t, err)
	requestID := "request-97"
	task := &BackgroundTaskRun{
		ID: 97, ResourceType: "openai_account", ResourceID: "97", DedupeKey: "account:97:credit:credit-97",
		Payload: payload, Status: BackgroundTaskStatusRunning, MaxAttempts: 1, IdempotencyKey: &requestID,
	}
	upstream := &backgroundTaskOpenAIStub{reset: func(context.Context, int64, string, string) (*OpenAIQuotaResetResult, error) {
		return nil, &OpenAIQuotaResetAttemptError{
			Kind: OpenAIQuotaResetErrorUpstream, UpstreamStatus: http.StatusConflict,
			Retryable: true, Err: errors.New("conflict without a definitive business outcome"),
		}
	}}

	result := (&openAIQuotaResetTaskHandler{quotaService: upstream}).Execute(
		context.Background(),
		&BackgroundTaskExecution{repo: &backgroundTaskRepoStub{}, task: task, owner: "worker"},
	)
	require.Equal(t, BackgroundTaskStatusIndeterminate, result.Status)
	require.False(t, result.ReleaseDedupeLock)
}

func TestOpenAIQuotaResetHandlerFirstDefinitiveValidationFailureUnlocks(t *testing.T) {
	expiresAt := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
	payload, err := json.Marshal(openAIQuotaResetTaskPayload{
		AccountID: 98, CreditID: "credit-98", CreditExpiresAt: expiresAt.Format(time.RFC3339),
	})
	require.NoError(t, err)
	requestID := "request-98"
	task := &BackgroundTaskRun{
		ID: 98, ResourceType: "openai_account", ResourceID: "98", DedupeKey: "account:98:credit:credit-98",
		Payload: payload, Status: BackgroundTaskStatusRunning, MaxAttempts: 5, IdempotencyKey: &requestID,
	}
	upstream := &backgroundTaskOpenAIStub{reset: func(context.Context, int64, string, string) (*OpenAIQuotaResetResult, error) {
		return nil, &OpenAIQuotaResetAttemptError{
			Kind: OpenAIQuotaResetErrorUpstream, UpstreamStatus: http.StatusUnprocessableEntity,
			DefinitiveNoConsumption: true, Err: errors.New("validation rejected"),
		}
	}}

	result := (&openAIQuotaResetTaskHandler{quotaService: upstream}).Execute(
		context.Background(),
		&BackgroundTaskExecution{repo: &backgroundTaskRepoStub{}, task: task, owner: "worker"},
	)
	require.Equal(t, BackgroundTaskStatusFailed, result.Status)
	require.True(t, result.ReleaseDedupeLock)
}

func TestOpenAIQuotaResetHandlerCanceledRequestRemainsIndeterminate(t *testing.T) {
	expiresAt := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
	payload, err := json.Marshal(openAIQuotaResetTaskPayload{
		AccountID: 91, CreditID: "credit-91", CreditExpiresAt: expiresAt.Format(time.RFC3339),
	})
	require.NoError(t, err)
	requestID := "request-91"
	task := &BackgroundTaskRun{
		ID: 91, ResourceType: "openai_account", ResourceID: "91", DedupeKey: "account:91:credit:credit-91",
		Payload: payload, Status: BackgroundTaskStatusRunning,
		MaxAttempts: 5, IdempotencyKey: &requestID,
	}
	upstream := &backgroundTaskOpenAIStub{reset: func(context.Context, int64, string, string) (*OpenAIQuotaResetResult, error) {
		return nil, &OpenAIQuotaResetAttemptError{
			Kind: OpenAIQuotaResetErrorRequest, Retryable: false, Err: context.Canceled,
		}
	}}
	result := (&openAIQuotaResetTaskHandler{quotaService: upstream}).Execute(
		context.Background(),
		&BackgroundTaskExecution{repo: &backgroundTaskRepoStub{}, task: task, owner: "worker"},
	)
	require.Equal(t, BackgroundTaskStatusIndeterminate, result.Status)
	require.False(t, result.ReleaseDedupeLock)
}

func TestOpenAIQuotaResetHandlerRejectsMismatchedResourceBeforeDispatch(t *testing.T) {
	expiresAt := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
	payload, err := json.Marshal(openAIQuotaResetTaskPayload{
		AccountID: 92, CreditID: "credit-92", CreditExpiresAt: expiresAt.Format(time.RFC3339),
	})
	require.NoError(t, err)
	requestID := "request-92"
	task := &BackgroundTaskRun{
		ID: 92, ResourceType: "openai_account", ResourceID: "another-account",
		DedupeKey: "account:92:credit:credit-92", Payload: payload,
		Status: BackgroundTaskStatusRunning, MaxAttempts: 5, IdempotencyKey: &requestID,
	}
	repo := &backgroundTaskRepoStub{}
	upstream := &backgroundTaskOpenAIStub{reset: func(context.Context, int64, string, string) (*OpenAIQuotaResetResult, error) {
		require.FailNow(t, "mismatched task reached upstream")
		return nil, nil
	}}
	result := (&openAIQuotaResetTaskHandler{quotaService: upstream}).Execute(
		context.Background(),
		&BackgroundTaskExecution{repo: repo, task: task, owner: "worker"},
	)
	require.Equal(t, BackgroundTaskStatusFailed, result.Status)
	require.Equal(t, "invalid_task_binding", result.ErrorCode)
	require.True(t, result.ReleaseDedupeLock)
	require.Zero(t, repo.beginDispatches)
}

func TestOpenAIQuotaResetHandlerBusinessOutcomes(t *testing.T) {
	tests := []struct {
		name        string
		code        string
		wantStatus  BackgroundTaskStatus
		wantRelease bool
	}{
		{name: "reset", code: "reset", wantStatus: BackgroundTaskStatusSucceeded},
		{name: "already redeemed", code: "already_redeemed", wantStatus: BackgroundTaskStatusSucceeded},
		{name: "nothing to reset", code: "nothing_to_reset", wantStatus: BackgroundTaskStatusSkipped, wantRelease: true},
		{name: "no credit", code: "no_credit", wantStatus: BackgroundTaskStatusSkipped, wantRelease: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			expiresAt := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
			payload, err := json.Marshal(openAIQuotaResetTaskPayload{
				AccountID: 10, CreditID: "credit", CreditExpiresAt: expiresAt.Format(time.RFC3339),
			})
			require.NoError(t, err)
			requestID := "request"
			task := &BackgroundTaskRun{
				ID: 10, ResourceType: "openai_account", ResourceID: "10", DedupeKey: "account:10:credit:credit",
				Payload: payload, Status: BackgroundTaskStatusRunning,
				MaxAttempts: 5, IdempotencyKey: &requestID,
			}
			upstream := &backgroundTaskOpenAIStub{reset: func(context.Context, int64, string, string) (*OpenAIQuotaResetResult, error) {
				return &OpenAIQuotaResetResult{Code: test.code}, nil
			}}
			result := (&openAIQuotaResetTaskHandler{quotaService: upstream}).Execute(
				context.Background(),
				&BackgroundTaskExecution{repo: &backgroundTaskRepoStub{}, task: task, owner: "worker"},
			)
			require.Equal(t, test.wantStatus, result.Status)
			require.Equal(t, test.wantRelease, result.ReleaseDedupeLock)
		})
	}
}

func TestOpenAIQuotaResetHandlerNoOpAfterAmbiguousDispatchStaysLocked(t *testing.T) {
	expiresAt := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
	payload, err := json.Marshal(openAIQuotaResetTaskPayload{
		AccountID: 11, CreditID: "credit", CreditExpiresAt: expiresAt.Format(time.RFC3339),
	})
	require.NoError(t, err)
	requestID := "request"
	firstDispatch := time.Now().Add(-time.Minute)
	task := &BackgroundTaskRun{
		ID: 11, ResourceType: "openai_account", ResourceID: "11", DedupeKey: "account:11:credit:credit",
		Payload: payload, Status: BackgroundTaskStatusRunning,
		DispatchCount: 1, MaxAttempts: 5, IdempotencyKey: &requestID, FirstDispatchAt: &firstDispatch,
	}
	upstream := &backgroundTaskOpenAIStub{reset: func(context.Context, int64, string, string) (*OpenAIQuotaResetResult, error) {
		return &OpenAIQuotaResetResult{Code: "nothing_to_reset"}, nil
	}}
	result := (&openAIQuotaResetTaskHandler{quotaService: upstream}).Execute(
		context.Background(),
		&BackgroundTaskExecution{repo: &backgroundTaskRepoStub{}, task: task, owner: "worker"},
	)
	require.Equal(t, BackgroundTaskStatusIndeterminate, result.Status)
	require.False(t, result.ReleaseDedupeLock)
}

func TestOpenAIQuotaResetHandlerAutomaticRetryDoesNotCrossExpiry(t *testing.T) {
	expiresAt := time.Now().Add(5 * time.Second).UTC().Truncate(time.Second)
	payload, err := json.Marshal(openAIQuotaResetTaskPayload{
		AccountID: 12, CreditID: "credit", CreditExpiresAt: expiresAt.Format(time.RFC3339),
	})
	require.NoError(t, err)
	requestID := "request"
	task := &BackgroundTaskRun{
		ID: 12, ResourceType: "openai_account", ResourceID: "12", DedupeKey: "account:12:credit:credit",
		Payload: payload, Status: BackgroundTaskStatusRunning,
		MaxAttempts: 5, IdempotencyKey: &requestID,
	}
	upstream := &backgroundTaskOpenAIStub{reset: func(context.Context, int64, string, string) (*OpenAIQuotaResetResult, error) {
		return nil, &OpenAIQuotaResetAttemptError{
			Kind: OpenAIQuotaResetErrorRequest, Retryable: true, Err: errors.New("timeout"),
		}
	}}
	result := (&openAIQuotaResetTaskHandler{quotaService: upstream}).Execute(
		context.Background(),
		&BackgroundTaskExecution{repo: &backgroundTaskRepoStub{}, task: task, owner: "worker"},
	)
	require.Nil(t, result.RetryAt)
	require.Equal(t, BackgroundTaskStatusIndeterminate, result.Status)
}

func TestOpenAIQuotaResetHandlerManualRetryAfterExpiryUsesOriginalPair(t *testing.T) {
	expiresAt := time.Now().Add(-time.Minute).UTC().Truncate(time.Second)
	payload, err := json.Marshal(openAIQuotaResetTaskPayload{
		AccountID: 13, CreditID: "expired-credit", CreditExpiresAt: expiresAt.Format(time.RFC3339),
	})
	require.NoError(t, err)
	requestID := "original-request"
	firstDispatch := time.Now().Add(-2 * time.Minute)
	task := &BackgroundTaskRun{
		ID: 13, ResourceType: "openai_account", ResourceID: "13", DedupeKey: "account:13:credit:expired-credit",
		Payload: payload, Status: BackgroundTaskStatusRunning,
		RunAt: time.Now(), DispatchCount: 1, MaxAttempts: 5,
		IdempotencyKey: &requestID, FirstDispatchAt: &firstDispatch,
	}
	var gotRequestID, gotCreditID string
	upstream := &backgroundTaskOpenAIStub{reset: func(_ context.Context, _ int64, requestID, creditID string) (*OpenAIQuotaResetResult, error) {
		gotRequestID, gotCreditID = requestID, creditID
		return &OpenAIQuotaResetResult{Code: "alreadyRedeemed"}, nil
	}}
	result := (&openAIQuotaResetTaskHandler{quotaService: upstream}).Execute(
		context.Background(),
		&BackgroundTaskExecution{repo: &backgroundTaskRepoStub{}, task: task, owner: "worker"},
	)
	require.Equal(t, BackgroundTaskStatusSucceeded, result.Status)
	require.Equal(t, "original-request", gotRequestID)
	require.Equal(t, "expired-credit", gotCreditID)
}

func TestCreateOpenAIQuotaResetSelectsNearestCreditAndRequestsDurableKey(t *testing.T) {
	now := time.Now().UTC()
	nearestExpiry := now.Add(90 * time.Minute).Truncate(time.Second)
	api := &backgroundTaskOpenAIStub{candidates: []OpenAIQuotaResetCreditCandidate{
		{ID: "later-credit", ExpiresAt: now.Add(3 * time.Hour)},
		{ID: "expired-credit", ExpiresAt: now.Add(-time.Minute)},
		{ID: "nearest-credit", ExpiresAt: nearestExpiry},
	}}
	repo := &backgroundTaskRepoStub{}
	service := &BackgroundTaskService{
		repo: repo, accountRepo: &backgroundTaskAccountRepoStub{account: &Account{ID: 42, Name: "codex-primary"}},
		quotaService: api,
	}

	task, created, err := service.CreateOpenAIQuotaReset(context.Background(), 42, nearestExpiry, 60, "create-reset-42", 101)
	require.NoError(t, err)
	require.True(t, created)
	require.NotNil(t, task)
	require.NotNil(t, repo.createdInput)
	require.True(t, repo.createdInput.GenerateIdempotencyKey)
	require.Nil(t, repo.createdInput.IdempotencyKey)
	require.Equal(t, "create-reset-42", repo.createdInput.CreationRequestKey)
	require.WithinDuration(t, nearestExpiry.Add(-time.Hour), repo.createdInput.RunAt, time.Second)
	require.Equal(t, "account:42:credit:nearest-credit", repo.createdInput.DedupeKey)

	var payload openAIQuotaResetTaskPayload
	require.NoError(t, json.Unmarshal(repo.createdInput.Payload, &payload))
	require.Equal(t, "nearest-credit", payload.CreditID)
	require.Equal(t, int64(42), payload.AccountID)

	encoded, err := json.Marshal(PublicBackgroundTask(task))
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "nearest-credit")
	require.NotContains(t, string(encoded), "persisted-redeem-request-id")
}

func TestCreateOpenAIQuotaResetImmediateAndChangedCredit(t *testing.T) {
	now := time.Now().UTC()
	expiresAt := now.Add(30 * time.Minute).Truncate(time.Second)
	api := &backgroundTaskOpenAIStub{candidates: []OpenAIQuotaResetCreditCandidate{{ID: "credit", ExpiresAt: expiresAt}}}
	repo := &backgroundTaskRepoStub{}
	service := &BackgroundTaskService{
		repo: repo, accountRepo: &backgroundTaskAccountRepoStub{account: &Account{ID: 8, Name: "account"}},
		quotaService: api,
	}

	before := time.Now()
	_, _, err := service.CreateOpenAIQuotaReset(context.Background(), 8, expiresAt, 60, "create-reset-immediate", 3)
	require.NoError(t, err)
	require.WithinDuration(t, before, repo.createdInput.RunAt, 2*time.Second)

	_, _, err = service.CreateOpenAIQuotaReset(context.Background(), 8, expiresAt.Add(time.Second), 10, "create-reset-changed", 3)
	require.Error(t, err)
	require.Equal(t, 409, infraerrors.Code(err))
}

func TestCreateOpenAIQuotaResetReplaysCreationRequestBeforeQueryingUpstream(t *testing.T) {
	existing := &BackgroundTaskRun{
		ID: 77, TaskType: BackgroundTaskTypeOpenAIQuotaReset,
		ResourceType: "openai_account", ResourceID: "42",
		Status: BackgroundTaskStatusCanceled,
	}
	repo := &backgroundTaskRepoStub{tasksByCreationKey: map[string]*BackgroundTaskRun{
		"same-create-request": existing,
	}}
	api := &backgroundTaskOpenAIStub{}
	service := &BackgroundTaskService{repo: repo, quotaService: api}

	task, created, err := service.CreateOpenAIQuotaReset(
		context.Background(), 42, time.Time{}, 999, " same-create-request ", 101,
	)
	require.NoError(t, err)
	require.False(t, created)
	require.Same(t, existing, task)
	require.Equal(t, 0, api.listCalls)
	require.Nil(t, repo.createdInput)
}

func TestCreateOpenAIQuotaResetRejectsCreationRequestKeyUsedForAnotherResource(t *testing.T) {
	existing := &BackgroundTaskRun{
		ID: 77, TaskType: BackgroundTaskTypeOpenAIQuotaReset,
		ResourceType: "openai_account", ResourceID: "99",
	}
	repo := &backgroundTaskRepoStub{tasksByCreationKey: map[string]*BackgroundTaskRun{
		"reused-create-request": existing,
	}}
	api := &backgroundTaskOpenAIStub{}
	service := &BackgroundTaskService{repo: repo, quotaService: api}

	_, _, err := service.CreateOpenAIQuotaReset(
		context.Background(), 42, time.Time{}, 60, "reused-create-request", 101,
	)
	require.Error(t, err)
	require.Equal(t, 409, infraerrors.Code(err))
	require.Equal(t, "BACKGROUND_TASK_CREATION_KEY_CONFLICT", infraerrors.Reason(err))
	require.Equal(t, 0, api.listCalls)
}

func TestCreateOpenAIQuotaResetValidatesCreationRequestKey(t *testing.T) {
	service := &BackgroundTaskService{repo: &backgroundTaskRepoStub{}}

	_, _, err := service.CreateOpenAIQuotaReset(context.Background(), 42, time.Time{}, 60, " ", 101)
	require.Error(t, err)
	require.Equal(t, 400, infraerrors.Code(err))

	_, _, err = service.CreateOpenAIQuotaReset(
		context.Background(), 42, time.Time{}, 60,
		strings.Repeat("k", BackgroundTaskCreationRequestKeyMaxLength+1), 101,
	)
	require.Error(t, err)
	require.Equal(t, 400, infraerrors.Code(err))
}

func TestPublicBackgroundTaskAllowsCancelBeforeRunningDispatchAndRedactsPayload(t *testing.T) {
	requestID := "secret-redeem-request-id"
	creationRequestKey := "secret-create-request-id"
	actorID := int64(99)
	task := &BackgroundTaskRun{
		ID: 5, TaskType: BackgroundTaskTypeOpenAIQuotaReset,
		ResourceType: "openai_account", ResourceID: "42",
		Payload:            json.RawMessage(`{"account_id":42,"credit_id":"secret-credit-id","oauth":"secret"}`),
		Display:            json.RawMessage(`{"account_id":42,"account_name":"safe-account","credit_expires_at":"2030-01-01T00:00:00Z"}`),
		Status:             BackgroundTaskStatusRunning,
		IdempotencyKey:     &requestID,
		CreationRequestKey: &creationRequestKey,
		CreatedBy:          actorID,
		CanceledBy:         &actorID,
	}
	public := PublicBackgroundTask(task)
	require.True(t, public.CanCancel)
	require.False(t, public.CanRetry)

	encoded, err := json.Marshal(public)
	require.NoError(t, err)
	require.Contains(t, string(encoded), "safe-account")
	require.NotContains(t, string(encoded), "secret-credit-id")
	require.NotContains(t, string(encoded), requestID)
	require.NotContains(t, string(encoded), creationRequestKey)
	require.NotContains(t, string(encoded), "oauth")
	require.NotContains(t, string(encoded), "created_by")
	require.NotContains(t, string(encoded), "canceled_by")

	dispatchedAt := time.Now()
	task.FirstDispatchAt = &dispatchedAt
	require.False(t, PublicBackgroundTask(task).CanCancel)
	task.Status = BackgroundTaskStatusIndeterminate
	require.True(t, PublicBackgroundTask(task).CanRetry)
}

func stringPointer(value string) *string {
	return &value
}
