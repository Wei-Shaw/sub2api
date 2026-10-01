package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func (h *AccountHandler) SearchModelMetadata(c *gin.Context) {
	if h.accountTestService == nil {
		response.BadRequest(c, "Model metadata search is unavailable")
		return
	}
	if len(c.Query("q")) > 200 {
		response.BadRequest(c, "Search query is too long")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
	defer cancel()
	results, err := h.accountTestService.SearchModelConfigs(ctx, c.Query("q"))
	if err != nil {
		response.Error(c, 502, "Unable to load models.dev catalogue")
		return
	}
	response.Success(c, results)
}

func (h *AccountHandler) GetModelMetadataCatalog(c *gin.Context) {
	if h.accountTestService == nil {
		response.BadRequest(c, "Model metadata catalogue is unavailable")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 45*time.Second)
	defer cancel()
	results, err := h.accountTestService.ListModelConfigs(ctx)
	if err != nil {
		response.Error(c, 502, "Unable to load models.dev catalogue")
		return
	}
	response.Success(c, results)
}

func (h *AccountHandler) GroupUpstreamModelConfig(ctx context.Context, group *service.Group, model string) (map[string]json.RawMessage, error) {
	if h.accountTestService == nil {
		return nil, fmt.Errorf("upstream discovery is unavailable")
	}
	return h.accountTestService.FetchGroupUpstreamModelConfig(ctx, group, model)
}
