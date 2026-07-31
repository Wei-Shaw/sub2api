package service

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

const (
	responseFailedUpstreamMessage = "Our servers are currently overloaded. Please try again later."
	responseFailedCustomMessage   = "service temporarily busy"
	responseFailedCustomStatus    = 521
)

func newResponseFailedKeywordPassthroughRule() *model.ErrorPassthroughRule {
	responseCode := responseFailedCustomStatus
	customMessage := responseFailedCustomMessage
	return &model.ErrorPassthroughRule{
		ID:              991,
		Name:            "response-failed-keyword",
		Enabled:         true,
		Priority:        1,
		ErrorCodes:      []int{http.StatusTeapot},
		Keywords:        []string{"our servers are currently overloaded"},
		MatchMode:       model.MatchModeAny,
		Platforms:       []string{PlatformOpenAI},
		PassthroughCode: false,
		ResponseCode:    &responseCode,
		PassthroughBody: false,
		CustomMessage:   &customMessage,
		SkipMonitoring:  true,
	}
}

func newResponseFailedKeywordPassthroughService() *ErrorPassthroughService {
	ruleSvc := &ErrorPassthroughService{}
	ruleSvc.setLocalCache([]*model.ErrorPassthroughRule{newResponseFailedKeywordPassthroughRule()})
	return ruleSvc
}

func bindResponseFailedKeywordPassthroughRule(c *gin.Context) {
	BindErrorPassthroughService(c, newResponseFailedKeywordPassthroughService())
}

func TestHandleSSEToJSONResponseFailedHonorsKeywordPassthroughRule(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	bindResponseFailedKeywordPassthroughRule(c)

	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
	}
	body := []byte(strings.Join([]string{
		`data: {"type":"response.failed","response":{"error":{"code":"server_is_overloaded","message":"` + responseFailedUpstreamMessage + `"}}}`,
		`data: [DONE]`,
	}, "\n"))

	usage, err := svc.handleSSEToJSON(resp, c, body, &Account{ID: 1, Platform: PlatformOpenAI}, "gpt-test", "gpt-test")
	require.Nil(t, usage)
	require.Error(t, err)
	require.Equal(t, responseFailedCustomStatus, rec.Code)
	require.Equal(t, responseFailedCustomMessage, gjson.Get(rec.Body.String(), "error.message").String())
	require.Contains(t, err.Error(), "passthrough rule matched")
}

func TestHandlePassthroughSSEToJSONResponseFailedHonorsKeywordPassthroughRule(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	bindResponseFailedKeywordPassthroughRule(c)

	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
	}
	body := []byte(strings.Join([]string{
		`data: {"type":"response.failed","response":{"error":{"code":"server_is_overloaded","message":"` + responseFailedUpstreamMessage + `"}}}`,
		`data: [DONE]`,
	}, "\n"))

	usage, err := svc.handlePassthroughSSEToJSON(resp, c, body, &Account{ID: 2, Platform: PlatformOpenAI}, "gpt-test", "gpt-test")
	require.Nil(t, usage)
	require.Error(t, err)
	require.Equal(t, responseFailedCustomStatus, rec.Code)
	require.Equal(t, responseFailedCustomMessage, gjson.Get(rec.Body.String(), "error.message").String())
}

func TestApplyOpenAIStreamFailedEventPassthroughRuleMatchesKeyword(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	bindResponseFailedKeywordPassthroughRule(c)

	payload := []byte(`{"type":"response.failed","response":{"error":{"code":"server_is_overloaded","message":"` + responseFailedUpstreamMessage + `"}}}`)
	updated, status, errType, errMsg, matched := applyOpenAIStreamFailedEventPassthroughRule(
		c,
		&Account{ID: 3, Platform: PlatformOpenAI},
		payload,
		responseFailedUpstreamMessage,
	)

	require.True(t, matched, "the configured status is intentionally different, so the keyword must match")
	require.Equal(t, responseFailedCustomStatus, status)
	require.Equal(t, "upstream_error", errType)
	require.Equal(t, responseFailedCustomMessage, errMsg)
	require.Equal(t, responseFailedCustomMessage, gjson.GetBytes(updated, "response.error.message").String())
	require.Equal(t, "server_is_overloaded", gjson.GetBytes(updated, "response.error.code").String())
	skipMonitoring, exists := c.Get(OpsSkipPassthroughKey)
	require.True(t, exists)
	require.Equal(t, true, skipMonitoring)
}

func TestApplyOpenAIStreamFailedEventPassthroughRulePreservesConfiguredBodyAndCode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	ruleSvc := &ErrorPassthroughService{}
	ruleSvc.setLocalCache([]*model.ErrorPassthroughRule{{
		ID:              992,
		Name:            "overloaded-passthrough",
		Enabled:         true,
		ErrorCodes:      []int{http.StatusBadGateway, http.StatusServiceUnavailable},
		Keywords:        []string{"upstream response failed", "our servers are currently overloaded"},
		MatchMode:       model.MatchModeAny,
		Platforms:       []string{PlatformOpenAI},
		PassthroughCode: true,
		PassthroughBody: true,
	}})
	BindErrorPassthroughService(c, ruleSvc)
	payload := []byte(`{"type":"response.failed","response":{"error":{"code":"server_is_overloaded","message":"` + responseFailedUpstreamMessage + `"}}}`)

	updated, status, errType, errMsg, matched := applyOpenAIStreamFailedEventPassthroughRule(
		c,
		&Account{ID: 5, Platform: PlatformOpenAI},
		payload,
		responseFailedUpstreamMessage,
	)

	require.True(t, matched)
	require.Equal(t, http.StatusServiceUnavailable, status)
	require.Equal(t, "upstream_error", errType)
	require.Equal(t, responseFailedUpstreamMessage, errMsg)
	require.Equal(t, payload, updated)
}

func TestApplyOpenAIStreamFailedEventPassthroughRuleLeavesUnmatchedPayloadUntouched(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	payload := []byte(`{"type":"response.failed","response":{"error":{"code":"server_is_overloaded","message":"` + responseFailedUpstreamMessage + `"}}}`)

	updated, status, errType, errMsg, matched := applyOpenAIStreamFailedEventPassthroughRule(
		c,
		&Account{ID: 6, Platform: PlatformOpenAI},
		payload,
		responseFailedUpstreamMessage,
	)

	require.False(t, matched)
	require.Equal(t, http.StatusServiceUnavailable, status)
	require.Equal(t, "upstream_error", errType)
	require.Equal(t, responseFailedUpstreamMessage, errMsg)
	require.Equal(t, payload, updated)
}

func TestOpenAIStreamingResponseFailedAfterOutputHonorsKeywordPassthroughRule(t *testing.T) {
	for _, passthrough := range []bool{false, true} {
		name := "normalized"
		if passthrough {
			name = "passthrough"
		}
		t.Run(name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			cfg := &config.Config{
				Gateway: config.GatewayConfig{
					StreamDataIntervalTimeout: 0,
					StreamKeepaliveInterval:   0,
					MaxLineSize:               defaultMaxLineSize,
				},
			}
			svc := &OpenAIGatewayService{cfg: cfg}
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			bindResponseFailedKeywordPassthroughRule(c)

			failedPayload := `{"type":"response.failed","response":{"id":"resp_overloaded","status":"failed","error":{"code":"server_is_overloaded","message":"` + responseFailedUpstreamMessage + `"}}}`
			resp := &http.Response{
				StatusCode: http.StatusOK,
				Body: io.NopCloser(strings.NewReader(strings.Join([]string{
					"event: response.output_text.delta",
					`data: {"type":"response.output_text.delta","delta":"partial"}`,
					"",
					"event: response.failed",
					"data: " + failedPayload,
					"",
				}, "\n"))),
				Header: http.Header{"X-Request-Id": []string{"rid-keyword-rule"}},
			}
			account := &Account{ID: 4, Platform: PlatformOpenAI, Name: "acc"}

			var err error
			if passthrough {
				_, err = svc.handleStreamingResponsePassthrough(c.Request.Context(), resp, c, account, time.Now(), "", "")
			} else {
				_, err = svc.handleStreamingResponse(c.Request.Context(), resp, c, account, time.Now(), "gpt-test", "gpt-test")
			}

			require.Error(t, err)
			require.Equal(t, http.StatusOK, rec.Code, "the HTTP stream status is already committed")
			require.Contains(t, rec.Body.String(), responseFailedCustomMessage)
			require.NotContains(t, rec.Body.String(), responseFailedUpstreamMessage)
			streamErr, marked := GetOpsStreamError(c)
			require.True(t, marked)
			require.Equal(t, responseFailedCustomStatus, streamErr.IntendedStatus)
			require.Equal(t, responseFailedCustomMessage, streamErr.Message)
			skipMonitoring, exists := c.Get(OpsSkipPassthroughKey)
			require.True(t, exists)
			require.Equal(t, true, skipMonitoring)
		})
	}
}
