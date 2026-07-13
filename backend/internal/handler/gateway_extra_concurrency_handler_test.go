//go:build unit

package handler

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	middleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type extraConcurrencySettingRepository struct {
	waitTimeoutSeconds string
	enabledFlag        *atomic.Bool
}

func (extraConcurrencySettingRepository) Get(context.Context, string) (*service.Setting, error) {
	return nil, nil
}
func (extraConcurrencySettingRepository) GetValue(context.Context, string) (string, error) {
	return "", nil
}
func (extraConcurrencySettingRepository) Set(context.Context, string, string) error { return nil }

func (r extraConcurrencySettingRepository) GetMultiple(context.Context, []string) (map[string]string, error) {
	waitTimeoutSeconds := r.waitTimeoutSeconds
	if waitTimeoutSeconds == "" {
		waitTimeoutSeconds = "1"
	}
	enabled := "true"
	if r.enabledFlag != nil && !r.enabledFlag.Load() {
		enabled = "false"
	}
	return map[string]string{
		service.SettingKeyExtraConcurrencyEnabled:            enabled,
		service.SettingKeyExtraConcurrencyWaitTimeoutSeconds: waitTimeoutSeconds,
		service.SettingKeyExtraConcurrencyReservePercent:     "0",
		service.SettingKeyExtraConcurrencyMinReservedSlots:   "0",
		service.SettingKeyExtraConcurrencyPlatformReserves:   "{}",
	}, nil
}
func (extraConcurrencySettingRepository) SetMultiple(context.Context, map[string]string) error {
	return nil
}
func (extraConcurrencySettingRepository) GetAll(context.Context) (map[string]string, error) {
	return nil, nil
}
func (extraConcurrencySettingRepository) Delete(context.Context, string) error { return nil }

type expiringTargetAdmissionStore struct{}

func (expiringTargetAdmissionStore) TryAcquireUserLease(context.Context, service.UserLeaseRequest) (service.UserLeaseResult, error) {
	return service.UserLeaseResult{Acquired: true, Class: service.AdmissionClassExtra}, nil
}
func (expiringTargetAdmissionStore) RenewUserLease(context.Context, int64, string, service.AdmissionClass) (bool, error) {
	return true, nil
}
func (expiringTargetAdmissionStore) ReleaseUserLease(context.Context, int64, string) error {
	return nil
}
func (expiringTargetAdmissionStore) TryAcquireTargetLease(context.Context, service.TargetLeaseRequest) (service.TargetLeaseResult, error) {
	return service.TargetLeaseResult{Expired: true}, nil
}

func (expiringTargetAdmissionStore) BeginTargetDispatch(context.Context, service.TargetDispatchRequest) (service.TargetDispatchResult, error) {
	return service.TargetDispatchResult{Started: true}, nil
}
func (expiringTargetAdmissionStore) RenewTargetLease(context.Context, string, int64, string) (bool, error) {
	return true, nil
}
func (expiringTargetAdmissionStore) ReleaseTargetLease(context.Context, string, int64, string) error {
	return nil
}

type waitThenAcquireAdmissionStore struct {
	targetAttempts atomic.Int32
}

func (s *waitThenAcquireAdmissionStore) TryAcquireUserLease(context.Context, service.UserLeaseRequest) (service.UserLeaseResult, error) {
	return service.UserLeaseResult{Acquired: true, Class: service.AdmissionClassExtra}, nil
}
func (s *waitThenAcquireAdmissionStore) RenewUserLease(context.Context, int64, string, service.AdmissionClass) (bool, error) {
	return true, nil
}
func (s *waitThenAcquireAdmissionStore) ReleaseUserLease(context.Context, int64, string) error {
	return nil
}
func (s *waitThenAcquireAdmissionStore) TryAcquireTargetLease(context.Context, service.TargetLeaseRequest) (service.TargetLeaseResult, error) {
	return service.TargetLeaseResult{Acquired: s.targetAttempts.Add(1) > 1}, nil
}

func (s *waitThenAcquireAdmissionStore) BeginTargetDispatch(context.Context, service.TargetDispatchRequest) (service.TargetDispatchResult, error) {
	return service.TargetDispatchResult{Started: true}, nil
}
func (s *waitThenAcquireAdmissionStore) RenewTargetLease(context.Context, string, int64, string) (bool, error) {
	return true, nil
}
func (s *waitThenAcquireAdmissionStore) ReleaseTargetLease(context.Context, string, int64, string) error {
	return nil
}

type changingBalanceCache struct {
	service.BillingCache
	reads atomic.Int32
}

type fixedBalanceCache struct {
	service.BillingCache
}

func (fixedBalanceCache) GetUserBalance(context.Context, int64) (float64, error) {
	return 100, nil
}

type countingUserRPMCache struct {
	count          atomic.Int32
	incrementCalls atomic.Int32
}

type exhaustedAPIKeyRepo struct {
	service.APIKeyRepository
	apiKey *service.APIKey
	reads  atomic.Int32
}

func (r *exhaustedAPIKeyRepo) GetByID(context.Context, int64) (*service.APIKey, error) {
	r.reads.Add(1)
	return r.apiKey, nil
}

func (c *countingUserRPMCache) IncrementUserGroupRPM(context.Context, int64, int64) (int, error) {
	return 0, nil
}

func (c *countingUserRPMCache) IncrementUserRPM(context.Context, int64) (int, error) {
	c.incrementCalls.Add(1)
	return int(c.count.Add(1)), nil
}

func (c *countingUserRPMCache) GetUserGroupRPM(context.Context, int64, int64) (int, error) {
	return 0, nil
}

func (c *countingUserRPMCache) GetUserRPM(context.Context, int64) (int, error) {
	return int(c.count.Load()), nil
}

type reusableAdmissionStore struct {
	mu              sync.Mutex
	userRequestID   string
	targetRequestID string
}

type websocketExtraAdmissionStore struct {
	userClaims     atomic.Int32
	targetClaims   atomic.Int32
	userReleases   atomic.Int32
	targetReleases atomic.Int32
}

type websocketRetargetAccountRepo struct {
	service.AccountRepository
	initial     *service.Account
	replacement *service.Account
}

func (r *websocketRetargetAccountRepo) ListSchedulableByPlatform(context.Context, string) ([]service.Account, error) {
	if r.initial != nil && r.initial.IsSchedulable() {
		return []service.Account{*r.initial}, nil
	}
	if r.replacement != nil && r.replacement.IsSchedulable() {
		return []service.Account{*r.replacement}, nil
	}
	return nil, nil
}

func (r *websocketRetargetAccountRepo) ListSchedulableByGroupIDAndPlatform(ctx context.Context, _ int64, platform string) ([]service.Account, error) {
	return r.ListSchedulableByPlatform(ctx, platform)
}

func (r *websocketRetargetAccountRepo) GetByID(_ context.Context, id int64) (*service.Account, error) {
	for _, account := range []*service.Account{r.initial, r.replacement} {
		if account != nil && account.ID == id {
			copy := *account
			return &copy, nil
		}
	}
	return nil, nil
}

type blockingFollowUpAdmissionStore struct {
	userClaims     atomic.Int32
	targetClaims   atomic.Int32
	userReleases   atomic.Int32
	targetReleases atomic.Int32

	secondTargetStarted chan struct{}
	secondTargetOnce    sync.Once
}

func (s *websocketExtraAdmissionStore) TryAcquireUserLease(context.Context, service.UserLeaseRequest) (service.UserLeaseResult, error) {
	s.userClaims.Add(1)
	return service.UserLeaseResult{Acquired: true, Class: service.AdmissionClassExtra}, nil
}
func (s *websocketExtraAdmissionStore) RenewUserLease(context.Context, int64, string, service.AdmissionClass) (bool, error) {
	return true, nil
}
func (s *websocketExtraAdmissionStore) ReleaseUserLease(context.Context, int64, string) error {
	s.userReleases.Add(1)
	return nil
}
func (s *websocketExtraAdmissionStore) TryAcquireTargetLease(context.Context, service.TargetLeaseRequest) (service.TargetLeaseResult, error) {
	s.targetClaims.Add(1)
	return service.TargetLeaseResult{Acquired: true}, nil
}

func (s *websocketExtraAdmissionStore) BeginTargetDispatch(context.Context, service.TargetDispatchRequest) (service.TargetDispatchResult, error) {
	return service.TargetDispatchResult{Started: true}, nil
}
func (s *websocketExtraAdmissionStore) RenewTargetLease(context.Context, string, int64, string) (bool, error) {
	return true, nil
}
func (s *websocketExtraAdmissionStore) ReleaseTargetLease(context.Context, string, int64, string) error {
	s.targetReleases.Add(1)
	return nil
}

func (s *blockingFollowUpAdmissionStore) TryAcquireUserLease(context.Context, service.UserLeaseRequest) (service.UserLeaseResult, error) {
	s.userClaims.Add(1)
	return service.UserLeaseResult{Acquired: true, Class: service.AdmissionClassExtra}, nil
}
func (s *blockingFollowUpAdmissionStore) RenewUserLease(context.Context, int64, string, service.AdmissionClass) (bool, error) {
	return true, nil
}
func (s *blockingFollowUpAdmissionStore) ReleaseUserLease(context.Context, int64, string) error {
	s.userReleases.Add(1)
	return nil
}
func (s *blockingFollowUpAdmissionStore) TryAcquireTargetLease(context.Context, service.TargetLeaseRequest) (service.TargetLeaseResult, error) {
	if s.targetClaims.Add(1) == 1 {
		return service.TargetLeaseResult{Acquired: true}, nil
	}
	s.secondTargetOnce.Do(func() {
		close(s.secondTargetStarted)
	})
	return service.TargetLeaseResult{}, nil
}

func (s *blockingFollowUpAdmissionStore) BeginTargetDispatch(context.Context, service.TargetDispatchRequest) (service.TargetDispatchResult, error) {
	return service.TargetDispatchResult{Started: true}, nil
}
func (s *blockingFollowUpAdmissionStore) RenewTargetLease(context.Context, string, int64, string) (bool, error) {
	return true, nil
}
func (s *blockingFollowUpAdmissionStore) ReleaseTargetLease(context.Context, string, int64, string) error {
	s.targetReleases.Add(1)
	return nil
}

func (s *reusableAdmissionStore) TryAcquireUserLease(_ context.Context, request service.UserLeaseRequest) (service.UserLeaseResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.userRequestID == "" {
		s.userRequestID = request.RequestID
	}
	return service.UserLeaseResult{
		Acquired: s.userRequestID == request.RequestID,
		Class:    service.AdmissionClassStandard,
	}, nil
}
func (s *reusableAdmissionStore) RenewUserLease(_ context.Context, _ int64, requestID string, _ service.AdmissionClass) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.userRequestID == requestID, nil
}
func (s *reusableAdmissionStore) ReleaseUserLease(_ context.Context, _ int64, requestID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.userRequestID == requestID {
		s.userRequestID = ""
	}
	return nil
}
func (s *reusableAdmissionStore) TryAcquireTargetLease(_ context.Context, request service.TargetLeaseRequest) (service.TargetLeaseResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.targetRequestID == "" {
		s.targetRequestID = request.RequestID
	}
	return service.TargetLeaseResult{Acquired: s.targetRequestID == request.RequestID}, nil
}

func (s *reusableAdmissionStore) BeginTargetDispatch(_ context.Context, request service.TargetDispatchRequest) (service.TargetDispatchResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return service.TargetDispatchResult{Started: s.targetRequestID == request.RequestID}, nil
}
func (s *reusableAdmissionStore) RenewTargetLease(_ context.Context, _ string, _ int64, requestID string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.targetRequestID == requestID, nil
}
func (s *reusableAdmissionStore) ReleaseTargetLease(_ context.Context, _ string, _ int64, requestID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.targetRequestID == requestID {
		s.targetRequestID = ""
	}
	return nil
}

type successfulAnthropicUpstream struct {
	calls atomic.Int32
}

type usageLogWriteRecorder struct {
	service.UsageLogRepository
	writes                 atomic.Int32
	lastAccountID          atomic.Int64
	contextAccountID       atomic.Int64
	contextAccountPlatform atomic.Value
}

func (r *usageLogWriteRecorder) Create(ctx context.Context, log *service.UsageLog) (bool, error) {
	r.writes.Add(1)
	r.recordContext(ctx)
	if log != nil {
		r.lastAccountID.Store(log.AccountID)
	}
	return true, nil
}

func (r *usageLogWriteRecorder) CreateBestEffort(ctx context.Context, log *service.UsageLog) error {
	r.writes.Add(1)
	r.recordContext(ctx)
	if log != nil {
		r.lastAccountID.Store(log.AccountID)
	}
	return nil
}

func (r *usageLogWriteRecorder) recordContext(ctx context.Context) {
	if ctx == nil {
		return
	}
	if accountID, ok := ctx.Value(ctxkey.AccountID).(int64); ok {
		r.contextAccountID.Store(accountID)
	}
	if platform, ok := ctx.Value(ctxkey.Platform).(string); ok {
		r.contextAccountPlatform.Store(platform)
	}
}

type dispatchRetargetAdmissionStore struct {
	initialAccountID          int64
	replacementAccountID      int64
	initialAccount            *service.Account
	userReleases              *atomic.Int32
	initialTargetReleases     *atomic.Int32
	replacementTargetReleases *atomic.Int32
}

func (s dispatchRetargetAdmissionStore) TryAcquireUserLease(_ context.Context, request service.UserLeaseRequest) (service.UserLeaseResult, error) {
	class := service.AdmissionClassExtra
	if request.ExtraLimit == 0 {
		class = service.AdmissionClassStandard
	}
	return service.UserLeaseResult{Acquired: true, Class: class}, nil
}

func (dispatchRetargetAdmissionStore) RenewUserLease(context.Context, int64, string, service.AdmissionClass) (bool, error) {
	return true, nil
}

func (s dispatchRetargetAdmissionStore) ReleaseUserLease(context.Context, int64, string) error {
	if s.userReleases != nil {
		s.userReleases.Add(1)
	}
	return nil
}

func (s dispatchRetargetAdmissionStore) TryAcquireTargetLease(_ context.Context, request service.TargetLeaseRequest) (service.TargetLeaseResult, error) {
	acquired := request.Class == service.AdmissionClassExtra && request.AccountID == s.initialAccountID
	if request.Class == service.AdmissionClassStandard {
		acquired = request.AccountID == s.replacementAccountID
	}
	return service.TargetLeaseResult{Acquired: acquired}, nil
}

func (s dispatchRetargetAdmissionStore) BeginTargetDispatch(_ context.Context, request service.TargetDispatchRequest) (service.TargetDispatchResult, error) {
	if request.Class == service.AdmissionClassExtra && request.AccountID == s.initialAccountID {
		if s.initialAccount != nil {
			s.initialAccount.Schedulable = false
		}
		return service.TargetDispatchResult{Draining: true}, nil
	}
	return service.TargetDispatchResult{
		Started: request.Class == service.AdmissionClassStandard && request.AccountID == s.replacementAccountID,
	}, nil
}

func (dispatchRetargetAdmissionStore) RenewTargetLease(context.Context, string, int64, string) (bool, error) {
	return true, nil
}

func (s dispatchRetargetAdmissionStore) ReleaseTargetLease(_ context.Context, _ string, accountID int64, _ string) error {
	if accountID == s.initialAccountID && s.initialTargetReleases != nil {
		s.initialTargetReleases.Add(1)
	}
	if accountID == s.replacementAccountID && s.replacementTargetReleases != nil {
		s.replacementTargetReleases.Add(1)
	}
	return nil
}

type dispatchRetargetCapacity struct {
	accountIDs []int64
}

func (c dispatchRetargetCapacity) AdmissionCapacity(context.Context, string) (service.AdmissionCapacitySnapshot, error) {
	accounts := make(map[int64]int, len(c.accountIDs))
	for _, accountID := range c.accountIDs {
		accounts[accountID] = 1
	}
	return service.AdmissionCapacitySnapshot{
		TotalConcurrency:   len(accounts),
		AccountConcurrency: accounts,
	}, nil
}

type accountCapturingAnthropicUpstream struct {
	accountID atomic.Int64
}

type fixedSessionGatewayCache struct {
	accountID int64
}

func (c fixedSessionGatewayCache) GetSessionAccountID(context.Context, int64, string) (int64, error) {
	return c.accountID, nil
}

func (fixedSessionGatewayCache) SetSessionAccountID(context.Context, int64, string, int64, time.Duration) error {
	return nil
}

func (fixedSessionGatewayCache) RefreshSessionTTL(context.Context, int64, string, time.Duration) error {
	return nil
}

func (fixedSessionGatewayCache) DeleteSessionAccountID(context.Context, int64, string) error {
	return nil
}

type geminiNativeBodyCaptureUpstream struct {
	mu        sync.Mutex
	accountID atomic.Int64
	body      []byte
}

func (u *geminiNativeBodyCaptureUpstream) Do(req *http.Request, _ string, accountID int64, _ int) (*http.Response, error) {
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	u.mu.Lock()
	u.body = append([]byte(nil), body...)
	u.mu.Unlock()
	u.accountID.Store(accountID)
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body: io.NopCloser(strings.NewReader(
			`{"candidates":[{"content":{"parts":[{"text":"ok"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":1,"candidatesTokenCount":1}}`,
		)),
	}, nil
}

func (u *geminiNativeBodyCaptureUpstream) DoWithTLS(
	req *http.Request,
	proxyURL string,
	accountID int64,
	accountConcurrency int,
	_ *tlsfingerprint.Profile,
) (*http.Response, error) {
	return u.Do(req, proxyURL, accountID, accountConcurrency)
}

func (u *geminiNativeBodyCaptureUpstream) capturedBody() []byte {
	u.mu.Lock()
	defer u.mu.Unlock()
	return append([]byte(nil), u.body...)
}

type accountRPMRecorder struct {
	accountID atomic.Int64
}

type recordingUserMsgQueueCache struct {
	mu       sync.Mutex
	acquired []int64
	released []int64
}

func (c *recordingUserMsgQueueCache) AcquireLock(_ context.Context, accountID int64, _ string, _ int) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.acquired = append(c.acquired, accountID)
	return true, nil
}

func (c *recordingUserMsgQueueCache) ReleaseLock(_ context.Context, accountID int64, _ string) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.released = append(c.released, accountID)
	return true, nil
}

func (*recordingUserMsgQueueCache) GetLastCompletedMs(context.Context, int64) (int64, error) {
	return 0, nil
}

func (*recordingUserMsgQueueCache) GetCurrentTimeMs(context.Context) (int64, error) {
	return 0, nil
}

func (*recordingUserMsgQueueCache) ReconcileExpiredLockCandidates(context.Context, int) (int, error) {
	return 0, nil
}

func (c *recordingUserMsgQueueCache) snapshot() ([]int64, []int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]int64(nil), c.acquired...), append([]int64(nil), c.released...)
}

func (r *accountRPMRecorder) IncrementRPM(_ context.Context, accountID int64) (int, error) {
	r.accountID.Store(accountID)
	return 1, nil
}

func (*accountRPMRecorder) GetRPM(context.Context, int64) (int, error) {
	return 0, nil
}

func (*accountRPMRecorder) GetRPMBatch(_ context.Context, accountIDs []int64) (map[int64]int, error) {
	result := make(map[int64]int, len(accountIDs))
	for _, accountID := range accountIDs {
		result[accountID] = 0
	}
	return result, nil
}

func (u *accountCapturingAnthropicUpstream) Do(_ *http.Request, _ string, accountID int64, _ int) (*http.Response, error) {
	u.accountID.Store(accountID)
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body: io.NopCloser(strings.NewReader(
			`{"id":"msg_retarget","type":"message","role":"assistant","model":"claude-sonnet-4-5","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`,
		)),
	}, nil
}

func (u *accountCapturingAnthropicUpstream) DoWithTLS(
	req *http.Request,
	proxyURL string,
	accountID int64,
	accountConcurrency int,
	_ *tlsfingerprint.Profile,
) (*http.Response, error) {
	return u.Do(req, proxyURL, accountID, accountConcurrency)
}

func (u *successfulAnthropicUpstream) Do(*http.Request, string, int64, int) (*http.Response, error) {
	u.calls.Add(1)
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body: io.NopCloser(strings.NewReader(
			`{"id":"msg_test","type":"message","role":"assistant","model":"claude-sonnet-4-5","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`,
		)),
	}, nil
}
func (u *successfulAnthropicUpstream) DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, proxyURL, accountID, accountConcurrency)
}

func newSuccessfulExtraConcurrencyHandler(
	t *testing.T,
	group *service.Group,
	account *service.Account,
	upstream service.HTTPUpstream,
) (*GatewayHandler, *helperConcurrencyCacheStub, func()) {
	t.Helper()
	schedulerCache := &fakeSchedulerCache{accounts: []*service.Account{account}}
	schedulerSnapshot := service.NewSchedulerSnapshotService(schedulerCache, nil, nil, nil, nil)
	cfg := &config.Config{RunMode: config.RunModeSimple}
	billingCacheService := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
	gatewayService := service.NewGatewayService(
		nil,
		&fakeGroupRepo{group: group},
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		cfg,
		schedulerSnapshot,
		nil,
		nil,
		&service.RateLimitService{},
		billingCacheService,
		nil,
		upstream,
		&service.DeferredService{},
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
	)
	pool := service.NewUsageRecordWorkerPoolWithOptions(service.UsageRecordWorkerPoolOptions{
		WorkerCount:      1,
		QueueSize:        1,
		AutoScaleEnabled: false,
	})
	pool.Stop()
	concurrencyCache := &helperConcurrencyCacheStub{}
	h := &GatewayHandler{
		gatewayService:        gatewayService,
		billingCacheService:   billingCacheService,
		concurrencyHelper:     NewConcurrencyHelper(service.NewConcurrencyService(concurrencyCache), SSEPingFormatClaude, 0),
		usageRecordWorkerPool: pool,
		maxAccountSwitches:    1,
		cfg:                   cfg,
	}
	return h, concurrencyCache, billingCacheService.Stop
}

func (c *changingBalanceCache) GetUserBalance(context.Context, int64) (float64, error) {
	if c.reads.Add(1) == 1 {
		return 100, nil
	}
	return 0, nil
}

type fixedAdmissionCapacity struct {
	accountID int64
}

func (c fixedAdmissionCapacity) AdmissionCapacity(context.Context, string) (service.AdmissionCapacitySnapshot, error) {
	return service.AdmissionCapacitySnapshot{
		TotalConcurrency:   1,
		AccountConcurrency: map[int64]int{c.accountID: 1},
	}, nil
}

func TestOpenAIResponsesWebSocketUsesExtraAdmissionInsteadOfLegacyConcurrency(t *testing.T) {
	gin.SetMode(gin.TestMode)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := coderws.Accept(w, r, &coderws.AcceptOptions{CompressionMode: coderws.CompressionContextTakeover})
		require.NoError(t, err)
		defer func() { _ = conn.CloseNow() }()

		for turn := 1; turn <= 2; turn++ {
			readCtx, cancelRead := context.WithTimeout(r.Context(), 3*time.Second)
			_, _, err = conn.Read(readCtx)
			cancelRead()
			require.NoError(t, err)

			writeCtx, cancelWrite := context.WithTimeout(r.Context(), 3*time.Second)
			response := fmt.Sprintf(`{"type":"response.completed","response":{"id":"resp_extra_ws_%d","model":"gpt-5.1","usage":{"input_tokens":1,"output_tokens":1}}}`, turn)
			err = conn.Write(writeCtx, coderws.MessageText, []byte(response))
			cancelWrite()
			require.NoError(t, err)
		}
		_ = conn.Close(coderws.StatusNormalClosure, "done")
	}))
	defer upstream.Close()

	groupID := int64(5101)
	accountID := int64(6101)
	account := service.Account{
		ID:          accountID,
		Name:        "openai-extra-ws",
		Platform:    service.PlatformOpenAI,
		Type:        service.AccountTypeAPIKey,
		Status:      service.StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{"api_key": "sk-ws-extra", "base_url": upstream.URL},
		Extra: map[string]any{
			"openai_apikey_responses_websockets_v2_enabled": true,
			"openai_apikey_responses_websockets_v2_mode":    service.OpenAIWSIngressModePassthrough,
		},
	}
	cfg := &config.Config{RunMode: config.RunModeSimple}
	cfg.Default.RateMultiplier = 1
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.APIKeyEnabled = true
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
	cfg.Gateway.OpenAIWS.ModeRouterV2Enabled = true
	cfg.Gateway.OpenAIWS.DialTimeoutSeconds = 3
	cfg.Gateway.OpenAIWS.ReadTimeoutSeconds = 3
	cfg.Gateway.OpenAIWS.WriteTimeoutSeconds = 3

	accountRepo := &openAIWSUsageHandlerAccountRepoStub{account: account}
	billingCacheService := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
	defer billingCacheService.Stop()
	gatewayService := service.NewOpenAIGatewayService(
		accountRepo, nil, nil, nil, nil, nil, nil, cfg, nil, nil,
		service.NewBillingService(cfg, nil), nil, billingCacheService, nil,
		&service.DeferredService{}, nil, nil, nil, nil, nil, nil, nil,
	)
	store := &websocketExtraAdmissionStore{}
	legacyCalls := atomic.Int32{}
	legacyCache := &concurrencyCacheMock{
		acquireUserSlotFn: func(context.Context, int64, int, string) (bool, error) {
			legacyCalls.Add(1)
			return false, errors.New("legacy user admission used")
		},
		acquireAccountSlotFn: func(context.Context, int64, int, string) (bool, error) {
			legacyCalls.Add(1)
			return false, errors.New("legacy target admission used")
		},
	}
	h := &OpenAIGatewayHandler{
		gatewayService:      gatewayService,
		billingCacheService: billingCacheService,
		apiKeyService:       &service.APIKeyService{},
		concurrencyHelper:   NewConcurrencyHelper(service.NewConcurrencyService(legacyCache), SSEPingFormatNone, time.Second),
		gatewayAdmission:    service.NewGatewayAdmission(store, nil, fixedAdmissionCapacity{accountID: accountID}),
		maxAccountSwitches:  1,
		cfg:                 cfg,
		settingService:      service.NewSettingService(extraConcurrencySettingRepository{}, cfg),
	}
	apiKey := &service.APIKey{
		ID:      7101,
		UserID:  8101,
		GroupID: &groupID,
		User: &service.User{
			ID:               8101,
			Status:           service.StatusActive,
			Concurrency:      1,
			ExtraConcurrency: 1,
			Balance:          100,
		},
		Group: &service.Group{ID: groupID, Platform: service.PlatformOpenAI, Status: service.StatusActive},
	}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyAPIKey), apiKey)
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{
			UserID:           apiKey.UserID,
			Concurrency:      1,
			ExtraConcurrency: 1,
		})
		c.Next()
	})
	router.GET("/openai/v1/responses", h.ResponsesWebSocket)
	handlerServer := httptest.NewServer(router)
	defer handlerServer.Close()

	dialCtx, cancelDial := context.WithTimeout(context.Background(), 3*time.Second)
	clientConn, _, err := coderws.Dial(dialCtx, "ws"+strings.TrimPrefix(handlerServer.URL, "http")+"/openai/v1/responses", nil)
	cancelDial()
	require.NoError(t, err)
	defer func() { _ = clientConn.CloseNow() }()

	for turn := 1; turn <= 2; turn++ {
		writeCtx, cancelWrite := context.WithTimeout(context.Background(), 3*time.Second)
		err = clientConn.Write(writeCtx, coderws.MessageText, []byte(`{"type":"response.create","model":"gpt-5.1","stream":false}`))
		cancelWrite()
		require.NoError(t, err)

		readCtx, cancelRead := context.WithTimeout(context.Background(), 5*time.Second)
		_, event, readErr := clientConn.Read(readCtx)
		cancelRead()
		require.NoError(t, readErr)
		require.Equal(t, "response.completed", gjson.GetBytes(event, "type").String())
		require.Equal(t, fmt.Sprintf("resp_extra_ws_%d", turn), gjson.GetBytes(event, "response.id").String())
	}
	require.Zero(t, legacyCalls.Load())
	require.Equal(t, int32(2), store.userClaims.Load())
	require.Equal(t, int32(2), store.targetClaims.Load())
	require.Eventually(t, func() bool {
		return store.userReleases.Load() == 2 && store.targetReleases.Load() == 2
	}, time.Second, 10*time.Millisecond)
}

func TestOpenAIResponsesWebSocketDispatchRetargetUsesReplacementAccount(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var initialUpstreamCalls atomic.Int32
	initialUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		initialUpstreamCalls.Add(1)
		conn, err := coderws.Accept(w, r, nil)
		require.NoError(t, err)
		defer func() { _ = conn.CloseNow() }()
		readCtx, cancelRead := context.WithTimeout(r.Context(), 3*time.Second)
		_, _, err = conn.Read(readCtx)
		cancelRead()
		require.NoError(t, err)
		writeCtx, cancelWrite := context.WithTimeout(r.Context(), 3*time.Second)
		err = conn.Write(writeCtx, coderws.MessageText, []byte(`{"type":"response.completed","response":{"id":"resp_initial","model":"gpt-5.1","usage":{"input_tokens":1,"output_tokens":1}}}`))
		cancelWrite()
		require.NoError(t, err)
	}))
	defer initialUpstream.Close()

	var replacementUpstreamCalls atomic.Int32
	replacementUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		replacementUpstreamCalls.Add(1)
		require.Equal(t, "Bearer sk-ws-replacement", r.Header.Get("Authorization"))
		conn, err := coderws.Accept(w, r, nil)
		require.NoError(t, err)
		defer func() { _ = conn.CloseNow() }()
		readCtx, cancelRead := context.WithTimeout(r.Context(), 3*time.Second)
		_, _, err = conn.Read(readCtx)
		cancelRead()
		require.NoError(t, err)
		writeCtx, cancelWrite := context.WithTimeout(r.Context(), 3*time.Second)
		err = conn.Write(writeCtx, coderws.MessageText, []byte(`{"type":"response.completed","response":{"id":"resp_replacement","model":"gpt-5.1","usage":{"input_tokens":1,"output_tokens":1}}}`))
		cancelWrite()
		require.NoError(t, err)
	}))
	defer replacementUpstream.Close()

	groupID := int64(5104)
	initialAccount := &service.Account{
		ID:          6104,
		Name:        "openai-ws-initial",
		Platform:    service.PlatformOpenAI,
		Type:        service.AccountTypeAPIKey,
		Status:      service.StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{"api_key": "sk-ws-initial", "base_url": initialUpstream.URL},
		Extra: map[string]any{
			"openai_apikey_responses_websockets_v2_enabled": true,
			"openai_apikey_responses_websockets_v2_mode":    service.OpenAIWSIngressModePassthrough,
		},
	}
	replacementAccount := &service.Account{
		ID:          6105,
		Name:        "openai-ws-replacement",
		Platform:    service.PlatformOpenAI,
		Type:        service.AccountTypeAPIKey,
		Status:      service.StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{"api_key": "sk-ws-replacement", "base_url": replacementUpstream.URL},
		Extra: map[string]any{
			"openai_apikey_responses_websockets_v2_enabled": true,
			"openai_apikey_responses_websockets_v2_mode":    service.OpenAIWSIngressModePassthrough,
		},
	}
	cfg := &config.Config{RunMode: config.RunModeSimple}
	cfg.Default.RateMultiplier = 1
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.APIKeyEnabled = true
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
	cfg.Gateway.OpenAIWS.ModeRouterV2Enabled = true
	cfg.Gateway.OpenAIWS.DialTimeoutSeconds = 3
	cfg.Gateway.OpenAIWS.ReadTimeoutSeconds = 3
	cfg.Gateway.OpenAIWS.WriteTimeoutSeconds = 3

	accountRepo := &websocketRetargetAccountRepo{initial: initialAccount, replacement: replacementAccount}
	usageRecorder := &usageLogWriteRecorder{}
	billingCacheService := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
	defer billingCacheService.Stop()
	gatewayService := service.NewOpenAIGatewayService(
		accountRepo, usageRecorder, nil, nil, nil, nil, nil, cfg, nil, nil,
		service.NewBillingService(cfg, nil), nil, billingCacheService, nil,
		&service.DeferredService{}, nil, nil, nil, nil, nil, nil, nil,
	)
	var userReleases atomic.Int32
	var initialTargetReleases atomic.Int32
	var replacementTargetReleases atomic.Int32
	store := dispatchRetargetAdmissionStore{
		initialAccountID:          initialAccount.ID,
		replacementAccountID:      replacementAccount.ID,
		initialAccount:            initialAccount,
		userReleases:              &userReleases,
		initialTargetReleases:     &initialTargetReleases,
		replacementTargetReleases: &replacementTargetReleases,
	}
	h := &OpenAIGatewayHandler{
		gatewayService:      gatewayService,
		billingCacheService: billingCacheService,
		apiKeyService:       &service.APIKeyService{},
		concurrencyHelper:   NewConcurrencyHelper(service.NewConcurrencyService(&concurrencyCacheMock{}), SSEPingFormatNone, time.Second),
		gatewayAdmission: service.NewGatewayAdmission(store, nil, dispatchRetargetCapacity{
			accountIDs: []int64{initialAccount.ID, replacementAccount.ID},
		}),
		maxAccountSwitches: 1,
		cfg:                cfg,
		settingService:     service.NewSettingService(extraConcurrencySettingRepository{}, cfg),
	}
	apiKey := &service.APIKey{
		ID:      7104,
		UserID:  8104,
		GroupID: &groupID,
		User: &service.User{
			ID:               8104,
			Status:           service.StatusActive,
			Concurrency:      1,
			ExtraConcurrency: 1,
			Balance:          100,
		},
		Group: &service.Group{ID: groupID, Platform: service.PlatformOpenAI, Status: service.StatusActive},
	}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyAPIKey), apiKey)
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{
			UserID:           apiKey.UserID,
			Concurrency:      1,
			ExtraConcurrency: 1,
		})
		c.Next()
	})
	router.GET("/openai/v1/responses", h.ResponsesWebSocket)
	handlerServer := httptest.NewServer(router)
	defer handlerServer.Close()

	dialCtx, cancelDial := context.WithTimeout(context.Background(), 3*time.Second)
	clientConn, _, err := coderws.Dial(dialCtx, "ws"+strings.TrimPrefix(handlerServer.URL, "http")+"/openai/v1/responses", nil)
	cancelDial()
	require.NoError(t, err)
	defer func() { _ = clientConn.CloseNow() }()

	writeCtx, cancelWrite := context.WithTimeout(context.Background(), 3*time.Second)
	err = clientConn.Write(writeCtx, coderws.MessageText, []byte(`{"type":"response.create","model":"gpt-5.1","stream":false}`))
	cancelWrite()
	require.NoError(t, err)

	readCtx, cancelRead := context.WithTimeout(context.Background(), 5*time.Second)
	_, event, err := clientConn.Read(readCtx)
	cancelRead()
	require.NoError(t, err)
	require.Equal(t, "resp_replacement", gjson.GetBytes(event, "response.id").String())
	require.Zero(t, initialUpstreamCalls.Load())
	require.Equal(t, int32(1), replacementUpstreamCalls.Load())
	require.Equal(t, replacementAccount.ID, usageRecorder.lastAccountID.Load())
	require.Equal(t, replacementAccount.ID, usageRecorder.contextAccountID.Load())
	require.Equal(t, replacementAccount.Platform, usageRecorder.contextAccountPlatform.Load())
	require.Eventually(t, func() bool {
		return userReleases.Load() == 1 &&
			initialTargetReleases.Load() == 1 &&
			replacementTargetReleases.Load() == 1
	}, time.Second, 10*time.Millisecond)
}

func TestOpenAIResponsesWebSocketRefreshesExtraConcurrencySettingForEachTurn(t *testing.T) {
	gin.SetMode(gin.TestMode)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := coderws.Accept(w, r, &coderws.AcceptOptions{CompressionMode: coderws.CompressionContextTakeover})
		require.NoError(t, err)
		defer func() { _ = conn.CloseNow() }()

		for turn := 1; turn <= 2; turn++ {
			readCtx, cancelRead := context.WithTimeout(r.Context(), 3*time.Second)
			_, _, err = conn.Read(readCtx)
			cancelRead()
			require.NoError(t, err)

			writeCtx, cancelWrite := context.WithTimeout(r.Context(), 3*time.Second)
			response := fmt.Sprintf(`{"type":"response.completed","response":{"id":"resp_dynamic_ws_%d","model":"gpt-5.1","usage":{"input_tokens":1,"output_tokens":1}}}`, turn)
			err = conn.Write(writeCtx, coderws.MessageText, []byte(response))
			cancelWrite()
			require.NoError(t, err)
		}
		_ = conn.Close(coderws.StatusNormalClosure, "done")
	}))
	defer upstream.Close()

	groupID := int64(5103)
	accountID := int64(6103)
	account := service.Account{
		ID:          accountID,
		Name:        "openai-dynamic-ws",
		Platform:    service.PlatformOpenAI,
		Type:        service.AccountTypeAPIKey,
		Status:      service.StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{"api_key": "sk-ws-dynamic", "base_url": upstream.URL},
		Extra: map[string]any{
			"openai_apikey_responses_websockets_v2_enabled": true,
			"openai_apikey_responses_websockets_v2_mode":    service.OpenAIWSIngressModePassthrough,
		},
	}
	cfg := &config.Config{RunMode: config.RunModeSimple}
	cfg.Default.RateMultiplier = 1
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.APIKeyEnabled = true
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
	cfg.Gateway.OpenAIWS.ModeRouterV2Enabled = true
	cfg.Gateway.OpenAIWS.DialTimeoutSeconds = 3
	cfg.Gateway.OpenAIWS.ReadTimeoutSeconds = 3
	cfg.Gateway.OpenAIWS.WriteTimeoutSeconds = 3

	accountRepo := &openAIWSUsageHandlerAccountRepoStub{account: account}
	billingCacheService := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
	defer billingCacheService.Stop()
	gatewayService := service.NewOpenAIGatewayService(
		accountRepo, nil, nil, nil, nil, nil, nil, cfg, nil, nil,
		service.NewBillingService(cfg, nil), nil, billingCacheService, nil,
		&service.DeferredService{}, nil, nil, nil, nil, nil, nil, nil,
	)
	enabled := &atomic.Bool{}
	enabled.Store(true)
	settingService := service.NewSettingService(extraConcurrencySettingRepository{enabledFlag: enabled}, cfg)
	store := &websocketExtraAdmissionStore{}
	gatewayAdmission := service.NewGatewayAdmission(store, nil, fixedAdmissionCapacity{accountID: accountID})
	gatewayAdmission.SetExtraConcurrencyRuntimeSettingsSource(settingService)
	legacyUserClaims := atomic.Int32{}
	legacyAccountClaims := atomic.Int32{}
	legacyCache := &concurrencyCacheMock{
		acquireUserSlotFn: func(context.Context, int64, int, string) (bool, error) {
			legacyUserClaims.Add(1)
			return true, nil
		},
		acquireAccountSlotFn: func(context.Context, int64, int, string) (bool, error) {
			legacyAccountClaims.Add(1)
			return true, nil
		},
	}
	h := &OpenAIGatewayHandler{
		gatewayService:      gatewayService,
		billingCacheService: billingCacheService,
		apiKeyService:       &service.APIKeyService{},
		concurrencyHelper:   NewConcurrencyHelper(service.NewConcurrencyService(legacyCache), SSEPingFormatNone, time.Second),
		gatewayAdmission:    gatewayAdmission,
		maxAccountSwitches:  1,
		cfg:                 cfg,
		settingService:      settingService,
	}
	apiKey := &service.APIKey{
		ID:      7103,
		UserID:  8103,
		GroupID: &groupID,
		User: &service.User{
			ID:               8103,
			Status:           service.StatusActive,
			Concurrency:      1,
			ExtraConcurrency: 1,
			Balance:          100,
		},
		Group: &service.Group{ID: groupID, Platform: service.PlatformOpenAI, Status: service.StatusActive},
	}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyAPIKey), apiKey)
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{
			UserID:           apiKey.UserID,
			Concurrency:      1,
			ExtraConcurrency: 1,
		})
		c.Next()
	})
	router.GET("/openai/v1/responses", h.ResponsesWebSocket)
	handlerServer := httptest.NewServer(router)
	defer handlerServer.Close()

	dialCtx, cancelDial := context.WithTimeout(context.Background(), 3*time.Second)
	clientConn, _, err := coderws.Dial(dialCtx, "ws"+strings.TrimPrefix(handlerServer.URL, "http")+"/openai/v1/responses", nil)
	cancelDial()
	require.NoError(t, err)
	defer func() { _ = clientConn.CloseNow() }()

	writeCtx, cancelWrite := context.WithTimeout(context.Background(), 3*time.Second)
	err = clientConn.Write(writeCtx, coderws.MessageText, []byte(`{"type":"response.create","model":"gpt-5.1","stream":false}`))
	cancelWrite()
	require.NoError(t, err)
	readCtx, cancelRead := context.WithTimeout(context.Background(), 5*time.Second)
	_, firstEvent, err := clientConn.Read(readCtx)
	cancelRead()
	require.NoError(t, err)
	require.Equal(t, "resp_dynamic_ws_1", gjson.GetBytes(firstEvent, "response.id").String())
	require.Equal(t, int32(1), store.userClaims.Load())
	require.Equal(t, int32(1), store.targetClaims.Load())
	require.Zero(t, legacyUserClaims.Load())
	require.Zero(t, legacyAccountClaims.Load())

	enabled.Store(false)
	settingService.InvalidateExtraConcurrencyRuntimeSettings()
	writeCtx, cancelWrite = context.WithTimeout(context.Background(), 3*time.Second)
	err = clientConn.Write(writeCtx, coderws.MessageText, []byte(`{"type":"response.create","model":"gpt-5.1","stream":false,"previous_response_id":"resp_dynamic_ws_1"}`))
	cancelWrite()
	require.NoError(t, err)
	readCtx, cancelRead = context.WithTimeout(context.Background(), 5*time.Second)
	_, secondEvent, err := clientConn.Read(readCtx)
	cancelRead()
	require.NoError(t, err)
	require.Equal(t, "resp_dynamic_ws_2", gjson.GetBytes(secondEvent, "response.id").String())

	require.Equal(t, int32(1), store.userClaims.Load())
	require.Equal(t, int32(1), store.targetClaims.Load())
	require.Equal(t, int32(1), legacyUserClaims.Load())
	require.Equal(t, int32(1), legacyAccountClaims.Load())
	require.Eventually(t, func() bool {
		return atomic.LoadInt32(&legacyCache.releaseUserCalled) == 1 && atomic.LoadInt32(&legacyCache.releaseAccountCalled) == 1
	}, time.Second, 10*time.Millisecond)
}

func TestOpenAIResponsesWebSocketCancelsBlockedFollowUpAdmissionWhenUpstreamCloses(t *testing.T) {
	gin.SetMode(gin.TestMode)

	secondTargetStarted := make(chan struct{})
	upstreamClosed := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := coderws.Accept(w, r, &coderws.AcceptOptions{CompressionMode: coderws.CompressionContextTakeover})
		require.NoError(t, err)
		defer func() { _ = conn.CloseNow() }()

		readCtx, cancelRead := context.WithTimeout(r.Context(), 3*time.Second)
		_, _, err = conn.Read(readCtx)
		cancelRead()
		require.NoError(t, err)

		writeCtx, cancelWrite := context.WithTimeout(r.Context(), 3*time.Second)
		err = conn.Write(writeCtx, coderws.MessageText, []byte(`{"type":"response.completed","response":{"id":"resp_extra_ws_cancel_1","model":"gpt-5.1","usage":{"input_tokens":1,"output_tokens":1}}}`))
		cancelWrite()
		require.NoError(t, err)

		select {
		case <-secondTargetStarted:
		case <-r.Context().Done():
			return
		}
		_ = conn.CloseNow()
		close(upstreamClosed)
	}))
	defer upstream.Close()

	groupID := int64(5102)
	accountID := int64(6102)
	account := service.Account{
		ID:          accountID,
		Name:        "openai-extra-ws-cancel",
		Platform:    service.PlatformOpenAI,
		Type:        service.AccountTypeAPIKey,
		Status:      service.StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{"api_key": "sk-ws-extra-cancel", "base_url": upstream.URL},
		Extra: map[string]any{
			"openai_apikey_responses_websockets_v2_enabled": true,
			"openai_apikey_responses_websockets_v2_mode":    service.OpenAIWSIngressModePassthrough,
		},
	}
	cfg := &config.Config{RunMode: config.RunModeSimple}
	cfg.Default.RateMultiplier = 1
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.APIKeyEnabled = true
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
	cfg.Gateway.OpenAIWS.ModeRouterV2Enabled = true
	cfg.Gateway.OpenAIWS.DialTimeoutSeconds = 3
	cfg.Gateway.OpenAIWS.ReadTimeoutSeconds = 3
	cfg.Gateway.OpenAIWS.WriteTimeoutSeconds = 3

	accountRepo := &openAIWSUsageHandlerAccountRepoStub{account: account}
	usageRecorder := &usageLogWriteRecorder{}
	billingCacheService := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
	defer billingCacheService.Stop()
	gatewayService := service.NewOpenAIGatewayService(
		accountRepo, usageRecorder, nil, nil, nil, nil, nil, cfg, nil, nil,
		service.NewBillingService(cfg, nil), nil, billingCacheService, nil,
		&service.DeferredService{}, nil, nil, nil, nil, nil, nil, nil,
	)
	store := &blockingFollowUpAdmissionStore{secondTargetStarted: secondTargetStarted}
	h := &OpenAIGatewayHandler{
		gatewayService:      gatewayService,
		billingCacheService: billingCacheService,
		apiKeyService:       &service.APIKeyService{},
		concurrencyHelper:   NewConcurrencyHelper(service.NewConcurrencyService(&concurrencyCacheMock{}), SSEPingFormatNone, time.Second),
		gatewayAdmission:    service.NewGatewayAdmission(store, nil, fixedAdmissionCapacity{accountID: accountID}),
		maxAccountSwitches:  1,
		cfg:                 cfg,
		settingService: service.NewSettingService(
			extraConcurrencySettingRepository{waitTimeoutSeconds: "5"},
			cfg,
		),
	}
	apiKey := &service.APIKey{
		ID:      7102,
		UserID:  8102,
		GroupID: &groupID,
		User: &service.User{
			ID:               8102,
			Status:           service.StatusActive,
			Concurrency:      1,
			ExtraConcurrency: 1,
			Balance:          100,
		},
		Group: &service.Group{ID: groupID, Platform: service.PlatformOpenAI, Status: service.StatusActive},
	}
	handlerCtx, cancelHandler := context.WithCancel(context.Background())
	handlerDone := make(chan struct{})
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(handlerCtx)
		c.Set(string(middleware.ContextKeyAPIKey), apiKey)
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{
			UserID:           apiKey.UserID,
			Concurrency:      1,
			ExtraConcurrency: 1,
		})
		c.Next()
	})
	router.GET("/openai/v1/responses", func(c *gin.Context) {
		h.ResponsesWebSocket(c)
		close(handlerDone)
	})
	handlerServer := httptest.NewServer(router)
	defer handlerServer.Close()
	defer cancelHandler()

	dialCtx, cancelDial := context.WithTimeout(context.Background(), 3*time.Second)
	clientConn, _, err := coderws.Dial(dialCtx, "ws"+strings.TrimPrefix(handlerServer.URL, "http")+"/openai/v1/responses", nil)
	cancelDial()
	require.NoError(t, err)
	defer func() { _ = clientConn.CloseNow() }()

	writeCtx, cancelWrite := context.WithTimeout(context.Background(), 3*time.Second)
	err = clientConn.Write(writeCtx, coderws.MessageText, []byte(`{"type":"response.create","model":"gpt-5.1","stream":false}`))
	cancelWrite()
	require.NoError(t, err)

	readCtx, cancelRead := context.WithTimeout(context.Background(), 3*time.Second)
	_, event, readErr := clientConn.Read(readCtx)
	cancelRead()
	require.NoError(t, readErr)
	require.Equal(t, "resp_extra_ws_cancel_1", gjson.GetBytes(event, "response.id").String())
	require.Eventually(t, func() bool {
		return usageRecorder.writes.Load() == 1
	}, time.Second, 10*time.Millisecond, "the completed first turn should produce exactly one usage record")

	writeCtx, cancelWrite = context.WithTimeout(context.Background(), 3*time.Second)
	err = clientConn.Write(writeCtx, coderws.MessageText, []byte(`{"type":"response.create","model":"gpt-5.1","stream":false,"previous_response_id":"resp_extra_ws_cancel_1"}`))
	cancelWrite()
	require.NoError(t, err)

	select {
	case <-upstreamClosed:
	case <-time.After(3 * time.Second):
		t.Fatal("upstream did not close while follow-up admission was blocked")
	}
	require.Eventually(t, func() bool {
		return store.userReleases.Load() == 2 && store.targetReleases.Load() == 2
	}, 500*time.Millisecond, 10*time.Millisecond, "follow-up admission state was not released promptly")
	_ = clientConn.CloseNow()
	select {
	case <-handlerDone:
	case <-time.After(2 * time.Second):
		cancelHandler()
		t.Fatal("websocket handler did not return promptly after upstream closed")
	}
	require.GreaterOrEqual(t, store.userClaims.Load(), int32(2))
	require.GreaterOrEqual(t, store.targetClaims.Load(), int32(2))
	require.Equal(t, int32(1), usageRecorder.writes.Load(),
		"disconnecting the blocked follow-up turn must not create another usage record")
}

func TestGatewayHandlerMessagesExtraTargetTimeoutReturnsDistinct429(t *testing.T) {
	gin.SetMode(gin.TestMode)
	groupID := int64(2101)
	accountID := int64(1101)
	group := &service.Group{
		ID:       groupID,
		Hydrated: true,
		Platform: service.PlatformAnthropic,
		Status:   service.StatusActive,
	}
	account := &service.Account{
		ID:          accountID,
		Name:        "anthropic-timeout",
		Platform:    service.PlatformAnthropic,
		Type:        service.AccountTypeOAuth,
		Concurrency: 1,
		Priority:    1,
		Status:      service.StatusActive,
		Schedulable: true,
		Credentials: map[string]any{
			"access_token":              "test-token",
			"intercept_warmup_requests": true,
		},
		AccountGroups: []service.AccountGroup{{AccountID: accountID, GroupID: groupID}},
	}

	usageRecorder := &usageLogWriteRecorder{}
	h, cleanup := newTestGatewayHandlerWithUsageLogRepo(t, group, []*service.Account{account}, usageRecorder)
	defer cleanup()
	h.settingService = service.NewSettingService(extraConcurrencySettingRepository{}, &config.Config{})
	h.gatewayAdmission = service.NewGatewayAdmission(
		expiringTargetAdmissionStore{},
		h.gatewayService,
		fixedAdmissionCapacity{accountID: accountID},
	)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	body := []byte(`{
		"model":"claude-sonnet-4-5",
		"max_tokens":256,
		"messages":[{"role":"user","content":[{"type":"text","text":"Warmup"}]}]
	}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), ctxkey.Group, group))
	c.Request = req
	apiKey := &service.APIKey{
		ID:      3101,
		UserID:  4101,
		GroupID: &groupID,
		Status:  service.StatusActive,
		User: &service.User{
			ID:               4101,
			Concurrency:      1,
			ExtraConcurrency: 1,
			Balance:          100,
		},
		Group: group,
	}
	c.Set(string(middleware.ContextKeyAPIKey), apiKey)
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{
		UserID:           apiKey.UserID,
		Concurrency:      1,
		ExtraConcurrency: 1,
	})

	h.Messages(c)

	require.Equal(t, http.StatusTooManyRequests, recorder.Code)
	require.Equal(t, "EXTRA_CONCURRENCY_UNAVAILABLE", gjson.GetBytes(recorder.Body.Bytes(), "error.type").String())
	require.Zero(t, usageRecorder.writes.Load())
}

func TestGatewayHandlerMessagesWarmupInterceptReleasesExtraAdmission(t *testing.T) {
	gin.SetMode(gin.TestMode)
	groupID := int64(2104)
	accountID := int64(1104)
	group := &service.Group{
		ID:       groupID,
		Hydrated: true,
		Platform: service.PlatformAnthropic,
		Status:   service.StatusActive,
	}
	account := &service.Account{
		ID:          accountID,
		Name:        "anthropic-warmup",
		Platform:    service.PlatformAnthropic,
		Type:        service.AccountTypeAPIKey,
		Concurrency: 1,
		Priority:    1,
		Status:      service.StatusActive,
		Schedulable: true,
		Credentials: map[string]any{
			"api_key":                   "upstream-key",
			"base_url":                  "https://api.anthropic.com",
			"intercept_warmup_requests": true,
		},
		Extra:         map[string]any{"anthropic_passthrough": true},
		AccountGroups: []service.AccountGroup{{AccountID: accountID, GroupID: groupID}},
	}
	upstream := &successfulAnthropicUpstream{}
	h, concurrencyCache, cleanup := newSuccessfulExtraConcurrencyHandler(t, group, account, upstream)
	defer cleanup()
	h.settingService = service.NewSettingService(extraConcurrencySettingRepository{}, &config.Config{})
	h.gatewayAdmission = service.NewGatewayAdmission(
		&reusableAdmissionStore{},
		h.gatewayService,
		fixedAdmissionCapacity{accountID: accountID},
	)
	apiKey := &service.APIKey{
		ID:      3104,
		UserID:  4104,
		GroupID: &groupID,
		Status:  service.StatusActive,
		User: &service.User{
			ID:               4104,
			Concurrency:      1,
			ExtraConcurrency: 1,
			Balance:          100,
		},
		Group: group,
	}

	for range 2 {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		body := []byte(`{
			"model":"claude-sonnet-4-5",
			"max_tokens":256,
			"messages":[{"role":"user","content":[{"type":"text","text":"Warmup"}]}]
		}`)
		req := httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req = req.WithContext(context.WithValue(req.Context(), ctxkey.Group, group))
		c.Request = req
		c.Set(string(middleware.ContextKeyAPIKey), apiKey)
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{
			UserID:           apiKey.UserID,
			Concurrency:      1,
			ExtraConcurrency: 1,
		})

		h.Messages(c)

		require.Equal(t, http.StatusOK, recorder.Code)
		require.Equal(t, "msg_mock_warmup", gjson.GetBytes(recorder.Body.Bytes(), "id").String())
	}
	require.Zero(t, upstream.calls.Load())
	require.Equal(t, 2, concurrencyCache.apiKeyTrackCalls)
	require.Equal(t, 2, concurrencyCache.apiKeyReleaseCalls)
}

func TestGatewayHandlerMessagesWaitedExtraRequestRechecksBillingBeforeUpstream(t *testing.T) {
	gin.SetMode(gin.TestMode)
	groupID := int64(2102)
	accountID := int64(1102)
	group := &service.Group{
		ID:       groupID,
		Hydrated: true,
		Platform: service.PlatformAnthropic,
		Status:   service.StatusActive,
	}
	account := &service.Account{
		ID:            accountID,
		Name:          "anthropic-recheck",
		Platform:      service.PlatformAnthropic,
		Type:          service.AccountTypeOAuth,
		Concurrency:   1,
		Priority:      1,
		Status:        service.StatusActive,
		Schedulable:   true,
		Credentials:   map[string]any{"access_token": "test-token"},
		AccountGroups: []service.AccountGroup{{AccountID: accountID, GroupID: groupID}},
	}

	usageRecorder := &usageLogWriteRecorder{}
	h, cleanup := newTestGatewayHandlerWithUsageLogRepo(t, group, []*service.Account{account}, usageRecorder)
	defer cleanup()
	h.settingService = service.NewSettingService(extraConcurrencySettingRepository{}, &config.Config{})
	store := &waitThenAcquireAdmissionStore{}
	h.gatewayAdmission = service.NewGatewayAdmission(
		store,
		h.gatewayService,
		fixedAdmissionCapacity{accountID: accountID},
	)
	balanceCache := &changingBalanceCache{}
	billingCacheService := service.NewBillingCacheService(balanceCache, nil, nil, nil, nil, nil, &config.Config{}, nil)
	defer billingCacheService.Stop()
	h.billingCacheService = billingCacheService

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	body := []byte(`{"model":"claude-sonnet-4-5","max_tokens":256,"messages":[{"role":"user","content":"hello"}]}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), ctxkey.Group, group))
	c.Request = req
	apiKey := &service.APIKey{
		ID:      3102,
		UserID:  4102,
		GroupID: &groupID,
		Status:  service.StatusActive,
		User: &service.User{
			ID:               4102,
			Concurrency:      1,
			ExtraConcurrency: 1,
			Balance:          100,
		},
		Group: group,
	}
	c.Set(string(middleware.ContextKeyAPIKey), apiKey)
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{
		UserID:           apiKey.UserID,
		Concurrency:      1,
		ExtraConcurrency: 1,
	})

	h.Messages(c)

	require.Equal(t, http.StatusForbidden, recorder.Code)
	require.Equal(t, "billing_error", gjson.GetBytes(recorder.Body.Bytes(), "error.type").String())
	require.Equal(t, int32(2), balanceCache.reads.Load())
	require.Zero(t, usageRecorder.writes.Load())
}

func TestGatewayHandlerMessagesWaitedExtraRequestCountsRPMOnce(t *testing.T) {
	gin.SetMode(gin.TestMode)
	groupID := int64(2105)
	accountID := int64(1105)
	group := &service.Group{
		ID:       groupID,
		Hydrated: true,
		Platform: service.PlatformAnthropic,
		Status:   service.StatusActive,
	}
	account := &service.Account{
		ID:            accountID,
		Name:          "anthropic-rpm-recheck",
		Platform:      service.PlatformAnthropic,
		Type:          service.AccountTypeAPIKey,
		Concurrency:   1,
		Priority:      1,
		Status:        service.StatusActive,
		Schedulable:   true,
		Credentials:   map[string]any{"api_key": "upstream-key", "base_url": "https://api.anthropic.com"},
		Extra:         map[string]any{"anthropic_passthrough": true},
		AccountGroups: []service.AccountGroup{{AccountID: accountID, GroupID: groupID}},
	}
	upstream := &successfulAnthropicUpstream{}
	h, _, cleanup := newSuccessfulExtraConcurrencyHandler(t, group, account, upstream)
	defer cleanup()
	h.settingService = service.NewSettingService(extraConcurrencySettingRepository{}, &config.Config{})
	h.gatewayAdmission = service.NewGatewayAdmission(
		&waitThenAcquireAdmissionStore{},
		h.gatewayService,
		fixedAdmissionCapacity{accountID: accountID},
	)
	rpmCache := &countingUserRPMCache{}
	billingCacheService := service.NewBillingCacheService(
		fixedBalanceCache{},
		nil,
		nil,
		nil,
		rpmCache,
		nil,
		&config.Config{RunMode: config.RunModeStandard},
		nil,
	)
	defer billingCacheService.Stop()
	h.billingCacheService = billingCacheService

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	body := []byte(`{"model":"claude-sonnet-4-5","max_tokens":256,"messages":[{"role":"user","content":"hello"}]}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), ctxkey.Group, group))
	c.Request = req
	apiKey := &service.APIKey{
		ID:      3105,
		UserID:  4105,
		GroupID: &groupID,
		Status:  service.StatusActive,
		User: &service.User{
			ID:               4105,
			Concurrency:      1,
			ExtraConcurrency: 1,
			Balance:          100,
			RPMLimit:         1,
		},
		Group: group,
	}
	c.Set(string(middleware.ContextKeyAPIKey), apiKey)
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{
		UserID:           apiKey.UserID,
		Concurrency:      1,
		ExtraConcurrency: 1,
	})

	h.Messages(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "msg_test", gjson.GetBytes(recorder.Body.Bytes(), "id").String())
	require.Equal(t, int32(1), upstream.calls.Load())
	require.Equal(t, int32(1), rpmCache.incrementCalls.Load())
}

func TestGatewayHandlerMessagesWaitedExtraRequestRechecksAPIKeyQuota(t *testing.T) {
	gin.SetMode(gin.TestMode)
	groupID := int64(2106)
	accountID := int64(1106)
	group := &service.Group{
		ID:       groupID,
		Hydrated: true,
		Platform: service.PlatformAnthropic,
		Status:   service.StatusActive,
	}
	account := &service.Account{
		ID:            accountID,
		Name:          "anthropic-api-key-recheck",
		Platform:      service.PlatformAnthropic,
		Type:          service.AccountTypeAPIKey,
		Concurrency:   1,
		Priority:      1,
		Status:        service.StatusActive,
		Schedulable:   true,
		Credentials:   map[string]any{"api_key": "upstream-key", "base_url": "https://api.anthropic.com"},
		Extra:         map[string]any{"anthropic_passthrough": true},
		AccountGroups: []service.AccountGroup{{AccountID: accountID, GroupID: groupID}},
	}
	upstream := &successfulAnthropicUpstream{}
	h, _, cleanup := newSuccessfulExtraConcurrencyHandler(t, group, account, upstream)
	defer cleanup()
	h.settingService = service.NewSettingService(extraConcurrencySettingRepository{}, &config.Config{})
	h.gatewayAdmission = service.NewGatewayAdmission(
		&waitThenAcquireAdmissionStore{},
		h.gatewayService,
		fixedAdmissionCapacity{accountID: accountID},
	)
	repo := &exhaustedAPIKeyRepo{apiKey: &service.APIKey{
		ID:        3106,
		Status:    service.StatusAPIKeyActive,
		Quota:     1,
		QuotaUsed: 1,
	}}
	billingCacheService := service.NewBillingCacheService(
		fixedBalanceCache{},
		nil,
		nil,
		repo,
		nil,
		nil,
		&config.Config{RunMode: config.RunModeStandard},
		nil,
	)
	defer billingCacheService.Stop()
	h.billingCacheService = billingCacheService

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	body := []byte(`{"model":"claude-sonnet-4-5","max_tokens":256,"messages":[{"role":"user","content":"hello"}]}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), ctxkey.Group, group))
	c.Request = req
	apiKey := &service.APIKey{
		ID:      3106,
		UserID:  4106,
		GroupID: &groupID,
		Status:  service.StatusAPIKeyActive,
		Quota:   1,
		User: &service.User{
			ID:               4106,
			Concurrency:      1,
			ExtraConcurrency: 1,
			Balance:          100,
		},
		Group: group,
	}
	c.Set(string(middleware.ContextKeyAPIKey), apiKey)
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{
		UserID:           apiKey.UserID,
		Concurrency:      1,
		ExtraConcurrency: 1,
	})

	h.Messages(c)

	require.Equal(t, http.StatusForbidden, recorder.Code)
	require.Equal(t, "billing_error", gjson.GetBytes(recorder.Body.Bytes(), "error.type").String())
	require.Zero(t, upstream.calls.Load())
	require.Equal(t, int32(1), repo.reads.Load())
}

func TestGatewayHandlerMessagesSuccessfulExtraRequestsReleaseAdmission(t *testing.T) {
	gin.SetMode(gin.TestMode)
	groupID := int64(2103)
	accountID := int64(1103)
	group := &service.Group{
		ID:       groupID,
		Hydrated: true,
		Platform: service.PlatformAnthropic,
		Status:   service.StatusActive,
	}
	account := &service.Account{
		ID:          accountID,
		Name:        "anthropic-success",
		Platform:    service.PlatformAnthropic,
		Type:        service.AccountTypeAPIKey,
		Concurrency: 1,
		Priority:    1,
		Status:      service.StatusActive,
		Schedulable: true,
		Credentials: map[string]any{
			"api_key":  "upstream-key",
			"base_url": "https://api.anthropic.com",
		},
		Extra:         map[string]any{"anthropic_passthrough": true},
		AccountGroups: []service.AccountGroup{{AccountID: accountID, GroupID: groupID}},
	}
	upstream := &successfulAnthropicUpstream{}
	h, concurrencyCache, cleanup := newSuccessfulExtraConcurrencyHandler(t, group, account, upstream)
	defer cleanup()
	h.settingService = service.NewSettingService(extraConcurrencySettingRepository{}, &config.Config{})
	store := &reusableAdmissionStore{}
	h.gatewayAdmission = service.NewGatewayAdmission(
		store,
		h.gatewayService,
		fixedAdmissionCapacity{accountID: accountID},
	)
	apiKey := &service.APIKey{
		ID:      3103,
		UserID:  4103,
		GroupID: &groupID,
		Status:  service.StatusActive,
		User: &service.User{
			ID:               4103,
			Concurrency:      1,
			ExtraConcurrency: 1,
			Balance:          100,
		},
		Group: group,
	}

	for range 2 {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		body := []byte(`{"model":"claude-sonnet-4-5","max_tokens":256,"messages":[{"role":"user","content":"hello"}]}`)
		req := httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req = req.WithContext(context.WithValue(req.Context(), ctxkey.Group, group))
		c.Request = req
		c.Set(string(middleware.ContextKeyAPIKey), apiKey)
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{
			UserID:           apiKey.UserID,
			Concurrency:      1,
			ExtraConcurrency: 1,
		})

		h.Messages(c)

		require.Equal(t, http.StatusOK, recorder.Code)
		require.Equal(t, "msg_test", gjson.GetBytes(recorder.Body.Bytes(), "id").String())
	}
	require.Equal(t, int32(2), upstream.calls.Load())
	require.Equal(t, 2, concurrencyCache.apiKeyTrackCalls)
	require.Equal(t, 2, concurrencyCache.apiKeyReleaseCalls)
}

func TestGatewayHandlerMessagesDispatchRetargetUsesReplacementAccountForPostDispatchEffects(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const (
		groupID              int64 = 2_104
		initialAccountID     int64 = 1_104
		replacementAccountID int64 = 1_105
	)
	group := &service.Group{
		ID:       groupID,
		Hydrated: true,
		Platform: service.PlatformAnthropic,
		Status:   service.StatusActive,
	}
	accountFor := func(id int64, name string) *service.Account {
		return &service.Account{
			ID:          id,
			Name:        name,
			Platform:    service.PlatformAnthropic,
			Type:        service.AccountTypeOAuth,
			Concurrency: 1,
			Priority:    1,
			Status:      service.StatusActive,
			Schedulable: true,
			Credentials: map[string]any{"access_token": "test-token"},
			Extra: map[string]any{
				"base_rpm":            100,
				"user_msg_queue_mode": config.UMQModeSerialize,
			},
			AccountGroups: []service.AccountGroup{{AccountID: id, GroupID: groupID}},
		}
	}
	initialAccount := accountFor(initialAccountID, "anthropic-initial-extra")
	replacementAccount := accountFor(replacementAccountID, "anthropic-replacement-standard")
	replacementAccount.Priority = 2
	usageRecorder := &usageLogWriteRecorder{}
	upstream := &accountCapturingAnthropicUpstream{}
	rpmRecorder := &accountRPMRecorder{}
	umqCache := &recordingUserMsgQueueCache{}
	schedulerCache := &fakeSchedulerCache{accounts: []*service.Account{initialAccount, replacementAccount}}
	schedulerSnapshot := service.NewSchedulerSnapshotService(schedulerCache, nil, nil, nil, nil)
	cfg := &config.Config{RunMode: config.RunModeSimple}
	billingCacheService := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
	defer billingCacheService.Stop()
	gatewayService := service.NewGatewayService(
		nil,
		&fakeGroupRepo{group: group},
		usageRecorder,
		nil,
		nil,
		nil,
		nil,
		nil,
		cfg,
		schedulerSnapshot,
		nil,
		nil,
		&service.RateLimitService{},
		billingCacheService,
		nil,
		upstream,
		&service.DeferredService{},
		nil,
		nil,
		rpmRecorder,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
	)
	h := &GatewayHandler{
		gatewayService:      gatewayService,
		billingCacheService: billingCacheService,
		concurrencyHelper: NewConcurrencyHelper(
			service.NewConcurrencyService(&helperConcurrencyCacheStub{}),
			SSEPingFormatClaude,
			0,
		),
		maxAccountSwitches: 1,
		cfg:                cfg,
		userMsgQueueHelper: NewUserMsgQueueHelper(
			service.NewUserMessageQueueService(umqCache, nil, &cfg.Gateway.UserMessageQueue),
			SSEPingFormatClaude,
			0,
		),
		settingService: service.NewSettingService(
			extraConcurrencySettingRepository{},
			&config.Config{},
		),
	}
	h.gatewayAdmission = service.NewGatewayAdmission(
		dispatchRetargetAdmissionStore{
			initialAccountID:     initialAccountID,
			replacementAccountID: replacementAccountID,
			initialAccount:       initialAccount,
		},
		gatewayService,
		dispatchRetargetCapacity{accountIDs: []int64{initialAccountID, replacementAccountID}},
	)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	body := []byte(`{"model":"claude-sonnet-4-5","max_tokens":256,"messages":[{"role":"user","content":"hello"}]}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), ctxkey.Group, group))
	c.Request = req
	groupIDValue := groupID
	apiKey := &service.APIKey{
		ID:      3_104,
		UserID:  4_104,
		GroupID: &groupIDValue,
		Status:  service.StatusAPIKeyActive,
		User: &service.User{
			ID:               4_104,
			Concurrency:      1,
			ExtraConcurrency: 1,
			Balance:          100,
		},
		Group: group,
	}
	c.Set(string(middleware.ContextKeyAPIKey), apiKey)
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{
		UserID:           apiKey.UserID,
		Concurrency:      1,
		ExtraConcurrency: 1,
	})

	h.Messages(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, replacementAccountID, upstream.accountID.Load())
	require.Equal(t, replacementAccountID, rpmRecorder.accountID.Load())
	acquired, released := umqCache.snapshot()
	require.Equal(t, []int64{initialAccountID, replacementAccountID}, acquired)
	require.Equal(t, []int64{initialAccountID, replacementAccountID}, released)
}

func TestGatewayHandlerGeminiV1BetaDispatchRetargetRemovesInitialAccountThoughtSignature(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const (
		groupID              int64 = 2_105
		initialAccountID     int64 = 1_106
		replacementAccountID int64 = 1_107
	)
	group := &service.Group{
		ID:       groupID,
		Hydrated: true,
		Platform: service.PlatformGemini,
		Status:   service.StatusActive,
	}
	accountFor := func(id int64, name string) *service.Account {
		return &service.Account{
			ID:          id,
			Name:        name,
			Platform:    service.PlatformGemini,
			Type:        service.AccountTypeAPIKey,
			Concurrency: 1,
			Priority:    1,
			Status:      service.StatusActive,
			Schedulable: true,
			Credentials: map[string]any{
				"api_key":  "gemini-test-key",
				"base_url": "https://generativelanguage.googleapis.com",
			},
			AccountGroups: []service.AccountGroup{{AccountID: id, GroupID: groupID}},
		}
	}
	initialAccount := accountFor(initialAccountID, "gemini-initial-extra")
	replacementAccount := accountFor(replacementAccountID, "gemini-replacement-standard")
	replacementAccount.Priority = 2
	upstream := &geminiNativeBodyCaptureUpstream{}
	stickyCache := fixedSessionGatewayCache{accountID: initialAccountID}
	schedulerCache := &fakeSchedulerCache{accounts: []*service.Account{initialAccount, replacementAccount}}
	schedulerSnapshot := service.NewSchedulerSnapshotService(schedulerCache, nil, nil, nil, nil)
	cfg := &config.Config{RunMode: config.RunModeSimple}
	billingCacheService := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
	defer billingCacheService.Stop()
	gatewayService := service.NewGatewayService(
		nil,
		&fakeGroupRepo{group: group},
		nil,
		nil,
		nil,
		nil,
		nil,
		stickyCache,
		cfg,
		schedulerSnapshot,
		nil,
		nil,
		&service.RateLimitService{},
		billingCacheService,
		nil,
		upstream,
		&service.DeferredService{},
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
	)
	pool := service.NewUsageRecordWorkerPoolWithOptions(service.UsageRecordWorkerPoolOptions{
		WorkerCount:      1,
		QueueSize:        1,
		AutoScaleEnabled: false,
	})
	pool.Stop()
	h := &GatewayHandler{
		gatewayService: gatewayService,
		geminiCompatService: service.NewGeminiMessagesCompatService(
			nil,
			&fakeGroupRepo{group: group},
			stickyCache,
			schedulerSnapshot,
			nil,
			&service.RateLimitService{},
			upstream,
			nil,
			cfg,
		),
		billingCacheService:      billingCacheService,
		concurrencyHelper:        NewConcurrencyHelper(service.NewConcurrencyService(&helperConcurrencyCacheStub{}), SSEPingFormatNone, 0),
		usageRecordWorkerPool:    pool,
		maxAccountSwitchesGemini: 1,
		cfg:                      cfg,
		settingService: service.NewSettingService(
			extraConcurrencySettingRepository{},
			&config.Config{},
		),
	}
	h.gatewayAdmission = service.NewGatewayAdmission(
		dispatchRetargetAdmissionStore{
			initialAccountID:     initialAccountID,
			replacementAccountID: replacementAccountID,
			initialAccount:       initialAccount,
		},
		gatewayService,
		dispatchRetargetCapacity{accountIDs: []int64{initialAccountID, replacementAccountID}},
	)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	body := []byte(`{"contents":[{"role":"model","parts":[{"functionCall":{"name":"lookup","args":{}},"thoughtSignature":"initial-account-signature"}]},{"role":"user","parts":[{"text":"continue"}]}]}`)
	req := httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-2.5-flash:generateContent", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), ctxkey.Group, group))
	c.Request = req
	c.Params = gin.Params{{Key: "modelAction", Value: "gemini-2.5-flash:generateContent"}}
	groupIDValue := groupID
	apiKey := &service.APIKey{
		ID:      3_105,
		UserID:  4_105,
		GroupID: &groupIDValue,
		Status:  service.StatusAPIKeyActive,
		User: &service.User{
			ID:               4_105,
			Concurrency:      1,
			ExtraConcurrency: 1,
			Balance:          100,
		},
		Group: group,
	}
	c.Set(string(middleware.ContextKeyAPIKey), apiKey)
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{
		UserID:           apiKey.UserID,
		Concurrency:      1,
		ExtraConcurrency: 1,
	})

	h.GeminiV1BetaModels(c)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.Equal(t, replacementAccountID, upstream.accountID.Load())
	capturedBody := upstream.capturedBody()
	require.Contains(t, string(capturedBody), `"functionCall"`)
	require.NotContains(t, string(capturedBody), "initial-account-signature")
}
