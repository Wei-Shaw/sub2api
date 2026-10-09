package admin

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// CodexSessionReauthRequest 是用 Codex auth.json / session JSON 重新授权单个账号的请求体。
type CodexSessionReauthRequest struct {
	Content string `json:"content" binding:"required"`
}

// CodexSessionReauthResult 返回更新后的账号与导入过程中的提示（如缺少 refresh_token）。
type CodexSessionReauthResult struct {
	Account  AccountWithConcurrency `json:"account"`
	Warnings []string               `json:"warnings,omitempty"`
}

// ReauthCodexSession 用 Codex auth.json / session JSON 重新授权指定的 OpenAI OAuth 账号。
// POST /api/v1/admin/accounts/:id/reauth/codex-session
//
// 与 /import/codex-session 的区别：
//   - 只作用于路径里的账号，不按身份在全量账号里匹配，也不会新建账号；
//   - 导入内容的 chatgpt_account_id / chatgpt_user_id 必须与该账号一致，否则拒绝，
//     避免把别人的凭据写进这个账号；
//   - 只替换凭据，不改并发、优先级、分组、代理等调度配置；
//   - 收尾与 /apply-oauth-credentials 一致：Extra 按键合并、清除错误状态、失效 token 缓存。
func (h *AccountHandler) ReauthCodexSession(c *gin.Context) {
	accountID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid account ID")
		return
	}

	var req CodexSessionReauthRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	ctx := c.Request.Context()
	existing, err := h.adminService.GetAccount(ctx, accountID)
	if err != nil {
		response.NotFound(c, "Account not found")
		return
	}
	if existing.Platform != service.PlatformOpenAI || existing.Type != service.AccountTypeOAuth {
		response.ErrorFrom(c, infraerrors.BadRequest("NOT_OPENAI_OAUTH", "only OpenAI OAuth accounts can be re-authorized with a Codex session"))
		return
	}
	if existing.IsOpenAIAgentIdentity() {
		response.ErrorFrom(c, infraerrors.BadRequest("AGENT_IDENTITY_UNSUPPORTED", "agent identity accounts cannot be re-authorized with a Codex session"))
		return
	}

	result, err := h.reauthCodexSession(ctx, existing, req.Content)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func (h *AccountHandler) reauthCodexSession(ctx context.Context, existing *service.Account, content string) (*CodexSessionReauthResult, error) {
	entries, err := parseCodexSessionImportEntries(CodexSessionImportRequest{Content: content})
	if err != nil {
		return nil, infraerrors.BadRequest("INVALID_CODEX_SESSION", err.Error())
	}
	if len(entries) != 1 {
		return nil, infraerrors.BadRequest("INVALID_CODEX_SESSION", "重新授权只接受一条 Codex 凭据，当前解析到 "+strconv.Itoa(len(entries))+" 条")
	}

	item, err := normalizeCodexImportEntry(entries[0])
	if err != nil {
		return nil, infraerrors.BadRequest("INVALID_CODEX_SESSION", err.Error())
	}
	if item.IsAgentIdentity {
		return nil, infraerrors.BadRequest("AGENT_IDENTITY_UNSUPPORTED", "重新授权不支持 agent identity 凭据")
	}
	if err := checkCodexReauthIdentity(existing, item); err != nil {
		return nil, infraerrors.BadRequest("CODEX_IDENTITY_MISMATCH", err.Error())
	}

	expiresAt, credentialExpiresAt, autoPauseOnExpired, expiryWarnings, err := resolveCodexImportExpiry(CodexSessionImportRequest{}, item)
	if err != nil {
		return nil, infraerrors.BadRequest("INVALID_CODEX_SESSION", err.Error())
	}
	warnings := append(append([]string(nil), item.WarningTexts...), expiryWarnings...)
	if credentialExpiresAt != nil {
		item.Credentials["expires_at"] = credentialExpiresAt.Format(time.RFC3339)
	}
	if item.RefreshToken == "" && codexCredentialString(existing.Credentials, "refresh_token") != "" {
		// 与批量导入一致：accessToken-only 的内容不覆盖已有 refresh_token，也不据此设置账号过期。
		warnings = append(warnings, "已有账号包含 refresh_token，本次 accessToken-only 重新授权已保留自动续期凭据")
		expiresAt = nil
		autoPauseOnExpired = nil
	}

	credentials := service.SanitizeStoredCredentials(existing.Platform, mergeCodexImportCredentials(existing.Credentials, item.Credentials, item))
	updated, err := h.adminService.UpdateAccount(ctx, existing.ID, &service.UpdateAccountInput{
		Type:               service.AccountTypeOAuth,
		Credentials:        credentials,
		ExpiresAt:          expiresAt,
		AutoPauseOnExpired: autoPauseOnExpired,
	})
	if err != nil {
		return nil, err
	}

	if len(item.Extra) > 0 {
		if extraErr := h.adminService.UpdateAccountExtra(ctx, existing.ID, item.Extra); extraErr != nil {
			slog.Error("reauth_codex_session.update_extra_failed", "account_id", existing.ID, "err", extraErr)
		}
	}
	if cleared, clearErr := h.adminService.ClearAccountError(ctx, existing.ID); clearErr != nil {
		slog.Warn("reauth_codex_session.clear_error_failed", "account_id", existing.ID, "err", clearErr)
	} else if cleared != nil {
		updated = cleared
	}
	if h.tokenCacheInvalidator != nil && updated != nil && updated.IsOAuth() {
		if invalidateErr := h.tokenCacheInvalidator.InvalidateToken(ctx, updated); invalidateErr != nil {
			slog.Warn("reauth_codex_session.invalidate_token_failed", "account_id", existing.ID, "err", invalidateErr)
		}
	}

	return &CodexSessionReauthResult{
		Account:  h.buildAccountResponseWithRuntime(ctx, updated),
		Warnings: warnings,
	}, nil
}

// checkCodexReauthIdentity 确认导入的凭据属于目标账号：
// chatgpt_account_id / chatgpt_user_id 双方都有值时必须相等；两者都无法比对时退回邮箱比对；
// 仍无法比对则拒绝，而不是盲目覆盖。
func checkCodexReauthIdentity(existing *service.Account, item *codexImportAccount) error {
	compared := false
	pairs := []struct {
		label    string
		stored   string
		incoming string
	}{
		{"chatgpt_account_id", codexCredentialString(existing.Credentials, "chatgpt_account_id"), item.AccountID},
		{"chatgpt_user_id", codexCredentialString(existing.Credentials, "chatgpt_user_id"), item.UserID},
	}
	for _, p := range pairs {
		stored, incoming := strings.TrimSpace(p.stored), strings.TrimSpace(p.incoming)
		if stored == "" || incoming == "" {
			continue
		}
		if stored != incoming {
			return errors.New("导入凭据的 " + p.label + " 与当前账号不一致（当前 " + stored + "，导入 " + incoming + "），请确认没有选错账号")
		}
		compared = true
	}
	if compared {
		return nil
	}

	storedEmail := strings.TrimSpace(codexCredentialString(existing.Credentials, "email"))
	incomingEmail := strings.TrimSpace(item.Email)
	if storedEmail != "" && incomingEmail != "" {
		if !strings.EqualFold(storedEmail, incomingEmail) {
			return errors.New("导入凭据的邮箱与当前账号不一致（当前 " + storedEmail + "，导入 " + incomingEmail + "），请确认没有选错账号")
		}
		return nil
	}
	return errors.New("无法确认导入凭据与当前账号属于同一 ChatGPT 用户（缺少 chatgpt_account_id / chatgpt_user_id / email）")
}
