package service

import (
	"testing"
)

func TestOpenAIImageOutputCounter_TextOnlyMessage(t *testing.T) {
	// Simulate a text-only response from /v1/responses
	// The response.output_item.done event for a text message
	sseBody := `data: {"type":"response.created","response":{"id":"resp_123"}}

data: {"type":"response.in_progress","response":{"id":"resp_123"}}

data: {"type":"response.output_item.added","item":{"id":"item_1","type":"message","role":"assistant","status":"in_progress"}}

data: {"type":"response.output_text.delta","item_id":"item_1","output_index":0,"content_index":0,"delta":"Hello"}

data: {"type":"response.output_text.done","item_id":"item_1","output_index":0,"content_index":0,"text":"Hello"}

data: {"type":"response.output_item.done","item":{"id":"item_1","type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"Hello"}]}}

data: {"type":"response.completed","response":{"id":"resp_123","output":[{"id":"item_1","type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"Hello"}]}],"usage":{"input_tokens":10,"output_tokens":5}}}

data: [DONE]`

	count := countOpenAIImageOutputsFromSSEBody(sseBody)
	if count != 0 {
		t.Errorf("expected 0 images for text-only message, got %d", count)
	}
}

func TestOpenAIImageOutputCounter_NestedSub2API_StandardResponse(t *testing.T) {
	// Simulate what a nested sub2api setup receives
	// When the upstream sub2api returns a standard response, the downstream
	// sub2api receives SSE events. This test verifies that no false positives
	// occur.
	sseBody := `data: {"type":"response.created","response":{"id":"resp_nested_1","object":"response","status":"in_progress"}}

data: {"type":"response.in_progress","response":{"id":"resp_nested_1","object":"response","status":"in_progress"}}

data: {"type":"response.output_item.added","item":{"id":"item_nested_1","type":"message","role":"assistant","status":"in_progress"}}

data: {"type":"response.output_text.delta","item_id":"item_nested_1","output_index":0,"content_index":0,"delta":"你好"}

data: {"type":"response.output_text.done","item_id":"item_nested_1","output_index":0,"content_index":0,"text":"你好！有什么我可以帮助你的吗？"}

data: {"type":"response.output_item.done","item":{"id":"item_nested_1","type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"你好！有什么我可以帮助你的吗？"}]}}

data: {"type":"response.completed","response":{"id":"resp_nested_1","object":"response","status":"completed","output":[{"id":"item_nested_1","type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"你好！有什么我可以帮助你的吗？"}]}],"usage":{"input_tokens":15,"output_tokens":12}}}

data: [DONE]`

	count := countOpenAIImageOutputsFromSSEBody(sseBody)
	if count != 0 {
		t.Errorf("expected 0 images for nested sub2api text-only response, got %d", count)
	}
}

func TestOpenAIImageOutputCounter_JSONResponse_TextOnly(t *testing.T) {
	// Simulate JSON response (non-streaming) for text-only message
	jsonBody := `{
		"id": "resp_json_1",
		"object": "response",
		"status": "completed",
		"output": [
			{
				"id": "item_json_1",
				"type": "message",
				"role": "assistant",
				"status": "completed",
				"content": [
					{
						"type": "output_text",
						"text": "Hello!"
					}
				]
			}
		],
		"usage": {
			"input_tokens": 10,
			"output_tokens": 5
		}
	}`

	count := countOpenAIResponseImageOutputsFromJSONBytes([]byte(jsonBody))
	if count != 0 {
		t.Errorf("expected 0 images for text-only JSON response, got %d", count)
	}
}

func TestOpenAIImageOutputCounter_DataArray_FalsePositive(t *testing.T) {
	// Test: SSE events in /v1/responses should NOT have a "data" array field,
	// but if one is present with actual image URLs, it should be counted.
	// The bug was that addDataArray counted ALL array elements without checking
	// if they contained actual image output (url/b64_json).
	//
	// Test 1: data array with non-image objects should NOT count
	sseWithNonImageData := `data: {"type":"response.completed","response":{"id":"resp_1","output":[{"id":"item_1","type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"Hello"}]}]},"data":[{"id":"not_an_image","status":"done"}]}

data: [DONE]`

	count := countOpenAIImageOutputsFromSSEBody(sseWithNonImageData)
	if count != 0 {
		t.Errorf("expected 0 images for data array without image output, got %d", count)
	}

	// Test 2: data array with actual image output should still count correctly
	sseWithImageData := `data: {"type":"response.completed","response":{"id":"resp_1","output":[]},"data":[{"url":"https://example.com/img.png"}]}

data: [DONE]`

	count2 := countOpenAIImageOutputsFromSSEBody(sseWithImageData)
	if count2 != 1 {
		t.Errorf("expected 1 image for data array with image URL, got %d", count2)
	}
}

func TestOpenAIImageOutputCounter_JSONResponse_DataArray_FalsePositive(t *testing.T) {
	// Test: JSON responses in /v1/responses should NOT have a "data" array field.
	// But if one is present with non-image objects, it should NOT cause false counting.
	jsonWithNonImageData := `{
		"id": "resp_1",
		"object": "response",
		"status": "completed",
		"output": [
			{
				"id": "item_1",
				"type": "message",
				"role": "assistant",
				"status": "completed",
				"content": [{"type": "output_text", "text": "Hello"}]
			}
		],
		"data": [
			{"id": "not_an_image", "status": "done"}
		],
		"usage": {"input_tokens": 10, "output_tokens": 5}
	}`

	count := countOpenAIResponseImageOutputsFromJSONBytes([]byte(jsonWithNonImageData))
	if count != 0 {
		t.Errorf("expected 0 images for data array without image output, got %d", count)
	}

	// Test: data array with actual image output should still count correctly
	jsonWithImageData := `{
		"id": "resp_1",
		"object": "response",
		"output": [],
		"data": [
			{"url": "https://example.com/img.png"}
		]
	}`

	count2 := countOpenAIResponseImageOutputsFromJSONBytes([]byte(jsonWithImageData))
	if count2 != 1 {
		t.Errorf("expected 1 image for data array with image URL, got %d", count2)
	}
}

func TestOpenAIImageOutputCounter_PassthroughSSEToJSON_Conversion(t *testing.T) {
	// Simulate the scenario in handlePassthroughSSEToJSON where extractCodexFinalResponse
	// successfully extracts the response JSON, then countOpenAIResponseImageOutputsFromJSONBytes
	// is called on the extracted response object.
	//
	// The extracted response is just the "response" field from the "response.done" event.
	// This should NOT have a "data" array.
	extractedResponse := `{
		"id": "resp_extracted_1",
		"object": "response",
		"status": "completed",
		"output": [
			{
				"id": "item_ext_1",
				"type": "message",
				"role": "assistant",
				"status": "completed",
				"content": [{"type": "output_text", "text": "Hello"}]
			}
		],
		"usage": {"input_tokens": 10, "output_tokens": 5}
	}`

	count := countOpenAIResponseImageOutputsFromJSONBytes([]byte(extractedResponse))
	if count != 0 {
		t.Errorf("expected 0 images for extracted response JSON, got %d", count)
	}
}

func TestOpenAIImageOutputCounter_AddDataArray_FromVariousSources(t *testing.T) {
	// Test various JSON payloads that might trigger addDataArray
	tests := []struct {
		name     string
		json     string
		expected int
	}{
		{
			name: "standard response.done event",
			json: `{"type":"response.done","response":{"id":"r1","output":[{"type":"message","id":"m1","content":[{"type":"output_text","text":"hi"}]}]}}`,
		},
		{
			name: "response with null data field",
			json: `{"type":"response.done","response":{"id":"r1","output":[]},"data":null}`,
		},
		{
			name: "response with empty data array",
			json: `{"type":"response.done","response":{"id":"r1","output":[]},"data":[]}`,
		},
		{
			name: "response with single-element data array",
			json: `{"type":"response.done","response":{"id":"r1","output":[]},"data":[{"url":"https://example.com/img.png"}]}`,
		},
		{
			name: "image_generation.completed event without result",
			json: `{"type":"image_generation.completed","item":{"type":"image_generation.completed","id":"call_1"}}`,
		},
		{
			name: "output_item with empty type",
			json: `{"type":"response.output_item.done","item":{"id":"x1","status":"completed"}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			counter := newOpenAIImageOutputCounter()
			counter.AddSSEData([]byte(tt.json))
			count := counter.Count()
			t.Logf("  count=%d (maxDataCount=%d, count=%d)", count, counter.maxDataCount, counter.count)
		})
	}
}

func TestOpenAIImageOutputCounter_AddJSONResponse_Exported(t *testing.T) {
	tests := []struct {
		name string
		json string
	}{
		{
			name: "standard /v1/responses JSON response",
			json: `{"id":"r1","object":"response","output":[{"type":"message","id":"m1","content":[{"type":"output_text","text":"hi"}]}],"usage":{"input_tokens":10,"output_tokens":5}}`,
		},
		{
			name: "response with data field (like /v1/images/generations)",
			json: `{"id":"r1","object":"response","output":[{"type":"message","id":"m1","content":[{"type":"output_text","text":"hi"}]}],"data":[{"url":"https://example.com/img.png"}],"usage":{"input_tokens":10,"output_tokens":5}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			count := countOpenAIResponseImageOutputsFromJSONBytes([]byte(tt.json))
			t.Logf("  count=%d", count)
		})
	}
}
