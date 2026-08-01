package service

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

var (
	ErrImagePlaygroundDisabled = infraerrors.Forbidden(
		"IMAGE_PLAYGROUND_DISABLED",
		"image playground is disabled by the administrator",
	)
	ErrImagePlaygroundUnavailable = infraerrors.ServiceUnavailable(
		"IMAGE_PLAYGROUND_UNAVAILABLE",
		"image playground is unavailable because asynchronous image storage is not enabled",
	)
	ErrImagePlaygroundGroupNotAvailable = infraerrors.New(
		http.StatusForbidden,
		"IMAGE_PLAYGROUND_GROUP_NOT_AVAILABLE",
		"the selected group is not available for image generation",
	)
	ErrImagePlaygroundAPIKeyRequired = infraerrors.Conflict(
		"IMAGE_PLAYGROUND_API_KEY_REQUIRED",
		"create a usable API key bound to the selected image group before using the image playground",
	)
	ErrImagePlaygroundModelNotAvailable = infraerrors.New(
		http.StatusBadRequest,
		"IMAGE_PLAYGROUND_MODEL_NOT_AVAILABLE",
		"the selected image model is not available for this group",
	)
)

const (
	ImagePlaygroundReasonStorageRequired = "IMAGE_STORAGE_REQUIRED"
	ImagePlaygroundReasonAPIKeyRequired  = "API_KEY_REQUIRED"
	ImagePlaygroundReasonAPIKeyUnusable  = "API_KEY_UNUSABLE"
)

type ImagePlaygroundModelOption struct {
	ID                      string                          `json:"id"`
	Sizes                   []string                        `json:"sizes"`
	CustomSizeConstraints   *ImagePlaygroundSizeConstraints `json:"custom_size_constraints,omitempty"`
	Qualities               []string                        `json:"qualities"`
	MaxImages               int                             `json:"max_images"`
	OutputFormats           []string                        `json:"output_formats"`
	Backgrounds             []string                        `json:"backgrounds"`
	OutputCompression       bool                            `json:"output_compression"`
	SupportsImageInput      bool                            `json:"supports_image_input"`
	MaxInputImages          int                             `json:"max_input_images,omitempty"`
	MaxInputImageBytes      int64                           `json:"max_input_image_bytes,omitempty"`
	InputImageFormats       []string                        `json:"input_image_formats,omitempty"`
	ExperimentalAbovePixels int64                           `json:"experimental_above_pixels,omitempty"`
}

type ImagePlaygroundSizeConstraints struct {
	MaxEdge        int   `json:"max_edge"`
	MultipleOf     int   `json:"multiple_of"`
	MaxAspectRatio int   `json:"max_aspect_ratio"`
	MinPixels      int64 `json:"min_pixels"`
	MaxPixels      int64 `json:"max_pixels"`
}

type ImagePlaygroundGroupOption struct {
	ID                int64                        `json:"id"`
	Name              string                       `json:"name"`
	Platform          string                       `json:"platform"`
	SubscriptionType  string                       `json:"subscription_type"`
	Available         bool                         `json:"available"`
	UnavailableReason string                       `json:"unavailable_reason,omitempty"`
	Models            []ImagePlaygroundModelOption `json:"models"`
}

type ImagePlaygroundOptions struct {
	Enabled        bool                         `json:"enabled"`
	RetentionHours int                          `json:"retention_hours"`
	Groups         []ImagePlaygroundGroupOption `json:"groups"`
}

type imagePlaygroundKeySource interface {
	GetAvailableGroups(ctx context.Context, userID int64) ([]Group, error)
	ListImagePlaygroundAPIKeys(ctx context.Context, userID int64) ([]APIKey, error)
}

type imagePlaygroundModelSource interface {
	GetAvailableModels(ctx context.Context, groupID *int64, platform string) []string
}

type imagePlaygroundModelEligibilitySource interface {
	IsImagePlaygroundModelEligible(ctx context.Context, groupID int64, platform, model string) bool
}

type imagePlaygroundSchedulablePlatformSource interface {
	GetSchedulablePlatforms(ctx context.Context, groupID *int64) map[string]struct{}
}

type imagePlaygroundTaskGate interface {
	Enabled() bool
}

type imagePlaygroundFeatureGate interface {
	IsImagePlaygroundEnabled(ctx context.Context) bool
}

type ImagePlaygroundService struct {
	keys   imagePlaygroundKeySource
	models imagePlaygroundModelSource
	tasks  imagePlaygroundTaskGate
	flags  imagePlaygroundFeatureGate
}

func NewImagePlaygroundService(apiKeys *APIKeyService, gateway *GatewayService, tasks *ImageTaskService, settings *SettingService) *ImagePlaygroundService {
	return &ImagePlaygroundService{keys: apiKeys, models: gateway, tasks: tasks, flags: settings}
}

func (s *ImagePlaygroundService) Enabled(ctx context.Context) bool {
	return s != nil && s.flags != nil && s.flags.IsImagePlaygroundEnabled(ctx)
}

func (s *ImagePlaygroundService) Options(ctx context.Context, userID int64) (*ImagePlaygroundOptions, error) {
	if !s.Enabled(ctx) {
		return nil, ErrImagePlaygroundDisabled
	}
	groups, keys, err := s.availableGroupsAndKeys(ctx, userID)
	if err != nil {
		return nil, err
	}

	enabled := s.tasks != nil && s.tasks.Enabled()
	keysByGroup := make(map[int64][]APIKey)
	for i := range keys {
		if keys[i].GroupID != nil {
			keysByGroup[*keys[i].GroupID] = append(keysByGroup[*keys[i].GroupID], keys[i])
		}
	}

	out := make([]ImagePlaygroundGroupOption, 0, len(groups))
	for i := range groups {
		group := groups[i]
		if !isImagePlaygroundGroup(&group) {
			continue
		}
		models := s.modelsForGroup(ctx, &group)
		option := ImagePlaygroundGroupOption{
			ID:               group.ID,
			Name:             group.Name,
			Platform:         group.Platform,
			SubscriptionType: group.SubscriptionType,
			Available:        true,
			Models:           models,
		}
		switch {
		case !enabled:
			option.Available = false
			option.UnavailableReason = ImagePlaygroundReasonStorageRequired
		case len(keysByGroup[group.ID]) == 0:
			option.Available = false
			option.UnavailableReason = ImagePlaygroundReasonAPIKeyRequired
		case selectImagePlaygroundAPIKey(keysByGroup[group.ID]) == nil:
			option.Available = false
			option.UnavailableReason = ImagePlaygroundReasonAPIKeyUnusable
		case len(models) == 0:
			option.Available = false
			option.UnavailableReason = "NO_IMAGE_MODELS"
		}
		out = append(out, option)
	}

	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Available != out[j].Available {
			return out[i].Available
		}
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].ID < out[j].ID
	})
	retentionHours := 24
	if retention, ok := s.tasks.(interface{ Retention() time.Duration }); ok {
		if hours := int(retention.Retention() / time.Hour); hours > 0 {
			retentionHours = hours
		}
	}
	return &ImagePlaygroundOptions{Enabled: enabled, RetentionHours: retentionHours, Groups: out}, nil
}

func (s *ImagePlaygroundService) ResolveAPIKey(ctx context.Context, userID, groupID int64) (*APIKey, error) {
	if !s.Enabled(ctx) {
		return nil, ErrImagePlaygroundDisabled
	}
	if s == nil || s.tasks == nil || !s.tasks.Enabled() {
		return nil, ErrImagePlaygroundUnavailable
	}
	groups, keys, err := s.availableGroupsAndKeys(ctx, userID)
	if err != nil {
		return nil, err
	}
	var selectedGroup *Group
	for i := range groups {
		if groups[i].ID == groupID && isImagePlaygroundGroup(&groups[i]) {
			group := groups[i]
			selectedGroup = &group
			break
		}
	}
	if selectedGroup == nil {
		return nil, ErrImagePlaygroundGroupNotAvailable
	}

	candidates := make([]APIKey, 0)
	for i := range keys {
		if keys[i].GroupID != nil && *keys[i].GroupID == groupID {
			candidates = append(candidates, keys[i])
		}
	}
	key := selectImagePlaygroundAPIKey(candidates)
	if key == nil {
		return nil, ErrImagePlaygroundAPIKeyRequired
	}
	return key, nil
}

func (s *ImagePlaygroundService) ValidateModel(ctx context.Context, group *Group, model string) error {
	model = strings.TrimSpace(model)
	if !isImagePlaygroundGroup(group) || model == "" {
		return ErrImagePlaygroundModelNotAvailable
	}
	for _, option := range s.modelsForGroup(ctx, group) {
		if option.ID == model {
			return nil
		}
	}
	return ErrImagePlaygroundModelNotAvailable
}

func (s *ImagePlaygroundService) ValidateGenerationOptions(
	ctx context.Context,
	group *Group,
	model, size, quality, outputFormat, background string,
	n int,
) error {
	model = strings.TrimSpace(model)
	var selected *ImagePlaygroundModelOption
	for _, option := range s.modelsForGroup(ctx, group) {
		if option.ID == model {
			candidate := option
			selected = &candidate
			break
		}
	}
	if selected == nil {
		return ErrImagePlaygroundModelNotAvailable
	}
	if n < 1 || n > selected.MaxImages {
		return infraerrors.New(
			http.StatusBadRequest,
			"IMAGE_PLAYGROUND_INVALID_COUNT",
			fmt.Sprintf("n must be between 1 and %d for %s", selected.MaxImages, model),
		)
	}
	if size != "" && !containsImagePlaygroundOption(selected.Sizes, size) {
		if err := validateImagePlaygroundCustomSize(size, selected.CustomSizeConstraints); err != nil {
			return err
		}
	}
	if quality != "" && !containsImagePlaygroundOption(selected.Qualities, quality) {
		return invalidImagePlaygroundOption("quality", quality)
	}
	if outputFormat != "" && !containsImagePlaygroundOption(selected.OutputFormats, outputFormat) {
		return invalidImagePlaygroundOption("output_format", outputFormat)
	}
	if background != "" && !containsImagePlaygroundOption(selected.Backgrounds, background) {
		return invalidImagePlaygroundOption("background", background)
	}
	if background == "transparent" && outputFormat == "jpeg" {
		return infraerrors.New(
			http.StatusBadRequest,
			"IMAGE_PLAYGROUND_INVALID_BACKGROUND_FORMAT",
			"transparent backgrounds require png or webp output",
		)
	}
	return nil
}

func (s *ImagePlaygroundService) availableGroupsAndKeys(ctx context.Context, userID int64) ([]Group, []APIKey, error) {
	if s == nil || s.keys == nil || userID <= 0 {
		return nil, nil, ErrImagePlaygroundGroupNotAvailable
	}
	groups, err := s.keys.GetAvailableGroups(ctx, userID)
	if err != nil {
		return nil, nil, err
	}
	keys, err := s.keys.ListImagePlaygroundAPIKeys(ctx, userID)
	if err != nil {
		return nil, nil, err
	}
	return groups, keys, nil
}

func (s *ImagePlaygroundService) modelsForGroup(ctx context.Context, group *Group) []ImagePlaygroundModelOption {
	if !isImagePlaygroundGroup(group) {
		return []ImagePlaygroundModelOption{}
	}
	raw := []string(nil)
	if s.models != nil {
		groupID := group.ID
		raw = s.models.GetAvailableModels(ctx, &groupID, group.Platform)
		if len(raw) == 0 {
			if platforms, ok := s.models.(imagePlaygroundSchedulablePlatformSource); ok {
				if _, schedulable := platforms.GetSchedulablePlatforms(ctx, &groupID)[group.Platform]; !schedulable {
					return []ImagePlaygroundModelOption{}
				}
			}
		}
	}
	if len(raw) == 0 {
		raw = defaultImagePlaygroundModels(group.Platform)
	}

	seen := make(map[string]struct{}, len(raw))
	models := make([]ImagePlaygroundModelOption, 0, len(raw))
	for _, model := range raw {
		model = strings.TrimSpace(model)
		if !isImagePlaygroundModel(group.Platform, model) || !groupAllowsPlaygroundModel(group, model) {
			continue
		}
		if eligibility, ok := s.models.(imagePlaygroundModelEligibilitySource); ok &&
			!eligibility.IsImagePlaygroundModelEligible(ctx, group.ID, group.Platform, model) {
			continue
		}
		if _, ok := seen[model]; ok {
			continue
		}
		seen[model] = struct{}{}
		models = append(models, imagePlaygroundModelOption(group.Platform, model))
	}
	sort.SliceStable(models, func(i, j int) bool { return models[i].ID < models[j].ID })
	return models
}

// IsImagePlaygroundModelEligible keeps the dashboard model picker aligned with
// the same channel pricing restriction enforced later by account scheduling.
func (s *GatewayService) IsImagePlaygroundModelEligible(ctx context.Context, groupID int64, platform, model string) bool {
	if s == nil {
		return false
	}
	var channel *Channel
	if s.channelService != nil {
		var err error
		channel, err = s.channelService.GetChannelForGroup(ctx, groupID)
		if err != nil {
			slog.Warn("failed to verify image playground channel pricing", "group_id", groupID, "error", err)
			return false
		}
	}
	upstreamPricing := channel != nil && channel.RestrictModels && channel.BillingModelSource == BillingModelSourceUpstream
	if channel != nil && channel.RestrictModels && !upstreamPricing && s.checkChannelPricingRestriction(ctx, &groupID, model) {
		return false
	}

	platform = strings.TrimSpace(platform)
	if s.accountRepo == nil || platform == "" {
		return false
	}
	accounts, err := s.accountRepo.ListModelAvailabilityCandidates(ctx, &groupID, []string{platform}, false)
	if err != nil {
		slog.Warn("failed to verify image playground upstream models", "group_id", groupID, "platform", platform, "error", err)
		return false
	}
	for i := range accounts {
		account := &accounts[i]
		upstreamModel := ""
		switch platform {
		case PlatformOpenAI:
			channelMappedModel := model
			if s.channelService != nil {
				channelMappedModel = s.channelService.ResolveChannelMapping(ctx, groupID, model).MappedModel
			}
			if !isOpenAIImageModelSupportedByAccount(account, model, channelMappedModel) {
				continue
			}
			upstreamModel = resolveOpenAIImagesUpstreamModel(account, model, channelMappedModel)
		case PlatformGrok:
			if !account.IsModelSupported(model) {
				continue
			}
			upstreamModel = account.GetMappedModel(model)
		}
		if upstreamModel == "" {
			continue
		}
		if !upstreamPricing || !s.channelService.IsModelRestricted(ctx, groupID, upstreamModel) {
			return true
		}
	}
	return false
}

func (s *APIKeyService) ListImagePlaygroundAPIKeys(ctx context.Context, userID int64) ([]APIKey, error) {
	if s == nil || s.apiKeyRepo == nil {
		return nil, ErrImagePlaygroundGroupNotAvailable
	}
	if repo, ok := s.apiKeyRepo.(apiKeyAllByUserIDLister); ok {
		keys, err := repo.ListAllByUserID(ctx, userID, APIKeyListFilters{})
		if err != nil {
			return nil, err
		}
		s.fillCurrentConcurrency(ctx, keys)
		return keys, nil
	}

	params := pagination.PaginationParams{Page: 1, PageSize: 1000, SortBy: "created_at", SortOrder: pagination.SortOrderDesc}
	keys, _, err := s.apiKeyRepo.ListByUserID(ctx, userID, params, APIKeyListFilters{})
	if err != nil {
		return nil, err
	}
	s.fillCurrentConcurrency(ctx, keys)
	return keys, nil
}

func selectImagePlaygroundAPIKey(keys []APIKey) *APIKey {
	candidates := make([]APIKey, 0, len(keys))
	for i := range keys {
		key := keys[i]
		if key.GroupID == nil || !key.IsActive() || key.IsExpired() || key.IsQuotaExhausted() {
			continue
		}
		candidates = append(candidates, key)
	}
	if len(candidates) == 0 {
		return nil
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		iRestricted := len(candidates[i].IPWhitelist) > 0 || len(candidates[i].IPBlacklist) > 0
		jRestricted := len(candidates[j].IPWhitelist) > 0 || len(candidates[j].IPBlacklist) > 0
		if iRestricted != jRestricted {
			return !iRestricted
		}
		if candidates[i].CurrentConcurrency != candidates[j].CurrentConcurrency {
			return candidates[i].CurrentConcurrency < candidates[j].CurrentConcurrency
		}
		return candidates[i].ID < candidates[j].ID
	})
	selected := candidates[0]
	return &selected
}

func isImagePlaygroundGroup(group *Group) bool {
	if group == nil || !group.IsActive() || !GroupAllowsImageGeneration(group) {
		return false
	}
	if group.Platform != PlatformOpenAI && group.Platform != PlatformGrok {
		return false
	}
	return isDedicatedImagePlaygroundGroup(group)
}

// Image Playground intentionally accepts only groups explicitly restricted to
// image models. This keeps dashboard-generated traffic out of general-purpose
// chat groups and gives administrators one unambiguous place to set pricing.
func isDedicatedImagePlaygroundGroup(group *Group) bool {
	if group == nil || !group.CustomModelsListEnabled() {
		return false
	}
	for _, model := range group.ModelsListConfig.Models {
		if !isImagePlaygroundModelPattern(group.Platform, model) {
			return false
		}
	}
	return true
}

func isImagePlaygroundModelPattern(platform, pattern string) bool {
	pattern = strings.ToLower(strings.TrimSpace(pattern))
	if pattern == "" {
		return false
	}
	if strings.HasSuffix(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		switch platform {
		case PlatformOpenAI:
			return strings.HasPrefix(prefix, "gpt-image-")
		case PlatformGrok:
			return prefix == "grok-imagine-" || strings.HasPrefix(prefix, "grok-imagine-image")
		default:
			return false
		}
	}
	return isImagePlaygroundModel(platform, pattern)
}

func isImagePlaygroundModel(platform, model string) bool {
	switch platform {
	case PlatformOpenAI:
		return IsGPTImageGenerationModel(model)
	case PlatformGrok:
		return IsGrokImageGenerationModel(model)
	default:
		return false
	}
}

func groupAllowsPlaygroundModel(group *Group, model string) bool {
	if group == nil || !group.CustomModelsListEnabled() {
		return true
	}
	for _, allowed := range group.ModelsListConfig.Models {
		allowed = strings.TrimSpace(allowed)
		if allowed == model || (strings.HasSuffix(allowed, "*") && strings.HasPrefix(model, strings.TrimSuffix(allowed, "*"))) {
			return true
		}
	}
	return false
}

func defaultImagePlaygroundModels(platform string) []string {
	switch platform {
	case PlatformOpenAI:
		return []string{"gpt-image-2", "gpt-image-1.5", "gpt-image-1"}
	case PlatformGrok:
		return []string{"grok-imagine-image", "grok-imagine-image-quality"}
	default:
		return nil
	}
}

func imagePlaygroundModelOption(platform, model string) ImagePlaygroundModelOption {
	option := ImagePlaygroundModelOption{
		ID:            model,
		Sizes:         []string{"auto", "1024x1024", "1536x1024", "1024x1536"},
		MaxImages:     4,
		OutputFormats: []string{"png", "jpeg", "webp"},
		Backgrounds:   []string{},
	}
	if platform == PlatformOpenAI {
		option.Qualities = []string{"auto", "low", "medium", "high"}
		option.MaxImages = 10
		option.Backgrounds = []string{"auto", "opaque", "transparent"}
		option.OutputCompression = true
		option.SupportsImageInput = true
		option.MaxInputImages = 4
		option.MaxInputImageBytes = 10 << 20
		option.InputImageFormats = []string{"image/png", "image/jpeg", "image/webp"}
		if isGPTImage2Model(model) {
			option.Sizes = []string{
				"auto",
				"1024x1024",
				"1536x1024",
				"1024x1536",
				"1536x864",
				"864x1536",
				"2048x2048",
				"2048x1152",
				"1152x2048",
				"3840x2160",
				"2160x3840",
			}
			option.CustomSizeConstraints = &ImagePlaygroundSizeConstraints{
				MaxEdge:        3840,
				MultipleOf:     16,
				MaxAspectRatio: 3,
				MinPixels:      655360,
				MaxPixels:      8294400,
			}
			option.ExperimentalAbovePixels = 3686400
			option.Backgrounds = []string{"auto", "opaque"}
		}
	} else {
		option.Qualities = []string{}
	}
	return option
}

func isGPTImage2Model(model string) bool {
	model = strings.ToLower(strings.TrimSpace(model))
	return model == "gpt-image-2" || strings.HasPrefix(model, "gpt-image-2-")
}

func containsImagePlaygroundOption(options []string, value string) bool {
	for _, option := range options {
		if option == value {
			return true
		}
	}
	return false
}

func invalidImagePlaygroundOption(name, value string) error {
	return infraerrors.New(
		http.StatusBadRequest,
		"IMAGE_PLAYGROUND_INVALID_OPTION",
		fmt.Sprintf("%s is not supported: %s", name, value),
	)
}

func validateImagePlaygroundCustomSize(size string, constraints *ImagePlaygroundSizeConstraints) error {
	if constraints == nil {
		return invalidImagePlaygroundOption("size", size)
	}
	widthText, heightText, ok := strings.Cut(strings.ToLower(strings.TrimSpace(size)), "x")
	if !ok || widthText == "" || heightText == "" || strings.Contains(heightText, "x") {
		return invalidImagePlaygroundOption("size", size)
	}
	width, widthErr := strconv.Atoi(widthText)
	height, heightErr := strconv.Atoi(heightText)
	if widthErr != nil || heightErr != nil || width <= 0 || height <= 0 {
		return invalidImagePlaygroundOption("size", size)
	}
	if constraints.MultipleOf > 0 && (width%constraints.MultipleOf != 0 || height%constraints.MultipleOf != 0) {
		return infraerrors.New(http.StatusBadRequest, "IMAGE_PLAYGROUND_INVALID_SIZE", fmt.Sprintf("width and height must be multiples of %d", constraints.MultipleOf))
	}
	if constraints.MaxEdge > 0 && (width > constraints.MaxEdge || height > constraints.MaxEdge) {
		return infraerrors.New(http.StatusBadRequest, "IMAGE_PLAYGROUND_INVALID_SIZE", fmt.Sprintf("width and height must not exceed %d", constraints.MaxEdge))
	}
	shortEdge, longEdge := width, height
	if shortEdge > longEdge {
		shortEdge, longEdge = longEdge, shortEdge
	}
	if constraints.MaxAspectRatio > 0 && longEdge > shortEdge*constraints.MaxAspectRatio {
		return infraerrors.New(http.StatusBadRequest, "IMAGE_PLAYGROUND_INVALID_SIZE", fmt.Sprintf("aspect ratio must not exceed %d:1", constraints.MaxAspectRatio))
	}
	pixels := int64(width) * int64(height)
	if pixels < constraints.MinPixels || pixels > constraints.MaxPixels {
		return infraerrors.New(http.StatusBadRequest, "IMAGE_PLAYGROUND_INVALID_SIZE", fmt.Sprintf("total pixels must be between %d and %d", constraints.MinPixels, constraints.MaxPixels))
	}
	return nil
}
