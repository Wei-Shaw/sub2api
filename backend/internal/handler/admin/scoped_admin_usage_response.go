package admin

import (
	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/adminaccess"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// scopedAdminUsageLogResponse 仅公开该次请求的用量、计费快照和请求元数据。
// 关联资源只返回 ID；新增通用 DTO 字段不会自动获得受限密钥的读取权限。
func scopedAdminUsageLogResponse(record *service.UsageLog) gin.H {
	if record == nil {
		return nil
	}
	// 在转换前移除关联，避免构造含原始密钥、用户余额等内容的嵌套 DTO。
	flat := *record
	flat.User, flat.APIKey, flat.Group, flat.Subscription, flat.Account = nil, nil, nil, nil, nil
	usage := dto.UsageLogFromServiceAdmin(&flat)
	return gin.H{
		"id":                           usage.ID,
		"user_id":                      usage.UserID,
		"api_key_id":                   usage.APIKeyID,
		"account_id":                   usage.AccountID,
		"request_id":                   usage.RequestID,
		"model":                        usage.Model,
		"service_tier":                 usage.ServiceTier,
		"reasoning_effort":             usage.ReasoningEffort,
		"inbound_endpoint":             usage.InboundEndpoint,
		"upstream_endpoint":            usage.UpstreamEndpoint,
		"group_id":                     usage.GroupID,
		"subscription_id":              usage.SubscriptionID,
		"input_tokens":                 usage.InputTokens,
		"output_tokens":                usage.OutputTokens,
		"cache_creation_tokens":        usage.CacheCreationTokens,
		"cache_read_tokens":            usage.CacheReadTokens,
		"cache_creation_5m_tokens":     usage.CacheCreation5mTokens,
		"cache_creation_1h_tokens":     usage.CacheCreation1hTokens,
		"input_cost":                   usage.InputCost,
		"output_cost":                  usage.OutputCost,
		"cache_creation_cost":          usage.CacheCreationCost,
		"cache_read_cost":              usage.CacheReadCost,
		"total_cost":                   usage.TotalCost,
		"actual_cost":                  usage.ActualCost,
		"rate_multiplier":              usage.RateMultiplier,
		"long_context_billing_applied": usage.LongContextBillingApplied,
		"billing_type":                 usage.BillingType,
		"request_type":                 usage.RequestType,
		"stream":                       usage.Stream,
		"openai_ws_mode":               usage.OpenAIWSMode,
		"native_compaction_v2":         usage.NativeCompactionV2,
		"duration_ms":                  usage.DurationMs,
		"first_token_ms":               usage.FirstTokenMs,
		"image_count":                  usage.ImageCount,
		"image_size":                   usage.ImageSize,
		"image_input_size":             usage.ImageInputSize,
		"image_output_size":            usage.ImageOutputSize,
		"image_input_tokens":           usage.ImageInputTokens,
		"image_input_cost":             usage.ImageInputCost,
		"image_output_tokens":          usage.ImageOutputTokens,
		"image_output_cost":            usage.ImageOutputCost,
		"image_size_source":            usage.ImageSizeSource,
		"image_size_breakdown":         usage.ImageSizeBreakdown,
		"media_type":                   usage.MediaType,
		"user_agent":                   usage.UserAgent,
		"ip_address":                   usage.IPAddress,
		"session_id":                   usage.SessionID,
		"cache_ttl_overridden":         usage.CacheTTLOverridden,
		"billing_mode":                 usage.BillingMode,
		"created_at":                   usage.CreatedAt,
		"upstream_model":               usage.UpstreamModel,
		"upstream_reasoning_effort":    usage.UpstreamReasoningEffort,
		"upstream_response_model":      usage.UpstreamResponseModel,
		"upstream_model_mismatch":      usage.UpstreamModelMismatch,
		"channel_id":                   usage.ChannelID,
		"model_mapping_chain":          usage.ModelMappingChain,
		"upstream_request_id":          usage.UpstreamRequestID,
		"billing_tier":                 usage.BillingTier,
		"account_rate_multiplier":      usage.AccountRateMultiplier,
		"account_stats_cost":           usage.AccountStatsCost,
	}
}

func adminUsageLogResponses(c *gin.Context, records []service.UsageLog) any {
	if adminaccess.FromContext(c.Request.Context()) != nil {
		out := make([]gin.H, 0, len(records))
		for i := range records {
			out = append(out, scopedAdminUsageLogResponse(&records[i]))
		}
		return out
	}
	out := make([]dto.AdminUsageLog, 0, len(records))
	for i := range records {
		out = append(out, *dto.UsageLogFromServiceAdmin(&records[i]))
	}
	return out
}
