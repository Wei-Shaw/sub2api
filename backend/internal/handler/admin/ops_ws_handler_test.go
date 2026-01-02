package admin

import (
	"net/http"
	"net/netip"
	"testing"
)

func TestIsAllowedOpsWSOrigin_AllowsEmptyOrigin(t *testing.T) {
	original := opsWSProxyConfig
	t.Cleanup(func() { opsWSProxyConfig = original REDACTED)
	opsWSProxyConfig = OpsWSProxyConfig{OriginPolicy: OriginPolicyPermissiveREDACTED

	req, err := http.NewRequest(http.MethodGet, "http://example.test", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
REDACTED

	if !isAllowedOpsWSOrigin(req) {
		t.Fatalf("expected empty Origin to be allowed")
REDACTED
REDACTED

func TestIsAllowedOpsWSOrigin_RejectsEmptyOrigin_WhenStrict(t *testing.T) {
	original := opsWSProxyConfig
	t.Cleanup(func() { opsWSProxyConfig = original REDACTED)
	opsWSProxyConfig = OpsWSProxyConfig{OriginPolicy: OriginPolicyStrictREDACTED

	req, err := http.NewRequest(http.MethodGet, "http://example.test", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
REDACTED

	if isAllowedOpsWSOrigin(req) {
		t.Fatalf("expected empty Origin to be rejected under strict policy")
REDACTED
REDACTED

func TestIsAllowedOpsWSOrigin_UsesXForwardedHostOnlyFromTrustedProxy(t *testing.T) {
	original := opsWSProxyConfig
	t.Cleanup(func() { opsWSProxyConfig = original REDACTED)

	opsWSProxyConfig = OpsWSProxyConfig{
		TrustProxy: true,
		TrustedProxies: []netip.Prefix{
			netip.MustParsePrefix("127.0.0.0/8"),
	REDACTED,
REDACTED

	// Untrusted peer: ignore X-Forwarded-Host and compare against r.Host.
	{
		req, err := http.NewRequest(http.MethodGet, "http://internal.service.local", nil)
		if err != nil {
			t.Fatalf("NewRequest: %v", err)
	REDACTED
		req.RemoteAddr = "192.0.2.1:12345"
		req.Host = "internal.service.local"
		req.Header.Set("Origin", "https://public.example.com")
		req.Header.Set("X-Forwarded-Host", "public.example.com")

		if isAllowedOpsWSOrigin(req) {
			t.Fatalf("expected Origin to be rejected when peer is not a trusted proxy")
	REDACTED
REDACTED

	// Trusted peer: allow X-Forwarded-Host to participate in Origin validation.
	{
		req, err := http.NewRequest(http.MethodGet, "http://internal.service.local", nil)
		if err != nil {
			t.Fatalf("NewRequest: %v", err)
	REDACTED
		req.RemoteAddr = "127.0.0.1:23456"
		req.Host = "internal.service.local"
		req.Header.Set("Origin", "https://public.example.com")
		req.Header.Set("X-Forwarded-Host", "public.example.com")

		if !isAllowedOpsWSOrigin(req) {
			t.Fatalf("expected Origin to be accepted when peer is a trusted proxy")
	REDACTED
REDACTED
REDACTED

func TestLoadOpsWSProxyConfigFromEnv_OriginPolicy(t *testing.T) {
	t.Setenv(envOpsWSOriginPolicy, "STRICT")
	cfg := loadOpsWSProxyConfigFromEnv()
	if cfg.OriginPolicy != OriginPolicyStrict {
		t.Fatalf("OriginPolicy=%q, want %q", cfg.OriginPolicy, OriginPolicyStrict)
REDACTED
REDACTED

func TestLoadOpsWSProxyConfigFromEnv_OriginPolicyInvalidUsesDefault(t *testing.T) {
	t.Setenv(envOpsWSOriginPolicy, "nope")
	cfg := loadOpsWSProxyConfigFromEnv()
	if cfg.OriginPolicy != OriginPolicyPermissive {
		t.Fatalf("OriginPolicy=%q, want %q", cfg.OriginPolicy, OriginPolicyPermissive)
REDACTED
REDACTED

func TestParseTrustedProxyList(t *testing.T) {
	prefixes, invalid := parseTrustedProxyList("10.0.0.1, 10.0.0.0/8, bad, ::1/128")
	if len(prefixes) != 3 {
		t.Fatalf("prefixes=%d, want 3", len(prefixes))
REDACTED
	if len(invalid) != 1 || invalid[0] != "bad" {
		t.Fatalf("invalid=%v, want [bad]", invalid)
REDACTED
REDACTED

func TestRequestPeerIP_ParsesIPv6(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "http://example.test", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
REDACTED
	req.RemoteAddr = "[::1]:1234"

	addr, ok := requestPeerIP(req)
	if !ok {
		t.Fatalf("expected IPv6 peer IP to parse")
REDACTED
	if addr != netip.MustParseAddr("::1") {
		t.Fatalf("addr=%s, want ::1", addr)
REDACTED
REDACTED
