package service

import (
	"fmt"
	"strings"
	"testing"

	"github.com/tidwall/gjson"
)

// These measure only the orphan-cleanup stage, not the complete gateway. B/op
// is cumulative allocated memory per operation, not retained heap or RSS.
func BenchmarkOpenAIResponsesOrphanPrecheck(b *testing.B) {
	for _, size := range []int{64 << 10, 1 << 20, 4 << 20} {
		for _, previous := range []bool{false, true} {
			name := fmt.Sprintf("bytes_%d/previous_%t", size, previous)
			bodyText := `{"input":[{"type":"message","content":"` + strings.Repeat(`text\n`, size/6) + `"}]`
			if previous {
				bodyText += `,"previous_response_id":"resp_1"`
			}
			body := []byte(bodyText + `}`)
			b.Run(name+"/before", func(b *testing.B) {
				b.ReportAllocs()
				b.SetBytes(int64(len(body)))
				for b.Loop() {
					if gjson.GetBytes(body, "input").IsArray() {
						var request map[string]any
						if err := decodeOpenAIJSONUseNumber(body, &request); err != nil {
							b.Fatal(err)
						}
						input, _ := request["input"].([]any)
						if sanitizeOpenAIResponsesOrphanToolOutputs(request, input, previous) {
							b.Fatal("unexpected cleanup")
						}
					}
				}
			})
			b.Run(name+"/after", func(b *testing.B) {
				b.ReportAllocs()
				b.SetBytes(int64(len(body)))
				for b.Loop() {
					if openAIResponsesInputMayNeedOrphanCleanup(body) {
						b.Fatal("unexpected cleanup candidate")
					}
				}
			})
		}
	}
}
