//go:build linux

package repository

import (
	"context"
	"net"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"
)

// TCP_USER_TIMEOUT in Linux include/uapi/linux/tcp.h.
const testTCPUserTimeoutOpt = 0x12

// A socket dialed by the upstream dialer carries the keep-alive configuration
// and TCP_USER_TIMEOUT, read back from the kernel.
func TestUpstreamDialerSetsKeepAliveAndUserTimeoutOnTheSocket(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })
	accepted := make(chan net.Conn, 1)
	go func() {
		server, err := listener.Accept()
		if err != nil {
			close(accepted)
			return
		}
		accepted <- server
	}()

	conn, err := newUpstreamDialer().DialContext(context.Background(), "tcp", listener.Addr().String())
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	if server, ok := <-accepted; ok {
		t.Cleanup(func() { _ = server.Close() })
	}
	tcpConn, ok := conn.(*net.TCPConn)
	require.True(t, ok, "%T", conn)
	raw, err := tcpConn.SyscallConn()
	require.NoError(t, err)

	got := map[string]int{}
	var readErr error
	require.NoError(t, raw.Control(func(fd uintptr) {
		for _, opt := range []struct {
			name         string
			level, which int
		}{
			{"SO_KEEPALIVE", syscall.SOL_SOCKET, syscall.SO_KEEPALIVE},
			{"TCP_KEEPIDLE", syscall.IPPROTO_TCP, syscall.TCP_KEEPIDLE},
			{"TCP_KEEPINTVL", syscall.IPPROTO_TCP, syscall.TCP_KEEPINTVL},
			{"TCP_KEEPCNT", syscall.IPPROTO_TCP, syscall.TCP_KEEPCNT},
			{"TCP_USER_TIMEOUT", syscall.IPPROTO_TCP, testTCPUserTimeoutOpt},
		} {
			value, err := syscall.GetsockoptInt(int(fd), opt.level, opt.which)
			if err != nil {
				readErr = err
				return
			}
			got[opt.name] = value
		}
	}))
	require.NoError(t, readErr)
	require.Equal(t, map[string]int{
		"SO_KEEPALIVE":     1,
		"TCP_KEEPIDLE":     15,
		"TCP_KEEPINTVL":    5,
		"TCP_KEEPCNT":      3,
		"TCP_USER_TIMEOUT": 30000,
	}, got)
}

// A socket option the kernel refuses must not fail the dial: TCP_USER_TIMEOUT
// does not apply to a UDP socket, so setting it there returns an error.
func TestUpstreamDialerIgnoresUserTimeoutFailure(t *testing.T) {
	packetConn, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = packetConn.Close() })

	conn, err := newUpstreamDialer().DialContext(context.Background(), "udp", packetConn.LocalAddr().String())
	require.NoError(t, err)
	require.NoError(t, conn.Close())
}
