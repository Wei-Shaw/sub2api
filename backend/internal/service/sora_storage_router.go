package service

import (
	"context"
	"fmt"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

// SoraStorageRouter 根据激活 profile 的 provider 字段路由到对应存储实现。
// 实现 SoraObjectStorage 接口。
type SoraStorageRouter struct {
	settingService *SettingService
	s3Storage      *SoraS3Storage
	gdriveStorage  SoraObjectStorage // 可为 nil（GDrive 未实现时）
}

// NewSoraStorageRouter 创建存储路由。
func NewSoraStorageRouter(
	settingService *SettingService,
	s3Storage *SoraS3Storage,
	gdriveStorage SoraObjectStorage,
) *SoraStorageRouter {
	return &SoraStorageRouter{
		settingService: settingService,
		s3Storage:      s3Storage,
		gdriveStorage:  gdriveStorage,
	}
}

// activeBackend 返回当前激活 profile 对应的存储后端。
func (r *SoraStorageRouter) activeBackend(ctx context.Context) SoraObjectStorage {
	if r.settingService == nil {
		return r.s3Storage // 默认 S3
	}

	profile, err := r.settingService.GetActiveStorageProfile(ctx)
	if err != nil || profile == nil {
		return r.s3Storage // 默认 S3
	}

	switch profile.GetProvider() {
	case SoraStorageTypeGDrive:
		if r.gdriveStorage != nil {
			return r.gdriveStorage
		}
		logger.LegacyPrintf("service.storage_router", "[StorageRouter] GDrive 后端未初始化，降级到 S3")
		return r.s3Storage
	default:
		return r.s3Storage
	}
}

func (r *SoraStorageRouter) Enabled(ctx context.Context) bool {
	backend := r.activeBackend(ctx)
	if backend == nil {
		return false
	}
	return backend.Enabled(ctx)
}

func (r *SoraStorageRouter) IsHealthy(ctx context.Context) bool {
	backend := r.activeBackend(ctx)
	if backend == nil {
		return false
	}
	return backend.IsHealthy(ctx)
}

func (r *SoraStorageRouter) TestConnection(ctx context.Context) error {
	backend := r.activeBackend(ctx)
	if backend == nil {
		return fmt.Errorf("no storage backend available")
	}
	return backend.TestConnection(ctx)
}

func (r *SoraStorageRouter) UploadFromURL(ctx context.Context, userID int64, sourceURL string) (string, int64, string, error) {
	backend := r.activeBackend(ctx)
	if backend == nil {
		return "", 0, "", fmt.Errorf("no storage backend available")
	}
	return backend.UploadFromURL(ctx, userID, sourceURL)
}

func (r *SoraStorageRouter) DeleteObjects(ctx context.Context, objectKeys []string) error {
	backend := r.activeBackend(ctx)
	if backend == nil {
		return fmt.Errorf("no storage backend available")
	}
	return backend.DeleteObjects(ctx, objectKeys)
}

func (r *SoraStorageRouter) GetAccessURL(ctx context.Context, objectKey string) (string, error) {
	backend := r.activeBackend(ctx)
	if backend == nil {
		return "", fmt.Errorf("no storage backend available")
	}
	return backend.GetAccessURL(ctx, objectKey)
}

func (r *SoraStorageRouter) RefreshClient() {
	if r.s3Storage != nil {
		r.s3Storage.RefreshClient()
	}
	if r.gdriveStorage != nil {
		r.gdriveStorage.RefreshClient()
	}
}

// RefreshAll 刷新所有后端客户端（用作配置变更回调）。
func (r *SoraStorageRouter) RefreshAll() {
	r.RefreshClient()
}

func (r *SoraStorageRouter) StorageType() string {
	// 不带 context 的方法，返回默认值
	// 真实的 StorageType 在 activeBackend 中动态确定
	return SoraStorageTypeS3
}

// StorageTypeWithContext 返回当前激活后端的存储类型。
func (r *SoraStorageRouter) StorageTypeWithContext(ctx context.Context) string {
	backend := r.activeBackend(ctx)
	if backend == nil {
		return SoraStorageTypeS3
	}
	return backend.StorageType()
}
