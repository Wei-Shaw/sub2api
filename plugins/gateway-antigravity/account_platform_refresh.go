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

// RefreshToken performs Antigravity OAuth token refresh via Google's
// OAuth2 token endpoint. It calls the token endpoint with
// grant_type=refresh_token and returns the updated credentials for
// the host to persist.
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

	clientSecret := gatewayutil.CredStr(creds, "client_secret")

	tokenResp, err := callGoogleTokenEndpoint(ctx, s.httpClient, refreshToken, clientSecret)
	if err != nil {
		return &pb.RefreshTokenResponse{
			Success: false,
			Error:   "token refresh failed: " + err.Error(),
		}, nil
	}

	updatedCreds := buildUpdatedCredentials(creds, tokenResp)
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

// googleTokenResponse mirrors the Google OAuth /token JSON response.
type googleTokenResponse struct {
	AccessToken  string `json:"access_token"`
	ExpiresIn    int64  `json:"expires_in"`
	TokenType    string `json:"token_type"`
	Scope        string `json:"scope,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`
}

// callGoogleTokenEndpoint calls oauth2.googleapis.com/token with
// grant_type=refresh_token and returns the parsed response.
func callGoogleTokenEndpoint(
	ctx context.Context, client *http.Client,
	refreshToken, clientSecret string,
) (*googleTokenResponse, error) {
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refreshToken)
	form.Set("client_id", googleClientID)
	if clientSecret != "" {
		form.Set("client_secret", clientSecret)
	}

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
// credential map, preserving fields that the token endpoint does not return
// (email, project_id, etc.).
func buildUpdatedCredentials(
	existing map[string]any, tok *googleTokenResponse,
) map[string]any {
	out := make(map[string]any, len(existing))
	for k, v := range existing {
		out[k] = v
	}

	out["access_token"] = tok.AccessToken
	// Compute expires_at with 5-minute safety window, matching host behavior.
	expiresAt := time.Now().Unix() + tok.ExpiresIn - 300
	out["expires_at"] = fmt.Sprintf("%d", expiresAt)

	if tok.TokenType != "" {
		out["token_type"] = tok.TokenType
	}

	// Only overwrite refresh_token if the endpoint returned a new one.
	if strings.TrimSpace(tok.RefreshToken) != "" {
		out["refresh_token"] = tok.RefreshToken
	}

	return out
}
