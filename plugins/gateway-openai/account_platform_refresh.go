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

// RefreshToken performs OpenAI OAuth token refresh via auth.openai.com.
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

	clientID := gatewayutil.CredStr(creds, "client_id")
	if clientID == "" {
		clientID = openaiDefaultCID
	}

	tokenResp, err := callOpenAITokenEndpoint(ctx, s.httpClient, refreshToken, clientID)
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

// openaiTokenResponse mirrors the OpenAI /oauth/token JSON response.
type openaiTokenResponse struct {
	AccessToken  string `json:"access_token"`
	IDToken      string `json:"id_token"`
	RefreshToken string `json:"refresh_token,omitempty"`
	ExpiresIn    int64  `json:"expires_in"`
}

// callOpenAITokenEndpoint calls auth.openai.com/oauth/token with
// grant_type=refresh_token and returns the parsed response.
func callOpenAITokenEndpoint(
	ctx context.Context, client *http.Client,
	refreshToken, clientID string,
) (*openaiTokenResponse, error) {
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refreshToken)
	form.Set("client_id", clientID)
	form.Set("scope", openaiRefreshScope)

	httpReq, err := http.NewRequestWithContext(
		ctx, http.MethodPost, openaiTokenURL, strings.NewReader(form.Encode()),
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

	const maxBodySize = 1 << 20 // 1 MB
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodySize))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, gatewayutil.TruncateBody(body))
	}

	var tokenResp openaiTokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	return &tokenResp, nil
}

// buildUpdatedCredentials merges the refresh response into the existing
// credential map, preserving fields that the token endpoint does not return.
func buildUpdatedCredentials(
	existing map[string]any, tok *openaiTokenResponse, clientID string,
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
	if tok.IDToken != "" {
		out["id_token"] = tok.IDToken
	}
	if strings.TrimSpace(clientID) != "" {
		out["client_id"] = clientID
	}

	return out
}
