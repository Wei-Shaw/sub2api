//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOpenAIVideoLegacyBillingPreservesDurationWithoutChangingGrok(t *testing.T) {
	for _, platform := range []string{PlatformOpenAI, PlatformGrok} {
		t.Run(platform, func(t *testing.T) {
			logs := &openAIRecordUsageLogRepoStub{inserted: true}
			svc := newOpenAIRecordUsageServiceForTest(logs, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{}, nil)
			price, groupID := 0.2, int64(112)
			key := &APIKey{ID: 2, UserID: 1, GroupID: &groupID, Group: &Group{ID: groupID, Platform: platform, RateMultiplier: 1, VideoPrice720P: &price}}
			result := &OpenAIForwardResult{RequestID: "legacy-video-duration", Model: "custom-video", VideoCount: 1, VideoResolution: "720p", VideoDurationSeconds: 30}
			wantSeconds := 30
			if platform == PlatformGrok {
				wantSeconds = 15
			}
			cost := svc.calculateOpenAIVideoCost(context.Background(), result.Model, key, result, 1)
			require.InDelta(t, float64(wantSeconds)*price, cost.TotalCost, 1e-9)
			require.NoError(t, svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
				Result: result, APIKey: key, User: &User{ID: 1}, Account: &Account{ID: 3, Type: AccountTypeAPIKey, Platform: platform},
			}))
			require.NotNil(t, logs.lastLog)
			require.Equal(t, wantSeconds, *logs.lastLog.VideoDurationSeconds)
			require.InDelta(t, float64(wantSeconds)*price, logs.lastLog.TotalCost, 1e-9)
		})
	}
}
