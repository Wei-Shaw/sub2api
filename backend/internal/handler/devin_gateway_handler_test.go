// devin_gateway_handler_test.go 覆盖错误归类与会话 hash 的纯函数路径。
package handler

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/devin"
	"github.com/Wei-Shaw/sub2api/internal/pkg/devin/llm"
	"github.com/Wei-Shaw/sub2api/internal/service"
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
		{ID: "swe-2", Name: "SWE-2"},
		{ID: "claude-fable-5-1", Name: "Claude Fable 5.1"},
		{ID: "gpt-6-astra", Name: "GPT-6 Astra"},
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
