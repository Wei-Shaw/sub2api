//go:build unit

package service

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"

	"github.com/stretchr/testify/require"
)

type ownerScopedMaterialRepo struct {
	UserMaterialRepository
	wantUserID int64
	wantID     int64
	material   *UserMaterial
}

func (r *ownerScopedMaterialRepo) GetByID(_ context.Context, userID, id int64) (*UserMaterial, error) {
	if userID != r.wantUserID || id != r.wantID {
		return nil, errors.New("unscoped material lookup")
	}
	return r.material, nil
}

func TestUserMaterialGetByIDIsOwnerScoped(t *testing.T) {
	t.Run("passes owner into repository query", func(t *testing.T) {
		repo := &ownerScopedMaterialRepo{wantUserID: 22, wantID: 7, material: &UserMaterial{ID: 7, UserID: 22}}
		material, err := NewUserMaterialService(repo, nil, nil, nil).GetByID(context.Background(), 22, 7)
		require.NoError(t, err)
		require.Equal(t, int64(22), material.UserID)
	})
	t.Run("other owner is indistinguishable from missing", func(t *testing.T) {
		repo := &ownerScopedMaterialRepo{wantUserID: 99, wantID: 7, material: nil}
		material, err := NewUserMaterialService(repo, nil, nil, nil).GetByID(context.Background(), 99, 7)
		require.Nil(t, material)
		require.ErrorIs(t, err, sql.ErrNoRows)
	})
}

// ---------------------------------------------------------------------------
// SSRF 防护：素材库 import-url 是唯一"URL 完全由终端用户控制"的下载入口，
// 必须挡住内网/回环/云元数据目标，否则任何登录用户都能拿它当 SSRF 跳板，
// 把内网响应转存到 COS 后通过公网地址读回来（数据外带）。
// ---------------------------------------------------------------------------

func TestDownloadUntrustedToBytes_RejectsInternalTargets(t *testing.T) {
	s := NewCOSImageTransferService(nil, nil, nil)

	cases := []struct {
		name string
		url  string
	}{
		{"loopback ipv4", "http://127.0.0.1:8080/admin"},
		{"loopback name", "http://localhost/admin"},
		{"ipv6 loopback", "http://[::1]:8080/admin"},
		{"cloud metadata", "http://169.254.169.254/latest/meta-data/iam/security-credentials/"},
		{"gcp metadata name", "http://metadata.google.internal/computeMetadata/v1/"},
		{"rfc1918 10/8", "http://10.0.0.5/internal"},
		{"rfc1918 172.16/12", "http://172.16.3.9/internal"},
		{"rfc1918 192.168/16", "http://192.168.1.1/router"},
		{"cgnat", "http://100.64.0.1/"},
		{"this network", "http://0.0.0.0:9000/"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := s.DownloadUntrustedToBytes(context.Background(), tc.url, 0)
			require.Error(t, err)
			require.ErrorIs(t, err, ErrUntrustedURLBlocked)
		})
	}
}

func TestDownloadUntrustedToBytes_RejectsNonHTTPSchemes(t *testing.T) {
	s := NewCOSImageTransferService(nil, nil, nil)
	// file:// 能读本地文件，gopher:// 历史上被用于打内网 Redis；一律拒绝。
	for _, raw := range []string{
		"file:///etc/passwd",
		"gopher://127.0.0.1:6379/_INFO",
		"ftp://example.com/x.png",
		"data:image/png;base64,AAAA",
	} {
		_, _, err := s.DownloadUntrustedToBytes(context.Background(), raw, 0)
		require.Error(t, err, "raw=%q", raw)
		// scheme 不合法属于输入问题，不应被误判为"被安全策略拦截"。
		require.NotErrorIs(t, err, ErrUntrustedURLBlocked, "raw=%q", raw)
	}
}

func TestDownloadUntrustedToBytes_RejectsEmptyAndMalformed(t *testing.T) {
	s := NewCOSImageTransferService(nil, nil, nil)
	for _, raw := range []string{"", "   ", "http://", "https://"} {
		_, _, err := s.DownloadUntrustedToBytes(context.Background(), raw, 0)
		require.Error(t, err, "raw=%q", raw)
	}
}

// ---------------------------------------------------------------------------
// 配额：单文件上限之外必须有"每用户总量"上限，否则可以靠反复上传刷满对象存储。
// ---------------------------------------------------------------------------

// quotaStubRepo 只实现配额校验用到的 UsageByUser；其余方法不会被调用。
type quotaStubRepo struct {
	UserMaterialRepository
	count      int64
	totalBytes int64
	err        error
}

type addMaterialRepo struct {
	UserMaterialRepository
	material *UserMaterial
}

type batchDeleteMaterialRepo struct {
	UserMaterialRepository
	wantUserID int64
	gotIDs     []string
	deletedIDs []string
	calls      int
}

func (r *batchDeleteMaterialRepo) SoftDeleteByPublicIDs(_ context.Context, userID int64, publicIDs []string) ([]string, error) {
	r.calls++
	if userID != r.wantUserID {
		return nil, errors.New("unscoped material batch delete")
	}
	r.gotIDs = append([]string(nil), publicIDs...)
	return append([]string(nil), r.deletedIDs...), nil
}

func TestBatchDeleteMaterialsValidatesAndPreservesRequestOrder(t *testing.T) {
	const (
		first  = "550e8400-e29b-41d4-a716-446655440000"
		second = "6ba7b810-9dad-11d1-80b4-00c04fd430c8"
		third  = "6ba7b811-9dad-11d1-80b4-00c04fd430c8"
	)
	repo := &batchDeleteMaterialRepo{
		wantUserID: 7,
		deletedIDs: []string{third, first},
	}
	svc := NewUserMaterialService(repo, nil, nil, nil)

	deleted, err := svc.BatchDeleteByPublicIDs(context.Background(), 7, []string{
		"  " + first + "  ", second, first, third,
	})
	require.NoError(t, err)
	require.Equal(t, []string{first, second, third}, repo.gotIDs, "repository input must be normalized and deduplicated")
	require.Equal(t, []string{first, third}, deleted, "response must follow the first occurrence in request order")
	require.Equal(t, 1, repo.calls)
}

func TestBatchDeleteMaterialsRejectsInvalidRequestBeforeRepositoryCall(t *testing.T) {
	repo := &batchDeleteMaterialRepo{wantUserID: 7}
	svc := NewUserMaterialService(repo, nil, nil, nil)

	tests := []struct {
		name   string
		userID int64
		ids    []string
		reason string
	}{
		{name: "invalid user", userID: 0, ids: []string{"550e8400-e29b-41d4-a716-446655440000"}, reason: "INVALID_USER"},
		{name: "empty ids", userID: 7, ids: nil, reason: "INVALID_IDS"},
		{name: "invalid id identifies indexed parameter", userID: 7, ids: []string{"550e8400-e29b-41d4-a716-446655440000", "not-a-uuid"}, reason: "INVALID_ID"},
		{name: "too many ids", userID: 7, ids: make([]string, MaterialBatchDeleteMaxIDs+1), reason: "TOO_MANY_IDS"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			callsBefore := repo.calls
			_, err := svc.BatchDeleteByPublicIDs(context.Background(), tc.userID, tc.ids)
			require.Error(t, err)
			require.Equal(t, tc.reason, infraerrors.Reason(err))
			require.Equal(t, callsBefore, repo.calls, "repository must not be called for invalid input")
			if tc.name == "invalid id identifies indexed parameter" {
				require.Contains(t, err.Error(), "ids[1]")
			}
		})
	}
}

func (r *addMaterialRepo) UsageByUser(context.Context, int64) (int64, int64, error) {
	return 0, 0, nil
}

func (r *addMaterialRepo) Insert(_ context.Context, material *UserMaterial) (int64, error) {
	material.ID = 91
	material.PublicID = "550e8400-e29b-41d4-a716-446655440000"
	r.material = material
	return material.ID, nil
}

func TestAddMaterialByURLRequiresConfiguredPublicCOSOrigin(t *testing.T) {
	store := &fakeObjectStore{}
	cos, _ := newCOSServiceForTest(t, store)
	enableCOS(t, cos)
	repo := &addMaterialRepo{}
	svc := NewUserMaterialService(repo, cos, nil, nil)

	material, err := svc.AddMaterialByURL(context.Background(), 7, "https://cdn.example.com/images/reference.png")
	require.NoError(t, err)
	require.Equal(t, "550e8400-e29b-41d4-a716-446655440000", material.PublicID)
	require.Equal(t, "https://cdn.example.com/images/reference.png", material.CosURL)
	require.Equal(t, "url", material.Source)
	require.Equal(t, "image", material.Kind)
	require.Equal(t, int32(0), atomic.LoadInt32(&store.uploads), "URL registration must not upload or download bytes")

	for _, raw := range []string{
		"https://evil.example.com/reference.png",
		"http://cdn.example.com/reference.png",
		"https://cdn.example.com/reference.png?signature=leak",
		"https://cdn.example.com.evil.example/reference.png",
	} {
		_, err := svc.AddMaterialByURL(context.Background(), 7, raw)
		require.Error(t, err, "raw=%q", raw)
		require.Equal(t, "INVALID_URL", infraerrors.Reason(err), "raw=%q", raw)
	}
}

func (r *quotaStubRepo) UsageByUser(context.Context, int64) (int64, int64, error) {
	return r.count, r.totalBytes, r.err
}

func TestCheckQuota(t *testing.T) {
	t.Run("allows when well under both limits", func(t *testing.T) {
		s := NewUserMaterialService(&quotaStubRepo{count: 10, totalBytes: 1 << 20}, nil, nil, nil)
		require.NoError(t, s.checkQuota(context.Background(), 1, 1<<20))
	})

	t.Run("rejects when count limit reached", func(t *testing.T) {
		s := NewUserMaterialService(&quotaStubRepo{count: MaterialMaxCountPerUser}, nil, nil, nil)
		err := s.checkQuota(context.Background(), 1, 1)
		require.Error(t, err)
		require.Equal(t, "MATERIAL_COUNT_QUOTA_EXCEEDED", infraerrors.Reason(err))
		require.True(t, infraerrors.IsBadRequest(err))
	})

	t.Run("rejects when incoming bytes would exceed size limit", func(t *testing.T) {
		s := NewUserMaterialService(&quotaStubRepo{count: 1, totalBytes: MaterialMaxTotalBytesPerUser - 10}, nil, nil, nil)
		err := s.checkQuota(context.Background(), 1, 11)
		require.Error(t, err)
		require.Equal(t, "MATERIAL_SIZE_QUOTA_EXCEEDED", infraerrors.Reason(err))
	})

	t.Run("allows when incoming bytes exactly fit", func(t *testing.T) {
		s := NewUserMaterialService(&quotaStubRepo{count: 1, totalBytes: MaterialMaxTotalBytesPerUser - 10}, nil, nil, nil)
		require.NoError(t, s.checkQuota(context.Background(), 1, 10))
	})

	t.Run("zero increment still catches an already-full account", func(t *testing.T) {
		// ImportFromURL 的下载前预检用的就是 incoming=0：已经超额时不该白拉一遍大文件。
		s := NewUserMaterialService(&quotaStubRepo{count: 1, totalBytes: MaterialMaxTotalBytesPerUser + 1}, nil, nil, nil)
		require.Error(t, s.checkQuota(context.Background(), 1, 0))
	})

	t.Run("fails open when the usage query errors", func(t *testing.T) {
		// 配额是成本保护而非安全边界：DB 抖动时不应让正常用户传不了东西
		// （单文件上限那一层仍然生效）。
		s := NewUserMaterialService(&quotaStubRepo{err: errors.New("db down")}, nil, nil, nil)
		require.NoError(t, s.checkQuota(context.Background(), 1, 1<<30))
	})
}

type materialPathUserRepo struct {
	UserRepository
	user *User
}

func (r *materialPathUserRepo) GetByID(context.Context, int64) (*User, error) {
	return r.user, nil
}

func TestMaterialUserPrefixIsOpaqueAndBoundToAccount(t *testing.T) {
	cfg := &config.Config{}
	cfg.JWT.Secret = "test-jwt-secret"
	userRepo := &materialPathUserRepo{user: &User{ID: 7, AccountID: "acct_public_7"}}
	svc := NewUserMaterialService(nil, nil, userRepo, cfg)

	prefix, err := svc.materialUserPrefix(context.Background(), 7)
	if err != nil {
		t.Fatalf("materialUserPrefix: %v", err)
	}
	if strings.Contains(prefix, "acct_public_7") || strings.HasPrefix(prefix, "users/7") {
		t.Fatalf("prefix leaks identity: %q", prefix)
	}
	if !strings.HasPrefix(prefix, "u_") || len(prefix) != 34 {
		t.Fatalf("prefix format = %q", prefix)
	}

	userRepo.user.AccountID = "acct_public_other"
	otherPrefix, err := svc.materialUserPrefix(context.Background(), 7)
	if err != nil {
		t.Fatalf("materialUserPrefix for changed account: %v", err)
	}
	if prefix == otherPrefix {
		t.Fatal("expected account_id to affect the derived prefix")
	}
}

func TestMaterialUserPrefixDoesNotFallbackWithoutIdentity(t *testing.T) {
	svc := NewUserMaterialService(nil, nil, nil, &config.Config{JWT: config.JWTConfig{Secret: "test-jwt-secret"}})
	_, err := svc.materialUserPrefix(context.Background(), 1)
	if !errors.Is(err, ErrMaterialPathIdentityUnavailable) {
		t.Fatalf("expected identity error, got %v", err)
	}
}
