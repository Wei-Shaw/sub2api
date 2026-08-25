package service

// 内建国产供应商注册入口。
//
// 注册顺序即 CNProviderCodes() / ConcretePlatforms() 等派生列表中的平台顺序，
// 与历史硬编码列表（kimi/zhipu/deepseek）保持一致——Go 同包多文件的 init()
// 按文件名字典序执行，无法表达该顺序，故集中在单个 init() 中显式注册。
// 各平台的 spec 与探测钩子实现见 cn_provider_{kimi,zhipu,deepseek}.go。
func init() {
	RegisterCNProvider(kimiProviderSpec)
	RegisterCNProvider(zhipuProviderSpec)
	RegisterCNProvider(deepseekProviderSpec)
}
