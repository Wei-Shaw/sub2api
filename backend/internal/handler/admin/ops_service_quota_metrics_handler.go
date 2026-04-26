package admin

import (
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/metrics"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/gin-gonic/gin"
)

// labelKeySeparator 与 metrics 包内 joinLabelValues 中使用的分隔符保持一致 (ASCII Unit Separator)。
// 因 metrics 包未导出该常量，此处复刻一份，仅用于反向解码 Snapshot key。
const labelKeySeparator = "\x1f"

// 服务配额 metrics 端点中导出的 PreCheck / FailOpen label 名称
// （与 metrics.ServiceQuotaPreCheckTotal / ServiceQuotaFailOpenTotal 的 LabelNames 一致）。
const (
	serviceQuotaLabelRuleID      = "rule_id"
	serviceQuotaLabelLimiterType = "limiter_type"
	serviceQuotaLabelResult      = "result"
	serviceQuotaLabelReason      = "reason"
)

// ServiceQuotaPreCheckMetric 描述单个 (rule, limiter, result) 桶的累计计数。
type ServiceQuotaPreCheckMetric struct {
	RuleID      string `json:"rule_id"`
	LimiterType string `json:"limiter_type"`
	Result      string `json:"result"`
	Count       uint64 `json:"count"`
}

// ServiceQuotaFailOpenMetric 描述单个 (limiter, reason) 桶的累计计数。
type ServiceQuotaFailOpenMetric struct {
	LimiterType string `json:"limiter_type"`
	Reason      string `json:"reason"`
	Count       uint64 `json:"count"`
}

// ServiceQuotaMetricsResponse 是 GET /api/v1/admin/ops/service-quota-metrics 的返回结构。
type ServiceQuotaMetricsResponse struct {
	PreCheck []ServiceQuotaPreCheckMetric `json:"precheck"`
	FailOpen []ServiceQuotaFailOpenMetric `json:"fail_open"`
}

// GetServiceQuotaMetrics 返回 service quota PreCheck / fail-open 的内存计数快照。
//
// 端点不依赖 ops monitoring 开关：metrics 是进程内 sync.Map+atomic 计数器，
// 即使 monitoring 关闭也希望运维能查到 PreCheck 拒绝率 / fail-open 率。
//
// GET /api/v1/admin/ops/service-quota-metrics
func (h *OpsHandler) GetServiceQuotaMetrics(c *gin.Context) {
	resp := ServiceQuotaMetricsResponse{
		PreCheck: collectPreCheckMetrics(),
		FailOpen: collectFailOpenMetrics(),
	}
	response.Success(c, resp)
}

// collectPreCheckMetrics 拉取 ServiceQuotaPreCheckTotal 的所有桶并解开 label。
func collectPreCheckMetrics() []ServiceQuotaPreCheckMetric {
	snap := metrics.ServiceQuotaPreCheckTotal.Snapshot()
	labelNames := metrics.ServiceQuotaPreCheckTotal.LabelNames()
	out := make([]ServiceQuotaPreCheckMetric, 0, len(snap))
	for key, count := range snap {
		labels := decodeLabelKey(key, labelNames)
		out = append(out, ServiceQuotaPreCheckMetric{
			RuleID:      labels[serviceQuotaLabelRuleID],
			LimiterType: labels[serviceQuotaLabelLimiterType],
			Result:      labels[serviceQuotaLabelResult],
			Count:       count,
		})
	}
	return out
}

// collectFailOpenMetrics 拉取 ServiceQuotaFailOpenTotal 的所有桶并解开 label。
func collectFailOpenMetrics() []ServiceQuotaFailOpenMetric {
	snap := metrics.ServiceQuotaFailOpenTotal.Snapshot()
	labelNames := metrics.ServiceQuotaFailOpenTotal.LabelNames()
	out := make([]ServiceQuotaFailOpenMetric, 0, len(snap))
	for key, count := range snap {
		labels := decodeLabelKey(key, labelNames)
		out = append(out, ServiceQuotaFailOpenMetric{
			LimiterType: labels[serviceQuotaLabelLimiterType],
			Reason:      labels[serviceQuotaLabelReason],
			Count:       count,
		})
	}
	return out
}

// decodeLabelKey 把 Snapshot 的 \x1f 拼接 key 还原成 label 名 -> 值 的 map。
// labelNames 缺失或 key 段数对不上时按位置截断/补空，避免 panic。
func decodeLabelKey(key string, labelNames []string) map[string]string {
	parts := strings.Split(key, labelKeySeparator)
	out := make(map[string]string, len(labelNames))
	for i, name := range labelNames {
		if i < len(parts) {
			out[name] = parts[i]
		} else {
			out[name] = ""
		}
	}
	return out
}
