package upstreamstation

import (
	"fmt"
	"math"
	"strings"
	"time"
)

const managedAPIKeyPrefix = "sub2api-auto-"

const (
	SiteTypeAuto    = "auto"
	SiteTypeNewAPI  = "newapi"
	SiteTypeSub2API = "sub2api"

	CredentialModePassword = "password"
	CredentialModeToken    = "token"
	CredentialModeAPIKey   = "api_key"

	RechargeSourceManual = "manual"
	RechargeSourceAuto   = "auto"

	HealthStatusUnknown = "unknown"
	HealthStatusHealthy = "healthy"
	HealthStatusError   = "error"
)

type Station struct {
	ID                   int64      `json:"id"`
	Name                 string     `json:"name"`
	SiteType             string     `json:"site_type"`
	BaseURL              string     `json:"base_url"`
	CredentialMode       string     `json:"credential_mode"`
	CredentialCipher     string     `json:"-"`
	CredentialConfigured bool       `json:"credential_configured"`
	RechargeMultiplier   float64    `json:"recharge_multiplier"`
	RechargeSource       string     `json:"recharge_source"`
	Balance              *float64   `json:"balance,omitempty"`
	Enabled              bool       `json:"enabled"`
	AutoSync             bool       `json:"auto_sync"`
	HealthStatus         string     `json:"health_status"`
	LastError            string     `json:"last_error,omitempty"`
	LastSyncAt           *time.Time `json:"last_sync_at,omitempty"`
	LastTestAt           *time.Time `json:"last_test_at,omitempty"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
}

type Route struct {
	ID                 int64      `json:"id"`
	StationID          int64      `json:"station_id"`
	RemoteGroupKey     string     `json:"remote_group_key"`
	RemoteGroupName    string     `json:"remote_group_name"`
	Platform           string     `json:"platform"`
	Models             []string   `json:"models"`
	GroupRate          float64    `json:"group_rate"`
	RechargeMultiplier float64    `json:"recharge_multiplier"`
	EffectiveRate      float64    `json:"effective_rate"`
	FixedRoute         bool       `json:"fixed_route"`
	RemoteAPIKeyID     string     `json:"remote_api_key_id,omitempty"`
	APIKeyCipher       string     `json:"-"`
	ManagedAccountID   *int64     `json:"managed_account_id,omitempty"`
	Schedulable        bool       `json:"schedulable"`
	HealthStatus       string     `json:"health_status"`
	LastError          string     `json:"last_error,omitempty"`
	LastTestAt         *time.Time `json:"last_test_at,omitempty"`
	LastSyncAt         *time.Time `json:"last_sync_at,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

type RateSnapshot struct {
	ID                 int64     `json:"id"`
	RouteID            int64     `json:"route_id"`
	GroupRate          float64   `json:"group_rate"`
	RechargeMultiplier float64   `json:"recharge_multiplier"`
	EffectiveRate      float64   `json:"effective_rate"`
	SampledAt          time.Time `json:"sampled_at"`
}

type SyncLog struct {
	ID        int64     `json:"id"`
	StationID int64     `json:"station_id"`
	Action    string    `json:"action"`
	Success   bool      `json:"success"`
	Message   string    `json:"message,omitempty"`
	Detail    string    `json:"detail,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// EffectiveRate converts an upstream group rate into the locally comparable P value.
func EffectiveRate(groupRate, rechargeMultiplier float64) float64 {
	if rechargeMultiplier <= 0 {
		rechargeMultiplier = 1
	}
	return math.Round((groupRate/rechargeMultiplier)*1e8) / 1e8
}

// ManagedAPIKeyName returns a deterministic name for keys owned by this module.
func ManagedAPIKeyName(stationID int64, remoteGroup string) string {
	var slug strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(strings.TrimSpace(remoteGroup)) {
		isASCIIAlphaNum := r >= 'a' && r <= 'z' || r >= '0' && r <= '9'
		if isASCIIAlphaNum {
			slug.WriteRune(r)
			lastDash = false
			continue
		}
		if slug.Len() > 0 && !lastDash {
			slug.WriteByte('-')
			lastDash = true
		}
	}
	group := strings.Trim(slug.String(), "-")
	if group == "" {
		group = "group"
	}
	return fmt.Sprintf("%s%d-%s", managedAPIKeyPrefix, stationID, group)
}
