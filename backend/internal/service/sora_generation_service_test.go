//go:build unit

package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/require"
)

// ==================== Stub: SoraGenerationRepository ====================

var _ SoraGenerationRepository = (*stubGenRepo)(nil)

type stubGenRepo struct {
	gens       map[int64]*SoraGeneration
	nextID     int64
	createErr  error
	getErr     error
	updateErr  error
	deleteErr  error
	listErr    error
	countErr   error
	countValue int64
}

func newStubGenRepo() *stubGenRepo {
	return &stubGenRepo{gens: make(map[int64]*SoraGeneration), nextID: 1}
}

func (r *stubGenRepo) Create(_ context.Context, gen *SoraGeneration) error {
	if r.createErr != nil {
		return r.createErr
	}
	gen.ID = r.nextID
	gen.CreatedAt = time.Now()
	r.nextID++
	r.gens[gen.ID] = gen
	return nil
}

func (r *stubGenRepo) GetByID(_ context.Context, id int64) (*SoraGeneration, error) {
	if r.getErr != nil {
		return nil, r.getErr
	}
	if gen, ok := r.gens[id]; ok {
		return gen, nil
	}
	return nil, fmt.Errorf("not found")
}

func (r *stubGenRepo) Update(_ context.Context, gen *SoraGeneration) error {
	if r.updateErr != nil {
		return r.updateErr
	}
	r.gens[gen.ID] = gen
	return nil
}

func (r *stubGenRepo) Delete(_ context.Context, id int64) error {
	if r.deleteErr != nil {
		return r.deleteErr
	}
	delete(r.gens, id)
	return nil
}

func (r *stubGenRepo) List(_ context.Context, params SoraGenerationListParams) ([]*SoraGeneration, int64, error) {
	if r.listErr != nil {
		return nil, 0, r.listErr
	}
	var result []*SoraGeneration
	for _, gen := range r.gens {
		if gen.UserID != params.UserID {
			continue
		}
		if params.Status != "" && gen.Status != params.Status {
			continue
		}
		if params.StorageType != "" && gen.StorageType != params.StorageType {
			continue
		}
		if params.MediaType != "" && gen.MediaType != params.MediaType {
			continue
		}
		result = append(result, gen)
	}
	return result, int64(len(result)), nil
}

func (r *stubGenRepo) CountByUserAndStatus(_ context.Context, userID int64, statuses []string) (int64, error) {
	if r.countErr != nil {
		return 0, r.countErr
	}
	if r.countValue > 0 {
		return r.countValue, nil
	}
	var count int64
	statusSet := make(map[string]struct{})
	for _, s := range statuses {
		statusSet[s] = struct{}{}
	}
	for _, gen := range r.gens {
		if gen.UserID == userID {
			if _, ok := statusSet[gen.Status]; ok {
				count++
			}
		}
	}
	return count, nil
}

func (r *stubGenRepo) CountByStorageType(_ context.Context, storageType string, statuses []string) (int64, error) {
	if r.countErr != nil {
		return 0, r.countErr
	}
	var count int64
	statusSet := make(map[string]struct{})
	for _, s := range statuses {
		statusSet[s] = struct{}{}
	}
	for _, gen := range r.gens {
		if gen.StorageType == storageType {
			if _, ok := statusSet[gen.Status]; ok {
				count++
			}
		}
	}
	return count, nil
}

// ==================== Stub: UserRepository (用于 SoraQuotaService) ====================

var _ UserRepository = (*stubUserRepoForQuota)(nil)

type stubUserRepoForQuota struct {
	users     map[int64]*User
	updateErr error
}

func newStubUserRepoForQuota() *stubUserRepoForQuota {
	return &stubUserRepoForQuota{users: make(map[int64]*User)}
}

func (r *stubUserRepoForQuota) GetByID(_ context.Context, id int64) (*User, error) {
	if u, ok := r.users[id]; ok {
		return u, nil
	}
	return nil, fmt.Errorf("user not found")
}
func (r *stubUserRepoForQuota) Update(_ context.Context, user *User) error {
	if r.updateErr != nil {
		return r.updateErr
	}
	r.users[user.ID] = user
	return nil
}
func (r *stubUserRepoForQuota) Create(context.Context, *User) error { return nil }
func (r *stubUserRepoForQuota) GetByEmail(context.Context, string) (*User, error) {
	return nil, nil
}
func (r *stubUserRepoForQuota) GetFirstAdmin(context.Context) (*User, error) { return nil, nil }
func (r *stubUserRepoForQuota) Delete(context.Context, int64) error          { return nil }
func (r *stubUserRepoForQuota) List(context.Context, pagination.PaginationParams) ([]User, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (r *stubUserRepoForQuota) ListWithFilters(context.Context, pagination.PaginationParams, UserListFilters) ([]User, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (r *stubUserRepoForQuota) UpdateBalance(context.Context, int64, float64) error { return nil }
func (r *stubUserRepoForQuota) DeductBalance(context.Context, int64, float64) error { return nil }
func (r *stubUserRepoForQuota) UpdateConcurrency(context.Context, int64, int) error { return nil }
func (r *stubUserRepoForQuota) ExistsByEmail(context.Context, string) (bool, error) {
	return false, nil
}
func (r *stubUserRepoForQuota) RemoveGroupFromAllowedGroups(context.Context, int64) (int64, error) {
	return 0, nil
}
func (r *stubUserRepoForQuota) UpdateTotpSecret(context.Context, int64, *string) error { return nil }
func (r *stubUserRepoForQuota) EnableTotp(context.Context, int64) error                { return nil }
func (r *stubUserRepoForQuota) DisableTotp(context.Context, int64) error               { return nil }
func (r *stubUserRepoForQuota) AddGroupToAllowedGroups(context.Context, int64, int64) error {
	return nil
}

// ==================== 辅助函数：构造带 CDN 缓存的 SoraS3Storage ====================

// newS3StorageWithCDN 创建一个预缓存了 CDN 配置的 SoraS3Storage，
// 避免实际初始化 AWS 客户端。用于测试 GetAccessURL 的 CDN 路径。
func newS3StorageWithCDN(cdnURL string) *SoraS3Storage {
	storage := &SoraS3Storage{}
	storage.cfg = &SoraS3Settings{
		Enabled: true,
		Bucket:  "test-bucket",
		CDNURL:  cdnURL,
	}
	// 需要 non-nil client 使 getClient 命中缓存
	storage.client = s3.New(s3.Options{})
	return storage
}

// newS3StorageFailingDelete 创建一个 settingService=nil 的 SoraS3Storage，
// 使 DeleteObjects 返回错误（无法获取配置）。用于测试 Delete 方法 S3 清理失败但仍继续的场景。
func newS3StorageFailingDelete() *SoraS3Storage {
	return &SoraS3Storage{} // settingService 为 nil → getConfig 返回 error
}

// ==================== CreatePending ====================

func TestCreatePending_Success(t *testing.T) {
	repo := newStubGenRepo()
	svc := NewSoraGenerationService(repo, nil, nil)

	gen, err := svc.CreatePending(context.Background(), 1, nil, "sora2-landscape-10s", "一只猫跳舞", "video")
	require.NoError(t, err)
	require.Equal(t, int64(1), gen.ID)
	require.Equal(t, int64(1), gen.UserID)
	require.Equal(t, "sora2-landscape-10s", gen.Model)
	require.Equal(t, "一只猫跳舞", gen.Prompt)
	require.Equal(t, "video", gen.MediaType)
	require.Equal(t, SoraGenStatusPending, gen.Status)
	require.Equal(t, SoraStorageTypeNone, gen.StorageType)
	require.Nil(t, gen.APIKeyID)
}

func TestCreatePending_WithAPIKeyID(t *testing.T) {
	repo := newStubGenRepo()
	svc := NewSoraGenerationService(repo, nil, nil)

	apiKeyID := int64(42)
	gen, err := svc.CreatePending(context.Background(), 1, &apiKeyID, "gpt-image", "画一朵花", "image")
	require.NoError(t, err)
	require.NotNil(t, gen.APIKeyID)
	require.Equal(t, int64(42), *gen.APIKeyID)
}

func TestCreatePending_RepoError(t *testing.T) {
	repo := newStubGenRepo()
	repo.createErr = fmt.Errorf("db write error")
	svc := NewSoraGenerationService(repo, nil, nil)

	gen, err := svc.CreatePending(context.Background(), 1, nil, "sora2-landscape-10s", "test", "video")
	require.Error(t, err)
	require.Nil(t, gen)
	require.Contains(t, err.Error(), "create generation")
}

// ==================== MarkGenerating ====================

func TestMarkGenerating_Success(t *testing.T) {
	repo := newStubGenRepo()
	repo.gens[1] = &SoraGeneration{ID: 1, UserID: 1, Status: SoraGenStatusPending}
	svc := NewSoraGenerationService(repo, nil, nil)

	err := svc.MarkGenerating(context.Background(), 1, "upstream-task-123")
	require.NoError(t, err)
	require.Equal(t, SoraGenStatusGenerating, repo.gens[1].Status)
	require.Equal(t, "upstream-task-123", repo.gens[1].UpstreamTaskID)
}

func TestMarkGenerating_NotFound(t *testing.T) {
	repo := newStubGenRepo()
	svc := NewSoraGenerationService(repo, nil, nil)

	err := svc.MarkGenerating(context.Background(), 999, "")
	require.Error(t, err)
}

func TestMarkGenerating_UpdateError(t *testing.T) {
	repo := newStubGenRepo()
	repo.gens[1] = &SoraGeneration{ID: 1, UserID: 1, Status: SoraGenStatusPending}
	repo.updateErr = fmt.Errorf("update failed")
	svc := NewSoraGenerationService(repo, nil, nil)

	err := svc.MarkGenerating(context.Background(), 1, "")
	require.Error(t, err)
}

// ==================== MarkCompleted ====================

func TestMarkCompleted_Success(t *testing.T) {
	repo := newStubGenRepo()
	repo.gens[1] = &SoraGeneration{ID: 1, UserID: 1, Status: SoraGenStatusGenerating}
	svc := NewSoraGenerationService(repo, nil, nil)

	err := svc.MarkCompleted(context.Background(), 1,
		"https://cdn.example.com/video.mp4",
		[]string{"https://cdn.example.com/video.mp4"},
		SoraStorageTypeS3,
		[]string{"sora/1/2024/01/01/uuid.mp4"},
		1048576,
	)
	require.NoError(t, err)
	gen := repo.gens[1]
	require.Equal(t, SoraGenStatusCompleted, gen.Status)
	require.Equal(t, "https://cdn.example.com/video.mp4", gen.MediaURL)
	require.Equal(t, []string{"https://cdn.example.com/video.mp4"}, gen.MediaURLs)
	require.Equal(t, SoraStorageTypeS3, gen.StorageType)
	require.Equal(t, []string{"sora/1/2024/01/01/uuid.mp4"}, gen.S3ObjectKeys)
	require.Equal(t, int64(1048576), gen.FileSizeBytes)
	require.NotNil(t, gen.CompletedAt)
}

func TestMarkCompleted_NotFound(t *testing.T) {
	repo := newStubGenRepo()
	svc := NewSoraGenerationService(repo, nil, nil)

	err := svc.MarkCompleted(context.Background(), 999, "", nil, "", nil, 0)
	require.Error(t, err)
}

func TestMarkCompleted_UpdateError(t *testing.T) {
	repo := newStubGenRepo()
	repo.gens[1] = &SoraGeneration{ID: 1, UserID: 1, Status: SoraGenStatusGenerating}
	repo.updateErr = fmt.Errorf("update failed")
	svc := NewSoraGenerationService(repo, nil, nil)

	err := svc.MarkCompleted(context.Background(), 1, "url", nil, SoraStorageTypeUpstream, nil, 0)
	require.Error(t, err)
}

// ==================== MarkFailed ====================

func TestMarkFailed_Success(t *testing.T) {
	repo := newStubGenRepo()
	repo.gens[1] = &SoraGeneration{ID: 1, UserID: 1, Status: SoraGenStatusGenerating}
	svc := NewSoraGenerationService(repo, nil, nil)

	err := svc.MarkFailed(context.Background(), 1, "上游返回 500 错误")
	require.NoError(t, err)
	gen := repo.gens[1]
	require.Equal(t, SoraGenStatusFailed, gen.Status)
	require.Equal(t, "上游返回 500 错误", gen.ErrorMessage)
	require.NotNil(t, gen.CompletedAt)
}

func TestMarkFailed_NotFound(t *testing.T) {
	repo := newStubGenRepo()
	svc := NewSoraGenerationService(repo, nil, nil)

	err := svc.MarkFailed(context.Background(), 999, "error")
	require.Error(t, err)
}

func TestMarkFailed_UpdateError(t *testing.T) {
	repo := newStubGenRepo()
	repo.gens[1] = &SoraGeneration{ID: 1, UserID: 1, Status: SoraGenStatusGenerating}
	repo.updateErr = fmt.Errorf("update failed")
	svc := NewSoraGenerationService(repo, nil, nil)

	err := svc.MarkFailed(context.Background(), 1, "err")
	require.Error(t, err)
}

// ==================== MarkCancelled ====================

func TestMarkCancelled_Pending(t *testing.T) {
	repo := newStubGenRepo()
	repo.gens[1] = &SoraGeneration{ID: 1, UserID: 1, Status: SoraGenStatusPending}
	svc := NewSoraGenerationService(repo, nil, nil)

	err := svc.MarkCancelled(context.Background(), 1)
	require.NoError(t, err)
	require.Equal(t, SoraGenStatusCancelled, repo.gens[1].Status)
	require.NotNil(t, repo.gens[1].CompletedAt)
}

func TestMarkCancelled_Generating(t *testing.T) {
	repo := newStubGenRepo()
	repo.gens[1] = &SoraGeneration{ID: 1, UserID: 1, Status: SoraGenStatusGenerating}
	svc := NewSoraGenerationService(repo, nil, nil)

	err := svc.MarkCancelled(context.Background(), 1)
	require.NoError(t, err)
	require.Equal(t, SoraGenStatusCancelled, repo.gens[1].Status)
}

func TestMarkCancelled_Completed(t *testing.T) {
	repo := newStubGenRepo()
	repo.gens[1] = &SoraGeneration{ID: 1, UserID: 1, Status: SoraGenStatusCompleted}
	svc := NewSoraGenerationService(repo, nil, nil)

	err := svc.MarkCancelled(context.Background(), 1)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrSoraGenerationNotActive)
}

func TestMarkCancelled_Failed(t *testing.T) {
	repo := newStubGenRepo()
	repo.gens[1] = &SoraGeneration{ID: 1, UserID: 1, Status: SoraGenStatusFailed}
	svc := NewSoraGenerationService(repo, nil, nil)

	err := svc.MarkCancelled(context.Background(), 1)
	require.Error(t, err)
}

func TestMarkCancelled_AlreadyCancelled(t *testing.T) {
	repo := newStubGenRepo()
	repo.gens[1] = &SoraGeneration{ID: 1, UserID: 1, Status: SoraGenStatusCancelled}
	svc := NewSoraGenerationService(repo, nil, nil)

	err := svc.MarkCancelled(context.Background(), 1)
	require.Error(t, err)
}

func TestMarkCancelled_NotFound(t *testing.T) {
	repo := newStubGenRepo()
	svc := NewSoraGenerationService(repo, nil, nil)

	err := svc.MarkCancelled(context.Background(), 999)
	require.Error(t, err)
}

func TestMarkCancelled_UpdateError(t *testing.T) {
	repo := newStubGenRepo()
	repo.gens[1] = &SoraGeneration{ID: 1, UserID: 1, Status: SoraGenStatusPending}
	repo.updateErr = fmt.Errorf("update failed")
	svc := NewSoraGenerationService(repo, nil, nil)

	err := svc.MarkCancelled(context.Background(), 1)
	require.Error(t, err)
}

// ==================== GetByID ====================

func TestGetByID_Success(t *testing.T) {
	repo := newStubGenRepo()
	repo.gens[1] = &SoraGeneration{ID: 1, UserID: 1, Status: SoraGenStatusCompleted, Model: "sora2-landscape-10s"}
	svc := NewSoraGenerationService(repo, nil, nil)

	gen, err := svc.GetByID(context.Background(), 1, 1)
	require.NoError(t, err)
	require.Equal(t, int64(1), gen.ID)
	require.Equal(t, "sora2-landscape-10s", gen.Model)
}

func TestGetByID_WrongUser(t *testing.T) {
	repo := newStubGenRepo()
	repo.gens[1] = &SoraGeneration{ID: 1, UserID: 2, Status: SoraGenStatusCompleted}
	svc := NewSoraGenerationService(repo, nil, nil)

	gen, err := svc.GetByID(context.Background(), 1, 1)
	require.Error(t, err)
	require.Nil(t, gen)
	require.Contains(t, err.Error(), "无权访问")
}

func TestGetByID_NotFound(t *testing.T) {
	repo := newStubGenRepo()
	svc := NewSoraGenerationService(repo, nil, nil)

	gen, err := svc.GetByID(context.Background(), 999, 1)
	require.Error(t, err)
	require.Nil(t, gen)
}

// ==================== List ====================

func TestList_Success(t *testing.T) {
	repo := newStubGenRepo()
	repo.gens[1] = &SoraGeneration{ID: 1, UserID: 1, Status: SoraGenStatusCompleted, MediaType: "video"}
	repo.gens[2] = &SoraGeneration{ID: 2, UserID: 1, Status: SoraGenStatusPending, MediaType: "image"}
	repo.gens[3] = &SoraGeneration{ID: 3, UserID: 2, Status: SoraGenStatusCompleted, MediaType: "video"}
	svc := NewSoraGenerationService(repo, nil, nil)

	gens, total, err := svc.List(context.Background(), SoraGenerationListParams{UserID: 1, Page: 1, PageSize: 20})
	require.NoError(t, err)
	require.Len(t, gens, 2) // 只有 userID=1 的
	require.Equal(t, int64(2), total)
}

func TestList_DefaultPagination(t *testing.T) {
	repo := newStubGenRepo()
	svc := NewSoraGenerationService(repo, nil, nil)

	// page=0, pageSize=0 → 应修正为 page=1, pageSize=20
	_, _, err := svc.List(context.Background(), SoraGenerationListParams{UserID: 1})
	require.NoError(t, err)
}

func TestList_MaxPageSize(t *testing.T) {
	repo := newStubGenRepo()
	svc := NewSoraGenerationService(repo, nil, nil)

	// pageSize > 100 → 应限制为 100
	_, _, err := svc.List(context.Background(), SoraGenerationListParams{UserID: 1, Page: 1, PageSize: 200})
	require.NoError(t, err)
}

func TestList_Error(t *testing.T) {
	repo := newStubGenRepo()
	repo.listErr = fmt.Errorf("db error")
	svc := NewSoraGenerationService(repo, nil, nil)

	_, _, err := svc.List(context.Background(), SoraGenerationListParams{UserID: 1})
	require.Error(t, err)
}

// ==================== Delete ====================

func TestDelete_Success(t *testing.T) {
	repo := newStubGenRepo()
	repo.gens[1] = &SoraGeneration{ID: 1, UserID: 1, Status: SoraGenStatusCompleted, StorageType: SoraStorageTypeUpstream}
	svc := NewSoraGenerationService(repo, nil, nil)

	err := svc.Delete(context.Background(), 1, 1)
	require.NoError(t, err)
	_, exists := repo.gens[1]
	require.False(t, exists)
}

func TestDelete_WrongUser(t *testing.T) {
	repo := newStubGenRepo()
	repo.gens[1] = &SoraGeneration{ID: 1, UserID: 2, Status: SoraGenStatusCompleted}
	svc := NewSoraGenerationService(repo, nil, nil)

	err := svc.Delete(context.Background(), 1, 1)
	require.Error(t, err)
	require.Contains(t, err.Error(), "无权删除")
}

func TestDelete_NotFound(t *testing.T) {
	repo := newStubGenRepo()
	svc := NewSoraGenerationService(repo, nil, nil)

	err := svc.Delete(context.Background(), 999, 1)
	require.Error(t, err)
}

func TestDelete_S3Cleanup_NilS3(t *testing.T) {
	repo := newStubGenRepo()
	repo.gens[1] = &SoraGeneration{ID: 1, UserID: 1, StorageType: SoraStorageTypeS3, S3ObjectKeys: []string{"key1"}}
	svc := NewSoraGenerationService(repo, nil, nil)

	err := svc.Delete(context.Background(), 1, 1)
	require.NoError(t, err) // objectStorage 为 nil，跳过清理
}

func TestDelete_QuotaRelease_NilQuota(t *testing.T) {
	repo := newStubGenRepo()
	repo.gens[1] = &SoraGeneration{ID: 1, UserID: 1, StorageType: SoraStorageTypeS3, FileSizeBytes: 1024}
	svc := NewSoraGenerationService(repo, nil, nil)

	err := svc.Delete(context.Background(), 1, 1)
	require.NoError(t, err) // quotaService 为 nil，跳过释放
}

func TestDelete_NonS3NoCleanup(t *testing.T) {
	repo := newStubGenRepo()
	repo.gens[1] = &SoraGeneration{ID: 1, UserID: 1, StorageType: SoraStorageTypeLocal, FileSizeBytes: 1024}
	svc := NewSoraGenerationService(repo, nil, nil)

	err := svc.Delete(context.Background(), 1, 1)
	require.NoError(t, err)
}

func TestDelete_DeleteRepoError(t *testing.T) {
	repo := newStubGenRepo()
	repo.gens[1] = &SoraGeneration{ID: 1, UserID: 1, StorageType: SoraStorageTypeUpstream}
	repo.deleteErr = fmt.Errorf("delete failed")
	svc := NewSoraGenerationService(repo, nil, nil)

	err := svc.Delete(context.Background(), 1, 1)
	require.Error(t, err)
}

// ==================== CountActiveByUser ====================

func TestCountActiveByUser_Success(t *testing.T) {
	repo := newStubGenRepo()
	repo.gens[1] = &SoraGeneration{ID: 1, UserID: 1, Status: SoraGenStatusPending}
	repo.gens[2] = &SoraGeneration{ID: 2, UserID: 1, Status: SoraGenStatusGenerating}
	repo.gens[3] = &SoraGeneration{ID: 3, UserID: 1, Status: SoraGenStatusCompleted} // 不算
	svc := NewSoraGenerationService(repo, nil, nil)

	count, err := svc.CountActiveByUser(context.Background(), 1)
	require.NoError(t, err)
	require.Equal(t, int64(2), count)
}

func TestCountActiveByUser_NoActive(t *testing.T) {
	repo := newStubGenRepo()
	repo.gens[1] = &SoraGeneration{ID: 1, UserID: 1, Status: SoraGenStatusCompleted}
	svc := NewSoraGenerationService(repo, nil, nil)

	count, err := svc.CountActiveByUser(context.Background(), 1)
	require.NoError(t, err)
	require.Equal(t, int64(0), count)
}

func TestCountActiveByUser_Error(t *testing.T) {
	repo := newStubGenRepo()
	repo.countErr = fmt.Errorf("db error")
	svc := NewSoraGenerationService(repo, nil, nil)

	_, err := svc.CountActiveByUser(context.Background(), 1)
	require.Error(t, err)
}

// ==================== ResolveMediaURLs ====================

func TestResolveMediaURLs_NilGen(t *testing.T) {
	svc := NewSoraGenerationService(newStubGenRepo(), nil, nil)
	require.NoError(t, svc.ResolveMediaURLs(context.Background(), nil))
}

func TestResolveMediaURLs_NonS3(t *testing.T) {
	svc := NewSoraGenerationService(newStubGenRepo(), nil, nil)
	gen := &SoraGeneration{StorageType: SoraStorageTypeUpstream, MediaURL: "https://original.com/v.mp4"}
	require.NoError(t, svc.ResolveMediaURLs(context.Background(), gen))
	require.Equal(t, "https://original.com/v.mp4", gen.MediaURL) // 不变
}

func TestResolveMediaURLs_S3NilStorage(t *testing.T) {
	svc := NewSoraGenerationService(newStubGenRepo(), nil, nil)
	gen := &SoraGeneration{StorageType: SoraStorageTypeS3, S3ObjectKeys: []string{"key1"}}
	require.NoError(t, svc.ResolveMediaURLs(context.Background(), gen))
}

func TestResolveMediaURLs_Local(t *testing.T) {
	svc := NewSoraGenerationService(newStubGenRepo(), nil, nil)
	gen := &SoraGeneration{StorageType: SoraStorageTypeLocal, MediaURL: "/video/2024/01/01/file.mp4"}
	require.NoError(t, svc.ResolveMediaURLs(context.Background(), gen))
	require.Equal(t, "/video/2024/01/01/file.mp4", gen.MediaURL) // 不变
}

// ==================== 状态流转完整测试 ====================

func TestStatusTransition_PendingToCompletedFlow(t *testing.T) {
	repo := newStubGenRepo()
	svc := NewSoraGenerationService(repo, nil, nil)

	// 1. 创建 pending
	gen, err := svc.CreatePending(context.Background(), 1, nil, "sora2-landscape-10s", "test", "video")
	require.NoError(t, err)
	require.Equal(t, SoraGenStatusPending, gen.Status)

	// 2. 标记 generating
	err = svc.MarkGenerating(context.Background(), gen.ID, "task-123")
	require.NoError(t, err)
	require.Equal(t, SoraGenStatusGenerating, repo.gens[gen.ID].Status)

	// 3. 标记 completed
	err = svc.MarkCompleted(context.Background(), gen.ID, "https://s3.com/video.mp4", nil, SoraStorageTypeS3, []string{"key"}, 1024)
	require.NoError(t, err)
	require.Equal(t, SoraGenStatusCompleted, repo.gens[gen.ID].Status)
}

func TestStatusTransition_PendingToFailedFlow(t *testing.T) {
	repo := newStubGenRepo()
	svc := NewSoraGenerationService(repo, nil, nil)

	gen, _ := svc.CreatePending(context.Background(), 1, nil, "sora2-landscape-10s", "test", "video")
	_ = svc.MarkGenerating(context.Background(), gen.ID, "")

	err := svc.MarkFailed(context.Background(), gen.ID, "上游超时")
	require.NoError(t, err)
	require.Equal(t, SoraGenStatusFailed, repo.gens[gen.ID].Status)
	require.Equal(t, "上游超时", repo.gens[gen.ID].ErrorMessage)
}

func TestStatusTransition_PendingToCancelledFlow(t *testing.T) {
	repo := newStubGenRepo()
	svc := NewSoraGenerationService(repo, nil, nil)

	gen, _ := svc.CreatePending(context.Background(), 1, nil, "sora2-landscape-10s", "test", "video")
	err := svc.MarkCancelled(context.Background(), gen.ID)
	require.NoError(t, err)
	require.Equal(t, SoraGenStatusCancelled, repo.gens[gen.ID].Status)
}

func TestStatusTransition_GeneratingToCancelledFlow(t *testing.T) {
	repo := newStubGenRepo()
	svc := NewSoraGenerationService(repo, nil, nil)

	gen, _ := svc.CreatePending(context.Background(), 1, nil, "sora2-landscape-10s", "test", "video")
	_ = svc.MarkGenerating(context.Background(), gen.ID, "")
	err := svc.MarkCancelled(context.Background(), gen.ID)
	require.NoError(t, err)
	require.Equal(t, SoraGenStatusCancelled, repo.gens[gen.ID].Status)
}

// ==================== 权限隔离测试 ====================

func TestUserIsolation_CannotAccessOthersRecord(t *testing.T) {
	repo := newStubGenRepo()
	svc := NewSoraGenerationService(repo, nil, nil)

	gen, _ := svc.CreatePending(context.Background(), 1, nil, "sora2-landscape-10s", "test", "video")

	// 用户 2 尝试访问用户 1 的记录
	_, err := svc.GetByID(context.Background(), gen.ID, 2)
	require.Error(t, err)
	require.Contains(t, err.Error(), "无权访问")
}

func TestUserIsolation_CannotDeleteOthersRecord(t *testing.T) {
	repo := newStubGenRepo()
	svc := NewSoraGenerationService(repo, nil, nil)

	gen, _ := svc.CreatePending(context.Background(), 1, nil, "sora2-landscape-10s", "test", "video")

	err := svc.Delete(context.Background(), gen.ID, 2)
	require.Error(t, err)
	require.Contains(t, err.Error(), "无权删除")
}

// ==================== Delete: S3 清理 + 配额释放路径 ====================

func TestDelete_S3Cleanup_WithS3Storage(t *testing.T) {
	// S3 存储存在但 deleteObjects 会失败（settingService=nil），
	// 验证 Delete 仍然成功（S3 错误只是记录日志）
	repo := newStubGenRepo()
	repo.gens[1] = &SoraGeneration{
		ID: 1, UserID: 1,
		StorageType:  SoraStorageTypeS3,
		S3ObjectKeys: []string{"sora/1/2024/01/01/abc.mp4"},
	}
	s3Storage := newS3StorageFailingDelete()
	svc := NewSoraGenerationService(repo, s3Storage, nil)

	err := svc.Delete(context.Background(), 1, 1)
	require.NoError(t, err) // S3 清理失败不影响删除
	_, exists := repo.gens[1]
	require.False(t, exists)
}

func TestDelete_QuotaRelease_WithQuotaService(t *testing.T) {
	// 有配额服务时，删除 S3 类型记录会释放配额
	repo := newStubGenRepo()
	repo.gens[1] = &SoraGeneration{
		ID: 1, UserID: 1,
		StorageType:   SoraStorageTypeS3,
		FileSizeBytes: 1048576, // 1MB
	}

	userRepo := newStubUserRepoForQuota()
	userRepo.users[1] = &User{ID: 1, SoraStorageUsedBytes: 2097152} // 2MB
	quotaService := NewSoraQuotaService(userRepo, nil, nil)

	svc := NewSoraGenerationService(repo, nil, quotaService)
	err := svc.Delete(context.Background(), 1, 1)
	require.NoError(t, err)
	// 配额应被释放: 2MB - 1MB = 1MB
	require.Equal(t, int64(1048576), userRepo.users[1].SoraStorageUsedBytes)
}

func TestDelete_S3Cleanup_And_QuotaRelease(t *testing.T) {
	// S3 清理 + 配额释放同时触发
	repo := newStubGenRepo()
	repo.gens[1] = &SoraGeneration{
		ID: 1, UserID: 1,
		StorageType:   SoraStorageTypeS3,
		S3ObjectKeys:  []string{"key1"},
		FileSizeBytes: 512,
	}

	userRepo := newStubUserRepoForQuota()
	userRepo.users[1] = &User{ID: 1, SoraStorageUsedBytes: 1024}
	quotaService := NewSoraQuotaService(userRepo, nil, nil)
	s3Storage := newS3StorageFailingDelete()

	svc := NewSoraGenerationService(repo, s3Storage, quotaService)
	err := svc.Delete(context.Background(), 1, 1)
	require.NoError(t, err)
	_, exists := repo.gens[1]
	require.False(t, exists)
	require.Equal(t, int64(512), userRepo.users[1].SoraStorageUsedBytes)
}

func TestDelete_QuotaRelease_LocalStorage(t *testing.T) {
	// 本地存储同样需要释放配额
	repo := newStubGenRepo()
	repo.gens[1] = &SoraGeneration{
		ID: 1, UserID: 1,
		StorageType:   SoraStorageTypeLocal,
		FileSizeBytes: 1024,
	}

	userRepo := newStubUserRepoForQuota()
	userRepo.users[1] = &User{ID: 1, SoraStorageUsedBytes: 2048}
	quotaService := NewSoraQuotaService(userRepo, nil, nil)

	svc := NewSoraGenerationService(repo, nil, quotaService)
	err := svc.Delete(context.Background(), 1, 1)
	require.NoError(t, err)
	require.Equal(t, int64(1024), userRepo.users[1].SoraStorageUsedBytes)
}

func TestDelete_QuotaRelease_ZeroFileSize(t *testing.T) {
	// FileSizeBytes=0 跳过配额释放
	repo := newStubGenRepo()
	repo.gens[1] = &SoraGeneration{
		ID: 1, UserID: 1,
		StorageType:   SoraStorageTypeS3,
		FileSizeBytes: 0,
	}

	userRepo := newStubUserRepoForQuota()
	userRepo.users[1] = &User{ID: 1, SoraStorageUsedBytes: 1024}
	quotaService := NewSoraQuotaService(userRepo, nil, nil)

	svc := NewSoraGenerationService(repo, nil, quotaService)
	err := svc.Delete(context.Background(), 1, 1)
	require.NoError(t, err)
	require.Equal(t, int64(1024), userRepo.users[1].SoraStorageUsedBytes)
}

// ==================== ResolveMediaURLs: S3 + CDN 路径 ====================

func TestResolveMediaURLs_S3_CDN_SingleKey(t *testing.T) {
	s3Storage := newS3StorageWithCDN("https://cdn.example.com")
	svc := NewSoraGenerationService(newStubGenRepo(), s3Storage, nil)

	gen := &SoraGeneration{
		StorageType:  SoraStorageTypeS3,
		S3ObjectKeys: []string{"sora/1/2024/01/01/video.mp4"},
		MediaURL:     "original",
	}
	err := svc.ResolveMediaURLs(context.Background(), gen)
	require.NoError(t, err)
	require.Equal(t, "https://cdn.example.com/sora/1/2024/01/01/video.mp4", gen.MediaURL)
}

func TestResolveMediaURLs_S3_CDN_MultipleKeys(t *testing.T) {
	s3Storage := newS3StorageWithCDN("https://cdn.example.com/")
	svc := NewSoraGenerationService(newStubGenRepo(), s3Storage, nil)

	gen := &SoraGeneration{
		StorageType: SoraStorageTypeS3,
		S3ObjectKeys: []string{
			"sora/1/2024/01/01/img1.png",
			"sora/1/2024/01/01/img2.png",
			"sora/1/2024/01/01/img3.png",
		},
		MediaURL: "original",
	}
	err := svc.ResolveMediaURLs(context.Background(), gen)
	require.NoError(t, err)
	// 主 URL 更新为第一个 key 的 CDN URL
	require.Equal(t, "https://cdn.example.com/sora/1/2024/01/01/img1.png", gen.MediaURL)
	// 多图 URLs 全部更新
	require.Len(t, gen.MediaURLs, 3)
	require.Equal(t, "https://cdn.example.com/sora/1/2024/01/01/img1.png", gen.MediaURLs[0])
	require.Equal(t, "https://cdn.example.com/sora/1/2024/01/01/img2.png", gen.MediaURLs[1])
	require.Equal(t, "https://cdn.example.com/sora/1/2024/01/01/img3.png", gen.MediaURLs[2])
}

func TestResolveMediaURLs_S3_EmptyKeys(t *testing.T) {
	s3Storage := newS3StorageWithCDN("https://cdn.example.com")
	svc := NewSoraGenerationService(newStubGenRepo(), s3Storage, nil)

	gen := &SoraGeneration{
		StorageType:  SoraStorageTypeS3,
		S3ObjectKeys: []string{},
		MediaURL:     "original",
	}
	err := svc.ResolveMediaURLs(context.Background(), gen)
	require.NoError(t, err)
	require.Equal(t, "original", gen.MediaURL) // 不变
}

func TestResolveMediaURLs_S3_GetAccessURL_Error(t *testing.T) {
	// 使用无 settingService 的 S3 Storage，getClient 会失败
	s3Storage := newS3StorageFailingDelete() // 同样 GetAccessURL 也会失败
	svc := NewSoraGenerationService(newStubGenRepo(), s3Storage, nil)

	gen := &SoraGeneration{
		StorageType:  SoraStorageTypeS3,
		S3ObjectKeys: []string{"sora/1/2024/01/01/video.mp4"},
		MediaURL:     "original",
	}
	err := svc.ResolveMediaURLs(context.Background(), gen)
	require.Error(t, err) // GetAccessURL 失败应传播错误
}

func TestResolveMediaURLs_S3_MultiKey_ErrorOnSecond(t *testing.T) {
	// 只有一个 key 时走主 URL 路径成功，但多 key 路径的错误也需覆盖
	s3Storage := newS3StorageFailingDelete()
	svc := NewSoraGenerationService(newStubGenRepo(), s3Storage, nil)

	gen := &SoraGeneration{
		StorageType: SoraStorageTypeS3,
		S3ObjectKeys: []string{
			"sora/1/2024/01/01/img1.png",
			"sora/1/2024/01/01/img2.png",
		},
		MediaURL: "original",
	}
	err := svc.ResolveMediaURLs(context.Background(), gen)
	require.Error(t, err) // 第一个 key 的 GetAccessURL 就会失败
}
