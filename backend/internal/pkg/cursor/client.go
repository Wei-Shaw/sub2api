package cursor

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	utls "github.com/refraction-networking/utls"
	"golang.org/x/net/http2"
)

const (
	BaseURLAPI          = "https://api2.cursor.sh"
	BaseURLAgent        = "https://agentn.api5.cursor.sh"
	BaseURLAgentN       = "https://agentn.api5.cursor.sh"
	BaseURLAgentNGlobal = "https://agentn.global.api5.cursor.sh"
	BaseURLAgentNEU     = "https://agentn-gcpp-eucentral.api5.cursor.sh"

	EndpointAgentRun    = "/agent.v1.AgentService/Run"
	EndpointAgentModels = "/agent.v1.AgentService/GetUsableModels"
	EndpointChat        = "/aiserver.v1.ChatService/StreamUnifiedChatWithTools"
	EndpointChatSimple  = "/aiserver.v1.ChatService/StreamUnifiedChat"
	EndpointChatLegacy  = "/aiserver.v1.AiService/StreamChat"
	EndpointModels      = "/aiserver.v1.AiService/AvailableModels"
	EndpointToken       = "/oauth/token"

	DefaultAuthClientID = "KbZUR41cY7W6zRSdpSUJ7I7mLYBKOCmB"

	nalHeartbeatInterval = 5 * time.Second
)

// Client communicates with Cursor's backend using Connect-RPC over HTTP/2.
type Client struct {
	HTTPClient *http.Client
	Creds      Credentials
	BaseURL    string // override agent base URL; defaults to BaseURLAgent
	APIBaseURL string // override api2 base URL; defaults to BaseURLAPI
	ProxyURL   string // optional SOCKS5/HTTP proxy
}

// NewClient creates a Client with an HTTP/2-capable transport.
func NewClient(creds Credentials) *Client {
	return &Client{
		HTTPClient: NewHTTP2Transport(),
		Creds:      creds,
	}
}

// NewHTTP2Transport returns an *http.Client forced to HTTP/2 with a Chromium
// ClientHello. Cursor's chat endpoints fingerprint Go's default TLS stack.
func NewHTTP2Transport() *http.Client {
	return &http.Client{
		Timeout:   180 * time.Second,
		Transport: newChromeHTTP2Transport(),
	}
}

func newChromeHTTP2Transport() *http2.Transport {
	return &http2.Transport{
		DialTLSContext: func(ctx context.Context, network, addr string, _ *tls.Config) (net.Conn, error) {
			d := net.Dialer{Timeout: 30 * time.Second}
			raw, err := d.DialContext(ctx, network, addr)
			if err != nil {
				return nil, err
			}
			host, _, err := net.SplitHostPort(addr)
			if err != nil {
				host = addr
			}
			uconn := utls.UClient(raw, &utls.Config{ServerName: host}, utls.HelloChrome_Auto)
			if err := uconn.HandshakeContext(ctx); err != nil {
				raw.Close()
				return nil, err
			}
			if uconn.ConnectionState().NegotiatedProtocol != "h2" {
				raw.Close()
				return nil, fmt.Errorf("cursor: ALPN is %q, want h2", uconn.ConnectionState().NegotiatedProtocol)
			}
			return uconn, nil
		},
	}
}

// EstablishSession calls AvailableModels on api2 to warm up the session.
func (c *Client) EstablishSession(ctx context.Context) error {
	url := BaseURLAPI + EndpointModels
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, http.NoBody)
	if err != nil {
		return err
	}
	headers := BuildHeaders(c.Creds)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	req.Header.Set("accept-encoding", "gzip")
	req.Header.Set("content-type", "application/proto")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("cursor: establish session: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("cursor: establish session status %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

// StreamChat sends a chat completion request and returns the raw HTTP response
// whose body contains Connect-RPC streaming frames. The caller must close the body.
func (c *Client) StreamChat(ctx context.Context, messages []ChatMessage, model string, thinkingLevel int) (*http.Response, error) {
	payload, _, runID := BuildAgentClientMessage(messages, model)
	frame, err := EncodeFrame(payload, false)
	if err != nil {
		return nil, fmt.Errorf("cursor: encode NAL frame: %w", err)
	}

	hosts := nalHosts(c.BaseURL)
	var errs []string
	for _, host := range hosts {
		resp, err := c.streamAgentRun(ctx, host, frame, runID)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s%s: %v", host, EndpointAgentRun, err))
			continue
		}
		return resp, nil
	}
	_ = thinkingLevel
	if len(errs) > 0 {
		return nil, fmt.Errorf("cursor: stream chat: %s", strings.Join(errs, " | "))
	}
	return nil, fmt.Errorf("cursor: stream chat: no NAL endpoint succeeded")
}

func nalHosts(override string) []string {
	if override != "" && override != BaseURLAPI {
		return []string{override}
	}
	return []string{BaseURLAgentNGlobal, BaseURLAgentN, BaseURLAgentNEU}
}

func (c *Client) streamAgentRun(ctx context.Context, host string, frame []byte, runID string) (*http.Response, error) {
	pr, pw := io.Pipe()
	lw := &lockedPipeWriter{w: pw}
	go func() {
		if _, err := lw.Write(frame); err != nil {
			lw.CloseWithError(err)
		}
	}()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, host+EndpointAgentRun, pr)
	if err != nil {
		lw.Close()
		return nil, err
	}
	headers := c.nalHeaders(runID)
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		lw.Close()
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		lw.Close()
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}

	raw, first, err := ReadRawFrame(resp.Body)
	if err != nil {
		resp.Body.Close()
		lw.Close()
		return nil, fmt.Errorf("first frame: %w", err)
	}
	if msg := ConnectErrorJSON(first); msg != "" || isRejectedCursorStream(first.Payload) {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		lw.Close()
		if msg == "" {
			msg = summarizeCursorError(first.Payload)
		}
		return nil, fmt.Errorf("%s", msg)
	}

	stopHB := make(chan struct{})
	go nalHeartbeatLoop(lw, stopHB)

	resp.Body = &nalReadCloser{
		src:    io.MultiReader(bytes.NewReader(raw), resp.Body),
		body:   resp.Body,
		writer: lw,
		stopHB: stopHB,
		blobs:  make(map[string][]byte),
	}
	return resp, nil
}

func (c *Client) nalHeaders(runID string) map[string]string {
	creds := c.Creds
	creds.ClientLayout = "unifiedAgent"
	h := BuildHeaders(creds)
	h["x-original-request-id"] = runID
	h["x-amzn-trace-id"] = fmt.Sprintf("Root=%s", h["x-request-id"])
	h["connect-accept-encoding"] = "gzip"
	h["accept"] = "application/connect+proto"
	return h
}

type lockedPipeWriter struct {
	mu sync.Mutex
	w  *io.PipeWriter
}

func (l *lockedPipeWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}

func (l *lockedPipeWriter) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Close()
}

func (l *lockedPipeWriter) CloseWithError(err error) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.CloseWithError(err)
}

func nalHeartbeatLoop(w *lockedPipeWriter, stop <-chan struct{}) {
	ticker := time.NewTicker(nalHeartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			frame, err := EncodeFrame(encodeClientHeartbeat(), false)
			if err != nil {
				return
			}
			if _, err := w.Write(frame); err != nil {
				return
			}
		}
	}
}

type nalReadCloser struct {
	src     io.Reader
	body    io.Closer
	writer  *lockedPipeWriter
	stopHB  chan struct{}
	blobs   map[string][]byte
	pending []byte
	closed  bool
}

func (n *nalReadCloser) Read(p []byte) (int, error) {
	for {
		if len(n.pending) > 0 {
			copied := copy(p, n.pending)
			n.pending = n.pending[copied:]
			return copied, nil
		}
		raw, frame, err := ReadRawFrame(n.src)
		if err != nil {
			return 0, err
		}
		if n.handleKV(frame.Payload) {
			continue
		}
		n.pending = raw
	}
}

func (n *nalReadCloser) handleKV(payload []byte) bool {
	op := parseAgentKV(payload)
	if op == nil {
		return false
	}
	var reply []byte
	switch {
	case len(op.setBlob) > 0:
		n.blobs[string(op.setBlob)] = append([]byte(nil), op.setData...)
		reply = encodeKVSetResult(op.id)
	case len(op.getBlob) > 0:
		if data, ok := n.blobs[string(op.getBlob)]; ok {
			reply = encodeKVGetResult(op.id, data, "")
		} else {
			reply = encodeKVGetResult(op.id, nil, "blob not found")
		}
	default:
		return true
	}
	frame, err := EncodeFrame(reply, false)
	if err != nil {
		return true
	}
	_, _ = n.writer.Write(frame)
	return true
}

func (n *nalReadCloser) Close() error {
	if n.closed {
		return nil
	}
	n.closed = true
	close(n.stopHB)
	_ = n.writer.Close()
	if n.body != nil {
		return n.body.Close()
	}
	return nil
}

func isDeprecatedCursorStream(prefix []byte) bool {
	return bytes.Contains(prefix, []byte("ERROR_DEPRECATED")) ||
		bytes.Contains(prefix, []byte("Request type deprecated")) ||
		bytes.Contains(prefix, []byte("outdated version of Cursor"))
}

func isRejectedCursorStream(prefix []byte) bool {
	return isDeprecatedCursorStream(prefix) ||
		bytes.Contains(prefix, []byte("Update Required")) ||
		bytes.Contains(prefix, []byte("ERROR_GPT_4_VISION_PREVIEW_RATE_LIMIT"))
}

func summarizeCursorError(prefix []byte) string {
	if i := bytes.IndexByte(prefix, '{'); i >= 0 {
		s := prefix[i:]
		if len(s) > 300 {
			s = s[:300]
		}
		return string(s)
	}
	return "blocked stream"
}

// RefreshToken exchanges a refresh token for a new access token.
func RefreshToken(ctx context.Context, httpClient *http.Client, refreshToken string) (newAccessToken string, err error) {
	return refreshTokenImpl(ctx, httpClient, refreshToken)
}
