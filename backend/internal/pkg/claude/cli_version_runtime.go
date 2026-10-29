package claude

import "sync/atomic"

// cliVersionResolver 返回运行期生效的 Claude CLI 版本号（管理员面板手动值 →
// 后台同步值）。由 SettingService 在装配时注入（wire.go），解析器内部自带
// TTL 缓存，热路径不触库。
//
// 未注入时（所有不经过 wire 的单元测试）EffectiveCLIVersion() 与 CLIVersion()
// 完全等价，行为不变。
type cliVersionResolverFunc func() string

var cliVersionResolver atomic.Pointer[cliVersionResolverFunc]

// SetCLIVersionResolver 注入运行期版本解析器；传 nil 可撤销注入。
func SetCLIVersionResolver(resolver func() string) {
	if resolver == nil {
		cliVersionResolver.Store(nil)
		return
	}
	r := cliVersionResolverFunc(resolver)
	cliVersionResolver.Store(&r)
}

// EffectiveCLIVersion 返回当前生效的 Claude CLI 版本号。
// resolver 返回值必须通过 IsSupportedCLIVersion 校验（严格三段 semver 且不低于
// 内置基线），否则回退 CLIVersion()（环境变量覆盖 → 内置基线）。
// 运行期值恒 >= 内置基线，identity_service 的"只升不降"语义不受影响。
func EffectiveCLIVersion() string {
	if r := cliVersionResolver.Load(); r != nil {
		if v := (*r)(); IsSupportedCLIVersion(v) {
			return v
		}
	}
	return CLIVersion()
}

// DefaultUserAgent 返回 Claude Code CLI 伪装 User-Agent。
// 所有出站路径都必须经本函数取 UA，保证 User-Agent 头与请求体 billing
// attribution 块中的 cc_version 始终源自同一个版本号。
func DefaultUserAgent() string {
	return "claude-cli/" + EffectiveCLIVersion() + " (external, cli)"
}
