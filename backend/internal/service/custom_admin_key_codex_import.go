package service

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Wei-Shaw/sub2api/internal/pkg/adminaccess"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// ScopedCodexIdentity 描述单次导入的身份；不携带刷新令牌或其他待写入凭证。
type ScopedCodexIdentity struct {
	AccountID       string
	UserID          string
	Email           string
	AccessToken     string
	AccessTokenOnly bool
	AgentIdentity   bool
}

// ScopedAdminCodexAccounts 仅供受限导入器内部使用，候选凭证不得写入 HTTP 响应。
type ScopedAdminCodexAccounts interface {
	ScopedAdminCodexCandidates(context.Context, ScopedCodexIdentity) ([]Account, error)
	ScopedAdminCreateCodex(context.Context, *CreateAccountInput) (*Account, error)
	ScopedAdminUpdateCodex(context.Context, int64, *UpdateAccountInput, func(*Account, *UpdateAccountInput) error) (*Account, error)
}

func (s *adminServiceImpl) ScopedAdminCodexCandidates(ctx context.Context, identity ScopedCodexIdentity) ([]Account, error) {
	repo, ok := s.accountRepo.(interface {
		ScopedAdminCodexCandidates(context.Context, ScopedCodexIdentity) ([]Account, error)
	})
	if !ok {
		return nil, fmt.Errorf("受限导入查询不可用")
	}
	return repo.ScopedAdminCodexCandidates(ctx, identity)
}

// 只允许解析器生成的认证元数据及独立授权的模型/连接配置，禁止任意 extra 穿透。
func scopedCodexPermissions(credentials, extra map[string]any, proxy *int64) ([]string, error) {
	permissions := []string{"accounts.credentials", "accounts.schedule"}
	for key, value := range credentials {
		switch key {
		case "access_token", "refresh_token", "id_token", "client_id", "expires_at", "email", "chatgpt_account_id", "chatgpt_user_id", "organization_id", "plan_type", "auth_mode", "agent_runtime_id", "agent_private_key", "task_id":
			if _, ok := value.(string); !ok {
				return nil, infraerrors.BadRequest("SCOPED_CODEX_INPUT", "认证字段格式无效")
			}
		case "chatgpt_account_is_fedramp":
			if _, ok := value.(bool); !ok {
				return nil, infraerrors.BadRequest("SCOPED_CODEX_INPUT", "认证字段格式无效")
			}
		case "base_url", "model_mapping":
			raw, err := json.Marshal(map[string]any{key: value})
			if err != nil {
				return nil, err
			}
			required, err := scopedAccountFieldPermissions(map[string]json.RawMessage{"credentials": raw}, false)
			if err != nil {
				return nil, err
			}
			permissions = append(permissions, required...)
		default:
			return nil, infraerrors.BadRequest("SCOPED_CODEX_FIELD", "导入包含未开放的凭证字段")
		}
	}
	for key := range extra {
		switch key {
		case "import_source", "imported_at", "auth_provider", "session_token_present", "session_expires_at", "user_image", "user_picture", "account_structure", "account_residency_region", "compute_residency", "access_token_sha256":
		case "codex_fingerprint_mode", "openai_oauth_responses_websockets_v2_enabled", "openai_oauth_responses_websockets_v2_mode":
			permissions = append(permissions, "accounts.network")
		default:
			return nil, infraerrors.BadRequest("SCOPED_CODEX_FIELD", "导入包含未开放的扩展字段")
		}
	}
	if proxy != nil {
		permissions = append(permissions, "accounts.network")
	}
	return permissions, nil
}

func requireScopedCodexPermissions(ctx context.Context, resource adminaccess.Account, permissions ...string) error {
	principal := adminaccess.FromContext(ctx)
	for _, permission := range permissions {
		if principal == nil || !principal.Policy.AllowsAccount(permission, resource) {
			return infraerrors.Forbidden("SCOPED_CODEX_FORBIDDEN", "导入权限不足")
		}
	}
	return nil
}

func (s *adminServiceImpl) ScopedAdminCreateCodex(ctx context.Context, input *CreateAccountInput) (*Account, error) {
	if input.Platform != PlatformOpenAI || input.Type != AccountTypeOAuth || input.GroupIDs == nil || len(input.Credentials) == 0 || input.SkipMixedChannelCheck {
		return nil, infraerrors.BadRequest("SCOPED_CODEX_INPUT", "导入须明确 OAuth 类型和目标分组，且不能跳过分组风险检查")
	}
	permissions, err := scopedCodexPermissions(input.Credentials, input.Extra, input.ProxyID)
	if input.ProxyMode != "" {
		permissions = append(permissions, "accounts.network")
	}
	if err != nil {
		return nil, err
	}
	resource := adminaccess.Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, GroupIDs: input.GroupIDs}
	if err = requireScopedCodexPermissions(ctx, resource, append(permissions, "accounts.create")...); err != nil {
		return nil, err
	}
	for _, group := range input.GroupIDs {
		if !adminaccess.FromContext(ctx).Policy.AllowsGroup("accounts.create", resource, group) {
			return nil, infraerrors.Forbidden("SCOPED_CODEX_GROUP", "目标分组未授权")
		}
	}
	if input.RateMultiplier != nil && *input.RateMultiplier != 1 {
		return nil, infraerrors.Forbidden("SCOPED_CODEX_RATE", "受限导入不能修改计费倍率")
	}
	safe := *input
	safe.SkipDefaultGroupBind = true
	return s.scopedAdminCreateTransaction(ctx, &safe)
}

func (s *adminServiceImpl) ScopedAdminUpdateCodex(ctx context.Context, id int64, input *UpdateAccountInput, prepare func(*Account, *UpdateAccountInput) error) (*Account, error) {
	permissions, err := scopedCodexPermissions(input.Credentials, input.Extra, input.ProxyID)
	if input.ProxyMode != "" {
		permissions = append(permissions, "accounts.network")
	}
	if err != nil {
		return nil, err
	}
	result, err := s.ScopedAdminWrite(ctx, id, "accounts.credentials", nil, func(txctx context.Context, current *Account) (*Account, error) {
		if !current.IsOpenAIOAuth() {
			return nil, infraerrors.NotFound("SCOPED_ACCOUNT_NOT_FOUND", "账户不存在")
		}
		resource := scopedAccountResource(current)
		if err := requireScopedCodexPermissions(txctx, resource, permissions...); err != nil {
			return nil, err
		}
		safe := *input
		if safe.SkipMixedChannelCheck {
			return nil, infraerrors.Forbidden("SCOPED_CODEX_INPUT", "受限导入不能跳过分组风险检查")
		}
		if safe.GroupIDs != nil {
			if err := requireScopedCodexPermissions(txctx, resource, "accounts.groups"); err != nil {
				return nil, err
			}
			groups, err := scopedAdminMergeGroups(adminaccess.FromContext(txctx).Policy, resource, current.GroupIDs, *safe.GroupIDs)
			if err != nil {
				return nil, err
			}
			safe.GroupIDs = &groups
		}
		// 客户端常附带默认倍率；只接受与现值相同的值，绝不借导入改价。
		if safe.RateMultiplier != nil && *safe.RateMultiplier != current.BillingRateMultiplier() {
			return nil, infraerrors.Forbidden("SCOPED_CODEX_RATE", "受限导入不能修改计费倍率")
		}
		safe.RateMultiplier = nil
		if err := prepare(current, &safe); err != nil {
			return nil, err
		}
		safe.ProxyID, err = s.scopedAccountProxySelection(txctx, safe.ProxyMode, safe.ProxyID)
		if err != nil {
			return nil, err
		}
		return s.UpdateAccount(txctx, id, &safe)
	})
	return result, err
}
