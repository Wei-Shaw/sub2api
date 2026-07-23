//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSessionBindingHashGroupsIPv6By64(t *testing.T) {
	first := (&SessionBinding{IP: "2001:db8:1234:5678::1", UserAgent: "test"}).Hash()
	second := (&SessionBinding{IP: "2001:db8:1234:5678::2", UserAgent: "test"}).Hash()
	otherPrefix := (&SessionBinding{IP: "2001:db8:1234:5679::1", UserAgent: "test"}).Hash()

	require.Equal(t, first, second)
	require.NotEqual(t, first, otherPrefix)
}

func TestSessionBindingHashKeepsIPv4Exact(t *testing.T) {
	first := (&SessionBinding{IP: "192.0.2.1", UserAgent: "test"}).Hash()
	second := (&SessionBinding{IP: "192.0.2.2", UserAgent: "test"}).Hash()
	require.NotEqual(t, first, second)
}
