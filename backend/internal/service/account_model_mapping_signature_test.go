//go:build unit

package service

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestModelMappingSignature_IndependentOfInsertionOrder(t *testing.T) {
	const size = 64
	forward := make(map[string]any, size)
	backward := make(map[string]any, size)
	for i := 0; i < size; i++ {
		forward[fmt.Sprintf("model-%02d", i)] = fmt.Sprintf("upstream-%02d", i)
	}
	for i := size - 1; i >= 0; i-- {
		backward[fmt.Sprintf("model-%02d", i)] = fmt.Sprintf("upstream-%02d", i)
	}
	forward["non-string"] = 42
	backward["non-string"] = 42

	want := modelMappingSignature(forward)
	require.NotZero(t, want)
	for i := 0; i < 20; i++ {
		require.Equal(t, want, modelMappingSignature(forward))
		require.Equal(t, want, modelMappingSignature(backward))
	}
}

func TestModelMappingSignature_DetectsContentChanges(t *testing.T) {
	base := map[string]any{
		"claude-sonnet": "sonnet-a",
		"claude-opus":   "opus-a",
		"gpt-5":         "gpt-5-upstream",
	}
	baseSig := modelMappingSignature(base)

	cases := map[string]map[string]any{
		"value changed": {
			"claude-sonnet": "sonnet-b",
			"claude-opus":   "opus-a",
			"gpt-5":         "gpt-5-upstream",
		},
		"key renamed": {
			"claude-sonnet-x": "sonnet-a",
			"claude-opus":     "opus-a",
			"gpt-5":           "gpt-5-upstream",
		},
		"values swapped between keys": {
			"claude-sonnet": "opus-a",
			"claude-opus":   "sonnet-a",
			"gpt-5":         "gpt-5-upstream",
		},
		"string value replaced by non-string": {
			"claude-sonnet": 1,
			"claude-opus":   "opus-a",
			"gpt-5":         "gpt-5-upstream",
		},
		"entry removed": {
			"claude-sonnet": "sonnet-a",
			"claude-opus":   "opus-a",
		},
	}
	for name, changed := range cases {
		require.NotEqual(t, baseSig, modelMappingSignature(changed), name)
	}

	// 非 string 的 value 统一按类型标记参与签名，具体取值不影响结果。
	require.Equal(t,
		modelMappingSignature(map[string]any{"m": 1}),
		modelMappingSignature(map[string]any{"m": true}),
	)
}

func TestModelMappingSignature_EmptyIsZero(t *testing.T) {
	require.Zero(t, modelMappingSignature(nil))
	require.Zero(t, modelMappingSignature(map[string]any{}))
}

func TestModelMappingSignature_DoesNotAllocate(t *testing.T) {
	mapping := map[string]any{
		"claude-sonnet": "sonnet-a",
		"claude-opus":   "opus-a",
		"gpt-5":         "gpt-5-upstream",
		"non-string":    3.14,
	}
	var sink uint64
	allocs := testing.AllocsPerRun(1000, func() {
		sink += modelMappingSignature(mapping)
	})
	require.Zero(t, allocs)
	require.NotZero(t, sink)
}
