package admin

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestEnrichShadowParentInfo(t *testing.T) {
	pid := int64(100)
	parent := &service.Account{
		ID: 100,
		Credentials: map[string]any{
			"email":                   "owner@example.com",
			"plan_type":               "pro",
			"subscription_expires_at": "2026-12-31T00:00:00Z",
			"chatgpt_account_id":      "acct_123",
		},
		Extra: map[string]any{
			"privacy_mode":                 "training_off",
			"codex_usage_updated_at":       "2026-08-25T08:00:00Z",
			"codex_5h_used_percent":        12.5,
			"codex_5h_reset_at":            "2026-08-25T12:00:00Z",
			"codex_5h_reset_after_seconds": 14400,
			"codex_5h_window_minutes":      300,
			"codex_7d_used_percent":        34.5,
			"codex_7d_reset_at":            "2026-08-31T08:00:00Z",
			"codex_7d_reset_after_seconds": 518400,
			"codex_7d_window_minutes":      10080,
		},
	}
	parents := map[int64]*service.Account{100: parent}

	linked := AccountWithConcurrency{Account: &dto.Account{
		ID:              200,
		ParentAccountID: &pid,
		QuotaDimension:  service.QuotaDimensionLinked,
		Extra: map[string]any{
			"codex_5h_used_percent": 99.0,
			"unrelated":             "preserved",
		},
	}}
	spark := AccountWithConcurrency{Account: &dto.Account{
		ID:              202,
		ParentAccountID: &pid,
		QuotaDimension:  service.QuotaDimensionSpark,
		Extra: map[string]any{
			"codex_5h_used_percent": 88.0,
		},
	}}
	normal := AccountWithConcurrency{Account: &dto.Account{ID: 1}}
	orphan := AccountWithConcurrency{Account: &dto.Account{ID: 201, ParentAccountID: ptrInt64(999)}}
	items := []AccountWithConcurrency{linked, spark, normal, orphan}

	enrichShadowParentInfo(items, parents)

	require.Equal(t, "owner@example.com", items[0].ParentEmail, "链接账号回填母账号邮箱")
	require.Equal(t, "pro", items[0].ParentPlanType)
	require.Equal(t, "training_off", items[0].ParentPrivacyMode)
	require.Equal(t, "2026-12-31T00:00:00Z", items[0].ParentSubscriptionExpiresAt)
	require.Equal(t, "acct_123", items[0].ParentChatGPTAccountID)
	require.Equal(t, "2026-08-25T08:00:00Z", items[0].Extra["codex_usage_updated_at"])
	require.Equal(t, 12.5, items[0].Extra["codex_5h_used_percent"])
	require.Equal(t, 34.5, items[0].Extra["codex_7d_used_percent"])
	require.Equal(t, "preserved", items[0].Extra["unrelated"])

	require.Equal(t, 88.0, items[1].Extra["codex_5h_used_percent"], "Spark 影子保留独立额度")
	require.Equal(t, "owner@example.com", items[1].ParentEmail, "Spark 影子仍回填母账号展示信息")
	require.Empty(t, items[2].ParentEmail, "非影子不回填")
	require.Empty(t, items[3].ParentEmail, "母账号缺失时优雅留空")
}

func ptrInt64(v int64) *int64 { return &v }
