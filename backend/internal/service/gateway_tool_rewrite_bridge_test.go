//go:build unit

package service

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestMCPToolNames_BridgeResponseRoundTrip(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, bridge := range []string{"chat", "responses"} {
		for _, stream := range []bool{false, true} {
			name := bridge + "/buffered"
			if stream {
				name = bridge + "/stream"
			}
			t.Run(name, func(t *testing.T) {
				rw := buildTestToolNameRewrite([]byte(`{"tools":[{"name":"read_file"}]}`))
				alias := rw.Forward["read_file"]
				upstream := strings.NewReplacer("codex_app__read_thread", alias, `"123"`, `"`+alias+`"`).Replace(namespaceToolAnthropicStream())
				recorder := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(recorder)
				c.Set(toolNameRewriteKey, rw)
				resp := &http.Response{Body: io.NopCloser(strings.NewReader(upstream))}
				svc := &GatewayService{}
				var err error
				if bridge == "chat" {
					if stream {
						_, err = svc.handleCCStreamingFromAnthropic(resp, c, "model", "model", nil, time.Now())
					} else {
						_, err = svc.handleCCBufferedFromAnthropic(resp, c, "model", "model", nil, time.Now())
					}
				} else if stream {
					_, err = svc.handleResponsesStreamingResponse(resp, c, "model", "model", nil, time.Now(), apicompat.ResponsesClientToolMapping{})
				} else {
					_, err = svc.handleResponsesBufferedStreamingResponse(resp, c, "model", "model", nil, time.Now(), apicompat.ResponsesClientToolMapping{})
				}
				require.NoError(t, err)
				body := recorder.Body.String()
				require.Contains(t, body, `"name":"read_file"`)
				require.NotContains(t, body, `"name":"`+alias+`"`)
				require.Contains(t, body, `\"thread_id\":\"`+alias+`\"`, "arguments must remain opaque")
			})
		}
	}
}

func TestMCPToolNames_RestoreBeforeResponsesNamespace(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rw := buildTestToolNameRewrite([]byte(`{"tools":[{"name":"codex_app__read_thread"}]}`))
	alias := rw.Forward["codex_app__read_thread"]
	upstream := strings.ReplaceAll(namespaceToolAnthropicStream(), "codex_app__read_thread", alias)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Set(toolNameRewriteKey, rw)
	resp := &http.Response{Body: io.NopCloser(strings.NewReader(upstream))}
	_, err := (&GatewayService{}).handleResponsesStreamingResponse(resp, c, "model", "model", nil, time.Now(), namespaceToolMapping())
	require.NoError(t, err)
	require.Contains(t, recorder.Body.String(), `"name":"read_thread"`)
	require.Contains(t, recorder.Body.String(), `"namespace":"codex_app"`)
	require.NotContains(t, recorder.Body.String(), alias)
}
