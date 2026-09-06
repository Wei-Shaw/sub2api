package service

import (
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// opsCyberPolicyKey 在 gin context 中携带 cyber_policy 命中标记。
// 由 gateway 服务层在检测到上游 error.code=="cyber_policy" 时设置，
// handler 在 Forward 返回后读取以触发风控记录、邮件与 tokens=0 用量行。
const opsCyberPolicyKey = "ops_cyber_policy"

// Serialize mark replacement, never mutation: readers may retain a snapshot
// while an authoritative terminal event supplies the final usage.
var opsCyberPolicyMu sync.Mutex

// errOpenAICyberPolicyForwarded 表示 cyber_policy 已按当前端点格式透传给客户端
// （error 已写出/下发）。compat 路径 ForwardAsChatCompletions / ForwardAsAnthropic 出口
// 据此丢弃 result 并返回该哨兵，使 handler 落入 tokens=0 免费用量行（对齐 /v1/responses），
// 既不计费、也不 failover、不重复写响应。
var errOpenAICyberPolicyForwarded = errors.New("openai cyber_policy forwarded to client")

// CyberPolicyMark 记录一次 cyber_policy 硬阻断的上游证据。
type CyberPolicyMark struct {
	hitAt          time.Time
	Code           string // 固定 "cyber_policy"
	Message        string // 上游 error.message
	Body           string // 上游 response.failed / 400 原始 body（已截断；未脱敏，ops_error 落库由 sanitizeErrorBodyForStorage、风控日志由 redactContentModerationSecrets 统一脱敏）
	UpstreamStatus int    // 上游 HTTP 状态（流式=200，非流式=400）
	UpstreamInTok  int    // 上游已报 input tokens（如有）
	UpstreamOutTok int    // 上游已报 output tokens（如有）
}

// MarkOpsCyberPolicy preserves the first hit and its deadline, while later
// terminal events can supply additional usage through immutable snapshots.
// WS 多轮场景由 handler 在每个 turn 结束后调用 ClearOpsCyberPolicy 重置。
func MarkOpsCyberPolicy(c *gin.Context, mark CyberPolicyMark) {
	if c == nil {
		return
	}
	opsCyberPolicyMu.Lock()
	if previous := GetOpsCyberPolicy(c); previous != nil {
		updated := *previous
		updated.UpstreamInTok = max(updated.UpstreamInTok, mark.UpstreamInTok)
		updated.UpstreamOutTok = max(updated.UpstreamOutTok, mark.UpstreamOutTok)
		c.Set(opsCyberPolicyKey, &updated)
		opsCyberPolicyMu.Unlock()
		activateCyberSessionIdentity(c)
		return
	}
	mark.Code = "cyber_policy"
	mark.hitAt = time.Now()
	mark.Message = strings.TrimSpace(mark.Message)
	mark.Body = strings.TrimSpace(mark.Body)
	c.Set(opsCyberPolicyKey, &mark)
	opsCyberPolicyMu.Unlock()
	activateCyberSessionIdentity(c)
}

// GetOpsCyberPolicy 返回 cyber 标记，未命中（或已被 Clear）返回 nil。
func GetOpsCyberPolicy(c *gin.Context) *CyberPolicyMark {
	if c == nil {
		return nil
	}
	if v, ok := c.Get(opsCyberPolicyKey); ok {
		if m, ok := v.(*CyberPolicyMark); ok && m != nil {
			return m
		}
	}
	return nil
}

// ClearOpsCyberPolicy 清除 cyber 标记（typed-nil 覆盖；gin context 无并发安全的
// 删除原语，Set 走内部锁，与异步 GetOpsCyberPolicy 不构成 data race）。
// 仅 WS 多轮路径在 turn 收尾调用；HTTP 单请求路径不调用（context 随请求销毁，
// 且中间件 shouldSkipOpsErrorLogForCyber 依赖标记防双写）。
// WS 路径 clear 发生在中间件收尾之前，连接响应状态为 101，不触发中间件 status>=400
// 落库分支，故无双写/漏写。
func ClearOpsCyberPolicy(c *gin.Context) {
	if c == nil {
		return
	}
	opsCyberPolicyMu.Lock()
	c.Set(opsCyberPolicyKey, (*CyberPolicyMark)(nil))
	opsCyberPolicyMu.Unlock()
}

func openAICyberPolicyErrorBody(payload []byte) gin.H {
	fields := extractOpenAIResponsesErrorFields(payload, 0)
	errType := fields.Type
	if errType == "" {
		errType = "invalid_request_error"
	}
	message := sanitizeUpstreamErrorMessage(fields.Message)
	if message == "" {
		message = "Request blocked by upstream cyber-security policy"
	}
	return gin.H{"code": "cyber_policy", "type": errType, "message": message}
}

func buildOpenAICyberPolicyErrorEvent(payload []byte) []byte {
	body, _ := marshalOpenAIUpstreamJSON(gin.H{"type": "error", "status": http.StatusBadRequest, "error": openAICyberPolicyErrorBody(payload)})
	return body
}

func writeOpenAICyberPolicyHTTPError(c *gin.Context, payload []byte, status int, usage *OpenAIUsage) bool {
	if !markOpenAICyberPolicyEvent(c, payload, status, usage) {
		return false
	}
	errBody := openAICyberPolicyErrorBody(payload)
	message, _ := errBody["message"].(string)
	if openAICompactClientWantsStream(c) && StopOpenAICompactSSEKeepaliveCommitted(c) {
		writeOpenAICompactSSEFailureMessage(c, http.StatusBadRequest, "cyber_policy", message)
	} else {
		c.JSON(http.StatusBadRequest, gin.H{"error": errBody})
	}
	return true
}

// detectOpenAICyberPolicy 精确识别 cyber_policy（对齐 codex api_bridge.rs:145 /
// sse/responses.rs:529）。命中返回 (true, "cyber_policy", message)。
func detectOpenAICyberPolicy(payload []byte) (bool, string, string) {
	fields := extractOpenAIResponsesErrorFields(payload, 0)
	if !strings.EqualFold(strings.TrimSpace(fields.Code), "cyber_policy") {
		return false, "", ""
	}
	return true, "cyber_policy", strings.TrimSpace(fields.Message)
}

func markOpenAICyberPolicyEvent(c *gin.Context, payload []byte, upstreamStatus int, usage *OpenAIUsage) bool {
	hit, code, message := detectOpenAICyberPolicy(payload)
	if !hit {
		return false
	}
	mark := CyberPolicyMark{
		Code:           code,
		Message:        message,
		Body:           truncateString(string(payload), 4096),
		UpstreamStatus: upstreamStatus,
	}
	if usage != nil {
		mark.UpstreamInTok = usage.InputTokens
		mark.UpstreamOutTok = usage.OutputTokens
	}
	ObserveCyberSessionResponseID(c, extractOpenAIResponseIDFromJSONBytes(payload))
	MarkOpsCyberPolicy(c, mark)
	return true
}
