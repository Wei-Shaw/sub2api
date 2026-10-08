package admin

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUpdateSubscriptionLimitsRequestSupportsTriStateFields(t *testing.T) {
	var request UpdateSubscriptionLimitsRequest
	require.NoError(t, json.Unmarshal([]byte(`{
		"daily_limit_usd": 10,
		"weekly_limit_usd": null
	}`), &request))

	require.True(t, request.DailyLimitUSD.set)
	require.NotNil(t, request.DailyLimitUSD.value)
	require.Equal(t, 10.0, *request.DailyLimitUSD.value)
	require.True(t, request.WeeklyLimitUSD.set)
	require.Nil(t, request.WeeklyLimitUSD.value)
	require.False(t, request.MonthlyLimitUSD.set)
}
