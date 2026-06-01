package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/Wei-Shaw/sub2api/plugin-sdk/gatewayutil"
	"log/slog"
	"strings"
	"time"

	pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
	"google.golang.org/grpc"
)

// Protocol identifiers used in gateway forwarding.
const (
	protocolAnthropic = "anthropic"
	protocolGemini    = "gemini"
)

// gatewayProviderServer implements GatewayProviderExtensionServer.
type gatewayProviderServer struct {
	pb.UnimplementedGatewayProviderExtensionServer
	upstreamClient *upstreamClient
	logger         func() *slog.Logger
}

func newGatewayProviderServer(logFn func() *slog.Logger) *gatewayProviderServer {
	return &gatewayProviderServer{
		upstreamClient: newUpstreamClient(),
		logger:         logFn,
	}
}

// Forward handles a gateway request by proxying it to the upstream API.
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

	s.logger().Info("forward request",
		"request_id", req.GetRequestId(),
		"model", req.GetModel(),
		"protocol", req.GetProtocol(),
		"account_type", acct.GetAccountType(),
		"stream", req.GetStream(),
		"sticky_session", req.GetIsStickySession(),
	)

	switch acct.GetAccountType() {
	case accountTypeUpstream:
		return s.forwardUpstream(ctx, stream, req, creds, startTime)
	default:
		return s.forwardViaAntigravity(ctx, stream, req, creds, startTime)
	}
}

// ShouldFailover returns true when the error from Forward is transient.
func (s *gatewayProviderServer) ShouldFailover(
	_ context.Context,
	req *pb.GatewayFailoverRequest,
) (*pb.GatewayFailoverResponse, error) {
	should := classifyShouldFailover(req)
	return &pb.GatewayFailoverResponse{ShouldFailover: should}, nil
}

// forwardUpstream handles upstream pass-through accounts.
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

	originalModel := req.GetModel()
	mappedModel := resolveModelMapping(originalModel, req.GetAccount())

	body := req.GetRawBody()
	if mappedModel != originalModel {
		body = gatewayutil.ReplaceModelInBody(body, mappedModel)
	}

	return s.upstreamClient.proxyRequest(ctx, stream, upstreamReq{
		method: "POST",
		url:    strings.TrimSuffix(baseURL, "/") + "/v1/messages",
		headers: map[string]string{
			"Content-Type":      "application/json",
			"Authorization":     "Bearer " + apiKey,
			"x-api-key":         apiKey,
			"anthropic-version": "2023-06-01",
		},
		body:              body,
		originalModel:     originalModel,
		mappedModel:       mappedModel,
		isStream:          req.GetStream(),
		startTime:         startTime,
		requestID:         req.GetRequestId(),
		extractor:         &claudeUsageExtractor{},
		clientHeaders:     collectClientHeaders(req),
		forceCacheBilling: req.GetForceCacheBilling(),
	})
}

// forwardViaAntigravity dispatches by protocol.
func (s *gatewayProviderServer) forwardViaAntigravity(
	ctx context.Context,
	stream grpc.ServerStreamingServer[pb.GatewayForwardChunk],
	req *pb.GatewayForwardRequest,
	creds *antigravityCredentials,
	startTime time.Time,
) error {
	switch req.GetProtocol() {
	case protocolAnthropic:
		return s.forwardAnthropicProtocol(ctx, stream, req, creds, startTime)
	case protocolGemini:
		return s.forwardGeminiProtocol(ctx, stream, req, creds, startTime)
	default:
		return fmt.Errorf("gateway-antigravity: unsupported protocol %q", req.GetProtocol())
	}
}

// forwardAnthropicProtocol forwards a Claude Messages API request.
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
		return fmt.Errorf(
			"gateway-antigravity: unsupported account type %q for anthropic protocol",
			acct.GetAccountType())
	}
}

// forwardClaudeAPIKey forwards to the Anthropic Messages API using an API key.
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

	originalModel := req.GetModel()
	mappedModel := resolveModelMapping(originalModel, req.GetAccount())

	body := req.GetRawBody()
	if mappedModel != originalModel {
		body = gatewayutil.ReplaceModelInBody(body, mappedModel)
	}

	return s.upstreamClient.proxyRequest(ctx, stream, upstreamReq{
		method: "POST",
		url:    strings.TrimSuffix(baseURL, "/") + "/v1/messages",
		headers: map[string]string{
			"Content-Type":      "application/json",
			"X-Api-Key":         apiKey,
			"anthropic-version": "2023-06-01",
		},
		body:              body,
		originalModel:     originalModel,
		mappedModel:       mappedModel,
		isStream:          req.GetStream(),
		startTime:         startTime,
		requestID:         req.GetRequestId(),
		extractor:         &claudeUsageExtractor{},
		clientHeaders:     collectClientHeaders(req),
		forceCacheBilling: req.GetForceCacheBilling(),
	})
}

// forwardOAuthClaude forwards a Claude request through the Antigravity
// v1internal endpoint.
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

	originalModel := req.GetModel()
	payload := buildAntigravityForwardPayload(
		strings.TrimSpace(creds.ProjectID), originalModel, req.GetRawBody())

	return s.upstreamClient.proxyRequest(ctx, stream, upstreamReq{
		method: "POST",
		url:    antigravityProdBaseURL + "/v1internal:streamGenerateContent?alt=sse",
		headers: map[string]string{
			"Content-Type":  "application/json",
			"Authorization": "Bearer " + accessToken,
			"User-Agent":    antigravityUserAgent,
		},
		body:              payload,
		originalModel:     originalModel,
		mappedModel:       originalModel,
		isStream:          true,
		startTime:         startTime,
		requestID:         req.GetRequestId(),
		extractor:         &geminiUsageExtractor{},
		forceCacheBilling: req.GetForceCacheBilling(),
	})
}

// forwardGeminiProtocol forwards a Gemini native request.
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
		return fmt.Errorf(
			"gateway-antigravity: unsupported account type %q for gemini protocol",
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

	originalModel := req.GetModel()
	payload := buildAntigravityForwardPayload(
		strings.TrimSpace(creds.ProjectID), originalModel, req.GetRawBody())

	action := req.GetGeminiAction()
	if action == "" {
		action = "streamGenerateContent"
	}

	return s.upstreamClient.proxyRequest(ctx, stream, upstreamReq{
		method: "POST",
		url:    antigravityProdBaseURL + "/v1internal:" + action + "?alt=sse",
		headers: map[string]string{
			"Content-Type":  "application/json",
			"Authorization": "Bearer " + accessToken,
			"User-Agent":    antigravityUserAgent,
		},
		body:              payload,
		originalModel:     originalModel,
		mappedModel:       originalModel,
		isStream:          true,
		startTime:         startTime,
		requestID:         req.GetRequestId(),
		extractor:         &geminiUsageExtractor{},
		forceCacheBilling: req.GetForceCacheBilling(),
	})
}

// forwardGeminiAPIKey forwards a Gemini request using an API key.
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

	originalModel := req.GetModel()
	mappedModel := resolveModelMapping(originalModel, req.GetAccount())

	action := req.GetGeminiAction()
	if action == "" {
		if req.GetStream() {
			action = "streamGenerateContent"
		} else {
			action = "generateContent"
		}
	}

	apiURL := fmt.Sprintf("%s/v1beta/models/%s:%s",
		strings.TrimSuffix(baseURL, "/"), mappedModel, action)
	if req.GetStream() || action == "streamGenerateContent" {
		apiURL += "?alt=sse"
	}

	return s.upstreamClient.proxyRequest(ctx, stream, upstreamReq{
		method: "POST",
		url:    apiURL,
		headers: map[string]string{
			"Content-Type":   "application/json",
			"x-goog-api-key": apiKey,
		},
		body:              req.GetRawBody(),
		originalModel:     originalModel,
		mappedModel:       mappedModel,
		isStream:          req.GetStream(),
		startTime:         startTime,
		requestID:         req.GetRequestId(),
		extractor:         &geminiUsageExtractor{},
		clientHeaders:     collectClientHeaders(req),
		forceCacheBilling: req.GetForceCacheBilling(),
	})
}

// --- request helpers ---

// buildAntigravityForwardPayload wraps a request body in the
// Antigravity v1internal envelope format.
func buildAntigravityForwardPayload(projectID, model string, body []byte) []byte {
	wrapper := map[string]any{
		"model": model,
	}
	if projectID != "" {
		wrapper["project"] = projectID
	}

	var inner any
	if err := json.Unmarshal(body, &inner); err != nil {
		wrapper["request"] = string(body)
	} else {
		wrapper["request"] = inner
	}

	result, _ := json.Marshal(wrapper)
	return result
}

// collectClientHeaders combines host-curated headers and raw client_headers.
func collectClientHeaders(req *pb.GatewayForwardRequest) map[string]string {
	merged := make(map[string]string)
	for k, v := range req.GetHeaders() {
		merged[k] = v
	}
	for k, v := range req.GetClientHeaders() {
		merged[k] = v
	}
	return merged
}

// resolveModelMapping applies model_mapping from account credentials.
func resolveModelMapping(requestedModel string, acct *pb.GatewayAccountInfo) string {
	return gatewayutil.ResolveModelMapping(
		requestedModel,
		acct.GetCredentialsJson(),
		[]string{accountTypeAPIKey, accountTypeUpstream},
		acct.GetAccountType(),
	)
}
