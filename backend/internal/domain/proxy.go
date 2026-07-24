package domain

import (
	"net"
	"net/url"
	"strconv"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// Proxy-domain errors used by persistence and application layers.
var (
	ErrProxyNotFound = infraerrors.NotFound("PROXY_NOT_FOUND", "proxy not found")
	ErrProxyInUse    = infraerrors.Conflict("PROXY_IN_USE", "proxy is in use by accounts")
)

// Proxy fallback modes for expired-proxy account reassignment.
const (
	FallbackModeNone   = "none"
	FallbackModeProxy  = "proxy"
	FallbackModeDirect = "direct"
)

// Proxy is a network proxy aggregate used by accounts for upstream access.
type Proxy struct {
	ID             int64
	Name           string
	Protocol       string
	Host           string
	Port           int
	Username       string
	Password       string
	Status         string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	ExpiresAt      *time.Time
	FallbackMode   string
	BackupProxyID  *int64
	ExpiryWarnDays int
}

func (p *Proxy) IsActive() bool {
	return p != nil && p.Status == StatusActive
}

// IsExpired reports whether the proxy is past expires_at (independent of status).
func (p *Proxy) IsExpired(now time.Time) bool {
	return p != nil && p.ExpiresAt != nil && !p.ExpiresAt.After(now)
}

// URL builds the proxy URL including optional credentials.
func (p *Proxy) URL() string {
	if p == nil {
		return ""
	}
	u := &url.URL{
		Scheme: p.Protocol,
		Host:   net.JoinHostPort(p.Host, strconv.Itoa(p.Port)),
	}
	if p.Username != "" && p.Password != "" {
		u.User = url.UserPassword(p.Username, p.Password)
	}
	return u.String()
}

// ProxyWithAccountCount is a list projection that includes account usage stats
// and optional latency/quality snapshots.
type ProxyWithAccountCount struct {
	Proxy
	AccountCount   int64
	LatencyMs      *int64
	LatencyStatus  string
	LatencyMessage string
	IPAddress      string
	Country        string
	CountryCode    string
	Region         string
	City           string
	QualityStatus  string
	QualityScore   *int
	QualityGrade   string
	QualitySummary string
	QualityChecked *int64
}

// ProxyAccountSummary is a shallow account projection for "accounts using proxy".
type ProxyAccountSummary struct {
	ID       int64
	Name     string
	Platform string
	Type     string
	Notes    *string
}

// ProxyLatencyInfo is a cached latency/quality snapshot for a proxy.
type ProxyLatencyInfo struct {
	Success          bool      `json:"success"`
	LatencyMs        *int64    `json:"latency_ms,omitempty"`
	Message          string    `json:"message,omitempty"`
	IPAddress        string    `json:"ip_address,omitempty"`
	Country          string    `json:"country,omitempty"`
	CountryCode      string    `json:"country_code,omitempty"`
	Region           string    `json:"region,omitempty"`
	City             string    `json:"city,omitempty"`
	QualityStatus    string    `json:"quality_status,omitempty"`
	QualityScore     *int      `json:"quality_score,omitempty"`
	QualityGrade     string    `json:"quality_grade,omitempty"`
	QualitySummary   string    `json:"quality_summary,omitempty"`
	QualityCheckedAt *int64    `json:"quality_checked_at,omitempty"`
	QualityCFRay     string    `json:"quality_cf_ray,omitempty"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// ProxyExitInfo is the exit IP geo snapshot returned by proxy probes.
type ProxyExitInfo struct {
	IP          string
	City        string
	Region      string
	Country     string
	CountryCode string
}

// ResolveProxyFallbackTarget computes where accounts on an expired proxy start
// should be reassigned.
//
// Returns (targetID, change):
//   - change=false: leave accounts unchanged (mode=none, cycle, or unresolved)
//   - change=true, targetID=nil: reassign to direct connection
//   - change=true, targetID!=nil: reassign to that backup proxy id
//
// byID is a full proxy snapshot (id -> Proxy); now is the evaluation time.
func ResolveProxyFallbackTarget(start Proxy, byID map[int64]Proxy, now time.Time) (*int64, bool) {
	switch start.FallbackMode {
	case FallbackModeDirect:
		return nil, true
	case FallbackModeProxy:
		visited := map[int64]struct{}{start.ID: {}}
		curID := start.BackupProxyID
		for {
			if curID == nil {
				return nil, false
			}
			if _, seen := visited[*curID]; seen {
				return nil, false
			}
			p, ok := byID[*curID]
			if !ok {
				return nil, false
			}
			if !(&p).IsExpired(now) && p.Status != StatusExpired {
				id := p.ID
				return &id, true
			}
			visited[*curID] = struct{}{}
			switch p.FallbackMode {
			case FallbackModeDirect:
				return nil, true
			case FallbackModeProxy:
				curID = p.BackupProxyID
			default:
				return nil, false
			}
		}
	default:
		return nil, false
	}
}
