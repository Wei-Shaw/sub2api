package service

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOpenAIResponsesStream_RecordsOnlyForwardedTerminalFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	contextLengthFailure := `{"type":"response.failed","padding":"` + strings.Repeat("x", 40*1024) +
		`","response":{"created_at":1782446336,"error":{"code":"context_length_exceeded","message":"Your input exceeds the context window of this model. Please adjust your input and try again."}}}`
	const rateLimitFailure = `{"type":"response.failed","response":{"error":{"code":"rate_limit_exceeded","message":"Rate limit exceeded"}}}`
	for _, path := range []string{"native", "passthrough"} {
		t.Run(path, func(t *testing.T) {
			for _, tt := range []struct {
				name     string
				payload  string
				failover bool
			}{
				{"forwarded", contextLengthFailure, false},
				{"failover", rateLimitFailure, true},
			} {
				t.Run(tt.name, func(t *testing.T) {
					wire := "event: response.failed\ndata: " + tt.payload + "\n\n"
					recorder := httptest.NewRecorder()
					c, _ := gin.CreateTestContext(recorder)
					c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
					response := &http.Response{
						StatusCode: http.StatusOK,
						Header:     http.Header{"Content-Type": {"text/event-stream"}},
						Body:       io.NopCloser(strings.NewReader(wire)),
					}
					svc := &OpenAIGatewayService{}
					account := &Account{ID: 11, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
					var err error
					if path == "native" {
						_, err = svc.handleStreamingResponse(c.Request.Context(), response, c, account, time.Now(), "model", "model")
					} else {
						_, err = svc.handleStreamingResponsePassthrough(c.Request.Context(), response, c, account, time.Now(), "model", "model")
					}
					require.Error(t, err)
					terminal, ok := GetOpsForwardedTerminalFailure(c)
					if tt.failover {
						var failoverErr *UpstreamFailoverError
						require.ErrorAs(t, err, &failoverErr)
						require.Equal(t, http.StatusTooManyRequests, failoverErr.StatusCode)
						require.False(t, c.Writer.Written())
						require.False(t, ok)
						return
					}
					require.Equal(t, http.StatusOK, recorder.Code)
					require.Equal(t, wire, recorder.Body.String())
					require.True(t, ok)
					require.Equal(t, "response.failed", terminal.EventType)
					require.Equal(t, tt.payload, string(terminal.Payload))
				})
			}
		})
	}
}
