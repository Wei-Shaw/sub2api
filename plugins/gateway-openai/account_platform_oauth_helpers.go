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

// maxOAuthResponseBody limits response body reads to prevent OOM.
const maxOAuthResponseBody = 1 << 20 // 1 MB

// openaiTokenResponse represents the token response from auth.openai.com.
type openaiFullTokenResponse struct {
	AccessToken  string `json:"access_token"`
	IDToken      string `json:"id_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
	RefreshToken string `json:"refresh_token,omitempty"`
	Scope        string `json:"scope,omitempty"`
}

// ---------- HTTP helpers ----------

// exchangeOpenAICode calls auth.openai.com/oauth/token with an authorization
// code and PKCE code verifier, using form-encoded POST.
func exchangeOpenAICode(
	ctx context.Context,
	client *http.Client,
	code, codeVerifier, redirectURI string,
) (*openaiFullTokenResponse, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("client_id", openaiDefaultCIDOAuth)
	form.Set("code", code)
	form.Set("redirect_uri", redirectURI)
	form.Set("code_verifier", codeVerifier)

	return postOpenAIToken(ctx, client, form)
}

// postOpenAIToken sends a form-encoded POST to the OpenAI token endpoint.
func postOpenAIToken(
	ctx context.Context,
	client *http.Client,
	form url.Values,
) (*openaiFullTokenResponse, error) {
	httpReq, err := http.NewRequestWithContext(
		ctx, http.MethodPost, openaiTokenURL,
		strings.NewReader(form.Encode()),
	)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	httpReq.Header.Set("User-Agent", "codex-cli/0.91.0")

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxOAuthResponseBody))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, gatewayutil.TruncateBody(body))
	}

	var tokenResp openaiFullTokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	return &tokenResp, nil
}

// ---------- credential builders ----------

// buildOpenAITokenCredentials converts a token response into the credential
// map format expected by the host.
func buildOpenAITokenCredentials(tok *openaiFullTokenResponse) map[string]any {
	creds := map[string]any{
		"access_token": tok.AccessToken,
		"expires_at":   time.Now().Add(time.Duration(tok.ExpiresIn) * time.Second).Format(time.RFC3339),
		"client_id":    openaiDefaultCIDOAuth,
	}
	if tok.RefreshToken != "" {
		creds["refresh_token"] = tok.RefreshToken
	}
	if tok.IDToken != "" {
		creds["id_token"] = tok.IDToken
		enrichFromIDToken(creds, tok.IDToken)
	}
	return creds
}

// buildValidateRefreshCredentials builds the credential map for a
// validate-refresh-token response, preserving the original refresh token
// if the endpoint didn't return a new one.
func buildValidateRefreshCredentials(
	originalRefresh string, tok *openaiTokenResponse,
) map[string]any {
	creds := map[string]any{
		"access_token":  tok.AccessToken,
		"refresh_token": originalRefresh,
		"expires_at":    time.Now().Add(time.Duration(tok.ExpiresIn) * time.Second).Format(time.RFC3339),
		"client_id":     openaiDefaultCIDOAuth,
	}
	if strings.TrimSpace(tok.RefreshToken) != "" {
		creds["refresh_token"] = tok.RefreshToken
	}
	if tok.IDToken != "" {
		creds["id_token"] = tok.IDToken
		enrichFromIDToken(creds, tok.IDToken)
	}
	return creds
}

// enrichFromIDToken extracts user info from an OpenAI ID token JWT
// and adds it to the credentials map (best-effort, no signature validation).
func enrichFromIDToken(creds map[string]any, idToken string) {
	claims, err := decodeIDTokenPayload(idToken)
	if err != nil {
		return
	}
	if email, _ := claims["email"].(string); email != "" {
		creds["email"] = email
	}

	auth, _ := claims["https://api.openai.com/auth"].(map[string]any)
	if auth == nil {
		return
	}
	if v, _ := auth["chatgpt_account_id"].(string); v != "" {
		creds["chatgpt_account_id"] = v
	}
	if v, _ := auth["chatgpt_user_id"].(string); v != "" {
		creds["chatgpt_user_id"] = v
	}
	if v, _ := auth["chatgpt_plan_type"].(string); v != "" {
		creds["plan_type"] = v
	}
	if v, _ := auth["user_id"].(string); v != "" {
		creds["user_id"] = v
	}

	// Extract default organization ID.
	if orgs, ok := auth["organizations"].([]any); ok {
		for _, orgRaw := range orgs {
			org, _ := orgRaw.(map[string]any)
			if org == nil {
				continue
			}
			if isDefault, _ := org["is_default"].(bool); isDefault {
				if id, _ := org["id"].(string); id != "" {
					creds["organization_id"] = id
				}
				break
			}
		}
		// Fallback to first org if no default found.
		if _, ok := creds["organization_id"]; !ok && len(orgs) > 0 {
			if org, _ := orgs[0].(map[string]any); org != nil {
				if id, _ := org["id"].(string); id != "" {
					creds["organization_id"] = id
				}
			}
		}
	}
}

// decodeIDTokenPayload decodes the payload (second segment) of a JWT
// without validating the signature. Returns the raw claims map.
func decodeIDTokenPayload(jwt string) (map[string]any, error) {
	parts := strings.Split(jwt, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid JWT: expected 3 parts, got %d", len(parts))
	}

	payload := parts[1]
	// Add base64 padding if necessary.
	switch len(payload) % 4 {
	case 2:
		payload += "=="
	case 3:
		payload += "="
	}

	decoded, err := base64.URLEncoding.DecodeString(payload)
	if err != nil {
		decoded, err = base64.StdEncoding.DecodeString(payload)
		if err != nil {
			return nil, fmt.Errorf("decode JWT payload: %w", err)
		}
	}

	var claims map[string]any
	if err := json.Unmarshal(decoded, &claims); err != nil {
		return nil, fmt.Errorf("parse JWT claims: %w", err)
	}
	return claims, nil
}

// deriveOpenAIAccountName builds a human-readable account name from the
// token response, preferring the email from the ID token.
func deriveOpenAIAccountName(tok *openaiFullTokenResponse) string {
	if tok.IDToken == "" {
		return ""
	}
	claims, err := decodeIDTokenPayload(tok.IDToken)
	if err != nil {
		return ""
	}
	if email, _ := claims["email"].(string); email != "" {
		return email
	}
	return ""
}

// ---------- PKCE / crypto helpers ----------

// generateRandomHex generates n random bytes and returns them as hex-encoded
// string. OpenAI uses hex encoding (not base64url) for state and code verifier.
func generateRandomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// computeCodeChallenge computes the S256 PKCE code challenge from a verifier.
// Uses base64url encoding without padding as per RFC 7636.
func computeCodeChallenge(verifier string) string {
	hash := sha256.Sum256([]byte(verifier))
	return strings.TrimRight(base64.URLEncoding.EncodeToString(hash[:]), "=")
}

// buildOpenAIAuthorizationURL constructs the OpenAI OAuth authorization URL.
func buildOpenAIAuthorizationURL(state, codeChallenge, redirectURI string) string {
	params := url.Values{}
	params.Set("response_type", "code")
	params.Set("client_id", openaiDefaultCIDOAuth)
	params.Set("redirect_uri", redirectURI)
	params.Set("scope", openaiDefaultScopes)
	params.Set("state", state)
	params.Set("code_challenge", codeChallenge)
	params.Set("code_challenge_method", "S256")
	params.Set("id_token_add_organizations", "true")
	params.Set("codex_cli_simplified_flow", "true")

	return fmt.Sprintf("%s?%s", openaiAuthorizeURL, params.Encode())
}

// ---------- privacy helpers ----------

// disableTraining calls the ChatGPT settings API to turn off "Improve the
// model for everyone". Returns privacy_mode value.
func disableTraining(ctx context.Context, client *http.Client, accessToken string) string {
	if accessToken == "" {
		return ""
	}

	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	reqURL := openAISettingsURL + "?feature=training_allowed&value=false"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPatch, reqURL, nil)
	if err != nil {
		return privacyModeFailed
	}

	httpReq.Header.Set("Authorization", "Bearer "+accessToken)
	httpReq.Header.Set("Origin", "https://chatgpt.com")
	httpReq.Header.Set("Referer", "https://chatgpt.com/")
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("sec-fetch-mode", "cors")
	httpReq.Header.Set("sec-fetch-site", "same-origin")
	httpReq.Header.Set("sec-fetch-dest", "empty")

	resp, err := client.Do(httpReq)
	if err != nil {
		return privacyModeFailed
	}
	defer func() {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxOAuthResponseBody))
		_ = resp.Body.Close()
	}()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return privacyModeTrainingOff
	}
	return privacyModeFailed
}

// extractPrivacyMode extracts the privacy_mode from extra JSON.
func extractPrivacyMode(extraJSON []byte) string {
	if len(extraJSON) == 0 {
		return ""
	}
	var extra map[string]any
	if err := json.Unmarshal(extraJSON, &extra); err != nil {
		return ""
	}
	mode, _ := extra["privacy_mode"].(string)
	return strings.TrimSpace(mode)
}

// shouldSkipPrivacy returns true if the privacy mode indicates a successful
// or non-retryable state (anything except "failed" or "cf_blocked").
func shouldSkipPrivacy(mode string) bool {
	if mode == "" {
		return false
	}
	return mode != privacyModeFailed && mode != "training_set_cf_blocked"
}

// privacyErrorMessage returns a human-readable error for non-success modes.
func privacyErrorMessage(mode string) string {
	if mode == privacyModeTrainingOff {
		return ""
	}
	if mode == privacyModeFailed {
		return "failed to disable training data sharing"
	}
	return "privacy setting failed: " + mode
}
