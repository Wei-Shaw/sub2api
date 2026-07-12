// Package service ...
//
// oidc_consent_service.go 实现 (user_id, client_id) 维度的同意 scope 记忆。
// 行为：
//   - LoadGrantedScopes 返回该用户对该 client 已同意的 scope 集合
//   - Grant 用并集语义 upsert (新 scope 与历史合并)，并刷新 last_used_at
//   - Revoke 删除该 (user_id, client_id) 行
//   - IsCovered 严格 superset 判断 (granted ⊇ requested)
//
// 与 design.md D5 一致：增量 scope 走重新 consent，整体 superset 命中则跳过 consent 页。
package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/oidcconsent"
)

// OidcConsentService 不持有内存状态；纯 ent 操作。
type OidcConsentService struct {
	client *ent.Client
	now    func() time.Time
}

// NewOidcConsentService 构造服务。
func NewOidcConsentService(client *ent.Client) *OidcConsentService {
	return &OidcConsentService{
		client: client,
		now:    func() time.Time { return time.Now().UTC() },
	}
}

// LoadGrantedScopes 返回 (granted, found)。found=false 时 granted 始终为 nil。
//
// found=true 但 granted=nil 是合法情况 (旧逻辑 Revoke 后行可能不再存在；本服务不会出现这种)。
func (s *OidcConsentService) LoadGrantedScopes(ctx context.Context, userID int64, clientID string) ([]string, bool, error) {
	if s == nil || s.client == nil {
		return nil, false, fmt.Errorf("oidc consent: nil client")
	}
	row, err := s.client.OidcConsent.Query().
		Where(
			oidcconsent.UserIDEQ(userID),
			oidcconsent.ClientIDEQ(clientID),
		).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("oidc consent: load: %w", err)
	}
	// 防御：保证返回切片非 nil
	if row.GrantedScopes == nil {
		return []string{}, true, nil
	}
	return row.GrantedScopes, true, nil
}

// Grant upsert：若已存在则 granted_scopes = 旧 ∪ 新，且更新 last_used_at；否则插入新行。
//
// scopes 为 nil 或 empty 时仍会更新 last_used_at（视为"再次使用"）。
func (s *OidcConsentService) Grant(ctx context.Context, userID int64, clientID string, scopes []string) error {
	if s == nil || s.client == nil {
		return fmt.Errorf("oidc consent: nil client")
	}
	if clientID == "" {
		return fmt.Errorf("oidc consent: empty client id")
	}

	now := s.now()
	row, err := s.client.OidcConsent.Query().
		Where(
			oidcconsent.UserIDEQ(userID),
			oidcconsent.ClientIDEQ(clientID),
		).
		Only(ctx)
	if err != nil {
		if !ent.IsNotFound(err) {
			return fmt.Errorf("oidc consent: load: %w", err)
		}
		// 插入新行
		toInsert := dedupeStrings(scopes)
		if toInsert == nil {
			toInsert = []string{}
		}
		_, err := s.client.OidcConsent.Create().
			SetUserID(userID).
			SetClientID(clientID).
			SetGrantedScopes(toInsert).
			SetGrantedAt(now).
			SetLastUsedAt(now).
			Save(ctx)
		if err != nil {
			return fmt.Errorf("oidc consent: insert: %w", err)
		}
		return nil
	}

	merged := unionStrings(row.GrantedScopes, scopes)
	if _, err := s.client.OidcConsent.UpdateOneID(row.ID).
		SetGrantedScopes(merged).
		SetLastUsedAt(now).
		Save(ctx); err != nil {
		return fmt.Errorf("oidc consent: update: %w", err)
	}
	return nil
}

// Revoke 删除指定 (user_id, client_id) 行。已不存在则返回 ErrOidcConsentNotFound。
func (s *OidcConsentService) Revoke(ctx context.Context, userID int64, clientID string) error {
	if s == nil || s.client == nil {
		return fmt.Errorf("oidc consent: nil client")
	}
	n, err := s.client.OidcConsent.Delete().
		Where(
			oidcconsent.UserIDEQ(userID),
			oidcconsent.ClientIDEQ(clientID),
		).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("oidc consent: revoke: %w", err)
	}
	if n == 0 {
		return ErrOidcConsentNotFound
	}
	return nil
}

// TouchLastUsed 仅刷新 last_used_at，不动 granted_scopes。
//
// 用于：authorize 命中已有 consent superset 时记录"该用户最近 X 用过此 client"。
// 行不存在时返回 ErrOidcConsentNotFound。
func (s *OidcConsentService) TouchLastUsed(ctx context.Context, userID int64, clientID string) error {
	if s == nil || s.client == nil {
		return fmt.Errorf("oidc consent: nil client")
	}
	n, err := s.client.OidcConsent.Update().
		Where(
			oidcconsent.UserIDEQ(userID),
			oidcconsent.ClientIDEQ(clientID),
		).
		SetLastUsedAt(s.now()).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("oidc consent: touch: %w", err)
	}
	if n == 0 {
		return ErrOidcConsentNotFound
	}
	return nil
}

// IsCovered 判断 granted 是否是 requested 的 superset (即 requested 完全被 granted 覆盖)。
//
// 空 requested 视为已覆盖 (true)。空 granted + 非空 requested 返回 false。
// 比较忽略顺序，对重复元素鲁棒。
func (s *OidcConsentService) IsCovered(granted, requested []string) bool {
	if len(requested) == 0 {
		return true
	}
	set := make(map[string]struct{}, len(granted))
	for _, g := range granted {
		set[g] = struct{}{}
	}
	for _, r := range requested {
		if _, ok := set[r]; !ok {
			return false
		}
	}
	return true
}

// ─── 错误哨兵 ────────────────────────────────────────────────────────────────

// ErrOidcConsentNotFound Revoke / TouchLastUsed 操作目标不存在。
var ErrOidcConsentNotFound = errors.New("oidc consent: not found")

// ─── 内部工具 ────────────────────────────────────────────────────────────────
//
// 注：去重函数 dedupeStrings 复用本包内 openai_images.go 中已有的同名实现。

// unionStrings 返回 a ∪ b 的去重切片。a 中元素优先保序。
func unionStrings(a, b []string) []string {
	seen := make(map[string]struct{}, len(a)+len(b))
	out := make([]string, 0, len(a)+len(b))
	for _, x := range a {
		if _, ok := seen[x]; ok {
			continue
		}
		seen[x] = struct{}{}
		out = append(out, x)
	}
	for _, x := range b {
		if _, ok := seen[x]; ok {
			continue
		}
		seen[x] = struct{}{}
		out = append(out, x)
	}
	return out
}
