package admin

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func enrichOrdersWithDeviceCode(ctx context.Context, paymentService *service.PaymentService, orders []*AdminPaymentOrderResult) []*AdminPaymentOrderResult {
	if len(orders) == 0 || paymentService == nil {
		return orders
	}

	userIDSet := make(map[int64]struct{})
	for _, o := range orders {
		if o != nil && o.UserID > 0 {
			userIDSet[o.UserID] = struct{}{}
		}
	}

	userIDs := make([]int64, 0, len(userIDSet))
	for id := range userIDSet {
		userIDs = append(userIDs, id)
	}

	deviceCodes := paymentService.GetDeviceCodesByUserIDs(ctx, userIDs)
	for _, o := range orders {
		if o != nil {
			o.DeviceCode = deviceCodes[o.UserID]
		}
	}
	return orders
}
