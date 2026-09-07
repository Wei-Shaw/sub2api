//go:build unit

package service

import (
	"bytes"
	"context"
	"encoding/binary"
	"hash/crc32"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/kiro"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// --- AWS event-stream 帧构造（复刻 internal/pkg/kiro/eventstream_test.go 与
// stream_test.go 里同名的未导出测试 helper——那两个属于 package kiro，本文件
// 在 package service，无法直接引用，只能照抄同一套编码逻辑）。

func kiroTestBuildFrame(headers [][2]string, payload []byte) []byte {
	var hdr []byte
	for _, kv := range headers {
		name := kv[0]
		val := kv[1]
		hdr = append(hdr, byte(len(name)))
		hdr = append(hdr, name...)
		hdr = append(hdr, 7) // string
		var l [2]byte
		binary.BigEndian.PutUint16(l[:], uint16(len(val)))
		hdr = append(hdr, l[:]...)
		hdr = append(hdr, val...)
	}

	total := uint32(12 + len(hdr) + len(payload) + 4)
	buf := make([]byte, 0, total)
	var u32 [4]byte

	binary.BigEndian.PutUint32(u32[:], total)
	buf = append(buf, u32[:]...)
	binary.BigEndian.PutUint32(u32[:], uint32(len(hdr)))
	buf = append(buf, u32[:]...)

	preludeCRC := crc32.ChecksumIEEE(buf[:8])
	binary.BigEndian.PutUint32(u32[:], preludeCRC)
	buf = append(buf, u32[:]...)

	buf = append(buf, hdr...)
	buf = append(buf, payload...)

	msgCRC := crc32.ChecksumIEEE(buf)
	binary.BigEndian.PutUint32(u32[:], msgCRC)
	buf = append(buf, u32[:]...)

	return buf
}

func kiroTestEventFrame(eventType, payload string) []byte {
	return kiroTestBuildFrame([][2]string{
		{":message-type", "event"},
		{":event-type", eventType},
	}, []byte(payload))
}

func kiroTestConcatFrames(frames ...[]byte) []byte {
	var out []byte
	for _, f := range frames {
		out = append(out, f...)
	}
	return out
}

// --- 假上游：用 httptest 起一个真实的本地 HTTP server，callEndpointOverride
// 把 ForwardUpstream 的每次上游调用都路由过来，而不是连 kiro.EndpointsFor
// 给出的真实 AWS/CLI 域名（单测环境连不通，也不该连）。

func kiroTestFakeUpstream(t *testing.T, handler func(callIndex int) (status int, body []byte)) (*httptest.Server, *int32) {
	t.Helper()
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		idx := int(atomic.AddInt32(&calls, 1)) - 1
		status, body := handler(idx)
		w.WriteHeader(status)
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

func kiroTestOverrideCallingServer(srv *httptest.Server) func(ctx context.Context, account *Account, ep kiro.Endpoint, payload []byte) (*http.Response, error) {
	return func(ctx context.Context, account *Account, ep kiro.Endpoint, payload []byte) (*http.Response, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, srv.URL, bytes.NewReader(payload))
		if err != nil {
			return nil, err
		}
		return http.DefaultClient.Do(req)
	}
}

// kiroRateLimitRecorder 记录每一次 SetModelRateLimit 调用——kiro 账号的
// 调度冷却走这个机制（model_rate_limit.go 的 PlatformKiro case），不经过
// AccountRuntimeBlocker（那个接口唯一的绑定实现对 kiro 账号恒为 no-op，
// 见 NewKiroGatewayService 的文档），所以红线断言（"绝不能因为
// INVALID_MODEL_ID 禁用账号"）与冷却断言都改成对这个 repo 调用记录做
// 断言，而不是对已删除的 runtimeBlocker 字段做断言。
type kiroRateLimitRecorder struct {
	AccountRepository
	mu    sync.Mutex
	calls []kiroRateLimitCall
}

type kiroRateLimitCall struct {
	accountID int64
	scope     string
	resetAt   time.Time
}

func (r *kiroRateLimitRecorder) SetModelRateLimit(_ context.Context, id int64, scope string, resetAt time.Time, _ ...string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, kiroRateLimitCall{accountID: id, scope: scope, resetAt: resetAt})
	return nil
}

func (r *kiroRateLimitRecorder) called() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.calls) > 0
}

var _ AccountRepository = (*kiroRateLimitRecorder)(nil)

// kiroTestOAuthAccount 构造一个通用测试账号，显式关闭假思考——本文件及
// 其余全部通过它构造账号的测试文件关心的都是转发/重试/计费等与假思考无关
// 的逻辑，假思考现在默认开启（KiroFakeThinking 的文档），继续显式关闭能让
// 这些既有测试的既有断言（假流式响应不含 <thinking>、上游 payload 断言等）
// 保持不变；需要测假思考本身行为的测试（如
// TestKiroForwardUpstreamThreadsRequestedEffortIntoFakeThinkingBudget）已经
// 在构造之后显式覆盖这个字段。
func kiroTestOAuthAccount(id int64) *Account {
	return &Account{
		ID:       id,
		Platform: PlatformKiro,
		Credentials: map[string]any{
			"auth_method":   "social",
			"access_token":  "at_1",
			"machine_id":    "stable-machine",
			"fake_thinking": false,
		},
	}
}

const kiroTestRequestBody = `{"model":"claude-sonnet-4-5-20250929","max_tokens":100,"messages":[{"role":"user","content":"hi"}],"stream":true}`

// TestKiroResolveProxyURLFallsBackToRepositoryWhenNotPreloaded 覆盖真实
// 账号测试后走查代码发现的缺口：account.Proxy 没有预加载时，之前的实现
// 直接返回空代理（悄悄走直连），跟 GrokQuotaService.resolveProxyURL 的
// 既有仓储兜底约定不一致。这里断言账号配了代理但 Proxy 字段是 nil（只有
// ProxyID）时，会去查 proxyRepo 补上，而不是假装没配代理。
func TestKiroResolveProxyURLFallsBackToRepositoryWhenNotPreloaded(t *testing.T) {
	proxyID := int64(7)
	repo := &mockProxyRepoForOAuth{
		getByIDFunc: func(ctx context.Context, id int64) (*Proxy, error) {
			require.EqualValues(t, proxyID, id)
			return &Proxy{ID: proxyID, Protocol: "http", Host: "proxy.internal", Port: 8080}, nil
		},
	}

	svc := &KiroGatewayService{proxyRepo: repo}
	account := &Account{ID: 1, Platform: PlatformKiro, ProxyID: &proxyID}

	got := svc.resolveProxyURL(context.Background(), account)
	require.Equal(t, "http://proxy.internal:8080", got)
	require.NotNil(t, account.Proxy, "查到之后应该把结果写回 account.Proxy，避免同一账号在同一次请求里重复查仓储")
}

// TestKiroResolveProxyURLPrefersPreloadedProxyOverRepository 覆盖已预加载
// 场景：account.Proxy 已经有值时，不应该再打一次仓储——快路径必须保留。
func TestKiroResolveProxyURLPrefersPreloadedProxyOverRepository(t *testing.T) {
	proxyID := int64(7)
	repo := &mockProxyRepoForOAuth{
		getByIDFunc: func(ctx context.Context, id int64) (*Proxy, error) {
			t.Fatal("account.Proxy 已经预加载时不应该再查仓储")
			return nil, nil
		},
	}

	svc := &KiroGatewayService{proxyRepo: repo}
	account := &Account{
		ID:       1,
		Platform: PlatformKiro,
		ProxyID:  &proxyID,
		Proxy:    &Proxy{ID: proxyID, Protocol: "socks5", Host: "preloaded.internal", Port: 1080},
	}

	got := svc.resolveProxyURL(context.Background(), account)
	require.Equal(t, "socks5://preloaded.internal:1080", got)
}

// TestKiroResolveProxyURLReturnsEmptyWhenAccountHasNoProxy 覆盖账号本来就
// 没配代理的情况（ProxyID 为 nil）——不应该去查仓储，直接走直连。
func TestKiroResolveProxyURLReturnsEmptyWhenAccountHasNoProxy(t *testing.T) {
	repo := &mockProxyRepoForOAuth{
		getByIDFunc: func(ctx context.Context, id int64) (*Proxy, error) {
			t.Fatal("账号没配代理（ProxyID 为 nil）不应该查仓储")
			return nil, nil
		},
	}

	svc := &KiroGatewayService{proxyRepo: repo}
	account := &Account{ID: 1, Platform: PlatformKiro}

	got := svc.resolveProxyURL(context.Background(), account)
	require.Equal(t, "", got)
}

func kiroTestContext() (*httptest.ResponseRecorder, *gin.Context) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader([]byte(kiroTestRequestBody)))
	return recorder, c
}

// TestKiroForwardUpstreamSuccessStreamingWithRealCacheReadTokens 覆盖成功路径：
// 客户端收到完整的 message_start -> content_block_delta -> message_stop，
// 且 ForwardResult.Usage.CacheReadInputTokens 等于 meteringEvent 里的真实值。
func TestKiroForwardUpstreamSuccessStreamingWithRealCacheReadTokens(t *testing.T) {
	frames := kiroTestConcatFrames(
		kiroTestEventFrame("assistantResponseEvent", `{"content":"Hello"}`),
		kiroTestEventFrame("metadataEvent", `{"stopReason":"end_turn"}`),
		kiroTestEventFrame("meteringEvent", `{"unit":"credit","usage":1.5,"cacheReadInputTokens":137,"cacheCreationInputTokens":9}`),
	)

	srv, calls := kiroTestFakeUpstream(t, func(int) (int, []byte) {
		return http.StatusOK, frames
	})

	svc := &KiroGatewayService{}
	svc.callEndpointOverride = kiroTestOverrideCallingServer(srv)

	account := kiroTestOAuthAccount(1)
	recorder, c := kiroTestContext()

	result, err := svc.ForwardUpstream(context.Background(), c, account, []byte(kiroTestRequestBody))
	require.NoError(t, err)
	require.NotNil(t, result)
	require.EqualValues(t, 1, atomic.LoadInt32(calls))

	require.Equal(t, 137, result.Usage.CacheReadInputTokens, "cache token 必须是 meteringEvent 的真实值")
	require.Equal(t, 9, result.Usage.CacheCreationInputTokens)
	require.Positive(t, result.Usage.OutputTokens)

	out := recorder.Body.String()
	require.Contains(t, out, "event: message_start")
	require.Contains(t, out, "event: content_block_delta")
	require.Contains(t, out, "text_delta")
	require.Contains(t, out, "Hello")
	require.Contains(t, out, "event: message_stop")
}

// TestKiroForwardUpstreamFirstEndpoint429ThenSecondSucceeds 覆盖端点级重试：
// 第一个端点 429，第二个端点成功，客户端完全无感（看不到第一次失败的任何痕迹）。
func TestKiroForwardUpstreamFirstEndpoint429ThenSecondSucceeds(t *testing.T) {
	successFrames := kiroTestConcatFrames(
		kiroTestEventFrame("assistantResponseEvent", `{"content":"ok"}`),
	)

	srv, calls := kiroTestFakeUpstream(t, func(idx int) (int, []byte) {
		if idx == 0 {
			return http.StatusTooManyRequests, []byte(`{"message":"rate limited, try again later"}`)
		}
		return http.StatusOK, successFrames
	})

	svc := &KiroGatewayService{}
	svc.callEndpointOverride = kiroTestOverrideCallingServer(srv)

	account := kiroTestOAuthAccount(2)
	recorder, c := kiroTestContext()

	result, err := svc.ForwardUpstream(context.Background(), c, account, []byte(kiroTestRequestBody))
	require.NoError(t, err)
	require.NotNil(t, result)
	require.EqualValues(t, 2, atomic.LoadInt32(calls), "第一个端点 429 后必须换第二个端点重试")

	out := recorder.Body.String()
	require.NotContains(t, out, "rate limited", "客户端不应看到第一个端点失败的任何痕迹")
	require.Contains(t, out, "event: message_start")
	require.Contains(t, out, "event: message_stop")
}

// TestKiroForwardUpstreamBadRequestNeverRetriesOrFailsOver 是红线二的集成测试：
// 400 只应该发起一次上游调用，绝不重试也绝不换端点/换账号。
func TestKiroForwardUpstreamBadRequestNeverRetriesOrFailsOver(t *testing.T) {
	srv, calls := kiroTestFakeUpstream(t, func(int) (int, []byte) {
		return http.StatusBadRequest, []byte(`{"message":"malformed schema"}`)
	})

	svc := &KiroGatewayService{}
	svc.callEndpointOverride = kiroTestOverrideCallingServer(srv)

	account := kiroTestOAuthAccount(3)
	_, c := kiroTestContext()

	result, err := svc.ForwardUpstream(context.Background(), c, account, []byte(kiroTestRequestBody))
	require.Error(t, err)
	require.Nil(t, result)

	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Equal(t, NextAccountStop, failoverErr.NextAccountAction, "400 必须显式挡住失败转移")
	require.EqualValues(t, 1, atomic.LoadInt32(calls), "400 不应重试也不应换端点")
}

// TestKiroForwardUpstreamInvalidModelIDExhaustsEndpointsWithoutBlockingAccount
// 是红线一的集成测试：INVALID_MODEL_ID 在三个端点全部复现后返回错误，
// 但账号绝不能被标记为故障——这是 decideKiroAction 两条红线里最容易被
// 破坏的一条，必须做到断言严丝合缝。
func TestKiroForwardUpstreamInvalidModelIDExhaustsEndpointsWithoutBlockingAccount(t *testing.T) {
	srv, calls := kiroTestFakeUpstream(t, func(int) (int, []byte) {
		return http.StatusBadRequest, []byte(`{"message":"INVALID_MODEL_ID: model not available in this region"}`)
	})

	svc := &KiroGatewayService{}
	svc.callEndpointOverride = kiroTestOverrideCallingServer(srv)

	repo := &kiroRateLimitRecorder{}
	svc.accountRepo = repo

	account := kiroTestOAuthAccount(4)
	_, c := kiroTestContext()

	result, err := svc.ForwardUpstream(context.Background(), c, account, []byte(kiroTestRequestBody))
	require.Error(t, err)
	require.Nil(t, result)

	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Equal(t, NextAccountStop, failoverErr.NextAccountAction,
		"红线一：网络/区域问题（INVALID_MODEL_ID）绝不能触发账号转移")

	require.EqualValues(t, 3, atomic.LoadInt32(calls), "OAuth 账号应轮完全部 3 个端点后才中止")

	require.False(t, repo.called(), "红线一：INVALID_MODEL_ID 绝不能禁用账号")
}

// --- Fix Round 1：creditsExhaustedCooldownUntil 惊群修复的验证 ---
//
// review 指出的 Important 发现：原实现在每一次触发 SignalCreditsExhausted
// 的失败请求上都独立发起一次现场 getUsageLimits 查询，credits 真耗尽时
// 恰好是一批并发请求同时失败的时刻，会对上游造成惊群。修复分两层：
//  1. 短路层——account 本地副本上已有未过期的限流记录时直接复用；
//  2. singleflight 层——真正同一时刻并发的调用共享同一次现场查询。
//
// creditsExhaustedCooldownUntil 本身不写回 account.Extra（写回是调用方
// finishWithAction 在拿到返回值之后做的事），所以下面两个测试各自独立地
// 覆盖一层：短路测试预置好 account.Extra 再断言零上游调用；并发测试从
// 干净账号出发，只能靠 singleflight 收敛。

// kiroTestCreditsFakeUpstream 起一个假的 getUsageLimits 端点，返回一条已
// 耗尽且带真实 nextDateReset 的用量，并用 atomic 计数每次被命中的次数——
// 与 kiroTestFakeUpstream 同款风格。
func kiroTestCreditsFakeUpstream(t *testing.T, resetAt time.Time, delay time.Duration) (*httptest.Server, *int32) {
	t.Helper()
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		if delay > 0 {
			time.Sleep(delay)
		}
		_, _ = w.Write([]byte(`{
			"subscriptionInfo":{"subscriptionTitle":"KIRO PRO+"},
			"overageConfiguration":{"overageStatus":"ENABLED"},
			"usageBreakdownList":[{
				"resourceType":"AGENTIC_REQUEST",
				"currentUsage":1000,"usageLimit":1000,
				"nextDateReset":` + itoa(resetAt.Unix()) + `
			}]
		}`))
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

// TestKiroCreditsExhaustedCooldownShortCircuitsOnExistingRateLimit 覆盖层 1：
// account 本地副本上已经有一条还没过期的 kiroCreditsExhaustedKey 记录（模拟
// "另一个并发请求刚做过这次查询并写回结果"），creditsExhaustedCooldownUntil
// 必须直接复用它，一次都不打上游。
func TestKiroCreditsExhaustedCooldownShortCircuitsOnExistingRateLimit(t *testing.T) {
	future := time.Now().Add(2 * time.Hour)
	srv, hits := kiroTestCreditsFakeUpstream(t, future, 0)

	svc := &KiroGatewayService{}
	svc.creditsQuotaFetcherOverride = &KiroQuotaFetcher{qHostFor: func(*Account) string { return srv.URL }}

	account := kiroTestOAuthAccount(200)
	setAccountModelRateLimitSnapshot(account, kiroCreditsExhaustedKey, future, "kiro_credits_exhausted", time.Now())

	until := svc.creditsExhaustedCooldownUntil(context.Background(), account)
	require.WithinDuration(t, future, until, time.Second)
	require.EqualValues(t, 0, atomic.LoadInt32(hits), "本地已有未过期记录时不应发起任何现场查询")
}

// TestKiroCreditsExhaustedCooldownConcurrentCallsCollapseViaSingleflight 覆盖
// 层 2：从一个干净账号（没有预置限流记录，层 1 帮不上忙）出发，N 个 goroutine
// 用一个启动屏障尽量同时调用 creditsExhaustedCooldownUntil，断言真正打到上游
// 的次数远小于 goroutine 数——singleflight 应该把它们收敛到 1 次。
func TestKiroCreditsExhaustedCooldownConcurrentCallsCollapseViaSingleflight(t *testing.T) {
	reset := time.Now().Add(6 * time.Hour)
	// 给假上游一点人为延迟，扩大"多个 goroutine 同时处于 Do() 内部"的窗口，
	// 让并发收敛效果在没有屏障也能大概率复现；配合下面的启动屏障双重保险。
	srv, hits := kiroTestCreditsFakeUpstream(t, reset, 20*time.Millisecond)

	svc := &KiroGatewayService{}
	svc.creditsQuotaFetcherOverride = &KiroQuotaFetcher{qHostFor: func(*Account) string { return srv.URL }}

	account := kiroTestOAuthAccount(201)

	const goroutines = 30
	results := make([]time.Time, goroutines)

	start := make(chan struct{})
	var ready sync.WaitGroup
	var wg sync.WaitGroup
	ready.Add(goroutines)
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			ready.Done()
			<-start
			results[idx] = svc.creditsExhaustedCooldownUntil(context.Background(), account)
		}(i)
	}
	ready.Wait() // 等所有 goroutine 都已启动、卡在屏障前，最大化真正同时调用的概率
	close(start)
	wg.Wait()

	got := atomic.LoadInt32(hits)
	t.Logf("goroutines=%d upstream_hits=%d", goroutines, got)

	// 期望正好收敛到 1 次：30 个 goroutine 共享同一个启动屏障，几乎同时进入
	// creditsExhaustedCooldownUntil，account 又没有预置限流记录（层 1 不生效），
	// 应该全部落进 singleflight.Do 的同一个 flightKey，只有第一个真正发起
	// 请求、其余全部拿到共享结果。留 <=2 的容差是为了容住"个别 goroutine
	// 被调度延迟到第一次 Do() 已经返回之后才进入"这种小概率时序窗口——
	// 出现这种情况时 singleflight 无法再合并，会诚实地发起第二次查询，这
	// 不代表去重失败，只是同一时刻并发窗口没有覆盖到全部 goroutine。
	require.LessOrEqual(t, got, int32(2), "singleflight 应该把 %d 个并发调用收敛到至多 1-2 次现场查询", goroutines)
	require.GreaterOrEqual(t, got, int32(1), "至少要真正发起一次查询，不能全部退回 fallback")

	for i, until := range results {
		require.WithinDuration(t, reset, until, 2*time.Second, "goroutine %d 应该拿到真实 nextDateReset，而不是 fallback 冷却", i)
	}
}

// --- I1 + I2：streamToClient 流内 exception 帧处理的回归 ---
//
// kiroTestExceptionFrame 构造一个 AWS event-stream 异常帧，字段命名/取值风格
// 与 internal/pkg/kiro/stream_test.go 里 TestStreamExceptionFrameReturnsError /
// TestStreamFeedReturnsPartialOutputAlongsideException 用的 buildFrame 调用
// 完全一致（那两个测试属于 package kiro，本文件在 package service，无法直接
// 复用，只能照抄同一套 header 组合）。
func kiroTestExceptionFrame(exceptionType, message string) []byte {
	return kiroTestBuildFrame([][2]string{
		{":message-type", "exception"},
		{":exception-type", exceptionType},
	}, []byte(`{"message":"`+message+`"}`))
}

// TestKiroForwardUpstreamPartialContentBeforeInStreamExceptionIsWrittenToClient
// 是 I2 的回归：同一次 Feed 调用里，正文帧先于 exception 帧到达时，正文帧产出
// 的合法事件（message_start/content_block_start/content_block_delta）必须先
// 写给客户端，不能被"先检查 tErr 再决定要不要写"的旧顺序悄悄吞掉。
//
// 场景设计：文本帧让 translator.SawContent() 变为 true，之后紧跟的 exception
// 帧因此走"已出内容只能截断"分支——优雅结束（Finalize 补发收尾事件），
// ForwardUpstream 返回成功而不是错误。这正是 bug 最隐蔽的地方：即便调用方后
// 续把 Finalize 的收尾事件正常写出，如果开头的事件在 tErr!=nil 分支里被跳过，
// 客户端看到的就是一条缺开头、有结尾的畸形 SSE 流。
func TestKiroForwardUpstreamPartialContentBeforeInStreamExceptionIsWrittenToClient(t *testing.T) {
	combined := kiroTestConcatFrames(
		kiroTestEventFrame("assistantResponseEvent", `{"content":"partial output"}`),
		kiroTestExceptionFrame("ThrottlingException", "slow down"),
	)

	srv, calls := kiroTestFakeUpstream(t, func(int) (int, []byte) {
		return http.StatusOK, combined
	})

	svc := &KiroGatewayService{}
	svc.callEndpointOverride = kiroTestOverrideCallingServer(srv)

	account := kiroTestOAuthAccount(300)
	recorder, c := kiroTestContext()

	result, err := svc.ForwardUpstream(context.Background(), c, account, []byte(kiroTestRequestBody))
	require.NoError(t, err, "已经吐出内容后 exception 帧只能优雅截断，不能让 ForwardUpstream 整体报错")
	require.NotNil(t, result)
	require.EqualValues(t, 1, atomic.LoadInt32(calls))

	out := recorder.Body.String()
	require.Contains(t, out, "event: message_start",
		"I2 回归核心断言：正文帧产出的 message_start 必须先于 exception 处理被写出，"+
			"旧实现会因为先检查 tErr 而把它连同 content_block_start/delta 一起吞掉")
	require.Contains(t, out, "event: content_block_start")
	require.Contains(t, out, "text_delta")
	require.Contains(t, out, "partial output")
	require.Contains(t, out, "event: content_block_stop")
	require.Contains(t, out, "event: message_delta")
	require.Contains(t, out, "event: message_stop")
	require.NotContains(t, out, "slow down", "exception 帧本身的内容不应该被当成正文写给客户端")
}

// TestKiroForwardUpstreamInStreamExceptionWithoutPriorContentRoutesThroughDecideKiroAction
// 是 I1 的回归：流内 exception 帧在还没有吐出任何内容时必须经过
// decideKiroAction/finishWithAction 分类，而不是把裸错误直接原样返回给
// ForwardUpstream 的调用方——修复前调用方拿到的只是一个普通 error，永远
// 触发不了失败转移/账号冷却，鉴权失效/限流/额度耗尽这些信号全部失效。
//
// 帧序列：metadataEvent（"valid" 但不产出任何 SSE 事件、也不置位
// SawContent 的帧）后紧跟一个 ThrottlingException 异常帧。ThrottlingException
// 经 kiro.ClassifyUpstreamError 归类为 SignalRateLimited；decideKiroAction
// 对 (SignalRateLimited, sawContent=false, alreadyRefreshed=false,
// hasMoreEndpoints=false) 的判定是 kiroActionFailoverAccount（hasMoreEndpoints
// 在流内阶段恒为 false——已经成功建立连接，没有"换端点"这个选项）。
func TestKiroForwardUpstreamInStreamExceptionWithoutPriorContentRoutesThroughDecideKiroAction(t *testing.T) {
	combined := kiroTestConcatFrames(
		kiroTestEventFrame("metadataEvent", `{"stopReason":"end_turn"}`),
		kiroTestExceptionFrame("ThrottlingException", "slow down"),
	)

	srv, calls := kiroTestFakeUpstream(t, func(int) (int, []byte) {
		return http.StatusOK, combined
	})

	svc := &KiroGatewayService{}
	svc.callEndpointOverride = kiroTestOverrideCallingServer(srv)
	repo := &kiroRateLimitRecorder{}
	svc.accountRepo = repo

	account := kiroTestOAuthAccount(301)
	recorder, c := kiroTestContext()

	result, err := svc.ForwardUpstream(context.Background(), c, account, []byte(kiroTestRequestBody))
	require.Error(t, err, "还没吐出任何内容时，流内 exception 帧必须被当作一次可分类的失败处理")
	require.Nil(t, result)
	require.EqualValues(t, 1, atomic.LoadInt32(calls))

	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr,
		"I1 回归核心断言：必须是已分类的 *UpstreamFailoverError，不能是裸的 *kiro.UpstreamError 或其它包装错误")
	require.Equal(t, GatewayFailureReason("kiro_rate_limited"), failoverErr.Reason)
	require.Equal(t, NextAccountLegacyRetry, failoverErr.NextAccountAction,
		"SignalRateLimited 走 FailoverAccount，必须允许换账号重试，不能被误判成 Abort")

	require.True(t, repo.called(), "分类后的 SignalRateLimited 必须触发账号调度冷却（写 model_rate_limits）")
	require.Equal(t, kiroCreditsExhaustedKey, repo.calls[0].scope,
		"kiro 的调度冷却统一走 KiroCredits 这一个 key，不分模型，见 model_rate_limit.go 的 PlatformKiro case")
	require.Equal(t, account.ID, repo.calls[0].accountID)
	require.WithinDuration(t, time.Now().Add(kiroRateLimitedExhaustedCooldown), repo.calls[0].resetAt, 5*time.Second)

	require.Empty(t, recorder.Body.String(), "分类失败必须在写出任何 SSE 事件之前发生——metadataEvent 本身不产出事件")
}

// --- TestConnection：管理端"测试连接"功能 ---
//
// 真实账号联调发现的回归：AccountTestService 里从来没有 Kiro 分支，任何
// Kiro 账号点"测试连接"都会落进通用的 testClaudeAccountConnection，报
// "No API key available"（Kiro 账号创建时 Type 统一填的是 apikey，不管实际
// auth_method）——KiroGatewayService.TestConnection 就是补上的真实实现，
// 完整复用 ForwardUpstream，不是另起一套简化探测。

func TestKiroTestConnectionReturnsResponseText(t *testing.T) {
	frames := kiroTestConcatFrames(
		kiroTestEventFrame("assistantResponseEvent", `{"content":"OK"}`),
		kiroTestEventFrame("metadataEvent", `{"stopReason":"end_turn"}`),
	)

	srv, calls := kiroTestFakeUpstream(t, func(int) (int, []byte) {
		return http.StatusOK, frames
	})

	svc := &KiroGatewayService{}
	svc.callEndpointOverride = kiroTestOverrideCallingServer(srv)

	account := kiroTestOAuthAccount(500)

	result, err := svc.TestConnection(context.Background(), account, "")
	require.NoError(t, err)
	require.Equal(t, "OK", result.Text)
	require.EqualValues(t, 1, atomic.LoadInt32(calls))
}

func TestKiroTestConnectionSurfacesUpstreamFailure(t *testing.T) {
	srv, _ := kiroTestFakeUpstream(t, func(int) (int, []byte) {
		return http.StatusBadRequest, []byte(`{"message":"malformed schema"}`)
	})

	svc := &KiroGatewayService{}
	svc.callEndpointOverride = kiroTestOverrideCallingServer(srv)

	account := kiroTestOAuthAccount(501)

	_, err := svc.TestConnection(context.Background(), account, "")
	require.Error(t, err, "上游拒绝时测试连接必须报错，不能悄悄返回空文本假装成功")
}

// TestKiroTestConnectionBypassesWhitelistAndAsksRealUpstream 是用户明确
// 要求的行为，也是对 ForwardUpstream 白名单拒绝（见
// TestKiroForwardUpstreamRejectsUnsupportedModel）的刻意不对称：测试连接
// 是管理员主动发起的一次性诊断调用，代价小（这里固定 max_tokens:64 的短
// 请求），完全负担得起直接问 Kiro 真实上游"这个模型到底支不支持"，不需要
// 先过本地白名单再猜——本地白名单已经两次证明会猜错（一次误收
// claude-fable-5、一次误拒 claude-sonnet-5，方向相反），能直接问上游拿
// 真实答案的场合就不该猜。这里断言一个本地白名单里没有的模型名确实会被
// 转发到上游（不会被拦下来），且上游的真实响应（这里模拟一次成功）会
// 原样返回给调用方。
func TestKiroTestConnectionBypassesWhitelistAndAsksRealUpstream(t *testing.T) {
	frames := kiroTestConcatFrames(
		kiroTestEventFrame("assistantResponseEvent", `{"content":"OK"}`),
		kiroTestEventFrame("metadataEvent", `{"stopReason":"end_turn"}`),
	)
	srv, calls := kiroTestFakeUpstream(t, func(int) (int, []byte) {
		return http.StatusOK, frames
	})

	svc := &KiroGatewayService{}
	svc.callEndpointOverride = kiroTestOverrideCallingServer(srv)

	account := kiroTestOAuthAccount(504)

	result, err := svc.TestConnection(context.Background(), account, "claude-sonnet-5")
	require.NoError(t, err, "测试连接不应该在本地拦截白名单没有的模型名")
	require.Equal(t, "OK", result.Text)
	require.EqualValues(t, 1, atomic.LoadInt32(calls), "必须真的转发到上游，不能在本地被拦下来")
}

// TestKiroTestConnectionSurfacesRealUpstreamRejectionForUnsupportedModel
// 覆盖上游真的拒绝的情况（真实场景：claude-fable-5 被 Kiro 判定
// INVALID_MODEL_ID）——测试连接必须把上游的真实拒绝原样报错给调用方，
// 而不是显示"完成"。INVALID_MODEL_ID 命中 classifyMarkers 归类成
// SignalNetworkRegion（"红线2"：网络/区域问题不能归咎账号），
// decideKiroAction 会在有更多端点时换端点重试，所以这里会看到 3 次调用
// （kiroTestOAuthAccount 是非 API Key 账号，对应 3 个端点）而不是 1 次——
// 与 TestKiroForwardUpstreamInvalidModelIDExhaustsEndpointsWithoutBlockingAccount
// 覆盖的是同一条不变式。
func TestKiroTestConnectionSurfacesRealUpstreamRejectionForUnsupportedModel(t *testing.T) {
	srv, calls := kiroTestFakeUpstream(t, func(int) (int, []byte) {
		return http.StatusBadRequest, []byte(`{"message":"Invalid model. Please select a different model to continue.","reason":"INVALID_MODEL_ID"}`)
	})

	svc := &KiroGatewayService{}
	svc.callEndpointOverride = kiroTestOverrideCallingServer(srv)

	account := kiroTestOAuthAccount(505)

	_, err := svc.TestConnection(context.Background(), account, "claude-fable-5")
	require.Error(t, err, "上游真实拒绝时必须报错，不能显示成功")
	require.EqualValues(t, 3, atomic.LoadInt32(calls), "INVALID_MODEL_ID 必须换遍所有端点重试，不归咎账号")
}

// TestKiroForwardUpstreamRejectsUnsupportedModel 是用户真实账号测试报告的
// 一系列回归收敛后的最终行为（完整历史见 models_test.go 的
// TestMapModelOpusFamilyRequiresRealVerification）：先是任何未识别模型名
// 都被静默换成 sonnet-4.6，收窄成白名单拒绝后又先后漏收、错收过几个
// 模型（opus-5 曾被误拒；fable-5 被基于第三方参考实现错误加入后又被真实
// 测试证伪移除）。
//
// 设计参照 AntigravityGatewayService 的既有约定（一份尽量准确的白名单，
// 命中就映射、未命中就干净拒绝，不转发不确定的请求去问上游）：这里断言
// 一个白名单确认不支持的模型名（claude-fable-5-2，一个假设的未来次版本
// 号）会在到达上游之前就被拒绝，不浪费一次上游调用。
func TestKiroForwardUpstreamRejectsUnsupportedModel(t *testing.T) {
	srv, calls := kiroTestFakeUpstream(t, func(int) (int, []byte) {
		t.Fatal("不支持的模型必须在到达上游之前就被拒绝")
		return 0, nil
	})

	svc := &KiroGatewayService{}
	svc.callEndpointOverride = kiroTestOverrideCallingServer(srv)

	account := kiroTestOAuthAccount(502)
	recorder, c := kiroTestContext()

	body := []byte(`{"model":"claude-fable-5-2","max_tokens":100,"messages":[{"role":"user","content":"hi"}],"stream":true}`)
	result, err := svc.ForwardUpstream(context.Background(), c, account, body)
	require.Error(t, err)
	require.Nil(t, result)
	require.EqualValues(t, 0, atomic.LoadInt32(calls))

	require.Equal(t, http.StatusForbidden, recorder.Code)
	require.Contains(t, recorder.Body.String(), "permission_error")
	require.Contains(t, recorder.Body.String(), "claude-fable-5-2")
}

// TestKiroForwardUpstreamAccountModelRestrictionUnconfiguredIsNoop 是"可选"
// 这个要求的核心断言：账号没有配置 credentials["model_mapping"] 时（本文件
// kiroTestOAuthAccount 构造的所有账号都是这个状态），新加的账号级限制分支
// 必须完全不改变行为——不能因为加了这个可选功能就让任何既有账号突然被
// 挡住。复用 kiroTestRequestBody 的连字符+日期后缀模型名，与其余全部既有
// 测试共享同一条基线请求体。
func TestKiroForwardUpstreamAccountModelRestrictionUnconfiguredIsNoop(t *testing.T) {
	frames := kiroTestConcatFrames(
		kiroTestEventFrame("assistantResponseEvent", `{"content":"ok"}`),
	)
	srv, calls := kiroTestFakeUpstream(t, func(int) (int, []byte) {
		return http.StatusOK, frames
	})

	svc := &KiroGatewayService{}
	svc.callEndpointOverride = kiroTestOverrideCallingServer(srv)

	account := kiroTestOAuthAccount(506)
	_, c := kiroTestContext()

	result, err := svc.ForwardUpstream(context.Background(), c, account, []byte(kiroTestRequestBody))
	require.NoError(t, err)
	require.NotNil(t, result)
	require.EqualValues(t, 1, atomic.LoadInt32(calls))
}

// TestKiroForwardUpstreamAccountModelRestrictionRejectsModelNotInWhitelist
// 覆盖白名单写法（前端 modelRestrictionMode='whitelist' 产出的 from==to）：
// 账号配置成只服务 claude-haiku-4.5，请求 claude-sonnet-4.6 必须被本地
// 拒绝，不浪费一次真实上游调用——与 TestKiroForwardUpstreamRejectsUnsupportedModel
// 覆盖的是同一条"不确定/不允许的模型不能打上游"红线，只是这次拒绝原因是
// 账号级限制而不是全局白名单。
func TestKiroForwardUpstreamAccountModelRestrictionRejectsModelNotInWhitelist(t *testing.T) {
	srv, calls := kiroTestFakeUpstream(t, func(int) (int, []byte) {
		t.Fatal("账号级限制之外的模型必须在到达上游之前就被拒绝")
		return 0, nil
	})

	svc := &KiroGatewayService{}
	svc.callEndpointOverride = kiroTestOverrideCallingServer(srv)

	account := kiroTestOAuthAccount(507)
	account.Credentials["model_mapping"] = map[string]any{
		"claude-haiku-4.5": "claude-haiku-4.5",
	}
	recorder, c := kiroTestContext()

	// kiroTestRequestBody 请求的是 claude-sonnet-4-5-20250929（映射到
	// claude-sonnet-4.5），不在账号只允许 haiku-4.5 的限制列表里。
	result, err := svc.ForwardUpstream(context.Background(), c, account, []byte(kiroTestRequestBody))
	require.Error(t, err)
	require.Nil(t, result)
	require.EqualValues(t, 0, atomic.LoadInt32(calls))

	require.Equal(t, http.StatusForbidden, recorder.Code)
	require.Contains(t, recorder.Body.String(), "permission_error")
}

// TestKiroForwardUpstreamAccountModelRestrictionAllowsWhitelistedModelDespiteHyphenForm
// 是本次实现前审出来的真实风险的回归测试：管理端模型选择器/预设映射填的
// 是 Kiro 规范点号形态（claude-sonnet-4.6），但真实客户端请求几乎总是
// 连字符+日期后缀形态。账号级限制必须按 kiro.MapModel 转换后的规范名去
// 匹配，而不是按客户端原始请求名——否则同一个被允许的模型只因客户端写法
// 不同就会被误拒。
func TestKiroForwardUpstreamAccountModelRestrictionAllowsWhitelistedModelDespiteHyphenForm(t *testing.T) {
	frames := kiroTestConcatFrames(
		kiroTestEventFrame("assistantResponseEvent", `{"content":"ok"}`),
	)
	srv, calls := kiroTestFakeUpstream(t, func(int) (int, []byte) {
		return http.StatusOK, frames
	})

	svc := &KiroGatewayService{}
	svc.callEndpointOverride = kiroTestOverrideCallingServer(srv)

	account := kiroTestOAuthAccount(508)
	account.Credentials["model_mapping"] = map[string]any{
		"claude-sonnet-4.5": "claude-sonnet-4.5",
	}
	_, c := kiroTestContext()

	// kiroTestRequestBody 请求的是连字符+日期后缀形态的
	// claude-sonnet-4-5-20250929，账号限制列表里写的是规范点号形态。
	result, err := svc.ForwardUpstream(context.Background(), c, account, []byte(kiroTestRequestBody))
	require.NoError(t, err, "账号限制列表用规范名写的模型，客户端用连字符形态请求时不应该被误拒")
	require.NotNil(t, result)
	require.EqualValues(t, 1, atomic.LoadInt32(calls))
}

// TestKiroForwardUpstreamAccountModelRestrictionMappingModeRemapsModel 覆盖
// 映射写法（from!=to）：账号把请求的 sonnet 模型强制改发成 opus-5——用于
// 管理员按账号做成本/质量调度的场景，与 Antigravity 的账号级映射是同一个
// 使用目的。
func TestKiroForwardUpstreamAccountModelRestrictionMappingModeRemapsModel(t *testing.T) {
	var gotBody []byte
	srv, calls := kiroTestFakeUpstream(t, func(int) (int, []byte) {
		return http.StatusOK, kiroTestConcatFrames(
			kiroTestEventFrame("assistantResponseEvent", `{"content":"ok"}`),
		)
	})
	// kiroTestFakeUpstream 不暴露请求体，另起一个中间层记录送到上游的原始
	// payload，验证映射结果（claude-opus-5）而不是原始请求模型（sonnet）
	// 真的被送了出去。
	svc := &KiroGatewayService{}
	svc.callEndpointOverride = func(ctx context.Context, account *Account, ep kiro.Endpoint, payload []byte) (*http.Response, error) {
		gotBody = payload
		return kiroTestOverrideCallingServer(srv)(ctx, account, ep, payload)
	}

	account := kiroTestOAuthAccount(509)
	account.Credentials["model_mapping"] = map[string]any{
		"claude-sonnet-4.5": "claude-opus-5",
	}
	_, c := kiroTestContext()

	result, err := svc.ForwardUpstream(context.Background(), c, account, []byte(kiroTestRequestBody))
	require.NoError(t, err)
	require.NotNil(t, result)
	require.EqualValues(t, 1, atomic.LoadInt32(calls))
	require.Contains(t, string(gotBody), "claude-opus-5", "映射模式必须真的把请求改发成映射目标模型")
	require.NotContains(t, string(gotBody), "claude-sonnet-4.5")
}

// TestKiroForwardUpstreamAccountModelRestrictionMappingModeTargetStillNeedsRealValidation
// 覆盖管理员配置错映射目标的情况：mapping 的 to 侧是自由文本，不保证是
// Kiro 真的认识的模型名，映射结果必须重新过 kiro.MapModel 白名单闸门——
// 不能因为账号级限制"匹配上了"就绕开"未经验证的模型名不能打真实流量"这条
// 红线（TestKiroForwardUpstreamRejectsUnsupportedModel 覆盖的同一条红线）。
func TestKiroForwardUpstreamAccountModelRestrictionMappingModeTargetStillNeedsRealValidation(t *testing.T) {
	srv, calls := kiroTestFakeUpstream(t, func(int) (int, []byte) {
		t.Fatal("映射目标不是已验证的 Kiro 模型名时，必须在到达上游之前就被拒绝")
		return 0, nil
	})

	svc := &KiroGatewayService{}
	svc.callEndpointOverride = kiroTestOverrideCallingServer(srv)

	account := kiroTestOAuthAccount(510)
	account.Credentials["model_mapping"] = map[string]any{
		"claude-sonnet-4.5": "claude-fable-5",
	}
	recorder, c := kiroTestContext()

	result, err := svc.ForwardUpstream(context.Background(), c, account, []byte(kiroTestRequestBody))
	require.Error(t, err)
	require.Nil(t, result)
	require.EqualValues(t, 0, atomic.LoadInt32(calls))
	require.Equal(t, http.StatusForbidden, recorder.Code)
}

// TestKiroTestConnectionBypassesAccountModelRestriction 覆盖 TestConnection
// 与账号级限制的交互：管理员主动发起的一次性诊断调用不应该被自己配置的
// 限制挡住——与它已经绕开全局白名单（TestKiroTestConnectionBypassesWhitelistAndAsksRealUpstream）
// 是同一条设计红线的延伸。
func TestKiroTestConnectionBypassesAccountModelRestriction(t *testing.T) {
	frames := kiroTestConcatFrames(
		kiroTestEventFrame("assistantResponseEvent", `{"content":"OK"}`),
		kiroTestEventFrame("metadataEvent", `{"stopReason":"end_turn"}`),
	)
	srv, calls := kiroTestFakeUpstream(t, func(int) (int, []byte) {
		return http.StatusOK, frames
	})

	svc := &KiroGatewayService{}
	svc.callEndpointOverride = kiroTestOverrideCallingServer(srv)

	account := kiroTestOAuthAccount(511)
	account.Credentials["model_mapping"] = map[string]any{
		"claude-haiku-4.5": "claude-haiku-4.5",
	}

	result, err := svc.TestConnection(context.Background(), account, "claude-sonnet-5")
	require.NoError(t, err, "测试连接不应该被账号自己配置的模型限制挡住")
	require.Equal(t, "OK", result.Text)
	require.EqualValues(t, 1, atomic.LoadInt32(calls))
}

// TestKiroForwardUpstreamRejectsEmptyModel 覆盖同一路径的空模型名情况：
// 连请求都构造不出来，同样不需要（也不应该）浪费一次上游调用。
func TestKiroForwardUpstreamRejectsEmptyModel(t *testing.T) {
	srv, calls := kiroTestFakeUpstream(t, func(int) (int, []byte) {
		t.Fatal("空模型名必须在到达上游之前就被拒绝")
		return 0, nil
	})

	svc := &KiroGatewayService{}
	svc.callEndpointOverride = kiroTestOverrideCallingServer(srv)

	account := kiroTestOAuthAccount(503)
	recorder, c := kiroTestContext()

	body := []byte(`{"model":"","max_tokens":100,"messages":[{"role":"user","content":"hi"}],"stream":true}`)
	result, err := svc.ForwardUpstream(context.Background(), c, account, body)
	require.Error(t, err)
	require.Nil(t, result)
	require.EqualValues(t, 0, atomic.LoadInt32(calls))

	require.Equal(t, http.StatusForbidden, recorder.Code)
	require.Contains(t, recorder.Body.String(), "permission_error")
}

// TestFinishWithActionAbortSuspendedWritesModelRateLimit 直接调用
// finishWithAction（不经过 ForwardUpstream 的三个真实调用点，它们目前都
// 恒以 sawContent=false 调 decideKiroAction，见 decideKiroAction 的文档：
// sawContent=true 时流式路径走的是优雅截断+成功返回，从不到达
// finishWithAction——TestKiroForwardUpstreamPartialContentBeforeInStreamExceptionIsWrittenToClient
// 覆盖的就是这条路径）。
//
// 这里验证的是 finishWithAction 自身对 (kiroActionAbort, SignalSuspended)
// 这个组合的正确性，不依赖当前调用方是否真的会产出这个组合——decideKiroAction
// 的不变式 1 明确允许 sawContent&&sig!=OK 走到 Abort，finishWithAction
// 必须对这个组合也做真实的账号冷却，而不是像修复前那样只调用一个对 kiro
// 恒为 no-op 的 AccountRuntimeBlocker。
func TestFinishWithActionAbortSuspendedWritesModelRateLimit(t *testing.T) {
	repo := &kiroRateLimitRecorder{}
	svc := &KiroGatewayService{accountRepo: repo}
	account := kiroTestOAuthAccount(600)

	err := svc.finishWithAction(context.Background(), account, kiroActionAbort, kiro.SignalSuspended, http.StatusForbidden, []byte(`{"message":"account is suspended"}`))
	require.Error(t, err)

	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Equal(t, NextAccountStop, failoverErr.NextAccountAction, "Abort 必须显式挡住失败转移")

	require.True(t, repo.called(), "Suspended 账号必须被真实冷却，不能只调用对 kiro 无效的 runtimeBlocker")
	require.Equal(t, kiroCreditsExhaustedKey, repo.calls[0].scope)
	require.Equal(t, account.ID, repo.calls[0].accountID)
	require.WithinDuration(t, time.Now().Add(kiroSuspendedCooldown), repo.calls[0].resetAt, 5*time.Second)
}

// TestFinishWithActionAbortOverageWritesModelRateLimit 覆盖同一组合的
// SignalOverage 变体——与 kiroActionFailoverAccount 分支把 Suspended/
// Overage 同等对待一致，Abort 分支不应该只处理 Suspended 漏掉 Overage。
func TestFinishWithActionAbortOverageWritesModelRateLimit(t *testing.T) {
	repo := &kiroRateLimitRecorder{}
	svc := &KiroGatewayService{accountRepo: repo}
	account := kiroTestOAuthAccount(601)

	err := svc.finishWithAction(context.Background(), account, kiroActionAbort, kiro.SignalOverage, http.StatusForbidden, []byte(`{"message":"overage limit reached"}`))
	require.Error(t, err)

	require.True(t, repo.called(), "Overage 账号必须被真实冷却，与 Suspended 同等对待")
	require.Equal(t, kiroCreditsExhaustedKey, repo.calls[0].scope)
}
