package admin

import (
	"strconv"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type VideoHandler struct {
	video      *service.VideoGatewayService
	reconciler *service.ReliabilityReconciler
}

func NewVideoHandler(video *service.VideoGatewayService) *VideoHandler {
	return NewVideoHandlerWithReliability(video, nil)
}

// NewVideoHandlerWithReliability keeps the existing handler constructor stable
// while allowing the composition root to expose the strictly read-only
// reconciliation dry-run to authenticated administrators.
func NewVideoHandlerWithReliability(video *service.VideoGatewayService, reconciler *service.ReliabilityReconciler) *VideoHandler {
	return &VideoHandler{video: video, reconciler: reconciler}
}

type videoProviderRequest struct {
	Provider           string          `json:"provider" binding:"omitempty,oneof=mock seedance kling"`
	DisplayName        string          `json:"display_name" binding:"omitempty,max=120"`
	Enabled            *bool           `json:"enabled"`
	APIKey             string          `json:"api_key" binding:"omitempty,max=4000"`
	BaseURL            string          `json:"base_url" binding:"omitempty,max=500"`
	DefaultModel       string          `json:"default_model" binding:"omitempty,max=200"`
	RateLimitPerMinute *int            `json:"rate_limit_per_minute" binding:"omitempty,min=0,max=100000"`
	Metadata           *map[string]any `json:"metadata_json"`
}

type videoProviderResponse struct {
	ID                 int64          `json:"id"`
	Provider           string         `json:"provider"`
	DisplayName        string         `json:"display_name"`
	Enabled            bool           `json:"enabled"`
	APIKeyConfigured   bool           `json:"api_key_configured"`
	MaskedKey          string         `json:"masked_key"`
	BaseURL            string         `json:"base_url"`
	DefaultModel       string         `json:"default_model"`
	RateLimitPerMinute int            `json:"rate_limit_per_minute"`
	Metadata           map[string]any `json:"metadata_json"`
	KeyStatus          string         `json:"key_status"`
	HealthStatus       string         `json:"health_status"`
	DiagnosticType     string         `json:"diagnostic_type"`
	SuggestedAction    string         `json:"suggested_action"`
	Priority           int            `json:"priority"`
	CurrentInflight    int64          `json:"current_inflight"`
	TodayTasks         int64          `json:"today_tasks"`
	TodayFailures      int64          `json:"today_failures"`
	LastError          string         `json:"last_error"`
	LastTestAt         string         `json:"last_test_at"`
	RouteAvailable     bool           `json:"route_available"`
	RouteSkipReason    string         `json:"route_skip_reason"`
	CreatedAt          string         `json:"created_at"`
	UpdatedAt          string         `json:"updated_at"`
}

type videoDashboardResponse struct {
	TodayTasks        int64                           `json:"today_tasks"`
	SuccessRate       float64                         `json:"success_rate"`
	FailedTasks       int64                           `json:"failed_tasks"`
	QueuedTasks       int64                           `json:"queued_tasks"`
	RunningTasks      int64                           `json:"running_tasks"`
	ProviderStatus    []videoProviderStatusResponse   `json:"provider_status"`
	HealthDiagnostics []videoHealthDiagnosticResponse `json:"health_diagnostics"`
	RecentFailures    []videoTaskSummaryResponse      `json:"recent_failures"`
	RecentSuccesses   []videoTaskSummaryResponse      `json:"recent_successes"`
	UsageOverview     []service.VideoUsageSummary     `json:"usage_overview"`
}

type videoProviderStatusResponse struct {
	Provider         string `json:"provider"`
	DisplayName      string `json:"display_name"`
	Enabled          bool   `json:"enabled"`
	APIKeyConfigured bool   `json:"api_key_configured"`
	MaskedKey        string `json:"masked_key"`
	DefaultModel     string `json:"default_model"`
	KeyStatus        string `json:"key_status"`
	HealthStatus     string `json:"health_status"`
	DiagnosticType   string `json:"diagnostic_type"`
	SuggestedAction  string `json:"suggested_action"`
	RouteAvailable   bool   `json:"route_available"`
	RouteSkipReason  string `json:"route_skip_reason"`
	Priority         int    `json:"priority"`
	CurrentInflight  int64  `json:"current_inflight"`
	LastError        string `json:"last_error"`
	LastTestAt       string `json:"last_test_at"`
	UpdatedAt        string `json:"updated_at"`
	TodayTasks       int64  `json:"today_tasks"`
	RunningTasks     int64  `json:"running_tasks"`
	FailedTasks      int64  `json:"failed_tasks"`
}

type videoHealthDiagnosticResponse struct {
	Provider        string `json:"provider"`
	DisplayName     string `json:"display_name"`
	RouteAccount    string `json:"route_account"`
	KeyStatus       string `json:"key_status"`
	LastTestAt      string `json:"last_test_at"`
	ExceptionType   string `json:"exception_type"`
	ImpactTasks     int64  `json:"impact_tasks"`
	RecentError     string `json:"recent_error"`
	SuggestedAction string `json:"suggested_action"`
	Status          string `json:"status"`
}

type videoTaskSummaryResponse struct {
	ID                  int64   `json:"id"`
	ProviderAccountID   int64   `json:"provider_account_id"`
	ProviderAccountName string  `json:"provider_account_name"`
	Provider            string  `json:"provider"`
	Model               string  `json:"model"`
	TaskType            string  `json:"task_type"`
	Prompt              string  `json:"prompt"`
	Status              string  `json:"status"`
	ResultURL           string  `json:"result_url"`
	ErrorMessage        string  `json:"error_message"`
	CostEstimate        float64 `json:"cost_estimate"`
	Currency            string  `json:"currency"`
	CreatedBy           int64   `json:"created_by"`
	CreatedByEmail      string  `json:"created_by_email"`
	CreatedByName       string  `json:"created_by_name"`
	CreatedAt           string  `json:"created_at"`
	UpdatedAt           string  `json:"updated_at"`
	CompletedAt         *string `json:"completed_at"`
}

func (h *VideoHandler) ListProviders(c *gin.Context) {
	items, err := h.video.ListProviderAccounts(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	out := make([]videoProviderResponse, 0, len(items))
	for _, item := range items {
		out = append(out, videoProviderToResponse(item))
	}
	response.Success(c, gin.H{"items": out})
}

func (h *VideoHandler) CreateProvider(c *gin.Context) {
	var req videoProviderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ErrorFrom(c, infraerrors.BadRequest("VALIDATION_ERROR", err.Error()))
		return
	}
	enabled := false
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	rateLimit := 0
	if req.RateLimitPerMinute != nil {
		rateLimit = *req.RateLimitPerMinute
	}
	var metadata map[string]any
	if req.Metadata != nil {
		metadata = *req.Metadata
	}
	item, err := h.video.CreateProviderAccount(c.Request.Context(), service.VideoProviderCreateParams{
		Provider:           req.Provider,
		DisplayName:        req.DisplayName,
		Enabled:            enabled,
		APIKey:             req.APIKey,
		BaseURL:            req.BaseURL,
		DefaultModel:       req.DefaultModel,
		RateLimitPerMinute: rateLimit,
		Metadata:           metadata,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Created(c, videoProviderToResponse(item))
}

func (h *VideoHandler) UpdateProvider(c *gin.Context) {
	id, ok := parseVideoID(c, "id")
	if !ok {
		return
	}
	var req videoProviderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ErrorFrom(c, infraerrors.BadRequest("VALIDATION_ERROR", err.Error()))
		return
	}
	var displayName *string
	if req.DisplayName != "" {
		displayName = &req.DisplayName
	}
	var apiKey *string
	if req.APIKey != "" {
		apiKey = &req.APIKey
	}
	var baseURL *string
	if req.BaseURL != "" {
		baseURL = &req.BaseURL
	}
	var defaultModel *string
	if req.DefaultModel != "" {
		defaultModel = &req.DefaultModel
	}
	item, err := h.video.UpdateProviderAccount(c.Request.Context(), id, service.VideoProviderUpdateParams{
		DisplayName:        displayName,
		Enabled:            req.Enabled,
		APIKey:             apiKey,
		BaseURL:            baseURL,
		DefaultModel:       defaultModel,
		RateLimitPerMinute: req.RateLimitPerMinute,
		Metadata:           req.Metadata,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, videoProviderToResponse(item))
}

func (h *VideoHandler) TestProvider(c *gin.Context) {
	id, ok := parseVideoID(c, "id")
	if !ok {
		return
	}
	result, err := h.video.TestProviderAccount(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func (h *VideoHandler) Dashboard(c *gin.Context) {
	dashboard, err := h.video.Dashboard(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, videoDashboardToResponse(dashboard))
}

// ReliabilityReconciliation reports a deliberately narrow, read-only
// projection of reliability drift. It does not release reservations or invoke
// a provider; remediation remains an explicit operator action.
func (h *VideoHandler) ReliabilityReconciliation(c *gin.Context) {
	if h == nil || h.reconciler == nil {
		response.ErrorFrom(c, infraerrors.ServiceUnavailable("RELIABILITY_RECONCILIATION_UNAVAILABLE", "reliability reconciliation is unavailable"))
		return
	}
	limit := 100
	if raw := c.Query("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 || parsed > 1000 {
			response.ErrorFrom(c, infraerrors.BadRequest("INVALID_RELIABILITY_LIMIT", "limit must be between 1 and 1000"))
			return
		}
		limit = parsed
	}
	items, err := h.reconciler.DryRun(c.Request.Context(), time.Now().UTC(), limit)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"items": items})
}

func (h *VideoHandler) ProviderPool(c *gin.Context) {
	items, err := h.video.DramaProviderPool(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"items": items})
}

func (h *VideoHandler) RoutingEvents(c *gin.Context) {
	limit := 100
	if raw := c.Query("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			limit = parsed
		}
	}
	items, err := h.video.DramaRoutingEvents(c.Request.Context(), limit)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"items": items})
}

func (h *VideoHandler) SkillCards(c *gin.Context) {
	items, err := h.video.DramaSkillCards(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"items": items})
}

func (h *VideoHandler) SkillAnalysisExport(c *gin.Context) {
	var req service.DramaSkillAnalysisExportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ErrorFrom(c, infraerrors.BadRequest("VALIDATION_ERROR", err.Error()))
		return
	}
	export, err := h.video.GenerateDramaSkillAnalysisExport(c.Request.Context(), req)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Created(c, export)
}

func (h *VideoHandler) DramaEngineCapabilityMatrix(c *gin.Context) {
	response.Success(c, gin.H{"items": h.video.DramaEngineCapabilityMatrix()})
}

func videoProviderToResponse(item *service.VideoProviderAccount) videoProviderResponse {
	if item == nil {
		return videoProviderResponse{}
	}
	return videoProviderResponse{
		ID:                 item.ID,
		Provider:           item.Provider,
		DisplayName:        item.DisplayName,
		Enabled:            item.Enabled,
		APIKeyConfigured:   item.APIKeyConfigured,
		MaskedKey:          item.MaskedKey,
		BaseURL:            item.BaseURL,
		DefaultModel:       item.DefaultModel,
		RateLimitPerMinute: item.RateLimitPerMinute,
		Metadata:           item.Metadata,
		KeyStatus:          item.KeyStatus,
		HealthStatus:       item.HealthStatus,
		DiagnosticType:     item.DiagnosticType,
		SuggestedAction:    item.SuggestedAction,
		Priority:           item.Priority,
		CurrentInflight:    item.CurrentInflight,
		TodayTasks:         item.TodayTasks,
		TodayFailures:      item.TodayFailures,
		LastError:          item.LastError,
		LastTestAt:         formatOptionalVideoTime(item.LastTestAt),
		RouteAvailable:     item.RouteAvailable,
		RouteSkipReason:    item.RouteSkipReason,
		CreatedAt:          formatVideoTime(item.CreatedAt),
		UpdatedAt:          formatVideoTime(item.UpdatedAt),
	}
}

func videoDashboardToResponse(d *service.VideoDashboard) videoDashboardResponse {
	if d == nil {
		return videoDashboardResponse{}
	}
	providers := make([]videoProviderStatusResponse, 0, len(d.ProviderStatus))
	for _, p := range d.ProviderStatus {
		providers = append(providers, videoProviderStatusResponse{
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
			LastTestAt:       formatOptionalVideoTime(p.LastTestAt),
			UpdatedAt:        formatVideoTime(p.UpdatedAt),
			TodayTasks:       p.TodayTasks,
			RunningTasks:     p.RunningTasks,
			FailedTasks:      p.FailedTasks,
		})
	}
	diagnostics := make([]videoHealthDiagnosticResponse, 0, len(d.HealthDiagnostics))
	for _, item := range d.HealthDiagnostics {
		diagnostics = append(diagnostics, videoHealthDiagnosticResponse{
			Provider:        item.Provider,
			DisplayName:     item.DisplayName,
			RouteAccount:    item.RouteAccount,
			KeyStatus:       item.KeyStatus,
			LastTestAt:      formatOptionalVideoTime(item.LastTestAt),
			ExceptionType:   item.ExceptionType,
			ImpactTasks:     item.ImpactTasks,
			RecentError:     item.RecentError,
			SuggestedAction: item.SuggestedAction,
			Status:          item.Status,
		})
	}
	return videoDashboardResponse{
		TodayTasks:        d.TodayTasks,
		SuccessRate:       d.SuccessRate,
		FailedTasks:       d.FailedTasks,
		QueuedTasks:       d.QueuedTasks,
		RunningTasks:      d.RunningTasks,
		ProviderStatus:    providers,
		HealthDiagnostics: diagnostics,
		RecentFailures:    videoTaskSummaries(d.RecentFailures),
		RecentSuccesses:   videoTaskSummaries(d.RecentSuccesses),
		UsageOverview:     d.UsageOverview,
	}
}

func videoTaskSummaries(tasks []*service.VideoTask) []videoTaskSummaryResponse {
	out := make([]videoTaskSummaryResponse, 0, len(tasks))
	for _, task := range tasks {
		out = append(out, videoTaskSummary(task))
	}
	return out
}

func videoTaskSummary(task *service.VideoTask) videoTaskSummaryResponse {
	if task == nil {
		return videoTaskSummaryResponse{}
	}
	var completed *string
	if task.CompletedAt != nil {
		v := formatVideoTime(*task.CompletedAt)
		completed = &v
	}
	return videoTaskSummaryResponse{
		ID:                  task.ID,
		ProviderAccountID:   task.ProviderAccountID,
		ProviderAccountName: task.ProviderAccountName,
		Provider:            task.Provider,
		Model:               task.Model,
		TaskType:            task.TaskType,
		Prompt:              task.Prompt,
		Status:              task.Status,
		ResultURL:           task.ResultURL,
		ErrorMessage:        task.ErrorMessage,
		CostEstimate:        task.CostEstimate,
		Currency:            service.NormalizeBillingCurrency(task.Currency),
		CreatedBy:           task.CreatedBy,
		CreatedByEmail:      task.CreatedByEmail,
		CreatedByName:       task.CreatedByName,
		CreatedAt:           formatVideoTime(task.CreatedAt),
		UpdatedAt:           formatVideoTime(task.UpdatedAt),
		CompletedAt:         completed,
	}
}

func parseVideoID(c *gin.Context, name string) (int64, bool) {
	id, err := strconv.ParseInt(c.Param(name), 10, 64)
	if err != nil || id <= 0 {
		response.ErrorFrom(c, infraerrors.BadRequest("INVALID_VIDEO_ID", "invalid video id"))
		return 0, false
	}
	return id, true
}

func formatVideoTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func formatOptionalVideoTime(t *time.Time) string {
	if t == nil {
		return ""
	}
	return formatVideoTime(*t)
}
