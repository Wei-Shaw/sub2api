package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestOpenAINonStreamingInvalidJSONSchemaFailureReturnsBadRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	resp := openAIInvalidJSONSchemaFailedSSEResponse(false)

	_, err := svc.handleNonStreamingResponse(c.Request.Context(), resp, c, &Account{Platform: PlatformOpenAI}, "gpt-5.6-sol", "gpt-5.6-sol")

	require.Error(t, err)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Equal(t, "invalid_request_error", gjson.Get(rec.Body.String(), "error.type").String())
	require.Contains(t, rec.Body.String(), "additionalProperties")
	var failoverErr *UpstreamFailoverError
	require.False(t, errors.As(err, &failoverErr))
}

func TestOpenAINonStreamingPassthroughInvalidJSONSchemaFailureReturnsBadRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	resp := openAIInvalidJSONSchemaFailedSSEResponse(true)

	_, err := svc.handleNonStreamingResponsePassthrough(c.Request.Context(), resp, c, "gpt-5.6-sol", "gpt-5.6-sol")

	require.Error(t, err)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Equal(t, "invalid_request_error", gjson.Get(rec.Body.String(), "error.type").String())
	require.Contains(t, rec.Body.String(), "additionalProperties")
	var failoverErr *UpstreamFailoverError
	require.False(t, errors.As(err, &failoverErr))
}

func TestOpenAINonStreamingInvalidJSONSchemaFailureAfterKeepaliveCommitUsesBadRequestEvent(t *testing.T) {
	c, rec := newCompactBridgeTestContext(t, true)
	stop := StartOpenAICompactSSEKeepalive(c, keepaliveTestInterval)
	defer stop()
	waitForKeepaliveBeats()

	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	resp := openAIInvalidJSONSchemaFailedSSEResponse(true)

	_, err := svc.handleNonStreamingResponse(c.Request.Context(), resp, c, &Account{Platform: PlatformOpenAI}, "gpt-5.6-sol", "gpt-5.6-sol")

	require.Error(t, err)
	require.Equal(t, http.StatusOK, rec.Code, "心跳已提交后 HTTP 状态不可再修改")
	events := parseCompactBridgeSSE(t, stripKeepaliveComments(rec.Body.String()))
	require.Len(t, events, 1)
	require.Equal(t, "response.failed", events[0][0])
	require.Equal(t, "invalid_request_error", gjson.Get(events[0][1], "response.error.code").String())
	streamErr, ok := GetOpsStreamError(c)
	require.True(t, ok)
	require.Equal(t, http.StatusBadRequest, streamErr.IntendedStatus)
}

func TestOpenAIHTTPInvalidJSONSchemaFailureReturnsBadRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	body := openAIInvalidJSONSchemaErrorBody()
	resp := &http.Response{
		StatusCode: http.StatusBadRequest,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(string(body))),
	}
	svc := &OpenAIGatewayService{cfg: &config.Config{}}

	_, err := svc.handleErrorResponse(context.Background(), resp, c, &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth}, nil)

	require.Error(t, err)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Equal(t, "invalid_request_error", gjson.Get(rec.Body.String(), "error.type").String())
	require.Contains(t, rec.Body.String(), "additionalProperties")
}

func TestOpenAIHTTPPassthroughInvalidJSONSchemaFailureReturnsBadRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	body := openAIInvalidJSONSchemaErrorBody()
	resp := &http.Response{
		StatusCode: http.StatusBadRequest,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
	}
	svc := &OpenAIGatewayService{cfg: &config.Config{}}

	err := svc.handleErrorResponsePassthrough(context.Background(), resp, c, &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth}, nil, body)

	require.Error(t, err)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Equal(t, "invalid_request_error", gjson.Get(rec.Body.String(), "error.type").String())
	require.Contains(t, rec.Body.String(), "additionalProperties")
}

func openAIInvalidJSONSchemaFailedSSEResponse(nested bool) *http.Response {
	payload := `{"type":"response.failed","error":{"code":"invalid_json_schema","message":"Invalid schema for response_format 'codex_output_schema': 'additionalProperties' is required to be supplied and to be false."}}`
	if nested {
		payload = `{"type":"response.failed","response":{"status":"failed","error":{"type":"invalid_request_error","code":"invalid_json_schema","message":"Invalid schema for response_format 'codex_output_schema': 'additionalProperties' is required to be supplied and to be false."}}}`
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			"event: response.failed",
			"data: " + payload,
			"",
		}, "\n"))),
	}
}

func openAIInvalidJSONSchemaErrorBody() []byte {
	return []byte(`{"error":{"type":"invalid_request_error","code":"invalid_json_schema","message":"Invalid schema for response_format 'codex_output_schema': 'additionalProperties' is required to be supplied and to be false."}}`)
}
