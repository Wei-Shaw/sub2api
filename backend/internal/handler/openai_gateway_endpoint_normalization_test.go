package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// TestOpenAIUpstreamEndpoint_ViaGetUpstreamEndpoint verifies that the
// unified GetUpstreamEndpoint helper produces the same results as the
// former normalizedOpenAIUpstreamEndpoint for OpenAI platform requests.
func TestOpenAIUpstreamEndpoint_ViaGetUpstreamEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name string
		path string
		want string
	}{
		{
			name: "responses root maps to responses upstream",
			path: "/v1/responses",
			want: EndpointResponses,
		},
		{
			name: "responses compact keeps compact suffix",
			path: "/openai/v1/responses/compact",
			want: "/v1/responses/compact",
		},
		{
			name: "responses nested suffix preserved",
			path: "/openai/v1/responses/compact/detail",
			want: "/v1/responses/compact/detail",
		},
		{
			name: "non responses path uses platform fallback",
			path: "/v1/messages",
			want: EndpointResponses,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, tt.path, nil)

			got := GetUpstreamEndpoint(c, service.PlatformOpenAI)
			require.Equal(t, tt.want, got)
		})
	}
}

// TestOpenAIUpstreamEndpoint_HonorsScheduledOverride verifies that once a
// chat-only API-key account has been scheduled (which records the real upstream
// endpoint via setOpsUpstreamEndpoint), GetUpstreamEndpoint reports
// /v1/chat/completions instead of the platform-default /v1/responses. This is
// what lets BOTH the usage record and the ops error log (which has no account
// object) log the endpoint the request actually hit.
func TestOpenAIUpstreamEndpoint_HonorsScheduledOverride(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	// Before scheduling an account, OpenAI platform defaults to /v1/responses.
	require.Equal(t, EndpointResponses, GetUpstreamEndpoint(c, service.PlatformOpenAI))

	// A chat-only account was scheduled and stored the real upstream endpoint.
	setOpsUpstreamEndpoint(c, EndpointChatCompletions)

	require.Equal(t, EndpointChatCompletions, GetUpstreamEndpoint(c, service.PlatformOpenAI))
}
