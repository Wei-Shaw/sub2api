package service

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"entgo.io/ent/dialect"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

const subscriptionDailyResetDeduction = 24 * time.Hour

var (
	ErrSubscriptionResetStorageUnavailable = infraerrors.ServiceUnavailable("SUBSCRIPTION_RESET_STORAGE_UNAVAILABLE", "subscription reset storage is not available")
	ErrSubscriptionResetInsufficientTime   = infraerrors.BadRequest("SUBSCRIPTION_RESET_INSUFFICIENT_TIME", "subscription must have at least 24 hours remaining")
	ErrSubscriptionDailyResetDisabled      = infraerrors.Forbidden("SUBSCRIPTION_DAILY_RESET_DISABLED", "subscription daily reset is disabled")
)

type SubscriptionResetAudit struct {
	ID                     int64      `json:"id"`
	SubscriptionID         int64      `json:"subscription_id"`
	UserID                 int64      `json:"user_id"`
	GroupID                int64      `json:"group_id"`
	OperatorID             int64      `json:"operator_id"`
	OperatorType           string     `json:"operator_type"`
	DeductedSeconds        int        `json:"deducted_seconds"`
	BeforeExpiresAt        time.Time  `json:"before_expires_at"`
	AfterExpiresAt         time.Time  `json:"after_expires_at"`
	BeforeDailyUsageUSD    float64    `json:"before_daily_usage_usd"`
	AfterDailyUsageUSD     float64    `json:"after_daily_usage_usd"`
	BeforeDailyWindowStart *time.Time `json:"before_daily_window_start,omitempty"`
	AfterDailyWindowStart  *time.Time `json:"after_daily_window_start,omitempty"`
	CreatedAt              time.Time  `json:"created_at"`
}

type SubscriptionResetAuditFilter struct {
	Pagination     pagination.PaginationParams
	SubscriptionID *int64
	UserID         *int64
	GroupID        *int64
	OperatorID     *int64
}

type lockedSubscriptionForDailyReset struct {
	ID               int64
	UserID           int64
	GroupID          int64
	Status           string
	ExpiresAt        time.Time
	DailyUsageUSD    float64
	DailyWindowStart *time.Time
}

func (s *SubscriptionService) ResetDailyUsageWithTimeDeduction(ctx context.Context, userID, subscriptionID int64) (*UserSubscription, error) {
	if s.entClient == nil {
		return nil, ErrSubscriptionResetStorageUnavailable
	}
	if !s.isSubscriptionDailyResetEnabled(ctx) {
		return nil, ErrSubscriptionDailyResetDisabled
	}

	now := time.Now()
	newExpiresAt := time.Time{}
	groupID := int64(0)

	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin subscription reset transaction: %w", err)
	}
	txCtx := dbent.NewTxContext(ctx, tx)
	client := tx.Client()

	sub, err := loadSubscriptionForDailyReset(txCtx, client, subscriptionID)
	if err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	if sub.UserID != userID {
		_ = tx.Rollback()
		return nil, ErrSubscriptionNotFound
	}
	if sub.Status != SubscriptionStatusActive {
		_ = tx.Rollback()
		return nil, ErrSubscriptionSuspended
	}
	if !sub.ExpiresAt.After(now) {
		_ = tx.Rollback()
		return nil, ErrSubscriptionExpired
	}
	if sub.ExpiresAt.Before(now.Add(subscriptionDailyResetDeduction)) {
		_ = tx.Rollback()
		return nil, ErrSubscriptionResetInsufficientTime
	}

	newExpiresAt = sub.ExpiresAt.Add(-subscriptionDailyResetDeduction)
	groupID = sub.GroupID

	if _, err := client.ExecContext(txCtx, `
		UPDATE user_subscriptions
		SET expires_at = $2,
			daily_usage_usd = 0,
			daily_window_start = $3,
			updated_at = $3
		WHERE id = $1
	`, sub.ID, newExpiresAt, now); err != nil {
		_ = tx.Rollback()
		return nil, fmt.Errorf("update subscription daily reset: %w", err)
	}

	if _, err := client.ExecContext(txCtx, `
		INSERT INTO subscription_reset_audits (
			subscription_id,
			user_id,
			group_id,
			operator_id,
			operator_type,
			deducted_seconds,
			before_expires_at,
			after_expires_at,
			before_daily_usage_usd,
			after_daily_usage_usd,
			before_daily_window_start,
			after_daily_window_start,
			created_at
		) VALUES (
			$1, $2, $3, $4, 'user', $5, $6, $7, $8, 0, $9, $10, $10
		)
	`, sub.ID, sub.UserID, sub.GroupID, userID, int(subscriptionDailyResetDeduction.Seconds()), sub.ExpiresAt, newExpiresAt, sub.DailyUsageUSD, sub.DailyWindowStart, now); err != nil {
		_ = tx.Rollback()
		return nil, fmt.Errorf("insert subscription reset audit: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit subscription reset transaction: %w", err)
	}

	s.InvalidateSubCache(userID, groupID)
	if s.subCacheL1 != nil {
		s.subCacheL1.Wait()
	}
	if s.billingCacheService != nil {
		_ = s.billingCacheService.InvalidateSubscription(ctx, userID, groupID)
	}

	return s.userSubRepo.GetByID(ctx, subscriptionID)
}

func (s *SubscriptionService) isSubscriptionDailyResetEnabled(ctx context.Context) bool {
	if s == nil || s.settingRepo == nil {
		return true
	}
	value, err := s.settingRepo.GetValue(ctx, SettingKeySubscriptionDailyResetEnabled)
	if err != nil {
		return true
	}
	return !isFalseSettingValue(value)
}

func (s *SubscriptionService) ListSubscriptionResetAudits(ctx context.Context, filter SubscriptionResetAuditFilter) ([]SubscriptionResetAudit, *pagination.PaginationResult, error) {
	if s.entClient == nil {
		return nil, nil, ErrSubscriptionResetStorageUnavailable
	}

	params := filter.Pagination
	if params.Page <= 0 {
		params.Page = 1
	}
	if params.PageSize <= 0 {
		params.PageSize = 20
	}

	where, args := subscriptionResetAuditWhere(filter)
	countQuery := "SELECT COUNT(*) FROM subscription_reset_audits" + where
	rows, err := s.entClient.QueryContext(ctx, countQuery, args...)
	if err != nil {
		return nil, nil, fmt.Errorf("count subscription reset audits: %w", err)
	}
	total, err := scanSingleInt64(rows)
	if err != nil {
		return nil, nil, err
	}

	queryArgs := append([]any{}, args...)
	limitArg := len(queryArgs) + 1
	offsetArg := len(queryArgs) + 2
	queryArgs = append(queryArgs, params.Limit(), params.Offset())
	query := fmt.Sprintf(`
		SELECT
			id,
			subscription_id,
			user_id,
			group_id,
			operator_id,
			operator_type,
			deducted_seconds,
			before_expires_at,
			after_expires_at,
			before_daily_usage_usd,
			after_daily_usage_usd,
			before_daily_window_start,
			after_daily_window_start,
			created_at
		FROM subscription_reset_audits
		%s
		ORDER BY created_at DESC, id DESC
		LIMIT $%d OFFSET $%d
	`, where, limitArg, offsetArg)

	rows, err = s.entClient.QueryContext(ctx, query, queryArgs...)
	if err != nil {
		return nil, nil, fmt.Errorf("list subscription reset audits: %w", err)
	}
	defer rows.Close()

	items := make([]SubscriptionResetAudit, 0, params.Limit())
	for rows.Next() {
		item, err := scanSubscriptionResetAudit(rows)
		if err != nil {
			return nil, nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	return items, subscriptionResetPagination(total, params), nil
}

func loadSubscriptionForDailyReset(ctx context.Context, client *dbent.Client, subscriptionID int64) (*lockedSubscriptionForDailyReset, error) {
	rows, err := client.QueryContext(ctx, `
		SELECT id, user_id, group_id, status, expires_at, daily_usage_usd, daily_window_start
		FROM user_subscriptions
		WHERE id = $1
			AND deleted_at IS NULL
		`+subscriptionResetLockSuffix(client), subscriptionID)
	if err != nil {
		return nil, fmt.Errorf("lock subscription for daily reset: %w", err)
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, ErrSubscriptionNotFound
	}

	var out lockedSubscriptionForDailyReset
	var dailyWindowStart sql.NullTime
	if err := rows.Scan(&out.ID, &out.UserID, &out.GroupID, &out.Status, &out.ExpiresAt, &out.DailyUsageUSD, &dailyWindowStart); err != nil {
		return nil, err
	}
	if dailyWindowStart.Valid {
		out.DailyWindowStart = &dailyWindowStart.Time
	}
	return &out, nil
}

func subscriptionResetLockSuffix(client *dbent.Client) string {
	if client != nil && client.Driver() != nil && client.Driver().Dialect() == dialect.Postgres {
		return "FOR UPDATE"
	}
	return ""
}

func subscriptionResetAuditWhere(filter SubscriptionResetAuditFilter) (string, []any) {
	clauses := make([]string, 0, 4)
	args := make([]any, 0, 4)
	add := func(field string, value int64) {
		args = append(args, value)
		clauses = append(clauses, fmt.Sprintf("%s = $%d", field, len(args)))
	}

	if filter.SubscriptionID != nil {
		add("subscription_id", *filter.SubscriptionID)
	}
	if filter.UserID != nil {
		add("user_id", *filter.UserID)
	}
	if filter.GroupID != nil {
		add("group_id", *filter.GroupID)
	}
	if filter.OperatorID != nil {
		add("operator_id", *filter.OperatorID)
	}
	if len(clauses) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(clauses, " AND "), args
}

func scanSingleInt64(rows *sql.Rows) (int64, error) {
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return 0, err
		}
		return 0, nil
	}
	var out int64
	if err := rows.Scan(&out); err != nil {
		return 0, err
	}
	return out, rows.Err()
}

func scanSubscriptionResetAudit(rows *sql.Rows) (SubscriptionResetAudit, error) {
	var out SubscriptionResetAudit
	var beforeWindow sql.NullTime
	var afterWindow sql.NullTime
	if err := rows.Scan(
		&out.ID,
		&out.SubscriptionID,
		&out.UserID,
		&out.GroupID,
		&out.OperatorID,
		&out.OperatorType,
		&out.DeductedSeconds,
		&out.BeforeExpiresAt,
		&out.AfterExpiresAt,
		&out.BeforeDailyUsageUSD,
		&out.AfterDailyUsageUSD,
		&beforeWindow,
		&afterWindow,
		&out.CreatedAt,
	); err != nil {
		return out, err
	}
	if beforeWindow.Valid {
		out.BeforeDailyWindowStart = &beforeWindow.Time
	}
	if afterWindow.Valid {
		out.AfterDailyWindowStart = &afterWindow.Time
	}
	return out, nil
}

func subscriptionResetPagination(total int64, params pagination.PaginationParams) *pagination.PaginationResult {
	pageSize := params.Limit()
	pages := int((total + int64(pageSize) - 1) / int64(pageSize))
	if pages < 1 {
		pages = 1
	}
	page := params.Page
	if page < 1 {
		page = 1
	}
	return &pagination.PaginationResult{
		Total:    total,
		Page:     page,
		PageSize: pageSize,
		Pages:    pages,
	}
}
