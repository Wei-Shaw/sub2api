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
// the quota fields it also returns are consumed by FetchUsageLimits.
const profileURL = "https://q.us-east-1.amazonaws.com/getUsageLimits?origin=AI_EDITOR&resourceType=AGENTIC_REQUEST&isEmailRequired=true"

// Profile contains the lightweight user identification fields surfaced from
// Kiro's getUsageLimits endpoint.
type Profile struct {
	Email  string
	UserID string
}

// UsageLimits is the full /getUsageLimits response. Used by the quota
// fetcher to populate account.extra.{usage, subscription} on a cron.
type UsageLimits struct {
	UserInfo         *UserInfo         `json:"userInfo,omitempty"`
	SubscriptionInfo *SubscriptionInfo `json:"subscriptionInfo,omitempty"`
	NextDateReset    json.Number       `json:"nextDateReset,omitempty"`
	UsageBreakdown   []UsageBreakdown  `json:"usageBreakdownList,omitempty"`
}

// UserInfo is the user identity portion of the response.
type UserInfo struct {
	Email  string `json:"email"`
	UserID string `json:"userId"`
}

// SubscriptionInfo describes the account's plan tier.
type SubscriptionInfo struct {
	SubscriptionName  string `json:"subscriptionName,omitempty"`
	SubscriptionTitle string `json:"subscriptionTitle,omitempty"`
	SubscriptionType  string `json:"subscriptionType,omitempty"`
	Status            string `json:"status,omitempty"`
}

// UsageBreakdown is one row of the per-resource usage table. The
// "AGENTIC_REQUEST" row is the one operators care about for chat
// requests; other resources may be returned but are ignored by callers
// that only look at the agentic figures.
type UsageBreakdown struct {
	ResourceType  string         `json:"resourceType,omitempty"`
	CurrentUsage  float64        `json:"currentUsage,omitempty"`
	UsageLimit    float64        `json:"usageLimit,omitempty"`
	Currency      string         `json:"currency,omitempty"`
	Unit          string         `json:"unit,omitempty"`
	OverageRate   float64        `json:"overageRate,omitempty"`
	FreeTrialInfo *FreeTrialInfo `json:"freeTrialInfo,omitempty"`
}

// FreeTrialInfo describes any active free-trial window. Status is one of
// ACTIVE / EXPIRED / NONE.
type FreeTrialInfo struct {
	CurrentUsage    float64     `json:"currentUsage,omitempty"`
	UsageLimit      float64     `json:"usageLimit,omitempty"`
	FreeTrialStatus string      `json:"freeTrialStatus,omitempty"`
	FreeTrialExpiry json.Number `json:"freeTrialExpiry,omitempty"`
}

// FetchProfile calls Kiro getUsageLimits with the bearer token and returns
// the user's email and userId. Returns an error on non-200; success with
// empty userInfo returns a Profile with empty fields rather than an error
// (some accounts are created without an email).
func FetchProfile(accessToken, proxyURL string) (*Profile, error) {
	return fetchProfileAt(resolvedProfileURL(), accessToken, HTTPClient(proxyURL))
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

// FetchUsageLimits returns the full /getUsageLimits response. Used by
// the periodic quota fetcher; FetchProfile remains for the one-off
// account-naming case.
func FetchUsageLimits(accessToken, proxyURL string) (*UsageLimits, error) {
	return fetchUsageLimitsAt(resolvedProfileURL(), accessToken, HTTPClient(proxyURL))
}

func fetchUsageLimitsAt(url, accessToken string, client *http.Client) (*UsageLimits, error) {
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
		return nil, fmt.Errorf("kiro usage limits: HTTP %d: %s", resp.StatusCode, string(body))
	}
	var parsed UsageLimits
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("kiro usage limits: decode: %w", err)
	}
	return &parsed, nil
}

// AgenticUsage extracts the AGENTIC_REQUEST row from a UsageLimits
// response. Returns nil when the response has no agentic-request row.
func (u *UsageLimits) AgenticUsage() *UsageBreakdown {
	if u == nil {
		return nil
	}
	for i := range u.UsageBreakdown {
		if u.UsageBreakdown[i].ResourceType == "AGENTIC_REQUEST" {
			return &u.UsageBreakdown[i]
		}
	}
	return nil
}
