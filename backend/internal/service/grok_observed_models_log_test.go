//go:build unit

package service

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/stretchr/testify/require"
)

type grokCatalogLogProxyRepo struct {
	ProxyRepository
	proxy *Proxy
}

func (r grokCatalogLogProxyRepo) GetByID(context.Context, int64) (*Proxy, error) {
	return r.proxy, nil
}

type grokCatalogLoggingUpstream struct {
	proxyURL string
	request  *http.Request
}

func (u *grokCatalogLoggingUpstream) Do(req *http.Request, proxyURL string, _ int64, _ int) (*http.Response, error) {
	u.proxyURL = proxyURL
	u.request = req
	// net/http puts the request URL on *url.Error. Include query and userinfo
	// here because a custom endpoint can carry both, while the proxy failure is
	// attached separately and may contain proxy credentials.
	return nil, &url.Error{
		Op:  "Get",
		URL: "https://relay-user:query-userinfo-secret@relay.example.test/v1/models?api_key=review-query-marker",
		Err: fmt.Errorf("proxy %s: dial failed", proxyURL),
	}
}

func (u *grokCatalogLoggingUpstream) DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, proxyURL, accountID, accountConcurrency)
}

type grokCatalogLogHandler struct {
	inner slog.Handler
	done  chan struct{}
}

func (h *grokCatalogLogHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

func (h *grokCatalogLogHandler) Handle(ctx context.Context, record slog.Record) error {
	err := h.inner.Handle(ctx, record)
	if record.Message == "grok_observed_models_sync_failed" {
		select {
		case h.done <- struct{}{}:
		default:
		}
	}
	return err
}

func (h *grokCatalogLogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &grokCatalogLogHandler{inner: h.inner.WithAttrs(attrs), done: h.done}
}

func (h *grokCatalogLogHandler) WithGroup(name string) slog.Handler {
	return &grokCatalogLogHandler{inner: h.inner.WithGroup(name), done: h.done}
}

func TestGrokBackgroundCatalogLogDoesNotExpandTransportURL(t *testing.T) {
	var buf bytes.Buffer
	handler := &grokCatalogLogHandler{
		inner: slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}),
		done:  make(chan struct{}, 1),
	}
	previous := slog.Default()
	slog.SetDefault(slog.New(handler))
	t.Cleanup(func() { slog.SetDefault(previous) })

	account := grokOAuthModelSyncTestAccount("https://relay-user:query-userinfo-secret@relay.example.test/v1?api_key=review-query-marker")
	account.ID = 917283
	proxyID := int64(7)
	account.ProxyID = &proxyID
	proxy := &Proxy{
		ID:       proxyID,
		Protocol: "http",
		Host:     "proxy.example.test",
		Port:     3128,
		Username: "proxy-user",
		Password: "proxy-pass-marker",
		Status:   StatusActive,
	}
	repo := newGrokCatalogMemoryRepo(account)
	upstream := &grokCatalogLoggingUpstream{}
	svc := &GrokQuotaService{
		accountRepo:  repo,
		proxyRepo:    grokCatalogLogProxyRepo{proxy: proxy},
		httpUpstream: upstream,
		cfg:          upstreamModelSyncTestConfig(),
	}

	svc.scheduleGrokObservedModelsSync(account)
	select {
	case <-handler.done:
	case <-time.After(5 * time.Second):
		t.Fatal("background catalog log missing")
	}

	logged := buf.String()
	require.NotContains(t, logged, "review-query-marker")
	require.NotContains(t, logged, "query-userinfo-secret")
	require.NotContains(t, logged, "proxy-pass-marker")
	require.NotContains(t, logged, "relay-user")
	require.Contains(t, logged, "account_id=917283")
	require.Contains(t, logged, "error_kind=upstream")
	require.Contains(t, logged, "status_code=0")
	require.NotContains(t, logged, "error=")
	require.NotNil(t, upstream.request)
	require.Contains(t, upstream.proxyURL, "proxy-pass-marker")

	stored, err := repo.GetByID(context.Background(), account.ID)
	require.NoError(t, err)
	snapshot := parseGrokObservedModels(stored.Extra)
	require.NotNil(t, snapshot)
	require.NotNil(t, snapshot.LastAttempt)
	require.Equal(t, grokCatalogStatusFailed, snapshot.LastAttempt.Status)
	require.Equal(t, "transport", snapshot.LastAttempt.ErrorCode)
	require.Empty(t, snapshot.Models)
}
