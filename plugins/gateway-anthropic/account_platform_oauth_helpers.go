package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/Wei-Shaw/sub2api/plugin-sdk/gatewayutil"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// maxResponseBody limits response body reads to prevent OOM.
const maxResponseBody = 1 << 20 // 1 MB

// claudeFullTokenResponse extends claudeTokenResponse with org/account info
// returned by the Claude token endpoint.
type claudeFullTokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
	RefreshToken string `json:"refresh_token,omitempty"`
	Scope        string `json:"scope,omitempty"`
	Organization *struct {
		UUID string `json:"uuid"`
	} `json:"organization,omitempty"`
	Account *struct {
		UUID         string `json:"uuid"`
		EmailAddress string `json:"email_address"`
	} `json:"account,omitempty"`
}

// cookieOAuthResult holds the results of a complete cookie-based OAuth flow.
type cookieOAuthResult struct {
	creds       map[string]any
	accountName string
}

// ---------- multi-step cookie flow ----------

// resolveCookieAuthScope maps the scope string from the RPC to the OAuth scope
// and whether this is a setup-token flow.
func resolveCookieAuthScope(scope string) (oauthScope string, isSetupToken bool) {
	if scope == "inference" {
		return scopeInference, true
	}
	return scopeAPI, false
}

// performCookieOAuthFlow executes the full cookie-based OAuth flow:
// get org -> PKCE -> get auth code -> exchange for token.
func performCookieOAuthFlow(
	ctx context.Context,
	client *http.Client,
	sessionKey, scope string,
	isSetupToken bool,
) (*cookieOAuthResult, error) {
	orgUUID, err := getOrganizationUUID(ctx, client, sessionKey)
	if err != nil {
		return nil, fmt.Errorf("get organization: %w", err)
	}

	codeVerifier, err := generateRandomBase64URL(32)
	if err != nil {
		return nil, fmt.Errorf("generate code verifier: %w", err)
	}
	codeChallenge := computeCodeChallenge(codeVerifier)
	state, err := generateRandomBase64URL(32)
	if err != nil {
		return nil, fmt.Errorf("generate state: %w", err)
	}

	authCode, err := getAuthorizationCode(
		ctx, client, sessionKey, orgUUID, scope, codeChallenge, state,
	)
	if err != nil {
		return nil, fmt.Errorf("get authorization code: %w", err)
	}

	tokenResp, err := exchangeCodeForToken(
		ctx, client, authCode, codeVerifier, state, isSetupToken,
	)
	if err != nil {
		return nil, fmt.Errorf("exchange code: %w", err)
	}

	creds := buildTokenCredentials(tokenResp)
	if _, ok := creds["org_uuid"]; !ok && orgUUID != "" {
		creds["org_uuid"] = orgUUID
	}

	return &cookieOAuthResult{
		creds:       creds,
		accountName: deriveAccountName(tokenResp),
	}, nil
}

// ---------- HTTP helpers ----------

// exchangeCodeForToken calls the Claude token endpoint with an authorization code.
func exchangeCodeForToken(
	ctx context.Context,
	client *http.Client,
	code, codeVerifier, state string,
	isSetupToken bool,
) (*claudeFullTokenResponse, error) {
	authCode, codeState := splitCodeAndState(code)

	reqBody := map[string]any{
		"code":          authCode,
		"grant_type":    "authorization_code",
		"client_id":     claudeClientID,
		"redirect_uri":  claudeRedirectURI,
		"code_verifier": codeVerifier,
	}
	if codeState != "" {
		reqBody["state"] = codeState
	}
	if isSetupToken {
		reqBody["expires_in"] = setupTokenExpiresIn
	}

	return postClaudeToken(ctx, client, reqBody)
}

// splitCodeAndState parses "authCode#state" format.
func splitCodeAndState(code string) (authCode, state string) {
	if idx := strings.Index(code, "#"); idx != -1 {
		return code[:idx], code[idx+1:]
	}
	return code, ""
}

// postClaudeToken sends a POST to the Claude token endpoint and parses the response.
func postClaudeToken(
	ctx context.Context,
	client *http.Client,
	reqBody map[string]any,
) (*claudeFullTokenResponse, error) {
	bodyBytes, _ := json.Marshal(reqBody)
	httpReq, err := http.NewRequestWithContext(
		ctx, http.MethodPost, claudeTokenURL,
		strings.NewReader(string(bodyBytes)),
	)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json, text/plain, */*")
	httpReq.Header.Set("User-Agent", "axios/1.13.6")

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBody))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, gatewayutil.TruncateBody(body))
	}

	var tokenResp claudeFullTokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	return &tokenResp, nil
}

// getOrganizationUUID fetches the user's organization UUID via claude.ai API
// using a session cookie.
func getOrganizationUUID(
	ctx context.Context,
	client *http.Client,
	sessionKey string,
) (string, error) {
	targetURL := claudeBaseURL + "/api/organizations"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	httpReq.AddCookie(&http.Cookie{Name: "sessionKey", Value: sessionKey})

	resp, err := client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBody))
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("status %d: %s", resp.StatusCode, gatewayutil.TruncateBody(body))
	}

	return parseOrganizationUUID(body)
}

// parseOrganizationUUID extracts the preferred org UUID from the organizations
// API response. Prefers team organizations when multiple exist.
func parseOrganizationUUID(body []byte) (string, error) {
	var orgs []struct {
		UUID      string  `json:"uuid"`
		Name      string  `json:"name"`
		RavenType *string `json:"raven_type"`
	}
	if err := json.Unmarshal(body, &orgs); err != nil {
		return "", fmt.Errorf("parse response: %w", err)
	}
	if len(orgs) == 0 {
		return "", fmt.Errorf("no organizations found")
	}

	if len(orgs) > 1 {
		for _, org := range orgs {
			if org.RavenType != nil && *org.RavenType == "team" {
				return org.UUID, nil
			}
		}
	}
	return orgs[0].UUID, nil
}

// getAuthorizationCode uses a session cookie to obtain an authorization code
// from the Claude OAuth authorize endpoint.
func getAuthorizationCode(
	ctx context.Context,
	client *http.Client,
	sessionKey, orgUUID, scope, codeChallenge, state string,
) (string, error) {
	authURL := fmt.Sprintf("%s/v1/oauth/%s/authorize", claudeBaseURL, orgUUID)

	reqBody := map[string]any{
		"response_type":         "code",
		"client_id":             claudeClientID,
		"organization_uuid":     orgUUID,
		"redirect_uri":          claudeRedirectURI,
		"scope":                 scope,
		"state":                 state,
		"code_challenge":        codeChallenge,
		"code_challenge_method": "S256",
	}
	bodyBytes, _ := json.Marshal(reqBody)

	httpReq, err := http.NewRequestWithContext(
		ctx, http.MethodPost, authURL,
		strings.NewReader(string(bodyBytes)),
	)
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	httpReq.AddCookie(&http.Cookie{Name: "sessionKey", Value: sessionKey})
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("Accept-Language", "en-US,en;q=0.9")
	httpReq.Header.Set("Cache-Control", "no-cache")
	httpReq.Header.Set("Origin", "https://claude.ai")
	httpReq.Header.Set("Referer", "https://claude.ai/new")
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBody))
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("status %d: %s", resp.StatusCode, gatewayutil.TruncateBody(body))
	}

	return parseRedirectCode(body)
}

// parseRedirectCode extracts the authorization code (and optional state)
// from the authorize endpoint's JSON redirect_uri response.
func parseRedirectCode(body []byte) (string, error) {
	var result struct {
		RedirectURI string `json:"redirect_uri"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("parse response: %w", err)
	}
	if result.RedirectURI == "" {
		return "", fmt.Errorf("no redirect_uri in response")
	}

	parsedURL, err := url.Parse(result.RedirectURI)
	if err != nil {
		return "", fmt.Errorf("parse redirect_uri: %w", err)
	}

	authCode := parsedURL.Query().Get("code")
	responseState := parsedURL.Query().Get("state")
	if authCode == "" {
		return "", fmt.Errorf("no authorization code in redirect_uri")
	}

	if responseState != "" {
		return authCode + "#" + responseState, nil
	}
	return authCode, nil
}

// ---------- credential builders ----------

// buildTokenCredentials converts a token response into the credential map
// format expected by the host.
func buildTokenCredentials(tok *claudeFullTokenResponse) map[string]any {
	creds := map[string]any{
		"access_token": tok.AccessToken,
		"token_type":   tok.TokenType,
		"expires_in":   tok.ExpiresIn,
		"expires_at":   time.Now().Unix() + tok.ExpiresIn,
	}
	if tok.RefreshToken != "" {
		creds["refresh_token"] = tok.RefreshToken
	}
	if tok.Scope != "" {
		creds["scope"] = tok.Scope
	}
	if tok.Organization != nil && tok.Organization.UUID != "" {
		creds["org_uuid"] = tok.Organization.UUID
	}
	if tok.Account != nil {
		if tok.Account.UUID != "" {
			creds["account_uuid"] = tok.Account.UUID
		}
		if tok.Account.EmailAddress != "" {
			creds["email_address"] = tok.Account.EmailAddress
		}
	}
	return creds
}

// buildRefreshCredentials builds the credential map for a validate-refresh-token
// response, preserving the original refresh token if the endpoint didn't return
// a new one.
func buildRefreshCredentials(
	originalRefresh string, tok *claudeTokenResponse,
) map[string]any {
	creds := map[string]any{
		"access_token":  tok.AccessToken,
		"refresh_token": originalRefresh,
		"expires_at":    time.Now().Add(time.Duration(tok.ExpiresIn) * time.Second).Format(time.RFC3339),
	}
	if strings.TrimSpace(tok.RefreshToken) != "" {
		creds["refresh_token"] = tok.RefreshToken
	}
	if tok.TokenType != "" {
		creds["token_type"] = tok.TokenType
	}
	if tok.Scope != "" {
		creds["scope"] = tok.Scope
	}
	return creds
}

// deriveAccountName builds a human-readable account name from the token response.
func deriveAccountName(tok *claudeFullTokenResponse) string {
	if tok.Account != nil && tok.Account.EmailAddress != "" {
		return tok.Account.EmailAddress
	}
	return ""
}

// ---------- PKCE / crypto helpers ----------

// generateRandomBase64URL generates n random bytes and returns them as
// base64url-no-pad (RFC 7636).
func generateRandomBase64URL(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return strings.TrimRight(base64.URLEncoding.EncodeToString(b), "="), nil
}

// generateSessionID generates a hex-encoded 16-byte random session ID.
func generateSessionID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// computeCodeChallenge computes the S256 PKCE code challenge from a verifier.
func computeCodeChallenge(verifier string) string {
	hash := sha256.Sum256([]byte(verifier))
	return strings.TrimRight(base64.URLEncoding.EncodeToString(hash[:]), "=")
}

// buildAuthorizationURL constructs the Claude OAuth authorization URL with
// the correct parameter format.
func buildAuthorizationURL(state, codeChallenge, scope string) string {
	encodedRedirectURI := url.QueryEscape(claudeRedirectURI)
	encodedScope := strings.ReplaceAll(url.QueryEscape(scope), "%20", "+")

	return fmt.Sprintf(
		"%s?code=true&client_id=%s&response_type=code&redirect_uri=%s&scope=%s&code_challenge=%s&code_challenge_method=S256&state=%s",
		claudeAuthorizeURL,
		claudeClientID,
		encodedRedirectURI,
		encodedScope,
		codeChallenge,
		state,
	)
}
