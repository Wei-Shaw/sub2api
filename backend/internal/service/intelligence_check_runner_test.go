package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestIntelligenceCheckAccountDue(t *testing.T) {
	now := time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC)
	settings := IntelligenceCheckGlobalSettings{
		IntervalMinutes:             720, // 12h
		AccountRetryCount:           3,
		AccountRetryIntervalMinutes: 10,
	}

	tests := []struct {
		name string
		item intelligenceCheckDueAccount
		want bool
	}{
		{
			name: "从没跑过的账号立刻跑",
			item: intelligenceCheckDueAccount{},
			want: true,
		},
		{
			name: "还在排队的不叠加",
			item: intelligenceCheckDueAccount{last: &IntelligenceCheckRun{
				Status: IntelligenceCheckStatusQueued, CreatedAt: now.Add(-24 * time.Hour),
			}},
			want: false,
		},
		{
			name: "正在跑的不叠加",
			item: intelligenceCheckDueAccount{last: &IntelligenceCheckRun{
				Status: IntelligenceCheckStatusRunning, CreatedAt: now.Add(-24 * time.Hour),
			}},
			want: false,
		},
		{
			name: "成功过但没到间隔",
			item: intelligenceCheckDueAccount{last: &IntelligenceCheckRun{
				Status: IntelligenceCheckStatusCompleted, CreatedAt: now.Add(-6 * time.Hour),
			}},
			want: false,
		},
		{
			name: "成功过且已过间隔",
			item: intelligenceCheckDueAccount{last: &IntelligenceCheckRun{
				Status: IntelligenceCheckStatusCompleted, CreatedAt: now.Add(-13 * time.Hour),
			}},
			want: true,
		},
		{
			name: "账号级间隔覆盖全局（更短）",
			item: intelligenceCheckDueAccount{
				config: IntelligenceCheckAccountConfig{IntervalMinutes: 60},
				last: &IntelligenceCheckRun{
					Status: IntelligenceCheckStatusCompleted, CreatedAt: now.Add(-2 * time.Hour),
				},
			},
			want: true,
		},
		{
			name: "账号级间隔覆盖全局（更长）",
			item: intelligenceCheckDueAccount{
				config: IntelligenceCheckAccountConfig{IntervalMinutes: 1440},
				last: &IntelligenceCheckRun{
					Status: IntelligenceCheckStatusCompleted, CreatedAt: now.Add(-13 * time.Hour),
				},
			},
			want: false,
		},
		{
			name: "失败后按账号恢复间隔等待",
			item: intelligenceCheckDueAccount{last: &IntelligenceCheckRun{
				Status: IntelligenceCheckStatusFailed, Attempt: 1, CreatedAt: now.Add(-5 * time.Minute),
			}},
			want: false,
		},
		{
			name: "失败后过了账号恢复间隔就该重试",
			item: intelligenceCheckDueAccount{last: &IntelligenceCheckRun{
				Status: IntelligenceCheckStatusFailed, Attempt: 1, CreatedAt: now.Add(-11 * time.Minute),
			}},
			want: true,
		},
		{
			name: "恢复次数用尽后暂停该账号",
			item: intelligenceCheckDueAccount{last: &IntelligenceCheckRun{
				Status: IntelligenceCheckStatusFailed, Attempt: 4, CreatedAt: now.Add(-72 * time.Hour),
			}},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, intelligenceCheckAccountDue(now, tt.item, settings))
		})
	}
}

func TestIntelligenceCheckNextAttempt(t *testing.T) {
	tests := []struct {
		name string
		last *IntelligenceCheckRun
		want int
	}{
		{name: "从没跑过从 1 开始", last: nil, want: 1},
		{
			name: "上次成功说明是新的一轮",
			last: &IntelligenceCheckRun{Status: IntelligenceCheckStatusCompleted, Attempt: 7},
			want: 1,
		},
		{
			name: "上次失败说明在走账号自动恢复",
			last: &IntelligenceCheckRun{Status: IntelligenceCheckStatusFailed, Attempt: 1},
			want: 2,
		},
		{
			name: "历史记录 attempt 缺失也能推进",
			last: &IntelligenceCheckRun{Status: IntelligenceCheckStatusFailed, Attempt: 0},
			want: 2,
		},
		{
			name: "排队中的记录不影响新序号计算",
			last: &IntelligenceCheckRun{Status: IntelligenceCheckStatusQueued, Attempt: 5},
			want: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, intelligenceCheckNextAttempt(tt.last))
		})
	}
}

// TestIntelligenceCheckGlobalSettingsRunnable 锁定「没有模型就不发请求」这条安全底线：
// 总开关开着但模型为空时必须判定为不可运行，否则调度器会拿空模型去打上游。
func TestIntelligenceCheckGlobalSettingsRunnable(t *testing.T) {
	tests := []struct {
		name     string
		settings IntelligenceCheckGlobalSettings
		want     bool
	}{
		{
			name:     "默认设置不可运行（开关关且无模型）",
			settings: DefaultIntelligenceCheckGlobalSettings(),
			want:     false,
		},
		{
			name:     "只开开关但没有模型仍然不可运行",
			settings: IntelligenceCheckGlobalSettings{Enabled: true},
			want:     false,
		},
		{
			name:     "只配模型但没开开关也不可运行",
			settings: IntelligenceCheckGlobalSettings{ModelID: "some-model"},
			want:     false,
		},
		{
			name:     "只有空白字符的模型视为未配置",
			settings: IntelligenceCheckGlobalSettings{Enabled: true, ModelID: "   "},
			want:     false,
		},
		{
			name:     "开关加模型才可运行",
			settings: IntelligenceCheckGlobalSettings{Enabled: true, ModelID: "some-model"},
			want:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, tt.settings.Runnable())
		})
	}
}
