package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const defaultImageMaxDownloadBytes int64 = 32 << 20 // 32 MiB

// ImageStorage stores generated images in a private object store.
//
// 这是对象存储的可插拔抽象：适配一个新的对象存储厂商，只需实现本接口
// （例如包一个厂商 SDK），无需改动任务/网关逻辑。仓库内自带一个 S3 兼容实现
// （repository.S3ImageStorage），适用于 AWS S3 / Cloudflare R2 / 阿里云 OSS / MinIO 等。
type ImageStorage interface {
	// Save writes image bytes. Implementations must not make the object public.
	Save(ctx context.Context, key, contentType string, data []byte) error
	// Load reads an object through server-side credentials and enforces maxBytes.
	Load(ctx context.Context, key string, maxBytes int64) (data []byte, contentType string, err error)
	// Size returns the stored object size without downloading the image body.
	Size(ctx context.Context, key string) (int64, error)
	// Delete 删除指定对象。实现必须保持幂等：对象已不存在时也应视为成功。
	Delete(ctx context.Context, key string) error
}

type StoredImageObject struct {
	Key  string
	Size int64
}

// ImageResultUploader 是 ImageStorage 的上层编排器（与具体厂商无关）：
// 把上游生图响应里的每张图片（b64_json 解码 / url 下载）转存到对象存储，
// 并把响应结果改写为不含图片字节或存储地址的紧凑 JSON，从而避免大 base64
// 落 Redis，也避免对象存储地址泄露给调用方。
type ImageResultUploader struct {
	storage          ImageStorage
	storageIdentity  string
	httpClient       *http.Client
	prefix           string
	maxDownloadBytes int64
}

// NewImageResultUploader 构造一个 uploader；storage 为 nil 时 Rewrite 直接透传。
func NewImageResultUploader(storage ImageStorage, prefix string, maxDownloadBytes int64, httpClient *http.Client) *ImageResultUploader {
	if httpClient == nil {
		httpClient = defaultImageDownloadHTTPClient()
	}
	if maxDownloadBytes <= 0 {
		maxDownloadBytes = defaultImageMaxDownloadBytes
	}
	return &ImageResultUploader{
		storage:          storage,
		httpClient:       httpClient,
		prefix:           prefix,
		maxDownloadBytes: maxDownloadBytes,
	}
}

// StorageIdentity identifies the object-storage target without exposing its
// credentials. Cleanup jobs use it to avoid deleting an old key from a newly
// configured bucket after an administrator changes storage settings.
func (u *ImageResultUploader) StorageIdentity() string {
	if u == nil {
		return ""
	}
	return u.storageIdentity
}

func defaultImageDownloadHTTPClient() *http.Client {
	return &http.Client{Timeout: 60 * time.Second}
}

// Rewrite 将 result（上游生图响应 JSON）里的每张图片转存到对象存储，
// 返回改写后的紧凑结果（data[i] 中的 url 和 b64_json 都被移除）。
// 任一图片转存失败即返回 error（调用方据此将任务标记为失败，绝不把大 blob 落 Redis）。
func (u *ImageResultUploader) Rewrite(ctx context.Context, taskID string, result json.RawMessage) (json.RawMessage, error) {
	out, _, err := u.RewriteWithObjects(ctx, taskID, result)
	return out, err
}

// RewriteWithKeys additionally returns the private object keys needed for
// explicit deletion and TTL cleanup. Keys are never included in public task
// responses.
func (u *ImageResultUploader) RewriteWithKeys(ctx context.Context, taskID string, result json.RawMessage) (out json.RawMessage, resultKeys []string, err error) {
	out, objects, err := u.RewriteWithObjects(ctx, taskID, result)
	if err != nil {
		return nil, nil, err
	}
	resultKeys = make([]string, len(objects))
	for i, object := range objects {
		resultKeys[i] = object.Key
	}
	return out, resultKeys, nil
}

// RewriteWithObjects additionally records the byte size of each stored image.
// This keeps the admin storage overview accurate without downloading objects.
func (u *ImageResultUploader) RewriteWithObjects(ctx context.Context, taskID string, result json.RawMessage) (out json.RawMessage, resultObjects []StoredImageObject, err error) {
	if u == nil || u.storage == nil {
		return result, nil, nil
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(result, &top); err != nil {
		return nil, nil, fmt.Errorf("parse image response: %w", err)
	}
	rawData, ok := top["data"]
	if !ok {
		// 没有 data 数组（结构不符合预期），保持原样返回，交由上层决定。
		return result, nil, nil
	}
	var items []map[string]json.RawMessage
	if err := json.Unmarshal(rawData, &items); err != nil {
		return nil, nil, fmt.Errorf("parse image response data: %w", err)
	}
	if len(items) == 0 {
		return result, nil, nil
	}
	uploadedObjects := make([]StoredImageObject, 0, len(items))
	defer func() {
		if err != nil && len(uploadedObjects) > 0 {
			keys := make([]string, len(uploadedObjects))
			for i, object := range uploadedObjects {
				keys[i] = object.Key
			}
			_ = u.Delete(context.Background(), keys)
			resultObjects = nil
		}
	}()
	for i, item := range items {
		data, contentType, err := u.fetchImageBytes(ctx, item)
		if err != nil {
			return nil, nil, fmt.Errorf("image %d: %w", i, err)
		}
		key := u.buildKey(taskID, i, contentType)
		if err := u.storage.Save(ctx, key, contentType, data); err != nil {
			return nil, nil, fmt.Errorf("image %d: upload to object storage: %w", i, err)
		}
		uploadedObjects = append(uploadedObjects, StoredImageObject{Key: key, Size: int64(len(data))})
		delete(item, "url")
		delete(item, "b64_json")
		items[i] = item
	}
	newData, err := json.Marshal(items)
	if err != nil {
		return nil, nil, fmt.Errorf("encode image response data: %w", err)
	}
	top["data"] = newData
	encoded, err := json.Marshal(top)
	if err != nil {
		return nil, nil, fmt.Errorf("encode image response: %w", err)
	}
	return encoded, uploadedObjects, nil
}

// Load reads a retained image without exposing the object-storage endpoint.
func (u *ImageResultUploader) Load(ctx context.Context, key string) ([]byte, string, error) {
	if u == nil || u.storage == nil {
		return nil, "", errors.New("image object storage is unavailable")
	}
	data, contentType, err := u.storage.Load(ctx, strings.TrimSpace(key), u.maxDownloadBytes)
	if err != nil {
		return nil, "", err
	}
	contentType = strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	if !strings.HasPrefix(contentType, "image/") {
		return nil, "", fmt.Errorf("stored object content type %q is not an image", contentType)
	}
	return data, contentType, nil
}

func (u *ImageResultUploader) Size(ctx context.Context, key string) (int64, error) {
	if u == nil || u.storage == nil {
		return 0, errors.New("image object storage is unavailable")
	}
	return u.storage.Size(ctx, strings.TrimSpace(key))
}

func (u *ImageResultUploader) Delete(ctx context.Context, keys []string) error {
	if len(keys) == 0 {
		return nil
	}
	if u == nil || u.storage == nil {
		return errors.New("image object storage is unavailable")
	}
	for _, key := range keys {
		if key = strings.TrimSpace(key); key == "" {
			continue
		}
		if err := u.storage.Delete(ctx, key); err != nil {
			return fmt.Errorf("delete image object %q: %w", key, err)
		}
	}
	return nil
}

func (u *ImageResultUploader) fetchImageBytes(ctx context.Context, item map[string]json.RawMessage) ([]byte, string, error) {
	if raw, ok := item["b64_json"]; ok {
		var b64 string
		if err := json.Unmarshal(raw, &b64); err == nil {
			if b64 = strings.TrimSpace(b64); b64 != "" {
				data, err := base64.StdEncoding.DecodeString(b64)
				if err != nil {
					return nil, "", fmt.Errorf("decode b64_json: %w", err)
				}
				return data, detectImageContentType(data), nil
			}
		}
	}
	if raw, ok := item["url"]; ok {
		var rawURL string
		if err := json.Unmarshal(raw, &rawURL); err == nil {
			if rawURL = strings.TrimSpace(rawURL); rawURL != "" {
				if len(rawURL) >= len("data:") && strings.EqualFold(rawURL[:len("data:")], "data:") {
					return u.decodeImageDataURL(rawURL)
				}
				return u.download(ctx, rawURL)
			}
		}
	}
	return nil, "", errors.New("image item has neither b64_json nor url")
}

func (u *ImageResultUploader) decodeImageDataURL(rawURL string) ([]byte, string, error) {
	header, payload, ok := strings.Cut(rawURL[len("data:"):], ",")
	if !ok {
		return nil, "", errors.New("decode image data URL: missing comma separator")
	}

	parts := strings.Split(header, ";")
	if strings.TrimSpace(parts[0]) == "" {
		return nil, "", errors.New("decode image data URL: missing media type")
	}
	base64Index := len(parts) - 1
	if base64Index < 1 || !strings.EqualFold(strings.TrimSpace(parts[base64Index]), "base64") {
		for i := 1; i < base64Index; i++ {
			if strings.EqualFold(strings.TrimSpace(parts[i]), "base64") {
				return nil, "", errors.New("decode image data URL: base64 marker must be the final header token")
			}
		}
		return nil, "", errors.New("decode image data URL: payload is not base64 encoded")
	}
	for i := 1; i < base64Index; i++ {
		if strings.EqualFold(strings.TrimSpace(parts[i]), "base64") {
			return nil, "", errors.New("decode image data URL: duplicate base64 marker")
		}
	}
	mediaTypeHeader := strings.Join(parts[:base64Index], ";")
	declaredType, _, err := mime.ParseMediaType(mediaTypeHeader)
	if err != nil {
		return nil, "", fmt.Errorf("decode image data URL: invalid media type: %w", err)
	}
	declaredType = strings.ToLower(declaredType)
	if !strings.HasPrefix(declaredType, "image/") {
		return nil, "", fmt.Errorf("decode image data URL: media type %q is not an image", declaredType)
	}

	limit := u.maxDownloadBytes
	if limit <= 0 {
		limit = defaultImageMaxDownloadBytes
	}
	decoder := base64.NewDecoder(base64.StdEncoding, strings.NewReader(payload))
	data, err := io.ReadAll(io.LimitReader(decoder, limit+1))
	if err != nil {
		return nil, "", fmt.Errorf("decode image data URL base64 payload: %w", err)
	}
	if int64(len(data)) > limit {
		return nil, "", fmt.Errorf("decoded image data URL exceeds %d bytes", limit)
	}

	contentType := detectedImageContentType(data)
	if contentType == "" {
		contentType = declaredType
	}
	return data, contentType, nil
}

func (u *ImageResultUploader) download(ctx context.Context, rawURL string) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, "", fmt.Errorf("build download request: %w", err)
	}
	resp, err := u.httpClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("download image: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, "", fmt.Errorf("download image: unexpected status %d", resp.StatusCode)
	}
	limit := u.maxDownloadBytes
	if limit <= 0 {
		limit = defaultImageMaxDownloadBytes
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, "", fmt.Errorf("read image body: %w", err)
	}
	if int64(len(data)) > limit {
		return nil, "", fmt.Errorf("downloaded image exceeds %d bytes", limit)
	}
	contentType := strings.TrimSpace(strings.Split(resp.Header.Get("Content-Type"), ";")[0])
	if !strings.HasPrefix(contentType, "image/") {
		contentType = detectImageContentType(data)
	}
	return data, contentType, nil
}

func (u *ImageResultUploader) buildKey(taskID string, index int, contentType string) string {
	return u.prefix + taskID + "-" + strconv.Itoa(index) + extensionForContentType(contentType)
}

func detectImageContentType(data []byte) string {
	if ct := detectedImageContentType(data); ct != "" {
		return ct
	}
	return "image/png"
}

func detectedImageContentType(data []byte) string {
	ct := strings.TrimSpace(strings.Split(http.DetectContentType(data), ";")[0])
	if strings.HasPrefix(ct, "image/") {
		return ct
	}
	return ""
}

func extensionForContentType(ct string) string {
	switch {
	case strings.Contains(ct, "png"):
		return ".png"
	case strings.Contains(ct, "jpeg"), strings.Contains(ct, "jpg"):
		return ".jpg"
	case strings.Contains(ct, "webp"):
		return ".webp"
	case strings.Contains(ct, "gif"):
		return ".gif"
	default:
		return ".png"
	}
}
