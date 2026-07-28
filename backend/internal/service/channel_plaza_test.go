//go:build unit

package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

// newPlazaChannelService 构造 ListPlazaGroups 测试用的 ChannelService。
func newPlazaChannelService(channels []Channel, groups []Group, pricing *PricingService) *ChannelService {
	repo := &mockChannelRepository{
		listAllFn: func(ctx context.Context) ([]Channel, error) { return channels, nil REDACTED,
REDACTED
	svc := NewChannelService(repo, &stubGroupRepoForAvailable{activeGroups: groupsREDACTED, nil, nil)
	svc.pricingService = pricing
	return svc
REDACTED

func plazaPricedChannel(id int64, name string, groupIDs []int64, platform string, models ...string) Channel {
	return Channel{
		ID:       id,
		Name:     name,
		Status:   StatusActive,
		GroupIDs: groupIDs,
		ModelPricing: []ChannelModelPricing{{
			Platform:    platform,
			Models:      models,
			BillingMode: BillingModeToken,
			InputPrice:  testPtrFloat64(3e-6),
			OutputPrice: testPtrFloat64(1.5e-5),
REDACTED
REDACTED
REDACTED

func TestListPlazaGroups_GroupCentricAggregation(t *testing.T) {
	// 两个渠道挂同一分组:模型并入同一 PlazaGroup;无模型的分组不返回。
	channels := []Channel{
		plazaPricedChannel(1, "chA", []int64{10REDACTED, "anthropic", "claude-sonnet"),
		plazaPricedChannel(2, "chB", []int64{10REDACTED, "anthropic", "claude-opus"),
REDACTED
	groups := []Group{
		{ID: 10, Name: "g-main", Description: "desc", Platform: "anthropic", RateMultiplier: 1REDACTED,
		{ID: 20, Name: "g-empty", Platform: "anthropic", RateMultiplier: 0.5REDACTED,
REDACTED
	svc := newPlazaChannelService(channels, groups, nil)
	out, err := svc.ListPlazaGroups(context.Background())
REDACTED
	require.Len(t, out, 1, "无模型的分组不应返回")
	require.Equal(t, int64(10), out[0].ID)
	require.Equal(t, "desc", out[0].Description)
	require.Len(t, out[0].Models, 2)
	// 组内模型按名称排序
	require.Equal(t, "claude-opus", out[0].Models[0].Name)
	require.Equal(t, "claude-sonnet", out[0].Models[1].Name)
REDACTED

func TestListPlazaGroups_DedupFirstWinsWithPricingUpgrade(t *testing.T) {
	// 同名模型:先见者胜;仅当已存条目无定价而新条目有定价时升级替换。
	unpriced := Channel{
		ID: 1, Name: "alpha", Status: StatusActive, GroupIDs: []int64{10REDACTED,
		// mapping-only → SupportedModels 产出无定价条目
		ModelMapping: map[string]map[string]string{
			"anthropic": {"claude-sonnet": "claude-sonnet"REDACTED,
	REDACTED,
REDACTED
	priced := plazaPricedChannel(2, "beta", []int64{10REDACTED, "anthropic", "claude-sonnet")
	groups := []Group{{ID: 10, Name: "g", Platform: "anthropic", RateMultiplier: 1REDACTEDREDACTED

	// alpha(无价)按名称序先于 beta(有价):先见者无价,应被有价条目升级。
	svc := newPlazaChannelService([]Channel{priced, unpricedREDACTED, groups, nil)
	out, err := svc.ListPlazaGroups(context.Background())
REDACTED
	require.Len(t, out, 1)
	require.Len(t, out[0].Models, 1)
	require.NotNil(t, out[0].Models[0].Pricing, "无价条目应被有价条目升级")
	require.NotNil(t, out[0].Models[0].Pricing.InputPrice)
REDACTED

func TestListPlazaGroups_PlatformIsolation(t *testing.T) {
	// 渠道同时有 anthropic/openai 定价,anthropic 分组只应看到 anthropic 模型。
	ch := Channel{
		ID: 1, Name: "multi", Status: StatusActive, GroupIDs: []int64{10, 20REDACTED,
		ModelPricing: []ChannelModelPricing{
			{Platform: "anthropic", Models: []string{"claude-sonnet"REDACTED, InputPrice: testPtrFloat64(3e-6)REDACTED,
			{Platform: "openai", Models: []string{"gpt-5"REDACTED, InputPrice: testPtrFloat64(2e-6)REDACTED,
	REDACTED,
REDACTED
	groups := []Group{
		{ID: 10, Name: "g-claude", Platform: "anthropic", RateMultiplier: 1REDACTED,
		{ID: 20, Name: "g-gpt", Platform: "openai", RateMultiplier: 1REDACTED,
REDACTED
	svc := newPlazaChannelService([]Channel{chREDACTED, groups, nil)
	out, err := svc.ListPlazaGroups(context.Background())
REDACTED
	require.Len(t, out, 2)
	byName := map[string][]PlazaModel{REDACTED
	for _, g := range out {
		byName[g.Name] = g.Models
REDACTED
	require.Len(t, byName["g-claude"], 1)
	require.Equal(t, "claude-sonnet", byName["g-claude"][0].Name)
	require.Len(t, byName["g-gpt"], 1)
	require.Equal(t, "gpt-5", byName["g-gpt"][0].Name)
REDACTED

func TestListPlazaGroups_InactiveChannelSkipped(t *testing.T) {
	inactive := plazaPricedChannel(1, "off", []int64{10REDACTED, "anthropic", "claude-sonnet")
	inactive.Status = "inactive"
	groups := []Group{{ID: 10, Name: "g", Platform: "anthropic", RateMultiplier: 1REDACTEDREDACTED
	svc := newPlazaChannelService([]Channel{inactiveREDACTED, groups, nil)
	out, err := svc.ListPlazaGroups(context.Background())
REDACTED
	require.Empty(t, out)
REDACTED

func TestListPlazaGroups_SortedByRateMultiplierAsc(t *testing.T) {
	channels := []Channel{
		plazaPricedChannel(1, "ch", []int64{10, 20, 30REDACTED, "anthropic", "claude-sonnet"),
REDACTED
	groups := []Group{
		{ID: 10, Name: "b-standard", Platform: "anthropic", RateMultiplier: 1REDACTED,
		{ID: 20, Name: "a-standard", Platform: "anthropic", RateMultiplier: 1REDACTED,
		{ID: 30, Name: "cheap", Platform: "anthropic", RateMultiplier: 0.5REDACTED,
REDACTED
	svc := newPlazaChannelService(channels, groups, nil)
	out, err := svc.ListPlazaGroups(context.Background())
REDACTED
	require.Len(t, out, 3)
	require.Equal(t, "cheap", out[0].Name, "倍率低者在前")
	require.Equal(t, "a-standard", out[1].Name, "同倍率按名称")
	require.Equal(t, "b-standard", out[2].Name)
REDACTED

func TestListPlazaGroups_OfficialPricingFill(t *testing.T) {
	pricingSvc := newStubPricingServiceFromMap(map[string]*LiteLLMModelPricing{
		"claude-sonnet": {
			Mode:                                "chat",
			InputCostPerToken:                   3e-6,
			OutputCostPerToken:                  1.5e-5,
			CacheCreationInputTokenCost:         3.75e-6,
			CacheCreationInputTokenCostAbove1hr: 6e-6,
			CacheReadInputTokenCost:             3e-7,
	REDACTED,
		"token-absent": {Mode: "image_generation", TokenPricingAbsent: true, OutputCostPerImage: 0.04REDACTED,
REDACTED)
	channels := []Channel{
		plazaPricedChannel(1, "ch", []int64{10REDACTED, "anthropic", "claude-sonnet", "unknown-model", "token-absent"),
REDACTED
	groups := []Group{{ID: 10, Name: "g", Platform: "anthropic", RateMultiplier: 1REDACTEDREDACTED
	svc := newPlazaChannelService(channels, groups, pricingSvc)
	out, err := svc.ListPlazaGroups(context.Background())
REDACTED
	require.Len(t, out, 1)
	require.Len(t, out[0].Models, 3)

	byName := map[string]PlazaModel{REDACTED
	for _, m := range out[0].Models {
		byName[m.Name] = m
REDACTED
	// 命中:填充完整官方价(含 1h 缓存写入)
	official := byName["claude-sonnet"].OfficialPricing
	require.NotNil(t, official)
	require.InDelta(t, 3e-6, *official.InputPrice, 1e-12)
	require.InDelta(t, 6e-6, *official.CacheWrite1hPrice, 1e-12)
	require.InDelta(t, 3e-7, *official.CacheReadPrice, 1e-12)
	// 未命中:nil(GetModelPricing 的 claude 系列模糊匹配对非 claude 名不生效)
	require.Nil(t, byName["unknown-model"].OfficialPricing)
	// TokenPricingAbsent 条目不作为官方 token 价展示
	require.Nil(t, byName["token-absent"].OfficialPricing)
REDACTED

func TestListPlazaGroups_RepoErrorsPropagate(t *testing.T) {
	sentinel := errors.New("boom")
	repo := &mockChannelRepository{
		listAllFn: func(ctx context.Context) ([]Channel, error) { return nil, sentinel REDACTED,
REDACTED
	svc := NewChannelService(repo, &stubGroupRepoForAvailable{REDACTED, nil, nil)
	out, err := svc.ListPlazaGroups(context.Background())
	require.Nil(t, out)
	require.ErrorIs(t, err, sentinel)

	svc2 := NewChannelService(
		&mockChannelRepository{listAllFn: func(ctx context.Context) ([]Channel, error) { return nil, nil REDACTEDREDACTED,
		&stubGroupRepoForAvailable{listActiveErr: sentinelREDACTED,
		nil, nil,
	)
	out2, err2 := svc2.ListPlazaGroups(context.Background())
	require.Nil(t, out2)
	require.ErrorIs(t, err2, sentinel)
REDACTED
