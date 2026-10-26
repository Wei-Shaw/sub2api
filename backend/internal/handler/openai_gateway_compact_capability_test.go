package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOpenAIResponsesRequiredCapabilityForNativeCompaction(t *testing.T) {
	for _, platform := range []string{service.PlatformOpenAI, service.PlatformDeepseek, service.PlatformGrok, service.PlatformOpenCodeGo} {
		t.Run(platform, func(t *testing.T) {
			require.Equal(t, service.OpenAIEndpointCapabilityRemoteCompactionV2,
				openAIResponsesRequiredCapabilityForRequest(false, false, true, platform))
			require.Equal(t, service.OpenAIEndpointCapabilityChatCompletions,
				openAIResponsesRequiredCapabilityForRequest(false, false, false, platform))
		})
	}
}
func TestNativeCompactionFailoverExhaustedReportsCapability(t *testing.T) {
	for _, started := range []bool{false, true} {
		t.Run(map[bool]string{false: "json", true: "after_keepalive"}[started], func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			if started {
				c.Writer.Header().Set("Content-Type", "text/event-stream")
				_, err := c.Writer.WriteString(": keepalive\n\n")
				require.NoError(t, err)
				c.Writer.Flush()
			}
			(&OpenAIGatewayHandler{}).handleFailoverExhausted(c, &service.UpstreamFailoverError{
				StatusCode:    http.StatusBadRequest,
				Stage:         service.GatewayFailureStageRequestCapability,
				Scope:         service.GatewayFailureScopeRequest,
				Reason:        service.OpenAINativeCompactionUnsupportedReason,
				ClientMessage: "Selected upstream does not support native remote compaction v2",
			}, started)
			if started {
				require.Equal(t, http.StatusOK, w.Code)
				require.Contains(t, w.Body.String(), "data: ")
			} else {
				require.Equal(t, http.StatusBadRequest, w.Code)
			}
			require.Contains(t, w.Body.String(), `"code":"compact_not_supported"`)
			require.Contains(t, w.Body.String(), "native remote compaction v2")
		})
	}
}
