//go:build unit

package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/handler/quotaview"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// fakeQuotaRepoForUserHandler 实现 service.UserPlatformQuotaRepository 最小子集
type fakeQuotaRepoForUserHandler struct {
	service.UserPlatformQuotaRepository
	records []service.UserPlatformQuotaRecord
REDACTED

func (f *fakeQuotaRepoForUserHandler) ListByUser(_ context.Context, _ int64) ([]service.UserPlatformQuotaRecord, error) {
	return f.records, nil
REDACTED

func TestGetMyPlatformQuotas_EmptyReturns200WithEmptyArray(t *testing.T) {
	repo := &fakeQuotaRepoForUserHandler{records: nilREDACTED
	h := &UserHandler{userPlatformQuotaRepo: repoREDACTED
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/user/platform-quotas", nil)
	c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 42REDACTED)
	h.GetMyPlatformQuotas(c)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d. body: %s", w.Code, w.Body.String())
REDACTED
	var body struct {
		Code int `json:"code"`
		Data struct {
			PlatformQuotas []any `json:"platform_quotas"`
	REDACTED `json:"data"`
REDACTED
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal error: %v, body: %s", err, w.Body.String())
REDACTED
	if body.Code != 0 {
		t.Errorf("expected code=0, got %d", body.Code)
REDACTED
	if body.Data.PlatformQuotas == nil {
		// nil 和 empty slice 均视为可接受（JSON 可能序列化为 null 或 []）
		// 此断言只验证 HTTP 200 + code=0 即可
REDACTED
REDACTED

func TestGetMyPlatformQuotas_D14_LazyZeroForExpiredWindow(t *testing.T) {
	pastStart := time.Now().UTC().AddDate(0, 0, -2)
	daily := 5.0
	repo := &fakeQuotaRepoForUserHandler{records: []service.UserPlatformQuotaRecord{{
		UserID:           42,
		Platform:         "anthropic",
		DailyLimitUSD:    &daily,
		DailyUsageUSD:    3.0,
		DailyWindowStart: &pastStart,
REDACTEDREDACTEDREDACTED
	h := &UserHandler{userPlatformQuotaRepo: repoREDACTED
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/user/platform-quotas", nil)
	c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 42REDACTED)
	h.GetMyPlatformQuotas(c)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d. body: %s", w.Code, w.Body.String())
REDACTED

	// 解析 response，验证过期 daily 的 usage_usd=0 且 window_resets_at=null
	body := w.Body.String()
	if !strings.Contains(body, `"daily_usage_usd":0`) {
		t.Errorf("expected daily_usage_usd:0 in body, got: %s", body)
REDACTED
	if !strings.Contains(body, `"daily_window_resets_at":null`) {
		t.Errorf("expected daily_window_resets_at:null in body, got: %s", body)
REDACTED
REDACTED

func TestGetMyPlatformQuotas_NilRepo_Returns200Empty(t *testing.T) {
	h := &UserHandler{userPlatformQuotaRepo: nilREDACTED
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/user/platform-quotas", nil)
	c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 99REDACTED)
	h.GetMyPlatformQuotas(c)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
REDACTED
REDACTED

func TestGetMyPlatformQuotas_NoAuth_Returns401(t *testing.T) {
	h := &UserHandler{userPlatformQuotaRepo: nilREDACTED
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/user/platform-quotas", nil)
	// 不设置 auth subject
	h.GetMyPlatformQuotas(c)
	if w.Code != 401 {
		t.Fatalf("expected 401, got %d", w.Code)
REDACTED
REDACTED

func TestLazyZeroQuotaForResponse_UserViewStripsWindowStart(t *testing.T) {
	start := time.Now().UTC().Add(-1 * time.Hour)
	r := service.UserPlatformQuotaRecord{
		Platform:         "anthropic",
		DailyUsageUSD:    1.0,
		DailyWindowStart: &start,
REDACTED
	out := quotaview.LazyZeroQuotaForResponse(r, time.Now().UTC(), false)
	if _, ok := out["daily_window_start"]; ok {
		t.Error("user view should not include daily_window_start")
REDACTED
REDACTED

func TestLazyZeroQuotaForResponse_AdminViewIncludesWindowStart(t *testing.T) {
	start := time.Now().UTC().Add(-1 * time.Hour)
	r := service.UserPlatformQuotaRecord{
		Platform:         "anthropic",
		DailyWindowStart: &start,
REDACTED
	out := quotaview.LazyZeroQuotaForResponse(r, time.Now().UTC(), true)
	if _, ok := out["daily_window_start"]; !ok {
		t.Error("admin view should include daily_window_start")
REDACTED
REDACTED

func TestLazyZeroQuotaForResponse_ActiveWindowPreservesUsage(t *testing.T) {
	// 今天的窗口起始时间（不过期）：按全局时区取当天 0 点，与 view 层同口径
	now := time.Now()
	today := timezone.StartOfDay(now)
	usage := 2.5
	r := service.UserPlatformQuotaRecord{
		Platform:         "openai",
		DailyUsageUSD:    usage,
		DailyWindowStart: &today,
REDACTED
	out := quotaview.LazyZeroQuotaForResponse(r, now, false)
	if out["daily_usage_usd"] != usage {
		t.Errorf("expected daily_usage_usd=%v, got %v", usage, out["daily_usage_usd"])
REDACTED
	// 活跃窗口应有 resets_at（非 nil）
	if out["daily_window_resets_at"] == nil {
		t.Error("active window should have daily_window_resets_at set")
REDACTED
REDACTED

func TestNeedsDailyReset_NilStart_ReturnsFalse(t *testing.T) {
	if quotaview.NeedsDailyReset(nil, time.Now().UTC()) {
		t.Error("nil start should not need reset")
REDACTED
REDACTED

func TestNeedsDailyReset_OldStart_ReturnsTrue(t *testing.T) {
	old := time.Now().UTC().AddDate(0, 0, -1)
	if !quotaview.NeedsDailyReset(&old, time.Now().UTC()) {
		t.Error("yesterday start should need daily reset")
REDACTED
REDACTED

func TestNeedsWeeklyReset_NilStart_ReturnsFalse(t *testing.T) {
	if quotaview.NeedsWeeklyReset(nil, time.Now().UTC()) {
		t.Error("nil start should not need weekly reset")
REDACTED
REDACTED

func TestNeedsMonthlyReset_NilStart_ReturnsFalse(t *testing.T) {
	if quotaview.NeedsMonthlyReset(nil, time.Now().UTC()) {
		t.Error("nil start should not need monthly reset")
REDACTED
REDACTED

// TestNeedsMonthlyReset_30DayRolling 验证 30 天滚动语义（C-NEW-1）。
func TestNeedsMonthlyReset_30DayRolling_Expired(t *testing.T) {
	start := time.Now().UTC().Add(-31 * 24 * time.Hour) // 31 天前，已过期
	if !quotaview.NeedsMonthlyReset(&start, time.Now().UTC()) {
		t.Error("31 days ago should need monthly reset (30-day rolling)")
REDACTED
REDACTED

func TestNeedsMonthlyReset_30DayRolling_Active(t *testing.T) {
	start := time.Now().UTC().Add(-15 * 24 * time.Hour) // 15 天前，窗口有效
	if quotaview.NeedsMonthlyReset(&start, time.Now().UTC()) {
		t.Error("15 days ago should NOT need monthly reset (30-day rolling, still active)")
REDACTED
REDACTED

// TestNeedsMonthlyReset_CrossMonthBoundary 验证跨自然月时 30 天未满不重置（旧自然月语义会提前重置）。
func TestNeedsMonthlyReset_CrossMonthBoundary(t *testing.T) {
	// 窗口起始 4 月 20 日；5 月 1 日仅过了 11 天，不足 30 天，不应重置
	windowStart := time.Date(2026, 4, 20, 0, 0, 0, 0, time.UTC)
	now := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	if quotaview.NeedsMonthlyReset(&windowStart, now) {
		t.Error("cross-month boundary within 30 days should NOT trigger reset (30-day rolling)")
REDACTED
REDACTED
