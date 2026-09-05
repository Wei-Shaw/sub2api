package tlsfingerprint

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestChromeDialerEnforcesTLSHandshakeTimeout(t *testing.T) {
	client, server := net.Pipe()
	t.Cleanup(func() { _ = server.Close() })
	dialer := NewChromeDialer(nil, func(context.Context, string, string) (net.Conn, error) {
		return client, nil
	}, 20*time.Millisecond)

	_, err := dialer.DialTLSContext(context.Background(), "tcp", "chatgpt.com:443", nil)
	require.Error(t, err)
	require.True(t, errors.Is(err, context.DeadlineExceeded), "expected deadline error, got %v", err)
}
