package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type openAIRecordUsageLogRepoStub struct {
	UsageLogRepository

	inserted bool
	err      error
	calls    int
	lastLog  *UsageLog
REDACTED

func (s *openAIRecordUsageLogRepoStub) Create(ctx context.Context, log *UsageLog) (bool, error) {
	s.calls++
	s.lastLog = log
	return s.inserted, s.err
REDACTED

type openAIRecordUsageUserRepoStub struct {
	UserRepository

	deductCalls int
	deductErr   error
	lastAmount  float64
REDACTED

func (s *openAIRecordUsageUserRepoStub) DeductBalance(ctx context.Context, id int64, amount float64) error {
	s.deductCalls++
	s.lastAmount = amount
	return s.deductErr
REDACTED

type openAIRecordUsageSubRepoStub struct {
	UserSubscriptionRepository

	incrementCalls int
	incrementErr   error
	lastAmount     float64
REDACTED

func (s *openAIRecordUsageSubRepoStub) IncrementUsage(ctx context.Context, id int64, costUSD float64) error {
	s.incrementCalls++
	s.lastAmount = costUSD
	return s.incrementErr
REDACTED

type openAIRecordUsageAPIKeyQuotaStub struct {
	quotaCalls     int
	rateLimitCalls int
	err            error
	lastAmount     float64
REDACTED

func (s *openAIRecordUsageAPIKeyQuotaStub) UpdateQuotaUsed(ctx context.Context, apiKeyID int64, cost float64) error {
	s.quotaCalls++
	s.lastAmount = cost
	return s.err
REDACTED

func (s *openAIRecordUsageAPIKeyQuotaStub) UpdateRateLimitUsage(ctx context.Context, apiKeyID int64, cost float64) error {
	s.rateLimitCalls++
	s.lastAmount = cost
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

	return &OpenAIGatewayService{
		usageLogRepo:        usageRepo,
		userRepo:            userRepo,
		userSubRepo:         subRepo,
		cfg:                 cfg,
		billingService:      NewBillingService(cfg, nil),
		billingCacheService: &BillingCacheService{REDACTED,
		deferredService:     &DeferredService{REDACTED,
		userGroupRateResolver: newUserGroupRateResolver(
			rateRepo,
			nil,
			resolveUserGroupRateCacheTTL(cfg),
			nil,
			"service.openai_gateway.test",
		),
REDACTED
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
	userRepo := &openAIRecordUsageUserRepoStub{REDACTED
	subRepo := &openAIRecordUsageSubRepoStub{REDACTED
	svc := newOpenAIRecordUsageServiceForTest(usageRepo, userRepo, subRepo, nil)

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
	require.Equal(t, 1, usageRepo.calls)
	require.Equal(t, 0, userRepo.deductCalls)
	require.Equal(t, 0, subRepo.incrementCalls)
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
