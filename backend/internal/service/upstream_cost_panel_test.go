//go:build unit

package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// 本文件是「经营对账」模块删除后，针对**取代它的新链路**补的回归测试。
//
// 被删掉的旧测试里有一条验的是「管理面板上配的 A6 凭据必须刷进客户端」——
// 这条断言在新架构下不但没有失效，反而更关键：新的成本取数功能同样要靠面板
// 上的 A6 连接去反查账单，凭据没生效就等于取数全线失败。因此这里把那条断言
// 原样搬到上游成本取数上，而不是把测试一起删掉。
//
// 为什么不用「配置层的地址」来构造客户端：客户端构造时故意给一个连不上的
// 地址（127.0.0.1:9），面板覆盖生效才会指向本测试起的假 A6。这样一旦覆盖
// 逻辑被破坏，测试会以「连不上」而不是「断言失败」的形式炸掉，定位更快。

// upstreamCostRepoStub 记录取数结果，供断言。
type upstreamCostRepoStub struct {
	mu       sync.Mutex
	pending  []UpstreamCostPending
	resolved map[int64]UpstreamCostValue
	retried  []int64

	// 兜底补账的观测点：记下每次调用的窗口与上限，并允许伪造返回值/错误。
	requeueCalls  []upstreamCostRequeueCall
	requeueResult int64
	requeueErr    error

	// 「给探针失败回填上游商户」这条旁路的数据。
	// 绝大多数用例不关心它，默认零值即可安全跳过（list 返回空）。
	opsBackfill []OpsSupplierBackfill
	opsResolved map[int64]OpsSupplierBackfillResult
}

// OpsSupplierBackfillResult 记录一次回填写入，供断言使用。
type OpsSupplierBackfillResult struct {
	SupplierID   *int
	SupplierName string
}

// upstreamCostRequeueCall 记下一次补账调用收到的参数，供断言窗口与封顶值。
type upstreamCostRequeueCall struct {
	since       time.Time
	until       time.Time
	maxAttempts int
	limit       int
}

func newUpstreamCostRepoStub(pending ...UpstreamCostPending) *upstreamCostRepoStub {
	return &upstreamCostRepoStub{pending: pending, resolved: map[int64]UpstreamCostValue{}}
}

func (r *upstreamCostRepoStub) ListUpstreamCostPending(
	_ context.Context, _ time.Time, _ time.Duration, _ int, _ int,
) ([]UpstreamCostPending, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]UpstreamCostPending(nil), r.pending...), nil
}

func (r *upstreamCostRepoStub) ResolveUpstreamCost(
	_ context.Context, usageLogID int64, value UpstreamCostValue, _ time.Time,
) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.resolved[usageLogID] = value
	return nil
}

func (r *upstreamCostRepoStub) RetryUpstreamCost(_ context.Context, usageLogID int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.retried = append(r.retried, usageLogID)
	return nil
}

func (r *upstreamCostRepoStub) RequeueExhaustedCosts(
	_ context.Context, since time.Time, until time.Time, maxAttempts int, limit int,
) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.requeueCalls = append(r.requeueCalls, upstreamCostRequeueCall{
		since: since, until: until, maxAttempts: maxAttempts, limit: limit,
	})
	return r.requeueResult, r.requeueErr
}

func (r *upstreamCostRepoStub) requeueCallsSnapshot() []upstreamCostRequeueCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]upstreamCostRequeueCall(nil), r.requeueCalls...)
}

// ListOpsSupplierBackfill 默认返回空：绝大多数用例不关心「给探针失败回填商户」
// 这条旁路，给它一个安全的空实现，避免每个用例都被迫准备数据。
func (r *upstreamCostRepoStub) ListOpsSupplierBackfill(_ context.Context, _ time.Time, _ int) ([]OpsSupplierBackfill, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]OpsSupplierBackfill(nil), r.opsBackfill...), nil
}

func (r *upstreamCostRepoStub) ResolveOpsSupplier(_ context.Context, opsLogID int64, supplierID *int, supplierName string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.opsResolved == nil {
		r.opsResolved = map[int64]OpsSupplierBackfillResult{}
	}
	r.opsResolved[opsLogID] = OpsSupplierBackfillResult{SupplierID: supplierID, SupplierName: supplierName}
	return nil
}

func (r *upstreamCostRepoStub) resolvedValue(id int64) (UpstreamCostValue, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	v, ok := r.resolved[id]
	return v, ok
}

// newUpstreamCostFakeA6 起一个最小可用的假 A6：/api/status 给计费单位，
// /api/log/self 回一条账单。calls 用来断言「凭据不齐时一个请求都不发」。
func newUpstreamCostFakeA6(t *testing.T, requestID string) (*httptest.Server, *int32) {
	t.Helper()
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case a6StatusPath:
			_, _ = w.Write([]byte(`{"success":true,"data":{"quota_per_unit":500000}}`))
		case a6SelfLogPath:
			atomic.AddInt32(&calls, 1)
			_, _ = w.Write([]byte(`{"success":true,"data":{"items":[
				{"request_id":"` + requestID + `","created_at":1730000100,"model_name":"claude-sonnet-4-5",
				 "token_name":"token-a","prompt_tokens":10,"completion_tokens":20,"quota":250000}
			],"total":1}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server, &calls
}

func newUpstreamCostPanelSettings(t *testing.T, panelBaseURL string) *ReconciliationA6SettingsService {
	t.Helper()
	settings := NewReconciliationA6SettingsService(
		newA6SettingsStateRepoStub(), &a6SettingsEncryptorStub{}, a6SettingsDefaults(), 7.2,
	)
	_, err := settings.Update(context.Background(), ReconciliationA6SettingsInput{
		BaseURL:     a6SettingsStringPtr(panelBaseURL),
		UserID:      a6SettingsStringPtr("panel-user"),
		AccessToken: a6SettingsStringPtr("panel-token-abcdefgh"),
	})
	require.NoError(t, err)
	return settings
}

func TestUpstreamCostServiceUsesPanelA6Override(t *testing.T) {
	ctx := context.Background()
	server, calls := newUpstreamCostFakeA6(t, "req-override")

	// 配置层给一个连不上的地址：面板覆盖没生效就会去连 127.0.0.1:9 并失败。
	client := NewA6Client(ReconciliationA6Config{
		BaseURL:     "http://127.0.0.1:9",
		AccessToken: "config-token-000000",
		UserID:      "config-user",
		Timeout:     5 * time.Second,
	})
	settings := newUpstreamCostPanelSettings(t, server.URL)

	repo := newUpstreamCostRepoStub(UpstreamCostPending{
		UsageLogID: 42,
		RequestID:  "req-override",
		CreatedAt:  time.Unix(1730000000, 0).UTC(),
	})
	svc := NewUpstreamCostService(repo, client, settings, UpstreamCostCollectorConfig{})

	result, err := svc.RunOnce(ctx, time.Unix(1730003600, 0).UTC())
	require.NoError(t, err)
	require.Equal(t, 1, result.Examined)
	require.Equal(t, 1, result.Resolved, "面板覆盖生效才能查到账单")
	require.Zero(t, result.Failed)
	require.Equal(t, int32(1), atomic.LoadInt32(calls))

	value, ok := repo.resolvedValue(42)
	require.True(t, ok, "查到账单后必须写回使用记录")
	require.Equal(t, "USD", value.Currency)

	// 客户端被刷成了面板上的那套凭据。
	require.Equal(t, server.URL, client.Config().BaseURL, "面板覆盖必须刷进客户端")
	require.Equal(t, "panel-token-abcdefgh", client.Config().AccessToken)
	require.Equal(t, "panel-user", client.Config().UserID)
}

func TestUpstreamCostServiceSendsNothingWithoutCredentials(t *testing.T) {
	ctx := context.Background()
	server, calls := newUpstreamCostFakeA6(t, "req-nocreds")

	// 客户端本身没有可用凭据（token 空），且设置服务里也解不出覆盖值：
	// 这种情况下一个请求都不该发出去，避免拿着空凭据去撞上游。
	client := NewA6Client(ReconciliationA6Config{BaseURL: server.URL, AccessToken: "", UserID: "u"})
	settings := NewReconciliationA6SettingsService(
		newA6SettingsStateRepoStub(), &a6SettingsEncryptorStub{},
		ReconciliationA6Config{BaseURL: server.URL}, 7.2,
	)

	repo := newUpstreamCostRepoStub(UpstreamCostPending{
		UsageLogID: 7,
		RequestID:  "req-nocreds",
		CreatedAt:  time.Unix(1730000000, 0).UTC(),
	})
	svc := NewUpstreamCostService(repo, client, settings, UpstreamCostCollectorConfig{})

	result, err := svc.RunOnce(ctx, time.Unix(1730003600, 0).UTC())
	// 凭据不齐时取数整体暂停，并以「未配置」这个可识别的错误上报；
	// 采集器据此把这一持续状态压成一次日志，而不是每 30 秒刷一条错误。
	require.ErrorIs(t, err, ErrReconciliationA6NotConfigured)
	require.Zero(t, result.Resolved)
	require.False(t, client.Configured(), "凭据不齐时客户端不应被视为可用")
	require.Zero(t, atomic.LoadInt32(calls), "凭据不齐时一个请求都不该发出去")
	_, resolved := repo.resolvedValue(7)
	require.False(t, resolved, "没查到就不能写成本，否则等于谎报为零成本")
}
