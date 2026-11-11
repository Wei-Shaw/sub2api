package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAccount_ShouldRecordFailedStreamUsage(t *testing.T) {
	t.Run("开启", func(t *testing.T) {
		account := &Account{
			Extra: map[string]any{"record_failed_stream_usage": true},
		}
		require.True(t, account.ShouldRecordFailedStreamUsage())
	})

	t.Run("关闭", func(t *testing.T) {
		account := &Account{
			Extra: map[string]any{"record_failed_stream_usage": false},
		}
		require.False(t, account.ShouldRecordFailedStreamUsage())
	})

	t.Run("字段缺失默认关闭", func(t *testing.T) {
		account := &Account{Extra: map[string]any{}}
		require.False(t, account.ShouldRecordFailedStreamUsage())
	})

	t.Run("Extra 为 nil 默认关闭", func(t *testing.T) {
		account := &Account{}
		require.False(t, account.ShouldRecordFailedStreamUsage())
	})

	t.Run("字段类型非法默认关闭", func(t *testing.T) {
		account := &Account{
			Extra: map[string]any{"record_failed_stream_usage": "true"},
		}
		require.False(t, account.ShouldRecordFailedStreamUsage())
	})

	t.Run("空账号不 panic", func(t *testing.T) {
		var account *Account
		require.False(t, account.ShouldRecordFailedStreamUsage())
	})
}

// TestPartialStreamUsageResult_RejectsFailoverError 锁定既有不变式：
// 携带 failover 错误时 partialStreamUsageResult 必须返回 nil，否则重试成功后
// 会与成功结果重复计费。本改动的补记只发生在重试耗尽路径，依赖该不变式成立。
func TestPartialStreamUsageResult_RejectsFailoverError(t *testing.T) {
	streamResult := &streamingResult{
		usage: &ClaudeUsage{InputTokens: 50, OutputTokens: 3163, CacheReadInputTokens: 195600},
	}
	got := partialStreamUsageResult(nil, nil, streamResult, "claude-opus-5-5", "claude-opus-5-5", time.Time{}, &UpstreamFailoverError{StatusCode: 502})
	require.Nil(t, got)
}

// TestPartialStreamUsageResult_SkipsWhenNoObservedTokens 确认没有观测到 token 时不产生部分结果。
func TestPartialStreamUsageResult_SkipsWhenNoObservedTokens(t *testing.T) {
	streamResult := &streamingResult{usage: &ClaudeUsage{}}
	got := partialStreamUsageResult(nil, nil, streamResult, "claude-opus-5-5", "claude-opus-5-5", time.Time{}, &UpstreamFailoverError{StatusCode: 502})
	require.Nil(t, got)
	require.Nil(t, partialStreamUsageResult(nil, nil, nil, "m", "m", time.Time{}, &UpstreamFailoverError{}))
}
