package repository

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
	"testing"
)

type socks5AuthRecord struct {
	Username string
	Password string
	Target   string
}

type socks5TestServer struct {
	listener net.Listener
	mu       sync.Mutex
	records  []socks5AuthRecord
}

func newSOCKS5TestServer(t *testing.T) *socks5TestServer {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen socks5 test server: %v", err)
	}
	srv := &socks5TestServer{listener: ln}
	go srv.serve(t)
	t.Cleanup(func() {
		_ = ln.Close()
	})
	return srv
}

func (s *socks5TestServer) Addr() string {
	if s == nil || s.listener == nil {
		return ""
	}
	return s.listener.Addr().String()
}

func (s *socks5TestServer) LastRecord() (socks5AuthRecord, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.records) == 0 {
		return socks5AuthRecord{}, false
	}
	return s.records[len(s.records)-1], true
}

func (s *socks5TestServer) serve(t *testing.T) {
	t.Helper()
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}
		go s.handleConn(t, conn)
	}
}

func (s *socks5TestServer) handleConn(t *testing.T, clientConn net.Conn) {
	t.Helper()
	defer func() { _ = clientConn.Close() }()

	methodsHeader := make([]byte, 2)
	if _, err := io.ReadFull(clientConn, methodsHeader); err != nil {
		return
	}
	if methodsHeader[0] != 0x05 {
		return
	}
	methods := make([]byte, int(methodsHeader[1]))
	if _, err := io.ReadFull(clientConn, methods); err != nil {
		return
	}
	if _, err := clientConn.Write([]byte{0x05, 0x02}); err != nil {
		return
	}

	authHeader := make([]byte, 2)
	if _, err := io.ReadFull(clientConn, authHeader); err != nil {
		return
	}
	if authHeader[0] != 0x01 {
		return
	}
	usernameBytes := make([]byte, int(authHeader[1]))
	if _, err := io.ReadFull(clientConn, usernameBytes); err != nil {
		return
	}
	passwordLen := make([]byte, 1)
	if _, err := io.ReadFull(clientConn, passwordLen); err != nil {
		return
	}
	passwordBytes := make([]byte, int(passwordLen[0]))
	if _, err := io.ReadFull(clientConn, passwordBytes); err != nil {
		return
	}
	if _, err := clientConn.Write([]byte{0x01, 0x00}); err != nil {
		return
	}

	requestHeader := make([]byte, 4)
	if _, err := io.ReadFull(clientConn, requestHeader); err != nil {
		return
	}
	if requestHeader[0] != 0x05 || requestHeader[1] != 0x01 {
		return
	}

	target, err := readSOCKS5Address(clientConn, requestHeader[3])
	if err != nil {
		return
	}
	portBytes := make([]byte, 2)
	if _, err := io.ReadFull(clientConn, portBytes); err != nil {
		return
	}
	targetAddr := fmt.Sprintf("%s:%d", target, binary.BigEndian.Uint16(portBytes))

	targetConn, err := (&net.Dialer{}).DialContext(context.Background(), "tcp", targetAddr)
	if err != nil {
		_, _ = clientConn.Write([]byte{0x05, 0x05, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		return
	}
	defer func() { _ = targetConn.Close() }()

	s.mu.Lock()
	s.records = append(s.records, socks5AuthRecord{
		Username: string(usernameBytes),
		Password: string(passwordBytes),
		Target:   targetAddr,
	})
	s.mu.Unlock()

	if _, err := clientConn.Write([]byte{0x05, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0}); err != nil {
		return
	}

	copyDone := make(chan struct{}, 2)
	go func() {
		_, _ = io.Copy(targetConn, clientConn)
		copyDone <- struct{}{}
	}()
	go func() {
		_, _ = io.Copy(clientConn, targetConn)
		copyDone <- struct{}{}
	}()
	<-copyDone
}

func readSOCKS5Address(r io.Reader, atyp byte) (string, error) {
	switch atyp {
	case 0x01:
		buf := make([]byte, 4)
		if _, err := io.ReadFull(r, buf); err != nil {
			return "", err
		}
		return net.IP(buf).String(), nil
	case 0x03:
		length := make([]byte, 1)
		if _, err := io.ReadFull(r, length); err != nil {
			return "", err
		}
		buf := make([]byte, int(length[0]))
		if _, err := io.ReadFull(r, buf); err != nil {
			return "", err
		}
		return string(buf), nil
	case 0x04:
		buf := make([]byte, 16)
		if _, err := io.ReadFull(r, buf); err != nil {
			return "", err
		}
		return net.IP(buf).String(), nil
	default:
		return "", fmt.Errorf("unsupported atyp %d", atyp)
	}
}
