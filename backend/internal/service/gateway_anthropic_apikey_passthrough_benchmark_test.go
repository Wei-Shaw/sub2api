package service

import "testing"

func BenchmarkGatewayService_ParseSSEUsage_MessageStart(b *testing.B) {
	svc := &GatewayService{}
	data := `{"type":"message_start","message":{"usage":{"input_tokens":123,"cache_creation_input_tokens":45,"cache_read_input_tokens":6,"cached_tokens":6,"cache_creation":{"ephemeral_5m_input_tokens":20,"ephemeral_1h_input_tokens":25}}}}`
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		usage := &ClaudeUsage{}
		svc.parseSSEUsage(data, usage)
	}
}

func BenchmarkGatewayService_ParseSSEUsagePassthrough_MessageStart(b *testing.B) {
	svc := &GatewayService{}
	data := `{"type":"message_start","message":{"usage":{"input_tokens":123,"cache_creation_input_tokens":45,"cache_read_input_tokens":6,"cached_tokens":6,"cache_creation":{"ephemeral_5m_input_tokens":20,"ephemeral_1h_input_tokens":25}}}}`
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		usage := &ClaudeUsage{}
		svc.parseSSEUsagePassthrough(data, usage)
	}
}

func BenchmarkGatewayService_ParseSSEUsage_MessageDelta(b *testing.B) {
	svc := &GatewayService{}
	data := `{"type":"message_delta","usage":{"output_tokens":456,"cache_creation_input_tokens":30,"cache_read_input_tokens":7,"cached_tokens":7,"cache_creation":{"ephemeral_5m_input_tokens":10,"ephemeral_1h_input_tokens":20}}}`
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		usage := &ClaudeUsage{}
		svc.parseSSEUsage(data, usage)
	}
}

func BenchmarkGatewayService_ParseSSEUsagePassthrough_MessageDelta(b *testing.B) {
	svc := &GatewayService{}
	data := `{"type":"message_delta","usage":{"output_tokens":456,"cache_creation_input_tokens":30,"cache_read_input_tokens":7,"cached_tokens":7,"cache_creation":{"ephemeral_5m_input_tokens":10,"ephemeral_1h_input_tokens":20}}}`
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		usage := &ClaudeUsage{}
		svc.parseSSEUsagePassthrough(data, usage)
	}
}

func BenchmarkParseClaudeUsageFromResponseBody(b *testing.B) {
	body := []byte(`{"id":"msg_123","type":"message","usage":{"input_tokens":123,"output_tokens":456,"cache_creation_input_tokens":45,"cache_read_input_tokens":6,"cached_tokens":6,"cache_creation":{"ephemeral_5m_input_tokens":20,"ephemeral_1h_input_tokens":25}}}`)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = parseClaudeUsageFromResponseBody(body)
	}
}
