package plugin

import "time"

// RestartConfig 是 PluginConfig.Restart 的本地等价类型。
// 当顶层 config 包补齐插件配置后,可以直接替换为 config.PluginRestartConfig。
type RestartConfig struct {
	MaxRetries   int           // 最大重试次数,达到后不再重启
	InitialDelay time.Duration // 初始退避延迟,通常 1s
	MaxDelay     time.Duration // 最大退避延迟上限
	ResetAfter   time.Duration // 进程稳定运行超过该时长后重置重试计数
}

// DefaultRestartConfig 返回一组合理的默认值,避免 manager 启动时缺省配置导致 panic。
func DefaultRestartConfig() RestartConfig {
	return RestartConfig{
		MaxRetries:   5,
		InitialDelay: time.Second,
		MaxDelay:     time.Minute,
		ResetAfter:   5 * time.Minute,
	}
}

// RestartPolicy 计算插件崩溃后的指数退避延迟,并提供重置逻辑。
type RestartPolicy struct {
	cfg RestartConfig
}

// NewRestartPolicy 根据配置创建重启策略。
// 缺失的字段会用 DefaultRestartConfig 兜底,避免运行时除零或负值。
func NewRestartPolicy(cfg RestartConfig) *RestartPolicy {
	def := DefaultRestartConfig()
	if cfg.MaxRetries <= 0 {
		cfg.MaxRetries = def.MaxRetries
	}
	if cfg.InitialDelay <= 0 {
		cfg.InitialDelay = def.InitialDelay
	}
	if cfg.MaxDelay <= 0 {
		cfg.MaxDelay = def.MaxDelay
	}
	if cfg.ResetAfter <= 0 {
		cfg.ResetAfter = def.ResetAfter
	}
	return &RestartPolicy{cfg: cfg}
}

// NextDelay 根据已重启次数返回下次重启前的等待时间。
// 第二个返回值 false 表示已达到 MaxRetries,不应再重启。
// restartCount 从 0 开始计数(0 = 第一次重启)。
func (p *RestartPolicy) NextDelay(restartCount int) (time.Duration, bool) {
	if restartCount < 0 {
		restartCount = 0
	}
	if restartCount >= p.cfg.MaxRetries {
		return 0, false
	}

	// 指数退避: initial * 2^restartCount, 受 MaxDelay 限制。
	// 用 int64 计算避免左移溢出,超过 62 直接饱和到 MaxDelay。
	if restartCount >= 62 {
		return p.cfg.MaxDelay, true
	}
	delay := p.cfg.InitialDelay << uint(restartCount)
	if delay <= 0 || delay > p.cfg.MaxDelay {
		delay = p.cfg.MaxDelay
	}
	return delay, true
}

// ShouldReset 判断进程稳定运行的时长是否达到重置阈值。
// 用于在插件长时间健康运行后清零重试计数。
func (p *RestartPolicy) ShouldReset(runDuration time.Duration) bool {
	return runDuration >= p.cfg.ResetAfter
}

// MaxRetries 暴露最大重试次数,便于上层日志输出。
func (p *RestartPolicy) MaxRetries() int {
	return p.cfg.MaxRetries
}
