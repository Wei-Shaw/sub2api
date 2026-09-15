// devin_oauth_service_test.go 覆盖 PKCE 会话与授权码交换逻辑。
package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/devin"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
)

// stubDevinUpstream 按请求路径返回预置响应的 HTTPUpstream 测试桩。
type stubDevinUpstream struct {
	respond func(req *http.Request) *http.Response
}

func (s *stubDevinUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	return s.respond(req), nil
}

func (s *stubDevinUpstream) DoWithTLS(req *http.Request, _ string, _ int64, _ int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return s.respond(req), nil
}

func devinProtoResp(body []byte) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/proto"}},
		Body:       io.NopCloser(strings.NewReader(string(body))),
	}
}

func TestDevinOAuthGenerateAuthURL(t *testing.T) {
	svc := NewDevinOAuthService(nil, &stubDevinUpstream{})
	res, err := svc.GenerateAuthURL(context.Background(), nil)
	if err != nil {
		t.Fatalf("GenerateAuthURL: %v", err)
	}
	if !strings.Contains(res.AuthURL, "/auth/cli/continue") {
		t.Fatalf("auth_url missing cli/continue: %s", res.AuthURL)
	}
	if !strings.Contains(res.AuthURL, "code_challenge=") || !strings.Contains(res.AuthURL, "cli_pkce_marker=1") {
		t.Fatalf("auth_url missing PKCE params: %s", res.AuthURL)
	}
	if res.SessionID == "" || res.State == "" {
		t.Fatalf("session_id/state empty: %+v", res)
	}
}

func TestDevinOAuthExchangePastedAPIKey(t *testing.T) {
	// 粘贴 key 跳过交换，但 ExchangeCode 仍会调 GetUserStatus——桩返回空消息。
	svc := NewDevinOAuthService(nil, &stubDevinUpstream{
		respond: func(req *http.Request) *http.Response {
			if req.URL.Path != devin.PathGetUserStatus {
				t.Fatalf("pasted api key must not hit exchange RPC: %s", req.URL.Path)
			}
			if req.Header.Get("Authorization") == "" {
				t.Error("GetUserStatus must carry Authorization")
			}
			return devinProtoResp(nil)
		},
	})
	res, err := svc.GenerateAuthURL(context.Background(), nil)
	if err != nil {
		t.Fatalf("GenerateAuthURL: %v", err)
	}
	info, err := svc.ExchangeCode(context.Background(), &DevinExchangeCodeInput{
		SessionID: res.SessionID,
		State:     res.State,
		Code:      "devin-session-token$abcdef1234567890abcdef1234567890abcdef12",
	})
	if err != nil {
		t.Fatalf("ExchangeCode: %v", err)
	}
	if info.AccessToken != "devin-session-token$abcdef1234567890abcdef1234567890abcdef12" {
		t.Fatalf("token = %q", info.AccessToken)
	}
}

func TestDevinOAuthExchangePastedAPIKeyWithoutSession(t *testing.T) {
	// 无会话的 token 直贴：session_id/state 为空也应通过。
	svc := NewDevinOAuthService(nil, &stubDevinUpstream{
		respond: func(req *http.Request) *http.Response {
			if req.URL.Path != devin.PathGetUserStatus {
				t.Fatalf("pasted api key must not hit exchange RPC: %s", req.URL.Path)
			}
			return devinProtoResp(nil)
		},
	})
	info, err := svc.ExchangeCode(context.Background(), &DevinExchangeCodeInput{
		Code: "devin-session-token$abcdef1234567890abcdef1234567890abcdef12",
	})
	if err != nil {
		t.Fatalf("ExchangeCode: %v", err)
	}
	if info.AccessToken == "" {
		t.Fatal("empty access token")
	}

	// 无会话的非 token 授权码必须报 session 错误。
	if _, err := svc.ExchangeCode(context.Background(), &DevinExchangeCodeInput{
		Code: "pkce-code-abc",
	}); err == nil {
		t.Fatal("PKCE code without session should fail")
	}
}

func TestDevinOAuthExchangeCodeViaRPC(t *testing.T) {
	// 上游为 ExchangePKCEAuthorizationCode 返回 {1: api_key, 3: api_server_url}。
	var sawNoAuth bool
	upstream := &stubDevinUpstream{
		respond: func(req *http.Request) *http.Response {
			if req.URL.Path == devin.PathExchangePKCEAuthCode {
				if req.Header.Get("Authorization") == "" {
					sawNoAuth = true
				}
				var body []byte
				body = devin.AppendTestString(body, 1, "devin-session-token$xyz")
				body = devin.AppendTestString(body, 3, "https://server.example.com")
				return devinProtoResp(body)
			}
			if req.URL.Path == devin.PathGetUserStatus {
				return devinProtoResp(nil)
			}
			t.Fatalf("unexpected path: %s", req.URL.Path)
			return nil
		},
	}
	svc := NewDevinOAuthService(nil, upstream)
	res, err := svc.GenerateAuthURL(context.Background(), nil)
	if err != nil {
		t.Fatalf("GenerateAuthURL: %v", err)
	}
	info, err := svc.ExchangeCode(context.Background(), &DevinExchangeCodeInput{
		SessionID: res.SessionID,
		State:     res.State,
		Code:      "short-pasted-code",
	})
	if err != nil {
		t.Fatalf("ExchangeCode: %v", err)
	}
	if info.AccessToken != "devin-session-token$xyz" {
		t.Fatalf("token = %q", info.AccessToken)
	}
	if info.APIServerURL != "https://server.example.com" {
		t.Fatalf("api_server_url = %q", info.APIServerURL)
	}
	if !sawNoAuth {
		t.Fatal("exchange request must not carry Authorization")
	}
}

func TestDevinOAuthExchangeRejectsBadState(t *testing.T) {
	svc := NewDevinOAuthService(nil, &stubDevinUpstream{})
	res, err := svc.GenerateAuthURL(context.Background(), nil)
	if err != nil {
		t.Fatalf("GenerateAuthURL: %v", err)
	}
	_, err = svc.ExchangeCode(context.Background(), &DevinExchangeCodeInput{
		SessionID: res.SessionID,
		State:     "wrong",
		Code:      "code",
	})
	if err == nil || !strings.Contains(err.Error(), "state") {
		t.Fatalf("expected state error, got %v", err)
	}
	_, err = svc.ExchangeCode(context.Background(), &DevinExchangeCodeInput{
		SessionID: "nonexistent",
		State:     res.State,
		Code:      "code",
	})
	if err == nil {
		t.Fatal("expected session error")
	}
}
