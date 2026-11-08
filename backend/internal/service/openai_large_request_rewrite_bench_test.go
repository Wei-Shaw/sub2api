package service

import (
	"bytes"
	"runtime"
	"testing"
)

func BenchmarkOpenAIStoreFalseMetadataRewrite(b *testing.B) {
	body := largeNativeResponsesBody(69 << 20)
	body = bytes.Replace(body, []byte(`"input":[`), []byte(`"input":[{"type":"reasoning","id":"rs_server_id","encrypted_content":"opaque","summary":[]},`), 1)
	b.ReportAllocs()
	b.SetBytes(int64(len(body)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		out, changed, err := normalizeOpenAIAPIKeyStoreFalseReasoningReplay(body, false)
		if err != nil || !changed || len(out) < 69<<20 {
			b.Fatalf("metadata rewrite failed or image was lost: changed=%v err=%v", changed, err)
		}
		runtime.KeepAlive(out)
	}
}
