package service

// opencode zen 网关（opencode.ai/zen/*）provider 适配。
//
// opencode 官方网关有两个端点，行为不同（2026-08-07 实证）：
//   - /zen/v1（免费/普通端点）：serde 结构要求每条 message 带顶层 id，
//     OpenAI 标准消息不带 id 会 400 "missing field `id`"（带图/多模态消息
//     100% 触发）
//   - /zen/go/v1（Go 计划付费端点）：标准 OpenAI 兼容，无 id 要求
//
// 账号以 base_url 声明端点；本模块按档次做转发前适配。账号仍是通用
// openai/apikey 账号（extra 无需新字段），识别完全靠 base_url。

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// openCodeUpstreamHost 是 opencode zen 网关的域名片段，用于识别上游归属。
const openCodeUpstreamHost = "opencode.ai/zen/"

// IsOpenCodeUpstream 判定账号上游是否为 opencode zen 网关。
func (a *Account) IsOpenCodeUpstream() bool {
	return a != nil && strings.Contains(a.GetOpenAIBaseURL(), openCodeUpstreamHost)
}

// OpenCodePlan 返回 opencode 账号档次：go（Go 计划付费端点 /zen/go/）或
// free（免费/普通端点 /zen/）。非 opencode 上游返回空字符串。
func (a *Account) OpenCodePlan() string {
	if a == nil {
		return ""
	}
	base := a.GetOpenAIBaseURL()
	switch {
	case strings.Contains(base, "/zen/go/"):
		return "go"
	case strings.Contains(base, "/zen/"):
		return "free"
	default:
		return ""
	}
}

// OpenCodePrepareUpstreamBody 按账号档次准备发往 opencode 上游的请求体。
// free 档次（/zen/v1）serde 要求 message 带顶层 id，补齐缺失 id；
// go 档次（/zen/go/v1）标准 OpenAI 兼容，原样返回。
func OpenCodePrepareUpstreamBody(account *Account, body []byte) ([]byte, error) {
	if account == nil || !account.IsOpenCodeUpstream() || account.OpenCodePlan() == "go" {
		return body, nil
	}
	return ensureChatMessagesIDs(body)
}

// ensureChatMessagesIDs 为 OpenAI Chat Completions 请求的每条 message 补齐
// 顶层 id 字段（缺失时）。标准 OpenAI / DeepSeek 等上游忽略消息未知字段，
// 补 id 无副作用；opencode /zen/v1 端点要求 id 必填。
func ensureChatMessagesIDs(body []byte) ([]byte, error) {
	count := gjson.GetBytes(body, "messages.#").Int()
	if count <= 0 {
		return body, nil
	}
	out := body
	for i := int64(0); i < count; i++ {
		path := fmt.Sprintf("messages.%d.id", i)
		if gjson.GetBytes(out, path).Exists() {
			continue
		}
		next, err := sjson.SetBytes(out, path, "msg_"+strings.ReplaceAll(uuid.NewString(), "-", ""))
		if err != nil {
			return body, err
		}
		out = next
	}
	return out, nil
}
