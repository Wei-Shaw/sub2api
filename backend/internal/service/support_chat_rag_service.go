// Package service — support_chat_rag_service.go 提供客服知识库 RAG 运行时配置投影。
//
// 与 support_chat_service.go 相同的设计哲学：把分散在 SettingService 多个 key 上的
// support_chat_rag_* 设置聚合成一个 SupportChatRAGRuntime 快照，让 chat handler /
// pipeline / FAQ service 一次性拿到所有需要的字段，避免反复查 settings。
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// SupportChatRAGRuntime 是 RAG 运行时配置投影。只读快照。
type SupportChatRAGRuntime struct {
	Enabled      bool
	DocURL       string
	DocDepth     int
	DocCron      string
	EmbedModel   string
	TopK         int
	ChunkSize    int
	ChunkOverlap int
}

// GetSupportChatRAGRuntime 一次读取 8 个 RAG 设置，损坏 / 缺失值用 Clamp/Parse 兜底。
// settings store 不可达时返回全部默认 + Enabled=false（caller 视作"RAG 关"自然短路）。
func (s *SettingService) GetSupportChatRAGRuntime(ctx context.Context) SupportChatRAGRuntime {
	keys := []string{
		SettingKeySupportChatRAGEnabled,
		SettingKeySupportChatRAGDocURL,
		SettingKeySupportChatRAGDocDepth,
		SettingKeySupportChatRAGDocCron,
		SettingKeySupportChatRAGEmbedModel,
		SettingKeySupportChatRAGTopK,
		SettingKeySupportChatRAGChunkSize,
		SettingKeySupportChatRAGChunkOverlap,
	}
	vals, err := s.settingRepo.GetMultiple(ctx, keys)
	if err != nil {
		return SupportChatRAGRuntime{
			Enabled:      false,
			DocURL:       "",
			DocDepth:     SupportChatRAGDocDepthDefault,
			DocCron:      SupportChatRAGDocCronDaily03,
			EmbedModel:   SupportChatRAGEmbedModelDefault,
			TopK:         SupportChatRAGTopKDefault,
			ChunkSize:    SupportChatRAGChunkSizeDefault,
			ChunkOverlap: SupportChatRAGChunkOverlapDefault,
		}
	}

	rt := SupportChatRAGRuntime{
		Enabled:    vals[SettingKeySupportChatRAGEnabled] == "true",
		DocURL:     ParseSupportChatRAGDocURL(vals[SettingKeySupportChatRAGDocURL]),
		DocCron:    ParseSupportChatRAGDocCron(vals[SettingKeySupportChatRAGDocCron]),
		EmbedModel: ParseSupportChatRAGEmbedModel(vals[SettingKeySupportChatRAGEmbedModel]),
	}
	if v, err := strconv.Atoi(strings.TrimSpace(vals[SettingKeySupportChatRAGDocDepth])); err == nil {
		rt.DocDepth = ClampSupportChatRAGDocDepth(v)
	} else {
		rt.DocDepth = SupportChatRAGDocDepthDefault
	}
	if v, err := strconv.Atoi(strings.TrimSpace(vals[SettingKeySupportChatRAGTopK])); err == nil {
		rt.TopK = ClampSupportChatRAGTopK(v)
	} else {
		rt.TopK = SupportChatRAGTopKDefault
	}
	if v, err := strconv.Atoi(strings.TrimSpace(vals[SettingKeySupportChatRAGChunkSize])); err == nil {
		rt.ChunkSize = ClampSupportChatRAGChunkSize(v)
	} else {
		rt.ChunkSize = SupportChatRAGChunkSizeDefault
	}
	if v, err := strconv.Atoi(strings.TrimSpace(vals[SettingKeySupportChatRAGChunkOverlap])); err == nil {
		rt.ChunkOverlap = ClampSupportChatRAGChunkOverlap(v)
	} else {
		rt.ChunkOverlap = SupportChatRAGChunkOverlapDefault
	}
	return rt
}

// GetSupportChatRAGEmbedModel 单字段快捷读取（embedding service 调用时仅需这一个）。
func (s *SettingService) GetSupportChatRAGEmbedModel(ctx context.Context) string {
	v, err := s.settingRepo.Get(ctx, SettingKeySupportChatRAGEmbedModel)
	if err != nil || v == nil {
		return SupportChatRAGEmbedModelDefault
	}
	return ParseSupportChatRAGEmbedModel(v.Value)
}

// SetSupportDocIndexStatus 把 doc 抓取管线的最终状态序列化进 setting key
// `support_chat_rag_doc_index_status`（覆盖写）。pipeline 自身调用。
//
// 失败时返回错误，但不冒泡到 pipeline 主流程；admin 看不到 status 最多体验问题。
func (s *SettingService) SetSupportDocIndexStatus(ctx context.Context, status SupportDocIndexStatus) error {
	buf, err := json.Marshal(status)
	if err != nil {
		return fmt.Errorf("marshal doc index status: %w", err)
	}
	return s.settingRepo.Set(ctx, SettingKeySupportChatRAGDocIndexStatus, string(buf))
}

// GetSupportDocIndexStatus 读取 setting 中的 doc index 状态；不存在时返回 zero-value。
// admin GET /status 用。
func (s *SettingService) GetSupportDocIndexStatus(ctx context.Context) SupportDocIndexStatus {
	zero := SupportDocIndexStatus{
		State:  SupportDocIndexStateIdle,
		Errors: []SupportDocIndexError{},
	}
	v, err := s.settingRepo.Get(ctx, SettingKeySupportChatRAGDocIndexStatus)
	if err != nil || v == nil || strings.TrimSpace(v.Value) == "" {
		return zero
	}
	var st SupportDocIndexStatus
	if err := json.Unmarshal([]byte(v.Value), &st); err != nil {
		return zero
	}
	if st.Errors == nil {
		st.Errors = []SupportDocIndexError{}
	}
	return st
}
