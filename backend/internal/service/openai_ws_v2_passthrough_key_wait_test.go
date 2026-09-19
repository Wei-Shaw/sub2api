package service

import (
	"context"
	"testing"
	"time"

	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// TestPassthroughSecondTurnKeyWaitCancelsPromptlyAndIgnoresLateTerminal drives
// the real passthrough relay: the second response.create waits for key
// admission while a late first-turn terminal arrives, then a pending
// response.cancel must end the wait promptly through the relay's existing
// frame consumer instead of after the queue timeout.
func TestPassthroughSecondTurnKeyWaitCancelsPromptlyAndIgnoresLateTerminal(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := passthroughLifecycleConfig()
	cfg.Gateway.OpenAIFirstOutputTimeoutSeconds = 60
	cfg.Gateway.OpenAIWS.ReadTimeoutSeconds = 60
	upstream := newStagedPassthroughConn()
	svc := newPassthroughLifecycleService(cfg, upstream)

	acquireStarted := make(chan struct{}, 1)
	afterTurns := make(chan *OpenAIForwardResult, 4)
	afterErrs := make(chan error, 4)
	controlCtx := context.Background()
	hooksFactory := func(c *gin.Context) *OpenAIWSIngressHooks {
		return &OpenAIWSIngressHooks{
			AfterTurn: func(_ int, result *OpenAIForwardResult, err error) {
				afterTurns <- result
				afterErrs <- err
			},
			BeforeTurn: func(turn int) error {
				if turn < 2 {
					return nil
				}
				_, err := WaitOpenAIWSKeyAdmission(controlCtx, c, func(waitCtx context.Context) (*APIKeySlotReservation, error) {
					select {
					case acquireStarted <- struct{}{}:
					default:
					}
					<-waitCtx.Done()
					return nil, waitCtx.Err()
				})
				return err
			},
		}
	}
	server, done := startPassthroughLifecycleServerWithHooks(t, controlCtx, svc, passthroughLifecycleAccount(), hooksFactory)
	defer server.Close()
	client := dialPassthroughLifecycleClient(t, server)
	defer func() { _ = client.CloseNow() }()

	requirePassthroughUpstreamWrite(t, upstream, time.Second)
	upstream.Send(`{"type":"response.completed","response":{"id":"resp_key_wait_1","usage":{"input_tokens":4,"output_tokens":2}}}`)
	payload, err := readPassthroughLifecycleFrame(t, client, time.Second)
	require.NoError(t, err)
	require.Contains(t, string(payload), "resp_key_wait_1")
	require.NotNil(t, <-afterTurns)
	require.NoError(t, <-afterErrs)

	require.NoError(t, client.Write(context.Background(), coderws.MessageText, []byte(`{"type":"response.create","model":"gpt-5.1"}`)))
	select {
	case <-acquireStarted:
	case <-time.After(3 * time.Second):
		t.Fatal("second turn did not enter key admission wait")
	}

	// A late terminal for the previous turn must not settle the pending turn.
	upstream.Send(`{"type":"response.completed","response":{"id":"resp_key_wait_1","usage":{"input_tokens":99,"output_tokens":99}}}`)
	select {
	case result := <-afterTurns:
		t.Fatalf("late terminal settled the pending turn: %+v", result)
	case err := <-afterErrs:
		t.Fatalf("late terminal reported a pending turn error: %v", err)
	case <-time.After(400 * time.Millisecond):
	}

	begin := time.Now()
	require.NoError(t, client.Write(context.Background(), coderws.MessageText, []byte(`{"type":"response.cancel"}`)))
	// The late terminal may still be forwarded as a client frame; keep reading
	// until the pending-cancel close frame arrives.
	readDeadline := time.Now().Add(3 * time.Second)
	var closeErr coderws.CloseError
	for {
		_, readErr := readPassthroughLifecycleFrame(t, client, time.Until(readDeadline))
		if readErr != nil {
			require.ErrorAs(t, readErr, &closeErr)
			break
		}
		if time.Now().After(readDeadline) {
			t.Fatal("pending cancel did not close the client session")
		}
	}
	require.Equal(t, coderws.StatusNormalClosure, closeErr.Code)
	require.Contains(t, closeErr.Reason, "canceled")
	require.Less(t, time.Since(begin), 3*time.Second)
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("relay did not stop after the pending response.cancel")
	}

	// The abandoned pending turn reports one cleanup callback without usage.
	require.Nil(t, <-afterTurns)
	require.Error(t, <-afterErrs)
}

// TestPassthroughSecondTurnKeyWaitRejectsOverlappingCreate keeps the existing
// protocol close for a second response.create while the turn waits for key
// admission.
func TestPassthroughSecondTurnKeyWaitRejectsOverlappingCreate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := passthroughLifecycleConfig()
	cfg.Gateway.OpenAIFirstOutputTimeoutSeconds = 60
	cfg.Gateway.OpenAIWS.ReadTimeoutSeconds = 60
	upstream := newStagedPassthroughConn()
	svc := newPassthroughLifecycleService(cfg, upstream)

	acquireStarted := make(chan struct{}, 1)
	controlCtx := context.Background()
	hooksFactory := func(c *gin.Context) *OpenAIWSIngressHooks {
		return &OpenAIWSIngressHooks{
			BeforeTurn: func(turn int) error {
				if turn < 2 {
					return nil
				}
				_, err := WaitOpenAIWSKeyAdmission(controlCtx, c, func(waitCtx context.Context) (*APIKeySlotReservation, error) {
					select {
					case acquireStarted <- struct{}{}:
					default:
					}
					<-waitCtx.Done()
					return nil, waitCtx.Err()
				})
				return err
			},
		}
	}
	server, done := startPassthroughLifecycleServerWithHooks(t, controlCtx, svc, passthroughLifecycleAccount(), hooksFactory)
	defer server.Close()
	client := dialPassthroughLifecycleClient(t, server)
	defer func() { _ = client.CloseNow() }()

	requirePassthroughUpstreamWrite(t, upstream, time.Second)
	upstream.Send(`{"type":"response.completed","response":{"id":"resp_overlap_1","usage":{"input_tokens":1,"output_tokens":1}}}`)
	_, err := readPassthroughLifecycleFrame(t, client, time.Second)
	require.NoError(t, err)

	require.NoError(t, client.Write(context.Background(), coderws.MessageText, []byte(`{"type":"response.create","model":"gpt-5.1"}`)))
	select {
	case <-acquireStarted:
	case <-time.After(3 * time.Second):
		t.Fatal("second turn did not enter key admission wait")
	}
	require.NoError(t, client.Write(context.Background(), coderws.MessageText, []byte(`{"type":"response.create","model":"gpt-5.1"}`)))

	_, readErr := readPassthroughLifecycleFrame(t, client, 3*time.Second)
	var closeErr coderws.CloseError
	require.ErrorAs(t, readErr, &closeErr)
	require.Equal(t, coderws.StatusPolicyViolation, closeErr.Code)
	require.Contains(t, closeErr.Reason, "overlapping response.create")
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("relay did not stop after the overlapping response.create")
	}
}
