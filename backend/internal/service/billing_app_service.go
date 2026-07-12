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
	// BillingAppIDPrefix 扣费 app 业务主键前缀。
	BillingAppIDPrefix = "bapp_"
	// billingAppIDRandBytes 16B 随机 → base32 无填充 26 字符。
	billingAppIDRandBytes = 16
	// billingAppNameMaxLen app_name 最大长度（与列定义一致）。
	billingAppNameMaxLen = 100
	// billingAppCacheTTL 本地 app 状态缓存 TTL：热路径命中缓存，避免每请求查 DB。
	billingAppCacheTTL = 5 * time.Second
)

// ErrBillingAppInvalidName 创建接入方时名称非法。
var ErrBillingAppInvalidName = infraerrors.BadRequest("BILLING_APP_INVALID_NAME", "billing app name is invalid")

// BillingAppService 管理扣费 app 的创建/启停/鉴权。
//
// 鉴权无状态：token = AES-256-GCM(本地密钥, {app_id})，解密成功即证明 token 由本方签发。
// 解密后再查 app 的启停状态——app 信息走进程内本地缓存（短 TTL + 启停时主动失效），
// 因此热路径不必每请求查 DB。
type BillingAppService struct {
	repo  BillingAppRepository
	codec *BillingAppTokenCodec

	cacheTTL     time.Duration
	now          func() time.Time
	randReadFunc func([]byte) (int, error)

	mu    sync.RWMutex
	cache map[string]billingAppCacheEntry
}

type billingAppCacheEntry struct {
	app     *BillingApp
	expires time.Time
}

// NewBillingAppService 构造服务。
func NewBillingAppService(repo BillingAppRepository, codec *BillingAppTokenCodec) *BillingAppService {
	return &BillingAppService{
		repo:         repo,
		codec:        codec,
		cacheTTL:     billingAppCacheTTL,
		now:          time.Now,
		randReadFunc: rand.Read,
		cache:        make(map[string]billingAppCacheEntry),
	}
}

// CreateApp 创建接入方，返回视图 + 一次性 token（之后无法再取）。
func (s *BillingAppService) CreateApp(ctx context.Context, name string) (*BillingApp, string, error) {
	if s == nil || s.repo == nil {
		return nil, "", fmt.Errorf("billing app: nil repo")
	}
	name = strings.TrimSpace(name)
	if name == "" || len(name) > billingAppNameMaxLen {
		return nil, "", ErrBillingAppInvalidName
	}
	// 密钥未配置则不创建（否则会留下无法签发 token 的 app）。
	if !s.codec.Configured() {
		return nil, "", ErrBillingAppTokenNotConfigured
	}

	appID, err := s.generateAppID()
	if err != nil {
		return nil, "", err
	}
	created, err := s.repo.Create(ctx, &BillingApp{
		AppID:        appID,
		AppName:      name,
		Enabled:      true,
		TokenVersion: 1,
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

// RefreshToken 自增 token 版本（旧 token 立即失效）并返回新 token。
func (s *BillingAppService) RefreshToken(ctx context.Context, appID string) (string, error) {
	if s == nil || s.repo == nil {
		return "", fmt.Errorf("billing app: nil repo")
	}
	if !s.codec.Configured() {
		return "", ErrBillingAppTokenNotConfigured
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
func (s *BillingAppService) DeleteApp(ctx context.Context, appID string) error {
	if s == nil || s.repo == nil {
		return fmt.Errorf("billing app: nil repo")
	}
	appID = strings.TrimSpace(appID)
	if err := s.repo.Delete(ctx, appID); err != nil {
		return err
	}
	s.cacheDelete(appID)
	return nil
}

// SetEnabled 启用/停用接入方，并失效本地缓存使其立即生效。
func (s *BillingAppService) SetEnabled(ctx context.Context, appID string, enabled bool) error {
	if s == nil || s.repo == nil {
		return fmt.Errorf("billing app: nil repo")
	}
	appID = strings.TrimSpace(appID)
	if err := s.repo.SetEnabled(ctx, appID, enabled); err != nil {
		return err
	}
	s.cacheDelete(appID)
	return nil
}

// List 列出所有接入方。
func (s *BillingAppService) List(ctx context.Context) ([]*BillingApp, error) {
	if s == nil || s.repo == nil {
		return nil, fmt.Errorf("billing app: nil repo")
	}
	return s.repo.List(ctx)
}

// Authenticate 校验 token：解密成功 + 版本最新 + app 存在且未停用。
// 解密失败 / 版本过期（已被刷新）/ app 不存在 / 已停用统一返回 ErrBillingAppUnauthenticated；
// 密钥未配置返回 ErrBillingAppTokenNotConfigured（配置错误，非调用方问题）。
func (s *BillingAppService) Authenticate(ctx context.Context, token string) (*BillingApp, error) {
	if s == nil || s.repo == nil || s.codec == nil {
		return nil, fmt.Errorf("billing app: nil deps")
	}
	appID, version, err := s.codec.Parse(token)
	if err != nil {
		if errors.Is(err, ErrBillingAppTokenNotConfigured) {
			return nil, err
		}
		return nil, ErrBillingAppUnauthenticated
	}

	app, err := s.lookupApp(ctx, appID)
	if err != nil {
		if errors.Is(err, ErrBillingAppNotFound) {
			return nil, ErrBillingAppUnauthenticated
		}
		return nil, err
	}
	if !app.Enabled || version != app.TokenVersion {
		// 已停用，或 token 版本过期（被刷新作废）。
		return nil, ErrBillingAppUnauthenticated
	}
	return app, nil
}

// lookupApp 优先本地缓存，未命中回源 DB 并缓存。
func (s *BillingAppService) lookupApp(ctx context.Context, appID string) (*BillingApp, error) {
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

func (s *BillingAppService) cacheGet(appID string) *BillingApp {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entry, ok := s.cache[appID]
	if !ok || s.now().After(entry.expires) {
		return nil
	}
	return entry.app
}

func (s *BillingAppService) cacheStore(app *BillingApp) {
	if app == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cache[app.AppID] = billingAppCacheEntry{app: app, expires: s.now().Add(s.cacheTTL)}
}

func (s *BillingAppService) cacheDelete(appID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.cache, appID)
}

func (s *BillingAppService) generateAppID() (string, error) {
	buf := make([]byte, billingAppIDRandBytes)
	if _, err := s.randReadFunc(buf); err != nil {
		return "", fmt.Errorf("billing app: rand app id: %w", err)
	}
	encoder := base32.StdEncoding.WithPadding(base32.NoPadding)
	return BillingAppIDPrefix + strings.ToLower(encoder.EncodeToString(buf)), nil
}
