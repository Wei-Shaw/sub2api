package service

import (
	"strings"
	"testing"
)

var imageGenerationIntentBenchmarkResult bool

func BenchmarkIsImageGenerationIntent(b *testing.B) {
	largeInput := strings.Repeat("x", 1<<20)
	benchmarks := []struct {
		name string
		body []byte
		want bool
	}{
		{
			name: "1MiBInputNoImage",
			body: []byte(`{"model":"gpt-5.5","tools":[],"input":"` + largeInput + `","tool_choice":"auto"}`),
			want: false,
		},
		{
			name: "1MiBInputLeadingImageTool",
			body: []byte(`{"model":"gpt-5.5","tools":[{"type":"image_generation"}],"input":"` + largeInput + `"}`),
			want: true,
		},
		{
			name: "1MiBInputTrailingToolChoice",
			body: []byte(`{"model":"gpt-5.5","tools":[],"input":"` + largeInput + `","tool_choice":{"type":"image_generation"}}`),
			want: true,
		},
		{
			name: "Invalid1MiBJSON",
			body: []byte(`{"model":"gpt-5.5","input":"` + largeInput),
			want: false,
		},
		{
			name: "DuplicateKeysFirstWins",
			body: []byte(`{"model":"gpt-5.5","model":"gpt-image-2","tools":[],"tools":[{"type":"image_generation"}],"input":"` + largeInput + `","tool_choice":"auto","tool_choice":{"type":"image_generation"}}`),
			want: false,
		},
	}

	for _, benchmark := range benchmarks {
		b.Run(benchmark.name, func(b *testing.B) {
			if got := IsImageGenerationIntent("/v1/responses", "gpt-5.5", benchmark.body); got != benchmark.want {
				b.Fatalf("IsImageGenerationIntent() = %v, want %v", got, benchmark.want)
			}
			b.ReportAllocs()
			b.SetBytes(int64(len(benchmark.body)))
			b.ResetTimer()
			var result bool
			for i := 0; i < b.N; i++ {
				result = IsImageGenerationIntent("/v1/responses", "gpt-5.5", benchmark.body)
			}
			imageGenerationIntentBenchmarkResult = result
		})
	}
}
