//go:build unit

package admin

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func setupSettingErrorRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	repo := &settingHandlerRepoStub{values: map[string]string{}}
	svc := service.NewSettingService(repo, &config.Config{})
	h := NewSettingHandler(svc, nil, nil, nil, nil, nil, nil)
	router.PUT("/api/v1/admin/settings", h.UpdateSettings)
	return router
}

// TestSettingHandler_InvalidBodyReturnsStructuredError 验证 setting Update 体绑定失败
// 走 PR-A 字段级错误协议（reason=INVALID_REQUEST_BODY + metadata.binding_error）。
func TestSettingHandler_InvalidBodyReturnsStructuredError(t *testing.T) {
	router := setupSettingErrorRouter()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/settings", bytes.NewBufferString(`not-json`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	body := decodeError(t, rec)
	require.Equal(t, "INVALID_REQUEST_BODY", body.Reason)
	// JSON 语法错不是 validator 错，fields 为 "null" / count=0；binding_error 携带原始报错。
	require.NotEmpty(t, body.Metadata["binding_error"])
	require.Equal(t, "0", body.Metadata["count"])
}

// TestSettingHandler_TurnstileEnabledRequiresKeys 验证业务级"Turnstile 启用必填"
// 现在返回结构化 reason TURNSTILE_SITE_KEY_REQUIRED。
func TestSettingHandler_TurnstileEnabledRequiresKeys(t *testing.T) {
	router := setupSettingErrorRouter()

	// 启用 Turnstile 但不提供 site key（JSON tag: turnstile_enabled / turnstile_site_key）。
	bodyJSON := `{"turnstile_enabled":true,"turnstile_site_key":"","turnstile_secret_key":""}`

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/settings", bytes.NewBufferString(bodyJSON))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	body := decodeError(t, rec)
	require.Equal(t, "TURNSTILE_SITE_KEY_REQUIRED", body.Reason,
		"启用 Turnstile 但 site key 为空时应返回结构化 reason，前端按此 i18n")
}
