package handler

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// AvailableChannelHandler 处理用户侧「可用渠道」查询。
//
// 用户侧接口委托 ChannelService.ListAvailable，并在返回前做三层过滤：
//  1. 行过滤：只保留状态为 Active 且与当前用户可访问分组有交集的渠道；
//  2. 分组过滤：渠道的 Groups 只保留用户可访问的那些；
//  3. 平台过滤：渠道的 SupportedModels 只保留平台在用户可见 Groups 中出现过的模型，
//     防止"渠道同时挂在 antigravity / anthropic 两个平台的分组上，用户只访问
//     antigravity，却看到 anthropic 模型"这类跨平台信息泄漏；
//  4. 字段白名单：仅返回用户需要的字段（省略 BillingModelSource / RestrictModels
//     / 内部 ID / Status 等管理字段）。
type AvailableChannelHandler struct {
	channelService *service.ChannelService
	apiKeyService  *service.APIKeyService
REDACTED

// NewAvailableChannelHandler 创建用户侧可用渠道 handler。
func NewAvailableChannelHandler(
	channelService *service.ChannelService,
	apiKeyService *service.APIKeyService,
) *AvailableChannelHandler {
	return &AvailableChannelHandler{
		channelService: channelService,
		apiKeyService:  apiKeyService,
REDACTED
REDACTED

// userAvailableGroup 用户可见的分组概要（白名单字段）。
type userAvailableGroup struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Platform string `json:"platform"`
REDACTED

// userSupportedModelPricing 用户可见的定价字段白名单。
type userSupportedModelPricing struct {
	BillingMode      string                   `json:"billing_mode"`
	InputPrice       *float64                 `json:"input_price"`
	OutputPrice      *float64                 `json:"output_price"`
	CacheWritePrice  *float64                 `json:"cache_write_price"`
	CacheReadPrice   *float64                 `json:"cache_read_price"`
	ImageOutputPrice *float64                 `json:"image_output_price"`
	PerRequestPrice  *float64                 `json:"per_request_price"`
	Intervals        []userPricingIntervalDTO `json:"intervals"`
REDACTED

// userPricingIntervalDTO 定价区间白名单（去掉内部 ID、SortOrder 等前端不渲染的字段）。
type userPricingIntervalDTO struct {
	MinTokens       int      `json:"min_tokens"`
	MaxTokens       *int     `json:"max_tokens"`
	TierLabel       string   `json:"tier_label,omitempty"`
	InputPrice      *float64 `json:"input_price"`
	OutputPrice     *float64 `json:"output_price"`
	CacheWritePrice *float64 `json:"cache_write_price"`
	CacheReadPrice  *float64 `json:"cache_read_price"`
	PerRequestPrice *float64 `json:"per_request_price"`
REDACTED

// userSupportedModel 用户可见的支持模型条目。
type userSupportedModel struct {
	Name     string                     `json:"name"`
	Platform string                     `json:"platform"`
	Pricing  *userSupportedModelPricing `json:"pricing"`
REDACTED

// userAvailableChannel 用户可见的渠道条目（白名单字段）。
type userAvailableChannel struct {
	Name            string               `json:"name"`
	Description     string               `json:"description"`
	Groups          []userAvailableGroup `json:"groups"`
	SupportedModels []userSupportedModel `json:"supported_models"`
REDACTED

// List 列出当前用户可见的「可用渠道」。
// GET /api/v1/channels/available
func (h *AvailableChannelHandler) List(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
REDACTED

	userGroups, err := h.apiKeyService.GetAvailableGroups(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	allowedGroupIDs := make(map[int64]struct{REDACTED, len(userGroups))
	for i := range userGroups {
		allowedGroupIDs[userGroups[i].ID] = struct{REDACTED{REDACTED
REDACTED

	channels, err := h.channelService.ListAvailable(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	out := make([]userAvailableChannel, 0, len(channels))
	for _, ch := range channels {
		if ch.Status != service.StatusActive {
			continue
	REDACTED
		visibleGroups := filterUserVisibleGroups(ch.Groups, allowedGroupIDs)
		if len(visibleGroups) == 0 {
			continue
	REDACTED
		allowedPlatforms := collectGroupPlatforms(visibleGroups)
		out = append(out, userAvailableChannel{
			Name:            ch.Name,
			Description:     ch.Description,
			Groups:          visibleGroups,
			SupportedModels: toUserSupportedModels(ch.SupportedModels, allowedPlatforms),
	REDACTED)
REDACTED

	response.Success(c, out)
REDACTED

// collectGroupPlatforms 聚合 visible groups 覆盖的平台集合，用于过滤 SupportedModels。
func collectGroupPlatforms(groups []userAvailableGroup) map[string]struct{REDACTED {
	set := make(map[string]struct{REDACTED, len(groups))
	for _, g := range groups {
		if g.Platform == "" {
			continue
	REDACTED
		set[g.Platform] = struct{REDACTED{REDACTED
REDACTED
	return set
REDACTED

// filterUserVisibleGroups 仅保留用户可访问的分组。
func filterUserVisibleGroups(
	groups []service.AvailableGroupRef,
	allowed map[int64]struct{REDACTED,
) []userAvailableGroup {
	visible := make([]userAvailableGroup, 0, len(groups))
	for _, g := range groups {
		if _, ok := allowed[g.ID]; !ok {
			continue
	REDACTED
		visible = append(visible, userAvailableGroup{
			ID:       g.ID,
			Name:     g.Name,
			Platform: g.Platform,
	REDACTED)
REDACTED
	return visible
REDACTED

// toUserSupportedModels 将 service 层支持模型转换为用户 DTO（字段白名单）。
// 仅保留平台在 allowedPlatforms 中的条目，防止跨平台模型信息泄漏。
// allowedPlatforms 为 nil 时不做平台过滤（保留全部，供测试或明确无过滤场景使用）。
func toUserSupportedModels(
	src []service.SupportedModel,
	allowedPlatforms map[string]struct{REDACTED,
) []userSupportedModel {
	out := make([]userSupportedModel, 0, len(src))
	for i := range src {
		m := src[i]
		if allowedPlatforms != nil {
			if _, ok := allowedPlatforms[m.Platform]; !ok {
				continue
		REDACTED
	REDACTED
		out = append(out, userSupportedModel{
			Name:     m.Name,
			Platform: m.Platform,
			Pricing:  toUserPricing(m.Pricing),
	REDACTED)
REDACTED
	return out
REDACTED

// toUserPricing 将 service 层定价转换为用户 DTO；入参为 nil 时返回 nil。
func toUserPricing(p *service.ChannelModelPricing) *userSupportedModelPricing {
	if p == nil {
		return nil
REDACTED
	intervals := make([]userPricingIntervalDTO, 0, len(p.Intervals))
	for _, iv := range p.Intervals {
		intervals = append(intervals, userPricingIntervalDTO{
			MinTokens:       iv.MinTokens,
			MaxTokens:       iv.MaxTokens,
			TierLabel:       iv.TierLabel,
			InputPrice:      iv.InputPrice,
			OutputPrice:     iv.OutputPrice,
			CacheWritePrice: iv.CacheWritePrice,
			CacheReadPrice:  iv.CacheReadPrice,
			PerRequestPrice: iv.PerRequestPrice,
	REDACTED)
REDACTED
	billingMode := string(p.BillingMode)
	if billingMode == "" {
		billingMode = string(service.BillingModeToken)
REDACTED
	return &userSupportedModelPricing{
		BillingMode:      billingMode,
		InputPrice:       p.InputPrice,
		OutputPrice:      p.OutputPrice,
		CacheWritePrice:  p.CacheWritePrice,
		CacheReadPrice:   p.CacheReadPrice,
		ImageOutputPrice: p.ImageOutputPrice,
		PerRequestPrice:  p.PerRequestPrice,
		Intervals:        intervals,
REDACTED
REDACTED
