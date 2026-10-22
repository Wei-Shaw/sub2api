package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

const codexTimezoneTestContext = "<environment_context>\n<current_date>2026-10-23</current_date>\n<timezone>America/Los_Angeles</timezone>\n</environment_context>"

func TestCodexTimezoneValidation(t *testing.T) {
	for _, value := range []string{"", "UTC", "Asia/Taipei", "America/Los_Angeles", " Asia/Taipei "} {
		got, err := NormalizeCodexTimezone(value)
		require.NoError(t, err, value)
		require.Equal(t, strings.TrimSpace(value), got)
	}
	for _, value := range []string{"Local", "not-a-timezone", "Asia/Taipei\nX-Test: injected", strings.Repeat("a", 129)} {
		_, err := NormalizeCodexTimezone(value)
		require.Error(t, err, value)
	}
}

func TestRewriteCodexEnvironmentTimezone(t *testing.T) {
	clientContext := "<codex_apps_client_time_context><timezone>UTC</timezone><current_date>2026-10-22</current_date></codex_apps_client_time_context>"
	body, err := json.Marshal(map[string]any{
		"instructions": clientContext,
		"input": []any{
			map[string]any{"role": "user", "content": []any{map[string]any{"type": "input_text", "text": codexTimezoneTestContext}}},
			map[string]any{"role": "developer", "content": codexTimezoneTestContext},
			map[string]any{"role": "system", "content": codexTimezoneTestContext},
			map[string]any{"role": "assistant", "content": codexTimezoneTestContext},
			map[string]any{"type": "function_call_output", "output": codexTimezoneTestContext},
			map[string]any{"role": "user", "content": "Explain <timezone>America/Los_Angeles</timezone>."},
		},
		"metadata": map[string]any{"timezone": "UTC"},
		"tools":    []any{map[string]any{"description": codexTimezoneTestContext}},
	})
	require.NoError(t, err)
	now := time.Date(2026, 10, 22, 16, 30, 0, 0, time.UTC)
	out, err := rewriteCodexEnvironmentTimezoneAt(body, "Asia/Taipei", now)
	require.NoError(t, err)
	for _, path := range []string{"instructions", "input.2.content"} {
		require.Contains(t, gjson.GetBytes(out, path).String(), "<timezone>Asia/Taipei</timezone>")
	}
	for _, path := range []string{"input.0.content.0.text", "input.1.content", "input.3.content", "input.4.output", "input.5.content", "tools.0.description", "metadata.timezone"} {
		require.Equal(t, gjson.GetBytes(body, path).String(), gjson.GetBytes(out, path).String(), path)
	}
	require.Contains(t, gjson.GetBytes(out, "input.2.content").String(), "<current_date>2026-10-23</current_date>")
	require.Contains(t, gjson.GetBytes(out, "instructions").String(), "<current_date>2026-10-23</current_date>")
	again, err := rewriteCodexEnvironmentTimezoneAt(out, "Asia/Taipei", now)
	require.NoError(t, err)
	require.Equal(t, out, again, "rewriting must be idempotent across retries")
}

func TestCodexTimezoneServerClock(t *testing.T) {
	for _, tc := range []struct {
		name, instant, timezone, wantDate, wantTime string
	}{
		{"Taipei next day", "2026-10-22T16:30:00Z", "Asia/Taipei", "2026-10-23", "2026-10-23T00:30:00+08:00"},
		{"UTC same day", "2026-10-22T16:30:00Z", "UTC", "2026-10-22", "2026-10-22T16:30:00Z"},
		{"server clock in another timezone", "2026-10-22T09:30:00-07:00", "Asia/Taipei", "2026-10-23", "2026-10-23T00:30:00+08:00"},
		{"year boundary", "2026-12-31T16:00:00Z", "Asia/Taipei", "2027-01-01", "2027-01-01T00:00:00+08:00"},
		{"leap day", "2028-02-28T16:00:00Z", "Asia/Taipei", "2028-02-29", "2028-02-29T00:00:00+08:00"},
		{"DST uses location rules", "2026-03-08T07:30:00Z", "America/New_York", "2026-03-08", "2026-03-08T03:30:00-04:00"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			now, err := time.Parse(time.RFC3339, tc.instant)
			require.NoError(t, err)
			body := []byte(`{"input":"<environment_context><timezone>UTC</timezone><current_date>2001-01-01</current_date><current_time>2001-01-01T00:00:00Z</current_time></environment_context>"}`)
			out, err := rewriteCodexEnvironmentTimezoneAt(body, tc.timezone, now)
			require.NoError(t, err)
			text := gjson.GetBytes(out, "input").String()
			require.Contains(t, text, "<timezone>"+tc.timezone+"</timezone>")
			require.Contains(t, text, "<current_date>"+tc.wantDate+"</current_date>")
			require.Contains(t, text, "<current_time>"+tc.wantTime+"</current_time>")
		})
	}
}

func TestCodexTimezoneServerClockPreservesHistoryAndMissingFields(t *testing.T) {
	now := time.Date(2026, 10, 22, 16, 30, 0, 0, time.UTC)
	old := "<environment_context><timezone>UTC</timezone><current_date>2000-01-01</current_date></environment_context>"
	current := "<environment_context><timezone>UTC</timezone><current_date>2001-01-01</current_date></environment_context>"
	for _, role := range []string{"user", "system", "developer"} {
		t.Run(role, func(t *testing.T) {
			body, err := json.Marshal(map[string]any{"input": []any{
				map[string]any{"role": role, "content": old},
				map[string]any{"role": role, "content": []any{map[string]any{"type": "input_text", "text": old + "\n" + current}}},
			}})
			require.NoError(t, err)
			out, err := rewriteCodexEnvironmentTimezoneAt(body, "Asia/Taipei", now)
			require.NoError(t, err)
			require.Equal(t, old, gjson.GetBytes(out, "input.0.content").String())
			latest := gjson.GetBytes(out, "input.1.content.0.text").String()
			require.True(t, strings.HasPrefix(latest, old), "earlier blocks within the same text must retain their original date and timezone")
			require.Contains(t, latest, "<timezone>Asia/Taipei</timezone>")
			require.Contains(t, latest, "<current_date>2026-10-23</current_date>")
			require.NotContains(t, latest, "<current_time>", "do not inject per-second context and disrupt prompt caching")
		})
	}
	dateOnly := []byte(`{"input":"<environment_context><current_date>2001-01-01</current_date></environment_context>"}`)
	out, err := rewriteCodexEnvironmentTimezoneAt(dateOnly, "Asia/Taipei", now)
	require.NoError(t, err)
	require.Contains(t, gjson.GetBytes(out, "input").String(), "<current_date>2026-10-23</current_date>")
	require.NotContains(t, gjson.GetBytes(out, "input").String(), "<timezone>")
	out, err = rewriteCodexEnvironmentTimezoneAt(dateOnly, "", now)
	require.NoError(t, err)
	require.Equal(t, dateOnly, out, "disabled override must preserve client dates")
}

func TestCodexTimezoneServerClockIncrementalContext(t *testing.T) {
	now := time.Date(2026, 10, 22, 16, 30, 0, 0, time.UTC)
	old := "<environment_context><timezone>UTC</timezone><current_date>2000-01-01</current_date></environment_context>"
	active := "<environment_context><timezone>UTC</timezone><current_date>2001-01-01</current_date></environment_context>"
	incremental := "<environment_context><timezone>America/Los_Angeles</timezone><cwd>/tmp/current</cwd></environment_context>"
	body, err := json.Marshal(map[string]any{"input": []any{
		map[string]any{"role": "user", "content": old},
		map[string]any{"role": "user", "content": active},
		map[string]any{"role": "user", "content": incremental},
	}})
	require.NoError(t, err)
	out, err := rewriteCodexEnvironmentTimezoneAt(body, "Asia/Taipei", now)
	require.NoError(t, err)
	require.Equal(t, old, gjson.GetBytes(out, "input.0.content").String())
	require.Contains(t, gjson.GetBytes(out, "input.1.content").String(), "<current_date>2026-10-23</current_date>")
	require.Contains(t, gjson.GetBytes(out, "input.2.content").String(), "<timezone>Asia/Taipei</timezone>")
	require.Contains(t, gjson.GetBytes(out, "input.2.content").String(), "<cwd>/tmp/current</cwd>")
	require.NotContains(t, gjson.GetBytes(out, "input.2.content").String(), "<current_date>")
}

func TestRewriteCodexEnvironmentTimezoneNoopAndRawPreservation(t *testing.T) {
	for _, body := range []string{
		`{"input":"ordinary prose: America/Los_Angeles","number":9007199254740993}`,
		`{"input":"<environment_context><cwd>/tmp</cwd></environment_context>"}`,
	} {
		out, err := rewriteCodexEnvironmentTimezone([]byte(body), "Asia/Taipei")
		require.NoError(t, err)
		require.Equal(t, body, string(out))
	}
	body := []byte(`{"input":"<environment_context><timezone>UTC</timezone></environment_context>","number":9007199254740993,"untouched": { "a" : 1 }}`)
	out, err := rewriteCodexEnvironmentTimezone(body, "Asia/Taipei")
	require.NoError(t, err)
	require.Contains(t, string(out), `"number":9007199254740993`)
	require.Contains(t, string(out), `"untouched": { "a" : 1 }`)
	_, err = rewriteCodexEnvironmentTimezone([]byte(`{invalid`), "Asia/Taipei")
	require.Error(t, err)
	out, err = rewriteCodexEnvironmentTimezone([]byte(`{invalid`), "")
	require.NoError(t, err)
	require.Equal(t, `{invalid`, string(out), "disabled override must not parse the body")
}

func TestRewriteCodexEnvironmentTimezoneWSFrames(t *testing.T) {
	frames := []struct {
		body string
		path string
	}{
		{`{"type":"response.create","input":"<environment_context><timezone>UTC</timezone></environment_context>"}`, "input"},
		{`{"type":"session.update","session":{"instructions":"<environment_context><timezone>UTC</timezone></environment_context>"}}`, "session.instructions"},
		{`{"type":"conversation.item.create","item":{"type":"message","role":"user","content":[{"type":"input_text","text":"<environment_context><timezone>UTC</timezone></environment_context>"}]}}`, "item.content.0.text"},
	}
	for _, frame := range frames {
		out, err := rewriteCodexEnvironmentTimezone([]byte(frame.body), "Asia/Taipei")
		require.NoError(t, err)
		require.Contains(t, gjson.GetBytes(out, frame.path).String(), "<timezone>Asia/Taipei</timezone>")
	}
}

func TestSettingServiceCodexTimezone(t *testing.T) {
	mappingOptions := xai.RuntimeModelMappingOptions()
	t.Cleanup(func() { xai.SetRuntimeModelMappingOptions(mappingOptions) })
	resetGatewayForwardingSettingsCacheForTest(t)
	repo := &gatewayTTLSettingRepo{data: map[string]string{}}
	svc := NewSettingService(repo, &config.Config{})
	ctx := context.Background()
	require.Empty(t, svc.GetOpenAICodexTimezone(ctx))
	settings, err := svc.GetAllSettings(ctx)
	require.NoError(t, err)
	settings.OpenAICodexTimezone = " Asia/Taipei "
	require.NoError(t, svc.UpdateSettings(ctx, settings))
	require.Equal(t, "Asia/Taipei", repo.data[SettingKeyOpenAICodexTimezone])
	require.Equal(t, "Asia/Taipei", svc.GetOpenAICodexTimezone(ctx), "saving must immediately replace the cached default")
	settings, err = svc.GetAllSettings(ctx)
	require.NoError(t, err)
	require.Equal(t, "Asia/Taipei", settings.OpenAICodexTimezone)
	settings.OpenAICodexTimezone = "invalid-zone"
	require.Error(t, svc.UpdateSettings(ctx, settings))
	require.Equal(t, "Asia/Taipei", repo.data[SettingKeyOpenAICodexTimezone])
	settings.OpenAICodexTimezone = ""
	require.NoError(t, svc.UpdateSettings(ctx, settings))
	require.Empty(t, svc.GetOpenAICodexTimezone(ctx))
}

func TestCodexTimezoneAccountScope(t *testing.T) {
	resetGatewayForwardingSettingsCacheForTest(t)
	svc := &OpenAIGatewayService{settingService: NewSettingService(&gatewayTTLSettingRepo{data: map[string]string{
		SettingKeyOpenAICodexTimezone: "Asia/Taipei",
	}}, &config.Config{})}
	body, err := json.Marshal(map[string]any{"input": codexTimezoneTestContext})
	require.NoError(t, err)
	for _, account := range []*Account{
		nil,
		{Platform: PlatformOpenAI, Type: AccountTypeAPIKey},
		{Platform: PlatformAnthropic, Type: AccountTypeOAuth},
	} {
		out, err := svc.applyCodexTimezone(context.Background(), account, body)
		require.NoError(t, err)
		require.Equal(t, body, out)
	}
	for _, accountType := range []string{AccountTypeOAuth, AccountTypeSetupToken} {
		out, err := svc.applyCodexTimezone(context.Background(), &Account{Platform: PlatformOpenAI, Type: accountType}, body)
		require.NoError(t, err)
		require.Contains(t, gjson.GetBytes(out, "input").String(), "<timezone>Asia/Taipei</timezone>")
	}
}

func TestCodexTimezoneAccountOverridesAndGlobalChanges(t *testing.T) {
	mappingOptions := xai.RuntimeModelMappingOptions()
	t.Cleanup(func() { xai.SetRuntimeModelMappingOptions(mappingOptions) })
	resetGatewayForwardingSettingsCacheForTest(t)
	ctx := context.Background()
	settingsService := NewSettingService(&gatewayTTLSettingRepo{data: map[string]string{}}, &config.Config{})
	svc := &OpenAIGatewayService{settingService: settingsService}
	body := []byte(`{"input":"<environment_context><timezone>Europe/London</timezone></environment_context>"}`)
	inherited := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	emptyInherits := &Account{Platform: PlatformOpenAI, Type: AccountTypeSetupToken, Extra: map[string]any{codexTimezoneExtraKey: ""}}
	overridden := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{codexTimezoneExtraKey: "America/New_York"}}
	client := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{codexTimezoneExtraKey: "off"}}
	check := func(account *Account, want string) {
		t.Helper()
		out, err := svc.applyCodexTimezone(ctx, account, body)
		require.NoError(t, err)
		require.Contains(t, gjson.GetBytes(out, "input").String(), "<timezone>"+want+"</timezone>")
	}
	for _, global := range []string{"Asia/Taipei", "UTC", ""} {
		settings, err := settingsService.GetAllSettings(ctx)
		require.NoError(t, err)
		settings.OpenAICodexTimezone = global
		require.NoError(t, settingsService.UpdateSettings(ctx, settings))
		wantInherited := global
		if wantInherited == "" {
			wantInherited = "Europe/London"
		}
		check(inherited, wantInherited)
		check(emptyInherits, wantInherited)
		check(overridden, "America/New_York")
		check(client, "Europe/London")
	}
	// Account choices still work without a global setting service.
	svc.settingService = nil
	check(overridden, "America/New_York")
	check(inherited, "Europe/London")
	check(client, "Europe/London")
}

func TestCodexTimezoneInvalidAccountWritesAreRejected(t *testing.T) {
	ctx := context.Background()
	for _, invalid := range []any{"not-a-timezone", "Local", true, 8} {
		extra := map[string]any{codexTimezoneExtraKey: invalid}
		// Invalid account settings must fail before touching any repository.
		admin := &adminServiceImpl{}
		_, err := buildAccountForCreate(&CreateAccountInput{}, extra)
		require.Error(t, err)
		_, err = admin.UpdateAccount(ctx, 1, &UpdateAccountInput{Extra: extra})
		require.Error(t, err)
		require.Error(t, admin.UpdateAccountExtra(ctx, 1, extra))
		_, err = admin.BulkUpdateAccounts(ctx, &BulkUpdateAccountsInput{Extra: extra})
		require.Error(t, err)
		accountService := &AccountService{}
		_, err = accountService.Create(ctx, CreateAccountRequest{Extra: extra})
		require.Error(t, err)
		_, err = accountService.Update(ctx, 1, UpdateAccountRequest{Extra: &extra})
		require.Error(t, err)
	}
	for _, valid := range []any{nil, "", "off", "Asia/Taipei", " UTC "} {
		require.NoError(t, ValidateCodexTimezoneExtra(map[string]any{codexTimezoneExtraKey: valid}))
	}
}

func TestCodexTimezoneHTTPForward(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, passthrough := range []bool{false, true} {
		t.Run(map[bool]string{false: "transform", true: "passthrough"}[passthrough], func(t *testing.T) {
			resetGatewayForwardingSettingsCacheForTest(t)
			body, err := json.Marshal(map[string]any{
				"model": "gpt-5.4", "stream": true, "instructions": "Reply briefly.",
				"input": []any{map[string]any{"role": "user", "content": codexTimezoneTestContext}},
			})
			require.NoError(t, err)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
			upstream := &httpUpstreamRecorder{err: errors.New("stop after capturing outbound body")}
			svc := &OpenAIGatewayService{
				cfg: &config.Config{}, httpUpstream: upstream,
				settingService: NewSettingService(&gatewayTTLSettingRepo{data: map[string]string{SettingKeyOpenAICodexTimezone: "Asia/Taipei"}}, &config.Config{}),
			}
			account := &Account{
				ID: 1724, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1,
				Credentials: map[string]any{"access_token": "test-token", "chatgpt_account_id": "test-account"},
				Extra:       map[string]any{"openai_passthrough": passthrough},
			}
			_, err = svc.Forward(context.Background(), c, account, body)
			require.Error(t, err)
			require.NotEmpty(t, upstream.lastBody)
			require.Contains(t, string(upstream.lastBody), "Asia/Taipei")
			require.NotContains(t, string(upstream.lastBody), "America/Los_Angeles")
			require.Contains(t, gjson.GetBytes(body, "input.0.content").String(), "America/Los_Angeles", "caller body must not be mutated")
		})
	}
}

func TestCodexTimezoneWSPassthroughMultipleTurns(t *testing.T) {
	gin.SetMode(gin.TestMode)
	resetGatewayForwardingSettingsCacheForTest(t)
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(context.Canceled)
	upstream := newStagedPassthroughConn()
	cfg := passthroughLifecycleConfig()
	cfg.Gateway.OpenAIWS.OAuthEnabled = true
	cfg.Gateway.OpenAIFirstOutputTimeoutSeconds = 5
	cfg.Gateway.OpenAIWS.IngressInterTurnIdleTimeoutSeconds = 5
	cfg.Gateway.OpenAIWS.ReadTimeoutSeconds = 5
	account := passthroughLifecycleAccount()
	account.Type = AccountTypeOAuth
	account.Credentials = map[string]any{"access_token": "test-token", "chatgpt_account_id": "test-account"}
	account.Extra = map[string]any{"openai_oauth_responses_websockets_v2_mode": OpenAIWSIngressModePassthrough}
	svc := newPassthroughLifecycleService(cfg, upstream)
	svc.settingService = NewSettingService(&gatewayTTLSettingRepo{data: map[string]string{SettingKeyOpenAICodexTimezone: "Asia/Taipei"}}, cfg)
	server, serverErr := startPassthroughLifecycleServer(t, ctx, svc, account)
	defer server.Close()
	defer func() { _ = upstream.Close() }()
	body, err := json.Marshal(map[string]any{
		"type": "response.create", "model": "gpt-5.4", "stream": true,
		"input": []any{map[string]any{"role": "user", "content": codexTimezoneTestContext}},
	})
	require.NoError(t, err)
	client := dialPassthroughLifecycleClientWithPayload(t, server, string(body))
	defer func() { _ = client.CloseNow() }()
	for turn := 0; turn < 2; turn++ {
		if turn > 0 {
			writeCtx, cancelWrite := context.WithTimeout(ctx, 3*time.Second)
			err = client.Write(writeCtx, coderws.MessageText, body)
			cancelWrite()
			require.NoError(t, err)
		}
		out := requirePassthroughUpstreamWrite(t, upstream, 3*time.Second)
		require.Contains(t, gjson.GetBytes(out, "input.0.content").String(), "<timezone>Asia/Taipei</timezone>")
		require.NotContains(t, string(out), "America/Los_Angeles")
		upstream.Send(`{"type":"response.completed","response":{"id":"resp_timezone_` + string(rune('1'+turn)) + `","model":"gpt-5.4","usage":{"input_tokens":1,"output_tokens":1}}}`)
		completed, readErr := readPassthroughLifecycleFrame(t, client, 3*time.Second)
		require.NoError(t, readErr)
		require.Equal(t, "response.completed", gjson.GetBytes(completed, "type").String())
	}
	_ = client.CloseNow()
	cancel(context.Canceled)
	select {
	case <-serverErr:
	case <-time.After(3 * time.Second):
		t.Fatal("websocket relay did not stop")
	}
}
