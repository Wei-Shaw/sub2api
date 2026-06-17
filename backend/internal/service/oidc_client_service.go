// Package service ...
//
// oidc_client_service.go 实现 sub2api 作为 OIDC Provider 时的"已注册第三方客户端 (RP)"
// CRUD 与 Authenticate。本文件不涉及 HTTP handler / route 注册；admin handler 由
// Stage 1B-2 的任务 4.7-4.10 单独实现。
//
// 关键约束 (与 design.md D3 / spec.md 对齐)：
//
//   - client_secret 永远不存明文，仅存 bcrypt hash。
//   - client_id 形如 "rp_<base32 lowercase no padding>" (16B 随机 → 26 字符)。
//   - redirect_uris 严格相等匹配；https:// 强制 (allow http://localhost:* for dev)。
//   - allowed_scopes 必须是 [AllowedOidcProviderScopes] 子集。
//   - Delete 必须级联清掉本 client_id 的 oidc_consent / oidc_authorization_code /
//     oidc_refresh_token / oidc_access_token 行 (在同一 tx 内)。
package service

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/oidcaccesstoken"
	"github.com/Wei-Shaw/sub2api/ent/oidcauthorizationcode"
	"github.com/Wei-Shaw/sub2api/ent/oidcclient"
	"github.com/Wei-Shaw/sub2api/ent/oidcconsent"
	"github.com/Wei-Shaw/sub2api/ent/oidcrefreshtoken"

	"golang.org/x/crypto/bcrypt"
)

// ─── 常量 ────────────────────────────────────────────────────────────────────

const (
	// OidcClientIDPrefix client_id 公开前缀，便于日志辨识。
	OidcClientIDPrefix = "rp_"

	// oidcClientIDRandBytes 16B 随机 → base32 后 26 字符。
	oidcClientIDRandBytes = 16

	// oidcClientSecretRandBytes 32B 随机 → base64url 后 43 字符。
	oidcClientSecretRandBytes = 32

	// OidcClientNameMaxLen 显示名称最大长度。
	OidcClientNameMaxLen = 100
)

// DefaultOidcClientGrantTypes 默认且本期唯一支持的 grant_types 集合 (决策 3)。
var DefaultOidcClientGrantTypes = []string{"authorization_code", "refresh_token"}

// ─── 错误哨兵 ────────────────────────────────────────────────────────────────

var (
	// ErrOidcClientNotFound Get / Update / Delete / Authenticate 目标不存在。
	ErrOidcClientNotFound = errors.New("oidc client: not found")
	// ErrOidcClientDisabled Authenticate 时 client.enabled=false。
	ErrOidcClientDisabled = errors.New("oidc client: disabled")
	// ErrOidcClientWrongSecret Authenticate 时 secret hash 比对失败。
	ErrOidcClientWrongSecret = errors.New("oidc client: wrong secret")
	// ErrOidcClientNameRequired Create/Update 时 client_name 为空。
	ErrOidcClientNameRequired = errors.New("oidc client: client_name is required")
	// ErrOidcClientNameTooLong client_name 超过最大长度。
	ErrOidcClientNameTooLong = fmt.Errorf("oidc client: client_name exceeds %d chars", OidcClientNameMaxLen)
	// ErrOidcClientRedirectURIsRequired Create 时未提供任何 redirect_uri。
	ErrOidcClientRedirectURIsRequired = errors.New("oidc client: at least one redirect_uri required")
	// ErrOidcClientInvalidRedirectURI 单个 redirect_uri 格式校验失败 (上层 wrap 时附带具体值)。
	ErrOidcClientInvalidRedirectURI = errors.New("oidc client: invalid redirect_uri")
	// ErrOidcClientInvalidScope allowed_scopes 含未授权 scope (上层 wrap 时附带具体值)。
	ErrOidcClientInvalidScope = errors.New("oidc client: invalid scope")
)

// ─── 数据结构 ────────────────────────────────────────────────────────────────

// OidcClientView 是 service 层向上暴露的安全视图：永远**不**包含 secret hash。
type OidcClientView struct {
	ID              int64
	ClientID        string
	ClientName      string
	RedirectURIs    []string
	AllowedScopes   []string
	GrantTypes      []string
	ConsentRequired bool
	Enabled         bool
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// CreateOidcClientRequest Create 入参。
type CreateOidcClientRequest struct {
	ClientName      string
	RedirectURIs    []string
	AllowedScopes   []string
	ConsentRequired bool
	Enabled         bool
}

// UpdateOidcClientPatch Update 入参；nil 字段表示不修改。
type UpdateOidcClientPatch struct {
	ClientName      *string
	RedirectURIs    *[]string
	AllowedScopes   *[]string
	ConsentRequired *bool
	Enabled         *bool
}

// OidcClientListFilters List 过滤条件 (零值表示不过滤)。
type OidcClientListFilters struct {
	OnlyEnabled bool
	NameLike    string
}

// ─── 服务体 ──────────────────────────────────────────────────────────────────

// OidcClientService 管理 oidc_clients 表的 CRUD 与 Authenticate。
type OidcClientService struct {
	client *ent.Client

	// 注入点 (测试覆写)
	now             func() time.Time
	randReadFunc    func([]byte) (int, error)
	bcryptCost      int
	hashFunc        func(plain []byte, cost int) ([]byte, error) // bcrypt.GenerateFromPassword
	compareHashFunc func(hash, plain []byte) error               // bcrypt.CompareHashAndPassword
}

// NewOidcClientService 构造服务。client 必须非 nil。
func NewOidcClientService(client *ent.Client) *OidcClientService {
	return &OidcClientService{
		client:          client,
		now:             func() time.Time { return time.Now().UTC() },
		randReadFunc:    rand.Read,
		bcryptCost:      bcrypt.DefaultCost,
		hashFunc:        bcrypt.GenerateFromPassword,
		compareHashFunc: bcrypt.CompareHashAndPassword,
	}
}

// ─── Create ──────────────────────────────────────────────────────────────────

// Create 注册新 RP。返回 view + 一次性明文 secret (调用方必须立即返回给 admin，
// 之后无法再获取)。如果 req.GrantTypes 为空，自动填入 [DefaultOidcClientGrantTypes]。
//
// 校验顺序：name → redirect_uris → allowed_scopes。任意一步失败返回对应哨兵错误，
// 不写 DB。
func (s *OidcClientService) Create(ctx context.Context, req CreateOidcClientRequest) (*OidcClientView, string, error) {
	if s == nil || s.client == nil {
		return nil, "", fmt.Errorf("oidc client: nil client")
	}
	if err := s.validateName(req.ClientName); err != nil {
		return nil, "", err
	}
	if err := s.validateRedirectURIs(req.RedirectURIs); err != nil {
		return nil, "", err
	}
	if err := ValidateOidcProviderScopeSubset(req.AllowedScopes); err != nil {
		return nil, "", fmt.Errorf("%w: %v", ErrOidcClientInvalidScope, err)
	}

	clientID, err := s.generateClientID()
	if err != nil {
		return nil, "", err
	}
	plaintextSecret, err := s.generateSecret()
	if err != nil {
		return nil, "", err
	}
	hash, err := s.hashFunc([]byte(plaintextSecret), s.bcryptCost)
	if err != nil {
		return nil, "", fmt.Errorf("oidc client: bcrypt hash: %w", err)
	}

	row, err := s.client.OidcClient.Create().
		SetClientID(clientID).
		SetClientSecretHash(string(hash)).
		SetClientName(strings.TrimSpace(req.ClientName)).
		SetRedirectUris(normalizeStringSlice(req.RedirectURIs)).
		SetAllowedScopes(normalizeStringSlice(req.AllowedScopes)).
		SetGrantTypes(append([]string{}, DefaultOidcClientGrantTypes...)).
		SetConsentRequired(req.ConsentRequired).
		SetEnabled(req.Enabled).
		Save(ctx)
	if err != nil {
		return nil, "", fmt.Errorf("oidc client: insert: %w", err)
	}

	return rowToView(row), plaintextSecret, nil
}

// ─── List / Get ──────────────────────────────────────────────────────────────

// List 返回所有匹配过滤条件的 client。结果按 client_id 升序。
func (s *OidcClientService) List(ctx context.Context, filters OidcClientListFilters) ([]*OidcClientView, error) {
	if s == nil || s.client == nil {
		return nil, fmt.Errorf("oidc client: nil client")
	}
	q := s.client.OidcClient.Query()
	if filters.OnlyEnabled {
		q = q.Where(oidcclient.EnabledEQ(true))
	}
	if name := strings.TrimSpace(filters.NameLike); name != "" {
		q = q.Where(oidcclient.ClientNameContainsFold(name))
	}
	rows, err := q.Order(ent.Asc(oidcclient.FieldClientID)).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("oidc client: list: %w", err)
	}
	out := make([]*OidcClientView, 0, len(rows))
	for _, r := range rows {
		out = append(out, rowToView(r))
	}
	return out, nil
}

// Get 根据 ent 主键 id 取行。
func (s *OidcClientService) Get(ctx context.Context, id int64) (*OidcClientView, error) {
	if s == nil || s.client == nil {
		return nil, fmt.Errorf("oidc client: nil client")
	}
	row, err := s.client.OidcClient.Get(ctx, id)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrOidcClientNotFound
		}
		return nil, fmt.Errorf("oidc client: get: %w", err)
	}
	return rowToView(row), nil
}

// GetByClientID 根据业务 client_id 取行 (供 OIDC token endpoint 使用)。
func (s *OidcClientService) GetByClientID(ctx context.Context, clientID string) (*OidcClientView, error) {
	if s == nil || s.client == nil {
		return nil, fmt.Errorf("oidc client: nil client")
	}
	row, err := s.client.OidcClient.Query().
		Where(oidcclient.ClientIDEQ(clientID)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrOidcClientNotFound
		}
		return nil, fmt.Errorf("oidc client: get by client_id: %w", err)
	}
	return rowToView(row), nil
}

// ─── Update ──────────────────────────────────────────────────────────────────

// Update 局部更新。client_id / client_secret_hash / grant_types 在本接口中不可改
// (secret 走 ResetSecret；grant_types 本期固定)。
func (s *OidcClientService) Update(ctx context.Context, id int64, patch UpdateOidcClientPatch) (*OidcClientView, error) {
	if s == nil || s.client == nil {
		return nil, fmt.Errorf("oidc client: nil client")
	}
	row, err := s.client.OidcClient.Get(ctx, id)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrOidcClientNotFound
		}
		return nil, fmt.Errorf("oidc client: get for update: %w", err)
	}

	upd := s.client.OidcClient.UpdateOneID(row.ID)

	if patch.ClientName != nil {
		if err := s.validateName(*patch.ClientName); err != nil {
			return nil, err
		}
		upd = upd.SetClientName(strings.TrimSpace(*patch.ClientName))
	}
	if patch.RedirectURIs != nil {
		if err := s.validateRedirectURIs(*patch.RedirectURIs); err != nil {
			return nil, err
		}
		upd = upd.SetRedirectUris(normalizeStringSlice(*patch.RedirectURIs))
	}
	if patch.AllowedScopes != nil {
		if err := ValidateOidcProviderScopeSubset(*patch.AllowedScopes); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrOidcClientInvalidScope, err)
		}
		upd = upd.SetAllowedScopes(normalizeStringSlice(*patch.AllowedScopes))
	}
	if patch.ConsentRequired != nil {
		upd = upd.SetConsentRequired(*patch.ConsentRequired)
	}
	if patch.Enabled != nil {
		upd = upd.SetEnabled(*patch.Enabled)
	}

	updated, err := upd.Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("oidc client: update: %w", err)
	}
	return rowToView(updated), nil
}

// ─── ResetSecret ─────────────────────────────────────────────────────────────

// ResetSecret 生成新明文 secret + 重写 hash 字段；返回新明文。
//
// 一旦本方法返回成功，旧 secret 立即失效；持有旧 token 的客户端会在 token 过期后
// 拿不到新 token (因为 refresh 必须用 client secret 认证)；活跃 access/refresh
// token 不受影响 (符合"reset 不撤销已发出的 token"约定)。
func (s *OidcClientService) ResetSecret(ctx context.Context, id int64) (string, error) {
	if s == nil || s.client == nil {
		return "", fmt.Errorf("oidc client: nil client")
	}
	row, err := s.client.OidcClient.Get(ctx, id)
	if err != nil {
		if ent.IsNotFound(err) {
			return "", ErrOidcClientNotFound
		}
		return "", fmt.Errorf("oidc client: get for reset: %w", err)
	}
	plaintext, err := s.generateSecret()
	if err != nil {
		return "", err
	}
	hash, err := s.hashFunc([]byte(plaintext), s.bcryptCost)
	if err != nil {
		return "", fmt.Errorf("oidc client: bcrypt hash: %w", err)
	}
	if _, err := s.client.OidcClient.UpdateOneID(row.ID).
		SetClientSecretHash(string(hash)).
		Save(ctx); err != nil {
		return "", fmt.Errorf("oidc client: persist new hash: %w", err)
	}
	return plaintext, nil
}

// ─── Delete (cascade) ────────────────────────────────────────────────────────

// Delete 删除指定 client，并在同一 tx 内清掉所有 oidc_consent /
// oidc_authorization_code / oidc_refresh_token / oidc_access_token 中
// 引用同 client_id 的行。
//
// 行为：
//   - id 不存在 → ErrOidcClientNotFound
//   - 任意 cascade 操作失败 → tx 回滚，返回 wrap 错误，DB 状态不变
func (s *OidcClientService) Delete(ctx context.Context, id int64) error {
	if s == nil || s.client == nil {
		return fmt.Errorf("oidc client: nil client")
	}

	row, err := s.client.OidcClient.Get(ctx, id)
	if err != nil {
		if ent.IsNotFound(err) {
			return ErrOidcClientNotFound
		}
		return fmt.Errorf("oidc client: get for delete: %w", err)
	}
	clientID := row.ClientID

	tx, err := s.client.Tx(ctx)
	if err != nil {
		return fmt.Errorf("oidc client: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.OidcConsent.Delete().
		Where(oidcconsent.ClientIDEQ(clientID)).
		Exec(ctx); err != nil {
		return fmt.Errorf("oidc client: cascade consent: %w", err)
	}
	if _, err := tx.OidcAuthorizationCode.Delete().
		Where(oidcauthorizationcode.ClientIDEQ(clientID)).
		Exec(ctx); err != nil {
		return fmt.Errorf("oidc client: cascade auth code: %w", err)
	}
	if _, err := tx.OidcRefreshToken.Delete().
		Where(oidcrefreshtoken.ClientIDEQ(clientID)).
		Exec(ctx); err != nil {
		return fmt.Errorf("oidc client: cascade refresh: %w", err)
	}
	if _, err := tx.OidcAccessToken.Delete().
		Where(oidcaccesstoken.ClientIDEQ(clientID)).
		Exec(ctx); err != nil {
		return fmt.Errorf("oidc client: cascade access: %w", err)
	}
	if err := tx.OidcClient.DeleteOneID(row.ID).Exec(ctx); err != nil {
		return fmt.Errorf("oidc client: delete row: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("oidc client: commit delete tx: %w", err)
	}
	return nil
}

// ─── Authenticate ────────────────────────────────────────────────────────────

// Authenticate 校验 (client_id, presented_secret) 是否匹配且 client.enabled=true。
//
// 返回的错误哨兵：
//   - ErrOidcClientNotFound: client_id 不存在
//   - ErrOidcClientDisabled: client.enabled=false
//   - ErrOidcClientWrongSecret: bcrypt 比对失败
//   - 其他 wrap 错误: DB 故障
//
// 调用方 (OIDC token handler) 必须把 NotFound + WrongSecret **统一**映射为
// HTTP 401 + `error=invalid_client`，避免泄露"该 client_id 是否存在"。
func (s *OidcClientService) Authenticate(ctx context.Context, clientID, presentedSecret string) (*OidcClientView, error) {
	if s == nil || s.client == nil {
		return nil, fmt.Errorf("oidc client: nil client")
	}
	clientID = strings.TrimSpace(clientID)
	if clientID == "" {
		return nil, ErrOidcClientNotFound
	}
	row, err := s.client.OidcClient.Query().
		Where(oidcclient.ClientIDEQ(clientID)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrOidcClientNotFound
		}
		return nil, fmt.Errorf("oidc client: lookup for auth: %w", err)
	}
	if !row.Enabled {
		return nil, ErrOidcClientDisabled
	}
	if err := s.compareHashFunc([]byte(row.ClientSecretHash), []byte(presentedSecret)); err != nil {
		return nil, ErrOidcClientWrongSecret
	}
	return rowToView(row), nil
}

// ─── 内部工具 ────────────────────────────────────────────────────────────────

// validateName 校验 client_name 长度。
func (s *OidcClientService) validateName(name string) error {
	v := strings.TrimSpace(name)
	if v == "" {
		return ErrOidcClientNameRequired
	}
	if len(v) > OidcClientNameMaxLen {
		return ErrOidcClientNameTooLong
	}
	return nil
}

// validateRedirectURIs 严格校验 redirect_uri 列表 (设计 D3 + spec 合规)。
//
// 规则：
//   - 至少 1 个
//   - 每个 entry 必须是合法 URL
//   - scheme 必须是 https，但允许 http://localhost{:port} 用于本地开发
//   - 不允许尾部 # / ? (避免和 OIDC 拼接错位)
//   - 严格相等匹配，存什么是什么 (不规范化)
func (s *OidcClientService) validateRedirectURIs(uris []string) error {
	if len(uris) == 0 {
		return ErrOidcClientRedirectURIsRequired
	}
	seen := make(map[string]struct{}, len(uris))
	for _, raw := range uris {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			return fmt.Errorf("%w: empty entry", ErrOidcClientInvalidRedirectURI)
		}
		if _, dup := seen[trimmed]; dup {
			return fmt.Errorf("%w: duplicate %q", ErrOidcClientInvalidRedirectURI, trimmed)
		}
		seen[trimmed] = struct{}{}

		u, err := url.Parse(trimmed)
		if err != nil {
			return fmt.Errorf("%w: %q (%v)", ErrOidcClientInvalidRedirectURI, trimmed, err)
		}
		if u.Fragment != "" || strings.Contains(trimmed, "#") {
			return fmt.Errorf("%w: %q must not contain fragment", ErrOidcClientInvalidRedirectURI, trimmed)
		}
		if u.Scheme == "https" {
			if u.Host == "" {
				return fmt.Errorf("%w: %q missing host", ErrOidcClientInvalidRedirectURI, trimmed)
			}
			continue
		}
		// 允许 http://localhost{:port} 仅供 dev
		if u.Scheme == "http" {
			host := u.Hostname()
			if host == "localhost" || host == "127.0.0.1" || host == "::1" {
				continue
			}
			return fmt.Errorf("%w: %q (only https or http://localhost is allowed)", ErrOidcClientInvalidRedirectURI, trimmed)
		}
		return fmt.Errorf("%w: %q has unsupported scheme %q", ErrOidcClientInvalidRedirectURI, trimmed, u.Scheme)
	}
	return nil
}

// generateClientID 生成 "rp_<base32 lowercase no padding>" 形式的 client_id。
func (s *OidcClientService) generateClientID() (string, error) {
	buf := make([]byte, oidcClientIDRandBytes)
	if _, err := s.randReadFunc(buf); err != nil {
		return "", fmt.Errorf("oidc client: rand client id: %w", err)
	}
	encoder := base32.StdEncoding.WithPadding(base32.NoPadding)
	return OidcClientIDPrefix + strings.ToLower(encoder.EncodeToString(buf)), nil
}

// generateSecret 生成 base64url 编码的随机 secret，调用方一次性返回给 admin。
func (s *OidcClientService) generateSecret() (string, error) {
	buf := make([]byte, oidcClientSecretRandBytes)
	if _, err := s.randReadFunc(buf); err != nil {
		return "", fmt.Errorf("oidc client: rand secret: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// rowToView 把 ent 行转成对外安全视图 (剥离 secret hash)。
func rowToView(row *ent.OidcClient) *OidcClientView {
	if row == nil {
		return nil
	}
	return &OidcClientView{
		ID:              int64(row.ID),
		ClientID:        row.ClientID,
		ClientName:      row.ClientName,
		RedirectURIs:    append([]string{}, row.RedirectUris...),
		AllowedScopes:   append([]string{}, row.AllowedScopes...),
		GrantTypes:      append([]string{}, row.GrantTypes...),
		ConsentRequired: row.ConsentRequired,
		Enabled:         row.Enabled,
		CreatedAt:       row.CreatedAt,
		UpdatedAt:       row.UpdatedAt,
	}
}

// normalizeStringSlice 去掉首尾空白 + 去重 (保序)，且永不返回 nil。
func normalizeStringSlice(in []string) []string {
	if len(in) == 0 {
		return []string{}
	}
	out := make([]string, 0, len(in))
	seen := make(map[string]struct{}, len(in))
	for _, v := range in {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}
