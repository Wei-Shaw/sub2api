//go:build unit

package handler

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAPIKeyAdmissionRealForwardingHandlersRejectBeforeUpstream(t *testing.T) {
	upstream := &openAIResponsesFailoverCancelUpstream{}
	h := newOpenAIResponsesFailoverTestHandler(t, upstream)
	helper, _ := newAPIKeyAdmissionHelper(t)
	h.concurrencyHelper = helper
	ctx, cancel := service.WithAPIKeyAdmissionOwner(context.Background())
	defer cancel()
	release, err := helper.AcquireAPIKeySlot(ctx, 99, 1)
	require.NoError(t, err)
	defer release()
	for _, tc := range []struct {
		name string
		grok bool
		run  func(*gin.Context)
	}{
		{"input_tokens", false, h.ResponsesInputTokens},
		{"count_tokens", false, h.CountTokens},
		{"realtime", true, h.GrokRealtime},
		{"tts", true, func(c *gin.Context) { h.GrokVoice(c, "tts") }},
		{"stt", true, func(c *gin.Context) { h.GrokVoice(c, "stt") }},
		{"custom_voices", true, func(c *gin.Context) { h.GrokVoice(c, "custom-voices") }},
		{"codex_models", false, h.CodexModels},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, response := newOpenAIResponsesFailoverTestContext(t, ctx)
			key, _ := middleware2.GetAPIKeyFromContext(c)
			key.ConcurrencyLimit = 1
			key.Group.AllowMessagesDispatch = true
			if tc.grok {
				key.Group.Platform = service.PlatformGrok
			}
			c.Request.Body = io.NopCloser(strings.NewReader(`{"model":"gpt-5.1","input":"hello","messages":[{"role":"user","content":"hello"}]}`))
			c.Request.Header.Set("Connection", "Upgrade")
			c.Request.Header.Set("Upgrade", "websocket")
			tc.run(c)
			require.Equal(t, http.StatusTooManyRequests, response.Code, response.Body.String())
			require.Contains(t, response.Body.String(), "API key")
			require.Empty(t, upstream.calls())
		})
	}
}

func TestAPIKeyAdmissionPrecedesUserQueueAndSSE(t *testing.T) {
	helper, _ := newAPIKeyAdmissionHelper(t)
	helper.pingFormat = SSEPingFormatComment
	ctx, cancel := service.WithAPIKeyAdmissionOwner(context.Background())
	defer cancel()
	release, acquired, err := helper.TryAcquireUserSlotForAPIKey(ctx, 1, 1, 77, 1)
	require.NoError(t, err)
	require.True(t, acquired)
	defer release()
	c, recorder := newHelperTestContext(http.MethodPost, "/v1/responses")
	c.Request = c.Request.WithContext(ctx)
	started := false
	start := time.Now()
	next, err := helper.acquireUserSlotWithWaitTimeout(c, 1, 1, 77, 1, time.Second, true, &started)
	require.Nil(t, next)
	status, _, _, message := concurrencyErrorResponse(err, "user")
	require.Equal(t, http.StatusTooManyRequests, status)
	require.Contains(t, message, "API key")
	require.Less(t, time.Since(start), 500*time.Millisecond)
	require.False(t, started)
	require.Empty(t, recorder.Body.String(), "no heartbeat may commit HTTP 200 before admission")
}

func TestAPIKeyAdmissionCanceledUserWaitReturnsReservation(t *testing.T) {
	helper, cache := newAPIKeyAdmissionHelper(t)
	userRelease, acquired, err := helper.TryAcquireUserSlot(context.Background(), 1, 1)
	require.NoError(t, err)
	require.True(t, acquired)
	defer userRelease()
	parent, cancelRequest := context.WithCancel(context.Background())
	defer cancelRequest()
	ctx, cancelOwner := service.WithAPIKeyAdmissionOwner(parent)
	defer cancelOwner()
	c, _ := newHelperTestContext(http.MethodPost, "/v1/responses")
	c.Request = c.Request.WithContext(ctx)
	done := make(chan error, 1)
	go func() {
		started := false
		release, err := helper.acquireUserSlotWithWaitTimeout(c, 1, 1, 77, 1, time.Second, false, &started)
		if release != nil {
			release()
		}
		done <- err
	}()
	keyCache := cache.(service.APIKeyConcurrencyCache)
	require.Eventually(t, func() bool {
		counts, err := keyCache.GetAPIKeyConcurrencyBatch(context.Background(), []int64{77})
		return err == nil && counts[77] == 1
	}, time.Second, time.Millisecond)
	cancelRequest()
	require.ErrorIs(t, <-done, context.Canceled)
	counts, err := keyCache.GetAPIKeyConcurrencyBatch(context.Background(), []int64{77})
	require.NoError(t, err)
	require.Zero(t, counts[77])
	userCount, err := cache.GetUserConcurrency(context.Background(), 1)
	require.NoError(t, err)
	require.Equal(t, 1, userCount, "the existing request's user slot must be preserved")
}
