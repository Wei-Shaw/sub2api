package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
)

const (
	// upstreamChatGPTCodexURL is the ChatGPT internal API for OAuth accounts.
	upstreamChatGPTCodexURL = "https://chatgpt.com/backend-api/codex/responses"
	// upstreamOpenAIPlatformURL is the default API key endpoint.
	upstreamOpenAIPlatformURL = "https://api.openai.com/v1/responses"
	// maxResponseBodySize limits error response reads to prevent OOM.
	maxResponseBodySize = 2 << 20 // 2 MB
)

// defaultHTTPClient is a shared HTTP client with sensible timeouts for
// upstream requests. Streaming responses need long timeouts since SSE
// connections stay open until the model finishes generating.
var defaultHTTPClient = &http.Client{
	// No overall timeout — streaming responses can take minutes.
	// Individual read deadlines are handled at the io level.
	Transport: &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
		TLSHandshakeTimeout:   15 * time.Second,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   20,
		IdleConnTimeout:       90 * time.Second,
		ResponseHeaderTimeout: 60 * time.Second,
		ForceAttemptHTTP2:     true,
	},
}

// buildUpstreamHTTPRequest constructs the HTTP request to send to the
// OpenAI upstream API. URL routing, auth headers, and host-specific
// headers are set based on account type (OAuth vs API key).
func buildUpstreamHTTPRequest(
	ctx context.Context,
	req *pb.GatewayForwardRequest,
	acct *decodedAccount,
) (*http.Request, error) {
	targetURL := resolveUpstreamURL(acct, req.GetPath())
	authToken := resolveAuthToken(acct)
	if authToken == "" {
		return nil, fmt.Errorf("no auth token available for account %d", acct.ID)
	}

	body := req.GetRawBody()
	httpReq, err := http.NewRequestWithContext(ctx, "POST", targetURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Authorization", "Bearer "+authToken)
	httpReq.Header.Set("Content-Type", "application/json")

	applyAccountHeaders(httpReq, acct)
	applyPassthroughHeaders(httpReq, req.GetHeaders())

	return httpReq, nil
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

// upstreamPassthroughHeaders is the whitelist of headers forwarded from
// the original client request to the upstream. Only low-risk headers are
// allowed to avoid leaking environment noise or triggering upstream
// rate-limiting heuristics.
var upstreamPassthroughHeaders = map[string]bool{
	"accept":          true,
	"accept-language": true,
	"content-type":    true,
	"user-agent":      true,
	"openai-beta":     true,
}

// applyPassthroughHeaders forwards whitelisted headers from the original
// request to the upstream.
func applyPassthroughHeaders(req *http.Request, headers map[string]string) {
	for k, v := range headers {
		if upstreamPassthroughHeaders[strings.ToLower(k)] {
			// Don't override headers already set by applyAccountHeaders
			if req.Header.Get(k) == "" {
				req.Header.Set(k, v)
			}
		}
	}
}
