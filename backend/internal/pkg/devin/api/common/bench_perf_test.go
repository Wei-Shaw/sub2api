// 性能基准：图片 base64 解码与 MIME 嗅探路径。
package common

import (
	"crypto/rand"
	"encoding/base64"
	"testing"
)

// benchPNGBase64 返回一段合法 PNG 魔数 + 随机体的 base64 串（约 1MB 解码后）。
func benchPNGBase64(b *testing.B) string {
	raw := make([]byte, 1<<20)
	raw[0] = 0x89
	raw[1] = 0x50 // 'P'
	raw[2] = 0x4e // 'N'
	raw[3] = 0x47 // 'G'
	if _, err := rand.Read(raw[4:]); err != nil {
		b.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(raw)
}

// BenchmarkDecodeRawBase64 测量大图 data URL 的三程成本：
// strings.Map 清洗复制 + DecodeString + EncodeToString 再编码。
func BenchmarkDecodeRawBase64(b *testing.B) {
	encoded := benchPNGBase64(b)
	b.Logf("base64 len = %d", len(encoded))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := DecodeRawBase64(encoded, "image/png"); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkSniffImageMIME 测量魔数嗅探的全量解码成本：
// 只需要前 12 字节却解码了整个 base64。
func BenchmarkSniffImageMIME(b *testing.B) {
	encoded := benchPNGBase64(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if got := SniffImageMIME(encoded); got != "image/png" {
			b.Fatalf("mime = %q", got)
		}
	}
}
