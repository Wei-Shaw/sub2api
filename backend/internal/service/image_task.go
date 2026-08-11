package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"math"
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
	defaultImageTaskMutationLockTTL  = 2 * time.Minute
	defaultImageTaskMutationWait     = 2 * time.Second
	defaultImageTaskMutationTimeout  = 90 * time.Second
)

var (
	ErrImageTaskNotFound       = infraerrors.New(http.StatusNotFound, "IMAGE_TASK_NOT_FOUND", "image task not found")
	ErrImageTaskForbidden      = infraerrors.New(http.StatusForbidden, "IMAGE_TASK_FORBIDDEN", "image task does not belong to this API key")
	ErrImageTaskUnavailable    = infraerrors.New(http.StatusServiceUnavailable, "IMAGE_TASK_UNAVAILABLE", "image task storage is unavailable")
	ErrImageTaskNotReady       = infraerrors.Conflict("IMAGE_TASK_NOT_READY", "image task is not completed")
	ErrImageTaskDeleteNotReady = infraerrors.Conflict("IMAGE_TASK_DELETE_NOT_READY", "an image task cannot be deleted while it is processing")
	ErrImageTaskImageNotFound  = infraerrors.New(http.StatusNotFound, "IMAGE_TASK_IMAGE_NOT_FOUND", "generated image not found")
	ErrImageTaskBusy           = infraerrors.Conflict("IMAGE_TASK_BUSY", "image task is being changed; retry shortly")
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
	StorageSizes    []int64         `json:"storage_sizes,omitempty"`
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
	ExpiresAt       int64           `json:"expires_at,omitempty"`
	ImageSizes      []int64         `json:"image_sizes,omitempty"`
	ImageIDs        []string        `json:"-"`
}

type AdminImageTask struct {
	Task         *ImageTask `json:"task"`
	UserID       int64      `json:"user_id"`
	APIKeyID     int64      `json:"api_key_id"`
	StorageBytes int64      `json:"storage_bytes"`
}

type AdminImageTaskPage struct {
	Tasks        []*AdminImageTask `json:"tasks"`
	Page         int               `json:"page"`
	PageSize     int               `json:"page_size"`
	Total        int               `json:"total"`
	TotalImages  int               `json:"total_images"`
	StorageBytes int64             `json:"storage_bytes"`
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

// ImageTaskCompletionFailure reports a successful upstream response that could
// not be finalized locally. When Stored is true, Complete already persisted the
// task as failed; callers only need to emit their external error/audit record.
type ImageTaskCompletionFailure struct {
	StatusCode int
	TaskError  json.RawMessage
	Stored     bool
	Cause      error
}

func (e *ImageTaskCompletionFailure) Error() string {
	if e == nil {
		return "image task completion failed"
	}
	if e.Cause != nil {
		return fmt.Sprintf("image task completion failed: %v", e.Cause)
	}
	return "image task completion failed"
}

func (e *ImageTaskCompletionFailure) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

type ImageTaskDownload struct {
	Data        []byte
	ContentType string
	Filename    string
}

type ImageTaskCleanup struct {
	TaskID          string           `json:"task_id"`
	Keys            []string         `json:"keys"`
	Sizes           []int64          `json:"sizes,omitempty"`
	ExpiresAt       int64            `json:"expires_at"`
	StorageIdentity string           `json:"storage_identity,omitempty"`
	Record          *ImageTaskRecord `json:"record,omitempty"`
}

type ImageTaskStore interface {
	Save(ctx context.Context, task *ImageTaskRecord, ttl time.Duration) error
	Get(ctx context.Context, id string) (*ImageTaskRecord, error)
	ListByUser(ctx context.Context, userID int64, limit int) ([]*ImageTaskRecord, error)
	ListForAdmin(ctx context.Context, offset int64, limit int) ([]*ImageTaskRecord, int, error)
	AdminStorageStats(ctx context.Context) (totalImages int, storageBytes int64, err error)
	Delete(ctx context.Context, id string) error
	ScheduleCleanup(ctx context.Context, cleanup ImageTaskCleanup) error
	GetCleanup(ctx context.Context, id string) (*ImageTaskCleanup, error)
	ListDueCleanup(ctx context.Context, now time.Time, limit int) ([]ImageTaskCleanup, error)
	DeleteCleanup(ctx context.Context, id string) error
	TryLock(ctx context.Context, id, token string, ttl time.Duration) (bool, error)
	Unlock(ctx context.Context, id, token string) error
}

type imageTaskMutationGuardContextKey struct{}

type imageTaskMutationGuard struct {
	taskID string
	token  string
}

// WithImageTaskMutationGuard marks Redis writes that must still own a task's
// mutation lock when they commit. The repository verifies the token atomically.
func WithImageTaskMutationGuard(ctx context.Context, taskID, token string) context.Context {
	return context.WithValue(ctx, imageTaskMutationGuardContextKey{}, imageTaskMutationGuard{
		taskID: strings.TrimSpace(taskID),
		token:  strings.TrimSpace(token),
	})
}

// ImageTaskMutationGuardFromContext returns the lock identity for guarded
// repository mutations. Calls outside a task mutation intentionally have none.
func ImageTaskMutationGuardFromContext(ctx context.Context) (taskID, token string, ok bool) {
	guard, ok := ctx.Value(imageTaskMutationGuardContextKey{}).(imageTaskMutationGuard)
	if !ok || guard.taskID == "" || guard.token == "" {
		return "", "", false
	}
	return guard.taskID, guard.token, true
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
	mutationLockTTL  time.Duration
	mutationWait     time.Duration
	mutationTimeout  time.Duration

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
	return &ImageTaskService{
		store: store, ttl: ttl, executionTimeout: executionTimeout,
		mutationLockTTL: defaultImageTaskMutationLockTTL,
		mutationWait:    defaultImageTaskMutationWait,
		mutationTimeout: defaultImageTaskMutationTimeout,
	}
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
		ExpiresAt:       0,
	}
	processingTTL := retention + s.ExecutionTimeout()
	if err := s.store.Save(ctx, task, processingTTL); err != nil {
		return nil, ErrImageTaskUnavailable.WithCause(err)
	}
	return imageTaskToPublic(task), nil
}

func (s *ImageTaskService) ListForAdmin(ctx context.Context, page, pageSize int) (*AdminImageTaskPage, error) {
	if s == nil || s.store == nil {
		return nil, ErrImageTaskUnavailable
	}
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 24
	}
	if pageSize > 100 {
		pageSize = 100
	}
	pageIndex := int64(page - 1)
	offset := int64(math.MaxInt64)
	if pageIndex <= math.MaxInt64/int64(pageSize) {
		offset = pageIndex * int64(pageSize)
	}
	records, total, err := s.store.ListForAdmin(ctx, offset, pageSize)
	if err != nil {
		return nil, ErrImageTaskUnavailable.WithCause(err)
	}
	all := make([]*AdminImageTask, 0, len(records))
	for _, record := range records {
		if record == nil {
			continue
		}
		if len(record.StorageKeys) > 0 && len(record.StorageSizes) != len(record.StorageKeys) {
			if err := s.withTaskMutation(ctx, record.ID, func(mutationCtx context.Context) error {
				fresh, getErr := s.getRecordForAdmin(mutationCtx, record.ID)
				if getErr != nil {
					return getErr
				}
				if sizeErr := s.ensureStorageSizes(mutationCtx, fresh); sizeErr != nil {
					return sizeErr
				}
				record = fresh
				return nil
			}); err != nil {
				logger.L().Warn("image_task.storage_size_failed", zap.String("task_id", record.ID), zap.Error(err))
			}
		}
		bytes := sumImageStorageSizes(record.StorageSizes)
		all = append(all, &AdminImageTask{
			Task: imageTaskToPublic(record), UserID: record.UserID, APIKeyID: record.APIKeyID, StorageBytes: bytes,
		})
	}
	totalImages, totalBytes, err := s.store.AdminStorageStats(ctx)
	if err != nil {
		return nil, ErrImageTaskUnavailable.WithCause(err)
	}
	return &AdminImageTaskPage{
		Tasks: all, Page: page, PageSize: pageSize, Total: total,
		TotalImages: totalImages, StorageBytes: totalBytes,
	}, nil
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

func (s *ImageTaskService) DownloadForUser(ctx context.Context, userID int64, id, imageRef string) (*ImageTaskDownload, error) {
	task, err := s.getRecord(ctx, id)
	if err != nil {
		return nil, err
	}
	if userID <= 0 || task.UserID != userID {
		return nil, ErrImageTaskNotFound
	}
	return s.downloadRecord(ctx, task, imageRef)
}

func (s *ImageTaskService) DownloadForAdmin(ctx context.Context, id, imageRef string) (*ImageTaskDownload, error) {
	task, err := s.getRecordForAdmin(ctx, id)
	if err != nil {
		return nil, err
	}
	return s.downloadRecord(ctx, task, imageRef)
}

// Download is the API-key scoped counterpart of DownloadForUser. Both the user
// and the exact key that submitted the asynchronous request must match.
func (s *ImageTaskService) Download(ctx context.Context, owner ImageTaskOwner, id, imageRef string) (*ImageTaskDownload, error) {
	task, err := s.getRecord(ctx, id)
	if err != nil {
		return nil, err
	}
	if owner.UserID <= 0 || owner.APIKeyID <= 0 || task.UserID != owner.UserID || task.APIKeyID != owner.APIKeyID {
		return nil, ErrImageTaskNotFound
	}
	return s.downloadRecord(ctx, task, imageRef)
}

func (s *ImageTaskService) downloadRecord(ctx context.Context, task *ImageTaskRecord, imageRef string) (*ImageTaskDownload, error) {
	if task.Status != ImageTaskStatusCompleted {
		return nil, ErrImageTaskNotReady
	}
	imageIndex := resolveImageTaskImageIndex(task, imageRef)
	if imageIndex < 0 {
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
	return s.withTaskMutation(ctx, id, func(mutationCtx context.Context) error {
		task, err := s.getRecord(mutationCtx, id)
		if err != nil {
			return err
		}
		if userID <= 0 || task.UserID != userID {
			return ErrImageTaskNotFound
		}
		if task.Status == ImageTaskStatusProcessing {
			return ErrImageTaskDeleteNotReady
		}
		return s.deleteRecord(mutationCtx, task)
	})
}

func (s *ImageTaskService) DeleteForAdmin(ctx context.Context, id string) error {
	return s.withTaskMutation(ctx, id, func(mutationCtx context.Context) error {
		task, err := s.getRecordForAdmin(mutationCtx, id)
		if err != nil {
			return err
		}
		if task.Status == ImageTaskStatusProcessing {
			return ErrImageTaskDeleteNotReady
		}
		return s.deleteRecord(mutationCtx, task)
	})
}

func (s *ImageTaskService) DeleteImageForUser(ctx context.Context, userID int64, id, imageRef string) (updated *ImageTask, err error) {
	err = s.withTaskMutation(ctx, id, func(mutationCtx context.Context) error {
		task, getErr := s.getRecord(mutationCtx, id)
		if getErr != nil {
			return getErr
		}
		if userID <= 0 || task.UserID != userID {
			return ErrImageTaskNotFound
		}
		updated, getErr = s.deleteImageRecord(mutationCtx, task, imageRef)
		return getErr
	})
	return updated, err
}

func (s *ImageTaskService) DeleteImageForAdmin(ctx context.Context, id, imageRef string) (updated *ImageTask, err error) {
	err = s.withTaskMutation(ctx, id, func(mutationCtx context.Context) error {
		task, getErr := s.getRecordForAdmin(mutationCtx, id)
		if getErr != nil {
			return getErr
		}
		updated, getErr = s.deleteImageRecord(mutationCtx, task, imageRef)
		return getErr
	})
	return updated, err
}

func (s *ImageTaskService) deleteRecord(ctx context.Context, task *ImageTaskRecord) error {
	if len(task.StorageKeys) > 0 {
		uploader, err := s.uploaderForStorage(task.StorageIdentity)
		if err != nil {
			return ErrImageTaskUnavailable.WithCause(err)
		}
		if err := uploader.Delete(ctx, task.StorageKeys); err != nil {
			return ErrImageTaskUnavailable.WithCause(err)
		}
	}
	if err := s.store.Delete(ctx, task.ID); err != nil {
		return ErrImageTaskUnavailable.WithCause(err)
	}
	if err := s.store.DeleteCleanup(ctx, task.ID); err != nil {
		return ErrImageTaskUnavailable.WithCause(err)
	}
	return nil
}

func (s *ImageTaskService) deleteImageRecord(ctx context.Context, task *ImageTaskRecord, imageRef string) (*ImageTask, error) {
	if task.Status == ImageTaskStatusProcessing {
		return nil, ErrImageTaskDeleteNotReady
	}
	imageIndex := resolveImageTaskImageIndex(task, imageRef)
	if imageIndex < 0 {
		return nil, ErrImageTaskImageNotFound
	}
	uploader, err := s.uploaderForStorage(task.StorageIdentity)
	if err != nil {
		return nil, ErrImageTaskUnavailable.WithCause(err)
	}
	if err := uploader.Delete(ctx, []string{task.StorageKeys[imageIndex]}); err != nil {
		return nil, ErrImageTaskUnavailable.WithCause(err)
	}
	task.StorageKeys = removeStringAt(task.StorageKeys, imageIndex)
	task.StorageSizes = removeInt64At(task.StorageSizes, imageIndex)
	task.Result = removeImageTaskResultAt(task.Result, imageIndex)
	if len(task.StorageKeys) == 0 {
		if err := s.store.Delete(ctx, task.ID); err != nil {
			return nil, ErrImageTaskUnavailable.WithCause(err)
		}
		if err := s.store.DeleteCleanup(ctx, task.ID); err != nil {
			return nil, ErrImageTaskUnavailable.WithCause(err)
		}
		return nil, nil
	}
	remainingTTL := time.Until(time.Unix(task.ExpiresAt, 0))
	if remainingTTL <= 0 {
		remainingTTL = time.Minute
	}
	if err := s.store.Save(ctx, task, remainingTTL); err != nil {
		return nil, ErrImageTaskUnavailable.WithCause(err)
	}
	if err := s.store.ScheduleCleanup(ctx, ImageTaskCleanup{
		TaskID: task.ID, Keys: task.StorageKeys, Sizes: task.StorageSizes, ExpiresAt: task.ExpiresAt,
		StorageIdentity: task.StorageIdentity, Record: cloneImageTaskRecord(task),
	}); err != nil {
		return nil, ErrImageTaskUnavailable.WithCause(err)
	}
	return imageTaskToPublic(task), nil
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

func (s *ImageTaskService) getRecordForAdmin(ctx context.Context, id string) (*ImageTaskRecord, error) {
	task, err := s.getRecord(ctx, id)
	if err == nil {
		return task, nil
	}
	if !errors.Is(err, ErrImageTaskNotFound) {
		return nil, err
	}
	cleanup, cleanupErr := s.store.GetCleanup(ctx, strings.TrimSpace(id))
	if cleanupErr != nil {
		if errors.Is(cleanupErr, ErrImageTaskNotFound) {
			return nil, ErrImageTaskNotFound
		}
		return nil, ErrImageTaskUnavailable.WithCause(cleanupErr)
	}
	if cleanup.Record != nil {
		return cloneImageTaskRecord(cleanup.Record), nil
	}
	return &ImageTaskRecord{
		ID: cleanup.TaskID, Status: ImageTaskStatusCompleted, ExpiresAt: cleanup.ExpiresAt,
		StorageKeys: append([]string(nil), cleanup.Keys...), StorageSizes: append([]int64(nil), cleanup.Sizes...),
		StorageIdentity: cleanup.StorageIdentity,
	}, nil
}

func (s *ImageTaskService) withTaskMutation(ctx context.Context, id string, fn func(context.Context) error) error {
	id = strings.TrimSpace(id)
	if s == nil || s.store == nil || id == "" {
		return ErrImageTaskUnavailable
	}
	token := uuid.NewString()
	lockTTL := s.mutationLockTTL
	if lockTTL <= 0 {
		lockTTL = defaultImageTaskMutationLockTTL
	}
	wait := s.mutationWait
	if wait <= 0 {
		wait = defaultImageTaskMutationWait
	}
	operationTimeout := s.mutationTimeout
	if operationTimeout <= 0 || operationTimeout >= lockTTL {
		operationTimeout = lockTTL * 3 / 4
	}
	deadline := time.Now().Add(wait)
	for {
		locked, err := s.store.TryLock(ctx, id, token, lockTTL)
		if err != nil {
			return ErrImageTaskUnavailable.WithCause(err)
		}
		if locked {
			defer func() {
				if err := s.store.Unlock(context.Background(), id, token); err != nil {
					logger.L().Warn("image_task.unlock_failed", zap.String("task_id", id), zap.Error(err))
				}
			}()
			mutationCtx, cancel := context.WithTimeout(ctx, operationTimeout)
			defer cancel()
			mutationCtx = WithImageTaskMutationGuard(mutationCtx, id, token)
			return fn(mutationCtx)
		}
		if time.Now().After(deadline) {
			return ErrImageTaskBusy
		}
		select {
		case <-ctx.Done():
			return ErrImageTaskBusy.WithCause(ctx.Err())
		case <-time.After(25 * time.Millisecond):
		}
	}
}

func (s *ImageTaskService) Complete(ctx context.Context, id string, statusCode int, result json.RawMessage) error {
	if !json.Valid(result) {
		return s.Fail(ctx, id, http.StatusBadGateway, imageTaskErrorJSON("api_error", "upstream returned a non-JSON image response"))
	}
	var storageKeys []string
	var storageSizes []int64
	uploader, _, _ := s.current()
	storageIdentity := ""
	if uploader != nil {
		rewritten, objects, err := uploader.RewriteWithObjects(ctx, id, result)
		if err != nil {
			// 转存失败不回退存 base64，避免大 blob 撑爆 Redis：直接把任务标记为失败。
			logger.L().Error("image_task.offload_failed", zap.String("task_id", id), zap.Error(err))
			taskErr := imageTaskErrorJSON("api_error", "failed to store generated image to object storage")
			failure := &ImageTaskCompletionFailure{
				StatusCode: http.StatusBadGateway,
				TaskError:  taskErr,
				Cause:      err,
			}
			if failErr := s.Fail(ctx, id, failure.StatusCode, taskErr); failErr != nil {
				failure.Cause = errors.Join(err, failErr)
				return failure
			}
			failure.Stored = true
			return failure
		}
		result = rewritten
		storageKeys = make([]string, len(objects))
		storageSizes = make([]int64, len(objects))
		for i, object := range objects {
			storageKeys[i] = object.Key
			storageSizes[i] = object.Size
		}
		storageIdentity = uploader.StorageIdentity()
	}
	result = sanitizeImageTaskResult(result)
	err := s.finish(ctx, id, ImageTaskStatusCompleted, statusCode, result, nil, storageKeys, storageSizes, storageIdentity)
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
	return s.finish(ctx, id, ImageTaskStatusFailed, statusCode, nil, taskErr, nil, nil, "")
}

func (s *ImageTaskService) finish(ctx context.Context, id, status string, statusCode int, result, taskErr json.RawMessage, storageKeys []string, storageSizes []int64, storageIdentity string) error {
	if s == nil || s.store == nil {
		return ErrImageTaskUnavailable
	}
	return s.withTaskMutation(ctx, id, func(mutationCtx context.Context) error {
		task, err := s.store.Get(mutationCtx, id)
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
		task.StorageSizes = append([]int64(nil), storageSizes...)
		task.StorageIdentity = storageIdentity
		task.CompletedAt = &completedAt
		retention := s.Retention()
		task.ExpiresAt = now.Add(retention).Unix()
		if len(storageKeys) > 0 {
			if err := s.store.ScheduleCleanup(mutationCtx, ImageTaskCleanup{
				TaskID: task.ID, Keys: task.StorageKeys, Sizes: task.StorageSizes, ExpiresAt: task.ExpiresAt,
				StorageIdentity: task.StorageIdentity, Record: cloneImageTaskRecord(task),
			}); err != nil {
				return ErrImageTaskUnavailable.WithCause(err)
			}
		}
		if err := s.store.Save(mutationCtx, task, retention); err != nil {
			return ErrImageTaskUnavailable.WithCause(err)
		}
		return nil
	})
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
		err := s.withTaskMutation(ctx, job.TaskID, func(mutationCtx context.Context) error {
			current, getErr := s.store.GetCleanup(mutationCtx, job.TaskID)
			if errors.Is(getErr, ErrImageTaskNotFound) {
				return nil
			}
			if getErr != nil {
				return getErr
			}
			if current.ExpiresAt > now.Unix() {
				return nil
			}
			uploader, resolveErr := s.uploaderForStorage(current.StorageIdentity)
			if resolveErr != nil {
				return resolveErr
			}
			if deleteErr := uploader.Delete(mutationCtx, current.Keys); deleteErr != nil {
				return deleteErr
			}
			if deleteErr := s.store.Delete(mutationCtx, current.TaskID); deleteErr != nil {
				return deleteErr
			}
			if deleteErr := s.store.DeleteCleanup(mutationCtx, current.TaskID); deleteErr != nil {
				return deleteErr
			}
			cleaned++
			return nil
		})
		if err != nil {
			logger.L().Warn("image_task.cleanup_failed", zap.String("task_id", job.TaskID), zap.Error(err))
		}
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
	imageIDs := make([]string, len(task.StorageKeys))
	for i, key := range task.StorageKeys {
		imageIDs[i] = imageTaskImageID(task.ID, key)
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
		ImageSizes:      append([]int64(nil), task.StorageSizes...),
		ImageIDs:        imageIDs,
	}
}

func imageTaskImageID(taskID, storageKey string) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(taskID) + "\x00" + strings.TrimSpace(storageKey)))
	return fmt.Sprintf("img_%x", digest[:12])
}

func resolveImageTaskImageIndex(task *ImageTaskRecord, imageRef string) int {
	if task == nil {
		return -1
	}
	imageRef = strings.TrimSpace(imageRef)
	if index, err := strconv.Atoi(imageRef); err == nil {
		if index >= 0 && index < len(task.StorageKeys) {
			return index
		}
		return -1
	}
	for index, key := range task.StorageKeys {
		if imageTaskImageID(task.ID, key) == imageRef {
			return index
		}
	}
	return -1
}

func cloneImageTaskRecord(task *ImageTaskRecord) *ImageTaskRecord {
	if task == nil {
		return nil
	}
	cloned := *task
	cloned.Result = append(json.RawMessage(nil), task.Result...)
	cloned.Error = append(json.RawMessage(nil), task.Error...)
	cloned.StorageKeys = append([]string(nil), task.StorageKeys...)
	cloned.StorageSizes = append([]int64(nil), task.StorageSizes...)
	return &cloned
}

func (s *ImageTaskService) ensureStorageSizes(ctx context.Context, task *ImageTaskRecord) error {
	if task == nil || len(task.StorageKeys) == 0 || len(task.StorageSizes) == len(task.StorageKeys) {
		return nil
	}
	uploader, err := s.uploaderForStorage(task.StorageIdentity)
	if err != nil {
		return err
	}
	sizes := make([]int64, len(task.StorageKeys))
	for i, key := range task.StorageKeys {
		sizes[i], err = uploader.Size(ctx, key)
		if err != nil {
			return err
		}
	}
	task.StorageSizes = sizes
	if cleanup, cleanupErr := s.store.GetCleanup(ctx, task.ID); cleanupErr == nil {
		cleanup.Keys = append([]string(nil), task.StorageKeys...)
		cleanup.Sizes = append([]int64(nil), sizes...)
		cleanup.Record = cloneImageTaskRecord(task)
		return s.store.ScheduleCleanup(ctx, *cleanup)
	} else if !errors.Is(cleanupErr, ErrImageTaskNotFound) {
		return cleanupErr
	}
	remainingTTL := time.Until(time.Unix(task.ExpiresAt, 0))
	if remainingTTL <= 0 {
		remainingTTL = time.Minute
	}
	return s.store.Save(ctx, task, remainingTTL)
}

func sumImageStorageSizes(sizes []int64) int64 {
	var total int64
	for _, size := range sizes {
		if size > 0 {
			total += size
		}
	}
	return total
}

func removeStringAt(values []string, index int) []string {
	if index < 0 || index >= len(values) {
		return values
	}
	return append(values[:index:index], values[index+1:]...)
}

func removeInt64At(values []int64, index int) []int64 {
	if index < 0 || index >= len(values) {
		return values
	}
	return append(values[:index:index], values[index+1:]...)
}

func removeImageTaskResultAt(result json.RawMessage, index int) json.RawMessage {
	if len(result) == 0 || !json.Valid(result) {
		return result
	}
	var top map[string]json.RawMessage
	if json.Unmarshal(result, &top) != nil {
		return result
	}
	var data []json.RawMessage
	if json.Unmarshal(top["data"], &data) != nil || index < 0 || index >= len(data) {
		return result
	}
	data = append(data[:index:index], data[index+1:]...)
	encodedData, err := json.Marshal(data)
	if err != nil {
		return result
	}
	top["data"] = encodedData
	encoded, err := json.Marshal(top)
	if err != nil {
		return result
	}
	return encoded
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
