package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// 自适应降档：按目标上游模型的 supported_reasoning_levels，把不支持的档位降到
// 支持列表里不超过请求的最高档。只降不升，无元数据时原样透传。
func TestApplyReasoningEffortStepdown(t *testing.T) {
	sparkLevels := []string{"minimal", "low", "medium", "high", "xhigh"}

	t.Run("max steps down to xhigh on muse-spark", func(t *testing.T) {
		got, changed, err := ApplyReasoningEffortStepdown(
			[]byte(`{"model":"m","reasoning":{"effort":"max"}}`), sparkLevels)
		require.NoError(t, err)
		require.True(t, changed)
		require.JSONEq(t, `{"model":"m","reasoning":{"effort":"xhigh"}}`, string(got))
	})

	t.Run("supported effort untouched", func(t *testing.T) {
		got, changed, err := ApplyReasoningEffortStepdown(
			[]byte(`{"model":"m","reasoning":{"effort":"xhigh"}}`), sparkLevels)
		require.NoError(t, err)
		require.False(t, changed)
		require.JSONEq(t, `{"model":"m","reasoning":{"effort":"xhigh"}}`, string(got))
	})

	t.Run("below floor stays untouched (down only)", func(t *testing.T) {
		got, changed, err := ApplyReasoningEffortStepdown(
			[]byte(`{"model":"m","reasoning":{"effort":"low"}}`), []string{"high"})
		require.NoError(t, err)
		require.False(t, changed)
		require.JSONEq(t, `{"model":"m","reasoning":{"effort":"low"}}`, string(got))
	})

	t.Run("ultra outranks max", func(t *testing.T) {
		got, changed, err := ApplyReasoningEffortStepdown(
			[]byte(`{"model":"m","reasoning":{"effort":"ultra"}}`),
			[]string{"low", "medium", "high", "xhigh", "max"})
		require.NoError(t, err)
		require.True(t, changed)
		require.JSONEq(t, `{"model":"m","reasoning":{"effort":"max"}}`, string(got))
	})

	t.Run("no metadata passes through", func(t *testing.T) {
		got, changed, err := ApplyReasoningEffortStepdown(
			[]byte(`{"model":"m","reasoning":{"effort":"max"}}`), nil)
		require.NoError(t, err)
		require.False(t, changed)
		require.JSONEq(t, `{"model":"m","reasoning":{"effort":"max"}}`, string(got))
	})

	t.Run("omitted effort untouched", func(t *testing.T) {
		got, changed, err := ApplyReasoningEffortStepdown(
			[]byte(`{"model":"m"}`), sparkLevels)
		require.NoError(t, err)
		require.False(t, changed)
		require.JSONEq(t, `{"model":"m"}`, string(got))
	})

	t.Run("chat completions flat shape", func(t *testing.T) {
		got, changed, err := ApplyReasoningEffortStepdown(
			[]byte(`{"model":"m","reasoning_effort":"max"}`), sparkLevels)
		require.NoError(t, err)
		require.True(t, changed)
		require.JSONEq(t, `{"model":"m","reasoning_effort":"xhigh"}`, string(got))
	})
}

func stepdownTestAccount(levels []string, reasoning bool) *Account {
	return &Account{
		Extra: map[string]any{
			UpstreamModelMetadataExtraKey: map[string]any{
				"models": map[string]any{
					"muse-spark-1.3-contributor": map[string]any{
						"reasoning":                  reasoning,
						"supported_reasoning_levels": levels,
					},
				},
			},
		},
	}
}

func TestApplyReasoningEffortStepdownForAccount(t *testing.T) {
	body := []byte(`{"model":"gpt-6.2-terra","reasoning":{"effort":"max"}}`)

	t.Run("toggle off passes through", func(t *testing.T) {
		ctx := WithReasoningEffortStepdown(context.Background(), false)
		got, changed := applyReasoningEffortStepdownForAccount(ctx, nil,
			stepdownTestAccount([]string{"minimal", "low", "medium", "high", "xhigh"}, true),
			"muse-spark-1.3-contributor", body)
		require.False(t, changed)
		require.JSONEq(t, string(body), string(got))
	})

	t.Run("toggle on steps down to upstream max", func(t *testing.T) {
		ctx := WithReasoningEffortStepdown(context.Background(), true)
		got, changed := applyReasoningEffortStepdownForAccount(ctx, nil,
			stepdownTestAccount([]string{"minimal", "low", "medium", "high", "xhigh"}, true),
			"muse-spark-1.3-contributor", body)
		require.True(t, changed)
		require.JSONEq(t, `{"model":"gpt-6.2-terra","reasoning":{"effort":"xhigh"}}`, string(got))
	})

	t.Run("missing metadata passes through", func(t *testing.T) {
		ctx := WithReasoningEffortStepdown(context.Background(), true)
		got, changed := applyReasoningEffortStepdownForAccount(ctx, nil,
			&Account{}, "muse-spark-1.3-contributor", body)
		require.False(t, changed)
		require.JSONEq(t, string(body), string(got))
	})

	t.Run("non reasoning model passes through", func(t *testing.T) {
		ctx := WithReasoningEffortStepdown(context.Background(), true)
		got, changed := applyReasoningEffortStepdownForAccount(ctx, nil,
			stepdownTestAccount([]string{"none"}, false),
			"muse-spark-1.3-contributor", body)
		require.False(t, changed)
		require.JSONEq(t, string(body), string(got))
	})
}
