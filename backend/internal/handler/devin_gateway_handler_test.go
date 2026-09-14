// devin_gateway_handler_test.go 覆盖错误归类与会话 hash 的纯函数路径。
package handler

import (
	"errors"
	"net/http"
	"testing"

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
