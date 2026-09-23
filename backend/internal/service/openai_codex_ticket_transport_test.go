package service

import (
	"bufio"
	"context"
	"io"
	"net"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCodexTicketConnectPreservesBufferedTunnelBytes(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = listener.Close() }()
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		if _, err := http.ReadRequest(bufio.NewReader(conn)); err != nil {
			return
		}
		_, _ = io.WriteString(conn, "HTTP/1.1 200 Connection Established\r\n\r\nhello")
	}()
	u, err := url.Parse("http://" + listener.Addr().String())
	require.NoError(t, err)
	dialer := &codexTicketHTTPConnectDialer{proxy: u, timeout: time.Second}
	conn, err := dialer.DialContext(context.Background(), "tcp", "example.com:443")
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()
	data := make([]byte, 5)
	_, err = io.ReadFull(conn, data)
	require.NoError(t, err)
	require.Equal(t, "hello", string(data))
}

func TestCodexTicketConnectCancelsStalledProxy(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = listener.Close() }()
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		_, _ = io.Copy(io.Discard, conn)
	}()
	u, err := url.Parse("http://" + listener.Addr().String())
	require.NoError(t, err)
	dialer := &codexTicketHTTPConnectDialer{proxy: u, timeout: time.Second}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	started := time.Now()
	conn, err := dialer.DialContext(ctx, "tcp", "example.com:443")
	require.Error(t, err)
	require.Nil(t, conn)
	require.Less(t, time.Since(started), time.Second)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("cancel did not close proxy connection")
	}
}
