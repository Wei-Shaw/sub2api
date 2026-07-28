//go:build unit

package handler

import (
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestGrokImageCreateAccountBindingOnlyReleasesExplicitRejections(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusTooManyRequests} {
		require.True(t, grokVideoCreateAccountRebindSafe(&service.UpstreamFailoverError{StatusCode: status}), status)
	}
	for _, status := range []int{0, http.StatusInternalServerError, http.StatusBadGateway, http.StatusGatewayTimeout} {
		require.False(t, grokVideoCreateAccountRebindSafe(&service.UpstreamFailoverError{StatusCode: status}), status)
	}
	// A non-UpstreamFailoverError transport failure never enters the release
	// branch in handleGrokMedia; the persisted image account remains bound.
}
