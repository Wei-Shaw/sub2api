package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	coderws "github.com/coder/websocket"
	"github.com/stretchr/testify/require"
)

func TestReadOpenAIWSClientMessage_ControlCloseFrames(t *testing.T) {
	tests := []struct {
		name          string
		timeout       time.Duration
		timeoutStatus coderws.StatusCode
		timeoutReason string
		cancelCause   error
		wantStatus    coderws.StatusCode
		wantReason    string
		readAhead     bool
	}{
		{
			name:          "inter-turn idle sends normal close",
			timeout:       25 * time.Millisecond,
			timeoutStatus: coderws.StatusNormalClosure,
			timeoutReason: "websocket idle timeout",
			wantStatus:    coderws.StatusNormalClosure,
			wantReason:    "websocket idle timeout",
		},
		{
			name:          "first message timeout sends policy close",
			timeout:       25 * time.Millisecond,
			timeoutStatus: coderws.StatusPolicyViolation,
			timeoutReason: "missing first response.create message",
			wantStatus:    coderws.StatusPolicyViolation,
			wantReason:    "missing first response.create message",
		},
		{
			name:        "lease loss sends retry close",
			cancelCause: ErrOpenAIWSIngressLeaseLost,
			wantStatus:  coderws.StatusTryAgainLater,
			wantReason:  "websocket ingress capacity lease lost; please reconnect",
		},
		{
			name: "read ahead idle close", readAhead: true,
			timeout: 25 * time.Millisecond, timeoutStatus: coderws.StatusNormalClosure,
			timeoutReason: "websocket idle timeout", wantStatus: coderws.StatusNormalClosure,
			wantReason: "websocket idle timeout",
		},
		{
			name: "read ahead lease loss", readAhead: true,
			cancelCause: ErrOpenAIWSIngressLeaseLost, wantStatus: coderws.StatusTryAgainLater,
			wantReason: "websocket ingress capacity lease lost; please reconnect",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			controlCtx, cancelControl := context.WithCancelCause(context.Background())
			defer cancelControl(context.Canceled)
			serverResult := make(chan error, 1)
			readStarted := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				conn, err := coderws.Accept(w, r, nil)
				if err != nil {
					serverResult <- err
					return
				}
				defer func() { _ = conn.CloseNow() }()
				readCtx := controlCtx
				if tt.readAhead {
					var cleanup func()
					readCtx, cleanup = BeginOpenAIWSClientReadAhead(readCtx, conn)
					defer cleanup()
				}
				close(readStarted)
				_, _, err = ReadOpenAIWSClientMessage(
					readCtx,
					conn,
					tt.timeout,
					tt.timeoutStatus,
					tt.timeoutReason,
				)
				serverResult <- err
			}))
			defer server.Close()

			dialCtx, cancelDial := context.WithTimeout(context.Background(), time.Second)
			clientConn, _, err := coderws.Dial(dialCtx, "ws"+strings.TrimPrefix(server.URL, "http"), nil)
			cancelDial()
			require.NoError(t, err)
			defer func() { _ = clientConn.CloseNow() }()
			<-readStarted
			if tt.cancelCause != nil {
				cancelControl(tt.cancelCause)
			}

			readCtx, cancelRead := context.WithTimeout(context.Background(), time.Second)
			_, _, err = clientConn.Read(readCtx)
			cancelRead()
			var clientClose coderws.CloseError
			require.ErrorAs(t, err, &clientClose)
			require.Equal(t, tt.wantStatus, clientClose.Code)
			require.Equal(t, tt.wantReason, clientClose.Reason)

			select {
			case serverErr := <-serverResult:
				var closeErr *OpenAIWSClientCloseError
				require.ErrorAs(t, serverErr, &closeErr)
				require.Equal(t, tt.wantStatus, closeErr.StatusCode())
				require.Equal(t, tt.wantReason, closeErr.Reason())
			case <-time.After(time.Second):
				t.Fatal("server read goroutine did not exit after close handshake")
			}
		})
	}
}

func TestOpenAIWSClientReadAhead_PreservesFramesAndCanRestart(t *testing.T) {
	buffered := make(chan struct{})
	readNext := make(chan struct{})
	watchingAgain := make(chan struct{})
	finished := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(finished)
		conn, err := coderws.Accept(w, r, nil)
		require.NoError(t, err)
		defer func() { _ = conn.CloseNow() }()
		ctx, cleanup := BeginOpenAIWSClientReadAhead(r.Context(), conn)
		defer cleanup()
		pending, ok := ctx.Value(openAIWSClientReadAheadKey{}).(*openAIWSClientReadAhead)
		require.True(t, ok)
		reusedCtx, reusedCleanup := BeginOpenAIWSClientReadAhead(ctx, conn)
		defer reusedCleanup()
		require.Same(t, ctx, reusedCtx)
		<-pending.done
		close(buffered)
		<-readNext
		for _, want := range []string{"first", "second"} {
			messageType, payload, readErr := ReadOpenAIWSClientMessage(ctx, conn, time.Second, coderws.StatusPolicyViolation, "test timeout")
			require.NoError(t, readErr)
			require.Equal(t, coderws.MessageText, messageType)
			require.Equal(t, want, string(payload))
		}
		nextCtx, nextCleanup := BeginOpenAIWSClientReadAhead(ctx, conn)
		defer nextCleanup()
		require.NotSame(t, pending, nextCtx.Value(openAIWSClientReadAheadKey{}))
		close(watchingAgain)
		select {
		case <-nextCtx.Done():
		case <-time.After(time.Second):
			t.Error("client disconnect did not cancel handshake recovery")
		}
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	conn, _, err := coderws.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), nil)
	require.NoError(t, err)
	defer func() { _ = conn.CloseNow() }()
	require.NoError(t, conn.Write(ctx, coderws.MessageText, []byte("first")))
	select {
	case <-buffered:
	case <-ctx.Done():
		t.Fatal("early frame was not buffered")
	}
	close(readNext)
	require.NoError(t, conn.Write(ctx, coderws.MessageText, []byte("second")))
	select {
	case <-watchingAgain:
	case <-ctx.Done():
		t.Fatal("reader did not hand over both frames")
	}
	_ = conn.CloseNow()
	select {
	case <-finished:
	case <-ctx.Done():
		t.Fatal("read ahead did not exit after disconnect")
	}
}

func TestReadOpenAIWSClientMessage_ParentCancellationStillJoinsRead(t *testing.T) {
	controlCtx, cancelControl := context.WithCancelCause(context.Background())
	serverResult := make(chan error, 1)
	readStarted := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := coderws.Accept(w, r, nil)
		if err != nil {
			serverResult <- err
			return
		}
		defer func() { _ = conn.CloseNow() }()
		close(readStarted)
		_, _, err = ReadOpenAIWSClientMessage(controlCtx, conn, 0, 0, "")
		serverResult <- err
	}))
	defer server.Close()

	dialCtx, cancelDial := context.WithTimeout(context.Background(), time.Second)
	clientConn, _, err := coderws.Dial(dialCtx, "ws"+strings.TrimPrefix(server.URL, "http"), nil)
	cancelDial()
	require.NoError(t, err)
	defer func() { _ = clientConn.CloseNow() }()
	<-readStarted
	cancelControl(errors.New("server shutting down"))
	readCtx, cancelRead := context.WithTimeout(context.Background(), time.Second)
	_, _, err = clientConn.Read(readCtx)
	cancelRead()
	var clientClose coderws.CloseError
	require.ErrorAs(t, err, &clientClose)
	require.Equal(t, coderws.StatusGoingAway, clientClose.Code)
	require.Equal(t, "websocket request canceled", clientClose.Reason)

	select {
	case <-serverResult:
	case <-time.After(time.Second):
		t.Fatal("server read goroutine leaked after parent cancellation")
	}
}
