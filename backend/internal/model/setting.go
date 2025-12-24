package model

import (
	"time"
)

// Setting 系统设置模型（Key-Value存储）
type Setting struct {
	ID        int64     `gorm:"primaryKey" json:"id"`
	Key       string    `gorm:"uniqueIndex;size:100;not null" json:"key"`
	Value     string    `gorm:"type:text;not null" json:"value"`
	UpdatedAt time.Time `gorm:"not null" json:"updated_at"`
}

func (Setting) TableName() string {
	return "settings"
}

// 设置Key常量
const (
	// 注册设置
	SettingKeyRegistrationEnabled = "registration_enabled" // 是否开放注册
	SettingKeyEmailVerifyEnabled  = "email_verify_enabled" // 是否开启邮件验证

	// 邮件服务设置
	SettingKeySmtpHost     = "smtp_host"      // SMTP服务器地址
	SettingKeySmtpPort     = "smtp_port"      // SMTP端口
	SettingKeySmtpUsername = "smtp_username"  // SMTP用户名
	SettingKeySmtpPassword = "smtp_password"  // SMTP密码（加密存储）
	SettingKeySmtpFrom     = "smtp_from"      // 发件人地址
	SettingKeySmtpFromName = "smtp_from_name" // 发件人名称
	SettingKeySmtpUseTLS   = "smtp_use_tls"   // 是否使用TLS

	// Cloudflare Turnstile 设置
	SettingKeyTurnstileEnabled   = "turnstile_enabled"    // 是否启用 Turnstile 验证
	SettingKeyTurnstileSiteKey   = "turnstile_site_key"   // Turnstile Site Key
	SettingKeyTurnstileSecretKey = "turnstile_secret_key" // Turnstile Secret Key

	// OEM设置
	SettingKeySiteName     = "site_name"     // 网站名称
	SettingKeySiteLogo     = "site_logo"     // 网站Logo (base64)
	SettingKeySiteSubtitle = "site_subtitle" // 网站副标题
	SettingKeyApiBaseUrl   = "api_base_url"  // API端点地址（用于客户端配置和导入）
	SettingKeyContactInfo  = "contact_info"  // 客服联系方式

	// 默认配置
	SettingKeyDefaultConcurrency = "default_concurrency" // 新用户默认并发量
	SettingKeyDefaultBalance     = "default_balance"     // 新用户默认余额

	// 管理员 API Key
	SettingKeyAdminApiKey = "admin_api_key" // 全局管理员 API Key（用于外部系统集成）
)

// 管理员 API Key 前缀（与用户 sk- 前缀区分）
const AdminApiKeyPrefix = "admin-"

// SystemSettings 系统设置结构体（用于API响应）
type SystemSettings struct {
	// 注册设置
	RegistrationEnabled bool `json:"registration_enabled"`
	EmailVerifyEnabled  bool `json:"email_verify_enabled"`

	// 邮件服务设置
	SmtpHost     string `json:"smtp_host"`
	SmtpPort     int    `json:"smtp_port"`
	SmtpUsername string `json:"smtp_username"`
	SmtpPassword string `json:"smtp_password,omitempty"` // 不返回明文密码
	// smtp_password_set 用于前端判断是否已配置过密码
	SmtpPasswordSet bool `json:"smtp_password_set"`
	SmtpFrom     string `json:"smtp_from_email"`
	SmtpFromName string `json:"smtp_from_name"`
	SmtpUseTLS   bool   `json:"smtp_use_tls"`

	// Cloudflare Turnstile 设置
	TurnstileEnabled   bool   `json:"turnstile_enabled"`
	TurnstileSiteKey   string `json:"turnstile_site_key"`
	TurnstileSecretKey string `json:"turnstile_secret_key,omitempty"` // 不返回明文密钥
	// turnstile_secret_key_set 用于前端展示“已配置”状态
	TurnstileSecretKeySet bool `json:"turnstile_secret_key_set"`

	// OEM设置
	SiteName     string `json:"site_name"`
	SiteLogo     string `json:"site_logo"`
	SiteSubtitle string `json:"site_subtitle"`
	ApiBaseUrl   string `json:"api_base_url"`
	ContactInfo  string `json:"contact_info"`

	// 默认配置
	DefaultConcurrency int     `json:"default_concurrency"`
	DefaultBalance     float64 `json:"default_balance"`
}

// PublicSettings 公开设置（无需登录即可获取）
type PublicSettings struct {
	RegistrationEnabled bool   `json:"registration_enabled"`
	EmailVerifyEnabled  bool   `json:"email_verify_enabled"`
	TurnstileEnabled    bool   `json:"turnstile_enabled"`
	TurnstileSiteKey    string `json:"turnstile_site_key"`
	SiteName            string `json:"site_name"`
	SiteLogo            string `json:"site_logo"`
	SiteSubtitle        string `json:"site_subtitle"`
	ApiBaseUrl          string `json:"api_base_url"`
	ContactInfo         string `json:"contact_info"`
	Version             string `json:"version"`
}
