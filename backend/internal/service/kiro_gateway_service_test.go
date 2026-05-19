package service

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Wei-Shaw/sub2api/internal/pkg/kiro"
)

// inMemTokenCache is a minimal GeminiTokenCache used only by Kiro
// gateway tests. The provider in our test scenarios doesn't need
// refresh-lock coordination — these tests start with a non-expired
// token, so the cache only mediates the cache-key fast path.
type inMemTokenCache struct {
	mu     sync.Mutex
	tokens map[string]string
	locks  map[string]bool
}

func newInMemTokenCache() *inMemTokenCache {
	return &inMemTokenCache{
		tokens: map[string]string{},
		locks:  map[string]bool{},
	}
}

func (c *inMemTokenCache) GetAccessToken(_ context.Context, k string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.tokens[k], nil
}
func (c *inMemTokenCache) SetAccessToken(_ context.Context, k, v string, _ time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.tokens[k] = v
	return nil
}
func (c *inMemTokenCache) DeleteAccessToken(_ context.Context, k string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.tokens, k)
	return nil
}
func (c *inMemTokenCache) AcquireRefreshLock(_ context.Context, k string, _ time.Duration) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.locks[k] {
		return false, nil
	}
	c.locks[k] = true
	return true, nil
}
func (c *inMemTokenCache) ReleaseRefreshLock(_ context.Context, k string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.locks, k)
	return nil
}

func newKiroAccount() *Account {
	return &Account{
		ID:       42,
		Platform: PlatformKiro,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token":  "tok-x",
			"refresh_token": "rt",
			"expires_at":    "9999999999",
			"auth_method":   "social",
		},
		Extra: map[string]any{},
	}
}

func newGatewayServiceWithFakeUpstream(t *testing.T, handler http.HandlerFunc) (*KiroGatewayService, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	kiro.OverrideEndpointURLForTest(t, 0, srv.URL)

	tp := NewKiroTokenProvider(nil, newInMemTokenCache(), nil)
	gs := NewKiroGatewayService(tp, nil)
	return gs, srv
}

func TestKiroGatewayService_StreamingForward(t *testing.T) {
	gs, _ := newGatewayServiceWithFakeUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok-x" {
			t.Fatalf("bearer = %q", r.Header.Get("Authorization"))
		}
		var buf bytes.Buffer
		_ = kiro.EncodeEventStream(&buf, []kiro.Event{
			{Type: "assistantResponseEvent", Payload: map[string]any{"content": "hello"}},
			{Type: "assistantResponseEvent", Payload: map[string]any{"content": "hello world"}},
			{Type: "meteringEvent", Payload: map[string]any{
				"usage": map[string]any{
					"inputTokens":  10.0,
					"outputTokens": 4.0,
				},
			}},
		})
		_, _ = w.Write(buf.Bytes())
	})

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	body, _ := json.Marshal(map[string]any{
		"model":      "claude-sonnet-4-6",
		"max_tokens": 1024,
		"stream":     true,
		"messages": []map[string]any{
			{"role": "user", "content": "hi"},
		},
	})

	result, err := gs.Forward(context.Background(), c, newKiroAccount(), body)
	if err != nil {
		t.Fatalf("forward: %v", err)
	}
	out := w.Body.String()
	if !strings.Contains(out, "event: message_start") {
		t.Fatalf("output missing message_start: %s", out)
	}
	if !strings.Contains(out, "event: message_stop") {
		t.Fatalf("output missing message_stop: %s", out)
	}
	if !strings.Contains(out, "\"text\":\"hello world\"") && !strings.Contains(out, "\"text\":\"hello\"") {
		t.Fatalf("output missing text deltas: %s", out)
	}
	if result.Model != "claude-sonnet-4-6" {
		t.Fatalf("requested model = %q", result.Model)
	}
	if result.UpstreamModel != "claude-sonnet-4.6" {
		t.Fatalf("upstream model = %q", result.UpstreamModel)
	}
	if result.Usage.InputTokens != 10 || result.Usage.OutputTokens != 4 {
		t.Fatalf("usage = %+v", result.Usage)
	}
}

func TestKiroGatewayService_NonStreamingForward(t *testing.T) {
	gs, _ := newGatewayServiceWithFakeUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		var buf bytes.Buffer
		_ = kiro.EncodeEventStream(&buf, []kiro.Event{
			{Type: "assistantResponseEvent", Payload: map[string]any{"content": "single"}},
			{Type: "meteringEvent", Payload: map[string]any{
				"usage": map[string]any{
					"inputTokens":  5.0,
					"outputTokens": 1.0,
				},
			}},
		})
		_, _ = w.Write(buf.Bytes())
	})

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	body, _ := json.Marshal(map[string]any{
		"model":      "claude-sonnet-4-6",
		"max_tokens": 1024,
		"stream":     false,
		"messages": []map[string]any{
			{"role": "user", "content": "hi"},
		},
	})

	result, err := gs.Forward(context.Background(), c, newKiroAccount(), body)
	if err != nil {
		t.Fatalf("forward: %v", err)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	var parsed map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &parsed)
	if parsed["type"] != "message" {
		t.Fatalf("response type = %v body=%s", parsed["type"], w.Body.String())
	}
	if result.Usage.InputTokens != 5 || result.Usage.OutputTokens != 1 {
		t.Fatalf("usage = %+v", result.Usage)
	}
}

func TestKiroGatewayService_BackfillsMachineID(t *testing.T) {
	gs, _ := newGatewayServiceWithFakeUpstream(t, func(w http.ResponseWriter, _ *http.Request) {
		var buf bytes.Buffer
		_ = kiro.EncodeEventStream(&buf, []kiro.Event{})
		_, _ = w.Write(buf.Bytes())
	})

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	body, _ := json.Marshal(map[string]any{
		"model":    "claude-sonnet-4-6",
		"stream":   false,
		"messages": []map[string]any{{"role": "user", "content": "hi"}},
	})

	acc := newKiroAccount()
	if acc.Extra["machine_id"] != nil {
		t.Fatal("machine_id should start empty")
	}
	_, err := gs.Forward(context.Background(), c, acc, body)
	if err != nil {
		t.Fatal(err)
	}
	if acc.Extra["machine_id"] == nil {
		t.Fatal("machine_id should be backfilled")
	}
}

func TestKiroGatewayService_IsModelSupported(t *testing.T) {
	gs := &KiroGatewayService{}
	if !gs.IsModelSupported("claude-sonnet-4-6") {
		t.Fatal("claude-sonnet-4-6 should be supported")
	}
	if !gs.IsModelSupported("claude-sonnet-4-6-thinking") {
		t.Fatal("thinking suffix should still resolve")
	}
	if gs.IsModelSupported("gpt-4") {
		t.Fatal("gpt-4 should not be supported")
	}
}
