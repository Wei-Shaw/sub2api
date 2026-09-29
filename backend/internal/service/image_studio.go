package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/tidwall/gjson"
	"go.uber.org/zap"
)

// Image Studio：登录用户的交互式生图（参考 codex2api Image Studio，见 docs/IMAGE_STUDIO_SPEC.md）。
// 任务经网关用所选 API Key 回放 /v1/images/*，因此分组权限、余额/订阅、审核与计费全部沿用网关逻辑。

const (
	ImageStudioStatusQueued    = "queued"
	ImageStudioStatusRunning   = "running"
	ImageStudioStatusSucceeded = "succeeded"
	ImageStudioStatusFailed    = "failed"

	ImageStudioKindGenerate = "generate"
	ImageStudioKindEdit     = "edit"

	imageStudioMaxUnfinishedPerUser = 2
	imageStudioMaxN                 = 4
	imageStudioMaxInputImages       = 4
	imageStudioMaxInputImageB64     = 28 << 20 // ≈20 MiB decoded
	imageStudioPromptRuneLimit      = 8000
	imageStudioMessageRuneLimit     = 2000
	imageStudioTimeoutPerImage      = 12 * time.Minute
	// 超过最长执行时间仍未结束的任务视为实例重启遗留，按用户惰性标记失败（多副本安全）。
	imageStudioStaleAfter = imageStudioTimeoutPerImage*imageStudioMaxN + 5*time.Minute
	// 转存用独立超时：生成可能已耗尽任务超时，而上游已计费，图片不能因此丢失。
	imageStudioSaveTimeout = 2 * time.Minute
	// Stop 取消在途任务后等待其落库的上限（整体清理预算为 10s）。
	imageStudioShutdownWait = 5 * time.Second
	// 等待超时后 Stop 代为把仍在途的任务落库为 interrupted 的上限。
	imageStudioShutdownWriteBack = 3 * time.Second

	imageStudioInterruptedMessage = "interrupted: the server shut down before the job finished"
)

var (
	ErrImageStudioUnavailable = infraerrors.New(http.StatusNotFound, "IMAGE_STUDIO_UNAVAILABLE", "image studio requires image object storage to be enabled")
	ErrImageStudioNotFound    = infraerrors.NotFound("IMAGE_STUDIO_NOT_FOUND", "image studio job or asset not found")
	ErrImageStudioBusy        = infraerrors.TooManyRequests("IMAGE_STUDIO_BUSY", "too many unfinished image studio jobs")
	ErrImageStudioJobRunning  = infraerrors.Conflict("IMAGE_STUDIO_JOB_RUNNING", "image studio job is still running")

	imageStudioInputImagePattern = regexp.MustCompile(`^data:image/(png|jpeg|webp);base64,`)
)

type ImageStudioJob struct {
	ID           int64              `json:"id"`
	UserID       int64              `json:"-"`
	APIKeyID     int64              `json:"api_key_id"`
	Status       string             `json:"status"`
	Kind         string             `json:"kind"`
	Model        string             `json:"model"`
	Prompt       string             `json:"prompt"`
	Params       json.RawMessage    `json:"params"`
	ErrorMessage string             `json:"error_message,omitempty"`
	Warning      string             `json:"warning,omitempty"`
	ImageCount   int                `json:"image_count"`
	DurationMs   *int64             `json:"duration_ms,omitempty"`
	CreatedAt    time.Time          `json:"created_at"`
	StartedAt    *time.Time         `json:"started_at,omitempty"`
	CompletedAt  *time.Time         `json:"completed_at,omitempty"`
	Assets       []ImageStudioAsset `json:"assets,omitempty"`
}

type ImageStudioAsset struct {
	ID            int64     `json:"id"`
	JobID         int64     `json:"job_id"`
	UserID        int64     `json:"-"`
	StorageKey    string    `json:"-"`
	MimeType      string    `json:"mime_type"`
	Bytes         int64     `json:"bytes"`
	RevisedPrompt string    `json:"revised_prompt,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

type ImageStudioRepository interface {
	CreateJob(ctx context.Context, job *ImageStudioJob) error
	// CreateJobWithinLimit 原子地计数并插入：用户已有 limit 个未结束任务时返回 ErrImageStudioBusy。
	CreateJobWithinLimit(ctx context.Context, job *ImageStudioJob, limit int) error
	FailStaleJobs(ctx context.Context, userID int64, before time.Time, message string) error
	CountUnfinishedJobs(ctx context.Context, userID int64) (int, error)
	MarkJobRunning(ctx context.Context, id int64) error
	FinishJob(ctx context.Context, job *ImageStudioJob, assets []ImageStudioAsset) error
	GetJob(ctx context.Context, userID, id int64) (*ImageStudioJob, error)
	ListJobs(ctx context.Context, userID int64, page, pageSize int) ([]ImageStudioJob, int64, error)
	// DeleteJob 只删除已结束的任务（进行中返回 ErrImageStudioJobRunning），返回其资产的存储 key。
	DeleteJob(ctx context.Context, userID, id int64) ([]string, error)
	ListAssets(ctx context.Context, userID int64, page, pageSize int) ([]ImageStudioAsset, int64, error)
	GetAsset(ctx context.Context, userID, id int64) (*ImageStudioAsset, error)
	DeleteAsset(ctx context.Context, userID, id int64) (string, error)
	// DeleteExpiredJobs 删除 before 之前创建的已结束任务（最多 limit 个），返回其资产 key 与删除数。
	DeleteExpiredJobs(ctx context.Context, before time.Time, limit int) ([]string, int, error)
}

type ImageStudioCreateInput struct {
	APIKeyID     int64    `json:"api_key_id"`
	Model        string   `json:"model"`
	Prompt       string   `json:"prompt"`
	Size         string   `json:"size"`
	Quality      string   `json:"quality"`
	OutputFormat string   `json:"output_format"`
	Background   string   `json:"background"`
	N            int      `json:"n"`
	InputImages  []string `json:"input_images"`
}

// ImageStudioSubmission 是已落库、待执行的任务；APIKey 仅在进程内用于回放，绝不序列化。
type ImageStudioSubmission struct {
	Job    *ImageStudioJob
	APIKey string
	Path   string
	Body   []byte
}

// ImageStudioExecutor 经网关执行一次生图请求，返回 HTTP 状态与响应体。
type ImageStudioExecutor func(ctx context.Context, path, apiKey string, body []byte) (int, []byte)

type imageStudioKeyLookup interface {
	GetByID(ctx context.Context, id int64) (*APIKey, error)
}

type ImageStudioService struct {
	repo      ImageStudioRepository
	apiKeys   imageStudioKeyLookup
	resolve   ImageStorageResolver
	retention func(ctx context.Context) time.Duration

	stopOnce sync.Once
	stop     chan struct{}
	done     chan struct{}

	// 在途任务运行在服务级上下文下：Stop 取消它并有限等待任务落库为 interrupted。
	runMu     sync.Mutex
	runCtx    context.Context
	runCancel context.CancelFunc
	runs      sync.WaitGroup
	active    map[*imageStudioRun]struct{}
}

// imageStudioRun 是一个在途任务；claimed 表示写回权已被 Run 或 Stop 取得（受 runMu 保护）。
type imageStudioRun struct {
	jobID   int64
	started time.Time
	claimed bool
}

func NewImageStudioService(repo ImageStudioRepository, apiKeys *APIKeyService, settings *ImageStorageSettingService) *ImageStudioService {
	return &ImageStudioService{repo: repo, apiKeys: apiKeys, resolve: settings.Resolver(), retention: settings.ImageStudioRetention}
}

// ProvideImageStudioService 构造服务并启动保留期清理循环。
func ProvideImageStudioService(repo ImageStudioRepository, apiKeys *APIKeyService, settings *ImageStorageSettingService) *ImageStudioService {
	s := NewImageStudioService(repo, apiKeys, settings)
	s.Start()
	return s
}

const (
	imageStudioCleanupInterval  = 30 * time.Minute
	imageStudioCleanupBatchSize = 200
	imageStudioCleanupMaxRounds = 10
)

// Start 启动保留期清理循环；多副本并行时由仓储的 SKIP LOCKED 保证互不重复处理。
func (s *ImageStudioService) Start() {
	if s == nil || s.repo == nil || s.stop != nil {
		return
	}
	s.stop, s.done = make(chan struct{}), make(chan struct{})
	go func() {
		defer close(s.done)
		ticker := time.NewTicker(imageStudioCleanupInterval)
		defer ticker.Stop()
		for {
			select {
			case <-s.stop:
				return
			case <-ticker.C:
				if _, err := s.RunRetentionOnce(context.Background(), time.Now()); err != nil {
					logger.L().Warn("image_studio.retention_cleanup_failed", zap.Error(err))
				}
			}
		}
	}()
}

func (s *ImageStudioService) Stop() {
	if s == nil {
		return
	}
	s.runMu.Lock()
	s.runContextLocked()
	s.runCancel()
	s.runMu.Unlock()
	finished := make(chan struct{})
	go func() {
		s.runs.Wait()
		close(finished)
	}()
	select {
	case <-finished:
	case <-time.After(imageStudioShutdownWait):
		// 网关回放的上游调用不响应取消，任务可能迟迟不返回：由服务代为落库 interrupted。
		logger.L().Warn("image_studio.shutdown_wait_timeout")
		s.interruptUnclaimedRuns()
	}

	if s.stop == nil {
		return
	}
	s.stopOnce.Do(func() { close(s.stop) })
	<-s.done
}

// runContextLocked 惰性创建服务级任务上下文；调用方须持有 runMu。
func (s *ImageStudioService) runContextLocked() context.Context {
	if s.runCtx == nil {
		s.runCtx, s.runCancel = context.WithCancel(context.Background())
	}
	return s.runCtx
}

// trackRun 登记一个在途任务并返回服务级上下文与完成回调。停机后不再登记（避免与
// Stop 的 Wait 并发 Add），任务拿到已取消的上下文，快速收尾为 interrupted。
func (s *ImageStudioService) trackRun(jobID int64, started time.Time) (context.Context, *imageStudioRun, func()) {
	s.runMu.Lock()
	defer s.runMu.Unlock()
	ctx := s.runContextLocked()
	run := &imageStudioRun{jobID: jobID, started: started}
	if ctx.Err() != nil {
		return ctx, run, func() {}
	}
	if s.active == nil {
		s.active = make(map[*imageStudioRun]struct{})
	}
	s.active[run] = struct{}{}
	s.runs.Add(1)
	return ctx, run, s.runs.Done
}

// claimRun 取得任务的写回权；Stop 已代为落库 interrupted 时返回 false。
func (s *ImageStudioService) claimRun(run *imageStudioRun) bool {
	s.runMu.Lock()
	defer s.runMu.Unlock()
	if run.claimed {
		return false
	}
	run.claimed = true
	delete(s.active, run)
	return true
}

// interruptUnclaimedRuns 取走仍未写回的在途任务的写回权并落库为 interrupted，迟到的结果不再覆盖。
func (s *ImageStudioService) interruptUnclaimedRuns() {
	s.runMu.Lock()
	pending := make([]*imageStudioRun, 0, len(s.active))
	for run := range s.active {
		run.claimed = true
		pending = append(pending, run)
	}
	s.active = nil
	s.runMu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), imageStudioShutdownWriteBack)
	defer cancel()
	for _, run := range pending {
		duration := time.Since(run.started).Milliseconds()
		job := &ImageStudioJob{ID: run.jobID, Status: ImageStudioStatusFailed, ErrorMessage: imageStudioInterruptedMessage, DurationMs: &duration}
		if err := s.repo.FinishJob(ctx, job, nil); err != nil {
			logger.L().Error("image_studio.finish_job_failed", zap.Int64("job_id", run.jobID), zap.Error(err))
		}
	}
}

// RunRetentionOnce 删除超过保留期的已结束任务（级联资产）并尽力清理对象，返回删除的任务数。
// 对象存储关闭时跳过，避免只删行不删对象而遗留孤儿文件。
func (s *ImageStudioService) RunRetentionOnce(ctx context.Context, now time.Time) (int, error) {
	if s.retention == nil {
		return 0, nil
	}
	retention := s.retention(ctx)
	if retention <= 0 || !s.Enabled() {
		return 0, nil
	}
	total := 0
	for round := 0; round < imageStudioCleanupMaxRounds; round++ {
		keys, deleted, err := s.repo.DeleteExpiredJobs(ctx, now.Add(-retention), imageStudioCleanupBatchSize)
		if err != nil {
			return total, err
		}
		s.deleteObjects(ctx, keys...)
		total += deleted
		if deleted < imageStudioCleanupBatchSize {
			break
		}
	}
	return total, nil
}

func (s *ImageStudioService) store() (*ImageResultUploader, ImageObjectStore, bool) {
	if s == nil || s.resolve == nil {
		return nil, nil, false
	}
	uploader, enabled := s.resolve()
	if !enabled || uploader == nil {
		return nil, nil, false
	}
	objects, ok := uploader.Storage().(ImageObjectStore)
	return uploader, objects, ok
}

// Enabled 表示对象存储已启用且支持读取/删除。
func (s *ImageStudioService) Enabled() bool {
	_, _, ok := s.store()
	return ok
}

// Submit 校验输入与 Key 归属、执行每用户并发限制并落库一个 queued 任务。
func (s *ImageStudioService) Submit(ctx context.Context, userID int64, in ImageStudioCreateInput) (*ImageStudioSubmission, error) {
	if !s.Enabled() {
		return nil, ErrImageStudioUnavailable
	}
	if err := normalizeImageStudioInput(&in); err != nil {
		return nil, err
	}
	apiKey, err := s.apiKeys.GetByID(ctx, in.APIKeyID)
	if err != nil || apiKey == nil || apiKey.UserID != userID {
		return nil, infraerrors.NotFound("API_KEY_NOT_FOUND", "API key not found")
	}
	if platform := imageStudioKeyPlatform(apiKey); platform != PlatformOpenAI && platform != PlatformGrok {
		return nil, infraerrors.BadRequest("IMAGE_STUDIO_PLATFORM_UNSUPPORTED", "image studio supports OpenAI and Grok groups only")
	}

	if err := s.repo.FailStaleJobs(ctx, userID, time.Now().Add(-imageStudioStaleAfter), "interrupted: the job did not finish (server restart)"); err != nil {
		return nil, err
	}
	unfinished, err := s.repo.CountUnfinishedJobs(ctx, userID)
	if err != nil {
		return nil, err
	}
	if unfinished >= imageStudioMaxUnfinishedPerUser {
		return nil, ErrImageStudioBusy
	}

	kind, path := ImageStudioKindGenerate, "/v1/images/generations"
	payload := map[string]any{"model": in.Model, "prompt": in.Prompt, "n": in.N, "response_format": "b64_json"}
	params := map[string]any{"n": in.N}
	for field, value := range map[string]string{"size": in.Size, "quality": in.Quality, "output_format": in.OutputFormat, "background": in.Background} {
		if value != "" {
			payload[field], params[field] = value, value
		}
	}
	if len(in.InputImages) > 0 {
		kind, path = ImageStudioKindEdit, "/v1/images/edits"
		images := make([]map[string]string, 0, len(in.InputImages))
		for _, image := range in.InputImages {
			images = append(images, map[string]string{"image_url": image})
		}
		payload["images"] = images
		params["input_image_count"] = len(in.InputImages)
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	paramsJSON, _ := json.Marshal(params)

	job := &ImageStudioJob{UserID: userID, APIKeyID: apiKey.ID, Status: ImageStudioStatusQueued, Kind: kind, Model: in.Model, Prompt: in.Prompt, Params: paramsJSON}
	// 上面的计数只是快速拒绝；并发提交由仓储在同一事务内复核上限。
	if err := s.repo.CreateJobWithinLimit(ctx, job, imageStudioMaxUnfinishedPerUser); err != nil {
		return nil, err
	}
	return &ImageStudioSubmission{Job: job, APIKey: apiKey.Key, Path: path, Body: body}, nil
}

// Run 执行任务并把结果写回；调用方应在独立 goroutine 中调用。
func (s *ImageStudioService) Run(sub *ImageStudioSubmission, execute ImageStudioExecutor) {
	job := sub.Job
	started := time.Now()
	runCtx, run, runDone := s.trackRun(job.ID, started)
	defer runDone()
	n := int(gjson.GetBytes(job.Params, "n").Int())
	if n <= 0 {
		n = 1
	}
	ctx, cancel := context.WithTimeout(runCtx, imageStudioTimeoutPerImage*time.Duration(n))
	defer cancel()

	if err := s.repo.MarkJobRunning(ctx, job.ID); err != nil {
		logger.L().Warn("image_studio.mark_running_failed", zap.Int64("job_id", job.ID), zap.Error(err))
	}

	var assets []ImageStudioAsset
	status, respBody := execute(ctx, sub.Path, sub.APIKey, sub.Body)
	if !s.claimRun(run) {
		// Stop 等待超时后已代为落库 interrupted，迟到的结果不再覆盖。
		return
	}
	succeeded := status >= http.StatusOK && status < http.StatusMultipleChoices
	switch {
	case runCtx.Err() != nil && !succeeded:
		job.Status, job.ErrorMessage = ImageStudioStatusFailed, imageStudioInterruptedMessage
	case ctx.Err() != nil && len(respBody) == 0:
		job.Status, job.ErrorMessage = ImageStudioStatusFailed, "image generation timed out"
	case !succeeded:
		job.Status, job.ErrorMessage = ImageStudioStatusFailed, imageStudioErrorMessage(status, respBody)
	default:
		saveCtx, saveCancel := context.WithTimeout(context.Background(), imageStudioSaveTimeout)
		assets = s.saveAssets(saveCtx, job, respBody)
		saveCancel()
	}
	job.ImageCount = len(assets)
	job.ErrorMessage = truncateRunes(job.ErrorMessage, imageStudioMessageRuneLimit)
	job.Warning = truncateRunes(job.Warning, imageStudioMessageRuneLimit)
	duration := time.Since(started).Milliseconds()
	job.DurationMs = &duration

	// 用独立上下文落库，避免任务超时后状态卡在 running。
	finishCtx, finishCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer finishCancel()
	if err := s.repo.FinishJob(finishCtx, job, assets); err != nil {
		logger.L().Error("image_studio.finish_job_failed", zap.Int64("job_id", job.ID), zap.Error(err))
	}
}

// saveAssets 转存图片并设置任务状态；部分成功保留已存图片并记录警告。
// 计费语义沿用网关：上游成功即计费，即使转存失败（见规格第 5 节）。
func (s *ImageStudioService) saveAssets(ctx context.Context, job *ImageStudioJob, respBody []byte) []ImageStudioAsset {
	uploader, _, ok := s.store()
	if !ok {
		job.Status, job.ErrorMessage = ImageStudioStatusFailed, "storage_failed: image storage is no longer available"
		return nil
	}
	stored, err := uploader.SaveImages(ctx, fmt.Sprintf("studio/%d/%d", job.UserID, job.ID), respBody)
	assets := make([]ImageStudioAsset, 0, len(stored))
	for _, image := range stored {
		assets = append(assets, ImageStudioAsset{JobID: job.ID, UserID: job.UserID, StorageKey: image.Key, MimeType: image.ContentType, Bytes: image.Bytes, RevisedPrompt: image.RevisedPrompt})
	}
	switch {
	case err != nil && len(assets) == 0:
		job.Status, job.ErrorMessage = ImageStudioStatusFailed, "storage_failed: "+err.Error()
	case err != nil:
		job.Status, job.Warning = ImageStudioStatusSucceeded, "storage_failed: "+err.Error()
	case len(assets) == 0:
		job.Status, job.ErrorMessage = ImageStudioStatusFailed, "upstream returned no images"
	default:
		job.Status = ImageStudioStatusSucceeded
	}
	return assets
}

func (s *ImageStudioService) ListJobs(ctx context.Context, userID int64, page, pageSize int) ([]ImageStudioJob, int64, error) {
	return s.repo.ListJobs(ctx, userID, page, pageSize)
}

func (s *ImageStudioService) GetJob(ctx context.Context, userID, id int64) (*ImageStudioJob, error) {
	return s.repo.GetJob(ctx, userID, id)
}

func (s *ImageStudioService) ListAssets(ctx context.Context, userID int64, page, pageSize int) ([]ImageStudioAsset, int64, error) {
	return s.repo.ListAssets(ctx, userID, page, pageSize)
}

func (s *ImageStudioService) DeleteJob(ctx context.Context, userID, id int64) error {
	keys, err := s.repo.DeleteJob(ctx, userID, id)
	if err != nil {
		return err
	}
	s.deleteObjects(ctx, keys...)
	return nil
}

func (s *ImageStudioService) DeleteAsset(ctx context.Context, userID, id int64) error {
	key, err := s.repo.DeleteAsset(ctx, userID, id)
	if err != nil {
		return err
	}
	s.deleteObjects(ctx, key)
	return nil
}

// OpenAsset 校验归属后打开对象；调用方负责关闭 body。
func (s *ImageStudioService) OpenAsset(ctx context.Context, userID, id int64) (*ImageStudioAsset, io.ReadCloser, error) {
	asset, err := s.repo.GetAsset(ctx, userID, id)
	if err != nil {
		return nil, nil, err
	}
	_, objects, ok := s.store()
	if !ok {
		return nil, nil, ErrImageStudioUnavailable
	}
	body, _, err := objects.Open(ctx, asset.StorageKey)
	if err != nil {
		return nil, nil, err
	}
	return asset, body, nil
}

// deleteObjects 在数据库行删除后尽力清理对象；失败只记日志（对象存储生命周期规则兜底）。
func (s *ImageStudioService) deleteObjects(ctx context.Context, keys ...string) {
	_, objects, ok := s.store()
	if !ok {
		return
	}
	for _, key := range keys {
		if err := objects.Delete(ctx, key); err != nil {
			logger.L().Warn("image_studio.delete_object_failed", zap.String("key", key), zap.Error(err))
		}
	}
}

func normalizeImageStudioInput(in *ImageStudioCreateInput) error {
	in.Model = strings.TrimSpace(in.Model)
	in.Prompt = strings.TrimSpace(in.Prompt)
	in.Size = strings.TrimSpace(in.Size)
	in.Quality = strings.TrimSpace(in.Quality)
	in.OutputFormat = strings.TrimSpace(in.OutputFormat)
	in.Background = strings.TrimSpace(in.Background)
	if in.N == 0 {
		in.N = 1
	}
	switch {
	case in.APIKeyID <= 0:
		return infraerrors.BadRequest("IMAGE_STUDIO_INVALID", "api_key_id is required")
	case in.Model == "" || len(in.Model) > 128:
		return infraerrors.BadRequest("IMAGE_STUDIO_INVALID", "model is required")
	case in.Prompt == "" || utf8.RuneCountInString(in.Prompt) > imageStudioPromptRuneLimit:
		return infraerrors.BadRequest("IMAGE_STUDIO_INVALID", "prompt must be non-empty and at most 8000 characters")
	case in.N < 1 || in.N > imageStudioMaxN:
		return infraerrors.BadRequest("IMAGE_STUDIO_INVALID", "n must be between 1 and 4")
	case len(in.InputImages) > imageStudioMaxInputImages:
		return infraerrors.BadRequest("IMAGE_STUDIO_INVALID", "at most 4 input images are allowed")
	}
	for _, field := range []string{in.Size, in.Quality, in.OutputFormat, in.Background} {
		if len(field) > 32 {
			return infraerrors.BadRequest("IMAGE_STUDIO_INVALID", "invalid image option")
		}
	}
	for _, image := range in.InputImages {
		if !imageStudioInputImagePattern.MatchString(image) || len(image) > imageStudioMaxInputImageB64 {
			return infraerrors.BadRequest("IMAGE_STUDIO_INVALID", "input images must be png/jpeg/webp data URLs of at most 20 MB")
		}
	}
	return nil
}

func imageStudioKeyPlatform(apiKey *APIKey) string {
	if apiKey == nil || apiKey.Group == nil {
		return ""
	}
	return apiKey.Group.Platform
}

func imageStudioErrorMessage(status int, body []byte) string {
	if message := strings.TrimSpace(gjson.GetBytes(body, "error.message").String()); message != "" {
		return message
	}
	if message := strings.TrimSpace(gjson.GetBytes(body, "message").String()); message != "" {
		return message
	}
	return fmt.Sprintf("image generation failed (HTTP %d)", status)
}

func truncateRunes(value string, limit int) string {
	if utf8.RuneCountInString(value) <= limit {
		return value
	}
	return string([]rune(value)[:limit])
}
