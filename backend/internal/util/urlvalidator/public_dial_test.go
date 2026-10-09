package urlvalidator

import (
	"context"
	"errors"
	"net"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDialPublicContextRejectsPrivateIPsBeforeDial(t *testing.T) {
	for _, address := range []string{"127.0.0.1:80", "10.0.0.1:443", "[::1]:443", "localhost:80"} {
		called := false
		_, err := DialPublicContext(context.Background(), "tcp", address, func(context.Context, string, string) (net.Conn, error) { called = true; return nil, nil })
		require.Error(t, err)
		require.False(t, called)
	}
}

func TestDialPublicContextUsesValidatedIP(t *testing.T) {
	stop := errors.New("fake dial")
	_, err := DialPublicContext(context.Background(), "tcp", "1.1.1.1:443", func(_ context.Context, network, address string) (net.Conn, error) {
		require.Equal(t, "tcp", network)
		require.Equal(t, "1.1.1.1:443", address)
		return nil, stop
	})
	require.ErrorIs(t, err, stop)
}
