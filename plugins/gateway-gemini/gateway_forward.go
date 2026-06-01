package main

import (
	"context"
	"fmt"
	"github.com/Wei-Shaw/sub2api/plugin-sdk/gatewayutil"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
	"google.golang.org/grpc"
)

const (
	// geminiCLIBaseURL is the Code Assist / Gemini CLI v1internal endpoint.
	geminiCLIBaseURL = "https://cloudcode-pa.googleapis.com"

	// geminiCLIUserAgent is the user agent string for Code Assist requests.
	geminiCLIUserAgent = "gemini-cli/1.0 linux/amd64"
)

// gatewayProviderServer implements GatewayProviderExtensionServer.
// It handles upstream API forwarding for the Gemini platform.
type gatewayProviderServer struct {
	pb.UnimplementedGatewayProviderExtensionServer
	logger func() *slog.Logger
}

func newGatewayProviderServer(logFn func() *slog.Logger) *gatewayProviderServer {
	return &gatewayProviderServer{
		logger: logFn,
	}
}

// Forward handles a gateway request by proxying it to the upstream
// Gemini API and streaming the response back as GatewayForwardChunk
// messages.
//
// Account type dispatch:
//   - apikey          -> AI Studio REST API (x-goog-api-key auth)
//   - oauth           -> AI Studio or Code Assist v1internal (Bearer auth)
//   - service_account -> Vertex AI API (Bearer auth)
func (s *gatewayProviderServer) Forward(
	req *pb.GatewayForwardRequest,
	stream grpc.ServerStreamingServer[pb.GatewayForwardChunk],
) error {
	startTime := time.Now()
	ctx := stream.Context()

	acct := req.GetAccount()
	if acct == nil {
		return fmt.Errorf("gateway-gemini: no account info in request")
	}

	creds, err := parseCredentials(acct.GetCredentialsJson())
	if err != nil {
		return fmt.Errorf("gateway-gemini: parse credentials: %w", err)
	}

	s.logger().Info("forward request",
		"request_id", req.GetRequestId(),
		"model", req.GetModel(),
		"account_type", acct.GetAccountType(),
		"stream", req.GetStream(),
	)

	switch acct.GetAccountType() {
	case accountTypeAPIKey:
		return s.forwardAPIKey(ctx, stream, req, creds, startTime)
	case accountTypeOAuth:
		return s.forwardOAuth(ctx, stream, req, creds, startTime)
	case accountTypeServiceAccount:
		return s.forwardServiceAccount(ctx, stream, req, creds, startTime)
	default:
		return fmt.Errorf("gateway-gemini: unsupported account type %q", acct.GetAccountType())
	}
}

// forwardAPIKey forwards a request to the Google AI Studio REST API
// using an API key.
func (s *gatewayProviderServer) forwardAPIKey(
	ctx context.Context,
	stream grpc.ServerStreamingServer[pb.GatewayForwardChunk],
	req *pb.GatewayForwardRequest,
	creds *geminiCredentials,
	startTime time.Time,
) error {
	upstream, err := buildUpstreamAPIKeyRequest(ctx, req, creds)
	if err != nil {
		return fmt.Errorf("gateway-gemini: %w", err)
	}

	action := req.GetGeminiAction()
	isStreaming := req.GetStream() || action == "streamGenerateContent"

	return s.executeAndStream(stream, upstream, isStreaming, startTime)
}

// forwardOAuth forwards a request using an OAuth access token. Two
// modes exist:
//  1. With project_id -> Code Assist v1internal endpoint (wrapped request)
//  2. Without project_id -> AI Studio API (direct, like API key mode)
func (s *gatewayProviderServer) forwardOAuth(
	ctx context.Context,
	stream grpc.ServerStreamingServer[pb.GatewayForwardChunk],
	req *pb.GatewayForwardRequest,
	creds *geminiCredentials,
	startTime time.Time,
) error {
	accessToken := strings.TrimSpace(creds.AccessToken)
	if accessToken == "" {
		return fmt.Errorf("gateway-gemini: no access token available")
	}

	projectID := strings.TrimSpace(creds.ProjectID)
	if projectID != "" {
		return s.forwardOAuthCodeAssist(ctx, stream, req, accessToken, projectID, startTime)
	}
	return s.forwardOAuthAIStudio(ctx, stream, req, creds, accessToken, startTime)
}

// forwardOAuthCodeAssist forwards through the Code Assist v1internal
// endpoint.
func (s *gatewayProviderServer) forwardOAuthCodeAssist(
	ctx context.Context,
	stream grpc.ServerStreamingServer[pb.GatewayForwardChunk],
	req *pb.GatewayForwardRequest,
	accessToken, projectID string,
	startTime time.Time,
) error {
	upstream, err := buildUpstreamCodeAssistRequest(ctx, req, accessToken, projectID)
	if err != nil {
		return fmt.Errorf("gateway-gemini: %w", err)
	}
	// v1internal always uses streaming.
	return s.executeAndStream(stream, upstream, true, startTime)
}

// forwardOAuthAIStudio forwards to AI Studio using Bearer token auth.
func (s *gatewayProviderServer) forwardOAuthAIStudio(
	ctx context.Context,
	stream grpc.ServerStreamingServer[pb.GatewayForwardChunk],
	req *pb.GatewayForwardRequest,
	creds *geminiCredentials,
	accessToken string,
	startTime time.Time,
) error {
	upstream, err := buildUpstreamOAuthAIStudioRequest(ctx, req, creds, accessToken)
	if err != nil {
		return fmt.Errorf("gateway-gemini: %w", err)
	}

	action := req.GetGeminiAction()
	isStreaming := req.GetStream() || action == "streamGenerateContent"

	return s.executeAndStream(stream, upstream, isStreaming, startTime)
}

// forwardServiceAccount forwards a request to Vertex AI using a
// service account's access token.
func (s *gatewayProviderServer) forwardServiceAccount(
	ctx context.Context,
	stream grpc.ServerStreamingServer[pb.GatewayForwardChunk],
	req *pb.GatewayForwardRequest,
	creds *geminiCredentials,
	startTime time.Time,
) error {
	accessToken := strings.TrimSpace(creds.AccessToken)
	if accessToken == "" {
		return fmt.Errorf("gateway-gemini: no access token for service account")
	}

	upstream, err := buildUpstreamVertexRequest(ctx, req, creds, accessToken)
	if err != nil {
		return fmt.Errorf("gateway-gemini: %w", err)
	}

	isStreaming := req.GetStream() ||
		strings.Contains(upstream.httpReq.URL.String(), "streamGenerateContent")

	return s.executeAndStream(stream, upstream, isStreaming, startTime)
}

// executeAndStream sends the upstream request and streams the response
// back through gRPC. This is the shared execution path for all account
// types, handling headers, errors, streaming/non-streaming responses,
// and the terminal Done chunk.
func (s *gatewayProviderServer) executeAndStream(
	stream grpc.ServerStreamingServer[pb.GatewayForwardChunk],
	upstream *upstreamRequestInfo,
	isStream bool,
	startTime time.Time,
) error {
	resp, err := defaultHTTPClient.Do(upstream.httpReq)
	if err != nil {
		return fmt.Errorf("gateway-gemini: upstream request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Send response headers.
	if err := sendResponseHeaders(stream, resp); err != nil {
		return fmt.Errorf("gateway-gemini: send headers: %w", err)
	}

	// Handle error responses with structured upstream error.
	if resp.StatusCode >= 400 {
		return s.handleErrorResponse(stream, resp, upstream, isStream, startTime)
	}

	// Process the response body.
	var result *streamResult
	if isStream {
		result, err = proxySSEStream(stream, resp, startTime,
			upstream.originalModel, upstream.mappedModel)
	} else {
		result, err = proxyNonStreamResponse(stream, resp)
	}

	// Build and send the Done chunk with usage data.
	done := buildDoneChunk(resp, upstream, result, isStream, startTime, err)
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
	upstream *upstreamRequestInfo,
	isStream bool,
	startTime time.Time,
) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodySize))

	result := &pb.GatewayForwardResult{
		RequestId:     resp.Header.Get("x-goog-request-id"),
		Model:         upstream.originalModel,
		UpstreamModel: upstream.mappedModel,
		Stream:        isStream,
		DurationMs:    time.Since(startTime).Milliseconds(),
	}

	return gatewayutil.HandleErrorResponse(
		stream, resp.StatusCode, body, result,
		gatewayutil.CollectResponseHeaders(resp, nil), nil, nil,
	)
}

// ShouldFailover determines whether a failed forward should be retried
// with a different account. See gateway_failover.go for classification.
func (s *gatewayProviderServer) ShouldFailover(
	_ context.Context,
	req *pb.GatewayFailoverRequest,
) (*pb.GatewayFailoverResponse, error) {
	should := classifyShouldFailover(req)
	return &pb.GatewayFailoverResponse{ShouldFailover: should}, nil
}
