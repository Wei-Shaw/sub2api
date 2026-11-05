//go:build unit

package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/model"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type staticErrorPassthroughRepo struct {
	service.ErrorPassthroughRepository
	rules []*model.ErrorPassthroughRule
}

func (r *staticErrorPassthroughRepo) List(context.Context) ([]*model.ErrorPassthroughRule, error) {
	return r.rules, nil
}

func newOpenAIHandlerWithRule(passthroughBody bool) *OpenAIGatewayHandler {
	rule := &model.ErrorPassthroughRule{
		ID:              1,
		Name:            "openai-4xx-passthrough",
		Enabled:         true,
		ErrorCodes:      []int{http.StatusNotFound},
		MatchMode:       model.MatchModeAny,
		Platforms:       []string{service.PlatformOpenAI},
		PassthroughCode: true,
		PassthroughBody: passthroughBody,
	}
	if !passthroughBody {
		msg := "custom"
		rule.CustomMessage = &msg
	}
	svc := service.NewErrorPassthroughService(&staticErrorPassthroughRepo{rules: []*model.ErrorPassthroughRule{rule}}, nil)
	return &OpenAIGatewayHandler{errorPassthroughService: svc}
}

const modelNotFound404Body = `{"error":{"message":"The model 'x' does not exist","type":"invalid_request_error","code":"model_not_found"}}`

func TestOpenAIFailoverExhausted_BodyPassthroughRulePreservesUpstreamFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	newOpenAIHandlerWithRule(true).handleFailoverExhausted(c, &service.UpstreamFailoverError{
		StatusCode:   http.StatusNotFound,
		ResponseBody: []byte(modelNotFound404Body),
	}, false)

	require.Equal(t, http.StatusNotFound, rec.Code)
	body := rec.Body.String()
	require.Equal(t, "invalid_request_error", gjson.Get(body, "error.type").String())
	require.Equal(t, "model_not_found", gjson.Get(body, "error.code").String())
	require.Equal(t, "The model 'x' does not exist", gjson.Get(body, "error.message").String())
}

func TestOpenAIFailoverExhausted_BodyPassthroughRuleKeepsCodeInStream(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	newOpenAIHandlerWithRule(true).handleFailoverExhausted(c, &service.UpstreamFailoverError{
		StatusCode:   http.StatusNotFound,
		ResponseBody: []byte(modelNotFound404Body),
	}, true)

	require.Contains(t, rec.Body.String(), "event: response.failed")
	require.Contains(t, rec.Body.String(), `"code":"model_not_found"`)
}

func TestOpenAIFailoverExhausted_CustomMessageRuleUnchanged(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	newOpenAIHandlerWithRule(false).handleFailoverExhausted(c, &service.UpstreamFailoverError{
		StatusCode:   http.StatusNotFound,
		ResponseBody: []byte(modelNotFound404Body),
	}, false)

	require.Equal(t, http.StatusNotFound, rec.Code)
	body := rec.Body.String()
	require.Equal(t, "upstream_error", gjson.Get(body, "error.type").String())
	require.Equal(t, "custom", gjson.Get(body, "error.message").String())
	require.False(t, gjson.Get(body, "error.code").Exists())
}
