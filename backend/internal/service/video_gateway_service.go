package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
)

const (
	videoDefaultPageSize  = 20
	videoMaxPageSize      = 100
	videoDashboardLimit   = 8
	videoDefaultBatchSize = 20
)

const (
	videoKeyStatusNormal         = "normal"
	videoKeyStatusMissing        = "missing"
	videoKeyStatusDisabled       = "disabled"
	videoKeyStatusAuthFailed     = "auth_failed"
	videoKeyStatusRateLimited    = "rate_limited"
	videoKeyStatusQuotaExhausted = "quota_exhausted"

	videoHealthStatusHealthy          = "healthy"
	videoHealthStatusNeedsKey         = "needs_key"
	videoHealthStatusDisabled         = "disabled"
	videoHealthStatusAuthFailed       = "auth_failed"
	videoHealthStatusRateLimited      = "rate_limited"
	videoHealthStatusQuotaExhausted   = "quota_exhausted"
	videoHealthStatusTimeout          = "timeout"
	videoHealthStatusModerationFailed = "moderation_failed"
	videoHealthStatusModelUnavailable = "model_unavailable"
	videoHealthStatusUnknown          = "unknown"
)

type VideoGatewayService struct {
	repo      VideoGatewayRepository
	encryptor VideoKeyEncryptor
	adapters  map[string]VideoAdapter
	cfg       *config.Config
	// budget is the optional VA1 budget gate / billing hook. nil => no gate (current
	// production default until phase-2 real billing wires a concrete guard).
	budget              VideoBudgetGuard
	userRepo            UserRepository
	settingService      *SettingService
	billingCacheService *BillingCacheService
	generationCollector *GenerationContentCollector
	videoPricing        *VideoPricingCatalog
	taskPricing         VideoTaskPricing
	creationRepo        VideoTaskCreationRepository
	taskFinalizer       *VideoTaskFinalizer
}

type videoRouteDecision struct {
	Account  *VideoProviderAccount
	Strategy string
	Reason   string
	Skipped  []videoRouteSkip
}

type videoRouteSkip struct {
	ID          int64  `json:"id"`
	DisplayName string `json:"display_name"`
	Provider    string `json:"provider"`
	Reason      string `json:"reason"`
}

func NewVideoGatewayService(repo VideoGatewayRepository, encryptor VideoKeyEncryptor, cfg *config.Config) *VideoGatewayService {
	svc := &VideoGatewayService{
		repo:         repo,
		encryptor:    encryptor,
		adapters:     NewVideoAdapterRegistry(),
		cfg:          cfg,
		videoPricing: NewVideoPricingCatalog(nil),
	}
	svc.taskPricing = svc
	svc.creationRepo = repo
	if finalizationRepo, ok := repo.(VideoTaskFinalizationRepository); ok {
		svc.taskFinalizer = NewVideoTaskFinalizer(finalizationRepo)
	}
	return svc
}

func (s *VideoGatewayService) SetVideoTaskPricing(pricing VideoTaskPricing) {
	if pricing == nil {
		pricing = s
	}
	s.taskPricing = pricing
}

func (s *VideoGatewayService) videoTaskPricing() VideoTaskPricing {
	if s == nil || s.taskPricing == nil {
		return s
	}
	return s.taskPricing
}

func (s *VideoGatewayService) SetVideoTaskCreationRepository(repo VideoTaskCreationRepository) {
	s.creationRepo = repo
}

// RegisterVideoAdapter is the narrow composition/test seam for a provider
// implementation. Production registration still happens through the default
// registry; callers cannot alter task pricing or bypass finalization.
func (s *VideoGatewayService) RegisterVideoAdapter(adapter VideoAdapter) error {
	if s == nil || adapter == nil {
		return fmt.Errorf("video adapter is required")
	}
	provider := strings.TrimSpace(adapter.Provider())
	if err := validateVideoProvider(provider); err != nil {
		return err
	}
	if s.adapters == nil {
		s.adapters = make(map[string]VideoAdapter)
	}
	s.adapters[provider] = adapter
	return nil
}

func (s *VideoGatewayService) ListProviderAccounts(ctx context.Context) ([]*VideoProviderAccount, error) {
	items, err := s.repo.ListProviderAccounts(ctx)
	if err != nil {
		return nil, fmt.Errorf("list video providers: %w", err)
	}
	stats, err := s.providerRuntimeStats(ctx)
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		s.decorateProviderForResponse(item, stats[item.ID])
	}
	return items, nil
}

func (s *VideoGatewayService) GetProviderAccount(ctx context.Context, id int64) (*VideoProviderAccount, error) {
	item, err := s.repo.GetProviderAccount(ctx, id)
	if err != nil {
		return nil, err
	}
	stats, err := s.providerRuntimeStats(ctx)
	if err != nil {
		return nil, err
	}
	s.decorateProviderForResponse(item, stats[item.ID])
	return item, nil
}

func (s *VideoGatewayService) CreateProviderAccount(ctx context.Context, p VideoProviderCreateParams) (*VideoProviderAccount, error) {
	if err := validateVideoProvider(p.Provider); err != nil {
		return nil, err
	}
	displayName := strings.TrimSpace(p.DisplayName)
	if displayName == "" {
		displayName = defaultVideoProviderDisplayName(p.Provider)
	}
	account := &VideoProviderAccount{
		Provider:           strings.TrimSpace(p.Provider),
		DisplayName:        displayName,
		Enabled:            p.Enabled,
		BaseURL:            strings.TrimSpace(p.BaseURL),
		DefaultModel:       firstNonEmptyVideo(strings.TrimSpace(p.DefaultModel), defaultVideoModel(p.Provider)),
		RateLimitPerMinute: defaultVideoRateLimit(p.RateLimitPerMinute),
		Metadata:           p.Metadata,
	}
	if strings.TrimSpace(p.APIKey) != "" {
		if err := s.applyProviderAPIKey(account, p.APIKey); err != nil {
			return nil, err
		}
	}
	if err := s.repo.CreateProviderAccount(ctx, account); err != nil {
		return nil, fmt.Errorf("create video provider: %w", err)
	}
	s.decorateProviderForResponse(account, VideoProviderRuntimeStats{})
	return account, nil
}

func (s *VideoGatewayService) UpdateProviderAccount(ctx context.Context, id int64, p VideoProviderUpdateParams) (*VideoProviderAccount, error) {
	account, err := s.repo.GetProviderAccount(ctx, id)
	if err != nil {
		return nil, err
	}
	if p.DisplayName != nil {
		account.DisplayName = strings.TrimSpace(*p.DisplayName)
		if account.DisplayName == "" {
			account.DisplayName = defaultVideoProviderDisplayName(account.Provider)
		}
	}
	if p.Enabled != nil {
		account.Enabled = *p.Enabled
	}
	if p.BaseURL != nil {
		account.BaseURL = strings.TrimSpace(*p.BaseURL)
	}
	if p.DefaultModel != nil {
		account.DefaultModel = firstNonEmptyVideo(strings.TrimSpace(*p.DefaultModel), defaultVideoModel(account.Provider))
	}
	if p.RateLimitPerMinute != nil {
		account.RateLimitPerMinute = defaultVideoRateLimit(*p.RateLimitPerMinute)
	}
	if p.Metadata != nil {
		account.Metadata = *p.Metadata
	}
	if p.APIKey != nil && strings.TrimSpace(*p.APIKey) != "" {
		if err := s.applyProviderAPIKey(account, *p.APIKey); err != nil {
			return nil, err
		}
	}
	if err := s.repo.UpdateProviderAccount(ctx, account); err != nil {
		return nil, fmt.Errorf("update video provider: %w", err)
	}
	stats, err := s.providerRuntimeStats(ctx)
	if err != nil {
		return nil, err
	}
	s.decorateProviderForResponse(account, stats[account.ID])
	return account, nil
}

func (s *VideoGatewayService) TestProviderAccount(ctx context.Context, id int64) (*VideoProviderTestResult, error) {
	account, err := s.repo.GetProviderAccount(ctx, id)
	if err != nil {
		return nil, err
	}
	s.decryptProviderKey(account)
	adapter, err := s.adapterFor(account.Provider)
	if err != nil {
		return nil, err
	}
	preview := adapter.BuildCreatePayload(account, &VideoTask{
		Model:       firstNonEmptyVideo(account.DefaultModel, defaultVideoModel(account.Provider)),
		TaskType:    VideoTaskTypeTextToVideo,
		Prompt:      "mock connection test",
		AspectRatio: "16:9",
		Duration:    5,
		Resolution:  "720p",
	})
	if account.Provider == VideoProviderMock {
		return &VideoProviderTestResult{
			Provider:         account.Provider,
			Configured:       true,
			Reachable:        account.Enabled,
			Message:          "mock provider is local and ready",
			NormalizedStatus: adapter.NormalizeStatus(VideoStatusSubmitted),
			PayloadPreview:   preview,
		}, nil
	}
	if !account.APIKeyConfigured || strings.TrimSpace(account.PlainAPIKey) == "" {
		return &VideoProviderTestResult{
			Provider:         account.Provider,
			Configured:       false,
			Reachable:        false,
			Message:          "api key is not configured; real upstream call skipped",
			NormalizedStatus: adapter.NormalizeStatus("processing"),
			PayloadPreview:   preview,
		}, nil
	}
	return &VideoProviderTestResult{
		Provider:         account.Provider,
		Configured:       true,
		Reachable:        false,
		Message:          "adapter skeleton is mapped; real network test is disabled in P0",
		NormalizedStatus: adapter.NormalizeStatus("processing"),
		PayloadPreview:   preview,
	}, nil
}

func (s *VideoGatewayService) ListAPIKeyMockOnlyProviders(ctx context.Context) ([]*VideoProviderAccount, error) {
	items, err := s.ListProviderAccounts(ctx)
	if err != nil {
		return nil, err
	}

	byProvider := make(map[string]*VideoProviderAccount)
	for _, item := range items {
		if item == nil {
			continue
		}
		existing := byProvider[item.Provider]
		if existing == nil ||
			(item.RouteAvailable && !existing.RouteAvailable) ||
			(item.RouteAvailable == existing.RouteAvailable && item.Priority < existing.Priority) {
			byProvider[item.Provider] = item
		}
	}

	out := make([]*VideoProviderAccount, 0, 3)
	mock := byProvider[VideoProviderMock]
	if mock == nil {
		mock = apiKeySyntheticVideoProvider(VideoProviderMock)
		mock.RouteSkipReason = "mock provider account is not configured"
	}
	out = append(out, mock)

	for _, provider := range []string{VideoProviderSeedance, VideoProviderKling} {
		item := byProvider[provider]
		if item == nil {
			item = apiKeySyntheticVideoProvider(provider)
		}
		item.Enabled = false
		item.APIKeyConfigured = false
		item.PlainAPIKey = ""
		item.MaskedKey = ""
		item.RouteAvailable = false
		item.RouteSkipReason = "disabled in API-key mock-only gateway"
		item.KeyStatus = videoKeyStatusDisabled
		item.HealthStatus = videoHealthStatusDisabled
		item.DiagnosticType = "mock-only"
		item.SuggestedAction = "use provider=mock for this API-key video gateway version"
		out = append(out, item)
	}

	return out, nil
}

func (s *VideoGatewayService) ListAPIKeyTrialProviders(ctx context.Context) ([]*VideoProviderAccount, error) {
	items, err := s.ListProviderAccounts(ctx)
	if err != nil {
		return nil, err
	}

	byProvider := make(map[string]*VideoProviderAccount)
	for _, item := range items {
		if item == nil {
			continue
		}
		existing := byProvider[item.Provider]
		if existing == nil ||
			(item.RouteAvailable && !existing.RouteAvailable) ||
			(item.RouteAvailable == existing.RouteAvailable && item.Priority < existing.Priority) {
			byProvider[item.Provider] = item
		}
	}

	out := make([]*VideoProviderAccount, 0, 3)

	// Mock
	mock := byProvider[VideoProviderMock]
	if mock == nil {
		mock = apiKeySyntheticVideoProvider(VideoProviderMock)
		mock.RouteSkipReason = "mock provider account is not configured"
	}
	out = append(out, mock)

	// Seedance
	seedance := byProvider[VideoProviderSeedance]
	if seedance == nil {
		seedance = apiKeySyntheticVideoProvider(VideoProviderSeedance)
		seedance.RouteSkipReason = "seedance provider account is not configured"
	} else {
		s.decorateProviderForResponse(seedance, VideoProviderRuntimeStats{})
		reasons := []string{}
		if strings.TrimSpace(os.Getenv("SUB2API_VIDEO_REAL_SMOKE_ENABLED")) != "1" {
			reasons = append(reasons, "SUB2API_VIDEO_REAL_SMOKE_ENABLED is not 1")
		}
		if !seedanceRealCallAuthorized(seedance) {
			reasons = append(reasons, "provider metadata does not record single smoke authorization or production authorization")
		}
		if strings.TrimSpace(os.Getenv("SUB2API_VIDEO_REDACTED_EVENT_LOG")) == "" {
			reasons = append(reasons, "redacted event log path is missing")
		}
		if configuredMediaURLAllowlist() == "" {
			reasons = append(reasons, mediaURLAllowlistMissingReason())
		}
		if len(reasons) > 0 {
			seedance.RouteSkipReason = strings.Join(reasons, "; ")
		} else {
			seedance.RouteSkipReason = ""
		}
		seedance.RouteAvailable = false // always false for API-key trial endpoint
		seedance.MaskedKey = ""         // do not expose masked key in API-key trial endpoint
		if seedance.Metadata == nil {
			seedance.Metadata = map[string]any{}
		}
		seedance.Metadata["tiny_real_trial_supported"] = true
		seedance.Metadata["requires_real_smoke_gate"] = true
		seedance.Metadata["blocked_reasons"] = reasons
	}
	out = append(out, seedance)

	// Kling — still blocked
	kling := byProvider[VideoProviderKling]
	if kling == nil {
		kling = apiKeySyntheticVideoProvider(VideoProviderKling)
	}
	kling.Enabled = false
	kling.APIKeyConfigured = false
	kling.PlainAPIKey = ""
	kling.MaskedKey = ""
	kling.RouteAvailable = false
	kling.RouteSkipReason = "kling is disabled in API-key video gateway"
	kling.KeyStatus = videoKeyStatusDisabled
	kling.HealthStatus = videoHealthStatusDisabled
	out = append(out, kling)

	return out, nil
}

func (s *VideoGatewayService) CreateAPIKeyMockOnlyTask(ctx context.Context, provider string, p VideoTaskCreateParams) (*VideoTask, error) {
	provider = strings.TrimSpace(provider)
	if provider == "" {
		provider = VideoProviderMock
	}
	if err := validateVideoProvider(provider); err != nil {
		return nil, err
	}
	if provider != VideoProviderMock {
		return nil, errVideoProviderBlockedMockOnly(provider)
	}

	mock, err := s.resolveAPIKeyMockOnlyRoute(ctx)
	if err != nil {
		return nil, err
	}
	p.ProviderAccountID = mock.ID
	task, err := s.CreateTask(ctx, p)
	if err != nil {
		return nil, err
	}
	_ = s.repo.AddTaskEvent(ctx, &VideoTaskEvent{
		VideoTaskID: task.ID,
		EventType:   "mock_only_gateway",
		Message:     "API-key video gateway accepted mock-only task",
		Payload: map[string]any{
			"provider":                     VideoProviderMock,
			"mock_only":                    true,
			"real_provider_dispatch_count": 0,
		},
	})
	return task, nil
}

func (s *VideoGatewayService) CreateAPIKeySeedanceTinyTrialTask(ctx context.Context, provider string, p VideoTaskCreateParams) (*VideoTask, error) {
	provider = strings.TrimSpace(provider)
	if provider != VideoProviderSeedance {
		return nil, errVideoProviderBlockedMockOnly(provider)
	}
	if task, found, err := s.replayExistingVideoTaskCreation(ctx, p); err != nil || found {
		return task, err
	}

	accounts, err := s.ListProviderAccounts(ctx)
	if err != nil {
		return nil, err
	}
	var seedanceAccount *VideoProviderAccount
	seedanceBlockedReason := ""
	for _, acc := range accounts {
		if acc != nil && acc.Provider == VideoProviderSeedance && acc.Enabled {
			s.decorateProviderForResponse(acc, VideoProviderRuntimeStats{})
			if acc.RouteAvailable {
				seedanceAccount = acc
				break
			}
			if seedanceBlockedReason == "" {
				seedanceBlockedReason = strings.TrimSpace(acc.RouteSkipReason)
			}
		}
	}
	if seedanceAccount == nil {
		metadata := map[string]string{
			"provider":                     VideoProviderSeedance,
			"mock_only":                    "false",
			"real_provider_dispatch_count": "0",
		}
		if seedanceBlockedReason != "" {
			metadata["reason"] = seedanceBlockedReason
		}
		return nil, infraerrors.Forbidden("VIDEO_PROVIDER_DISABLED", "seedance provider account not available for trial").WithMetadata(metadata)
	}

	model := firstNonEmptyVideo(strings.TrimSpace(p.Model), seedanceAccount.DefaultModel, defaultVideoModel(VideoProviderSeedance))
	dummyTask := &VideoTask{
		Provider: seedanceAccount.Provider,
		Model:    model,
		TaskType: p.TaskType,
		Prompt:   strings.TrimSpace(p.Prompt),
		Duration: defaultVideoDuration(p.Duration),
	}
	blockedReasons := seedanceSmokeGateBlockedReasons(seedanceAccount, dummyTask)
	if len(blockedReasons) > 0 {
		return nil, infraerrors.Forbidden("VIDEO_TRIAL_BLOCKED", "seedance tiny real trial blocked by gate").WithMetadata(map[string]string{
			"provider":                     VideoProviderSeedance,
			"blocked_reasons":              strings.Join(blockedReasons, "; "),
			"real_provider_dispatch_count": "0",
		})
	}

	p.ProviderAccountID = seedanceAccount.ID
	route := &videoRouteDecision{
		Account:  seedanceAccount,
		Strategy: "api-key-seedance-tiny-trial",
		Reason:   "seedance tiny real trial gate passed",
	}
	task, err := s.createTaskWithRoute(ctx, p, route, func(ctx context.Context, task *VideoTask) error {
		reserved, err := s.repo.CreateDailyTrialTask(ctx, task, VideoProviderSeedance, task.CreatedBy, timezone.Today())
		if err != nil {
			return err
		}
		if !reserved {
			return ErrVideoTrialLimitExceeded.WithMetadata(map[string]string{
				"provider":                     VideoProviderSeedance,
				"real_provider_dispatch_count": "0",
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	if !task.CreationReplayed {
		_ = s.repo.AddTaskEvent(ctx, &VideoTaskEvent{
			VideoTaskID: task.ID,
			EventType:   "trial_gate",
			Message:     "seedance tiny real trial gate passed",
			Payload: map[string]any{
				"provider":                        VideoProviderSeedance,
				"trial_mode":                      "tiny_real",
				"gate_result":                     "passed",
				"planned_provider_dispatch_count": 1,
				"real_provider_dispatch_count":    0,
			},
		})
	}

	return task, nil
}

func (s *VideoGatewayService) CreateAPIKeySeedanceProductionTask(ctx context.Context, provider string, p VideoTaskCreateParams) (*VideoTask, error) {
	provider = strings.TrimSpace(provider)
	if provider != VideoProviderSeedance {
		return nil, errVideoProviderBlockedMockOnly(provider)
	}
	if task, found, err := s.replayExistingVideoTaskCreation(ctx, p); err != nil || found {
		return task, err
	}

	accounts, err := s.ListProviderAccounts(ctx)
	if err != nil {
		return nil, err
	}
	var seedanceAccount *VideoProviderAccount
	seedanceBlockedReason := ""
	for _, acc := range accounts {
		if acc != nil && acc.Provider == VideoProviderSeedance && acc.Enabled {
			s.decorateProviderForResponse(acc, VideoProviderRuntimeStats{})
			if acc.RouteAvailable && seedanceProductionAuthorized(acc) {
				seedanceAccount = acc
				break
			}
			if seedanceBlockedReason == "" {
				if !seedanceProductionAuthorized(acc) {
					seedanceBlockedReason = "provider metadata production_authorized is not true"
				} else {
					seedanceBlockedReason = strings.TrimSpace(acc.RouteSkipReason)
				}
			}
		}
	}
	if seedanceAccount == nil {
		metadata := map[string]string{
			"provider":                     VideoProviderSeedance,
			"mock_only":                    "false",
			"real_provider_dispatch_count": "0",
			"production_authorized":        "false",
		}
		if seedanceBlockedReason != "" {
			metadata["reason"] = seedanceBlockedReason
		}
		return nil, infraerrors.Forbidden("VIDEO_PRODUCTION_NOT_AUTHORIZED", "seedance production is not authorized for this provider account").WithMetadata(metadata)
	}

	model := firstNonEmptyVideo(strings.TrimSpace(p.Model), seedanceAccount.DefaultModel, defaultVideoModel(VideoProviderSeedance))
	dummyTask := &VideoTask{
		Provider: seedanceAccount.Provider,
		Model:    model,
		TaskType: p.TaskType,
		Prompt:   strings.TrimSpace(p.Prompt),
		Duration: defaultVideoDuration(p.Duration),
	}
	blockedReasons := seedanceSmokeGateBlockedReasons(seedanceAccount, dummyTask)
	if len(blockedReasons) > 0 {
		return nil, infraerrors.Forbidden("VIDEO_PRODUCTION_BLOCKED", "seedance production blocked by gate").WithMetadata(map[string]string{
			"provider":                     VideoProviderSeedance,
			"blocked_reasons":              strings.Join(blockedReasons, "; "),
			"real_provider_dispatch_count": "0",
			"production_authorized":        "true",
		})
	}

	p.ProviderAccountID = seedanceAccount.ID
	route := &videoRouteDecision{
		Account:  seedanceAccount,
		Strategy: "api-key-seedance-production",
		Reason:   "seedance production gate passed",
	}
	task, err := s.createTaskWithRoute(ctx, p, route, s.repo.CreateTask)
	if err != nil {
		return nil, err
	}

	if !task.CreationReplayed {
		_ = s.repo.AddTaskEvent(ctx, &VideoTaskEvent{
			VideoTaskID: task.ID,
			EventType:   "production_gate",
			Message:     "seedance production gate passed",
			Payload: map[string]any{
				"provider":                        VideoProviderSeedance,
				"gate_result":                     "passed",
				"production_authorized":           true,
				"planned_provider_dispatch_count": 1,
				"real_provider_dispatch_count":    0,
			},
		})
	}

	return task, nil
}

func (s *VideoGatewayService) GetAPIKeyMockOnlyTask(ctx context.Context, id, userID int64, isAdmin bool) (*VideoTask, []*VideoTaskEvent, error) {
	task, events, err := s.GetTask(ctx, id, userID, isAdmin)
	if err != nil {
		return nil, nil, err
	}
	if task.Provider != VideoProviderMock {
		return nil, nil, errVideoProviderBlockedMockOnly(task.Provider)
	}
	return task, events, nil
}

func (s *VideoGatewayService) CancelAPIKeyMockOnlyTask(ctx context.Context, id, userID int64, isAdmin bool) (*VideoTask, error) {
	task, _, err := s.GetAPIKeyMockOnlyTask(ctx, id, userID, isAdmin)
	if err != nil {
		return nil, err
	}
	if IsTerminalVideoStatus(task.Status) {
		if task.Status == VideoStatusCancelled {
			return task, nil
		}
		return nil, infraerrors.Conflict("VIDEO_TASK_NOT_CANCELABLE", fmt.Sprintf("video task is already %s and cannot be canceled", task.Status))
	}
	return s.CancelTask(ctx, id, userID, isAdmin)
}

func (s *VideoGatewayService) GetAPIKeyTrialTask(ctx context.Context, id, userID int64, isAdmin bool) (*VideoTask, []*VideoTaskEvent, error) {
	task, events, err := s.GetTask(ctx, id, userID, isAdmin)
	if err != nil {
		return nil, nil, err
	}
	if task.Provider != VideoProviderMock && task.Provider != VideoProviderSeedance {
		return nil, nil, errVideoProviderBlockedMockOnly(task.Provider)
	}
	return task, events, nil
}

func (s *VideoGatewayService) CancelAPIKeyTrialTask(ctx context.Context, id, userID int64, isAdmin bool) (*VideoTask, []*VideoTaskEvent, error) {
	task, events, err := s.GetAPIKeyTrialTask(ctx, id, userID, isAdmin)
	if err != nil {
		return nil, nil, err
	}
	if IsTerminalVideoStatus(task.Status) {
		if task.Status == VideoStatusCancelled {
			return task, events, nil
		}
		return nil, nil, infraerrors.Conflict("VIDEO_TASK_NOT_CANCELABLE", fmt.Sprintf("video task is already %s and cannot be canceled", task.Status))
	}
	cancelled, err := s.CancelTask(ctx, id, userID, isAdmin)
	if err != nil {
		return nil, nil, err
	}
	return cancelled, events, nil
}

func (s *VideoGatewayService) resolveAPIKeyMockOnlyRoute(ctx context.Context) (*VideoProviderAccount, error) {
	providers, err := s.ListProviderAccounts(ctx)
	if err != nil {
		return nil, err
	}
	var selected *VideoProviderAccount
	for _, provider := range providers {
		if provider == nil || provider.Provider != VideoProviderMock || !provider.RouteAvailable {
			continue
		}
		if selected == nil ||
			provider.Priority < selected.Priority ||
			(provider.Priority == selected.Priority && provider.ID < selected.ID) {
			selected = provider
		}
	}
	if selected == nil {
		return nil, ErrVideoMockUnavailable.WithMetadata(map[string]string{
			"provider": VideoProviderMock,
			"boundary": "api-key-video-mock-only",
		})
	}
	return selected, nil
}

func apiKeySyntheticVideoProvider(provider string) *VideoProviderAccount {
	return &VideoProviderAccount{
		Provider:           provider,
		DisplayName:        defaultVideoProviderDisplayName(provider),
		Enabled:            false,
		DefaultModel:       defaultVideoModel(provider),
		RateLimitPerMinute: defaultVideoRateLimit(0),
		KeyStatus:          videoKeyStatusDisabled,
		HealthStatus:       videoHealthStatusDisabled,
		DiagnosticType:     "mock-only",
		SuggestedAction:    "use provider=mock for this API-key video gateway version",
		RouteAvailable:     false,
	}
}

func errVideoProviderBlockedMockOnly(provider string) error {
	return infraerrors.Forbidden(
		"VIDEO_PROVIDER_DISABLED",
		fmt.Sprintf("provider %s is disabled in API-key mock-only video gateway; use provider=mock", provider),
	).WithMetadata(map[string]string{
		"provider":                     provider,
		"mock_only":                    "true",
		"real_provider_dispatch_count": "0",
	})
}

func (s *VideoGatewayService) CreateTask(ctx context.Context, p VideoTaskCreateParams) (*VideoTask, error) {
	if err := validateVideoTaskType(p.TaskType); err != nil {
		return nil, err
	}
	if strings.TrimSpace(p.Prompt) == "" {
		return nil, ErrVideoMissingPrompt
	}
	if task, found, err := s.replayExistingVideoTaskCreation(ctx, p); err != nil || found {
		return task, err
	}
	if p.SafeDemoOnly {
		mock, err := s.resolveAPIKeyMockOnlyRoute(ctx)
		if err != nil {
			return nil, err
		}
		p.ProviderAccountID = mock.ID
	}
	route, err := s.resolveVideoRoute(ctx, p.ProviderAccountID)
	if err != nil {
		return nil, err
	}
	account := route.Account
	if _, err := s.adapterFor(account.Provider); err != nil {
		return nil, err
	}
	if account.Provider == VideoProviderSeedance && p.RequireSeedanceProductionAuthorization && !seedanceProductionAuthorized(account) {
		return nil, infraerrors.Forbidden("VIDEO_PRODUCTION_NOT_AUTHORIZED", "seedance production is not authorized for this provider account").WithMetadata(map[string]string{
			"provider":              VideoProviderSeedance,
			"provider_account_id":   strconv.FormatInt(account.ID, 10),
			"production_authorized": "false",
			"reason":                "provider metadata production_authorized is not true",
		})
	}
	createFn := s.repo.CreateTask
	if p.EnforceRealProviderTrial {
		switch account.Provider {
		case VideoProviderMock:
			// allowed without trial reservation
		case VideoProviderSeedance:
			dummyTask := &VideoTask{
				Provider: account.Provider,
				Model:    firstNonEmptyVideo(strings.TrimSpace(p.Model), account.DefaultModel, defaultVideoModel(VideoProviderSeedance)),
				TaskType: p.TaskType,
				Prompt:   strings.TrimSpace(p.Prompt),
				Duration: defaultVideoDuration(p.Duration),
			}
			if blockedReasons := seedanceSmokeGateBlockedReasons(account, dummyTask); len(blockedReasons) > 0 {
				return nil, infraerrors.Forbidden("VIDEO_TRIAL_BLOCKED", "seedance trial blocked by gate").WithMetadata(map[string]string{
					"provider":        VideoProviderSeedance,
					"blocked_reasons": strings.Join(blockedReasons, "; "),
				})
			}
			createFn = func(ctx context.Context, task *VideoTask) error {
				reserved, err := s.repo.CreateDailyTrialTask(ctx, task, VideoProviderSeedance, task.CreatedBy, timezone.Today())
				if err != nil {
					return err
				}
				if !reserved {
					return ErrVideoTrialLimitExceeded.WithMetadata(map[string]string{
						"provider": VideoProviderSeedance,
					})
				}
				return nil
			}
		default:
			return nil, infraerrors.Forbidden("VIDEO_REAL_PROVIDER_BLOCKED", "real provider requires API key trial path").WithMetadata(map[string]string{
				"provider": account.Provider,
			})
		}
	}
	return s.createTaskWithRoute(ctx, p, route, createFn)
}

func (s *VideoGatewayService) createTaskWithRoute(ctx context.Context, p VideoTaskCreateParams, route *videoRouteDecision, create func(context.Context, *VideoTask) error) (*VideoTask, error) {
	if route == nil || route.Account == nil {
		return nil, ErrVideoProviderDisabled
	}
	account := route.Account
	model := firstNonEmptyVideo(strings.TrimSpace(p.Model), account.DefaultModel, defaultVideoModel(account.Provider))
	duration := defaultVideoDuration(p.Duration)
	resolution := firstNonEmptyVideo(strings.TrimSpace(p.Resolution), "720p")
	content, hasVideoInput, err := normalizeVideoTaskContent(p)
	if err != nil {
		return nil, err
	}
	if err := validateVideoGenerationContract(account.Provider, model, duration, resolution); err != nil {
		return nil, err
	}
	task := &VideoTask{
		APIKeyID:            p.APIKeyID,
		ProviderAccountID:   account.ID,
		ProviderAccountName: account.DisplayName,
		Provider:            account.Provider,
		Model:               model,
		TaskType:            p.TaskType,
		Prompt:              strings.TrimSpace(p.Prompt),
		NegativePrompt:      strings.TrimSpace(p.NegativePrompt),
		ReferenceImageURL:   strings.TrimSpace(p.ReferenceImageURL),
		ReferenceVideoURL:   strings.TrimSpace(p.ReferenceVideoURL),
		Content:             content,
		HasVideoInput:       hasVideoInput,
		AspectRatio:         firstNonEmptyVideo(strings.TrimSpace(p.AspectRatio), "16:9"),
		Duration:            duration,
		Resolution:          resolution,
		GenerateAudio:       p.GenerateAudio,
		Watermark:           p.Watermark,
		CameraFixed:         p.CameraFixed,
		ReturnLastFrame:     p.ReturnLastFrame,
		Status:              VideoStatusQueued,
		Version:             1,
		DispatchState:       VideoDispatchStatePending,
		SettlementStatus:    VideoSettlementStatusPending,
		ArchiveStatus:       VideoSideEffectStatusPending,
		CaptureStatus:       VideoSideEffectStatusPending,
		CreatedBy:           p.CreatedBy,
	}
	mockIdempotency := s.cfg != nil && s.cfg.ReliabilityCore.VideoEnabled && account.Provider == VideoProviderMock
	if mockIdempotency {
		if len(p.CreationKey) != 64 || len(p.CreationFingerprint) != 64 {
			return nil, ErrIdempotencyKeyInvalid
		}
		task.CreationKey = p.CreationKey
		task.CreationFingerprint = p.CreationFingerprint
	}
	// VA1 per-call budget gate: check affordability BEFORE persisting/dispatching.
	// Fail-closed — any error (insufficient budget or inability to determine it)
	// rejects the create with no task row and no provider call.
	if s.budget != nil {
		if err := s.budget.CheckBudget(ctx, task.CreatedBy, s.estimateVideoCost(task)); err != nil {
			return nil, err
		}
	}
	replayedCreation := false
	if s.reliabilityVideoCreationEnabled(account.Provider) {
		if s.creationRepo == nil {
			return nil, ErrBillingReservationBalanceUnavailable.WithCause(errors.New("video task creation repository is unavailable"))
		}
		if len(p.CreationKey) != 64 || len(p.CreationFingerprint) != 64 {
			return nil, ErrIdempotencyKeyInvalid
		}
		task.CreationKey = p.CreationKey
		task.CreationFingerprint = p.CreationFingerprint
		reservedAmountUSD, pricingSnapshot, err := s.videoTaskPricing().EstimatePrice(ctx, task)
		if err != nil {
			return nil, err
		}
		zero := MustUSD("0")
		if comparison, err := reservedAmountUSD.Compare(zero); err != nil || comparison <= 0 {
			if err != nil {
				return nil, err
			}
			return nil, ErrBillingReservationBalanceUnavailable.WithCause(errors.New("video price is unavailable"))
		}
		ttlHours := 6
		if s.cfg != nil && s.cfg.ReliabilityCore.ReservationTTLHours > 0 {
			ttlHours = s.cfg.ReliabilityCore.ReservationTTLHours
		}
		creationInput := VideoTaskCreationInput{
			Task:                 task,
			ReservedAmountUSD:    reservedAmountUSD,
			PricingSnapshot:      pricingSnapshot,
			ReservationExpiresAt: time.Now().UTC().Add(time.Duration(ttlHours) * time.Hour),
		}
		if p.EnforceRealProviderTrial {
			creationInput.DailyTrialProvider = account.Provider
			creationInput.DailyTrialDate = timezone.Today()
		}
		result, err := s.creationRepo.CreateWithReservation(ctx, creationInput)
		if err != nil {
			return nil, err
		}
		if result == nil || result.Task == nil {
			return nil, fmt.Errorf("create video task with reservation: empty result")
		}
		task = result.Task
		replayedCreation = result.Replayed
		task.CreationReplayed = result.Replayed
		if !result.Replayed && task.ReservationID != nil {
			RecordReliabilityMetricAdd("billing_reservation_active_total", 1, nil)
		}
	} else if err := create(ctx, task); err != nil {
		if mockIdempotency {
			if replayed, found, replayErr := s.replayExistingVideoTaskCreation(ctx, p); replayErr != nil || found {
				if found && replayed != nil {
					replayed.CreationReplayed = true
				}
				return replayed, replayErr
			}
		}
		var appErr *infraerrors.ApplicationError
		if errors.As(err, &appErr) {
			return nil, err
		}
		return nil, fmt.Errorf("create video task: %w", err)
	}
	if replayedCreation {
		return task, nil
	}
	_ = s.repo.AddTaskEvent(ctx, &VideoTaskEvent{
		VideoTaskID: task.ID,
		EventType:   "routed",
		Message:     "video task routed",
		Payload: map[string]any{
			"strategy":              route.Strategy,
			"reason":                route.Reason,
			"selected_account_id":   account.ID,
			"selected_account_name": account.DisplayName,
			"provider":              account.Provider,
			"skipped_accounts":      route.Skipped,
		},
	})
	_ = s.repo.AddTaskEvent(ctx, &VideoTaskEvent{
		VideoTaskID: task.ID,
		EventType:   "queued",
		Message:     "video task queued",
		Payload: map[string]any{
			"provider":            task.Provider,
			"model":               task.Model,
			"provider_account_id": task.ProviderAccountID,
			"route_strategy":      route.Strategy,
		},
	})
	return task, nil
}

func (s *VideoGatewayService) reliabilityVideoCreationEnabled(provider string) bool {
	return s != nil && s.cfg != nil && s.cfg.ReliabilityCore.VideoEnabled && provider != VideoProviderMock
}

func (s *VideoGatewayService) replayExistingVideoTaskCreation(ctx context.Context, p VideoTaskCreateParams) (*VideoTask, bool, error) {
	if s == nil || s.creationRepo == nil || p.CreationKey == "" {
		return nil, false, nil
	}
	replayRepo, ok := s.creationRepo.(VideoTaskCreationReplayRepository)
	if !ok {
		return nil, false, nil
	}
	result, found, err := replayRepo.ReplayExisting(ctx, VideoTaskCreationReplayInput{
		CreationKey:         p.CreationKey,
		CreationFingerprint: p.CreationFingerprint,
		CreatedBy:           p.CreatedBy,
		APIKeyID:            p.APIKeyID,
	})
	if err != nil || !found {
		return nil, false, err
	}
	if result == nil || result.Task == nil {
		return nil, false, fmt.Errorf("replay video task creation: empty result")
	}
	return result.Task, true, nil
}

func (s *VideoGatewayService) ListTasks(ctx context.Context, p VideoTaskListParams) ([]*VideoTask, int64, error) {
	if p.Page < 1 {
		p.Page = 1
	}
	if p.PageSize <= 0 {
		p.PageSize = videoDefaultPageSize
	}
	if p.PageSize > videoMaxPageSize {
		p.PageSize = videoMaxPageSize
	}
	if p.Status != "" {
		if err := validateVideoStatus(p.Status); err != nil {
			return nil, 0, err
		}
	}
	if p.Provider != "" {
		if err := validateVideoProvider(p.Provider); err != nil {
			return nil, 0, err
		}
	}
	return s.repo.ListTasks(ctx, p)
}

func (s *VideoGatewayService) GetTask(ctx context.Context, id, userID int64, isAdmin bool) (*VideoTask, []*VideoTaskEvent, error) {
	task, err := s.repo.GetTask(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	if !isAdmin && task.CreatedBy != userID {
		return nil, nil, ErrVideoTaskNotFound
	}
	events, err := s.repo.ListTaskEvents(ctx, id, 200)
	if err != nil {
		return nil, nil, fmt.Errorf("list video task events: %w", err)
	}
	return task, events, nil
}

func (s *VideoGatewayService) CancelTask(ctx context.Context, id, userID int64, isAdmin bool) (*VideoTask, error) {
	task, err := s.repo.GetTask(ctx, id)
	if err != nil {
		return nil, err
	}
	if !isAdmin && task.CreatedBy != userID {
		return nil, ErrVideoTaskNotFound
	}
	if IsTerminalVideoStatus(task.Status) {
		return task, nil
	}
	expectedVersion := task.Version
	var finalizer *VideoTaskFinalizer
	if s.videoReliabilityCoreEnabled() {
		finalizer = s.videoTaskFinalizer()
		if finalizer == nil {
			return nil, fmt.Errorf("cancel video task: atomic finalizer is unavailable")
		}
	}
	account, err := s.repo.GetProviderAccount(ctx, task.ProviderAccountID)
	if err != nil {
		return nil, err
	}
	s.decryptProviderKey(account)
	adapter, err := s.adapterFor(task.Provider)
	if err != nil {
		return nil, err
	}
	result, err := adapter.CancelTask(ctx, account, task)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, fmt.Errorf("cancel video task: provider returned no result")
	}
	if s.videoReliabilityCoreEnabled() {
		now := time.Now().UTC()
		zero := MustUSD("0")
		finalized, err := finalizer.Finalize(ctx, VideoTaskFinalizationInput{
			TaskID:               task.ID,
			ExpectedVersion:      expectedVersion,
			TerminalStatus:       VideoStatusCancelled,
			ProviderResultURL:    result.ResultURL,
			ProviderErrorMessage: result.ErrorMessage,
			ProviderPayload:      result.Payload,
			ActualDuration:       result.ActualDuration,
			ActualResolution:     result.ActualResolution,
			ActualTokens:         result.UsageTotalTokens,
			LastFrameURL:         result.LastFrameURL,
			PollCount:            task.PollCount,
			ActualCostUSD:        zero,
			PricingSnapshot: PricingSnapshot{
				AmountOriginal: zero,
				ExchangeRate:   "1.0000000000",
				PricingSource:  PricingSourceFallback,
				PricingVersion: "reliability-core-v1",
			},
			CompletedAt: now,
		})
		if err != nil {
			return nil, fmt.Errorf("cancel video task finalization: %w", err)
		}
		task.CompletedAt = &now
		applyVideoTaskFinalizationResult(task, finalized)
		return task, nil
	}
	task.Status = firstNonEmptyVideo(result.Status, VideoStatusCancelled)
	now := time.Now().UTC()
	task.CompletedAt = &now
	if err := s.repo.UpdateTask(ctx, task); err != nil {
		return nil, fmt.Errorf("cancel video task: %w", err)
	}
	_ = s.repo.AddTaskEvent(ctx, &VideoTaskEvent{
		VideoTaskID: task.ID,
		EventType:   "cancelled",
		Message:     "video task cancelled",
		Payload:     result.Payload,
	})
	s.applyVideoBillingMetadata(task)
	_ = s.repo.InsertUsageLog(ctx, task)
	return task, nil
}

func (s *VideoGatewayService) Dashboard(ctx context.Context) (*VideoDashboard, error) {
	since := timezone.Today()
	statusCounts, err := s.repo.CountTasksSince(ctx, since)
	if err != nil {
		return nil, fmt.Errorf("count video tasks: %w", err)
	}
	providers, err := s.repo.ListProviderAccounts(ctx)
	if err != nil {
		return nil, fmt.Errorf("list video providers: %w", err)
	}
	providerStats, err := s.repo.ProviderAccountTaskStatsSince(ctx, since)
	if err != nil {
		return nil, fmt.Errorf("count video provider account tasks: %w", err)
	}
	failures, err := s.repo.ListRecentTasksByStatus(ctx, VideoStatusFailed, videoDashboardLimit)
	if err != nil {
		return nil, fmt.Errorf("list recent failed video tasks: %w", err)
	}
	successes, err := s.repo.ListRecentTasksByStatus(ctx, VideoStatusSucceeded, videoDashboardLimit)
	if err != nil {
		return nil, fmt.Errorf("list recent successful video tasks: %w", err)
	}
	usage, err := s.repo.UsageSummarySince(ctx, since.AddDate(0, 0, -7))
	if err != nil {
		return nil, fmt.Errorf("summarize video usage: %w", err)
	}
	total := sumStatusCounts(statusCounts)
	success := statusCounts[VideoStatusSucceeded]
	successRate := 0.0
	if total > 0 {
		successRate = float64(success) * 100 / float64(total)
	}
	providerStatus := make([]VideoProviderStatus, 0, len(providers))
	for _, p := range providers {
		s.decorateProviderForResponse(p, providerStats[p.ID])
		providerStatus = append(providerStatus, VideoProviderStatus{
			Provider:         p.Provider,
			DisplayName:      p.DisplayName,
			Enabled:          p.Enabled,
			APIKeyConfigured: p.APIKeyConfigured,
			MaskedKey:        p.MaskedKey,
			DefaultModel:     p.DefaultModel,
			KeyStatus:        p.KeyStatus,
			HealthStatus:     p.HealthStatus,
			DiagnosticType:   p.DiagnosticType,
			SuggestedAction:  p.SuggestedAction,
			RouteAvailable:   p.RouteAvailable,
			RouteSkipReason:  p.RouteSkipReason,
			Priority:         p.Priority,
			CurrentInflight:  p.CurrentInflight,
			LastError:        p.LastError,
			LastTestAt:       p.LastTestAt,
			UpdatedAt:        p.UpdatedAt,
			TodayTasks:       p.TodayTasks,
			RunningTasks:     p.CurrentInflight,
			FailedTasks:      p.TodayFailures,
		})
	}
	return &VideoDashboard{
		TodayTasks:        total,
		SuccessRate:       successRate,
		FailedTasks:       statusCounts[VideoStatusFailed],
		QueuedTasks:       statusCounts[VideoStatusQueued],
		RunningTasks:      statusCounts[VideoStatusRunning] + statusCounts[VideoStatusSubmitted],
		ProviderStatus:    providerStatus,
		HealthDiagnostics: buildVideoHealthDiagnostics(providers),
		RecentFailures:    failures,
		RecentSuccesses:   successes,
		UsageOverview:     usage,
	}, nil
}

func (s *VideoGatewayService) providerRuntimeStats(ctx context.Context) (map[int64]VideoProviderRuntimeStats, error) {
	since := timezone.Today()
	stats, err := s.repo.ProviderAccountTaskStatsSince(ctx, since)
	if err != nil {
		return nil, fmt.Errorf("count video provider account tasks: %w", err)
	}
	return stats, nil
}

func (s *VideoGatewayService) resolveVideoRoute(ctx context.Context, providerAccountID int64) (*videoRouteDecision, error) {
	stats, err := s.providerRuntimeStats(ctx)
	if err != nil {
		return nil, err
	}
	if providerAccountID > 0 {
		account, err := s.repo.GetProviderAccount(ctx, providerAccountID)
		if err != nil {
			return nil, err
		}
		s.decorateProviderForResponse(account, stats[account.ID])
		if !account.RouteAvailable {
			return nil, ErrVideoProviderDisabled.WithMetadata(map[string]string{
				"provider_account_id": fmt.Sprintf("%d", account.ID),
				"reason":              account.RouteSkipReason,
			})
		}
		return &videoRouteDecision{
			Account:  account,
			Strategy: "explicit",
			Reason:   "指定账号可用",
		}, nil
	}

	accounts, err := s.repo.ListProviderAccounts(ctx)
	if err != nil {
		return nil, fmt.Errorf("list video providers: %w", err)
	}
	for _, account := range accounts {
		s.decorateProviderForResponse(account, stats[account.ID])
	}
	route := selectLeastInflightVideoRoute(accounts)
	if route == nil {
		return nil, ErrVideoProviderDisabled.WithMetadata(map[string]string{
			"reason": "没有可用的视频 API 账号",
		})
	}
	return route, nil
}

func selectLeastInflightVideoRoute(accounts []*VideoProviderAccount) *videoRouteDecision {
	candidates := make([]*VideoProviderAccount, 0, len(accounts))
	skipped := make([]videoRouteSkip, 0)
	for _, account := range accounts {
		if account == nil {
			continue
		}
		if !account.RouteAvailable {
			reason := strings.TrimSpace(account.RouteSkipReason)
			if reason == "" {
				reason = "账号不可调度"
			}
			skipped = append(skipped, videoRouteSkip{
				ID:          account.ID,
				DisplayName: account.DisplayName,
				Provider:    account.Provider,
				Reason:      reason,
			})
			continue
		}
		candidates = append(candidates, account)
	}
	if len(candidates) == 0 {
		return nil
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		left, right := candidates[i], candidates[j]
		if left.CurrentInflight != right.CurrentInflight {
			return left.CurrentInflight < right.CurrentInflight
		}
		if left.TodayFailures != right.TodayFailures {
			return left.TodayFailures < right.TodayFailures
		}
		if left.Priority != right.Priority {
			return left.Priority < right.Priority
		}
		return left.ID < right.ID
	})
	selected := candidates[0]
	return &videoRouteDecision{
		Account:  selected,
		Strategy: VideoRouteStrategyLeastInflight,
		Reason: fmt.Sprintf(
			"选择当前处理中 %d、今日失败 %d、优先级 %d 的可用账号",
			selected.CurrentInflight,
			selected.TodayFailures,
			selected.Priority,
		),
		Skipped: skipped,
	}
}

func (s *VideoGatewayService) decorateProviderForResponse(account *VideoProviderAccount, stats VideoProviderRuntimeStats) {
	s.prepareProviderForResponse(account)
	if account == nil {
		return
	}
	account.TodayTasks = stats.TodayTasks
	account.CurrentInflight = stats.CurrentInflight
	account.TodayFailures = stats.TodayFailures
	account.LastError = firstNonEmptyVideo(videoMetadataString(account.Metadata, "last_error"), stats.LastError)
	if stats.LastErrorAt != nil {
		lastErrorAt := *stats.LastErrorAt
		account.LastTestAt = &lastErrorAt
	}
	if lastTestAt := videoMetadataTime(account.Metadata, "last_test_at"); lastTestAt != nil {
		account.LastTestAt = lastTestAt
	}
	account.Priority = videoMetadataInt(account.Metadata, "priority", 50)
	account.KeyStatus = deriveVideoKeyStatus(account)
	account.HealthStatus = deriveVideoHealthStatus(account)
	account.DiagnosticType = firstNonEmptyVideo(
		videoMetadataString(account.Metadata, "diagnostic_type"),
		videoProviderDiagnosticType(account.KeyStatus, account.HealthStatus),
	)
	account.SuggestedAction = firstNonEmptyVideo(
		videoMetadataString(account.Metadata, "suggested_action"),
		videoProviderSuggestedAction(account.DiagnosticType, account.KeyStatus, account.HealthStatus),
	)
	account.RouteAvailable, account.RouteSkipReason = videoProviderRouteAvailability(account)
	if account.Provider == VideoProviderMock && account.KeyStatus == videoKeyStatusNormal {
		account.APIKeyConfigured = true
	}
}

func deriveVideoKeyStatus(account *VideoProviderAccount) string {
	if account == nil {
		return videoKeyStatusMissing
	}
	if !account.Enabled {
		return firstNonEmptyVideo(videoMetadataString(account.Metadata, "key_status"), videoKeyStatusDisabled)
	}
	status := firstNonEmptyVideo(videoMetadataString(account.Metadata, "key_status"))
	if status != "" {
		return status
	}
	if account.Provider == VideoProviderMock || account.APIKeyConfigured {
		return videoKeyStatusNormal
	}
	return videoKeyStatusMissing
}

func deriveVideoHealthStatus(account *VideoProviderAccount) string {
	if account == nil {
		return videoHealthStatusUnknown
	}
	if !account.Enabled {
		return firstNonEmptyVideo(videoMetadataString(account.Metadata, "health_status"), videoHealthStatusDisabled)
	}
	status := firstNonEmptyVideo(videoMetadataString(account.Metadata, "health_status"))
	if status != "" {
		return status
	}
	switch account.KeyStatus {
	case videoKeyStatusNormal:
		return videoHealthStatusHealthy
	case videoKeyStatusMissing:
		return videoHealthStatusNeedsKey
	case videoKeyStatusAuthFailed:
		return videoHealthStatusAuthFailed
	case videoKeyStatusRateLimited:
		return videoHealthStatusRateLimited
	case videoKeyStatusQuotaExhausted:
		return videoHealthStatusQuotaExhausted
	default:
		return videoHealthStatusUnknown
	}
}

func videoProviderRouteAvailability(account *VideoProviderAccount) (bool, string) {
	if account == nil {
		return false, "账号不存在"
	}
	if !account.Enabled {
		return false, "账号已停用"
	}
	if account.APIKeyDecryptFailed {
		return false, "凭证解密失败"
	}
	switch account.KeyStatus {
	case videoKeyStatusMissing:
		return false, "凭证未配置"
	case videoKeyStatusDisabled:
		return false, "账号已停用"
	case videoKeyStatusAuthFailed:
		return false, "鉴权失败"
	case videoKeyStatusRateLimited:
		return false, "触发限流"
	case videoKeyStatusQuotaExhausted:
		return false, "额度不足"
	}
	switch account.HealthStatus {
	case videoHealthStatusHealthy:
		return true, ""
	case videoHealthStatusNeedsKey:
		return false, "凭证未配置"
	case videoHealthStatusDisabled:
		return false, "账号已停用"
	case videoHealthStatusAuthFailed:
		return false, "鉴权失败"
	case videoHealthStatusRateLimited:
		return false, "触发限流"
	case videoHealthStatusQuotaExhausted:
		return false, "额度不足"
	case videoHealthStatusTimeout:
		return false, "上游超时"
	case videoHealthStatusModerationFailed:
		return false, "内容审核失败"
	case videoHealthStatusModelUnavailable:
		return false, "模型不可用"
	case videoHealthStatusUnknown:
		return false, "未知异常"
	default:
		return true, ""
	}
}

func videoProviderDiagnosticType(keyStatus, healthStatus string) string {
	switch keyStatus {
	case videoKeyStatusMissing:
		return "凭证未配置"
	case videoKeyStatusAuthFailed:
		return "鉴权失败"
	case videoKeyStatusQuotaExhausted:
		return "额度不足"
	case videoKeyStatusRateLimited:
		return "触发限流"
	}
	switch healthStatus {
	case videoHealthStatusNeedsKey:
		return "凭证未配置"
	case videoHealthStatusAuthFailed:
		return "鉴权失败"
	case videoHealthStatusQuotaExhausted:
		return "额度不足"
	case videoHealthStatusRateLimited:
		return "触发限流"
	case videoHealthStatusTimeout:
		return "上游超时"
	case videoHealthStatusModerationFailed:
		return "内容审核失败"
	case videoHealthStatusModelUnavailable:
		return "模型不可用"
	case videoHealthStatusUnknown:
		return "未知异常"
	}
	return ""
}

func videoProviderSuggestedAction(diagnosticType, keyStatus, healthStatus string) string {
	switch firstNonEmptyVideo(diagnosticType, videoProviderDiagnosticType(keyStatus, healthStatus)) {
	case "凭证未配置":
		return "请配置真实凭证后启用真实调用"
	case "鉴权失败":
		return "请检查凭证是否过期或填错"
	case "额度不足":
		return "请确认上游账号余额或额度"
	case "触发限流":
		return "请降低并发或增加账号"
	case "上游超时":
		return "稍后重试或切换通道"
	case "内容审核失败":
		return "调整提示词或素材后重试"
	case "模型不可用":
		return "切换模型或确认上游模型可用"
	case "未知异常":
		return "查看任务详情和事件时间线"
	default:
		return ""
	}
}

func buildVideoHealthDiagnostics(providers []*VideoProviderAccount) []VideoHealthDiagnostic {
	out := make([]VideoHealthDiagnostic, 0, len(providers))
	for _, provider := range providers {
		if provider == nil {
			continue
		}
		exceptionType := provider.DiagnosticType
		if strings.TrimSpace(exceptionType) == "" {
			exceptionType = "正常"
		}
		status := "正常"
		if !provider.RouteAvailable {
			status = "需处理"
		}
		out = append(out, VideoHealthDiagnostic{
			Provider:        provider.Provider,
			DisplayName:     defaultVideoProviderDisplayName(provider.Provider),
			RouteAccount:    provider.DisplayName,
			KeyStatus:       provider.KeyStatus,
			LastTestAt:      provider.LastTestAt,
			ExceptionType:   exceptionType,
			ImpactTasks:     provider.TodayFailures,
			RecentError:     provider.LastError,
			SuggestedAction: provider.SuggestedAction,
			Status:          status,
		})
	}
	return out
}

func (s *VideoGatewayService) applyProviderAPIKey(account *VideoProviderAccount, plain string) error {
	plain = strings.TrimSpace(plain)
	encrypted, err := s.encryptor.Encrypt(plain)
	if err != nil {
		return fmt.Errorf("encrypt video provider key: %w", err)
	}
	account.EncryptedAPIKey = encrypted
	account.MaskedKey = MaskVideoAPIKey(plain)
	return nil
}

func (s *VideoGatewayService) prepareProviderForResponse(account *VideoProviderAccount) {
	if account == nil {
		return
	}
	account.APIKeyConfigured = strings.TrimSpace(account.EncryptedAPIKey) != ""
	account.PlainAPIKey = ""
}

func videoMetadataString(metadata map[string]any, key string) string {
	if metadata == nil {
		return ""
	}
	value, ok := metadata[key]
	if !ok || value == nil {
		return ""
	}
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	case fmt.Stringer:
		return strings.TrimSpace(v.String())
	default:
		return strings.TrimSpace(fmt.Sprint(v))
	}
}

func videoMetadataInt(metadata map[string]any, key string, fallback int) int {
	if metadata == nil {
		return fallback
	}
	value, ok := metadata[key]
	if !ok || value == nil {
		return fallback
	}
	switch v := value.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case json.Number:
		if parsed, err := v.Int64(); err == nil {
			return int(parsed)
		}
	case string:
		var parsed int
		if _, err := fmt.Sscanf(strings.TrimSpace(v), "%d", &parsed); err == nil {
			return parsed
		}
	}
	return fallback
}

func videoMetadataTime(metadata map[string]any, key string) *time.Time {
	raw := videoMetadataString(metadata, key)
	if raw == "" {
		return nil
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05"} {
		if parsed, err := time.Parse(layout, raw); err == nil {
			return &parsed
		}
	}
	return nil
}

func (s *VideoGatewayService) decryptProviderKey(account *VideoProviderAccount) {
	if account == nil || strings.TrimSpace(account.EncryptedAPIKey) == "" {
		if account != nil {
			account.APIKeyConfigured = false
		}
		return
	}
	account.APIKeyConfigured = true
	plain, err := s.encryptor.Decrypt(account.EncryptedAPIKey)
	if err != nil {
		slog.Warn("video_gateway: decrypt provider key failed", "provider_id", account.ID, "provider", account.Provider, "error", err)
		account.APIKeyDecryptFailed = true
		account.PlainAPIKey = ""
		return
	}
	account.PlainAPIKey = plain
}

func (s *VideoGatewayService) adapterFor(provider string) (VideoAdapter, error) {
	adapter, ok := s.adapters[provider]
	if !ok {
		return nil, ErrVideoInvalidProvider
	}
	return adapter, nil
}

func MaskVideoAPIKey(plain string) string {
	plain = strings.TrimSpace(plain)
	if plain == "" {
		return ""
	}
	if len(plain) <= 8 {
		return "***"
	}
	return plain[:4] + "***" + plain[len(plain)-4:]
}

func validateVideoProvider(provider string) error {
	switch strings.TrimSpace(provider) {
	case VideoProviderMock, VideoProviderSeedance, VideoProviderKling:
		return nil
	default:
		return ErrVideoInvalidProvider
	}
}

func validateVideoTaskType(taskType string) error {
	switch strings.TrimSpace(taskType) {
	case VideoTaskTypeTextToVideo, VideoTaskTypeImageToVideo, VideoTaskTypeReferenceToVideo:
		return nil
	default:
		return ErrVideoInvalidTaskType
	}
}

func validateVideoStatus(status string) error {
	switch strings.TrimSpace(status) {
	case VideoStatusQueued, VideoStatusSubmitted, VideoStatusRunning, VideoStatusSucceeded, VideoStatusFailed, VideoStatusCancelled:
		return nil
	default:
		return ErrVideoInvalidStatus
	}
}

func defaultVideoProviderDisplayName(provider string) string {
	switch provider {
	case VideoProviderMock:
		return "演示通道"
	case VideoProviderSeedance:
		return "Seedance 2.0"
	case VideoProviderKling:
		return "Kling"
	default:
		return provider
	}
}

func defaultVideoModel(provider string) string {
	switch provider {
	case VideoProviderMock:
		return "mock-video-v1"
	case VideoProviderSeedance:
		return "seedance-2-0-pro"
	case VideoProviderKling:
		return "kling-v1"
	default:
		return "video-model"
	}
}

func defaultVideoRateLimit(v int) int {
	if v <= 0 {
		return 60
	}
	return v
}

func defaultVideoDuration(v int) int {
	if v == -1 {
		return -1
	}
	if v <= 0 {
		return 5
	}
	return v
}

func firstNonEmptyVideo(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func sumStatusCounts(counts map[string]int64) int64 {
	var total int64
	for _, v := range counts {
		total += v
	}
	return total
}
