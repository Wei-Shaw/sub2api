package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

// openAICodexTurnStateHeader 是 Codex 的回合状态头。上游在响应头中铸造该
// 不透明 blob，客户端在同一回合的后续请求中原样回带（codex-rs 侧从
// /responses SSE、/responses/compact JSON 与 WS 握手三种响应中捕获，见
// codex-api/src/sse/responses.rs 与 endpoint/compact.rs）。
const openAICodexTurnStateHeader = "x-codex-turn-state"

const (
	openAICodexTurnStateBindingPrefix = "openai:codex:turn-state:v2:"
	openAICodexTurnStateCacheTimeout  = 500 * time.Millisecond
)

// turn-state blob 是上游在"出站身份"下铸造的。每个 exact opaque state
// 单独绑定铸造账号，避免同一会话后续响应覆盖旧 state 的来源信息。state
// 明文本身不持久化；内存和 Redis 都只使用作用域化摘要键。
type openAICodexTurnStateOrigin struct {
	accountID int64
	expiresAt time.Time
}

func openAICodexTurnStateBindingKey(c *gin.Context, state string) string {
	seed := openAICodexTurnStateSeed(c)
	if seed == "" || state == "" {
		return ""
	}
	digest := sha256.Sum256([]byte(seed + "\x00" + state))
	return openAICodexTurnStateBindingPrefix + hex.EncodeToString(digest[:])
}

// openAICodexTurnStateSeed 返回溯源表键：API Key + 客户端原始会话标识。
// 客户端会话标识取自请求头（与指纹收敛的 thread 派生同源，见
// extractClientSessionID），确保同一下游会话的记录/守卫两侧使用同一键。
// 无会话标识时返回空串，表示不做跟踪（保持透传现状）。
func openAICodexTurnStateSeed(c *gin.Context) string {
	if c == nil || c.Request == nil {
		return ""
	}
	sessionID := extractClientSessionID(c.Request.Header)
	if sessionID == "" {
		return ""
	}
	apiKeyID := getAPIKeyIDFromContext(c)
	if apiKeyID <= 0 {
		return ""
	}
	return strconv.FormatInt(apiKeyID, 10) + "\x00" + sessionID
}

// relayOpenAICodexTurnState 将上游响应中的 turn-state 显式写入下游响应头，
// 并记录铸造账号。必须在响应头提交点调用（WriteHeader 之前、且确认本次
// 上游响应就是将要写回客户端的响应之后）。上游无该头时主动清除 writer 上
// 可能残留的上一 failover attempt 的值——否则换号后旧账号的 blob 会粘到
// 新账号的响应上，这正是本文件要防止的跨账号矛盾。
func (s *OpenAIGatewayService) relayOpenAICodexTurnState(c *gin.Context, account *Account, upstream http.Header) {
	if c == nil || c.Writer == nil {
		return
	}
	canonical := http.CanonicalHeaderKey(openAICodexTurnStateHeader)
	state := extractOpenAICodexTurnState(upstream)
	if state == "" {
		c.Writer.Header().Del(canonical)
		return
	}
	c.Writer.Header().Set(canonical, state)
	s.noteOpenAICodexTurnStateProvenance(c, account, state)
}

// stageOpenAICodexTurnState 将上游 turn-state 暂存到延迟提交的响应头集合
// （首输出守卫路径先缓存头、见到首个输出事件才提交）。此处**不**记录铸造
// 账号：该 attempt 仍可能在首输出超时后 failover，暂存头会被整体丢弃，
// 客户端从未收到该 blob。溯源必须在真正提交时记录，见
// noteStagedOpenAICodexTurnStateCommitted。
func stageOpenAICodexTurnState(dst *http.Header, upstream http.Header) {
	if dst == nil {
		return
	}
	canonical := http.CanonicalHeaderKey(openAICodexTurnStateHeader)
	state := extractOpenAICodexTurnState(upstream)
	if state == "" {
		if *dst != nil {
			dst.Del(canonical)
		}
		return
	}
	if *dst == nil {
		*dst = http.Header{}
	}
	dst.Set(canonical, state)
}

// noteStagedOpenAICodexTurnStateCommitted 在暂存响应头真正写入下游时记录
// 铸造账号——只有此刻客户端才确定收到了该 blob，溯源表才与客户端持有的
// 值一致（否则被 failover 丢弃的 attempt 会污染溯源，导致后续误剥离）。
func (s *OpenAIGatewayService) noteStagedOpenAICodexTurnStateCommitted(c *gin.Context, account *Account, staged http.Header) {
	if staged == nil {
		return
	}
	state := staged.Get(openAICodexTurnStateHeader)
	if state == "" {
		return
	}
	s.noteOpenAICodexTurnStateProvenance(c, account, state)
}

func extractOpenAICodexTurnState(upstream http.Header) string {
	if upstream == nil {
		return ""
	}
	return upstream.Get(openAICodexTurnStateHeader)
}

// noteOpenAICodexTurnStateProvenance records one exact committed state.
func (s *OpenAIGatewayService) noteOpenAICodexTurnStateProvenance(c *gin.Context, account *Account, state string) {
	if s == nil || account == nil || !account.IsOpenAIOAuth() || account.ID <= 0 {
		return
	}
	bindingKey := openAICodexTurnStateBindingKey(c, state)
	if bindingKey == "" {
		return
	}
	ttl := s.openAIWSSessionStickyTTL()
	s.openaiCodexTurnStateOrigins.Store(bindingKey, openAICodexTurnStateOrigin{
		accountID: account.ID,
		expiresAt: time.Now().Add(ttl),
	})
	s.sweepOpenAICodexTurnStateOrigins()

	if s.cache == nil {
		return
	}
	cacheCtx, cancel := context.WithTimeout(context.Background(), openAICodexTurnStateCacheTimeout)
	defer cancel()
	_ = s.cache.SetSessionAccountID(cacheCtx, getOpenAIGroupIDFromContext(c), bindingKey, account.ID, ttl)
}

// guardOpenAICodexTurnStateEcho 出站守卫：客户端回带的 turn-state 若已知由
// 其他账号铸造则剥离，同账号或无溯源记录时保持原样。只剥离、不注入——
// /responses 路径的客户端是真实 Codex，会按自身回合语义自行回带；服务端
// 注入是 Claude 兼容桥（无法回带的客户端）的专属行为。
func (s *OpenAIGatewayService) guardOpenAICodexTurnStateEcho(c *gin.Context, account *Account, h http.Header) {
	if s == nil || h == nil || account == nil || !account.IsOpenAIOAuth() {
		return
	}
	state := h.Get(openAICodexTurnStateHeader)
	if state == "" {
		return
	}
	bindingKey := openAICodexTurnStateBindingKey(c, state)
	if bindingKey == "" {
		return
	}
	origin, known := s.loadOpenAICodexTurnStateOrigin(c, bindingKey)
	if !known {
		return
	}
	if origin.accountID != account.ID {
		h.Del(openAICodexTurnStateHeader)
	}
}

func (s *OpenAIGatewayService) loadOpenAICodexTurnStateOrigin(c *gin.Context, bindingKey string) (openAICodexTurnStateOrigin, bool) {
	if raw, ok := s.openaiCodexTurnStateOrigins.Load(bindingKey); ok {
		origin, valid := raw.(openAICodexTurnStateOrigin)
		if !valid {
			s.openaiCodexTurnStateOrigins.Delete(bindingKey)
		} else if origin.expiresAt.IsZero() || time.Now().Before(origin.expiresAt) {
			return origin, true
		} else {
			s.openaiCodexTurnStateOrigins.Delete(bindingKey)
		}
	}
	if s.cache == nil {
		return openAICodexTurnStateOrigin{}, false
	}
	cacheCtx, cancel := context.WithTimeout(context.Background(), openAICodexTurnStateCacheTimeout)
	defer cancel()
	accountID, err := s.cache.GetSessionAccountID(cacheCtx, getOpenAIGroupIDFromContext(c), bindingKey)
	if err != nil || accountID <= 0 {
		return openAICodexTurnStateOrigin{}, false
	}
	origin := openAICodexTurnStateOrigin{
		accountID: accountID,
		expiresAt: time.Now().Add(s.openAIWSSessionStickyTTL()),
	}
	s.openaiCodexTurnStateOrigins.Store(bindingKey, origin)
	return origin, true
}

// sweepOpenAICodexTurnStateOrigins 机会式清扫过期溯源记录：每 256 次写入
// 全量遍历一轮，防止仅靠读侧惰性删除导致的慢泄漏（会话键无上界）。
func (s *OpenAIGatewayService) sweepOpenAICodexTurnStateOrigins() {
	if s.openaiCodexTurnStateWrites.Add(1)%256 != 0 {
		return
	}
	now := time.Now()
	s.openaiCodexTurnStateOrigins.Range(func(key, value any) bool {
		origin, ok := value.(openAICodexTurnStateOrigin)
		if !ok || (!origin.expiresAt.IsZero() && now.After(origin.expiresAt)) {
			s.openaiCodexTurnStateOrigins.Delete(key)
		}
		return true
	})
}
