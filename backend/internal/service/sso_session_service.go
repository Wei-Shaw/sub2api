// Package service ...
//
// sso_session_service.go 实现 sub2api 作为 OIDC Provider 时的浏览器会话载体。
// 与前端 localStorage JWT (走 cfg.JWT) 完全独立：
//
//   - 前端 SPA 自身鉴权：localStorage JWT, Authorization: Bearer
//   - OIDC /oidc/authorize 浏览器跳转识别登录态：本服务管理的 sub2api_sso HttpOnly cookie
//
// 两套并存的原因见 design.md D6：HttpOnly cookie 是 OIDC 浏览器流程唯一可靠的"已登录"
// 信号；与 SPA 的 localStorage 同时存在没有耦合，因为 sub2api 的 v1 API 仍然只看 JWT。
//
// 本阶段 (Stage 1B-1) 只实现 service 层 + 单测。**不**修改任何登录 handler；接入由
// Stage 1B-2 完成。
package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/ssosession"
)

// ─── 常量 ────────────────────────────────────────────────────────────────────

const (
	// SsoCookieName HttpOnly SSO cookie 名称。
	SsoCookieName = "sub2api_sso"

	// ssoSessionIDByteLen 32B 随机字节 → 43 个 base64url 字符。
	ssoSessionIDByteLen = 32

	// SsoTouchLastSeenMinInterval TouchLastSeen 的最小写入间隔，避免 /oidc/authorize
	// 高频访问压垮 DB；与 design.md "Risks / Trade-offs" 的限流约束 (60s) 对齐。
	SsoTouchLastSeenMinInterval = 60 * time.Second
)

// ─── 错误哨兵 ────────────────────────────────────────────────────────────────

// ErrSsoSessionNotFound Resolve / Revoke / TouchLastSeen 操作目标不存在。
var ErrSsoSessionNotFound = errors.New("sso session: not found")

// ErrSsoSessionExpired Resolve 时会话已过 expires_at。
var ErrSsoSessionExpired = errors.New("sso session: expired")

// ErrSsoSessionRevoked Resolve 时会话已被吊销 (revoked_at != nil)。
var ErrSsoSessionRevoked = errors.New("sso session: revoked")

// ─── 数据结构 ────────────────────────────────────────────────────────────────

// SsoSessionInfo Resolve 命中后返回给调用方的精简结构。
type SsoSessionInfo struct {
	SessionID      string
	UserID         int64
	IssuedAt       time.Time
	LastSeenAt     time.Time
	ExpiresAt      time.Time
	TotpVerifiedAt *time.Time
	UserAgent      string
	IPAddress      string
}

// ssoCookieConfig 当前生效的 cookie 配置快照 (从 setting repo 读取)。
type ssoCookieConfig struct {
	maxAgeSeconds int
	domain        string
}

// ─── 服务体 ──────────────────────────────────────────────────────────────────

// SsoSessionService 管理 sub2api_sso cookie 的签发、解析、吊销。
//
// 线程安全：所有 Issue / Resolve / Revoke / TouchLastSeen 都是无锁纯 ent 调用；
// touchCache 用于限流 TouchLastSeen 写入，由内部 mutex 保护。
type SsoSessionService struct {
	client      *ent.Client
	settingRepo SettingRepository

	mu         sync.Mutex
	touchCache map[string]time.Time // sessionID → 最近一次 TouchLastSeen 写入时间

	// 注入点 (测试覆写)
	now           func() time.Time
	randReadFunc  func([]byte) (int, error)
	touchInterval time.Duration
}

// NewSsoSessionService 构造服务。
//
// settingRepo 用于读取 oidc_provider.sso_cookie_max_age_seconds /
// oidc_provider.sso_cookie_domain。client 与 settingRepo 都必须非 nil。
func NewSsoSessionService(client *ent.Client, settingRepo SettingRepository) *SsoSessionService {
	return &SsoSessionService{
		client:        client,
		settingRepo:   settingRepo,
		touchCache:    make(map[string]time.Time),
		now:           func() time.Time { return time.Now().UTC() },
		randReadFunc:  rand.Read,
		touchInterval: SsoTouchLastSeenMinInterval,
	}
}

// ─── Cookie 签发 ─────────────────────────────────────────────────────────────

// Issue 生成新的 sso_session 行 + 写出 Set-Cookie 响应头。
//
// 行为：
//
//  1. 生成 32B base64url 随机 session_id
//  2. 从 setting repo 读取 cookie 配置 (max-age / domain)，取不到则使用默认值
//  3. 写入 sso_sessions 行 (issued_at = now, last_seen_at = now,
//     expires_at = now + max_age, user_agent / ip 从 r 提取)
//  4. 调用 http.SetCookie 写出 sub2api_sso=<session_id>; Path=/;
//     HttpOnly; Secure; SameSite=Lax; Max-Age=<seconds>; [Domain=<domain>]
//
// 调用方负责：在所有"成功登录"的路径之后调用本方法 (登录 handler / OAuth 完成 handler /
// 注册自动登录 handler)。本方法不写 JWT、不动 localStorage。
//
// 返回 sessionID 便于审计日志 / 测试断言。
func (s *SsoSessionService) Issue(ctx context.Context, w http.ResponseWriter, r *http.Request, userID int64) (string, error) {
	if s == nil || s.client == nil {
		return "", fmt.Errorf("sso session: nil client")
	}
	if userID <= 0 {
		return "", fmt.Errorf("sso session: invalid user id %d", userID)
	}

	cfg, err := s.loadCookieConfig(ctx)
	if err != nil {
		return "", err
	}

	sessionID, err := s.generateSessionID()
	if err != nil {
		return "", err
	}

	now := s.now()
	expiresAt := now.Add(time.Duration(cfg.maxAgeSeconds) * time.Second)

	ua, ip := extractUserAgentAndIP(r)

	if _, err := s.client.SsoSession.Create().
		SetSessionID(sessionID).
		SetUserID(userID).
		SetIssuedAt(now).
		SetLastSeenAt(now).
		SetExpiresAt(expiresAt).
		SetUserAgent(ua).
		SetIPAddress(ip).
		Save(ctx); err != nil {
		return "", fmt.Errorf("sso session: create row: %w", err)
	}

	http.SetCookie(w, &http.Cookie{
		Name:     SsoCookieName,
		Value:    sessionID,
		Path:     "/",
		Domain:   cfg.domain,
		MaxAge:   cfg.maxAgeSeconds,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})

	return sessionID, nil
}

// ─── Cookie 解析 ─────────────────────────────────────────────────────────────

// Resolve 从 r.Cookies() 中解析 sub2api_sso，校验 DB 状态并返回会话信息。
//
// 错误语义：
//
//   - 无 cookie / cookie 空 → ErrSsoSessionNotFound (调用方应跳转登录)
//   - cookie 在 DB 中找不到 → ErrSsoSessionNotFound
//   - revoked_at != nil → ErrSsoSessionRevoked
//   - now > expires_at → ErrSsoSessionExpired
//   - 其他 DB 错误 → wrap
func (s *SsoSessionService) Resolve(ctx context.Context, r *http.Request) (*SsoSessionInfo, error) {
	if s == nil || s.client == nil {
		return nil, fmt.Errorf("sso session: nil client")
	}
	if r == nil {
		return nil, ErrSsoSessionNotFound
	}
	c, err := r.Cookie(SsoCookieName)
	if err != nil || c == nil || strings.TrimSpace(c.Value) == "" {
		return nil, ErrSsoSessionNotFound
	}
	return s.resolveSessionID(ctx, c.Value)
}

// ResolveSessionID 用于不便从 *http.Request 取 cookie 的场景 (e.g. 中间件拼装好的
// session id 字符串)；行为与 Resolve 一致。
func (s *SsoSessionService) ResolveSessionID(ctx context.Context, sessionID string) (*SsoSessionInfo, error) {
	if s == nil || s.client == nil {
		return nil, fmt.Errorf("sso session: nil client")
	}
	return s.resolveSessionID(ctx, sessionID)
}

func (s *SsoSessionService) resolveSessionID(ctx context.Context, sessionID string) (*SsoSessionInfo, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, ErrSsoSessionNotFound
	}
	row, err := s.client.SsoSession.Query().
		Where(ssosession.SessionIDEQ(sessionID)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrSsoSessionNotFound
		}
		return nil, fmt.Errorf("sso session: lookup: %w", err)
	}
	if row.RevokedAt != nil {
		return nil, ErrSsoSessionRevoked
	}
	now := s.now()
	if !row.ExpiresAt.IsZero() && now.After(row.ExpiresAt) {
		return nil, ErrSsoSessionExpired
	}
	return &SsoSessionInfo{
		SessionID:      row.SessionID,
		UserID:         row.UserID,
		IssuedAt:       row.IssuedAt,
		LastSeenAt:     row.LastSeenAt,
		ExpiresAt:      row.ExpiresAt,
		TotpVerifiedAt: row.TotpVerifiedAt,
		UserAgent:      row.UserAgent,
		IPAddress:      row.IPAddress,
	}, nil
}

// ─── Cookie 吊销 ─────────────────────────────────────────────────────────────

// Revoke 把指定 session_id 标记为 revoked (写 revoked_at = now) 并写出
// 过期的 Set-Cookie 让浏览器立即清除。
//
//   - sessionID 不存在 → ErrSsoSessionNotFound (但仍会发出空 cookie 让浏览器清理)
//   - 已 revoked → 视为成功 (幂等)，仍写空 cookie
//
// w 可以是 nil (服务端单方面吊销，不需要响应清 cookie)；为 nil 时不写 cookie。
func (s *SsoSessionService) Revoke(ctx context.Context, w http.ResponseWriter, sessionID string) error {
	if s == nil || s.client == nil {
		return fmt.Errorf("sso session: nil client")
	}

	if w != nil {
		// 总是写出过期 cookie，无论 DB 行是否存在 — 让浏览器始终清掉本地状态
		http.SetCookie(w, &http.Cookie{
			Name:     SsoCookieName,
			Value:    "",
			Path:     "/",
			MaxAge:   -1,
			HttpOnly: true,
			Secure:   true,
			SameSite: http.SameSiteLaxMode,
		})
	}

	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return ErrSsoSessionNotFound
	}

	row, err := s.client.SsoSession.Query().
		Where(ssosession.SessionIDEQ(sessionID)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return ErrSsoSessionNotFound
		}
		return fmt.Errorf("sso session: lookup for revoke: %w", err)
	}

	now := s.now()
	if row.RevokedAt != nil {
		// 幂等：已吊销直接 OK
		s.forgetTouchCache(sessionID)
		return nil
	}

	if _, err := s.client.SsoSession.UpdateOneID(row.ID).
		SetRevokedAt(now).
		Save(ctx); err != nil {
		return fmt.Errorf("sso session: mark revoked: %w", err)
	}
	s.forgetTouchCache(sessionID)
	return nil
}

// RevokeAllForUser 把指定用户的所有未吊销会话全部标记为 revoked。
//
// 用途：admin 一键踢人；用户自助"在所有设备登出"。
// 已吊销的行不会被再次更新；本方法仅设置当前未吊销的行。
//
// 返回成功标记的行数。
func (s *SsoSessionService) RevokeAllForUser(ctx context.Context, userID int64) (int, error) {
	if s == nil || s.client == nil {
		return 0, fmt.Errorf("sso session: nil client")
	}
	if userID <= 0 {
		return 0, fmt.Errorf("sso session: invalid user id %d", userID)
	}

	n, err := s.client.SsoSession.Update().
		Where(
			ssosession.UserIDEQ(userID),
			ssosession.RevokedAtIsNil(),
		).
		SetRevokedAt(s.now()).
		Save(ctx)
	if err != nil {
		return 0, fmt.Errorf("sso session: revoke all: %w", err)
	}

	// 清掉 touch 缓存中该用户的所有 session — 简化为直接清空整张表
	// (RevokeAllForUser 极低频调用，全表清空成本可忽略)
	s.mu.Lock()
	s.touchCache = make(map[string]time.Time)
	s.mu.Unlock()
	return n, nil
}

// ─── LastSeen 更新 ───────────────────────────────────────────────────────────

// TouchLastSeen 异步更新 last_seen_at，带最小间隔限流。
//
// 调用方应在每次 /oidc/authorize 命中已登录 SSO 时调用本方法；本方法立刻返回，
// 实际 DB 写入由后台 goroutine 完成；写入之前会先检查内存里的 touchCache，若
// 距上次写入不到 [SsoTouchLastSeenMinInterval]，则跳过本次写入。
//
// 返回值仅用于测试可观测性 (是否真的派发了写入)；生产代码无需关心。
func (s *SsoSessionService) TouchLastSeen(sessionID string) bool {
	if s == nil || s.client == nil {
		return false
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return false
	}

	now := s.now()
	s.mu.Lock()
	last, ok := s.touchCache[sessionID]
	if ok && now.Sub(last) < s.touchInterval {
		s.mu.Unlock()
		return false
	}
	s.touchCache[sessionID] = now
	s.mu.Unlock()

	go s.applyTouchLastSeen(sessionID, now)
	return true
}

// TouchLastSeenSync 同步版本，主要用于单测。
//
// 行为：忽略限流，直接写一次 DB；返回是否实际更新到行 (false 表示 session 不存在)。
func (s *SsoSessionService) TouchLastSeenSync(ctx context.Context, sessionID string) (bool, error) {
	if s == nil || s.client == nil {
		return false, fmt.Errorf("sso session: nil client")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return false, nil
	}
	n, err := s.client.SsoSession.Update().
		Where(ssosession.SessionIDEQ(sessionID)).
		SetLastSeenAt(s.now()).
		Save(ctx)
	if err != nil {
		return false, fmt.Errorf("sso session: touch last seen: %w", err)
	}
	return n > 0, nil
}

func (s *SsoSessionService) applyTouchLastSeen(sessionID string, t time.Time) {
	// 后台写入用独立 background context (调用栈结束后请求 ctx 已 cancel)
	ctx := context.Background()
	if _, err := s.client.SsoSession.Update().
		Where(ssosession.SessionIDEQ(sessionID)).
		SetLastSeenAt(t).
		Save(ctx); err != nil {
		// 后台任务不要拉垮主流程；忽略 "database is closed" (典型于测试 cleanup 后的延迟写入)。
		// 上层 admin 服务会有定期巡检逻辑，本错误不重要。
		msg := err.Error()
		if strings.Contains(msg, "database is closed") || strings.Contains(msg, "sql: connection is already closed") {
			return
		}
		fmt.Printf("[sso_session] touch_last_seen background write failed: sid=%s err=%v\n", sessionID, err)
	}
}

func (s *SsoSessionService) forgetTouchCache(sessionID string) {
	s.mu.Lock()
	delete(s.touchCache, sessionID)
	s.mu.Unlock()
}

// ─── 辅助函数 ────────────────────────────────────────────────────────────────

// generateSessionID 生成 [ssoSessionIDByteLen] 字节随机 + base64url 编码无 padding。
func (s *SsoSessionService) generateSessionID() (string, error) {
	buf := make([]byte, ssoSessionIDByteLen)
	if _, err := s.randReadFunc(buf); err != nil {
		return "", fmt.Errorf("sso session: rand read: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// loadCookieConfig 从 settingRepo 读取 max-age 与 domain，缺省走默认值。
//
// 解析失败 (e.g. max_age 值不是合法整数) 直接退化为默认值并继续；不阻断登录。
func (s *SsoSessionService) loadCookieConfig(ctx context.Context) (ssoCookieConfig, error) {
	cfg := ssoCookieConfig{
		maxAgeSeconds: DefaultOidcProviderSSOCookieMaxAgeSeconds,
		domain:        DefaultOidcProviderSSOCookieDomain,
	}
	if s.settingRepo == nil {
		return cfg, nil
	}

	if v, err := s.settingRepo.GetValue(ctx, SettingKeyOidcProviderSSOCookieMaxAgeSeconds); err == nil {
		v = strings.TrimSpace(v)
		if v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				cfg.maxAgeSeconds = n
			}
		}
	} else if !errors.Is(err, ErrSettingNotFound) {
		return cfg, fmt.Errorf("sso session: read max age setting: %w", err)
	}

	if v, err := s.settingRepo.GetValue(ctx, SettingKeyOidcProviderSSOCookieDomain); err == nil {
		cfg.domain = strings.TrimSpace(v)
	} else if !errors.Is(err, ErrSettingNotFound) {
		return cfg, fmt.Errorf("sso session: read domain setting: %w", err)
	}

	return cfg, nil
}

// extractUserAgentAndIP 从 *http.Request 抓取审计字段；nil 安全。
//
// IP 优先取 X-Forwarded-For 第一段，其次 r.RemoteAddr。
func extractUserAgentAndIP(r *http.Request) (string, string) {
	if r == nil {
		return "", ""
	}
	ua := r.Header.Get("User-Agent")
	ip := ""
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// "ip1, ip2, ..." → ip1
		if comma := strings.Index(xff, ","); comma >= 0 {
			ip = strings.TrimSpace(xff[:comma])
		} else {
			ip = strings.TrimSpace(xff)
		}
	}
	if ip == "" {
		ip = r.RemoteAddr
		// 去掉 ":port" 后缀
		if colon := strings.LastIndex(ip, ":"); colon > 0 {
			// IPv6 形如 [::1]:1234 — 简化处理：去最后一个冒号后的 port
			if !strings.Contains(ip[:colon], ":") || strings.HasSuffix(ip[:colon], "]") {
				ip = ip[:colon]
			}
		}
		ip = strings.Trim(ip, "[]")
	}
	return ua, ip
}
