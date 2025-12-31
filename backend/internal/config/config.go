package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
)

const (
	RunModeStandard = "standard"
	RunModeSimple   = "simple"
)

// 连接池隔离策略常量
// 用于控制上游 HTTP 连接池的隔离粒度，影响连接复用和资源消耗
const (
	// ConnectionPoolIsolationProxy: 按代理隔离
	// 同一代理地址共享连接池，适合代理数量少、账户数量多的场景
	ConnectionPoolIsolationProxy = "proxy"
	// ConnectionPoolIsolationAccount: 按账户隔离
	// 每个账户独立连接池，适合账户数量少、需要严格隔离的场景
	ConnectionPoolIsolationAccount = "account"
	// ConnectionPoolIsolationAccountProxy: 按账户+代理组合隔离（默认）
	// 同一账户+代理组合共享连接池，提供最细粒度的隔离
	ConnectionPoolIsolationAccountProxy = "account_proxy"
)

type Config struct {
	Server       ServerConfig       `mapstructure:"server"`
	Database     DatabaseConfig     `mapstructure:"database"`
	Redis        RedisConfig        `mapstructure:"redis"`
	JWT          JWTConfig          `mapstructure:"jwt"`
	Default      DefaultConfig      `mapstructure:"default"`
	RateLimit    RateLimitConfig    `mapstructure:"rate_limit"`
	Pricing      PricingConfig      `mapstructure:"pricing"`
	Gateway      GatewayConfig      `mapstructure:"gateway"`
	TokenRefresh TokenRefreshConfig `mapstructure:"token_refresh"`
	RunMode      string             `mapstructure:"run_mode" yaml:"run_mode"`
	Timezone     string             `mapstructure:"timezone"` // e.g. "Asia/Shanghai", "UTC"
	Gemini       GeminiConfig       `mapstructure:"gemini"`
REDACTED

type GeminiConfig struct {
	OAuth GeminiOAuthConfig `mapstructure:"oauth"`
REDACTED

type GeminiOAuthConfig struct {
	ClientID     string `mapstructure:"client_id"`
	ClientSecret string `mapstructure:"client_secret"`
	Scopes       string `mapstructure:"scopes"`
REDACTED

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
REDACTED

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
REDACTED

type ServerConfig struct {
	Host              string `mapstructure:"host"`
	Port              int    `mapstructure:"port"`
	Mode              string `mapstructure:"mode"`                // debug/release
	ReadHeaderTimeout int    `mapstructure:"read_header_timeout"` // 读取请求头超时（秒）
	IdleTimeout       int    `mapstructure:"idle_timeout"`        // 空闲连接超时（秒）
REDACTED

// GatewayConfig API网关相关配置
type GatewayConfig struct {
	// 等待上游响应头的超时时间（秒），0表示无超时
	// 注意：这不影响流式数据传输，只控制等待响应头的时间
	ResponseHeaderTimeout int `mapstructure:"response_header_timeout"`
	// 请求体最大字节数，用于网关请求体大小限制
	MaxBodySize int64 `mapstructure:"max_body_size"`
	// ConnectionPoolIsolation: 上游连接池隔离策略（proxy/account/account_proxy）
	ConnectionPoolIsolation string `mapstructure:"connection_pool_isolation"`

	// HTTP 上游连接池配置（性能优化：支持高并发场景调优）
	// MaxIdleConns: 所有主机的最大空闲连接总数
	MaxIdleConns int `mapstructure:"max_idle_conns"`
	// MaxIdleConnsPerHost: 每个主机的最大空闲连接数（关键参数，影响连接复用率）
	MaxIdleConnsPerHost int `mapstructure:"max_idle_conns_per_host"`
	// MaxConnsPerHost: 每个主机的最大连接数（包括活跃+空闲），0表示无限制
	MaxConnsPerHost int `mapstructure:"max_conns_per_host"`
	// IdleConnTimeoutSeconds: 空闲连接超时时间（秒）
	IdleConnTimeoutSeconds int `mapstructure:"idle_conn_timeout_seconds"`
	// MaxUpstreamClients: 上游连接池客户端最大缓存数量
	// 当使用连接池隔离策略时，系统会为不同的账户/代理组合创建独立的 HTTP 客户端
	// 此参数限制缓存的客户端数量，超出后会淘汰最久未使用的客户端
	// 建议值：预估的活跃账户数 * 1.2（留有余量）
	MaxUpstreamClients int `mapstructure:"max_upstream_clients"`
	// ClientIdleTTLSeconds: 上游连接池客户端空闲回收阈值（秒）
	// 超过此时间未使用的客户端会被标记为可回收
	// 建议值：根据用户访问频率设置，一般 10-30 分钟
	ClientIdleTTLSeconds int `mapstructure:"client_idle_ttl_seconds"`
	// ConcurrencySlotTTLMinutes: 并发槽位过期时间（分钟）
	// 应大于最长 LLM 请求时间，防止请求完成前槽位过期
	ConcurrencySlotTTLMinutes int `mapstructure:"concurrency_slot_ttl_minutes"`

	// 是否记录上游错误响应体摘要（避免输出请求内容）
	LogUpstreamErrorBody bool `mapstructure:"log_upstream_error_body"`
	// 上游错误响应体记录最大字节数（超过会截断）
	LogUpstreamErrorBodyMaxBytes int `mapstructure:"log_upstream_error_body_max_bytes"`

	// API-key 账号在客户端未提供 anthropic-beta 时，是否按需自动补齐（默认关闭以保持兼容）
	InjectBetaForApiKey bool `mapstructure:"inject_beta_for_apikey"`

	// 是否允许对部分 400 错误触发 failover（默认关闭以避免改变语义）
	FailoverOn400 bool `mapstructure:"failover_on_400"`

	// Scheduling: 账号调度相关配置
	Scheduling GatewaySchedulingConfig `mapstructure:"scheduling"`
REDACTED

// GatewaySchedulingConfig accounts scheduling configuration.
type GatewaySchedulingConfig struct {
	// 粘性会话排队配置
	StickySessionMaxWaiting  int           `mapstructure:"sticky_session_max_waiting"`
	StickySessionWaitTimeout time.Duration `mapstructure:"sticky_session_wait_timeout"`

	// 兜底排队配置
	FallbackWaitTimeout time.Duration `mapstructure:"fallback_wait_timeout"`
	FallbackMaxWaiting  int           `mapstructure:"fallback_max_waiting"`

	// 负载计算
	LoadBatchEnabled bool `mapstructure:"load_batch_enabled"`

	// 过期槽位清理周期（0 表示禁用）
	SlotCleanupInterval time.Duration `mapstructure:"slot_cleanup_interval"`
REDACTED

func (s *ServerConfig) Address() string {
	return fmt.Sprintf("%s:%d", s.Host, s.Port)
REDACTED

// DatabaseConfig 数据库连接配置
// 性能优化：新增连接池参数，避免频繁创建/销毁连接
type DatabaseConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
	DBName   string `mapstructure:"dbname"`
	SSLMode  string `mapstructure:"sslmode"`
	// 连接池配置（性能优化：可配置化连接池参数）
	// MaxOpenConns: 最大打开连接数，控制数据库连接上限，防止资源耗尽
	MaxOpenConns int `mapstructure:"max_open_conns"`
	// MaxIdleConns: 最大空闲连接数，保持热连接减少建连延迟
	MaxIdleConns int `mapstructure:"max_idle_conns"`
	// ConnMaxLifetimeMinutes: 连接最大存活时间，防止长连接导致的资源泄漏
	ConnMaxLifetimeMinutes int `mapstructure:"conn_max_lifetime_minutes"`
	// ConnMaxIdleTimeMinutes: 空闲连接最大存活时间，及时释放不活跃连接
	ConnMaxIdleTimeMinutes int `mapstructure:"conn_max_idle_time_minutes"`
REDACTED

func (d *DatabaseConfig) DSN() string {
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		d.Host, d.Port, d.User, d.Password, d.DBName, d.SSLMode,
	)
REDACTED

// DSNWithTimezone returns DSN with timezone setting
func (d *DatabaseConfig) DSNWithTimezone(tz string) string {
	if tz == "" {
		tz = "Asia/Shanghai"
REDACTED
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s TimeZone=%s",
		d.Host, d.Port, d.User, d.Password, d.DBName, d.SSLMode, tz,
	)
REDACTED

// RedisConfig Redis 连接配置
// 性能优化：新增连接池和超时参数，提升高并发场景下的吞吐量
type RedisConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
	// 连接池与超时配置（性能优化：可配置化连接池参数）
	// DialTimeoutSeconds: 建立连接超时，防止慢连接阻塞
	DialTimeoutSeconds int `mapstructure:"dial_timeout_seconds"`
	// ReadTimeoutSeconds: 读取超时，避免慢查询阻塞连接池
	ReadTimeoutSeconds int `mapstructure:"read_timeout_seconds"`
	// WriteTimeoutSeconds: 写入超时，避免慢写入阻塞连接池
	WriteTimeoutSeconds int `mapstructure:"write_timeout_seconds"`
	// PoolSize: 连接池大小，控制最大并发连接数
	PoolSize int `mapstructure:"pool_size"`
	// MinIdleConns: 最小空闲连接数，保持热连接减少冷启动延迟
	MinIdleConns int `mapstructure:"min_idle_conns"`
REDACTED

func (r *RedisConfig) Address() string {
	return fmt.Sprintf("%s:%d", r.Host, r.Port)
REDACTED

type JWTConfig struct {
	Secret     string `mapstructure:"secret"`
	ExpireHour int    `mapstructure:"expire_hour"`
REDACTED

type DefaultConfig struct {
	AdminEmail      string  `mapstructure:"admin_email"`
	AdminPassword   string  `mapstructure:"admin_password"`
	UserConcurrency int     `mapstructure:"user_concurrency"`
	UserBalance     float64 `mapstructure:"user_balance"`
	ApiKeyPrefix    string  `mapstructure:"api_key_prefix"`
	RateMultiplier  float64 `mapstructure:"rate_multiplier"`
REDACTED

type RateLimitConfig struct {
	OverloadCooldownMinutes int `mapstructure:"overload_cooldown_minutes"` // 529过载冷却时间(分钟)
REDACTED

func NormalizeRunMode(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch normalized {
	case RunModeStandard, RunModeSimple:
		return normalized
	default:
		return RunModeStandard
REDACTED
REDACTED

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
	REDACTED
		// 配置文件不存在时使用默认值
REDACTED

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config error: %w", err)
REDACTED

	cfg.RunMode = NormalizeRunMode(cfg.RunMode)

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validate config error: %w", err)
REDACTED

	return &cfg, nil
REDACTED

func setDefaults() {
	viper.SetDefault("run_mode", RunModeStandard)

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
	viper.SetDefault("database.max_open_conns", 50)
	viper.SetDefault("database.max_idle_conns", 10)
	viper.SetDefault("database.conn_max_lifetime_minutes", 30)
	viper.SetDefault("database.conn_max_idle_time_minutes", 5)

	// Redis
	viper.SetDefault("redis.host", "localhost")
	viper.SetDefault("redis.port", 6379)
	viper.SetDefault("redis.password", "")
	viper.SetDefault("redis.db", 0)
	viper.SetDefault("redis.dial_timeout_seconds", 5)
	viper.SetDefault("redis.read_timeout_seconds", 3)
	viper.SetDefault("redis.write_timeout_seconds", 3)
	viper.SetDefault("redis.pool_size", 128)
	viper.SetDefault("redis.min_idle_conns", 10)

	// JWT
	viper.SetDefault("jwt.secret", "change-me-in-production")
	viper.SetDefault("jwt.expire_hour", 24)

	// Default
	// Admin credentials are created via the setup flow (web wizard / CLI / AUTO_SETUP).
	// Do not ship fixed defaults here to avoid insecure "known credentials" in production.
	viper.SetDefault("default.admin_email", "")
	viper.SetDefault("default.admin_password", "")
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
	viper.SetDefault("gateway.log_upstream_error_body", false)
	viper.SetDefault("gateway.log_upstream_error_body_max_bytes", 2048)
	viper.SetDefault("gateway.inject_beta_for_apikey", false)
	viper.SetDefault("gateway.failover_on_400", false)
	viper.SetDefault("gateway.max_body_size", int64(100*1024*1024))
	viper.SetDefault("gateway.connection_pool_isolation", ConnectionPoolIsolationAccountProxy)
	// HTTP 上游连接池配置（针对 5000+ 并发用户优化）
	viper.SetDefault("gateway.max_idle_conns", 240)            // 最大空闲连接总数（HTTP/2 场景默认）
	viper.SetDefault("gateway.max_idle_conns_per_host", 120)   // 每主机最大空闲连接（HTTP/2 场景默认）
	viper.SetDefault("gateway.max_conns_per_host", 240)        // 每主机最大连接数（含活跃，HTTP/2 场景默认）
	viper.SetDefault("gateway.idle_conn_timeout_seconds", 300) // 空闲连接超时（秒）
	viper.SetDefault("gateway.max_upstream_clients", 5000)
	viper.SetDefault("gateway.client_idle_ttl_seconds", 900)
	viper.SetDefault("gateway.concurrency_slot_ttl_minutes", 15) // 并发槽位过期时间（支持超长请求）
	viper.SetDefault("gateway.scheduling.sticky_session_max_waiting", 3)
	viper.SetDefault("gateway.scheduling.sticky_session_wait_timeout", 45*time.Second)
	viper.SetDefault("gateway.scheduling.fallback_wait_timeout", 30*time.Second)
	viper.SetDefault("gateway.scheduling.fallback_max_waiting", 100)
	viper.SetDefault("gateway.scheduling.load_batch_enabled", true)
	viper.SetDefault("gateway.scheduling.slot_cleanup_interval", 30*time.Second)

	// TokenRefresh
	viper.SetDefault("token_refresh.enabled", true)
	viper.SetDefault("token_refresh.check_interval_minutes", 5)        // 每5分钟检查一次
	viper.SetDefault("token_refresh.refresh_before_expiry_hours", 0.5) // 提前30分钟刷新（适配Google 1小时token）
	viper.SetDefault("token_refresh.max_retries", 3)                   // 最多重试3次
	viper.SetDefault("token_refresh.retry_backoff_seconds", 2)         // 重试退避基础2秒

	// Gemini OAuth - configure via environment variables or config file
	// GEMINI_OAUTH_CLIENT_ID and GEMINI_OAUTH_CLIENT_SECRET
	// Default: uses Gemini CLI public credentials (set via environment)
	viper.SetDefault("gemini.oauth.client_id", "")
	viper.SetDefault("gemini.oauth.client_secret", "")
	viper.SetDefault("gemini.oauth.scopes", "")
REDACTED

func (c *Config) Validate() error {
	if c.JWT.Secret == "" {
		return fmt.Errorf("jwt.secret is required")
REDACTED
	if c.JWT.Secret == "change-me-in-production" && c.Server.Mode == "release" {
		return fmt.Errorf("jwt.secret must be changed in production")
REDACTED
	if c.Database.MaxOpenConns <= 0 {
		return fmt.Errorf("database.max_open_conns must be positive")
REDACTED
	if c.Database.MaxIdleConns < 0 {
		return fmt.Errorf("database.max_idle_conns must be non-negative")
REDACTED
	if c.Database.MaxIdleConns > c.Database.MaxOpenConns {
		return fmt.Errorf("database.max_idle_conns cannot exceed database.max_open_conns")
REDACTED
	if c.Database.ConnMaxLifetimeMinutes < 0 {
		return fmt.Errorf("database.conn_max_lifetime_minutes must be non-negative")
REDACTED
	if c.Database.ConnMaxIdleTimeMinutes < 0 {
		return fmt.Errorf("database.conn_max_idle_time_minutes must be non-negative")
REDACTED
	if c.Redis.DialTimeoutSeconds <= 0 {
		return fmt.Errorf("redis.dial_timeout_seconds must be positive")
REDACTED
	if c.Redis.ReadTimeoutSeconds <= 0 {
		return fmt.Errorf("redis.read_timeout_seconds must be positive")
REDACTED
	if c.Redis.WriteTimeoutSeconds <= 0 {
		return fmt.Errorf("redis.write_timeout_seconds must be positive")
REDACTED
	if c.Redis.PoolSize <= 0 {
		return fmt.Errorf("redis.pool_size must be positive")
REDACTED
	if c.Redis.MinIdleConns < 0 {
		return fmt.Errorf("redis.min_idle_conns must be non-negative")
REDACTED
	if c.Redis.MinIdleConns > c.Redis.PoolSize {
		return fmt.Errorf("redis.min_idle_conns cannot exceed redis.pool_size")
REDACTED
	if c.Gateway.MaxBodySize <= 0 {
		return fmt.Errorf("gateway.max_body_size must be positive")
REDACTED
	if strings.TrimSpace(c.Gateway.ConnectionPoolIsolation) != "" {
		switch c.Gateway.ConnectionPoolIsolation {
		case ConnectionPoolIsolationProxy, ConnectionPoolIsolationAccount, ConnectionPoolIsolationAccountProxy:
		default:
			return fmt.Errorf("gateway.connection_pool_isolation must be one of: %s/%s/%s",
				ConnectionPoolIsolationProxy, ConnectionPoolIsolationAccount, ConnectionPoolIsolationAccountProxy)
	REDACTED
REDACTED
	if c.Gateway.MaxIdleConns <= 0 {
		return fmt.Errorf("gateway.max_idle_conns must be positive")
REDACTED
	if c.Gateway.MaxIdleConnsPerHost <= 0 {
		return fmt.Errorf("gateway.max_idle_conns_per_host must be positive")
REDACTED
	if c.Gateway.MaxConnsPerHost < 0 {
		return fmt.Errorf("gateway.max_conns_per_host must be non-negative")
REDACTED
	if c.Gateway.IdleConnTimeoutSeconds <= 0 {
		return fmt.Errorf("gateway.idle_conn_timeout_seconds must be positive")
REDACTED
	if c.Gateway.MaxUpstreamClients <= 0 {
		return fmt.Errorf("gateway.max_upstream_clients must be positive")
REDACTED
	if c.Gateway.ClientIdleTTLSeconds <= 0 {
		return fmt.Errorf("gateway.client_idle_ttl_seconds must be positive")
REDACTED
	if c.Gateway.ConcurrencySlotTTLMinutes <= 0 {
		return fmt.Errorf("gateway.concurrency_slot_ttl_minutes must be positive")
REDACTED
	if c.Gateway.Scheduling.StickySessionMaxWaiting <= 0 {
		return fmt.Errorf("gateway.scheduling.sticky_session_max_waiting must be positive")
REDACTED
	if c.Gateway.Scheduling.StickySessionWaitTimeout <= 0 {
		return fmt.Errorf("gateway.scheduling.sticky_session_wait_timeout must be positive")
REDACTED
	if c.Gateway.Scheduling.FallbackWaitTimeout <= 0 {
		return fmt.Errorf("gateway.scheduling.fallback_wait_timeout must be positive")
REDACTED
	if c.Gateway.Scheduling.FallbackMaxWaiting <= 0 {
		return fmt.Errorf("gateway.scheduling.fallback_max_waiting must be positive")
REDACTED
	if c.Gateway.Scheduling.SlotCleanupInterval < 0 {
		return fmt.Errorf("gateway.scheduling.slot_cleanup_interval must be non-negative")
REDACTED
	return nil
REDACTED

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
REDACTED
