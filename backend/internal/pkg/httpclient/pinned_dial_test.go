package httpclient

import (
	"context"
	"errors"
	"net"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPinnedPublicDialRejectsPrivateResolutionBeforeDial(t *testing.T) {
	dialCalled := false
	dial := newPinnedPublicDialContext(
		func(context.Context, string) ([]net.IPAddr, error) {
			return []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}, nil
		},
		func(context.Context, string, string) (net.Conn, error) {
			dialCalled = true
			return nil, errors.New("must not dial")
		},
	)

	_, err := dial(context.Background(), "tcp", "images.example.test:443")

	require.Error(t, err)
	require.False(t, dialCalled)
}

func TestPinnedPublicDialUsesValidatedResolvedAddress(t *testing.T) {
	var dialedAddress string
	expectedErr := errors.New("dial reached")
	dial := newPinnedPublicDialContext(
		func(_ context.Context, host string) ([]net.IPAddr, error) {
			require.Equal(t, "images.example.test", host)
			return []net.IPAddr{{IP: net.ParseIP("203.0.113.10")}}, nil
		},
		func(_ context.Context, network, address string) (net.Conn, error) {
			require.Equal(t, "tcp", network)
			dialedAddress = address
			return nil, expectedErr
		},
	)

	_, err := dial(context.Background(), "tcp", "images.example.test:443")

	require.ErrorIs(t, err, expectedErr)
	require.Equal(t, "203.0.113.10:443", dialedAddress)
}

func TestPinnedPublicDialRejectsMixedPublicAndPrivateResolution(t *testing.T) {
	dialCalled := false
	dial := newPinnedPublicDialContext(
		func(context.Context, string) ([]net.IPAddr, error) {
			return []net.IPAddr{
				{IP: net.ParseIP("203.0.113.10")},
				{IP: net.ParseIP("10.0.0.1")},
			}, nil
		},
		func(context.Context, string, string) (net.Conn, error) {
			dialCalled = true
			return nil, errors.New("must not dial")
		},
	)

	_, err := dial(context.Background(), "tcp", "images.example.test:443")

	require.Error(t, err)
	require.False(t, dialCalled)
}
