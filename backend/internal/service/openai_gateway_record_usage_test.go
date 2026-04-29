package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

func openAIRecordUsagePtrFloat64(v float64) *float64 { return &v }

func openAIRecordUsagePtrInt(v int) *int { return &v }

func newOpenAITestBillingServiceForResolver() *BillingService {
	bs := &BillingService{fallbackPrices: make(map[string]*ModelPricing)}
	bs.fallbackPrices["gpt-5.4"] = &ModelPricing{
		InputPricePerToken:         3e-6,
		OutputPricePerToken:        15e-6,
		CacheCreationPricePerToken: 3.75e-6,
		CacheReadPricePerToken:     0.3e-6,
		SupportsCacheBreakdown:     false,
	}
	bs.fallbackPrices["gpt-5.1"] = bs.fallbackPrices["gpt-5.4"]
	return bs
}

type openAIChannelRepositoryStub struct {
	listAllFn           func(ctx context.Context) ([]Channel, error)
	getGroupPlatformsFn func(ctx context.Context, groupIDs []int64) (map[int64]string, error)
}

func (s *openAIChannelRepositoryStub) Create(context.Context, *Channel) error { return nil }
func (s *openAIChannelRepositoryStub) GetByID(context.Context, int64) (*Channel, error) {
	return nil, nil
}
func (s *openAIChannelRepositoryStub) Update(context.Context, *Channel) error { return nil }
func (s *openAIChannelRepositoryStub) Delete(context.Context, int64) error    { return nil }
func (s *openAIChannelRepositoryStub) List(context.Context, pagination.PaginationParams, string, string) ([]Channel, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (s *openAIChannelRepositoryStub) ListAll(ctx context.Context) ([]Channel, error) {
	if s.listAllFn != nil {
		return s.listAllFn(ctx)
	}
	return nil, nil
}
func (s *openAIChannelRepositoryStub) ExistsByName(context.Context, string) (bool, error) {
	return false, nil
}
func (s *openAIChannelRepositoryStub) ExistsByNameExcluding(context.Context, string, int64) (bool, error) {
	return false, nil
}
func (s *openAIChannelRepositoryStub) GetGroupIDs(context.Context, int64) ([]int64, error) {
	return nil, nil
}
func (s *openAIChannelRepositoryStub) SetGroupIDs(context.Context, int64, []int64) error { return nil }
func (s *openAIChannelRepositoryStub) GetChannelIDByGroupID(context.Context, int64) (int64, error) {
	return 0, nil
}
func (s *openAIChannelRepositoryStub) GetGroupsInOtherChannels(context.Context, int64, []int64) ([]int64, error) {
	return nil, nil
}
func (s *openAIChannelRepositoryStub) GetGroupPlatforms(ctx context.Context, groupIDs []int64) (map[int64]string, error) {
	if s.getGroupPlatformsFn != nil {
		return s.getGroupPlatformsFn(ctx, groupIDs)
	}
	return nil, nil
}
func (s *openAIChannelRepositoryStub) ListModelPricing(context.Context, int64) ([]ChannelModelPricing, error) {
	return nil, nil
}
func (s *openAIChannelRepositoryStub) CreateModelPricing(context.Context, *ChannelModelPricing) error {
	return nil
}
func (s *openAIChannelRepositoryStub) UpdateModelPricing(context.Context, *ChannelModelPricing) error {
	return nil
}
func (s *openAIChannelRepositoryStub) DeleteModelPricing(context.Context, int64) error { return nil }
func (s *openAIChannelRepositoryStub) ReplaceModelPricing(context.Context, int64, []ChannelModelPricing) error {
	return nil
}

type openAIRecordUsageLogRepoStub struct {
	UsageLogRepository

	inserted   bool
	err        error
	calls      int
	lastLog    *UsageLog
	lastCtxErr error
}

func (s *openAIRecordUsageLogRepoStub) Create(ctx context.Context, log *UsageLog) (bool, error) {
	s.calls++
	s.lastLog = log
	s.lastCtxErr = ctx.Err()
	return s.inserted, s.err
}

type openAIRecordUsageBillingRepoStub struct {
	UsageBillingRepository

	result     *UsageBillingApplyResult
	err        error
	calls      int
	lastCmd    *UsageBillingCommand
	lastCtxErr error
}

func (s *openAIRecordUsageBillingRepoStub) Apply(ctx context.Context, cmd *UsageBillingCommand) (*UsageBillingApplyResult, error) {
	s.calls++
	s.lastCmd = cmd
	s.lastCtxErr = ctx.Err()
	if s.err != nil {
		return nil, s.err
	}
	if s.result != nil {
		return s.result, nil
	}
	return &UsageBillingApplyResult{Applied: true}, nil
}

type openAIRecordUsageUserRepoStub struct {
	UserRepository

	deductCalls int
	deductErr   error
	lastAmount  float64
	lastCtxErr  error
}

func (s *openAIRecordUsageUserRepoStub) DeductBalance(ctx context.Context, id int64, amount float64) error {
	s.deductCalls++
	s.lastAmount = amount
	s.lastCtxErr = ctx.Err()
	return s.deductErr
}

type openAIRecordUsageSubRepoStub struct {
	UserSubscriptionRepository

	incrementCalls int
	incrementErr   error
	lastCtxErr     error
	lastCost       float64
}

func (s *openAIRecordUsageSubRepoStub) IncrementUsage(ctx context.Context, id int64, costUSD float64) error {
	s.incrementCalls++
	s.lastCtxErr = ctx.Err()
	s.lastCost = costUSD
	return s.incrementErr
}

type openAIRecordUsageAPIKeyQuotaStub struct {
	quotaCalls          int
	rateLimitCalls      int
	err                 error
	lastAmount          float64
	lastQuotaCtxErr     error
	lastRateLimitCtxErr error
}

func (s *openAIRecordUsageAPIKeyQuotaStub) UpdateQuotaUsed(ctx context.Context, apiKeyID int64, cost float64) error {
	s.quotaCalls++
	s.lastAmount = cost
	s.lastQuotaCtxErr = ctx.Err()
	return s.err
}

func (s *openAIRecordUsageAPIKeyQuotaStub) UpdateRateLimitUsage(ctx context.Context, apiKeyID int64, cost float64) error {
	s.rateLimitCalls++
	s.lastAmount = cost
	s.lastRateLimitCtxErr = ctx.Err()
	return s.err
}

type openAIUserGroupRateRepoStub struct {
	UserGroupRateRepository

	rate  *float64
	err   error
	calls int
}

func (s *openAIUserGroupRateRepoStub) GetByUserAndGroup(ctx context.Context, userID, groupID int64) (*float64, error) {
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	return s.rate, nil
}

func i64p(v int64) *int64 {
	return &v
}

func usageTestF64p(v float64) *float64 {
	return &v
}

func usageTokensFromOpenAIUsage(usage OpenAIUsage) UsageTokens {
	return UsageTokens{
		InputTokens:         max(usage.InputTokens-usage.CacheReadInputTokens, 0),
		OutputTokens:        usage.OutputTokens,
		CacheCreationTokens: usage.CacheCreationInputTokens,
		CacheReadTokens:     usage.CacheReadInputTokens,
		ImageOutputTokens:   usage.ImageOutputTokens,
	}
}

func newOpenAIRecordUsageServiceForTest(usageRepo UsageLogRepository, userRepo UserRepository, subRepo UserSubscriptionRepository, rateRepo UserGroupRateRepository) *OpenAIGatewayService {
	cfg := &config.Config{}
	cfg.Default.RateMultiplier = 1.1
	svc := NewOpenAIGatewayService(
		nil,
		usageRepo,
		nil,
		userRepo,
		subRepo,
		rateRepo,
		nil,
		cfg,
		nil,
		nil,
		NewBillingService(cfg, nil),
		nil,
		&BillingCacheService{},
		nil,
		&DeferredService{},
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
	)
	svc.userGroupRateResolver = newUserGroupRateResolver(
		rateRepo,
		nil,
		resolveUserGroupRateCacheTTL(cfg),
		nil,
		"service.openai_gateway.test",
	)
	return svc
}

func newOpenAIRecordUsageServiceWithBillingRepoForTest(usageRepo UsageLogRepository, billingRepo UsageBillingRepository, userRepo UserRepository, subRepo UserSubscriptionRepository, rateRepo UserGroupRateRepository) *OpenAIGatewayService {
	svc := newOpenAIRecordUsageServiceForTest(usageRepo, userRepo, subRepo, rateRepo)
	svc.usageBillingRepo = billingRepo
	return svc
}

func newOpenAIChannelResolverForTest(t *testing.T, pricing []ChannelModelPricing) *ModelPricingResolver {
	t.Helper()
	const groupID = 11
	repo := &openAIChannelRepositoryStub{
		listAllFn: func(_ context.Context) ([]Channel, error) {
			return []Channel{{
				ID:           1,
				Name:         "openai-test-channel",
				Status:       StatusActive,
				GroupIDs:     []int64{groupID},
				ModelPricing: pricing,
			}}, nil
		},
		getGroupPlatformsFn: func(_ context.Context, _ []int64) (map[int64]string, error) {
			return map[int64]string{groupID: "openai"}, nil
		},
	}
	cs := NewChannelService(repo, nil, nil, nil)
	bs := newOpenAITestBillingServiceForResolver()
	return NewModelPricingResolver(cs, bs)
}

func expectedOpenAICost(t *testing.T, svc *OpenAIGatewayService, model string, usage OpenAIUsage, multiplier float64) *CostBreakdown {
	t.Helper()

	cost, err := svc.billingService.CalculateCost(model, usageTokensFromOpenAIUsage(usage), multiplier)
	require.NoError(t, err)
	return cost
}

func expectedOpenAIServiceTierCost(t *testing.T, svc *OpenAIGatewayService, model string, usage OpenAIUsage, serviceTier string) *CostBreakdown {
	t.Helper()

	cost, err := svc.billingService.CalculateCostWithServiceTier(model, usageTokensFromOpenAIUsage(usage), 1.0, serviceTier)
	require.NoError(t, err)
	return cost
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func TestOpenAIGatewayServiceRecordUsage_UsesUserSpecificGroupRate(t *testing.T) {
	groupID := int64(11)
	groupRate := 1.4
	userRate := 1.8
	usage := OpenAIUsage{InputTokens: 15, OutputTokens: 4, CacheReadInputTokens: 3}

	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	userRepo := &openAIRecordUsageUserRepoStub{}
	subRepo := &openAIRecordUsageSubRepoStub{}
	rateRepo := &openAIUserGroupRateRepoStub{rate: &userRate}
	svc := newOpenAIRecordUsageServiceForTest(usageRepo, userRepo, subRepo, rateRepo)

	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID: "resp_user_group_rate",
			Usage:     usage,
			Model:     "gpt-5.1",
			Duration:  time.Second,
		},
		APIKey: &APIKey{
			ID:      1001,
			GroupID: i64p(groupID),
			Group: &Group{
				ID:             groupID,
				RateMultiplier: groupRate,
			},
		},
		User:    &User{ID: 2001},
		Account: &Account{ID: 3001},
	})

	require.NoError(t, err)
	require.Equal(t, 1, rateRepo.calls)
	require.NotNil(t, usageRepo.lastLog)
	require.Equal(t, userRate, usageRepo.lastLog.RateMultiplier)
	require.Equal(t, 12, usageRepo.lastLog.InputTokens)
	require.Equal(t, 3, usageRepo.lastLog.CacheReadTokens)

	expected := expectedOpenAICost(t, svc, "gpt-5.1", usage, userRate)
	require.InDelta(t, expected.ActualCost, usageRepo.lastLog.ActualCost, 1e-12)
	require.InDelta(t, expected.ActualCost, userRepo.lastAmount, 1e-12)
	require.Equal(t, 1, userRepo.deductCalls)
}

func TestOpenAIGatewayServiceRecordUsage_NilConfigDefaultsRateMultiplierToOne(t *testing.T) {
	usage := OpenAIUsage{InputTokens: 12, OutputTokens: 3, CacheReadInputTokens: 2}

	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	userRepo := &openAIRecordUsageUserRepoStub{}
	subRepo := &openAIRecordUsageSubRepoStub{}
	svc := newOpenAIRecordUsageServiceForTest(usageRepo, userRepo, subRepo, nil)
	svc.cfg = nil

	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID: "resp_nil_config_rate_multiplier",
			Usage:     usage,
			Model:     "gpt-5.1",
			Duration:  time.Second,
		},
		APIKey:  &APIKey{ID: 1009},
		User:    &User{ID: 2009},
		Account: &Account{ID: 3009},
	})

	require.NoError(t, err)
	require.NotNil(t, usageRepo.lastLog)
	require.Equal(t, 1.0, usageRepo.lastLog.RateMultiplier)

	expected := expectedOpenAICost(t, svc, "gpt-5.1", usage, 1.0)
	require.InDelta(t, expected.ActualCost, usageRepo.lastLog.ActualCost, 1e-12)
	require.InDelta(t, expected.ActualCost, userRepo.lastAmount, 1e-12)
}

func TestOpenAIGatewayServiceRecordUsage_PersistsOpenAIRoutingSelectedGroupFields(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	userRepo := &openAIRecordUsageUserRepoStub{}
	subRepo := &openAIRecordUsageSubRepoStub{}
	svc := newOpenAIRecordUsageServiceForTest(usageRepo, userRepo, subRepo, nil)

	snapshot := &OpenAIRoutingSnapshot{
		TargetGroup:         "exhausted",
		SelectedGroup:       "reserve",
		ScheduleLayer:       "load_balance",
		SelectedAccountID:   i64p(66),
		SelectedAccountName: strPtr("acc-66"),
		RequestedModel:      "gpt-5.4-Sys",
		EffectiveModel:      "gpt-5.4",
		FailoverCount:       1,
		FailoverFinalReason: "selected_exhausted_fallback",
	}

	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID: "resp-routing-usage",
			Usage: OpenAIUsage{
				InputTokens:  12,
				OutputTokens: 3,
			},
			Model:    "gpt-5.4",
			Duration: time.Second,
		},
		APIKey: &APIKey{ID: 1001},
		User:   &User{ID: 2001},
		Account: &Account{
			ID: 3001,
		},
		RoutingSnapshot: snapshot,
	})

	require.NoError(t, err)
	require.NotNil(t, usageRepo.lastLog)
	require.Equal(t, snapshot.RequestedModel, usageRepo.lastLog.Model)
	require.Equal(t, snapshot.RequestedModel, usageRepo.lastLog.RequestedModel)
	require.NotNil(t, usageRepo.lastLog.UpstreamModel)
	require.Equal(t, snapshot.EffectiveModel, *usageRepo.lastLog.UpstreamModel)
	require.Equal(t, &snapshot.TargetGroup, usageRepo.lastLog.RoutingTargetGroup)
	require.Equal(t, &snapshot.SelectedGroup, usageRepo.lastLog.RoutingSelectedGroup)
	require.Equal(t, &snapshot.ScheduleLayer, usageRepo.lastLog.RoutingScheduleLayer)
	require.Equal(t, snapshot.SelectedAccountID, usageRepo.lastLog.RoutingSelectedAccountID)
	require.Equal(t, snapshot.SelectedAccountName, usageRepo.lastLog.RoutingSelectedAccountName)
	require.Equal(t, &snapshot.EffectiveModel, usageRepo.lastLog.RoutingEffectiveModel)
	require.Equal(t, &snapshot.FailoverCount, usageRepo.lastLog.RoutingFailoverCount)
	require.Equal(t, &snapshot.FailoverFinalReason, usageRepo.lastLog.RoutingFailoverFinalReason)
}

func TestUsageAndOps_ReserveTargetsPersistTargetAndSelectedGroupInUsage(t *testing.T) {
	for _, tc := range []struct {
		name        string
		targetGroup AccountTargetGroup
		wantTarget  string
	}{
		{name: "active", targetGroup: TargetGroupActive, wantTarget: "active"},
		{name: "any", targetGroup: TargetGroupAny, wantTarget: "any"},
		{name: "exhausted", targetGroup: TargetGroupExhausted, wantTarget: "exhausted"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
			userRepo := &openAIRecordUsageUserRepoStub{}
			subRepo := &openAIRecordUsageSubRepoStub{}
			svc := newOpenAIRecordUsageServiceForTest(usageRepo, userRepo, subRepo, nil)

			snapshot := NewOpenAIRoutingSnapshot(OpenAIRoutingSnapshotInput{
				TargetGroup:    tc.targetGroup,
				SelectedGroup:  openAISelectedGroupReserve,
				ScheduleLayer:  string(openAIAccountScheduleLayerLoadBalance),
				Account:        &Account{ID: 3101, Name: "reserve-3101"},
				RequestedModel: "gpt-5.5",
				EffectiveModel: "gpt-5.5",
			})

			err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
				Result: &OpenAIForwardResult{
					RequestID: "resp_usage_reserve_" + tc.name,
					Usage: OpenAIUsage{
						InputTokens:  12,
						OutputTokens: 3,
					},
					Model:    "gpt-5.5",
					Duration: time.Second,
				},
				APIKey:          &APIKey{ID: 1001},
				User:            &User{ID: 2001},
				Account:         &Account{ID: 3101},
				RoutingSnapshot: snapshot,
			})

			require.NoError(t, err)
			require.NotNil(t, usageRepo.lastLog)
			require.NotNil(t, usageRepo.lastLog.RoutingTargetGroup)
			require.Equal(t, tc.wantTarget, *usageRepo.lastLog.RoutingTargetGroup)
			require.NotNil(t, usageRepo.lastLog.RoutingSelectedGroup)
			require.Equal(t, openAISelectedGroupReserve, *usageRepo.lastLog.RoutingSelectedGroup)
		})
	}
}

func TestUsageAndOps_AnyReserveCarriesRequestIDAndRoutingSnapshotToUsage(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	userRepo := &openAIRecordUsageUserRepoStub{}
	subRepo := &openAIRecordUsageSubRepoStub{}
	svc := newOpenAIRecordUsageServiceForTest(usageRepo, userRepo, subRepo, nil)

	requestCtx := context.WithValue(context.Background(), ctxkey.ClientRequestID, "req-any-reserve-observable")
	snapshot := NewOpenAIRoutingSnapshot(OpenAIRoutingSnapshotInput{
		TargetGroup:        TargetGroupAny,
		SelectedGroup:      openAISelectedGroupReserve,
		ScheduleLayer:      string(openAIAccountScheduleLayerLoadBalance),
		ProjectionVersion:  55,
		ProjectionModelKey: "gpt-5.5",
		ProjectionBuiltAt:  time.Unix(1_716_000_555, 0).UTC(),
		Account:            &Account{ID: 5501, Name: "reserve-5501"},
		RequestedModel:     "gpt-5.5",
		EffectiveModel:     "gpt-5.5",
	})

	err := svc.RecordUsage(requestCtx, &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID: "upstream-volatile-any-reserve",
			Usage: OpenAIUsage{
				InputTokens:  21,
				OutputTokens: 7,
			},
			Model:    "gpt-5.5",
			Duration: time.Second,
		},
		APIKey:          &APIKey{ID: 1001},
		User:            &User{ID: 2001},
		Account:         &Account{ID: 5501},
		RoutingSnapshot: snapshot,
	})

	require.NoError(t, err)
	require.NotNil(t, usageRepo.lastLog)
	require.Equal(t, "client:req-any-reserve-observable", usageRepo.lastLog.RequestID)
	require.NotNil(t, usageRepo.lastLog.RoutingTargetGroup)
	require.Equal(t, "any", *usageRepo.lastLog.RoutingTargetGroup)
	require.NotNil(t, usageRepo.lastLog.RoutingSelectedGroup)
	require.Equal(t, openAISelectedGroupReserve, *usageRepo.lastLog.RoutingSelectedGroup)
	require.Equal(t, snapshot.SelectedAccountID, usageRepo.lastLog.RoutingSelectedAccountID)
	require.Equal(t, snapshot.SelectedAccountName, usageRepo.lastLog.RoutingSelectedAccountName)
	require.NotNil(t, usageRepo.lastLog.RoutingEffectiveModel)
	require.Equal(t, "gpt-5.5", *usageRepo.lastLog.RoutingEffectiveModel)
}

func TestOpenAIGatewayServiceRecordUsage_IncludesEndpointMetadata(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	userRepo := &openAIRecordUsageUserRepoStub{}
	subRepo := &openAIRecordUsageSubRepoStub{}
	rateRepo := &openAIUserGroupRateRepoStub{}
	svc := newOpenAIRecordUsageServiceForTest(usageRepo, userRepo, subRepo, rateRepo)

	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID: "resp_endpoint_metadata",
			Usage: OpenAIUsage{
				InputTokens:  8,
				OutputTokens: 2,
			},
			Model:    "gpt-5.1",
			Duration: time.Second,
		},
		APIKey: &APIKey{
			ID:    1002,
			Group: &Group{RateMultiplier: 1},
		},
		User:             &User{ID: 2002},
		Account:          &Account{ID: 3002},
		InboundEndpoint:  " /v1/chat/completions ",
		UpstreamEndpoint: " /v1/responses ",
	})

	require.NoError(t, err)
	require.NotNil(t, usageRepo.lastLog)
	require.NotNil(t, usageRepo.lastLog.InboundEndpoint)
	require.Equal(t, "/v1/chat/completions", *usageRepo.lastLog.InboundEndpoint)
	require.NotNil(t, usageRepo.lastLog.UpstreamEndpoint)
	require.Equal(t, "/v1/responses", *usageRepo.lastLog.UpstreamEndpoint)
}

func TestOpenAIGatewayServiceRecordUsage_FallsBackToGroupDefaultRateOnResolverError(t *testing.T) {
	groupID := int64(12)
	groupRate := 1.6
	usage := OpenAIUsage{InputTokens: 10, OutputTokens: 5, CacheReadInputTokens: 2}

	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	userRepo := &openAIRecordUsageUserRepoStub{}
	subRepo := &openAIRecordUsageSubRepoStub{}
	rateRepo := &openAIUserGroupRateRepoStub{err: errors.New("db unavailable")}
	svc := newOpenAIRecordUsageServiceForTest(usageRepo, userRepo, subRepo, rateRepo)

	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID: "resp_group_default_on_error",
			Usage:     usage,
			Model:     "gpt-5.1",
			Duration:  time.Second,
		},
		APIKey: &APIKey{
			ID:      1002,
			GroupID: i64p(groupID),
			Group: &Group{
				ID:             groupID,
				RateMultiplier: groupRate,
			},
		},
		User:    &User{ID: 2002},
		Account: &Account{ID: 3002},
	})

	require.NoError(t, err)
	require.Equal(t, 1, rateRepo.calls)
	require.NotNil(t, usageRepo.lastLog)
	require.Equal(t, groupRate, usageRepo.lastLog.RateMultiplier)

	expected := expectedOpenAICost(t, svc, "gpt-5.1", usage, groupRate)
	require.InDelta(t, expected.ActualCost, userRepo.lastAmount, 1e-12)
}

func TestOpenAIGatewayServiceRecordUsage_FallsBackToGroupDefaultRateWhenResolverMissing(t *testing.T) {
	groupID := int64(13)
	groupRate := 1.25
	usage := OpenAIUsage{InputTokens: 9, OutputTokens: 4, CacheReadInputTokens: 1}

	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	userRepo := &openAIRecordUsageUserRepoStub{}
	subRepo := &openAIRecordUsageSubRepoStub{}
	svc := newOpenAIRecordUsageServiceForTest(usageRepo, userRepo, subRepo, nil)
	svc.userGroupRateResolver = nil

	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID: "resp_group_default_nil_resolver",
			Usage:     usage,
			Model:     "gpt-5.1",
			Duration:  time.Second,
		},
		APIKey: &APIKey{
			ID:      1003,
			GroupID: i64p(groupID),
			Group: &Group{
				ID:             groupID,
				RateMultiplier: groupRate,
			},
		},
		User:    &User{ID: 2003},
		Account: &Account{ID: 3003},
	})

	require.NoError(t, err)
	require.NotNil(t, usageRepo.lastLog)
	require.Equal(t, groupRate, usageRepo.lastLog.RateMultiplier)
}

func TestOpenAIGatewayServiceRecordUsage_DuplicateUsageLogSkipsBilling(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: false}
	billingRepo := &openAIRecordUsageBillingRepoStub{result: &UsageBillingApplyResult{Applied: false}}
	userRepo := &openAIRecordUsageUserRepoStub{}
	subRepo := &openAIRecordUsageSubRepoStub{}
	svc := newOpenAIRecordUsageServiceWithBillingRepoForTest(usageRepo, billingRepo, userRepo, subRepo, nil)

	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID: "resp_duplicate",
			Usage: OpenAIUsage{
				InputTokens:  8,
				OutputTokens: 4,
			},
			Model:    "gpt-5.1",
			Duration: time.Second,
		},
		APIKey:  &APIKey{ID: 1004},
		User:    &User{ID: 2004},
		Account: &Account{ID: 3004},
	})

	require.NoError(t, err)
	require.Equal(t, 1, billingRepo.calls)
	require.Equal(t, 1, usageRepo.calls)
	require.Equal(t, 0, userRepo.deductCalls)
	require.Equal(t, 0, subRepo.incrementCalls)
}

func TestOpenAIGatewayServiceRecordUsage_DuplicateBillingKeySkipsBillingWithRepo(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: false}
	billingRepo := &openAIRecordUsageBillingRepoStub{result: &UsageBillingApplyResult{Applied: false}}
	userRepo := &openAIRecordUsageUserRepoStub{}
	subRepo := &openAIRecordUsageSubRepoStub{}
	quotaSvc := &openAIRecordUsageAPIKeyQuotaStub{}
	svc := newOpenAIRecordUsageServiceWithBillingRepoForTest(usageRepo, billingRepo, userRepo, subRepo, nil)

	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID: "resp_duplicate_billing_key",
			Usage: OpenAIUsage{
				InputTokens:  8,
				OutputTokens: 4,
			},
			Model:    "gpt-5.1",
			Duration: time.Second,
		},
		APIKey: &APIKey{
			ID:    10045,
			Quota: 100,
		},
		User:          &User{ID: 20045},
		Account:       &Account{ID: 30045},
		APIKeyService: quotaSvc,
	})

	require.NoError(t, err)
	require.Equal(t, 1, billingRepo.calls)
	require.Equal(t, 1, usageRepo.calls)
	require.Equal(t, 0, userRepo.deductCalls)
	require.Equal(t, 0, subRepo.incrementCalls)
	require.Equal(t, 0, quotaSvc.quotaCalls)
}

func TestOpenAIGatewayServiceRecordUsage_BillsWhenUsageLogCreateReturnsError(t *testing.T) {
	usage := OpenAIUsage{InputTokens: 8, OutputTokens: 4}
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: false, err: errors.New("usage log batch state uncertain")}
	userRepo := &openAIRecordUsageUserRepoStub{}
	subRepo := &openAIRecordUsageSubRepoStub{}
	svc := newOpenAIRecordUsageServiceForTest(usageRepo, userRepo, subRepo, nil)

	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID: "resp_usage_log_error",
			Usage:     usage,
			Model:     "gpt-5.1",
			Duration:  time.Second,
		},
		APIKey:  &APIKey{ID: 10041},
		User:    &User{ID: 20041},
		Account: &Account{ID: 30041},
	})

	require.NoError(t, err)
	require.Equal(t, 1, usageRepo.calls)
	require.Equal(t, 1, userRepo.deductCalls)
	require.Equal(t, 0, subRepo.incrementCalls)
}

func TestOpenAIGatewayServiceRecordUsage_UsageLogWriteErrorDoesNotSkipBilling(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: false, err: MarkUsageLogCreateNotPersisted(context.Canceled)}
	userRepo := &openAIRecordUsageUserRepoStub{}
	subRepo := &openAIRecordUsageSubRepoStub{}
	quotaSvc := &openAIRecordUsageAPIKeyQuotaStub{}
	svc := newOpenAIRecordUsageServiceForTest(usageRepo, userRepo, subRepo, nil)

	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID: "resp_not_persisted",
			Usage: OpenAIUsage{
				InputTokens:  8,
				OutputTokens: 4,
			},
			Model:    "gpt-5.1",
			Duration: time.Second,
		},
		APIKey: &APIKey{
			ID:    10043,
			Quota: 100,
		},
		User:          &User{ID: 20043},
		Account:       &Account{ID: 30043},
		APIKeyService: quotaSvc,
	})

	require.NoError(t, err)
	require.Equal(t, 1, usageRepo.calls)
	require.Equal(t, 1, userRepo.deductCalls)
	require.Equal(t, 0, subRepo.incrementCalls)
	require.Equal(t, 1, quotaSvc.quotaCalls)
}

func TestOpenAIGatewayServiceRecordUsage_BillingUsesDetachedContext(t *testing.T) {
	usage := OpenAIUsage{InputTokens: 10, OutputTokens: 6, CacheReadInputTokens: 2}
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: false, err: context.DeadlineExceeded}
	userRepo := &openAIRecordUsageUserRepoStub{}
	subRepo := &openAIRecordUsageSubRepoStub{}
	quotaSvc := &openAIRecordUsageAPIKeyQuotaStub{}
	svc := newOpenAIRecordUsageServiceForTest(usageRepo, userRepo, subRepo, nil)

	reqCtx, cancel := context.WithCancel(context.Background())
	cancel()

	err := svc.RecordUsage(reqCtx, &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID: "resp_detached_billing_ctx",
			Usage:     usage,
			Model:     "gpt-5.1",
			Duration:  time.Second,
		},
		APIKey: &APIKey{
			ID:    10042,
			Quota: 100,
		},
		User:          &User{ID: 20042},
		Account:       &Account{ID: 30042},
		APIKeyService: quotaSvc,
	})

	require.NoError(t, err)
	require.Equal(t, 1, userRepo.deductCalls)
	require.NoError(t, userRepo.lastCtxErr)
	require.Equal(t, 1, quotaSvc.quotaCalls)
	require.NoError(t, quotaSvc.lastQuotaCtxErr)
}

func TestOpenAIGatewayServiceRecordUsage_BillingRepoUsesDetachedContext(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{}
	billingRepo := &openAIRecordUsageBillingRepoStub{result: &UsageBillingApplyResult{Applied: true}}
	userRepo := &openAIRecordUsageUserRepoStub{}
	subRepo := &openAIRecordUsageSubRepoStub{}
	svc := newOpenAIRecordUsageServiceWithBillingRepoForTest(usageRepo, billingRepo, userRepo, subRepo, nil)

	reqCtx, cancel := context.WithCancel(context.Background())
	cancel()

	err := svc.RecordUsage(reqCtx, &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID: "resp_detached_billing_repo_ctx",
			Usage: OpenAIUsage{
				InputTokens:  8,
				OutputTokens: 4,
			},
			Model:    "gpt-5.1",
			Duration: time.Second,
		},
		APIKey:  &APIKey{ID: 10046},
		User:    &User{ID: 20046},
		Account: &Account{ID: 30046},
	})

	require.NoError(t, err)
	require.Equal(t, 1, billingRepo.calls)
	require.NoError(t, billingRepo.lastCtxErr)
	require.Equal(t, 1, usageRepo.calls)
	require.NoError(t, usageRepo.lastCtxErr)
}

func TestOpenAIGatewayServiceRecordUsage_BillingFingerprintIncludesRequestPayloadHash(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{}
	billingRepo := &openAIRecordUsageBillingRepoStub{result: &UsageBillingApplyResult{Applied: true}}
	svc := newOpenAIRecordUsageServiceWithBillingRepoForTest(usageRepo, billingRepo, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{}, nil)

	payloadHash := HashUsageRequestPayload([]byte(`{"model":"gpt-5","input":"hello"}`))
	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID: "openai_payload_hash",
			Usage: OpenAIUsage{
				InputTokens:  10,
				OutputTokens: 6,
			},
			Model:    "gpt-5",
			Duration: time.Second,
		},
		APIKey:             &APIKey{ID: 501, Quota: 100},
		User:               &User{ID: 601},
		Account:            &Account{ID: 701},
		RequestPayloadHash: payloadHash,
	})
	require.NoError(t, err)
	require.NotNil(t, billingRepo.lastCmd)
	require.Equal(t, payloadHash, billingRepo.lastCmd.RequestPayloadHash)
}

func TestOpenAIGatewayServiceRecordUsage_UsesFallbackRequestIDForBillingAndUsageLog(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{}
	billingRepo := &openAIRecordUsageBillingRepoStub{result: &UsageBillingApplyResult{Applied: true}}
	userRepo := &openAIRecordUsageUserRepoStub{}
	subRepo := &openAIRecordUsageSubRepoStub{}
	svc := newOpenAIRecordUsageServiceWithBillingRepoForTest(usageRepo, billingRepo, userRepo, subRepo, nil)

	ctx := context.WithValue(context.Background(), ctxkey.RequestID, "req-local-fallback")
	err := svc.RecordUsage(ctx, &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID: "",
			Usage: OpenAIUsage{
				InputTokens:  8,
				OutputTokens: 4,
			},
			Model:    "gpt-5.1",
			Duration: time.Second,
		},
		APIKey:  &APIKey{ID: 10047},
		User:    &User{ID: 20047},
		Account: &Account{ID: 30047},
	})

	require.NoError(t, err)
	require.NotNil(t, billingRepo.lastCmd)
	require.Equal(t, "local:req-local-fallback", billingRepo.lastCmd.RequestID)
	require.NotNil(t, usageRepo.lastLog)
	require.Equal(t, "local:req-local-fallback", usageRepo.lastLog.RequestID)
}

func TestOpenAIGatewayServiceRecordUsage_PrefersClientRequestIDOverUpstreamRequestID(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{}
	billingRepo := &openAIRecordUsageBillingRepoStub{result: &UsageBillingApplyResult{Applied: true}}
	userRepo := &openAIRecordUsageUserRepoStub{}
	subRepo := &openAIRecordUsageSubRepoStub{}
	svc := newOpenAIRecordUsageServiceWithBillingRepoForTest(usageRepo, billingRepo, userRepo, subRepo, nil)

	ctx := context.WithValue(context.Background(), ctxkey.ClientRequestID, "openai-client-stable-123")
	err := svc.RecordUsage(ctx, &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID: "upstream-openai-volatile-456",
			Usage: OpenAIUsage{
				InputTokens:  8,
				OutputTokens: 4,
			},
			Model:    "gpt-5.1",
			Duration: time.Second,
		},
		APIKey:  &APIKey{ID: 10049},
		User:    &User{ID: 20049},
		Account: &Account{ID: 30049},
	})

	require.NoError(t, err)
	require.NotNil(t, billingRepo.lastCmd)
	require.Equal(t, "client:openai-client-stable-123", billingRepo.lastCmd.RequestID)
	require.NotNil(t, usageRepo.lastLog)
	require.Equal(t, "client:openai-client-stable-123", usageRepo.lastLog.RequestID)
}

func TestOpenAIGatewayServiceRecordUsage_GeneratesRequestIDWhenAllSourcesMissing(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{}
	billingRepo := &openAIRecordUsageBillingRepoStub{result: &UsageBillingApplyResult{Applied: true}}
	userRepo := &openAIRecordUsageUserRepoStub{}
	subRepo := &openAIRecordUsageSubRepoStub{}
	svc := newOpenAIRecordUsageServiceWithBillingRepoForTest(usageRepo, billingRepo, userRepo, subRepo, nil)

	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID: "",
			Usage: OpenAIUsage{
				InputTokens:  8,
				OutputTokens: 4,
			},
			Model:    "gpt-5.1",
			Duration: time.Second,
		},
		APIKey:  &APIKey{ID: 10050},
		User:    &User{ID: 20050},
		Account: &Account{ID: 30050},
	})

	require.NoError(t, err)
	require.NotNil(t, billingRepo.lastCmd)
	require.True(t, strings.HasPrefix(billingRepo.lastCmd.RequestID, "generated:"))
	require.NotNil(t, usageRepo.lastLog)
	require.Equal(t, billingRepo.lastCmd.RequestID, usageRepo.lastLog.RequestID)
}

func TestOpenAIGatewayServiceRecordUsage_BillingErrorSkipsUsageLogWrite(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{}
	billingRepo := &openAIRecordUsageBillingRepoStub{err: errors.New("billing tx failed")}
	userRepo := &openAIRecordUsageUserRepoStub{}
	subRepo := &openAIRecordUsageSubRepoStub{}
	svc := newOpenAIRecordUsageServiceWithBillingRepoForTest(usageRepo, billingRepo, userRepo, subRepo, nil)

	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID: "resp_billing_fail",
			Usage: OpenAIUsage{
				InputTokens:  8,
				OutputTokens: 4,
			},
			Model:    "gpt-5.1",
			Duration: time.Second,
		},
		APIKey:  &APIKey{ID: 10048},
		User:    &User{ID: 20048},
		Account: &Account{ID: 30048},
	})

	require.Error(t, err)
	require.Equal(t, 1, billingRepo.calls)
	require.Equal(t, 0, usageRepo.calls)
}

func TestOpenAIGatewayServiceRecordUsage_UpdatesAPIKeyQuotaWhenConfigured(t *testing.T) {
	usage := OpenAIUsage{InputTokens: 10, OutputTokens: 6, CacheReadInputTokens: 2}
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	userRepo := &openAIRecordUsageUserRepoStub{}
	subRepo := &openAIRecordUsageSubRepoStub{}
	quotaSvc := &openAIRecordUsageAPIKeyQuotaStub{}
	svc := newOpenAIRecordUsageServiceForTest(usageRepo, userRepo, subRepo, nil)

	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID: "resp_quota_update",
			Usage:     usage,
			Model:     "gpt-5.1",
			Duration:  time.Second,
		},
		APIKey: &APIKey{
			ID:    1005,
			Quota: 100,
		},
		User:          &User{ID: 2005},
		Account:       &Account{ID: 3005},
		APIKeyService: quotaSvc,
	})

	require.NoError(t, err)
	require.Equal(t, 1, quotaSvc.quotaCalls)
	require.Equal(t, 0, quotaSvc.rateLimitCalls)
	expected := expectedOpenAICost(t, svc, "gpt-5.1", usage, 1.1)
	require.InDelta(t, expected.ActualCost, quotaSvc.lastAmount, 1e-12)
}

func TestOpenAIGatewayServiceRecordUsage_ClampsActualInputTokensToZero(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	userRepo := &openAIRecordUsageUserRepoStub{}
	subRepo := &openAIRecordUsageSubRepoStub{}
	svc := newOpenAIRecordUsageServiceForTest(usageRepo, userRepo, subRepo, nil)

	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID: "resp_clamp_actual_input",
			Usage: OpenAIUsage{
				InputTokens:          2,
				OutputTokens:         1,
				CacheReadInputTokens: 5,
			},
			Model:    "gpt-5.1",
			Duration: time.Second,
		},
		APIKey:  &APIKey{ID: 1006},
		User:    &User{ID: 2006},
		Account: &Account{ID: 3006},
	})

	require.NoError(t, err)
	require.NotNil(t, usageRepo.lastLog)
	require.Equal(t, 0, usageRepo.lastLog.InputTokens)
}

func TestOpenAIGatewayServiceRecordUsage_Gpt54LongContextBillsWholeSession(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	userRepo := &openAIRecordUsageUserRepoStub{}
	subRepo := &openAIRecordUsageSubRepoStub{}
	svc := newOpenAIRecordUsageServiceForTest(usageRepo, userRepo, subRepo, nil)

	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID: "resp_gpt54_long_context",
			Usage: OpenAIUsage{
				InputTokens:  300000,
				OutputTokens: 2000,
			},
			Model:    "gpt-5.4-2026-03-05",
			Duration: time.Second,
		},
		APIKey:  &APIKey{ID: 1014},
		User:    &User{ID: 2014},
		Account: &Account{ID: 3014},
	})

	require.NoError(t, err)
	require.NotNil(t, usageRepo.lastLog)

	expectedInput := 300000 * 2.5e-6 * 2.0
	expectedOutput := 2000 * 15e-6 * 1.5
	require.InDelta(t, expectedInput, usageRepo.lastLog.InputCost, 1e-10)
	require.InDelta(t, expectedOutput, usageRepo.lastLog.OutputCost, 1e-10)
	require.InDelta(t, expectedInput+expectedOutput, usageRepo.lastLog.TotalCost, 1e-10)
	require.InDelta(t, (expectedInput+expectedOutput)*1.1, usageRepo.lastLog.ActualCost, 1e-10)
	require.Equal(t, 1, userRepo.deductCalls)
}

func TestOpenAIGatewayServiceRecordUsage_ServiceTierPriorityUsesFastPricing(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	userRepo := &openAIRecordUsageUserRepoStub{}
	subRepo := &openAIRecordUsageSubRepoStub{}
	svc := newOpenAIRecordUsageServiceForTest(usageRepo, userRepo, subRepo, nil)
	serviceTier := "priority"
	usage := OpenAIUsage{InputTokens: 100, OutputTokens: 50}

	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID:   "resp_service_tier_priority",
			ServiceTier: &serviceTier,
			Usage:       usage,
			Model:       "gpt-5.4",
			Duration:    time.Second,
		},
		APIKey:  &APIKey{ID: 1015},
		User:    &User{ID: 2015},
		Account: &Account{ID: 3015},
	})

	require.NoError(t, err)
	require.NotNil(t, usageRepo.lastLog)
	require.NotNil(t, usageRepo.lastLog.ServiceTier)
	require.Equal(t, serviceTier, *usageRepo.lastLog.ServiceTier)

	baseCost, calcErr := svc.billingService.CalculateCost("gpt-5.4", UsageTokens{InputTokens: 100, OutputTokens: 50}, 1.0)
	require.NoError(t, calcErr)
	require.InDelta(t, baseCost.TotalCost*2, usageRepo.lastLog.TotalCost, 1e-10)
}

func TestOpenAIGatewayServiceRecordUsage_ServiceTierFlexHalvesCost(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	userRepo := &openAIRecordUsageUserRepoStub{}
	subRepo := &openAIRecordUsageSubRepoStub{}
	svc := newOpenAIRecordUsageServiceForTest(usageRepo, userRepo, subRepo, nil)
	serviceTier := "flex"
	usage := OpenAIUsage{InputTokens: 100, OutputTokens: 50, CacheReadInputTokens: 20}

	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID:   "resp_service_tier_flex",
			ServiceTier: &serviceTier,
			Usage:       usage,
			Model:       "gpt-5.4",
			Duration:    time.Second,
		},
		APIKey:  &APIKey{ID: 1016},
		User:    &User{ID: 2016},
		Account: &Account{ID: 3016},
	})

	require.NoError(t, err)
	require.NotNil(t, usageRepo.lastLog)

	baseCost, calcErr := svc.billingService.CalculateCost("gpt-5.4", UsageTokens{InputTokens: 80, OutputTokens: 50, CacheReadTokens: 20}, 1.0)
	require.NoError(t, calcErr)
	require.InDelta(t, baseCost.TotalCost*0.5, usageRepo.lastLog.TotalCost, 1e-10)
}

func TestOpenAIGatewayServiceRecordUsage_ActiveTargetGroupAppliesPriorityAccountMultiplierOnly(t *testing.T) {
	groupID := int64(101)
	baseMultiplier := 1.25
	accountMultiplier := 1.5
	serviceTier := "default"
	usage := OpenAIUsage{InputTokens: 100, OutputTokens: 50, CacheReadInputTokens: 20}

	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	userRepo := &openAIRecordUsageUserRepoStub{}
	subRepo := &openAIRecordUsageSubRepoStub{}
	rateRepo := &openAIUserGroupRateRepoStub{rate: &baseMultiplier}
	svc := newOpenAIRecordUsageServiceForTest(usageRepo, userRepo, subRepo, rateRepo)

	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID:   "resp_priority_account_multiplier",
			ServiceTier: &serviceTier,
			Usage:       usage,
			Model:       "gpt-5.1",
			Duration:    time.Second,
		},
		APIKey: &APIKey{
			ID:      1001,
			GroupID: i64p(groupID),
			Group: &Group{
				ID:             groupID,
				RateMultiplier: 0.9,
			},
		},
		User:    &User{ID: 2001},
		Account: &Account{ID: 3001, RateMultiplier: usageTestF64p(accountMultiplier)},
		RoutingSnapshot: &OpenAIRoutingSnapshot{
			TargetGroup: string(TargetGroupActive),
		},
	})

	require.NoError(t, err)
	require.NotNil(t, usageRepo.lastLog)

	expectedCost := expectedOpenAIServiceTierCost(t, svc, "gpt-5.1", usage, serviceTier)
	expectedEffectiveMultiplier := baseMultiplier * accountMultiplier * 100.0

	require.NotNil(t, usageRepo.lastLog.PriorityAccountMultiplier)
	require.Equal(t, 100.0, *usageRepo.lastLog.PriorityAccountMultiplier)
	require.NotNil(t, usageRepo.lastLog.EffectiveMultiplier)
	require.InDelta(t, expectedEffectiveMultiplier, *usageRepo.lastLog.EffectiveMultiplier, 1e-12)
	require.InDelta(t, expectedCost.TotalCost*expectedEffectiveMultiplier, usageRepo.lastLog.ActualCost, 1e-12)
}

func TestOpenAIGatewayServiceRecordUsage_PriorityServiceTierUsesPriorityUnitPriceWithoutExtraMultiplier(t *testing.T) {
	groupID := int64(102)
	baseMultiplier := 1.35
	accountMultiplier := 1.6
	serviceTier := "priority"
	usage := OpenAIUsage{InputTokens: 80, OutputTokens: 40}

	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	userRepo := &openAIRecordUsageUserRepoStub{}
	subRepo := &openAIRecordUsageSubRepoStub{}
	rateRepo := &openAIUserGroupRateRepoStub{rate: &baseMultiplier}
	svc := newOpenAIRecordUsageServiceForTest(usageRepo, userRepo, subRepo, rateRepo)

	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID:   "resp_fast_mode_multiplier",
			ServiceTier: &serviceTier,
			Usage:       usage,
			Model:       "gpt-5.4",
			Duration:    time.Second,
		},
		APIKey: &APIKey{
			ID:      1002,
			GroupID: i64p(groupID),
			Group: &Group{
				ID:             groupID,
				RateMultiplier: 1.0,
			},
		},
		User:    &User{ID: 2002},
		Account: &Account{ID: 3002, RateMultiplier: usageTestF64p(accountMultiplier)},
		RoutingSnapshot: &OpenAIRoutingSnapshot{
			TargetGroup: string(TargetGroupExhausted),
		},
	})

	require.NoError(t, err)
	require.NotNil(t, usageRepo.lastLog)

	expectedCost := expectedOpenAIServiceTierCost(t, svc, "gpt-5.4", usage, serviceTier)
	expectedEffectiveMultiplier := baseMultiplier * accountMultiplier

	require.NotNil(t, usageRepo.lastLog.PriorityAccountMultiplier)
	require.Equal(t, 1.0, *usageRepo.lastLog.PriorityAccountMultiplier)
	require.NotNil(t, usageRepo.lastLog.EffectiveMultiplier)
	require.InDelta(t, expectedEffectiveMultiplier, *usageRepo.lastLog.EffectiveMultiplier, 1e-12)
	require.InDelta(t, expectedCost.TotalCost*expectedEffectiveMultiplier, usageRepo.lastLog.ActualCost, 1e-12)
	require.NotNil(t, usageRepo.lastLog.EffectiveInputUnitPrice)
	require.NotNil(t, usageRepo.lastLog.EffectiveOutputUnitPrice)
	require.Equal(t, 5e-6, *usageRepo.lastLog.EffectiveInputUnitPrice)
	require.Equal(t, 30e-6, *usageRepo.lastLog.EffectiveOutputUnitPrice)
	require.NotNil(t, usageRepo.lastLog.PricingSource)
	require.Contains(t, *usageRepo.lastLog.PricingSource, "priority_pricing")
}

func TestOpenAIGatewayServiceRecordUsage_ActiveTargetGroupAndPriorityTierCombineWithoutFastExtraMultiplier(t *testing.T) {
	groupID := int64(103)
	baseMultiplier := 1.4
	accountMultiplier := 1.7
	serviceTier := "priority"
	usage := OpenAIUsage{InputTokens: 120, OutputTokens: 60, CacheCreationInputTokens: 10}

	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	userRepo := &openAIRecordUsageUserRepoStub{}
	subRepo := &openAIRecordUsageSubRepoStub{}
	rateRepo := &openAIUserGroupRateRepoStub{rate: &baseMultiplier}
	svc := newOpenAIRecordUsageServiceForTest(usageRepo, userRepo, subRepo, rateRepo)

	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID:   "resp_combined_multiplier",
			ServiceTier: &serviceTier,
			Usage:       usage,
			Model:       "gpt-5.4",
			Duration:    time.Second,
		},
		APIKey: &APIKey{
			ID:      1003,
			GroupID: i64p(groupID),
			Group: &Group{
				ID:             groupID,
				RateMultiplier: 1.0,
			},
		},
		User:    &User{ID: 2003},
		Account: &Account{ID: 3003, RateMultiplier: usageTestF64p(accountMultiplier)},
		RoutingSnapshot: &OpenAIRoutingSnapshot{
			TargetGroup: string(TargetGroupActive),
		},
	})

	require.NoError(t, err)
	require.NotNil(t, usageRepo.lastLog)

	expectedCost := expectedOpenAIServiceTierCost(t, svc, "gpt-5.4", usage, serviceTier)
	expectedEffectiveMultiplier := baseMultiplier * accountMultiplier * 100.0

	require.NotNil(t, usageRepo.lastLog.PriorityAccountMultiplier)
	require.Equal(t, 100.0, *usageRepo.lastLog.PriorityAccountMultiplier)
	require.NotNil(t, usageRepo.lastLog.EffectiveMultiplier)
	require.InDelta(t, expectedEffectiveMultiplier, *usageRepo.lastLog.EffectiveMultiplier, 1e-12)
	require.InDelta(t, expectedCost.TotalCost*expectedEffectiveMultiplier, usageRepo.lastLog.ActualCost, 1e-12)
	require.NotNil(t, usageRepo.lastLog.PricingSource)
	require.Contains(t, *usageRepo.lastLog.PricingSource, "priority_pricing")
	require.Contains(t, *usageRepo.lastLog.PricingSource, "priority_account_multiplier")
}

func TestOpenAIGatewayServiceRecordUsage_ExhaustedTargetGroupKeepsPriorityAccountMultiplierAtOne(t *testing.T) {
	groupID := int64(104)
	baseMultiplier := 1.2
	accountMultiplier := 1.8
	serviceTier := "default"
	usage := OpenAIUsage{InputTokens: 90, OutputTokens: 30, CacheReadInputTokens: 10}

	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	userRepo := &openAIRecordUsageUserRepoStub{}
	subRepo := &openAIRecordUsageSubRepoStub{}
	rateRepo := &openAIUserGroupRateRepoStub{rate: &baseMultiplier}
	svc := newOpenAIRecordUsageServiceForTest(usageRepo, userRepo, subRepo, rateRepo)

	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID:   "resp_exhausted_priority_account_multiplier",
			ServiceTier: &serviceTier,
			Usage:       usage,
			Model:       "gpt-5.1",
			Duration:    time.Second,
		},
		APIKey: &APIKey{
			ID:      1004,
			GroupID: i64p(groupID),
			Group: &Group{
				ID:             groupID,
				RateMultiplier: 1.0,
			},
		},
		User:    &User{ID: 2004},
		Account: &Account{ID: 3004, RateMultiplier: usageTestF64p(accountMultiplier)},
		RoutingSnapshot: &OpenAIRoutingSnapshot{
			TargetGroup: string(TargetGroupExhausted),
		},
	})

	require.NoError(t, err)
	require.NotNil(t, usageRepo.lastLog)

	expectedCost := expectedOpenAIServiceTierCost(t, svc, "gpt-5.1", usage, serviceTier)
	expectedEffectiveMultiplier := baseMultiplier * accountMultiplier

	require.NotNil(t, usageRepo.lastLog.PriorityAccountMultiplier)
	require.Equal(t, 1.0, *usageRepo.lastLog.PriorityAccountMultiplier)
	require.NotNil(t, usageRepo.lastLog.EffectiveMultiplier)
	require.InDelta(t, expectedEffectiveMultiplier, *usageRepo.lastLog.EffectiveMultiplier, 1e-12)
	require.InDelta(t, expectedCost.TotalCost*expectedEffectiveMultiplier, usageRepo.lastLog.ActualCost, 1e-12)
}

func TestOpenAIGatewayServiceRecordUsage_StoresEffectiveUnitPricesAndPricingSource(t *testing.T) {
	serviceTier := "priority"
	usage := OpenAIUsage{InputTokens: 50, OutputTokens: 25, CacheReadInputTokens: 10}

	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	userRepo := &openAIRecordUsageUserRepoStub{}
	subRepo := &openAIRecordUsageSubRepoStub{}
	svc := newOpenAIRecordUsageServiceForTest(usageRepo, userRepo, subRepo, nil)

	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID:   "resp_pricing_source",
			ServiceTier: &serviceTier,
			Usage:       usage,
			Model:       "gpt-5.4",
			Duration:    time.Second,
		},
		APIKey:  &APIKey{ID: 1005},
		User:    &User{ID: 2005},
		Account: &Account{ID: 3005},
	})

	require.NoError(t, err)
	require.NotNil(t, usageRepo.lastLog)
	require.NotNil(t, usageRepo.lastLog.EffectiveInputUnitPrice)
	require.NotNil(t, usageRepo.lastLog.EffectiveOutputUnitPrice)
	require.NotNil(t, usageRepo.lastLog.EffectiveCacheReadUnitPrice)
	require.NotNil(t, usageRepo.lastLog.PricingSource)
	require.Equal(t, 5e-6, *usageRepo.lastLog.EffectiveInputUnitPrice)
	require.Equal(t, 30e-6, *usageRepo.lastLog.EffectiveOutputUnitPrice)
	require.Equal(t, 0.5e-6, *usageRepo.lastLog.EffectiveCacheReadUnitPrice)
	require.Equal(t, "priority_pricing", *usageRepo.lastLog.PricingSource)
}

func TestOpenAIGatewayServiceRecordUsage_PersistsChannelAndImageOutputFields(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	svc := newOpenAIRecordUsageServiceForTest(usageRepo, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{}, nil)
	svc.resolver = newOpenAIChannelResolverForTest(t, []ChannelModelPricing{{
		Platform:    "openai",
		Models:      []string{"gpt-5.4"},
		BillingMode: BillingModeToken,
		InputPrice:  openAIRecordUsagePtrFloat64(10e-6),
		OutputPrice: openAIRecordUsagePtrFloat64(50e-6),
	}})
	usage := OpenAIUsage{InputTokens: 20, OutputTokens: 10, ImageOutputTokens: 4}

	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID: "resp_channel_image_fields",
			Usage:     usage,
			Model:     "gpt-5.4",
			Duration:  time.Second,
		},
		APIKey:  &APIKey{ID: 10, GroupID: i64p(11), Group: &Group{ID: 11, RateMultiplier: 1.0}},
		User:    &User{ID: 20},
		Account: &Account{ID: 30},
		ChannelUsageFields: ChannelUsageFields{
			ChannelID:          7,
			OriginalModel:      "glm",
			ChannelMappedModel: "gpt-5.4",
			BillingModelSource: BillingModelSourceChannelMapped,
			ModelMappingChain:  "glm→gpt-5.4",
		},
	})

	require.NoError(t, err)
	require.NotNil(t, usageRepo.lastLog)
	require.Equal(t, 4, usageRepo.lastLog.ImageOutputTokens)
	require.NotZero(t, usageRepo.lastLog.ImageOutputCost)
	require.NotNil(t, usageRepo.lastLog.ChannelID)
	require.Equal(t, int64(7), *usageRepo.lastLog.ChannelID)
	require.NotNil(t, usageRepo.lastLog.ModelMappingChain)
	require.Equal(t, "glm→gpt-5.4", *usageRepo.lastLog.ModelMappingChain)
	require.NotNil(t, usageRepo.lastLog.BillingTier)
	require.Equal(t, PricingSourceChannel, *usageRepo.lastLog.BillingTier)
	require.NotNil(t, usageRepo.lastLog.BillingMode)
	require.Equal(t, string(BillingModeToken), *usageRepo.lastLog.BillingMode)
	require.NotNil(t, usageRepo.lastLog.EffectiveInputUnitPrice)
	require.NotNil(t, usageRepo.lastLog.EffectiveOutputUnitPrice)
	require.InDelta(t, 10e-6, *usageRepo.lastLog.EffectiveInputUnitPrice, 1e-12)
	require.InDelta(t, 50e-6, *usageRepo.lastLog.EffectiveOutputUnitPrice, 1e-12)
	require.NotNil(t, usageRepo.lastLog.PricingSource)
	require.Equal(t, PricingSourceChannel, *usageRepo.lastLog.PricingSource)
}

func TestNormalizeOpenAIServiceTier(t *testing.T) {
	t.Run("fast maps to priority", func(t *testing.T) {
		got := normalizeOpenAIServiceTier(" fast ")
		require.NotNil(t, got)
		require.Equal(t, "priority", *got)
	})

	t.Run("default ignored", func(t *testing.T) {
		require.Nil(t, normalizeOpenAIServiceTier("default"))
	})

	t.Run("invalid ignored", func(t *testing.T) {
		require.Nil(t, normalizeOpenAIServiceTier("turbo"))
	})
}

func TestExtractOpenAIServiceTier(t *testing.T) {
	require.Equal(t, "priority", *extractOpenAIServiceTier(map[string]any{"service_tier": "fast"}))
	require.Equal(t, "flex", *extractOpenAIServiceTier(map[string]any{"service_tier": "flex"}))
	require.Nil(t, extractOpenAIServiceTier(map[string]any{"service_tier": 1}))
	require.Nil(t, extractOpenAIServiceTier(nil))
}

func TestExtractOpenAIServiceTierFromBody(t *testing.T) {
	require.Equal(t, "priority", *extractOpenAIServiceTierFromBody([]byte(`{"service_tier":"fast"}`)))
	require.Equal(t, "flex", *extractOpenAIServiceTierFromBody([]byte(`{"service_tier":"flex"}`)))
	require.Nil(t, extractOpenAIServiceTierFromBody([]byte(`{"service_tier":"default"}`)))
	require.Nil(t, extractOpenAIServiceTierFromBody(nil))
}

func TestOpenAIGatewayServiceRecordUsage_UsesRequestedModelAndUpstreamModelMetadataFields(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	userRepo := &openAIRecordUsageUserRepoStub{}
	subRepo := &openAIRecordUsageSubRepoStub{}
	svc := newOpenAIRecordUsageServiceForTest(usageRepo, userRepo, subRepo, nil)
	serviceTier := "priority"
	reasoning := "high"

	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID:       "resp_billing_model_override",
			BillingModel:    "gpt-5.1-codex",
			Model:           "gpt-5.1",
			UpstreamModel:   "gpt-5.1-codex",
			ServiceTier:     &serviceTier,
			ReasoningEffort: &reasoning,
			Usage: OpenAIUsage{
				InputTokens:  20,
				OutputTokens: 10,
			},
			Duration:     2 * time.Second,
			FirstTokenMs: func() *int { v := 120; return &v }(),
		},
		APIKey:    &APIKey{ID: 10, GroupID: i64p(11), Group: &Group{ID: 11, RateMultiplier: 1.2}},
		User:      &User{ID: 20},
		Account:   &Account{ID: 30},
		UserAgent: "codex-cli/1.0",
		IPAddress: "127.0.0.1",
	})

	require.NoError(t, err)
	require.NotNil(t, usageRepo.lastLog)
	require.Equal(t, "gpt-5.1", usageRepo.lastLog.Model)
	require.Equal(t, "gpt-5.1", usageRepo.lastLog.RequestedModel)
	require.NotNil(t, usageRepo.lastLog.UpstreamModel)
	require.Equal(t, "gpt-5.1-codex", *usageRepo.lastLog.UpstreamModel)
	require.NotNil(t, usageRepo.lastLog.ServiceTier)
	require.Equal(t, serviceTier, *usageRepo.lastLog.ServiceTier)
	require.NotNil(t, usageRepo.lastLog.ReasoningEffort)
	require.Equal(t, reasoning, *usageRepo.lastLog.ReasoningEffort)
	require.NotNil(t, usageRepo.lastLog.UserAgent)
	require.Equal(t, "codex-cli/1.0", *usageRepo.lastLog.UserAgent)
	require.NotNil(t, usageRepo.lastLog.IPAddress)
	require.Equal(t, "127.0.0.1", *usageRepo.lastLog.IPAddress)
	require.NotNil(t, usageRepo.lastLog.GroupID)
	require.Equal(t, int64(11), *usageRepo.lastLog.GroupID)
	require.Equal(t, 1, userRepo.deductCalls)
}

func TestOpenAIGatewayServiceRecordUsage_BillsMappedRequestsUsingRequestedModel(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	userRepo := &openAIRecordUsageUserRepoStub{}
	subRepo := &openAIRecordUsageSubRepoStub{}
	svc := newOpenAIRecordUsageServiceForTest(usageRepo, userRepo, subRepo, nil)
	usage := OpenAIUsage{InputTokens: 20, OutputTokens: 10}

	// Billing should use the requested model ("gpt-5.1"), not the upstream mapped model ("gpt-5.1-codex").
	// This ensures pricing is always based on the model the user requested.
	expectedCost, err := svc.billingService.CalculateCost("gpt-5.1", UsageTokens{
		InputTokens:  20,
		OutputTokens: 10,
	}, 1.1)
	require.NoError(t, err)

	err = svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID:     "resp_upstream_model_billing_fallback",
			Model:         "gpt-5.1",
			UpstreamModel: "gpt-5.1-codex",
			Usage:         usage,
			Duration:      time.Second,
		},
		APIKey:  &APIKey{ID: 10},
		User:    &User{ID: 20},
		Account: &Account{ID: 30},
	})

	require.NoError(t, err)
	require.NotNil(t, usageRepo.lastLog)
	require.Equal(t, "gpt-5.1", usageRepo.lastLog.Model)
	require.Equal(t, expectedCost.ActualCost, usageRepo.lastLog.ActualCost)
	require.Equal(t, expectedCost.TotalCost, usageRepo.lastLog.TotalCost)
	require.Equal(t, expectedCost.ActualCost, userRepo.lastAmount)
}

func TestOpenAIGatewayServiceRecordUsage_ChannelMappedDoesNotOverrideBillingModelWhenUnmapped(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	userRepo := &openAIRecordUsageUserRepoStub{}
	subRepo := &openAIRecordUsageSubRepoStub{}
	svc := newOpenAIRecordUsageServiceForTest(usageRepo, userRepo, subRepo, nil)
	usage := OpenAIUsage{InputTokens: 20, OutputTokens: 10}

	// When channel did NOT map the model (ChannelMappedModel == OriginalModel),
	// billing should use result.BillingModel (the actual model used after group
	// DefaultMappedModel resolution), not the unmapped original model.
	expectedCost, err := svc.billingService.CalculateCost("gpt-5.1", UsageTokens{
		InputTokens:  20,
		OutputTokens: 10,
	}, 1.0)
	require.NoError(t, err)

	err = svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID:     "resp_channel_unmapped_billing",
			Model:         "glm",
			BillingModel:  "gpt-5.1",
			UpstreamModel: "gpt-5.1",
			Usage:         usage,
			Duration:      time.Second,
		},
		APIKey:  &APIKey{ID: 10, GroupID: i64p(11), Group: &Group{ID: 11, RateMultiplier: 1.0}},
		User:    &User{ID: 20},
		Account: &Account{ID: 30},
		ChannelUsageFields: ChannelUsageFields{
			ChannelID:          1,
			OriginalModel:      "glm",
			ChannelMappedModel: "glm",
			BillingModelSource: BillingModelSourceChannelMapped,
		},
	})

	require.NoError(t, err)
	require.NotNil(t, usageRepo.lastLog)
	require.Equal(t, expectedCost.TotalCost, usageRepo.lastLog.TotalCost)
	require.Equal(t, expectedCost.ActualCost, usageRepo.lastLog.ActualCost)
}

func TestOpenAIGatewayServiceRecordUsage_ChannelMappedOverridesBillingModelWhenMapped(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	userRepo := &openAIRecordUsageUserRepoStub{}
	subRepo := &openAIRecordUsageSubRepoStub{}
	svc := newOpenAIRecordUsageServiceForTest(usageRepo, userRepo, subRepo, nil)
	usage := OpenAIUsage{InputTokens: 20, OutputTokens: 10}

	// When channel DID map the model (ChannelMappedModel != OriginalModel),
	expectedCost, err := svc.billingService.CalculateCost("gpt-5.1", UsageTokens{
		InputTokens:  20,
		OutputTokens: 10,
	}, 1.0)
	require.NoError(t, err)

	err = svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID:     "resp_channel_mapped_billing",
			Model:         "glm",
			BillingModel:  "gpt-5.1-codex",
			UpstreamModel: "gpt-5.1-codex",
			Usage:         usage,
			Duration:      time.Second,
		},
		APIKey:  &APIKey{ID: 10, GroupID: i64p(11), Group: &Group{ID: 11, RateMultiplier: 1.0}},
		User:    &User{ID: 20},
		Account: &Account{ID: 30},
		ChannelUsageFields: ChannelUsageFields{
			ChannelID:          1,
			OriginalModel:      "glm",
			ChannelMappedModel: "gpt-5.1",
			BillingModelSource: BillingModelSourceChannelMapped,
		},
	})

	require.NoError(t, err)
	require.NotNil(t, usageRepo.lastLog)
	require.Equal(t, expectedCost.TotalCost, usageRepo.lastLog.TotalCost)
	require.Equal(t, expectedCost.ActualCost, usageRepo.lastLog.ActualCost)
}

func TestOpenAIGatewayServiceRecordUsage_ChannelPerRequestPricingUsesContextTier(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	userRepo := &openAIRecordUsageUserRepoStub{}
	subRepo := &openAIRecordUsageSubRepoStub{}
	svc := newOpenAIRecordUsageServiceForTest(usageRepo, userRepo, subRepo, nil)
	svc.resolver = newOpenAIChannelResolverForTest(t, []ChannelModelPricing{{
		Platform:        "openai",
		Models:          []string{"gpt-5.4"},
		BillingMode:     BillingModePerRequest,
		PerRequestPrice: openAIRecordUsagePtrFloat64(0.05),
		Intervals: []PricingInterval{
			{MinTokens: 0, MaxTokens: openAIRecordUsagePtrInt(128000), PerRequestPrice: openAIRecordUsagePtrFloat64(0.03)},
			{MinTokens: 128000, MaxTokens: nil, PerRequestPrice: openAIRecordUsagePtrFloat64(0.10)},
		},
	}})
	usage := OpenAIUsage{InputTokens: 20, OutputTokens: 10}

	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID: "resp_channel_per_request_pricing_uses_context_tier",
			Usage:     usage,
			Model:     "gpt-5.4",
			Duration:  time.Second,
		},
		APIKey:  &APIKey{ID: 10, GroupID: i64p(11), Group: &Group{ID: 11, RateMultiplier: 1.0}},
		User:    &User{ID: 20},
		Account: &Account{ID: 30},
		ChannelUsageFields: ChannelUsageFields{
			ChannelID:          1,
			OriginalModel:      "gpt-5.4",
			ChannelMappedModel: "gpt-5.4",
			BillingModelSource: BillingModelSourceChannelMapped,
		},
	})

	require.NoError(t, err)
	require.NotNil(t, usageRepo.lastLog)
	require.NotNil(t, usageRepo.lastLog.BillingMode)
	require.Equal(t, string(BillingModePerRequest), *usageRepo.lastLog.BillingMode)
	require.NotNil(t, usageRepo.lastLog.BillingTier)
	require.Equal(t, PricingSourceChannel, *usageRepo.lastLog.BillingTier)
	require.InDelta(t, 0.03, usageRepo.lastLog.TotalCost, 1e-12)
	require.InDelta(t, 0.03, usageRepo.lastLog.ActualCost, 1e-12)
}

func TestOpenAIGatewayServiceRecordUsage_SubscriptionBillingSetsSubscriptionFields(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	userRepo := &openAIRecordUsageUserRepoStub{}
	subRepo := &openAIRecordUsageSubRepoStub{}
	svc := newOpenAIRecordUsageServiceForTest(usageRepo, userRepo, subRepo, nil)
	subscription := &UserSubscription{ID: 99}
	expectedCost := expectedOpenAICost(t, svc, "gpt-5.1", OpenAIUsage{InputTokens: 10, OutputTokens: 5}, 1)

	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID: "resp_subscription_billing",
			Usage:     OpenAIUsage{InputTokens: 10, OutputTokens: 5},
			Model:     "gpt-5.1",
			Duration:  time.Second,
		},
		APIKey:       &APIKey{ID: 100, GroupID: i64p(88), Group: &Group{ID: 88, SubscriptionType: SubscriptionTypeSubscription, RateMultiplier: 1.0}},
		User:         &User{ID: 200},
		Account:      &Account{ID: 300},
		Subscription: subscription,
	})

	require.NoError(t, err)
	require.NotNil(t, usageRepo.lastLog)
	require.Equal(t, BillingTypeSubscription, usageRepo.lastLog.BillingType)
	require.NotNil(t, usageRepo.lastLog.SubscriptionID)
	require.Equal(t, subscription.ID, *usageRepo.lastLog.SubscriptionID)
	require.Equal(t, 0, userRepo.deductCalls)
	require.InDelta(t, expectedCost.ActualCost, usageRepo.lastLog.ActualCost, 1e-12)
}

func TestPostUsageBilling_SubscriptionUsesActualCostForUsage(t *testing.T) {
	subRepo := &openAIRecordUsageSubRepoStub{}
	cache := &billingCacheWorkerStub{}
	billingCache := NewBillingCacheService(cache, nil, nil, nil, nil, nil, &config.Config{})
	t.Cleanup(billingCache.Stop)

	postUsageBilling(context.Background(), &postUsageBillingParams{
		Cost:               &CostBreakdown{TotalCost: 12.5, ActualCost: 0.125},
		User:               &User{ID: 1},
		APIKey:             &APIKey{ID: 2, GroupID: i64p(88), Group: &Group{ID: 88, SubscriptionType: SubscriptionTypeSubscription}},
		Account:            &Account{ID: 3, Type: AccountTypeOAuth},
		Subscription:       &UserSubscription{ID: 99},
		IsSubscriptionBill: true,
	}, &billingDeps{
		userSubRepo:         subRepo,
		billingCacheService: billingCache,
		deferredService:     &DeferredService{},
	})

	require.Equal(t, 1, subRepo.incrementCalls)
	require.InDelta(t, 0.125, subRepo.lastCost, 1e-12)
}

func TestBuildUsageBillingCommand_SubscriptionUsesActualCost(t *testing.T) {
	usageLog := &UsageLog{SubscriptionID: i64p(99)}
	cmd := buildUsageBillingCommand("req-subscription-cost", usageLog, &postUsageBillingParams{
		Cost:               &CostBreakdown{TotalCost: 12.5, ActualCost: 0.125},
		User:               &User{ID: 1},
		APIKey:             &APIKey{ID: 2, GroupID: i64p(88), Group: &Group{ID: 88, SubscriptionType: SubscriptionTypeSubscription}},
		Account:            &Account{ID: 3, Type: AccountTypeOAuth},
		Subscription:       &UserSubscription{ID: 99},
		IsSubscriptionBill: true,
	})

	require.NotNil(t, cmd)
	require.NotNil(t, cmd.SubscriptionID)
	require.InDelta(t, 0.125, cmd.SubscriptionCost, 1e-12)
}

func TestOpenAIGatewayServiceRecordUsage_SimpleModeSkipsBillingAfterPersist(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	userRepo := &openAIRecordUsageUserRepoStub{}
	subRepo := &openAIRecordUsageSubRepoStub{}
	svc := newOpenAIRecordUsageServiceForTest(usageRepo, userRepo, subRepo, nil)
	svc.cfg.RunMode = config.RunModeSimple

	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID: "resp_simple_mode",
			Usage:     OpenAIUsage{InputTokens: 10, OutputTokens: 5},
			Model:     "gpt-5.1",
			Duration:  time.Second,
		},
		APIKey:  &APIKey{ID: 1000},
		User:    &User{ID: 2000},
		Account: &Account{ID: 3000},
	})

	require.NoError(t, err)
	require.Equal(t, 1, usageRepo.calls)
	require.Equal(t, 0, userRepo.deductCalls)
	require.Equal(t, 0, subRepo.incrementCalls)
}

func TestOpenAIGatewayServiceRecordUsage_ImageOnlyUsageStillPersists(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	userRepo := &openAIRecordUsageUserRepoStub{}
	subRepo := &openAIRecordUsageSubRepoStub{}
	svc := newOpenAIRecordUsageServiceForTest(usageRepo, userRepo, subRepo, nil)

	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID:  "resp_image_only_usage",
			Model:      "gpt-image-2",
			ImageCount: 2,
			ImageSize:  "1K",
			Duration:   time.Second,
		},
		APIKey:  &APIKey{ID: 1007},
		User:    &User{ID: 2007},
		Account: &Account{ID: 3007},
	})

	require.NoError(t, err)
	require.NotNil(t, usageRepo.lastLog)
	require.Equal(t, 2, usageRepo.lastLog.ImageCount)
	require.NotNil(t, usageRepo.lastLog.ImageSize)
	require.Equal(t, "1K", *usageRepo.lastLog.ImageSize)
	require.NotNil(t, usageRepo.lastLog.BillingMode)
	require.Equal(t, string(BillingModeImage), *usageRepo.lastLog.BillingMode)
}

func TestOpenAIGatewayServiceRecordUsage_ImageUsesPerImageBillingEvenWithUsageTokens(t *testing.T) {
	imagePrice := 0.02
	groupID := int64(12)

	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	userRepo := &openAIRecordUsageUserRepoStub{}
	subRepo := &openAIRecordUsageSubRepoStub{}
	svc := newOpenAIRecordUsageServiceForTest(usageRepo, userRepo, subRepo, nil)

	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID: "resp_image_per_request",
			Model:     "gpt-image-2",
			Usage: OpenAIUsage{
				InputTokens:       1110,
				OutputTokens:      1756,
				ImageOutputTokens: 1756,
			},
			ImageCount: 2,
			ImageSize:  "1K",
			Duration:   time.Second,
		},
		APIKey: &APIKey{
			ID:      1008,
			GroupID: i64p(groupID),
			Group: &Group{
				ID:             groupID,
				RateMultiplier: 1.0,
				ImagePrice1K:   &imagePrice,
			},
		},
		User:    &User{ID: 2008},
		Account: &Account{ID: 3008},
	})

	require.NoError(t, err)
	require.NotNil(t, usageRepo.lastLog)
	require.NotNil(t, usageRepo.lastLog.BillingMode)
	require.Equal(t, string(BillingModeImage), *usageRepo.lastLog.BillingMode)
	require.Equal(t, 2, usageRepo.lastLog.ImageCount)
	require.InDelta(t, 0.04, usageRepo.lastLog.TotalCost, 1e-12)
	require.InDelta(t, 0.04, usageRepo.lastLog.ActualCost, 1e-12)
	require.InDelta(t, 0.0, usageRepo.lastLog.InputCost, 1e-12)
	require.InDelta(t, 0.0, usageRepo.lastLog.OutputCost, 1e-12)
	require.InDelta(t, 0.0, usageRepo.lastLog.ImageOutputCost, 1e-12)
}
