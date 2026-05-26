//go:build unit

package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// ============ 临时限流单元测试 ============

// TestMatchTempUnschedKeyword 测试关键词匹配函数
func TestMatchTempUnschedKeyword(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		keywords []string
		want     string
	}{
		{
			name:     "match_first",
			body:     "server is overloaded",
			keywords: []string{"overloaded", "capacity"},
			want:     "overloaded",
		},
		{
			name:     "match_second",
			body:     "no capacity available",
			keywords: []string{"overloaded", "capacity"},
			want:     "capacity",
		},
		{
			name:     "no_match",
			body:     "internal error",
			keywords: []string{"overloaded", "capacity"},
			want:     "",
		},
		{
			name:     "empty_body",
			body:     "",
			keywords: []string{"overloaded"},
			want:     "",
		},
		{
			name:     "empty_keywords",
			body:     "server is overloaded",
			keywords: []string{},
			want:     "",
		},
		{
			name:     "whitespace_keyword",
			body:     "server is overloaded",
			keywords: []string{"  ", "overloaded"},
			want:     "overloaded",
		},
		{
			// matchTempUnschedKeyword 期望 body 已经是小写的
			// 所以要测试大小写不敏感匹配，需要传入小写的 body
			name:     "case_insensitive_body_lowered",
			body:     "server is overloaded", // body 已经是小写
			keywords: []string{"OVERLOADED"}, // keyword 会被转为小写比较
			want:     "OVERLOADED",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchTempUnschedKeyword(tt.body, tt.keywords)
			require.Equal(t, tt.want, got)
		})
	}
}

// TestAccountIsSchedulable_TempUnschedulable 测试临时限流账号不可调度
func TestAccountIsSchedulable_TempUnschedulable(t *testing.T) {
	future := time.Now().Add(10 * time.Minute)
	past := time.Now().Add(-10 * time.Minute)

	tests := []struct {
		name    string
		account *Account
		want    bool
	}{
		{
			name: "temp_unschedulable_active",
			account: &Account{
				Status:                 StatusActive,
				Schedulable:            true,
				TempUnschedulableUntil: &future,
			},
			want: false,
		},
		{
			name: "temp_unschedulable_expired",
			account: &Account{
				Status:                 StatusActive,
				Schedulable:            true,
				TempUnschedulableUntil: &past,
			},
			want: true,
		},
		{
			name: "no_temp_unschedulable",
			account: &Account{
				Status:                 StatusActive,
				Schedulable:            true,
				TempUnschedulableUntil: nil,
			},
			want: true,
		},
		{
			name: "temp_unschedulable_with_rate_limit",
			account: &Account{
				Status:                 StatusActive,
				Schedulable:            true,
				TempUnschedulableUntil: &future,
				RateLimitResetAt:       &past, // 过期的限流不影响
			},
			want: false, // 临时限流生效
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.account.IsSchedulable()
			require.Equal(t, tt.want, got)
		})
	}
}

// TestAccount_IsTempUnschedulableEnabled 测试临时限流开关
func TestAccount_IsTempUnschedulableEnabled(t *testing.T) {
	tests := []struct {
		name    string
		account *Account
		want    bool
	}{
		{
			name: "enabled",
			account: &Account{
				Credentials: map[string]any{
					"temp_unschedulable_enabled": true,
				},
			},
			want: true,
		},
		{
			name: "disabled",
			account: &Account{
				Credentials: map[string]any{
					"temp_unschedulable_enabled": false,
				},
			},
			want: false,
		},
		{
			name: "not_set",
			account: &Account{
				Credentials: map[string]any{},
			},
			want: false,
		},
		{
			name:    "nil_credentials",
			account: &Account{},
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.account.IsTempUnschedulableEnabled()
			require.Equal(t, tt.want, got)
		})
	}
}

// TestAccount_GetTempUnschedulableRules 测试获取临时限流规则
func TestAccount_GetTempUnschedulableRules(t *testing.T) {
	tests := []struct {
		name      string
		account   *Account
		wantCount int
	}{
		{
			name: "has_rules",
			account: &Account{
				Credentials: map[string]any{
					"temp_unschedulable_rules": []any{
						map[string]any{
							"error_code":       float64(503),
							"keywords":         []any{"overloaded"},
							"duration_minutes": float64(5),
						},
						map[string]any{
							"error_code":       float64(500),
							"keywords":         []any{"internal"},
							"duration_minutes": float64(10),
						},
					},
				},
			},
			wantCount: 2,
		},
		{
			name: "empty_rules",
			account: &Account{
				Credentials: map[string]any{
					"temp_unschedulable_rules": []any{},
				},
			},
			wantCount: 0,
		},
		{
			name: "no_rules",
			account: &Account{
				Credentials: map[string]any{},
			},
			wantCount: 0,
		},
		{
			name:      "nil_credentials",
			account:   &Account{},
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rules := tt.account.GetTempUnschedulableRules()
			require.Len(t, rules, tt.wantCount)
		})
	}
}

// TestTempUnschedulableRule_Parse 测试规则解析
func TestTempUnschedulableRule_Parse(t *testing.T) {
	account := &Account{
		Credentials: map[string]any{
			"temp_unschedulable_rules": []any{
				map[string]any{
					"error_code":       float64(503),
					"keywords":         []any{"overloaded", "capacity"},
					"duration_minutes": float64(5),
				},
			},
		},
	}

	rules := account.GetTempUnschedulableRules()
	require.Len(t, rules, 1)

	rule := rules[0]
	require.Equal(t, 503, rule.ErrorCode)
	require.Equal(t, []string{"overloaded", "capacity"}, rule.Keywords)
	require.Equal(t, 5, rule.DurationMinutes)
}

// TestTruncateTempUnschedMessage 测试消息截断
func TestTruncateTempUnschedMessage(t *testing.T) {
	tests := []struct {
		name     string
		body     []byte
		maxBytes int
		want     string
	}{
		{
			name:     "short_message",
			body:     []byte("short"),
			maxBytes: 100,
			want:     "short",
		},
		{
			// 截断后会 TrimSpace，所以末尾的空格会被移除
			name:     "truncate_long_message",
			body:     []byte("this is a very long message that needs to be truncated"),
			maxBytes: 20,
			want:     "this is a very long", // 截断后 TrimSpace
		},
		{
			name:     "empty_body",
			body:     []byte{},
			maxBytes: 100,
			want:     "",
		},
		{
			name:     "zero_max_bytes",
			body:     []byte("test"),
			maxBytes: 0,
			want:     "",
		},
		{
			name:     "whitespace_trimmed",
			body:     []byte("  test  "),
			maxBytes: 100,
			want:     "test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateTempUnschedMessage(tt.body, tt.maxBytes)
			require.Equal(t, tt.want, got)
		})
	}
}

// TestTempUnschedState 测试临时限流状态结构
func TestTempUnschedState(t *testing.T) {
	now := time.Now()
	until := now.Add(5 * time.Minute)

	state := &TempUnschedState{
		UntilUnix:       until.Unix(),
		TriggeredAtUnix: now.Unix(),
		StatusCode:      503,
		MatchedKeyword:  "overloaded",
		RuleIndex:       0,
		ErrorMessage:    "Server is overloaded",
	}

	require.Equal(t, 503, state.StatusCode)
	require.Equal(t, "overloaded", state.MatchedKeyword)
	require.Equal(t, 0, state.RuleIndex)

	// 验证时间戳
	require.Equal(t, until.Unix(), state.UntilUnix)
	require.Equal(t, now.Unix(), state.TriggeredAtUnix)
}

// TestAccount_TempUnschedulableUntil 测试临时限流时间字段
func TestAccount_TempUnschedulableUntil(t *testing.T) {
	future := time.Now().Add(10 * time.Minute)
	past := time.Now().Add(-10 * time.Minute)

	tests := []struct {
		name        string
		account     *Account
		schedulable bool
	}{
		{
			name: "active_temp_unsched_not_schedulable",
			account: &Account{
				Status:                 StatusActive,
				Schedulable:            true,
				TempUnschedulableUntil: &future,
			},
			schedulable: false,
		},
		{
			name: "expired_temp_unsched_is_schedulable",
			account: &Account{
				Status:                 StatusActive,
				Schedulable:            true,
				TempUnschedulableUntil: &past,
			},
			schedulable: true,
		},
		{
			name: "nil_temp_unsched_is_schedulable",
			account: &Account{
				Status:                 StatusActive,
				Schedulable:            true,
				TempUnschedulableUntil: nil,
			},
			schedulable: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.account.IsSchedulable()
			require.Equal(t, tt.schedulable, got)
		})
	}
}
