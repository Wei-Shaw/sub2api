//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOpenAISelectAccountForModelWithExclusions_ChannelMappedRestrictionRejectsEarly(t *testing.T) {
	t.Parallel()

	channelSvc := newTestChannelService(makeStandardRepo(Channel{
		ID:                 1,
		Status:             StatusActive,
		GroupIDs:           []int64{10REDACTED,
		RestrictModels:     true,
		BillingModelSource: BillingModelSourceChannelMapped,
		ModelPricing: []ChannelModelPricing{
			{Platform: PlatformOpenAI, Models: []string{"gpt-4o"REDACTEDREDACTED,
	REDACTED,
		ModelMapping: map[string]map[string]string{
			PlatformOpenAI: {"gpt-4.1": "o3-mini"REDACTED,
	REDACTED,
REDACTED, map[int64]string{10: PlatformOpenAIREDACTED))

	svc := &OpenAIGatewayService{
		accountRepo: stubOpenAIAccountRepo{accounts: []Account{
			{ID: 1, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: trueREDACTED,
REDACTED
		channelService: channelSvc,
REDACTED

	groupID := int64(10)
	_, err := svc.SelectAccountForModelWithExclusions(context.Background(), &groupID, "", "gpt-4.1", nil)
	require.ErrorIs(t, err, ErrNoAvailableAccounts)
	require.Contains(t, err.Error(), "channel pricing restriction")
REDACTED

func TestOpenAISelectAccountForModelWithExclusions_UpstreamRestrictionSkipsDisallowedAccount(t *testing.T) {
	t.Parallel()

	channelSvc := newTestChannelService(makeStandardRepo(Channel{
		ID:                 1,
		Status:             StatusActive,
		GroupIDs:           []int64{10REDACTED,
		RestrictModels:     true,
		BillingModelSource: BillingModelSourceUpstream,
		ModelPricing: []ChannelModelPricing{
			{Platform: PlatformOpenAI, Models: []string{"o3-mini"REDACTEDREDACTED,
	REDACTED,
REDACTED, map[int64]string{10: PlatformOpenAIREDACTED))

	svc := &OpenAIGatewayService{
		accountRepo: stubOpenAIAccountRepo{accounts: []Account{
			{
				ID:          1,
				Platform:    PlatformOpenAI,
				Status:      StatusActive,
				Schedulable: true,
				Priority:    10,
		REDACTED
					"model_mapping": map[string]any{"gpt-4.1": "gpt-4o"REDACTED,
			REDACTED,
		REDACTED,
			{
				ID:          2,
				Platform:    PlatformOpenAI,
				Status:      StatusActive,
				Schedulable: true,
				Priority:    20,
		REDACTED
					"model_mapping": map[string]any{"gpt-4.1": "o3-mini"REDACTED,
			REDACTED,
		REDACTED,
REDACTED
		channelService: channelSvc,
REDACTED

	groupID := int64(10)
	account, err := svc.SelectAccountForModelWithExclusions(context.Background(), &groupID, "", "gpt-4.1", nil)
REDACTED
	require.NotNil(t, account)
	require.Equal(t, int64(2), account.ID)
REDACTED

func TestIsUpstreamModelRestrictedByChannel_CompactMappingMatchesForwardPath(t *testing.T) {
	t.Parallel()

	account := &Account{
		Platform: PlatformOpenAI,
REDACTED
			"model_mapping":         map[string]any{"gpt-5.4-channel": "gpt-5.4-account"REDACTED,
			"compact_model_mapping": map[string]any{"gpt-5.4-account": "gpt-5.4-compact"REDACTED,
	REDACTED,
REDACTED
	tests := []struct {
		name                   string
		allowedUpstreamModel   string
		useCompactModelMapping bool
REDACTED{
		{
			name:                   "legacy compact applies compact mapping after channel and account mapping",
			allowedUpstreamModel:   "gpt-5.4-compact",
			useCompactModelMapping: true,
	REDACTED,
		{
			name:                   "native v2 stops after channel and account mapping",
			allowedUpstreamModel:   "gpt-5.4-account",
			useCompactModelMapping: false,
	REDACTED,
REDACTED

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			channelSvc := newTestChannelService(makeStandardRepo(Channel{
				ID:                 1,
				Status:             StatusActive,
				GroupIDs:           []int64{10REDACTED,
				RestrictModels:     true,
				BillingModelSource: BillingModelSourceUpstream,
				ModelPricing: []ChannelModelPricing{
					{Platform: PlatformOpenAI, Models: []string{tt.allowedUpstreamModelREDACTEDREDACTED,
			REDACTED,
				ModelMapping: map[string]map[string]string{
					PlatformOpenAI: {"gpt-5.4": "gpt-5.4-channel"REDACTED,
			REDACTED,
		REDACTED, map[int64]string{10: PlatformOpenAIREDACTED))
			svc := &OpenAIGatewayService{channelService: channelSvcREDACTED
			mapping := channelSvc.ResolveChannelMapping(context.Background(), 10, "gpt-5.4")
			require.True(t, mapping.Mapped)
			require.Equal(t, "gpt-5.4-channel", mapping.MappedModel)

			ctx := WithOpenAIForwardModel(
				context.Background(),
				mapping.MappedModel,
				tt.useCompactModelMapping,
			)
			require.False(t, svc.isUpstreamModelRestrictedByChannel(
				ctx, 10, account, "gpt-5.4", true,
			))
			require.True(t, svc.isUpstreamModelRestrictedByChannel(
				context.Background(), 10, account, "gpt-5.4", true,
			), "without the forward-model context the restriction check follows a different chain")
	REDACTED)
REDACTED
REDACTED

func TestIsUpstreamModelRestrictedByChannel_PassthroughMatchesForwardPath(t *testing.T) {
	t.Parallel()

	account := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
REDACTED
			"model_mapping": map[string]any{"gpt-5.4-channel": "gpt-5.4-account"REDACTED,
			"compact_model_mapping": map[string]any{
				"gpt-5.4-channel": "gpt-5.4-compact",
		REDACTED,
	REDACTED,
		Extra: map[string]any{"openai_passthrough": trueREDACTED,
REDACTED
	tests := []struct {
		name                   string
		allowedUpstreamModel   string
		useCompactModelMapping bool
REDACTED{
		{
			name:                   "native v2 keeps channel-mapped model and ignores normal account mapping",
			allowedUpstreamModel:   "gpt-5.4-channel",
			useCompactModelMapping: false,
	REDACTED,
		{
			name:                   "legacy compact applies compact mapping to channel-mapped model",
			allowedUpstreamModel:   "gpt-5.4-compact",
			useCompactModelMapping: true,
	REDACTED,
REDACTED

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			channelSvc := newTestChannelService(makeStandardRepo(Channel{
				ID:                 1,
				Status:             StatusActive,
				GroupIDs:           []int64{10REDACTED,
				RestrictModels:     true,
				BillingModelSource: BillingModelSourceUpstream,
				ModelPricing: []ChannelModelPricing{
					{Platform: PlatformOpenAI, Models: []string{tt.allowedUpstreamModelREDACTEDREDACTED,
			REDACTED,
				ModelMapping: map[string]map[string]string{
					PlatformOpenAI: {"gpt-5.4": "gpt-5.4-channel"REDACTED,
			REDACTED,
		REDACTED, map[int64]string{10: PlatformOpenAIREDACTED))
			svc := &OpenAIGatewayService{channelService: channelSvcREDACTED
			mapping := channelSvc.ResolveChannelMapping(context.Background(), 10, "gpt-5.4")
			require.True(t, mapping.Mapped)
			require.Equal(t, "gpt-5.4-channel", mapping.MappedModel)

			ctx := WithOpenAIForwardModel(
				context.Background(),
				mapping.MappedModel,
				tt.useCompactModelMapping,
			)
			require.False(t, svc.isUpstreamModelRestrictedByChannel(
				ctx, 10, account, "gpt-5.4", true,
			))
	REDACTED)
REDACTED
REDACTED

func TestIsUpstreamModelRestrictedByChannel_PassthroughFlagWithRawChatFallbackMatchesForwardPath(t *testing.T) {
	t.Parallel()

	account := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
REDACTED
			"model_mapping": map[string]any{"gpt-5.4-channel": "gpt-5.4-account"REDACTED,
			"compact_model_mapping": map[string]any{
				"gpt-5.4-account": "gpt-5.4-compact",
		REDACTED,
	REDACTED,
		Extra: map[string]any{
			"openai_passthrough":         true,
			"openai_responses_supported": false,
	REDACTED,
REDACTED

	for _, useCompactModelMapping := range []bool{false, trueREDACTED {
		useCompactModelMapping := useCompactModelMapping
		name := "native v2"
		if useCompactModelMapping {
			name = "legacy compact"
	REDACTED
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			channelSvc := newTestChannelService(makeStandardRepo(Channel{
				ID:                 1,
				Status:             StatusActive,
				GroupIDs:           []int64{10REDACTED,
				RestrictModels:     true,
				BillingModelSource: BillingModelSourceUpstream,
				ModelPricing: []ChannelModelPricing{
					{Platform: PlatformOpenAI, Models: []string{"gpt-5.4-account"REDACTEDREDACTED,
			REDACTED,
				ModelMapping: map[string]map[string]string{
					PlatformOpenAI: {"gpt-5.4": "gpt-5.4-channel"REDACTED,
			REDACTED,
		REDACTED, map[int64]string{10: PlatformOpenAIREDACTED))
			svc := &OpenAIGatewayService{channelService: channelSvcREDACTED
			ctx := WithOpenAIForwardModel(
				context.Background(),
				"gpt-5.4-channel",
				useCompactModelMapping,
			)

			require.False(t, svc.isUpstreamModelRestrictedByChannel(
				ctx, 10, account, "gpt-5.4", true,
			))
	REDACTED)
REDACTED
REDACTED

func TestIsUpstreamModelRestrictedByChannel_ForwardModelContextMatchesNormalForwardPath(t *testing.T) {
	t.Parallel()

	account := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
REDACTED
			"model_mapping": map[string]any{"gpt-5.4-channel": "gpt-5.4-account"REDACTED,
	REDACTED,
		Extra: map[string]any{
			"openai_passthrough":         true,
			"openai_responses_supported": false,
	REDACTED,
REDACTED
	channelSvc := newTestChannelService(makeStandardRepo(Channel{
		ID:                 1,
		Status:             StatusActive,
		GroupIDs:           []int64{10REDACTED,
		RestrictModels:     true,
		BillingModelSource: BillingModelSourceUpstream,
		ModelPricing: []ChannelModelPricing{
			{Platform: PlatformOpenAI, Models: []string{"gpt-5.4-account"REDACTEDREDACTED,
	REDACTED,
		ModelMapping: map[string]map[string]string{
			PlatformOpenAI: {"gpt-5.4": "gpt-5.4-channel"REDACTED,
	REDACTED,
REDACTED, map[int64]string{10: PlatformOpenAIREDACTED))
	svc := &OpenAIGatewayService{channelService: channelSvcREDACTED
	ctx := WithOpenAIForwardModel(context.Background(), "gpt-5.4-channel", false)

	require.False(t, svc.isUpstreamModelRestrictedByChannel(
		ctx, 10, account, "gpt-5.4", false,
	))
REDACTED

func TestOpenAISelectAccountForModelWithExclusions_StickyRestrictedUpstreamFallsBack(t *testing.T) {
	t.Parallel()

	channelSvc := newTestChannelService(makeStandardRepo(Channel{
		ID:                 1,
		Status:             StatusActive,
		GroupIDs:           []int64{10REDACTED,
		RestrictModels:     true,
		BillingModelSource: BillingModelSourceUpstream,
		ModelPricing: []ChannelModelPricing{
			{Platform: PlatformOpenAI, Models: []string{"o3-mini"REDACTEDREDACTED,
	REDACTED,
REDACTED, map[int64]string{10: PlatformOpenAIREDACTED))

	cache := &stubGatewayCache{
		sessionBindings: map[string]int64{"openai:sticky-session": 1REDACTED,
REDACTED
	svc := &OpenAIGatewayService{
		accountRepo: stubOpenAIAccountRepo{accounts: []Account{
			{
				ID:          1,
				Platform:    PlatformOpenAI,
				Status:      StatusActive,
				Schedulable: true,
				Priority:    10,
		REDACTED
					"model_mapping": map[string]any{"gpt-4.1": "gpt-4o"REDACTED,
			REDACTED,
		REDACTED,
			{
				ID:          2,
				Platform:    PlatformOpenAI,
				Status:      StatusActive,
				Schedulable: true,
				Priority:    20,
		REDACTED
					"model_mapping": map[string]any{"gpt-4.1": "o3-mini"REDACTED,
			REDACTED,
		REDACTED,
REDACTED
		channelService: channelSvc,
		cache:          cache,
REDACTED

	groupID := int64(10)
	account, err := svc.SelectAccountForModelWithExclusions(context.Background(), &groupID, "sticky-session", "gpt-4.1", nil)
REDACTED
	require.NotNil(t, account)
	require.Equal(t, int64(2), account.ID)
	require.Equal(t, 1, cache.deletedSessions["openai:sticky-session"])
	require.Equal(t, int64(2), cache.sessionBindings["openai:sticky-session"])
REDACTED
