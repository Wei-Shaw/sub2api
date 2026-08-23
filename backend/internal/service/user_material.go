// Package service: 用户素材库。
//
// 素材库是"给演练台的图片/音频/视频输入控件用的用户私有文件仓库"。功能拆分：
//  1. 上传字节 / 从 URL 导入 → 转存到 COS（密钥派生的用户目录）→ 落库记录；
//  2. 按 user_id 分页读取；
//  3. 软删：DB 立即标记 deleted_at；COS 对象保留一段时间后由后台清理任务真删。
//     本文件只做前两步；后台清理任务先不实现（避免摊得太大）。
//
// 依赖：
//   - UserMaterialRepository（SQL 层）
//   - COSImageTransferService（转存 / 下载）
package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/base32"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

// 各素材类型的上传体积上限（服务侧兜底；前端也会同步校验）。
// 参考：图片 32 MiB / 音频 64 MiB / 视频 512 MiB —— 与 COSImageTransferService 内部
// 图片/视频下载限值对齐；音频按视频的 1/8 给个中间值，绝大多数场景足够。
const (
	MaterialImageMaxBytes int64 = 32 << 20
	MaterialAudioMaxBytes int64 = 64 << 20
	MaterialVideoMaxBytes int64 = 512 << 20
	// MaterialBatchDeleteMaxIDs limits one batch delete request.
	MaterialBatchDeleteMaxIDs = 100
)

// ErrMaterialPathIdentityUnavailable 表示无法为素材生成不可枚举的用户目录。
// 不回退到明文 user_id，避免身份信息泄漏。
var ErrMaterialPathIdentityUnavailable = infraerrors.InternalServer(
	"MATERIAL_PATH_IDENTITY_UNAVAILABLE",
	"material path identity is unavailable",
)

// 每用户配额。单文件上限之外还必须有"总量"上限，否则任何登录用户都能通过
// 反复上传把对象存储刷满（COS 按存储量+流量计费，等于直接烧钱）。
//
// 取值考虑：素材库是"演练台的图片/音频/视频输入仓库"，正常用法是攒几十到几百
// 张参考图。给 500 条 / 5 GiB 对真实使用足够宽松，又能把恶意刷量挡在可接受
// 的成本内。两个维度都要卡：只卡容量挡不住"海量小文件"（DB 行数与 list 性能），
// 只卡条数挡不住"512MB 视频 × 500"。
const (
	// MaterialMaxCountPerUser 单用户最多保留多少条未删除素材。
	MaterialMaxCountPerUser int64 = 500
	// MaterialMaxTotalBytesPerUser 单用户未删除素材的总字节上限。
	MaterialMaxTotalBytesPerUser int64 = 5 << 30 // 5 GiB
	// MaterialFileNameMaxRunes matches user_materials.file_name VARCHAR(512).
	MaterialFileNameMaxRunes = 512
)

var (
	ErrMaterialInvalidFileName = infraerrors.BadRequest("INVALID_FILE_NAME", "material file name is invalid")
	ErrMaterialNotFound        = infraerrors.NotFound("MATERIAL_NOT_FOUND", "material not found")
)

// UserMaterial 是一条素材库记录（领域模型）。DB 侧字段一一对应 user_materials 表。
type UserMaterial struct {
	ID          int64
	PublicID    string // opaque UUID exposed to RPC callers; never the database id
	UserID      int64
	FileName    string // 原始文件名（前端展示用；上传时若为空会由 URL 派生）
	CosKey      string // COS 桶内 key，用户目录由 user_id + account_id 的密钥派生值生成
	CosURL      string // 对外可访问 URL（写回业务侧、传给上游模型用的就是它）
	ContentType string
	SizeBytes   int64
	Kind        string // image | audio | video | other
	Source      string // upload | url_import
	CreatedAt   time.Time
}

// UserMaterialRepository 定义 SQL 层依赖，实现见 repository/user_material_repo.go。
type UserMaterialRepository interface {
	Insert(ctx context.Context, m *UserMaterial) (int64, error)
	GetByID(ctx context.Context, userID, id int64) (*UserMaterial, error)
	GetByPublicID(ctx context.Context, userID int64, publicID string) (*UserMaterial, error)
	UpdateFileNameByID(ctx context.Context, userID, id int64, fileName string) (*UserMaterial, error)
	UpdateFileNameByPublicID(ctx context.Context, userID int64, publicID, fileName string) (*UserMaterial, error)
	List(ctx context.Context, userID int64, kind, keyword string, offset, limit int) ([]*UserMaterial, int64, error)
	SoftDelete(ctx context.Context, userID, id int64) error
	SoftDeleteByPublicID(ctx context.Context, userID int64, publicID string) error
	SoftDeleteByPublicIDs(ctx context.Context, userID int64, publicIDs []string) ([]string, error)
	// UsageByUser 返回该用户未删除素材的条数与总字节数，用于配额校验。
	UsageByUser(ctx context.Context, userID int64) (count int64, totalBytes int64, err error)
}

// UserMaterialService 负责编排"字节 / URL → COS → DB"三步。
type UserMaterialService struct {
	repo     UserMaterialRepository
	cos      *COSImageTransferService
	userRepo UserRepository
	pathKey  []byte
}

// NewUserMaterialService 构造。写入素材时会用 userRepo 读取 account_id，
// 再结合 cfg.JWT.Secret 派生不可枚举的用户目录；缺少任一身份材料时拒绝写入。
// cos 允许为 nil：此时所有写入类操作会返回 ErrCOSNotConfigured，
// 但 List / GetByID 仍可用（只是历史数据的只读视图）。
func NewUserMaterialService(repo UserMaterialRepository, cos *COSImageTransferService, userRepo UserRepository, cfg *config.Config) *UserMaterialService {
	var pathKey []byte
	if cfg != nil && strings.TrimSpace(cfg.JWT.Secret) != "" {
		domainKey := sha256.Sum256([]byte("sub2api/material-path/v1\x00" + cfg.JWT.Secret))
		pathKey = domainKey[:]
	}
	return &UserMaterialService{repo: repo, cos: cos, userRepo: userRepo, pathKey: pathKey}
}

// UploadBytes 把上传上来的字节转存到 COS 并落库。
//
// 校验规则：
//   - userID>0；否则 400
//   - data 非空；否则 400
//   - Content-Type 自动嗅探；据此推断 Kind 与体积上限
//   - 超限直接拒绝，绝不写 COS（防止刷桶）
func (s *UserMaterialService) UploadBytes(ctx context.Context, userID int64, filename, contentType string, data []byte) (*UserMaterial, error) {
	if userID <= 0 {
		return nil, infraerrors.BadRequest("INVALID_USER", "invalid user id")
	}
	if len(data) == 0 {
		return nil, infraerrors.BadRequest("EMPTY_FILE", "uploaded file is empty")
	}
	if s.cos == nil {
		return nil, ErrCOSNotConfigured
	}
	// 嗅探 Content-Type（前端有时不给或给错）。
	ct := strings.TrimSpace(contentType)
	if ct == "" {
		ct = http.DetectContentType(data)
	}
	// 剥参数
	if i := strings.IndexByte(ct, ';'); i >= 0 {
		ct = strings.TrimSpace(ct[:i])
	}
	kind := KindFromContentType(ct)
	// 只允许 image / audio / video；other 一律拒绝，避免变成"通用网盘"。
	if kind != "image" && kind != "audio" && kind != "video" {
		return nil, infraerrors.BadRequest("UNSUPPORTED_CONTENT_TYPE",
			fmt.Sprintf("unsupported content type: %s (only image/audio/video allowed)", ct))
	}
	if err := checkMaterialSize(kind, int64(len(data))); err != nil {
		return nil, err
	}
	// 配额校验放在写 COS 之前：超限时一个字节都不落桶。
	if err := s.checkQuota(ctx, userID, int64(len(data))); err != nil {
		return nil, err
	}

	userPrefix, err := s.materialUserPrefix(ctx, userID)
	if err != nil {
		return nil, err
	}
	key := buildUserMaterialKey(userPrefix, filename, ct)
	cosURL, err := s.cos.UploadBytesWithKey(ctx, key, data, ct)
	if err != nil {
		return nil, fmt.Errorf("upload to cos: %w", err)
	}

	fname := strings.TrimSpace(filename)
	if fname == "" {
		fname = keyBaseName(key)
	}
	m := &UserMaterial{
		UserID:      userID,
		FileName:    truncateRunes(fname, 512),
		CosKey:      key,
		CosURL:      cosURL,
		ContentType: truncateRunes(ct, 128),
		SizeBytes:   int64(len(data)),
		Kind:        kind,
		Source:      "upload",
	}
	if _, err := s.repo.Insert(ctx, m); err != nil {
		// COS 已上传成功但 DB 写入失败：把对象删掉避免孤儿。删除失败仅告警，不阻断错误上抛。
		if delErr := s.cos.DeleteObject(ctx, key); delErr != nil {
			logger.LegacyPrintf("service.user_material",
				"[MATERIAL] rollback cos delete failed key=%s err=%v", key, delErr)
		}
		return nil, err
	}
	return m, nil
}

// ImportFromURL 从外部 URL 下载后转存到 COS 并落库（source='url_import'）。
// 与 UploadBytes 语义一致，只是数据源不同。
//
// 安全性：srcURL 完全由终端用户控制，是典型的 SSRF 入口，因此必须走
// cos.DownloadUntrustedToBytes（scheme 白名单 + host 预检 + dial 层 IP 校验），
// 绝不能用普通的下载路径。
func (s *UserMaterialService) ImportFromURL(ctx context.Context, userID int64, srcURL string) (*UserMaterial, error) {
	if userID <= 0 {
		return nil, infraerrors.BadRequest("INVALID_USER", "invalid user id")
	}
	srcURL = strings.TrimSpace(srcURL)
	if srcURL == "" || (!strings.HasPrefix(srcURL, "http://") && !strings.HasPrefix(srcURL, "https://")) {
		return nil, infraerrors.BadRequest("INVALID_URL", "url must start with http(s)://")
	}
	if s.cos == nil {
		return nil, ErrCOSNotConfigured
	}
	// 先做一次配额预检（用 0 增量，只看条数与当前总量是否已达上限），
	// 避免"明明已经超额还是先把几百 MB 拉下来"的浪费。
	if err := s.checkQuota(ctx, userID, 0); err != nil {
		return nil, err
	}
	// 先按视频上限拉取（能兜住图片/音频）；后面根据 kind 再复查。
	data, ct, err := s.cos.DownloadUntrustedToBytes(ctx, srcURL, MaterialVideoMaxBytes)
	if err != nil {
		// 指向内网/回环/云元数据：明确回一个专用错误码，且不泄漏解析细节。
		if errors.Is(err, ErrUntrustedURLBlocked) {
			return nil, infraerrors.BadRequest("URL_BLOCKED",
				"url points to a disallowed address (internal or loopback)")
		}
		return nil, infraerrors.BadRequest("URL_FETCH_FAILED", "fetch url failed: "+err.Error())
	}
	if len(data) == 0 {
		return nil, infraerrors.BadRequest("EMPTY_REMOTE_FILE", "remote file is empty")
	}
	if i := strings.IndexByte(ct, ';'); i >= 0 {
		ct = strings.TrimSpace(ct[:i])
	}
	if ct == "" {
		ct = http.DetectContentType(data)
	}
	kind := KindFromContentType(ct)
	if kind != "image" && kind != "audio" && kind != "video" {
		return nil, infraerrors.BadRequest("UNSUPPORTED_CONTENT_TYPE",
			fmt.Sprintf("unsupported content type: %s", ct))
	}
	if err := checkMaterialSize(kind, int64(len(data))); err != nil {
		return nil, err
	}
	// 拿到真实体积后再按增量复查一次配额。
	if err := s.checkQuota(ctx, userID, int64(len(data))); err != nil {
		return nil, err
	}

	// 文件名：URL 最后一段（去 query），实在没有就用 key 的 base。
	fname := deriveFileNameFromURL(srcURL)
	userPrefix, err := s.materialUserPrefix(ctx, userID)
	if err != nil {
		return nil, err
	}
	key := buildUserMaterialKey(userPrefix, fname, ct)
	cosURL, err := s.cos.UploadBytesWithKey(ctx, key, data, ct)
	if err != nil {
		return nil, fmt.Errorf("upload to cos: %w", err)
	}
	m := &UserMaterial{
		UserID:      userID,
		FileName:    truncateRunes(fname, 512),
		CosKey:      key,
		CosURL:      cosURL,
		ContentType: truncateRunes(ct, 128),
		SizeBytes:   int64(len(data)),
		Kind:        kind,
		Source:      "url_import",
	}
	if _, err := s.repo.Insert(ctx, m); err != nil {
		if delErr := s.cos.DeleteObject(ctx, key); delErr != nil {
			logger.LegacyPrintf("service.user_material",
				"[MATERIAL] rollback cos delete failed key=%s err=%v", key, delErr)
		}
		return nil, err
	}
	return m, nil
}

// AddMaterialByURL registers an already-public COS URL without downloading or
// copying it. The caller is restricted to the configured public COS origin.
func (s *UserMaterialService) AddMaterialByURL(ctx context.Context, userID int64, rawURL string) (*UserMaterial, error) {
	if userID <= 0 {
		return nil, infraerrors.BadRequest("INVALID_USER", "invalid user id")
	}
	if s == nil || s.cos == nil {
		return nil, ErrCOSNotConfigured
	}
	cfg, err := s.cos.GetConfig(ctx)
	if err != nil {
		return nil, err
	}
	if cfg == nil || !cfg.Enabled || !cfg.IsConfigured() || strings.TrimSpace(cfg.PublicBaseURL) == "" {
		return nil, ErrCOSNotConfigured
	}
	publicURL, err := validatePublicCOSURL(rawURL, cfg.PublicBaseURL)
	if err != nil {
		return nil, err
	}
	if err := s.checkQuota(ctx, userID, 0); err != nil {
		return nil, err
	}
	parsed, _ := url.Parse(publicURL)
	filename := deriveFileNameFromURL(publicURL)
	contentType := mime.TypeByExtension(path.Ext(parsed.Path))
	if contentType == "" || !strings.HasPrefix(contentType, "image/") {
		contentType = "image/*"
	}
	m := &UserMaterial{
		UserID:      userID,
		FileName:    truncateRunes(filename, 512),
		CosURL:      publicURL,
		ContentType: contentType,
		Kind:        "image",
		Source:      "url",
	}

	// Developer file uploads are temporary. Only URLs inside the configured
	// file_uploads root are candidates for a move; ordinary same-origin COS URLs
	// retain the historical register-without-copy behavior.
	sourceKey, keyErr := s.cos.PublicURLToKey(ctx, publicURL)
	fileRoot := developerFileRoot(cfg) + "/"
	moved := false
	if keyErr == nil && strings.HasPrefix(sourceKey, fileRoot) {
		userPrefix, prefixErr := s.materialUserPrefix(ctx, userID)
		if prefixErr != nil {
			return nil, prefixErr
		}
		ownedPrefix, prefixErr := developerFileUserPrefix(cfg, userPrefix)
		if prefixErr != nil {
			return nil, prefixErr
		}
		if !strings.HasPrefix(sourceKey, ownedPrefix+"/") {
			return nil, ErrDeveloperFileForbidden
		}
		destinationKey := buildUserMaterialKey(userPrefix, filename, contentType)
		destinationURL, moveErr := s.cos.MoveFile(ctx, sourceKey, destinationKey)
		if moveErr != nil {
			return nil, fmt.Errorf("move temporary upload to material storage: %w", moveErr)
		}
		m.CosKey = destinationKey
		m.CosURL = destinationURL
		moved = true
	}
	if _, err := s.repo.Insert(ctx, m); err != nil {
		if moved {
			if _, rollbackErr := s.cos.MoveFile(ctx, m.CosKey, sourceKey); rollbackErr != nil {
				logger.LegacyPrintf("service.user_material",
					"[MATERIAL] rollback temporary file move failed src=%s dst=%s err=%v",
					m.CosKey, sourceKey, rollbackErr)
			}
		}
		return nil, err
	}
	return m, nil
}

// List 分页读取当前用户的素材。kind 为空时不过滤类型；keyword 走文件名前缀匹配。
func (s *UserMaterialService) List(ctx context.Context, userID int64, kind, keyword string, page, pageSize int) ([]*UserMaterial, int64, error) {
	if userID <= 0 {
		return nil, 0, infraerrors.BadRequest("INVALID_USER", "invalid user id")
	}
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	offset := (page - 1) * pageSize
	return s.repo.List(ctx, userID, kind, keyword, offset, pageSize)
}

// Delete 软删单条（仅 DB 标记；COS 对象保留，由后台清理任务在保留期后真删）。
// 只能删自己的；不是本人的素材返回 not found。
func (s *UserMaterialService) Delete(ctx context.Context, userID, id int64) error {
	if userID <= 0 {
		return infraerrors.BadRequest("INVALID_USER", "invalid user id")
	}
	if id <= 0 {
		return infraerrors.BadRequest("INVALID_ID", "invalid id")
	}
	return s.repo.SoftDelete(ctx, userID, id)
}

// GetByID 按 id + userID 读取素材元信息。
//
// 必须带 userID：素材是用户私有资源，只按 id 查会让任何登录用户通过遍历 id
// 读到别人的素材（文件名 + COS 公网地址）。归属不匹配时返回 sql.ErrNoRows
// 语义的 not found，而不是 403 —— 不向调用方泄漏"这个 id 存在但不属于你"。
func (s *UserMaterialService) GetByID(ctx context.Context, userID, id int64) (*UserMaterial, error) {
	if userID <= 0 {
		return nil, infraerrors.BadRequest("INVALID_USER", "invalid user id")
	}
	m, err := s.repo.GetByID(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	if m == nil {
		return nil, sql.ErrNoRows
	}
	return m, nil
}

// GetByPublicID resolves the opaque UUID exposed by the internal RPC.
func (s *UserMaterialService) GetByPublicID(ctx context.Context, userID int64, publicID string) (*UserMaterial, error) {
	if userID <= 0 {
		return nil, infraerrors.BadRequest("INVALID_USER", "invalid user id")
	}
	if !isMaterialPublicID(publicID) {
		return nil, infraerrors.BadRequest("INVALID_ID", "invalid material id")
	}
	m, err := s.repo.GetByPublicID(ctx, userID, strings.TrimSpace(publicID))
	if err != nil {
		return nil, err
	}
	if m == nil {
		return nil, sql.ErrNoRows
	}
	return m, nil
}

// Rename updates only the display name using the numeric ID exposed by the
// authenticated user HTTP API. The COS object key and URL remain unchanged.
func (s *UserMaterialService) Rename(ctx context.Context, userID, id int64, fileName string) (*UserMaterial, error) {
	if userID <= 0 {
		return nil, infraerrors.BadRequest("INVALID_USER", "invalid user id")
	}
	if id <= 0 {
		return nil, infraerrors.BadRequest("INVALID_ID", "invalid material id")
	}
	fileName, err := normalizeMaterialFileName(fileName)
	if err != nil {
		return nil, err
	}
	material, err := s.repo.UpdateFileNameByID(ctx, userID, id, fileName)
	if err != nil {
		return nil, err
	}
	if material == nil {
		return nil, ErrMaterialNotFound
	}
	return material, nil
}

// RenameByPublicID updates only the display name. The COS object key and URL
// remain unchanged, and ownership is enforced atomically by the repository.
func (s *UserMaterialService) RenameByPublicID(ctx context.Context, userID int64, publicID, fileName string) (*UserMaterial, error) {
	if userID <= 0 {
		return nil, infraerrors.BadRequest("INVALID_USER", "invalid user id")
	}
	if !isMaterialPublicID(publicID) {
		return nil, infraerrors.BadRequest("INVALID_ID", "invalid material id")
	}
	fileName, err := normalizeMaterialFileName(fileName)
	if err != nil {
		return nil, err
	}
	parsedID, _ := uuid.Parse(strings.TrimSpace(publicID))
	material, err := s.repo.UpdateFileNameByPublicID(ctx, userID, parsedID.String(), fileName)
	if err != nil {
		return nil, err
	}
	if material == nil {
		return nil, ErrMaterialNotFound
	}
	return material, nil
}

func normalizeMaterialFileName(fileName string) (string, error) {
	fileName = strings.TrimSpace(fileName)
	if fileName == "" || !utf8.ValidString(fileName) || utf8.RuneCountInString(fileName) > MaterialFileNameMaxRunes {
		return "", ErrMaterialInvalidFileName
	}
	return fileName, nil
}

// DeleteByPublicID soft-deletes a material using its opaque UUID.
func (s *UserMaterialService) DeleteByPublicID(ctx context.Context, userID int64, publicID string) error {
	if userID <= 0 {
		return infraerrors.BadRequest("INVALID_USER", "invalid user id")
	}
	if !isMaterialPublicID(publicID) {
		return infraerrors.BadRequest("INVALID_ID", "invalid material id")
	}
	return s.repo.SoftDeleteByPublicID(ctx, userID, strings.TrimSpace(publicID))
}

// BatchDeleteByPublicIDs validates the complete request before soft-deleting
// the caller's materials. Missing, already deleted, and foreign IDs are omitted.
func (s *UserMaterialService) BatchDeleteByPublicIDs(ctx context.Context, userID int64, publicIDs []string) ([]string, error) {
	if userID <= 0 {
		return nil, infraerrors.BadRequest("INVALID_USER", "invalid user id")
	}
	if len(publicIDs) == 0 {
		return nil, infraerrors.BadRequest("INVALID_IDS", "ids must contain at least one material id")
	}
	if len(publicIDs) > MaterialBatchDeleteMaxIDs {
		return nil, infraerrors.BadRequest("TOO_MANY_IDS",
			fmt.Sprintf("ids must contain at most %d material ids", MaterialBatchDeleteMaxIDs))
	}

	normalized := make([]string, 0, len(publicIDs))
	seen := make(map[string]struct{}, len(publicIDs))
	for i, publicID := range publicIDs {
		parsed, err := uuid.Parse(strings.TrimSpace(publicID))
		if err != nil {
			return nil, infraerrors.BadRequest("INVALID_ID",
				fmt.Sprintf("ids[%d] is not a valid material id", i))
		}
		canonical := parsed.String()
		if _, ok := seen[canonical]; ok {
			continue
		}
		seen[canonical] = struct{}{}
		normalized = append(normalized, canonical)
	}

	deleted, err := s.repo.SoftDeleteByPublicIDs(ctx, userID, normalized)
	if err != nil {
		return nil, err
	}
	deletedSet := make(map[string]struct{}, len(deleted))
	for _, publicID := range deleted {
		parsed, err := uuid.Parse(publicID)
		if err == nil {
			deletedSet[parsed.String()] = struct{}{}
		}
	}
	ordered := make([]string, 0, len(deletedSet))
	for _, publicID := range normalized {
		if _, ok := deletedSet[publicID]; ok {
			ordered = append(ordered, publicID)
		}
	}
	return ordered, nil
}

// checkQuota 校验"再写入 incomingBytes 字节"是否会突破该用户的素材库配额。
//
// incomingBytes 传 0 时只校验条数与当前总量是否已达上限（用于下载前的预检，
// 避免先白拉几百 MB 再发现超额）。
//
// 统计失败时选择放行而不是拦截：配额是成本保护，不是安全边界，
// 不该因为一次 DB 抖动就让正常用户传不了文件（单文件上限那一层仍然生效）。
func (s *UserMaterialService) checkQuota(ctx context.Context, userID, incomingBytes int64) error {
	count, totalBytes, err := s.repo.UsageByUser(ctx, userID)
	if err != nil {
		logger.LegacyPrintf("service.user_material",
			"[MATERIAL] quota check skipped (usage query failed) user=%d err=%v", userID, err)
		return nil
	}
	if count >= MaterialMaxCountPerUser {
		return infraerrors.BadRequest("MATERIAL_COUNT_QUOTA_EXCEEDED",
			fmt.Sprintf("material count quota exceeded: %d/%d, please delete some first",
				count, MaterialMaxCountPerUser))
	}
	if totalBytes+incomingBytes > MaterialMaxTotalBytesPerUser {
		return infraerrors.BadRequest("MATERIAL_SIZE_QUOTA_EXCEEDED",
			fmt.Sprintf("material storage quota exceeded: %d/%d bytes used, please delete some first",
				totalBytes, MaterialMaxTotalBytesPerUser))
	}
	return nil
}

func isMaterialPublicID(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) != 36 {
		return false
	}
	_, err := uuid.Parse(value)
	return err == nil
}

func validatePublicCOSURL(rawURL, rawBaseURL string) (string, error) {
	value := strings.TrimSpace(rawURL)
	baseValue := strings.TrimRight(strings.TrimSpace(rawBaseURL), "/")
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
		return "", infraerrors.BadRequest("INVALID_URL", "url must be a public COS HTTP(S) URL")
	}
	base, err := url.Parse(baseValue)
	if err != nil || base.Scheme == "" || base.Host == "" || base.User != nil || base.Fragment != "" {
		return "", ErrCOSNotConfigured
	}
	if !isHTTPScheme(parsed.Scheme) || !isHTTPScheme(base.Scheme) {
		return "", ErrCOSNotConfigured
	}
	if !strings.EqualFold(parsed.Scheme, base.Scheme) || !strings.EqualFold(parsed.Hostname(), base.Hostname()) || effectiveURLPort(parsed) != effectiveURLPort(base) {
		return "", infraerrors.BadRequest("INVALID_URL", "url must use the configured public COS domain")
	}
	if parsed.RawQuery != "" {
		return "", infraerrors.BadRequest("INVALID_URL", "url query parameters are not allowed")
	}
	return value, nil
}

func isHTTPScheme(scheme string) bool {
	return strings.EqualFold(scheme, "http") || strings.EqualFold(scheme, "https")
}

func effectiveURLPort(value *url.URL) string {
	if port := value.Port(); port != "" {
		return port
	}
	if strings.EqualFold(value.Scheme, "https") {
		return "443"
	}
	return "80"
}

// ------------------------------- helpers -------------------------------

// materialUserPrefix 生成不可枚举的用户目录。account_id 参与计算，避免仅凭连续数据库 ID 猜测目录。
func (s *UserMaterialService) materialUserPrefix(ctx context.Context, userID int64) (string, error) {
	if s == nil || len(s.pathKey) == 0 || s.userRepo == nil {
		return "", ErrMaterialPathIdentityUnavailable
	}
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil || user == nil || strings.TrimSpace(user.AccountID) == "" {
		return "", ErrMaterialPathIdentityUnavailable
	}
	h := hmac.New(sha256.New, s.pathKey)
	_, _ = fmt.Fprintf(h, "%d\x00%s", userID, strings.TrimSpace(user.AccountID))
	encoded := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(h.Sum(nil))
	return "u_" + strings.ToLower(encoded[:32]), nil
}

// UserStoragePrefix returns the opaque directory shared by temporary uploads
// and durable user materials.
func (s *UserMaterialService) UserStoragePrefix(ctx context.Context, userID int64) (string, error) {
	return s.materialUserPrefix(ctx, userID)
}

// buildUserMaterialKey 生成 COS 对象 key。用户目录是密钥派生值，不包含原始 user_id/account_id。
func buildUserMaterialKey(userPrefix, filename, contentType string) string {
	ext := ExtFromURLOrType(filename, contentType)
	now := time.Now().UTC()
	return fmt.Sprintf("users/%s/materials/%04d/%02d/%s%s",
		userPrefix, now.Year(), int(now.Month()), uuid.NewString(), ext)
}

// checkMaterialSize 按 kind 校验体积上限。
func checkMaterialSize(kind string, size int64) error {
	var limit int64
	switch kind {
	case "image":
		limit = MaterialImageMaxBytes
	case "audio":
		limit = MaterialAudioMaxBytes
	case "video":
		limit = MaterialVideoMaxBytes
	default:
		return infraerrors.BadRequest("UNSUPPORTED_KIND", "unsupported material kind: "+kind)
	}
	if size > limit {
		return infraerrors.BadRequest("FILE_TOO_LARGE",
			fmt.Sprintf("file size %d exceeds %s limit %d", size, kind, limit))
	}
	return nil
}

// deriveFileNameFromURL 从 URL 派生一个文件名（去 query、取路径最后一段）。
func deriveFileNameFromURL(rawURL string) string {
	s := rawURL
	if i := strings.IndexAny(s, "?#"); i >= 0 {
		s = s[:i]
	}
	if idx := strings.LastIndex(s, "/"); idx >= 0 && idx < len(s)-1 {
		return s[idx+1:]
	}
	return ""
}

// keyBaseName 从 COS key 里取 base 段（不含目录）。
func keyBaseName(key string) string {
	if idx := strings.LastIndex(key, "/"); idx >= 0 && idx < len(key)-1 {
		return key[idx+1:]
	}
	return key
}

// truncateRunes 简单按 rune 长度截断，避免超过 DB 列长度限制。
func truncateRunes(s string, max int) string {
	if max <= 0 {
		return ""
	}
	rs := []rune(s)
	if len(rs) <= max {
		return s
	}
	return string(rs[:max])
}
