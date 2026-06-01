package main

import (
	"bytes"
	"context"
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

// loadCodeAssist IDE metadata constants.
const (
	loadCodeAssistIDEType    = "ANTIGRAVITY"
	loadCodeAssistIDEVersion = "1.20.6"
	loadCodeAssistIDEName    = "antigravity"
)

// loadCodeAssistURLs are the Antigravity API endpoints for loadCodeAssist
// (prod first, daily sandbox fallback).
var loadCodeAssistURLs = []string{
	"https://cloudcode-pa.googleapis.com/v1internal:loadCodeAssist",
	"https://daily-cloudcode-pa.sandbox.googleapis.com/v1internal:loadCodeAssist",
}

// ---------- Token exchange ----------

// exchangeAntigravityCode calls the Google OAuth token endpoint to exchange
// an authorization code for tokens.
func exchangeAntigravityCode(
	ctx context.Context, client *http.Client,
	code, codeVerifier string,
) (*googleTokenResponse, error) {
	form := url.Values{}
	form.Set("client_id", googleClientID)
	form.Set("client_secret", antigravityDefaultClientSecret)
	form.Set("code", code)
	form.Set("redirect_uri", antigravityRedirectURI)
	form.Set("grant_type", "authorization_code")
	form.Set("code_verifier", codeVerifier)

	return postGoogleToken(ctx, client, form)
}

// refreshAntigravityToken calls the Google OAuth token endpoint with
// grant_type=refresh_token. clientSecret is optional; defaults to the
// built-in Antigravity OAuth client secret.
func refreshAntigravityToken(
	ctx context.Context, client *http.Client,
	refreshToken, clientSecret string,
) (*googleTokenResponse, error) {
	if clientSecret == "" {
		clientSecret = antigravityDefaultClientSecret
	}

	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refreshToken)
	form.Set("client_id", googleClientID)
	form.Set("client_secret", clientSecret)

	return postGoogleToken(ctx, client, form)
}

// postGoogleToken sends a form-encoded POST to the Google token endpoint.
func postGoogleToken(
	ctx context.Context, client *http.Client,
	form url.Values,
) (*googleTokenResponse, error) {
	httpReq, err := http.NewRequestWithContext(
		ctx, http.MethodPost, antigravityOAuthTokenURL,
		strings.NewReader(form.Encode()),
	)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

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

	var tokenResp googleTokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	return &tokenResp, nil
}

// ---------- User info ----------

// fetchAntigravityEmail fetches the user's email via the Google userinfo API.
// Returns empty string on any error (best-effort).
func fetchAntigravityEmail(
	ctx context.Context, client *http.Client, accessToken string,
) string {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, antigravityUserInfoURL, nil)
	if err != nil {
		return ""
	}
	httpReq.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := client.Do(httpReq)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBody))
	if err != nil || resp.StatusCode != http.StatusOK {
		return ""
	}

	var info struct {
		Email string `json:"email"`
	}
	if json.Unmarshal(body, &info) != nil {
		return ""
	}
	return info.Email
}

// ---------- LoadCodeAssist (project_id + plan_type) ----------

// loadAntigravityProjectInfo calls the Antigravity loadCodeAssist endpoint
// to fetch project_id and plan_type. Returns empty strings on failure.
func loadAntigravityProjectInfo(
	ctx context.Context, client *http.Client, accessToken string,
) (projectID, planType string) {
	reqBody := map[string]any{
		"metadata": map[string]any{
			"ideType":    loadCodeAssistIDEType,
			"ideVersion": loadCodeAssistIDEVersion,
			"ideName":    loadCodeAssistIDEName,
		},
	}
	bodyBytes, _ := json.Marshal(reqBody)

	for _, apiURL := range loadCodeAssistURLs {
		pid, pt := tryLoadCodeAssist(ctx, client, apiURL, accessToken, bodyBytes)
		if pid != "" {
			return pid, pt
		}
	}
	return "", ""
}

// tryLoadCodeAssist attempts a single loadCodeAssist call.
func tryLoadCodeAssist(
	ctx context.Context, client *http.Client,
	apiURL, accessToken string, body []byte,
) (projectID, planType string) {
	httpReq, err := http.NewRequestWithContext(
		ctx, http.MethodPost, apiURL, bytes.NewReader(body),
	)
	if err != nil {
		return "", ""
	}
	httpReq.Header.Set("Authorization", "Bearer "+accessToken)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("User-Agent", antigravityUserAgent)

	resp, err := client.Do(httpReq)
	if err != nil {
		return "", ""
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBody))
	if err != nil || resp.StatusCode != http.StatusOK {
		return "", ""
	}

	var result struct {
		CloudAICompanionProject string `json:"cloudaicompanionProject"`
		CurrentTier             *struct {
			ID string `json:"id"`
		} `json:"currentTier,omitempty"`
		PaidTier *struct {
			ID string `json:"id"`
		} `json:"paidTier,omitempty"`
	}
	if json.Unmarshal(respBody, &result) != nil {
		return "", ""
	}

	projectID = strings.TrimSpace(result.CloudAICompanionProject)

	// Determine plan_type from tier info.
	tierID := ""
	if result.PaidTier != nil && result.PaidTier.ID != "" {
		tierID = result.PaidTier.ID
	} else if result.CurrentTier != nil {
		tierID = result.CurrentTier.ID
	}
	planType = antigravityTierToPlanType(tierID)

	return projectID, planType
}

// antigravityTierToPlanType maps tier IDs to plan type names.
func antigravityTierToPlanType(tierID string) string {
	switch strings.ToLower(strings.TrimSpace(tierID)) {
	case "free-tier":
		return "Free"
	case "g1-pro-tier":
		return "Pro"
	case "g1-ultra-tier":
		return "Ultra"
	default:
		if tierID == "" {
			return "Free"
		}
		return tierID
	}
}

// ---------- Privacy API ----------

// setAntigravityPrivacyViaAPI calls the Antigravity privacy API to disable
// telemetry. Returns the privacy mode string or empty on failure.
func setAntigravityPrivacyViaAPI(
	ctx context.Context, client *http.Client,
	accessToken, projectID string,
) string {
	if accessToken == "" {
		return ""
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Step 1: Call setUserSettings to clear telemetry.
	if !callSetUserSettings(ctx, client, accessToken) {
		return privacyModeFailed
	}

	// Step 2: Verify via fetchUserInfo (requires project_id).
	if strings.TrimSpace(projectID) == "" {
		return privacyModeFailed
	}
	if !verifyPrivacyViaFetchUserInfo(ctx, client, accessToken, projectID) {
		return privacyModeFailed
	}

	return privacyModeSet
}

// callSetUserSettings sends empty user_settings to clear telemetry.
func callSetUserSettings(
	ctx context.Context, client *http.Client, accessToken string,
) bool {
	payload := map[string]any{"user_settings": map[string]any{}}
	bodyBytes, _ := json.Marshal(payload)

	apiURL := privacyAPIBaseURL + "/v1internal:setUserSettings"
	httpReq, err := http.NewRequestWithContext(
		ctx, http.MethodPost, apiURL, bytes.NewReader(bodyBytes),
	)
	if err != nil {
		return false
	}
	httpReq.Header.Set("Authorization", "Bearer "+accessToken)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "*/*")
	httpReq.Header.Set("User-Agent", antigravityUserAgent)
	httpReq.Header.Set("X-Goog-Api-Client", "gl-node/22.21.1")
	httpReq.Host = "daily-cloudcode-pa.googleapis.com"

	resp, err := client.Do(httpReq)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseBody))
	if resp.StatusCode != http.StatusOK {
		return false
	}

	// Verify response: userSettings should be empty (no telemetryEnabled).
	var result struct {
		UserSettings map[string]any `json:"userSettings,omitempty"`
	}
	if json.Unmarshal(respBody, &result) != nil {
		return false
	}
	_, hasTelemetry := result.UserSettings["telemetryEnabled"]
	return !hasTelemetry
}

// verifyPrivacyViaFetchUserInfo checks that privacy is set via fetchUserInfo.
func verifyPrivacyViaFetchUserInfo(
	ctx context.Context, client *http.Client,
	accessToken, projectID string,
) bool {
	payload := map[string]any{"project": projectID}
	bodyBytes, _ := json.Marshal(payload)

	apiURL := privacyAPIBaseURL + "/v1internal:fetchUserInfo"
	httpReq, err := http.NewRequestWithContext(
		ctx, http.MethodPost, apiURL, bytes.NewReader(bodyBytes),
	)
	if err != nil {
		return false
	}
	httpReq.Header.Set("Authorization", "Bearer "+accessToken)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "*/*")
	httpReq.Header.Set("User-Agent", antigravityUserAgent)
	httpReq.Header.Set("X-Goog-Api-Client", "gl-node/22.21.1")
	httpReq.Host = "daily-cloudcode-pa.googleapis.com"

	resp, err := client.Do(httpReq)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseBody))
	if resp.StatusCode != http.StatusOK {
		return false
	}

	var result struct {
		UserSettings map[string]any `json:"userSettings,omitempty"`
	}
	if json.Unmarshal(respBody, &result) != nil {
		return false
	}

	// Privacy is set when userSettings is nil/empty or has no telemetryEnabled.
	if result.UserSettings == nil {
		return true
	}
	_, hasTelemetry := result.UserSettings["telemetryEnabled"]
	return !hasTelemetry
}
