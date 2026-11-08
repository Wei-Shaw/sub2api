package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// upstreamCostRepo 是上游成本取数任务在使用记录表上的读写实现。
//
// 只碰 usage_logs 这张表，且只碰 246 号迁移新增的那几列与 upstream_request_id，
// 不改动任何计费口径的列。
type upstreamCostRepo struct {
	db *sql.DB
}

// NewUpstreamCostRepository 创建上游成本取数仓储。
func NewUpstreamCostRepository(db *sql.DB) service.UpstreamCostRepository {
	return &upstreamCostRepo{db: db}
}

// upstreamCostPendingQuery 取一批待查记录。
//
// 条件与 246 号迁移里的部分索引谓词保持一致（带请求 ID + 尚未取到），
// 这样这条查询能直接走那个小索引，而不是扫全表。
//
// ORDER BY created_at ASC 是有意的：按记账时刻先来先查，
// 突发流量把队列撑爆时不会让早先的记录永远排在后面饿死。
const upstreamCostPendingQuery = `
SELECT id, upstream_request_id, upstream_cost_attempts, created_at
FROM usage_logs
WHERE upstream_request_id IS NOT NULL
  AND upstream_request_id <> ''
  AND upstream_cost_fetched_at IS NULL
  AND created_at <= $1
  AND upstream_cost_attempts < $2
ORDER BY created_at ASC
LIMIT $3`

// ListUpstreamCostPending 实现 service.UpstreamCostRepository。
func (r *upstreamCostRepo) ListUpstreamCostPending(
	ctx context.Context,
	now time.Time,
	firstDelay time.Duration,
	maxAttempts int,
	limit int,
) ([]service.UpstreamCostPending, error) {
	if r == nil || r.db == nil {
		return nil, nil
	}
	if limit <= 0 {
		return nil, nil
	}
	// 只查「记账时刻已经早于 now-firstDelay」的记录：上游账单落库需要时间，
	// 太早查是注定白跑，还会把重试次数浪费掉。
	cutoff := now.Add(-firstDelay)

	rows, err := r.db.QueryContext(ctx, upstreamCostPendingQuery, cutoff, maxAttempts, limit)
	if err != nil {
		return nil, fmt.Errorf("list upstream cost pending: %w", err)
	}
	defer func() { _ = rows.Close() }()

	items := make([]service.UpstreamCostPending, 0, limit)
	for rows.Next() {
		var item service.UpstreamCostPending
		if err := rows.Scan(&item.UsageLogID, &item.RequestID, &item.Attempts, &item.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan upstream cost pending: %w", err)
		}
		item.RequestID = strings.TrimSpace(item.RequestID)
		// 理论上 SQL 已经把空 ID 挡在外面，这里再挡一次：取数任务唯一的输入
		// 就是这个 ID，放一个空串进去只会换来一次必然失败的请求。
		if item.RequestID == "" {
			continue
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate upstream cost pending: %w", err)
	}
	return items, nil
}

// upstreamCostResolveQuery 写入取到的成本。
//
// 同时写 fetched_at —— 它是「已取到」的唯一判据，写上有值即代表这条记录从此
// 不再进入待查队列（部分索引也会随之把它移出）。
//
// 只写原币金额与币种，不写任何换算结果：页面上的「费用」就是美元原值，
// 两者同币种直接相减才是主人要看的毛利。
const upstreamCostResolveQuery = `
UPDATE usage_logs
SET upstream_cost_original   = $2,
    upstream_cost_currency   = $3,
    upstream_cost_fetched_at = $4,
    upstream_first_token_ms  = $5,
    upstream_duration_ms     = $6,
    upstream_supplier_id     = $7,
    upstream_supplier_name   = $8,
    upstream_channel_id      = $9,
    upstream_target_channel_id = $10
WHERE id = $1`

// ResolveUpstreamCost 实现 service.UpstreamCostRepository。
func (r *upstreamCostRepo) ResolveUpstreamCost(
	ctx context.Context,
	usageLogID int64,
	value service.UpstreamCostValue,
	at time.Time,
) error {
	if r == nil || r.db == nil {
		return nil
	}
	if _, err := r.db.ExecContext(ctx, upstreamCostResolveQuery,
		usageLogID, value.Original, value.Currency, at,
		value.FirstTokenMs, value.UpstreamDurationMs,
		value.SupplierID, value.SupplierName, value.ChannelID, value.TargetChannelID); err != nil {
		return fmt.Errorf("resolve upstream cost: %w", err)
	}
	return nil
}

// upstreamCostRetryQuery 记一次「查了但没有」。
//
// 只加计数、不写 fetched_at：这条记录下一轮仍会被捞出来重试，
// 直到计数达到上限后自然退出队列（呈现为「未取到」）。
const upstreamCostRetryQuery = `
UPDATE usage_logs
SET upstream_cost_attempts = upstream_cost_attempts + 1
WHERE id = $1`

// RetryUpstreamCost 实现 service.UpstreamCostRepository。
func (r *upstreamCostRepo) RetryUpstreamCost(ctx context.Context, usageLogID int64) error {
	if r == nil || r.db == nil {
		return nil
	}
	if _, err := r.db.ExecContext(ctx, upstreamCostRetryQuery, usageLogID); err != nil {
		return fmt.Errorf("retry upstream cost: %w", err)
	}
	return nil
}

// upstreamCostRequeueQuery 把窗口内「重试用尽仍未取到」的记录重新排回待查队列。
//
// 这是状态机的第四条边。前三条边（查到 / 查了没有 / 用尽认输）都建立在
// 「重试用尽是终局」这个假设上，而线上实测推翻了这个假设：
// 2026-10-02 UTC 16:00~18:59 上游出现延迟尖峰，177 条里有 16 条在
// 90s+20×30s≈11.5 分钟的预算内始终查不到；次日把 attempts 清零重排队，
// 采集器一轮就 examined=16 resolved=16 —— 账单一直都在，只是迟到了。
// 所以「用尽」必须可撤销，否则一次上游抖动会留下永久性的账目缺口。
//
// 三条闸门缺一不可：
//   - upstream_cost_attempts >= $3：只捞已经认输的。仍在重试队列里的记录
//     本来就还会被查，重排它们等于把已有的重试进度白白清掉。
//   - created_at 落在 [$1, $2) 窗口内：窗口本身就是重排次数的天然上限——
//     一条记录最多被同一个窗口扫到「窗口天数」次，之后永久出局。
//     这是刻意不用新增计数列换来的：usage_logs 是全站最热的表，
//     为「这条记录被补账过几次」再加一列，代价远大于收益。
//   - LIMIT $4：封顶。某天若因异常堆积出几万条，不封顶就会在下一次补账时
//     对上游连发几万次请求，把配额和连接一起打穿。超出的留到明天，
//     按记账时刻升序处理，早的记录优先。
//
// 只清计数，绝不碰金额与 fetched_at：记录随后回到待查队列，由采集器
// 按正常路径重查——查询、ID 校验、币种判定全部复用同一条代码路径，
// 不产生第二条会和主流程漂移的写入口。
const upstreamCostRequeueQuery = `
UPDATE usage_logs
SET upstream_cost_attempts = 0
WHERE id IN (
	SELECT id
	FROM usage_logs
	WHERE upstream_request_id IS NOT NULL
	  AND upstream_request_id <> ''
	  AND upstream_cost_fetched_at IS NULL
	  AND upstream_cost_attempts >= $3
	  AND created_at >= $1
	  AND created_at < $2
	ORDER BY created_at ASC
	LIMIT $4
)`

// RequeueExhaustedCosts 实现 service.UpstreamCostRepository。
func (r *upstreamCostRepo) RequeueExhaustedCosts(
	ctx context.Context,
	since time.Time,
	until time.Time,
	maxAttempts int,
	limit int,
) (int64, error) {
	if r == nil || r.db == nil {
		return 0, nil
	}
	// 参数不自洽时直接返回 0 而不是硬跑一条注定扫不到东西的语句：
	// 窗口倒置会让 `created_at >= since AND created_at < until` 恒为假，
	// limit<=0 会让 LIMIT 0 静默命中 0 行——两者都会伪装成「补账跑了但没东西可补」。
	if maxAttempts <= 0 || limit <= 0 || !until.After(since) {
		return 0, nil
	}

	res, err := r.db.ExecContext(ctx, upstreamCostRequeueQuery, since, until, maxAttempts, limit)
	if err != nil {
		return 0, fmt.Errorf("requeue exhausted upstream cost: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("requeue exhausted upstream cost rows: %w", err)
	}
	return affected, nil
}

// opsSupplierBackfillQuery 取等待回填上游商户的 ops 错误日志。
//
// 只取「有上游请求 ID、还没回填商户、且足够新」的行：
//   - 没有上游请求 ID 的行（网络层失败、本地失败）**永远查不到商户**，
//     让它们留在结果里只会每轮白跑一次上游；
//   - 上游的「失败类日志」保留量极小（实测约 405 条），超出窗口后即便当时有
//     也补不回来，所以只回看最近一小段时间，不做无限重试。
//
// 部分索引 idx_ops_error_logs_supplier_backfill 与这里的谓词保持一致。
const opsSupplierBackfillQuery = `
SELECT id, upstream_request_id
FROM ops_error_logs
WHERE upstream_request_id IS NOT NULL
  AND upstream_request_id <> ''
  AND upstream_supplier_id IS NULL
  AND upstream_supplier_name IS NULL
  AND created_at >= $1
ORDER BY created_at ASC
LIMIT $2`

// ListOpsSupplierBackfill 实现 service.UpstreamCostRepository。
func (r *upstreamCostRepo) ListOpsSupplierBackfill(ctx context.Context, since time.Time, limit int) ([]service.OpsSupplierBackfill, error) {
	if r == nil || r.db == nil || limit <= 0 {
		return nil, nil
	}
	rows, err := r.db.QueryContext(ctx, opsSupplierBackfillQuery, since, limit)
	if err != nil {
		return nil, fmt.Errorf("list ops supplier backfill: %w", err)
	}
	// 只读查询；迭代错误已由下面的 rows.Err() 检查，Close 的错误无需重复上报。
	defer rows.Close() //nolint:errcheck

	out := make([]service.OpsSupplierBackfill, 0, limit)
	for rows.Next() {
		var item service.OpsSupplierBackfill
		if err := rows.Scan(&item.ID, &item.UpstreamRequestID); err != nil {
			return nil, fmt.Errorf("scan ops supplier backfill: %w", err)
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate ops supplier backfill: %w", err)
	}
	return out, nil
}

// opsSupplierResolveQuery 回填商户。
//
// 条件里带上 `upstream_supplier_id IS NULL AND upstream_supplier_name IS NULL`：
// 回填是幂等的，重复执行不会覆盖已有结果（同一行被两轮同时捞到时只写一次）。
const opsSupplierResolveQuery = `
UPDATE ops_error_logs
   SET upstream_supplier_id = $2,
       upstream_supplier_name = $3
 WHERE id = $1
   AND upstream_supplier_id IS NULL
   AND upstream_supplier_name IS NULL`

// ResolveOpsSupplier 实现 service.UpstreamCostRepository。
func (r *upstreamCostRepo) ResolveOpsSupplier(ctx context.Context, opsLogID int64, supplierID *int, supplierName string) error {
	if r == nil || r.db == nil || opsLogID <= 0 {
		return nil
	}
	// 名字为空时写空串而不是 NULL：否则 `upstream_supplier_name IS NULL` 恒为真，
	// 这条记录会被每一轮反复捞出来重查。
	name := strings.TrimSpace(supplierName)
	if _, err := r.db.ExecContext(ctx, opsSupplierResolveQuery, opsLogID, supplierID, name); err != nil {
		return fmt.Errorf("resolve ops supplier: %w", err)
	}
	return nil
}
