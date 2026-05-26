package config

import "github.com/google/wire"

// ProviderSet 提供配置层的依赖
var ProviderSet = wire.NewSet(
	ProvideConfig,
)

// ProvideConfig 提供应用配置
func ProvideConfig() (*Config, error) {
	return LoadForBootstrap()
}
