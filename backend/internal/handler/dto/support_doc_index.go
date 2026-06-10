// Package dto — support_doc_index.go
//
// 客服文档索引管线 admin 接口 DTO。
//
// 与 service.SupportDocIndexStatus 一一对应，但保留 dto 边界以便：
//   - 控制 JSON 字段名（snake_case）；
//   - 屏蔽 service 内部字段在未来增加时不破坏 API 契约。
package dto

import "time"

// SupportDocIndexErrorEntry 单条错误项。
type SupportDocIndexErrorEntry struct {
	URL     string `json:"url"`
	Message string `json:"message"`
}

// SupportDocIndexStatusResponse GET /admin/support/doc-index/status 响应。
type SupportDocIndexStatusResponse struct {
	State             string                      `json:"state"`
	StartedAt         time.Time                   `json:"started_at"`
	LastRunAt         time.Time                   `json:"last_run_at"`
	DurationSeconds   int                         `json:"duration_seconds"`
	PagesVisited      int                         `json:"pages_visited"`
	PagesCapHit       bool                        `json:"pages_cap_hit"`
	ChunksTotal       int                         `json:"chunks_total"`
	ChunksAdded       int                         `json:"chunks_added"`
	ChunksRemoved     int                         `json:"chunks_removed"`
	ChunksFailedEmbed int                         `json:"chunks_failed_embed"`
	Errors            []SupportDocIndexErrorEntry `json:"errors"`
}
