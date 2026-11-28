package service

import (
	"bytes"
	"fmt"
	"runtime"
	"strings"
	"testing"

	"github.com/tidwall/gjson"
)

type stripFunc func(body []byte, keep bool) ([]byte, error)

func benchmarkStripOpenAIResponsesInputNamespaces(b *testing.B, variant string, fn stripFunc) {
	for _, size := range []int{1 << 20, 8 << 20} {
		message := `{"type":"message","content":"` + strings.Repeat("x", size) + `"}`
		call := `{"type":"function_call","namespace":"files","name":"search"}`
		for _, tc := range []struct {
			name       string
			input      string
			keep       bool
			stripIndex int
		}{
			// tools 里带 namespace 声明、input 里没有：Codex 常态，入口预检查必然放行。
			{"tools_only", message, false, -1},
			{"nested_only", `{"content":{"namespace":"nested"}},` + message, false, -1},
			{"keep_calls", call + "," + message, true, -1},
			{"strip_first", call + "," + message, false, 0},
			{"strip_last", message + "," + call, false, 1},
		} {
			b.Run(fmt.Sprintf("%s/%s/%dMiB", variant, tc.name, size>>20), func(b *testing.B) {
				body := []byte(`{"tools":[{"type":"namespace","name":"files","tools":[]}],"input":[` + tc.input + `]}`)
				out, err := fn(body, tc.keep)
				if err != nil {
					b.Fatal(err)
				}
				if tc.stripIndex < 0 {
					if !bytes.Equal(body, out) {
						b.Fatal("no-op changed body")
					}
				} else if !gjson.ValidBytes(out) || gjson.GetBytes(out, fmt.Sprintf("input.%d.namespace", tc.stripIndex)).Exists() {
					b.Fatal("namespace was not removed")
				}
				b.ReportAllocs()
				b.SetBytes(int64(len(body)))
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					out, err = fn(body, tc.keep)
					if err != nil {
						b.Fatal(err)
					}
					runtime.KeepAlive(out)
				}
			})
		}
	}
}

func BenchmarkStripOpenAIResponsesInputNamespaces(b *testing.B) {
	benchmarkStripOpenAIResponsesInputNamespaces(b, "current", stripOpenAIResponsesInputNamespaces)
}

func BenchmarkStripOpenAIResponsesInputNamespacesLegacy(b *testing.B) {
	benchmarkStripOpenAIResponsesInputNamespaces(b, "legacy", legacyStripOpenAIResponsesInputNamespaces)
}

func BenchmarkStripOpenAIResponsesInputNamespacesKeepExternalMessage(b *testing.B) {
	body := []byte(`{"tools":[{"type":"function","name":"exec"}],"input":[` +
		`{"type":"function_call_output","id":"fco_1","namespace":"agent","name":"browser_tabs","output":"` + strings.Repeat("x", 1<<20) + `"},` +
		`{"type":"message","role":"user","content":"hi"}]}`)
	opts := openAIResponsesInputNamespaceStripOptions{
		keepToolCallNamespaces:        false,
		keepExternalMessageNamespaces: true,
	}
	out, err := stripOpenAIResponsesInputNamespacesWithOptions(body, opts)
	if err != nil || !bytes.Equal(body, out) {
		b.Fatalf("expected no-op preservation, err=%v", err)
	}
	b.ReportAllocs()
	b.SetBytes(int64(len(body)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		out, err = stripOpenAIResponsesInputNamespacesWithOptions(body, opts)
		if err != nil {
			b.Fatal(err)
		}
		runtime.KeepAlive(out)
	}
}
