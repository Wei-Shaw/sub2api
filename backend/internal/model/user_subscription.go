package model

import (
	"time"
)

// 订阅状态常量
const (
	SubscriptionStatusActive    = "active"
	SubscriptionStatusExpired   = "expired"
	SubscriptionStatusSuspended = "suspended"
)

// UserSubscription 用户订阅模型
type UserSubscription struct {
	ID      int64 `gorm:"primaryKey" json:"id"`
	UserID  int64 `gorm:"index;not null" json:"user_id"`
	GroupID int64 `gorm:"index;not null" json:"group_id"`

	// 订阅有效期
	StartsAt  time.Time `gorm:"not null" json:"starts_at"`
	ExpiresAt time.Time `gorm:"not null" json:"expires_at"`
	Status    string    `gorm:"size:20;default:active;not null" json:"status"` // active/expired/suspended

	// 滑动窗口起始时间（nil = 未激活）
	DailyWindowStart   *time.Time `json:"daily_window_start"`
	WeeklyWindowStart  *time.Time `json:"weekly_window_start"`
	MonthlyWindowStart *time.Time `json:"monthly_window_start"`

	// 当前窗口已用额度（USD，基于 total_cost 计算）
	DailyUsageUSD   float64 `gorm:"type:decimal(20,10);default:0;not null" json:"daily_usage_usd"`
	WeeklyUsageUSD  float64 `gorm:"type:decimal(20,10);default:0;not null" json:"weekly_usage_usd"`
	MonthlyUsageUSD float64 `gorm:"type:decimal(20,10);default:0;not null" json:"monthly_usage_usd"`

	// 管理员分配信息
	AssignedBy *int64    `gorm:"index" json:"assigned_by"`
	AssignedAt time.Time `gorm:"not null" json:"assigned_at"`
	Notes      string    `gorm:"type:text" json:"notes"`

	CreatedAt time.Time `gorm:"not null" json:"created_at"`
	UpdatedAt time.Time `gorm:"not null" json:"updated_at"`

	// 关联
	User           *User  `gorm:"foreignKey:UserID" json:"user,omitempty"`
	Group          *Group `gorm:"foreignKey:GroupID" json:"group,omitempty"`
	AssignedByUser *User  `gorm:"foreignKey:AssignedBy" json:"assigned_by_user,omitempty"`
REDACTED

func (UserSubscription) TableName() string {
	return "user_subscriptions"
REDACTED

// IsActive 检查订阅是否有效（状态为active且未过期）
func (s *UserSubscription) IsActive() bool {
	return s.Status == SubscriptionStatusActive && time.Now().Before(s.ExpiresAt)
REDACTED

// IsExpired 检查订阅是否已过期
func (s *UserSubscription) IsExpired() bool {
	return time.Now().After(s.ExpiresAt)
REDACTED

// DaysRemaining 返回订阅剩余天数
func (s *UserSubscription) DaysRemaining() int {
	if s.IsExpired() {
		return 0
REDACTED
	return int(time.Until(s.ExpiresAt).Hours() / 24)
REDACTED

// IsWindowActivated 检查窗口是否已激活
func (s *UserSubscription) IsWindowActivated() bool {
	return s.DailyWindowStart != nil || s.WeeklyWindowStart != nil || s.MonthlyWindowStart != nil
REDACTED

// NeedsDailyReset 检查日窗口是否需要重置
func (s *UserSubscription) NeedsDailyReset() bool {
	if s.DailyWindowStart == nil {
		return false
REDACTED
	return time.Since(*s.DailyWindowStart) >= 24*time.Hour
REDACTED

// NeedsWeeklyReset 检查周窗口是否需要重置
func (s *UserSubscription) NeedsWeeklyReset() bool {
	if s.WeeklyWindowStart == nil {
		return false
REDACTED
	return time.Since(*s.WeeklyWindowStart) >= 7*24*time.Hour
REDACTED

// NeedsMonthlyReset 检查月窗口是否需要重置
func (s *UserSubscription) NeedsMonthlyReset() bool {
	if s.MonthlyWindowStart == nil {
		return false
REDACTED
	return time.Since(*s.MonthlyWindowStart) >= 30*24*time.Hour
REDACTED

// DailyResetTime 返回日窗口重置时间
func (s *UserSubscription) DailyResetTime() *time.Time {
	if s.DailyWindowStart == nil {
		return nil
REDACTED
	t := s.DailyWindowStart.Add(24 * time.Hour)
	return &t
REDACTED

// WeeklyResetTime 返回周窗口重置时间
func (s *UserSubscription) WeeklyResetTime() *time.Time {
	if s.WeeklyWindowStart == nil {
		return nil
REDACTED
	t := s.WeeklyWindowStart.Add(7 * 24 * time.Hour)
	return &t
REDACTED

// MonthlyResetTime 返回月窗口重置时间
func (s *UserSubscription) MonthlyResetTime() *time.Time {
	if s.MonthlyWindowStart == nil {
		return nil
REDACTED
	t := s.MonthlyWindowStart.Add(30 * 24 * time.Hour)
	return &t
REDACTED

// CheckDailyLimit 检查是否超出日限额
func (s *UserSubscription) CheckDailyLimit(group *Group, additionalCost float64) bool {
	if !group.HasDailyLimit() {
		return true // 无限制
REDACTED
	return s.DailyUsageUSD+additionalCost <= *group.DailyLimitUSD
REDACTED

// CheckWeeklyLimit 检查是否超出周限额
func (s *UserSubscription) CheckWeeklyLimit(group *Group, additionalCost float64) bool {
	if !group.HasWeeklyLimit() {
		return true // 无限制
REDACTED
	return s.WeeklyUsageUSD+additionalCost <= *group.WeeklyLimitUSD
REDACTED

// CheckMonthlyLimit 检查是否超出月限额
func (s *UserSubscription) CheckMonthlyLimit(group *Group, additionalCost float64) bool {
	if !group.HasMonthlyLimit() {
		return true // 无限制
REDACTED
	return s.MonthlyUsageUSD+additionalCost <= *group.MonthlyLimitUSD
REDACTED

// CheckAllLimits 检查所有限额
func (s *UserSubscription) CheckAllLimits(group *Group, additionalCost float64) (daily, weekly, monthly bool) {
	daily = s.CheckDailyLimit(group, additionalCost)
	weekly = s.CheckWeeklyLimit(group, additionalCost)
	monthly = s.CheckMonthlyLimit(group, additionalCost)
	return
REDACTED
