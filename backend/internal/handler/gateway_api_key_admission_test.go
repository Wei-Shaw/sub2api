package handler

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/internal/testutil"
	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func newAPIKeyAdmissionHelper(t *testing.T) (*ConcurrencyHelper, service.ConcurrencyCache) {
	t.Helper()
	cache := testutil.NewRedisConcurrencyCache(t)
	return NewConcurrencyHelper(service.NewConcurrencyService(cache), SSEPingFormatNone, time.Millisecond), cache
}

func TestAPIKeyAdmissionHTTPRejectsAndReleasesUser(t *testing.T) {
	helper, cache := newAPIKeyAdmissionHelper(t)
	ctx, cancelOwner := service.WithAPIKeyAdmissionOwner(context.Background())
	defer cancelOwner()
	hold, acquired, err := helper.TryAcquireUserSlotForAPIKey(ctx, 1, 3, 77, 1)
	require.NoError(t, err)
	require.True(t, acquired)
	defer hold()
	c, _ := newHelperTestContext(http.MethodPost, "/v1/messages")
	c.Request = c.Request.WithContext(ctx)
	started := false
	release, err := helper.AcquireUserSlotWithWait(c, 2, 3, 77, 1, false, &started)
	require.Nil(t, release)
	status, _, _, message := concurrencyErrorResponse(err, "user")
	require.Equal(t, http.StatusTooManyRequests, status)
	require.Contains(t, message, "API key")
	for _, respond := range []func(*gin.Context, error, string, bool){(&GatewayHandler{}).handleConcurrencyError, (&OpenAIGatewayHandler{}).handleConcurrencyError} {
		responseContext, recorder := newHelperTestContext(http.MethodPost, "/v1/responses")
		respond(responseContext, err, "user", false)
		require.Equal(t, http.StatusTooManyRequests, recorder.Code)
		require.Contains(t, recorder.Body.String(), "API key")
	}
	require.Equal(t, coderws.StatusTryAgainLater, openAIWSUserSlotAcquireError(err).StatusCode())
	count, err := cache.GetUserConcurrency(ctx, 2)
	require.NoError(t, err)
	require.Zero(t, count, "key rejection must undo the acquired user slot")
	other, ok, err := helper.TryAcquireUserSlotForAPIKey(ctx, 2, 1, 78, 1)
	require.NoError(t, err)
	require.True(t, ok)
	defer other()
	blocked, ok, err := helper.TryAcquireUserSlotForAPIKey(ctx, 2, 1, 79, 0)
	require.NoError(t, err)
	require.False(t, ok, "unlimited key cannot bypass the existing user limit")
	require.Nil(t, blocked)
	hold()
	release, err = helper.AcquireUserSlotWithWait(c, 3, 3, 77, 1, false, &started)
	require.NoError(t, err)
	require.NotNil(t, release)
	release()
}

type unavailableKeyAdmissionCache struct{ service.ConcurrencyCache }

func (*unavailableKeyAdmissionCache) TrackAPIKeySlot(context.Context, int64, string) error {
	return errors.New("redis unavailable")
}
func (*unavailableKeyAdmissionCache) AcquireAPIKeySlot(context.Context, int64, int, string) (bool, error) {
	return false, errors.New("redis unavailable")
}
func (*unavailableKeyAdmissionCache) ReleaseAPIKeySlot(context.Context, int64, string) error {
	return nil
}
func (*unavailableKeyAdmissionCache) GetAPIKeyConcurrencyBatch(context.Context, []int64) (map[int64]int, error) {
	return nil, errors.New("redis unavailable")
}

func TestAPIKeyAdmissionRedisFailureDoesNotAdmit(t *testing.T) {
	_, cache := newAPIKeyAdmissionHelper(t)
	helper := NewConcurrencyHelper(service.NewConcurrencyService(&unavailableKeyAdmissionCache{cache}), SSEPingFormatNone, time.Millisecond)
	c, _ := newHelperTestContext(http.MethodPost, "/v1/responses")
	started := false
	release, err := helper.AcquireUserSlotWithWait(c, 1, 3, 77, 1, false, &started)
	require.Nil(t, release)
	status, _, _, _ := concurrencyErrorResponse(err, "user")
	require.Equal(t, http.StatusServiceUnavailable, status)
	count, err := cache.GetUserConcurrency(context.Background(), 1)
	require.NoError(t, err)
	require.Zero(t, count)
}

func TestWSAPIKeyAdmissionSecondTurnRejectsWithoutUpstreamRetry(t *testing.T) {
	helper, cache := newAPIKeyAdmissionHelper(t)
	var upstreamCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_1\",\"usage\":{\"input_tokens\":2,\"output_tokens\":1}}}\n\n")
	}))
	defer upstream.Close()
	cfg := &config.Config{}
	cfg.Security.URLAllowlist.AllowPrivateHosts = true
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	cfg.Gateway.OpenAIWS.ModeRouterV2Enabled = true
	cfg.Gateway.OpenAIWS.WriteTimeoutSeconds = 1
	svc := service.NewOpenAIGatewayService(nil, nil, nil, nil, nil, nil, nil, cfg, nil, nil, nil, nil, nil, testutil.NewRealHTTPUpstream(cfg), nil, nil, nil, nil, nil, nil, nil, nil)
	account := &service.Account{ID: 1, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey, Concurrency: 3,
		Credentials: map[string]any{"base_url": upstream.URL, "api_key": "sk-test"},
		Extra:       map[string]any{"openai_apikey_responses_websockets_v2_mode": service.OpenAIWSIngressModeHTTPBridge}}
	firstSettled := make(chan struct{}, 1)
	done := make(chan error, 1)
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := coderws.Accept(w, r, nil)
		if err != nil {
			done <- err
			return
		}
		defer func() { _ = conn.CloseNow() }()
		ctx, cancelOwner := service.WithAPIKeyAdmissionOwner(r.Context())
		defer cancelOwner()
		release, acquired, err := helper.TryAcquireWSUserSlotForAPIKey(ctx, 202, 3, 77, 1)
		if err != nil || !acquired {
			done <- errors.New("first turn not acquired")
			return
		}
		defer func() {
			if release != nil {
				release()
			}
		}()
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = r.WithContext(ctx)
		err = svc.ProxyResponsesWebSocketFromClient(ctx, c, conn, account, "sk-test", []byte(`{"type":"response.create","model":"gpt-5","input":"hi"}`), &service.OpenAIWSIngressHooks{
			BeforeTurn: func(turn int) error {
				if turn == 1 {
					return nil
				}
				var err error
				release, acquired, err = helper.TryAcquireWSUserSlotForAPIKey(ctx, 202, 3, 77, 1)
				if err != nil {
					return openAIWSUserSlotAcquireError(err)
				}
				if !acquired {
					return errors.New("user full")
				}
				return nil
			},
			AfterTurn: func(turn int, _ *service.OpenAIForwardResult, _ error) {
				if release != nil {
					release()
					release = nil
				}
				if turn == 1 {
					firstSettled <- struct{}{}
				}
			},
		})
		var closeErr *service.OpenAIWSClientCloseError
		if errors.As(err, &closeErr) {
			_ = conn.Close(closeErr.StatusCode(), closeErr.Reason())
		}
		done <- err
	}))
	defer gateway.Close()
	client, _, err := coderws.Dial(context.Background(), "ws"+strings.TrimPrefix(gateway.URL, "http"), nil)
	require.NoError(t, err)
	defer func() { _ = client.CloseNow() }()
	readCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, _, err = client.Read(readCtx)
	require.NoError(t, err)
	select {
	case <-firstSettled:
	case <-readCtx.Done():
		t.Fatal("first turn did not release")
	}
	competitorCtx, cancelCompetitor := service.WithAPIKeyAdmissionOwner(context.Background())
	defer cancelCompetitor()
	competitor, acquired, err := helper.TryAcquireWSUserSlotForAPIKey(competitorCtx, 203, 3, 77, 1)
	require.NoError(t, err)
	require.True(t, acquired)
	defer competitor()
	require.NoError(t, client.Write(readCtx, coderws.MessageText, []byte(`{"type":"response.create","model":"gpt-5","input":"next"}`)))
	_, _, err = client.Read(readCtx)
	require.Equal(t, coderws.StatusTryAgainLater, coderws.CloseStatus(err))
	require.Contains(t, err.Error(), "API key concurrency")
	select {
	case err := <-done:
		require.Error(t, err)
	case <-readCtx.Done():
		t.Fatal("rejected turn did not stop")
	}
	require.EqualValues(t, 1, upstreamCalls.Load(), "key rejection must not forward or retry")
	count, err := cache.GetUserConcurrency(context.Background(), 202)
	require.NoError(t, err)
	require.Zero(t, count)
}
