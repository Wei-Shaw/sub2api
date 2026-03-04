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
type Profile struct {
	Name         string // Profile name for identification
	CipherSuites []uint16
	Curves       []uint16
	PointFormats []uint8
	EnableGREASE bool
REDACTED

// Dialer creates TLS connections with custom fingerprints.
type Dialer struct {
	profile    *Profile
	baseDialer func(ctx context.Context, network, addr string) (net.Conn, error)
REDACTED

// HTTPProxyDialer creates TLS connections through HTTP/HTTPS proxies with custom fingerprints.
// It handles the CONNECT tunnel establishment before performing TLS handshake.
type HTTPProxyDialer struct {
	profile  *Profile
	proxyURL *url.URL
REDACTED

// SOCKS5ProxyDialer creates TLS connections through SOCKS5 proxies with custom fingerprints.
// It uses golang.org/x/net/proxy to establish the SOCKS5 tunnel.
type SOCKS5ProxyDialer struct {
	profile  *Profile
	proxyURL *url.URL
REDACTED

// Default TLS fingerprint values captured from Claude CLI 2.x (Node.js 20.x + OpenSSL 3.x)
// Captured using: tshark -i lo -f "tcp port 8443" -Y "tls.handshake.type == 1" -V
// JA3 Hash: 1a28e69016765d92e3b381168d68922c
//
// Note: JA3/JA4 may have slight variations due to:
// - Session ticket presence/absence
// - Extension negotiation state
var (
	// defaultCipherSuites contains all 59 cipher suites from Claude CLI
	// Order is critical for JA3 fingerprint matching
	defaultCipherSuites = []uint16{
		// TLS 1.3 cipher suites (MUST be first)
		0x1302, // TLS_AES_256_GCM_SHA384
		0x1303, // TLS_CHACHA20_POLY1305_SHA256
		0x1301, // TLS_AES_128_GCM_SHA256

		// ECDHE + AES-GCM
		0xc02f, // TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256
		0xc02b, // TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256
		0xc030, // TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384
		0xc02c, // TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384

		// DHE + AES-GCM
		0x009e, // TLS_DHE_RSA_WITH_AES_128_GCM_SHA256

		// ECDHE/DHE + AES-CBC-SHA256/384
		0xc027, // TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA256
		0x0067, // TLS_DHE_RSA_WITH_AES_128_CBC_SHA256
		0xc028, // TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA384
		0x006b, // TLS_DHE_RSA_WITH_AES_256_CBC_SHA256

		// DHE-DSS/RSA + AES-GCM
		0x00a3, // TLS_DHE_DSS_WITH_AES_256_GCM_SHA384
		0x009f, // TLS_DHE_RSA_WITH_AES_256_GCM_SHA384

		// ChaCha20-Poly1305
		0xcca9, // TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256
		0xcca8, // TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256
		0xccaa, // TLS_DHE_RSA_WITH_CHACHA20_POLY1305_SHA256

		// AES-CCM (256-bit)
		0xc0af, // TLS_ECDHE_ECDSA_WITH_AES_256_CCM_8
		0xc0ad, // TLS_ECDHE_ECDSA_WITH_AES_256_CCM
		0xc0a3, // TLS_DHE_RSA_WITH_AES_256_CCM_8
		0xc09f, // TLS_DHE_RSA_WITH_AES_256_CCM

		// ARIA (256-bit)
		0xc05d, // TLS_ECDHE_ECDSA_WITH_ARIA_256_GCM_SHA384
		0xc061, // TLS_ECDHE_RSA_WITH_ARIA_256_GCM_SHA384
		0xc057, // TLS_DHE_DSS_WITH_ARIA_256_GCM_SHA384
		0xc053, // TLS_DHE_RSA_WITH_ARIA_256_GCM_SHA384

		// DHE-DSS + AES-GCM (128-bit)
		0x00a2, // TLS_DHE_DSS_WITH_AES_128_GCM_SHA256

		// AES-CCM (128-bit)
		0xc0ae, // TLS_ECDHE_ECDSA_WITH_AES_128_CCM_8
		0xc0ac, // TLS_ECDHE_ECDSA_WITH_AES_128_CCM
		0xc0a2, // TLS_DHE_RSA_WITH_AES_128_CCM_8
		0xc09e, // TLS_DHE_RSA_WITH_AES_128_CCM

		// ARIA (128-bit)
		0xc05c, // TLS_ECDHE_ECDSA_WITH_ARIA_128_GCM_SHA256
		0xc060, // TLS_ECDHE_RSA_WITH_ARIA_128_GCM_SHA256
		0xc056, // TLS_DHE_DSS_WITH_ARIA_128_GCM_SHA256
		0xc052, // TLS_DHE_RSA_WITH_ARIA_128_GCM_SHA256

		// ECDHE/DHE + AES-CBC-SHA384/256 (more)
		0xc024, // TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA384
		0x006a, // TLS_DHE_DSS_WITH_AES_256_CBC_SHA256
		0xc023, // TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA256
		0x0040, // TLS_DHE_DSS_WITH_AES_128_CBC_SHA256

		// ECDHE/DHE + AES-CBC-SHA (legacy)
		0xc00a, // TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA
		0xc014, // TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA
		0x0039, // TLS_DHE_RSA_WITH_AES_256_CBC_SHA
		0x0038, // TLS_DHE_DSS_WITH_AES_256_CBC_SHA
		0xc009, // TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA
		0xc013, // TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA
		0x0033, // TLS_DHE_RSA_WITH_AES_128_CBC_SHA
		0x0032, // TLS_DHE_DSS_WITH_AES_128_CBC_SHA

		// RSA + AES-GCM/CCM/ARIA (non-PFS, 256-bit)
		0x009d, // TLS_RSA_WITH_AES_256_GCM_SHA384
		0xc0a1, // TLS_RSA_WITH_AES_256_CCM_8
		0xc09d, // TLS_RSA_WITH_AES_256_CCM
		0xc051, // TLS_RSA_WITH_ARIA_256_GCM_SHA384

		// RSA + AES-GCM/CCM/ARIA (non-PFS, 128-bit)
		0x009c, // TLS_RSA_WITH_AES_128_GCM_SHA256
		0xc0a0, // TLS_RSA_WITH_AES_128_CCM_8
		0xc09c, // TLS_RSA_WITH_AES_128_CCM
		0xc050, // TLS_RSA_WITH_ARIA_128_GCM_SHA256

		// RSA + AES-CBC (non-PFS, legacy)
		0x003d, // TLS_RSA_WITH_AES_256_CBC_SHA256
		0x003c, // TLS_RSA_WITH_AES_128_CBC_SHA256
		0x0035, // TLS_RSA_WITH_AES_256_CBC_SHA
		0x002f, // TLS_RSA_WITH_AES_128_CBC_SHA

		// Renegotiation indication
		0x00ff, // TLS_EMPTY_RENEGOTIATION_INFO_SCSV
REDACTED

	// defaultCurves contains the 10 supported groups from Claude CLI (including FFDHE)
	defaultCurves = []utls.CurveID{
		utls.X25519,          // 0x001d
		utls.CurveP256,       // 0x0017 (secp256r1)
		utls.CurveID(0x001e), // x448
		utls.CurveP521,       // 0x0019 (secp521r1)
		utls.CurveP384,       // 0x0018 (secp384r1)
		utls.CurveID(0x0100), // ffdhe2048
		utls.CurveID(0x0101), // ffdhe3072
		utls.CurveID(0x0102), // ffdhe4096
		utls.CurveID(0x0103), // ffdhe6144
		utls.CurveID(0x0104), // ffdhe8192
REDACTED

	// defaultPointFormats contains all 3 point formats from Claude CLI
	defaultPointFormats = []uint8{
		0, // uncompressed
		1, // ansiX962_compressed_prime
		2, // ansiX962_compressed_char2
REDACTED

	// defaultSignatureAlgorithms contains the 20 signature algorithms from Claude CLI
	defaultSignatureAlgorithms = []utls.SignatureScheme{
		0x0403, // ecdsa_secp256r1_sha256
		0x0503, // ecdsa_secp384r1_sha384
		0x0603, // ecdsa_secp521r1_sha512
		0x0807, // ed25519
		0x0808, // ed448
		0x0809, // rsa_pss_pss_sha256
		0x080a, // rsa_pss_pss_sha384
		0x080b, // rsa_pss_pss_sha512
		0x0804, // rsa_pss_rsae_sha256
		0x0805, // rsa_pss_rsae_sha384
		0x0806, // rsa_pss_rsae_sha512
		0x0401, // rsa_pkcs1_sha256
		0x0501, // rsa_pkcs1_sha384
		0x0601, // rsa_pkcs1_sha512
		0x0303, // ecdsa_sha224
		0x0301, // rsa_pkcs1_sha224
		0x0302, // dsa_sha224
		0x0402, // dsa_sha256
		0x0502, // dsa_sha384
		0x0602, // dsa_sha512
REDACTED
)

// NewDialer creates a new TLS fingerprint dialer.
// baseDialer is used for TCP connection establishment (supports proxy scenarios).
// If baseDialer is nil, direct TCP dial is used.
func NewDialer(profile *Profile, baseDialer func(ctx context.Context, network, addr string) (net.Conn, error)) *Dialer {
	if baseDialer == nil {
		baseDialer = (&net.Dialer{REDACTED).DialContext
REDACTED
	return &Dialer{profile: profile, baseDialer: baseDialerREDACTED
REDACTED

// NewHTTPProxyDialer creates a new TLS fingerprint dialer that works through HTTP/HTTPS proxies.
// It establishes a CONNECT tunnel before performing TLS handshake with custom fingerprint.
func NewHTTPProxyDialer(profile *Profile, proxyURL *url.URL) *HTTPProxyDialer {
	return &HTTPProxyDialer{profile: profile, proxyURL: proxyURLREDACTED
REDACTED

// NewSOCKS5ProxyDialer creates a new TLS fingerprint dialer that works through SOCKS5 proxies.
// It establishes a SOCKS5 tunnel before performing TLS handshake with custom fingerprint.
func NewSOCKS5ProxyDialer(profile *Profile, proxyURL *url.URL) *SOCKS5ProxyDialer {
	return &SOCKS5ProxyDialer{profile: profile, proxyURL: proxyURLREDACTED
REDACTED

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
	REDACTED
REDACTED

	// Determine proxy address
	proxyAddr := d.proxyURL.Host
	if d.proxyURL.Port() == "" {
		proxyAddr = net.JoinHostPort(d.proxyURL.Hostname(), "1080") // Default SOCKS5 port
REDACTED

	socksDialer, err := proxy.SOCKS5("tcp", proxyAddr, auth, proxy.Direct)
	if err != nil {
		slog.Debug("tls_fingerprint_socks5_dialer_failed", "error", err)
		return nil, fmt.Errorf("create SOCKS5 dialer: %w", err)
REDACTED

	// Step 2: Establish SOCKS5 tunnel to target
	slog.Debug("tls_fingerprint_socks5_establishing_tunnel", "target", addr)
	conn, err := socksDialer.Dial("tcp", addr)
	if err != nil {
		slog.Debug("tls_fingerprint_socks5_connect_failed", "error", err)
		return nil, fmt.Errorf("SOCKS5 connect: %w", err)
REDACTED
	slog.Debug("tls_fingerprint_socks5_tunnel_established")

	// Step 3: Perform TLS handshake on the tunnel with utls fingerprint
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
REDACTED
	slog.Debug("tls_fingerprint_socks5_starting_handshake", "host", host)

	// Build ClientHello specification from profile (Node.js/Claude CLI fingerprint)
	spec := buildClientHelloSpecFromProfile(d.profile)
	slog.Debug("tls_fingerprint_socks5_clienthello_spec",
		"cipher_suites", len(spec.CipherSuites),
		"extensions", len(spec.Extensions),
		"compression_methods", spec.CompressionMethods,
		"tls_vers_max", spec.TLSVersMax,
		"tls_vers_min", spec.TLSVersMin)

	if d.profile != nil {
		slog.Debug("tls_fingerprint_socks5_using_profile", "name", d.profile.Name, "grease", d.profile.EnableGREASE)
REDACTED

	// Create uTLS connection on the tunnel
	tlsConn := utls.UClient(conn, &utls.Config{
		ServerName: host,
REDACTED, utls.HelloCustom)

	if err := tlsConn.ApplyPreset(spec); err != nil {
		slog.Debug("tls_fingerprint_socks5_apply_preset_failed", "error", err)
		_ = conn.Close()
		return nil, fmt.Errorf("apply TLS preset: %w", err)
REDACTED

	if err := tlsConn.HandshakeContext(ctx); err != nil {
		slog.Debug("tls_fingerprint_socks5_handshake_failed", "error", err)
		_ = conn.Close()
		return nil, fmt.Errorf("TLS handshake failed: %w", err)
REDACTED

	state := tlsConn.ConnectionState()
	slog.Debug("tls_fingerprint_socks5_handshake_success",
		"version", state.Version,
		"cipher_suite", state.CipherSuite,
		"alpn", state.NegotiatedProtocol)

	return tlsConn, nil
REDACTED

// DialTLSContext establishes a TLS connection through HTTP proxy with the configured fingerprint.
// Flow: TCP connect to proxy -> CONNECT tunnel -> TLS handshake with utls
func (d *HTTPProxyDialer) DialTLSContext(ctx context.Context, network, addr string) (net.Conn, error) {
	slog.Debug("tls_fingerprint_http_proxy_connecting", "proxy", d.proxyURL.Host, "target", addr)

	// Step 1: TCP connect to proxy server
	var proxyAddr string
	if d.proxyURL.Port() != "" {
		proxyAddr = d.proxyURL.Host
REDACTED else {
		// Default ports
		if d.proxyURL.Scheme == "https" {
			proxyAddr = net.JoinHostPort(d.proxyURL.Hostname(), "443")
	REDACTED else {
			proxyAddr = net.JoinHostPort(d.proxyURL.Hostname(), "80")
	REDACTED
REDACTED

	dialer := &net.Dialer{REDACTED
	conn, err := dialer.DialContext(ctx, "tcp", proxyAddr)
	if err != nil {
		slog.Debug("tls_fingerprint_http_proxy_connect_failed", "error", err)
		return nil, fmt.Errorf("connect to proxy: %w", err)
REDACTED
	slog.Debug("tls_fingerprint_http_proxy_connected", "proxy_addr", proxyAddr)

	// Step 2: Send CONNECT request to establish tunnel
	req := &http.Request{
		Method: "CONNECT",
		URL:    &url.URL{Opaque: addrREDACTED,
		Host:   addr,
		Header: make(http.Header),
REDACTED

	// Add proxy authentication if present
	if d.proxyURL.User != nil {
		username := d.proxyURL.User.Username()
		password, _ := d.proxyURL.User.Password()
		auth := base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
		req.Header.Set("Proxy-Authorization", "Basic "+auth)
REDACTED

	slog.Debug("tls_fingerprint_http_proxy_sending_connect", "target", addr)
	if err := req.Write(conn); err != nil {
		_ = conn.Close()
		slog.Debug("tls_fingerprint_http_proxy_write_failed", "error", err)
		return nil, fmt.Errorf("write CONNECT request: %w", err)
REDACTED

	// Step 3: Read CONNECT response
	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, req)
	if err != nil {
		_ = conn.Close()
		slog.Debug("tls_fingerprint_http_proxy_read_response_failed", "error", err)
		return nil, fmt.Errorf("read CONNECT response: %w", err)
REDACTED
	defer func() { _ = resp.Body.Close() REDACTED()

	if resp.StatusCode != http.StatusOK {
		_ = conn.Close()
		slog.Debug("tls_fingerprint_http_proxy_connect_failed_status", "status_code", resp.StatusCode, "status", resp.Status)
		return nil, fmt.Errorf("proxy CONNECT failed: %s", resp.Status)
REDACTED
	slog.Debug("tls_fingerprint_http_proxy_tunnel_established")

	// Step 4: Perform TLS handshake on the tunnel with utls fingerprint
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
REDACTED
	slog.Debug("tls_fingerprint_http_proxy_starting_handshake", "host", host)

	// Build ClientHello specification (reuse the shared method)
	spec := buildClientHelloSpecFromProfile(d.profile)
	slog.Debug("tls_fingerprint_http_proxy_clienthello_spec",
		"cipher_suites", len(spec.CipherSuites),
		"extensions", len(spec.Extensions))

	if d.profile != nil {
		slog.Debug("tls_fingerprint_http_proxy_using_profile", "name", d.profile.Name, "grease", d.profile.EnableGREASE)
REDACTED

	// Create uTLS connection on the tunnel
	// Note: TLS 1.3 cipher suites are handled automatically by utls when TLS 1.3 is in SupportedVersions
	tlsConn := utls.UClient(conn, &utls.Config{
		ServerName: host,
REDACTED, utls.HelloCustom)

	if err := tlsConn.ApplyPreset(spec); err != nil {
		slog.Debug("tls_fingerprint_http_proxy_apply_preset_failed", "error", err)
		_ = conn.Close()
		return nil, fmt.Errorf("apply TLS preset: %w", err)
REDACTED

	if err := tlsConn.HandshakeContext(ctx); err != nil {
		slog.Debug("tls_fingerprint_http_proxy_handshake_failed", "error", err)
		_ = conn.Close()
		return nil, fmt.Errorf("TLS handshake failed: %w", err)
REDACTED

	state := tlsConn.ConnectionState()
	slog.Debug("tls_fingerprint_http_proxy_handshake_success",
		"version", state.Version,
		"cipher_suite", state.CipherSuite,
		"alpn", state.NegotiatedProtocol)

	return tlsConn, nil
REDACTED

// DialTLSContext establishes a TLS connection with the configured fingerprint.
// This method is designed to be used as http.Transport.DialTLSContext.
func (d *Dialer) DialTLSContext(ctx context.Context, network, addr string) (net.Conn, error) {
	// Establish TCP connection using base dialer (supports proxy)
	slog.Debug("tls_fingerprint_dialing_tcp", "addr", addr)
	conn, err := d.baseDialer(ctx, network, addr)
	if err != nil {
		slog.Debug("tls_fingerprint_tcp_dial_failed", "error", err)
		return nil, err
REDACTED
	slog.Debug("tls_fingerprint_tcp_connected", "addr", addr)

	// Extract hostname for SNI
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
REDACTED
	slog.Debug("tls_fingerprint_sni_hostname", "host", host)

	// Build ClientHello specification
	spec := d.buildClientHelloSpec()
	slog.Debug("tls_fingerprint_clienthello_spec",
		"cipher_suites", len(spec.CipherSuites),
		"extensions", len(spec.Extensions))

	// Log profile info
	if d.profile != nil {
		slog.Debug("tls_fingerprint_using_profile", "name", d.profile.Name, "grease", d.profile.EnableGREASE)
REDACTED else {
		slog.Debug("tls_fingerprint_using_default_profile")
REDACTED

	// Create uTLS connection
	// Note: TLS 1.3 cipher suites are handled automatically by utls when TLS 1.3 is in SupportedVersions
	tlsConn := utls.UClient(conn, &utls.Config{
		ServerName: host,
REDACTED, utls.HelloCustom)

	// Apply fingerprint
	if err := tlsConn.ApplyPreset(spec); err != nil {
		slog.Debug("tls_fingerprint_apply_preset_failed", "error", err)
		_ = conn.Close()
		return nil, err
REDACTED
	slog.Debug("tls_fingerprint_preset_applied")

	// Perform TLS handshake
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		slog.Debug("tls_fingerprint_handshake_failed",
			"error", err,
			"local_addr", conn.LocalAddr(),
			"remote_addr", conn.RemoteAddr())
		_ = conn.Close()
		return nil, fmt.Errorf("TLS handshake failed: %w", err)
REDACTED

	// Log successful handshake details
	state := tlsConn.ConnectionState()
	slog.Debug("tls_fingerprint_handshake_success",
		"version", state.Version,
		"cipher_suite", state.CipherSuite,
		"alpn", state.NegotiatedProtocol)

	return tlsConn, nil
REDACTED

// buildClientHelloSpec constructs the ClientHello specification based on the profile.
func (d *Dialer) buildClientHelloSpec() *utls.ClientHelloSpec {
	return buildClientHelloSpecFromProfile(d.profile)
REDACTED

// toUTLSCurves converts uint16 slice to utls.CurveID slice.
func toUTLSCurves(curves []uint16) []utls.CurveID {
	result := make([]utls.CurveID, len(curves))
	for i, c := range curves {
		result[i] = utls.CurveID(c)
REDACTED
	return result
REDACTED

// buildClientHelloSpecFromProfile constructs ClientHelloSpec from a Profile.
// This is a standalone function that can be used by both Dialer and HTTPProxyDialer.
func buildClientHelloSpecFromProfile(profile *Profile) *utls.ClientHelloSpec {
	// Get cipher suites
	var cipherSuites []uint16
	if profile != nil && len(profile.CipherSuites) > 0 {
		cipherSuites = profile.CipherSuites
REDACTED else {
		cipherSuites = defaultCipherSuites
REDACTED

	// Get curves
	var curves []utls.CurveID
	if profile != nil && len(profile.Curves) > 0 {
		curves = toUTLSCurves(profile.Curves)
REDACTED else {
		curves = defaultCurves
REDACTED

	// Get point formats
	var pointFormats []uint8
	if profile != nil && len(profile.PointFormats) > 0 {
		pointFormats = profile.PointFormats
REDACTED else {
		pointFormats = defaultPointFormats
REDACTED

	// Check if GREASE is enabled
	enableGREASE := profile != nil && profile.EnableGREASE

	extensions := make([]utls.TLSExtension, 0, 16)

	if enableGREASE {
		extensions = append(extensions, &utls.UtlsGREASEExtension{REDACTED)
REDACTED

	// SNI extension - MUST be explicitly added for HelloCustom mode
	// utls will populate the server name from Config.ServerName
	extensions = append(extensions, &utls.SNIExtension{REDACTED)

	// Claude CLI extension order (captured from tshark):
	// server_name(0), ec_point_formats(11), supported_groups(10), session_ticket(35),
	// alpn(16), encrypt_then_mac(22), extended_master_secret(23),
	// signature_algorithms(13), supported_versions(43),
	// psk_key_exchange_modes(45), key_share(51)
	extensions = append(extensions,
		&utls.SupportedPointsExtension{SupportedPoints: pointFormatsREDACTED,
		&utls.SupportedCurvesExtension{Curves: curvesREDACTED,
		&utls.SessionTicketExtension{REDACTED,
		&utls.ALPNExtension{AlpnProtocols: []string{"http/1.1"REDACTEDREDACTED,
		&utls.GenericExtension{Id: 22REDACTED,
		&utls.ExtendedMasterSecretExtension{REDACTED,
		&utls.SignatureAlgorithmsExtension{SupportedSignatureAlgorithms: defaultSignatureAlgorithmsREDACTED,
		&utls.SupportedVersionsExtension{Versions: []uint16{
			utls.VersionTLS13,
			utls.VersionTLS12,
REDACTED
		&utls.PSKKeyExchangeModesExtension{Modes: []uint8{utls.PskModeDHEREDACTEDREDACTED,
		&utls.KeyShareExtension{KeyShares: []utls.KeyShare{
			{Group: utls.X25519REDACTED,
REDACTED
	)

	if enableGREASE {
		extensions = append(extensions, &utls.UtlsGREASEExtension{REDACTED)
REDACTED

	return &utls.ClientHelloSpec{
		CipherSuites:       cipherSuites,
		CompressionMethods: []uint8{0REDACTED, // null compression only (standard)
		Extensions:         extensions,
		TLSVersMax:         utls.VersionTLS13,
		TLSVersMin:         utls.VersionTLS10,
REDACTED
REDACTED
