package service

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

// openAINativeCompactionV2Key 标记本请求是原生 remote compaction v2
// （裸 /responses + stream:true + compaction_trigger），由 handler 在判定后
// 写入，供上游请求构造时补注协商头。
const openAINativeCompactionV2Key = "openai_native_compaction_v2"

const openAIRemoteCompactionV2Feature = "remote_compaction_v2"

// MarkOpenAINativeCompactionV2 由 handler 在识别出原生 v2 压缩请求时调用。
func MarkOpenAINativeCompactionV2(c *gin.Context) {
	if c != nil {
		c.Set(openAINativeCompactionV2Key, true)
	}
}

// NormalizeCompactionTriggerInputOrder keeps a single compaction trigger as
// the final Responses input item, as required by the upstream v2 wire format.
func NormalizeCompactionTriggerInputOrder(body []byte) ([]byte, bool, error) {
	if len(body) == 0 {
		return body, false, nil
	}
	var payload map[string]any
	if err := decodeOpenAIJSONUseNumber(body, &payload); err != nil {
		return body, false, err
	}
	input, ok := payload["input"].([]any)
	if !ok || len(input) == 0 {
		return body, false, nil
	}
	triggerCount := 0
	normalized := make([]any, 0, len(input))
	for _, raw := range input {
		item, itemOK := raw.(map[string]any)
		if itemOK && item["type"] == "compaction_trigger" {
			triggerCount++
			continue
		}
		normalized = append(normalized, raw)
	}
	if triggerCount == 0 {
		return body, false, nil
	}
	if triggerCount == 1 {
		if last, ok := input[len(input)-1].(map[string]any); ok && last["type"] == "compaction_trigger" {
			return body, false, nil
		}
	}
	normalized = append(normalized, map[string]any{"type": "compaction_trigger"})
	payload["input"] = normalized
	encoded, err := marshalOpenAIUpstreamJSON(payload)
	if err != nil {
		return body, false, err
	}
	return encoded, true, nil
}

func isOpenAINativeCompactionV2(c *gin.Context) bool {
	if c == nil {
		return false
	}
	return c.GetBool(openAINativeCompactionV2Key)
}

// ensureOpenAIRemoteCompactionV2BetaFeature 确保出站 x-codex-beta-features
// 头包含 remote_compaction_v2。真实 Codex 发送 compaction_trigger 时总会同时
// 携带该协商头（codex-rs build_model_client_beta_features_header 对该 feature
// 特判 advertise）；上游或下游网关链剥掉它后，请求会在依赖该头做门控的
// 环节被降级（#5586）。这里在原生 v2 请求出站前补齐，使线型与真实 Codex
// 一致。已存在时保持原样，不重复追加。
func ensureOpenAIRemoteCompactionV2BetaFeature(h http.Header) {
	if h == nil {
		return
	}
	tokens := make([]string, 0, 4)
	for _, value := range h.Values("x-codex-beta-features") {
		for _, token := range strings.Split(value, ",") {
			token = strings.TrimSpace(token)
			if token == "" {
				continue
			}
			if token == openAIRemoteCompactionV2Feature {
				return
			}
			tokens = append(tokens, token)
		}
	}
	tokens = append(tokens, openAIRemoteCompactionV2Feature)
	h.Set("x-codex-beta-features", strings.Join(tokens, ","))
}

// hasOpenAICodexBetaFeaturesHeader 报告出站头里是否已存在非空的
// x-codex-beta-features（即客户端自己声明过能力集）。
func hasOpenAICodexBetaFeaturesHeader(h http.Header) bool {
	if h == nil {
		return false
	}
	for _, value := range h.Values("x-codex-beta-features") {
		if strings.TrimSpace(value) != "" {
			return true
		}
	}
	return false
}

// applyOpenAICodexBetaFeatures 按真实 Codex 的会话级行为补注
// x-codex-beta-features。
//
// codex 侧规则（codex-rs：session/mod.rs build_model_client_beta_features_header
// 组装、client.rs build_responses_headers 附加）：该头是**会话级常量**，挂在
// /responses、WS 握手、/responses/compact 三处的**每一个**请求上，而不是只挂
// 压缩回合。其内容是"已启用且需要 advertise 的 feature 列表"，RemoteCompactionV2
// 被特判 advertise；实测默认安装下没有任何 Experimental 特性默认开启，
// 因此默认 Codex 的头值恰好就是单个 "remote_compaction_v2"。
//
// 网关据此对齐：
//   - 原生 v2 压缩回合（body 带 compaction_trigger 实锤）：无论账号类型都确保
//     v2 在列，覆盖中间网关裁剪 token 的情形（#5586）；
//   - ChatGPT codex 上游（OAuth）的其余请求：客户端**未**声明该头时补成默认
//     Codex 形态，消除"仅压缩回合才带该头"这种真实 Codex 不会产生的模式；
//   - 客户端已声明该头：原样保留。非空但不含 v2 表示用户显式关闭了该特性，
//     网关不得替其改写能力声明；
//   - 非 OAuth 上游（API Key/第三方兼容网关）：不做会话级注入，只保留压缩回合
//     的那一条，避免向非 Codex 后端撒 Codex 专属头。
//
// 已知无解的歧义：用户关掉 v2 且无其他特性时，真实 Codex 同样不发该头，与"老
// 客户端"在线型上不可区分，此时按默认形态补注。该用户的 legacy 压缩端点本就
// 已被上游下线（404），不存在可回退的正确行为。
func applyOpenAICodexBetaFeatures(c *gin.Context, account *Account, h http.Header) {
	if h == nil {
		return
	}
	if isOpenAINativeCompactionV2(c) {
		ensureOpenAIRemoteCompactionV2BetaFeature(h)
		return
	}
	if account == nil || !account.IsOpenAIOAuthLike() {
		return
	}
	if hasOpenAICodexBetaFeaturesHeader(h) {
		return
	}
	h.Set("x-codex-beta-features", openAIRemoteCompactionV2Feature)
}

// HasCompactionTriggerInInput detects an input item with
// type="compaction_trigger". The handler combines this body signal with the
// request path and stream flag to distinguish the native remote compaction v2
// wire from the legacy /responses/compact bridge.
func HasCompactionTriggerInInput(body []byte) bool {
	if len(body) == 0 {
		return false
	}
	input := gjson.GetBytes(body, "input")
	if !input.IsArray() {
		return false
	}
	found := false
	input.ForEach(func(_, item gjson.Result) bool {
		if item.Get("type").String() == "compaction_trigger" {
			found = true
			return false
		}
		return true
	})
	return found
}
