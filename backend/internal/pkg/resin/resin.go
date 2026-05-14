package resin

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/proxyurl"
)

const HeaderAccount = "X-Resin-Account"
const ProxyMarker = "resin"

type accountContextKey struct{}

type Config struct {
	BaseURL  *url.URL
	Platform string
	Secret   string
}

func Parse(rawURL string) (*Config, error) {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return nil, nil
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return nil, fmt.Errorf("invalid resin url: %w", err)
	}
	if !isMarkedProxyURL(parsed) {
		return nil, nil
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("invalid resin url: missing scheme or host")
	}

	platform := strings.TrimSpace(platformFromURL(parsed))
	if platform == "" {
		return nil, fmt.Errorf("resin platform name is empty")
	}
	secret := secretFromMarkedProxyURL(parsed)

	base := *parsed
	base.User = nil
	q := base.Query()
	q.Del("token")
	q.Del("secret")
	q.Del("resin")
	base.RawQuery = q.Encode()
	base.Fragment = ""

	return &Config{
		BaseURL:  &base,
		Platform: platform,
		Secret:   secret,
	}, nil
}

func isMarkedProxyURL(u *url.URL) bool {
	if u == nil {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(u.Fragment), ProxyMarker) {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(u.Query().Get("resin")), "1") ||
		strings.EqualFold(strings.TrimSpace(u.Query().Get("resin")), "true")
}

func platformFromURL(u *url.URL) string {
	if u == nil || u.User == nil {
		return ""
	}
	username := strings.TrimSpace(u.User.Username())
	if username == "" {
		return ""
	}
	if idx := strings.Index(username, "."); idx > 0 {
		return strings.TrimSpace(username[:idx])
	}
	return username
}

func secretFromMarkedProxyURL(u *url.URL) string {
	if u == nil {
		return ""
	}
	if u.User != nil {
		if password, ok := u.User.Password(); ok && strings.TrimSpace(password) != "" {
			return strings.TrimSpace(password)
		}
	}
	query := u.Query()
	if value := strings.TrimSpace(query.Get("token")); value != "" {
		return value
	}
	return strings.TrimSpace(query.Get("secret"))
}

func reverseProtocol(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "ws":
		return "http"
	case "wss":
		return "https"
	default:
		return strings.ToLower(strings.TrimSpace(raw))
	}
}

func AccountKey(accountID int64) string {
	return "acct-" + strconv.FormatInt(accountID, 10)
}

func WithAccountID(ctx context.Context, accountID int64) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if accountID <= 0 {
		return ctx
	}
	return context.WithValue(ctx, accountContextKey{}, accountID)
}

func AccountIDFromContext(ctx context.Context) int64 {
	if ctx == nil {
		return 0
	}
	raw := ctx.Value(accountContextKey{})
	switch v := raw.(type) {
	case int64:
		return v
	case int:
		return int64(v)
	default:
		return 0
	}
}

func (c *Config) ForwardProxyBaseURL() string {
	if c == nil || c.BaseURL == nil {
		return ""
	}
	cloned := c.forwardProxyURL()
	return cloned.String()
}

func (c *Config) BaseAuthProxyURL() string {
	if c == nil || c.BaseURL == nil {
		return ""
	}
	cloned := c.forwardProxyURL()
	cloned.User = url.UserPassword(strings.TrimSpace(c.Platform), c.Secret)
	return cloned.String()
}

func (c *Config) ForwardProxyURLForAccount(accountID int64) string {
	if c == nil || c.BaseURL == nil || accountID <= 0 {
		return ""
	}
	cloned := c.forwardProxyURL()
	cloned.User = url.UserPassword(c.forwardProxyUsername(accountID), c.Secret)
	return cloned.String()
}

func (c *Config) forwardProxyUsername(accountID int64) string {
	return strings.TrimSpace(c.Platform) + "." + AccountKey(accountID)
}

func (c *Config) ProxyAuthorizationHeader(accountID int64) string {
	if c == nil || accountID <= 0 || c.UsesSOCKS5ForwardProxy() {
		return ""
	}
	token := base64.StdEncoding.EncodeToString([]byte(c.forwardProxyUsername(accountID) + ":" + c.Secret))
	return "Basic " + token
}

func (c *Config) ProxyConnectHeadersForContext(ctx context.Context) http.Header {
	if c == nil || c.UsesSOCKS5ForwardProxy() {
		return nil
	}
	accountID := AccountIDFromContext(ctx)
	if accountID <= 0 {
		return nil
	}
	value := c.ProxyAuthorizationHeader(accountID)
	if value == "" {
		return nil
	}
	headers := make(http.Header, 1)
	headers.Set("Proxy-Authorization", value)
	return headers
}

func (c *Config) MatchesForwardProxyURL(raw string) bool {
	if c == nil {
		return false
	}
	normalized, parsed, err := proxyurl.Parse(raw)
	if err != nil || normalized == "" {
		return false
	}
	if parsed != nil {
		parsed.User = nil
		q := parsed.Query()
		q.Del("token")
		q.Del("secret")
		q.Del("resin")
		parsed.RawQuery = q.Encode()
		parsed.Fragment = ""
		normalized = parsed.String()
	}
	expected, _, err := proxyurl.Parse(c.ForwardProxyBaseURL())
	if err != nil {
		return false
	}
	return normalized == expected
}

func (c *Config) Scheme() string {
	if c == nil || c.BaseURL == nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(c.BaseURL.Scheme))
}

func (c *Config) UsesSOCKS5ForwardProxy() bool {
	switch c.Scheme() {
	case "socks5", "socks5h":
		return true
	default:
		return false
	}
}

func (c *Config) SupportsReverseProxy() bool {
	return !c.UsesSOCKS5ForwardProxy()
}

func (c *Config) forwardProxyURL() url.URL {
	cloned := *c.BaseURL
	cloned.Path = ""
	cloned.RawPath = ""
	cloned.RawQuery = ""
	cloned.Fragment = ""
	return cloned
}

func (c *Config) BuildReverseURL(original *url.URL) (*url.URL, error) {
	if c == nil || c.BaseURL == nil {
		return nil, fmt.Errorf("resin config is nil")
	}
	if !c.SupportsReverseProxy() {
		return nil, fmt.Errorf("resin reverse proxy is not supported for %s", c.Scheme())
	}
	if original == nil {
		return nil, fmt.Errorf("original url is nil")
	}
	protocol := reverseProtocol(original.Scheme)
	if protocol == "" {
		return nil, fmt.Errorf("original url missing scheme")
	}
	host := strings.TrimSpace(original.Host)
	if host == "" {
		return nil, fmt.Errorf("original url missing host")
	}
	cloned := *c.BaseURL
	cloned.Path = joinURLPath(c.BaseURL.Path, c.Platform, protocol, host, original.Path)
	cloned.RawPath = ""
	cloned.RawQuery = mergeQueries(c.BaseURL.RawQuery, original.RawQuery)
	cloned.Fragment = ""
	return &cloned, nil
}

func PrepareReverseRequest(req *http.Request, cfg *Config, accountID int64) (*http.Request, error) {
	if req == nil {
		return nil, fmt.Errorf("request is nil")
	}
	if req.URL == nil {
		return nil, fmt.Errorf("request url is nil")
	}
	reverseURL, err := cfg.BuildReverseURL(req.URL)
	if err != nil {
		return nil, err
	}
	cloned := req.Clone(WithAccountID(req.Context(), accountID))
	cloned.URL = reverseURL
	cloned.Host = req.URL.Host
	cloned.Header = req.Header.Clone()
	if accountID > 0 {
		cloned.Header.Set(HeaderAccount, AccountKey(accountID))
	}
	return cloned, nil
}

func PrepareReverseWS(rawWSURL string, headers http.Header, cfg *Config, accountID int64) (string, string, http.Header, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawWSURL))
	if err != nil {
		return "", "", nil, fmt.Errorf("invalid websocket url: %w", err)
	}
	reverseURL, err := cfg.BuildReverseURL(parsed)
	if err != nil {
		return "", "", nil, err
	}
	clonedHeaders := cloneHeader(headers)
	if accountID > 0 {
		clonedHeaders.Set(HeaderAccount, AccountKey(accountID))
	}
	return reverseURL.String(), parsed.Host, clonedHeaders, nil
}

func WrapForwardProxyRoundTripper(base http.RoundTripper, cfg *Config) http.RoundTripper {
	if base == nil || cfg == nil || cfg.UsesSOCKS5ForwardProxy() {
		return base
	}
	return roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req == nil {
			return base.RoundTrip(req)
		}
		accountID := AccountIDFromContext(req.Context())
		if accountID <= 0 {
			return base.RoundTrip(req)
		}
		clone := req.Clone(req.Context())
		clone.Header = req.Header.Clone()
		if value := cfg.ProxyAuthorizationHeader(accountID); value != "" {
			clone.Header.Set("Proxy-Authorization", value)
		}
		return base.RoundTrip(clone)
	})
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func cloneHeader(header http.Header) http.Header {
	if header == nil {
		return make(http.Header)
	}
	return header.Clone()
}

func joinURLPath(basePath string, segments ...string) string {
	parts := make([]string, 0, len(segments)+1)
	if strings.TrimSpace(basePath) != "" {
		parts = append(parts, basePath)
	}
	for _, segment := range segments {
		if strings.TrimSpace(segment) == "" {
			continue
		}
		parts = append(parts, segment)
	}
	if len(parts) == 0 {
		return "/"
	}
	joined := path.Join(parts...)
	if strings.HasPrefix(joined, "/") {
		return joined
	}
	return "/" + joined
}

func mergeQueries(baseQuery, originalQuery string) string {
	switch {
	case baseQuery == "":
		return originalQuery
	case originalQuery == "":
		return baseQuery
	default:
		return baseQuery + "&" + originalQuery
	}
}
