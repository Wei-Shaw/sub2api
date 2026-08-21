package service

import (
	"testing"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

func TestNormalizeBotSalesFulfillmentRequest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   BotSalesFulfillmentRequest
		wantErr string
		wantDlg string
	}{
		{
			name: "normalizes buyer and device fields",
			input: BotSalesFulfillmentRequest{
				ExternalOrderCode:   " order-123 ",
				ExternalOrderItemID: " item-1 ",
				IdempotencyKey:      " bot-sales:order-123:item-1 ",
				Buyer:               BotSalesBuyer{ExternalUserID: " channel:telegram:user:42 ", Email: " Buyer@Example.COM "},
				BalanceAmount:       12.5,
				IssueDeviceCode:     true,
				DeviceCode:          " dlg-abcd-efgh-ijkl ",
			},
			wantDlg: "DLG-ABCD-EFGH-IJKL",
		},
		{
			name: "rejects non-positive balance",
			input: BotSalesFulfillmentRequest{
				ExternalOrderCode:   "order-123",
				ExternalOrderItemID: "item-1",
				IdempotencyKey:      "key-1",
				BalanceAmount:       0,
			},
			wantErr: "BOT_SALES_BALANCE_AMOUNT_INVALID",
		},
		{
			name: "rejects malformed device code",
			input: BotSalesFulfillmentRequest{
				ExternalOrderCode:   "order-123",
				ExternalOrderItemID: "item-1",
				IdempotencyKey:      "key-2",
				BalanceAmount:       1,
				DeviceCode:          "not-a-dlg",
			},
			wantErr: "BOT_SALES_DEVICE_CODE_INVALID",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeBotSalesFulfillmentRequest(tt.input)
			if tt.wantErr != "" {
				if err == nil || infraerrors.Reason(err) != tt.wantErr {
					t.Fatalf("error reason = %q, want %q (err=%v)", infraerrors.Reason(err), tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("normalize request: %v", err)
			}
			if got.ExternalOrderCode != "order-123" || got.ExternalOrderItemID != "item-1" {
				t.Fatalf("normalized order fields = %#v", got)
			}
			if got.Buyer.ExternalUserID != "channel:telegram:user:42" || got.Buyer.Email != "buyer@example.com" {
				t.Fatalf("normalized buyer = %#v", got.Buyer)
			}
			if got.DeviceCode != tt.wantDlg {
				t.Fatalf("device code = %q, want %q", got.DeviceCode, tt.wantDlg)
			}
		})
	}
}
