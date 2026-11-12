package service

import (
	"bytes"
	"fmt"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func compactionMemoryRequestBody(imageBytes int, final bool) []byte {
	body := oauthInputMetadataRequestBody(imageBytes, 0)
	if final {
		return bytes.Replace(body, []byte(`],"opaque":`), []byte(`,{"type":"compaction_trigger"}],"opaque":`), 1)
	}
	return bytes.Replace(body, []byte(`"input":[`), []byte(`"input":[{"type":"compaction_trigger"},`), 1)
}

func TestCompactionNormalizationLargeTriggerRequestAllocations(t *testing.T) {
	for _, final := range []bool{true, false} {
		t.Run(fmt.Sprintf("already_final=%v", final), func(t *testing.T) {
			body := compactionMemoryRequestBody(2<<20, final)
			runtime.GC()
			var before, after runtime.MemStats
			runtime.ReadMemStats(&before)
			out, changed, err := NormalizeCompactionTriggerInputOrder(body)
			runtime.ReadMemStats(&after)
			require.NoError(t, err)
			require.Equal(t, !final, changed)
			budget := uint64(3 * len(body))
			if final {
				budget = 1 << 20
				require.Same(t, &body[0], &out[0])
			}
			require.Less(t, after.TotalAlloc-before.TotalAlloc, budget, "压缩标记整理的分配随请求体积增长")
			runtime.KeepAlive(out)
		})
	}
}

func BenchmarkCompactionTriggerLargeRequest(b *testing.B) {
	for _, size := range []int{16 << 20, 112 << 20} {
		for _, final := range []bool{true, false} {
			b.Run(fmt.Sprintf("%dMiB_final=%v", size>>20, final), func(b *testing.B) {
				body := compactionMemoryRequestBody(size, final)
				b.ReportAllocs()
				b.SetBytes(int64(len(body)))
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					out, changed, err := NormalizeCompactionTriggerInputOrder(body)
					if err != nil || changed == final {
						b.Fatalf("压缩标记整理结果：changed=%v err=%v", changed, err)
					}
					runtime.KeepAlive(out)
				}
			})
		}
	}
}

func TestCompactionRawHistoryMatchesDecodedContract(t *testing.T) {
	fixtures := []string{
		` {"input":[{"type":"compaction_trigger"},{"type":"message","content":"keep","sequence":9007199254740993}],"tail":true} `,
		`{"input":[{"type":"compaction_trigger"},null,42,"text",{"type":"compaction_trigger"}]}`,
		`{"input":[{"type":"compaction\u005ftrigger"},{"type":"message","content":"compaction_trigger"}]}`,
		`{"input":[{"type":"compaction_trigger","extra":"first"},{"type":"message"},{"type":"compaction_trigger","extra":"last"}]}`,
		`{"input":[{"type":"message","type":"compaction_trigger"},{"type":"message"}]}`,
		`{"input":[{"type":"compaction_trigger"}],"input":[{"type":"message"}]}`,
		`{"input":[{"type":"message"}],"input":[{"type":"compaction_trigger"},{"type":"message"}]}`,
		`{"input":[{"type":"message"},{"type":"compaction_trigger","extra":"keep"}]}`,
		`{"input":"compaction_trigger"}`,
		`{"input":[{"type":"compaction_trigger"}]} trailing`,
		`[{"type":"compaction_trigger"}]`,
		`null`,
	}
	for _, fixture := range fixtures {
		t.Run(fixture, func(t *testing.T) {
			body := []byte(fixture)
			original := bytes.Clone(body)
			want, wantChanged, wantErr := normalizeCompactionTriggerInputOrderDecoded(body)
			got, changed, err := NormalizeCompactionTriggerInputOrder(body)
			require.Equal(t, wantErr != nil, err != nil)
			require.Equal(t, wantChanged, changed)
			require.Equal(t, original, body)
			if err == nil {
				if changed {
					require.JSONEq(t, string(want), string(got))
				} else {
					require.Equal(t, want, got)
				}
			}
		})
	}
}

func TestCompactionNormalizationPreservesNestedDuplicateKeyValues(t *testing.T) {
	for _, fixture := range []struct {
		body    string
		path    string
		want    string
		changed bool
	}{
		{
			body:    `{"reasoning":{"effort":"low","effort":"high"},"input":[{"type":"compaction_trigger"},{"role":"user","content":"hello"}]}`,
			path:    "reasoning.effort",
			want:    "high",
			changed: true,
		},
		{
			body:    `{"input":[{"type":"compaction_trigger"},{"role":"user","content":[{"type":"input_text","text":"first","text":"latest"}]}]}`,
			path:    "input.0.content.0.text",
			want:    "latest",
			changed: true,
		},
		{
			body: `{"reasoning":{"effort":"low","effort":"high"},"input":[{"role":"user","content":"hello"},{"type":"compaction_trigger"}]}`,
			path: "reasoning.effort",
			want: "low",
		},
	} {
		t.Run(fmt.Sprintf("%s_changed=%v", fixture.path, fixture.changed), func(t *testing.T) {
			out, changed, err := NormalizeCompactionTriggerInputOrder([]byte(fixture.body))
			require.NoError(t, err)
			require.Equal(t, fixture.changed, changed)
			require.Equal(t, fixture.want, gjson.GetBytes(out, fixture.path).String())
			if fixture.path == "reasoning.effort" {
				require.Equal(t, fixture.want, newOpenAIRequestView(out).ReasoningEffort)
			}
		})
	}
}
