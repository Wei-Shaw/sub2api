package service

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/packguard"
	openaiwsv2 "github.com/Wei-Shaw/sub2api/internal/service/openai_ws_v2"
	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
)

type prepaidWSAccountKey struct{}

func withPrepaidWSAccount(ctx context.Context, id int64) context.Context {
	return context.WithValue(ctx, prepaidWSAccountKey{}, id)
}

type prepaidWSRefusal struct{ unavailable bool }

func (e *prepaidWSRefusal) Error() string {
	if e.unavailable {
		return "Funding policy is unavailable; no unmetered upstream request was sent"
	}
	return "Pack-only funding requires metered HTTP text or video task requests; upstream WebSocket writes are not supported"
}

type prepaidWSDialer struct {
	inner  openAIWSClientDialer
	ledger packguard.Ledger
}

func (s *OpenAIGatewayService) prepaidAwareWSDialer(inner openAIWSClientDialer) openAIWSClientDialer {
	if inner == nil {
		return nil
	}
	if _, ok := inner.(*prepaidWSDialer); ok {
		return inner
	}
	if guarded, ok := s.httpUpstream.(*bytePlusPackHTTP); ok {
		return &prepaidWSDialer{inner: inner, ledger: guarded.guard.Ledger}
	}
	// Directly constructed unit-test services have no funding store. The real
	// application requires NewPackGuardedHTTPUpstream through generated wiring.
	return inner
}
func (d *prepaidWSDialer) check(ctx context.Context, id int64, raw string, headers http.Header) error {
	target, err := url.Parse(raw)
	if err != nil {
		target = &url.URL{}
	}
	switch target.Scheme {
	case "wss":
		target.Scheme = "https"
	case "ws":
		target.Scheme = "http"
	}
	auth := strings.Fields(headers.Get("Authorization"))
	key := ""
	if len(auth) == 2 && strings.EqualFold(auth[0], "Bearer") {
		key = auth[1]
	}
	scope, err := d.ledger.Lookup(ctx, id, packguard.HashCredential(key), target)
	if err != nil {
		return NewOpenAIWSClientCloseError(coderws.StatusPolicyViolation, "prepaid funding state unavailable", &prepaidWSRefusal{unavailable: true})
	}
	if scope != nil {
		return NewOpenAIWSClientCloseError(coderws.StatusPolicyViolation, "prepaid-only funding requires HTTP", &prepaidWSRefusal{})
	}
	return nil
}
func (d *prepaidWSDialer) Dial(ctx context.Context, target string, headers http.Header, proxy string) (openAIWSClientConn, int, http.Header, error) {
	id, _ := ctx.Value(prepaidWSAccountKey{}).(int64)
	if err := d.check(ctx, id, target, headers); err != nil {
		return nil, 400, nil, err
	}
	conn, status, responseHeaders, err := d.inner.Dial(ctx, target, headers, proxy)
	if err != nil {
		return conn, status, responseHeaders, err
	}
	return &prepaidWSConn{inner: conn, dialer: d, accountID: id, target: target, headers: headers.Clone()}, status, responseHeaders, nil
}
func (d *prepaidWSDialer) SnapshotTransportMetrics() OpenAIWSTransportMetricsSnapshot {
	if metrics, ok := d.inner.(openAIWSTransportMetricsDialer); ok {
		return metrics.SnapshotTransportMetrics()
	}
	return OpenAIWSTransportMetricsSnapshot{}
}

// Connections can outlive policy enrollment. Every upstream write is checked,
// including pooled warmups and turns on an already connected passthrough socket.
type prepaidWSConn struct {
	inner     openAIWSClientConn
	dialer    *prepaidWSDialer
	accountID int64
	target    string
	headers   http.Header
}

func (c *prepaidWSConn) WriteJSON(ctx context.Context, value any) error {
	if err := c.dialer.check(ctx, c.accountID, c.target, c.headers); err != nil {
		return err
	}
	return c.inner.WriteJSON(ctx, value)
}
func (c *prepaidWSConn) ReadMessage(ctx context.Context) ([]byte, error) {
	return c.inner.ReadMessage(ctx)
}
func (c *prepaidWSConn) Ping(ctx context.Context) error { return c.inner.Ping(ctx) }
func (c *prepaidWSConn) Close() error                   { return c.inner.Close() }
func (c *prepaidWSConn) ReadFrame(ctx context.Context) (coderws.MessageType, []byte, error) {
	if inner, ok := c.inner.(openaiwsv2.FrameConn); ok {
		return inner.ReadFrame(ctx)
	}
	return 0, nil, errors.New("upstream frame interface unavailable")
}
func (c *prepaidWSConn) WriteFrame(ctx context.Context, kind coderws.MessageType, body []byte) error {
	if err := c.dialer.check(ctx, c.accountID, c.target, c.headers); err != nil {
		return err
	}
	if inner, ok := c.inner.(openaiwsv2.FrameConn); ok {
		return inner.WriteFrame(ctx, kind, body)
	}
	return errors.New("upstream frame interface unavailable")
}
func (c *prepaidWSConn) SupportsIdlePingWithoutReader() bool {
	inner, ok := c.inner.(openAIWSIdlePingCapable)
	return ok && inner.SupportsIdlePingWithoutReader()
}
func (c *prepaidWSConn) RequiresReaderLoop() bool {
	inner, ok := c.inner.(openAIWSReaderLoopCapable)
	return ok && inner.RequiresReaderLoop()
}
func (c *prepaidWSConn) UpstreamPingCount() int64 {
	if inner, ok := c.inner.(openAIWSUpstreamPingCounter); ok {
		return inner.UpstreamPingCount()
	}
	return 0
}
func (c *prepaidWSConn) CloseNow() error {
	if inner, ok := c.inner.(openAIWSForceCloser); ok {
		return inner.CloseNow()
	}
	return c.inner.Close()
}

func finalizePrepaidWSError(c *gin.Context, err error) error {
	var refusal *prepaidWSRefusal
	if !errors.As(err, &refusal) {
		return err
	}
	if c != nil && c.Request != nil && strings.EqualFold(c.GetHeader("Upgrade"), "websocket") {
		return NewOpenAIWSClientCloseError(coderws.StatusPolicyViolation, "prepaid-only funding requires HTTP", err)
	}
	if c != nil {
		MarkResponseCommitted(c)
		if !c.Writer.Written() {
			status, code := 400, "prepaid_pack_unsupported"
			if refusal.unavailable {
				status, code = 503, "prepaid_pack_state_unavailable"
			}
			c.Header("Cache-Control", "no-store")
			c.JSON(status, gin.H{"error": gin.H{"type": "prepaid_pack_error", "code": code, "message": refusal.Error()}})
		}
	}
	return refusal
}
