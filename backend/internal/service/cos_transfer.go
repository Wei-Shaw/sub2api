package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"io"
	"net/http"
	"net/url"
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

	// cosVideoDownloadMaxBytes 单个视频下载大小上限（视频体积远大于图片，独立配置）。
	cosVideoDownloadMaxBytes int64 = 512 << 20

	// cosDownloadTimeout 单次下载/上传 HTTP 超时。
	cosDownloadTimeout = 60 * time.Second

	// cosVideoKeyPrefix 视频转存对象 key 的固定前缀（不受 setting.prefix 影响）。
	cosVideoKeyPrefix = "videos"
)

var (
	// ErrCOSConfigCorrupt 表示 COS 配置存储损坏。
	ErrCOSConfigCorrupt = infraerrors.InternalServer("COS_CONFIG_CORRUPT", "cos image transfer config data is corrupted")

	// ErrCOSNotConfigured 表示 COS 转存尚未启用/配置完整。
	// 用户素材库、图片输入控件依赖 COS 转存；未配置时应向用户提示"请联系管理员配置存储"，
	// 而不是静默回退（这些能力就是围绕 COS 建的）。
	ErrCOSNotConfigured        = infraerrors.BadRequest("COS_NOT_CONFIGURED", "cos image transfer is not enabled or not configured")
	ErrCOSStreamingUnsupported = infraerrors.InternalServer("COS_STREAMING_UNSUPPORTED", "configured object store does not support streaming uploads")
	ErrCOSMoveUnsupported      = infraerrors.InternalServer("COS_MOVE_UNSUPPORTED", "configured object store does not support server-side moves")
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
	// httpClient 用于"可信来源"下载：URL 来自上游 API 的响应（fal/atlascloud/apiz
	// 返回的临时图片/视频地址），管理员自己配置的上游地址亦属此列。
	httpClient *http.Client
	// untrustedClient 用于"用户可控 URL"下载（素材库 import-url）。
	// 必须走 safeDialContext：在 socket 层逐个校验解析出的真实 IP，
	// 这样即便攻击者用 DNS rebinding 或 30x 重定向指向内网也会被拒。
	untrustedClient *http.Client

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
		settingRepo:     settingRepo,
		encryptor:       encryptor,
		storeFactory:    storeFactory,
		httpClient:      &http.Client{Timeout: cosDownloadTimeout},
		untrustedClient: newSSRFSafeHTTPClient(cosDownloadTimeout),
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

// TransferVideos 将一批上游视频 url 转存到 COS。
//
// 与 TransferImages 语义一致：返回与输入等长的 cos url 列表，成功项为 COS 地址，
// 失败（重试耗尽/未启用/未配置/下载体积超限）项为空字符串，调用方据此回退使用原始 url。
// 第二个返回值表示是否全部成功。
//
// 区别：
//   - 对象 key 前缀强制为 "videos/"（不使用 setting 里配置的 prefix）
//   - 下载体积上限使用 cosVideoDownloadMaxBytes（512 MiB）
//   - key 后缀走视频扩展名推断（.mp4/.mov/.webm/.m4v）
func (s *COSImageTransferService) TransferVideos(ctx context.Context, urls []string) ([]string, bool) {
	out, _, allOK := s.TransferVideosWithDurations(ctx, urls)
	return out, allOK
}

// TransferVideosWithDurations transfers videos and extracts duration from the
// bytes already downloaded for upload. Duration parsing is best effort and does
// not turn an otherwise successful COS transfer into a failure.
func (s *COSImageTransferService) TransferVideosWithDurations(ctx context.Context, urls []string) ([]string, []int, bool) {
	out := make([]string, len(urls))
	durations := make([]int, len(urls))
	if len(urls) == 0 {
		return out, durations, true
	}

	cfg, err := s.loadConfig(ctx)
	if err != nil || cfg == nil || !cfg.Enabled || !cfg.IsConfigured() {
		logger.LegacyPrintf("service.cos_transfer", "[COS] video transfer disabled or not configured: enabled=%v, configured=%v, err=%v", cfg != nil && cfg.Enabled, cfg != nil && cfg.IsConfigured(), err)
		return out, durations, false
	}
	logger.LegacyPrintf("service.cos_transfer", "[COS] video transfer enabled, processing %d videos", len(urls))

	store, err := s.getOrCreateStore(ctx, cfg)
	if err != nil {
		logger.LegacyPrintf("service.cos_transfer", "[COS] video get store failed: %v", err)
		return out, durations, false
	}

	allOK := true
	for i, rawURL := range urls {
		rawURL = strings.TrimSpace(rawURL)
		if rawURL == "" {
			allOK = false
			continue
		}
		cosURL, duration, transferErr := s.transferOneVideo(ctx, store, cfg, rawURL)
		if transferErr != nil {
			logger.LegacyPrintf("service.cos_transfer", "[COS] video transfer failed after retries url=%s err=%v", rawURL, transferErr)
			out[i] = "" // 回退：留空，调用方使用原始 url
			allOK = false
			continue
		}
		logger.LegacyPrintf("service.cos_transfer", "[COS] video transfer succeeded: %s -> %s", rawURL, cosURL)
		out[i] = cosURL
		durations[i] = duration
	}

	logger.LegacyPrintf("service.cos_transfer", "[COS] video transfer completed: %d/%d succeeded", len(urls)-countEmpty(out), len(urls))
	return out, durations, allOK
}

// transferOneVideo 下载单个视频并上传到 COS，最多重试 cosTransferMaxAttempts 次。
// 与 transferOne 的差异：使用视频下载体积上限、视频 key 前缀、视频扩展名推断。
func (s *COSImageTransferService) transferOneVideo(ctx context.Context, store BackupObjectStore, cfg *COSImageConfig, srcURL string) (string, int, error) {
	var lastErr error
	for attempt := 1; attempt <= cosTransferMaxAttempts; attempt++ {
		if ctx.Err() != nil {
			logger.LegacyPrintf("service.cos_transfer", "[COS] video transfer cancelled: %v", ctx.Err())
			return "", 0, ctx.Err()
		}
		logger.LegacyPrintf("service.cos_transfer", "[COS] downloading video (attempt %d/%d): %s", attempt, cosTransferMaxAttempts, srcURL)
		data, contentType, err := s.downloadWithLimit(ctx, srcURL, cosVideoDownloadMaxBytes)
		if err != nil {
			logger.LegacyPrintf("service.cos_transfer", "[COS] video download failed (attempt %d): %v", attempt, err)
			lastErr = err
			continue
		}
		logger.LegacyPrintf("service.cos_transfer", "[COS] video download succeeded: size=%d bytes, content-type=%s", len(data), contentType)
		duration, durationErr := parseVideoDurationSeconds(data)
		if durationErr != nil {
			logger.LegacyPrintf("service.cos_transfer", "[COS] video duration parse failed: url=%s err=%v", srcURL, durationErr)
		}

		key := s.buildVideoKey(cfg, srcURL, contentType)
		logger.LegacyPrintf("service.cos_transfer", "[COS] uploading video (attempt %d): key=%s", attempt, key)
		if _, err := store.Upload(ctx, key, bytes.NewReader(data), contentType); err != nil {
			logger.LegacyPrintf("service.cos_transfer", "[COS] video upload failed (attempt %d): %v", attempt, err)
			lastErr = err
			continue
		}
		cosURL := s.buildPublicURL(cfg, key)
		logger.LegacyPrintf("service.cos_transfer", "[COS] video upload succeeded: %s", cosURL)
		return cosURL, duration, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("video transfer failed")
	}
	logger.LegacyPrintf("service.cos_transfer", "[COS] all video transfer attempts failed: %v", lastErr)
	return "", 0, lastErr
}

// ProbeVideoDuration downloads a trusted upstream result and reads its MP4
// movie header. It is used only when the provider response omitted duration.
func (s *COSImageTransferService) ProbeVideoDuration(ctx context.Context, srcURL string) (int, error) {
	if s == nil {
		return 0, errors.New("video duration probe is unavailable")
	}
	data, _, err := s.downloadWithLimit(ctx, srcURL, cosVideoDownloadMaxBytes)
	if err != nil {
		return 0, err
	}
	return parseVideoDurationSeconds(data)
}

// TransferImages 将一批 fal 图片 url 转存到 COS。
//
// 返回与输入等长的 cos url 列表：成功项为 COS 地址，失败（重试耗尽）项为空字符串，
// 调用方据此回退使用原始 fal url。第二个返回值表示是否全部成功。
func (s *COSImageTransferService) TransferImages(ctx context.Context, urls []string) ([]string, bool) {
	out, _, allOK := s.TransferImagesWithSizes(ctx, urls)
	return out, allOK
}

// TransferImagesWithSizes 转存图片，同时从已下载的实际图片文件中解析每张图的宽高。
// sizes 与 urls 等长；尺寸解析失败不会影响转存结果。
func (s *COSImageTransferService) TransferImagesWithSizes(ctx context.Context, urls []string) ([]string, []string, bool) {
	out := make([]string, len(urls))
	sizes := make([]string, len(urls))
	if len(urls) == 0 {
		return out, sizes, true
	}

	cfg, err := s.loadConfig(ctx)
	if err != nil || cfg == nil || !cfg.Enabled || !cfg.IsConfigured() {
		logger.LegacyPrintf("service.cos_transfer", "[COS] transfer disabled or not configured: enabled=%v, configured=%v, err=%v", cfg != nil && cfg.Enabled, cfg != nil && cfg.IsConfigured(), err)
		return out, sizes, false
	}
	logger.LegacyPrintf("service.cos_transfer", "[COS] transfer enabled, processing %d images", len(urls))

	store, err := s.getOrCreateStore(ctx, cfg)
	if err != nil {
		logger.LegacyPrintf("service.cos_transfer", "[COS] get store failed: %v", err)
		return out, sizes, false
	}

	allOK := true
	for i, rawURL := range urls {
		rawURL = strings.TrimSpace(rawURL)
		if rawURL == "" {
			allOK = false
			continue
		}
		cosURL, imageSize, transferErr := s.transferOne(ctx, store, cfg, rawURL)
		sizes[i] = imageSize
		if transferErr != nil {
			logger.LegacyPrintf("service.cos_transfer", "[COS] transfer failed after retries url=%s err=%v", rawURL, transferErr)
			out[i] = ""
			allOK = false
			continue
		}
		logger.LegacyPrintf("service.cos_transfer", "[COS] transfer succeeded: %s -> %s size=%s", rawURL, cosURL, imageSize)
		out[i] = cosURL
	}
	logger.LegacyPrintf("service.cos_transfer", "[COS] transfer completed: %d/%d succeeded", len(urls)-countEmpty(out), len(urls))
	return out, sizes, allOK
}

// DetectImageSizes 下载上游图片并读取文件头中的实际宽高，不依赖 COS 配置。
func (s *COSImageTransferService) DetectImageSizes(ctx context.Context, urls []string) []string {
	sizes := make([]string, len(urls))
	if s == nil {
		return sizes
	}
	for i, rawURL := range urls {
		rawURL = strings.TrimSpace(rawURL)
		if rawURL == "" {
			continue
		}
		data, _, err := s.download(ctx, rawURL)
		if err != nil {
			logger.LegacyPrintf("service.cos_transfer", "[COS] detect image size download failed: url=%s err=%v", rawURL, err)
			continue
		}
		size, err := decodeDownloadedImageSize(data)
		if err != nil {
			logger.LegacyPrintf("service.cos_transfer", "[COS] detect image size decode failed: url=%s err=%v", rawURL, err)
			continue
		}
		sizes[i] = size
		logger.LegacyPrintf("service.cos_transfer", "[COS] detected image size: url=%s size=%s", rawURL, size)
	}
	return sizes
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
func (s *COSImageTransferService) transferOne(ctx context.Context, store BackupObjectStore, cfg *COSImageConfig, srcURL string) (string, string, error) {
	var lastErr error
	imageSize := ""
	for attempt := 1; attempt <= cosTransferMaxAttempts; attempt++ {
		if ctx.Err() != nil {
			logger.LegacyPrintf("service.cos_transfer", "[COS] transfer cancelled: %v", ctx.Err())
			return "", imageSize, ctx.Err()
		}
		logger.LegacyPrintf("service.cos_transfer", "[COS] downloading image (attempt %d/%d): %s", attempt, cosTransferMaxAttempts, srcURL)
		data, contentType, err := s.download(ctx, srcURL)
		if err != nil {
			logger.LegacyPrintf("service.cos_transfer", "[COS] download failed (attempt %d): %v", attempt, err)
			lastErr = err
			continue
		}
		logger.LegacyPrintf("service.cos_transfer", "[COS] download succeeded: size=%d bytes, content-type=%s", len(data), contentType)
		if detected, detectErr := decodeDownloadedImageSize(data); detectErr != nil {
			logger.LegacyPrintf("service.cos_transfer", "[COS] image size decode failed: url=%s err=%v", srcURL, detectErr)
		} else {
			imageSize = detected
		}

		key := s.buildKey(cfg, srcURL, contentType)
		logger.LegacyPrintf("service.cos_transfer", "[COS] uploading to COS (attempt %d): key=%s", attempt, key)
		if _, err := store.Upload(ctx, key, bytes.NewReader(data), contentType); err != nil {
			logger.LegacyPrintf("service.cos_transfer", "[COS] upload failed (attempt %d): %v", attempt, err)
			lastErr = err
			continue
		}
		cosURL := s.buildPublicURL(cfg, key)
		logger.LegacyPrintf("service.cos_transfer", "[COS] upload succeeded: %s", cosURL)
		return cosURL, imageSize, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("transfer failed")
	}
	logger.LegacyPrintf("service.cos_transfer", "[COS] all transfer attempts failed: %v", lastErr)
	return "", imageSize, lastErr
}

func decodeDownloadedImageSize(data []byte) (string, error) {
	if len(data) == 0 {
		return "", fmt.Errorf("empty image data")
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("decode image config: %w", err)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 {
		return "", fmt.Errorf("invalid image dimensions: %dx%d", cfg.Width, cfg.Height)
	}
	return fmt.Sprintf("%dx%d", cfg.Width, cfg.Height), nil
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
	return s.downloadWithLimit(ctx, srcURL, cosDownloadMaxBytes)
}

// downloadWithLimit 通用下载：由调用方指定单次下载的最大字节数上限。
// 图片路径复用 cosDownloadMaxBytes；视频路径使用 cosVideoDownloadMaxBytes。
//
// 走 httpClient（普通 client），仅供"可信来源"使用 —— 即 URL 来自上游 API
// 响应或管理员配置。用户可控的 URL 必须走 DownloadUntrustedToBytes。
func (s *COSImageTransferService) downloadWithLimit(ctx context.Context, srcURL string, maxBytes int64) ([]byte, string, error) {
	return s.doDownload(ctx, s.httpClient, srcURL, maxBytes)
}

// doDownload 实际执行 GET 并按 maxBytes 截断读取。client 由调用方按信任级别选择。
func (s *COSImageTransferService) doDownload(
	ctx context.Context,
	client *http.Client,
	srcURL string,
	maxBytes int64,
) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srcURL, nil)
	if err != nil {
		return nil, "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", fmt.Errorf("download: status %d", resp.StatusCode)
	}
	if maxBytes <= 0 {
		maxBytes = cosDownloadMaxBytes
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes))
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

// buildVideoKey 生成视频专用 COS 对象 key：videos/{yyyy/mm/dd}/{uuid}{ext}
// 视频前缀固定为 videos/，与图片存储分离，便于对象存储侧做生命周期/权限治理。
func (s *COSImageTransferService) buildVideoKey(cfg *COSImageConfig, srcURL, contentType string) string {
	_ = cfg // 保留签名对称，未来若允许自定义 video 前缀可从 cfg 读取
	ext := videoExtFromURLOrType(srcURL, contentType)
	return fmt.Sprintf("%s/%s/%s%s", cosVideoKeyPrefix, time.Now().Format("2006/01/02"), uuid.NewString(), ext)
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

// videoExtFromURLOrType 推断视频扩展名（含点），优先从 url 路径，再从 content-type。
// 无法识别时兜底 ".mp4"（fal 视频产物绝大多数为 mp4）。
func videoExtFromURLOrType(srcURL, contentType string) string {
	if ext := strings.ToLower(path.Ext(stripQuery(srcURL))); isVideoExt(ext) {
		return ext
	}
	switch strings.ToLower(strings.TrimSpace(strings.SplitN(contentType, ";", 2)[0])) {
	case "video/mp4":
		return ".mp4"
	case "video/quicktime":
		return ".mov"
	case "video/webm":
		return ".webm"
	case "video/x-m4v":
		return ".m4v"
	default:
		return ".mp4"
	}
}

func isVideoExt(ext string) bool {
	switch ext {
	case ".mp4", ".mov", ".webm", ".m4v":
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

// -----------------------------------------------------------------------------
// 用户素材库专用扩展 API
//
// 与 UploadImageBytes / TransferImages 的差别：
//   1. 允许调用方指定完整对象 key，跳过 cfg.Prefix 的自动拼接，从而支持
//      "users/u_<opaque>/materials/YYYY/MM/{uuid}.{ext}" 这类按用户隔离的目录结构；
//   2. 支持任意 MIME（image/audio/video），不再限定 image/*；
//   3. 由调用方（UserMaterialService）负责元信息落库，本层只做"字节到桶"的搬运。
//
// 这些方法有意与视频 / 图片转存的原有路径解耦，避免污染 async_media / async_video
// 已有的稳定行为。
// -----------------------------------------------------------------------------

// UploadBytesWithKey 把内存中的字节以调用方指定的完整 key 上传到 COS。
//
// - key 必须是完整对象 key（不再拼 cfg.Prefix）；调用方自行保证唯一性与目录结构。
// - contentType 允许为空：为空时会自动 http.DetectContentType 嗅探。
// - 上传成功后返回对外可访问 URL（走 cfg.PublicBaseURL / endpoint 的既有规则）。
// - COS 未启用或未配置时返回 ("", ErrCOSNotConfigured)，调用方据此提示用户先在后台配好 COS。
func (s *COSImageTransferService) UploadBytesWithKey(ctx context.Context, key string, data []byte, contentType string) (string, error) {
	key = strings.TrimLeft(strings.TrimSpace(key), "/")
	if key == "" {
		return "", fmt.Errorf("empty cos key")
	}
	if len(data) == 0 {
		return "", fmt.Errorf("empty bytes")
	}
	cfg, err := s.loadConfig(ctx)
	if err != nil {
		return "", err
	}
	if cfg == nil || !cfg.Enabled || !cfg.IsConfigured() {
		return "", ErrCOSNotConfigured
	}
	store, err := s.getOrCreateStore(ctx, cfg)
	if err != nil {
		return "", err
	}
	ct := strings.TrimSpace(contentType)
	if ct == "" {
		ct = http.DetectContentType(data)
	}
	// 与 UploadImageBytes 一致：截掉 ";charset=..." 之类的参数，避免 S3 v4 签名不匹配。
	if i := strings.IndexByte(ct, ';'); i >= 0 {
		ct = strings.TrimSpace(ct[:i])
	}
	if ct == "" {
		ct = "application/octet-stream"
	}
	logger.LegacyPrintf("service.cos_transfer",
		"[COS] UploadBytesWithKey uploading: key=%s size=%d content-type=%s", key, len(data), ct)
	if _, err := store.Upload(ctx, key, bytes.NewReader(data), ct); err != nil {
		logger.LegacyPrintf("service.cos_transfer", "[COS] UploadBytesWithKey failed: %v", err)
		return "", err
	}
	cosURL := s.buildPublicURL(cfg, key)
	logger.LegacyPrintf("service.cos_transfer", "[COS] UploadBytesWithKey succeeded: %s", cosURL)
	return cosURL, nil
}

// UploadReaderWithKey streams a known-size body to a caller-selected object key.
func (s *COSImageTransferService) UploadReaderWithKey(ctx context.Context, key string, body io.Reader, size int64, contentType string) (string, error) {
	key, err := sanitizeObjectKey(key)
	if err != nil {
		return "", err
	}
	if body == nil || size <= 0 {
		return "", infraerrors.BadRequest("EMPTY_FILE", "uploaded file is empty")
	}
	cfg, err := s.loadConfig(ctx)
	if err != nil {
		return "", err
	}
	if cfg == nil || !cfg.Enabled || !cfg.IsConfigured() {
		return "", ErrCOSNotConfigured
	}
	store, err := s.getOrCreateStore(ctx, cfg)
	if err != nil {
		return "", err
	}
	uploader, ok := store.(BackupObjectSizedUploader)
	if !ok {
		return "", ErrCOSStreamingUnsupported
	}
	ct := strings.TrimSpace(strings.SplitN(contentType, ";", 2)[0])
	if ct == "" {
		ct = "application/octet-stream"
	}
	if err := uploader.UploadSized(ctx, key, io.LimitReader(body, size), size, ct); err != nil {
		return "", err
	}
	return s.buildPublicURL(cfg, key), nil
}

// PublicURLToKey validates a URL against the configured public COS base and
// returns the decoded object key relative to that base path.
func (s *COSImageTransferService) PublicURLToKey(ctx context.Context, rawURL string) (string, error) {
	cfg, err := s.loadConfig(ctx)
	if err != nil {
		return "", err
	}
	if cfg == nil || !cfg.Enabled || !cfg.IsConfigured() {
		return "", ErrCOSNotConfigured
	}
	baseURL := s.publicBaseURL(cfg)
	value := strings.TrimSpace(rawURL)
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", infraerrors.BadRequest("INVALID_FILE_URL", "url must be an unqualified public COS URL")
	}
	base, err := url.Parse(baseURL)
	if err != nil || base.Scheme == "" || base.Host == "" {
		return "", ErrCOSNotConfigured
	}
	if !strings.EqualFold(parsed.Scheme, base.Scheme) || !strings.EqualFold(parsed.Hostname(), base.Hostname()) || effectiveURLPort(parsed) != effectiveURLPort(base) {
		return "", infraerrors.BadRequest("INVALID_FILE_URL", "url must use the configured public COS domain")
	}
	basePath := strings.TrimRight(base.Path, "/")
	prefix := basePath + "/"
	if !strings.HasPrefix(parsed.Path, prefix) {
		return "", infraerrors.BadRequest("INVALID_FILE_URL", "url is outside the configured COS path")
	}
	key, err := sanitizeObjectKey(strings.TrimPrefix(parsed.Path, prefix))
	if err != nil {
		return "", infraerrors.BadRequest("INVALID_FILE_URL", "url contains an invalid object key")
	}
	return key, nil
}

// MoveFile performs the object-store rename primitive: server-side copy then delete.
func (s *COSImageTransferService) MoveFile(ctx context.Context, srcKey, dstKey string) (string, error) {
	srcKey, err := sanitizeObjectKey(srcKey)
	if err != nil {
		return "", err
	}
	dstKey, err = sanitizeObjectKey(dstKey)
	if err != nil {
		return "", err
	}
	if srcKey == dstKey {
		return "", infraerrors.BadRequest("INVALID_FILE_MOVE", "source and destination are identical")
	}
	cfg, err := s.loadConfig(ctx)
	if err != nil {
		return "", err
	}
	if cfg == nil || !cfg.Enabled || !cfg.IsConfigured() {
		return "", ErrCOSNotConfigured
	}
	store, err := s.getOrCreateStore(ctx, cfg)
	if err != nil {
		return "", err
	}
	copier, ok := store.(BackupObjectCopier)
	if !ok {
		return "", ErrCOSMoveUnsupported
	}
	if err := copier.CopyObject(ctx, srcKey, dstKey); err != nil {
		return "", err
	}
	if err := store.Delete(ctx, srcKey); err != nil {
		return "", fmt.Errorf("copied to %s but failed to delete source %s: %w", dstKey, srcKey, err)
	}
	return s.buildPublicURL(cfg, dstKey), nil
}

func (s *COSImageTransferService) publicBaseURL(cfg *COSImageConfig) string {
	if base := strings.TrimRight(cfg.PublicBaseURL, "/"); base != "" {
		return base
	}
	endpoint := strings.TrimRight(cfg.Endpoint, "/")
	if cfg.ForcePathStyle {
		return endpoint + "/" + cfg.Bucket
	}
	if scheme, host, ok := splitScheme(endpoint); ok {
		return fmt.Sprintf("%s://%s.%s", scheme, cfg.Bucket, host)
	}
	return endpoint + "/" + cfg.Bucket
}

// ErrUntrustedURLBlocked 表示用户提供的 URL 指向内网/回环/云元数据地址，被 SSRF 策略拒绝。
// 调用方应把它映射为 4xx（是用户输入问题），而不是 5xx。
var ErrUntrustedURLBlocked = errors.New("url blocked by security policy")

// DownloadUntrustedToBytes 拉取**用户可控** url 对应的字节并返回，附带上游 Content-Type。
// maxBytes<=0 时按图片限制（cosDownloadMaxBytes）；调用方需要根据素材类型选择合适上限。
//
// 与 downloadWithLimit 的区别：本方法专供"URL 由终端用户提供"的场景（素材库
// import-url）。这类入口若不加防护就是一个 SSRF 跳板 —— 任何登录用户都能让
// 服务端去请求内网服务、云元数据端点（169.254.169.254），并把响应体转存到
// 对象存储后拿到公网可读地址，等于任意内网数据外带。
//
// 三层防护：
//  1. 只允许 http / https，拒绝 file:// gopher:// 等 scheme；
//  2. 请求前预检 hostname 的所有 A/AAAA 记录，命中私网/回环/link-local 即拒绝
//     （错误信息明确，便于用户自查）；
//  3. 走 untrustedClient，其 DialContext 在 socket 层对每个候选 IP 二次校验 ——
//     这一层才是真正兜住 DNS rebinding 与 30x 重定向到内网的关键。
func (s *COSImageTransferService) DownloadUntrustedToBytes(
	ctx context.Context,
	srcURL string,
	maxBytes int64,
) ([]byte, string, error) {
	srcURL = strings.TrimSpace(srcURL)
	if srcURL == "" {
		return nil, "", fmt.Errorf("empty source url")
	}
	u, err := url.Parse(srcURL)
	if err != nil {
		return nil, "", fmt.Errorf("invalid source url")
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
	default:
		return nil, "", fmt.Errorf("unsupported url scheme %q", u.Scheme)
	}
	host := u.Hostname()
	if host == "" {
		return nil, "", fmt.Errorf("invalid source url: missing host")
	}
	blocked, lookupErr := isPrivateOrLoopbackHost(ctx, host)
	if lookupErr != nil {
		return nil, "", fmt.Errorf("resolve %s: %w", host, lookupErr)
	}
	if blocked {
		return nil, "", ErrUntrustedURLBlocked
	}

	if maxBytes <= 0 {
		maxBytes = cosDownloadMaxBytes
	}
	client := s.untrustedClient
	if client == nil {
		// 兜底：若服务由零值结构体构造（测试里可能出现），也不要退化成无防护 client。
		client = newSSRFSafeHTTPClient(cosDownloadTimeout)
	}
	data, ct, err := s.doDownload(ctx, client, srcURL, maxBytes)
	if err != nil {
		// dial 层拒绝时错误里带 "blocked by SSRF policy"，归一成同一个哨兵错误，
		// 让 handler 统一按 4xx 处理，也避免把内网拓扑细节透给用户。
		if strings.Contains(err.Error(), "blocked by SSRF policy") {
			return nil, "", ErrUntrustedURLBlocked
		}
		return nil, "", err
	}
	return data, ct, nil
}

// ExtFromURLOrType 供外部（UserMaterialService）复用的扩展名推断。
// 依次优先：url 路径后缀（图片/视频白名单）→ Content-Type 主类型映射 → 兜底 ".bin"。
func ExtFromURLOrType(srcURL, contentType string) string {
	if ext := strings.ToLower(path.Ext(stripQuery(srcURL))); ext != "" {
		if isImageExt(ext) || isVideoExt(ext) || isAudioExt(ext) {
			return ext
		}
	}
	ct := strings.ToLower(strings.TrimSpace(strings.SplitN(contentType, ";", 2)[0]))
	switch ct {
	case "image/png":
		return ".png"
	case "image/jpeg", "image/jpg":
		return ".jpg"
	case "image/webp":
		return ".webp"
	case "image/gif":
		return ".gif"
	case "video/mp4":
		return ".mp4"
	case "video/quicktime":
		return ".mov"
	case "video/webm":
		return ".webm"
	case "video/x-m4v":
		return ".m4v"
	case "audio/mpeg", "audio/mp3":
		return ".mp3"
	case "audio/wav", "audio/x-wav":
		return ".wav"
	case "audio/ogg":
		return ".ogg"
	case "audio/mp4", "audio/x-m4a":
		return ".m4a"
	case "audio/aac":
		return ".aac"
	case "audio/flac":
		return ".flac"
	}
	return ".bin"
}

// KindFromContentType 按 MIME 主类型分流出素材大类：image / audio / video / other。
// 供 UserMaterialService 决定素材落到哪个 kind 分区，也决定列表页/输入控件的过滤条件。
func KindFromContentType(contentType string) string {
	ct := strings.ToLower(strings.TrimSpace(strings.SplitN(contentType, ";", 2)[0]))
	switch {
	case strings.HasPrefix(ct, "image/"):
		return "image"
	case strings.HasPrefix(ct, "audio/"):
		return "audio"
	case strings.HasPrefix(ct, "video/"):
		return "video"
	default:
		return "other"
	}
}

// isAudioExt：素材库需要识别音频扩展名，图片/视频的判定复用已有 isImageExt/isVideoExt。
func isAudioExt(ext string) bool {
	switch ext {
	case ".mp3", ".wav", ".ogg", ".m4a", ".aac", ".flac":
		return true
	default:
		return false
	}
}

// DeleteObject 从 COS 删除单个对象 key（不校验归属，调用方需先做业务侧权限校验）。
// 用于素材库软删后台清理，或用户主动删除。
func (s *COSImageTransferService) DeleteObject(ctx context.Context, key string) error {
	key = strings.TrimLeft(strings.TrimSpace(key), "/")
	if key == "" {
		return fmt.Errorf("empty cos key")
	}
	cfg, err := s.loadConfig(ctx)
	if err != nil {
		return err
	}
	if cfg == nil || !cfg.Enabled || !cfg.IsConfigured() {
		return ErrCOSNotConfigured
	}
	store, err := s.getOrCreateStore(ctx, cfg)
	if err != nil {
		return err
	}
	return store.Delete(ctx, key)
}
