package admin

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCreateGroupRequestLongContextPricingTriState(t *testing.T) {
	t.Run("omitted keeps service default", func(t *testing.T) {
		var req CreateGroupRequest
		require.NoError(t, json.Unmarshal([]byte(`{"name":"group"}`), &req))
		require.Nil(t, req.LongContextPricingEnabled)
	})

	t.Run("explicit false remains distinguishable", func(t *testing.T) {
		var req CreateGroupRequest
		require.NoError(t, json.Unmarshal([]byte(`{"name":"group","long_context_pricing_enabled":false}`), &req))
		require.NotNil(t, req.LongContextPricingEnabled)
		require.False(t, *req.LongContextPricingEnabled)
	})
}
