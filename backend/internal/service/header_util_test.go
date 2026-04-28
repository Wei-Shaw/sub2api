//go:build unit

package service

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDelHeaderRaw(t *testing.T) {
	t.Run("removes wire-casing lowercase key (anthropic-beta)", func(t *testing.T) {
		// 模拟 setHeaderRaw 的存储形式：anthropic-beta 以小写 wire casing 存于 map。
		h := http.Header{}
		setHeaderRaw(h, "anthropic-beta", "claude-code-20250219,oauth-2025-04-20")
		require.NotEmpty(t, h["anthropic-beta"])

		// 普通 Header.Del 走 canonical（"Anthropic-Beta"），无法删小写 key——这正是 bug 根因。
		h2 := http.Header{}
		setHeaderRaw(h2, "anthropic-beta", "claude-code-20250219")
		h2.Del("anthropic-beta")
		require.NotEmpty(t, h2["anthropic-beta"], "vanilla Header.Del must NOT remove wire-casing key (regression guard)")

		// delHeaderRaw 应同时删 canonical + wire casing + raw 三种形式。
		delHeaderRaw(h, "anthropic-beta")
		require.Empty(t, h["anthropic-beta"])
		require.Empty(t, h["Anthropic-Beta"])
		require.Empty(t, h.Get("anthropic-beta"))
	})

	t.Run("removes canonical-cased key", func(t *testing.T) {
		h := http.Header{}
		h.Set("Authorization", "Bearer x")
		require.NotEmpty(t, h["Authorization"])

		delHeaderRaw(h, "Authorization")
		require.Empty(t, h["Authorization"])
		require.Empty(t, h.Get("Authorization"))
	})

	t.Run("noop on missing key", func(t *testing.T) {
		h := http.Header{}
		require.NotPanics(t, func() {
			delHeaderRaw(h, "anthropic-beta")
		})
	})

	t.Run("removes both canonical and wire casing entries simultaneously", func(t *testing.T) {
		// 极端情况：map 里同时有 canonical 和 wire casing 两份（不应该发生但要防御）。
		h := http.Header{}
		h["Anthropic-Beta"] = []string{"a"}
		h["anthropic-beta"] = []string{"b"}

		delHeaderRaw(h, "anthropic-beta")
		require.Empty(t, h["Anthropic-Beta"])
		require.Empty(t, h["anthropic-beta"])
	})
}

func TestContainsBetaTokenIgnoreCase(t *testing.T) {
	cases := []struct {
		name   string
		header string
		token  string
		want   bool
	}{
		{"exact lowercase match", "advisor-tool-2026-03-01,oauth-2025-04-20", "advisor-tool-2026-03-01", true},
		{"upper-cased token in header", "Advisor-Tool-2026-03-01,oauth-2025-04-20", "advisor-tool-2026-03-01", true},
		{"mixed case both sides", "ADVISOR-tool-2026-03-01", "Advisor-Tool-2026-03-01", true},
		{"token absent", "claude-code-20250219,oauth-2025-04-20", "advisor-tool-2026-03-01", false},
		{"empty header", "", "advisor-tool-2026-03-01", false},
		{"empty token", "advisor-tool-2026-03-01", "", false},
		{"whitespace around tokens", " advisor-tool-2026-03-01 , oauth-2025-04-20 ", "advisor-tool-2026-03-01", true},
		{"prefix collision should not match", "advisor-tool-2026-03-01-extra", "advisor-tool-2026-03-01", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := containsBetaTokenIgnoreCase(tc.header, tc.token)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestStripBetaTokenIgnoreCase(t *testing.T) {
	cases := []struct {
		name   string
		header string
		token  string
		want   string
	}{
		{"strip only token leaves empty string", "advisor-tool-2026-03-01", "advisor-tool-2026-03-01", ""},
		{"strip case-insensitive variant", "Advisor-Tool-2026-03-01", "advisor-tool-2026-03-01", ""},
		{"keeps other tokens", "claude-code-20250219,advisor-tool-2026-03-01,oauth-2025-04-20", "advisor-tool-2026-03-01", "claude-code-20250219,oauth-2025-04-20"},
		{"strips whitespace and reformats", " claude-code-20250219 , Advisor-Tool-2026-03-01 ", "advisor-tool-2026-03-01", "claude-code-20250219"},
		{"token absent unchanged", "claude-code-20250219", "advisor-tool-2026-03-01", "claude-code-20250219"},
		{"empty header unchanged", "", "advisor-tool-2026-03-01", ""},
		{"empty token unchanged", "claude-code-20250219", "", "claude-code-20250219"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := stripBetaTokenIgnoreCase(tc.header, tc.token)
			require.Equal(t, tc.want, got)
		})
	}
}
