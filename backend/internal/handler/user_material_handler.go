// Package handler: 用户素材库 HTTP handler。
//
// 路由挂在 /api/v1/user/materials 下（JWT 保护，见 routes/user.go）。
// 权限：所有接口只能操作当前登录用户的素材（内部通过 subject.UserID 过滤）。
package handler

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// UserMaterialHandler 编辑器/演练台图片输入控件 + 独立素材库页面依赖的用户端 API。
type UserMaterialHandler struct {
	svc *service.UserMaterialService
}

func NewUserMaterialHandler(svc *service.UserMaterialService) *UserMaterialHandler {
	return &UserMaterialHandler{svc: svc}
}

// userMaterialItem 是对外 DTO；与 service.UserMaterial 解耦以便未来加字段不破坏协议。
type userMaterialItem struct {
	ID          int64  `json:"id"`
	FileName    string `json:"file_name"`
	URL         string `json:"url"`
	ContentType string `json:"content_type"`
	SizeBytes   int64  `json:"size_bytes"`
	Kind        string `json:"kind"`
	Source      string `json:"source"`
	CreatedAt   string `json:"created_at"`
}

func toUserMaterialItem(m *service.UserMaterial) userMaterialItem {
	if m == nil {
		return userMaterialItem{}
	}
	return userMaterialItem{
		ID:          m.ID,
		FileName:    m.FileName,
		URL:         m.CosURL,
		ContentType: m.ContentType,
		SizeBytes:   m.SizeBytes,
		Kind:        m.Kind,
		Source:      m.Source,
		CreatedAt:   m.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
	}
}

// Upload POST /user/materials/upload （multipart/form-data，字段名 "file"）。
// 单文件上传；服务端会把字节转存到 COS 的用户目录下并落库。
func (h *UserMaterialHandler) Upload(c *gin.Context) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		response.Error(c, http.StatusUnauthorized, "unauthorized")
		return
	}
	// 32MB 一次读满即可，MaxMultipartMemory 由 gin 默认 32MB 兜底。
	// 保守起见我们主动 set 一次，与素材库总上限对齐。
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, service.MaterialVideoMaxBytes+(1<<20))
	fileHeader, err := c.FormFile("file")
	if err != nil {
		response.Error(c, http.StatusBadRequest, "missing file field: "+err.Error())
		return
	}
	f, err := fileHeader.Open()
	if err != nil {
		response.Error(c, http.StatusBadRequest, "open uploaded file: "+err.Error())
		return
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(f)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "read uploaded file: "+err.Error())
		return
	}
	m, err := h.svc.UploadBytes(c.Request.Context(), subject.UserID,
		fileHeader.Filename, fileHeader.Header.Get("Content-Type"), data)
	if err != nil {
		respondUserMaterialErr(c, err)
		return
	}
	response.Success(c, toUserMaterialItem(m))
}

// ImportFromURL POST /user/materials/import-url  body: {"url": "https://..."}
// 服务端下载 URL、转存到 COS，最终保存的是 COS 的 URL。
func (h *UserMaterialHandler) ImportFromURL(c *gin.Context) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		response.Error(c, http.StatusUnauthorized, "unauthorized")
		return
	}
	var req struct {
		URL string `json:"url"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "invalid body: "+err.Error())
		return
	}
	m, err := h.svc.ImportFromURL(c.Request.Context(), subject.UserID, req.URL)
	if err != nil {
		respondUserMaterialErr(c, err)
		return
	}
	response.Success(c, toUserMaterialItem(m))
}

// List GET /user/materials?kind=&keyword=&page=&page_size=
func (h *UserMaterialHandler) List(c *gin.Context) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		response.Error(c, http.StatusUnauthorized, "unauthorized")
		return
	}
	kind := strings.TrimSpace(c.Query("kind"))
	keyword := strings.TrimSpace(c.Query("keyword"))
	page := parseIntQuery(c, "page", 1, 1, 1_000_000)
	pageSize := parseIntQuery(c, "page_size", 20, 1, 100)

	items, total, err := h.svc.List(c.Request.Context(), subject.UserID, kind, keyword, page, pageSize)
	if err != nil {
		respondUserMaterialErr(c, err)
		return
	}
	dto := make([]userMaterialItem, 0, len(items))
	for _, m := range items {
		dto = append(dto, toUserMaterialItem(m))
	}
	response.Success(c, gin.H{
		"items":     dto,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

// Rename PATCH /user/materials/:id body: {"file_name":"new name.png"}.
// Only the display name changes; the COS object key and URL stay stable.
func (h *UserMaterialHandler) Rename(c *gin.Context) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		response.Error(c, http.StatusUnauthorized, "unauthorized")
		return
	}
	id, err := strconv.ParseInt(strings.TrimSpace(c.Param("id")), 10, 64)
	if err != nil || id <= 0 {
		response.Error(c, http.StatusBadRequest, "invalid id")
		return
	}
	var req struct {
		FileName string `json:"file_name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "invalid body: "+err.Error())
		return
	}
	material, err := h.svc.Rename(c.Request.Context(), subject.UserID, id, req.FileName)
	if err != nil {
		respondUserMaterialErr(c, err)
		return
	}
	response.Success(c, toUserMaterialItem(material))
}

// Delete DELETE /user/materials/:id  软删自己的素材。
func (h *UserMaterialHandler) Delete(c *gin.Context) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		response.Error(c, http.StatusUnauthorized, "unauthorized")
		return
	}
	idStr := strings.TrimSpace(c.Param("id"))
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		response.Error(c, http.StatusBadRequest, "invalid id")
		return
	}
	if err := h.svc.Delete(c.Request.Context(), subject.UserID, id); err != nil {
		respondUserMaterialErr(c, err)
		return
	}
	response.Success(c, gin.H{"deleted": id})
}

// respondUserMaterialErr 把 service 层的 *ApplicationError 直接透传 code/message。
// 其它错误统一 500。
func respondUserMaterialErr(c *gin.Context, err error) {
	var appErr *infraerrors.ApplicationError
	if errors.As(err, &appErr) {
		response.ErrorFrom(c, err)
		return
	}
	// 特判 not found（SoftDelete 返回 sql.ErrNoRows）。
	if strings.Contains(err.Error(), "no rows") {
		response.Error(c, http.StatusNotFound, "material not found")
		return
	}
	response.Error(c, http.StatusInternalServerError, err.Error())
}
