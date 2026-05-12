package main

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"time"

	pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
	"google.golang.org/grpc"
)

// forwardBedrock handles forwarding for Bedrock accounts. It supports both
// SigV4 (aws_access_key_id + aws_secret_access_key) and API Key auth modes.
//
// The flow mirrors the core's forwardBedrock:
//  1. Resolve model to Bedrock model ID (with region prefix adjustment)
//  2. Prepare request body (inject anthropic_version/beta, remove model/stream)
//  3. Build HTTP request with auth (SigV4 signing or Bearer token)
//  4. Execute and stream response (EventStream binary -> SSE conversion)
func (s *gatewayProviderServer) forwardBedrock(
	req *pb.GatewayForwardRequest,
	stream grpc.ServerStreamingServer[pb.GatewayForwardChunk],
) error {
	startTime := time.Now()
	ctx := stream.Context()
	acct := req.GetAccount()

	creds := parseBedrockCredentialsRaw(acct.GetCredentialsJson())

	// Resolve model.
	originalModel := req.GetModel()
	mappedModel, ok := resolveBedrockModel(originalModel, acct.GetCredentialsJson())
	if !ok {
		return fmt.Errorf("unsupported bedrock model: %s", originalModel)
	}

	isStream := req.GetStream()

	// Prepare request body for Bedrock.
	betaHeader := getClientBetaHeader(req)
	bedrockBody, err := prepareBedrockRequestBody(req.GetRawBody(), betaHeader)
	if err != nil {
		return fmt.Errorf("prepare bedrock body: %w", err)
	}

	// Build the upstream HTTP request.
	targetURL := buildBedrockURL(creds.region, mappedModel, isStream)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(bedrockBody))
	if err != nil {
		return fmt.Errorf("build bedrock request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")

	// Apply authentication.
	if creds.isAPIKey() {
		httpReq.Header.Set("Authorization", "Bearer "+creds.apiKey)
	} else {
		if creds.accessKeyID == "" || creds.secretAccessKey == "" {
			return fmt.Errorf("bedrock SigV4 requires aws_access_key_id and aws_secret_access_key")
		}
		if err := signBedrockRequest(ctx, httpReq, bedrockBody, creds); err != nil {
			return fmt.Errorf("sign bedrock request: %w", err)
		}
	}

	// Execute the upstream request.
	resp, err := s.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("bedrock request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Map Bedrock response headers.
	respHeaders := make(map[string]string)
	for key, values := range resp.Header {
		if len(values) > 0 {
			respHeaders[key] = values[0]
		}
	}
	mapBedrockResponseHeaders(respHeaders)

	// Send response headers to the host.
	if err := stream.Send(&pb.GatewayForwardChunk{
		Chunk: &pb.GatewayForwardChunk_Headers{
			Headers: &pb.GatewayResponseHeaders{
				StatusCode: int32(resp.StatusCode),
				Headers:    respHeaders,
			},
		},
	}); err != nil {
		return err
	}

	// Handle error responses.
	if resp.StatusCode >= 400 {
		return s.handleBedrockErrorResponse(stream, resp, respHeaders, originalModel, mappedModel, isStream, startTime)
	}

	// Process the response body.
	var result *streamResult
	if isStream {
		result, err = processBedrockStream(stream, resp.Body, startTime, originalModel, mappedModel)
	} else {
		result, err = processBedrockNonStreamResponse(stream, resp.Body, originalModel, mappedModel)
	}

	// Build and send the Done chunk.
	reqID := bedrockRequestID(respHeaders)
	done := buildBedrockDoneChunk(reqID, originalModel, mappedModel, isStream, startTime, result, err)
	if sendErr := stream.Send(done); sendErr != nil {
		return sendErr
	}

	return nil
}

// handleBedrockErrorResponse processes Bedrock 4xx/5xx error responses.
func (s *gatewayProviderServer) handleBedrockErrorResponse(
	stream grpc.ServerStreamingServer[pb.GatewayForwardChunk],
	resp *http.Response,
	headers map[string]string,
	originalModel, mappedModel string,
	isStream bool,
	startTime time.Time,
) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodySize))

	if len(body) > 0 {
		if err := sendBodyChunk(stream, body); err != nil {
			return err
		}
	}

	reqID := bedrockRequestID(headers)
	fwdResult := &pb.GatewayForwardResult{
		RequestId:     reqID,
		Model:         originalModel,
		UpstreamModel: mappedModel,
		Stream:        isStream,
		DurationMs:    time.Since(startTime).Milliseconds(),
	}

	if shouldFailoverStatus(resp.StatusCode) {
		fwdResult.UpstreamError = &pb.GatewayUpstreamError{
			StatusCode:   int32(resp.StatusCode),
			ErrorType:    classifyErrorType(resp.StatusCode),
			ResponseBody: truncateBytes(body, maxErrorBodyForProto),
		}
	}

	done := &pb.GatewayForwardChunk{
		Chunk: &pb.GatewayForwardChunk_Done{
			Done: &pb.GatewayResponseDone{
				Error: fmt.Sprintf("bedrock upstream returned %d", resp.StatusCode),
				Result: fwdResult,
			},
		},
	}
	return stream.Send(done)
}

// buildBedrockDoneChunk constructs the terminal Done chunk for Bedrock responses.
func buildBedrockDoneChunk(
	requestID string,
	originalModel, mappedModel string,
	isStream bool,
	startTime time.Time,
	result *streamResult,
	streamErr error,
) *pb.GatewayForwardChunk {
	done := &pb.GatewayResponseDone{
		Result: &pb.GatewayForwardResult{
			RequestId:     requestID,
			Model:         originalModel,
			UpstreamModel: mappedModel,
			Stream:        isStream,
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