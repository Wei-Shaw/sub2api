package repository

import (
	"context"
	"database/sql"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type userGroupRateRepository struct {
	sql sqlExecutor
REDACTED

// NewUserGroupRateRepository 创建用户专属分组倍率仓储
func NewUserGroupRateRepository(sqlDB *sql.DB) service.UserGroupRateRepository {
	return &userGroupRateRepository{sql: sqlDBREDACTED
REDACTED

// GetByUserID 获取用户的所有专属分组倍率
func (r *userGroupRateRepository) GetByUserID(ctx context.Context, userID int64) (map[int64]float64, error) {
	query := `SELECT group_id, rate_multiplier FROM user_group_rate_multipliers WHERE user_id = $1`
	rows, err := r.sql.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, err
REDACTED
	defer func() { _ = rows.Close() REDACTED()

	result := make(map[int64]float64)
	for rows.Next() {
		var groupID int64
		var rate float64
		if err := rows.Scan(&groupID, &rate); err != nil {
			return nil, err
	REDACTED
		result[groupID] = rate
REDACTED
	if err := rows.Err(); err != nil {
		return nil, err
REDACTED
	return result, nil
REDACTED

// GetByUserAndGroup 获取用户在特定分组的专属倍率
func (r *userGroupRateRepository) GetByUserAndGroup(ctx context.Context, userID, groupID int64) (*float64, error) {
	query := `SELECT rate_multiplier FROM user_group_rate_multipliers WHERE user_id = $1 AND group_id = $2`
	var rate float64
	err := scanSingleRow(ctx, r.sql, query, []any{userID, groupIDREDACTED, &rate)
	if err == sql.ErrNoRows {
		return nil, nil
REDACTED
	if err != nil {
		return nil, err
REDACTED
	return &rate, nil
REDACTED

// SyncUserGroupRates 同步用户的分组专属倍率
func (r *userGroupRateRepository) SyncUserGroupRates(ctx context.Context, userID int64, rates map[int64]*float64) error {
	if len(rates) == 0 {
		// 如果传入空 map，删除该用户的所有专属倍率
		_, err := r.sql.ExecContext(ctx, `DELETE FROM user_group_rate_multipliers WHERE user_id = $1`, userID)
		return err
REDACTED

	// 分离需要删除和需要 upsert 的记录
	var toDelete []int64
	toUpsert := make(map[int64]float64)
	for groupID, rate := range rates {
		if rate == nil {
			toDelete = append(toDelete, groupID)
	REDACTED else {
			toUpsert[groupID] = *rate
	REDACTED
REDACTED

	// 删除指定的记录
	for _, groupID := range toDelete {
		_, err := r.sql.ExecContext(ctx,
			`DELETE FROM user_group_rate_multipliers WHERE user_id = $1 AND group_id = $2`,
			userID, groupID)
		if err != nil {
			return err
	REDACTED
REDACTED

	// Upsert 记录
	now := time.Now()
	for groupID, rate := range toUpsert {
		_, err := r.sql.ExecContext(ctx, `
			INSERT INTO user_group_rate_multipliers (user_id, group_id, rate_multiplier, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $4)
			ON CONFLICT (user_id, group_id) DO UPDATE SET rate_multiplier = $3, updated_at = $4
		`, userID, groupID, rate, now)
		if err != nil {
			return err
	REDACTED
REDACTED

	return nil
REDACTED

// DeleteByGroupID 删除指定分组的所有用户专属倍率
func (r *userGroupRateRepository) DeleteByGroupID(ctx context.Context, groupID int64) error {
	_, err := r.sql.ExecContext(ctx, `DELETE FROM user_group_rate_multipliers WHERE group_id = $1`, groupID)
	return err
REDACTED

// DeleteByUserID 删除指定用户的所有专属倍率
func (r *userGroupRateRepository) DeleteByUserID(ctx context.Context, userID int64) error {
	_, err := r.sql.ExecContext(ctx, `DELETE FROM user_group_rate_multipliers WHERE user_id = $1`, userID)
	return err
REDACTED
