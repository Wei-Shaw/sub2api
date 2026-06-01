package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/plugin-sdk/gatewayutil"
	pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
)

// Claude OAuth constants -- mirrored from backend/internal/pkg/oauth/oauth.go.
// The plugin cannot import host internal packages, so these are duplicated.
const (
	claudeTokenURL = "https://platform.claude.com/v1/oauth/token"
	claudeClientID = "9d1c250a-e61b-44d9-88ed-5944d1962f5e"
)

// RefreshToken performs Claude OAuth token refresh via platform.claude.com.
// It calls the OAuth token endpoint with grant_type=refresh_token and
// returns the updated credentials for the host to persist.
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

	tokenResp, err := callClaudeTokenEndpoint(ctx, s.httpClient, refreshToken)
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

// claudeTokenResponse mirrors the Claude OAuth /v1/oauth/token JSON response.
type claudeTokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
	RefreshToken string `json:"refresh_token,omitempty"`
	Scope        string `json:"scope,omitempty"`
}

// callClaudeTokenEndpoint calls platform.claude.com/v1/oauth/token with
// grant_type=refresh_token and returns the parsed response.
func callClaudeTokenEndpoint(
	ctx context.Context, client *http.Client,
	refreshToken string,
) (*claudeTokenResponse, error) {
	reqBody := map[string]any{
		"grant_type":    "refresh_token",
		"refresh_token": refreshToken,
		"client_id":     claudeClientID,
	}
	bodyBytes, _ := json.Marshal(reqBody)

	httpReq, err := http.NewRequestWithContext(
		ctx, http.MethodPost, claudeTokenURL, strings.NewReader(string(bodyBytes)),
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

	const maxBodySize = 1 << 20 // 1 MB
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodySize))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, gatewayutil.TruncateBody(body))
	}

	var tokenResp claudeTokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	return &tokenResp, nil
}

// buildUpdatedCredentials merges the refresh response into the existing
// credential map, preserving fields that the token endpoint does not return.
func buildUpdatedCredentials(
	existing map[string]any, tok *claudeTokenResponse,
) map[string]any {
	out := make(map[string]any, len(existing))
	for k, v := range existing {
		out[k] = v
	}

	out["access_token"] = tok.AccessToken
	expiresAt := time.Now().Add(time.Duration(tok.ExpiresIn) * time.Second)
	out["expires_at"] = expiresAt.Format(time.RFC3339)

	// Only overwrite refresh_token if the endpoint returned a new one.
	if strings.TrimSpace(tok.RefreshToken) != "" {
		out["refresh_token"] = tok.RefreshToken
	}
	if tok.TokenType != "" {
		out["token_type"] = tok.TokenType
	}
	if tok.Scope != "" {
		out["scope"] = tok.Scope
	}

	return out
}
