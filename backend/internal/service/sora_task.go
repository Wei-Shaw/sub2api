package service

import (
	"context"
	"time"
)

// ── 任务状态常量 ──

const (
	SoraTaskQueued     = "queued"
	SoraTaskInProgress = "in_progress"
	SoraTaskCompleted  = "completed"
	SoraTaskFailed     = "failed"
)

// ── 对象类型常量 ──

const (
	SoraObjectVideo     = "video"
	SoraObjectCharacter = "character"
	SoraObjectImage     = "image"
)

// SoraTask 表示一个 Sora 异步任务记录。
type SoraTask struct {
	ID             string
	AccountID      int64
	APIKeyID       *int64
	UpstreamTaskID string
	ObjectType     string
	Model          string
	Prompt         string
	Status         string
	Progress       int
	VideoURL       string // 原始上游 URL
	StoredKey      string // 存储后的 key（本地路径或 S3 key）
	StorageType    string // local / s3 / gdrive / 空
	ShareID        string
	CharacterInfo  *SoraCharacter
	ErrorMessage   string
	ErrorType      string
	RequestBody    []byte
	Seconds        string
	Size           string
	CreatedAt      time.Time
	CompletedAt    *time.Time
}

// SoraCharacter 角色信息。
type SoraCharacter struct {
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
}

// SoraTaskRepository 持久层接口。
type SoraTaskRepository interface {
	Create(ctx context.Context, task *SoraTask) error
	GetByID(ctx context.Context, id string) (*SoraTask, error)
	GetByIDAndAPIKey(ctx context.Context, id string, apiKeyID int64) (*SoraTask, error)
	Update(ctx context.Context, task *SoraTask) error
	ListPending(ctx context.Context) ([]*SoraTask, error)
}
