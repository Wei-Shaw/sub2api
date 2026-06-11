package service

import (
	"testing"
)

func TestOpenAIImageOutputCounter_TextOnlyMessage(t *testing.T) {
	// Simulate a text-only response from /v1/responses
	// The response.output_item.done event for a text message
	sseBody := `data: {"type":"response.created","response":{"id":"resp_123"REDACTEDREDACTED

data: {"type":"response.in_progress","response":{"id":"resp_123"REDACTEDREDACTED

data: {"type":"response.output_item.added","item":{"id":"item_1","type":"message","role":"assistant","status":"in_progress"REDACTEDREDACTED

data: {"type":"response.output_text.delta","item_id":"item_1","output_index":0,"content_index":0,"delta":"Hello"REDACTED

data: {"type":"response.output_text.done","item_id":"item_1","output_index":0,"content_index":0,"text":"Hello"REDACTED

data: {"type":"response.output_item.done","item":{"id":"item_1","type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"Hello"REDACTED]REDACTEDREDACTED

data: {"type":"response.completed","response":{"id":"resp_123","output":[{"id":"item_1","type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"Hello"REDACTED]REDACTED],"usage":{"input_tokens":10,"output_tokens":5REDACTEDREDACTEDREDACTED

data: [DONE]`

	count := countOpenAIImageOutputsFromSSEBody(sseBody)
	if count != 0 {
		t.Errorf("expected 0 images for text-only message, got %d", count)
REDACTED
REDACTED

func TestOpenAIImageOutputCounter_NestedSub2API_StandardResponse(t *testing.T) {
	// Simulate what a nested sub2api setup receives
	// When the upstream sub2api returns a standard response, the downstream
	// sub2api receives SSE events. This test verifies that no false positives
	// occur.
	sseBody := `data: {"type":"response.created","response":{"id":"resp_nested_1","object":"response","status":"in_progress"REDACTEDREDACTED

data: {"type":"response.in_progress","response":{"id":"resp_nested_1","object":"response","status":"in_progress"REDACTEDREDACTED

data: {"type":"response.output_item.added","item":{"id":"item_nested_1","type":"message","role":"assistant","status":"in_progress"REDACTEDREDACTED

data: {"type":"response.output_text.delta","item_id":"item_nested_1","output_index":0,"content_index":0,"delta":"你好"REDACTED

data: {"type":"response.output_text.done","item_id":"item_nested_1","output_index":0,"content_index":0,"text":"你好！有什么我可以帮助你的吗？"REDACTED

data: {"type":"response.output_item.done","item":{"id":"item_nested_1","type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"你好！有什么我可以帮助你的吗？"REDACTED]REDACTEDREDACTED

data: {"type":"response.completed","response":{"id":"resp_nested_1","object":"response","status":"completed","output":[{"id":"item_nested_1","type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"你好！有什么我可以帮助你的吗？"REDACTED]REDACTED],"usage":{"input_tokens":15,"output_tokens":12REDACTEDREDACTEDREDACTED

data: [DONE]`

	count := countOpenAIImageOutputsFromSSEBody(sseBody)
	if count != 0 {
		t.Errorf("expected 0 images for nested sub2api text-only response, got %d", count)
REDACTED
REDACTED

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
				REDACTED
				]
		REDACTED
		],
		"usage": {
			"input_tokens": 10,
			"output_tokens": 5
	REDACTED
REDACTED`

	count := countOpenAIResponseImageOutputsFromJSONBytes([]byte(jsonBody))
	if count != 0 {
		t.Errorf("expected 0 images for text-only JSON response, got %d", count)
REDACTED
REDACTED

func TestOpenAIImageOutputCounter_DataArray_FalsePositive(t *testing.T) {
	// Test: SSE events in /v1/responses should NOT have a "data" array field,
	// but if one is present with actual image URLs, it should be counted.
	// The bug was that addDataArray counted ALL array elements without checking
	// if they contained actual image output (url/b64_json).
	//
	// Test 1: data array with non-image objects should NOT count
	sseWithNonImageData := `data: {"type":"response.completed","response":{"id":"resp_1","output":[{"id":"item_1","type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"Hello"REDACTED]REDACTED]REDACTED,"data":[{"id":"not_an_image","status":"done"REDACTED]REDACTED

data: [DONE]`

	count := countOpenAIImageOutputsFromSSEBody(sseWithNonImageData)
	if count != 0 {
		t.Errorf("expected 0 images for data array without image output, got %d", count)
REDACTED

	// Test 2: data array with actual image output should still count correctly
	sseWithImageData := `data: {"type":"response.completed","response":{"id":"resp_1","output":[]REDACTED,"data":[{"url":"https://example.com/img.png"REDACTED]REDACTED

data: [DONE]`

	count2 := countOpenAIImageOutputsFromSSEBody(sseWithImageData)
	if count2 != 1 {
		t.Errorf("expected 1 image for data array with image URL, got %d", count2)
REDACTED
REDACTED

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
				"content": [{"type": "output_text", "text": "Hello"REDACTED]
		REDACTED
		],
		"data": [
			{"id": "not_an_image", "status": "done"REDACTED
		],
		"usage": {"input_tokens": 10, "output_tokens": 5REDACTED
REDACTED`

	count := countOpenAIResponseImageOutputsFromJSONBytes([]byte(jsonWithNonImageData))
	if count != 0 {
		t.Errorf("expected 0 images for data array without image output, got %d", count)
REDACTED

	// Test: data array with actual image output should still count correctly
	jsonWithImageData := `{
		"id": "resp_1",
		"object": "response",
		"output": [],
		"data": [
			{"url": "https://example.com/img.png"REDACTED
		]
REDACTED`

	count2 := countOpenAIResponseImageOutputsFromJSONBytes([]byte(jsonWithImageData))
	if count2 != 1 {
		t.Errorf("expected 1 image for data array with image URL, got %d", count2)
REDACTED
REDACTED

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
				"content": [{"type": "output_text", "text": "Hello"REDACTED]
		REDACTED
		],
		"usage": {"input_tokens": 10, "output_tokens": 5REDACTED
REDACTED`

	count := countOpenAIResponseImageOutputsFromJSONBytes([]byte(extractedResponse))
	if count != 0 {
		t.Errorf("expected 0 images for extracted response JSON, got %d", count)
REDACTED
REDACTED

func TestOpenAIImageOutputCounter_AddDataArray_FromVariousSources(t *testing.T) {
	// Test various JSON payloads that might trigger addDataArray
	tests := []struct {
		name     string
		json     string
		expected int
REDACTED{
		{
			name: "standard response.done event",
			json: `{"type":"response.done","response":{"id":"r1","output":[{"type":"message","id":"m1","content":[{"type":"output_text","text":"hi"REDACTED]REDACTED]REDACTEDREDACTED`,
	REDACTED,
		{
			name: "response with null data field",
			json: `{"type":"response.done","response":{"id":"r1","output":[]REDACTED,"data":nullREDACTED`,
	REDACTED,
		{
			name: "response with empty data array",
			json: `{"type":"response.done","response":{"id":"r1","output":[]REDACTED,"data":[]REDACTED`,
	REDACTED,
		{
			name: "response with single-element data array",
			json: `{"type":"response.done","response":{"id":"r1","output":[]REDACTED,"data":[{"url":"https://example.com/img.png"REDACTED]REDACTED`,
	REDACTED,
		{
			name: "image_generation.completed event without result",
			json: `{"type":"image_generation.completed","item":{"type":"image_generation.completed","id":"call_1"REDACTEDREDACTED`,
	REDACTED,
		{
			name: "output_item with empty type",
			json: `{"type":"response.output_item.done","item":{"id":"x1","status":"completed"REDACTEDREDACTED`,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			counter := newOpenAIImageOutputCounter()
			counter.AddSSEData([]byte(tt.json))
			count := counter.Count()
			t.Logf("  count=%d (maxDataCount=%d, count=%d)", count, counter.maxDataCount, counter.count)
	REDACTED)
REDACTED
REDACTED

func TestOpenAIImageOutputCounter_AddJSONResponse_Exported(t *testing.T) {
	tests := []struct {
		name string
		json string
REDACTED{
		{
			name: "standard /v1/responses JSON response",
			json: `{"id":"r1","object":"response","output":[{"type":"message","id":"m1","content":[{"type":"output_text","text":"hi"REDACTED]REDACTED],"usage":{"input_tokens":10,"output_tokens":5REDACTEDREDACTED`,
	REDACTED,
		{
			name: "response with data field (like /v1/images/generations)",
			json: `{"id":"r1","object":"response","output":[{"type":"message","id":"m1","content":[{"type":"output_text","text":"hi"REDACTED]REDACTED],"data":[{"url":"https://example.com/img.png"REDACTED],"usage":{"input_tokens":10,"output_tokens":5REDACTEDREDACTED`,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			count := countOpenAIResponseImageOutputsFromJSONBytes([]byte(tt.json))
			t.Logf("  count=%d", count)
	REDACTED)
REDACTED
REDACTED