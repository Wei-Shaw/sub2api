package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/plugin-sdk/gatewayutil"
	pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
	"google.golang.org/grpc"
)

// forwardVertex handles forwarding for Vertex AI (service_account) accounts.
//
// The flow mirrors the core's buildUpstreamRequestAnthropicVertex:
//  1. Parse credentials and obtain a Google OAuth2 access token
//  2. Resolve model (apply mapping + Vertex date normalization)
//  3. Build the Vertex request body (remove model, add anthropic_version)
//  4. Construct the Vertex rawPredict/streamRawPredict URL
//  5. Execute and stream the response (same SSE format as standard Anthropic)
func (s *gatewayProviderServer) forwardVertex(
	req *pb.GatewayForwardRequest,
	stream grpc.ServerStreamingServer[pb.GatewayForwardChunk],
) error {
	startTime := time.Now()
	ctx := stream.Context()
	acct := req.GetAccount()
	creds := parseVertexCredentialsRaw(acct.GetCredentialsJson())

	accessToken, err := exchangeVertexServiceAccountToken(ctx, acct.GetProxyUrl(), creds.serviceAccountJSON)
	if err != nil {
		return fmt.Errorf("vertex token exchange: %w", err)
	}

	originalModel := req.GetModel()
	mappedModel := resolveVertexModel(originalModel, acct.GetCredentialsJson())
	isStream := req.GetStream()

	httpReq, err := buildVertexHTTPRequest(ctx, req, creds, accessToken, mappedModel, isStream)
	if err != nil {
		return err
	}
	forwardVertexClientHeaders(httpReq, req.GetHeaders())
	forwardVertexClientHeaders(httpReq, req.GetClientHeaders())

	resp, err := defaultHTTPClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("vertex request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if err := sendResponseHeaders(stream, resp); err != nil {
		return err
	}

	if resp.StatusCode >= 400 {
		return handleVertexError(stream, resp, originalModel, mappedModel, isStream, startTime)
	}
	return sendVertexSuccess(stream, resp, originalModel, mappedModel, isStream, startTime)
}

// buildVertexHTTPRequest constructs the upstream HTTP request for Vertex AI.
func buildVertexHTTPRequest(
	ctx context.Context,
	req *pb.GatewayForwardRequest,
	creds *vertexCreds,
	accessToken, mappedModel string,
	isStream bool,
) (*http.Request, error) {
	vertexBody, err := buildVertexAnthropicRequestBody(req.GetRawBody())
	if err != nil {
		return nil, fmt.Errorf("build vertex body: %w", err)
	}
	projectID, err := creds.resolveProjectID()
	if err != nil {
		return nil, fmt.Errorf("vertex credentials: %w", err)
	}
	location := creds.locationForModel(mappedModel)
	targetURL, err := buildVertexAnthropicURL(projectID, location, mappedModel, isStream)
	if err != nil {
		return nil, fmt.Errorf("build vertex url: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(vertexBody))
	if err != nil {
		return nil, fmt.Errorf("build vertex http request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+accessToken)
	httpReq.Header.Set("Content-Type", "application/json")
	return httpReq, nil
}

// sendVertexSuccess processes the successful response and sends the Done chunk.
func sendVertexSuccess(
	stream grpc.ServerStreamingServer[pb.GatewayForwardChunk],
	resp *http.Response,
	originalModel, mappedModel string,
	isStream bool,
	startTime time.Time,
) error {
	var result *streamResult
	var err error
	if isStream {
		result, err = processSSEStream(stream, resp.Body, startTime, originalModel, mappedModel)
	} else {
		result, err = processNonStreamResponse(stream, resp.Body, originalModel, mappedModel)
	}
	done := buildVertexDoneChunk(resp, originalModel, mappedModel, isStream, startTime, result, err)
	return stream.Send(done)
}

// handleVertexError processes Vertex 4xx/5xx error responses.
func handleVertexError(
	stream grpc.ServerStreamingServer[pb.GatewayForwardChunk],
	resp *http.Response,
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

	fwdResult := &pb.GatewayForwardResult{
		RequestId:     resp.Header.Get("x-request-id"),
		Model:         originalModel,
		UpstreamModel: mappedModel,
		Stream:        isStream,
		DurationMs:    time.Since(startTime).Milliseconds(),
	}
	if gatewayutil.ShouldFailoverStatus(resp.StatusCode, extraFailoverCodes) {
		fwdResult.UpstreamError = &pb.GatewayUpstreamError{
			StatusCode:   int32(resp.StatusCode),
			ErrorType:    gatewayutil.ClassifyErrorType(resp.StatusCode, extraOverloadedCodes),
			ResponseBody: gatewayutil.TruncateBytes(body, gatewayutil.MaxErrorBodyForProto),
		}
	}

	done := &pb.GatewayForwardChunk{
		Chunk: &pb.GatewayForwardChunk_Done{
			Done: &pb.GatewayResponseDone{
				Error:  fmt.Sprintf("vertex upstream returned %d", resp.StatusCode),
				Result: fwdResult,
			},
		},
	}
	return stream.Send(done)
}

// buildVertexDoneChunk constructs the terminal Done chunk for Vertex responses.
func buildVertexDoneChunk(
	resp *http.Response,
	originalModel, mappedModel string,
	isStream bool,
	startTime time.Time,
	result *streamResult,
	streamErr error,
) *pb.GatewayForwardChunk {
	done := &pb.GatewayResponseDone{
		Result: &pb.GatewayForwardResult{
			RequestId:     resp.Header.Get("x-request-id"),
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

// vertexClientHeadersAllowList lists headers from the client that may be
// forwarded to Vertex AI.
var vertexClientHeadersAllowList = map[string]bool{
	"content-type":    true,
	"user-agent":      true,
	"accept-encoding": true,
}

// forwardVertexClientHeaders copies allowed client headers to the Vertex
// upstream request.
func forwardVertexClientHeaders(httpReq *http.Request, clientHeaders map[string]string) {
	for key, value := range clientHeaders {
		lower := strings.ToLower(key)
		if !vertexClientHeadersAllowList[lower] {
			continue
		}
		if httpReq.Header.Get(key) == "" {
			httpReq.Header.Set(key, value)
		}
	}
}
