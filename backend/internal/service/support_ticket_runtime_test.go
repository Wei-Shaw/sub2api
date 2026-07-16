//go:build unit

// Package service — support_ticket_runtime_test.go
//
// 覆盖 enabledNotifyEmails helper 的语义：SupportTicketRuntime.TicketNotifyEmails
// 是"已解禁"白名单（去除 disabled=true / 空 / 重复项）。
// 这个 helper 是 runtime 层的关键去污点：admin 保存的 disabled=true 项在存储里
// 保留（UI 需要），但通知路径必须过滤掉，否则违背 admin 意图。
package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEnabledNotifyEmails_Nil_ReturnsNil(t *testing.T) {
	require.Nil(t, enabledNotifyEmails(nil))
	require.Nil(t, enabledNotifyEmails([]NotifyEmailEntry{}))
}

func TestEnabledNotifyEmails_FiltersDisabled(t *testing.T) {
	// disabled=true 项必须被过滤：UI 保留但通知不发。
	got := enabledNotifyEmails([]NotifyEmailEntry{
		{Email: "a@example.com", Disabled: false},
		{Email: "b@example.com", Disabled: true},
		{Email: "c@example.com", Disabled: false},
	})
	require.Equal(t, []string{"a@example.com", "c@example.com"}, got)
}

func TestEnabledNotifyEmails_TrimAndLowercase(t *testing.T) {
	// runtime 层只关心邮件投递地址，做 trim + lowercase 让下游 dedup 更稳。
	got := enabledNotifyEmails([]NotifyEmailEntry{
		{Email: "  Foo@Example.COM  "},
		{Email: "BAR@example.com"},
	})
	require.Equal(t, []string{"foo@example.com", "bar@example.com"}, got)
}

func TestEnabledNotifyEmails_Dedups(t *testing.T) {
	// 去重规则：trim + lower 后同 key 只保留第一次出现。
	got := enabledNotifyEmails([]NotifyEmailEntry{
		{Email: "Foo@Example.com"},
		{Email: "foo@example.com"},
		{Email: "  FOO@EXAMPLE.COM  "},
		{Email: "bar@example.com"},
	})
	require.Equal(t, []string{"foo@example.com", "bar@example.com"}, got)
}

func TestEnabledNotifyEmails_AllDisabled_ReturnsNil(t *testing.T) {
	// 全部 disabled → 视为"无白名单"，返回 nil（下游会退化为发全体 admin）。
	got := enabledNotifyEmails([]NotifyEmailEntry{
		{Email: "a@example.com", Disabled: true},
		{Email: "b@example.com", Disabled: true},
	})
	require.Nil(t, got)
}

func TestEnabledNotifyEmails_BlankAfterTrim_Dropped(t *testing.T) {
	// 只有空白的 email 被 trim 后变空 → 丢弃，不算入结果。
	got := enabledNotifyEmails([]NotifyEmailEntry{
		{Email: "   "},
		{Email: "\t\n"},
		{Email: "keep@example.com"},
	})
	require.Equal(t, []string{"keep@example.com"}, got)
}
