// randid.go 集中随机 ID 生成，避免各 wire 编码点各写一份 crypto/rand。
//
// 移植自 WncFht/devin2api 的 internal/randid（MIT）。
package devin

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

// RandomHex 返回 size 字节随机数的十六进制编码；失败返回空串。
func RandomHex(size int) string {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return ""
	}
	return hex.EncodeToString(buf)
}

// UUID 返回随机 v4 UUID。crypto/rand 失败仅在理论可达，
// 返回全零 UUID 保持返回值可用而不引入错误分支。
func UUID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "00000000-0000-4000-8000-000000000000"
	}
	return FormatUUID(value)
}

// FormatUUID 把 16 字节格式化为 v4 UUID 字符串（固定 version/variant 位）。
// 随机 ID 与会话内容派生 ID 共用。
func FormatUUID(value [16]byte) string {
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	var out [36]byte
	hex.Encode(out[0:8], value[0:4])
	out[8] = '-'
	hex.Encode(out[9:13], value[4:6])
	out[13] = '-'
	hex.Encode(out[14:18], value[6:8])
	out[18] = '-'
	hex.Encode(out[19:23], value[8:10])
	out[23] = '-'
	hex.Encode(out[24:36], value[10:16])
	return string(out[:])
}

// PrefixedID 返回 prefix + 16 字节随机数的十六进制串，如 "msg_…"。
// crypto/rand 失败时退化为纳秒时间戳，保持 ID 唯一性。
func PrefixedID(prefix string) string {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return fmt.Sprintf("%s%x", prefix, time.Now().UnixNano())
	}
	return prefix + hex.EncodeToString(value)
}
