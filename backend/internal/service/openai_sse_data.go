package service

import "github.com/Wei-Shaw/sub2api/internal/pkg/openai"

// openAISSEDataAccumulator is a service-local alias of the pkg/openai helper.
// Kept unexported so existing call sites compile without import churn.
type openAISSEDataAccumulator = openai.SSEDataAccumulator

func forEachOpenAISSEDataPayload(body string, fn func([]byte)) {
	openai.ForEachSSEDataPayload(body, fn)
}

func extractOpenAISSEDataLine(line string) (string, bool) {
	return openai.ExtractSSEDataLine(line)
}

func extractOpenAISSEEventLine(line string) (string, bool) {
	return openai.ExtractSSEEventLine(line)
}
