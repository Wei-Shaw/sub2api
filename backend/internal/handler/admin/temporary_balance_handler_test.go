package admin

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGrantTemporaryBalanceHandlerAcceptsExpiryAndReturnsFields(t *testing.T) {
	router, _ := setupAdminRouter()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/users/42/temporary-balance", bytes.NewBufferString(`{"amount":12.5,"expires_at":"2099-01-01T00:00:00Z","notes":"welcome"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"temporary_balance":12.5`)
	require.Contains(t, rec.Body.String(), `"active_temporary_balance":12.5`)
	require.Contains(t, rec.Body.String(), `"temporary_balance_expires_at":"2099-01-01T00:00:00Z"`)
}

func TestGrantTemporaryBalanceHandlerRequiresExpiry(t *testing.T) {
	router, _ := setupAdminRouter()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/users/42/temporary-balance", bytes.NewBufferString(`{"amount":12.5}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestUpdateBalanceHandlerSupportsTemporaryBalanceType(t *testing.T) {
	router, _ := setupAdminRouter()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/users/42/balance", bytes.NewBufferString(`{"balance_type":"temporary","amount":7.25,"expires_at":"2099-01-01T00:00:00Z"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"temporary_balance":7.25`)
}

func TestUpdateBalanceHandlerKeepsPermanentBalanceContract(t *testing.T) {
	router, _ := setupAdminRouter()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/users/42/balance", bytes.NewBufferString(`{"balance":7.25,"operation":"add"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"balance":7.25`)
}
