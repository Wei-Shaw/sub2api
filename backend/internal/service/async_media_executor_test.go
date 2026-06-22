//go:build unit

package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/fal"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// fakes
// ---------------------------------------------------------------------------

// fakeUserRepo 仅实现余额扣减/退还，其余方法继承 nil 接口（不会被调用）。
type fakeUserRepo struct {
	UserRepository
	mu        sync.Mutex
	balance   float64
	deducts   []float64
	refunds   []float64
	deductErr error
}

func (r *fakeUserRepo) DeductBalance(_ context.Context, _ int64, amount float64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.deductErr != nil {
		return r.deductErr
	}
	r.balance -= amount
	r.deducts = append(r.deducts, amount)
	return nil
}

func (r *fakeUserRepo) UpdateBalance(_ context.Context, _ int64, amount float64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.balance += amount
	r.refunds = append(r.refunds, amount)
	return nil
}

func (r *fakeUserRepo) totalRefunded() float64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	var sum float64
	for _, v := range r.refunds {
		sum += v
	}
	return sum
}

func (r *fakeUserRepo) refundCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.refunds)
}

// fakeTaskRepo 内存实现 AsyncMediaTaskRepository，模拟终态去重（幂等）。
type fakeTaskRepo struct {
	mu       sync.Mutex
	seq      int64
	byID     map[int64]*AsyncMediaTask
	usageLog []*TerminalUsageLogInput
}

func newFakeTaskRepo() *fakeTaskRepo {
	return &fakeTaskRepo{byID: map[int64]*AsyncMediaTask{}}
}

func (r *fakeTaskRepo) Create(_ context.Context, task *AsyncMediaTask) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.seq++
	task.ID = r.seq
	task.CreatedAt = time.Now()
	task.UpdatedAt = time.Now()
	cp := *task
	r.byID[task.ID] = &cp
	return nil
}

func (r *fakeTaskRepo) GetByID(_ context.Context, id int64) (*AsyncMediaTask, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if t, ok := r.byID[id]; ok {
		cp := *t
		return &cp, nil
	}
	return nil, nil
}

func (r *fakeTaskRepo) GetByInternalRequestID(_ context.Context, id string) (*AsyncMediaTask, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, t := range r.byID {
		if t.InternalRequestID == id {
			cp := *t
			return &cp, nil
		}
	}
	return nil, nil
}

func (r *fakeTaskRepo) GetByUpstreamRequestID(_ context.Context, id string) (*AsyncMediaTask, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, t := range r.byID {
		if t.UpstreamRequestID != nil && *t.UpstreamRequestID == id {
			cp := *t
			return &cp, nil
		}
	}
	return nil, nil
}

func (r *fakeTaskRepo) UpdateUpstreamRef(_ context.Context, id int64, upstreamID, statusURL, responseURL string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.byID[id]
	if !ok {
		return nil
	}
	t.UpstreamRequestID = &upstreamID
	t.StatusURL = &statusURL
	t.ResponseURL = &responseURL
	t.Status = AsyncMediaStatusRunning
	return nil
}

func (r *fakeTaskRepo) MarkSucceeded(_ context.Context, id int64, imageURLs, cosURLs []string, finalCost float64) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.byID[id]
	if !ok || t.IsTerminal() {
		return false, nil // 幂等：已终态不重复结算
	}
	t.Status = AsyncMediaStatusSucceeded
	t.ImageURLs = imageURLs
	t.CosURLs = cosURLs
	t.FinalCost = finalCost
	return true, nil
}

func (r *fakeTaskRepo) MarkRefunded(_ context.Context, id int64, status, reason string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.byID[id]
	if !ok || t.IsTerminal() {
		return false, nil // 幂等：已终态不重复退费
	}
	t.Status = status
	t.ErrorReason = &reason
	return true, nil
}

func (r *fakeTaskRepo) ListUnfinished(_ context.Context, limit int) ([]*AsyncMediaTask, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []*AsyncMediaTask
	for _, t := range r.byID {
		if !t.IsTerminal() {
			cp := *t
			out = append(out, &cp)
		}
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (r *fakeTaskRepo) InsertTerminalUsageLog(_ context.Context, in *TerminalUsageLogInput) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.usageLog = append(r.usageLog, in)
	return true, nil
}

func (r *fakeTaskRepo) usageLogCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.usageLog)
}

func (r *fakeTaskRepo) lastUsageLog() *TerminalUsageLogInput {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.usageLog) == 0 {
		return nil
	}
	return r.usageLog[len(r.usageLog)-1]
}

// ---------------------------------------------------------------------------
// fal upstream test server
// ---------------------------------------------------------------------------

// falTestServer 模拟 fal queue 协议，状态/结果可配置。
type falTestServer struct {
	*httptest.Server
	statusCode     int      // status 接口返回的 HTTP code（非 200 表示上游错误）
	queueStatus    string   // status 接口返回的 fal status 字段
	images         []string // result 接口返回的图片
	statusHits     int32
}

func newFalTestServer(t *testing.T) *falTestServer {
	t.Helper()
	fs := &falTestServer{statusCode: http.StatusOK, queueStatus: fal.StatusCompleted, images: []string{"https://fal.media/out-1.png"}}
	fs.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		path := req.URL.Path
		switch {
		case req.Method == http.MethodPost && !strings.Contains(path, "/requests/"):
			// submit
			reqID := "req-test-1"
			base := fs.URL + path + "/requests/" + reqID
			writeJSON(w, http.StatusOK, fal.SubmitResponse{
				RequestID:   reqID,
				Status:      fal.StatusInQueue,
				StatusURL:   base + "/status",
				ResponseURL: base,
				CancelURL:   base + "/cancel",
			})
		case req.Method == http.MethodGet && strings.HasSuffix(path, "/status"):
			atomic.AddInt32(&fs.statusHits, 1)
			if fs.statusCode != http.StatusOK {
				w.WriteHeader(fs.statusCode)
				_, _ = w.Write([]byte(`{"detail":"bad request"}`))
				return
			}
			writeJSON(w, http.StatusOK, fal.StatusResponse{Status: fs.queueStatus, RequestID: "req-test-1"})
		case req.Method == http.MethodPut && strings.HasSuffix(path, "/cancel"):
			w.WriteHeader(http.StatusOK)
		case req.Method == http.MethodGet:
			// result
			resp := fal.Response{}
			for _, u := range fs.images {
				resp.Images = append(resp.Images, fal.Image{URL: u})
			}
			writeJSON(w, http.StatusOK, resp)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	return fs
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// newImageBillingResolver 构造一个对指定模型返回 image 模式 + 1K 单价的解析器。
func newImageBillingResolver(t *testing.T, groupID int64, model string, price1K float64) *ModelPricingResolver {
	t.Helper()
	cs := newTestChannelServiceWithCache(t, &channelCache{
		pricingByGroupModel: map[channelModelKey]*ChannelModelPricing{
			{groupID: groupID, model: model}: {
				BillingMode: BillingModeImage,
				Intervals: []PricingInterval{
					{TierLabel: ImageBillingSize1K, PerRequestPrice: testPtrFloat64(price1K)},
				},
			},
		},
		channelByGroupID:        map[int64]*Channel{groupID: {ID: groupID, Status: StatusActive}},
		groupPlatform:           map[int64]string{groupID: ""},
		wildcardByGroupPlatform: map[channelGroupPlatformKey][]*wildcardPricingEntry{},
		mappingByGroupModel:     map[channelModelKey]string{},
		wildcardMappingByGP:     map[channelGroupPlatformKey][]*wildcardMappingEntry{},
		byID:                    map[int64]*Channel{},
	})
	return NewModelPricingResolver(cs, newTestBillingService())
}

func newFalAccount(serverURL string) *Account {
	return &Account{
		ID:       7,
		Platform: PlatformFal,
		Type:     "apikey",
		Status:   StatusActive,
		Credentials: map[string]any{
			"api_key":  "test-fal-key",
			"base_url": serverURL,
		},
	}
}

func newSubmitInput(acc *Account, groupID int64, n int) *AsyncMediaSubmitInput {
	gid := groupID
	return &AsyncMediaSubmitInput{
		Account:           acc,
		APIKeyID:          11,
		UserID:            22,
		AccountID:         acc.ID,
		GroupID:           &gid,
		Facade:            AsyncMediaFacadeOpenAI,
		InternalRequestID: "intreq-1",
		RequestedModel:    "gpt-image-2",
		Input:             fal.ImageGenInput{Prompt: "a cat", Size: "1024x1024", N: n},
		BillingType:       BillingTypeBalance,
		RateMultiplier:    1.0,
	}
}

// ---------------------------------------------------------------------------
// tests
// ---------------------------------------------------------------------------

func TestAsyncMedia_SubmitAndSucceed_RefundsDelta(t *testing.T) {
	fs := newFalTestServer(t)
	defer fs.Close()

	groupID := int64(1)
	resolver := newImageBillingResolver(t, groupID, domain.FalSlugTextToImage, 0.05)
	userRepo := &fakeUserRepo{balance: 100}
	taskRepo := newFakeTaskRepo()
	billing := newTestBillingService()

	svc := NewAsyncMediaService(taskRepo, userRepo, billing, resolver, nil)
	svc.SetPollInterval(time.Millisecond)

	acc := newFalAccount(fs.URL)
	in := newSubmitInput(acc, groupID, 2) // 预扣 2 张

	task, err := svc.SubmitAsync(context.Background(), in)
	require.NoError(t, err)
	require.Equal(t, AsyncMediaStatusRunning, task.Status)
	// 预扣 2 × 0.05 = 0.10
	require.InDelta(t, 0.10, task.HeldCost, 1e-9)
	require.InDelta(t, 99.90, userRepo.balance, 1e-9)

	final, err := svc.WaitForTerminal(context.Background(), task, in)
	require.NoError(t, err)
	require.Equal(t, AsyncMediaStatusSucceeded, final.Status)
	// 实际出图 1 张 → finalCost = 0.05，退差 0.05
	require.InDelta(t, 0.05, final.FinalCost, 1e-9)
	require.InDelta(t, 99.95, userRepo.balance, 1e-9)
	require.Equal(t, []string{"https://fal.media/out-1.png"}, final.ImageURLs)

	// 终态写一条 charged usage_log
	require.Equal(t, 1, taskRepo.usageLogCount())
	require.Equal(t, BillingStatusCharged, taskRepo.lastUsageLog().BillingStatus)
}

func TestAsyncMedia_UpstreamFailure_RefundsFull(t *testing.T) {
	fs := newFalTestServer(t)
	fs.statusCode = http.StatusBadRequest // status 返回 4xx → 明确失败
	defer fs.Close()

	groupID := int64(1)
	resolver := newImageBillingResolver(t, groupID, domain.FalSlugTextToImage, 0.05)
	userRepo := &fakeUserRepo{balance: 100}
	taskRepo := newFakeTaskRepo()
	svc := NewAsyncMediaService(taskRepo, userRepo, newTestBillingService(), resolver, nil)
	svc.SetPollInterval(time.Millisecond)

	acc := newFalAccount(fs.URL)
	in := newSubmitInput(acc, groupID, 2)

	task, err := svc.SubmitAsync(context.Background(), in)
	require.NoError(t, err)
	require.InDelta(t, 99.90, userRepo.balance, 1e-9)

	final, err := svc.WaitForTerminal(context.Background(), task, in)
	require.NoError(t, err)
	require.Equal(t, AsyncMediaStatusRefunded, final.Status)
	// 全额退还预扣 0.10 → 余额恢复 100
	require.InDelta(t, 100.0, userRepo.balance, 1e-9)
	// 终态写一条 refunded usage_log
	require.Equal(t, BillingStatusRefunded, taskRepo.lastUsageLog().BillingStatus)
}

func TestAsyncMedia_PseudoSyncTimeout_NoRefundNoTerminal(t *testing.T) {
	fs := newFalTestServer(t)
	fs.queueStatus = fal.StatusInProgress // 永不终态
	defer fs.Close()

	groupID := int64(1)
	resolver := newImageBillingResolver(t, groupID, domain.FalSlugTextToImage, 0.05)
	userRepo := &fakeUserRepo{balance: 100}
	taskRepo := newFakeTaskRepo()
	svc := NewAsyncMediaService(taskRepo, userRepo, newTestBillingService(), resolver, nil)
	svc.SetPollInterval(5 * time.Millisecond)

	acc := newFalAccount(fs.URL)
	in := newSubmitInput(acc, groupID, 1)

	task, err := svc.SubmitAsync(context.Background(), in)
	require.NoError(t, err)
	balanceAfterCharge := userRepo.balance

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	_, err = svc.WaitForTerminal(ctx, task, in)
	require.ErrorIs(t, err, ErrAsyncMediaPending)

	// 超时：不退费、不终结
	require.InDelta(t, balanceAfterCharge, userRepo.balance, 1e-9)
	require.Equal(t, 0, userRepo.refundCount())
	stored, _ := taskRepo.GetByID(context.Background(), task.ID)
	require.False(t, stored.IsTerminal())
	require.Equal(t, 0, taskRepo.usageLogCount())
}

func TestAsyncMedia_Idempotent_NoDoubleRefund(t *testing.T) {
	fs := newFalTestServer(t)
	defer fs.Close()

	groupID := int64(1)
	resolver := newImageBillingResolver(t, groupID, domain.FalSlugTextToImage, 0.05)
	userRepo := &fakeUserRepo{balance: 100}
	taskRepo := newFakeTaskRepo()
	svc := NewAsyncMediaService(taskRepo, userRepo, newTestBillingService(), resolver, nil)
	svc.SetPollInterval(time.Millisecond)

	acc := newFalAccount(fs.URL)
	in := newSubmitInput(acc, groupID, 2)

	task, err := svc.SubmitAsync(context.Background(), in)
	require.NoError(t, err)

	final, err := svc.WaitForTerminal(context.Background(), task, in)
	require.NoError(t, err)
	require.Equal(t, AsyncMediaStatusSucceeded, final.Status)

	refundsAfterFirst := userRepo.totalRefunded()
	usageAfterFirst := taskRepo.usageLogCount()

	// 模拟 reconciler 再次推进同一任务（已终态）：应直接返回，不重复退费/写日志。
	stored, _ := taskRepo.GetByID(context.Background(), task.ID)
	require.NoError(t, svc.ReconcileTask(context.Background(), stored, acc))

	// 直接对已终态任务再调一次 markSucceeded/markFailedAndRefund 也应被幂等拦截。
	svc.markFailedAndRefund(context.Background(), stored, BillingTypeBalance, "double")

	require.InDelta(t, refundsAfterFirst, userRepo.totalRefunded(), 1e-9)
	require.Equal(t, usageAfterFirst, taskRepo.usageLogCount())
}

func TestAsyncMedia_DeadlineExceeded_Reconciler_Refunds(t *testing.T) {
	fs := newFalTestServer(t)
	fs.queueStatus = fal.StatusInProgress
	defer fs.Close()

	groupID := int64(1)
	resolver := newImageBillingResolver(t, groupID, domain.FalSlugTextToImage, 0.05)
	userRepo := &fakeUserRepo{balance: 100}
	taskRepo := newFakeTaskRepo()
	svc := NewAsyncMediaService(taskRepo, userRepo, newTestBillingService(), resolver, nil)
	svc.SetPollInterval(time.Millisecond)

	acc := newFalAccount(fs.URL)
	in := newSubmitInput(acc, groupID, 1)
	task, err := svc.SubmitAsync(context.Background(), in)
	require.NoError(t, err)

	// 将失败兜底时间置为过去，触发 reconciler 强制退费置 expired。
	stored, _ := taskRepo.GetByID(context.Background(), task.ID)
	past := time.Now().Add(-time.Minute)
	stored.FailDeadlineAt = &past

	require.NoError(t, svc.ReconcileTask(context.Background(), stored, acc))
	require.Equal(t, AsyncMediaStatusExpired, stored.Status)
	require.InDelta(t, 100.0, userRepo.balance, 1e-9) // 全额退还
	require.Equal(t, BillingStatusRefunded, taskRepo.lastUsageLog().BillingStatus)
}
