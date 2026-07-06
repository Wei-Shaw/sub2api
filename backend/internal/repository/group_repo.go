package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/group"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"

	entsql "entgo.io/ent/dialect/sql"
)

type sqlExecutor interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
REDACTED

type groupRepository struct {
	client *dbent.Client
	sql    sqlExecutor
REDACTED

func NewGroupRepository(client *dbent.Client, sqlDB *sql.DB) service.GroupRepository {
	return newGroupRepositoryWithSQL(client, sqlDB)
REDACTED

func newGroupRepositoryWithSQL(client *dbent.Client, sqlq sqlExecutor) *groupRepository {
	return &groupRepository{client: client, sql: sqlqREDACTED
REDACTED

func (r *groupRepository) Create(ctx context.Context, groupIn *service.Group) error {
	builder := r.client.Group.Create().
		SetName(groupIn.Name).
		SetDescription(groupIn.Description).
		SetPlatform(groupIn.Platform).
		SetRateMultiplier(groupIn.RateMultiplier).
		SetSortOrder(groupIn.SortOrder).
		SetIsExclusive(groupIn.IsExclusive).
		SetStatus(groupIn.Status).
		SetSubscriptionType(groupIn.SubscriptionType).
		SetNillableDailyLimitUsd(groupIn.DailyLimitUSD).
		SetNillableWeeklyLimitUsd(groupIn.WeeklyLimitUSD).
		SetNillableMonthlyLimitUsd(groupIn.MonthlyLimitUSD).
		SetAllowImageGeneration(groupIn.AllowImageGeneration).
		SetAllowBatchImageGeneration(groupIn.AllowBatchImageGeneration).
		SetImageRateIndependent(groupIn.ImageRateIndependent).
		SetImageRateMultiplier(groupIn.ImageRateMultiplier).
		SetNillableImagePrice1k(groupIn.ImagePrice1K).
		SetNillableImagePrice2k(groupIn.ImagePrice2K).
		SetNillableImagePrice4k(groupIn.ImagePrice4K).
		SetBatchImageDiscountMultiplier(groupIn.BatchImageDiscountMultiplier).
		SetBatchImageHoldMultiplier(groupIn.BatchImageHoldMultiplier).
		SetDefaultValidityDays(groupIn.DefaultValidityDays).
		SetClaudeCodeOnly(groupIn.ClaudeCodeOnly).
		SetNillableFallbackGroupID(groupIn.FallbackGroupID).
		SetNillableFallbackGroupIDOnInvalidRequest(groupIn.FallbackGroupIDOnInvalidRequest).
		SetModelRoutingEnabled(groupIn.ModelRoutingEnabled).
		SetMcpXMLInject(groupIn.MCPXMLInject).
		SetAllowMessagesDispatch(groupIn.AllowMessagesDispatch).
		SetRequireOauthOnly(groupIn.RequireOAuthOnly).
		SetRequirePrivacySet(groupIn.RequirePrivacySet).
		SetDefaultMappedModel(groupIn.DefaultMappedModel).
		SetMessagesDispatchModelConfig(groupIn.MessagesDispatchModelConfig).
		SetModelsListConfig(groupIn.ModelsListConfig).
		SetRpmLimit(groupIn.RPMLimit).
		SetPeakRateEnabled(groupIn.PeakRateEnabled).
		SetPeakStart(groupIn.PeakStart).
		SetPeakEnd(groupIn.PeakEnd).
		SetPeakRateMultiplier(groupIn.PeakRateMultiplier)

	// 设置模型路由配置
	if groupIn.ModelRouting != nil {
		builder = builder.SetModelRouting(groupIn.ModelRouting)
REDACTED

	// 设置支持的模型系列（始终设置，空数组表示不限制）
	builder = builder.SetSupportedModelScopes(groupIn.SupportedModelScopes)

	created, err := builder.Save(ctx)
	if err == nil {
		groupIn.ID = created.ID
		groupIn.CreatedAt = created.CreatedAt
		groupIn.UpdatedAt = created.UpdatedAt
		if err := enqueueSchedulerOutbox(ctx, r.sql, service.SchedulerOutboxEventGroupChanged, nil, &groupIn.ID, nil); err != nil {
			logger.LegacyPrintf("repository.group", "[SchedulerOutbox] enqueue group create failed: group=%d err=%v", groupIn.ID, err)
	REDACTED
REDACTED
	return translatePersistenceError(err, nil, service.ErrGroupExists)
REDACTED

func (r *groupRepository) GetByID(ctx context.Context, id int64) (*service.Group, error) {
	out, err := r.GetByIDLite(ctx, id)
	if err != nil {
		return nil, err
REDACTED
	counts, err := r.loadAccountCounts(ctx, []int64{out.IDREDACTED)
	if err == nil {
		c := counts[out.ID]
		out.AccountCount = c.Total
		out.ActiveAccountCount = c.Active
		out.RateLimitedAccountCount = c.RateLimited
REDACTED
	return out, nil
REDACTED

func (r *groupRepository) GetByIDLite(ctx context.Context, id int64) (*service.Group, error) {
	// AccountCount is intentionally not loaded here; use GetByID when needed.
	m, err := r.client.Group.Query().
		Where(group.IDEQ(id)).
		Only(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrGroupNotFound, nil)
REDACTED
	return groupEntityToService(m), nil
REDACTED

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
		SetAllowImageGeneration(groupIn.AllowImageGeneration).
		SetAllowBatchImageGeneration(groupIn.AllowBatchImageGeneration).
		SetImageRateIndependent(groupIn.ImageRateIndependent).
		SetImageRateMultiplier(groupIn.ImageRateMultiplier).
		SetNillableImagePrice1k(groupIn.ImagePrice1K).
		SetNillableImagePrice2k(groupIn.ImagePrice2K).
		SetNillableImagePrice4k(groupIn.ImagePrice4K).
		SetBatchImageDiscountMultiplier(groupIn.BatchImageDiscountMultiplier).
		SetBatchImageHoldMultiplier(groupIn.BatchImageHoldMultiplier).
		SetDefaultValidityDays(groupIn.DefaultValidityDays).
		SetClaudeCodeOnly(groupIn.ClaudeCodeOnly).
		SetModelRoutingEnabled(groupIn.ModelRoutingEnabled).
		SetMcpXMLInject(groupIn.MCPXMLInject).
		SetAllowMessagesDispatch(groupIn.AllowMessagesDispatch).
		SetRequireOauthOnly(groupIn.RequireOAuthOnly).
		SetRequirePrivacySet(groupIn.RequirePrivacySet).
		SetDefaultMappedModel(groupIn.DefaultMappedModel).
		SetMessagesDispatchModelConfig(groupIn.MessagesDispatchModelConfig).
		SetModelsListConfig(groupIn.ModelsListConfig).
		SetRpmLimit(groupIn.RPMLimit).
		SetPeakRateEnabled(groupIn.PeakRateEnabled).
		SetPeakStart(groupIn.PeakStart).
		SetPeakEnd(groupIn.PeakEnd).
		SetPeakRateMultiplier(groupIn.PeakRateMultiplier)

	// 显式处理可空字段：nil 需要 clear，非 nil 需要 set。
	if groupIn.DailyLimitUSD != nil {
		builder = builder.SetDailyLimitUsd(*groupIn.DailyLimitUSD)
REDACTED else {
		builder = builder.ClearDailyLimitUsd()
REDACTED
	if groupIn.WeeklyLimitUSD != nil {
		builder = builder.SetWeeklyLimitUsd(*groupIn.WeeklyLimitUSD)
REDACTED else {
		builder = builder.ClearWeeklyLimitUsd()
REDACTED
	if groupIn.MonthlyLimitUSD != nil {
		builder = builder.SetMonthlyLimitUsd(*groupIn.MonthlyLimitUSD)
REDACTED else {
		builder = builder.ClearMonthlyLimitUsd()
REDACTED
	if groupIn.ImagePrice1K != nil {
		builder = builder.SetImagePrice1k(*groupIn.ImagePrice1K)
REDACTED else {
		builder = builder.ClearImagePrice1k()
REDACTED
	if groupIn.ImagePrice2K != nil {
		builder = builder.SetImagePrice2k(*groupIn.ImagePrice2K)
REDACTED else {
		builder = builder.ClearImagePrice2k()
REDACTED
	if groupIn.ImagePrice4K != nil {
		builder = builder.SetImagePrice4k(*groupIn.ImagePrice4K)
REDACTED else {
		builder = builder.ClearImagePrice4k()
REDACTED

	// 处理 FallbackGroupID：nil 时清除，否则设置
	if groupIn.FallbackGroupID != nil {
		builder = builder.SetFallbackGroupID(*groupIn.FallbackGroupID)
REDACTED else {
		builder = builder.ClearFallbackGroupID()
REDACTED
	// 处理 FallbackGroupIDOnInvalidRequest：nil 时清除，否则设置
	if groupIn.FallbackGroupIDOnInvalidRequest != nil {
		builder = builder.SetFallbackGroupIDOnInvalidRequest(*groupIn.FallbackGroupIDOnInvalidRequest)
REDACTED else {
		builder = builder.ClearFallbackGroupIDOnInvalidRequest()
REDACTED

	// 处理 ModelRouting：nil 时清除，否则设置
	if groupIn.ModelRouting != nil {
		builder = builder.SetModelRouting(groupIn.ModelRouting)
REDACTED else {
		builder = builder.ClearModelRouting()
REDACTED

	// 处理 SupportedModelScopes（始终设置，空数组表示不限制）
	builder = builder.SetSupportedModelScopes(groupIn.SupportedModelScopes)

	updated, err := builder.Save(ctx)
	if err != nil {
		return translatePersistenceError(err, service.ErrGroupNotFound, service.ErrGroupExists)
REDACTED
	groupIn.UpdatedAt = updated.UpdatedAt
	if err := enqueueSchedulerOutbox(ctx, r.sql, service.SchedulerOutboxEventGroupChanged, nil, &groupIn.ID, nil); err != nil {
		logger.LegacyPrintf("repository.group", "[SchedulerOutbox] enqueue group update failed: group=%d err=%v", groupIn.ID, err)
REDACTED
	return nil
REDACTED

func (r *groupRepository) Delete(ctx context.Context, id int64) error {
	_, err := r.client.Group.Delete().Where(group.IDEQ(id)).Exec(ctx)
	if err != nil {
		return translatePersistenceError(err, service.ErrGroupNotFound, nil)
REDACTED
	if err := enqueueSchedulerOutbox(ctx, r.sql, service.SchedulerOutboxEventGroupChanged, nil, &id, nil); err != nil {
		logger.LegacyPrintf("repository.group", "[SchedulerOutbox] enqueue group delete failed: group=%d err=%v", id, err)
REDACTED
	return nil
REDACTED

func (r *groupRepository) List(ctx context.Context, params pagination.PaginationParams) ([]service.Group, *pagination.PaginationResult, error) {
	return r.ListWithFilters(ctx, params, "", "", "", nil)
REDACTED

func (r *groupRepository) ListWithFilters(ctx context.Context, params pagination.PaginationParams, platform, status, search string, isExclusive *bool) ([]service.Group, *pagination.PaginationResult, error) {
	q := r.client.Group.Query()

	if platform != "" {
		q = q.Where(group.PlatformEQ(platform))
REDACTED
	if status != "" {
		q = q.Where(group.StatusEQ(status))
REDACTED
	if search != "" {
		q = q.Where(group.Or(
			group.NameContainsFold(search),
			group.DescriptionContainsFold(search),
		))
REDACTED
	if isExclusive != nil {
		q = q.Where(group.IsExclusiveEQ(*isExclusive))
REDACTED

	total, err := q.Clone().Count(ctx)
	if err != nil {
		return nil, nil, err
REDACTED

	if strings.EqualFold(strings.TrimSpace(params.SortBy), "account_count") {
		return r.listWithAccountCountSort(ctx, q, params, total)
REDACTED

	groupsQuery := q.
		Offset(params.Offset()).
		Limit(params.Limit())
	for _, order := range groupListOrder(params) {
		groupsQuery = groupsQuery.Order(order)
REDACTED

	groups, err := groupsQuery.All(ctx)
	if err != nil {
		return nil, nil, err
REDACTED

	groupIDs := make([]int64, 0, len(groups))
	outGroups := make([]service.Group, 0, len(groups))
	for i := range groups {
		g := groupEntityToService(groups[i])
		outGroups = append(outGroups, *g)
		groupIDs = append(groupIDs, g.ID)
REDACTED

	counts, err := r.loadAccountCounts(ctx, groupIDs)
	if err == nil {
		for i := range outGroups {
			c := counts[outGroups[i].ID]
			outGroups[i].AccountCount = c.Total
			outGroups[i].ActiveAccountCount = c.Active
			outGroups[i].RateLimitedAccountCount = c.RateLimited
	REDACTED
REDACTED

	return outGroups, paginationResultFromTotal(int64(total), params), nil
REDACTED

func (r *groupRepository) listWithAccountCountSort(ctx context.Context, q *dbent.GroupQuery, params pagination.PaginationParams, total int) ([]service.Group, *pagination.PaginationResult, error) {
	// 第一步：只查 ID + sort_order（轻量，不做分页 — 需要全量排序 account_count）。
	rows, err := q.Clone().
		Select(group.FieldID, group.FieldSortOrder).
		Order(dbent.Asc(group.FieldSortOrder), dbent.Asc(group.FieldID)).
		All(ctx)
	if err != nil {
		return nil, nil, err
REDACTED

	type sortEntry struct {
		id           int64
		sortOrder    int
		accountCount int64
REDACTED
	entries := make([]sortEntry, 0, len(rows))
	groupIDs := make([]int64, len(rows))
	for i, r := range rows {
		groupIDs[i] = r.ID
		entries = append(entries, sortEntry{id: r.ID, sortOrder: r.SortOrderREDACTED)
REDACTED

	// 第二步：批量加载 account counts（一次 SQL）。
	counts, err := r.loadAccountCounts(ctx, groupIDs)
	if err != nil {
		return nil, nil, err
REDACTED
	for i := range entries {
		c := counts[entries[i].id]
		if c.Total > 0 {
			entries[i].accountCount = c.Total
	REDACTED
REDACTED

	// 第三步：Go 侧排序（数据量 = Group 总数，通常 < 200，安全）。
	sortOrder := params.NormalizedSortOrder(pagination.SortOrderDesc)
	tieCmp := func(a, b sortEntry) bool {
		if a.sortOrder == b.sortOrder {
			return a.id < b.id
	REDACTED
		return a.sortOrder < b.sortOrder
REDACTED
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].accountCount == entries[j].accountCount {
			return tieCmp(entries[i], entries[j])
	REDACTED
		if sortOrder == pagination.SortOrderAsc {
			return entries[i].accountCount < entries[j].accountCount
	REDACTED
		return entries[i].accountCount > entries[j].accountCount
REDACTED)

	// 第四步：分页，只加载当前页需要的完整 Group。
	page := paginateSlice(entries, params)
	if len(page) == 0 {
		return nil, paginationResultFromTotal(int64(total), params), nil
REDACTED

	pageIDs := make([]int64, len(page))
	pageIdx := make(map[int64]int, len(page))
	for i, e := range page {
		pageIDs[i] = e.id
		pageIdx[e.id] = i
REDACTED

	groups, err := r.client.Group.Query().
		Where(group.IDIn(pageIDs...)).
		All(ctx)
	if err != nil {
		return nil, nil, err
REDACTED

	outGroups := make([]service.Group, len(page))
	for i := range groups {
		g := groupEntityToService(groups[i])
		c := counts[g.ID]
		g.AccountCount = c.Total
		g.ActiveAccountCount = c.Active
		g.RateLimitedAccountCount = c.RateLimited
		if idx, ok := pageIdx[g.ID]; ok {
			outGroups[idx] = *g
	REDACTED
REDACTED

	return outGroups, paginationResultFromTotal(int64(total), params), nil
REDACTED

func groupListOrder(params pagination.PaginationParams) []func(*entsql.Selector) {
	sortBy := strings.ToLower(strings.TrimSpace(params.SortBy))
	sortOrder := params.NormalizedSortOrder(pagination.SortOrderAsc)

	var field string
	tieField := group.FieldID
	defaultOrder := true
	switch sortBy {
	case "", "sort_order":
		field = group.FieldSortOrder
	case "name":
		field = group.FieldName
		defaultOrder = false
	case "platform":
		field = group.FieldPlatform
		defaultOrder = false
	case "billing_type", "subscription_type":
		field = group.FieldSubscriptionType
		defaultOrder = false
	case "rate_multiplier":
		field = group.FieldRateMultiplier
		defaultOrder = false
	case "is_exclusive":
		field = group.FieldIsExclusive
		defaultOrder = false
	case "status":
		field = group.FieldStatus
		defaultOrder = false
	case "created_at":
		field = group.FieldCreatedAt
		defaultOrder = false
	case "id":
		field = group.FieldID
		defaultOrder = false
		tieField = ""
	default:
		field = group.FieldSortOrder
REDACTED

	if sortOrder == pagination.SortOrderDesc && sortBy != "" {
		if tieField == "" {
			return []func(*entsql.Selector){dbent.Desc(field)REDACTED
	REDACTED
		return []func(*entsql.Selector){dbent.Desc(field), dbent.Desc(tieField)REDACTED
REDACTED
	if defaultOrder {
		return []func(*entsql.Selector){dbent.Asc(group.FieldSortOrder), dbent.Asc(group.FieldID)REDACTED
REDACTED
	if tieField == "" {
		return []func(*entsql.Selector){dbent.Asc(field)REDACTED
REDACTED
	return []func(*entsql.Selector){dbent.Asc(field), dbent.Asc(tieField)REDACTED
REDACTED

func (r *groupRepository) ListActive(ctx context.Context) ([]service.Group, error) {
	groups, err := r.client.Group.Query().
		Where(group.StatusEQ(service.StatusActive)).
		Order(dbent.Asc(group.FieldSortOrder), dbent.Asc(group.FieldID)).
		All(ctx)
	if err != nil {
		return nil, err
REDACTED

	groupIDs := make([]int64, 0, len(groups))
	outGroups := make([]service.Group, 0, len(groups))
	for i := range groups {
		g := groupEntityToService(groups[i])
		outGroups = append(outGroups, *g)
		groupIDs = append(groupIDs, g.ID)
REDACTED

	counts, err := r.loadAccountCounts(ctx, groupIDs)
	if err == nil {
		for i := range outGroups {
			c := counts[outGroups[i].ID]
			outGroups[i].AccountCount = c.Total
			outGroups[i].ActiveAccountCount = c.Active
			outGroups[i].RateLimitedAccountCount = c.RateLimited
	REDACTED
REDACTED

	return outGroups, nil
REDACTED

func (r *groupRepository) ListActiveByPlatform(ctx context.Context, platform string) ([]service.Group, error) {
	groups, err := r.client.Group.Query().
		Where(group.StatusEQ(service.StatusActive), group.PlatformEQ(platform)).
		Order(dbent.Asc(group.FieldSortOrder), dbent.Asc(group.FieldID)).
		All(ctx)
	if err != nil {
		return nil, err
REDACTED

	groupIDs := make([]int64, 0, len(groups))
	outGroups := make([]service.Group, 0, len(groups))
	for i := range groups {
		g := groupEntityToService(groups[i])
		outGroups = append(outGroups, *g)
		groupIDs = append(groupIDs, g.ID)
REDACTED

	counts, err := r.loadAccountCounts(ctx, groupIDs)
	if err == nil {
		for i := range outGroups {
			c := counts[outGroups[i].ID]
			outGroups[i].AccountCount = c.Total
			outGroups[i].ActiveAccountCount = c.Active
			outGroups[i].RateLimitedAccountCount = c.RateLimited
	REDACTED
REDACTED

	return outGroups, nil
REDACTED

func (r *groupRepository) ExistsByName(ctx context.Context, name string) (bool, error) {
	return r.client.Group.Query().Where(group.NameEQ(name)).Exist(ctx)
REDACTED

// ExistsByIDs 批量检查分组是否存在（仅检查未软删除记录）。
// 返回结构：map[groupID]exists。
func (r *groupRepository) ExistsByIDs(ctx context.Context, ids []int64) (map[int64]bool, error) {
	result := make(map[int64]bool, len(ids))
	if len(ids) == 0 {
		return result, nil
REDACTED

	uniqueIDs := make([]int64, 0, len(ids))
	seen := make(map[int64]struct{REDACTED, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
	REDACTED
		if _, ok := seen[id]; ok {
			continue
	REDACTED
		seen[id] = struct{REDACTED{REDACTED
		uniqueIDs = append(uniqueIDs, id)
		result[id] = false
REDACTED
	if len(uniqueIDs) == 0 {
		return result, nil
REDACTED

	rows, err := r.sql.QueryContext(ctx, `
		SELECT id
		FROM groups
		WHERE id = ANY($1) AND deleted_at IS NULL
	`, pq.Array(uniqueIDs))
	if err != nil {
		return nil, err
REDACTED
	defer func() { _ = rows.Close() REDACTED()

	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
	REDACTED
		result[id] = true
REDACTED
	if err := rows.Err(); err != nil {
		return nil, err
REDACTED
	return result, nil
REDACTED

func (r *groupRepository) GetAccountCount(ctx context.Context, groupID int64) (total int64, active int64, err error) {
	var rateLimited int64
	err = scanSingleRow(ctx, r.sql,
		fmt.Sprintf(`SELECT
			COUNT(*) FILTER (WHERE a.deleted_at IS NULL),
			COUNT(*) FILTER (WHERE %s),
			COUNT(*) FILTER (WHERE %s)
		FROM account_groups ag JOIN accounts a ON a.id = ag.account_id
		WHERE ag.group_id = $1`, groupAccountAvailableSQL, groupAccountTemporarilyLimitedSQL),
		[]any{groupIDREDACTED, &total, &active, &rateLimited)
	return
REDACTED

func (r *groupRepository) DeleteAccountGroupsByGroupID(ctx context.Context, groupID int64) (int64, error) {
	res, err := r.sql.ExecContext(ctx, "DELETE FROM account_groups WHERE group_id = $1", groupID)
	if err != nil {
		return 0, err
REDACTED
	affected, _ := res.RowsAffected()
	if err := enqueueSchedulerOutbox(ctx, r.sql, service.SchedulerOutboxEventGroupChanged, nil, &groupID, nil); err != nil {
		logger.LegacyPrintf("repository.group", "[SchedulerOutbox] enqueue group account clear failed: group=%d err=%v", groupID, err)
REDACTED
	return affected, nil
REDACTED

func (r *groupRepository) DeleteCascade(ctx context.Context, id int64) ([]int64, error) {
	g, err := r.client.Group.Query().Where(group.IDEQ(id)).Only(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrGroupNotFound, nil)
REDACTED
	groupSvc := groupEntityToService(g)

	// 使用 ent 事务统一包裹：避免手工基于 *sql.Tx 构造 ent client 带来的驱动断言问题，
	// 同时保证级联删除的原子性。
	tx, err := r.client.Tx(ctx)
	if err != nil && !errors.Is(err, dbent.ErrTxStarted) {
		return nil, err
REDACTED
	exec := r.client
	txClient := r.client
	if err == nil {
		defer func() { _ = tx.Rollback() REDACTED()
		exec = tx.Client()
		txClient = exec
REDACTED
	// err 为 dbent.ErrTxStarted 时，复用当前 client 参与同一事务。

	// Lock the group row to avoid concurrent writes while we cascade.
	// 这里使用 exec.QueryContext 手动扫描，确保同一事务内加锁并能区分"未找到"与其他错误。
	rows, err := exec.QueryContext(ctx, "SELECT id FROM groups WHERE id = $1 AND deleted_at IS NULL FOR UPDATE", id)
	if err != nil {
		return nil, err
REDACTED
	var lockedID int64
	if rows.Next() {
		if err := rows.Scan(&lockedID); err != nil {
			_ = rows.Close()
			return nil, err
	REDACTED
REDACTED
	if err := rows.Close(); err != nil {
		return nil, err
REDACTED
	if err := rows.Err(); err != nil {
		return nil, err
REDACTED
	if lockedID == 0 {
		return nil, service.ErrGroupNotFound
REDACTED

	var affectedUserIDs []int64
	if groupSvc.IsSubscriptionType() {
		// 只查询未软删除的订阅，避免通知已取消订阅的用户
		rows, err := exec.QueryContext(ctx, "SELECT user_id FROM user_subscriptions WHERE group_id = $1 AND deleted_at IS NULL", id)
		if err != nil {
			return nil, err
	REDACTED
		for rows.Next() {
			var userID int64
			if scanErr := rows.Scan(&userID); scanErr != nil {
				_ = rows.Close()
				return nil, scanErr
		REDACTED
			affectedUserIDs = append(affectedUserIDs, userID)
	REDACTED
		if err := rows.Close(); err != nil {
			return nil, err
	REDACTED
		if err := rows.Err(); err != nil {
			return nil, err
	REDACTED

		// 软删除订阅：设置 deleted_at 而非硬删除
		if _, err := exec.ExecContext(ctx, "UPDATE user_subscriptions SET deleted_at = NOW() WHERE group_id = $1 AND deleted_at IS NULL", id); err != nil {
			return nil, err
	REDACTED
REDACTED

	// 2. Remove the group id from user_allowed_groups join table.
	// Legacy users.allowed_groups 列已弃用，不再同步。
	if _, err := exec.ExecContext(ctx, "DELETE FROM user_allowed_groups WHERE group_id = $1", id); err != nil {
		return nil, err
REDACTED

	// 3. Delete account_groups join rows.
	if _, err := exec.ExecContext(ctx, "DELETE FROM account_groups WHERE group_id = $1", id); err != nil {
		return nil, err
REDACTED

	// 4. Soft-delete group itself.
	if _, err := txClient.Group.Delete().Where(group.IDEQ(id)).Exec(ctx); err != nil {
		return nil, err
REDACTED

	if tx != nil {
		if err := tx.Commit(); err != nil {
			return nil, err
	REDACTED
REDACTED
	if err := enqueueSchedulerOutbox(ctx, r.sql, service.SchedulerOutboxEventGroupChanged, nil, &id, nil); err != nil {
		logger.LegacyPrintf("repository.group", "[SchedulerOutbox] enqueue group cascade delete failed: group=%d err=%v", id, err)
REDACTED

	return affectedUserIDs, nil
REDACTED

type groupAccountCounts struct {
	Total       int64
	Active      int64
	RateLimited int64
REDACTED

const (
	// 分组页的"可用"账号数必须与账号仓储的 ListSchedulableByGroupID 过滤口径一致。
	groupAccountAvailableSQL = `a.deleted_at IS NULL
				AND a.status = 'active'
				AND a.schedulable = true
				AND (a.expires_at IS NULL OR a.expires_at > NOW() OR a.auto_pause_on_expired = FALSE)
				AND (a.rate_limit_reset_at IS NULL OR a.rate_limit_reset_at <= NOW())
				AND (a.overload_until IS NULL OR a.overload_until <= NOW())
				AND (a.temp_unschedulable_until IS NULL OR a.temp_unschedulable_until <= NOW())`

	// 这里沿用历史字段名 RateLimitedAccountCount，但统计的是会让账号暂时退出调度的时间窗口。
	groupAccountTemporarilyLimitedSQL = `a.deleted_at IS NULL
				AND a.status = 'active'
				AND a.schedulable = true
				AND (a.expires_at IS NULL OR a.expires_at > NOW() OR a.auto_pause_on_expired = FALSE)
				AND (
					a.rate_limit_reset_at > NOW() OR
					a.overload_until > NOW() OR
					a.temp_unschedulable_until > NOW()
				)`
)

func (r *groupRepository) loadAccountCounts(ctx context.Context, groupIDs []int64) (counts map[int64]groupAccountCounts, err error) {
	counts = make(map[int64]groupAccountCounts, len(groupIDs))
	if len(groupIDs) == 0 {
		return counts, nil
REDACTED

	rows, err := r.sql.QueryContext(
		ctx,
		fmt.Sprintf(`SELECT ag.group_id,
			COUNT(*) FILTER (WHERE a.deleted_at IS NULL) AS total,
			COUNT(*) FILTER (WHERE %s) AS active,
			COUNT(*) FILTER (WHERE %s) AS rate_limited
		FROM account_groups ag
		JOIN accounts a ON a.id = ag.account_id
		WHERE ag.group_id = ANY($1)
		GROUP BY ag.group_id`, groupAccountAvailableSQL, groupAccountTemporarilyLimitedSQL),
		pq.Array(groupIDs),
	)
	if err != nil {
		return nil, err
REDACTED
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = closeErr
			counts = nil
	REDACTED
REDACTED()

	for rows.Next() {
		var groupID int64
		var c groupAccountCounts
		if err = rows.Scan(&groupID, &c.Total, &c.Active, &c.RateLimited); err != nil {
			return nil, err
	REDACTED
		counts[groupID] = c
REDACTED
	if err = rows.Err(); err != nil {
		return nil, err
REDACTED

	return counts, nil
REDACTED

// GetAccountIDsByGroupIDs 获取多个分组的所有账号 ID（去重）
func (r *groupRepository) GetAccountIDsByGroupIDs(ctx context.Context, groupIDs []int64) ([]int64, error) {
	if len(groupIDs) == 0 {
		return nil, nil
REDACTED

	rows, err := r.sql.QueryContext(
		ctx,
		"SELECT DISTINCT account_id FROM account_groups WHERE group_id = ANY($1) ORDER BY account_id",
		pq.Array(groupIDs),
	)
	if err != nil {
		return nil, err
REDACTED
	defer func() { _ = rows.Close() REDACTED()

	var accountIDs []int64
	for rows.Next() {
		var accountID int64
		if err := rows.Scan(&accountID); err != nil {
			return nil, err
	REDACTED
		accountIDs = append(accountIDs, accountID)
REDACTED
	if err := rows.Err(); err != nil {
		return nil, err
REDACTED

	return accountIDs, nil
REDACTED

// BindAccountsToGroup 将多个账号绑定到指定分组（批量插入，忽略已存在的绑定）
func (r *groupRepository) BindAccountsToGroup(ctx context.Context, groupID int64, accountIDs []int64) error {
	if len(accountIDs) == 0 {
		return nil
REDACTED

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
REDACTED

	// 发送调度器事件
	if err := enqueueSchedulerOutbox(ctx, r.sql, service.SchedulerOutboxEventGroupChanged, nil, &groupID, nil); err != nil {
		logger.LegacyPrintf("repository.group", "[SchedulerOutbox] enqueue bind accounts to group failed: group=%d err=%v", groupID, err)
REDACTED

	return nil
REDACTED

// UpdateSortOrders 批量更新分组排序
func (r *groupRepository) UpdateSortOrders(ctx context.Context, updates []service.GroupSortOrderUpdate) error {
	if len(updates) == 0 {
		return nil
REDACTED

	// 去重后保留最后一次排序值，避免重复 ID 造成 CASE 分支冲突。
	sortOrderByID := make(map[int64]int, len(updates))
	groupIDs := make([]int64, 0, len(updates))
	for _, u := range updates {
		if u.ID <= 0 {
			continue
	REDACTED
		if _, exists := sortOrderByID[u.ID]; !exists {
			groupIDs = append(groupIDs, u.ID)
	REDACTED
		sortOrderByID[u.ID] = u.SortOrder
REDACTED
	if len(groupIDs) == 0 {
		return nil
REDACTED

	// 与旧实现保持一致：任何不存在/已删除的分组都返回 not found，且不执行更新。
	var existingCount int
	if err := scanSingleRow(
		ctx,
		r.sql,
		`SELECT COUNT(*) FROM groups WHERE deleted_at IS NULL AND id = ANY($1)`,
		[]any{pq.Array(groupIDs)REDACTED,
		&existingCount,
	); err != nil {
		return err
REDACTED
	if existingCount != len(groupIDs) {
		return service.ErrGroupNotFound
REDACTED

	args := make([]any, 0, len(groupIDs)*2+1)
	caseClauses := make([]string, 0, len(groupIDs))
	placeholder := 1
	for _, id := range groupIDs {
		caseClauses = append(caseClauses, fmt.Sprintf("WHEN $%d THEN $%d", placeholder, placeholder+1))
		args = append(args, id, sortOrderByID[id])
		placeholder += 2
REDACTED
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
REDACTED
	affected, err := result.RowsAffected()
	if err != nil {
		return err
REDACTED
	if affected != int64(len(groupIDs)) {
		return service.ErrGroupNotFound
REDACTED

	for _, id := range groupIDs {
		if err := enqueueSchedulerOutbox(ctx, r.sql, service.SchedulerOutboxEventGroupChanged, nil, &id, nil); err != nil {
			logger.LegacyPrintf("repository.group", "[SchedulerOutbox] enqueue group sort update failed: group=%d err=%v", id, err)
	REDACTED
REDACTED
	return nil
REDACTED
