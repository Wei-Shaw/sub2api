package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type simpleModeRateLimitLoaderStub struct {
	data *APIKeyRateLimitData
	err  error
}

func (s *simpleModeRateLimitLoaderStub) GetRateLimitData(context.Context, int64) (*APIKeyRateLimitData, error) {
	return s.data, s.err
}

func TestCheckBillingEligibilitySimpleModeKeyRateLimitsAreOptInAndDBAuthoritative(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name      string
		enabled   bool
		data      *APIKeyRateLimitData
		loaderErr error
		wantErr   error
	}{
		{
			name:    "disabled preserves simple mode bypass",
			enabled: false,
			data:    &APIKeyRateLimitData{Usage5h: 100},
		},
		{
			name:    "enabled allows below all windows",
			enabled: true,
			data:    &APIKeyRateLimitData{Usage5h: 9, Usage1d: 19, Usage7d: 29, Window5hStart: &now, Window1dStart: &now, Window7dStart: &now},
		},
		{
			name:    "enabled rejects 5h",
			enabled: true,
			data:    &APIKeyRateLimitData{Usage5h: 10, Window5hStart: &now},
			wantErr: ErrAPIKeyRateLimit5hExceeded,
		},
		{
			name:    "enabled rejects 1d",
			enabled: true,
			data:    &APIKeyRateLimitData{Usage1d: 20, Window1dStart: &now},
			wantErr: ErrAPIKeyRateLimit1dExceeded,
		},
		{
			name:    "enabled rejects 7d",
			enabled: true,
			data:    &APIKeyRateLimitData{Usage7d: 30, Window7dStart: &now},
			wantErr: ErrAPIKeyRateLimit7dExceeded,
		},
		{
			name:    "expired window resets for eligibility",
			enabled: true,
			data:    &APIKeyRateLimitData{Usage5h: 10, Window5hStart: simplePtrTime(now.Add(-RateLimitWindow5h - time.Minute))},
		},
		{
			name:      "database failure fails closed",
			enabled:   true,
			loaderErr: errors.New("database unavailable"),
			wantErr:   ErrBillingServiceUnavailable,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				RunMode:                       config.RunModeSimple,
				SimpleModeKeyRateLimitEnabled: tt.enabled,
			}
			svc := &BillingCacheService{
				cfg:                   cfg,
				apiKeyRateLimitLoader: &simpleModeRateLimitLoaderStub{data: tt.data, err: tt.loaderErr},
			}
			key := &APIKey{ID: 42, RateLimit5h: 10, RateLimit1d: 20, RateLimit7d: 30}

			err := svc.CheckBillingEligibility(context.Background(), &User{ID: 7, Balance: 0}, key, nil, nil, "")
			if tt.wantErr == nil {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, tt.wantErr)
		})
	}
}

func simplePtrTime(t time.Time) *time.Time {
	return &t
}
