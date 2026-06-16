package main

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/plugin-sdk/gatewayutil"
	pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
)

const (
	// upstreamChatGPTCodexURL is the ChatGPT internal API for OAuth accounts.
	upstreamChatGPTCodexURL = "https://chatgpt.com/backend-api/codex/responses"
	// upstreamOpenAIPlatformURL is the default API key endpoint.
	upstreamOpenAIPlatformURL = "https://api.openai.com/v1/responses"
)

// maxResponseBodySize is the shared upper bound for upstream response body
// reads. See gatewayutil.MaxResponseBodySize for rationale.
const maxResponseBodySize = gatewayutil.MaxResponseBodySize

// defaultHTTPClient is a shared HTTP client tuned for streaming upstream
// requests. See gatewayutil.NewStreamingHTTPClient for tuning rationale.
var defaultHTTPClient = gatewayutil.NewStreamingHTTPClient()

// upstreamRequestInfo holds a fully-built HTTP request ready to send to
// the upstream, along with model metadata needed for billing and response
// rewriting.
type upstreamRequestInfo struct {
	httpReq       *http.Request
	originalModel string // model as requested by the client (for billing)
	mappedModel   string // model after mapping (actually sent upstream)
}

// buildUpstreamRequest constructs the HTTP request to send to the OpenAI
// upstream API. It handles:
//   - credential resolution (OAuth Bearer vs API key)
//   - custom base URL
//   - model mapping from credentials
//   - client header forwarding (allowlisted)
func buildUpstreamRequest(
	ctx context.Context,
	req *pb.GatewayForwardRequest,
) (*upstreamRequestInfo, error) {
	acct, err := decodeAccountInfo(req.GetAccount())
	if err != nil {
		return nil, fmt.Errorf("decode account: %w", err)
	}

	targetURL := resolveUpstreamURL(acct, req.GetPath())
	authToken := resolveAuthToken(acct)
	if authToken == "" {
		return nil, fmt.Errorf("no auth token for account %d", acct.ID)
	}

	originalModel := req.GetModel()
	mappedModel := resolveModelMapping(originalModel, acct, req.GetAccount())

	body := req.GetRawBody()
	if mappedModel != originalModel {
		body = gatewayutil.ReplaceModelInBody(body, mappedModel)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", targetURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Authorization", "Bearer "+authToken)
	httpReq.Header.Set("Content-Type", "application/json")

	applyAccountHeaders(httpReq, acct)
	forwardClientHeaders(httpReq, req.GetHeaders())
	forwardClientHeaders(httpReq, req.GetClientHeaders())

	return &upstreamRequestInfo{
		httpReq:       httpReq,
		originalModel: originalModel,
		mappedModel:   mappedModel,
	}, nil
}

// resolveUpstreamURL determines the target URL based on account type.
//
// OAuth accounts use the ChatGPT internal Codex endpoint.
// API key accounts use the OpenAI platform API or a custom base_url.
func resolveUpstreamURL(acct *decodedAccount, requestPath string) string {
	switch acct.AccountType {
	case accountTypeOAuth, accountTypeSetupToken:
		return upstreamChatGPTCodexURL
	case accountTypeAPIKey, accountTypeUpstream:
		if acct.BaseURL != "" {
			base := strings.TrimSuffix(acct.BaseURL, "/")
			return base + "/v1/responses"
		}
		return upstreamOpenAIPlatformURL
	default:
		return upstreamOpenAIPlatformURL
	}
}

// resolveAuthToken returns the authentication token for the account.
func resolveAuthToken(acct *decodedAccount) string {
	switch acct.AccountType {
	case accountTypeOAuth, accountTypeSetupToken:
		return acct.AccessToken
	case accountTypeAPIKey, accountTypeUpstream:
		return acct.APIKey
	default:
		if acct.APIKey != "" {
			return acct.APIKey
		}
		return acct.AccessToken
	}
}

// resolveModelMapping applies model_mapping from account credentials.
// API key / upstream accounts may define explicit mappings; OAuth accounts
// pass through unchanged.
func resolveModelMapping(
	requestedModel string,
	acct *decodedAccount,
	info *pb.GatewayAccountInfo,
) string {
	return gatewayutil.ResolveModelMapping(
		requestedModel,
		info.GetCredentialsJson(),
		[]string{accountTypeAPIKey, accountTypeUpstream},
		acct.AccountType,
	)
}

// applyAccountHeaders sets headers specific to the account type.
// OAuth accounts require Host=chatgpt.com and chatgpt-account-id.
func applyAccountHeaders(req *http.Request, acct *decodedAccount) {
	switch acct.AccountType {
	case accountTypeOAuth, accountTypeSetupToken:
		req.Host = "chatgpt.com"
		if acct.ChatGPTAccountID != "" {
			req.Header.Set("chatgpt-account-id", acct.ChatGPTAccountID)
		}
		req.Header.Set("Accept", "text/event-stream")
		req.Header.Set("OpenAI-Beta", "responses=experimental")
	default:
		// API key accounts: Accept defaults to application/json
	}
}

// clientHeadersAllowList lists headers from the client that may be
// forwarded to the upstream OpenAI API. Auth headers (authorization,
// cookie) are always overwritten by buildUpstreamRequest and excluded.
var clientHeadersAllowList = map[string]bool{
	"accept":                true,
	"accept-language":       true,
	"content-type":          true,
	"user-agent":            true,
	"openai-beta":           true,
	"originator":            true,
	"session_id":            true,
	"conversation_id":       true,
	"x-codex-turn-state":    true,
	"x-codex-turn-metadata": true,
	"x-client-request-id":   true,
	"x-request-id":          true,
}

// forwardClientHeaders copies allowed client headers to the upstream
// request. It merges OpenAI-Beta rather than overwriting.
func forwardClientHeaders(httpReq *http.Request, clientHeaders map[string]string) {
	for key, value := range clientHeaders {
		lower := strings.ToLower(key)
		if !clientHeadersAllowList[lower] {
			continue
		}
		if lower == "openai-beta" {
			existing := httpReq.Header.Get("OpenAI-Beta")
			merged := gatewayutil.MergeBetaHeaders(existing, value)
			httpReq.Header.Set("OpenAI-Beta", merged)
			continue
		}
		if httpReq.Header.Get(key) == "" {
			httpReq.Header.Set(key, value)
		}
	}
}
