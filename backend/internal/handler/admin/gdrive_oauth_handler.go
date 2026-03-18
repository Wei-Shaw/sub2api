package admin

import (
	"log/slog"
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// GDriveOAuthHandler 处理 Google Drive OAuth 授权流程。
type GDriveOAuthHandler struct {
	settingService *service.SettingService
	gdriveOAuth    *service.SoraGDriveOAuthService
	gdriveStorage  *service.SoraGDriveStorage
}

// NewGDriveOAuthHandler 创建 GDrive OAuth Handler。
func NewGDriveOAuthHandler(settingService *service.SettingService, gdriveOAuth *service.SoraGDriveOAuthService, gdriveStorage *service.SoraGDriveStorage) *GDriveOAuthHandler {
	return &GDriveOAuthHandler{
		settingService: settingService,
		gdriveOAuth:    gdriveOAuth,
		gdriveStorage:  gdriveStorage,
	}
}

// StartOAuthRequest 启动 OAuth 授权请求。
type StartOAuthRequest struct {
	ClientID     string `json:"client_id" binding:"required"`
	ClientSecret string `json:"client_secret" binding:"required"`
	RedirectURI  string `json:"redirect_uri" binding:"required"`
}

// StartOAuth 生成 Google OAuth 授权 URL。
// POST /api/v1/admin/settings/sora-storage/gdrive-oauth/start
func (h *GDriveOAuthHandler) StartOAuth(c *gin.Context) {
	if h.gdriveOAuth == nil {
		response.Error(c, http.StatusInternalServerError, "GDrive OAuth service not initialized")
		return
	}

	var req StartOAuthRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	authURL, state, err := h.gdriveOAuth.GenerateAuthURL(req.ClientID, req.ClientSecret, req.RedirectURI)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "生成授权 URL 失败: "+err.Error())
		return
	}

	response.Success(c, gin.H{
		"auth_url": authURL,
		"state":    state,
	})
}

// OAuthCallbackRequest OAuth 回调请求。
type OAuthCallbackRequest struct {
	ClientID     string `json:"client_id" binding:"required"`
	ClientSecret string `json:"client_secret" binding:"required"`
	RedirectURI  string `json:"redirect_uri" binding:"required"`
	Code         string `json:"code" binding:"required"`
	ProfileID    string `json:"profile_id"` // 要保存到的 profile ID（可选）
}

// OAuthCallback 用授权码换取 refresh_token 并保存到 profile。
// POST /api/v1/admin/settings/sora-storage/gdrive-oauth/callback
func (h *GDriveOAuthHandler) OAuthCallback(c *gin.Context) {
	if h.gdriveOAuth == nil {
		response.Error(c, http.StatusInternalServerError, "GDrive OAuth service not initialized")
		return
	}

	var req OAuthCallbackRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	refreshToken, err := h.gdriveOAuth.ExchangeCode(c.Request.Context(), req.ClientID, req.ClientSecret, req.RedirectURI, req.Code)
	if err != nil {
		slog.Error("[GDriveOAuth] exchange failed",
			"client_id_len", len(req.ClientID),
			"client_secret_len", len(req.ClientSecret),
			"redirect_uri", req.RedirectURI,
			"code_len", len(req.Code),
			"error", err,
		)
		response.Error(c, http.StatusBadRequest, "换取 refresh_token 失败: "+err.Error())
		return
	}

	// 如果指定了 profile_id，自动保存 refresh_token 到 profile
	if req.ProfileID != "" {
		profiles, err := h.settingService.ListSoraS3Profiles(c.Request.Context())
		if err == nil {
			for _, p := range profiles.Items {
				if p.ProfileID == req.ProfileID {
					_, _ = h.settingService.UpdateSoraS3Profile(c.Request.Context(), req.ProfileID, &service.SoraS3Profile{
						Name:                     p.Name,
						Provider:                 p.Provider,
						AccessMode:               p.AccessMode,
						Enabled:                  p.Enabled,
						Endpoint:                 p.Endpoint,
						Region:                   p.Region,
						Bucket:                   p.Bucket,
						AccessKeyID:              p.AccessKeyID,
						Prefix:                   p.Prefix,
						ForcePathStyle:           p.ForcePathStyle,
						CDNURL:                   p.CDNURL,
						DefaultStorageQuotaBytes: p.DefaultStorageQuotaBytes,
						AuthType:                 p.AuthType,
						ClientID:                 p.ClientID,
						FolderID:                 p.FolderID,
						RefreshToken:             refreshToken,
					})
					break
				}
			}
		}
	}

	response.Success(c, gin.H{
		"refresh_token": refreshToken,
		"message":       "OAuth 授权成功",
	})
}

// TestGDriveStorage 测试 GDrive 存储的完整上传→下载→删除流程。
// POST /api/v1/admin/settings/sora-storage/gdrive-test
func (h *GDriveOAuthHandler) TestGDriveStorage(c *gin.Context) {
	if h.gdriveStorage == nil {
		response.Error(c, http.StatusInternalServerError, "GDrive storage not initialized")
		return
	}

	result, err := h.gdriveStorage.TestFullCycle(c.Request.Context())
	if err != nil {
		response.Error(c, http.StatusBadRequest, "GDrive 测试失败: "+err.Error())
		return
	}

	response.Success(c, result)
}
