package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type provisionRepository struct {
	client *dbent.Client
	sql    sqlExecutor
}

func NewProvisionRepository(client *dbent.Client, sqlDB *sql.DB) service.ProvisionRepository {
	return &provisionRepository{client: client, sql: sqlDB}
}

func (r *provisionRepository) sqlExec(ctx context.Context) sqlQueryExecutor {
	return txAwareSQLExecutor(ctx, r.sql, r.client)
}

func (r *provisionRepository) queryRow(ctx context.Context, query string, args ...any) rowScanner {
	return &provisionQueryRow{ctx: ctx, exec: r.sqlExec(ctx), query: query, args: args}
}

func (r *provisionRepository) ListPlans(ctx context.Context) ([]service.ProvisionPlan, error) {
	rows, err := r.sqlExec(ctx).QueryContext(ctx, `
SELECT p.id, p.code, p.name, p.group_id, p.balance, p.quota, p.expires_in_days,
       p.rate_limit_5h, p.rate_limit_1d, p.rate_limit_7d, p.concurrency, p.rpm_limit,
       p.enabled, p.created_at, p.updated_at,
       g.name, g.platform, g.rate_multiplier, g.is_exclusive, g.status, g.subscription_type
FROM provision_plans p
LEFT JOIN groups g ON g.id = p.group_id
ORDER BY p.created_at DESC, p.id DESC`)
	if err != nil {
		return nil, fmt.Errorf("list provision plans: %w", err)
	}
	defer rows.Close()

	var plans []service.ProvisionPlan
	for rows.Next() {
		plan, err := scanProvisionPlan(rows)
		if err != nil {
			return nil, err
		}
		plans = append(plans, *plan)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return plans, nil
}

func (r *provisionRepository) GetPlanByID(ctx context.Context, id int64) (*service.ProvisionPlan, error) {
	row := r.queryRow(ctx, `
SELECT p.id, p.code, p.name, p.group_id, p.balance, p.quota, p.expires_in_days,
       p.rate_limit_5h, p.rate_limit_1d, p.rate_limit_7d, p.concurrency, p.rpm_limit,
       p.enabled, p.created_at, p.updated_at,
       g.name, g.platform, g.rate_multiplier, g.is_exclusive, g.status, g.subscription_type
FROM provision_plans p
LEFT JOIN groups g ON g.id = p.group_id
WHERE p.id = $1`, id)
	return scanProvisionPlan(row)
}

func (r *provisionRepository) GetPlanByCode(ctx context.Context, code string) (*service.ProvisionPlan, error) {
	row := r.queryRow(ctx, `
SELECT p.id, p.code, p.name, p.group_id, p.balance, p.quota, p.expires_in_days,
       p.rate_limit_5h, p.rate_limit_1d, p.rate_limit_7d, p.concurrency, p.rpm_limit,
       p.enabled, p.created_at, p.updated_at,
       g.name, g.platform, g.rate_multiplier, g.is_exclusive, g.status, g.subscription_type
FROM provision_plans p
LEFT JOIN groups g ON g.id = p.group_id
WHERE p.code = $1`, code)
	return scanProvisionPlan(row)
}

func (r *provisionRepository) CreatePlan(ctx context.Context, input service.ProvisionPlanInput) (*service.ProvisionPlan, error) {
	row := r.queryRow(ctx, `
INSERT INTO provision_plans (
    code, name, group_id, balance, quota, expires_in_days,
    rate_limit_5h, rate_limit_1d, rate_limit_7d, concurrency, rpm_limit, enabled
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
RETURNING id`, input.Code, input.Name, input.GroupID, input.Balance, input.Quota, nullableInt(input.ExpiresInDays),
		input.RateLimit5h, input.RateLimit1d, input.RateLimit7d, input.Concurrency, input.RPMLimit, input.Enabled)
	var id int64
	if err := row.Scan(&id); err != nil {
		return nil, translatePersistenceError(err, nil, service.ErrProvisionPlanExists)
	}
	return r.GetPlanByID(ctx, id)
}

func (r *provisionRepository) UpdatePlan(ctx context.Context, id int64, input service.ProvisionPlanInput) (*service.ProvisionPlan, error) {
	res, err := r.sqlExec(ctx).ExecContext(ctx, `
UPDATE provision_plans
SET code = $2, name = $3, group_id = $4, balance = $5, quota = $6, expires_in_days = $7,
    rate_limit_5h = $8, rate_limit_1d = $9, rate_limit_7d = $10,
    concurrency = $11, rpm_limit = $12, enabled = $13, updated_at = NOW()
WHERE id = $1`, id, input.Code, input.Name, input.GroupID, input.Balance, input.Quota, nullableInt(input.ExpiresInDays),
		input.RateLimit5h, input.RateLimit1d, input.RateLimit7d, input.Concurrency, input.RPMLimit, input.Enabled)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrProvisionPlanNotFound, service.ErrProvisionPlanExists)
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return nil, service.ErrProvisionPlanNotFound
	}
	return r.GetPlanByID(ctx, id)
}

func (r *provisionRepository) DeletePlan(ctx context.Context, id int64) error {
	res, err := r.sqlExec(ctx).ExecContext(ctx, `DELETE FROM provision_plans WHERE id = $1`, id)
	if err != nil {
		return err
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return service.ErrProvisionPlanNotFound
	}
	return nil
}

func (r *provisionRepository) GetResultByOrderID(ctx context.Context, orderID string) (*service.ProvisionResult, error) {
	row := r.queryRow(ctx, `
SELECT o.status, o.plan_code, o.user_id, o.api_key_id, o.plan_snapshot, k.key
FROM provision_orders o
LEFT JOIN api_keys k ON k.id = o.api_key_id
WHERE o.order_id = $1`, orderID)

	var (
		status      string
		planCode    string
		userID      sql.NullInt64
		apiKeyID    sql.NullInt64
		snapshotRaw []byte
		apiKey      sql.NullString
	)
	if err := row.Scan(&status, &planCode, &userID, &apiKeyID, &snapshotRaw, &apiKey); err != nil {
		return nil, translatePersistenceError(err, service.ErrProvisionOrderNotFound, nil)
	}
	if status == service.ProvisionOrderStatusProcessing {
		return nil, service.ErrProvisionOrderProcessing
	}
	if status != service.ProvisionOrderStatusCompleted || !userID.Valid || !apiKeyID.Valid || !apiKey.Valid {
		return nil, service.ErrProvisionOrderIncomplete
	}
	var snapshot service.ProvisionSnapshot
	if len(snapshotRaw) > 0 {
		_ = json.Unmarshal(snapshotRaw, &snapshot)
	}
	return &service.ProvisionResult{
		OrderID:        orderID,
		APIKey:         apiKey.String,
		KeyID:          apiKeyID.Int64,
		UserID:         userID.Int64,
		PlanCode:       planCode,
		GroupID:        snapshot.GroupID,
		Balance:        snapshot.Balance,
		Quota:          snapshot.Quota,
		RateMultiplier: snapshot.RateMultiplier,
	}, nil
}

func (r *provisionRepository) CreateOrderProcessing(ctx context.Context, order *service.ProvisionOrder) (*service.ProvisionOrder, error) {
	snapshotRaw, err := json.Marshal(order.PlanSnapshot)
	if err != nil {
		return nil, err
	}
	row := r.queryRow(ctx, `
INSERT INTO provision_orders (order_id, plan_id, plan_code, plan_snapshot, status, customer_label)
VALUES ($1,$2,$3,$4,$5,$6)
RETURNING id, created_at, updated_at`, order.OrderID, nullableInt64(order.PlanID), order.PlanCode, snapshotRaw, service.ProvisionOrderStatusProcessing, order.CustomerLabel)
	if err := row.Scan(&order.ID, &order.CreatedAt, &order.UpdatedAt); err != nil {
		return nil, translatePersistenceError(err, nil, service.ErrProvisionOrderExists)
	}
	return order, nil
}

func (r *provisionRepository) CompleteOrder(ctx context.Context, orderID string, userID, apiKeyID int64, snapshot service.ProvisionSnapshot) error {
	snapshotRaw, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	res, err := r.sqlExec(ctx).ExecContext(ctx, `
UPDATE provision_orders
SET user_id = $2, api_key_id = $3, plan_snapshot = $4, status = $5, updated_at = NOW()
WHERE order_id = $1`, orderID, userID, apiKeyID, snapshotRaw, service.ProvisionOrderStatusCompleted)
	if err != nil {
		return err
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return service.ErrProvisionOrderNotFound
	}
	return nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

type provisionQueryRow struct {
	ctx   context.Context
	exec  sqlQueryExecutor
	query string
	args  []any
}

func (r *provisionQueryRow) Scan(dest ...any) error {
	rows, err := r.exec.QueryContext(r.ctx, r.query, r.args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return err
		}
		return sql.ErrNoRows
	}
	if err := rows.Scan(dest...); err != nil {
		return err
	}
	return rows.Err()
}

func nullableInt(v *int) any {
	if v == nil {
		return nil
	}
	return *v
}

func nullableInt64(v *int64) any {
	if v == nil {
		return nil
	}
	return *v
}

func scanProvisionPlan(row rowScanner) (*service.ProvisionPlan, error) {
	var (
		plan           service.ProvisionPlan
		expiresInDays  sql.NullInt64
		groupName      sql.NullString
		groupPlatform  sql.NullString
		groupRate      sql.NullFloat64
		groupExclusive sql.NullBool
		groupStatus    sql.NullString
		groupSubType   sql.NullString
	)
	err := row.Scan(
		&plan.ID, &plan.Code, &plan.Name, &plan.GroupID, &plan.Balance, &plan.Quota, &expiresInDays,
		&plan.RateLimit5h, &plan.RateLimit1d, &plan.RateLimit7d, &plan.Concurrency, &plan.RPMLimit,
		&plan.Enabled, &plan.CreatedAt, &plan.UpdatedAt,
		&groupName, &groupPlatform, &groupRate, &groupExclusive, &groupStatus, &groupSubType,
	)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrProvisionPlanNotFound, nil)
	}
	if expiresInDays.Valid {
		v := int(expiresInDays.Int64)
		plan.ExpiresInDays = &v
	}
	if groupName.Valid {
		plan.Group = &service.Group{
			ID:               plan.GroupID,
			Name:             groupName.String,
			Platform:         groupPlatform.String,
			RateMultiplier:   groupRate.Float64,
			IsExclusive:      groupExclusive.Bool,
			Status:           groupStatus.String,
			SubscriptionType: groupSubType.String,
		}
	}
	return &plan, nil
}
