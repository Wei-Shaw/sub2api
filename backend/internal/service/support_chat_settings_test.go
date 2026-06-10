package service

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// ====================================================================
// excluded_routes
// ====================================================================

func TestNormalizeSupportChatExcludedRoutes_HappyPath(t *testing.T) {
	got, err := NormalizeSupportChatExcludedRoutes([]string{" /login ", "/admin/*", "/forgot-password"})
	require.NoError(t, err)
	require.Equal(t, []string{"/login", "/admin/*", "/forgot-password"}, got)
}

func TestNormalizeSupportChatExcludedRoutes_AllowsEmpty(t *testing.T) {
	// 空数组合法（admin 可清空）
	got, err := NormalizeSupportChatExcludedRoutes(nil)
	require.NoError(t, err)
	require.Equal(t, []string{}, got)

	got, err = NormalizeSupportChatExcludedRoutes([]string{})
	require.NoError(t, err)
	require.Equal(t, []string{}, got)
}

func TestNormalizeSupportChatExcludedRoutes_RejectsBlankItem(t *testing.T) {
	_, err := NormalizeSupportChatExcludedRoutes([]string{"/login", "   "})
	require.Error(t, err)
}

func TestNormalizeSupportChatExcludedRoutes_RejectsMissingLeadingSlash(t *testing.T) {
	_, err := NormalizeSupportChatExcludedRoutes([]string{"login"})
	require.Error(t, err)
}

func TestNormalizeSupportChatExcludedRoutes_RejectsDuplicate(t *testing.T) {
	_, err := NormalizeSupportChatExcludedRoutes([]string{"/login", "/login"})
	require.Error(t, err)
}

func TestNormalizeSupportChatExcludedRoutes_RejectsTooLong(t *testing.T) {
	long := "/" + strings.Repeat("x", SupportChatExcludedRouteMaxLen)
	_, err := NormalizeSupportChatExcludedRoutes([]string{long})
	require.Error(t, err)
}

func TestNormalizeSupportChatExcludedRoutes_RejectsTooManyItems(t *testing.T) {
	tooMany := make([]string, SupportChatExcludedRoutesMaxItem+1)
	for i := range tooMany {
		// 用 i 保证唯一性，避免重复触发其它错误
		tooMany[i] = "/r" + strings.Repeat("a", 1) + "_" + string(rune('A'+i%26)) + string(rune('a'+(i/26)%26))
	}
	_, err := NormalizeSupportChatExcludedRoutes(tooMany)
	require.Error(t, err)
}

func TestParseSupportChatExcludedRoutes_FallsBackOnInvalidJSON(t *testing.T) {
	for _, raw := range []string{"", "   ", "null", "not-json", "{\"x\":1}"} {
		got := ParseSupportChatExcludedRoutes(raw)
		require.Equal(t, SupportChatDefaultExcludedRoutes, got, "raw=%q", raw)
		// 必须是新切片，外部修改不能影响包级常量
		got[0] = "tampered"
		require.NotEqual(t, "tampered", SupportChatDefaultExcludedRoutes[0])
	}
}

func TestParseSupportChatExcludedRoutes_EmptyArrayFallsBack(t *testing.T) {
	// "[]" 视为未配置，回退到默认
	got := ParseSupportChatExcludedRoutes("[]")
	require.Equal(t, SupportChatDefaultExcludedRoutes, got)
}

func TestParseSupportChatExcludedRoutes_LenientCleansBadEntries(t *testing.T) {
	// 已持久化数据：含空项、未带 / 前缀、重复——lenient 模式应过滤而非报错
	got := ParseSupportChatExcludedRoutes(`["/login", "  ", "broken", "/login", "/admin/*"]`)
	require.Equal(t, []string{"/login", "/admin/*"}, got)
}

func TestMarshalSupportChatExcludedRoutes_RoundTrip(t *testing.T) {
	in := []string{"/login", "/register", "/admin/*"}
	raw, err := MarshalSupportChatExcludedRoutes(in)
	require.NoError(t, err)
	got := ParseSupportChatExcludedRoutes(raw)
	require.Equal(t, in, got)
}

func TestMarshalSupportChatExcludedRoutes_NilBecomesEmptyArray(t *testing.T) {
	raw, err := MarshalSupportChatExcludedRoutes(nil)
	require.NoError(t, err)
	require.Equal(t, "[]", raw)
}

// ====================================================================
// faqs
// ====================================================================

func TestNormalizeSupportChatFAQs_HappyPath(t *testing.T) {
	in := []SupportChatFAQ{
		{Question: " 如何重置密码 ", Answer: " 点击登录页的「忘记密码」 ", SortOrder: 10, Enabled: true},
		{Question: "如何充值", Answer: "前往充值页", SortOrder: 20, Enabled: false},
	}
	got, err := NormalizeSupportChatFAQs(in)
	require.NoError(t, err)
	require.Len(t, got, 2)
	require.Equal(t, "如何重置密码", got[0].Question)
	require.Equal(t, "点击登录页的「忘记密码」", got[0].Answer)
	require.Equal(t, 10, got[0].SortOrder)
	require.True(t, got[0].Enabled)
	require.Equal(t, "如何充值", got[1].Question)
	require.False(t, got[1].Enabled)
}

func TestNormalizeSupportChatFAQs_EmptyAllowed(t *testing.T) {
	got, err := NormalizeSupportChatFAQs(nil)
	require.NoError(t, err)
	require.Equal(t, []SupportChatFAQ{}, got)

	got, err = NormalizeSupportChatFAQs([]SupportChatFAQ{})
	require.NoError(t, err)
	require.Equal(t, []SupportChatFAQ{}, got)
}

func TestNormalizeSupportChatFAQs_RejectsTooManyItems(t *testing.T) {
	tooMany := make([]SupportChatFAQ, SupportChatFAQMaxItems+1)
	for i := range tooMany {
		tooMany[i] = SupportChatFAQ{Question: "q", Answer: "a"}
	}
	_, err := NormalizeSupportChatFAQs(tooMany)
	require.Error(t, err)
}

func TestNormalizeSupportChatFAQs_RejectsBlankQuestion(t *testing.T) {
	_, err := NormalizeSupportChatFAQs([]SupportChatFAQ{{Question: "  ", Answer: "ok"}})
	require.Error(t, err)
}

func TestNormalizeSupportChatFAQs_RejectsBlankAnswer(t *testing.T) {
	_, err := NormalizeSupportChatFAQs([]SupportChatFAQ{{Question: "ok", Answer: "  "}})
	require.Error(t, err)
}

func TestNormalizeSupportChatFAQs_RejectsTooLongQuestion(t *testing.T) {
	long := strings.Repeat("Q", SupportChatFAQQuestionMaxLen+1)
	_, err := NormalizeSupportChatFAQs([]SupportChatFAQ{{Question: long, Answer: "ok"}})
	require.Error(t, err)
}

func TestNormalizeSupportChatFAQs_RejectsTooLongAnswer(t *testing.T) {
	long := strings.Repeat("A", SupportChatFAQAnswerMaxLen+1)
	_, err := NormalizeSupportChatFAQs([]SupportChatFAQ{{Question: "ok", Answer: long}})
	require.Error(t, err)
}

func TestParseSupportChatFAQs_FallsBackOnInvalid(t *testing.T) {
	for _, raw := range []string{"", "   ", "null", "[]", "not-json", `{"x":1}`} {
		got := ParseSupportChatFAQs(raw)
		require.Equal(t, []SupportChatFAQ{}, got, "raw=%q", raw)
	}
}

func TestMarshalSupportChatFAQs_RoundTrip(t *testing.T) {
	in := []SupportChatFAQ{
		{Question: "Q1", Answer: "A1", SortOrder: 1, Enabled: true},
		{Question: "Q2", Answer: "A2", SortOrder: 2, Enabled: false},
	}
	raw, err := MarshalSupportChatFAQs(in)
	require.NoError(t, err)
	got := ParseSupportChatFAQs(raw)
	require.Equal(t, in, got)
}

func TestMarshalSupportChatFAQs_NilBecomesEmptyArray(t *testing.T) {
	raw, err := MarshalSupportChatFAQs(nil)
	require.NoError(t, err)
	require.Equal(t, "[]", raw)
}

// ====================================================================
// max_turns / max_request_tokens / rate_limit clamp & validate
// ====================================================================

func TestClampSupportChatMaxTurns(t *testing.T) {
	require.Equal(t, SupportChatMaxTurnsDefault, ClampSupportChatMaxTurns(0))
	require.Equal(t, SupportChatMaxTurnsDefault, ClampSupportChatMaxTurns(-5))
	require.Equal(t, SupportChatMaxTurnsMin, ClampSupportChatMaxTurns(SupportChatMaxTurnsMin))
	require.Equal(t, SupportChatMaxTurnsMax, ClampSupportChatMaxTurns(SupportChatMaxTurnsMax))
	require.Equal(t, SupportChatMaxTurnsMax, ClampSupportChatMaxTurns(SupportChatMaxTurnsMax+10))
	require.Equal(t, 7, ClampSupportChatMaxTurns(7))
}

func TestValidateSupportChatMaxTurns(t *testing.T) {
	for _, ok := range []int{SupportChatMaxTurnsMin, SupportChatMaxTurnsMax, 5} {
		got, err := ValidateSupportChatMaxTurns(ok)
		require.NoError(t, err)
		require.Equal(t, ok, got)
	}
	for _, bad := range []int{0, -1, SupportChatMaxTurnsMin - 1, SupportChatMaxTurnsMax + 1, 9999} {
		_, err := ValidateSupportChatMaxTurns(bad)
		require.Error(t, err, "v=%d", bad)
	}
}

func TestClampSupportChatMaxRequestTokens(t *testing.T) {
	require.Equal(t, SupportChatMaxRequestTokensDef, ClampSupportChatMaxRequestTokens(0))
	require.Equal(t, SupportChatMaxRequestTokensDef, ClampSupportChatMaxRequestTokens(-1))
	require.Equal(t, SupportChatMaxRequestTokensMin, ClampSupportChatMaxRequestTokens(SupportChatMaxRequestTokensMin))
	require.Equal(t, SupportChatMaxRequestTokensMax, ClampSupportChatMaxRequestTokens(SupportChatMaxRequestTokensMax))
	require.Equal(t, SupportChatMaxRequestTokensMax, ClampSupportChatMaxRequestTokens(SupportChatMaxRequestTokensMax+1))
	require.Equal(t, 20000, ClampSupportChatMaxRequestTokens(20000))
}

func TestValidateSupportChatMaxRequestTokens(t *testing.T) {
	got, err := ValidateSupportChatMaxRequestTokens(SupportChatMaxRequestTokensDef)
	require.NoError(t, err)
	require.Equal(t, SupportChatMaxRequestTokensDef, got)

	for _, bad := range []int{0, SupportChatMaxRequestTokensMin - 1, SupportChatMaxRequestTokensMax + 1} {
		_, err := ValidateSupportChatMaxRequestTokens(bad)
		require.Error(t, err, "v=%d", bad)
	}
}

func TestClampSupportChatRateLimit(t *testing.T) {
	const fb = 60

	// 0 / 负数 → fallback
	require.Equal(t, fb, ClampSupportChatRateLimit(0, fb))
	require.Equal(t, fb, ClampSupportChatRateLimit(-100, fb))

	// 边界值不变
	require.Equal(t, SupportChatRateLimitMin, ClampSupportChatRateLimit(SupportChatRateLimitMin, fb))
	require.Equal(t, SupportChatRateLimitMax, ClampSupportChatRateLimit(SupportChatRateLimitMax, fb))

	// 超上限 → 上限
	require.Equal(t, SupportChatRateLimitMax, ClampSupportChatRateLimit(SupportChatRateLimitMax+1, fb))

	// 合法值原样返回
	require.Equal(t, 50, ClampSupportChatRateLimit(50, fb))
}

func TestValidateSupportChatRateLimit(t *testing.T) {
	got, err := ValidateSupportChatRateLimit(60, "rl_user_per_day")
	require.NoError(t, err)
	require.Equal(t, 60, got)

	for _, bad := range []int{0, -1, SupportChatRateLimitMin - 1, SupportChatRateLimitMax + 1} {
		_, err := ValidateSupportChatRateLimit(bad, "rl_user_per_day")
		require.Error(t, err, "v=%d", bad)
		require.Contains(t, err.Error(), "rl_user_per_day")
	}
}
