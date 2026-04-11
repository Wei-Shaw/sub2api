package websearch

import (
	"context"
	"encoding/json"
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
