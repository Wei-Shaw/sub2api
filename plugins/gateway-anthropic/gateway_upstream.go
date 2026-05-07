package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
)

// --- upstream request construction ---

// upstreamRequest holds a fully-built HTTP request ready to send to the
// Anthropic Messages API, along with metadata the stream processor needs.
type upstreamRequest struct {
	httpReq       *http.Request
	originalModel string // model as requested by the client (for billing)
	mappedModel   string // model after mapping (actually sent upstream)
}

// buildUpstreamRequest constructs the HTTP request to the Anthropic Messages
// API from the gRPC ForwardRequest. It handles:
//   - credential resolution (OAuth Bearer vs API key)
//   - custom base URL
//   - required Anthropic headers
//   - model mapping from credentials
func buildUpstreamRequest(
	ctx context.Context,
	req *pb.GatewayForwardRequest,
) (*upstreamRequest, error) {
	acct := req.GetAccount()
	if acct == nil {
		return nil, fmt.Errorf("missing account info")
	}

	creds, err := parseCredentials(acct.GetCredentialsJson())
	if err != nil {
		return nil, fmt.Errorf("parse credentials: %w", err)
	}

	authToken, targetURL, useBearer, err := resolveForwardAuth(acct.GetAccountType(), creds)
	if err != nil {
		return nil, err
	}

	body := req.GetRawBody()
	originalModel := req.GetModel()
	mappedModel := resolveModel(originalModel, acct.GetAccountType(), acct.GetCredentialsJson())

	// Replace model in body if mapping changed it.
	if mappedModel != originalModel {
		body = replaceModelInBody(body, mappedModel)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build http request: %w", err)
	}

	// Set authentication.
	if useBearer {
		httpReq.Header.Set("Authorization", "Bearer "+authToken)
	} else {
		httpReq.Header.Set("x-api-key", authToken)
	}

	// Set required Anthropic headers.
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("anthropic-version", anthropicAPIVersion)

	// Set beta header based on account type.
	if useBearer {
		httpReq.Header.Set("anthropic-beta", defaultBetaHeader)
	} else {
		httpReq.Header.Set("anthropic-beta", apiKeyBetaHeader)
	}

	// Forward relevant client headers (anthropic-beta from client can
	// augment the defaults; content-type and auth are already set above).
	forwardClientHeaders(httpReq, req.GetHeaders())

	return &upstreamRequest{
		httpReq:       httpReq,
		originalModel: originalModel,
		mappedModel:   mappedModel,
	}, nil
}

// resolveForwardAuth determines auth credentials for the forward path.
// Unlike the test-connection path, this also supports setup-token accounts.
func resolveForwardAuth(
	accountType string, creds *anthropicCredentials,
) (authToken, apiURL string, useBearer bool, err error) {
	switch accountType {
	case accountTypeOAuth, accountTypeSetupToken:
		useBearer = true
		authToken = strings.TrimSpace(creds.AccessToken)
		if authToken == "" {
			return "", "", false, fmt.Errorf("no access_token for %s account", accountType)
		}
		apiURL = anthropicMessagesURL

	case accountTypeAPIKey:
		authToken = strings.TrimSpace(creds.APIKey)
		if authToken == "" {
			return "", "", false, fmt.Errorf("no api_key for apikey account")
		}
		baseURL := strings.TrimSpace(creds.BaseURL)
		if baseURL == "" {
			baseURL = "https://api.anthropic.com"
		}
		apiURL = strings.TrimSuffix(baseURL, "/") + "/v1/messages?beta=true"

	case accountTypeBedrock:
		return "", "", false, fmt.Errorf("bedrock forwarding not yet implemented in plugin")

	default:
		return "", "", false, fmt.Errorf("unsupported account type: %s", accountType)
	}
	return authToken, apiURL, useBearer, nil
}

// resolveModel applies model mapping from account credentials.
// For API key accounts, explicit model_mapping is checked.
// For OAuth/setup-token, the model is passed through unchanged
// (host-side mimicry and NormalizeModelID are not replicated here).
func resolveModel(requestedModel, accountType string, credentialsJSON []byte) string {
	if accountType != accountTypeAPIKey {
		return requestedModel
	}
	mapping := extractModelMapping(credentialsJSON)
	if len(mapping) == 0 {
		return requestedModel
	}
	if mapped, ok := mapping[requestedModel]; ok {
		return mapped
	}
	return requestedModel
}

// replaceModelInBody replaces the "model" field in the JSON body with the
// mapped model name.
func replaceModelInBody(body []byte, newModel string) []byte {
	var parsed map[string]json.RawMessage
	if err := json.Unmarshal(body, &parsed); err != nil {
		return body // best effort
	}
	modelBytes, err := json.Marshal(newModel)
	if err != nil {
		return body
	}
	parsed["model"] = modelBytes
	result, err := json.Marshal(parsed)
	if err != nil {
		return body
	}
	return result
}

// clientHeadersAllowList lists headers from the client that may be forwarded
// to the upstream Anthropic API. Auth and content-type are always set by
// buildUpstreamRequest and are excluded from forwarding.
var clientHeadersAllowList = map[string]bool{
	"anthropic-beta":    true,
	"anthropic-version": true,
}

// forwardClientHeaders copies allowed client headers to the upstream request.
// It merges anthropic-beta rather than overwriting.
func forwardClientHeaders(httpReq *http.Request, clientHeaders map[string]string) {
	for key, value := range clientHeaders {
		lower := strings.ToLower(key)
		if !clientHeadersAllowList[lower] {
			continue
		}
		if lower == "anthropic-beta" {
			// Merge client betas with defaults (avoid duplicates).
			existing := httpReq.Header.Get("anthropic-beta")
			merged := mergeBetaHeaders(existing, value)
			httpReq.Header.Set("anthropic-beta", merged)
			continue
		}
		// For other allowed headers, only set if not already present.
		if httpReq.Header.Get(key) == "" {
			httpReq.Header.Set(key, value)
		}
	}
}

// mergeBetaHeaders merges two comma-separated beta header values, deduplicating.
func mergeBetaHeaders(existing, additional string) string {
	if existing == "" {
		return additional
	}
	if additional == "" {
		return existing
	}
	seen := make(map[string]struct{})
	var parts []string
	for _, part := range strings.Split(existing, ",") {
		t := strings.TrimSpace(part)
		if t == "" {
			continue
		}
		if _, ok := seen[t]; !ok {
			seen[t] = struct{}{}
			parts = append(parts, t)
		}
	}
	for _, part := range strings.Split(additional, ",") {
		t := strings.TrimSpace(part)
		if t == "" {
			continue
		}
		if _, ok := seen[t]; !ok {
			seen[t] = struct{}{}
			parts = append(parts, t)
		}
	}
	return strings.Join(parts, ",")
}
