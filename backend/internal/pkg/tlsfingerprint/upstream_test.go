//go:build integration

package tlsfingerprint

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

// TestUpstreamWithAndWithoutFingerprint compares a real upstream relay request
// with standard Go TLS vs uTLS Node.js fingerprint.
//
// Run with:
//
//	ANTHROPIC_BASE_URL=https://... ANTHROPIC_AUTH_TOKEN=sk-... \
//	  go test -tags=integration -v -run TestUpstreamWithAndWithoutFingerprint ./internal/pkg/tlsfingerprint/
func TestUpstreamWithAndWithoutFingerprint(t *testing.T) {
	baseURL := os.Getenv("ANTHROPIC_BASE_URL")
	authToken := os.Getenv("ANTHROPIC_AUTH_TOKEN")
	if baseURL == "" || authToken == "" {
		t.Skip("set ANTHROPIC_BASE_URL and ANTHROPIC_AUTH_TOKEN to run this test")
	}

	body := `{"model":"claude-haiku-4-5","max_tokens":5,"messages":[{"role":"user","content":"say hi"}]}`

	makeRequest := func(t *testing.T, transport http.RoundTripper) (int, string) {
		t.Helper()
		url := strings.TrimRight(baseURL, "/") + "/v1/messages"
		req, err := http.NewRequestWithContext(context.Background(), "POST", url, strings.NewReader(body))
		if err != nil {
			t.Fatalf("create request: %v", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("anthropic-version", "2023-06-01")
		req.Header.Set("x-api-key", authToken)
		req.Header.Set("User-Agent", "claude-code/1.5.7 node/22.17.1 (linux; x64)")

		client := &http.Client{Transport: transport, Timeout: 20 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return 0, fmt.Sprintf("ERROR: %v", err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return resp.StatusCode, string(b)
	}

	t.Run("standard_go_tls", func(t *testing.T) {
		transport := &http.Transport{
			TLSClientConfig: &tls.Config{},
		}
		status, body := makeRequest(t, transport)
		t.Logf("Status: %d", status)
		t.Logf("Body:   %s", body)
		if status >= 400 {
			t.Errorf("standard TLS failed with status %d", status)
		}
	})

	t.Run("utls_node_v24_fingerprint", func(t *testing.T) {
		profile := &Profile{Name: "default_node_v24", EnableGREASE: false}
		dialer := NewDialer(profile, nil)
		transport := &http.Transport{
			DialTLSContext: dialer.DialTLSContext,
		}
		status, body := makeRequest(t, transport)
		t.Logf("Status: %d", status)
		t.Logf("Body:   %s", body)
		if status >= 400 {
			t.Errorf("uTLS fingerprint failed with status %d", status)
		}
	})

	t.Run("utls_node_v22_fingerprint", func(t *testing.T) {
		profile := &Profile{
			Name:         "linux_x64_node_v22171",
			EnableGREASE: false,
			CipherSuites: []uint16{4866, 4867, 4865, 49199, 49195, 49200, 49196, 158, 49191, 103, 49192, 107, 163, 159, 52393, 52392, 52394, 49327, 49325, 49315, 49311, 49245, 49249, 49239, 49235, 162, 49326, 49324, 49314, 49310, 49244, 49248, 49238, 49234, 49188, 106, 49187, 64, 49162, 49172, 57, 56, 49161, 49171, 51, 50, 157, 49313, 49309, 49233, 156, 49312, 49308, 49232, 61, 60, 53, 47, 255},
			Curves:       []uint16{29, 23, 30, 25, 24, 256, 257, 258, 259, 260},
			PointFormats: []uint16{0, 1, 2},
			Extensions:   []uint16{0, 11, 10, 35, 16, 22, 23, 13, 43, 45, 51},
		}
		dialer := NewDialer(profile, nil)
		transport := &http.Transport{
			DialTLSContext: dialer.DialTLSContext,
		}
		status, body := makeRequest(t, transport)
		t.Logf("Status: %d", status)
		t.Logf("Body:   %s", body)
		if status >= 400 {
			t.Errorf("uTLS v22 fingerprint failed with status %d", status)
		}
	})
}
