package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/apikey"
	"github.com/Wei-Shaw/sub2api/ent/group"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

type sqlExecutor interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

type groupRepository struct {
	client *dbent.Client
	sql    sqlExecutor
}

func NewGroupRepository(client *dbent.Client, sqlDB *sql.DB) service.GroupRepository {
	return newGroupRepositoryWithSQL(client, sqlDB)
}

func newGroupRepositoryWithSQL(client *dbent.Client, sqlq sqlExecutor) *groupRepository {
	return &groupRepository{client: client, sql: sqlq}
}

func (r *groupRepository) Create(ctx context.Context, groupIn *service.Group) error {
	builder := r.client.Group.Create().
		SetName(groupIn.Name).
		SetDescription(groupIn.Description).
		SetPlatform(groupIn.Platform).
		SetRateMultiplier(groupIn.RateMultiplier).
		SetIsExclusive(groupIn.IsExclusive).
		SetStatus(groupIn.Status).
		SetSubscriptionType(groupIn.SubscriptionType).
		SetNillableDailyLimitUsd(groupIn.DailyLimitUSD).
		SetNillableWeeklyLimitUsd(groupIn.WeeklyLimitUSD).
		SetNillableMonthlyLimitUsd(groupIn.MonthlyLimitUSD).
		SetNillableImagePrice1k(groupIn.ImagePrice1K).
		SetNillableImagePrice2k(groupIn.ImagePrice2K).
		SetNillableImagePrice4k(groupIn.ImagePrice4K).
		SetDefaultValidityDays(groupIn.DefaultValidityDays).
		SetClaudeCodeOnly(groupIn.ClaudeCodeOnly).
		SetNillableFallbackGroupID(groupIn.FallbackGroupID).
		SetNillableFallbackGroupIDOnInvalidRequest(groupIn.FallbackGroupIDOnInvalidRequest).
		SetModelRoutingEnabled(groupIn.ModelRoutingEnabled).
		SetMcpXMLInject(groupIn.MCPXMLInject).
		SetAllowMessagesDispatch(groupIn.AllowMessagesDispatch).
		SetDefaultMappedModel(groupIn.DefaultMappedModel)

	// 设置模型路由配置
	if groupIn.ModelRouting != nil {
		builder = builder.SetModelRouting(groupIn.ModelRouting)
	}

	// 设置支持的模型系列（始终设置，空数组表示不限制）
	builder = builder.SetSupportedModelScopes(groupIn.SupportedModelScopes)

	created, err := builder.Save(ctx)
	if err == nil {
		groupIn.ID = created.ID
		groupIn.CreatedAt = created.CreatedAt
		groupIn.UpdatedAt = created.UpdatedAt
		if err := enqueueSchedulerOutbox(ctx, r.sql, service.SchedulerOutboxEventGroupChanged, nil, &groupIn.ID, nil); err != nil {
			logger.LegacyPrintf("repository.group", "[SchedulerOutbox] enqueue group create failed: group=%d err=%v", groupIn.ID, err)
		}
	}
	return translatePersistenceError(err, nil, service.ErrGroupExists)
}

func (r *groupRepository) GetByID(ctx context.Context, id int64) (*service.Group, error) {
	out, err := r.GetByIDLite(ctx, id)
	if err != nil {
		return nil, err
	}
	total, active, _ := r.GetAccountCount(ctx, out.ID)
	out.AccountCount = total
	out.ActiveAccountCount = active
	return out, nil
}

func (r *groupRepository) GetByIDLite(ctx context.Context, id int64) (*service.Group, error) {
	// AccountCount is intentionally not loaded here; use GetByID when needed.
	m, err := r.client.Group.Query().
		Where(group.IDEQ(id)).
		Only(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrGroupNotFound, nil)
	}
	return groupEntityToService(m), nil
}

func (r *groupRepository) Update(ctx context.Context, groupIn *service.Group) error {
	builder := r.client.Group.UpdateOneID(groupIn.ID).
		SetName(groupIn.Name).
		SetDescription(groupIn.Description).
		SetPlatform(groupIn.Platform).
		SetRateMultiplier(groupIn.RateMultiplier).
		SetIsExclusive(groupIn.IsExclusive).
		SetStatus(groupIn.Status).
		SetSubscriptionType(groupIn.SubscriptionType).
		SetNillableDailyLimitUsd(groupIn.DailyLimitUSD).
		SetNillableWeeklyLimitUsd(groupIn.WeeklyLimitUSD).
		SetNillableMonthlyLimitUsd(groupIn.MonthlyLimitUSD).
		SetNillableImagePrice1k(groupIn.ImagePrice1K).
		SetNillableImagePrice2k(groupIn.ImagePrice2K).
		SetNillableImagePrice4k(groupIn.ImagePrice4K).
		SetDefaultValidityDays(groupIn.DefaultValidityDays).
		SetClaudeCodeOnly(groupIn.ClaudeCodeOnly).
		SetModelRoutingEnabled(groupIn.ModelRoutingEnabled).
		SetMcpXMLInject(groupIn.MCPXMLInject).
		SetAllowMessagesDispatch(groupIn.AllowMessagesDispatch).
		SetDefaultMappedModel(groupIn.DefaultMappedModel)

	// 显式处理可空字段：nil 需要 clear，非 nil 需要 set。
	if groupIn.DailyLimitUSD != nil {
		builder = builder.SetDailyLimitUsd(*groupIn.DailyLimitUSD)
	} else {
		builder = builder.ClearDailyLimitUsd()
	}
	if groupIn.WeeklyLimitUSD != nil {
		builder = builder.SetWeeklyLimitUsd(*groupIn.WeeklyLimitUSD)
	} else {
		builder = builder.ClearWeeklyLimitUsd()
	}
	if groupIn.MonthlyLimitUSD != nil {
		builder = builder.SetMonthlyLimitUsd(*groupIn.MonthlyLimitUSD)
	} else {
		builder = builder.ClearMonthlyLimitUsd()
	}
	if groupIn.ImagePrice1K != nil {
		builder = builder.SetImagePrice1k(*groupIn.ImagePrice1K)
	} else {
		builder = builder.ClearImagePrice1k()
	}
	if groupIn.ImagePrice2K != nil {
		builder = builder.SetImagePrice2k(*groupIn.ImagePrice2K)
	} else {
		builder = builder.ClearImagePrice2k()
	}
	if groupIn.ImagePrice4K != nil {
		builder = builder.SetImagePrice4k(*groupIn.ImagePrice4K)
	} else {
		builder = builder.ClearImagePrice4k()
	}

	// 处理 FallbackGroupID：nil 时清除，否则设置
	if groupIn.FallbackGroupID != nil {
		builder = builder.SetFallbackGroupID(*groupIn.FallbackGroupID)
	} else {
		builder = builder.ClearFallbackGroupID()
	}
	// 处理 FallbackGroupIDOnInvalidRequest：nil 时清除，否则设置
	if groupIn.FallbackGroupIDOnInvalidRequest != nil {
		builder = builder.SetFallbackGroupIDOnInvalidRequest(*groupIn.FallbackGroupIDOnInvalidRequest)
	} else {
		builder = builder.ClearFallbackGroupIDOnInvalidRequest()
	}

	// 处理 ModelRouting：nil 时清除，否则设置
	if groupIn.ModelRouting != nil {
		builder = builder.SetModelRouting(groupIn.ModelRouting)
	} else {
		builder = builder.ClearModelRouting()
	}

	// 处理 SupportedModelScopes（始终设置，空数组表示不限制）
	builder = builder.SetSupportedModelScopes(groupIn.SupportedModelScopes)

	updated, err := builder.Save(ctx)
	if err != nil {
		return translatePersistenceError(err, service.ErrGroupNotFound, service.ErrGroupExists)
	}
	groupIn.UpdatedAt = updated.UpdatedAt
	if err := enqueueSchedulerOutbox(ctx, r.sql, service.SchedulerOutboxEventGroupChanged, nil, &groupIn.ID, nil); err != nil {
		logger.LegacyPrintf("repository.group", "[SchedulerOutbox] enqueue group update failed: group=%d err=%v", groupIn.ID, err)
	}
	return nil
}

func (r *groupRepository) Delete(ctx context.Context, id int64) error {
	_, err := r.client.Group.Delete().Where(group.IDEQ(id)).Exec(ctx)
	if err != nil {
		return translatePersistenceError(err, service.ErrGroupNotFound, nil)
	}
	if err := enqueueSchedulerOutbox(ctx, r.sql, service.SchedulerOutboxEventGroupChanged, nil, &id, nil); err != nil {
		logger.LegacyPrintf("repository.group", "[SchedulerOutbox] enqueue group delete failed: group=%d err=%v", id, err)
	}
	return nil
}

func (r *groupRepository) List(ctx context.Context, params pagination.PaginationParams) ([]service.Group, *pagination.PaginationResult, error) {
	return r.ListWithFilters(ctx, params, "", "", "", nil)
}

func (r *groupRepository) ListWithFilters(ctx context.Context, params pagination.PaginationParams, platform, status, search string, isExclusive *bool) ([]service.Group, *pagination.PaginationResult, error) {
	q := r.client.Group.Query()

	if platform != "" {
		q = q.Where(group.PlatformEQ(platform))
	}
	if status != "" {
		q = q.Where(group.StatusEQ(status))
	}
	if search != "" {
		q = q.Where(group.Or(
			group.NameContainsFold(search),
			group.DescriptionContainsFold(search),
		))
	}
	if isExclusive != nil {
		q = q.Where(group.IsExclusiveEQ(*isExclusive))
	}

	total, err := q.Clone().Count(ctx)
	if err != nil {
		return nil, nil, err
	}

	groups, err := q.
		Offset(params.Offset()).
		Limit(params.Limit()).
		Order(dbent.Asc(group.FieldSortOrder), dbent.Asc(group.FieldID)).
		All(ctx)
	if err != nil {
		return nil, nil, err
	}

	groupIDs := make([]int64, 0, len(groups))
	outGroups := make([]service.Group, 0, len(groups))
	for i := range groups {
		g := groupEntityToService(groups[i])
		outGroups = append(outGroups, *g)
		groupIDs = append(groupIDs, g.ID)
	}

	counts, err := r.loadAccountCounts(ctx, groupIDs)
	if err == nil {
		for i := range outGroups {
			c := counts[outGroups[i].ID]
			outGroups[i].AccountCount = c.Total
			outGroups[i].ActiveAccountCount = c.Active
			outGroups[i].RateLimitedAccountCount = c.RateLimited
		}
	}

	return outGroups, paginationResultFromTotal(int64(total), params), nil
}

func (r *groupRepository) ListActive(ctx context.Context) ([]service.Group, error) {
	groups, err := r.client.Group.Query().
		Where(group.StatusEQ(service.StatusActive)).
		Order(dbent.Asc(group.FieldSortOrder), dbent.Asc(group.FieldID)).
		All(ctx)
	if err != nil {
		return nil, err
	}

	groupIDs := make([]int64, 0, len(groups))
	outGroups := make([]service.Group, 0, len(groups))
	for i := range groups {
		g := groupEntityToService(groups[i])
		outGroups = append(outGroups, *g)
		groupIDs = append(groupIDs, g.ID)
	}

	counts, err := r.loadAccountCounts(ctx, groupIDs)
	if err == nil {
		for i := range outGroups {
			c := counts[outGroups[i].ID]
			outGroups[i].AccountCount = c.Total
			outGroups[i].ActiveAccountCount = c.Active
			outGroups[i].RateLimitedAccountCount = c.RateLimited
		}
	}

	return outGroups, nil
}

func (r *groupRepository) ListActiveByPlatform(ctx context.Context, platform string) ([]service.Group, error) {
	groups, err := r.client.Group.Query().
		Where(group.StatusEQ(service.StatusActive), group.PlatformEQ(platform)).
		Order(dbent.Asc(group.FieldSortOrder), dbent.Asc(group.FieldID)).
		All(ctx)
	if err != nil {
		return nil, err
	}

	groupIDs := make([]int64, 0, len(groups))
	outGroups := make([]service.Group, 0, len(groups))
	for i := range groups {
		g := groupEntityToService(groups[i])
		outGroups = append(outGroups, *g)
		groupIDs = append(groupIDs, g.ID)
	}

	counts, err := r.loadAccountCounts(ctx, groupIDs)
	if err == nil {
		for i := range outGroups {
			c := counts[outGroups[i].ID]
			outGroups[i].AccountCount = c.Total
			outGroups[i].ActiveAccountCount = c.Active
			outGroups[i].RateLimitedAccountCount = c.RateLimited
		}
	}

	return outGroups, nil
}

func (r *groupRepository) ExistsByName(ctx context.Context, name string) (bool, error) {
	return r.client.Group.Query().Where(group.NameEQ(name)).Exist(ctx)
}

// ExistsByIDs 批量检查分组是否存在（仅检查未软删除记录）。
// 返回结构：map[groupID]exists。
func (r *groupRepository) ExistsByIDs(ctx context.Context, ids []int64) (map[int64]bool, error) {
	result := make(map[int64]bool, len(ids))
	if len(ids) == 0 {
		return result, nil
	}

	uniqueIDs := make([]int64, 0, len(ids))
	seen := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		uniqueIDs = append(uniqueIDs, id)
		result[id] = false
	}
	if len(uniqueIDs) == 0 {
		return result, nil
	}

	rows, err := r.sql.QueryContext(ctx, `
		SELECT id
		FROM groups
		WHERE id = ANY($1) AND deleted_at IS NULL
	`, pq.Array(uniqueIDs))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		result[id] = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func (r *groupRepository) GetAccountCount(ctx context.Context, groupID int64) (total int64, active int64, err error) {
	var rateLimited int64
	err = scanSingleRow(ctx, r.sql,
		`SELECT COUNT(*),
			COUNT(*) FILTER (WHERE a.status = 'active' AND a.schedulable = true),
			COUNT(*) FILTER (WHERE a.status = 'active' AND (
				a.rate_limit_reset_at > NOW() OR
				a.overload_until > NOW() OR
				a.temp_unschedulable_until > NOW()
			))
		FROM account_groups ag JOIN accounts a ON a.id = ag.account_id
		WHERE ag.group_id = $1`,
		[]any{groupID}, &total, &active, &rateLimited)
	return
}

func (r *groupRepository) DeleteAccountGroupsByGroupID(ctx context.Context, groupID int64) (int64, error) {
	res, err := r.sql.ExecContext(ctx, "DELETE FROM account_groups WHERE group_id = $1", groupID)
	if err != nil {
		return 0, err
	}
	affected, _ := res.RowsAffected()
	if err := enqueueSchedulerOutbox(ctx, r.sql, service.SchedulerOutboxEventGroupChanged, nil, &groupID, nil); err != nil {
		logger.LegacyPrintf("repository.group", "[SchedulerOutbox] enqueue group account clear failed: group=%d err=%v", groupID, err)
	}
	return affected, nil
}

func (r *groupRepository) DeleteCascade(ctx context.Context, id int64) ([]int64, error) {
	g, err := r.client.Group.Query().Where(group.IDEQ(id)).Only(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrGroupNotFound, nil)
	}
	groupSvc := groupEntityToService(g)

	// 使用 ent 事务统一包裹：避免手工基于 *sql.Tx 构造 ent client 带来的驱动断言问题，
	// 同时保证级联删除的原子性。
	tx, err := r.client.Tx(ctx)
	if err != nil && !errors.Is(err, dbent.ErrTxStarted) {
		return nil, err
	}
	exec := r.client
	txClient := r.client
	if err == nil {
		defer func() { _ = tx.Rollback() }()
		exec = tx.Client()
		txClient = exec
	}
	// err 为 dbent.ErrTxStarted 时，复用当前 client 参与同一事务。

	// Lock the group row to avoid concurrent writes while we cascade.
	// 这里使用 exec.QueryContext 手动扫描，确保同一事务内加锁并能区分"未找到"与其他错误。
	rows, err := exec.QueryContext(ctx, "SELECT id FROM groups WHERE id = $1 AND deleted_at IS NULL FOR UPDATE", id)
	if err != nil {
		return nil, err
	}
	var lockedID int64
	if rows.Next() {
		if err := rows.Scan(&lockedID); err != nil {
			_ = rows.Close()
			return nil, err
		}
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if lockedID == 0 {
		return nil, service.ErrGroupNotFound
	}

	var affectedUserIDs []int64
	if groupSvc.IsSubscriptionType() {
		// 只查询未软删除的订阅，避免通知已取消订阅的用户
		rows, err := exec.QueryContext(ctx, "SELECT user_id FROM user_subscriptions WHERE group_id = $1 AND deleted_at IS NULL", id)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var userID int64
			if scanErr := rows.Scan(&userID); scanErr != nil {
				_ = rows.Close()
				return nil, scanErr
			}
			affectedUserIDs = append(affectedUserIDs, userID)
		}
		if err := rows.Close(); err != nil {
			return nil, err
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}

		// 软删除订阅：设置 deleted_at 而非硬删除
		if _, err := exec.ExecContext(ctx, "UPDATE user_subscriptions SET deleted_at = NOW() WHERE group_id = $1 AND deleted_at IS NULL", id); err != nil {
			return nil, err
		}
	}

	// 2. Clear group_id for api keys bound to this group.
	// 仅更新未软删除的记录，避免修改已删除数据，保证审计与历史回溯一致性。
	// 与 APIKeyRepository 的软删除语义保持一致，减少跨模块行为差异。
	if _, err := txClient.APIKey.Update().
		Where(apikey.GroupIDEQ(id), apikey.DeletedAtIsNil()).
		ClearGroupID().
		Save(ctx); err != nil {
		return nil, err
	}

	// 3. Remove the group id from user_allowed_groups join table.
	// Legacy users.allowed_groups 列已弃用，不再同步。
	if _, err := exec.ExecContext(ctx, "DELETE FROM user_allowed_groups WHERE group_id = $1", id); err != nil {
		return nil, err
	}

	// 4. Delete account_groups join rows.
	if _, err := exec.ExecContext(ctx, "DELETE FROM account_groups WHERE group_id = $1", id); err != nil {
		return nil, err
	}

	// 5. Soft-delete group itself.
	if _, err := txClient.Group.Delete().Where(group.IDEQ(id)).Exec(ctx); err != nil {
		return nil, err
	}

	if tx != nil {
		if err := tx.Commit(); err != nil {
			return nil, err
		}
	}
	if err := enqueueSchedulerOutbox(ctx, r.sql, service.SchedulerOutboxEventGroupChanged, nil, &id, nil); err != nil {
		logger.LegacyPrintf("repository.group", "[SchedulerOutbox] enqueue group cascade delete failed: group=%d err=%v", id, err)
	}

	return affectedUserIDs, nil
}

type groupAccountCounts struct {
	Total       int64
	Active      int64
	RateLimited int64
}

func (r *groupRepository) loadAccountCounts(ctx context.Context, groupIDs []int64) (counts map[int64]groupAccountCounts, err error) {
	counts = make(map[int64]groupAccountCounts, len(groupIDs))
	if len(groupIDs) == 0 {
		return counts, nil
	}

	rows, err := r.sql.QueryContext(
		ctx,
		`SELECT ag.group_id,
			COUNT(*) AS total,
			COUNT(*) FILTER (WHERE a.status = 'active' AND a.schedulable = true) AS active,
			COUNT(*) FILTER (WHERE a.status = 'active' AND (
				a.rate_limit_reset_at > NOW() OR
				a.overload_until > NOW() OR
				a.temp_unschedulable_until > NOW()
			)) AS rate_limited
		FROM account_groups ag
		JOIN accounts a ON a.id = ag.account_id
		WHERE ag.group_id = ANY($1)
		GROUP BY ag.group_id`,
		pq.Array(groupIDs),
	)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = closeErr
			counts = nil
		}
	}()

	for rows.Next() {
		var groupID int64
		var c groupAccountCounts
		if err = rows.Scan(&groupID, &c.Total, &c.Active, &c.RateLimited); err != nil {
			return nil, err
		}
		counts[groupID] = c
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}

	return counts, nil
}

// GetAccountIDsByGroupIDs 获取多个分组的所有账号 ID（去重）
func (r *groupRepository) GetAccountIDsByGroupIDs(ctx context.Context, groupIDs []int64) ([]int64, error) {
	if len(groupIDs) == 0 {
		return nil, nil
	}

	rows, err := r.sql.QueryContext(
		ctx,
		"SELECT DISTINCT account_id FROM account_groups WHERE group_id = ANY($1) ORDER BY account_id",
		pq.Array(groupIDs),
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var accountIDs []int64
	for rows.Next() {
		var accountID int64
		if err := rows.Scan(&accountID); err != nil {
			return nil, err
		}
		accountIDs = append(accountIDs, accountID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return accountIDs, nil
}

// BindAccountsToGroup 将多个账号绑定到指定分组（批量插入，忽略已存在的绑定）
func (r *groupRepository) BindAccountsToGroup(ctx context.Context, groupID int64, accountIDs []int64) error {
	if len(accountIDs) == 0 {
		return nil
	}

	// 使用 INSERT ... ON CONFLICT DO NOTHING 忽略已存在的绑定
	_, err := r.sql.ExecContext(
		ctx,
		`INSERT INTO account_groups (account_id, group_id, priority, created_at)
		 SELECT unnest($1::bigint[]), $2, 50, NOW()
		 ON CONFLICT (account_id, group_id) DO NOTHING`,
		pq.Array(accountIDs),
		groupID,
	)
	if err != nil {
		return err
	}

	// 发送调度器事件
	if err := enqueueSchedulerOutbox(ctx, r.sql, service.SchedulerOutboxEventGroupChanged, nil, &groupID, nil); err != nil {
		logger.LegacyPrintf("repository.group", "[SchedulerOutbox] enqueue bind accounts to group failed: group=%d err=%v", groupID, err)
	}

	return nil
}

// UpdateSortOrders 批量更新分组排序
func (r *groupRepository) UpdateSortOrders(ctx context.Context, updates []service.GroupSortOrderUpdate) error {
	if len(updates) == 0 {
		return nil
	}

	// 去重后保留最后一次排序值，避免重复 ID 造成 CASE 分支冲突。
	sortOrderByID := make(map[int64]int, len(updates))
	groupIDs := make([]int64, 0, len(updates))
	for _, u := range updates {
		if u.ID <= 0 {
			continue
		}
		if _, exists := sortOrderByID[u.ID]; !exists {
			groupIDs = append(groupIDs, u.ID)
		}
		sortOrderByID[u.ID] = u.SortOrder
	}
	if len(groupIDs) == 0 {
		return nil
	}

	// 与旧实现保持一致：任何不存在/已删除的分组都返回 not found，且不执行更新。
	var existingCount int
	if err := scanSingleRow(
		ctx,
		r.sql,
		`SELECT COUNT(*) FROM groups WHERE deleted_at IS NULL AND id = ANY($1)`,
		[]any{pq.Array(groupIDs)},
		&existingCount,
	); err != nil {
		return err
	}
	if existingCount != len(groupIDs) {
		return service.ErrGroupNotFound
	}

	args := make([]any, 0, len(groupIDs)*2+1)
	caseClauses := make([]string, 0, len(groupIDs))
	placeholder := 1
	for _, id := range groupIDs {
		caseClauses = append(caseClauses, fmt.Sprintf("WHEN $%d THEN $%d", placeholder, placeholder+1))
		args = append(args, id, sortOrderByID[id])
		placeholder += 2
	}
	args = append(args, pq.Array(groupIDs))

	query := fmt.Sprintf(`
		UPDATE groups
		SET sort_order = CASE id
			%s
			ELSE sort_order
		END
		WHERE deleted_at IS NULL AND id = ANY($%d)
	`, strings.Join(caseClauses, "\n\t\t\t"), placeholder)

	result, err := r.sql.ExecContext(ctx, query, args...)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != int64(len(groupIDs)) {
		return service.ErrGroupNotFound
	}

	for _, id := range groupIDs {
		if err := enqueueSchedulerOutbox(ctx, r.sql, service.SchedulerOutboxEventGroupChanged, nil, &id, nil); err != nil {
			logger.LegacyPrintf("repository.group", "[SchedulerOutbox] enqueue group sort update failed: group=%d err=%v", id, err)
		}
	}
	return nil
}
