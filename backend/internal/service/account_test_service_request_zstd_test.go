//go:build unit

package service

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai_compat"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/stretchr/testify/require"
)

// zstdProbeUpstream 扮演传输层：看到 ctx 上的压缩探针就记一次（真实压缩在 repository 层，有单独测试）。
type zstdProbeUpstream struct {
	sawProbe []bool
}

func (u *zstdProbeUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	return u.serve(req)
}

func (u *zstdProbeUpstream) DoWithTLS(req *http.Request, _ string, _ int64, _ int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.serve(req)
}

func (u *zstdProbeUpstream) serve(req *http.Request) (*http.Response, error) {
	probe := UpstreamRequestZstdProbeFromContext(req.Context())
	u.sawProbe = append(u.sawProbe, probe != nil)
	if probe != nil {
		body, _ := io.ReadAll(req.Body)
		probe.Record(UpstreamRequestZstdAttempt{Host: req.URL.Host, RawBytes: len(body), WireBytes: len(body) / 2})
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader("data: {\"type\":\"response.completed\"}\n\n")),
	}, nil
}

func zstdTestAccount(enabled bool) *Account {
	extra := map[string]any{openai_compat.ExtraKeyResponsesSupported: true}
	if enabled {
		extra[AccountExtraUpstreamRequestZstd] = true
	}
	return &Account{
		ID:          96,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Concurrency: 1,
		Credentials: map[string]any{"api_key": "sk-test", "base_url": "https://relay.example/v1"},
		Extra:       extra,
	}
}

func sseEventsOf(t *testing.T, raw string) []map[string]any {
	t.Helper()
	var events []map[string]any
	for _, line := range strings.Split(raw, "\n") {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var ev map[string]any
		require.NoError(t, json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &ev))
		events = append(events, ev)
	}
	return events
}

func TestAccountTestCompressRequestBodyReportsAttempts(t *testing.T) {
	c, rec := newTestContext()
	account := zstdTestAccount(true)
	upstream := &zstdProbeUpstream{}
	svc := &AccountTestService{
		accountRepo:  &mockAccountRepoForGemini{accountsByID: map[int64]*Account{account.ID: account}},
		httpUpstream: upstream,
		cfg:          &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: false}}},
	}

	err := svc.TestAccountConnection(c, account.ID, "gpt-5.4-mini", "", "", AccountTestOptions{CompressRequestBody: new(true)})
	require.NoError(t, err)
	require.Equal(t, []bool{true}, upstream.sawProbe)
	require.Nil(t, UpstreamRequestZstdProbeFromContext(c.Request.Context()))

	events := sseEventsOf(t, rec.Body.String())
	require.NotEmpty(t, events)
	last := events[len(events)-1]
	require.Equal(t, "test_complete", last["type"])
	require.Equal(t, true, last["success"])
	data, ok := last["request_compression"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, true, data["account_enabled"])
	attempts, ok := data["attempts"].([]any)
	require.True(t, ok)
	require.Len(t, attempts, 1)
	attempt, ok := attempts[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "relay.example", attempt["host"])
	rawBytes, ok := attempt["raw_bytes"].(float64)
	require.True(t, ok)
	require.Positive(t, rawBytes)
}

// 没压缩到任何请求时 attempts 是空数组（不是 null），前端据此提示「无法判断」。
func TestAccountTestCompressRequestBodyEmptyAttemptsIsArray(t *testing.T) {
	c, rec := newTestContext()
	account := zstdTestAccount(false)
	svc := &AccountTestService{
		accountRepo:  &mockAccountRepoForGemini{accountsByID: map[int64]*Account{account.ID: account}},
		httpUpstream: &queuedHTTPUpstream{},
		cfg:          &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: false}}},
	}

	_ = svc.TestAccountConnection(c, account.ID, "gpt-5.4-mini", "", "", AccountTestOptions{CompressRequestBody: new(true)})
	require.Contains(t, rec.Body.String(), `"request_compression":{"attempts":[],"account_enabled":false}`)
}

func TestAccountTestWithoutCompressRequestBodyHasNoProbe(t *testing.T) {
	c, rec := newTestContext()
	account := zstdTestAccount(true)
	upstream := &zstdProbeUpstream{}
	svc := &AccountTestService{
		accountRepo:  &mockAccountRepoForGemini{accountsByID: map[int64]*Account{account.ID: account}},
		httpUpstream: upstream,
		cfg:          &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: false}}},
	}

	require.NoError(t, svc.TestAccountConnection(c, account.ID, "gpt-5.4-mini", "", ""))
	require.Equal(t, []bool{false}, upstream.sawProbe)
	require.NotContains(t, rec.Body.String(), "request_compression")
}

func TestAccountIsUpstreamRequestZstdEnabled(t *testing.T) {
	require.False(t, (*Account)(nil).IsUpstreamRequestZstdEnabled())
	require.False(t, (&Account{}).IsUpstreamRequestZstdEnabled())
	require.False(t, (&Account{Type: AccountTypeAPIKey, Extra: map[string]any{AccountExtraUpstreamRequestZstd: "true"}}).IsUpstreamRequestZstdEnabled())
	require.True(t, (&Account{Type: AccountTypeAPIKey, Extra: map[string]any{AccountExtraUpstreamRequestZstd: true}}).IsUpstreamRequestZstdEnabled())
}
