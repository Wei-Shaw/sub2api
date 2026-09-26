package handler

import (
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

// compositeAccountPoolCandidates 返回当前 composite 请求的账号池候选集合；
// 非池请求返回 false。与 service pool 语义一致：已有明确 resolved endpoint
// pin（如 Gemini fallback）时按 single 处理——候选数据保留在 ctx 但不作为
// 活跃池，handler/route 准入均以 pin 优先。
func compositeAccountPoolCandidates(c *gin.Context) ([]string, bool) {
	if c == nil || c.Request == nil {
		return nil, false
	}
	if _, resolved := service.ResolvedTargetPlatformFromContext(c.Request.Context()); resolved {
		return nil, false
	}
	candidates := service.CompositeCandidatePlatformsFromContext(c.Request.Context())
	if len(candidates) == 0 {
		return nil, false
	}
	return candidates, true
}

// compositeCandidatesWithinAllowed 报告账号池候选是否被允许集完整包含；
// 池不折叠为单一平台，任一候选越界即不允许。
func compositeCandidatesWithinAllowed(candidates, allowed []string) bool {
	if len(candidates) == 0 {
		return false
	}
	for _, candidate := range candidates {
		found := false
		for _, allowedPlatform := range allowed {
			if candidate == allowedPlatform {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func ensureCompositeTargetPlatform(c *gin.Context, apiKey *service.APIKey, model string) {
	if c == nil || c.Request == nil || apiKey == nil || apiKey.Group == nil || apiKey.Group.Platform != service.PlatformComposite {
		return
	}
	if _, ok := service.ResolvedTargetPlatformFromContext(c.Request.Context()); ok {
		return
	}
	// 账号池请求已由路由层写入完整池决策：不在这里做单平台 detector，
	// 避免把池折叠成一个平台或产生与 selector 不一致的二次解析。
	if _, ok := compositeAccountPoolCandidates(c); ok {
		return
	}
	if platform, ok := service.DetectModelPlatform(model); ok {
		c.Request = c.Request.WithContext(service.WithResolvedTargetPlatform(c.Request.Context(), platform))
	}
}

func compositeTargetPlatformAllowed(c *gin.Context, apiKey *service.APIKey, model string, allowed ...string) bool {
	if c == nil || c.Request == nil || apiKey == nil || apiKey.Group == nil || apiKey.Group.Platform != service.PlatformComposite {
		return true
	}
	ensureCompositeTargetPlatform(c, apiKey, model)
	if candidates, ok := compositeAccountPoolCandidates(c); ok {
		return compositeCandidatesWithinAllowed(candidates, allowed)
	}
	platform, ok := service.ResolvedTargetPlatformFromContext(c.Request.Context())
	if !ok {
		return false
	}
	for _, allowedPlatform := range allowed {
		if platform == allowedPlatform {
			return true
		}
	}
	return false
}

func compositeTargetPlatformResolved(c *gin.Context, apiKey *service.APIKey, model string) bool {
	if c == nil || c.Request == nil || apiKey == nil || apiKey.Group == nil || apiKey.Group.Platform != service.PlatformComposite {
		return true
	}
	ensureCompositeTargetPlatform(c, apiKey, model)
	// 账号池是已解析的完整决策（候选集合非空且已落 ctx），不因缺少单一
	// ResolvedTargetPlatform 而视为未解析。
	if _, ok := compositeAccountPoolCandidates(c); ok {
		return true
	}
	_, ok := service.ResolvedTargetPlatformFromContext(c.Request.Context())
	return ok
}

func effectiveAPIKeyPlatform(c *gin.Context, apiKey *service.APIKey) string {
	if c != nil && c.Request != nil {
		if platform, ok := service.ResolvedTargetPlatformFromContext(c.Request.Context()); ok {
			return platform
		}
	}
	if apiKey == nil || apiKey.Group == nil {
		return ""
	}
	return apiKey.Group.Platform
}

func openAIReasoningEffortPolicyForRequest(c *gin.Context, apiKey *service.APIKey) (string, []service.ReasoningEffortMapping, string, bool) {
	if apiKey == nil || apiKey.Group == nil {
		return "", nil, "", false
	}
	if apiKey.Group.Platform != service.PlatformAnthropic && apiKey.Group.Platform != service.PlatformOpenAI && apiKey.Group.Platform != service.PlatformComposite {
		return "", nil, "", false
	}
	effectivePlatform := effectiveAPIKeyPlatform(c, apiKey)
	if effectivePlatform != service.PlatformAnthropic && effectivePlatform != service.PlatformOpenAI {
		return "", nil, "", false
	}
	maxEffort, mappings := apiKey.Group.MaxReasoningEffort, apiKey.Group.ReasoningEffortMappings
	if effectivePlatform == service.PlatformAnthropic {
		maxEffort, mappings = anthropicCompatibleReasoningEffortPolicy(maxEffort, mappings)
	}
	return maxEffort, mappings, apiKey.Group.MaxReasoningEffortOverLimit, true
}

func anthropicReasoningEffortPolicyForRequest(c *gin.Context, apiKey *service.APIKey) (string, []service.ReasoningEffortMapping, string, bool) {
	if apiKey == nil || apiKey.Group == nil {
		return "", nil, "", false
	}
	if apiKey.Group.Platform != service.PlatformAnthropic && apiKey.Group.Platform != service.PlatformComposite {
		return "", nil, "", false
	}
	if effectiveAPIKeyPlatform(c, apiKey) != service.PlatformAnthropic {
		return "", nil, "", false
	}
	maxEffort, mappings := anthropicCompatibleReasoningEffortPolicy(apiKey.Group.MaxReasoningEffort, apiKey.Group.ReasoningEffortMappings)
	return maxEffort, mappings, apiKey.Group.MaxReasoningEffortOverLimit, true
}

func anthropicCompatibleReasoningEffortPolicy(maxEffort string, mappings []service.ReasoningEffortMapping) (string, []service.ReasoningEffortMapping) {
	if service.NormalizeMaxReasoningEffort(maxEffort) == "minimal" {
		maxEffort = "low"
	}
	normalizedMappings := append([]service.ReasoningEffortMapping(nil), mappings...)
	for i := range normalizedMappings {
		if service.NormalizeMaxReasoningEffort(normalizedMappings[i].To) == "minimal" {
			normalizedMappings[i].To = "low"
		}
	}
	return maxEffort, normalizedMappings
}

func bindRequestedReasoningEffort(c *gin.Context, body []byte, model string) {
	if c == nil || c.Request == nil {
		return
	}
	effort := service.CanonicalRequestedReasoningEffort(body, model)
	if effort == nil {
		return
	}
	c.Request = c.Request.WithContext(service.WithRequestedReasoningEffort(c.Request.Context(), *effort))
}

func stampOpenAIRequestedReasoningEffort(result *service.OpenAIForwardResult, c *gin.Context) {
	if result == nil || result.RequestedReasoningEffort != nil {
		return
	}
	if c == nil || c.Request == nil {
		return
	}
	result.RequestedReasoningEffort = service.RequestedReasoningEffortFromContext(c.Request.Context())
}

func stampForwardRequestedReasoningEffort(result *service.ForwardResult, requested *string) {
	if result == nil || result.RequestedReasoningEffort != nil {
		return
	}
	result.RequestedReasoningEffort = requested
}

func applyOpenAIReasoningEffortPolicyForRequest(c *gin.Context, apiKey *service.APIKey, body []byte) ([]byte, bool, error) {
	bindRequestedReasoningEffort(c, body, strings.TrimSpace(gjson.GetBytes(body, "model").String()))
	maxEffort, mappings, overLimit, ok := openAIReasoningEffortPolicyForRequest(c, apiKey)
	if !ok {
		return body, false, nil
	}
	return service.ApplyOpenAIReasoningEffortPolicy(body, maxEffort, mappings, overLimit)
}

func respondOpenAIReasoningEffortPolicyError(c *gin.Context, err error, write func(*gin.Context, int, string, string)) {
	if c == nil || err == nil || write == nil {
		return
	}
	service.MarkOpsClientBusinessLimited(c, service.OpsClientBusinessLimitedReasonLocalPolicyDenied)
	write(c, http.StatusForbidden, "permission_error", err.Error())
}

func applyAnthropicReasoningEffortPolicyForRequest(c *gin.Context, apiKey *service.APIKey, body []byte) ([]byte, bool, error) {
	maxEffort, mappings, overLimit, ok := anthropicReasoningEffortPolicyForRequest(c, apiKey)
	if !ok {
		return body, false, nil
	}
	return service.ApplyReasoningEffortPolicy(body, maxEffort, mappings, overLimit)
}

func bindOpenAIReasoningEffortPolicyForMessagesRequest(c *gin.Context, apiKey *service.APIKey, body []byte) {
	if c == nil || c.Request == nil {
		return
	}
	bindRequestedReasoningEffort(c, body, strings.TrimSpace(gjson.GetBytes(body, "model").String()))
	// The Messages bridge synthesizes a default OpenAI effort when
	// output_config.effort is omitted. Bind the group policy only for an
	// explicit client value so the ceiling does not alter that default.
	effort := gjson.GetBytes(body, "output_config.effort")
	if !effort.Exists() || effort.Type != gjson.String || strings.TrimSpace(effort.String()) == "" {
		return
	}
	maxEffort, mappings, overLimit, ok := openAIReasoningEffortPolicyForRequest(c, apiKey)
	if !ok {
		// composite 账号池请求没有单一目标平台，准入期判不出策略族；Forward
		// 服务侧按选中账号是否 openai 决定是否应用（与 single-openai 目标同
		// 语义），因此这里直接绑定组策略，其余平台的选中结果服务侧会跳过。
		if _, isPool := compositeAccountPoolCandidates(c); !isPool {
			return
		}
		maxEffort, mappings, overLimit = apiKey.Group.MaxReasoningEffort, apiKey.Group.ReasoningEffortMappings, apiKey.Group.MaxReasoningEffortOverLimit
	}
	c.Request = c.Request.WithContext(service.WithOpenAIReasoningEffortPolicy(c.Request.Context(), maxEffort, mappings, overLimit))
}

// compositePoolResponseStateGuardApplies 报告当前请求是否为 composite 账号池
// 请求（无单一目标 pin）。previous_response_id 状态归属守卫仅适用池请求；
// 单目标请求保留既有转发语义。
func compositePoolResponseStateGuardApplies(c *gin.Context, apiKey *service.APIKey) bool {
	if c == nil || c.Request == nil || apiKey == nil || apiKey.Group == nil || apiKey.Group.Platform != service.PlatformComposite {
		return false
	}
	_, isPool := compositeAccountPoolCandidates(c)
	return isPool
}

// releaseCompositePoolSelection 在选号之后、handler 抢槽之前的守卫早退路径上
// 释放 selector 已持有的账号槽位（Acquired=true 时 ReleaseFunc 非空）。置空
// ReleaseFunc 保证恰一次释放，不与后续 acquire/release 路径重复。
func releaseCompositePoolSelection(selection *service.AccountSelectionResult) {
	if selection != nil && selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
		selection.ReleaseFunc = nil
	}
}

// applyCompositePoolReasoningEffortPolicyForSelectedAccount 在账号选定后为
// composite 账号池请求补齐组 reasoning-effort 策略（CC/Responses 直转路径：
// 服务侧 forward 不再应用组策略）。池请求准入期没有单一目标平台，无法预判
// 策略族；选中 openai 账号时与 single-openai 同语义应用组上限/映射，选中
// grok/CN/OpenCode Go 时维持这些平台独立分组「不套用」的既有语义。单目标
// 请求已在准入期应用，这里跳过。err 非 nil 表示 deny/over-limit，调用方须以
// respondOpenAIReasoningEffortPolicyError 终止请求。
func applyCompositePoolReasoningEffortPolicyForSelectedAccount(c *gin.Context, apiKey *service.APIKey, account *service.Account, body []byte) ([]byte, bool, error) {
	if c == nil || c.Request == nil || apiKey == nil || apiKey.Group == nil || apiKey.Group.Platform != service.PlatformComposite || account == nil {
		return body, false, nil
	}
	if _, resolved := service.ResolvedTargetPlatformFromContext(c.Request.Context()); resolved {
		return body, false, nil
	}
	if _, isPool := compositeAccountPoolCandidates(c); !isPool {
		return body, false, nil
	}
	if account.Platform != service.PlatformOpenAI {
		return body, false, nil
	}
	return service.ApplyOpenAIReasoningEffortPolicy(body, apiKey.Group.MaxReasoningEffort, apiKey.Group.ReasoningEffortMappings, apiKey.Group.MaxReasoningEffortOverLimit)
}
