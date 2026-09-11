package service

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

const (
	openAIWSThreadIDHeader = "thread-id"
	openAIWSWindowIDHeader = "x-codex-window-id"

	// request_kind 是 codex 放在 x-codex-turn-metadata 里的请求类型，
	// 对应源码枚举 CodexResponsesRequestKind，目前四个取值：
	//   turn        用户轮次的采样请求，含工具调用后的每次续采样
	//   prewarm     预热请求：会话启动预开的连接会被首个 turn 复用，
	//               新连接上也是先 warmup 再在同一连接发 turn
	//   compaction  上下文压缩：在两次采样之间顺序执行，复用当前 turn 的
	//               client session
	//   memory      记忆整理：独立 root turn，后台并发发起，却沿用会话的
	//               thread_id，与用户 turn 同键时互相切断在飞轮次
	// 前三种与用户 turn 是同一条连接或同一段顺序动作，必须同道；
	// 未声明 request_kind 的请求（旧版 codex、只带 thread-id 头）视同 turn，
	// 键与此前完全一致；明确声明但认不出的取值各自成道，宁可只在同类间
	// 互相替换，也不让未知副请求打断用户轮次。
	openAIWSRequestKindTurn       = "turn"
	openAIWSRequestKindPrewarm    = "prewarm"
	openAIWSRequestKindCompaction = "compaction"
)

// resolveOpenAIWSClientThreadID 提取 codex 客户端的线程标识。codex 多智能体会话里
// 父线程与全部子智能体共用同一个 session-id 头，只有线程标识能把它们区分开。
// 优先级：thread-id 头 → x-codex-turn-metadata 头 → x-codex-window-id 头 → 请求体 client_metadata。
func resolveOpenAIWSClientThreadID(c *gin.Context, body []byte) string {
	if c != nil && c.Request != nil {
		if id := strings.TrimSpace(c.GetHeader(openAIWSThreadIDHeader)); id != "" {
			return id
		}
		if id := codexTurnMetadataThreadID(c.GetHeader(openAIWSTurnMetadataHeader)); id != "" {
			return id
		}
		if window := strings.TrimSpace(c.GetHeader(openAIWSWindowIDHeader)); window != "" {
			if id := strings.TrimSpace(strings.SplitN(window, ":", 2)[0]); id != "" {
				return id
			}
		}
	}
	if len(body) == 0 {
		return ""
	}
	if id := strings.TrimSpace(gjson.GetBytes(body, "client_metadata.thread_id").String()); id != "" {
		return id
	}
	return codexTurnMetadataThreadID(gjson.GetBytes(body, "client_metadata."+openAIWSTurnMetadataHeader).String())
}

// codexTurnMetadata 是 x-codex-turn-metadata JSON 里执行作用域用到的字段。
type codexTurnMetadata struct {
	ThreadID    string `json:"thread_id"`
	RequestKind string `json:"request_kind"`
}

func parseCodexTurnMetadata(raw string) (codexTurnMetadata, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return codexTurnMetadata{}, false
	}
	var metadata codexTurnMetadata
	if err := json.Unmarshal([]byte(raw), &metadata); err != nil {
		return codexTurnMetadata{}, false
	}
	metadata.ThreadID = strings.TrimSpace(metadata.ThreadID)
	metadata.RequestKind = strings.ToLower(strings.TrimSpace(metadata.RequestKind))
	return metadata, true
}

func codexTurnMetadataThreadID(raw string) string {
	metadata, _ := parseCodexTurnMetadata(raw)
	return metadata.ThreadID
}

// openAIWSExecutionTurnMetadata 取本次连接的 turn 元数据：x-codex-turn-metadata 头优先，
// 其次请求体 client_metadata 内嵌的同名 JSON（Desktop 走 HTTP 时只有后者）。
func openAIWSExecutionTurnMetadata(c *gin.Context, body []byte) codexTurnMetadata {
	if c != nil && c.Request != nil {
		if metadata, ok := parseCodexTurnMetadata(c.GetHeader(openAIWSTurnMetadataHeader)); ok {
			return metadata
		}
	}
	if len(body) == 0 {
		return codexTurnMetadata{}
	}
	metadata, _ := parseCodexTurnMetadata(gjson.GetBytes(body, "client_metadata."+openAIWSTurnMetadataHeader).String())
	return metadata
}

func openAIWSExecutionSubagent(c *gin.Context, body []byte) string {
	if c != nil && c.Request != nil {
		if subagent := strings.TrimSpace(c.GetHeader(openAISubagentHeader)); subagent != "" {
			return strings.ToLower(subagent)
		}
	}
	if len(body) == 0 {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(gjson.GetBytes(body, "client_metadata."+openAISubagentHeader).String()))
}

// resolveOpenAIWSExecutionLane 派生执行作用域里的“道”，同道才互相抢占、共享会话级状态。
// 主道（turn / prewarm / compaction / 未声明）返回空串，其余 request_kind 各自成道，取值
// 语义见上面的常量注释；没有线程元数据但声明了 x-openai-subagent 的（guardian 评分器）
// 按子智能体值成道，它们与用户 turn 同键时同样会互相切断在飞轮次。
func resolveOpenAIWSExecutionLane(c *gin.Context, body []byte) string {
	metadata := openAIWSExecutionTurnMetadata(c, body)
	switch metadata.RequestKind {
	case "", openAIWSRequestKindTurn, openAIWSRequestKindPrewarm, openAIWSRequestKindCompaction:
	default:
		return "kind=" + metadata.RequestKind
	}
	if metadata.ThreadID == "" {
		if subagent := openAIWSExecutionSubagent(c, body); subagent != "" {
			return "subagent=" + subagent
		}
	}
	return ""
}

func openAIWSExecutionScopeSeed(apiKeyID int64, identity, value, lane string) string {
	seed := fmt.Sprintf("openai_ws_exec:%d|%s=%s", apiKeyID, identity, value)
	if lane != "" {
		seed += "|" + lane
	}
	return seed
}

// resolveOpenAIWSExecutionScope 派生 WS 接入用于抢占与会话级状态的执行作用域键，
// 并一并返回解析到的线程标识供日志使用。身份只取客户端声明的线程标识或显式会话标识，
// 不掺入请求内容，因此同线程重连在输入变化后仍命中同一键；键里带 API key，同组不同 key
// 互不影响；同线程上与用户 turn 并发的独立请求按 resolveOpenAIWSExecutionLane 另成一道。
// 两种身份都没有时返回空作用域：按请求内容推导的粘性种子只服务账号亲和，
// 不是可靠身份，不参与抢占。
func resolveOpenAIWSExecutionScope(c *gin.Context, body []byte, apiKeyID int64) (scope, threadID string) {
	lane := resolveOpenAIWSExecutionLane(c, body)
	if threadID = resolveOpenAIWSClientThreadID(c, body); threadID != "" {
		scope, _ = deriveOpenAISessionHashes(openAIWSExecutionScopeSeed(apiKeyID, "thread", threadID, lane))
		return scope, threadID
	}
	if sessionID := strings.TrimSpace(explicitOpenAIRequestSessionID(c, body)); sessionID != "" {
		scope, _ = deriveOpenAISessionHashes(openAIWSExecutionScopeSeed(apiKeyID, "session", sessionID, lane))
		return scope, ""
	}
	return "", ""
}
