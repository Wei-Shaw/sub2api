package admin

// file_handler.go
//
// 管理员「文件管理」接口：直接浏览/上传/下载/改名/删除图片转存桶里的对象。
//
// 全部依赖图片转存（COS / S3 兼容）已启用；未启用时统一返回 COS_NOT_CONFIGURED，
// 前端据此渲染"请先在系统设置启用图片转存"的引导页，而不是一个空列表。

import (
	"errors"
	"io"
	"net/http"
	"path"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// FileHandler 暴露 admin 端的对象存储文件管理接口。
type FileHandler struct {
	svc *service.AdminFileService
}

func NewFileHandler(svc *service.AdminFileService) *FileHandler {
	return &FileHandler{svc: svc}
}

// GetStatus GET /admin/files/status
//
// 返回文件管理是否可用，以及正在管理的桶与前缀。
// 管理端用它决定"显示文件列表"还是"显示未启用引导"，也用于侧边栏入口的显隐。
func (h *FileHandler) GetStatus(c *gin.Context) {
	ctx := c.Request.Context()
	if h.svc == nil || !h.svc.Enabled(ctx) {
		response.Success(c, gin.H{"enabled": false, "bucket": "", "prefix": ""})
		return
	}
	bucket, prefix, err := h.svc.StorageInfo(ctx)
	if err != nil {
		// 能走到这里说明 Enabled 为真却读不到配置（极少见的竞态），
		// 按"未启用"返回即可，让前端走引导页而不是报错页。
		response.Success(c, gin.H{"enabled": false, "bucket": "", "prefix": ""})
		return
	}
	response.Success(c, gin.H{"enabled": true, "bucket": bucket, "prefix": prefix})
}

// List GET /admin/files?prefix=&token=&limit=&flat=
//
// flat=1 时递归平铺该前缀下所有对象（用于跨层级查看）；默认按 "/" 聚合目录，
// 前端逐层浏览。分页用 token（对象存储只支持游标式分页，没有跳页）。
func (h *FileHandler) List(c *gin.Context) {
	prefix := c.Query("prefix")
	token := c.Query("token")
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))

	// 默认按目录聚合；flat=1/true 时递归平铺。
	delimiter := "/"
	if flat := strings.ToLower(c.Query("flat")); flat == "1" || flat == "true" {
		delimiter = ""
	}

	res, err := h.svc.List(c.Request.Context(), prefix, delimiter, token, int32(limit)) //nolint:gosec // limit 已在 service 层夹到 [1,1000]
	if err != nil {
		respondFileErr(c, err)
		return
	}
	response.Success(c, res)
}

// DownloadURL GET /admin/files/download-url?key=
//
// 返回预签名直链而不是代理文件流：管理端下载的常是几十上百 MB 的视频，
// 经后端转发会白占一份带宽和内存。
func (h *FileHandler) DownloadURL(c *gin.Context) {
	key := c.Query("key")
	url, err := h.svc.DownloadURL(c.Request.Context(), key)
	if err != nil {
		respondFileErr(c, err)
		return
	}
	response.Success(c, gin.H{"url": url, "key": key, "expires_in": 600})
}

// Upload POST /admin/files/upload  (multipart/form-data)
//
// 表单字段：
//   - file：文件本体（必填）
//   - prefix：目标目录前缀（可选，如 "images/2026/"）
//   - name：覆盖文件名（可选，默认用上传文件的原名）
//
// 最终 key = prefix + name。允许覆盖同名对象（管理员显式操作，前端会先确认）。
func (h *FileHandler) Upload(c *gin.Context) {
	// 主动设限，避免超大 body 把内存打满；+1MiB 容纳 multipart 边界等开销。
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, service.AdminFileUploadMaxBytes()+(1<<20))

	fileHeader, err := c.FormFile("file")
	if err != nil {
		response.BadRequest(c, "missing file field: "+err.Error())
		return
	}
	f, err := fileHeader.Open()
	if err != nil {
		response.BadRequest(c, "open uploaded file: "+err.Error())
		return
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(f)
	if err != nil {
		response.BadRequest(c, "read uploaded file: "+err.Error())
		return
	}

	// 文件名：优先表单里显式给的 name，其次上传件原名。
	// 只取 base：浏览器在某些情况下会带上相对路径（如目录上传），
	// 直接拼进 key 会造出意料之外的层级。
	name := strings.TrimSpace(c.PostForm("name"))
	if name == "" {
		name = fileHeader.Filename
	}
	name = path.Base(strings.ReplaceAll(strings.TrimSpace(name), `\`, "/"))
	if name == "" || name == "." || name == "/" {
		response.BadRequest(c, "invalid file name")
		return
	}

	prefix := strings.TrimLeft(strings.TrimSpace(c.PostForm("prefix")), "/")
	if prefix != "" && !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	entry, err := h.svc.Upload(c.Request.Context(), prefix+name, data, fileHeader.Header.Get("Content-Type"))
	if err != nil {
		respondFileErr(c, err)
		return
	}
	response.Success(c, entry)
}

// renameFileRequest 是 Rename 的请求体。
//
// 支持两种用法：
//   - 只给 name：在原目录内改名（最常见）
//   - 给 key：直接指定完整目标键，可跨目录移动
type renameFileRequest struct {
	Key  string `json:"key" binding:"required"`
	Name string `json:"name"`
	// NewKey 完整目标键；与 Name 二选一，同时给出时以 NewKey 为准。
	NewKey string `json:"new_key"`
}

// Rename PUT /admin/files/rename
//
// 对象存储没有 rename，服务层实现为服务端 copy + 删源。目标已存在时拒绝覆盖。
func (h *FileHandler) Rename(c *gin.Context) {
	var req renameFileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	dst := strings.TrimSpace(req.NewKey)
	if dst == "" {
		// 只改名：保持原目录不变，替换最后一段。
		name := path.Base(strings.ReplaceAll(strings.TrimSpace(req.Name), `\`, "/"))
		if name == "" || name == "." || name == "/" {
			response.BadRequest(c, "either new_key or a valid name is required")
			return
		}
		dir := path.Dir(strings.TrimLeft(strings.TrimSpace(req.Key), "/"))
		if dir == "." || dir == "/" {
			dst = name
		} else {
			dst = dir + "/" + name
		}
	}

	entry, err := h.svc.Rename(c.Request.Context(), req.Key, dst)
	if err != nil {
		respondFileErr(c, err)
		return
	}
	response.Success(c, entry)
}

// deleteFilesRequest 是 Delete 的请求体（支持单个与批量）。
type deleteFilesRequest struct {
	Keys []string `json:"keys" binding:"required,min=1,max=100"`
}

// Delete DELETE /admin/files
//
// 批量删除。部分失败不影响其余，逐条错误按 key 汇总返回，前端据此提示。
func (h *FileHandler) Delete(c *gin.Context) {
	var req deleteFilesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	// 未启用时直接短路，避免逐条都去 loadConfig 报同一个错。
	if h.svc == nil || !h.svc.Enabled(c.Request.Context()) {
		respondFileErr(c, service.ErrCOSNotConfigured)
		return
	}

	deleted, failures := h.svc.DeleteMany(c.Request.Context(), req.Keys)
	response.Success(c, gin.H{
		"deleted":  deleted,
		"failed":   len(failures),
		"failures": failures,
	})
}

// respondFileErr 把服务层错误映射成合适的 HTTP 状态码与错误码。
//
// 分得细一点是为了让前端能给出可操作的提示：未配置 → 引导去系统设置；
// key 非法 / 目标已存在 → 用户输入问题；store 不支持 → 环境能力问题。
// 走 ErrorWithDetails 带上 reason，前端 extractApiErrorCode 读的就是它。
func respondFileErr(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrCOSNotConfigured):
		response.ErrorWithDetails(c, http.StatusBadRequest,
			"image transfer (object storage) is not enabled or not configured",
			"COS_NOT_CONFIGURED", nil)
	case errors.Is(err, service.ErrInvalidObjectKey):
		response.ErrorWithDetails(c, http.StatusBadRequest, err.Error(), "INVALID_OBJECT_KEY", nil)
	case errors.Is(err, service.ErrObjectKeyExists):
		response.ErrorWithDetails(c, http.StatusConflict,
			"an object with the target key already exists", "OBJECT_KEY_EXISTS", nil)
	case errors.Is(err, service.ErrObjectStoreNoListSupport):
		response.ErrorWithDetails(c, http.StatusNotImplemented, err.Error(), "LIST_NOT_SUPPORTED", nil)
	case errors.Is(err, service.ErrObjectStoreNoCopySupport):
		response.ErrorWithDetails(c, http.StatusNotImplemented, err.Error(), "COPY_NOT_SUPPORTED", nil)
	default:
		response.ErrorFrom(c, err)
	}
}
