//go:build unit

package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type duplicateAccountAdminServiceStub struct {
	service.AdminService
	account      *service.Account
	calls        int
	accountID    int64
	operationKey string
REDACTED

func (s *duplicateAccountAdminServiceStub) DuplicateAccount(_ context.Context, accountID int64, operationKey string) (*service.Account, error) {
	s.calls++
	s.accountID = accountID
	s.operationKey = operationKey
	return s.account, nil
REDACTED

func setupDuplicateAccountRouter(t *testing.T, svc *duplicateAccountAdminServiceStub) *gin.Engine {
REDACTED
	previousCoordinator := service.DefaultIdempotencyCoordinator()
	service.SetDefaultIdempotencyCoordinator(nil)
	t.Cleanup(func() { service.SetDefaultIdempotencyCoordinator(previousCoordinator) REDACTED)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewAccountHandler(svc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	router.POST("/api/v1/admin/accounts/:id/duplicate", handler.Duplicate)
	return router
REDACTED

func TestDuplicateAccountHandlerRedactsCredentials(t *testing.T) {
	svc := &duplicateAccountAdminServiceStub{
		account: &service.Account{
			ID:          43,
			Name:        "primary (Copy)",
			Platform:    service.PlatformAnthropic,
			Type:        service.AccountTypeAPIKey,
			Status:      service.StatusActive,
			Schedulable: false,
	REDACTED"api_key": "top-secret-key"REDACTED,
	REDACTED,
REDACTED
	router := setupDuplicateAccountRouter(t, svc)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/42/duplicate", nil)

	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, 1, svc.calls)
	require.Contains(t, recorder.Body.String(), `"name":"primary (Copy)"`)
	require.NotContains(t, recorder.Body.String(), "top-secret-key")
	var responseBody struct {
		Data struct {
			Credentials map[string]any `json:"credentials"`
			Schedulable bool           `json:"schedulable"`
	REDACTED `json:"data"`
REDACTED
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &responseBody))
	require.Empty(t, responseBody.Data.Credentials)
	require.False(t, responseBody.Data.Schedulable)
REDACTED

func TestDuplicateAccountHandlerRejectsInvalidID(t *testing.T) {
	svc := &duplicateAccountAdminServiceStub{REDACTED
	router := setupDuplicateAccountRouter(t, svc)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/not-a-number/duplicate", nil)

	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Zero(t, svc.calls)
REDACTED

func TestDuplicateAccountHandlerReplaysSameIdempotencyKey(t *testing.T) {
	svc := &duplicateAccountAdminServiceStub{
		account: &service.Account{
			ID:          43,
			Name:        "primary (Copy)",
			Platform:    service.PlatformAnthropic,
			Type:        service.AccountTypeAPIKey,
			Status:      service.StatusActive,
			Schedulable: false,
	REDACTED,
REDACTED
	router := setupDuplicateAccountRouter(t, svc)
	repo := newMemoryIdempotencyRepoStub()
	service.SetDefaultIdempotencyCoordinator(service.NewIdempotencyCoordinator(repo, service.DefaultIdempotencyConfig()))

	call := func() *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/42/duplicate", nil)
		request.Header.Set("Idempotency-Key", "duplicate-account-42")
		router.ServeHTTP(recorder, request)
		return recorder
REDACTED

	first := call()
	second := call()

	require.Equal(t, http.StatusOK, first.Code)
	require.Equal(t, http.StatusOK, second.Code)
	require.Equal(t, 1, svc.calls)
	require.Equal(t, int64(42), svc.accountID)
	require.Equal(t, "duplicate-account-42", svc.operationKey)
	require.Equal(t, "true", second.Header().Get("X-Idempotency-Replayed"))
REDACTED
