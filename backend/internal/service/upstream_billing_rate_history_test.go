package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func validRateHistorySnapshot(at time.Time) *UpstreamBillingProbeSnapshot {
	return &UpstreamBillingProbeSnapshot{
		Status:     UpstreamBillingProbeStatusOK,
		ReceivedAt: probeTimePtr(at),
		Data: map[string]any{
			"billing_scope":             "token",
			"group_rate_multiplier":     1.0,
			"resolved_rate_multiplier":  1.0,
			"peak_rate_enabled":         false,
			"effective_rate_multiplier": 1.0,
		},
	}
}

func TestBuildUpstreamBillingRateHistoryEventAndCompare(t *testing.T) {
	at := time.Date(2026, 7, 20, 1, 0, 0, 0, time.UTC)
	snapshot := validRateHistorySnapshot(at)
	event, err := BuildUpstreamBillingRateHistoryEvent(snapshot)
	require.NoError(t, err)
	require.Equal(t, at, event.DetectedAt)
	require.True(t, SameUpstreamBillingRateHistoryValues(event, event))

	sameWithFloatNoise := *event
	sameWithFloatNoise.EffectiveRateMultiplier += 1e-12
	require.True(t, SameUpstreamBillingRateHistoryValues(event, &sameWithFloatNoise))

	changed := *event
	changed.GroupRateMultiplier = 2
	changed.EffectiveRateMultiplier = 1
	require.False(t, SameUpstreamBillingRateHistoryValues(event, &changed))
}

func TestBuildUpstreamBillingRateHistoryEventValidatesPeakFields(t *testing.T) {
	snapshot := validRateHistorySnapshot(time.Now())
	snapshot.Data["peak_rate_enabled"] = true
	snapshot.Data["peak_start"] = "09:00"
	snapshot.Data["peak_end"] = "18:00"
	snapshot.Data["timezone"] = "Asia/Shanghai"
	snapshot.Data["peak_rate_multiplier"] = 2.0

	event, err := BuildUpstreamBillingRateHistoryEvent(snapshot)
	require.NoError(t, err)
	require.True(t, event.PeakRateEnabled)
	require.Equal(t, "Asia/Shanghai", *event.PeakTimezone)

	snapshot.Data["peak_end"] = "bad"
	_, err = BuildUpstreamBillingRateHistoryEvent(snapshot)
	require.Error(t, err)
}

type rateHistoryServiceRepo struct {
	AccountRepository
	account   *Account
	events    []UpstreamBillingRateHistoryEvent
	truncated bool
	since     time.Time
	limit     int
}

func (r *rateHistoryServiceRepo) GetByID(context.Context, int64) (*Account, error) {
	return r.account, nil
}

func (r *rateHistoryServiceRepo) ListUpstreamBillingRateHistory(
	_ context.Context,
	_ int64,
	since time.Time,
	limit int,
) ([]UpstreamBillingRateHistoryEvent, bool, error) {
	r.since = since
	r.limit = limit
	return append([]UpstreamBillingRateHistoryEvent(nil), r.events...), r.truncated, nil
}

func TestGetAccountRateHistoryDerivesIntervals(t *testing.T) {
	now := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	first := now.Add(-8 * 24 * time.Hour)
	second := now.Add(-2 * 24 * time.Hour)
	repo := &rateHistoryServiceRepo{
		account: &Account{ID: 7, Platform: PlatformOpenAI, Type: AccountTypeAPIKey},
		events: []UpstreamBillingRateHistoryEvent{
			{ID: 1, DetectedAt: first},
			{ID: 2, DetectedAt: second},
		},
	}
	service := NewUpstreamBillingProbeService(repo, nil, nil)
	service.now = func() time.Time { return now }

	history, err := service.GetAccountRateHistory(context.Background(), 7, 7, 50)
	require.NoError(t, err)
	require.Equal(t, now.Add(-7*24*time.Hour), repo.since)
	require.Equal(t, 50, repo.limit)
	require.Len(t, history.Events, 2)
	require.True(t, history.Events[0].CarriedIn)
	require.Equal(t, second, *history.Events[0].IntervalEnd)
	require.False(t, history.Events[1].CarriedIn)
	require.Nil(t, history.Events[1].IntervalEnd)
}

func TestGetAccountRateHistoryRejectsInvalidRangeAndAccount(t *testing.T) {
	repo := &rateHistoryServiceRepo{account: &Account{ID: 7, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}}
	service := NewUpstreamBillingProbeService(repo, nil, nil)

	_, err := service.GetAccountRateHistory(context.Background(), 7, 8, 50)
	require.ErrorIs(t, err, ErrUpstreamBillingRateHistoryRangeInvalid)

	repo.account.Type = AccountTypeOAuth
	_, err = service.GetAccountRateHistory(context.Background(), 7, 7, 50)
	require.ErrorIs(t, err, ErrUpstreamBillingProbeAccountInvalid)
}
