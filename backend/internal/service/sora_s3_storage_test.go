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
REDACTED

func TestRefreshClient_AlreadyNil(t *testing.T) {
	s := NewSoraS3Storage(nil)
	s.RefreshClient() // 不应 panic
	require.Nil(t, s.client)
	require.Nil(t, s.cfg)
REDACTED

// ==================== GetMediaTypeFromKey ====================

func TestGetMediaTypeFromKey_VideoExtensions(t *testing.T) {
	for _, ext := range []string{".mp4", ".mov", ".webm", ".m4v", ".avi", ".mkv", ".3gp", ".flv"REDACTED {
		require.Equal(t, "video", GetMediaTypeFromKey("path/to/file"+ext), "ext=%s", ext)
REDACTED
REDACTED

func TestGetMediaTypeFromKey_VideoUpperCase(t *testing.T) {
	require.Equal(t, "video", GetMediaTypeFromKey("file.MP4"))
	require.Equal(t, "video", GetMediaTypeFromKey("file.MOV"))
REDACTED

func TestGetMediaTypeFromKey_ImageExtensions(t *testing.T) {
	require.Equal(t, "image", GetMediaTypeFromKey("file.png"))
	require.Equal(t, "image", GetMediaTypeFromKey("file.jpg"))
	require.Equal(t, "image", GetMediaTypeFromKey("file.jpeg"))
	require.Equal(t, "image", GetMediaTypeFromKey("file.gif"))
	require.Equal(t, "image", GetMediaTypeFromKey("file.webp"))
REDACTED

func TestGetMediaTypeFromKey_NoExtension(t *testing.T) {
	require.Equal(t, "image", GetMediaTypeFromKey("file"))
	require.Equal(t, "image", GetMediaTypeFromKey("path/to/file"))
REDACTED

func TestGetMediaTypeFromKey_UnknownExtension(t *testing.T) {
	require.Equal(t, "image", GetMediaTypeFromKey("file.bin"))
	require.Equal(t, "image", GetMediaTypeFromKey("file.xyz"))
REDACTED

// ==================== Enabled ====================

func TestEnabled_NilSettingService(t *testing.T) {
	s := NewSoraS3Storage(nil)
	require.False(t, s.Enabled(context.Background()))
REDACTED

func TestEnabled_ConfigDisabled(t *testing.T) {
	settingRepo := newStubSettingRepoForQuota(map[string]string{
		SettingKeySoraS3Enabled: "false",
		SettingKeySoraS3Bucket:  "test-bucket",
REDACTED)
	settingService := NewSettingService(settingRepo, &config.Config{REDACTED)
	s := NewSoraS3Storage(settingService)
	require.False(t, s.Enabled(context.Background()))
REDACTED

func TestEnabled_ConfigEnabledWithBucket(t *testing.T) {
	settingRepo := newStubSettingRepoForQuota(map[string]string{
		SettingKeySoraS3Enabled: "true",
		SettingKeySoraS3Bucket:  "my-bucket",
REDACTED)
	settingService := NewSettingService(settingRepo, &config.Config{REDACTED)
	s := NewSoraS3Storage(settingService)
	require.True(t, s.Enabled(context.Background()))
REDACTED

func TestEnabled_ConfigEnabledEmptyBucket(t *testing.T) {
	settingRepo := newStubSettingRepoForQuota(map[string]string{
		SettingKeySoraS3Enabled: "true",
REDACTED)
	settingService := NewSettingService(settingRepo, &config.Config{REDACTED)
	s := NewSoraS3Storage(settingService)
	require.False(t, s.Enabled(context.Background()))
REDACTED

// ==================== initClient ====================

func TestInitClient_Disabled(t *testing.T) {
	settingRepo := newStubSettingRepoForQuota(map[string]string{
		SettingKeySoraS3Enabled: "false",
REDACTED)
	settingService := NewSettingService(settingRepo, &config.Config{REDACTED)
	s := NewSoraS3Storage(settingService)

	_, _, err := s.getClient(context.Background())
REDACTED
	require.Contains(t, err.Error(), "disabled")
REDACTED

func TestInitClient_IncompleteConfig(t *testing.T) {
	settingRepo := newStubSettingRepoForQuota(map[string]string{
		SettingKeySoraS3Enabled: "true",
		SettingKeySoraS3Bucket:  "test-bucket",
		// 缺少 access_key_id 和 secret_access_key
REDACTED)
	settingService := NewSettingService(settingRepo, &config.Config{REDACTED)
	s := NewSoraS3Storage(settingService)

	_, _, err := s.getClient(context.Background())
REDACTED
	require.Contains(t, err.Error(), "incomplete")
REDACTED

func TestInitClient_DefaultRegion(t *testing.T) {
	settingRepo := newStubSettingRepoForQuota(map[string]string{
		SettingKeySoraS3Enabled:         "true",
		SettingKeySoraS3Bucket:          "test-bucket",
		SettingKeySoraS3AccessKeyID:     "AKID",
		SettingKeySoraS3SecretAccessKey: "SECRET",
		// Region 为空 → 默认 us-east-1
REDACTED)
	settingService := NewSettingService(settingRepo, &config.Config{REDACTED)
	s := NewSoraS3Storage(settingService)

	client, cfg, err := s.getClient(context.Background())
REDACTED
	require.NotNil(t, client)
	require.Equal(t, "test-bucket", cfg.Bucket)
REDACTED

func TestInitClient_DoubleCheck(t *testing.T) {
	// 验证双重检查锁定：第二次 getClient 命中缓存
	settingRepo := newStubSettingRepoForQuota(map[string]string{
		SettingKeySoraS3Enabled:         "true",
		SettingKeySoraS3Bucket:          "test-bucket",
		SettingKeySoraS3AccessKeyID:     "AKID",
		SettingKeySoraS3SecretAccessKey: "SECRET",
REDACTED)
	settingService := NewSettingService(settingRepo, &config.Config{REDACTED)
	s := NewSoraS3Storage(settingService)

	client1, _, err1 := s.getClient(context.Background())
	require.NoError(t, err1)
	client2, _, err2 := s.getClient(context.Background())
	require.NoError(t, err2)
	require.Equal(t, client1, client2) // 同一客户端实例
REDACTED

func TestInitClient_NilSettingService(t *testing.T) {
	s := NewSoraS3Storage(nil)
	_, _, err := s.getClient(context.Background())
REDACTED
	require.Contains(t, err.Error(), "setting service not available")
REDACTED

// ==================== GenerateObjectKey ====================

func TestGenerateObjectKey_ExtWithoutDot(t *testing.T) {
	s := NewSoraS3Storage(nil)
	key := s.GenerateObjectKey("", 1, "mp4")
	require.Contains(t, key, ".mp4")
	require.True(t, len(key) > 0)
REDACTED

func TestGenerateObjectKey_ExtWithDot(t *testing.T) {
	s := NewSoraS3Storage(nil)
	key := s.GenerateObjectKey("", 1, ".mp4")
	require.Contains(t, key, ".mp4")
	// 不应出现 ..mp4
	require.NotContains(t, key, "..mp4")
REDACTED

func TestGenerateObjectKey_WithPrefix(t *testing.T) {
	s := NewSoraS3Storage(nil)
	key := s.GenerateObjectKey("uploads/", 42, ".png")
	require.True(t, len(key) > 0)
	require.Contains(t, key, "uploads/sora/42/")
REDACTED

func TestGenerateObjectKey_PrefixWithoutTrailingSlash(t *testing.T) {
	s := NewSoraS3Storage(nil)
	key := s.GenerateObjectKey("uploads", 42, ".png")
	require.Contains(t, key, "uploads/sora/42/")
REDACTED

// ==================== GeneratePresignedURL ====================

func TestGeneratePresignedURL_GetClientError(t *testing.T) {
	s := NewSoraS3Storage(nil) // settingService=nil → getClient 失败
	_, err := s.GeneratePresignedURL(context.Background(), "key", 3600)
REDACTED
REDACTED

// ==================== GetAccessURL ====================

func TestGetAccessURL_CDN(t *testing.T) {
	s := newS3StorageWithCDN("https://cdn.example.com")
	url, err := s.GetAccessURL(context.Background(), "sora/1/2024/01/01/video.mp4")
REDACTED
	require.Equal(t, "https://cdn.example.com/sora/1/2024/01/01/video.mp4", url)
REDACTED

func TestGetAccessURL_CDNTrailingSlash(t *testing.T) {
	s := newS3StorageWithCDN("https://cdn.example.com/")
	url, err := s.GetAccessURL(context.Background(), "key.mp4")
REDACTED
	require.Equal(t, "https://cdn.example.com/key.mp4", url)
REDACTED

func TestGetAccessURL_GetClientError(t *testing.T) {
	s := NewSoraS3Storage(nil)
	_, err := s.GetAccessURL(context.Background(), "key")
REDACTED
REDACTED

// ==================== TestConnection ====================

func TestTestConnection_GetClientError(t *testing.T) {
	s := NewSoraS3Storage(nil)
	err := s.TestConnection(context.Background())
REDACTED
REDACTED

// ==================== UploadFromURL ====================

func TestUploadFromURL_GetClientError(t *testing.T) {
	s := NewSoraS3Storage(nil)
	_, _, err := s.UploadFromURL(context.Background(), 1, "https://example.com/file.mp4")
REDACTED
REDACTED

// ==================== DeleteObjects ====================

func TestDeleteObjects_EmptyKeys(t *testing.T) {
	s := NewSoraS3Storage(nil)
	err := s.DeleteObjects(context.Background(), []string{REDACTED)
REDACTED // 空列表直接返回
REDACTED

func TestDeleteObjects_NilKeys(t *testing.T) {
	s := NewSoraS3Storage(nil)
	err := s.DeleteObjects(context.Background(), nil)
REDACTED // nil 列表直接返回
REDACTED

func TestDeleteObjects_GetClientError(t *testing.T) {
	s := NewSoraS3Storage(nil)
	err := s.DeleteObjects(context.Background(), []string{"key1", "key2"REDACTED)
REDACTED
REDACTED
