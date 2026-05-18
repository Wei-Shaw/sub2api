//go:build embed

package seo

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadFAQ(t *testing.T) {
	t.Run("zh_has_at_least_10_entries", func(t *testing.T) {
		items, err := LoadFAQ("zh")
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(items), 10)
		assert.NotEmpty(t, items[0].Q)
		assert.NotEmpty(t, items[0].A)
		assert.NotEmpty(t, items[0].ID)
	})

	t.Run("en_has_at_least_10_entries", func(t *testing.T) {
		items, err := LoadFAQ("en")
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(items), 10)
	})

	t.Run("unknown_lang_returns_error", func(t *testing.T) {
		_, err := LoadFAQ("ja")
		assert.Error(t, err)
	})

	t.Run("answer_is_short_enough_for_AI_summary_window", func(t *testing.T) {
		items, err := LoadFAQ("zh")
		require.NoError(t, err)
		for _, it := range items {
			assert.LessOrEqual(t, len([]rune(it.A)), 200, "answer too long for %q", it.ID)
		}
	})
}
