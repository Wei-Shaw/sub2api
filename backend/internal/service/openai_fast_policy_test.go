package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type openAIFastPolicyRepoStub struct {
	values map[string]string
REDACTED

func (s *openAIFastPolicyRepoStub) Get(ctx context.Context, key string) (*Setting, error) {
	panic("unexpected Get call")
REDACTED

func (s *openAIFastPolicyRepoStub) GetValue(ctx context.Context, key string) (string, error) {
	if v, ok := s.values[key]; ok {
		return v, nil
REDACTED
	return "", ErrSettingNotFound
REDACTED

func (s *openAIFastPolicyRepoStub) Set(ctx context.Context, key, value string) error {
	if s.values == nil {
		s.values = map[string]string{REDACTED
REDACTED
	s.values[key] = value
	return nil
REDACTED

func (s *openAIFastPolicyRepoStub) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	panic("unexpected GetMultiple call")
REDACTED

func (s *openAIFastPolicyRepoStub) SetMultiple(ctx context.Context, settings map[string]string) error {
	panic("unexpected SetMultiple call")
REDACTED

func (s *openAIFastPolicyRepoStub) GetAll(ctx context.Context) (map[string]string, error) {
	panic("unexpected GetAll call")
REDACTED

func (s *openAIFastPolicyRepoStub) Delete(ctx context.Context, key string) error {
	panic("unexpected Delete call")
REDACTED

func newOpenAIGatewayServiceWithSettings(t *testing.T, settings *OpenAIFastPolicySettings) *OpenAIGatewayService {
REDACTED
	repo := &openAIFastPolicyRepoStub{values: map[string]string{REDACTEDREDACTED
	if settings != nil {
		raw, err := json.Marshal(settings)
	REDACTED
		repo.values[SettingKeyOpenAIFastPolicySettings] = string(raw)
REDACTED
	return &OpenAIGatewayService{
		settingService: NewSettingService(repo, &config.Config{REDACTED),
REDACTED
REDACTED

func openAIFastFilterPriorityPolicy() *OpenAIFastPolicySettings {
	return &OpenAIFastPolicySettings{
		Rules: []OpenAIFastPolicyRule{{
			ServiceTier:    OpenAIFastTierPriority,
			Action:         BetaPolicyActionFilter,
			Scope:          BetaPolicyScopeAll,
			ModelWhitelist: []string{REDACTED,
			FallbackAction: BetaPolicyActionPass,
REDACTED
REDACTED
REDACTED

func TestEvaluateOpenAIFastPolicy_DefaultPassesKnownTiers(t *testing.T) {
	require.Empty(t, DefaultOpenAIFastPolicySettings().Rules, "default policy must not rewrite service_tier unless admin configured rules")

	svc := newOpenAIGatewayServiceWithSettings(t, DefaultOpenAIFastPolicySettings())
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKeyREDACTED

	action, _ := svc.evaluateOpenAIFastPolicy(context.Background(), account, "gpt-5.5", OpenAIFastTierPriority)
	require.Equal(t, BetaPolicyActionPass, action)

	action, _ = svc.evaluateOpenAIFastPolicy(context.Background(), account, "gpt-5.5-turbo", OpenAIFastTierPriority)
	require.Equal(t, BetaPolicyActionPass, action)

	action, _ = svc.evaluateOpenAIFastPolicy(context.Background(), account, "gpt-4", OpenAIFastTierPriority)
	require.Equal(t, BetaPolicyActionPass, action)

	action, _ = svc.evaluateOpenAIFastPolicy(context.Background(), account, "gpt-5.5", OpenAIFastTierFlex)
	require.Equal(t, BetaPolicyActionPass, action)

	// empty tier → pass
	action, _ = svc.evaluateOpenAIFastPolicy(context.Background(), account, "gpt-5.5", "")
	require.Equal(t, BetaPolicyActionPass, action)
REDACTED

func TestEvaluateOpenAIFastPolicy_BlockRuleCarriesMessage(t *testing.T) {
	settings := &OpenAIFastPolicySettings{
		Rules: []OpenAIFastPolicyRule{{
			ServiceTier:    OpenAIFastTierPriority,
			Action:         BetaPolicyActionBlock,
			Scope:          BetaPolicyScopeAll,
			ErrorMessage:   "fast mode is not allowed",
			ModelWhitelist: []string{"gpt-5.5"REDACTED,
			FallbackAction: BetaPolicyActionPass,
REDACTED
REDACTED
	svc := newOpenAIGatewayServiceWithSettings(t, settings)
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKeyREDACTED

	action, msg := svc.evaluateOpenAIFastPolicy(context.Background(), account, "gpt-5.5", OpenAIFastTierPriority)
	require.Equal(t, BetaPolicyActionBlock, action)
	require.Equal(t, "fast mode is not allowed", msg)
REDACTED

func TestEvaluateOpenAIFastPolicy_ScopeFiltersOAuth(t *testing.T) {
	settings := &OpenAIFastPolicySettings{
		Rules: []OpenAIFastPolicyRule{{
			ServiceTier: OpenAIFastTierAny,
			Action:      BetaPolicyActionFilter,
			Scope:       BetaPolicyScopeOAuth,
REDACTED
REDACTED
	svc := newOpenAIGatewayServiceWithSettings(t, settings)

	// OAuth account → rule matches
	oauthAccount := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuthREDACTED
	action, _ := svc.evaluateOpenAIFastPolicy(context.Background(), oauthAccount, "gpt-4", OpenAIFastTierPriority)
	require.Equal(t, BetaPolicyActionFilter, action)

	// API Key account → rule skipped → pass
	apiKeyAccount := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKeyREDACTED
	action, _ = svc.evaluateOpenAIFastPolicy(context.Background(), apiKeyAccount, "gpt-4", OpenAIFastTierPriority)
	require.Equal(t, BetaPolicyActionPass, action)
REDACTED

func TestApplyOpenAIFastPolicyToBody_DefaultPassesPriorityAndFast(t *testing.T) {
	svc := newOpenAIGatewayServiceWithSettings(t, DefaultOpenAIFastPolicySettings())
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKeyREDACTED

	body := []byte(`{"model":"gpt-5.5","service_tier":"priority","messages":[]REDACTED`)
	updated, err := svc.applyOpenAIFastPolicyToBody(context.Background(), account, "gpt-5.5", body)
REDACTED
	require.Equal(t, string(body), string(updated))

	body = []byte(`{"model":"gpt-5.5","service_tier":"fast"REDACTED`)
	updated, err = svc.applyOpenAIFastPolicyToBody(context.Background(), account, "gpt-5.5", body)
REDACTED
	require.Equal(t, "priority", gjson.GetBytes(updated, "service_tier").String())

	body = []byte(`{"model":"gpt-4","service_tier":"priority"REDACTED`)
	updated, err = svc.applyOpenAIFastPolicyToBody(context.Background(), account, "gpt-4", body)
REDACTED
	require.Equal(t, string(body), string(updated))

	// No service_tier → no-op
	body = []byte(`{"model":"gpt-5.5"REDACTED`)
	updated, err = svc.applyOpenAIFastPolicyToBody(context.Background(), account, "gpt-5.5", body)
REDACTED
	require.Equal(t, string(body), string(updated))
REDACTED

func TestApplyOpenAIFastPolicyToBody_ExplicitFilterRemovesField(t *testing.T) {
	svc := newOpenAIGatewayServiceWithSettings(t, openAIFastFilterPriorityPolicy())
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKeyREDACTED

	body := []byte(`{"model":"gpt-5.5","service_tier":"priority","messages":[]REDACTED`)
	updated, err := svc.applyOpenAIFastPolicyToBody(context.Background(), account, "gpt-5.5", body)
REDACTED
	require.NotContains(t, string(updated), `"service_tier"`)

	body = []byte(`{"model":"gpt-5.5","service_tier":"fast"REDACTED`)
	updated, err = svc.applyOpenAIFastPolicyToBody(context.Background(), account, "gpt-5.5", body)
REDACTED
	require.NotContains(t, string(updated), `"service_tier"`)
REDACTED

func TestApplyOpenAIFastPolicyToBody_ForcePriorityRewritesKnownTier(t *testing.T) {
	settings := &OpenAIFastPolicySettings{
		Rules: []OpenAIFastPolicyRule{{
			ServiceTier: OpenAIFastTierAny,
			Action:      OpenAIFastPolicyActionForcePriority,
			Scope:       BetaPolicyScopeAll,
REDACTED
REDACTED
	svc := newOpenAIGatewayServiceWithSettings(t, settings)
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKeyREDACTED

	for _, tier := range []string{"flex", "auto", "default", "scale", "fast", "priority"REDACTED {
		body := []byte(`{"model":"gpt-5.5","service_tier":"` + tier + `"REDACTED`)
		updated, err := svc.applyOpenAIFastPolicyToBody(context.Background(), account, "gpt-5.5", body)
	REDACTED
		require.Equal(t, OpenAIFastTierPriority, gjson.GetBytes(updated, "service_tier").String(),
			"tier %q should be forced to priority", tier)
REDACTED
REDACTED

// TestApplyOpenAIFastPolicyToBody_OfficialTiersBypassDefaultRule 验证默认配置
// 下客户端显式发送的 OpenAI 官方合法 tier 能透传到上游而不被静默剥离。
func TestApplyOpenAIFastPolicyToBody_OfficialTiersBypassDefaultRule(t *testing.T) {
	svc := newOpenAIGatewayServiceWithSettings(t, DefaultOpenAIFastPolicySettings())
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKeyREDACTED

	for _, tier := range []string{"auto", "default", "scale"REDACTED {
		body := []byte(`{"model":"gpt-5.5","service_tier":"` + tier + `"REDACTED`)
		updated, err := svc.applyOpenAIFastPolicyToBody(context.Background(), account, "gpt-5.5", body)
		require.NoError(t, err, "tier %q should pass without error", tier)
		require.Contains(t, string(updated), `"service_tier":"`+tier+`"`,
			"tier %q should be preserved in body under default policy", tier)
REDACTED

	// evaluate 层也应判定为 pass（默认配置没有内置规则）。
	for _, tier := range []string{"auto", "default", "scale"REDACTED {
		action, _ := svc.evaluateOpenAIFastPolicy(context.Background(), account, "gpt-5.5", tier)
		require.Equal(t, BetaPolicyActionPass, action, "tier %q should evaluate to pass", tier)
REDACTED
REDACTED

// TestApplyOpenAIFastPolicyToBody_AllRuleStripsOfficialTiers 验证管理员显式配置
// ServiceTier=all + Action=filter 规则后，auto/default/scale 等官方 tier 也会
// 被剥离。这是符合预期的——首条匹配 short-circuit，"all" 覆盖任意已识别 tier。
func TestApplyOpenAIFastPolicyToBody_AllRuleStripsOfficialTiers(t *testing.T) {
	settings := &OpenAIFastPolicySettings{
		Rules: []OpenAIFastPolicyRule{{
			ServiceTier: OpenAIFastTierAny,
			Action:      BetaPolicyActionFilter,
			Scope:       BetaPolicyScopeAll,
REDACTED
REDACTED
	svc := newOpenAIGatewayServiceWithSettings(t, settings)
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKeyREDACTED

	for _, tier := range []string{"auto", "default", "scale", "priority", "flex"REDACTED {
		body := []byte(`{"model":"gpt-5.5","service_tier":"` + tier + `"REDACTED`)
		updated, err := svc.applyOpenAIFastPolicyToBody(context.Background(), account, "gpt-5.5", body)
	REDACTED
		require.NotContains(t, string(updated), `"service_tier"`,
			"tier %q should be stripped under ServiceTier=all + filter rule", tier)
REDACTED
REDACTED

// TestApplyOpenAIFastPolicyToBody_UnknownTierStripped 验证真未知 tier 仍被剥离
// （normalize 返回 nil → normalizeResponsesBodyServiceTier 删除字段；
// applyOpenAIFastPolicyToBody 在 normTier 为空时直接 no-op，因为字段已不可能存在
// 于经过前置归一化的请求里。这里直接调 apply 验证它对未识别值不会异常）。
func TestApplyOpenAIFastPolicyToBody_UnknownTierStripped(t *testing.T) {
	svc := newOpenAIGatewayServiceWithSettings(t, DefaultOpenAIFastPolicySettings())
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKeyREDACTED

	// normalize 阶段会将未知值剥离
	require.Nil(t, normalizeOpenAIServiceTier("xxx"))

	// applyOpenAIFastPolicyToBody 收到未识别 tier 时不报错，body 透传不变
	// （不属于本函数职责——上层 normalizeResponsesBodyServiceTier 已剥离）
	body := []byte(`{"model":"gpt-5.5","service_tier":"xxx"REDACTED`)
	updated, err := svc.applyOpenAIFastPolicyToBody(context.Background(), account, "gpt-5.5", body)
REDACTED
	require.Equal(t, string(body), string(updated))
REDACTED

func TestApplyOpenAIFastPolicyToBody_BlockReturnsTypedError(t *testing.T) {
	settings := &OpenAIFastPolicySettings{
		Rules: []OpenAIFastPolicyRule{{
			ServiceTier:    OpenAIFastTierPriority,
			Action:         BetaPolicyActionBlock,
			Scope:          BetaPolicyScopeAll,
			ErrorMessage:   "fast mode is blocked for gpt-5.5",
			ModelWhitelist: []string{"gpt-5.5"REDACTED,
			FallbackAction: BetaPolicyActionPass,
REDACTED
REDACTED
	svc := newOpenAIGatewayServiceWithSettings(t, settings)
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKeyREDACTED

	body := []byte(`{"model":"gpt-5.5","service_tier":"priority"REDACTED`)
	updated, err := svc.applyOpenAIFastPolicyToBody(context.Background(), account, "gpt-5.5", body)
REDACTED
	var blocked *OpenAIFastBlockedError
	require.True(t, errors.As(err, &blocked))
	require.Contains(t, blocked.Message, "fast mode is blocked")
	require.Equal(t, string(body), string(updated)) // body not mutated on block
REDACTED

func TestSetOpenAIFastPolicySettings_Validation(t *testing.T) {
	repo := &openAIFastPolicyRepoStub{values: map[string]string{REDACTEDREDACTED
	svc := NewSettingService(repo, &config.Config{REDACTED)

	// Invalid action rejected
	err := svc.SetOpenAIFastPolicySettings(context.Background(), &OpenAIFastPolicySettings{
		Rules: []OpenAIFastPolicyRule{{
			ServiceTier: OpenAIFastTierPriority,
			Action:      "bogus",
			Scope:       BetaPolicyScopeAll,
REDACTED
REDACTED)
REDACTED

	// Invalid service_tier rejected
	err = svc.SetOpenAIFastPolicySettings(context.Background(), &OpenAIFastPolicySettings{
		Rules: []OpenAIFastPolicyRule{{
			ServiceTier: "turbo",
			Action:      BetaPolicyActionPass,
			Scope:       BetaPolicyScopeAll,
REDACTED
REDACTED)
REDACTED

	// Valid settings persisted
	err = svc.SetOpenAIFastPolicySettings(context.Background(), &OpenAIFastPolicySettings{
		Rules: []OpenAIFastPolicyRule{{
			ServiceTier: OpenAIFastTierPriority,
			Action:      OpenAIFastPolicyActionForcePriority,
			Scope:       BetaPolicyScopeAll,
REDACTED
REDACTED)
REDACTED

	got, err := svc.GetOpenAIFastPolicySettings(context.Background())
REDACTED
	require.Len(t, got.Rules, 1)
	require.Equal(t, OpenAIFastTierPriority, got.Rules[0].ServiceTier)
	require.Equal(t, OpenAIFastPolicyActionForcePriority, got.Rules[0].Action)
REDACTED
