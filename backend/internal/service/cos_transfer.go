package service

import (
	"bytes"
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
		logger.LegacyPrintf("service.cos_transfer",
			"[COS] UpdateConfig: encrypted new SecretAccessKey, plaintext_len=%d cipher_len=%d cipher_prefix=%s",
			len(secret), len(encrypted), safePrefix(encrypted, 16))
		// 立即用同一个 encryptor 自检解密，确保本进程能正确读回（排除 key 不一致或 Encrypt/Decrypt 实现 bug）。
		if back, derr := s.encryptor.Decrypt(encrypted); derr != nil || back != secret {
			logger.LegacyPrintf("service.cos_transfer",
				"[COS] UpdateConfig: 自检解密失败！encryptor 可能配置错误: err=%v match=%v", derr, back == secret)
			return nil, fmt.Errorf("cos secret encrypt self-check failed: err=%v match=%v", derr, back == secret)
		}
	} else if existing := s.loadRawSecret(ctx); existing != "" {
		// 留空表示保持原密钥不变（避免管理后台每次都要重输）。
		toStore.SecretAccessKey = existing
		cfg.SecretAccessKey = ""
		logger.LegacyPrintf("service.cos_transfer",
			"[COS] UpdateConfig: 前端未提交新 SecretAccessKey，沿用数据库中已有密文 cipher_len=%d cipher_prefix=%s（如希望换 SK，请在表单中填入新值）",
			len(existing), safePrefix(existing, 16))
	} else {
		logger.LegacyPrintf("service.cos_transfer",
			"[COS] UpdateConfig: SecretAccessKey 为空且数据库无历史密文，将保存为空")
	}

	data, err := json.Marshal(&toStore)
	if err != nil {
		return nil, fmt.Errorf("marshal cos config: %w", err)
	}
	if err := s.settingRepo.Set(ctx, settingKeyCOSImageConfig, string(data)); err != nil {
		return nil, fmt.Errorf("persist cos config: %w", err)
	}
	logger.LegacyPrintf("service.cos_transfer",
		"[COS] UpdateConfig: 配置已持久化 stored_secret_len=%d stored_secret_prefix=%s",
		len(toStore.SecretAccessKey), safePrefix(toStore.SecretAccessKey, 16))

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
		logger.LegacyPrintf("service.cos_transfer", "[COS] transfer disabled or not configured: enabled=%v, configured=%v, err=%v", cfg != nil && cfg.Enabled, cfg != nil && cfg.IsConfigured(), err)
		return out, false
	}
	logger.LegacyPrintf("service.cos_transfer", "[COS] transfer enabled, processing %d images", len(urls))

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
		logger.LegacyPrintf("service.cos_transfer", "[COS] transfer succeeded: %s -> %s", rawURL, cosURL)
		out[i] = cosURL
	}
	logger.LegacyPrintf("service.cos_transfer", "[COS] transfer completed: %d/%d succeeded", len(urls)-countEmpty(out), len(urls))
	return out, allOK
}

// countEmpty 统计空字符串数量
func countEmpty(strs []string) int {
	count := 0
	for _, s := range strs {
		if s == "" {
			count++
		}
	}
	return count
}

// transferOne 下载单张图片并上传到 COS，最多重试 cosTransferMaxAttempts 次。
func (s *COSImageTransferService) transferOne(ctx context.Context, store BackupObjectStore, cfg *COSImageConfig, srcURL string) (string, error) {
	var lastErr error
	for attempt := 1; attempt <= cosTransferMaxAttempts; attempt++ {
		if ctx.Err() != nil {
			logger.LegacyPrintf("service.cos_transfer", "[COS] transfer cancelled: %v", ctx.Err())
			return "", ctx.Err()
		}
		logger.LegacyPrintf("service.cos_transfer", "[COS] downloading image (attempt %d/%d): %s", attempt, cosTransferMaxAttempts, srcURL)
		data, contentType, err := s.download(ctx, srcURL)
		if err != nil {
			logger.LegacyPrintf("service.cos_transfer", "[COS] download failed (attempt %d): %v", attempt, err)
			lastErr = err
			continue
		}
		logger.LegacyPrintf("service.cos_transfer", "[COS] download succeeded: size=%d bytes, content-type=%s", len(data), contentType)

		key := s.buildKey(cfg, srcURL, contentType)
		logger.LegacyPrintf("service.cos_transfer", "[COS] uploading to COS (attempt %d): key=%s", attempt, key)
		if _, err := store.Upload(ctx, key, strings.NewReader(string(data)), contentType); err != nil {
			logger.LegacyPrintf("service.cos_transfer", "[COS] upload failed (attempt %d): %v", attempt, err)
			lastErr = err
			continue
		}
		cosURL := s.buildPublicURL(cfg, key)
		logger.LegacyPrintf("service.cos_transfer", "[COS] upload succeeded: %s", cosURL)
		return cosURL, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("transfer failed")
	}
	logger.LegacyPrintf("service.cos_transfer", "[COS] all transfer attempts failed: %v", lastErr)
	return "", lastErr
}

// FetchAsBase64 下载单张图片并返回其 base64 编码（标准编码，不含 data URL 前缀），
// 复用内部 HTTP 客户端与下载大小上限。供伪同步门面构造 OpenAI b64_json 响应。
func (s *COSImageTransferService) FetchAsBase64(ctx context.Context, srcURL string) (string, error) {
	srcURL = strings.TrimSpace(srcURL)
	if srcURL == "" {
		logger.LegacyPrintf("service.cos_transfer", "[COS] FetchAsBase64: empty image url")
		return "", fmt.Errorf("empty image url")
	}
	logger.LegacyPrintf("service.cos_transfer", "[COS] FetchAsBase64: downloading %s", srcURL)
	data, _, err := s.download(ctx, srcURL)
	if err != nil {
		logger.LegacyPrintf("service.cos_transfer", "[COS] FetchAsBase64 download failed: %v", err)
		return "", err
	}
	b64 := base64.StdEncoding.EncodeToString(data)
	logger.LegacyPrintf("service.cos_transfer", "[COS] FetchAsBase64 succeeded: size=%d bytes, base64_len=%d", len(data), len(b64))
	return b64, nil
}

// UploadImageBytes 把已在内存中的图片字节直接上传到 COS，返回对外可访问的 URL。
//
// 适用场景：上游（如 OpenAI Images）直接返回 base64 图片内容，无需再次下载，
// 由调用方解码后传入字节数据。contentTypeHint 仅作为提示，最终上传到 COS 的 ContentType
// 会被规范化为不含参数（如 charset）的 image/* 形式，避免 v4 签名 canonical 化差异
// 在腾讯云 COS / 阿里云 OSS 上引发 SignatureDoesNotMatch。srcHint 仅用于日志与扩展名推断，可为空。
//
// 转存未启用或未配置时返回 ("", nil)，调用方据此判定是否应回退（不视作错误）。
func (s *COSImageTransferService) UploadImageBytes(ctx context.Context, data []byte, contentTypeHint, srcHint string) (string, error) {
	if len(data) == 0 {
		return "", fmt.Errorf("empty image bytes")
	}
	cfg, err := s.loadConfig(ctx)
	if err != nil {
		logger.LegacyPrintf("service.cos_transfer", "[COS] UploadImageBytes load config failed: %v", err)
		return "", err
	}
	if cfg == nil || !cfg.Enabled || !cfg.IsConfigured() {
		logger.LegacyPrintf("service.cos_transfer", "[COS] UploadImageBytes disabled or not configured")
		return "", nil
	}
	store, err := s.getOrCreateStore(ctx, cfg)
	if err != nil {
		logger.LegacyPrintf("service.cos_transfer", "[COS] UploadImageBytes get store failed: %v", err)
		return "", err
	}

	// 规范化 ContentType：
	//  1) 优先使用调用方提供的 hint（裁掉参数部分，如 ";charset=utf-8"）
	//  2) hint 为空时用 http.DetectContentType 嗅探，但同样裁掉参数
	//  3) 嗅探结果不是合法 image/* 时，按 OpenAI 出图默认 image/png 兜底
	// 这一步是为了规避部分 S3 兼容服务（COS/OSS）在 v4 签名 canonical
	// 处理 ContentType 带参数时与服务端不一致导致 403 SignatureDoesNotMatch。
	rawHint := contentTypeHint
	contentType := normalizeImageContentType(strings.TrimSpace(contentTypeHint))
	if contentType == "" {
		detected := normalizeImageContentType(http.DetectContentType(data))
		if detected != "" {
			contentType = detected
		}
	}
	if contentType == "" {
		contentType = "image/png"
	}

	key := s.buildKey(cfg, srcHint, contentType)
	logger.LegacyPrintf("service.cos_transfer",
		"[COS] UploadImageBytes uploading: size=%d hint=%q final-content-type=%s key=%s",
		len(data), rawHint, contentType, key)
	if _, err := store.Upload(ctx, key, bytes.NewReader(data), contentType); err != nil {
		logger.LegacyPrintf("service.cos_transfer", "[COS] UploadImageBytes upload failed: %v", err)
		return "", err
	}
	cosURL := s.buildPublicURL(cfg, key)
	logger.LegacyPrintf("service.cos_transfer", "[COS] UploadImageBytes succeeded: %s", cosURL)
	return cosURL, nil
}

// normalizeImageContentType 把 ContentType 截断到分号前，仅保留 type/subtype 部分；
// 对非 image/* 的输入返回空串。这样可避免 ";charset=utf-8" 等参数进入 S3 v4 签名的 canonical request。
func normalizeImageContentType(ct string) string {
	ct = strings.TrimSpace(ct)
	if ct == "" {
		return ""
	}
	if i := strings.IndexByte(ct, ';'); i >= 0 {
		ct = strings.TrimSpace(ct[:i])
	}
	ct = strings.ToLower(ct)
	if !strings.HasPrefix(ct, "image/") {
		return ""
	}
	return ct
}

// isCipherAuthFailure 判断 AESEncryptor.Decrypt 返回的错误是否属于
// "数据是合法密文、但加密密钥不匹配"（GCM 认证失败）的情形。
// 该情形下不能把原值当明文使用，否则上层会拿密文去做签名导致 403。
func isCipherAuthFailure(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	// AESEncryptor.Decrypt 在 gcm.Open 失败时返回 "decrypt: cipher: message authentication failed"
	return strings.Contains(msg, "message authentication failed")
}

// safePrefix 返回字符串前 n 个字符的安全展示形式，用于在日志中诊断密文是否变化。
// 由于密文是 base64(nonce+ciphertext+tag)，nonce 每次随机，前缀也每次不同，
// 所以"两次保存得到不同前缀"是正常的；但"保存前后数据库里前缀一样"则说明根本没换。
func safePrefix(s string, n int) string {
	if n <= 0 || s == "" {
		return ""
	}
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
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
			// AES-GCM Decrypt 的错误分两类：
			//  1) 不是合法 base64 / 长度过短 —— 极大概率是历史明文，回退保留原值
			//  2) "cipher: message authentication failed" —— 数据是密文但加密 key 不匹配，
			//     此时若回退原值，会把密文当 SecretKey 拿去签名，必然 403 SignatureDoesNotMatch。
			//     这种情况必须直接返回错误，提示运维去后台重新保存配置或修正 TOTP_ENCRYPTION_KEY。
			if isCipherAuthFailure(err) {
				logger.LegacyPrintf("service.cos_transfer",
					"[COS] secret 解密失败：密文与当前加密密钥不匹配（请到后台 COS 配置页重新保存 SecretAccessKey，或检查 TOTP_ENCRYPTION_KEY 是否被改动）"+
						" cipher_len=%d cipher_prefix=%s err=%v",
					len(cfg.SecretAccessKey), safePrefix(cfg.SecretAccessKey, 16), err)
				return nil, fmt.Errorf("cos secret decrypt failed (key mismatch): %w", err)
			}
			logger.LegacyPrintf("service.cos_transfer",
				"[COS] secret 解密失败（疑似历史明文，按明文使用）cipher_len=%d cipher_prefix=%s err=%v",
				len(cfg.SecretAccessKey), safePrefix(cfg.SecretAccessKey, 16), err)
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
