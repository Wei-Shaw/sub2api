package middleware

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type smartRoutingErrorReader struct{}

func (smartRoutingErrorReader) Read([]byte) (int, error) {
	return 0, errors.New("request body read failed")
}

func TestSmartRoutingRejectsBeforeSelectingGroup(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tt := range []struct {
		name       string
		method     string
		path       string
		body       string
		nilGateway bool
		readError  bool
		bodyLimit  int64
		status     int
		code       string
	}{
		{name: "nil gateway", method: http.MethodPost, path: "/v1/chat/completions", body: `{"model":"gpt-4o"}`, nilGateway: true, status: 503, code: "SMART_ROUTING_UNAVAILABLE"},
		{name: "invalid JSON", method: http.MethodPost, path: "/v1/chat/completions", body: `{"model":"gpt-4o"`, status: 400, code: "INVALID_REQUEST"},
		{name: "missing model", method: http.MethodPost, path: "/v1/responses", body: `{}`, status: 400, code: "INVALID_REQUEST"},
		{name: "numeric model", method: http.MethodPost, path: "/v1/responses", body: `{"model":42}`, status: 400, code: "INVALID_REQUEST"},
		{name: "blank model", method: http.MethodPost, path: "/v1/responses/compact", body: `{"model":"  "}`, status: 400, code: "INVALID_REQUEST"},
		{name: "conflicting models", method: http.MethodPost, path: "/v1/chat/completions", body: `{"model":"gpt-4o","model":"gpt-4o-mini"}`, status: 400, code: "INVALID_REQUEST"},
		{name: "body read error", method: http.MethodPost, path: "/v1/responses", readError: true, status: 400, code: "INVALID_REQUEST"},
		{name: "body too large", method: http.MethodPost, path: "/v1/responses", body: `{"model":"gpt-4o"}`, bodyLimit: 4, status: 413, code: "INVALID_REQUEST"},
		{name: "embeddings", method: http.MethodPost, path: "/v1/embeddings", body: `{"model":"text-embedding-3-small"}`, status: 400, code: "SMART_ROUTING_ENDPOINT_UNSUPPORTED"},
		{name: "websocket responses", method: http.MethodGet, path: "/v1/responses", status: 400, code: "SMART_ROUTING_ENDPOINT_UNSUPPORTED"},
		{name: "images", method: http.MethodPost, path: "/v1/images/generations", body: `{"model":"gpt-image-1"}`, status: 400, code: "SMART_ROUTING_ENDPOINT_UNSUPPORTED"},
		{name: "codex unsupported", method: http.MethodPost, path: "/backend-api/codex/embeddings", body: `{"model":"gpt-4o"}`, status: 400, code: "SMART_ROUTING_ENDPOINT_UNSUPPORTED"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			gateway := &service.OpenAIGatewayService{}
			if tt.nilGateway {
				gateway = nil
			}
			key := &service.APIKey{ID: 1, UserID: 2, RoutingGroupIDs: []int64{3, 4}}
			router := gin.New()
			handlerCalled := false
			router.Use(func(c *gin.Context) {
				// No key service is supplied: rejected requests must not load groups.
				selected, ok := resolveSmartRoutingKey(c, nil, gateway, key)
				require.False(t, ok)
				require.Nil(t, selected)
				require.True(t, c.IsAborted())
			})
			router.Handle(tt.method, tt.path, func(c *gin.Context) {
				handlerCalled = true
				c.Status(http.StatusOK)
			})
			req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			if tt.readError {
				req.Body = io.NopCloser(smartRoutingErrorReader{})
				req.ContentLength = -1
			}
			w := httptest.NewRecorder()
			if tt.bodyLimit > 0 {
				req.Body = http.MaxBytesReader(w, req.Body, tt.bodyLimit)
			}
			router.ServeHTTP(w, req)
			require.Equal(t, tt.status, w.Code, w.Body.String())
			var response ErrorResponse
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
			require.Equal(t, tt.code, response.Code)
			require.NotEmpty(t, response.Message)
			require.False(t, handlerCalled, "rejected requests must not reach the handler")
			require.Equal(t, []int64{3, 4}, key.RoutingGroupIDs)
		})
	}
}
