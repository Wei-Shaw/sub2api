// Package tlsfingerprint provides TLS fingerprint simulation for HTTP clients.
// It uses the utls library to create TLS connections that mimic Node.js/Claude Code clients.
package tlsfingerprint

import (
	"bufio"
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"

	utls "github.com/refraction-networking/utls"
	"golang.org/x/net/proxy"
)

// Profile contains TLS fingerprint configuration.
// All slice fields use built-in defaults when empty.
type Profile struct {
	Name                string // Profile name for identification
	CipherSuites        []uint16
	Curves              []uint16
	PointFormats        []uint16
	EnableGREASE        bool
	SignatureAlgorithms []uint16 // Empty uses defaultSignatureAlgorithms
	ALPNProtocols       []string // Empty uses ["http/1.1"]
	SupportedVersions   []uint16 // Empty uses [TLS1.3, TLS1.2]
	KeyShareGroups      []uint16 // Empty uses [X25519]
	PSKModes            []uint16 // Empty uses [psk_dhe_ke]
	Extensions          []uint16 // Extension type IDs in order; empty uses default Node.js 24.x order
}

// Dialer creates TLS connections with custom fingerprints.
type Dialer struct {
	profile    *Profile
	baseDialer func(ctx context.Context, network, addr string) (net.Conn, error)
}

// HTTPProxyDialer creates TLS connections through HTTP/HTTPS proxies with custom fingerprints.
// It handles the CONNECT tunnel establishment before performing TLS handshake.
type HTTPProxyDialer struct {
	profile  *Profile
	proxyURL *url.URL
}

// SOCKS5ProxyDialer creates TLS connections through SOCKS5 proxies with custom fingerprints.
// It uses golang.org/x/net/proxy to establish the SOCKS5 tunnel.
type SOCKS5ProxyDialer struct {
	profile  *Profile
	proxyURL *url.URL
}

// Default TLS fingerprint values captured from Claude Code 2.1.109 on
// Node.js 24.14.1 / macOS arm64, pointed at a local capture server via
// `ANTHROPIC_BASE_URL`. Capture tool source at backend/tools/capture_fingerprint.
//
// Capture date: 2026-04-15
// JA3 string:
//
//	771,4866-4867-4865-49199-49195-49200-49196-158-49191-103-49192-107-163-159-
//	52393-52392-52394-49325-49311-49245-49249-49239-49235-162-49324-49310-49244-
//	49248-49238-49234-49188-106-49187-64-49162-49172-57-56-49161-49171-51-50-
//	157-49309-49233-156-49308-49232-61-60-53-47,
//	65281-0-11-10-35-16-22-23-13-43-45-51,
//	29-23-30-24-25-256-257,0-1-2
//
// JA3 hash: d67b094811e5145139d7cea5f014309f
// JA4:      t13d5212h1 (part-a prefix — 52 ciphers, 12 extensions, http/1.1 ALPN)
//
// Critical findings from the capture that influenced these defaults:
//   - Real Claude Code 2.1.109 advertises ONLY `http/1.1` in ALPN — it does
//     NOT offer `h2`. Earlier concerns about Go-vs-Node HTTP/2 SETTINGS
//     mismatch were moot because the real CLI does not use HTTP/2 on the
//     api.anthropic.com path at all.
//   - 52 cipher suites (not 17 — the previous hand-authored list was a
//     substantial undercount).
//   - 26 signature schemes, 8 supported groups (incl. 2 FFDHE + X448 + P521).
//   - Extension order is: renegotiation_info, server_name, ec_point_formats,
//     supported_groups, session_ticket, alpn, encrypt_then_mac(22),
//     extended_master_secret, signature_algorithms, supported_versions,
//     psk_key_exchange_modes, key_share. No ECH, no SCT, no status_request.
var (
	// defaultCipherSuites — 52 cipher suites in the order Node.js 24.14.1
	// OpenSSL sends them. Do NOT reorder: JA3 hash depends on order.
	defaultCipherSuites = []uint16{
		// TLS 1.3 (note: 1302 comes before 1303/1301)
		0x1302, // TLS_AES_256_GCM_SHA384
		0x1303, // TLS_CHACHA20_POLY1305_SHA256
		0x1301, // TLS_AES_128_GCM_SHA256

		// ECDHE + AES-GCM (RSA before ECDSA)
		0xc02f, // TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256
		0xc02b, // TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256
		0xc030, // TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384
		0xc02c, // TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384

		// DHE_RSA + AES-GCM
		0x009e, // TLS_DHE_RSA_WITH_AES_128_GCM_SHA256

		// ECDHE + AES-CBC-SHA256 (SHA256 HMAC variants)
		0xc027, // TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA256
		0x0067, // TLS_DHE_RSA_WITH_AES_128_CBC_SHA256
		0xc028, // TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA384
		0x006b, // TLS_DHE_RSA_WITH_AES_256_CBC_SHA256
		0x00a3, // TLS_DHE_DSS_WITH_AES_256_GCM_SHA384
		0x009f, // TLS_DHE_RSA_WITH_AES_256_GCM_SHA384

		// ChaCha20-Poly1305 family (3 variants: ECDSA, RSA, DHE)
		0xcca9, // TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256
		0xcca8, // TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256
		0xccaa, // TLS_DHE_RSA_WITH_CHACHA20_POLY1305_SHA256

		// ECCPWD, camellia, ARIA (legacy PSK/ARIA from OpenSSL enable-all)
		0xc0ad, // TLS_ECDHE_ECDSA_WITH_AES_256_CCM
		0xc09f, // TLS_DHE_RSA_WITH_AES_256_CCM
		0xc05d, // TLS_ECDHE_ECDSA_WITH_ARIA_256_GCM_SHA384
		0xc061, // TLS_ECDHE_RSA_WITH_ARIA_256_GCM_SHA384
		0xc057, // TLS_DHE_DSS_WITH_ARIA_256_GCM_SHA384
		0xc053, // TLS_DHE_RSA_WITH_ARIA_256_GCM_SHA384
		0x00a2, // TLS_DHE_DSS_WITH_AES_128_GCM_SHA256
		0xc0ac, // TLS_ECDHE_ECDSA_WITH_AES_128_CCM
		0xc09e, // TLS_DHE_RSA_WITH_AES_128_CCM
		0xc05c, // TLS_ECDHE_ECDSA_WITH_ARIA_128_GCM_SHA256
		0xc060, // TLS_ECDHE_RSA_WITH_ARIA_128_GCM_SHA256
		0xc056, // TLS_DHE_DSS_WITH_ARIA_128_GCM_SHA256
		0xc052, // TLS_DHE_RSA_WITH_ARIA_128_GCM_SHA256

		// ECDHE/DHE + CBC-SHA256/384 (legacy)
		0xc024, // TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA384
		0x006a, // TLS_DHE_DSS_WITH_AES_256_CBC_SHA256
		0xc023, // TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA256
		0x0040, // TLS_DHE_DSS_WITH_AES_128_CBC_SHA256

		// ECDHE + AES-CBC-SHA (legacy — SHA1 HMAC)
		0xc00a, // TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA
		0xc014, // TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA
		0x0039, // TLS_DHE_RSA_WITH_AES_256_CBC_SHA
		0x0038, // TLS_DHE_DSS_WITH_AES_256_CBC_SHA
		0xc009, // TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA
		0xc013, // TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA
		0x0033, // TLS_DHE_RSA_WITH_AES_128_CBC_SHA
		0x0032, // TLS_DHE_DSS_WITH_AES_128_CBC_SHA

		// RSA key exchange + AES-GCM (non-PFS)
		0x009d, // TLS_RSA_WITH_AES_256_GCM_SHA384
		0xc09d, // TLS_RSA_WITH_AES_256_CCM
		0xc051, // TLS_RSA_WITH_ARIA_256_GCM_SHA384
		0x009c, // TLS_RSA_WITH_AES_128_GCM_SHA256
		0xc09c, // TLS_RSA_WITH_AES_128_CCM
		0xc050, // TLS_RSA_WITH_ARIA_128_GCM_SHA256

		// RSA + AES-CBC-SHA256 (legacy)
		0x003d, // TLS_RSA_WITH_AES_256_CBC_SHA256
		0x003c, // TLS_RSA_WITH_AES_128_CBC_SHA256

		// RSA + AES-CBC-SHA (very legacy)
		0x0035, // TLS_RSA_WITH_AES_256_CBC_SHA
		0x002f, // TLS_RSA_WITH_AES_128_CBC_SHA
	}

	// defaultCurves — supported groups we advertise.
	//
	// Real Claude Code / Node.js 24 advertises 8 groups (incl. x448,
	// ffdhe2048, ffdhe3072) but utls's HelloRetryRequest path can only
	// regenerate key shares for curves in curveForCurveID (X25519, P256,
	// P384, P521). If we advertise a group utls can't handle and the
	// server picks it via HRR, handshake fails with
	// "tls: CurvePreferences includes unsupported curve".
	//
	// X25519MLKEM768 is kept because defaultKeyShareGroups always sends its
	// key share on the initial ClientHello, so servers never need to HRR
	// back to it. The randomizer must preserve this invariant.
	defaultCurves = []utls.CurveID{
		utls.X25519MLKEM768, // 0x11ec — post-quantum hybrid
		utls.X25519,         // 0x001d
		utls.CurveP256,      // 0x0017 (secp256r1)
		utls.CurveP384,      // 0x0018 (secp384r1)
		utls.CurveP521,      // 0x0019 (secp521r1)
	}

	// defaultKeyShareGroups — X25519MLKEM768 + X25519, matching the real
	// Claude Code capture (the real CLI sends two key shares: a ~1216-byte
	// MLKEM payload and a 32-byte X25519 public key).
	defaultKeyShareGroups = []utls.CurveID{
		utls.X25519MLKEM768,
		utls.X25519,
	}

	// defaultPointFormats — 3 formats (uncompressed, ansiX962, compressed).
	defaultPointFormats = []uint16{
		0, // uncompressed
		1, // ansiX962_compressed_prime
		2, // ansiX962_compressed_char2
	}

	// defaultSignatureAlgorithms — 26 schemes Node.js 24.14.1 advertises,
	// in capture order. Includes Brainpool TLS 1.3 curves and several
	// legacy SHA1 entries.
	defaultSignatureAlgorithms = []utls.SignatureScheme{
		0x0905, // experimental TLS 1.3 (Node.js OpenSSL)
		0x0906, // experimental TLS 1.3
		0x0904, // experimental TLS 1.3
		0x0403, // ecdsa_secp256r1_sha256
		0x0503, // ecdsa_secp384r1_sha384
		0x0603, // ecdsa_secp521r1_sha512
		0x0807, // ed25519
		0x0808, // ed448
		0x081a, // ecdsa_brainpoolP256r1tls13_sha256
		0x081b, // ecdsa_brainpoolP384r1tls13_sha384
		0x081c, // ecdsa_brainpoolP512r1tls13_sha512
		0x0809, // rsa_pss_pss_sha256
		0x080a, // rsa_pss_pss_sha384
		0x080b, // rsa_pss_pss_sha512
		0x0804, // rsa_pss_rsae_sha256
		0x0805, // rsa_pss_rsae_sha384
		0x0806, // rsa_pss_rsae_sha512
		0x0401, // rsa_pkcs1_sha256
		0x0501, // rsa_pkcs1_sha384
		0x0601, // rsa_pkcs1_sha512
		0x0303, // SHA224-ECDSA (legacy)
		0x0301, // SHA224-RSA (legacy)
		0x0302, // SHA224-DSA (legacy)
		0x0402, // SHA256-DSA (legacy)
		0x0502, // SHA384-DSA (legacy)
		0x0602, // SHA512-DSA (legacy)
	}
)

// NewDialer creates a new TLS fingerprint dialer.
// baseDialer is used for TCP connection establishment (supports proxy scenarios).
// If baseDialer is nil, direct TCP dial is used.
func NewDialer(profile *Profile, baseDialer func(ctx context.Context, network, addr string) (net.Conn, error)) *Dialer {
	if baseDialer == nil {
		baseDialer = (&net.Dialer{}).DialContext
	}
	return &Dialer{profile: profile, baseDialer: baseDialer}
}

// NewHTTPProxyDialer creates a new TLS fingerprint dialer that works through HTTP/HTTPS proxies.
// It establishes a CONNECT tunnel before performing TLS handshake with custom fingerprint.
func NewHTTPProxyDialer(profile *Profile, proxyURL *url.URL) *HTTPProxyDialer {
	return &HTTPProxyDialer{profile: profile, proxyURL: proxyURL}
}

// NewSOCKS5ProxyDialer creates a new TLS fingerprint dialer that works through SOCKS5 proxies.
// It establishes a SOCKS5 tunnel before performing TLS handshake with custom fingerprint.
func NewSOCKS5ProxyDialer(profile *Profile, proxyURL *url.URL) *SOCKS5ProxyDialer {
	return &SOCKS5ProxyDialer{profile: profile, proxyURL: proxyURL}
}

// DialTLSContext establishes a TLS connection through SOCKS5 proxy with the configured fingerprint.
// Flow: SOCKS5 CONNECT to target -> TLS handshake with utls on the tunnel
func (d *SOCKS5ProxyDialer) DialTLSContext(ctx context.Context, network, addr string) (net.Conn, error) {
	slog.Debug("tls_fingerprint_socks5_connecting", "proxy", d.proxyURL.Host, "target", addr)

	// Step 1: Create SOCKS5 dialer
	var auth *proxy.Auth
	if d.proxyURL.User != nil {
		username := d.proxyURL.User.Username()
		password, _ := d.proxyURL.User.Password()
		auth = &proxy.Auth{
			User:     username,
			Password: password,
		}
	}

	// Determine proxy address
	proxyAddr := d.proxyURL.Host
	if d.proxyURL.Port() == "" {
		proxyAddr = net.JoinHostPort(d.proxyURL.Hostname(), "1080") // Default SOCKS5 port
	}

	socksDialer, err := proxy.SOCKS5("tcp", proxyAddr, auth, proxy.Direct)
	if err != nil {
		slog.Debug("tls_fingerprint_socks5_dialer_failed", "error", err)
		return nil, fmt.Errorf("create SOCKS5 dialer: %w", err)
	}

	// Step 2: Establish SOCKS5 tunnel to target.
	// Use ContextDialer so the caller's ctx (timeout/cancel) covers the SOCKS5
	// handshake; otherwise an unresponsive proxy hangs goroutines for minutes
	// and exhausts connections. x/net's *socks.Dialer satisfies ContextDialer
	// (verified for golang.org/x/net v0.49.0).
	slog.Debug("tls_fingerprint_socks5_establishing_tunnel", "target", addr)
	ctxDialer, ok := socksDialer.(proxy.ContextDialer)
	if !ok {
		return nil, fmt.Errorf("SOCKS5 dialer does not implement ContextDialer (x/net contract changed)")
	}
	conn, err := ctxDialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		slog.Debug("tls_fingerprint_socks5_connect_failed", "error", err)
		return nil, fmt.Errorf("SOCKS5 connect: %w", err)
	}
	slog.Debug("tls_fingerprint_socks5_tunnel_established")

	// Step 3: Perform TLS handshake on the tunnel with utls fingerprint
	return performTLSHandshake(ctx, conn, d.profile, addr)
}

// DialTLSContext establishes a TLS connection through HTTP proxy with the configured fingerprint.
// Flow: TCP connect to proxy -> CONNECT tunnel -> TLS handshake with utls
func (d *HTTPProxyDialer) DialTLSContext(ctx context.Context, network, addr string) (net.Conn, error) {
	slog.Debug("tls_fingerprint_http_proxy_connecting", "proxy", d.proxyURL.Host, "target", addr)

	// Step 1: TCP connect to proxy server
	var proxyAddr string
	if d.proxyURL.Port() != "" {
		proxyAddr = d.proxyURL.Host
	} else {
		// Default ports
		if d.proxyURL.Scheme == "https" {
			proxyAddr = net.JoinHostPort(d.proxyURL.Hostname(), "443")
		} else {
			proxyAddr = net.JoinHostPort(d.proxyURL.Hostname(), "80")
		}
	}

	dialer := &net.Dialer{}
	conn, err := dialer.DialContext(ctx, "tcp", proxyAddr)
	if err != nil {
		slog.Debug("tls_fingerprint_http_proxy_connect_failed", "error", err)
		return nil, fmt.Errorf("connect to proxy: %w", err)
	}
	slog.Debug("tls_fingerprint_http_proxy_connected", "proxy_addr", proxyAddr)

	// Step 2: Send CONNECT request to establish tunnel
	req := &http.Request{
		Method: "CONNECT",
		URL:    &url.URL{Opaque: addr},
		Host:   addr,
		Header: make(http.Header),
	}

	// Add proxy authentication if present
	if d.proxyURL.User != nil {
		username := d.proxyURL.User.Username()
		password, _ := d.proxyURL.User.Password()
		auth := base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
		req.Header.Set("Proxy-Authorization", "Basic "+auth)
	}

	slog.Debug("tls_fingerprint_http_proxy_sending_connect", "target", addr)
	if err := req.Write(conn); err != nil {
		_ = conn.Close()
		slog.Debug("tls_fingerprint_http_proxy_write_failed", "error", err)
		return nil, fmt.Errorf("write CONNECT request: %w", err)
	}

	// Step 3: Read CONNECT response
	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, req)
	if err != nil {
		_ = conn.Close()
		slog.Debug("tls_fingerprint_http_proxy_read_response_failed", "error", err)
		return nil, fmt.Errorf("read CONNECT response: %w", err)
	}
	// CONNECT response has no body; do not defer resp.Body.Close() as it wraps the
	// same conn that will be used for the TLS handshake.

	if resp.StatusCode != http.StatusOK {
		_ = conn.Close()
		slog.Debug("tls_fingerprint_http_proxy_connect_failed_status", "status_code", resp.StatusCode, "status", resp.Status)
		return nil, fmt.Errorf("proxy CONNECT failed: %s", resp.Status)
	}
	slog.Debug("tls_fingerprint_http_proxy_tunnel_established")

	// Step 4: Perform TLS handshake on the tunnel with utls fingerprint
	return performTLSHandshake(ctx, conn, d.profile, addr)
}

// DialTLSContext establishes a TLS connection with the configured fingerprint.
// This method is designed to be used as http.Transport.DialTLSContext.
func (d *Dialer) DialTLSContext(ctx context.Context, network, addr string) (net.Conn, error) {
	// Establish TCP connection using base dialer (supports proxy)
	slog.Debug("tls_fingerprint_dialing_tcp", "addr", addr)
	conn, err := d.baseDialer(ctx, network, addr)
	if err != nil {
		slog.Debug("tls_fingerprint_tcp_dial_failed", "error", err)
		return nil, err
	}
	slog.Debug("tls_fingerprint_tcp_connected", "addr", addr)

	// Perform TLS handshake with utls fingerprint
	return performTLSHandshake(ctx, conn, d.profile, addr)
}

// performTLSHandshake performs the uTLS handshake on an established connection.
// It builds a ClientHello spec from the profile, applies it, and completes the handshake.
// On failure, conn is closed and an error is returned.
func performTLSHandshake(ctx context.Context, conn net.Conn, profile *Profile, addr string) (net.Conn, error) {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}

	spec := buildClientHelloSpecFromProfile(profile)
	tlsConn := utls.UClient(conn, &utls.Config{ServerName: host}, utls.HelloCustom)

	if err := tlsConn.ApplyPreset(spec); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("apply TLS preset: %w", err)
	}

	if err := tlsConn.HandshakeContext(ctx); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("TLS handshake failed: %w", err)
	}

	state := tlsConn.ConnectionState()
	slog.Debug("tls_fingerprint_handshake_success",
		"host", host,
		"version", state.Version,
		"cipher_suite", state.CipherSuite,
		"alpn", state.NegotiatedProtocol)

	return tlsConn, nil
}

// toUTLSCurves converts uint16 slice to utls.CurveID slice.
func toUTLSCurves(curves []uint16) []utls.CurveID {
	result := make([]utls.CurveID, len(curves))
	for i, c := range curves {
		result[i] = utls.CurveID(c)
	}
	return result
}

// defaultExtensionOrder — 12 extensions in the exact order Node.js 24.14.1
// sends them (captured from live Claude Code 2.1.109).
//
// Differences from the previous hand-authored list:
//   - Dropped: encrypted_client_hello (65037 / ECH) — real Node.js 24 does
//     NOT send ECH unless a server config is known.
//   - Dropped: status_request (5) and signed_certificate_timestamp (18) —
//     Node.js 24's default TLS extension set doesn't include them.
//   - Added:   encrypt_then_mac (22) — RFC 7366, emitted by OpenSSL.
//   - Reordered: renegotiation_info is now FIRST; extended_master_secret
//     moved after encrypt_then_mac; ec_point_formats precedes supported_groups.
//
// Used when Profile.Extensions is empty.
var defaultExtensionOrder = []uint16{
	65281, // renegotiation_info (0xff01)
	0,     // server_name
	11,    // ec_point_formats
	10,    // supported_groups
	35,    // session_ticket
	16,    // alpn
	22,    // encrypt_then_mac (RFC 7366) — sent by OpenSSL/Node.js
	23,    // extended_master_secret
	13,    // signature_algorithms
	43,    // supported_versions
	45,    // psk_key_exchange_modes
	51,    // key_share
}

// isGREASEValue checks if a uint16 value matches the TLS GREASE pattern (0x?a?a).
func isGREASEValue(v uint16) bool {
	return v&0x0f0f == 0x0a0a && v>>8 == v&0xff
}

// buildClientHelloSpecFromProfile constructs ClientHelloSpec from a Profile.
// This is a standalone function that can be used by both Dialer and HTTPProxyDialer.
func buildClientHelloSpecFromProfile(profile *Profile) *utls.ClientHelloSpec {
	// Resolve effective values (profile overrides or built-in defaults)
	cipherSuites := defaultCipherSuites
	if profile != nil && len(profile.CipherSuites) > 0 {
		cipherSuites = profile.CipherSuites
	}

	curves := defaultCurves
	if profile != nil && len(profile.Curves) > 0 {
		curves = toUTLSCurves(profile.Curves)
	}

	pointFormats := defaultPointFormats
	if profile != nil && len(profile.PointFormats) > 0 {
		pointFormats = profile.PointFormats
	}

	signatureAlgorithms := defaultSignatureAlgorithms
	if profile != nil && len(profile.SignatureAlgorithms) > 0 {
		signatureAlgorithms = make([]utls.SignatureScheme, len(profile.SignatureAlgorithms))
		for i, s := range profile.SignatureAlgorithms {
			signatureAlgorithms[i] = utls.SignatureScheme(s)
		}
	}

	alpnProtocols := []string{"http/1.1"}
	if profile != nil && len(profile.ALPNProtocols) > 0 {
		alpnProtocols = profile.ALPNProtocols
	}
	// Defensive: strip "h2" from any profile (including ones persisted before
	// the randomizer fix). http.Transport with a custom DialTLSContext cannot
	// speak HTTP/2 — if the server negotiates h2 via ALPN, the transport writes
	// HTTP/1.1 and the server responds with HTTP/2 SETTINGS/WINDOW_UPDATE/GOAWAY
	// frames, surfacing as "malformed HTTP response" errors.
	alpnProtocols = filterHTTP2FromALPN(alpnProtocols)

	supportedVersions := []uint16{utls.VersionTLS13, utls.VersionTLS12}
	if profile != nil && len(profile.SupportedVersions) > 0 {
		supportedVersions = profile.SupportedVersions
	}

	keyShareGroups := defaultKeyShareGroups
	if profile != nil && len(profile.KeyShareGroups) > 0 {
		keyShareGroups = toUTLSCurves(profile.KeyShareGroups)
	}

	pskModes := []uint16{uint16(utls.PskModeDHE)}
	if profile != nil && len(profile.PSKModes) > 0 {
		pskModes = profile.PSKModes
	}

	enableGREASE := profile != nil && profile.EnableGREASE

	// Build key shares
	keyShares := make([]utls.KeyShare, len(keyShareGroups))
	for i, g := range keyShareGroups {
		keyShares[i] = utls.KeyShare{Group: g}
	}

	// Determine extension order
	extOrder := defaultExtensionOrder
	if profile != nil && len(profile.Extensions) > 0 {
		extOrder = profile.Extensions
	}

	// Build extensions list from the ordered IDs.
	// Parametric extensions (curves, sigalgs, etc.) are populated with resolved profile values.
	// Unknown IDs use GenericExtension (sends type ID with empty data).
	extensions := make([]utls.TLSExtension, 0, len(extOrder)+2)
	for _, id := range extOrder {
		if isGREASEValue(id) {
			extensions = append(extensions, &utls.UtlsGREASEExtension{})
			continue
		}
		switch id {
		case 0: // server_name
			extensions = append(extensions, &utls.SNIExtension{})
		case 5: // status_request (OCSP)
			extensions = append(extensions, &utls.StatusRequestExtension{})
		case 10: // supported_groups
			extensions = append(extensions, &utls.SupportedCurvesExtension{Curves: curves})
		case 11: // ec_point_formats
			extensions = append(extensions, &utls.SupportedPointsExtension{SupportedPoints: toUint8s(pointFormats)})
		case 13: // signature_algorithms
			extensions = append(extensions, &utls.SignatureAlgorithmsExtension{SupportedSignatureAlgorithms: signatureAlgorithms})
		case 16: // alpn
			extensions = append(extensions, &utls.ALPNExtension{AlpnProtocols: alpnProtocols})
		case 18: // signed_certificate_timestamp
			extensions = append(extensions, &utls.SCTExtension{})
		case 23: // extended_master_secret
			extensions = append(extensions, &utls.ExtendedMasterSecretExtension{})
		case 35: // session_ticket
			extensions = append(extensions, &utls.SessionTicketExtension{})
		case 43: // supported_versions
			extensions = append(extensions, &utls.SupportedVersionsExtension{Versions: supportedVersions})
		case 45: // psk_key_exchange_modes
			extensions = append(extensions, &utls.PSKKeyExchangeModesExtension{Modes: toUint8s(pskModes)})
		case 50: // signature_algorithms_cert
			extensions = append(extensions, &utls.SignatureAlgorithmsCertExtension{SupportedSignatureAlgorithms: signatureAlgorithms})
		case 51: // key_share
			extensions = append(extensions, &utls.KeyShareExtension{KeyShares: keyShares})
		case 0xfe0d: // encrypted_client_hello (ECH, 65037)
			// Send GREASE ECH with random payload — mimics Node.js behavior when no real ECHConfig is available.
			// An empty GenericExtension causes "error decoding message" from servers that validate ECH format.
			extensions = append(extensions, &utls.GREASEEncryptedClientHelloExtension{})
		case 0xff01: // renegotiation_info
			extensions = append(extensions, &utls.RenegotiationInfoExtension{})
		default:
			// Unknown extension — send as GenericExtension (type ID + empty data).
			// This covers encrypt_then_mac(22) and any future extensions.
			extensions = append(extensions, &utls.GenericExtension{Id: id})
		}
	}

	// For default extension order with EnableGREASE, wrap with GREASE bookends
	if enableGREASE && (profile == nil || len(profile.Extensions) == 0) {
		extensions = append([]utls.TLSExtension{&utls.UtlsGREASEExtension{}}, extensions...)
		extensions = append(extensions, &utls.UtlsGREASEExtension{})
	}

	return &utls.ClientHelloSpec{
		CipherSuites:       cipherSuites,
		CompressionMethods: []uint8{0}, // null compression only (standard)
		Extensions:         extensions,
		TLSVersMax:         utls.VersionTLS13,
		TLSVersMin:         utls.VersionTLS10,
	}
}

// filterHTTP2FromALPN removes "h2" (and the deprecated "h2c") entries from an
// ALPN list, guaranteeing at least ["http/1.1"] is advertised. Called before
// every TLS handshake — see comment at call site for why h2 is unsafe.
func filterHTTP2FromALPN(alpn []string) []string {
	out := make([]string, 0, len(alpn))
	for _, p := range alpn {
		if p == "h2" || p == "h2c" {
			continue
		}
		out = append(out, p)
	}
	if len(out) == 0 {
		return []string{"http/1.1"}
	}
	return out
}

// toUint8s converts []uint16 to []uint8 (for utls fields that require []uint8).
func toUint8s(vals []uint16) []uint8 {
	out := make([]uint8, len(vals))
	for i, v := range vals {
		out[i] = uint8(v)
	}
	return out
}
