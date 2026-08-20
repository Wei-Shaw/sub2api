package handler

import (
	"context"
	"crypto/sha256"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/securityaudit"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

const securityAuditCompletedContextKey = "sub2api.security_audit.completed"
const securityAuditWSTurnContextKey = "sub2api.security_audit.ws_turn"
const securityAuditWSDedupeContextKey = "sub2api.security_audit.ws_dedupe"
const securityAuditFallbackUsedContextKey = "sub2api.security_audit.prompt_fallback_used"

type securityAuditWSDedupeEntry struct {
	stage    string
	turn     int
	bodyHash [sha256.Size]byte
	decision securityaudit.Decision
}

type securityAuditGroupResolver interface {
	ResolveGroupByID(context.Context, int64) (*service.Group, error)
}

type securityAuditFallbackAvailability interface {
	HasPromptAuditFallbackAccounts(context.Context, *service.Group, string, string) (bool, error)
}

func promptAuditFallbackUsed(c *gin.Context) bool {
	if c == nil {
		return false
	}
	used, _ := c.Get(securityAuditFallbackUsedContextKey)
	return used == true
}

// cachesSecurityAuditCompletion reports whether a successful audit may be
// reused for the rest of the gin request. WebSocket turns share one Context
// across many response.create frames and must be audited independently.
func cachesSecurityAuditCompletion(stage string) bool {
	switch strings.TrimSpace(stage) {
	case "", "http":
		return true
	default:
		return false
	}
}

func isSecurityAuditWebSocketStage(stage string) bool {
	switch strings.TrimSpace(stage) {
	case "first_turn", "subsequent_turn":
		return true
	default:
		return false
	}
}

func (h *GatewayHandler) checkSecurityAudit(c *gin.Context, reqLog *zap.Logger, apiKey *service.APIKey, subject middleware2.AuthSubject, protocol, model string, body []byte) *securityaudit.Decision {
	if h == nil {
		return nil
	}
	return runSecurityAudit(c, reqLog, h.securityAuditCoordinator, h.contentModerationService, apiKey, subject, protocol, model, body, "http", h.gatewayService)
}

func (h *OpenAIGatewayHandler) checkSecurityAudit(c *gin.Context, reqLog *zap.Logger, apiKey *service.APIKey, subject middleware2.AuthSubject, protocol, model string, body []byte) *securityaudit.Decision {
	if h == nil {
		return nil
	}
	return runSecurityAudit(c, reqLog, h.securityAuditCoordinator, h.contentModerationService, apiKey, subject, protocol, model, body, "http", h.gatewayService)
}

func (h *OpenAIGatewayHandler) checkSecurityAuditStage(c *gin.Context, reqLog *zap.Logger, apiKey *service.APIKey, subject middleware2.AuthSubject, protocol, model string, body []byte, stage string) *securityaudit.Decision {
	if h == nil {
		return nil
	}
	return runSecurityAudit(c, reqLog, h.securityAuditCoordinator, h.contentModerationService, apiKey, subject, protocol, model, body, stage, h.gatewayService)
}

func runSecurityAudit(c *gin.Context, reqLog *zap.Logger, coordinator *securityaudit.Coordinator, legacy *service.ContentModerationService, apiKey *service.APIKey, subject middleware2.AuthSubject, protocol, model string, body []byte, stage string, resolvers ...securityAuditGroupResolver) *securityaudit.Decision {
	if c == nil || c.Request == nil {
		return nil
	}
	cacheCompletion := cachesSecurityAuditCompletion(stage)
	if cacheCompletion {
		if completed, exists := c.Get(securityAuditCompletedContextKey); exists && completed == true {
			return nil
		}
	}
	if coordinator == nil {
		legacyDecision := runContentModeration(c, reqLog, legacy, apiKey, subject, protocol, model, body)
		if legacyDecision == nil {
			return nil
		}
		decision := securityaudit.Decision{Kind: securityaudit.DecisionAllow, HTTPStatus: http.StatusOK, AllowNextStage: true}
		decision.Legacy = &securityaudit.LegacyDecision{
			Allowed: legacyDecision.Allowed, Blocked: legacyDecision.Blocked, Flagged: legacyDecision.Flagged,
			Message: legacyDecision.Message, StatusCode: legacyDecision.StatusCode,
			ErrorCode: "content_policy_violation", Action: legacyDecision.Action,
		}
		if legacyDecision.Blocked {
			decision.Kind, decision.HTTPStatus, decision.ErrorCode, decision.ClientMessage, decision.AllowNextStage = securityaudit.DecisionBlock, contentModerationStatus(legacyDecision), "content_policy_violation", legacyDecision.Message, false
		}
		if len(resolvers) > 0 {
			tryPromptAuditFallback(c, reqLog, apiKey, protocol, model, stage, contentModerationRequestID(c.Request.Context()), &decision, resolvers[0])
		}
		if decision.AllowNextStage && cacheCompletion {
			c.Set(securityAuditCompletedContextKey, true)
		}
		return &decision
	}
	request := buildSecurityAuditRequest(c, apiKey, subject, protocol, model, body, stage)
	if isSecurityAuditWebSocketStage(request.Stage) {
		if turnNo, ok := securityAuditWSTurn(c); ok {
			bodyHash := sha256.Sum256(body)
			if cached, exists := c.Get(securityAuditWSDedupeContextKey); exists {
				if entry, ok := cached.(securityAuditWSDedupeEntry); ok &&
					entry.stage == request.Stage && entry.turn == turnNo && entry.bodyHash == bodyHash {
					decision := entry.decision
					logSecurityAuditDone(reqLog, request, decision, true)
					return &decision
				}
			}
			logSecurityAuditStart(reqLog, request, len(body), false)
			decision := coordinator.Check(c.Request.Context(), request)
			if decision.Kind == securityaudit.DecisionAllow {
				c.Set(securityAuditWSDedupeContextKey, securityAuditWSDedupeEntry{
					stage: request.Stage, turn: turnNo, bodyHash: bodyHash, decision: decision,
				})
			}
			if len(resolvers) > 0 {
				tryPromptAuditFallback(c, reqLog, apiKey, protocol, model, stage, request.RequestID, &decision, resolvers[0])
			}
			logSecurityAuditDone(reqLog, request, decision, false)
			return &decision
		}
	}
	logSecurityAuditStart(reqLog, request, len(body), false)
	decision := coordinator.Check(c.Request.Context(), request)
	if len(resolvers) > 0 {
		tryPromptAuditFallback(c, reqLog, apiKey, protocol, model, stage, request.RequestID, &decision, resolvers[0])
	}
	if decision.AllowNextStage && cacheCompletion {
		c.Set(securityAuditCompletedContextKey, true)
	}
	logSecurityAuditDone(reqLog, request, decision, false)
	return &decision
}

func logSecurityAuditStart(reqLog *zap.Logger, request securityaudit.Request, bodyBytes int, cached bool) {
	if reqLog == nil {
		return
	}
	reqLog.Info("security_audit.gateway_check_start",
		zap.String("request_id", request.RequestID), zap.Int64("user_id", request.UserID),
		zap.Int64("api_key_id", request.APIKeyID), zap.Int64p("group_id", request.GroupID),
		zap.String("endpoint", request.Endpoint), zap.String("provider", request.Provider),
		zap.String("protocol", request.Protocol), zap.String("model", request.Model), zap.String("stage", request.Stage),
		zap.Int("body_bytes", bodyBytes), zap.Bool("cached", cached))
}

func logSecurityAuditDone(reqLog *zap.Logger, request securityaudit.Request, decision securityaudit.Decision, cached bool) {
	if reqLog == nil {
		return
	}
	reqLog.Info("security_audit.gateway_check_done",
		zap.String("request_id", request.RequestID), zap.String("decision", string(decision.Kind)),
		zap.String("error_code", decision.ErrorCode), zap.Bool("allow_next_stage", decision.AllowNextStage),
		zap.String("stage", request.Stage), zap.Bool("cached", cached))
}

func tryPromptAuditFallback(c *gin.Context, reqLog *zap.Logger, apiKey *service.APIKey, protocol, model, stage, requestID string, decision *securityaudit.Decision, resolver securityAuditGroupResolver) bool {
	if c == nil || c.Request == nil || apiKey == nil || apiKey.Group == nil || decision == nil || resolver == nil {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(stage), "subsequent_turn") || decision.Kind != securityaudit.DecisionBlock {
		return false
	}
	promptBlocked := decision.Prompt != nil && decision.Prompt.Kind == securityaudit.DecisionBlock
	if !promptBlocked {
		return false
	}
	if decision.Legacy != nil && decision.Legacy.Blocked {
		return false
	}
	if promptAuditFallbackUsed(c) {
		return false
	}
	fallbackID := apiKey.Group.FallbackGroupIDOnPromptAuditBlock
	if fallbackID == nil || *fallbackID <= 0 || *fallbackID == apiKey.Group.ID {
		return false
	}

	sourceGroupID := apiKey.Group.ID
	targetGroup, err := resolver.ResolveGroupByID(c.Request.Context(), *fallbackID)
	if err != nil || targetGroup == nil {
		logPromptAuditFallbackUnavailable(reqLog, requestID, sourceGroupID, *fallbackID, "target_group_unavailable", err)
		return false
	}
	if !targetGroup.IsActive() {
		logPromptAuditFallbackUnavailable(reqLog, requestID, sourceGroupID, targetGroup.ID, "target_group_inactive", nil)
		return false
	}
	if targetGroup.IsSubscriptionType() {
		logPromptAuditFallbackUnavailable(reqLog, requestID, sourceGroupID, targetGroup.ID, "target_group_subscription", nil)
		return false
	}
	if targetGroup.IsExclusive && (apiKey.User == nil || !apiKey.User.CanBindGroup(targetGroup.ID, true)) {
		logPromptAuditFallbackUnavailable(reqLog, requestID, sourceGroupID, targetGroup.ID, "target_group_not_allowed", nil)
		return false
	}
	if !promptAuditFallbackPlatformCompatible(c.Request.Context(), apiKey.Group, targetGroup, protocol, model) {
		logPromptAuditFallbackUnavailable(reqLog, requestID, sourceGroupID, targetGroup.ID, "target_group_protocol_incompatible", nil)
		return false
	}
	availability, ok := resolver.(securityAuditFallbackAvailability)
	if !ok {
		logPromptAuditFallbackUnavailable(reqLog, requestID, sourceGroupID, targetGroup.ID, "target_group_availability_unknown", nil)
		return false
	}
	hasAccounts, availabilityErr := availability.HasPromptAuditFallbackAccounts(c.Request.Context(), targetGroup, protocol, model)
	if availabilityErr != nil || !hasAccounts {
		logPromptAuditFallbackUnavailable(reqLog, requestID, sourceGroupID, targetGroup.ID, "target_group_no_available_account", availabilityErr)
		return false
	}
	if targetGroup.Platform == service.PlatformComposite {
		if _, resolved := service.ResolvedTargetPlatformFromContext(c.Request.Context()); !resolved {
			if _, detected := service.DetectModelPlatform(model); !detected {
				logPromptAuditFallbackUnavailable(reqLog, requestID, sourceGroupID, targetGroup.ID, "target_group_platform_unresolved", nil)
				return false
			}
		}
	}

	fallbackAPIKey := cloneAPIKeyWithGroup(apiKey, targetGroup)
	*apiKey = *fallbackAPIKey
	c.Set(string(middleware2.ContextKeyAPIKey), apiKey)
	requestContext := c.Request.Context()
	requestContext = context.WithValue(requestContext, ctxkey.Group, targetGroup)
	requestContext = context.WithValue(requestContext, ctxkey.ForcePlatform, "")
	requestContext = context.WithValue(requestContext, ctxkey.ResolvedTargetPlatform, "")
	requestContext = context.WithValue(requestContext, ctxkey.ResolvedUpstreamModel, "")
	requestContext = context.WithValue(requestContext, ctxkey.RequestedPublicModel, "")
	requestContext = context.WithValue(requestContext, ctxkey.CompositeRouteSource, "")
	requestContext = service.WithPrefetchedStickySession(requestContext, 0, 0, true)
	requestContext = service.WithSingleAccountRetry(requestContext, false, true)
	c.Request = c.Request.WithContext(requestContext)
	if c.Keys != nil {
		delete(c.Keys, string(middleware2.ContextKeyForcePlatform))
	}
	c.Set(string(middleware2.ContextKeySubscription), nil)
	if targetGroup.Platform == service.PlatformComposite {
		ensureCompositeTargetPlatform(c, apiKey, model)
	} else {
		c.Request = c.Request.WithContext(service.WithResolvedTargetPlatform(c.Request.Context(), targetGroup.Platform))
	}
	if resolvedPlatform, ok := service.ResolvedTargetPlatformFromContext(c.Request.Context()); ok && resolvedPlatform == service.PlatformAntigravity {
		c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), ctxkey.ForcePlatform, resolvedPlatform))
		c.Set(string(middleware2.ContextKeyForcePlatform), resolvedPlatform)
	}
	c.Set(securityAuditFallbackUsedContextKey, true)
	decision.Kind = securityaudit.DecisionAllow
	decision.HTTPStatus = http.StatusOK
	decision.ErrorCode = ""
	decision.ClientMessage = ""
	decision.AllowNextStage = true
	if reqLog != nil {
		reqLog.Info("security_audit.prompt_block_fallback",
			zap.String("request_id", requestID),
			zap.Int64("source_group_id", sourceGroupID),
			zap.Int64("target_group_id", targetGroup.ID),
			zap.Bool("fallback_used", true),
		)
	}
	return true
}

func logPromptAuditFallbackUnavailable(reqLog *zap.Logger, requestID string, sourceGroupID, targetGroupID int64, reason string, err error) {
	if reqLog == nil {
		return
	}
	fields := []zap.Field{
		zap.String("request_id", requestID),
		zap.Int64("source_group_id", sourceGroupID),
		zap.Int64("target_group_id", targetGroupID),
		zap.String("reason", reason),
		zap.Bool("fallback_used", false),
	}
	if err != nil {
		fields = append(fields, zap.Error(err))
	}
	reqLog.Info("security_audit.prompt_block_fallback_unavailable", fields...)
}

func promptAuditFallbackPlatformCompatible(ctx context.Context, sourceGroup, targetGroup *service.Group, protocol, model string) bool {
	if sourceGroup == nil || targetGroup == nil {
		return false
	}
	if targetGroup.Platform == service.PlatformComposite {
		return true
	}

	sourcePlatform := strings.TrimSpace(sourceGroup.Platform)
	if sourcePlatform == service.PlatformComposite {
		if resolved, ok := service.ResolvedTargetPlatformFromContext(ctx); ok {
			sourcePlatform = resolved
		} else if detected, ok := service.DetectModelPlatform(model); ok {
			sourcePlatform = detected
		}
	}
	targetPlatform := strings.TrimSpace(targetGroup.Platform)
	if sourcePlatform == "" || targetPlatform == "" {
		return false
	}

	switch protocol {
	case service.ContentModerationProtocolGemini:
		return targetPlatform == service.PlatformGemini || targetPlatform == service.PlatformAntigravity
	case "openai_embeddings", "openai_alpha_search", "openai_live":
		return targetPlatform == service.PlatformOpenAI
	case "grok_media", "grok_web_search", "grok_audio":
		return targetPlatform == service.PlatformGrok
	case service.ContentModerationProtocolOpenAIImages:
		switch sourcePlatform {
		case service.PlatformGemini, service.PlatformAntigravity:
			return targetPlatform == service.PlatformGemini || targetPlatform == service.PlatformAntigravity
		case service.PlatformGrok:
			return targetPlatform == service.PlatformGrok
		default:
			return targetPlatform == service.PlatformOpenAI
		}
	}

	if service.IsCNProvider(sourcePlatform) {
		return targetPlatform == sourcePlatform
	}
	switch sourcePlatform {
	case service.PlatformOpenAI, service.PlatformGrok:
		return targetPlatform == service.PlatformOpenAI || targetPlatform == service.PlatformGrok
	case service.PlatformGemini:
		return targetPlatform == service.PlatformGemini || targetPlatform == service.PlatformAntigravity
	case service.PlatformAnthropic, service.PlatformAntigravity:
		return targetPlatform == service.PlatformAnthropic || targetPlatform == service.PlatformAntigravity
	default:
		return false
	}
}

func securityAuditWSTurn(c *gin.Context) (int, bool) {
	turn, exists := c.Get(securityAuditWSTurnContextKey)
	if !exists {
		return 0, false
	}
	turnNo, ok := turn.(int)
	return turnNo, ok
}

func buildSecurityAuditRequest(c *gin.Context, apiKey *service.APIKey, subject middleware2.AuthSubject, protocol, model string, body []byte, stage string) securityaudit.Request {
	legacy := buildContentModerationInput(c, apiKey, subject, protocol, model, body)
	request := securityaudit.Request{
		RequestID: legacy.RequestID, UserID: legacy.UserID, UserEmail: legacy.UserEmail,
		APIKeyID: legacy.APIKeyID, APIKeyName: legacy.APIKeyName, GroupID: cloneSecurityAuditGroupID(legacy.GroupID),
		GroupName: legacy.GroupName, Provider: legacy.Provider, Endpoint: legacy.Endpoint,
		Protocol: legacy.Protocol, Model: legacy.Model, Body: body, Stage: strings.TrimSpace(stage),
	}
	if apiKey != nil && apiKey.User != nil {
		request.Username = apiKey.User.Username
		if request.UserEmail == "" {
			request.UserEmail = apiKey.User.Email
		}
	}
	if request.Stage == "" {
		request.Stage = "http"
	}
	return request
}

func securityAuditStatus(decision *securityaudit.Decision) int {
	if decision == nil || decision.HTTPStatus < 400 || decision.HTTPStatus > 599 {
		return http.StatusForbidden
	}
	return decision.HTTPStatus
}

func securityAuditErrorCode(decision *securityaudit.Decision) string {
	if decision == nil || strings.TrimSpace(decision.ErrorCode) == "" {
		return "content_policy_violation"
	}
	return decision.ErrorCode
}

func securityAuditMessage(decision *securityaudit.Decision) string {
	if decision == nil {
		return "Request blocked by content policy"
	}
	if decision.Legacy != nil && decision.Legacy.Blocked && strings.TrimSpace(decision.Legacy.Message) != "" {
		return decision.Legacy.Message
	}
	if strings.TrimSpace(decision.ClientMessage) != "" {
		return decision.ClientMessage
	}
	return "Request blocked by content policy"
}

func cloneSecurityAuditGroupID(value *int64) *int64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
