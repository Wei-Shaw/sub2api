package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/stretchr/testify/require"
)

func freeWebImageTestModelIDs() []string {
	var ids []string
	for _, model := range openai.DefaultModels {
		if IsGPTImageGenerationModel(model.ID) {
			ids = append(ids, model.ID)
		}
	}
	return ids
}

func TestFreeWebImageSchedulingUsesEnabled154WithoutImageMapping(t *testing.T) {
	for _, mode := range []struct {
		name, advanced string
		batch          bool
	}{{"legacy", "false", false}, {"legacy_batch", "false", true}, {"advanced", "true", true}} {
		for _, mapping := range []map[string]any{nil, {"gpt-6-luna": "gpt-6-luna"}, {"gpt-image-2.5-flare": "gpt-6-luna"}} {
			t.Run(mode.name, func(t *testing.T) {
				resetOpenAIAdvancedSchedulerSettingCacheForTest()
				allowed, unconfigured := webImageSchedulingAccount(270), webImageSchedulingAccount(273)
				allowed.Credentials["model_mapping"] = mapping
				allowed.Priority = 10
				unconfigured.Extra = nil
				unconfigured.Credentials["model_mapping"] = nil
				cfg := &config.Config{}
				cfg.Gateway.Scheduling.LoadBatchEnabled = mode.batch
				cache := &schedulerTestGatewayCache{sessionBindings: map[string]int64{"openai:free-image": 273}}
				var acquired []int64
				svc := &OpenAIGatewayService{accountRepo: schedulerTestOpenAIAccountRepo{accounts: []Account{unconfigured, allowed}},
					cache: cache, cfg: cfg, rateLimitService: newOpenAIAdvancedSchedulerRateLimitService(mode.advanced),
					concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{acquiredIDs: &acquired})}
				groupID := int64(101)
				selection, _, err := svc.SelectAccountWithSchedulerForImages(context.Background(), &groupID, "free-image", "gpt-image-2.5-flare", nil, OpenAIImagesCapabilityNative)
				require.NoError(t, err)
				require.Equal(t, int64(270), selection.Account.ID)
				require.Equal(t, []int64{270}, acquired)
				selection.ReleaseFunc()
			})
		}
	}
}

func TestFreeWebImageCompositeOwnershipOnlyBypassesNativeImageRequests(t *testing.T) {
	a := webImageSchedulingAccount(270)
	a.Credentials["model_mapping"] = map[string]any{"gpt-6-luna": "gpt-6-luna"}
	for _, tc := range []struct {
		model           string
		images, allowed bool
	}{
		{"gpt-image-2.5-flare", true, true},
		{"image-alias", true, false},
		{"gpt-image-2.5-flare", false, false},
	} {
		ctx := WithCompositeRouteDecision(context.Background(), CompositeRouteDecision{Matched: true, Source: CompositeRouteSourceAccount, GroupID: 101, PublicModel: tc.model, UpstreamModel: tc.model, TargetPlatform: PlatformOpenAI})
		req := OpenAIAccountScheduleRequest{RequestedModel: tc.model}
		if tc.images {
			ctx = WithOpenAIImagesEndpoint(ctx)
			req.RequiredImageCapability = OpenAIImagesCapabilityNative
		}
		reason := openAICompatibleAccountEligibilityFailureReason(ctx, &a, PlatformOpenAI, tc.model, false, "")
		require.Equal(t, tc.allowed, reason == "", reason)
		ok, why := (&defaultOpenAIAccountScheduler{}).isAccountRequestCompatibleReason(ctx, &a, req)
		require.Equal(t, tc.allowed, ok, why)
	}
}

func TestFreeWebImageStandardCatalogIgnoresChatMappingAndRetainsGroupPolicy(t *testing.T) {
	body := []byte(`{"object":"list","data":[{"id":"gpt-6-luna","display_name":"Chat model"},{"id":"unlisted-chat"}]}`)
	for _, tc := range []struct {
		name   string
		change func(*Account)
		group  *Group
		images []string
	}{
		{"enabled", func(*Account) {}, nil, freeWebImageTestModelIDs()},
		{"unconfigured", func(a *Account) { a.Extra = nil }, nil, nil},
		{"disabled", func(a *Account) { a.Extra[AccountExtraChatGPTWebImageEnabled] = false }, nil, nil},
		{"no_consent", func(a *Account) { a.Extra[AccountExtraChatGPTWebImageRegularChat] = false }, nil, nil},
		{"group_images_disabled", func(*Account) {}, &Group{AllowImageGeneration: false}, nil},
		{"group_allowlist", func(*Account) {}, &Group{AllowImageGeneration: true, ModelAllowlist: GroupModelAllowlist{Enabled: true, Models: []string{"gpt-6-luna", "gpt-image-2.5-flare"}}}, []string{"gpt-image-2.5-flare"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := webImagePolicyAccount(map[string]any{"gpt-6-luna": "gpt-6-luna"})
			tc.change(a)
			projected, err := projectAccountModelsBody(body, a, tc.group, false)
			require.NoError(t, err)
			var payload struct {
				Data []openai.Model `json:"data"`
			}
			require.NoError(t, json.Unmarshal(projected, &payload))
			var ids []string
			for _, model := range payload.Data {
				ids = append(ids, model.ID)
			}
			require.ElementsMatch(t, append([]string{"gpt-6-luna"}, tc.images...), ids)
		})
	}
	// Codex's chat-model manifest remains distinct from the Images API catalog.
	projected, err := projectAccountModelsBody([]byte(`{"models":[{"slug":"gpt-6-luna"}]}`), webImagePolicyAccount(map[string]any{"gpt-6-luna": "gpt-6-luna"}), nil, true)
	require.NoError(t, err)
	require.NotContains(t, string(projected), "gpt-image-")
}

func TestFreeWebImageOnlyResponsesUsesLocalToolAndFixedDefault(t *testing.T) {
	a := webImagePolicyAccount(map[string]any{"gpt-image-2.5-flare": "gpt-6-luna"})
	a.Extra[AccountExtraChatGPTWebImageDefaultModel] = "obsolete-entry"
	prepared, selected, err := prepareWebImageResponsesForAccount([]byte(`{"model":"gpt-image-2.5-flare","prompt":"draw a cat"}`), a, true)
	require.NoError(t, err)
	require.Equal(t, "gpt-image-2.5-flare", selected.RequestedModel)
	var request map[string]any
	require.NoError(t, json.Unmarshal(prepared, &request))
	require.Equal(t, openAIImagesResponsesMainModelValue(), request["model"])
	require.Equal(t, "draw a cat", request["input"])
	require.Equal(t, map[string]any{"type": "function", "name": webImageFunctionName}, request["tool_choice"])
	require.NotContains(t, string(prepared), `"type":"image_generation"`)
	_, selected, err = prepareWebImageResponsesForAccount([]byte(`{"model":"gpt-6-luna","input":"draw a cat"}`), a, true)
	require.NoError(t, err)
	require.Equal(t, "gpt-image-2", selected.RequestedModel)
}
