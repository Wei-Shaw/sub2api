package tlsfingerprint

import (
	"context"
	"crypto/md5"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

// This regression fixture was captured from a standalone codex-cli 0.160.1
// model-provider request to an isolated HTTPS endpoint on macOS arm64.
const codexCLIJA3 = "771,255-49196-49195-49188-49187-49162-49161-49160-49200-49199-49192-49191-49172-49171-49170-157-156-61-60-53-47-10,0-10-11-13-5-18-23,23-24-25,0"

func TestCodexCLIClientHelloJA3(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listener.Close() }()
	type helloResult struct {
		ja3 string
		ext map[uint16]string
	}
	result := make(chan helloResult, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			result <- helloResult{ja3: err.Error()}
			return
		}
		defer func() { _ = conn.Close() }()
		_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		header := make([]byte, 5)
		if _, err := io.ReadFull(conn, header); err != nil {
			result <- helloResult{ja3: err.Error()}
			return
		}
		body := make([]byte, int(binary.BigEndian.Uint16(header[3:])))
		if _, err := io.ReadFull(conn, body); err != nil {
			result <- helloResult{ja3: err.Error()}
			return
		}
		ja3, ext := clientHelloJA3(body)
		result <- helloResult{ja3: ja3, ext: ext}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go func() {
		baseDialer := func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, listener.Addr().String())
		}
		_, _ = NewDialer(CodexCLIProfile(), baseDialer).DialTLSContext(ctx, "tcp", "chatgpt.com:443")
	}()
	select {
	case got := <-result:
		if got.ja3 != codexCLIJA3 {
			t.Fatalf("JA3 mismatch\ngot  %s\nwant %s", got.ja3, codexCLIJA3)
		}
		if hash := fmt.Sprintf("%x", md5.Sum([]byte(got.ja3))); hash != "e4d448cdfe06dc1243c1eb026c74ac9a" {
			t.Fatalf("JA3 hash mismatch: %s", hash)
		}
		for id, want := range map[uint16]string{
			5: "0100000000", 10: "0006001700180019", 11: "0100",
			13: "001004010201050106010403020305030603", 18: "", 23: "",
		} {
			if got.ext[id] != want {
				t.Errorf("extension %d payload: got %q, want %q", id, got.ext[id], want)
			}
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}

func TestCodexCLIProfileLiveHandshake(t *testing.T) {
	if os.Getenv("CODEX_TLS_LIVE_TEST") != "1" {
		t.Skip("requires network access")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conn, err := NewDialer(CodexCLIProfile(), nil).DialTLSContext(ctx, "tcp", "chatgpt.com:443")
	if err != nil {
		t.Fatal(err)
	}
	_ = conn.Close()
	client := &http.Client{Transport: &http.Transport{DialTLSContext: NewDialer(CodexCLIProfile(), nil).DialTLSContext}, Timeout: 10 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://chatgpt.com/backend-api/codex/models", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.ProtoMajor != 1 || resp.StatusCode == 0 {
		t.Fatalf("unexpected upstream protocol/status: %s %d", resp.Proto, resp.StatusCode)
	}
}

func TestCodexCLIClientHelloThroughHTTPSProxy(t *testing.T) {
	seen := make(chan string, 1)
	proxyServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodConnect || r.Host != "chatgpt.com:443" || r.Header.Get("Proxy-Authorization") != "Basic dXNlcjpwYXNz" {
			seen <- "invalid CONNECT request"
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		hijacker, ok := w.(http.Hijacker)
		if !ok {
			seen <- "hijack unavailable"
			return
		}
		conn, reader, err := hijacker.Hijack()
		if err != nil {
			seen <- err.Error()
			return
		}
		defer func() { _ = conn.Close() }()
		_, _ = conn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
		_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		header := make([]byte, 5)
		if _, err := io.ReadFull(reader, header); err != nil {
			seen <- err.Error()
			return
		}
		body := make([]byte, int(binary.BigEndian.Uint16(header[3:])))
		if _, err := io.ReadFull(reader, body); err != nil {
			seen <- err.Error()
			return
		}
		ja3, _ := clientHelloJA3(body)
		seen <- ja3
	}))
	defer proxyServer.Close()
	proxyURL, err := url.Parse(proxyServer.URL)
	if err != nil {
		t.Fatal(err)
	}
	proxyURL.User = url.UserPassword("user", "pass")
	cert, err := x509.ParseCertificate(proxyServer.Certificate().Raw)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(cert)
	dialer := NewHTTPProxyDialer(CodexCLIProfile(), proxyURL)
	dialer.proxyTLSConfig = &tls.Config{RootCAs: roots}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go func() { _, _ = dialer.DialTLSContext(ctx, "tcp", "chatgpt.com:443") }()
	select {
	case got := <-seen:
		if got != codexCLIJA3 {
			t.Fatalf("HTTPS CONNECT inner JA3 mismatch: %s", got)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}

func clientHelloJA3(record []byte) (string, map[uint16]string) {
	if len(record) < 40 || record[0] != 1 {
		return "invalid ClientHello", nil
	}
	p := 4
	version := binary.BigEndian.Uint16(record[p:])
	p += 2 + 32
	p += 1 + int(record[p])
	cipherBytes := int(binary.BigEndian.Uint16(record[p:]))
	p += 2
	ciphers := make([]string, 0, cipherBytes/2)
	for i := 0; i < cipherBytes; i += 2 {
		ciphers = append(ciphers, fmt.Sprint(binary.BigEndian.Uint16(record[p+i:])))
	}
	p += cipherBytes
	p += 1 + int(record[p])
	end := p + 2 + int(binary.BigEndian.Uint16(record[p:]))
	p += 2
	var extensions, curves, points []string
	payloads := make(map[uint16]string)
	for p+4 <= end {
		id, size := binary.BigEndian.Uint16(record[p:]), int(binary.BigEndian.Uint16(record[p+2:]))
		q := p + 4
		extensions = append(extensions, fmt.Sprint(id))
		payloads[id] = hex.EncodeToString(record[q : q+size])
		switch id {
		case 10:
			for i := 0; i < int(binary.BigEndian.Uint16(record[q:])); i += 2 {
				curves = append(curves, fmt.Sprint(binary.BigEndian.Uint16(record[q+2+i:])))
			}
		case 11:
			for i := 0; i < int(record[q]); i++ {
				points = append(points, fmt.Sprint(record[q+1+i]))
			}
		}
		p = q + size
	}
	return fmt.Sprintf("%d,%s,%s,%s,%s", version, strings.Join(ciphers, "-"), strings.Join(extensions, "-"), strings.Join(curves, "-"), strings.Join(points, "-")), payloads
}
