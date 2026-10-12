package handler

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestStrictAnthropicParserKeepsCompatibilityRepairsObservable(t *testing.T) {
	apiKey := &service.APIKey{Group: &service.Group{Platform: service.PlatformAnthropic}}
	for _, raw := range []string{
		`{ "model":"claude-sonnet-4-6[1m]", "messages":[] }`,
		"{\"model\":\"claude-sonnet-4-6\",\"messages\":[{\"role\":\"user\",\"content\":\"raw\nnewline\"}]}",
	} {
		original := []byte(raw)
		parsed, err := parseAnthropicGatewayRequest(service.NewRequestBodyRef(original), nil, apiKey)
		require.NoError(t, err)
		require.Equal(t, "claude-sonnet-4-6", parsed.Model)
		if strings.Contains(raw, "raw\nnewline") {
			// Compatibility can repair this; the strict admission guard observes
			// the difference from the original bytes and rejects it.
			require.False(t, bytes.Equal(original, parsed.Body.Bytes()))
		} else {
			require.Equal(t, original, parsed.Body.Bytes())
		}
		require.Equal(t, raw, string(original))
	}
}

func TestStrictAnthropicReadPreservesDecodedBody(t *testing.T) {
	for _, body := range []string{"{\n \"model\": \"test\",\"unknown\":9007199254740993 }\n", "{\"text\":\"raw\ncontrol\"}"} {
		req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
		out, err := readAnthropicGatewayRequestBody(req, nil, &service.APIKey{Group: &service.Group{Platform: service.PlatformAnthropic}})
		require.NoError(t, err)
		require.Equal(t, body, string(out))
	}
}
func TestStrictAnthropicAdmissionDoesNotStartResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	c.Set("gateway_suppress_wait_ping", true)
	h := &ConcurrencyHelper{pingFormat: SSEPingFormatClaude, pingInterval: time.Millisecond}
	started := false
	// The timeout precedes the first slot poll: exercise a real queued wait with
	// many potential ping ticks without requiring a cache or upstream service.
	_, err := h.waitForSlotWithPingTimeout(c, "account", 1, 1, 15*time.Millisecond, true, &started, false)
	require.Error(t, err)
	require.False(t, started)
	require.False(t, c.Writer.Written())
	require.Empty(t, rec.Body.String())
}
