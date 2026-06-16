package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/gateway"
	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ip"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// geminiV1BetaPipeline handles Gemini V1Beta generateContent /
// streamGenerateContent requests through the GatewayPipeline.
// This is the sole request path (legacy inline handler code has been removed).
//
// Gemini-specific concerns handled here:
//   - Model + action extraction from URL path (:modelAction param)
//   - CLI session hash (x-gemini-api-privileged-user-id + tmp dir hash)
//   - Digest-chain fallback session matching
//   - thoughtSignature cleaning on account switch
//   - Long-context double billing (200K threshold)
//   - Single-account retry context for Antigravity groups
func (h *GatewayHandler) geminiV1BetaPipeline(c *gin.Context) {
	if h.pipeline == nil {
		googleError(c, http.StatusInternalServerError, "Gateway pipeline not initialized")
		return
	}
	if h.errorPassthroughService != nil {
		service.BindErrorPassthroughService(c, h.errorPassthroughService)
	}

	apiKey, ok := middleware2.GetAPIKeyFromContext(c)
	if !ok || apiKey == nil {
		googleError(c, http.StatusUnauthorized, "Invalid API key")
		return
	}
	_, ok = middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		googleError(c, http.StatusInternalServerError, "User context not found")
		return
	}

	// 检查平台：优先使用强制平台（/antigravity 路由），否则要求 gemini 分组
	if !middleware2.HasForcePlatform(c) {
		if apiKey.Group == nil || apiKey.Group.Platform != service.PlatformGemini {
			googleError(c, http.StatusBadRequest, "API key group platform is not gemini")
			return
		}
	}

	// Parse model + action from URL path
	modelName, action, err := parseGeminiModelAction(strings.TrimPrefix(c.Param("modelAction"), "/"))
	if err != nil {
		googleError(c, http.StatusNotFound, err.Error())
		return
	}
	stream := action == "streamGenerateContent"

	// Single-account retry context for mixed-scheduling groups
	if h.gatewayService.IsSingleCompatibleAccountGroup(c.Request.Context(), apiKey.GroupID) {
		ctx := service.WithSingleAccountRetry(c.Request.Context(), true, h.metadataBridgeEnabled())
		c.Request = c.Request.WithContext(ctx)
	}

	// Share ForwardRequest pointer between parse and record closures so
	// record can read pipeline-mutated fields (e.g. ForceCacheBilling).
	var fwdReq *gateway.ForwardRequest
	parse := h.buildGeminiParseFunc(c, modelName, action, stream, &fwdReq)
	record := h.buildGeminiRecordFunc(c, &fwdReq)
	forcePlatform := h.resolveGeminiForcePlatform(c)

	err = h.pipeline.Execute(c, gateway.ProtocolGemini, forcePlatform, parse, record)
	if err != nil {
		h.handleGeminiPipelineError(c, err)
	}
}

// buildGeminiParseFunc builds the ParseRequestFunc for Gemini V1Beta.
// Handles Gemini-specific session logic: CLI session hash, digest chain
// fallback, and thoughtSignature cleaning on account switch.
func (h *GatewayHandler) buildGeminiParseFunc(
	c *gin.Context,
	modelName, action string,
	stream bool,
	fwdReqOut **gateway.ForwardRequest,
) gateway.ParseRequestFunc {
	return func(body []byte) (*gateway.ForwardRequest, error) {
		apiKey, _ := middleware2.GetAPIKeyFromContext(c)

		setOpsRequestContext(c, modelName, stream, body)
		setOpsEndpointContext(c, "", int16(service.RequestTypeFromLegacy(stream, false)))

		// --- Session hash computation ---
		sessionHash := extractGeminiCLISessionHash(c, body)
		if sessionHash == "" {
			parsedReq, _ := service.ParseGatewayRequest(service.NewRequestBodyRef(body), domain.PlatformGemini)
			if parsedReq != nil {
				parsedReq.SessionContext = &service.SessionContext{
					ClientIP:  ip.GetClientIP(c),
					UserAgent: c.GetHeader("User-Agent"),
					APIKeyID:  apiKey.ID,
				}
			}
			sessionHash = h.gatewayService.GenerateSessionHash(parsedReq)
		}
		if sessionHash != "" {
			sessionHash = "gemini:" + sessionHash
		}

		// --- Digest-chain fallback session matching ---
		sessionHash = h.resolveGeminiDigestSession(c, apiKey, body, modelName, sessionHash)

		// --- thoughtSignature cleaning ---
		// If no sticky session binding exists but body contains thoughtSignature,
		// clean it to avoid 400 errors from a new account.
		if sessionHash != "" {
			boundID, _ := h.gatewayService.GetCachedSessionAccountID(
				c.Request.Context(), apiKey.GroupID, sessionHash,
			)
			if boundID == 0 && bytes.Contains(body, []byte(`"thoughtSignature"`)) {
				body = service.CleanGeminiNativeThoughtSignatures(body)
			}
		}

		req := &gateway.ForwardRequest{
			Model:        modelName,
			Stream:       stream,
			RawBody:      body,
			GinContext:    c,
			SessionHash:  sessionHash,
			GeminiAction: action,
		}
		*fwdReqOut = req
		return req, nil
	}
}

// resolveGeminiDigestSession attempts digest-chain fallback session matching
// when the primary session key has no bound account. Returns the (possibly
// updated) session key.
func (h *GatewayHandler) resolveGeminiDigestSession(
	c *gin.Context,
	apiKey *service.APIKey,
	body []byte,
	modelName, sessionKey string,
) string {
	authSubject, _ := middleware2.GetAuthSubjectFromContext(c)

	// Check if primary session already has a binding
	if sessionKey != "" {
		boundID, _ := h.gatewayService.GetCachedSessionAccountID(
			c.Request.Context(), apiKey.GroupID, sessionKey,
		)
		if boundID > 0 {
			return sessionKey // primary session works, no fallback needed
		}
	}

	// Parse Gemini request for digest chain
	var geminiReq antigravity.GeminiRequest
	if err := json.Unmarshal(body, &geminiReq); err != nil || len(geminiReq.Contents) == 0 {
		return sessionKey
	}

	digestChain := service.BuildGeminiDigestChain(&geminiReq)
	if digestChain == "" {
		return sessionKey
	}

	platform := ""
	if apiKey.Group != nil {
		platform = apiKey.Group.Platform
	}
	prefixHash := service.GenerateGeminiPrefixHash(
		authSubject.UserID, apiKey.ID,
		ip.GetClientIP(c), c.GetHeader("User-Agent"),
		platform, modelName,
	)

	foundUUID, foundAccountID, matchedChain, found := h.gatewayService.FindGeminiSession(
		c.Request.Context(),
		derefGroupID(apiKey.GroupID),
		prefixHash, digestChain,
	)

	reqLog := requestLogger(c, "handler.gemini_v1beta.pipeline")

	if found {
		reqLog.Info("gemini.digest_fallback_matched",
			zap.String("session_uuid_prefix", safeShortPrefix(foundUUID, 8)),
			zap.Int64("account_id", foundAccountID),
			zap.String("digest_chain", truncateDigestChain(digestChain)),
		)
		if sessionKey == "" {
			sessionKey = service.GenerateGeminiDigestSessionKey(prefixHash, foundUUID)
		}
		_ = h.gatewayService.BindStickySession(c.Request.Context(), apiKey.GroupID, sessionKey, foundAccountID)
	} else {
		sessionUUID := uuid.New().String()
		if sessionKey == "" {
			sessionKey = service.GenerateGeminiDigestSessionKey(prefixHash, sessionUUID)
		}
	}

	// Store digest context for post-forward saving
	c.Set("gemini_digest_chain", digestChain)
	c.Set("gemini_prefix_hash", prefixHash)
	c.Set("gemini_matched_chain", matchedChain)
	if !found {
		c.Set("gemini_session_uuid", uuid.New().String())
	} else {
		c.Set("gemini_session_uuid", foundUUID)
	}

	return sessionKey
}

// buildGeminiRecordFunc builds the RecordUsageFunc for Gemini V1Beta.
// Uses RecordUsageWithLongContext for Gemini's 200K double-billing.
func (h *GatewayHandler) buildGeminiRecordFunc(c *gin.Context, fwdReq **gateway.ForwardRequest) gateway.RecordUsageFunc {
	return func(ctx context.Context, account *service.Account, result *gateway.ForwardResult) error {
		apiKey, _ := middleware2.GetAPIKeyFromContext(c)
		subscription, _ := middleware2.GetSubscriptionFromContext(c)

		userAgent := c.GetHeader("User-Agent")
		clientIP := ip.GetClientIP(c)
		inboundEndpoint := GetInboundEndpoint(c)
		upstreamEndpoint := GetUpstreamEndpoint(c, account.Platform)

		reqModel := result.Model
		body := []byte(nil)
		if raw, ok := c.Get("ops_request_body"); ok {
			if b, ok := raw.([]byte); ok {
				body = b
			}
		}
		requestPayloadHash := service.HashUsageRequestPayload(body)

		channelMapping := h.resolveChannelMappingFromContext(c, apiKey, reqModel)

		// Save Gemini digest session (for future fallback matching)
		h.saveGeminiDigestSession(c, apiKey, account.ID)

		svcResult := toServiceForwardResult(result)

		h.submitUsageRecordTask(func(recordCtx context.Context) {
			if err := h.gatewayService.RecordUsageWithLongContext(recordCtx, &service.RecordUsageLongContextInput{
				Result:                svcResult,
				APIKey:                apiKey,
				User:                  apiKey.User,
				Account:               account,
				Subscription:          subscription,
				InboundEndpoint:       inboundEndpoint,
				UpstreamEndpoint:      upstreamEndpoint,
				UserAgent:             userAgent,
				IPAddress:             clientIP,
				RequestPayloadHash:    requestPayloadHash,
				ForceCacheBilling:     *fwdReq != nil && (*fwdReq).ForceCacheBilling,
				LongContextThreshold:  200000, // Gemini 200K 阈值
				LongContextMultiplier: 2.0,    // 超出部分双倍计费
				APIKeyService:         h.apiKeyService,
				ChannelUsageFields:    channelMapping.ToUsageFields(reqModel, result.UpstreamModel),
			}); err != nil {
				requestLogger(c, "handler.gemini_v1beta.pipeline").
					Error("gemini.pipeline.record_usage_failed",
						zap.Int64("account_id", account.ID),
						zap.Error(err),
					)
			}
		})
		return nil
	}
}

// saveGeminiDigestSession saves the digest session data stored in gin.Context
// by the parse func (resolveGeminiDigestSession).
func (h *GatewayHandler) saveGeminiDigestSession(c *gin.Context, apiKey *service.APIKey, accountID int64) {
	digestChain, _ := c.Get("gemini_digest_chain")
	prefixHash, _ := c.Get("gemini_prefix_hash")
	sessionUUID, _ := c.Get("gemini_session_uuid")
	matchedChain, _ := c.Get("gemini_matched_chain")

	dc, _ := digestChain.(string)
	ph, _ := prefixHash.(string)
	su, _ := sessionUUID.(string)
	mc, _ := matchedChain.(string)

	if dc == "" || ph == "" {
		return
	}

	if err := h.gatewayService.SaveGeminiSession(
		c.Request.Context(),
		derefGroupID(apiKey.GroupID),
		ph, dc, su, accountID, mc,
	); err != nil {
		requestLogger(c, "handler.gemini_v1beta.pipeline").
			Warn("gemini.digest_session_save_failed",
				zap.Int64("account_id", accountID),
				zap.Error(err),
			)
	}
}

// resolveGeminiForcePlatform resolves the force-platform for Gemini V1Beta.
func (h *GatewayHandler) resolveGeminiForcePlatform(c *gin.Context) string {
	if fp, ok := middleware2.GetForcePlatformFromContext(c); ok {
		return fp
	}
	return ""
}

// handleGeminiPipelineError translates pipeline errors into Google API
// error format responses.
func (h *GatewayHandler) handleGeminiPipelineError(c *gin.Context, err error) {
	status, _, message, metadata := classifyGeminiPipelineErrorWithContext(c, err)

	// Gemini streaming uses JSON lines, not SSE; if bytes were already written
	// we cannot reliably inject a Google-format error, so we silently return.
	if c.Writer.Size() > 0 {
		return
	}

	if len(metadata) > 0 {
		googleErrorWithReason(c, status, "", message, metadata)
	} else {
		googleError(c, status, message)
	}
}

// classifyGeminiPipelineErrorWithContext inspects a pipeline error with
// context-aware error passthrough and upstream error mapping.
func classifyGeminiPipelineErrorWithContext(c *gin.Context, err error) (status int, code, message string, metadata map[string]string) {
	httpStatus, _, msg, md := classifyPipelineErrorWithContext(c, service.PlatformGemini, err)
	return httpStatus, "", msg, md
}
