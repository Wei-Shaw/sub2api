package rpc

import "github.com/google/wire"

// ProviderSet 内部 API RPC 服务的 Wire provider 集合。
var ProviderSet = wire.NewSet(
	NewInnerAPIRPCServer,
)
