package main

import "time"

const (
	DefaultBaseURL      = "https://api.x.ai/v1"
	DefaultAuthorizeURL = "https://auth.x.ai/oauth2/authorize"
	DefaultTokenURL     = "https://auth.x.ai/oauth2/token"
	DefaultClientID     = "b1a00492-073a-47ea-816f-4c329264a828"
	DefaultScope        = "openid profile email offline_access grok-cli:access api:access"
	DefaultRedirectURI  = "http://127.0.0.1:56121/callback"
)

type Account struct {
	ID            int64      `json:"id"`
	Name          string     `json:"name"`
	Enabled       bool       `json:"enabled"`
	AccessToken   string     `json:"access_token,omitempty"`
	RefreshToken  string     `json:"refresh_token,omitempty"`
	IDToken       string     `json:"id_token,omitempty"`
	TokenType     string     `json:"token_type,omitempty"`
	ExpiresAt     *time.Time `json:"expires_at,omitempty"`
	ClientID      string     `json:"client_id,omitempty"`
	Scope         string     `json:"scope,omitempty"`
	Email         string     `json:"email,omitempty"`
	BaseURL       string     `json:"base_url"`
	ProxyURL      string     `json:"proxy_url,omitempty"`
	Concurrency   int        `json:"concurrency"`
	Priority      int        `json:"priority"`
	InFlight      int        `json:"in_flight"`
	LastUsedAt    *time.Time `json:"last_used_at,omitempty"`
	CooldownUntil *time.Time `json:"cooldown_until,omitempty"`
	LastError     string     `json:"last_error,omitempty"`
	LastQuota     any        `json:"last_quota,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

type APIKey struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Key       string    `json:"key,omitempty"`
	Prefix    string    `json:"prefix"`
	CreatedAt time.Time `json:"created_at"`
}

type VideoBinding struct {
	RequestID string    `json:"request_id"`
	AccountID int64     `json:"account_id"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

type RequestLog struct {
	Time       time.Time `json:"time"`
	Endpoint   string    `json:"endpoint"`
	AccountID  int64     `json:"account_id,omitempty"`
	StatusCode int       `json:"status_code,omitempty"`
	Message    string    `json:"message,omitempty"`
}

type Model struct {
	ID          string `json:"id"`
	Object      string `json:"object"`
	OwnedBy     string `json:"owned_by"`
	DisplayName string `json:"display_name,omitempty"`
}

type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token,omitempty"`
	IDToken      string `json:"id_token,omitempty"`
	TokenType    string `json:"token_type,omitempty"`
	ExpiresIn    int64  `json:"expires_in,omitempty"`
	Scope        string `json:"scope,omitempty"`
}

type TokenInfo struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token,omitempty"`
	IDToken      string `json:"id_token,omitempty"`
	TokenType    string `json:"token_type,omitempty"`
	ExpiresIn    int64  `json:"expires_in"`
	ExpiresAt    int64  `json:"expires_at"`
	ClientID     string `json:"client_id,omitempty"`
	Scope        string `json:"scope,omitempty"`
	Email        string `json:"email,omitempty"`
}

type PublicAccount struct {
	ID              int64      `json:"id"`
	Name            string     `json:"name"`
	Enabled         bool       `json:"enabled"`
	Email           string     `json:"email,omitempty"`
	BaseURL         string     `json:"base_url"`
	ProxyURL        string     `json:"proxy_url,omitempty"`
	Concurrency     int        `json:"concurrency"`
	Priority        int        `json:"priority"`
	InFlight        int        `json:"in_flight"`
	ExpiresAt       *time.Time `json:"expires_at,omitempty"`
	CooldownUntil   *time.Time `json:"cooldown_until,omitempty"`
	LastUsedAt      *time.Time `json:"last_used_at,omitempty"`
	LastError       string     `json:"last_error,omitempty"`
	HasAccessToken  bool       `json:"has_access_token"`
	HasRefreshToken bool       `json:"has_refresh_token"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	LastQuota       any        `json:"last_quota,omitempty"`
}

func (a *Account) Public() PublicAccount {
	if a == nil {
		return PublicAccount{}
	}
	return PublicAccount{
		ID:              a.ID,
		Name:            a.Name,
		Enabled:         a.Enabled,
		Email:           a.Email,
		BaseURL:         a.BaseURL,
		ProxyURL:        a.ProxyURL,
		Concurrency:     a.Concurrency,
		Priority:        a.Priority,
		InFlight:        a.InFlight,
		ExpiresAt:       a.ExpiresAt,
		CooldownUntil:   a.CooldownUntil,
		LastUsedAt:      a.LastUsedAt,
		LastError:       a.LastError,
		HasAccessToken:  a.AccessToken != "",
		HasRefreshToken: a.RefreshToken != "",
		CreatedAt:       a.CreatedAt,
		UpdatedAt:       a.UpdatedAt,
		LastQuota:       a.LastQuota,
	}
}

func DefaultModels() []Model {
	return []Model{
		{ID: "grok-4.3", Object: "model", OwnedBy: "xai", DisplayName: "Grok 4.3"},
		{ID: "grok-build-0.1", Object: "model", OwnedBy: "xai", DisplayName: "Grok Build 0.1"},
		{ID: "grok-4.20-0309-reasoning", Object: "model", OwnedBy: "xai", DisplayName: "Grok 4.20 Reasoning"},
		{ID: "grok-4.20-0309-non-reasoning", Object: "model", OwnedBy: "xai", DisplayName: "Grok 4.20 Non Reasoning"},
		{ID: "grok-4.20-multi-agent-0309", Object: "model", OwnedBy: "xai", DisplayName: "Grok 4.20 Multi Agent"},
		{ID: "grok-imagine", Object: "model", OwnedBy: "xai", DisplayName: "Grok Imagine"},
		{ID: "grok-imagine-image", Object: "model", OwnedBy: "xai", DisplayName: "Grok Imagine Image"},
		{ID: "grok-imagine-image-quality", Object: "model", OwnedBy: "xai", DisplayName: "Grok Imagine Image Quality"},
		{ID: "grok-imagine-edit", Object: "model", OwnedBy: "xai", DisplayName: "Grok Imagine Edit"},
		{ID: "grok-imagine-video", Object: "model", OwnedBy: "xai", DisplayName: "Grok Imagine Video"},
		{ID: "grok-imagine-video-1.5", Object: "model", OwnedBy: "xai", DisplayName: "Grok Imagine Video 1.5"},
	}
}

func ModelMapping() map[string]string {
	out := map[string]string{
		"grok":                    "grok-4.3",
		"grok-latest":             "grok-4.3",
		"grok-build":              "grok-build-0.1",
		"grok-4.20-reasoning":     "grok-4.20-0309-reasoning",
		"grok-4.20-non-reasoning": "grok-4.20-0309-non-reasoning",
	}
	for _, model := range DefaultModels() {
		out[model.ID] = model.ID
	}
	return out
}
