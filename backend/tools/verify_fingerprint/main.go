// verify_fingerprint dials a local capture server using the tlsfingerprint
// package's built-in default profile (same path sub2api's outbound Anthropic
// traffic takes), so we can compare the resulting ClientHello against the
// real Claude Code capture byte-for-byte.
//
// Usage:
//
//	go run ./tools/verify_fingerprint -capture https://127.0.0.1:8443
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
)

func main() {
	captureURL := flag.String("capture", "https://127.0.0.1:8443", "capture server base URL")
	flag.Parse()

	u, err := url.Parse(*captureURL)
	if err != nil {
		log.Fatalf("parse capture url: %v", err)
	}
	_ = u

	// Use the tlsfingerprint package's default profile — same as what the
	// sub2api outbound path uses when no per-account profile is set.
	profile := &tlsfingerprint.Profile{
		Name:          "default",
		ALPNProtocols: []string{"http/1.1"},
	}

	dialer := tlsfingerprint.NewDialer(profile, nil)

	transport := &http.Transport{
		DialTLSContext:        dialer.DialTLSContext,
		ForceAttemptHTTP2:     false,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   15 * time.Second,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", *captureURL+"/v1/messages", strings.NewReader(`{"probe":true}`))
	if err != nil {
		log.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "sub2api-verify/1.0")

	resp, err := client.Do(req)
	if err != nil {
		// We expect a TLS failure (self-signed cert). That's fine — the
		// capture server still records the ClientHello before our side
		// closes. Print the error and exit 0 so the capture log is the
		// source of truth.
		fmt.Printf("dial error (expected — cert not trusted): %v\n", err)
		fmt.Printf("check capture server log for ClientHello details\n")
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	fmt.Printf("status=%d body=%s\n", resp.StatusCode, body)
}
