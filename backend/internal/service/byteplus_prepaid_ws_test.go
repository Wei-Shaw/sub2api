package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/packguard"
	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
)

type prepaidWSPolicyFixture struct {
	packguard.Ledger
	enrolled bool
	err      error
	account  int64
	hash     string
	scheme   string
}

func (p *prepaidWSPolicyFixture) Ready(context.Context) error { return nil }
func (p *prepaidWSPolicyFixture) Lookup(_ context.Context, id int64, hash string, target *url.URL) (*packguard.Scope, error) {
	p.account, p.hash, p.scheme = id, hash, target.Scheme
	if p.err != nil {
		return nil, p.err
	}
	if p.enrolled {
		return &packguard.Scope{ID: "fixture"}, nil
	}
	return nil, nil
}

type prepaidSocketFixture struct{ jsonWrites, frameWrites, pings int }

func (c *prepaidSocketFixture) WriteJSON(context.Context, any) error        { c.jsonWrites++; return nil }
func (c *prepaidSocketFixture) ReadMessage(context.Context) ([]byte, error) { return nil, nil }
func (c *prepaidSocketFixture) Ping(context.Context) error                  { c.pings++; return nil }
func (c *prepaidSocketFixture) Close() error                                { return nil }
func (c *prepaidSocketFixture) ReadFrame(context.Context) (coderws.MessageType, []byte, error) {
	return coderws.MessageText, nil, nil
}
func (c *prepaidSocketFixture) WriteFrame(context.Context, coderws.MessageType, []byte) error {
	c.frameWrites++
	return nil
}

type prepaidDialFixture struct {
	calls int
	conn  *prepaidSocketFixture
}

func (d *prepaidDialFixture) Dial(context.Context, string, http.Header, string) (openAIWSClientConn, int, http.Header, error) {
	d.calls++
	return d.conn, 0, http.Header{}, nil
}

func TestPrepaidUnmeteredSocketsBlockDialAndReusedWrites(t *testing.T) {
	policy := &prepaidWSPolicyFixture{enrolled: true}
	raw := &prepaidDialFixture{conn: &prepaidSocketFixture{}}
	dialer := &prepaidWSDialer{inner: raw, ledger: policy}
	ctx := withPrepaidWSAccount(context.Background(), 11)
	headers := http.Header{"Authorization": []string{"Bearer fixture-key-not-a-provider-credential"}}
	conn, _, _, err := dialer.Dial(ctx, "wss://ark.ap-southeast.bytepluses.com/api/v3/responses", headers, "")
	var closeErr *OpenAIWSClientCloseError
	if conn != nil || !errors.As(err, &closeErr) || raw.calls != 0 || policy.account != 11 || policy.scheme != "https" {
		t.Fatal("enrolled key reached unmetered dial")
	}
	policy.enrolled = false
	conn, _, _, err = dialer.Dial(ctx, "wss://ark.ap-southeast.bytepluses.com/api/v3/responses", headers, "")
	if err != nil {
		t.Fatal(err)
	}
	if err = conn.WriteJSON(ctx, map[string]string{"type": "response.create"}); err != nil || raw.conn.jsonWrites != 1 {
		t.Fatal("ordinary PAYG socket behavior changed")
	}
	// Enrollment can happen after a connection enters the pool. It must not
	// wait for reconnect, a policy TTL, or another HTTP request to take effect.
	policy.enrolled = true
	headers.Set("Authorization", "Bearer changed-caller-header")
	if err = conn.WriteJSON(ctx, map[string]string{"type": "response.create"}); !errors.As(err, &closeErr) || raw.conn.jsonWrites != 1 {
		t.Fatal("pooled connection bypassed policy")
	}
	frameConn, ok := conn.(interface {
		WriteFrame(context.Context, coderws.MessageType, []byte) error
	})
	if !ok {
		t.Fatal("frame interface was lost")
	}
	if err = frameConn.WriteFrame(ctx, coderws.MessageText, []byte(`{"type":"response.create"}`)); !errors.As(err, &closeErr) || raw.conn.frameWrites != 0 {
		t.Fatal("passthrough frame bypassed policy")
	}
	if policy.hash != packguard.HashCredential("fixture-key-not-a-provider-credential") {
		t.Fatal("write checked mutable caller headers instead of connection authentication")
	}
	policy.enrolled = false
	policy.err = errors.New("fixture funding database unavailable")
	if err = conn.WriteJSON(ctx, map[string]string{"type": "response.create"}); !errors.As(err, &closeErr) || raw.conn.jsonWrites != 1 {
		t.Fatal("store failure allowed an unmetered write")
	}
	_ = conn.Close()
}

func TestPrepaidWebSocketErrorsAreTerminalForHTTPAndIngress(t *testing.T) {
	for _, unavailable := range []bool{false, true} {
		err := wrapOpenAIWSFallback("write_request", NewOpenAIWSClientCloseError(coderws.StatusPolicyViolation, "fixture", &prepaidWSRefusal{unavailable: unavailable}))
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
		httpErr := finalizePrepaidWSError(c, err)
		var fallback *openAIWSFallbackError
		if errors.As(httpErr, &fallback) {
			t.Fatal("funding rejection remained retryable")
		}
		expected := 400
		if unavailable {
			expected = 503
		}
		if recorder.Code != expected || !IsResponseCommitted(c) || !strings.Contains(recorder.Body.String(), "prepaid_pack_") {
			t.Fatalf("HTTP refusal can trigger fallback: %d %s", recorder.Code, recorder.Body.String())
		}
		wsRecorder := httptest.NewRecorder()
		ws, _ := gin.CreateTestContext(wsRecorder)
		ws.Request = httptest.NewRequest("GET", "/v1/responses", nil)
		ws.Request.Header.Set("Upgrade", "websocket")
		result := finalizePrepaidWSError(ws, fmt.Errorf("wrapped turn failure: %w", err))
		var closeErr *OpenAIWSClientCloseError
		if !errors.As(result, &closeErr) || ws.Writer.Written() {
			t.Fatal("ingress refusal was not terminal or wrote to a hijacked HTTP response")
		}
	}
}

func TestPrepaidSocketFactoriesUseTheRequiredSharedLedger(t *testing.T) {
	policy := &prepaidWSPolicyFixture{enrolled: true}
	u, err := NewBytePlusPackHTTP(&packGuardUpstreamFixture{}, policy)
	if err != nil {
		t.Fatal(err)
	}
	s := &OpenAIGatewayService{httpUpstream: u}
	if _, ok := s.getOpenAIWSConnPool().clientDialer.(*prepaidWSDialer); !ok {
		t.Fatal("pool dialer did not retain the required funding ledger")
	}
	if _, ok := s.getOpenAIWSPassthroughDialer().(*prepaidWSDialer); !ok {
		t.Fatal("passthrough/sideband dialer did not retain the required funding ledger")
	}
	s.getOpenAIWSConnPool().Close()
}
