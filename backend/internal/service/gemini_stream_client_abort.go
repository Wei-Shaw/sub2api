package service

import (
	"context"
	"errors"
	"io"

	"github.com/gin-gonic/gin"
)

// geminiStreamAbortedWithBillableOutput 判断流式读取中断是否由客户端断开引起，且断开前
// 已观测到可计费的输出（token 用量或已回吐的图片）。满足时调用方以已收集的结果收尾并照常计费。
// usageMetadata 为累计值，按断开前收到的最后一份计费：读到结束分片后断开即完整用量；
// 中途断开时中间分片可能只含输入计数或不含用量，此时按已有计数计费或不计费。
func geminiStreamAbortedWithBillableOutput(c *gin.Context, err error, usage *ClaudeUsage) bool {
	if err == nil || errors.Is(err, io.EOF) {
		return false
	}
	clientGone := errors.Is(err, context.Canceled) ||
		(c != nil && c.Request != nil && c.Request.Context().Err() != nil)
	if !clientGone {
		return false
	}
	return usage.hasObservedTokens() || observedGeminiImageOutputs(c) > 0
}
