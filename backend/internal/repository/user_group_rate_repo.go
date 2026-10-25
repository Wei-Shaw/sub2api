package repository

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

type userGroupRateRepository struct {
	sql sqlExecutor
}

// NewUserGroupRateRepository 创建用户专属分组倍率/RPM 仓储
func NewUserGroupRateRepository(sqlDB *sql.DB) service.UserGroupRateRepository {
	return &userGroupRateRepository{sql: sqlDB}
}

// GetByUserID 获取用户所有专属分组 rate_multiplier（仅返回非 NULL 的条目）
func (r *userGroupRateRepository) GetByUserID(ctx context.Context, userID int64) (map[int64]float64, error) {
	query := `SELECT group_id, rate_multiplier FROM user_group_rate_multipliers WHERE user_id = $1 AND rate_multiplier IS NOT NULL`
	if IsMySQL() {
		query = `SELECT group_id, rate_multiplier FROM user_group_rate_multipliers WHERE user_id = ? AND rate_multiplier IS NOT NULL`
	}
	rows, err := r.sql.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	result := make(map[int64]float64)
	for rows.Next() {
		var groupID int64
		var rate float64
		if err := rows.Scan(&groupID, &rate); err != nil {
			return nil, err
		}
		result[groupID] = rate
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

// GetByUserIDs 批量获取多个用户的专属分组 rate_multiplier（仅返回非 NULL 的条目）
func (r *userGroupRateRepository) GetByUserIDs(ctx context.Context, userIDs []int64) (map[int64]map[int64]float64, error) {
	result := make(map[int64]map[int64]float64, len(userIDs))
	if len(userIDs) == 0 {
		return result, nil
	}

	uniqueIDs := make([]int64, 0, len(userIDs))
	seen := make(map[int64]struct{}, len(userIDs))
	for _, userID := range userIDs {
		if userID <= 0 {
			continue
		}
		if _, exists := seen[userID]; exists {
			continue
		}
		seen[userID] = struct{}{}
		uniqueIDs = append(uniqueIDs, userID)
		result[userID] = make(map[int64]float64)
	}
	if len(uniqueIDs) == 0 {
		return result, nil
	}
	if IsMySQL() {
		return r.getByUserIDsMySQL(ctx, uniqueIDs, result)
	}

	rows, err := r.sql.QueryContext(ctx, `
		SELECT user_id, group_id, rate_multiplier
		FROM user_group_rate_multipliers
		WHERE user_id = ANY($1) AND rate_multiplier IS NOT NULL
	`, pq.Array(uniqueIDs))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var userID int64
		var groupID int64
		var rate float64
		if err := rows.Scan(&userID, &groupID, &rate); err != nil {
			return nil, err
		}
		if _, ok := result[userID]; !ok {
			result[userID] = make(map[int64]float64)
		}
		result[userID][groupID] = rate
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

// GetByGroupID 获取指定分组下所有用户的专属配置（rate 与 rpm_override 任一非 NULL 即返回）
func (r *userGroupRateRepository) GetByGroupID(ctx context.Context, groupID int64) ([]service.UserGroupRateEntry, error) {
	query := `
		SELECT ugr.user_id, u.username, u.email, COALESCE(u.notes, ''), u.status, ugr.rate_multiplier, ugr.rpm_override
		FROM user_group_rate_multipliers ugr
		JOIN users u ON u.id = ugr.user_id AND u.deleted_at IS NULL
		WHERE ugr.group_id = $1
		ORDER BY ugr.user_id
	`
	if IsMySQL() {
		query = `
		SELECT ugr.user_id, u.username, u.email, COALESCE(u.notes, ''), u.status, ugr.rate_multiplier, ugr.rpm_override
		FROM user_group_rate_multipliers ugr
		JOIN users u ON u.id = ugr.user_id AND u.deleted_at IS NULL
		WHERE ugr.group_id = ?
		ORDER BY ugr.user_id
	`
	}
	rows, err := r.sql.QueryContext(ctx, query, groupID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var result []service.UserGroupRateEntry
	for rows.Next() {
		var entry service.UserGroupRateEntry
		var rate sql.NullFloat64
		var rpm sql.NullInt32
		if err := rows.Scan(&entry.UserID, &entry.UserName, &entry.UserEmail, &entry.UserNotes, &entry.UserStatus, &rate, &rpm); err != nil {
			return nil, err
		}
		if rate.Valid {
			v := rate.Float64
			entry.RateMultiplier = &v
		}
		if rpm.Valid {
			v := int(rpm.Int32)
			entry.RPMOverride = &v
		}
		result = append(result, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

// GetByUserAndGroup 获取用户在特定分组的专属 rate_multiplier（NULL 返回 nil）
func (r *userGroupRateRepository) GetByUserAndGroup(ctx context.Context, userID, groupID int64) (*float64, error) {
	query := `SELECT rate_multiplier FROM user_group_rate_multipliers WHERE user_id = $1 AND group_id = $2`
	if IsMySQL() {
		query = `SELECT rate_multiplier FROM user_group_rate_multipliers WHERE user_id = ? AND group_id = ?`
	}
	var rate sql.NullFloat64
	err := scanSingleRow(ctx, r.sql, query, []any{userID, groupID}, &rate)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !rate.Valid {
		return nil, nil
	}
	v := rate.Float64
	return &v, nil
}

// GetRPMOverrideByUserAndGroup 获取用户在特定分组的 rpm_override（NULL 返回 nil）
func (r *userGroupRateRepository) GetRPMOverrideByUserAndGroup(ctx context.Context, userID, groupID int64) (*int, error) {
	query := `SELECT rpm_override FROM user_group_rate_multipliers WHERE user_id = $1 AND group_id = $2`
	if IsMySQL() {
		query = `SELECT rpm_override FROM user_group_rate_multipliers WHERE user_id = ? AND group_id = ?`
	}
	var rpm sql.NullInt32
	err := scanSingleRow(ctx, r.sql, query, []any{userID, groupID}, &rpm)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !rpm.Valid {
		return nil, nil
	}
	v := int(rpm.Int32)
	return &v, nil
}

// SyncUserGroupRates 同步用户的分组专属 rate_multiplier。
//   - 传入空 map：清空该用户所有行的 rate_multiplier；若 rpm_override 也为 NULL 则整行删除。
//   - 值为 nil：清空对应行的 rate_multiplier（保留 rpm_override）。
//   - 值非 nil：upsert rate_multiplier（保留已有 rpm_override）。
func (r *userGroupRateRepository) SyncUserGroupRates(ctx context.Context, userID int64, rates map[int64]*float64) error {
	if IsMySQL() {
		return r.syncUserGroupRatesMySQL(ctx, userID, rates)
	}
	if len(rates) == 0 {
		if _, err := r.sql.ExecContext(ctx, `
			UPDATE user_group_rate_multipliers
			SET rate_multiplier = NULL, updated_at = NOW()
			WHERE user_id = $1
		`, userID); err != nil {
			return err
		}
		_, err := r.sql.ExecContext(ctx,
			`DELETE FROM user_group_rate_multipliers WHERE user_id = $1 AND rate_multiplier IS NULL AND rpm_override IS NULL`,
			userID)
		return err
	}

	var clearGroupIDs []int64
	upsertGroupIDs := make([]int64, 0, len(rates))
	upsertRates := make([]float64, 0, len(rates))
	for groupID, rate := range rates {
		if rate == nil {
			clearGroupIDs = append(clearGroupIDs, groupID)
		} else {
			upsertGroupIDs = append(upsertGroupIDs, groupID)
			upsertRates = append(upsertRates, *rate)
		}
	}

	if len(clearGroupIDs) > 0 {
		if _, err := r.sql.ExecContext(ctx, `
			UPDATE user_group_rate_multipliers
			SET rate_multiplier = NULL, updated_at = NOW()
			WHERE user_id = $1 AND group_id = ANY($2)
		`, userID, pq.Array(clearGroupIDs)); err != nil {
			return err
		}
		if _, err := r.sql.ExecContext(ctx,
			`DELETE FROM user_group_rate_multipliers WHERE user_id = $1 AND group_id = ANY($2) AND rate_multiplier IS NULL AND rpm_override IS NULL`,
			userID, pq.Array(clearGroupIDs)); err != nil {
			return err
		}
	}

	if len(upsertGroupIDs) > 0 {
		now := time.Now()
		_, err := r.sql.ExecContext(ctx, `
			INSERT INTO user_group_rate_multipliers (user_id, group_id, rate_multiplier, created_at, updated_at)
			SELECT
				$1::bigint,
				data.group_id,
				data.rate_multiplier,
				$2::timestamptz,
				$2::timestamptz
			FROM unnest($3::bigint[], $4::double precision[]) AS data(group_id, rate_multiplier)
			ON CONFLICT (user_id, group_id)
			DO UPDATE SET
				rate_multiplier = EXCLUDED.rate_multiplier,
				updated_at = EXCLUDED.updated_at
		`, userID, now, pq.Array(upsertGroupIDs), pq.Array(upsertRates))
		if err != nil {
			return err
		}
	}

	return nil
}

// SyncGroupRateMultipliers 同步分组的 rate_multiplier 部分（不触动 rpm_override）。
// 语义：
//   - 未出现在 entries 中的用户行：rate_multiplier 归 NULL；若 rpm_override 也为 NULL 则整行删除。
//   - 出现的用户行：upsert rate_multiplier。
func (r *userGroupRateRepository) SyncGroupRateMultipliers(ctx context.Context, groupID int64, entries []service.GroupRateMultiplierInput) error {
	if IsMySQL() {
		return r.syncGroupRateMultipliersMySQL(ctx, groupID, entries)
	}
	keepUserIDs := make([]int64, 0, len(entries))
	for _, e := range entries {
		keepUserIDs = append(keepUserIDs, e.UserID)
	}

	// 未在 entries 列表中的行：清空 rate_multiplier。
	if len(keepUserIDs) == 0 {
		if _, err := r.sql.ExecContext(ctx, `
			UPDATE user_group_rate_multipliers
			SET rate_multiplier = NULL, updated_at = NOW()
			WHERE group_id = $1
		`, groupID); err != nil {
			return err
		}
	} else {
		if _, err := r.sql.ExecContext(ctx, `
			UPDATE user_group_rate_multipliers
			SET rate_multiplier = NULL, updated_at = NOW()
			WHERE group_id = $1 AND user_id <> ALL($2)
		`, groupID, pq.Array(keepUserIDs)); err != nil {
			return err
		}
	}

	// 清空后若整行 NULL 则删除。
	if _, err := r.sql.ExecContext(ctx, `
		DELETE FROM user_group_rate_multipliers
		WHERE group_id = $1 AND rate_multiplier IS NULL AND rpm_override IS NULL
	`, groupID); err != nil {
		return err
	}

	if len(entries) == 0 {
		return nil
	}

	userIDs := make([]int64, len(entries))
	rates := make([]float64, len(entries))
	for i, e := range entries {
		userIDs[i] = e.UserID
		rates[i] = e.RateMultiplier
	}
	now := time.Now()
	_, err := r.sql.ExecContext(ctx, `
		INSERT INTO user_group_rate_multipliers (user_id, group_id, rate_multiplier, created_at, updated_at)
		SELECT data.user_id, $1::bigint, data.rate_multiplier, $2::timestamptz, $2::timestamptz
		FROM unnest($3::bigint[], $4::double precision[]) AS data(user_id, rate_multiplier)
		ON CONFLICT (user_id, group_id)
		DO UPDATE SET rate_multiplier = EXCLUDED.rate_multiplier, updated_at = EXCLUDED.updated_at
	`, groupID, now, pq.Array(userIDs), pq.Array(rates))
	return err
}

// SyncGroupRPMOverrides 同步分组的 rpm_override 部分（不触动 rate_multiplier）。
// 语义：
//   - 未出现的用户行：rpm_override 归 NULL；若 rate_multiplier 也为 NULL 则整行删除。
//   - 出现的用户行：若 RPMOverride 为 nil 则清空；非 nil 则 upsert。
func (r *userGroupRateRepository) SyncGroupRPMOverrides(ctx context.Context, groupID int64, entries []service.GroupRPMOverrideInput) error {
	if IsMySQL() {
		return r.syncGroupRPMOverridesMySQL(ctx, groupID, entries)
	}
	keepUserIDs := make([]int64, 0, len(entries))
	var clearUserIDs []int64
	upsertUserIDs := make([]int64, 0, len(entries))
	upsertValues := make([]int32, 0, len(entries))
	for _, e := range entries {
		keepUserIDs = append(keepUserIDs, e.UserID)
		if e.RPMOverride == nil {
			clearUserIDs = append(clearUserIDs, e.UserID)
		} else {
			upsertUserIDs = append(upsertUserIDs, e.UserID)
			upsertValues = append(upsertValues, int32(*e.RPMOverride))
		}
	}

	// 未在 entries 列表中的行：清空 rpm_override。
	if len(keepUserIDs) == 0 {
		if _, err := r.sql.ExecContext(ctx, `
			UPDATE user_group_rate_multipliers
			SET rpm_override = NULL, updated_at = NOW()
			WHERE group_id = $1
		`, groupID); err != nil {
			return err
		}
	} else {
		if _, err := r.sql.ExecContext(ctx, `
			UPDATE user_group_rate_multipliers
			SET rpm_override = NULL, updated_at = NOW()
			WHERE group_id = $1 AND user_id <> ALL($2)
		`, groupID, pq.Array(keepUserIDs)); err != nil {
			return err
		}
	}

	// 显式 clear 的行。
	if len(clearUserIDs) > 0 {
		if _, err := r.sql.ExecContext(ctx, `
			UPDATE user_group_rate_multipliers
			SET rpm_override = NULL, updated_at = NOW()
			WHERE group_id = $1 AND user_id = ANY($2)
		`, groupID, pq.Array(clearUserIDs)); err != nil {
			return err
		}
	}

	// 清空后若整行 NULL 则删除。
	if _, err := r.sql.ExecContext(ctx, `
		DELETE FROM user_group_rate_multipliers
		WHERE group_id = $1 AND rate_multiplier IS NULL AND rpm_override IS NULL
	`, groupID); err != nil {
		return err
	}

	if len(upsertUserIDs) > 0 {
		now := time.Now()
		_, err := r.sql.ExecContext(ctx, `
			INSERT INTO user_group_rate_multipliers (user_id, group_id, rpm_override, created_at, updated_at)
			SELECT data.user_id, $1::bigint, data.rpm_override, $2::timestamptz, $2::timestamptz
			FROM unnest($3::bigint[], $4::integer[]) AS data(user_id, rpm_override)
			ON CONFLICT (user_id, group_id)
			DO UPDATE SET rpm_override = EXCLUDED.rpm_override, updated_at = EXCLUDED.updated_at
		`, groupID, now, pq.Array(upsertUserIDs), pq.Array(upsertValues))
		if err != nil {
			return err
		}
	}

	return nil
}

// ClearGroupRPMOverrides 清空指定分组所有行的 rpm_override。
func (r *userGroupRateRepository) ClearGroupRPMOverrides(ctx context.Context, groupID int64) error {
	updateQuery := `
		UPDATE user_group_rate_multipliers
		SET rpm_override = NULL, updated_at = NOW()
		WHERE group_id = $1
	`
	if IsMySQL() {
		updateQuery = `
		UPDATE user_group_rate_multipliers
		SET rpm_override = NULL, updated_at = NOW(3)
		WHERE group_id = ?
	`
	}
	if _, err := r.sql.ExecContext(ctx, updateQuery, groupID); err != nil {
		return err
	}
	deleteQuery := `
		DELETE FROM user_group_rate_multipliers
		WHERE group_id = $1 AND rate_multiplier IS NULL AND rpm_override IS NULL
	`
	if IsMySQL() {
		deleteQuery = `
		DELETE FROM user_group_rate_multipliers
		WHERE group_id = ? AND rate_multiplier IS NULL AND rpm_override IS NULL
	`
	}
	_, err := r.sql.ExecContext(ctx, deleteQuery, groupID)
	return err
}

// DeleteByGroupID 删除指定分组的所有用户专属条目
func (r *userGroupRateRepository) DeleteByGroupID(ctx context.Context, groupID int64) error {
	query := `DELETE FROM user_group_rate_multipliers WHERE group_id = $1`
	if IsMySQL() {
		query = `DELETE FROM user_group_rate_multipliers WHERE group_id = ?`
	}
	_, err := r.sql.ExecContext(ctx, query, groupID)
	return err
}

// DeleteByUserID 删除指定用户的所有专属条目
func (r *userGroupRateRepository) DeleteByUserID(ctx context.Context, userID int64) error {
	query := `DELETE FROM user_group_rate_multipliers WHERE user_id = $1`
	if IsMySQL() {
		query = `DELETE FROM user_group_rate_multipliers WHERE user_id = ?`
	}
	_, err := r.sql.ExecContext(ctx, query, userID)
	return err
}

// ---------------------------------------------------------------------------
// MySQL 方言实现（见 DIALECT_MYSQL_GUIDE.md）
// ---------------------------------------------------------------------------

// getByUserIDsMySQL 是 GetByUserIDs 的 MySQL 实现：
// `user_id = ANY($1)` + pq.Array → IN (?,?,...) 展开，其余一致。
// result 已由调用方为每个 uniqueID 预置空 map。
func (r *userGroupRateRepository) getByUserIDsMySQL(ctx context.Context, uniqueIDs []int64, result map[int64]map[int64]float64) (map[int64]map[int64]float64, error) {
	rows, err := r.sql.QueryContext(ctx, `
		SELECT user_id, group_id, rate_multiplier
		FROM user_group_rate_multipliers
		WHERE user_id IN (`+Placeholders(len(uniqueIDs))+`) AND rate_multiplier IS NOT NULL
	`, int64Args(uniqueIDs)...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var userID int64
		var groupID int64
		var rate float64
		if err := rows.Scan(&userID, &groupID, &rate); err != nil {
			return nil, err
		}
		if _, ok := result[userID]; !ok {
			result[userID] = make(map[int64]float64)
		}
		result[userID][groupID] = rate
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

// upsertUserGroupRatesMySQL 是 user_group_rate_multipliers 批量 upsert 的 MySQL 公共实现：
// PG 版 `INSERT..SELECT FROM unnest($3::bigint[], $4::double precision[]) AS data(...)`
// + ON CONFLICT DO UPDATE 改为多行 VALUES + ODKU。ODKU 中引用新行值使用 8.0.19+
// 的行别名语法（AS new_rows），替代已弃用的 VALUES() 函数；conflict 时仅更新
// rate_multiplier / rpm_override 与 updated_at，保留 created_at 与另一列，与
// PG EXCLUDED 语义一致。
// REVIEW: 行别名（INSERT ... VALUES ... AS alias）要求 MySQL 8.0.19+。
func upsertUserGroupRatesMySQL(ctx context.Context, sqlq sqlExecutor, setColumn string, rows [][]any) error {
	if len(rows) == 0 {
		return nil
	}
	var query strings.Builder
	query.WriteString("INSERT INTO user_group_rate_multipliers (user_id, group_id, ")
	query.WriteString(setColumn)
	query.WriteString(", created_at, updated_at) VALUES ")
	args := make([]any, 0, len(rows)*5)
	for i, row := range rows {
		if i > 0 {
			query.WriteString(",")
		}
		query.WriteString("(?, ?, ?, ?, ?)")
		args = append(args, row[0], row[1], row[2], row[3], row[4])
	}
	query.WriteString(" AS new_rows ON DUPLICATE KEY UPDATE ")
	query.WriteString(setColumn)
	query.WriteString(" = new_rows.")
	query.WriteString(setColumn)
	query.WriteString(", updated_at = new_rows.updated_at")
	_, err := sqlq.ExecContext(ctx, query.String(), args...)
	return err
}

// syncUserGroupRatesMySQL 是 SyncUserGroupRates 的 MySQL 实现：与 PG 版逐语句对应
// （$n→?、NOW()→NOW(3)、ANY→IN 展开、unnest 双数组→多行 VALUES upsert）。
func (r *userGroupRateRepository) syncUserGroupRatesMySQL(ctx context.Context, userID int64, rates map[int64]*float64) error {
	if len(rates) == 0 {
		if _, err := r.sql.ExecContext(ctx, `
			UPDATE user_group_rate_multipliers
			SET rate_multiplier = NULL, updated_at = NOW(3)
			WHERE user_id = ?
		`, userID); err != nil {
			return err
		}
		_, err := r.sql.ExecContext(ctx,
			`DELETE FROM user_group_rate_multipliers WHERE user_id = ? AND rate_multiplier IS NULL AND rpm_override IS NULL`,
			userID)
		return err
	}

	var clearGroupIDs []int64
	upsertRows := make([][]any, 0, len(rates))
	for groupID, rate := range rates {
		if rate == nil {
			clearGroupIDs = append(clearGroupIDs, groupID)
		} else {
			upsertRows = append(upsertRows, []any{userID, groupID, *rate})
		}
	}

	if len(clearGroupIDs) > 0 {
		if _, err := r.sql.ExecContext(ctx, `
			UPDATE user_group_rate_multipliers
			SET rate_multiplier = NULL, updated_at = NOW(3)
			WHERE user_id = ? AND group_id IN (`+Placeholders(len(clearGroupIDs))+`)
		`, append([]any{userID}, int64Args(clearGroupIDs)...)...); err != nil {
			return err
		}
		if _, err := r.sql.ExecContext(ctx,
			`DELETE FROM user_group_rate_multipliers WHERE user_id = ? AND group_id IN (`+Placeholders(len(clearGroupIDs))+`) AND rate_multiplier IS NULL AND rpm_override IS NULL`,
			append([]any{userID}, int64Args(clearGroupIDs)...)...); err != nil {
			return err
		}
	}

	if len(upsertRows) > 0 {
		now := time.Now()
		// 补齐 created_at / updated_at 两个时间参数（PG 版由 $2::timestamptz 提供）。
		fullRows := make([][]any, 0, len(upsertRows))
		for _, row := range upsertRows {
			fullRows = append(fullRows, []any{row[0], row[1], row[2], now, now})
		}
		if err := upsertUserGroupRatesMySQL(ctx, r.sql, "rate_multiplier", fullRows); err != nil {
			return err
		}
	}

	return nil
}

// syncGroupRateMultipliersMySQL 是 SyncGroupRateMultipliers 的 MySQL 实现：
// `user_id <> ALL($2)` → `user_id NOT IN (...)`（调用方保证列表非空），
// unnest 双数组 → 多行 VALUES upsert。
func (r *userGroupRateRepository) syncGroupRateMultipliersMySQL(ctx context.Context, groupID int64, entries []service.GroupRateMultiplierInput) error {
	keepUserIDs := make([]int64, 0, len(entries))
	for _, e := range entries {
		keepUserIDs = append(keepUserIDs, e.UserID)
	}

	// 未在 entries 列表中的行：清空 rate_multiplier。
	if len(keepUserIDs) == 0 {
		if _, err := r.sql.ExecContext(ctx, `
			UPDATE user_group_rate_multipliers
			SET rate_multiplier = NULL, updated_at = NOW(3)
			WHERE group_id = ?
		`, groupID); err != nil {
			return err
		}
	} else {
		if _, err := r.sql.ExecContext(ctx, `
			UPDATE user_group_rate_multipliers
			SET rate_multiplier = NULL, updated_at = NOW(3)
			WHERE group_id = ? AND user_id NOT IN (`+Placeholders(len(keepUserIDs))+`)
		`, append([]any{groupID}, int64Args(keepUserIDs)...)...); err != nil {
			return err
		}
	}

	// 清空后若整行 NULL 则删除。
	if _, err := r.sql.ExecContext(ctx, `
		DELETE FROM user_group_rate_multipliers
		WHERE group_id = ? AND rate_multiplier IS NULL AND rpm_override IS NULL
	`, groupID); err != nil {
		return err
	}

	if len(entries) == 0 {
		return nil
	}

	now := time.Now()
	rows := make([][]any, 0, len(entries))
	for _, e := range entries {
		rows = append(rows, []any{e.UserID, groupID, e.RateMultiplier, now, now})
	}
	return upsertUserGroupRatesMySQL(ctx, r.sql, "rate_multiplier", rows)
}

// syncGroupRPMOverridesMySQL 是 SyncGroupRPMOverrides 的 MySQL 实现：
// `<> ALL` / `= ANY` → NOT IN / IN 展开，unnest 双数组 → 多行 VALUES upsert。
func (r *userGroupRateRepository) syncGroupRPMOverridesMySQL(ctx context.Context, groupID int64, entries []service.GroupRPMOverrideInput) error {
	keepUserIDs := make([]int64, 0, len(entries))
	var clearUserIDs []int64
	upsertRows := make([][]any, 0, len(entries))
	for _, e := range entries {
		keepUserIDs = append(keepUserIDs, e.UserID)
		if e.RPMOverride == nil {
			clearUserIDs = append(clearUserIDs, e.UserID)
		} else {
			upsertRows = append(upsertRows, []any{e.UserID, groupID, int32(*e.RPMOverride)})
		}
	}

	// 未在 entries 列表中的行：清空 rpm_override。
	if len(keepUserIDs) == 0 {
		if _, err := r.sql.ExecContext(ctx, `
			UPDATE user_group_rate_multipliers
			SET rpm_override = NULL, updated_at = NOW(3)
			WHERE group_id = ?
		`, groupID); err != nil {
			return err
		}
	} else {
		if _, err := r.sql.ExecContext(ctx, `
			UPDATE user_group_rate_multipliers
			SET rpm_override = NULL, updated_at = NOW(3)
			WHERE group_id = ? AND user_id NOT IN (`+Placeholders(len(keepUserIDs))+`)
		`, append([]any{groupID}, int64Args(keepUserIDs)...)...); err != nil {
			return err
		}
	}

	// 显式 clear 的行。
	if len(clearUserIDs) > 0 {
		if _, err := r.sql.ExecContext(ctx, `
			UPDATE user_group_rate_multipliers
			SET rpm_override = NULL, updated_at = NOW(3)
			WHERE group_id = ? AND user_id IN (`+Placeholders(len(clearUserIDs))+`)
		`, append([]any{groupID}, int64Args(clearUserIDs)...)...); err != nil {
			return err
		}
	}

	// 清空后若整行 NULL 则删除。
	if _, err := r.sql.ExecContext(ctx, `
		DELETE FROM user_group_rate_multipliers
		WHERE group_id = ? AND rate_multiplier IS NULL AND rpm_override IS NULL
	`, groupID); err != nil {
		return err
	}

	if len(upsertRows) > 0 {
		now := time.Now()
		fullRows := make([][]any, 0, len(upsertRows))
		for _, row := range upsertRows {
			fullRows = append(fullRows, []any{row[0], row[1], row[2], now, now})
		}
		if err := upsertUserGroupRatesMySQL(ctx, r.sql, "rpm_override", fullRows); err != nil {
			return err
		}
	}

	return nil
}
