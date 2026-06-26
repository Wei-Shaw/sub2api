//go:build unit

package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// upsertCapturingQuotaRepo 实现 service.UserPlatformQuotaRepository，捕获 UpsertForUser 调用。
type upsertCapturingQuotaRepo struct {
	service.UserPlatformQuotaRepository
	listRecords []service.UserPlatformQuotaRecord
	listErr     error
	upsertCalls []upsertCall
	upsertErr   error
	resetCalls  []resetCall
	resetErr    error
REDACTED

type upsertCall struct {
	userID  int64
	records []service.UserPlatformQuotaRecord
REDACTED
type resetCall struct {
	userID   int64
	platform string
	window   string
	newStart time.Time
REDACTED

func (r *upsertCapturingQuotaRepo) ListByUser(_ context.Context, _ int64) ([]service.UserPlatformQuotaRecord, error) {
	return r.listRecords, r.listErr
REDACTED
func (r *upsertCapturingQuotaRepo) UpsertForUser(_ context.Context, userID int64, records []service.UserPlatformQuotaRecord) error {
	cloned := make([]service.UserPlatformQuotaRecord, len(records))
	copy(cloned, records)
	r.upsertCalls = append(r.upsertCalls, upsertCall{userID: userID, records: clonedREDACTED)
	return r.upsertErr
REDACTED
func (r *upsertCapturingQuotaRepo) ResetExpiredWindow(_ context.Context, userID int64, platform string, window string, newStart time.Time) error {
	r.resetCalls = append(r.resetCalls, resetCall{userID, platform, window, newStartREDACTED)
	return r.resetErr
REDACTED

// billingCacheStub 实现 service.BillingCache 中本测试关心的 Delete 方法；其他方法 panic。
type billingCacheStub struct {
	service.BillingCache
	deleteCalls []deleteCall
	deleteErr   error
REDACTED

type deleteCall struct {
	userID   int64
	platform string
REDACTED

func (b *billingCacheStub) DeleteUserPlatformQuotaCache(_ context.Context, userID int64, platform string) error {
	b.deleteCalls = append(b.deleteCalls, deleteCall{userID, platformREDACTED)
	return b.deleteErr
REDACTED

func buildTestHandler(repo service.UserPlatformQuotaRepository, cache service.BillingCache) *UserHandler {
	return &UserHandler{
		userPlatformQuotaRepo: repo,
		billingCache:          cache,
		adminService:          newStubAdminService(),
REDACTED
REDACTED

func putReq(t *testing.T, body string) (*gin.Context, *httptest.ResponseRecorder) {
REDACTED
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req, _ := http.NewRequest(http.MethodPut, "/", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req
	c.Params = []gin.Param{{Key: "id", Value: "42"REDACTEDREDACTED
	return c, w
REDACTED

func TestUpdateUserPlatformQuotas_Success(t *testing.T) {
	repo := &upsertCapturingQuotaRepo{REDACTED
	cache := &billingCacheStub{REDACTED
	h := buildTestHandler(repo, cache)

	body := `{"quotas":[
		{"platform":"anthropic","daily_limit_usd":10.0,"weekly_limit_usd":null,"monthly_limit_usd":100.0REDACTED,
		{"platform":"openai","daily_limit_usd":null,"weekly_limit_usd":null,"monthly_limit_usd":nullREDACTED
	]REDACTED`
	c, w := putReq(t, body)
	h.UpdateUserPlatformQuotas(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
REDACTED
	if len(repo.upsertCalls) != 1 {
		t.Fatalf("UpsertForUser should be called once, got %d", len(repo.upsertCalls))
REDACTED
	if repo.upsertCalls[0].userID != 42 || len(repo.upsertCalls[0].records) != 2 {
		t.Errorf("unexpected upsert call: %+v", repo.upsertCalls[0])
REDACTED
	// 缓存失效：请求中 2 个 platform + 软删除的 3 个 platform（gemini, antigravity, grok）= 5 次
	if len(cache.deleteCalls) != 5 {
		t.Errorf("expected 5 cache delete calls, got %d: %+v", len(cache.deleteCalls), cache.deleteCalls)
REDACTED
REDACTED

func TestUpdateUserPlatformQuotas_RejectsDuplicatePlatform(t *testing.T) {
	h := buildTestHandler(&upsertCapturingQuotaRepo{REDACTED, &billingCacheStub{REDACTED)
	body := `{"quotas":[
		{"platform":"anthropic","daily_limit_usd":1REDACTED,
		{"platform":"anthropic","daily_limit_usd":2REDACTED
	]REDACTED`
	c, w := putReq(t, body)
	h.UpdateUserPlatformQuotas(c)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
REDACTED
REDACTED

func TestUpdateUserPlatformQuotas_RejectsInvalidPlatform(t *testing.T) {
	h := buildTestHandler(&upsertCapturingQuotaRepo{REDACTED, &billingCacheStub{REDACTED)
	body := `{"quotas":[{"platform":"unknown","daily_limit_usd":1REDACTED]REDACTED`
	c, w := putReq(t, body)
	h.UpdateUserPlatformQuotas(c)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
REDACTED
REDACTED

func TestUpdateUserPlatformQuotas_RejectsNegativeLimit(t *testing.T) {
	h := buildTestHandler(&upsertCapturingQuotaRepo{REDACTED, &billingCacheStub{REDACTED)
	body := `{"quotas":[{"platform":"anthropic","daily_limit_usd":-1REDACTED]REDACTED`
	c, w := putReq(t, body)
	h.UpdateUserPlatformQuotas(c)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
REDACTED
REDACTED

func TestUpdateUserPlatformQuotas_RejectsTooManyEntries(t *testing.T) {
	h := buildTestHandler(&upsertCapturingQuotaRepo{REDACTED, &billingCacheStub{REDACTED)
	body := `{"quotas":[
		{"platform":"anthropic"REDACTED,{"platform":"openai"REDACTED,{"platform":"gemini"REDACTED,{"platform":"antigravity"REDACTED,{"platform":"anthropic"REDACTED
	]REDACTED`
	c, w := putReq(t, body)
	h.UpdateUserPlatformQuotas(c)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
REDACTED
REDACTED

func TestUpdateUserPlatformQuotas_ReturnsLatestState(t *testing.T) {
	repo := &upsertCapturingQuotaRepo{
		listRecords: []service.UserPlatformQuotaRecord{
			{UserID: 42, Platform: "anthropic"REDACTED,
	REDACTED,
REDACTED
	cache := &billingCacheStub{REDACTED
	h := buildTestHandler(repo, cache)

	body := `{"quotas":[{"platform":"anthropic","daily_limit_usd":10REDACTED]REDACTED`
	c, w := putReq(t, body)
	h.UpdateUserPlatformQuotas(c)
	if !strings.Contains(w.Body.String(), `"platform_quotas"`) {
		t.Errorf("response should contain platform_quotas array: %s", w.Body.String())
REDACTED
REDACTED

// ───────── T4: Reset 测试 ─────────

func postReq(t *testing.T, body string) (*gin.Context, *httptest.ResponseRecorder) {
REDACTED
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req, _ := http.NewRequest(http.MethodPost, "/", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req
	c.Params = []gin.Param{{Key: "id", Value: "42"REDACTEDREDACTED
	return c, w
REDACTED

func TestResetUserPlatformQuotaWindow_Success(t *testing.T) {
	repo := &upsertCapturingQuotaRepo{REDACTED
	cache := &billingCacheStub{REDACTED
	h := buildTestHandler(repo, cache)
	body := `{"platform":"anthropic","window":"daily"REDACTED`
	c, w := postReq(t, body)
	h.ResetUserPlatformQuotaWindow(c)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
REDACTED
	if len(repo.resetCalls) != 1 {
		t.Fatalf("ResetExpiredWindow should be called once, got %d", len(repo.resetCalls))
REDACTED
	if repo.resetCalls[0].userID != 42 ||
		repo.resetCalls[0].platform != "anthropic" ||
		repo.resetCalls[0].window != "daily" {
		t.Errorf("unexpected reset call: %+v", repo.resetCalls[0])
REDACTED
	if len(cache.deleteCalls) != 1 ||
		cache.deleteCalls[0].userID != 42 ||
		cache.deleteCalls[0].platform != "anthropic" {
		t.Errorf("expected 1 cache delete for anthropic, got %+v", cache.deleteCalls)
REDACTED
REDACTED

func TestResetUserPlatformQuotaWindow_RejectsInvalidWindow(t *testing.T) {
	h := buildTestHandler(&upsertCapturingQuotaRepo{REDACTED, &billingCacheStub{REDACTED)
	c, w := postReq(t, `{"platform":"anthropic","window":"yearly"REDACTED`)
	h.ResetUserPlatformQuotaWindow(c)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
REDACTED
REDACTED

func TestResetUserPlatformQuotaWindow_RejectsInvalidPlatform(t *testing.T) {
	h := buildTestHandler(&upsertCapturingQuotaRepo{REDACTED, &billingCacheStub{REDACTED)
	c, w := postReq(t, `{"platform":"unknown","window":"daily"REDACTED`)
	h.ResetUserPlatformQuotaWindow(c)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
REDACTED
REDACTED

func TestResetUserPlatformQuotaWindow_NotFound(t *testing.T) {
	// handler 检查 service.ErrUserPlatformQuotaNotFound（由 adapter 包装而来）
	repo := &upsertCapturingQuotaRepo{resetErr: service.ErrUserPlatformQuotaNotFoundREDACTED
	h := buildTestHandler(repo, &billingCacheStub{REDACTED)
	c, w := postReq(t, `{"platform":"anthropic","window":"daily"REDACTED`)
	h.ResetUserPlatformQuotaWindow(c)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d: %s", w.Code, w.Body.String())
REDACTED
REDACTED

func TestUpdateUserPlatformQuotas_JSONErrorOnRepoFailure(t *testing.T) {
	repo := &upsertCapturingQuotaRepo{upsertErr: errors.New("db down")REDACTED
	cache := &billingCacheStub{REDACTED
	h := buildTestHandler(repo, cache)
	body := `{"quotas":[{"platform":"anthropic","daily_limit_usd":10REDACTED]REDACTED`
	c, w := putReq(t, body)
	h.UpdateUserPlatformQuotas(c)
	if w.Code < 500 {
		t.Errorf("expected 5xx, got %d", w.Code)
REDACTED
	// 返回 JSON 错误响应
	var body2 map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body2); err != nil {
		t.Errorf("expected JSON error body, got: %s", w.Body.String())
REDACTED
REDACTED

func TestUpdateUserPlatformQuotas_UserNotFound(t *testing.T) {
	repo := &upsertCapturingQuotaRepo{REDACTED
	cache := &billingCacheStub{REDACTED
	adminSvc := newStubAdminService()
	adminSvc.getUserErr = service.ErrUserNotFound
	h := &UserHandler{
		userPlatformQuotaRepo: repo,
		billingCache:          cache,
		adminService:          adminSvc,
REDACTED
	body := `{"quotas":[{"platform":"anthropic","daily_limit_usd":10REDACTED]REDACTED`
	c, w := putReq(t, body)
	h.UpdateUserPlatformQuotas(c)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 when user not found, got %d: %s", w.Code, w.Body.String())
REDACTED
REDACTED

func TestResetUserPlatformQuotaWindow_UserNotFound(t *testing.T) {
	repo := &upsertCapturingQuotaRepo{REDACTED
	cache := &billingCacheStub{REDACTED
	adminSvc := newStubAdminService()
	adminSvc.getUserErr = service.ErrUserNotFound
	h := &UserHandler{
		userPlatformQuotaRepo: repo,
		billingCache:          cache,
		adminService:          adminSvc,
REDACTED
	c, w := postReq(t, `{"platform":"anthropic","window":"daily"REDACTED`)
	h.ResetUserPlatformQuotaWindow(c)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 when user not found, got %d: %s", w.Code, w.Body.String())
REDACTED
REDACTED
