package cursor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type tokenRefreshRequest struct {
	GrantType    string `json:"grant_type"`
	ClientID     string `json:"client_id"`
	RefreshToken string `json:"refresh_token"`
}

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

func refreshTokenImpl(ctx context.Context, httpClient *http.Client, refreshToken string) (string, error) {
	payload, _ := json.Marshal(tokenRefreshRequest{
		GrantType:    "refresh_token",
		ClientID:     DefaultAuthClientID,
		RefreshToken: refreshToken,
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, BaseURLAPI+EndpointToken, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("cursor: build refresh request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("cursor: token refresh: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("cursor: read refresh response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("cursor: token refresh status %d: %s", resp.StatusCode, string(body))
	}

	var tr tokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return "", fmt.Errorf("cursor: parse refresh response: %w", err)
	}
	if tr.AccessToken == "" {
		return "", fmt.Errorf("cursor: empty access token in refresh response: %s", string(body))
	}
	return tr.AccessToken, nil
}
