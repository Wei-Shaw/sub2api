package websearch

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNewManager_PreservesOrder(t *testing.T) {
	configs := []ProviderConfig{
		{Type: "brave", APIKey: "k3"},
		{Type: "tavily", APIKey: "k1"},
	}
	m := NewManager(configs, nil)
	require.Equal(t, "brave", m.configs[0].Type)
	require.Equal(t, "tavily", m.configs[1].Type)
}

func TestManager_SearchWithBestProvider_EmptyQuery(t *testing.T) {
	m := NewManager([]ProviderConfig{{Type: "brave", APIKey: "k"}}, nil)
	_, _, err := m.SearchWithBestProvider(context.Background(), SearchRequest{Query: ""})
	require.ErrorContains(t, err, "empty search query")

	_, _, err = m.SearchWithBestProvider(context.Background(), SearchRequest{Query: "   "})
	require.ErrorContains(t, err, "empty search query")
}

func TestManager_SearchWithBestProvider_SkipEmptyAPIKey(t *testing.T) {
	m := NewManager([]ProviderConfig{{Type: "brave", APIKey: ""}}, nil)
	_, _, err := m.SearchWithBestProvider(context.Background(), SearchRequest{Query: "test"})
	require.ErrorContains(t, err, "no available provider")
}

func TestManager_SearchWithBestProvider_SkipExpired(t *testing.T) {
	past := time.Now().Add(-1 * time.Hour).Unix()
	m := NewManager([]ProviderConfig{
		{Type: "brave", APIKey: "k", ExpiresAt: &past},
	}, nil)
	_, _, err := m.SearchWithBestProvider(context.Background(), SearchRequest{Query: "test"})
	require.ErrorContains(t, err, "no available provider")
}

func TestManager_SearchWithBestProvider_UsesFirstAvailable(t *testing.T) {
	srvBrave := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := braveResponse{}
		resp.Web.Results = []braveResult{{URL: "https://brave.com", Title: "Brave", Description: "from brave"}}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srvBrave.Close()

	origURL := *braveSearchURL
	u, _ := http.NewRequest("GET", srvBrave.URL, nil)
	*braveSearchURL = *u.URL
	defer func() { *braveSearchURL = origURL }()

	m := NewManager([]ProviderConfig{
		{Type: "brave", APIKey: "k1"},
		{Type: "tavily", APIKey: "k2"},
	}, nil)
	m.clientCache[srvBrave.URL] = srvBrave.Client()
	m.clientCache[""] = srvBrave.Client()

	resp, providerName, err := m.SearchWithBestProvider(context.Background(), SearchRequest{Query: "test"})
	require.NoError(t, err)
	require.Equal(t, "brave", providerName)
	require.Len(t, resp.Results, 1)
	require.Equal(t, "from brave", resp.Results[0].Snippet)
}

func TestManager_SearchWithBestProvider_NilRedis(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := braveResponse{}
		resp.Web.Results = []braveResult{{URL: "https://test.com", Title: "Test", Description: "result"}}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	origURL := *braveSearchURL
	u, _ := http.NewRequest("GET", srv.URL, nil)
	*braveSearchURL = *u.URL
	defer func() { *braveSearchURL = origURL }()

	m := NewManager([]ProviderConfig{
		{Type: "brave", APIKey: "k", QuotaLimit: 100},
	}, nil)
	m.clientCache[""] = srv.Client()

	resp, _, err := m.SearchWithBestProvider(context.Background(), SearchRequest{Query: "test"})
	require.NoError(t, err)
	require.Len(t, resp.Results, 1)
}

func TestManager_GetUsage_NilRedis(t *testing.T) {
	m := NewManager(nil, nil)
	used, err := m.GetUsage(context.Background(), "brave")
	require.NoError(t, err)
	require.Equal(t, int64(0), used)
}

func TestManager_GetAllUsage_NilRedis(t *testing.T) {
	m := NewManager([]ProviderConfig{
		{Type: "brave"},
	}, nil)
	usage := m.GetAllUsage(context.Background())
	require.Equal(t, int64(0), usage["brave"])
}

// --- Quota TTL from subscription ---

func TestQuotaTTLFromSubscription_NilSubscription(t *testing.T) {
	ttl := quotaTTLFromSubscription(nil)
	require.Equal(t, defaultQuotaTTL, ttl)
}

func TestQuotaTTLFromSubscription_ZeroSubscription(t *testing.T) {
	zero := int64(0)
	ttl := quotaTTLFromSubscription(&zero)
	require.Equal(t, defaultQuotaTTL, ttl)
}

func TestQuotaTTLFromSubscription_ValidSubscription(t *testing.T) {
	// Subscribed 10 days ago — next reset in ~20 days
	sub := time.Now().Add(-10 * 24 * time.Hour).Unix()
	ttl := quotaTTLFromSubscription(&sub)
	require.Greater(t, ttl, 15*24*time.Hour) // at least 15 days
	require.Less(t, ttl, 25*24*time.Hour+quotaTTLBuffer)
}

func TestNextMonthlyReset_SubscribedRecentPast(t *testing.T) {
	// Subscribed on the 10th of this month (always valid day)
	now := time.Now().UTC()
	sub := time.Date(now.Year(), now.Month(), 10, 0, 0, 0, 0, time.UTC)
	next := nextMonthlyReset(sub)
	require.True(t, next.After(now) || next.Equal(now), "next reset should be in the future or now")
	require.True(t, next.Before(now.AddDate(0, 1, 1)))
}

func TestNextMonthlyReset_SubscribedLongAgo(t *testing.T) {
	// Subscribed 6 months ago on the 1st
	sub := time.Now().UTC().AddDate(0, -6, 0)
	sub = time.Date(sub.Year(), sub.Month(), 1, 0, 0, 0, 0, time.UTC)
	next := nextMonthlyReset(sub)
	require.True(t, next.After(time.Now().UTC()))
	// Should be within the next 31 days
	require.True(t, next.Before(time.Now().UTC().AddDate(0, 1, 1)))
}

func TestNextMonthlyReset_FutureSubscription(t *testing.T) {
	sub := time.Now().UTC().AddDate(0, 0, 5)
	next := nextMonthlyReset(sub)
	require.True(t, next.After(time.Now().UTC()))
}

func TestAddMonthsClamped_Jan31ToFeb(t *testing.T) {
	sub := time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC)
	next := addMonthsClamped(sub, 1)
	require.Equal(t, time.Month(2), next.Month())
	require.Equal(t, 28, next.Day()) // Feb 28 (2026 is not a leap year)
}

func TestAddMonthsClamped_Jan31ToFebLeapYear(t *testing.T) {
	sub := time.Date(2028, 1, 31, 0, 0, 0, 0, time.UTC)
	next := addMonthsClamped(sub, 1)
	require.Equal(t, time.Month(2), next.Month())
	require.Equal(t, 29, next.Day()) // Feb 29 (2028 is a leap year)
}

func TestAddMonthsClamped_Mar31ToApr(t *testing.T) {
	sub := time.Date(2026, 3, 31, 0, 0, 0, 0, time.UTC)
	next := addMonthsClamped(sub, 1)
	require.Equal(t, time.Month(4), next.Month())
	require.Equal(t, 30, next.Day()) // Apr has 30 days
}

func TestAddMonthsClamped_NormalDay(t *testing.T) {
	sub := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	next := addMonthsClamped(sub, 1)
	require.Equal(t, time.Month(2), next.Month())
	require.Equal(t, 15, next.Day()) // no clamping needed
}

// --- Redis key ---

func TestQuotaRedisKey_Format(t *testing.T) {
	key := quotaRedisKey("brave")
	require.Equal(t, "websearch:quota:brave", key)
}

// --- isProviderAvailable ---

func TestIsProviderAvailable_EmptyAPIKey(t *testing.T) {
	m := NewManager(nil, nil)
	require.False(t, m.isProviderAvailable(ProviderConfig{APIKey: ""}))
}

func TestIsProviderAvailable_Expired(t *testing.T) {
	m := NewManager(nil, nil)
	past := time.Now().Add(-1 * time.Hour).Unix()
	require.False(t, m.isProviderAvailable(ProviderConfig{APIKey: "k", ExpiresAt: &past}))
}

func TestIsProviderAvailable_Valid(t *testing.T) {
	m := NewManager(nil, nil)
	future := time.Now().Add(1 * time.Hour).Unix()
	require.True(t, m.isProviderAvailable(ProviderConfig{APIKey: "k", ExpiresAt: &future}))
	require.True(t, m.isProviderAvailable(ProviderConfig{APIKey: "k"})) // no expiry
}

// --- resolveProxyID ---

func TestResolveProxyID_AccountProxyOverrides(t *testing.T) {
	cfg := ProviderConfig{ProxyID: 42}
	require.Equal(t, int64(0), resolveProxyID(cfg, "http://account-proxy:8080"))
	require.Equal(t, int64(42), resolveProxyID(cfg, ""))
}

// --- isProxyError ---

func TestIsProxyError_Nil(t *testing.T) {
	require.False(t, isProxyError(nil))
}

func TestIsProxyError_ConnectionRefused(t *testing.T) {
	require.True(t, isProxyError(fmt.Errorf("dial tcp: connection refused")))
}

func TestIsProxyError_Timeout(t *testing.T) {
	require.True(t, isProxyError(fmt.Errorf("i/o timeout while connecting to proxy")))
}

func TestIsProxyError_SOCKS(t *testing.T) {
	require.True(t, isProxyError(fmt.Errorf("socks connect failed")))
}

func TestIsProxyError_TLSHandshake(t *testing.T) {
	require.True(t, isProxyError(fmt.Errorf("tls handshake timeout")))
}

func TestIsProxyError_APIError_NotProxy(t *testing.T) {
	require.False(t, isProxyError(fmt.Errorf("API rate limit exceeded")))
}

// --- isProxyAvailable (nil Redis) ---

func TestIsProxyAvailable_NilRedis(t *testing.T) {
	m := NewManager(nil, nil)
	require.True(t, m.isProxyAvailable(context.Background(), 42))
}

func TestIsProxyAvailable_ZeroID(t *testing.T) {
	m := NewManager(nil, nil)
	require.True(t, m.isProxyAvailable(context.Background(), 0))
}

// --- selectByQuotaWeight ---

func TestSelectByQuotaWeight_NoQuotaLast(t *testing.T) {
	m := NewManager(nil, nil)
	candidates := []ProviderConfig{
		{Type: "brave", APIKey: "k1", QuotaLimit: 0},
		{Type: "tavily", APIKey: "k2", QuotaLimit: 100},
	}
	result := m.selectByQuotaWeight(context.Background(), candidates)
	require.Len(t, result, 2)
	require.Equal(t, "tavily", result[0].Type)
	require.Equal(t, "brave", result[1].Type)
}

func TestSelectByQuotaWeight_AllNoQuota(t *testing.T) {
	m := NewManager(nil, nil)
	candidates := []ProviderConfig{
		{Type: "brave", APIKey: "k1", QuotaLimit: 0},
		{Type: "tavily", APIKey: "k2", QuotaLimit: 0},
	}
	result := m.selectByQuotaWeight(context.Background(), candidates)
	require.Len(t, result, 2)
}

func TestSelectByQuotaWeight_Empty(t *testing.T) {
	m := NewManager(nil, nil)
	result := m.selectByQuotaWeight(context.Background(), nil)
	require.Empty(t, result)
}

// --- newHTTPClient ---

func TestNewHTTPClient_NoProxy(t *testing.T) {
	c, err := newHTTPClient("")
	require.NoError(t, err)
	require.NotNil(t, c)
}

func TestNewHTTPClient_InvalidProxy(t *testing.T) {
	_, err := newHTTPClient("://bad-url")
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid proxy URL")
}

func TestNewHTTPClient_ValidHTTPProxy(t *testing.T) {
	c, err := newHTTPClient("http://proxy.example.com:8080")
	require.NoError(t, err)
	require.NotNil(t, c)
}

func TestNewHTTPClient_ValidSOCKS5Proxy(t *testing.T) {
	c, err := newHTTPClient("socks5://proxy.example.com:1080")
	require.NoError(t, err)
	require.NotNil(t, c)
}

// --- ResetUsage ---

func TestManager_ResetUsage_NilRedis(t *testing.T) {
	m := NewManager(nil, nil)
	err := m.ResetUsage(context.Background(), "brave")
	require.NoError(t, err)
}
