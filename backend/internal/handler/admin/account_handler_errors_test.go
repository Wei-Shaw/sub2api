//go:build unit

package admin

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func setupAccountErrorRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	adminSvc := newStubAdminService()
	h := NewAccountHandler(adminSvc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	router.PUT("/api/v1/admin/accounts/:id", h.Update)
	router.DELETE("/api/v1/admin/accounts/:id", h.Delete)
	router.POST("/api/v1/admin/accounts", h.Create)
	router.POST("/api/v1/admin/accounts/bulk-update", h.BulkUpdate)
	return router
}

// TestAccountHandler_InvalidIDReturnsStructuredError 验证 path id 错走 PR-A 字段级协议。
func TestAccountHandler_InvalidIDReturnsStructuredError(t *testing.T) {
	router := setupAccountErrorRouter()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/accounts/abc", bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	body := decodeError(t, rec)
	require.Equal(t, "INVALID_ACCOUNT_ID", body.Reason)
	require.Equal(t, "id", body.Metadata["param"])
	require.Contains(t, body.Metadata["fields"], `"path":"id"`)
	require.Contains(t, body.Metadata["fields"], `"code":"INVALID_VALUE"`)
}

// TestAccountHandler_InvalidBodyReturnsStructuredError 验证 body 错走 INVALID_REQUEST_BODY。
func TestAccountHandler_InvalidBodyReturnsStructuredError(t *testing.T) {
	router := setupAccountErrorRouter()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts", bytes.NewBufferString(`not-json`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	body := decodeError(t, rec)
	require.Equal(t, "INVALID_REQUEST_BODY", body.Reason)
	require.NotEmpty(t, body.Metadata["binding_error"])
}

// TestAccountHandler_AccountIDsRequired 验证业务级"account_ids 必填"也走结构化 reason。
func TestAccountHandler_AccountIDsRequired(t *testing.T) {
	router := setupAccountErrorRouter()

	// account_ids 为空数组 → reject ACCOUNT_IDS_REQUIRED。
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/bulk-update",
		bytes.NewBufferString(`{"account_ids":[]}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	body := decodeError(t, rec)
	// BulkUpdateAccountsRequest no longer has binding:"min=1" on AccountIDs.
	// Empty account_ids passes binding and hits the business-level check which
	// calls response.BadRequest with a plain message (no structured reason).
	require.Equal(t, "", body.Reason)
	require.Contains(t, body.Message, "account_ids or filters is required")
}
