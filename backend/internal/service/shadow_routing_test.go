package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDefaultSparkShadowModelMapping(t *testing.T) {
	mapping := defaultSparkShadowModelMapping()

	require.Len(t, mapping, 1, "spark 无 effort 变体，默认只含 base 模型")
	require.Equal(t, "gpt-5.3-codex-spark", mapping["gpt-5.3-codex-spark"], "恒等映射：base 映射到自身")
}

func TestSparkModelVariantsDerivedFromAliases(t *testing.T) {
	got := sparkModelVariants()
	require.ElementsMatch(t, []string{
		"gpt-5.3-codex-spark",
	}, got, "spark 只有 base：effort 变体不存在，已从 codexModelMap 移除")
}
