package dto

import (
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestAPIKeyConcurrencyLimitJSON(t *testing.T) {
	for _, limit := range []int{0, 8} {
		out := APIKeyFromService(&service.APIKey{ConcurrencyLimit: limit, CurrentConcurrency: 3})
		data, err := json.Marshal(out)
		require.NoError(t, err)
		var fields map[string]any
		require.NoError(t, json.Unmarshal(data, &fields))
		require.Equal(t, float64(limit), fields["concurrency_limit"])
		require.Equal(t, float64(3), fields["current_concurrency"])
	}
}
