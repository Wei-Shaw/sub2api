// Package dto — support_faq.go
//
// 客服知识库 FAQ admin 接口的请求 / 响应 DTO。
//
// 与 support_chat_faqs JSON setting 时代的 dto.SupportChatFAQ 不同，这里把 FAQ 视作
// 数据库行：携带 ID / Indexed / 时间戳。Update / Create 用 *T 指针支持部分更新。
package dto

import (
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// SupportFaqItem 是 FAQ 行的对外响应 DTO。
type SupportFaqItem struct {
	ID        int64     `json:"id"`
	Question  string    `json:"question"`
	Answer    string    `json:"answer"`
	Tags      []string  `json:"tags"`
	Enabled   bool      `json:"enabled"`
	SortOrder int       `json:"sort_order"`
	Indexed   bool      `json:"indexed"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// SupportFaqItemFromService 把 service 域模型翻译成 DTO。
func SupportFaqItemFromService(in *service.SupportFaqItem) *SupportFaqItem {
	if in == nil {
		return nil
	}
	tags := append([]string(nil), in.Tags...)
	if tags == nil {
		tags = []string{}
	}
	return &SupportFaqItem{
		ID:        in.ID,
		Question:  in.Question,
		Answer:    in.Answer,
		Tags:      tags,
		Enabled:   in.Enabled,
		SortOrder: in.SortOrder,
		Indexed:   in.Indexed,
		CreatedAt: in.CreatedAt,
		UpdatedAt: in.UpdatedAt,
	}
}

// SupportFaqMutationResponse 是 Create / Update 的响应：item + 可选 warning。
type SupportFaqMutationResponse struct {
	Item             *SupportFaqItem `json:"item"`
	EmbeddingWarning string          `json:"embedding_warning,omitempty"`
}

// SupportFaqMutationFromService 翻译 service 结果。
func SupportFaqMutationFromService(res *service.SupportFaqMutationResult) *SupportFaqMutationResponse {
	if res == nil {
		return nil
	}
	return &SupportFaqMutationResponse{
		Item:             SupportFaqItemFromService(&res.Item),
		EmbeddingWarning: res.EmbeddingWarning,
	}
}

// SupportFaqCreateRequest 是 POST /api/v1/admin/support/faqs 请求体。
type SupportFaqCreateRequest struct {
	Question  string   `json:"question" binding:"required"`
	Answer    string   `json:"answer" binding:"required"`
	Tags      []string `json:"tags"`
	Enabled   *bool    `json:"enabled"`
	SortOrder *int     `json:"sort_order"`
}

// ToService 翻译为 service 域模型；nil 字段套用合理默认。
func (r SupportFaqCreateRequest) ToService() service.SupportFaqItem {
	out := service.SupportFaqItem{
		Question: r.Question,
		Answer:   r.Answer,
		Tags:     r.Tags,
		Enabled:  true, // 默认启用
	}
	if r.Enabled != nil {
		out.Enabled = *r.Enabled
	}
	if r.SortOrder != nil {
		out.SortOrder = *r.SortOrder
	}
	return out
}

// SupportFaqUpdateRequest 是 PUT /api/v1/admin/support/faqs/:id 请求体。
//
// 全部字段都是 *T —— nil 表示"不修改"。前端必须显式提交才覆盖。
type SupportFaqUpdateRequest struct {
	Question  *string   `json:"question"`
	Answer    *string   `json:"answer"`
	Tags      *[]string `json:"tags"`
	Enabled   *bool     `json:"enabled"`
	SortOrder *int      `json:"sort_order"`
}

// ToService 翻译为 patch。
func (r SupportFaqUpdateRequest) ToService() service.SupportFaqItemPatch {
	return service.SupportFaqItemPatch{
		Question:  r.Question,
		Answer:    r.Answer,
		Tags:      r.Tags,
		Enabled:   r.Enabled,
		SortOrder: r.SortOrder,
	}
}

// SupportFaqReindexResponse 是 POST /api/v1/admin/support/faqs/reindex 的响应。
type SupportFaqReindexResponse struct {
	Succeeded int `json:"succeeded"`
	Failed    int `json:"failed"`
}
