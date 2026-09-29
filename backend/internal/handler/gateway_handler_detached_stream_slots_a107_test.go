//go:build unit

package handler

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	middleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// a107ConcurrencyCache 记录用户/账号槽位释放次数。
type a107ConcurrencyCache struct {
	fakeConcurrencyCache
	accountReleases atomic.Int64
	userReleases    atomic.Int64
}

func (f *a107ConcurrencyCache) ReleaseAccountSlot(context.Context, int64, string) error {
	f.accountReleases.Add(1)
	return nil
}

func (f *a107ConcurrencyCache) ReleaseUserSlot(context.Context, int64, string) error {
	f.userReleases.Add(1)
	return nil
}

// a107PipeUpstream 返回 200 SSE，响应体由测试通过 io.Pipe 控制何时结束。
type a107PipeUpstream struct {
	body   *io.PipeReader
	called chan struct{}
	calls  atomic.Int64
}

func (u *a107PipeUpstream) respond() *http.Response {
	if u.calls.Add(1) == 1 {
		close(u.called)
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       u.body,
	}
}

func (u *a107PipeUpstream) Do(*http.Request, string, int64, int) (*http.Response, error) {
	return u.respond(), nil
}

func (u *a107PipeUpstream) DoWithTLS(*http.Request, string, int64, int, *tlsfingerprint.Profile) (*http.Response, error) {
	return u.respond(), nil
}

type a107Env struct {
	h      *GatewayHandler
	cache  *a107ConcurrencyCache
	c      *gin.Context
	rec    *httptest.ResponseRecorder
	cancel context.CancelFunc
}

func a107NewEnv(t *testing.T, upstream service.HTTPUpstream, path string, body []byte, umqMode string) *a107Env {
	t.Helper()
	gin.SetMode(gin.TestMode)

	groupID := int64(10701)
	accountID := int64(10702)
	group := &service.Group{ID: groupID, Hydrated: true, Platform: service.PlatformAnthropic, Status: service.StatusActive}
	extra := map[string]any{}
	if umqMode != "" {
		extra["user_msg_queue_mode"] = umqMode
	}
	account := &service.Account{
		ID:            accountID,
		Name:          "oauth-a107",
		Platform:      service.PlatformAnthropic,
		Type:          service.AccountTypeOAuth,
		Credentials:   map[string]any{"access_token": "tok_a107"},
		Extra:         extra,
		Concurrency:   1,
		Priority:      1,
		Status:        service.StatusActive,
		Schedulable:   true,
		AccountGroups: []service.AccountGroup{{AccountID: accountID, GroupID: groupID}},
	}

	cfg := &config.Config{RunMode: config.RunModeSimple}
	cfg.Gateway.Scheduling.LoadBatchEnabled = false
	cfg.Gateway.UserMessageQueue.WaitTimeoutMs = 5000
	cfg.Gateway.UserMessageQueue.MinDelayMs = 5000
	cfg.Gateway.UserMessageQueue.MaxDelayMs = 5000

	cache := &a107ConcurrencyCache{}
	concurrencySvc := service.NewConcurrencyService(cache)
	schedulerSnapshot := service.NewSchedulerSnapshotService(&fakeSchedulerCache{accounts: []*service.Account{account}}, nil, nil, nil, nil)
	gwSvc := service.NewGatewayService(
		nil, &fakeGroupRepo{group: group}, nil, nil, nil, nil, nil, nil, cfg,
		schedulerSnapshot, concurrencySvc, nil, nil, nil, nil, upstream, nil, nil,
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
	)
	billingCacheSvc := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(billingCacheSvc.Stop)

	h := &GatewayHandler{
		gatewayService:      gwSvc,
		billingCacheService: billingCacheSvc,
		concurrencyHelper:   NewConcurrencyHelper(concurrencySvc, SSEPingFormatClaude, 0),
		maxAccountSwitches:  1,
		cfg:                 cfg,
	}
	if umqMode != "" {
		umqSvc := service.NewUserMessageQueueService(a106HeldLockUMQCache{}, nil, &cfg.Gateway.UserMessageQueue)
		h.userMsgQueueHelper = NewUserMsgQueueHelper(umqSvc, SSEPingFormatClaude, time.Hour)
	}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	ctx, cancel := context.WithCancel(context.WithValue(context.Background(), ctxkey.Group, group))
	t.Cleanup(cancel)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	apiKey := &service.APIKey{
		ID: 10703, UserID: 10704, GroupID: &groupID, Group: group, Status: service.StatusActive,
		User: &service.User{ID: 10704, Concurrency: 10, Balance: 100},
	}
	c.Set(string(middleware.ContextKeyAPIKey), apiKey)
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: apiKey.UserID, Concurrency: 10})

	return &a107Env{h: h, cache: cache, c: c, rec: rec, cancel: cancel}
}

const a107SSEHead = "event: message_start\n" +
	`data: {"type":"message_start","message":{"id":"msg_a107","type":"message","role":"assistant","model":"claude-sonnet-4-5","content":[],"stop_reason":null,"usage":{"input_tokens":12,"output_tokens":1}}}` + "\n\n" +
	"event: content_block_start\n" +
	`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}` + "\n\n" +
	"event: content_block_delta\n" +
	`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hi"}}` + "\n\n"

const a107SSETail = "event: content_block_stop\n" +
	`data: {"type":"content_block_stop","index":0}` + "\n\n" +
	"event: message_delta\n" +
	`data: {"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":40}}` + "\n\n" +
	"event: message_stop\n" +
	`data: {"type":"message_stop"}` + "\n\n"

// 客户端断开后流式上游被分离继续排空（计费），期间用户/账号并发槽位必须保持占用，
// 直到上游结束、Forward 返回后才释放；否则 Redis 视其为空闲，并发上限被绕过。
func TestA107DetachedStreamHoldsSlotsUntilForwardReturns(t *testing.T) {
	cases := []struct {
		name  string
		path  string
		body  string
		serve func(h *GatewayHandler, c *gin.Context)
	}{
		{
			name:  "messages",
			path:  "/v1/messages",
			body:  `{"model":"claude-sonnet-4-5","max_tokens":256,"stream":true,"messages":[{"role":"user","content":"hello a107"}]}`,
			serve: func(h *GatewayHandler, c *gin.Context) { h.Messages(c) },
		},
		{
			name:  "chat_completions",
			path:  "/v1/chat/completions",
			body:  `{"model":"claude-sonnet-4-5","max_tokens":256,"stream":true,"messages":[{"role":"user","content":"hello a107"}]}`,
			serve: func(h *GatewayHandler, c *gin.Context) { h.ChatCompletions(c) },
		},
		{
			name:  "responses",
			path:  "/v1/responses",
			body:  `{"model":"claude-sonnet-4-5","max_output_tokens":256,"stream":true,"input":"hello a107"}`,
			serve: func(h *GatewayHandler, c *gin.Context) { h.Responses(c) },
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pr, pw := io.Pipe()
			defer func() { _ = pw.Close() }()
			upstream := &a107PipeUpstream{body: pr, called: make(chan struct{})}
			env := a107NewEnv(t, upstream, tc.path, []byte(tc.body), "")

			done := make(chan struct{})
			go func() {
				defer close(done)
				tc.serve(env.h, env.c)
			}()

			select {
			case <-upstream.called:
			case <-done:
				t.Fatalf("handler returned before reaching upstream (status=%d size=%d)", env.c.Writer.Status(), env.c.Writer.Size())
			case <-time.After(10 * time.Second):
				t.Fatal("upstream was never called")
			}
			_, err := io.WriteString(pw, a107SSEHead)
			require.NoError(t, err)

			// 模拟客户端断开：上游仍在生成（pipe 未关闭）。
			env.cancel()
			time.Sleep(200 * time.Millisecond)
			select {
			case <-done:
				t.Fatal("handler returned while the detached upstream stream was still open")
			default:
			}
			require.Zero(t, env.cache.accountReleases.Load(), "account slot released while the detached upstream stream is still running")
			require.Zero(t, env.cache.userReleases.Load(), "user slot released while the detached upstream stream is still running")

			_, _ = io.WriteString(pw, a107SSETail)
			require.NoError(t, pw.Close())
			select {
			case <-done:
			case <-time.After(10 * time.Second):
				t.Fatal("handler did not return after upstream stream ended")
			}
			require.Equal(t, int64(1), env.cache.accountReleases.Load(), "account slot must be released exactly once after Forward returns")
			require.Equal(t, int64(1), env.cache.userReleases.Load(), "user slot must be released exactly once after the handler returns")
		})
	}
}

// Forward 之前的提前返回（UMQ 排队期间客户端断开）仍须释放账号/用户槽位。
func TestA107EarlyReturnBeforeForwardStillReleasesSlots(t *testing.T) {
	upstream := &a107PipeUpstream{called: make(chan struct{})}
	body := `{"model":"claude-sonnet-4-5","max_tokens":256,"stream":true,"messages":[{"role":"user","content":"hello a107"}]}`
	env := a107NewEnv(t, upstream, "/v1/messages", []byte(body), config.UMQModeSerialize)

	time.AfterFunc(100*time.Millisecond, env.cancel)
	done := make(chan struct{})
	go func() {
		defer close(done)
		env.h.Messages(env.c)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Messages did not return")
	}
	require.Zero(t, upstream.calls.Load(), "request must not be forwarded")
	require.Equal(t, int64(1), env.cache.accountReleases.Load(), "account slot must be released on early return")
	require.Equal(t, int64(1), env.cache.userReleases.Load(), "user slot must be released on early return")
}

// 释放包装：Forward 前 context 取消仍即时释放；解除后只由显式调用释放，且至多一次。
func TestA107WrapReleaseOnDoneUntilDisarmed(t *testing.T) {
	t.Run("cancel before disarm releases early", func(t *testing.T) {
		var n atomic.Int64
		ctx, cancel := context.WithCancel(context.Background())
		release, disarm := wrapReleaseOnDoneDisarmable(ctx, func() { n.Add(1) })
		cancel()
		require.Eventually(t, func() bool { return n.Load() == 1 }, time.Second, 5*time.Millisecond)
		disarm()
		release()
		require.Equal(t, int64(1), n.Load())
	})
	t.Run("cancel after disarm waits for explicit release", func(t *testing.T) {
		var n atomic.Int64
		ctx, cancel := context.WithCancel(context.Background())
		release, disarm := wrapReleaseOnDoneDisarmable(ctx, func() { n.Add(1) })
		disarm()
		cancel()
		time.Sleep(50 * time.Millisecond)
		require.Zero(t, n.Load())
		release()
		release()
		require.Equal(t, int64(1), n.Load())
	})
	t.Run("nil release", func(t *testing.T) {
		release, disarm := wrapReleaseOnDoneDisarmable(context.Background(), nil)
		require.Nil(t, release)
		require.NotNil(t, disarm)
		disarm()
	})
}
