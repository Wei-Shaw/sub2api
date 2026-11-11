package httputil

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestBodyReadTimeoutReleasesStalledUpload(t *testing.T) {
	readDone := make(chan error, 1)
	srv := httptest.NewServer(WithBodyReadTimeout(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := io.ReadAll(r.Body)
		readDone <- err
	}), 100*time.Millisecond))
	defer srv.Close()
	conn, err := net.Dial("tcp", srv.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	if _, err := io.WriteString(conn, "POST / HTTP/1.1\r\nHost: test\r\nContent-Length: 1024\r\n\r\nx"); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-readDone:
		var timeout net.Error
		if !errors.As(err, &timeout) || !timeout.Timeout() {
			t.Fatalf("want upload timeout, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("stalled upload retained handler")
	}
}

func TestBodyReadTimeoutClosesStalledHTTP1Connection(t *testing.T) {
	closed := make(chan struct{}, 1)
	handlerDone := make(chan struct{})
	srv := httptest.NewUnstartedServer(WithWriteTimeout(WithBodyReadTimeout(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.ReadAll(r.Body); err != nil {
			w.WriteHeader(http.StatusBadRequest)
		}
		close(handlerDone)
	}), 100*time.Millisecond), 100*time.Millisecond))
	srv.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateClosed {
			select {
			case closed <- struct{}{}:
			default:
			}
		}
	}
	srv.Start()
	defer srv.Close()
	conn, err := net.Dial("tcp", srv.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	// Keep the remainder small enough for net/http's automatic post-handler
	// drain. Clearing the failed read's deadline would strand that drain even
	// though the application handler itself has already returned.
	if _, err := io.WriteString(conn, "POST / HTTP/1.1\r\nHost: test\r\nContent-Length: 1024\r\n\r\nx"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-handlerDone:
	case <-time.After(2 * time.Second):
		t.Fatal("stalled upload retained handler")
	}
	select {
	case <-closed:
	case <-time.After(2 * time.Second):
		t.Fatal("timed-out upload retained the connection while net/http drained the unread body")
	}
}

func TestBodyReadTimeoutClosesRejectedHTTP1Upload(t *testing.T) {
	for _, partialRead := range []bool{false, true} {
		for _, response := range []string{"buffered", "flush", "large_write", "close_body"} {
			name := response + "/unread"
			if partialRead {
				name = response + "/partial_read"
			}
			t.Run(name, func(t *testing.T) {
				handlerDone := make(chan error, 1)
				closed := make(chan struct{}, 1)
				handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if partialRead {
						if _, err := io.ReadFull(r.Body, make([]byte, 1)); err != nil {
							handlerDone <- err
							return
						}
					}
					ctx, release := WithRequestBodyMemoryBudget(r.Context(), NewBodyMemoryBudget(1))
					defer release()
					_, err := ReadRequestBodyWithPrealloc(r.WithContext(ctx))
					if !errors.Is(err, ErrRequestBodyMemoryBudget) {
						handlerDone <- errors.New("request did not exhaust its body budget")
						return
					}
					w.WriteHeader(http.StatusServiceUnavailable)
					switch response {
					case "flush":
						_ = http.NewResponseController(w).Flush()
					case "large_write":
						_, _ = w.Write([]byte(strings.Repeat("x", 8192)))
					case "close_body":
						_ = r.Body.Close()
					default:
						_, _ = io.WriteString(w, "busy")
					}
					handlerDone <- nil
				})
				srv := httptest.NewUnstartedServer(WithWriteTimeout(WithBodyReadTimeout(handler, 50*time.Millisecond), 100*time.Millisecond))
				srv.Config.ConnState = func(_ net.Conn, state http.ConnState) {
					if state == http.StateClosed {
						select {
						case closed <- struct{}{}:
						default:
						}
					}
				}
				srv.Start()
				defer srv.Close()
				conn, err := net.Dial("tcp", srv.Listener.Addr().String())
				if err != nil {
					t.Fatal(err)
				}
				defer func() { _ = conn.Close() }()
				if _, err := io.WriteString(conn, "POST / HTTP/1.1\r\nHost: test\r\nContent-Length: 1024\r\n\r\nx"); err != nil {
					t.Fatal(err)
				}
				select {
				case err := <-handlerDone:
					if err != nil {
						t.Fatal(err)
					}
				case <-time.After(2 * time.Second):
					t.Fatal("rejection blocked while writing the response or closing the body")
				}
				select {
				case <-closed:
				case <-time.After(2 * time.Second):
					t.Fatal("rejected upload retained its HTTP/1 connection")
				}
			})
		}
	}
}

func TestBodyReadTimeoutDoesNotLimitHTTP1ResponseLifetime(t *testing.T) {
	for _, readAll := range []bool{false, true} {
		name := "implicit_drain"
		if readAll {
			name = "complete_body"
		}
		t.Run(name, func(t *testing.T) {
			handlerErr := make(chan error, 1)
			srv := httptest.NewServer(WithBodyReadTimeout(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var err error
				if readAll {
					_, err = io.Copy(io.Discard, r.Body)
				} else {
					_, err = io.ReadFull(r.Body, make([]byte, 1))
				}
				if err != nil {
					handlerErr <- err
					return
				}
				_, _ = io.WriteString(w, "first\n")
				if err = http.NewResponseController(w).Flush(); err != nil {
					handlerErr <- err
					return
				}
				// The first flush implicitly consumes any unread body. Its read
				// deadline must not cancel the response during upstream thinking.
				timer := time.NewTimer(300 * time.Millisecond)
				defer timer.Stop()
				select {
				case <-r.Context().Done():
					handlerErr <- r.Context().Err()
					return
				case <-timer.C:
				}
				_, err = io.WriteString(w, "last\n")
				handlerErr <- err
			}), 100*time.Millisecond))
			defer srv.Close()
			client := srv.Client()
			client.Timeout = 3 * time.Second
			resp, err := client.Post(srv.URL, "application/json", strings.NewReader(`{"input":"hello"}`))
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = resp.Body.Close() }()
			body, err := io.ReadAll(resp.Body)
			if err != nil || string(body) != "first\nlast\n" {
				t.Fatalf("response was truncated: %q, %v", body, err)
			}
			if err := <-handlerErr; err != nil {
				t.Fatalf("response lifetime was limited by the upload deadline: %v", err)
			}
		})
	}
}

func TestBodyReadTimeoutHTTP2DoesNotCancelOtherStreams(t *testing.T) {
	readDone := make(chan error, 1)
	srv := httptest.NewUnstartedServer(WithBodyReadTimeout(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/slow" {
			_, err := io.ReadAll(r.Body)
			readDone <- err
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}), 100*time.Millisecond))
	srv.EnableHTTP2 = true
	srv.StartTLS()
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	reader, writer := io.Pipe()
	defer func() { _ = writer.Close() }()
	slow, _ := http.NewRequestWithContext(ctx, http.MethodPost, srv.URL+"/slow", reader)
	clientDone := make(chan struct{})
	go func() {
		defer close(clientDone)
		resp, err := srv.Client().Do(slow)
		if err == nil {
			_ = resp.Body.Close()
		}
	}()
	defer func() {
		cancel()
		_ = writer.Close()
		<-clientDone
	}()
	select {
	case err := <-readDone:
		if err == nil {
			t.Fatal("stalled HTTP/2 upload accepted")
		}
	case <-ctx.Done():
		t.Fatal("stalled HTTP/2 upload retained handler")
	}
	fast, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/fast", nil)
	resp, err := srv.Client().Do(fast)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.ProtoMajor != 2 || resp.StatusCode != http.StatusNoContent {
		t.Fatalf("other stream failed: %s %s", resp.Proto, resp.Status)
	}
}
