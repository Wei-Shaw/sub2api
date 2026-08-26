package cursor

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

var oauthTokenURL = BaseURLAPI + EndpointToken

type tokenRefreshRequest struct {
	GrantType    string `json:"grant_type"`
	ClientID     string `json:"client_id"`
	RefreshToken string `json:"refresh_token"`
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
}

// TokenRefreshResult is a Cursor OAuth token rotation.
type TokenRefreshResult struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int
}

// ExpiresAt is when the new access token should be treated as stale.
func (r TokenRefreshResult) ExpiresAt(now time.Time) time.Time {
	if r.ExpiresIn > 0 {
		return now.Add(time.Duration(r.ExpiresIn) * time.Second)
	}
	if exp := AccessTokenExpiry(r.AccessToken); exp != nil {
		return *exp
	}
	return now.Add(time.Hour)
}

// RefreshToken exchanges a refresh token for a new access token.
func RefreshToken(ctx context.Context, httpClient *http.Client, refreshToken string) (newAccessToken string, err error) {
	result, err := RefreshSession(ctx, httpClient, refreshToken)
	if err != nil {
		return "", err
	}
	return result.AccessToken, nil
}

// RefreshSession exchanges a refresh token for a new Cursor session.
func RefreshSession(ctx context.Context, httpClient *http.Client, refreshToken string) (*TokenRefreshResult, error) {
	return refreshTokenImpl(ctx, httpClient, refreshToken)
}

func refreshTokenImpl(ctx context.Context, httpClient *http.Client, refreshToken string) (*TokenRefreshResult, error) {
	refreshToken = strings.TrimSpace(refreshToken)
	if refreshToken == "" {
		return nil, fmt.Errorf("cursor: missing refresh token")
	}
	payload, err := json.Marshal(tokenRefreshRequest{
		GrantType:    "refresh_token",
		ClientID:     DefaultAuthClientID,
		RefreshToken: refreshToken,
	})
	if err != nil {
		return nil, fmt.Errorf("cursor: encode refresh request: %w", err)
	}

	if httpClient == nil {
		httpClient = NewHTTP2Transport()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, oauthTokenURL, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("cursor: build refresh request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cursor: token refresh: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("cursor: read refresh response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		msg := strings.TrimSpace(string(body))
		if len(msg) > 300 {
			msg = msg[:300]
		}
		return nil, fmt.Errorf("cursor: token refresh status %d: %s", resp.StatusCode, msg)
	}

	var tr tokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return nil, fmt.Errorf("cursor: parse refresh response: %w", err)
	}
	if strings.TrimSpace(tr.AccessToken) == "" {
		return nil, fmt.Errorf("cursor: empty access token in refresh response")
	}
	return &TokenRefreshResult{
		AccessToken:  strings.TrimSpace(tr.AccessToken),
		RefreshToken: strings.TrimSpace(tr.RefreshToken),
		ExpiresIn:    tr.ExpiresIn,
	}, nil
}

// AccessTokenExpiry reads exp from an unverified Cursor JWT.
// Tokens may be stored as "userId::jwt".
func AccessTokenExpiry(token string) *time.Time {
	token = strings.TrimSpace(token)
	if i := strings.Index(token, "::"); i >= 0 && i < len(token)-2 {
		token = strings.TrimSpace(token[i+2:])
	}
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return nil
	}
	payload, err := decodeJWTSegment(parts[1])
	if err != nil {
		return nil
	}
	var claims struct {
		Exp json.Number `json:"exp"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil || claims.Exp == "" {
		return nil
	}
	sec, err := claims.Exp.Int64()
	if err != nil {
		f, ferr := claims.Exp.Float64()
		if ferr != nil {
			return nil
		}
		sec = int64(f)
	}
	if sec <= 0 {
		return nil
	}
	t := time.Unix(sec, 0).UTC()
	return &t
}

func decodeJWTSegment(seg string) ([]byte, error) {
	if payload, err := base64.RawURLEncoding.DecodeString(seg); err == nil {
		return payload, nil
	}
	return base64.URLEncoding.DecodeString(seg)
}
