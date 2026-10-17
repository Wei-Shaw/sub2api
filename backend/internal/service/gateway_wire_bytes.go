package service

import (
	"context"
	"sync/atomic"
)

// 线上字节账（wire bytes）的三路累计器，随请求 ctx 贯穿：
//   - in/out：网关边界，客户端请求体/回给客户端的响应体的线上字节，
//     由 ByteCounter 中间件在请求结束时写回；
//   - upstream：发往上游的请求体字节合计，由上游发送点对每一次尝试累加
//     （失败重试与 failover 各算一次——重试风暴的字节账就在这里）。
//
// usage 落库经 GatewayWireBytesFromContext 读取；未过网关的写入路径
// （batch、admin 等）ctx 里没有累计器，返回全零。

type gatewayWireBytesKey struct{}

type gatewayWireBytes struct {
	in, out, upstream atomic.Int64
}

// WithGatewayWireBytes 在请求入口挂上累计器。
func WithGatewayWireBytes(ctx context.Context) context.Context {
	return context.WithValue(ctx, gatewayWireBytesKey{}, &gatewayWireBytes{})
}

// SetClientWireBytes 由 ByteCounter 中间件写回客户端侧两路字节。
func SetClientWireBytes(ctx context.Context, in, out int64) {
	if w, ok := ctx.Value(gatewayWireBytesKey{}).(*gatewayWireBytes); ok && w != nil {
		w.in.Store(in)
		w.out.Store(out)
	}
}

// AddUpstreamOutBytes 由上游发送点对每次尝试调用，累计发往上游的请求体字节。
func AddUpstreamOutBytes(ctx context.Context, n int64) {
	if n <= 0 {
		return
	}
	if w, ok := ctx.Value(gatewayWireBytesKey{}).(*gatewayWireBytes); ok && w != nil {
		w.upstream.Add(n)
	}
}

// GatewayWireBytesFromContext 读取三路字节；无累计器时返回全零。
func GatewayWireBytesFromContext(ctx context.Context) (inBytes, outBytes, upstreamOutBytes int64) {
	if w, ok := ctx.Value(gatewayWireBytesKey{}).(*gatewayWireBytes); ok && w != nil {
		return w.in.Load(), w.out.Load(), w.upstream.Load()
	}
	return 0, 0, 0
}
