package service

import (
	"context"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	porttlsfp "github.com/Wei-Shaw/sub2api/internal/port/tlsfingerprint"
)

// Port type aliases keep existing service call sites compiling while ports live
// in internal/port/tlsfingerprint.
type TLSFingerprintProfileRepository = porttlsfp.ProfileRepository
type TLSFingerprintProfileCache = porttlsfp.ProfileCache

// Domain type alias for incremental migration of call sites that still import
// service-level names via model → domain move.
type TLSFingerprintProfile = domain.TLSFingerprintProfile

// TLSFingerprintProfileService TLS 指纹模板管理服务
type TLSFingerprintProfileService struct {
	repo  porttlsfp.ProfileRepository
	cache porttlsfp.ProfileCache

	// 本地 ID→Profile 映射缓存，用于 DoWithTLS 热路径快速查找
	localCache map[int64]*domain.TLSFingerprintProfile
	localMu    sync.RWMutex
}

// NewTLSFingerprintProfileService 创建 TLS 指纹模板服务
func NewTLSFingerprintProfileService(
	repo porttlsfp.ProfileRepository,
	cache porttlsfp.ProfileCache,
) *TLSFingerprintProfileService {
	svc := &TLSFingerprintProfileService{
		repo:       repo,
		cache:      cache,
		localCache: make(map[int64]*domain.TLSFingerprintProfile),
	}

	ctx := context.Background()
	if err := svc.reloadFromDB(ctx); err != nil {
		logger.LegacyPrintf("service.tls_fp_profile", "[TLSFPProfileService] Failed to load profiles from DB on startup: %v", err)
		if fallbackErr := svc.refreshLocalCache(ctx); fallbackErr != nil {
			logger.LegacyPrintf("service.tls_fp_profile", "[TLSFPProfileService] Failed to load profiles from cache fallback on startup: %v", fallbackErr)
		}
	}

	if cache != nil {
		cache.SubscribeUpdates(ctx, func() {
			if err := svc.refreshLocalCache(context.Background()); err != nil {
				logger.LegacyPrintf("service.tls_fp_profile", "[TLSFPProfileService] Failed to refresh cache on notification: %v", err)
			}
		})
	}

	return svc
}

// --- CRUD ---

// List 获取所有模板
func (s *TLSFingerprintProfileService) List(ctx context.Context) ([]*domain.TLSFingerprintProfile, error) {
	return s.repo.List(ctx)
}

// GetByID 根据 ID 获取模板
func (s *TLSFingerprintProfileService) GetByID(ctx context.Context, id int64) (*domain.TLSFingerprintProfile, error) {
	return s.repo.GetByID(ctx, id)
}

// Create 创建模板
func (s *TLSFingerprintProfileService) Create(ctx context.Context, profile *domain.TLSFingerprintProfile) (*domain.TLSFingerprintProfile, error) {
	if err := profile.Validate(); err != nil {
		return nil, err
	}

	created, err := s.repo.Create(ctx, profile)
	if err != nil {
		return nil, err
	}

	refreshCtx, cancel := s.newCacheRefreshContext()
	defer cancel()
	s.invalidateAndNotify(refreshCtx)

	return created, nil
}

// Update 更新模板
func (s *TLSFingerprintProfileService) Update(ctx context.Context, profile *domain.TLSFingerprintProfile) (*domain.TLSFingerprintProfile, error) {
	if err := profile.Validate(); err != nil {
		return nil, err
	}

	updated, err := s.repo.Update(ctx, profile)
	if err != nil {
		return nil, err
	}

	refreshCtx, cancel := s.newCacheRefreshContext()
	defer cancel()
	s.invalidateAndNotify(refreshCtx)

	return updated, nil
}

// Delete 删除模板
func (s *TLSFingerprintProfileService) Delete(ctx context.Context, id int64) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}

	refreshCtx, cancel := s.newCacheRefreshContext()
	defer cancel()
	s.invalidateAndNotify(refreshCtx)

	return nil
}

// --- 热路径：运行时 Profile 查找 ---

// GetProfileByID 根据 ID 从本地缓存获取 Profile（用于 DoWithTLS 热路径）
// 返回 nil 表示未找到，调用方应 fallback 到内置默认 Profile
func (s *TLSFingerprintProfileService) GetProfileByID(id int64) *tlsfingerprint.Profile {
	s.localMu.RLock()
	p, ok := s.localCache[id]
	s.localMu.RUnlock()

	if ok && p != nil {
		return p.ToTLSProfile()
	}
	return nil
}

// getRandomProfile 从本地缓存中随机选择一个 Profile
func (s *TLSFingerprintProfileService) getRandomProfile() *tlsfingerprint.Profile {
	s.localMu.RLock()
	defer s.localMu.RUnlock()

	if len(s.localCache) == 0 {
		return nil
	}

	// 收集所有 profile
	profiles := make([]*domain.TLSFingerprintProfile, 0, len(s.localCache))
	for _, p := range s.localCache {
		if p != nil {
			profiles = append(profiles, p)
		}
	}
	if len(profiles) == 0 {
		return nil
	}

	return profiles[rand.IntN(len(profiles))].ToTLSProfile()
}

// ResolveTLSProfile 根据 Account 的配置解析出运行时 TLS Profile
//
// 逻辑：
//  1. 未启用 TLS 指纹 → 返回 nil（不伪装）
//  2. 启用 + 绑定了 profile_id → 从缓存查找对应 profile
//  3. 启用 + 未绑定或找不到 → 返回空 Profile（使用代码内置默认值）
func (s *TLSFingerprintProfileService) ResolveTLSProfile(account *Account) *tlsfingerprint.Profile {
	if account == nil || !account.IsTLSFingerprintEnabled() {
		return nil
	}
	id := account.GetTLSFingerprintProfileID()
	if id > 0 {
		if p := s.GetProfileByID(id); p != nil {
			return p
		}
	}
	if id == -1 {
		// 随机选择一个 profile
		if p := s.getRandomProfile(); p != nil {
			return p
		}
	}
	// TLS 启用但无绑定 profile → 空 Profile → dialer 使用内置默认值
	return &tlsfingerprint.Profile{Name: "Built-in Default (Node.js 24.x)"}
}

// --- 缓存管理 ---

func (s *TLSFingerprintProfileService) refreshLocalCache(ctx context.Context) error {
	if s.cache != nil {
		if profiles, ok := s.cache.Get(ctx); ok {
			s.setLocalCache(profiles)
			return nil
		}
	}
	return s.reloadFromDB(ctx)
}

func (s *TLSFingerprintProfileService) reloadFromDB(ctx context.Context) error {
	profiles, err := s.repo.List(ctx)
	if err != nil {
		return err
	}

	if s.cache != nil {
		if err := s.cache.Set(ctx, profiles); err != nil {
			logger.LegacyPrintf("service.tls_fp_profile", "[TLSFPProfileService] Failed to set cache: %v", err)
		}
	}

	s.setLocalCache(profiles)
	return nil
}

func (s *TLSFingerprintProfileService) setLocalCache(profiles []*domain.TLSFingerprintProfile) {
	m := make(map[int64]*domain.TLSFingerprintProfile, len(profiles))
	for _, p := range profiles {
		m[p.ID] = p
	}

	s.localMu.Lock()
	s.localCache = m
	s.localMu.Unlock()
}

func (s *TLSFingerprintProfileService) newCacheRefreshContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 3*time.Second)
}

func (s *TLSFingerprintProfileService) invalidateAndNotify(ctx context.Context) {
	if s.cache != nil {
		if err := s.cache.Invalidate(ctx); err != nil {
			logger.LegacyPrintf("service.tls_fp_profile", "[TLSFPProfileService] Failed to invalidate cache: %v", err)
		}
	}

	if err := s.reloadFromDB(ctx); err != nil {
		logger.LegacyPrintf("service.tls_fp_profile", "[TLSFPProfileService] Failed to refresh local cache: %v", err)
		s.localMu.Lock()
		s.localCache = make(map[int64]*domain.TLSFingerprintProfile)
		s.localMu.Unlock()
	}

	if s.cache != nil {
		if err := s.cache.NotifyUpdate(ctx); err != nil {
			logger.LegacyPrintf("service.tls_fp_profile", "[TLSFPProfileService] Failed to notify cache update: %v", err)
		}
	}
}
