//go:build unit

package handler

import (
	"io"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// A1-01: 已输出内容后上游下发 `event: error`，Forward 返回部分结果 + 非 failover 错误；
// handler 仍须按原 failover 耗尽的映射写出且仅写出一条终止错误帧。
func TestA101MessagesSSEErrorAfterOutputWritesSingleMappedErrorFrame(t *testing.T) {
	pr, pw := io.Pipe()
	upstream := &a107PipeUpstream{body: pr, called: make(chan struct{})}
	body := `{"model":"claude-sonnet-4-5","max_tokens":256,"stream":true,"messages":[{"role":"user","content":"hello a101"}]}`
	env := a107NewEnv(t, upstream, "/v1/messages", []byte(body), "")

	go func() {
		_, _ = io.WriteString(pw, a107SSEHead)
		_, _ = io.WriteString(pw, "event: error\n"+`data: {"type":"error","error":{"type":"overloaded_error","message":"Overloaded"}}`+"\n\n")
		_ = pw.Close()
	}()

	done := make(chan struct{})
	go func() {
		defer close(done)
		env.h.Messages(env.c)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Messages did not return")
	}

	rec := env.rec.Body.String()
	require.Contains(t, rec, "message_start")
	require.Equal(t, 1, strings.Count(rec, `"type":"error"`), "must write exactly one terminal error frame: %s", rec)
	require.Contains(t, rec, `"overloaded_error"`, "error frame keeps the semantic mapping of the upstream error type")
	require.Equal(t, int64(1), env.cache.accountReleases.Load())
}
