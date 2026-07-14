package service

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSchedulerCandidateScore_PreservesPriorityAndLRUBusinessOrder(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	older := now.Add(-time.Hour)
	bucket := SchedulerBucket{GroupID: 10, Platform: PlatformOpenAI, Mode: SchedulerModeSingle}

	higherPriority := Account{ID: 1, Priority: 1, LastUsedAt: &now}
	lowerPriority := Account{ID: 2, Priority: 2, LastUsedAt: nil}
	require.Less(t,
		SchedulerCandidateScore(higherPriority, bucket),
		SchedulerCandidateScore(lowerPriority, bucket),
		"priority must dominate LRU",
	)

	olderUse := Account{ID: 3, Priority: 1, LastUsedAt: &older}
	require.Less(t,
		SchedulerCandidateScore(olderUse, bucket),
		SchedulerCandidateScore(higherPriority, bucket),
		"older last_used_at must win within one priority",
	)

	neverUsed := Account{ID: 4, Priority: 1}
	require.Less(t,
		SchedulerCandidateScore(neverUsed, bucket),
		SchedulerCandidateScore(olderUse, bucket),
		"never-used accounts must be first within one priority",
	)
}

func TestSchedulerCandidateScore_CoversSupportedPlatformsAndAccountTypes(t *testing.T) {
	platformTypes := map[string][]string{
		PlatformAnthropic:   {AccountTypeAPIKey, AccountTypeOAuth, AccountTypeSetupToken, AccountTypeBedrock, AccountTypeServiceAccount},
		PlatformGemini:      {AccountTypeAPIKey, AccountTypeOAuth, AccountTypeServiceAccount},
		PlatformOpenAI:      {AccountTypeAPIKey, AccountTypeOAuth},
		PlatformAntigravity: {AccountTypeAPIKey, AccountTypeOAuth, AccountTypeUpstream},
		PlatformGrok:        {AccountTypeAPIKey, AccountTypeOAuth},
	}
	for platform, accountTypes := range platformTypes {
		for _, accountType := range accountTypes {
			t.Run(platform+"/"+accountType, func(t *testing.T) {
				account := Account{ID: 1, Platform: platform, Type: accountType, Priority: 50}
				bucket := SchedulerBucket{GroupID: 1, Platform: platform, Mode: SchedulerModeSingle}
				score := SchedulerCandidateScore(account, bucket)
				require.False(t, math.IsNaN(score))
				require.False(t, math.IsInf(score, 0))
				breakdown := SchedulerCandidateScoreBreakdown(account, bucket)
				require.Contains(t, breakdown, "priority")
				require.Contains(t, breakdown, "last_used_at")
				require.Contains(t, breakdown, "gemini_oauth_tiebreak")
			})
		}
	}
}

func TestSchedulerCandidateScore_GeminiOAuthWinsExactTie(t *testing.T) {
	lastUsed := time.Now().UTC().Truncate(time.Second)
	bucket := SchedulerBucket{Platform: PlatformGemini, Mode: SchedulerModeMixed}
	for _, priority := range []int{1, schedulerMaxPriority} {
		oauth := Account{Platform: PlatformGemini, Type: AccountTypeOAuth, Priority: priority, LastUsedAt: &lastUsed}
		apiKey := Account{Platform: PlatformGemini, Type: AccountTypeAPIKey, Priority: priority, LastUsedAt: &lastUsed}
		oauthScore := SchedulerCandidateScore(oauth, bucket)
		apiKeyScore := SchedulerCandidateScore(apiKey, bucket)

		require.Less(t, oauthScore, apiKeyScore)
		require.InDelta(t, 0.25, apiKeyScore-oauthScore, 0.001)
	}
}

func TestSchedulerCandidateScore_InvalidTimesRemainFinite(t *testing.T) {
	beforeEpoch := time.Unix(-1, 0)
	farFuture := time.Unix(schedulerPriorityScoreWeight+100, 0)
	bucket := SchedulerBucket{Platform: PlatformOpenAI, Mode: SchedulerModeSingle}

	for _, account := range []Account{
		{Priority: math.MaxInt, LastUsedAt: &farFuture},
		{Priority: math.MinInt, LastUsedAt: &beforeEpoch},
	} {
		score := SchedulerCandidateScore(account, bucket)
		require.False(t, math.IsNaN(score))
		require.False(t, math.IsInf(score, 0))
	}
}
