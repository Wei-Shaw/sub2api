package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsExplicitImageGenerationIntent_IgnoresPassiveNamespace(t *testing.T) {
	body := []byte(`{
		"model": "gpt-5.5",
		"input": [{"type":"message","role":"user","content":"hello"REDACTED],
		"tools": [
			{"type":"function","name":"Read"REDACTED,
			{"type":"namespace","name":"image_gen","tools":[{"type":"function","name":"imagegen"REDACTED]REDACTED
		],
		"tool_choice": "auto"
REDACTED`)

	assert.False(t, IsExplicitImageGenerationIntent("/v1/responses", "gpt-5.5", body),
		"passive image_gen namespace should NOT be explicit image intent")

	assert.True(t, IsImageGenerationIntent("/v1/responses", "gpt-5.5", body),
		"passive image_gen namespace SHOULD be general image intent (for permission check)")
REDACTED

func TestIsExplicitImageGenerationIntent_DetectsNativeTool(t *testing.T) {
	body := []byte(`{
		"model": "gpt-5.5",
		"tools": [{"type":"image_generation","model":"gpt-image-2"REDACTED],
		"tool_choice": "auto"
REDACTED`)

	assert.True(t, IsExplicitImageGenerationIntent("/v1/responses", "gpt-5.5", body),
		"native image_generation tool IS explicit intent")
REDACTED

func TestIsExplicitImageGenerationIntent_DetectsImageModel(t *testing.T) {
	assert.True(t, IsExplicitImageGenerationIntent("/v1/responses", "gpt-image-2", nil),
		"image model IS explicit intent")
REDACTED

func TestIsExplicitImageGenerationIntent_DetectsImageEndpoint(t *testing.T) {
	assert.True(t, IsExplicitImageGenerationIntent("/v1/images/generations", "gpt-5.5", nil),
		"image endpoint IS explicit intent")
REDACTED

func TestIsExplicitImageGenerationIntent_DetectsExplicitToolChoice(t *testing.T) {
	body := []byte(`{"model":"gpt-5.5","tools":[{"type":"function","name":"Read"REDACTED],"tool_choice":"image_generation"REDACTED`)
	assert.True(t, IsExplicitImageGenerationIntent("/v1/responses", "gpt-5.5", body),
		"explicit tool_choice selecting image_generation IS explicit intent")
REDACTED

func TestIsExplicitImageGenerationIntent_PlainTextRequest(t *testing.T) {
	body := []byte(`{
		"model": "gpt-5.5",
		"input": "hello",
		"tools": [{"type":"function","name":"Read"REDACTED]
REDACTED`)

	assert.False(t, IsExplicitImageGenerationIntent("/v1/responses", "gpt-5.5", body),
		"plain text request should NOT be explicit image intent")
REDACTED
