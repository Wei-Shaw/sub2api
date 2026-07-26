package openai_ws_v2

import (
	"context"
)

// runCaddyStyleRelay 采用 Caddy reverseproxy 的双向隧道思想：
// 连接建立后并发复制两个方向，任一方向退出触发收敛关闭。
//
// Reference:
// - Project: caddyserver/caddy (Apache-2.0)
// - Commit: f283062d37c50627d53ca682ebae2ce219b35515
// - Files:
//   - modules/caddyhttp/reverseproxy/streaming.go
//   - modules/caddyhttp/reverseproxy/reverseproxy.go
func runCaddyStyleRelay(
	ctx context.Context,
	clientConn FrameConn,
	upstreamConn FrameConn,
	firstClientMessage []byte,
	options RelayOptions,
) (RelayResult, *RelayExit) {
	return Relay(ctx, clientConn, upstreamConn, firstClientMessage, options)
}
