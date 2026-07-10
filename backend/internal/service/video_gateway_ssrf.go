package service

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/util/urlvalidator"
)

const (
	mediaURLAllowlistEnv       = "SUB2API_MEDIA_URL_ALLOWLIST"
	legacyVideoURLAllowlistEnv = "SUB2API_VIDEO_URL_ALLOWLIST"
)

// validateExternalVideoURL enforces a hard SSRF/open-proxy guard on the two
// externally-controlled URLs in the seedance path:
//
//   - reference_image_url: user-supplied, forwarded to Ark as image_url. A
//     malicious value (http://169.254.169.254/, http://localhost:port/,
//     file:///etc/passwd) would turn the gateway into an SSRF amplifier.
//   - result_url (content.video_url): returned by Ark, then played by the
//     frontend and written to Day0. A tampered/unexpected URL must not be
//     trusted blindly.
//
// Layering:
//  1. Delegate the core checks (https-only scheme, blocked localhost/private/
//     loopback/link-local hosts, optional host allowlist with *.domain
//     wildcards, port range) to the shared, tested internal/util/urlvalidator.
//  2. The seedance smoke gate (seedanceSmokeGateBlockedReasons) REQUIRES
//     SUB2API_MEDIA_URL_ALLOWLIST or legacy SUB2API_VIDEO_URL_ALLOWLIST before any real call, so the
//     production path always runs the strict allowlist branch (fail-closed) —
//     a domain allowlist inherently rejects decimal/hex/octal/CGNAT hosts since
//     they cannot match a domain suffix.
//  3. Defense-in-depth host hardening below catches the obfuscated-IP /
//     trailing-dot / CGNAT (100.64/10) / 0.0.0.0/8 tricks that net.ParseIP
//     (and therefore urlvalidator's literal-IP check) misses, applied even when
//     no allowlist is set.
//
// DNS-rebinding (post-resolution IP check, urlvalidator.ValidateResolvedIP) is
// intentionally NOT performed here: neither URL is fetched by this process
// (Ark fetches reference_image_url; the frontend plays result_url), and live
// DNS inside a validator is a TOCTOU / non-determinism trap. It is tracked in
// the smoke checklist instead.
func validateExternalVideoURL(rawURL string) error {
	raw := strings.TrimSpace(rawURL)
	if raw == "" {
		return fmt.Errorf("url is empty")
	}
	// Reject characters that cause parser-differential SSRF: a backslash is a
	// path separator under WHATWG/browser parsing but not under Go's url.Parse,
	// so "https://10.0.0.1\@allowed.host/" can validate as allowed.host here yet
	// be fetched as 10.0.0.1 downstream. Also reject whitespace/control bytes.
	if strings.ContainsAny(raw, "\\ \t\r\n\f\v") {
		return fmt.Errorf("url contains illegal characters")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("url is not parseable: %w", err)
	}
	// Reject embedded credentials (userinfo): "https://evil@allowed.host/" is a
	// classic confusion vector and different parsers disagree on the real host.
	if u.User != nil {
		return fmt.Errorf("url must not contain userinfo")
	}

	// Normalize a single trailing dot on the host BEFORE delegating, so the
	// shared validator's blocked-host AND allowlist checks see the canonical
	// host: "localhost." -> "localhost" (still blocked), and an allowlisted
	// "volces.com." -> "volces.com" (still matches, no false reject). A trailing
	// dot only occurs on DNS names, never IPv6 literals, so bracket handling is
	// moot; rebuild u.Host preserving the port.
	origHost := u.Hostname()
	host := strings.TrimSuffix(origHost, ".")
	if host == "" {
		return fmt.Errorf("url host is empty")
	}
	if host != origHost {
		if port := u.Port(); port != "" {
			u.Host = host + ":" + port
		} else {
			u.Host = host
		}
	}

	opts := urlvalidator.ValidationOptions{}
	if allow := configuredMediaURLAllowlist(); allow != "" {
		opts.AllowedHosts = videoURLAllowlistHosts(allow)
		opts.RequireAllowlist = true
	}
	if _, err := urlvalidator.ValidateHTTPSURL(u.String(), opts); err != nil {
		return err
	}

	// Defense-in-depth host hardening for tricks the shared validator's
	// net.ParseIP-based check misses, applied even in the no-allowlist branch.
	lower := strings.ToLower(host)
	if lower == "localhost" ||
		strings.HasSuffix(lower, ".localhost") ||
		strings.HasSuffix(lower, ".local") ||
		strings.HasSuffix(lower, ".internal") {
		return fmt.Errorf("url host %q is not allowed", lower)
	}
	if isObfuscatedNumericHost(lower) {
		return fmt.Errorf("url host %q is an obfuscated numeric address", lower)
	}
	// Re-check the host as an IP, covering both the ranges net.IP exposes and
	// the ones it omits (CGNAT, 0.0.0.0/8).
	if ip := net.ParseIP(lower); ip != nil && isDisallowedVideoIP(ip) {
		return fmt.Errorf("url host %q is a disallowed (internal/reserved) address", lower)
	}
	return nil
}

// isDisallowedVideoIP rejects loopback/private/link-local/unspecified/multicast
// (via net.IP helpers) plus CGNAT 100.64.0.0/10 and 0.0.0.0/8, which
// net.IP.IsPrivate does NOT cover but are commonly used for internal services.
func isDisallowedVideoIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
		return true
	}
	if v4 := ip.To4(); v4 != nil {
		if v4[0] == 100 && v4[1] >= 64 && v4[1] <= 127 { // 100.64.0.0/10 (CGNAT)
			return true
		}
		if v4[0] == 0 { // 0.0.0.0/8 ("this network")
			return true
		}
	}
	return false
}

// isObfuscatedNumericHost detects alternate IP encodings that resolve to an
// address but are not recognised by net.ParseIP (so they would otherwise slip
// through as a "hostname"): decimal integer (2130706433), hex (0x7f000001),
// and octal / leading-zero dotted labels (0177.0.0.1).
func isObfuscatedNumericHost(host string) bool {
	if host == "" {
		return false
	}
	if isAllDigits(host) { // pure decimal integer form
		return true
	}
	if strings.HasPrefix(host, "0x") || strings.HasPrefix(host, "0X") { // hex form
		return true
	}
	// dotted, all-numeric, but not a valid IPv4 literal => octal/overflow form
	// (e.g. 0177.0.0.1, 010.0.0.1). Scoped to all-numeric hosts so we do NOT
	// over-flag RFC-1123-legal names whose labels merely start with 0
	// (e.g. 007.cdn.example.com).
	if onlyDigitsAndDots(host) && strings.Contains(host, ".") && net.ParseIP(host) == nil {
		return true
	}
	// hex octet label anywhere (e.g. 0x7f.0.0.1)
	for _, label := range strings.Split(host, ".") {
		if strings.HasPrefix(label, "0x") || strings.HasPrefix(label, "0X") {
			return true
		}
	}
	return false
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func onlyDigitsAndDots(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if (r < '0' || r > '9') && r != '.' {
			return false
		}
	}
	return true
}

func configuredMediaURLAllowlist() string {
	if allow := strings.TrimSpace(os.Getenv(mediaURLAllowlistEnv)); allow != "" {
		return allow
	}
	return strings.TrimSpace(os.Getenv(legacyVideoURLAllowlistEnv))
}

func mediaURLAllowlistMissingReason() string {
	return "media url allowlist (SUB2API_MEDIA_URL_ALLOWLIST or SUB2API_VIDEO_URL_ALLOWLIST) is missing"
}

// videoURLAllowlistHosts converts a comma-separated suffix list (e.g.
// "volces.com, volccdn.com" from SUB2API_MEDIA_URL_ALLOWLIST) into urlvalidator
// host entries that match both the apex and any subdomain — "volces.com" plus
// "*.volces.com".
func videoURLAllowlistHosts(allow string) []string {
	out := []string{}
	for _, item := range strings.Split(allow, ",") {
		entry := strings.ToLower(strings.TrimSpace(item))
		entry = strings.TrimPrefix(entry, ".")
		if entry == "" {
			continue
		}
		out = append(out, entry, "*."+entry)
	}
	return out
}
