//go:build unit

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type grokVideoSettlementRepoStub struct {
	settlement         *GrokVideoSettlement
	createCalls        int
	getCalls           int
	markTerminalCalls  int
	markSettledCalls   int
	lastTerminalStatus string
	createErr          error
	getErr             error
	markErr            error
}

type grokVideoConcurrentBillingRepoStub struct {
	UsageBillingRepository
	settlement *GrokVideoSettlement
	err        error
	calls      int
}

func (r *grokVideoConcurrentBillingRepoStub) Apply(_ context.Context, _ *UsageBillingCommand) (*UsageBillingApplyResult, error) {
	r.calls++
	if r.settlement != nil {
		r.settlement.Status = GrokVideoSettlementStatusSettled
	}
	return nil, r.err
}

func (r *grokVideoSettlementRepoStub) CreateGrokVideoSettlement(_ context.Context, settlement *GrokVideoSettlement) (*GrokVideoSettlement, error) {
	r.createCalls++
	r.settlement = settlement
	if r.createErr != nil {
		return nil, r.createErr
	}
	return settlement, nil
}

func (r *grokVideoSettlementRepoStub) GetGrokVideoSettlementForOwner(_ context.Context, requestID string, userID, apiKeyID int64) (*GrokVideoSettlement, error) {
	r.getCalls++
	if r.getErr != nil {
		return nil, r.getErr
	}
	if r.settlement == nil || r.settlement.RequestID != requestID || r.settlement.UserID != userID || r.settlement.APIKeyID != apiKeyID {
		return nil, ErrGrokVideoSettlementNotFound
	}
	return r.settlement, nil
}

func (r *grokVideoSettlementRepoStub) MarkGrokVideoSettlementTerminal(_ context.Context, _ int64, status string) error {
	r.markTerminalCalls++
	r.lastTerminalStatus = status
	if r.markErr == nil && r.settlement != nil {
		r.settlement.Status = status
	}
	return r.markErr
}

func (r *grokVideoSettlementRepoStub) MarkGrokVideoSettlementSettled(_ context.Context, _ int64) error {
	r.markSettledCalls++
	if r.markErr == nil && r.settlement != nil {
		r.settlement.Status = GrokVideoSettlementStatusSettled
	}
	return r.markErr
}

func newGrokVideoRegistrationTestContext(repo GrokVideoSettlementRepository) (*OpenAIGatewayService, *APIKey, *Account) {
	groupID := int64(7)
	videoPrice := 0.05
	svc := newOpenAIRecordUsageServiceForTest(nil, nil, nil, nil)
	svc.grokVideoSettlementRepo = repo
	apiKey := &APIKey{
		ID: 12, User: &User{ID: 11}, GroupID: &groupID,
		Group: &Group{
			ID: groupID, Platform: PlatformGrok, RateMultiplier: 1,
			VideoRateIndependent: true, VideoRateMultiplier: 1, VideoPrice480P: &videoPrice,
		},
	}
	account := &Account{ID: 13, Platform: PlatformGrok, Type: AccountTypeOAuth}
	return svc, apiKey, account
}

func TestNormalizeGrokVideoUpstreamStatus(t *testing.T) {
	tests := map[string]string{
		"queued":     GrokVideoUpstreamStatusPending,
		"processing": GrokVideoUpstreamStatusPending,
		"completed":  GrokVideoUpstreamStatusDone,
		"succeeded":  GrokVideoUpstreamStatusDone,
		"failure":    GrokVideoUpstreamStatusFailed,
		"expired":    GrokVideoUpstreamStatusExpired,
		"canceled":   GrokVideoSettlementStatusCancelled,
	}
	for input, want := range tests {
		require.Equal(t, want, NormalizeGrokVideoUpstreamStatus(input))
	}
}

func TestRegisterGrokVideoSettlementNormalizesImmutableSnapshot(t *testing.T) {
	repo := &grokVideoSettlementRepoStub{}
	svc, apiKey, account := newGrokVideoRegistrationTestContext(repo)
	settlement := &GrokVideoSettlement{
		RequestID:      " video-request-1 ",
		RequestedModel: " grok-imagine-video ",
		SessionID:      " client-session-1 ",
	}

	err := svc.RegisterGrokVideoSettlement(context.Background(), settlement, apiKey, account, nil)

	require.NoError(t, err)
	require.Equal(t, 1, repo.createCalls)
	require.Equal(t, "video-request-1", settlement.RequestID)
	require.Equal(t, "grok-imagine-video", settlement.BillingModel)
	require.Equal(t, VideoBillingResolution480P, settlement.VideoResolution)
	require.Equal(t, VideoBillingDefaultDurationSeconds, settlement.VideoDurationSeconds)
	require.Equal(t, "client-session-1", settlement.SessionID)
	require.Equal(t, GrokVideoSettlementStatusPending, settlement.Status)
	require.Equal(t, GrokVideoPricingSnapshotVersion, settlement.PricingSnapshotVersion)
	require.Equal(t, GrokVideoPricingBasisVideoSecond, settlement.PricingBasis)
	require.Equal(t, string(BillingModeVideo), settlement.BillingMode)
	require.InDelta(t, 0.4, settlement.ActualCost, 1e-12)
	require.Len(t, settlement.RequestFingerprint, 64)
}

func TestGrokVideoSettlementFingerprintIncludesSessionID(t *testing.T) {
	left := &GrokVideoSettlement{RequestID: "video-request-1", SessionID: "session-a"}
	right := &GrokVideoSettlement{RequestID: "video-request-1", SessionID: "session-b"}

	left.Normalize()
	right.Normalize()

	require.NotEqual(t, left.RequestFingerprint, right.RequestFingerprint)
}

func TestPrepareGrokVideoSettlementRejectsTokenPricingWithoutUpstreamRequest(t *testing.T) {
	repo := &grokVideoSettlementRepoStub{}
	groupID := int64(132)
	svc := newOpenAIRecordUsageServiceForTest(nil, nil, nil, nil)
	svc.grokVideoSettlementRepo = repo
	svc.resolver = newOpenAITokenImageChannelPricingResolverForTest(t, groupID, "grok-imagine-video")
	apiKey := &APIKey{
		ID: 12, User: &User{ID: 11}, GroupID: &groupID,
		Group: &Group{ID: groupID, Platform: PlatformGrok, RateMultiplier: 1},
	}
	account := &Account{ID: 13, Platform: PlatformGrok, Type: AccountTypeAPIKey}

	err := svc.PrepareGrokVideoSettlement(context.Background(), &GrokVideoSettlement{
		RequestedModel: "grok-imagine-video",
		BillingModel:   "grok-imagine-video",
	}, apiKey, account, nil)

	require.ErrorIs(t, err, ErrGrokVideoTokenPricingUnsupported)
	require.Zero(t, repo.createCalls)
}

func TestPrepareGrokVideoSettlementHydratesMediaMultiplierBeforeFreezing(t *testing.T) {
	repo := &grokVideoSettlementRepoStub{}
	groupID := int64(133)
	videoPrice := 0.05
	svc := newOpenAIRecordUsageServiceForTest(nil, nil, nil, nil)
	svc.grokVideoSettlementRepo = repo
	svc.channelService = &ChannelService{groupRepo: &openAIMediaPriceGroupRepoStub{group: &Group{
		ID: groupID, Platform: PlatformGrok, RateMultiplier: 1,
		VideoRateIndependent: true, VideoRateMultiplier: 2, VideoPrice480P: &videoPrice,
	}}}
	apiKey := &APIKey{
		ID: 12, User: &User{ID: 11}, GroupID: &groupID,
		Group: &Group{ID: groupID, Platform: PlatformGrok, RateMultiplier: 1},
	}
	account := &Account{ID: 13, Platform: PlatformGrok, Type: AccountTypeOAuth}
	settlement := &GrokVideoSettlement{
		RequestedModel: "grok-imagine-video", BillingModel: "grok-imagine-video",
		VideoResolution: VideoBillingResolution480P, VideoDurationSeconds: 8,
	}

	err := svc.PrepareGrokVideoSettlement(context.Background(), settlement, apiKey, account, nil)

	require.NoError(t, err)
	require.Equal(t, GrokVideoPricingBasisVideoSecond, settlement.PricingBasis)
	require.InDelta(t, 2, settlement.RateMultiplier, 1e-12)
	require.InDelta(t, 0.8, settlement.ActualCost, 1e-12)
	require.Zero(t, repo.createCalls)
}

func TestCalculateGrokVideoSnapshotCostUsesPreResolvedPricingForCostAndBasis(t *testing.T) {
	groupID := int64(134)
	svc := newOpenAIRecordUsageServiceForTest(nil, nil, nil, nil)
	// A different live resolver proves the snapshot calculation does not resolve again.
	svc.resolver = newOpenAITokenImageChannelPricingResolverForTest(t, groupID, "grok-imagine-video")
	apiKey := &APIKey{GroupID: &groupID, Group: &Group{ID: groupID, Platform: PlatformGrok}}
	resolved := &ResolvedPricing{
		Mode: BillingModePerRequest, Source: PricingSourceChannel, DefaultPerRequestPrice: 0.07,
	}

	cost, basis, err := svc.calculateGrokVideoSnapshotCost(
		context.Background(), "grok-imagine-video", apiKey,
		&OpenAIForwardResult{VideoCount: 1, VideoResolution: VideoBillingResolution480P, VideoDurationSeconds: 8},
		resolved, UsageTokens{}, 1, 1, false,
	)

	require.NoError(t, err)
	require.Equal(t, GrokVideoPricingBasisFixedRequest, basis)
	require.Equal(t, string(BillingModeVideo), cost.BillingMode)
	require.InDelta(t, 0.07, cost.TotalCost, 1e-12)
}

func TestDeferredGrokVideoAccountStatsOverrideIsNotDurationScaled(t *testing.T) {
	accountStatsCost := 0.25
	settlement := &GrokVideoSettlement{
		PricingSnapshotVersion: GrokVideoPricingSnapshotVersion,
		PricingBasis:           GrokVideoPricingBasisVideoSecond,
		VideoDurationSeconds:   8,
		TotalCost:              0.4,
		ActualCost:             0.4,
		AccountStatsCost:       &accountStatsCost,
	}

	snapshot := settlement.DeferredBillingSnapshot(4)

	require.NotNil(t, snapshot)
	require.InDelta(t, 0.2, snapshot.Cost.TotalCost, 1e-12)
	require.NotNil(t, snapshot.AccountStatsCost)
	require.InDelta(t, 0.25, *snapshot.AccountStatsCost, 1e-12)
}

func TestGrokVideoSettlementUsesSubmissionPriceAfterPricingChanges(t *testing.T) {
	repo := &grokVideoSettlementRepoStub{}
	svc, apiKey, account := newGrokVideoRegistrationTestContext(repo)
	billingRepo := &openAIRecordUsageBillingRepoStub{result: &UsageBillingApplyResult{Applied: true}}
	svc.usageBillingRepo = billingRepo
	settlement := &GrokVideoSettlement{
		RequestID: "video-frozen-price", RequestedModel: "grok-imagine-video",
		VideoResolution: VideoBillingResolution480P, VideoDurationSeconds: 8,
		QuotaPlatform: PlatformGrok,
	}

	require.NoError(t, svc.RegisterGrokVideoSettlement(context.Background(), settlement, apiKey, account, nil))
	require.InDelta(t, 0.4, settlement.ActualCost, 1e-12)
	require.Zero(t, billingRepo.calls, "submission freezes price without charging")
	newPrice := 0.50
	apiKey.Group.VideoPrice480P = &newPrice
	apiKey.Group.VideoRateMultiplier = 3

	err := svc.SettleGrokVideoStatus(context.Background(), GrokVideoStatusSettlementInput{
		RequestID: settlement.RequestID, Status: "done", APIKey: apiKey, Account: account,
	})

	require.NoError(t, err)
	require.Equal(t, 1, billingRepo.calls)
	require.InDelta(t, 0.4, billingRepo.lastCmd.BalanceCost, 1e-12)
}

func TestSettleGrokVideoStatusPendingDoesNotReadOrBill(t *testing.T) {
	repo := &grokVideoSettlementRepoStub{}
	svc := &OpenAIGatewayService{grokVideoSettlementRepo: repo}

	err := svc.SettleGrokVideoStatus(context.Background(), GrokVideoStatusSettlementInput{
		RequestID: "video-request-1",
		Status:    "processing",
		APIKey:    &APIKey{ID: 12, User: &User{ID: 11}},
		Account:   &Account{ID: 13},
	})

	require.NoError(t, err)
	require.Zero(t, repo.getCalls)
	require.Zero(t, repo.markTerminalCalls)
}

func TestSettleGrokVideoTerminalFailureNeverBillsAPIKeyOrOAuth(t *testing.T) {
	for _, accountType := range []string{AccountTypeAPIKey, AccountTypeOAuth} {
		for observedStatus, wantStatus := range map[string]string{
			"failed": GrokVideoSettlementStatusFailed, "expired": GrokVideoSettlementStatusExpired,
			"cancelled": GrokVideoSettlementStatusCancelled, "canceled": GrokVideoSettlementStatusCancelled,
		} {
			t.Run(accountType+"/"+observedStatus, func(t *testing.T) {
				repo := &grokVideoSettlementRepoStub{settlement: &GrokVideoSettlement{
					ID: 1, RequestID: "video-request-failed", UserID: 11, APIKeyID: 12,
					AccountID: 13, AccountType: accountType, RequestedModel: "grok-imagine-video",
					Status: GrokVideoSettlementStatusPending,
				}}
				svc := &OpenAIGatewayService{grokVideoSettlementRepo: repo}

				err := svc.SettleGrokVideoStatus(context.Background(), GrokVideoStatusSettlementInput{
					RequestID: "video-request-failed",
					Status:    observedStatus,
					APIKey:    &APIKey{ID: 12, User: &User{ID: 11}},
					Account:   &Account{ID: 13, Type: accountType},
				})

				require.NoError(t, err)
				require.Equal(t, 1, repo.markTerminalCalls)
				require.Equal(t, wantStatus, repo.lastTerminalStatus)
				require.Zero(t, repo.markSettledCalls)
			})
		}
	}
}

func TestSettleGrokVideoStatusDoneBillsExactlyOnceAPIKeyAndOAuth(t *testing.T) {
	for _, accountType := range []string{AccountTypeAPIKey, AccountTypeOAuth} {
		t.Run(accountType, func(t *testing.T) {
			groupID := int64(126)
			videoPrice720P := 0.14
			usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
			billingRepo := &openAIRecordUsageBillingRepoStub{result: &UsageBillingApplyResult{Applied: true}}
			svc := newOpenAIRecordUsageServiceWithBillingRepoForTest(
				usageRepo,
				billingRepo,
				&openAIRecordUsageUserRepoStub{},
				&openAIRecordUsageSubRepoStub{},
				nil,
			)
			settlementRepo := &grokVideoSettlementRepoStub{settlement: &GrokVideoSettlement{
				ID:                     1,
				RequestID:              "video-request-done",
				UserID:                 11,
				APIKeyID:               12,
				GroupID:                &groupID,
				AccountID:              13,
				AccountType:            accountType,
				RequestedModel:         "grok-imagine-video",
				BillingModel:           "grok-imagine-video",
				UpstreamModel:          "grok-imagine-video",
				PricingSnapshotVersion: GrokVideoPricingSnapshotVersion,
				PricingBasis:           GrokVideoPricingBasisVideoSecond,
				BillingMode:            string(BillingModeVideo),
				BillingType:            BillingTypeBalance,
				TotalCost:              0.56,
				ActualCost:             0.56,
				RateMultiplier:         1,
				AccountRateMultiplier:  1,
				VideoResolution:        VideoBillingResolution720P,
				VideoDurationSeconds:   8,
				RequestDuration:        time.Second,
				RequestPayloadHash:     "payload-hash",
				SessionID:              "client-session-done",
				QuotaPlatform:          PlatformGrok,
				ChannelUsageFields:     ChannelUsageFields{OriginalModel: "grok-imagine-video", ChannelMappedModel: "grok-imagine-video"},
				Status:                 GrokVideoSettlementStatusPending,
			}}
			svc.grokVideoSettlementRepo = settlementRepo
			apiKey := &APIKey{
				ID:      12,
				User:    &User{ID: 11},
				GroupID: &groupID,
				Group: &Group{
					ID: groupID, Platform: PlatformGrok, RateMultiplier: 1,
					VideoRateIndependent: true, VideoRateMultiplier: 1, VideoPrice720P: &videoPrice720P,
				},
			}
			input := GrokVideoStatusSettlementInput{
				RequestID:             "video-request-done",
				Status:                "completed",
				ActualDurationSeconds: 10,
				APIKey:                apiKey,
				Account:               &Account{ID: 13, Platform: PlatformGrok, Type: accountType},
			}

			require.NoError(t, svc.SettleGrokVideoStatus(context.Background(), input))
			require.Equal(t, 1, billingRepo.calls)
			require.Equal(t, GrokVideoBillingRequestID("video-request-done"), billingRepo.lastCmd.RequestID)
			require.Equal(t, int64(1), billingRepo.lastCmd.GrokVideoSettlementID)
			require.InDelta(t, 0.70, billingRepo.lastCmd.BalanceCost, 1e-12, "actual duration scales the frozen per-second quote")
			require.Equal(t, 1, usageRepo.calls)
			require.NotNil(t, usageRepo.lastLog.VideoDurationSeconds)
			require.Equal(t, 10, *usageRepo.lastLog.VideoDurationSeconds)
			require.NotNil(t, usageRepo.lastLog.SessionID)
			require.Equal(t, "client-session-done", *usageRepo.lastLog.SessionID)
			require.Equal(t, 1, settlementRepo.markSettledCalls)
			require.Equal(t, GrokVideoSettlementStatusSettled, settlementRepo.settlement.Status)

			require.NoError(t, svc.SettleGrokVideoStatus(context.Background(), input))
			require.Equal(t, 1, billingRepo.calls, "repeated successful polling must not bill twice")
			require.Equal(t, 1, usageRepo.calls, "repeated successful polling must not duplicate usage logs")
		})
	}
}

func TestSettleGrokVideoStatusTreatsConcurrentSettledBillingConflictAsSuccess(t *testing.T) {
	groupID := int64(7)
	settlement := &GrokVideoSettlement{
		ID: 1, RequestID: "video-request-race", UserID: 11, APIKeyID: 12,
		GroupID:   &groupID,
		AccountID: 13, AccountType: AccountTypeAPIKey, RequestedModel: "grok-imagine-video",
		BillingModel: "grok-imagine-video", VideoResolution: VideoBillingResolution480P,
		VideoDurationSeconds: 8, Status: GrokVideoSettlementStatusPending,
	}
	settlementRepo := &grokVideoSettlementRepoStub{settlement: settlement}
	billingRepo := &grokVideoConcurrentBillingRepoStub{settlement: settlement, err: ErrUsageBillingRequestConflict}
	svc := newOpenAIRecordUsageServiceWithBillingRepoForTest(
		&openAIRecordUsageLogRepoStub{}, billingRepo,
		&openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{}, nil,
	)
	svc.grokVideoSettlementRepo = settlementRepo

	err := svc.SettleGrokVideoStatus(context.Background(), GrokVideoStatusSettlementInput{
		RequestID: "video-request-race",
		Status:    "done",
		APIKey: &APIKey{
			ID: 12, User: &User{ID: 11}, GroupID: &groupID,
			Group: &Group{ID: groupID, Platform: PlatformGrok, RateMultiplier: 1},
		},
		Account: &Account{ID: 13, Type: AccountTypeAPIKey, Platform: PlatformGrok},
	})

	require.NoError(t, err)
	require.Equal(t, 1, billingRepo.calls)
	require.Equal(t, GrokVideoSettlementStatusSettled, settlement.Status)
	require.Zero(t, settlementRepo.markSettledCalls)
}

func TestSettleGrokVideoStatusUsesFrozenGroupAndPricingAfterGroupChange(t *testing.T) {
	originalGroupID := int64(7)
	currentGroupID := int64(8)
	settlementRepo := &grokVideoSettlementRepoStub{settlement: &GrokVideoSettlement{
		ID: 1, RequestID: "video-request-group-change", UserID: 11, APIKeyID: 12,
		GroupID: &originalGroupID, AccountID: 13, AccountType: AccountTypeAPIKey,
		RequestedModel: "grok-imagine-video", BillingModel: "grok-imagine-video",
		VideoResolution: VideoBillingResolution480P, VideoDurationSeconds: 8,
		PricingSnapshotVersion: GrokVideoPricingSnapshotVersion,
		PricingBasis:           GrokVideoPricingBasisVideoSecond, BillingMode: string(BillingModeVideo),
		BillingType: BillingTypeBalance, TotalCost: 0.4, ActualCost: 0.4,
		RateMultiplier: 1, AccountRateMultiplier: 1,
		Status: GrokVideoSettlementStatusPending,
	}}
	billingRepo := &openAIRecordUsageBillingRepoStub{}
	svc := newOpenAIRecordUsageServiceWithBillingRepoForTest(
		&openAIRecordUsageLogRepoStub{}, billingRepo,
		&openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{}, nil,
	)
	svc.grokVideoSettlementRepo = settlementRepo

	err := svc.SettleGrokVideoStatus(context.Background(), GrokVideoStatusSettlementInput{
		RequestID: "video-request-group-change",
		Status:    "done",
		APIKey: &APIKey{
			ID: 12, User: &User{ID: 11}, GroupID: &currentGroupID,
			Group: &Group{ID: currentGroupID, Platform: PlatformGrok, RateMultiplier: 1},
		},
		Account: &Account{ID: 13, Type: AccountTypeAPIKey, Platform: PlatformGrok},
	})

	require.NoError(t, err)
	require.Equal(t, 1, billingRepo.calls)
	require.InDelta(t, 0.4, billingRepo.lastCmd.BalanceCost, 1e-12)
	require.Equal(t, GrokVideoSettlementStatusSettled, settlementRepo.settlement.Status)
}

func TestSettleGrokVideoStatusRejectsConflictingStoredTerminalState(t *testing.T) {
	tests := []struct {
		name, stored, observed string
	}{
		{name: "failed then done", stored: GrokVideoSettlementStatusFailed, observed: GrokVideoUpstreamStatusDone},
		{name: "settled then failed", stored: GrokVideoSettlementStatusSettled, observed: GrokVideoUpstreamStatusFailed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &grokVideoSettlementRepoStub{settlement: &GrokVideoSettlement{
				ID: 1, RequestID: "video-terminal-conflict", UserID: 11, APIKeyID: 12,
				AccountID: 13, AccountType: AccountTypeAPIKey, Status: tt.stored,
			}}
			svc := &OpenAIGatewayService{grokVideoSettlementRepo: repo}
			err := svc.SettleGrokVideoStatus(context.Background(), GrokVideoStatusSettlementInput{
				RequestID: "video-terminal-conflict", Status: tt.observed,
				APIKey:  &APIKey{ID: 12, User: &User{ID: 11}},
				Account: &Account{ID: 13, Type: AccountTypeAPIKey},
			})
			require.ErrorIs(t, err, ErrGrokVideoTerminalConflict)
		})
	}
}

func TestSelectGrokMediaVideoLookupAccountBypassesSchedulableState(t *testing.T) {
	account := &Account{
		ID: 13, Platform: PlatformGrok, Type: AccountTypeOAuth,
		Status: StatusDisabled, Schedulable: false, Concurrency: 2,
	}
	svc := &OpenAIGatewayService{accountRepo: &openAIRecordUsageAccountRepoStub{account: account}}

	selection, err := svc.SelectGrokMediaVideoLookupAccount(context.Background(), account.ID)

	require.NoError(t, err)
	require.Same(t, account, selection.Account)
	require.NotNil(t, selection.WaitPlan)
	require.Equal(t, 2, selection.WaitPlan.MaxConcurrency)
}

func TestResolveGrokMediaVideoRequestAccountFallsBackToSettlementRepository(t *testing.T) {
	groupID := int64(7)
	cache := &stubGatewayCache{}
	repo := &grokVideoSettlementRepoStub{settlement: &GrokVideoSettlement{
		RequestID: "video-request-1", UserID: 11, APIKeyID: 12, AccountID: 13,
	}}
	svc := &OpenAIGatewayService{cache: cache, grokVideoSettlementRepo: repo}

	accountID, err := svc.ResolveGrokMediaVideoRequestAccount(context.Background(), &groupID, "video-request-1", 11, 12)

	require.NoError(t, err)
	require.Equal(t, int64(13), accountID)
	require.Equal(t, 1, repo.getCalls)

	accountID, err = svc.ResolveGrokMediaVideoRequestAccount(context.Background(), &groupID, "video-request-1", 11, 12)
	require.NoError(t, err)
	require.Equal(t, int64(13), accountID)
	require.Equal(t, 2, repo.getCalls, "durable ownership remains authoritative over the cache")
}

func TestResolveGrokMediaVideoRequestAccountRepairsStaleCacheBinding(t *testing.T) {
	groupID := int64(7)
	cache := &stubGatewayCache{}
	repo := &grokVideoSettlementRepoStub{settlement: &GrokVideoSettlement{
		RequestID: "video-request-stale", UserID: 11, APIKeyID: 12, AccountID: 13,
	}}
	svc := &OpenAIGatewayService{cache: cache, grokVideoSettlementRepo: repo}
	cacheKey := svc.openAISessionCacheKey(GrokMediaVideoRequestSessionHash("video-request-stale", 11, 12))
	cache.sessionBindings = map[string]int64{cacheKey: 99}

	accountID, err := svc.ResolveGrokMediaVideoRequestAccount(context.Background(), &groupID, "video-request-stale", 11, 12)

	require.NoError(t, err)
	require.Equal(t, int64(13), accountID)
	require.Equal(t, int64(13), cache.sessionBindings[cacheKey])
}

func TestRegisterGrokVideoSettlementReturnsRepositoryError(t *testing.T) {
	wantErr := errors.New("db unavailable")
	repo := &grokVideoSettlementRepoStub{createErr: wantErr}
	svc, apiKey, account := newGrokVideoRegistrationTestContext(repo)

	err := svc.RegisterGrokVideoSettlement(context.Background(), &GrokVideoSettlement{
		RequestID: "video-request-1", RequestedModel: "grok-imagine-video",
	}, apiKey, account, nil)

	require.ErrorIs(t, err, wantErr)
}
