package service

import (
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
)

// panickingStreamSource 在第一次 Read 时 panic，用来模拟“上游返回的字节流触发解析
// 越界”这类场景：panic 发生在泵协程里，gin 的 Recovery() 不覆盖它。
type panickingStreamSource struct {
	message string
	closed  bool
}

func (r *panickingStreamSource) Read([]byte) (int, error) {
	panic(r.message)
}

func (r *panickingStreamSource) Close() error {
	r.closed = true
	return nil
}

func TestRecoverStreamGoroutineRunsCleanupWithPanicError(t *testing.T) {
	done := make(chan error, 1)
	go func() {
		defer close(done)
		defer recoverStreamGoroutine("unit pump", func(err error) {
			done <- err
		})
		panic("boom")
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected the panic to be delivered as an error")
		}
		if !strings.Contains(err.Error(), "unit pump") || !strings.Contains(err.Error(), "boom") {
			t.Fatalf("error should name the site and the panic value, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cleanup never ran: consumer would hang")
	}
}

func TestRecoverStreamGoroutineDoesNotRunCleanupWithoutPanic(t *testing.T) {
	called := false
	func() {
		defer recoverStreamGoroutine("unit pump", func(error) { called = true })
	}()
	if called {
		t.Fatal("cleanup must only run on panic")
	}
}

func TestRecoverStreamPipeWriterUnblocksReaderWithError(t *testing.T) {
	reader, writer := io.Pipe()
	go func() {
		defer recoverStreamPipeWriter("unit pipe", writer)
		panic("pipe boom")
	}()

	type readResult struct {
		err error
	}
	results := make(chan readResult, 1)
	go func() {
		_, err := io.ReadAll(reader)
		results <- readResult{err: err}
	}()

	select {
	case result := <-results:
		if result.err == nil {
			t.Fatal("reader saw a clean EOF instead of the panic error")
		}
		if !strings.Contains(result.err.Error(), "pipe boom") {
			t.Fatalf("unexpected reader error: %v", result.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("pipe reader hung: the writer was never closed")
	}
}

// TestAntigravityCompatScannerPanicClosesEventChannel 覆盖 SSE 行泵：泵协程 panic
// 后，消费方必须先看到一个错误事件、再看到通道关闭，而不是阻塞在通道上。
func TestAntigravityCompatScannerPanicClosesEventChannel(t *testing.T) {
	service := &AntigravityGatewayService{}
	body := &panickingStreamSource{message: "sse pump boom"}

	events, stop, _ := service.startAntigravityCompatScanner(body)
	defer stop()

	select {
	case event, ok := <-events:
		if !ok {
			t.Fatal("channel closed without reporting the panic as an error")
		}
		if event.err == nil {
			t.Fatalf("expected an error event, got line %q", event.line)
		}
		if !strings.Contains(event.err.Error(), "sse pump boom") {
			t.Fatalf("unexpected event error: %v", event.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("consumer hung waiting for an event after the pump panicked")
	}

	select {
	case _, ok := <-events:
		if ok {
			t.Fatal("expected the event channel to be closed after the panic")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("event channel was never closed after the pump panicked")
	}
}

// TestGrokBillingPingFilterPanicUnblocksBodyReader 覆盖 io.Pipe 转换协程：panic
// 之后写端必须以错误关闭，否则 http.Response.Body 的读者会永远阻塞。
func TestGrokBillingPingFilterPanicUnblocksBodyReader(t *testing.T) {
	source := &panickingStreamSource{message: "grok filter boom"}
	body := newGrokResponsesBillingPingFilterBody(source, &Account{Platform: PlatformGrok}, 0)
	defer func() { _ = body.Close() }()

	errs := make(chan error, 1)
	go func() {
		_, err := io.ReadAll(body)
		errs <- err
	}()

	select {
	case err := <-errs:
		if err == nil {
			t.Fatal("body reader saw a clean EOF instead of the panic error")
		}
		if !strings.Contains(err.Error(), "grok filter boom") {
			t.Fatalf("unexpected body error: %v", err)
		}
		if errors.Is(err, io.EOF) {
			t.Fatalf("panic must not be reported as EOF: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("body reader hung: the pipe writer was never closed after the panic")
	}
}

// TestResponsesClientToolStreamPanicUnblocksBodyReader 覆盖第二个 io.Pipe 站点。
func TestResponsesClientToolStreamPanicUnblocksBodyReader(t *testing.T) {
	source := &panickingStreamSource{message: "tool stream boom"}
	body := newResponsesClientToolStreamBody(source, apicompat.ResponsesClientToolMapping{}, 0)
	defer func() { _ = body.Close() }()

	errs := make(chan error, 1)
	go func() {
		_, err := io.ReadAll(body)
		errs <- err
	}()

	select {
	case err := <-errs:
		if err == nil {
			t.Fatal("body reader saw a clean EOF instead of the panic error")
		}
		if !strings.Contains(err.Error(), "tool stream boom") {
			t.Fatalf("unexpected body error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("body reader hung: the pipe writer was never closed after the panic")
	}
}
