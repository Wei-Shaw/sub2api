package dto

import (
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestBotSalesHistoryMetadataIsAdminOnly(t *testing.T) {
	orderID := int64(1075)
	item := &service.RedeemCode{
		ID: -1000000000326, Code: "BS_1793192965095", Type: service.RedeemTypeBotSalesBalance,
		Value: 110, ExternalOrderItemID: "2898", PaymentOrderID: &orderID,
	}
	admin := RedeemCodeFromServiceAdmin(item)
	require.Equal(t, orderID, *admin.PaymentOrderID)
	require.Equal(t, "2898", admin.ExternalOrderItemID)
	encoded, err := json.Marshal(admin)
	require.NoError(t, err)
	require.Contains(t, string(encoded), `"payment_order_id":1075`)
	require.Contains(t, string(encoded), `"external_order_item_id":"2898"`)

	user, err := json.Marshal(RedeemCodeFromService(item))
	require.NoError(t, err)
	require.NotContains(t, string(user), `payment_order_id`)
	require.NotContains(t, string(user), `external_order_item_id`)
}
