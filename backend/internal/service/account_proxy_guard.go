package service

import (
	"context"
	"log/slog"
	"strings"
)

// FailClosedProxyURL is returned instead of "" when an account is bound to a
// proxy that cannot be resolved. It points at a local port nobody listens on, so
// the transport fails immediately instead of silently dialing upstream directly.
//
// Background: historically call sites used
//
//	proxyURL := ""
//	if account.ProxyID != nil && account.Proxy != nil { proxyURL = account.Proxy.URL() }
//
// and an empty proxyURL means "direct". Any failure to load the proxy (DB error,
// soft-deleted row, stale cache entry) therefore leaked the request out of the
// host's own IP. Every account-scoped proxy lookup must go through the helpers
// below, which never return "" for an account that has a ProxyID.
const FailClosedProxyURL = "socks5h://127.0.0.1:1"

// AccountProxyURLFailClosed returns the proxy URL to use for account.
//   - account without ProxyID → "" (direct, as configured)
//   - account with ProxyID and a matching loaded Proxy → that proxy's URL
//   - account with ProxyID but no usable Proxy → FailClosedProxyURL
func AccountProxyURLFailClosed(account *Account) string {
	if account == nil || account.ProxyID == nil {
		return ""
	}
	// Proxy.ID == 0 means a hand-built Proxy (never from the DB); a non-zero
	// mismatching ID means a stale cached Proxy and is rejected.
	if account.Proxy != nil && (account.Proxy.ID == 0 || account.Proxy.ID == *account.ProxyID) {
		if u := strings.TrimSpace(account.Proxy.URL()); u != "" && strings.TrimSpace(account.Proxy.Host) != "" {
			return u
		}
	}
	slog.Error("account_proxy_fail_closed",
		"account_id", account.ID,
		"proxy_id", *account.ProxyID,
		"reason", "bound proxy not loaded; refusing direct connection")
	return FailClosedProxyURL
}

// ResolveAccountProxyURLFailClosed is like AccountProxyURLFailClosed but loads
// the proxy from repo (fresh read). Any lookup error or missing row yields
// FailClosedProxyURL, never "".
func ResolveAccountProxyURLFailClosed(ctx context.Context, repo ProxyRepository, account *Account) string {
	if account == nil || account.ProxyID == nil {
		return ""
	}
	return ResolveProxyIDURLFailClosed(ctx, repo, account.ID, account.ProxyID)
}

// ResolveProxyIDURLFailClosed resolves proxyID through repo. nil proxyID → "".
// accountID is only used for logging (pass 0 when not account-scoped).
func ResolveProxyIDURLFailClosed(ctx context.Context, repo ProxyRepository, accountID int64, proxyID *int64) string {
	if proxyID == nil {
		return ""
	}
	if repo != nil {
		p, err := repo.GetByID(ctx, *proxyID)
		if err == nil && p != nil && strings.TrimSpace(p.Host) != "" {
			if u := strings.TrimSpace(p.URL()); u != "" {
				return u
			}
		}
		slog.Error("account_proxy_fail_closed",
			"account_id", accountID,
			"proxy_id", *proxyID,
			"reason", "bound proxy lookup failed; refusing direct connection",
			"error", err)
		return FailClosedProxyURL
	}
	slog.Error("account_proxy_fail_closed",
		"account_id", accountID,
		"proxy_id", *proxyID,
		"reason", "no proxy repository; refusing direct connection")
	return FailClosedProxyURL
}

// LoadAccountProxyURLFailClosed uses account.Proxy when it is already loaded
// and usable; otherwise it loads the proxy from repo and caches it on
// account.Proxy so later lookups in the same request skip the DB. A lookup
// failure yields FailClosedProxyURL, never "".
func LoadAccountProxyURLFailClosed(ctx context.Context, repo ProxyRepository, account *Account) string {
	if account == nil || account.ProxyID == nil {
		return ""
	}
	if account.Proxy != nil && (account.Proxy.ID == 0 || account.Proxy.ID == *account.ProxyID) {
		return AccountProxyURLFailClosed(account)
	}
	if repo == nil {
		return ResolveAccountProxyURLFailClosed(ctx, nil, account)
	}
	p, err := repo.GetByID(ctx, *account.ProxyID)
	if err == nil && p != nil && strings.TrimSpace(p.Host) != "" {
		account.Proxy = p
		return AccountProxyURLFailClosed(account)
	}
	slog.Error("account_proxy_fail_closed",
		"account_id", account.ID,
		"proxy_id", *account.ProxyID,
		"reason", "bound proxy lookup failed; refusing direct connection",
		"error", err)
	return FailClosedProxyURL
}
