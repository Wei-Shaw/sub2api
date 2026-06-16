package plugin

import "time"

// Config 是插件子系统的运行时配置。
// 现阶段在本地定义,后续会迁移到 internal/config 包中。
type Config struct {
	// BuiltinDir 镜像内置（官方）插件目录（默认 /app/plugins）。
	// 该目录中的插件被视为"官方插件"，首次启动会自动启用（受 AutoEnableBuiltin 控制）。
	BuiltinDir string

	// PluginsDir 用户安装目录（通常是 {data_dir}/plugins）。
	// 与 BuiltinDir 同名时 BuiltinDir 优先。
	PluginsDir string

	// AutoEnableBuiltin 控制 BuiltinDir 中发现的插件是否在首次启动时自动启用。
	// 已被用户显式禁用的插件不会被再次自动启用（持久化到数据库）。
	AutoEnableBuiltin bool

	// SDKListenAddr 核心 SDK gRPC 服务器监听地址,例如 "127.0.0.1:50051" 或 "127.0.0.1:0"
	SDKListenAddr string

	// HandshakeTimeout 等待插件输出 handshake JSON 的最长时间。
	HandshakeTimeout time.Duration

	// GRPCDialTimeout 拨号到插件 gRPC 服务的最长时间。
	GRPCDialTimeout time.Duration

	// ManifestTimeout 调用 GetManifest 的最长时间。
	ManifestTimeout time.Duration

	// ShutdownTimeout 调用 Shutdown RPC 的最长时间。
	ShutdownTimeout time.Duration

	// ProcessExitTimeout 等待插件进程退出的最长时间。超时后强制 SIGKILL。
	ProcessExitTimeout time.Duration

	// HealthInterval 健康检查间隔。
	HealthInterval time.Duration

	// HealthFailThreshold 触发自动重启的连续健康检查失败次数。
	HealthFailThreshold int

	// Restart 重启策略配置。
	Restart RestartConfig

	// SecretEncryptionMasterKeyHex 是 V5 W5 SecretEncryption 服务的主密钥
	// （hex 编码，必须解码后正好 32 字节）。复用 cfg.Totp.EncryptionKey 即可，
	// host 在派生 plugin 子密钥时保留主密钥不出进程。空串 = 禁用加密服务，
	// 调用插件会得到 Unimplemented。
	SecretEncryptionMasterKeyHex string

	// AllowNonLoopbackPluginAddr 允许 plugin handshake 上报非 loopback 地址。
	// 默认 false（强制 loopback,生产唯一安全配置）— 防止恶意/被替换的 plugin
	// binary 把 gRPC/HTTP 监听绑定到 0.0.0.0 让同机其他容器/进程接管 plugin
	// 通道。仅供 e2e 测试中 host 与 plugin 跨容器联调时设为 true。
	AllowNonLoopbackPluginAddr bool
}

// DefaultConfig 返回一份合理的默认配置。
func DefaultConfig() Config {
	return Config{
		PluginsDir:          "",
		SDKListenAddr:       "127.0.0.1:0",
		HandshakeTimeout:    10 * time.Second,
		GRPCDialTimeout:     5 * time.Second,
		ManifestTimeout:     5 * time.Second,
		ShutdownTimeout:     5 * time.Second,
		ProcessExitTimeout:  3 * time.Second,
		HealthInterval:      15 * time.Second,
		HealthFailThreshold: 3,
		Restart:             DefaultRestartConfig(),
	}
}

// withDefaults 用 DefaultConfig 的值填充 c 中缺失的字段。
func (c Config) withDefaults() Config {
	def := DefaultConfig()
	if c.SDKListenAddr == "" {
		c.SDKListenAddr = def.SDKListenAddr
	}
	if c.HandshakeTimeout <= 0 {
		c.HandshakeTimeout = def.HandshakeTimeout
	}
	if c.GRPCDialTimeout <= 0 {
		c.GRPCDialTimeout = def.GRPCDialTimeout
	}
	if c.ManifestTimeout <= 0 {
		c.ManifestTimeout = def.ManifestTimeout
	}
	if c.ShutdownTimeout <= 0 {
		c.ShutdownTimeout = def.ShutdownTimeout
	}
	if c.ProcessExitTimeout <= 0 {
		c.ProcessExitTimeout = def.ProcessExitTimeout
	}
	if c.HealthInterval <= 0 {
		c.HealthInterval = def.HealthInterval
	}
	if c.HealthFailThreshold <= 0 {
		c.HealthFailThreshold = def.HealthFailThreshold
	}
	return c
}
