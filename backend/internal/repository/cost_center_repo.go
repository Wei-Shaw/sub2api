package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type costCenterRepository struct{ db *sql.DB }

func NewCostCenterRepository(db *sql.DB) service.CostCenterRepository {
	return &costCenterRepository{db: db}
}

func (r *costCenterRepository) CreateEvent(ctx context.Context, in *service.CreateCostCenterEventInput) (*service.CostCenterEvent, error) {
	occurred := time.Now().UTC()
	if in.OccurredAt != nil {
		occurred = in.OccurredAt.UTC()
	}
	metadata, err := json.Marshal(in.Metadata)
	if err != nil {
		return nil, fmt.Errorf("marshal metadata: %w", err)
	}
	const q = `INSERT INTO cost_center_events (event_type,status,source_type,source_id,idempotency_key,account_id,user_id,plan_id,platform,group_id,model,category,amount_usd,original_amount,original_currency,fx_rate,occurred_at,note,metadata,operator_id,reversal_of)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21)
ON CONFLICT (idempotency_key) WHERE idempotency_key IS NOT NULL DO UPDATE SET updated_at=NOW()
	RETURNING id,event_type,status,source_type,source_id,account_id,user_id,plan_id,platform,group_id,model,category,amount_usd,original_amount,original_currency,fx_rate,occurred_at,note,metadata,operator_id,reversal_of,created_at`
	row := r.db.QueryRowContext(ctx, q, in.EventType, in.Status, in.SourceType, in.SourceID, in.IdempotencyKey, in.AccountID, in.UserID, in.PlanID, in.Platform, in.GroupID, in.Model, in.Category, in.AmountUSD, in.OriginalAmount, in.OriginalCurrency, in.FXRate, occurred, in.Note, metadata, in.OperatorID, in.ReversalOf)
	return scanCostEvent(row)
}

type costCenterRowScanner interface{ Scan(...any) error }

func scanCostEvent(row costCenterRowScanner) (*service.CostCenterEvent, error) {
	var e service.CostCenterEvent
	var metadata []byte
	if err := row.Scan(&e.ID, &e.EventType, &e.Status, &e.SourceType, &e.SourceID, &e.AccountID, &e.UserID, &e.PlanID, &e.Platform, &e.GroupID, &e.Model, &e.Category, &e.AmountUSD, &e.OriginalAmount, &e.OriginalCurrency, &e.FXRate, &e.OccurredAt, &e.Note, &metadata, &e.OperatorID, &e.ReversalOf, &e.CreatedAt); err != nil {
		return nil, err
	}
	if len(metadata) > 0 {
		_ = json.Unmarshal(metadata, &e.Metadata)
	}
	return &e, nil
}

func (r *costCenterRepository) ListEvents(ctx context.Context, f service.CostCenterReportFilter, page, pageSize int) ([]service.CostCenterEvent, int64, error) {
	where, args := costCenterWhere(f)
	offset := (page - 1) * pageSize
	var total int64
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM cost_center_events WHERE "+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.db.QueryContext(ctx, `SELECT cost_center_events.id,cost_center_events.event_type,cost_center_events.status,cost_center_events.source_type,cost_center_events.source_id,cost_center_events.account_id,cost_center_events.user_id,cost_center_events.plan_id,cost_center_events.platform,cost_center_events.group_id,cost_center_events.model,cost_center_events.category,cost_center_events.amount_usd,cost_center_events.original_amount,cost_center_events.original_currency,cost_center_events.fx_rate,cost_center_events.occurred_at,cost_center_events.note,cost_center_events.metadata,cost_center_events.operator_id,cost_center_events.reversal_of,cost_center_events.created_at,
		COALESCE(a.name,''),COALESCE(NULLIF(u.username,''),u.email,''),COALESCE(NULLIF(op.username,''),op.email,'')
		FROM cost_center_events
		LEFT JOIN accounts a ON a.id=cost_center_events.account_id
		LEFT JOIN users u ON u.id=cost_center_events.user_id
		LEFT JOIN users op ON op.id=cost_center_events.operator_id
		WHERE `+where+" ORDER BY occurred_at DESC,cost_center_events.id DESC LIMIT $"+fmt.Sprint(len(args)+1)+" OFFSET $"+fmt.Sprint(len(args)+2), append(args, pageSize, offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()
	out := make([]service.CostCenterEvent, 0)
	for rows.Next() {
		e, err := scanListedCostEvent(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *e)
	}
	return out, total, rows.Err()
}

func scanListedCostEvent(row costCenterRowScanner) (*service.CostCenterEvent, error) {
	var e service.CostCenterEvent
	var metadata []byte
	if err := row.Scan(&e.ID, &e.EventType, &e.Status, &e.SourceType, &e.SourceID, &e.AccountID, &e.UserID, &e.PlanID, &e.Platform, &e.GroupID, &e.Model, &e.Category, &e.AmountUSD, &e.OriginalAmount, &e.OriginalCurrency, &e.FXRate, &e.OccurredAt, &e.Note, &metadata, &e.OperatorID, &e.ReversalOf, &e.CreatedAt, &e.AccountName, &e.UserName, &e.OperatorName); err != nil {
		return nil, err
	}
	if len(metadata) > 0 {
		_ = json.Unmarshal(metadata, &e.Metadata)
	}
	return &e, nil
}

func costCenterWhere(f service.CostCenterReportFilter) (string, []any) {
	where := []string{
		"cost_center_events.occurred_at >= $1",
		"cost_center_events.occurred_at < $2",
		"cost_center_events.source_type <> 'upstream'",
		"NOT EXISTS (SELECT 1 FROM cost_center_events upstream_event WHERE upstream_event.id=cost_center_events.reversal_of AND upstream_event.source_type='upstream')",
	}
	args := []any{f.Start, f.End}
	if f.AccountID != nil {
		where = append(where, fmt.Sprintf("cost_center_events.account_id=$%d", len(args)+1))
		args = append(args, *f.AccountID)
	}
	if f.Category != "" {
		where = append(where, fmt.Sprintf("cost_center_events.category=$%d", len(args)+1))
		args = append(args, f.Category)
	}
	if f.SourceType != "" {
		where = append(where, fmt.Sprintf("cost_center_events.source_type=$%d", len(args)+1))
		args = append(args, f.SourceType)
	}
	if f.Platform != "" {
		where = append(where, fmt.Sprintf("cost_center_events.platform=$%d", len(args)+1))
		args = append(args, f.Platform)
	}
	if f.UserID != nil {
		where = append(where, fmt.Sprintf("cost_center_events.user_id=$%d", len(args)+1))
		args = append(args, *f.UserID)
	}
	if f.GroupID != nil {
		where = append(where, fmt.Sprintf("cost_center_events.group_id=$%d", len(args)+1))
		args = append(args, *f.GroupID)
	}
	if f.Model != "" {
		where = append(where, fmt.Sprintf("cost_center_events.model=$%d", len(args)+1))
		args = append(args, f.Model)
	}
	if f.PlanID != nil {
		where = append(where, fmt.Sprintf("cost_center_events.plan_id=$%d", len(args)+1))
		args = append(args, *f.PlanID)
	}
	return joinAnd(where), args
}
func joinAnd(v []string) string {
	out := ""
	for i, s := range v {
		if i > 0 {
			out += " AND "
		}
		out += s
	}
	return out
}

func (r *costCenterRepository) Summarize(ctx context.Context, f service.CostCenterReportFilter) (*service.CostCenterSummary, error) {
	where, args := costCenterWhere(f)
	q := `SELECT COALESCE(SUM(CASE WHEN event_type='reversal' THEN -amount_usd ELSE amount_usd END) FILTER (WHERE event_type IN ('income','reversal') AND status='settled'),0), COALESCE(SUM(CASE WHEN event_type='reversal' THEN -amount_usd ELSE amount_usd END) FILTER (WHERE event_type IN ('consumption','subscription_recognition','reversal') AND status='settled'),0), COALESCE(SUM(CASE WHEN event_type='reversal' THEN -amount_usd ELSE amount_usd END) FILTER (WHERE event_type IN ('promotional_consumption','reversal') AND status='settled'),0), COALESCE(SUM(CASE WHEN event_type='reversal' THEN -amount_usd ELSE amount_usd END) FILTER (WHERE event_type IN ('expense','reversal') AND status='settled'),0), COALESCE(SUM(CASE WHEN event_type='reversal' THEN -amount_usd ELSE amount_usd END) FILTER (WHERE event_type IN ('expense','reversal') AND category='rebate' AND status='settled'),0), COALESCE(SUM(amount_usd) FILTER (WHERE event_type='expense' AND status='pending'),0), COALESCE(SUM(amount_usd) FILTER (WHERE source_type='unknown'),0) FROM cost_center_events WHERE ` + where
	var cash, realized, promo, expenses, rebate, pending, unknown float64
	if err := r.db.QueryRowContext(ctx, q, args...).Scan(&cash, &realized, &promo, &expenses, &rebate, &pending, &unknown); err != nil {
		return nil, err
	}
	s := &service.CostCenterSummary{CashIncome: cash, RealizedIncome: realized, PromotionalConsumption: promo, SettledExpenses: expenses, RebateAmount: rebate, PendingForecast: pending, UnknownSourceAmount: unknown}
	_ = r.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(price_usd-recognized_usd) FILTER (WHERE expires_at >= $1 AND starts_at < $2),0), COALESCE(SUM(price_usd-recognized_usd) FILTER (WHERE expires_at < $2),0) FROM cost_center_subscription_entitlements WHERE starts_at < $2`, f.Start, f.End).Scan(&s.DeferredSubscriptionUSD, &s.ExpiredEntitlementUSD)
	s.CashProfit = cash - expenses
	s.OperatingProfit = realized - expenses
	if realized != 0 {
		s.ProfitMargin = s.OperatingProfit / realized
	}
	return s, nil
}

func (r *costCenterRepository) CreateExpensePlan(ctx context.Context, in *service.CreateExpensePlanInput) (*service.ExpensePlan, error) {
	if in.IntervalUnit == "" {
		in.IntervalUnit = "month"
	}
	if in.IntervalValue <= 0 {
		in.IntervalValue = 1
	}
	var p service.ExpensePlan
	err := r.db.QueryRowContext(ctx, `INSERT INTO cost_center_expense_plans(account_id,category,amount_usd,interval_unit,interval_value,starts_at,ends_at,next_due_at,note,operator_id) VALUES($1,$2,$3,$4,$5,$6,$7,$6,$8,$9) RETURNING id,account_id,category,amount_usd,interval_unit,interval_value,starts_at,ends_at,next_due_at,active,note,operator_id`, in.AccountID, in.Category, in.AmountUSD, in.IntervalUnit, in.IntervalValue, in.StartsAt, in.EndsAt, in.StartsAt, in.Note, in.OperatorID).Scan(&p.ID, &p.AccountID, &p.Category, &p.AmountUSD, &p.IntervalUnit, &p.IntervalValue, &p.StartsAt, &p.EndsAt, &p.NextDueAt, &p.Active, &p.Note, &p.OperatorID)
	return &p, err
}

func (r *costCenterRepository) MaterializeExpensePlans(ctx context.Context, at time.Time) (int, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id,account_id,category,amount_usd,interval_unit,interval_value,next_due_at,note,operator_id FROM cost_center_expense_plans WHERE active=true AND next_due_at <= $1 AND (ends_at IS NULL OR next_due_at < ends_at) FOR UPDATE SKIP LOCKED`, at)
	if err != nil {
		return 0, err
	}
	defer func() { _ = rows.Close() }()
	count := 0
	for rows.Next() {
		var id int64
		var account *int64
		var category string
		var amount float64
		var unit string
		var value int
		var due time.Time
		var note string
		var operator *int64
		if err := rows.Scan(&id, &account, &category, &amount, &unit, &value, &due, &note, &operator); err != nil {
			return count, err
		}
		period := due.UTC().Format("2006-01-02T15:04:05Z")
		key := fmt.Sprintf("expense-plan:%d:%s", id, period)
		if _, err := r.CreateEvent(ctx, &service.CreateCostCenterEventInput{EventType: service.CostEventExpense, Status: "pending", SourceType: "recurring", SourceID: &key, IdempotencyKey: &key, AccountID: account, Category: category, AmountUSD: amount, Note: note, OperatorID: operator, OccurredAt: &due}); err != nil {
			return count, err
		}
		next := due
		switch unit {
		case "day":
			next = next.AddDate(0, 0, value)
		case "week":
			next = next.AddDate(0, 0, 7*value)
		case "month":
			next = next.AddDate(0, value, 0)
		case "quarter":
			next = next.AddDate(0, 3*value, 0)
		case "year":
			next = next.AddDate(value, 0, 0)
		default:
			next = next.Add(time.Duration(value) * time.Hour)
		}
		if _, err := r.db.ExecContext(ctx, `UPDATE cost_center_expense_plans SET next_due_at=$1, active=CASE WHEN ends_at IS NOT NULL AND $1 >= ends_at THEN false ELSE active END, updated_at=NOW() WHERE id=$2`, next, id); err != nil {
			return count, err
		}
		count++
	}
	return count, rows.Err()
}

func (r *costCenterRepository) UpdateEventStatus(ctx context.Context, id int64, status, reason string, operator *int64) (*service.CostCenterEvent, error) {
	row := r.db.QueryRowContext(ctx, `UPDATE cost_center_events SET status=$1,note=CASE WHEN $2='' THEN note ELSE note || ' [' || $2 || ']' END,operator_id=COALESCE($3,operator_id),updated_at=NOW() WHERE id=$4 RETURNING id,event_type,status,source_type,source_id,account_id,user_id,plan_id,platform,group_id,model,category,amount_usd,original_amount,original_currency,fx_rate,occurred_at,note,metadata,operator_id,reversal_of,created_at`, status, reason, operator, id)
	return scanCostEvent(row)
}

func (r *costCenterRepository) ReverseEvent(ctx context.Context, id int64, reason string, operator *int64) (*service.CostCenterEvent, error) {
	row := r.db.QueryRowContext(ctx, `SELECT id,event_type,status,source_type,source_id,account_id,user_id,plan_id,platform,group_id,model,category,amount_usd,original_amount,original_currency,fx_rate,occurred_at,note,metadata,operator_id,reversal_of,created_at FROM cost_center_events WHERE id=$1`, id)
	original, err := scanCostEvent(row)
	if err != nil {
		return nil, err
	}
	key := fmt.Sprintf("reversal:%d", id)
	meta := map[string]any{"reason": reason, "reversal_of": id}
	return r.CreateEvent(ctx, &service.CreateCostCenterEventInput{EventType: service.CostEventReversal, Status: "settled", SourceType: "reversal", SourceID: &key, IdempotencyKey: &key, AccountID: original.AccountID, UserID: original.UserID, PlanID: original.PlanID, Platform: original.Platform, GroupID: original.GroupID, Model: original.Model, Category: original.Category, AmountUSD: original.AmountUSD, OccurredAt: &original.OccurredAt, Note: reason, Metadata: meta, OperatorID: operator, ReversalOf: &original.ID})
}

func (r *costCenterRepository) Reconcile(ctx context.Context, f service.CostCenterReportFilter) (*service.CostCenterReconciliation, error) {
	where, args := costCenterWhere(f)
	var out service.CostCenterReconciliation
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FILTER (WHERE source_type='unknown'), COUNT(*) FILTER (WHERE status='pending') FROM cost_center_events WHERE `+where, args...).Scan(&out.UnknownEvents, &out.PendingEvents); err != nil {
		return nil, err
	}
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM (SELECT idempotency_key FROM cost_center_events WHERE idempotency_key IS NOT NULL AND source_type <> 'upstream' AND NOT EXISTS (SELECT 1 FROM cost_center_events upstream_event WHERE upstream_event.id=cost_center_events.reversal_of AND upstream_event.source_type='upstream') GROUP BY idempotency_key HAVING COUNT(*)>1) d`).Scan(&out.DuplicateKeys); err != nil {
		return nil, err
	}
	return &out, nil
}

func (r *costCenterRepository) SnapshotSubscriptionEntitlement(ctx context.Context, in *service.SubscriptionEntitlementSnapshot) error {
	factor := 0.0
	if in.StandardQuotaTokens > 0 {
		factor = in.PriceUSD / float64(in.StandardQuotaTokens)
	}
	_, err := r.db.ExecContext(ctx, `INSERT INTO cost_center_subscription_entitlements(order_id,user_id,plan_id,group_id,price_usd,standard_quota_tokens,realization_factor,starts_at,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) ON CONFLICT(order_id) DO NOTHING`, in.OrderID, in.UserID, in.PlanID, in.GroupID, in.PriceUSD, in.StandardQuotaTokens, factor, in.StartsAt, in.ExpiresAt)
	return err
}

func (r *costCenterRepository) RecognizeSubscriptionUsage(ctx context.Context, entitlementID int64, requestID string, tokens int64, standardCost float64, occurred time.Time) (*service.CostCenterEvent, error) {
	return r.recognizeSubscriptionUsage(ctx, entitlementID, nil, nil, requestID, tokens, standardCost, occurred)
}

func (r *costCenterRepository) RecognizeSubscriptionUsageForUsage(ctx context.Context, userID int64, groupID *int64, requestID string, tokens int64, standardCost float64, occurred time.Time) (*service.CostCenterEvent, error) {
	return r.recognizeSubscriptionUsage(ctx, 0, &userID, groupID, requestID, tokens, standardCost, occurred)
}

func (r *costCenterRepository) recognizeSubscriptionUsage(ctx context.Context, entitlementID int64, userID, groupID *int64, requestID string, tokens int64, standardCost float64, occurred time.Time) (*service.CostCenterEvent, error) {
	if requestID == "" || tokens <= 0 || standardCost <= 0 {
		return nil, nil
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	var price, factor, recognized float64
	var planID, entitlementGroup, entitlementUser int64
	if entitlementID > 0 {
		err = tx.QueryRowContext(ctx, `SELECT price_usd,realization_factor,recognized_usd,user_id,COALESCE(group_id,0),COALESCE(plan_id,0) FROM cost_center_subscription_entitlements WHERE id=$1 FOR UPDATE`, entitlementID).Scan(&price, &factor, &recognized, &entitlementUser, &entitlementGroup, &planID)
	} else {
		if userID == nil || groupID == nil {
			return nil, nil
		}
		err = tx.QueryRowContext(ctx, `SELECT price_usd,realization_factor,recognized_usd,user_id,COALESCE(group_id,0),COALESCE(plan_id,0),id FROM cost_center_subscription_entitlements WHERE user_id=$1 AND group_id=$2 AND starts_at <= $3 AND expires_at > $3 ORDER BY expires_at, id FOR UPDATE LIMIT 1`, *userID, *groupID, occurred).Scan(&price, &factor, &recognized, &entitlementUser, &entitlementGroup, &planID, &entitlementID)
	}
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	key := "subscription-recognition:" + requestID
	var existingID int64
	err = tx.QueryRowContext(ctx, `SELECT id FROM cost_center_events WHERE idempotency_key=$1`, key).Scan(&existingID)
	if err == nil {
		_ = tx.Commit()
		return r.getEvent(ctx, existingID)
	}
	if err != sql.ErrNoRows {
		return nil, err
	}
	amount := standardCost * factor
	remaining := price - recognized
	if amount > remaining {
		amount = remaining
	}
	if amount <= 0 {
		_ = tx.Commit()
		return nil, nil
	}
	user := entitlementUser
	var plan *int64
	if planID > 0 {
		plan = &planID
	}
	var group *int64
	if entitlementGroup > 0 {
		group = &entitlementGroup
	}
	metadata, _ := json.Marshal(map[string]any{"tokens": tokens, "standard_cost": standardCost, "entitlement_id": entitlementID})
	var id int64
	err = tx.QueryRowContext(ctx, `INSERT INTO cost_center_events(event_type,status,source_type,source_id,idempotency_key,user_id,plan_id,group_id,amount_usd,occurred_at,note,metadata) VALUES('subscription_recognition','settled','subscription',$1,$2,$3,$4,$5,$6,$7,'subscription usage recognition',$8) RETURNING id`, requestID, key, user, plan, group, amount, occurred, metadata).Scan(&id)
	if err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE cost_center_subscription_entitlements SET recognized_usd=LEAST(price_usd,recognized_usd+$1),consumed_tokens=consumed_tokens+$2,updated_at=NOW() WHERE id=$3`, amount, tokens, entitlementID); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return r.getEvent(ctx, id)
}

func (r *costCenterRepository) getEvent(ctx context.Context, id int64) (*service.CostCenterEvent, error) {
	return scanCostEvent(r.db.QueryRowContext(ctx, `SELECT id,event_type,status,source_type,source_id,account_id,user_id,plan_id,platform,group_id,model,category,amount_usd,original_amount,original_currency,fx_rate,occurred_at,note,metadata,operator_id,reversal_of,created_at FROM cost_center_events WHERE id=$1`, id))
}
