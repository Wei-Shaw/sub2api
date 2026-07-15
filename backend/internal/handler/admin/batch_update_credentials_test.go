//go:build unit

package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// failingAdminService 嵌入 stubAdminService，可配置 UpdateAccount 在指定 ID 时失败。
type failingAdminService struct {
	*stubAdminService
	failOnAccountID int64
	updateCallCount atomic.Int64
REDACTED

func (f *failingAdminService) UpdateAccount(ctx context.Context, id int64, input *service.UpdateAccountInput) (*service.Account, error) {
	f.updateCallCount.Add(1)
	if id == f.failOnAccountID {
		return nil, errors.New("database error")
REDACTED
	return f.stubAdminService.UpdateAccount(ctx, id, input)
REDACTED

func setupAccountHandlerWithService(adminSvc service.AdminService) (*gin.Engine, *AccountHandler) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewAccountHandler(adminSvc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	router.POST("/api/v1/admin/accounts/batch-update-credentials", handler.BatchUpdateCredentials)
	return router, handler
REDACTED

func TestBatchUpdateCredentials_AllSuccess(t *testing.T) {
	svc := &failingAdminService{stubAdminService: newStubAdminService()REDACTED
	router, _ := setupAccountHandlerWithService(svc)

	body, _ := json.Marshal(BatchUpdateCredentialsRequest{
		AccountIDs: []int64{1, 2, 3REDACTED,
		Field:      "account_uuid",
		Value:      "test-uuid",
REDACTED)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/admin/accounts/batch-update-credentials", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, "全部成功时应返回 200")
	require.Equal(t, int64(3), svc.updateCallCount.Load(), "应调用 3 次 UpdateAccount")
REDACTED

func TestBatchUpdateCredentials_PartialFailure(t *testing.T) {
	// 让第 2 个账号（ID=2）更新时失败
	svc := &failingAdminService{
		stubAdminService: newStubAdminService(),
		failOnAccountID:  2,
REDACTED
	router, _ := setupAccountHandlerWithService(svc)

	body, _ := json.Marshal(BatchUpdateCredentialsRequest{
		AccountIDs: []int64{1, 2, 3REDACTED,
		Field:      "org_uuid",
		Value:      "test-org",
REDACTED)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/admin/accounts/batch-update-credentials", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	// 实现采用"部分成功"模式：总是返回 200 + 成功/失败明细
	require.Equal(t, http.StatusOK, w.Code, "批量更新返回 200 + 成功/失败明细")

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	data := resp["data"].(map[string]any)
	require.Equal(t, float64(2), data["success"], "应有 2 个成功")
	require.Equal(t, float64(1), data["failed"], "应有 1 个失败")

	// 所有 3 个账号都会被尝试更新（非 fail-fast）
	require.Equal(t, int64(3), svc.updateCallCount.Load(),
		"应调用 3 次 UpdateAccount（逐个尝试，失败后继续）")
REDACTED

func TestBatchUpdateCredentials_FirstAccountNotFound(t *testing.T) {
	// GetAccount 在 stubAdminService 中总是成功的，需要创建一个 GetAccount 会失败的 stub
	svc := &getAccountFailingService{
		stubAdminService: newStubAdminService(),
		failOnAccountID:  1,
REDACTED
	router, _ := setupAccountHandlerWithService(svc)

	body, _ := json.Marshal(BatchUpdateCredentialsRequest{
		AccountIDs: []int64{1, 2, 3REDACTED,
		Field:      "account_uuid",
		Value:      "test",
REDACTED)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/admin/accounts/batch-update-credentials", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusNotFound, w.Code, "第一阶段验证失败应返回 404")
REDACTED

// getAccountFailingService 模拟 GetAccount 在特定 ID 时返回 not found。
type getAccountFailingService struct {
	*stubAdminService
	failOnAccountID int64
REDACTED

func (f *getAccountFailingService) GetAccount(ctx context.Context, id int64) (*service.Account, error) {
	if id == f.failOnAccountID {
		return nil, errors.New("not found")
REDACTED
	return f.stubAdminService.GetAccount(ctx, id)
REDACTED

func TestBatchUpdateCredentials_InterceptWarmupRequests_NonBool(t *testing.T) {
	svc := &failingAdminService{stubAdminService: newStubAdminService()REDACTED
	router, _ := setupAccountHandlerWithService(svc)

	// intercept_warmup_requests 传入非 bool 类型（string），应返回 400
	body, _ := json.Marshal(map[string]any{
		"account_ids": []int64{1REDACTED,
		"field":       "intercept_warmup_requests",
		"value":       "not-a-bool",
REDACTED)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/admin/accounts/batch-update-credentials", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code,
		"intercept_warmup_requests 传入非 bool 值应返回 400")
REDACTED

func TestBatchUpdateCredentials_InterceptWarmupRequests_ValidBool(t *testing.T) {
	svc := &failingAdminService{stubAdminService: newStubAdminService()REDACTED
	router, _ := setupAccountHandlerWithService(svc)

	body, _ := json.Marshal(map[string]any{
		"account_ids": []int64{1REDACTED,
		"field":       "intercept_warmup_requests",
		"value":       true,
REDACTED)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/admin/accounts/batch-update-credentials", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code,
		"intercept_warmup_requests 传入合法 bool 值应返回 200")
REDACTED

func TestBatchUpdateCredentials_AccountUUID_NonString(t *testing.T) {
	svc := &failingAdminService{stubAdminService: newStubAdminService()REDACTED
	router, _ := setupAccountHandlerWithService(svc)

	// account_uuid 传入非 string 类型（number），应返回 400
	body, _ := json.Marshal(map[string]any{
		"account_ids": []int64{1REDACTED,
		"field":       "account_uuid",
		"value":       12345,
REDACTED)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/admin/accounts/batch-update-credentials", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code,
		"account_uuid 传入非 string 值应返回 400")
REDACTED

func TestBatchUpdateCredentials_AccountUUID_NullValue(t *testing.T) {
	svc := &failingAdminService{stubAdminService: newStubAdminService()REDACTED
	router, _ := setupAccountHandlerWithService(svc)

	// account_uuid 传入 null（设置为空），应正常通过
	body, _ := json.Marshal(map[string]any{
		"account_ids": []int64{1REDACTED,
		"field":       "account_uuid",
		"value":       nil,
REDACTED)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/admin/accounts/batch-update-credentials", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code,
		"account_uuid 传入 null 应返回 200")
REDACTED
