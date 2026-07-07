package admin

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type BackupHandler struct {
	backupService *service.BackupService
	userService   *service.UserService
}

func NewBackupHandler(backupService *service.BackupService, userService *service.UserService) *BackupHandler {
	return &BackupHandler{
		backupService: backupService,
		userService:   userService,
	}
}

// ─── S3 配置 ───

func (h *BackupHandler) GetS3Config(c *gin.Context) {
	cfg, err := h.backupService.GetS3Config(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, cfg)
}

func (h *BackupHandler) UpdateS3Config(c *gin.Context) {
	var req service.BackupS3Config
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	cfg, err := h.backupService.UpdateS3Config(c.Request.Context(), req)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, cfg)
}

func (h *BackupHandler) TestS3Connection(c *gin.Context) {
	var req service.BackupS3Config
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	err := h.backupService.TestS3Connection(c.Request.Context(), req)
	if err != nil {
		response.Success(c, gin.H{"ok": false, "message": err.Error()})
		return
	}
	response.Success(c, gin.H{"ok": true, "message": "connection successful"})
}

// ─── 定时备份 ───

func (h *BackupHandler) GetSchedule(c *gin.Context) {
	cfg, err := h.backupService.GetSchedule(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, cfg)
}

func (h *BackupHandler) UpdateSchedule(c *gin.Context) {
	var req service.BackupScheduleConfig
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	cfg, err := h.backupService.UpdateSchedule(c.Request.Context(), req)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, cfg)
}

// ─── 备份操作 ───

type CreateBackupRequest struct {
	ExpireDays *int `json:"expire_days"` // nil=使用默认值14，0=永不过期
}

func (h *BackupHandler) CreateBackup(c *gin.Context) {
	var req CreateBackupRequest
	_ = c.ShouldBindJSON(&req) // 允许空 body

	expireDays := 14 // 默认14天过期
	if req.ExpireDays != nil {
		expireDays = *req.ExpireDays
	}

	record, err := h.backupService.StartBackup(c.Request.Context(), "manual", expireDays)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Accepted(c, record)
}

func (h *BackupHandler) ImportBackupsFromS3(c *gin.Context) {
	result, err := h.backupService.ImportBackupRecordsFromS3(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func (h *BackupHandler) ListBackups(c *gin.Context) {
	records, err := h.backupService.ListBackups(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if records == nil {
		records = []service.BackupRecord{}
	}
	response.Success(c, gin.H{"items": records})
}

func (h *BackupHandler) GetBackup(c *gin.Context) {
	backupID := c.Param("id")
	if backupID == "" {
		response.BadRequest(c, "backup ID is required")
		return
	}
	record, err := h.backupService.GetBackupRecord(c.Request.Context(), backupID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, record)
}

func (h *BackupHandler) DeleteBackup(c *gin.Context) {
	backupID := c.Param("id")
	if backupID == "" {
		response.BadRequest(c, "backup ID is required")
		return
	}
	if err := h.backupService.DeleteBackup(c.Request.Context(), backupID); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"deleted": true})
}

func (h *BackupHandler) GetDownloadURL(c *gin.Context) {
	backupID := c.Param("id")
	if backupID == "" {
		response.BadRequest(c, "backup ID is required")
		return
	}
	url, err := h.backupService.GetBackupDownloadURL(c.Request.Context(), backupID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"url": url})
}

// ─── 恢复操作（需要重新输入管理员密码） ───

type RestoreBackupRequest struct {
	Password string `json:"password" binding:"required"`
}

func (h *BackupHandler) RestoreBackup(c *gin.Context) {
	backupID := c.Param("id")
	if backupID == "" {
		response.BadRequest(c, "backup ID is required")
		return
	}

	var req RestoreBackupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "password is required for restore operation")
		return
	}

	// 从上下文获取当前管理员用户 ID
	sub, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "unauthorized")
		return
	}

	// 获取管理员用户并验证密码
	user, err := h.userService.GetByID(c.Request.Context(), sub.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if !user.CheckPassword(req.Password) {
		response.BadRequest(c, "incorrect admin password")
		return
	}

	record, err := h.backupService.StartRestore(c.Request.Context(), backupID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Accepted(c, record)
}
