package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
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
	upstreamClient *upstreamClient
	logger         *slog.Logger
}

func newGatewayProviderServer(logger *slog.Logger) *gatewayProviderServer {
	return &gatewayProviderServer{
		upstreamClient: newUpstreamClient(),
		logger:         logger,
	}
}

// Forward handles a gateway request by proxying it to the upstream
// Gemini API and streaming the response back as GatewayForwardChunk
// messages.
//
// Account type dispatch:
//   - apikey         → AI Studio REST API (x-goog-api-key auth)
//   - oauth          → AI Studio or Code Assist v1internal (Bearer auth)
//   - service_account → Vertex AI API (Bearer auth)
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

	s.logger.Info("forward request",
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
	apiKey := strings.TrimSpace(creds.APIKey)
	if apiKey == "" {
		return fmt.Errorf("gateway-gemini: no API key available")
	}

	baseURL := strings.TrimSpace(creds.BaseURL)
	if baseURL == "" {
		baseURL = aiStudioBaseURL
	}

	action, apiURL := buildGeminiAPIURL(baseURL, req.GetModel(), req.GetGeminiAction(), req.GetStream())

	headers := map[string]string{
		"Content-Type":    "application/json",
		"x-goog-api-key": apiKey,
	}

	isStreaming := req.GetStream() || action == "streamGenerateContent"
	return s.upstreamClient.proxyRequest(ctx, stream, upstreamRequest{
		method:    "POST",
		url:       apiURL,
		headers:   headers,
		body:      req.GetRawBody(),
		model:     req.GetModel(),
		isStream:  isStreaming,
		startTime: startTime,
		requestID: req.GetRequestId(),
		extractor: &geminiUsageExtractor{},
	})
}

// forwardOAuth forwards a request using an OAuth access token. Two
// modes exist:
//  1. With project_id → Code Assist v1internal endpoint (wrapped request)
//  2. Without project_id → AI Studio API (direct, like API key mode)
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
		return s.forwardOAuthCodeAssist(ctx, stream, req, creds, accessToken, projectID, startTime)
	}
	return s.forwardOAuthAIStudio(ctx, stream, req, creds, accessToken, startTime)
}

// forwardOAuthCodeAssist forwards through the Code Assist v1internal endpoint.
func (s *gatewayProviderServer) forwardOAuthCodeAssist(
	ctx context.Context,
	stream grpc.ServerStreamingServer[pb.GatewayForwardChunk],
	req *pb.GatewayForwardRequest,
	creds *geminiCredentials,
	accessToken, projectID string,
	startTime time.Time,
) error {
	action := req.GetGeminiAction()
	if action == "" {
		action = "streamGenerateContent"
	}

	apiURL := geminiCLIBaseURL + "/v1internal:" + action + "?alt=sse"

	// Wrap the request body for the v1internal endpoint.
	payload := buildCodeAssistPayload(projectID, req.GetModel(), req.GetRawBody())

	headers := map[string]string{
		"Content-Type":  "application/json",
		"Authorization": "Bearer " + accessToken,
		"User-Agent":    geminiCLIUserAgent,
	}

	return s.upstreamClient.proxyRequest(ctx, stream, upstreamRequest{
		method:    "POST",
		url:       apiURL,
		headers:   headers,
		body:      payload,
		model:     req.GetModel(),
		isStream:  true, // v1internal always uses streaming
		startTime: startTime,
		requestID: req.GetRequestId(),
		extractor: &geminiUsageExtractor{},
	})
}

// forwardOAuthAIStudio forwards to AI Studio using Bearer token auth
// (same as API key mode but with Authorization header).
func (s *gatewayProviderServer) forwardOAuthAIStudio(
	ctx context.Context,
	stream grpc.ServerStreamingServer[pb.GatewayForwardChunk],
	req *pb.GatewayForwardRequest,
	creds *geminiCredentials,
	accessToken string,
	startTime time.Time,
) error {
	baseURL := strings.TrimSpace(creds.BaseURL)
	if baseURL == "" {
		baseURL = aiStudioBaseURL
	}

	action, apiURL := buildGeminiAPIURL(baseURL, req.GetModel(), req.GetGeminiAction(), req.GetStream())

	headers := map[string]string{
		"Content-Type":  "application/json",
		"Authorization": "Bearer " + accessToken,
	}

	isStreaming := req.GetStream() || action == "streamGenerateContent"
	return s.upstreamClient.proxyRequest(ctx, stream, upstreamRequest{
		method:    "POST",
		url:       apiURL,
		headers:   headers,
		body:      req.GetRawBody(),
		model:     req.GetModel(),
		isStream:  isStreaming,
		startTime: startTime,
		requestID: req.GetRequestId(),
		extractor: &geminiUsageExtractor{},
	})
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
		return fmt.Errorf("gateway-gemini: no access token available for service account")
	}

	apiURL, err := buildVertexForwardURL(creds, req.GetModel(), req.GetGeminiAction(), req.GetStream())
	if err != nil {
		return fmt.Errorf("gateway-gemini: %w", err)
	}

	headers := map[string]string{
		"Content-Type":  "application/json",
		"Authorization": "Bearer " + accessToken,
	}

	isStreaming := req.GetStream() || strings.Contains(apiURL, "streamGenerateContent")
	return s.upstreamClient.proxyRequest(ctx, stream, upstreamRequest{
		method:    "POST",
		url:       apiURL,
		headers:   headers,
		body:      req.GetRawBody(),
		model:     req.GetModel(),
		isStream:  isStreaming,
		startTime: startTime,
		requestID: req.GetRequestId(),
		extractor: &geminiUsageExtractor{},
	})
}

// ShouldFailover returns whether a failed forward should be retried
// with a different account. For Gemini, we failover on 429 (rate
// limit), 503 (service unavailable), and authentication errors.
func (s *gatewayProviderServer) ShouldFailover(
	_ context.Context,
	req *pb.GatewayFailoverRequest,
) (*pb.GatewayFailoverResponse, error) {
	errMsg := req.GetErrorMessage()
	errType := req.GetErrorType()

	shouldFailover := strings.Contains(errType, "UpstreamFailoverError") ||
		strings.Contains(errMsg, "rate limit") ||
		strings.Contains(errMsg, "429") ||
		strings.Contains(errMsg, "503") ||
		strings.Contains(errMsg, "502")

	return &pb.GatewayFailoverResponse{
		ShouldFailover: shouldFailover,
	}, nil
}

// buildGeminiAPIURL constructs the AI Studio REST API URL for a Gemini
// model. Returns (action, fullURL).
func buildGeminiAPIURL(baseURL, model, geminiAction string, isStream bool) (string, string) {
	action := geminiAction
	if action == "" {
		if isStream {
			action = "streamGenerateContent"
		} else {
			action = "generateContent"
		}
	}

	apiURL := fmt.Sprintf("%s/v1beta/models/%s:%s",
		strings.TrimRight(baseURL, "/"), model, action)
	if isStream || action == "streamGenerateContent" {
		apiURL += "?alt=sse"
	}
	return action, apiURL
}

// buildVertexForwardURL constructs the Vertex AI API URL for a Gemini model.
func buildVertexForwardURL(creds *geminiCredentials, model, geminiAction string, isStream bool) (string, error) {
	projectID := strings.TrimSpace(creds.VertexProjectID)
	if projectID == "" {
		return "", fmt.Errorf("vertex_project_id is required for service accounts")
	}

	location := strings.TrimSpace(creds.VertexLocation)
	if location == "" {
		location = vertexDefaultLocation
	}
	if !vertexLocationPattern.MatchString(location) {
		return "", fmt.Errorf("invalid vertex location: %s", location)
	}

	action := geminiAction
	if action == "" {
		if isStream {
			action = "streamGenerateContent"
		} else {
			action = "generateContent"
		}
	}

	host := fmt.Sprintf("%s-aiplatform.googleapis.com", location)
	if location == "global" {
		host = "aiplatform.googleapis.com"
	}

	apiURL := fmt.Sprintf(
		"https://%s/v1/projects/%s/locations/%s/publishers/google/models/%s:%s",
		host,
		url.PathEscape(projectID),
		url.PathEscape(location),
		url.PathEscape(model),
		action,
	)
	if isStream || action == "streamGenerateContent" {
		apiURL += "?alt=sse"
	}
	return apiURL, nil
}
