package gatewayutil

import (
	"encoding/json"
	"fmt"
	"net/http"

	pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
	"google.golang.org/grpc"
)

// SendErrorEnd sends a terminal test_end event with success=false and the
// given error message. Used by AccountPlatformExtension.TestConnection
// implementations.
func SendErrorEnd(
	stream grpc.ServerStreamingServer[pb.TestConnectionEvent],
	msg string,
) error {
	_ = stream.Send(&pb.TestConnectionEvent{
		Type:    "test_end",
		Success: false,
		Error:   msg,
	})
	return nil
}

// SendBodyChunk sends a GatewayResponseBody chunk to the gRPC stream.
// Used by GatewayProviderExtension.Forward implementations to stream
// upstream response data back to the host.
func SendBodyChunk(
	stream grpc.ServerStreamingServer[pb.GatewayForwardChunk],
	data []byte,
) error {
	return stream.Send(&pb.GatewayForwardChunk{
		Chunk: &pb.GatewayForwardChunk_Body{
			Body: &pb.GatewayResponseBody{Data: data},
		},
	})
}

// SendDone sends a terminal Done chunk with an optional error message.
// Used by GatewayProviderExtension.Forward implementations to signal
// request completion.
func SendDone(
	stream grpc.ServerStreamingServer[pb.GatewayForwardChunk],
	result *pb.GatewayForwardResult,
	errMsg string,
) error {
	return stream.Send(&pb.GatewayForwardChunk{
		Chunk: &pb.GatewayForwardChunk_Done{
			Done: &pb.GatewayResponseDone{
				Result: result,
				Error:  errMsg,
			},
		},
	})
}

// HandleErrorResponse processes upstream 4xx/5xx responses. It sends
// the error body downstream and attaches a structured GatewayUpstreamError
// to the Done chunk so the host can decide failover strategy.
//
// Parameters:
//   - stream: the gRPC server stream
//   - statusCode: upstream HTTP status code
//   - body: upstream response body (already read)
//   - result: pre-populated GatewayForwardResult (model, timing, etc.)
//   - responseHeaders: upstream response headers (collected via
//     CollectResponseHeaders); included in UpstreamError for host-side
//     rate-limit and diagnostic processing
//   - extraFailoverCodes: platform-specific failover status codes
//   - extraOverloaded: platform-specific overloaded status codes
func HandleErrorResponse(
	stream grpc.ServerStreamingServer[pb.GatewayForwardChunk],
	statusCode int,
	body []byte,
	result *pb.GatewayForwardResult,
	responseHeaders map[string]string,
	extraFailoverCodes map[int]bool,
	extraOverloaded []int,
) error {
	if len(body) > 0 {
		if err := SendBodyChunk(stream, body); err != nil {
			return err
		}
	}

	if ShouldFailoverStatus(statusCode, extraFailoverCodes) {
		result.UpstreamError = &pb.GatewayUpstreamError{
			StatusCode:      int32(statusCode),
			ErrorType:       ClassifyErrorType(statusCode, extraOverloaded),
			ResponseBody:    TruncateBytes(body, MaxErrorBodyForProto),
			ResponseHeaders: responseHeaders,
		}
	}

	errMsg := fmt.Sprintf("upstream returned %d", statusCode)
	return SendDone(stream, result, errMsg)
}

// CollectResponseHeaders extracts upstream response headers that the host
// needs for rate-limit tracking, Codex usage snapshot extraction, and
// diagnostic passthrough. Returns nil when no relevant headers are present.
//
// Plugins call this once after receiving the upstream HTTP response and
// pass the result to HandleErrorResponse (error path) or set it on
// GatewayForwardResult.ResponseHeaders (success path).
func CollectResponseHeaders(resp *http.Response, extraKeys []string) map[string]string {
	if resp == nil {
		return nil
	}
	// commonKeys covers rate-limit, Codex, and diagnostic headers that
	// multiple platforms emit. Platform-specific keys come via extraKeys.
	commonKeys := []string{
		"x-request-id",
		"x-ratelimit-limit-requests",
		"x-ratelimit-remaining-requests",
		"x-ratelimit-limit-tokens",
		"x-ratelimit-remaining-tokens",
		"x-ratelimit-reset-requests",
		"x-ratelimit-reset-tokens",
		"cf-ray",
		"cf-mitigated",
		"content-type",
		"retry-after",
	}
	headers := make(map[string]string, len(commonKeys)+len(extraKeys))
	for _, key := range commonKeys {
		if v := resp.Header.Get(key); v != "" {
			headers[key] = v
		}
	}
	for _, key := range extraKeys {
		if v := resp.Header.Get(key); v != "" {
			headers[key] = v
		}
	}
	if len(headers) == 0 {
		return nil
	}
	return headers
}

// ReplaceModelInBody replaces the "model" field in a JSON request body
// with the mapped model name. Returns the original body on any error.
func ReplaceModelInBody(body []byte, newModel string) []byte {
	var parsed map[string]json.RawMessage
	if err := json.Unmarshal(body, &parsed); err != nil {
		return body
	}
	modelBytes, err := json.Marshal(newModel)
	if err != nil {
		return body
	}
	parsed["model"] = modelBytes
	result, err := json.Marshal(parsed)
	if err != nil {
		return body
	}
	return result
}
