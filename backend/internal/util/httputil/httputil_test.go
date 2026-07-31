//go:build unit

package httputil

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsCloudflareGeneratedErrorResponse(t *testing.T) {
	t.Run("cf-error-type identifies Cloudflare generated 403", func(t *testing.T) {
		headers := http.Header{"Cf-Error-Type": []string{"waf"}}
		require.True(t, IsCloudflareGeneratedErrorResponse(http.StatusForbidden, headers, []byte("blocked")))
	})

	t.Run("cf-error-origin identifies Cloudflare generated 403", func(t *testing.T) {
		headers := http.Header{"Cf-Error-Origin": []string{"edge"}}
		require.True(t, IsCloudflareGeneratedErrorResponse(http.StatusForbidden, headers, []byte("blocked")))
	})

	t.Run("challenge marker identifies Cloudflare generated 403", func(t *testing.T) {
		headers := http.Header{"Cf-Mitigated": []string{"challenge"}}
		require.True(t, IsCloudflareGeneratedErrorResponse(http.StatusForbidden, headers, nil))
	})

	t.Run("cf-ray alone does not classify an origin 403 as Cloudflare generated", func(t *testing.T) {
		headers := http.Header{
			"Cf-Ray": []string{"abc123-SJC"},
			"Server": []string{"cloudflare"},
		}
		body := []byte(`{"error":{"message":"workspace access forbidden"}}`)
		require.False(t, IsCloudflareGeneratedErrorResponse(http.StatusForbidden, headers, body))
	})

	t.Run("diagnostic headers do not remap non Cloudflare error statuses", func(t *testing.T) {
		headers := http.Header{"Cf-Error-Type": []string{"waf"}}
		require.False(t, IsCloudflareGeneratedErrorResponse(http.StatusNotFound, headers, nil))
	})
}
