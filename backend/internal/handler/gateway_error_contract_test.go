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
)

type staticErrorPassthroughRepo struct {
	rules []*model.ErrorPassthroughRule
}

func (r *staticErrorPassthroughRepo) List(context.Context) ([]*model.ErrorPassthroughRule, error) {
	return r.rules, nil
}

func (r *staticErrorPassthroughRepo) GetByID(_ context.Context, id int64) (*model.ErrorPassthroughRule, error) {
	for _, rule := range r.rules {
		if rule.ID == id {
			return rule, nil
		}
	}
	return nil, nil
}

func (r *staticErrorPassthroughRepo) Create(_ context.Context, rule *model.ErrorPassthroughRule) (*model.ErrorPassthroughRule, error) {
	return rule, nil
}

func (r *staticErrorPassthroughRepo) Update(_ context.Context, rule *model.ErrorPassthroughRule) (*model.ErrorPassthroughRule, error) {
	return rule, nil
}

func (r *staticErrorPassthroughRepo) Delete(context.Context, int64) error {
	return nil
}

func new429ErrorPassthroughService() *service.ErrorPassthroughService {
	responseCode := http.StatusTooManyRequests
	message := "号池算力紧张，请稍后再试"
	return service.NewErrorPassthroughService(&staticErrorPassthroughRepo{
		rules: []*model.ErrorPassthroughRule{
			{
				ID:              3,
				Name:            "429",
				Enabled:         true,
				Priority:        -1,
				ErrorCodes:      []int{http.StatusTooManyRequests},
				Keywords:        []string{"rate_limit"},
				MatchMode:       model.MatchModeAny,
				PassthroughCode: false,
				ResponseCode:    &responseCode,
				PassthroughBody: false,
				CustomMessage:   &message,
			},
		},
	}, nil)
}

func TestGatewayCompatibleFailoverExhaustionUsesConfigured429Message(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &GatewayHandler{errorPassthroughService: new429ErrorPassthroughService()}
	failoverErr := &service.UpstreamFailoverError{
		StatusCode: http.StatusTooManyRequests,
		ResponseBody: []byte(`{
			"error":{"type":"rate_limit_error","message":"Upstream rate limit exceeded"}
		}`),
		ResponseHeaders: http.Header{"Retry-After": []string{"45"}},
	}

	t.Run("chat completions", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)

		h.handleCCFailoverExhausted(c, failoverErr, service.PlatformAnthropic, false)

		require.Equal(t, http.StatusTooManyRequests, recorder.Code)
		require.Equal(t, "45", recorder.Header().Get("Retry-After"))
		require.JSONEq(t, `{"error":{"type":"upstream_error","message":"号池算力紧张，请稍后再试"}}`, recorder.Body.String())
	})

	t.Run("responses", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)

		h.handleResponsesFailoverExhausted(c, failoverErr, service.PlatformAnthropic, false)

		require.Equal(t, http.StatusTooManyRequests, recorder.Code)
		require.Equal(t, "45", recorder.Header().Get("Retry-After"))
		require.JSONEq(t, `{"error":{"code":"upstream_error","message":"号池算力紧张，请稍后再试"}}`, recorder.Body.String())
	})

	t.Run("gemini model discovery", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)

		h.writeGeminiAIStudioResponse(c, &service.UpstreamHTTPResult{
			StatusCode: http.StatusTooManyRequests,
			Body:       failoverErr.ResponseBody,
		})

		require.Equal(t, http.StatusTooManyRequests, recorder.Code)
		require.JSONEq(t, `{"error":{"code":429,"message":"号池算力紧张，请稍后再试","status":"RESOURCE_EXHAUSTED"}}`, recorder.Body.String())
	})
}

func TestMatchGatewayErrorPassthroughSupportsCodeOnlyEmptyBody(t *testing.T) {
	responseCode := 524
	message := "服务器繁忙，请稍后再试"
	svc := service.NewErrorPassthroughService(&staticErrorPassthroughRepo{
		rules: []*model.ErrorPassthroughRule{
			{
				ID:              5,
				Name:            "524",
				Enabled:         true,
				ErrorCodes:      []int{524},
				MatchMode:       model.MatchModeAny,
				PassthroughCode: false,
				ResponseCode:    &responseCode,
				PassthroughBody: false,
				CustomMessage:   &message,
				SkipMonitoring:  true,
			},
		},
	}, nil)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	status, gotMessage, matched := matchGatewayErrorPassthrough(c, svc, service.PlatformAnthropic, 524, nil)

	require.True(t, matched)
	require.Equal(t, 524, status)
	require.Equal(t, message, gotMessage)
	skipMonitoring, exists := c.Get(service.OpsSkipPassthroughKey)
	require.True(t, exists)
	require.Equal(t, true, skipMonitoring)
}
