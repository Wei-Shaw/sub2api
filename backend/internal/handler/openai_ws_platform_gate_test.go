package handler

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	coderws "github.com/coder/websocket"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func closeStatusOf(t *testing.T, err error) coderws.StatusCode {
	t.Helper()
	var closeErr *service.OpenAIWSClientCloseError
	require.ErrorAs(t, err, &closeErr)
	return closeErr.StatusCode()
}

func TestOpenAIWSWaitDisabledBudgetRejectsImmediately(t *testing.T) {
	f := newOpenAIWSSessionWithOptions(t, openAIWSSessionOptions{mode: service.OpenAIWSIngressModeCtxPool, timeout: 3 * time.Second, skipDial: true})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	budget := &openAIWSAccountWaitBudget{turn: 1, waitDisabled: true}

	ok, err := f.cache.AcquireUserSlot(ctx, 1702, 1, "other-user")
	require.NoError(t, err)
	require.True(t, ok)
	started := time.Now()
	release, err := f.handler.acquireWSUserSlot(ctx, 1702, 1, 1802, budget, "", openAIWSAccountWaitPhaseInitial, nil, zap.NewNop())
	require.Nil(t, release)
	require.Equal(t, coderws.StatusTryAgainLater, closeStatusOf(t, err))
	require.Less(t, time.Since(started), time.Second, "用户槽不排队")

	ok, err = f.cache.AcquireAccountSlot(ctx, 801, 1, "other-account")
	require.NoError(t, err)
	require.True(t, ok)
	started = time.Now()
	accountRelease, err := f.handler.acquireWSAccountSlot(ctx, &service.Account{ID: 801, Platform: service.PlatformOpenAI}, 1,
		&service.AccountWaitPlan{Timeout: 3 * time.Second, MaxWaiting: 3}, budget, service.OpenAIWSIngressModeCtxPool, openAIWSAccountWaitPhaseInitial, time.Time{}, zap.NewNop())
	require.Nil(t, accountRelease)
	require.Equal(t, coderws.StatusTryAgainLater, closeStatusOf(t, err))
	require.Less(t, time.Since(started), time.Second, "账号槽不排队")
}

func TestOpenAIWSNonOpenAIPlatformRejectsBusyUserSlotWithoutWaiting(t *testing.T) {
	f := newOpenAIWSSessionWithOptions(t, openAIWSSessionOptions{mode: service.OpenAIWSIngressModeHTTPBridge, timeout: 3 * time.Second, platform: service.PlatformGrok})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ok, err := f.cache.AcquireUserSlot(ctx, 1702, 1, "other-user")
	require.NoError(t, err)
	require.True(t, ok)
	started := time.Now()
	require.NoError(t, f.conn.Write(ctx, coderws.MessageText, []byte(`{"type":"response.create","model":"grok-4.3","input":"first"}`)))
	_, _, err = f.conn.Read(ctx)
	require.Equal(t, coderws.StatusTryAgainLater, coderws.CloseStatus(err), "非 openai 平台用户槽满时照上游立即断开")
	require.Less(t, time.Since(started), time.Second)
	require.Empty(t, f.requests)
}

func TestOpenAIHTTPContinuationQueueOnlyForOpenAI(t *testing.T) {
	for _, tc := range []struct{ platform, model string }{{"", "gpt-5.1"}, {service.PlatformGrok, "grok-4.3"}} {
		name := tc.platform
		if name == "" {
			name = service.PlatformOpenAI
		}
		t.Run(name, func(t *testing.T) {
			f := newOpenAIWSSessionWithOptions(t, openAIWSSessionOptions{mode: service.OpenAIWSIngressModeCtxPool, timeout: 3 * time.Second, burstLimit: 2, bindings: &openAIWSStickyBindings{}, skipDial: true, platform: tc.platform})
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			defer cancel()
			post := func() <-chan error {
				done := make(chan error, 1)
				go func() {
					req, err := http.NewRequestWithContext(ctx, http.MethodPost, f.server.URL+"/v1/responses", strings.NewReader(`{"model":"`+tc.model+`","input":"hello","stream":true}`))
					if err != nil {
						done <- err
						return
					}
					req.Header.Set("Content-Type", "application/json")
					req.Header.Set("session_id", "bound")
					resp, err := http.DefaultClient.Do(req)
					if err == nil {
						_, err = io.Copy(io.Discard, resp.Body)
						_ = resp.Body.Close()
					}
					done <- err
				}()
				return done
			}
			require.NoError(t, <-post())
			<-f.requests
			require.Eventually(t, func() bool { return accountConcurrency(t, ctx, f.cache) == 0 }, time.Second, time.Millisecond)

			ok, err := f.cache.AcquireAccountSlot(ctx, 801, 1, "busy")
			require.NoError(t, err)
			require.True(t, ok)
			done := post()
			wantContinuation := name == service.PlatformOpenAI
			require.Eventually(t, func() bool {
				legacy, _ := f.cache.GetAccountWaitingCount(ctx, 801)
				continuation, _ := f.cache.GetAccountContinuationWaitingCount(ctx, 801)
				if wantContinuation {
					return continuation == 1 && legacy == 0
				}
				return legacy == 1 && continuation == 0
			}, 2*time.Second, time.Millisecond, "openai 续聊进续聊队列，其余平台照上游进普通队列")
			require.NoError(t, f.cache.ReleaseAccountSlot(ctx, 801, "busy"))
			require.NoError(t, <-done)
			<-f.requests
		})
	}
}

func TestOpenAIWSNonOpenAIPlatformReleasesSlotEachTurn(t *testing.T) {
	f := newOpenAIWSSessionWithOptions(t, openAIWSSessionOptions{mode: service.OpenAIWSIngressModeHTTPBridge, timeout: 3 * time.Second, holdSeconds: 5, platform: service.PlatformGrok})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, f.conn.Write(ctx, coderws.MessageText, []byte(`{"type":"response.create","model":"grok-4.3","input":"first"}`)))
	_, body, err := f.conn.Read(ctx)
	require.NoError(t, err)
	require.Contains(t, string(body), "response.completed")
	<-f.requests
	require.Eventually(t, func() bool { return accountConcurrency(t, ctx, f.cache) == 0 }, time.Second, 10*time.Millisecond, "非 openai 平台不做轮间保槽")
}
