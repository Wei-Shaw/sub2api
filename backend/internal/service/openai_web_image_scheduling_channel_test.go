//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestWebImageSchedulingAuthorizesTheChannelMappedModel(t *testing.T) {
	for _, advanced := range []string{"false", "true"} {
		t.Run(advanced, func(t *testing.T) {
			resetOpenAIAdvancedSchedulerSettingCacheForTest()
			allowed, invalid := webImageSchedulingAccount(270), webImageSchedulingAccount(273)
			// The enabled Free account accepts the image family without a mapping;
			// the unconfigured Free account cannot fall back to native Codex.
			allowed.Priority = 10
			allowed.Credentials["model_mapping"] = nil
			invalid.Extra = nil
			channel := Channel{ID: 1, Status: StatusActive, GroupIDs: []int64{101},
				ModelMapping: map[string]map[string]string{PlatformOpenAI: {"gpt-image-2": "gpt-image-1.5"}}}
			svc := &OpenAIGatewayService{
				accountRepo: schedulerTestOpenAIAccountRepo{accounts: []Account{invalid, allowed}}, cfg: &config.Config{},
				rateLimitService: newOpenAIAdvancedSchedulerRateLimitService(advanced),
				channelService:   newTestChannelService(makeStandardRepo(channel, map[int64]string{101: PlatformOpenAI})),
			}
			groupID := int64(101)
			selection, _, err := svc.SelectAccountWithSchedulerForImages(context.Background(), &groupID, "", "gpt-image-2", nil, OpenAIImagesCapabilityNative)
			require.NoError(t, err)
			require.Equal(t, allowed.ID, selection.Account.ID)
			selection.ReleaseFunc()
		})
	}
}
