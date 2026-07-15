package admin

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/promptcompression/rtk"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// PromptCompressionHandler exposes the RTK control plane. The handler remains
// useful while the engine is disabled: operators can inspect the deployment
// guard, publish a policy snapshot, preview a payload, and emergency-stop the
// feature without touching gateway handlers.
type PromptCompressionHandler struct {
	service *service.PromptCompressionService
}

func NewPromptCompressionHandler(svc *service.PromptCompressionService) *PromptCompressionHandler {
	return &PromptCompressionHandler{service: svc}
}

func (h *PromptCompressionHandler) unavailable(c *gin.Context) bool {
	if h == nil || h.service == nil {
		response.Error(c, http.StatusServiceUnavailable, "Prompt compression service not available")
		return true
	}
	return false
}

func (h *PromptCompressionHandler) GetStatus(c *gin.Context) {
	if h.unavailable(c) {
		return
	}
	response.Success(c, h.service.Status())
}

func (h *PromptCompressionHandler) GetConfig(c *gin.Context) {
	if h.unavailable(c) {
		return
	}
	response.Success(c, h.service.Snapshot())
}

type promptCompressionPolicyRequest struct {
	Enabled              *bool    `json:"enabled"`
	Mode                 string   `json:"mode"`
	Intensity            string   `json:"intensity"`
	ProfileVersionID     string   `json:"profile_version_id"`
	FilterPackVersionID  string   `json:"filter_pack_version_id"`
	RolloutPercent       *int     `json:"rollout_percent"`
	HoldoutPercent       *int     `json:"holdout_percent"`
	AllowedProtocols     []string `json:"allowed_protocols"`
	MinCandidateTokens   *int     `json:"min_candidate_tokens"`
	MinSavingsTokens     *int     `json:"min_savings_tokens"`
	MaxBodyBytes         *int64   `json:"max_body_bytes"`
	MaxResultBytes       *int64   `json:"max_result_bytes"`
	MaxDurationMS        *int     `json:"max_duration_ms"`
	AllowRequestOverride *bool    `json:"allow_request_override"`
}

func (h *PromptCompressionHandler) UpdateConfig(c *gin.Context) {
	if h.unavailable(c) {
		return
	}
	var req promptCompressionPolicyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	p := h.service.Snapshot()
	if req.Enabled != nil {
		p.Enabled = *req.Enabled
	}
	if req.Mode != "" {
		p.Mode = strings.ToLower(strings.TrimSpace(req.Mode))
	}
	if req.Intensity != "" {
		p.Intensity = strings.ToLower(strings.TrimSpace(req.Intensity))
	}
	if req.ProfileVersionID != "" {
		p.ProfileVersionID = strings.TrimSpace(req.ProfileVersionID)
	}
	if req.FilterPackVersionID != "" {
		p.FilterPackVersionID = strings.TrimSpace(req.FilterPackVersionID)
	}
	if req.RolloutPercent != nil {
		p.RolloutPercent = *req.RolloutPercent
	}
	if req.HoldoutPercent != nil {
		p.HoldoutPercent = *req.HoldoutPercent
	}
	if req.AllowedProtocols != nil {
		p.AllowedProtocols = req.AllowedProtocols
	}
	if req.MinCandidateTokens != nil {
		p.MinCandidateTokens = *req.MinCandidateTokens
	}
	if req.MinSavingsTokens != nil {
		p.MinSavingsTokens = *req.MinSavingsTokens
	}
	if req.MaxBodyBytes != nil {
		p.MaxBodyBytes = *req.MaxBodyBytes
	}
	if req.MaxResultBytes != nil {
		p.MaxResultBytes = *req.MaxResultBytes
	}
	if req.MaxDurationMS != nil {
		p.MaxDurationMS = *req.MaxDurationMS
	}
	if req.AllowRequestOverride != nil {
		p.AllowRequestOverride = *req.AllowRequestOverride
	}
	if err := h.service.UpdatePolicy(p); err != nil {
		response.Error(c, http.StatusUnprocessableEntity, err.Error())
		return
	}
	response.Success(c, h.service.Snapshot())
}

type promptCompressionEmergencyRequest struct {
	Reason string `json:"reason"`
}

func (h *PromptCompressionHandler) EmergencyStop(c *gin.Context) {
	if h.unavailable(c) {
		return
	}
	var req promptCompressionEmergencyRequest
	if err := c.ShouldBindJSON(&req); err != nil && err.Error() != "EOF" {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	h.service.EmergencyStop(adminActor(c), strings.TrimSpace(req.Reason))
	response.Success(c, h.service.Status())
}

func (h *PromptCompressionHandler) Resume(c *gin.Context) {
	if h.unavailable(c) {
		return
	}
	var req promptCompressionEmergencyRequest
	if err := c.ShouldBindJSON(&req); err != nil && err.Error() != "EOF" {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	h.service.Resume(adminActor(c), strings.TrimSpace(req.Reason))
	response.Success(c, h.service.Status())
}

type promptCompressionPreviewRequest struct {
	Protocol string          `json:"protocol"`
	Body     json.RawMessage `json:"body"`
}

func (h *PromptCompressionHandler) Preview(c *gin.Context) {
	if h.unavailable(c) {
		return
	}
	var req promptCompressionPreviewRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if len(req.Body) == 0 {
		response.BadRequest(c, "body is required")
		return
	}
	response.Success(c, h.service.Preview(c.Request.Context(), req.Protocol, []byte(req.Body)))
}

func (h *PromptCompressionHandler) ListTelemetry(c *gin.Context) {
	if h.unavailable(c) {
		return
	}
	// The service owns the bounded, non-blocking queue. This endpoint is a
	// diagnostic drain and is intentionally limited to avoid exposing payloads.
	response.Success(c, h.service.DrainTelemetry(100))
}

func (h *PromptCompressionHandler) GetGroupPolicy(c *gin.Context) {
	if h.unavailable(c) {
		return
	}
	id, err := strconv.ParseInt(c.Param("group_id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid group ID")
		return
	}
	policy := h.service.GroupPolicy(id)
	if policy == nil {
		policy = &service.PromptCompressionGroupPolicy{SchemaVersion: 1, Mode: "inherit"}
	}
	response.Success(c, policy)
}

func (h *PromptCompressionHandler) UpdateGroupPolicy(c *gin.Context) {
	if h.unavailable(c) {
		return
	}
	id, err := strconv.ParseInt(c.Param("group_id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid group ID")
		return
	}
	var policy service.PromptCompressionGroupPolicy
	if err := c.ShouldBindJSON(&policy); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if err := h.service.UpdateGroupPolicy(id, policy); err != nil {
		response.Error(c, http.StatusUnprocessableEntity, err.Error())
		return
	}
	response.Success(c, h.service.GroupPolicy(id))
}

func (h *PromptCompressionHandler) ListFilterPacks(c *gin.Context) {
	if h.unavailable(c) {
		return
	}
	response.Success(c, h.service.ListFilterPacks())
}

type promptCompressionFilterPackRequest struct {
	ID      string       `json:"id"`
	Filters []rtk.Filter `json:"filters"`
}

func (h *PromptCompressionHandler) ValidateFilterPack(c *gin.Context) {
	if h.unavailable(c) {
		return
	}
	var req promptCompressionFilterPackRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if err := h.service.ValidateFilterPack(req.Filters); err != nil {
		response.Error(c, http.StatusUnprocessableEntity, err.Error())
		return
	}
	response.Success(c, gin.H{"valid": true, "filter_count": len(req.Filters)})
}

func (h *PromptCompressionHandler) PublishFilterPack(c *gin.Context) {
	if h.unavailable(c) {
		return
	}
	var req promptCompressionFilterPackRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if err := h.service.PublishFilterPack(service.PromptCompressionFilterPack{ID: req.ID, Filters: req.Filters}); err != nil {
		response.Error(c, http.StatusUnprocessableEntity, err.Error())
		return
	}
	response.Success(c, h.service.ListFilterPacks())
}

func adminActor(c *gin.Context) string {
	if c == nil {
		return "admin"
	}
	if value, ok := c.Get(string(middleware.ContextKeyUser)); ok {
		if subject, ok := value.(middleware.AuthSubject); ok && subject.UserID > 0 {
			return strconv.FormatInt(subject.UserID, 10)
		}
	}
	return "admin"
}
