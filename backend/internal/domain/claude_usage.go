package domain

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/Wei-Shaw/sub2api/internal/port/cache"
)

// ClaudeUsageWindow is a single Anthropic OAuth usage window.
type ClaudeUsageWindow struct {
	Utilization float64 `json:"utilization"`
	ResetsAt    string  `json:"resets_at"`
}

// ClaudeUsageResponse Anthropic API返回的usage结构
type ClaudeUsageResponse struct {
	FiveHour struct {
		Utilization float64 `json:"utilization"`
		ResetsAt    string  `json:"resets_at"`
	} `json:"five_hour"`
	SevenDay struct {
		Utilization float64 `json:"utilization"`
		ResetsAt    string  `json:"resets_at"`
	} `json:"seven_day"`
	SevenDaySonnet struct {
		Utilization float64 `json:"utilization"`
		ResetsAt    string  `json:"resets_at"`
	} `json:"seven_day_sonnet"`
	// Fable 专属 7d 窗口（对应响应头 7d_oi，claim 名为 seven_day_overage_included，
	// 见 anthropic-ratelimit-unified-representative-claim 头）。上游 usage API
	// 若不下发该字段，GetUsage 会用被动采样数据回填。
	SevenDayOverageIncluded ClaudeUsageWindow `json:"seven_day_overage_included"`
}

// ClaudeUsageFetchOptions 包含获取 Claude 用量数据所需的所有选项
type ClaudeUsageFetchOptions struct {
	AccessToken string                  // OAuth access token
	ProxyURL    string                  // 代理 URL（可选）
	AccountID   int64                   // 账号 ID（用于连接池隔离）
	TLSProfile  *tlsfingerprint.Profile // TLS 指纹 Profile（nil 表示不启用）
	Fingerprint *cache.Fingerprint      // 缓存的指纹信息（User-Agent 等）
}
