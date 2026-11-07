//go:build unit

package service

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	"github.com/stretchr/testify/require"
)

func quotaSummaryFixture() *antigravity.QuotaSummaryResponse {
	return &antigravity.QuotaSummaryResponse{Groups: []antigravity.QuotaSummaryGroup{
		{DisplayName: "Gemini Models", Description: "Models within this group: Gemini Flash, Gemini Pro",
			Buckets: []antigravity.QuotaSummaryBucket{
				{BucketID: "gemini-weekly", Window: "weekly", ResetTime: "2026-11-11T15:52:48Z", RemainingFraction: 0.75},
				{BucketID: "gemini-5h", Window: "5h", ResetTime: "2026-11-04T20:52:48Z", RemainingFraction: 0.4},
			}},
		{DisplayName: "Claude and GPT models",
			Buckets: []antigravity.QuotaSummaryBucket{
				{BucketID: "3p-5h", Window: "5h", RemainingFraction: 1},
				{BucketID: "3p-weekly", Window: "weekly", ResetTime: "bad-time", RemainingFraction: 1.2},
			}},
	}}
}

// fetchAvailableModels reports one fraction per model, whichever window is tighter, so the
// 5-hour and the weekly limit could not be told apart. The quota summary reports both.
func TestBuildQuotaGroups_SplitsFiveHourAndWeekly(t *testing.T) {
	groups := buildAntigravityQuotaGroups(quotaSummaryFixture())
	require.Len(t, groups, 2)

	gemini := groups[0]
	require.Equal(t, "gemini", gemini.Key)
	require.Equal(t, "Gemini Models", gemini.Name)
	// Windows are ordered 5h first, then weekly, whatever order Google returns them in.
	require.Len(t, gemini.Windows, 2)
	require.Equal(t, "5h", gemini.Windows[0].Window)
	require.InDelta(t, 60.0, gemini.Windows[0].Utilization, 0.001)
	require.Equal(t, "2026-11-04T20:52:48Z", gemini.Windows[0].ResetsAt.UTC().Format(time.RFC3339))
	require.Equal(t, "weekly", gemini.Windows[1].Window)
	require.InDelta(t, 25.0, gemini.Windows[1].Utilization, 0.001)

	other := groups[1]
	require.Equal(t, "3p", other.Key)
	require.Nil(t, other.Windows[0].ResetsAt, "an idle window has no reset time")
	require.Nil(t, other.Windows[1].ResetsAt, "an unparseable reset time is dropped, not fatal")
	require.Equal(t, 0.0, other.Windows[1].Utilization, "a fraction above 1 never shows negative usage")
}

func TestBuildQuotaGroups_EmptyOrNil(t *testing.T) {
	require.Nil(t, buildAntigravityQuotaGroups(nil))
	require.Nil(t, buildAntigravityQuotaGroups(&antigravity.QuotaSummaryResponse{}))
	require.Nil(t, buildAntigravityQuotaGroups(&antigravity.QuotaSummaryResponse{Groups: []antigravity.QuotaSummaryGroup{{DisplayName: "x"}}}),
		"a group without buckets is not shown")
}

func TestUsageInfo_QuotaGroupsJSON(t *testing.T) {
	info := &UsageInfo{AntigravityQuotaGroups: buildAntigravityQuotaGroups(quotaSummaryFixture())}
	raw, err := json.Marshal(info)
	require.NoError(t, err)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(raw, &decoded))
	groups, ok := decoded["antigravity_quota_groups"].([]any)
	require.True(t, ok, "antigravity_quota_groups must be in the JSON: %s", raw)
	first := groups[0].(map[string]any)
	require.Equal(t, "gemini", first["key"])
	window := first["windows"].([]any)[0].(map[string]any)
	require.Equal(t, "5h", window["window"])
	require.Contains(t, window, "resets_at")
	require.Contains(t, window, "utilization")

	// Absent when there is no summary, so other platforms' JSON is unchanged.
	raw, err = json.Marshal(&UsageInfo{})
	require.NoError(t, err)
	require.NotContains(t, string(raw), "antigravity_quota_groups")
}

// untouchedBucketFixture mirrors what the summary endpoint answers for a
// bucket nobody has drawn from: full remainingFraction and a rolling
// resetTime pinned to the request time plus the full window, with no
// description. A used bucket carries a description and an anchored reset.
func untouchedBucketFixture() *antigravity.QuotaSummaryResponse {
	return &antigravity.QuotaSummaryResponse{Groups: []antigravity.QuotaSummaryGroup{
		{DisplayName: "Gemini Models",
			Buckets: []antigravity.QuotaSummaryBucket{
				// Placeholder: request time (12:20:28Z) + 5h, no description.
				{BucketID: "gemini-5h", Window: "5h", ResetTime: "2026-11-07T17:20:28Z", RemainingFraction: 1},
				// Placeholder: request time + 7d, no description.
				{BucketID: "gemini-weekly", Window: "weekly", ResetTime: "2026-11-14T12:20:28Z", RemainingFraction: 1},
			}},
		{DisplayName: "Claude and GPT models",
			Buckets: []antigravity.QuotaSummaryBucket{
				// Used bucket: partial fraction, description, anchored reset far
				// from request + window.
				{BucketID: "3p-5h", Window: "5h", Description: "You have used some of your 5-hour limit.",
					ResetTime: "2026-11-07T14:05:11Z", RemainingFraction: 0.62},
			}},
	}}
}

func TestBuildQuotaGroups_DropsRollingPlaceholderResets(t *testing.T) {
	requestedAt := time.Date(2026, 11, 7, 12, 20, 28, 0, time.UTC)
	groups := buildAntigravityQuotaGroupsAt(untouchedBucketFixture(), requestedAt)
	require.Len(t, groups, 2)

	gemini := groups[0]
	require.Nil(t, gemini.Windows[0].ResetsAt, "untouched 5h placeholder reset must be dropped")
	require.Nil(t, gemini.Windows[1].ResetsAt, "untouched weekly placeholder reset must be dropped")
	require.InDelta(t, 0.0, gemini.Windows[0].Utilization, 0.001)

	other := groups[1]
	require.NotNil(t, other.Windows[0].ResetsAt, "a used bucket keeps its anchored reset")
	require.Equal(t, "2026-11-07T14:05:11Z", other.Windows[0].ResetsAt.UTC().Format(time.RFC3339))
	require.InDelta(t, 38.0, other.Windows[0].Utilization, 0.001)
}

func TestBuildQuotaGroups_PlaceholderWithinTolerance(t *testing.T) {
	requestedAt := time.Date(2026, 11, 7, 12, 20, 28, 0, time.UTC)
	// Upstream clocks can drift a little: a reset 42s past request+window is
	// still a placeholder.
	fixture := untouchedBucketFixture()
	fixture.Groups[0].Buckets[0].ResetTime = "2026-11-07T17:21:10Z"
	groups := buildAntigravityQuotaGroupsAt(fixture, requestedAt)
	require.Nil(t, groups[0].Windows[0].ResetsAt, "drift within the 60s tolerance is still a placeholder")
}

func TestBuildQuotaGroups_FullBucketWithDescriptionKeepsReset(t *testing.T) {
	requestedAt := time.Date(2026, 11, 7, 12, 20, 28, 0, time.UTC)
	// A full bucket that carries a description is measured, not a placeholder:
	// its reset is anchored to the request + window only by coincidence, so it
	// must survive.
	fixture := &antigravity.QuotaSummaryResponse{Groups: []antigravity.QuotaSummaryGroup{{
		DisplayName: "Gemini Models",
		Buckets: []antigravity.QuotaSummaryBucket{{
			BucketID: "gemini-5h", Window: "5h", Description: "You have used some of your 5-hour limit.",
			ResetTime: "2026-11-07T17:20:28Z", RemainingFraction: 1,
		}},
	}}}
	groups := buildAntigravityQuotaGroupsAt(fixture, requestedAt)
	require.NotNil(t, groups[0].Windows[0].ResetsAt, "a described bucket keeps its reset even when full")
}

func TestBuildQuotaGroups_UnknownWindowNeverDropped(t *testing.T) {
	requestedAt := time.Date(2026, 11, 7, 12, 20, 28, 0, time.UTC)
	fixture := &antigravity.QuotaSummaryResponse{Groups: []antigravity.QuotaSummaryGroup{{
		DisplayName: "x",
		Buckets: []antigravity.QuotaSummaryBucket{{
			BucketID: "x-daily", Window: "daily", ResetTime: "2026-11-07T17:20:28Z", RemainingFraction: 1,
		}},
	}}}
	groups := buildAntigravityQuotaGroupsAt(fixture, requestedAt)
	require.NotNil(t, groups[0].Windows[0].ResetsAt, "an unknown window cannot be classified as a placeholder")
}
