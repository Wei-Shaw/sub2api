package service

import (
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

// geminiClientRejectsSSEComments 判断下游客户端是否是「无法容忍 SSE 注释行」的 Google GenAI SDK。
//
// Gemini 原生流里我们用 ":\n\n" 作为空闲心跳（防止代理/Cloudflare Tunnel 断开），但官方 SDK 里
// 有三个实现不按 SSE 规范忽略注释行：
//   - go-genai（Antigravity CLI 在用）：api_client.go iterateResponseStream 对任何非 "data:" 前缀的
//     事件直接报 "invalid stream chunk: :"，整个流中断；
//   - python-genai：_api_client.py 把非 "data:" 行当作错误 JSON 拼接后 json.loads，同样抛错；
//   - js-genai < 1.32.0：processStreamResponse 只用 /^\s*data: (.*)(?:\n\n|...)/ 匹配缓冲区开头，
//     一遇到注释帧就再也消费不了后续事件，流结束时报 "Incomplete JSON segment at the end"。
//     Gemini CLI（至少到 0.70.0）锁定 js-genai 1.30.0。1.32.0（googleapis/js-genai c9c1359）起
//     改为按事件分隔符切分并丢弃非 data 事件，能正确忽略注释，所以新版本照常发心跳。
//
// Google 官方 API 从不下发注释行，所以这些 SDK 从未处理过这种输入。识别依据是 SDK 固定携带的
// User-Agent / X-Goog-Api-Client，形如 "google-genai-sdk/1.71.0 gl-go/go1.28 ..."、
// "google-genai-sdk/1.x gl-python/3.12" 或 "google-genai-sdk/1.30.0 gl-node/v24.22.3"
// （Gemini CLI 的 User-Agent 是 "GeminiCLI/..."，SDK 信息在 X-Goog-Api-Client 里）。
func geminiClientRejectsSSEComments(clientHint string) bool {
	hint := strings.ToLower(strings.TrimSpace(clientHint))
	_, version, ok := strings.Cut(hint, "google-genai-sdk/")
	if !ok {
		return false
	}
	if strings.Contains(hint, "gl-go/") || strings.Contains(hint, "gl-python/") {
		return true
	}
	if strings.Contains(hint, "gl-node/") {
		return jsGenAIVersionRejectsSSEComments(version)
	}
	return false
}

// jsGenAIVersionRejectsSSEComments 判断 js-genai 版本是否早于 1.32.0（能忽略注释的第一个版本）。
// version 以 "google-genai-sdk/" 之后的内容开头，例如 "1.30.0 gl-node/v24.22.3"。
// 解析不出版本时按新版本处理，保持原有的发心跳行为。
func jsGenAIVersionRejectsSSEComments(version string) bool {
	version, _, _ = strings.Cut(version, " ")
	majorStr, rest, ok := strings.Cut(version, ".")
	if !ok {
		return false
	}
	minorStr, _, _ := strings.Cut(rest, ".")
	major, err := strconv.Atoi(majorStr)
	if err != nil {
		return false
	}
	minor, err := strconv.Atoi(minorStr)
	if err != nil {
		return false
	}
	return major < 1 || (major == 1 && minor < 32)
}

// downstreamRejectsSSEComments 从请求头识别下游客户端（同时看 User-Agent 与 X-Goog-Api-Client）。
func downstreamRejectsSSEComments(c *gin.Context) bool {
	if c == nil || c.Request == nil {
		return false
	}
	return geminiClientRejectsSSEComments(c.GetHeader("User-Agent")) ||
		geminiClientRejectsSSEComments(c.GetHeader("X-Goog-Api-Client"))
}
