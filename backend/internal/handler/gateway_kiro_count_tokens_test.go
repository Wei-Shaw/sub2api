//go:build unit

package handler

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// kiroCountTokensTestContext 起一个最小 *gin.Context——KiroCountTokens
// 只依赖 h.cfg（可为 nil，见 readLenientJSONRequestBodyWithPrealloc 的
// gatewayMaxBodySize 对 nil cfg 的处理）和请求体本身，不需要
// newTestGatewayHandler 那一整套账号/计费/DB 依赖（这正是 KiroCountTokens
// 存在的意义：不选账号、不查计费资格）。
func kiroCountTokensTestContext(body string) (*httptest.ResponseRecorder, *gin.Context) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest("POST", "/v1/messages/count_tokens", bytes.NewReader([]byte(body)))
	c.Request.Header.Set("Content-Type", "application/json")
	return recorder, c
}

func TestGatewayHandlerKiroCountTokensReturnsLocalEstimate(t *testing.T) {
	h := &GatewayHandler{}
	recorder, c := kiroCountTokensTestContext(`{"model":"claude-sonnet-4.6","messages":[{"role":"user","content":"hello world"}]}`)

	h.KiroCountTokens(c)

	require.Equal(t, 200, recorder.Code)
	var resp struct {
		InputTokens int `json:"input_tokens"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	require.Positive(t, resp.InputTokens, "必须是本地估算出的正数，不能是 0 或者转发失败的空响应")
}

func TestGatewayHandlerKiroCountTokensRejectsMissingModel(t *testing.T) {
	h := &GatewayHandler{}
	recorder, c := kiroCountTokensTestContext(`{"messages":[{"role":"user","content":"hello"}]}`)

	h.KiroCountTokens(c)

	require.Equal(t, 400, recorder.Code)
	require.Contains(t, recorder.Body.String(), "invalid_request_error")
}

func TestGatewayHandlerKiroCountTokensRejectsEmptyBody(t *testing.T) {
	h := &GatewayHandler{}
	recorder, c := kiroCountTokensTestContext(``)

	h.KiroCountTokens(c)

	require.Equal(t, 400, recorder.Code)
	require.Contains(t, recorder.Body.String(), "Request body is empty")
}

// TestGatewayHandlerKiroCountTokensDoesNotRequireAccountOrBilling 是本处理器
// 存在的核心理由的回归：h 除了 cfg 之外不需要任何依赖（gatewayService/
// billingCacheService 等全部留空），因为 count_tokens 不选账号、不计费。
// 如果未来有人往 KiroCountTokens 里加了一处对这些字段的依赖，这里会先
// panic 出来，而不是留到真实环境里才发现。
func TestGatewayHandlerKiroCountTokensDoesNotRequireAccountOrBilling(t *testing.T) {
	h := &GatewayHandler{}
	_, c := kiroCountTokensTestContext(`{"model":"claude-sonnet-4.6","messages":[{"role":"user","content":"hi"}]}`)

	require.NotPanics(t, func() {
		h.KiroCountTokens(c)
	})
}
