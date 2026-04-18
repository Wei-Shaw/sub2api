package main

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"sync"
)

// peekConn buffers bytes read from the underlying conn so they can be
// replayed to crypto/tls after we've inspected them for fingerprinting.
type peekConn struct {
	net.Conn
	buf     *bytes.Buffer
	drained bool
	mu      sync.Mutex
}

func newPeekConn(c net.Conn) *peekConn {
	return &peekConn{Conn: c, buf: &bytes.Buffer{}}
}

// peek reads n bytes from the underlying conn and records them.
func (p *peekConn) peek(n int) ([]byte, error) {
	chunk := make([]byte, n)
	if _, err := io.ReadFull(p.Conn, chunk); err != nil {
		return nil, err
	}
	p.buf.Write(chunk)
	return chunk, nil
}

func (p *peekConn) Read(b []byte) (int, error) {
	p.mu.Lock()
	if !p.drained && p.buf.Len() > 0 {
		n, _ := p.buf.Read(b)
		if p.buf.Len() == 0 {
			p.drained = true
		}
		p.mu.Unlock()
		return n, nil
	}
	p.mu.Unlock()
	return p.Conn.Read(b)
}

// readTLSHandshakeRecord reads a single TLS record (5-byte header + fragment)
// and returns the raw handshake message bytes (the handshake payload, i.e.
// ClientHello starting with the handshake-type byte 0x01).
func readTLSHandshakeRecord(p *peekConn) ([]byte, error) {
	header, err := p.peek(5)
	if err != nil {
		return nil, err
	}
	if header[0] != 0x16 { // not a handshake record
		return nil, fmt.Errorf("not a tls handshake record: first byte=0x%02x", header[0])
	}
	length := int(header[3])<<8 | int(header[4])
	if length <= 0 || length > 16*1024 {
		return nil, fmt.Errorf("bad handshake record length %d", length)
	}
	fragment, err := p.peek(length)
	if err != nil {
		return nil, err
	}
	return fragment, nil
}
