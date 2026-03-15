package service

import (
	"context"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

// SoraTaskWorker polls unfinished Sora tasks in the background.
// When a task completes, it downloads media to configured storage if available.
type SoraTaskWorker struct {
	taskService   *SoraTaskService
	accountRepo   AccountRepository
	objectStorage SoraObjectStorage
	mediaStorage  *SoraMediaStorage
	interval      time.Duration
	pollTimeout   time.Duration
	stopCh        chan struct{}
	stopOnce      sync.Once
	wg            sync.WaitGroup
}

func NewSoraTaskWorker(
	taskService *SoraTaskService,
	accountRepo AccountRepository,
	objectStorage SoraObjectStorage,
	mediaStorage *SoraMediaStorage,
	interval time.Duration,
) *SoraTaskWorker {
	if interval <= 0 {
		interval = 60 * time.Second
	}
	return &SoraTaskWorker{
		taskService:   taskService,
		accountRepo:   accountRepo,
		objectStorage: objectStorage,
		mediaStorage:  mediaStorage,
		interval:      interval,
		pollTimeout:   30 * time.Second,
		stopCh:        make(chan struct{}),
	}
}

func (w *SoraTaskWorker) Start() {
	if w == nil || w.taskService == nil {
		return
	}
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		ticker := time.NewTicker(w.interval)
		defer ticker.Stop()

		logger.LegacyPrintf("service.sora_task_worker", "[Start] polling interval=%s", w.interval)
		w.pollAll()

		for {
			select {
			case <-ticker.C:
				w.pollAll()
			case <-w.stopCh:
				return
			}
		}
	}()
}

func (w *SoraTaskWorker) Stop() {
	if w == nil {
		return
	}
	w.stopOnce.Do(func() {
		close(w.stopCh)
	})
	w.wg.Wait()
}

func (w *SoraTaskWorker) pollAll() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	tasks, err := w.taskService.ListPendingTasks(ctx)
	if err != nil {
		logger.LegacyPrintf("service.sora_task_worker", "[PollAll] list pending tasks error: %v", err)
		return
	}
	if len(tasks) == 0 {
		return
	}

	logger.LegacyPrintf("service.sora_task_worker", "[PollAll] found %d pending tasks", len(tasks))

	for _, task := range tasks {
		select {
		case <-w.stopCh:
			return
		default:
		}
		w.pollOne(ctx, task)
	}
}

func (w *SoraTaskWorker) pollOne(ctx context.Context, task *SoraTask) {
	pollCtx, cancel := context.WithTimeout(ctx, w.pollTimeout)
	defer cancel()

	account, err := w.accountRepo.GetByID(pollCtx, task.AccountID)
	if err != nil {
		logger.LegacyPrintf("service.sora_task_worker",
			"[PollOne] task=%s get account=%d error: %v", task.ID, task.AccountID, err)
		w.markTaskFailed(pollCtx, task, "account not found")
		return
	}

	if err := w.taskService.PollTask(pollCtx, task, account); err != nil {
		logger.LegacyPrintf("service.sora_task_worker",
			"[PollOne] task=%s poll error: %v", task.ID, err)
		return
	}

	logger.LegacyPrintf("service.sora_task_worker",
		"[PollOne] task=%s status=%s progress=%d", task.ID, task.Status, task.Progress)

	if task.Status == SoraTaskCompleted && task.VideoURL != "" && task.StoredKey == "" {
		w.tryStoreMedia(pollCtx, task)
	}
}

// tryStoreMedia downloads media to configured storage.
// Priority: S3 > local disk > keep upstream URL.
func (w *SoraTaskWorker) tryStoreMedia(ctx context.Context, task *SoraTask) {
	mediaType := "video"
	if task.ObjectType == SoraObjectImage {
		mediaType = "image"
	}

	if w.objectStorage != nil && w.objectStorage.Enabled(ctx) {
		key, _, storageType, err := w.objectStorage.UploadFromURL(ctx, 0, task.VideoURL)
		if err != nil {
			logger.LegacyPrintf("service.sora_task_worker",
				"[StoreMedia] task=%s object storage upload error: %v", task.ID, err)
		} else {
			task.StoredKey = key
			task.StorageType = storageType
			if updateErr := w.taskService.UpdateTask(ctx, task); updateErr != nil {
				logger.LegacyPrintf("service.sora_task_worker",
					"[StoreMedia] task=%s update stored_key error: %v", task.ID, updateErr)
			}
			return
		}
	}

	if w.mediaStorage != nil && w.mediaStorage.Enabled() {
		stored, err := w.mediaStorage.StoreFromURLs(ctx, mediaType, []string{task.VideoURL})
		if err != nil {
			logger.LegacyPrintf("service.sora_task_worker",
				"[StoreMedia] task=%s local storage error: %v", task.ID, err)
			return
		}
		if len(stored) > 0 && stored[0] != task.VideoURL {
			task.StoredKey = stored[0]
			task.StorageType = "local"
			if updateErr := w.taskService.UpdateTask(ctx, task); updateErr != nil {
				logger.LegacyPrintf("service.sora_task_worker",
					"[StoreMedia] task=%s update stored_key error: %v", task.ID, updateErr)
			}
		}
	}
}

func (w *SoraTaskWorker) markTaskFailed(ctx context.Context, task *SoraTask, message string) {
	now := time.Now()
	task.Status = SoraTaskFailed
	task.ErrorMessage = message
	task.ErrorType = "server_error"
	task.CompletedAt = &now
	if updateErr := w.taskService.UpdateTask(ctx, task); updateErr != nil {
		logger.LegacyPrintf("service.sora_task_worker",
			"[PollOne] task=%s update failed: %v", task.ID, updateErr)
	}
}
