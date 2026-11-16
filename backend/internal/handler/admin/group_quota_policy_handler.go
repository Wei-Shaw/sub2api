package admin

import (
	"context"
	"encoding/json"
	"io"
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type groupQuotaPolicyService interface {
	Get(context.Context, int64) (*service.GroupQuotaPolicyView, error)
	Save(context.Context, int64, int64, service.GroupQuotaPolicyConfig) (*service.GroupQuotaPolicyView, error)
	Preview(context.Context, int64, service.GroupQuotaPolicyConfig, string) (*service.GroupQuotaPolicyPreview, error)
	Refresh(context.Context, int64) (*service.GroupQuotaPolicyView, error)
	Release(context.Context, int64, int64, string) (*service.GroupQuotaPolicyView, error)
	Models(context.Context, int64) (*service.GroupQuotaModels, error)
	ValidateMapping(context.Context, int64, service.GroupQuotaMappingRequest) (*service.GroupQuotaMappingValidation, error)
}

func (h *GroupHandler) SetQuotaPolicyService(s *service.GroupQuotaPolicyService) {
	if s == nil {
		h.quotaPolicyService = nil
		return
	}
	h.quotaPolicyService = s
}

func (h *GroupHandler) quotaPolicyGroupID(c *gin.Context) (int64, bool) {
	if h.rejectUnsupportedSimpleModeOperation(c, "quota_policy") {
		return 0, false
	}
	id, ok := parsePositiveIDParam(c, "id")
	if !ok {
		return 0, false
	}
	if h.quotaPolicyService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Quota policy service is unavailable")
		return 0, false
	}
	return id, true
}

// Policies contain only bounded configuration, never inference payloads.
func decodeQuotaPolicyRequest(c *gin.Context, target any) bool {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 128<<10)
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		response.BadRequest(c, "Invalid quota policy request")
		return false
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		response.BadRequest(c, "Expected a single quota policy request")
		return false
	}
	return true
}

func (h *GroupHandler) GetQuotaPolicy(c *gin.Context) {
	id, ok := h.quotaPolicyGroupID(c)
	if !ok {
		return
	}
	result, err := h.quotaPolicyService.Get(c.Request.Context(), id)
	if !response.ErrorFrom(c, err) {
		response.Success(c, result)
	}
}

func (h *GroupHandler) GetQuotaPolicyModels(c *gin.Context) {
	id, ok := h.quotaPolicyGroupID(c)
	if !ok {
		return
	}
	result, err := h.quotaPolicyService.Models(c.Request.Context(), id)
	if !response.ErrorFrom(c, err) {
		response.Success(c, result)
	}
}

func (h *GroupHandler) ValidateQuotaPolicyMapping(c *gin.Context) {
	id, ok := h.quotaPolicyGroupID(c)
	if !ok {
		return
	}
	var req service.GroupQuotaMappingRequest
	if !decodeQuotaPolicyRequest(c, &req) {
		return
	}
	result, err := h.quotaPolicyService.ValidateMapping(c.Request.Context(), id, req)
	if !response.ErrorFrom(c, err) {
		response.Success(c, result)
	}
}

func (h *GroupHandler) SaveQuotaPolicy(c *gin.Context) {
	id, ok := h.quotaPolicyGroupID(c)
	if !ok {
		return
	}
	var req struct {
		Revision *int64                          `json:"revision"`
		Config   *service.GroupQuotaPolicyConfig `json:"config"`
	}
	if !decodeQuotaPolicyRequest(c, &req) {
		return
	}
	if req.Revision == nil || *req.Revision < 0 || req.Config == nil {
		response.BadRequest(c, "revision and config are required")
		return
	}
	result, err := h.quotaPolicyService.Save(c.Request.Context(), id, *req.Revision, *req.Config)
	if !response.ErrorFrom(c, err) {
		response.Success(c, result)
	}
}

func (h *GroupHandler) PreviewQuotaPolicy(c *gin.Context) {
	id, ok := h.quotaPolicyGroupID(c)
	if !ok {
		return
	}
	var req struct {
		Config *service.GroupQuotaPolicyConfig `json:"config"`
		Model  string                          `json:"model"`
	}
	if !decodeQuotaPolicyRequest(c, &req) {
		return
	}
	if req.Config == nil || len(req.Model) > 256 {
		response.BadRequest(c, "A valid config and model are required")
		return
	}
	result, err := h.quotaPolicyService.Preview(c.Request.Context(), id, *req.Config, req.Model)
	if !response.ErrorFrom(c, err) {
		response.Success(c, result)
	}
}

func (h *GroupHandler) RefreshQuotaPolicy(c *gin.Context) {
	id, ok := h.quotaPolicyGroupID(c)
	if !ok {
		return
	}
	result, err := h.quotaPolicyService.Refresh(c.Request.Context(), id)
	if !response.ErrorFrom(c, err) {
		response.Success(c, result)
	}
}

func (h *GroupHandler) ReleaseQuotaPolicy(c *gin.Context) {
	id, ok := h.quotaPolicyGroupID(c)
	if !ok {
		return
	}
	var req struct {
		Revision *int64 `json:"revision"`
		RuleID   string `json:"rule_id"`
	}
	if !decodeQuotaPolicyRequest(c, &req) {
		return
	}
	if req.Revision == nil || *req.Revision < 0 || req.RuleID == "" || len(req.RuleID) > 100 {
		response.BadRequest(c, "revision and rule_id are required")
		return
	}
	result, err := h.quotaPolicyService.Release(c.Request.Context(), id, *req.Revision, req.RuleID)
	if !response.ErrorFrom(c, err) {
		response.Success(c, result)
	}
}
