package service

// 本文件承载「上游 body 出口兜底清理」：在真正构造发往 ChatGPT internal Codex
// 端点的 HTTP 请求之前，做最后一道不支持字段的清理与诊断埋点。
//
// 背景（#5803）：ChatGPT internal Codex 端点拒绝一批 Platform API 顶层字段，
// 例如 prompt_cache_retention 会被以
//
//	{"error":{"code":"invalid_parameter","message":"prompt_cache_retention is not supported on this model",...}}
//
// 拒绝。这类参数错误换账号 failover 也救不了（换号后同样的 body 依旧被拒），
// 客户端直接吃 400。
//
// 现状是各转换分支各自清理：普通 Responses 路径（applyCodexOAuthTransform）、
// 透传路径（normalizeOpenAIPassthroughOAuthBody）、Chat Completions 的
// Responses-shape 短路（cursorResponsesUnsupportedFields）以及 grok 路径，
// 彼此独立且清单需要人工保持同步。分支多、且部分分支会在清理之后重建 body，
// 任一处遗漏都会让字段重新出现在出口。线上实测确有请求带着该字段到达上游，
// 但静态定位不到是哪条分支绕过了清理。
//
// 因此把清理收敛到唯一出口：无论上游哪条分支如何重建 body，最终发出前必过这里。
// 出口若真的删掉了字段，说明上游某条分支漏了，按 Warn 记录字段名用于定位——
// 只记字段名与出口名，不记字段值、不记请求正文（低基数、无敏感内容）。

import (
	"context"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	"go.uber.org/zap"
)

// OpenAI 上游 body 出口名，用于区分埋点来自哪个构造器。
const (
	openAIEgressForward     = "forward"
	openAIEgressPassthrough = "passthrough"
)

// stripChatGPTInternalUnsupportedFieldsAtEgress 删除 ChatGPT internal Codex 端点
// 不支持的顶层字段，仅对走该端点的 OpenAI OAuth 账号生效；其余账号（APIKey、
// 第三方 OpenAI 兼容上游、非 OpenAI 平台）原样返回，避免误删它们合法使用的字段。
//
// 返回清理后的 body 与实际被删掉的字段名。字段清单复用
// openAIChatGPTInternalUnsupportedFields，保持单一事实来源；清单内均为无 "." 的
// 顶层字段名，可直接作为 gjson/sjson 路径使用。
func stripChatGPTInternalUnsupportedFieldsAtEgress(account *Account, body []byte) ([]byte, []string) {
	if account == nil || !account.IsOpenAI() || account.Type != AccountTypeOAuth || len(body) == 0 {
		return body, nil
	}

	out := body
	var stripped []string
	for _, field := range openAIChatGPTInternalUnsupportedFields {
		if !gjson.GetBytes(out, field).Exists() {
			continue
		}
		next, err := sjson.DeleteBytes(out, field)
		if err != nil {
			// 单个字段删除失败不影响其余字段：宁可少删一个，也不要因为
			// 一次 sjson 失败把整个请求打回，出口清理是尽力而为的兜底。
			continue
		}
		out = next
		stripped = append(stripped, field)
	}
	return out, stripped
}

// logChatGPTInternalEgressStripped 记录出口兜底清理命中的字段。
//
// 命中即意味着上游某条转换分支没有清干净，属于需要跟进的实现缺陷，故用 Warn。
// 只记字段名与出口名，不记字段值、不记请求正文。
func logChatGPTInternalEgressStripped(ctx context.Context, account *Account, egress string, stripped []string) {
	if len(stripped) == 0 {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	accountID := int64(0)
	accountType := ""
	if account != nil {
		accountID = account.ID
		accountType = strings.TrimSpace(string(account.Type))
	}
	logger.FromContext(ctx).With(
		zap.String("component", "service.openai_gateway"),
		zap.Int64("account_id", accountID),
		zap.String("account_type", accountType),
		zap.String("egress", egress),
		zap.Strings("stripped_fields", stripped),
	).Warn("OpenAI 出口兜底清理：ChatGPT internal 不支持的顶层字段在构造上游请求前仍然存在")
}

// applyChatGPTInternalEgressStrip 是 stripChatGPTInternalUnsupportedFieldsAtEgress
// 与埋点的组合，供各上游请求构造器在 http.NewRequest 之前调用。
func applyChatGPTInternalEgressStrip(ctx context.Context, account *Account, body []byte, egress string) []byte {
	strippedBody, stripped := stripChatGPTInternalUnsupportedFieldsAtEgress(account, body)
	if len(stripped) == 0 {
		return body
	}
	logChatGPTInternalEgressStripped(ctx, account, egress, stripped)
	return strippedBody
}
