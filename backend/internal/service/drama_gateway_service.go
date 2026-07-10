package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	DramaTaskIDPrefix          = "drama_task_"
	DramaExportSchemaVersion   = "ai_drama_skill_export_v1"
	dramaDefaultExportIDPrefix = "skill_export_"
)

type DramaReferenceAsset struct {
	AssetType         string `json:"asset_type"`
	AssetID           string `json:"asset_id"`
	SelectedTimeRange string `json:"selected_time_range"`
}

type DramaEngineCapabilityRequest struct {
	SupportsRealPerson      bool `json:"supports_real_person"`
	SupportsImageToVideo    bool `json:"supports_image_to_video"`
	SupportsMotionControl   bool `json:"supports_motion_control"`
	SupportsLipsync         bool `json:"supports_lipsync"`
	SupportsNativeAudio     bool `json:"supports_native_audio"`
	SupportsGlobalReference bool `json:"supports_global_reference"`
	SupportsAnimeDrama      bool `json:"supports_anime_drama"`
}

type DramaTaskCreateParams struct {
	EmployeeAlias               string                       `json:"employee_alias"`
	APIClientID                 string                       `json:"api_client_id"`
	ProjectID                   string                       `json:"project_id"`
	DramaType                   string                       `json:"drama_type"`
	Genre                       string                       `json:"genre"`
	EpisodeNo                   int                          `json:"episode_no"`
	SceneType                   string                       `json:"scene_type"`
	ShotRole                    string                       `json:"shot_role"`
	DramaticGoal                string                       `json:"dramatic_goal"`
	ReferenceAssets             []DramaReferenceAsset        `json:"reference_assets"`
	Prompt                      string                       `json:"prompt"`
	PromptStructure             map[string]any               `json:"prompt_structure"`
	RequestedEngineCapabilities DramaEngineCapabilityRequest `json:"requested_engine_capabilities"`
	DurationSeconds             int                          `json:"duration_seconds"`
	AspectRatio                 string                       `json:"aspect_ratio"`
	CreatedBy                   int64                        `json:"-"`
}

type DramaProviderRecommendParams struct {
	DramaType                   string                       `json:"drama_type"`
	SceneType                   string                       `json:"scene_type"`
	ShotRole                    string                       `json:"shot_role"`
	ReferenceAssetType          string                       `json:"reference_asset_type"`
	RequestedEngineCapabilities DramaEngineCapabilityRequest `json:"requested_engine_capabilities"`
	DurationSeconds             int                          `json:"duration_seconds"`
	AspectRatio                 string                       `json:"aspect_ratio"`
}

type DramaEngineProfile struct {
	ID                         string   `json:"id"`
	ProviderCode               string   `json:"provider_code"`
	EngineFamily               string   `json:"engine_family"`
	ModelName                  string   `json:"model_name"`
	ModelVersion               string   `json:"model_version"`
	Mode                       string   `json:"mode"`
	SupportsTextToVideo        bool     `json:"supports_text_to_video"`
	SupportsImageToVideo       bool     `json:"supports_image_to_video"`
	SupportsFirstLastFrame     bool     `json:"supports_first_last_frame"`
	SupportsGlobalReference    bool     `json:"supports_global_reference"`
	SupportsReferenceImages    bool     `json:"supports_reference_images"`
	SupportsReferenceVideo     bool     `json:"supports_reference_video"`
	SupportsReferenceAudio     bool     `json:"supports_reference_audio"`
	SupportsMotionControl      bool     `json:"supports_motion_control"`
	SupportsNativeAudio        bool     `json:"supports_native_audio"`
	SupportsLipsync            bool     `json:"supports_lipsync"`
	SupportsRealPerson         bool     `json:"supports_real_person"`
	SupportsAnimeDrama         bool     `json:"supports_anime_drama"`
	SupportsShortDrama         bool     `json:"supports_short_drama"`
	SupportsMiddleDrama        bool     `json:"supports_middle_drama"`
	SupportsMultiCharacter     bool     `json:"supports_multi_character"`
	SupportsDialogue           bool     `json:"supports_dialogue"`
	SupportsCameraControl      bool     `json:"supports_camera_control"`
	MaxDurationSeconds         int      `json:"max_duration_seconds"`
	AspectRatios               []string `json:"aspect_ratios"`
	BestForSceneTypes          []string `json:"best_for_scene_types"`
	WeakForSceneTypes          []string `json:"weak_for_scene_types"`
	RecommendedPromptStructure []string `json:"recommended_prompt_structure"`
	CommonFailureModes         []string `json:"common_failure_modes"`
	CredentialRequired         bool     `json:"credential_required"`
	RealProviderVerified       bool     `json:"real_provider_verified"`
	InternalSafeModeOnly       bool     `json:"internal_safe_mode_only"`
	SourceNote                 string   `json:"source_note"`
	LastVerifiedAt             string   `json:"last_verified_at"`
}

type DramaProviderSkip struct {
	Provider string `json:"provider"`
	Reason   string `json:"reason"`
	Detail   string `json:"detail,omitempty"`
}

type DramaProviderRecommendation struct {
	SelectedProvider    string              `json:"selected_provider"`
	SelectedModel       string              `json:"selected_model"`
	SelectedMode        string              `json:"selected_mode"`
	EngineProfileID     string              `json:"engine_profile_id"`
	RoutingReason       string              `json:"routing_reason"`
	SkippedProviders    []DramaProviderSkip `json:"skipped_providers"`
	FallbackReady       bool                `json:"fallback_ready"`
	SafeDemoMode        bool                `json:"safe_demo_mode"`
	NeedsRealValidation bool                `json:"needs_real_validation"`
}

type DramaTaskContract struct {
	TaskID                 string                       `json:"task_id"`
	Status                 string                       `json:"status"`
	SelectedProvider       string                       `json:"selected_provider"`
	SelectedModel          string                       `json:"selected_model"`
	SelectedMode           string                       `json:"selected_mode"`
	RoutingReason          string                       `json:"routing_reason"`
	SkippedProviders       []DramaProviderSkip          `json:"skipped_providers"`
	ResultURLOrPlaceholder string                       `json:"result_url_or_placeholder"`
	RoutingTrace           []map[string]any             `json:"routing_trace"`
	PromptArtifactID       string                       `json:"prompt_artifact_id"`
	ShotDecisionID         string                       `json:"shot_decision_id"`
	SkillEventID           string                       `json:"skill_event_id"`
	SkillEventCreated      bool                         `json:"skill_event_created"`
	EmployeeAlias          string                       `json:"employee_alias"`
	APIClientID            string                       `json:"api_client_id"`
	ProjectID              string                       `json:"project_id"`
	DramaType              string                       `json:"drama_type"`
	Genre                  string                       `json:"genre"`
	SceneType              string                       `json:"scene_type"`
	ShotRole               string                       `json:"shot_role"`
	DramaticGoal           string                       `json:"dramatic_goal"`
	PromptStructure        map[string]any               `json:"prompt_structure,omitempty"`
	RequestedCapabilities  DramaEngineCapabilityRequest `json:"requested_engine_capabilities"`
	DurationSeconds        int                          `json:"duration_seconds"`
	AspectRatio            string                       `json:"aspect_ratio"`
	CreatedAt              string                       `json:"created_at"`
	UpdatedAt              string                       `json:"updated_at"`
}

type DramaShotDecisionParams struct {
	TaskID               string         `json:"task_id"`
	EmployeeAlias        string         `json:"employee_alias"`
	ProjectID            string         `json:"project_id"`
	ReferenceAssetID     string         `json:"reference_asset_id"`
	SelectedTimeRange    string         `json:"selected_time_range"`
	ShotRole             string         `json:"shot_role"`
	WhySelected          string         `json:"why_selected"`
	VisualElements       map[string]any `json:"visual_elements_json"`
	KeptOrReplaced       string         `json:"kept_or_replaced"`
	EngineRecommendation string         `json:"engine_recommendation"`
	EmployeeActualEngine string         `json:"employee_actual_engine"`
	EmployeeNote         string         `json:"employee_note"`
	CreatedBy            int64          `json:"-"`
}

type DramaPromptArtifactParams struct {
	TaskID            string           `json:"task_id"`
	EmployeeAlias     string           `json:"employee_alias"`
	EngineProfileID   string           `json:"engine_profile_id"`
	PromptFull        string           `json:"prompt_full"`
	PromptStructure   map[string]any   `json:"prompt_structure_json"`
	NegativePrompt    string           `json:"negative_prompt"`
	PromptVersion     string           `json:"prompt_version"`
	ManualEditHistory []map[string]any `json:"manual_edit_history_json"`
	ReuseStatus       string           `json:"reuse_status"`
	QualityScore      int              `json:"quality_score"`
	ManagerNote       string           `json:"manager_note"`
	CreatedBy         int64            `json:"-"`
}

type DramaEventRecord struct {
	ID        string         `json:"id"`
	TaskID    string         `json:"task_id"`
	EventType string         `json:"event_type"`
	Payload   map[string]any `json:"payload_json"`
	CreatedAt string         `json:"created_at"`
}

type DramaProviderPoolItem struct {
	ProviderAccountStatus string   `json:"provider_account_status"`
	ProviderCode          string   `json:"provider_code"`
	DisplayName           string   `json:"display_name"`
	CredentialConfigured  bool     `json:"credential_configured"`
	RealProviderVerified  bool     `json:"real_provider_verified"`
	LastErrorType         string   `json:"last_error_type"`
	CooldownUntil         string   `json:"cooldown_until"`
	FailureCountToday     int64    `json:"failure_count_today"`
	FallbackReady         bool     `json:"fallback_ready"`
	SuitableSceneTypes    []string `json:"suitable_scene_types"`
	UnsuitableSceneTypes  []string `json:"unsuitable_scene_types"`
	SupportedModes        []string `json:"supported_modes"`
	RoutingReason         string   `json:"routing_reason"`
	SkippedReason         string   `json:"skipped_reason"`
	ConcurrencyLimit      int      `json:"concurrency_limit"`
	CurrentInflight       int64    `json:"current_inflight"`
	DailyQuotaEstimate    int      `json:"daily_quota_estimate"`
	CostEstimate          string   `json:"cost_estimate"`
	LatencyEstimate       string   `json:"latency_estimate"`
	SafeDemoMode          bool     `json:"safe_demo_mode"`
}

type DramaRoutingEventSummary struct {
	TaskID           string              `json:"task_id"`
	EventID          int64               `json:"event_id"`
	EventType        string              `json:"event_type"`
	EmployeeAlias    string              `json:"employee_alias"`
	APIClientID      string              `json:"api_client_id"`
	DramaType        string              `json:"drama_type"`
	SceneType        string              `json:"scene_type"`
	SelectedProvider string              `json:"selected_provider"`
	SelectedModel    string              `json:"selected_model"`
	SelectedMode     string              `json:"selected_mode"`
	RoutingReason    string              `json:"routing_reason"`
	SkippedProviders []DramaProviderSkip `json:"skipped_providers"`
	CreatedAt        string              `json:"created_at"`
}

type DramaSkillCard struct {
	EmployeeAlias             string         `json:"employee_alias"`
	ScopeType                 string         `json:"scope_type"`
	DramaTypes                []string       `json:"drama_types"`
	SceneTypes                []string       `json:"scene_types"`
	EngineModes               []string       `json:"engine_modes"`
	TotalTasks                int            `json:"total_tasks"`
	StrongPatterns            []string       `json:"strong_patterns_json"`
	PromptPatterns            []string       `json:"prompt_patterns_json"`
	ShotSelectionPatterns     []string       `json:"shot_selection_patterns_json"`
	EngineSelectionPatterns   []string       `json:"engine_selection_patterns_json"`
	FailurePatterns           []string       `json:"failure_patterns_json"`
	AutomationRecommendations []string       `json:"automation_recommendations_json"`
	GeneratedByModel          bool           `json:"generated_by_model"`
	ReviewStatus              string         `json:"approval_status"`
	Version                   string         `json:"version"`
	UpdatedAt                 string         `json:"updated_at"`
	Counts                    map[string]int `json:"counts"`
}

type DramaSkillAnalysisExportRequest struct {
	ExportType     string `json:"export_type"`
	TimeRangeStart string `json:"time_range_start"`
	TimeRangeEnd   string `json:"time_range_end"`
	TargetAIModel  string `json:"target_ai_model"`
}

type DramaSkillAnalysisExport struct {
	ID             string            `json:"id"`
	ExportType     string            `json:"export_type"`
	TargetAIModel  string            `json:"target_ai_model"`
	Anonymized     bool              `json:"anonymized"`
	SchemaVersion  string            `json:"schema_version"`
	ExportJSON     map[string]any    `json:"export_json"`
	AnalysisPrompt string            `json:"analysis_prompt"`
	Prompts        map[string]string `json:"analysis_prompts"`
	CreatedAt      string            `json:"created_at"`
}

func (s *VideoGatewayService) DramaEngineCapabilityMatrix() []DramaEngineProfile {
	return dramaEngineProfiles()
}

func (s *VideoGatewayService) RecommendDramaProvider(ctx context.Context, p DramaProviderRecommendParams) (*DramaProviderRecommendation, error) {
	family := recommendedDramaFamily(p)
	mode := recommendedDramaMode(p)
	selectedProfileID := family + "_safe_demo"
	if family == VideoProviderKling && mode == VideoTaskTypeTextToVideo {
		mode = VideoTaskTypeImageToVideo
	}
	profile := dramaProfileByID(selectedProfileID)
	if profile == nil {
		profile = dramaProfileByID("safe_demo_provider")
	}
	skipped := s.dramaSkippedProviders(ctx, family)
	reason := dramaRoutingReason(p, family, mode)
	return &DramaProviderRecommendation{
		SelectedProvider:    profile.ProviderCode,
		SelectedModel:       profile.ModelName,
		SelectedMode:        profile.Mode,
		EngineProfileID:     profile.ID,
		RoutingReason:       reason,
		SkippedProviders:    skipped,
		FallbackReady:       true,
		SafeDemoMode:        profile.InternalSafeModeOnly,
		NeedsRealValidation: true,
	}, nil
}

func (s *VideoGatewayService) CreateDramaTask(ctx context.Context, p DramaTaskCreateParams) (*DramaTaskContract, error) {
	if strings.TrimSpace(p.Prompt) == "" {
		return nil, ErrVideoMissingPrompt
	}
	employeeAlias := firstNonEmptyVideo(strings.TrimSpace(p.EmployeeAlias), fmt.Sprintf("E%03d", p.CreatedBy))
	recommendation, err := s.RecommendDramaProvider(ctx, DramaProviderRecommendParams{
		DramaType:                   p.DramaType,
		SceneType:                   p.SceneType,
		ShotRole:                    p.ShotRole,
		ReferenceAssetType:          firstDramaReferenceAssetType(p.ReferenceAssets),
		RequestedEngineCapabilities: p.RequestedEngineCapabilities,
		DurationSeconds:             p.DurationSeconds,
		AspectRatio:                 p.AspectRatio,
	})
	if err != nil {
		return nil, err
	}
	task, err := s.CreateTask(ctx, VideoTaskCreateParams{
		TaskType:          dramaModeToVideoTaskType(recommendation.SelectedMode),
		Model:             recommendation.SelectedModel,
		Prompt:            p.Prompt,
		NegativePrompt:    safeStringFromMap(p.PromptStructure, "negative_prompt"),
		ReferenceImageURL: firstDramaReferencePlaceholder(p.ReferenceAssets, "reference_image"),
		ReferenceVideoURL: firstDramaReferencePlaceholder(p.ReferenceAssets, "reference_video"),
		AspectRatio:       firstNonEmptyVideo(strings.TrimSpace(p.AspectRatio), "9:16"),
		Duration:          defaultVideoDuration(p.DurationSeconds),
		Resolution:        "1080p",
		CreatedBy:         p.CreatedBy,
		SafeDemoOnly:      recommendation.SafeDemoMode,
	})
	if err != nil {
		return nil, err
	}
	contextPayload := map[string]any{
		"schema_version":                "drama_context_v1",
		"employee_alias":                employeeAlias,
		"api_client_id":                 strings.TrimSpace(p.APIClientID),
		"project_id":                    strings.TrimSpace(p.ProjectID),
		"drama_type":                    strings.TrimSpace(p.DramaType),
		"genre":                         strings.TrimSpace(p.Genre),
		"episode_no":                    p.EpisodeNo,
		"scene_type":                    strings.TrimSpace(p.SceneType),
		"shot_role":                     strings.TrimSpace(p.ShotRole),
		"dramatic_goal":                 strings.TrimSpace(p.DramaticGoal),
		"reference_assets":              p.ReferenceAssets,
		"requested_engine_capabilities": p.RequestedEngineCapabilities,
		"selected_provider":             recommendation.SelectedProvider,
		"selected_model":                recommendation.SelectedModel,
		"selected_mode":                 recommendation.SelectedMode,
		"engine_profile_id":             recommendation.EngineProfileID,
		"routing_reason":                recommendation.RoutingReason,
		"skipped_providers":             recommendation.SkippedProviders,
		"fallback_ready":                recommendation.FallbackReady,
		"safe_demo_mode":                recommendation.SafeDemoMode,
		"duration_seconds":              defaultVideoDuration(p.DurationSeconds),
		"aspect_ratio":                  firstNonEmptyVideo(strings.TrimSpace(p.AspectRatio), "9:16"),
	}
	_ = s.repo.AddTaskEvent(ctx, &VideoTaskEvent{VideoTaskID: task.ID, EventType: "drama_context", Message: "drama task context captured", Payload: contextPayload})
	promptEvent := &VideoTaskEvent{VideoTaskID: task.ID, EventType: "prompt_artifact", Message: "drama prompt artifact captured", Payload: map[string]any{
		"schema_version":        "prompt_artifact_v1",
		"employee_alias":        employeeAlias,
		"engine_profile_id":     recommendation.EngineProfileID,
		"prompt_full":           strings.TrimSpace(p.Prompt),
		"prompt_structure_json": p.PromptStructure,
		"negative_prompt":       safeStringFromMap(p.PromptStructure, "negative_prompt"),
		"prompt_version":        "v1",
		"reuse_status":          "draft",
		"quality_score":         0,
	}}
	_ = s.repo.AddTaskEvent(ctx, promptEvent)
	shotEvent := &VideoTaskEvent{VideoTaskID: task.ID, EventType: "shot_decision", Message: "drama shot decision captured", Payload: map[string]any{
		"schema_version":         "shot_decision_v1",
		"employee_alias":         employeeAlias,
		"project_id":             strings.TrimSpace(p.ProjectID),
		"reference_asset_id":     firstDramaReferenceAssetID(p.ReferenceAssets),
		"selected_time_range":    firstDramaReferenceTimeRange(p.ReferenceAssets),
		"shot_role":              strings.TrimSpace(p.ShotRole),
		"why_selected":           "created_from_api_request",
		"engine_recommendation":  recommendation.SelectedProvider,
		"employee_actual_engine": recommendation.SelectedProvider,
	}}
	_ = s.repo.AddTaskEvent(ctx, shotEvent)
	skillEvent := &VideoTaskEvent{VideoTaskID: task.ID, EventType: "skill_event", Message: "drama skill event created", Payload: map[string]any{
		"schema_version":       "skill_event_v1",
		"employee_alias":       employeeAlias,
		"drama_type":           strings.TrimSpace(p.DramaType),
		"scene_type":           strings.TrimSpace(p.SceneType),
		"shot_role":            strings.TrimSpace(p.ShotRole),
		"engine_profile_id":    recommendation.EngineProfileID,
		"selected_provider":    recommendation.SelectedProvider,
		"selected_model":       recommendation.SelectedModel,
		"selected_mode":        recommendation.SelectedMode,
		"learning_pool_status": "draft",
		"review_status":        "pending_admin_review",
		"rollback_status":      "active",
	}}
	_ = s.repo.AddTaskEvent(ctx, skillEvent)

	events, _ := s.repo.ListTaskEvents(ctx, task.ID, 200)
	return dramaTaskContract(task, events), nil
}

func (s *VideoGatewayService) GetDramaTask(ctx context.Context, rawID string, userID int64, isAdmin bool) (*DramaTaskContract, error) {
	id, err := ParseDramaTaskID(rawID)
	if err != nil {
		return nil, err
	}
	task, events, err := s.GetTask(ctx, id, userID, isAdmin)
	if err != nil {
		return nil, err
	}
	return dramaTaskContract(task, events), nil
}

func (s *VideoGatewayService) ListDramaTasks(ctx context.Context, p VideoTaskListParams, filters map[string]string) ([]*DramaTaskContract, int64, error) {
	if p.PageSize <= 0 || p.PageSize > videoMaxPageSize {
		p.PageSize = videoMaxPageSize
	}
	if p.Page <= 0 {
		p.Page = 1
	}
	tasks, total, err := s.repo.ListDramaTasks(ctx, p, filters)
	if err != nil {
		return nil, 0, err
	}
	out := make([]*DramaTaskContract, 0, len(tasks))
	for _, task := range tasks {
		events, err := s.repo.ListTaskEvents(ctx, task.ID, 200)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, dramaTaskContract(task, events))
	}
	return out, total, nil
}

func (s *VideoGatewayService) RecordDramaShotDecision(ctx context.Context, p DramaShotDecisionParams, userID int64, isAdmin bool) (*DramaEventRecord, error) {
	id, err := ParseDramaTaskID(p.TaskID)
	if err != nil {
		return nil, err
	}
	if _, _, err := s.GetTask(ctx, id, userID, isAdmin); err != nil {
		return nil, err
	}
	event := &VideoTaskEvent{VideoTaskID: id, EventType: "shot_decision", Message: "drama shot decision recorded", Payload: map[string]any{
		"schema_version":         "shot_decision_v1",
		"employee_alias":         strings.TrimSpace(p.EmployeeAlias),
		"project_id":             strings.TrimSpace(p.ProjectID),
		"reference_asset_id":     strings.TrimSpace(p.ReferenceAssetID),
		"selected_time_range":    strings.TrimSpace(p.SelectedTimeRange),
		"shot_role":              strings.TrimSpace(p.ShotRole),
		"why_selected":           strings.TrimSpace(p.WhySelected),
		"visual_elements_json":   p.VisualElements,
		"kept_or_replaced":       strings.TrimSpace(p.KeptOrReplaced),
		"engine_recommendation":  strings.TrimSpace(p.EngineRecommendation),
		"employee_actual_engine": strings.TrimSpace(p.EmployeeActualEngine),
		"employee_note":          strings.TrimSpace(p.EmployeeNote),
	}}
	if err := s.repo.AddTaskEvent(ctx, event); err != nil {
		return nil, err
	}
	return dramaEventRecord(event), nil
}

func (s *VideoGatewayService) RecordDramaPromptArtifact(ctx context.Context, p DramaPromptArtifactParams, userID int64, isAdmin bool) (*DramaEventRecord, error) {
	id, err := ParseDramaTaskID(p.TaskID)
	if err != nil {
		return nil, err
	}
	if _, _, err := s.GetTask(ctx, id, userID, isAdmin); err != nil {
		return nil, err
	}
	event := &VideoTaskEvent{VideoTaskID: id, EventType: "prompt_artifact", Message: "drama prompt artifact recorded", Payload: map[string]any{
		"schema_version":           "prompt_artifact_v1",
		"employee_alias":           strings.TrimSpace(p.EmployeeAlias),
		"engine_profile_id":        strings.TrimSpace(p.EngineProfileID),
		"prompt_full":              strings.TrimSpace(p.PromptFull),
		"prompt_structure_json":    p.PromptStructure,
		"negative_prompt":          strings.TrimSpace(p.NegativePrompt),
		"prompt_version":           firstNonEmptyVideo(strings.TrimSpace(p.PromptVersion), "v1"),
		"manual_edit_history_json": p.ManualEditHistory,
		"reuse_status":             firstNonEmptyVideo(strings.TrimSpace(p.ReuseStatus), "draft"),
		"quality_score":            p.QualityScore,
		"manager_note":             strings.TrimSpace(p.ManagerNote),
	}}
	if err := s.repo.AddTaskEvent(ctx, event); err != nil {
		return nil, err
	}
	return dramaEventRecord(event), nil
}

func (s *VideoGatewayService) DramaProviderPool(ctx context.Context) ([]DramaProviderPoolItem, error) {
	providers, err := s.ListProviderAccounts(ctx)
	if err != nil {
		return nil, err
	}
	fallbackReady := false
	for _, provider := range providers {
		if provider.Provider == VideoProviderMock && provider.RouteAvailable {
			fallbackReady = true
			break
		}
	}
	out := make([]DramaProviderPoolItem, 0, len(providers))
	for _, provider := range providers {
		profiles := dramaProfilesByFamily(provider.Provider)
		out = append(out, DramaProviderPoolItem{
			ProviderAccountStatus: providerRuntimeStatusForPool(provider),
			ProviderCode:          provider.Provider,
			DisplayName:           provider.DisplayName,
			CredentialConfigured:  provider.APIKeyConfigured,
			RealProviderVerified:  false,
			LastErrorType:         firstNonEmptyVideo(provider.DiagnosticType, provider.LastError),
			CooldownUntil:         videoMetadataString(provider.Metadata, "cooldown_until"),
			FailureCountToday:     provider.TodayFailures,
			FallbackReady:         fallbackReady,
			SuitableSceneTypes:    mergeDramaBestScenes(profiles),
			UnsuitableSceneTypes:  mergeDramaWeakScenes(profiles),
			SupportedModes:        mergeDramaModes(profiles),
			RoutingReason:         providerRouteReasonForPool(provider),
			SkippedReason:         provider.RouteSkipReason,
			ConcurrencyLimit:      videoMetadataInt(provider.Metadata, "concurrency_limit", 1),
			CurrentInflight:       provider.CurrentInflight,
			DailyQuotaEstimate:    videoMetadataInt(provider.Metadata, "daily_quota_estimate", 0),
			CostEstimate:          firstNonEmptyVideo(videoMetadataString(provider.Metadata, "cost_estimate"), "NEEDS_REAL_PROVIDER_VALIDATION"),
			LatencyEstimate:       firstNonEmptyVideo(videoMetadataString(provider.Metadata, "latency_estimate"), "NEEDS_REAL_PROVIDER_VALIDATION"),
			SafeDemoMode:          provider.Provider == VideoProviderMock,
		})
	}
	return out, nil
}

func (s *VideoGatewayService) DramaRoutingEvents(ctx context.Context, limit int) ([]DramaRoutingEventSummary, error) {
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	tasks, _, err := s.ListTasks(ctx, VideoTaskListParams{Page: 1, PageSize: limit, IsAdmin: true})
	if err != nil {
		return nil, err
	}
	out := make([]DramaRoutingEventSummary, 0, len(tasks))
	for _, task := range tasks {
		events, err := s.repo.ListTaskEvents(ctx, task.ID, 200)
		if err != nil {
			return nil, err
		}
		contract := dramaTaskContract(task, events)
		for _, event := range events {
			if event.EventType != "routed" && event.EventType != "drama_context" {
				continue
			}
			out = append(out, DramaRoutingEventSummary{
				TaskID:           FormatDramaTaskID(task.ID),
				EventID:          event.ID,
				EventType:        event.EventType,
				EmployeeAlias:    contract.EmployeeAlias,
				APIClientID:      contract.APIClientID,
				DramaType:        contract.DramaType,
				SceneType:        contract.SceneType,
				SelectedProvider: contract.SelectedProvider,
				SelectedModel:    contract.SelectedModel,
				SelectedMode:     contract.SelectedMode,
				RoutingReason:    contract.RoutingReason,
				SkippedProviders: contract.SkippedProviders,
				CreatedAt:        formatServiceTime(event.CreatedAt),
			})
		}
	}
	return out, nil
}

func (s *VideoGatewayService) DramaSkillCards(ctx context.Context) ([]DramaSkillCard, error) {
	tasks, _, err := s.ListTasks(ctx, VideoTaskListParams{Page: 1, PageSize: 100, IsAdmin: true})
	if err != nil {
		return nil, err
	}
	cards := map[string]*DramaSkillCard{}
	for _, task := range tasks {
		events, err := s.repo.ListTaskEvents(ctx, task.ID, 200)
		if err != nil {
			return nil, err
		}
		contract := dramaTaskContract(task, events)
		if contract.EmployeeAlias == "" {
			continue
		}
		card := cards[contract.EmployeeAlias]
		if card == nil {
			card = &DramaSkillCard{
				EmployeeAlias:    contract.EmployeeAlias,
				ScopeType:        "employee_alias",
				GeneratedByModel: false,
				ReviewStatus:     "draft_pending_admin_review",
				Version:          "skill_card_v1",
				UpdatedAt:        formatServiceTime(task.UpdatedAt),
				Counts:           map[string]int{},
			}
			cards[contract.EmployeeAlias] = card
		}
		card.TotalTasks++
		card.DramaTypes = appendUniqueString(card.DramaTypes, contract.DramaType)
		card.SceneTypes = appendUniqueString(card.SceneTypes, contract.SceneType)
		card.EngineModes = appendUniqueString(card.EngineModes, contract.SelectedProvider+"/"+contract.SelectedMode)
		card.Counts["tasks"] = card.TotalTasks
		card.StrongPatterns = appendUniqueString(card.StrongPatterns, "API task captured for "+firstNonEmptyVideo(contract.SceneType, "unknown_scene"))
		card.PromptPatterns = appendUniqueString(card.PromptPatterns, firstNonEmptyVideo(safeStringFromMap(contract.PromptStructure, "dramatic_goal"), contract.DramaticGoal))
		card.ShotSelectionPatterns = appendUniqueString(card.ShotSelectionPatterns, contract.ShotRole)
		card.EngineSelectionPatterns = appendUniqueString(card.EngineSelectionPatterns, contract.SelectedProvider+" selected for "+firstNonEmptyVideo(contract.DramaType, "drama"))
		if task.Status == VideoStatusFailed {
			card.FailurePatterns = appendUniqueString(card.FailurePatterns, firstNonEmptyVideo(task.ErrorMessage, "failed_without_message"))
		}
		card.AutomationRecommendations = appendUniqueString(card.AutomationRecommendations, "Convert repeated scene/prompt/engine choices into reusable drama production templates.")
	}
	out := make([]DramaSkillCard, 0, len(cards))
	for _, card := range cards {
		out = append(out, *card)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].EmployeeAlias < out[j].EmployeeAlias })
	return out, nil
}

func (s *VideoGatewayService) GenerateDramaSkillAnalysisExport(ctx context.Context, req DramaSkillAnalysisExportRequest) (*DramaSkillAnalysisExport, error) {
	records, err := s.dramaExportRecords(ctx)
	if err != nil {
		return nil, err
	}
	target := strings.ToLower(strings.TrimSpace(req.TargetAIModel))
	if target == "" {
		target = "all"
	}
	start := firstNonEmptyVideo(strings.TrimSpace(req.TimeRangeStart), time.Now().Format("2006-01-02"))
	end := firstNonEmptyVideo(strings.TrimSpace(req.TimeRangeEnd), time.Now().Format("2006-01-02"))
	exportJSON := map[string]any{
		"schema_version":   DramaExportSchemaVersion,
		"business_context": "AI中剧/AI短剧内部生产经验沉淀",
		"deployment_mode":  "company_host_api_gateway",
		"anonymized":       true,
		"time_range": map[string]string{
			"start": start,
			"end":   end,
		},
		"api_usage_summary": map[string]any{
			"total_requests":   len(records),
			"success_rate":     dramaExportSuccessRate(records),
			"top_engine_modes": dramaTopEngineModes(records),
		},
		"records": records,
	}
	prompts := DramaAnalysisPrompts()
	selectedPrompt := prompts["gpt"]
	if target == "gemini" || target == "kimi" || target == "gpt" {
		selectedPrompt = prompts[target]
	}
	return &DramaSkillAnalysisExport{
		ID:             dramaDefaultExportIDPrefix + time.Now().UTC().Format("20060102T150405Z"),
		ExportType:     firstNonEmptyVideo(strings.TrimSpace(req.ExportType), "skill_learning_snapshot"),
		TargetAIModel:  target,
		Anonymized:     true,
		SchemaVersion:  DramaExportSchemaVersion,
		ExportJSON:     exportJSON,
		AnalysisPrompt: selectedPrompt,
		Prompts:        prompts,
		CreatedAt:      time.Now().UTC().Format(time.RFC3339),
	}, nil
}

func (s *VideoGatewayService) dramaExportRecords(ctx context.Context) ([]map[string]any, error) {
	tasks, _, err := s.ListTasks(ctx, VideoTaskListParams{Page: 1, PageSize: 100, IsAdmin: true})
	if err != nil {
		return nil, err
	}
	records := make([]map[string]any, 0, len(tasks))
	for _, task := range tasks {
		events, err := s.repo.ListTaskEvents(ctx, task.ID, 200)
		if err != nil {
			return nil, err
		}
		contract := dramaTaskContract(task, events)
		if contract.EmployeeAlias == "" {
			continue
		}
		records = append(records, map[string]any{
			"employee_alias":       contract.EmployeeAlias,
			"api_client_id":        contract.APIClientID,
			"drama_type":           contract.DramaType,
			"genre":                contract.Genre,
			"scene_type":           contract.SceneType,
			"shot_role":            contract.ShotRole,
			"engine":               engineFamilyFromProvider(contract.SelectedProvider),
			"model":                contract.SelectedModel,
			"mode":                 contract.SelectedMode,
			"reference_asset_type": firstNonEmptyVideo(firstReferenceTypeFromEvents(events), "not_provided"),
			"selected_time_range":  firstSelectedRangeFromEvents(events),
			"why_selected":         firstWhySelectedFromEvents(events),
			"prompt_structure":     contract.PromptStructure,
			"routing_trace": map[string]any{
				"selected_provider": contract.SelectedProvider,
				"skipped_providers": contract.SkippedProviders,
			},
			"result_status":  dramaResultStatus(task),
			"admin_score":    0,
			"failure_reason": optionalString(task.ErrorMessage),
			"manager_note":   "",
		})
	}
	return records, nil
}

func DramaAnalysisPrompts() map[string]string {
	return map[string]string{
		"gemini": "你是 AI 中剧/AI 短剧生产经验分析助手。请基于输入的脱敏 JSON，总结员工和团队在不同剧种、场景、镜头目标、视频引擎下的 Skill。不得输出真实姓名。不得推断员工隐私。只分析工作任务相关数据。请输出严格 JSON，结构为 {\"employee_skill_cards\":[],\"team_common_patterns\":[],\"drama_type_strategies\":[],\"engine_selection_strategies\":[],\"prompt_templates\":[],\"shot_selection_rules\":[],\"failure_patterns\":[],\"automation_recommendations\":[]}。",
		"gpt":    "你是 AI 短剧生产系统架构顾问。请从脱敏 API 调用、镜头选择、prompt 结构、模型选择和结果反馈中，归纳可自动化的生产规则。请优先输出：1. 不同剧种的 prompt 框架 2. 不同场景的镜头选择规则 3. 不同视频引擎的适用边界 4. 失败规避规则 5. 可产品化的自动推荐逻辑。",
		"kimi":   "你是 AI 中剧/短剧生产知识库整理助手。请处理大量脱敏历史任务，按 employee_alias、剧种、场景、引擎、模型、模式分类总结。输出：1. 每周 Skill 变化 2. 可复用 prompt 资产 3. 可复用镜头策略 4. 失败高频原因 5. 下周自动化建议。",
	}
}

func ParseDramaTaskID(raw string) (int64, error) {
	value := strings.TrimSpace(raw)
	value = strings.TrimPrefix(value, DramaTaskIDPrefix)
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id <= 0 {
		return 0, ErrVideoTaskNotFound
	}
	return id, nil
}

func FormatDramaTaskID(id int64) string {
	return fmt.Sprintf("%s%d", DramaTaskIDPrefix, id)
}

func dramaEngineProfiles() []DramaEngineProfile {
	return []DramaEngineProfile{
		{
			ID: "seedance_real_reference_to_video", ProviderCode: "seedance_real", EngineFamily: VideoProviderSeedance, ModelName: "seedance-2.0-pro", ModelVersion: "2.0", Mode: "reference-to-video",
			SupportsTextToVideo: true, SupportsImageToVideo: true, SupportsGlobalReference: true, SupportsReferenceImages: true, SupportsReferenceVideo: true, SupportsReferenceAudio: true,
			SupportsNativeAudio: true, SupportsShortDrama: true, SupportsMiddleDrama: true, SupportsMultiCharacter: true, SupportsDialogue: true, SupportsCameraControl: true, MaxDurationSeconds: 10,
			AspectRatios: []string{"9:16", "16:9", "1:1"}, BestForSceneTypes: []string{"AI中剧", "AI短剧", "全局参考", "多模态 reference", "多镜头潜力"}, WeakForSceneTypes: []string{"强 lip sync 闭环", "未授权真人账号验证"},
			RecommendedPromptStructure: []string{"character_identity", "scene_context", "dramatic_goal", "reference_assets", "camera", "lighting", "performance", "engine_specific_controls", "negative_prompt"},
			CommonFailureModes:         []string{"reference consistency drift", "native audio mismatch", "needs authorized API verification"}, CredentialRequired: true, RealProviderVerified: false, InternalSafeModeOnly: false,
			SourceNote: "NEEDS_REAL_PROVIDER_VALIDATION; capability profile is readiness planning only.", LastVerifiedAt: "NEEDS_AUTHORIZED_TEST",
		},
		{
			ID: "kling_real_image_to_video", ProviderCode: "kling_real", EngineFamily: VideoProviderKling, ModelName: "kling-2.6-pro", ModelVersion: "2.6", Mode: "image-to-video",
			SupportsTextToVideo: true, SupportsImageToVideo: true, SupportsFirstLastFrame: true, SupportsReferenceImages: true, SupportsReferenceVideo: true, SupportsMotionControl: true,
			SupportsNativeAudio: true, SupportsLipsync: true, SupportsRealPerson: true, SupportsAnimeDrama: true, SupportsShortDrama: true, SupportsMultiCharacter: true, SupportsDialogue: true, SupportsCameraControl: true,
			MaxDurationSeconds: 10, AspectRatios: []string{"9:16", "16:9", "1:1"}, BestForSceneTypes: []string{"真人短剧", "漫剧", "真人转漫剧", "情绪爆发", "动作冲突", "多角色对话"}, WeakForSceneTypes: []string{"未授权真实 provider 闭环", "长篇连续中剧一致性未验证"},
			RecommendedPromptStructure: []string{"character_identity", "scene_context", "dramatic_goal", "shot_type", "camera", "performance", "dialogue", "engine_specific_controls", "negative_prompt"},
			CommonFailureModes:         []string{"face drift", "motion-control overconstraint", "lip sync mismatch", "needs authorized API verification"}, CredentialRequired: true, RealProviderVerified: false, InternalSafeModeOnly: false,
			SourceNote: "NEEDS_REAL_PROVIDER_VALIDATION; split by model version and mode in future authorized tests.", LastVerifiedAt: "NEEDS_AUTHORIZED_TEST",
		},
		{
			ID: "official_api_provider_readiness", ProviderCode: "official_api_provider", EngineFamily: "official_api", ModelName: "official-api-profile", ModelVersion: "readiness", Mode: "provider-specific",
			SupportsTextToVideo: true, SupportsImageToVideo: true, SupportsShortDrama: true, SupportsMiddleDrama: true, MaxDurationSeconds: 10, AspectRatios: []string{"provider_specific"},
			BestForSceneTypes: []string{"authorized production validation"}, WeakForSceneTypes: []string{"not configured in Phase 4B.3"}, RecommendedPromptStructure: []string{"provider_contract", "scene_context", "result_feedback"},
			CommonFailureModes: []string{"credential_not_configured", "provider_contract_unverified"}, CredentialRequired: true, RealProviderVerified: false, InternalSafeModeOnly: false, SourceNote: "Reserved for later authorized official API integration.", LastVerifiedAt: "NOT_VERIFIED",
		},
		{
			ID: "subscription_channel_readiness", ProviderCode: "subscription_channel", EngineFamily: "subscription", ModelName: "subscription-profile", ModelVersion: "readiness", Mode: "manual_or_internal_authorized",
			SupportsTextToVideo: true, SupportsImageToVideo: true, SupportsShortDrama: true, MaxDurationSeconds: 10, AspectRatios: []string{"provider_specific"},
			BestForSceneTypes: []string{"internal authorized workflow only"}, WeakForSceneTypes: []string{"no automatic login state collection", "no production verification"}, RecommendedPromptStructure: []string{"business_request", "manual_review_note"},
			CommonFailureModes: []string{"manual SLA uncertainty", "credential policy required"}, CredentialRequired: true, RealProviderVerified: false, InternalSafeModeOnly: false, SourceNote: "No automatic cookie/token/login-state collection is implemented.", LastVerifiedAt: "NOT_VERIFIED",
		},
		{
			ID: "internal_authorized_channel_readiness", ProviderCode: "internal_authorized_channel", EngineFamily: "internal_authorized", ModelName: "internal-safe-profile", ModelVersion: "readiness", Mode: "adapter",
			SupportsTextToVideo: true, SupportsImageToVideo: true, SupportsShortDrama: true, SupportsMiddleDrama: true, MaxDurationSeconds: 10, AspectRatios: []string{"9:16", "16:9"},
			BestForSceneTypes: []string{"company host controlled experiments"}, WeakForSceneTypes: []string{"external production use"}, RecommendedPromptStructure: []string{"api_client_id", "drama_context", "manager_note"},
			CommonFailureModes: []string{"authorization_scope_missing", "fallback_required"}, CredentialRequired: true, RealProviderVerified: false, InternalSafeModeOnly: false, SourceNote: "Reserved for explicit internal authorization in a later phase.", LastVerifiedAt: "NOT_VERIFIED",
		},
		{
			ID: "seedance_safe_demo", ProviderCode: "seedance_safe_demo", EngineFamily: VideoProviderSeedance, ModelName: "seedance-2.0-safe-profile", ModelVersion: "safe-demo", Mode: "reference-to-video",
			SupportsTextToVideo: true, SupportsImageToVideo: true, SupportsGlobalReference: true, SupportsReferenceImages: true, SupportsReferenceVideo: true, SupportsReferenceAudio: true, SupportsShortDrama: true, SupportsMiddleDrama: true, SupportsCameraControl: true,
			MaxDurationSeconds: 5, AspectRatios: []string{"9:16", "16:9"}, BestForSceneTypes: []string{"AI短剧", "全局参考", "多模态 reference"}, WeakForSceneTypes: []string{"真实上游效果"}, RecommendedPromptStructure: []string{"scene_context", "dramatic_goal", "reference_assets", "camera", "negative_prompt"},
			CommonFailureModes: []string{"safe demo placeholder only"}, CredentialRequired: false, RealProviderVerified: false, InternalSafeModeOnly: true, SourceNote: "Safe demo profile; does not call real Seedance.", LastVerifiedAt: time.Now().UTC().Format("2006-01-02"),
		},
		{
			ID: "kling_safe_demo", ProviderCode: "kling_safe_demo", EngineFamily: VideoProviderKling, ModelName: "kling-2.6-pro-safe-profile", ModelVersion: "safe-demo", Mode: "image-to-video",
			SupportsTextToVideo: true, SupportsImageToVideo: true, SupportsFirstLastFrame: true, SupportsReferenceImages: true, SupportsReferenceVideo: true, SupportsMotionControl: true, SupportsLipsync: true, SupportsRealPerson: true, SupportsAnimeDrama: true, SupportsShortDrama: true, SupportsMultiCharacter: true, SupportsDialogue: true, SupportsCameraControl: true,
			MaxDurationSeconds: 5, AspectRatios: []string{"9:16", "16:9"}, BestForSceneTypes: []string{"真人短剧", "漫剧", "真人转漫剧", "情绪爆发", "动作冲突", "多角色对话"}, WeakForSceneTypes: []string{"真实上游效果"}, RecommendedPromptStructure: []string{"character_identity", "scene_context", "dramatic_goal", "shot_type", "camera", "performance", "engine_specific_controls", "negative_prompt"},
			CommonFailureModes: []string{"safe demo placeholder only"}, CredentialRequired: false, RealProviderVerified: false, InternalSafeModeOnly: true, SourceNote: "Safe demo profile; does not call real Kling.", LastVerifiedAt: time.Now().UTC().Format("2006-01-02"),
		},
		{
			ID: "safe_demo_provider", ProviderCode: "safe_demo_provider", EngineFamily: VideoProviderMock, ModelName: "mock-video-v1", ModelVersion: "safe-demo", Mode: "text-to-video",
			SupportsTextToVideo: true, SupportsImageToVideo: true, SupportsReferenceImages: true, SupportsReferenceVideo: true, SupportsShortDrama: true, SupportsCameraControl: false,
			MaxDurationSeconds: 5, AspectRatios: []string{"9:16", "16:9", "1:1"}, BestForSceneTypes: []string{"内部演示", "数据链路验证", "Skill Learning 验证", "路由 trace 验证", "错误处理验证"}, WeakForSceneTypes: []string{"真实生产生成质量"},
			RecommendedPromptStructure: []string{"employee_alias", "api_client_id", "drama_context", "prompt_structure", "result_feedback"}, CommonFailureModes: []string{"intentional demo failure"}, CredentialRequired: false, RealProviderVerified: false, InternalSafeModeOnly: true,
			SourceNote: "Local safe demo provider; never described as a production provider.", LastVerifiedAt: time.Now().UTC().Format("2006-01-02"),
		},
	}
}

func dramaTaskContract(task *VideoTask, events []*VideoTaskEvent) *DramaTaskContract {
	if task == nil {
		return &DramaTaskContract{}
	}
	context := dramaContextPayload(events)
	selectedProvider := safeStringFromMap(context, "selected_provider")
	selectedModel := safeStringFromMap(context, "selected_model")
	selectedMode := safeStringFromMap(context, "selected_mode")
	if selectedProvider == "" {
		selectedProvider = providerCodeFromVideoProvider(task.Provider)
	}
	if selectedModel == "" {
		selectedModel = task.Model
	}
	if selectedMode == "" {
		selectedMode = videoTaskTypeToDramaMode(task.TaskType)
	}
	return &DramaTaskContract{
		TaskID:                 FormatDramaTaskID(task.ID),
		Status:                 task.Status,
		SelectedProvider:       selectedProvider,
		SelectedModel:          selectedModel,
		SelectedMode:           selectedMode,
		RoutingReason:          firstNonEmptyVideo(safeStringFromMap(context, "routing_reason"), dramaRoutedReason(events)),
		SkippedProviders:       dramaSkippedProvidersFromContext(context),
		ResultURLOrPlaceholder: firstNonEmptyVideo(task.ResultURL, "safe-demo://result/"+FormatDramaTaskID(task.ID)),
		RoutingTrace:           dramaRoutingTrace(events),
		PromptArtifactID:       firstDramaEventID(events, "prompt_artifact"),
		ShotDecisionID:         firstDramaEventID(events, "shot_decision"),
		SkillEventID:           firstDramaEventID(events, "skill_event"),
		SkillEventCreated:      firstDramaEventID(events, "skill_event") != "",
		EmployeeAlias:          safeStringFromMap(context, "employee_alias"),
		APIClientID:            safeStringFromMap(context, "api_client_id"),
		ProjectID:              safeStringFromMap(context, "project_id"),
		DramaType:              safeStringFromMap(context, "drama_type"),
		Genre:                  safeStringFromMap(context, "genre"),
		SceneType:              safeStringFromMap(context, "scene_type"),
		ShotRole:               safeStringFromMap(context, "shot_role"),
		DramaticGoal:           safeStringFromMap(context, "dramatic_goal"),
		PromptStructure:        firstPromptStructure(events),
		RequestedCapabilities:  dramaCapabilityFromContext(context),
		DurationSeconds:        intFromMap(context, "duration_seconds", task.Duration),
		AspectRatio:            firstNonEmptyVideo(safeStringFromMap(context, "aspect_ratio"), task.AspectRatio),
		CreatedAt:              formatServiceTime(task.CreatedAt),
		UpdatedAt:              formatServiceTime(task.UpdatedAt),
	}
}

func (s *VideoGatewayService) dramaSkippedProviders(ctx context.Context, preferredFamily string) []DramaProviderSkip {
	items, err := s.ListProviderAccounts(ctx)
	status := map[string]string{}
	if err == nil {
		for _, item := range items {
			if item.Provider == VideoProviderMock {
				continue
			}
			if item.RouteSkipReason != "" {
				status[item.Provider] = routeSkipReasonCode(item.RouteSkipReason)
			} else if !item.APIKeyConfigured {
				status[item.Provider] = "credential_not_configured"
			} else {
				status[item.Provider] = "real_provider_not_verified"
			}
		}
	}
	reasons := []DramaProviderSkip{}
	for _, provider := range []string{VideoProviderSeedance, VideoProviderKling} {
		reason := firstNonEmptyVideo(status[provider], "real_provider_not_verified")
		if provider == preferredFamily && reason == "credential_not_configured" {
			reason = "real_provider_not_verified"
		}
		reasons = append(reasons, DramaProviderSkip{
			Provider: provider + "_real",
			Reason:   reason,
			Detail:   "Phase 4B.3 does not call real providers; use safe demo profile until authorized validation.",
		})
	}
	return reasons
}

func recommendedDramaFamily(p DramaProviderRecommendParams) string {
	text := strings.Join([]string{p.DramaType, p.SceneType, p.ShotRole, p.ReferenceAssetType}, " ")
	if p.RequestedEngineCapabilities.SupportsLipsync ||
		p.RequestedEngineCapabilities.SupportsMotionControl ||
		p.RequestedEngineCapabilities.SupportsRealPerson ||
		strings.Contains(text, "真人") ||
		strings.Contains(text, "漫剧") ||
		strings.Contains(text, "情绪爆发") ||
		strings.Contains(text, "动作冲突") {
		return VideoProviderKling
	}
	if p.RequestedEngineCapabilities.SupportsGlobalReference ||
		p.RequestedEngineCapabilities.SupportsNativeAudio ||
		strings.Contains(text, "全局参考") ||
		strings.Contains(text, "中剧") ||
		strings.Contains(text, "reference") ||
		strings.Contains(text, "参考") {
		return VideoProviderSeedance
	}
	return VideoProviderKling
}

func recommendedDramaMode(p DramaProviderRecommendParams) string {
	if p.RequestedEngineCapabilities.SupportsImageToVideo || strings.Contains(p.ReferenceAssetType, "image") || strings.Contains(p.ReferenceAssetType, "图片") {
		return VideoTaskTypeImageToVideo
	}
	if p.RequestedEngineCapabilities.SupportsGlobalReference || strings.Contains(p.ReferenceAssetType, "video") || strings.Contains(p.ReferenceAssetType, "参考") {
		return VideoTaskTypeReferenceToVideo
	}
	return VideoTaskTypeTextToVideo
}

func dramaRoutingReason(p DramaProviderRecommendParams, family, mode string) string {
	scene := firstNonEmptyVideo(strings.TrimSpace(p.SceneType), "unknown_scene")
	return fmt.Sprintf("scene_type=%s mapped to %s/%s; real provider not verified in Phase 4B.3, using safe demo profile for company-host API gateway readiness", scene, family, mode)
}

func dramaModeToVideoTaskType(mode string) string {
	switch strings.ReplaceAll(strings.TrimSpace(mode), "-", "_") {
	case VideoTaskTypeImageToVideo:
		return VideoTaskTypeImageToVideo
	case VideoTaskTypeReferenceToVideo:
		return VideoTaskTypeReferenceToVideo
	default:
		return VideoTaskTypeTextToVideo
	}
}

func videoTaskTypeToDramaMode(taskType string) string {
	return strings.ReplaceAll(taskType, "_", "-")
}

func providerCodeFromVideoProvider(provider string) string {
	if provider == VideoProviderMock {
		return "safe_demo_provider"
	}
	return provider + "_real"
}

func dramaProfileByID(id string) *DramaEngineProfile {
	for _, profile := range dramaEngineProfiles() {
		if profile.ID == id {
			p := profile
			return &p
		}
	}
	return nil
}

func dramaProfilesByFamily(family string) []DramaEngineProfile {
	out := []DramaEngineProfile{}
	for _, profile := range dramaEngineProfiles() {
		if profile.EngineFamily == family {
			out = append(out, profile)
		}
	}
	return out
}

func firstDramaReferenceAssetType(items []DramaReferenceAsset) string {
	if len(items) == 0 {
		return ""
	}
	return strings.TrimSpace(items[0].AssetType)
}

func firstDramaReferenceAssetID(items []DramaReferenceAsset) string {
	if len(items) == 0 {
		return ""
	}
	return strings.TrimSpace(items[0].AssetID)
}

func firstDramaReferenceTimeRange(items []DramaReferenceAsset) string {
	if len(items) == 0 {
		return ""
	}
	return strings.TrimSpace(items[0].SelectedTimeRange)
}

func firstDramaReferencePlaceholder(items []DramaReferenceAsset, target string) string {
	for _, item := range items {
		assetType := strings.ToLower(item.AssetType)
		if (target == "reference_image" && strings.Contains(assetType, "image")) ||
			(target == "reference_video" && strings.Contains(assetType, "video")) {
			return "safe-demo://asset/" + strings.TrimSpace(item.AssetID)
		}
	}
	return ""
}

func safeStringFromMap(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	value, ok := m[key]
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

func intFromMap(m map[string]any, key string, fallback int) int {
	if m == nil {
		return fallback
	}
	switch v := m[key].(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case json.Number:
		i, _ := v.Int64()
		return int(i)
	default:
		return fallback
	}
}

func dramaContextPayload(events []*VideoTaskEvent) map[string]any {
	for _, event := range events {
		if event.EventType == "drama_context" {
			return event.Payload
		}
	}
	return map[string]any{}
}

func firstPromptStructure(events []*VideoTaskEvent) map[string]any {
	for _, event := range events {
		if event.EventType != "prompt_artifact" {
			continue
		}
		if value, ok := event.Payload["prompt_structure_json"].(map[string]any); ok {
			return value
		}
	}
	return map[string]any{}
}

func dramaCapabilityFromContext(context map[string]any) DramaEngineCapabilityRequest {
	if raw, ok := context["requested_engine_capabilities"].(DramaEngineCapabilityRequest); ok {
		return raw
	}
	if raw, ok := context["requested_engine_capabilities"].(map[string]any); ok {
		return DramaEngineCapabilityRequest{
			SupportsRealPerson:      boolFromMap(raw, "supports_real_person"),
			SupportsImageToVideo:    boolFromMap(raw, "supports_image_to_video"),
			SupportsMotionControl:   boolFromMap(raw, "supports_motion_control"),
			SupportsLipsync:         boolFromMap(raw, "supports_lipsync"),
			SupportsNativeAudio:     boolFromMap(raw, "supports_native_audio"),
			SupportsGlobalReference: boolFromMap(raw, "supports_global_reference"),
			SupportsAnimeDrama:      boolFromMap(raw, "supports_anime_drama"),
		}
	}
	return DramaEngineCapabilityRequest{}
}

func boolFromMap(m map[string]any, key string) bool {
	switch v := m[key].(type) {
	case bool:
		return v
	case string:
		return strings.EqualFold(v, "true") || v == "1" || strings.EqualFold(v, "yes")
	default:
		return false
	}
}

func dramaSkippedProvidersFromContext(context map[string]any) []DramaProviderSkip {
	value := context["skipped_providers"]
	switch raw := value.(type) {
	case []DramaProviderSkip:
		return raw
	case []any:
		out := make([]DramaProviderSkip, 0, len(raw))
		for _, item := range raw {
			if m, ok := item.(map[string]any); ok {
				out = append(out, DramaProviderSkip{
					Provider: safeStringFromMap(m, "provider"),
					Reason:   safeStringFromMap(m, "reason"),
					Detail:   safeStringFromMap(m, "detail"),
				})
			}
		}
		return out
	default:
		return []DramaProviderSkip{}
	}
}

func dramaRoutingTrace(events []*VideoTaskEvent) []map[string]any {
	out := []map[string]any{}
	for _, event := range events {
		if event.EventType != "routed" && event.EventType != "drama_context" && event.EventType != "skill_event" {
			continue
		}
		out = append(out, map[string]any{
			"event_id":   event.ID,
			"event_type": event.EventType,
			"message":    event.Message,
			"payload":    event.Payload,
			"created_at": formatServiceTime(event.CreatedAt),
		})
	}
	return out
}

func dramaRoutedReason(events []*VideoTaskEvent) string {
	for _, event := range events {
		if event.EventType == "routed" {
			return safeStringFromMap(event.Payload, "reason")
		}
	}
	return ""
}

func firstDramaEventID(events []*VideoTaskEvent, eventType string) string {
	for _, event := range events {
		if event.EventType == eventType {
			return fmt.Sprintf("%s_%d", eventType, event.ID)
		}
	}
	return ""
}

func dramaEventRecord(event *VideoTaskEvent) *DramaEventRecord {
	if event == nil {
		return &DramaEventRecord{}
	}
	return &DramaEventRecord{
		ID:        fmt.Sprintf("%s_%d", event.EventType, event.ID),
		TaskID:    FormatDramaTaskID(event.VideoTaskID),
		EventType: event.EventType,
		Payload:   event.Payload,
		CreatedAt: formatServiceTime(event.CreatedAt),
	}
}

func dramaTaskMatchesFilters(task *DramaTaskContract, filters map[string]string) bool {
	if task == nil {
		return false
	}
	checks := map[string]string{
		"employee_alias": task.EmployeeAlias,
		"api_client_id":  task.APIClientID,
		"project_id":     task.ProjectID,
		"drama_type":     task.DramaType,
		"genre":          task.Genre,
		"scene_type":     task.SceneType,
		"engine":         engineFamilyFromProvider(task.SelectedProvider),
		"model":          task.SelectedModel,
		"mode":           task.SelectedMode,
		"status":         task.Status,
	}
	for key, want := range filters {
		if strings.TrimSpace(want) == "" {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(checks[key]), strings.TrimSpace(want)) {
			return false
		}
	}
	return true
}

func engineFamilyFromProvider(provider string) string {
	if strings.Contains(provider, VideoProviderSeedance) {
		return VideoProviderSeedance
	}
	if strings.Contains(provider, VideoProviderKling) {
		return VideoProviderKling
	}
	return VideoProviderMock
}

func routeSkipReasonCode(reason string) string {
	normalized := strings.ToLower(reason)
	if strings.Contains(normalized, "未配置") || strings.Contains(normalized, "missing") || strings.Contains(normalized, "key") {
		return "credential_not_configured"
	}
	if strings.Contains(normalized, "鉴权") || strings.Contains(normalized, "auth") {
		return "auth_failed"
	}
	if strings.Contains(normalized, "限流") || strings.Contains(normalized, "rate") {
		return "rate_limited"
	}
	if strings.Contains(normalized, "额度") || strings.Contains(normalized, "quota") {
		return "quota_exhausted"
	}
	if strings.Contains(normalized, "停用") || strings.Contains(normalized, "disabled") {
		return "disabled"
	}
	return "real_provider_not_verified"
}

func providerRuntimeStatusForPool(provider *VideoProviderAccount) string {
	if provider == nil {
		return "missing"
	}
	if provider.RouteAvailable {
		if provider.Provider == VideoProviderMock {
			return "safe_demo_ready"
		}
		return "route_available_pending_real_validation"
	}
	return routeSkipReasonCode(provider.RouteSkipReason)
}

func providerRouteReasonForPool(provider *VideoProviderAccount) string {
	if provider == nil {
		return ""
	}
	if provider.Provider == VideoProviderMock && provider.RouteAvailable {
		return "safe/demo fallback provider for internal pilot"
	}
	if provider.RouteAvailable {
		return "route candidate, but Phase 4B.3 does not call real providers"
	}
	return provider.RouteSkipReason
}

func mergeDramaBestScenes(profiles []DramaEngineProfile) []string {
	out := []string{}
	for _, profile := range profiles {
		for _, item := range profile.BestForSceneTypes {
			out = appendUniqueString(out, item)
		}
	}
	return out
}

func mergeDramaWeakScenes(profiles []DramaEngineProfile) []string {
	out := []string{}
	for _, profile := range profiles {
		for _, item := range profile.WeakForSceneTypes {
			out = appendUniqueString(out, item)
		}
	}
	return out
}

func mergeDramaModes(profiles []DramaEngineProfile) []string {
	out := []string{}
	for _, profile := range profiles {
		out = appendUniqueString(out, profile.Mode)
	}
	return out
}

func appendUniqueString(items []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return items
	}
	for _, item := range items {
		if item == value {
			return items
		}
	}
	return append(items, value)
}

func formatServiceTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func optionalString(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

func dramaResultStatus(task *VideoTask) string {
	if task == nil {
		return "unknown"
	}
	switch task.Status {
	case VideoStatusSucceeded:
		return "accepted"
	case VideoStatusFailed:
		return "failed"
	case VideoStatusCancelled:
		return "cancelled"
	default:
		return "pending"
	}
}

func firstReferenceTypeFromEvents(events []*VideoTaskEvent) string {
	context := dramaContextPayload(events)
	raw, ok := context["reference_assets"].([]DramaReferenceAsset)
	if ok && len(raw) > 0 {
		return raw[0].AssetType
	}
	if items, ok := context["reference_assets"].([]any); ok {
		for _, item := range items {
			if m, ok := item.(map[string]any); ok {
				return safeStringFromMap(m, "asset_type")
			}
		}
	}
	return ""
}

func firstSelectedRangeFromEvents(events []*VideoTaskEvent) string {
	for _, event := range events {
		if event.EventType == "shot_decision" {
			return safeStringFromMap(event.Payload, "selected_time_range")
		}
	}
	return ""
}

func firstWhySelectedFromEvents(events []*VideoTaskEvent) string {
	for _, event := range events {
		if event.EventType == "shot_decision" {
			return safeStringFromMap(event.Payload, "why_selected")
		}
	}
	return ""
}

func dramaExportSuccessRate(records []map[string]any) float64 {
	if len(records) == 0 {
		return 0
	}
	success := 0
	for _, record := range records {
		if record["result_status"] == "accepted" {
			success++
		}
	}
	return float64(success) * 100 / float64(len(records))
}

func dramaTopEngineModes(records []map[string]any) []string {
	counts := map[string]int{}
	for _, record := range records {
		key := fmt.Sprintf("%s/%s", record["engine"], record["mode"])
		counts[key]++
	}
	type pair struct {
		key   string
		count int
	}
	pairs := make([]pair, 0, len(counts))
	for key, count := range counts {
		pairs = append(pairs, pair{key: key, count: count})
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].count != pairs[j].count {
			return pairs[i].count > pairs[j].count
		}
		return pairs[i].key < pairs[j].key
	})
	out := []string{}
	for _, item := range pairs {
		out = append(out, item.key)
		if len(out) >= 5 {
			break
		}
	}
	return out
}
