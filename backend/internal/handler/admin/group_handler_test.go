package admin

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// setupCreateGroupRouter 构造一个最小 gin engine，仅挂载 POST /api/v1/admin/groups。
func setupCreateGroupRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewGroupHandler(newStubAdminService(), nil, nil)
	router.POST("/api/v1/admin/groups", handler.Create)
	return router
}

// callCreateGroup 以给定 JSON body 调用 Create handler。
func callCreateGroup(t *testing.T, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/groups", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	setupCreateGroupRouter(t).ServeHTTP(w, req)
	return w
}

func TestCreateGroup_TokenTypeRejectsUSDLimits(t *testing.T) {
	body := `{"name":"g","platform":"anthropic","subscription_type":"subscription_token","daily_limit_usd":5}`
	w := callCreateGroup(t, body)
	if w.Code != 400 {
		t.Errorf("token 型分组带 USD 限额应 400，得到 %d", w.Code)
	}
}

func TestCreateGroup_TokenTypeAcceptsTokenLimits(t *testing.T) {
	body := `{"name":"g","platform":"anthropic","subscription_type":"subscription_token","daily_limit_tokens":1000000}`
	w := callCreateGroup(t, body)
	if w.Code != 200 {
		t.Errorf("token 型分组带 token 限额应 200，得到 %d body=%s", w.Code, w.Body.String())
	}
}
