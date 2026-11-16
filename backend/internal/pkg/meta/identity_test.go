//go:build unit

package meta

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestUserAgentForPlatformPair(t *testing.T) {
	require.Equal(t,
		"muse-build/1.4.14 (non-interactive; linux-x86_64; build dec7d7497fecdad135ec5835a8161dbc92e55b58)",
		UserAgentFor("linux", "amd64"))
	require.Equal(t,
		"muse-build/1.4.14 (non-interactive; macos-aarch64; build dec7d7497fecdad135ec5835a8161dbc92e55b58)",
		UserAgentFor("darwin", "arm64"))
}

func TestApplyClientIdentityStampsMuseHeaders(t *testing.T) {
	header := http.Header{}
	ApplyClientIdentity(header, "")
	require.Equal(t, UserAgent(), header.Get("User-Agent"))
	require.Equal(t, "tbh:exec", header.Get("X-Client-Id"))
	sessionID := header.Get("X-Tbh-Session-Id")
	require.NotEmpty(t, sessionID)
	require.Equal(t, sessionID, header.Get("X-Meta-Ai-Gateway-Session-Id"))
	require.Equal(t, sessionID, header.Get("X-Tbh-Thread-Id"))
	require.Equal(t, sessionID, header.Get("X-Tbh-Root-Session-Id"))
	require.Equal(t, "main", header.Get("X-Tbh-Agent-Role"))
}

func TestApplyClientIdentityKeepsProvidedSession(t *testing.T) {
	header := http.Header{}
	header.Set("X-Tbh-Session-Id", "external-session")
	ApplyClientIdentity(header, "ignored")
	require.Equal(t, "external-session", header.Get("X-Tbh-Session-Id"))
	require.Empty(t, header.Get("X-Tbh-Root-Session-Id"), "a pre-stamped session is left untouched")
}

func TestNewSessionIDIsUUIDv7(t *testing.T) {
	id := NewSessionID()
	require.Len(t, id, 36)
	require.Equal(t, byte('7'), id[14])
}

func TestIsContributorModel(t *testing.T) {
	require.True(t, IsContributorModel("muse-spark-1.3-contributor"))
	require.True(t, IsContributorModel("  MUSE-WONDER-FULL-1.0-CONTRIBUTOR "))
	require.False(t, IsContributorModel("muse-spark-1.3"))
	require.False(t, IsContributorModel(""))
}

func TestApplyContributorMode(t *testing.T) {
	header := http.Header{}
	ApplyContributorMode(header, "muse-spark-1.3")
	require.Empty(t, header.Get("X-Tbh-Contributor-Mode"))
	ApplyContributorMode(header, "muse-spark-1.3-contributor")
	require.Equal(t, "1", header.Get("X-Tbh-Contributor-Mode"))
}

func TestEnsureMuseToolNamespace(t *testing.T) {
	t.Run("tool bearing request appends namespace last", func(t *testing.T) {
		body := []byte(`{"model":"muse-spark-1.3","tools":[{"type":"function","name":"shell"},{"type":"web_search"}]}`)
		got := EnsureMuseToolNamespace(body)
		tools := gjson.GetBytes(got, "tools").Array()
		require.Len(t, tools, 3)
		require.Equal(t, "shell", tools[0].Get("name").String())
		require.Equal(t, "web_search", tools[1].Get("type").String())
		require.Equal(t, "namespace", tools[2].Get("type").String())
		require.Equal(t, "muse", tools[2].Get("name").String())
		sub := tools[2].Get("tools.0")
		require.Equal(t, "submit_reminder_decision", sub.Get("name").String())
		require.True(t, sub.Get("strict").Bool())
		require.Equal(t, "muse-spark-1.3", gjson.GetBytes(got, "model").String())
	})

	t.Run("idempotent once namespace exists", func(t *testing.T) {
		body := []byte(`{"tools":[{"type":"function","name":"shell"},{"type":"namespace","name":"muse","tools":[]}]}`)
		require.Equal(t, string(body), string(EnsureMuseToolNamespace(body)))
	})

	t.Run("tool-less request untouched", func(t *testing.T) {
		body := []byte(`{"model":"muse-spark-1.3","input":"hi"}`)
		require.Equal(t, string(body), string(EnsureMuseToolNamespace(body)))
	})

	t.Run("empty tools untouched", func(t *testing.T) {
		body := []byte(`{"tools":[]}`)
		require.Equal(t, string(body), string(EnsureMuseToolNamespace(body)))
	})

	t.Run("invalid json untouched", func(t *testing.T) {
		body := []byte("{not json")
		require.Equal(t, string(body), string(EnsureMuseToolNamespace(body)))
	})

	t.Run("appended namespace keeps json valid and big numbers intact", func(t *testing.T) {
		body := []byte(`{"temperature":0.7,"seed":12345678901234567890,"tools":[{"type":"function","name":"x"}]}`)
		got := EnsureMuseToolNamespace(body)
		require.True(t, json.Valid(got))
		require.Equal(t, "12345678901234567890", gjson.GetBytes(got, "seed").Raw)
	})
}

func TestParseRetryAfter(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	future := now.Add(90 * time.Second).Unix()
	past := now.Add(-time.Minute).Unix()
	body := []byte(`{"error":{"resets_at":` + strconv.FormatInt(future, 10) + `}}`)

	retryAfter := ParseRetryAfter(http.StatusTooManyRequests, body, now)
	require.NotNil(t, retryAfter)
	require.Equal(t, 90*time.Second, *retryAfter)
	require.NotNil(t, ParseRetryAfter(http.StatusNotFound, body, now))
	require.Nil(t, ParseRetryAfter(http.StatusBadRequest, body, now))
	require.Nil(t, ParseRetryAfter(http.StatusTooManyRequests, []byte("{not json"), now))
	require.Nil(t, ParseRetryAfter(http.StatusTooManyRequests, []byte(`{"error":{"resets_at":`+strconv.FormatInt(past, 10)+`}}`), now))
	require.Nil(t, ParseRetryAfter(http.StatusTooManyRequests, nil, now))
}

func TestIsSubscriptionQuota(t *testing.T) {
	require.True(t, IsSubscriptionQuota(http.StatusTooManyRequests, []byte(`{"error":{"message":"subscription quota exhausted"}}`)))
	require.True(t, IsSubscriptionQuota(http.StatusTooManyRequests, []byte(`{"error":{"message":"Quota Exhausted"}}`)))
	require.True(t, IsSubscriptionQuota(http.StatusTooManyRequests, []byte(`{"error":{"code":"rate_limit_exceeded","resets_at":1800000090}}`)))
	require.True(t, IsSubscriptionQuota(http.StatusTooManyRequests, []byte(`{"error":{"code":"subscription_quota","resets_at":1800000090}}`)))
	require.False(t, IsSubscriptionQuota(http.StatusTooManyRequests, []byte(`{"error":{"message":"too many requests"}}`)))
	require.False(t, IsSubscriptionQuota(http.StatusNotFound, []byte(`{"error":{"message":"subscription quota"}}`)))
	require.False(t, IsSubscriptionQuota(http.StatusTooManyRequests, nil))
}
