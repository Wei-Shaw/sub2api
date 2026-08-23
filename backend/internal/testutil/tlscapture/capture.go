//go:build tls_capture

// Package tlscapture provides loopback-only ClientHello capture helpers for
// manually enabled TLS fingerprint tests.
package tlscapture

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"testing"
	"time"
)

type ClientHello struct {
	CipherSuites []uint16
	Extensions   []uint16
}

type captureResult struct {
	hello ClientHello
	err   error
}

func Capture(
	t testing.TB,
	scheme string,
	send func(context.Context, string) error,
) ClientHello {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen for ClientHello: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	result := make(chan captureResult, 1)
	go func() {
		conn, acceptErr := listener.Accept()
		if acceptErr != nil {
			result <- captureResult{err: acceptErr}
			return
		}
		defer func() { _ = conn.Close() }()
		hello, captureErr := readClientHello(conn)
		result <- captureResult{hello: hello, err: captureErr}
	}()

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	sendErr := send(ctx, scheme+"://"+listener.Addr().String()+"/capture")
	if sendErr == nil {
		t.Fatal("capture server closes before completing the TLS handshake; expected request error")
	}

	select {
	case captured := <-result:
		if captured.err != nil {
			t.Fatalf("capture ClientHello: %v", captured.err)
		}
		if len(captured.hello.CipherSuites) == 0 || len(captured.hello.Extensions) == 0 {
			t.Fatalf("captured empty ClientHello: %#v", captured.hello)
		}
		return captured.hello
	case <-ctx.Done():
		t.Fatalf("timed out waiting for ClientHello capture: %v", ctx.Err())
		return ClientHello{}
	}
}

func readClientHello(conn net.Conn) (ClientHello, error) {
	var handshake []byte
	expectedLength := -1
	for expectedLength < 0 || len(handshake) < expectedLength {
		var recordHeader [5]byte
		if _, err := io.ReadFull(conn, recordHeader[:]); err != nil {
			return ClientHello{}, fmt.Errorf("read TLS record header: %w", err)
		}
		if recordHeader[0] != 22 {
			return ClientHello{}, fmt.Errorf("unexpected TLS record type %d", recordHeader[0])
		}
		record := make([]byte, int(binary.BigEndian.Uint16(recordHeader[3:5])))
		if _, err := io.ReadFull(conn, record); err != nil {
			return ClientHello{}, fmt.Errorf("read TLS handshake record: %w", err)
		}
		handshake = append(handshake, record...)
		if len(handshake) >= 4 && expectedLength < 0 {
			if handshake[0] != 1 {
				return ClientHello{}, fmt.Errorf("unexpected TLS handshake type %d", handshake[0])
			}
			expectedLength = 4 + int(handshake[1])<<16 + int(handshake[2])<<8 + int(handshake[3])
		}
	}
	return parseClientHello(handshake[4:expectedLength])
}

func parseClientHello(body []byte) (ClientHello, error) {
	const fixedPrefixLength = 2 + 32
	if len(body) < fixedPrefixLength+1 {
		return ClientHello{}, errors.New("truncated ClientHello fixed fields")
	}
	cursor := fixedPrefixLength

	sessionIDLength := int(body[cursor])
	cursor++
	if cursor+sessionIDLength+2 > len(body) {
		return ClientHello{}, errors.New("truncated ClientHello session ID")
	}
	cursor += sessionIDLength

	cipherBytes := int(binary.BigEndian.Uint16(body[cursor : cursor+2]))
	cursor += 2
	if cipherBytes%2 != 0 || cursor+cipherBytes+1 > len(body) {
		return ClientHello{}, errors.New("invalid ClientHello cipher suites")
	}
	cipherSuites := make([]uint16, 0, cipherBytes/2)
	for end := cursor + cipherBytes; cursor < end; cursor += 2 {
		cipherSuites = append(cipherSuites, binary.BigEndian.Uint16(body[cursor:cursor+2]))
	}

	compressionLength := int(body[cursor])
	cursor++
	if cursor+compressionLength+2 > len(body) {
		return ClientHello{}, errors.New("truncated ClientHello compression methods")
	}
	cursor += compressionLength

	extensionBytes := int(binary.BigEndian.Uint16(body[cursor : cursor+2]))
	cursor += 2
	if cursor+extensionBytes > len(body) {
		return ClientHello{}, errors.New("truncated ClientHello extensions")
	}
	extensionEnd := cursor + extensionBytes
	extensions := make([]uint16, 0, 16)
	for cursor < extensionEnd {
		if cursor+4 > extensionEnd {
			return ClientHello{}, errors.New("truncated ClientHello extension header")
		}
		extensionType := binary.BigEndian.Uint16(body[cursor : cursor+2])
		extensionLength := int(binary.BigEndian.Uint16(body[cursor+2 : cursor+4]))
		cursor += 4
		if cursor+extensionLength > extensionEnd {
			return ClientHello{}, fmt.Errorf("truncated ClientHello extension %d", extensionType)
		}
		extensions = append(extensions, extensionType)
		cursor += extensionLength
	}

	return ClientHello{CipherSuites: cipherSuites, Extensions: extensions}, nil
}

func IsGREASE(value uint16) bool {
	return value&0x0f0f == 0x0a0a && value>>8 == value&0xff
}
