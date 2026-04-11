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

func TestNewManager_SortsByPriority(t *testing.T) {
	configs := []ProviderConfig{
		{Type: "brave", APIKey: "k3", Priority: 30REDACTED,
		{Type: "tavily", APIKey: "k1", Priority: 10REDACTED,
REDACTED
	m := NewManager(configs, nil)
	require.Equal(t, 10, m.configs[0].Priority)
	require.Equal(t, 30, m.configs[1].Priority)
REDACTED

func TestManager_SearchWithBestProvider_EmptyQuery(t *testing.T) {
	m := NewManager([]ProviderConfig{{Type: "brave", APIKey: "k"REDACTEDREDACTED, nil)
	_, _, err := m.SearchWithBestProvider(context.Background(), SearchRequest{Query: ""REDACTED)
	require.ErrorContains(t, err, "empty search query")

	_, _, err = m.SearchWithBestProvider(context.Background(), SearchRequest{Query: "   "REDACTED)
	require.ErrorContains(t, err, "empty search query")
REDACTED

func TestManager_SearchWithBestProvider_SkipEmptyAPIKey(t *testing.T) {
	m := NewManager([]ProviderConfig{{Type: "brave", APIKey: ""REDACTEDREDACTED, nil)
	_, _, err := m.SearchWithBestProvider(context.Background(), SearchRequest{Query: "test"REDACTED)
	require.ErrorContains(t, err, "no available provider")
REDACTED

func TestManager_SearchWithBestProvider_SkipExpired(t *testing.T) {
	past := time.Now().Add(-1 * time.Hour).Unix()
	m := NewManager([]ProviderConfig{
		{Type: "brave", APIKey: "k", ExpiresAt: &pastREDACTED,
REDACTED, nil)
	_, _, err := m.SearchWithBestProvider(context.Background(), SearchRequest{Query: "test"REDACTED)
	require.ErrorContains(t, err, "no available provider")
REDACTED

func TestManager_SearchWithBestProvider_PriorityOrder(t *testing.T) {
	// Create two mock servers that return different results
	srvBrave := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := braveResponse{REDACTED
		resp.Web.Results = []braveResult{{URL: "https://brave.com", Title: "Brave", Description: "from brave"REDACTEDREDACTED
		json.NewEncoder(w).Encode(resp)
REDACTED))
	defer srvBrave.Close()

	// Override brave endpoint for test
	origURL := *braveSearchURL
	u, _ := http.NewRequest("GET", srvBrave.URL, nil)
	*braveSearchURL = *u.URL
	defer func() { *braveSearchURL = origURL REDACTED()

	m := NewManager([]ProviderConfig{
		{Type: "brave", APIKey: "k1", Priority: 1REDACTED,
		{Type: "tavily", APIKey: "k2", Priority: 2REDACTED,
REDACTED, nil)
	// Inject the test server's client
	m.clientCache[srvBrave.URL] = srvBrave.Client()
	m.clientCache[""] = srvBrave.Client()

	resp, providerName, err := m.SearchWithBestProvider(context.Background(), SearchRequest{Query: "test"REDACTED)
REDACTED
	require.Equal(t, "brave", providerName)
	require.Len(t, resp.Results, 1)
	require.Equal(t, "from brave", resp.Results[0].Snippet)
REDACTED

func TestManager_SearchWithBestProvider_NilRedis(t *testing.T) {
	// With nil Redis, quota check is skipped (always allowed)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := braveResponse{REDACTED
		resp.Web.Results = []braveResult{{URL: "https://test.com", Title: "Test", Description: "result"REDACTEDREDACTED
		json.NewEncoder(w).Encode(resp)
REDACTED))
	defer srv.Close()

	origURL := *braveSearchURL
	u, _ := http.NewRequest("GET", srv.URL, nil)
	*braveSearchURL = *u.URL
	defer func() { *braveSearchURL = origURL REDACTED()

	m := NewManager([]ProviderConfig{
		{Type: "brave", APIKey: "k", Priority: 1, QuotaLimit: 100REDACTED,
REDACTED, nil) // nil Redis
	m.clientCache[""] = srv.Client()

	resp, _, err := m.SearchWithBestProvider(context.Background(), SearchRequest{Query: "test"REDACTED)
REDACTED
	require.Len(t, resp.Results, 1)
REDACTED

func TestManager_GetUsage_NilRedis(t *testing.T) {
	m := NewManager(nil, nil)
	used, err := m.GetUsage(context.Background(), "brave", "monthly")
REDACTED
	require.Equal(t, int64(0), used)
REDACTED

func TestManager_GetAllUsage_NilRedis(t *testing.T) {
	m := NewManager([]ProviderConfig{
		{Type: "brave", QuotaRefreshInterval: "monthly"REDACTED,
REDACTED, nil)
	usage := m.GetAllUsage(context.Background())
	require.Equal(t, int64(0), usage["brave"])
REDACTED

// --- Key/TTL helpers ---

func TestQuotaTTL_Daily(t *testing.T) {
	require.Equal(t, 24*time.Hour+quotaTTLBuffer, quotaTTL(QuotaRefreshDaily))
REDACTED

func TestQuotaTTL_Weekly(t *testing.T) {
	require.Equal(t, 7*24*time.Hour+quotaTTLBuffer, quotaTTL(QuotaRefreshWeekly))
REDACTED

func TestQuotaTTL_Monthly(t *testing.T) {
	require.Equal(t, 31*24*time.Hour+quotaTTLBuffer, quotaTTL(QuotaRefreshMonthly))
REDACTED

func TestPeriodKey_Daily(t *testing.T) {
	key := periodKey(QuotaRefreshDaily)
	require.Regexp(t, `^\d{4REDACTED-\d{2REDACTED-\d{2REDACTED$`, key)
REDACTED

func TestPeriodKey_Weekly(t *testing.T) {
	key := periodKey(QuotaRefreshWeekly)
	require.Regexp(t, `^\d{4REDACTED-W\d{2REDACTED$`, key)
REDACTED

func TestPeriodKey_Monthly(t *testing.T) {
	key := periodKey(QuotaRefreshMonthly)
	require.Regexp(t, `^\d{4REDACTED-\d{2REDACTED$`, key)
REDACTED

func TestQuotaRedisKey_Format(t *testing.T) {
	key := quotaRedisKey("brave", QuotaRefreshDaily)
	require.Contains(t, key, "websearch:quota:brave:")
REDACTED

// --- isProviderAvailable ---

func TestIsProviderAvailable_EmptyAPIKey(t *testing.T) {
	m := NewManager(nil, nil)
	require.False(t, m.isProviderAvailable(ProviderConfig{APIKey: ""REDACTED))
REDACTED

func TestIsProviderAvailable_Expired(t *testing.T) {
	m := NewManager(nil, nil)
	past := time.Now().Add(-1 * time.Hour).Unix()
	require.False(t, m.isProviderAvailable(ProviderConfig{APIKey: "k", ExpiresAt: &pastREDACTED))
REDACTED

func TestIsProviderAvailable_Valid(t *testing.T) {
	m := NewManager(nil, nil)
	future := time.Now().Add(1 * time.Hour).Unix()
	require.True(t, m.isProviderAvailable(ProviderConfig{APIKey: "k", ExpiresAt: &futureREDACTED))
	require.True(t, m.isProviderAvailable(ProviderConfig{APIKey: "k"REDACTED)) // no expiry
REDACTED

// --- resolveProxyID ---

func TestResolveProxyID_AccountProxyOverrides(t *testing.T) {
	cfg := ProviderConfig{ProxyID: 42REDACTED
	// account proxy present → return 0 (account proxy has no config-level ID)
	require.Equal(t, int64(0), resolveProxyID(cfg, "http://account-proxy:8080"))
	// no account proxy → return provider's proxy ID
	require.Equal(t, int64(42), resolveProxyID(cfg, ""))
REDACTED

// --- isProxyError ---

func TestIsProxyError_Nil(t *testing.T) {
	require.False(t, isProxyError(nil))
REDACTED

func TestIsProxyError_ConnectionRefused(t *testing.T) {
	err := fmt.Errorf("dial tcp: connection refused")
	require.True(t, isProxyError(err))
REDACTED

func TestIsProxyError_Timeout(t *testing.T) {
	err := fmt.Errorf("i/o timeout while connecting to proxy")
	require.True(t, isProxyError(err))
REDACTED

func TestIsProxyError_SOCKS(t *testing.T) {
	err := fmt.Errorf("socks connect failed")
	require.True(t, isProxyError(err))
REDACTED

func TestIsProxyError_TLSHandshake(t *testing.T) {
	err := fmt.Errorf("tls handshake timeout")
	require.True(t, isProxyError(err))
REDACTED

func TestIsProxyError_APIError_NotProxy(t *testing.T) {
	err := fmt.Errorf("API rate limit exceeded")
	require.False(t, isProxyError(err))
REDACTED

// --- isProxyAvailable (nil Redis) ---

func TestIsProxyAvailable_NilRedis(t *testing.T) {
	m := NewManager(nil, nil)
	require.True(t, m.isProxyAvailable(context.Background(), 42))
REDACTED

func TestIsProxyAvailable_ZeroID(t *testing.T) {
	m := NewManager(nil, nil)
	require.True(t, m.isProxyAvailable(context.Background(), 0))
REDACTED

// --- selectByQuotaWeight ---

func TestSelectByQuotaWeight_NoQuotaLast(t *testing.T) {
	m := NewManager(nil, nil) // nil Redis → GetUsage returns 0
	candidates := []ProviderConfig{
		{Type: "brave", APIKey: "k1", QuotaLimit: 0REDACTED,    // no limit → weight 0
		{Type: "tavily", APIKey: "k2", QuotaLimit: 100REDACTED, // remaining 100
REDACTED
	result := m.selectByQuotaWeight(context.Background(), candidates)
	require.Len(t, result, 2)
	// tavily (with quota) should come first
	require.Equal(t, "tavily", result[0].Type)
	require.Equal(t, "brave", result[1].Type)
REDACTED

func TestSelectByQuotaWeight_AllNoQuota(t *testing.T) {
	m := NewManager(nil, nil)
	candidates := []ProviderConfig{
		{Type: "brave", APIKey: "k1", QuotaLimit: 0REDACTED,
		{Type: "tavily", APIKey: "k2", QuotaLimit: 0REDACTED,
REDACTED
	result := m.selectByQuotaWeight(context.Background(), candidates)
	require.Len(t, result, 2)
	// both have weight 0, original order preserved
REDACTED

func TestSelectByQuotaWeight_Empty(t *testing.T) {
	m := NewManager(nil, nil)
	result := m.selectByQuotaWeight(context.Background(), nil)
	require.Empty(t, result)
REDACTED

// --- newHTTPClient ---

func TestNewHTTPClient_NoProxy(t *testing.T) {
	c, err := newHTTPClient("")
REDACTED
	require.NotNil(t, c)
REDACTED

func TestNewHTTPClient_InvalidProxy(t *testing.T) {
	_, err := newHTTPClient("://bad-url")
REDACTED
	require.Contains(t, err.Error(), "invalid proxy URL")
REDACTED

func TestNewHTTPClient_ValidHTTPProxy(t *testing.T) {
	c, err := newHTTPClient("http://proxy.example.com:8080")
REDACTED
	require.NotNil(t, c)
REDACTED

func TestNewHTTPClient_ValidSOCKS5Proxy(t *testing.T) {
	c, err := newHTTPClient("socks5://proxy.example.com:1080")
REDACTED
	require.NotNil(t, c)
REDACTED
