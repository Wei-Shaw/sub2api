//go:build unit

package service

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"testing"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestTransportErrorCooldownPolicy(t *testing.T) {
	for _, accountType := range []string{AccountTypeAPIKey, AccountTypeBedrock, AccountTypeOAuth} {
		for _, tc := range []struct {
			name    string
			extra   map[string]any
			minutes int
		}{
			{"legacy default", nil, 10},
			{"explicit enabled", map[string]any{"transport_error_cooldown_enabled": true}, 10},
			{"disabled", map[string]any{"transport_error_cooldown_enabled": false}, 0},
			{"custom duration", map[string]any{"transport_error_cooldown_minutes": float64(2)}, 2},
			{"disabled with duration", map[string]any{"transport_error_cooldown_enabled": false, "transport_error_cooldown_minutes": float64(2)}, 0},
		} {
			t.Run(accountType+"/"+tc.name, func(t *testing.T) {
				repo := &transportTempUnschedRepoStub{}
				svc := &GatewayService{accountRepo: repo}
				account := &Account{ID: 1, Platform: PlatformAnthropic, Type: accountType, Extra: tc.extra,
					Credentials: map[string]any{"pool_mode": true, "pool_mode_retry_status_codes": []any{float64(503)}}}
				c := newTransportErrorTestGin(t)
				before := time.Now()
				err := svc.handleUpstreamTransportError(context.Background(), c, account, errors.New("connection refused"), OpsUpstreamErrorEvent{})
				var failover *UpstreamFailoverError
				require.ErrorAs(t, err, &failover)
				require.Equal(t, http.StatusBadGateway, failover.StatusCode)
				require.False(t, failover.RetryableOnSameAccount)
				require.True(t, failover.ShouldRetryNextAccount())
				require.False(t, c.Writer.Written())
				events, exists := c.Get(OpsUpstreamErrorsKey)
				require.True(t, exists)
				require.Len(t, events, 1)
				if tc.minutes == 0 {
					require.Zero(t, repo.calls)
				} else {
					require.Equal(t, 1, repo.calls)
					duration := time.Duration(tc.minutes) * time.Minute
					require.False(t, repo.lastUntil.Before(before.Add(duration)))
					require.False(t, repo.lastUntil.After(time.Now().Add(duration)))
				}
			})
		}
	}
}

type transportCooldownSchedulingRepo struct {
	*mockAccountRepoForPlatform
}

func (r *transportCooldownSchedulingRepo) SetTempUnschedulable(_ context.Context, id int64, until time.Time, reason string) error {
	account := r.accountsByID[id]
	account.TempUnschedulableUntil = &until
	account.TempUnschedulableReason = reason
	return nil
}

func TestTransportErrorCooldownSingleAccountSelection(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		t.Run(map[bool]string{true: "enabled", false: "disabled"}[enabled], func(t *testing.T) {
			base := &mockAccountRepoForPlatform{accounts: []Account{{
				ID: 1, Platform: PlatformAnthropic, Type: AccountTypeAPIKey,
				Status: StatusActive, Schedulable: true,
				Extra: map[string]any{"transport_error_cooldown_enabled": enabled},
			}}}
			base.accountsByID = map[int64]*Account{1: &base.accounts[0]}
			repo := &transportCooldownSchedulingRepo{base}
			svc := &GatewayService{accountRepo: repo, cache: &mockGatewayCacheForPlatform{}, cfg: testConfig()}
			selectAccount := func() (*Account, error) {
				return svc.selectAccountForModelWithPlatform(context.Background(), nil, "", "claude-test", nil, PlatformAnthropic)
			}
			account, err := selectAccount()
			require.NoError(t, err)
			err = svc.handleUpstreamTransportError(context.Background(), newTransportErrorTestGin(t), account, errors.New("connection refused"), OpsUpstreamErrorEvent{})
			require.Error(t, err)
			selected, err := selectAccount()
			if enabled {
				require.ErrorIs(t, err, ErrNoAvailableAccounts)
				require.Nil(t, selected)
			} else {
				require.NoError(t, err)
				require.Equal(t, account.ID, selected.ID)
			}
		})
	}
}

func TestTransportErrorCooldownDoesNotClearExistingBlock(t *testing.T) {
	until := time.Now().Add(time.Hour)
	account := &Account{ID: 1, Platform: PlatformAnthropic, Status: StatusActive, Schedulable: true,
		Extra: map[string]any{"transport_error_cooldown_enabled": false}, TempUnschedulableUntil: &until}
	repo := &transportTempUnschedRepoStub{}
	svc := &GatewayService{accountRepo: repo}
	_ = svc.handleUpstreamTransportError(context.Background(), newTransportErrorTestGin(t), account, errors.New("connection refused"), OpsUpstreamErrorEvent{})
	require.Zero(t, repo.calls)
	require.Equal(t, &until, account.TempUnschedulableUntil)
	require.False(t, account.IsSchedulable())
}

func TestTransportErrorCooldownAdminValidation(t *testing.T) {
	for _, extra := range []map[string]any{
		{"transport_error_cooldown_enabled": "false"},
		{"transport_error_cooldown_enabled": nil},
		{"transport_error_cooldown_minutes": float64(0)},
		{"transport_error_cooldown_minutes": float64(-1)},
		{"transport_error_cooldown_minutes": float64(1.5)},
		{"transport_error_cooldown_minutes": float64(1441)},
		{"transport_error_cooldown_minutes": "2"},
		{"transport_error_cooldown_minutes": nil},
	} {
		repo := &longContextBillingRepoStub{account: &Account{ID: 1, Platform: PlatformAnthropic, Type: AccountTypeAPIKey}}
		svc := &adminServiceImpl{accountRepo: repo}
		_, err := svc.CreateAccount(context.Background(), &CreateAccountInput{
			Platform: PlatformAnthropic, Type: AccountTypeAPIKey, Extra: extra, SkipDefaultGroupBind: true,
		})
		require.Equal(t, http.StatusBadRequest, infraerrors.Code(err), "create extra=%v", extra)
		require.Nil(t, repo.createdAccount)
		_, err = svc.UpdateAccount(context.Background(), 1, &UpdateAccountInput{Extra: extra})
		require.Equal(t, http.StatusBadRequest, infraerrors.Code(err), "update extra=%v", extra)
		err = svc.UpdateAccountExtra(context.Background(), 1, extra)
		require.Equal(t, http.StatusBadRequest, infraerrors.Code(err), "patch extra=%v", extra)
		require.Zero(t, repo.updateExtraCalls)
		_, err = svc.BulkUpdateAccounts(context.Background(), &BulkUpdateAccountsInput{AccountIDs: []int64{1}, Extra: extra})
		require.Equal(t, http.StatusBadRequest, infraerrors.Code(err), "bulk extra=%v", extra)
		require.Zero(t, repo.bulkUpdateCalls)
	}
}

func TestTransportErrorCooldownStoredValues(t *testing.T) {
	for _, value := range []any{nil, "2", 0, -1, 1441, 1.5, math.NaN(), math.Inf(1), json.Number("bad")} {
		account := &Account{Platform: PlatformAnthropic, Extra: map[string]any{transportErrorCooldownMinutesKey: value}}
		require.Equal(t, 10*time.Minute, account.transportErrorCooldown(), "value=%v", value)
	}
	for _, value := range []any{1, int64(1), float64(1), json.Number("1")} {
		account := &Account{Platform: PlatformAnthropic, Extra: map[string]any{transportErrorCooldownMinutesKey: value}}
		require.NoError(t, ValidateTransportErrorCooldownExtra(account.Extra))
		require.Equal(t, time.Minute, account.transportErrorCooldown())
	}
	account := &Account{Platform: PlatformAnthropic, Extra: map[string]any{
		transportErrorCooldownEnabledKey: "false", transportErrorCooldownMinutesKey: 1440,
	}}
	require.Equal(t, 24*time.Hour, account.transportErrorCooldown(), "malformed boolean must not disable protection")
}

func TestTransportErrorCooldownDoesNotChangeOtherPlatforms(t *testing.T) {
	for _, platform := range []string{PlatformGemini, PlatformOpenAI} {
		account := &Account{ID: 1, Platform: platform, Extra: map[string]any{
			transportErrorCooldownEnabledKey: false, transportErrorCooldownMinutesKey: 2,
		}}
		repo := &transportTempUnschedRepoStub{}
		c := newTransportErrorTestGin(t)
		before := time.Now()
		if platform == PlatformGemini {
			svc := &GeminiMessagesCompatService{accountRepo: repo}
			_ = svc.handleUpstreamTransportError(context.Background(), c, account, errors.New("connection refused"))
		} else {
			svc := &OpenAIGatewayService{accountRepo: repo}
			_ = svc.handleOpenAIUpstreamTransportError(context.Background(), c, account, errors.New("connection refused"), false)
		}
		require.Equal(t, 1, repo.calls, "platform=%s", platform)
		require.WithinDuration(t, before.Add(10*time.Minute), repo.lastUntil, time.Second)
	}
}

func TestTransportErrorCooldownDoesNotDisableHTTPOverload(t *testing.T) {
	repo := &overloadAccountRepoStub{}
	cfg := testConfig()
	cfg.RateLimit.OverloadCooldownMinutes = 5
	svc := NewRateLimitService(repo, nil, cfg, nil, nil)
	account := &Account{ID: 1, Platform: PlatformAnthropic, Type: AccountTypeOAuth,
		Extra: map[string]any{transportErrorCooldownEnabledKey: false}}
	before := time.Now()
	svc.HandleUpstreamError(context.Background(), account, 529, nil, []byte(`{"error":{"message":"overloaded"}}`))
	require.Equal(t, 1, repo.overloadCalls)
	require.WithinDuration(t, before.Add(5*time.Minute), repo.lastOverloadEnd, time.Second)
}

func TestTransportErrorCooldownAdminRoundTrip(t *testing.T) {
	repo := &longContextBillingRepoStub{}
	svc := &adminServiceImpl{accountRepo: repo}
	account, err := svc.CreateAccount(context.Background(), &CreateAccountInput{
		Platform: PlatformAnthropic, Type: AccountTypeAPIKey, SkipDefaultGroupBind: true,
		Extra: map[string]any{transportErrorCooldownEnabledKey: false, transportErrorCooldownMinutesKey: float64(2)},
	})
	require.NoError(t, err)
	require.Zero(t, account.transportErrorCooldown())
	account, err = svc.UpdateAccount(context.Background(), account.ID, &UpdateAccountInput{Extra: map[string]any{"unrelated": true}})
	require.NoError(t, err)
	require.Equal(t, false, account.Extra[transportErrorCooldownEnabledKey])
	require.Equal(t, float64(2), account.Extra[transportErrorCooldownMinutesKey])
	account, err = svc.UpdateAccount(context.Background(), account.ID, &UpdateAccountInput{Extra: map[string]any{transportErrorCooldownEnabledKey: true}})
	require.NoError(t, err)
	require.Equal(t, 2*time.Minute, account.transportErrorCooldown())
}
