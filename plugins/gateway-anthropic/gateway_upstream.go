package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/plugin-sdk/gatewayutil"
	pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
)

// errBedrockAccount is returned by resolveForwardAuth when the account is
// a Bedrock type, signalling that the caller should use forwardBedrock
// instead of the standard Anthropic Messages API path.
var errBedrockAccount = errors.New("bedrock account")

// errVertexAccount is returned by resolveForwardAuth when the account is
// a Vertex (service_account) type, signalling that the caller should use
// forwardVertex instead of the standard Anthropic Messages API path.
var errVertexAccount = errors.New("vertex account")

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
		body = gatewayutil.ReplaceModelInBody(body, mappedModel)
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

	// Forward allowed client headers from both the host-curated `headers` map
	// and the raw `client_headers` map. The host populates `headers` with
	// already-filtered upstream headers; `client_headers` carries the full
	// downstream set so we apply the allowlist ourselves.
	forwardClientHeaders(httpReq, req.GetHeaders())
	forwardClientHeaders(httpReq, req.GetClientHeaders())

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
		// Bedrock uses a dedicated forwarding path (forwardBedrock) that
		// bypasses the standard Anthropic Messages API flow. Return a
		// sentinel error so buildUpstreamRequest callers know to branch.
		return "", "", false, errBedrockAccount

	case accountTypeServiceAccount:
		// Vertex AI uses a dedicated forwarding path (forwardVertex) that
		// handles Google OAuth2 token exchange and Vertex URL construction.
		return "", "", false, errVertexAccount

	default:
		return "", "", false, fmt.Errorf("unsupported account type: %s", accountType)
	}
	return authToken, apiURL, useBearer, nil
}

// resolveModel applies model mapping from account credentials.
// For API key and Bedrock accounts, explicit model_mapping is checked.
// For Vertex (service_account), resolveVertexModel handles both mapping
// and Vertex date normalization (hyphen -> @).
// For OAuth/setup-token, the model is passed through unchanged
// (host-side mimicry and NormalizeModelID are not replicated here).
func resolveModel(requestedModel, accountType string, credentialsJSON []byte) string {
	if accountType == accountTypeServiceAccount {
		return resolveVertexModel(requestedModel, credentialsJSON)
	}
	if accountType != accountTypeAPIKey && accountType != accountTypeBedrock {
		return requestedModel
	}
	mapping := gatewayutil.ExtractModelMapping(credentialsJSON)
	if len(mapping) == 0 {
		return requestedModel
	}
	if mapped, ok := mapping[requestedModel]; ok {
		return mapped
	}
	return requestedModel
}

// clientHeadersAllowList lists headers from the client that may be forwarded
// to the upstream Anthropic API. Matches the core's allowedHeaders whitelist.
// Auth headers (authorization, x-api-key, cookie) are always overwritten by
// buildUpstreamRequest and excluded from forwarding.
var clientHeadersAllowList = map[string]bool{
	"anthropic-beta":                            true,
	"anthropic-version":                         true,
	"anthropic-dangerous-direct-browser-access": true,
	"content-type":                              true,
	"user-agent":                                true,
	"accept-encoding":                           true,
	"accept-language":                           true,
	"sec-fetch-mode":                            true,
	"x-stainless-lang":                          true,
	"x-stainless-package-version":               true,
	"x-stainless-os":                            true,
	"x-stainless-arch":                          true,
	"x-stainless-runtime":                       true,
	"x-stainless-runtime-version":               true,
	"x-stainless-helper-method":                 true,
	"x-app":                                     true,
	"x-claude-code-session-id":                  true,
	"x-client-request-id":                       true,
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
			merged := gatewayutil.MergeBetaHeaders(existing, value)
			httpReq.Header.Set("anthropic-beta", merged)
			continue
		}
		// For other allowed headers, only set if not already present.
		if httpReq.Header.Get(key) == "" {
			httpReq.Header.Set(key, value)
		}
	}
}
