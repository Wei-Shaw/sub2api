package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type imagePlaygroundKeySourceStub struct {
	groups []Group
	keys   []APIKey
	err    error
}

func (s *imagePlaygroundKeySourceStub) GetAvailableGroups(context.Context, int64) ([]Group, error) {
	return s.groups, s.err
}

func (s *imagePlaygroundKeySourceStub) ListImagePlaygroundAPIKeys(context.Context, int64) ([]APIKey, error) {
	return s.keys, s.err
}

type imagePlaygroundModelSourceStub struct {
	byGroup map[int64][]string
}

func (s *imagePlaygroundModelSourceStub) GetAvailableModels(_ context.Context, groupID *int64, _ string) []string {
	if groupID == nil {
		return nil
	}
	return s.byGroup[*groupID]
}

type imagePlaygroundSchedulableModelSourceStub struct {
	imagePlaygroundModelSourceStub
	platformsByGroup map[int64]map[string]struct{}
}

func (s *imagePlaygroundSchedulableModelSourceStub) GetSchedulablePlatforms(_ context.Context, groupID *int64) map[string]struct{} {
	if groupID == nil {
		return nil
	}
	return s.platformsByGroup[*groupID]
}

type imagePlaygroundTaskGateStub bool

func (s imagePlaygroundTaskGateStub) Enabled() bool { return bool(s) }

type imagePlaygroundFeatureGateStub bool

func (s imagePlaygroundFeatureGateStub) IsImagePlaygroundEnabled(context.Context) bool {
	return bool(s)
}

func imagePlaygroundGroup(id int64, platform string) Group {
	models := []string{"gpt-image-*"}
	if platform == PlatformGrok {
		models = []string{"grok-imagine-*"}
	}
	return Group{
		ID:                   id,
		Name:                 platform,
		Platform:             platform,
		Status:               StatusActive,
		AllowImageGeneration: true,
		ModelsListConfig:     GroupModelsListConfig{Enabled: true, Models: models},
	}
}

func imagePlaygroundKey(id, userID, groupID int64) APIKey {
	return APIKey{ID: id, UserID: userID, GroupID: &groupID, Key: "sk-test", Status: StatusActive}
}

func TestImagePlaygroundOptionsFiltersGroupsModelsAndAvailability(t *testing.T) {
	openAI := imagePlaygroundGroup(1, PlatformOpenAI)
	grok := imagePlaygroundGroup(2, PlatformGrok)
	disabled := imagePlaygroundGroup(3, PlatformOpenAI)
	disabled.AllowImageGeneration = false
	unsupported := imagePlaygroundGroup(4, PlatformGemini)
	svc := &ImagePlaygroundService{
		keys: &imagePlaygroundKeySourceStub{
			groups: []Group{grok, unsupported, disabled, openAI},
			keys:   []APIKey{imagePlaygroundKey(10, 7, openAI.ID)},
		},
		models: &imagePlaygroundModelSourceStub{byGroup: map[int64][]string{
			openAI.ID: {"gpt-5", "gpt-image-2", "gpt-image-2"},
			grok.ID:   {"grok-imagine-image"},
		}},
		tasks: imagePlaygroundTaskGateStub(true),
		flags: imagePlaygroundFeatureGateStub(true),
	}

	options, err := svc.Options(context.Background(), 7)
	require.NoError(t, err)
	require.True(t, options.Enabled)
	require.Equal(t, 24, options.RetentionHours)
	require.Len(t, options.Groups, 2)
	require.Equal(t, openAI.ID, options.Groups[0].ID)
	require.True(t, options.Groups[0].Available)
	require.Equal(t, []ImagePlaygroundModelOption{imagePlaygroundModelOption(PlatformOpenAI, "gpt-image-2")}, options.Groups[0].Models)
	require.Equal(t, grok.ID, options.Groups[1].ID)
	require.False(t, options.Groups[1].Available)
	require.Equal(t, ImagePlaygroundReasonAPIKeyRequired, options.Groups[1].UnavailableReason)
}

func TestImagePlaygroundOptionsExcludesMixedAndUnrestrictedGroups(t *testing.T) {
	dedicated := imagePlaygroundGroup(1, PlatformOpenAI)
	mixed := imagePlaygroundGroup(2, PlatformOpenAI)
	mixed.ModelsListConfig.Models = []string{"gpt-image-2", "gpt-5.4"}
	unrestricted := imagePlaygroundGroup(3, PlatformOpenAI)
	unrestricted.ModelsListConfig = GroupModelsListConfig{}
	svc := &ImagePlaygroundService{
		keys: &imagePlaygroundKeySourceStub{
			groups: []Group{mixed, unrestricted, dedicated},
			keys:   []APIKey{imagePlaygroundKey(10, 7, dedicated.ID)},
		},
		models: &imagePlaygroundModelSourceStub{byGroup: map[int64][]string{
			dedicated.ID: {"gpt-image-2"},
		}},
		tasks: imagePlaygroundTaskGateStub(true),
		flags: imagePlaygroundFeatureGateStub(true),
	}

	options, err := svc.Options(context.Background(), 7)
	require.NoError(t, err)
	require.Len(t, options.Groups, 1)
	require.Equal(t, dedicated.ID, options.Groups[0].ID)
}

func TestImagePlaygroundOptionsExcludesEmptyCustomModelList(t *testing.T) {
	empty := imagePlaygroundGroup(1, PlatformOpenAI)
	empty.ModelsListConfig.Models = nil
	svc := &ImagePlaygroundService{
		keys: &imagePlaygroundKeySourceStub{
			groups: []Group{empty},
			keys:   []APIKey{imagePlaygroundKey(10, 7, empty.ID)},
		},
		models: &imagePlaygroundModelSourceStub{},
		tasks:  imagePlaygroundTaskGateStub(true),
		flags:  imagePlaygroundFeatureGateStub(true),
	}

	options, err := svc.Options(context.Background(), 7)
	require.NoError(t, err)
	require.Empty(t, options.Groups)
}

func TestImagePlaygroundOptionsDoesNotInventModelsWithoutSchedulableAccounts(t *testing.T) {
	group := imagePlaygroundGroup(1, PlatformOpenAI)
	svc := &ImagePlaygroundService{
		keys: &imagePlaygroundKeySourceStub{
			groups: []Group{group},
			keys:   []APIKey{imagePlaygroundKey(10, 7, group.ID)},
		},
		models: &imagePlaygroundSchedulableModelSourceStub{},
		tasks:  imagePlaygroundTaskGateStub(true),
		flags:  imagePlaygroundFeatureGateStub(true),
	}

	options, err := svc.Options(context.Background(), 7)
	require.NoError(t, err)
	require.Len(t, options.Groups, 1)
	require.False(t, options.Groups[0].Available)
	require.Equal(t, "NO_IMAGE_MODELS", options.Groups[0].UnavailableReason)
	require.Empty(t, options.Groups[0].Models)
}

func TestImagePlaygroundOptionsUsesDefaultsForSchedulableAccountsWithoutMappings(t *testing.T) {
	group := imagePlaygroundGroup(1, PlatformOpenAI)
	svc := &ImagePlaygroundService{
		keys: &imagePlaygroundKeySourceStub{
			groups: []Group{group},
			keys:   []APIKey{imagePlaygroundKey(10, 7, group.ID)},
		},
		models: &imagePlaygroundSchedulableModelSourceStub{
			platformsByGroup: map[int64]map[string]struct{}{
				group.ID: {PlatformOpenAI: {}},
			},
		},
		tasks: imagePlaygroundTaskGateStub(true),
		flags: imagePlaygroundFeatureGateStub(true),
	}

	options, err := svc.Options(context.Background(), 7)
	require.NoError(t, err)
	require.Len(t, options.Groups, 1)
	require.True(t, options.Groups[0].Available)
	require.NotEmpty(t, options.Groups[0].Models)
}

func TestImagePlaygroundOptionsReportsStorageDisabled(t *testing.T) {
	group := imagePlaygroundGroup(1, PlatformOpenAI)
	svc := &ImagePlaygroundService{
		keys: &imagePlaygroundKeySourceStub{
			groups: []Group{group},
			keys:   []APIKey{imagePlaygroundKey(10, 7, group.ID)},
		},
		models: &imagePlaygroundModelSourceStub{},
		tasks:  imagePlaygroundTaskGateStub(false),
		flags:  imagePlaygroundFeatureGateStub(true),
	}

	options, err := svc.Options(context.Background(), 7)
	require.NoError(t, err)
	require.False(t, options.Enabled)
	require.Len(t, options.Groups, 1)
	require.False(t, options.Groups[0].Available)
	require.Equal(t, ImagePlaygroundReasonStorageRequired, options.Groups[0].UnavailableReason)
}

func TestImagePlaygroundResolveAPIKeySelectsUsableLeastBusyKey(t *testing.T) {
	group := imagePlaygroundGroup(1, PlatformOpenAI)
	restricted := imagePlaygroundKey(1, 7, group.ID)
	restricted.IPWhitelist = []string{"192.0.2.1"}
	restricted.CurrentConcurrency = 0
	busy := imagePlaygroundKey(2, 7, group.ID)
	busy.CurrentConcurrency = 3
	selected := imagePlaygroundKey(3, 7, group.ID)
	selected.CurrentConcurrency = 1
	disabled := imagePlaygroundKey(4, 7, group.ID)
	disabled.Status = StatusAPIKeyDisabled
	svc := &ImagePlaygroundService{
		keys: &imagePlaygroundKeySourceStub{
			groups: []Group{group},
			keys:   []APIKey{restricted, busy, disabled, selected},
		},
		tasks: imagePlaygroundTaskGateStub(true),
		flags: imagePlaygroundFeatureGateStub(true),
	}

	key, err := svc.ResolveAPIKey(context.Background(), 7, group.ID)
	require.NoError(t, err)
	require.Equal(t, selected.ID, key.ID)
}

func TestImagePlaygroundDisabledRejectsOptionsAndKeyResolution(t *testing.T) {
	svc := &ImagePlaygroundService{flags: imagePlaygroundFeatureGateStub(false)}

	_, err := svc.Options(context.Background(), 7)
	require.ErrorIs(t, err, ErrImagePlaygroundDisabled)
	_, err = svc.ResolveAPIKey(context.Background(), 7, 1)
	require.ErrorIs(t, err, ErrImagePlaygroundDisabled)
}

func TestImagePlaygroundValidateModelHonorsCustomModelList(t *testing.T) {
	group := imagePlaygroundGroup(1, PlatformOpenAI)
	group.ModelsListConfig = GroupModelsListConfig{Enabled: true, Models: []string{"gpt-image-2"}}
	svc := &ImagePlaygroundService{
		models: &imagePlaygroundModelSourceStub{byGroup: map[int64][]string{
			group.ID: {"gpt-image-1", "gpt-image-2"},
		}},
	}

	require.NoError(t, svc.ValidateModel(context.Background(), &group, "gpt-image-2"))
	require.ErrorIs(t, svc.ValidateModel(context.Background(), &group, "gpt-image-1"), ErrImagePlaygroundModelNotAvailable)
	require.ErrorIs(t, svc.ValidateModel(context.Background(), &group, "gpt-5"), ErrImagePlaygroundModelNotAvailable)
}

func TestImagePlaygroundGPTImage2ExposesCurrentOpenAICapabilities(t *testing.T) {
	option := imagePlaygroundModelOption(PlatformOpenAI, "gpt-image-2")

	require.Equal(t, 10, option.MaxImages)
	require.Contains(t, option.Sizes, "1536x864")
	require.Contains(t, option.Sizes, "3840x2160")
	require.Equal(t, []string{"auto", "opaque"}, option.Backgrounds)
	require.True(t, option.OutputCompression)
	require.True(t, option.SupportsImageInput)
	require.Equal(t, 4, option.MaxInputImages)
	require.Equal(t, int64(10<<20), option.MaxInputImageBytes)
	require.NotNil(t, option.CustomSizeConstraints)
	require.Equal(t, 16, option.CustomSizeConstraints.MultipleOf)
	require.Equal(t, int64(8294400), option.CustomSizeConstraints.MaxPixels)
}

func TestImagePlaygroundLegacyGPTImageUsesFixedSizesAndSupportsTransparency(t *testing.T) {
	option := imagePlaygroundModelOption(PlatformOpenAI, "gpt-image-1.5")

	require.Nil(t, option.CustomSizeConstraints)
	require.Equal(t, []string{"auto", "1024x1024", "1536x1024", "1024x1536"}, option.Sizes)
	require.Contains(t, option.Backgrounds, "transparent")
}

func TestImagePlaygroundValidateGenerationOptionsChecksModelSizeRules(t *testing.T) {
	group := imagePlaygroundGroup(1, PlatformOpenAI)
	svc := &ImagePlaygroundService{models: &imagePlaygroundModelSourceStub{byGroup: map[int64][]string{
		group.ID: {"gpt-image-1.5", "gpt-image-2"},
	}}}

	require.NoError(t, svc.ValidateGenerationOptions(
		context.Background(), &group, "gpt-image-2", "1536x864", "high", "webp", "opaque", 10,
	))
	require.Error(t, svc.ValidateGenerationOptions(
		context.Background(), &group, "gpt-image-2", "1537x864", "high", "webp", "opaque", 1,
	))
	require.Error(t, svc.ValidateGenerationOptions(
		context.Background(), &group, "gpt-image-2", "3840x3840", "high", "webp", "opaque", 1,
	))
	require.Error(t, svc.ValidateGenerationOptions(
		context.Background(), &group, "gpt-image-2", "1024x1024", "high", "png", "transparent", 1,
	))
	require.Error(t, svc.ValidateGenerationOptions(
		context.Background(), &group, "gpt-image-1.5", "1536x864", "high", "png", "auto", 1,
	))
	require.Error(t, svc.ValidateGenerationOptions(
		context.Background(), &group, "gpt-image-1.5", "1024x1024", "high", "jpeg", "transparent", 1,
	))
}
