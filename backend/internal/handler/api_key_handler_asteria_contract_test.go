//go:build unit

package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// asteria-control 依赖这个事实：PUT /api/v1/api-keys/:id 的 status 校验是
// `oneof=active inactive`——控制面吊销 key 发的是 "inactive"，而 service 层的
// 域常量 "disabled" 反而过不了这道校验。这里把这个怪相钉住：谁把 oneof 收紧成
// 只认 disabled，控制面的吊销就会静默变成 400，这条用例先红。
//
// 直接走 ShouldBindJSON 而不是 validateAPIKeyUpdateRequest：oneof 是 binding tag，
// 后者只查数值上下限，不看 status。
func TestAsteriaContract_UpdateStatusBindingAcceptsInactiveNotDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)

	bind := func(body string) (int, UpdateAPIKeyRequest) {
		var got UpdateAPIKeyRequest
		r := gin.New()
		r.PUT("/k", func(c *gin.Context) {
			if err := c.ShouldBindJSON(&got); err != nil {
				c.String(http.StatusBadRequest, err.Error())
				return
			}
			c.Status(http.StatusOK)
		})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/k", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		return w.Code, got
	}

	tests := []struct {
		name     string
		body     string
		wantCode int
	}{
		{name: "inactive passes (asteria-control revoke)", body: `{"status":"inactive"}`, wantCode: http.StatusOK},
		{name: "active passes", body: `{"status":"active"}`, wantCode: http.StatusOK},
		{name: "omitted passes", body: `{"name":"x"}`, wantCode: http.StatusOK},
		{name: "disabled rejected (domain constant not in oneof)", body: `{"status":"disabled"}`, wantCode: http.StatusBadRequest},
		{name: "quota_exhausted rejected", body: `{"status":"quota_exhausted"}`, wantCode: http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, got := bind(tt.body)
			require.Equal(t, tt.wantCode, code)
			if tt.wantCode == http.StatusOK && strings.Contains(tt.body, `"status"`) {
				// 通过校验的值原样留在 Status 里，handler 会不加翻译地转给 service（api_key_handler.go Update）。
				require.Contains(t, tt.body, `"`+got.Status+`"`)
			}
		})
	}
}
