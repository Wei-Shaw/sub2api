//go:build unit

package service

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type configuredPoolCooldownRepo struct {
	AccountRepository
	account   *Account
	getErr    error
	cooldowns int
}

func (r *configuredPoolCooldownRepo) GetByID(context.Context, int64) (*Account, error) {
	return r.account, r.getErr
}

func (r *configuredPoolCooldownRepo) SetTempUnschedulable(context.Context, int64, time.Time, string) error {
	r.cooldowns++
	return nil
}

func TestTempUnscheduleRetryableErrorPreservesConfiguredPoolStatus(t *testing.T) {
	for _, status := range []int{http.StatusBadRequest, http.StatusBadGateway} {
		for _, tc := range []struct {
			name          string
			pool          bool
			codes         any
			getErr        error
			wantCooldowns int
		}{
			{name: "explicit status", pool: true, codes: []any{float64(status)}, wantCooldowns: 0},
			{name: "default codes", pool: true, wantCooldowns: 1},
			{name: "empty codes", pool: true, codes: []any{}, wantCooldowns: 1},
			{name: "other status", pool: true, codes: []any{float64(503)}, wantCooldowns: 1},
			{name: "non pool", codes: []any{float64(status)}, wantCooldowns: 1},
			{name: "lookup failed", pool: true, codes: []any{float64(status)}, getErr: errors.New("unavailable"), wantCooldowns: 1},
		} {
			t.Run(tc.name+"/"+http.StatusText(status), func(t *testing.T) {
				repo := &configuredPoolCooldownRepo{account: &Account{ID: 1, Platform: PlatformAnthropic, Type: AccountTypeAPIKey,
					Credentials: map[string]any{"pool_mode": tc.pool, "pool_mode_retry_status_codes": tc.codes}}, getErr: tc.getErr}
				(&GatewayService{accountRepo: repo}).TempUnscheduleRetryableError(context.Background(), 1,
					&UpstreamFailoverError{StatusCode: status, RetryableOnSameAccount: true})
				require.Equal(t, tc.wantCooldowns, repo.cooldowns)
			})
		}
	}
}
