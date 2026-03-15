//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

// ==================== RefreshClient ====================

func TestRefreshClient(t *testing.T) {
	s := newS3StorageWithCDN("https://cdn.example.com")
	require.NotNil(t, s.client)
	require.NotNil(t, s.cfg)

	s.RefreshClient()
	require.Nil(t, s.client)
	require.Nil(t, s.cfg)
}

func TestRefreshClient_AlreadyNil(t *testing.T) {
	s := NewSoraS3Storage(nil)
	s.RefreshClient() // 不应 panic
	require.Nil(t, s.client)
	require.Nil(t, s.cfg)
}

// ==================== GetMediaTypeFromKey ====================

func TestGetMediaTypeFromKey_VideoExtensions(t *testing.T) {
	for _, ext := range []string{".mp4", ".mov", ".webm", ".m4v", ".avi", ".mkv", ".3gp", ".flv"} {
		require.Equal(t, "video", GetMediaTypeFromKey("path/to/file"+ext), "ext=%s", ext)
	}
}

func TestGetMediaTypeFromKey_VideoUpperCase(t *testing.T) {
	require.Equal(t, "video", GetMediaTypeFromKey("file.MP4"))
	require.Equal(t, "video", GetMediaTypeFromKey("file.MOV"))
}

func TestGetMediaTypeFromKey_ImageExtensions(t *testing.T) {
	require.Equal(t, "image", GetMediaTypeFromKey("file.png"))
	require.Equal(t, "image", GetMediaTypeFromKey("file.jpg"))
	require.Equal(t, "image", GetMediaTypeFromKey("file.jpeg"))
	require.Equal(t, "image", GetMediaTypeFromKey("file.gif"))
	require.Equal(t, "image", GetMediaTypeFromKey("file.webp"))
}

func TestGetMediaTypeFromKey_NoExtension(t *testing.T) {
	require.Equal(t, "image", GetMediaTypeFromKey("file"))
	require.Equal(t, "image", GetMediaTypeFromKey("path/to/file"))
}

func TestGetMediaTypeFromKey_UnknownExtension(t *testing.T) {
	require.Equal(t, "image", GetMediaTypeFromKey("file.bin"))
	require.Equal(t, "image", GetMediaTypeFromKey("file.xyz"))
}

// ==================== Enabled ====================

func TestEnabled_NilSettingService(t *testing.T) {
	s := NewSoraS3Storage(nil)
	require.False(t, s.Enabled(context.Background()))
}

func TestEnabled_ConfigDisabled(t *testing.T) {
	settingRepo := newStubSettingRepoForQuota(map[string]string{
		SettingKeySoraS3Enabled: "false",
		SettingKeySoraS3Bucket:  "test-bucket",
	})
	settingService := NewSettingService(settingRepo, &config.Config{})
	s := NewSoraS3Storage(settingService)
	require.False(t, s.Enabled(context.Background()))
}

func TestEnabled_ConfigEnabledWithBucket(t *testing.T) {
	settingRepo := newStubSettingRepoForQuota(map[string]string{
		SettingKeySoraS3Enabled: "true",
		SettingKeySoraS3Bucket:  "my-bucket",
	})
	settingService := NewSettingService(settingRepo, &config.Config{})
	s := NewSoraS3Storage(settingService)
	require.True(t, s.Enabled(context.Background()))
}

func TestEnabled_ConfigEnabledEmptyBucket(t *testing.T) {
	settingRepo := newStubSettingRepoForQuota(map[string]string{
		SettingKeySoraS3Enabled: "true",
	})
	settingService := NewSettingService(settingRepo, &config.Config{})
	s := NewSoraS3Storage(settingService)
	require.False(t, s.Enabled(context.Background()))
}

// ==================== initClient ====================

func TestInitClient_Disabled(t *testing.T) {
	settingRepo := newStubSettingRepoForQuota(map[string]string{
		SettingKeySoraS3Enabled: "false",
	})
	settingService := NewSettingService(settingRepo, &config.Config{})
	s := NewSoraS3Storage(settingService)

	_, _, err := s.getClient(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "disabled")
}

func TestInitClient_IncompleteConfig(t *testing.T) {
	settingRepo := newStubSettingRepoForQuota(map[string]string{
		SettingKeySoraS3Enabled: "true",
		SettingKeySoraS3Bucket:  "test-bucket",
		// 缺少 access_key_id 和 secret_access_key
	})
	settingService := NewSettingService(settingRepo, &config.Config{})
	s := NewSoraS3Storage(settingService)

	_, _, err := s.getClient(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "incomplete")
}

func TestInitClient_DefaultRegion(t *testing.T) {
	settingRepo := newStubSettingRepoForQuota(map[string]string{
		SettingKeySoraS3Enabled:         "true",
		SettingKeySoraS3Bucket:          "test-bucket",
		SettingKeySoraS3AccessKeyID:     "AKID",
		SettingKeySoraS3SecretAccessKey: "SECRET",
		// Region 为空 → 默认 us-east-1
	})
	settingService := NewSettingService(settingRepo, &config.Config{})
	s := NewSoraS3Storage(settingService)

	client, cfg, err := s.getClient(context.Background())
	require.NoError(t, err)
	require.NotNil(t, client)
	require.Equal(t, "test-bucket", cfg.Bucket)
}

func TestInitClient_DoubleCheck(t *testing.T) {
	// 验证双重检查锁定：第二次 getClient 命中缓存
	settingRepo := newStubSettingRepoForQuota(map[string]string{
		SettingKeySoraS3Enabled:         "true",
		SettingKeySoraS3Bucket:          "test-bucket",
		SettingKeySoraS3AccessKeyID:     "AKID",
		SettingKeySoraS3SecretAccessKey: "SECRET",
	})
	settingService := NewSettingService(settingRepo, &config.Config{})
	s := NewSoraS3Storage(settingService)

	client1, _, err1 := s.getClient(context.Background())
	require.NoError(t, err1)
	client2, _, err2 := s.getClient(context.Background())
	require.NoError(t, err2)
	require.Equal(t, client1, client2) // 同一客户端实例
}

func TestInitClient_NilSettingService(t *testing.T) {
	s := NewSoraS3Storage(nil)
	_, _, err := s.getClient(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "setting service not available")
}

// ==================== GenerateObjectKey ====================

func TestGenerateObjectKey_ExtWithoutDot(t *testing.T) {
	s := NewSoraS3Storage(nil)
	key := s.GenerateObjectKey("", 1, "mp4")
	require.Contains(t, key, ".mp4")
	require.True(t, len(key) > 0)
}

func TestGenerateObjectKey_ExtWithDot(t *testing.T) {
	s := NewSoraS3Storage(nil)
	key := s.GenerateObjectKey("", 1, ".mp4")
	require.Contains(t, key, ".mp4")
	// 不应出现 ..mp4
	require.NotContains(t, key, "..mp4")
}

func TestGenerateObjectKey_WithPrefix(t *testing.T) {
	s := NewSoraS3Storage(nil)
	key := s.GenerateObjectKey("uploads/", 42, ".png")
	require.True(t, len(key) > 0)
	require.Contains(t, key, "uploads/sora/42/")
}

func TestGenerateObjectKey_PrefixWithoutTrailingSlash(t *testing.T) {
	s := NewSoraS3Storage(nil)
	key := s.GenerateObjectKey("uploads", 42, ".png")
	require.Contains(t, key, "uploads/sora/42/")
}

// ==================== GeneratePresignedURL ====================

func TestGeneratePresignedURL_GetClientError(t *testing.T) {
	s := NewSoraS3Storage(nil) // settingService=nil → getClient 失败
	_, err := s.GeneratePresignedURL(context.Background(), "key", 3600)
	require.Error(t, err)
}

// ==================== GetAccessURL ====================

func TestGetAccessURL_CDN(t *testing.T) {
	s := newS3StorageWithCDN("https://cdn.example.com")
	url, err := s.GetAccessURL(context.Background(), "sora/1/2024/01/01/video.mp4")
	require.NoError(t, err)
	require.Equal(t, "https://cdn.example.com/sora/1/2024/01/01/video.mp4", url)
}

func TestGetAccessURL_CDNTrailingSlash(t *testing.T) {
	s := newS3StorageWithCDN("https://cdn.example.com/")
	url, err := s.GetAccessURL(context.Background(), "key.mp4")
	require.NoError(t, err)
	require.Equal(t, "https://cdn.example.com/key.mp4", url)
}

func TestGetAccessURL_GetClientError(t *testing.T) {
	s := NewSoraS3Storage(nil)
	_, err := s.GetAccessURL(context.Background(), "key")
	require.Error(t, err)
}

// ==================== TestConnection ====================

func TestTestConnection_GetClientError(t *testing.T) {
	s := NewSoraS3Storage(nil)
	err := s.TestConnection(context.Background())
	require.Error(t, err)
}

// ==================== UploadFromURL ====================

func TestUploadFromURL_GetClientError(t *testing.T) {
	s := NewSoraS3Storage(nil)
	_, _, _, err := s.UploadFromURL(context.Background(), 1, "https://example.com/file.mp4")
	require.Error(t, err)
}

// ==================== DeleteObjects ====================

func TestDeleteObjects_EmptyKeys(t *testing.T) {
	s := NewSoraS3Storage(nil)
	err := s.DeleteObjects(context.Background(), []string{})
	require.NoError(t, err) // 空列表直接返回
}

func TestDeleteObjects_NilKeys(t *testing.T) {
	s := NewSoraS3Storage(nil)
	err := s.DeleteObjects(context.Background(), nil)
	require.NoError(t, err) // nil 列表直接返回
}

func TestDeleteObjects_GetClientError(t *testing.T) {
	s := NewSoraS3Storage(nil)
	err := s.DeleteObjects(context.Background(), []string{"key1", "key2"})
	require.Error(t, err)
}
