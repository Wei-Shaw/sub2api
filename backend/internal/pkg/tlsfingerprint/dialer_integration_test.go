//go:build integration

// Package tlsfingerprint provides TLS fingerprint simulation for HTTP clients.
//
// Integration tests for verifying TLS fingerprint correctness.
// These tests make actual network requests to external services and should be run manually.
//
// Run with: go test -v -tags=integration ./internal/pkg/tlsfingerprint/...
package tlsfingerprint

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// skipIfExternalServiceUnavailable checks if the external service is available.
// If not, it skips the test instead of failing.
func skipIfExternalServiceUnavailable(t *testing.T, err error) {
REDACTED
	if err != nil {
		// Check for common network/TLS errors that indicate external service issues
		errStr := err.Error()
		if strings.Contains(errStr, "certificate has expired") ||
			strings.Contains(errStr, "certificate is not yet valid") ||
			strings.Contains(errStr, "connection refused") ||
			strings.Contains(errStr, "no such host") ||
			strings.Contains(errStr, "network is unreachable") ||
			strings.Contains(errStr, "timeout") ||
			strings.Contains(errStr, "deadline exceeded") {
			t.Skipf("skipping test: external service unavailable: %v", err)
	REDACTED
		t.Fatalf("failed to get fingerprint: %v", err)
REDACTED
REDACTED

// TestJA3Fingerprint verifies the JA3/JA4 fingerprint matches expected value.
// This test uses tls.peet.ws to verify the fingerprint.
// Expected JA3 hash: 1a28e69016765d92e3b381168d68922c (Claude CLI / Node.js 20.x)
// Expected JA4: t13d5911h1_a33745022dd6_1f22a2ca17c4 (d=domain) or t13i5911h1_... (i=IP)
func TestJA3Fingerprint(t *testing.T) {
	// Skip if network is unavailable or if running in short mode
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
REDACTED

	profile := &Profile{
		Name:         "Claude CLI Test",
		EnableGREASE: false,
REDACTED
	dialer := NewDialer(profile, nil)

	client := &http.Client{
		Transport: &http.Transport{
			DialTLSContext: dialer.DialTLSContext,
	REDACTED,
		Timeout: 30 * time.Second,
REDACTED

	// Use tls.peet.ws fingerprint detection API
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", "https://tls.peet.ws/api/all", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
REDACTED
	req.Header.Set("User-Agent", "Claude Code/2.0.0 Node.js/20.0.0")

	resp, err := client.Do(req)
	skipIfExternalServiceUnavailable(t, err)
	defer func() { _ = resp.Body.Close() REDACTED()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response: %v", err)
REDACTED

	var fpResp FingerprintResponse
	if err := json.Unmarshal(body, &fpResp); err != nil {
		t.Logf("Response body: %s", string(body))
		t.Fatalf("failed to parse fingerprint response: %v", err)
REDACTED

	// Log all fingerprint information
	t.Logf("JA3: %s", fpResp.TLS.JA3)
	t.Logf("JA3 Hash: %s", fpResp.TLS.JA3Hash)
	t.Logf("JA4: %s", fpResp.TLS.JA4)
	t.Logf("PeetPrint: %s", fpResp.TLS.PeetPrint)
	t.Logf("PeetPrint Hash: %s", fpResp.TLS.PeetPrintHash)

	// Verify JA3 hash matches expected value
	expectedJA3Hash := "1a28e69016765d92e3b381168d68922c"
	if fpResp.TLS.JA3Hash == expectedJA3Hash {
		t.Logf("✓ JA3 hash matches expected value: %s", expectedJA3Hash)
REDACTED else {
		t.Errorf("✗ JA3 hash mismatch: got %s, expected %s", fpResp.TLS.JA3Hash, expectedJA3Hash)
REDACTED

	// Verify JA4 fingerprint
	// JA4 format: t[version][sni][cipher_count][ext_count][alpn]_[cipher_hash]_[ext_hash]
	// Expected: t13d5910h1 (d=domain) or t13i5910h1 (i=IP)
	// The suffix _a33745022dd6_1f22a2ca17c4 should match
	expectedJA4Suffix := "_a33745022dd6_1f22a2ca17c4"
	if strings.HasSuffix(fpResp.TLS.JA4, expectedJA4Suffix) {
		t.Logf("✓ JA4 suffix matches expected value: %s", expectedJA4Suffix)
REDACTED else {
		t.Errorf("✗ JA4 suffix mismatch: got %s, expected suffix %s", fpResp.TLS.JA4, expectedJA4Suffix)
REDACTED

	// Verify JA4 prefix (t13d5911h1 or t13i5911h1)
	// d = domain (SNI present), i = IP (no SNI)
	// Since we connect to tls.peet.ws (domain), we expect 'd'
	expectedJA4Prefix := "t13d5911h1"
	if strings.HasPrefix(fpResp.TLS.JA4, expectedJA4Prefix) {
		t.Logf("✓ JA4 prefix matches: %s (t13=TLS1.3, d=domain, 59=ciphers, 11=extensions, h1=HTTP/1.1)", expectedJA4Prefix)
REDACTED else {
		// Also accept 'i' variant for IP connections
		altPrefix := "t13i5911h1"
		if strings.HasPrefix(fpResp.TLS.JA4, altPrefix) {
			t.Logf("✓ JA4 prefix matches (IP variant): %s", altPrefix)
	REDACTED else {
			t.Errorf("✗ JA4 prefix mismatch: got %s, expected %s or %s", fpResp.TLS.JA4, expectedJA4Prefix, altPrefix)
	REDACTED
REDACTED

	// Verify JA3 contains expected cipher suites (TLS 1.3 ciphers at the beginning)
	if strings.Contains(fpResp.TLS.JA3, "4866-4867-4865") {
		t.Logf("✓ JA3 contains expected TLS 1.3 cipher suites")
REDACTED else {
		t.Logf("Warning: JA3 does not contain expected TLS 1.3 cipher suites")
REDACTED

	// Verify extension list (should be 11 extensions including SNI)
	// Expected: 0-11-10-35-16-22-23-13-43-45-51
	expectedExtensions := "0-11-10-35-16-22-23-13-43-45-51"
	if strings.Contains(fpResp.TLS.JA3, expectedExtensions) {
		t.Logf("✓ JA3 contains expected extension list: %s", expectedExtensions)
REDACTED else {
		t.Logf("Warning: JA3 extension list may differ")
REDACTED
REDACTED

// TestProfileExpectation defines expected fingerprint values for a profile.
type TestProfileExpectation struct {
	Profile       *Profile
	ExpectedJA3   string // Expected JA3 hash (empty = don't check)
	ExpectedJA4   string // Expected full JA4 (empty = don't check)
	JA4CipherHash string // Expected JA4 cipher hash - the stable middle part (empty = don't check)
REDACTED

// TestAllProfiles tests multiple TLS fingerprint profiles against tls.peet.ws.
// Run with: go test -v -tags=integration -run TestAllProfiles ./internal/pkg/tlsfingerprint/...
func TestAllProfiles(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
REDACTED

	// Define all profiles to test with their expected fingerprints
	// These profiles are from config.yaml gateway.tls_fingerprint.profiles
	profiles := []TestProfileExpectation{
		{
			// Linux x64 Node.js v22.17.1
			// Expected JA3 Hash: 1a28e69016765d92e3b381168d68922c
			// Expected JA4: t13d5911h1_a33745022dd6_1f22a2ca17c4
			Profile: &Profile{
				Name:         "linux_x64_node_v22171",
				EnableGREASE: false,
				CipherSuites: []uint16{4866, 4867, 4865, 49199, 49195, 49200, 49196, 158, 49191, 103, 49192, 107, 163, 159, 52393, 52392, 52394, 49327, 49325, 49315, 49311, 49245, 49249, 49239, 49235, 162, 49326, 49324, 49314, 49310, 49244, 49248, 49238, 49234, 49188, 106, 49187, 64, 49162, 49172, 57, 56, 49161, 49171, 51, 50, 157, 49313, 49309, 49233, 156, 49312, 49308, 49232, 61, 60, 53, 47, 255REDACTED,
				Curves:       []uint16{29, 23, 30, 25, 24, 256, 257, 258, 259, 260REDACTED,
				PointFormats: []uint8{0, 1, 2REDACTED,
		REDACTED,
			JA4CipherHash: "a33745022dd6", // stable part
	REDACTED,
		{
			// MacOS arm64 Node.js v22.18.0
			// Expected JA3 Hash: 70cb5ca646080902703ffda87036a5ea
			// Expected JA4: t13d5912h1_a33745022dd6_dbd39dd1d406
			Profile: &Profile{
				Name:         "macos_arm64_node_v22180",
				EnableGREASE: false,
				CipherSuites: []uint16{4866, 4867, 4865, 49199, 49195, 49200, 49196, 158, 49191, 103, 49192, 107, 163, 159, 52393, 52392, 52394, 49327, 49325, 49315, 49311, 49245, 49249, 49239, 49235, 162, 49326, 49324, 49314, 49310, 49244, 49248, 49238, 49234, 49188, 106, 49187, 64, 49162, 49172, 57, 56, 49161, 49171, 51, 50, 157, 49313, 49309, 49233, 156, 49312, 49308, 49232, 61, 60, 53, 47, 255REDACTED,
				Curves:       []uint16{29, 23, 30, 25, 24, 256, 257, 258, 259, 260REDACTED,
				PointFormats: []uint8{0, 1, 2REDACTED,
		REDACTED,
			JA4CipherHash: "a33745022dd6", // stable part (same cipher suites)
	REDACTED,
REDACTED

	for _, tc := range profiles {
		tc := tc // capture range variable
		t.Run(tc.Profile.Name, func(t *testing.T) {
			fp := fetchFingerprint(t, tc.Profile)
			if fp == nil {
				return // fetchFingerprint already called t.Fatal
		REDACTED

			t.Logf("Profile: %s", tc.Profile.Name)
			t.Logf("  JA3:           %s", fp.JA3)
			t.Logf("  JA3 Hash:      %s", fp.JA3Hash)
			t.Logf("  JA4:           %s", fp.JA4)
			t.Logf("  PeetPrint:     %s", fp.PeetPrint)
			t.Logf("  PeetPrintHash: %s", fp.PeetPrintHash)

			// Verify expectations
			if tc.ExpectedJA3 != "" {
				if fp.JA3Hash == tc.ExpectedJA3 {
					t.Logf("  ✓ JA3 hash matches: %s", tc.ExpectedJA3)
			REDACTED else {
					t.Errorf("  ✗ JA3 hash mismatch: got %s, expected %s", fp.JA3Hash, tc.ExpectedJA3)
			REDACTED
		REDACTED

			if tc.ExpectedJA4 != "" {
				if fp.JA4 == tc.ExpectedJA4 {
					t.Logf("  ✓ JA4 matches: %s", tc.ExpectedJA4)
			REDACTED else {
					t.Errorf("  ✗ JA4 mismatch: got %s, expected %s", fp.JA4, tc.ExpectedJA4)
			REDACTED
		REDACTED

			// Check JA4 cipher hash (stable middle part)
			// JA4 format: prefix_cipherHash_extHash
			if tc.JA4CipherHash != "" {
				if strings.Contains(fp.JA4, "_"+tc.JA4CipherHash+"_") {
					t.Logf("  ✓ JA4 cipher hash matches: %s", tc.JA4CipherHash)
			REDACTED else {
					t.Errorf("  ✗ JA4 cipher hash mismatch: got %s, expected cipher hash %s", fp.JA4, tc.JA4CipherHash)
			REDACTED
		REDACTED
	REDACTED)
REDACTED
REDACTED

// fetchFingerprint makes a request to tls.peet.ws and returns the TLS fingerprint info.
func fetchFingerprint(t *testing.T, profile *Profile) *TLSInfo {
REDACTED

	dialer := NewDialer(profile, nil)
	client := &http.Client{
		Transport: &http.Transport{
			DialTLSContext: dialer.DialTLSContext,
	REDACTED,
		Timeout: 30 * time.Second,
REDACTED

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", "https://tls.peet.ws/api/all", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
		return nil
REDACTED
	req.Header.Set("User-Agent", "Claude Code/2.0.0 Node.js/20.0.0")

	resp, err := client.Do(req)
	skipIfExternalServiceUnavailable(t, err)
	defer func() { _ = resp.Body.Close() REDACTED()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response: %v", err)
		return nil
REDACTED

	var fpResp FingerprintResponse
	if err := json.Unmarshal(body, &fpResp); err != nil {
		t.Logf("Response body: %s", string(body))
		t.Fatalf("failed to parse fingerprint response: %v", err)
		return nil
REDACTED

	return &fpResp.TLS
REDACTED
