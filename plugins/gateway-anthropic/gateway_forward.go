package main

import (
	"fmt"
	"io"
	"net/http"
	"time"

	pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
	"google.golang.org/grpc"
)

// gatewayProviderServer implements the GatewayProviderExtension gRPC service.
// It handles upstream API forwarding for the Anthropic Messages API.
type gatewayProviderServer struct {
	pb.UnimplementedGatewayProviderExtensionServer
	httpClient *http.Client
}

func newGatewayProviderServer() *gatewayProviderServer {
	return &gatewayProviderServer{
		httpClient: &http.Client{
			// No global timeout; streaming responses can be long-lived.
			// Per-request timeouts are handled by context cancellation.
		},
	}
}

// Forward handles a single gateway request by:
//  1. Building the upstream HTTP request (auth, headers, model mapping)
//  2. Sending the request to the Anthropic Messages API
//  3. Streaming the response back as GatewayForwardChunk messages
//  4. Reporting usage data in the final Done chunk
//
// Bedrock accounts are not yet supported; they return an error that
// triggers failover to the host's built-in implementation.
func (s *gatewayProviderServer) Forward(
	req *pb.GatewayForwardRequest,
	stream grpc.ServerStreamingServer[pb.GatewayForwardChunk],
) error {
	startTime := time.Now()
	ctx := stream.Context()

	// Build the upstream HTTP request.
	upstream, err := buildUpstreamRequest(ctx, req)
	if err != nil {
		return fmt.Errorf("build upstream request: %w", err)
	}

	// Execute the upstream request.
	resp, err := s.httpClient.Do(upstream.httpReq)
	if err != nil {
		return fmt.Errorf("upstream request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Send response headers to the host.
	if err := sendResponseHeaders(stream, resp); err != nil {
		return err
	}

	// Handle error responses (4xx/5xx).
	if resp.StatusCode >= 400 {
		return s.handleErrorResponse(stream, resp, upstream, startTime)
	}

	// Process the response body.
	var result *streamResult
	if req.GetStream() {
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
		resp, upstream, result, startTime, err,
	)
	if sendErr := stream.Send(done); sendErr != nil {
		return sendErr
	}

	return nil
}

// handleErrorResponse processes upstream 4xx/5xx responses. For failover-
// eligible status codes, it reads the body and returns it so the host
// can attempt another account. For other errors, it sends the error
// body downstream.
func (s *gatewayProviderServer) handleErrorResponse(
	stream grpc.ServerStreamingServer[pb.GatewayForwardChunk],
	resp *http.Response,
	upstream *upstreamRequest,
	startTime time.Time,
) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodySize))

	// Send the error body to the host so it can forward to the client
	// if it decides not to failover.
	if len(body) > 0 {
		if err := sendBodyChunk(stream, body); err != nil {
			return err
		}
	}

	errMsg := fmt.Sprintf("upstream returned %d", resp.StatusCode)
	done := &pb.GatewayForwardChunk{
		Chunk: &pb.GatewayForwardChunk_Done{
			Done: &pb.GatewayResponseDone{
				Error: errMsg,
				Result: &pb.GatewayForwardResult{
					RequestId:     resp.Header.Get("x-request-id"),
					Model:         upstream.originalModel,
					UpstreamModel: upstream.mappedModel,
					DurationMs:    time.Since(startTime).Milliseconds(),
				},
			},
		},
	}
	if sendErr := stream.Send(done); sendErr != nil {
		return sendErr
	}

	// Signal failover for eligible status codes.
	if shouldFailoverStatus(resp.StatusCode) {
		return fmt.Errorf("UpstreamFailoverError: status %d", resp.StatusCode)
	}

	return nil
}

// sendResponseHeaders sends the initial GatewayResponseHeaders chunk
// containing the upstream status code and headers.
func sendResponseHeaders(
	stream grpc.ServerStreamingServer[pb.GatewayForwardChunk],
	resp *http.Response,
) error {
	headers := make(map[string]string)
	for key, values := range resp.Header {
		if len(values) > 0 {
			headers[key] = values[0]
		}
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

// buildDoneChunk constructs the terminal GatewayForwardChunk_Done message
// with usage data from the stream processing result.
func buildDoneChunk(
	resp *http.Response,
	upstream *upstreamRequest,
	result *streamResult,
	startTime time.Time,
	streamErr error,
) *pb.GatewayForwardChunk {
	done := &pb.GatewayResponseDone{
		Result: &pb.GatewayForwardResult{
			RequestId:     resp.Header.Get("x-request-id"),
			Model:         upstream.originalModel,
			UpstreamModel: upstream.mappedModel,
			Stream:        true,
			DurationMs:    time.Since(startTime).Milliseconds(),
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
