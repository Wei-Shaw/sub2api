package service

import (
	"fmt"
	"regexp"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

// openAIImageDebugLogMaxBytes 限制单条出图 debug 日志的最大长度，避免日志爆量。
const openAIImageDebugLogMaxBytes = 8192

// openAILongBase64Pattern 匹配出图请求/回包中的超长 base64 串（如图片输入或出图结果），
// 在 debug 日志里压缩展示，避免把整张图的 base64 打进日志。
var openAILongBase64Pattern = regexp.MustCompile(`[A-Za-z0-9+/]{200,}={0,2}`)

// sanitizeImagePayloadForLog 压缩出图请求/回包内容里的超长 base64 数据，并整体截断长度。
func sanitizeImagePayloadForLog(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	cleaned := openAILongBase64Pattern.ReplaceAllStringFunc(string(raw), func(m string) string {
		return fmt.Sprintf("<base64 omitted %d chars>", len(m))
	})
	return truncateString(cleaned, openAIImageDebugLogMaxBytes)
}

// logImageGenerationRequest 以 debug 级别打印出图请求体（发往上游前的最终内容）。
func logImageGenerationRequest(label, model string, body []byte) {
	logger.LegacyPrintf("service.openai_gateway",
		"[debug][OpenAI image] request %s model=%s body=%s",
		label, model, sanitizeImagePayloadForLog(body))
}

// logImageGenerationResponse 以 debug 级别打印出图回包内容（已压缩 base64）。
func logImageGenerationResponse(label string, stream bool, body []byte) {
	logger.LegacyPrintf("service.openai_gateway",
		"[debug][OpenAI image] response %s stream=%v body=%s",
		label, stream, sanitizeImagePayloadForLog(body))
}
