package service

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	// InnerAPIAppIDPrefix 内部 API app 业务主键前缀。
	InnerAPIAppIDPrefix = "iapp_"
	// innerAPIAppIDRandBytes 16B 随机 → base32 无填充 26 字符。
	innerAPIAppIDRandBytes = 16
	// innerAPIAppNameMaxLen app_name 最大长度（与列定义一致）。
	innerAPIAppNameMaxLen = 100
	// innerAPIAppCacheTTL 本地 app 状态缓存 TTL：热路径命中缓存，避免每请求查 DB。
	innerAPIAppCacheTTL = 5 * time.Second
)

// ErrInnerAPIAppInvalidName 创建接入方时名称非法。
var ErrInnerAPIAppInvalidName = infraerrors.BadRequest("INNER_API_APP_INVALID_NAME", "inner api app name is invalid")

// InnerAPIAppService 管理内部 API app 的创建/授权/启停/鉴权。
//
// 鉴权无状态：token = AES-256-GCM(本地密钥, {app_id})，解密成功即证明 token 由本方签发。
// 解密后再查 app 的启停状态——app 信息走进程内本地缓存（短 TTL + 启停时主动失效），
// 因此热路径不必每请求查 DB。
type InnerAPIAppService struct {
	repo  InnerAPIAppRepository
	codec *InnerAPITokenCodec

	cacheTTL     time.Duration
	now          func() time.Time
	randReadFunc func([]byte) (int, error)

	mu    sync.RWMutex
	cache map[string]innerAPIAppCacheEntry
}

type innerAPIAppCacheEntry struct {
	app     *InnerAPIApp
	expires time.Time
}

// NewInnerAPIAppService 构造服务。
func NewInnerAPIAppService(repo InnerAPIAppRepository, codec *InnerAPITokenCodec) *InnerAPIAppService {
	return &InnerAPIAppService{
		repo:         repo,
		codec:        codec,
		cacheTTL:     innerAPIAppCacheTTL,
		now:          time.Now,
		randReadFunc: rand.Read,
		cache:        make(map[string]innerAPIAppCacheEntry),
	}
}

// CreateApp 创建接入方，返回视图 + 一次性 token（之后无法再取）。
func (s *InnerAPIAppService) CreateApp(ctx context.Context, name string, permissions []string) (*InnerAPIApp, string, error) {
	if s == nil || s.repo == nil {
		return nil, "", fmt.Errorf("inner api app: nil repo")
	}
	name = strings.TrimSpace(name)
	if name == "" || len(name) > innerAPIAppNameMaxLen {
		return nil, "", ErrInnerAPIAppInvalidName
	}
	permissions, err := ValidateInnerAPIPermissions(permissions)
	if err != nil {
		return nil, "", err
	}
	// 密钥未配置则不创建（否则会留下无法签发 token 的 app）。
	if s.codec == nil || !s.codec.Configured() {
		return nil, "", ErrInnerAPIAppTokenNotConfigured
	}

	appID, err := s.generateAppID()
	if err != nil {
		return nil, "", err
	}
	created, err := s.repo.Create(ctx, &InnerAPIApp{
		AppID:        appID,
		AppName:      name,
		Enabled:      true,
		TokenVersion: 1,
		Permissions:  permissions,
	})
	if err != nil {
		return nil, "", err
	}
	token, err := s.codec.Mint(created.AppID, created.TokenVersion)
	if err != nil {
		return nil, "", err
	}
	s.cacheStore(created)
	return created, token, nil
}

// SetPermissions 更新接入方权限，并立即刷新本地鉴权缓存。
func (s *InnerAPIAppService) SetPermissions(ctx context.Context, appID string, permissions []string) error {
	permissions, err := ValidateInnerAPIPermissions(permissions)
	if err != nil {
		return err
	}
	if s == nil || s.repo == nil {
		return fmt.Errorf("inner api app: nil repo")
	}
	appID = strings.TrimSpace(appID)
	if err := s.repo.SetPermissions(ctx, appID, permissions); err != nil {
		return err
	}
	s.cacheDelete(appID)
	return nil
}

// RefreshToken 自增 token 版本（旧 token 立即失效）并返回新 token。
func (s *InnerAPIAppService) RefreshToken(ctx context.Context, appID string) (string, error) {
	if s == nil || s.repo == nil {
		return "", fmt.Errorf("inner api app: nil repo")
	}
	if s.codec == nil || !s.codec.Configured() {
		return "", ErrInnerAPIAppTokenNotConfigured
	}
	appID = strings.TrimSpace(appID)
	newVersion, err := s.repo.BumpTokenVersion(ctx, appID)
	if err != nil {
		return "", err
	}
	s.cacheDelete(appID)
	return s.codec.Mint(appID, newVersion)
}

// DeleteApp 删除接入方并失效本地缓存（其历史流水仍保留做审计）。
func (s *InnerAPIAppService) DeleteApp(ctx context.Context, appID string) error {
	if s == nil || s.repo == nil {
		return fmt.Errorf("inner api app: nil repo")
	}
	appID = strings.TrimSpace(appID)
	if err := s.repo.Delete(ctx, appID); err != nil {
		return err
	}
	s.cacheDelete(appID)
	return nil
}

// SetEnabled 启用/停用接入方，并失效本地缓存使其立即生效。
func (s *InnerAPIAppService) SetEnabled(ctx context.Context, appID string, enabled bool) error {
	if s == nil || s.repo == nil {
		return fmt.Errorf("inner api app: nil repo")
	}
	appID = strings.TrimSpace(appID)
	if err := s.repo.SetEnabled(ctx, appID, enabled); err != nil {
		return err
	}
	s.cacheDelete(appID)
	return nil
}

// List 列出所有接入方。
func (s *InnerAPIAppService) List(ctx context.Context) ([]*InnerAPIApp, error) {
	if s == nil || s.repo == nil {
		return nil, fmt.Errorf("inner api app: nil repo")
	}
	return s.repo.List(ctx)
}

// Authenticate 校验 token：解密成功 + 版本最新 + app 存在且未停用。
// 解密失败 / 版本过期（已被刷新）/ app 不存在 / 已停用统一返回 ErrInnerAPIAppUnauthenticated；
// 密钥未配置返回 ErrInnerAPIAppTokenNotConfigured（配置错误，非调用方问题）。
func (s *InnerAPIAppService) Authenticate(ctx context.Context, token string) (*InnerAPIApp, error) {
	if s == nil || s.repo == nil || s.codec == nil {
		return nil, fmt.Errorf("inner api app: nil deps")
	}
	appID, version, err := s.codec.Parse(token)
	if err != nil {
		if errors.Is(err, ErrInnerAPIAppTokenNotConfigured) {
			return nil, err
		}
		return nil, ErrInnerAPIAppUnauthenticated
	}

	app, err := s.lookupApp(ctx, appID)
	if err != nil {
		if errors.Is(err, ErrInnerAPIAppNotFound) {
			return nil, ErrInnerAPIAppUnauthenticated
		}
		return nil, err
	}
	if !app.Enabled || version != app.TokenVersion {
		// 已停用，或 token 版本过期（被刷新作废）。
		return nil, ErrInnerAPIAppUnauthenticated
	}
	return app, nil
}

// lookupApp 优先本地缓存，未命中回源 DB 并缓存。
func (s *InnerAPIAppService) lookupApp(ctx context.Context, appID string) (*InnerAPIApp, error) {
	if app := s.cacheGet(appID); app != nil {
		return app, nil
	}
	app, err := s.repo.GetByAppID(ctx, appID)
	if err != nil {
		return nil, err
	}
	s.cacheStore(app)
	return app, nil
}

func (s *InnerAPIAppService) cacheGet(appID string) *InnerAPIApp {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entry, ok := s.cache[appID]
	if !ok || s.now().After(entry.expires) {
		return nil
	}
	return entry.app
}

func (s *InnerAPIAppService) cacheStore(app *InnerAPIApp) {
	if app == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	copyApp := *app
	copyApp.Permissions = append([]string(nil), app.Permissions...)
	s.cache[app.AppID] = innerAPIAppCacheEntry{app: &copyApp, expires: s.now().Add(s.cacheTTL)}
}

func (s *InnerAPIAppService) cacheDelete(appID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.cache, appID)
}

func (s *InnerAPIAppService) generateAppID() (string, error) {
	buf := make([]byte, innerAPIAppIDRandBytes)
	if _, err := s.randReadFunc(buf); err != nil {
		return "", fmt.Errorf("inner api app: rand app id: %w", err)
	}
	encoder := base32.StdEncoding.WithPadding(base32.NoPadding)
	return InnerAPIAppIDPrefix + strings.ToLower(encoder.EncodeToString(buf)), nil
}
