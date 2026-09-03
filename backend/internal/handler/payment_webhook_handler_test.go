//go:build unit

package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestUnknownOrderWebhookAcksWithSuccess exercises the response contract that
// handleNotify relies on when HandlePaymentNotification returns ErrOrderNotFound:
// we still need to emit a 2xx so the gateway stops retrying. We can't easily
// drive handleNotify end-to-end without mocking the concrete
// *service.PaymentService, so this test locks down the two ingredients the fix
// depends on:
//  1. errors.Is recognises the sentinel through fmt.Errorf %w wrapping (which
//     is how the service layer wraps it with the out_trade_no context).
//  2. writeSuccessResponse produces the acknowledged body handleNotify emits.
//
// If either contract breaks, the "unknown order → 500 loop" regresses.
func TestUnknownOrderWebhookAcksWithSuccess(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// 1) Sentinel recognition through wrapping.
	wrapped := fmt.Errorf("%w: out_trade_no=sub2_missing_42", service.ErrOrderNotFound)
	require.True(t, errors.Is(wrapped, service.ErrOrderNotFound),
		"handleNotify uses errors.Is on the wrapped service error; regression here "+
			"would mean unknown-order webhooks go back to returning 500 and looping forever")

	// A distinct error must NOT match — otherwise a DB failure would be silently
	// swallowed as an ack.
	other := errors.New("lookup order failed: connection refused")
	require.False(t, errors.Is(other, service.ErrOrderNotFound))

	// 2) The success body is what handleNotify emits on the ack path.
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	writeSuccessResponse(c, payment.TypeSePay)
	require.Equal(t, http.StatusOK, w.Code,
		"the gateway requires 2xx to stop retrying; anything else restarts the retry loop")
	require.Equal(t, "success", w.Body.String())
}

func TestWebhookConstants(t *testing.T) {
	t.Run("maxWebhookBodySize is 1MB", func(t *testing.T) {
		assert.Equal(t, int64(1<<20), int64(maxWebhookBodySize))
	})

	t.Run("webhookLogTruncateLen is 200", func(t *testing.T) {
		assert.Equal(t, 200, webhookLogTruncateLen)
	})
}

func TestExtractOutTradeNo(t *testing.T) {
	tests := []struct {
		name    string
		rawBody string
		want    string
	}{
		{
			name:    "sepay json payload",
			rawBody: `{"merchant":"M1","order_invoice_number":"sub2_123","order_status":"COMPLETED"}`,
			want:    "sub2_123",
		},
		{
			name:    "sepay nested json payload",
			rawBody: `{"data":{"order_invoice_number":"sub2_456"}}`,
			want:    "sub2_456",
		},
		{
			name:    "form encoded payload",
			rawBody: "order_invoice_number=sub2_789&order_status=COMPLETED",
			want:    "sub2_789",
		},
		{
			name:    "payload without an order reference",
			rawBody: "{}",
			want:    "",
		},
		{
			name:    "empty body",
			rawBody: "",
			want:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, extractOutTradeNo(tt.rawBody))
		})
	}
}

func TestVerifyNotificationWithProvidersReturnsMatchedProvider(t *testing.T) {
	firstErr := errors.New("wrong provider")
	providers := []payment.Provider{
		webhookHandlerProviderStub{
			key:       payment.TypeSePayNapas,
			verifyErr: firstErr,
		},
		webhookHandlerProviderStub{
			key: payment.TypeSePayNapas,
			notification: &payment.PaymentNotification{
				OrderID: "sub2_42",
				TradeNo: "trade-42",
				Status:  payment.NotificationStatusSuccess,
			},
		},
	}

	providerKey, notification, err := verifyNotificationWithProviders(context.Background(), providers, "{}", map[string]string{"wechatpay-signature": "sig"})
	require.NoError(t, err)
	require.Equal(t, payment.TypeSePayNapas, providerKey)
	require.NotNil(t, notification)
	require.Equal(t, "sub2_42", notification.OrderID)
}

func TestVerifyNotificationWithProvidersFailsWhenAllProvidersReject(t *testing.T) {
	providers := []payment.Provider{
		webhookHandlerProviderStub{
			key:       payment.TypeSePayNapas,
			verifyErr: errors.New("verify failed a"),
		},
		webhookHandlerProviderStub{
			key:       payment.TypeSePayNapas,
			verifyErr: errors.New("verify failed b"),
		},
	}

	_, _, err := verifyNotificationWithProviders(context.Background(), providers, "{}", nil)
	require.Error(t, err)
}

type webhookHandlerProviderStub struct {
	key          string
	notification *payment.PaymentNotification
	verifyErr    error
}

func (p webhookHandlerProviderStub) Name() string        { return p.key }
func (p webhookHandlerProviderStub) ProviderKey() string { return p.key }
func (p webhookHandlerProviderStub) SupportedTypes() []payment.PaymentType {
	return []payment.PaymentType{payment.PaymentType(p.key)}
}
func (p webhookHandlerProviderStub) CreatePayment(context.Context, payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	panic("unexpected call")
}
func (p webhookHandlerProviderStub) QueryOrder(context.Context, string) (*payment.QueryOrderResponse, error) {
	panic("unexpected call")
}
func (p webhookHandlerProviderStub) VerifyNotification(context.Context, string, map[string]string) (*payment.PaymentNotification, error) {
	if p.verifyErr != nil {
		return nil, p.verifyErr
	}
	return p.notification, nil
}
