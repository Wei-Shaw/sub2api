//go:build unit

package service

import (
	"encoding/json"
	"testing"

	"github.com/tidwall/gjson"
)

func TestIsOpenCodeUpstream(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		baseURL string
		want    bool
	}{
		{"zen/go 付费端点", "https://opencode.ai/zen/go/v1", true},
		{"zen/v1 免费端点", "https://opencode.ai/zen/v1", true},
		{"deepseek 官方", "https://api.deepseek.com", false},
		{"openai 官方", "https://api.openai.com/v1", false},
		{"空 base", "", false},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			a := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
			if tc.baseURL != "" {
				a.Credentials = map[string]any{"base_url": tc.baseURL}
			}
			if got := a.IsOpenCodeUpstream(); got != tc.want {
				t.Fatalf("IsOpenCodeUpstream()=%v, want %v (base=%q)", got, tc.want, tc.baseURL)
			}
		})
	}
}

func TestOpenCodePlan(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		baseURL string
		want    string
	}{
		{"go 计划", "https://opencode.ai/zen/go/v1", "go"},
		{"免费端点", "https://opencode.ai/zen/v1", "free"},
		{"非 opencode", "https://api.deepseek.com", ""},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			a := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
			a.Credentials = map[string]any{"base_url": tc.baseURL}
			if got := a.OpenCodePlan(); got != tc.want {
				t.Fatalf("OpenCodePlan()=%q, want %q", got, tc.want)
			}
		})
	}
}

func TestOpenCodePrepareUpstreamBody(t *testing.T) {
	t.Parallel()

	body := []byte(`{"model":"deepseek-v4-flash","messages":[{"role":"user","content":"hi"},{"role":"assistant","content":"a1","id":"keep"}]}`)

	t.Run("free 档次补齐 id", func(t *testing.T) {
		t.Parallel()
		a := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
		a.Credentials = map[string]any{"base_url": "https://opencode.ai/zen/v1"}
		out, err := OpenCodePrepareUpstreamBody(a, body)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if !json.Valid(out) {
			t.Fatalf("invalid json: %s", out)
		}
		// 已有 id 保留
		if got := gjson.GetBytes(out, "messages.1.id").String(); got != "keep" {
			t.Fatalf("existing id not preserved: %q", got)
		}
		// 缺失 id 补齐
		if got := gjson.GetBytes(out, "messages.0.id").String(); got == "" {
			t.Fatalf("missing id not filled")
		}
	})

	t.Run("go 档次原样", func(t *testing.T) {
		t.Parallel()
		a := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
		a.Credentials = map[string]any{"base_url": "https://opencode.ai/zen/go/v1"}
		out, err := OpenCodePrepareUpstreamBody(a, body)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if string(out) != string(body) {
			t.Fatalf("go tier body changed: %s", out)
		}
	})

	t.Run("非 opencode 原样", func(t *testing.T) {
		t.Parallel()
		a := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
		a.Credentials = map[string]any{"base_url": "https://api.deepseek.com"}
		out, err := OpenCodePrepareUpstreamBody(a, body)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if string(out) != string(body) {
			t.Fatalf("non-opencode body changed: %s", out)
		}
	})

	t.Run("无 messages 原样", func(t *testing.T) {
		t.Parallel()
		a := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
		a.Credentials = map[string]any{"base_url": "https://opencode.ai/zen/v1"}
		plain := []byte(`{"model":"x"}`)
		out, err := OpenCodePrepareUpstreamBody(a, plain)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if string(out) != string(plain) {
			t.Fatalf("plain body changed: %s", out)
		}
	})
}

func TestParseOpenCodeCreditsError(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		body string
		want string
	}{
		{
			"no payment method",
			`{"type":"error","error":{"type":"CreditsError","message":"No payment method. Add a payment method here: https://opencode.ai/workspace/x/billing"}}`,
			openCodeCreditsNoPaymentMethod,
		},
		{
			"insufficient balance",
			`{"type":"error","error":{"type":"CreditsError","message":"Insufficient balance. Manage your billing here: https://opencode.ai/workspace/x/billing"}}`,
			openCodeCreditsInsufficientBalance,
		},
		{"非 CreditsError", `{"error":{"type":"invalid_request_error","message":"bad"}}`, ""},
		{"空 body", `{}`, ""},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			typ, msg := parseOpenCodeCreditsError([]byte(tc.body))
			if typ != openCodeCreditsErrorType && typ != "" {
				t.Fatalf("unexpected type %q", typ)
			}
			if tc.want != "" && !containsOpenCodeCreditMessage(msg, tc.want) {
				t.Fatalf("message %q does not contain %q", msg, tc.want)
			}
			if tc.want == "" && (typ != "" || msg != "") {
				t.Fatalf("expected no match, got type=%q msg=%q", typ, msg)
			}
		})
	}
}

func containsOpenCodeCreditMessage(msg, sub string) bool {
	return len(msg) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(msg); i++ {
			if msg[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}
