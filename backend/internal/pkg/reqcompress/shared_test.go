package reqcompress

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func sharedTestRedis(t *testing.T) (*miniredis.Miniredis, *redis.Client) {
	t.Helper()
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr(), MaxRetries: -1})
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	return server, client
}

func sharedTestRegistry(client *redis.Client) *Registry {
	registry := NewRegistry()
	registry.SetRedis(client)
	return registry
}

func requireSharedAlgorithm(t *testing.T, server *miniredis.Miniredis, accountID int64, want Algorithm) {
	t.Helper()
	got, err := server.Get(sharedAlgorithmKey(accountID))
	require.NoError(t, err)
	require.Equal(t, string(want), got)
}

func TestRegistrySharesSettledAlgorithmAcrossInstances(t *testing.T) {
	server, client := sharedTestRedis(t)
	first := sharedTestRegistry(client)
	second := sharedTestRegistry(client)
	const accountID int64 = 701
	first.Sync(accountID, true, time.Time{})
	second.Sync(accountID, true, time.Time{})

	_, err := first.Execute(accountID, newTestRequest(t, testBody()), func(req *http.Request) (*http.Response, error) {
		return okResponse("ok"), nil
	})
	require.NoError(t, err)
	requireSharedAlgorithm(t, server, accountID, AlgorithmBrotli)
	require.Zero(t, server.TTL(sharedAlgorithmKey(accountID)), "shared algorithms must not expire")

	// A second process reuses the result without probing br/zstd/deflate again.
	require.Equal(t, AlgorithmBrotli, second.SettledAlgorithm(context.Background(), accountID))
	var sent []string
	_, err = second.Execute(accountID, newTestRequest(t, testBody()), func(req *http.Request) (*http.Response, error) {
		sent = append(sent, req.Header.Get("Content-Encoding"))
		return okResponse("ok"), nil
	})
	require.NoError(t, err)
	require.Equal(t, []string{"br"}, sent)

	// Once initialized, the hot path is memory-only and survives a Redis outage.
	server.Close()
	_, err = second.Execute(accountID, newTestRequest(t, testBody()), func(req *http.Request) (*http.Response, error) {
		require.Equal(t, "br", req.Header.Get("Content-Encoding"))
		return okResponse("ok"), nil
	})
	require.NoError(t, err)
}

func TestRegistrySharedWritesSurviveRequestCancellation(t *testing.T) {
	server, client := sharedTestRedis(t)
	registry := sharedTestRegistry(client)
	const accountID int64 = 709
	registry.Sync(accountID, true, time.Time{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req := newTestRequest(t, testBody()).WithContext(ctx)
	_, err := registry.Execute(accountID, req, func(attempt *http.Request) (*http.Response, error) {
		if attempt.Header.Get("Content-Encoding") == "zstd" {
			// The upstream succeeded, so cancellation of the client-facing request
			// must not discard the negotiated shared result.
			cancel()
			return okResponse("ok"), nil
		}
		return rejectionResponse("unsupported content encoding"), nil
	})
	require.NoError(t, err)
	requireSharedAlgorithm(t, server, accountID, AlgorithmZstd)

	registry.deleteSharedAlgorithm(ctx, accountID, AlgorithmZstd)
	require.False(t, server.Exists(sharedAlgorithmKey(accountID)))
}

func TestRegistrySharedLoadFailureDoesNotRestartNegotiation(t *testing.T) {
	server, client := sharedTestRedis(t)
	writer := sharedTestRegistry(client)
	observer := sharedTestRegistry(client)
	const accountID int64 = 710
	writer.Sync(accountID, true, time.Time{})
	observer.Sync(accountID, true, time.Time{})

	_, err := writer.Execute(accountID, newTestRequest(t, testBody()), func(attempt *http.Request) (*http.Response, error) {
		if attempt.Header.Get("Content-Encoding") == "br" {
			return rejectionResponse("unsupported content encoding"), nil
		}
		return okResponse("ok"), nil
	})
	require.NoError(t, err)
	requireSharedAlgorithm(t, server, accountID, AlgorithmZstd)

	// A fresh instance that cannot read the shared result must forward identity
	// and retry initialization later, not probe br and overwrite zstd.
	server.Close()
	var sent []string
	_, err = observer.Execute(accountID, newTestRequest(t, testBody()), func(attempt *http.Request) (*http.Response, error) {
		sent = append(sent, attempt.Header.Get("Content-Encoding"))
		return okResponse("ok"), nil
	})
	require.NoError(t, err)
	require.Equal(t, []string{""}, sent)
}

func TestRegistrySettledAlgorithmTracksSharedChanges(t *testing.T) {
	server, client := sharedTestRedis(t)
	writer := sharedTestRegistry(client)
	observer := sharedTestRegistry(client)
	const accountID int64 = 708
	writer.Sync(accountID, true, time.Time{})
	observer.Sync(accountID, true, time.Time{})

	_, err := writer.Execute(accountID, newTestRequest(t, testBody()), func(req *http.Request) (*http.Response, error) {
		return okResponse("ok"), nil
	})
	require.NoError(t, err)
	require.Equal(t, AlgorithmBrotli, observer.SettledAlgorithm(context.Background(), accountID))

	// Simulate another instance publishing a replacement and then clearing it.
	// Administrative display reads must not keep returning the observer's first
	// process-local snapshot after the shared value has moved on.
	require.NoError(t, server.Set(sharedAlgorithmKey(accountID), string(AlgorithmZstd)))
	require.Equal(t, AlgorithmZstd, observer.SettledAlgorithm(context.Background(), accountID))
	require.NoError(t, client.Del(context.Background(), sharedAlgorithmKey(accountID)).Err())
	require.Equal(t, AlgorithmNone, observer.SettledAlgorithm(context.Background(), accountID))
}

func TestRegistryRetiresSharedAlgorithmAndPublishesReplacement(t *testing.T) {
	server, client := sharedTestRedis(t)
	first := sharedTestRegistry(client)
	second := sharedTestRegistry(client)
	const accountID int64 = 702
	first.Sync(accountID, true, time.Time{})
	second.Sync(accountID, true, time.Time{})

	_, err := first.Execute(accountID, newTestRequest(t, testBody()), func(req *http.Request) (*http.Response, error) {
		return okResponse("ok"), nil
	})
	require.NoError(t, err)
	require.Equal(t, AlgorithmBrotli, second.SettledAlgorithm(context.Background(), accountID))

	var sent []string
	_, err = second.Execute(accountID, newTestRequest(t, testBody()), func(req *http.Request) (*http.Response, error) {
		encoding := req.Header.Get("Content-Encoding")
		sent = append(sent, encoding)
		if encoding == "br" {
			return rejectionResponse("unsupported content encoding"), nil
		}
		return okResponse("ok"), nil
	})
	require.NoError(t, err)
	require.Equal(t, []string{"br", "zstd"}, sent)
	requireSharedAlgorithm(t, server, accountID, AlgorithmZstd)

	third := sharedTestRegistry(client)
	third.Sync(accountID, true, time.Time{})
	require.Equal(t, AlgorithmZstd, third.SettledAlgorithm(context.Background(), accountID))
}

func TestRegistryResetClearsLocalAndSharedAlgorithm(t *testing.T) {
	server, client := sharedTestRedis(t)
	registry := sharedTestRegistry(client)
	const accountID int64 = 703
	registry.Sync(accountID, true, time.Time{})

	_, err := registry.Execute(accountID, newTestRequest(t, testBody()), func(req *http.Request) (*http.Response, error) {
		return okResponse("ok"), nil
	})
	require.NoError(t, err)
	require.True(t, server.Exists(sharedAlgorithmKey(accountID)))

	registry.Reset(context.Background(), accountID)
	require.False(t, server.Exists(sharedAlgorithmKey(accountID)))
	require.Equal(t, AlgorithmNone, registry.SettledAlgorithm(context.Background(), accountID))

	// Re-enabling starts with an empty shared snapshot rather than resurrecting
	// the algorithm that was pinned before the administrative reset.
	registry.Sync(accountID, true, time.Time{})
	require.Equal(t, AlgorithmNone, registry.SettledAlgorithm(context.Background(), accountID))
}

func TestRegistryDisableDeletesSharedAlgorithm(t *testing.T) {
	server, client := sharedTestRedis(t)
	registry := sharedTestRegistry(client)
	const accountID int64 = 706
	registry.Sync(accountID, true, time.Time{})

	_, err := registry.Execute(accountID, newTestRequest(t, testBody()), func(req *http.Request) (*http.Response, error) {
		return okResponse("ok"), nil
	})
	require.NoError(t, err)
	require.True(t, server.Exists(sharedAlgorithmKey(accountID)))

	registry.Sync(accountID, false, time.Time{})
	require.False(t, registry.Enabled(accountID))
	require.False(t, server.Exists(sharedAlgorithmKey(accountID)))

	registry.Sync(accountID, true, time.Time{})
	require.Equal(t, AlgorithmNone, registry.SettledAlgorithm(context.Background(), accountID))
}

func TestRegistryDoesNotPersistIdentity(t *testing.T) {
	server, client := sharedTestRedis(t)
	registry := sharedTestRegistry(client)
	const accountID int64 = 704
	registry.Sync(accountID, true, time.Time{})

	var sent []string
	_, err := registry.Execute(accountID, newTestRequest(t, testBody()), func(req *http.Request) (*http.Response, error) {
		encoding := req.Header.Get("Content-Encoding")
		sent = append(sent, encoding)
		if encoding == "" {
			return okResponse("identity"), nil
		}
		return rejectionResponse("unsupported content encoding"), nil
	})
	require.NoError(t, err)
	require.Equal(t, []string{"br", "zstd", "deflate", ""}, sent)
	require.False(t, server.Exists(sharedAlgorithmKey(accountID)))
}

func TestRegistryDoesNotPersistAlgorithmAfterReset(t *testing.T) {
	server, client := sharedTestRedis(t)
	registry := sharedTestRegistry(client)
	const accountID int64 = 707
	registry.Sync(accountID, true, time.Time{})
	registry.Reset(context.Background(), accountID)

	// Simulate a coded request that completed after the administrative reset.
	registry.persistSharedAlgorithm(context.Background(), accountID, AlgorithmBrotli)
	require.False(t, server.Exists(sharedAlgorithmKey(accountID)))
}

func TestRegistryDeleteSharedAlgorithmDoesNotRemoveDifferentCoding(t *testing.T) {
	server, client := sharedTestRedis(t)
	registry := sharedTestRegistry(client)
	const accountID int64 = 705
	require.NoError(t, server.Set(sharedAlgorithmKey(accountID), "zstd"))

	registry.deleteSharedAlgorithm(context.Background(), accountID, AlgorithmBrotli)
	requireSharedAlgorithm(t, server, accountID, AlgorithmZstd)

	registry.deleteSharedAlgorithm(context.Background(), accountID, AlgorithmZstd)
	require.False(t, server.Exists(sharedAlgorithmKey(accountID)))
}
