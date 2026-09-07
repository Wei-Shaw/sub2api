package kiro

import (
	"testing"

	"github.com/stretchr/testify/require"
)

const sampleUsageLimits = `{
  "nextDateReset": 1789000000,
  "subscriptionInfo": {"subscriptionTitle": "KIRO PRO+", "overageCapability": "OVERAGE_CAPABLE"},
  "overageConfiguration": {"overageStatus": "ENABLED"},
  "usageBreakdownList": [
    {
      "resourceType": "AGENTIC_REQUEST",
      "currentUsage": 120,
      "currentUsageWithPrecision": 120.5,
      "usageLimit": 1000,
      "usageLimitWithPrecision": 1000.0,
      "nextDateReset": 1789000000,
      "overageCap": 50.0,
      "overageRate": 0.04,
      "currentOverages": 1.2,
      "bonuses": [
        {"currentUsage": 0, "usageLimit": 200, "status": "ACTIVE"},
        {"currentUsage": 50, "usageLimit": 50, "status": "EXPIRED"}
      ]
    },
    {"resourceType": "CODE_COMPLETION", "currentUsage": 5, "usageLimit": 100}
  ]
}`

func TestParseUsageLimits(t *testing.T) {
	t.Parallel()

	u, err := ParseUsageLimits([]byte(sampleUsageLimits))
	require.NoError(t, err)
	require.Equal(t, "KIRO PRO+", u.SubscriptionTitle)
	require.Equal(t, "ENABLED", u.OverageStatus)
	require.Equal(t, "OVERAGE_CAPABLE", u.OverageCapability)
	require.NotNil(t, u.NextDateReset)
	require.Len(t, u.Breakdowns, 2)
}

func TestUsageLimitsAgenticRequestPicksRightBreakdown(t *testing.T) {
	t.Parallel()

	u, err := ParseUsageLimits([]byte(sampleUsageLimits))
	require.NoError(t, err)

	b := u.AgenticRequest()
	require.NotNil(t, b)
	require.Equal(t, "AGENTIC_REQUEST", b.ResourceType)
	require.InDelta(t, 120.5, b.CurrentUsage, 1e-9, "有精确值时优先用精确值")
	require.InDelta(t, 1000.0, b.UsageLimit, 1e-9)
	require.NotNil(t, b.NextDateReset)
}

// TestEffectiveLimitIncludesActiveBonuses 覆盖一个易错点：
// 只看 usageLimit 会把有赠送额度的账号误判为已耗尽。
func TestEffectiveLimitIncludesActiveBonuses(t *testing.T) {
	t.Parallel()

	u, err := ParseUsageLimits([]byte(sampleUsageLimits))
	require.NoError(t, err)

	b := u.AgenticRequest()
	// 1000 基础 + 200 ACTIVE 赠送；EXPIRED 的 50 不计。
	require.InDelta(t, 1200.0, b.EffectiveLimit(), 1e-9)
}

func TestExhaustedUsesEffectiveLimit(t *testing.T) {
	t.Parallel()

	b := &UsageBreakdown{CurrentUsage: 1100, UsageLimit: 1000,
		Bonuses: []Bonus{{UsageLimit: 200, Status: "ACTIVE"}}}
	require.False(t, b.Exhausted(), "1100 < 1000+200，未耗尽")

	b.CurrentUsage = 1200
	require.True(t, b.Exhausted())
}

func TestUtilizationPercent(t *testing.T) {
	t.Parallel()

	b := &UsageBreakdown{CurrentUsage: 600, UsageLimit: 1000}
	require.InDelta(t, 60.0, b.UtilizationPercent(), 1e-9)

	// 零额度不得除零。
	require.Zero(t, (&UsageBreakdown{CurrentUsage: 5}).UtilizationPercent())
}

func TestParseUsageLimitsHandlesMissingFields(t *testing.T) {
	t.Parallel()

	u, err := ParseUsageLimits([]byte(`{}`))
	require.NoError(t, err)
	require.Empty(t, u.SubscriptionTitle)
	require.Nil(t, u.AgenticRequest())

	_, err = ParseUsageLimits([]byte(`not json`))
	require.Error(t, err)
}

// TestParseUsageLimitsFreeTrialInfoTwoStageParsing 覆盖 Task 19 评审记录
// 的 deferred minor：FreeTrialInfo 走两段式解析（json.Unmarshal 直接填
// Status/RawExpiry，随后 ParseUsageLimits 里再用 RawExpiry 算出
// ExpiryDate），给定实现从未测过这条路径——评审当时用独立 repro 验证过
// 两个 breakdown 各自的 FreeTrial 指针不重叠、没有别名 bug，但那次验证
// 没有固化成仓库里的回归测试，未来有人"简化"这段代码不会有任何测试信号
// 能抓到回归。这里补上：两条 breakdown 各自带不同的 freeTrialInfo，断言
// Status 与算出的 ExpiryDate 都各自正确，且两个指针不是同一个对象。
func TestParseUsageLimitsFreeTrialInfoTwoStageParsing(t *testing.T) {
	t.Parallel()

	const withTwoFreeTrials = `{
	  "usageBreakdownList": [
	    {
	      "resourceType": "AGENTIC_REQUEST",
	      "currentUsage": 1,
	      "usageLimit": 10,
	      "freeTrialInfo": {"freeTrialStatus": "ACTIVE", "freeTrialExpiry": 1789000000}
	    },
	    {
	      "resourceType": "CODE_COMPLETION",
	      "currentUsage": 2,
	      "usageLimit": 20,
	      "freeTrialInfo": {"freeTrialStatus": "EXPIRED", "freeTrialExpiry": 1700000000}
	    }
	  ]
	}`

	u, err := ParseUsageLimits([]byte(withTwoFreeTrials))
	require.NoError(t, err)
	require.Len(t, u.Breakdowns, 2)

	first := u.Breakdowns[0].FreeTrial
	second := u.Breakdowns[1].FreeTrial
	require.NotNil(t, first)
	require.NotNil(t, second)
	require.NotSame(t, first, second, "两条 breakdown 的 FreeTrial 不应该是同一个指针（别名 bug会导致两条记录互相覆盖）")

	require.Equal(t, "ACTIVE", first.Status)
	require.NotNil(t, first.ExpiryDate, "RawExpiry>0 时必须算出 ExpiryDate")
	require.EqualValues(t, 1789000000, first.ExpiryDate.Unix())

	require.Equal(t, "EXPIRED", second.Status)
	require.NotNil(t, second.ExpiryDate)
	require.EqualValues(t, 1700000000, second.ExpiryDate.Unix())
}

// TestParseUsageLimitsFreeTrialInfoMissingOrZeroExpiry 覆盖两条边界：
// 完全没有 freeTrialInfo（FreeTrial 应为 nil，不能 panic）；
// freeTrialExpiry<=0（unixPtr 的既有约定：不代表一个真实时间点，
// ExpiryDate 必须是 nil 而不是 unix 纪元时间）。
func TestParseUsageLimitsFreeTrialInfoMissingOrZeroExpiry(t *testing.T) {
	t.Parallel()

	u, err := ParseUsageLimits([]byte(`{"usageBreakdownList":[{"resourceType":"AGENTIC_REQUEST"}]}`))
	require.NoError(t, err)
	require.Len(t, u.Breakdowns, 1)
	require.Nil(t, u.Breakdowns[0].FreeTrial)

	u2, err := ParseUsageLimits([]byte(`{"usageBreakdownList":[{"resourceType":"AGENTIC_REQUEST","freeTrialInfo":{"freeTrialStatus":"NONE","freeTrialExpiry":0}}]}`))
	require.NoError(t, err)
	require.NotNil(t, u2.Breakdowns[0].FreeTrial)
	require.Nil(t, u2.Breakdowns[0].FreeTrial.ExpiryDate, "freeTrialExpiry<=0 不代表真实时间点，ExpiryDate 必须是 nil")
}

func TestBuildUsageLimitsURL(t *testing.T) {
	t.Parallel()

	got := BuildUsageLimitsURL("https://q.us-east-1.amazonaws.com", "arn:aws:x:::profile/A B")
	require.Contains(t, got, "/getUsageLimits?")
	require.Contains(t, got, "origin=AI_EDITOR")
	require.Contains(t, got, "resourceType=AGENTIC_REQUEST")
	require.Contains(t, got, "isEmailRequired=true")
	require.Contains(t, got, "profileArn=arn%3Aaws%3Ax%3A%3A%3Aprofile%2FA+B",
		"profileArn 必须 URL 编码")

	// 无 profileArn（API Key 账号）时不带该参数。
	require.NotContains(t, BuildUsageLimitsURL("https://q.us-east-1.amazonaws.com", ""), "profileArn=")
}
