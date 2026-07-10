package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestParseTimeRange(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodGet, "/?start_date=2024-01-01&end_date=2024-01-02&timezone=UTC", nil)
	c.Request = req

	start, end := parseTimeRange(c)
	require.Equal(t, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), start)
	require.Equal(t, time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC), end)

	req = httptest.NewRequest(http.MethodGet, "/?start_date=bad&timezone=UTC", nil)
	c.Request = req
	start, end = parseTimeRange(c)
	require.False(t, start.IsZero())
	require.False(t, end.IsZero())
REDACTED

func TestParseOpsViewParam(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/?view=excluded", nil)
	require.Equal(t, opsListViewExcluded, parseOpsViewParam(c))

	c2, _ := gin.CreateTestContext(w)
	c2.Request = httptest.NewRequest(http.MethodGet, "/?view=all", nil)
	require.Equal(t, opsListViewAll, parseOpsViewParam(c2))

	c3, _ := gin.CreateTestContext(w)
	c3.Request = httptest.NewRequest(http.MethodGet, "/?view=unknown", nil)
	require.Equal(t, opsListViewErrors, parseOpsViewParam(c3))

	require.Equal(t, "", parseOpsViewParam(nil))
REDACTED

func TestParseOpsDuration(t *testing.T) {
	dur, ok := parseOpsDuration("1h")
	require.True(t, ok)
	require.Equal(t, time.Hour, dur)

	_, ok = parseOpsDuration("invalid")
	require.False(t, ok)
REDACTED

func TestParseOpsOpenAITokenStatsDuration(t *testing.T) {
	tests := []struct {
		input string
		want  time.Duration
		ok    bool
REDACTED{
		{input: "30m", want: 30 * time.Minute, ok: trueREDACTED,
		{input: "1h", want: time.Hour, ok: trueREDACTED,
		{input: "1d", want: 24 * time.Hour, ok: trueREDACTED,
		{input: "15d", want: 15 * 24 * time.Hour, ok: trueREDACTED,
		{input: "30d", want: 30 * 24 * time.Hour, ok: trueREDACTED,
		{input: "7d", want: 0, ok: falseREDACTED,
REDACTED

	for _, tt := range tests {
		got, ok := parseOpsOpenAITokenStatsDuration(tt.input)
		require.Equal(t, tt.ok, ok, "input=%s", tt.input)
		require.Equal(t, tt.want, got, "input=%s", tt.input)
REDACTED
REDACTED

func TestParseOpsOpenAITokenStatsFilter_Defaults(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	before := time.Now().UTC()
	filter, err := parseOpsOpenAITokenStatsFilter(c)
	after := time.Now().UTC()

REDACTED
	require.NotNil(t, filter)
	require.Equal(t, "30d", filter.TimeRange)
	require.Equal(t, 1, filter.Page)
	require.Equal(t, 20, filter.PageSize)
	require.Equal(t, 0, filter.TopN)
	require.Nil(t, filter.GroupID)
	require.Equal(t, "", filter.Platform)
	require.True(t, filter.StartTime.Before(filter.EndTime))
	require.WithinDuration(t, before.Add(-30*24*time.Hour), filter.StartTime, 2*time.Second)
	require.WithinDuration(t, after, filter.EndTime, 2*time.Second)
REDACTED

func TestParseOpsOpenAITokenStatsFilter_WithTopN(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(
		http.MethodGet,
		"/?time_range=1h&platform=openai&group_id=12&top_n=50",
		nil,
	)

	filter, err := parseOpsOpenAITokenStatsFilter(c)
REDACTED
	require.Equal(t, "1h", filter.TimeRange)
	require.Equal(t, "openai", filter.Platform)
	require.NotNil(t, filter.GroupID)
	require.Equal(t, int64(12), *filter.GroupID)
	require.Equal(t, 50, filter.TopN)
	require.Equal(t, 0, filter.Page)
	require.Equal(t, 0, filter.PageSize)
REDACTED

func TestParseOpsOpenAITokenStatsFilter_InvalidParams(t *testing.T) {
	tests := []string{
		"/?time_range=7d",
		"/?group_id=0",
		"/?group_id=abc",
		"/?top_n=0",
		"/?top_n=101",
		"/?top_n=10&page=1",
		"/?top_n=10&page_size=20",
		"/?page=0",
		"/?page_size=0",
		"/?page_size=101",
REDACTED

	gin.SetMode(gin.TestMode)
	for _, rawURL := range tests {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, rawURL, nil)

		_, err := parseOpsOpenAITokenStatsFilter(c)
		require.Error(t, err, "url=%s", rawURL)
REDACTED
REDACTED

func TestParseOpsTimeRange(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	now := time.Now().UTC()
	startStr := now.Add(-time.Hour).Format(time.RFC3339)
	endStr := now.Format(time.RFC3339)
	c.Request = httptest.NewRequest(http.MethodGet, "/?start_time="+startStr+"&end_time="+endStr, nil)
	start, end, err := parseOpsTimeRange(c, "1h")
REDACTED
	require.True(t, start.Before(end))

	c2, _ := gin.CreateTestContext(w)
	c2.Request = httptest.NewRequest(http.MethodGet, "/?start_time=bad", nil)
	_, _, err = parseOpsTimeRange(c2, "1h")
REDACTED
REDACTED

func TestParseOpsRealtimeWindow(t *testing.T) {
	dur, label, ok := parseOpsRealtimeWindow("5m")
	require.True(t, ok)
	require.Equal(t, 5*time.Minute, dur)
	require.Equal(t, "5min", label)

	_, _, ok = parseOpsRealtimeWindow("invalid")
	require.False(t, ok)
REDACTED

func TestPickThroughputBucketSeconds(t *testing.T) {
	require.Equal(t, 60, pickThroughputBucketSeconds(30*time.Minute))
	require.Equal(t, 300, pickThroughputBucketSeconds(6*time.Hour))
	require.Equal(t, 3600, pickThroughputBucketSeconds(48*time.Hour))
REDACTED

func TestParseOpsQueryMode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/?mode=raw", nil)
	require.Equal(t, service.ParseOpsQueryMode("raw"), parseOpsQueryMode(c))
	require.Equal(t, service.OpsQueryMode(""), parseOpsQueryMode(nil))
REDACTED

func TestOpsAlertRuleValidation(t *testing.T) {
	raw := map[string]json.RawMessage{
		"name":        json.RawMessage(`"High error rate"`),
		"metric_type": json.RawMessage(`"error_rate"`),
		"operator":    json.RawMessage(`">"`),
		"threshold":   json.RawMessage(`90`),
REDACTED

	validated, err := validateOpsAlertRulePayload(raw)
REDACTED
	require.Equal(t, "High error rate", validated.Name)

	_, err = validateOpsAlertRulePayload(map[string]json.RawMessage{REDACTED)
REDACTED

	require.True(t, isPercentOrRateMetric("error_rate"))
	require.False(t, isPercentOrRateMetric("concurrency_queue_depth"))
REDACTED

func TestOpsWSHelpers(t *testing.T) {
	prefixes, invalid := parseTrustedProxyList("10.0.0.0/8,invalid")
	require.Len(t, prefixes, 1)
	require.Len(t, invalid, 1)

	host := hostWithoutPort("example.com:443")
	require.Equal(t, "example.com", host)

	addr := netip.MustParseAddr("10.0.0.1")
	require.True(t, isAddrInTrustedProxies(addr, prefixes))
	require.False(t, isAddrInTrustedProxies(netip.MustParseAddr("192.168.0.1"), prefixes))
REDACTED

// TestOpenAIFastPolicySettingsFromDTO_NormalizesServiceTier 验证 admin
// 写入路径会把 ServiceTier 的空字符串/空白/大小写归一化为
// service.OpenAIFastTierAny ("all")，避免落盘时 "" 与 "all" 双语义。
func TestOpenAIFastPolicySettingsFromDTO_NormalizesServiceTier(t *testing.T) {
	t.Run("nil input returns nil", func(t *testing.T) {
		require.Nil(t, openaiFastPolicySettingsFromDTO(nil))
REDACTED)

	t.Run("empty service_tier becomes 'all'", func(t *testing.T) {
		in := &dto.OpenAIFastPolicySettings{
			Rules: []dto.OpenAIFastPolicyRule{{
				ServiceTier: "",
				Action:      "filter",
				Scope:       "all",
	REDACTED
	REDACTED
		out := openaiFastPolicySettingsFromDTO(in)
		require.NotNil(t, out)
		require.Len(t, out.Rules, 1)
		require.Equal(t, service.OpenAIFastTierAny, out.Rules[0].ServiceTier)
		require.Equal(t, "all", out.Rules[0].ServiceTier)
REDACTED)

	t.Run("whitespace-only service_tier becomes 'all'", func(t *testing.T) {
		in := &dto.OpenAIFastPolicySettings{
			Rules: []dto.OpenAIFastPolicyRule{{
				ServiceTier: "   ",
				Action:      "pass",
				Scope:       "all",
	REDACTED
	REDACTED
		out := openaiFastPolicySettingsFromDTO(in)
		require.Equal(t, service.OpenAIFastTierAny, out.Rules[0].ServiceTier)
REDACTED)

	t.Run("uppercase service_tier is lowercased", func(t *testing.T) {
		in := &dto.OpenAIFastPolicySettings{
			Rules: []dto.OpenAIFastPolicyRule{{
				ServiceTier: "PRIORITY",
				Action:      "filter",
				Scope:       "all",
				UserIDs:     []int64{42REDACTED,
	REDACTED
	REDACTED
		out := openaiFastPolicySettingsFromDTO(in)
		require.Equal(t, service.OpenAIFastTierPriority, out.Rules[0].ServiceTier)
		require.Equal(t, []int64{42REDACTED, out.Rules[0].UserIDs)
REDACTED)

	t.Run("non-empty values pass through (lowercased)", func(t *testing.T) {
		in := &dto.OpenAIFastPolicySettings{
			Rules: []dto.OpenAIFastPolicyRule{
				{ServiceTier: "priority", Action: "filter", Scope: "all"REDACTED,
				{ServiceTier: "flex", Action: "block", Scope: "oauth"REDACTED,
				{ServiceTier: "all", Action: "pass", Scope: "apikey"REDACTED,
		REDACTED,
	REDACTED
		out := openaiFastPolicySettingsFromDTO(in)
		require.Len(t, out.Rules, 3)
		require.Equal(t, service.OpenAIFastTierPriority, out.Rules[0].ServiceTier)
		require.Equal(t, service.OpenAIFastTierFlex, out.Rules[1].ServiceTier)
		require.Equal(t, service.OpenAIFastTierAny, out.Rules[2].ServiceTier)
REDACTED)
REDACTED
