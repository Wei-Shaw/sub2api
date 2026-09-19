package repository

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type pricingRoundTripper func(*http.Request) (*http.Response, error)

func (f pricingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type pricingProxyListerStub struct {
	proxies []service.Proxy
	calls   int
}

func (s *pricingProxyListerStub) ListActive(context.Context) ([]service.Proxy, error) {
	s.calls++
	return s.proxies, nil
}

type pricingLatencyReaderStub struct {
	latencies map[int64]*service.ProxyLatencyInfo
}

func (s *pricingLatencyReaderStub) GetProxyLatencies(context.Context, []int64) (map[int64]*service.ProxyLatencyInfo, error) {
	return s.latencies, nil
}

func newPricingTestHTTPClient(fn pricingRoundTripper) *http.Client {
	return &http.Client{Transport: fn}
}

func pricingTestResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}
}

type PricingServiceSuite struct {
	suite.Suite
	ctx    context.Context
	srv    *httptest.Server
	client *pricingRemoteClient
}

func (s *PricingServiceSuite) SetupTest() {
	s.ctx = context.Background()
	client, ok := NewPricingRemoteClient("", false).(*pricingRemoteClient)
	require.True(s.T(), ok, "type assertion failed")
	s.client = client
}

func (s *PricingServiceSuite) TearDownTest() {
	if s.srv != nil {
		s.srv.Close()
		s.srv = nil
	}
}

func (s *PricingServiceSuite) setupServer(handler http.HandlerFunc) {
	s.srv = newLocalTestServer(s.T(), handler)
}

func (s *PricingServiceSuite) TestFetchPricingJSON_Success() {
	s.setupServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/ok" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true}`))
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))

	body, err := s.client.FetchPricingJSON(s.ctx, s.srv.URL+"/ok")
	require.NoError(s.T(), err, "FetchPricingJSON")
	require.Equal(s.T(), `{"ok":true}`, string(body), "body mismatch")
}

func (s *PricingServiceSuite) TestFetchPricingJSON_NonOKStatus() {
	s.setupServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))

	_, err := s.client.FetchPricingJSON(s.ctx, s.srv.URL+"/err")
	require.Error(s.T(), err, "expected error for non-200 status")
}

func (s *PricingServiceSuite) TestFetchHashText_ParsesFields() {
	s.setupServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/hashfile":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("abc123  model_prices.json\n"))
		case "/hashonly":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("def456\n"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))

	hash, err := s.client.FetchHashText(s.ctx, s.srv.URL+"/hashfile")
	require.NoError(s.T(), err, "FetchHashText")
	require.Equal(s.T(), "abc123", hash, "hash mismatch")

	hash2, err := s.client.FetchHashText(s.ctx, s.srv.URL+"/hashonly")
	require.NoError(s.T(), err, "FetchHashText")
	require.Equal(s.T(), "def456", hash2, "hash mismatch")
}

func (s *PricingServiceSuite) TestFetchHashText_NonOKStatus() {
	s.setupServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))

	_, err := s.client.FetchHashText(s.ctx, s.srv.URL+"/nope")
	require.Error(s.T(), err, "expected error for non-200 status")
}

func (s *PricingServiceSuite) TestFetchPricingJSON_InvalidURL() {
	_, err := s.client.FetchPricingJSON(s.ctx, "://invalid-url")
	require.Error(s.T(), err, "expected error for invalid URL")
}

func (s *PricingServiceSuite) TestFetchHashText_EmptyBody() {
	s.setupServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		// empty body
	}))

	hash, err := s.client.FetchHashText(s.ctx, s.srv.URL+"/empty")
	require.NoError(s.T(), err, "FetchHashText empty body should not error")
	require.Equal(s.T(), "", hash, "expected empty hash")
}

func (s *PricingServiceSuite) TestFetchHashText_WhitespaceOnly() {
	s.setupServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("   \n"))
	}))

	hash, err := s.client.FetchHashText(s.ctx, s.srv.URL+"/ws")
	require.NoError(s.T(), err, "FetchHashText whitespace body should not error")
	require.Equal(s.T(), "", hash, "expected empty hash after trimming")
}

func (s *PricingServiceSuite) TestFetchPricingJSON_ContextCancel() {
	started := make(chan struct{})
	s.setupServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
	}))

	ctx, cancel := context.WithCancel(s.ctx)

	done := make(chan error, 1)
	go func() {
		_, err := s.client.FetchPricingJSON(ctx, s.srv.URL+"/block")
		done <- err
	}()

	<-started
	cancel()

	err := <-done
	require.Error(s.T(), err)
}

func TestNewPricingRemoteClient_InvalidProxy_NoFallback(t *testing.T) {
	client := NewPricingRemoteClient("://bad", false)
	_, ok := client.(*pricingRemoteClientError)
	require.True(t, ok, "should return error client when proxy is invalid and fallback disabled")

	_, err := client.FetchPricingJSON(context.Background(), "http://example.com")
	require.Error(t, err)
	require.Contains(t, err.Error(), "proxy client init failed")
}

func TestNewPricingRemoteClient_InvalidProxy_WithFallback(t *testing.T) {
	client := NewPricingRemoteClient("://bad", true)
	_, ok := client.(*pricingRemoteClient)
	require.True(t, ok, "should fallback to direct client when allowed")
}

func TestPricingRemoteClient_ManagedProxySelectsByLatencyAndReusesWinner(t *testing.T) {
	now := time.Now()
	latency := func(value int64) *int64 { return &value }
	proxyRepo := &pricingProxyListerStub{proxies: []service.Proxy{
		{ID: 1, Name: "slow", Protocol: "http", Host: "proxy-1", Port: 8001, Status: service.StatusActive},
		{ID: 2, Name: "fast-failing", Protocol: "http", Host: "proxy-2", Port: 8002, Status: service.StatusActive},
		{ID: 3, Name: "winner", Protocol: "http", Host: "proxy-3", Port: 8003, Status: service.StatusActive},
	}}
	latencyCache := &pricingLatencyReaderStub{latencies: map[int64]*service.ProxyLatencyInfo{
		1: {Success: true, LatencyMs: latency(80), UpdatedAt: now},
		2: {Success: true, LatencyMs: latency(10), UpdatedAt: now},
		3: {Success: true, LatencyMs: latency(20), UpdatedAt: now},
	}}

	var calls []string
	client := &pricingRemoteClient{
		httpClient: newPricingTestHTTPClient(func(*http.Request) (*http.Response, error) {
			calls = append(calls, "direct")
			return nil, errors.New("direct unavailable")
		}),
		proxyRepo:    proxyRepo,
		latencyCache: latencyCache,
		clientFactory: func(proxyURL string) (*http.Client, error) {
			return newPricingTestHTTPClient(func(*http.Request) (*http.Response, error) {
				switch {
				case strings.Contains(proxyURL, "proxy-2"):
					calls = append(calls, "proxy-2")
					return pricingTestResponse(http.StatusBadGateway, ""), nil
				case strings.Contains(proxyURL, "proxy-3"):
					calls = append(calls, "proxy-3")
					return pricingTestResponse(http.StatusOK, "selected"), nil
				default:
					calls = append(calls, "proxy-1")
					return pricingTestResponse(http.StatusOK, "unexpected"), nil
				}
			}), nil
		},
	}

	body, err := client.FetchPricingJSON(context.Background(), "https://raw.githubusercontent.com/pricing.json")
	require.NoError(t, err)
	require.Equal(t, "selected", string(body))
	require.Equal(t, []string{"direct", "proxy-2", "proxy-3"}, calls)
	require.Equal(t, int64(3), client.selectedProxyID)
	require.Equal(t, 1, proxyRepo.calls)

	body, err = client.FetchPricingJSON(context.Background(), "https://raw.githubusercontent.com/pricing.json")
	require.NoError(t, err)
	require.Equal(t, "selected", string(body))
	require.Equal(t, []string{"direct", "proxy-2", "proxy-3", "proxy-3"}, calls)
	require.Equal(t, 1, proxyRepo.calls, "selected proxy should be reused without querying candidates again")
}

func TestPricingRemoteClient_ManagedProxyTriesAtMostThreeCandidates(t *testing.T) {
	now := time.Now()
	latency := func(value int64) *int64 { return &value }
	proxyRepo := &pricingProxyListerStub{proxies: []service.Proxy{
		{ID: 1, Protocol: "http", Host: "proxy-1", Port: 8001, Status: service.StatusActive},
		{ID: 2, Protocol: "http", Host: "proxy-2", Port: 8002, Status: service.StatusActive},
		{ID: 3, Protocol: "http", Host: "proxy-3", Port: 8003, Status: service.StatusActive},
		{ID: 4, Protocol: "http", Host: "proxy-4", Port: 8004, Status: service.StatusActive},
	}}
	latencyCache := &pricingLatencyReaderStub{latencies: map[int64]*service.ProxyLatencyInfo{
		1: {Success: true, LatencyMs: latency(40), UpdatedAt: now},
		2: {Success: true, LatencyMs: latency(10), UpdatedAt: now},
		3: {Success: true, LatencyMs: latency(20), UpdatedAt: now},
		4: {Success: true, LatencyMs: latency(30), UpdatedAt: now},
	}}

	var calls []string
	client := &pricingRemoteClient{
		httpClient: newPricingTestHTTPClient(func(*http.Request) (*http.Response, error) {
			return nil, errors.New("direct unavailable")
		}),
		proxyRepo:    proxyRepo,
		latencyCache: latencyCache,
		clientFactory: func(proxyURL string) (*http.Client, error) {
			return newPricingTestHTTPClient(func(*http.Request) (*http.Response, error) {
				calls = append(calls, proxyURL)
				return nil, errors.New("proxy unavailable")
			}), nil
		},
	}

	_, err := client.FetchHashText(context.Background(), "https://raw.githubusercontent.com/pricing.sha256")
	require.Error(t, err)
	require.Len(t, calls, 3)
	require.Contains(t, calls[0], "proxy-2")
	require.Contains(t, calls[1], "proxy-3")
	require.Contains(t, calls[2], "proxy-4")
}

func TestPricingRemoteClient_ManagedProxyFiltersInvalidCandidates(t *testing.T) {
	now := time.Now()
	past := now.Add(-time.Minute)
	latency := func(value int64) *int64 { return &value }
	proxyRepo := &pricingProxyListerStub{proxies: []service.Proxy{
		{ID: 1, Protocol: "http", Host: "inactive", Port: 8001, Status: service.StatusDisabled},
		{ID: 2, Protocol: "http", Host: "expired", Port: 8002, Status: service.StatusActive, ExpiresAt: &past},
		{ID: 3, Protocol: "http", Host: "last-test-failed", Port: 8003, Status: service.StatusActive},
		{ID: 4, Protocol: "http", Host: "valid", Port: 8004, Status: service.StatusActive},
	}}
	latencyCache := &pricingLatencyReaderStub{latencies: map[int64]*service.ProxyLatencyInfo{
		1: {Success: true, LatencyMs: latency(1), UpdatedAt: now},
		2: {Success: true, LatencyMs: latency(2), UpdatedAt: now},
		3: {Success: false, LatencyMs: latency(3), UpdatedAt: now},
		4: {Success: true, LatencyMs: latency(4), UpdatedAt: now.Add(-24 * time.Hour)},
	}}

	client := &pricingRemoteClient{proxyRepo: proxyRepo, latencyCache: latencyCache}
	candidates, err := client.listManagedProxyCandidates(context.Background(), nil)
	require.NoError(t, err)
	require.Len(t, candidates, 1)
	require.Equal(t, int64(4), candidates[0].proxy.ID)
}

func TestPricingRemoteClient_SelectedProxyFailureReturnsToDirect(t *testing.T) {
	directAvailable := false
	directClient := newPricingTestHTTPClient(func(*http.Request) (*http.Response, error) {
		if directAvailable {
			return pricingTestResponse(http.StatusOK, "direct-restored"), nil
		}
		return nil, errors.New("direct unavailable")
	})
	selectedClient := newPricingTestHTTPClient(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("selected proxy unavailable")
	})
	client := &pricingRemoteClient{
		httpClient:      directClient,
		proxyRepo:       &pricingProxyListerStub{},
		latencyCache:    &pricingLatencyReaderStub{},
		clientFactory:   func(string) (*http.Client, error) { return nil, errors.New("unexpected") },
		selectedClient:  selectedClient,
		selectedProxyID: 9,
	}

	directAvailable = true
	body, err := client.FetchPricingJSON(context.Background(), "https://raw.githubusercontent.com/pricing.json")
	require.NoError(t, err)
	require.Equal(t, "direct-restored", string(body))
	require.Nil(t, client.selectedClient)
	require.Zero(t, client.selectedProxyID)
}

func TestPricingRemoteClient_DoesNotFallbackForNonRetryableStatus(t *testing.T) {
	proxyRepo := &pricingProxyListerStub{}
	client := &pricingRemoteClient{
		httpClient: newPricingTestHTTPClient(func(*http.Request) (*http.Response, error) {
			return pricingTestResponse(http.StatusNotFound, ""), nil
		}),
		proxyRepo:     proxyRepo,
		latencyCache:  &pricingLatencyReaderStub{},
		clientFactory: func(string) (*http.Client, error) { return nil, errors.New("unexpected") },
	}

	_, err := client.FetchPricingJSON(context.Background(), "https://raw.githubusercontent.com/missing.json")
	require.Error(t, err)
	require.Equal(t, 0, proxyRepo.calls)
}

func TestPricingServiceSuite(t *testing.T) {
	suite.Run(t, new(PricingServiceSuite))
}
