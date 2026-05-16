//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSanitizeSystemText_ReplacesOpenCodeIdentity(t *testing.T) {
	// The exact phrase that gets replaced
	input := "You are OpenCode, the best coding agent on the planet."
	result := sanitizeSystemText(input)
	assert.NotContains(t, result, "best coding agent on the planet")
}

func TestSanitizeSystemText_PreservesNormalText(t *testing.T) {
	input := "You are a helpful assistant for coding tasks."
	assert.Equal(t, input, sanitizeSystemText(input))
}

func TestSanitizeSystemText_EmptyInput(t *testing.T) {
	assert.Equal(t, "", sanitizeSystemText(""))
}

func TestIsClaudeCodeCredentialScopeError_Matches(t *testing.T) {
	tests := []struct {
		msg  string
		want bool
	}{
		{"This API key is only authorized for use with Claude Code and cannot be used for other API requests", true},
		{"only authorized for use with claude code and cannot be used for other api requests", true},
		{"rate limit exceeded", false},
		{"only authorized for use with claude code", false}, // needs both parts
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.msg, func(t *testing.T) {
			assert.Equal(t, tt.want, isClaudeCodeCredentialScopeError(tt.msg))
		})
	}
}

func TestBuildJSONArrayRaw_Empty(t *testing.T) {
	result := buildJSONArrayRaw(nil)
	assert.Equal(t, []byte("[]"), result)

	result = buildJSONArrayRaw([][]byte{})
	assert.Equal(t, []byte("[]"), result)
}

func TestBuildJSONArrayRaw_SingleItem(t *testing.T) {
	items := [][]byte{[]byte(`{"key":"value"}`)}
	result := buildJSONArrayRaw(items)
	assert.Equal(t, `[{"key":"value"}]`, string(result))
}

func TestBuildJSONArrayRaw_MultipleItems(t *testing.T) {
	items := [][]byte{
		[]byte(`{"a":1}`),
		[]byte(`{"b":2}`),
		[]byte(`{"c":3}`),
	}
	result := buildJSONArrayRaw(items)
	assert.Equal(t, `[{"a":1},{"b":2},{"c":3}]`, string(result))
}

func TestSetJSONValueBytes_SetString(t *testing.T) {
	body := []byte(`{"model":"old"}`)
	result, ok := setJSONValueBytes(body, "model", "new")
	require.True(t, ok)
	assert.Contains(t, string(result), `"new"`)
}

func TestSetJSONValueBytes_AddNewField(t *testing.T) {
	body := []byte(`{"model":"test"}`)
	result, ok := setJSONValueBytes(body, "temperature", 0.7)
	require.True(t, ok)
	assert.Contains(t, string(result), `"temperature"`)
}

func TestSetJSONRawBytes_InsertRawJSON(t *testing.T) {
	body := []byte(`{"model":"test"}`)
	raw := []byte(`[{"type":"text","text":"hello"}]`)
	result, ok := setJSONRawBytes(body, "system", raw)
	require.True(t, ok)
	assert.Contains(t, string(result), `"system"`)
}

func TestDeleteJSONPathBytes_RemoveField(t *testing.T) {
	body := []byte(`{"model":"test","extra":"remove"}`)
	result, ok := deleteJSONPathBytes(body, "extra")
	require.True(t, ok)
	assert.NotContains(t, string(result), `"extra"`)
	assert.Contains(t, string(result), `"model"`)
}

func TestDeleteJSONPathBytes_NonExistentField(t *testing.T) {
	body := []byte(`{"model":"test"}`)
	result, ok := deleteJSONPathBytes(body, "nonexistent")
	// sjson returns the original if path doesn't exist
	assert.Equal(t, string(body), string(result))
	_ = ok
}

func TestMarshalAnthropicSystemTextBlock_WithCache(t *testing.T) {
	result, err := marshalAnthropicSystemTextBlock("Hello world", true)
	require.NoError(t, err)
	assert.Contains(t, string(result), `"text":"Hello world"`)
	assert.Contains(t, string(result), `"cache_control"`)
}

func TestMarshalAnthropicSystemTextBlock_WithoutCache(t *testing.T) {
	result, err := marshalAnthropicSystemTextBlock("Hello world", false)
	require.NoError(t, err)
	assert.Contains(t, string(result), `"text":"Hello world"`)
	assert.NotContains(t, string(result), `"cache_control"`)
}

func TestMarshalAnthropicMetadata(t *testing.T) {
	result, err := marshalAnthropicMetadata("user-123")
	require.NoError(t, err)
	assert.Contains(t, string(result), `"user_id":"user-123"`)
}
