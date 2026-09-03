package openai_ws_v2

import (
	"context"
	"strings"
	"testing"
	"time"

	coderws "github.com/coder/websocket"
	"github.com/stretchr/testify/require"
)

// panickingFrameConn 在读取帧时 panic，模拟对端可控字节流触发的解析越界。
type panickingFrameConn struct {
	message string
}

func (c *panickingFrameConn) ReadFrame(context.Context) (coderws.MessageType, []byte, error) {
	panic(c.message)
}

func (c *panickingFrameConn) WriteFrame(context.Context, coderws.MessageType, []byte) error {
	return nil
}

func (c *panickingFrameConn) Close() error { return nil }

func TestRecoverRelayGoroutineRunsCleanupWithPanicError(t *testing.T) {
	t.Parallel()

	done := make(chan error, 1)
	go func() {
		defer close(done)
		defer recoverRelayGoroutine("unit relay", func(err error) { done <- err })
		panic("relay boom")
	}()

	select {
	case err := <-done:
		require.Error(t, err)
		require.True(t, strings.Contains(err.Error(), "unit relay"))
		require.True(t, strings.Contains(err.Error(), "relay boom"))
	case <-time.After(2 * time.Second):
		t.Fatal("cleanup never ran: the relay loop would hang on exitCh")
	}
}

// TestRunClientToUpstreamPanicSignalsExit 证明客户端读泵 panic 后中继主循环仍能
// 从 exitCh 收到退出信号，而不是永久阻塞。
func TestRunClientToUpstreamPanicSignalsExit(t *testing.T) {
	t.Parallel()

	exitCh := make(chan relayExitSignal, 1)
	go runClientToUpstream(
		context.Background(),
		&panickingFrameConn{message: "client read boom"},
		nil,
		func(_ coderws.MessageType, _ []byte) error { return nil },
		func() {},
		nil,
		nil,
		exitCh,
	)

	select {
	case sig := <-exitCh:
		require.Equal(t, "read_client", sig.stage)
		require.Error(t, sig.err)
		require.True(t, strings.Contains(sig.err.Error(), "client read boom"))
		require.False(t, sig.graceful)
	case <-time.After(2 * time.Second):
		t.Fatal("relay loop hung: the panicking client pump never signalled an exit")
	}
}
