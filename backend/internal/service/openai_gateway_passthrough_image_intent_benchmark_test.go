package service

import "testing"

var passthroughImageIntentBenchmarkSink bool

func BenchmarkOpenAIPassthroughImageIntentReuse_LargeBody(b *testing.B) {
	body := buildLargeOpenAIResponsesImageToolBody(32 << 20)

	b.Run("Once", func(b *testing.B) {
		b.SetBytes(int64(len(body)))
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			passthroughImageIntentBenchmarkSink = IsImageGenerationIntent(openAIResponsesEndpoint, "gpt-5.4", body)
		}
	})

	b.Run("Twice", func(b *testing.B) {
		b.SetBytes(int64(len(body)))
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			permissionIntent := IsImageGenerationIntent(openAIResponsesEndpoint, "gpt-5.4", body)
			billingIntent := IsImageGenerationIntent(openAIResponsesEndpoint, "gpt-5.4", body)
			passthroughImageIntentBenchmarkSink = permissionIntent && billingIntent
		}
	})
}
