package handler

import (
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestBuildCheckoutBalancePackagesExposesActualCredits(t *testing.T) {
	packages := buildCheckoutBalancePackages([]service.BalanceRechargePackage{
		{
			ID:            "standard",
			Label:         "Standard",
			AmountLedger:  7.34,
			ActualCredits: 27000000,
			CreditUnit:    "tokens",
			Badge:         "Tiết kiệm 95.4%",
			Popular:       true,
			SortOrder:     10,
		},
	})

	require.Len(t, packages, 1)
	require.Equal(t, int64(27000000), packages[0].ActualCredits)
	require.Equal(t, "tokens", packages[0].CreditUnit)

	body, err := json.Marshal(packages[0])
	require.NoError(t, err)
	require.JSONEq(t, `{
		"id":"standard",
		"label":"Standard",
		"amount_ledger":7.34,
		"actual_credits":27000000,
		"credit_unit":"tokens",
		"badge":"Tiết kiệm 95.4%",
		"popular":true,
		"sort_order":10
	}`, string(body))
}
