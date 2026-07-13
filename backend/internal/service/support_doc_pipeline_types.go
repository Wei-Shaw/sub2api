// Package service — support_doc_pipeline_types.go
//
// add-support-knowledge-rag 文档抓取管线的领域类型 + repository 接口契约。
//
// 之所以把"类型"独立成文件，与 support_doc_pipeline.go 中的"实现"分开，是为了：
//   - 让 repository 包能 import 这些类型而不引入实现（避免反向依赖）；
//   - 状态结构（SupportDocIndexStatus）也是 admin handler GET /status 的响应载体，
//     单独成文件便于 dto/handler 引用而不拖入整个 pipeline 实现。
package service

import (
	"context"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// SupportDocIndexState pipeline 当前阶段。
const (
	SupportDocIndexStateIdle      = "idle"
	SupportDocIndexStateRunning   = "running"
	SupportDocIndexStateCompleted = "completed"
	SupportDocIndexStateFailed    = "failed"
)

// SupportDocIndexHardPageCap 单次 pipeline 最多抓取页数（design D3 安全阀）。
const SupportDocIndexHardPageCap = 50

// SupportDocIndexAdvisoryLockKey 是 pg_try_advisory_lock 的固定 key。
// 用 64-bit 整数；任意常量即可，避免与项目其它 advisory lock 冲突。
const SupportDocIndexAdvisoryLockKey int64 = 0x517069704e6f7773 // "QpipNows"

// SupportDocFetchUserAgent pipeline 抓取 HTTP 请求的固定 UA（design D3）。
const SupportDocFetchUserAgent = "Sub2APIDocBot/1.0"

// SupportDocFetchTimeout 单页抓取的硬超时（design D3）。
const SupportDocFetchTimeout = 30 * time.Second

// SupportDocChunkMinChars 切片下限（design D3：太短没语义就 SKIP）。
const SupportDocChunkMinChars = 50

// SupportDocEmbedBatchSize 批量 embed 的页面 chunk 数（design D3）。
const SupportDocEmbedBatchSize = 100

// SupportDocIndexError 单条页面错误。
type SupportDocIndexError struct {
	URL     string `json:"url"`
	Message string `json:"message"`
}

// SupportDocIndexStatus 是 pipeline 完整状态快照，序列化进 setting key
// `support_chat_rag_doc_index_status` 持久化（覆盖写）。
type SupportDocIndexStatus struct {
	State             string                 `json:"state"`
	StartedAt         time.Time              `json:"started_at"`
	LastRunAt         time.Time              `json:"last_run_at"`
	DurationSeconds   int                    `json:"duration_seconds"`
	PagesVisited      int                    `json:"pages_visited"`
	PagesCapHit       bool                   `json:"pages_cap_hit"`
	ChunksTotal       int                    `json:"chunks_total"`
	ChunksAdded       int                    `json:"chunks_added"`
	ChunksRemoved     int                    `json:"chunks_removed"`
	ChunksFailedEmbed int                    `json:"chunks_failed_embed"`
	Errors            []SupportDocIndexError `json:"errors"`
}

// 文档抓取管线相关错误。
var (
	// ErrSupportDocIndexAlreadyRunning：advisory lock 拿不到 → 同一时刻只能一个 pipeline。
	ErrSupportDocIndexAlreadyRunning = infraerrors.Conflict(
		"SUPPORT_DOC_INDEX_ALREADY_RUNNING",
		"support doc index pipeline is already running",
	)
	// ErrSupportDocURLEmpty：admin 没填 doc_url。
	ErrSupportDocURLEmpty = infraerrors.BadRequest(
		"SUPPORT_DOC_URL_EMPTY",
		"support doc url is empty",
	)
)

// SupportDocChunkRepository 是 doc chunks 表的最小持久化抽象。
// 实现位于 repository 包；service 层（pipeline / purge handler）使用此接口。
type SupportDocChunkRepository interface {
	// UpsertChunk 写入一条 chunk。已存在 (source_url, content_hash) 时跳过。
	// 返回 inserted=true 表示本次实际新增了一行。
	UpsertChunk(ctx context.Context, sourceURL, contentHash, chunkText string, vec []float32) (bool, error)
	// DeleteOrphans 删除某 URL 下不在 keepHashes 集合中的全部 chunks；返回删除条数。
	DeleteOrphans(ctx context.Context, sourceURL string, keepHashes []string) (int, error)
	// DeleteAll 物理删除全部 chunks（admin purge）；返回删除条数。
	DeleteAll(ctx context.Context) (int, error)
	// CountAll 全表行数（status.chunks_total 报告）。
	CountAll(ctx context.Context) (int, error)
	// DistinctSourceURLs 已抓过的 URL 列表。
	DistinctSourceURLs(ctx context.Context) ([]string, error)
}
