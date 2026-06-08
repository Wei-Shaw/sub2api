package pluginsdk

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
)

// Sentinel errors returned by SafeHTTPClient. Plugins can errors.Is() against
// these to distinguish SSRF blocks from regular network errors.
var (
	ErrBlockedTarget    = errors.New("safeurl: target IP in block list")
	ErrTooManyRedirects = errors.New("safeurl: redirect cap reached")
	ErrBodyTooLarge     = errors.New("safeurl: response body exceeds max bytes")
	ErrHostNotAllowed   = errors.New("safeurl: host not in allow-list")
)

// defaultBlockedCIDRs are the SSRF-relevant ranges blocked when the host does
// not provide its own list. Sources: RFC1918 + loopback + link-local +
// IPv4 multicast + CGNAT + IPv6 ULA / link-local / loopback.
var defaultBlockedCIDRs = []string{
	"127.0.0.0/8",    // IPv4 loopback
	"10.0.0.0/8",     // RFC1918
	"172.16.0.0/12",  // RFC1918
	"192.168.0.0/16", // RFC1918
	"169.254.0.0/16", // link-local (cloud metadata 169.254.169.254)
	"100.64.0.0/10",  // CGNAT
	"0.0.0.0/8",      // "this network"
	"224.0.0.0/4",    // multicast
	"::1/128",        // IPv6 loopback
	"fc00::/7",       // IPv6 ULA
	"fe80::/10",      // IPv6 link-local
}

// outboundDefaults holds the host-pushed defaults captured at Init time. The
// SDK runner populates it; NewSafeHTTPClient reads it under RLock so callers
// always see a coherent snapshot.
type outboundDefaultsSnapshot struct {
	blockedCIDRs []string
	allowedHosts []string
	maxRedirects int
	timeout      time.Duration
	maxBody      int64
}

var (
	outboundMu       sync.RWMutex
	outboundDefaults outboundDefaultsSnapshot
)

// setOutboundDefaultsFromInit is called by the runner once after Init to copy
// the host-pushed OutboundDefaults into the SDK-global snapshot. Empty fields
// are left as zero so NewSafeHTTPClient can apply per-call defaults.
func setOutboundDefaultsFromInit(defaults *pb.OutboundDefaults) {
	outboundMu.Lock()
	defer outboundMu.Unlock()
	if defaults == nil {
		outboundDefaults = outboundDefaultsSnapshot{}
		return
	}
	outboundDefaults = outboundDefaultsSnapshot{
		blockedCIDRs: append([]string(nil), defaults.GetBlockedCidrs()...),
		allowedHosts: append([]string(nil), defaults.GetAllowedHosts()...),
		maxRedirects: int(defaults.GetMaxRedirects()),
		timeout:      time.Duration(defaults.GetTimeoutNanos()),
		maxBody:      defaults.GetMaxBodyBytes(),
	}
}

// getOutboundDefaultsSnapshot is exported for tests.
func getOutboundDefaultsSnapshot() outboundDefaultsSnapshot {
	outboundMu.RLock()
	defer outboundMu.RUnlock()
	return outboundDefaults
}

// OutboundConfig is the per-call configuration for NewSafeHTTPClient. All
// fields are optional; zero values trigger SDK defaults (or fall back to the
// host-pushed OutboundDefaults).
type OutboundConfig struct {
	// ExtraBlockedCIDRs are appended to the host-pushed block list. Use this
	// for plugin-specific paranoia (e.g. blocking the host's own VPC range).
	ExtraBlockedCIDRs []string
	// AllowedHosts is the plugin's host allow-list. Merged with the host's
	// global allow-list (if any). Empty here means "no plugin-level
	// allow-list"; if the host pushed one, it still applies.
	AllowedHosts []string
	// MaxRedirects caps redirects (default: 3, or host value if set).
	MaxRedirects int
	// MaxBodyBytes caps response body size for LimitedReadAll (default 1 MiB).
	MaxBodyBytes int64
	// Timeout is the per-request timeout (default 30s, or host value).
	Timeout time.Duration
}

// NewSafeHTTPClient returns an *http.Client whose Transport refuses to dial
// any IP address in the blocked-CIDR list. The check runs inside DialContext
// so DNS rebinding (TTL=0 returning a public IP first, an internal IP on the
// second resolve) is caught at every dial.
// safeHTTPConfig is the resolved configuration NewSafeHTTPClient passes to
// the dialer / transport / client builders below. Centralising the
// host-defaults + plugin-overrides resolution into resolveSafeHTTPConfig
// (T52) keeps NewSafeHTTPClient itself an orchestrator at <30 lines.
type safeHTTPConfig struct {
	blockedNets  []*net.IPNet
	allowSet     map[string]struct{}
	maxRedirects int
	timeout      time.Duration
	maxBody      int64
}

// resolveSafeHTTPConfig merges host-pushed outbound defaults with the
// per-call OutboundConfig overrides, producing the concrete values the
// dialer / transport / client builders need. Splitting this out (T52)
// keeps NewSafeHTTPClient at orchestrator size and lets future changes to
// the precedence rules live in one place.
func resolveSafeHTTPConfig(cfg OutboundConfig) (safeHTTPConfig, error) {
	defaults := getOutboundDefaultsSnapshot()

	// Merge block list: defaults from host + plugin extras + SDK fallback.
	blocked := defaults.blockedCIDRs
	if len(blocked) == 0 {
		blocked = defaultBlockedCIDRs
	}
	blocked = append(append([]string{}, blocked...), cfg.ExtraBlockedCIDRs...)
	blockedNets, err := parseCIDRs(blocked)
	if err != nil {
		return safeHTTPConfig{}, fmt.Errorf("pluginsdk: parse blocked cidrs: %w", err)
	}

	// Merge allow-list: host's global + plugin's local. Empty union → no
	// host-level allow-list (block-list still enforced).
	hostAllow := append(append([]string{}, defaults.allowedHosts...), cfg.AllowedHosts...)
	allowSet := make(map[string]struct{}, len(hostAllow))
	for _, h := range hostAllow {
		allowSet[strings.ToLower(strings.TrimSpace(h))] = struct{}{}
	}

	maxRedirects := cfg.MaxRedirects
	if maxRedirects == 0 {
		maxRedirects = defaults.maxRedirects
	}
	if maxRedirects == 0 {
		maxRedirects = 3
	}

	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = defaults.timeout
	}
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	maxBody := cfg.MaxBodyBytes
	if maxBody == 0 {
		maxBody = defaults.maxBody
	}
	if maxBody == 0 {
		maxBody = 1 << 20 // 1 MiB
	}

	return safeHTTPConfig{
		blockedNets:  blockedNets,
		allowSet:     allowSet,
		maxRedirects: maxRedirects,
		timeout:      timeout,
		maxBody:      maxBody,
	}, nil
}

// buildSSRFTransport wires the SSRF-aware DialContext closure onto an
// http.Transport. The DialContext re-resolves and re-checks the destination
// IP on every dial — even if the application validated the hostname
// earlier, the actual dialed IP is checked here, defeating TOCTOU / DNS
// rebinding attacks.
func buildSSRFTransport(cfg safeHTTPConfig) *http.Transport {
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	return &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, splitErr := net.SplitHostPort(addr)
			if splitErr != nil {
				return nil, splitErr
			}
			if len(cfg.allowSet) > 0 {
				if _, ok := cfg.allowSet[strings.ToLower(host)]; !ok {
					return nil, fmt.Errorf("%w: %s", ErrHostNotAllowed, host)
				}
			}
			ips, lookupErr := net.DefaultResolver.LookupIPAddr(ctx, host)
			if lookupErr != nil {
				return nil, lookupErr
			}
			if len(ips) == 0 {
				return nil, fmt.Errorf("pluginsdk: no IPs for host %q", host)
			}
			// All resolved IPs must clear the block list — partial match is
			// not enough, since net.Dialer may pick any of them.
			for _, ip := range ips {
				if cidrContains(cfg.blockedNets, ip.IP) {
					return nil, fmt.Errorf("%w: host=%s ip=%s", ErrBlockedTarget, host, ip.IP)
				}
			}
			// Re-attach port and dial the first resolved IP. Using the IP
			// (not the hostname) prevents the resolver from being consulted
			// again under us.
			return dialer.DialContext(ctx, network, net.JoinHostPort(ips[0].IP.String(), port))
		},
	}
}

// buildSafeHTTPClient assembles the http.Client with the SSRF transport,
// timeout, redirect cap, and body-cap wrapper. Wrapping the transport via
// limitedBodyTransport lets LimitedReadAll honour the per-call max without
// the caller passing it explicitly.
func buildSafeHTTPClient(transport *http.Transport, cfg safeHTTPConfig) *http.Client {
	client := &http.Client{
		Transport: transport,
		Timeout:   cfg.timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= cfg.maxRedirects {
				return ErrTooManyRedirects
			}
			// Re-validate the redirect target host against the allow-list.
			if len(cfg.allowSet) > 0 {
				h := strings.ToLower(req.URL.Hostname())
				if _, ok := cfg.allowSet[h]; !ok {
					return fmt.Errorf("%w: %s", ErrHostNotAllowed, h)
				}
			}
			return nil
		},
	}
	// Stash the body cap on the client via a wrapper transport so callers
	// can later use LimitedReadAll without passing the cap explicitly.
	client.Transport = &limitedBodyTransport{base: transport, max: cfg.maxBody}
	return client
}

// NewSafeHTTPClient constructs the SSRF-hardened HTTP client plugins use
// for outbound traffic. The body delegates to resolveSafeHTTPConfig +
// buildSSRFTransport + buildSafeHTTPClient (T52) so each concern lives in
// its own helper and adding a new defence (e.g. mTLS, metrics) means
// extending one of them rather than the orchestrator.
func NewSafeHTTPClient(cfg OutboundConfig) (*http.Client, error) {
	resolved, err := resolveSafeHTTPConfig(cfg)
	if err != nil {
		return nil, err
	}
	transport := buildSSRFTransport(resolved)
	return buildSafeHTTPClient(transport, resolved), nil
}

// LimitedReadAll reads up to max bytes from r and returns ErrBodyTooLarge if
// the body would exceed max. Callers should use this instead of io.ReadAll on
// untrusted upstream bodies.
func LimitedReadAll(r io.Reader, max int64) ([]byte, error) {
	if max <= 0 {
		max = 1 << 20
	}
	buf, err := io.ReadAll(io.LimitReader(r, max+1))
	if err != nil {
		return buf, err
	}
	if int64(len(buf)) > max {
		return buf[:max], ErrBodyTooLarge
	}
	return buf, nil
}

// limitedBodyTransport wraps base and caps response.Body using LimitedReader.
// It does not enforce ErrBodyTooLarge inline (that would require buffering);
// instead, callers reading the body get a truncated stream.
type limitedBodyTransport struct {
	base http.RoundTripper
	max  int64
}

func (t *limitedBodyTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.base.RoundTrip(req)
	if err != nil || resp == nil || resp.Body == nil || t.max <= 0 {
		return resp, err
	}
	resp.Body = &readCloser{Reader: io.LimitReader(resp.Body, t.max), Closer: resp.Body}
	return resp, nil
}

type readCloser struct {
	io.Reader
	io.Closer
}

// parseCIDRs converts string CIDRs to *net.IPNet. Single-IP entries (no "/")
// are auto-extended to /32 (IPv4) or /128 (IPv6) for caller convenience.
func parseCIDRs(in []string) ([]*net.IPNet, error) {
	out := make([]*net.IPNet, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if !strings.Contains(s, "/") {
			ip := net.ParseIP(s)
			if ip == nil {
				return nil, fmt.Errorf("invalid IP/CIDR %q", s)
			}
			if ip.To4() != nil {
				s += "/32"
			} else {
				s += "/128"
			}
		}
		_, n, err := net.ParseCIDR(s)
		if err != nil {
			return nil, fmt.Errorf("invalid CIDR %q: %w", s, err)
		}
		out = append(out, n)
	}
	return out, nil
}

func cidrContains(nets []*net.IPNet, ip net.IP) bool {
	for _, n := range nets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}
