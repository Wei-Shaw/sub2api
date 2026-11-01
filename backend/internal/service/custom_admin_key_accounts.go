package service

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"
	"unicode/utf8"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/adminaccess"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

// ScopedAdminAccounts 只提供受限管理密钥需要的窄接口。
type ScopedAdminAccounts interface {
	ScopedAdminList(context.Context, pagination.PaginationParams, string, string, string, string) ([]Account, int64, error)
	ScopedAdminWrite(context.Context, int64, string, map[string]json.RawMessage, func(context.Context, *Account) (*Account, error)) (*Account, error)
}

func scopedAccountResource(a *Account) adminaccess.Account {
	return adminaccess.Account{ID: a.ID, GroupIDs: a.GroupIDs, Type: a.Type, Platform: a.Platform}
}

func scopedAccountFieldPermissions(fields map[string]json.RawMessage, create bool) ([]string, error) {
	permissions := []string{}
	for key, raw := range fields {
		permission := ""
		switch key {
		case "name", "notes":
			permission = "accounts.update"
		case "status":
			var status string
			if json.Unmarshal(raw, &status) != nil || !slices.Contains([]string{StatusActive, "inactive", StatusError}, status) {
				return nil, infraerrors.BadRequest("SCOPED_ACCOUNT_STATUS", "账户状态无效")
			}
			permission = "accounts.status"
		case "schedulable":
			permission = "accounts.status"
		case "concurrency", "priority", "load_factor", "expires_at", "auto_pause_on_expired":
			permission = "accounts.schedule"
		case "proxy_id", "proxy_mode", "random_proxy":
			permission = "accounts.network"
		case "group_ids":
			permission = "accounts.groups"
		case "platform", "type":
			if !create {
				return nil, infraerrors.BadRequest("SCOPED_ACCOUNT_INPUT", "禁止更改类型或平台")
			}
			continue
		case "credentials":
			var credentials map[string]json.RawMessage
			if json.Unmarshal(raw, &credentials) != nil || len(credentials) == 0 {
				return nil, infraerrors.BadRequest("SCOPED_ACCOUNT_CREDENTIALS", "凭证格式无效")
			}
			for name, value := range credentials {
				// 白名单与映射共用 model_mapping，只授予模型配置写入权限。
				if name == "model_mapping" {
					var mapping map[string]json.RawMessage
					if len(value) > 65536 || json.Unmarshal(value, &mapping) != nil {
						return nil, infraerrors.BadRequest("SCOPED_ACCOUNT_MODELS", "模型映射格式无效")
					}
					for source, target := range mapping {
						var model string
						if strings.TrimSpace(source) == "" || json.Unmarshal(target, &model) != nil || strings.TrimSpace(model) == "" {
							return nil, infraerrors.BadRequest("SCOPED_ACCOUNT_MODELS", "模型 ID 格式无效")
						}
					}
					permissions = append(permissions, "accounts.models.write")
					continue
				}
				var text string
				if json.Unmarshal(value, &text) != nil || len(text) > 65536 {
					return nil, infraerrors.BadRequest("SCOPED_ACCOUNT_CREDENTIALS", "凭证值格式无效")
				}
				switch name {
				case "api_key", "access_token", "refresh_token", "id_token", "session_key":
					permissions = append(permissions, "accounts.credentials")
				case "base_url":
					permissions = append(permissions, "accounts.network")
				default:
					return nil, infraerrors.BadRequest("SCOPED_ACCOUNT_FIELD", "请求包含不开放的凭证字段")
				}
			}
			continue
		default:
			return nil, infraerrors.BadRequest("SCOPED_ACCOUNT_FIELD", "请求包含不开放的账户字段")
		}
		if !create || permission == "accounts.schedule" || permission == "accounts.network" {
			permissions = append(permissions, permission)
		} else if key == "status" || key == "schedulable" {
			return nil, infraerrors.BadRequest("SCOPED_ACCOUNT_INPUT", "创建不支持状态字段")
		}
	}
	return permissions, nil
}

func (s *adminServiceImpl) ScopedAdminList(ctx context.Context, p pagination.PaginationParams, platform, kind, status, search string) ([]Account, int64, error) {
	repo, ok := s.accountRepo.(interface {
		ScopedAdminList(context.Context, pagination.PaginationParams, string, string, string, string) ([]Account, int64, error)
	})
	if !ok {
		return nil, 0, fmt.Errorf("受限账户查询不可用")
	}
	return repo.ScopedAdminList(ctx, p, platform, kind, status, search)
}

func (s *adminServiceImpl) ScopedAdminWrite(ctx context.Context, id int64, action string, fields map[string]json.RawMessage, run func(context.Context, *Account) (*Account, error)) (*Account, error) {
	principal := adminaccess.FromContext(ctx)
	if principal == nil {
		return nil, infraerrors.Forbidden("SCOPED_ACCOUNT_FORBIDDEN", "缺少受限身份")
	}
	permissions, err := scopedAccountFieldPermissions(fields, id == 0)
	if err != nil {
		return nil, err
	}
	if id != 0 && action == "" && len(permissions) == 0 {
		return nil, infraerrors.BadRequest("SCOPED_ACCOUNT_INPUT", "缺少实际授权操作")
	}
	if id == 0 {
		var input struct {
			ProxyID     *int64         `json:"proxy_id"`
			ProxyMode   string         `json:"proxy_mode"`
			RandomProxy bool           `json:"random_proxy"`
			Name        string         `json:"name"`
			Notes       *string        `json:"notes"`
			Platform    string         `json:"platform"`
			Type        string         `json:"type"`
			Credentials map[string]any `json:"credentials"`
			GroupIDs    []int64        `json:"group_ids"`
			Concurrency int            `json:"concurrency"`
			Priority    int            `json:"priority"`
			LoadFactor  *int           `json:"load_factor"`
			ExpiresAt   *int64         `json:"expires_at"`
			AutoPause   *bool          `json:"auto_pause_on_expired"`
		}
		body, _ := json.Marshal(fields)
		if err = json.Unmarshal(body, &input); err != nil {
			return nil, infraerrors.BadRequest("SCOPED_ACCOUNT_INPUT", "请求格式无效")
		}
		if strings.TrimSpace(input.Name) == "" || utf8.RuneCountInString(input.Name) > maxAccountNameRunes || len(input.Credentials) == 0 || fields["group_ids"] == nil {
			return nil, infraerrors.BadRequest("SCOPED_ACCOUNT_INPUT", "创建须提供名称、凭证及明确分组")
		}
		if !slices.Contains([]string{"oauth", "setup-token", "apikey", "upstream", "bedrock", "service_account"}, input.Type) {
			return nil, infraerrors.BadRequest("SCOPED_ACCOUNT_INPUT", "账户类型无效")
		}
		if !slices.Contains(domain.ConcretePlatformIDs(), input.Platform) || input.Concurrency < 0 || input.Priority < 0 {
			return nil, infraerrors.BadRequest("SCOPED_ACCOUNT_INPUT", "创建参数无效")
		}
		resource := adminaccess.Account{Type: input.Type, Platform: input.Platform, GroupIDs: input.GroupIDs}
		if input.Type == "" || input.Platform == "" || !principal.Policy.AllowsAccount("accounts.create", resource) {
			return nil, infraerrors.Forbidden("SCOPED_ACCOUNT_FORBIDDEN", "创建范围未授权")
		}
		for _, g := range input.GroupIDs {
			if !principal.Policy.AllowsGroup("accounts.create", resource, g) {
				return nil, infraerrors.Forbidden("SCOPED_ACCOUNT_FORBIDDEN", "创建分组未授权")
			}
		}
		// 创建本身允许初始化普通字段；凭证和网络须独立授权。
		for _, p := range permissions {
			if !principal.Policy.AllowsAccount(p, resource) {
				return nil, infraerrors.Forbidden("SCOPED_ACCOUNT_FORBIDDEN", "字段权限不足")
			}
		}
		proxyMode, e := NormalizeAccountProxyMode(input.ProxyMode, input.ProxyID, input.RandomProxy)
		if e != nil {
			return nil, e
		}
		return s.scopedAdminCreateTransaction(ctx, &CreateAccountInput{ProxyID: input.ProxyID, ProxyMode: proxyMode, Name: input.Name, Notes: input.Notes, Platform: input.Platform, Type: input.Type, Credentials: input.Credentials, GroupIDs: input.GroupIDs, Concurrency: input.Concurrency, Priority: input.Priority, LoadFactor: input.LoadFactor, ExpiresAt: input.ExpiresAt, AutoPauseOnExpired: input.AutoPause, SkipDefaultGroupBind: true})
	}
	result, err := s.scopedAdminTransaction(ctx, id, action == "accounts.delete", func(txctx context.Context) (*Account, error) {
		a, e := s.accountRepo.GetByID(txctx, id)
		if e != nil {
			return nil, infraerrors.NotFound("SCOPED_ACCOUNT_NOT_FOUND", "账户不存在")
		}
		if a == nil {
			return nil, infraerrors.NotFound("SCOPED_ACCOUNT_NOT_FOUND", "账户不存在")
		}
		resource := scopedAccountResource(a)
		// 先隐藏授权范围外的账户，再区分范围内缺少具体字段权限的情况。
		inScope := false
		for _, rule := range principal.Policy.Rules {
			if slices.ContainsFunc(rule.Permissions, func(permission string) bool { return strings.HasPrefix(permission, "accounts.") }) && rule.Matches(resource) {
				inScope = true
				break
			}
		}
		if !inScope {
			return nil, infraerrors.NotFound("SCOPED_ACCOUNT_NOT_FOUND", "账户不存在")
		}
		if action != "" && !principal.Policy.AllowsAccount(action, resource) {
			return nil, infraerrors.NotFound("SCOPED_ACCOUNT_NOT_FOUND", "账户不存在")
		}
		for _, p := range permissions {
			if !principal.Policy.AllowsAccount(p, resource) {
				return nil, infraerrors.Forbidden("SCOPED_ACCOUNT_FORBIDDEN", "账户字段未授权")
			}
		}
		if a.IsShadow() {
			return nil, infraerrors.Forbidden("SCOPED_ACCOUNT_FORBIDDEN", "不支持修改影子账户")
		}
		shadows, e := s.accountRepo.ListShadowsByParent(txctx, id)
		if e != nil {
			return nil, e
		}
		if len(shadows) > 0 {
			return nil, infraerrors.Forbidden("SCOPED_ACCOUNT_FORBIDDEN", "请由完整管理员管理关联影子账户")
		}

		if run != nil {
			return run(txctx, a)
		}
		if action == "accounts.delete" {
			return a, s.DeleteAccount(txctx, id)
		}
		if action == "accounts.recover" {
			return s.ClearAccountError(txctx, id)
		}
		var input struct {
			ProxyID     *int64         `json:"proxy_id"`
			ProxyMode   string         `json:"proxy_mode"`
			RandomProxy bool           `json:"random_proxy"`
			Name        string         `json:"name"`
			Notes       *string        `json:"notes"`
			Credentials map[string]any `json:"credentials"`
			Concurrency *int           `json:"concurrency"`
			Priority    *int           `json:"priority"`
			LoadFactor  *int           `json:"load_factor"`
			Status      string         `json:"status"`
			GroupIDs    *[]int64       `json:"group_ids"`
			ExpiresAt   *int64         `json:"expires_at"`
			AutoPause   *bool          `json:"auto_pause_on_expired"`
			Schedulable *bool          `json:"schedulable"`
		}
		body, _ := json.Marshal(fields)
		if e = json.Unmarshal(body, &input); e != nil {
			return nil, infraerrors.BadRequest("SCOPED_ACCOUNT_INPUT", "请求格式无效")
		}
		if input.GroupIDs != nil {
			groups, err := scopedAdminMergeGroups(principal.Policy, resource, a.GroupIDs, *input.GroupIDs)
			if err != nil {
				return nil, err
			}
			input.GroupIDs = &groups
		}
		proxyMode, e := NormalizeAccountProxyMode(input.ProxyMode, input.ProxyID, input.RandomProxy)
		if e != nil {
			return nil, e
		}
		input.ProxyID, e = s.scopedAccountProxySelection(txctx, proxyMode, input.ProxyID)
		if e != nil {
			return nil, e
		}
		// 受限更新按已授权字段补丁合并，避免上游完整编辑语义删除未授予的连接配置。
		if input.Credentials != nil {
			merged := maps.Clone(a.Credentials)
			if merged == nil {
				merged = make(map[string]any)
			}
			maps.Copy(merged, input.Credentials)
			input.Credentials = merged
		}
		updated, e := s.UpdateAccount(txctx, id, &UpdateAccountInput{ProxyID: input.ProxyID, Name: input.Name, Notes: input.Notes, Credentials: input.Credentials, Concurrency: input.Concurrency, Priority: input.Priority, LoadFactor: input.LoadFactor, Status: input.Status, GroupIDs: input.GroupIDs, ExpiresAt: input.ExpiresAt, AutoPauseOnExpired: input.AutoPause})
		if e != nil {
			return nil, e
		}
		if input.Schedulable != nil {
			return s.SetAccountSchedulable(txctx, id, *input.Schedulable)
		}
		return updated, nil
	})
	return result, err
}

// scopedAdminTransaction 锁住账户行，分组更改和外部操作均须在重新验权之后执行。
func (s *adminServiceImpl) scopedAdminTransaction(ctx context.Context, id int64, deleted bool, run func(context.Context) (*Account, error)) (*Account, error) {
	if s.entClient == nil {
		return nil, fmt.Errorf("受限写入需要数据库事务")
	}
	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	txctx := dbent.NewTxContext(ctx, tx)
	if err = lockScopedAdminAccount(txctx, tx, id); err != nil {
		return nil, err
	}
	if _, err = tx.Client().ExecContext(txctx, "SELECT id FROM accounts WHERE id = $1 AND deleted_at IS NULL FOR UPDATE", id); err != nil {
		return nil, err
	}
	if _, err = tx.Client().ExecContext(txctx, "SELECT account_id FROM account_groups WHERE account_id = $1 FOR UPDATE", id); err != nil {
		return nil, err
	}
	result, err := run(txctx)
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	if repo, ok := s.accountRepo.(interface {
		ScopedAdminCommitted(context.Context, int64, bool)
	}); ok {
		repo.ScopedAdminCommitted(ctx, id, deleted)
	}
	return result, nil
}

// scopedAdminMergeGroups 只替换授权关联，保留调用者不可操作的既有分组。
func scopedAdminMergeGroups(policy adminaccess.Policy, resource adminaccess.Account, existing, requested []int64) ([]int64, error) {
	groups := []int64{}
	for _, id := range existing {
		if !policy.AllowsGroup("accounts.groups", resource, id) {
			groups = append(groups, id)
		}
	}
	for _, id := range requested {
		if !policy.AllowsGroup("accounts.groups", resource, id) {
			return nil, infraerrors.Forbidden("SCOPED_ACCOUNT_GROUP", "目标分组未授权")
		}
		if !slices.Contains(groups, id) {
			groups = append(groups, id)
		}
	}
	return groups, nil
}

// scopedAdminCreateTransaction 保证创建与绑定分组一同提交或回滚。
func (s *adminServiceImpl) scopedAdminCreateTransaction(ctx context.Context, input *CreateAccountInput) (*Account, error) {
	if s.entClient == nil {
		return nil, fmt.Errorf("受限写入需要数据库事务")
	}
	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	txctx := dbent.NewTxContext(ctx, tx)
	safe := *input
	safe.ProxyID, err = s.scopedAccountProxySelection(txctx, safe.ProxyMode, safe.ProxyID)
	if err != nil {
		return nil, err
	}
	// 创建以 NULL 表示直连；更新接口的 0 仅是清除既有绑定的指令。
	if safe.ProxyID != nil && *safe.ProxyID == 0 {
		safe.ProxyID = nil
	}
	account, err := s.CreateAccount(txctx, &safe)
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	if repo, ok := s.accountRepo.(interface {
		ScopedAdminCommitted(context.Context, int64, bool)
	}); ok {
		repo.ScopedAdminCommitted(ctx, account.ID, false)
	}
	return account, nil
}
