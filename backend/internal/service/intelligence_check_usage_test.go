package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// ==================== 捕获器 ====================

func intelligenceCheckTestGinContext(t *testing.T) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	return ctx
}

func upstreamRequestIDAccount() *Account {
	return &Account{Extra: map[string]any{AccountExtraUpstreamRequestIDHeader: "X-Upstream-Request-Id"}}
}

func TestIntelligenceCheckCaptureCollectsAndDedupsRequestIDs(t *testing.T) {
	c := intelligenceCheckTestGinContext(t)
	capture := &intelligenceCheckUpstreamCapture{}
	withIntelligenceCheckUpstreamCapture(c, capture)
	account := upstreamRequestIDAccount()

	responded, ids := capture.snapshot()
	require.False(t, responded)
	require.Empty(t, ids)

	header := http.Header{}
	header.Set("X-Upstream-Request-Id", "req-1")
	captureIntelligenceCheckUpstreamRequestID(c, account, header)

	responded, ids = capture.snapshot()
	require.True(t, responded)
	require.Equal(t, []string{"req-1"}, ids)

	// 同一个 ID 再来一次不重复；换成新 ID 追加。
	captureIntelligenceCheckUpstreamRequestID(c, account, header)
	header.Set("X-Upstream-Request-Id", "req-2")
	captureIntelligenceCheckUpstreamRequestID(c, account, header)

	responded, ids = capture.snapshot()
	require.True(t, responded)
	require.Equal(t, []string{"req-1", "req-2"}, ids)
}

func TestIntelligenceCheckCaptureRecordsResponseWithoutRequestID(t *testing.T) {
	c := intelligenceCheckTestGinContext(t)
	capture := &intelligenceCheckUpstreamCapture{}
	withIntelligenceCheckUpstreamCapture(c, capture)

	// 账号没配 upstream_request_id_header：仍要记下「收到过响应」。
	captureIntelligenceCheckUpstreamRequestID(c, &Account{}, http.Header{})

	responded, ids := capture.snapshot()
	require.True(t, responded)
	require.Empty(t, ids)
}

func TestCaptureIntelligenceCheckUpstreamRequestIDNoopWithoutCapture(t *testing.T) {
	c := intelligenceCheckTestGinContext(t)
	// 官方连通性测试没装捕获器：钩子必须退化为空操作。
	require.NotPanics(t, func() {
		captureIntelligenceCheckUpstreamRequestID(c, upstreamRequestIDAccount(), http.Header{})
	})
}

func TestIntelligenceCheckCaptureKeepsFirstContentTimestamp(t *testing.T) {
	c := intelligenceCheckTestGinContext(t)
	capture := &intelligenceCheckUpstreamCapture{}
	withIntelligenceCheckUpstreamCapture(c, capture)

	require.True(t, capture.firstContentTimestamp().IsZero(), "没内容事件前不该有首字时刻")
	markIntelligenceCheckFirstContent(c)
	first := capture.firstContentTimestamp()
	require.False(t, first.IsZero())

	time.Sleep(2 * time.Millisecond)
	markIntelligenceCheckFirstContent(c)
	require.Equal(t, first, capture.firstContentTimestamp(), "首字只认第一次")
}

func TestMarkIntelligenceCheckFirstContentNoopWithoutCapture(t *testing.T) {
	c := intelligenceCheckTestGinContext(t)
	require.NotPanics(t, func() { markIntelligenceCheckFirstContent(c) })
}

func TestIntelligenceCheckCaptureUsage(t *testing.T) {
	c := intelligenceCheckTestGinContext(t)
	capture := &intelligenceCheckUpstreamCapture{}
	withIntelligenceCheckUpstreamCapture(c, capture)
	require.True(t, capture.usageSnapshot().IsZero())

	markIntelligenceCheckUsage(c, parseProbeJSONObject("{\"usage\":{\"input_tokens\":100,\"output_tokens\":20,\"input_tokens_details\":{\"cached_tokens\":30}}}"))
	usage := capture.usageSnapshot()
	require.Equal(t, 70, usage.Input)
	require.Equal(t, 20, usage.Output)
	require.Equal(t, 30, usage.CacheRead)
}

func TestMarkIntelligenceCheckUsageNoopWithoutCapture(t *testing.T) {
	c := intelligenceCheckTestGinContext(t)
	require.NotPanics(t, func() {
		markIntelligenceCheckUsage(c, map[string]any{"usage": map[string]any{"input_tokens": 1}})
	})
}

// ==================== 记账 ====================

type intelligenceCheckUsageLogRepoStub struct {
	UsageLogRepository
	logs []*UsageLog
}

func (s *intelligenceCheckUsageLogRepoStub) CreateBestEffort(_ context.Context, log *UsageLog) error {
	s.logs = append(s.logs, log)
	return nil
}

type intelligenceCheckUsageUserRepoStub struct {
	UserRepository
	admin *User
	err   error
}

func (s *intelligenceCheckUsageUserRepoStub) GetFirstAdmin(context.Context) (*User, error) {
	return s.admin, s.err
}

type intelligenceCheckUsageAPIKeyRepoStub struct {
	APIKeyRepository
	existing  []APIKey
	searchErr error
	created   *APIKey
	createErr error
	nextID    int64
}

func (s *intelligenceCheckUsageAPIKeyRepoStub) SearchAPIKeys(context.Context, int64, string, int) ([]APIKey, error) {
	return s.existing, s.searchErr
}

func (s *intelligenceCheckUsageAPIKeyRepoStub) Create(_ context.Context, key *APIKey) error {
	if s.createErr != nil {
		return s.createErr
	}
	if s.nextID == 0 {
		s.nextID = 1000
	}
	key.ID = s.nextID
	s.nextID++
	s.created = key
	return nil
}

func newIntelligenceCheckUsageService(logs *intelligenceCheckUsageLogRepoStub, users *intelligenceCheckUsageUserRepoStub, keys *intelligenceCheckUsageAPIKeyRepoStub) *IntelligenceCheckService {
	return NewIntelligenceCheckService(nil, nil, nil, nil, logs, users, keys)
}

func TestRecordRunUsageWritesPureCostRowPerUpstreamRequest(t *testing.T) {
	logs := &intelligenceCheckUsageLogRepoStub{}
	users := &intelligenceCheckUsageUserRepoStub{admin: &User{ID: 1}}
	keys := &intelligenceCheckUsageAPIKeyRepoStub{existing: []APIKey{{ID: 55, UserID: 1, Name: intelligenceCheckUsageKeyName}}}
	svc := newIntelligenceCheckUsageService(logs, users, keys)

	startedAt := time.Now().Add(-time.Minute)
	firstToken := 250
	run := &IntelligenceCheckRun{ID: 9, AccountID: 7, ModelID: "claude-sonnet-4", ReasoningEffort: "xhigh"}
	probe := &IntelligenceCheckProbeResult{
		Status:             IntelligenceCheckStatusCompleted,
		UpstreamModel:      "claude-sonnet-4",
		LatencyMs:          1234,
		StartedAt:          startedAt,
		FinishedAt:         startedAt.Add(time.Second),
		UpstreamResponded:  true,
		UpstreamRequestIDs: []string{"req-a", "req-b"},
		FirstTokenMs:       &firstToken,
		Usage:              ProbeUsageTokens{Input: 11, Output: 22, CacheCreation: 3, CacheRead: 4},
	}

	svc.recordRunUsage(context.Background(), run, IntelligenceCheckRequest{AccountID: 7, DisableStream: true}, probe)

	require.Len(t, logs.logs, 2)
	for i, log := range logs.logs {
		require.Equal(t, int64(1), log.UserID)
		require.Equal(t, int64(55), log.APIKeyID)
		require.Equal(t, int64(7), log.AccountID)
		require.Equal(t, "claude-sonnet-4", log.Model)
		require.Equal(t, intelligenceCheckUsageInboundEndpoint, *log.InboundEndpoint)
		// 纯成本口径：没有向任何人收费，费用留 0，成本由 A6 反查回填。
		require.Zero(t, log.TotalCost)
		require.Zero(t, log.ActualCost)
		require.Equal(t, 1.0, log.RateMultiplier)
		require.Equal(t, RequestTypeSync, log.RequestType)
		require.False(t, log.Stream)
		require.Equal(t, 1234, *log.DurationMs)
		require.Equal(t, 250, *log.FirstTokenMs)
		require.Equal(t, 11, log.InputTokens)
		require.Equal(t, 22, log.OutputTokens)
		require.Equal(t, 3, log.CacheCreationTokens)
		require.Equal(t, 4, log.CacheReadTokens)
		require.Equal(t, startedAt, log.CreatedAt)
		require.Equal(t, "claude-sonnet-4", *log.UpstreamModel)
		require.Equal(t, "xhigh", *log.ReasoningEffort)
		require.NotNil(t, log.UpstreamRequestID)
		require.Equal(t, []string{"req-a", "req-b"}[i], *log.UpstreamRequestID)
		require.NotEqual(t, logs.logs[0].RequestID, logs.logs[1].RequestID, "同一轮的两个上游请求必须各占一行")
	}
}

func TestRecordRunUsageSkipsWhenUpstreamNeverResponded(t *testing.T) {
	logs := &intelligenceCheckUsageLogRepoStub{}
	users := &intelligenceCheckUsageUserRepoStub{admin: &User{ID: 1}}
	keys := &intelligenceCheckUsageAPIKeyRepoStub{existing: []APIKey{{ID: 55, UserID: 1, Name: intelligenceCheckUsageKeyName}}}
	svc := newIntelligenceCheckUsageService(logs, users, keys)

	run := &IntelligenceCheckRun{ID: 9, AccountID: 7, ModelID: "claude-sonnet-4"}
	probe := &IntelligenceCheckProbeResult{Status: IntelligenceCheckStatusFailed, StartedAt: time.Now()}

	svc.recordRunUsage(context.Background(), run, IntelligenceCheckRequest{AccountID: 7}, probe)
	require.Empty(t, logs.logs, "没有上游响应就不该记任何消费")
}

func TestRecordRunUsageRecordsRowWithoutRequestID(t *testing.T) {
	logs := &intelligenceCheckUsageLogRepoStub{}
	users := &intelligenceCheckUsageUserRepoStub{admin: &User{ID: 1}}
	keys := &intelligenceCheckUsageAPIKeyRepoStub{existing: []APIKey{{ID: 55, UserID: 1, Name: intelligenceCheckUsageKeyName}}}
	svc := newIntelligenceCheckUsageService(logs, users, keys)

	run := &IntelligenceCheckRun{ID: 9, AccountID: 7, ModelID: "claude-sonnet-4"}
	probe := &IntelligenceCheckProbeResult{UpstreamResponded: true, StartedAt: time.Now()}

	svc.recordRunUsage(context.Background(), run, IntelligenceCheckRequest{AccountID: 7}, probe)

	require.Len(t, logs.logs, 1)
	require.Nil(t, logs.logs[0].UpstreamRequestID, "拿不到 ID 时留空，绝不编一个去查账单")
}

func TestRecordRunUsageCreatesBookkeepingKeyUnderAdmin(t *testing.T) {
	logs := &intelligenceCheckUsageLogRepoStub{}
	users := &intelligenceCheckUsageUserRepoStub{admin: &User{ID: 1}}
	keys := &intelligenceCheckUsageAPIKeyRepoStub{nextID: 77}
	svc := newIntelligenceCheckUsageService(logs, users, keys)

	run := &IntelligenceCheckRun{ID: 9, AccountID: 7, ModelID: "claude-sonnet-4"}
	probe := &IntelligenceCheckProbeResult{UpstreamResponded: true, UpstreamRequestIDs: []string{"req-a"}, StartedAt: time.Now()}

	svc.recordRunUsage(context.Background(), run, IntelligenceCheckRequest{AccountID: 7}, probe)

	require.Len(t, logs.logs, 1)
	require.NotNil(t, keys.created)
	require.Equal(t, intelligenceCheckUsageKeyName, keys.created.Name)
	require.Equal(t, StatusDisabled, keys.created.Status)
	require.Equal(t, int64(77), logs.logs[0].APIKeyID)
	require.Equal(t, int64(1), logs.logs[0].UserID)
}

func TestRecordRunUsageSkipsWhenNoAdmin(t *testing.T) {
	logs := &intelligenceCheckUsageLogRepoStub{}
	users := &intelligenceCheckUsageUserRepoStub{err: ErrUserNotFound}
	keys := &intelligenceCheckUsageAPIKeyRepoStub{}
	svc := newIntelligenceCheckUsageService(logs, users, keys)

	run := &IntelligenceCheckRun{ID: 9, AccountID: 7, ModelID: "claude-sonnet-4"}
	probe := &IntelligenceCheckProbeResult{UpstreamResponded: true, UpstreamRequestIDs: []string{"req-a"}, StartedAt: time.Now()}

	require.NotPanics(t, func() {
		svc.recordRunUsage(context.Background(), run, IntelligenceCheckRequest{AccountID: 7}, probe)
	})
	require.Empty(t, logs.logs)
}

func TestIntelligenceCheckUsageRequestIDIsUniquePerIndex(t *testing.T) {
	first := intelligenceCheckUsageRequestID(9, 0)
	second := intelligenceCheckUsageRequestID(9, 1)
	require.NotEqual(t, first, second)
	require.LessOrEqual(t, len(first), 64, "request_id 列宽 64")
}
