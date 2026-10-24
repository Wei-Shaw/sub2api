//go:build unit

package antigravity

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFetchAvailableModelsFiltersPermissionPlaceholders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `{"models":{"gemini-4-argon-disabled":{},"disabled-flag":{"disabled":true},"internal-flag":{"isInternal":true},"gemini-4-argon-low":{},"gemini-pro-agent":{}}}`)
	}))
	defer server.Close()
	for _, fixed := range []bool{false, true} {
		t.Run(fmt.Sprint(fixed), func(t *testing.T) {
			client := mustNewClient(t, "")
			var response *FetchAvailableModelsResponse
			var raw map[string]any
			var err error
			if fixed {
				response, raw, err = client.FetchAvailableModelsFromURL(context.Background(), server.URL, "token", "project", defaultFetchAvailableModelsBodyLimit)
			} else {
				response, raw, err = client.fetchAvailableModels(context.Background(), []string{server.URL}, "token", "project", defaultFetchAvailableModelsBodyLimit, false)
			}
			require.NoError(t, err)
			require.Len(t, response.Models, 2)
			require.Contains(t, response.Models, "gemini-4-argon-low")
			require.Contains(t, response.Models, "gemini-pro-agent")
			require.Len(t, raw["models"], 2)
		})
	}
}
