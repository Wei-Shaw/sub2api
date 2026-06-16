//go:build unit

package admin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

// errorEnvelope 与 task #31 / #33 引入的字段级错误协议对齐：
// {"code": <HTTP int>, "reason": <业务错误码>, "message": <英文>, "metadata": {"fields": <JSON>, "count": <n>, ...}}
type errorEnvelope struct {
	Code     int               `json:"code"`
	Reason   string            `json:"reason"`
	Message  string            `json:"message"`
	Metadata map[string]string `json:"metadata"`
}

func decodeError(t *testing.T, w *httptest.ResponseRecorder) errorEnvelope {
	t.Helper()
	var body errorEnvelope
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body), "non-JSON response: %s", w.Body.String())
	return body
}

// TestUserHandler_InvalidIDReturnsStructuredError 验证 user handler path id 解析失败返回
// PR-A 字段级错误协议（reason=INVALID_USER_ID + metadata.fields）。
func TestUserHandler_InvalidIDReturnsStructuredError(t *testing.T) {
	router, _ := setupAdminRouter()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users/not-a-number", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	body := decodeError(t, rec)
	require.Equal(t, "INVALID_USER_ID", body.Reason)
	require.Equal(t, "id", body.Metadata["param"])
	require.Equal(t, "not-a-number", body.Metadata["value"])
	// path 参数错也走字段级 fields 列表，让前端 parseFieldErrors 能统一消费。
	require.Contains(t, body.Metadata["fields"], `"path":"id"`)
	require.Contains(t, body.Metadata["fields"], `"code":"INVALID_VALUE"`)
}

// TestUserHandler_InvalidBodyReturnsStructuredError 验证 user Update 体绑定失败返回
// reason=INVALID_REQUEST_BODY + metadata.binding_error 携带 gin 原始报错。
func TestUserHandler_InvalidBodyReturnsStructuredError(t *testing.T) {
	router, _ := setupAdminRouter()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/users/1", bytes.NewBufferString(`not-json`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	body := decodeError(t, rec)
	require.Equal(t, "INVALID_REQUEST_BODY", body.Reason)
	require.NotEmpty(t, body.Metadata["binding_error"], "metadata.binding_error 应保留 gin binding 原始错误")
}

// TestUserHandler_DeleteInvalidIDReturnsStructuredError 验证 Delete 也走同一份 ParseInt64Param。
func TestUserHandler_DeleteInvalidIDReturnsStructuredError(t *testing.T) {
	router, _ := setupAdminRouter()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/users/abc", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	body := decodeError(t, rec)
	require.Equal(t, "INVALID_USER_ID", body.Reason)
	require.Contains(t, body.Metadata["fields"], `"path":"id"`)
}
