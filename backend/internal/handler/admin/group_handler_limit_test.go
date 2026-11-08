package admin

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUpdateGroupRequestLimitFieldsTriState(t *testing.T) {
	t.Run("omitted means unchanged", func(t *testing.T) {
		var req UpdateGroupRequest
		require.NoError(t, json.Unmarshal([]byte(`{}`), &req))
		require.Nil(t, req.DailyLimitUSD.ToServiceInput())
		require.Nil(t, req.WeeklyLimitUSD.ToServiceInput())
		require.Nil(t, req.MonthlyLimitUSD.ToServiceInput())
	})

	t.Run("null means unlimited", func(t *testing.T) {
		var req UpdateGroupRequest
		require.NoError(t, json.Unmarshal([]byte(`{"daily_limit_usd":null}`), &req))
		limit := req.DailyLimitUSD.ToServiceInput()
		require.NotNil(t, limit)
		require.Negative(t, *limit)
	})

	t.Run("number is preserved", func(t *testing.T) {
		var req UpdateGroupRequest
		require.NoError(t, json.Unmarshal([]byte(`{"weekly_limit_usd":0,"monthly_limit_usd":42.5}`), &req))
		require.Equal(t, 0.0, *req.WeeklyLimitUSD.ToServiceInput())
		require.Equal(t, 42.5, *req.MonthlyLimitUSD.ToServiceInput())
	})
}
