package plugin

import "time"

// Config 是插件子系统的运行时配置。
// 现阶段在本地定义,后续会迁移到 internal/config 包中。
type Config struct {
	// PluginsDir 插件二进制扫描目录,通常是 {data_dir}/plugins
	PluginsDir string

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
