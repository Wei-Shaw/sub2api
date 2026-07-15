package codebuddy

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

// GenerateRequestUUID 生成 RFC4122 v4 风格的随机 UUID 字符串，用于 codebuddy
// 上游要求的 X-Conversation-* / X-Request-ID 等头。这些头在官方 WorkBuddy 客户端
// 中每次请求都随机生成，上游不校验历史关联性。
func GenerateRequestUUID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // RFC4122 variant
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// GenerateHexID 生成 n 字节的随机十六进制字符串，用于 codebuddy 上游要求的
// X-B3-* / traceparent / X-Trace-ID 等链路追踪头。这些头在官方客户端中每次请求
// 随机生成（16 字节 trace id、8 字节 span id），上游不校验历史关联性。
func GenerateHexID(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%x", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}
