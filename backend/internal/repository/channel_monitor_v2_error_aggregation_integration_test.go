//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

// 回归测试：composite 平台解析修复（49752060f）曾引入单参数 NULLIF
// （NULLIF(TRIM(a.platform))），NULLIF 需要两个参数，导致
// channelMonitorV2ErrorAggregationSQL 解析失败，线上聚合每分钟报
// `pq: syntax error at or near ")"`，渠道监控数据停更。
//
// 该测试在真库上执行 RecomputeRange，任何 SQL 层面的语法/解析问题都会在 CI 暴露，
// 并验证 composite 分组的错误事实按账号具体平台聚合（而不是永远健康的 'composite'）。
func TestChannelMonitorV2RecomputeErrorAggregationCompositePlatform(t *testing.T) {
	ctx := context.Background()
	repo := NewChannelMonitorV2Repository(integrationDB)

	compositeGroup := mustCreateGroup(t, integrationEntClient, &service.Group{
		Name: "cmv2-composite-regress", Platform: service.PlatformComposite,
	})
	t.Cleanup(func() { _ = integrationEntClient.Group.DeleteOneID(compositeGroup.ID).Exec(ctx) })

	account := mustCreateAccount(t, integrationEntClient, &service.Account{
		Name: "cmv2-composite-acc", Platform: domain.PlatformOpenAI, Type: service.AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "sk-cmv2-regress"},
	})
	t.Cleanup(func() { _ = integrationEntClient.Account.DeleteOneID(account.ID).Exec(ctx) })

	start := time.Now().UTC().Truncate(time.Minute)
	end := start.Add(time.Minute)

	// composite 分组的前置路由失败：记录平台是 'composite'，但聚合时必须落到账号平台。
	var compositeErrID int64
	err := integrationDB.QueryRowContext(ctx, `
		INSERT INTO ops_error_logs (error_phase, error_type, severity, status_code, group_id, account_id, platform, request_id, created_at)
		VALUES ('upstream', 'upstream_error', 'error', 429, $1, $2, 'composite', 'cmv2-regress-req', $3)
		RETURNING id`,
		compositeGroup.ID, account.ID, start.Add(30*time.Second),
	).Scan(&compositeErrID)
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = integrationDB.ExecContext(ctx, `DELETE FROM ops_error_logs WHERE id = $1`, compositeErrID) })

	// 对照组：非 composite 分组错误保持记录平台。
	var plainErrID int64
	err = integrationDB.QueryRowContext(ctx, `
		INSERT INTO ops_error_logs (error_phase, error_type, severity, status_code, platform, request_id, created_at)
		VALUES ('upstream', 'upstream_error', 'error', 500, 'openai', 'cmv2-regress-req2', $1)
		RETURNING id`,
		start.Add(30*time.Second),
	).Scan(&plainErrID)
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = integrationDB.ExecContext(ctx, `DELETE FROM ops_error_logs WHERE id = $1`, plainErrID) })

	// 核心回归点：这条 SQL 曾以 `syntax error at or near ")"` 失败。
	require.NoError(t, repo.RecomputeRange(ctx, start, end))

	var resolved int64
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
		SELECT count(*) FROM channel_monitor_v2_error_metrics_1m
		WHERE group_id = $1 AND platform = $2 AND bucket_start >= $3 AND bucket_start < $4`,
		compositeGroup.ID, "openai", start, end,
	).Scan(&resolved))
	require.GreaterOrEqual(t, resolved, int64(1), "composite group errors must aggregate under the concrete account platform")

	var compositeKey int64
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
		SELECT count(*) FROM channel_monitor_v2_error_metrics_1m
		WHERE platform = 'composite' AND group_id = $1 AND bucket_start >= $2 AND bucket_start < $3`,
		compositeGroup.ID, start, end,
	).Scan(&compositeKey))
	require.Zero(t, compositeKey, "'composite' must never surface as an aggregated platform key")

	var control int64
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
		SELECT count(*) FROM channel_monitor_v2_error_metrics_1m
		WHERE group_id = 0 AND platform = 'openai' AND bucket_start >= $1 AND bucket_start < $2`,
		start, end,
	).Scan(&control))
	require.GreaterOrEqual(t, control, int64(1), "non-composite errors keep their recorded platform")
}
