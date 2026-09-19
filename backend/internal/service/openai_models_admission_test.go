//go:build unit

package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type admissionModelsResult struct {
	manifest *OpenAIModelsResponse
	err      error
}

func newAdmissionModelsService(upstream HTTPUpstream) *OpenAIGatewayService {
	s := newCodexModelsAPIKeyTestService(upstream)
	s.cfg.Security.URLAllowlist.AllowPrivateHosts = true
	s.cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	return s
}

func TestAPIKeyAdmissionModelsStaleRefreshHoldsSlot(t *testing.T) {
	var calls atomic.Int32
	started, allow := make(chan struct{}), make(chan struct{})
	release := sync.OnceFunc(func() { close(allow) })
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) > 1 {
			close(started)
			select {
			case <-allow:
			case <-r.Context().Done():
				return
			}
			_, _ = io.WriteString(w, `{"models":[{"slug":"refreshed-model"}]}`)
			return
		}
		_, _ = io.WriteString(w, `{"models":[{"slug":"cached-model"}]}`)
	}))
	defer func() { release(); server.Close() }()
	s := newAdmissionModelsService(&codexModelsHTTPUpstreamStub{do: func(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
		return http.DefaultClient.Do(req)
	}})
	account := newCodexModelsAPIKeyTestAccount(server.URL)
	_, err := s.FetchCodexModelsManifest(context.Background(), account, "", "")
	require.NoError(t, err)
	expireCodexModelsManifestCache(s, 2*time.Minute)
	ctx, cancel := WithAPIKeyAdmissionOwner(context.Background())
	defer cancel()
	cache := &apiKeyAdmissionLifecycleCache{}
	lease, err := NewConcurrencyService(cache).AcquireAPIKeySlot(ctx, 8, 1)
	require.NoError(t, err)
	defer lease.ReleaseFunc()
	done := make(chan admissionModelsResult, 1)
	go func() {
		manifest, err := s.FetchCodexModelsManifest(ctx, account, "", "")
		lease.ReleaseFunc()
		done <- admissionModelsResult{manifest, err}
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("stale refresh did not start")
	}
	select {
	case result := <-done:
		t.Fatalf("stale response released active refresh: %v", result.err)
	case <-time.After(20 * time.Millisecond):
	}
	cache.mu.Lock()
	require.Empty(t, cache.releases)
	cache.mu.Unlock()
	release()
	select {
	case result := <-done:
		require.NoError(t, result.err)
		require.Contains(t, string(result.manifest.Body), "refreshed-model")
	case <-time.After(time.Second):
		t.Fatal("refresh did not join")
	}
	cache.mu.Lock()
	require.Len(t, cache.releases, 1)
	cache.mu.Unlock()
}

type modelsAdmissionCloseBarrier struct {
	io.ReadCloser
	closing    chan struct{}
	allowClose chan struct{}
}

func (b *modelsAdmissionCloseBarrier) Close() error {
	close(b.closing)
	<-b.allowClose
	return b.ReadCloser.Close()
}

func TestAPIKeyAdmissionModelsColdCancellationJoinsWithoutCancelingPeer(t *testing.T) {
	for _, limitedPeer := range []bool{true, false} {
		t.Run(map[bool]string{true: "limited_peer", false: "unlimited_peer"}[limitedPeer], func(t *testing.T) {
			var calls atomic.Int32
			firstStarted, firstStopped := make(chan struct{}), make(chan struct{})
			bodyReady, forceStop := make(chan struct{}), make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if calls.Add(1) == 1 {
					w.WriteHeader(http.StatusOK)
					w.(http.Flusher).Flush()
					close(firstStarted)
					select {
					case <-r.Context().Done():
					case <-forceStop:
					}
					close(firstStopped)
					return
				}
				_, _ = io.WriteString(w, `{"models":[{"slug":"peer-model"}]}`)
			}))
			ctx, cancel := WithAPIKeyAdmissionOwner(context.Background())
			barrier := &modelsAdmissionCloseBarrier{closing: make(chan struct{}), allowClose: make(chan struct{})}
			allow := sync.OnceFunc(func() { close(barrier.allowClose) })
			defer func() { cancel(); allow(); close(forceStop); server.Close() }()
			type firstCallerKey struct{}
			ctx = context.WithValue(ctx, firstCallerKey{}, true)
			s := newAdmissionModelsService(&codexModelsHTTPUpstreamStub{do: func(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
				response, err := http.DefaultClient.Do(req)
				if err == nil && req.Context().Value(firstCallerKey{}) == true {
					barrier.ReadCloser = response.Body
					response.Body = barrier
					close(bodyReady)
				}
				return response, err
			}})
			account := newCodexModelsAPIKeyTestAccount(server.URL)
			cache := &apiKeyAdmissionLifecycleCache{}
			lease, err := NewConcurrencyService(cache).AcquireAPIKeySlot(ctx, 8, 1)
			require.NoError(t, err)
			defer lease.ReleaseFunc()
			done := make(chan error, 1)
			go func() {
				_, err := s.FetchCodexModelsManifest(ctx, account, "", "")
				lease.ReleaseFunc()
				done <- err
			}()
			select {
			case <-firstStarted:
			case <-time.After(time.Second):
				t.Fatal("cold refresh did not start")
			}
			select {
			case <-bodyReady:
			case <-time.After(time.Second):
				t.Fatal("refresh did not receive upstream headers")
			}
			peerCtx := context.Background()
			if limitedPeer {
				var stop context.CancelFunc
				peerCtx, stop = WithAPIKeyAdmissionOwner(peerCtx)
				defer stop()
				peerLease, err := NewConcurrencyService(&apiKeyAdmissionLifecycleCache{}).AcquireAPIKeySlot(peerCtx, 9, 1)
				require.NoError(t, err)
				defer peerLease.ReleaseFunc()
			}
			peerDone := make(chan admissionModelsResult, 1)
			go func() {
				response, err := s.FetchCodexModelsManifest(peerCtx, account, "", "")
				peerDone <- admissionModelsResult{response, err}
			}()
			owner, ok := apiKeyAdmissionOwnerFromContext(ctx)
			require.True(t, ok)
			owner.cancel(ErrAPIKeySlotLeaseLost)
			select {
			case <-firstStopped:
			case <-time.After(time.Second):
				t.Fatal("lease loss did not cancel HTTP transport")
			}
			select {
			case <-barrier.closing:
			case <-time.After(time.Second):
				t.Fatal("body close did not begin")
			}
			select {
			case err := <-done:
				t.Fatalf("returned while upstream close was blocked: %v", err)
			default:
			}
			cache.mu.Lock()
			require.Empty(t, cache.releases)
			cache.mu.Unlock()
			select {
			case peer := <-peerDone:
				require.NoError(t, peer.err)
				require.Contains(t, string(peer.manifest.Body), "peer-model")
			case <-time.After(time.Second):
				t.Fatal("independent peer was canceled or stranded behind another owner")
			}
			allow()
			select {
			case err := <-done:
				require.Error(t, err)
			case <-time.After(time.Second):
				t.Fatal("canceled refresh did not join")
			}
			cached, err := s.FetchCodexModelsManifest(context.Background(), account, "", "")
			require.NoError(t, err)
			require.True(t, strings.Contains(string(cached.Body), "peer-model"))
			require.EqualValues(t, 2, calls.Load(), "callers share completed cache entries")
			cache.mu.Lock()
			require.Len(t, cache.releases, 1)
			cache.mu.Unlock()
		})
	}
}
