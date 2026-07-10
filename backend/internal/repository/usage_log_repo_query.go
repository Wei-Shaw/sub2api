package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	dbaccount "github.com/Wei-Shaw/sub2api/ent/account"
	dbapikey "github.com/Wei-Shaw/sub2api/ent/apikey"
	dbgroup "github.com/Wei-Shaw/sub2api/ent/group"
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"
	dbuser "github.com/Wei-Shaw/sub2api/ent/user"
	dbusersub "github.com/Wei-Shaw/sub2api/ent/usersubscription"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

const usageLogSelectColumns = "id, user_id, api_key_id, account_id, request_id, model, requested_model, upstream_model, group_id, subscription_id, input_tokens, output_tokens, cache_creation_tokens, cache_read_tokens, cache_creation_5m_tokens, cache_creation_1h_tokens, image_output_tokens, image_output_cost, input_cost, output_cost, cache_creation_cost, cache_read_cost, total_cost, actual_cost, rate_multiplier, account_rate_multiplier, billing_type, request_type, stream, openai_ws_mode, duration_ms, first_token_ms, user_agent, ip_address, image_count, image_size, image_input_size, image_output_size, image_size_source, image_size_breakdown, video_count, video_resolution, video_duration_seconds, service_tier, reasoning_effort, inbound_endpoint, upstream_endpoint, cache_ttl_overridden, channel_id, model_mapping_chain, billing_tier, billing_mode, account_stats_cost, created_at"

func (r *usageLogRepository) GetByID(ctx context.Context, id int64) (log *service.UsageLog, err error) {
	query := "SELECT " + usageLogSelectColumns + " FROM usage_logs WHERE id = $1"
	rows, err := r.sql.QueryContext(ctx, query, id)
	if err != nil {
		return nil, err
	}
	defer func() {
		// 保持主错误优先；仅在无错误时回传 Close 失败。
		// 同时清空返回值，避免误用不完整结果。
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = closeErr
			log = nil
		}
	}()
	if !rows.Next() {
		if err = rows.Err(); err != nil {
			return nil, err
		}
		return nil, service.ErrUsageLogNotFound
	}
	log, err = scanUsageLog(rows)
	if err != nil {
		return nil, err
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return log, nil
}

func (r *usageLogRepository) ListByUser(ctx context.Context, userID int64, params pagination.PaginationParams) ([]service.UsageLog, *pagination.PaginationResult, error) {
	return r.listUsageLogsWithPagination(ctx, "WHERE user_id = $1", []any{userID}, params)
}

func (r *usageLogRepository) ListByAPIKey(ctx context.Context, apiKeyID int64, params pagination.PaginationParams) ([]service.UsageLog, *pagination.PaginationResult, error) {
	return r.listUsageLogsWithPagination(ctx, "WHERE api_key_id = $1", []any{apiKeyID}, params)
}

func (r *usageLogRepository) ListByAccount(ctx context.Context, accountID int64, params pagination.PaginationParams) ([]service.UsageLog, *pagination.PaginationResult, error) {
	return r.listUsageLogsWithPagination(ctx, "WHERE account_id = $1", []any{accountID}, params)
}

func (r *usageLogRepository) ListByUserAndTimeRange(ctx context.Context, userID int64, startTime, endTime time.Time) ([]service.UsageLog, *pagination.PaginationResult, error) {
	query := "SELECT " + usageLogSelectColumns + " FROM usage_logs WHERE user_id = $1 AND created_at >= $2 AND created_at < $3 ORDER BY id DESC LIMIT 10000"
	logs, err := r.queryUsageLogs(ctx, query, userID, startTime, endTime)
	return logs, nil, err
}

func (r *usageLogRepository) ListByAPIKeyAndTimeRange(ctx context.Context, apiKeyID int64, startTime, endTime time.Time) ([]service.UsageLog, *pagination.PaginationResult, error) {
	query := "SELECT " + usageLogSelectColumns + " FROM usage_logs WHERE api_key_id = $1 AND created_at >= $2 AND created_at < $3 ORDER BY id DESC LIMIT 10000"
	logs, err := r.queryUsageLogs(ctx, query, apiKeyID, startTime, endTime)
	return logs, nil, err
}

func (r *usageLogRepository) ListByAccountAndTimeRange(ctx context.Context, accountID int64, startTime, endTime time.Time) ([]service.UsageLog, *pagination.PaginationResult, error) {
	query := "SELECT " + usageLogSelectColumns + " FROM usage_logs WHERE account_id = $1 AND created_at >= $2 AND created_at < $3 ORDER BY id DESC LIMIT 10000"
	logs, err := r.queryUsageLogs(ctx, query, accountID, startTime, endTime)
	return logs, nil, err
}

func (r *usageLogRepository) ListByModelAndTimeRange(ctx context.Context, modelName string, startTime, endTime time.Time) ([]service.UsageLog, *pagination.PaginationResult, error) {
	query := fmt.Sprintf("SELECT %s FROM usage_logs WHERE %s = $1 AND created_at >= $2 AND created_at < $3 ORDER BY id DESC LIMIT 10000", usageLogSelectColumns, rawUsageLogModelColumn)
	logs, err := r.queryUsageLogs(ctx, query, modelName, startTime, endTime)
	return logs, nil, err
}

func (r *usageLogRepository) Delete(ctx context.Context, id int64) error {
	_, err := r.sql.ExecContext(ctx, "DELETE FROM usage_logs WHERE id = $1", id)
	return err
}

// UsageLogFilters represents filters for usage log queries
type UsageLogFilters = usagestats.UsageLogFilters

// ListWithFilters lists usage logs with optional filters (for admin)
func (r *usageLogRepository) ListWithFilters(ctx context.Context, params pagination.PaginationParams, filters UsageLogFilters) ([]service.UsageLog, *pagination.PaginationResult, error) {
	conditions := make([]string, 0, 9)
	args := make([]any, 0, 9)

	if filters.UserID > 0 {
		conditions = append(conditions, fmt.Sprintf("user_id = $%d", len(args)+1))
		args = append(args, filters.UserID)
	}
	if filters.APIKeyID > 0 {
		conditions = append(conditions, fmt.Sprintf("api_key_id = $%d", len(args)+1))
		args = append(args, filters.APIKeyID)
	}
	if filters.AccountID > 0 {
		conditions = append(conditions, fmt.Sprintf("account_id = $%d", len(args)+1))
		args = append(args, filters.AccountID)
	}
	if filters.GroupID > 0 {
		conditions = append(conditions, fmt.Sprintf("group_id = $%d", len(args)+1))
		args = append(args, filters.GroupID)
	}
	conditions, args = appendUsageLogModelWhereCondition(conditions, args, filters.Model, filters.ModelFilterSource)
	conditions, args = appendRequestTypeOrStreamWhereCondition(conditions, args, filters.RequestType, filters.Stream)
	if filters.BillingType != nil {
		conditions = append(conditions, fmt.Sprintf("billing_type = $%d", len(args)+1))
		args = append(args, int16(*filters.BillingType))
	}
	conditions, args = appendUsageLogBillingModeWhereCondition(conditions, args, filters.BillingMode)
	if filters.StartTime != nil {
		conditions = append(conditions, fmt.Sprintf("created_at >= $%d", len(args)+1))
		args = append(args, *filters.StartTime)
	}
	if filters.EndTime != nil {
		conditions = append(conditions, fmt.Sprintf("created_at < $%d", len(args)+1))
		args = append(args, *filters.EndTime)
	}

	whereClause := buildWhere(conditions)
	var (
		logs []service.UsageLog
		page *pagination.PaginationResult
		err  error
	)
	if shouldUseFastUsageLogTotal(filters) {
		logs, page, err = r.listUsageLogsWithFastPagination(ctx, whereClause, args, params)
	} else {
		logs, page, err = r.listUsageLogsWithPagination(ctx, whereClause, args, params)
	}
	if err != nil {
		return nil, nil, err
	}

	if err := r.hydrateUsageLogAssociations(ctx, logs); err != nil {
		return nil, nil, err
	}
	return logs, page, nil
}

func shouldUseFastUsageLogTotal(filters UsageLogFilters) bool {
	if filters.ExactTotal {
		return false
	}
	// 强选择过滤下记录集通常较小，保留精确总数。
	return filters.UserID == 0 && filters.APIKeyID == 0 && filters.AccountID == 0
}

func (r *usageLogRepository) listUsageLogsWithPagination(ctx context.Context, whereClause string, args []any, params pagination.PaginationParams) ([]service.UsageLog, *pagination.PaginationResult, error) {
	countQuery := "SELECT COUNT(*) FROM usage_logs " + whereClause
	var total int64
	if err := scanSingleRow(ctx, r.sql, countQuery, args, &total); err != nil {
		return nil, nil, err
	}

	limitPos := len(args) + 1
	offsetPos := len(args) + 2
	listArgs := append(append([]any{}, args...), params.Limit(), params.Offset())
	query := fmt.Sprintf("SELECT %s FROM usage_logs %s ORDER BY %s LIMIT $%d OFFSET $%d", usageLogSelectColumns, whereClause, usageLogOrderBy(params), limitPos, offsetPos)
	logs, err := r.queryUsageLogs(ctx, query, listArgs...)
	if err != nil {
		return nil, nil, err
	}
	return logs, paginationResultFromTotal(total, params), nil
}

func (r *usageLogRepository) listUsageLogsWithFastPagination(ctx context.Context, whereClause string, args []any, params pagination.PaginationParams) ([]service.UsageLog, *pagination.PaginationResult, error) {
	limit := params.Limit()
	offset := params.Offset()

	limitPos := len(args) + 1
	offsetPos := len(args) + 2
	listArgs := append(append([]any{}, args...), limit+1, offset)
	query := fmt.Sprintf("SELECT %s FROM usage_logs %s ORDER BY %s LIMIT $%d OFFSET $%d", usageLogSelectColumns, whereClause, usageLogOrderBy(params), limitPos, offsetPos)

	logs, err := r.queryUsageLogs(ctx, query, listArgs...)
	if err != nil {
		return nil, nil, err
	}

	hasMore := false
	if len(logs) > limit {
		hasMore = true
		logs = logs[:limit]
	}

	total := int64(offset) + int64(len(logs))
	if hasMore {
		// 只保证“还有下一页”，避免对超大表做全量 COUNT(*)。
		total = int64(offset) + int64(limit) + 1
	}

	return logs, paginationResultFromTotal(total, params), nil
}

func usageLogOrderBy(params pagination.PaginationParams) string {
	sortBy := strings.ToLower(strings.TrimSpace(params.SortBy))
	sortOrder := strings.ToUpper(params.NormalizedSortOrder(pagination.SortOrderDesc))

	var column string
	switch sortBy {
	case "model":
		column = "COALESCE(NULLIF(TRIM(requested_model), ''), model)"
	case "created_at":
		column = "created_at"
	default:
		column = "id"
	}

	if column == "id" {
		return fmt.Sprintf("id %s", sortOrder)
	}
	return fmt.Sprintf("%s %s, id %s", column, sortOrder, sortOrder)
}

func (r *usageLogRepository) queryUsageLogs(ctx context.Context, query string, args ...any) (logs []service.UsageLog, err error) {
	rows, err := r.sql.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() {
		// 保持主错误优先；仅在无错误时回传 Close 失败。
		// 同时清空返回值，避免误用不完整结果。
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = closeErr
			logs = nil
		}
	}()

	logs = make([]service.UsageLog, 0)
	for rows.Next() {
		var log *service.UsageLog
		log, err = scanUsageLog(rows)
		if err != nil {
			return nil, err
		}
		logs = append(logs, *log)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return logs, nil
}

func (r *usageLogRepository) hydrateUsageLogAssociations(ctx context.Context, logs []service.UsageLog) error {
	// 关联数据使用 Ent 批量加载，避免把复杂 SQL 继续膨胀。
	if len(logs) == 0 {
		return nil
	}

	ids := collectUsageLogIDs(logs)
	users, err := r.loadUsers(ctx, ids.userIDs)
	if err != nil {
		return err
	}
	apiKeys, err := r.loadAPIKeys(ctx, ids.apiKeyIDs)
	if err != nil {
		return err
	}
	accounts, err := r.loadAccounts(ctx, ids.accountIDs)
	if err != nil {
		return err
	}
	groups, err := r.loadGroups(ctx, ids.groupIDs)
	if err != nil {
		return err
	}
	subs, err := r.loadSubscriptions(ctx, ids.subscriptionIDs)
	if err != nil {
		return err
	}

	for i := range logs {
		if user, ok := users[logs[i].UserID]; ok {
			logs[i].User = user
		}
		if key, ok := apiKeys[logs[i].APIKeyID]; ok {
			logs[i].APIKey = key
		}
		if acc, ok := accounts[logs[i].AccountID]; ok {
			logs[i].Account = acc
		}
		if logs[i].GroupID != nil {
			if group, ok := groups[*logs[i].GroupID]; ok {
				logs[i].Group = group
			}
		}
		if logs[i].SubscriptionID != nil {
			if sub, ok := subs[*logs[i].SubscriptionID]; ok {
				logs[i].Subscription = sub
			}
		}
	}
	return nil
}

type usageLogIDs struct {
	userIDs         []int64
	apiKeyIDs       []int64
	accountIDs      []int64
	groupIDs        []int64
	subscriptionIDs []int64
}

func collectUsageLogIDs(logs []service.UsageLog) usageLogIDs {
	idSet := func() map[int64]struct{} { return make(map[int64]struct{}) }

	userIDs := idSet()
	apiKeyIDs := idSet()
	accountIDs := idSet()
	groupIDs := idSet()
	subscriptionIDs := idSet()

	for i := range logs {
		userIDs[logs[i].UserID] = struct{}{}
		apiKeyIDs[logs[i].APIKeyID] = struct{}{}
		accountIDs[logs[i].AccountID] = struct{}{}
		if logs[i].GroupID != nil {
			groupIDs[*logs[i].GroupID] = struct{}{}
		}
		if logs[i].SubscriptionID != nil {
			subscriptionIDs[*logs[i].SubscriptionID] = struct{}{}
		}
	}

	return usageLogIDs{
		userIDs:         setToSlice(userIDs),
		apiKeyIDs:       setToSlice(apiKeyIDs),
		accountIDs:      setToSlice(accountIDs),
		groupIDs:        setToSlice(groupIDs),
		subscriptionIDs: setToSlice(subscriptionIDs),
	}
}

func (r *usageLogRepository) loadUsers(ctx context.Context, ids []int64) (map[int64]*service.User, error) {
	out := make(map[int64]*service.User)
	if len(ids) == 0 {
		return out, nil
	}
	// 无条件穿透软删除：ids 来自调用方已按 user_id 筛选的日志行；普通用户路径强制 UserID=本人（本人必为活跃用户），不会借此解析他人已删身份；仅 admin 路径可借此显示已删用户。
	models, err := r.client.User.Query().Where(dbuser.IDIn(ids...)).All(mixins.SkipSoftDelete(ctx))
	if err != nil {
		return nil, err
	}
	for _, m := range models {
		out[m.ID] = userEntityToService(m)
	}
	return out, nil
}

func (r *usageLogRepository) loadAPIKeys(ctx context.Context, ids []int64) (map[int64]*service.APIKey, error) {
	out := make(map[int64]*service.APIKey)
	if len(ids) == 0 {
		return out, nil
	}
	models, err := r.client.APIKey.Query().Where(dbapikey.IDIn(ids...)).All(ctx)
	if err != nil {
		return nil, err
	}
	for _, m := range models {
		out[m.ID] = apiKeyEntityToService(m)
	}
	return out, nil
}

func (r *usageLogRepository) loadAccounts(ctx context.Context, ids []int64) (map[int64]*service.Account, error) {
	out := make(map[int64]*service.Account)
	if len(ids) == 0 {
		return out, nil
	}
	models, err := r.client.Account.Query().Where(dbaccount.IDIn(ids...)).All(ctx)
	if err != nil {
		return nil, err
	}
	for _, m := range models {
		out[m.ID] = accountEntityToService(m)
	}
	return out, nil
}

func (r *usageLogRepository) loadGroups(ctx context.Context, ids []int64) (map[int64]*service.Group, error) {
	out := make(map[int64]*service.Group)
	if len(ids) == 0 {
		return out, nil
	}
	models, err := r.client.Group.Query().Where(dbgroup.IDIn(ids...)).All(ctx)
	if err != nil {
		return nil, err
	}
	for _, m := range models {
		out[m.ID] = groupEntityToService(m)
	}
	return out, nil
}

func (r *usageLogRepository) loadSubscriptions(ctx context.Context, ids []int64) (map[int64]*service.UserSubscription, error) {
	out := make(map[int64]*service.UserSubscription)
	if len(ids) == 0 {
		return out, nil
	}
	models, err := r.client.UserSubscription.Query().Where(dbusersub.IDIn(ids...)).All(ctx)
	if err != nil {
		return nil, err
	}
	for _, m := range models {
		out[m.ID] = userSubscriptionEntityToService(m)
	}
	return out, nil
}

func scanUsageLog(scanner interface{ Scan(...any) error }) (*service.UsageLog, error) {
	var (
		id                    int64
		userID                int64
		apiKeyID              int64
		accountID             int64
		requestID             sql.NullString
		model                 string
		requestedModel        sql.NullString
		upstreamModel         sql.NullString
		groupID               sql.NullInt64
		subscriptionID        sql.NullInt64
		inputTokens           int
		outputTokens          int
		cacheCreationTokens   int
		cacheReadTokens       int
		cacheCreation5m       int
		cacheCreation1h       int
		imageOutputTokens     int
		imageOutputCost       float64
		inputCost             float64
		outputCost            float64
		cacheCreationCost     float64
		cacheReadCost         float64
		totalCost             float64
		actualCost            float64
		rateMultiplier        float64
		accountRateMultiplier sql.NullFloat64
		billingType           int16
		requestTypeRaw        int16
		stream                bool
		openaiWSMode          bool
		durationMs            sql.NullInt64
		firstTokenMs          sql.NullInt64
		userAgent             sql.NullString
		ipAddress             sql.NullString
		imageCount            int
		imageSize             sql.NullString
		imageInputSize        sql.NullString
		imageOutputSize       sql.NullString
		imageSizeSource       sql.NullString
		imageSizeBreakdown    sql.NullString
		videoCount            int
		videoResolution       sql.NullString
		videoDurationSeconds  sql.NullInt64
		serviceTier           sql.NullString
		reasoningEffort       sql.NullString
		inboundEndpoint       sql.NullString
		upstreamEndpoint      sql.NullString
		cacheTTLOverridden    bool
		channelID             sql.NullInt64
		modelMappingChain     sql.NullString
		billingTier           sql.NullString
		billingMode           sql.NullString
		accountStatsCost      sql.NullFloat64
		createdAt             time.Time
	)

	if err := scanner.Scan(
		&id,
		&userID,
		&apiKeyID,
		&accountID,
		&requestID,
		&model,
		&requestedModel,
		&upstreamModel,
		&groupID,
		&subscriptionID,
		&inputTokens,
		&outputTokens,
		&cacheCreationTokens,
		&cacheReadTokens,
		&cacheCreation5m,
		&cacheCreation1h,
		&imageOutputTokens,
		&imageOutputCost,
		&inputCost,
		&outputCost,
		&cacheCreationCost,
		&cacheReadCost,
		&totalCost,
		&actualCost,
		&rateMultiplier,
		&accountRateMultiplier,
		&billingType,
		&requestTypeRaw,
		&stream,
		&openaiWSMode,
		&durationMs,
		&firstTokenMs,
		&userAgent,
		&ipAddress,
		&imageCount,
		&imageSize,
		&imageInputSize,
		&imageOutputSize,
		&imageSizeSource,
		&imageSizeBreakdown,
		&videoCount,
		&videoResolution,
		&videoDurationSeconds,
		&serviceTier,
		&reasoningEffort,
		&inboundEndpoint,
		&upstreamEndpoint,
		&cacheTTLOverridden,
		&channelID,
		&modelMappingChain,
		&billingTier,
		&billingMode,
		&accountStatsCost,
		&createdAt,
	); err != nil {
		return nil, err
	}

	log := &service.UsageLog{
		ID:                    id,
		UserID:                userID,
		APIKeyID:              apiKeyID,
		AccountID:             accountID,
		Model:                 model,
		RequestedModel:        coalesceTrimmedString(requestedModel, model),
		InputTokens:           inputTokens,
		OutputTokens:          outputTokens,
		CacheCreationTokens:   cacheCreationTokens,
		CacheReadTokens:       cacheReadTokens,
		CacheCreation5mTokens: cacheCreation5m,
		CacheCreation1hTokens: cacheCreation1h,
		ImageOutputTokens:     imageOutputTokens,
		ImageOutputCost:       imageOutputCost,
		InputCost:             inputCost,
		OutputCost:            outputCost,
		CacheCreationCost:     cacheCreationCost,
		CacheReadCost:         cacheReadCost,
		TotalCost:             totalCost,
		ActualCost:            actualCost,
		RateMultiplier:        rateMultiplier,
		AccountRateMultiplier: nullFloat64Ptr(accountRateMultiplier),
		BillingType:           int8(billingType),
		RequestType:           service.RequestTypeFromInt16(requestTypeRaw),
		ImageCount:            imageCount,
		VideoCount:            videoCount,
		CacheTTLOverridden:    cacheTTLOverridden,
		CreatedAt:             createdAt,
	}
	// 先回填 legacy 字段，再基于 legacy + request_type 计算最终请求类型，保证历史数据兼容。
	log.Stream = stream
	log.OpenAIWSMode = openaiWSMode
	log.RequestType = log.EffectiveRequestType()
	log.Stream, log.OpenAIWSMode = service.ApplyLegacyRequestFields(log.RequestType, stream, openaiWSMode)

	if requestID.Valid {
		log.RequestID = requestID.String
	}
	if groupID.Valid {
		value := groupID.Int64
		log.GroupID = &value
	}
	if subscriptionID.Valid {
		value := subscriptionID.Int64
		log.SubscriptionID = &value
	}
	if durationMs.Valid {
		value := int(durationMs.Int64)
		log.DurationMs = &value
	}
	if firstTokenMs.Valid {
		value := int(firstTokenMs.Int64)
		log.FirstTokenMs = &value
	}
	if userAgent.Valid {
		log.UserAgent = &userAgent.String
	}
	if ipAddress.Valid {
		log.IPAddress = &ipAddress.String
	}
	if imageSize.Valid {
		log.ImageSize = &imageSize.String
	}
	if imageInputSize.Valid {
		log.ImageInputSize = &imageInputSize.String
	}
	if imageOutputSize.Valid {
		log.ImageOutputSize = &imageOutputSize.String
	}
	if imageSizeSource.Valid {
		log.ImageSizeSource = &imageSizeSource.String
	}
	log.ImageSizeBreakdown = stringIntMapFromNullJSON(imageSizeBreakdown)
	if videoResolution.Valid {
		log.VideoResolution = &videoResolution.String
	}
	if videoDurationSeconds.Valid {
		value := int(videoDurationSeconds.Int64)
		log.VideoDurationSeconds = &value
	}
	if serviceTier.Valid {
		log.ServiceTier = &serviceTier.String
	}
	if reasoningEffort.Valid {
		log.ReasoningEffort = &reasoningEffort.String
	}
	if inboundEndpoint.Valid {
		log.InboundEndpoint = &inboundEndpoint.String
	}
	if upstreamEndpoint.Valid {
		log.UpstreamEndpoint = &upstreamEndpoint.String
	}
	if upstreamModel.Valid {
		log.UpstreamModel = &upstreamModel.String
	}
	if channelID.Valid {
		value := channelID.Int64
		log.ChannelID = &value
	}
	if modelMappingChain.Valid {
		log.ModelMappingChain = &modelMappingChain.String
	}
	if billingTier.Valid {
		log.BillingTier = &billingTier.String
	}
	if billingMode.Valid {
		log.BillingMode = &billingMode.String
	}
	if accountStatsCost.Valid {
		log.AccountStatsCost = &accountStatsCost.Float64
	}

	return log, nil
}

func nullInt64(v *int64) sql.NullInt64 {
	if v == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: *v, Valid: true}
}

func nullInt(v *int) sql.NullInt64 {
	if v == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: int64(*v), Valid: true}
}

func nullFloat64Ptr(v sql.NullFloat64) *float64 {
	if !v.Valid {
		return nil
	}
	out := v.Float64
	return &out
}

func nullString(v *string) sql.NullString {
	if v == nil || *v == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: *v, Valid: true}
}

func nullStringIntMapJSON(v map[string]int) any {
	if len(v) == 0 {
		return nil
	}
	payload, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return string(payload)
}

func stringIntMapFromNullJSON(v sql.NullString) map[string]int {
	if !v.Valid || strings.TrimSpace(v.String) == "" {
		return nil
	}
	var out map[string]int
	if err := json.Unmarshal([]byte(v.String), &out); err != nil {
		return nil
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func coalesceTrimmedString(v sql.NullString, fallback string) string {
	if v.Valid && strings.TrimSpace(v.String) != "" {
		return v.String
	}
	return fallback
}

func setToSlice(set map[int64]struct{}) []int64 {
	out := make([]int64, 0, len(set))
	for id := range set {
		out = append(out, id)
	}
	return out
}
