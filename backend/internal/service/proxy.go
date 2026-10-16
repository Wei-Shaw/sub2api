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

// ProxyPlatformAccountCount 是某个 proxy 下单个平台的账号统计
type ProxyPlatformAccountCount struct {
	Platform   string
	Count      int64
	ErrorCount int64
}

// ProxyAccountStats 是单个 proxy 的账号聚合结果（repo -> service 内部传递用）
type ProxyAccountStats struct {
	Total      int64
	ErrorCount int64
	Platforms  []ProxyPlatformAccountCount
}

type ProxyWithAccountCount struct {
	Proxy
	AccountCount int64
	// ErrorAccountCount 只统计 status == StatusError 的账号，
	// 与 ops_account_availability.go 的 hasError 口径保持一致
	ErrorAccountCount int64
	// PlatformCounts 按 platform 升序排列，只包含 Count > 0 的平台
	PlatformCounts []ProxyPlatformAccountCount
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

type ProxyAccountSummary struct {
	ID       int64
	Name     string
	Platform string
	Type     string
	Notes    *string
}
