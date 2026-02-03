// Package tlsfingerprint provides TLS fingerprint simulation for HTTP clients.
//
// Unit tests for TLS fingerprint dialer.
// Integration tests that require external network are in dialer_integration_test.go
// and require the 'integration' build tag.
//
// Run unit tests: go test -v ./internal/pkg/tlsfingerprint/...
// Run integration tests: go test -v -tags=integration ./internal/pkg/tlsfingerprint/...
package tlsfingerprint

import (
	"net/url"
	"testing"
)

// FingerprintResponse represents the response from tls.peet.ws/api/all.
type FingerprintResponse struct {
	IP    string  `json:"ip"`
	TLS   TLSInfo `json:"tls"`
	HTTP2 any     `json:"http2"`
REDACTED

// TLSInfo contains TLS fingerprint details.
type TLSInfo struct {
	JA3           string `json:"ja3"`
	JA3Hash       string `json:"ja3_hash"`
	JA4           string `json:"ja4"`
	PeetPrint     string `json:"peetprint"`
	PeetPrintHash string `json:"peetprint_hash"`
	ClientRandom  string `json:"client_random"`
	SessionID     string `json:"session_id"`
REDACTED

// TestDialerWithProfile tests that different profiles produce different fingerprints.
func TestDialerWithProfile(t *testing.T) {
	// Create two dialers with different profiles
	profile1 := &Profile{
		Name:         "Profile 1 - No GREASE",
		EnableGREASE: false,
REDACTED
	profile2 := &Profile{
		Name:         "Profile 2 - With GREASE",
		EnableGREASE: true,
REDACTED

	dialer1 := NewDialer(profile1, nil)
	dialer2 := NewDialer(profile2, nil)

	// Build specs and compare
	// Note: We can't directly compare JA3 without making network requests
	// but we can verify the specs are different
	spec1 := dialer1.buildClientHelloSpec()
	spec2 := dialer2.buildClientHelloSpec()

	// Profile with GREASE should have more extensions
	if len(spec2.Extensions) <= len(spec1.Extensions) {
		t.Error("expected GREASE profile to have more extensions")
REDACTED
REDACTED

// TestHTTPProxyDialerBasic tests HTTP proxy dialer creation.
// Note: This is a unit test - actual proxy testing requires a proxy server.
func TestHTTPProxyDialerBasic(t *testing.T) {
	profile := &Profile{
		Name:         "Test Profile",
		EnableGREASE: false,
REDACTED

	// Test that dialer is created without panic
	proxyURL := mustParseURL("http://proxy.example.com:8080")
	dialer := NewHTTPProxyDialer(profile, proxyURL)

	if dialer == nil {
		t.Fatal("expected dialer to be created")
REDACTED
	if dialer.profile != profile {
		t.Error("expected profile to be set")
REDACTED
	if dialer.proxyURL != proxyURL {
		t.Error("expected proxyURL to be set")
REDACTED
REDACTED

// TestSOCKS5ProxyDialerBasic tests SOCKS5 proxy dialer creation.
// Note: This is a unit test - actual proxy testing requires a proxy server.
func TestSOCKS5ProxyDialerBasic(t *testing.T) {
	profile := &Profile{
		Name:         "Test Profile",
		EnableGREASE: false,
REDACTED

	// Test that dialer is created without panic
	proxyURL := mustParseURL("socks5://proxy.example.com:1080")
	dialer := NewSOCKS5ProxyDialer(profile, proxyURL)

	if dialer == nil {
		t.Fatal("expected dialer to be created")
REDACTED
	if dialer.profile != profile {
		t.Error("expected profile to be set")
REDACTED
	if dialer.proxyURL != proxyURL {
		t.Error("expected proxyURL to be set")
REDACTED
REDACTED

// TestBuildClientHelloSpec tests ClientHello spec construction.
func TestBuildClientHelloSpec(t *testing.T) {
	// Test with nil profile (should use defaults)
	spec := buildClientHelloSpecFromProfile(nil)

	if len(spec.CipherSuites) == 0 {
		t.Error("expected cipher suites to be set")
REDACTED
	if len(spec.Extensions) == 0 {
		t.Error("expected extensions to be set")
REDACTED

	// Verify default cipher suites are used
	if len(spec.CipherSuites) != len(defaultCipherSuites) {
		t.Errorf("expected %d cipher suites, got %d", len(defaultCipherSuites), len(spec.CipherSuites))
REDACTED

	// Test with custom profile
	customProfile := &Profile{
		Name:         "Custom",
		EnableGREASE: false,
		CipherSuites: []uint16{0x1301, 0x1302REDACTED,
REDACTED
	spec = buildClientHelloSpecFromProfile(customProfile)

	if len(spec.CipherSuites) != 2 {
		t.Errorf("expected 2 cipher suites, got %d", len(spec.CipherSuites))
REDACTED
REDACTED

// TestToUTLSCurves tests curve ID conversion.
func TestToUTLSCurves(t *testing.T) {
	input := []uint16{0x001d, 0x0017, 0x0018REDACTED
	result := toUTLSCurves(input)

	if len(result) != len(input) {
		t.Errorf("expected %d curves, got %d", len(input), len(result))
REDACTED

	for i, curve := range result {
		if uint16(curve) != input[i] {
			t.Errorf("curve %d: expected 0x%04x, got 0x%04x", i, input[i], uint16(curve))
	REDACTED
REDACTED
REDACTED

// Helper function to parse URL without error handling.
func mustParseURL(rawURL string) *url.URL {
	u, err := url.Parse(rawURL)
	if err != nil {
		panic(err)
REDACTED
	return u
REDACTED
