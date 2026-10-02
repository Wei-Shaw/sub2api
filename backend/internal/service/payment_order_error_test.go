//go:build unit

package service

import (
	"errors"
	"testing"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClassifyCreatePaymentErrorKeepsProviderDetail(t *testing.T) {
	t.Parallel()

	// The provider already knows which parameter the gateway rejected. Rewrapping
	// it produces a bare "payment gateway error" and throws that away, which
	// leaves the only diagnosis path as guesswork.
	providerErr := infraerrors.ServiceUnavailable("PAYMENT_GATEWAY_ERROR",
		"nowpayments create payment: upstream returned 500: INTERNAL_ERROR").
		WithMetadata(map[string]string{"price_currency": "vnd", "price_amount": "250000"})

	err := classifyCreatePaymentError(CreateOrderRequest{}, "nowpayments", providerErr)
	require.Error(t, err)

	var appErr *infraerrors.ApplicationError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, "vnd", appErr.Metadata["price_currency"])
	assert.Equal(t, "250000", appErr.Metadata["price_amount"])
	assert.Contains(t, appErr.Message, "INTERNAL_ERROR")
}

func TestClassifyCreatePaymentErrorWrapsPlainErrors(t *testing.T) {
	t.Parallel()

	err := classifyCreatePaymentError(CreateOrderRequest{}, "gpmpay", errors.New("dial tcp: timeout"))
	require.Error(t, err)

	var appErr *infraerrors.ApplicationError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, "PAYMENT_GATEWAY_ERROR", appErr.Reason)
	assert.Contains(t, appErr.Message, "dial tcp: timeout")
	assert.Equal(t, "gpmpay", appErr.Metadata["provider_key"])

	assert.NoError(t, classifyCreatePaymentError(CreateOrderRequest{}, "gpmpay", nil))
}

func TestBuildPaymentNotifyURL(t *testing.T) {
	t.Parallel()

	// The origin comes from the already-validated return URL, not the Host
	// header: behind a reverse proxy that header can be an internal name the
	// gateway cannot reach, and for some gateways the IPN is the only proof a
	// payment happened.
	got, err := buildPaymentNotifyURL("https://panel.example.com/payment/result", "nowpayments")
	require.NoError(t, err)
	assert.Equal(t, "https://panel.example.com/api/v1/payment/webhook/nowpayments", got)

	// The path and query of the return URL must not leak into the webhook URL.
	got, err = buildPaymentNotifyURL("https://panel.example.com:8443/payment/result?order_id=7", "gpmpay")
	require.NoError(t, err)
	assert.Equal(t, "https://panel.example.com:8443/api/v1/payment/webhook/gpmpay", got)

	// No return URL means no derivable origin. That is not an error here — only
	// the gateways that need a callback get to refuse, and refusing for all of
	// them would take the others down with it.
	got, err = buildPaymentNotifyURL("", "nowpayments")
	require.NoError(t, err)
	assert.Empty(t, got)

	_, err = buildPaymentNotifyURL("/payment/result", "nowpayments")
	require.Error(t, err)
	assert.Equal(t, "INVALID_RETURN_URL", infraerrors.Reason(err))
}
