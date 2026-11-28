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

	t.Run("case and separator variants", func(t *testing.T) {
		for _, raw := range []string{"MAX", " Max ", "m_a_x"} {
			got, changed, err := ApplyReasoningEffortStepdown(
				[]byte(`{"model":"m","reasoning":{"effort":"`+raw+`"}}`), sparkLevels)
			require.NoError(t, err)
			require.True(t, changed, "raw=%q", raw)
			require.JSONEq(t, `{"model":"m","reasoning":{"effort":"xhigh"}}`, string(got))
		}
		got, changed, err := ApplyReasoningEffortStepdown(
			[]byte(`{"model":"m","reasoning":{"effort":"X-High"}}`), sparkLevels)
		require.NoError(t, err)
		require.False(t, changed)
		require.JSONEq(t, `{"model":"m","reasoning":{"effort":"X-High"}}`, string(got))
	})

	t.Run("unknown requested level passes through", func(t *testing.T) {
		got, changed, err := ApplyReasoningEffortStepdown(
			[]byte(`{"model":"m","reasoning":{"effort":"turbo"}}`), sparkLevels)
		require.NoError(t, err)
		require.False(t, changed)
		require.JSONEq(t, `{"model":"m","reasoning":{"effort":"turbo"}}`, string(got))
	})

	t.Run("explicit none untouched", func(t *testing.T) {
		got, changed, err := ApplyReasoningEffortStepdown(
			[]byte(`{"model":"m","reasoning":{"effort":"none"}}`), []string{"low", "medium", "high"})
		require.NoError(t, err)
		require.False(t, changed)
		require.JSONEq(t, `{"model":"m","reasoning":{"effort":"none"}}`, string(got))
	})

	t.Run("unknown supported entries ignored", func(t *testing.T) {
		got, changed, err := ApplyReasoningEffortStepdown(
			[]byte(`{"model":"m","reasoning":{"effort":"max"}}`),
			[]string{"turbo", "high", "ultra-plus"})
		require.NoError(t, err)
		require.True(t, changed)
		require.JSONEq(t, `{"model":"m","reasoning":{"effort":"high"}}`, string(got))
	})

	t.Run("only none supported passes through", func(t *testing.T) {
		got, changed, err := ApplyReasoningEffortStepdown(
			[]byte(`{"model":"m","reasoning":{"effort":"high"}}`), []string{"none"})
		require.NoError(t, err)
		require.False(t, changed)
		require.JSONEq(t, `{"model":"m","reasoning":{"effort":"high"}}`, string(got))
	})

	t.Run("all three shapes rewritten together", func(t *testing.T) {
		got, changed, err := ApplyReasoningEffortStepdown(
			[]byte(`{"model":"m","reasoning":{"effort":"max"},"reasoning_effort":"max","output_config":{"effort":"ultra"}}`),
			sparkLevels)
		require.NoError(t, err)
		require.True(t, changed)
		require.JSONEq(t, `{"model":"m","reasoning":{"effort":"xhigh"},"reasoning_effort":"xhigh","output_config":{"effort":"xhigh"}}`, string(got))
	})

	t.Run("non string effort untouched", func(t *testing.T) {
		raw := `{"model":"m","reasoning":{"effort":5}}`
		got, changed, err := ApplyReasoningEffortStepdown([]byte(raw), sparkLevels)
		require.NoError(t, err)
		require.False(t, changed)
		require.JSONEq(t, raw, string(got))
	})

	t.Run("empty body untouched", func(t *testing.T) {
		got, changed, err := ApplyReasoningEffortStepdown(nil, sparkLevels)
		require.NoError(t, err)
		require.False(t, changed)
		require.Empty(t, got)
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
