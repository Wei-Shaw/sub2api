package rpc

import "github.com/google/wire"

// ProviderSet 余额 RPC 服务的 Wire provider 集合。
var ProviderSet = wire.NewSet(
	NewBalanceRPCServer,
)
