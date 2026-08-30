package service

import (
	"context"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

type officialPriceFetcherStub struct {
	snapshot *OfficialPriceSourceSnapshot
	err      error
	calls    int
}

func (f *officialPriceFetcherStub) Fetch(context.Context) (*OfficialPriceSourceSnapshot, error) {
	f.calls++
	return f.snapshot, f.err
}

type officialPriceSyncRepoStub struct {
	stubDisplayPricingRepo
	updates    []OfficialPriceUpdate
	applyCalls int
}

func (r *officialPriceSyncRepoStub) ApplyOfficialPriceUpdates(_ context.Context, updates []OfficialPriceUpdate) error {
	r.applyCalls++
	r.updates = append(r.updates, updates...)
	return nil
}

func TestOfficialPricePreviewIsReadOnlyAndExplainsIneligibleModels(t *testing.T) {
	now := time.Date(2026, 8, 30, 10, 0, 0, 0, time.UTC)
	input, output, read := mustDecimal("9.8"), mustDecimal("30.9"), mustDecimal("1.9")
	repo := &officialPriceSyncRepoStub{stubDisplayPricingRepo: stubDisplayPricingRepo{models: []DisplayModelPrice{
		{ID: 1, ModelName: "glm-5.1", Provider: "zhipu", BillingMode: DisplayBillingModeToken, Currency: DisplayCurrencyCNY, UpdatedAt: now},
		{ID: 2, ModelName: "glm-5.1", Provider: "zhipu", BillingMode: DisplayBillingModePerRequest, Currency: DisplayCurrencyCNY, UpdatedAt: now},
		{ID: 3, ModelName: "gpt-5.6", Provider: "openai", BillingMode: DisplayBillingModeToken, Currency: DisplayCurrencyUSD, UpdatedAt: now},
	}}}
	fetcher := &officialPriceFetcherStub{snapshot: &OfficialPriceSourceSnapshot{FetchedAt: now, Models: map[string]OfficialPriceCandidate{
		"glm-5.1": {ModelName: "glm-5.1", ProviderKey: "glm", Currency: DisplayCurrencyCNY, Enabled: true, Input: &input, Output: &output, CacheRead: &read},
	}}}
	svc := NewOfficialPriceSyncService(NewDisplayPricingService(repo), fetcher)
	preview, err := svc.Preview(context.Background())
	require.NoError(t, err)
	require.Equal(t, 0, repo.applyCalls)
	require.Len(t, preview.Items, 2)
	items := map[int64]OfficialPricePreviewItem{}
	for _, item := range preview.Items {
		items[item.ModelID] = item
	}
	require.True(t, items[1].Applicable)
	require.Equal(t, 9.8, *items[1].Proposed.InputPerMillion)
	require.True(t, items[1].Diff.HasChanges)
	require.NotEmpty(t, items[1].ProposalHash)
	require.Equal(t, OfficialPriceReasonCurrencyMismatch, items[3].Reason)
}

func TestOfficialPriceApplyRecomputesAndPreservesNonOfficialFields(t *testing.T) {
	now := time.Date(2026, 8, 30, 10, 0, 0, 0, time.UTC)
	input, output := mustDecimal("1.6"), mustDecimal("4.7")
	multiplier, perRequest := 0.2, 0.01
	repo := &officialPriceSyncRepoStub{stubDisplayPricingRepo: stubDisplayPricingRepo{models: []DisplayModelPrice{{
		ID: 7, ModelName: "deepseek-v4-flash-0731", Provider: "deepseek", BillingMode: DisplayBillingModeToken,
		Currency: DisplayCurrencyCNY, UpdatedAt: now, ModelMultiplier: &multiplier, PerRequestLTE256K: &perRequest, ModelNote: "keep me",
	}}}}
	fetcher := &officialPriceFetcherStub{snapshot: &OfficialPriceSourceSnapshot{FetchedAt: now, Models: map[string]OfficialPriceCandidate{
		"deepseek-v4-flash-0731": {ModelName: "deepseek-v4-flash-0731", ProviderKey: "deepseek", Currency: DisplayCurrencyCNY, Enabled: true, Input: &input, Output: &output},
	}}}
	svc := NewOfficialPriceSyncService(NewDisplayPricingService(repo), fetcher)
	svc.now = func() time.Time { return now.Add(time.Minute) }
	proposalHash := officialPriceProposalHash(7, OfficialPriceValues{InputPerMillion: officialTestFloat64Ptr(1.6), OutputPerMillion: officialTestFloat64Ptr(4.7)})
	result, err := svc.Apply(context.Background(), []OfficialPriceApplySelection{{ModelID: 7, ExpectedUpdatedAt: now, ProposalHash: proposalHash}})
	require.NoError(t, err)
	require.Equal(t, 1, result.AppliedCount)
	require.Equal(t, 1, repo.applyCalls)
	require.Equal(t, 1, fetcher.calls)
	require.Len(t, repo.updates, 1)
	require.Equal(t, OfficialPriceSourceHerohaoAggregate, repo.updates[0].OfficialPriceSource)
	require.Equal(t, HerohaoOfficialPriceCandidateURL, repo.updates[0].OfficialPriceSourceURL)
	require.Equal(t, "1.6", repo.updates[0].InputPerMillion.String())
	require.Equal(t, "4.7", repo.updates[0].OutputPerMillion.String())
	require.Equal(t, "keep me", repo.models[0].ModelNote)
	require.Equal(t, 0.2, *repo.models[0].ModelMultiplier)
	require.Equal(t, 0.01, *repo.models[0].PerRequestLTE256K)
}

func TestOfficialPriceApplyRejectsStalePreviewBeforeRepositoryWrite(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Microsecond)
	input := mustDecimal("1")
	repo := &officialPriceSyncRepoStub{stubDisplayPricingRepo: stubDisplayPricingRepo{models: []DisplayModelPrice{{
		ID: 1, ModelName: "glm-5.1", Provider: "zhipu", BillingMode: DisplayBillingModeToken, Currency: DisplayCurrencyCNY, UpdatedAt: now,
	}}}}
	fetcher := &officialPriceFetcherStub{snapshot: &OfficialPriceSourceSnapshot{FetchedAt: now, Models: map[string]OfficialPriceCandidate{
		"glm-5.1": {ModelName: "glm-5.1", ProviderKey: "glm", Currency: DisplayCurrencyCNY, Enabled: true, Input: &input},
	}}}
	svc := NewOfficialPriceSyncService(NewDisplayPricingService(repo), fetcher)
	_, err := svc.Apply(context.Background(), []OfficialPriceApplySelection{{ModelID: 1, ExpectedUpdatedAt: now.Add(-time.Second), ProposalHash: "stale-proposal"}})
	require.ErrorIs(t, err, ErrOfficialPriceApplyConflict)
	require.Zero(t, repo.applyCalls)
}

func TestOfficialPriceApplyRejectsChangedSourceProposal(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Microsecond)
	input := mustDecimal("1")
	repo := &officialPriceSyncRepoStub{stubDisplayPricingRepo: stubDisplayPricingRepo{models: []DisplayModelPrice{{
		ID: 2, ModelName: "glm-5.1", Provider: "zhipu", BillingMode: DisplayBillingModeToken, Currency: DisplayCurrencyCNY, UpdatedAt: now,
	}}}}
	fetcher := &officialPriceFetcherStub{snapshot: &OfficialPriceSourceSnapshot{FetchedAt: now, Models: map[string]OfficialPriceCandidate{
		"glm-5.1": {ModelName: "glm-5.1", ProviderKey: "glm", Currency: DisplayCurrencyCNY, Enabled: true, Input: &input},
	}}}
	svc := NewOfficialPriceSyncService(NewDisplayPricingService(repo), fetcher)

	_, err := svc.Apply(context.Background(), []OfficialPriceApplySelection{{
		ModelID: 2, ExpectedUpdatedAt: now, ProposalHash: "different-from-preview",
	}})
	require.ErrorIs(t, err, ErrOfficialPriceApplyConflict)
	require.Zero(t, repo.applyCalls)
}

func mustDecimal(value string) decimal.Decimal {
	result, err := decimal.NewFromString(value)
	if err != nil {
		panic(err)
	}
	return result
}

func officialTestFloat64Ptr(value float64) *float64 { return &value }
