package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func (h *GroupHandler) SetCodexModelCatalog(load func(context.Context, *service.Group) ([]byte, error)) {
	h.codexModelCatalog = load
}
func (h *GroupHandler) SetCodexModelUpstream(load func(context.Context, *service.Group, string) (map[string]json.RawMessage, error)) {
	h.codexModelUpstream = load
}

func (h *GroupHandler) baseCodexModels(ctx context.Context, group *service.Group) ([]map[string]json.RawMessage, error) {
	if h.codexModelCatalog == nil {
		return nil, fmt.Errorf("model catalogue is not configured")
	}
	copy := *group
	copy.CodexModelsManifestConfig.ModelOverrides = nil
	body, err := h.codexModelCatalog(ctx, &copy)
	if err != nil {
		return nil, err
	}
	var catalog struct {
		Models []map[string]json.RawMessage `json:"models"`
	}
	if err := json.Unmarshal(body, &catalog); err != nil {
		return nil, err
	}
	return catalog.Models, nil
}

// GetCodexModelConfig 返回未覆盖的基线，避免编辑时把覆盖值误认为上游信息。
func (h *GroupHandler) GetCodexModelConfig(c *gin.Context) {
	h.serveCodexModelConfig(c, nil, c.Query("model"))
}

// PreviewCodexModelConfig 只在分组副本上应用弹窗白名单草稿，不写入持久化配置。
func (h *GroupHandler) PreviewCodexModelConfig(c *gin.Context) {
	var req struct {
		ModelAllowlist *service.GroupModelAllowlist `json:"model_allowlist"`
		Model          string                       `json:"model"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid model catalogue preview: "+err.Error())
		return
	}
	h.serveCodexModelConfig(c, req.ModelAllowlist, req.Model)
}

func (h *GroupHandler) serveCodexModelConfig(c *gin.Context, allowlist *service.GroupModelAllowlist, model string) {
	if h.rejectUnsupportedSimpleModeOperation(c, "advanced") {
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid group ID")
		return
	}
	group, err := h.adminService.GetGroup(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if allowlist != nil {
		copy := *group
		copy.ModelAllowlist = *allowlist
		group = &copy
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 45*time.Second)
	defer cancel()
	models, err := h.baseCodexModels(ctx, group)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if model != "" {
		found := false
		for _, entry := range models {
			var slug string
			_ = json.Unmarshal(entry["slug"], &slug)
			found = found || slug == model
		}
		if !found {
			response.BadRequest(c, "Model is not available in this group")
			return
		}
		if h.codexModelUpstream == nil {
			response.BadRequest(c, "Upstream model discovery is unavailable")
			return
		}
		fields, err := h.codexModelUpstream(ctx, group, model)
		if err != nil {
			response.ErrorFrom(c, err)
			return
		}
		response.Success(c, gin.H{"fields": fields})
		return
	}
	response.Success(c, gin.H{"models": models})
}

func (h *GroupHandler) validateModelOverrideUpdate(ctx context.Context, id int64, req *UpdateGroupRequest) error {
	if req.CodexModelsManifestConfig == nil {
		return nil
	}
	patches := req.CodexModelsManifestConfig.ModelOverrides
	if err := service.ValidateCodexModelOverrides(patches); err != nil {
		return err
	}
	if len(patches) == 0 {
		return nil
	}
	group, err := h.adminService.GetGroup(ctx, id)
	if err != nil {
		return err
	}
	// 未改动的旧配置可以保留，暂时不可用的模型不会因此阻塞其它分组编辑。
	changed := make(map[string]bool)
	for model, fields := range patches {
		if !modelOverrideFieldsEqual(fields, group.CodexModelsManifestConfig.ModelOverrides[model]) {
			changed[model] = true
		}
	}
	if len(changed) == 0 {
		return nil
	}
	copy := *group
	if req.Platform != "" {
		copy.Platform = req.Platform
	}
	if req.ModelAllowlist != nil {
		copy.ModelAllowlist = *req.ModelAllowlist
	}
	copy.CodexModelsManifestConfig = *req.CodexModelsManifestConfig
	models, err := h.baseCodexModels(ctx, &copy)
	if err != nil {
		return err
	}
	for _, entry := range models {
		var slug string
		_ = json.Unmarshal(entry["slug"], &slug)
		if changed[slug] {
			if err := service.ValidateEffectiveCodexModelOverride(slug, entry, patches[slug]); err != nil {
				return err
			}
		}
		delete(changed, slug)
	}
	for model := range changed {
		return fmt.Errorf("model %q is not available in this group", model)
	}
	return nil
}

// JSONB 与浏览器会重新排版 JSON；比较语义，避免未改配置触发上游可用性检查。
func modelOverrideFieldsEqual(left, right map[string]json.RawMessage) bool {
	decode := func(fields map[string]json.RawMessage) any {
		body, _ := json.Marshal(fields)
		var value any
		_ = json.Unmarshal(body, &value)
		return value
	}
	return reflect.DeepEqual(decode(left), decode(right))
}
