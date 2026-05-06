package service

import (
	"testing"
	"github.com/tidwall/gjson"
)

func TestParseSSEUsageBytes_ToolUsageFallback(t *testing.T) {
	// issue #1999 报告的真实上游 response 形态
	data := []byte(`{"type":"response.completed","response":{"model":"gpt-5.5","output":[],"usage":{"input_tokens":1820,"output_tokens":210},"tool_usage":{"image_gen":{"output_tokens":7024,"output_tokens_details":{"image_tokens":7024}}}}}`)
	if !gjson.ValidBytes(data) {
		t.Fatal("invalid json fixture")
	}
	usage := &OpenAIUsage{}
	s := &OpenAIGatewayService{}
	s.parseSSEUsageBytes(data, usage)
	if usage.ImageOutputTokens != 7024 {
		t.Fatalf("want ImageOutputTokens=7024, got %d", usage.ImageOutputTokens)
	}
	if usage.InputTokens != 1820 || usage.OutputTokens != 210 {
		t.Fatalf("usage tokens wrong: in=%d out=%d", usage.InputTokens, usage.OutputTokens)
	}
}
