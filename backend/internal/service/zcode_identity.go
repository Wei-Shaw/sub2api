package service

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"
)

// zhipu 账号出站客户端身份：发往 open.bigmodel.cn / api.z.ai 的推理请求统一
// 模拟为 ZCode Desktop 客户端指纹（真实抓包 CLIProxyAPI#4105，ZCode Desktop
// 3.2.5）。上游按客户端身份做风控与降载，claude-cli / Codex / 浏览器身份会
// 落在不利一侧，因此在两条共享上游构造器（sendCCUpstreamRequest /
// buildNativeAnthropicUpstreamRequest）与账号探测路径中、ApplyHeaderOverrides
// 之前强制覆盖；账号级 header_overrides 仍是最后一步，可覆盖注入值。
// 非 zhipu 账号一律 no-op。zhipu 无原生 Responses，全部推理流量只经这两条
// 共享构造器出站。

// zcodeIdentityKind 区分出站协议面：ZCode Desktop 的 OpenAI（ai-sdk/
// provider-utils）与 Anthropic（ai-sdk/anthropic）两条 SDK 栈 UA 不同。
type zcodeIdentityKind int

const (
	zcodeIdentityOpenAI zcodeIdentityKind = iota
	zcodeIdentityAnthropic
)

// zcodeAppVersion 抓包对应的 ZCode Desktop 版本，UA 与 X-Zcode-App-Version 同源。
const zcodeAppVersion = "3.2.5"

// zcodeSessionIDNamespace X-Session-Id 派生用的固定 UUIDv5 命名空间（值本身
// 任意，仅用于让同一账号恒定派生同一会话 ID）。
var zcodeSessionIDNamespace = uuid.MustParse("5f3d9a1c-7b2e-4c48-9d6a-1e0f2b3c4d5e")

// zcodeConflictingIdentityHeaders 常见客户端残留、且与 ZCode 指纹冲突的身份头。
// 只剥身份头：authorization / x-api-key / content-type / accept /
// anthropic-version / anthropic-beta 等鉴权与协议头不在剥离范围。
var zcodeConflictingIdentityHeaders = []string{
	"user-agent",
	"x-app",
	"x-claude-code-session-id",
	"x-client-request-id",
	"anthropic-dangerous-direct-browser-access",
	"originator",
	"version",
	"session_id",
	"conversation_id",
}

// applyZCodeIdentityHeaders 把出站请求头改写为 ZCode Desktop 指纹。
// account 为 nil、非 zhipu 平台或 h 为 nil 时直接返回。
func applyZCodeIdentityHeaders(h http.Header, account *Account, kind zcodeIdentityKind) {
	if h == nil || account == nil || !account.IsZhipu() {
		return
	}
	// 先剥离冲突身份头，再强制写入指纹，避免 passthrough 以不同 casing 写入
	// 的同名头与指纹并发出站。
	for _, key := range zcodeConflictingIdentityHeaders {
		deleteHeaderAllForms(h, key)
	}
	deleteZCodePrefixedHeaders(h, "x-codex-")
	deleteZCodePrefixedHeaders(h, "x-stainless-")

	// setHeaderRaw 写字面 casing（与 header_util.go 惯例一致）；HTTP-Referer
	// 尤其必须如此，Header.Set 会把它规范化成 Http-Referer，与真实客户端不符。
	setHeaderRaw(h, "User-Agent", zcodeUserAgent(kind))
	setHeaderRaw(h, "X-Zcode-App-Version", zcodeAppVersion)
	setHeaderRaw(h, "X-Zcode-Agent", "glm")
	setHeaderRaw(h, "X-Title", "Z Code@electron")
	setHeaderRaw(h, "HTTP-Referer", "https://zcode.z.ai")
	setHeaderRaw(h, "X-Platform", "darwin-arm64")
	setHeaderRaw(h, "X-Os-Category", "macos")
	setHeaderRaw(h, "X-Os-Version", "25.5.0")
	// 逐请求随机 UUID。
	setHeaderRaw(h, "X-Zcode-Trace-Id", uuid.NewString())
	setHeaderRaw(h, "X-Request-Id", uuid.NewString())
	setHeaderRaw(h, "X-Query-Id", uuid.NewString())
	// 按账号稳定派生，使同一账号的多次请求共享同一会话指纹。
	setHeaderRaw(h, "X-Session-Id", zcodeSessionIDForAccount(account.ID))
}

// zcodeUserAgent 按 kind 返回对应 SDK 栈的 User-Agent（取自真实抓包）。
func zcodeUserAgent(kind zcodeIdentityKind) string {
	if kind == zcodeIdentityAnthropic {
		return "ZCode/" + zcodeAppVersion + " ai-sdk/anthropic/3.0.81"
	}
	return "ZCode/" + zcodeAppVersion + " ai-sdk/provider-utils/4.0.27 runtime/node.js/24"
}

// zcodeSessionIDForAccount 用固定命名空间对账号 ID 做 UUIDv5 派生，
// 同一账号恒定得到同一 X-Session-Id。
func zcodeSessionIDForAccount(accountID int64) string {
	return uuid.NewSHA1(zcodeSessionIDNamespace, []byte(strconv.FormatInt(accountID, 10))).String()
}

// deleteZCodePrefixedHeaders 删除全部匹配 lowercase 前缀的身份头（任意 casing
// 形态），覆盖 x-codex-* / x-stainless-* 这类客户端 SDK 指纹族。
func deleteZCodePrefixedHeaders(h http.Header, prefix string) {
	for key := range h {
		if strings.HasPrefix(strings.ToLower(key), prefix) {
			deleteHeaderAllForms(h, key)
		}
	}
}
