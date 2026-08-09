package service

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenAIModelTransient_FirstFailureDoesNotCreateLongBlock(t *testing.T) {
	state := newOpenAIAccountModelTransientState(128)
	now := time.Date(2026, 7, 10, 10, 0, 0, 0, time.UTC)

	decision := state.recordFailure(35, "gpt-5.5", now)

	assert.Equal(t, 1, decision.FailureStreak)
	assert.Zero(t, decision.Cooldown)
	assert.False(t, state.isBlocked(35, "gpt-5.5", now))
}

func TestOpenAIModelTransient_SecondFailureCreatesShortModelBlock(t *testing.T) {
	state := newOpenAIAccountModelTransientState(128)
	now := time.Date(2026, 7, 10, 10, 0, 0, 0, time.UTC)
	state.recordFailure(35, "gpt-5.5", now)

	decision := state.recordFailure(35, "gpt-5.5", now.Add(time.Second))

	assert.Equal(t, 2, decision.FailureStreak)
	assert.Equal(t, openAIModelTransientShortCooldown, decision.Cooldown)
	assert.True(t, state.isBlocked(35, "gpt-5.5", now.Add(2*time.Second)))
	assert.False(t, state.isBlocked(35, "gpt-5.5", now.Add(openAIModelTransientShortCooldown+2*time.Second)))
}

func TestOpenAIModelTransient_ThirdFailureCreatesFortyFiveSecondModelBlock(t *testing.T) {
	state := newOpenAIAccountModelTransientState(128)
	now := time.Date(2026, 7, 10, 10, 0, 0, 0, time.UTC)
	state.recordFailure(35, "gpt-5.5", now)
	state.recordFailure(35, "gpt-5.5", now.Add(time.Second))

	decision := state.recordFailure(35, "gpt-5.5", now.Add(2*time.Second))

	assert.Equal(t, 3, decision.FailureStreak)
	assert.Equal(t, 45*time.Second, decision.Cooldown)
	assert.True(t, state.isBlocked(35, "gpt-5.5", now.Add(40*time.Second)))
	assert.False(t, state.isBlocked(35, "gpt-5.5", now.Add(48*time.Second)))
}

func TestOpenAIModelTransient_BlockIsIsolatedByModel(t *testing.T) {
	state := newOpenAIAccountModelTransientState(128)
	now := time.Date(2026, 7, 10, 10, 0, 0, 0, time.UTC)
	state.recordFailure(35, "gpt-5.6-terra", now)
	state.recordFailure(35, "GPT-5.6-TERRA", now.Add(time.Second))

	assert.True(t, state.isBlocked(35, "gpt-5.6-terra", now.Add(2*time.Second)))
	assert.False(t, state.isBlocked(35, "gpt-5.5", now.Add(2*time.Second)))
	assert.False(t, state.isBlocked(47, "gpt-5.6-terra", now.Add(2*time.Second)))
}

func TestOpenAIModelTransient_SuccessClearsStreakAndBlock(t *testing.T) {
	state := newOpenAIAccountModelTransientState(128)
	now := time.Date(2026, 7, 10, 10, 0, 0, 0, time.UTC)
	state.recordFailure(35, "gpt-5.5", now)
	state.recordFailure(35, "gpt-5.5", now.Add(time.Second))
	require.True(t, state.isBlocked(35, "gpt-5.5", now.Add(2*time.Second)))

	state.recordSuccess(35, "gpt-5.5")

	assert.False(t, state.isBlocked(35, "gpt-5.5", now.Add(2*time.Second)))
	decision := state.recordFailure(35, "gpt-5.5", now.Add(3*time.Second))
	assert.Equal(t, 1, decision.FailureStreak)
	assert.Zero(t, decision.Cooldown)
}

func TestOpenAIModelTransient_StaleStreakExpires(t *testing.T) {
	state := newOpenAIAccountModelTransientState(128)
	now := time.Date(2026, 7, 10, 10, 0, 0, 0, time.UTC)
	state.recordFailure(35, "gpt-5.5", now)

	decision := state.recordFailure(35, "gpt-5.5", now.Add(openAIModelTransientStreakTTL+time.Second))

	assert.Equal(t, 1, decision.FailureStreak)
	assert.Zero(t, decision.Cooldown)
}

// A streak must not depend on how often the gateway is called. Sparse traffic
// used to reset the streak between every request, so a broken account+model was
// never cooled down and each request paid a failed attempt plus a failover.
func TestOpenAIModelTransient_StreakSurvivesSparseTraffic(t *testing.T) {
	state := newOpenAIAccountModelTransientState(128)
	now := time.Date(2026, 7, 10, 10, 0, 0, 0, time.UTC)
	gap := 5 * time.Minute
	require.Greater(t, gap, openAIModelTransientLongCooldown,
		"the gap must exceed every cooldown, otherwise this passes for the wrong reason")

	first := state.recordFailure(35, "gpt-5.5", now)
	second := state.recordFailure(35, "gpt-5.5", now.Add(gap))
	third := state.recordFailure(35, "gpt-5.5", now.Add(2*gap))

	assert.Equal(t, 1, first.FailureStreak)
	assert.Zero(t, first.Cooldown)
	assert.Equal(t, 2, second.FailureStreak)
	assert.Equal(t, openAIModelTransientShortCooldown, second.Cooldown)
	assert.Equal(t, 3, third.FailureStreak)
	assert.Equal(t, openAIModelTransientLongCooldown, third.Cooldown)
	assert.True(t, state.isBlocked(35, "gpt-5.5", now.Add(2*gap+time.Second)))
}

// A success between two sparse failures still clears the streak, so an account
// that intermittently works is not pushed into the long cooldown.
func TestOpenAIModelTransient_SuccessResetsStreakAcrossSparseTraffic(t *testing.T) {
	state := newOpenAIAccountModelTransientState(128)
	now := time.Date(2026, 7, 10, 10, 0, 0, 0, time.UTC)
	gap := 5 * time.Minute

	state.recordFailure(35, "gpt-5.5", now)
	state.recordSuccess(35, "gpt-5.5")

	decision := state.recordFailure(35, "gpt-5.5", now.Add(gap))

	assert.Equal(t, 1, decision.FailureStreak)
	assert.Zero(t, decision.Cooldown)
	assert.False(t, state.isBlocked(35, "gpt-5.5", now.Add(gap+time.Second)))
}

func TestOpenAIModelTransient_IgnoresInvalidKeys(t *testing.T) {
	state := newOpenAIAccountModelTransientState(128)
	now := time.Date(2026, 7, 10, 10, 0, 0, 0, time.UTC)

	assert.Zero(t, state.recordFailure(0, "gpt-5.5", now).FailureStreak)
	assert.Zero(t, state.recordFailure(35, " ", now).FailureStreak)
	assert.False(t, state.isBlocked(0, "gpt-5.5", now))
	assert.False(t, state.isBlocked(35, "", now))
	assert.Equal(t, 0, state.size())
}

func TestOpenAIModelTransient_IgnoresOversizedModelKey(t *testing.T) {
	state := newOpenAIAccountModelTransientState(128)
	now := time.Date(2026, 7, 10, 10, 0, 0, 0, time.UTC)
	model := strings.Repeat("m", openAIModelTransientMaxModelBytes+1)

	decision := state.recordFailure(35, model, now)

	assert.Zero(t, decision.FailureStreak)
	assert.False(t, state.isBlocked(35, model, now))
	assert.Equal(t, 0, state.size())
}

func TestOpenAIModelTransient_StateIsBoundedAndConcurrencySafe(t *testing.T) {
	const maxEntries = 16
	state := newOpenAIAccountModelTransientState(maxEntries)
	now := time.Date(2026, 7, 10, 10, 0, 0, 0, time.UTC)
	var wg sync.WaitGroup

	for i := 0; i < 128; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			model := fmt.Sprintf("gpt-test-%d", i)
			state.recordFailure(int64(i+1), model, now.Add(time.Duration(i)*time.Millisecond))
			_ = state.isBlocked(int64(i+1), model, now.Add(time.Second))
		}(i)
	}
	wg.Wait()

	assert.LessOrEqual(t, state.size(), maxEntries)
}
