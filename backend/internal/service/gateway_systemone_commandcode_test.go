package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func commandCodeSystemOneAccount(baseURL string) *Account {
	return &Account{ID: 10, Platform: PlatformCommandCode, Type: AccountTypeAPIKey, Credentials: map[string]any{
		"base_url": baseURL, "api_key": "cc-secret",
		// A synced chat whitelist: Jev is never in Command Code's chat catalog.
		"model_mapping": map[string]any{"deepseek/deepseek-v4-flash": "deepseek/deepseek-v4-flash"},
	}}
}

func TestForwardSystemOneToCommandCode(t *testing.T) {
	requestBody := []byte(`{"model":"typesafe/jev","state":"Payments failed for three days.","questions":{"urgent":{"type":"noul","instructions":"Urgent?"}}}`)
	responseBody := []byte(`{"model":"typesafe/jev","answers":{"urgent":{"type":"noul","noul":0.89}},"usage":{"input_tokens":278,"output_tokens":20}}`)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/provider/v1/systemone", r.URL.Path)
		require.Equal(t, "Bearer cc-secret", r.Header.Get("Authorization"))
		got, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.Equal(t, requestBody, got)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(responseBody)
	}))
	defer server.Close()

	svc := newSystemOneTestService(&systemOneHTTPUpstream{do: server.Client().Do})
	result, err := svc.ForwardSystemOneModel(context.Background(), newSystemOneTestContext(),
		commandCodeSystemOneAccount(server.URL+"/provider/v1"), requestBody, "typesafe/jev")
	require.NoError(t, err)
	require.Equal(t, responseBody, result.Body)
	require.Equal(t, "typesafe/jev", result.Model)
	require.Equal(t, 278, result.Usage.InputTokens)
	require.Equal(t, 20, result.Usage.OutputTokens)
}

func TestForwardSystemOneSendsCanonicalUserAgentToOfficialCommandCode(t *testing.T) {
	var gotUA, gotURL string
	upstream := &systemOneHTTPUpstream{do: func(r *http.Request) (*http.Response, error) {
		gotUA, gotURL = r.Header.Get("User-Agent"), r.URL.String()
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}},
			Body: io.NopCloser(strings.NewReader(`{"model":"typesafe/jev","answers":{},"usage":{}}`))}, nil
	}}
	svc := newSystemOneTestService(upstream)
	_, err := svc.ForwardSystemOneModel(context.Background(), newSystemOneTestContext(),
		commandCodeSystemOneAccount("https://api.commandcode.ai/provider/v1"),
		[]byte(`{"model":"typesafe/jev","state":"s","questions":{"q":{"type":"noul"}}}`), "typesafe/jev")
	require.NoError(t, err)
	require.Equal(t, "https://api.commandcode.ai/provider/v1/systemone", gotURL)
	require.Equal(t, CodexCanonicalUserAgent(), gotUA)
}

func TestSystemOneModelServedBy(t *testing.T) {
	cc := commandCodeSystemOneAccount("https://api.commandcode.ai/provider/v1")
	ts := &Account{Platform: PlatformTypeSafe, Type: AccountTypeAPIKey}
	require.True(t, SystemOneModelServedBy(cc, "typesafe/jev"))
	require.True(t, SystemOneModelServedBy(cc, "typesafe/jev-1.14.0"))
	require.False(t, SystemOneModelServedBy(cc, "jev-latest"))
	require.True(t, SystemOneModelServedBy(ts, "jev-latest"))
	require.False(t, SystemOneModelServedBy(ts, "typesafe/jev"))
	require.False(t, SystemOneModelServedBy(&Account{Platform: PlatformAnthropic, Type: AccountTypeAPIKey}, "typesafe/jev"))
	// A Command Code chat model is not a System One model.
	_, err := newSystemOneTestService(nil).ForwardSystemOneModel(context.Background(), newSystemOneTestContext(), cc, []byte(`{}`), "deepseek/deepseek-v4-flash")
	require.Error(t, err)
}

func TestCommandCodeAccountSchedulesJevDespiteChatWhitelist(t *testing.T) {
	svc := &GatewayService{}
	cc := commandCodeSystemOneAccount("https://api.commandcode.ai/provider/v1")
	require.True(t, svc.isModelSupportedByAccount(cc, "typesafe/jev"))
	require.True(t, svc.isModelSupportedByAccount(cc, "typesafe/jev-1.14.0"))
	require.True(t, svc.isModelSupportedByAccount(cc, "deepseek/deepseek-v4-flash"))
	// The whitelist still applies to everything else.
	require.False(t, svc.isModelSupportedByAccount(cc, "claude-sonnet-5-5"))
	// Other platforms do not pick up Jev ids.
	other := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{
		"model_mapping": map[string]any{"gpt-5.5": "gpt-5.5"}}}
	require.False(t, svc.isModelSupportedByAccount(other, "typesafe/jev"))
}
