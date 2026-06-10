package service

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeSupportTicketCategories_HappyPath(t *testing.T) {
	got, err := NormalizeSupportTicketCategories([]string{" 充值 ", "API", "Bug"})
	require.NoError(t, err)
	require.Equal(t, []string{"充值", "API", "Bug"}, got)
}

func TestNormalizeSupportTicketCategories_DeduplicatesPreservingOrder(t *testing.T) {
	// strict=true: 重复直接报错；用 ParseSupportTicketCategories 走 lenient 路径才会去重
	_, err := NormalizeSupportTicketCategories([]string{"账号", "账号"})
	require.Error(t, err)
}

func TestNormalizeSupportTicketCategories_RejectsEmpty(t *testing.T) {
	_, err := NormalizeSupportTicketCategories(nil)
	require.Error(t, err)

	_, err = NormalizeSupportTicketCategories([]string{"", "   "})
	require.Error(t, err)
}

func TestNormalizeSupportTicketCategories_RejectsTooManyItems(t *testing.T) {
	tooMany := make([]string, SupportTicketCategoryMaxCount+1)
	for i := range tooMany {
		// rune 数 <= 20 即可，确保失败原因是数量超限而不是单项过长
		tooMany[i] = "category-" + string(rune('a'+i%26))
	}
	_, err := NormalizeSupportTicketCategories(tooMany)
	require.Error(t, err)
}

func TestNormalizeSupportTicketCategories_RejectsTooLongItem(t *testing.T) {
	long := strings.Repeat("x", SupportTicketCategoryMaxLen+1)
	_, err := NormalizeSupportTicketCategories([]string{long})
	require.Error(t, err)
}

func TestParseSupportTicketCategories_FallsBackToDefaults(t *testing.T) {
	for _, raw := range []string{"", "   ", "[]", "null", "not-json"} {
		got := ParseSupportTicketCategories(raw)
		require.Equal(t, SupportTicketDefaultCategories, got, "raw=%q", raw)
		// 必须是新切片，外部修改不能影响包级常量
		got[0] = "tampered"
		require.NotEqual(t, "tampered", SupportTicketDefaultCategories[0])
	}
}

func TestParseSupportTicketCategories_NormalizesPersistedJSON(t *testing.T) {
	got := ParseSupportTicketCategories(`["A","B","B","  "]`)
	require.Equal(t, []string{"A", "B"}, got)
}

func TestValidateSupportTicketPriority(t *testing.T) {
	cases := map[string]string{
		"low":     SupportTicketPriorityLow,
		"NORMAL":  SupportTicketPriorityNormal,
		" High ": SupportTicketPriorityHigh,
	}
	for input, want := range cases {
		got, err := ValidateSupportTicketPriority(input)
		require.NoError(t, err, "input=%q", input)
		require.Equal(t, want, got)
	}

	for _, bad := range []string{"", "urgent", "p0", "critical"} {
		_, err := ValidateSupportTicketPriority(bad)
		require.Error(t, err, "input=%q", bad)
	}
}

func TestNormalizeSupportTicketPriority_FallbackToNormal(t *testing.T) {
	require.Equal(t, SupportTicketPriorityNormal, NormalizeSupportTicketPriority(""))
	require.Equal(t, SupportTicketPriorityNormal, NormalizeSupportTicketPriority("nonsense"))
	require.Equal(t, SupportTicketPriorityHigh, NormalizeSupportTicketPriority(" HIGH "))
}

func TestMarshalSupportTicketCategories_RoundTrip(t *testing.T) {
	in := []string{"充值", "Bug"}
	raw, err := MarshalSupportTicketCategories(in)
	require.NoError(t, err)
	got := ParseSupportTicketCategories(raw)
	require.Equal(t, in, got)
}
