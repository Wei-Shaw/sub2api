package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	coderws "github.com/coder/websocket"
	"github.com/stretchr/testify/require"
)

func TestOpenAIWSAccountWaitReader(t *testing.T) {
	for _, action := range []string{"handoff", "disconnect_after_frame", "overflow"} {
		t.Run(action, func(t *testing.T) {
			ready, resume := make(chan struct{}), make(chan struct{})
			resumed := make(chan struct{})
			result := make(chan error, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				conn, err := coderws.Accept(w, r, nil)
				if err != nil {
					result <- err
					return
				}
				ctx, cleanup := WithOpenAIWSAccountWaitReader(r.Context(), conn)
				defer cleanup()
				waitCtx, stop := BeginOpenAIWSAccountWait(ctx)
				defer stop()
				close(ready)
				if action != "handoff" {
					select {
					case <-waitCtx.Done():
						result <- context.Cause(waitCtx)
					case <-time.After(2 * time.Second):
						result <- context.DeadlineExceeded
					}
					return
				}
				<-resume
				stop()
				close(resumed)
				for _, expected := range []string{"early", "next"} {
					kind, body, err := ReadOpenAIWSClientMessage(ctx, conn, time.Second, coderws.StatusGoingAway, "timeout")
					if err != nil {
						result <- err
						return
					}
					if kind != coderws.MessageBinary || string(body) != expected {
						t.Errorf("wrong preserved frame: %v %s", kind, body)
					}
				}
				result <- nil
			}))
			defer server.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			conn, _, err := coderws.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), nil)
			require.NoError(t, err)
			defer func() { _ = conn.CloseNow() }()
			go func() { _, _, _ = conn.Read(ctx) }()
			<-ready
			require.NoError(t, conn.Write(ctx, coderws.MessageBinary, []byte("early")))
			// A pong after the data frame proves the server kept reading controls.
			require.NoError(t, conn.Ping(ctx))
			switch action {
			case "handoff":
				close(resume)
				<-resumed
				require.NoError(t, conn.Write(ctx, coderws.MessageBinary, []byte("next")))
			case "disconnect_after_frame":
				_ = conn.CloseNow()
			case "overflow":
				require.NoError(t, conn.Write(ctx, coderws.MessageBinary, []byte("overflow")))
			}
			select {
			case err := <-result:
				if action == "handoff" {
					require.NoError(t, err)
				} else {
					require.Error(t, err)
					require.NotErrorIs(t, err, context.DeadlineExceeded)
				}
			case <-ctx.Done():
				t.Fatal("reader did not finish")
			}
		})
	}
}
