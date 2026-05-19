// Package kiro contains provider-agnostic helpers for interacting with the
// Kiro / CodeWhisperer upstream. Mirrors the layout of internal/pkg/antigravity.
//
// This package is the lowest layer of the Kiro integration: it knows about
// HTTP, Kiro's OAuth endpoints, and the upstream wire format, but nothing
// about the DB, Gin, or our internal Account model. The internal/service
// layer wires this up to the rest of the system.
package kiro

// AuthMethod identifies how a Kiro account was authenticated.
// Stored on account.credentials.auth_method and used by RefreshToken to
// pick the correct refresh endpoint.
type AuthMethod string

const (
	AuthMethodSocial    AuthMethod = "social"
	AuthMethodIdC       AuthMethod = "idc"
	AuthMethodBuilderID AuthMethod = "builderid"
)

// TokenInfo is the canonical token shape returned by all three auth flows.
// All times are Unix seconds.
type TokenInfo struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresAt    int64  `json:"expires_at"`
	ProfileARN   string `json:"profile_arn,omitempty"`

	// AuthMethod identifies which refresh endpoint to use later.
	AuthMethod AuthMethod `json:"auth_method"`

	// IdC / BuilderID extras (empty for Social). Populated by phase 3.
	ClientID     string `json:"client_id,omitempty"`
	ClientSecret string `json:"client_secret,omitempty"`
	Region       string `json:"region,omitempty"`
	StartURL     string `json:"start_url,omitempty"`

	// Profile data, populated post-token for naming the account.
	Email  string `json:"email,omitempty"`
	UserID string `json:"user_id,omitempty"`
}

// RefreshableAccount is the minimal account shape the refresh helpers need.
// Decouples pkg/kiro from internal/service.Account so this package can be
// imported by tests and tools without pulling in Ent.
type RefreshableAccount struct {
	AuthMethod   AuthMethod
	RefreshToken string
	ClientID     string
	ClientSecret string
	Region       string
	ProxyURL     string
}
