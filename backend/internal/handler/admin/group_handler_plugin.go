package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/plugin"
	"github.com/gin-gonic/gin"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// SetPlatformRegistry sets the platform registry after construction.
// Used by wire_gen.go when pluginManager is created after GroupHandler.
func (h *GroupHandler) SetPlatformRegistry(registry *plugin.PlatformRegistry) {
	h.platformRegistry = registry
}

// validateGroupConfigResult holds the plugin validation outcome.
type validateGroupConfigResult struct {
	FieldErrors         map[string]string
	ProcessedGroupExtra map[string]any
}

// tryPluginValidateGroupConfig delegates group_extra validation to the
// plugin for plugin-owned platforms. Returns (result, err):
//   - result==nil, err==nil  -> plugin unavailable / Unimplemented; skip validation
//   - result!=nil, err==nil  -> validation completed; check FieldErrors / ProcessedGroupExtra
//   - err!=nil               -> transient plugin failure (crash / network); caller should
//     not fall through to "skip" silently.
func (h *GroupHandler) tryPluginValidateGroupConfig(
	ctx context.Context,
	platform string,
	groupExtra map[string]any,
	isUpdate bool,
) (*validateGroupConfigResult, error) {
	if h.platformRegistry == nil || len(groupExtra) == 0 {
		return nil, nil
	}

	client, err := plugin.ClientForPlatform(h.platformRegistry, platform)
	if err != nil {
		return nil, nil
	}

	extraJSON, err := json.Marshal(groupExtra)
	if err != nil {
		return nil, fmt.Errorf("marshal group_extra for plugin validation: %w", err)
	}

	resp, err := client.ValidateGroupConfig(ctx, platform, extraJSON, isUpdate)
	if err != nil {
		if s, ok := status.FromError(err); ok && s.Code() == codes.Unimplemented {
			return nil, nil
		}
		return nil, fmt.Errorf("plugin validation failed: %w", err)
	}

	result := &validateGroupConfigResult{
		FieldErrors: resp.GetFieldErrors(),
	}
	if len(resp.GetProcessedGroupExtraJson()) > 0 {
		var processed map[string]any
		if err := json.Unmarshal(resp.GetProcessedGroupExtraJson(), &processed); err == nil {
			result.ProcessedGroupExtra = processed
		}
	}
	return result, nil
}

// applyGroupConfigValidation runs plugin-side group_extra validation and
// either writes an error response (returning true to signal "handled") or
// mutates groupExtra in place with the processed output. Returns true when
// the caller should abort.
func (h *GroupHandler) applyGroupConfigValidation(
	c *gin.Context,
	platform string,
	groupExtra map[string]any,
	isUpdate bool,
) (processedExtra map[string]any, abort bool) {
	vr, err := h.tryPluginValidateGroupConfig(
		c.Request.Context(), platform, groupExtra, isUpdate,
	)
	if err != nil {
		response.InternalError(c, err.Error())
		return nil, true
	}
	if vr == nil {
		return nil, false
	}
	if len(vr.FieldErrors) > 0 {
		metadata := make(map[string]string, len(vr.FieldErrors))
		for field, msg := range vr.FieldErrors {
			metadata["field."+field] = msg
		}
		response.ErrorWithDetails(c, http.StatusBadRequest,
			"plugin validation failed", "PLUGIN_VALIDATION_ERROR", metadata)
		return nil, true
	}
	return vr.ProcessedGroupExtra, false
}
