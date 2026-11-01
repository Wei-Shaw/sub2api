package admin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/adminaccess"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type scopedCodexSessionImportRequest struct {
	CodexSessionImportRequest
	ProxyMode   string `json:"proxy_mode"`
	RandomProxy bool   `json:"random_proxy"`
}

func validateScopedCodexRequest(req scopedCodexSessionImportRequest) error {
	if _, err := service.NormalizeAccountProxyMode(req.ProxyMode, req.ProxyID, req.RandomProxy); err != nil {
		return err
	}
	if req.GroupIDs == nil || (req.ConfirmMixedChannelRisk != nil && *req.ConfirmMixedChannelRisk) {
		return infraerrors.BadRequest("SCOPED_CODEX_INPUT", "须明确提供 group_ids，不能跳过分组风险检查")
	}
	if (req.Concurrency != nil && *req.Concurrency < 0) || (req.Priority != nil && *req.Priority < 0) || (req.LoadFactor != nil && (*req.LoadFactor < 1 || *req.LoadFactor > 10000)) || (req.RateMultiplier != nil && *req.RateMultiplier < 0) {
		return infraerrors.BadRequest("SCOPED_CODEX_INPUT", "导入参数无效")
	}
	for key := range req.CredentialExtras {
		if key != "base_url" && key != "model_mapping" {
			return infraerrors.BadRequest("SCOPED_CODEX_FIELD", "credential_extras 仅支持独立授权的 base_url 和 model_mapping")
		}
	}
	for key, value := range req.Extra {
		valid := false
		switch key {
		case "codex_fingerprint_mode", "openai_oauth_responses_websockets_v2_mode":
			_, valid = value.(string)
		case "openai_oauth_responses_websockets_v2_enabled":
			_, valid = value.(bool)
		}
		if !valid {
			return infraerrors.BadRequest("SCOPED_CODEX_FIELD", "extra 包含不支持的字段或值类型")
		}
	}
	return nil
}

func (h *AccountHandler) scopedAdminImportCodexSession(c *gin.Context) {
	var req scopedCodexSessionImportRequest
	if c.ShouldBindJSON(&req) != nil {
		response.BadRequest(c, "导入请求格式无效")
		return
	}
	if err := validateScopedCodexRequest(req); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	entries, err := parseCodexSessionImportEntries(req.CodexSessionImportRequest)
	if err != nil || len(entries) == 0 {
		response.BadRequest(c, "请输入有效的 accessToken 或 Codex session JSON")
		return
	}
	api, ok := h.adminService.(service.ScopedAdminCodexAccounts)
	if !ok {
		response.Error(c, 503, "受限导入服务不可用")
		return
	}
	principal := adminaccess.FromContext(c.Request.Context())
	if principal == nil || !adminaccess.RouteAllowed(principal.Policy, c.Request.Method, c.FullPath()) {
		response.Error(c, 403, "导入未授权")
		return
	}
	// 隔离完整管理员、不同 Key 和权限修改前后的幂等结果。
	policy, _ := json.Marshal(principal.Policy)
	digest := sha256.Sum256(policy)
	scope := "scoped.codex:" + principal.KeyID + ":" + hex.EncodeToString(digest[:])
	executeAdminIdempotentJSON(c, scope, req, service.DefaultWriteIdempotencyTTL(), func(ctx context.Context) (any, error) {
		result, err := h.importScopedCodexSessions(ctx, api, req, entries)
		if err != nil {
			code, message := scopedAdminAccountErrorDetails(err)
			return nil, infraerrors.New(code, "SCOPED_CODEX_FAILED", message)
		}
		return result, nil
	})
}

func (h *AccountHandler) importScopedCodexSessions(ctx context.Context, api service.ScopedAdminCodexAccounts, req scopedCodexSessionImportRequest, entries []codexImportEntry) (CodexSessionImportResult, error) {
	proxyMode, err := service.NormalizeAccountProxyMode(req.ProxyMode, req.ProxyID, req.RandomProxy)
	if err != nil {
		return CodexSessionImportResult{}, err
	}
	result := CodexSessionImportResult{Total: len(entries), Items: make([]CodexSessionImportItem, 0, len(entries))}
	updateExisting := req.UpdateExisting == nil || *req.UpdateExisting
	seen := map[string]codexSeenIdentity{}
	fail := func(entry int, err error) {
		_, message := scopedAdminAccountErrorDetails(err)
		result.Failed++
		result.Items = append(result.Items, CodexSessionImportItem{Index: entry, Action: "failed", Message: message})
		result.Errors = append(result.Errors, CodexSessionImportMessage{Index: entry, Message: message})
	}
	for _, entry := range entries {
		item, err := normalizeCodexImportEntry(entry)
		if err != nil {
			fail(entry.Index, infraerrors.BadRequest("SCOPED_CODEX_PARSE", "认证内容无效"))
			continue
		}
		expires, credentialExpiry, autoPause, warnings, err := resolveCodexImportExpiry(req.CodexSessionImportRequest, item)
		if err != nil {
			fail(entry.Index, infraerrors.BadRequest("SCOPED_CODEX_EXPIRY", "认证有效期无效"))
			continue
		}
		if credentialExpiry != nil {
			item.Credentials["expires_at"] = credentialExpiry.Format(time.RFC3339)
		}
		if _, duplicate := firstSeenCodexIdentity(seen, item.IdentityKeys, item.UserID); duplicate {
			result.Skipped++
			result.Items = append(result.Items, CodexSessionImportItem{Index: entry.Index, Action: "skipped", Message: "同批次重复导入项"})
			continue
		}
		markCodexIdentitySeen(seen, item.IdentityKeys, entry.Index, item.UserID)
		credentials := mergeCodexImportMap(item.Credentials, req.CredentialExtras)
		extra := mergeCodexImportMap(req.Extra, item.Extra)
		name := buildCodexCreateAccountName(req.Name, item, entry.Index, len(entries))
		var account *service.Account
		action := "created"
		var existing *service.Account
		if updateExisting {
			candidates, lookupErr := api.ScopedAdminCodexCandidates(ctx, service.ScopedCodexIdentity{
				AccountID: item.AccountID, UserID: item.UserID, Email: item.Email, AccessToken: item.AccessToken,
				AccessTokenOnly: item.RefreshToken == "" && item.AccessToken != "", AgentIdentity: item.IsAgentIdentity,
			})
			if lookupErr != nil {
				return result, lookupErr
			}
			existing, _ = buildCodexAccountIndex(candidates).Find(item.IdentityKeys, item.UserID)
		}
		if updateExisting && existing != nil {
			action = "updated"
			input := &service.UpdateAccountInput{ProxyMode: proxyMode, Credentials: credentials, Extra: extra, ProxyID: req.ProxyID, Concurrency: req.Concurrency, Priority: req.Priority, RateMultiplier: req.RateMultiplier, LoadFactor: req.LoadFactor, ExpiresAt: expires, AutoPauseOnExpired: autoPause}
			if len(req.GroupIDs) > 0 {
				input.GroupIDs = &req.GroupIDs
			}
			account, err = api.ScopedAdminUpdateCodex(ctx, existing.ID, input, func(current *service.Account, safe *service.UpdateAccountInput) error {
				// 锁内重新匹配，不能将候选快照匹配结果应用到已被其他操作换绑的身份。
				fresh := buildCodexAccountIndex([]service.Account{*current})
				if matched, _ := fresh.Find(item.IdentityKeys, item.UserID); matched == nil {
					return infraerrors.Conflict("SCOPED_CODEX_CHANGED", "账户身份已变更")
				}
				safe.Credentials = mergeCodexImportCredentials(current.Credentials, credentials, item)
				safe.Extra = mergeCodexImportMap(current.Extra, extra)
				if item.RefreshToken == "" && codexCredentialString(current.Credentials, "refresh_token") != "" {
					safe.ExpiresAt = nil
					safe.AutoPauseOnExpired = nil
				}
				return nil
			})
		} else {
			concurrency, priority := 3, 50
			if req.Concurrency != nil {
				concurrency = *req.Concurrency
			}
			if req.Priority != nil {
				priority = *req.Priority
			}
			account, err = api.ScopedAdminCreateCodex(ctx, &service.CreateAccountInput{ProxyMode: proxyMode, Name: name, Notes: req.Notes, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth, Credentials: credentials, Extra: extra, GroupIDs: req.GroupIDs, ProxyID: req.ProxyID, Concurrency: concurrency, Priority: priority, RateMultiplier: req.RateMultiplier, LoadFactor: req.LoadFactor, ExpiresAt: expires, AutoPauseOnExpired: autoPause, SkipDefaultGroupBind: true})
		}
		if err != nil {
			fail(entry.Index, err)
			continue
		}
		if action == "updated" {
			result.Updated++
			h.invalidateScopedAccountToken(ctx, account)
		} else {
			result.Created++
		}
		// 不返回候选名称或任何已有凭证，仅返回本次操作结果。
		result.Items = append(result.Items, CodexSessionImportItem{Index: entry.Index, Action: action, AccountID: account.ID})
		for _, warning := range append(item.WarningTexts, warnings...) {
			result.Warnings = append(result.Warnings, CodexSessionImportMessage{Index: entry.Index, Message: warning})
		}
	}
	return result, nil
}
