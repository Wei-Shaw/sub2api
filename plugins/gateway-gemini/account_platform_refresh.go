package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/Wei-Shaw/sub2api/plugin-sdk/gatewayutil"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
)

const (
	// googleTokenURL is the Google OAuth2 token endpoint.
	googleTokenURL = "https://oauth2.googleapis.com/token"

	// defaultGeminiCLIClientID is the public Gemini CLI OAuth client ID.
	defaultGeminiCLIClientID = "681255809395-oo8ft2oprdrnp9e3aqf6av3hmdib135j.apps.googleusercontent.com"

	// defaultGeminiCLIClientSecret is the public Gemini CLI OAuth client secret.
	defaultGeminiCLIClientSecret = "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"

	// safetyWindow is the number of seconds subtracted from expires_in to
	// account for network latency and clock drift.
	safetyWindow = 300

	// minTTL is the minimum remaining TTL in seconds to prevent refresh storms.
	minTTL = 30
)

// RefreshToken performs Google OAuth token refresh via oauth2.googleapis.com.
func (s *accountPlatformServer) RefreshToken(
	ctx context.Context,
	req *pb.RefreshTokenRequest,
) (*pb.RefreshTokenResponse, error) {
	var creds map[string]any
	if err := json.Unmarshal(req.GetCredentialsJson(), &creds); err != nil {
		return &pb.RefreshTokenResponse{
			Success: false,
			Error:   "failed to parse credentials: " + err.Error(),
		}, nil
	}

	refreshToken := gatewayutil.CredStr(creds, "refresh_token")
	if refreshToken == "" {
		return gatewayutil.HandleNoRefreshToken(creds)
	}

	clientID, clientSecret := resolveOAuthClient(creds)

	tokenResp, err := callGoogleTokenEndpoint(ctx, s.httpClient, refreshToken, clientID, clientSecret)
	if err != nil {
		return &pb.RefreshTokenResponse{
			Success: false,
			Error:   "token refresh failed: " + err.Error(),
		}, nil
	}

	updatedCreds := buildUpdatedCredentials(creds, tokenResp, clientID)
	updatedJSON, err := json.Marshal(updatedCreds)
	if err != nil {
		return &pb.RefreshTokenResponse{
			Success: false,
			Error:   "failed to marshal updated credentials: " + err.Error(),
		}, nil
	}

	return &pb.RefreshTokenResponse{
		Success:                true,
		UpdatedCredentialsJson: updatedJSON,
	}, nil
}

// resolveOAuthClient determines the OAuth client_id and client_secret to use.
// Falls back to the built-in Gemini CLI client if not configured.
func resolveOAuthClient(creds map[string]any) (clientID, clientSecret string) {
	clientID = gatewayutil.CredStr(creds, "client_id")
	clientSecret = gatewayutil.CredStr(creds, "client_secret")
	if clientID == "" || clientSecret == "" {
		clientID = defaultGeminiCLIClientID
		clientSecret = defaultGeminiCLIClientSecret
	}
	return clientID, clientSecret
}

// googleTokenResponse mirrors the Google /token JSON response.
type googleTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token,omitempty"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
	Scope        string `json:"scope,omitempty"`
}

// callGoogleTokenEndpoint calls oauth2.googleapis.com/token with
// grant_type=refresh_token and returns the parsed response.
func callGoogleTokenEndpoint(
	ctx context.Context, client *http.Client,
	refreshToken, clientID, clientSecret string,
) (*googleTokenResponse, error) {
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refreshToken)
	form.Set("client_id", clientID)
	form.Set("client_secret", clientSecret)

	httpReq, err := http.NewRequestWithContext(
		ctx, http.MethodPost, googleTokenURL, strings.NewReader(form.Encode()),
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

	const maxBodySize = 1 << 20 // 1 MB
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodySize))
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

// buildUpdatedCredentials merges the refresh response into the existing
// credential map, preserving fields that the token endpoint does not return.
func buildUpdatedCredentials(
	existing map[string]any, tok *googleTokenResponse, clientID string,
) map[string]any {
	out := make(map[string]any, len(existing))
	for k, v := range existing {
		out[k] = v
	}

	out["access_token"] = tok.AccessToken

	// Compute expires_at with safety window and minimum TTL.
	expiresAt := time.Now().Unix() + tok.ExpiresIn - safetyWindow
	minExpiresAt := time.Now().Unix() + minTTL
	if expiresAt < minExpiresAt {
		expiresAt = minExpiresAt
	}
	out["expires_at"] = time.Unix(expiresAt, 0).Format(time.RFC3339)

	// Only overwrite refresh_token if the endpoint returned a new one.
	if strings.TrimSpace(tok.RefreshToken) != "" {
		out["refresh_token"] = tok.RefreshToken
	}
	if strings.TrimSpace(clientID) != "" {
		out["client_id"] = clientID
	}

	return out
}
