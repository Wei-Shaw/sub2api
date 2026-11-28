//go:build unit

package service

import (
	"context"
	"errors"
	"net"
	"os"
	"syscall"
	"testing"
)

// TestClassifyUpstreamTransportError covers proxy authentication and network
// reachability failures in both typed errors and transport error messages.
func TestClassifyUpstreamTransportError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want upstreamTransportErrorClass
	}{
		{"socks5 proxy credential rejected", errors.New(`socks connect: username/password authentication failed`), upstreamTransportProxyAuth},
		{"HTTP CONNECT proxy credential rejected", errors.New(`Proxy Authentication Required`), upstreamTransportProxyAuth},
		{"proxy connection refused", errors.New(`proxyconnect tcp: dial tcp 1.2.3.4:1080: connect: connection refused`), upstreamTransportReachability},
		{"no route to host", errors.New(`dial tcp 1.2.3.4:443: connect: no route to host`), upstreamTransportReachability},
		{"dns resolution failure", errors.New(`dial tcp: lookup proxy.example.com: no such host`), upstreamTransportReachability},
		{"network unreachable", errors.New(`dial tcp 1.2.3.4:443: connect: network is unreachable`), upstreamTransportReachability},

		// Transient — a temporary blip. Fail over, but do NOT evict the account.
		{"client timeout", errors.New(`Post "https://chatgpt.com/...": context deadline exceeded (Client.Timeout exceeded while awaiting headers)`), upstreamTransportTransient},
		{"i/o timeout", errors.New(`dial tcp 1.2.3.4:443: i/o timeout`), upstreamTransportTransient},
		{"connection reset by peer", errors.New(`read tcp 10.0.0.1:5->2.2.2.2:443: read: connection reset by peer`), upstreamTransportTransient},
		{"unexpected eof", errors.New(`unexpected EOF`), upstreamTransportTransient},
		{"broken pipe", errors.New(`write tcp 10.0.0.1:5->2.2.2.2:443: write: broken pipe`), upstreamTransportTransient},

		{"nil error", nil, upstreamTransportTransient},

		// ── Typed-error cases ──────────────────────────────────────────────
		// ECONNREFUSED wrapped in the canonical net.OpError shape Go produces.
		{
			"ECONNREFUSED via net.OpError",
			&net.OpError{
				Op:  "dial",
				Net: "tcp",
				Err: &os.SyscallError{Syscall: "connect", Err: syscall.ECONNREFUSED},
			},
			upstreamTransportReachability,
		},
		// Bare syscall error (errors.Is traverses the chain).
		{"ECONNREFUSED bare", syscall.ECONNREFUSED, upstreamTransportReachability},
		{"EHOSTUNREACH bare", syscall.EHOSTUNREACH, upstreamTransportReachability},
		{"ENETUNREACH bare", syscall.ENETUNREACH, upstreamTransportReachability},

		// *net.DNSError with IsNotFound — permanent DNS lookup failure.
		{
			"DNS not found (IsNotFound=true)",
			&net.DNSError{Err: "no such host", Name: "proxy.example.com", IsNotFound: true},
			upstreamTransportReachability,
		},
		// *net.DNSError with IsNotFound=false — transient DNS timeout (not persistent).
		{
			"DNS timeout (IsNotFound=false)",
			&net.DNSError{Err: "i/o timeout", Name: "proxy.example.com", IsTimeout: true},
			upstreamTransportTransient,
		},

		// context.Canceled — client gone; NOT classified as persistent.
		{"context.Canceled", context.Canceled, upstreamTransportTransient},
		// context.DeadlineExceeded — slow upstream; NOT persistent.
		{"context.DeadlineExceeded", context.DeadlineExceeded, upstreamTransportTransient},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyUpstreamTransportError(tc.err)
			if got != tc.want {
				t.Fatalf("classifyUpstreamTransportError(%q) = %v, want %v", errString(tc.err), got, tc.want)
			}
		})
	}
}

func errString(err error) string {
	if err == nil {
		return "<nil>"
	}
	return err.Error()
}
