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
	// 空 account_ids 触发 binding tag min=1（来自 BulkUpdateAccountsRequest 结构体），
	// validator 错被 BindJSONOrError 转字段级 INVALID_REQUEST_BODY；
	// 业务级 ACCOUNT_IDS_REQUIRED 是 binding 通过后再校验时使用，本 case 命中前者。
	require.Equal(t, "INVALID_REQUEST_BODY", body.Reason)
	// validatorFieldPath 当前用 lowerFirstSegment（task #33）—— Go 字段名 AccountIDs 转 "accountIDs"，
	// 还没做完整 snake_case；前端按 path 大小写不敏感匹配即可。下一轮可在 helper 内升级 snake_case
	// 而不影响调用方契约（只需更新测试断言）。
	require.Contains(t, body.Metadata["fields"], `"path":"accountIDs"`)
	require.Contains(t, body.Metadata["fields"], `"code":"MIN"`)
}
