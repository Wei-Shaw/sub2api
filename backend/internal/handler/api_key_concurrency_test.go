package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/internal/testutil"
	coderws "github.com/coder/websocket"
	"github.com/stretchr/testify/require"
)

type concurrencyTestAPIKeyRepo struct {
	service.APIKeyRepository
	key   *service.APIKey
	limit *atomic.Int64
}

func (r *concurrencyTestAPIKeyRepo) GetByKeyForAuth(context.Context, string) (*service.APIKey, error) {
	key := *r.key
	if r.limit != nil {
		key.ConcurrencyLimit = int(r.limit.Load())
	}
	return &key, nil
}

func newConcurrencyTestAPIKeyService(key *service.APIKey) *service.APIKeyService {
	if key.Key == "" {
		key.Key = "sk-concurrency-test"
	}
	return service.NewAPIKeyService(&concurrencyTestAPIKeyRepo{key: key}, nil, nil, nil, nil, nil, &config.Config{})
}

func TestAPIKeyConcurrencySharedUserAndRelease(t *testing.T) {
	cache := testutil.NewRedisConcurrencyCache(t)
	svc := service.NewConcurrencyService(cache)
	helper := NewConcurrencyHelper(svc, SSEPingFormatNone, time.Second)
	ctx := context.Background()
	c, _ := newHelperTestContext(http.MethodPost, "/v1/responses")
	c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{ID: 1, ConcurrencyLimit: 1})
	started := false
	first, err := helper.AcquireUserSlotWithWait(c, 9, 2, false, &started)
	require.NoError(t, err)
	t.Cleanup(first)
	blocked, _ := newHelperTestContext(http.MethodPost, "/v1/responses")
	blocked.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{ID: 1, ConcurrencyLimit: 1})
	_, err = helper.AcquireUserSlotWithWait(blocked, 9, 2, true, &started)
	require.ErrorIs(t, err, service.ErrAPIKeyConcurrencyExceeded)
	require.False(t, blocked.Writer.Written())
	status, _, _, _ := concurrencyErrorResponse(err, "user")
	require.Equal(t, http.StatusTooManyRequests, status)
	second, ok, err := helper.TryAcquireUserSlotForAPIKey(ctx, 9, 2, 2, 1, nil)
	require.NoError(t, err)
	require.True(t, ok)
	t.Cleanup(second)
	_, ok, err = helper.TryAcquireUserSlotForAPIKey(ctx, 9, 2, 3, 1, nil)
	require.NoError(t, err)
	require.False(t, ok, "another key cannot bypass the shared user limit")
	counts, err := svc.GetAPIKeyConcurrencyBatch(ctx, []int64{1, 2, 3})
	require.NoError(t, err)
	require.Equal(t, map[int64]int{1: 1, 2: 1, 3: 0}, counts)
	first()
	first()
	third, ok, err := helper.TryAcquireUserSlotForAPIKey(ctx, 9, 2, 3, 1, nil)
	require.NoError(t, err)
	require.True(t, ok)
	third()
	second()
}

func TestAPIKeyConcurrencyWebSocketReloadsPolicyBetweenTurns(t *testing.T) {
	var upstreamRequests atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := coderws.Accept(w, r, &coderws.AcceptOptions{CompressionMode: coderws.CompressionContextTakeover})
		if err != nil {
			return
		}
		defer func() { _ = conn.CloseNow() }()
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		for {
			if _, _, err := conn.Read(ctx); err != nil {
				return
			}
			upstreamRequests.Add(1)
			if err := conn.Write(ctx, coderws.MessageText, []byte(`{"type":"response.completed","response":{"id":"resp_key_limit","model":"gpt-5.1","usage":{"input_tokens":2,"output_tokens":1}}}`)); err != nil {
				return
			}
		}
	}))
	defer upstream.Close()
	svc := service.NewConcurrencyService(testutil.NewRedisConcurrencyCache(t))
	var limit atomic.Int64 // Existing and new keys initially have no limit.
	harness := newOpenAIWSPassthroughHandlerHarness(t, upstream.URL, func(h *OpenAIGatewayHandler, key *service.APIKey) {
		h.concurrencyHelper = NewConcurrencyHelper(svc, SSEPingFormatNone, time.Second)
		h.apiKeyService = service.NewAPIKeyService(&concurrencyTestAPIKeyRepo{key: key, limit: &limit}, nil, nil, nil, nil, nil, &config.Config{})
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	payload := []byte(`{"type":"response.create","model":"gpt-5.1","input":"first"}`)
	require.NoError(t, harness.clientConn.Write(ctx, coderws.MessageText, payload))
	_, response, err := harness.clientConn.Read(ctx)
	require.NoError(t, err)
	require.Contains(t, string(response), "response.completed")
	require.Eventually(t, func() bool {
		counts, err := svc.GetAPIKeyConcurrencyBatch(ctx, []int64{harness.apiKey.ID})
		return err == nil && counts[harness.apiKey.ID] == 0
	}, time.Second, time.Millisecond, "an idle WebSocket must not occupy request capacity")
	other, err := svc.AcquireAPIKeySlot(ctx, harness.apiKey.ID, 0, nil)
	require.NoError(t, err)
	defer other()
	limit.Store(1)
	require.NoError(t, harness.clientConn.Write(ctx, coderws.MessageText, payload))
	_, _, err = harness.clientConn.Read(ctx)
	require.Equal(t, coderws.StatusTryAgainLater, coderws.CloseStatus(err))
	require.True(t, strings.Contains(err.Error(), "api_key_concurrency_limit"), err)
	require.EqualValues(t, 1, upstreamRequests.Load(), "the rejected turn must never reach upstream")
}
