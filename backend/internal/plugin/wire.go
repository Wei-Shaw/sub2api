package plugin

import "github.com/google/wire"

// ProviderSet 暴露给 Wire 的依赖集合。
// 注:NewPluginManager 与 NewPluginRouter 需要外部参数(db / redis / cfg / coreHandler / 鉴权中间件),
// 这些应由调用方在更高一层的 Wire 配置中注入。
var ProviderSet = wire.NewSet(
	NewPluginRepository,
	NewSDKServer,
	NewPluginManager,
)
