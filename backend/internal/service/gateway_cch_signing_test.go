package service

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

const claudeCCH21220BaseBody = `{"model":"model-a","messages":[{"role":"user","content":[{"type":"text","text":"x"}]}],"system":[{"type":"text","text":"x-anthropic-billing-header: cc_version=2.1.220.test; cc_entrypoint=sdk-cli; cch=00000;"},{"type":"text","text":"system-x"}],"tools":[],"metadata":{"user_id":"meta-x"},"max_tokens":1,"thinking":{"type":"adaptive","display":"omitted"},"context_management":{"edits":[{"type":"clear_thinking_20251015","keep":"all"}]},"output_config":{"effort":"high"},"stream":true}`

func TestSignAnthropicRequestCCH_KnownVectors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
		want string
	}{
		{name: "base", body: claudeCCH21220BaseBody, want: "7ee87"},
		{name: "model value ignored", body: strings.Replace(claudeCCH21220BaseBody, `"model":"model-a"`, `"model":"model-b"`, 1), want: "7ee87"},
		{name: "max tokens ignored", body: strings.Replace(claudeCCH21220BaseBody, `"max_tokens":1`, `"max_tokens":2`, 1), want: "7ee87"},
		{name: "message changes hash", body: strings.Replace(claudeCCH21220BaseBody, `"text":"x"`, `"text":"y"`, 1), want: "b9cc8"},
		{name: "system changes hash", body: strings.Replace(claudeCCH21220BaseBody, `"system-x"`, `"system-y"`, 1), want: "a30d3"},
		{name: "metadata changes hash", body: strings.Replace(claudeCCH21220BaseBody, `"user_id":"meta-x"`, `"user_id":"meta-y"`, 1), want: "7a89d"},
		{name: "thinking changes hash", body: strings.Replace(claudeCCH21220BaseBody, `"thinking":{"type":"adaptive","display":"omitted"}`, `"thinking":{"type":"disabled"}`, 1), want: "7205c"},
		{name: "context changes hash", body: strings.Replace(claudeCCH21220BaseBody, `"context_management":{"edits":[{"type":"clear_thinking_20251015","keep":"all"}]}`, `"context_management":{"edits":[]}`, 1), want: "05073"},
		{name: "effort changes hash", body: strings.Replace(claudeCCH21220BaseBody, `"effort":"high"`, `"effort":"low"`, 1), want: "12366"},
		{name: "stream changes hash", body: strings.Replace(claudeCCH21220BaseBody, `"stream":true`, `"stream":false`, 1), want: "60400"},
		{name: "tool changes hash", body: strings.Replace(claudeCCH21220BaseBody, `"tools":[]`, `"tools":[{"name":"t","description":"d","input_schema":{"type":"object"}}]`, 1), want: "3d78d"},
		{name: "extra field changes hash", body: strings.Replace(claudeCCH21220BaseBody, `"stream":true}`, `"stream":true,"extra_top":"extra"}`, 1), want: "2d622"},
		{
			name: "field order remains significant",
			body: `{"stream":true,"output_config":{"effort":"high"},"context_management":{"edits":[{"type":"clear_thinking_20251015","keep":"all"}]},"thinking":{"type":"adaptive","display":"omitted"},"max_tokens":1,"metadata":{"user_id":"meta-x"},"tools":[],"system":[{"type":"text","text":"x-anthropic-billing-header: cc_version=2.1.220.test; cc_entrypoint=sdk-cli; cch=00000;"},{"type":"text","text":"system-x"}],"messages":[{"role":"user","content":[{"type":"text","text":"x"}]}],"model":"model-a"}`,
			want: "e5b6c",
		},
		{name: "nested model value ignored", body: strings.Replace(claudeCCH21220BaseBody, `"metadata":{"user_id":"meta-x"}`, `"metadata":{"user_id":"meta-x","model":"a"}`, 1), want: "0601b"},
		{name: "nested max tokens member omitted", body: strings.Replace(claudeCCH21220BaseBody, `"metadata":{"user_id":"meta-x"}`, `"metadata":{"user_id":"meta-x","max_tokens":2}`, 1), want: "7ee87"},
		{name: "top level fallbacks member omitted", body: strings.Replace(claudeCCH21220BaseBody, `"stream":true}`, `"stream":true,"fallbacks":[{"model":"fallback-a"}]}`, 1), want: "7ee87"},
		{name: "nested fallbacks member omitted", body: strings.Replace(claudeCCH21220BaseBody, `"metadata":{"user_id":"meta-x"}`, `"metadata":{"user_id":"meta-x","fallbacks":[{"model":"nested-a"}]}`, 1), want: "7ee87"},
		{name: "top level fallback credit token omitted", body: strings.Replace(claudeCCH21220BaseBody, `"stream":true}`, `"stream":true,"fallback_credit_token":"a"}`, 1), want: "7ee87"},
		{name: "nested fallback credit token omitted", body: strings.Replace(claudeCCH21220BaseBody, `"metadata":{"user_id":"meta-x"}`, `"metadata":{"user_id":"meta-x","fallback_credit_token":"a"}`, 1), want: "7ee87"},
		{name: "trailing dispatch run keeps native comma", body: strings.Replace(claudeCCH21220BaseBody, `"metadata":{"user_id":"meta-x"}`, `"metadata":{"user_id":"meta-x","max_tokens":999,"fallbacks":[{"model":"fallback-model"}]}`, 1), want: "4589b"},
		{name: "model before trailing dispatch run", body: strings.Replace(claudeCCH21220BaseBody, `"metadata":{"user_id":"meta-x"}`, `"metadata":{"user_id":"meta-x","model":"nested-model","max_tokens":999,"fallbacks":[{"model":"fallback-model"}],"fallback_credit_token":"not-a-real-token"}`, 1), want: "2d312"},
		{name: "model splits dispatch runs", body: strings.Replace(claudeCCH21220BaseBody, `"metadata":{"user_id":"meta-x"}`, `"metadata":{"user_id":"meta-x","max_tokens":999,"model":"nested-model","fallbacks":[{"model":"fallback-model"}]}`, 1), want: "0601b"},
		{name: "ordinary nested member remains", body: strings.Replace(claudeCCH21220BaseBody, `"metadata":{"user_id":"meta-x"}`, `"metadata":{"user_id":"meta-x","plain":"a"}`, 1), want: "8d74c"},
		{name: "billing block only", body: `{"system":[{"type":"text","text":"x-anthropic-billing-header: cc_version=2.1.220.test; cc_entrypoint=sdk-cli; cch=00000;"}]}`, want: "f2edb"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			signed, err := signAnthropicRequestCCH([]byte(tt.body))
			require.NoError(t, err)
			require.Equal(t, tt.want, claudeCCHFromBody(t, signed))
		})
	}
}

func TestFinalizeAnthropicRequestCCH_GenuineClaudeCodeCaptures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		version string
		want    string
	}{
		{version: "2.1.293", want: "477e3"},
		{version: "2.1.326", want: "f22a5"},
	}
	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join("testdata", "claude_code_"+tt.version+"_cch.json.gz.b64")
			captured := readClaudeCCHGolden(t, path)
			require.Equal(t, tt.want, claudeCCHFromBody(t, captured))

			unsigned := bytes.Replace(captured, []byte("cch="+tt.want+";"), []byte("cch=00000;"), 1)
			require.NotEqual(t, captured, unsigned)
			signed, err := signAnthropicRequestCCH(unsigned)
			require.NoError(t, err)
			require.Equal(t, captured, signed, "signature must preserve every byte outside cch")

			withoutCCH := bytes.Replace(captured, []byte(" cch="+tt.want+";"), nil, 1)
			require.NotEqual(t, captured, withoutCCH)
			finalized, err := finalizeAnthropicRequestCCH(withoutCCH)
			require.NoError(t, err)
			require.Equal(t, captured, finalized, "placeholder insertion must reproduce the genuine capture")
		})
	}
}

func TestSignAnthropicRequestCCH_PreservesLiteralCCHInUserText(t *testing.T) {
	t.Parallel()

	literal := "keep literal cch=00000; in the message"
	body := []byte(strings.Replace(claudeCCH21220BaseBody, `"text":"x"`, `"text":"`+literal+`"`, 1))
	signed, err := signAnthropicRequestCCH(body)
	require.NoError(t, err)
	require.Equal(t, literal, gjson.GetBytes(signed, "messages.0.content.0.text").String())

	cchOffset, ok := claudeBillingCCHDigitsOffset(signed)
	require.True(t, ok)
	unsigned := bytes.Clone(signed)
	copy(unsigned[cchOffset:cchOffset+claudeCCHLength], claudeCCHZero)
	require.Equal(t, body, unsigned)
}

func TestFinalizeAnthropicRequestCCH_NoBillingBlockIsNoop(t *testing.T) {
	t.Parallel()
	body := []byte(`{"model":"claude","messages":[{"role":"user","content":"literal cch=00000;"}]}`)
	got, err := finalizeAnthropicRequestCCH(body)
	require.NoError(t, err)
	require.Equal(t, body, got)
}

func TestNormalizeClaudeCCHInput_PreservesRawJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
		want string
	}{
		{name: "model string becomes empty", body: `{"model":"claude","keep":1}`, want: `{"model":"","keep":1}`},
		{name: "excluded first member", body: `{"max_tokens":1,"keep":2}`, want: `{"keep":2}`},
		{name: "excluded middle member", body: `{"keep":1,"fallbacks":[{"model":"x"}],"tail":2}`, want: `{"keep":1,"tail":2}`},
		{name: "excluded last member", body: `{"keep":1,"fallback_credit_token":"secret"}`, want: `{"keep":1}`},
		{name: "all members excluded", body: `{"max_tokens":1,"fallbacks":[],"fallback_credit_token":"secret"}`, want: `{}`},
		{name: "adjacent excluded members", body: `{"keep":1,"max_tokens":1,"fallbacks":[],"tail":2}`, want: `{"keep":1,"tail":2}`},
		{name: "native trailing dispatch run", body: `{"keep":1,"max_tokens":1,"fallbacks":[]}`, want: `{"keep":1,}`},
		{name: "nested fields", body: `{"outer":{"model":"x","max_tokens":1,"keep":"y"}}`, want: `{"outer":{"model":"","keep":"y"}}`},
		{name: "escaped key text stays inside string", body: `{"text":"literal \"model\":\"x\" and \"max_tokens\":1"}`, want: `{"text":"literal \"model\":\"x\" and \"max_tokens\":1"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := normalizeClaudeCCHInput([]byte(tt.body))
			require.NoError(t, err)
			require.Equal(t, tt.want, string(got))
		})
	}
}

func TestShouldSignAnthropicCCH(t *testing.T) {
	t.Parallel()
	oauth := &Account{Platform: PlatformAnthropic, Type: AccountTypeOAuth}
	setupToken := &Account{Platform: PlatformAnthropic, Type: AccountTypeSetupToken}
	apiKey := &Account{Platform: PlatformAnthropic, Type: AccountTypeAPIKey}
	vertex := &Account{Platform: PlatformAnthropic, Type: AccountTypeServiceAccount}

	require.True(t, shouldSignAnthropicCCH(oauth, claudeAPIURL))
	require.True(t, shouldSignAnthropicCCH(oauth, claudeAPICountTokensURL))
	require.True(t, shouldSignAnthropicCCH(setupToken, claudeAPIURL))
	require.False(t, shouldSignAnthropicCCH(apiKey, claudeAPIURL))
	require.False(t, shouldSignAnthropicCCH(vertex, claudeAPIURL))
	require.False(t, shouldSignAnthropicCCH(oauth, "https://relay.example/v1/messages?beta=true"))
	require.False(t, shouldSignAnthropicCCH(nil, claudeAPIURL))
}

func TestBuildAnthropicRequest_CCHSetting(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, endpoint := range []string{"messages", "count_tokens"} {
		for _, enabled := range []bool{false, true} {
			t.Run(endpoint+"/enabled="+strconv.FormatBool(enabled), func(t *testing.T) {
				resetGatewayForwardingSettingsCacheForTest(t)
				cfg := &config.Config{}
				svc := &GatewayService{
					cfg: cfg,
					settingService: NewSettingService(&gatewayTTLSettingRepo{data: map[string]string{
						SettingKeyEnableCCHSigning: strconv.FormatBool(enabled),
					}}, cfg),
				}
				account := &Account{ID: 1, Platform: PlatformAnthropic, Type: AccountTypeOAuth}
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
				body := []byte(`{"model":"claude-haiku-4-5","system":[{"type":"text","text":"x-anthropic-billing-header: cc_version=2.1.293.feb; cc_entrypoint=cli;"}],"messages":[{"role":"user","content":"hello"}],"max_tokens":128}`)

				var req *http.Request
				var wireBody []byte
				var err error
				if endpoint == "messages" {
					req, wireBody, err = svc.buildUpstreamRequest(context.Background(), c, account, body, "token", "oauth", "claude-haiku-4-5", false, true)
				} else {
					req, wireBody, err = svc.buildCountTokensRequest(context.Background(), c, account, body, "token", "oauth", "claude-haiku-4-5", true)
				}
				require.NoError(t, err)
				actualBody, err := io.ReadAll(req.Body)
				require.NoError(t, err)
				require.NoError(t, req.Body.Close())
				require.Equal(t, wireBody, actualBody)

				billing := gjson.GetBytes(wireBody, "system.0.text").String()
				if !enabled {
					require.NotContains(t, billing, "cch=")
					return
				}
				require.Contains(t, billing, "cc_entrypoint=cli; cch=")
				got := claudeCCHFromBody(t, wireBody)
				require.NotEqual(t, claudeCCHZero, got)

				offset, ok := claudeBillingCCHDigitsOffset(wireBody)
				require.True(t, ok)
				unsigned := bytes.Clone(wireBody)
				copy(unsigned[offset:offset+claudeCCHLength], claudeCCHZero)
				resigned, err := signAnthropicRequestCCH(unsigned)
				require.NoError(t, err)
				require.Equal(t, wireBody, resigned)
			})
		}
	}
}

func readClaudeCCHGolden(t *testing.T, path string) []byte {
	t.Helper()
	encoded, err := os.ReadFile(path)
	require.NoError(t, err)
	compressed, err := base64.StdEncoding.DecodeString(string(encoded))
	require.NoError(t, err)
	reader, err := gzip.NewReader(bytes.NewReader(compressed))
	require.NoError(t, err)
	body, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.NoError(t, reader.Close())
	return body
}

func claudeCCHFromBody(t *testing.T, body []byte) string {
	t.Helper()
	offset, ok := claudeBillingCCHDigitsOffset(body)
	require.True(t, ok, "billing CCH not found in body")
	return string(body[offset : offset+claudeCCHLength])
}
