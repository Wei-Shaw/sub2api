//go:build unit

package handler

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestExtractCaptchaPayload 覆盖 §4.3 / §4.6 的字段归一化契约：
//   - 优先级：captcha_payload > captcha_token > turnstile_token
//   - 值需 trim
//   - 三层全空时返回空 map（不返回 nil，便于下游统一调用 len(payload)）
//   - captcha_payload 命中时不混入老字段，避免上游误把 token 与 ticket/randstr 共存
//   - 老字段（captcha_token / turnstile_token）只在 captcha_payload 缺省时回退，且统一归一为 {"token": ...}
func TestExtractCaptchaPayload(t *testing.T) {
	cases := []struct {
		name           string
		captchaPayload map[string]string
		captchaToken   string
		turnstileToken string
		expected       map[string]string
	}{
		{
			name:           "all empty returns empty map",
			captchaPayload: nil,
			expected:       map[string]string{},
		},
		{
			name:           "all empty (whitespace only) returns empty map",
			captchaToken:   "   ",
			turnstileToken: "\t",
			expected:       map[string]string{},
		},
		{
			name:           "only legacy captcha_token wraps to token",
			captchaToken:   "tk-1",
			turnstileToken: "",
			expected:       map[string]string{"token": "tk-1"},
		},
		{
			name:           "only legacy turnstile_token wraps to token",
			captchaToken:   "",
			turnstileToken: "ts-1",
			expected:       map[string]string{"token": "ts-1"},
		},
		{
			name:           "captcha_token wins over turnstile_token when both present",
			captchaToken:   "tk-2",
			turnstileToken: "ts-2",
			expected:       map[string]string{"token": "tk-2"},
		},
		{
			name:           "captcha_token trimmed",
			captchaToken:   "  tk-3  ",
			turnstileToken: "",
			expected:       map[string]string{"token": "tk-3"},
		},
		{
			name:           "captcha_payload structured (tencent ticket+randstr) is preserved",
			captchaPayload: map[string]string{"ticket": "tx-ticket", "randstr": "@1A"},
			expected:       map[string]string{"ticket": "tx-ticket", "randstr": "@1A"},
		},
		{
			name:           "captcha_payload preempts legacy fields",
			captchaPayload: map[string]string{"ticket": "tx-ticket", "randstr": "@1A"},
			captchaToken:   "should-be-ignored",
			turnstileToken: "should-be-ignored-too",
			expected:       map[string]string{"ticket": "tx-ticket", "randstr": "@1A"},
		},
		{
			name:           "captcha_payload values trimmed but empty entries kept (downstream verifier decides)",
			captchaPayload: map[string]string{"ticket": "  tx-ticket  ", "randstr": "  "},
			expected:       map[string]string{"ticket": "tx-ticket", "randstr": ""},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := extractCaptchaPayload(tc.captchaPayload, tc.captchaToken, tc.turnstileToken)
			require.Equal(t, tc.expected, got)
		})
	}
}
