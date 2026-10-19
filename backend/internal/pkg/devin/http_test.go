package devin

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func localHTTPDevinClient(t *testing.T, chat http.HandlerFunc) *Client {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc(authPath, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/proto" || r.Header.Get("Authorization") != "" {
			t.Error("incorrect native HTTP auth request")
			http.Error(w, "bad auth shape", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/proto")
		_, _ = w.Write([]byte{0x0a, 0x03, 'j', 'w', 't'})
	})
	mux.HandleFunc(ChatPath, func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil || len(body) < 5 || body[0] != 1 || r.Header.Get("Content-Type") != "application/connect+proto" || r.Header.Get("Authorization") != "" {
			t.Error("incorrect native HTTP chat request")
			http.Error(w, "bad chat shape", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/connect+proto")
		chat(w, r)
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	target, _ := url.Parse(server.URL)
	client := server.Client()
	t.Cleanup(client.CloseIdleConnections)
	return &Client{Do: func(req *http.Request) (*http.Response, error) {
		// Test-only routing to the in-process peer, while exercising net/http
		// request serialization, chunked response bodies and cancellation.
		local := req.Clone(req.Context())
		local.URL.Scheme, local.URL.Host = target.Scheme, target.Host
		return client.Do(local)
	}}
}

func TestDevinRealHTTPChunkedConnectStream(t *testing.T) {
	message := ps(nil, 3, "hello via HTTP")
	message = pb(message, 7, pv(pv(nil, 2, 8), 3, 3))
	frame, err := gzipFrame(message)
	if err != nil {
		t.Fatal(err)
	}
	frames := append(frame, connectFrame(2, []byte(`{}`))...)
	client := localHTTPDevinClient(t, func(w http.ResponseWriter, _ *http.Request) {
		for len(frames) > 0 {
			n := min(7, len(frames))
			if _, err := w.Write(frames[:n]); err != nil {
				return
			}
			w.(http.Flusher).Flush()
			frames = frames[n:]
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	resp, err := client.Chat(ctx, "synthetic", "cascade", DefaultModels()[0], []byte(streamBody))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "hello via HTTP") || !strings.Contains(string(body), `"total_tokens":11`) || !strings.Contains(string(body), "data: [DONE]") {
		t.Fatal("chunked/gzipped HTTP response did not produce complete text, usage and terminal SSE")
	}
}

func TestDevinRealHTTPCancellationClosesUpstream(t *testing.T) {
	closed := make(chan struct{})
	client := localHTTPDevinClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(connectFrame(0, ps(nil, 3, "first")))
		w.(http.Flusher).Flush()
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
			t.Error("upstream HTTP request was not canceled")
		}
		close(closed)
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	resp, err := client.Chat(ctx, "synthetic", "cascade", DefaultModels()[0], []byte(streamBody))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	reader := bufio.NewReader(resp.Body)
	if _, err := reader.ReadString('\n'); err != nil {
		t.Fatal(err)
	}
	cancel()
	body, err := io.ReadAll(reader)
	if !errors.Is(err, context.Canceled) || strings.Contains(string(body), "[DONE]") {
		t.Fatal("canceled HTTP stream was not reported as canceled")
	}
	await(t, closed)
}
