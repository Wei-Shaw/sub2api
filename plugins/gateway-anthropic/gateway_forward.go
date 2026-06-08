package main

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/Wei-Shaw/sub2api/plugin-sdk/gatewayutil"
	pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
	"google.golang.org/grpc"
)

// defaultHTTPClient is a shared HTTP client tuned for streaming upstream
// requests. See gatewayutil.NewStreamingHTTPClient for tuning rationale.
var defaultHTTPClient = gatewayutil.NewStreamingHTTPClient()

// gatewayProviderServer implements the GatewayProviderExtension gRPC service.
// It handles upstream API forwarding for the Anthropic Messages API.
type gatewayProviderServer struct {
	pb.UnimplementedGatewayProviderExtensionServer
	logger func() *slog.Logger
}

func newGatewayProviderServer(logFn func() *slog.Logger) *gatewayProviderServer {
	return &gatewayProviderServer{logger: logFn}
}

// Forward handles a single gateway request by:
//  1. Building the upstream HTTP request (auth, headers, model mapping)
//  2. Sending the request to the upstream API
//  3. Streaming the response back as GatewayForwardChunk messages
//  4. Reporting usage data in the final Done chunk
//
// Bedrock accounts are routed to forwardBedrock which handles SigV4
// signing, request body transformation, and EventStream decoding.
func (s *gatewayProviderServer) Forward(
	req *pb.GatewayForwardRequest,
	stream grpc.ServerStreamingServer[pb.GatewayForwardChunk],
) error {
	startTime := time.Now()
	ctx := stream.Context()

	s.logger().Info("forward request",
		"request_id", req.GetRequestId(),
		"model", req.GetModel(),
		"account_type", req.GetAccount().GetAccountType(),
		"stream", req.GetStream(),
	)

	// Build the upstream HTTP request. Bedrock and Vertex accounts use
	// dedicated forwarding paths with provider-specific auth and URLs.
	upstream, err := buildUpstreamRequest(ctx, req)
	if err != nil {
		if errors.Is(err, errBedrockAccount) {
			return s.forwardBedrock(req, stream)
		}
		if errors.Is(err, errVertexAccount) {
			return s.forwardVertex(req, stream)
		}
		return fmt.Errorf("build upstream request: %w", err)
	}

	// Execute the upstream request.
	resp, err := defaultHTTPClient.Do(upstream.httpReq)
	if err != nil {
		return fmt.Errorf("upstream request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Send response headers to the host.
	if err := sendResponseHeaders(stream, resp); err != nil {
		return err
	}

	isStream := req.GetStream()

	// Handle error responses (4xx/5xx) with structured upstream error.
	if resp.StatusCode >= 400 {
		return s.handleErrorResponse(stream, resp, upstream, isStream, startTime)
	}

	// Process the response body.
	var result *streamResult
	if isStream {
		result, err = processSSEStream(
			stream, resp.Body, startTime,
			upstream.originalModel, upstream.mappedModel,
		)
	} else {
		result, err = processNonStreamResponse(
			stream, resp.Body,
			upstream.originalModel, upstream.mappedModel,
		)
	}

	// Build the Done chunk with usage data.
	done := buildDoneChunk(
		resp, upstream, result, isStream, startTime, err,
	)
	if sendErr := stream.Send(done); sendErr != nil {
		return sendErr
	}

	return nil
}

// handleErrorResponse processes upstream 4xx/5xx responses. It sends
// the error body downstream and attaches a structured GatewayUpstreamError
// to the Done chunk so the host can decide failover strategy without
// parsing error message strings.
func (s *gatewayProviderServer) handleErrorResponse(
	stream grpc.ServerStreamingServer[pb.GatewayForwardChunk],
	resp *http.Response,
	upstream *upstreamRequest,
	isStream bool,
	startTime time.Time,
) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodySize))

	result := &pb.GatewayForwardResult{
		RequestId:     resp.Header.Get("x-request-id"),
		Model:         upstream.originalModel,
		UpstreamModel: upstream.mappedModel,
		Stream:        isStream,
		DurationMs:    time.Since(startTime).Milliseconds(),
	}

	return gatewayutil.HandleErrorResponse(
		stream, resp.StatusCode, body, result, gatewayutil.CollectResponseHeaders(resp, nil),
		extraFailoverCodes, extraOverloadedCodes,
	)
}

// responseHeaderWhitelist lists upstream headers forwarded to the host.
// Only safe, non-sensitive headers are included; Set-Cookie / Server /
// X-Powered-By and other infrastructure leakage headers are dropped.
var responseHeaderWhitelist = []string{
	"x-request-id",
	"retry-after",
	"anthropic-ratelimit-requests-limit",
	"anthropic-ratelimit-requests-remaining",
	"anthropic-ratelimit-requests-reset",
	"anthropic-ratelimit-tokens-limit",
	"anthropic-ratelimit-tokens-remaining",
	"anthropic-ratelimit-tokens-reset",
	"anthropic-ratelimit-input-tokens-limit",
	"anthropic-ratelimit-input-tokens-remaining",
	"anthropic-ratelimit-input-tokens-reset",
	"anthropic-ratelimit-output-tokens-limit",
	"anthropic-ratelimit-output-tokens-remaining",
	"anthropic-ratelimit-output-tokens-reset",
}

// sendResponseHeaders sends the initial GatewayResponseHeaders chunk
// containing the upstream status code and a curated set of headers.
// Only headers in responseHeaderWhitelist are forwarded; Content-Type is
// always propagated for downstream rendering.
func sendResponseHeaders(
	stream grpc.ServerStreamingServer[pb.GatewayForwardChunk],
	resp *http.Response,
) error {
	return gatewayutil.SendResponseHeaders(stream, resp, responseHeaderWhitelist)
}

// buildDoneChunk constructs the terminal GatewayForwardChunk_Done message
// with usage data from the stream processing result.
func buildDoneChunk(
	resp *http.Response,
	upstream *upstreamRequest,
	result *streamResult,
	isStream bool,
	startTime time.Time,
	streamErr error,
) *pb.GatewayForwardChunk {
	done := &pb.GatewayResponseDone{
		Result: &pb.GatewayForwardResult{
			RequestId:       resp.Header.Get("x-request-id"),
			Model:           upstream.originalModel,
			UpstreamModel:   upstream.mappedModel,
			Stream:          isStream,
			DurationMs:      time.Since(startTime).Milliseconds(),
			ResponseHeaders: gatewayutil.CollectResponseHeaders(resp, nil),
		},
	}

	if result != nil {
		done.Result.InputTokens = result.inputTokens
		done.Result.OutputTokens = result.outputTokens
		done.Result.CacheCreationTokens = result.cacheCreationTokens
		done.Result.CacheReadTokens = result.cacheReadTokens
		done.Result.ClientDisconnect = result.clientDisconnect
		if result.firstTokenMs > 0 {
			done.Result.FirstTokenMs = result.firstTokenMs
		}
	}

	if streamErr != nil {
		done.Error = streamErr.Error()
	}

	return &pb.GatewayForwardChunk{
		Chunk: &pb.GatewayForwardChunk_Done{Done: done},
	}
}
