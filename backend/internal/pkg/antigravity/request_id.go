package antigravity

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// GenerateOfficialRequestID 构造符合 Google 官方 Antigravity 规范的 5 段式 Request ID。
// 格式: agent/{conv16}/{unixMs}/{traj8}/{step}
// - conv16: conversationID 存在时为 sha256 前 16 位 hex；为空时生成 16 位随机 hex
// - unixMs: 当前 Unix 毫秒时间戳
// - traj8: 8 位随机 hex 字符串（用于 trajectory 跟踪与幂等隔离）
// - step: 当前交互步数，>= 0
func GenerateOfficialRequestID(conversationID string, step int) string {
	if step < 0 {
		step = 0
	}

	var conv16 string
	conversationID = strings.TrimSpace(conversationID)
	if conversationID != "" {
		h := sha256.Sum256([]byte(conversationID))
		conv16 = hex.EncodeToString(h[:])[:16]
	} else {
		conv16 = randomHex(8) // 8 bytes = 16 hex chars
	}

	unixMs := time.Now().UnixMilli()
	traj8 := randomHex(4) // 4 bytes = 8 hex chars

	return fmt.Sprintf("agent/%s/%d/%s/%d", conv16, unixMs, traj8, step)
}

func randomHex(byteLen int) string {
	b := make([]byte, byteLen)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%0*x", byteLen*2, time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}
