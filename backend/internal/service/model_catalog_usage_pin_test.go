package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCatalogDecisionUsagePinCapturesEpochRevisionsAndPricingByValue(t *testing.T) {
	now := time.Unix(1_000, 0).UTC()
	pricing := LiteLLMModelPricing{
		InputCostPerToken:       0.000001,
		OutputCostPerToken:      0.000002,
		CacheReadInputTokenCost: 0.0000002,
		Mode:                    "chat",
	}
	snapshot, err := NewCatalogRuntimeSnapshot(CatalogSnapshotSpec{
		Scope:      "*",
		Epoch:      7,
		RevisionID: 31,
		Revision:   9,
		Checksum:   "checksum-7",
		VerifiedAt: now,
		Models: []CatalogSnapshotModelSpec{{
			ID:                   11,
			RevisionID:           21,
			CanonicalKey:         "provider/model-effective",
			OperatorState:        CatalogOperatorStateEnabled,
			SourceState:          CatalogSourceStatePresent,
			Provider:             "provider",
			Platform:             "openai",
			PricingSchemaVersion: 1,
			PricingValid:         true,
			PricingSource:        "catalog-source",
			Pricing:              &pricing,
		}},
	})
	require.NoError(t, err)

	decision := CatalogDecision{
		Requested: CatalogModelDecision{
			ID:                10,
			RevisionID:        20,
			CatalogRevisionID: 31,
			CatalogEpoch:      7,
		},
		Effective: CatalogModelDecision{
			ID:                   11,
			RevisionID:           21,
			CatalogRevisionID:    31,
			CatalogEpoch:         7,
			PricingSource:        "catalog-source",
			Pricing:              pricing,
			PricingSchemaVersion: 1,
			HasPricing:           true,
		},
		View:      snapshot.readView(),
		CreatedAt: now,
	}

	pin, err := decision.UsagePin()
	require.NoError(t, err)
	require.Equal(t, int64(7), pin.CatalogEpoch)
	require.Equal(t, int64(31), pin.CatalogRevisionID)
	require.Equal(t, int64(20), pin.RequestedModelRevisionID)
	require.Equal(t, int64(21), pin.EffectiveModelRevisionID)
	require.Equal(t, "catalog-source", pin.PricingSource)
	require.Equal(t, 0.000001, pin.PricingSnapshot["input_cost_per_token"])
	require.Equal(t, "chat", pin.PricingSnapshot["mode"])

	decision.Effective.Pricing.InputCostPerToken = 999
	require.Equal(t, 0.000001, pin.PricingSnapshot["input_cost_per_token"], "pin must not alias the decision pricing value")
}

func TestCatalogUsagePinApplyToUsageLogCopiesAllProvenance(t *testing.T) {
	pin := CatalogUsagePin{
		CatalogEpoch:             7,
		CatalogRevisionID:        31,
		RequestedModelRevisionID: 20,
		EffectiveModelRevisionID: 21,
		PricingSource:            "catalog-source",
		PricingSnapshot: map[string]any{
			"input_cost_per_token": 0.000001,
		},
	}
	log := &UsageLog{}

	require.NoError(t, pin.ApplyToUsageLog(log))
	require.NotNil(t, log.CatalogEpoch)
	require.Equal(t, int64(7), *log.CatalogEpoch)
	require.Equal(t, int64(31), *log.CatalogRevisionID)
	require.Equal(t, int64(20), *log.RequestedModelRevisionID)
	require.Equal(t, int64(21), *log.EffectiveModelRevisionID)
	require.Equal(t, "catalog-source", *log.PricingSource)
	require.Equal(t, 0.000001, log.PricingSnapshot["input_cost_per_token"])

	pin.PricingSnapshot["input_cost_per_token"] = 999
	require.Equal(t, 0.000001, log.PricingSnapshot["input_cost_per_token"], "usage log must own its snapshot map")
}

func TestChannelUsageFieldsCarryCatalogDecisionAndAdmissionError(t *testing.T) {
	decision := catalogDecisionForUsagePinIntegrationTest(t)
	fields := (ChannelMappingResult{
		MappedModel:     "catalog/model-effective",
		CatalogDecision: decision,
		CatalogError:    ErrCatalogModelDisabled,
	}).ToUsageFields("catalog/model-requested", "catalog/model-effective")

	require.Same(t, decision, fields.CatalogDecision)
	require.ErrorIs(t, fields.CatalogError, ErrCatalogModelDisabled)
}

func TestUsageSettlementRejectsCatalogAdmissionError(t *testing.T) {
	fields := ChannelUsageFields{CatalogError: ErrCatalogModelDisabled}

	gatewayErr := (&GatewayService{}).RecordUsage(context.Background(), &RecordUsageInput{
		ChannelUsageFields: fields,
	})
	require.ErrorIs(t, gatewayErr, ErrCatalogModelDisabled)

	openAIErr := (&OpenAIGatewayService{}).RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		ChannelUsageFields: fields,
	})
	require.ErrorIs(t, openAIErr, ErrCatalogModelDisabled)
}

func TestOpenAIRecordUsageCarriesCatalogUsagePinIntoUsageLog(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	billingRepo := &openAIRecordUsageBillingRepoStub{result: &UsageBillingApplyResult{Applied: true}}
	svc := newOpenAIRecordUsageServiceWithBillingRepoForTest(
		usageRepo,
		billingRepo,
		&openAIRecordUsageUserRepoStub{},
		&openAIRecordUsageSubRepoStub{},
		nil,
	)

	decision := catalogDecisionForUsagePinIntegrationTest(t)
	usageFields := (ChannelMappingResult{
		MappedModel:     "catalog/model-effective",
		CatalogDecision: decision,
	}).ToUsageFields("catalog/model-requested", "catalog/model-effective")
	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID: "catalog-pinned-usage",
			Model:     "catalog-model",
			Duration:  time.Second,
		},
		APIKey:             &APIKey{ID: 1001, Quota: 100},
		User:               &User{ID: 2001},
		Account:            &Account{ID: 3001, Type: AccountTypeAPIKey},
		ChannelUsageFields: usageFields,
	})

	require.NoError(t, err)
	require.NotNil(t, usageRepo.lastLog)
	require.NotNil(t, usageRepo.lastLog.CatalogEpoch)
	require.Equal(t, int64(7), *usageRepo.lastLog.CatalogEpoch)
	require.Equal(t, int64(31), *usageRepo.lastLog.CatalogRevisionID)
	require.Equal(t, int64(20), *usageRepo.lastLog.RequestedModelRevisionID)
	require.Equal(t, int64(21), *usageRepo.lastLog.EffectiveModelRevisionID)
	require.Equal(t, "catalog-source", *usageRepo.lastLog.PricingSource)
	require.Equal(t, 0.000001, usageRepo.lastLog.PricingSnapshot["input_cost_per_token"])

	require.NotNil(t, billingRepo.lastCmd)
	require.Equal(t, int64(7), billingRepo.lastCmd.CatalogEpoch)
	require.Equal(t, int64(31), billingRepo.lastCmd.CatalogRevisionID)
	require.Equal(t, int64(20), billingRepo.lastCmd.RequestedModelRevisionID)
	require.Equal(t, int64(21), billingRepo.lastCmd.EffectiveModelRevisionID)
	require.Equal(t, "catalog-source", billingRepo.lastCmd.PricingSource)
	require.NotEmpty(t, billingRepo.lastCmd.PricingSnapshotHash)
	require.NotEmpty(t, billingRepo.lastCmd.RequestFingerprint)
}

func TestUsageBillingFingerprintIncludesCatalogPin(t *testing.T) {
	base := UsageBillingCommand{UserID: 1, AccountID: 2, APIKeyID: 3, Model: "model", InputTokens: 10}
	legacyFingerprint := buildUsageBillingFingerprint(&base)

	pinned := base
	pinned.CatalogEpoch = 7
	pinned.CatalogRevisionID = 31
	pinned.RequestedModelRevisionID = 20
	pinned.EffectiveModelRevisionID = 21
	pinned.PricingSource = "catalog-source"
	pinned.PricingSnapshotHash = "snapshot-a"
	pinnedFingerprint := buildUsageBillingFingerprint(&pinned)

	require.NotEqual(t, legacyFingerprint, pinnedFingerprint)
	pinned.CatalogEpoch = 8
	require.NotEqual(t, pinnedFingerprint, buildUsageBillingFingerprint(&pinned))
	pinned.CatalogEpoch = 7
	pinned.PricingSnapshotHash = "snapshot-b"
	require.NotEqual(t, pinnedFingerprint, buildUsageBillingFingerprint(&pinned))
}

func TestCatalogDecisionWithEffectiveModelPinsActualUpstreamFromSameView(t *testing.T) {
	now := time.Unix(2_000, 0).UTC()
	channelPricing := LiteLLMModelPricing{InputCostPerToken: 1e-6, OutputCostPerToken: 2e-6}
	upstreamPricing := LiteLLMModelPricing{InputCostPerToken: 3e-6, OutputCostPerToken: 4e-6}
	snapshot, err := NewCatalogRuntimeSnapshot(CatalogSnapshotSpec{
		Scope: CatalogScopeGlobal, Epoch: 8, RevisionID: 80, Revision: 1, Checksum: "effective-finalize",
		VerifiedAt: now,
		Models: []CatalogSnapshotModelSpec{
			{
				ID: 10, RevisionID: 20, CanonicalKey: "catalog/requested",
				OperatorState: CatalogOperatorStateEnabled, SourceState: CatalogSourceStatePresent,
				Platform: PlatformOpenAI,
			},
			{
				ID: 11, RevisionID: 21, CanonicalKey: "catalog/channel",
				OperatorState: CatalogOperatorStateEnabled, SourceState: CatalogSourceStatePresent,
				Platform: PlatformOpenAI, PricingValid: true, PricingSource: "catalog-source", Pricing: &channelPricing,
			},
			{
				ID: 12, RevisionID: 22, CanonicalKey: "catalog/upstream",
				OperatorState: CatalogOperatorStateEnabled, SourceState: CatalogSourceStatePresent,
				Platform: PlatformOpenAI, PricingValid: true, PricingSource: "catalog-source", Pricing: &upstreamPricing,
			},
		},
	})
	require.NoError(t, err)
	view := snapshot.readView()
	decision, err := ResolveCatalogDecision(view, CatalogDecisionRequest{
		RequestedModel: "catalog/requested",
		EffectiveModel: "catalog/channel",
		Platform:       PlatformOpenAI,
	}, now)
	require.NoError(t, err)

	finalized, err := decision.WithEffectiveModel("catalog/upstream", PlatformOpenAI)
	require.NoError(t, err)
	require.Equal(t, int64(20), finalized.Requested.RevisionID)
	require.Equal(t, int64(22), finalized.Effective.RevisionID)
	require.Equal(t, int64(80), finalized.CatalogRevisionID())
	pin, err := finalized.UsagePin()
	require.NoError(t, err)
	require.Equal(t, int64(22), pin.EffectiveModelRevisionID)
	require.Equal(t, 3e-6, pin.PricingSnapshot["input_cost_per_token"])
}

func catalogDecisionForUsagePinIntegrationTest(t *testing.T) *CatalogDecision {
	t.Helper()
	pricing := LiteLLMModelPricing{InputCostPerToken: 0.000001, OutputCostPerToken: 0.000002}
	snapshot, err := NewCatalogRuntimeSnapshot(CatalogSnapshotSpec{
		Scope: "*", Epoch: 7, RevisionID: 31, Revision: 9, Checksum: "usage-pin-test",
		VerifiedAt: time.Unix(1_000, 0).UTC(),
		Models: []CatalogSnapshotModelSpec{{
			ID: 11, RevisionID: 21, CanonicalKey: "catalog/model-effective",
			OperatorState: CatalogOperatorStateEnabled, SourceState: CatalogSourceStatePresent,
			Provider: "catalog", Platform: "openai", PricingSchemaVersion: 1,
			PricingValid: true, PricingSource: "catalog-source", Pricing: &pricing,
		}},
	})
	require.NoError(t, err)
	return &CatalogDecision{
		Requested: CatalogModelDecision{ID: 10, RevisionID: 20, CatalogRevisionID: 31, CatalogEpoch: 7},
		Effective: CatalogModelDecision{
			ID: 11, RevisionID: 21, CatalogRevisionID: 31, CatalogEpoch: 7,
			PricingSource: "catalog-source", Pricing: pricing, PricingSchemaVersion: 1, HasPricing: true,
		},
		View: snapshot.readView(), CreatedAt: time.Unix(1_000, 0).UTC(),
	}
}

func TestCatalogDecisionUsagePinRejectsInvalidDecision(t *testing.T) {
	_, err := (CatalogDecision{}).UsagePin()
	require.ErrorIs(t, err, ErrCatalogDecisionInvalid)
}
