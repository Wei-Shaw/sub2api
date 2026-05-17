package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/Wei-Shaw/sub2api/internal/service"
	gocache "github.com/patrickmn/go-cache"
)

const rawUsageLogModelColumn = "model"

// rawUsageLogModelColumn preserves the exact stored usage_logs.model semantics for direct filters.
// Historical rows may contain upstream/billing model values, while newer rows store requested_model.
// Requested/upstream/mapping analytics must use resolveModelDimensionExpression instead.

// usageLogSuccessFilterUL 用于把"失败请求 usage log"（tokens=0、cost=0、不计费的占位记录）
// 从统计性聚合中排除，避免污染 Dashboard / 用量拆分等指标。
//
// schema 中没有 success bool 列；新增列要做迁移，风险大；这里用 actual_cost > 0 作为代理：
// 任何成功落账的请求都会产生 actual_cost（包括 token 计费、纯图片 token 计费、按次/按图计费），
// 反之 failed-request usage log 的 actual_cost 为 0。
// 早期版本用 4 项 token 和 > 0 判定会把"按次/按图计费"与"image_output_tokens 独立计费"的纯图片
// 请求误判为失败，导致这部分请求从用量统计里消失，故改用 actual_cost。
// 配合 `FROM usage_logs ul` JOIN 查询使用。
const usageLogSuccessFilterUL = "ul.actual_cost > 0"

// usageLogEffectivePlatformExpr 用于按"有效平台"维度聚合 usage_logs：
// 优先取请求实际走的分组 platform，若分组未设置 platform 再 fallback 到 account.platform。
// Composite groups are a routing layer, so platform analytics must use the
// resolved concrete account platform instead of grouping spend under "composite".
// 配套要求查询里 LEFT JOIN groups g ON g.id = ul.group_id 与 LEFT JOIN accounts a ON a.id = ul.account_id。
const usageLogEffectivePlatformExpr = "CASE WHEN g.platform = 'composite' THEN a.platform ELSE COALESCE(NULLIF(g.platform,''), a.platform) END"

// dateFormatWhitelist 将 granularity 参数映射为 PostgreSQL TO_CHAR 格式字符串，防止外部输入直接拼入 SQL
var dateFormatWhitelist = map[string]string{
	"hour":  "YYYY-MM-DD HH24:00",
	"day":   "YYYY-MM-DD",
	"week":  "IYYY-IW",
	"month": "YYYY-MM",
}

// safeDateFormat 根据白名单获取 dateFormat，未匹配时返回默认值
func safeDateFormat(granularity string) string {
	if f, ok := dateFormatWhitelist[granularity]; ok {
		return f
	}
	return "YYYY-MM-DD"
}

// appendRawUsageLogModelWhereCondition keeps direct model filters on the raw model column for backward
// compatibility with historical rows. Requested/upstream analytics must use
// resolveModelDimensionExpression instead.
func appendRawUsageLogModelWhereCondition(conditions []string, args []any, model string) ([]string, []any) {
	if strings.TrimSpace(model) == "" {
		return conditions, args
	}
	conditions = append(conditions, fmt.Sprintf("%s = $%d", rawUsageLogModelColumn, len(args)+1))
	args = append(args, model)
	return conditions, args
}

func appendUsageLogBillingModeWhereCondition(conditions []string, args []any, billingMode string) ([]string, []any) {
	return appendUsageLogBillingModeWhereConditionWithAlias(conditions, args, billingMode, "")
}

func appendUsageLogBillingModeWhereConditionWithAlias(conditions []string, args []any, billingMode string, alias string) ([]string, []any) {
	mode := strings.TrimSpace(billingMode)
	if mode == "" {
		return conditions, args
	}
	column := func(name string) string {
		if alias == "" {
			return name
		}
		return alias + "." + name
	}
	placeholder := fmt.Sprintf("$%d", len(args)+1)
	switch service.BillingMode(mode) {
	case service.BillingModeImage:
		conditions = append(conditions, fmt.Sprintf("(%s = %s OR ((%s IS NULL OR %s = '') AND COALESCE(%s, 0) > 0))", column("billing_mode"), placeholder, column("billing_mode"), column("billing_mode"), column("image_count")))
	case service.BillingModeVideo:
		conditions = append(conditions, fmt.Sprintf("%s = %s", column("billing_mode"), placeholder))
	case service.BillingModeToken:
		conditions = append(conditions, fmt.Sprintf("(%s = %s OR ((%s IS NULL OR %s = '') AND COALESCE(%s, 0) <= 0))", column("billing_mode"), placeholder, column("billing_mode"), column("billing_mode"), column("image_count")))
	default:
		conditions = append(conditions, fmt.Sprintf("%s = %s", column("billing_mode"), placeholder))
	}
	args = append(args, mode)
	return conditions, args
}

func appendUsageLogBillingModeQueryFilter(query string, args []any, billingMode string, alias string) (string, []any) {
	conditions, args := appendUsageLogBillingModeWhereConditionWithAlias(nil, args, billingMode, alias)
	if len(conditions) == 0 {
		return query, args
	}
	return query + " AND " + conditions[0], args
}

func appendUsageLogModelWhereCondition(conditions []string, args []any, model string, source string) ([]string, []any) {
	if strings.TrimSpace(source) == "" {
		return appendRawUsageLogModelWhereCondition(conditions, args, model)
	}
	if strings.TrimSpace(model) == "" {
		return conditions, args
	}
	conditions = append(conditions, fmt.Sprintf("%s = $%d", resolveModelDimensionExpression(source), len(args)+1))
	args = append(args, model)
	return conditions, args
}

// appendRawUsageLogModelQueryFilter keeps direct model filters on the raw model column for backward
// compatibility with historical rows. Requested/upstream analytics must use
// resolveModelDimensionExpression instead.
func appendRawUsageLogModelQueryFilter(query string, args []any, model string) (string, []any) {
	if strings.TrimSpace(model) == "" {
		return query, args
	}
	query += fmt.Sprintf(" AND %s = $%d", rawUsageLogModelColumn, len(args)+1)
	args = append(args, model)
	return query, args
}

func appendUsageLogModelQueryFilter(query string, args []any, model string, source string) (string, []any) {
	if strings.TrimSpace(source) == "" {
		return appendRawUsageLogModelQueryFilter(query, args, model)
	}
	if strings.TrimSpace(model) == "" {
		return query, args
	}
	query += fmt.Sprintf(" AND %s = $%d", resolveModelDimensionExpression(source), len(args)+1)
	args = append(args, model)
	return query, args
}

type usageLogRepository struct {
	client *dbent.Client
	sql    sqlExecutor
	db     *sql.DB

	createBatchOnce     sync.Once
	createBatchCh       chan usageLogCreateRequest
	bestEffortBatchOnce sync.Once
	bestEffortBatchCh   chan usageLogBestEffortRequest
	bestEffortRecent    *gocache.Cache
}

func NewUsageLogRepository(client *dbent.Client, sqlDB *sql.DB) service.UsageLogRepository {
	return newUsageLogRepositoryWithSQL(client, sqlDB)
}

func newUsageLogRepositoryWithSQL(client *dbent.Client, sqlq sqlExecutor) *usageLogRepository {
	// 使用 scanSingleRow 替代 QueryRowContext，保证 ent.Tx 作为 sqlExecutor 可用。
	repo := &usageLogRepository{client: client, sql: sqlq}
	if db, ok := sqlq.(*sql.DB); ok {
		repo.db = db
	}
	repo.bestEffortRecent = gocache.New(usageLogBestEffortRecentTTL, time.Minute)
	return repo
}

// creditUsageUnitScale is DEPRECATED — kept only for backward-compatible JSON output.
// The actual credit unit is now tokens (1 credit = 1 token). Purchased credits come from
// payment_orders.actual_credits; remaining/used credits are derived proportionally from balance.
const creditUsageUnitScale = 1.0

// GetUserCreditUsageSummary reconstructs aggregate credit usage from existing monetary ledgers.
// It intentionally does not allocate balance consumption to a specific purchased package: usage_logs
// do not store payment_order_id/balance_package_id, so this endpoint is an aggregate estimate surface.
func (r *usageLogRepository) GetUserCreditUsageSummary(ctx context.Context, userID int64) (*usagestats.CreditUsageSummary, error) {
	var balanceLedger float64
	if err := scanSingleRow(ctx, r.sql, `
		SELECT COALESCE(balance, 0)
		FROM users
		WHERE id = $1 AND deleted_at IS NULL
	`, []any{userID}, &balanceLedger); err != nil {
		return nil, fmt.Errorf("query user balance: %w", err)
	}

	summary := &usagestats.CreditUsageSummary{
		UserID:              userID,
		CreditUnitScale:     creditUsageUnitScale,
		BalanceLedgerAmount: balanceLedger,
		Accuracy:            "balance_derived",
		AccuracyNotes: []string{
			"used credits = purchased_credits - remaining_credits (derived from current balance)",
			"purchased credits are summed from payment_orders.actual_credits (tokens stored at order creation)",
			"remaining_credits = purchased_credits × (current_balance / purchased_ledger) — proportional to USD remaining",
		},
	}

	if err := r.populateUserCreditUsageTotals(ctx, summary); err != nil {
		return nil, err
	}
	if err := r.populateUserCreditPurchaseGroups(ctx, summary); err != nil {
		return nil, err
	}

	// Derive used credits from balance rather than summing usage_logs.
	// This is the source of truth: used_ledger = purchased_ledger - current_balance.
	summary.TotalUsedLedgerAmount = summary.TotalPurchasedLedgerAmount - balanceLedger
	if summary.TotalUsedLedgerAmount < 0 {
		summary.TotalUsedLedgerAmount = 0
	}
	// used_credits = purchased_credits - remaining_credits
	// remaining_credits is already computed per-group in populateUserCreditPurchaseGroups;
	// for the aggregate total, we sum remaining across groups.
	totalRemaining := 0.0
	for _, est := range summary.GroupEstimates {
		totalRemaining += est.RemainingCredits
	}
	summary.TotalUsedCredits = summary.TotalPurchasedCredits - totalRemaining
	if summary.TotalUsedCredits < 0 {
		summary.TotalUsedCredits = 0
	}

	return summary, nil
}

func (r *usageLogRepository) populateUserCreditUsageTotals(ctx context.Context, summary *usagestats.CreditUsageSummary) error {
	query := `
		SELECT COUNT(*)
		FROM usage_logs
		WHERE user_id = $1
	`
	if err := scanSingleRow(ctx, r.sql, query, []any{summary.UserID}, &summary.UsageLogCount); err != nil {
		return fmt.Errorf("query user credit usage totals: %w", err)
	}
	return nil
}

func (r *usageLogRepository) populateUserCreditPurchaseGroups(ctx context.Context, summary *usagestats.CreditUsageSummary) (err error) {
	query := `
		SELECT
			COALESCE(po.balance_group_id, 0) AS group_id,
			COALESCE(g.name, '') AS group_name,
			COALESCE(g.rate_multiplier, 0) AS rate_multiplier,
			COALESCE(SUM(po.ledger_amount), 0) AS purchased_ledger_amount,
			COALESCE(SUM(COALESCE(po.actual_credits, 0)), 0) AS purchased_credits
		FROM payment_orders po
		LEFT JOIN groups g ON g.id = po.balance_group_id
		WHERE po.user_id = $1
			AND po.order_type = $2
			AND po.status = $3
		GROUP BY COALESCE(po.balance_group_id, 0), COALESCE(g.name, ''), COALESCE(g.rate_multiplier, 0)
		ORDER BY group_id
	`
	rows, err := r.sql.QueryContext(ctx, query, summary.UserID, payment.OrderTypeBalance, payment.OrderStatusCompleted)
	if err != nil {
		return fmt.Errorf("query user credit purchase groups: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			err = errors.Join(err, closeErr)
		}
	}()

	estimates := make([]usagestats.CreditUsageGroupEstimate, 0)
	for rows.Next() {
		var estimate usagestats.CreditUsageGroupEstimate
		if err := rows.Scan(&estimate.GroupID, &estimate.GroupName, &estimate.RateMultiplier, &estimate.PurchasedLedgerAmount, &estimate.PurchasedCredits); err != nil {
			return fmt.Errorf("scan user credit purchase group: %w", err)
		}
		summary.TotalPurchasedLedgerAmount += estimate.PurchasedLedgerAmount
		summary.TotalPurchasedCredits += estimate.PurchasedCredits
		if estimate.GroupID == 0 || estimate.RateMultiplier <= 0 {
			summary.UnassignedPurchasedLedgerAmount += estimate.PurchasedLedgerAmount
			continue
		}
		// Remaining credits = proportional to balance remaining vs total purchased ledger.
		// This avoids any hardcoded pricing constant — purely USD-ratio based.
		if estimate.PurchasedLedgerAmount > 0 && summary.BalanceLedgerAmount > 0 {
			estimate.RemainingCredits = estimate.PurchasedCredits * (summary.BalanceLedgerAmount / estimate.PurchasedLedgerAmount)
			if estimate.RemainingCredits > estimate.PurchasedCredits {
				estimate.RemainingCredits = estimate.PurchasedCredits
			}
		}
		estimates = append(estimates, estimate)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate user credit purchase groups: %w", err)
	}
	summary.GroupEstimates = estimates
	return nil
}

func buildWhere(conditions []string) string {
	if len(conditions) == 0 {
		return ""
	}
	return "WHERE " + strings.Join(conditions, " AND ")
}

func appendRequestTypeOrStreamWhereCondition(conditions []string, args []any, requestType *int16, stream *bool) ([]string, []any) {
	if requestType != nil {
		condition, conditionArgs := buildRequestTypeFilterCondition(len(args)+1, *requestType)
		conditions = append(conditions, condition)
		args = append(args, conditionArgs...)
		return conditions, args
	}
	if stream != nil {
		conditions = append(conditions, fmt.Sprintf("stream = $%d", len(args)+1))
		args = append(args, *stream)
	}
	return conditions, args
}

func appendRequestTypeOrStreamQueryFilter(query string, args []any, requestType *int16, stream *bool) (string, []any) {
	if requestType != nil {
		condition, conditionArgs := buildRequestTypeFilterCondition(len(args)+1, *requestType)
		query += " AND " + condition
		args = append(args, conditionArgs...)
		return query, args
	}
	if stream != nil {
		query += fmt.Sprintf(" AND stream = $%d", len(args)+1)
		args = append(args, *stream)
	}
	return query, args
}

// buildRequestTypeFilterCondition 在 request_type 过滤时兼容 legacy 字段，避免历史数据漏查。
func buildRequestTypeFilterCondition(startArgIndex int, requestType int16) (string, []any) {
	return buildRequestTypeFilterConditionWithAlias(startArgIndex, requestType, "")
}

func buildRequestTypeFilterConditionWithAlias(startArgIndex int, requestType int16, alias string) (string, []any) {
	normalized := service.RequestTypeFromInt16(requestType)
	requestTypeArg := int16(normalized)
	prefix := ""
	if alias != "" {
		prefix = alias + "."
	}
	switch normalized {
	case service.RequestTypeSync:
		return fmt.Sprintf("(%srequest_type = $%d OR (%srequest_type = %d AND %sstream = FALSE AND %sopenai_ws_mode = FALSE))", prefix, startArgIndex, prefix, int16(service.RequestTypeUnknown), prefix, prefix), []any{requestTypeArg}
	case service.RequestTypeStream:
		return fmt.Sprintf("(%srequest_type = $%d OR (%srequest_type = %d AND %sstream = TRUE AND %sopenai_ws_mode = FALSE))", prefix, startArgIndex, prefix, int16(service.RequestTypeUnknown), prefix, prefix), []any{requestTypeArg}
	case service.RequestTypeWSV2:
		return fmt.Sprintf("(%srequest_type = $%d OR (%srequest_type = %d AND %sopenai_ws_mode = TRUE))", prefix, startArgIndex, prefix, int16(service.RequestTypeUnknown), prefix), []any{requestTypeArg}
	default:
		return fmt.Sprintf("%srequest_type = $%d", prefix, startArgIndex), []any{requestTypeArg}
	}
}
