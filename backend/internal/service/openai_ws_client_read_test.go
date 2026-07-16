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
REDACTED{
		{
			name:          "inter-turn idle sends normal close",
			timeout:       25 * time.Millisecond,
			timeoutStatus: coderws.StatusNormalClosure,
			timeoutReason: "websocket idle timeout",
			wantStatus:    coderws.StatusNormalClosure,
			wantReason:    "websocket idle timeout",
	REDACTED,
		{
			name:          "first message timeout sends policy close",
			timeout:       25 * time.Millisecond,
			timeoutStatus: coderws.StatusPolicyViolation,
			timeoutReason: "missing first response.create message",
			wantStatus:    coderws.StatusPolicyViolation,
			wantReason:    "missing first response.create message",
	REDACTED,
		{
			name:        "lease loss sends retry close",
			cancelCause: ErrOpenAIWSIngressLeaseLost,
			wantStatus:  coderws.StatusTryAgainLater,
			wantReason:  "websocket ingress capacity lease lost; please reconnect",
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			controlCtx, cancelControl := context.WithCancelCause(context.Background())
			defer cancelControl(context.Canceled)
			serverResult := make(chan error, 1)
			readStarted := make(chan struct{REDACTED)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				conn, err := coderws.Accept(w, r, nil)
				if err != nil {
					serverResult <- err
					return
			REDACTED
				defer func() { _ = conn.CloseNow() REDACTED()
				close(readStarted)
				_, _, err = ReadOpenAIWSClientMessage(
					controlCtx,
					conn,
					tt.timeout,
					tt.timeoutStatus,
					tt.timeoutReason,
				)
				serverResult <- err
		REDACTED))
			defer server.Close()

			dialCtx, cancelDial := context.WithTimeout(context.Background(), time.Second)
			clientConn, _, err := coderws.Dial(dialCtx, "ws"+strings.TrimPrefix(server.URL, "http"), nil)
			cancelDial()
		REDACTED
			defer func() { _ = clientConn.CloseNow() REDACTED()
			<-readStarted
			if tt.cancelCause != nil {
				cancelControl(tt.cancelCause)
		REDACTED

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
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestReadOpenAIWSClientMessage_ParentCancellationStillJoinsRead(t *testing.T) {
	controlCtx, cancelControl := context.WithCancelCause(context.Background())
	serverResult := make(chan error, 1)
	readStarted := make(chan struct{REDACTED)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := coderws.Accept(w, r, nil)
		if err != nil {
			serverResult <- err
			return
	REDACTED
		defer func() { _ = conn.CloseNow() REDACTED()
		close(readStarted)
		_, _, err = ReadOpenAIWSClientMessage(controlCtx, conn, 0, 0, "")
		serverResult <- err
REDACTED))
	defer server.Close()

	dialCtx, cancelDial := context.WithTimeout(context.Background(), time.Second)
	clientConn, _, err := coderws.Dial(dialCtx, "ws"+strings.TrimPrefix(server.URL, "http"), nil)
	cancelDial()
REDACTED
	defer func() { _ = clientConn.CloseNow() REDACTED()
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
REDACTED
REDACTED
