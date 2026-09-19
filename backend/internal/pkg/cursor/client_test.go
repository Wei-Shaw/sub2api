package cursor

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDialProxyConnRejectsUnsupportedScheme(t *testing.T) {
	_, err := dialProxyConn(context.Background(), testDialer(), "ftp://proxy.local:21", "target.local:443")
	require.ErrorContains(t, err, "unsupported proxy scheme")
}

func TestDialProxyConnInvalidURL(t *testing.T) {
	_, err := dialProxyConn(context.Background(), testDialer(), "http://[::1", "target.local:443")
	require.Error(t, err)
}

// TestDialHTTPConnectTunnelsThroughProxy spins up a minimal CONNECT proxy and
// verifies the tunnel is established (auth header present, 200 reply) and the
// returned conn is usable for application traffic.
func TestDialHTTPConnectTunnelsThroughProxy(t *testing.T) {
	requestSeen := make(chan string, 1)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		// Read the CONNECT request headers.
		buf := make([]byte, 0, 512)
		tmp := make([]byte, 256)
		for !strings.Contains(string(buf), "\r\n\r\n") {
			n, err := conn.Read(tmp)
			if err != nil {
				return
			}
			buf = append(buf, tmp[:n]...)
		}
		select {
		case requestSeen <- string(buf):
		default:
		}
		if _, err := conn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n")); err != nil {
			return
		}
		_, _ = io.Copy(conn, conn) // echo anything sent through the tunnel
	}()
	t.Cleanup(func() { _ = ln.Close() })

	conn, err := dialProxyConn(context.Background(), testDialer(), "http://user:pass@"+ln.Addr().String(), "target.local:443")
	require.NoError(t, err)
	defer conn.Close()

	request := <-requestSeen
	require.Contains(t, request, "CONNECT target.local:443")
	require.Contains(t, request, "Proxy-Authorization: Basic ")

	_, err = conn.Write([]byte("ping"))
	require.NoError(t, err)
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(5*time.Second)))
	echo := make([]byte, 4)
	_, err = io.ReadFull(conn, echo)
	require.NoError(t, err)
	require.Equal(t, "ping", string(echo))
}

func TestStreamingHTTPClientReusesTransportPerProxy(t *testing.T) {
	first, err := StreamingHTTPClient("")
	require.NoError(t, err)
	second, err := StreamingHTTPClient("")
	require.NoError(t, err)
	require.Same(t, first.Transport, second.Transport)
	require.Zero(t, first.Timeout, "streaming client must not set an overall timeout")
}

func TestUnaryHTTPClientSetsOverallTimeout(t *testing.T) {
	client, err := UnaryHTTPClient("")
	require.NoError(t, err)
	require.Equal(t, cursorUnaryTimeout, client.Timeout)
}

func testDialer() *net.Dialer {
	return &net.Dialer{Timeout: 5 * time.Second}
}

// newTestReadCloser builds a nalReadCloser over the given upstream frames with
// a capturing writer, standing in for the request pipe.
// captureWriter records protocol frames written by the responder without the
// synchronization hazards of an io.Pipe.
type captureWriter struct {
	mu  sync.Mutex
	buf []byte
}

func (w *captureWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf = append(w.buf, p...)
	return len(p), nil
}

func (w *captureWriter) Close() error { return nil }

func (w *captureWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return string(w.buf)
}

func newTestReadCloser(tools []AgentTool, upstream []byte) (*nalReadCloser, *captureWriter) {
	captured := &captureWriter{}
	return &nalReadCloser{
			src:       bytes.NewReader(upstream),
			writer:    captured,
			blobs:     make(map[string][]byte),
			responder: newExecResponder(tools),
		},
		captured
}

func encodeAgentExecToolCallFrame(t *testing.T, id uint64, callID, name, argsJSON string) []byte {
	t.Helper()
	schema, err := EncodeProtoJSONValue(json.RawMessage(argsJSON))
	require.NoError(t, err)
	var mcpArgs ProtobufWriter
	mcpArgs.String(fieldMcpArgsName, name)
	mcpArgs.Bytes(fieldMcpArgsArgs, func() []byte {
		var entry ProtobufWriter
		entry.String(1, "city")
		entry.Bytes(2, schema)
		return entry.Result()
	}())
	mcpArgs.String(fieldMcpArgsToolCallID, callID)
	var exec ProtobufWriter
	exec.Varint(fieldExecID, int(id))
	exec.Bytes(fieldExecMcp, mcpArgs.Result())
	var server ProtobufWriter
	server.Bytes(fieldAgentServerExec, exec.Result())
	frame, err := EncodeFrame(server.Result(), false)
	require.NoError(t, err)
	return frame
}

func encodeAgentTurnEndedFrame(t *testing.T) []byte {
	t.Helper()
	var ended ProtobufWriter
	ended.Varint(fieldTurnEndedOutputTokens, 7)
	var endUpdate ProtobufWriter
	endUpdate.Bytes(fieldInteractionTurnEnded, ended.Result())
	var server ProtobufWriter
	server.Bytes(fieldAgentServerInteraction, endUpdate.Result())
	frame, err := EncodeFrame(server.Result(), false)
	require.NoError(t, err)
	return frame
}

func TestNalReadCloserAnswersDeclaredToolCall(t *testing.T) {
	tools := []AgentTool{{Name: "get_weather", Description: "weather", InputSchema: json.RawMessage(`{"type":"object"}`)}}
	upstream := append(
		encodeAgentExecToolCallFrame(t, 5, "call-5", "get_weather", `{"city":"Paris"}`),
		encodeAgentTurnEndedFrame(t)...,
	)
	nrc, captured := newTestReadCloser(tools, upstream)

	consumed, err := io.ReadAll(nrc)
	require.NoError(t, err)

	// The exec frame is answered inline and never reaches the downstream
	// parser; the turn_ended frame passes through.
	require.NotContains(t, string(consumed), "get_weather")
	// The turn_ended frame (AgentServerMessage field 1) still passes through.
	require.Contains(t, string(consumed), string([]byte{byte(fieldAgentServerInteraction)<<3 | 2}))

	// The inline reply is the handoff acknowledgment.
	require.Contains(t, captured.String(), MCPExternalHandoffMessage)
}

func TestNalReadCloserRejectsUndeclaredToolCall(t *testing.T) {
	tools := []AgentTool{{Name: "declared_tool"}}
	upstream := encodeAgentExecToolCallFrame(t, 6, "call-6", "phantom_tool", `{"city":"Paris"}`)
	nrc, captured := newTestReadCloser(tools, upstream)

	consumed, err := io.ReadAll(nrc)
	require.NoError(t, err)
	require.NotContains(t, string(consumed), "phantom_tool")
	require.Contains(t, captured.String(), "phantom_tool")
}

func TestNalReadCloserPassthroughWithoutTools(t *testing.T) {
	// Ask-mode runs (no tools) never intercept exec frames.
	upstream := encodeAgentExecToolCallFrame(t, 5, "call-5", "get_weather", `{"city":"Paris"}`)
	nrc, captured := newTestReadCloser(nil, upstream)

	consumed, err := io.ReadAll(nrc)
	require.NoError(t, err)
	require.Contains(t, string(consumed), "get_weather")
	require.Empty(t, captured.String())
}
