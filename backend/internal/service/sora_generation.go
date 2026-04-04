package service

import (
	"context"
	"time"
)

// SoraGeneration 代表一条 Sora 客户端生成记录。
type SoraGeneration struct {
	ID             int64      `json:"id"`
	UserID         int64      `json:"user_id"`
	APIKeyID       *int64     `json:"api_key_id,omitempty"`
	Model          string     `json:"model"`
	Prompt         string     `json:"prompt"`
	MediaType      string     `json:"media_type"` // video / image
	Status         string     `json:"status"`     // pending / generating / completed / failed / cancelled
	MediaURL       string     `json:"media_url"`  // 主媒体 URL（预签名或 CDN）
	MediaURLs      []string   `json:"media_urls"` // 多图时的 URL 数组
	FileSizeBytes  int64      `json:"file_size_bytes"`
	StorageType    string     `json:"storage_type"`   // s3 / local / upstream / none
	S3ObjectKeys   []string   `json:"s3_object_keys"` // S3 object key 数组
	UpstreamTaskID string     `json:"upstream_task_id"`
	ErrorMessage   string     `json:"error_message"`
	CreatedAt      time.Time  `json:"created_at"`
	CompletedAt    *time.Time `json:"completed_at,omitempty"`
}

// Sora 生成记录状态常量
const (
	SoraGenStatusPending    = "pending"
	SoraGenStatusGenerating = "generating"
	SoraGenStatusCompleted  = "completed"
	SoraGenStatusFailed     = "failed"
	SoraGenStatusCancelled  = "cancelled"
)

// Sora 存储类型常量
const (
	SoraStorageTypeS3       = "s3"
	SoraStorageTypeLocal    = "local"
	SoraStorageTypeUpstream = "upstream"
	SoraStorageTypeNone     = "none"
)

// SoraGenerationListParams 查询生成记录的参数。
type SoraGenerationListParams struct {
	UserID      int64
	Status      string // 可选筛选
	StorageType string // 可选筛选
	MediaType   string // 可选筛选
	Page        int
	PageSize    int
}

// SoraGenerationRepository 生成记录持久化接口。
type SoraGenerationRepository interface {
	Create(ctx context.Context, gen *SoraGeneration) error
	GetByID(ctx context.Context, id int64) (*SoraGeneration, error)
	Update(ctx context.Context, gen *SoraGeneration) error
	Delete(ctx context.Context, id int64) error
	List(ctx context.Context, params SoraGenerationListParams) ([]*SoraGeneration, int64, error)
	CountByUserAndStatus(ctx context.Context, userID int64, statuses []string) (int64, error)
}
