// devin_gateway_handler_test.go 覆盖错误归类与会话 hash 的纯函数路径。
package handler

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/devin"
	"github.com/Wei-Shaw/sub2api/internal/pkg/devin/llm"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func TestDevinFailoverErrorClassification(t *testing.T) {
	cases := []struct {
		name           string
		err            error
		wantStop       bool
		wantAuth       bool
		wantClientCode int
	}{
		{
			name:           "unauthenticated is credential failure",
			err:            &devin.ConnectError{Code: "unauthenticated", Message: "invalid api key", HTTPStatus: 401},
			wantAuth:       true,
			wantClientCode: http.StatusUnauthorized,
		},
		{
			name:           "resource_exhausted fails over",
			err:            &devin.ConnectError{Code: "resource_exhausted", Message: "reset in 30 seconds"},
			wantClientCode: http.StatusTooManyRequests,
		},
		{
			name:           "invalid_argument does not fail over",
			err:            &devin.ConnectError{Code: "invalid_argument", Message: "tool_choice names tool \"x\" which is not in the tools list"},
			wantStop:       true,
			wantClientCode: http.StatusBadRequest,
		},
		{
			name:           "invalid request does not fail over",
			err:            devin.NewInvalidRequest(errors.New("devin: request model is required")),
			wantStop:       true,
			wantClientCode: http.StatusBadRequest,
		},
		{
			name:           "bare transport error fails over",
			err:            errors.New("read tcp: connection reset by peer"),
			wantClientCode: http.StatusBadGateway,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fe := service.DevinFailoverError(tc.err)
			if fe == nil {
				t.Fatal("nil failover error")
			}
			if tc.wantStop && fe.ShouldRetryNextAccount() {
				t.Error("expected NextAccountStop")
			}
			if !tc.wantStop && !fe.ShouldRetryNextAccount() {
				t.Error("expected failover-allowed")
			}
			if tc.wantAuth && !fe.IsCredentialFailure() {
				t.Error("expected credential failure stage")
			}
			if fe.ClientStatusCode != tc.wantClientCode {
				t.Errorf("client status = %d, want %d", fe.ClientStatusCode, tc.wantClientCode)
			}
		})
	}
}

func TestDevinSessionHash(t *testing.T) {
	withKey := &llm.RequestMessages{SessionKey: "user-42", Model: "swe-2"}
	if got := devinSessionHash(withKey); got != "devin:user-42" {
		t.Fatalf("session key hash = %q", got)
	}
	if got := devinSessionHash(withKey); got != devinSessionHash(&llm.RequestMessages{SessionKey: "user-42", Model: "other"}) {
		t.Fatal("session key must dominate over model")
	}
	noKey := &llm.RequestMessages{
		Model:        "swe-2",
		SystemPrompt: "sys",
		Messages:     []llm.Message{llm.UserMessage{Content: []llm.Content{llm.TextContent{Text: "hi"}}}},
	}
	a := devinSessionHash(noKey)
	if a == "" || a == "devin:user-42" {
		t.Fatalf("auto hash = %q", a)
	}
	b := devinSessionHash(&llm.RequestMessages{
		Model:        "swe-2",
		SystemPrompt: "sys",
		Messages:     []llm.Message{llm.UserMessage{Content: []llm.Content{llm.TextContent{Text: "hi"}}}},
	})
	if a != b {
		t.Fatal("same request must produce same auto hash")
	}
	c := devinSessionHash(&llm.RequestMessages{
		Model:    "swe-2",
		Messages: []llm.Message{llm.UserMessage{Content: []llm.Content{llm.TextContent{Text: "different"}}}},
	})
	if a == c {
		t.Fatal("different first message must change hash")
	}
}

// --- /v1/models 列表组装（model_mapping 白名单 + 分组 allowlist） ---

func devinCatalogFixture() []devin.GroupedModel {
	return []devin.GroupedModel{
		{ID: "swe-2", Name: "SWE-2", ContextWindow: 262144, MaxTokens: 32768, OwnedBy: "devin"},
		{ID: "claude-fable-5-1", Name: "Claude Fable 5.1", ContextWindow: 400000, MaxTokens: 64000, OwnedBy: "anthropic"},
		{ID: "gpt-6-astra", Name: "GPT-6 Astra", ContextWindow: 1000000, MaxTokens: 128000, OwnedBy: "openai"},
	}
}

// TestDevinModelEntriesNoMapping 未配 model_mapping 时回落上游目录全量。
func TestDevinModelEntriesNoMapping(t *testing.T) {
	entries := devinModelEntries(devinCatalogFixture(), nil, nil)
	if len(entries) != 3 {
		t.Fatalf("expected full catalog, got %d entries", len(entries))
	}
	if entries[0].id != "swe-2" || entries[0].name != "SWE-2" {
		t.Fatalf("unexpected first entry: %+v", entries[0])
	}
	if entries[0].contextWindow != 262144 || entries[0].maxTokens != 32768 {
		t.Fatalf("context/max tokens not propagated: %+v", entries[0])
	}
	if entries[1].ownedBy != "anthropic" {
		t.Fatalf("ownedBy = %q, want anthropic", entries[1].ownedBy)
	}
}

// TestDevinModelEntriesMappingWhitelist 配了 model_mapping 时只暴露映射键，
// 目录外的映射键（改名映射）也要出现。
func TestDevinModelEntriesMappingWhitelist(t *testing.T) {
	entries := devinModelEntries(devinCatalogFixture(), []string{"swe-2", "my-alias"}, nil)
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	ids := []string{entries[0].id, entries[1].id}
	if ids[0] != "swe-2" || ids[1] != "my-alias" {
		t.Fatalf("unexpected ids: %v", ids)
	}
	if entries[1].name != "my-alias" {
		t.Fatalf("catalog miss should fall back to id as name, got %q", entries[1].name)
	}
	if entries[1].contextWindow != 0 || entries[1].ownedBy != "devin" {
		t.Fatalf("alias entry should carry no catalog metadata, got %+v", entries[1])
	}
}

// TestDevinModelEntriesGroupAllowlist 分组白名单在账号白名单之上叠加过滤。
func TestDevinModelEntriesGroupAllowlist(t *testing.T) {
	group := &service.Group{
		ModelAllowlist: service.GroupModelAllowlist{Enabled: true, Models: []string{"swe-2"}},
	}
	entries := devinModelEntries(devinCatalogFixture(), []string{"swe-2", "gpt-6-astra"}, group)
	if len(entries) != 1 || entries[0].id != "swe-2" {
		t.Fatalf("group allowlist should filter to swe-2 only, got %+v", entries)
	}
}

// --- ForwardResult 组装（推理强度 + 延迟 + stream 标志） ---

func devinFinalMessage(responseModel string) *llm.AssistantMessage {
	return &llm.AssistantMessage{
		UpstreamRequestID: "req-1",
		ResponseModel:     responseModel,
		Usage:             llm.Usage{Input: 4, Output: 4},
	}
}

// TestBuildDevinForwardResultFields 校验用量/延迟/stream 标志落字段。
func TestBuildDevinForwardResultFields(t *testing.T) {
	ft := 682
	result := buildDevinForwardResult("swe-2", devinFinalMessage("swe-2-high"), false,
		time.Now().Add(-3*time.Second), &ft, "")
	if result.Stream {
		t.Error("non-stream request must not be recorded as stream")
	}
	if result.FirstTokenMs == nil || *result.FirstTokenMs != 682 {
		t.Fatalf("FirstTokenMs = %v, want 682", result.FirstTokenMs)
	}
	if result.Duration < 3*time.Second {
		t.Fatalf("Duration = %v, want >= 3s", result.Duration)
	}
	if result.UpstreamModel != "swe-2-high" {
		t.Fatalf("UpstreamModel = %q, want swe-2-high", result.UpstreamModel)
	}
	if result.RequestID != "req-1" {
		t.Fatalf("RequestID = %q", result.RequestID)
	}
}

// TestBuildDevinForwardResultEffort 推理强度三层取值：
// 请求值（参数或 :level 后缀）记 RequestedReasoningEffort；
// 上游实际 uid 回推的档位记 ReasoningEffort（真生效档）。
func TestBuildDevinForwardResultEffort(t *testing.T) {
	cases := []struct {
		name            string
		reqModel        string
		responseModel   string
		requestedEffort string
		wantRequested   string
		wantEffective   string
	}{
		{"explicit param", "swe-2", "swe-2-max", "max", "max", "max"},
		{"level suffix", "swe-2:max", "swe-2-max", "", "max", "max"},
		{"default high from uid", "swe-2", "swe-2-high", "", "", "high"},
		{"walked down", "swe-2", "swe-2-medium", "xhigh", "xhigh", "medium"}, // 请求 xhigh 实际 medium
		{"unparsable uid falls back", "swe-2", "swe-2", "low", "low", "low"},
		{"nothing", "swe-2", "swe-2", "", "", ""},
	}
	for _, c := range cases {
		result := buildDevinForwardResult(c.reqModel, devinFinalMessage(c.responseModel), true,
			time.Now(), nil, c.requestedEffort)
		gotReq := ""
		if result.RequestedReasoningEffort != nil {
			gotReq = *result.RequestedReasoningEffort
		}
		gotEff := ""
		if result.ReasoningEffort != nil {
			gotEff = *result.ReasoningEffort
		}
		if gotReq != c.wantRequested || gotEff != c.wantEffective {
			t.Errorf("%s: requested=%q want %q, effective=%q want %q",
				c.name, gotReq, c.wantRequested, gotEff, c.wantEffective)
		}
	}
}

func TestDevinMappedModelInCatalog(t *testing.T) {
	groups := devinCatalogFixture()
	cases := []struct {
		name, mapped string
		want         bool
	}{
		{"known group id", "swe-2", true},
		{"with level suffix", "swe-2:max", true},
		{"bogus target", "swe-9-typo", false},
		{"flattened uid not a group", "swe-2-high", false},
		{"empty", "", false},
	}
	for _, tc := range cases {
		if got := devinMappedModelInCatalog(groups, tc.mapped); got != tc.want {
			t.Fatalf("%s: devinMappedModelInCatalog(%q) = %v, want %v", tc.name, tc.mapped, got, tc.want)
		}
	}
}

// stallThenFinishStream 模拟"首个事件前静默"的上游流：delay 期间无事件，
// 之后吐一个 text delta + done。用于验证中流 keepalive 在静默期补心跳。
type stallThenFinishStream struct {
	delay  time.Duration
	events []llm.ResponseEvent
	idx    int
}

func (s *stallThenFinishStream) Recv(ctx context.Context) (llm.ResponseEvent, error) {
	if s.idx == 0 {
		select {
		case <-time.After(s.delay):
		case <-ctx.Done():
			return llm.ResponseEvent{}, ctx.Err()
		}
	}
	if s.idx >= len(s.events) {
		return llm.ResponseEvent{}, io.EOF
	}
	ev := s.events[s.idx]
	s.idx++
	return ev, nil
}

// TestPumpDevinStreamEmitsKeepaliveDuringSilence 复刻线上形态：上游首帧
// 静默 >1s 时，pump 必须向客户端写 SSE 注释心跳，而不是让连接饿着。
func TestPumpDevinStreamEmitsKeepaliveDuringSilence(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	h := &DevinGatewayHandler{cfg: &config.Config{}}
	h.cfg.Gateway.StreamKeepaliveInterval = 1 // 秒，配置最小粒度

	stream := &stallThenFinishStream{
		delay: 1500 * time.Millisecond,
		events: []llm.ResponseEvent{
			{Type: llm.ResponseEventTextDelta, Delta: "hi", Partial: &llm.AssistantMessage{}},
			{Type: llm.ResponseEventDone, Reason: llm.StopReasonStop, Message: &llm.AssistantMessage{
				Content: []llm.Content{llm.TextContent{Text: "hi"}},
			}},
		},
	}
	adapted := devinAdapted{context: llm.RequestMessages{Model: "swe-2"}, stream: true}
	streamStarted := false
	_, _, err := h.pumpDevinStream(c, devinProtocolMessages, stream, adapted, true, &streamStarted, time.Now())
	if err != nil {
		t.Fatalf("pump: %v", err)
	}
	body := rec.Body.String()
	if !strings.HasPrefix(body, ":\n\n") {
		t.Fatalf("expected keepalive comment emitted during silence, body=%q", body)
	}
	if !strings.Contains(body, "message_stop") {
		t.Fatalf("expected stream events after keepalive, body=%q", body)
	}
}
