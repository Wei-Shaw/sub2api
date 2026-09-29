package handler

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// 4 张 20 MiB 参考图的 base64 与 JSON 开销。
const imageStudioMaxRequestBytes = 128 << 20

// imageStudioForwardedHeaders 回放时透传，使网关的 Key IP 白名单/审计看到真实客户端。
var imageStudioForwardedHeaders = []string{"X-Forwarded-For", "X-Real-IP", "CF-Connecting-IP", "User-Agent"}

type ImageStudioHandler struct {
	studio *service.ImageStudioService
	// gateway 是完整的 HTTP 路由；任务经它回放 /v1/images/*，复用网关全部鉴权、计费与审核。
	gateway http.Handler
}

func NewImageStudioHandler(studio *service.ImageStudioService) *ImageStudioHandler {
	return &ImageStudioHandler{studio: studio}
}

// SetGateway 在路由构建完成后、开始服务前注入（路由依赖 handler，无法经 DI 注入）。
func (h *ImageStudioHandler) SetGateway(gateway http.Handler) {
	h.gateway = gateway
}

// CreateJob POST /api/v1/image-studio/jobs
func (h *ImageStudioHandler) CreateJob(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, imageStudioMaxRequestBytes)
	var in service.ImageStudioCreateInput
	if err := c.ShouldBindJSON(&in); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
	}
	sub, err := h.studio.Submit(c.Request.Context(), subject.UserID, in)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	execute := h.executor(c)
	go func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				// 任务保持 running，超时后由 FailStaleJobs 收尾。
				logger.L().Error("image_studio.run_panicked", zap.Int64("job_id", sub.Job.ID), zap.Any("panic", recovered))
			}
		}()
		h.studio.Run(sub, execute)
	}()
	response.Accepted(c, sub.Job)
}

// executor 捕获发起请求的客户端信息，返回经网关回放的执行器。
func (h *ImageStudioHandler) executor(c *gin.Context) service.ImageStudioExecutor {
	remoteAddr := c.Request.RemoteAddr
	forwarded := make(http.Header)
	for _, name := range imageStudioForwardedHeaders {
		if value := c.GetHeader(name); value != "" {
			forwarded.Set(name, value)
		}
	}
	return func(ctx context.Context, path, apiKey string, body []byte) (int, []byte) {
		if h.gateway == nil {
			return http.StatusServiceUnavailable, []byte(`{"error":{"message":"image gateway is unavailable"}}`)
		}
		// 回放写入 recorder，关闭 JSON 心跳以保留上游真实状态码。
		req, err := http.NewRequestWithContext(service.WithOpenAIImagesJSONKeepaliveDisabled(ctx), http.MethodPost, path, bytes.NewReader(body))
		if err != nil {
			return http.StatusInternalServerError, []byte(`{"error":{"message":"failed to build image request"}}`)
		}
		req.RemoteAddr = remoteAddr
		for name, values := range forwarded {
			req.Header[name] = values
		}
		req.Header.Set("Authorization", "Bearer "+apiKey)
		req.Header.Set("Content-Type", "application/json")
		recorder := httptest.NewRecorder()
		h.gateway.ServeHTTP(recorder, req)
		return recorder.Code, recorder.Body.Bytes()
	}
}

// ListJobs GET /api/v1/image-studio/jobs
func (h *ImageStudioHandler) ListJobs(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	page, pageSize := response.ParsePagination(c)
	jobs, total, err := h.studio.ListJobs(c.Request.Context(), subject.UserID, page, pageSize)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, jobs, total, page, pageSize)
}

// GetJob GET /api/v1/image-studio/jobs/:id
func (h *ImageStudioHandler) GetJob(c *gin.Context) {
	subject, id, ok := imageStudioSubjectAndID(c)
	if !ok {
		return
	}
	job, err := h.studio.GetJob(c.Request.Context(), subject.UserID, id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, job)
}

// DeleteJob DELETE /api/v1/image-studio/jobs/:id
func (h *ImageStudioHandler) DeleteJob(c *gin.Context) {
	subject, id, ok := imageStudioSubjectAndID(c)
	if !ok {
		return
	}
	if err := h.studio.DeleteJob(c.Request.Context(), subject.UserID, id); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, nil)
}

// ListAssets GET /api/v1/image-studio/assets
func (h *ImageStudioHandler) ListAssets(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	page, pageSize := response.ParsePagination(c)
	assets, total, err := h.studio.ListAssets(c.Request.Context(), subject.UserID, page, pageSize)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, assets, total, page, pageSize)
}

// DeleteAsset DELETE /api/v1/image-studio/assets/:id
func (h *ImageStudioHandler) DeleteAsset(c *gin.Context) {
	subject, id, ok := imageStudioSubjectAndID(c)
	if !ok {
		return
	}
	if err := h.studio.DeleteAsset(c.Request.Context(), subject.UserID, id); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, nil)
}

// AssetFile GET /api/v1/image-studio/assets/:id/file[?download=1]
func (h *ImageStudioHandler) AssetFile(c *gin.Context) {
	subject, id, ok := imageStudioSubjectAndID(c)
	if !ok {
		return
	}
	asset, body, err := h.studio.OpenAsset(c.Request.Context(), subject.UserID, id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	defer func() { _ = body.Close() }()
	disposition := "inline"
	if c.Query("download") == "1" {
		disposition = "attachment"
	}
	c.Header("Content-Type", asset.MimeType)
	c.Header("Content-Length", strconv.FormatInt(asset.Bytes, 10))
	c.Header("Content-Disposition", fmt.Sprintf(`%s; filename="image-%d%s"`, disposition, asset.ID, imageStudioExtension(asset.MimeType)))
	c.Header("Cache-Control", "private, no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Status(http.StatusOK)
	if _, err := io.Copy(c.Writer, body); err != nil {
		logger.L().Warn("image_studio.asset_stream_failed", zap.Int64("asset_id", asset.ID), zap.Error(err))
	}
}

func imageStudioSubjectAndID(c *gin.Context) (middleware2.AuthSubject, int64, bool) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return subject, 0, false
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid ID")
		return subject, 0, false
	}
	return subject, id, true
}

func imageStudioExtension(mimeType string) string {
	switch mimeType {
	case "image/jpeg":
		return ".jpg"
	case "image/webp":
		return ".webp"
	default:
		return ".png"
	}
}
