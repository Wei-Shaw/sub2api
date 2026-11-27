package service

import (
	"errors"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

// opsCyberPolicyKey 在 gin context 中携带 upstream prompt-policy 命中标记。
// 由 gateway 服务层在检测到上游策略拦截（cyber_policy / invalid_prompt）时设置，
// handler 在 Forward 返回后读取以触发风控记录、邮件与 tokens=0 用量行。
const opsCyberPolicyKey = "ops_cyber_policy"

// errOpenAICyberPolicyForwarded 表示策略拦截已按当前端点格式透传给客户端
// （error 已写出/下发）。compat 路径 ForwardAsChatCompletions / ForwardAsAnthropic 出口
// 据此丢弃 result 并返回该哨兵，使 handler 落入 tokens=0 免费用量行（对齐 /v1/responses），
// 既不计费、也不 failover、不重复写响应。
var errOpenAICyberPolicyForwarded = errors.New("openai policy block forwarded to client")

// CyberPolicyMark 记录一次上游策略硬阻断的凭证。
// Code 保留上游原始 error.code（"cyber_policy" 或 "invalid_prompt"）。
type CyberPolicyMark struct {
	Code           string // 上游 error.code；空值回落为 "cyber_policy"
	Message        string // 上游 error.message
	Body           string // 上游 response.failed / 400 原始 body（已截断；未脱敏，ops_error 落库由 sanitizeErrorBodyForStorage、风控日志由 redactContentModerationSecrets 统一脱敏）
	UpstreamStatus int    // 上游 HTTP 状态（流式=200，非流式=400）
	UpstreamInTok  int    // 上游已报 input tokens（如有）
	UpstreamOutTok int    // 上游已报 output tokens（如有）
}

// MarkOpsCyberPolicy 记录策略拦截标记；首个写入生效，后续忽略（同一 turn 只记一次）。
// WS 多轮场景由 handler 在每个 turn 结束后调用 ClearOpsCyberPolicy 重置。
func MarkOpsCyberPolicy(c *gin.Context, mark CyberPolicyMark) {
	if c == nil {
		return
	}
	if GetOpsCyberPolicy(c) != nil {
		return
	}
	mark.Code = strings.TrimSpace(mark.Code)
	if mark.Code == "" {
		mark.Code = "cyber_policy"
	}
	mark.Message = strings.TrimSpace(mark.Message)
	mark.Body = strings.TrimSpace(mark.Body)
	c.Set(opsCyberPolicyKey, &mark)
}

// GetOpsCyberPolicy 返回策略拦截标记，未命中（或已被 Clear）返回 nil。
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

// ClearOpsCyberPolicy 清除策略拦截标记（typed-nil 覆盖；gin context 无并发安全的
// 删除原语，Set 走内部锁，与异步 GetOpsCyberPolicy 不构成 data race）。
// 仅 WS 多轮路径在 turn 收尾调用；HTTP 单请求路径不调用（context 随请求销毁，
// 且中间件 shouldSkipOpsErrorLogForCyber 依赖标记防双写）。
// WS 路径 clear 发生在中间件收尾之前，连接响应状态为 101，不触发中间件 status>=400
// 落库分支，故无双写/漏写。
func ClearOpsCyberPolicy(c *gin.Context) {
	if c == nil {
		return
	}
	c.Set(opsCyberPolicyKey, (*CyberPolicyMark)(nil))
}

// detectOpenAIPolicyBlock 精确识别上游的 prompt 策略拦截。
//
// 两类错误都表示请求内容被上游策略拒绝，而不是账号或请求参数有问题：
//   - code=cyber_policy：Cyber abuse 硬阻断（对齐 codex api_bridge.rs:145 /
//     sse/responses.rs:529）。
//   - code=invalid_prompt：prompt 被判定违反 usage policy。必须同时命中策略
//     文案（"flagged as potentially violating our usage policy" 等），避免把
//     普通 invalid_prompt / 400 参数错误误判成风控事件。
//
// 不带 code 时，仅接受 invalid_request_error type + 明确的策略文案兜底。
func detectOpenAIPolicyBlock(payload []byte) (bool, string, string) {
	code := strings.TrimSpace(gjson.GetBytes(payload, "error.code").String())
	if code == "" {
		code = strings.TrimSpace(gjson.GetBytes(payload, "response.error.code").String())
	}
	msg := strings.TrimSpace(gjson.GetBytes(payload, "error.message").String())
	if msg == "" {
		msg = strings.TrimSpace(gjson.GetBytes(payload, "response.error.message").String())
	}
	if strings.EqualFold(code, "cyber_policy") {
		return true, "cyber_policy", msg
	}
	if strings.EqualFold(code, "invalid_prompt") && isOpenAIPromptPolicyMessage(msg) {
		return true, "invalid_prompt", msg
	}
	if code == "" && isOpenAIPromptPolicyMessage(msg) {
		errType := strings.TrimSpace(gjson.GetBytes(payload, "error.type").String())
		if errType == "" {
			errType = strings.TrimSpace(gjson.GetBytes(payload, "response.error.type").String())
		}
		if strings.EqualFold(errType, "invalid_request_error") || strings.EqualFold(errType, "invalid_prompt") {
			return true, "invalid_prompt", msg
		}
	}
	return false, "", ""
}

// isOpenAIPromptPolicyMessage 只认上游明确给出的策略文案。刻意不匹配单独的
// "invalid_prompt" / "Request blocked."：Codex 也用同一个 code 表示加密推理
// 重放被拒，那属于请求格式问题，不应计入风控。
func isOpenAIPromptPolicyMessage(message string) bool {
	lower := strings.ToLower(strings.TrimSpace(message))
	if lower == "" {
		return false
	}
	for _, marker := range []string{
		"violating our usage policy",
		"violates our usage polic",
		"flagged as potentially violating",
		"flagged by our safety",
		"your request was flagged",
		"prompt was flagged",
		"usage policy violation",
	} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func markOpenAICyberPolicyEvent(c *gin.Context, payload []byte, upstreamStatus int, usage *OpenAIUsage) bool {
	hit, code, message := detectOpenAIPolicyBlock(payload)
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
	MarkOpsCyberPolicy(c, mark)
	return true
}
