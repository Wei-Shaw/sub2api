package middleware

import (
	"errors"
	"net"
	"net/http"
	"os"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/gin-gonic/gin"
)

const abortTruncatedStreamKey = "sub2api.abort_truncated_stream"

// IsCommittedEventStream 判断当前响应是否已经提交为 200 OK 的 SSE 事件流。
func IsCommittedEventStream(c *gin.Context) bool {
	if c == nil || c.Writer == nil || !c.Writer.Written() {
		return false
	}
	return c.Writer.Status() == http.StatusOK &&
		strings.Contains(strings.ToLower(c.Writer.Header().Get("Content-Type")), "text/event-stream")
}

// MarkAbortTruncatedStream 标记当前未正常完结的 SSE 流需要在所有 Gin 中间件（含 Ops 错误落库与访问日志）
// 执行完毕后，在最外层触发 http.ErrAbortHandler 中止底层 HTTP/1.1 chunked 或 HTTP/2 流。
func MarkAbortTruncatedStream(c *gin.Context) {
	if !IsCommittedEventStream(c) || c.Request == nil || c.Request.Context().Err() != nil {
		return
	}
	c.Set(abortTruncatedStreamKey, true)
}

// Recovery converts panics into the project's standard JSON error envelope.
//
// It preserves Gin's broken-pipe handling by not attempting to write a response
// when the client connection is already gone.
func Recovery() gin.HandlerFunc {
	inner := gin.CustomRecoveryWithWriter(gin.DefaultErrorWriter, func(c *gin.Context, recovered any) {
		recoveredErr, _ := recovered.(error)
		if errors.Is(recoveredErr, http.ErrAbortHandler) {
			panic(http.ErrAbortHandler)
		}

		if isBrokenPipe(recoveredErr) {
			if recoveredErr != nil {
				_ = c.Error(recoveredErr)
			}
			c.Abort()
			return
		}

		if c.Writer.Written() {
			c.Abort()
			return
		}

		response.ErrorWithDetails(
			c,
			http.StatusInternalServerError,
			infraerrors.UnknownMessage,
			infraerrors.UnknownReason,
			nil,
		)
		c.Abort()
	})
	return func(c *gin.Context) {
		inner(c)
		if c.GetBool(abortTruncatedStreamKey) &&
			c.Request != nil &&
			c.Request.Context().Err() == nil &&
			c.Request.Context().Value(http.ServerContextKey) != nil {
			panic(http.ErrAbortHandler)
		}
	}
}

func isBrokenPipe(err error) bool {
	if err == nil {
		return false
	}

	var opErr *net.OpError
	if !errors.As(err, &opErr) {
		return false
	}

	var syscallErr *os.SyscallError
	if !errors.As(opErr.Err, &syscallErr) {
		return false
	}

	msg := strings.ToLower(syscallErr.Error())
	return strings.Contains(msg, "broken pipe") || strings.Contains(msg, "connection reset by peer")
}
