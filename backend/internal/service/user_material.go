// Package service: 用户素材库。
//
// 素材库是"给演练台的图片/音频/视频输入控件用的用户私有文件仓库"。功能拆分：
//  1. 上传字节 / 从 URL 导入 → 转存到 COS（用户 ID 前缀目录）→ 落库记录；
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
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

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
)

// UserMaterial 是一条素材库记录（领域模型）。DB 侧字段一一对应 user_materials 表。
type UserMaterial struct {
	ID          int64
	UserID      int64
	FileName    string // 原始文件名（前端展示用；上传时若为空会由 URL 派生）
	CosKey      string // COS 桶内 key，"users/{user_id}/materials/YYYY/MM/{uuid}.{ext}"
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
	GetByID(ctx context.Context, id int64) (*UserMaterial, error)
	List(ctx context.Context, userID int64, kind, keyword string, offset, limit int) ([]*UserMaterial, int64, error)
	SoftDelete(ctx context.Context, userID, id int64) error
}

// UserMaterialService 负责编排"字节 / URL → COS → DB"三步。
type UserMaterialService struct {
	repo UserMaterialRepository
	cos  *COSImageTransferService
}

// NewUserMaterialService 构造。cos 允许为 nil：此时所有写入类操作会返回 ErrCOSNotConfigured，
// 但 List / GetByID 仍可用（只是历史数据的只读视图）。
func NewUserMaterialService(repo UserMaterialRepository, cos *COSImageTransferService) *UserMaterialService {
	return &UserMaterialService{repo: repo, cos: cos}
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

	key := buildUserMaterialKey(userID, filename, ct)
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
	// 先按视频上限拉取（能兜住图片/音频）；后面根据 kind 再复查。
	data, ct, err := s.cos.DownloadToBytes(ctx, srcURL, MaterialVideoMaxBytes)
	if err != nil {
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

	// 文件名：URL 最后一段（去 query），实在没有就用 key 的 base。
	fname := deriveFileNameFromURL(srcURL)
	key := buildUserMaterialKey(userID, fname, ct)
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

// GetByID 只用于内部（或未来 API）按 id 反查素材元信息；调用方需自行校验归属。
func (s *UserMaterialService) GetByID(ctx context.Context, id int64) (*UserMaterial, error) {
	return s.repo.GetByID(ctx, id)
}

// ------------------------------- helpers -------------------------------

// buildUserMaterialKey 生成 COS 对象 key："users/{user_id}/materials/YYYY/MM/{uuid}{ext}"。
func buildUserMaterialKey(userID int64, filename, contentType string) string {
	ext := ExtFromURLOrType(filename, contentType)
	now := time.Now().UTC()
	return fmt.Sprintf("users/%d/materials/%04d/%02d/%s%s",
		userID, now.Year(), int(now.Month()), uuid.NewString(), ext)
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
