package service

import (
	"strings"

	"github.com/gin-gonic/gin"
)

// geminiClientRejectsSSEComments 判断下游客户端是否是「无法容忍 SSE 注释行」的 Google GenAI SDK。
//
// Gemini 原生流里我们用 ":\n\n" 作为空闲心跳（防止代理/Cloudflare Tunnel 断开），但官方 SDK 里
// 有两个实现不按 SSE 规范忽略注释行：
//   - go-genai（Antigravity CLI 在用）：api_client.go iterateResponseStream 对任何非 "data:" 前缀的
//     事件直接报 "invalid stream chunk: :"，整个流中断；
//   - python-genai：_api_client.py 把非 "data:" 行当作错误 JSON 拼接后 json.loads，同样抛错。
//
// js-genai 用的是规范的 event-stream 解析器，能正确忽略注释。Google 官方 API 从不下发注释行，
// 所以这两个 SDK 从未处理过这种输入。识别依据是 SDK 固定携带的 User-Agent / X-Goog-Api-Client，
// 形如 "google-genai-sdk/1.71.0 gl-go/go1.28 ..." 或 "google-genai-sdk/1.x gl-python/3.12"。
func geminiClientRejectsSSEComments(clientHint string) bool {
	hint := strings.ToLower(strings.TrimSpace(clientHint))
	if !strings.Contains(hint, "google-genai-sdk/") {
		return false
	}
	return strings.Contains(hint, "gl-go/") || strings.Contains(hint, "gl-python/")
}

// downstreamRejectsSSEComments 从请求头识别下游客户端（同时看 User-Agent 与 X-Goog-Api-Client）。
func downstreamRejectsSSEComments(c *gin.Context) bool {
	if c == nil || c.Request == nil {
		return false
	}
	return geminiClientRejectsSSEComments(c.GetHeader("User-Agent")) ||
		geminiClientRejectsSSEComments(c.GetHeader("X-Goog-Api-Client"))
}
