//go:build unit

package admin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// stubServiceQuotaSvc 用 embedded interface 实现"满足类型即可"——所有未在测试中显式覆盖的方法
// 都会因 nil 嵌入而在调用时 panic。本测试只覆盖 binding/parse 失败路径，正常路径不会触达 svc 方法，
// 所以这种最小 stub 既保持类型安全又避免堆叠 ~20 个空方法占满文件。
type stubServiceQuotaSvc struct {
	service.ServiceQuotaService
}

// quotaErrorResponse 与 response.ErrorFrom 渲染的 envelope 对齐：
// {"code": <HTTP status int>, "message": <英文 message>, "reason": <业务错误码>, "metadata": {...}}
// 前端按 reason 做 i18n 映射；metadata 携带定位字段值。
type quotaErrorResponse struct {
	Code     int               `json:"code"`
	Message  string            `json:"message"`
	Reason   string            `json:"reason"`
	Metadata map[string]string `json:"metadata"`
}

func newServiceQuotaTestRouter(t *testing.T, svc service.ServiceQuotaService) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := NewServiceQuotaHandler(svc)
	r.POST("/api/v1/admin/service-quota", h.Create)
	r.PUT("/api/v1/admin/service-quota/:id", h.Update)
	r.DELETE("/api/v1/admin/service-quota/:id", h.Delete)
	r.POST("/api/v1/admin/service-quota/reset", h.ResetCounter)
	return r
}

// readQuotaError 解析错误响应体，返回 status + 解析后的 envelope。
func readQuotaError(t *testing.T, w *httptest.ResponseRecorder) quotaErrorResponse {
	t.Helper()
	var body quotaErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body), "response should be JSON: %s", w.Body.String())
	return body
}

// TestServiceQuotaHandler_ServiceUnavailable 验证 svc=nil 时返回结构化 SERVICE_QUOTA_UNAVAILABLE，
// 不再吐英文文案 "service quota unavailable"。
func TestServiceQuotaHandler_ServiceUnavailable(t *testing.T) {
	t.Parallel()
	r := newServiceQuotaTestRouter(t, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/service-quota", bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusNotFound, w.Code)
	body := readQuotaError(t, w)
	require.Equal(t, "SERVICE_QUOTA_UNAVAILABLE", body.Reason)
}

// TestServiceQuotaHandler_Create_InvalidBody 验证 Create 体绑定失败时返回 INVALID_REQUEST_BODY
// + metadata.reason 含 gin binding 详细错误（前端可选 expand 给开发者看）。
func TestServiceQuotaHandler_Create_InvalidBody(t *testing.T) {
	t.Parallel()
	r := newServiceQuotaTestRouter(t, &stubServiceQuotaSvc{})

	w := httptest.NewRecorder()
	// 故意发非 JSON：让 ShouldBindJSON 必失败。
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/service-quota", bytes.NewBufferString(`not-json`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
	body := readQuotaError(t, w)
	require.Equal(t, "INVALID_REQUEST_BODY", body.Reason)
	// task #33 升级后：binding_error 保留 gin 原始报错（开发者排查用），fields 是字段级错误数组。
	// 非 validator 错（JSON 语法错）时 fields 为空 / "null"，count=0；前端走兜底文案。
	require.NotEmpty(t, body.Metadata["binding_error"], "metadata.binding_error 应保留 gin binding 原始错误")
	require.Contains(t, body.Metadata, "count", "metadata.count 字段应总是存在")
}

// TestServiceQuotaHandler_Update_InvalidID 验证 path id 解析失败返回 INVALID_ID + metadata 含原值。
func TestServiceQuotaHandler_Update_InvalidID(t *testing.T) {
	t.Parallel()
	r := newServiceQuotaTestRouter(t, &stubServiceQuotaSvc{})

	w := httptest.NewRecorder()
	// id="abc" 非数字 → ParseInt 失败。
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/service-quota/abc", bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
	body := readQuotaError(t, w)
	require.Equal(t, "INVALID_ID", body.Reason)
	require.Equal(t, "id", body.Metadata["param"])
	require.Equal(t, "abc", body.Metadata["value"])
}

// TestServiceQuotaHandler_Update_InvalidBody 验证 ID 合法但 body 错时仍走 INVALID_REQUEST_BODY，
// 而不是被 ID 校验吞掉。
func TestServiceQuotaHandler_Update_InvalidBody(t *testing.T) {
	t.Parallel()
	r := newServiceQuotaTestRouter(t, &stubServiceQuotaSvc{})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/service-quota/1", bytes.NewBufferString(`not-json`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
	body := readQuotaError(t, w)
	require.Equal(t, "INVALID_REQUEST_BODY", body.Reason)
}

// TestServiceQuotaHandler_Update_IDZero 验证 id=0（语义无效，虽然 ParseInt 能解析）也被 reject 为 INVALID_ID。
func TestServiceQuotaHandler_Update_IDZero(t *testing.T) {
	t.Parallel()
	r := newServiceQuotaTestRouter(t, &stubServiceQuotaSvc{})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/service-quota/0", bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
	body := readQuotaError(t, w)
	require.Equal(t, "INVALID_ID", body.Reason)
	require.Equal(t, "0", body.Metadata["value"])
}

// TestServiceQuotaHandler_Delete_InvalidID 验证 Delete 走同一份 ParseInt64Param。
func TestServiceQuotaHandler_Delete_InvalidID(t *testing.T) {
	t.Parallel()
	r := newServiceQuotaTestRouter(t, &stubServiceQuotaSvc{})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/service-quota/not-a-number", nil)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
	body := readQuotaError(t, w)
	require.Equal(t, "INVALID_ID", body.Reason)
}

// TestServiceQuotaHandler_ResetCounter_InvalidBody 验证第 4 处 binding 错（ResetCounter）。
func TestServiceQuotaHandler_ResetCounter_InvalidBody(t *testing.T) {
	t.Parallel()
	r := newServiceQuotaTestRouter(t, &stubServiceQuotaSvc{})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/service-quota/reset", bytes.NewBufferString(`bad`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
	body := readQuotaError(t, w)
	require.Equal(t, "INVALID_REQUEST_BODY", body.Reason)
}
