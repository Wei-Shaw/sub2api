package service

import "context"

// SoraObjectStorage 是 Sora 媒体文件的通用对象存储接口。
// S3 和 Google Drive 等存储后端均实现此接口。
type SoraObjectStorage interface {
	// Enabled 返回存储是否已启用且配置有效。
	Enabled(ctx context.Context) bool

	// IsHealthy 返回存储健康状态（带短缓存）。
	IsHealthy(ctx context.Context) bool

	// TestConnection 测试存储连接。
	TestConnection(ctx context.Context) error

	// UploadFromURL 从上游 URL 下载并上传到存储。
	// 返回 object key（S3 key 或 GDrive file ID）、文件大小、实际使用的存储类型。
	UploadFromURL(ctx context.Context, userID int64, sourceURL string) (objectKey string, sizeBytes int64, storageType string, err error)

	// DeleteObjects 删除一组存储对象。
	DeleteObjects(ctx context.Context, objectKeys []string) error

	// GetAccessURL 获取存储文件的访问 URL。
	GetAccessURL(ctx context.Context, objectKey string) (string, error)

	// RefreshClient 清除缓存客户端，配置变更时调用。
	RefreshClient()

	// StorageType 返回存储类型标识（"s3" / "gdrive"）。
	StorageType() string
}

// IsObjectStorageType 判断是否为对象存储类型（S3 或 Google Drive）。
func IsObjectStorageType(t string) bool {
	return t == SoraStorageTypeS3 || t == SoraStorageTypeGDrive
}
