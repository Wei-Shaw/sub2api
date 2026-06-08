package service

import (
	"net"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	resinpkg "github.com/Wei-Shaw/sub2api/internal/pkg/resin"
)

const (
	ProxyProtocolHTTP       = "http"
	ProxyProtocolHTTPS      = "https"
	ProxyProtocolSOCKS5     = "socks5"
	ProxyProtocolSOCKS5H    = "socks5h"
	ProxyProtocolResinHTTP  = "resin_http"
	ProxyProtocolResinHTTPS = "resin_https"
	ProxyProtocolResinSOCKS = "resin_socks5"
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
	BasePath       string
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

func (p *Proxy) IsResin() bool {
	if p == nil {
		return false
	}
	return isResinProtocol(strings.TrimSpace(p.Protocol))
}

func (p *Proxy) transportScheme() string {
	if p == nil {
		return ""
	}
	switch strings.TrimSpace(p.Protocol) {
	case ProxyProtocolResinHTTP:
		return ProxyProtocolHTTP
	case ProxyProtocolResinHTTPS:
		return ProxyProtocolHTTPS
	case ProxyProtocolResinSOCKS:
		return ProxyProtocolSOCKS5H
	default:
		return strings.TrimSpace(p.Protocol)
	}
}

// IsExpired 报告代理是否已过期（基于 expires_at，与 status 无关）。
func (p *Proxy) IsExpired(now time.Time) bool {
	return p.ExpiresAt != nil && !p.ExpiresAt.After(now)
}

func (p *Proxy) URL() string {
	u := &url.URL{
		Scheme: p.transportScheme(),
		Host:   net.JoinHostPort(p.Host, strconv.Itoa(p.Port)),
	}
	if resinPath := p.resinBasePath(); resinPath != "" {
		u.Path = resinPath
	} else if p.BasePath != "" {
		u.Path = p.BasePath
	}
	if p.IsResin() {
		if p.Username != "" && p.Password != "" {
			u.User = url.UserPassword(p.Username, p.Password)
		} else if p.Username != "" {
			u.User = url.User(p.Username)
		}
		u.Fragment = resinpkg.ProxyMarker
		return u.String()
	}
	if p.Username != "" && p.Password != "" {
		u.User = url.UserPassword(p.Username, p.Password)
	}
	return u.String()
}

func (p *Proxy) ResinConfig() (*resinpkg.Config, error) {
	if p == nil || !p.IsResin() {
		return nil, nil
	}
	return resinpkg.Parse(p.URL())
}

func (p *Proxy) resinBasePath() string {
	if p == nil || !p.IsResin() {
		return ""
	}
	if !supportsResinReversePath(strings.TrimSpace(p.Protocol)) {
		return ""
	}
	if strings.TrimSpace(p.BasePath) != "" {
		return p.BasePath
	}
	token := strings.TrimSpace(p.Password)
	if token == "" {
		return ""
	}
	return "/" + strings.TrimLeft(token, "/")
}

func normalizeProxyBasePath(protocol, raw string) (string, error) {
	if !isResinProtocol(strings.TrimSpace(protocol)) {
		return "", nil
	}
	if !supportsResinReversePath(strings.TrimSpace(protocol)) {
		return "", nil
	}
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", nil
	}
	if strings.Contains(trimmed, "://") || strings.HasPrefix(trimmed, "//") {
		return "", infraerrors.BadRequest("PROXY_BASE_PATH_INVALID", "Resin base path must not include scheme or host")
	}
	if strings.ContainsAny(trimmed, "?#") {
		return "", infraerrors.BadRequest("PROXY_BASE_PATH_INVALID", "Resin base path must not include query or fragment")
	}
	cleaned := path.Clean("/" + strings.TrimLeft(trimmed, "/"))
	if cleaned == "/" || cleaned == "." {
		return "", nil
	}
	return cleaned, nil
}

func validateResinProxyCredentials(protocol, username, password string) error {
	if !isResinProtocol(strings.TrimSpace(protocol)) {
		return nil
	}
	if strings.TrimSpace(username) == "" {
		return infraerrors.BadRequest("PROXY_RESIN_PLATFORM_REQUIRED", "Resin platform is required")
	}
	return nil
}

func isResinProtocol(protocol string) bool {
	switch strings.TrimSpace(protocol) {
	case ProxyProtocolResinHTTP, ProxyProtocolResinHTTPS, ProxyProtocolResinSOCKS:
		return true
	default:
		return false
	}
}

func supportsResinReversePath(protocol string) bool {
	switch strings.TrimSpace(protocol) {
	case ProxyProtocolResinHTTP, ProxyProtocolResinHTTPS:
		return true
	default:
		return false
	}
}

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

type ProxyAccountSummary struct {
	ID       int64
	Name     string
	Platform string
	Type     string
	Notes    *string
}
