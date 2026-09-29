//go:build unit

package handler

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// SePay 真实 IPN 把订单嵌在 "order" 下；实例预路由必须读到 order.order_invoice_number，
// 且不能误取 SePay 自己的 order.order_id。
func TestB04ExtractOutTradeNoReadsSePayNestedOrder(t *testing.T) {
	tests := []struct {
		name    string
		rawBody string
		want    string
	}{
		{
			name: "sepay real IPN nests order",
			rawBody: `{"timestamp":1767225600,"notification_type":"ORDER_PAID",` +
				`"order":{"id":"e2c1","order_id":"SP-9","order_invoice_number":"sub2_1","order_status":"CAPTURED"},` +
				`"transaction":{"payment_method":"BANK_TRANSFER"}}`,
			want: "sub2_1",
		},
		{
			name:    "nested order without invoice number does not pick sepay order_id",
			rawBody: `{"order":{"order_id":"SP-9"}}`,
			want:    "",
		},
		{
			name:    "top level order_id still used for nowpayments",
			rawBody: `{"order_id":"sub2_np","payment_status":"finished"}`,
			want:    "sub2_np",
		},
		{
			name:    "non-object order does not lose other candidates",
			rawBody: `{"order":"SP-9","order_id":"sub2_np"}`,
			want:    "sub2_np",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, extractOutTradeNo(tt.rawBody))
		})
	}
}
