package kiro

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// profileURL is Kiro's getUsageLimits endpoint, which conveniently returns
// the calling user's email + userId in the userInfo field when
// isEmailRequired=true. We use it after a fresh token to name new accounts;
// the quota fields it also returns are consumed by the dedicated quota
// fetcher (phase 6).
const profileURL = "https://q.us-east-1.amazonaws.com/getUsageLimits?origin=AI_EDITOR&resourceType=AGENTIC_REQUEST&isEmailRequired=true"

// Profile contains the lightweight user identification fields surfaced from
// Kiro's getUsageLimits endpoint.
type Profile struct {
	Email  string
	UserID string
}

// FetchProfile calls Kiro getUsageLimits with the bearer token and returns
// the user's email and userId. Returns an error on non-200; success with
// empty userInfo returns a Profile with empty fields rather than an error
// (some accounts are created without an email).
func FetchProfile(accessToken, proxyURL string) (*Profile, error) {
	return fetchProfileAt(profileURL, accessToken, HTTPClient(proxyURL))
}

func fetchProfileAt(url, accessToken string, client *http.Client) (*Profile, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "aws-sdk-js/1.0.18 sub2api")
	req.Header.Set("x-amz-user-agent", "aws-sdk-js/1.0.18 sub2api")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("kiro profile: HTTP %d: %s", resp.StatusCode, string(body))
	}

	var parsed struct {
		UserInfo struct {
			Email  string `json:"email"`
			UserID string `json:"userId"`
		} `json:"userInfo"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("kiro profile: decode: %w", err)
	}
	return &Profile{Email: parsed.UserInfo.Email, UserID: parsed.UserInfo.UserID}, nil
}
