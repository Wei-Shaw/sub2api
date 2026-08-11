package service

// admin_file_manager.go
//
// 管理员「文件管理」：直接管理图片转存（COS / S3 兼容）桶里的对象 ——
// 浏览、上传、下载、改名、删除。
//
// 为什么需要它：图片转存开启后，桶里会不断堆积 fal 出图/出片的转存件、用户素材
// 库文件等。此前除了登云控制台没有任何办法查看与清理，磁盘账单只能靠猜。
//
// 设计要点：
//   - 复用 COSImageTransferService 的配置与 store 缓存，不再单独维护一份连接；
//   - 列举/复制依赖对象存储的**可选**扩展接口，store 不支持时给出明确错误，
//     而不是静默返回空列表；
//   - 所有入参 key 都过 sanitizeObjectKey：拒绝 ".." 路径穿越、控制字符、超长键；
//   - 下载走预签名直链（临时有效），不经后端转发，避免大文件把后端带宽打满。

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
	"unicode/utf8"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

const (
	// adminFileListDefaultLimit 未指定 limit 时每页条数。
	adminFileListDefaultLimit int32 = 100
	// adminFileListMaxLimit 每页条数硬上限（S3 单次 MaxKeys 也是 1000）。
	adminFileListMaxLimit int32 = 1000
	// adminFileDownloadURLExpiry 下载直链有效期。够用户点开即可，不宜过长。
	adminFileDownloadURLExpiry = 10 * time.Minute
	// adminFileUploadMaxBytes 管理端单次上传上限。
	adminFileUploadMaxBytes int64 = 512 << 20 // 512 MiB
	// adminFileMaxKeyLength 对象键长度上限（S3 规范为 1024 字节）。
	adminFileMaxKeyLength = 1024
)

var (
	// ErrObjectStoreNoListSupport 当前 store 实现不支持列举。
	ErrObjectStoreNoListSupport = errors.New("object store does not support listing")
	// ErrObjectStoreNoCopySupport 当前 store 实现不支持服务端复制（改名依赖它）。
	ErrObjectStoreNoCopySupport = errors.New("object store does not support server-side copy")
	// ErrInvalidObjectKey 对象键非法（空/含 ".."/含控制字符/过长）。
	ErrInvalidObjectKey = errors.New("invalid object key")
	// ErrObjectKeyExists 目标键已存在（改名时拒绝覆盖）。
	ErrObjectKeyExists = errors.New("target object key already exists")
)

// AdminFileEntry 是返回给管理端的单条文件/目录信息。
type AdminFileEntry struct {
	Key          string    `json:"key"`
	Name         string    `json:"name"`
	Size         int64     `json:"size"`
	LastModified time.Time `json:"last_modified"`
	ETag         string    `json:"etag"`
	IsDir        bool      `json:"is_dir"`
	// PublicURL 按 COS 配置拼出的对外地址。桶为私有读时该地址不可直接访问，
	// 仅作展示/复制用；真正下载走 DownloadURL 预签名直链。
	PublicURL string `json:"public_url"`
}

// AdminFileListResult 是一页列举结果。
type AdminFileListResult struct {
	Entries []AdminFileEntry `json:"entries"`
	// NextToken 非空表示还有下一页。
	NextToken string `json:"next_token"`
	// Prefix 本次列举使用的前缀（已归一化），供前端渲染面包屑。
	Prefix string `json:"prefix"`
}

// AdminFileService 提供管理端的对象存储文件管理能力。
type AdminFileService struct {
	cos *COSImageTransferService
}

// NewAdminFileService 构造服务。cos 为 nil 时所有方法返回 ErrCOSNotConfigured。
func NewAdminFileService(cos *COSImageTransferService) *AdminFileService {
	return &AdminFileService{cos: cos}
}

// Enabled 报告图片转存是否已启用且配置完整。
// 管理端据此决定是否展示"文件管理"入口。
func (s *AdminFileService) Enabled(ctx context.Context) bool {
	if s == nil || s.cos == nil {
		return false
	}
	return s.cos.IsEnabled(ctx)
}

// StorageInfo 返回当前生效的桶与前缀，供管理端展示"正在管理哪个桶"。
func (s *AdminFileService) StorageInfo(ctx context.Context) (bucket, prefix string, err error) {
	cfg, err := s.requireConfig(ctx)
	if err != nil {
		return "", "", err
	}
	return cfg.Bucket, cfg.Prefix, nil
}

// List 列举 prefix 下的对象。
//
// delimiter 传 "/" 时按目录层级聚合（推荐，管理端逐层浏览）；传空则递归平铺
// 该前缀下的所有对象（用于"搜索整个桶"这类场景）。
func (s *AdminFileService) List(
	ctx context.Context,
	prefix, delimiter, token string,
	limit int32,
) (*AdminFileListResult, error) {
	cfg, err := s.requireConfig(ctx)
	if err != nil {
		return nil, err
	}
	store, err := s.cos.getOrCreateStore(ctx, cfg)
	if err != nil {
		return nil, err
	}
	lister, ok := store.(BackupObjectLister)
	if !ok {
		return nil, ErrObjectStoreNoListSupport
	}

	// prefix 允许为空（列举桶根），但不允许穿越。
	prefix = normalizeKeyPrefix(prefix)
	if prefix != "" && !isSafeObjectKey(prefix) {
		return nil, ErrInvalidObjectKey
	}
	if limit <= 0 {
		limit = adminFileListDefaultLimit
	}
	if limit > adminFileListMaxLimit {
		limit = adminFileListMaxLimit
	}

	page, err := lister.ListObjects(ctx, prefix, delimiter, token, limit)
	if err != nil {
		return nil, err
	}

	res := &AdminFileListResult{
		Entries:   make([]AdminFileEntry, 0, len(page.Entries)),
		NextToken: page.NextToken,
		Prefix:    prefix,
	}
	for _, e := range page.Entries {
		entry := AdminFileEntry{
			Key:          e.Key,
			Name:         displayNameForKey(e.Key, prefix, e.IsDir),
			Size:         e.Size,
			LastModified: e.LastModified,
			ETag:         e.ETag,
			IsDir:        e.IsDir,
		}
		if !e.IsDir {
			entry.PublicURL = s.cos.buildPublicURL(cfg, e.Key)
		}
		res.Entries = append(res.Entries, entry)
	}
	return res, nil
}

// DownloadURL 生成对象的预签名下载直链（有效期 adminFileDownloadURLExpiry）。
//
// 之所以返回直链而不是由后端读流转发：管理端下载的往往是几十上百 MB 的视频，
// 经后端转发会白占一份带宽与内存。预签名直链由客户端直连对象存储。
func (s *AdminFileService) DownloadURL(ctx context.Context, key string) (string, error) {
	cfg, err := s.requireConfig(ctx)
	if err != nil {
		return "", err
	}
	key, err = sanitizeObjectKey(key)
	if err != nil {
		return "", err
	}
	store, err := s.cos.getOrCreateStore(ctx, cfg)
	if err != nil {
		return "", err
	}
	return store.PresignURL(ctx, key, adminFileDownloadURLExpiry)
}

// Upload 上传一个对象。默认拒绝覆盖；只有 overwrite=true 才允许替换同名对象。
//
// key 为完整对象键；调用方（handler）负责把"当前目录 + 文件名"拼好。
func (s *AdminFileService) Upload(
	ctx context.Context,
	key string,
	data []byte,
	contentType string,
	overwrite bool,
) (*AdminFileEntry, error) {
	cfg, err := s.requireConfig(ctx)
	if err != nil {
		return nil, err
	}
	key, err = sanitizeObjectKey(key)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("empty file")
	}
	if int64(len(data)) > adminFileUploadMaxBytes {
		return nil, fmt.Errorf("file too large: %d bytes (max %d)", len(data), adminFileUploadMaxBytes)
	}
	if !overwrite {
		store, storeErr := s.cos.getOrCreateStore(ctx, cfg)
		if storeErr != nil {
			return nil, storeErr
		}
		exists, existsErr := s.objectExists(ctx, store, key)
		if existsErr != nil {
			return nil, existsErr
		}
		if exists {
			return nil, ErrObjectKeyExists
		}
	}

	ct := strings.TrimSpace(contentType)
	if ct == "" {
		ct = http.DetectContentType(data)
	}
	if _, err := s.cos.UploadBytesWithKey(ctx, key, data, ct); err != nil {
		return nil, err
	}
	logger.LegacyPrintf("service.admin_file", "[FILES] uploaded key=%s size=%d", key, len(data))
	return &AdminFileEntry{
		Key:          key,
		Name:         path.Base(key),
		Size:         int64(len(data)),
		LastModified: time.Now(),
		PublicURL:    s.cos.buildPublicURL(cfg, key),
	}, nil
}

// ImportFromURL 安全下载外部 URL，并把文件写入指定目录。
// srcURL 属于用户可控输入，必须复用 DownloadUntrustedToBytes 的 SSRF 防护。
func (s *AdminFileService) ImportFromURL(
	ctx context.Context,
	prefix, name, srcURL string,
	overwrite bool,
) (*AdminFileEntry, error) {
	cfg, err := s.requireConfig(ctx)
	if err != nil {
		return nil, err
	}
	prefix, name, err = normalizeAdminImportTarget(prefix, name)
	if err != nil {
		return nil, err
	}
	srcURL = strings.TrimSpace(srcURL)
	parsed, err := url.Parse(srcURL)
	if err != nil || parsed == nil {
		return nil, infraerrors.BadRequest("INVALID_URL", "url must start with http(s)://")
	}
	scheme := strings.ToLower(parsed.Scheme)
	if parsed.Hostname() == "" || (scheme != "http" && scheme != "https") {
		return nil, infraerrors.BadRequest("INVALID_URL", "url must start with http(s)://")
	}
	// URL 路径能确定文件名时，下载前先检查冲突，避免拉完大文件才提示覆盖。
	if name == "" {
		if inferred := fileNameFromURLPath(parsed.Path); inferred != "" {
			name = inferred
		}
	}
	if !overwrite && name != "" {
		store, storeErr := s.cos.getOrCreateStore(ctx, cfg)
		if storeErr != nil {
			return nil, storeErr
		}
		exists, existsErr := s.objectExists(ctx, store, prefix+name)
		if existsErr != nil {
			return nil, existsErr
		}
		if exists {
			return nil, ErrObjectKeyExists
		}
	}

	data, contentType, err := s.cos.DownloadUntrustedToBytes(ctx, srcURL, adminFileUploadMaxBytes)
	if err != nil {
		if errors.Is(err, ErrUntrustedURLBlocked) {
			return nil, infraerrors.BadRequest("URL_BLOCKED", "url points to a disallowed address")
		}
		return nil, infraerrors.BadRequest("URL_FETCH_FAILED", "fetch url failed: "+err.Error())
	}
	if len(data) == 0 {
		return nil, infraerrors.BadRequest("EMPTY_REMOTE_FILE", "remote file is empty")
	}

	if name == "" {
		name = adminImportFileName(srcURL, contentType)
	}
	return s.Upload(ctx, prefix+name, data, contentType, overwrite)
}

func normalizeAdminImportTarget(prefix, name string) (string, string, error) {
	prefix = normalizeKeyPrefix(prefix)
	if prefix != "" {
		if !isSafeObjectKey(prefix) {
			return "", "", ErrInvalidObjectKey
		}
		if !strings.HasSuffix(prefix, "/") {
			prefix += "/"
		}
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return prefix, "", nil
	}
	name = path.Base(strings.ReplaceAll(name, `\`, "/"))
	if name == "." || name == "/" {
		return "", "", ErrInvalidObjectKey
	}
	if _, err := sanitizeObjectKey(prefix + name); err != nil {
		return "", "", err
	}
	return prefix, name, nil
}

func adminImportFileName(srcURL, contentType string) string {
	parsed, err := url.Parse(srcURL)
	if err == nil {
		if name := fileNameFromURLPath(parsed.Path); name != "" {
			return name
		}
	}
	return "download" + ExtFromURLOrType("", contentType)
}

func fileNameFromURLPath(urlPath string) string {
	name := path.Base(strings.TrimSuffix(urlPath, "/"))
	if name == "" || name == "." || name == "/" {
		return ""
	}
	return name
}

// Rename 把 srcKey 改名/移动到 dstKey。
//
// 对象存储没有 rename：实现为服务端 CopyObject + 删除源对象。
// 这不是原子操作 —— copy 成功但 delete 失败时会留下一份副本，此时返回错误，
// 让管理员看到并自行处理（比静默留下孤儿对象要好）。
//
// 覆盖保护：dstKey 已存在时直接拒绝。改名是高频误操作场景，静默覆盖会丢数据。
func (s *AdminFileService) Rename(ctx context.Context, srcKey, dstKey string) (*AdminFileEntry, error) {
	cfg, err := s.requireConfig(ctx)
	if err != nil {
		return nil, err
	}
	srcKey, err = sanitizeObjectKey(srcKey)
	if err != nil {
		return nil, fmt.Errorf("source key: %w", err)
	}
	dstKey, err = sanitizeObjectKey(dstKey)
	if err != nil {
		return nil, fmt.Errorf("target key: %w", err)
	}
	if srcKey == dstKey {
		return nil, fmt.Errorf("source and target keys are identical")
	}
	store, err := s.cos.getOrCreateStore(ctx, cfg)
	if err != nil {
		return nil, err
	}
	copier, ok := store.(BackupObjectCopier)
	if !ok {
		return nil, ErrObjectStoreNoCopySupport
	}

	// 覆盖检查：用精确前缀列举探测目标是否已存在。
	// 没有直接的 HeadObject 抽象，用 lister 探一条即可，代价可忽略。
	if exists, checkErr := s.objectExists(ctx, store, dstKey); checkErr == nil && exists {
		return nil, ErrObjectKeyExists
	}

	if err := copier.CopyObject(ctx, srcKey, dstKey); err != nil {
		return nil, err
	}
	if err := store.Delete(ctx, srcKey); err != nil {
		// copy 已成功：明确告知"新键已生成但旧键未删除"，避免管理员以为整体失败
		// 而重试，结果又多出一份。
		logger.LegacyPrintf("service.admin_file",
			"[FILES] rename copied but source delete failed src=%s dst=%s err=%v", srcKey, dstKey, err)
		return nil, fmt.Errorf("copied to %s but failed to delete source %s: %w", dstKey, srcKey, err)
	}
	logger.LegacyPrintf("service.admin_file", "[FILES] renamed %s -> %s", srcKey, dstKey)
	return &AdminFileEntry{
		Key:          dstKey,
		Name:         path.Base(dstKey),
		LastModified: time.Now(),
		PublicURL:    s.cos.buildPublicURL(cfg, dstKey),
	}, nil
}

// Delete 删除单个对象。
func (s *AdminFileService) Delete(ctx context.Context, key string) error {
	if _, err := s.requireConfig(ctx); err != nil {
		return err
	}
	key, err := sanitizeObjectKey(key)
	if err != nil {
		return err
	}
	if err := s.cos.DeleteObject(ctx, key); err != nil {
		return err
	}
	logger.LegacyPrintf("service.admin_file", "[FILES] deleted key=%s", key)
	return nil
}

// DeleteMany 批量删除，返回成功条数与逐条错误。
//
// 逐个删除而不是 DeleteObjects 批量 API：批量 API 未被抽象在接口里，且管理端
// 单次选中量有限（前端限制 100 条）。部分失败不影响其余，错误按 key 汇总返回。
func (s *AdminFileService) DeleteMany(ctx context.Context, keys []string) (deleted int, failures map[string]string) {
	failures = make(map[string]string)
	for _, k := range keys {
		if err := s.Delete(ctx, k); err != nil {
			failures[k] = err.Error()
			continue
		}
		deleted++
	}
	return deleted, failures
}

// AdminFileUploadMaxBytes 暴露上传上限给 handler 层设置 body 读取限制。
func AdminFileUploadMaxBytes() int64 { return adminFileUploadMaxBytes }

// requireConfig 读取配置并要求"已启用且配置完整"。
func (s *AdminFileService) requireConfig(ctx context.Context) (*COSImageConfig, error) {
	if s == nil || s.cos == nil {
		return nil, ErrCOSNotConfigured
	}
	cfg, err := s.cos.loadConfig(ctx)
	if err != nil {
		return nil, err
	}
	if cfg == nil || !cfg.Enabled || !cfg.IsConfigured() {
		return nil, ErrCOSNotConfigured
	}
	return cfg, nil
}

// objectExists 用精确前缀列举探测 key 是否存在。
// store 不支持列举时返回 (false, ErrObjectStoreNoListSupport)，调用方会跳过覆盖检查。
func (s *AdminFileService) objectExists(ctx context.Context, store BackupObjectStore, key string) (bool, error) {
	lister, ok := store.(BackupObjectLister)
	if !ok {
		return false, ErrObjectStoreNoListSupport
	}
	// delimiter 传空：要的是精确匹配，不需要目录聚合。
	page, err := lister.ListObjects(ctx, key, "", "", 1)
	if err != nil {
		return false, err
	}
	for _, e := range page.Entries {
		if e.Key == key {
			return true, nil
		}
	}
	return false, nil
}

// ─── key 处理 ───

// sanitizeObjectKey 校验并归一化对象键。
//
// 拒绝以下输入（管理员也不该绕过，这些多半是手误或被诱导构造的）：
//   - 空键
//   - 含 ".." 路径段：对象存储虽然没有真实目录，但很多网关/CDN 会做路径归一，
//     ".." 可能被解析到预期之外的对象上
//   - 含控制字符或非法 UTF-8：会让签名、日志、前端渲染都出问题
//   - 超过 1024 字节（S3 规范上限）
func sanitizeObjectKey(key string) (string, error) {
	k := strings.TrimLeft(strings.TrimSpace(key), "/")
	if k == "" {
		return "", ErrInvalidObjectKey
	}
	if len(k) > adminFileMaxKeyLength {
		return "", fmt.Errorf("%w: too long", ErrInvalidObjectKey)
	}
	if !isSafeObjectKey(k) {
		return "", ErrInvalidObjectKey
	}
	return k, nil
}

// isSafeObjectKey 判断键是否不含穿越段与控制字符。
func isSafeObjectKey(k string) bool {
	if !utf8.ValidString(k) {
		return false
	}
	for _, r := range k {
		// 拒绝 C0 控制字符与 DEL。\r\n 混进 key 还可能被用于日志注入。
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	for _, seg := range strings.Split(k, "/") {
		if seg == ".." {
			return false
		}
	}
	return true
}

// normalizeKeyPrefix 归一化列举前缀：去首斜杠与空白。
// 不强制补尾部 "/" —— 前缀匹配语义下 "img" 也可能是有意的（匹配 img*）。
func normalizeKeyPrefix(prefix string) string {
	return strings.TrimLeft(strings.TrimSpace(prefix), "/")
}

// displayNameForKey 计算列表里展示的名字：去掉当前前缀，目录再去掉尾部 "/"。
// 例如 prefix="images/2026/" key="images/2026/a.png" → "a.png"。
func displayNameForKey(key, prefix string, isDir bool) string {
	name := strings.TrimPrefix(key, prefix)
	if isDir {
		name = strings.TrimSuffix(name, "/")
	}
	if name == "" {
		// key 恰好等于 prefix（罕见）：回退到最后一段，至少不显示空白。
		name = path.Base(strings.TrimSuffix(key, "/"))
	}
	return name
}
