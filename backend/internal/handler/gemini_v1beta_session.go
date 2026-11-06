package handler

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ip"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"go.uber.org/zap"
)

// geminiCLITmpDirRegex 用于从 Gemini CLI 请求体中提取 tmp 目录的哈希值
// 匹配格式: /Users/xxx/.gemini/tmp/[64位十六进制哈希]
var geminiCLITmpDirRegex = regexp.MustCompile(`/\.gemini/tmp/([A-Fa-f0-9]{64})`)

type geminiStickySessionState struct {
	sessionKey            string
	sessionBoundAccountID int64
	useDigestFallback     bool
	geminiDigestChain     string
	geminiPrefixHash      string
	geminiSessionUUID     string
	matchedDigestChain    string
	geminiAnchorKey       string
}

func (h *GatewayHandler) attachPrefetchedStickySession(c *gin.Context, groupID *int64, accountID int64) {
	if accountID <= 0 {
		return
	}
	ctx := service.WithPrefetchedStickySession(c.Request.Context(), accountID, derefGroupID(groupID), h.metadataBridgeEnabled())
	c.Request = c.Request.WithContext(ctx)
}

func (h *GatewayHandler) resolveGeminiStickySession(
	c *gin.Context,
	reqLog *zap.Logger,
	userID int64,
	apiKey *service.APIKey,
	modelName string,
	body []byte,
	cachedContentBinding *service.GeminiCachedContent,
) geminiStickySessionState {
	var state geminiStickySessionState
	if cachedContentBinding != nil {
		return state
	}

	// 1. 优先使用显式会话标识（Gemini CLI tmp 目录哈希 / x-gemini-session-id / x-session-id / body.sessionId / body.session_id）
	sessionHash := extractGeminiExplicitSessionHash(c, apiKey.ID, body)
	if sessionHash == "" {
		if cliHash := extractGeminiCLISessionHash(c, body); cliHash != "" {
			// CLI paths and privileged-user headers are caller-controlled, not
			// authenticated identities. Scope them to the verified API key.
			sum := sha256.Sum256([]byte(strconv.FormatInt(apiKey.ID, 10) + ":" + cliHash))
			sessionHash = hex.EncodeToString(sum[:])
		}
	}
	hasExplicitSession := sessionHash != ""

	// 2. 无显式标识时，从解析的请求与客户端上下文生成初始粘性 key。
	if !hasExplicitSession {
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
		state.sessionKey = "gemini:" + sessionHash
	}

	if state.sessionKey != "" {
		state.sessionBoundAccountID, _ = h.gatewayService.GetCachedSessionAccountID(c.Request.Context(), apiKey.GroupID, state.sessionKey)
		if state.sessionBoundAccountID > 0 {
			h.attachPrefetchedStickySession(c, apiKey.GroupID, state.sessionBoundAccountID)
		}
	}

	// 3. 当未命中已有会话绑定时，启用 Gemini 内容摘要链 + Redis 首轮锚点双重粘性会话机制
	state.useDigestFallback = state.sessionBoundAccountID == 0
	if !state.useDigestFallback {
		return state
	}

	var geminiReq antigravity.GeminiRequest
	if err := json.Unmarshal(body, &geminiReq); err != nil || len(geminiReq.Contents) == 0 {
		return state
	}

	state.geminiDigestChain = service.BuildGeminiDigestChain(&geminiReq)
	if state.geminiDigestChain == "" {
		return state
	}

	userAgent := c.GetHeader("User-Agent")
	clientIP := ip.GetClientIP(c)
	platform := ""
	if apiKey.Group != nil {
		platform = apiKey.Group.Platform
	}
	state.geminiPrefixHash = service.GenerateGeminiPrefixHash(
		userID,
		apiKey.ID,
		clientIP,
		userAgent,
		platform,
		modelName,
	)
	state.geminiAnchorKey = service.BuildGeminiStickyAnchorKey(state.geminiPrefixHash, &geminiReq)

	foundUUID, foundAccountID, foundMatchedChain, found := h.gatewayService.FindGeminiSession(
		c.Request.Context(),
		derefGroupID(apiKey.GroupID),
		state.geminiPrefixHash,
		state.geminiDigestChain,
	)
	if found {
		state.matchedDigestChain = foundMatchedChain
		state.sessionBoundAccountID = foundAccountID
		state.geminiSessionUUID = foundUUID
		reqLog.Info("gemini.digest_fallback_matched",
			zap.String("session_uuid_prefix", safeShortPrefix(foundUUID, 8)),
			zap.Int64("account_id", foundAccountID),
			zap.String("digest_chain", truncateDigestChain(state.geminiDigestChain)),
		)
		// 内容增长时沿用匹配到的摘要会话，调度仍会检查该账号当前的资格。
		if !hasExplicitSession || state.sessionKey == "" {
			state.sessionKey = service.GenerateGeminiDigestSessionKey(state.geminiPrefixHash, foundUUID)
		}
		_ = h.gatewayService.BindStickySession(c.Request.Context(), apiKey.GroupID, state.sessionKey, foundAccountID)
		h.attachPrefetchedStickySession(c, apiKey.GroupID, foundAccountID)
		return state
	}

	// 摘要链未命中（例如超过 5 分钟内存 TTL 或进程重启）时，尝试通过 Redis 首轮锚点恢复粘性账号
	if state.geminiAnchorKey != "" {
		if anchorAccountID, _ := h.gatewayService.GetCachedSessionAccountID(c.Request.Context(), apiKey.GroupID, state.geminiAnchorKey); anchorAccountID > 0 {
			state.sessionBoundAccountID = anchorAccountID
			state.geminiSessionUUID = service.DeriveGeminiAnchorSessionUUID(state.geminiAnchorKey)
			if !hasExplicitSession || state.sessionKey == "" {
				state.sessionKey = service.GenerateGeminiDigestSessionKey(state.geminiPrefixHash, state.geminiSessionUUID)
			}
			_ = h.gatewayService.BindStickySession(c.Request.Context(), apiKey.GroupID, state.sessionKey, anchorAccountID)
			h.attachPrefetchedStickySession(c, apiKey.GroupID, anchorAccountID)
			reqLog.Info("gemini.anchor_fallback_matched",
				zap.Int64("account_id", anchorAccountID),
				zap.String("anchor_key", state.geminiAnchorKey),
			)
			return state
		}
	}

	// 从同一个锚点派生首轮和恢复后的标识，避免内存过期或重启改变上游 sessionId。
	state.geminiSessionUUID = service.DeriveGeminiAnchorSessionUUID(state.geminiAnchorKey)
	if state.geminiSessionUUID == "" {
		state.geminiSessionUUID = service.DeriveGeminiAnchorSessionUUID(state.geminiPrefixHash + ":" + state.geminiDigestChain)
	}
	if !hasExplicitSession || state.sessionKey == "" {
		state.sessionKey = service.GenerateGeminiDigestSessionKey(state.geminiPrefixHash, state.geminiSessionUUID)
	}
	return state
}

func (h *GatewayHandler) persistGeminiStickySession(
	c *gin.Context,
	reqLog *zap.Logger,
	groupID *int64,
	accountID int64,
	state *geminiStickySessionState,
) {
	if state == nil || accountID <= 0 {
		return
	}
	if state.useDigestFallback && state.geminiDigestChain != "" && state.geminiPrefixHash != "" {
		if err := h.gatewayService.SaveGeminiSession(
			c.Request.Context(),
			derefGroupID(groupID),
			state.geminiPrefixHash,
			state.geminiDigestChain,
			state.geminiSessionUUID,
			accountID,
			state.matchedDigestChain,
		); err != nil {
			reqLog.Warn("gemini.digest_session_save_failed", zap.Int64("account_id", accountID), zap.Error(err))
		}
	}
	if state.geminiAnchorKey != "" {
		_ = h.gatewayService.BindStickySession(c.Request.Context(), groupID, state.geminiAnchorKey, accountID)
	}
}

func extractGeminiExplicitSessionHash(c *gin.Context, apiKeyID int64, body []byte) string {
	var explicitID string
	if c != nil {
		explicitID = strings.TrimSpace(c.GetHeader("x-gemini-session-id"))
		if explicitID == "" {
			explicitID = strings.TrimSpace(service.ExtractClientSessionID(c))
		}
	}
	if explicitID == "" && len(body) > 0 {
		explicitID = strings.TrimSpace(gjson.GetBytes(body, "sessionId").String())
		if explicitID == "" {
			explicitID = strings.TrimSpace(gjson.GetBytes(body, "session_id").String())
		}
	}
	if explicitID == "" {
		return ""
	}
	combined := strconv.FormatInt(apiKeyID, 10) + ":" + explicitID
	sum := sha256.Sum256([]byte(combined))
	return hex.EncodeToString(sum[:])
}

// extractGeminiCLISessionHash 从 Gemini CLI 请求中提取会话标识。
// 组合 x-gemini-api-privileged-user-id header 和请求体中的 tmp 目录哈希。
func extractGeminiCLISessionHash(c *gin.Context, body []byte) string {
	match := geminiCLITmpDirRegex.FindSubmatch(body)
	if len(match) < 2 {
		return ""
	}
	tmpDirHash := string(match[1])
	privilegedUserID := strings.TrimSpace(c.GetHeader("x-gemini-api-privileged-user-id"))
	if privilegedUserID != "" {
		combined := privilegedUserID + ":" + tmpDirHash
		hash := sha256.Sum256([]byte(combined))
		return hex.EncodeToString(hash[:])
	}
	return tmpDirHash
}

// truncateDigestChain 截断摘要链用于日志显示
func truncateDigestChain(chain string) string {
	if len(chain) <= 50 {
		return chain
	}
	return chain[:50] + "..."
}

// safeShortPrefix 返回字符串前 n 个字符；长度不足时返回原字符串。
func safeShortPrefix(value string, n int) string {
	if n <= 0 || len(value) <= n {
		return value
	}
	return value[:n]
}

// derefGroupID 安全解引用 *int64，nil 返回 0
func derefGroupID(groupID *int64) int64 {
	if groupID == nil {
		return 0
	}
	return *groupID
}
