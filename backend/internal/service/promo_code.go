package service

import (
	"time"
)

// PromoCode 注册优惠码
type PromoCode struct {
	ID          int64
	Code        string
	BonusAmount float64
	MaxUses     int
	UsedCount   int
	Status      string
	ExpiresAt   *time.Time
	Notes       string
	CreatedAt   time.Time
	UpdatedAt   time.Time

	// 关联
	UsageRecords []PromoCodeUsage
REDACTED

// PromoCodeUsage 优惠码使用记录
type PromoCodeUsage struct {
	ID          int64
	PromoCodeID int64
	UserID      int64
	BonusAmount float64
	UsedAt      time.Time

	// 关联
	PromoCode *PromoCode
	User      *User
REDACTED

// CanUse 检查优惠码是否可用
func (p *PromoCode) CanUse() bool {
	if p.Status != PromoCodeStatusActive {
		return false
REDACTED
	if p.ExpiresAt != nil && time.Now().After(*p.ExpiresAt) {
		return false
REDACTED
	if p.MaxUses > 0 && p.UsedCount >= p.MaxUses {
		return false
REDACTED
	return true
REDACTED

// IsExpired 检查是否已过期
func (p *PromoCode) IsExpired() bool {
	return p.ExpiresAt != nil && time.Now().After(*p.ExpiresAt)
REDACTED

// CreatePromoCodeInput 创建优惠码输入
type CreatePromoCodeInput struct {
	Code        string
	BonusAmount float64
	MaxUses     int
	ExpiresAt   *time.Time
	Notes       string
REDACTED

// UpdatePromoCodeInput 更新优惠码输入
type UpdatePromoCodeInput struct {
	Code        *string
	BonusAmount *float64
	MaxUses     *int
	Status      *string
	ExpiresAt   *time.Time
	Notes       *string
REDACTED
