// Package service — support_chat_faq.go 定义客服知识库 FAQ 的领域类型 + Repository 接口。
//
// FAQ 持久化由 add-support-knowledge-rag 引入。设计要点：
//
//   - 所有非 embedding 列由 ent 管理（自动迁移 / CRUD）；
//     embedding 列（PostgreSQL `vector(1536)`）由 SQL migration 直接创建，
//     repository 通过原生 *sql.DB 读写（pgvector ent 集成度低）。
//
//   - List 路径返回 Indexed bool 字段：true ⇔ embedding IS NOT NULL；admin UI
//     用此显示"未索引"badge。Indexed = false 的行不参与 RAG 检索。
//
//   - 写入流程（service 层负责）：先 ent.Create / Update 写入非 embedding 字段，
//     再调 EmbeddingService 拿向量，再 SetEmbedding(id, vec) 写 embedding 列；
//     embedding 失败时 row 仍持久化，response 携带 warning="embedding_failed"。
package service

import (
	"context"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// SupportFaqItem 是 FAQ 领域模型（不含 embedding 二进制字段）。
type SupportFaqItem struct {
	ID        int64
	Question  string
	Answer    string
	Tags      []string
	Enabled   bool
	SortOrder int
	// Indexed 表示 embedding 列是否非 NULL；List/GetByID 路径会填充。
	Indexed   bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

// SupportFaqItemPatch 是 admin 更新 FAQ 的字段集合。nil 表示"不修改"。
type SupportFaqItemPatch struct {
	Question  *string
	Answer    *string
	Tags      *[]string
	Enabled   *bool
	SortOrder *int
}

// 字段长度约束（与 SQL VARCHAR(200)/200 字符 spec / 5000 字符 spec 对齐）。
const (
	SupportFaqQuestionMaxLen = 200
	SupportFaqAnswerMaxLen   = 5000
	SupportFaqTagMaxLen      = 30
	SupportFaqTagsMaxItems   = 20
)

// FAQ service 业务错误。
var (
	// ErrSupportFaqNotFound：FAQ id 不存在。
	ErrSupportFaqNotFound = infraerrors.NotFound(
		"SUPPORT_FAQ_NOT_FOUND",
		"support faq not found",
	)
	// ErrSupportFaqQuestionRequired：question 空或仅空白。
	ErrSupportFaqQuestionRequired = infraerrors.BadRequest(
		"SUPPORT_FAQ_QUESTION_REQUIRED",
		"support faq question is required",
	)
	// ErrSupportFaqQuestionTooLong：question 超过 200 字符。
	ErrSupportFaqQuestionTooLong = infraerrors.BadRequest(
		"SUPPORT_FAQ_QUESTION_TOO_LONG",
		"support faq question exceeds maximum length",
	)
	// ErrSupportFaqAnswerRequired：answer 空或仅空白。
	ErrSupportFaqAnswerRequired = infraerrors.BadRequest(
		"SUPPORT_FAQ_ANSWER_REQUIRED",
		"support faq answer is required",
	)
	// ErrSupportFaqAnswerTooLong：answer 超过 5000 字符。
	ErrSupportFaqAnswerTooLong = infraerrors.BadRequest(
		"SUPPORT_FAQ_ANSWER_TOO_LONG",
		"support faq answer exceeds maximum length",
	)
	// ErrSupportFaqTagsTooMany：tags 数组超过 20 项。
	ErrSupportFaqTagsTooMany = infraerrors.BadRequest(
		"SUPPORT_FAQ_TAGS_TOO_MANY",
		"support faq tags must contain at most 20 entries",
	)
	// ErrSupportFaqTagInvalid：单个 tag 空或超过 30 字符。
	ErrSupportFaqTagInvalid = infraerrors.BadRequest(
		"SUPPORT_FAQ_TAG_INVALID",
		"support faq tag must be 1..30 characters",
	)
)

// SupportFaqRepository 是 FAQ 存储抽象。Service 层依赖该接口；具体实现在 repository 包。
type SupportFaqRepository interface {
	// Create 新建 FAQ（不写 embedding 列）；回填 ID + 时间戳。
	Create(ctx context.Context, item *SupportFaqItem) error
	// GetByID 读取单条。Indexed 字段会被填充。
	GetByID(ctx context.Context, id int64) (*SupportFaqItem, error)
	// List 返回全部 FAQ，按 sort_order ASC, id ASC 排序。Indexed 字段填充。
	// onlyEnabled = true 时仅返回 enabled = true 的行（公开 FAQ 列表用）。
	List(ctx context.Context, onlyEnabled bool) ([]SupportFaqItem, error)
	// UpdatePartial 部分更新；nil 字段不修改。返回 ErrSupportFaqNotFound 当 id 不存在。
	UpdatePartial(ctx context.Context, id int64, patch SupportFaqItemPatch) error
	// Delete 物理删除。返回 ErrSupportFaqNotFound 当 id 不存在。
	Delete(ctx context.Context, id int64) error
	// CountAll 全表行数（用于 FAQ 数据迁移启动钩子的"表为空"判断）。
	CountAll(ctx context.Context) (int, error)

	// SetEmbedding 写入 embedding 列（向量长度必须等于 SupportChatRAGEmbedDimension）；
	// vec=nil 时把列置 NULL。通过原生 SQL 完成（ent 不管理 vector 列）。
	SetEmbedding(ctx context.Context, id int64, vec []float32) error
	// ClearEmbedding 等价于 SetEmbedding(id, nil)；语义更清晰的别名。
	ClearEmbedding(ctx context.Context, id int64) error
	// ListIDsWithoutEmbedding 返回所有 embedding IS NULL 且 enabled = true 的 id（用于
	// 启动数据迁移后异步补 embedding；以及 admin 触发批量 reindex）。
	ListIDsWithoutEmbedding(ctx context.Context) ([]int64, error)
}
