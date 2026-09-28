package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

const geoEnvironment = "<environment_context>\n<cwd>D:\\项目\\代码</cwd>\n<shell>powershell</shell>\n<current_date>2026-09-28</current_date>\n<timezone>Asia/Shanghai</timezone>\n<client_ip>192.0.2.10</client_ip>\n<filesystem><root>D:\\项目\\代码</root></filesystem>\n</environment_context>"
const cleanEnvironment = "<environment_context>\n<cwd>D:\\项目\\代码</cwd>\n<shell>powershell</shell>\n<current_date>2026-09-28</current_date>\n<filesystem><root>D:\\项目\\代码</root></filesystem>\n</environment_context>"

func privacyTestService() *GatewayService {
	return &GatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{RedactClientGeoMetadata: true}}}
}

func TestClientGeoPrivacyAllRegions(t *testing.T) {
	for _, region := range []struct{ timezone, city, country string }{
		{"Asia/Shanghai", "Shanghai", "CN"},
		{"Asia/Tokyo", "Tokyo", "JP"},
		{"America/New_York", "New York", "US"},
		{"Europe/Berlin", "Berlin", "DE"},
		{"Australia/Sydney", "Sydney", "AU"},
	} {
		t.Run(region.timezone, func(t *testing.T) {
			environment := fmt.Sprintf("<environment_context>\n<timezone>%s</timezone>\nCity: %s\nCountry: %s\n<cwd>/work/%s</cwd>\n</environment_context>", region.timezone, region.city, region.country, region.city)
			task := fmt.Sprintf("Compare %s and %s using %s.", region.city, region.country, region.timezone)
			body, err := json.Marshal(map[string]any{
				"metadata":     map[string]any{"timezone": region.timezone, "city": region.city, "country": region.country, "user_id": "kept"},
				"instructions": environment,
				"input":        task,
			})
			require.NoError(t, err)
			got, err := RedactClientGeoMetadata(body)
			require.NoError(t, err)
			require.JSONEq(t, `{"user_id":"kept"}`, gjson.GetBytes(got, "metadata").Raw)
			require.Equal(t, fmt.Sprintf("<environment_context>\n<cwd>/work/%s</cwd>\n</environment_context>", region.city), gjson.GetBytes(got, "instructions").String())
			require.Equal(t, task, gjson.GetBytes(got, "input").String())
		})
	}
}

func TestClientGeoPrivacyEnvironmentScope(t *testing.T) {
	for _, tt := range []struct {
		name, input, want string
		system            bool
	}{
		{"codex user environment", geoEnvironment, cleanEnvironment, false},
		{"system envelope", "Instructions\n" + geoEnvironment + "\nContinue.", "Instructions\n" + cleanEnvironment + "\nContinue.", true},
		{"oauth system wrapper", "[System Instructions]\nInstructions\n" + geoEnvironment, "[System Instructions]\nInstructions\n" + cleanEnvironment, false},
		{"task discussion", "Please edit this:\n" + geoEnvironment, "Please edit this:\n" + geoEnvironment, false},
		{"inline system example", "Example: " + geoEnvironment, "Example: " + geoEnvironment, true},
		{"system file example", "<document>\n" + geoEnvironment + "\n</document>", "<document>\n" + geoEnvironment + "\n</document>", true},
		{"XML comment", "<env>\n<!--\n<timezone>Asia/Shanghai</timezone>\n-->\nTimezone: Asia/Shanghai\n</env>", "<env>\n<!--\n<timezone>Asia/Shanghai</timezone>\n-->\n</env>", false},
		{"fenced system example", "````xml\n```\n" + geoEnvironment + "\n```\n````", "````xml\n```\n" + geoEnvironment + "\n```\n````", true},
		{"multiline field and CRLF", "<env>\r\n<timezone>\r\nAsia/Shanghai\r\n</timezone>\r\nUser timezone: Asia/Shanghai\r\nClient IP: 192.0.2.10\r\nWorking directory: /tmp/Asia/Shanghai\r\n</env>", "<env>\r\nWorking directory: /tmp/Asia/Shanghai\r\n</env>", false},
		{"nested task XML", "<env>\n<document>\n<timezone>Asia/Shanghai</timezone>\nLocation: Shanghai\n</document>\nCity: Shanghai\n</env>", "<env>\n<document>\n<timezone>Asia/Shanghai</timezone>\nLocation: Shanghai\n</document>\n</env>", false},
		{"fenced contents", "<env>\n~~~xml\n<timezone>Asia/Shanghai</timezone>\n~~~\nTimezone: Asia/Shanghai\n</env>", "<env>\n~~~xml\n<timezone>Asia/Shanghai</timezone>\n~~~\n</env>", false},
		{"ordinary task", "Convert Asia/Shanghai to UTC, test 192.0.2.10 and preserve D:\\项目.", "Convert Asia/Shanghai to UTC, test 192.0.2.10 and preserve D:\\项目.", false},
	} {
		t.Run(tt.name, func(t *testing.T) { require.Equal(t, tt.want, redactAutomaticEnvironmentText(tt.input, tt.system)) })
	}
}

func TestClientGeoPrivacyPreservesTaskPayloadsAndWireBytes(t *testing.T) {
	fixture := map[string]any{
		"model": "claude-opus-5-5", "max_tokens": 32,
		"metadata": map[string]any{"timezone": "Asia/Shanghai", "client_ip": "192.0.2.10", "location": map[string]any{"city": "Shanghai"}, "user_id": "session-kept", "custom": "keep"},
		"system":   []any{map[string]any{"type": "text", "text": geoEnvironment, "cache_control": map[string]any{"type": "ephemeral"}}},
		"messages": []any{
			map[string]any{"role": "user", "content": []any{map[string]any{"type": "text", "text": geoEnvironment}, map[string]any{"type": "text", "text": "Task: use Asia/Shanghai and 192.0.2.10"}, map[string]any{"type": "tool_result", "tool_use_id": "t", "content": geoEnvironment}, map[string]any{"type": "image", "source": map[string]any{"type": "url", "url": "https://example.com/Asia/Shanghai.png"}}}},
			map[string]any{"role": "assistant", "content": []any{map[string]any{"type": "text", "text": geoEnvironment}, map[string]any{"type": "tool_use", "id": "t", "name": "write", "input": map[string]any{"timezone": "Asia/Shanghai", "text": geoEnvironment}}}},
		},
		"tools": []any{map[string]any{"name": "write", "description": geoEnvironment, "input_schema": map[string]any{"type": "object"}}},
	}
	body, err := json.Marshal(fixture)
	require.NoError(t, err)
	original := string(body)
	got, err := privacyTestService().redactClientGeoMetadata(body)
	require.NoError(t, err)
	require.Equal(t, original, string(body), "caller buffer must not be mutated")
	require.False(t, gjson.GetBytes(got, "metadata.timezone").Exists())
	require.False(t, gjson.GetBytes(got, "metadata.client_ip").Exists())
	require.False(t, gjson.GetBytes(got, "metadata.location").Exists())
	require.Equal(t, cleanEnvironment, gjson.GetBytes(got, "system.0.text").String())
	require.Equal(t, cleanEnvironment, gjson.GetBytes(got, "messages.0.content.0.text").String())
	for _, path := range []string{"metadata.user_id", "metadata.custom", "system.0.cache_control", "messages.0.content.1", "messages.0.content.2", "messages.0.content.3", "messages.1", "tools"} {
		require.Equal(t, gjson.GetBytes(body, path).Raw, gjson.GetBytes(got, path).Raw, path)
	}
	again, err := privacyTestService().redactClientGeoMetadata(got)
	require.NoError(t, err)
	require.Equal(t, got, again, "idempotent filtering")
	disabled, err := (&GatewayService{}).redactClientGeoMetadata(body)
	require.NoError(t, err)
	require.Equal(t, body, disabled)
	unchanged := []byte("{ \"messages\": [{\"role\":\"user\",\"content\":\"Timezone: Asia/Shanghai\"}] }")
	got, err = privacyTestService().redactClientGeoMetadata(unchanged)
	require.NoError(t, err)
	require.Equal(t, unchanged, got)
	_, err = privacyTestService().redactClientGeoMetadata([]byte(`{"broken":`))
	require.Error(t, err)
}

func TestClientGeoPrivacyOutgoingRequests(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, route := range []string{"oauth", "apikey", "passthrough", "count-oauth", "count-passthrough", "vertex", "bedrock"} {
		t.Run(route, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
			c.Request.Header.Set("X-Forwarded-For", "192.0.2.10")
			c.Request.Header.Set("Accept-Language", "zh-CN")
			account := &Account{Platform: PlatformAnthropic, Type: AccountTypeAPIKey, Credentials: map[string]any{"header_override_enabled": true, "header_overrides": map[string]any{"X-Timezone": "Asia/Shanghai", "X-Forwarded-For": "192.0.2.10", "X-Privacy-Test": "kept"}}}
			body, err := json.Marshal(map[string]any{"model": "claude-opus-5-5", "max_tokens": 32, "messages": []any{map[string]any{"role": "user", "content": geoEnvironment}}})
			require.NoError(t, err)
			svc := privacyTestService()
			var req *http.Request
			switch route {
			case "oauth":
				account.Type = AccountTypeOAuth
				req, _, err = svc.buildUpstreamRequest(context.Background(), c, account, body, "test-token", "oauth", "claude-opus-5-5", false, true)
			case "apikey":
				req, _, err = svc.buildUpstreamRequest(context.Background(), c, account, body, "test-key", "apikey", "claude-opus-5-5", false, false)
			case "passthrough":
				req, _, err = svc.buildUpstreamRequestAnthropicAPIKeyPassthrough(context.Background(), c, account, body, "test-key")
			case "count-oauth":
				account.Type = AccountTypeOAuth
				req, _, err = svc.buildCountTokensRequest(context.Background(), c, account, body, "test-token", "oauth", "claude-opus-5-5", true)
			case "count-passthrough":
				req, err = svc.buildCountTokensRequestAnthropicAPIKeyPassthrough(context.Background(), c, account, body, "test-key")
			case "vertex":
				account.Type = AccountTypeServiceAccount
				account.Credentials = map[string]any{"project_id": "test-project", "location": "us-east5"}
				req, _, err = svc.buildUpstreamRequest(context.Background(), c, account, body, "test-token", "service_account", "claude-opus-5-5", false, false)
			case "bedrock":
				req, err = svc.buildUpstreamRequestBedrockAPIKey(context.Background(), body, "claude-opus-5-5", "us-east-1", false, "test-key")
			}
			require.NoError(t, err)
			got, err := io.ReadAll(req.Body)
			require.NoError(t, err)
			require.Equal(t, cleanEnvironment, gjson.GetBytes(got, "messages.0.content").String())
			for _, header := range []string{"x-timezone", "x-forwarded-for", "accept-language"} {
				require.Empty(t, getHeaderRaw(req.Header, header), header)
			}
			if account.Type == AccountTypeAPIKey && route != "bedrock" {
				require.Equal(t, "kept", getHeaderRaw(req.Header, "x-privacy-test"))
			}
			require.NotEmpty(t, getHeaderRaw(req.Header, "content-type"))
		})
	}
}

func TestClientGeoPrivacyResponsesConversionAndOAuthWrapper(t *testing.T) {
	input, err := json.Marshal([]any{
		map[string]any{"type": "message", "role": "user", "content": []any{map[string]any{"type": "input_text", "text": geoEnvironment}}},
		map[string]any{"type": "message", "role": "user", "content": []any{map[string]any{"type": "input_text", "text": "Task: use Asia/Shanghai"}}},
		map[string]any{"type": "function_call", "call_id": "call_test", "name": "read", "arguments": `{"path":"D:\\项目\\Asia\\Shanghai.txt"}`},
		map[string]any{"type": "function_call_output", "call_id": "call_test", "output": geoEnvironment},
	})
	require.NoError(t, err)
	converted, err := apicompat.ResponsesToAnthropicRequest(&apicompat.ResponsesRequest{Model: "claude-opus-5-5", Instructions: "Instructions\n" + geoEnvironment, Input: input})
	require.NoError(t, err)
	body, err := json.Marshal(converted)
	require.NoError(t, err)
	svc := privacyTestService()
	account := &Account{ID: 10, Platform: PlatformAnthropic, Type: AccountTypeOAuth}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	body = svc.applyClaudeCodeOAuthMimicryToBody(context.Background(), c, account, body, converted.System, "claude-opus-5-5")
	req, _, err := svc.buildUpstreamRequest(context.Background(), c, account, body, "test-token", "oauth", "claude-opus-5-5", true, true)
	require.NoError(t, err)
	got, err := io.ReadAll(req.Body)
	require.NoError(t, err)
	textBlocks, toolResults, toolUses := 0, 0, 0
	gjson.GetBytes(got, "messages").ForEach(func(_, message gjson.Result) bool {
		message.Get("content").ForEach(func(_, block gjson.Result) bool {
			switch block.Get("type").String() {
			case "text":
				text := block.Get("text").String()
				require.NotContains(t, text, "<timezone>")
				if text == "Task: use Asia/Shanghai" {
					textBlocks++
				}
				if text == cleanEnvironment {
					textBlocks++
				}
			case "tool_result":
				require.Contains(t, block.Raw, "Asia/Shanghai")
				toolResults++
			case "tool_use":
				require.Contains(t, block.Get("input.path").String(), "Asia\\Shanghai.txt")
				toolUses++
			}
			return true
		})
		return true
	})
	require.Equal(t, 2, textBlocks)
	require.Equal(t, 1, toolResults)
	require.Equal(t, 1, toolUses)
}

func TestClientGeoPrivacyHeaderCaseAndDisabled(t *testing.T) {
	header := http.Header{"x-forwarded-for": {"192.0.2.10"}, "X-Forwarded-For": {"192.0.2.20"}, "CF-IPCountry": {"CN"}, "Accept-Language": {"zh-CN"}, "Authorization": {"Bearer test"}, "User-Agent": {"client"}}
	unchanged := header.Clone()
	(&GatewayService{}).redactClientGeoHeaders(header)
	require.Equal(t, unchanged, header)
	privacyTestService().redactClientGeoHeaders(header)
	require.Equal(t, http.Header{"Authorization": {"Bearer test"}, "User-Agent": {"client"}}, header)
}
