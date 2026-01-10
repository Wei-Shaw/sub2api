package service

import "time"

type Group struct {
	ID             int64
	Name           string
	Description    string
	Platform       string
	RateMultiplier float64
	IsExclusive    bool
	Status         string
	Hydrated       bool // indicates the group was loaded from a trusted repository source

	SubscriptionType    string
	DailyLimitUSD       *float64
	WeeklyLimitUSD      *float64
	MonthlyLimitUSD     *float64
	DefaultValidityDays int

	// 图片生成计费配置（antigravity 和 gemini 平台使用）
	ImagePrice1K *float64
	ImagePrice2K *float64
	ImagePrice4K *float64

	// Claude Code 客户端限制
	ClaudeCodeOnly  bool
	FallbackGroupID *int64

	CreatedAt time.Time
	UpdatedAt time.Time

	AccountGroups []AccountGroup
	AccountCount  int64
REDACTED

func (g *Group) IsActive() bool {
	return g.Status == StatusActive
REDACTED

func (g *Group) IsSubscriptionType() bool {
	return g.SubscriptionType == SubscriptionTypeSubscription
REDACTED

func (g *Group) IsFreeSubscription() bool {
	return g.IsSubscriptionType() && g.RateMultiplier == 0
REDACTED

func (g *Group) HasDailyLimit() bool {
	return g.DailyLimitUSD != nil && *g.DailyLimitUSD > 0
REDACTED

func (g *Group) HasWeeklyLimit() bool {
	return g.WeeklyLimitUSD != nil && *g.WeeklyLimitUSD > 0
REDACTED

func (g *Group) HasMonthlyLimit() bool {
	return g.MonthlyLimitUSD != nil && *g.MonthlyLimitUSD > 0
REDACTED

// GetImagePrice 根据 image_size 返回对应的图片生成价格
// 如果分组未配置价格，返回 nil（调用方应使用默认值）
func (g *Group) GetImagePrice(imageSize string) *float64 {
	switch imageSize {
	case "1K":
		return g.ImagePrice1K
	case "2K":
		return g.ImagePrice2K
	case "4K":
		return g.ImagePrice4K
	default:
		// 未知尺寸默认按 2K 计费
		return g.ImagePrice2K
REDACTED
REDACTED

// IsGroupContextValid reports whether a group from context has the fields required for routing decisions.
func IsGroupContextValid(group *Group) bool {
	if group == nil {
		return false
REDACTED
	if group.ID <= 0 {
		return false
REDACTED
	if !group.Hydrated {
		return false
REDACTED
	if group.Platform == "" || group.Status == "" {
		return false
REDACTED
	return true
REDACTED
