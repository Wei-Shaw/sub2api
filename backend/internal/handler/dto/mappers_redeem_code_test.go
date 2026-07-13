package dto

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestRedeemCodeFromServiceIncludesNotesForExtraConcurrencyAdjustment(t *testing.T) {
	got := RedeemCodeFromService(&service.RedeemCode{
		Type:  service.AdjustmentTypeAdminExtraConcurrency,
		Notes: "temporary burst allowance",
	})

	require.NotNil(t, got.Notes)
	require.Equal(t, "temporary burst allowance", *got.Notes)
}
