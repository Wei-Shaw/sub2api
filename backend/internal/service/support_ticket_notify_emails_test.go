//go:build unit

// Package service — support_ticket_notify_emails_test.go
//
// 覆盖 normalizeSupportTicketNotifyEmails 的 3 条核心分支：
//   - 空 email / 超长 email 丢弃（不报错）；
//   - 大小写 + trim 后去重（保留第一次出现，保留其 Disabled/Verified）；
//   - 超出 SupportTicketNotifyEmailsMaxCount 截断。
package service

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeSupportTicketNotifyEmails_Nil_ReturnsEmpty(t *testing.T) {
	// nil 输入 → []NotifyEmailEntry{}（非 nil），前端 v-for 依赖非 nil。
	out := normalizeSupportTicketNotifyEmails(nil)
	require.NotNil(t, out)
	require.Empty(t, out)
}

func TestNormalizeSupportTicketNotifyEmails_TrimsAndDropsEmpty(t *testing.T) {
	in := []NotifyEmailEntry{
		{Email: "   "},                // 全空白 → 丢弃
		{Email: ""},                   // 空 → 丢弃
		{Email: "  foo@example.com "}, // 保留（外部空白 trim，内部值保留原始 case）
	}
	out := normalizeSupportTicketNotifyEmails(in)
	require.Len(t, out, 1)
	require.Equal(t, "foo@example.com", out[0].Email)
}

func TestNormalizeSupportTicketNotifyEmails_DropsOverLength(t *testing.T) {
	// > SupportTicketNotifyEmailMaxLen 的 email 直接丢弃（宽松归一，不报错）。
	overlong := strings.Repeat("a", SupportTicketNotifyEmailMaxLen+1) + "@example.com"
	in := []NotifyEmailEntry{
		{Email: overlong},
		{Email: "ok@example.com"},
	}
	out := normalizeSupportTicketNotifyEmails(in)
	require.Len(t, out, 1)
	require.Equal(t, "ok@example.com", out[0].Email)
}

func TestNormalizeSupportTicketNotifyEmails_DedupsByLowerCase(t *testing.T) {
	// 去重规则：以 strings.ToLower(TrimSpace(Email)) 为 key，
	// 保留第一次出现（含其原始 case 与 Disabled/Verified 状态）。
	in := []NotifyEmailEntry{
		{Email: "Foo@Example.com", Disabled: false, Verified: true},
		{Email: "foo@example.com", Disabled: true, Verified: false}, // 被去重
		{Email: "bar@example.com", Disabled: false, Verified: false},
		{Email: " FOO@EXAMPLE.COM ", Disabled: false, Verified: false}, // 被去重（trim + case）
	}
	out := normalizeSupportTicketNotifyEmails(in)
	require.Len(t, out, 2)
	// 保留第一次出现的 email + case + 状态
	require.Equal(t, "Foo@Example.com", out[0].Email)
	require.False(t, out[0].Disabled)
	require.True(t, out[0].Verified)
	require.Equal(t, "bar@example.com", out[1].Email)
}

func TestNormalizeSupportTicketNotifyEmails_TruncatesAtMaxCount(t *testing.T) {
	// 超过 SupportTicketNotifyEmailsMaxCount 时直接截断（保留前 N 个）。
	in := make([]NotifyEmailEntry, 0, SupportTicketNotifyEmailsMaxCount+5)
	for i := 0; i < SupportTicketNotifyEmailsMaxCount+5; i++ {
		in = append(in, NotifyEmailEntry{Email: uniqueEmail(i)})
	}
	out := normalizeSupportTicketNotifyEmails(in)
	require.Len(t, out, SupportTicketNotifyEmailsMaxCount)
	// 截断保留的是前 N 个（顺序稳定），可校验第一个和最后一个。
	require.Equal(t, uniqueEmail(0), out[0].Email)
	require.Equal(t, uniqueEmail(SupportTicketNotifyEmailsMaxCount-1), out[len(out)-1].Email)
}

// uniqueEmail 生成 iN@example.com 形式的唯一邮箱，
// 用来撑起数量截断测试的输入。
func uniqueEmail(i int) string {
	// 避免依赖 strconv：直接拼接。
	return "u" + itoa(i) + "@example.com"
}

// itoa 是极简 int → string，避免 import strconv 拖入更多依赖。
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	buf := [20]byte{}
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
