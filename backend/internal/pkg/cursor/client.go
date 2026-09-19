package cursor

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	utls "github.com/refraction-networking/utls"
	"golang.org/x/net/http2"
	"golang.org/x/net/proxy"
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

	// cursorDialTimeout bounds TCP establishment to the (possibly proxied)
	// endpoint. TLS handshake is bounded by the caller's context; h2
	// ReadIdleTimeout/PingTimeout below surface dead connections that never
	// answer, so no overall client Timeout is needed (it would also kill
	// streams that legitimately run for minutes).
	cursorDialTimeout = 30 * time.Second
	// cursorUnaryTimeout bounds whole short requests (token refresh).
	cursorUnaryTimeout = 30 * time.Second
)

// Client communicates with Cursor's backend using Connect-RPC over HTTP/2.
type Client struct {
	// HTTPClient overrides the shared transport-backed client when set.
	HTTPClient *http.Client
	Creds      Credentials
	BaseURL    string // override agent base URL; defaults to BaseURLAgent
	APIBaseURL string // override api2 base URL; defaults to BaseURLAPI
	ProxyURL   string // optional HTTP CONNECT / SOCKS5 proxy
}

// NewClient creates a Client. HTTP connections come from the package-shared
// transport pool (keyed by ProxyURL) so TLS handshakes are reused across
// requests; override HTTPClient to fully control the dialing.
func NewClient(creds Credentials) *Client {
	return &Client{Creds: creds}
}

func (c *Client) httpClient() (*http.Client, error) {
	if c.HTTPClient != nil {
		return c.HTTPClient, nil
	}
	return StreamingHTTPClient(c.ProxyURL)
}

// sharedTransports pools HTTP/2 transports by proxy URL. Auth rides in
// per-request headers, so connections can be reused across accounts.
var sharedTransports sync.Map // proxyURL string -> *http2.Transport

func sharedTransport(proxyURL string) (*http2.Transport, error) {
	if v, ok := sharedTransports.Load(proxyURL); ok {
		return v.(*http2.Transport), nil
	}
	tr, err := newChromeHTTP2Transport(proxyURL)
	if err != nil {
		return nil, err
	}
	loaded, _ := sharedTransports.LoadOrStore(proxyURL, tr)
	return loaded.(*http2.Transport), nil
}

// StreamingHTTPClient returns an *http.Client for long-lived Cursor streams.
// It intentionally has no overall Timeout: that limit includes reading the
// response body and would kill generations lasting past the limit. The stream
// lifetime is bounded by the caller's request context instead.
func StreamingHTTPClient(proxyURL string) (*http.Client, error) {
	tr, err := sharedTransport(proxyURL)
	if err != nil {
		return nil, err
	}
	return &http.Client{Transport: tr}, nil
}

// UnaryHTTPClient returns an *http.Client with an overall timeout for short
// requests such as the OAuth token exchange.
func UnaryHTTPClient(proxyURL string) (*http.Client, error) {
	tr, err := sharedTransport(proxyURL)
	if err != nil {
		return nil, err
	}
	return &http.Client{Transport: tr, Timeout: cursorUnaryTimeout}, nil
}

// NewHTTP2Transport returns the default streaming client (direct connection).
func NewHTTP2Transport() *http.Client {
	client, err := StreamingHTTPClient("")
	if err != nil {
		// unreachable today: direct dialing validates nothing up front
		return &http.Client{Transport: &http2.Transport{}}
	}
	return client
}

func newChromeHTTP2Transport(proxyURL string) (*http2.Transport, error) {
	if proxyURL != "" {
		if _, err := url.Parse(proxyURL); err != nil {
			return nil, fmt.Errorf("cursor: parse proxy url: %w", err)
		}
	}
	return &http2.Transport{
		DialTLSContext: func(ctx context.Context, network, addr string, _ *tls.Config) (net.Conn, error) {
			return dialCursorTLS(ctx, proxyURL, network, addr)
		},
		ReadIdleTimeout: nalHeartbeatInterval * 6,
		PingTimeout:     nalHeartbeatInterval * 3,
	}, nil
}

// dialCursorTLS connects to addr (directly or through proxyURL) and completes
// a Chromium-fingerprinted TLS handshake with h2 ALPN.
func dialCursorTLS(ctx context.Context, proxyURL string, network, addr string) (net.Conn, error) {
	d := &net.Dialer{Timeout: cursorDialTimeout, KeepAlive: 30 * time.Second}
	var raw net.Conn
	var err error
	if proxyURL == "" {
		raw, err = d.DialContext(ctx, network, addr)
		if err != nil {
			return nil, err
		}
	} else {
		raw, err = dialProxyConn(ctx, d, proxyURL, addr)
		if err != nil {
			return nil, err
		}
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
}

// dialProxyConn tunnels a TCP connection to addr through an HTTP CONNECT or
// SOCKS5 proxy (mirrors proxyutil's supported schemes).
func dialProxyConn(ctx context.Context, d *net.Dialer, proxyURL, addr string) (net.Conn, error) {
	u, err := url.Parse(proxyURL)
	if err != nil {
		return nil, fmt.Errorf("cursor: parse proxy url: %w", err)
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
		return dialHTTPConnect(ctx, d, u, addr)
	case "socks5", "socks5h":
		forward := &net.Dialer{Timeout: cursorDialTimeout, KeepAlive: 30 * time.Second}
		socksDialer, err := proxy.FromURL(u, forward)
		if err != nil {
			return nil, fmt.Errorf("cursor: create socks5 dialer: %w", err)
		}
		if contextDialer, ok := socksDialer.(proxy.ContextDialer); ok {
			return contextDialer.DialContext(ctx, "tcp", addr)
		}
		return socksDialer.Dial("tcp", addr)
	default:
		return nil, fmt.Errorf("cursor: unsupported proxy scheme: %s", u.Scheme)
	}
}

// dialHTTPConnect establishes a CONNECT tunnel through an HTTP proxy. The
// CONNECT response has no body; the conn is reused for the TLS handshake.
func dialHTTPConnect(ctx context.Context, d *net.Dialer, u *url.URL, addr string) (net.Conn, error) {
	proxyAddr := u.Host
	if u.Port() == "" {
		if strings.EqualFold(u.Scheme, "https") {
			proxyAddr = net.JoinHostPort(u.Hostname(), "443")
		} else {
			proxyAddr = net.JoinHostPort(u.Hostname(), "80")
		}
	}
	conn, err := d.DialContext(ctx, "tcp", proxyAddr)
	if err != nil {
		return nil, fmt.Errorf("cursor: connect to proxy: %w", err)
	}

	req := &http.Request{
		Method: http.MethodConnect,
		URL:    &url.URL{Opaque: addr},
		Host:   addr,
		Header: make(http.Header),
	}
	if u.User != nil {
		password, _ := u.User.Password()
		auth := base64.StdEncoding.EncodeToString([]byte(u.User.Username() + ":" + password))
		req.Header.Set("Proxy-Authorization", "Basic "+auth)
	}
	if err := req.Write(conn); err != nil {
		conn.Close()
		return nil, fmt.Errorf("cursor: write CONNECT request: %w", err)
	}
	resp, err := http.ReadResponse(bufio.NewReader(conn), req)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("cursor: read CONNECT response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		conn.Close()
		return nil, fmt.Errorf("cursor: proxy CONNECT failed: %s", resp.Status)
	}
	return conn, nil
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

	httpClient, err := c.httpClient()
	if err != nil {
		return err
	}
	resp, err := httpClient.Do(req)
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
func (c *Client) StreamChat(ctx context.Context, req AgentRunRequest) (*http.Response, error) {
	payload, _, runID := buildAgentRunMessage(req)
	frame, err := EncodeFrame(payload, false)
	if err != nil {
		return nil, fmt.Errorf("cursor: encode NAL frame: %w", err)
	}

	hosts := nalHosts(c.BaseURL)
	var errs []string
	for _, host := range hosts {
		resp, err := c.streamAgentRun(ctx, host, frame, runID, req)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s%s: %v", host, EndpointAgentRun, err))
			continue
		}
		return resp, nil
	}
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

func (c *Client) streamAgentRun(ctx context.Context, host string, frame []byte, runID string, req AgentRunRequest) (*http.Response, error) {
	pr, pw := io.Pipe()
	lw := &lockedPipeWriter{w: pw}
	go func() {
		if _, err := lw.Write(frame); err != nil {
			lw.CloseWithError(err)
		}
	}()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, host+EndpointAgentRun, pr)
	if err != nil {
		lw.Close()
		return nil, err
	}
	headers := c.nalHeaders(runID)
	for k, v := range headers {
		httpReq.Header.Set(k, v)
	}

	httpClient, err := c.httpClient()
	if err != nil {
		lw.Close()
		return nil, err
	}
	resp, err := httpClient.Do(httpReq)
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
		src:       io.MultiReader(bytes.NewReader(raw), resp.Body),
		body:      resp.Body,
		writer:    lw,
		stopHB:    stopHB,
		blobs:     make(map[string][]byte),
		responder: newExecResponder(req.Tools),
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

// frameWriter receives the client-side protocol frames (exec replies, KV
// results, heartbeats) written while the response is being consumed.
type frameWriter interface {
	Write(p []byte) (int, error)
	Close() error
}

type nalReadCloser struct {
	src       io.Reader
	body      io.Closer
	writer    frameWriter
	stopHB    chan struct{}
	blobs     map[string][]byte
	pending   []byte
	closed    bool
	responder *execResponder
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
		if n.handleServerFrame(frame.Payload) {
			continue
		}
		n.pending = raw
	}
}

// handleServerFrame answers protocol frames that require an inline reply
// (KV blob store, exec handshakes, interaction queries). Answered frames are
// consumed and never reach the downstream parser; conversation content passes
// through untouched.
func (n *nalReadCloser) handleServerFrame(payload []byte) bool {
	if n.handleKV(payload) {
		return true
	}
	return n.responder.handle(payload, n.writeReply)
}

type execResponder struct {
	tools    []AgentTool
	declared map[string]struct{}
}

func newExecResponder(tools []AgentTool) *execResponder {
	if len(tools) == 0 {
		return nil
	}
	declared := make(map[string]struct{}, len(tools))
	for _, tool := range tools {
		declared[strings.ToLower(tool.Name)] = struct{}{}
	}
	return &execResponder{tools: tools, declared: declared}
}

// handle answers exec and interaction-query frames. Declared MCP tool calls
// receive the external-handoff acknowledgment (the API caller executes the
// tool and delivers the result on its next request); anything else the gateway
// cannot serve is explicitly rejected so the server-side turn never strands
// on a frame nobody will answer.
func (r *execResponder) handle(payload []byte, writeReply func([]byte)) bool {
	if r == nil {
		return false
	}
	exec := findAgentServerExec(payload)
	if exec != nil {
		frame := parseExecServerFrame(exec)
		var reply []byte
		switch frame.Kind {
		case execKindRequestContext:
			reply = EncodeRequestContextReply(frame.ID, frame.ExecID, r.tools)
		case execKindMcpCall:
			_, isDeclared := r.declared[strings.ToLower(frame.Tool.Name)]
			if isDeclared {
				reply = EncodeMcpHandoffReply(frame.ID, frame.ExecID)
			} else {
				reply = EncodeMcpToolNotFoundReply(frame.ID, frame.ExecID, frame.Tool.Name, declaredToolNames(r.tools))
			}
		case execKindOther:
			reply = EncodeExecThrowReply(frame.ID, "unsupported exec request", "unsupported_exec_variant")
		default:
			return false
		}
		writeReply(reply)
		return true
	}
	if queryID, queryField, ok := findAgentServerQuery(payload); ok {
		if reply := EncodeInteractionRejectedReply(queryID, queryField); reply != nil {
			writeReply(reply)
		}
		return true
	}
	return false
}

func (n *nalReadCloser) writeReply(reply []byte) {
	if len(reply) == 0 {
		return
	}
	frame, err := EncodeFrame(reply, false)
	if err != nil {
		return
	}
	_, _ = n.writer.Write(frame)
}

func declaredToolNames(tools []AgentTool) []string {
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Name)
	}
	return names
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
