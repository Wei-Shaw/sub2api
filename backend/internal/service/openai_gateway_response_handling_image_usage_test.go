package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tidwall/gjson"
)

func TestExtractOpenAIUsageFromJSONBytes_MergesHostedImageGenToolUsage(t *testing.T) {
	// SSE response.completed event with response.usage + response.tool_usage.image_gen
	body := []byte(`{
		"type": "response.completed",
		"response": {
			"usage": {
				"input_tokens": 43792,
				"output_tokens": 1005,
				"total_tokens": 44797
		REDACTED,
			"tool_usage": {
				"image_gen": {
					"input_tokens": 7918,
					"input_tokens_details": {"image_tokens": 7620, "text_tokens": 298REDACTED,
					"output_tokens": 186,
					"output_tokens_details": {"image_tokens": 186, "text_tokens": 0REDACTED,
					"total_tokens": 8104
			REDACTED
		REDACTED
	REDACTED
REDACTED`)

	usage, ok := extractOpenAIUsageFromJSONBytes(body)
	assert.True(t, ok)
	assert.Equal(t, 43792, usage.InputTokens, "input_tokens from response.usage")
	assert.Equal(t, 1005, usage.OutputTokens, "output_tokens from response.usage")
	assert.Equal(t, 186, usage.ImageOutputTokens, "image output tokens merged from tool_usage.image_gen")
	assert.Equal(t, 7620, usage.ImageInputTokens, "image input tokens merged from tool_usage.image_gen")
REDACTED

func TestExtractOpenAIUsageFromJSONBytes_NonStreamingMergesImageGen(t *testing.T) {
	// Non-streaming response with top-level usage + tool_usage
	body := []byte(`{
		"id": "resp_abc123",
		"object": "response",
		"usage": {
			"input_tokens": 5000,
			"output_tokens": 200
	REDACTED,
		"tool_usage": {
			"image_gen": {
				"input_tokens": 3000,
				"input_tokens_details": {"image_tokens": 2800, "text_tokens": 200REDACTED,
				"output_tokens": 150,
				"output_tokens_details": {"image_tokens": 150, "text_tokens": 0REDACTED,
				"total_tokens": 3150
		REDACTED
	REDACTED
REDACTED`)

	usage, ok := extractOpenAIUsageFromJSONBytes(body)
	assert.True(t, ok)
	assert.Equal(t, 5000, usage.InputTokens)
	assert.Equal(t, 200, usage.OutputTokens)
	assert.Equal(t, 150, usage.ImageOutputTokens, "image output tokens from tool_usage.image_gen")
	assert.Equal(t, 2800, usage.ImageInputTokens, "image input tokens from tool_usage.image_gen")
REDACTED

func TestExtractOpenAIUsageFromJSONBytes_NoToolUsageUnchanged(t *testing.T) {
	// Standard response without tool_usage — behavior should be unchanged
	body := []byte(`{
		"usage": {
			"input_tokens": 100,
			"output_tokens": 50
	REDACTED
REDACTED`)

	usage, ok := extractOpenAIUsageFromJSONBytes(body)
	assert.True(t, ok)
	assert.Equal(t, 100, usage.InputTokens)
	assert.Equal(t, 50, usage.OutputTokens)
	assert.Equal(t, 0, usage.ImageOutputTokens, "no image tokens without tool_usage")
	assert.Equal(t, 0, usage.ImageInputTokens, "no image tokens without tool_usage")
REDACTED

func TestExtractOpenAIUsageFromJSONBytes_BaseUsageHasImageTokensNoOverride(t *testing.T) {
	// If base usage already has image tokens (e.g. from output_tokens_details),
	// tool_usage should NOT override them.
	body := []byte(`{
		"usage": {
			"input_tokens": 100,
			"output_tokens": 50,
			"output_tokens_details": {"image_tokens": 30REDACTED
	REDACTED,
		"tool_usage": {
			"image_gen": {
				"input_tokens": 200,
				"output_tokens": 100,
				"output_tokens_details": {"image_tokens": 100, "text_tokens": 0REDACTED,
				"total_tokens": 300
		REDACTED
	REDACTED
REDACTED`)

	usage, ok := extractOpenAIUsageFromJSONBytes(body)
	assert.True(t, ok)
	assert.Equal(t, 30, usage.ImageOutputTokens, "base usage image tokens preserved, not overridden")
REDACTED

func TestMergeHostedImageGenToolUsage_EmptyImageGen(t *testing.T) {
	tests := []struct {
		name string
		json string
REDACTED{
		{"missing", `{REDACTED`REDACTED,
		{"null", `{"image_gen": nullREDACTED`REDACTED,
		{"not object", `{"image_gen": 42REDACTED`REDACTED,
REDACTED
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			usage := OpenAIUsage{InputTokens: 100, OutputTokens: 50REDACTED
			original := usage
			body := []byte(tt.json)
			mergeHostedImageGenToolUsage(gjson.GetBytes(body, "image_gen"), &usage)
			assert.Equal(t, original, usage, "usage should be unchanged")
	REDACTED)
REDACTED
REDACTED

func TestParseSSEUsageBytes_ResponseCompletedWithImageGen(t *testing.T) {
	svc := &OpenAIGatewayService{REDACTED
	data := []byte(`{
		"type": "response.completed",
		"response": {
			"usage": {
				"input_tokens": 10000,
				"output_tokens": 500
		REDACTED,
			"tool_usage": {
				"image_gen": {
					"input_tokens": 4000,
					"input_tokens_details": {"image_tokens": 3800, "text_tokens": 200REDACTED,
					"output_tokens": 186,
					"output_tokens_details": {"image_tokens": 186, "text_tokens": 0REDACTED,
					"total_tokens": 4186
			REDACTED
		REDACTED
	REDACTED
REDACTED`)

	usage := &OpenAIUsage{REDACTED
	svc.parseSSEUsageBytes(data, usage)

	assert.Equal(t, 10000, usage.InputTokens)
	assert.Equal(t, 500, usage.OutputTokens)
	assert.Equal(t, 186, usage.ImageOutputTokens, "image output tokens from SSE tool_usage")
	assert.Equal(t, 3800, usage.ImageInputTokens, "image input tokens from SSE tool_usage")
REDACTED
