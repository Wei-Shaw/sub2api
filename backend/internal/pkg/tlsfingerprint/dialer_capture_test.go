//go:build integration

package tlsfingerprint

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	utls "github.com/refraction-networking/utls"
)

// CapturedFingerprint mirrors the Fingerprint struct from tls-fingerprint-web.
// Used to deserialize the JSON response from the capture server.
type CapturedFingerprint struct {
	JA3Raw              string   `json:"ja3_raw"`
	JA3Hash             string   `json:"ja3_hash"`
	JA4                 string   `json:"ja4"`
	HTTP2               string   `json:"http2"`
	CipherSuites        []int    `json:"cipher_suites"`
	Curves              []int    `json:"curves"`
	PointFormats        []int    `json:"point_formats"`
	Extensions          []int    `json:"extensions"`
	SignatureAlgorithms []int    `json:"signature_algorithms"`
	ALPNProtocols       []string `json:"alpn_protocols"`
	SupportedVersions   []int    `json:"supported_versions"`
	KeyShareGroups      []int    `json:"key_share_groups"`
	PSKModes            []int    `json:"psk_modes"`
	CompressCertAlgos   []int    `json:"compress_cert_algos"`
	EnableGREASE        bool     `json:"enable_grease"`
}

// TestDialerAgainstCaptureServer connects to the tls-fingerprint-web capture server
// and verifies that the dialer's TLS fingerprint matches the configured Profile.
//
// Default capture server: https://tls.sub2api.org:8090
// Override with env: TLSFINGERPRINT_CAPTURE_URL=https://localhost:8443
//
// Run: go test -v -run TestDialerAgainstCaptureServer ./internal/pkg/tlsfingerprint/...
func TestDialerAgainstCaptureServer(t *testing.T) {
	captureURL := os.Getenv("TLSFINGERPRINT_CAPTURE_URL")
	if captureURL == "" {
		captureURL = "https://tls.sub2api.org:8090"
	}

	tests := []struct {
		name    string
		profile *Profile
	}{
		{
			name: "default_profile",
			profile: &Profile{
				Name:         "default",
				EnableGREASE: false,
				// All empty → uses built-in defaults
			},
		},
		{
			name: "linux_x64_node_v22171",
			profile: &Profile{
				Name:                "linux_x64_node_v22171",
				EnableGREASE:        false,
				CipherSuites:        []uint16{4866, 4867, 4865, 49199, 49195, 49200, 49196, 158, 49191, 103, 49192, 107, 163, 159, 52393, 52392, 52394, 49327, 49325, 49315, 49311, 49245, 49249, 49239, 49235, 162, 49326, 49324, 49314, 49310, 49244, 49248, 49238, 49234, 49188, 106, 49187, 64, 49162, 49172, 57, 56, 49161, 49171, 51, 50, 157, 49313, 49309, 49233, 156, 49312, 49308, 49232, 61, 60, 53, 47, 255},
				Curves:              []uint16{29, 23, 30, 25, 24, 256, 257, 258, 259, 260},
				PointFormats:        []uint16{0, 1, 2},
				SignatureAlgorithms: []uint16{0x0403, 0x0503, 0x0603, 0x0807, 0x0808, 0x0809, 0x080a, 0x080b, 0x0804, 0x0805, 0x0806, 0x0401, 0x0501, 0x0601, 0x0303, 0x0301, 0x0302, 0x0402, 0x0502, 0x0602},
				ALPNProtocols:       []string{"http/1.1"},
				SupportedVersions:   []uint16{0x0304, 0x0303},
				KeyShareGroups:      []uint16{29},
				PSKModes:            []uint16{1},
				Extensions:          []uint16{0, 11, 10, 35, 16, 22, 23, 13, 43, 45, 51},
			},
		},
		{
			name: "macos_arm64_node_v2430",
			profile: &Profile{
				Name:                "MacOS_arm64_node_v2430",
				EnableGREASE:        false,
				CipherSuites:        []uint16{4865, 4866, 4867, 49195, 49199, 49196, 49200, 52393, 52392, 49161, 49171, 49162, 49172, 156, 157, 47, 53},
				Curves:              []uint16{29, 23, 24},
				PointFormats:        []uint16{0},
				SignatureAlgorithms: []uint16{0x0403, 0x0804, 0x0401, 0x0503, 0x0805, 0x0501, 0x0806, 0x0601, 0x0201},
				ALPNProtocols:       []string{"http/1.1"},
				SupportedVersions:   []uint16{0x0304, 0x0303},
				KeyShareGroups:      []uint16{29},
				PSKModes:            []uint16{1},
				Extensions:          []uint16{0, 65037, 23, 65281, 10, 11, 35, 16, 5, 13, 18, 51, 45, 43},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			captured := fetchCapturedFingerprint(t, captureURL, tc.profile)
			if captured == nil {
				return
			}

			t.Logf("JA3 Hash: %s", captured.JA3Hash)
			t.Logf("JA4:      %s", captured.JA4)

			// Resolve effective profile values (what the dialer actually uses)
			effectiveCipherSuites := tc.profile.CipherSuites
			if len(effectiveCipherSuites) == 0 {
				effectiveCipherSuites = defaultCipherSuites
			}
			effectiveCurves := tc.profile.Curves
			if len(effectiveCurves) == 0 {
				effectiveCurves = make([]uint16, len(defaultCurves))
				for i, c := range defaultCurves {
					effectiveCurves[i] = uint16(c)
				}
			}
			effectivePointFormats := tc.profile.PointFormats
			if len(effectivePointFormats) == 0 {
				effectivePointFormats = defaultPointFormats
			}
			effectiveSigAlgs := tc.profile.SignatureAlgorithms
			if len(effectiveSigAlgs) == 0 {
				effectiveSigAlgs = make([]uint16, len(defaultSignatureAlgorithms))
				for i, s := range defaultSignatureAlgorithms {
					effectiveSigAlgs[i] = uint16(s)
				}
			}
			effectiveALPN := tc.profile.ALPNProtocols
			if len(effectiveALPN) == 0 {
				effectiveALPN = []string{"http/1.1"}
			}
			effectiveVersions := tc.profile.SupportedVersions
			if len(effectiveVersions) == 0 {
				effectiveVersions = []uint16{0x0304, 0x0303}
			}
			effectiveKeyShare := tc.profile.KeyShareGroups
			if len(effectiveKeyShare) == 0 {
				effectiveKeyShare = []uint16{29} // X25519
			}
			effectivePSKModes := tc.profile.PSKModes
			if len(effectivePSKModes) == 0 {
				effectivePSKModes = []uint16{1} // psk_dhe_ke
			}

			// Verify each field
			assertIntSliceEqual(t, "cipher_suites", uint16sToInts(effectiveCipherSuites), captured.CipherSuites)
			assertIntSliceEqual(t, "curves", uint16sToInts(effectiveCurves), captured.Curves)
			assertIntSliceEqual(t, "point_formats", uint16sToInts(effectivePointFormats), captured.PointFormats)
			assertIntSliceEqual(t, "signature_algorithms", uint16sToInts(effectiveSigAlgs), captured.SignatureAlgorithms)
			assertStringSliceEqual(t, "alpn_protocols", effectiveALPN, captured.ALPNProtocols)
			assertIntSliceEqual(t, "supported_versions", uint16sToInts(effectiveVersions), captured.SupportedVersions)
			assertIntSliceEqual(t, "key_share_groups", uint16sToInts(effectiveKeyShare), captured.KeyShareGroups)
			assertIntSliceEqual(t, "psk_modes", uint16sToInts(effectivePSKModes), captured.PSKModes)

			if captured.EnableGREASE != tc.profile.EnableGREASE {
				t.Errorf("enable_grease: got %v, want %v", captured.EnableGREASE, tc.profile.EnableGREASE)
			} else {
				t.Logf("  enable_grease: %v OK", captured.EnableGREASE)
			}

			// Verify extension order
			// Use profile.Extensions if set, otherwise the default order (Node.js 24.x)
			expectedExtOrder := uint16sToInts(defaultExtensionOrder)
			if len(tc.profile.Extensions) > 0 {
				expectedExtOrder = uint16sToInts(tc.profile.Extensions)
			}
			// Strip GREASE values from both expected and captured for comparison
			var filteredExpected, filteredActual []int
			for _, e := range expectedExtOrder {
				if !isGREASEValue(uint16(e)) {
					filteredExpected = append(filteredExpected, e)
				}
			}
			for _, e := range captured.Extensions {
				if !isGREASEValue(uint16(e)) {
					filteredActual = append(filteredActual, e)
				}
			}
			assertIntSliceEqual(t, "extensions (order, non-GREASE)", filteredExpected, filteredActual)

			// Print full captured data as JSON for debugging
			capturedJSON, _ := json.MarshalIndent(captured, "  ", "  ")
			t.Logf("Full captured fingerprint:\n  %s", string(capturedJSON))
		})
	}
}

func fetchCapturedFingerprint(t *testing.T, captureURL string, profile *Profile) *CapturedFingerprint {
	t.Helper()

	dialer := NewDialer(profile, nil)
	client := &http.Client{
		Transport: &http.Transport{
			DialTLSContext: dialer.DialTLSContext,
		},
		Timeout: 10 * time.Second,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", captureURL, strings.NewReader(`{"model":"test"}`))
	if err != nil {
		t.Fatalf("create request: %v", err)
		return nil
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-token")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
		return nil
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
		return nil
	}

	var fp CapturedFingerprint
	if err := json.Unmarshal(body, &fp); err != nil {
		t.Logf("Response body: %s", string(body))
		t.Fatalf("parse response: %v", err)
		return nil
	}

	return &fp
}

func uint16sToInts(vals []uint16) []int {
	result := make([]int, len(vals))
	for i, v := range vals {
		result[i] = int(v)
	}
	return result
}

func assertIntSliceEqual(t *testing.T, name string, expected, actual []int) {
	t.Helper()
	if len(expected) != len(actual) {
		t.Errorf("%s: length mismatch: got %d, want %d", name, len(actual), len(expected))
		if len(actual) < 20 && len(expected) < 20 {
			t.Errorf("  got:  %v", actual)
			t.Errorf("  want: %v", expected)
		}
		return
	}
	mismatches := 0
	for i := range expected {
		if expected[i] != actual[i] {
			if mismatches < 5 {
				t.Errorf("%s[%d]: got %d (0x%04x), want %d (0x%04x)", name, i, actual[i], actual[i], expected[i], expected[i])
			}
			mismatches++
		}
	}
	if mismatches == 0 {
		t.Logf("  %s: %d items OK", name, len(expected))
	} else if mismatches > 5 {
		t.Errorf("  %s: %d/%d mismatches (showing first 5)", name, mismatches, len(expected))
	}
}

func assertStringSliceEqual(t *testing.T, name string, expected, actual []string) {
	t.Helper()
	if len(expected) != len(actual) {
		t.Errorf("%s: length mismatch: got %d (%v), want %d (%v)", name, len(actual), actual, len(expected), expected)
		return
	}
	for i := range expected {
		if expected[i] != actual[i] {
			t.Errorf("%s[%d]: got %q, want %q", name, i, actual[i], expected[i])
			return
		}
	}
	t.Logf("  %s: %v OK", name, expected)
}

// TestBuildClientHelloSpecNewFields tests that new Profile fields are correctly applied.
func TestBuildClientHelloSpecNewFields(t *testing.T) {
	// Test custom ALPN, versions, key shares, PSK modes
	profile := &Profile{
		Name:                "custom_full",
		EnableGREASE:        false,
		CipherSuites:        []uint16{0x1301, 0x1302},
		Curves:              []uint16{29, 23},
		PointFormats:        []uint16{0},
		SignatureAlgorithms: []uint16{0x0403, 0x0804},
		ALPNProtocols:       []string{"h2", "http/1.1"},
		SupportedVersions:   []uint16{0x0304},
		KeyShareGroups:      []uint16{29, 23},
		PSKModes:            []uint16{1},
	}

	spec := buildClientHelloSpecFromProfile(profile)

	// Verify cipher suites
	if len(spec.CipherSuites) != 2 || spec.CipherSuites[0] != 0x1301 {
		t.Errorf("cipher suites: got %v", spec.CipherSuites)
	}

	// Check extensions for expected values
	var foundALPN, foundVersions, foundKeyShare, foundPSK, foundSigAlgs bool
	for _, ext := range spec.Extensions {
		switch e := ext.(type) {
		case *utls.ALPNExtension:
			foundALPN = true
			if len(e.AlpnProtocols) != 2 || e.AlpnProtocols[0] != "h2" {
				t.Errorf("ALPN: got %v, want [h2, http/1.1]", e.AlpnProtocols)
			}
		case *utls.SupportedVersionsExtension:
			foundVersions = true
			if len(e.Versions) != 1 || e.Versions[0] != 0x0304 {
				t.Errorf("versions: got %v, want [0x0304]", e.Versions)
			}
		case *utls.KeyShareExtension:
			foundKeyShare = true
			if len(e.KeyShares) != 2 {
				t.Errorf("key shares: got %d entries, want 2", len(e.KeyShares))
			}
		case *utls.PSKKeyExchangeModesExtension:
			foundPSK = true
			if len(e.Modes) != 1 || e.Modes[0] != 1 {
				t.Errorf("PSK modes: got %v, want [1]", e.Modes)
			}
		case *utls.SignatureAlgorithmsExtension:
			foundSigAlgs = true
			if len(e.SupportedSignatureAlgorithms) != 2 {
				t.Errorf("sig algs: got %d, want 2", len(e.SupportedSignatureAlgorithms))
			}
		}
	}

	for name, found := range map[string]bool{
		"ALPN": foundALPN, "Versions": foundVersions, "KeyShare": foundKeyShare,
		"PSK": foundPSK, "SigAlgs": foundSigAlgs,
	} {
		if !found {
			t.Errorf("extension %s not found in spec", name)
		}
	}

	// Test nil profile uses all defaults
	specDefault := buildClientHelloSpecFromProfile(nil)
	for _, ext := range specDefault.Extensions {
		switch e := ext.(type) {
		case *utls.ALPNExtension:
			if len(e.AlpnProtocols) != 1 || e.AlpnProtocols[0] != "http/1.1" {
				t.Errorf("default ALPN: got %v, want [http/1.1]", e.AlpnProtocols)
			}
		case *utls.SupportedVersionsExtension:
			if len(e.Versions) != 2 {
				t.Errorf("default versions: got %v, want 2 entries", e.Versions)
			}
		case *utls.KeyShareExtension:
			if len(e.KeyShares) != 1 {
				t.Errorf("default key shares: got %d, want 1", len(e.KeyShares))
			}
		}
	}

	t.Log("TestBuildClientHelloSpecNewFields passed")
}
