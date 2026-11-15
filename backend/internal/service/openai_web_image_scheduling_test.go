package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func webImageSchedulingAccount(id int64) Account {
	a := webImagePolicyAccount(map[string]any{"gpt-image-2": "gpt-image-2"})
	a.ID, a.Status, a.Schedulable, a.Concurrency = id, StatusActive, true, 1
	a.GroupIDs = []int64{101}
	return *a
}

func TestWebImageSchedulingSkipsUnauthorizedAccountsBeforeAcquiringSlots(t *testing.T) {
	for _, mode := range []struct {
		name, advanced string
		batch          bool
	}{
		{"legacy", "false", false},
		{"legacy_batch", "false", true},
		{"advanced", "true", true},
	} {
		for _, tc := range []struct {
			name, model string
			change      func(*Account)
		}{
			{"unconfigured", "gpt-image-2", func(a *Account) { delete(a.Extra, AccountExtraChatGPTWebImageEnabled) }},
			{"disabled", "gpt-image-2", func(a *Account) { a.Extra[AccountExtraChatGPTWebImageEnabled] = false }},
			{"no_consent", "gpt-image-2", func(a *Account) { delete(a.Extra, AccountExtraChatGPTWebImageRegularChat) }},
			{"invalid_switch", "gpt-image-2", func(a *Account) { a.Extra[AccountExtraChatGPTWebImageEnabled] = "true" }},
			{"unconfigured_flare", "gpt-image-2.5-flare", func(a *Account) { a.Extra = nil; a.Credentials["model_mapping"] = nil }},
		} {
			for _, sticky := range []bool{false, true} {
				name := mode.name + "/" + tc.name
				if sticky {
					name += "/sticky"
				}
				t.Run(name, func(t *testing.T) {
					resetOpenAIAdvancedSchedulerSettingCacheForTest()
					invalid, allowed := webImageSchedulingAccount(270), webImageSchedulingAccount(239)
					tc.change(&invalid)
					allowed.Priority = 10
					allowed.Credentials["model_mapping"] = map[string]any{tc.model: "gpt-image-2"}
					cfg := &config.Config{}
					cfg.Gateway.Scheduling.LoadBatchEnabled = mode.batch
					cache := &schedulerTestGatewayCache{}
					session := ""
					if sticky {
						session = "image_session"
						cache.sessionBindings = map[string]int64{"openai:" + session: invalid.ID}
					}
					var acquired []int64
					svc := &OpenAIGatewayService{
						accountRepo: schedulerTestOpenAIAccountRepo{accounts: []Account{invalid, allowed}},
						cache:       cache, cfg: cfg,
						rateLimitService:   newOpenAIAdvancedSchedulerRateLimitService(mode.advanced),
						concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{acquiredIDs: &acquired}),
					}
					groupID := int64(101)
					selection, _, err := svc.SelectAccountWithSchedulerForImages(context.Background(), &groupID, session, tc.model, nil, OpenAIImagesCapabilityNative)
					require.NoError(t, err)
					require.NotNil(t, selection)
					require.Equal(t, allowed.ID, selection.Account.ID)
					require.Equal(t, []int64{allowed.ID}, acquired, "unauthorized image accounts must not consume concurrency slots")
					selection.ReleaseFunc()
				})
			}
		}
	}
}

func TestWebImageSchedulingNoEligibleAccountDoesNotFallBackToUnauthorizedOAuth(t *testing.T) {
	for _, advanced := range []string{"false", "true"} {
		t.Run(advanced, func(t *testing.T) {
			resetOpenAIAdvancedSchedulerSettingCacheForTest()
			a := webImageSchedulingAccount(273)
			a.Credentials["model_mapping"] = nil
			a.Extra = nil
			var acquired []int64
			svc := &OpenAIGatewayService{
				accountRepo: schedulerTestOpenAIAccountRepo{accounts: []Account{a}}, cfg: &config.Config{},
				rateLimitService:   newOpenAIAdvancedSchedulerRateLimitService(advanced),
				concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{acquiredIDs: &acquired}),
			}
			groupID := int64(101)
			selection, _, err := svc.SelectAccountWithSchedulerForImages(context.Background(), &groupID, "", "gpt-image-2.5-flare", nil, OpenAIImagesCapabilityNative)
			require.ErrorIs(t, err, ErrNoAvailableAccounts)
			require.Nil(t, selection)
			require.Empty(t, acquired)
		})
	}
}

func TestWebImageSchedulingPreservesAliasesLegacyAccountsAndOrdinaryChat(t *testing.T) {
	for _, tc := range []struct {
		name, model string
		images      bool
		change      func(*Account)
	}{
		{"mapped_alias", "poster", true, func(a *Account) { a.Credentials["model_mapping"] = map[string]any{"poster": "gpt-image-2.5-flare"} }},
		{"mapped_wildcard", "gpt-image-2.5-flare", true, func(a *Account) { a.Credentials["model_mapping"] = map[string]any{"gpt-image-*": "gpt-image-2"} }},
		{"enabled_empty_mapping", "gpt-image-2", true, func(a *Account) { a.Credentials["model_mapping"] = nil }},
		{"enabled_chat_only_mapping", "gpt-image-2.5-flare", true, func(a *Account) { a.Credentials["model_mapping"] = map[string]any{"gpt-6-luna": "gpt-6-luna"} }},
		{"enabled_image_to_text_mapping", "gpt-image-2", true, func(a *Account) { a.Credentials["model_mapping"] = map[string]any{"gpt-image-2": "gpt-6-luna"} }},
		{"nonfree_legacy", "gpt-image-2", true, func(a *Account) {
			a.Extra = nil
			a.Credentials["plan_type"] = "pro"
			a.Credentials["model_mapping"] = nil
		}},
		{"unrestricted_chat", "gpt-6-luna", false, func(a *Account) { a.Credentials["model_mapping"] = nil }},
		{"disabled_bridge_chat", "gpt-6-luna", false, func(a *Account) {
			a.Extra[AccountExtraChatGPTWebImageEnabled] = false
			a.Credentials["model_mapping"] = nil
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := webImageSchedulingAccount(1)
			tc.change(&a)
			ctx := context.Background()
			req := OpenAIAccountScheduleRequest{RequestedModel: tc.model}
			if tc.images {
				ctx = WithOpenAIImagesEndpoint(ctx)
				req.RequiredImageCapability = OpenAIImagesCapabilityNative
			}
			require.Empty(t, openAICompatibleAccountEligibilityFailureReason(ctx, &a, PlatformOpenAI, tc.model, false, ""))
			scheduler := &defaultOpenAIAccountScheduler{}
			compatible, reason := scheduler.isAccountRequestCompatibleReason(context.Background(), &a, req)
			require.True(t, compatible, reason)
		})
	}
}

func TestWebImageSchedulingRechecksRevokedPermissionsFromDatabase(t *testing.T) {
	stale, backup := webImageSchedulingAccount(270), webImageSchedulingAccount(239)
	fresh := webImageSchedulingAccount(270)
	fresh.Extra[AccountExtraChatGPTWebImageEnabled] = false
	var acquired, released []int64
	svc := &OpenAIGatewayService{
		accountRepo: schedulerTestOpenAIAccountRepo{accounts: []Account{fresh, backup}}, cfg: &config.Config{},
		schedulerSnapshot: &SchedulerSnapshotService{cache: &openAISnapshotCacheStub{
			snapshotAccounts: []*Account{&stale, &backup}, accountsByID: map[int64]*Account{stale.ID: &stale, backup.ID: &backup},
		}},
		concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{acquiredIDs: &acquired, releasedIDs: &released}),
	}
	ctx := WithOpenAIImagesEndpoint(context.Background())
	groupID := int64(101)
	// Legacy fresh/DB checks share the same image admission gate.
	require.Nil(t, svc.recheckSelectedOpenAIAccountFromDB(ctx, &stale, &groupID, PlatformOpenAI, "gpt-image-2", false, ""))
	scheduler := &defaultOpenAIAccountScheduler{service: svc}
	selection, _, err := scheduler.tryAcquireOpenAISelectionOrder(ctx, OpenAIAccountScheduleRequest{
		GroupID: &groupID, Platform: PlatformOpenAI, RequestedModel: "gpt-image-2", RequiredImageCapability: OpenAIImagesCapabilityNative,
	}, []openAIAccountCandidateScore{
		{account: &stale, loadInfo: &AccountLoadInfo{AccountID: stale.ID}},
		{account: &backup, loadInfo: &AccountLoadInfo{AccountID: backup.ID}},
	})
	require.NoError(t, err)
	require.Equal(t, backup.ID, selection.Account.ID)
	require.Equal(t, []int64{stale.ID, backup.ID}, acquired)
	require.Equal(t, []int64{stale.ID}, released, "revoked candidate slot must be released before selecting the fallback")
	selection.ReleaseFunc()
}
