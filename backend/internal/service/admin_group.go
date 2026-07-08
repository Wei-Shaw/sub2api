package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/geminicli"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
)

// Group management implementations
func (s *adminServiceImpl) ListGroups(ctx context.Context, page, pageSize int, platform, status, search string, isExclusive *bool, sortBy, sortOrder string) ([]Group, int64, error) {
	params := pagination.PaginationParams{Page: page, PageSize: pageSize, SortBy: sortBy, SortOrder: sortOrderREDACTED
	groups, result, err := s.groupRepo.ListWithFilters(ctx, params, platform, status, search, isExclusive)
	if err != nil {
		return nil, 0, err
REDACTED
	return groups, result.Total, nil
REDACTED

func (s *adminServiceImpl) GetAllGroups(ctx context.Context) ([]Group, error) {
	return s.groupRepo.ListActive(ctx)
REDACTED

func (s *adminServiceImpl) GetAllGroupsByPlatform(ctx context.Context, platform string) ([]Group, error) {
	return s.groupRepo.ListActiveByPlatform(ctx, platform)
REDACTED

func (s *adminServiceImpl) GetAllGroupsIncludingInactive(ctx context.Context) ([]Group, error) {
	// ListWithFilters with empty status = no status filter, so active + disabled groups are returned.
	// PageSize 10000 is intentionally large; group count is O(dozens) in practice.
	groups, _, err := s.groupRepo.ListWithFilters(ctx, pagination.PaginationParams{Page: 1, PageSize: 10000REDACTED, "", "", "", nil)
	return groups, err
REDACTED

func (s *adminServiceImpl) GetGroup(ctx context.Context, id int64) (*Group, error) {
	return s.groupRepo.GetByID(ctx, id)
REDACTED

func (s *adminServiceImpl) GetGroupModelsListCandidates(ctx context.Context, id int64, platform string) ([]string, error) {
	platform = strings.TrimSpace(platform)
	if id > 0 {
		group, err := s.groupRepo.GetByIDLite(ctx, id)
		if err != nil {
			return nil, err
	REDACTED
		if platform == "" {
			platform = group.Platform
	REDACTED
REDACTED
	if platform == "" {
		platform = PlatformAnthropic
REDACTED

	candidates := defaultModelsListCandidateIDs(platform)
	if id <= 0 || s.accountRepo == nil {
		return candidates, nil
REDACTED

	accounts, err := s.accountRepo.ListSchedulableByGroupID(ctx, id)
	if err != nil {
		return nil, err
REDACTED

	seen := make(map[string]struct{REDACTED, len(candidates))
	for _, model := range candidates {
		seen[model] = struct{REDACTED{REDACTED
REDACTED
	for _, acc := range accounts {
		if acc.Platform != platform {
			continue
	REDACTED
		for model := range acc.GetModelMapping() {
			model = strings.TrimSpace(model)
			if model == "" {
				continue
		REDACTED
			if _, ok := seen[model]; ok {
				continue
		REDACTED
			seen[model] = struct{REDACTED{REDACTED
			candidates = append(candidates, model)
	REDACTED
REDACTED
	return candidates, nil
REDACTED

func defaultModelsListCandidateIDs(platform string) []string {
	switch platform {
	case PlatformOpenAI:
		return openai.DefaultModelIDs()
	case PlatformGemini:
		ids := make([]string, 0, len(geminicli.DefaultModels))
		for _, model := range geminicli.DefaultModels {
			ids = append(ids, model.ID)
	REDACTED
		return ids
	case PlatformAntigravity:
		models := antigravity.DefaultModels()
		ids := make([]string, 0, len(models))
		for _, model := range models {
			ids = append(ids, model.ID)
	REDACTED
		return ids
	case PlatformGrok:
		return xai.DefaultModelIDs()
	default:
		ids := make([]string, 0, len(claude.DefaultModels))
		for _, model := range claude.DefaultModels {
			ids = append(ids, model.ID)
	REDACTED
		return ids
REDACTED
REDACTED

func defaultAllowImageGenerationForPlatform(platform string) bool {
	// Grok image and video generation routes share the legacy image-generation gate.
	// Older clients send the false zero value, so Grok groups must default enabled.
	return platform == PlatformGrok
REDACTED

func (s *adminServiceImpl) CreateGroup(ctx context.Context, input *CreateGroupInput) (*Group, error) {
	if input.RateMultiplier <= 0 {
		return nil, errors.New("rate_multiplier must be > 0")
REDACTED

	platform := input.Platform
	if platform == "" {
		platform = PlatformAnthropic
REDACTED

	subscriptionType := input.SubscriptionType
	if subscriptionType == "" {
		subscriptionType = SubscriptionTypeStandard
REDACTED

	// 限额字段：nil/负数 表示"无限制"，0 表示"不允许用量"，正数表示具体限额
	dailyLimit := normalizeLimit(input.DailyLimitUSD)
	weeklyLimit := normalizeLimit(input.WeeklyLimitUSD)
	monthlyLimit := normalizeLimit(input.MonthlyLimitUSD)

	// 图片价格：负数表示清除（使用默认价格），0 保留（表示免费）
	imagePrice1K := normalizePrice(input.ImagePrice1K)
	imagePrice2K := normalizePrice(input.ImagePrice2K)
	imagePrice4K := normalizePrice(input.ImagePrice4K)
	imageRateMultiplier := 1.0
	if input.ImageRateMultiplier != nil {
		if *input.ImageRateMultiplier < 0 {
			return nil, errors.New("image_rate_multiplier must be >= 0")
	REDACTED
		imageRateMultiplier = *input.ImageRateMultiplier
REDACTED
	batchImageDiscountMultiplier := defaultBatchImageDiscountMultiplier
	if input.BatchImageDiscountMultiplier != nil {
		if *input.BatchImageDiscountMultiplier < 0 {
			return nil, errors.New("batch_image_discount_multiplier must be >= 0")
	REDACTED
		batchImageDiscountMultiplier = *input.BatchImageDiscountMultiplier
REDACTED
	batchImageHoldMultiplier := defaultBatchImageHoldMultiplier
	if input.BatchImageHoldMultiplier != nil {
		if *input.BatchImageHoldMultiplier < 0 {
			return nil, errors.New("batch_image_hold_multiplier must be >= 0")
	REDACTED
		batchImageHoldMultiplier = *input.BatchImageHoldMultiplier
REDACTED
	// 不变式：hold 比例 >= discount 比例。否则批量任务成功率足够高时
	// 实际成本会超过冻结额，结算永远失败、用户冻结余额无法解冻。
	if batchImageHoldMultiplier < batchImageDiscountMultiplier {
		return nil, errors.New("batch_image_hold_multiplier must be >= batch_image_discount_multiplier")
REDACTED

	peakRateMultiplier := 1.0
	if input.PeakRateMultiplier != nil {
		peakRateMultiplier = *input.PeakRateMultiplier
REDACTED
	// 先归一化（非订阅分组清空高峰配置、清洗停用状态下的脏字段）再校验，与 UpdateGroup 同一收口。
	peakRateEnabled, peakStart, peakEnd, peakRateMultiplier := NormalizePeakRateConfig(subscriptionType, input.PeakRateEnabled, input.PeakStart, input.PeakEnd, peakRateMultiplier)
	if err := ValidatePeakRateConfig(subscriptionType, peakRateEnabled, peakStart, peakEnd, peakRateMultiplier); err != nil {
		return nil, err
REDACTED

	// 校验降级分组
	if input.FallbackGroupID != nil {
		if err := s.validateFallbackGroup(ctx, 0, *input.FallbackGroupID); err != nil {
			return nil, err
	REDACTED
REDACTED
	fallbackOnInvalidRequest := input.FallbackGroupIDOnInvalidRequest
	if fallbackOnInvalidRequest != nil && *fallbackOnInvalidRequest <= 0 {
		fallbackOnInvalidRequest = nil
REDACTED
	// 校验无效请求兜底分组
	if fallbackOnInvalidRequest != nil {
		if err := s.validateFallbackGroupOnInvalidRequest(ctx, 0, platform, subscriptionType, *fallbackOnInvalidRequest); err != nil {
			return nil, err
	REDACTED
REDACTED

	// MCPXMLInject：默认为 true，仅当显式传入 false 时关闭
	mcpXMLInject := true
	if input.MCPXMLInject != nil {
		mcpXMLInject = *input.MCPXMLInject
REDACTED

	allowImageGeneration := input.AllowImageGeneration || defaultAllowImageGenerationForPlatform(platform)
	allowBatchImageGeneration := input.AllowBatchImageGeneration && allowImageGeneration && platform == PlatformGemini

	// 如果指定了复制账号的源分组，先获取账号 ID 列表
	var accountIDsToCopy []int64
	if len(input.CopyAccountsFromGroupIDs) > 0 {
		// 去重源分组 IDs
		seen := make(map[int64]struct{REDACTED)
		uniqueSourceGroupIDs := make([]int64, 0, len(input.CopyAccountsFromGroupIDs))
		for _, srcGroupID := range input.CopyAccountsFromGroupIDs {
			if _, exists := seen[srcGroupID]; !exists {
				seen[srcGroupID] = struct{REDACTED{REDACTED
				uniqueSourceGroupIDs = append(uniqueSourceGroupIDs, srcGroupID)
		REDACTED
	REDACTED

		// 校验源分组的平台是否与新分组一致
		for _, srcGroupID := range uniqueSourceGroupIDs {
			srcGroup, err := s.groupRepo.GetByIDLite(ctx, srcGroupID)
			if err != nil {
				return nil, fmt.Errorf("source group %d not found: %w", srcGroupID, err)
		REDACTED
			if srcGroup.Platform != platform {
				return nil, fmt.Errorf("source group %d platform mismatch: expected %s, got %s", srcGroupID, platform, srcGroup.Platform)
		REDACTED
	REDACTED

		// 获取所有源分组的账号（去重）
		var err error
		accountIDsToCopy, err = s.groupRepo.GetAccountIDsByGroupIDs(ctx, uniqueSourceGroupIDs)
		if err != nil {
			return nil, fmt.Errorf("failed to get accounts from source groups: %w", err)
	REDACTED
REDACTED

	group := &Group{
		Name:                            input.Name,
		Description:                     input.Description,
		Platform:                        platform,
		RateMultiplier:                  input.RateMultiplier,
		IsExclusive:                     input.IsExclusive,
		Status:                          StatusActive,
		SubscriptionType:                subscriptionType,
		DailyLimitUSD:                   dailyLimit,
		WeeklyLimitUSD:                  weeklyLimit,
		MonthlyLimitUSD:                 monthlyLimit,
		AllowImageGeneration:            allowImageGeneration,
		AllowBatchImageGeneration:       allowBatchImageGeneration,
		ImageRateIndependent:            input.ImageRateIndependent,
		ImageRateMultiplier:             imageRateMultiplier,
		BatchImageDiscountMultiplier:    batchImageDiscountMultiplier,
		BatchImageHoldMultiplier:        batchImageHoldMultiplier,
		PeakRateEnabled:                 peakRateEnabled,
		PeakStart:                       peakStart,
		PeakEnd:                         peakEnd,
		PeakRateMultiplier:              peakRateMultiplier,
		ImagePrice1K:                    imagePrice1K,
		ImagePrice2K:                    imagePrice2K,
		ImagePrice4K:                    imagePrice4K,
		ClaudeCodeOnly:                  input.ClaudeCodeOnly,
		FallbackGroupID:                 input.FallbackGroupID,
		FallbackGroupIDOnInvalidRequest: fallbackOnInvalidRequest,
		ModelRouting:                    input.ModelRouting,
		MCPXMLInject:                    mcpXMLInject,
		SupportedModelScopes:            input.SupportedModelScopes,
		AllowMessagesDispatch:           input.AllowMessagesDispatch,
		RequireOAuthOnly:                input.RequireOAuthOnly,
		RequirePrivacySet:               input.RequirePrivacySet,
		DefaultMappedModel:              input.DefaultMappedModel,
		MessagesDispatchModelConfig:     normalizeOpenAIMessagesDispatchModelConfig(input.MessagesDispatchModelConfig),
		ModelsListConfig:                normalizeGroupModelsListConfig(input.ModelsListConfig),
		RPMLimit:                        input.RPMLimit,
REDACTED
	sanitizeGroupMessagesDispatchFields(group)
	if err := s.groupRepo.Create(ctx, group); err != nil {
		return nil, err
REDACTED

	// require_oauth_only: 过滤掉 apikey 类型账号
	if group.RequireOAuthOnly && (group.Platform == PlatformOpenAI || group.Platform == PlatformAntigravity || group.Platform == PlatformAnthropic || group.Platform == PlatformGemini || group.Platform == PlatformGrok) && len(accountIDsToCopy) > 0 {
		accounts, err := s.accountRepo.GetByIDs(ctx, accountIDsToCopy)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch accounts for oauth filter: %w", err)
	REDACTED
		oauthIDs := make(map[int64]struct{REDACTED, len(accounts))
		for _, acc := range accounts {
			if acc.Type != AccountTypeAPIKey {
				oauthIDs[acc.ID] = struct{REDACTED{REDACTED
		REDACTED
	REDACTED
		var filtered []int64
		for _, aid := range accountIDsToCopy {
			if _, ok := oauthIDs[aid]; ok {
				filtered = append(filtered, aid)
		REDACTED
	REDACTED
		accountIDsToCopy = filtered
REDACTED

	// 如果有需要复制的账号，绑定到新分组
	if len(accountIDsToCopy) > 0 {
		if err := s.groupRepo.BindAccountsToGroup(ctx, group.ID, accountIDsToCopy); err != nil {
			return nil, fmt.Errorf("failed to bind accounts to new group: %w", err)
	REDACTED
		group.AccountCount = int64(len(accountIDsToCopy))
REDACTED

	return group, nil
REDACTED

// normalizeLimit 将负数转换为 nil（表示无限制），0 保留（表示限额为零）
func normalizeLimit(limit *float64) *float64 {
	if limit == nil || *limit < 0 {
		return nil
REDACTED
	return limit
REDACTED

// normalizePrice 将负数转换为 nil（表示使用默认价格），0 保留（表示免费）
func normalizePrice(price *float64) *float64 {
	if price == nil || *price < 0 {
		return nil
REDACTED
	return price
REDACTED

// validateFallbackGroup 校验降级分组的有效性
// currentGroupID: 当前分组 ID（新建时为 0）
// fallbackGroupID: 降级分组 ID
func (s *adminServiceImpl) validateFallbackGroup(ctx context.Context, currentGroupID, fallbackGroupID int64) error {
	// 不能将自己设置为降级分组
	if currentGroupID > 0 && currentGroupID == fallbackGroupID {
		return fmt.Errorf("cannot set self as fallback group")
REDACTED

	visited := map[int64]struct{REDACTED{REDACTED
	nextID := fallbackGroupID
	for {
		if _, seen := visited[nextID]; seen {
			return fmt.Errorf("fallback group cycle detected")
	REDACTED
		visited[nextID] = struct{REDACTED{REDACTED
		if currentGroupID > 0 && nextID == currentGroupID {
			return fmt.Errorf("fallback group cycle detected")
	REDACTED

		// 检查降级分组是否存在
		fallbackGroup, err := s.groupRepo.GetByIDLite(ctx, nextID)
		if err != nil {
			return fmt.Errorf("fallback group not found: %w", err)
	REDACTED

		// 降级分组不能启用 claude_code_only，否则会造成死循环
		if nextID == fallbackGroupID && fallbackGroup.ClaudeCodeOnly {
			return fmt.Errorf("fallback group cannot have claude_code_only enabled")
	REDACTED

		if fallbackGroup.FallbackGroupID == nil {
			return nil
	REDACTED
		nextID = *fallbackGroup.FallbackGroupID
REDACTED
REDACTED

// validateFallbackGroupOnInvalidRequest 校验无效请求兜底分组的有效性
// currentGroupID: 当前分组 ID（新建时为 0）
// platform/subscriptionType: 当前分组的有效平台/订阅类型
// fallbackGroupID: 兜底分组 ID
func (s *adminServiceImpl) validateFallbackGroupOnInvalidRequest(ctx context.Context, currentGroupID int64, platform, subscriptionType string, fallbackGroupID int64) error {
	if platform != PlatformAnthropic && platform != PlatformAntigravity {
		return fmt.Errorf("invalid request fallback only supported for anthropic or antigravity groups")
REDACTED
	if subscriptionType == SubscriptionTypeSubscription {
		return fmt.Errorf("subscription groups cannot set invalid request fallback")
REDACTED
	if currentGroupID > 0 && currentGroupID == fallbackGroupID {
		return fmt.Errorf("cannot set self as invalid request fallback group")
REDACTED

	fallbackGroup, err := s.groupRepo.GetByIDLite(ctx, fallbackGroupID)
	if err != nil {
		return fmt.Errorf("fallback group not found: %w", err)
REDACTED
	if fallbackGroup.Platform != PlatformAnthropic {
		return fmt.Errorf("fallback group must be anthropic platform")
REDACTED
	if fallbackGroup.SubscriptionType == SubscriptionTypeSubscription {
		return fmt.Errorf("fallback group cannot be subscription type")
REDACTED
	if fallbackGroup.FallbackGroupIDOnInvalidRequest != nil {
		return fmt.Errorf("fallback group cannot have invalid request fallback configured")
REDACTED
	return nil
REDACTED

func (s *adminServiceImpl) UpdateGroup(ctx context.Context, id int64, input *UpdateGroupInput) (*Group, error) {
	group, err := s.groupRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
REDACTED

	if input.Name != "" {
		group.Name = input.Name
REDACTED
	if input.Description != nil {
		group.Description = *input.Description
REDACTED
	if input.Platform != "" {
		group.Platform = input.Platform
REDACTED
	if input.RateMultiplier != nil {
		if *input.RateMultiplier <= 0 {
			return nil, errors.New("rate_multiplier must be > 0")
	REDACTED
		group.RateMultiplier = *input.RateMultiplier
REDACTED
	if input.IsExclusive != nil {
		group.IsExclusive = *input.IsExclusive
REDACTED
	if input.Status != "" {
		group.Status = input.Status
REDACTED

	// 订阅相关字段
	if input.SubscriptionType != "" {
		group.SubscriptionType = input.SubscriptionType
REDACTED
	// 限额字段：nil/负数 表示"无限制"，0 表示"不允许用量"，正数表示具体限额
	// 前端始终发送这三个字段，无需 nil 守卫
	group.DailyLimitUSD = normalizeLimit(input.DailyLimitUSD)
	group.WeeklyLimitUSD = normalizeLimit(input.WeeklyLimitUSD)
	group.MonthlyLimitUSD = normalizeLimit(input.MonthlyLimitUSD)
	// 图片生成计费配置：负数表示清除（使用默认价格）
	if input.AllowImageGeneration != nil {
		group.AllowImageGeneration = *input.AllowImageGeneration
REDACTED
	if input.AllowBatchImageGeneration != nil {
		group.AllowBatchImageGeneration = *input.AllowBatchImageGeneration
REDACTED
	if !group.AllowImageGeneration || group.Platform != PlatformGemini {
		group.AllowBatchImageGeneration = false
REDACTED
	if input.ImageRateIndependent != nil {
		group.ImageRateIndependent = *input.ImageRateIndependent
REDACTED
	if input.ImageRateMultiplier != nil {
		if *input.ImageRateMultiplier < 0 {
			return nil, errors.New("image_rate_multiplier must be >= 0")
	REDACTED
		group.ImageRateMultiplier = *input.ImageRateMultiplier
REDACTED
	if input.BatchImageDiscountMultiplier != nil {
		if *input.BatchImageDiscountMultiplier < 0 {
			return nil, errors.New("batch_image_discount_multiplier must be >= 0")
	REDACTED
		group.BatchImageDiscountMultiplier = *input.BatchImageDiscountMultiplier
REDACTED
	if input.BatchImageHoldMultiplier != nil {
		if *input.BatchImageHoldMultiplier < 0 {
			return nil, errors.New("batch_image_hold_multiplier must be >= 0")
	REDACTED
		group.BatchImageHoldMultiplier = *input.BatchImageHoldMultiplier
REDACTED
	// 仅在本次更新显式触碰任一比例时校验合并后的不变式（hold >= discount），
	// 避免存量脏数据阻塞其他字段的正常更新（提交侧另有钳制兜底）。
	if (input.BatchImageDiscountMultiplier != nil || input.BatchImageHoldMultiplier != nil) &&
		group.BatchImageHoldMultiplier < group.BatchImageDiscountMultiplier {
		return nil, errors.New("batch_image_hold_multiplier must be >= batch_image_discount_multiplier")
REDACTED
	if input.PeakRateEnabled != nil {
		group.PeakRateEnabled = *input.PeakRateEnabled
REDACTED
	if input.PeakStart != nil {
		group.PeakStart = *input.PeakStart
REDACTED
	if input.PeakEnd != nil {
		group.PeakEnd = *input.PeakEnd
REDACTED
	if input.PeakRateMultiplier != nil {
		group.PeakRateMultiplier = *input.PeakRateMultiplier
REDACTED
	// 先归一化（非订阅分组——含本次更新转为非订阅——静默清空高峰配置，清洗停用状态下的脏字段），
	// 再收敛校验：Update 可能只传部分 peak 字段，需对合并后的最终配置统一校验，
	// 防止单独修改 start/end 导致最终 start>=end 等非法配置入库。与 CreateGroup 同一收口。
	group.PeakRateEnabled, group.PeakStart, group.PeakEnd, group.PeakRateMultiplier = NormalizePeakRateConfig(group.SubscriptionType, group.PeakRateEnabled, group.PeakStart, group.PeakEnd, group.PeakRateMultiplier)
	if err := ValidatePeakRateConfig(group.SubscriptionType, group.PeakRateEnabled, group.PeakStart, group.PeakEnd, group.PeakRateMultiplier); err != nil {
		return nil, err
REDACTED
	if input.ImagePrice1K != nil {
		group.ImagePrice1K = normalizePrice(input.ImagePrice1K)
REDACTED
	if input.ImagePrice2K != nil {
		group.ImagePrice2K = normalizePrice(input.ImagePrice2K)
REDACTED
	if input.ImagePrice4K != nil {
		group.ImagePrice4K = normalizePrice(input.ImagePrice4K)
REDACTED

	// Claude Code 客户端限制
	if input.ClaudeCodeOnly != nil {
		group.ClaudeCodeOnly = *input.ClaudeCodeOnly
REDACTED
	if input.FallbackGroupID != nil {
		// 校验降级分组
		if *input.FallbackGroupID > 0 {
			if err := s.validateFallbackGroup(ctx, id, *input.FallbackGroupID); err != nil {
				return nil, err
		REDACTED
			group.FallbackGroupID = input.FallbackGroupID
	REDACTED else {
			// 传入 0 或负数表示清除降级分组
			group.FallbackGroupID = nil
	REDACTED
REDACTED
	fallbackOnInvalidRequest := group.FallbackGroupIDOnInvalidRequest
	if input.FallbackGroupIDOnInvalidRequest != nil {
		if *input.FallbackGroupIDOnInvalidRequest > 0 {
			fallbackOnInvalidRequest = input.FallbackGroupIDOnInvalidRequest
	REDACTED else {
			fallbackOnInvalidRequest = nil
	REDACTED
REDACTED
	if fallbackOnInvalidRequest != nil {
		if err := s.validateFallbackGroupOnInvalidRequest(ctx, id, group.Platform, group.SubscriptionType, *fallbackOnInvalidRequest); err != nil {
			return nil, err
	REDACTED
REDACTED
	group.FallbackGroupIDOnInvalidRequest = fallbackOnInvalidRequest

	// 模型路由配置
	if input.ModelRouting != nil {
		group.ModelRouting = input.ModelRouting
REDACTED
	if input.ModelRoutingEnabled != nil {
		group.ModelRoutingEnabled = *input.ModelRoutingEnabled
REDACTED
	if input.MCPXMLInject != nil {
		group.MCPXMLInject = *input.MCPXMLInject
REDACTED

	// 支持的模型系列（仅 antigravity 平台使用）
	if input.SupportedModelScopes != nil {
		group.SupportedModelScopes = *input.SupportedModelScopes
REDACTED

	// OpenAI Messages 调度配置
	if input.AllowMessagesDispatch != nil {
		group.AllowMessagesDispatch = *input.AllowMessagesDispatch
REDACTED
	if input.RequireOAuthOnly != nil {
		group.RequireOAuthOnly = *input.RequireOAuthOnly
REDACTED
	if input.RequirePrivacySet != nil {
		group.RequirePrivacySet = *input.RequirePrivacySet
REDACTED
	if input.DefaultMappedModel != nil {
		group.DefaultMappedModel = *input.DefaultMappedModel
REDACTED
	if input.MessagesDispatchModelConfig != nil {
		group.MessagesDispatchModelConfig = normalizeOpenAIMessagesDispatchModelConfig(*input.MessagesDispatchModelConfig)
REDACTED
	if input.ModelsListConfig != nil {
		group.ModelsListConfig = normalizeGroupModelsListConfig(*input.ModelsListConfig)
REDACTED
	if input.RPMLimit != nil {
		group.RPMLimit = *input.RPMLimit
REDACTED
	sanitizeGroupMessagesDispatchFields(group)

	if err := s.groupRepo.Update(ctx, group); err != nil {
		return nil, err
REDACTED

	if s.authCacheInvalidator != nil {
		s.authCacheInvalidator.InvalidateAuthCacheByGroupID(ctx, id)
REDACTED

	// 如果指定了复制账号的源分组，同步绑定（替换当前分组的账号）
	if len(input.CopyAccountsFromGroupIDs) > 0 {
		// 去重源分组 IDs
		seen := make(map[int64]struct{REDACTED)
		uniqueSourceGroupIDs := make([]int64, 0, len(input.CopyAccountsFromGroupIDs))
		for _, srcGroupID := range input.CopyAccountsFromGroupIDs {
			// 校验：源分组不能是自身
			if srcGroupID == id {
				return nil, fmt.Errorf("cannot copy accounts from self")
		REDACTED
			// 去重
			if _, exists := seen[srcGroupID]; !exists {
				seen[srcGroupID] = struct{REDACTED{REDACTED
				uniqueSourceGroupIDs = append(uniqueSourceGroupIDs, srcGroupID)
		REDACTED
	REDACTED

		// 校验源分组的平台是否与当前分组一致
		for _, srcGroupID := range uniqueSourceGroupIDs {
			srcGroup, err := s.groupRepo.GetByIDLite(ctx, srcGroupID)
			if err != nil {
				return nil, fmt.Errorf("source group %d not found: %w", srcGroupID, err)
		REDACTED
			if srcGroup.Platform != group.Platform {
				return nil, fmt.Errorf("source group %d platform mismatch: expected %s, got %s", srcGroupID, group.Platform, srcGroup.Platform)
		REDACTED
	REDACTED

		// 获取所有源分组的账号（去重）
		accountIDsToCopy, err := s.groupRepo.GetAccountIDsByGroupIDs(ctx, uniqueSourceGroupIDs)
		if err != nil {
			return nil, fmt.Errorf("failed to get accounts from source groups: %w", err)
	REDACTED

		// 先清空当前分组的所有账号绑定
		if _, err := s.groupRepo.DeleteAccountGroupsByGroupID(ctx, id); err != nil {
			return nil, fmt.Errorf("failed to clear existing account bindings: %w", err)
	REDACTED

		// require_oauth_only: 过滤掉 apikey 类型账号
		if group.RequireOAuthOnly && (group.Platform == PlatformOpenAI || group.Platform == PlatformAntigravity || group.Platform == PlatformAnthropic || group.Platform == PlatformGemini || group.Platform == PlatformGrok) && len(accountIDsToCopy) > 0 {
			accounts, err := s.accountRepo.GetByIDs(ctx, accountIDsToCopy)
			if err != nil {
				return nil, fmt.Errorf("failed to fetch accounts for oauth filter: %w", err)
		REDACTED
			oauthIDs := make(map[int64]struct{REDACTED, len(accounts))
			for _, acc := range accounts {
				if acc.Type != AccountTypeAPIKey {
					oauthIDs[acc.ID] = struct{REDACTED{REDACTED
			REDACTED
		REDACTED
			var filtered []int64
			for _, aid := range accountIDsToCopy {
				if _, ok := oauthIDs[aid]; ok {
					filtered = append(filtered, aid)
			REDACTED
		REDACTED
			accountIDsToCopy = filtered
	REDACTED

		// 再绑定源分组的账号
		if len(accountIDsToCopy) > 0 {
			if err := s.groupRepo.BindAccountsToGroup(ctx, id, accountIDsToCopy); err != nil {
				return nil, fmt.Errorf("failed to bind accounts to group: %w", err)
		REDACTED
	REDACTED
REDACTED

	return group, nil
REDACTED

func (s *adminServiceImpl) DeleteGroup(ctx context.Context, id int64) error {
	var groupKeys []string
	if s.authCacheInvalidator != nil {
		keys, err := s.apiKeyRepo.ListKeysByGroupID(ctx, id)
		if err == nil {
			groupKeys = keys
	REDACTED
REDACTED

	affectedUserIDs, err := s.groupRepo.DeleteCascade(ctx, id)
	if err != nil {
		return err
REDACTED
	// 注意：user_group_rate_multipliers 表通过外键 ON DELETE CASCADE 自动清理

	// 事务成功后，异步失效受影响用户的订阅缓存
	if len(affectedUserIDs) > 0 && s.billingCacheService != nil {
		groupID := id
		go func() {
			cacheCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			for _, userID := range affectedUserIDs {
				if err := s.billingCacheService.InvalidateSubscription(cacheCtx, userID, groupID); err != nil {
					logger.LegacyPrintf("service.admin", "invalidate subscription cache failed: user_id=%d group_id=%d err=%v", userID, groupID, err)
			REDACTED
		REDACTED
	REDACTED()
REDACTED
	if s.authCacheInvalidator != nil {
		for _, key := range groupKeys {
			s.authCacheInvalidator.InvalidateAuthCacheByKey(ctx, key)
	REDACTED
REDACTED

	return nil
REDACTED

func (s *adminServiceImpl) GetGroupAPIKeys(ctx context.Context, groupID int64, page, pageSize int) ([]APIKey, int64, error) {
	params := pagination.PaginationParams{Page: page, PageSize: pageSizeREDACTED
	keys, result, err := s.apiKeyRepo.ListByGroupID(ctx, groupID, params)
	if err != nil {
		return nil, 0, err
REDACTED
	return keys, result.Total, nil
REDACTED

func (s *adminServiceImpl) GetGroupRateMultipliers(ctx context.Context, groupID int64) ([]UserGroupRateEntry, error) {
	if s.userGroupRateRepo == nil {
		return nil, nil
REDACTED
	return s.userGroupRateRepo.GetByGroupID(ctx, groupID)
REDACTED

func (s *adminServiceImpl) ClearGroupRateMultipliers(ctx context.Context, groupID int64) error {
	if s.userGroupRateRepo == nil {
		return nil
REDACTED
	return s.userGroupRateRepo.DeleteByGroupID(ctx, groupID)
REDACTED

func (s *adminServiceImpl) BatchSetGroupRateMultipliers(ctx context.Context, groupID int64, entries []GroupRateMultiplierInput) error {
	if s.userGroupRateRepo == nil {
		return nil
REDACTED
	for _, e := range entries {
		if e.RateMultiplier <= 0 {
			return fmt.Errorf("rate_multiplier must be > 0 (user_id=%d)", e.UserID)
	REDACTED
REDACTED
	return s.userGroupRateRepo.SyncGroupRateMultipliers(ctx, groupID, entries)
REDACTED

func (s *adminServiceImpl) ClearGroupRPMOverrides(ctx context.Context, groupID int64) error {
	if s.userGroupRateRepo == nil {
		return nil
REDACTED
	if err := s.userGroupRateRepo.ClearGroupRPMOverrides(ctx, groupID); err != nil {
		return err
REDACTED
	// RPM override 已嵌入 auth cache snapshot (v7)，变更后必须失效相关缓存。
	if s.authCacheInvalidator != nil {
		s.authCacheInvalidator.InvalidateAuthCacheByGroupID(ctx, groupID)
REDACTED
	return nil
REDACTED

func (s *adminServiceImpl) BatchSetGroupRPMOverrides(ctx context.Context, groupID int64, entries []GroupRPMOverrideInput) error {
	if s.userGroupRateRepo == nil {
		return nil
REDACTED
	for _, e := range entries {
		if e.RPMOverride != nil && *e.RPMOverride < 0 {
			return infraerrors.BadRequest("INVALID_RPM_OVERRIDE", fmt.Sprintf("rpm_override must be >= 0 (user_id=%d)", e.UserID))
	REDACTED
REDACTED
	if err := s.userGroupRateRepo.SyncGroupRPMOverrides(ctx, groupID, entries); err != nil {
		return err
REDACTED
	// RPM override 已嵌入 auth cache snapshot (v7)，变更后必须失效相关缓存。
	if s.authCacheInvalidator != nil {
		s.authCacheInvalidator.InvalidateAuthCacheByGroupID(ctx, groupID)
REDACTED
	return nil
REDACTED

func (s *adminServiceImpl) UpdateGroupSortOrders(ctx context.Context, updates []GroupSortOrderUpdate) error {
	return s.groupRepo.UpdateSortOrders(ctx, updates)
REDACTED

// AdminUpdateAPIKeyGroupID 管理员修改 API Key 分组绑定
// groupID: nil=不修改, 指向0=解绑, 指向正整数=绑定到目标分组
func (s *adminServiceImpl) AdminUpdateAPIKeyGroupID(ctx context.Context, keyID int64, groupID *int64) (*AdminUpdateAPIKeyGroupIDResult, error) {
	apiKey, err := s.apiKeyRepo.GetByID(ctx, keyID)
	if err != nil {
		return nil, err
REDACTED

	if groupID == nil {
		// nil 表示不修改，直接返回
		return &AdminUpdateAPIKeyGroupIDResult{APIKey: apiKeyREDACTED, nil
REDACTED

	if *groupID < 0 {
		return nil, infraerrors.BadRequest("INVALID_GROUP_ID", "group_id must be non-negative")
REDACTED

	result := &AdminUpdateAPIKeyGroupIDResult{REDACTED

	if *groupID == 0 {
		// 0 表示解绑分组（不修改 user_allowed_groups，避免影响用户其他 Key）
		apiKey.GroupID = nil
		apiKey.Group = nil
REDACTED else {
		// 验证目标分组存在且状态为 active
		group, err := s.groupRepo.GetByID(ctx, *groupID)
		if err != nil {
			return nil, err
	REDACTED
		if group.Status != StatusActive {
			return nil, infraerrors.BadRequest("GROUP_NOT_ACTIVE", "target group is not active")
	REDACTED
		// 订阅类型分组：用户须持有该分组的有效订阅才可绑定
		if group.IsSubscriptionType() {
			if s.userSubRepo == nil {
				return nil, infraerrors.InternalServer("SUBSCRIPTION_REPOSITORY_UNAVAILABLE", "subscription repository is not configured")
		REDACTED
			if _, err := s.userSubRepo.GetActiveByUserIDAndGroupID(ctx, apiKey.UserID, *groupID); err != nil {
				if errors.Is(err, ErrSubscriptionNotFound) {
					return nil, infraerrors.BadRequest("SUBSCRIPTION_REQUIRED", "user does not have an active subscription for this group")
			REDACTED
				return nil, err
		REDACTED
	REDACTED

		gid := *groupID
		apiKey.GroupID = &gid
		apiKey.Group = group

		// 专属标准分组：使用事务保证「添加分组权限」与「更新 API Key」的原子性
		if group.IsExclusive && !group.IsSubscriptionType() {
			opCtx := ctx
			var tx *dbent.Tx
			if s.entClient == nil {
				logger.LegacyPrintf("service.admin", "Warning: entClient is nil, skipping transaction protection for exclusive group binding")
		REDACTED else {
				var txErr error
				tx, txErr = s.entClient.Tx(ctx)
				if txErr != nil {
					return nil, fmt.Errorf("begin transaction: %w", txErr)
			REDACTED
				defer func() { _ = tx.Rollback() REDACTED()
				opCtx = dbent.NewTxContext(ctx, tx)
		REDACTED

			if addErr := s.userRepo.AddGroupToAllowedGroups(opCtx, apiKey.UserID, gid); addErr != nil {
				return nil, fmt.Errorf("add group to user allowed groups: %w", addErr)
		REDACTED
			if err := s.apiKeyRepo.Update(opCtx, apiKey); err != nil {
				return nil, fmt.Errorf("update api key: %w", err)
		REDACTED
			if tx != nil {
				if err := tx.Commit(); err != nil {
					return nil, fmt.Errorf("commit transaction: %w", err)
			REDACTED
		REDACTED

			result.AutoGrantedGroupAccess = true
			result.GrantedGroupID = &gid
			result.GrantedGroupName = group.Name

			// 失效认证缓存（在事务提交后执行）
			if s.authCacheInvalidator != nil {
				s.authCacheInvalidator.InvalidateAuthCacheByKey(ctx, apiKey.Key)
		REDACTED

			result.APIKey = apiKey
			return result, nil
	REDACTED
REDACTED

	// 非专属分组 / 解绑：无需事务，单步更新即可
	if err := s.apiKeyRepo.Update(ctx, apiKey); err != nil {
		return nil, fmt.Errorf("update api key: %w", err)
REDACTED

	// 失效认证缓存
	if s.authCacheInvalidator != nil {
		s.authCacheInvalidator.InvalidateAuthCacheByKey(ctx, apiKey.Key)
REDACTED

	result.APIKey = apiKey
	return result, nil
REDACTED

// AdminResetAPIKeyRateLimitUsage resets all API key rate-limit usage windows.
func (s *adminServiceImpl) AdminResetAPIKeyRateLimitUsage(ctx context.Context, keyID int64) (*APIKey, error) {
	apiKey, err := s.apiKeyRepo.GetByID(ctx, keyID)
	if err != nil {
		return nil, err
REDACTED
	apiKey.Usage5h = 0
	apiKey.Usage1d = 0
	apiKey.Usage7d = 0
	apiKey.Window5hStart = nil
	apiKey.Window1dStart = nil
	apiKey.Window7dStart = nil
	if err := s.apiKeyRepo.Update(ctx, apiKey); err != nil {
		return nil, fmt.Errorf("reset api key rate limit usage: %w", err)
REDACTED
	if s.authCacheInvalidator != nil {
		s.authCacheInvalidator.InvalidateAuthCacheByKey(ctx, apiKey.Key)
REDACTED
	if s.billingCacheService != nil {
		_ = s.billingCacheService.InvalidateAPIKeyRateLimit(ctx, apiKey.ID)
REDACTED
	return apiKey, nil
REDACTED

// ReplaceUserGroup 替换用户的专属分组
func (s *adminServiceImpl) ReplaceUserGroup(ctx context.Context, userID, oldGroupID, newGroupID int64) (*ReplaceUserGroupResult, error) {
	if oldGroupID == newGroupID {
		return nil, infraerrors.BadRequest("SAME_GROUP", "old and new group must be different")
REDACTED

	// 验证新分组存在且为活跃的专属标准分组
	newGroup, err := s.groupRepo.GetByID(ctx, newGroupID)
	if err != nil {
		return nil, err
REDACTED
	if newGroup.Status != StatusActive {
		return nil, infraerrors.BadRequest("GROUP_NOT_ACTIVE", "target group is not active")
REDACTED
	if !newGroup.IsExclusive {
		return nil, infraerrors.BadRequest("GROUP_NOT_EXCLUSIVE", "target group is not exclusive")
REDACTED
	if newGroup.IsSubscriptionType() {
		return nil, infraerrors.BadRequest("GROUP_IS_SUBSCRIPTION", "subscription groups are not supported for replacement")
REDACTED

	// 事务保证原子性
	if s.entClient == nil {
		return nil, fmt.Errorf("entClient is nil, cannot perform group replacement")
REDACTED
	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
REDACTED
	defer func() { _ = tx.Rollback() REDACTED()
	opCtx := dbent.NewTxContext(ctx, tx)

	// 1. 授予新分组权限
	if err := s.userRepo.AddGroupToAllowedGroups(opCtx, userID, newGroupID); err != nil {
		return nil, fmt.Errorf("add new group to allowed groups: %w", err)
REDACTED

	// 2. 迁移绑定旧分组的 Key 到新分组
	migrated, err := s.apiKeyRepo.UpdateGroupIDByUserAndGroup(opCtx, userID, oldGroupID, newGroupID)
	if err != nil {
		return nil, fmt.Errorf("migrate api keys: %w", err)
REDACTED

	// 3. 移除旧分组权限
	if err := s.userRepo.RemoveGroupFromUserAllowedGroups(opCtx, userID, oldGroupID); err != nil {
		return nil, fmt.Errorf("remove old group from allowed groups: %w", err)
REDACTED

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit transaction: %w", err)
REDACTED

	// 失效该用户所有 Key 的认证缓存
	if s.authCacheInvalidator != nil {
		keys, keyErr := s.apiKeyRepo.ListKeysByUserID(ctx, userID)
		if keyErr == nil {
			for _, k := range keys {
				s.authCacheInvalidator.InvalidateAuthCacheByKey(ctx, k)
		REDACTED
	REDACTED
REDACTED

	return &ReplaceUserGroupResult{MigratedKeys: migratedREDACTED, nil
REDACTED
