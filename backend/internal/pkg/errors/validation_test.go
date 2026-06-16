package errors

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFieldErrorCollector_NoErrorsBuildsNil(t *testing.T) {
	t.Parallel()
	c := NewFieldErrorCollector("X")
	require.False(t, c.HasErrors())
	require.Nil(t, c.Build(), "无字段错误时 Build() 必须返回 nil，让调用方 if err != nil 惯性保持")
}

func TestFieldErrorCollector_AddBuildsBadRequest(t *testing.T) {
	t.Parallel()
	c := NewFieldErrorCollector("MY_REASON")
	c.Add("name", "REQUIRED")
	c.Add("limit_value", "MUST_BE_POSITIVE")
	require.True(t, c.HasErrors())

	err := c.Build()
	require.NotNil(t, err)
	require.Equal(t, int32(http.StatusBadRequest), err.Code)
	require.Equal(t, "MY_REASON", err.Reason)
	require.Equal(t, "2", err.Metadata["count"])
	require.JSONEq(t,
		`[{"path":"name","code":"REQUIRED"},{"path":"limit_value","code":"MUST_BE_POSITIVE"}]`,
		err.Metadata["fields"],
		"metadata.fields 必须是 FieldError 列表的 JSON 字符串，前端 JSON.parse 后渲染")
}

func TestFieldErrorCollector_WithMessageOverridesDefault(t *testing.T) {
	t.Parallel()
	c := NewFieldErrorCollector("X").WithMessage("custom english message")
	c.Add("a", "B")
	err := c.Build()
	require.Equal(t, "custom english message", err.Message)
}

func TestFieldErrorCollector_DefaultMessage(t *testing.T) {
	t.Parallel()
	c := NewFieldErrorCollector("X")
	c.Add("a", "B")
	err := c.Build()
	require.Equal(t, "validation failed", err.Message,
		"未调 WithMessage 时使用默认 message")
}

func TestValidationFailed_DirectConstructor(t *testing.T) {
	t.Parallel()
	fields := []FieldError{
		{Path: "limiters[0].limit_value", Code: "MUST_BE_POSITIVE"},
		{Path: "paths[0].platform", Code: "PLATFORM_REQUIRED"},
	}
	err := ValidationFailed("INVALID_REQUEST_BODY", fields)
	require.Equal(t, int32(http.StatusBadRequest), err.Code)
	require.Equal(t, "INVALID_REQUEST_BODY", err.Reason)
	require.Equal(t, "2", err.Metadata["count"])

	// 反序列化 metadata.fields 验证内容完整。
	var got []FieldError
	require.NoError(t, json.Unmarshal([]byte(err.Metadata["fields"]), &got))
	require.Equal(t, fields, got)
}

// TestValidationFailed_EmptyFieldsStillProducesError 验证空 fields 仍构造非 nil 错误。
//
// 理由：调用方用便利构造器时通常已经 sure 有错误（例如 BindJSONOrError 检测到 validator 错），
// 传空切片是 caller 编程错误，应该收到一个有意义的返回（reason 仍然是 INVALID_REQUEST_BODY），
// 而不是 nil（会让 caller 误以为通过）。
func TestValidationFailed_EmptyFieldsStillProducesError(t *testing.T) {
	t.Parallel()
	err := ValidationFailed("X", nil)
	require.NotNil(t, err)
	require.Equal(t, "X", err.Reason)
	require.Equal(t, "0", err.Metadata["count"])
	// nil slice 经 json.Marshal 序列化为 "null"（不是 "[]"），这是 Go 标准库行为。
	// 对前端而言 null 与 [] 都意味着"无具体字段错误",前端 JSON.parse 后判断 Array.isArray 即可。
	require.Equal(t, "null", err.Metadata["fields"])
}

// TestFieldErrorCollector_IsAndCloneSafety 验证 Build() 返回的错误能被 errors.Is 识别（与同 reason+code 比较）。
// 这是 Clone 内部正确传递 Code/Reason 的验收。
func TestFieldErrorCollector_IsAndCloneSafety(t *testing.T) {
	t.Parallel()
	c := NewFieldErrorCollector("MY")
	c.Add("a", "B")
	err1 := c.Build()
	err2 := ValidationFailed("MY", []FieldError{{Path: "a", Code: "B"}})
	// Is 比较 Code + Reason，两者应等价。
	require.True(t, err1.Is(err2))
}
