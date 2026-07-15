package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// fakeProxyLookup implements just the GetByID slice of ProxyRepository that the
// runtime fallback resolver needs; the embedded interface is nil so any other
// method call would panic (and thus surface an unexpected dependency).
type fakeProxyLookup struct {
	ProxyRepository
	byID map[int64]*Proxy
}

func (f *fakeProxyLookup) GetByID(_ context.Context, id int64) (*Proxy, error) {
	return f.byID[id], nil
}

func i64(v int64) *int64 { return &v }

func proxyForTest(id int64, host string, port int) *Proxy {
	return &Proxy{ID: id, Protocol: "http", Host: host, Port: port, Status: StatusActive}
}

func TestResolveRuntimeProxyFallbackURL(t *testing.T) {
	t.Run("nil account or proxy", func(t *testing.T) {
		s := &OpenAIGatewayService{}
		_, _, ok := s.resolveRuntimeProxyFallbackURL(context.Background(), nil)
		require.False(t, ok)
		_, _, ok = s.resolveRuntimeProxyFallbackURL(context.Background(), &Account{})
		require.False(t, ok)
	})

	t.Run("mode none has no fallback", func(t *testing.T) {
		s := &OpenAIGatewayService{}
		primary := proxyForTest(1, "10.0.0.1", 7890)
		primary.FallbackMode = FallbackModeNone
		_, _, ok := s.resolveRuntimeProxyFallbackURL(context.Background(), &Account{Proxy: primary})
		require.False(t, ok)
	})

	t.Run("direct mode returns empty url", func(t *testing.T) {
		s := &OpenAIGatewayService{}
		primary := proxyForTest(1, "10.0.0.1", 7890)
		primary.FallbackMode = FallbackModeDirect
		url, label, ok := s.resolveRuntimeProxyFallbackURL(context.Background(), &Account{Proxy: primary})
		require.True(t, ok)
		require.Equal(t, "", url)
		require.Equal(t, "direct", label)
	})

	t.Run("proxy mode without repo cannot resolve", func(t *testing.T) {
		s := &OpenAIGatewayService{}
		primary := proxyForTest(1, "10.0.0.1", 7890)
		primary.FallbackMode = FallbackModeProxy
		primary.BackupProxyID = i64(2)
		_, _, ok := s.resolveRuntimeProxyFallbackURL(context.Background(), &Account{Proxy: primary})
		require.False(t, ok)
	})

	t.Run("proxy mode returns active backup url", func(t *testing.T) {
		backup := proxyForTest(2, "10.0.0.2", 7892)
		s := &OpenAIGatewayService{proxyRepo: &fakeProxyLookup{byID: map[int64]*Proxy{2: backup}}}
		primary := proxyForTest(1, "10.0.0.1", 7890)
		primary.FallbackMode = FallbackModeProxy
		primary.BackupProxyID = i64(2)
		url, label, ok := s.resolveRuntimeProxyFallbackURL(context.Background(), &Account{Proxy: primary})
		require.True(t, ok)
		require.Equal(t, "http://10.0.0.2:7892", url)
		require.Equal(t, "10.0.0.2:7892", label)
	})

	t.Run("proxy mode without backup id has no fallback", func(t *testing.T) {
		s := &OpenAIGatewayService{proxyRepo: &fakeProxyLookup{byID: map[int64]*Proxy{}}}
		primary := proxyForTest(1, "10.0.0.1", 7890)
		primary.FallbackMode = FallbackModeProxy
		_, _, ok := s.resolveRuntimeProxyFallbackURL(context.Background(), &Account{Proxy: primary})
		require.False(t, ok)
	})

	t.Run("walks past an unusable backup to the next active one", func(t *testing.T) {
		expired := proxyForTest(2, "10.0.0.2", 7892)
		past := time.Now().Add(-time.Hour)
		expired.ExpiresAt = &past
		expired.FallbackMode = FallbackModeProxy
		expired.BackupProxyID = i64(3)
		healthy := proxyForTest(3, "10.0.0.3", 7894)
		s := &OpenAIGatewayService{proxyRepo: &fakeProxyLookup{byID: map[int64]*Proxy{2: expired, 3: healthy}}}
		primary := proxyForTest(1, "10.0.0.1", 7890)
		primary.FallbackMode = FallbackModeProxy
		primary.BackupProxyID = i64(2)
		url, _, ok := s.resolveRuntimeProxyFallbackURL(context.Background(), &Account{Proxy: primary})
		require.True(t, ok)
		require.Equal(t, "http://10.0.0.3:7894", url)
	})

	t.Run("cycle is broken and yields no fallback", func(t *testing.T) {
		a := proxyForTest(2, "10.0.0.2", 7892)
		a.Status = StatusDisabled
		a.FallbackMode = FallbackModeProxy
		a.BackupProxyID = i64(1) // points back to the primary
		s := &OpenAIGatewayService{proxyRepo: &fakeProxyLookup{byID: map[int64]*Proxy{2: a}}}
		primary := proxyForTest(1, "10.0.0.1", 7890)
		primary.FallbackMode = FallbackModeProxy
		primary.BackupProxyID = i64(2)
		_, _, ok := s.resolveRuntimeProxyFallbackURL(context.Background(), &Account{Proxy: primary})
		require.False(t, ok)
	})
}

func TestCloneUpstreamRequestForRetry(t *testing.T) {
	body := []byte(`{"model":"gpt-5","input":"hi"}`)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "https://example.com/v1/responses", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer secret")
	require.NotNil(t, req.GetBody, "bytes body must expose GetBody so retries are safe")

	clone, err := cloneUpstreamRequestForRetry(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, req.Method, clone.Method)
	require.Equal(t, req.URL.String(), clone.URL.String())
	require.Equal(t, "Bearer secret", clone.Header.Get("Authorization"))

	got, err := io.ReadAll(clone.Body)
	require.NoError(t, err)
	require.Equal(t, body, got)

	// Original request body must still be independently readable.
	orig, err := io.ReadAll(req.Body)
	require.NoError(t, err)
	require.Equal(t, body, orig)
}
