package admin

import (
	"strconv"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type CodexIdentityTemplateHandler struct {
	service *service.CodexIdentityTemplateService
}

func NewCodexIdentityTemplateHandler(service *service.CodexIdentityTemplateService) *CodexIdentityTemplateHandler {
	return &CodexIdentityTemplateHandler{service: service}
}

type codexIdentityTemplateCreateRequest struct {
	Name               string                                 `json:"name" binding:"required,max=100"`
	Description        string                                 `json:"description" binding:"max=500"`
	SessionPolicy      service.CodexSessionPolicySpec         `json:"session_policy"`
	AffinityTTLSeconds int                                    `json:"affinity_ttl_seconds"`
	UnsupportedPolicy  service.CodexUnsupportedProfilePolicy  `json:"unsupported_policy"`
	Profiles           []service.CodexIdentityTemplateProfile `json:"profiles" binding:"required,min=1"`
}

type codexIdentityTemplateUpdateRequest struct {
	ExpectedRevision        int64                                  `json:"expected_revision" binding:"required,min=1"`
	ConfirmAssignedAccounts bool                                   `json:"confirm_assigned_accounts"`
	Name                    string                                 `json:"name" binding:"required,max=100"`
	Description             string                                 `json:"description" binding:"max=500"`
	SessionPolicy           service.CodexSessionPolicySpec         `json:"session_policy"`
	AffinityTTLSeconds      int                                    `json:"affinity_ttl_seconds"`
	UnsupportedPolicy       service.CodexUnsupportedProfilePolicy  `json:"unsupported_policy"`
	Profiles                []service.CodexIdentityTemplateProfile `json:"profiles" binding:"required,min=1"`
}

func (r codexIdentityTemplateCreateRequest) serviceInput() service.CodexIdentityTemplateCreateInput {
	return service.CodexIdentityTemplateCreateInput{
		Name:               r.Name,
		Description:        r.Description,
		SessionPolicy:      r.SessionPolicy,
		AffinityTTLSeconds: r.AffinityTTLSeconds,
		UnsupportedPolicy:  r.UnsupportedPolicy,
		Profiles:           r.Profiles,
	}
}

func parseCodexIdentityTemplateID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.ErrorFrom(c, infraerrors.BadRequest(
			"INVALID_CODEX_IDENTITY_TEMPLATE_ID",
			"invalid Codex identity template id",
		))
		return 0, false
	}
	return id, true
}

// List handles GET /api/v1/admin/settings/codex-identity-templates.
func (h *CodexIdentityTemplateHandler) List(c *gin.Context) {
	items, err := h.service.List(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"items": items})
}

// Get handles GET /api/v1/admin/settings/codex-identity-templates/:id.
func (h *CodexIdentityTemplateHandler) Get(c *gin.Context) {
	id, ok := parseCodexIdentityTemplateID(c)
	if !ok {
		return
	}
	template, err := h.service.Get(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, template)
}

// Create handles POST /api/v1/admin/settings/codex-identity-templates.
func (h *CodexIdentityTemplateHandler) Create(c *gin.Context) {
	var req codexIdentityTemplateCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ErrorFrom(c, infraerrors.BadRequest("VALIDATION_ERROR", err.Error()))
		return
	}
	template, err := h.service.Create(c.Request.Context(), req.serviceInput())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Created(c, template)
}

// Update handles full replacement with optimistic concurrency.
// PUT /api/v1/admin/settings/codex-identity-templates/:id.
func (h *CodexIdentityTemplateHandler) Update(c *gin.Context) {
	id, ok := parseCodexIdentityTemplateID(c)
	if !ok {
		return
	}
	var req codexIdentityTemplateUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ErrorFrom(c, infraerrors.BadRequest("VALIDATION_ERROR", err.Error()))
		return
	}
	template, err := h.service.Update(c.Request.Context(), id, service.CodexIdentityTemplateUpdateInput{
		CodexIdentityTemplateCreateInput: service.CodexIdentityTemplateCreateInput{
			Name:               req.Name,
			Description:        req.Description,
			SessionPolicy:      req.SessionPolicy,
			AffinityTTLSeconds: req.AffinityTTLSeconds,
			UnsupportedPolicy:  req.UnsupportedPolicy,
			Profiles:           req.Profiles,
		},
		ExpectedRevision:        req.ExpectedRevision,
		ConfirmAssignedAccounts: req.ConfirmAssignedAccounts,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, template)
}

// Delete handles DELETE /api/v1/admin/settings/codex-identity-templates/:id.
func (h *CodexIdentityTemplateHandler) Delete(c *gin.Context) {
	id, ok := parseCodexIdentityTemplateID(c)
	if !ok {
		return
	}
	if err := h.service.Delete(c.Request.Context(), id); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"message": "Codex identity template deleted"})
}
