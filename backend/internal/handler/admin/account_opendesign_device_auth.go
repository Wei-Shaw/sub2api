package admin

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// OpenDesign 一键设备授权（OAuth 取凭据）管理端接口。
//
// 路由（统一前缀 /api/v1/admin）：
//
//	POST   /opendesign/device-auth              创建设备授权会话
//	GET    /opendesign/device-auth/:sessionId   查询会话状态（后端节流轮询上游）
//	DELETE /opendesign/device-auth/:sessionId   取消会话
//	POST   /opendesign/accounts                 用已授权会话创建/更新账号
//
// 硬约束：落库账号 type 恒为 apikey（探测判定与转发都依赖它），
// UI 上的「OAuth」由 extra.opendesign_auth_source="oauth" 标识。
// controlKey / runtimeKey / deviceSecret 全程不返回前端。

const openDesignMaxAccountScanPageSize = 200

// openDesignDeviceAuthInitiateRequest 是创建设备授权会话的请求体。
type openDesignDeviceAuthInitiateRequest struct {
	Host string `json:"host"`
}

// CreateOpenDesignDeviceAuth 创建设备授权会话。
// POST /api/v1/admin/opendesign/device-auth
func (h *AccountHandler) CreateOpenDesignDeviceAuth(c *gin.Context) {
	if !h.requireOpenDesignDeviceAuth(c) {
		return
	}

	var req openDesignDeviceAuthInitiateRequest
	// 允许空 body：host 可省略（服务端取主机名兜底）。
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	result, err := h.opendesignDeviceAuth.InitiateDeviceAuth(c.Request.Context(), strings.TrimSpace(req.Host))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

// GetOpenDesignDeviceAuth 查询设备授权会话状态。
// GET /api/v1/admin/opendesign/device-auth/:sessionId
func (h *AccountHandler) GetOpenDesignDeviceAuth(c *gin.Context) {
	if !h.requireOpenDesignDeviceAuth(c) {
		return
	}
	sessionID := strings.TrimSpace(c.Param("sessionId"))
	if sessionID == "" {
		response.BadRequest(c, "Invalid request: sessionId is required")
		return
	}

	result, err := h.opendesignDeviceAuth.GetSessionStatus(c.Request.Context(), sessionID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

// CancelOpenDesignDeviceAuth 取消设备授权会话。
// DELETE /api/v1/admin/opendesign/device-auth/:sessionId
func (h *AccountHandler) CancelOpenDesignDeviceAuth(c *gin.Context) {
	if !h.requireOpenDesignDeviceAuth(c) {
		return
	}
	sessionID := strings.TrimSpace(c.Param("sessionId"))
	if sessionID == "" {
		response.BadRequest(c, "Invalid request: sessionId is required")
		return
	}
	h.opendesignDeviceAuth.CancelSession(sessionID)
	response.Success(c, gin.H{"ok": true})
}

// openDesignAccountFromSessionRequest 是用会话创建账号的请求体。
type openDesignAccountFromSessionRequest struct {
	SessionID string  `json:"session_id" binding:"required"`
	Name      string  `json:"name"`
	Notes     string  `json:"notes"`
	GroupIDs  []int64 `json:"group_ids"`
	Priority  int     `json:"priority"`
	// AccountID 为「重新授权已有账号」时的目标账号；不传则按授权身份匹配已有
	// open_design 账号，匹配不到才新建（§6）。
	AccountID   *int64 `json:"account_id"`
	Concurrency *int   `json:"concurrency"`
	// 以下为建号弹窗里可见的通用字段，必须一并落库，否则会出现
	// 「界面选了代理/倍率/到期，账号却没用上」的静默偏差（2026-11-15 复核中等 3）。
	ProxyID             *int64   `json:"proxy_id"`
	RateMultiplier      *float64 `json:"rate_multiplier"`
	LoadFactor          *int     `json:"load_factor"`
	ExpiresAt           *int64   `json:"expires_at"`
	AutoPauseOnExpired  *bool    `json:"auto_pause_on_expired"`
	ConfirmMixedChannel bool     `json:"confirm_mixed_channel"`
}

// CreateOpenDesignAccountFromSession 用已授权会话创建或更新 OpenDesign 账号。
// POST /api/v1/admin/opendesign/accounts
func (h *AccountHandler) CreateOpenDesignAccountFromSession(c *gin.Context) {
	if !h.requireOpenDesignDeviceAuth(c) {
		return
	}

	var req openDesignAccountFromSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	ctx := c.Request.Context()
	sessionID := strings.TrimSpace(req.SessionID)
	creds, err := h.opendesignDeviceAuth.BeginSessionConsumption(sessionID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	account, action, err := h.upsertOpenDesignAccountFromSession(ctx, &req, creds)
	if err != nil {
		// 落库失败：回滚会话到 authorized，允许直接重试（§3.2）。
		h.opendesignDeviceAuth.AbortSessionConsumption(sessionID)
		var mixedErr *service.MixedChannelError
		if errors.As(err, &mixedErr) {
			c.JSON(409, gin.H{
				"error":   "mixed_channel_warning",
				"message": mixedErr.Error(),
			})
			return
		}
		response.ErrorFrom(c, err)
		return
	}

	// 成功后才抹掉内存中的敏感值（一次性，防重放）。
	h.opendesignDeviceAuth.FinalizeSessionConsumption(sessionID)

	c.JSON(201, gin.H{
		"action": action,
		"data":   h.buildAccountResponseWithRuntime(ctx, account),
	})
}

// upsertOpenDesignAccountFromSession 决定「更新哪个账号 / 新建账号」并精确写入凭据。
// 返回 action：created / updated。
func (h *AccountHandler) upsertOpenDesignAccountFromSession(
	ctx context.Context,
	req *openDesignAccountFromSessionRequest,
	creds *service.OpenDesignSessionCredentials,
) (*service.Account, string, error) {
	// 更新路径只写这几个键——绝不整份覆盖 credentials（否则 model_mapping /
	// api_base_urls 会被清空，见 2026-11-15 账号 1323 事故）。
	authCredentials := map[string]any{
		"api_key":                             creds.RuntimeKey,
		service.OpenDesignUsageCredentialsKey: creds.ControlKey,
		service.OpenDesignUsageWorkspaceKey:   creds.WorkspaceID,
	}
	if creds.Email != "" {
		authCredentials["email"] = creds.Email
	}

	existingIDs, err := h.resolveOpenDesignAccountIDs(ctx, req.AccountID, creds)
	if err != nil {
		return nil, "", err
	}
	if len(existingIDs) > 1 {
		// 同一授权邮箱命中多个 open_design 账号：拒绝猜测，要求显式指定目标账号。
		// 否则可能把新凭据写到错误账号并影响其现有流量（2026-11-15 复核高危 2）。
		return nil, "", service.ErrOpenDesignDeviceAuthAccountAmbiguous.WithMetadata(map[string]string{
			"candidate_account_ids": joinInt64s(existingIDs),
		})
	}

	if len(existingIDs) == 1 {
		existingID := existingIDs[0]
		updateInput := &service.UpdateAccountInput{
			// 强制 apikey：OAuth 只是取凭据方式，账号类型不可漂移。
			Type:        service.AccountTypeAPIKey,
			Credentials: authCredentials,
			// 重试时前端会带 confirm_mixed_channel，更新路径同样必须透传，
			// 否则「同邮箱更新 + 绑定分组 + 混合渠道」会陷入重复 409（2026-11-15 复核中等 1）。
			SkipMixedChannelCheck: req.ConfirmMixedChannel,
			ProxyID:               req.ProxyID,
			RateMultiplier:        req.RateMultiplier,
			LoadFactor:            req.LoadFactor,
			ExpiresAt:             req.ExpiresAt,
			AutoPauseOnExpired:    req.AutoPauseOnExpired,
		}
		if len(req.GroupIDs) > 0 {
			groupIDs := req.GroupIDs
			updateInput.GroupIDs = &groupIDs
		}
		updated, uerr := h.adminService.UpdateAccount(ctx, existingID, updateInput)
		if uerr != nil {
			return nil, "", uerr
		}
		// Extra 走 key 级增量合并，避免把 web_search_emulation 等键整块覆盖。
		if xerr := h.adminService.UpdateAccountExtra(ctx, existingID, map[string]any{
			service.OpenDesignAuthSourceExtraKey: service.OpenDesignAuthSourceOAuth,
		}); xerr != nil {
			slog.Warn("opendesign_device_auth.update_extra_failed", "account_id", existingID, "err", xerr)
		}
		if cleared, cerr := h.adminService.ClearAccountError(ctx, existingID); cerr == nil && cleared != nil {
			updated = cleared
		}
		slog.Info("opendesign_device_auth.account_updated", "account_id", existingID)
		return updated, "updated", nil
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		if creds.Email != "" {
			name = "OpenDesign-go-" + creds.Email
		} else {
			name = "OpenDesign-go-Account"
		}
	}

	createCredentials := map[string]any{
		"api_key":  creds.RuntimeKey,
		"base_url": service.DefaultOpenDesignBaseURL,
		// adaptive + 三端点：OpenDesign 同时提供 CC / 原生 Responses（/v1）与
		// Anthropic 兼容端点（裸域），与生产既有账号形态一致。
		"api_protocol": service.APIProtocolAdaptive,
		"account_mode": service.AccountModePayG,
		"api_base_urls": map[string]any{
			service.APIProtocolChatCompletions: service.DefaultOpenDesignBaseURL,
			service.APIProtocolResponses:       service.DefaultOpenDesignBaseURL,
			service.APIProtocolAnthropic:       service.DefaultOpenDesignAnthropicBaseURL,
		},
		service.OpenDesignUsageCredentialsKey: creds.ControlKey,
		service.OpenDesignUsageWorkspaceKey:   creds.WorkspaceID,
	}
	if creds.Email != "" {
		createCredentials["email"] = creds.Email
	}

	var notes *string
	if trimmed := strings.TrimSpace(req.Notes); trimmed != "" {
		notes = &trimmed
	}
	// create 路径的 0 是「无代理」而非「proxy_id=0」：accounts.proxy_id 是外键，
	// 直接落 0 会建号失败（低 1）。更新路径由 admin_account 自身归一，这里只处理创建。
	proxyID := req.ProxyID
	if proxyID != nil && *proxyID == 0 {
		proxyID = nil
	}
	account, cerr := h.adminService.CreateAccount(ctx, &service.CreateAccountInput{
		Name:                  name,
		Notes:                 notes,
		Platform:              service.PlatformOpenDesign,
		Type:                  service.AccountTypeAPIKey,
		Credentials:           createCredentials,
		Extra:                 map[string]any{service.OpenDesignAuthSourceExtraKey: service.OpenDesignAuthSourceOAuth},
		GroupIDs:              req.GroupIDs,
		Priority:              req.Priority,
		Concurrency:           derefIntOrZero(req.Concurrency),
		ProxyID:               proxyID,
		RateMultiplier:        req.RateMultiplier,
		LoadFactor:            req.LoadFactor,
		ExpiresAt:             req.ExpiresAt,
		AutoPauseOnExpired:    req.AutoPauseOnExpired,
		SkipMixedChannelCheck: req.ConfirmMixedChannel,
	})
	if cerr != nil {
		return nil, "", cerr
	}
	slog.Info("opendesign_device_auth.account_created", "account_id", account.ID)
	return account, "created", nil
}

// resolveOpenDesignAccountIDs 找出本次授权应更新哪些已有账号：
//  1. 显式 account_id（重新授权按钮）——必须是 open_design 账号；
//  2. 授权邮箱与既有 open_design 账号匹配：优先 credentials.email 精确相等；
//     名称兜底仅在账号没有 credentials.email 时启用（§6）。
//
// 返回空 slice 表示需要新建；返回多个表示存在歧义，调用方必须拒绝并让用户显式指定，
// 绝不能用「第一条命中」当默认决策（2026-11-15 复核高危 2）。
func (h *AccountHandler) resolveOpenDesignAccountIDs(
	ctx context.Context,
	accountID *int64,
	creds *service.OpenDesignSessionCredentials,
) ([]int64, error) {
	if accountID != nil {
		account, err := h.adminService.GetAccount(ctx, *accountID)
		if err != nil {
			return nil, service.ErrOpenDesignDeviceAuthAccountInvalid.WithCause(err)
		}
		if account.Platform != service.PlatformOpenDesign {
			return nil, service.ErrOpenDesignDeviceAuthAccountInvalid
		}
		return []int64{account.ID}, nil
	}

	email := strings.ToLower(strings.TrimSpace(creds.Email))
	workspaceID := strings.TrimSpace(creds.WorkspaceID)
	if email == "" && workspaceID == "" {
		return nil, nil
	}

	// OpenDesign 账号量级很小（个位数），按平台整表扫描即可，避免依赖 search
	// 只匹配名称的语义。
	accounts, _, err := h.adminService.ListAccounts(ctx, 1, openDesignMaxAccountScanPageSize,
		service.PlatformOpenDesign, "", "", "", 0, "", "id", "asc")
	if err != nil {
		return nil, err
	}

	var byEmailExact []int64
	var byWorkspace []int64
	var byNameFallback []int64
	for i := range accounts {
		candidate := &accounts[i]
		if candidate.Platform != service.PlatformOpenDesign {
			continue
		}
		storedEmail := strings.ToLower(strings.TrimSpace(candidate.GetCredential("email")))
		storedWorkspace := strings.TrimSpace(candidate.GetCredential(service.OpenDesignUsageWorkspaceKey))
		if workspaceID != "" && storedWorkspace == workspaceID {
			byWorkspace = append(byWorkspace, candidate.ID)
		}
		if email == "" {
			continue
		}
		if storedEmail != "" {
			if storedEmail == email {
				byEmailExact = append(byEmailExact, candidate.ID)
			}
			continue // 已有权威邮箱时不再用名称猜测
		}
		if openDesignNameMatchesEmail(candidate.Name, email) {
			byNameFallback = append(byNameFallback, candidate.ID)
		}
	}

	// 优先级：workspaceId（更强身份）> 权威邮箱 > 名称兜底。
	for _, tier := range [][]int64{byWorkspace, byEmailExact, byNameFallback} {
		if len(tier) > 0 {
			return tier, nil
		}
	}
	return nil, nil
}

// openDesignNameMatchesEmail 判断账号名是否以该邮箱结尾且前面是分隔符，
// 用于匹配本功能之前建的「OpenDesign-go-<邮箱>」老账号。
// 必须带分隔符校验，否则 `a@b.com` 会误匹配到 `xa@b.com`。
func openDesignNameMatchesEmail(name, email string) bool {
	loweredName := strings.ToLower(strings.TrimSpace(name))
	if !strings.HasSuffix(loweredName, email) {
		return false
	}
	prefix := loweredName[:len(loweredName)-len(email)]
	if prefix == "" {
		return true
	}
	switch prefix[len(prefix)-1] {
	case '-', '_', '(', '[', ' ', '/', ':':
		return true
	default:
		return false
	}
}

// joinInt64s 把候选账号 ID 拼成审计/错误元数据里的字符串。
func joinInt64s(ids []int64) string {
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		parts = append(parts, strconv.FormatInt(id, 10))
	}
	return strings.Join(parts, ",")
}

func (h *AccountHandler) requireOpenDesignDeviceAuth(c *gin.Context) bool {
	if h != nil && h.opendesignDeviceAuth != nil {
		return true
	}
	response.ErrorFrom(c, service.ErrOpenDesignUsageUnavailable)
	return false
}

func derefIntOrZero(v *int) int {
	if v == nil {
		return 0
	}
	return *v
}
