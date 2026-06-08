//go:build unit

package service

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPricingOverrideCache_GetSetDelete(t *testing.T) {
	c := NewPricingOverrideCache()

	// Empty cache returns nothing.
	got, ok := c.Get(1, "anthropic", "claude-opus-4-7")
	require.False(t, ok)
	require.Nil(t, got)
	require.Equal(t, 0, c.Len())

	// Insert an entry. Mixed-case input must be looked up regardless of case.
	c.Set(PricingOverride{
		Key: PricingOverrideKey{
			GroupID:  1,
			Platform: "Anthropic",
			Model:    "Claude-Opus-4-7",
		},
		BillingMode: "token",
		InputPrice:  3.0e-6,
		OutputPrice: 15.0e-6,
	})
	require.Equal(t, 1, c.Len())

	got, ok = c.Get(1, "anthropic", "claude-opus-4-7")
	require.True(t, ok)
	require.NotNil(t, got)
	require.Equal(t, "token", got.BillingMode)
	require.InEpsilon(t, 3.0e-6, got.InputPrice, 1e-12)

	// Case-insensitive lookup.
	got, ok = c.Get(1, "ANTHROPIC", "CLAUDE-OPUS-4-7")
	require.True(t, ok)
	require.NotNil(t, got)

	// Different group id is a different key.
	_, ok = c.Get(2, "anthropic", "claude-opus-4-7")
	require.False(t, ok)

	// Delete removes the entry.
	c.Delete(PricingOverrideKey{
		GroupID:  1,
		Platform: "anthropic",
		Model:    "claude-opus-4-7",
	})
	_, ok = c.Get(1, "anthropic", "claude-opus-4-7")
	require.False(t, ok)
	require.Equal(t, 0, c.Len())
}

func TestPricingOverrideCache_ReplaceAll(t *testing.T) {
	c := NewPricingOverrideCache()

	c.Set(PricingOverride{
		Key: PricingOverrideKey{GroupID: 1, Platform: "openai", Model: "gpt-5"},
	})
	require.Equal(t, 1, c.Len())

	overrides := []PricingOverride{
		{
			Key:         PricingOverrideKey{GroupID: 1, Platform: "anthropic", Model: "claude-opus-4-7"},
			BillingMode: "token",
			InputPrice:  3.0e-6,
		},
		{
			Key:         PricingOverrideKey{GroupID: 1, Platform: "anthropic", Model: "claude-sonnet-4"},
			BillingMode: "token",
			InputPrice:  3.0e-6,
		},
	}
	c.ReplaceAll(overrides, "v42")

	require.Equal(t, 2, c.Len())
	require.Equal(t, "v42", c.Version())

	// The previous entry is gone — ReplaceAll wipes prior state.
	_, ok := c.Get(1, "openai", "gpt-5")
	require.False(t, ok)

	got, ok := c.Get(1, "anthropic", "claude-opus-4-7")
	require.True(t, ok)
	require.NotNil(t, got)
	require.Equal(t, "token", got.BillingMode)
}

func TestPricingOverrideCache_GetReturnsCopy(t *testing.T) {
	c := NewPricingOverrideCache()
	c.Set(PricingOverride{
		Key: PricingOverrideKey{GroupID: 1, Platform: "anthropic", Model: "x"},
		Intervals: []PricingOverrideInterval{
			{MinTokens: 0, MaxTokens: 100, InputPrice: 1.0},
		},
	})

	got, ok := c.Get(1, "anthropic", "x")
	require.True(t, ok)
	require.Len(t, got.Intervals, 1)

	// Mutate the returned slice — the cache must remain unaffected.
	got.Intervals[0].InputPrice = 999.0

	got2, ok := c.Get(1, "anthropic", "x")
	require.True(t, ok)
	require.InDelta(t, 1.0, got2.Intervals[0].InputPrice, 0)
}

func TestPricingOverrideCache_NilSafe(t *testing.T) {
	var c *PricingOverrideCache
	got, ok := c.Get(1, "anthropic", "x")
	require.False(t, ok)
	require.Nil(t, got)
	require.Equal(t, 0, c.Len())
	require.Equal(t, "", c.Version())

	// Mutators on nil receiver are no-ops, not panics.
	c.Set(PricingOverride{})
	c.Delete(PricingOverrideKey{})
	c.ReplaceAll(nil, "")
}

func TestPricingOverrideCache_ConcurrentAccess(t *testing.T) {
	c := NewPricingOverrideCache()
	const writers = 4
	const readers = 8
	const iterations = 200

	var wg sync.WaitGroup
	wg.Add(writers + readers)

	for w := 0; w < writers; w++ {
		w := w
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				c.Set(PricingOverride{
					Key: PricingOverrideKey{
						GroupID:  int64(w),
						Platform: "anthropic",
						Model:    "claude-opus-4-7",
					},
					BillingMode: "token",
					InputPrice:  float64(i),
				})
			}
		}()
	}

	for r := 0; r < readers; r++ {
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				_, _ = c.Get(0, "anthropic", "claude-opus-4-7")
				_ = c.Len()
			}
		}()
	}

	wg.Wait()

	// After the dust settles, every writer should have left a final entry.
	require.Equal(t, writers, c.Len())
}
