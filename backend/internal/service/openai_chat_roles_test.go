//go:build unit

package service

import (
	"bytes"
	"context"
	"fmt"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai_compat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// Only the upstream transport is stubbed. Requests go through the actual Chat
// dispatcher, model mapping, request construction and response/usage handling.
type chatRoleContractUpstream struct {
	httpUpstreamRecorder
	strict bool
}

func (u *chatRoleContractUpstream) Do(req *http.Request, proxyURL string, accountID int64, concurrency int) (*http.Response, error) {
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	req.Body = io.NopCloser(bytes.NewReader(body))
	status, contentType := http.StatusOK, "application/json"
	result := `{"id":"chatcmpl_roles","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"pong"},"finish_reason":"stop"}],"usage":{"prompt_tokens":9,"completion_tokens":1,"total_tokens":10}}`
	if u.strict {
		for _, message := range gjson.GetBytes(body, "messages").Array() {
			if message.Get("role").String() == "developer" {
				status = http.StatusBadRequest
				result = "{\"error\":{\"message\":\"unknown variant `developer`\",\"type\":\"invalid_request_error\"}}"
			}
		}
	}
	if status == http.StatusOK && gjson.GetBytes(body, "stream").Bool() {
		contentType = "text/event-stream"
		result = "data: {\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"pong\"},\"finish_reason\":\"stop\"}]}\n\ndata: {\"choices\":[],\"usage\":{\"prompt_tokens\":9,\"completion_tokens\":1,\"total_tokens\":10}}\n\ndata: [DONE]\n\n"
	}
	u.resp = &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {contentType}}, Body: io.NopCloser(strings.NewReader(result))}
	return u.httpUpstreamRecorder.Do(req, proxyURL, accountID, concurrency)
}

// DoWithTLS routes through this stub's Do override so per-test behavior
// (strict roles, quota sequences) applies on the DoWithTLS path too.
func (u *chatRoleContractUpstream) DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, proxyURL, accountID, accountConcurrency)
}

func TestForwardAsChatCompletions_StrictDeveloperRole(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, stream := range []bool{false, true} {
		for _, target := range []struct{ platform, model, base string }{
			{PlatformDeepseek, "deepseek-flash", "https://api.deepseek.com"},
			{PlatformKimi, "k3", "https://api.kimi.com/coding"},
			{PlatformZhipu, "glm-5.3", "https://open.bigmodel.cn/api/paas/v4"},
			{PlatformOpenAI, "deepseek-flash", "https://api.deepseek.com"},
			{PlatformOpenAI, "k3", "https://api.kimi.com/coding"},
			{PlatformOpenAI, "kimi-k3", "https://api.moonshot.ai"},
			{PlatformOpenAI, "glm-5.3", "https://open.bigmodel.cn/api/paas/v4"},
		} {
			t.Run(fmt.Sprintf("%s/%s/%s/stream=%t", target.platform, target.model, target.base, stream), func(t *testing.T) {
				body := []byte(fmt.Sprintf(`{"model":"example-alias","messages":[{"role":"developer","content":"Keep these instructions."},{"role":"user","content":"ping"}],"stream":%t,"max_tokens":10}`, stream))
				original := bytes.Clone(body)
				upstream := &chatRoleContractUpstream{strict: true}
				svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
				account := rawChatCompletionsTestAccount()
				account.Platform = target.platform
				account.Credentials["base_url"] = target.base
				account.Credentials["api_protocol"] = APIProtocolChatCompletions
				account.Credentials["model_mapping"] = map[string]any{"example-alias": target.model}
				account.Extra = map[string]any{openai_compat.ExtraKeyResponsesSupported: false}

				recorder := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(recorder)
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
				result, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")
				require.NoError(t, err)
				require.NotNil(t, result)
				require.Equal(t, 9, result.Usage.InputTokens)
				require.Equal(t, 1, result.Usage.OutputTokens)
				require.Equal(t, target.model, gjson.GetBytes(upstream.lastBody, "model").String())
				require.Equal(t, "system", gjson.GetBytes(upstream.lastBody, "messages.0.role").String())
				require.Equal(t, "Keep these instructions.", gjson.GetBytes(upstream.lastBody, "messages.0.content").String())
				require.Equal(t, original, body, "per-account adaptation must not mutate the retry body")
				require.Equal(t, http.StatusOK, recorder.Code)
				require.Contains(t, recorder.Body.String(), "pong")
				if stream {
					require.Contains(t, recorder.Body.String(), "data: [DONE]")
				}

				// A subsequent attempt with the same body on a compatible account
				// must still see developer, not the previous account's system role.
				upstream.strict = false
				account.Platform = PlatformOpenAI
				account.Credentials["base_url"] = "https://compatible.example.test/v1"
				recorder = httptest.NewRecorder()
				c, _ = gin.CreateTestContext(recorder)
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
				_, err = svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")
				require.NoError(t, err)
				require.Equal(t, "developer", gjson.GetBytes(upstream.lastBody, "messages.0.role").String())
			})
		}
	}
}

func TestNormalizeStrictChatDeveloperRoles_PreservesConversation(t *testing.T) {
	body := []byte(`{"model":"alias","messages":[{"role":"system","content":"first"},{"role":"developer","content":[{"type":"text","text":"developer is just text","cache_control":{"type":"ephemeral"}}],"name":"rules"},{"role":"user","content":"ping"},{"role":"assistant","content":null,"tool_calls":[{"id":"call_1","type":"function","function":{"name":"echo","arguments":"{\"role\":\"developer\"}"}}]},{"role":"tool","tool_call_id":"call_1","content":"ok"},{"role":"developer","content":"last"}],"metadata":{"large":9007199254740993,"role":"developer"},"stream":true,"reasoning_effort":"high"}`)
	original := bytes.Clone(body)
	want := strings.ReplaceAll(string(body), `"role":"developer","content"`, `"role":"system","content"`)
	for _, platform := range []string{PlatformDeepseek, PlatformKimi, PlatformZhipu} {
		t.Run(platform, func(t *testing.T) {
			account := &Account{Platform: platform, Type: AccountTypeAPIKey}
			got, err := normalizeStrictChatDeveloperRoles(account, "https://relay.example.test/v1/chat/completions", body)
			require.NoError(t, err)
			require.JSONEq(t, want, string(got))
			require.Contains(t, string(got), "9007199254740993")
			require.Equal(t, original, body)
			again, err := normalizeStrictChatDeveloperRoles(account, "https://relay.example.test/v1/chat/completions", got)
			require.NoError(t, err)
			require.Equal(t, got, again)
		})
	}
}

func TestNormalizeStrictChatDeveloperRoles_ExactOfficialHosts(t *testing.T) {
	body := []byte(`{"messages":[{"role":"developer","content":"rules"}]}`)
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	for _, host := range []string{"api.deepseek.com", "api.kimi.com", "api.moonshot.cn", "api.moonshot.ai", "open.bigmodel.cn", "api.z.ai", "API.DEEPSEEK.COM:443"} {
		t.Run(host, func(t *testing.T) {
			got, err := normalizeStrictChatDeveloperRoles(account, "https://"+host+"/v1/chat/completions", body)
			require.NoError(t, err)
			require.JSONEq(t, `{"messages":[{"role":"system","content":"rules"}]}`, string(got))
		})
	}
}

func TestNormalizeStrictChatDeveloperRoles_OtherTargetsUntouched(t *testing.T) {
	// Neither model names nor host-like strings in the path/userinfo may enable
	// normalization on an otherwise compatible provider.
	body := []byte("{ \"model\": \"deepseek-flash\", \"messages\": [{\"role\":\"developer\",\"content\":\"rules\"}] }")
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	for _, target := range []string{
		"https://api.openai.com/v1/chat/completions",
		"https://relay.example.test/v1/chat/completions",
		"https://api.deepseek.com.example.test/v1/chat/completions",
		"https://example.test/api.kimi.com/chat/completions",
		"https://api.deepseek.com@example.test/v1/chat/completions",
		"://invalid-url",
	} {
		got, err := normalizeStrictChatDeveloperRoles(account, target, body)
		require.NoError(t, err)
		require.Equal(t, body, got)
	}
	for _, account := range []*Account{nil, {Platform: PlatformKimi, Type: AccountTypeOAuth}, {Platform: PlatformOpenAI, Type: AccountTypeOAuth}} {
		got, err := normalizeStrictChatDeveloperRoles(account, "https://api.kimi.com/coding/v1/chat/completions", body)
		require.NoError(t, err)
		require.Equal(t, body, got)
	}
}

func TestNormalizeStrictChatDeveloperRoles_CompatibleBodyUntouched(t *testing.T) {
	account := &Account{Platform: PlatformDeepseek, Type: AccountTypeAPIKey}
	for _, body := range []string{
		"{ \"messages\": [{\"role\":\"system\",\"content\":\"rules\"}] }",
		`{"messages":[]}`,
		`{"input":[{"role":"developer","content":"Responses is not Chat"}]}`,
	} {
		got, err := normalizeStrictChatDeveloperRoles(account, "https://api.deepseek.com/v1/chat/completions", []byte(body))
		require.NoError(t, err)
		require.Equal(t, body, string(got))
	}
}

func TestNormalizeStrictChatDeveloperRoles_MalformedBodyDoesNotLeak(t *testing.T) {
	account := &Account{Platform: PlatformDeepseek, Type: AccountTypeAPIKey}
	for _, body := range []string{`{"secret":`, `null`, `[]`, `{"messages":"secret"}`, `{"messages":["secret"]}`, `{"messages":[null]}`, `{"messages":[{"content":"secret"}]}`, `{"messages":[{"role":12,"content":"secret"}]}`} {
		_, err := normalizeStrictChatDeveloperRoles(account, "https://api.deepseek.com/v1/chat/completions", []byte(body))
		require.Error(t, err)
		require.NotContains(t, err.Error(), "secret")
	}
}
