//go:build unit

package service

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strconv"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

type settingUpdateRepoStub struct {
	updates        map[string]string
	setMultipleErr error
REDACTED

func (s *settingUpdateRepoStub) Get(ctx context.Context, key string) (*Setting, error) {
	panic("unexpected Get call")
REDACTED

func (s *settingUpdateRepoStub) GetValue(ctx context.Context, key string) (string, error) {
	panic("unexpected GetValue call")
REDACTED

func (s *settingUpdateRepoStub) Set(ctx context.Context, key, value string) error {
	panic("unexpected Set call")
REDACTED

func (s *settingUpdateRepoStub) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	panic("unexpected GetMultiple call")
REDACTED

func (s *settingUpdateRepoStub) SetMultiple(ctx context.Context, settings map[string]string) error {
	s.updates = make(map[string]string, len(settings))
	for k, v := range settings {
		s.updates[k] = v
REDACTED
	return s.setMultipleErr
REDACTED

func (s *settingUpdateRepoStub) GetAll(ctx context.Context) (map[string]string, error) {
	panic("unexpected GetAll call")
REDACTED

func (s *settingUpdateRepoStub) Delete(ctx context.Context, key string) error {
	panic("unexpected Delete call")
REDACTED

type settingGetAllRepoStub struct {
	values map[string]string
REDACTED

func (s *settingGetAllRepoStub) Get(ctx context.Context, key string) (*Setting, error) {
	panic("unexpected Get call")
REDACTED

func (s *settingGetAllRepoStub) GetValue(ctx context.Context, key string) (string, error) {
	panic("unexpected GetValue call")
REDACTED

func (s *settingGetAllRepoStub) Set(ctx context.Context, key, value string) error {
	panic("unexpected Set call")
REDACTED

func (s *settingGetAllRepoStub) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	panic("unexpected GetMultiple call")
REDACTED

func (s *settingGetAllRepoStub) SetMultiple(ctx context.Context, settings map[string]string) error {
	panic("unexpected SetMultiple call")
REDACTED

func (s *settingGetAllRepoStub) GetAll(ctx context.Context) (map[string]string, error) {
	out := make(map[string]string, len(s.values))
	for key, value := range s.values {
		out[key] = value
REDACTED
	return out, nil
REDACTED

func (s *settingGetAllRepoStub) Delete(ctx context.Context, key string) error {
	panic("unexpected Delete call")
REDACTED

type forwardedIPMigrationRepoStub struct {
	values         map[string]string
	updates        map[string]string
	getMultipleErr error
	setMultipleErr error
REDACTED

func (s *forwardedIPMigrationRepoStub) Get(context.Context, string) (*Setting, error) {
	panic("unexpected Get call")
REDACTED

func (s *forwardedIPMigrationRepoStub) GetValue(_ context.Context, key string) (string, error) {
	value, ok := s.values[key]
	if !ok {
		return "", ErrSettingNotFound
REDACTED
	return value, nil
REDACTED

func (s *forwardedIPMigrationRepoStub) Set(context.Context, string, string) error {
	panic("unexpected Set call")
REDACTED

func (s *forwardedIPMigrationRepoStub) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	if s.getMultipleErr != nil {
		return nil, s.getMultipleErr
REDACTED
	result := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := s.values[key]; ok {
			result[key] = value
	REDACTED
REDACTED
	return result, nil
REDACTED

func (s *forwardedIPMigrationRepoStub) SetMultiple(_ context.Context, values map[string]string) error {
	if s.setMultipleErr != nil {
		return s.setMultipleErr
REDACTED
	s.updates = make(map[string]string, len(values))
	for key, value := range values {
		s.values[key] = value
		s.updates[key] = value
REDACTED
	return nil
REDACTED

func (s *forwardedIPMigrationRepoStub) GetAll(context.Context) (map[string]string, error) {
	panic("unexpected GetAll call")
REDACTED

func (s *forwardedIPMigrationRepoStub) Delete(context.Context, string) error {
	panic("unexpected Delete call")
REDACTED

type settingAntigravityUARepoStub struct {
	values map[string]string
REDACTED

func (s *settingAntigravityUARepoStub) Get(ctx context.Context, key string) (*Setting, error) {
	panic("unexpected Get call")
REDACTED

func (s *settingAntigravityUARepoStub) GetValue(ctx context.Context, key string) (string, error) {
	if value, ok := s.values[key]; ok {
		return value, nil
REDACTED
	return "", ErrSettingNotFound
REDACTED

func (s *settingAntigravityUARepoStub) Set(ctx context.Context, key, value string) error {
	panic("unexpected Set call")
REDACTED

func (s *settingAntigravityUARepoStub) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	panic("unexpected GetMultiple call")
REDACTED

func (s *settingAntigravityUARepoStub) SetMultiple(ctx context.Context, settings map[string]string) error {
	panic("unexpected SetMultiple call")
REDACTED

func (s *settingAntigravityUARepoStub) GetAll(ctx context.Context) (map[string]string, error) {
	panic("unexpected GetAll call")
REDACTED

func (s *settingAntigravityUARepoStub) Delete(ctx context.Context, key string) error {
	panic("unexpected Delete call")
REDACTED

type defaultSubGroupReaderStub struct {
	byID  map[int64]*Group
	errBy map[int64]error
	calls []int64
REDACTED

func TestSettingService_AffiliateAdminRechargeSetting(t *testing.T) {
	t.Run("missing value defaults to disabled", func(t *testing.T) {
		svc := NewSettingService(&settingGetAllRepoStub{values: map[string]string{REDACTEDREDACTED, &config.Config{REDACTED)

		settings, err := svc.GetAllSettings(context.Background())
	REDACTED
		require.False(t, settings.AdminRechargeRebateEnabled)
REDACTED)

	t.Run("explicit value is parsed", func(t *testing.T) {
		svc := NewSettingService(&settingGetAllRepoStub{values: map[string]string{
			SettingKeyAffiliateAdminRechargeEnabled: "true",
REDACTED &config.Config{REDACTED)

		settings, err := svc.GetAllSettings(context.Background())
	REDACTED
		require.True(t, settings.AdminRechargeRebateEnabled)
REDACTED)

	t.Run("value is persisted", func(t *testing.T) {
		repo := &settingUpdateRepoStub{REDACTED
		svc := NewSettingService(repo, &config.Config{REDACTED)

		err := svc.UpdateSettings(context.Background(), &SystemSettings{
			AdminRechargeRebateEnabled: true,
	REDACTED)
	REDACTED
		require.Equal(t, "true", repo.updates[SettingKeyAffiliateAdminRechargeEnabled])
REDACTED)
REDACTED

func (s *defaultSubGroupReaderStub) GetByID(ctx context.Context, id int64) (*Group, error) {
	s.calls = append(s.calls, id)
	if err, ok := s.errBy[id]; ok {
		return nil, err
REDACTED
	if g, ok := s.byID[id]; ok {
		return g, nil
REDACTED
	return nil, ErrGroupNotFound
REDACTED

func TestSettingService_UpdateSettings_DefaultSubscriptions_ValidGroup(t *testing.T) {
	repo := &settingUpdateRepoStub{REDACTED
	groupReader := &defaultSubGroupReaderStub{
		byID: map[int64]*Group{
			11: {ID: 11, SubscriptionType: SubscriptionTypeSubscriptionREDACTED,
	REDACTED,
REDACTED
	svc := NewSettingService(repo, &config.Config{REDACTED)
	svc.SetDefaultSubscriptionGroupReader(groupReader)

	err := svc.UpdateSettings(context.Background(), &SystemSettings{
		DefaultSubscriptions: []DefaultSubscriptionSetting{
			{GroupID: 11, ValidityDays: 30REDACTED,
	REDACTED,
REDACTED)
REDACTED
	require.Equal(t, []int64{11REDACTED, groupReader.calls)

	raw, ok := repo.updates[SettingKeyDefaultSubscriptions]
	require.True(t, ok)

	var got []DefaultSubscriptionSetting
	require.NoError(t, json.Unmarshal([]byte(raw), &got))
	require.Equal(t, []DefaultSubscriptionSetting{
		{GroupID: 11, ValidityDays: 30REDACTED,
REDACTED, got)
REDACTED

func TestSettingService_UpdateSettings_DefaultSubscriptions_RejectsNonSubscriptionGroup(t *testing.T) {
	repo := &settingUpdateRepoStub{REDACTED
	groupReader := &defaultSubGroupReaderStub{
		byID: map[int64]*Group{
			12: {ID: 12, SubscriptionType: SubscriptionTypeStandardREDACTED,
	REDACTED,
REDACTED
	svc := NewSettingService(repo, &config.Config{REDACTED)
	svc.SetDefaultSubscriptionGroupReader(groupReader)

	err := svc.UpdateSettings(context.Background(), &SystemSettings{
		DefaultSubscriptions: []DefaultSubscriptionSetting{
			{GroupID: 12, ValidityDays: 7REDACTED,
	REDACTED,
REDACTED)
REDACTED
	require.Equal(t, "DEFAULT_SUBSCRIPTION_GROUP_INVALID", infraerrors.Reason(err))
	require.Nil(t, repo.updates)
REDACTED

func TestSettingService_UpdateSettings_DefaultSubscriptions_RejectsNotFoundGroup(t *testing.T) {
	repo := &settingUpdateRepoStub{REDACTED
	groupReader := &defaultSubGroupReaderStub{
		errBy: map[int64]error{
			13: ErrGroupNotFound,
	REDACTED,
REDACTED
	svc := NewSettingService(repo, &config.Config{REDACTED)
	svc.SetDefaultSubscriptionGroupReader(groupReader)

	err := svc.UpdateSettings(context.Background(), &SystemSettings{
		DefaultSubscriptions: []DefaultSubscriptionSetting{
			{GroupID: 13, ValidityDays: 7REDACTED,
	REDACTED,
REDACTED)
REDACTED
	require.Equal(t, "DEFAULT_SUBSCRIPTION_GROUP_INVALID", infraerrors.Reason(err))
	require.Equal(t, "13", infraerrors.FromError(err).Metadata["group_id"])
	require.Nil(t, repo.updates)
REDACTED

func TestSettingService_UpdateSettings_DefaultSubscriptions_RejectsDuplicateGroup(t *testing.T) {
	repo := &settingUpdateRepoStub{REDACTED
	groupReader := &defaultSubGroupReaderStub{
		byID: map[int64]*Group{
			11: {ID: 11, SubscriptionType: SubscriptionTypeSubscriptionREDACTED,
	REDACTED,
REDACTED
	svc := NewSettingService(repo, &config.Config{REDACTED)
	svc.SetDefaultSubscriptionGroupReader(groupReader)

	err := svc.UpdateSettings(context.Background(), &SystemSettings{
		DefaultSubscriptions: []DefaultSubscriptionSetting{
			{GroupID: 11, ValidityDays: 30REDACTED,
			{GroupID: 11, ValidityDays: 60REDACTED,
	REDACTED,
REDACTED)
REDACTED
	require.Equal(t, "DEFAULT_SUBSCRIPTION_GROUP_DUPLICATE", infraerrors.Reason(err))
	require.Equal(t, "11", infraerrors.FromError(err).Metadata["group_id"])
	require.Nil(t, repo.updates)
REDACTED

func TestSettingService_UpdateSettings_DefaultSubscriptions_RejectsDuplicateGroupWithoutGroupReader(t *testing.T) {
	repo := &settingUpdateRepoStub{REDACTED
	svc := NewSettingService(repo, &config.Config{REDACTED)

	err := svc.UpdateSettings(context.Background(), &SystemSettings{
		DefaultSubscriptions: []DefaultSubscriptionSetting{
			{GroupID: 11, ValidityDays: 30REDACTED,
			{GroupID: 11, ValidityDays: 60REDACTED,
	REDACTED,
REDACTED)
REDACTED
	require.Equal(t, "DEFAULT_SUBSCRIPTION_GROUP_DUPLICATE", infraerrors.Reason(err))
	require.Equal(t, "11", infraerrors.FromError(err).Metadata["group_id"])
	require.Nil(t, repo.updates)
REDACTED

func TestSettingService_UpdateSettings_RegistrationEmailSuffixWhitelist_Normalized(t *testing.T) {
	repo := &settingUpdateRepoStub{REDACTED
	svc := NewSettingService(repo, &config.Config{REDACTED)

	err := svc.UpdateSettings(context.Background(), &SystemSettings{
		RegistrationEmailSuffixWhitelist: []string{"example.com", "@EXAMPLE.com", " @foo.bar ", "*.EDU.CN"REDACTED,
REDACTED)
REDACTED
	require.Equal(t, `["@example.com","@foo.bar","*.edu.cn"]`, repo.updates[SettingKeyRegistrationEmailSuffixWhitelist])
REDACTED

func TestSettingService_UpdateSettings_RegistrationEmailSuffixWhitelist_Invalid(t *testing.T) {
	repo := &settingUpdateRepoStub{REDACTED
	svc := NewSettingService(repo, &config.Config{REDACTED)

	err := svc.UpdateSettings(context.Background(), &SystemSettings{
		RegistrationEmailSuffixWhitelist: []string{"@invalid_domain"REDACTED,
REDACTED)
REDACTED
	require.Equal(t, "INVALID_REGISTRATION_EMAIL_SUFFIX_WHITELIST", infraerrors.Reason(err))
REDACTED

func TestParseDefaultSubscriptions_NormalizesValues(t *testing.T) {
	got := parseDefaultSubscriptions(`[{"group_id":11,"validity_days":30REDACTED,{"group_id":11,"validity_days":60REDACTED,{"group_id":0,"validity_days":10REDACTED,{"group_id":12,"validity_days":99999REDACTED]`)
	require.Equal(t, []DefaultSubscriptionSetting{
		{GroupID: 11, ValidityDays: 30REDACTED,
		{GroupID: 11, ValidityDays: 60REDACTED,
		{GroupID: 12, ValidityDays: MaxValidityDaysREDACTED,
REDACTED, got)
REDACTED

func TestSettingService_UpdateSettings_TablePreferences(t *testing.T) {
	repo := &settingUpdateRepoStub{REDACTED
	svc := NewSettingService(repo, &config.Config{REDACTED)

	err := svc.UpdateSettings(context.Background(), &SystemSettings{
		TableDefaultPageSize: 50,
		TablePageSizeOptions: []int{20, 50, 100REDACTED,
REDACTED)
REDACTED
	require.Equal(t, "50", repo.updates[SettingKeyTableDefaultPageSize])
	require.Equal(t, "[20,50,100]", repo.updates[SettingKeyTablePageSizeOptions])

	err = svc.UpdateSettings(context.Background(), &SystemSettings{
		TableDefaultPageSize: 1000,
		TablePageSizeOptions: []int{20, 100REDACTED,
REDACTED)
REDACTED
	require.Equal(t, "1000", repo.updates[SettingKeyTableDefaultPageSize])
	require.Equal(t, "[20,100]", repo.updates[SettingKeyTablePageSizeOptions])
REDACTED

func TestSettingService_UpdateSettings_PaymentVisibleMethodsAndAdvancedScheduler(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()
	defer resetOpenAIAdvancedSchedulerSettingCacheForTest()

	repo := &settingUpdateRepoStub{REDACTED
	svc := NewSettingService(repo, &config.Config{REDACTED)

	err := svc.UpdateSettings(context.Background(), &SystemSettings{
		PaymentVisibleMethodAlipaySource:                   "alipay",
		PaymentVisibleMethodWxpaySource:                    "easypay",
		PaymentVisibleMethodAlipayEnabled:                  true,
		PaymentVisibleMethodWxpayEnabled:                   false,
		OpenAILowUpstreamRatePriorityEnabled:               true,
		OpenAIOAuthSchedulingRateMultiplier:                0.05,
		OpenAIAdvancedSchedulerEnabled:                     true,
		OpenAIAdvancedSchedulerStickyWeightedEnabled:       true,
		OpenAIAdvancedSchedulerSubscriptionPriorityEnabled: true,
		OpenAIAdvancedSchedulerLBTopK:                      " 3 ",
		OpenAIAdvancedSchedulerWeightPriority:              "2.50",
		OpenAIAdvancedSchedulerWeightLoad:                  "0",
		OpenAIAdvancedSchedulerWeightQueue:                 "0.75",
		OpenAIAdvancedSchedulerWeightErrorRate:             "1.25",
		OpenAIAdvancedSchedulerWeightTTFT:                  "0.5",
		OpenAIAdvancedSchedulerWeightReset:                 "",
		OpenAIAdvancedSchedulerWeightQuotaHeadroom:         "0.2",
		OpenAIAdvancedSchedulerWeightUpstreamCost:          "1.5",
		OpenAIAdvancedSchedulerWeightPreviousResponse:      "8",
		OpenAIAdvancedSchedulerWeightSessionSticky:         "4",
REDACTED)
REDACTED
	require.Equal(t, VisibleMethodSourceOfficialAlipay, repo.updates[SettingPaymentVisibleMethodAlipaySource])
	require.Equal(t, VisibleMethodSourceEasyPayWechat, repo.updates[SettingPaymentVisibleMethodWxpaySource])
	require.Equal(t, "true", repo.updates[SettingPaymentVisibleMethodAlipayEnabled])
	require.Equal(t, "false", repo.updates[SettingPaymentVisibleMethodWxpayEnabled])
	require.Equal(t, "true", repo.updates[SettingKeyOpenAILowUpstreamRatePriorityEnabled])
	require.Equal(t, "0.05", repo.updates[SettingKeyOpenAIOAuthSchedulingRateMultiplier])
	require.Equal(t, "true", repo.updates[openAIAdvancedSchedulerSettingKey])
	require.Equal(t, "true", repo.updates[SettingKeyOpenAIAdvancedSchedulerStickyWeightedEnabled])
	require.Equal(t, "true", repo.updates[SettingKeyOpenAIAdvancedSchedulerSubscriptionPriorityEnabled])
	require.Equal(t, "3", repo.updates[SettingKeyOpenAIAdvancedSchedulerLBTopK])
	require.Equal(t, "2.5", repo.updates[SettingKeyOpenAIAdvancedSchedulerWeightPriority])
	require.Equal(t, "0", repo.updates[SettingKeyOpenAIAdvancedSchedulerWeightLoad])
	require.Equal(t, "0.75", repo.updates[SettingKeyOpenAIAdvancedSchedulerWeightQueue])
	require.Equal(t, "1.25", repo.updates[SettingKeyOpenAIAdvancedSchedulerWeightErrorRate])
	require.Equal(t, "0.5", repo.updates[SettingKeyOpenAIAdvancedSchedulerWeightTTFT])
	require.Equal(t, "", repo.updates[SettingKeyOpenAIAdvancedSchedulerWeightReset])
	require.Equal(t, "0.2", repo.updates[SettingKeyOpenAIAdvancedSchedulerWeightQuotaHeadroom])
	require.Equal(t, "1.5", repo.updates[SettingKeyOpenAIAdvancedSchedulerWeightUpstreamCost])
	require.Equal(t, "8", repo.updates[SettingKeyOpenAIAdvancedSchedulerWeightPreviousResponse])
	require.Equal(t, "4", repo.updates[SettingKeyOpenAIAdvancedSchedulerWeightSessionSticky])
REDACTED

func TestSettingService_UpdateSettingsRejectsInvalidOpenAIOAuthSchedulingRateMultiplier(t *testing.T) {
	repo := &settingUpdateRepoStub{REDACTED
	svc := NewSettingService(repo, &config.Config{REDACTED)

	for _, rate := range []float64{-0.01, math.NaN(), math.Inf(1)REDACTED {
		err := svc.UpdateSettings(context.Background(), &SystemSettings{OpenAIOAuthSchedulingRateMultiplier: rateREDACTED)
	REDACTED
REDACTED
REDACTED

func TestSettingService_UpdateSettings_OpenAIAdvancedSchedulerWeightSums(t *testing.T) {
	maxFloat := strconv.FormatFloat(math.MaxFloat64, 'g', -1, 64)
	tests := []struct {
		name    string
		weights SystemSettings
		wantErr bool
REDACTED{
		{
			name: "reset only base is valid",
			weights: SystemSettings{
				OpenAIAdvancedSchedulerWeightPriority:         "0",
				OpenAIAdvancedSchedulerWeightLoad:             "0",
				OpenAIAdvancedSchedulerWeightQueue:            "0",
				OpenAIAdvancedSchedulerWeightErrorRate:        "0",
				OpenAIAdvancedSchedulerWeightTTFT:             "0",
				OpenAIAdvancedSchedulerWeightReset:            "1",
				OpenAIAdvancedSchedulerWeightQuotaHeadroom:    "0",
				OpenAIAdvancedSchedulerWeightUpstreamCost:     "0",
				OpenAIAdvancedSchedulerWeightPreviousResponse: "0",
				OpenAIAdvancedSchedulerWeightSessionSticky:    "0",
		REDACTED,
	REDACTED,
		{
			name: "base sum overflow is rejected",
			weights: SystemSettings{
				OpenAIAdvancedSchedulerWeightPriority: maxFloat,
				OpenAIAdvancedSchedulerWeightLoad:     maxFloat,
		REDACTED,
			wantErr: true,
	REDACTED,
		{
			name: "sticky total sum overflow is rejected",
			weights: SystemSettings{
				OpenAIAdvancedSchedulerWeightPriority:         maxFloat,
				OpenAIAdvancedSchedulerWeightPreviousResponse: maxFloat,
		REDACTED,
			wantErr: true,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := NewSettingService(&settingUpdateRepoStub{REDACTED, &config.Config{REDACTED)
			err := svc.UpdateSettings(context.Background(), &tt.weights)
			if tt.wantErr {
			REDACTED
				return
		REDACTED
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestSettingService_ParseSettingsDefaultsOpenAIOAuthSchedulingRateMultiplier(t *testing.T) {
	svc := NewSettingService(&settingUpdateRepoStub{REDACTED, &config.Config{REDACTED)

	require.Equal(t, 1.0, svc.parseSettings(map[string]string{REDACTED).OpenAIOAuthSchedulingRateMultiplier)
	require.Equal(t, 0.05, svc.parseSettings(map[string]string{SettingKeyOpenAIOAuthSchedulingRateMultiplier: "0.05"REDACTED).OpenAIOAuthSchedulingRateMultiplier)
REDACTED

func TestSettingService_GetAllSettings_OpenAIAdvancedSchedulerEffectiveValuesUseConfig(t *testing.T) {
	cfg := &config.Config{REDACTED
	cfg.Gateway.OpenAIWS.LBTopK = 13
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights = config.GatewayOpenAIWSSchedulerScoreWeights{
		Priority:         2,
		Load:             3,
		Queue:            4,
		ErrorRate:        5,
		TTFT:             6,
		Reset:            7,
		QuotaHeadroom:    8,
		UpstreamCost:     9,
		PreviousResponse: 10,
		SessionSticky:    11,
REDACTED
	svc := NewSettingService(&settingGetAllRepoStub{values: map[string]string{
		SettingKeyOpenAIAdvancedSchedulerLBTopK:              "3",
		SettingKeyOpenAIAdvancedSchedulerWeightPriority:      "99",
		SettingKeyOpenAIAdvancedSchedulerWeightSessionSticky: "88",
REDACTEDREDACTED, cfg)

	settings, err := svc.GetAllSettings(context.Background())
REDACTED
	require.Equal(t, "3", settings.OpenAIAdvancedSchedulerLBTopK)
	require.Equal(t, "99", settings.OpenAIAdvancedSchedulerWeightPriority)
	require.Equal(t, "88", settings.OpenAIAdvancedSchedulerWeightSessionSticky)
	require.Equal(t, "13", settings.OpenAIAdvancedSchedulerEffectiveLBTopK)
	require.Equal(t, "2", settings.OpenAIAdvancedSchedulerEffectiveWeightPriority)
	require.Equal(t, "3", settings.OpenAIAdvancedSchedulerEffectiveWeightLoad)
	require.Equal(t, "9", settings.OpenAIAdvancedSchedulerEffectiveWeightUpstreamCost)
	require.Equal(t, "11", settings.OpenAIAdvancedSchedulerEffectiveWeightSessionSticky)
REDACTED

func TestSettingService_UpdateSettings_AntigravityUserAgentVersion(t *testing.T) {
	repo := &settingUpdateRepoStub{REDACTED
	svc := NewSettingService(repo, &config.Config{REDACTED)

	err := svc.UpdateSettings(context.Background(), &SystemSettings{
		AntigravityUserAgentVersion: "1.23.2",
REDACTED)
REDACTED
	require.Equal(t, "1.23.2", repo.updates[SettingKeyAntigravityUserAgentVersion])
REDACTED

func TestSettingService_InitializeDefaultSettingsPersistsConfiguredForwardedClientIPHeaders(t *testing.T) {
	repo := &forwardedIPMigrationRepoStub{values: map[string]string{REDACTEDREDACTED
	cfg := &config.Config{REDACTED
	cfg.SetForwardedClientIPSettings(true, []string{"X-Cdn-Ip", "True-Client-Ip"REDACTED)
	svc := NewSettingService(repo, cfg)

	require.NoError(t, svc.InitializeDefaultSettings(context.Background()))
	require.JSONEq(t, `["X-Cdn-Ip","True-Client-Ip"]`, repo.values[SettingKeyForwardedClientIPHeaders])
REDACTED

func TestSettingService_UpdateSettings_APIKeyACLTrustForwardedIPRefreshesConfig(t *testing.T) {
	repo := &settingUpdateRepoStub{REDACTED
	cfg := &config.Config{REDACTED
	svc := NewSettingService(repo, cfg)

	err := svc.UpdateSettings(context.Background(), &SystemSettings{
		APIKeyACLTrustForwardedIP: true,
		ForwardedClientIPHeaders:  []string{" x-cdn-ip ", "X-CDN-IP", "true-client-ip"REDACTED,
REDACTED)
REDACTED
	require.Equal(t, "true", repo.updates[SettingKeyAPIKeyACLTrustForwardedIP])
	require.JSONEq(t, `["X-Cdn-Ip","True-Client-Ip"]`, repo.updates[SettingKeyForwardedClientIPHeaders])
	runtimeSettings := cfg.ForwardedClientIPSettings()
	require.True(t, runtimeSettings.TrustForwardedIP)
	require.Equal(t, []string{"X-Cdn-Ip", "True-Client-Ip"REDACTED, runtimeSettings.Headers)

	runtimeSettings.Headers[0] = "X-Mutated"
	require.Equal(t, []string{"X-Cdn-Ip", "True-Client-Ip"REDACTED, cfg.ForwardedClientIPSettings().Headers)
REDACTED

func TestSettingService_UpdateSettings_RejectsInvalidForwardedClientIPHeadersWithoutRefreshing(t *testing.T) {
	repo := &settingUpdateRepoStub{REDACTED
	cfg := &config.Config{REDACTED
	cfg.SetForwardedClientIPSettings(true, []string{"X-Existing-IP"REDACTED)
	svc := NewSettingService(repo, cfg)

	err := svc.UpdateSettings(context.Background(), &SystemSettings{
		ForwardedClientIPHeaders: []string{"X Invalid"REDACTED,
REDACTED)

REDACTED
	require.Nil(t, repo.updates)
	runtimeSettings := cfg.ForwardedClientIPSettings()
	require.True(t, runtimeSettings.TrustForwardedIP)
	require.Equal(t, []string{"X-Existing-IP"REDACTED, runtimeSettings.Headers)
REDACTED

func TestSettingService_UpdateSettings_WriteFailureDoesNotRefreshForwardedIPRuntime(t *testing.T) {
	repo := &settingUpdateRepoStub{setMultipleErr: errors.New("database unavailable")REDACTED
	cfg := &config.Config{REDACTED
	cfg.SetForwardedClientIPSettings(false, []string{"X-Existing-IP"REDACTED)
	svc := NewSettingService(repo, cfg)

	err := svc.UpdateSettings(context.Background(), &SystemSettings{
		APIKeyACLTrustForwardedIP: true,
		ForwardedClientIPHeaders:  []string{"X-New-IP"REDACTED,
REDACTED)

	require.ErrorContains(t, err, "database unavailable")
	runtimeSettings := cfg.ForwardedClientIPSettings()
	require.False(t, runtimeSettings.TrustForwardedIP)
	require.Equal(t, []string{"X-Existing-IP"REDACTED, runtimeSettings.Headers)
REDACTED

func TestSettingService_ParseSettings_APIKeyACLTrustForwardedIPFallsBackToConfigWhenMissing(t *testing.T) {
	cfg := &config.Config{REDACTED
	cfg.Security.TrustForwardedIPForAPIKeyACL = true
	svc := NewSettingService(&settingUpdateRepoStub{REDACTED, cfg)

	got := svc.parseSettings(map[string]string{REDACTED)

	require.True(t, got.APIKeyACLTrustForwardedIP)
REDACTED

func TestSettingService_ParseSettings_APIKeyACLTrustForwardedIPUsesStoredValue(t *testing.T) {
	cfg := &config.Config{REDACTED
	cfg.SetTrustForwardedIPForAPIKeyACL(true)
	svc := NewSettingService(&settingUpdateRepoStub{REDACTED, cfg)

	got := svc.parseSettings(map[string]string{SettingKeyAPIKeyACLTrustForwardedIP: "false"REDACTED)

	require.False(t, got.APIKeyACLTrustForwardedIP)
REDACTED

func TestSettingService_ParseSettings_ForwardedClientIPHeaders(t *testing.T) {
	cfg := &config.Config{REDACTED
	cfg.SetForwardedClientIPSettings(true, []string{"X-Config-IP"REDACTED)
	svc := NewSettingService(&settingUpdateRepoStub{REDACTED, cfg)

	t.Run("stored value is normalized", func(t *testing.T) {
		got := svc.parseSettings(map[string]string{
			SettingKeyForwardedClientIPHeaders: `[" x-cdn-ip ","X-CDN-IP","true-client-ip"]`,
	REDACTED)
		require.Equal(t, []string{"X-Cdn-Ip", "True-Client-Ip"REDACTED, got.ForwardedClientIPHeaders)
REDACTED)

	t.Run("missing value falls back to config", func(t *testing.T) {
		got := svc.parseSettings(map[string]string{REDACTED)
		require.Equal(t, []string{"X-Config-IP"REDACTED, got.ForwardedClientIPHeaders)
REDACTED)

	t.Run("malformed value disables forwarded trust", func(t *testing.T) {
		got := svc.parseSettings(map[string]string{
			SettingKeyAPIKeyACLTrustForwardedIP: "true",
			SettingKeyForwardedClientIPHeaders:  `{"not":"an array"REDACTED`,
	REDACTED)
		require.False(t, got.APIKeyACLTrustForwardedIP)
		require.Empty(t, got.ForwardedClientIPHeaders)
REDACTED)
REDACTED

func TestSettingService_LoadForwardedClientIPSettingsMigration(t *testing.T) {
	tests := []struct {
		name                   string
		values                 map[string]string
		trustedProxiesSet      bool
		configDefault          bool
		wantEnabled            bool
		wantForwardedIPUpdate  string
		wantMigrationMarkerSet bool
REDACTED{
		{
			name:                   "missing setting follows configured default",
			values:                 map[string]string{REDACTED,
			configDefault:          true,
			wantEnabled:            true,
			wantMigrationMarkerSet: true,
	REDACTED,
		{
			name:                   "legacy false without proxy config migrates to compatibility",
			values:                 map[string]string{SettingKeyAPIKeyACLTrustForwardedIP: "false"REDACTED,
			wantEnabled:            true,
			wantForwardedIPUpdate:  "true",
			wantMigrationMarkerSet: true,
	REDACTED,
		{
			name:                   "legacy false with explicit proxy config stays secure",
			values:                 map[string]string{SettingKeyAPIKeyACLTrustForwardedIP: "false"REDACTED,
			trustedProxiesSet:      true,
			wantEnabled:            false,
			wantMigrationMarkerSet: true,
	REDACTED,
		{
			name: "completed migration preserves later false choice",
			values: map[string]string{
				SettingKeyAPIKeyACLTrustForwardedIP: "false",
				settingKeyForwardedClientIPModeV2:   "true",
		REDACTED,
			wantEnabled: false,
	REDACTED,
REDACTED

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := &forwardedIPMigrationRepoStub{values: test.valuesREDACTED
			cfg := &config.Config{Server: config.ServerConfig{TrustedProxiesConfigured: test.trustedProxiesSetREDACTEDREDACTED
			cfg.Security.TrustForwardedIPForAPIKeyACL = test.configDefault
			svc := NewSettingService(repo, cfg)

			require.NoError(t, svc.LoadForwardedClientIPSettings(context.Background()))
			require.Equal(t, test.wantEnabled, cfg.TrustForwardedIPForAPIKeyACL())
			require.Equal(t, test.wantForwardedIPUpdate, repo.updates[SettingKeyAPIKeyACLTrustForwardedIP])
			require.JSONEq(t, `[]`, repo.updates[SettingKeyForwardedClientIPHeaders])
			if test.wantMigrationMarkerSet {
				require.Equal(t, "true", repo.updates[settingKeyForwardedClientIPModeV2])
		REDACTED else {
				require.NotContains(t, repo.updates, settingKeyForwardedClientIPModeV2)
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestSettingService_LoadForwardedClientIPSettingsLoadsHeaders(t *testing.T) {
	repo := &forwardedIPMigrationRepoStub{values: map[string]string{
		SettingKeyAPIKeyACLTrustForwardedIP: "true",
		SettingKeyForwardedClientIPHeaders:  `[" x-cdn-ip ","true-client-ip"]`,
		settingKeyForwardedClientIPModeV2:   "true",
REDACTEDREDACTED
	cfg := &config.Config{REDACTED
	svc := NewSettingService(repo, cfg)

	require.NoError(t, svc.LoadForwardedClientIPSettings(context.Background()))
	runtimeSettings := cfg.ForwardedClientIPSettings()
	require.True(t, runtimeSettings.TrustForwardedIP)
	require.Equal(t, []string{"X-Cdn-Ip", "True-Client-Ip"REDACTED, runtimeSettings.Headers)
	require.Nil(t, repo.updates)
REDACTED

func TestSettingService_LoadForwardedClientIPSettingsMalformedHeadersDisablesCustomTrust(t *testing.T) {
	repo := &forwardedIPMigrationRepoStub{values: map[string]string{
		SettingKeyAPIKeyACLTrustForwardedIP: "true",
		SettingKeyForwardedClientIPHeaders:  `["X Invalid"]`,
REDACTEDREDACTED
	cfg := &config.Config{REDACTED
	svc := NewSettingService(repo, cfg)

	err := svc.LoadForwardedClientIPSettings(context.Background())

	require.ErrorContains(t, err, "load forwarded client ip headers")
	runtimeSettings := cfg.ForwardedClientIPSettings()
	require.False(t, runtimeSettings.TrustForwardedIP)
	require.Empty(t, runtimeSettings.Headers)
	require.Equal(t, "true", repo.updates[settingKeyForwardedClientIPModeV2])
	require.NotContains(t, repo.updates, SettingKeyAPIKeyACLTrustForwardedIP)
REDACTED

func TestSettingService_LoadForwardedClientIPSettingsBackfillsConfigHeaders(t *testing.T) {
	repo := &forwardedIPMigrationRepoStub{values: map[string]string{
		settingKeyForwardedClientIPModeV2: "true",
REDACTEDREDACTED
	cfg := &config.Config{REDACTED
	cfg.SetForwardedClientIPSettings(false, []string{"X-Config-IP"REDACTED)
	svc := NewSettingService(repo, cfg)

	require.NoError(t, svc.LoadForwardedClientIPSettings(context.Background()))
	require.JSONEq(t, `["X-Config-IP"]`, repo.updates[SettingKeyForwardedClientIPHeaders])
	require.Equal(t, []string{"X-Config-IP"REDACTED, cfg.ForwardedClientIPSettings().Headers)
REDACTED

func TestSettingService_LoadForwardedClientIPSettingsReadFailureFailsClosed(t *testing.T) {
	repo := &forwardedIPMigrationRepoStub{
		getMultipleErr: errors.New("database unavailable"),
REDACTED
	cfg := &config.Config{REDACTED
	cfg.SetTrustForwardedIPForAPIKeyACL(true)
	svc := NewSettingService(repo, cfg)

	err := svc.LoadForwardedClientIPSettings(context.Background())

	require.ErrorContains(t, err, "get forwarded client ip settings")
	runtimeSettings := cfg.ForwardedClientIPSettings()
	require.False(t, runtimeSettings.TrustForwardedIP)
	require.Empty(t, runtimeSettings.Headers)
REDACTED

func TestSettingService_LoadForwardedClientIPSettingsWriteFailureUsesComputedMode(t *testing.T) {
	tests := []struct {
		name              string
		trustedProxiesSet bool
		wantEnabled       bool
REDACTED{
		{name: "compatibility migration remains effective", wantEnabled: trueREDACTED,
		{name: "explicit proxy policy remains secure", trustedProxiesSet: true, wantEnabled: falseREDACTED,
REDACTED

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := &forwardedIPMigrationRepoStub{
				values:         map[string]string{SettingKeyAPIKeyACLTrustForwardedIP: "false"REDACTED,
				setMultipleErr: errors.New("database unavailable"),
		REDACTED
			cfg := &config.Config{Server: config.ServerConfig{TrustedProxiesConfigured: test.trustedProxiesSetREDACTEDREDACTED
			svc := NewSettingService(repo, cfg)

			err := svc.LoadForwardedClientIPSettings(context.Background())

			require.ErrorContains(t, err, "migrate forwarded client ip setting")
			require.Equal(t, test.wantEnabled, cfg.TrustForwardedIPForAPIKeyACL())
	REDACTED)
REDACTED
REDACTED

func TestSettingService_GetAntigravityUserAgentVersion_Precedence(t *testing.T) {
	t.Run("后台设置优先", func(t *testing.T) {
		svc := NewSettingService(&settingAntigravityUARepoStub{values: map[string]string{
			SettingKeyAntigravityUserAgentVersion: "1.24.0",
REDACTED &config.Config{REDACTED)

		require.Equal(t, "1.24.0", svc.GetAntigravityUserAgentVersion(context.Background()))
REDACTED)

	t.Run("空值回退配置默认值", func(t *testing.T) {
		svc := NewSettingService(&settingAntigravityUARepoStub{values: map[string]string{
			SettingKeyAntigravityUserAgentVersion: "",
REDACTED &config.Config{REDACTED)

		require.Equal(t, antigravity.GetDefaultUserAgentVersion(), svc.GetAntigravityUserAgentVersion(context.Background()))
REDACTED)

	t.Run("缺失回退配置默认值", func(t *testing.T) {
		svc := NewSettingService(&settingAntigravityUARepoStub{values: map[string]string{REDACTEDREDACTED, &config.Config{REDACTED)

		require.Equal(t, antigravity.GetDefaultUserAgentVersion(), svc.GetAntigravityUserAgentVersion(context.Background()))
REDACTED)
REDACTED

func TestSettingService_UpdateSettings_RejectsInvalidPaymentVisibleMethodSource(t *testing.T) {
	repo := &settingUpdateRepoStub{REDACTED
	svc := NewSettingService(repo, &config.Config{REDACTED)

	err := svc.UpdateSettings(context.Background(), &SystemSettings{
		PaymentVisibleMethodAlipaySource: "not-a-provider",
REDACTED)
REDACTED
	require.Equal(t, "INVALID_PAYMENT_VISIBLE_METHOD_SOURCE", infraerrors.Reason(err))
	require.Nil(t, repo.updates)
REDACTED

func TestSettingService_PasskeySwitchPersistsAndDefaultsToConfigured(t *testing.T) {
	cfg := &config.Config{WebAuthn: config.WebAuthnConfig{
		Enabled:   true,
		RPID:      "sub3.nebula-spaces.com",
		RPOrigins: []string{"https://sub3.nebula-spaces.com"REDACTED,
REDACTEDREDACTED
	runtimeRepo := &forwardedIPMigrationRepoStub{values: map[string]string{REDACTEDREDACTED
	runtimeService := NewSettingService(runtimeRepo, cfg)

	enabled, err := runtimeService.PasskeyEnabled(context.Background())
REDACTED
	require.True(t, enabled)

	updateRepo := &settingUpdateRepoStub{REDACTED
	updateService := NewSettingService(updateRepo, cfg)
	require.NoError(t, updateService.UpdateSettings(context.Background(), &SystemSettings{
		PasskeyEnabled: false,
REDACTED))
	require.Equal(t, "false", updateRepo.updates[SettingKeyPasskeyEnabled])

	runtimeRepo.values[SettingKeyPasskeyEnabled] = "false"
	enabled, err = runtimeService.PasskeyEnabled(context.Background())
REDACTED
	require.False(t, enabled)
	publicSettings, err := runtimeService.GetPublicSettings(context.Background())
REDACTED
	require.False(t, publicSettings.PasskeyEnabled)
REDACTED

// 移除 WebAuthn 配置后，残留的 passkey_enabled="true" 不得再让 GetAllSettings
// 报告开关开启：admin 更新门控以此为准，一旦误报为 true 会拒绝所有设置保存，
// 而此时前端开关处于禁用态，管理员无法在 UI 里自救。
func TestSettingService_StalePasskeyTrueWithoutConfigReportsDisabled(t *testing.T) {
	repo := &settingGetAllRepoStub{values: map[string]string{
		SettingKeyPasskeyEnabled: "true",
REDACTEDREDACTED
	service := NewSettingService(repo, &config.Config{REDACTED)

	settings, err := service.GetAllSettings(context.Background())
REDACTED
	require.False(t, settings.PasskeyEnabled)
REDACTED
