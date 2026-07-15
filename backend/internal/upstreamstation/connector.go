package upstreamstation

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"
)

type Credentials struct {
	Username     string         `json:"username,omitempty"`
	Password     string         `json:"password,omitempty"`
	AccessToken  string         `json:"access_token,omitempty"`
	RefreshToken string         `json:"refresh_token,omitempty"`
	Cookie       string         `json:"cookie,omitempty"`
	UserID       string         `json:"user_id,omitempty"`
	APIKey       string         `json:"api_key,omitempty"`
	Extra        map[string]any `json:"extra,omitempty"`
}

type Session struct {
	AccessToken  string
	RefreshToken string
	Cookie       string
	UserID       string
	ExpiresAt    time.Time
}

type RemoteGroup struct {
	Key         string
	Name        string
	Description string
	Rate        float64
	Platform    string
}

type ManagedKey struct {
	ID       string
	Name     string
	GroupKey string
	Key      string
	Status   string
}

type Connector interface {
	Type() string
	Detect(ctx context.Context, baseURL string) (bool, error)
	Authenticate(ctx context.Context, station *Station, credentials Credentials) (*Session, error)
	GetBalance(ctx context.Context, baseURL string, session *Session) (float64, error)
	ListGroups(ctx context.Context, baseURL string, session *Session) ([]RemoteGroup, error)
	GetRechargeMultiplier(ctx context.Context, baseURL string, session *Session) (float64, error)
}

type APIKeyManager interface {
	ListAPIKeys(ctx context.Context, baseURL string, session *Session) ([]ManagedKey, error)
	CreateAPIKey(ctx context.Context, baseURL string, session *Session, name string, group RemoteGroup) (*ManagedKey, error)
	RevealAPIKey(ctx context.Context, baseURL string, session *Session, id string) (string, error)
}

type ModelDiscoverer interface {
	ListModels(ctx context.Context, baseURL, apiKey, platform string) ([]string, error)
}

func connectorHTTPClient(client *http.Client) *http.Client {
	if client != nil {
		return client
	}
	return &http.Client{Timeout: 30 * time.Second}
}

func normalizeBaseURL(value string) string {
	return strings.TrimRight(strings.TrimSpace(value), "/")
}

func validateSession(session *Session) error {
	if session == nil {
		return errors.New("upstream session is required")
	}
	return nil
}
