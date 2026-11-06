package handler

import (
	"encoding/json"
	"net/http"
	"slices"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	"github.com/Wei-Shaw/sub2api/internal/pkg/gemini"
	"github.com/Wei-Shaw/sub2api/internal/pkg/googleapi"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// GeminiV1BetaListModels proxies:
// GET /v1beta/models
func (h *GatewayHandler) GeminiV1BetaListModels(c *gin.Context) {
	apiKey, ok := middleware.GetAPIKeyFromContext(c)
	if !ok || apiKey == nil {
		googleError(c, http.StatusUnauthorized, "Invalid API key")
		return
	}
	// 检查平台：优先使用强制平台（/antigravity 路由），否则要求 gemini 分组（composite 分组允许路由到 antigravity）
	forcePlatform, hasForcePlatform := middleware.GetForcePlatformFromContext(c)
	effectivePlatform := effectiveAPIKeyPlatform(c, apiKey)
	if !hasForcePlatform && effectivePlatform != service.PlatformGemini &&
		!servesCompositeAntigravityGemini(apiKey, effectivePlatform) {
		googleError(c, http.StatusBadRequest, "API key group platform is not gemini")
		return
	}

	// 分组级模型白名单开启时过滤 models[].name（名字形如 models/xxx）。
	filterGeminiModels := func(models []gemini.Model) []gemini.Model {
		if apiKey.Group == nil || !apiKey.Group.ModelAllowlistEnabled() {
			return models
		}
		filtered := make([]gemini.Model, 0, len(models))
		for _, model := range models {
			if apiKey.Group.ModelAllowlist.Allows(model.Name) {
				filtered = append(filtered, model)
			}
		}
		return filtered
	}

	agModelIDs, err := h.geminiCompatService.AntigravityGeminiModelIDs(c.Request.Context(), apiKey.GroupID, forcePlatform != service.PlatformAntigravity)
	if err != nil {
		googleError(c, http.StatusServiceUnavailable, "Unable to list Antigravity models")
		return
	}
	agModels := make([]gemini.Model, 0, len(agModelIDs))
	for _, id := range agModelIDs {
		agModels = append(agModels, gemini.FallbackModel(id))
	}
	if forcePlatform == service.PlatformAntigravity {
		c.JSON(http.StatusOK, gemini.ModelsListResponse{Models: filterGeminiModels(agModels)})
		return
	}

	// 分组内 Gemini 账号都声明了 model_mapping 时，列表与 /v1/models 同源（含白名单口径），不透传单个账号的上游列表。
	if declared, err := h.geminiCompatService.DeclaredModelIDs(c.Request.Context(), apiKey.GroupID); err == nil && len(declared) > 0 {
		ids := append([]string{}, declared...)
		for _, id := range agModelIDs {
			if !slices.Contains(declared, id) {
				ids = append(ids, id)
			}
		}
		if apiKey.Group != nil && apiKey.Group.ModelAllowlistEnabled() {
			ids = apiKey.Group.ModelAllowlist.FilterForListing(ids)
		}
		declaredModels := make([]gemini.Model, 0, len(ids))
		for _, id := range ids {
			declaredModels = append(declaredModels, gemini.FallbackModel(id))
		}
		c.JSON(http.StatusOK, gemini.ModelsListResponse{Models: declaredModels})
		return
	}

	account, err := h.geminiCompatService.SelectAccountForModelsEndpoints(c.Request.Context(), apiKey.GroupID)
	if err != nil {
		if len(agModels) > 0 {
			c.JSON(http.StatusOK, gemini.ModelsListResponse{Models: filterGeminiModels(agModels)})
			return
		}
		markOpsRoutingCapacityLimitedIfNoAvailable(c, err)
		googleError(c, http.StatusServiceUnavailable, "No available Gemini accounts: "+err.Error())
		return
	}

	res, err := h.geminiCompatService.ForwardListModels(c.Request.Context(), account)
	if err != nil {
		googleError(c, http.StatusBadGateway, err.Error())
		return
	}
	if shouldFallbackGeminiModels(res) {
		c.JSON(http.StatusOK, gemini.ModelsListResponse{Models: filterGeminiModels(mergeGeminiModelLists(gemini.DefaultModels(), agModels))})
		return
	}
	if res.StatusCode == http.StatusOK && len(agModels) > 0 {
		if merged, ok := appendUpstreamGeminiModels(res.Body, agModels); ok {
			res.Body = merged
		}
	}

	if apiKey.Group != nil && apiKey.Group.ModelAllowlistEnabled() {
		if filtered, dropped, ok := filterUpstreamGeminiModelsBody(res.Body, apiKey.Group.ModelAllowlist); ok && dropped {
			// 只在确有条目被过滤时替换响应体；全命中或解析失败时保持原始响应，
			// 统一经 writeUpstreamResponse 写出（保留全部上游响应头）。
			res.Body = filtered
		}
	}
	writeUpstreamResponse(c, res)
}

// mergeGeminiModelLists keeps native metadata when both sources advertise a model.
func mergeGeminiModelLists(native, extra []gemini.Model) []gemini.Model {
	result := append([]gemini.Model{}, native...)
	seen := make(map[string]bool, len(native))
	for _, model := range native {
		seen[model.Name] = true
	}
	for _, model := range extra {
		if !seen[model.Name] {
			result = append(result, model)
			seen[model.Name] = true
		}
	}
	return result
}

// appendUpstreamGeminiModels preserves unknown model metadata and envelope fields.
func appendUpstreamGeminiModels(body []byte, extra []gemini.Model) ([]byte, bool) {
	var envelope map[string]json.RawMessage
	if json.Unmarshal(body, &envelope) != nil || envelope == nil {
		return body, false
	}
	var models []json.RawMessage
	raw, exists := envelope["models"]
	if !exists || json.Unmarshal(raw, &models) != nil {
		return body, false
	}
	seen := make(map[string]bool, len(models))
	for _, raw := range models {
		var model struct {
			Name string `json:"name"`
		}
		if json.Unmarshal(raw, &model) != nil {
			return body, false
		}
		seen[model.Name] = true
	}
	changed := false
	for _, model := range extra {
		if seen[model.Name] {
			continue
		}
		raw, err := json.Marshal(model)
		if err != nil {
			return body, false
		}
		models = append(models, raw)
		seen[model.Name] = true
		changed = true
	}
	if !changed {
		return body, true
	}
	envelope["models"], _ = json.Marshal(models)
	merged, err := json.Marshal(envelope)
	return merged, err == nil
}

// filterUpstreamGeminiModelsBody 按白名单过滤上游 /v1beta/models 响应中的
// models[].name，其余信封字段（如 nextPageToken）原样保留。
// 返回值：filtered 为过滤后的响应体；dropped 表示是否有条目被移除（全命中时
// 为 false，调用方应保持原始响应以完整透传上游头）；ok=false 表示解析失败，
// 调用方同样应透传原始响应。
func filterUpstreamGeminiModelsBody(body []byte, allowlist service.GroupModelAllowlist) (filtered []byte, dropped bool, ok bool) {
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, false, false
	}
	rawModels, hasModels := envelope["models"]
	if !hasModels {
		return body, false, true
	}
	type geminiModelName struct {
		Name string `json:"name"`
	}
	var models []json.RawMessage
	if err := json.Unmarshal(rawModels, &models); err != nil {
		return nil, false, false
	}
	kept := make([]json.RawMessage, 0, len(models))
	for _, raw := range models {
		var model geminiModelName
		if err := json.Unmarshal(raw, &model); err != nil {
			return nil, false, false
		}
		if allowlist.Allows(model.Name) {
			kept = append(kept, raw)
		}
	}
	if len(kept) == len(models) {
		// 全部命中时直接透传原始响应体。
		return body, false, true
	}
	mergedModels, err := json.Marshal(kept)
	if err != nil {
		return nil, false, false
	}
	envelope["models"] = mergedModels
	merged, err := json.Marshal(envelope)
	if err != nil {
		return nil, false, false
	}
	return merged, true, true
}

// GeminiV1BetaGetModel proxies:
// GET /v1beta/models/{model}
func (h *GatewayHandler) GeminiV1BetaGetModel(c *gin.Context) {
	apiKey, ok := middleware.GetAPIKeyFromContext(c)
	if !ok || apiKey == nil {
		googleError(c, http.StatusUnauthorized, "Invalid API key")
		return
	}
	// 检查平台：优先使用强制平台（/antigravity 路由），否则要求 gemini 分组（composite 分组允许路由到 antigravity）
	forcePlatform, hasForcePlatform := middleware.GetForcePlatformFromContext(c)
	effectivePlatform := effectiveAPIKeyPlatform(c, apiKey)
	if !hasForcePlatform && effectivePlatform != service.PlatformGemini &&
		!servesCompositeAntigravityGemini(apiKey, effectivePlatform) {
		googleError(c, http.StatusBadRequest, "API key group platform is not gemini")
		return
	}

	modelName := strings.TrimSpace(c.Param("model"))
	if modelName == "" {
		googleError(c, http.StatusBadRequest, "Missing model in URL")
		return
	}
	// 模型名会被拼进上游 URL 的 path，先在入口校验片段合规性，
	// 见 service/upstream_path_guard.go。
	if !service.IsSafeGeminiModelPathSegment(modelName) {
		googleError(c, http.StatusBadRequest, "Invalid model in URL")
		return
	}
	if resolvedModel, ok := service.ResolvedUpstreamModelFromContext(c.Request.Context()); ok && strings.TrimSpace(resolvedModel) != "" {
		modelName = strings.TrimSpace(resolvedModel)
	}
	if !hasForcePlatform && servesCompositeAntigravityGemini(apiKey, effectivePlatform) && !isAntigravityGeminiModel(modelName) {
		googleError(c, http.StatusBadRequest, "API key group platform is not gemini")
		return
	}

	// 强制 antigravity 模式或 composite 路由到 antigravity：返回 antigravity 模型信息
	if forcePlatform == service.PlatformAntigravity || effectivePlatform == service.PlatformAntigravity {
		c.JSON(http.StatusOK, antigravity.FallbackGeminiModel(modelName))
		return
	}

	// 分组内 Gemini 账号都声明了 model_mapping 时，按列表接口的同一来源判断模型是否存在。
	if declared, err := h.geminiCompatService.DeclaredModelIDs(c.Request.Context(), apiKey.GroupID); err == nil && len(declared) > 0 {
		if agModelIDs, agErr := h.geminiCompatService.AntigravityGeminiModelIDs(c.Request.Context(), apiKey.GroupID, true); agErr == nil {
			if service.DeclaredModelsCover(declared, modelName) || service.DeclaredModelsCover(agModelIDs, modelName) {
				c.JSON(http.StatusOK, gemini.FallbackModel(modelName))
				return
			}
			googleError(c, http.StatusNotFound, "models/"+modelName+" is not found")
			return
		}
	}

	account, err := h.geminiCompatService.SelectAccountForModelsEndpoints(c.Request.Context(), apiKey.GroupID)
	if err != nil {
		// 没有 gemini 账户，检查是否有 antigravity 账户可用
		hasAntigravity, _ := h.geminiCompatService.HasAntigravityAccounts(c.Request.Context(), apiKey.GroupID)
		if hasAntigravity {
			// antigravity 账户使用静态模型信息
			c.JSON(http.StatusOK, gemini.FallbackModel(modelName))
			return
		}
		markOpsRoutingCapacityLimitedIfNoAvailable(c, err)
		googleError(c, http.StatusServiceUnavailable, "No available Gemini accounts: "+err.Error())
		return
	}

	res, err := h.geminiCompatService.ForwardGetModel(c.Request.Context(), account, modelName)
	if err != nil {
		googleError(c, http.StatusBadGateway, err.Error())
		return
	}
	if shouldFallbackGeminiModel(modelName, res) {
		c.JSON(http.StatusOK, gemini.FallbackModel(modelName))
		return
	}
	writeUpstreamResponse(c, res)
}

func shouldFallbackGeminiModels(res *service.UpstreamHTTPResult) bool {
	if res == nil {
		return true
	}
	if res.StatusCode != http.StatusUnauthorized && res.StatusCode != http.StatusForbidden {
		return false
	}
	if strings.Contains(strings.ToLower(res.Headers.Get("Www-Authenticate")), "insufficient_scope") {
		return true
	}
	if strings.Contains(strings.ToLower(string(res.Body)), "insufficient authentication scopes") {
		return true
	}
	if strings.Contains(strings.ToLower(string(res.Body)), "access_token_scope_insufficient") {
		return true
	}
	return false
}

func shouldFallbackGeminiModel(modelName string, res *service.UpstreamHTTPResult) bool {
	if shouldFallbackGeminiModels(res) {
		return true
	}
	if res == nil || res.StatusCode != http.StatusNotFound {
		return false
	}
	return gemini.HasFallbackModel(modelName)
}

func parseGeminiModelAction(rest string) (model string, action string, err error) {
	rest = strings.TrimSpace(rest)
	if rest == "" {
		return "", "", &pathParseError{"missing path"}
	}
	if i := strings.Index(rest, ":"); i > 0 && i < len(rest)-1 {
		return rest[:i], rest[i+1:], nil
	}
	if i := strings.Index(rest, "/"); i > 0 && i < len(rest)-1 {
		return rest[:i], rest[i+1:], nil
	}
	return "", "", &pathParseError{"invalid model action path"}
}

func (h *GatewayHandler) handleGeminiFailoverExhausted(c *gin.Context, failoverErr *service.UpstreamFailoverError) {
	if failoverErr == nil {
		googleError(c, http.StatusBadGateway, "Upstream request failed")
		return
	}
	statusCode := failoverErr.StatusCode
	responseBody := failoverErr.ResponseBody

	if h.errorPassthroughService != nil && len(responseBody) > 0 {
		if rule := h.errorPassthroughService.MatchRule(service.PlatformGemini, statusCode, responseBody); rule != nil {
			respCode := statusCode
			if !rule.PassthroughCode && rule.ResponseCode != nil {
				respCode = *rule.ResponseCode
			}
			msg := service.ExtractUpstreamErrorMessage(responseBody)
			if !rule.PassthroughBody && rule.CustomMessage != nil {
				msg = *rule.CustomMessage
			}
			if rule.SkipMonitoring {
				c.Set(service.OpsSkipPassthroughKey, true)
			}
			googleErrorWithType(c, respCode, "", "", msg)
			return
		}
	}

	upstreamMsg := service.ExtractUpstreamErrorMessage(responseBody)
	service.SetOpsUpstreamError(c, statusCode, upstreamMsg, "")
	status, message := mapGeminiUpstreamError(statusCode)
	googleError(c, status, message)
}

func mapGeminiUpstreamError(statusCode int) (int, string) {
	switch statusCode {
	case 401:
		return http.StatusBadGateway, "Upstream authentication failed, please contact administrator"
	case 403:
		return http.StatusBadGateway, "Upstream access forbidden, please contact administrator"
	case 429:
		return http.StatusTooManyRequests, "Upstream rate limit exceeded, please retry later"
	case 529:
		return http.StatusServiceUnavailable, "Upstream service overloaded, please retry later"
	case 500, 502, 503, 504:
		return http.StatusBadGateway, "Upstream service temporarily unavailable"
	default:
		return http.StatusBadGateway, "Upstream request failed"
	}
}

type pathParseError struct{ msg string }

func (e *pathParseError) Error() string { return e.msg }

func googleError(c *gin.Context, status int, message string) {
	googleErrorWithType(c, status, googleClientErrorType(status), "", message)
}

func googlePlatformError(c *gin.Context, status int, message string) {
	googleErrorWithType(c, status, "", "", message)
}

func googleErrorWithType(c *gin.Context, status int, errType, code, message string) {
	service.SetOpsLocalErrorType(c, errType, code)
	c.JSON(status, gin.H{
		"error": gin.H{
			"code":    status,
			"message": message,
			"status":  googleapi.HTTPStatusToGoogleStatus(status),
		},
	})
}

func googleClientErrorType(status int) string {
	switch status {
	case http.StatusBadRequest, http.StatusRequestEntityTooLarge:
		return "invalid_request_error"
	case http.StatusUnauthorized:
		return "authentication_error"
	case http.StatusForbidden:
		return "permission_error"
	case http.StatusNotFound:
		return "not_found_error"
	default:
		return ""
	}
}

func googleConcurrencyError(c *gin.Context, err error, slotType string) {
	status, errType, code, message := concurrencyErrorResponse(err, slotType)
	googleErrorWithType(c, status, errType, code, message)
}

func writeUpstreamResponse(c *gin.Context, res *service.UpstreamHTTPResult) {
	if res == nil {
		googleError(c, http.StatusBadGateway, "Empty upstream response")
		return
	}
	for k, vv := range res.Headers {
		if strings.EqualFold(k, "Content-Length") || strings.EqualFold(k, "Transfer-Encoding") || strings.EqualFold(k, "Connection") {
			continue
		}
		for _, v := range vv {
			c.Writer.Header().Add(k, v)
		}
	}
	contentType := res.Headers.Get("Content-Type")
	if contentType == "" {
		contentType = "application/json"
	}
	c.Data(res.StatusCode, contentType, res.Body)
}

func servesCompositeAntigravityGemini(apiKey *service.APIKey, effectivePlatform string) bool {
	return apiKey.Group != nil && apiKey.Group.Platform == service.PlatformComposite && effectivePlatform == service.PlatformAntigravity
}

func isAntigravityGeminiModel(model string) bool {
	trimmed := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(model, "models/")))
	if trimmed == "" || strings.HasPrefix(trimmed, "claude-") {
		return false
	}
	detected, ok := service.DetectModelPlatform(trimmed)
	return ok && detected == service.PlatformGemini
}
