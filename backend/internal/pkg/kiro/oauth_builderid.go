package kiro

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// builderIDStartURL is the AWS Builder ID issuer.
const builderIDStartURL = "https://view.awsapps.com/start"

// builderIDSessionTTL bounds the lifetime of a pending device-code flow.
// AWS returns expiresIn=600s; this is just our local upper bound.
const builderIDSessionTTL = 12 * time.Minute

// BuilderIDLoginStarted is the result of StartBuilderIDLogin. The admin
// opens VerificationURI in a browser, enters UserCode, then polls.
type BuilderIDLoginStarted struct {
	SessionID       string
	UserCode        string
	VerificationURI string
	Interval        int   // seconds between polls
	ExpiresAtUnix   int64 // when the upstream auth window closes
}

// BuilderIDPollStatus describes one poll attempt.
type BuilderIDPollStatus string

const (
	BuilderIDPollPending   BuilderIDPollStatus = "pending"
	BuilderIDPollSlowDown  BuilderIDPollStatus = "slow_down"
	BuilderIDPollCompleted BuilderIDPollStatus = "completed"
)

// BuilderIDPollResult bundles the poll outcome.
type BuilderIDPollResult struct {
	Status BuilderIDPollStatus
	Token  *TokenInfo // populated when Status == Completed
}

// StartBuilderIDLogin registers an OIDC client and kicks off a device-code
// flow against AWS Builder ID in the supplied region (defaults to us-east-1).
func StartBuilderIDLogin(store *SessionStore, region, proxyURL string) (*BuilderIDLoginStarted, error) {
	if region == "" {
		region = "us-east-1"
	}
	client := HTTPClient(proxyURL)
	oidcBase := fmt.Sprintf("https://oidc.%s.amazonaws.com", region)

	clientID, clientSecret, err := registerOIDCClient(client, oidcBase, builderIDStartURL, nil,
		[]string{"urn:ietf:params:oauth:grant-type:device_code", "refresh_token"})
	if err != nil {
		return nil, fmt.Errorf("kiro builderid: register client: %w", err)
	}

	// Device-authorization request.
	payload, _ := json.Marshal(map[string]string{
		"clientId":     clientID,
		"clientSecret": clientSecret,
		"startUrl":     builderIDStartURL,
	})
	req, err := http.NewRequest(http.MethodPost, oidcBase+"/device_authorization", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("kiro builderid: device authorization: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("kiro builderid: device authorization HTTP %d: %s", resp.StatusCode, string(body))
	}

	var parsed struct {
		DeviceCode              string `json:"deviceCode"`
		UserCode                string `json:"userCode"`
		VerificationURI         string `json:"verificationUri"`
		VerificationURIComplete string `json:"verificationUriComplete"`
		Interval                int    `json:"interval"`
		ExpiresIn               int    `json:"expiresIn"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("kiro builderid: decode device authorization: %w", err)
	}
	if parsed.DeviceCode == "" || parsed.UserCode == "" {
		return nil, fmt.Errorf("kiro builderid: empty device_code or user_code")
	}
	if parsed.Interval == 0 {
		parsed.Interval = 5
	}
	if parsed.ExpiresIn == 0 {
		parsed.ExpiresIn = 600
	}
	verificationURI := parsed.VerificationURIComplete
	if verificationURI == "" {
		verificationURI = parsed.VerificationURI
	}

	sessionID := uuid.New().String()
	now := time.Now()
	expiresAt := now.Add(time.Duration(parsed.ExpiresIn) * time.Second)
	if expiresAt.Sub(now) > builderIDSessionTTL {
		expiresAt = now.Add(builderIDSessionTTL)
	}

	sess := &BuilderIDSession{
		ID:              sessionID,
		ClientID:        clientID,
		ClientSecret:    clientSecret,
		DeviceCode:      parsed.DeviceCode,
		UserCode:        parsed.UserCode,
		VerificationURI: verificationURI,
		Interval:        parsed.Interval,
		Region:          region,
		ProxyURL:        proxyURL,
		ExpiresAt:       expiresAt,
	}

	if store != nil {
		store.SetBuilderID(sessionID, sess)
	}

	return &BuilderIDLoginStarted{
		SessionID:       sessionID,
		UserCode:        parsed.UserCode,
		VerificationURI: verificationURI,
		Interval:        parsed.Interval,
		ExpiresAtUnix:   expiresAt.Unix(),
	}, nil
}

// PollBuilderIDLogin polls AWS once for the device-code session.
// Returns BuilderIDPollPending or BuilderIDPollSlowDown to indicate the
// admin hasn't completed login yet (the UI should poll again after
// Interval seconds — note the interval is bumped on slow_down).
// On BuilderIDPollCompleted, Token is populated with AuthMethod=BuilderID
// plus client_id/secret/region for subsequent refreshes.
func PollBuilderIDLogin(store *SessionStore, sessionID string) (*BuilderIDPollResult, error) {
	if store == nil {
		return nil, fmt.Errorf("kiro builderid: session store required")
	}
	sess, ok := store.GetBuilderID(sessionID)
	if !ok {
		return nil, fmt.Errorf("kiro builderid: session not found or expired")
	}
	if time.Now().After(sess.ExpiresAt) {
		store.DeleteBuilderID(sessionID)
		return nil, fmt.Errorf("kiro builderid: session expired")
	}

	client := HTTPClient(sess.ProxyURL)
	oidcBase := fmt.Sprintf("https://oidc.%s.amazonaws.com", sess.Region)
	payload, _ := json.Marshal(map[string]string{
		"clientId":     sess.ClientID,
		"clientSecret": sess.ClientSecret,
		"grantType":    "urn:ietf:params:oauth:grant-type:device_code",
		"deviceCode":   sess.DeviceCode,
	})
	req, err := http.NewRequest(http.MethodPost, oidcBase+"/token", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("kiro builderid: poll: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == http.StatusOK {
		var parsed struct {
			AccessToken  string `json:"accessToken"`
			RefreshToken string `json:"refreshToken"`
			ExpiresIn    int    `json:"expiresIn"`
			ProfileArn   string `json:"profileArn"`
		}
		if err := json.Unmarshal(body, &parsed); err != nil {
			return nil, fmt.Errorf("kiro builderid: decode token: %w", err)
		}
		if parsed.AccessToken == "" {
			return nil, fmt.Errorf("kiro builderid: empty access token")
		}
		store.DeleteBuilderID(sessionID)
		return &BuilderIDPollResult{
			Status: BuilderIDPollCompleted,
			Token: &TokenInfo{
				AccessToken:  parsed.AccessToken,
				RefreshToken: parsed.RefreshToken,
				ExpiresAt:    time.Now().Unix() + int64(parsed.ExpiresIn),
				ProfileARN:   parsed.ProfileArn,
				AuthMethod:   AuthMethodBuilderID,
				ClientID:     sess.ClientID,
				ClientSecret: sess.ClientSecret,
				Region:       sess.Region,
			},
		}, nil
	}

	if resp.StatusCode == http.StatusBadRequest {
		var errResp struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(body, &errResp)
		switch errResp.Error {
		case "authorization_pending":
			return &BuilderIDPollResult{Status: BuilderIDPollPending}, nil
		case "slow_down":
			store.UpdateBuilderIDInterval(sessionID, 5)
			return &BuilderIDPollResult{Status: BuilderIDPollSlowDown}, nil
		case "expired_token":
			store.DeleteBuilderID(sessionID)
			return nil, fmt.Errorf("kiro builderid: device code expired")
		case "access_denied":
			store.DeleteBuilderID(sessionID)
			return nil, fmt.Errorf("kiro builderid: user denied authorization")
		default:
			return nil, fmt.Errorf("kiro builderid: authorization error: %s", errResp.Error)
		}
	}

	return nil, fmt.Errorf("kiro builderid: unexpected HTTP %d: %s", resp.StatusCode, string(body))
}
