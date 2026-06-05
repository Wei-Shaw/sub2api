package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// geminiMaxResponseBody limits response body reads to prevent OOM.
const geminiMaxResponseBody = 1 << 20 // 1 MB

// loadCodeAssist IDE metadata constants.
const (
	loadCodeAssistIDEType    = "ANTIGRAVITY"
	loadCodeAssistIDEVersion = "1.20.6"
	loadCodeAssistIDEName    = "antigravity"
)

// geminiLoadCodeAssistURLs are the Gemini Code Assist API endpoints
// (prod first, daily sandbox fallback).
var geminiLoadCodeAssistURLs = []string{
	"https://cloudcode-pa.googleapis.com/v1internal:loadCodeAssist",
	"https://daily-cloudcode-pa.sandbox.googleapis.com/v1internal:loadCodeAssist",
}

// geminiTokenResponse mirrors the Google /token JSON response.
type geminiTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token,omitempty"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
	Scope        string `json:"scope,omitempty"`
}

// ---------- Token exchange ----------

// exchangeGeminiCode calls the Google OAuth token endpoint to exchange
// an authorization code for tokens.
func exchangeGeminiCode(
	ctx context.Context, client *http.Client,
	code, codeVerifier, clientID, clientSecret, redirectURI string,
) (*geminiTokenResponse, error) {
	form := url.Values{}
	form.Set("client_id", clientID)
	form.Set("client_secret", clientSecret)
	form.Set("code", code)
	form.Set("redirect_uri", redirectURI)
	form.Set("grant_type", "authorization_code")
	form.Set("code_verifier", codeVerifier)

	return postGeminiToken(ctx, client, form)
}

// refreshGeminiToken calls the Google OAuth token endpoint with
// grant_type=refresh_token.
func refreshGeminiToken(
	ctx context.Context, client *http.Client,
	refreshToken, clientID, clientSecret string,
) (*geminiTokenResponse, error) {
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refreshToken)
	form.Set("client_id", clientID)
	form.Set("client_secret", clientSecret)

	return postGeminiToken(ctx, client, form)
}

// postGeminiToken sends a form-encoded POST to the Google token endpoint.
func postGeminiToken(
	ctx context.Context, client *http.Client,
	form url.Values,
) (*geminiTokenResponse, error) {
	httpReq, err := http.NewRequestWithContext(
		ctx, http.MethodPost, geminiTokenURL,
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

	body, err := io.ReadAll(io.LimitReader(resp.Body, geminiMaxResponseBody))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, geminiTruncateBody(body))
	}

	var tokenResp geminiTokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	return &tokenResp, nil
}

// ---------- User info ----------

// fetchGeminiEmail fetches the user's email via the Google userinfo API.
// Returns empty string on any error (best-effort).
func fetchGeminiEmail(
	ctx context.Context, client *http.Client, accessToken string,
) string {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, geminiUserInfoURL, nil)
	if err != nil {
		return ""
	}
	httpReq.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := client.Do(httpReq)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, geminiMaxResponseBody))
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

// ---------- LoadCodeAssist (project_id + tier) ----------

// loadGeminiProjectInfo calls the Gemini Code Assist loadCodeAssist endpoint
// to fetch project_id and tier_id. Returns empty strings on failure.
func loadGeminiProjectInfo(
	ctx context.Context, client *http.Client, accessToken string,
) (projectID, tierID string) {
	reqBody := map[string]any{
		"metadata": map[string]any{
			"ideType":    loadCodeAssistIDEType,
			"ideVersion": loadCodeAssistIDEVersion,
			"ideName":    loadCodeAssistIDEName,
		},
	}
	bodyBytes, _ := json.Marshal(reqBody)

	for _, apiURL := range geminiLoadCodeAssistURLs {
		pid, tid := tryGeminiLoadCodeAssist(ctx, client, apiURL, accessToken, bodyBytes)
		if pid != "" {
			return pid, tid
		}
	}
	return "", ""
}

// tryGeminiLoadCodeAssist attempts a single loadCodeAssist call.
func tryGeminiLoadCodeAssist(
	ctx context.Context, client *http.Client,
	apiURL, accessToken string, body []byte,
) (projectID, tierID string) {
	httpReq, err := http.NewRequestWithContext(
		ctx, http.MethodPost, apiURL, bytes.NewReader(body),
	)
	if err != nil {
		return "", ""
	}
	httpReq.Header.Set("Authorization", "Bearer "+accessToken)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("User-Agent", geminiCLIUserAgent)

	resp, err := client.Do(httpReq)
	if err != nil {
		return "", ""
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, geminiMaxResponseBody))
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
		AllowedTiers []struct {
			ID        string `json:"id"`
			IsDefault bool   `json:"isDefault"`
		} `json:"allowedTiers,omitempty"`
	}
	if json.Unmarshal(respBody, &result) != nil {
		return "", ""
	}

	projectID = strings.TrimSpace(result.CloudAICompanionProject)

	// Determine tier from response.
	rawTier := ""
	if result.PaidTier != nil && result.PaidTier.ID != "" {
		rawTier = result.PaidTier.ID
	} else if result.CurrentTier != nil && result.CurrentTier.ID != "" {
		rawTier = result.CurrentTier.ID
	} else {
		// Try allowedTiers (default first).
		for _, t := range result.AllowedTiers {
			if t.IsDefault && strings.TrimSpace(t.ID) != "" {
				rawTier = t.ID
				break
			}
		}
		if rawTier == "" {
			for _, t := range result.AllowedTiers {
				if strings.TrimSpace(t.ID) != "" {
					rawTier = t.ID
					break
				}
			}
		}
	}

	tierID = canonicalizeGeminiTierID(rawTier)
	return projectID, tierID
}

// canonicalizeGeminiTierID maps raw tier IDs to canonical sub2api tier IDs.
func canonicalizeGeminiTierID(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	lower := strings.ToLower(raw)
	switch lower {
	case "google_one_free", "google_ai_pro", "google_ai_ultra",
		"gcp_standard", "gcp_enterprise",
		"aistudio_free", "aistudio_paid",
		"google_one_unknown":
		return lower
	}

	upper := strings.ToUpper(raw)
	switch upper {
	case "AI_PREMIUM":
		return "google_ai_pro"
	case "GOOGLE_ONE_UNLIMITED":
		return "google_ai_ultra"
	case "FREE", "GOOGLE_ONE_BASIC", "GOOGLE_ONE_STANDARD":
		return "google_one_free"
	case "GOOGLE_ONE_UNKNOWN":
		return "google_one_unknown"
	case "STANDARD", "PRO", "LEGACY":
		return "gcp_standard"
	case "ENTERPRISE", "ULTRA":
		return "gcp_enterprise"
	}

	switch lower {
	case "standard-tier", "pro-tier":
		return "gcp_standard"
	case "ultra-tier":
		return "gcp_enterprise"
	}

	return ""
}

// geminiTruncateBody returns at most 200 bytes of body text for error messages.
func geminiTruncateBody(b []byte) string {
	const maxLen = 200
	if len(b) <= maxLen {
		return string(b)
	}
	return string(b[:maxLen]) + "..."
}
