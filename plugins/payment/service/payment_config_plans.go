package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"

	"github.com/shopspring/decimal"
)

// errPlansDBUnavailable is returned by every plan read when the SDK has
// not been wired with a *sql.DB (host older than V5 or test harness).
var errPlansDBUnavailable = errors.New("payment: plan database connection unavailable")

// scanPlanRow scans a single row from subscription_plans into a SubscriptionPlan.
// Column order MUST match the SELECT statement in queryPlans.
//
// price / original_price come from the host's DECIMAL columns; we read them
// into decimal.Decimal directly via shopspring/decimal's sql.Scanner so
// the conversion stays exact.
func scanPlanRow(rows *sql.Rows) (*SubscriptionPlan, error) {
	var p SubscriptionPlan
	var originalPrice decimal.NullDecimal
	if err := rows.Scan(
		&p.ID,
		&p.GroupID,
		&p.Name,
		&p.Description,
		&p.Price,
		&originalPrice,
		&p.ValidityDays,
		&p.ValidityUnit,
		&p.Features,
		&p.ProductName,
		&p.ForSale,
		&p.SortOrder,
	); err != nil {
		return nil, err
	}
	if originalPrice.Valid {
		v := originalPrice.Decimal
		p.OriginalPrice = &v
	}
	return &p, nil
}

const planSelectColumns = "id, group_id, name, description, price, original_price, validity_days, validity_unit, features, product_name, for_sale, sort_order"

// queryPlans is a small helper that runs an arbitrary WHERE clause against
// subscription_plans and returns the resulting slice.
func (s *PaymentConfigService) queryPlans(ctx context.Context, where string, args ...interface{}) ([]*SubscriptionPlan, error) {
	if s == nil || s.db == nil {
		return nil, errPlansDBUnavailable
	}
	q := fmt.Sprintf("SELECT %s FROM subscription_plans", planSelectColumns)
	if where != "" {
		q += " " + where
	}
	q += " ORDER BY sort_order ASC, id ASC"
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*SubscriptionPlan
	for rows.Next() {
		p, err := scanPlanRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// ListPlans returns every subscription plan known to the host.
func (s *PaymentConfigService) ListPlans(ctx context.Context) ([]*SubscriptionPlan, error) {
	return s.queryPlans(ctx, "")
}

// ListPlansForSale returns plans whose for_sale flag is true.
func (s *PaymentConfigService) ListPlansForSale(ctx context.Context) ([]*SubscriptionPlan, error) {
	return s.queryPlans(ctx, "WHERE for_sale = true")
}

// GetPlan returns a single plan by id.
func (s *PaymentConfigService) GetPlan(ctx context.Context, id int64) (*SubscriptionPlan, error) {
	if s == nil || s.db == nil {
		return nil, errPlansDBUnavailable
	}
	q := fmt.Sprintf("SELECT %s FROM subscription_plans WHERE id = $1 LIMIT 1", planSelectColumns)
	rows, err := s.db.QueryContext(ctx, q, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, sql.ErrNoRows
	}
	return scanPlanRow(rows)
}

// CreatePlan, UpdatePlan, DeletePlan are not supported because the plugin
// holds no DB.write capability for the host's subscription_plans table.
func (s *PaymentConfigService) CreatePlan(_ context.Context, _ CreatePlanRequest) (*SubscriptionPlan, error) {
	return nil, errPlanWriteNotSupported
}

func (s *PaymentConfigService) UpdatePlan(_ context.Context, _ int64, _ UpdatePlanRequest) (*SubscriptionPlan, error) {
	return nil, errPlanWriteNotSupported
}

func (s *PaymentConfigService) DeletePlan(_ context.Context, _ int64) error {
	return errPlanWriteNotSupported
}

// GetGroupPlatformMap returns a group-id -> platform-string map for the
// supplied plans, used by the user-facing /plans response to colour-code
// cards by platform (sora, antigravity, etc.).
func (s *PaymentConfigService) GetGroupPlatformMap(ctx context.Context, plans []*SubscriptionPlan) map[int64]string {
	out := map[int64]string{}
	if s == nil || s.db == nil || len(plans) == 0 {
		return out
	}
	ids := uniqueGroupIDs(plans)
	if len(ids) == 0 {
		return out
	}
	rows, err := s.queryGroupPlatforms(ctx, ids)
	if err != nil {
		if s.logger != nil {
			s.logger.Warn("payment: GetGroupPlatformMap failed", "error", err)
		}
		return out
	}
	return rows
}

// GetGroupInfoMap returns a richer group-id -> metadata map than
// GetGroupPlatformMap.
func (s *PaymentConfigService) GetGroupInfoMap(ctx context.Context, plans []*SubscriptionPlan) map[int64]PlanGroupInfo {
	out := map[int64]PlanGroupInfo{}
	if s == nil || s.db == nil || len(plans) == 0 {
		return out
	}
	ids := uniqueGroupIDs(plans)
	if len(ids) == 0 {
		return out
	}
	info, err := s.queryGroupInfo(ctx, ids)
	if err != nil {
		if s.logger != nil {
			s.logger.Warn("payment: GetGroupInfoMap failed", "error", err)
		}
		return out
	}
	return info
}

// uniqueGroupIDs collects distinct group IDs from a plan slice.
func uniqueGroupIDs(plans []*SubscriptionPlan) []int64 {
	seen := map[int64]struct{}{}
	out := make([]int64, 0, len(plans))
	for _, p := range plans {
		if p == nil {
			continue
		}
		if _, ok := seen[p.GroupID]; ok {
			continue
		}
		seen[p.GroupID] = struct{}{}
		out = append(out, p.GroupID)
	}
	return out
}

// queryGroupPlatforms runs SELECT id, platform FROM groups WHERE id IN (...).
func (s *PaymentConfigService) queryGroupPlatforms(ctx context.Context, ids []int64) (map[int64]string, error) {
	out := map[int64]string{}
	query, args, err := buildInClause("SELECT id, platform FROM groups WHERE id IN", ids)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var platform string
		if err := rows.Scan(&id, &platform); err != nil {
			return nil, err
		}
		out[id] = platform
	}
	return out, rows.Err()
}

// queryGroupInfo loads (id, platform, name) for the supplied group IDs.
func (s *PaymentConfigService) queryGroupInfo(ctx context.Context, ids []int64) (map[int64]PlanGroupInfo, error) {
	out := map[int64]PlanGroupInfo{}
	query, args, err := buildInClause("SELECT id, platform, name FROM groups WHERE id IN", ids)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var info PlanGroupInfo
		if err := rows.Scan(&info.GroupID, &info.Platform, &info.Name); err != nil {
			return nil, err
		}
		out[info.GroupID] = info
	}
	return out, rows.Err()
}

// buildInClause renders a "(... IN ($1, $2, ...))" suffix with positional
// placeholders. Returns the full query string and the args slice.
func buildInClause(prefix string, ids []int64) (string, []interface{}, error) {
	if len(ids) == 0 {
		return "", nil, errors.New("buildInClause: empty ids")
	}
	parts := make([]string, 0, len(ids))
	args := make([]interface{}, 0, len(ids))
	for i, id := range ids {
		parts = append(parts, fmt.Sprintf("$%d", i+1))
		args = append(args, id)
		_ = i
	}
	q := prefix + " (" + joinComma(parts) + ")"
	return q, args, nil
}

func joinComma(parts []string) string {
	if len(parts) == 0 {
		return ""
	}
	out := parts[0]
	for _, p := range parts[1:] {
		out += "," + p
	}
	return out
}

// validatePlanRequired performs the body-level validation of CreatePlan.
func validatePlanRequired(name string, groupID int64, price decimal.Decimal, validityDays int, validityUnit string, originalPrice *decimal.Decimal) error {
	if name == "" {
		return errors.New("name required")
	}
	if groupID == 0 {
		return errors.New("group_id required")
	}
	if price.IsNegative() {
		return errors.New("price must be non-negative")
	}
	if validityDays <= 0 {
		return errors.New("validity_days must be positive")
	}
	if validityUnit == "" {
		return errors.New("validity_unit required")
	}
	if originalPrice != nil && originalPrice.IsNegative() {
		return errors.New("original_price must be non-negative")
	}
	return nil
}

// validatePlanPatch is the analogue for UpdatePlan.
func validatePlanPatch(req UpdatePlanRequest) error {
	if req.Price != nil && req.Price.IsNegative() {
		return errors.New("price must be non-negative")
	}
	if req.OriginalPrice != nil && req.OriginalPrice.IsNegative() {
		return errors.New("original_price must be non-negative")
	}
	if req.ValidityDays != nil && *req.ValidityDays <= 0 {
		return errors.New("validity_days must be positive")
	}
	return nil
}

// planMigrationFootprint keeps a logger-friendly debug helper.
func planMigrationFootprint(logger *slog.Logger) {
	if logger == nil {
		return
	}
	logger.Debug("payment plan migration footprint")
}

// hostDB exposes the *sql.DB held by the service.
func (s *PaymentConfigService) hostDB() *sql.DB {
	if s == nil {
		return nil
	}
	return s.db
}
