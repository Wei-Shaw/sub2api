// Package handler — support_ticket_attachment_handler.go
//
// 用户端工单附件上传 handler。
//
// 单接口：
//
//	POST /api/v1/support/tickets/attachments
//	Content-Type: multipart/form-data
//	form fields:
//	  - file (required, single): 单张图片（png/jpeg），≤ 5 MB
//
// 返回：
//
//	{ code: 200, data: { key, url, size, mime } }
//
// 设计要点：
//   - 只允许单文件上传（form field 名固定为 "file"）——前端多张图片就调用多次，
//     便于逐张展示进度、逐张失败重试，避免整包 rollback 语义复杂化。
//   - handler 层通过 http.MaxBytesReader 卡在 5 MB + multipart 元数据一点小余量，
//     防止恶意大文件占满内存；service 层再按 magic bytes 校验 + 二次尺寸校验。
//   - 未登录 → 401；未开启功能 / COS 未配置 / 单张超限 / 非 png|jpg → 由 service
//     sentinel 走 ErrorFrom 统一映射（404/400），前端根据 code 精确提示。
package handler

import (
	"errors"
	"mime/multipart"
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// supportTicketAttachmentMaxBodyBytes 是整个 multipart 请求的硬上限：
//
//	图片本体 5 MB + form 元数据 & boundary 冗余 ≈ 32 KiB。
//
// 超出直接由 gin 的 MaxBytesReader 拒绝，避免 service 层浪费内存。
const supportTicketAttachmentMaxBodyBytes = service.SupportTicketImageMaxBytes + 32*1024

// SupportTicketAttachmentHandler 处理工单附件上传接口。
//
// 与 SupportTicketHandler 分开是因为它的 URL 前缀虽然同属 /support/tickets/，
// 但语义（multipart 上传）与 CRUD 完全独立，拆分让路由注册和职责都更清晰。
type SupportTicketAttachmentHandler struct {
	service *service.SupportTicketService
}

// NewSupportTicketAttachmentHandler 构造附件上传 handler。
func NewSupportTicketAttachmentHandler(svc *service.SupportTicketService) *SupportTicketAttachmentHandler {
	return &SupportTicketAttachmentHandler{service: svc}
}

// Upload 处理 POST /api/v1/support/tickets/attachments。
func (h *SupportTicketAttachmentHandler) Upload(c *gin.Context) {
	if _, ok := middleware2.GetAuthSubjectFromContext(c); !ok {
		response.Unauthorized(c, "User not found in context")
		return
	}

	// 兜底限制整个请求体：http.MaxBytesReader 会替换 c.Request.Body，
	// 后续 c.FormFile / c.Request.MultipartReader 都会自动共享这个上限。
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, supportTicketAttachmentMaxBodyBytes)

	fileHeader, err := c.FormFile("file")
	if err != nil {
		// MaxBytesReader 触发时 gin 会包装成 "http: request body too large"。
		// 这里显式判定，翻成 service 侧 sentinel，前端识别 code=SUPPORT_TICKET_IMAGE_TOO_LARGE。
		if isBodyTooLargeErr(err) {
			response.ErrorFrom(c, service.ErrSupportTicketImageTooLarge)
			return
		}
		response.BadRequest(c, "missing form field 'file': "+err.Error())
		return
	}
	if fileHeader == nil {
		response.BadRequest(c, "missing form field 'file'")
		return
	}
	// 快速失败：MultipartFile.Size 是可信任的（gin 已经完整解析过 body）。
	if fileHeader.Size <= 0 || fileHeader.Size > service.SupportTicketImageMaxBytes {
		response.ErrorFrom(c, service.ErrSupportTicketImageTooLarge)
		return
	}

	data, err := readMultipartFile(fileHeader)
	if err != nil {
		if isBodyTooLargeErr(err) {
			response.ErrorFrom(c, service.ErrSupportTicketImageTooLarge)
			return
		}
		response.BadRequest(c, "read upload body: "+err.Error())
		return
	}

	img, err := h.service.UploadAttachment(c.Request.Context(), service.SupportTicketAttachmentUpload{
		FileName:        fileHeader.Filename,
		ContentTypeHint: fileHeader.Header.Get("Content-Type"),
		Data:            data,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.SupportTicketImage{
		Key:  img.Key,
		URL:  img.URL,
		Size: img.Size,
		MIME: img.MIME,
	})
}

// readMultipartFile 打开并把 multipart file 内容一次性读入内存。
// 借助 service.ReadAttachmentBody 复用同一份体积上限，避免 handler / service 各写一遍。
func readMultipartFile(fh *multipart.FileHeader) ([]byte, error) {
	f, err := fh.Open()
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	return service.ReadAttachmentBody(f)
}

// isBodyTooLargeErr 识别 http.MaxBytesReader 触发的错误类型。
//
// stdlib 在 Go 1.19+ 返回 *http.MaxBytesError，历史版本返回带 "http: request body too large"
// 文本的普通 error。这里两种都处理，兼容将来 Go 升级 & 反向兼容旧运行时。
func isBodyTooLargeErr(err error) bool {
	if err == nil {
		return false
	}
	var maxBytesErr *http.MaxBytesError
	if errors.As(err, &maxBytesErr) {
		return true
	}
	return err.Error() == "http: request body too large"
}
