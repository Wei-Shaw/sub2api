//go:build unit

package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type openAI403TempRecorder struct {
	*rateLimitAccountRepoStub
	until time.Time
}

func (r *openAI403TempRecorder) SetTempUnschedulable(ctx context.Context, id int64, until time.Time, reason string) error {
	r.until = until
	return r.rateLimitAccountRepoStub.SetTempUnschedulable(ctx, id, until, reason)
}

type openAI403WindowRecorder struct {
	*countingOpenAI403CounterCache
	windows []int
}

func (r *openAI403WindowRecorder) IncrementOpenAI403Count(ctx context.Context, accountID int64, window int) (int64, error) {
	r.windows = append(r.windows, window)
	return r.countingOpenAI403CounterCache.IncrementOpenAI403Count(ctx, accountID, window)
}

func setOpenAI403TestSettings(t *testing.T, h *openAI403TestHarness, settings OpenAI403CooldownSettings) {
	t.Helper()
	repo := newMockSettingRepo()
	data, err := json.Marshal(settings)
	require.NoError(t, err)
	repo.data[SettingKeyOpenAI403CooldownSettings] = string(data)
	h.svc.SetSettingService(NewSettingService(repo, &config.Config{}))
}

func TestGetOpenAI403CooldownSettings_DefaultsWhenNotSet(t *testing.T) {
	svc := NewSettingService(newMockSettingRepo(), &config.Config{})

	settings, err := svc.GetOpenAI403CooldownSettings(context.Background())
	require.NoError(t, err)
	// MUTATION-SANITY: changing any OpenAI 403 default constant makes these assertions fail.
	require.True(t, settings.Enabled)
	require.Equal(t, 10, settings.CooldownMinutes)
	require.Equal(t, 3, settings.DisableThreshold)
	require.Equal(t, 180, settings.WindowMinutes)
}

func TestGetOpenAI403CooldownSettings_ClampsOutOfRange(t *testing.T) {
	repo := newMockSettingRepo()
	repo.data[SettingKeyOpenAI403CooldownSettings] = `{"enabled":true,"cooldown_minutes":99999,"disable_threshold":0,"window_minutes":-5}`
	svc := NewSettingService(repo, &config.Config{})

	settings, err := svc.GetOpenAI403CooldownSettings(context.Background())
	require.NoError(t, err)
	// MUTATION-SANITY: removing the read-side clamps returns 99999, 0, and -5 here.
	require.Equal(t, 1440, settings.CooldownMinutes)
	require.Equal(t, 1, settings.DisableThreshold)
	require.Equal(t, 1, settings.WindowMinutes)
}

func TestSetOpenAI403CooldownSettings_RejectsOutOfRangeWhenEnabled(t *testing.T) {
	tests := []struct {
		name     string
		settings OpenAI403CooldownSettings
		field    string
	}{
		{
			name: "cooldown_minutes",
			settings: OpenAI403CooldownSettings{
				Enabled: true, CooldownMinutes: 0, DisableThreshold: 3, WindowMinutes: 180,
			},
			field: "cooldown_minutes",
		},
		{
			name: "disable_threshold",
			settings: OpenAI403CooldownSettings{
				Enabled: true, CooldownMinutes: 10, DisableThreshold: 101, WindowMinutes: 180,
			},
			field: "disable_threshold",
		},
		{
			name: "window_minutes",
			settings: OpenAI403CooldownSettings{
				Enabled: true, CooldownMinutes: 10, DisableThreshold: 3, WindowMinutes: 1441,
			},
			field: "window_minutes",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			svc := NewSettingService(newMockSettingRepo(), &config.Config{})

			err := svc.SetOpenAI403CooldownSettings(context.Background(), &test.settings)

			// MUTATION-SANITY: removing the enabled-state validation makes this call succeed.
			require.Error(t, err)
			require.Contains(t, err.Error(), test.field)
		})
	}
}

func TestSetOpenAI403CooldownSettings_NormalizesWhenDisabled(t *testing.T) {
	repo := newMockSettingRepo()
	svc := NewSettingService(repo, &config.Config{})
	settings := &OpenAI403CooldownSettings{
		Enabled:          false,
		CooldownMinutes:  0,
		DisableThreshold: 101,
		WindowMinutes:    -1,
	}

	err := svc.SetOpenAI403CooldownSettings(context.Background(), settings)
	require.NoError(t, err)
	stored, err := svc.GetOpenAI403CooldownSettings(context.Background())
	require.NoError(t, err)
	// MUTATION-SANITY: changing the disabled branch to reject invalid values fails before this normalized result.
	require.False(t, stored.Enabled)
	require.Equal(t, 10, stored.CooldownMinutes)
	require.Equal(t, 3, stored.DisableThreshold)
	require.Equal(t, 180, stored.WindowMinutes)
}

func TestHandleOpenAI403_UsesConfiguredCooldownMinutes(t *testing.T) {
	h := newOpenAI403TestHarness(t, 601, 1)
	setOpenAI403TestSettings(t, h, OpenAI403CooldownSettings{
		Enabled: true, CooldownMinutes: 2, DisableThreshold: 3, WindowMinutes: 180,
	})
	recorder := &openAI403TempRecorder{rateLimitAccountRepoStub: h.repo}
	h.svc.accountRepo = recorder
	before := time.Now()

	require.True(t, h.handle(`{"error":{"message":"temporary edge rejection"}}`))

	// MUTATION-SANITY: replacing the configured cooldown with the 10-minute constant misses this window.
	require.WithinDuration(t, before.Add(2*time.Minute), recorder.until, 5*time.Second)
	require.Less(t, recorder.until.Sub(before), 5*time.Minute)
}

func TestHandleOpenAI403_UsesConfiguredThreshold(t *testing.T) {
	t.Run("at_threshold_disables", func(t *testing.T) {
		h := newOpenAI403TestHarness(t, 602, 2)
		setOpenAI403TestSettings(t, h, OpenAI403CooldownSettings{
			Enabled: true, CooldownMinutes: 10, DisableThreshold: 2, WindowMinutes: 180,
		})

		require.True(t, h.handle(`{"error":{"message":"workspace forbidden"}}`))
		// MUTATION-SANITY: comparing against the original threshold of 3 leaves this account temporary instead.
		require.Equal(t, 1, h.repo.setErrorCalls)
		require.Equal(t, 0, h.repo.tempCalls)
	})

	t.Run("below_threshold_is_temporary", func(t *testing.T) {
		h := newOpenAI403TestHarness(t, 603, 1)
		setOpenAI403TestSettings(t, h, OpenAI403CooldownSettings{
			Enabled: true, CooldownMinutes: 10, DisableThreshold: 2, WindowMinutes: 180,
		})

		require.True(t, h.handle(`{"error":{"message":"temporary edge rejection"}}`))
		// MUTATION-SANITY: reversing the configured threshold comparison permanently disables this account.
		require.Equal(t, 0, h.repo.setErrorCalls)
		require.Equal(t, 1, h.repo.tempCalls)
	})
}

func TestHandleOpenAI403_PassesConfiguredWindowToCounter(t *testing.T) {
	h := newOpenAI403TestHarness(t, 604, 1)
	setOpenAI403TestSettings(t, h, OpenAI403CooldownSettings{
		Enabled: true, CooldownMinutes: 10, DisableThreshold: 3, WindowMinutes: 30,
	})
	counter := &openAI403WindowRecorder{countingOpenAI403CounterCache: h.counter}
	h.svc.SetOpenAI403CounterCache(counter)

	require.True(t, h.handle(`{"error":{"message":"temporary edge rejection"}}`))

	// MUTATION-SANITY: passing the original 180-minute constant records 180 instead of 30.
	require.Equal(t, []int{30}, counter.windows)
}

func TestHandleOpenAI403_DisabledSkipsAccountPenalty(t *testing.T) {
	h := newOpenAI403TestHarness(t, 605, 1)
	setOpenAI403TestSettings(t, h, OpenAI403CooldownSettings{
		Enabled: false, CooldownMinutes: 10, DisableThreshold: 3, WindowMinutes: 180,
	})

	// MUTATION-SANITY: deleting the disabled guard returns true and records counter and penalty calls.
	require.False(t, h.handle(`{"error":{"message":"temporary edge rejection"}}`))
	h.requireNoAccountPenalty(t)
}

func TestHandleOpenAI403_FallsBackToDefaultsWithoutSettingService(t *testing.T) {
	h := newOpenAI403TestHarness(t, 606, 1)
	recorder := &openAI403TempRecorder{rateLimitAccountRepoStub: h.repo}
	h.svc.accountRepo = recorder
	counter := &openAI403WindowRecorder{countingOpenAI403CounterCache: h.counter}
	h.svc.SetOpenAI403CounterCache(counter)
	before := time.Now()

	require.True(t, h.handle(`{"error":{"message":"temporary edge rejection"}}`))

	// MUTATION-SANITY: breaking the no-service fallback changes the 10-minute duration or 180-minute window.
	require.WithinDuration(t, before.Add(10*time.Minute), recorder.until, 5*time.Second)
	require.Equal(t, []int{180}, counter.windows)
}

func TestHandleOpenAI403_HTMLBodyStillSkipsPenaltyWhenEnabled(t *testing.T) {
	h := newOpenAI403TestHarness(t, 607, 1)
	setOpenAI403TestSettings(t, h, OpenAI403CooldownSettings{
		Enabled: true, CooldownMinutes: 10, DisableThreshold: 3, WindowMinutes: 180,
	})

	// MUTATION-SANITY: moving the settings branch ahead of the HTML guard allows account penalty side effects.
	require.False(t, h.handle(openAI403HTMLBody))
	h.requireNoAccountPenalty(t)
}
