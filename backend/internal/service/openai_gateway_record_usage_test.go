package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/stretchr/testify/require"
)

type openAIRecordUsageLogRepoStub struct {
	UsageLogRepository

	inserted   bool
	err        error
	calls      int
	lastLog    *UsageLog
	lastCtxErr error
REDACTED

func (s *openAIRecordUsageLogRepoStub) Create(ctx context.Context, log *UsageLog) (bool, error) {
	s.calls++
	s.lastLog = log
	s.lastCtxErr = ctx.Err()
	return s.inserted, s.err
REDACTED

type openAIRecordUsageBillingRepoStub struct {
	UsageBillingRepository

	result     *UsageBillingApplyResult
	err        error
	calls      int
	lastCmd    *UsageBillingCommand
	lastCtxErr error
REDACTED

func (s *openAIRecordUsageBillingRepoStub) Apply(ctx context.Context, cmd *UsageBillingCommand) (*UsageBillingApplyResult, error) {
	s.calls++
	s.lastCmd = cmd
	s.lastCtxErr = ctx.Err()
	if s.err != nil {
		return nil, s.err
REDACTED
	if s.result != nil {
		return s.result, nil
REDACTED
	return &UsageBillingApplyResult{Applied: trueREDACTED, nil
REDACTED

type openAIRecordUsageUserRepoStub struct {
	UserRepository

	deductCalls int
	deductErr   error
	lastAmount  float64
	lastCtxErr  error
REDACTED

func (s *openAIRecordUsageUserRepoStub) DeductBalance(ctx context.Context, id int64, amount float64) error {
	s.deductCalls++
	s.lastAmount = amount
	s.lastCtxErr = ctx.Err()
	return s.deductErr
REDACTED

type openAIRecordUsageSubRepoStub struct {
	UserSubscriptionRepository

	incrementCalls int
	incrementErr   error
	lastCtxErr     error
REDACTED

func (s *openAIRecordUsageSubRepoStub) IncrementUsage(ctx context.Context, id int64, costUSD float64) error {
	s.incrementCalls++
	s.lastCtxErr = ctx.Err()
	return s.incrementErr
REDACTED

type openAIRecordUsageAPIKeyQuotaStub struct {
	quotaCalls          int
	rateLimitCalls      int
	err                 error
	lastAmount          float64
	lastQuotaCtxErr     error
	lastRateLimitCtxErr error
REDACTED

func (s *openAIRecordUsageAPIKeyQuotaStub) UpdateQuotaUsed(ctx context.Context, apiKeyID int64, cost float64) error {
	s.quotaCalls++
	s.lastAmount = cost
	s.lastQuotaCtxErr = ctx.Err()
	return s.err
REDACTED

func (s *openAIRecordUsageAPIKeyQuotaStub) UpdateRateLimitUsage(ctx context.Context, apiKeyID int64, cost float64) error {
	s.rateLimitCalls++
	s.lastAmount = cost
	s.lastRateLimitCtxErr = ctx.Err()
	return s.err
REDACTED

type openAIUserGroupRateRepoStub struct {
	UserGroupRateRepository

	rate  *float64
	err   error
	calls int
REDACTED

func (s *openAIUserGroupRateRepoStub) GetByUserAndGroup(ctx context.Context, userID, groupID int64) (*float64, error) {
	s.calls++
	if s.err != nil {
		return nil, s.err
REDACTED
	return s.rate, nil
REDACTED

func i64p(v int64) *int64 {
	return &v
REDACTED

func newOpenAIRecordUsageServiceForTest(usageRepo UsageLogRepository, userRepo UserRepository, subRepo UserSubscriptionRepository, rateRepo UserGroupRateRepository) *OpenAIGatewayService {
	cfg := &config.Config{REDACTED
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
		&BillingCacheService{REDACTED,
		nil,
		&DeferredService{REDACTED,
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
REDACTED

func newOpenAIRecordUsageServiceWithBillingRepoForTest(usageRepo UsageLogRepository, billingRepo UsageBillingRepository, userRepo UserRepository, subRepo UserSubscriptionRepository, rateRepo UserGroupRateRepository) *OpenAIGatewayService {
	svc := newOpenAIRecordUsageServiceForTest(usageRepo, userRepo, subRepo, rateRepo)
	svc.usageBillingRepo = billingRepo
	return svc
REDACTED

func expectedOpenAICost(t *testing.T, svc *OpenAIGatewayService, model string, usage OpenAIUsage, multiplier float64) *CostBreakdown {
REDACTED

	cost, err := svc.billingService.CalculateCost(model, UsageTokens{
		InputTokens:         max(usage.InputTokens-usage.CacheReadInputTokens, 0),
		OutputTokens:        usage.OutputTokens,
		CacheCreationTokens: usage.CacheCreationInputTokens,
		CacheReadTokens:     usage.CacheReadInputTokens,
REDACTED, multiplier)
REDACTED
	return cost
REDACTED

func max(a, b int) int {
	if a > b {
		return a
REDACTED
	return b
REDACTED

func TestOpenAIGatewayServiceRecordUsage_ZeroUsageStillWritesUsageLog(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: trueREDACTED
	billingRepo := &openAIRecordUsageBillingRepoStub{result: &UsageBillingApplyResult{Applied: trueREDACTEDREDACTED
	userRepo := &openAIRecordUsageUserRepoStub{REDACTED
	subRepo := &openAIRecordUsageSubRepoStub{REDACTED
	quotaSvc := &openAIRecordUsageAPIKeyQuotaStub{REDACTED
	svc := newOpenAIRecordUsageServiceWithBillingRepoForTest(usageRepo, billingRepo, userRepo, subRepo, nil)

	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID: "resp_zero_usage",
			Usage:     OpenAIUsage{REDACTED,
			Model:     "gpt-5.1",
			Duration:  time.Second,
	REDACTED,
		APIKey:        &APIKey{ID: 1000, Quota: 100, Group: &Group{RateMultiplier: 1REDACTEDREDACTED,
		User:          &User{ID: 2000REDACTED,
		Account:       &Account{ID: 3000, Type: AccountTypeAPIKeyREDACTED,
		APIKeyService: quotaSvc,
REDACTED)

REDACTED
	require.Equal(t, 1, billingRepo.calls)
	require.Equal(t, 1, usageRepo.calls)
	require.Equal(t, 0, userRepo.deductCalls)
	require.Equal(t, 0, subRepo.incrementCalls)
	require.Equal(t, 0, quotaSvc.quotaCalls)
	require.Equal(t, 0, quotaSvc.rateLimitCalls)

	require.NotNil(t, usageRepo.lastLog)
	require.Equal(t, "resp_zero_usage", usageRepo.lastLog.RequestID)
	require.Zero(t, usageRepo.lastLog.InputTokens)
	require.Zero(t, usageRepo.lastLog.OutputTokens)
	require.Zero(t, usageRepo.lastLog.CacheCreationTokens)
	require.Zero(t, usageRepo.lastLog.CacheReadTokens)
	require.Zero(t, usageRepo.lastLog.ImageOutputTokens)
	require.Zero(t, usageRepo.lastLog.ImageCount)
	require.Zero(t, usageRepo.lastLog.InputCost)
	require.Zero(t, usageRepo.lastLog.OutputCost)
	require.Zero(t, usageRepo.lastLog.TotalCost)
	require.Zero(t, usageRepo.lastLog.ActualCost)

	require.NotNil(t, billingRepo.lastCmd)
	require.Zero(t, billingRepo.lastCmd.BalanceCost)
	require.Zero(t, billingRepo.lastCmd.SubscriptionCost)
	require.Zero(t, billingRepo.lastCmd.APIKeyQuotaCost)
	require.Zero(t, billingRepo.lastCmd.APIKeyRateLimitCost)
	require.Zero(t, billingRepo.lastCmd.AccountQuotaCost)
REDACTED

func TestOpenAIGatewayServiceRecordUsage_UsesUserSpecificGroupRate(t *testing.T) {
	groupID := int64(11)
	groupRate := 1.4
	userRate := 1.8
	usage := OpenAIUsage{InputTokens: 15, OutputTokens: 4, CacheReadInputTokens: 3REDACTED

	usageRepo := &openAIRecordUsageLogRepoStub{inserted: trueREDACTED
	userRepo := &openAIRecordUsageUserRepoStub{REDACTED
	subRepo := &openAIRecordUsageSubRepoStub{REDACTED
	rateRepo := &openAIUserGroupRateRepoStub{rate: &userRateREDACTED
	svc := newOpenAIRecordUsageServiceForTest(usageRepo, userRepo, subRepo, rateRepo)

	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID: "resp_user_group_rate",
			Usage:     usage,
			Model:     "gpt-5.1",
			Duration:  time.Second,
	REDACTED,
		APIKey: &APIKey{
			ID:      1001,
			GroupID: i64p(groupID),
			Group: &Group{
				ID:             groupID,
				RateMultiplier: groupRate,
		REDACTED,
	REDACTED,
		User:    &User{ID: 2001REDACTED,
		Account: &Account{ID: 3001REDACTED,
REDACTED)

REDACTED
	require.Equal(t, 1, rateRepo.calls)
	require.NotNil(t, usageRepo.lastLog)
	require.Equal(t, userRate, usageRepo.lastLog.RateMultiplier)
	require.Equal(t, 12, usageRepo.lastLog.InputTokens)
	require.Equal(t, 3, usageRepo.lastLog.CacheReadTokens)

	expected := expectedOpenAICost(t, svc, "gpt-5.1", usage, userRate)
	require.InDelta(t, expected.ActualCost, usageRepo.lastLog.ActualCost, 1e-12)
	require.InDelta(t, expected.ActualCost, userRepo.lastAmount, 1e-12)
	require.Equal(t, 1, userRepo.deductCalls)
REDACTED

func TestOpenAIGatewayServiceRecordUsage_IncludesEndpointMetadata(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: trueREDACTED
	userRepo := &openAIRecordUsageUserRepoStub{REDACTED
	subRepo := &openAIRecordUsageSubRepoStub{REDACTED
	rateRepo := &openAIUserGroupRateRepoStub{REDACTED
	svc := newOpenAIRecordUsageServiceForTest(usageRepo, userRepo, subRepo, rateRepo)

	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID: "resp_endpoint_metadata",
			Usage: OpenAIUsage{
				InputTokens:  8,
				OutputTokens: 2,
		REDACTED,
			Model:    "gpt-5.1",
			Duration: time.Second,
	REDACTED,
		APIKey: &APIKey{
			ID:    1002,
			Group: &Group{RateMultiplier: 1REDACTED,
	REDACTED,
		User:             &User{ID: 2002REDACTED,
		Account:          &Account{ID: 3002REDACTED,
		InboundEndpoint:  " /v1/chat/completions ",
		UpstreamEndpoint: " /v1/responses ",
REDACTED)

REDACTED
	require.NotNil(t, usageRepo.lastLog)
	require.NotNil(t, usageRepo.lastLog.InboundEndpoint)
	require.Equal(t, "/v1/chat/completions", *usageRepo.lastLog.InboundEndpoint)
	require.NotNil(t, usageRepo.lastLog.UpstreamEndpoint)
	require.Equal(t, "/v1/responses", *usageRepo.lastLog.UpstreamEndpoint)
REDACTED

func TestOpenAIGatewayServiceRecordUsage_FallsBackToGroupDefaultRateOnResolverError(t *testing.T) {
	groupID := int64(12)
	groupRate := 1.6
	usage := OpenAIUsage{InputTokens: 10, OutputTokens: 5, CacheReadInputTokens: 2REDACTED

	usageRepo := &openAIRecordUsageLogRepoStub{inserted: trueREDACTED
	userRepo := &openAIRecordUsageUserRepoStub{REDACTED
	subRepo := &openAIRecordUsageSubRepoStub{REDACTED
	rateRepo := &openAIUserGroupRateRepoStub{err: errors.New("db unavailable")REDACTED
	svc := newOpenAIRecordUsageServiceForTest(usageRepo, userRepo, subRepo, rateRepo)

	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID: "resp_group_default_on_error",
			Usage:     usage,
			Model:     "gpt-5.1",
			Duration:  time.Second,
	REDACTED,
		APIKey: &APIKey{
			ID:      1002,
			GroupID: i64p(groupID),
			Group: &Group{
				ID:             groupID,
				RateMultiplier: groupRate,
		REDACTED,
	REDACTED,
		User:    &User{ID: 2002REDACTED,
		Account: &Account{ID: 3002REDACTED,
REDACTED)

REDACTED
	require.Equal(t, 1, rateRepo.calls)
	require.NotNil(t, usageRepo.lastLog)
	require.Equal(t, groupRate, usageRepo.lastLog.RateMultiplier)

	expected := expectedOpenAICost(t, svc, "gpt-5.1", usage, groupRate)
	require.InDelta(t, expected.ActualCost, userRepo.lastAmount, 1e-12)
REDACTED

func TestOpenAIGatewayServiceRecordUsage_FallsBackToGroupDefaultRateWhenResolverMissing(t *testing.T) {
	groupID := int64(13)
	groupRate := 1.25
	usage := OpenAIUsage{InputTokens: 9, OutputTokens: 4, CacheReadInputTokens: 1REDACTED

	usageRepo := &openAIRecordUsageLogRepoStub{inserted: trueREDACTED
	userRepo := &openAIRecordUsageUserRepoStub{REDACTED
	subRepo := &openAIRecordUsageSubRepoStub{REDACTED
	svc := newOpenAIRecordUsageServiceForTest(usageRepo, userRepo, subRepo, nil)
	svc.userGroupRateResolver = nil

	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID: "resp_group_default_nil_resolver",
			Usage:     usage,
			Model:     "gpt-5.1",
			Duration:  time.Second,
	REDACTED,
		APIKey: &APIKey{
			ID:      1003,
			GroupID: i64p(groupID),
			Group: &Group{
				ID:             groupID,
				RateMultiplier: groupRate,
		REDACTED,
	REDACTED,
		User:    &User{ID: 2003REDACTED,
		Account: &Account{ID: 3003REDACTED,
REDACTED)

REDACTED
	require.NotNil(t, usageRepo.lastLog)
	require.Equal(t, groupRate, usageRepo.lastLog.RateMultiplier)
REDACTED

func TestOpenAIGatewayServiceRecordUsage_DuplicateUsageLogSkipsBilling(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: falseREDACTED
	billingRepo := &openAIRecordUsageBillingRepoStub{result: &UsageBillingApplyResult{Applied: falseREDACTEDREDACTED
	userRepo := &openAIRecordUsageUserRepoStub{REDACTED
	subRepo := &openAIRecordUsageSubRepoStub{REDACTED
	svc := newOpenAIRecordUsageServiceWithBillingRepoForTest(usageRepo, billingRepo, userRepo, subRepo, nil)

	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID: "resp_duplicate",
			Usage: OpenAIUsage{
				InputTokens:  8,
				OutputTokens: 4,
		REDACTED,
			Model:    "gpt-5.1",
			Duration: time.Second,
	REDACTED,
		APIKey:  &APIKey{ID: 1004REDACTED,
		User:    &User{ID: 2004REDACTED,
		Account: &Account{ID: 3004REDACTED,
REDACTED)

REDACTED
	require.Equal(t, 1, billingRepo.calls)
	require.Equal(t, 1, usageRepo.calls)
	require.Equal(t, 0, userRepo.deductCalls)
	require.Equal(t, 0, subRepo.incrementCalls)
REDACTED

func TestOpenAIGatewayServiceRecordUsage_DuplicateBillingKeySkipsBillingWithRepo(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: falseREDACTED
	billingRepo := &openAIRecordUsageBillingRepoStub{result: &UsageBillingApplyResult{Applied: falseREDACTEDREDACTED
	userRepo := &openAIRecordUsageUserRepoStub{REDACTED
	subRepo := &openAIRecordUsageSubRepoStub{REDACTED
	quotaSvc := &openAIRecordUsageAPIKeyQuotaStub{REDACTED
	svc := newOpenAIRecordUsageServiceWithBillingRepoForTest(usageRepo, billingRepo, userRepo, subRepo, nil)

	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID: "resp_duplicate_billing_key",
			Usage: OpenAIUsage{
				InputTokens:  8,
				OutputTokens: 4,
		REDACTED,
			Model:    "gpt-5.1",
			Duration: time.Second,
	REDACTED,
		APIKey: &APIKey{
			ID:    10045,
			Quota: 100,
	REDACTED,
		User:          &User{ID: 20045REDACTED,
		Account:       &Account{ID: 30045REDACTED,
		APIKeyService: quotaSvc,
REDACTED)

REDACTED
	require.Equal(t, 1, billingRepo.calls)
	require.Equal(t, 1, usageRepo.calls)
	require.Equal(t, 0, userRepo.deductCalls)
	require.Equal(t, 0, subRepo.incrementCalls)
	require.Equal(t, 0, quotaSvc.quotaCalls)
REDACTED

func TestOpenAIGatewayServiceRecordUsage_BillsWhenUsageLogCreateReturnsError(t *testing.T) {
	usage := OpenAIUsage{InputTokens: 8, OutputTokens: 4REDACTED
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: false, err: errors.New("usage log batch state uncertain")REDACTED
	userRepo := &openAIRecordUsageUserRepoStub{REDACTED
	subRepo := &openAIRecordUsageSubRepoStub{REDACTED
	svc := newOpenAIRecordUsageServiceForTest(usageRepo, userRepo, subRepo, nil)

	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID: "resp_usage_log_error",
			Usage:     usage,
			Model:     "gpt-5.1",
			Duration:  time.Second,
	REDACTED,
		APIKey:  &APIKey{ID: 10041REDACTED,
		User:    &User{ID: 20041REDACTED,
		Account: &Account{ID: 30041REDACTED,
REDACTED)

REDACTED
	require.Equal(t, 1, usageRepo.calls)
	require.Equal(t, 1, userRepo.deductCalls)
	require.Equal(t, 0, subRepo.incrementCalls)
REDACTED

func TestOpenAIGatewayServiceRecordUsage_UsageLogWriteErrorDoesNotSkipBilling(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: false, err: MarkUsageLogCreateNotPersisted(context.Canceled)REDACTED
	userRepo := &openAIRecordUsageUserRepoStub{REDACTED
	subRepo := &openAIRecordUsageSubRepoStub{REDACTED
	quotaSvc := &openAIRecordUsageAPIKeyQuotaStub{REDACTED
	svc := newOpenAIRecordUsageServiceForTest(usageRepo, userRepo, subRepo, nil)

	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID: "resp_not_persisted",
			Usage: OpenAIUsage{
				InputTokens:  8,
				OutputTokens: 4,
		REDACTED,
			Model:    "gpt-5.1",
			Duration: time.Second,
	REDACTED,
		APIKey: &APIKey{
			ID:    10043,
			Quota: 100,
	REDACTED,
		User:          &User{ID: 20043REDACTED,
		Account:       &Account{ID: 30043REDACTED,
		APIKeyService: quotaSvc,
REDACTED)

REDACTED
	require.Equal(t, 1, usageRepo.calls)
	require.Equal(t, 1, userRepo.deductCalls)
	require.Equal(t, 0, subRepo.incrementCalls)
	require.Equal(t, 1, quotaSvc.quotaCalls)
REDACTED

func TestOpenAIGatewayServiceRecordUsage_BillingUsesDetachedContext(t *testing.T) {
	usage := OpenAIUsage{InputTokens: 10, OutputTokens: 6, CacheReadInputTokens: 2REDACTED
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: false, err: context.DeadlineExceededREDACTED
	userRepo := &openAIRecordUsageUserRepoStub{REDACTED
	subRepo := &openAIRecordUsageSubRepoStub{REDACTED
	quotaSvc := &openAIRecordUsageAPIKeyQuotaStub{REDACTED
	svc := newOpenAIRecordUsageServiceForTest(usageRepo, userRepo, subRepo, nil)

	reqCtx, cancel := context.WithCancel(context.Background())
	cancel()

	err := svc.RecordUsage(reqCtx, &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID: "resp_detached_billing_ctx",
			Usage:     usage,
			Model:     "gpt-5.1",
			Duration:  time.Second,
	REDACTED,
		APIKey: &APIKey{
			ID:    10042,
			Quota: 100,
	REDACTED,
		User:          &User{ID: 20042REDACTED,
		Account:       &Account{ID: 30042REDACTED,
		APIKeyService: quotaSvc,
REDACTED)

REDACTED
	require.Equal(t, 1, userRepo.deductCalls)
	require.NoError(t, userRepo.lastCtxErr)
	require.Equal(t, 1, quotaSvc.quotaCalls)
	require.NoError(t, quotaSvc.lastQuotaCtxErr)
REDACTED

func TestOpenAIGatewayServiceRecordUsage_BillingRepoUsesDetachedContext(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{REDACTED
	billingRepo := &openAIRecordUsageBillingRepoStub{result: &UsageBillingApplyResult{Applied: trueREDACTEDREDACTED
	userRepo := &openAIRecordUsageUserRepoStub{REDACTED
	subRepo := &openAIRecordUsageSubRepoStub{REDACTED
	svc := newOpenAIRecordUsageServiceWithBillingRepoForTest(usageRepo, billingRepo, userRepo, subRepo, nil)

	reqCtx, cancel := context.WithCancel(context.Background())
	cancel()

	err := svc.RecordUsage(reqCtx, &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID: "resp_detached_billing_repo_ctx",
			Usage: OpenAIUsage{
				InputTokens:  8,
				OutputTokens: 4,
		REDACTED,
			Model:    "gpt-5.1",
			Duration: time.Second,
	REDACTED,
		APIKey:  &APIKey{ID: 10046REDACTED,
		User:    &User{ID: 20046REDACTED,
		Account: &Account{ID: 30046REDACTED,
REDACTED)

REDACTED
	require.Equal(t, 1, billingRepo.calls)
	require.NoError(t, billingRepo.lastCtxErr)
	require.Equal(t, 1, usageRepo.calls)
	require.NoError(t, usageRepo.lastCtxErr)
REDACTED

func TestOpenAIGatewayServiceRecordUsage_BillingFingerprintIncludesRequestPayloadHash(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{REDACTED
	billingRepo := &openAIRecordUsageBillingRepoStub{result: &UsageBillingApplyResult{Applied: trueREDACTEDREDACTED
	svc := newOpenAIRecordUsageServiceWithBillingRepoForTest(usageRepo, billingRepo, &openAIRecordUsageUserRepoStub{REDACTED, &openAIRecordUsageSubRepoStub{REDACTED, nil)

	payloadHash := HashUsageRequestPayload([]byte(`{"model":"gpt-5","input":"hello"REDACTED`))
	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID: "openai_payload_hash",
			Usage: OpenAIUsage{
				InputTokens:  10,
				OutputTokens: 6,
		REDACTED,
			Model:    "gpt-5",
			Duration: time.Second,
	REDACTED,
		APIKey:             &APIKey{ID: 501, Quota: 100REDACTED,
		User:               &User{ID: 601REDACTED,
		Account:            &Account{ID: 701REDACTED,
		RequestPayloadHash: payloadHash,
REDACTED)
REDACTED
	require.NotNil(t, billingRepo.lastCmd)
	require.Equal(t, payloadHash, billingRepo.lastCmd.RequestPayloadHash)
REDACTED

func TestOpenAIGatewayServiceRecordUsage_UsesFallbackRequestIDForBillingAndUsageLog(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{REDACTED
	billingRepo := &openAIRecordUsageBillingRepoStub{result: &UsageBillingApplyResult{Applied: trueREDACTEDREDACTED
	userRepo := &openAIRecordUsageUserRepoStub{REDACTED
	subRepo := &openAIRecordUsageSubRepoStub{REDACTED
	svc := newOpenAIRecordUsageServiceWithBillingRepoForTest(usageRepo, billingRepo, userRepo, subRepo, nil)

	ctx := context.WithValue(context.Background(), ctxkey.RequestID, "req-local-fallback")
	err := svc.RecordUsage(ctx, &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID: "",
			Usage: OpenAIUsage{
				InputTokens:  8,
				OutputTokens: 4,
		REDACTED,
			Model:    "gpt-5.1",
			Duration: time.Second,
	REDACTED,
		APIKey:  &APIKey{ID: 10047REDACTED,
		User:    &User{ID: 20047REDACTED,
		Account: &Account{ID: 30047REDACTED,
REDACTED)

REDACTED
	require.NotNil(t, billingRepo.lastCmd)
	require.Equal(t, "local:req-local-fallback", billingRepo.lastCmd.RequestID)
	require.NotNil(t, usageRepo.lastLog)
	require.Equal(t, "local:req-local-fallback", usageRepo.lastLog.RequestID)
REDACTED

func TestOpenAIGatewayServiceRecordUsage_PrefersClientRequestIDOverUpstreamRequestID(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{REDACTED
	billingRepo := &openAIRecordUsageBillingRepoStub{result: &UsageBillingApplyResult{Applied: trueREDACTEDREDACTED
	userRepo := &openAIRecordUsageUserRepoStub{REDACTED
	subRepo := &openAIRecordUsageSubRepoStub{REDACTED
	svc := newOpenAIRecordUsageServiceWithBillingRepoForTest(usageRepo, billingRepo, userRepo, subRepo, nil)

	ctx := context.WithValue(context.Background(), ctxkey.ClientRequestID, "openai-client-stable-123")
	err := svc.RecordUsage(ctx, &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID: "upstream-openai-volatile-456",
			Usage: OpenAIUsage{
				InputTokens:  8,
				OutputTokens: 4,
		REDACTED,
			Model:    "gpt-5.1",
			Duration: time.Second,
	REDACTED,
		APIKey:  &APIKey{ID: 10049REDACTED,
		User:    &User{ID: 20049REDACTED,
		Account: &Account{ID: 30049REDACTED,
REDACTED)

REDACTED
	require.NotNil(t, billingRepo.lastCmd)
	require.Equal(t, "client:openai-client-stable-123", billingRepo.lastCmd.RequestID)
	require.NotNil(t, usageRepo.lastLog)
	require.Equal(t, "client:openai-client-stable-123", usageRepo.lastLog.RequestID)
REDACTED

func TestOpenAIGatewayServiceRecordUsage_GeneratesRequestIDWhenAllSourcesMissing(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{REDACTED
	billingRepo := &openAIRecordUsageBillingRepoStub{result: &UsageBillingApplyResult{Applied: trueREDACTEDREDACTED
	userRepo := &openAIRecordUsageUserRepoStub{REDACTED
	subRepo := &openAIRecordUsageSubRepoStub{REDACTED
	svc := newOpenAIRecordUsageServiceWithBillingRepoForTest(usageRepo, billingRepo, userRepo, subRepo, nil)

	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID: "",
			Usage: OpenAIUsage{
				InputTokens:  8,
				OutputTokens: 4,
		REDACTED,
			Model:    "gpt-5.1",
			Duration: time.Second,
	REDACTED,
		APIKey:  &APIKey{ID: 10050REDACTED,
		User:    &User{ID: 20050REDACTED,
		Account: &Account{ID: 30050REDACTED,
REDACTED)

REDACTED
	require.NotNil(t, billingRepo.lastCmd)
	require.True(t, strings.HasPrefix(billingRepo.lastCmd.RequestID, "generated:"))
	require.NotNil(t, usageRepo.lastLog)
	require.Equal(t, billingRepo.lastCmd.RequestID, usageRepo.lastLog.RequestID)
REDACTED

func TestOpenAIGatewayServiceRecordUsage_BillingErrorSkipsUsageLogWrite(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{REDACTED
	billingRepo := &openAIRecordUsageBillingRepoStub{err: errors.New("billing tx failed")REDACTED
	userRepo := &openAIRecordUsageUserRepoStub{REDACTED
	subRepo := &openAIRecordUsageSubRepoStub{REDACTED
	svc := newOpenAIRecordUsageServiceWithBillingRepoForTest(usageRepo, billingRepo, userRepo, subRepo, nil)

	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID: "resp_billing_fail",
			Usage: OpenAIUsage{
				InputTokens:  8,
				OutputTokens: 4,
		REDACTED,
			Model:    "gpt-5.1",
			Duration: time.Second,
	REDACTED,
		APIKey:  &APIKey{ID: 10048REDACTED,
		User:    &User{ID: 20048REDACTED,
		Account: &Account{ID: 30048REDACTED,
REDACTED)

REDACTED
	require.Equal(t, 1, billingRepo.calls)
	require.Equal(t, 0, usageRepo.calls)
REDACTED

func TestOpenAIGatewayServiceRecordUsage_UpdatesAPIKeyQuotaWhenConfigured(t *testing.T) {
	usage := OpenAIUsage{InputTokens: 10, OutputTokens: 6, CacheReadInputTokens: 2REDACTED
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: trueREDACTED
	userRepo := &openAIRecordUsageUserRepoStub{REDACTED
	subRepo := &openAIRecordUsageSubRepoStub{REDACTED
	quotaSvc := &openAIRecordUsageAPIKeyQuotaStub{REDACTED
	svc := newOpenAIRecordUsageServiceForTest(usageRepo, userRepo, subRepo, nil)

	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID: "resp_quota_update",
			Usage:     usage,
			Model:     "gpt-5.1",
			Duration:  time.Second,
	REDACTED,
		APIKey: &APIKey{
			ID:    1005,
			Quota: 100,
	REDACTED,
		User:          &User{ID: 2005REDACTED,
		Account:       &Account{ID: 3005REDACTED,
		APIKeyService: quotaSvc,
REDACTED)

REDACTED
	require.Equal(t, 1, quotaSvc.quotaCalls)
	require.Equal(t, 0, quotaSvc.rateLimitCalls)
	expected := expectedOpenAICost(t, svc, "gpt-5.1", usage, 1.1)
	require.InDelta(t, expected.ActualCost, quotaSvc.lastAmount, 1e-12)
REDACTED

func TestOpenAIGatewayServiceRecordUsage_ClampsActualInputTokensToZero(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: trueREDACTED
	userRepo := &openAIRecordUsageUserRepoStub{REDACTED
	subRepo := &openAIRecordUsageSubRepoStub{REDACTED
	svc := newOpenAIRecordUsageServiceForTest(usageRepo, userRepo, subRepo, nil)

	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID: "resp_clamp_actual_input",
			Usage: OpenAIUsage{
				InputTokens:          2,
				OutputTokens:         1,
				CacheReadInputTokens: 5,
		REDACTED,
			Model:    "gpt-5.1",
			Duration: time.Second,
	REDACTED,
		APIKey:  &APIKey{ID: 1006REDACTED,
		User:    &User{ID: 2006REDACTED,
		Account: &Account{ID: 3006REDACTED,
REDACTED)

REDACTED
	require.NotNil(t, usageRepo.lastLog)
	require.Equal(t, 0, usageRepo.lastLog.InputTokens)
REDACTED

func TestOpenAIGatewayServiceRecordUsage_Gpt54LongContextBillsWholeSession(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: trueREDACTED
	userRepo := &openAIRecordUsageUserRepoStub{REDACTED
	subRepo := &openAIRecordUsageSubRepoStub{REDACTED
	svc := newOpenAIRecordUsageServiceForTest(usageRepo, userRepo, subRepo, nil)

	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID: "resp_gpt54_long_context",
			Usage: OpenAIUsage{
				InputTokens:  300000,
				OutputTokens: 2000,
		REDACTED,
			Model:    "gpt-5.4-2026-03-05",
			Duration: time.Second,
	REDACTED,
		APIKey:  &APIKey{ID: 1014REDACTED,
		User:    &User{ID: 2014REDACTED,
		Account: &Account{ID: 3014REDACTED,
REDACTED)

REDACTED
	require.NotNil(t, usageRepo.lastLog)

	expectedInput := 300000 * 2.5e-6 * 2.0
	expectedOutput := 2000 * 15e-6 * 1.5
	require.InDelta(t, expectedInput, usageRepo.lastLog.InputCost, 1e-10)
	require.InDelta(t, expectedOutput, usageRepo.lastLog.OutputCost, 1e-10)
	require.InDelta(t, expectedInput+expectedOutput, usageRepo.lastLog.TotalCost, 1e-10)
	require.InDelta(t, (expectedInput+expectedOutput)*1.1, usageRepo.lastLog.ActualCost, 1e-10)
	require.Equal(t, 1, userRepo.deductCalls)
REDACTED

func TestOpenAIGatewayServiceRecordUsage_ServiceTierPriorityUsesFastPricing(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: trueREDACTED
	userRepo := &openAIRecordUsageUserRepoStub{REDACTED
	subRepo := &openAIRecordUsageSubRepoStub{REDACTED
	svc := newOpenAIRecordUsageServiceForTest(usageRepo, userRepo, subRepo, nil)
	serviceTier := "priority"
	usage := OpenAIUsage{InputTokens: 100, OutputTokens: 50REDACTED

	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID:   "resp_service_tier_priority",
			ServiceTier: &serviceTier,
			Usage:       usage,
			Model:       "gpt-5.4",
			Duration:    time.Second,
	REDACTED,
		APIKey:  &APIKey{ID: 1015REDACTED,
		User:    &User{ID: 2015REDACTED,
		Account: &Account{ID: 3015REDACTED,
REDACTED)

REDACTED
	require.NotNil(t, usageRepo.lastLog)
	require.NotNil(t, usageRepo.lastLog.ServiceTier)
	require.Equal(t, serviceTier, *usageRepo.lastLog.ServiceTier)

	baseCost, calcErr := svc.billingService.CalculateCost("gpt-5.4", UsageTokens{InputTokens: 100, OutputTokens: 50REDACTED, 1.0)
	require.NoError(t, calcErr)
	require.InDelta(t, baseCost.TotalCost*2, usageRepo.lastLog.TotalCost, 1e-10)
REDACTED

func TestOpenAIGatewayServiceRecordUsage_ServiceTierFlexHalvesCost(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: trueREDACTED
	userRepo := &openAIRecordUsageUserRepoStub{REDACTED
	subRepo := &openAIRecordUsageSubRepoStub{REDACTED
	svc := newOpenAIRecordUsageServiceForTest(usageRepo, userRepo, subRepo, nil)
	serviceTier := "flex"
	usage := OpenAIUsage{InputTokens: 100, OutputTokens: 50, CacheReadInputTokens: 20REDACTED

	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID:   "resp_service_tier_flex",
			ServiceTier: &serviceTier,
			Usage:       usage,
			Model:       "gpt-5.4",
			Duration:    time.Second,
	REDACTED,
		APIKey:  &APIKey{ID: 1016REDACTED,
		User:    &User{ID: 2016REDACTED,
		Account: &Account{ID: 3016REDACTED,
REDACTED)

REDACTED
	require.NotNil(t, usageRepo.lastLog)

	baseCost, calcErr := svc.billingService.CalculateCost("gpt-5.4", UsageTokens{InputTokens: 80, OutputTokens: 50, CacheReadTokens: 20REDACTED, 1.0)
	require.NoError(t, calcErr)
	require.InDelta(t, baseCost.TotalCost*0.5, usageRepo.lastLog.TotalCost, 1e-10)
REDACTED

func TestNormalizeOpenAIServiceTier(t *testing.T) {
	t.Run("fast maps to priority", func(t *testing.T) {
		got := normalizeOpenAIServiceTier(" fast ")
		require.NotNil(t, got)
		require.Equal(t, "priority", *got)
REDACTED)

	t.Run("openai official tiers preserved", func(t *testing.T) {
		// OpenAI 官方文档定义的合法 tier 值都应被透传保留，避免因白名单过窄
		// 静默剥离客户端显式发送的合法字段。Codex 客户端只发 priority/flex，
		// 所以扩大白名单对 Codex 流量零影响（见 codex-rs/core/src/client.rs）。
		for _, tier := range []string{"priority", "flex", "auto", "default", "scale"REDACTED {
			got := normalizeOpenAIServiceTier(tier)
			require.NotNil(t, got, "tier %q should not be normalized to nil", tier)
			require.Equal(t, tier, *got)
	REDACTED
REDACTED)

	t.Run("invalid ignored", func(t *testing.T) {
		require.Nil(t, normalizeOpenAIServiceTier("turbo"))
		require.Nil(t, normalizeOpenAIServiceTier("xxx"))
REDACTED)
REDACTED

func TestExtractOpenAIServiceTier(t *testing.T) {
	require.Equal(t, "priority", *extractOpenAIServiceTier(map[string]any{"service_tier": "fast"REDACTED))
	require.Equal(t, "flex", *extractOpenAIServiceTier(map[string]any{"service_tier": "flex"REDACTED))
	require.Equal(t, "auto", *extractOpenAIServiceTier(map[string]any{"service_tier": "auto"REDACTED))
	require.Equal(t, "default", *extractOpenAIServiceTier(map[string]any{"service_tier": "default"REDACTED))
	require.Equal(t, "scale", *extractOpenAIServiceTier(map[string]any{"service_tier": "scale"REDACTED))
	require.Nil(t, extractOpenAIServiceTier(map[string]any{"service_tier": 1REDACTED))
	require.Nil(t, extractOpenAIServiceTier(nil))
REDACTED

func TestExtractOpenAIServiceTierFromBody(t *testing.T) {
	require.Equal(t, "priority", *extractOpenAIServiceTierFromBody([]byte(`{"service_tier":"fast"REDACTED`)))
	require.Equal(t, "flex", *extractOpenAIServiceTierFromBody([]byte(`{"service_tier":"flex"REDACTED`)))
	require.Equal(t, "auto", *extractOpenAIServiceTierFromBody([]byte(`{"service_tier":"auto"REDACTED`)))
	require.Equal(t, "default", *extractOpenAIServiceTierFromBody([]byte(`{"service_tier":"default"REDACTED`)))
	require.Equal(t, "scale", *extractOpenAIServiceTierFromBody([]byte(`{"service_tier":"scale"REDACTED`)))
	require.Nil(t, extractOpenAIServiceTierFromBody([]byte(`{"service_tier":"turbo"REDACTED`)))
	require.Nil(t, extractOpenAIServiceTierFromBody(nil))
REDACTED

func TestOpenAIGatewayServiceRecordUsage_UsesRequestedModelAndUpstreamModelMetadataFields(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: trueREDACTED
	userRepo := &openAIRecordUsageUserRepoStub{REDACTED
	subRepo := &openAIRecordUsageSubRepoStub{REDACTED
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
		REDACTED,
			Duration:     2 * time.Second,
			FirstTokenMs: func() *int { v := 120; return &v REDACTED(),
	REDACTED,
		APIKey:    &APIKey{ID: 10, GroupID: i64p(11), Group: &Group{ID: 11, RateMultiplier: 1.2REDACTEDREDACTED,
		User:      &User{ID: 20REDACTED,
		Account:   &Account{ID: 30REDACTED,
		UserAgent: "codex-cli/1.0",
		IPAddress: "127.0.0.1",
REDACTED)

REDACTED
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
REDACTED

func TestOpenAIGatewayServiceRecordUsage_BillsMappedRequestsUsingRequestedModel(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: trueREDACTED
	userRepo := &openAIRecordUsageUserRepoStub{REDACTED
	subRepo := &openAIRecordUsageSubRepoStub{REDACTED
	svc := newOpenAIRecordUsageServiceForTest(usageRepo, userRepo, subRepo, nil)
	usage := OpenAIUsage{InputTokens: 20, OutputTokens: 10REDACTED

	// Billing should use the requested model ("gpt-5.1"), not the upstream mapped model ("gpt-5.1-codex").
	// This ensures pricing is always based on the model the user requested.
	expectedCost, err := svc.billingService.CalculateCost("gpt-5.1", UsageTokens{
		InputTokens:  20,
		OutputTokens: 10,
REDACTED, 1.1)
REDACTED

	err = svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID:     "resp_upstream_model_billing_fallback",
			Model:         "gpt-5.1",
			UpstreamModel: "gpt-5.1-codex",
			Usage:         usage,
			Duration:      time.Second,
	REDACTED,
		APIKey:  &APIKey{ID: 10REDACTED,
		User:    &User{ID: 20REDACTED,
		Account: &Account{ID: 30REDACTED,
REDACTED)

REDACTED
	require.NotNil(t, usageRepo.lastLog)
	require.Equal(t, "gpt-5.1", usageRepo.lastLog.Model)
	require.Equal(t, expectedCost.ActualCost, usageRepo.lastLog.ActualCost)
	require.Equal(t, expectedCost.TotalCost, usageRepo.lastLog.TotalCost)
	require.Equal(t, expectedCost.ActualCost, userRepo.lastAmount)
REDACTED

func TestOpenAIGatewayServiceRecordUsage_ChannelMappedDoesNotOverrideBillingModelWhenUnmapped(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: trueREDACTED
	userRepo := &openAIRecordUsageUserRepoStub{REDACTED
	subRepo := &openAIRecordUsageSubRepoStub{REDACTED
	svc := newOpenAIRecordUsageServiceForTest(usageRepo, userRepo, subRepo, nil)
	usage := OpenAIUsage{InputTokens: 20, OutputTokens: 10REDACTED

	// 渠道未发生模型映射时，应使用 result.BillingModel 中记录的实际上游计费模型，
	// 而不是未映射的原始请求模型。
	expectedCost, err := svc.billingService.CalculateCost("gpt-5.1", UsageTokens{
		InputTokens:  20,
		OutputTokens: 10,
REDACTED, 1.1)
REDACTED

	err = svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID:     "resp_channel_unmapped_billing",
			Model:         "glm",
			BillingModel:  "gpt-5.1",
			UpstreamModel: "gpt-5.1",
			Usage:         usage,
			Duration:      time.Second,
	REDACTED,
		APIKey:  &APIKey{ID: 10REDACTED,
		User:    &User{ID: 20REDACTED,
		Account: &Account{ID: 30REDACTED,
		ChannelUsageFields: ChannelUsageFields{
			ChannelID:          1,
			OriginalModel:      "glm",
			ChannelMappedModel: "glm", // channel did NOT map
			BillingModelSource: BillingModelSourceChannelMapped,
	REDACTED,
REDACTED)

REDACTED
	require.NotNil(t, usageRepo.lastLog)
	require.Equal(t, expectedCost.ActualCost, usageRepo.lastLog.ActualCost)
	require.True(t, usageRepo.lastLog.ActualCost > 0, "cost must not be zero")
REDACTED

func TestOpenAIGatewayServiceRecordUsage_ChannelMappedOverridesBillingModelWhenMapped(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: trueREDACTED
	userRepo := &openAIRecordUsageUserRepoStub{REDACTED
	subRepo := &openAIRecordUsageSubRepoStub{REDACTED
	svc := newOpenAIRecordUsageServiceForTest(usageRepo, userRepo, subRepo, nil)
	usage := OpenAIUsage{InputTokens: 20, OutputTokens: 10REDACTED

	// When channel DID map the model (ChannelMappedModel != OriginalModel),
	// billing should use the channel-mapped model, honoring admin intent.
	expectedCost, err := svc.billingService.CalculateCost("gpt-5.1", UsageTokens{
		InputTokens:  20,
		OutputTokens: 10,
REDACTED, 1.1)
REDACTED

	err = svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID:     "resp_channel_mapped_billing",
			Model:         "glm",
			BillingModel:  "gpt-5.1-codex",
			UpstreamModel: "gpt-5.1-codex",
			Usage:         usage,
			Duration:      time.Second,
	REDACTED,
		APIKey:  &APIKey{ID: 10REDACTED,
		User:    &User{ID: 20REDACTED,
		Account: &Account{ID: 30REDACTED,
		ChannelUsageFields: ChannelUsageFields{
			ChannelID:          1,
			OriginalModel:      "glm",
			ChannelMappedModel: "gpt-5.1", // channel mapped glm → gpt-5.1
			BillingModelSource: BillingModelSourceChannelMapped,
	REDACTED,
REDACTED)

REDACTED
	require.NotNil(t, usageRepo.lastLog)
	require.Equal(t, expectedCost.ActualCost, usageRepo.lastLog.ActualCost)
	require.True(t, usageRepo.lastLog.ActualCost > 0, "cost must not be zero")
REDACTED

func TestOpenAIGatewayServiceRecordUsage_SubscriptionBillingSetsSubscriptionFields(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: trueREDACTED
	userRepo := &openAIRecordUsageUserRepoStub{REDACTED
	subRepo := &openAIRecordUsageSubRepoStub{REDACTED
	svc := newOpenAIRecordUsageServiceForTest(usageRepo, userRepo, subRepo, nil)
	subscription := &UserSubscription{ID: 99REDACTED

	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID: "resp_subscription_billing",
			Usage:     OpenAIUsage{InputTokens: 10, OutputTokens: 5REDACTED,
			Model:     "gpt-5.1",
			Duration:  time.Second,
	REDACTED,
		APIKey:       &APIKey{ID: 100, GroupID: i64p(88), Group: &Group{ID: 88, SubscriptionType: SubscriptionTypeSubscription, RateMultiplier: 1.0REDACTEDREDACTED,
		User:         &User{ID: 200REDACTED,
		Account:      &Account{ID: 300REDACTED,
		Subscription: subscription,
REDACTED)

REDACTED
	require.NotNil(t, usageRepo.lastLog)
	require.Equal(t, BillingTypeSubscription, usageRepo.lastLog.BillingType)
	require.NotNil(t, usageRepo.lastLog.SubscriptionID)
	require.Equal(t, subscription.ID, *usageRepo.lastLog.SubscriptionID)
	require.Equal(t, 1, subRepo.incrementCalls)
	require.Equal(t, 0, userRepo.deductCalls)
REDACTED

func TestOpenAIGatewayServiceRecordUsage_SimpleModeSkipsBillingAfterPersist(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: trueREDACTED
	userRepo := &openAIRecordUsageUserRepoStub{REDACTED
	subRepo := &openAIRecordUsageSubRepoStub{REDACTED
	svc := newOpenAIRecordUsageServiceForTest(usageRepo, userRepo, subRepo, nil)
	svc.cfg.RunMode = config.RunModeSimple

	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID: "resp_simple_mode",
			Usage:     OpenAIUsage{InputTokens: 10, OutputTokens: 5REDACTED,
			Model:     "gpt-5.1",
			Duration:  time.Second,
	REDACTED,
		APIKey:  &APIKey{ID: 1000REDACTED,
		User:    &User{ID: 2000REDACTED,
		Account: &Account{ID: 3000REDACTED,
REDACTED)

REDACTED
	require.Equal(t, 1, usageRepo.calls)
	require.Equal(t, 0, userRepo.deductCalls)
	require.Equal(t, 0, subRepo.incrementCalls)
REDACTED

func TestOpenAIGatewayServiceRecordUsage_ImageOnlyUsageStillPersists(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: trueREDACTED
	userRepo := &openAIRecordUsageUserRepoStub{REDACTED
	subRepo := &openAIRecordUsageSubRepoStub{REDACTED
	svc := newOpenAIRecordUsageServiceForTest(usageRepo, userRepo, subRepo, nil)

	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID:  "resp_image_only_usage",
			Model:      "gpt-image-2",
			ImageCount: 2,
			ImageSize:  "1K",
			Duration:   time.Second,
	REDACTED,
		APIKey:  &APIKey{ID: 1007REDACTED,
		User:    &User{ID: 2007REDACTED,
		Account: &Account{ID: 3007REDACTED,
REDACTED)

REDACTED
	require.NotNil(t, usageRepo.lastLog)
	require.Equal(t, 2, usageRepo.lastLog.ImageCount)
	require.NotNil(t, usageRepo.lastLog.ImageSize)
	require.Equal(t, "1K", *usageRepo.lastLog.ImageSize)
	require.NotNil(t, usageRepo.lastLog.BillingMode)
	require.Equal(t, string(BillingModeImage), *usageRepo.lastLog.BillingMode)
REDACTED

func TestOpenAIGatewayServiceRecordUsage_ImageUsesPerImageBillingEvenWithUsageTokens(t *testing.T) {
	imagePrice := 0.02
	groupID := int64(12)

	usageRepo := &openAIRecordUsageLogRepoStub{inserted: trueREDACTED
	userRepo := &openAIRecordUsageUserRepoStub{REDACTED
	subRepo := &openAIRecordUsageSubRepoStub{REDACTED
	svc := newOpenAIRecordUsageServiceForTest(usageRepo, userRepo, subRepo, nil)

	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID: "resp_image_per_request",
			Model:     "gpt-image-2",
			Usage: OpenAIUsage{
				InputTokens:       1110,
				OutputTokens:      1756,
				ImageOutputTokens: 1756,
		REDACTED,
			ImageCount: 2,
			ImageSize:  "1K",
			Duration:   time.Second,
	REDACTED,
		APIKey: &APIKey{
			ID:      1008,
			GroupID: i64p(groupID),
			Group: &Group{
				ID:             groupID,
				RateMultiplier: 1.0,
				ImagePrice1K:   &imagePrice,
		REDACTED,
	REDACTED,
		User:    &User{ID: 2008REDACTED,
		Account: &Account{ID: 3008REDACTED,
REDACTED)

REDACTED
	require.NotNil(t, usageRepo.lastLog)
	require.NotNil(t, usageRepo.lastLog.BillingMode)
	require.Equal(t, string(BillingModeImage), *usageRepo.lastLog.BillingMode)
	require.Equal(t, 2, usageRepo.lastLog.ImageCount)
	require.InDelta(t, 0.04, usageRepo.lastLog.TotalCost, 1e-12)
	require.InDelta(t, 0.04, usageRepo.lastLog.ActualCost, 1e-12)
	require.InDelta(t, 0.0, usageRepo.lastLog.InputCost, 1e-12)
	require.InDelta(t, 0.0, usageRepo.lastLog.OutputCost, 1e-12)
	require.InDelta(t, 0.0, usageRepo.lastLog.ImageOutputCost, 1e-12)
REDACTED
