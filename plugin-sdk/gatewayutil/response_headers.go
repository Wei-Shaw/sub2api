// Package gatewayutil — response helpers for streaming forward APIs.
package gatewayutil

import (
	"net/http"

	pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
	"google.golang.org/grpc"
)

// SendResponseHeaders sends the initial GatewayResponseHeaders chunk with
// the upstream HTTP status code and a curated subset of response headers.
//
// Only headers listed in headerWhitelist are forwarded; Content-Type is
// always propagated for downstream rendering. Sensitive headers like
// Set-Cookie / Server / X-Powered-By are intentionally dropped.
//
// Each gateway plugin owns its own header whitelist (e.g. anthropic forwards
// anthropic-* headers, openai forwards x-ratelimit-*) and passes it in.
func SendResponseHeaders(
	stream grpc.ServerStreamingServer[pb.GatewayForwardChunk],
	resp *http.Response,
	headerWhitelist []string,
) error {
	headers := make(map[string]string)
	for _, key := range headerWhitelist {
		if v := resp.Header.Get(key); v != "" {
			headers[key] = v
		}
	}
	if ct := resp.Header.Get("Content-Type"); ct != "" {
		headers["Content-Type"] = ct
	}
	return stream.Send(&pb.GatewayForwardChunk{
		Chunk: &pb.GatewayForwardChunk_Headers{
			Headers: &pb.GatewayResponseHeaders{
				StatusCode: int32(resp.StatusCode),
				Headers:    headers,
			},
		},
	})
}
