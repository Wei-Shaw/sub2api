//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

// 本文件验证「兜底补账」那条 UPDATE 的闸门与封顶行为。
//
// 刻意用真库跑而不是 sqlmock：这条语句的全部要点都在 SQL 谓词与 LIMIT 上，
// mock 只能断言「SQL 文本长什么样」，验不了「它到底选中了哪几行」——
// 而「选中哪几行」正是这个功能唯一会出错的地方（漏选=账单永久缺口，
// 多选=把还在重试的记录进度清掉、甚至误碰已取到的记录）。

// requeueFixture 是一次测试所需的三个父行。usage_logs 对它们都有 NOT NULL
// 外键约束，所以哪怕只关心「重试计数」这一列，也得先把链路铺好。
type requeueFixture struct {
	userID    int64
	apiKeyID  int64
	accountID int64
}

// newRequeueFixture 建好 user / account / api_key 三行，并注册逆序清理。
//
// 用 integrationDB 直接落库而不是 ent 事务：被测仓储持有的是连接池，
// 看不见未提交的事务。代价是这些行真会写进库，所以清理必须自己负责。
func newRequeueFixture(t *testing.T) requeueFixture {
	t.Helper()
	client := testEntClient(t)

	user := mustCreateUser(t, client, &service.User{})
	account := mustCreateAccount(t, client, &service.Account{Name: "upstream-cost-requeue-test"})
	apiKey := mustCreateApiKey(t, client, &service.APIKey{UserID: user.ID})

	fixture := requeueFixture{userID: user.ID, apiKeyID: apiKey.ID, accountID: account.ID}

	t.Cleanup(func() {
		ctx := context.Background()
		// 按外键依赖逆序删除：usage_logs → api_keys / accounts → users。
		// 这些表都是 ON DELETE CASCADE，删 users 也会带走其余的；
		// 但显式逐条删能让「哪一步失败」在报错里一眼可见。
		for _, stmt := range []struct {
			sql string
			arg int64
		}{
			{`DELETE FROM usage_logs WHERE user_id = $1`, fixture.userID},
			{`DELETE FROM api_keys WHERE id = $1`, fixture.apiKeyID},
			{`DELETE FROM accounts WHERE id = $1`, fixture.accountID},
			{`DELETE FROM users WHERE id = $1`, fixture.userID},
		} {
			_, err := integrationDB.ExecContext(ctx, stmt.sql, stmt.arg)
			require.NoError(t, err, "cleanup %q", stmt.sql)
		}
	})

	return fixture
}

// requeueTestRow 描述一条待铺设的使用记录。
type requeueTestRow struct {
	createdAt     time.Time
	attempts      int16
	fetchedAt     *time.Time
	requestID     string
	expectedTouch bool
}

// seedRequeueRows 铺好测试数据，返回与入参同序的主键列表。
func seedRequeueRows(t *testing.T, f requeueFixture, rows []requeueTestRow) []int64 {
	t.Helper()
	ctx := context.Background()
	ids := make([]int64, 0, len(rows))

	for _, row := range rows {
		var id int64
		err := integrationDB.QueryRowContext(ctx, `
			INSERT INTO usage_logs (user_id, api_key_id, account_id, model,
			                        upstream_request_id, upstream_cost_attempts,
			                        upstream_cost_fetched_at, created_at, actual_cost)
			VALUES ($1, $2, $3, 'gpt-requeue-test', $4, $5, $6, $7, 0.000015)
			RETURNING id`,
			f.userID, f.apiKeyID, f.accountID,
			row.requestID, row.attempts, row.fetchedAt, row.createdAt).Scan(&id)
		require.NoError(t, err, "seed usage_logs row %q", row.requestID)
		ids = append(ids, id)
	}
	return ids
}

// attemptsOf 读回某行的重试计数。
func attemptsOf(t *testing.T, id int64) int16 {
	t.Helper()
	var attempts int16
	err := integrationDB.QueryRowContext(context.Background(),
		`SELECT upstream_cost_attempts FROM usage_logs WHERE id = $1`, id).Scan(&attempts)
	require.NoError(t, err, "read attempts of %d", id)
	return attempts
}

// TestUpstreamCostRequeueExhaustedCostsGates 逐条验证四个闸门：
// 只有「窗口内 + 有请求 ID + 未取到 + 已用尽」的记录会被重排。
func TestUpstreamCostRequeueExhaustedCostsGates(t *testing.T) {
	ctx := context.Background()
	repo := NewUpstreamCostRepository(integrationDB)
	fixture := newRequeueFixture(t)

	now := time.Date(2026, 10, 3, 6, 0, 0, 0, time.UTC)
	windowStart := now.Add(-72 * time.Hour)
	fetchedAt := now.Add(-23 * time.Hour)
	tooOld := now.Add(-96 * time.Hour) // 窗口外（4 天前）
	inWindow := now.Add(-24 * time.Hour)

	rows := []requeueTestRow{
		{createdAt: inWindow, attempts: 60, requestID: "req-exhausted", expectedTouch: true},
		{createdAt: inWindow, attempts: 59, requestID: "req-still-retrying"},
		{createdAt: inWindow, attempts: 60, fetchedAt: &fetchedAt, requestID: "req-already-fetched"},
		{createdAt: inWindow, attempts: 60, requestID: ""},
		{createdAt: tooOld, attempts: 60, requestID: "req-too-old"},
		{createdAt: now.Add(-time.Hour), attempts: 60, requestID: "req-recent-exhausted", expectedTouch: true},
	}
	ids := seedRequeueRows(t, fixture, rows)

	affected, err := repo.RequeueExhaustedCosts(ctx, windowStart, now, 60, 500)
	require.NoError(t, err)
	require.EqualValues(t, 2, affected, "只应重排窗口内两条已用尽的记录")

	for i, row := range rows {
		got := attemptsOf(t, ids[i])
		if row.expectedTouch {
			require.Zero(t, got, "记录 %q 应被重排（attempts 清零）", row.requestID)
			continue
		}
		require.Equal(t, row.attempts, got, "记录 %q 不该被碰（attempts 应保持原值）", row.requestID)
	}
}

// TestUpstreamCostRequeueExhaustedCostsLeavesMoneyUntouched 验证「只清计数」这条承诺：
// 重排绝不能顺手改动任何金额或取到时刻，否则一条「重新排队」会变成一次静默的数据改写。
func TestUpstreamCostRequeueExhaustedCostsLeavesMoneyUntouched(t *testing.T) {
	ctx := context.Background()
	repo := NewUpstreamCostRepository(integrationDB)
	fixture := newRequeueFixture(t)

	now := time.Date(2026, 10, 3, 6, 0, 0, 0, time.UTC)
	ids := seedRequeueRows(t, fixture, []requeueTestRow{
		{createdAt: now.Add(-24 * time.Hour), attempts: 60, requestID: "req-money"},
	})
	id := ids[0]

	_, err := integrationDB.ExecContext(ctx,
		`UPDATE usage_logs SET upstream_cost_original = 0, upstream_cost_currency = 'USD' WHERE id = $1`, id)
	require.NoError(t, err)

	affected, err := repo.RequeueExhaustedCosts(ctx, now.Add(-72*time.Hour), now, 60, 500)
	require.NoError(t, err)
	require.EqualValues(t, 1, affected)

	var (
		cost       *float64
		currency   *string
		fetchedAt  *time.Time
		requestID  string
		actualCost float64
	)
	err = integrationDB.QueryRowContext(ctx, `
		SELECT upstream_cost_original, upstream_cost_currency, upstream_cost_fetched_at,
		       upstream_request_id, actual_cost
		FROM usage_logs WHERE id = $1`, id).
		Scan(&cost, &currency, &fetchedAt, &requestID, &actualCost)
	require.NoError(t, err)

	require.NotNil(t, cost, "重排不得清空已写入的金额")
	require.Equal(t, 0.0, *cost)
	require.NotNil(t, currency)
	require.Equal(t, "USD", *currency)
	require.Nil(t, fetchedAt, "未取到的记录 fetched_at 本就应为空，重排也不该写它")
	require.Equal(t, "req-money", requestID)
	require.InDelta(t, 0.000015, actualCost, 1e-12, "下游收入不属于补账的职责范围")
}

// TestUpstreamCostRequeueExhaustedCostsCapsAndOrdersOldestFirst 验证封顶与优先级：
// 超出上限的记录必须留到下一次，且先处理记账更早的——否则积压时早的记录会饿死。
func TestUpstreamCostRequeueExhaustedCostsCapsAndOrdersOldestFirst(t *testing.T) {
	ctx := context.Background()
	repo := NewUpstreamCostRepository(integrationDB)
	fixture := newRequeueFixture(t)

	now := time.Date(2026, 10, 3, 6, 0, 0, 0, time.UTC)
	ids := seedRequeueRows(t, fixture, []requeueTestRow{
		{createdAt: now.Add(-70 * time.Hour), attempts: 60, requestID: "req-oldest", expectedTouch: true},
		{createdAt: now.Add(-2 * time.Hour), attempts: 60, requestID: "req-newer", expectedTouch: true},
		{createdAt: now.Add(-1 * time.Hour), attempts: 60, requestID: "req-newest"},
	})

	affected, err := repo.RequeueExhaustedCosts(ctx, now.Add(-72*time.Hour), now, 60, 2)
	require.NoError(t, err)
	require.EqualValues(t, 2, affected, "上限为 2 时只能重排 2 条")

	require.Zero(t, attemptsOf(t, ids[0]), "最早的记录必须优先被重排")
	require.Zero(t, attemptsOf(t, ids[1]))
	require.EqualValues(t, 60, attemptsOf(t, ids[2]), "超限的记录留到下一次补账")
}

// TestUpstreamCostRequeueExhaustedCostsRejectsIncoherentArgs 验证参数不自洽时
// 静默返回 0，而不是硬跑一条注定扫不到东西的语句——后者会伪装成
// 「补账跑了但没东西可补」，让配置写错变得难以发现。
func TestUpstreamCostRequeueExhaustedCostsRejectsIncoherentArgs(t *testing.T) {
	ctx := context.Background()
	repo := NewUpstreamCostRepository(integrationDB)
	fixture := newRequeueFixture(t)

	now := time.Date(2026, 10, 3, 6, 0, 0, 0, time.UTC)
	ids := seedRequeueRows(t, fixture, []requeueTestRow{
		{createdAt: now.Add(-time.Hour), attempts: 60, requestID: "req-args"},
	})

	for name, args := range map[string]struct {
		since, until time.Time
		maxAttempts  int
		limit        int
	}{
		"窗口倒置":   {until: now.Add(-72 * time.Hour), since: now, maxAttempts: 60, limit: 500},
		"窗口为空":   {since: now, until: now, maxAttempts: 60, limit: 500},
		"上限为零":   {since: now.Add(-72 * time.Hour), until: now, maxAttempts: 60, limit: 0},
		"次数上限为零": {since: now.Add(-72 * time.Hour), until: now, maxAttempts: 0, limit: 500},
	} {
		t.Run(name, func(t *testing.T) {
			affected, err := repo.RequeueExhaustedCosts(ctx, args.since, args.until, args.maxAttempts, args.limit)
			require.NoError(t, err)
			require.Zero(t, affected)
		})
	}

	require.EqualValues(t, 60, attemptsOf(t, ids[0]), "参数不自洽时一行都不许动")
}
