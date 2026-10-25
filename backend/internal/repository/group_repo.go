package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
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
}

type groupRepository struct {
	client *dbent.Client
	sql    sqlExecutor
}

// lockLiveGroups makes account-group inserts participate in the same row-lock
// protocol as guarded group deletion. FOR SHARE conflicts with the deleter's
// FOR UPDATE lock, and READ COMMITTED rechecks deleted_at after any wait.
func lockLiveGroups(ctx context.Context, exec sqlExecutor, groupIDs []int64) error {
	if len(groupIDs) == 0 {
		return nil
	}
	unique := make(map[int64]struct{}, len(groupIDs))
	for _, id := range groupIDs {
		unique[id] = struct{}{}
	}
	if IsMySQL() {
		return lockLiveGroupsMySQL(ctx, exec, unique)
	}
	rows, err := exec.QueryContext(ctx, `/* account_group_live_group_lock */
		SELECT id FROM groups
		WHERE id = ANY($1) AND deleted_at IS NULL
		ORDER BY id
		FOR SHARE`, pq.Array(groupIDs))
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	locked := 0
	for rows.Next() {
		locked++
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if locked != len(unique) {
		return service.ErrGroupNotFound
	}
	return nil
}

func NewGroupRepository(client *dbent.Client, sqlDB *sql.DB) service.GroupRepository {
	return newGroupRepositoryWithSQL(client, sqlDB)
}

// NewAdminGroupRepository exposes the atomic group-duplication capability as
// an explicit dependency of the admin service.
func NewAdminGroupRepository(client *dbent.Client, sqlDB *sql.DB) service.AdminGroupRepository {
	return newGroupRepositoryWithSQL(client, sqlDB)
}

func newGroupRepositoryWithSQL(client *dbent.Client, sqlq sqlExecutor) *groupRepository {
	return &groupRepository{client: client, sql: sqlq}
}

func (r *groupRepository) Create(ctx context.Context, groupIn *service.Group) error {
	if err := createGroupRecord(ctx, r.client, groupIn); err != nil {
		return err
	}
	if err := enqueueSchedulerOutbox(ctx, r.sql, service.SchedulerOutboxEventGroupChanged, nil, &groupIn.ID, nil); err != nil {
		logger.LegacyPrintf("repository.group", "[SchedulerOutbox] enqueue group create failed: group=%d err=%v", groupIn.ID, err)
	}
	return nil
}

func createGroupRecord(ctx context.Context, client *dbent.Client, groupIn *service.Group) error {
	if groupIn == nil {
		return errors.New("group is nil")
	}
	modelPricing, err := json.Marshal(groupIn.ModelPricing)
	if err != nil {
		return fmt.Errorf("marshal group model pricing: %w", err)
	}
	builder := client.Group.Create().
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
		SetVideoRateIndependent(groupIn.VideoRateIndependent).
		SetVideoRateMultiplier(groupIn.VideoRateMultiplier).
		SetNillableVideoPrice480p(groupIn.VideoPrice480P).
		SetNillableVideoPrice720p(groupIn.VideoPrice720P).
		SetNillableVideoPrice1080p(groupIn.VideoPrice1080P).
		SetVideoModelPrices(service.NormalizeVideoModelPrices(groupIn.VideoModelPrices)).
		SetNillableWebSearchPricePerCall(groupIn.WebSearchPricePerCall).
		SetNillableSearchPricePer1k(groupIn.SearchPricePer1k).
		SetNillableAudioRealtimePricePerMin(groupIn.AudioRealtimePricePerMin).
		SetNillableAudioTtsPricePerMillionChars(groupIn.AudioTTSPricePerMillionChars).
		SetNillableAudioSttPricePerHour(groupIn.AudioSTTPricePerHour).
		SetLongContextPricingEnabled(groupIn.LongContextPricingEnabled).
		SetModelPricing(modelPricing).
		SetDefaultValidityDays(groupIn.DefaultValidityDays).
		SetClaudeCodeOnly(groupIn.ClaudeCodeOnly).
		SetNillableFallbackGroupID(groupIn.FallbackGroupID).
		SetNillableFallbackGroupIDOnInvalidRequest(groupIn.FallbackGroupIDOnInvalidRequest).
		SetModelRoutingEnabled(groupIn.ModelRoutingEnabled).
		SetMcpXMLInject(groupIn.MCPXMLInject).
		SetAllowMessagesDispatch(groupIn.AllowMessagesDispatch).
		SetAllowLive(groupIn.AllowLive).
		SetForceOpenaiFast(groupIn.ForceOpenAIFast).
		SetFreeOpenaiFast(groupIn.FreeOpenAIFast).
		SetRequireOauthOnly(groupIn.RequireOAuthOnly).
		SetRequirePrivacySet(groupIn.RequirePrivacySet).
		SetDefaultMappedModel(groupIn.DefaultMappedModel).
		SetMessagesDispatchModelConfig(groupIn.MessagesDispatchModelConfig).
		SetModelAllowlist(service.DomainGroupModelAllowlist(groupIn.ModelAllowlist)).
		SetCodexModelsManifestConfig(groupIn.CodexModelsManifestConfig).
		SetRpmLimit(groupIn.RPMLimit).
		SetMaxReasoningEffort(groupIn.MaxReasoningEffort).
		SetMaxReasoningEffortOverLimit(groupIn.MaxReasoningEffortOverLimit).
		SetReasoningEffortMappings(groupIn.ReasoningEffortMappings).
		SetPeakRateEnabled(groupIn.PeakRateEnabled).
		SetPeakStart(groupIn.PeakStart).
		SetPeakEnd(groupIn.PeakEnd).
		SetPeakRateMultiplier(groupIn.PeakRateMultiplier).
		SetProfitControlEnabled(groupIn.ProfitControlEnabled).
		SetProfitMinMargin(groupIn.ProfitMinMargin).
		SetProfitSafetyBuffer(groupIn.ProfitSafetyBuffer)
	if groupIn.DuplicateOperationID != "" {
		builder = builder.SetDuplicateOperationID(groupIn.DuplicateOperationID)
	}

	// 设置模型路由配置
	if groupIn.ModelRouting != nil {
		builder = builder.SetModelRouting(groupIn.ModelRouting)
	}

	// 设置支持的模型系列（始终设置，空数组表示不限制）
	builder = builder.SetSupportedModelScopes(groupIn.SupportedModelScopes)

	if groupIn.I18n != nil {
		builder = builder.SetI18n(groupIn.I18n)
	}

	created, err := builder.Save(ctx)
	if err != nil {
		return translatePersistenceError(err, nil, service.ErrGroupExists)
	}
	groupIn.ID = created.ID
	groupIn.CreatedAt = created.CreatedAt
	groupIn.UpdatedAt = created.UpdatedAt
	return nil
}

func (r *groupRepository) FindByDuplicateOperationID(ctx context.Context, operationID string) (*service.Group, error) {
	operationID = strings.TrimSpace(operationID)
	if operationID == "" {
		return nil, nil
	}
	row, err := r.client.Group.Query().
		Where(group.DuplicateOperationIDEQ(operationID)).
		Order(dbent.Asc(group.FieldID)).
		First(ctx)
	if dbent.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find group duplicate operation: %w", err)
	}
	return groupEntityToService(row), nil
}

// CreateFromSource atomically persists a copied group, clones the source
// account bindings with their exact priorities, and writes its scheduler event.
func (r *groupRepository) CreateFromSource(ctx context.Context, groupIn *service.Group, sourceGroupID int64) error {
	if groupIn == nil {
		return errors.New("group is nil")
	}
	tx, err := r.client.Tx(ctx)
	if err != nil && !errors.Is(err, dbent.ErrTxStarted) {
		return err
	}

	var txClient *dbent.Client
	if err == nil {
		defer func() { _ = tx.Rollback() }()
		txClient = tx.Client()
	} else {
		// Reuse a caller-owned transaction when this repository is already transactional.
		txClient = r.client
	}

	if err := createGroupRecord(ctx, txClient, groupIn); err != nil {
		return err
	}
	if IsMySQL() {
		// MySQL 分支：见 insertAccountBindingsFromSourceGroupMySQL。
		count, insertErr := insertAccountBindingsFromSourceGroupMySQL(ctx, txClient, sourceGroupID, groupIn.ID, groupIn.RequireOAuthOnly, service.AccountTypeAPIKey)
		if insertErr != nil {
			return insertErr
		}
		groupIn.AccountCount = count
	} else {
		result, err := txClient.ExecContext(
			ctx,
			`INSERT INTO account_groups (account_id, group_id, priority, created_at)
			 SELECT ag.account_id, $2, ag.priority, NOW()
			 FROM account_groups ag
			 JOIN accounts a ON a.id = ag.account_id
			 WHERE ag.group_id = $1
			   AND a.deleted_at IS NULL
			   AND (NOT $3 OR a.type <> $4)
			 ON CONFLICT (account_id, group_id) DO NOTHING`,
			sourceGroupID,
			groupIn.ID,
			groupIn.RequireOAuthOnly,
			service.AccountTypeAPIKey,
		)
		if err != nil {
			return err
		}
		if count, countErr := result.RowsAffected(); countErr == nil {
			groupIn.AccountCount = count
		}
	}
	if err := enqueueSchedulerOutbox(ctx, txClient, service.SchedulerOutboxEventGroupChanged, nil, &groupIn.ID, nil); err != nil {
		return err
	}

	if tx != nil {
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

func (r *groupRepository) GetByID(ctx context.Context, id int64) (*service.Group, error) {
	out, err := r.GetByIDLite(ctx, id)
	if err != nil {
		return nil, err
	}
	counts, err := r.loadAccountCounts(ctx, []int64{out.ID})
	if err == nil {
		c := counts[out.ID]
		out.AccountCount = c.Total
		out.ActiveAccountCount = c.Active
		out.RateLimitedAccountCount = c.RateLimited
	}
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
	modelPricing, err := json.Marshal(groupIn.ModelPricing)
	if err != nil {
		return fmt.Errorf("marshal group model pricing: %w", err)
	}
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
		SetVideoRateIndependent(groupIn.VideoRateIndependent).
		SetVideoRateMultiplier(groupIn.VideoRateMultiplier).
		SetNillableVideoPrice480p(groupIn.VideoPrice480P).
		SetNillableVideoPrice720p(groupIn.VideoPrice720P).
		SetNillableVideoPrice1080p(groupIn.VideoPrice1080P).
		SetVideoModelPrices(service.NormalizeVideoModelPrices(groupIn.VideoModelPrices)).
		SetLongContextPricingEnabled(groupIn.LongContextPricingEnabled).
		SetModelPricing(modelPricing).
		SetDefaultValidityDays(groupIn.DefaultValidityDays).
		SetClaudeCodeOnly(groupIn.ClaudeCodeOnly).
		SetModelRoutingEnabled(groupIn.ModelRoutingEnabled).
		SetMcpXMLInject(groupIn.MCPXMLInject).
		SetAllowMessagesDispatch(groupIn.AllowMessagesDispatch).
		SetAllowLive(groupIn.AllowLive).
		SetForceOpenaiFast(groupIn.ForceOpenAIFast).
		SetFreeOpenaiFast(groupIn.FreeOpenAIFast).
		SetRequireOauthOnly(groupIn.RequireOAuthOnly).
		SetRequirePrivacySet(groupIn.RequirePrivacySet).
		SetDefaultMappedModel(groupIn.DefaultMappedModel).
		SetMessagesDispatchModelConfig(groupIn.MessagesDispatchModelConfig).
		SetModelAllowlist(service.DomainGroupModelAllowlist(groupIn.ModelAllowlist)).
		SetCodexModelsManifestConfig(groupIn.CodexModelsManifestConfig).
		SetRpmLimit(groupIn.RPMLimit).
		SetMaxReasoningEffort(groupIn.MaxReasoningEffort).
		SetMaxReasoningEffortOverLimit(groupIn.MaxReasoningEffortOverLimit).
		SetReasoningEffortMappings(groupIn.ReasoningEffortMappings).
		SetPeakRateEnabled(groupIn.PeakRateEnabled).
		SetPeakStart(groupIn.PeakStart).
		SetPeakEnd(groupIn.PeakEnd).
		SetPeakRateMultiplier(groupIn.PeakRateMultiplier).
		SetProfitControlEnabled(groupIn.ProfitControlEnabled).
		SetProfitMinMargin(groupIn.ProfitMinMargin).
		SetProfitSafetyBuffer(groupIn.ProfitSafetyBuffer)

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
	if groupIn.VideoPrice480P != nil {
		builder = builder.SetVideoPrice480p(*groupIn.VideoPrice480P)
	} else {
		builder = builder.ClearVideoPrice480p()
	}
	if groupIn.VideoPrice720P != nil {
		builder = builder.SetVideoPrice720p(*groupIn.VideoPrice720P)
	} else {
		builder = builder.ClearVideoPrice720p()
	}
	if groupIn.VideoPrice1080P != nil {
		builder = builder.SetVideoPrice1080p(*groupIn.VideoPrice1080P)
	} else {
		builder = builder.ClearVideoPrice1080p()
	}
	if groupIn.WebSearchPricePerCall != nil {
		builder = builder.SetWebSearchPricePerCall(*groupIn.WebSearchPricePerCall)
	} else {
		builder = builder.ClearWebSearchPricePerCall()
	}
	if groupIn.SearchPricePer1k != nil {
		builder = builder.SetSearchPricePer1k(*groupIn.SearchPricePer1k)
	} else {
		builder = builder.ClearSearchPricePer1k()
	}
	if groupIn.AudioRealtimePricePerMin != nil {
		builder = builder.SetAudioRealtimePricePerMin(*groupIn.AudioRealtimePricePerMin)
	} else {
		builder = builder.ClearAudioRealtimePricePerMin()
	}
	if groupIn.AudioTTSPricePerMillionChars != nil {
		builder = builder.SetAudioTtsPricePerMillionChars(*groupIn.AudioTTSPricePerMillionChars)
	} else {
		builder = builder.ClearAudioTtsPricePerMillionChars()
	}
	if groupIn.AudioSTTPricePerHour != nil {
		builder = builder.SetAudioSttPricePerHour(*groupIn.AudioSTTPricePerHour)
	} else {
		builder = builder.ClearAudioSttPricePerHour()
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

	i18n := groupIn.I18n
	if i18n == nil {
		i18n = service.GroupI18n{}
	}
	builder = builder.SetI18n(i18n)

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
	return r.listWithFiltersQuery(ctx, q, params, platform, status, search, isExclusive)
}

func (r *groupRepository) ListBindableWithFilters(ctx context.Context, params pagination.PaginationParams, platform, status, search string, isExclusive *bool) ([]service.Group, *pagination.PaginationResult, error) {
	q := r.client.Group.Query().Where(group.PlatformNEQ(service.PlatformComposite))
	return r.listWithFiltersQuery(ctx, q, params, platform, status, search, isExclusive)
}

func (r *groupRepository) listWithFiltersQuery(ctx context.Context, q *dbent.GroupQuery, params pagination.PaginationParams, platform, status, search string, isExclusive *bool) ([]service.Group, *pagination.PaginationResult, error) {

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

	if strings.EqualFold(strings.TrimSpace(params.SortBy), "account_count") {
		return r.listWithAccountCountSort(ctx, q, params, total)
	}

	groupsQuery := q.
		Offset(params.Offset()).
		Limit(params.Limit())
	for _, order := range groupListOrder(params) {
		groupsQuery = groupsQuery.Order(order)
	}

	groups, err := groupsQuery.All(ctx)
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

func (r *groupRepository) listWithAccountCountSort(ctx context.Context, q *dbent.GroupQuery, params pagination.PaginationParams, total int) ([]service.Group, *pagination.PaginationResult, error) {
	// 第一步：只查 ID + sort_order（轻量，不做分页 — 需要全量排序 account_count）。
	rows, err := q.Clone().
		Select(group.FieldID, group.FieldSortOrder).
		Order(dbent.Asc(group.FieldSortOrder), dbent.Asc(group.FieldID)).
		All(ctx)
	if err != nil {
		return nil, nil, err
	}

	type sortEntry struct {
		id           int64
		sortOrder    int
		accountCount int64
	}
	entries := make([]sortEntry, 0, len(rows))
	groupIDs := make([]int64, len(rows))
	for i, r := range rows {
		groupIDs[i] = r.ID
		entries = append(entries, sortEntry{id: r.ID, sortOrder: r.SortOrder})
	}

	// 第二步：批量加载 account counts（一次 SQL）。
	counts, err := r.loadAccountCounts(ctx, groupIDs)
	if err != nil {
		return nil, nil, err
	}
	for i := range entries {
		c := counts[entries[i].id]
		if c.Total > 0 {
			entries[i].accountCount = c.Total
		}
	}

	// 第三步：Go 侧排序（数据量 = Group 总数，通常 < 200，安全）。
	sortOrder := params.NormalizedSortOrder(pagination.SortOrderDesc)
	tieCmp := func(a, b sortEntry) bool {
		if a.sortOrder == b.sortOrder {
			return a.id < b.id
		}
		return a.sortOrder < b.sortOrder
	}
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].accountCount == entries[j].accountCount {
			return tieCmp(entries[i], entries[j])
		}
		if sortOrder == pagination.SortOrderAsc {
			return entries[i].accountCount < entries[j].accountCount
		}
		return entries[i].accountCount > entries[j].accountCount
	})

	// 第四步：分页，只加载当前页需要的完整 Group。
	page := paginateSlice(entries, params)
	if len(page) == 0 {
		return nil, paginationResultFromTotal(int64(total), params), nil
	}

	pageIDs := make([]int64, len(page))
	pageIdx := make(map[int64]int, len(page))
	for i, e := range page {
		pageIDs[i] = e.id
		pageIdx[e.id] = i
	}

	groups, err := r.client.Group.Query().
		Where(group.IDIn(pageIDs...)).
		All(ctx)
	if err != nil {
		return nil, nil, err
	}

	outGroups := make([]service.Group, len(page))
	for i := range groups {
		g := groupEntityToService(groups[i])
		c := counts[g.ID]
		g.AccountCount = c.Total
		g.ActiveAccountCount = c.Active
		g.RateLimitedAccountCount = c.RateLimited
		if idx, ok := pageIdx[g.ID]; ok {
			outGroups[idx] = *g
		}
	}

	return outGroups, paginationResultFromTotal(int64(total), params), nil
}

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
	}

	if sortOrder == pagination.SortOrderDesc && sortBy != "" {
		if tieField == "" {
			return []func(*entsql.Selector){dbent.Desc(field)}
		}
		return []func(*entsql.Selector){dbent.Desc(field), dbent.Desc(tieField)}
	}
	if defaultOrder {
		return []func(*entsql.Selector){dbent.Asc(group.FieldSortOrder), dbent.Asc(group.FieldID)}
	}
	if tieField == "" {
		return []func(*entsql.Selector){dbent.Asc(field)}
	}
	return []func(*entsql.Selector){dbent.Asc(field), dbent.Asc(tieField)}
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

func (r *groupRepository) ListActiveIDs(ctx context.Context) ([]int64, error) {
	if r.sql != nil {
		query := `
			SELECT id
			FROM groups
			WHERE status = $1
			  AND deleted_at IS NULL
			ORDER BY sort_order ASC, id ASC
		`
		if IsMySQL() {
			query = "SELECT id FROM `groups` WHERE status = ? AND deleted_at IS NULL ORDER BY sort_order ASC, id ASC"
		}
		rows, err := r.sql.QueryContext(ctx, query, service.StatusActive)
		if err != nil {
			return nil, err
		}
		defer func() { _ = rows.Close() }()

		ids := make([]int64, 0)
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				return nil, err
			}
			ids = append(ids, id)
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return ids, nil
	}

	groups, err := r.client.Group.Query().
		Where(group.StatusEQ(service.StatusActive)).
		Select(group.FieldID).
		Order(dbent.Asc(group.FieldSortOrder), dbent.Asc(group.FieldID)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	ids := make([]int64, 0, len(groups))
	for i := range groups {
		ids = append(ids, groups[i].ID)
	}
	return ids, nil
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
	if IsMySQL() {
		return r.existsByIDsMySQL(ctx, uniqueIDs, result)
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
	if IsMySQL() {
		return r.getAccountCountMySQL(ctx, groupID)
	}
	var rateLimited int64
	err = scanSingleRow(ctx, r.sql,
		fmt.Sprintf(`SELECT
			COUNT(*) FILTER (WHERE a.deleted_at IS NULL),
			COUNT(*) FILTER (WHERE %s),
			COUNT(*) FILTER (WHERE %s)
		FROM account_groups ag JOIN accounts a ON a.id = ag.account_id
		WHERE ag.group_id = $1`, groupAccountAvailableSQL, groupAccountTemporarilyLimitedSQL),
		[]any{groupID}, &total, &active, &rateLimited)
	return
}

func (r *groupRepository) DeleteAccountGroupsByGroupID(ctx context.Context, groupID int64) (int64, error) {
	// PG "$1" → MySQL "?"（见 DIALECT_MYSQL_GUIDE.md 占位符一节）。
	deleteSQL := "DELETE FROM account_groups WHERE group_id = $1"
	if IsMySQL() {
		deleteSQL = "DELETE FROM account_groups WHERE group_id = ?"
	}
	res, err := r.sql.ExecContext(ctx, deleteSQL, groupID)
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
	return r.deleteCascade(ctx, id, false)
}

func (r *groupRepository) DeleteCascadeIfEmpty(ctx context.Context, id int64) ([]int64, error) {
	return r.deleteCascade(ctx, id, true)
}

func (r *groupRepository) deleteCascade(ctx context.Context, id int64, requireEmpty bool) ([]int64, error) {
	if IsMySQL() {
		return r.deleteCascadeMySQL(ctx, id, requireEmpty)
	}
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
	rows, err := exec.QueryContext(ctx, "SELECT id, subscription_type FROM groups WHERE id = $1 AND deleted_at IS NULL FOR UPDATE", id)
	if err != nil {
		return nil, err
	}
	var lockedID int64
	var subscriptionType string
	if rows.Next() {
		if err := rows.Scan(&lockedID, &subscriptionType); err != nil {
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
	if requireEmpty {
		var hasAccount bool
		if err := scanSingleRow(ctx, exec, `SELECT EXISTS (
			SELECT 1 FROM account_groups ag
			JOIN accounts a ON a.id = ag.account_id
			WHERE ag.group_id = $1 AND a.deleted_at IS NULL
		)`, []any{id}, &hasAccount); err != nil {
			return nil, err
		}
		if hasAccount {
			return nil, service.ErrGroupNotEmpty
		}
	}

	var affectedUserIDs []int64
	if subscriptionType == service.SubscriptionTypeSubscription {
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

	// 2. Remove the group id from user_allowed_groups join table.
	// Legacy users.allowed_groups 列已弃用，不再同步。
	if _, err := exec.ExecContext(ctx, "DELETE FROM user_allowed_groups WHERE group_id = $1", id); err != nil {
		return nil, err
	}

	// 3. Delete account_groups join rows.
	if _, err := exec.ExecContext(ctx, "DELETE FROM account_groups WHERE group_id = $1", id); err != nil {
		return nil, err
	}

	// 4. Soft-delete composite model routes owned by this group.
	if _, err := exec.ExecContext(ctx, "UPDATE composite_model_routes SET deleted_at = NOW() WHERE group_id = $1 AND deleted_at IS NULL", id); err != nil {
		return nil, err
	}

	// 5. Soft-delete group itself.
	if _, err := txClient.Group.Delete().Where(group.IDEQ(id)).Exec(ctx); err != nil {
		return nil, err
	}
	// Lock and validate the text value before using JSONB operators. A legacy
	// malformed setting must not prevent deletion or be silently replaced.
	var moderationValue string
	err = scanSingleRow(ctx, exec, `SELECT value FROM settings WHERE key = $1 FOR UPDATE`,
		[]any{service.SettingKeyContentModerationConfig}, &moderationValue)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	// Keep the moderation scope in sync in the same transaction. Updating the
	// setting row in SQL also serializes concurrent group deletions across nodes.
	if err == nil && json.Valid([]byte(moderationValue)) {
		if _, err := exec.ExecContext(ctx, `UPDATE settings
		SET value = jsonb_set(value::jsonb, '{group_ids}',
			(SELECT COALESCE(jsonb_agg(group_id ORDER BY ord), '[]'::jsonb)
			 FROM jsonb_array_elements(value::jsonb->'group_ids') WITH ORDINALITY AS ids(group_id, ord)
			 WHERE group_id <> to_jsonb($2::bigint)))::text,
			updated_at = NOW()
		WHERE key = $1 AND jsonb_typeof(value::jsonb->'group_ids') = 'array'
			AND value::jsonb->'group_ids' @> jsonb_build_array($2::bigint)`,
			service.SettingKeyContentModerationConfig, id); err != nil {
			return nil, err
		}
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
	}
	if IsMySQL() {
		return r.loadAccountCountsMySQL(ctx, groupIDs)
	}

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
	if IsMySQL() {
		return r.getAccountIDsByGroupIDsMySQL(ctx, groupIDs)
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

	tx, err := r.client.Tx(ctx)
	if err != nil && !errors.Is(err, dbent.ErrTxStarted) {
		return err
	}
	exec := sqlExecutor(r.client)
	if tx != nil {
		defer func() { _ = tx.Rollback() }()
		exec = tx.Client()
	}
	if err := lockLiveGroups(ctx, exec, []int64{groupID}); err != nil {
		return err
	}

	// 使用 INSERT ... ON CONFLICT DO NOTHING 忽略已存在的绑定（PG）；
	// MySQL 分支见 bindAccountsToGroupMySQL（多行 VALUES + ODKU no-op）。
	if IsMySQL() {
		err = bindAccountsToGroupMySQL(ctx, exec, groupID, accountIDs)
	} else {
		_, err = exec.ExecContext(
			ctx,
			`INSERT INTO account_groups (account_id, group_id, priority, created_at)
			 SELECT unnest($1::bigint[]), $2, 50, NOW()
			 ON CONFLICT (account_id, group_id) DO NOTHING`,
			pq.Array(accountIDs),
			groupID,
		)
	}
	if err != nil {
		return err
	}
	if tx != nil {
		if err := tx.Commit(); err != nil {
			return err
		}
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
	if IsMySQL() {
		return r.updateSortOrdersMySQL(ctx, groupIDs, sortOrderByID)
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

// ---------------------------------------------------------------------------
// MySQL 方言实现（见 DIALECT_MYSQL_GUIDE.md）
// ---------------------------------------------------------------------------

// int64Args 把 []int64 展开为 database/sql 可变参数列表，配合 Placeholders(n)
// 生成 IN (?,?,...) 子句（替代 PG 的 `= ANY($1)` + pq.Array）。
func int64Args(ids []int64) []any {
	args := make([]any, len(ids))
	for i, v := range ids {
		args[i] = v
	}
	return args
}

// groupAccountAvailableSQLMySQL / groupAccountTemporarilyLimitedSQLMySQL 是
// groupAccountAvailableSQL / groupAccountTemporarilyLimitedSQL 的 MySQL 版本：
// 语义逐条对应，仅把 NOW() 换成 NOW(3)（DATETIME(3) 毫秒精度对齐）。
const (
	groupAccountAvailableSQLMySQL = `a.deleted_at IS NULL
				AND a.status = 'active'
				AND a.schedulable = true
				AND (a.expires_at IS NULL OR a.expires_at > NOW(3) OR a.auto_pause_on_expired = FALSE)
				AND (a.rate_limit_reset_at IS NULL OR a.rate_limit_reset_at <= NOW(3))
				AND (a.overload_until IS NULL OR a.overload_until <= NOW(3))
				AND (a.temp_unschedulable_until IS NULL OR a.temp_unschedulable_until <= NOW(3))`

	groupAccountTemporarilyLimitedSQLMySQL = `a.deleted_at IS NULL
				AND a.status = 'active'
				AND a.schedulable = true
				AND (a.expires_at IS NULL OR a.expires_at > NOW(3) OR a.auto_pause_on_expired = FALSE)
				AND (
					a.rate_limit_reset_at > NOW(3) OR
					a.overload_until > NOW(3) OR
					a.temp_unschedulable_until > NOW(3)
				)`
)

// lockLiveGroupsMySQL 是 lockLiveGroups 的 MySQL 实现：
// `id = ANY($1)` + pq.Array → IN (?,?,...) 展开；`groups` 是 MySQL 8 保留字需反引号；
// FOR SHARE 行锁 MySQL 8 原生支持，与删除方的 FOR UPDATE 互斥，锁协议与 PG 一致。
func lockLiveGroupsMySQL(ctx context.Context, exec sqlExecutor, unique map[int64]struct{}) error {
	ids := make([]int64, 0, len(unique))
	for id := range unique {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })

	rows, err := exec.QueryContext(ctx, "/* account_group_live_group_lock */\n\t\tSELECT id FROM `groups`\n\t\tWHERE id IN ("+Placeholders(len(ids))+") AND deleted_at IS NULL\n\t\tORDER BY id\n\t\tFOR SHARE", int64Args(ids)...)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	locked := 0
	for rows.Next() {
		locked++
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if locked != len(unique) {
		return service.ErrGroupNotFound
	}
	return nil
}

// insertAccountBindingsFromSourceGroupMySQL 是 CreateFromSource 中「按源分组克隆账号
// 绑定」语句的 MySQL 实现。PG 版是 INSERT..SELECT..ON CONFLICT DO NOTHING；
// MySQL 改为同结构 INSERT..SELECT + ODKU。ODKU 中裸 account_id 会因 JOIN 双方
// （account_groups/accounts）同名列歧义报 1052，故 RHS 用 SELECT 侧别名 ag 限定；
// 且 account_id 是冲突键的组成部分，冲突行上 ag.account_id 与目标行同值，
// 构成幂等 no-op，RowsAffected 语义与 DO NOTHING 一致（仅统计新插入行）。
// REVIEW: ODKU 引用 INSERT..SELECT 源表别名的写法依赖 MySQL 8.0 对
// INSERT..SELECT..ODKU 的作用域解析，上线前需在目标版本上验证。
func insertAccountBindingsFromSourceGroupMySQL(ctx context.Context, exec sqlExecutor, sourceGroupID, newGroupID int64, requireOAuthOnly bool, apiKeyType string) (int64, error) {
	res, err := exec.ExecContext(ctx, `
		INSERT INTO account_groups (account_id, group_id, priority, created_at)
		SELECT ag.account_id, ?, ag.priority, NOW(3)
		FROM account_groups ag
		JOIN accounts a ON a.id = ag.account_id
		WHERE ag.group_id = ?
		  AND a.deleted_at IS NULL
		  AND (NOT ? OR a.type <> ?)
		ON DUPLICATE KEY UPDATE account_id = ag.account_id`,
		newGroupID,
		sourceGroupID,
		requireOAuthOnly,
		apiKeyType,
	)
	if err != nil {
		return 0, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	return affected, nil
}

func (r *groupRepository) existsByIDsMySQL(ctx context.Context, uniqueIDs []int64, result map[int64]bool) (map[int64]bool, error) {
	rows, err := r.sql.QueryContext(ctx, "SELECT id FROM `groups` WHERE id IN ("+Placeholders(len(uniqueIDs))+") AND deleted_at IS NULL", int64Args(uniqueIDs)...)
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

// getAccountCountMySQL 是 GetAccountCount 的 MySQL 实现：
// `COUNT(*) FILTER (WHERE cond)` → `COUNT(CASE WHEN cond THEN 1 END)`
// （非 NULL 才计数，空结果集返回 0，与 FILTER 语义一致）。
func (r *groupRepository) getAccountCountMySQL(ctx context.Context, groupID int64) (total int64, active int64, err error) {
	var rateLimited int64
	err = scanSingleRow(ctx, r.sql,
		fmt.Sprintf(`SELECT
			COUNT(CASE WHEN a.deleted_at IS NULL THEN 1 END),
			COUNT(CASE WHEN %s THEN 1 END),
			COUNT(CASE WHEN %s THEN 1 END)
		FROM account_groups ag JOIN accounts a ON a.id = ag.account_id
		WHERE ag.group_id = ?`, groupAccountAvailableSQLMySQL, groupAccountTemporarilyLimitedSQLMySQL),
		[]any{groupID}, &total, &active, &rateLimited)
	return
}

// deleteCascadeMySQL 是 deleteCascade 的 MySQL 实现：语句级一一对应
// （$n→?、NOW()→NOW(3)、保留字 `groups`/`key` 反引号），事务结构与行锁
// （groups FOR UPDATE、settings FOR UPDATE）与 PG 版完全一致。
// settings 的 group_ids 重排见 removeModerationGroupIDMySQLQuery 注释。
func (r *groupRepository) deleteCascadeMySQL(ctx context.Context, id int64, requireEmpty bool) ([]int64, error) {
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

	// Lock the group row to avoid concurrent writes while we cascade.
	rows, err := exec.QueryContext(ctx, "SELECT id, subscription_type FROM `groups` WHERE id = ? AND deleted_at IS NULL FOR UPDATE", id)
	if err != nil {
		return nil, err
	}
	var lockedID int64
	var subscriptionType string
	if rows.Next() {
		if err := rows.Scan(&lockedID, &subscriptionType); err != nil {
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
	if requireEmpty {
		var hasAccount bool
		if err := scanSingleRow(ctx, exec, `SELECT EXISTS (
			SELECT 1 FROM account_groups ag
			JOIN accounts a ON a.id = ag.account_id
			WHERE ag.group_id = ? AND a.deleted_at IS NULL
		)`, []any{id}, &hasAccount); err != nil {
			return nil, err
		}
		if hasAccount {
			return nil, service.ErrGroupNotEmpty
		}
	}

	var affectedUserIDs []int64
	if subscriptionType == service.SubscriptionTypeSubscription {
		rows, err := exec.QueryContext(ctx, "SELECT user_id FROM user_subscriptions WHERE group_id = ? AND deleted_at IS NULL", id)
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
		if _, err := exec.ExecContext(ctx, "UPDATE user_subscriptions SET deleted_at = NOW(3) WHERE group_id = ? AND deleted_at IS NULL", id); err != nil {
			return nil, err
		}
	}

	// 2. Remove the group id from user_allowed_groups join table.
	if _, err := exec.ExecContext(ctx, "DELETE FROM user_allowed_groups WHERE group_id = ?", id); err != nil {
		return nil, err
	}

	// 3. Delete account_groups join rows.
	if _, err := exec.ExecContext(ctx, "DELETE FROM account_groups WHERE group_id = ?", id); err != nil {
		return nil, err
	}

	// 4. Soft-delete composite model routes owned by this group.
	if _, err := exec.ExecContext(ctx, "UPDATE composite_model_routes SET deleted_at = NOW(3) WHERE group_id = ? AND deleted_at IS NULL", id); err != nil {
		return nil, err
	}

	// 5. Soft-delete group itself.
	if _, err := txClient.Group.Delete().Where(group.IDEQ(id)).Exec(ctx); err != nil {
		return nil, err
	}

	// Lock and validate the text value before using JSON operators.
	var moderationValue string
	err = scanSingleRow(ctx, exec, "SELECT value FROM settings WHERE `key` = ? FOR UPDATE",
		[]any{service.SettingKeyContentModerationConfig}, &moderationValue)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	// Keep the moderation scope in sync in the same transaction. Updating the
	// setting row in SQL also serializes concurrent group deletions across nodes.
	// PG 版 jsonb 重排的语义等价实现见 removeModerationGroupIDMySQLQuery；
	// 输入 value 取自 FOR UPDATE 锁定读快照（同一事务行锁保证其不变）。
	if err == nil && json.Valid([]byte(moderationValue)) {
		if _, err := exec.ExecContext(ctx, removeModerationGroupIDMySQLQuery,
			moderationValue,
			moderationValue,
			strconv.FormatInt(id, 10),
			service.SettingKeyContentModerationConfig,
			strconv.FormatInt(id, 10),
		); err != nil {
			return nil, err
		}
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

// removeModerationGroupIDMySQLQuery 是 deleteCascade 中 content moderation 设置
// group_ids 数组「排除目标分组并保序重排」的 MySQL 等价语句。PG 原语句：
//
//	UPDATE settings
//	SET value = jsonb_set(value::jsonb, '{group_ids}',
//	    (SELECT COALESCE(jsonb_agg(group_id ORDER BY ord), '[]'::jsonb)
//	     FROM jsonb_array_elements(value::jsonb->'group_ids') WITH ORDINALITY AS ids(group_id, ord)
//	     WHERE group_id <> to_jsonb($2::bigint)))::text,
//	    updated_at = NOW()
//	WHERE key = $1 AND jsonb_typeof(value::jsonb->'group_ids') = 'array'
//	    AND value::jsonb->'group_ids' @> jsonb_build_array($2::bigint)
//
// 语义对应关系：
//   - PG 以服务器端 value 列作为 jsonb 输入；MySQL 版（为规避 UPDATE 目标表
//     自引用子查询的 1093 限制）改为绑定 FOR UPDATE 锁定读到的 value 文本，
//     同一事务行锁保证其与列值一致。
//   - jsonb_array_elements WITH ORDINALITY → JSON_TABLE(CAST(? AS JSON),
//     '$.group_ids[*]' COLUMNS (raw JSON PATH '$', ord FOR ORDINALITY))；
//     raw 以 JSON 类型原样带出每个元素（数字/字符串/对象均不误伤）。
//   - `group_id <> to_jsonb($2::bigint)`（JSON 值级不等比较）→
//     NOT JSON_CONTAINS(raw, CAST(? AS JSON))：两者对数值按数值相等比较
//     （5 与 5.0 相等）、字符串 "5" 与数字 5 不相等，语义一致。
//   - jsonb_agg(... ORDER BY ord) → 先在派生表按 ord 排序，再 JSON_ARRAYAGG
//     重聚合（JSON_ARRAYAGG 本身无序）；全部被排除时聚合为 NULL →
//     COALESCE 补 CAST('[]' AS JSON)，对齐 PG 的 COALESCE '[]'::jsonb。
//   - jsonb_set(..., '{group_ids}', x)::text → JSON_SET(CAST(? AS JSON),
//     '$.group_ids', x) 后 CAST AS CHAR 存回文本列。
//   - WHERE 守卫：JSON_TYPE(JSON_EXTRACT(value,'$.group_ids')) = 'ARRAY' ⇔
//     jsonb_typeof(...) = 'array'；JSON_CONTAINS(...) ⇔ @> jsonb_build_array(id)。
//
// REVIEW: JSON_ARRAYAGG 依赖派生表 ORDER BY 的行产出顺序，MySQL 官方未对
// 聚合保序做形式化保证（实践上按序聚合）；如需绝对确定可改为在 Go 侧重排。
// REVIEW: value 为合法 JSON 但非对象时（标量/数组），PG 的 -> 操作符会直接报错
// 导致删除失败；MySQL 的 JSON_EXTRACT 对标量返回 NULL → 守卫不成立 → 跳过更新
// （更宽容）。这是对 PG 现有缺陷行为的刻意偏离，如需严格对齐可改为报错。
const removeModerationGroupIDMySQLQuery = `
	UPDATE settings
	SET value = CAST(JSON_SET(
			CAST(? AS JSON),
			'$.group_ids',
			COALESCE((
				SELECT JSON_ARRAYAGG(ordered.raw)
				FROM (
					SELECT jt.raw
					FROM JSON_TABLE(CAST(? AS JSON), '$.group_ids[*]'
						COLUMNS (raw JSON PATH '$', ord FOR ORDINALITY)) AS jt
					WHERE NOT JSON_CONTAINS(jt.raw, CAST(? AS JSON))
					ORDER BY jt.ord
				) AS ordered
			), CAST('[]' AS JSON))
		) AS CHAR),
		updated_at = NOW(3)
	WHERE ` + "`key`" + ` = ?
	  AND JSON_TYPE(JSON_EXTRACT(value, '$.group_ids')) = 'ARRAY'
	  AND JSON_CONTAINS(JSON_EXTRACT(value, '$.group_ids'), CAST(? AS JSON))
`

// loadAccountCountsMySQL 是 loadAccountCounts 的 MySQL 实现：
// ANY→IN 展开 + FILTER→COUNT(CASE)，其余（GROUP BY、扫描）与 PG 版一致。
func (r *groupRepository) loadAccountCountsMySQL(ctx context.Context, groupIDs []int64) (counts map[int64]groupAccountCounts, err error) {
	counts = make(map[int64]groupAccountCounts, len(groupIDs))
	rows, err := r.sql.QueryContext(
		ctx,
		fmt.Sprintf(`SELECT ag.group_id,
			COUNT(CASE WHEN a.deleted_at IS NULL THEN 1 END) AS total,
			COUNT(CASE WHEN %s THEN 1 END) AS active,
			COUNT(CASE WHEN %s THEN 1 END) AS rate_limited
		FROM account_groups ag
		JOIN accounts a ON a.id = ag.account_id
		WHERE ag.group_id IN (%s)
		GROUP BY ag.group_id`, groupAccountAvailableSQLMySQL, groupAccountTemporarilyLimitedSQLMySQL, Placeholders(len(groupIDs))),
		int64Args(groupIDs)...,
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

// getAccountIDsByGroupIDsMySQL 是 GetAccountIDsByGroupIDs 的 MySQL 实现：
// `group_id = ANY($1)` → IN 展开，其余一致。
func (r *groupRepository) getAccountIDsByGroupIDsMySQL(ctx context.Context, groupIDs []int64) ([]int64, error) {
	rows, err := r.sql.QueryContext(
		ctx,
		"SELECT DISTINCT account_id FROM account_groups WHERE group_id IN ("+Placeholders(len(groupIDs))+") ORDER BY account_id",
		int64Args(groupIDs)...,
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

// bindAccountsToGroupMySQL 是 BindAccountsToGroup 中批量绑定语句的 MySQL 实现：
// `SELECT unnest($1::bigint[]), $2, 50, NOW()` → 多行 VALUES (?,?,50,NOW(3)) 展开；
// ON CONFLICT DO NOTHING → ODKU no-op（account_groups 为 (account_id, group_id)
// 复合主键、无 id 列，裸 account_id 无歧义；赋同值不影响受影响行数语义，
// 本调用方也不消费受影响行数）。
func bindAccountsToGroupMySQL(ctx context.Context, exec sqlExecutor, groupID int64, accountIDs []int64) error {
	var query strings.Builder
	query.WriteString("INSERT INTO account_groups (account_id, group_id, priority, created_at) VALUES ")
	args := make([]any, 0, len(accountIDs)*2)
	for i, accountID := range accountIDs {
		if i > 0 {
			query.WriteString(",")
		}
		query.WriteString("(?, ?, 50, NOW(3))")
		args = append(args, accountID, groupID)
	}
	query.WriteString(" ON DUPLICATE KEY UPDATE account_id = account_id")
	_, err := exec.ExecContext(ctx, query.String(), args...)
	return err
}

// updateSortOrdersMySQL 是 UpdateSortOrders 的 MySQL 实现。
// 与 PG 版的差异点：
//  1. `id = ANY($n)` → IN 展开，`groups` 反引号；
//  2. MySQL 的 UPDATE RowsAffected 只统计「值发生变化的行」（除非 DSN 开
//     clientFoundRows），而 PG 统计所有命中行——若沿用 `affected != len` 判定，
//     排序值完全未变的合法请求会被误判为 ErrGroupNotFound。因此先用一条普通
//     SELECT 同时完成存在性校验（替代 COUNT(*) 预检）并推算预期变化行数
//     expectedChanged，再与 UPDATE 的 RowsAffected 严格比对。
//
// REVIEW: SELECT 与 UPDATE 之间存在微小时窗，若并发写恰好改动了某行的
// sort_order，expectedChanged 可能与实际变化行数不符而误报 ErrGroupNotFound
// （PG 版同窗口语义是「命中行数变少而报 not found」，仅在值恰好变化的极端
// 场景下有差异）。
func (r *groupRepository) updateSortOrdersMySQL(ctx context.Context, groupIDs []int64, sortOrderByID map[int64]int) error {
	rows, err := r.sql.QueryContext(ctx,
		"SELECT id, sort_order FROM `groups` WHERE deleted_at IS NULL AND id IN ("+Placeholders(len(groupIDs))+")",
		int64Args(groupIDs)...)
	if err != nil {
		return err
	}
	expectedChanged := int64(0)
	seen := 0
	for rows.Next() {
		var id int64
		var sortOrder int
		if err := rows.Scan(&id, &sortOrder); err != nil {
			_ = rows.Close()
			return err
		}
		seen++
		if v, ok := sortOrderByID[id]; ok && v != sortOrder {
			expectedChanged++
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if seen != len(groupIDs) {
		return service.ErrGroupNotFound
	}

	var update strings.Builder
	update.WriteString("UPDATE `groups` SET sort_order = CASE")
	updateArgs := make([]any, 0, len(groupIDs)*3)
	for _, id := range groupIDs {
		update.WriteString(" WHEN id = ? THEN ?")
		updateArgs = append(updateArgs, id, sortOrderByID[id])
	}
	update.WriteString(" ELSE sort_order END WHERE deleted_at IS NULL AND id IN (")
	update.WriteString(Placeholders(len(groupIDs)))
	update.WriteString(")")
	updateArgs = append(updateArgs, int64Args(groupIDs)...)

	result, err := r.sql.ExecContext(ctx, update.String(), updateArgs...)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != expectedChanged {
		return service.ErrGroupNotFound
	}

	for _, id := range groupIDs {
		if err := enqueueSchedulerOutbox(ctx, r.sql, service.SchedulerOutboxEventGroupChanged, nil, &id, nil); err != nil {
			logger.LegacyPrintf("repository.group", "[SchedulerOutbox] enqueue group sort update failed: group=%d err=%v", id, err)
		}
	}
	return nil
}
