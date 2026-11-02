package handler

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/internal/testutil"
	coderws "github.com/coder/websocket"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// 场景用例：用真实 WS 会话与进程内上游桩，把准入优先级与轮间槽保留的设计决策逐条走一遍。

func scenarioSessionHeader(sessionID string) http.Header {
	header := http.Header{}
	header.Set("session_id", sessionID)
	return header
}

func scenarioWaitFinished(t *testing.T, ctx context.Context, f *openAIWSAccountWaitSession) {
	t.Helper()
	select {
	case <-f.finished:
	case <-ctx.Done():
		t.Fatal("handler did not exit")
	}
}

func scenarioBindingsAllPointTo(t *testing.T, bindings *openAIWSStickyBindings, accountID int64) {
	t.Helper()
	snapshot := bindings.snapshot()
	checked := 0
	for key, bound := range snapshot {
		// 响应 ID 到账号的映射（openai:response:*）记录的是产出该响应的账号，HTTP 的响应归属
		// （openai:http-response-owner:*）记录的是用户，两者都不随会话绑定迁移。
		if strings.HasPrefix(key, "openai:response:") || strings.HasPrefix(key, "openai:http-response-owner:") {
			continue
		}
		checked++
		require.Equal(t, accountID, bound, "binding %s", key)
	}
	require.NotZero(t, checked, "no session binding recorded")
}

// 显式会话标识的续聊命中绑定时，即使账号上有续聊等待者也不让出，直接拿空槽。
func TestScenario_ExplicitSessionContinuationKeepsPriority(t *testing.T) {
	bindings := &openAIWSStickyBindings{}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	first := newOpenAIWSSessionWithOptions(t, openAIWSSessionOptions{mode: service.OpenAIWSIngressModeCtxPool, timeout: 250 * time.Millisecond, bindings: bindings, dialHeader: scenarioSessionHeader("sess-priority")})
	completeOpenAIWSAccountWaitTurn(t, ctx, first)
	scenarioBindingsAllPointTo(t, bindings, 801)
	require.NoError(t, first.conn.Close(coderws.StatusNormalClosure, ""))
	scenarioWaitFinished(t, ctx, first)

	ok, err := first.cache.IncrementAccountContinuationWaitCount(ctx, 801, 3)
	require.NoError(t, err)
	require.True(t, ok)

	second := newOpenAIWSSessionWithOptions(t, openAIWSSessionOptions{mode: service.OpenAIWSIngressModeCtxPool, timeout: 250 * time.Millisecond, bindings: bindings, concurrency: first.cache, dialHeader: scenarioSessionHeader("sess-priority")})
	require.NoError(t, second.conn.Write(ctx, coderws.MessageText, []byte(`{"type":"response.create","model":"gpt-5.1","input":"first"}`)))
	_, body, err := second.conn.Read(ctx)
	require.NoError(t, err, "带显式会话标识的续聊命中绑定，有空槽时不让出")
	require.Equal(t, "response.completed", gjson.GetBytes(body, "type").String())
	<-second.requests
}

// 错误轮不挂起：上游失败的那一轮结束后账号槽立即释放，不等保留期。
func TestScenario_ErrorTurnReleasesSlotImmediately(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	f := newOpenAIWSSessionWithOptions(t, openAIWSSessionOptions{mode: service.OpenAIWSIngressModeHTTPBridge, timeout: 2 * time.Second, holdSeconds: 5})
	completeOpenAIWSTurnKeepingSlot(t, ctx, f, "first")
	require.Equal(t, 1, accountConcurrency(t, ctx, f.cache), "成功轮结束后槽被挂起保留")

	f.failUpstream.Store(true)
	require.NoError(t, f.conn.Write(ctx, coderws.MessageText, []byte(`{"type":"response.create","model":"gpt-5.1","input":"second"}`)))
	require.Eventually(t, func() bool { return accountConcurrency(t, ctx, f.cache) == 0 }, 2*time.Second, 10*time.Millisecond, "错误轮结束必须立即释放，不能等 5 秒保留期")
}

// 失租回退：保留期内槽成员被外部清掉，下一轮续租失败后走正常抢槽，不会卡住也不会双占。
func TestScenario_LostLeaseFallsBackToNormalAcquire(t *testing.T) {
	cache, redisServer := testutil.NewTestConcurrencyCacheWithRedis(t)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	f := newOpenAIWSSessionWithOptions(t, openAIWSSessionOptions{mode: service.OpenAIWSIngressModeCtxPool, timeout: 2 * time.Second, holdSeconds: 5, concurrency: cache})
	completeOpenAIWSTurnKeepingSlot(t, ctx, f, "first")
	require.Equal(t, 1, accountConcurrency(t, ctx, f.cache))

	require.True(t, redisServer.Del("concurrency:account:801"), "模拟保留期内槽成员被 TTL 清掉")

	completeOpenAIWSTurnKeepingSlot(t, ctx, f, "second")
	require.Equal(t, 1, accountConcurrency(t, ctx, f.cache), "失租后重抢一个新槽，且不会出现双占")
}

// 透传首帧带 previous_response_id 的连接被队满溢出到活绑定之外的账号时不可迁移：释放槽、不改绑定、1013。
// 溢出本身在同样设置的 ctx_pool 场景（TestScenario_SpillPreservesBindingInCtxPool）里得到证明，
// 本用例的 1013 因此只能来自不可迁移判定而非队满。
func TestScenario_PassthroughContinuationNotMigratableOnSpill(t *testing.T) {
	bindings := &openAIWSStickyBindings{}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	// 802 优先级更高，空闲时首个连接落到 802 并绑定。
	extra := []service.Account{{ID: 802, Name: "account-spill", Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey, Status: service.StatusActive, Schedulable: true, Concurrency: 1, Priority: -1}}

	first := newOpenAIWSSessionWithOptions(t, openAIWSSessionOptions{mode: service.OpenAIWSIngressModePassthrough, timeout: 250 * time.Millisecond, bindings: bindings, extraAccounts: extra, dialHeader: scenarioSessionHeader("sess-spill"), loadBatch: true})
	completeOpenAIWSAccountWaitTurnOn(t, ctx, first, 802)
	scenarioBindingsAllPointTo(t, bindings, 802)
	require.NoError(t, first.conn.Close(coderws.StatusNormalClosure, ""))
	scenarioWaitFinished(t, ctx, first)

	ok, err := first.cache.AcquireAccountSlot(ctx, 802, 1, "other-802")
	require.NoError(t, err)
	require.True(t, ok, "802 满槽")
	for range 2 {
		ok, err = first.cache.IncrementAccountContinuationWaitCount(ctx, 802, 2)
		require.NoError(t, err)
		require.True(t, ok, "802 的续聊队列排满，触发溢出")
	}

	second := newOpenAIWSSessionWithOptions(t, openAIWSSessionOptions{mode: service.OpenAIWSIngressModePassthrough, timeout: 250 * time.Millisecond, bindings: bindings, concurrency: first.cache, extraAccounts: extra, dialHeader: scenarioSessionHeader("sess-spill"), loadBatch: true})
	require.NoError(t, second.conn.Write(ctx, coderws.MessageText, []byte(`{"type":"response.create","model":"gpt-5.1","input":"first","previous_response_id":"resp_prev"}`)))
	_, _, err = second.conn.Read(ctx)
	require.Equal(t, coderws.StatusTryAgainLater, coderws.CloseStatus(err), "透传续聊帧被溢出到 801 后不可迁移，应快速失败")
	scenarioWaitFinished(t, ctx, second)
	scenarioBindingsAllPointTo(t, bindings, 802)
	require.Equal(t, 0, accountConcurrency(t, ctx, first.cache), "拒绝时释放刚抢到的 801 槽")
	select {
	case <-second.requests:
		t.Fatal("non-migratable frame reached upstream")
	default:
	}
}

// ctx_pool 模式同样的溢出可以迁移上下文，但绑定保留在原账号（Preserve）。
func TestScenario_SpillPreservesBindingInCtxPool(t *testing.T) {
	bindings := &openAIWSStickyBindings{}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	extra := []service.Account{{ID: 802, Name: "account-spill", Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey, Status: service.StatusActive, Schedulable: true, Concurrency: 1, Priority: -1}}

	first := newOpenAIWSSessionWithOptions(t, openAIWSSessionOptions{mode: service.OpenAIWSIngressModeCtxPool, timeout: 250 * time.Millisecond, bindings: bindings, extraAccounts: extra, dialHeader: scenarioSessionHeader("sess-preserve"), loadBatch: true})
	completeOpenAIWSAccountWaitTurnOn(t, ctx, first, 802)
	scenarioBindingsAllPointTo(t, bindings, 802)
	require.NoError(t, first.conn.Close(coderws.StatusNormalClosure, ""))
	scenarioWaitFinished(t, ctx, first)

	ok, err := first.cache.AcquireAccountSlot(ctx, 802, 1, "other-802")
	require.NoError(t, err)
	require.True(t, ok)
	for range 2 {
		ok, err = first.cache.IncrementAccountContinuationWaitCount(ctx, 802, 2)
		require.NoError(t, err)
		require.True(t, ok)
	}

	second := newOpenAIWSSessionWithOptions(t, openAIWSSessionOptions{mode: service.OpenAIWSIngressModeCtxPool, timeout: 250 * time.Millisecond, bindings: bindings, concurrency: first.cache, extraAccounts: extra, dialHeader: scenarioSessionHeader("sess-preserve"), loadBatch: true})
	require.NoError(t, second.conn.Write(ctx, coderws.MessageText, []byte(`{"type":"response.create","model":"gpt-5.1","input":"first"}`)))
	_, body, err := second.conn.Read(ctx)
	require.NoError(t, err, "ctx_pool 能重建上下文，溢出到 801 后正常完成")
	require.Equal(t, "response.completed", gjson.GetBytes(body, "type").String())
	<-second.requests
	require.Equal(t, 1, accountConcurrency(t, ctx, first.cache), "801 被溢出连接占用")
	scenarioBindingsAllPointTo(t, bindings, 802)
}

// failover 换号且新账号完成一轮后，绑定迁移到新账号（Migrate）。
func TestScenario_FailoverMigratesBinding(t *testing.T) {
	bindings := &openAIWSStickyBindings{}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var failing atomic.Bool
	var failingHits atomic.Int32
	shared := newOpenAIWSSessionWithOptions(t, openAIWSSessionOptions{mode: service.OpenAIWSIngressModeCtxPool, timeout: 250 * time.Millisecond, bindings: bindings, skipDial: true, loadBatch: true})
	flaky := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if failing.Load() {
			failingHits.Add(1)
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		shared.upstream.Config.Handler.ServeHTTP(w, r)
	}))
	t.Cleanup(flaky.Close)
	extra := []service.Account{{
		ID: 802, Name: "account-flaky", Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey, Status: service.StatusActive, Schedulable: true, Concurrency: 1, Priority: -1,
		Credentials: map[string]any{"api_key": "sk-test", "base_url": flaky.URL},
	}}

	first := newOpenAIWSSessionWithOptions(t, openAIWSSessionOptions{mode: service.OpenAIWSIngressModeCtxPool, timeout: 250 * time.Millisecond, bindings: bindings, concurrency: shared.cache, extraAccounts: extra, dialHeader: scenarioSessionHeader("sess-migrate"), loadBatch: true})
	require.NoError(t, first.conn.Write(ctx, coderws.MessageText, []byte(`{"type":"response.create","model":"gpt-5.1","input":"first"}`)))
	_, _, err := first.conn.Read(ctx)
	require.NoError(t, err)
	<-shared.requests
	scenarioBindingsAllPointTo(t, bindings, 802)
	require.NoError(t, first.conn.Close(coderws.StatusNormalClosure, ""))
	scenarioWaitFinished(t, ctx, first)

	failing.Store(true)
	second := newOpenAIWSSessionWithOptions(t, openAIWSSessionOptions{mode: service.OpenAIWSIngressModeCtxPool, timeout: 250 * time.Millisecond, bindings: bindings, concurrency: shared.cache, extraAccounts: extra, dialHeader: scenarioSessionHeader("sess-migrate"), loadBatch: true})
	require.NoError(t, second.conn.Write(ctx, coderws.MessageText, []byte(`{"type":"response.create","model":"gpt-5.1","input":"first"}`)))
	_, body, err := second.conn.Read(ctx)
	require.NoError(t, err, "802 故障后应 failover 到 801 完成")
	require.Equal(t, "response.completed", gjson.GetBytes(body, "type").String())
	<-second.requests
	require.Positive(t, failingHits.Load(), "确实先命中了故障账号")
	scenarioBindingsAllPointTo(t, bindings, 801)
}

// 后续轮刷新绑定 TTL：长连接第二轮起每轮都刷新一次。
func TestScenario_LaterTurnsRefreshBindingTTL(t *testing.T) {
	bindings := &openAIWSStickyBindings{}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	f := newOpenAIWSSessionWithOptions(t, openAIWSSessionOptions{mode: service.OpenAIWSIngressModeCtxPool, timeout: 250 * time.Millisecond, bindings: bindings, dialHeader: scenarioSessionHeader("sess-ttl")})
	completeOpenAIWSTurnKeepingSlot(t, ctx, f, "first")
	afterFirst := bindings.refreshCount()
	completeOpenAIWSTurnKeepingSlot(t, ctx, f, "second")
	require.Equal(t, afterFirst+1, bindings.refreshCount(), "第二轮刷新一次")
	completeOpenAIWSTurnKeepingSlot(t, ctx, f, "third")
	require.Equal(t, afterFirst+2, bindings.refreshCount(), "第三轮再刷新一次")
}

// HTTP 三个文本入口与 WS 同规则：显式标识的续聊不让出，内容回退哈希命中的共享绑定让出。
func TestScenario_HTTPEndpointsClassifyByExplicitSession(t *testing.T) {
	endpoints := []struct {
		name   string
		path   string
		body   string
		header string
	}{
		{"responses", "/v1/responses", `{"model":"gpt-5.1","input":"scenario"}`, "session_id"},
		{"chat_completions", "/v1/chat/completions", `{"model":"gpt-5.1","messages":[{"role":"user","content":"scenario"}]}`, "session_id"},
		{"messages", "/v1/messages", `{"model":"gpt-5.1","max_tokens":16,"messages":[{"role":"user","content":"scenario"}]}`, "X-Claude-Code-Session-Id"},
	}
	for _, endpoint := range endpoints {
		t.Run(endpoint.name, func(t *testing.T) {
			bindings := &openAIWSStickyBindings{}
			f := newOpenAIWSSessionWithOptions(t, openAIWSSessionOptions{mode: service.OpenAIWSIngressModeCtxPool, timeout: 250 * time.Millisecond, bindings: bindings, skipDial: true})
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			defer cancel()
			post := func(sessionID string) int {
				t.Helper()
				req, err := http.NewRequestWithContext(ctx, http.MethodPost, f.server.URL+endpoint.path, strings.NewReader(endpoint.body))
				require.NoError(t, err)
				req.Header.Set("Content-Type", "application/json")
				if sessionID != "" {
					req.Header.Set(endpoint.header, sessionID)
				}
				resp, err := http.DefaultClient.Do(req)
				require.NoError(t, err)
				defer func() { _ = resp.Body.Close() }()
				payload, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
				t.Logf("%s session=%q status=%d body=%s", endpoint.path, sessionID, resp.StatusCode, payload)
				return resp.StatusCode
			}
			drain := func() int {
				n := 0
				for {
					select {
					case <-f.requests:
						n++
					default:
						return n
					}
				}
			}

			require.NotEqual(t, http.StatusTooManyRequests, post("sess-http"), "显式标识首次请求建立绑定")
			require.NotEqual(t, http.StatusTooManyRequests, post(""), "无标识请求按内容回退哈希建立共享绑定")
			require.Eventually(t, func() bool { return accountConcurrency(t, ctx, f.cache) == 0 }, time.Second, 10*time.Millisecond)
			require.Equal(t, 2, drain(), "两次请求都到达上游")

			ok, err := f.cache.IncrementAccountContinuationWaitCount(ctx, 801, 3)
			require.NoError(t, err)
			require.True(t, ok)

			require.NotEqual(t, http.StatusTooManyRequests, post("sess-http"), "显式标识的续聊命中绑定不让出")
			require.Equal(t, 1, drain(), "续聊请求到达上游")
			require.Equal(t, http.StatusTooManyRequests, post(""), "无标识请求命中共享绑定，有续聊等待者时让出直到超时")
			require.Zero(t, drain(), "让出的请求不得到达上游")
		})
	}
}

// HTTP 续聊被队满溢出、在别的账号上排队获槽后，准入后不得改写原绑定（Preserve）：
// 否则一次短时突发就会把整段会话迁到冷缓存账号。
func TestScenario_HTTPSpillWaitPreservesBinding(t *testing.T) {
	bindings := &openAIWSStickyBindings{}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	// 802 优先级更高：溢出后第三层兜底按优先级把等待计划落到 802。
	extra := []service.Account{{ID: 802, Name: "account-spill", Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey, Status: service.StatusActive, Schedulable: true, Concurrency: 1, Priority: -1}}
	f := newOpenAIWSSessionWithOptions(t, openAIWSSessionOptions{mode: service.OpenAIWSIngressModeCtxPool, timeout: 2 * time.Second, bindings: bindings, extraAccounts: extra, skipDial: true, loadBatch: true})
	post := func() int {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, f.server.URL+"/v1/responses", strings.NewReader(`{"model":"gpt-5.1","input":"scenario"}`))
		if err != nil {
			return -1
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("session_id", "sess-http-spill")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return -1
		}
		defer func() { _ = resp.Body.Close() }()
		_, _ = io.Copy(io.Discard, resp.Body)
		return resp.StatusCode
	}

	ok, err := f.cache.AcquireAccountSlot(ctx, 802, 1, "other-802")
	require.NoError(t, err)
	require.True(t, ok, "802 满槽，首个请求落到 801 并绑定")
	require.NotEqual(t, http.StatusTooManyRequests, post())
	<-f.requests
	scenarioBindingsAllPointTo(t, bindings, 801)
	require.Eventually(t, func() bool { return accountConcurrency(t, ctx, f.cache) == 0 }, time.Second, 10*time.Millisecond)

	ok, err = f.cache.AcquireAccountSlot(ctx, 801, 1, "other-801")
	require.NoError(t, err)
	require.True(t, ok)
	for range 2 {
		ok, err = f.cache.IncrementAccountContinuationWaitCount(ctx, 801, 2)
		require.NoError(t, err)
		require.True(t, ok, "801 的续聊队列排满，触发溢出")
	}

	status := make(chan int, 1)
	go func() { status <- post() }()
	require.Eventually(t, func() bool {
		n, _ := f.cache.GetAccountWaitingCount(ctx, 802)
		return n > 0
	}, 2*time.Second, 5*time.Millisecond, "溢出后在 802 上排队")
	require.NoError(t, f.cache.ReleaseAccountSlot(ctx, 802, "other-802"))
	require.NotEqual(t, http.StatusTooManyRequests, <-status)
	<-f.requests
	scenarioBindingsAllPointTo(t, bindings, 801)
}

func completeOpenAIWSAccountWaitTurnOn(t *testing.T, ctx context.Context, f *openAIWSAccountWaitSession, accountID int64) {
	t.Helper()
	require.NoError(t, f.conn.Write(ctx, coderws.MessageText, []byte(`{"type":"response.create","model":"gpt-5.1","input":"first"}`)))
	_, _, err := f.conn.Read(ctx)
	require.NoError(t, err)
	<-f.requests
	require.Eventually(t, func() bool { n, _ := f.cache.GetAccountConcurrency(ctx, accountID); return n == 0 }, time.Second, time.Millisecond)
}
