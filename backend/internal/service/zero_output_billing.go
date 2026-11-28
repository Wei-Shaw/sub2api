package service

import "github.com/Wei-Shaw/sub2api/internal/config"

// zeroOutputFreeCost 在启用配置且请求没有任何输出 token 时清零客户费用。
// 用量日志仍会记录真实的输入、输出和请求元数据；媒体与附加计费请求由调用方传入
// mediaUsage=true 后保持原有计费，避免图片、视频、语音或搜索请求误判为免费。
func zeroOutputFreeCost(
	cfg *config.Config,
	cost *CostBreakdown,
	outputTokens int,
	imageOutputTokens int,
	mediaUsage bool,
) *CostBreakdown {
	if cfg == nil || !cfg.Billing.ZeroOutputFree || cost == nil || mediaUsage {
		return cost
	}
	if outputTokens != 0 || imageOutputTokens != 0 {
		return cost
	}

	// 保留计费模式和长上下文标记，便于后台按原始计费路径筛选记录。
	mode := cost.BillingMode
	longContextApplied := cost.LongContextBillingApplied
	*cost = CostBreakdown{
		BillingMode:               mode,
		LongContextBillingApplied: longContextApplied,
	}
	return cost
}
