package service

import (
	"net"
	"net/url"
	"strconv"
	"time"
)

const (
	FallbackModeNone   = "none"
	FallbackModeProxy  = "proxy"
	FallbackModeDirect = "direct"
)

type Proxy struct {
	ID             int64      `json:"id"`
	Name           string     `json:"name"`
	Protocol       string     `json:"protocol"`
	Host           string     `json:"host"`
	Port           int        `json:"port"`
	Username       string     `json:"username"`
	Password       string     `json:"password"`
	Status         string     `json:"status"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	ExpiresAt      *time.Time `json:"expires_at"`
	FallbackMode   string     `json:"fallback_mode"`
	BackupProxyID  *int64     `json:"backup_proxy_id"`
	ExpiryWarnDays int        `json:"expiry_warn_days"`
}

func (p *Proxy) IsActive() bool {
	return p.Status == StatusActive
}

// IsExpired 报告代理是否已过期（基于 expires_at，与 status 无关）。
func (p *Proxy) IsExpired(now time.Time) bool {
	return p.ExpiresAt != nil && !p.ExpiresAt.After(now)
}

func (p *Proxy) URL() string {
	u := &url.URL{
		Scheme: p.Protocol,
		Host:   net.JoinHostPort(p.Host, strconv.Itoa(p.Port)),
	}
	if p.Username != "" && p.Password != "" {
		u.User = url.UserPassword(p.Username, p.Password)
	}
	return u.String()
}

type ProxyWithAccountCount struct {
	Proxy
	AccountCount   int64  `json:"account_count"`
	LatencyMs      *int64 `json:"latency_ms,omitempty"`
	LatencyStatus  string `json:"latency_status"`
	LatencyMessage string `json:"latency_message"`
	IPAddress      string `json:"ip_address"`
	Country        string `json:"country"`
	CountryCode    string `json:"country_code"`
	Region         string `json:"region"`
	City           string `json:"city"`
	QualityStatus  string `json:"quality_status"`
	QualityScore   *int   `json:"quality_score,omitempty"`
	QualityGrade   string `json:"quality_grade"`
	QualitySummary string `json:"quality_summary"`
	QualityChecked *int64 `json:"quality_checked,omitempty"`
}

type ProxyAccountSummary struct {
	ID       int64
	Name     string
	Platform string
	Type     string
	Notes    *string
}
