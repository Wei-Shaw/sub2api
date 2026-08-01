package service

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAtomicModelCatalogReaderPinsOneImmutableViewAcrossPublish(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	first := mustCatalogSnapshot(t, catalogSnapshotFixture(7, 701, now, 5e-6))
	reader := NewAtomicModelCatalogReader(first, 5*time.Minute)

	view, err := reader.BeginDecision(now.Add(time.Minute))
	require.NoError(t, err)
	require.Equal(t, int64(7), view.Epoch())

	requested, ok := view.Resolve("GPT-5.6", "openai")
	require.True(t, ok)
	require.Equal(t, "gpt-5.6-sol", requested.CanonicalKey)
	require.Equal(t, CatalogLifecycleActive, requested.Lifecycle())
	require.True(t, requested.HasPricing)
	require.Equal(t, 5e-6, requested.Pricing.InputCostPerToken)

	secondSpec := catalogSnapshotFixture(8, 801, now.Add(2*time.Minute), 6e-6)
	secondSpec.Models[0].CanonicalKey = "gpt-5.6-terra"
	secondSpec.Models[0].Aliases = []CatalogAliasSpec{{Alias: "gpt-5.6", PlatformScope: "openai"}}
	second := mustCatalogSnapshot(t, secondSpec)
	require.NoError(t, reader.Publish(second))

	// The already accepted decision remains pinned to epoch 7.
	requestedAgain, ok := view.Resolve("gpt-5.6", "openai")
	require.True(t, ok)
	require.Equal(t, int64(7), view.Epoch())
	require.Equal(t, "gpt-5.6-sol", requestedAgain.CanonicalKey)
	require.Equal(t, 5e-6, requestedAgain.Pricing.InputCostPerToken)

	// A new decision sees epoch 8.
	newView, err := reader.BeginDecision(now.Add(3 * time.Minute))
	require.NoError(t, err)
	require.Equal(t, int64(8), newView.Epoch())
	newModel, ok := newView.Resolve("gpt-5.6", "openai")
	require.True(t, ok)
	require.Equal(t, "gpt-5.6-terra", newModel.CanonicalKey)
	require.Equal(t, 6e-6, newModel.Pricing.InputCostPerToken)
}

func TestCatalogReadViewReturnsPricingByValue(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	reader := NewAtomicModelCatalogReader(mustCatalogSnapshot(t, catalogSnapshotFixture(1, 101, now, 5e-6)), time.Hour)
	view, err := reader.BeginDecision(now)
	require.NoError(t, err)

	first, ok := view.Resolve("gpt-5.6-sol", "openai")
	require.True(t, ok)
	first.Pricing.InputCostPerToken = 999

	second, ok := view.Resolve("gpt-5.6-sol", "openai")
	require.True(t, ok)
	require.Equal(t, 5e-6, second.Pricing.InputCostPerToken)
}

func TestCatalogSnapshotScopedAliasPrecedenceAndLifecycle(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	spec := CatalogSnapshotSpec{
		Scope:      CatalogScopeGlobal,
		Epoch:      3,
		RevisionID: 301,
		Revision:   3,
		Checksum:   "fixture-3",
		VerifiedAt: now,
		Models: []CatalogSnapshotModelSpec{
			{
				ID:            1,
				RevisionID:    11,
				CanonicalKey:  "shared-global",
				OperatorState: CatalogOperatorStateEnabled,
				SourceState:   CatalogSourceStatePresent,
				Platform:      "global",
				PricingValid:  true,
				Pricing:       &LiteLLMModelPricing{InputCostPerToken: 1e-6},
				Aliases:       []CatalogAliasSpec{{Alias: "shared", PlatformScope: CatalogPlatformScopeAny}},
			},
			{
				ID:            2,
				RevisionID:    12,
				CanonicalKey:  "shared-openai",
				OperatorState: CatalogOperatorStateDisabled,
				SourceState:   CatalogSourceStatePresent,
				Platform:      "openai",
				PricingValid:  true,
				Pricing:       &LiteLLMModelPricing{InputCostPerToken: 2e-6},
				Aliases:       []CatalogAliasSpec{{Alias: "shared", PlatformScope: "openai"}},
			},
			{
				ID:            3,
				RevisionID:    13,
				CanonicalKey:  "missing-source",
				OperatorState: CatalogOperatorStateEnabled,
				SourceState:   CatalogSourceStateMissing,
				Platform:      "anthropic",
				PricingValid:  true,
				Pricing:       &LiteLLMModelPricing{InputCostPerToken: 3e-6},
			},
			{
				ID:            4,
				RevisionID:    14,
				CanonicalKey:  "retired-model",
				OperatorState: CatalogOperatorStateRetired,
				SourceState:   CatalogSourceStatePresent,
				Platform:      "anthropic",
			},
		},
	}
	view := mustCatalogSnapshot(t, spec).readView()

	scoped, ok := view.Resolve(" SHARED ", "OpenAI")
	require.True(t, ok)
	require.Equal(t, "shared-openai", scoped.CanonicalKey)
	require.Equal(t, CatalogLifecycleDisabled, scoped.Lifecycle())

	global, ok := view.Resolve("shared", "anthropic")
	require.True(t, ok)
	require.Equal(t, "shared-global", global.CanonicalKey)
	require.Equal(t, CatalogLifecycleActive, global.Lifecycle())

	missing, ok := view.Resolve("missing-source", "anthropic")
	require.True(t, ok)
	require.Equal(t, CatalogLifecycleUnavailable, missing.Lifecycle())

	retired, ok := view.Resolve("retired-model", "anthropic")
	require.True(t, ok)
	require.Equal(t, CatalogLifecycleRetired, retired.Lifecycle())
}

func TestNewCatalogRuntimeSnapshotRejectsUnsafeProjection(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name   string
		mutate func(*CatalogSnapshotSpec)
		errIs  error
	}{
		{
			name: "zero epoch",
			mutate: func(spec *CatalogSnapshotSpec) {
				spec.Epoch = 0
			},
			errIs: ErrCatalogInvalidSnapshot,
		},
		{
			name: "duplicate canonical key",
			mutate: func(spec *CatalogSnapshotSpec) {
				duplicate := spec.Models[0]
				duplicate.ID = 2
				duplicate.RevisionID = 12
				spec.Models = append(spec.Models, duplicate)
			},
			errIs: ErrCatalogDuplicateLookup,
		},
		{
			name: "duplicate scoped alias",
			mutate: func(spec *CatalogSnapshotSpec) {
				duplicate := spec.Models[0]
				duplicate.ID = 2
				duplicate.RevisionID = 12
				duplicate.CanonicalKey = "other-model"
				duplicate.Aliases = []CatalogAliasSpec{{Alias: "gpt-5.6", PlatformScope: "openai"}}
				spec.Models = append(spec.Models, duplicate)
			},
			errIs: ErrCatalogDuplicateLookup,
		},
		{
			name: "pricing marked valid without pricing",
			mutate: func(spec *CatalogSnapshotSpec) {
				spec.Models[0].Pricing = nil
				spec.Models[0].PricingValid = true
			},
			errIs: ErrCatalogInvalidSnapshot,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := catalogSnapshotFixture(1, 101, now, 5e-6)
			tt.mutate(&spec)
			_, err := NewCatalogRuntimeSnapshot(spec)
			require.ErrorIs(t, err, tt.errIs)
		})
	}
}

func TestAtomicModelCatalogReaderFencesEpochsAndFailsSafeOnFreshness(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	current := mustCatalogSnapshot(t, catalogSnapshotFixture(10, 1001, now, 5e-6))
	reader := NewAtomicModelCatalogReader(current, 5*time.Minute)

	require.ErrorIs(t, reader.Publish(mustCatalogSnapshot(t, catalogSnapshotFixture(9, 901, now, 4e-6))), ErrCatalogEpochRegression)
	conflictingEpoch := catalogSnapshotFixture(10, 1002, now, 6e-6)
	conflictingEpoch.Checksum = "fixture-10-conflict"
	require.ErrorIs(t, reader.Publish(mustCatalogSnapshot(t, conflictingEpoch)), ErrCatalogEpochConflict)

	_, err := reader.BeginDecision(now.Add(4*time.Minute + 59*time.Second))
	require.NoError(t, err)

	_, err = reader.BeginDecision(now.Add(5*time.Minute + time.Nanosecond))
	require.ErrorIs(t, err, ErrCatalogSnapshotStale)

	reader.MarkVerified(10, now.Add(5*time.Minute))
	_, err = reader.BeginDecision(now.Add(9 * time.Minute))
	require.NoError(t, err)

	reader.ObserveEpoch(11, now.Add(9*time.Minute))
	_, err = reader.BeginDecision(now.Add(9 * time.Minute))
	require.ErrorIs(t, err, ErrCatalogKnownStale)
}

func TestAtomicModelCatalogReaderConcurrentReadersNeverObservePartialSnapshot(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	reader := NewAtomicModelCatalogReader(mustCatalogSnapshot(t, catalogSnapshotFixture(1, 101, now, 1e-6)), time.Hour)

	const readerCount = 16
	const publishCount = 50
	var wg sync.WaitGroup
	errCh := make(chan error, readerCount)

	for i := 0; i < readerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < publishCount*4; j++ {
				view, err := reader.BeginDecision(now.Add(time.Minute))
				if err != nil {
					errCh <- err
					return
				}
				model, ok := view.Resolve("gpt-5.6", "openai")
				if !ok || model.RevisionID == 0 || !model.HasPricing || model.Pricing.InputCostPerToken <= 0 {
					errCh <- fmt.Errorf("partial projection at epoch %d: %#v", view.Epoch(), model)
					return
				}
			}
		}()
	}

	for epoch := int64(2); epoch <= publishCount; epoch++ {
		snapshot := mustCatalogSnapshot(t, catalogSnapshotFixture(epoch, epoch*100+1, now, float64(epoch)*1e-6))
		require.NoError(t, reader.Publish(snapshot))
	}

	wg.Wait()
	close(errCh)
	for err := range errCh {
		require.NoError(t, err)
	}
}

func TestAtomicModelCatalogReaderWithoutSnapshotFailsClosed(t *testing.T) {
	t.Parallel()

	reader := NewAtomicModelCatalogReader(nil, time.Minute)
	_, err := reader.BeginDecision(time.Now())
	require.True(t, errors.Is(err, ErrCatalogUnavailable))
}

func BenchmarkCatalogReadViewResolve(b *testing.B) {
	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	reader := NewAtomicModelCatalogReader(mustCatalogSnapshot(b, catalogSnapshotFixture(1, 101, now, 5e-6)), time.Hour)
	view, err := reader.BeginDecision(now)
	require.NoError(b, err)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		model, ok := view.Resolve("gpt-5.6", "openai")
		if !ok || model.RevisionID == 0 {
			b.Fatal("model not resolved")
		}
	}
}

func catalogSnapshotFixture(epoch, modelRevisionID int64, verifiedAt time.Time, inputPrice float64) CatalogSnapshotSpec {
	return CatalogSnapshotSpec{
		Scope:      CatalogScopeGlobal,
		Epoch:      epoch,
		RevisionID: epoch * 100,
		Revision:   epoch,
		Checksum:   fmt.Sprintf("fixture-%d", epoch),
		VerifiedAt: verifiedAt,
		Models: []CatalogSnapshotModelSpec{
			{
				ID:            1,
				RevisionID:    modelRevisionID,
				CanonicalKey:  "gpt-5.6-sol",
				OperatorState: CatalogOperatorStateEnabled,
				SourceState:   CatalogSourceStatePresent,
				Provider:      "openai",
				Platform:      "openai",
				Mode:          "chat",
				PricingValid:  true,
				Pricing: &LiteLLMModelPricing{
					InputCostPerToken:   inputPrice,
					OutputCostPerToken:  inputPrice * 6,
					LiteLLMProvider:     "openai",
					Mode:                "chat",
					SupportsServiceTier: true,
					MaxInputTokens:      272000,
					MaxOutputTokens:     128000,
					TokenPricingAbsent:  false,
				},
				Aliases: []CatalogAliasSpec{{Alias: "gpt-5.6", PlatformScope: "openai"}},
			},
		},
	}
}

type catalogTestingT interface {
	Helper()
	Fatalf(format string, args ...any)
}

func mustCatalogSnapshot(t catalogTestingT, spec CatalogSnapshotSpec) *CatalogRuntimeSnapshot {
	t.Helper()
	snapshot, err := NewCatalogRuntimeSnapshot(spec)
	if err != nil {
		t.Fatalf("NewCatalogRuntimeSnapshot() error = %v", err)
	}
	return snapshot
}
