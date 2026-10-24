package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// startKeyAdmissionWSServer accepts one real WebSocket connection and keeps the
// server handler alive until cleanup, so the shared reader observes real
// socket-level closes instead of a stub.
func startKeyAdmissionWSServer(t *testing.T) (client *coderws.Conn, serverConn *coderws.Conn) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	connCh := make(chan *coderws.Conn, 1)
	stop := make(chan struct{})
	var stopOnce sync.Once
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := coderws.Accept(w, r, nil)
		if err != nil {
			return
		}
		connCh <- conn
		<-stop
		_ = conn.CloseNow()
	}))
	client, _, err := coderws.Dial(context.Background(), "ws"+strings.TrimPrefix(server.URL, "http"), nil)
	require.NoError(t, err)
	serverConn = <-connCh
	t.Cleanup(func() {
		stopOnce.Do(func() { close(stop) })
		_ = client.CloseNow()
		server.Close()
	})
	return client, serverConn
}

func newKeyAdmissionGinContext() *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/openai/v1/responses", nil)
	return c
}

type keyAdmissionResult struct {
	reservation *APIKeySlotReservation
	err         error
}

// blockingKeyAdmission models an occupied key queue: the worker stays pending
// until its gate context is cancelled.
func blockingKeyAdmission(started chan<- struct{}) func(context.Context) (*APIKeySlotReservation, error) {
	return func(ctx context.Context) (*APIKeySlotReservation, error) {
		if started != nil {
			select {
			case started <- struct{}{}:
			default:
			}
		}
		<-ctx.Done()
		return nil, ctx.Err()
	}
}

func waitKeyAdmissionResult(t *testing.T, resultCh <-chan keyAdmissionResult) keyAdmissionResult {
	t.Helper()
	select {
	case got := <-resultCh:
		return got
	case <-time.After(3 * time.Second):
		t.Fatal("key admission wait did not finish promptly")
		return keyAdmissionResult{}
	}
}

func TestOpenAIWSKeyAdmissionStopsPromptlyWhenClientCloses(t *testing.T) {
	client, serverConn := startKeyAdmissionWSServer(t)
	c := newKeyAdmissionGinContext()
	EnsureOpenAIWSIngressReader(c, serverConn)

	started := make(chan struct{}, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	resultCh := make(chan keyAdmissionResult, 1)
	begin := time.Now()
	go func() {
		reservation, err := WaitOpenAIWSKeyAdmission(ctx, c, blockingKeyAdmission(started))
		resultCh <- keyAdmissionResult{reservation: reservation, err: err}
	}()
	<-started
	require.NoError(t, client.CloseNow())

	got := waitKeyAdmissionResult(t, resultCh)
	require.Less(t, time.Since(begin), 3*time.Second)
	require.Nil(t, got.reservation)
	require.True(t, IsOpenAIWSClientGoneError(got.err), "got %v", got.err)
}

func TestOpenAIWSKeyAdmissionPassthroughCancelEndsWait(t *testing.T) {
	client, serverConn := startKeyAdmissionWSServer(t)
	c := newKeyAdmissionGinContext()
	EnsureOpenAIWSIngressReader(c, serverConn)
	openAIWSIngressReaderFromContext(c).setWaitFrameMode(true)

	started := make(chan struct{}, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	resultCh := make(chan keyAdmissionResult, 1)
	begin := time.Now()
	go func() {
		reservation, err := WaitOpenAIWSKeyAdmission(ctx, c, blockingKeyAdmission(started))
		resultCh <- keyAdmissionResult{reservation: reservation, err: err}
	}()
	<-started
	require.NoError(t, client.Write(context.Background(), coderws.MessageText, []byte(`{"type":"response.cancel"}`)))

	got := waitKeyAdmissionResult(t, resultCh)
	require.Less(t, time.Since(begin), 3*time.Second)
	require.Nil(t, got.reservation)
	require.False(t, IsOpenAIWSClientGoneError(got.err))
	var closeErr *OpenAIWSClientCloseError
	require.ErrorAs(t, got.err, &closeErr)
	require.Equal(t, coderws.StatusNormalClosure, closeErr.StatusCode())
	require.Contains(t, closeErr.Reason(), "canceled")
}

// TestOpenAIWSKeyAdmissionFirstTurnCancelWithoutMode pins the mode-unknown
// first-turn wait: no account or ingress mode exists yet, but the request was
// never forwarded, so a pending response.cancel is a local cancellation and
// ends the wait with the same explicit normal close.
func TestOpenAIWSKeyAdmissionFirstTurnCancelWithoutMode(t *testing.T) {
	client, serverConn := startKeyAdmissionWSServer(t)
	c := newKeyAdmissionGinContext()
	EnsureOpenAIWSIngressReader(c, serverConn)

	started := make(chan struct{}, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	resultCh := make(chan keyAdmissionResult, 1)
	begin := time.Now()
	go func() {
		reservation, err := WaitOpenAIWSKeyAdmission(ctx, c, blockingKeyAdmission(started))
		resultCh <- keyAdmissionResult{reservation: reservation, err: err}
	}()
	<-started
	require.NoError(t, client.Write(context.Background(), coderws.MessageText, []byte(`{"type":"response.cancel"}`)))

	got := waitKeyAdmissionResult(t, resultCh)
	require.Less(t, time.Since(begin), 3*time.Second)
	require.Nil(t, got.reservation)
	require.False(t, IsOpenAIWSClientGoneError(got.err))
	var closeErr *OpenAIWSClientCloseError
	require.ErrorAs(t, got.err, &closeErr)
	require.Equal(t, coderws.StatusNormalClosure, closeErr.StatusCode())
	require.Contains(t, closeErr.Reason(), "canceled")
}

// TestOpenAIWSKeyAdmissionFirstTurnPreservesFramesAndOldTarget keeps the
// mode-unknown backlog bounded by the same 8-frame preserve budget and keeps
// target matching: a cancel for an older response waits instead of canceling
// the new pending turn.
func TestOpenAIWSKeyAdmissionFirstTurnPreservesFramesAndOldTarget(t *testing.T) {
	client, serverConn := startKeyAdmissionWSServer(t)
	c := newKeyAdmissionGinContext()
	EnsureOpenAIWSIngressReader(c, serverConn)
	reader := openAIWSIngressReaderFromContext(c)

	started := make(chan struct{}, 1)
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(context.Canceled)
	resultCh := make(chan keyAdmissionResult, 1)
	go func() {
		reservation, err := WaitOpenAIWSKeyAdmission(ctx, c, blockingKeyAdmission(started))
		resultCh <- keyAdmissionResult{reservation: reservation, err: err}
	}()
	<-started
	require.NoError(t, client.Write(context.Background(), coderws.MessageText, []byte(`{"type":"session.update","session":{"model":"gpt-5.2"}}`)))
	require.Eventually(t, func() bool {
		reader.waitMu.Lock()
		defer reader.waitMu.Unlock()
		return len(reader.waitFrames) == 1
	}, 2*time.Second, 10*time.Millisecond)
	// A cancel for an older response must not cancel the pending turn.
	require.NoError(t, client.Write(context.Background(), coderws.MessageText, []byte(`{"type":"response.cancel","response_id":"resp_old"}`)))
	require.Eventually(t, func() bool {
		reader.waitMu.Lock()
		defer reader.waitMu.Unlock()
		return len(reader.waitFrames) == 2
	}, 2*time.Second, 10*time.Millisecond)
	// Retained frames must not end the wait on their own.
	select {
	case got := <-resultCh:
		t.Fatalf("retained frames ended the mode-unknown wait: %+v", got)
	default:
	}

	cancel(context.Canceled)
	got := waitKeyAdmissionResult(t, resultCh)
	require.ErrorIs(t, got.err, context.Canceled)

	readCtx, stop := context.WithTimeout(context.Background(), 2*time.Second)
	defer stop()
	msgType, payload, err := reader.read(readCtx)
	require.NoError(t, err)
	require.Equal(t, coderws.MessageText, msgType)
	require.Contains(t, string(payload), "session.update")
	msgType, payload, err = reader.read(readCtx)
	require.NoError(t, err)
	require.Equal(t, coderws.MessageText, msgType)
	require.Contains(t, string(payload), "resp_old")
}

// TestOpenAIWSKeyAdmissionPlainModeDoesNotConsumeCancel keeps established
// native/bridge behavior: outside passthrough the gate only observes the
// reader, so a cancel frame is left for the normal consumer instead of
// extending protocol handling.
func TestOpenAIWSKeyAdmissionPlainModeDoesNotConsumeCancel(t *testing.T) {
	client, serverConn := startKeyAdmissionWSServer(t)
	c := newKeyAdmissionGinContext()
	EnsureOpenAIWSIngressReader(c, serverConn)
	reader := openAIWSIngressReaderFromContext(c)
	reader.setWaitFrameMode(false)

	started := make(chan struct{}, 1)
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(context.Canceled)
	resultCh := make(chan keyAdmissionResult, 1)
	go func() {
		reservation, err := WaitOpenAIWSKeyAdmission(ctx, c, blockingKeyAdmission(started))
		resultCh <- keyAdmissionResult{reservation: reservation, err: err}
	}()
	<-started
	require.NoError(t, client.Write(context.Background(), coderws.MessageText, []byte(`{"type":"response.cancel"}`)))
	select {
	case got := <-resultCh:
		t.Fatalf("plain mode consumed a cancel frame: %+v", got)
	case <-time.After(200 * time.Millisecond):
	}

	cancel(context.Canceled)
	got := waitKeyAdmissionResult(t, resultCh)
	require.ErrorIs(t, got.err, context.Canceled)
	readCtx, stop := context.WithTimeout(context.Background(), 2*time.Second)
	defer stop()
	_, payload, err := reader.read(readCtx)
	require.NoError(t, err)
	require.Contains(t, string(payload), "response.cancel")
}

func TestOpenAIWSKeyAdmissionPassthroughOverlapRejected(t *testing.T) {
	client, serverConn := startKeyAdmissionWSServer(t)
	c := newKeyAdmissionGinContext()
	EnsureOpenAIWSIngressReader(c, serverConn)
	openAIWSIngressReaderFromContext(c).setWaitFrameMode(true)

	started := make(chan struct{}, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	resultCh := make(chan keyAdmissionResult, 1)
	go func() {
		reservation, err := WaitOpenAIWSKeyAdmission(ctx, c, blockingKeyAdmission(started))
		resultCh <- keyAdmissionResult{reservation: reservation, err: err}
	}()
	<-started
	require.NoError(t, client.Write(context.Background(), coderws.MessageText, []byte(`{"type":"response.create","model":"gpt-5.1"}`)))

	got := waitKeyAdmissionResult(t, resultCh)
	require.Nil(t, got.reservation)
	var closeErr *OpenAIWSClientCloseError
	require.ErrorAs(t, got.err, &closeErr)
	require.Equal(t, coderws.StatusPolicyViolation, closeErr.StatusCode())
	require.Contains(t, closeErr.Reason(), "overlapping response.create")
}

func TestOpenAIWSKeyAdmissionPreservesClientFrames(t *testing.T) {
	client, serverConn := startKeyAdmissionWSServer(t)
	c := newKeyAdmissionGinContext()
	EnsureOpenAIWSIngressReader(c, serverConn)
	reader := openAIWSIngressReaderFromContext(c)
	reader.setWaitFrameMode(true)

	started := make(chan struct{}, 1)
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(context.Canceled)
	resultCh := make(chan keyAdmissionResult, 1)
	go func() {
		reservation, err := WaitOpenAIWSKeyAdmission(ctx, c, blockingKeyAdmission(started))
		resultCh <- keyAdmissionResult{reservation: reservation, err: err}
	}()
	<-started
	require.NoError(t, client.Write(context.Background(), coderws.MessageText, []byte(`{"type":"session.update","session":{"model":"gpt-5.2"}}`)))
	require.Eventually(t, func() bool {
		reader.waitMu.Lock()
		defer reader.waitMu.Unlock()
		return len(reader.waitFrames) == 1
	}, 2*time.Second, 10*time.Millisecond)
	// A cancel for an older response must not cancel the pending turn.
	require.NoError(t, client.Write(context.Background(), coderws.MessageText, []byte(`{"type":"response.cancel","response_id":"resp_old"}`)))
	require.Eventually(t, func() bool {
		reader.waitMu.Lock()
		defer reader.waitMu.Unlock()
		return len(reader.waitFrames) == 2
	}, 2*time.Second, 10*time.Millisecond)
	cancel(context.Canceled)
	got := waitKeyAdmissionResult(t, resultCh)
	require.ErrorIs(t, got.err, context.Canceled)

	readCtx, stop := context.WithTimeout(context.Background(), 2*time.Second)
	defer stop()
	msgType, payload, err := reader.read(readCtx)
	require.NoError(t, err)
	require.Equal(t, coderws.MessageText, msgType)
	require.Contains(t, string(payload), "session.update")
	msgType, payload, err = reader.read(readCtx)
	require.NoError(t, err)
	require.Equal(t, coderws.MessageText, msgType)
	require.Contains(t, string(payload), "resp_old")
}

func TestOpenAIWSKeyAdmissionLeaseLostKeepsTypedClose(t *testing.T) {
	_, serverConn := startKeyAdmissionWSServer(t)
	c := newKeyAdmissionGinContext()
	EnsureOpenAIWSIngressReader(c, serverConn)

	started := make(chan struct{}, 1)
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(context.Canceled)
	resultCh := make(chan keyAdmissionResult, 1)
	go func() {
		reservation, err := WaitOpenAIWSKeyAdmission(ctx, c, blockingKeyAdmission(started))
		resultCh <- keyAdmissionResult{reservation: reservation, err: err}
	}()
	<-started
	cancel(ErrOpenAIWSIngressLeaseLost)

	got := waitKeyAdmissionResult(t, resultCh)
	require.Nil(t, got.reservation)
	require.False(t, IsOpenAIWSClientGoneError(got.err))
	var closeErr *OpenAIWSClientCloseError
	require.ErrorAs(t, got.err, &closeErr)
	require.Equal(t, coderws.StatusTryAgainLater, closeErr.StatusCode())
	require.Contains(t, closeErr.Reason(), "lease lost")
	require.ErrorIs(t, got.err, ErrOpenAIWSIngressLeaseLost)
}

func TestOpenAIWSKeyAdmissionOverflowKeepsBoundedPolicyClose(t *testing.T) {
	client, serverConn := startKeyAdmissionWSServer(t)
	c := newKeyAdmissionGinContext()
	EnsureOpenAIWSIngressReader(c, serverConn)
	// Pin the established plain (native/bridge) state: the gate does not consume
	// frames there, so the socket read-ahead backlog trips the existing policy
	// close instead of growing another queue.
	openAIWSIngressReaderFromContext(c).setWaitFrameMode(false)

	started := make(chan struct{}, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	resultCh := make(chan keyAdmissionResult, 1)
	go func() {
		reservation, err := WaitOpenAIWSKeyAdmission(ctx, c, blockingKeyAdmission(started))
		resultCh <- keyAdmissionResult{reservation: reservation, err: err}
	}()
	<-started
	writeCtx, stop := context.WithTimeout(context.Background(), 2*time.Second)
	defer stop()
	for i := 0; i < openAIWSIngressReaderFrameLimit+1; i++ {
		if err := client.Write(writeCtx, coderws.MessageText, []byte(`{"type":"conversation.item.create"}`)); err != nil {
			break
		}
	}
	// Echo the policy close so the reader's close handshake completes promptly.
	readCtx, stopRead := context.WithTimeout(context.Background(), 2*time.Second)
	defer stopRead()
	_, _, readErr := client.Read(readCtx)
	require.Equal(t, coderws.StatusPolicyViolation, coderws.CloseStatus(readErr))

	got := waitKeyAdmissionResult(t, resultCh)
	require.Nil(t, got.reservation)
	var closeErr *OpenAIWSClientCloseError
	require.ErrorAs(t, got.err, &closeErr)
	require.Equal(t, coderws.StatusPolicyViolation, closeErr.StatusCode())
	require.Contains(t, closeErr.Reason(), "too many pending websocket requests")
	require.False(t, IsOpenAIWSClientGoneError(got.err))
}
