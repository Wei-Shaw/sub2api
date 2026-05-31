//go:build ignore

package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	tlsfp "github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
)

func main() {
	baseURL := os.Getenv("ANTHROPIC_BASE_URL")
	authToken := os.Getenv("ANTHROPIC_AUTH_TOKEN")
	if baseURL == "" || authToken == "" {
		fmt.Fprintln(os.Stderr, "set ANTHROPIC_BASE_URL and ANTHROPIC_AUTH_TOKEN")
		os.Exit(1)
	}

	fmt.Printf("=== Test 1: standard Go TLS (no fingerprint) ===\n")
	doRequest(baseURL, authToken, nil)

	fmt.Printf("\n=== Test 2: uTLS Node.js fingerprint ===\n")
	profile, err := tlsfp.GetDefaultProfile()
	if err != nil {
		fmt.Fprintf(os.Stderr, "profile error: %v\n", err)
		os.Exit(1)
	}
	dialer := tlsfp.NewDialer(profile, nil)
	transport := &http.Transport{
		DialTLSContext: dialer.DialTLSContext,
		DialContext: (&net.Dialer{Timeout: 10 * time.Second}).DialContext,
	}
	doRequest(baseURL, authToken, transport)
}

func doRequest(baseURL, authToken string, transport http.RoundTripper) {
	body := `{"model":"claude-opus-4-8","max_tokens":10,"messages":[{"role":"user","content":"hi"}]}`
	
	var client http.Client
	if transport != nil {
		client.Transport = transport
	} else {
		client.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: false},
		}
	}
	client.Timeout = 20 * time.Second

	url := strings.TrimRight(baseURL, "/") + "/v1/messages"
	req, _ := http.NewRequestWithContext(context.Background(), "POST", url, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("x-api-key", authToken)
	req.Header.Set("User-Agent", "claude-code/1.0.0 (node/22.17.1)")

	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("  ERROR: %v\n", err)
		return
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
	fmt.Printf("  Status: %d\n", resp.StatusCode)
	fmt.Printf("  Body: %s\n", string(respBody))
}
