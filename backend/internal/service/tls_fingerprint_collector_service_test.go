package service

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestTLSFingerprintCollectorServiceLifecycleAndCapture(t *testing.T) {
	port := reserveTLSFingerprintCollectorPort(t)
	cfg := &config.Config{
		Server: config.ServerConfig{
			TLSFingerprintCollector: config.TLSFingerprintCollectorConfig{
				Host:                 "127.0.0.1",
				Port:                 port,
				PublicBaseURL:        fmt.Sprintf("https://127.0.0.1:%d", port),
				SessionTTLSeconds:    60,
				MaxRecordsPerSession: 5,
			},
		},
	}
	svc := NewTLSFingerprintCollectorService(cfg)
	ctx := context.Background()

	status, err := svc.Start(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, svc.Stop(ctx)) })
	require.True(t, status.Running)
	require.True(t, status.UsingGeneratedCert)
	require.NotEmpty(t, status.CAPEM)

	session, err := svc.CreateSession()
	require.NoError(t, err)
	require.Contains(t, session.CaptureURL, "/capture/"+session.Token)
	require.Equal(t, status.CAPEM, session.CAPEM)

	client := newTLSFingerprintCollectorTestClient(t, session.CAPEM)
	for _, tc := range []struct {
		userAgent string
		wantKind  string
	}{
		{userAgent: "Claude Code/2.0.0 Node.js/24.3.0", wantKind: "claude_code"},
		{userAgent: "codex-cli/0.1.0", wantKind: "codex"},
	} {
		req, err := http.NewRequest(http.MethodPost, session.CaptureURL, nil)
		require.NoError(t, err)
		req.Header.Set("User-Agent", tc.userAgent)
		resp, err := client.Do(req)
		require.NoError(t, err)
		body, _ := io.ReadAll(resp.Body)
		require.NoError(t, resp.Body.Close())
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))

		records, err := svc.ListCaptures(session.Token)
		require.NoError(t, err)
		require.NotEmpty(t, records)
		require.Equal(t, tc.wantKind, records[0].ClientKind)
		require.NotNil(t, records[0].Profile)
		require.NotEmpty(t, records[0].Profile.CipherSuites)
		require.NotEmpty(t, records[0].Profile.Extensions)
		require.Contains(t, records[0].YAML, "captured_profile:")
		require.NotEmpty(t, records[0].JA3Hash)
	}

	svc.DeleteSession(session.Token)
	_, err = svc.ListCaptures(session.Token)
	require.Error(t, err)

	require.NoError(t, svc.Stop(ctx))
	stopped := svc.Status()
	require.False(t, stopped.Running)
	require.Empty(t, stopped.CAPEM)
	require.False(t, stopped.UsingGeneratedCert)
}

func TestCaptureTokenFromRequest(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{name: "base path", path: "/capture/token-1", want: "token-1"},
		{name: "client appended path", path: "/capture/token-2/v1/responses", want: "token-2"},
		{name: "unsupported path", path: "/v1/responses", want: ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodPost, "https://collector.local"+tc.path, nil)
			require.NoError(t, err)
			require.Equal(t, tc.want, captureTokenFromRequest(req))
		})
	}

	req, err := http.NewRequest(http.MethodPost, "https://collector.local/v1/responses?token=query-token", nil)
	require.NoError(t, err)
	req.Header.Set("X-TLS-Fingerprint-Token", "header-token")
	require.Equal(t, "query-token", captureTokenFromRequest(req))

	req, err = http.NewRequest(http.MethodPost, "https://collector.local/v1/messages", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer claude-token")
	require.Equal(t, "claude-token", captureTokenFromRequest(req))

	req, err = http.NewRequest(http.MethodPost, "https://collector.local/v1/messages", nil)
	require.NoError(t, err)
	req.Header.Set("X-Api-Key", "api-key-token")
	require.Equal(t, "api-key-token", captureTokenFromRequest(req))
}

func TestTLSFingerprintCollectorServiceSessionLimits(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			TLSFingerprintCollector: config.TLSFingerprintCollectorConfig{
				SessionTTLSeconds:    1,
				MaxRecordsPerSession: 1,
			},
		},
	}
	svc := NewTLSFingerprintCollectorService(cfg)
	token := "token"
	now := time.Now()
	svc.sessions[token] = &tlsFingerprintCollectorSessionState{
		token:     token,
		expiresAt: now.Add(time.Minute),
	}

	require.NoError(t, svc.appendCapture(token, &TLSFingerprintCaptureRecord{ID: "1", CapturedAt: now}))
	require.NoError(t, svc.appendCapture(token, &TLSFingerprintCaptureRecord{ID: "2", CapturedAt: now.Add(time.Second)}))
	records, err := svc.ListCaptures(token)
	require.NoError(t, err)
	require.Len(t, records, 1)
	require.Equal(t, "2", records[0].ID)

	svc.sessions[token].expiresAt = time.Now().Add(-time.Second)
	_, err = svc.ListCaptures(token)
	require.Error(t, err)
}

func TestTLSFingerprintCaptureConnAcceptsLargeClientHello(t *testing.T) {
	record := buildLargeTLSFingerprintClientHelloRecord(t, 8*1024)
	serverConn, clientConn := net.Pipe()
	t.Cleanup(func() {
		_ = serverConn.Close()
		_ = clientConn.Close()
	})

	go func() {
		_, _ = clientConn.Write(record)
	}()

	captureConn := newTLSFingerprintCaptureConn(serverConn, &tls.Certificate{})
	err := captureConn.captureClientHello()
	require.NoError(t, err)
	require.NotNil(t, captureConn.captured)
	require.Contains(t, captureConn.captured.Extensions, uint16(21))
}

func reserveTLSFingerprintCollectorPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = ln.Close() }()
	addr, ok := ln.Addr().(*net.TCPAddr)
	require.True(t, ok)
	return addr.Port
}

func newTLSFingerprintCollectorTestClient(t *testing.T, caPEM string) *http.Client {
	t.Helper()
	pool := x509.NewCertPool()
	require.True(t, pool.AppendCertsFromPEM([]byte(caPEM)))
	return &http.Client{
		Transport: &http.Transport{
			TLSClientConfig:       &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12},
			ForceAttemptHTTP2:     false,
			ResponseHeaderTimeout: time.Second,
		},
		Timeout: 3 * time.Second,
	}
}

func buildLargeTLSFingerprintClientHelloRecord(t *testing.T, paddingLen int) []byte {
	t.Helper()
	extensions := make([]byte, 0, paddingLen+64)
	extensions = appendTLSExtension(extensions, 0, []byte{0, 0})
	extensions = appendTLSExtension(extensions, 10, []byte{0, 2, 0, 29})
	extensions = appendTLSExtension(extensions, 21, make([]byte, paddingLen))

	hello := make([]byte, 0, len(extensions)+128)
	hello = append(hello, 0x03, 0x03)
	hello = append(hello, make([]byte, 32)...)
	hello = append(hello, 0)
	hello = append(hello, 0, 2, 0x13, 0x01)
	hello = append(hello, 1, 0)
	hello = appendUint16(hello, uint16(len(extensions)))
	hello = append(hello, extensions...)

	handshake := []byte{1, byte(len(hello) >> 16), byte(len(hello) >> 8), byte(len(hello))}
	handshake = append(handshake, hello...)
	record := []byte{22, 0x03, 0x01, byte(len(handshake) >> 8), byte(len(handshake))}
	record = append(record, handshake...)
	require.Greater(t, len(record), 4096)
	return record
}

func appendTLSExtension(dst []byte, id uint16, payload []byte) []byte {
	dst = appendUint16(dst, id)
	dst = appendUint16(dst, uint16(len(payload)))
	return append(dst, payload...)
}

func appendUint16(dst []byte, v uint16) []byte {
	var buf [2]byte
	binary.BigEndian.PutUint16(buf[:], v)
	return append(dst, buf[:]...)
}
