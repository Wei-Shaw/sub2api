//go:build unit

package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// ==================== 流式解析 ====================

func TestMonitorRequestBodyStreams(t *testing.T) {
	require.True(t, monitorRequestBodyStreams([]byte("{\"stream\":true,\"model\":\"m\"}")))
	require.False(t, monitorRequestBodyStreams([]byte("{\"stream\":false}")))
	require.False(t, monitorRequestBodyStreams([]byte("{\"model\":\"m\"}")))
	require.False(t, monitorRequestBodyStreams([]byte("not-json")))
}

func TestMonitorStreamContent(t *testing.T) {
	chat, done := monitorStreamContent(monitorStreamChat, "{\"choices\":[{\"delta\":{\"content\":\"7\"}}]}")
	require.Equal(t, "7", chat)
	require.False(t, done)

	delta, done := monitorStreamContent(monitorStreamResponses, "{\"type\":\"response.output_text.delta\",\"delta\":\"ok\"}")
	require.Equal(t, "ok", delta)
	require.False(t, done)

	_, done = monitorStreamContent(monitorStreamResponses, "{\"type\":\"response.completed\"}")
	require.True(t, done)

	empty, done := monitorStreamContent(monitorStreamChat, "{\"choices\":[]}")
	require.Empty(t, empty)
	require.False(t, done)
}

func TestReadMonitorSSEResponses(t *testing.T) {
	body := strings.Join([]string{
		"event: response.output_text.delta",
		"data: {\"type\":\"response.output_text.delta\",\"delta\":\"ok\"}",
		"",
		"data: {\"type\":\"response.completed\",\"response\":{}}",
		"",
	}, "\n")

	text, firstTokenMs, raw, _ := readMonitorSSE(strings.NewReader(body), monitorStreamResponses, time.Now())
	require.Equal(t, "ok", text)
	require.NotNil(t, firstTokenMs, "首个内容块到达就该记下首字")
	require.Contains(t, raw, "response.completed")
}

func TestReadMonitorSSEChat(t *testing.T) {
	body := "data: {\"choices\":[{\"delta\":{\"content\":\"1\"}}]}\n\n" +
		"data: {\"choices\":[{\"delta\":{\"content\":\"2\"}}]}\n\n" +
		"data: [DONE]\n\n"

	text, firstTokenMs, _, _ := readMonitorSSE(strings.NewReader(body), monitorStreamChat, time.Now())
	require.Equal(t, "12", text)
	require.NotNil(t, firstTokenMs)
}

func TestMonitorUpstreamRequestID(t *testing.T) {
	headers := http.Header{}
	headers.Set("X-Oneapi-Request-Id", "req-1")
	opts := &CheckOptions{UpstreamRequestIDHeader: "X-Oneapi-Request-Id"}
	require.Equal(t, "req-1", monitorUpstreamRequestID(opts, headers))

	// 没配头名 / 没响应头 / nil opts 都返回空串。
	require.Empty(t, monitorUpstreamRequestID(&CheckOptions{}, headers))
	require.Empty(t, monitorUpstreamRequestID(opts, http.Header{}))
	require.Empty(t, monitorUpstreamRequestID(nil, headers))

	// 超长 ID 必须被裁到列宽（usage_logs.upstream_request_id VARCHAR(128)）。
	headers.Set("X-Oneapi-Request-Id", strings.Repeat("a", 400))
	got := monitorUpstreamRequestID(opts, headers)
	require.Len(t, got, maxUsageUpstreamRequestIDLen)
}

// TestCallProviderStreamReadsFirstToken 覆盖「Body 里 stream:true → 走 SSE → 记首字」的整条链路。
func TestCallProviderStreamReadsFirstToken(t *testing.T) {
	swapMonitorHTTPClient(t)
	sse := "event: response.output_text.delta\n" +
		"data: {\"type\":\"response.output_text.delta\",\"delta\":\"7\"}\n\n" +
		"data: {\"type\":\"response.completed\"}\n\n"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("X-Oneapi-Request-Id", "req-stream-1")
		_, _ = w.Write([]byte(sse))
	}))
	t.Cleanup(server.Close)

	opts := &CheckOptions{
		APIMode:                 MonitorAPIModeResponses,
		BodyOverrideMode:        MonitorBodyOverrideModeReplace,
		BodyOverride:            map[string]any{"model": "gpt-6-astra", "instructions": "x", "input": "hi", "stream": true},
		UpstreamRequestIDHeader: "X-Oneapi-Request-Id",
	}
	call, err := callProvider(context.Background(), MonitorProviderOpenAI, server.URL, "k", "gpt-6-astra", "hi", opts)
	require.NoError(t, err)
	require.True(t, call.Stream)
	require.Equal(t, http.StatusOK, call.Status)
	require.Equal(t, "7", call.Text)
	require.NotNil(t, call.FirstTokenMs)
	require.Equal(t, "req-stream-1", monitorUpstreamRequestID(opts, call.Headers))
	// 记账行的「上游」列要显示**协议路径**，不是 base URL：
	// 拿到域名管理员看不出这次探针验的是哪条协议链路。
	//
	// ⚠️ 这里必须跟着本用例的 APIMode 走：opts 设的是 responses，
	// 所以真实路径就是 providerOpenAIResponsesPath。曾误写成 providerOpenAIPath
	// （chat 路径），而当时用的 -run 过滤器没命中本测试名，导致错误断言
	// 在"验证通过"的表象下存活了一轮 —— 断言协议路径时务必与 apiMode 对齐。
	require.Equal(t, providerOpenAIResponsesPath, call.Endpoint)
	require.True(t, strings.HasPrefix(call.Endpoint, "/v1/"), "必须是路径形态，不能是完整 URL")

	// 同样的 Body 换成 stream:false（默认 body 形态）时必须走整包读取，不产生首字。
	opts.BodyOverride["stream"] = false
	call, err = callProvider(context.Background(), MonitorProviderOpenAI, server.URL, "k", "gpt-6-astra", "hi", opts)
	require.NoError(t, err)
	require.False(t, call.Stream)
	require.Nil(t, call.FirstTokenMs)
}

func TestReadMonitorSSEResponsesUsage(t *testing.T) {
	body := "data: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":100,\"output_tokens\":20,\"input_tokens_details\":{\"cached_tokens\":30}}}}\n\n"
	_, _, _, usage := readMonitorSSE(strings.NewReader(body), monitorStreamResponses, time.Now())
	require.Equal(t, 70, usage.Input, "OpenAI 口径：input_tokens 含缓存，存储时拆出去")
	require.Equal(t, 20, usage.Output)
	require.Equal(t, 30, usage.CacheRead)
}

// ==================== 记账 ====================

type monitorUsageAPIKeyRepoStub struct {
	APIKeyRepository
	local    *APIKey
	localErr error
	existing []APIKey
	created  *APIKey
	nextID   int64
}

func (s *monitorUsageAPIKeyRepoStub) GetByKey(context.Context, string) (*APIKey, error) {
	return s.local, s.localErr
}

func (s *monitorUsageAPIKeyRepoStub) SearchAPIKeys(context.Context, int64, string, int) ([]APIKey, error) {
	return s.existing, nil
}

func (s *monitorUsageAPIKeyRepoStub) Create(_ context.Context, key *APIKey) error {
	if s.nextID == 0 {
		s.nextID = 900
	}
	key.ID = s.nextID
	s.nextID++
	s.created = key
	return nil
}

type monitorUsageAccountRepoStub struct {
	AccountRepository
	accounts []Account
	err      error
}

func (s *monitorUsageAccountRepoStub) ListSchedulableByGroupID(context.Context, int64) ([]Account, error) {
	return s.accounts, s.err
}

func newMonitorUsageRecorder(
	logs *intelligenceCheckUsageLogRepoStub,
	keys *monitorUsageAPIKeyRepoStub,
	accounts *monitorUsageAccountRepoStub,
) *ChannelMonitorUsageRecorder {
	users := &intelligenceCheckUsageUserRepoStub{admin: &User{ID: 1}}
	return NewChannelMonitorUsageRecorder(logs, users, keys, accounts)
}

func monitorUsageTestMonitor() *ChannelMonitor {
	groupID := int64(37)
	return &ChannelMonitor{ID: 13, GroupID: &groupID, APIKey: "a6-upstream-key", PrimaryModel: "gpt-6-astra"}
}

func TestChannelMonitorUsagePrepareRecordsExternalProbe(t *testing.T) {
	keys := &monitorUsageAPIKeyRepoStub{localErr: ErrAPIKeyNotFound}
	accounts := &monitorUsageAccountRepoStub{accounts: []Account{{
		ID:    48,
		Extra: map[string]any{AccountExtraUpstreamRequestIDHeader: "X-Oneapi-Request-Id"},
	}}}
	rec := newMonitorUsageRecorder(&intelligenceCheckUsageLogRepoStub{}, keys, accounts)

	probe := rec.prepareProbeUsage(context.Background(), monitorUsageTestMonitor())
	require.NotNil(t, probe)
	require.Equal(t, int64(48), probe.accountID)
	require.Equal(t, "X-Oneapi-Request-Id", probe.requestIDHeader)
}

func TestChannelMonitorUsagePrepareSkipsLocalProbe(t *testing.T) {
	// 探针 Key 能在本站 api_keys 里查到 → 走的是本地网关，网关已经记过账。
	keys := &monitorUsageAPIKeyRepoStub{local: &APIKey{ID: 7, UserID: 1}}
	accounts := &monitorUsageAccountRepoStub{accounts: []Account{{ID: 48}}}
	rec := newMonitorUsageRecorder(&intelligenceCheckUsageLogRepoStub{}, keys, accounts)

	require.Nil(t, rec.prepareProbeUsage(context.Background(), monitorUsageTestMonitor()))
}

func TestChannelMonitorUsagePrepareSkipsWithoutGroup(t *testing.T) {
	keys := &monitorUsageAPIKeyRepoStub{localErr: ErrAPIKeyNotFound}
	accounts := &monitorUsageAccountRepoStub{accounts: []Account{{ID: 48}}}
	rec := newMonitorUsageRecorder(&intelligenceCheckUsageLogRepoStub{}, keys, accounts)

	m := monitorUsageTestMonitor()
	m.GroupID = nil
	require.Nil(t, rec.prepareProbeUsage(context.Background(), m))

	empty := int64(0)
	m.GroupID = &empty
	require.Nil(t, rec.prepareProbeUsage(context.Background(), m))
}

func TestChannelMonitorUsageRecordWritesRow(t *testing.T) {
	logs := &intelligenceCheckUsageLogRepoStub{}
	keys := &monitorUsageAPIKeyRepoStub{
		localErr: ErrAPIKeyNotFound,
		existing: []APIKey{{ID: 302, UserID: 1, Name: channelMonitorUsageKeyName}},
	}
	accounts := &monitorUsageAccountRepoStub{accounts: []Account{{ID: 48}}}
	rec := newMonitorUsageRecorder(logs, keys, accounts)
	m := monitorUsageTestMonitor()
	probe := rec.prepareProbeUsage(context.Background(), m)
	require.NotNil(t, probe)

	latency := 2500
	firstToken := 321
	results := []*CheckResult{{
		Model:             "gpt-6-astra",
		Status:            MonitorStatusOperational,
		StatusCode:        200,
		Stream:            true,
		FirstTokenMs:      &firstToken,
		Usage:             ProbeUsageTokens{Input: 70, Output: 20, CacheCreation: 5, CacheRead: 30},
		LatencyMs:         &latency,
		UpstreamRequestID: "req-1",
		// 探针真实打到的上游端点：使用记录页的「上游」列读的就是它。
		// 留空会让那一列显示成「-」，管理员就看不出这次探针验的是哪条协议链路。
		UpstreamEndpoint: "/v1/responses",
		CheckedAt:        time.Now(),
	}}

	rec.record(context.Background(), m, results, probe)

	require.Len(t, logs.logs, 1)
	log := logs.logs[0]
	require.Equal(t, int64(1), log.UserID)
	require.Equal(t, int64(302), log.APIKeyID)
	require.Equal(t, int64(48), log.AccountID)
	require.NotNil(t, log.GroupID)
	require.Equal(t, int64(37), *log.GroupID)
	require.Equal(t, "gpt-6-astra", log.Model)
	require.Equal(t, ChannelMonitorUsageInboundEndpoint, *log.InboundEndpoint)
	require.Equal(t, RequestTypeStream, log.RequestType)
	require.True(t, log.Stream)
	require.Equal(t, 2500, *log.DurationMs)
	require.Equal(t, 321, *log.FirstTokenMs)
	require.Equal(t, 70, log.InputTokens)
	require.Equal(t, 20, log.OutputTokens)
	require.Equal(t, 5, log.CacheCreationTokens)
	require.Equal(t, 30, log.CacheReadTokens)
	require.Equal(t, "req-1", *log.UpstreamRequestID)
	require.NotNil(t, log.UpstreamEndpoint)
	require.Equal(t, "/v1/responses", *log.UpstreamEndpoint)
	// 纯成本：没有向任何人收费，成本由 A6 反查回填。
	require.Zero(t, log.TotalCost)
	require.Zero(t, log.ActualCost)
}

// 首字口径：**收到第一个 SSE 数据块**即计时，而不是等第一段正文。
//
// 这是使用记录页「延迟差」能否成立的前提：上游首字来自 A6 账单（上游第一个字节），
// 若这里等到正文才计时，推理模型下会虚高整个推理时长 —— 「中转开销」会从几百毫秒
// 变成几秒甚至十几秒。
//
// 本用例刻意构造「整条流里没有任何正文」的流：旧实现在这种流上返回 nil，
// 新实现返回一个值，因此无需依赖真实计时即可确定性地钉住口径。
func TestReadMonitorSSEFirstTokenAtFirstDataChunk(t *testing.T) {
	body := strings.Join([]string{
		`data: {"type":"response.created","response":{"id":"r1"}}`,
		"",
		`data: {"type":"response.reasoning_summary_text.delta","delta":"thinking"}`,
		"",
		`data: {"type":"response.completed","response":{}}`,
		"",
	}, "\n")

	text, firstTokenMs, _, _ := readMonitorSSE(strings.NewReader(body), monitorStreamResponses, time.Now())

	require.Empty(t, text, "整条流没有正文，正文应为空")
	require.NotNil(t, firstTokenMs, "首个数据块到达就该记首字，不能等到正文出现")
	require.GreaterOrEqual(t, *firstTokenMs, 0)
}

func TestChannelMonitorUsageRecordSkipsNon2xx(t *testing.T) {
	logs := &intelligenceCheckUsageLogRepoStub{}
	keys := &monitorUsageAPIKeyRepoStub{
		localErr: ErrAPIKeyNotFound,
		existing: []APIKey{{ID: 302, UserID: 1, Name: channelMonitorUsageKeyName}},
	}
	rec := newMonitorUsageRecorder(logs, keys, &monitorUsageAccountRepoStub{accounts: []Account{{ID: 48}}})
	m := monitorUsageTestMonitor()
	probe := rec.prepareProbeUsage(context.Background(), m)
	require.NotNil(t, probe)

	results := []*CheckResult{
		{Model: "gpt-6-astra", Status: MonitorStatusError, StatusCode: 500},
		{Model: "gpt-6-astra", Status: MonitorStatusError, StatusCode: 0},
	}
	rec.record(context.Background(), m, results, probe)
	require.Empty(t, logs.logs, "非 2xx / 没拿到响应不该记消费")
}

func TestChannelMonitorUsageRecordNoopWithoutProbe(t *testing.T) {
	logs := &intelligenceCheckUsageLogRepoStub{}
	rec := newMonitorUsageRecorder(logs, &monitorUsageAPIKeyRepoStub{}, &monitorUsageAccountRepoStub{})
	results := []*CheckResult{{Model: "m", StatusCode: 200}}
	require.NotPanics(t, func() {
		rec.record(context.Background(), monitorUsageTestMonitor(), results, nil)
	})
	require.Empty(t, logs.logs)
}
