package service

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCatalogModelDecisionPubliclyListableRequiresEnabledPresentAndPricing(t *testing.T) {
	base := CatalogModelDecision{
		OperatorState: CatalogOperatorStateEnabled,
		SourceState:   CatalogSourceStatePresent,
		HasPricing:    true,
		PricingSource: "catalog-source",
	}
	require.True(t, base.PubliclyListable())

	for name, mutate := range map[string]func(*CatalogModelDecision){
		"disabled": func(decision *CatalogModelDecision) {
			decision.OperatorState = CatalogOperatorStateDisabled
		},
		"retired": func(decision *CatalogModelDecision) {
			decision.OperatorState = CatalogOperatorStateRetired
		},
		"source missing": func(decision *CatalogModelDecision) {
			decision.SourceState = CatalogSourceStateMissing
		},
		"source invalid": func(decision *CatalogModelDecision) {
			decision.SourceState = CatalogSourceStateInvalid
		},
		"unpriced": func(decision *CatalogModelDecision) {
			decision.HasPricing = false
		},
		"pricing source missing": func(decision *CatalogModelDecision) {
			decision.PricingSource = ""
		},
	} {
		t.Run(name, func(t *testing.T) {
			decision := base
			mutate(&decision)
			require.False(t, decision.PubliclyListable())
		})
	}
}

func TestResolveCatalogDecisionChecksRequestedAndEffectiveOnOnePinnedView(t *testing.T) {
	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	spec := catalogSnapshotFixture(7, 701, now, 5e-6)
	spec.Models = append(spec.Models,
		CatalogSnapshotModelSpec{
			ID:            2,
			RevisionID:    702,
			CanonicalKey:  "disabled-target",
			OperatorState: CatalogOperatorStateDisabled,
			SourceState:   CatalogSourceStatePresent,
			Platform:      "openai",
			PricingValid:  true,
			Pricing:       &LiteLLMModelPricing{InputCostPerToken: 7e-6},
		},
		CatalogSnapshotModelSpec{
			ID:            3,
			RevisionID:    703,
			CanonicalKey:  "unpriced-target",
			OperatorState: CatalogOperatorStateEnabled,
			SourceState:   CatalogSourceStatePresent,
			PricingValid:  false,
			Platform:      "openai",
		},
	)
	reader := NewAtomicModelCatalogReader(mustCatalogSnapshot(t, spec), time.Hour)
	view, err := reader.BeginDecision(now.Add(time.Minute))
	require.NoError(t, err)

	decision, err := ResolveCatalogDecision(view, CatalogDecisionRequest{
		RequestedModel: "gpt-5.6-sol",
		EffectiveModel: "gpt-5.6-sol",
		Platform:       "openai",
	}, now.Add(time.Minute))
	require.NoError(t, err)
	require.True(t, decision.Valid())
	require.Equal(t, int64(7), decision.CatalogEpoch())
	require.Equal(t, decision.Requested.CatalogEpoch, decision.Effective.CatalogEpoch)

	_, err = ResolveCatalogDecision(view, CatalogDecisionRequest{
		RequestedModel: "gpt-5.6-sol",
		EffectiveModel: "disabled-target",
		Platform:       "openai",
	}, now.Add(time.Minute))
	require.ErrorIs(t, err, ErrCatalogModelDisabled)

	_, err = ResolveCatalogDecision(view, CatalogDecisionRequest{
		RequestedModel: "gpt-5.6-sol",
		EffectiveModel: "unpriced-target",
		Platform:       "openai",
	}, now.Add(time.Minute))
	require.ErrorIs(t, err, ErrCatalogPricingUnavailable)
}

func TestResolveCatalogDecisionRejectsSourceUnavailableBeforePricing(t *testing.T) {
	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	spec := catalogSnapshotFixture(1, 101, now, 5e-6)
	spec.Models[0].SourceState = CatalogSourceStateMissing
	spec.Models[0].PricingValid = false
	view := mustCatalogSnapshot(t, spec).readView()

	_, err := ResolveCatalogDecision(view, CatalogDecisionRequest{
		RequestedModel: "gpt-5.6-sol",
		Platform:       "openai",
	}, now)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrCatalogSourceUnavailable))
	require.False(t, errors.Is(err, ErrCatalogPricingUnavailable), "status gate must run before pricing gate")
}

func TestCatalogDecisionRemainsPinnedAfterReaderSwap(t *testing.T) {
	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	first := mustCatalogSnapshot(t, catalogSnapshotFixture(1, 101, now, 5e-6))
	reader := NewAtomicModelCatalogReader(first, time.Hour)
	view, err := reader.BeginDecision(now.Add(time.Minute))
	require.NoError(t, err)

	decision, err := ResolveCatalogDecision(view, CatalogDecisionRequest{
		RequestedModel: "gpt-5.6",
		Platform:       "openai",
	}, now.Add(time.Minute))
	require.NoError(t, err)

	secondSpec := catalogSnapshotFixture(2, 201, now.Add(2*time.Minute), 99e-6)
	require.NoError(t, reader.Publish(mustCatalogSnapshot(t, secondSpec)))

	require.Equal(t, int64(1), decision.CatalogEpoch())
	require.Equal(t, 5e-6, decision.Effective.Pricing.InputCostPerToken)
	require.Equal(t, int64(1), decision.Effective.CatalogEpoch)
}
