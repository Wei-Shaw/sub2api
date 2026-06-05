//go:build unit

package admin

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestGroupHandler_InvalidIDReturnsStructuredError 验证 group handler path id 解析失败返回
// PR-A 字段级错误协议（reason=INVALID_GROUP_ID + metadata.fields）。
func TestGroupHandler_InvalidIDReturnsStructuredError(t *testing.T) {
	router, _ := setupAdminRouter()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/groups/not-a-number", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	body := decodeError(t, rec)
	require.Equal(t, "INVALID_GROUP_ID", body.Reason)
	require.Equal(t, "id", body.Metadata["param"])
	require.Contains(t, body.Metadata["fields"], `"path":"id"`)
	require.Contains(t, body.Metadata["fields"], `"code":"INVALID_VALUE"`)
}

// TestGroupHandler_InvalidBodyReturnsStructuredError 验证 group Update 体绑定失败返回 INVALID_REQUEST_BODY。
func TestGroupHandler_InvalidBodyReturnsStructuredError(t *testing.T) {
	router, _ := setupAdminRouter()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/groups/1", bytes.NewBufferString(`not-json`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	body := decodeError(t, rec)
	require.Equal(t, "INVALID_REQUEST_BODY", body.Reason)
	require.NotEmpty(t, body.Metadata["binding_error"])
}

// TestGroupHandler_DeleteInvalidID 验证 Delete 同款 helper 路径。
func TestGroupHandler_DeleteInvalidID(t *testing.T) {
	router, _ := setupAdminRouter()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/groups/abc", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	body := decodeError(t, rec)
	require.Equal(t, "INVALID_GROUP_ID", body.Reason)
}
