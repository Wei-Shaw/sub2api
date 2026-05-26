package dto

import (
	"time"

	monitorservice "github.com/Wei-Shaw/sub2api/plugins/channel-management/monitor/service"
)

const (
	// monitorAPIKeyMaskPrefix 脱敏时保留的明文前缀长度。
	monitorAPIKeyMaskPrefix = 4
	// monitorAPIKeyMaskSuffix 脱敏后追加的占位字符串。
	monitorAPIKeyMaskSuffix = "***"
)

// MaskAPIKey 对 API Key 明文做脱敏：前 4 字符 + "***"，长度 ≤ 4 时只显示 "***"。
// 暴露成首字母大写，handler 直接 `dto.MaskAPIKey(...)` 复用 — 之前 admin
// handler 自己写了一份私有 maskAPIKey, 现在合并为单点。
func MaskAPIKey(plain string) string {
	if len(plain) <= monitorAPIKeyMaskPrefix {
		return monitorAPIKeyMaskSuffix
	}
	return plain[:monitorAPIKeyMaskPrefix] + monitorAPIKeyMaskSuffix
}

// MonitorToResponse 把 domain 模型转成 admin/list/get 接口共用的 view。
//
// 拆三个 helper 让本函数 ≤ 30 行：
//   - normalizeMonitorCollections：[]string / map nil 归一化为空集合
//   - assignMonitorTimestamps：UTC RFC3339 时间格式化（含可空 LastCheckedAt）
//
// summary 字段在 List 接口需要批量聚合后再注入，详见 ApplyMonitorSummary。
func MonitorToResponse(m *monitorservice.ChannelMonitor) *ChannelMonitorResponse {
	if m == nil {
		return nil
	}
	extras, headers := normalizeMonitorCollections(m)
	resp := &ChannelMonitorResponse{
		ID:                  m.ID,
		Name:                m.Name,
		Provider:            m.Provider,
		Endpoint:            m.Endpoint,
		APIKeyMasked:        MaskAPIKey(m.APIKey),
		APIKeyDecryptFailed: m.APIKeyDecryptFailed,
		PrimaryModel:        m.PrimaryModel,
		ExtraModels:         extras,
		GroupName:           m.GroupName,
		Enabled:             m.Enabled,
		IntervalSeconds:     m.IntervalSeconds,
		CreatedBy:           m.CreatedBy,
		TemplateID:          m.TemplateID,
		ExtraHeaders:        headers,
		BodyOverrideMode:    m.BodyOverrideMode,
		BodyOverride:        m.BodyOverride,
		ExtraModelsStatus:   []ChannelMonitorExtraModelStatus{},
	}
	assignMonitorTimestamps(m, resp)
	return resp
}

// normalizeMonitorCollections 把 ExtraModels / ExtraHeaders 的 nil 归一化为空集合，
// 让前端不需要区分 nil 与空数组。
func normalizeMonitorCollections(m *monitorservice.ChannelMonitor) ([]string, map[string]string) {
	extras := m.ExtraModels
	if extras == nil {
		extras = []string{}
	}
	headers := m.ExtraHeaders
	if headers == nil {
		headers = map[string]string{}
	}
	return extras, headers
}

// assignMonitorTimestamps 把 monitor 的 CreatedAt / UpdatedAt / LastCheckedAt
// 写入 response 的对应 RFC3339 UTC 字符串字段。
func assignMonitorTimestamps(m *monitorservice.ChannelMonitor, resp *ChannelMonitorResponse) {
	resp.CreatedAt = m.CreatedAt.UTC().Format(time.RFC3339)
	resp.UpdatedAt = m.UpdatedAt.UTC().Format(time.RFC3339)
	if m.LastCheckedAt != nil {
		s := m.LastCheckedAt.UTC().Format(time.RFC3339)
		resp.LastCheckedAt = &s
	}
}

// ApplyMonitorSummary 把批量聚合得到的 summary 写入 response 的衍生字段。
// admin List 接口先 BatchMonitorStatusSummary 一次拿到所有 id 的汇总，
// 再对每个 monitor 调本函数 — 由此消除 N+1 SQL。
func ApplyMonitorSummary(resp *ChannelMonitorResponse, summary monitorservice.MonitorStatusSummary) {
	if resp == nil {
		return
	}
	resp.PrimaryStatus = summary.PrimaryStatus
	resp.PrimaryLatencyMs = summary.PrimaryLatencyMs
	resp.Availability7d = summary.Availability7d
	resp.ExtraModelsStatus = make([]ChannelMonitorExtraModelStatus, 0, len(summary.ExtraModels))
	for _, e := range summary.ExtraModels {
		resp.ExtraModelsStatus = append(resp.ExtraModelsStatus, ChannelMonitorExtraModelStatus{
			Model:     e.Model,
			Status:    e.Status,
			LatencyMs: e.LatencyMs,
		})
	}
}

// CheckResultToResponse RunNow 返回的单条结果。
func CheckResultToResponse(r *monitorservice.CheckResult) ChannelMonitorCheckResultResponse {
	return ChannelMonitorCheckResultResponse{
		Model:         r.Model,
		Status:        r.Status,
		LatencyMs:     r.LatencyMs,
		PingLatencyMs: r.PingLatencyMs,
		Message:       r.Message,
		CheckedAt:     r.CheckedAt.UTC().Format(time.RFC3339),
	}
}

// HistoryEntryToResponse history 接口单行映射。
func HistoryEntryToResponse(e *monitorservice.ChannelMonitorHistoryEntry) ChannelMonitorHistoryItemResponse {
	return ChannelMonitorHistoryItemResponse{
		ID:            e.ID,
		Model:         e.Model,
		Status:        e.Status,
		LatencyMs:     e.LatencyMs,
		PingLatencyMs: e.PingLatencyMs,
		Message:       e.Message,
		CheckedAt:     e.CheckedAt.UTC().Format(time.RFC3339),
	}
}
