package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type imageOptionsAccountRepo struct{ schedulerTestOpenAIAccountRepo }

func (r imageOptionsAccountRepo) ListModelAvailabilityCandidates(context.Context, *int64, []string, bool) ([]Account, error) {
	return r.accounts, nil
}

func TestNativeImageOptionsScheduling(t *testing.T) {
	for _, advanced := range []bool{false, true} {
		for _, tc := range []struct {
			name string
			req  OpenAIImagesRequest
		}{
			{"n", OpenAIImagesRequest{N: 2}}, {"mask", OpenAIImagesRequest{HasMask: true}},
			{"mask URL", OpenAIImagesRequest{MaskImageURL: "https://example.com/mask.png"}},
			{"mask upload", OpenAIImagesRequest{MaskUpload: &OpenAIImagesUpload{}}},
			{"jpeg", OpenAIImagesRequest{OutputFormat: "jpeg"}}, {"webp", OpenAIImagesRequest{OutputFormat: "webp"}},
			{"compression", OpenAIImagesRequest{OutputCompression: new(int)}},
			{"moderation", OpenAIImagesRequest{Moderation: "low"}}, {"style", OpenAIImagesRequest{Style: "vivid"}},
		} {
			t.Run(fmt.Sprintf("advanced=%v/%s", advanced, tc.name), func(t *testing.T) {
				native := *nativeImageTestAccount()
				native.Status = StatusActive
				native.Schedulable = true
				native.Concurrency = 1
				oauth := native
				oauth.ID = 2
				oauth.Extra = nil
				oauth.Priority = 1
				repo := imageOptionsAccountRepo{schedulerTestOpenAIAccountRepo{accounts: []Account{native, oauth}}}
				cache := &schedulerTestGatewayCache{}
				ctx := WithOpenAIImagesRequestOptions(context.Background(), &tc.req)
				require.NoError(t, cache.SetSessionAccountID(ctx, 0, "sticky-native", native.ID, time.Hour))
				svc := &OpenAIGatewayService{accountRepo: repo, cache: cache, cfg: &config.Config{}, concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{}), rateLimitService: newOpenAIAdvancedSchedulerRateLimitService(fmt.Sprint(advanced))}
				defer resetOpenAIAdvancedSchedulerSettingCacheForTest()
				selection, _, err := svc.SelectAccountWithSchedulerForImages(ctx, nil, "sticky-native", "gpt-image-2", nil, OpenAIImagesCapabilityNative)
				require.NoError(t, err)
				require.NotNil(t, selection)
				require.Equal(t, oauth.ID, selection.Account.ID)
				if selection.ReleaseFunc != nil {
					selection.ReleaseFunc()
				}
				require.False(t, svc.OpenAIImagesOptionsUnsupported(ctx, nil, "gpt-image-2"))
			})
		}
	}
}
func TestNativeImageOptionsOnlyPoolAndUnavailableCompatibleAccount(t *testing.T) {
	native := *nativeImageTestAccount()
	native.Status = StatusActive
	native.Schedulable = true
	native.Concurrency = 1
	ctx := WithOpenAIImagesRequestOptions(context.Background(), &OpenAIImagesRequest{N: 2})
	svc := &OpenAIGatewayService{accountRepo: imageOptionsAccountRepo{schedulerTestOpenAIAccountRepo{accounts: []Account{native}}}, cache: &schedulerTestGatewayCache{}, cfg: &config.Config{}, concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{})}
	selection, _, err := svc.SelectAccountWithSchedulerForImages(ctx, nil, "", "gpt-image-2", nil, OpenAIImagesCapabilityNative)
	require.Error(t, err)
	require.Nil(t, selection)
	require.True(t, svc.OpenAIImagesOptionsUnsupported(ctx, nil, "gpt-image-2"))
	oauth := native
	oauth.ID = 2
	oauth.Extra = nil
	until := time.Now().Add(time.Hour)
	oauth.RateLimitResetAt = &until
	svc.accountRepo = imageOptionsAccountRepo{schedulerTestOpenAIAccountRepo{accounts: []Account{native, oauth}}}
	selection, _, err = svc.SelectAccountWithSchedulerForImages(ctx, nil, "", "gpt-image-2", nil, OpenAIImagesCapabilityNative)
	require.Error(t, err)
	require.Nil(t, selection)
	require.False(t, svc.OpenAIImagesOptionsUnsupported(ctx, nil, "gpt-image-2"), "temporary unavailability must not be reported as unsupported options")
}
func TestNativeImageOptionsOrdinaryRequestStillEligible(t *testing.T) {
	native := *nativeImageTestAccount()
	native.Status = StatusActive
	native.Schedulable = true
	native.Concurrency = 1
	svc := &OpenAIGatewayService{accountRepo: imageOptionsAccountRepo{schedulerTestOpenAIAccountRepo{accounts: []Account{native}}}, cache: &schedulerTestGatewayCache{}, cfg: &config.Config{}, concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{})}
	ctx := WithOpenAIImagesRequestOptions(context.Background(), &OpenAIImagesRequest{Model: "gpt-image-2", N: 1, Size: "1024x1024", Quality: "high", Stream: true})
	selection, _, err := svc.SelectAccountWithSchedulerForImages(ctx, nil, "", "gpt-image-2", nil, OpenAIImagesCapabilityNative)
	require.NoError(t, err)
	require.Equal(t, native.ID, selection.Account.ID)
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
	require.False(t, svc.OpenAIImagesOptionsUnsupported(ctx, nil, "gpt-image-2"))
}

func TestNativeImagePartialOptionsCompatibility(t *testing.T) {
	for _, stream := range []bool{false, true} {
		for _, count := range []int{-1, 0, 1, 2, 3, 4, 9999} {
			t.Run(fmt.Sprintf("stream=%v/partial=%d", stream, count), func(t *testing.T) {
				req := &OpenAIImagesRequest{N: 1, Stream: stream, PartialImages: &count}
				want := true
				require.Equal(t, want, supportsOpenAINativeImageOptions(req))
				ctx := WithOpenAIImagesRequestOptions(context.Background(), req)
				require.Equal(t, want, accountSupportsOpenAIImageRequestOptions(ctx, nativeImageTestAccount()))
				_, err := buildOpenAINativeImagesBody(req, "gpt-image-2.5-sunburst")
				if want {
					require.NoError(t, err)
				} else {
					require.Error(t, err)
				}
			})
		}
	}
}
