package muse

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/proxyurl"
	"github.com/Wei-Shaw/sub2api/internal/pkg/proxyutil"
	"github.com/coder/websocket"
	"github.com/google/uuid"
)

const authCheckEndpoint = "https://muse.ai/api/auth/check"

type nativeSocket struct{ conn *websocket.Conn }

func (s nativeSocket) Read(ctx context.Context) ([]byte, error) {
	kind, body, err := s.conn.Read(ctx)
	if err != nil || kind != websocket.MessageBinary {
		return nil, ErrNoiseProtocol
	}
	return body, nil
}
func (s nativeSocket) Write(ctx context.Context, body []byte) error {
	if s.conn.Write(ctx, websocket.MessageBinary, body) != nil {
		return ErrNoiseProtocol
	}
	return nil
}

type nativeConnection struct {
	socket NoiseSocket
	wire   *NoiseTransport
	close  func()
}

// Each operation owns a connection and its cipher state. No retry can reuse an
// encrypted nonce or replay a POST after a transport failure.
func dialNativeNoise(ctx context.Context, session Session, endpoint string) (NoiseSocket, func(), error) {
	base, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return nil, nil, ErrTransportUnqualified
	}
	transport := base.Clone()
	transport.Proxy = nil
	transport.TLSHandshakeTimeout = 10 * time.Second
	if session.ProxyURL != "" {
		_, proxy, err := proxyurl.Parse(session.ProxyURL)
		if err != nil || proxyutil.ConfigureTransportProxy(transport, proxy) != nil {
			return nil, nil, ErrSessionResponse
		}
	}
	client := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	conn, response, err := websocket.Dial(ctx, endpoint, &websocket.DialOptions{HTTPClient: client, HTTPHeader: http.Header{"Origin": {"https://muse.ai"}, "User-Agent": {SessionUserAgent}}})
	if err != nil {
		if response != nil && response.Body != nil {
			_ = response.Body.Close()
		}
		transport.CloseIdleConnections()
		// WebSocket errors may include the credential-bearing URL.
		return nil, nil, ErrNoiseProtocol
	}
	conn.SetReadLimit(65535)
	return nativeSocket{conn}, func() { _ = conn.CloseNow(); transport.CloseIdleConnections() }, nil
}

type nativeBootstrap struct {
	document map[string]any
	expires  *time.Time
	target   GatewayTarget
	viewer   string
	token    *GatewayToken
}

func (p *NativeProvider) bootstrap(ctx context.Context, session Session) (*nativeBootstrap, error) {
	// Include lock contention in the bootstrap budget. Persistence gets an
	// additional detached commit window and cannot expire under network work.
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if session.WithCredentials == nil {
		return p.bootstrapCredentials(ctx, session)
	}
	var boot *nativeBootstrap
	err := session.WithCredentials(ctx, func(latest Session) error {
		var err error
		boot, err = p.bootstrapCredentials(ctx, latest)
		return err
	})
	return boot, err
}

func (p *NativeProvider) bootstrapCredentials(ctx context.Context, session Session) (*nativeBootstrap, error) {
	if p == nil || p.do == nil {
		return nil, ErrTransportUnqualified
	}
	refreshed, err := (&SessionClient{Do: p.do}).Refresh(ctx, session)
	if err != nil {
		return nil, err
	}
	if refreshed.Gateway == nil || refreshed.VMType != "hatch_vm" || refreshed.Check.VMState != "RUNNING" {
		return nil, ErrNoiseTrust
	}
	session.Document = refreshed.Document
	cookies, err := ParseCookieSession(session.Document)
	if err != nil {
		return nil, err
	}
	check, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(check, http.MethodPost, authCheckEndpoint, nil)
	req.Header.Set("Origin", "https://muse.ai")
	req.Header.Set("Referer", "https://muse.ai/")
	req.Header.Set("User-Agent", SessionUserAgent)
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	req.Header.Set("Sec-Fetch-Dest", "empty")
	for _, name := range SessionCookieNames {
		if value := cookies.Cookies[name]; value != "" {
			req.AddCookie(&http.Cookie{Name: name, Value: value})
		}
	}
	response, err := p.do(req, session)
	if err != nil || response == nil || response.Body == nil {
		return nil, ErrSessionResponse
	}
	defer func() { _ = response.Body.Close() }()
	document, _, err := persistSessionCookies(ctx, session, response, cookies)
	if err != nil {
		return nil, err
	}
	session.Document = document
	if response.StatusCode == http.StatusUnauthorized {
		return nil, ErrSessionExpired
	}
	if response.StatusCode != http.StatusOK {
		return nil, ErrSessionResponse
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 64<<10+1))
	var identity struct {
		Outcome string `json:"outcome"`
		Viewer  string `json:"viewer_id"`
		Binding string `json:"session_binding_id"`
	}
	if err != nil || len(body) > 64<<10 || json.Unmarshal(body, &identity) != nil || identity.Outcome != "validated" || !validID(identity.Viewer, 256) || !validID(identity.Binding, 256) {
		return nil, ErrSessionResponse
	}
	token, document, err := (&GatewayClient{Do: p.do}).TokenAndRefresh(ctx, session, *refreshed.Gateway)
	if err != nil {
		return nil, err
	}
	cookies, err = ParseCookieSession(document)
	if err != nil {
		return nil, err
	}
	var expires *time.Time
	for _, timestamp := range cookies.Expires {
		at := time.Unix(timestamp, 0)
		if expires == nil || at.Before(*expires) {
			expires = &at
		}
	}
	return &nativeBootstrap{document: document, target: *refreshed.Gateway, viewer: identity.Viewer, token: token, expires: expires}, nil
}

func (p *NativeProvider) connect(ctx context.Context, session Session) (*nativeConnection, *nativeBootstrap, error) {
	boot, err := p.bootstrap(ctx, session)
	if err != nil {
		return nil, nil, err
	}
	endpoint, err := boot.token.WebSocketURL(boot.target, uuid.NewString())
	if err != nil {
		return nil, nil, err
	}
	socket, closeConn, err := p.dial(ctx, session, endpoint)
	if err != nil {
		return nil, nil, ErrNoiseProtocol
	}
	wire, err := HandshakeNoise(ctx, socket, VerifyStandardNoisePeer)
	if err != nil {
		closeConn()
		return nil, nil, err
	}
	return &nativeConnection{socket: socket, wire: wire, close: closeConn}, boot, nil
}

func (c *nativeConnection) send(ctx context.Context, method, path string, params any) (int64, error) {
	var body []byte
	var err error
	if params != nil {
		body, err = json.Marshal(params)
		if err != nil {
			return 0, ErrInvalid
		}
	}
	id, frames, err := c.wire.EncryptRequest(method, path, []NoiseHeader{{Key: "x-app-id", Value: "hatch-web"}, {Key: "Content-Type", Value: "application/json"}}, body)
	if err != nil {
		return 0, err
	}
	for _, frame := range frames {
		if err = c.socket.Write(ctx, frame); err != nil {
			return 0, err
		}
	}
	return id, nil
}

func (c *nativeConnection) read(ctx context.Context) (*NoiseFrame, error) {
	for {
		frame, err := c.socket.Read(ctx)
		if err != nil {
			return nil, err
		}
		event, err := c.wire.DecryptFrame(frame)
		if err != nil {
			return nil, err
		}
		if event != nil {
			return event, nil
		}
	}
}

func (c *nativeConnection) request(ctx context.Context, method, path string, params any) ([]byte, error) {
	id, err := c.send(ctx, method, path, params)
	if err != nil {
		return nil, err
	}
	var body []byte
	status := 0
	for {
		event, err := c.read(ctx)
		if err != nil {
			return nil, err
		}
		if event.StreamID != id || event.Kind == "reset" {
			return nil, ErrNoiseProtocol
		}
		if event.Kind == "response" {
			status = event.Status
		}
		if len(body)+len(event.Body) > 4<<20 {
			return nil, ErrNoiseProtocol
		}
		body = append(body, event.Body...)
		if event.EndBody {
			if status != http.StatusOK {
				return nil, ErrNoiseProtocol
			}
			return body, nil
		}
	}
}
