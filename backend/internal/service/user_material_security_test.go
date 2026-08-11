package service

import (
	"context"
	"errors"
	"testing"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"

	"github.com/stretchr/testify/require"
)

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

func (r *quotaStubRepo) UsageByUser(context.Context, int64) (int64, int64, error) {
	return r.count, r.totalBytes, r.err
}

func TestCheckQuota(t *testing.T) {
	t.Run("allows when well under both limits", func(t *testing.T) {
		s := NewUserMaterialService(&quotaStubRepo{count: 10, totalBytes: 1 << 20}, nil)
		require.NoError(t, s.checkQuota(context.Background(), 1, 1<<20))
	})

	t.Run("rejects when count limit reached", func(t *testing.T) {
		s := NewUserMaterialService(&quotaStubRepo{count: MaterialMaxCountPerUser}, nil)
		err := s.checkQuota(context.Background(), 1, 1)
		require.Error(t, err)
		require.Equal(t, "MATERIAL_COUNT_QUOTA_EXCEEDED", infraerrors.Reason(err))
		require.True(t, infraerrors.IsBadRequest(err))
	})

	t.Run("rejects when incoming bytes would exceed size limit", func(t *testing.T) {
		s := NewUserMaterialService(&quotaStubRepo{count: 1, totalBytes: MaterialMaxTotalBytesPerUser - 10}, nil)
		err := s.checkQuota(context.Background(), 1, 11)
		require.Error(t, err)
		require.Equal(t, "MATERIAL_SIZE_QUOTA_EXCEEDED", infraerrors.Reason(err))
	})

	t.Run("allows when incoming bytes exactly fit", func(t *testing.T) {
		s := NewUserMaterialService(&quotaStubRepo{count: 1, totalBytes: MaterialMaxTotalBytesPerUser - 10}, nil)
		require.NoError(t, s.checkQuota(context.Background(), 1, 10))
	})

	t.Run("zero increment still catches an already-full account", func(t *testing.T) {
		// ImportFromURL 的下载前预检用的就是 incoming=0：已经超额时不该白拉一遍大文件。
		s := NewUserMaterialService(&quotaStubRepo{count: 1, totalBytes: MaterialMaxTotalBytesPerUser + 1}, nil)
		require.Error(t, s.checkQuota(context.Background(), 1, 0))
	})

	t.Run("fails open when the usage query errors", func(t *testing.T) {
		// 配额是成本保护而非安全边界：DB 抖动时不应让正常用户传不了东西
		// （单文件上限那一层仍然生效）。
		s := NewUserMaterialService(&quotaStubRepo{err: errors.New("db down")}, nil)
		require.NoError(t, s.checkQuota(context.Background(), 1, 1<<30))
	})
}
