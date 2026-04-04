package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

var (
	// ErrSoraGenerationConcurrencyLimit 表示用户进行中的任务数超限。
	ErrSoraGenerationConcurrencyLimit = errors.New("sora generation concurrent limit exceeded")
	// ErrSoraGenerationStateConflict 表示状态已发生变化（例如任务已取消）。
	ErrSoraGenerationStateConflict = errors.New("sora generation state conflict")
	// ErrSoraGenerationNotActive 表示任务不在可取消状态。
	ErrSoraGenerationNotActive = errors.New("sora generation is not active")
)

const soraGenerationActiveLimit = 3

type soraGenerationRepoAtomicCreator interface {
	CreatePendingWithLimit(ctx context.Context, gen *SoraGeneration, activeStatuses []string, maxActive int64) error
}

type soraGenerationRepoConditionalUpdater interface {
	UpdateGeneratingIfPending(ctx context.Context, id int64, upstreamTaskID string) (bool, error)
	UpdateCompletedIfActive(ctx context.Context, id int64, mediaURL string, mediaURLs []string, storageType string, s3Keys []string, fileSizeBytes int64, completedAt time.Time) (bool, error)
	UpdateFailedIfActive(ctx context.Context, id int64, errMsg string, completedAt time.Time) (bool, error)
	UpdateCancelledIfActive(ctx context.Context, id int64, completedAt time.Time) (bool, error)
	UpdateStorageIfCompleted(ctx context.Context, id int64, mediaURL string, mediaURLs []string, storageType string, s3Keys []string, fileSizeBytes int64) (bool, error)
}

// SoraGenerationService 管理 Sora 客户端的生成记录 CRUD。
type SoraGenerationService struct {
	genRepo      SoraGenerationRepository
	s3Storage    *SoraS3Storage
	quotaService *SoraQuotaService
}

// NewSoraGenerationService 创建生成记录服务。
func NewSoraGenerationService(
	genRepo SoraGenerationRepository,
	s3Storage *SoraS3Storage,
	quotaService *SoraQuotaService,
) *SoraGenerationService {
	return &SoraGenerationService{
		genRepo:      genRepo,
		s3Storage:    s3Storage,
		quotaService: quotaService,
	}
}

// CreatePending 创建一条 pending 状态的生成记录。
func (s *SoraGenerationService) CreatePending(ctx context.Context, userID int64, apiKeyID *int64, model, prompt, mediaType string) (*SoraGeneration, error) {
	gen := &SoraGeneration{
		UserID:      userID,
		APIKeyID:    apiKeyID,
		Model:       model,
		Prompt:      prompt,
		MediaType:   mediaType,
		Status:      SoraGenStatusPending,
		StorageType: SoraStorageTypeNone,
	}
	if atomicCreator, ok := s.genRepo.(soraGenerationRepoAtomicCreator); ok {
		if err := atomicCreator.CreatePendingWithLimit(
			ctx,
			gen,
			[]string{SoraGenStatusPending, SoraGenStatusGenerating},
			soraGenerationActiveLimit,
		); err != nil {
			if errors.Is(err, ErrSoraGenerationConcurrencyLimit) {
				return nil, err
			}
			return nil, fmt.Errorf("create generation: %w", err)
		}
		logger.LegacyPrintf("service.sora_gen", "[SoraGen] 创建记录 id=%d user=%d model=%s", gen.ID, userID, model)
		return gen, nil
	}

	if err := s.genRepo.Create(ctx, gen); err != nil {
		return nil, fmt.Errorf("create generation: %w", err)
	}
	logger.LegacyPrintf("service.sora_gen", "[SoraGen] 创建记录 id=%d user=%d model=%s", gen.ID, userID, model)
	return gen, nil
}

// MarkGenerating 标记为生成中。
func (s *SoraGenerationService) MarkGenerating(ctx context.Context, id int64, upstreamTaskID string) error {
	if updater, ok := s.genRepo.(soraGenerationRepoConditionalUpdater); ok {
		updated, err := updater.UpdateGeneratingIfPending(ctx, id, upstreamTaskID)
		if err != nil {
			return err
		}
		if !updated {
			return ErrSoraGenerationStateConflict
		}
		return nil
	}

	gen, err := s.genRepo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if gen.Status != SoraGenStatusPending {
		return ErrSoraGenerationStateConflict
	}
	gen.Status = SoraGenStatusGenerating
	gen.UpstreamTaskID = upstreamTaskID
	return s.genRepo.Update(ctx, gen)
}

// MarkCompleted 标记为已完成。
func (s *SoraGenerationService) MarkCompleted(ctx context.Context, id int64, mediaURL string, mediaURLs []string, storageType string, s3Keys []string, fileSizeBytes int64) error {
	now := time.Now()
	if updater, ok := s.genRepo.(soraGenerationRepoConditionalUpdater); ok {
		updated, err := updater.UpdateCompletedIfActive(ctx, id, mediaURL, mediaURLs, storageType, s3Keys, fileSizeBytes, now)
		if err != nil {
			return err
		}
		if !updated {
			return ErrSoraGenerationStateConflict
		}
		return nil
	}

	gen, err := s.genRepo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if gen.Status != SoraGenStatusPending && gen.Status != SoraGenStatusGenerating {
		return ErrSoraGenerationStateConflict
	}
	gen.Status = SoraGenStatusCompleted
	gen.MediaURL = mediaURL
	gen.MediaURLs = mediaURLs
	gen.StorageType = storageType
	gen.S3ObjectKeys = s3Keys
	gen.FileSizeBytes = fileSizeBytes
	gen.CompletedAt = &now
	return s.genRepo.Update(ctx, gen)
}

// MarkFailed 标记为失败。
func (s *SoraGenerationService) MarkFailed(ctx context.Context, id int64, errMsg string) error {
	now := time.Now()
	if updater, ok := s.genRepo.(soraGenerationRepoConditionalUpdater); ok {
		updated, err := updater.UpdateFailedIfActive(ctx, id, errMsg, now)
		if err != nil {
			return err
		}
		if !updated {
			return ErrSoraGenerationStateConflict
		}
		return nil
	}

	gen, err := s.genRepo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if gen.Status != SoraGenStatusPending && gen.Status != SoraGenStatusGenerating {
		return ErrSoraGenerationStateConflict
	}
	gen.Status = SoraGenStatusFailed
	gen.ErrorMessage = errMsg
	gen.CompletedAt = &now
	return s.genRepo.Update(ctx, gen)
}

// MarkCancelled 标记为已取消。
func (s *SoraGenerationService) MarkCancelled(ctx context.Context, id int64) error {
	now := time.Now()
	if updater, ok := s.genRepo.(soraGenerationRepoConditionalUpdater); ok {
		updated, err := updater.UpdateCancelledIfActive(ctx, id, now)
		if err != nil {
			return err
		}
		if !updated {
			return ErrSoraGenerationNotActive
		}
		return nil
	}

	gen, err := s.genRepo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if gen.Status != SoraGenStatusPending && gen.Status != SoraGenStatusGenerating {
		return ErrSoraGenerationNotActive
	}
	gen.Status = SoraGenStatusCancelled
	gen.CompletedAt = &now
	return s.genRepo.Update(ctx, gen)
}

// UpdateStorageForCompleted 更新已完成记录的存储信息（不重置 completed_at）。
func (s *SoraGenerationService) UpdateStorageForCompleted(
	ctx context.Context,
	id int64,
	mediaURL string,
	mediaURLs []string,
	storageType string,
	s3Keys []string,
	fileSizeBytes int64,
) error {
	if updater, ok := s.genRepo.(soraGenerationRepoConditionalUpdater); ok {
		updated, err := updater.UpdateStorageIfCompleted(ctx, id, mediaURL, mediaURLs, storageType, s3Keys, fileSizeBytes)
		if err != nil {
			return err
		}
		if !updated {
			return ErrSoraGenerationStateConflict
		}
		return nil
	}

	gen, err := s.genRepo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if gen.Status != SoraGenStatusCompleted {
		return ErrSoraGenerationStateConflict
	}
	gen.MediaURL = mediaURL
	gen.MediaURLs = mediaURLs
	gen.StorageType = storageType
	gen.S3ObjectKeys = s3Keys
	gen.FileSizeBytes = fileSizeBytes
	return s.genRepo.Update(ctx, gen)
}

// GetByID 获取记录详情（含权限校验）。
func (s *SoraGenerationService) GetByID(ctx context.Context, id, userID int64) (*SoraGeneration, error) {
	gen, err := s.genRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if gen.UserID != userID {
		return nil, fmt.Errorf("无权访问此生成记录")
	}
	return gen, nil
}

// List 查询生成记录列表（分页 + 筛选）。
func (s *SoraGenerationService) List(ctx context.Context, params SoraGenerationListParams) ([]*SoraGeneration, int64, error) {
	if params.Page <= 0 {
		params.Page = 1
	}
	if params.PageSize <= 0 {
		params.PageSize = 20
	}
	if params.PageSize > 100 {
		params.PageSize = 100
	}
	return s.genRepo.List(ctx, params)
}

// Delete 删除记录（联动 S3/本地文件清理 + 配额释放）。
func (s *SoraGenerationService) Delete(ctx context.Context, id, userID int64) error {
	gen, err := s.genRepo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if gen.UserID != userID {
		return fmt.Errorf("无权删除此生成记录")
	}

	// 清理 S3 文件
	if gen.StorageType == SoraStorageTypeS3 && len(gen.S3ObjectKeys) > 0 && s.s3Storage != nil {
		if err := s.s3Storage.DeleteObjects(ctx, gen.S3ObjectKeys); err != nil {
			logger.LegacyPrintf("service.sora_gen", "[SoraGen] S3 清理失败 id=%d err=%v", id, err)
		}
	}

	// 释放配额（S3/本地均释放）
	if gen.FileSizeBytes > 0 && (gen.StorageType == SoraStorageTypeS3 || gen.StorageType == SoraStorageTypeLocal) && s.quotaService != nil {
		if err := s.quotaService.ReleaseUsage(ctx, userID, gen.FileSizeBytes); err != nil {
			logger.LegacyPrintf("service.sora_gen", "[SoraGen] 配额释放失败 id=%d err=%v", id, err)
		}
	}

	return s.genRepo.Delete(ctx, id)
}

// CountActiveByUser 统计用户进行中的任务数（用于并发限制）。
func (s *SoraGenerationService) CountActiveByUser(ctx context.Context, userID int64) (int64, error) {
	return s.genRepo.CountByUserAndStatus(ctx, userID, []string{SoraGenStatusPending, SoraGenStatusGenerating})
}

// ResolveMediaURLs 为 S3 记录动态生成预签名 URL。
func (s *SoraGenerationService) ResolveMediaURLs(ctx context.Context, gen *SoraGeneration) error {
	if gen == nil || gen.StorageType != SoraStorageTypeS3 || s.s3Storage == nil {
		return nil
	}
	if len(gen.S3ObjectKeys) == 0 {
		return nil
	}

	urls := make([]string, len(gen.S3ObjectKeys))
	var wg sync.WaitGroup
	var firstErr error
	var errMu sync.Mutex

	for idx, key := range gen.S3ObjectKeys {
		wg.Add(1)
		go func(i int, objectKey string) {
			defer wg.Done()
			url, err := s.s3Storage.GetAccessURL(ctx, objectKey)
			if err != nil {
				errMu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				errMu.Unlock()
				return
			}
			urls[i] = url
		}(idx, key)
	}
	wg.Wait()
	if firstErr != nil {
		return firstErr
	}

	gen.MediaURL = urls[0]
	gen.MediaURLs = urls

	return nil
}
