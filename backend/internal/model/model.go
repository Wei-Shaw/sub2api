package model

import (
	"gorm.io/gorm"
)

// AutoMigrate 自动迁移所有模型
func AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(
		&User{REDACTED,
		&ApiKey{REDACTED,
		&Group{REDACTED,
		&Account{REDACTED,
		&AccountGroup{REDACTED,
		&Proxy{REDACTED,
		&RedeemCode{REDACTED,
		&UsageLog{REDACTED,
		&Setting{REDACTED,
		&UserSubscription{REDACTED,
	)
REDACTED

// 状态常量
const (
	StatusActive   = "active"
	StatusDisabled = "disabled"
	StatusError    = "error"
	StatusUnused   = "unused"
	StatusUsed     = "used"
	StatusExpired  = "expired"
)

// 角色常量
const (
	RoleAdmin = "admin"
	RoleUser  = "user"
)

// 平台常量
const (
	PlatformAnthropic = "anthropic"
	PlatformOpenAI    = "openai"
	PlatformGemini    = "gemini"
)

// 账号类型常量
const (
	AccountTypeOAuth      = "oauth"       // OAuth类型账号（full scope: profile + inference）
	AccountTypeSetupToken = "setup-token" // Setup Token类型账号（inference only scope）
	AccountTypeApiKey     = "apikey"      // API Key类型账号
)

// 卡密类型常量
const (
	RedeemTypeBalance      = "balance"
	RedeemTypeConcurrency  = "concurrency"
	RedeemTypeSubscription = "subscription"
)

// 管理员调整类型常量
const (
	AdjustmentTypeAdminBalance     = "admin_balance"     // 管理员调整余额
	AdjustmentTypeAdminConcurrency = "admin_concurrency" // 管理员调整并发数
)
