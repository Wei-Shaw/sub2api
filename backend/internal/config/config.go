package config

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	Server       ServerConfig       `mapstructure:"server"`
	Database     DatabaseConfig     `mapstructure:"database"`
	Redis        RedisConfig        `mapstructure:"redis"`
	JWT          JWTConfig          `mapstructure:"jwt"`
	// 安装向导相关配置（非本地访问需 token）
	Setup        SetupConfig        `mapstructure:"setup"`
	// CORS 跨域配置（空列表表示允许所有来源但不携带 Cookie）
	CORS         CORSConfig         `mapstructure:"cors"`
	// 安全相关配置（API Key 哈希等）
	Security     SecurityConfig     `mapstructure:"security"`
	// 代理探测配置（TLS 校验开关）
	Proxy        ProxyConfig        `mapstructure:"proxy"`
	Default      DefaultConfig      `mapstructure:"default"`
	RateLimit    RateLimitConfig    `mapstructure:"rate_limit"`
	Pricing      PricingConfig      `mapstructure:"pricing"`
	Gateway      GatewayConfig      `mapstructure:"gateway"`
	TokenRefresh TokenRefreshConfig `mapstructure:"token_refresh"`
	Timezone     string             `mapstructure:"timezone"` // e.g. "Asia/Shanghai", "UTC"
}

// TokenRefreshConfig OAuth token自动刷新配置
type TokenRefreshConfig struct {
	// 是否启用自动刷新
	Enabled bool `mapstructure:"enabled"`
	// 检查间隔（分钟）
	CheckIntervalMinutes int `mapstructure:"check_interval_minutes"`
	// 提前刷新时间（小时），在token过期前多久开始刷新
	RefreshBeforeExpiryHours float64 `mapstructure:"refresh_before_expiry_hours"`
	// 最大重试次数
	MaxRetries int `mapstructure:"max_retries"`
	// 重试退避基础时间（秒）
	RetryBackoffSeconds int `mapstructure:"retry_backoff_seconds"`
}

type PricingConfig struct {
	// 价格数据远程URL（默认使用LiteLLM镜像）
	RemoteURL string `mapstructure:"remote_url"`
	// 哈希校验文件URL
	HashURL string `mapstructure:"hash_url"`
	// 本地数据目录
	DataDir string `mapstructure:"data_dir"`
	// 回退文件路径
	FallbackFile string `mapstructure:"fallback_file"`
	// 更新间隔（小时）
	UpdateIntervalHours int `mapstructure:"update_interval_hours"`
	// 哈希校验间隔（分钟）
	HashCheckIntervalMinutes int `mapstructure:"hash_check_interval_minutes"`
}

type ServerConfig struct {
	Host              string   `mapstructure:"host"`
	Port              int      `mapstructure:"port"`
	Mode              string   `mapstructure:"mode"`                // debug/release
	ReadHeaderTimeout int      `mapstructure:"read_header_timeout"` // 读取请求头超时（秒）
	IdleTimeout       int      `mapstructure:"idle_timeout"`        // 空闲连接超时（秒）
	TrustedProxies    []string `mapstructure:"trusted_proxies"`      // 可信代理列表
}

// GatewayConfig API网关相关配置
type GatewayConfig struct {
	// 等待上游响应头的超时时间（秒），0表示无超时
	// 注意：这不影响流式数据传输，只控制等待响应头的时间
	ResponseHeaderTimeout int `mapstructure:"response_header_timeout"`
	// 上游请求总超时（秒），仅用于非流式请求
	UpstreamTimeout int `mapstructure:"upstream_timeout"`
}

func (s *ServerConfig) Address() string {
	return fmt.Sprintf("%s:%d", s.Host, s.Port)
}

type DatabaseConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
	DBName   string `mapstructure:"dbname"`
	SSLMode  string `mapstructure:"sslmode"`
}

func (d *DatabaseConfig) DSN() string {
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		d.Host, d.Port, d.User, d.Password, d.DBName, d.SSLMode,
	)
}

// DSNWithTimezone returns DSN with timezone setting
func (d *DatabaseConfig) DSNWithTimezone(tz string) string {
	if tz == "" {
		tz = "Asia/Shanghai"
	}
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s TimeZone=%s",
		d.Host, d.Port, d.User, d.Password, d.DBName, d.SSLMode, tz,
	)
}

type RedisConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}

func (r *RedisConfig) Address() string {
	return fmt.Sprintf("%s:%d", r.Host, r.Port)
}

type JWTConfig struct {
	Secret     string `mapstructure:"secret"`
	ExpireHour int    `mapstructure:"expire_hour"`
}

type SetupConfig struct {
	Token string `mapstructure:"token"`
}

type CORSConfig struct {
	AllowedOrigins []string `mapstructure:"allowed_origins"`
}

type SecurityConfig struct {
	ApiKeyHMACSecret string `mapstructure:"api_key_hmac_secret"`
	// Cookie SameSite 策略：lax/strict/none
	AuthCookieSameSite string `mapstructure:"auth_cookie_same_site"`
	// Cookie Secure 策略：auto/true/false
	AuthCookieSecure string `mapstructure:"auth_cookie_secure"`
	// 是否强制要求 Origin/Referer（Cookie 鉴权时）
	AuthCookieRequireOrigin bool `mapstructure:"auth_cookie_require_origin"`
}

type ProxyConfig struct {
	TLSInsecureSkipVerify bool `mapstructure:"tls_insecure_skip_verify"`
}

type DefaultConfig struct {
	AdminEmail      string  `mapstructure:"admin_email"`
	AdminPassword   string  `mapstructure:"admin_password"`
	UserConcurrency int     `mapstructure:"user_concurrency"`
	UserBalance     float64 `mapstructure:"user_balance"`
	ApiKeyPrefix    string  `mapstructure:"api_key_prefix"`
	RateMultiplier  float64 `mapstructure:"rate_multiplier"`
}

type RateLimitConfig struct {
	OverloadCooldownMinutes int `mapstructure:"overload_cooldown_minutes"` // 529过载冷却时间(分钟)
}

func Load() (*Config, error) {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")
	viper.AddConfigPath("./config")
	viper.AddConfigPath("/etc/sub2api")

	// 环境变量支持
	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	// 默认值
	setDefaults()

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("read config error: %w", err)
		}
		// 配置文件不存在时使用默认值
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config error: %w", err)
	}

	if len(cfg.Server.TrustedProxies) == 0 {
		cfg.Server.TrustedProxies = parseCommaList(viper.GetString("server.trusted_proxies"))
	}
	if len(cfg.CORS.AllowedOrigins) == 0 {
		cfg.CORS.AllowedOrigins = parseCommaList(viper.GetString("cors.allowed_origins"))
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validate config error: %w", err)
	}

	return &cfg, nil
}

func setDefaults() {
	// Server
	viper.SetDefault("server.host", "0.0.0.0")
	viper.SetDefault("server.port", 8080)
	viper.SetDefault("server.mode", "debug")
	viper.SetDefault("server.read_header_timeout", 30) // 30秒读取请求头
	viper.SetDefault("server.idle_timeout", 120)       // 120秒空闲超时

	// Database
	viper.SetDefault("database.host", "localhost")
	viper.SetDefault("database.port", 5432)
	viper.SetDefault("database.user", "postgres")
	viper.SetDefault("database.password", "postgres")
	viper.SetDefault("database.dbname", "sub2api")
	viper.SetDefault("database.sslmode", "disable")

	// Redis
	viper.SetDefault("redis.host", "localhost")
	viper.SetDefault("redis.port", 6379)
	viper.SetDefault("redis.password", "")
	viper.SetDefault("redis.db", 0)

	// JWT
	viper.SetDefault("jwt.secret", "change-me-in-production")
	viper.SetDefault("jwt.expire_hour", 24)

	// 安装向导
	viper.SetDefault("setup.token", "")

	// 跨域
	viper.SetDefault("cors.allowed_origins", []string{})

	// 安全
	viper.SetDefault("security.api_key_hmac_secret", "")
	viper.SetDefault("security.auth_cookie_same_site", "lax")
	viper.SetDefault("security.auth_cookie_secure", "auto")
	viper.SetDefault("security.auth_cookie_require_origin", true)

	// 代理
	viper.SetDefault("proxy.tls_insecure_skip_verify", false)

	// Default
	viper.SetDefault("default.admin_email", "admin@sub2api.com")
	viper.SetDefault("default.admin_password", "admin123")
	viper.SetDefault("default.user_concurrency", 5)
	viper.SetDefault("default.user_balance", 0)
	viper.SetDefault("default.api_key_prefix", "sk-")
	viper.SetDefault("default.rate_multiplier", 1.0)

	// RateLimit
	viper.SetDefault("rate_limit.overload_cooldown_minutes", 10)

	// Pricing - 从 price-mirror 分支同步，该分支维护了 sha256 哈希文件用于增量更新检查
	viper.SetDefault("pricing.remote_url", "https://raw.githubusercontent.com/Wei-Shaw/claude-relay-service/price-mirror/model_prices_and_context_window.json")
	viper.SetDefault("pricing.hash_url", "https://raw.githubusercontent.com/Wei-Shaw/claude-relay-service/price-mirror/model_prices_and_context_window.sha256")
	viper.SetDefault("pricing.data_dir", "./data")
	viper.SetDefault("pricing.fallback_file", "./resources/model-pricing/model_prices_and_context_window.json")
	viper.SetDefault("pricing.update_interval_hours", 24)
	viper.SetDefault("pricing.hash_check_interval_minutes", 10)

	// Timezone (default to Asia/Shanghai for Chinese users)
	viper.SetDefault("timezone", "Asia/Shanghai")

	// Gateway
	viper.SetDefault("gateway.response_header_timeout", 300) // 300秒(5分钟)等待上游响应头，LLM高负载时可能排队较久
	viper.SetDefault("gateway.upstream_timeout", 120)         // 120秒非流式上游请求总超时

	// TokenRefresh
	viper.SetDefault("token_refresh.enabled", true)
	viper.SetDefault("token_refresh.check_interval_minutes", 5)        // 每5分钟检查一次
	viper.SetDefault("token_refresh.refresh_before_expiry_hours", 1.5) // 提前1.5小时刷新
	viper.SetDefault("token_refresh.max_retries", 3)                   // 最多重试3次
	viper.SetDefault("token_refresh.retry_backoff_seconds", 2)         // 重试退避基础2秒
}

func (c *Config) Validate() error {
	if c.JWT.Secret == "" {
		return fmt.Errorf("jwt.secret is required")
	}
	if c.Server.Mode == "release" && isDefaultSecret(c.JWT.Secret) {
		return fmt.Errorf("jwt.secret must be changed in production")
	}
	if c.Server.Mode == "release" {
		if c.Default.AdminPassword == "" || isWeakPassword(c.Default.AdminPassword) {
			return fmt.Errorf("default.admin_password must be changed in production")
		}
	}
	if c.Gateway.UpstreamTimeout < 0 {
		return fmt.Errorf("gateway.upstream_timeout must be >= 0")
	}
	if err := validateAuthCookieConfig(c.Security); err != nil {
		return err
	}
	return nil
}

// validateAuthCookieConfig 校验 Cookie 策略配置合法性。
func validateAuthCookieConfig(cfg SecurityConfig) error {
	sameSite := strings.ToLower(strings.TrimSpace(cfg.AuthCookieSameSite))
	if sameSite != "" {
		switch sameSite {
		case "lax", "strict", "none":
		default:
			return fmt.Errorf("security.auth_cookie_same_site must be lax/strict/none")
		}
	}

	secure := strings.ToLower(strings.TrimSpace(cfg.AuthCookieSecure))
	if secure != "" {
		switch secure {
		case "auto", "true", "false":
		default:
			return fmt.Errorf("security.auth_cookie_secure must be auto/true/false")
		}
	}

	if sameSite == "none" && secure == "false" {
		return fmt.Errorf("security.auth_cookie_secure cannot be false when auth_cookie_same_site is none")
	}

	return nil
}

func parseCommaList(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	items := make([]string, 0, len(parts))
	for _, part := range parts {
		item := strings.TrimSpace(part)
		if item != "" {
			items = append(items, item)
		}
	}
	return items
}

func isDefaultSecret(secret string) bool {
	knownDefaults := []string{
		"change-me-in-production",
		"your-secret-key-change-in-production",
		"changeme",
		"change-me",
	}
	for _, value := range knownDefaults {
		if secret == value {
			return true
		}
	}
	return false
}

func isWeakPassword(password string) bool {
	knownDefaults := []string{
		"admin123",
		"admin",
		"password",
		"changeme",
		"change-me",
	}
	if len(password) < 8 {
		return true
	}
	for _, value := range knownDefaults {
		if password == value {
			return true
		}
	}
	return false
}

// GetServerAddress returns the server address (host:port) from config file or environment variable.
// This is a lightweight function that can be used before full config validation,
// such as during setup wizard startup.
// Priority: config.yaml > environment variables > defaults
func GetServerAddress() string {
	v := viper.New()
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(".")
	v.AddConfigPath("./config")
	v.AddConfigPath("/etc/sub2api")

	// Support SERVER_HOST and SERVER_PORT environment variables
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.SetDefault("server.host", "0.0.0.0")
	v.SetDefault("server.port", 8080)

	// Try to read config file (ignore errors if not found)
	_ = v.ReadInConfig()

	host := v.GetString("server.host")
	port := v.GetInt("server.port")
	return fmt.Sprintf("%s:%d", host, port)
}

// GetSetupToken 获取安装向导的访问令牌（优先环境变量，其次配置文件）。
func GetSetupToken() string {
	v := viper.New()
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(".")
	v.AddConfigPath("./config")
	v.AddConfigPath("/etc/sub2api")

	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.SetDefault("setup.token", "")

	_ = v.ReadInConfig()

	return strings.TrimSpace(v.GetString("setup.token"))
}
