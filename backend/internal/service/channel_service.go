package service

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync/atomic"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	"golang.org/x/sync/singleflight"
)

var (
	ErrChannelNotFound       = infraerrors.NotFound("CHANNEL_NOT_FOUND", "channel not found")
	ErrChannelExists         = infraerrors.Conflict("CHANNEL_EXISTS", "channel name already exists")
	ErrGroupAlreadyInChannel = infraerrors.Conflict(
		"GROUP_ALREADY_IN_CHANNEL",
		"one or more groups already belong to another channel",
	)
)

// ChannelRepository 渠道数据访问接口
type ChannelRepository interface {
	Create(ctx context.Context, channel *Channel) error
	GetByID(ctx context.Context, id int64) (*Channel, error)
	Update(ctx context.Context, channel *Channel) error
	Delete(ctx context.Context, id int64) error
	List(ctx context.Context, params pagination.PaginationParams, status, search string) ([]Channel, *pagination.PaginationResult, error)
	ListAll(ctx context.Context) ([]Channel, error)
	ExistsByName(ctx context.Context, name string) (bool, error)
	ExistsByNameExcluding(ctx context.Context, name string, excludeID int64) (bool, error)

	// 分组关联
	GetGroupIDs(ctx context.Context, channelID int64) ([]int64, error)
	SetGroupIDs(ctx context.Context, channelID int64, groupIDs []int64) error
	GetChannelIDByGroupID(ctx context.Context, groupID int64) (int64, error)
	GetGroupsInOtherChannels(ctx context.Context, channelID int64, groupIDs []int64) ([]int64, error)

	// 分组平台查询
	GetGroupPlatforms(ctx context.Context, groupIDs []int64) (map[int64]string, error)

	// 模型定价
	ListModelPricing(ctx context.Context, channelID int64) ([]ChannelModelPricing, error)
	CreateModelPricing(ctx context.Context, pricing *ChannelModelPricing) error
	UpdateModelPricing(ctx context.Context, pricing *ChannelModelPricing) error
	DeleteModelPricing(ctx context.Context, id int64) error
	ReplaceModelPricing(ctx context.Context, channelID int64, pricingList []ChannelModelPricing) error
REDACTED

// channelModelKey 渠道缓存复合键（显式包含 platform 防止跨平台同名模型冲突）
type channelModelKey struct {
	groupID  int64
	platform string // 平台标识
	model    string // lowercase
REDACTED

// channelGroupPlatformKey 通配符定价缓存键
type channelGroupPlatformKey struct {
	groupID  int64
	platform string
REDACTED

// wildcardPricingEntry 通配符定价条目
type wildcardPricingEntry struct {
	prefix  string
	pricing *ChannelModelPricing
REDACTED

// wildcardMappingEntry 通配符映射条目
type wildcardMappingEntry struct {
	prefix string
	target string
REDACTED

// channelCache 渠道缓存快照（扁平化哈希结构，热路径 O(1) 查找）
type channelCache struct {
	// 热路径查找
	pricingByGroupModel     map[channelModelKey]*ChannelModelPricing            // (groupID, platform, model) → 定价
	wildcardByGroupPlatform map[channelGroupPlatformKey][]*wildcardPricingEntry // (groupID, platform) → 通配符定价（按配置顺序，先匹配先使用）
	mappingByGroupModel     map[channelModelKey]string                          // (groupID, platform, model) → 映射目标
	wildcardMappingByGP     map[channelGroupPlatformKey][]*wildcardMappingEntry // (groupID, platform) → 通配符映射（按配置顺序，先匹配先使用）
	channelByGroupID        map[int64]*Channel                                  // groupID → 渠道
	groupPlatform           map[int64]string                                    // groupID → platform

	// 冷路径（CRUD 操作）
	byID     map[int64]*Channel
	loadedAt time.Time
REDACTED

// ChannelMappingResult 渠道映射查找结果
type ChannelMappingResult struct {
	MappedModel        string // 映射后的模型名（无映射时等于原始模型名）
	ChannelID          int64  // 渠道 ID（0 = 无渠道关联）
	Mapped             bool   // 是否发生了映射
	BillingModelSource string // 计费模型来源（"requested" / "upstream" / "channel_mapped"）
REDACTED

// BuildModelMappingChain 根据映射结果和上游实际模型构建映射链描述。
// reqModel: 客户端请求的原始模型名。
// upstreamModel: 上游实际使用的模型名（ForwardResult.UpstreamModel）。
// 返回空字符串表示无映射。
func (r ChannelMappingResult) BuildModelMappingChain(reqModel, upstreamModel string) string {
	if !r.Mapped {
		if upstreamModel != "" && upstreamModel != reqModel {
			return reqModel + "→" + upstreamModel
	REDACTED
		return ""
REDACTED
	if upstreamModel != "" && upstreamModel != r.MappedModel {
		return reqModel + "→" + r.MappedModel + "→" + upstreamModel
REDACTED
	return reqModel + "→" + r.MappedModel
REDACTED

// ToUsageFields 将渠道映射结果转为使用记录字段
func (r ChannelMappingResult) ToUsageFields(reqModel, upstreamModel string) ChannelUsageFields {
	channelMappedModel := reqModel
	if r.Mapped {
		channelMappedModel = r.MappedModel
REDACTED
	return ChannelUsageFields{
		ChannelID:          r.ChannelID,
		OriginalModel:      reqModel,
		ChannelMappedModel: channelMappedModel,
		BillingModelSource: r.BillingModelSource,
		ModelMappingChain:  r.BuildModelMappingChain(reqModel, upstreamModel),
REDACTED
REDACTED

const (
	channelCacheTTL       = 10 * time.Minute
	channelErrorTTL       = 5 * time.Second // DB 错误时的短缓存
	channelCacheDBTimeout = 10 * time.Second
)

// ChannelService 渠道管理服务
type ChannelService struct {
	repo                 ChannelRepository
	groupRepo            GroupRepository
	authCacheInvalidator APIKeyAuthCacheInvalidator
	pricingService       *PricingService // 用于「可用渠道」展示时回落到全局定价；可为 nil（测试场景）

	cache   atomic.Value // *channelCache
	cacheSF singleflight.Group
REDACTED

// NewChannelService 创建渠道服务实例。
// pricingService 仅供 ListAvailable 在渠道未配置定价时回落到全局 LiteLLM 数据；
// 计费热路径走独立的 ModelPricingResolver，与此参数无关。可传 nil。
func NewChannelService(repo ChannelRepository, groupRepo GroupRepository, authCacheInvalidator APIKeyAuthCacheInvalidator, pricingService *PricingService) *ChannelService {
	s := &ChannelService{
		repo:                 repo,
		groupRepo:            groupRepo,
		authCacheInvalidator: authCacheInvalidator,
		pricingService:       pricingService,
REDACTED
	return s
REDACTED

// loadCache 加载或返回缓存的渠道数据
func (s *ChannelService) loadCache(ctx context.Context) (*channelCache, error) {
	if cached, ok := s.cache.Load().(*channelCache); ok && cached != nil {
		if time.Since(cached.loadedAt) < channelCacheTTL {
			return cached, nil
	REDACTED
REDACTED

	result, err, _ := s.cacheSF.Do("channel_cache", func() (any, error) {
		// 双重检查
		if cached, ok := s.cache.Load().(*channelCache); ok && cached != nil {
			if time.Since(cached.loadedAt) < channelCacheTTL {
				return cached, nil
		REDACTED
	REDACTED
		return s.buildCache(ctx)
REDACTED)
	if err != nil {
		return nil, err
REDACTED
	cache, ok := result.(*channelCache)
	if !ok {
		return nil, fmt.Errorf("unexpected cache type")
REDACTED
	return cache, nil
REDACTED

// newEmptyChannelCache 创建空的渠道缓存（所有 map 已初始化）
func newEmptyChannelCache() *channelCache {
	return &channelCache{
		pricingByGroupModel:     make(map[channelModelKey]*ChannelModelPricing),
		wildcardByGroupPlatform: make(map[channelGroupPlatformKey][]*wildcardPricingEntry),
		mappingByGroupModel:     make(map[channelModelKey]string),
		wildcardMappingByGP:     make(map[channelGroupPlatformKey][]*wildcardMappingEntry),
		channelByGroupID:        make(map[int64]*Channel),
		groupPlatform:           make(map[int64]string),
		byID:                    make(map[int64]*Channel),
REDACTED
REDACTED

// expandPricingToCache 将渠道的模型定价展开到缓存（按分组+平台维度）。
// 各平台严格独立：antigravity 分组只匹配 antigravity 定价，不会匹配 anthropic/gemini 的定价。
// 查找时通过 lookupPricingAcrossPlatforms() 在本平台内查找。
func expandPricingToCache(cache *channelCache, ch *Channel, gid int64, platform string) {
	for j := range ch.ModelPricing {
		pricing := &ch.ModelPricing[j]
		if !isPlatformPricingMatch(platform, pricing.Platform) {
			continue // 跳过非本平台的定价
	REDACTED
		// 使用定价条目的原始平台作为缓存 key，防止跨平台同名模型冲突
		pricingPlatform := pricing.Platform
		gpKey := channelGroupPlatformKey{groupID: gid, platform: pricingPlatformREDACTED
		for _, model := range pricing.Models {
			if strings.HasSuffix(model, "*") {
				prefix := strings.ToLower(strings.TrimSuffix(model, "*"))
				cache.wildcardByGroupPlatform[gpKey] = append(cache.wildcardByGroupPlatform[gpKey], &wildcardPricingEntry{
					prefix:  prefix,
					pricing: pricing,
			REDACTED)
		REDACTED else {
				key := channelModelKey{groupID: gid, platform: pricingPlatform, model: strings.ToLower(model)REDACTED
				cache.pricingByGroupModel[key] = pricing
		REDACTED
	REDACTED
REDACTED
REDACTED

// expandMappingToCache 将渠道的模型映射展开到缓存（按分组+平台维度）。
// 各平台严格独立：antigravity 分组只匹配 antigravity 映射。
func expandMappingToCache(cache *channelCache, ch *Channel, gid int64, platform string) {
	for _, mappingPlatform := range matchingPlatforms(platform) {
		platformMapping, ok := ch.ModelMapping[mappingPlatform]
		if !ok {
			continue
	REDACTED
		// 使用映射条目的原始平台作为缓存 key，防止跨平台同名映射冲突
		gpKey := channelGroupPlatformKey{groupID: gid, platform: mappingPlatformREDACTED
		for src, dst := range platformMapping {
			if strings.HasSuffix(src, "*") {
				prefix := strings.ToLower(strings.TrimSuffix(src, "*"))
				cache.wildcardMappingByGP[gpKey] = append(cache.wildcardMappingByGP[gpKey], &wildcardMappingEntry{
					prefix: prefix,
					target: dst,
			REDACTED)
		REDACTED else {
				key := channelModelKey{groupID: gid, platform: mappingPlatform, model: strings.ToLower(src)REDACTED
				cache.mappingByGroupModel[key] = dst
		REDACTED
	REDACTED
REDACTED
REDACTED

// storeErrorCache 存入短 TTL 空缓存，防止 DB 错误后紧密重试。
// 通过回退 loadedAt 使剩余 TTL = channelErrorTTL。
func (s *ChannelService) storeErrorCache() {
	errorCache := newEmptyChannelCache()
	errorCache.loadedAt = time.Now().Add(-(channelCacheTTL - channelErrorTTL))
	s.cache.Store(errorCache)
REDACTED

// buildCache 从数据库构建渠道缓存。
// 使用独立 context 避免请求取消导致空值被长期缓存。
func (s *ChannelService) buildCache(ctx context.Context) (*channelCache, error) {
	dbCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), channelCacheDBTimeout)
	defer cancel()

	channels, groupPlatforms, err := s.fetchChannelData(dbCtx)
	if err != nil {
		return nil, err
REDACTED

	cache := populateChannelCache(channels, groupPlatforms)
	s.cache.Store(cache)
	return cache, nil
REDACTED

// fetchChannelData 从数据库加载渠道列表和分组平台映射。
func (s *ChannelService) fetchChannelData(ctx context.Context) ([]Channel, map[int64]string, error) {
	channels, err := s.repo.ListAll(ctx)
	if err != nil {
		slog.Warn("failed to build channel cache", "error", err)
		s.storeErrorCache()
		return nil, nil, fmt.Errorf("list all channels: %w", err)
REDACTED

	var allGroupIDs []int64
	for i := range channels {
		allGroupIDs = append(allGroupIDs, channels[i].GroupIDs...)
REDACTED

	groupPlatforms := make(map[int64]string)
	if len(allGroupIDs) > 0 {
		groupPlatforms, err = s.repo.GetGroupPlatforms(ctx, allGroupIDs)
		if err != nil {
			slog.Warn("failed to load group platforms for channel cache", "error", err)
			s.storeErrorCache()
			return nil, nil, fmt.Errorf("get group platforms: %w", err)
	REDACTED
REDACTED
	return channels, groupPlatforms, nil
REDACTED

// populateChannelCache 将渠道列表和分组平台映射填充到缓存快照中。
// 装填时对每个 Channel 统一归一化 BillingModelSource，让缓存命中的所有下游
// （gateway routing / billing / 未来任何 cache-backed 读路径）都拿到已归一化的实体，
// 避免"每个出口各自记得 normalize"反模式。
func populateChannelCache(channels []Channel, groupPlatforms map[int64]string) *channelCache {
	cache := newEmptyChannelCache()
	cache.groupPlatform = groupPlatforms
	cache.byID = make(map[int64]*Channel, len(channels))
	cache.loadedAt = time.Now()

	for i := range channels {
		channels[i].normalizeBillingModelSource()
		ch := &channels[i]
		cache.byID[ch.ID] = ch
		for _, gid := range ch.GroupIDs {
			cache.channelByGroupID[gid] = ch
			platform := groupPlatforms[gid]
			expandPricingToCache(cache, ch, gid, platform)
			expandMappingToCache(cache, ch, gid, platform)
	REDACTED
REDACTED

	return cache
REDACTED

// invalidateCache 使缓存失效，让下次读取时自然重建

// isPlatformPricingMatch 判断定价条目的平台是否匹配分组平台。
// 各平台（antigravity / anthropic / gemini / openai）严格独立，不跨平台匹配。
func isPlatformPricingMatch(groupPlatform, pricingPlatform string) bool {
	return groupPlatform == pricingPlatform
REDACTED

// matchingPlatforms 返回分组平台对应的可匹配平台列表。
// 各平台严格独立，只返回自身。
func matchingPlatforms(groupPlatform string) []string {
	return []string{groupPlatformREDACTED
REDACTED
func (s *ChannelService) invalidateCache() {
	s.cache.Store((*channelCache)(nil))
	s.cacheSF.Forget("channel_cache")

	// 主动重建缓存，确保 CRUD 后立即生效
	if _, err := s.buildCache(context.Background()); err != nil {
		slog.Warn("failed to rebuild channel cache after invalidation", "error", err)
REDACTED
REDACTED

// matchWildcard 在通配符定价中查找匹配项（最先匹配到优先）
func (c *channelCache) matchWildcard(groupID int64, platform, modelLower string) *ChannelModelPricing {
	gpKey := channelGroupPlatformKey{groupID: groupID, platform: platformREDACTED
	wildcards := c.wildcardByGroupPlatform[gpKey]
	for _, wc := range wildcards {
		if strings.HasPrefix(modelLower, wc.prefix) {
			return wc.pricing
	REDACTED
REDACTED
	return nil
REDACTED

// matchWildcardMapping 在通配符映射中查找匹配项（最先匹配到优先）
func (c *channelCache) matchWildcardMapping(groupID int64, platform, modelLower string) string {
	gpKey := channelGroupPlatformKey{groupID: groupID, platform: platformREDACTED
	wildcards := c.wildcardMappingByGP[gpKey]
	for _, wc := range wildcards {
		if strings.HasPrefix(modelLower, wc.prefix) {
			return wc.target
	REDACTED
REDACTED
	return ""
REDACTED

// lookupPricingAcrossPlatforms 在分组平台内查找模型定价。
// 各平台严格独立，只在本平台内查找（先精确匹配，再通配符）。
func lookupPricingAcrossPlatforms(cache *channelCache, groupID int64, groupPlatform, modelLower string) *ChannelModelPricing {
	for _, p := range matchingPlatforms(groupPlatform) {
		key := channelModelKey{groupID: groupID, platform: p, model: modelLowerREDACTED
		if pricing, ok := cache.pricingByGroupModel[key]; ok {
			return pricing
	REDACTED
REDACTED
	// 精确查找全部失败，依次尝试通配符匹配
	for _, p := range matchingPlatforms(groupPlatform) {
		if pricing := cache.matchWildcard(groupID, p, modelLower); pricing != nil {
			return pricing
	REDACTED
REDACTED
	return nil
REDACTED

// lookupMappingAcrossPlatforms 在分组平台内查找模型映射。
// 逻辑与 lookupPricingAcrossPlatforms 相同：先精确查找，再通配符。
func lookupMappingAcrossPlatforms(cache *channelCache, groupID int64, groupPlatform, modelLower string) string {
	for _, p := range matchingPlatforms(groupPlatform) {
		key := channelModelKey{groupID: groupID, platform: p, model: modelLowerREDACTED
		if mapped, ok := cache.mappingByGroupModel[key]; ok {
			return mapped
	REDACTED
REDACTED
	for _, p := range matchingPlatforms(groupPlatform) {
		if mapped := cache.matchWildcardMapping(groupID, p, modelLower); mapped != "" {
			return mapped
	REDACTED
REDACTED
	return ""
REDACTED

// GetChannelForGroup 获取分组关联的渠道（热路径 O(1)）
func (s *ChannelService) GetChannelForGroup(ctx context.Context, groupID int64) (*Channel, error) {
	cache, err := s.loadCache(ctx)
	if err != nil {
		return nil, err
REDACTED

	ch, ok := cache.channelByGroupID[groupID]
	if !ok || !ch.IsActive() {
		return nil, nil
REDACTED

	return ch.Clone(), nil
REDACTED

// GetGroupPlatform 获取分组的平台标识（从缓存）
func (s *ChannelService) GetGroupPlatform(ctx context.Context, groupID int64) string {
	cache, err := s.loadCache(ctx)
	if err != nil {
		return ""
REDACTED
	return cache.groupPlatform[groupID]
REDACTED

// channelLookup 热路径公共查找结果
type channelLookup struct {
	cache    *channelCache
	channel  *Channel
	platform string
REDACTED

// lookupGroupChannel 加载缓存并查找分组对应的渠道信息（公共热路径前置逻辑）。
// 返回 nil 且 err==nil 表示分组无活跃渠道；err!=nil 表示缓存加载失败。
func (s *ChannelService) lookupGroupChannel(ctx context.Context, groupID int64) (*channelLookup, error) {
	cache, err := s.loadCache(ctx)
	if err != nil {
		return nil, err
REDACTED
	ch, ok := cache.channelByGroupID[groupID]
	if !ok || !ch.IsActive() {
		return nil, nil
REDACTED
	return &channelLookup{
		cache:    cache,
		channel:  ch,
		platform: cache.groupPlatform[groupID],
REDACTED, nil
REDACTED

// GetChannelModelPricing 获取指定分组+模型的渠道定价（热路径 O(1)）。
// 各平台严格独立，只在本平台内查找定价。
func (s *ChannelService) GetChannelModelPricing(ctx context.Context, groupID int64, model string) *ChannelModelPricing {
	lk, err := s.lookupGroupChannel(ctx, groupID)
	if err != nil {
		slog.Warn("failed to load channel cache", "group_id", groupID, "error", err)
		return nil
REDACTED
	if lk == nil {
		return nil
REDACTED

	modelLower := strings.ToLower(model)
	pricing := lookupPricingAcrossPlatforms(lk.cache, groupID, lk.platform, modelLower)
	if pricing == nil {
		return nil
REDACTED

	cp := pricing.Clone()
	return &cp
REDACTED

// ResolveChannelMapping 解析渠道级模型映射（热路径 O(1)）
// 返回映射结果，包含映射后的模型名、渠道 ID、计费模型来源。
func (s *ChannelService) ResolveChannelMapping(ctx context.Context, groupID int64, model string) ChannelMappingResult {
	lk, err := s.lookupGroupChannel(ctx, groupID)
	if err != nil {
		slog.Warn("failed to load channel cache for mapping", "group_id", groupID, "error", err)
REDACTED
	if lk == nil {
		return ChannelMappingResult{MappedModel: modelREDACTED
REDACTED
	return resolveMapping(lk, groupID, model)
REDACTED

// IsModelRestricted 检查模型是否被渠道限制。
// 返回 true 表示模型被限制（不在允许列表中）。
// 如果渠道未启用模型限制或分组无渠道关联，返回 false。
func (s *ChannelService) IsModelRestricted(ctx context.Context, groupID int64, model string) bool {
	lk, err := s.lookupGroupChannel(ctx, groupID)
	if err != nil {
		slog.Warn("failed to load channel cache for model restriction check", "group_id", groupID, "error", err)
REDACTED
	if lk == nil {
		return false
REDACTED
	return checkRestricted(lk, groupID, model)
REDACTED

// ResolveChannelMappingAndRestrict 解析渠道映射。
// 返回映射结果。模型限制检查已移至调度阶段（GatewayService.checkChannelPricingRestriction），
// restricted 始终返回 false，保留签名兼容性。
func (s *ChannelService) ResolveChannelMappingAndRestrict(ctx context.Context, groupID *int64, model string) (ChannelMappingResult, bool) {
	if groupID == nil {
		return ChannelMappingResult{MappedModel: modelREDACTED, false
REDACTED
	lk, _ := s.lookupGroupChannel(ctx, *groupID)
	if lk == nil {
		return ChannelMappingResult{MappedModel: modelREDACTED, false
REDACTED
	return resolveMapping(lk, *groupID, model), false
REDACTED

// resolveMapping 基于已查找的渠道信息解析模型映射。
// antigravity 分组依次尝试所有匹配平台，确保跨平台同名映射各自独立。
func resolveMapping(lk *channelLookup, groupID int64, model string) ChannelMappingResult {
	// lk.channel 来自已装填的缓存，BillingModelSource 已在 populateChannelCache 阶段归一化，
	// 这里无需重复兜底。
	result := ChannelMappingResult{
		MappedModel:        model,
		ChannelID:          lk.channel.ID,
		BillingModelSource: lk.channel.BillingModelSource,
REDACTED

	modelLower := strings.ToLower(model)
	if mapped := lookupMappingAcrossPlatforms(lk.cache, groupID, lk.platform, modelLower); mapped != "" {
		result.MappedModel = mapped
		result.Mapped = true
REDACTED

	return result
REDACTED

// checkRestricted 基于已查找的渠道信息检查模型是否被限制。
// 只在本平台的定价列表中查找。
func checkRestricted(lk *channelLookup, groupID int64, model string) bool {
	if !lk.channel.RestrictModels {
		return false
REDACTED
	modelLower := strings.ToLower(model)
	// 使用与查找定价相同的跨平台逻辑
	if lookupPricingAcrossPlatforms(lk.cache, groupID, lk.platform, modelLower) != nil {
		return false
REDACTED
	return true
REDACTED

// ReplaceModelInBody 替换请求体 JSON 中的 model 字段。
func ReplaceModelInBody(body []byte, newModel string) []byte {
	if len(body) == 0 {
		return body
REDACTED
	if current := gjson.GetBytes(body, "model"); current.Exists() && current.String() == newModel {
		return body
REDACTED
	newBody, err := sjson.SetBytes(body, "model", newModel)
	if err != nil {
		return body
REDACTED
	return newBody
REDACTED

// RemovePreviousResponseIDFromBody 删除请求体中的 previous_response_id，用于会话失配时改用完整 input 重建上下文。
func RemovePreviousResponseIDFromBody(body []byte) []byte {
	if len(body) == 0 {
		return body
REDACTED
	if !gjson.GetBytes(body, "previous_response_id").Exists() {
		return body
REDACTED
	newBody, err := sjson.DeleteBytes(body, "previous_response_id")
	if err != nil {
		return body
REDACTED
	return newBody
REDACTED

// validateChannelConfig 校验渠道的定价和映射配置（冲突检测 + 区间校验 + 计费模式校验）。
// Create 和 Update 共用此函数，避免重复。
func validateChannelConfig(pricing []ChannelModelPricing, mapping map[string]map[string]string) error {
	if err := validatePricingEntries(pricing); err != nil {
		return err
REDACTED
	return validateNoConflictingMappings(mapping)
REDACTED

// validatePricingEntries 校验定价条目（冲突检测 + 区间校验 + 计费模式校验），
// 同时用于主渠道定价和 account_stats_pricing_rules 的内部定价。
func validatePricingEntries(pricing []ChannelModelPricing) error {
	if err := validateNoConflictingModels(pricing); err != nil {
		return err
REDACTED
	if err := validatePricingIntervals(pricing); err != nil {
		return err
REDACTED
	return validatePricingBillingMode(pricing)
REDACTED

// validatePricingBillingMode 校验计费模式配置：按次/图片模式必须配价格或区间，所有价格字段不能为负，区间至少有一个价格字段。
func validatePricingBillingMode(pricing []ChannelModelPricing) error {
	for _, p := range pricing {
		if err := checkBillingModeRequirements(p); err != nil {
			return err
	REDACTED
		if err := checkPricesNotNegative(p); err != nil {
			return err
	REDACTED
		if err := checkIntervalsHavePrices(p); err != nil {
			return err
	REDACTED
REDACTED
	return nil
REDACTED

func checkBillingModeRequirements(p ChannelModelPricing) error {
	if p.BillingMode == BillingModePerRequest || p.BillingMode == BillingModeImage {
		if p.PerRequestPrice == nil && len(p.Intervals) == 0 {
			return infraerrors.BadRequest(
				"BILLING_MODE_MISSING_PRICE",
				"per-request price or intervals required for per_request/image billing mode",
			)
	REDACTED
REDACTED
	return nil
REDACTED

func checkPricesNotNegative(p ChannelModelPricing) error {
	checks := []struct {
		field string
		val   *float64
REDACTED{
		{"input_price", p.InputPriceREDACTED,
		{"output_price", p.OutputPriceREDACTED,
		{"cache_write_price", p.CacheWritePriceREDACTED,
		{"cache_read_price", p.CacheReadPriceREDACTED,
		{"image_output_price", p.ImageOutputPriceREDACTED,
		{"per_request_price", p.PerRequestPriceREDACTED,
REDACTED
	for _, c := range checks {
		if c.val != nil && *c.val < 0 {
			return infraerrors.BadRequest("NEGATIVE_PRICE", fmt.Sprintf("%s must be >= 0", c.field))
	REDACTED
REDACTED
	return nil
REDACTED

func checkIntervalsHavePrices(p ChannelModelPricing) error {
	for _, iv := range p.Intervals {
		if iv.InputPrice == nil && iv.OutputPrice == nil &&
			iv.CacheWritePrice == nil && iv.CacheReadPrice == nil &&
			iv.PerRequestPrice == nil {
			return infraerrors.BadRequest(
				"INTERVAL_MISSING_PRICE",
				fmt.Sprintf("interval [%d, %s] has no price fields set for model %v",
					iv.MinTokens, formatMaxTokens(iv.MaxTokens), p.Models),
			)
	REDACTED
REDACTED
	return nil
REDACTED

func formatMaxTokens(max *int) string {
	if max == nil {
		return "∞"
REDACTED
	return fmt.Sprintf("%d", *max)
REDACTED

// --- CRUD ---

// Create 创建渠道
func (s *ChannelService) Create(ctx context.Context, input *CreateChannelInput) (*Channel, error) {
	exists, err := s.repo.ExistsByName(ctx, input.Name)
	if err != nil {
		return nil, fmt.Errorf("check channel exists: %w", err)
REDACTED
	if exists {
		return nil, ErrChannelExists
REDACTED

	if err := s.checkGroupConflicts(ctx, 0, input.GroupIDs); err != nil {
		return nil, err
REDACTED

	channel := &Channel{
		Name:                       input.Name,
		Description:                input.Description,
		Status:                     StatusActive,
		BillingModelSource:         input.BillingModelSource,
		RestrictModels:             input.RestrictModels,
		GroupIDs:                   input.GroupIDs,
		ModelPricing:               input.ModelPricing,
		ModelMapping:               input.ModelMapping,
		Features:                   input.Features,
		FeaturesConfig:             input.FeaturesConfig,
		ApplyPricingToAccountStats: input.ApplyPricingToAccountStats,
		AccountStatsPricingRules:   input.AccountStatsPricingRules,
REDACTED
	channel.normalizeBillingModelSource()

	if err := validateChannelConfig(channel.ModelPricing, channel.ModelMapping); err != nil {
		return nil, err
REDACTED
	for i, rule := range channel.AccountStatsPricingRules {
		if err := validatePricingEntries(rule.Pricing); err != nil {
			return nil, fmt.Errorf("account stats pricing rule #%d: %w", i+1, err)
	REDACTED
REDACTED

	if err := s.repo.Create(ctx, channel); err != nil {
		return nil, fmt.Errorf("create channel: %w", err)
REDACTED

	s.invalidateCache()
	created, err := s.repo.GetByID(ctx, channel.ID)
	if err != nil {
		return nil, err
REDACTED
	created.normalizeBillingModelSource()
	return created, nil
REDACTED

// GetByID 获取渠道详情。返回前统一把空 BillingModelSource 回填为 ChannelMapped，
// 让所有 handler 无需重复处理历史空值。
func (s *ChannelService) GetByID(ctx context.Context, id int64) (*Channel, error) {
	ch, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
REDACTED
	ch.normalizeBillingModelSource()
	return ch, nil
REDACTED

// Update 更新渠道
func (s *ChannelService) Update(ctx context.Context, id int64, input *UpdateChannelInput) (*Channel, error) {
	channel, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get channel: %w", err)
REDACTED

	if err := s.applyUpdateInput(ctx, channel, input); err != nil {
		return nil, err
REDACTED

	if err := validateChannelConfig(channel.ModelPricing, channel.ModelMapping); err != nil {
		return nil, err
REDACTED
	for i, rule := range channel.AccountStatsPricingRules {
		if err := validatePricingEntries(rule.Pricing); err != nil {
			return nil, fmt.Errorf("account stats pricing rule #%d: %w", i+1, err)
	REDACTED
REDACTED

	oldGroupIDs := s.getOldGroupIDs(ctx, id)

	if err := s.repo.Update(ctx, channel); err != nil {
		return nil, fmt.Errorf("update channel: %w", err)
REDACTED

	s.invalidateCache()
	s.invalidateAuthCacheForGroups(ctx, oldGroupIDs, channel.GroupIDs)

	updated, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
REDACTED
	updated.normalizeBillingModelSource()
	return updated, nil
REDACTED

// applyUpdateInput 将更新请求的字段应用到渠道实体上。
func (s *ChannelService) applyUpdateInput(ctx context.Context, channel *Channel, input *UpdateChannelInput) error {
	if input.Name != "" && input.Name != channel.Name {
		exists, err := s.repo.ExistsByNameExcluding(ctx, input.Name, channel.ID)
		if err != nil {
			return fmt.Errorf("check channel exists: %w", err)
	REDACTED
		if exists {
			return ErrChannelExists
	REDACTED
		channel.Name = input.Name
REDACTED
	if input.Description != nil {
		channel.Description = *input.Description
REDACTED
	if input.Status != "" {
		channel.Status = input.Status
REDACTED
	if input.RestrictModels != nil {
		channel.RestrictModels = *input.RestrictModels
REDACTED
	if input.Features != nil {
		channel.Features = *input.Features
REDACTED
	if input.GroupIDs != nil {
		if err := s.checkGroupConflicts(ctx, channel.ID, *input.GroupIDs); err != nil {
			return err
	REDACTED
		channel.GroupIDs = *input.GroupIDs
REDACTED
	if input.ModelPricing != nil {
		channel.ModelPricing = *input.ModelPricing
REDACTED
	if input.ModelMapping != nil {
		channel.ModelMapping = input.ModelMapping
REDACTED
	if input.BillingModelSource != "" {
		channel.BillingModelSource = input.BillingModelSource
REDACTED
	if input.FeaturesConfig != nil {
		channel.FeaturesConfig = input.FeaturesConfig
REDACTED
	if input.ApplyPricingToAccountStats != nil {
		channel.ApplyPricingToAccountStats = *input.ApplyPricingToAccountStats
REDACTED
	if input.AccountStatsPricingRules != nil {
		channel.AccountStatsPricingRules = *input.AccountStatsPricingRules
REDACTED
	return nil
REDACTED

// checkGroupConflicts 检查待关联的分组是否已属于其他渠道。
// channelID 为当前渠道 ID（Create 时传 0）。
func (s *ChannelService) checkGroupConflicts(ctx context.Context, channelID int64, groupIDs []int64) error {
	if len(groupIDs) == 0 {
		return nil
REDACTED
	conflicting, err := s.repo.GetGroupsInOtherChannels(ctx, channelID, groupIDs)
	if err != nil {
		return fmt.Errorf("check group conflicts: %w", err)
REDACTED
	if len(conflicting) > 0 {
		return ErrGroupAlreadyInChannel
REDACTED
	return nil
REDACTED

// getOldGroupIDs 获取渠道更新前的关联分组 ID（用于失效 auth 缓存）。
func (s *ChannelService) getOldGroupIDs(ctx context.Context, channelID int64) []int64 {
	if s.authCacheInvalidator == nil {
		return nil
REDACTED
	oldGroupIDs, err := s.repo.GetGroupIDs(ctx, channelID)
	if err != nil {
		slog.Warn("failed to get old group IDs for cache invalidation", "channel_id", channelID, "error", err)
REDACTED
	return oldGroupIDs
REDACTED

// invalidateAuthCacheForGroups 对新旧分组去重后逐个失效 auth 缓存。
func (s *ChannelService) invalidateAuthCacheForGroups(ctx context.Context, groupIDSets ...[]int64) {
	if s.authCacheInvalidator == nil {
		return
REDACTED
	seen := make(map[int64]struct{REDACTED)
	for _, ids := range groupIDSets {
		for _, gid := range ids {
			if _, ok := seen[gid]; ok {
				continue
		REDACTED
			seen[gid] = struct{REDACTED{REDACTED
			s.authCacheInvalidator.InvalidateAuthCacheByGroupID(ctx, gid)
	REDACTED
REDACTED
REDACTED

// Delete 删除渠道
func (s *ChannelService) Delete(ctx context.Context, id int64) error {
	groupIDs, err := s.repo.GetGroupIDs(ctx, id)
	if err != nil {
		slog.Warn("failed to get group IDs before delete", "channel_id", id, "error", err)
REDACTED

	if err := s.repo.Delete(ctx, id); err != nil {
		return fmt.Errorf("delete channel: %w", err)
REDACTED

	s.invalidateCache()
	s.invalidateAuthCacheForGroups(ctx, groupIDs)

	return nil
REDACTED

// List 获取渠道列表
func (s *ChannelService) List(ctx context.Context, params pagination.PaginationParams, status, search string) ([]Channel, *pagination.PaginationResult, error) {
	channels, res, err := s.repo.List(ctx, params, status, search)
	if err != nil {
		return nil, nil, err
REDACTED
	for i := range channels {
		channels[i].normalizeBillingModelSource()
REDACTED
	return channels, res, nil
REDACTED

// modelEntry 表示一个模型模式条目（用于冲突检测）
type modelEntry struct {
	pattern  string // 原始模式（如 "claude-*" 或 "claude-opus-4"）
	prefix   string // lowercase 前缀（通配符去掉 *，精确名保持原样）
	wildcard bool
REDACTED

// conflictsBetween 检查两个模型模式是否冲突
func conflictsBetween(a, b modelEntry) bool {
	switch {
	case !a.wildcard && !b.wildcard:
		return a.prefix == b.prefix
	case a.wildcard && !b.wildcard:
		return strings.HasPrefix(b.prefix, a.prefix)
	case !a.wildcard && b.wildcard:
		return strings.HasPrefix(a.prefix, b.prefix)
	default:
		return strings.HasPrefix(a.prefix, b.prefix) ||
			strings.HasPrefix(b.prefix, a.prefix)
REDACTED
REDACTED

// toModelEntry 将模型名转换为 modelEntry
func toModelEntry(pattern string) modelEntry {
	prefix, isWild := splitWildcardSuffix(strings.ToLower(pattern))
	return modelEntry{pattern: pattern, prefix: prefix, wildcard: isWildREDACTED
REDACTED

// validateNoConflictingModels 检查定价列表中是否有冲突模型模式（同一平台下）。
// 冲突包括：精确重复、通配符之间的前缀包含、通配符与精确名的前缀匹配。
func validateNoConflictingModels(pricingList []ChannelModelPricing) error {
	byPlatform := make(map[string][]modelEntry)
	for _, p := range pricingList {
		for _, model := range p.Models {
			byPlatform[p.Platform] = append(byPlatform[p.Platform], toModelEntry(model))
	REDACTED
REDACTED
	for platform, entries := range byPlatform {
		if err := detectConflicts(entries, platform, "MODEL_PATTERN_CONFLICT", "model patterns"); err != nil {
			return err
	REDACTED
REDACTED
	return nil
REDACTED

// validateNoConflictingMappings 检查模型映射中是否有冲突的源模式
func validateNoConflictingMappings(mapping map[string]map[string]string) error {
	for platform, platformMapping := range mapping {
		entries := make([]modelEntry, 0, len(platformMapping))
		for src := range platformMapping {
			entries = append(entries, toModelEntry(src))
	REDACTED
		if err := detectConflicts(entries, platform, "MAPPING_PATTERN_CONFLICT", "mapping source patterns"); err != nil {
			return err
	REDACTED
REDACTED
	return nil
REDACTED

func validatePricingIntervals(pricingList []ChannelModelPricing) error {
	for _, pricing := range pricingList {
		if err := ValidateIntervals(pricing.Intervals, pricing.BillingMode); err != nil {
			return infraerrors.BadRequest(
				"INVALID_PRICING_INTERVALS",
				fmt.Sprintf("invalid pricing intervals for platform '%s' models %v: %v",
					pricing.Platform, pricing.Models, err),
			)
	REDACTED
REDACTED
	return nil
REDACTED

// detectConflicts 在一组 modelEntry 中检测冲突，返回带有 errCode 和 label 的错误
func detectConflicts(entries []modelEntry, platform, errCode, label string) error {
	for i := 0; i < len(entries); i++ {
		for j := i + 1; j < len(entries); j++ {
			if conflictsBetween(entries[i], entries[j]) {
				return infraerrors.BadRequest(errCode,
					fmt.Sprintf("%s '%s' and '%s' conflict in platform '%s': overlapping match range",
						label, entries[i].pattern, entries[j].pattern, platform))
		REDACTED
	REDACTED
REDACTED
	return nil
REDACTED

// --- Input types ---

// CreateChannelInput 创建渠道输入
type CreateChannelInput struct {
	Name                       string
	Description                string
	GroupIDs                   []int64
	ModelPricing               []ChannelModelPricing
	ModelMapping               map[string]map[string]string // platform → {src→dstREDACTED
	BillingModelSource         string
	RestrictModels             bool
	Features                   string
	FeaturesConfig             map[string]any
	ApplyPricingToAccountStats bool
	AccountStatsPricingRules   []AccountStatsPricingRule
REDACTED

// UpdateChannelInput 更新渠道输入
type UpdateChannelInput struct {
	Name                       string
	Description                *string
	Status                     string
	GroupIDs                   *[]int64
	ModelPricing               *[]ChannelModelPricing
	ModelMapping               map[string]map[string]string // platform → {src→dstREDACTED
	BillingModelSource         string
	RestrictModels             *bool
	Features                   *string
	FeaturesConfig             map[string]any
	ApplyPricingToAccountStats *bool
	AccountStatsPricingRules   *[]AccountStatsPricingRule
REDACTED
