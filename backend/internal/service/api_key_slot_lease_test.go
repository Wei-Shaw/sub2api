//go:build unit

package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/synctest"
	"time"

	coderws "github.com/coder/websocket"
	"github.com/stretchr/testify/require"
)

func TestAPIKeyAdmissionLeaseLossBeforeExpiry(t *testing.T) {
	for _, mode := range []string{"renew_error", "missing", "blocked"} {
		t.Run(mode, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				cache := &apiKeyAdmissionLifecycleCache{}
				ctx, cancel := WithAPIKeyAdmissionOwner(context.Background())
				defer cancel()
				result, err := NewConcurrencyService(cache).AcquireAPIKeySlot(ctx, 8, 1)
				require.NoError(t, err)
				cache.mu.Lock()
				oldID, expires := cache.id, cache.expires
				switch mode {
				case "renew_error":
					cache.refreshErr = errors.New("redis unavailable")
				case "missing":
					cache.id = ""
				case "blocked":
					cache.blockRefresh = true
				}
				cache.mu.Unlock()
				upstream, stop := detachUpstreamContext(ctx)
				defer stop()
				time.Sleep(2 * time.Second)
				synctest.Wait()
				require.True(t, APIKeySlotLeaseLost(ctx))
				require.ErrorIs(t, context.Cause(upstream), ErrAPIKeySlotLeaseLost)
				require.True(t, time.Now().Before(expires), "stop before the last acknowledged TTL, not failure time + TTL")
				require.Empty(t, cache.releases, "cancellation must not release before upstream has joined")
				if mode != "missing" {
					require.Equal(t, oldID, cache.id)
					otherCtx, cancelOther := WithAPIKeyAdmissionOwner(context.Background())
					defer cancelOther()
					other, otherErr := NewConcurrencyService(cache).AcquireAPIKeySlot(otherCtx, 8, 1)
					require.NoError(t, otherErr)
					require.False(t, other.Acquired, "another client must not enter while the old transport is joining")
				}
				refreshes := cache.refreshes
				time.Sleep(time.Second)
				synctest.Wait()
				require.Equal(t, refreshes, cache.refreshes, "lost lease worker must stop")
				result.ReleaseFunc()
				result.ReleaseFunc()
				require.Equal(t, []string{oldID}, cache.releases)
			})
		})
	}
}

func TestAPIKeyAdmissionDetachedBuilderReleasePreservesOwner(t *testing.T) {
	type routingKey struct{}
	ctx, cancel := WithAPIKeyAdmissionOwner(context.Background())
	defer cancel()
	ctx = context.WithValue(ctx, routingKey{}, "latest routing decision")
	upstream, release := detachUpstreamContext(ctx)
	release() // Existing builders release before httpUpstream.Do.
	require.NoError(t, upstream.Err())
	require.Equal(t, "latest routing decision", upstream.Value(routingKey{}))
	owner, ok := apiKeyAdmissionOwnerFromContext(ctx)
	require.True(t, ok)
	owner.cancel(ErrAPIKeySlotLeaseLost)
	require.ErrorIs(t, context.Cause(upstream), ErrAPIKeySlotLeaseLost)
}

func TestAPIKeyAdmissionRejectsInvalidCancellationOwner(t *testing.T) {
	for name, ctx := range map[string]context.Context{
		"nil context":     nil,
		"absent":          context.Background(),
		"wrong type":      context.WithValue(context.Background(), apiKeyAdmissionOwnerKey{}, "invalid"),
		"typed nil":       context.WithValue(context.Background(), apiKeyAdmissionOwnerKey{}, (*apiKeyAdmissionOwner)(nil)),
		"missing control": context.WithValue(context.Background(), apiKeyAdmissionOwnerKey{}, &apiKeyAdmissionOwner{cancel: func(error) {}}),
		"missing cancel":  context.WithValue(context.Background(), apiKeyAdmissionOwnerKey{}, &apiKeyAdmissionOwner{ctx: context.Background()}),
	} {
		t.Run(name, func(t *testing.T) {
			require.False(t, HasAPIKeyAdmissionOwner(ctx))
			require.False(t, APIKeySlotLeaseLost(ctx))
			cache := &apiKeyAdmissionLifecycleCache{}
			result, err := NewConcurrencyService(cache).AcquireAPIKeySlot(ctx, 8, 1)
			require.Error(t, err)
			require.Nil(t, result)
			require.Empty(t, cache.id, "invalid owners must fail before Redis admission")
			if ctx != nil {
				repaired, cancel := WithAPIKeyAdmissionOwner(ctx)
				defer cancel()
				require.True(t, HasAPIKeyAdmissionOwner(repaired))
			}
		})
	}
}

func TestAPIKeyAdmissionLeaseLossCancelsDetachedHTTPTransport(t *testing.T) {
	upstreamStopped := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
		close(upstreamStopped)
	}))
	defer upstream.Close()
	cache := &apiKeyAdmissionLifecycleCache{}
	ctx, cancel := WithAPIKeyAdmissionOwner(context.Background())
	defer cancel()
	lease, err := NewConcurrencyService(cache).AcquireAPIKeySlot(ctx, 8, 1)
	require.NoError(t, err)
	defer lease.ReleaseFunc()
	transportCtx, stop := detachStreamUpstreamContext(ctx, true)
	defer stop()
	request, err := http.NewRequestWithContext(transportCtx, http.MethodGet, upstream.URL, nil)
	require.NoError(t, err)
	response, err := http.DefaultClient.Do(request)
	require.NoError(t, err)
	defer response.Body.Close()
	cache.mu.Lock()
	cache.id = ""
	cache.mu.Unlock()
	var b [1]byte
	_, err = response.Body.Read(b[:])
	require.Error(t, err)
	require.True(t, APIKeySlotLeaseLost(ctx))
	select {
	case <-upstreamStopped:
	case <-time.After(time.Second):
		t.Fatal("detached HTTP transport did not stop")
	}
	cache.mu.Lock()
	require.Empty(t, cache.releases)
	cache.mu.Unlock()
}

type grokLeaseBarrierConn struct {
	*stagedPassthroughConn
	started, stopping, allowStop chan struct{}
}

func (c *grokLeaseBarrierConn) ReadMessage(ctx context.Context) ([]byte, error) {
	close(c.started)
	<-ctx.Done()
	close(c.stopping)
	<-c.allowStop
	return nil, ctx.Err()
}

func TestAPIKeyAdmissionGrokRealtimeJoinsBeforeRelease(t *testing.T) {
	ctx, cancel := WithAPIKeyAdmissionOwner(context.Background())
	defer cancel()
	cache := &apiKeyAdmissionLifecycleCache{}
	upstream := &grokLeaseBarrierConn{stagedPassthroughConn: newStagedPassthroughConn(), started: make(chan struct{}), stopping: make(chan struct{}), allowStop: make(chan struct{})}
	allowClosed := false
	done := make(chan error, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		client, err := coderws.Accept(w, r, nil)
		if err != nil {
			done <- err
			return
		}
		defer client.CloseNow()
		lease, err := NewConcurrencyService(cache).AcquireAPIKeySlot(ctx, 8, 1)
		if err != nil {
			done <- err
			return
		}
		_, err = (&OpenAIGatewayService{}).ProxyGrokRealtimeConn(ctx, nil, client, &GrokRealtimeUpstream{conn: upstream})
		lease.ReleaseFunc()
		done <- err
	}))
	defer func() {
		if !allowClosed {
			close(upstream.allowStop)
		}
		cancel()
		server.Close()
	}()
	client, _, err := coderws.Dial(context.Background(), "ws"+server.URL[len("http"):], nil)
	require.NoError(t, err)
	defer client.CloseNow()
	select {
	case <-upstream.started:
	case <-time.After(time.Second):
		t.Fatal("upstream reader did not start")
	}
	cache.mu.Lock()
	cache.id = ""
	cache.mu.Unlock()
	select {
	case <-upstream.stopping:
	case <-time.After(2 * time.Second):
		t.Fatal("lease loss did not stop realtime")
	}
	cache.mu.Lock()
	require.Empty(t, cache.releases)
	cache.mu.Unlock()
	select {
	case err := <-done:
		t.Fatalf("returned before upstream joined: %v", err)
	default:
	}
	close(upstream.allowStop)
	allowClosed = true
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("realtime reader did not join")
	}
	cache.mu.Lock()
	require.Len(t, cache.releases, 1)
	cache.mu.Unlock()
}
