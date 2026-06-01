package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/Wei-Shaw/sub2api/plugin-sdk/gatewayutil"
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

	originalModel := req.GetModel()
	mappedModel, ok := resolveBedrockModel(originalModel, acct.GetCredentialsJson())
	if !ok {
		return fmt.Errorf("unsupported bedrock model: %s", originalModel)
	}

	isStream := req.GetStream()
	httpReq, bedrockBody, err := buildBedrockHTTPRequest(ctx, req, creds, mappedModel, isStream)
	if err != nil {
		return err
	}
	if err := authenticateBedrockRequest(ctx, httpReq, bedrockBody, creds); err != nil {
		return err
	}

	resp, err := s.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("bedrock request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respHeaders := collectSingleValueHeaders(resp)
	mapBedrockResponseHeaders(respHeaders)
	if err := sendBedrockResponseHeaders(stream, resp.StatusCode, respHeaders); err != nil {
		return err
	}

	if resp.StatusCode >= 400 {
		return handleBedrockError(stream, resp, respHeaders, originalModel, mappedModel, isStream, startTime)
	}

	return sendBedrockSuccess(stream, resp, respHeaders, originalModel, mappedModel, isStream, startTime)
}

// buildBedrockHTTPRequest constructs the upstream HTTP request for Bedrock.
func buildBedrockHTTPRequest(
	ctx context.Context,
	req *pb.GatewayForwardRequest,
	creds *bedrockCreds,
	mappedModel string,
	isStream bool,
) (*http.Request, []byte, error) {
	betaHeader := getClientBetaHeader(req)
	bedrockBody, err := prepareBedrockRequestBody(req.GetRawBody(), betaHeader)
	if err != nil {
		return nil, nil, fmt.Errorf("prepare bedrock body: %w", err)
	}
	targetURL := buildBedrockURL(creds.region, mappedModel, isStream)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(bedrockBody))
	if err != nil {
		return nil, nil, fmt.Errorf("build bedrock request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	return httpReq, bedrockBody, nil
}

// authenticateBedrockRequest applies either SigV4 or API key authentication.
func authenticateBedrockRequest(ctx context.Context, httpReq *http.Request, body []byte, creds *bedrockCreds) error {
	if creds.isAPIKey() {
		httpReq.Header.Set("Authorization", "Bearer "+creds.apiKey)
		return nil
	}
	if creds.accessKeyID == "" || creds.secretAccessKey == "" {
		return fmt.Errorf("bedrock SigV4 requires aws_access_key_id and aws_secret_access_key")
	}
	if err := signBedrockRequest(ctx, httpReq, body, creds); err != nil {
		return fmt.Errorf("sign bedrock request: %w", err)
	}
	return nil
}

// collectSingleValueHeaders extracts single-value headers from the response.
func collectSingleValueHeaders(resp *http.Response) map[string]string {
	headers := make(map[string]string)
	for key, values := range resp.Header {
		if len(values) > 0 {
			headers[key] = values[0]
		}
	}
	return headers
}

// sendBedrockResponseHeaders sends response headers to the host.
func sendBedrockResponseHeaders(
	stream grpc.ServerStreamingServer[pb.GatewayForwardChunk],
	statusCode int,
	headers map[string]string,
) error {
	return stream.Send(&pb.GatewayForwardChunk{
		Chunk: &pb.GatewayForwardChunk_Headers{
			Headers: &pb.GatewayResponseHeaders{
				StatusCode: int32(statusCode),
				Headers:    headers,
			},
		},
	})
}

// sendBedrockSuccess processes the successful response body and sends the Done chunk.
func sendBedrockSuccess(
	stream grpc.ServerStreamingServer[pb.GatewayForwardChunk],
	resp *http.Response,
	respHeaders map[string]string,
	originalModel, mappedModel string,
	isStream bool,
	startTime time.Time,
) error {
	var result *streamResult
	var err error
	if isStream {
		result, err = processBedrockStream(stream, resp.Body, startTime, originalModel, mappedModel)
	} else {
		result, err = processBedrockNonStreamResponse(stream, resp.Body, originalModel, mappedModel)
	}

	reqID := bedrockRequestID(respHeaders)
	done := buildBedrockDoneChunk(reqID, originalModel, mappedModel, isStream, startTime, result, err)
	if dc, ok := done.Chunk.(*pb.GatewayForwardChunk_Done); ok && dc.Done.GetResult() != nil {
		dc.Done.GetResult().ResponseHeaders = gatewayutil.CollectResponseHeaders(resp, nil)
	}
	return stream.Send(done)
}

// handleBedrockError processes Bedrock 4xx/5xx error responses.
func handleBedrockError(
	stream grpc.ServerStreamingServer[pb.GatewayForwardChunk],
	resp *http.Response,
	headers map[string]string,
	originalModel, mappedModel string,
	isStream bool,
	startTime time.Time,
) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodySize))

	if len(body) > 0 {
		if err := gatewayutil.SendBodyChunk(stream, body); err != nil {
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

	if gatewayutil.ShouldFailoverStatus(resp.StatusCode, extraFailoverCodes) {
		fwdResult.UpstreamError = &pb.GatewayUpstreamError{
			StatusCode:      int32(resp.StatusCode),
			ErrorType:       gatewayutil.ClassifyErrorType(resp.StatusCode, extraOverloadedCodes),
			ResponseBody:    gatewayutil.TruncateBytes(body, gatewayutil.MaxErrorBodyForProto),
			ResponseHeaders: gatewayutil.CollectResponseHeaders(resp, nil),
		}
	}

	done := &pb.GatewayForwardChunk{
		Chunk: &pb.GatewayForwardChunk_Done{
			Done: &pb.GatewayResponseDone{
				Error:  fmt.Sprintf("bedrock upstream returned %d", resp.StatusCode),
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
