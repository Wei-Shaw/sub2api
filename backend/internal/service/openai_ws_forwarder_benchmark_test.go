package service

import (
	"fmt"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

var (
	benchmarkOpenAIWSPayloadJSONSink string
	benchmarkOpenAIWSStringSink      string
	benchmarkOpenAIWSBoolSink        bool
	benchmarkOpenAIWSBytesSink       []byte
)

func BenchmarkOpenAIWSForwarderHotPath(b *testing.B) {
	cfg := &config.Config{}
	svc := &OpenAIGatewayService{cfg: cfg}
	account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	reqBody := benchmarkOpenAIWSHotPathRequest()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		payload := svc.buildOpenAIWSCreatePayload(reqBody, account)
		_, _ = applyOpenAIWSRetryPayloadStrategy(payload, 2)
		setOpenAIWSTurnMetadata(payload, `{"trace":"bench","turn":"1"}`)

		benchmarkOpenAIWSStringSink = openAIWSPayloadString(payload, "previous_response_id")
		benchmarkOpenAIWSBoolSink = payload["tools"] != nil
		benchmarkOpenAIWSStringSink = summarizeOpenAIWSPayloadKeySizes(payload, openAIWSPayloadKeySizeTopN)
		benchmarkOpenAIWSStringSink = summarizeOpenAIWSInput(payload["input"])
		benchmarkOpenAIWSPayloadJSONSink = payloadAsJSON(payload)
	}
}

func benchmarkOpenAIWSHotPathRequest() map[string]any {
	tools := make([]map[string]any, 0, 24)
	for i := 0; i < 24; i++ {
		tools = append(tools, map[string]any{
			"type":        "function",
			"name":        fmt.Sprintf("tool_%02d", i),
			"description": "benchmark tool schema",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{"type": "string"},
					"limit": map[string]any{"type": "number"},
				},
				"required": []string{"query"},
			},
		})
	}

	input := make([]map[string]any, 0, 16)
	for i := 0; i < 16; i++ {
		input = append(input, map[string]any{
			"role":    "user",
			"type":    "message",
			"content": fmt.Sprintf("benchmark message %d", i),
		})
	}

	return map[string]any{
		"type":                 "response.create",
		"model":                "gpt-5.3-codex",
		"input":                input,
		"tools":                tools,
		"parallel_tool_calls":  true,
		"previous_response_id": "resp_benchmark_prev",
		"prompt_cache_key":     "bench-cache-key",
		"reasoning":            map[string]any{"effort": "medium"},
		"instructions":         "benchmark instructions",
		"store":                false,
	}
}

func BenchmarkOpenAIWSEventEnvelopeParse(b *testing.B) {
	event := []byte(`{"type":"response.completed","response":{"id":"resp_bench_1","model":"gpt-5.1","usage":{"input_tokens":12,"output_tokens":8}}}`)
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		eventType, responseID, response := parseOpenAIWSEventEnvelope(event)
		benchmarkOpenAIWSStringSink = eventType
		benchmarkOpenAIWSStringSink = responseID
		benchmarkOpenAIWSBoolSink = response.Exists()
	}
}

func BenchmarkOpenAIWSErrorEventFieldReuse(b *testing.B) {
	event := []byte(`{"type":"error","error":{"type":"invalid_request_error","code":"invalid_request","message":"invalid input"}}`)
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		codeRaw, errTypeRaw, errMsgRaw := parseOpenAIWSErrorEventFields(event)
		benchmarkOpenAIWSStringSink, benchmarkOpenAIWSBoolSink = classifyOpenAIWSErrorEventFromRaw(codeRaw, errTypeRaw, errMsgRaw)
		code, errType, errMsg := summarizeOpenAIWSErrorEventFieldsFromRaw(codeRaw, errTypeRaw, errMsgRaw)
		benchmarkOpenAIWSStringSink = code
		benchmarkOpenAIWSStringSink = errType
		benchmarkOpenAIWSStringSink = errMsg
		benchmarkOpenAIWSBoolSink = openAIWSErrorHTTPStatusFromRaw(codeRaw, errTypeRaw) > 0
	}
}

func BenchmarkReplaceOpenAIWSMessageModel_NoMatchFastPath(b *testing.B) {
	event := []byte(`{"type":"response.output_text.delta","delta":"hello world"}`)
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		benchmarkOpenAIWSBytesSink = replaceOpenAIWSMessageModel(event, "gpt-5.1", "custom-model")
	}
}

func BenchmarkReplaceOpenAIWSMessageModel_DualReplace(b *testing.B) {
	event := []byte(`{"type":"response.completed","model":"gpt-5.1","response":{"id":"resp_1","model":"gpt-5.1"}}`)
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		benchmarkOpenAIWSBytesSink = replaceOpenAIWSMessageModel(event, "gpt-5.1", "custom-model")
	}
}
