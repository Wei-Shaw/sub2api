package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

const (
	// settingKeyCOSImageConfig 是全局图片转存（腾讯云 COS / S3 兼容）配置的 setting key。
	settingKeyCOSImageConfig = "cos_image_transfer_config"

	// cosTransferMaxAttempts 单张图片转存（下载+上传）最大尝试次数。
	cosTransferMaxAttempts = 3

	// cosDownloadMaxBytes 单张图片下载大小上限。
	cosDownloadMaxBytes int64 = 32 << 20

	// cosDownloadTimeout 单次下载/上传 HTTP 超时。
	cosDownloadTimeout = 60 * time.Second
)

var (
	// ErrCOSConfigCorrupt 表示 COS 配置存储损坏。
	ErrCOSConfigCorrupt = infraerrors.InternalServer("COS_CONFIG_CORRUPT", "cos image transfer config data is corrupted")
)

// COSImageConfig 全局图片转存配置（腾讯云 COS 兼容 S3 协议）。
//
// 系统全局共享一份配置，由管理后台维护。SecretKey 入库前加密。
type COSImageConfig struct {
	Enabled         bool   `json:"enabled"`
	Endpoint        string `json:"endpoint"`                    // e.g. https://cos.ap-guangzhou.myqcloud.com
	Region          string `json:"region"`                      // e.g. ap-guangzhou
	Bucket          string `json:"bucket"`                      // e.g. mybucket-1250000000
	AccessKeyID     string `json:"access_key_id"`               // 腾讯云 SecretId
	SecretAccessKey string `json:"secret_access_key,omitempty"` //nolint:revive // AWS convention
	Prefix          string `json:"prefix"`                      // key 前缀，如 "images/"
	ForcePathStyle  bool   `json:"force_path_style"`
	// PublicBaseURL 用于拼接对外可访问的图片地址（如自定义 CDN 域名）。
	// 为空时回退使用 Endpoint + Bucket 拼接。
	PublicBaseURL string `json:"public_base_url"`
}

// IsConfigured 检查必要字段是否齐备。
func (c *COSImageConfig) IsConfigured() bool {
	return c != nil && c.Bucket != "" && c.AccessKeyID != "" && c.SecretAccessKey != ""
}

// COSImageTransferService 负责把上游（fal）出的临时图片转存到 COS。
type COSImageTransferService struct {
	settingRepo  SettingRepository
	encryptor    SecretEncryptor
	storeFactory BackupObjectStoreFactory
	httpClient   *http.Client

	mu    sync.Mutex
	store BackupObjectStore
	cfg   *COSImageConfig
}

// NewCOSImageTransferService 创建图片转存服务。
func NewCOSImageTransferService(
	settingRepo SettingRepository,
	encryptor SecretEncryptor,
	storeFactory BackupObjectStoreFactory,
) *COSImageTransferService {
	return &COSImageTransferService{
		settingRepo:  settingRepo,
		encryptor:    encryptor,
		storeFactory: storeFactory,
		httpClient:   &http.Client{Timeout: cosDownloadTimeout},
	}
}

// GetConfig 读取当前配置（SecretKey 已解密）。未配置时返回空配置。
func (s *COSImageTransferService) GetConfig(ctx context.Context) (*COSImageConfig, error) {
	cfg, err := s.loadConfig(ctx)
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		return &COSImageConfig{}, nil
	}
	return cfg, nil
}

// UpdateConfig 持久化配置（加密 SecretKey），并清空缓存的 store。
func (s *COSImageTransferService) UpdateConfig(ctx context.Context, cfg COSImageConfig) (*COSImageConfig, error) {
	cfg.Endpoint = strings.TrimSpace(cfg.Endpoint)
	cfg.Region = strings.TrimSpace(cfg.Region)
	cfg.Bucket = strings.TrimSpace(cfg.Bucket)
	cfg.AccessKeyID = strings.TrimSpace(cfg.AccessKeyID)
	cfg.Prefix = strings.TrimSpace(cfg.Prefix)
	cfg.PublicBaseURL = strings.TrimRight(strings.TrimSpace(cfg.PublicBaseURL), "/")

	toStore := cfg
	if secret := strings.TrimSpace(cfg.SecretAccessKey); secret != "" {
		encrypted, err := s.encryptor.Encrypt(secret)
		if err != nil {
			return nil, fmt.Errorf("encrypt cos secret: %w", err)
		}
		toStore.SecretAccessKey = encrypted
	} else if existing := s.loadRawSecret(ctx); existing != "" {
		// 留空表示保持原密钥不变（避免管理后台每次都要重输）。
		toStore.SecretAccessKey = existing
		cfg.SecretAccessKey = ""
	}

	data, err := json.Marshal(&toStore)
	if err != nil {
		return nil, fmt.Errorf("marshal cos config: %w", err)
	}
	if err := s.settingRepo.Set(ctx, settingKeyCOSImageConfig, string(data)); err != nil {
		return nil, fmt.Errorf("persist cos config: %w", err)
	}

	s.mu.Lock()
	s.store = nil
	s.cfg = nil
	s.mu.Unlock()

	return &cfg, nil
}

// IsEnabled 返回转存是否启用且配置完整。
func (s *COSImageTransferService) IsEnabled(ctx context.Context) bool {
	cfg, err := s.loadConfig(ctx)
	if err != nil || cfg == nil {
		return false
	}
	return cfg.Enabled && cfg.IsConfigured()
}

// TransferImages 将一批 fal 图片 url 转存到 COS。
//
// 返回与输入等长的 cos url 列表：成功项为 COS 地址，失败（重试耗尽）项为空字符串，
// 调用方据此回退使用原始 fal url。第二个返回值表示是否全部成功。
//
// 转存未启用或未配置时，返回全空切片 + allOK=false（不视为错误）。
func (s *COSImageTransferService) TransferImages(ctx context.Context, urls []string) ([]string, bool) {
	out := make([]string, len(urls))
	if len(urls) == 0 {
		return out, true
	}

	cfg, err := s.loadConfig(ctx)
	if err != nil || cfg == nil || !cfg.Enabled || !cfg.IsConfigured() {
		return out, false
	}
	store, err := s.getOrCreateStore(ctx, cfg)
	if err != nil {
		logger.LegacyPrintf("service.cos_transfer", "[COS] get store failed: %v", err)
		return out, false
	}

	allOK := true
	for i, rawURL := range urls {
		rawURL = strings.TrimSpace(rawURL)
		if rawURL == "" {
			allOK = false
			continue
		}
		cosURL, transferErr := s.transferOne(ctx, store, cfg, rawURL)
		if transferErr != nil {
			logger.LegacyPrintf("service.cos_transfer", "[COS] transfer failed after retries url=%s err=%v", rawURL, transferErr)
			out[i] = "" // 回退：留空，调用方使用原始 url
			allOK = false
			continue
		}
		out[i] = cosURL
	}
	return out, allOK
}

// transferOne 下载单张图片并上传到 COS，最多重试 cosTransferMaxAttempts 次。
func (s *COSImageTransferService) transferOne(ctx context.Context, store BackupObjectStore, cfg *COSImageConfig, srcURL string) (string, error) {
	var lastErr error
	for attempt := 1; attempt <= cosTransferMaxAttempts; attempt++ {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		data, contentType, err := s.download(ctx, srcURL)
		if err != nil {
			lastErr = err
			continue
		}
		key := s.buildKey(cfg, srcURL, contentType)
		if _, err := store.Upload(ctx, key, strings.NewReader(string(data)), contentType); err != nil {
			lastErr = err
			continue
		}
		return s.buildPublicURL(cfg, key), nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("transfer failed")
	}
	return "", lastErr
}

// FetchAsBase64 下载单张图片并返回其 base64 编码（标准编码，不含 data URL 前缀），
// 复用内部 HTTP 客户端与下载大小上限。供伪同步门面构造 OpenAI b64_json 响应。
func (s *COSImageTransferService) FetchAsBase64(ctx context.Context, srcURL string) (string, error) {
	srcURL = strings.TrimSpace(srcURL)
	if srcURL == "" {
		return "", fmt.Errorf("empty image url")
	}
	data, _, err := s.download(ctx, srcURL)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(data), nil
}

func (s *COSImageTransferService) download(ctx context.Context, srcURL string) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srcURL, nil)
	if err != nil {
		return nil, "", err
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", fmt.Errorf("download image: status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, cosDownloadMaxBytes))
	if err != nil {
		return nil, "", err
	}
	contentType := strings.TrimSpace(resp.Header.Get("Content-Type"))
	if contentType == "" {
		contentType = http.DetectContentType(data)
	}
	return data, contentType, nil
}

// buildKey 生成 COS 对象 key：{prefix}/{yyyy/mm/dd}/{uuid}{ext}
func (s *COSImageTransferService) buildKey(cfg *COSImageConfig, srcURL, contentType string) string {
	prefix := strings.Trim(cfg.Prefix, "/")
	if prefix == "" {
		prefix = "images"
	}
	ext := imageExtFromURLOrType(srcURL, contentType)
	return fmt.Sprintf("%s/%s/%s%s", prefix, time.Now().Format("2006/01/02"), uuid.NewString(), ext)
}

// buildPublicURL 拼接对外可访问地址。
func (s *COSImageTransferService) buildPublicURL(cfg *COSImageConfig, key string) string {
	if base := strings.TrimRight(cfg.PublicBaseURL, "/"); base != "" {
		return base + "/" + key
	}
	endpoint := strings.TrimRight(cfg.Endpoint, "/")
	if endpoint == "" {
		return key
	}
	if cfg.ForcePathStyle {
		return fmt.Sprintf("%s/%s/%s", endpoint, cfg.Bucket, key)
	}
	// virtual-hosted style: https://{bucket}.{host}/{key}
	if scheme, host, ok := splitScheme(endpoint); ok {
		return fmt.Sprintf("%s://%s.%s/%s", scheme, cfg.Bucket, host, key)
	}
	return fmt.Sprintf("%s/%s/%s", endpoint, cfg.Bucket, key)
}

func (s *COSImageTransferService) loadConfig(ctx context.Context) (*COSImageConfig, error) {
	raw, err := s.settingRepo.GetValue(ctx, settingKeyCOSImageConfig)
	if err != nil || raw == "" {
		return nil, nil //nolint:nilnil // 未配置是合法状态
	}
	var cfg COSImageConfig
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return nil, ErrCOSConfigCorrupt
	}
	if cfg.SecretAccessKey != "" {
		decrypted, err := s.encryptor.Decrypt(cfg.SecretAccessKey)
		if err != nil {
			// 兼容未加密旧数据：解密失败保持原值
			logger.LegacyPrintf("service.cos_transfer", "[COS] secret 解密失败（可能为旧的未加密数据）: %v", err)
		} else {
			cfg.SecretAccessKey = decrypted
		}
	}
	return &cfg, nil
}

// loadRawSecret 读取已存储的（仍为密文的）SecretKey，用于更新时保留原密钥。
func (s *COSImageTransferService) loadRawSecret(ctx context.Context) string {
	raw, err := s.settingRepo.GetValue(ctx, settingKeyCOSImageConfig)
	if err != nil || raw == "" {
		return ""
	}
	var cfg COSImageConfig
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return ""
	}
	return cfg.SecretAccessKey
}

func (s *COSImageTransferService) getOrCreateStore(ctx context.Context, cfg *COSImageConfig) (BackupObjectStore, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.store != nil && s.cfg != nil {
		return s.store, nil
	}
	store, err := s.storeFactory(ctx, &BackupS3Config{
		Endpoint:        cfg.Endpoint,
		Region:          cfg.Region,
		Bucket:          cfg.Bucket,
		AccessKeyID:     cfg.AccessKeyID,
		SecretAccessKey: cfg.SecretAccessKey,
		Prefix:          cfg.Prefix,
		ForcePathStyle:  cfg.ForcePathStyle,
	})
	if err != nil {
		return nil, err
	}
	s.store = store
	s.cfg = cfg
	return store, nil
}

// imageExtFromURLOrType 推断图片扩展名（含点），优先从 url 路径，再从 content-type。
func imageExtFromURLOrType(srcURL, contentType string) string {
	if ext := strings.ToLower(path.Ext(stripQuery(srcURL))); isImageExt(ext) {
		return ext
	}
	switch strings.ToLower(strings.TrimSpace(strings.SplitN(contentType, ";", 2)[0])) {
	case "image/png":
		return ".png"
	case "image/jpeg", "image/jpg":
		return ".jpg"
	case "image/webp":
		return ".webp"
	case "image/gif":
		return ".gif"
	default:
		return ".png"
	}
}

func isImageExt(ext string) bool {
	switch ext {
	case ".png", ".jpg", ".jpeg", ".webp", ".gif":
		return true
	default:
		return false
	}
}

func stripQuery(s string) string {
	if idx := strings.IndexAny(s, "?#"); idx >= 0 {
		return s[:idx]
	}
	return s
}

func splitScheme(endpoint string) (scheme, host string, ok bool) {
	idx := strings.Index(endpoint, "://")
	if idx < 0 {
		return "", "", false
	}
	return endpoint[:idx], endpoint[idx+3:], true
}
