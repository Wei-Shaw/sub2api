// capture_fingerprint spins up a local HTTPS+H2 server that records the
// TLS ClientHello and the first HTTP/2 frames sent by the connecting client,
// then replies with a minimal valid Anthropic /v1/messages SSE stream so the
// real Claude Code CLI treats the exchange as successful.
//
// Usage:
//
//	go run ./tools/capture_fingerprint -addr 127.0.0.1:8443 -out fp.json
//
// Then, from another shell, point Claude Code at it:
//
//	export ANTHROPIC_BASE_URL="https://localhost:8443"
//	export ANTHROPIC_API_KEY="sk-ant-capture-dummy"
//	export NODE_TLS_REJECT_UNAUTHORIZED=0
//	echo 'hi' | claude -p --model claude-sonnet-4-5 >/dev/null
//
// After the CLI call returns, the server prints the captured fingerprint as
// JSON to stdout and (optionally) writes it to the -out file.
package main

import (
	"context"
	ctls "crypto/tls"
	"encoding/hex"
	"errors"
	"flag"
	"log"
	"net"
	"time"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8443", "listen address")
	outFile := flag.String("out", "", "optional: write captured fingerprint JSON to this file")
	flag.Parse()

	cert, err := generateSelfSignedCert()
	if err != nil {
		log.Fatalf("generate cert: %v", err)
	}

	tlsCfg := &ctls.Config{
		Certificates: []ctls.Certificate{cert},
		NextProtos:   []string{"h2", "http/1.1"},
		MinVersion:   ctls.VersionTLS12,
	}

	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatalf("listen %s: %v", *addr, err)
	}
	defer ln.Close()

	logStartupBanner(*addr)
	acceptLoop(ln, tlsCfg, *outFile)
}

func logStartupBanner(addr string) {
	log.Printf("capture server listening on https://%s", addr)
	log.Printf("run in another shell:")
	log.Printf("  NODE_TLS_REJECT_UNAUTHORIZED=0 \\")
	log.Printf("  ANTHROPIC_BASE_URL=https://localhost:%s \\", portOf(addr))
	log.Printf("  ANTHROPIC_API_KEY=sk-ant-capture-dummy \\")
	log.Printf("  claude -p --model claude-sonnet-4-5 'hi'")
	log.Printf("")
}

func acceptLoop(ln net.Listener, tlsCfg *ctls.Config, outFile string) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}
			log.Printf("accept: %v", err)
			continue
		}
		go handleConn(conn, tlsCfg, outFile)
	}
}

func handleConn(raw net.Conn, tlsCfg *ctls.Config, outFile string) {
	defer raw.Close()
	_ = raw.SetDeadline(time.Now().Add(30 * time.Second))

	capture := &Capture{
		CapturedAt: time.Now().UTC().Format(time.RFC3339),
		RemoteAddr: raw.RemoteAddr().String(),
	}
	// Always dump whatever we captured, even if handshake / h2 fails later.
	defer func() { printCapture(capture, outFile) }()

	tlsConn, ok := sniffAndHandshake(raw, tlsCfg, capture)
	if !ok {
		return
	}
	dispatchALPN(tlsConn, capture)
}

// sniffAndHandshake peeks the ClientHello for fingerprinting, then completes
// the TLS handshake by replaying the buffered bytes back to crypto/tls.
func sniffAndHandshake(raw net.Conn, tlsCfg *ctls.Config, capture *Capture) (*ctls.Conn, bool) {
	// Step 1: sniff the first ClientHello record so we can parse
	// extensions / cipher order before the TLS library eats them.
	peeker := newPeekConn(raw)
	helloBytes, err := readTLSHandshakeRecord(peeker)
	if err != nil {
		log.Printf("read ClientHello: %v", err)
		return nil, false
	}
	capture.ClientHelloRaw = hex.EncodeToString(helloBytes)
	if err := parseClientHello(helloBytes, capture); err != nil {
		log.Printf("parse ClientHello: %v", err)
	}

	// Step 2: do the actual TLS handshake using the sniffed bytes
	// replayed back to crypto/tls via our peekConn buffer.
	tlsConn := ctls.Server(peeker, tlsCfg)
	hsCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := tlsConn.HandshakeContext(hsCtx); err != nil {
		log.Printf("tls handshake: %v", err)
		return nil, false
	}
	state := tlsConn.ConnectionState()
	capture.NegotiatedProto = state.NegotiatedProtocol
	capture.TLSVersion = tlsVersionName(state.Version)
	capture.ServerName = state.ServerName
	return tlsConn, true
}

func dispatchALPN(tlsConn *ctls.Conn, capture *Capture) {
	switch tlsConn.ConnectionState().NegotiatedProtocol {
	case "h2":
		if err := serveH2(tlsConn, capture); err != nil {
			log.Printf("h2: %v", err)
		}
	case "http/1.1", "":
		if err := serveH1(tlsConn, capture); err != nil {
			log.Printf("h1: %v", err)
		}
	default:
		log.Printf("unknown ALPN: %q", tlsConn.ConnectionState().NegotiatedProtocol)
	}
}
