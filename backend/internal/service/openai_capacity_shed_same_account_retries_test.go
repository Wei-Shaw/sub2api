package service

import (
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestOpenAICapacityShedSameAccountRetriesSetting(t *testing.T) {
	overloaded := []byte(`{"error":{"type":"server_error","code":"server_is_overloaded","message":"Our servers are currently overloaded. Please try again later."}}`)
	rateLimited := []byte(`{"error":{"type":"rate_limit_error","message":"Rate limit reached"}}`)
	account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth}

	newErr := func(limit int, status int, body []byte) *UpstreamFailoverError {
		svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{MaxCapacityShedSameAccountRetries: limit}}}
		return svc.newOpenAIAccountFailoverError(account, status, http.Header{}, body, "", false, false)
	}

	t.Run("default_keeps_account_setting", func(t *testing.T) {
		err := newErr(0, http.StatusServiceUnavailable, overloaded)
		require.True(t, err.RetryableOnSameAccount)
		require.Zero(t, err.SameAccountRetryMax)
	})

	t.Run("caps_capacity_shed", func(t *testing.T) {
		err := newErr(1, http.StatusServiceUnavailable, overloaded)
		require.True(t, err.RetryableOnSameAccount)
		require.Equal(t, 1, err.SameAccountRetryMax)
	})

	t.Run("other_errors_untouched", func(t *testing.T) {
		require.Zero(t, newErr(1, http.StatusTooManyRequests, rateLimited).SameAccountRetryMax)
		require.Zero(t, newErr(1, http.StatusBadGateway, []byte(`{"error":{"message":"bad gateway"}}`)).SameAccountRetryMax)
	})

	t.Run("nil_service", func(t *testing.T) {
		var svc *OpenAIGatewayService
		require.Zero(t, svc.capacityShedSameAccountRetries())
	})
}
