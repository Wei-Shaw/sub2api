//go:build integration

package tlsfingerprint

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	utls "github.com/refraction-networking/utls"
)

// Integration tests validating the default TLS fingerprint against real
// ClientHello captures of Claude Code 2.1.280 (Node.js runtime with the
// X25519MLKEM768 post-quantum hybrid group).
//
// Requires a local capture server (peek ClientHello → parse JA3 → complete
// handshake → log JSON lines) and env:
//
//	TLSFINGERPRINT_CAPTURE_ADDR  host:port of the capture server
//	TLSFINGERPRINT_CAPTURE_FILE  path to its JSONL output file
//
// Run: go test -tags=integration -run TestMLKEM768 ./internal/pkg/tlsfingerprint/

// captureEntry mirrors the capture server JSON fields used here.
type captureEntry struct {
	JA3String string `json:"ja3_string"`
	JA3Hash   string `json:"ja3_hash"`
	ALPN      string `json:"alpn_negotiated"`
	Error     string `json:"error"`
}

func readLastCapture(t *testing.T) captureEntry {
	t.Helper()
	path := os.Getenv("TLSFINGERPRINT_CAPTURE_FILE")
	if path == "" {
		t.Skip("TLSFINGERPRINT_CAPTURE_FILE not set")
	}
	// The capture server appends asynchronously; retry until a complete JSON
	// line shows up.
	var lastErr error
	for attempt := 0; attempt < 30; attempt++ {
		data, err := os.ReadFile(path)
		if err == nil {
			for i := len(data) - 1; i >= 0; i-- {
				if data[i] == '\n' && i < len(data)-1 {
					var e captureEntry
					if err := json.Unmarshal(data[i+1:], &e); err == nil {
						return e
					}
				}
			}
			var e captureEntry
			if err := json.Unmarshal(data, &e); err == nil && data[len(data)-1] == '\n' {
				return e
			}
			lastErr = fmt.Errorf("no complete capture entry yet (file %d bytes)", len(data))
		} else {
			lastErr = err
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("read capture file: %v", lastErr)
	var e captureEntry
	return e
}

// Real Claude Code 2.1.280 ClientHello captured 2026-09-22 (IP-target capture,
// so no SNI/ECH GREASE; JA3 de-GREASE + SNI drop makes that immaterial).
const wantClaudeCode280JA3 = "0303,1301-1302-1303-c02b-c02f-c02c-c030-cca9-cca8-c009-c013-c00a-c014-009c-009d-002f-0035,0017-ff01-000a-000b-0023-0010-0005-000d-0012-0033-002d-002b,11ec-001d-0017-0018,00"

// TestMLKEM768JA3MatchesClaudeCode280 drives a no-SNI/no-ECH profile with the
// post-quantum defaults against the capture server and byte-compares the
// resulting JA3 with the real Claude Code 2.1.280 ClientHello. The PQ key
// share is a real ML-KEM-768 encapsulation key (Go crypto/mlkem via utls), so
// the server can complete the handshake for either a PQ-selected or
// classical-selected share.
func TestMLKEM768JA3MatchesClaudeCode280(t *testing.T) {
	addr := os.Getenv("TLSFINGERPRINT_CAPTURE_ADDR")
	if addr == "" {
		t.Skip("TLSFINGERPRINT_CAPTURE_ADDR not set")
	}
	prof := &Profile{
		Name:           "claude-code-2.1.280-at-IP",
		EnableGREASE:   false,
		Curves:         []uint16{4588, 29, 23, 24}, // 0x11ec, 0x1d, 0x17, 0x18
		KeyShareGroups: []uint16{4588, 29},          // PQ hybrid + classical, independent keys like OpenSSL 3.5
		ALPNProtocols:  []string{"http/1.1"},
		Extensions:     []uint16{23, 65281, 10, 11, 35, 16, 5, 13, 18, 51, 45, 43},
	}
	d := NewDialer(prof, nil)
	conn, err := d.DialTLSContext(context.Background(), "tcp", addr)
	if err != nil {
		t.Fatalf("PQ handshake failed: %v", err)
	}
	uc, ok := conn.(*utls.UConn)
	if !ok {
		t.Fatalf("expected *utls.UConn, got %T", conn)
	}
	t.Logf("handshake OK: alpn=%q cipher=0x%04x", uc.ConnectionState().NegotiatedProtocol, uc.ConnectionState().CipherSuite)
	// Behave like the real CLI: send a request so the capture server completes
	// its normal read/response cycle, then drain briefly and close.
	req := "POST /v1/messages?beta=true HTTP/1.1\r\nHost: " + addr + "\r\nContent-Type: application/json\r\nContent-Length: 0\r\n\r\n"
	if _, err := conn.Write([]byte(req)); err != nil {
		t.Fatalf("post-handshake write failed: %v", err)
	}
	buf := make([]byte, 256)
	_, _ = conn.Read(buf)
	_ = conn.Close()

	got := readLastCapture(t)
	// Session-level errors after the ClientHello peek (e.g. alerts during the
	// server's post-handshake read) do not affect JA3 — the ClientHello is
	// parsed from raw bytes before any session state exists.
	t.Logf("capture session error (informational): %q", got.Error)
	if got.JA3String != wantClaudeCode280JA3 {
		t.Fatalf("JA3 mismatch\n got: %s\nwant: %s", got.JA3String, wantClaudeCode280JA3)
	}
	t.Logf("JA3 matches real Claude Code 2.1.280: %s", got.JA3Hash)
}

// TestDefaultProfilePQInterop verifies the unmodified default profile (SNI +
// ECH GREASE + post-quantum key share) still completes a handshake — i.e.
// real ML-KEM key generation works and PQ-capable and classical-only servers
// both interop.
func TestDefaultProfilePQInterop(t *testing.T) {
	addr := os.Getenv("TLSFINGERPRINT_CAPTURE_ADDR")
	if addr == "" {
		t.Skip("TLSFINGERPRINT_CAPTURE_ADDR not set")
	}
	d := NewDialer(nil, nil)
	conn, err := d.DialTLSContext(context.Background(), "tcp", addr)
	if err != nil {
		t.Fatalf("default-profile PQ handshake failed: %v", err)
	}
	uc := conn.(*utls.UConn)
	t.Logf("default profile handshake OK: alpn=%q", uc.ConnectionState().NegotiatedProtocol)
	_ = conn.Close()
}
