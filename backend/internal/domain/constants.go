package domain

import "strings"

// Status constants
const (
	StatusActive   = "active"
	StatusDisabled = "disabled"
	StatusError    = "error"
	StatusUnused   = "unused"
	StatusUsed     = "used"
	StatusExpired  = "expired"
)

// Role constants
const (
	RoleAdmin = "admin"
	RoleUser  = "user"
)

// Platform constants
const (
	PlatformAnthropic   = "anthropic"
	PlatformOpenAI      = "openai"
	PlatformGemini      = "gemini"
	PlatformAntigravity = "antigravity"
	PlatformKiro        = "kiro"
	PlatformGrok        = "grok"
	PlatformFal         = "fal"
	PlatformAtlasCloud  = "atlascloud"
	PlatformApiz        = "apiz"
	PlatformComposite   = "composite"
)

// Account type constants
const (
	AccountTypeOAuth          = "oauth"           // OAuth类型账号（full scope: profile + inference）
	AccountTypeSetupToken     = "setup-token"     // Setup Token类型账号（inference only scope）
	AccountTypeAPIKey         = "apikey"          // API Key类型账号
	AccountTypeUpstream       = "upstream"        // 上游透传类型账号（通过 Base URL + API Key 连接上游）
	AccountTypeBedrock        = "bedrock"         // AWS Bedrock 类型账号（通过 SigV4 签名或 API Key 连接 Bedrock，由 credentials.auth_mode 区分）
	AccountTypeServiceAccount = "service_account" // Google Service Account 类型账号（用于 Vertex AI）
)

// Redeem type constants
const (
	RedeemTypeBalance      = "balance"
	RedeemTypeConcurrency  = "concurrency"
	RedeemTypeSubscription = "subscription"
	RedeemTypeInvitation   = "invitation"
)

// PromoCode status constants
const (
	PromoCodeStatusActive   = "active"
	PromoCodeStatusDisabled = "disabled"
)

// Admin adjustment type constants
const (
	AdjustmentTypeAdminBalance     = "admin_balance"     // 管理员调整余额
	AdjustmentTypeAdminConcurrency = "admin_concurrency" // 管理员调整并发数
)

// Group subscription type constants
const (
	SubscriptionTypeStandard     = "standard"     // 标准计费模式（按余额扣费）
	SubscriptionTypeSubscription = "subscription" // 订阅模式（按限额控制）
)

// Subscription status constants
const (
	SubscriptionStatusActive    = "active"
	SubscriptionStatusExpired   = "expired"
	SubscriptionStatusSuspended = "suspended"
)

// AntigravityGemini31ProAgentModel is the upstream route for Gemini 3.1 Pro High.
const AntigravityGemini31ProAgentModel = "gemini-pro-agent"

// DefaultAntigravityModelMapping 是 Antigravity 平台的默认模型映射
// 当账号未配置 model_mapping 时使用此默认值
// 与前端 useModelWhitelist.ts 中的 antigravityDefaultMappings 保持一致
var DefaultAntigravityModelMapping = map[string]string{
	// Claude 白名单
	"claude-fable-5":             "claude-fable-5",           // 官方模型
	"claude-opus-4-8":            "claude-opus-4-8",          // 官方模型
	"claude-opus-4-7":            "claude-opus-4-7",          // 官方模型
	"claude-opus-4-6-thinking":   "claude-opus-4-6-thinking", // 官方模型
	"claude-opus-4-6":            "claude-opus-4-6-thinking", // 简称映射
	"claude-opus-4-5-thinking":   "claude-opus-4-6-thinking", // 迁移旧模型
	"claude-sonnet-4-6":          "claude-sonnet-4-6",
	"claude-sonnet-4-5":          "claude-sonnet-4-5",
	"claude-sonnet-4-5-thinking": "claude-sonnet-4-5-thinking",
	// Claude 详细版本 ID 映射
	"claude-opus-4-5-20251101":   "claude-opus-4-6-thinking", // 迁移旧模型
	"claude-sonnet-4-5-20250929": "claude-sonnet-4-5",
	// Claude Haiku → Sonnet（无 Haiku 支持）
	"claude-haiku-4-5":          "claude-sonnet-4-6",
	"claude-haiku-4-5-20251001": "claude-sonnet-4-6",
	// Gemini 2.5 白名单
	"gemini-2.5-flash":               "gemini-2.5-flash",
	"gemini-2.5-flash-image":         "gemini-2.5-flash-image",
	"gemini-2.5-flash-image-preview": "gemini-2.5-flash-image",
	"gemini-2.5-flash-lite":          "gemini-2.5-flash-lite",
	"gemini-2.5-flash-thinking":      "gemini-2.5-flash-thinking",
	"gemini-2.5-pro":                 "gemini-2.5-pro",
	// Gemini 3 白名单
	"gemini-3-flash":    "gemini-3-flash",
	"gemini-3-pro-high": "gemini-3-pro-high",
	"gemini-3-pro-low":  "gemini-3-pro-low",
	// Gemini 3 preview 映射
	"gemini-3-flash-preview": "gemini-3-flash",
	"gemini-3-pro-preview":   "gemini-3-pro-high",
	// Gemini 3.1 白名单
	AntigravityGemini31ProAgentModel: AntigravityGemini31ProAgentModel,
	"gemini-3.1-pro":                 AntigravityGemini31ProAgentModel,
	"gemini-3.1-pro-high":            AntigravityGemini31ProAgentModel,
	"gemini-3.1-pro-low":             "gemini-3.1-pro-low",
	// Gemini 3.1 preview 映射
	"gemini-3.1-pro-preview": AntigravityGemini31ProAgentModel,
	// Gemini 3.1 image 白名单
	"gemini-3.1-flash-image": "gemini-3.1-flash-image",
	// Gemini 3.1 image preview 映射
	"gemini-3.1-flash-image-preview": "gemini-3.1-flash-image",
	// Gemini 3.6 Flash tiered models
	"gemini-3.6-flash":        "gemini-3.6-flash",
	"gemini-3.6-flash-high":   "gemini-3.6-flash-high",
	"gemini-3.6-flash-low":    "gemini-3.6-flash-low",
	"gemini-3.6-flash-medium": "gemini-3.6-flash-medium",
	"gemini-3.6-flash-tiered": "gemini-3.6-flash-tiered",
	// Gemini 3 image 兼容映射（向 3.1 image 迁移）
	"gemini-3-pro-image":         "gemini-3.1-flash-image",
	"gemini-3-pro-image-preview": "gemini-3.1-flash-image",
	// 其他官方模型
	"gpt-oss-120b-medium":    "gpt-oss-120b-medium",
	"tab_flash_lite_preview": "tab_flash_lite_preview",
}

// DefaultKiroModelMapping 是 Kiro 平台的默认模型映射。
// 键为对外暴露/允许请求的模型名，值为实际发送到 Kiro 上游的模型名。
var DefaultKiroModelMapping = map[string]string{
	"claude-opus-4-8":                     "claude-opus-4.8",
	"claude-opus-4-8-thinking":            "claude-opus-4.8",
	"claude-opus-4-7":                     "claude-opus-4.7",
	"claude-opus-4-7-thinking":            "claude-opus-4.7",
	"claude-opus-4-6":                     "claude-opus-4.6",
	"claude-opus-4-6-thinking":            "claude-opus-4.6",
	"claude-sonnet-4-6":                   "claude-sonnet-4.6",
	"claude-sonnet-4-6-thinking":          "claude-sonnet-4.6",
	"claude-opus-4-5-20251101":            "claude-opus-4.5",
	"claude-opus-4-5-20251101-thinking":   "claude-opus-4.5",
	"claude-sonnet-4-5-20250929":          "claude-sonnet-4.5",
	"claude-sonnet-4-5-20250929-thinking": "claude-sonnet-4.5",
	"claude-haiku-4-5-20251001":           "claude-haiku-4.5",
	"claude-haiku-4-5-20251001-thinking":  "claude-haiku-4.5",
}

// DefaultBedrockModelMapping 是 AWS Bedrock 平台的默认模型映射
// 将 Anthropic 标准模型名映射到 Bedrock 模型 ID
// 注意：此处的 "us." 前缀仅为默认值，ResolveBedrockModelID 会根据账号配置的
// aws_region 自动调整为匹配的区域前缀（如 eu.、apac.、jp. 等）
var DefaultBedrockModelMapping = map[string]string{
	// Claude Fable
	"claude-fable-5": "anthropic.claude-fable-5",
	// Claude Opus
	"claude-opus-5":            "us.anthropic.claude-opus-5-v1",
	"claude-opus-4-8":          "us.anthropic.claude-opus-4-8-v1",
	"claude-opus-4-7":          "us.anthropic.claude-opus-4-7-v1",
	"claude-opus-4-6-thinking": "us.anthropic.claude-opus-4-6-v1",
	"claude-opus-4-6":          "us.anthropic.claude-opus-4-6-v1",
	"claude-opus-4-5-thinking": "us.anthropic.claude-opus-4-5-20251101-v1:0",
	"claude-opus-4-5-20251101": "us.anthropic.claude-opus-4-5-20251101-v1:0",
	"claude-opus-4-1":          "us.anthropic.claude-opus-4-1-20250805-v1:0",
	"claude-opus-4-20250514":   "us.anthropic.claude-opus-4-20250514-v1:0",
	// Claude Sonnet
	"claude-sonnet-5":            "us.anthropic.claude-sonnet-5-v1",
	"claude-sonnet-4-6-thinking": "us.anthropic.claude-sonnet-4-6",
	"claude-sonnet-4-6":          "us.anthropic.claude-sonnet-4-6",
	"claude-sonnet-4-5":          "us.anthropic.claude-sonnet-4-5-20250929-v1:0",
	"claude-sonnet-4-5-thinking": "us.anthropic.claude-sonnet-4-5-20250929-v1:0",
	"claude-sonnet-4-5-20250929": "us.anthropic.claude-sonnet-4-5-20250929-v1:0",
	"claude-sonnet-4-20250514":   "us.anthropic.claude-sonnet-4-20250514-v1:0",
	// Claude Haiku
	"claude-haiku-4-5":          "us.anthropic.claude-haiku-4-5-20251001-v1:0",
	"claude-haiku-4-5-20251001": "us.anthropic.claude-haiku-4-5-20251001-v1:0",
}

// DefaultFalModelMapping 是 fal 平台的默认模型映射
// 将对外的（OpenAI 风格）图片模型名映射为 fal 的应用 slug（用于拼接 queue/sync 端点路径）。
// 当账号/渠道未配置 model_mapping 时使用此默认值。
// 说明：fal 文生图 slug 为 openai/gpt-image-2，图生图/编辑 slug 为 openai/gpt-image-2/edit。
var DefaultFalModelMapping = map[string]string{
	// 文生图
	"gpt-image-2":        "openai/gpt-image-2",
	"openai/gpt-image-2": "openai/gpt-image-2",
	"gpt-image-1":        "openai/gpt-image-2",
	"dall-e-3":           "openai/gpt-image-2",
	// 图生图 / 编辑
	"gpt-image-2-edit":        "openai/gpt-image-2/edit",
	"gpt-image-2/edit":        "openai/gpt-image-2/edit",
	"openai/gpt-image-2/edit": "openai/gpt-image-2/edit",
}

// FalSlugTextToImage 是 fal 文生图默认应用 slug。
const FalSlugTextToImage = "openai/gpt-image-2"

// FalSlugImageEdit 是 fal 图生图/编辑默认应用 slug。
const FalSlugImageEdit = "openai/gpt-image-2/edit"

// FalQueueBaseURL 是 fal 队列协议默认 base URL（异步：submit/status/result/cancel）。
const FalQueueBaseURL = "https://queue.fal.run"

// FalSyncBaseURL 是 fal 同步协议默认 base URL。
const FalSyncBaseURL = "https://fal.run"

// AtlasCloud 上游REST 端点路径（相对 base_url）。
//
// atlascloud 采用异步轮询协议：
//
//	-提交视频：POST {base_url}/api/v1/model/generateVideo -> { id, status, ... }
//	- 提交图片：POST {base_url}/api/v1/model/generateImage -> { id, status, ... }
//	- 上传媒体：POST {base_url}/api/v1/model/uploadMedia
//	- 轮询结果：GET  {base_url}/api/v1/model/prediction/{id} -> { status, outputs:[...] }
//
// 鉴权：Authorization: Bearer {api_key}。base_url 无默认值，由账户凭证手动填写。
const (
	AtlasCloudPathGenerateVideo = "/api/v1/model/generateVideo"
	AtlasCloudPathGenerateImage = "/api/v1/model/generateImage"
	AtlasCloudPathUploadMedia   = "/api/v1/model/uploadMedia"
	AtlasCloudPathPrediction    = "/api/v1/model/prediction" // + /{id}
)

// Apiz 上游 REST 端点路径（相对 base_url）。
//
// apiz 采用"创建任务 + 查询任务"的异步协议，两个端点都是 POST：
//   - 创建任务：POST {base_url}/api/v3/tasks/create -> { task_id, status, ... }
//   - 查询任务：POST {base_url}/api/v3/tasks/query  body={ task_id } -> { status, video_url|outputs, ... }
//
// 鉴权：Authorization: Bearer {api_key}。
//
// 创建任务参数（透传客户端 payload）：
//
//	prompt(必填,1-5000) / duration(480P:4-30, 720P:4-29, 默认8)
//	resolution(480P|720P, 默认720P) / aspect_ratio(21:9|16:9|4:3|1:1|3:4|9:16)
//	audio(bool) / image_url(首帧,带图即图生视频) / end_image_url(尾帧,需同时给image_url)
//	reference_image_urls(<=30) / reference_video_urls(<=10) / reference_audio_urls(<=10)
const (
	ApizPathTasksCreate = "/api/v3/tasks/create"
	ApizPathTasksQuery  = "/api/v3/tasks/query"
)

// ApizBaseURL 是 apiz 平台默认 base URL，可由账户 credential "base_url" 覆盖。
const ApizBaseURL = "https://api.apiz.ai"

// ============================================================
// 视频模型识别
// ============================================================

// IsVideoModelName 判断一个 fal 模型名是否属于"视频模型"。
//
// 判定规则（动态、无白名单）：去掉可选的 "fal-ai/" 前缀后，
// 按 "/" 拆分若 ≥ 2 段则视为视频模型（例如
// "bytedance/seedance-2.5"、
// "bytedance/seedance-2.5/text-to-video"、
// "bytedance/seedance-2.0/mini/image-to-video"）。
//
// 该判定用于视频门面 /api/v1/model/{model} 的白名单，避免被当作任意
// fal endpoint 的透传通道；账号是否真正提供该模型另由分组内 fal
// 账号的 model_mapping 与"支持视频模型"开关决定。
func IsVideoModelName(model string) bool {
	m := strings.TrimSpace(model)
	if m == "" {
		return false
	}
	m = strings.TrimPrefix(m, "fal-ai/")
	m = strings.Trim(m, "/")
	if m == "" {
		return false
	}
	return strings.Count(m, "/") >= 1
}

// VideoModelsEnabledExtraKey 是视频平台账号 Extra 字段中"是否支持视频模型"的开关键。
//
// 语义：只有当账号的 Extra[FalVideoModelsEnabledExtraKey] == true 时，
// 该账号的 model_mapping 中两段及以上的模型才会作为视频模型暴露给用户菜单
// /user/video-models。开关关闭（缺省 false）时该账号不提供视频能力，
// 视频门面 /api/v1/model/{model} 不会调度到此账号。
const VideoModelsEnabledExtraKey = "fal_video_models_enabled"

// FalVideoModelsEnabledExtraKey 保留旧名称，兼容已有调用和持久化数据。
const FalVideoModelsEnabledExtraKey = VideoModelsEnabledExtraKey

// IsVideoModelsEnabled 从账号 Extra 中读出"支持视频模型"开关。
//
// 该函数容忍多种 JSON 反序列化形态：
//   - Go bool 直接对应 true/false；
//   - 字符串 "true"/"false"（忽略大小写、两侧空白）也能识别，防止前端字段类型漂移；
//   - 缺 key、nil、其它类型均视为 false。
func IsVideoModelsEnabled(extra map[string]any) bool {
	if len(extra) == 0 {
		return false
	}
	raw, ok := extra[VideoModelsEnabledExtraKey]
	if !ok {
		return false
	}
	switch v := raw.(type) {
	case bool:
		return v
	case string:
		return strings.EqualFold(strings.TrimSpace(v), "true")
	default:
		return false
	}
}

// IsFalVideoModelsEnabled 保留旧名称以兼容现有调用。
func IsFalVideoModelsEnabled(extra map[string]any) bool {
	return IsVideoModelsEnabled(extra)
}

// VideoModelSlugs 从账号模型映射中提取对外视频模型名。
// fal 的 value 是 endpoint；atlascloud/apiz 的 key 是统一门面的公开模型名。
func VideoModelSlugs(platform string, mapping map[string]string) []string {
	out := make([]string, 0, len(mapping))
	for requestedModel, upstreamModel := range mapping {
		candidate := requestedModel
		if platform == PlatformFal {
			candidate = upstreamModel
		}
		if slug := NormalizeFalVideoModelEndpoint(candidate); slug != "" {
			out = append(out, slug)
		}
	}
	return out
}

// NormalizeFalVideoModelEndpoint 把 fal endpoint 归一化为对外暴露的模型名。
//
// 输入是 model_mapping 的 value（如 "fal-ai/bytedance/seedance-2.5/text-to-video"
// 或已经去过前缀的 "bytedance/seedance-2.5/text-to-video"、"bytedance/seedance-2.5"）。
// 输出：去掉可选 "fal-ai/" 前缀、两侧 "/"，若段数 < 2 返回空字符串（非视频）。
//
// 保持简单：不做别名映射、不做美化 label，"配什么展示什么"。
func NormalizeFalVideoModelEndpoint(endpoint string) string {
	m := strings.TrimSpace(endpoint)
	if m == "" {
		return ""
	}
	m = strings.TrimPrefix(m, "fal-ai/")
	m = strings.Trim(m, "/")
	if m == "" {
		return ""
	}
	if strings.Count(m, "/") < 1 {
		return ""
	}
	return m
}
