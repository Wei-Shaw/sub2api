//go:build unit

package service

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestIsAdvisorToolUnsupportedError(t *testing.T) {
	cases := []struct {
		name     string
		body     string
		patterns []string
		want     bool
	}{
		{
			name: "exact upstream message hit",
			body: `{"error":{"message":"Unexpected value(s) ` + "`advisor-tool-2026-03-01`" + ` for the ` + "`anthropic-beta`" + ` header.","type":"upstream_error"}}`,
			want: true,
		},
		{
			name: "case-insensitive token",
			body: `{"error":{"message":"unexpected value(s) Advisor-Tool-2026-03-01 for the Anthropic-Beta header"}}`,
			want: true,
		},
		{
			name: "missing anthropic-beta keyword",
			body: `{"error":{"message":"advisor-tool-2026-03-01 is not enabled"}}`,
			want: false,
		},
		{
			name: "missing advisor token keyword",
			body: `{"error":{"message":"Unexpected value(s) for the anthropic-beta header"}}`,
			want: false,
		},
		{
			name: "empty body",
			body: ``,
			want: false,
		},
		{
			name: "non-json body",
			body: `not json`,
			want: false,
		},
		{
			name: "missing error.message field",
			body: `{"error":{"type":"upstream_error"}}`,
			want: false,
		},
		{
			name:     "custom pattern hit",
			body:     `{"error":{"message":"unsupported beta feature: my-custom-flag"}}`,
			patterns: []string{"my-custom-flag"},
			want:     true,
		},
		{
			name:     "custom pattern with empty/whitespace entries skipped",
			body:     `{"error":{"message":"random unrelated text"}}`,
			patterns: []string{"", "  ", "\t"},
			want:     false,
		},
		{
			name:     "custom pattern case-insensitive",
			body:     `{"error":{"message":"BLAH UNRECOGNIZED"}}`,
			patterns: []string{"unrecognized"},
			want:     true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := IsAdvisorToolUnsupportedError([]byte(tc.body), tc.patterns)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestRectifyAdvisorTool(t *testing.T) {
	t.Run("removes single advisor tool", func(t *testing.T) {
		body := []byte(`{"tools":[{"type":"advisor_20260301","name":"advisor","model":"claude-opus-4-6"}]}`)
		out, applied := RectifyAdvisorTool(body)
		require.True(t, applied)
		require.False(t, gjson.GetBytes(out, "tools.0").Exists())
	})

	t.Run("preserves other tools", func(t *testing.T) {
		body := []byte(`{"tools":[
			{"type":"custom","name":"calc"},
			{"type":"advisor_20260301","name":"advisor","model":"claude-opus-4-6"},
			{"type":"web_search_20250305","name":"search"}
		]}`)
		out, applied := RectifyAdvisorTool(body)
		require.True(t, applied)
		tools := gjson.GetBytes(out, "tools").Array()
		require.Len(t, tools, 2)
		require.Equal(t, "custom", tools[0].Get("type").String())
		require.Equal(t, "web_search_20250305", tools[1].Get("type").String())
	})

	t.Run("removes multiple advisor tools", func(t *testing.T) {
		body := []byte(`{"tools":[
			{"type":"advisor_20260301","name":"a1"},
			{"type":"custom","name":"keep"},
			{"type":"advisor_20260301","name":"a2"}
		]}`)
		out, applied := RectifyAdvisorTool(body)
		require.True(t, applied)
		tools := gjson.GetBytes(out, "tools").Array()
		require.Len(t, tools, 1)
		require.Equal(t, "custom", tools[0].Get("type").String())
	})

	t.Run("no-op when tools missing", func(t *testing.T) {
		body := []byte(`{"model":"claude-sonnet-4-6","messages":[]}`)
		out, applied := RectifyAdvisorTool(body)
		require.False(t, applied)
		require.Equal(t, string(body), string(out))
	})

	t.Run("no-op when tools is empty array", func(t *testing.T) {
		body := []byte(`{"tools":[]}`)
		out, applied := RectifyAdvisorTool(body)
		require.False(t, applied)
		require.Equal(t, string(body), string(out))
	})

	t.Run("no-op when tools is non-array", func(t *testing.T) {
		body := []byte(`{"tools":"oops"}`)
		out, applied := RectifyAdvisorTool(body)
		require.False(t, applied)
		require.Equal(t, string(body), string(out))
	})

	t.Run("no-op when tools has no advisor", func(t *testing.T) {
		body := []byte(`{"tools":[{"type":"custom","name":"calc"}]}`)
		out, applied := RectifyAdvisorTool(body)
		require.False(t, applied)
		require.Equal(t, string(body), string(out))
	})

	t.Run("returns valid json", func(t *testing.T) {
		body := []byte(`{"tools":[{"type":"advisor_20260301"},{"type":"custom"}],"messages":[{"role":"user"}]}`)
		out, applied := RectifyAdvisorTool(body)
		require.True(t, applied)
		require.True(t, gjson.ValidBytes(out))
		require.Equal(t, "user", gjson.GetBytes(out, "messages.0.role").String())
	})
}

func TestAdvisorBetaTokenConstants(t *testing.T) {
	// 防御回归：客户端约定的 beta token 与 tool type 字面值不能漂移。
	require.Equal(t, "advisor-tool-2026-03-01", AdvisorBetaToken)
	require.Equal(t, "advisor_20260301", AdvisorToolType)
	// DefaultAdvisorToolPattern 必须包含 token 名称，确保管理员后台默认关键词仍能命中上游报错。
	require.True(t, strings.Contains(DefaultAdvisorToolPattern, AdvisorBetaToken))
	require.True(t, strings.Contains(DefaultAdvisorToolPattern, "anthropic-beta"))
}
