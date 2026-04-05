package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/apikey"
	"github.com/Wei-Shaw/sub2api/ent/group"
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"
	"github.com/Wei-Shaw/sub2api/ent/user"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

type apiKeyRepository struct {
	client *dbent.Client
	sql    sqlExecutor
}

func NewAPIKeyRepository(client *dbent.Client, sqlDB *sql.DB) service.APIKeyRepository {
	return newAPIKeyRepositoryWithSQL(client, sqlDB)
}

func newAPIKeyRepositoryWithSQL(client *dbent.Client, sqlq sqlExecutor) *apiKeyRepository {
	return &apiKeyRepository{client: client, sql: sqlq}
}

func (r *apiKeyRepository) activeQuery() *dbent.APIKeyQuery {
	// 默认过滤已软删除记录，避免删除后仍被查询到。
	return r.client.APIKey.Query().Where(apikey.DeletedAtIsNil())
}

func (r *apiKeyRepository) Create(ctx context.Context, key *service.APIKey) error {
	builder := r.client.APIKey.Create().
		SetUserID(key.UserID).
		SetKey(key.Key).
		SetName(key.Name).
		SetStatus(key.Status).
		SetNillableGroupID(key.GroupID).
		SetNillableLastUsedAt(key.LastUsedAt).
		SetQuota(key.Quota).
		SetQuotaUsed(key.QuotaUsed).
		SetNillableExpiresAt(key.ExpiresAt).
		SetRateLimit5h(key.RateLimit5h).
		SetRateLimit1d(key.RateLimit1d).
		SetRateLimit7d(key.RateLimit7d)

	if len(key.IPWhitelist) > 0 {
		builder.SetIPWhitelist(key.IPWhitelist)
	}
	if len(key.IPBlacklist) > 0 {
		builder.SetIPBlacklist(key.IPBlacklist)
	}

	created, err := builder.Save(ctx)
	if err == nil {
		key.ID = created.ID
		key.LastUsedAt = created.LastUsedAt
		key.CreatedAt = created.CreatedAt
		key.UpdatedAt = created.UpdatedAt
	}
	return translatePersistenceError(err, nil, service.ErrAPIKeyExists)
}

func (r *apiKeyRepository) GetByID(ctx context.Context, id int64) (*service.APIKey, error) {
	m, err := r.activeQuery().
		Where(apikey.IDEQ(id)).
		WithUser().
		WithGroup().
		Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, service.ErrAPIKeyNotFound
		}
		return nil, err
	}
	return apiKeyEntityToService(m), nil
}

// GetKeyAndOwnerID 根据 API Key ID 获取其 key 与所有者（用户）ID。
// 相比 GetByID，此方法性能更优，因为：
//   - 使用 Select() 只查询必要字段，减少数据传输量
//   - 不加载完整的 API Key 实体及其关联数据（User、Group 等）
//   - 适用于删除等只需 key 与用户 ID 的场景
func (r *apiKeyRepository) GetKeyAndOwnerID(ctx context.Context, id int64) (string, int64, error) {
	m, err := r.activeQuery().
		Where(apikey.IDEQ(id)).
		Select(apikey.FieldKey, apikey.FieldUserID).
		Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return "", 0, service.ErrAPIKeyNotFound
		}
		return "", 0, err
	}
	return m.Key, m.UserID, nil
}

func (r *apiKeyRepository) GetByKey(ctx context.Context, key string) (*service.APIKey, error) {
	m, err := r.activeQuery().
		Where(apikey.KeyEQ(key)).
		WithUser().
		WithGroup().
		Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, service.ErrAPIKeyNotFound
		}
		return nil, err
	}
	return apiKeyEntityToService(m), nil
}

func (r *apiKeyRepository) GetByKeyForAuth(ctx context.Context, key string) (*service.APIKey, error) {
	m, err := r.activeQuery().
		Where(apikey.KeyEQ(key)).
		Select(
			apikey.FieldID,
			apikey.FieldUserID,
			apikey.FieldGroupID,
			apikey.FieldStatus,
			apikey.FieldIPWhitelist,
			apikey.FieldIPBlacklist,
			apikey.FieldQuota,
			apikey.FieldQuotaUsed,
			apikey.FieldExpiresAt,
			apikey.FieldRateLimit5h,
			apikey.FieldRateLimit1d,
			apikey.FieldRateLimit7d,
		).
		WithUser(func(q *dbent.UserQuery) {
			q.Select(
				user.FieldID,
				user.FieldStatus,
				user.FieldRole,
				user.FieldBalance,
				user.FieldConcurrency,
			)
		}).
		WithGroup(func(q *dbent.GroupQuery) {
			q.Select(
				group.FieldID,
				group.FieldName,
				group.FieldPlatform,
				group.FieldStatus,
				group.FieldSubscriptionType,
				group.FieldRateMultiplier,
				group.FieldDailyLimitUsd,
				group.FieldWeeklyLimitUsd,
				group.FieldMonthlyLimitUsd,
				group.FieldImagePrice1k,
				group.FieldImagePrice2k,
				group.FieldImagePrice4k,
				group.FieldClaudeCodeOnly,
				group.FieldFallbackGroupID,
				group.FieldFallbackGroupIDOnInvalidRequest,
				group.FieldModelRoutingEnabled,
				group.FieldModelRouting,
				group.FieldMcpXMLInject,
				group.FieldSupportedModelScopes,
				group.FieldAllowMessagesDispatch,
				group.FieldDefaultMappedModel,
			)
		}).
		Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, service.ErrAPIKeyNotFound
		}
		return nil, err
	}
	return apiKeyEntityToService(m), nil
}

func (r *apiKeyRepository) Update(ctx context.Context, key *service.APIKey) error {
	// 使用原子操作：将软删除检查与更新合并到同一语句，避免竞态条件。
	// 之前的实现先检查 Exist 再 UpdateOneID，若在两步之间发生软删除，
	// 则会更新已删除的记录。
	// 这里选择 Update().Where()，确保只有未软删除记录能被更新。
	// 同时显式设置 updated_at，避免二次查询带来的并发可见性问题。
	client := clientFromContext(ctx, r.client)
	now := time.Now()
	builder := client.APIKey.Update().
		Where(apikey.IDEQ(key.ID), apikey.DeletedAtIsNil()).
		SetName(key.Name).
		SetStatus(key.Status).
		SetQuota(key.Quota).
		SetQuotaUsed(key.QuotaUsed).
		SetRateLimit5h(key.RateLimit5h).
		SetRateLimit1d(key.RateLimit1d).
		SetRateLimit7d(key.RateLimit7d).
		SetUsage5h(key.Usage5h).
		SetUsage1d(key.Usage1d).
		SetUsage7d(key.Usage7d).
		SetUpdatedAt(now)
	if key.GroupID != nil {
		builder.SetGroupID(*key.GroupID)
	} else {
		builder.ClearGroupID()
	}

	// Expiration time
	if key.ExpiresAt != nil {
		builder.SetExpiresAt(*key.ExpiresAt)
	} else {
		builder.ClearExpiresAt()
	}

	// Rate limit window start times
	if key.Window5hStart != nil {
		builder.SetWindow5hStart(*key.Window5hStart)
	} else {
		builder.ClearWindow5hStart()
	}
	if key.Window1dStart != nil {
		builder.SetWindow1dStart(*key.Window1dStart)
	} else {
		builder.ClearWindow1dStart()
	}
	if key.Window7dStart != nil {
		builder.SetWindow7dStart(*key.Window7dStart)
	} else {
		builder.ClearWindow7dStart()
	}

	// IP 限制字段
	if len(key.IPWhitelist) > 0 {
		builder.SetIPWhitelist(key.IPWhitelist)
	} else {
		builder.ClearIPWhitelist()
	}
	if len(key.IPBlacklist) > 0 {
		builder.SetIPBlacklist(key.IPBlacklist)
	} else {
		builder.ClearIPBlacklist()
	}

	affected, err := builder.Save(ctx)
	if err != nil {
		return err
	}
	if affected == 0 {
		// 更新影响行数为 0，说明记录不存在或已被软删除。
		return service.ErrAPIKeyNotFound
	}

	// 使用同一时间戳回填，避免并发删除导致二次查询失败。
	key.UpdatedAt = now
	return nil
}

func (r *apiKeyRepository) Delete(ctx context.Context, id int64) error {
	// 存在唯一键约束 生成tombstone key 用来释放原key，长度远小于 128，满足 schema 限制
	tombstoneKey := fmt.Sprintf("__deleted__%d__%d", id, time.Now().UnixNano())
	// 显式软删除：避免依赖 Hook 行为，确保 deleted_at 一定被设置。
	affected, err := r.client.APIKey.Update().
		Where(apikey.IDEQ(id), apikey.DeletedAtIsNil()).
		SetKey(tombstoneKey).
		SetDeletedAt(time.Now()).
		Save(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return service.ErrAPIKeyNotFound
		}
		return err
	}
	if affected == 0 {
		exists, err := r.client.APIKey.Query().
			Where(apikey.IDEQ(id)).
			Exist(mixins.SkipSoftDelete(ctx))
		if err != nil {
			return err
		}
		if exists {
			return nil
		}
		return service.ErrAPIKeyNotFound
	}
	return nil
}

func (r *apiKeyRepository) ListByUserID(ctx context.Context, userID int64, params pagination.PaginationParams, filters service.APIKeyListFilters) ([]service.APIKey, *pagination.PaginationResult, error) {
	q := r.activeQuery().Where(apikey.UserIDEQ(userID))

	// Apply filters
	if filters.Search != "" {
		q = q.Where(apikey.Or(
			apikey.NameContainsFold(filters.Search),
			apikey.KeyContainsFold(filters.Search),
		))
	}
	if filters.Status != "" {
		q = q.Where(apikey.StatusEQ(filters.Status))
	}
	if filters.GroupID != nil {
		if *filters.GroupID == 0 {
			q = q.Where(apikey.GroupIDIsNil())
		} else {
			q = q.Where(apikey.GroupIDEQ(*filters.GroupID))
		}
	}

	total, err := q.Count(ctx)
	if err != nil {
		return nil, nil, err
	}

	keys, err := q.
		WithGroup().
		Offset(params.Offset()).
		Limit(params.Limit()).
		Order(dbent.Desc(apikey.FieldID)).
		All(ctx)
	if err != nil {
		return nil, nil, err
	}

	outKeys := make([]service.APIKey, 0, len(keys))
	for i := range keys {
		outKeys = append(outKeys, *apiKeyEntityToService(keys[i]))
	}

	return outKeys, paginationResultFromTotal(int64(total), params), nil
}

func (r *apiKeyRepository) VerifyOwnership(ctx context.Context, userID int64, apiKeyIDs []int64) ([]int64, error) {
	if len(apiKeyIDs) == 0 {
		return []int64{}, nil
	}

	ids, err := r.client.APIKey.Query().
		Where(apikey.UserIDEQ(userID), apikey.IDIn(apiKeyIDs...), apikey.DeletedAtIsNil()).
		IDs(ctx)
	if err != nil {
		return nil, err
	}
	return ids, nil
}

func (r *apiKeyRepository) CountByUserID(ctx context.Context, userID int64) (int64, error) {
	count, err := r.activeQuery().Where(apikey.UserIDEQ(userID)).Count(ctx)
	return int64(count), err
}

func (r *apiKeyRepository) ExistsByKey(ctx context.Context, key string) (bool, error) {
	count, err := r.activeQuery().Where(apikey.KeyEQ(key)).Count(ctx)
	return count > 0, err
}

func (r *apiKeyRepository) ListByGroupID(ctx context.Context, groupID int64, params pagination.PaginationParams) ([]service.APIKey, *pagination.PaginationResult, error) {
	q := r.activeQuery().Where(apikey.GroupIDEQ(groupID))

	total, err := q.Count(ctx)
	if err != nil {
		return nil, nil, err
	}

	keys, err := q.
		WithUser().
		Offset(params.Offset()).
		Limit(params.Limit()).
		Order(dbent.Desc(apikey.FieldID)).
		All(ctx)
	if err != nil {
		return nil, nil, err
	}

	outKeys := make([]service.APIKey, 0, len(keys))
	for i := range keys {
		outKeys = append(outKeys, *apiKeyEntityToService(keys[i]))
	}

	return outKeys, paginationResultFromTotal(int64(total), params), nil
}

// SearchAPIKeys searches API keys by user ID and/or keyword (name)
func (r *apiKeyRepository) SearchAPIKeys(ctx context.Context, userID int64, keyword string, limit int) ([]service.APIKey, error) {
	q := r.activeQuery()
	if userID > 0 {
		q = q.Where(apikey.UserIDEQ(userID))
	}

	if keyword != "" {
		q = q.Where(apikey.NameContainsFold(keyword))
	}

	keys, err := q.Limit(limit).Order(dbent.Desc(apikey.FieldID)).All(ctx)
	if err != nil {
		return nil, err
	}

	outKeys := make([]service.APIKey, 0, len(keys))
	for i := range keys {
		outKeys = append(outKeys, *apiKeyEntityToService(keys[i]))
	}
	return outKeys, nil
}

// ClearGroupIDByGroupID 将指定分组的所有 API Key 的 group_id 设为 nil
func (r *apiKeyRepository) ClearGroupIDByGroupID(ctx context.Context, groupID int64) (int64, error) {
	n, err := r.client.APIKey.Update().
		Where(apikey.GroupIDEQ(groupID), apikey.DeletedAtIsNil()).
		ClearGroupID().
		Save(ctx)
	return int64(n), err
}

// UpdateGroupIDByUserAndGroup 将用户下绑定 oldGroupID 的所有 Key 迁移到 newGroupID
func (r *apiKeyRepository) UpdateGroupIDByUserAndGroup(ctx context.Context, userID, oldGroupID, newGroupID int64) (int64, error) {
	client := clientFromContext(ctx, r.client)
	n, err := client.APIKey.Update().
		Where(apikey.UserIDEQ(userID), apikey.GroupIDEQ(oldGroupID), apikey.DeletedAtIsNil()).
		SetGroupID(newGroupID).
		Save(ctx)
	return int64(n), err
}

// CountByGroupID 获取分组的 API Key 数量
func (r *apiKeyRepository) CountByGroupID(ctx context.Context, groupID int64) (int64, error) {
	count, err := r.activeQuery().Where(apikey.GroupIDEQ(groupID)).Count(ctx)
	return int64(count), err
}

func (r *apiKeyRepository) ListKeysByUserID(ctx context.Context, userID int64) ([]string, error) {
	keys, err := r.activeQuery().
		Where(apikey.UserIDEQ(userID)).
		Select(apikey.FieldKey).
		Strings(ctx)
	if err != nil {
		return nil, err
	}
	return keys, nil
}

func (r *apiKeyRepository) ListKeysByGroupID(ctx context.Context, groupID int64) ([]string, error) {
	keys, err := r.activeQuery().
		Where(apikey.GroupIDEQ(groupID)).
		Select(apikey.FieldKey).
		Strings(ctx)
	if err != nil {
		return nil, err
	}
	return keys, nil
}

// IncrementQuotaUsed 使用 Ent 原子递增 quota_used 字段并返回新值
func (r *apiKeyRepository) IncrementQuotaUsed(ctx context.Context, id int64, amount float64) (float64, error) {
	updated, err := r.client.APIKey.UpdateOneID(id).
		Where(apikey.DeletedAtIsNil()).
		AddQuotaUsed(amount).
		Save(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return 0, service.ErrAPIKeyNotFound
		}
		return 0, err
	}
	return updated.QuotaUsed, nil
}

// IncrementQuotaUsedAndGetState atomically increments quota_used, conditionally marks the key
// as quota_exhausted, and returns the latest quota state in one round trip.
func (r *apiKeyRepository) IncrementQuotaUsedAndGetState(ctx context.Context, id int64, amount float64) (*service.APIKeyQuotaUsageState, error) {
	query := `
		UPDATE api_keys
		SET
			quota_used = quota_used + $1,
			status = CASE
				WHEN quota > 0 AND quota_used + $1 >= quota THEN $2
				ELSE status
			END,
			updated_at = NOW()
		WHERE id = $3 AND deleted_at IS NULL
		RETURNING quota_used, quota, key, status
	`

	state := &service.APIKeyQuotaUsageState{}
	if err := scanSingleRow(ctx, r.sql, query, []any{amount, service.StatusAPIKeyQuotaExhausted, id}, &state.QuotaUsed, &state.Quota, &state.Key, &state.Status); err != nil {
		if err == sql.ErrNoRows {
			return nil, service.ErrAPIKeyNotFound
		}
		return nil, err
	}
	return state, nil
}

func (r *apiKeyRepository) UpdateLastUsed(ctx context.Context, id int64, usedAt time.Time) error {
	affected, err := r.client.APIKey.Update().
		Where(apikey.IDEQ(id), apikey.DeletedAtIsNil()).
		SetLastUsedAt(usedAt).
		SetUpdatedAt(usedAt).
		Save(ctx)
	if err != nil {
		return err
	}
	if affected == 0 {
		return service.ErrAPIKeyNotFound
	}
	return nil
}

// IncrementRateLimitUsage atomically increments all rate limit usage counters and initializes
// window start times via COALESCE if not already set.
func (r *apiKeyRepository) IncrementRateLimitUsage(ctx context.Context, id int64, cost float64) error {
	_, err := r.sql.ExecContext(ctx, `
		UPDATE api_keys SET
			usage_5h = CASE WHEN window_5h_start IS NOT NULL AND window_5h_start + INTERVAL '5 hours' <= NOW() THEN $1 ELSE usage_5h + $1 END,
			usage_1d = CASE WHEN window_1d_start IS NOT NULL AND window_1d_start + INTERVAL '24 hours' <= NOW() THEN $1 ELSE usage_1d + $1 END,
			usage_7d = CASE WHEN window_7d_start IS NOT NULL AND window_7d_start + INTERVAL '7 days' <= NOW() THEN $1 ELSE usage_7d + $1 END,
			window_5h_start = CASE WHEN window_5h_start IS NULL OR window_5h_start + INTERVAL '5 hours' <= NOW() THEN NOW() ELSE window_5h_start END,
			window_1d_start = CASE WHEN window_1d_start IS NULL OR window_1d_start + INTERVAL '24 hours' <= NOW() THEN date_trunc('day', NOW()) ELSE window_1d_start END,
			window_7d_start = CASE WHEN window_7d_start IS NULL OR window_7d_start + INTERVAL '7 days' <= NOW() THEN date_trunc('day', NOW()) ELSE window_7d_start END,
			updated_at = NOW()
		WHERE id = $2 AND deleted_at IS NULL`,
		cost, id)
	return err
}

// ResetRateLimitWindows resets expired rate limit windows atomically.
func (r *apiKeyRepository) ResetRateLimitWindows(ctx context.Context, id int64) error {
	_, err := r.sql.ExecContext(ctx, `
		UPDATE api_keys SET
			usage_5h = CASE WHEN window_5h_start IS NOT NULL AND window_5h_start + INTERVAL '5 hours' <= NOW() THEN 0 ELSE usage_5h END,
			window_5h_start = CASE WHEN window_5h_start IS NOT NULL AND window_5h_start + INTERVAL '5 hours' <= NOW() THEN NOW() ELSE window_5h_start END,
			usage_1d = CASE WHEN window_1d_start IS NOT NULL AND window_1d_start + INTERVAL '24 hours' <= NOW() THEN 0 ELSE usage_1d END,
			window_1d_start = CASE WHEN window_1d_start IS NOT NULL AND window_1d_start + INTERVAL '24 hours' <= NOW() THEN date_trunc('day', NOW()) ELSE window_1d_start END,
			usage_7d = CASE WHEN window_7d_start IS NOT NULL AND window_7d_start + INTERVAL '7 days' <= NOW() THEN 0 ELSE usage_7d END,
			window_7d_start = CASE WHEN window_7d_start IS NOT NULL AND window_7d_start + INTERVAL '7 days' <= NOW() THEN date_trunc('day', NOW()) ELSE window_7d_start END,
			updated_at = NOW()
		WHERE id = $1 AND deleted_at IS NULL`,
		id)
	return err
}

// GetRateLimitData returns the current rate limit usage and window start times for an API key.
func (r *apiKeyRepository) GetRateLimitData(ctx context.Context, id int64) (result *service.APIKeyRateLimitData, err error) {
	rows, err := r.sql.QueryContext(ctx, `
		SELECT usage_5h, usage_1d, usage_7d, window_5h_start, window_1d_start, window_7d_start
		FROM api_keys
		WHERE id = $1 AND deleted_at IS NULL`,
		id)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
	}()
	if !rows.Next() {
		return nil, service.ErrAPIKeyNotFound
	}
	data := &service.APIKeyRateLimitData{}
	if err := rows.Scan(&data.Usage5h, &data.Usage1d, &data.Usage7d, &data.Window5hStart, &data.Window1dStart, &data.Window7dStart); err != nil {
		return nil, err
	}
	return data, rows.Err()
}

func apiKeyEntityToService(m *dbent.APIKey) *service.APIKey {
	if m == nil {
		return nil
	}
	out := &service.APIKey{
		ID:            m.ID,
		UserID:        m.UserID,
		Key:           m.Key,
		Name:          m.Name,
		Status:        m.Status,
		IPWhitelist:   m.IPWhitelist,
		IPBlacklist:   m.IPBlacklist,
		LastUsedAt:    m.LastUsedAt,
		CreatedAt:     m.CreatedAt,
		UpdatedAt:     m.UpdatedAt,
		GroupID:       m.GroupID,
		Quota:         m.Quota,
		QuotaUsed:     m.QuotaUsed,
		ExpiresAt:     m.ExpiresAt,
		RateLimit5h:   m.RateLimit5h,
		RateLimit1d:   m.RateLimit1d,
		RateLimit7d:   m.RateLimit7d,
		Usage5h:       m.Usage5h,
		Usage1d:       m.Usage1d,
		Usage7d:       m.Usage7d,
		Window5hStart: m.Window5hStart,
		Window1dStart: m.Window1dStart,
		Window7dStart: m.Window7dStart,
	}
	if m.Edges.User != nil {
		out.User = userEntityToService(m.Edges.User)
	}
	if m.Edges.Group != nil {
		out.Group = groupEntityToService(m.Edges.Group)
	}
	return out
}

func userEntityToService(u *dbent.User) *service.User {
	if u == nil {
		return nil
	}
	return &service.User{
		ID:                  u.ID,
		Email:               u.Email,
		Username:            u.Username,
		Notes:               u.Notes,
		PasswordHash:        u.PasswordHash,
		Role:                u.Role,
		Balance:             u.Balance,
		Concurrency:         u.Concurrency,
		Status:              u.Status,
		TotpSecretEncrypted: u.TotpSecretEncrypted,
		TotpEnabled:         u.TotpEnabled,
		TotpEnabledAt:       u.TotpEnabledAt,
		CreatedAt:           u.CreatedAt,
		UpdatedAt:           u.UpdatedAt,
	}
}

func groupEntityToService(g *dbent.Group) *service.Group {
	if g == nil {
		return nil
	}
	return &service.Group{
		ID:                              g.ID,
		Name:                            g.Name,
		Description:                     derefString(g.Description),
		Platform:                        g.Platform,
		RateMultiplier:                  g.RateMultiplier,
		IsExclusive:                     g.IsExclusive,
		Status:                          g.Status,
		Hydrated:                        true,
		SubscriptionType:                g.SubscriptionType,
		DailyLimitUSD:                   g.DailyLimitUsd,
		WeeklyLimitUSD:                  g.WeeklyLimitUsd,
		MonthlyLimitUSD:                 g.MonthlyLimitUsd,
		ImagePrice1K:                    g.ImagePrice1k,
		ImagePrice2K:                    g.ImagePrice2k,
		ImagePrice4K:                    g.ImagePrice4k,
		DefaultValidityDays:             g.DefaultValidityDays,
		ClaudeCodeOnly:                  g.ClaudeCodeOnly,
		FallbackGroupID:                 g.FallbackGroupID,
		FallbackGroupIDOnInvalidRequest: g.FallbackGroupIDOnInvalidRequest,
		ModelRouting:                    g.ModelRouting,
		ModelRoutingEnabled:             g.ModelRoutingEnabled,
		MCPXMLInject:                    g.McpXMLInject,
		SupportedModelScopes:            g.SupportedModelScopes,
		SortOrder:                       g.SortOrder,
		AllowMessagesDispatch:           g.AllowMessagesDispatch,
		DefaultMappedModel:              g.DefaultMappedModel,
		CreatedAt:                       g.CreatedAt,
		UpdatedAt:                       g.UpdatedAt,
	}
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
