package service

import (
	"bytes"
	"fmt"
	"runtime"
	"strings"
	"testing"

	"github.com/tidwall/gjson"
)

func BenchmarkStripOpenAIResponsesInputNamespaces(b *testing.B) {
	for _, size := range []int{1 << 20, 8 << 20} {
		message := `{"type":"message","content":"` + strings.Repeat("x", size) + `"}`
		call := `{"type":"function_call","namespace":"files","name":"search"}`
		for _, tc := range []struct {
			name       string
			input      string
			keep       bool
			stripIndex int
		}{
			{"tools_only", message, false, -1},
			{"nested_only", `{"content":{"namespace":"nested"}},` + message, false, -1},
			{"keep_calls", call + "," + message, true, -1},
			{"strip_first", call + "," + message, false, 0},
			{"strip_last", message + "," + call, false, 1},
		} {
			b.Run(fmt.Sprintf("%s/%dMiB", tc.name, size>>20), func(b *testing.B) {
				body := []byte(`{"tools":[{"type":"namespace","name":"files","tools":[]}],"input":[` + tc.input + `]}`)
				out, err := stripOpenAIResponsesInputNamespaces(body, tc.keep)
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
					out, err = stripOpenAIResponsesInputNamespaces(body, tc.keep)
					if err != nil {
						b.Fatal(err)
					}
					runtime.KeepAlive(out)
				}
			})
		}
	}
}
