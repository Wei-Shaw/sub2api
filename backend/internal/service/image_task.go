package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

const (
	ImageTaskStatusProcessing = "processing"
	ImageTaskStatusCompleted  = "completed"
	ImageTaskStatusFailed     = "failed"

	defaultImageTaskTTL              = 24 * time.Hour
	defaultImageTaskExecutionTimeout = 30 * time.Minute
	defaultImageTaskCleanupInterval  = 5 * time.Minute
	defaultImageTaskCleanupBatchSize = 100
)

var (
	ErrImageTaskNotFound       = infraerrors.New(http.StatusNotFound, "IMAGE_TASK_NOT_FOUND", "image task not found")
	ErrImageTaskForbidden      = infraerrors.New(http.StatusForbidden, "IMAGE_TASK_FORBIDDEN", "image task does not belong to this API key")
	ErrImageTaskUnavailable    = infraerrors.New(http.StatusServiceUnavailable, "IMAGE_TASK_UNAVAILABLE", "image task storage is unavailable")
	ErrImageTaskNotReady       = infraerrors.Conflict("IMAGE_TASK_NOT_READY", "image task is not completed")
	ErrImageTaskDeleteNotReady = infraerrors.Conflict("IMAGE_TASK_DELETE_NOT_READY", "an image task cannot be deleted while it is processing")
	ErrImageTaskImageNotFound  = infraerrors.New(http.StatusNotFound, "IMAGE_TASK_IMAGE_NOT_FOUND", "generated image not found")
)

// ImageTaskRecord is the private Redis representation of an asynchronous image
// request. Ownership fields are intentionally omitted from the public view.
type ImageTaskRecord struct {
	ID              string          `json:"id"`
	UserID          int64           `json:"user_id"`
	APIKeyID        int64           `json:"api_key_id"`
	GroupID         int64           `json:"group_id,omitempty"`
	Platform        string          `json:"platform,omitempty"`
	Model           string          `json:"model,omitempty"`
	PromptPreview   string          `json:"prompt_preview,omitempty"`
	InputImageCount int             `json:"input_image_count,omitempty"`
	Status          string          `json:"status"`
	HTTPStatus      int             `json:"http_status,omitempty"`
	Result          json.RawMessage `json:"result,omitempty"`
	Error           json.RawMessage `json:"error,omitempty"`
	CreatedAt       int64           `json:"created_at"`
	CompletedAt     *int64          `json:"completed_at,omitempty"`
	ExpiresAt       int64           `json:"expires_at"`
	StorageKeys     []string        `json:"storage_keys,omitempty"`
	StorageIdentity string          `json:"storage_identity,omitempty"`
}

// ImageTask is the API-safe task representation returned to callers.
type ImageTask struct {
	ID              string          `json:"id"`
	TaskID          string          `json:"task_id"`
	Object          string          `json:"object"`
	GroupID         int64           `json:"group_id,omitempty"`
	Platform        string          `json:"platform,omitempty"`
	Model           string          `json:"model,omitempty"`
	PromptPreview   string          `json:"prompt_preview,omitempty"`
	InputImageCount int             `json:"input_image_count,omitempty"`
	Status          string          `json:"status"`
	HTTPStatus      int             `json:"http_status,omitempty"`
	ImageURL        string          `json:"image_url,omitempty"` // reserved for compatibility; never contains an object-store URL
	ImageCount      int             `json:"image_count,omitempty"`
	Result          json.RawMessage `json:"result,omitempty"`
	Error           json.RawMessage `json:"error,omitempty"`
	CreatedAt       int64           `json:"created_at"`
	CompletedAt     *int64          `json:"completed_at,omitempty"`
	ExpiresAt       int64           `json:"expires_at"`
}

type ImageTaskOwner struct {
	UserID   int64
	APIKeyID int64
}

type ImageTaskMetadata struct {
	GroupID         int64
	Platform        string
	Model           string
	PromptPreview   string
	InputImageCount int
}

type ImageTaskDownload struct {
	Data        []byte
	ContentType string
	Filename    string
}

type ImageTaskCleanup struct {
	TaskID          string   `json:"task_id"`
	Keys            []string `json:"keys"`
	ExpiresAt       int64    `json:"expires_at"`
	StorageIdentity string   `json:"storage_identity,omitempty"`
}

type ImageTaskStore interface {
	Save(ctx context.Context, task *ImageTaskRecord, ttl time.Duration) error
	Get(ctx context.Context, id string) (*ImageTaskRecord, error)
	ListByUser(ctx context.Context, userID int64, limit int) ([]*ImageTaskRecord, error)
	Delete(ctx context.Context, id string) error
	ScheduleCleanup(ctx context.Context, cleanup ImageTaskCleanup) error
	ListDueCleanup(ctx context.Context, now time.Time, limit int) ([]ImageTaskCleanup, error)
	DeleteCleanup(ctx context.Context, id string) error
}

// ImageStorageResolver reports the currently effective object-storage binding.
// It exists so the async image feature can be switched on and off from the admin
// UI without a restart: the wiring below is fixed at startup, but the answer to
// "is object storage configured right now" is re-read (and cached) per call.
type ImageStorageResolver func() (uploader *ImageResultUploader, enabled bool, retention time.Duration)

type ImageTaskService struct {
	store            ImageTaskStore
	uploader         *ImageResultUploader
	enabled          bool
	resolve          ImageStorageResolver
	ttl              time.Duration
	executionTimeout time.Duration

	cleanupCancel context.CancelFunc
	cleanupDone   chan struct{}
	cleanupMu     sync.Mutex
}

func NewImageTaskService(store ImageTaskStore) *ImageTaskService {
	return NewImageTaskServiceWithOptions(store, defaultImageTaskTTL, defaultImageTaskExecutionTimeout)
}

func NewImageTaskServiceWithOptions(store ImageTaskStore, ttl, executionTimeout time.Duration) *ImageTaskService {
	if ttl <= 0 {
		ttl = defaultImageTaskTTL
	}
	if executionTimeout <= 0 {
		executionTimeout = defaultImageTaskExecutionTimeout
	}
	return &ImageTaskService{store: store, ttl: ttl, executionTimeout: executionTimeout}
}

// NewImageTaskServiceWithUploader 构造一个已启用的图片任务服务：结果会先经 uploader
// 转存到对象存储再落 Redis。uploader 为 nil 时不做转存（仅用于测试）。
func NewImageTaskServiceWithUploader(store ImageTaskStore, uploader *ImageResultUploader, ttl, executionTimeout time.Duration) *ImageTaskService {
	s := NewImageTaskServiceWithOptions(store, ttl, executionTimeout)
	s.uploader = uploader
	s.enabled = true
	return s
}

// NewImageTaskServiceWithResolver 构造一个由 resolver 决定启用状态的服务：
// 开关与凭证来自后台设置，保存后立即生效，无需重启。
func NewImageTaskServiceWithResolver(store ImageTaskStore, resolve ImageStorageResolver, ttl, executionTimeout time.Duration) *ImageTaskService {
	s := NewImageTaskServiceWithOptions(store, ttl, executionTimeout)
	s.resolve = resolve
	return s
}

// current 返回当前生效的 uploader 与启用状态。
// 注入了 resolver 时以 resolver 为准（后台设置可热切换），否则回落到构造时固定的值。
func (s *ImageTaskService) current() (*ImageResultUploader, bool, time.Duration) {
	if s == nil {
		return nil, false, defaultImageTaskTTL
	}
	if s.resolve != nil {
		uploader, enabled, retention := s.resolve()
		if retention <= 0 {
			retention = defaultImageTaskTTL
		}
		return uploader, enabled, retention
	}
	return s.uploader, s.enabled, s.ttl
}

// Enabled 表示异步图片任务功能是否可用（总开关 + 凭证齐全）。
// 关闭时 handler 直接返回 404，不创建任务、不写 Redis。
func (s *ImageTaskService) Enabled() bool {
	if s == nil || s.store == nil {
		return false
	}
	_, enabled, _ := s.current()
	return enabled
}

func (s *ImageTaskService) Retention() time.Duration {
	if s == nil {
		return defaultImageTaskTTL
	}
	_, _, retention := s.current()
	return retention
}

// Pollable 表示已创建的任务能否被查询。
// 比 Enabled 弱：只要 store 可用即可，从而在功能被关掉后仍能取回进行中的任务结果。
func (s *ImageTaskService) Pollable() bool {
	return s != nil && s.store != nil
}

func (s *ImageTaskService) ExecutionTimeout() time.Duration {
	if s == nil || s.executionTimeout <= 0 {
		return defaultImageTaskExecutionTimeout
	}
	return s.executionTimeout
}

func (s *ImageTaskService) Create(ctx context.Context, owner ImageTaskOwner) (*ImageTask, error) {
	return s.CreateWithMetadata(ctx, owner, ImageTaskMetadata{})
}

func (s *ImageTaskService) CreateWithMetadata(ctx context.Context, owner ImageTaskOwner, metadata ImageTaskMetadata) (*ImageTask, error) {
	if s == nil || s.store == nil {
		return nil, ErrImageTaskUnavailable
	}
	now := time.Now().UTC()
	retention := s.Retention()
	task := &ImageTaskRecord{
		ID:              "imgtask_" + strings.ReplaceAll(uuid.NewString(), "-", ""),
		UserID:          owner.UserID,
		APIKeyID:        owner.APIKeyID,
		GroupID:         metadata.GroupID,
		Platform:        strings.TrimSpace(metadata.Platform),
		Model:           strings.TrimSpace(metadata.Model),
		PromptPreview:   truncateImageTaskText(metadata.PromptPreview, 240),
		InputImageCount: metadata.InputImageCount,
		Status:          ImageTaskStatusProcessing,
		CreatedAt:       now.Unix(),
		ExpiresAt:       now.Add(retention).Unix(),
	}
	if err := s.store.Save(ctx, task, retention); err != nil {
		return nil, ErrImageTaskUnavailable.WithCause(err)
	}
	return imageTaskToPublic(task), nil
}

func (s *ImageTaskService) Get(ctx context.Context, owner ImageTaskOwner, id string) (*ImageTask, error) {
	if s == nil || s.store == nil {
		return nil, ErrImageTaskUnavailable
	}
	task, err := s.store.Get(ctx, strings.TrimSpace(id))
	if err != nil {
		if errors.Is(err, ErrImageTaskNotFound) {
			return nil, ErrImageTaskNotFound
		}
		return nil, ErrImageTaskUnavailable.WithCause(err)
	}
	if task.UserID != owner.UserID || task.APIKeyID != owner.APIKeyID {
		// Do not reveal whether a random task ID exists for another caller.
		return nil, ErrImageTaskNotFound
	}
	return imageTaskToPublic(task), nil
}

// GetForUser is used by the authenticated dashboard. External API task reads
// remain scoped to both user and API key through Get.
func (s *ImageTaskService) GetForUser(ctx context.Context, userID int64, id string) (*ImageTask, error) {
	task, err := s.getRecord(ctx, id)
	if err != nil {
		return nil, err
	}
	if userID <= 0 || task.UserID != userID {
		return nil, ErrImageTaskNotFound
	}
	return imageTaskToPublic(task), nil
}

func (s *ImageTaskService) ListForUser(ctx context.Context, userID int64, limit int) ([]*ImageTask, error) {
	if s == nil || s.store == nil {
		return nil, ErrImageTaskUnavailable
	}
	if userID <= 0 {
		return []*ImageTask{}, nil
	}
	if limit <= 0 {
		limit = 24
	}
	if limit > 100 {
		limit = 100
	}
	records, err := s.store.ListByUser(ctx, userID, limit)
	if err != nil {
		return nil, ErrImageTaskUnavailable.WithCause(err)
	}
	tasks := make([]*ImageTask, 0, len(records))
	for _, record := range records {
		if record == nil || record.UserID != userID {
			continue
		}
		tasks = append(tasks, imageTaskToPublic(record))
		if len(tasks) == limit {
			break
		}
	}
	return tasks, nil
}

func (s *ImageTaskService) DownloadForUser(ctx context.Context, userID int64, id string, imageIndex int) (*ImageTaskDownload, error) {
	task, err := s.getRecord(ctx, id)
	if err != nil {
		return nil, err
	}
	if userID <= 0 || task.UserID != userID {
		return nil, ErrImageTaskNotFound
	}
	return s.downloadRecord(ctx, task, imageIndex)
}

// Download is the API-key scoped counterpart of DownloadForUser. Both the user
// and the exact key that submitted the asynchronous request must match.
func (s *ImageTaskService) Download(ctx context.Context, owner ImageTaskOwner, id string, imageIndex int) (*ImageTaskDownload, error) {
	task, err := s.getRecord(ctx, id)
	if err != nil {
		return nil, err
	}
	if owner.UserID <= 0 || owner.APIKeyID <= 0 || task.UserID != owner.UserID || task.APIKeyID != owner.APIKeyID {
		return nil, ErrImageTaskNotFound
	}
	return s.downloadRecord(ctx, task, imageIndex)
}

func (s *ImageTaskService) downloadRecord(ctx context.Context, task *ImageTaskRecord, imageIndex int) (*ImageTaskDownload, error) {
	if task.Status != ImageTaskStatusCompleted {
		return nil, ErrImageTaskNotReady
	}
	if imageIndex < 0 || imageIndex >= len(task.StorageKeys) {
		return nil, ErrImageTaskImageNotFound
	}
	uploader, err := s.uploaderForStorage(task.StorageIdentity)
	if err != nil {
		return nil, ErrImageTaskUnavailable.WithCause(err)
	}
	data, contentType, err := uploader.Load(ctx, task.StorageKeys[imageIndex])
	if err != nil {
		return nil, ErrImageTaskUnavailable.WithCause(err)
	}
	return &ImageTaskDownload{
		Data:        data,
		ContentType: contentType,
		Filename:    task.ID + "-" + strconv.Itoa(imageIndex+1) + extensionForContentType(contentType),
	}, nil
}

func (s *ImageTaskService) DeleteForUser(ctx context.Context, userID int64, id string) error {
	task, err := s.getRecord(ctx, id)
	if err != nil {
		return err
	}
	if userID <= 0 || task.UserID != userID {
		return ErrImageTaskNotFound
	}
	if task.Status == ImageTaskStatusProcessing {
		return ErrImageTaskDeleteNotReady
	}
	uploader, err := s.uploaderForStorage(task.StorageIdentity)
	if err != nil {
		return ErrImageTaskUnavailable.WithCause(err)
	}
	if err := uploader.Delete(ctx, task.StorageKeys); err != nil {
		return ErrImageTaskUnavailable.WithCause(err)
	}
	if err := s.store.Delete(ctx, task.ID); err != nil {
		return ErrImageTaskUnavailable.WithCause(err)
	}
	if err := s.store.DeleteCleanup(ctx, task.ID); err != nil {
		return ErrImageTaskUnavailable.WithCause(err)
	}
	return nil
}

func (s *ImageTaskService) getRecord(ctx context.Context, id string) (*ImageTaskRecord, error) {
	if s == nil || s.store == nil {
		return nil, ErrImageTaskUnavailable
	}
	task, err := s.store.Get(ctx, strings.TrimSpace(id))
	if err != nil {
		if errors.Is(err, ErrImageTaskNotFound) {
			return nil, ErrImageTaskNotFound
		}
		return nil, ErrImageTaskUnavailable.WithCause(err)
	}
	return task, nil
}

func (s *ImageTaskService) Complete(ctx context.Context, id string, statusCode int, result json.RawMessage) error {
	if !json.Valid(result) {
		return s.Fail(ctx, id, http.StatusBadGateway, imageTaskErrorJSON("api_error", "upstream returned a non-JSON image response"))
	}
	var storageKeys []string
	uploader, _, _ := s.current()
	storageIdentity := ""
	if uploader != nil {
		rewritten, keys, err := uploader.RewriteWithKeys(ctx, id, result)
		if err != nil {
			// 转存失败不回退存 base64，避免大 blob 撑爆 Redis：直接把任务标记为失败。
			logger.L().Error("image_task.offload_failed", zap.String("task_id", id), zap.Error(err))
			return s.Fail(ctx, id, http.StatusBadGateway, imageTaskErrorJSON("api_error", "failed to store generated image to object storage"))
		}
		result = rewritten
		storageKeys = keys
		storageIdentity = uploader.StorageIdentity()
	}
	result = sanitizeImageTaskResult(result)
	err := s.finish(ctx, id, ImageTaskStatusCompleted, statusCode, result, nil, storageKeys, storageIdentity)
	if err != nil && len(storageKeys) > 0 {
		if uploader != nil {
			_ = uploader.Delete(context.Background(), storageKeys)
		}
	}
	return err
}

func (s *ImageTaskService) Fail(ctx context.Context, id string, statusCode int, taskErr json.RawMessage) error {
	if !json.Valid(taskErr) {
		taskErr = imageTaskErrorJSON("api_error", "image generation failed")
	}
	return s.finish(ctx, id, ImageTaskStatusFailed, statusCode, nil, taskErr, nil, "")
}

func (s *ImageTaskService) finish(ctx context.Context, id, status string, statusCode int, result, taskErr json.RawMessage, storageKeys []string, storageIdentity string) error {
	if s == nil || s.store == nil {
		return ErrImageTaskUnavailable
	}
	task, err := s.store.Get(ctx, id)
	if err != nil {
		if errors.Is(err, ErrImageTaskNotFound) {
			return ErrImageTaskNotFound
		}
		return ErrImageTaskUnavailable.WithCause(err)
	}
	now := time.Now().UTC()
	completedAt := now.Unix()
	task.Status = status
	task.HTTPStatus = statusCode
	task.Result = result
	task.Error = taskErr
	task.StorageKeys = append([]string(nil), storageKeys...)
	task.StorageIdentity = storageIdentity
	task.CompletedAt = &completedAt
	retention := s.Retention()
	task.ExpiresAt = now.Add(retention).Unix()
	if len(storageKeys) > 0 {
		if err := s.store.ScheduleCleanup(ctx, ImageTaskCleanup{
			TaskID:          task.ID,
			Keys:            task.StorageKeys,
			ExpiresAt:       task.ExpiresAt,
			StorageIdentity: task.StorageIdentity,
		}); err != nil {
			return ErrImageTaskUnavailable.WithCause(err)
		}
	}
	if err := s.store.Save(ctx, task, retention); err != nil {
		return ErrImageTaskUnavailable.WithCause(err)
	}
	return nil
}

func (s *ImageTaskService) RunCleanupOnce(ctx context.Context, now time.Time) (int, error) {
	if s == nil || s.store == nil {
		return 0, ErrImageTaskUnavailable
	}
	jobs, err := s.store.ListDueCleanup(ctx, now, defaultImageTaskCleanupBatchSize)
	if err != nil {
		return 0, ErrImageTaskUnavailable.WithCause(err)
	}
	cleaned := 0
	for _, job := range jobs {
		uploader, resolveErr := s.uploaderForStorage(job.StorageIdentity)
		if resolveErr != nil {
			logger.L().Warn("image_task.cleanup_storage_changed", zap.String("task_id", job.TaskID), zap.Error(resolveErr))
			continue
		}
		if err := uploader.Delete(ctx, job.Keys); err != nil {
			logger.L().Warn("image_task.cleanup_delete_failed", zap.String("task_id", job.TaskID), zap.Error(err))
			continue
		}
		if err := s.store.DeleteCleanup(ctx, job.TaskID); err != nil {
			logger.L().Warn("image_task.cleanup_record_failed", zap.String("task_id", job.TaskID), zap.Error(err))
			continue
		}
		cleaned++
	}
	return cleaned, nil
}

func (s *ImageTaskService) uploaderForStorage(identity string) (*ImageResultUploader, error) {
	uploader, _, _ := s.current()
	if uploader == nil {
		return nil, errors.New("image object storage is unavailable")
	}
	identity = strings.TrimSpace(identity)
	if current := uploader.StorageIdentity(); identity != "" && current != identity {
		return nil, errors.New("image object storage target changed; restore the previous target before deleting retained files")
	}
	return uploader, nil
}

func (s *ImageTaskService) Start() {
	if s == nil || s.store == nil {
		return
	}
	s.cleanupMu.Lock()
	defer s.cleanupMu.Unlock()
	if s.cleanupCancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.cleanupCancel = cancel
	s.cleanupDone = make(chan struct{})
	go func() {
		defer close(s.cleanupDone)
		ticker := time.NewTicker(defaultImageTaskCleanupInterval)
		defer ticker.Stop()
		for {
			_, _ = s.RunCleanupOnce(ctx, time.Now())
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}

func (s *ImageTaskService) Stop() {
	if s == nil {
		return
	}
	s.cleanupMu.Lock()
	cancel, done := s.cleanupCancel, s.cleanupDone
	s.cleanupCancel, s.cleanupDone = nil, nil
	s.cleanupMu.Unlock()
	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
}

func imageTaskToPublic(task *ImageTaskRecord) *ImageTask {
	if task == nil {
		return nil
	}
	return &ImageTask{
		ID:              task.ID,
		TaskID:          task.ID,
		Object:          "image.generation.task",
		GroupID:         task.GroupID,
		Platform:        task.Platform,
		Model:           task.Model,
		PromptPreview:   task.PromptPreview,
		InputImageCount: task.InputImageCount,
		Status:          task.Status,
		HTTPStatus:      task.HTTPStatus,
		ImageCount:      len(task.StorageKeys),
		Result:          sanitizeImageTaskResult(task.Result),
		Error:           task.Error,
		CreatedAt:       task.CreatedAt,
		CompletedAt:     task.CompletedAt,
		ExpiresAt:       task.ExpiresAt,
	}
}

func truncateImageTaskText(value string, maxRunes int) string {
	value = strings.TrimSpace(value)
	if maxRunes <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= maxRunes {
		return value
	}
	return string(runes[:maxRunes])
}

// sanitizeImageTaskResult removes both upstream image bytes and every kind of
// object URL from persisted legacy records before they cross an API boundary.
func sanitizeImageTaskResult(result json.RawMessage) json.RawMessage {
	if len(result) == 0 || !json.Valid(result) {
		return nil
	}
	var value any
	if json.Unmarshal(result, &value) != nil {
		return nil
	}
	removeImageResultSecrets(value)
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	return encoded
}

func removeImageResultSecrets(value any) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if isPrivateImageResultField(key) {
				delete(typed, key)
				continue
			}
			removeImageResultSecrets(child)
		}
	case []any:
		for _, child := range typed {
			removeImageResultSecrets(child)
		}
	}
}

func isPrivateImageResultField(key string) bool {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "url", "image_url", "download_url", "b64_json":
		return true
	default:
		return false
	}
}

func imageTaskErrorJSON(errorType, message string) json.RawMessage {
	data, _ := json.Marshal(map[string]string{"type": errorType, "message": message})
	return data
}
