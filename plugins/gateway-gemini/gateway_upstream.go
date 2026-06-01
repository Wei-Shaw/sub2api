package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/plugin-sdk/gatewayutil"
	pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
	"google.golang.org/grpc"
)

// defaultHTTPClient is a shared HTTP client tuned for streaming upstream
// requests. See gatewayutil.NewStreamingHTTPClient for tuning rationale.
var defaultHTTPClient = gatewayutil.NewStreamingHTTPClient()

// upstreamRequestInfo holds a fully-built HTTP request ready to send to
// the upstream, along with model metadata needed for billing.
type upstreamRequestInfo struct {
	httpReq       *http.Request
	originalModel string // model as requested by the client (for billing)
	mappedModel   string // model after mapping (actually sent upstream)
}

// buildUpstreamAPIKeyRequest constructs the HTTP request for an API key
// account targeting the AI Studio REST API.
func buildUpstreamAPIKeyRequest(
	ctx context.Context,
	req *pb.GatewayForwardRequest,
	creds *geminiCredentials,
) (*upstreamRequestInfo, error) {
	apiKey := strings.TrimSpace(creds.APIKey)
	if apiKey == "" {
		return nil, fmt.Errorf("no API key available")
	}

	baseURL := strings.TrimSpace(creds.BaseURL)
	if baseURL == "" {
		baseURL = aiStudioBaseURL
	}

	originalModel := req.GetModel()
	mappedModel := resolveModelMapping(originalModel, req.GetAccount())

	action, apiURL := buildGeminiAPIURL(
		baseURL, mappedModel, req.GetGeminiAction(), req.GetStream(),
	)

	body := req.GetRawBody()
	if mappedModel != originalModel {
		body = gatewayutil.ReplaceModelInBody(body, mappedModel)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-goog-api-key", apiKey)

	forwardClientHeaders(httpReq, req.GetHeaders())
	forwardClientHeaders(httpReq, req.GetClientHeaders())

	isStreaming := req.GetStream() || action == "streamGenerateContent"
	_ = isStreaming // used by caller to determine response processing

	return &upstreamRequestInfo{
		httpReq:       httpReq,
		originalModel: originalModel,
		mappedModel:   mappedModel,
	}, nil
}

// buildUpstreamOAuthAIStudioRequest constructs the HTTP request for an
// OAuth account targeting AI Studio (no project_id).
func buildUpstreamOAuthAIStudioRequest(
	ctx context.Context,
	req *pb.GatewayForwardRequest,
	creds *geminiCredentials,
	accessToken string,
) (*upstreamRequestInfo, error) {
	baseURL := strings.TrimSpace(creds.BaseURL)
	if baseURL == "" {
		baseURL = aiStudioBaseURL
	}

	originalModel := req.GetModel()
	mappedModel := resolveModelMapping(originalModel, req.GetAccount())

	_, apiURL := buildGeminiAPIURL(
		baseURL, mappedModel, req.GetGeminiAction(), req.GetStream(),
	)

	body := req.GetRawBody()
	if mappedModel != originalModel {
		body = gatewayutil.ReplaceModelInBody(body, mappedModel)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+accessToken)

	forwardClientHeaders(httpReq, req.GetHeaders())
	forwardClientHeaders(httpReq, req.GetClientHeaders())

	return &upstreamRequestInfo{
		httpReq:       httpReq,
		originalModel: originalModel,
		mappedModel:   mappedModel,
	}, nil
}

// buildUpstreamCodeAssistRequest constructs the HTTP request for an
// OAuth account targeting the Code Assist v1internal endpoint.
func buildUpstreamCodeAssistRequest(
	ctx context.Context,
	req *pb.GatewayForwardRequest,
	accessToken, projectID string,
) (*upstreamRequestInfo, error) {
	action := req.GetGeminiAction()
	if action == "" {
		action = "streamGenerateContent"
	}
	apiURL := geminiCLIBaseURL + "/v1internal:" + action + "?alt=sse"

	originalModel := req.GetModel()
	payload := buildCodeAssistPayload(projectID, originalModel, req.GetRawBody())

	httpReq, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+accessToken)
	httpReq.Header.Set("User-Agent", geminiCLIUserAgent)

	return &upstreamRequestInfo{
		httpReq:       httpReq,
		originalModel: originalModel,
		mappedModel:   originalModel, // no mapping for Code Assist
	}, nil
}

// buildUpstreamVertexRequest constructs the HTTP request for a service
// account targeting Vertex AI.
func buildUpstreamVertexRequest(
	ctx context.Context,
	req *pb.GatewayForwardRequest,
	creds *geminiCredentials,
	accessToken string,
) (*upstreamRequestInfo, error) {
	originalModel := req.GetModel()
	mappedModel := resolveModelMapping(originalModel, req.GetAccount())

	apiURL, err := buildVertexForwardURL(
		creds, mappedModel, req.GetGeminiAction(), req.GetStream(),
	)
	if err != nil {
		return nil, err
	}

	body := req.GetRawBody()
	if mappedModel != originalModel {
		body = gatewayutil.ReplaceModelInBody(body, mappedModel)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+accessToken)

	forwardClientHeaders(httpReq, req.GetHeaders())
	forwardClientHeaders(httpReq, req.GetClientHeaders())

	return &upstreamRequestInfo{
		httpReq:       httpReq,
		originalModel: originalModel,
		mappedModel:   mappedModel,
	}, nil
}

// --- model mapping ---

// resolveModelMapping applies model_mapping from account credentials.
// Only API key and service account types support explicit mappings.
func resolveModelMapping(
	requestedModel string,
	info *pb.GatewayAccountInfo,
) string {
	if info == nil {
		return requestedModel
	}
	return gatewayutil.ResolveModelMapping(
		requestedModel,
		info.GetCredentialsJson(),
		[]string{accountTypeAPIKey, accountTypeServiceAccount},
		info.GetAccountType(),
	)
}

// --- client header forwarding ---

// clientHeadersAllowList lists headers from the client that may be
// forwarded to the upstream Gemini API. Auth headers (authorization,
// x-goog-api-key) are always overwritten by the request builders.
var clientHeadersAllowList = map[string]bool{
	"content-type":      true,
	"user-agent":        true,
	"accept":            true,
	"accept-encoding":   true,
	"accept-language":   true,
	"x-goog-api-client": true,
}

// forwardClientHeaders copies allowed client headers to the upstream
// request. Only sets headers not already present.
func forwardClientHeaders(httpReq *http.Request, clientHeaders map[string]string) {
	for key, value := range clientHeaders {
		lower := strings.ToLower(key)
		if !clientHeadersAllowList[lower] {
			continue
		}
		// Only set if not already present (auth headers take priority).
		if httpReq.Header.Get(key) == "" {
			httpReq.Header.Set(key, value)
		}
	}
}

// --- done chunk builder ---

// buildDoneChunk constructs the terminal GatewayForwardChunk_Done
// message with usage data from the stream processing result.
func buildDoneChunk(
	resp *http.Response,
	upstream *upstreamRequestInfo,
	result *streamResult,
	isStream bool,
	startTime time.Time,
	streamErr error,
) *pb.GatewayForwardChunk {
	done := &pb.GatewayResponseDone{
		Result: &pb.GatewayForwardResult{
			RequestId:       resp.Header.Get("x-goog-request-id"),
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
		done.Result.CacheReadTokens = result.cacheReadTokens
		done.Result.ImageOutputTokens = result.imageOutputTokens
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

// --- URL builders ---

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

// buildVertexForwardURL constructs the Vertex AI API URL for a Gemini
// model.
func buildVertexForwardURL(
	creds *geminiCredentials,
	model, geminiAction string,
	isStream bool,
) (string, error) {
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

// buildCodeAssistPayload wraps a Gemini request body for the Code
// Assist v1internal endpoint:
//
//	{"model": "<model>", "project": "<projectID>", "request": <body>}
func buildCodeAssistPayload(projectID, model string, body []byte) []byte {
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

// --- helpers ---

// sendDone sends a terminal Done chunk with an optional error message.
func sendDone(
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
