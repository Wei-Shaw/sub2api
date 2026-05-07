package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
	"google.golang.org/grpc"
)

// gatewayProviderServer implements GatewayProviderExtensionServer.
// It handles upstream API forwarding for the Antigravity platform.
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

// Forward handles a gateway request by proxying it to the upstream API
// and streaming the response back as GatewayForwardChunk messages.
//
// Dispatch logic:
//   - protocol "anthropic" → Claude API (via Antigravity v1internal or upstream)
//   - protocol "gemini"    → Gemini API (via Antigravity v1internal or upstream)
func (s *gatewayProviderServer) Forward(
	req *pb.GatewayForwardRequest,
	stream grpc.ServerStreamingServer[pb.GatewayForwardChunk],
) error {
	startTime := time.Now()
	ctx := stream.Context()

	acct := req.GetAccount()
	if acct == nil {
		return fmt.Errorf("gateway-antigravity: no account info in request")
	}

	creds, err := parseCredentials(acct.GetCredentialsJson())
	if err != nil {
		return fmt.Errorf("gateway-antigravity: parse credentials: %w", err)
	}

	s.logger.Info("forward request",
		"request_id", req.GetRequestId(),
		"model", req.GetModel(),
		"protocol", req.GetProtocol(),
		"account_type", acct.GetAccountType(),
		"stream", req.GetStream(),
	)

	switch acct.GetAccountType() {
	case accountTypeUpstream:
		return s.forwardUpstream(ctx, stream, req, creds, startTime)
	default:
		return s.forwardViaAntigravity(ctx, stream, req, creds, startTime)
	}
}

// forwardUpstream handles upstream pass-through accounts. The request
// body is forwarded as-is to the configured base_url.
func (s *gatewayProviderServer) forwardUpstream(
	ctx context.Context,
	stream grpc.ServerStreamingServer[pb.GatewayForwardChunk],
	req *pb.GatewayForwardRequest,
	creds *antigravityCredentials,
	startTime time.Time,
) error {
	baseURL := strings.TrimSpace(creds.BaseURL)
	apiKey := strings.TrimSpace(creds.APIKey)
	if baseURL == "" || apiKey == "" {
		return fmt.Errorf("gateway-antigravity: upstream account missing base_url or api_key")
	}

	upstreamURL := strings.TrimSuffix(baseURL, "/") + "/v1/messages"
	headers := map[string]string{
		"Content-Type":       "application/json",
		"Authorization":      "Bearer " + apiKey,
		"x-api-key":          apiKey,
		"anthropic-version":  "2023-06-01",
	}

	return s.upstreamClient.proxyRequest(ctx, stream, upstreamRequest{
		method:    "POST",
		url:       upstreamURL,
		headers:   headers,
		body:      req.GetRawBody(),
		model:     req.GetModel(),
		isStream:  req.GetStream(),
		startTime: startTime,
		requestID: req.GetRequestId(),
		extractor: &claudeUsageExtractor{},
	})
}

// forwardViaAntigravity handles OAuth and API key accounts by forwarding
// through the Antigravity v1internal endpoint (for OAuth) or directly
// to Claude/Gemini APIs (for API key accounts).
func (s *gatewayProviderServer) forwardViaAntigravity(
	ctx context.Context,
	stream grpc.ServerStreamingServer[pb.GatewayForwardChunk],
	req *pb.GatewayForwardRequest,
	creds *antigravityCredentials,
	startTime time.Time,
) error {
	switch req.GetProtocol() {
	case "anthropic":
		return s.forwardAnthropicProtocol(ctx, stream, req, creds, startTime)
	case "gemini":
		return s.forwardGeminiProtocol(ctx, stream, req, creds, startTime)
	default:
		return fmt.Errorf("gateway-antigravity: unsupported protocol %q", req.GetProtocol())
	}
}

// forwardAnthropicProtocol forwards a Claude Messages API request.
// OAuth accounts go through the Antigravity v1internal endpoint with
// Claude→Gemini format conversion; API key accounts call the Anthropic
// Messages API directly.
func (s *gatewayProviderServer) forwardAnthropicProtocol(
	ctx context.Context,
	stream grpc.ServerStreamingServer[pb.GatewayForwardChunk],
	req *pb.GatewayForwardRequest,
	creds *antigravityCredentials,
	startTime time.Time,
) error {
	acct := req.GetAccount()
	switch acct.GetAccountType() {
	case accountTypeAPIKey:
		return s.forwardClaudeAPIKey(ctx, stream, req, creds, startTime)
	case accountTypeOAuth:
		return s.forwardOAuthClaude(ctx, stream, req, creds, startTime)
	default:
		return fmt.Errorf("gateway-antigravity: unsupported account type %q for anthropic protocol",
			acct.GetAccountType())
	}
}

// forwardClaudeAPIKey forwards to the Anthropic Messages API using an
// API key (direct passthrough, no format conversion needed).
func (s *gatewayProviderServer) forwardClaudeAPIKey(
	ctx context.Context,
	stream grpc.ServerStreamingServer[pb.GatewayForwardChunk],
	req *pb.GatewayForwardRequest,
	creds *antigravityCredentials,
	startTime time.Time,
) error {
	apiKey := strings.TrimSpace(creds.APIKey)
	if apiKey == "" {
		return fmt.Errorf("gateway-antigravity: no API key available")
	}

	baseURL := strings.TrimSpace(creds.BaseURL)
	if baseURL == "" {
		baseURL = "https://api.anthropic.com"
	}
	apiURL := strings.TrimSuffix(baseURL, "/") + "/v1/messages"

	headers := map[string]string{
		"Content-Type":      "application/json",
		"X-Api-Key":         apiKey,
		"anthropic-version": "2023-06-01",
	}

	return s.upstreamClient.proxyRequest(ctx, stream, upstreamRequest{
		method:    "POST",
		url:       apiURL,
		headers:   headers,
		body:      req.GetRawBody(),
		model:     req.GetModel(),
		isStream:  req.GetStream(),
		startTime: startTime,
		requestID: req.GetRequestId(),
		extractor: &claudeUsageExtractor{},
	})
}

// forwardOAuthClaude forwards a Claude request through the Antigravity
// v1internal endpoint. The Claude request body is sent as-is for now
// (Phase 1: basic proxy without Claude→Gemini conversion).
//
// Note: Full Claude→Gemini format conversion requires the antigravity
// package from the host, which is not available to the plugin. Phase 2
// will implement this conversion within the plugin.
func (s *gatewayProviderServer) forwardOAuthClaude(
	ctx context.Context,
	stream grpc.ServerStreamingServer[pb.GatewayForwardChunk],
	req *pb.GatewayForwardRequest,
	creds *antigravityCredentials,
	startTime time.Time,
) error {
	accessToken := strings.TrimSpace(creds.AccessToken)
	if accessToken == "" {
		return fmt.Errorf("gateway-antigravity: no access token available")
	}

	projectID := strings.TrimSpace(creds.ProjectID)

	// Build Antigravity v1internal payload (wraps the raw body as Gemini format).
	// Phase 1: use the raw body directly since we cannot do Claude→Gemini
	// conversion without the host's antigravity package.
	payload := buildAntigravityForwardPayload(projectID, req.GetModel(), req.GetRawBody())

	apiURL := antigravityProdBaseURL + "/v1internal:streamGenerateContent?alt=sse"
	headers := map[string]string{
		"Content-Type":  "application/json",
		"Authorization": "Bearer " + accessToken,
		"User-Agent":    antigravityUserAgent,
	}

	return s.upstreamClient.proxyRequest(ctx, stream, upstreamRequest{
		method:    "POST",
		url:       apiURL,
		headers:   headers,
		body:      payload,
		model:     req.GetModel(),
		isStream:  true, // Antigravity v1internal always uses streaming
		startTime: startTime,
		requestID: req.GetRequestId(),
		extractor: &geminiUsageExtractor{},
	})
}

// forwardGeminiProtocol forwards a Gemini native request through the
// Antigravity v1internal endpoint or via direct API key access.
func (s *gatewayProviderServer) forwardGeminiProtocol(
	ctx context.Context,
	stream grpc.ServerStreamingServer[pb.GatewayForwardChunk],
	req *pb.GatewayForwardRequest,
	creds *antigravityCredentials,
	startTime time.Time,
) error {
	acct := req.GetAccount()
	switch acct.GetAccountType() {
	case accountTypeOAuth:
		return s.forwardOAuthGemini(ctx, stream, req, creds, startTime)
	case accountTypeAPIKey:
		return s.forwardGeminiAPIKey(ctx, stream, req, creds, startTime)
	default:
		return fmt.Errorf("gateway-antigravity: unsupported account type %q for gemini protocol",
			acct.GetAccountType())
	}
}

// forwardOAuthGemini forwards a Gemini request through the Antigravity
// v1internal endpoint using an OAuth access token.
func (s *gatewayProviderServer) forwardOAuthGemini(
	ctx context.Context,
	stream grpc.ServerStreamingServer[pb.GatewayForwardChunk],
	req *pb.GatewayForwardRequest,
	creds *antigravityCredentials,
	startTime time.Time,
) error {
	accessToken := strings.TrimSpace(creds.AccessToken)
	if accessToken == "" {
		return fmt.Errorf("gateway-antigravity: no access token available")
	}

	projectID := strings.TrimSpace(creds.ProjectID)

	// Wrap the Gemini request body for the v1internal endpoint.
	payload := buildAntigravityForwardPayload(projectID, req.GetModel(), req.GetRawBody())

	action := req.GetGeminiAction()
	if action == "" {
		action = "streamGenerateContent"
	}

	apiURL := antigravityProdBaseURL + "/v1internal:" + action + "?alt=sse"
	headers := map[string]string{
		"Content-Type":  "application/json",
		"Authorization": "Bearer " + accessToken,
		"User-Agent":    antigravityUserAgent,
	}

	return s.upstreamClient.proxyRequest(ctx, stream, upstreamRequest{
		method:    "POST",
		url:       apiURL,
		headers:   headers,
		body:      payload,
		model:     req.GetModel(),
		isStream:  true,
		startTime: startTime,
		requestID: req.GetRequestId(),
		extractor: &geminiUsageExtractor{},
	})
}

// forwardGeminiAPIKey forwards a Gemini request using an API key to
// the standard Gemini generateContent endpoint.
func (s *gatewayProviderServer) forwardGeminiAPIKey(
	ctx context.Context,
	stream grpc.ServerStreamingServer[pb.GatewayForwardChunk],
	req *pb.GatewayForwardRequest,
	creds *antigravityCredentials,
	startTime time.Time,
) error {
	apiKey := strings.TrimSpace(creds.APIKey)
	if apiKey == "" {
		return fmt.Errorf("gateway-antigravity: no API key available")
	}

	baseURL := strings.TrimSpace(creds.BaseURL)
	if baseURL == "" {
		baseURL = "https://generativelanguage.googleapis.com"
	}

	action := req.GetGeminiAction()
	if action == "" {
		if req.GetStream() {
			action = "streamGenerateContent"
		} else {
			action = "generateContent"
		}
	}

	apiURL := fmt.Sprintf("%s/v1beta/models/%s:%s",
		strings.TrimSuffix(baseURL, "/"), req.GetModel(), action)
	if req.GetStream() || action == "streamGenerateContent" {
		apiURL += "?alt=sse"
	}

	headers := map[string]string{
		"Content-Type":   "application/json",
		"x-goog-api-key": apiKey,
	}

	return s.upstreamClient.proxyRequest(ctx, stream, upstreamRequest{
		method:    "POST",
		url:       apiURL,
		headers:   headers,
		body:      req.GetRawBody(),
		model:     req.GetModel(),
		isStream:  req.GetStream(),
		startTime: startTime,
		requestID: req.GetRequestId(),
		extractor: &geminiUsageExtractor{},
	})
}

// ShouldFailover returns whether a failed forward should be retried with
// a different account. For Antigravity, we failover on 429 (rate limit),
// 503 (service unavailable), and 502 (bad gateway).
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

// buildAntigravityForwardPayload wraps a request body in the
// Antigravity v1internal envelope format:
//
//	{"model": "<model>", "project": "<projectID>", "request": <body>}
func buildAntigravityForwardPayload(projectID, model string, body []byte) []byte {
	wrapper := map[string]any{
		"model": model,
	}
	if projectID != "" {
		wrapper["project"] = projectID
	}

	// Parse the body to embed as the "request" field.
	var inner any
	if err := json.Unmarshal(body, &inner); err != nil {
		// If we cannot parse it, wrap as a string fallback.
		wrapper["request"] = string(body)
	} else {
		wrapper["request"] = inner
	}

	result, _ := json.Marshal(wrapper)
	return result
}
