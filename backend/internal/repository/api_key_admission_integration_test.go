//go:build integration

package repository

import (
	"context"
	"fmt"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestAPIKeyAdmissionDockerCompetition(t *testing.T) {
	// Both independent pools use the same isolated namespace on real Redis.
	prefix := fmt.Sprintf("key-admission:%d:", time.Now().UnixNano())
	var clients []*redis.Client
	for i := 0; i < 2; i++ {
		client := redis.NewClient(integrationRedis.Options())
		client.AddHook(prefixHook{prefix: prefix})
		clients = append(clients, client)
		t.Cleanup(func() { _ = client.Close() })
	}
	t.Cleanup(func() {
		_ = integrationRedis.Del(context.Background(), prefix+apiKeySlotKey(901), prefix+apiKeySlotKey(902)).Err()
	})
	testAPIKeyAdmissionCompetition(t, clients)
	testAPIKeyLiveAdmission(t, clients)
	testAPIKeyLeaseFailureOnRedis(t, clients)
}

type admissionRenewalFailureCache struct { *concurrencyCache }

func (*admissionRenewalFailureCache) RefreshAPIKeySlot(context.Context, int64, string) (bool, error) {
	return false, errors.New("injected renewal transport failure")
}

func testAPIKeyLeaseFailureOnRedis(t *testing.T, clients []*redis.Client) {
	t.Helper()
	first := NewConcurrencyCache(clients[0], 1, 60).(*concurrencyCache)
	second := NewConcurrencyCache(clients[1], 1, 60).(*concurrencyCache)
	first.slotTTLSeconds, second.slotTTLSeconds = 6, 6
	ctx, cancel := service.WithAPIKeyAdmissionOwner(context.Background())
	defer cancel()
	lease, err := service.NewConcurrencyService(&admissionRenewalFailureCache{first}).AcquireAPIKeySlot(ctx, 905, 1)
	require.NoError(t, err)
	defer lease.ReleaseFunc()
	stopped := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
		close(stopped)
	}))
	defer upstream.Close()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, upstream.URL, nil)
	require.NoError(t, err)
	response, err := http.DefaultClient.Do(request)
	require.NoError(t, err)
	defer response.Body.Close()
	select { case <-ctx.Done(): case <-time.After(5*time.Second): t.Fatal("lease owner was not canceled before expiry") }
	require.ErrorIs(t, context.Cause(ctx), service.ErrAPIKeySlotLeaseLost)
	otherCtx, stopOther := service.WithAPIKeyAdmissionOwner(context.Background())
	defer stopOther()
	other, err := service.NewConcurrencyService(second).AcquireAPIKeySlot(otherCtx, 905, 1)
	require.NoError(t, err)
	require.False(t, other.Acquired, "Redis still reserves capacity while the old transport joins")
	select { case <-stopped: case <-time.After(time.Second): t.Fatal("old HTTP upstream still active") }
	require.NoError(t, response.Body.Close())
	lease.ReleaseFunc()
	other, err = service.NewConcurrencyService(second).AcquireAPIKeySlot(otherCtx, 905, 1)
	require.NoError(t, err)
	require.True(t, other.Acquired)
	other.ReleaseFunc()
}
