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
	"fmt"
	"net/http"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/sso_session"
	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

var (
	ErrSSOSessionNotFound = infraerrors.Unauthorized("SSO_SESSION_NOT_FOUND", "SSO session not found or expired")
	ErrSSOSessionRevoked  = infraerrors.Unauthorized("SSO_SESSION_REVOKED", "SSO session has been revoked")
	ErrSSOSessionExpired = infraerrors.Unauthorized("SSO_SESSION_EXPIRED", "SSO session has expired")
)

// SSOSessionService 处理 HttpOnly SSO Cookie 的签发、验证和撤销
// 仅供 /oidc/authorize 浏览器跳转识别登录态使用，与前端 localStorage JWT 完全独立
type SSOSessionService struct {
	entClient *dbent.Client
	cfg       *config.Config
}

// NewSSOSessionService 创建 SSO Session Service 实例
func NewSSOSessionService(entClient *dbent.Client, cfg *config.Config) *SSOSessionService {
	return &SSOSessionService{
		entClient: entClient,
		cfg:       cfg,
	}
}

// Issue 签发新的 SSO Session Cookie
// 在所有用户登录成功的路径中调用此方法
func (s *SSOSessionService) Issue(ctx context.Context, userID int64, w http.ResponseWriter, r *http.Request) error {
	// 生成随机的 session ID
	sessionID, err := s.generateSessionID()
	if err != nil {
		return fmt.Errorf("generate session ID: %w", err)
	}

	now := time.Now().UTC()
	expiresAt := now.Add(time.Duration(s.cfg.SSO.SessionTTLDays) * 24 * time.Hour)

	// 创建 SSO Session 记录
	session, err := s.entClient.SsoSession.Create().
		SetSessionID(sessionID).
		SetUserID(userID).
		SetIssuedAt(now).
		SetLastSeenAt(now).
		SetExpiresAt(expiresAt).
		SetUserAgent(r.UserAgent()).
		SetIPAddress(getClientIP(r)).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("create SSO session: %w", err)
	}

	// 设置 HttpOnly Cookie
	s.setSSOCookie(w, sessionID, expiresAt)

	logger.LegacyPrintf("service.sso_session", "[SSO] Session issued for user %d: %s", userID, sessionID)
	return nil
}

// Validate 验证 SSO Session Cookie
// 在 /oidc/authorize 端点中调用此方法验证用户登录态
func (s *SSOSessionService) Validate(ctx context.Context, r *http.Request) (int64, error) {
	// 从 Cookie 中获取 session ID
	sessionID, err := s.getSSOCookie(r)
	if err != nil {
		return 0, ErrSSOSessionNotFound
	}

	// 查询 SSO Session
	session, err := s.entClient.SsoSession.Query().
		Where(sso_session.SessionID(sessionID)).
		First(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return 0, ErrSSOSessionNotFound
		}
		return 0, fmt.Errorf("query SSO session: %w", err)
	}

	// 检查 session 状态
	now := time.Now().UTC()
	if session.RevokedAt != nil && !session.RevokedAt.IsZero() {
		return 0, ErrSSOSessionRevoked
	}

	if now.After(session.ExpiresAt) {
		return 0, ErrSSOSessionExpired
	}

	// 异步更新 last_seen_at（不阻塞响应）
	go s.touchLastSeen(ctx, session.ID)

	return session.UserID, nil
}

// Revoke 撤销 SSO Session
// 在用户登出时调用此方法
func (s *SSOSessionService) Revoke(ctx context.Context, sessionID string, w http.ResponseWriter) error {
	// 标记 session 为已撤销
	now := time.Now().UTC()
	_, err := s.entClient.SsoSession.Update().
		Where(sso_session.SessionID(sessionID)).
		SetRevokedAt(now).
		Save(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return ErrSSOSessionNotFound
		}
		return fmt.Errorf("revoke SSO session: %w", err)
	}

	// 清除 Cookie
	s.clearSSOCookie(w)

	logger.LegacyPrintf("service.sso_session", "[SSO] Session revoked: %s", sessionID)
	return nil
}

// RevokeAllUserSessions 撤销用户的所有 SSO Session
// 在密码更改或用户主动登出所有设备时调用
func (s *SSOSessionService) RevokeAllUserSessions(ctx context.Context, userID int64) error {
	now := time.Now().UTC()
	_, err := s.entClient.SsoSession.Update().
		Where(sso_session.UserID(userID)).
		SetRevokedAt(now).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("revoke all user SSO sessions: %w", err)
	}

	logger.LegacyPrintf("service.sso_session", "[SSO] All sessions revoked for user %d", userID)
	return nil
}

// generateSessionID 生成随机的 session ID
func (s *SSOSessionService) generateSessionID() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generate random bytes: %w", err)
	}
	return base64.URLEncoding.EncodeToString(bytes), nil
}

// setSSOCookie 设置 HttpOnly SSO Cookie
func (s *SSOSessionService) setSSOCookie(w http.ResponseWriter, sessionID string, expiresAt time.Time) {
	cookie := &http.Cookie{
		Name:     s.cfg.SSO.CookieName,
		Value:    sessionID,
		Path:     "/",
		Domain:   s.cfg.SSO.CookieDomain,
		Expires:  expiresAt,
		HttpOnly: true,
		Secure:   s.cfg.SSO.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	}
	http.SetCookie(w, cookie)
}

// getSSOCookie 从请求中获取 SSO Cookie
func (s *SSOSessionService) getSSOCookie(r *http.Request) (string, error) {
	cookie, err := r.Cookie(s.cfg.SSO.CookieName)
	if err != nil {
		return "", err
	}
	return cookie.Value, nil
}

// clearSSOCookie 清除 SSO Cookie
func (s *SSOSessionService) clearSSOCookie(w http.ResponseWriter) {
	cookie := &http.Cookie{
		Name:     s.cfg.SSO.CookieName,
		Value:    "",
		Path:     "/",
		Domain:   s.cfg.SSO.CookieDomain,
		Expires:  time.Unix(0, 0),
		HttpOnly: true,
		Secure:   s.cfg.SSO.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	}
	http.SetCookie(w, cookie)
}

// touchLastSeen 异步更新 last_seen_at
func (s *SSOSessionService) touchLastSeen(ctx context.Context, sessionID int64) {
	now := time.Now().UTC()
	_, err := s.entClient.SsoSession.UpdateOneID(sessionID).
		SetLastSeenAt(now).
		Save(ctx)
	if err != nil {
		logger.LegacyPrintf("service.sso_session", "[SSO] Failed to update last_seen_at for session %d: %v", sessionID, err)
	}
}

// getClientIP 获取客户端 IP 地址
func getClientIP(r *http.Request) string {
	// 优先从 X-Forwarded-For 获取真实 IP
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		return forwarded
	}
	return r.RemoteAddr
}
