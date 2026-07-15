package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOpenAIGatewayHandleFailoverExhausted_PassthroughPreservesFinalUpstreamResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"error":{"message":"final upstream","code":"account_dead"},"vendor":"keep-me"}`)
	h := &OpenAIGatewayHandler{}
	failoverErr := &service.UpstreamFailoverError{
		StatusCode:   http.StatusBadRequest,
		ResponseBody: body,
		ResponseHeaders: http.Header{
			"Content-Type":      []string{"application/problem+json"},
			"X-Request-Id":      []string{"rid-final"},
			"Retry-After":       []string{"7"},
			"Set-Cookie":        []string{"admin_token=secret"},
			"WWW-Authenticate":  []string{`Bearer realm="secret"`},
			"Connection":        []string{"keep-alive, X-Internal-Hop"},
			"X-Internal-Hop":    []string{"must-not-leak"},
			"Content-Length":    []string{"999"},
			"Transfer-Encoding": []string{"chunked"},
		},
		PreserveUpstreamResponse: true,
	}

	h.handleFailoverExhausted(c, failoverErr, false)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Equal(t, string(body), rec.Body.String())
	require.Equal(t, "application/problem+json", rec.Header().Get("Content-Type"))
	require.Empty(t, rec.Header().Get("X-Request-Id"))
	require.Equal(t, "7", rec.Header().Get("Retry-After"))
	require.Empty(t, rec.Header().Get("Set-Cookie"))
	require.Empty(t, rec.Header().Get("WWW-Authenticate"))
	require.Empty(t, rec.Header().Get("Connection"))
	require.Empty(t, rec.Header().Get("X-Internal-Hop"))
	require.NotEqual(t, "999", rec.Header().Get("Content-Length"))
	require.Empty(t, rec.Header().Get("Transfer-Encoding"))
}

func TestOpenAIGatewayHandleFailoverExhausted_PassthroughPreserveTakesPrecedenceOverSilentRefusalMapping(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"error":{"type":"upstream_error","code":"openai_silent_refusal","message":"raw passthrough refusal"}}`)
	h := &OpenAIGatewayHandler{}
	failoverErr := &service.UpstreamFailoverError{
		StatusCode:               http.StatusBadRequest,
		ResponseBody:             body,
		ResponseHeaders:          http.Header{"X-Request-Id": []string{"rid-silent-final"}},
		PreserveUpstreamResponse: true,
	}

	h.handleFailoverExhausted(c, failoverErr, false)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Equal(t, string(body), rec.Body.String())
	require.Empty(t, rec.Header().Get("X-Request-Id"))
}

func TestOpenAIGatewayHandleFailoverExhausted_NonPassthroughSilentRefusalRemainsMapped(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	h := &OpenAIGatewayHandler{}
	failoverErr := &service.UpstreamFailoverError{
		StatusCode:      http.StatusBadRequest,
		ResponseBody:    []byte(`{"error":{"code":"openai_silent_refusal","message":"internal detector detail"}}`),
		ResponseHeaders: http.Header{"X-Request-Id": []string{"must-not-leak"}},
	}

	h.handleFailoverExhausted(c, failoverErr, false)

	require.Equal(t, http.StatusBadGateway, rec.Code)
	require.Contains(t, rec.Body.String(), service.OpenAISilentRefusalClientMessage())
	require.NotContains(t, rec.Body.String(), "internal detector detail")
	require.Empty(t, rec.Header().Get("X-Request-Id"))
}

func TestOpenAIGatewayHandleFailoverExhausted_DefaultBehaviorRemainsMapped(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	h := &OpenAIGatewayHandler{}
	failoverErr := &service.UpstreamFailoverError{
		StatusCode:      http.StatusBadRequest,
		ResponseBody:    []byte(`{"error":{"message":"private upstream detail"}}`),
		ResponseHeaders: http.Header{"X-Request-Id": []string{"must-not-leak"}},
	}

	h.handleFailoverExhausted(c, failoverErr, false)

	require.Equal(t, http.StatusBadGateway, rec.Code)
	require.NotContains(t, rec.Body.String(), "private upstream detail")
	require.Empty(t, rec.Header().Get("X-Request-Id"))
}
