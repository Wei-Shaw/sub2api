// Package admin — support_doc_index_handler.go
//
// admin 端客服文档索引管线 HTTP handler。覆盖：
//
//   - POST /api/v1/admin/support/doc-index/rebuild  异步触发抓取
//     (返回 {accepted: true})；advisory lock 保证不并发。
//   - GET  /api/v1/admin/support/doc-index/status   查询最近一次的 pipeline 状态。
//   - POST /api/v1/admin/support/doc-index/purge    清空 doc_chunks 表（admin 二次确认后）。
package admin

import (
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// SupportDocIndexHandler 处理 admin 客服文档索引接口。
type SupportDocIndexHandler struct {
	pipeline       *service.SupportDocPipeline
	settingService *service.SettingService
	chunkRepo      service.SupportDocChunkRepository
}

// NewSupportDocIndexHandler 构造 handler。
func NewSupportDocIndexHandler(
	pipeline *service.SupportDocPipeline,
	settingService *service.SettingService,
	chunkRepo service.SupportDocChunkRepository,
) *SupportDocIndexHandler {
	return &SupportDocIndexHandler{
		pipeline:       pipeline,
		settingService: settingService,
		chunkRepo:      chunkRepo,
	}
}

// Rebuild 异步触发文档索引重建。
//
// 立即返回 {accepted: true}；pipeline 在 goroutine 中跑。
// advisory lock 拿不到时 goroutine 会自然退出（admin 通过 status 接口看到旧状态）。
func (h *SupportDocIndexHandler) Rebuild(c *gin.Context) {
	if h == nil || h.pipeline == nil {
		response.InternalError(c, "doc index pipeline not configured")
		return
	}
	h.pipeline.RunAsync()
	c.JSON(http.StatusAccepted, gin.H{"accepted": true})
}

// Status 查询最近一次 pipeline 状态。
//
// 返回 setting 中持久化的 SupportDocIndexStatus；不存在时返回 state="idle"。
func (h *SupportDocIndexHandler) Status(c *gin.Context) {
	st := h.settingService.GetSupportDocIndexStatus(c.Request.Context())
	out := dto.SupportDocIndexStatusResponse{
		State:             st.State,
		StartedAt:         st.StartedAt,
		LastRunAt:         st.LastRunAt,
		DurationSeconds:   st.DurationSeconds,
		PagesVisited:      st.PagesVisited,
		PagesCapHit:       st.PagesCapHit,
		ChunksTotal:       st.ChunksTotal,
		ChunksAdded:       st.ChunksAdded,
		ChunksRemoved:     st.ChunksRemoved,
		ChunksFailedEmbed: st.ChunksFailedEmbed,
		Errors:            make([]dto.SupportDocIndexErrorEntry, 0, len(st.Errors)),
	}
	for _, e := range st.Errors {
		out.Errors = append(out.Errors, dto.SupportDocIndexErrorEntry{
			URL: e.URL, Message: e.Message,
		})
	}
	response.Success(c, out)
}

// Purge 清空 doc_chunks 表；返回删除条数。admin UI 应在调用前做二次确认。
func (h *SupportDocIndexHandler) Purge(c *gin.Context) {
	deleted, err := h.chunkRepo.DeleteAll(c.Request.Context())
	if err != nil {
		response.InternalError(c, "purge doc chunks failed: "+err.Error())
		return
	}
	response.Success(c, gin.H{"deleted": deleted})
}
