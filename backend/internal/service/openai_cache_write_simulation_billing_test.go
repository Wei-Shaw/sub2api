package service

// These isolated simulations assert safety properties, rather than preserving
// the current unsafe behavior. They intentionally fail on an implementation
// that reprices the original bill, loses its log, or ignores the disable switch.
// All identities, repositories, settings, and price cards are local test data.

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The existing helper overwrites duplicate rows. Model the production insert's
// ON CONFLICT DO NOTHING result here so replay cannot conceal snapshot drift.
type cacheWriteSimulationUsageRepo struct {
	cacheWriteBillingUsageRepoStub
	createErrors      []error
	inserted          []bool
	failuresRemaining int
	afterCreate       func()
}

func (r *cacheWriteSimulationUsageRepo) Create(ctx context.Context, log *UsageLog) (bool, error) {
	if err := ctx.Err(); err != nil {
		r.createErrors = append(r.createErrors, err)
		return false, err
	}
	if r.failuresRemaining > 0 {
		r.failuresRemaining--
		return false, errors.New("simulated temporary usage-log failure")
	}
	if r.afterCreate != nil {
		afterCreate := r.afterCreate
		r.afterCreate = nil
		afterCreate()
	}
	key := cacheWriteBillingUsageKey(log.RequestID, log.APIKeyID)
	if _, exists := r.logs[key]; exists {
		r.inserted = append(r.inserted, false)
		return false, nil
	}
	inserted, err := r.cacheWriteBillingUsageRepoStub.Create(ctx, log)
	r.inserted = append(r.inserted, inserted)
	return inserted, err
}

type cacheWriteSimulationBillingRepo struct {
	cacheWriteBillingRepoStub
	applied         []bool
	contextErrors   []error
	afterFirstDebit func()
}

func (r *cacheWriteSimulationBillingRepo) Apply(ctx context.Context, cmd *UsageBillingCommand) (*UsageBillingApplyResult, error) {
	r.contextErrors = append(r.contextErrors, ctx.Err())
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	result, err := r.cacheWriteBillingRepoStub.Apply(ctx, cmd)
	if err != nil {
		return result, err
	}
	r.applied = append(r.applied, result.Applied)
	if result.Applied && cmd.CacheWriteCorrection == nil && r.afterFirstDebit != nil {
		afterDebit := r.afterFirstDebit
		r.afterFirstDebit = nil
		afterDebit()
	}
	return result, nil
}

type cacheWriteSimulationFixture struct {
	svc         *OpenAIGatewayService
	usageRepo   *cacheWriteSimulationUsageRepo
	billingRepo *cacheWriteSimulationBillingRepo
	input       *OpenAIRecordUsageInput
}

func newCacheWriteSimulationFixture(t *testing.T) *cacheWriteSimulationFixture {
	t.Helper()
	// The runtime switch is process-global. These simulations deliberately do
	// not use t.Parallel and restore its state using the established helper.
	resetOpenAICacheWriteInferenceSwitchForTest(t)
	usageRepo := &cacheWriteSimulationUsageRepo{}
	billingRepo := &cacheWriteSimulationBillingRepo{}
	billingRepo.usageRepo = &usageRepo.cacheWriteBillingUsageRepoStub
	svc := newCacheWriteBillingServiceForTest(usageRepo, billingRepo)
	svc.settingService = &SettingService{}
	svc.settingService.refreshCachedSettings(&SystemSettings{OpenAICacheWriteInferenceEnabled: true})
	svc.openaiCacheWriteInferenceTracker.resetForSettingEpoch(openAICacheWriteInferenceSettingEpoch.Load())

	groupID := int64(99001)
	inputPrice, outputPrice, writePrice, readPrice := 10e-6, 20e-6, 12.5e-6, 1e-6
	group := &Group{
		ID:             groupID,
		RateMultiplier: 0.2,
		ModelPricing: []ChannelModelPricing{{
			Models:          []string{"gpt-6-astra"},
			BillingMode:     BillingModeToken,
			InputPrice:      &inputPrice,
			OutputPrice:     &outputPrice,
			CacheWritePrice: &writePrice,
			CacheReadPrice:  &readPrice,
		}},
	}
	return &cacheWriteSimulationFixture{
		svc:         svc,
		usageRepo:   usageRepo,
		billingRepo: billingRepo,
		input: &OpenAIRecordUsageInput{
			Result: &OpenAIForwardResult{
				RequestID: "simulated-cache-write-original",
				Model:     "gpt-6-astra",
				Usage: OpenAIUsage{
					InputTokens:          2000,
					OutputTokens:         500,
					CacheReadInputTokens: 1000,
				},
			},
			APIKey:                  &APIKey{ID: 99002, GroupID: &groupID, Group: group},
			User:                    &User{ID: 99003},
			Account:                 &Account{ID: 99004, Platform: PlatformOpenAI, Type: AccountTypeOAuth},
			CacheWriteObservationID: "simulated-cache-write-original",
			CacheWriteSettingEpoch:  openAICacheWriteInferenceSettingEpoch.Load(),
			RequestPayloadHash:      "simulated-payload-original",
			PricingAt:               time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC),
		},
	}
}

func (f *cacheWriteSimulationFixture) originalLog() *UsageLog {
	return f.usageRepo.logs[cacheWriteBillingUsageKey(f.input.Result.RequestID, f.input.APIKey.ID)]
}

func (f *cacheWriteSimulationFixture) reconcile900Tokens() {
	f.svc.openaiCacheWriteInferenceTracker.registerInference(f.input.CacheWriteObservationID, openAICacheWriteInference{
		Epoch:               f.input.CacheWriteSettingEpoch,
		Tokens:              900,
		ResidualInputTokens: 100,
		PreviousCacheRead:   1000,
		NextCacheRead:       1900,
		Source:              "inferred_next_hit",
	}, time.Now())
	f.svc.processReadyOpenAICacheWriteReconciliations(context.Background())
}

func TestOpenAICacheWriteSimulation_PriceChangeBetweenOriginalCostAndProbeMustNotMultiplyRepricing(t *testing.T) {
	for _, changedBucket := range []string{"input", "output"} {
		t.Run(changedBucket, func(t *testing.T) {
			f := newCacheWriteSimulationFixture(t)
			ctx := context.Background()
			tokens := UsageTokens{InputTokens: 1000, OutputTokens: 500, CacheReadTokens: 1000}
			models := []string{f.input.Result.Model}
			longContext := false
			cost, err := f.svc.calculateOpenAIRecordUsageCost(ctx, f.input.Result, f.input.APIKey,
				models, 0.2, 1, 1, 0.2, tokens, "", &longContext, f.input.PricingAt)
			require.NoError(t, err)
			require.InDelta(t, 0.021, cost.TotalCost, 1e-12)
			require.InDelta(t, 0.0042, cost.ActualCost, 1e-12)

			billingMode := string(BillingModeToken)
			log := &UsageLog{
				RequestID: f.input.Result.RequestID, APIKeyID: f.input.APIKey.ID,
				UserID: f.input.User.ID, AccountID: f.input.Account.ID,
				Model: f.input.Result.Model, BillingType: BillingTypeBalance, BillingMode: &billingMode,
				InputTokens: 1000, OutputTokens: 500, CacheReadTokens: 1000,
				InputCost: cost.InputCost, OutputCost: cost.OutputCost, CacheReadCost: cost.CacheReadCost,
				TotalCost: cost.TotalCost, ActualCost: cost.ActualCost,
			}

			// Exercise the actual seam between the original cost calculation and
			// the one-token probe, without scheduling a racy goroutine. The request
			// timestamp and customer multiplier remain unchanged. Only the mutable
			// test price card changes, as an admin pricing refresh could do.
			card := &f.input.APIKey.Group.ModelPricing[0]
			if changedBucket == "input" {
				*card.InputPrice = 11e-6
			} else {
				*card.OutputPrice = 30e-6
			}
			snapshot := f.svc.prepareOpenAICacheWriteBillingSnapshot(ctx, f.input,
				log, cost, tokens, false, 1, false)
			require.NotNil(t, snapshot)

			applied, err := applyUsageBilling(ctx, log.RequestID, log, &postUsageBillingParams{
				Cost: cost, User: f.input.User, APIKey: f.input.APIKey, Account: f.input.Account,
				RequestPayloadHash: f.input.RequestPayloadHash, AccountRateMultiplier: 1,
			}, f.svc.billingDeps(), f.billingRepo)
			require.NoError(t, err)
			require.True(t, applied)
			inserted, err := f.usageRepo.Create(ctx, log)
			require.NoError(t, err)
			require.True(t, inserted)
			f.svc.openaiCacheWriteInferenceTracker.attachBillingSnapshot(f.input.CacheWriteObservationID, *snapshot)
			f.reconcile900Tokens()

			require.Len(t, f.billingRepo.cmds, 2)
			// Only moving 900 tokens from $10 to $12.5/MTok is billable:
			// $0.00225 upstream, or $0.00045 after the 0.2 customer multiplier.
			assert.InDelta(t, 2.5e-6, snapshot.DeltaTotalCostPerToken, 1e-12,
				"a price-card change must not enter the one-token marginal delta")
			assert.InDelta(t, 0.00045, f.billingRepo.cmds[1].BalanceCost, 1e-12,
				"unrelated repricing must not be multiplied by inferred write tokens")
			assert.InDelta(t, 0.001, f.originalLog().InputCost, 1e-12)
			assert.InDelta(t, 0.02325, f.originalLog().TotalCost, 1e-12)
			assert.InDelta(t, 0.00465, f.originalLog().ActualCost, 1e-12)
		})
	}
}

func TestOpenAICacheWriteSimulation_ReplayedOriginalMustPreserveFirstBillingSnapshot(t *testing.T) {
	f := newCacheWriteSimulationFixture(t)
	require.NoError(t, f.svc.RecordUsage(context.Background(), f.input))
	firstSnapshot, exists := f.svc.openaiCacheWriteInferenceTracker.billingSnapshots[f.input.CacheWriteObservationID]
	require.True(t, exists)
	require.NotNil(t, f.originalLog())
	firstLog := *f.originalLog()

	// The missing write bucket is zero in the original bill, so changing only
	// its rate preserves the original monetary fingerprint. Replaying the
	// original request is a legitimate no-op debit and no-op usage-row insert.
	*f.input.APIKey.Group.ModelPricing[0].CacheWritePrice = 25e-6
	require.NoError(t, f.svc.RecordUsage(context.Background(), f.input))
	require.Equal(t, []bool{true, false}, f.billingRepo.applied)
	require.Equal(t, []bool{true, false}, f.usageRepo.inserted)
	require.Equal(t, f.billingRepo.cmds[0].RequestFingerprint, f.billingRepo.cmds[1].RequestFingerprint,
		"the replay must also be a valid same-fingerprint duplicate in the real billing repository")
	require.Equal(t, firstLog, *f.originalLog(), "the simulated repository must preserve the first persisted row")

	afterReplay, exists := f.svc.openaiCacheWriteInferenceTracker.billingSnapshots[f.input.CacheWriteObservationID]
	require.True(t, exists)
	assert.InDelta(t, firstSnapshot.DeltaActualCostPerToken, afterReplay.DeltaActualCostPerToken, 1e-12,
		"an unapplied duplicate bill and uninserted row must not replace the first snapshot")
	f.reconcile900Tokens()
	require.Len(t, f.billingRepo.cmds, 3, "two original attempts and one correction attempt")
	require.Equal(t, []bool{true, false, true}, f.billingRepo.applied)
	assert.InDelta(t, 0.00045, f.billingRepo.cmds[2].BalanceCost, 1e-12,
		"the correction must retain the original $12.5/MTok write price")
	assert.InDelta(t, 0.01125, f.originalLog().CacheCreationCost, 1e-12)
	assert.InDelta(t, 0.00465, f.originalLog().ActualCost, 1e-12)
}

func TestOpenAICacheWriteSimulation_CancelledContextMustNotLoseLogAfterSuccessfulDebit(t *testing.T) {
	for _, interruption := range []string{"cancelled_after_debit", "expired_request_deadline"} {
		for _, withObservation := range []bool{false, true} {
			path := "ordinary_path_control"
			if withObservation {
				path = "cache_write_snapshot_path"
			}
			t.Run(interruption+"/"+path, func(t *testing.T) {
				f := newCacheWriteSimulationFixture(t)
				if !withObservation {
					f.input.CacheWriteObservationID = ""
				}
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				if interruption == "cancelled_after_debit" {
					f.billingRepo.afterFirstDebit = cancel
				} else {
					// A pre-expired deadline is deterministic. Detached billing
					// still commits, so the subsequent log must also survive it.
					var deadlineCancel context.CancelFunc
					ctx, deadlineCancel = context.WithDeadline(context.Background(), time.Unix(0, 0))
					defer deadlineCancel()
				}

				err := f.svc.RecordUsage(ctx, f.input)
				require.NoError(t, err)
				require.Error(t, ctx.Err(), "the request context must be interrupted before the log write")
				require.Equal(t, []error{nil}, f.billingRepo.contextErrors,
					"the real billing wrapper must detach cancellation before the fake debit")
				require.Equal(t, []bool{true}, f.billingRepo.applied)
				require.InDelta(t, 0.0042, f.billingRepo.cmds[0].BalanceCost, 1e-12)
				assert.NotNil(t, f.originalLog(),
					"a successful debit must retain its usage log even if the request context ended; create errors: %v", f.usageRepo.createErrors)
				if withObservation {
					_, exists := f.svc.openaiCacheWriteInferenceTracker.billingSnapshots[f.input.CacheWriteObservationID]
					assert.True(t, exists, "the successfully billed turn must remain reconcilable after logging")
				}
			})
		}
	}
}

func TestOpenAICacheWriteSimulation_DisableMustStopAlreadyQueuedReconciliation(t *testing.T) {
	for _, reenable := range []bool{false, true} {
		name := "stays_disabled"
		if reenable {
			name = "off_on_without_another_observation"
		}
		t.Run(name, func(t *testing.T) {
			f := newCacheWriteSimulationFixture(t)
			ctx := context.Background()
			const cacheIdentity = "simulated-cache-write-lineage"
			// Use the real telemetry path and its conservative residual bound.
			f.input.Result.Usage.InputTokens = 154668
			f.input.Result.Usage.CacheReadInputTokens = 152400
			firstBody := []byte(`{"model":"gpt-6-astra","input":[{"role":"user","content":"first"}]}`)
			firstRequest := f.svc.BeginOpenAICacheWriteObservation(ctx, f.input.Account, f.input.APIKey.ID, f.input.Result.Model, cacheIdentity, firstBody)
			require.NotNil(t, firstRequest)
			f.input.Result.CacheWritePromptEvidence = buildOpenAICacheWritePromptEvidence(firstBody)
			f.input.Result.CacheWriteOutputEvidence = buildOpenAICacheWriteOutputEvidence([]byte(`{"output":[{"role":"assistant","content":"answer"}]}`))
			f.input.CacheWriteSettingEpoch = firstRequest.Epoch
			f.input.CacheWriteObservationID = f.svc.ObserveOpenAICacheWriteTelemetry(
				ctx, f.input.Account, f.input.Result.Model, cacheIdentity, f.input.Result, firstRequest)
			require.NotEmpty(t, f.input.CacheWriteObservationID)
			require.NoError(t, f.svc.RecordUsage(ctx, f.input))
			require.NotNil(t, f.originalLog())
			firstLog := *f.originalLog()

			secondResult := *f.input.Result
			secondResult.RequestID = "simulated-cache-write-next-turn"
			secondResult.Usage.InputTokens = 156900
			secondResult.Usage.CacheReadInputTokens = 154600
			second := *f.input
			second.Result = &secondResult
			second.RequestPayloadHash = "simulated-payload-next-turn"
			secondBody := []byte(`{"model":"gpt-6-astra","input":[{"role":"user","content":"first"},{"role":"assistant","content":"answer"},{"role":"user","content":"second"}]}`)
			secondRequest := f.svc.BeginOpenAICacheWriteObservation(ctx, second.Account, second.APIKey.ID, second.Result.Model, cacheIdentity, secondBody)
			require.NotNil(t, secondRequest)
			second.Result.CacheWritePromptEvidence = buildOpenAICacheWritePromptEvidence(secondBody)
			second.Result.CacheWriteOutputEvidence = buildOpenAICacheWriteOutputEvidence([]byte(`{"output":[]}`))
			second.CacheWriteSettingEpoch = secondRequest.Epoch
			second.CacheWriteObservationID = f.svc.ObserveOpenAICacheWriteTelemetry(
				ctx, second.Account, second.Result.Model, cacheIdentity, second.Result, secondRequest)
			require.Len(t, f.svc.openaiCacheWriteInferenceTracker.ready, 1)

			// The completed second request is already queued for RecordUsage.
			// No new observation runs after the admin disables the feature.
			f.svc.settingService.refreshCachedSettings(&SystemSettings{OpenAICacheWriteInferenceEnabled: false})
			if reenable {
				f.svc.settingService.refreshCachedSettings(&SystemSettings{OpenAICacheWriteInferenceEnabled: true})
			}
			require.NoError(t, f.svc.RecordUsage(ctx, &second))
			assert.Len(t, f.billingRepo.cmds, 2,
				"only the two normal bills may run; a disable transition invalidates already-queued inference")
			assert.Empty(t, f.usageRepo.corrections, "queued inference must not rewrite usage after disable")
			assert.Equal(t, firstLog, *f.originalLog(), "the original row must remain unchanged")
		})
	}
}

func TestOpenAICacheWriteSimulation_LogFailureRetriesAndRetainsChargedSnapshot(t *testing.T) {
	for _, failures := range []int{1, 2} {
		t.Run(strconv.Itoa(failures), func(t *testing.T) {
			f := newCacheWriteSimulationFixture(t)
			f.usageRepo.failuresRemaining = failures
			err := f.svc.RecordUsage(context.Background(), f.input)
			if failures == 1 {
				require.NoError(t, err, "the second detached write must recover a transient failure")
				require.NotNil(t, f.originalLog())
			} else {
				require.Error(t, err, "a committed charge without a persisted row must not report success")
				require.Nil(t, f.originalLog())
			}
			require.Len(t, f.billingRepo.cmds, 1)
			snapshot := f.svc.openaiCacheWriteInferenceTracker.lookupBillingSnapshot(f.input.CacheWriteObservationID)
			require.NotNil(t, snapshot, "retain first charged prices and row for recovery")
			require.NotNil(t, snapshot.OriginalUsageLog)
			*f.input.APIKey.Group.ModelPricing[0].CacheWritePrice = 25e-6
			f.reconcile900Tokens()
			require.Len(t, f.billingRepo.cmds, 2)
			require.NotNil(t, f.originalLog())
			require.InDelta(t, 0.00045, f.billingRepo.cmds[1].BalanceCost, 1e-12)
			require.InDelta(t, 0.00465, f.originalLog().ActualCost, 1e-12)
		})
	}
}

func TestOpenAICacheWriteSimulation_DeduplicatedRetryRestoresOriginalRow(t *testing.T) {
	f := newCacheWriteSimulationFixture(t)
	f.usageRepo.failuresRemaining = 2
	require.Error(t, f.svc.RecordUsage(context.Background(), f.input))
	*f.input.APIKey.Group.ModelPricing[0].CacheWritePrice = 25e-6
	require.NoError(t, f.svc.RecordUsage(context.Background(), f.input))
	require.Equal(t, []bool{true, false}, f.billingRepo.applied)
	require.NotNil(t, f.originalLog())
	f.reconcile900Tokens()
	require.Len(t, f.billingRepo.cmds, 3)
	require.InDelta(t, 0.00045, f.billingRepo.cmds[2].BalanceCost, 1e-12)
}

func TestOpenAICacheWriteSimulation_DisableDuringLogRecoveryStopsAdjustment(t *testing.T) {
	f := newCacheWriteSimulationFixture(t)
	require.NoError(t, f.svc.RecordUsage(context.Background(), f.input))
	f.usageRepo.afterCreate = func() {
		f.svc.settingService.refreshCachedSettings(&SystemSettings{OpenAICacheWriteInferenceEnabled: false})
	}
	f.reconcile900Tokens()
	require.Len(t, f.billingRepo.cmds, 1, "the flag is rechecked after the recovery write")
	require.Empty(t, f.usageRepo.corrections)
}

func TestOpenAICacheWriteSimulation_FreeFastUsesFrozenSeparateStandardPlan(t *testing.T) {
	f := newCacheWriteSimulationFixture(t)
	tokens := UsageTokens{InputTokens: 1000, OutputTokens: 500, CacheReadTokens: 1000}
	fastMultiplier := 3.0
	f.input.APIKey.Group.ModelPricing[0].FastMultiplier = &fastMultiplier
	priority, err := f.svc.calculateOpenAIRecordUsageCost(context.Background(), f.input.Result, f.input.APIKey,
		[]string{f.input.Result.Model}, 0.2, 1, 1, 0.2, tokens, "priority", nil, f.input.PricingAt)
	require.NoError(t, err)
	standard, err := f.svc.calculateOpenAIRecordUsageCost(context.Background(), f.input.Result, f.input.APIKey,
		[]string{f.input.Result.Model}, 0.2, 1, 1, 0.2, tokens, "", nil, f.input.PricingAt)
	require.NoError(t, err)
	priority.ActualCost = standard.ActualCost
	priority.cacheWritePricing.standard = standard.cacheWritePricing
	priority.cacheWritePricing.requiresStandard = true
	fastMultiplier = 9
	*f.input.APIKey.Group.ModelPricing[0].CacheWritePrice = 50e-6
	mode := string(BillingModeToken)
	log := &UsageLog{RequestID: f.input.Result.RequestID, BillingMode: &mode,
		InputTokens: tokens.InputTokens, InputCost: priority.InputCost, CacheCreationCost: priority.CacheCreationCost,
		TotalCost: priority.TotalCost, ActualCost: priority.ActualCost}
	snapshot := f.svc.prepareOpenAICacheWriteBillingSnapshot(context.Background(), f.input, log, priority, tokens, false, 1, false)
	require.NotNil(t, snapshot)
	require.InDelta(t, 7.5e-6, snapshot.DeltaTotalCostPerToken, 1e-12)
	require.InDelta(t, 0.5e-6, snapshot.DeltaActualCostPerToken, 1e-12)
}

func TestOpenAICacheWriteSimulation_AccountStatsUsesFrozenSelectedRule(t *testing.T) {
	f := newCacheWriteSimulationFixture(t)
	tokens := UsageTokens{InputTokens: 1000, OutputTokens: 500, CacheReadTokens: 1000}
	inputPrice, outputPrice, writePrice := 3e-6, 4e-6, 5e-6
	channel := Channel{ID: 17, Status: StatusActive, GroupIDs: []int64{*f.input.APIKey.GroupID},
		AccountStatsPricingRules: []AccountStatsPricingRule{{AccountIDs: []int64{f.input.Account.ID},
			Pricing: []ChannelModelPricing{{Models: []string{f.input.Result.Model}, InputPrice: &inputPrice,
				OutputPrice: &outputPrice, CacheWritePrice: &writePrice}}}}}
	f.svc.channelService = &ChannelService{}
	f.svc.channelService.cache.Store(populateChannelCache([]Channel{channel}, map[int64]string{*f.input.APIKey.GroupID: PlatformOpenAI}))
	cost, err := f.svc.calculateOpenAIRecordUsageCost(context.Background(), f.input.Result, f.input.APIKey,
		[]string{f.input.Result.Model}, 0.2, 1, 1, 0.2, tokens, "", nil, f.input.PricingAt)
	require.NoError(t, err)
	mode := string(BillingModeToken)
	log := &UsageLog{RequestID: f.input.Result.RequestID, Model: f.input.Result.Model,
		GroupID: f.input.APIKey.GroupID, AccountID: f.input.Account.ID, BillingMode: &mode,
		InputTokens: tokens.InputTokens, InputCost: cost.InputCost, CacheCreationCost: cost.CacheCreationCost,
		TotalCost: cost.TotalCost, ActualCost: cost.ActualCost}
	f.svc.applyFrozenOpenAICacheWriteAccountStatsCost(context.Background(), log, cost, tokens, f.input.Result.UpstreamModel, f.input.Result.Model, f.input.PricingAt, false)
	require.NotNil(t, log.AccountStatsCost)
	require.InDelta(t, 0.005, *log.AccountStatsCost, 1e-12)
	inputPrice, outputPrice, writePrice = 30e-6, 40e-6, 50e-6
	snapshot := f.svc.prepareOpenAICacheWriteBillingSnapshot(context.Background(), f.input, log, cost, tokens, false, 1, false)
	require.NotNil(t, snapshot)
	require.NotNil(t, snapshot.DeltaAccountStatsCostPerToken)
	require.InDelta(t, 2e-6, *snapshot.DeltaAccountStatsCostPerToken, 1e-12)
}

func TestOpenAICacheWriteSimulation_FrozenAccountStatsPreservesOriginalPriority(t *testing.T) {
	for _, source := range []string{"custom_interval", "customer_total", "catalog", "zero_custom_falls_through"} {
		t.Run(source, func(t *testing.T) {
			f := newCacheWriteSimulationFixture(t)
			tokens := UsageTokens{InputTokens: 1000, OutputTokens: 500, CacheReadTokens: 1000}
			inputPrice, outputPrice, writePrice := 3e-6, 4e-6, 5e-6
			channel := Channel{ID: 18, Status: StatusActive, GroupIDs: []int64{*f.input.APIKey.GroupID}}
			switch source {
			case "custom_interval":
				channel.AccountStatsPricingRules = []AccountStatsPricingRule{{AccountIDs: []int64{f.input.Account.ID},
					Pricing: []ChannelModelPricing{{Models: []string{f.input.Result.Model}, Intervals: []PricingInterval{{
						MinTokens: 1, InputPrice: &inputPrice, OutputPrice: &outputPrice, CacheWritePrice: &writePrice,
					}}}}}}
			case "customer_total":
				channel.ApplyPricingToAccountStats = true
			case "zero_custom_falls_through":
				channel.ApplyPricingToAccountStats = true
				channel.AccountStatsPricingRules = []AccountStatsPricingRule{{AccountIDs: []int64{f.input.Account.ID},
					Pricing: []ChannelModelPricing{{Models: []string{f.input.Result.Model}}}}}
			}
			f.svc.channelService = &ChannelService{}
			f.svc.channelService.cache.Store(populateChannelCache([]Channel{channel}, map[int64]string{*f.input.APIKey.GroupID: PlatformOpenAI}))
			cost, err := f.svc.calculateOpenAIRecordUsageCost(context.Background(), f.input.Result, f.input.APIKey,
				[]string{f.input.Result.Model}, 0.2, 1, 1, 0.2, tokens, "", nil, f.input.PricingAt)
			require.NoError(t, err)
			mode := string(BillingModeToken)
			log := &UsageLog{RequestID: f.input.Result.RequestID, Model: f.input.Result.Model,
				GroupID: f.input.APIKey.GroupID, AccountID: f.input.Account.ID, BillingMode: &mode,
				InputTokens: tokens.InputTokens, InputCost: cost.InputCost, CacheCreationCost: cost.CacheCreationCost,
				TotalCost: cost.TotalCost, ActualCost: cost.ActualCost}
			reference := *log
			applyAccountStatsCost(context.Background(), &reference, f.svc.channelService, f.svc.billingService,
				f.input.Account.ID, *f.input.APIKey.GroupID, f.input.Result.UpstreamModel, f.input.Result.Model,
				tokens, cost.TotalCost, f.input.PricingAt, false)
			f.svc.applyFrozenOpenAICacheWriteAccountStatsCost(context.Background(), log, cost, tokens,
				f.input.Result.UpstreamModel, f.input.Result.Model, f.input.PricingAt, false)
			require.NotNil(t, reference.AccountStatsCost)
			require.Equal(t, reference.AccountStatsCost, log.AccountStatsCost)
			inputPrice, outputPrice, writePrice = 30e-6, 40e-6, 50e-6
			snapshot := f.svc.prepareOpenAICacheWriteBillingSnapshot(context.Background(), f.input, log, cost, tokens, false, 1, false)
			require.NotNil(t, snapshot)
			require.NotNil(t, snapshot.DeltaAccountStatsCostPerToken)
			wantDelta := 2.5e-6
			if source == "custom_interval" {
				wantDelta = 2e-6
			}
			require.InDelta(t, wantDelta, *snapshot.DeltaAccountStatsCostPerToken, 1e-12)
		})
	}
}
