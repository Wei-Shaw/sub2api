package admin

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/kiro"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// kiroDevicePollDefaultInterval is used when reporting a pending device-code
// poll. KiroOAuthService.PollDeviceAuth discards the session on
// ErrAuthorizationPending/ErrSlowDown (it only returns the error, not the
// *kiro.OAuthSession), so the handler has no way to read back the exact
// interval AWS originally handed out in StartDeviceAuthorization. 5 seconds
// matches AWS's own device-authorization default (see
// kiro.StartDeviceAuthorization) and the value already returned to the
// frontend by DeviceStart, which the frontend should already be honoring for
// its own polling cadence — this is only a hint for callers that ignore that.
const kiroDevicePollDefaultInterval = 5

// KiroOAuthHandler 承载 Kiro（Amazon Q Developer / CodeWhisperer）账号的
// 授权流程：IdC 授权码（管理员手动粘贴回调 URL，见 CompleteIdC）与
// Builder ID 设备码（轮询）两条路径。
type KiroOAuthHandler struct {
	svc *service.KiroOAuthService
}

// NewKiroOAuthHandler 创建 handler。
func NewKiroOAuthHandler(svc *service.KiroOAuthService) *KiroOAuthHandler {
	return &KiroOAuthHandler{svc: svc}
}

type kiroAuthorizeURLRequest struct {
	ProxyID   *int64 `json:"proxy_id"`
	IssuerURL string `json:"issuer_url"`
	Region    string `json:"region"`
}

// AuthorizeURL 发起 IdC 授权码流程：动态注册客户端 + PKCE，返回跳转地址。
// redirect_uri 不再由调用方提供——固定为 kiroIdCRedirectURI（见其文档：
// AWS SSO-OIDC 的 public client 注册强制要求 loopback 回调地址，真实账号
// 联调验证过，服务端自建回调页这条路走不通）。必填参数校验交给
// svc.GenerateAuthURL（唯一的校验来源，避免 handler/service 两处各写一份、
// 报错文案还不一致）。
func (h *KiroOAuthHandler) AuthorizeURL(c *gin.Context) {
	var req kiroAuthorizeURLRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		req = kiroAuthorizeURLRequest{}
	}
	result, err := h.svc.GenerateAuthURL(c.Request.Context(), &service.KiroAuthURLInput{
		ProxyID:   req.ProxyID,
		IssuerURL: req.IssuerURL,
		Region:    req.Region,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

type kiroIdCCompleteRequest struct {
	SessionID   string `json:"session_id" binding:"required"`
	CallbackURL string `json:"callback_url" binding:"required"`
	ProxyID     *int64 `json:"proxy_id"`
}

// CompleteIdC 用管理员手动粘贴回来的（打不开的）回调地址完成 IdC 授权码
// 兑换。见 service.KiroOAuthService.CompleteIdCLogin 的文档：redirect_uri
// 固定指向一个没有任何服务监听的 loopback 地址，AWS 授权完成后浏览器会
// 跳过去显示"无法连接"，但地址栏里的完整 URL（带 code+state）还在——这是
// 前端唯一能把授权结果带回来的路径。
func (h *KiroOAuthHandler) CompleteIdC(c *gin.Context) {
	var req kiroIdCCompleteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	ts, sess, err := h.svc.CompleteIdCLogin(c.Request.Context(), req.SessionID, req.CallbackURL, req.ProxyID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	creds := h.svc.BuildAccountCredentials(service.KiroCredentialInput{
		TokenSet:     ts,
		Method:       sess.Method,
		Region:       sess.Region,
		IssuerURL:    sess.IssuerURL,
		ClientID:     sess.ClientID,
		ClientSecret: sess.ClientSecret,
	})
	discoverKiroProfileArnIfMissing(c.Request.Context(), h.svc, creds, req.ProxyID)
	response.Success(c, gin.H{"status": "ok", "credentials": creds})
}

type kiroDeviceStartRequest struct {
	ProxyID *int64 `json:"proxy_id"`
	Region  string `json:"region"`
}

// DeviceStart 发起 Builder ID 设备码授权。
func (h *KiroOAuthHandler) DeviceStart(c *gin.Context) {
	var req kiroDeviceStartRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		req = kiroDeviceStartRequest{}
	}
	result, err := h.svc.StartDeviceAuth(c.Request.Context(), req.ProxyID, req.Region)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

type kiroDevicePollRequest struct {
	SessionID string `json:"session_id" binding:"required"`
	ProxyID   *int64 `json:"proxy_id"`
}

// DevicePoll 轮询设备码。pending 不是错误——返回 200 + {"status":"pending",
// "interval":N}，让前端按 interval 继续轮询；只有终态失败才返回 4xx/5xx。
func (h *KiroOAuthHandler) DevicePoll(c *gin.Context) {
	var req kiroDevicePollRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	ts, sess, err := h.svc.PollDeviceAuth(c.Request.Context(), req.SessionID, req.ProxyID)
	if err != nil {
		if errors.Is(err, kiro.ErrAuthorizationPending) || errors.Is(err, kiro.ErrSlowDown) {
			response.Success(c, gin.H{"status": "pending", "interval": kiroDevicePollDefaultInterval})
			return
		}
		response.ErrorFrom(c, err)
		return
	}

	creds := h.svc.BuildAccountCredentials(service.KiroCredentialInput{
		TokenSet:     ts,
		Method:       sess.Method,
		Region:       sess.Region,
		IssuerURL:    sess.IssuerURL,
		ClientID:     sess.ClientID,
		ClientSecret: sess.ClientSecret,
	})
	discoverKiroProfileArnIfMissing(c.Request.Context(), h.svc, creds, req.ProxyID)
	response.Success(c, gin.H{"status": "ok", "credentials": creds})
}

// discoverKiroProfileArnIfMissing 在新建 IdC/Builder ID 账号时顺手尝试一次
// profileArn 自动发现（可选，见 KiroOAuthService.DiscoverProfileArn 的
// 文档）——两处授权完成的回调（CompleteIdC/DevicePoll）在把 creds 返回给
// 前端之前调用，创建的账号从一开始就带上 profileArn，不用等到第一次
// token 刷新才补上。发现失败/没有可用 profile 都不阻断账号创建，
// 管理端手填入口（KiroCredentialFields.vue 的 Profile ARN 字段）仍然
// 保留作为兜底。
func discoverKiroProfileArnIfMissing(ctx context.Context, svc *service.KiroOAuthService, creds map[string]any, proxyID *int64) {
	if creds == nil {
		return
	}
	authMethod, _ := creds["auth_method"].(string)
	if authMethod != string(kiro.AuthIdC) && authMethod != string(kiro.AuthBuilderID) {
		return
	}
	if existing, _ := creds["profile_arn"].(string); strings.TrimSpace(existing) != "" {
		return
	}
	accessToken, _ := creds["access_token"].(string)
	machineID, _ := creds["machine_id"].(string)
	discovered, err := svc.DiscoverProfileArn(ctx, accessToken, machineID, proxyID)
	if err != nil {
		slog.Debug("kiro_profile_arn_discovery_failed", "auth_method", authMethod, "error", err)
		return
	}
	if discovered != "" {
		creds["profile_arn"] = discovered
	}
}
