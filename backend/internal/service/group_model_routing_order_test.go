//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGroupGetRoutingAccountIDsPrefersMostSpecificPattern(t *testing.T) {
	g := &Group{
		ModelRoutingEnabled: true,
		ModelRouting: map[string][]int64{
			"*":                 {1},
			"claude-*":          {2},
			"claude-opus-*":     {3},
			"claude-opus-4-*":   {4},
			"claude-sonnet-4-*": {},
			"gpt-*":             {5},
			"claude-opus-4-1":   {6},
		},
	}

	// Go map 遍历顺序随机：重复多次才能暴露按遍历顺序取第一个命中的旧行为。
	for i := 0; i < 1000; i++ {
		require.Equal(t, []int64{6}, g.GetRoutingAccountIDs("claude-opus-4-1"), "exact match wins")
		require.Equal(t, []int64{4}, g.GetRoutingAccountIDs("claude-opus-4-20250514"))
		require.Equal(t, []int64{3}, g.GetRoutingAccountIDs("claude-opus-3"))
		require.Equal(t, []int64{2}, g.GetRoutingAccountIDs("claude-sonnet-4-5"), "rules without accounts are skipped")
		require.Equal(t, []int64{5}, g.GetRoutingAccountIDs("gpt-5"))
		require.Equal(t, []int64{1}, g.GetRoutingAccountIDs("gemini-2.5-pro"))
	}
}
