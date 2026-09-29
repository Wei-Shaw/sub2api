//go:build unit

package handler

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// A1-04: 桥接上游在已输出后中途读错误：客户端只收到一条协议内失败终止帧
// （CC: error chunk 无 [DONE]；Responses: response.failed 无 response.completed），
// handler 不再追加通用错误帧。
func TestA104BridgeReadErrorAfterOutputSingleTerminalFrame(t *testing.T) {
	cases := []struct {
		name   string
		path   string
		body   string
		serve  func(h *GatewayHandler, c *gin.Context)
		assert func(t *testing.T, out string)
	}{
		{
			name:  "chat_completions",
			path:  "/v1/chat/completions",
			body:  `{"model":"claude-sonnet-4-5","max_tokens":256,"stream":true,"messages":[{"role":"user","content":"hello a104"}]}`,
			serve: func(h *GatewayHandler, c *gin.Context) { h.ChatCompletions(c) },
			assert: func(t *testing.T, out string) {
				require.NotContains(t, out, "[DONE]")
				require.Equal(t, 1, strings.Count(out, `"error":`), out)
				require.Contains(t, out, "stream_read_error")
			},
		},
		{
			name:  "responses",
			path:  "/v1/responses",
			body:  `{"model":"claude-sonnet-4-5","max_output_tokens":256,"stream":true,"input":"hello a104"}`,
			serve: func(h *GatewayHandler, c *gin.Context) { h.Responses(c) },
			assert: func(t *testing.T, out string) {
				require.NotContains(t, out, "response.completed")
				require.Equal(t, 1, strings.Count(out, "event: response.failed"), out)
				require.Contains(t, out, "stream_read_error")
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pr, pw := io.Pipe()
			upstream := &a107PipeUpstream{body: pr, called: make(chan struct{})}
			env := a107NewEnv(t, upstream, tc.path, []byte(tc.body), "")
			go func() {
				_, _ = io.WriteString(pw, a107SSEHead)
				_ = pw.CloseWithError(io.ErrUnexpectedEOF)
			}()

			done := make(chan struct{})
			go func() {
				defer close(done)
				tc.serve(env.h, env.c)
			}()
			select {
			case <-done:
			case <-time.After(10 * time.Second):
				t.Fatal("handler did not return")
			}
			require.Equal(t, http.StatusOK, env.rec.Code)
			tc.assert(t, env.rec.Body.String())
			require.Equal(t, int64(1), env.cache.accountReleases.Load())
		})
	}
}
