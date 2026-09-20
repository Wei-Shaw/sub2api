package ticketproxy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type providerTransport func(*http.Request) (*http.Response, error)

func (f providerTransport) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }
func providerResponse(code int, body string) *http.Response {
	return &http.Response{StatusCode: code, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}
}
func testProvider(t *testing.T) *OnesProxyProvider {
	t.Helper()
	dir := t.TempDir()
	p := NewOnesProxyProvider(filepath.Join(dir, "config.json"), NewStore(filepath.Join(dir, "pool.json")))
	cfg := defaultProviderConfig()
	cfg.UserID = "test-user"
	cfg.Token = "secret-token"
	cfg.ProxyID = "test-proxy"
	_, err := p.Save(cfg)
	require.NoError(t, err)
	return p
}

func TestOnesProxyGenerates2000InOneRequestAndPreservesExisting(t *testing.T) {
	p := testProvider(t)
	existing := make([]string, 200)
	for i := range existing {
		existing[i] = fmt.Sprintf("http://existing-%d.example:8080", i)
	}
	importLines(t, p.pool, existing...)
	lines := make([]string, 2000)
	for i := range lines {
		lines[i] = fmt.Sprintf("proxy.example:8080:user-sid-%d:pass", i)
	}
	raw, err := json.Marshal(map[string]any{"code": 200, "data": map[string]any{"results": lines}})
	require.NoError(t, err)
	calls := 0
	p.client.Transport = providerTransport(func(req *http.Request) (*http.Response, error) {
		calls++
		require.Equal(t, http.MethodPost, req.Method)
		require.Equal(t, onesProxyOrigin+onesProxyGeneratePath, req.URL.String())
		require.Equal(t, "test-user", req.Header.Get("Userid"))
		require.Equal(t, "secret-token", req.Header.Get("Token"))
		var body map[string]any
		require.NoError(t, json.NewDecoder(req.Body).Decode(&body))
		require.Equal(t, float64(2000), body["proxy_num"])
		require.Equal(t, float64(2), body["protocol_type"])
		require.Equal(t, float64(1001), body["proxy_conn_type"])
		require.Equal(t, float64(30), body["session_time"])
		require.Equal(t, "US", body["country_code"])
		require.Equal(t, "NA", body["conn_route_code"])
		return providerResponse(200, string(raw)), nil
	})
	result, err := p.Fetch(context.Background(), 2000)
	require.NoError(t, err)
	require.Equal(t, 1, calls)
	require.Equal(t, 2000, result.Requested)
	require.Equal(t, 2000, result.Received)
	require.Equal(t, 2000, result.Added)
	require.Equal(t, 2200, result.Count)
	result, err = p.Fetch(context.Background(), 2000)
	require.NoError(t, err)
	require.Equal(t, 2000, result.Duplicates)
	require.Zero(t, result.Added)
	require.Equal(t, 2200, result.Count)
}

func TestOnesProxySecretsPersistWithoutBeingReturned(t *testing.T) {
	p := testProvider(t)
	status, err := p.Status()
	require.NoError(t, err)
	require.True(t, status.Configured)
	require.True(t, status.TokenConfigured)
	raw, err := json.Marshal(status)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "secret-token")
	status.Country = "GB"
	status.Route = "EU"
	_, err = p.Save(status.ProviderConfig)
	require.NoError(t, err)
	reloaded := NewOnesProxyProvider(p.path, p.pool)
	cfg, err := reloaded.read()
	require.NoError(t, err)
	require.Equal(t, "secret-token", cfg.Token)
	require.Equal(t, "GB", cfg.Country)
	info, err := os.Stat(p.path)
	require.NoError(t, err)
	require.Zero(t, info.Mode().Perm()&0077)
	cfg.Type = "extract"
	cfg.ExtractURL = onesProxyOrigin + onesProxyExtractPath + "?key=secret-key&index=7"
	status, err = p.Save(cfg)
	require.NoError(t, err)
	require.True(t, status.ExtractConfigured)
	raw, err = json.Marshal(status)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "secret-key")
	require.NotContains(t, string(raw), "secret-token")
}

func TestOnesProxyExtractLinkUsesRequestedCountAndActualResult(t *testing.T) {
	p := testProvider(t)
	cfg := defaultProviderConfig()
	cfg.Type = "extract"
	cfg.ExtractURL = onesProxyOrigin + onesProxyExtractPath + "?key=secret-key&index=7&num=1&return_type=text&unrelated=1"
	_, err := p.Save(cfg)
	require.NoError(t, err)
	p.client.Transport = providerTransport(func(req *http.Request) (*http.Response, error) {
		require.Equal(t, http.MethodGet, req.Method)
		require.Equal(t, "2000", req.URL.Query().Get("num"))
		require.Equal(t, "7", req.URL.Query().Get("index"))
		require.Equal(t, "json", req.URL.Query().Get("return_type"))
		require.Empty(t, req.URL.Query().Get("unrelated"))
		require.Empty(t, req.Header.Get("Token"))
		return providerResponse(200, `{"code":0,"data":["192.0.2.1:8080","192.0.2.1:8080","192.0.2.2:8080"]}`), nil
	})
	result, err := p.Fetch(context.Background(), 2000)
	require.NoError(t, err)
	require.Equal(t, 2000, result.Requested)
	require.Equal(t, 3, result.Received)
	require.Equal(t, 2, result.Added)
	require.Equal(t, 1, result.Duplicates)
}

func TestOnesProxyErrorsNeverChangePoolOrLeakSecrets(t *testing.T) {
	for _, body := range []string{`{"code":401,"message":"secret-token secret-key"}`, `{"code":200,"data":{"results":[]}}`, `{"code":200,"data":{"results":["bad-proxy"]}}`, `not-json`} {
		t.Run(body, func(t *testing.T) {
			p := testProvider(t)
			importLines(t, p.pool, "192.0.2.1:80")
			p.client.Transport = providerTransport(func(*http.Request) (*http.Response, error) { return providerResponse(200, body), nil })
			_, err := p.Fetch(context.Background(), 2000)
			require.Error(t, err)
			require.NotContains(t, err.Error(), "secret-token")
			require.NotContains(t, err.Error(), "secret-key")
			status, err := p.pool.Status()
			require.NoError(t, err)
			require.Equal(t, 1, status.Count)
		})
	}
}

func TestOnesProxyRejectsForeignURLsAndRedirects(t *testing.T) {
	p := testProvider(t)
	for _, raw := range []string{"http://thirdapi.onesproxy.com/api/proxy/getProxyList?key=x", "https://evil.example/api/proxy/getProxyList?key=x", "https://thirdapi.onesproxy.com:444/api/proxy/getProxyList?key=x", "https://thirdapi.onesproxy.com/other?key=x", onesProxyOrigin + onesProxyExtractPath + "?key=x&index=0"} {
		cfg := defaultProviderConfig()
		cfg.Type = "extract"
		cfg.ExtractURL = raw
		_, err := p.Save(cfg)
		require.Error(t, err)
	}
	calls := 0
	p.client.Transport = providerTransport(func(*http.Request) (*http.Response, error) {
		calls++
		response := providerResponse(302, "")
		response.Header.Set("Location", "https://evil.example/")
		return response, nil
	})
	_, err := p.Fetch(context.Background(), 2000)
	require.Error(t, err)
	require.Equal(t, 1, calls)
}

func TestOnesProxyRejectsConcurrentFetchAndInvalidCounts(t *testing.T) {
	p := testProvider(t)
	started := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	p.client.Transport = providerTransport(func(*http.Request) (*http.Response, error) {
		close(started)
		<-release
		return providerResponse(200, `{"code":200,"data":{"results":["192.0.2.1:80"]}}`), nil
	})
	go func() { _, err := p.Fetch(context.Background(), 2000); done <- err }()
	<-started
	_, err := p.Fetch(context.Background(), 2000)
	require.ErrorContains(t, err, "正在提取")
	close(release)
	require.NoError(t, <-done)
	for _, n := range []int{0, -1, 2001} {
		_, err = p.Fetch(context.Background(), n)
		require.Error(t, err)
	}
}

func enableAutoRefill(t *testing.T, p *OnesProxyProvider) {
	t.Helper()
	cfg, err := p.read()
	require.NoError(t, err)
	cfg.AutoRefill = true
	_, err = p.Save(cfg)
	require.NoError(t, err)
}

func TestOnesProxyAutoRefills2000WhenLastProxyFails(t *testing.T) {
	p := testProvider(t)
	enableAutoRefill(t, p)
	p = NewOnesProxyProvider(p.path, p.pool) // The switch survives a restart.
	importLines(t, p.pool, "192.0.2.1:80")
	lease, _, err := p.pool.Acquire()
	require.NoError(t, err)
	removed, err := p.pool.Report(lease.ID, false)
	require.NoError(t, err)
	require.True(t, removed)
	lines := make([]string, 2000)
	for i := range lines {
		lines[i] = fmt.Sprintf("proxy-%d.example:8080", i)
	}
	raw, err := json.Marshal(map[string]any{"code": 200, "data": map[string]any{"results": lines}})
	require.NoError(t, err)
	calls := 0
	p.client.Transport = providerTransport(func(req *http.Request) (*http.Response, error) {
		calls++
		var body map[string]any
		require.NoError(t, json.NewDecoder(req.Body).Decode(&body))
		require.Equal(t, float64(2000), body["proxy_num"])
		return providerResponse(200, string(raw)), nil
	})
	result, err := p.RefillIfEmpty(context.Background())
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 2000, result.Added)
	require.Equal(t, 1, result.Removed)
	// No top-up while proxies remain, even after the throttle expires.
	p.nextAutoRefill = time.Time{}
	result, err = p.RefillIfEmpty(context.Background())
	require.NoError(t, err)
	require.Nil(t, result)
	require.Equal(t, 1, calls)
}

func TestOnesProxyAutoRefillSkipsDisabledUnmanagedAndUnconfiguredPools(t *testing.T) {
	for _, reason := range []string{"disabled", "unmanaged", "unconfigured", "cancelled"} {
		t.Run(reason, func(t *testing.T) {
			p := testProvider(t)
			require.NoError(t, p.pool.write(state{Version: 1}))
			if reason != "disabled" {
				enableAutoRefill(t, p)
			}
			if reason == "unmanaged" {
				require.NoError(t, os.Remove(p.pool.path))
			}
			if reason == "unconfigured" {
				require.NoError(t, os.WriteFile(p.path, []byte(`{"type":"generate","auto_refill":true}`), 0600))
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if reason == "cancelled" {
				cancel()
			}
			calls := 0
			p.client.Transport = providerTransport(func(*http.Request) (*http.Response, error) {
				calls++
				return providerResponse(500, ""), nil
			})
			result, err := p.RefillIfEmpty(ctx)
			require.NoError(t, err)
			require.Nil(t, result)
			require.Zero(t, calls)
		})
	}
}

func TestOnesProxyAutoRefillRetriesAfterCooldown(t *testing.T) {
	p := testProvider(t)
	enableAutoRefill(t, p)
	require.NoError(t, p.pool.write(state{Version: 1}))
	calls := 0
	p.client.Transport = providerTransport(func(*http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return providerResponse(200, `{"code":401,"message":"secret-token"}`), nil
		}
		return providerResponse(200, `{"code":200,"data":{"results":["192.0.2.1:8080"]}}`), nil
	})
	result, err := p.RefillIfEmpty(context.Background())
	require.Error(t, err)
	require.NotContains(t, err.Error(), "secret-token")
	require.Nil(t, result)
	require.WithinDuration(t, time.Now().Add(time.Minute), p.nextAutoRefill, time.Second)
	for i := 0; i < 5; i++ {
		result, err = p.RefillIfEmpty(context.Background())
		require.NoError(t, err)
		require.Nil(t, result)
	}
	require.Equal(t, 1, calls)
	p.nextAutoRefill = time.Time{}
	result, err = p.RefillIfEmpty(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, result.Added)
	// Even short successful batches cannot trigger a tight refill loop.
	lease, _, err := p.pool.Acquire()
	require.NoError(t, err)
	_, err = p.pool.Report(lease.ID, false)
	require.NoError(t, err)
	result, err = p.RefillIfEmpty(context.Background())
	require.NoError(t, err)
	require.Nil(t, result)
	require.Equal(t, 2, calls)
	p.nextAutoRefill = time.Time{}
	result, err = p.RefillIfEmpty(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, result.Added)
	require.Equal(t, 3, calls)
}

func TestOnesProxyAutomaticAndManualFetchShareLock(t *testing.T) {
	for _, automatic := range []bool{false, true} {
		t.Run(fmt.Sprint(automatic), func(t *testing.T) {
			p := testProvider(t)
			enableAutoRefill(t, p)
			require.NoError(t, p.pool.write(state{Version: 1}))
			started, release := make(chan struct{}), make(chan struct{})
			done := make(chan error, 1)
			var calls atomic.Int32
			p.client.Transport = providerTransport(func(*http.Request) (*http.Response, error) {
				calls.Add(1)
				close(started)
				<-release
				return providerResponse(200, `{"code":200,"data":{"results":["192.0.2.1:8080"]}}`), nil
			})
			go func() {
				if automatic {
					_, err := p.RefillIfEmpty(context.Background())
					done <- err
				} else {
					_, err := p.Fetch(context.Background(), 2000)
					done <- err
				}
			}()
			<-started
			result, autoErr := p.RefillIfEmpty(context.Background())
			_, manualErr := p.Fetch(context.Background(), 2000)
			close(release)
			require.NoError(t, <-done)
			require.NoError(t, autoErr)
			require.Nil(t, result)
			require.ErrorContains(t, manualErr, "正在提取")
			require.EqualValues(t, 1, calls.Load())
		})
	}
}

func TestOnesProxyAutoRefillCancellationDoesNotImport(t *testing.T) {
	p := testProvider(t)
	enableAutoRefill(t, p)
	require.NoError(t, p.pool.write(state{Version: 1}))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p.client.Transport = providerTransport(func(req *http.Request) (*http.Response, error) {
		cancel()
		return providerResponse(200, `{"code":200,"data":{"results":["192.0.2.1:8080"]}}`), nil
	})
	result, err := p.RefillIfEmpty(ctx)
	require.Error(t, err)
	require.Nil(t, result)
	status, err := p.pool.Status()
	require.NoError(t, err)
	require.Zero(t, status.Count)
}
