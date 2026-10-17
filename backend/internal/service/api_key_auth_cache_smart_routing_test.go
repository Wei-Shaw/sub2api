package service

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAPIKeyAuthSnapshotSmartRoutingRoundtrip(t *testing.T) {
	first, second := int64(101), int64(202)
	apiKey := &APIKey{
		ID:              82,
		UserID:          40,
		Key:             "sk-smart-routing-cache",
		Name:            "smart routing",
		GroupID:         &first,
		RoutingGroupIDs: []int64{first, second},
		Status:          StatusActive,
		User:            &User{ID: 40, Status: StatusActive},
		Group:           &Group{ID: first, Name: "first", Platform: PlatformOpenAI, Status: StatusActive},
	}
	svc := &APIKeyService{}

	payload, err := json.Marshal(&APIKeyAuthCacheEntry{Snapshot: svc.snapshotFromAPIKey(context.Background(), apiKey)})
	require.NoError(t, err)
	var cached APIKeyAuthCacheEntry
	require.NoError(t, json.Unmarshal(payload, &cached))

	materialized, used, err := svc.applyAuthCacheEntry(apiKey.Key, &cached)
	require.NoError(t, err)
	require.True(t, used)
	require.Equal(t, []int64{first, second}, materialized.RoutingGroupIDs)
}

func TestSmartRoutingAccountModelsUsesStrictCatalog(t *testing.T) {
	account := &Account{
		Platform: PlatformOpenAI,
		Credentials: map[string]any{
			"model_mapping": map[string]any{
				"gpt-4o":      "gpt-4o-mini",
				"deepseek-v3": "deepseek-chat",
				"*":           "fallback",
				"":            "invalid",
			},
		},
	}

	require.ElementsMatch(t, []string{"gpt-4o", "deepseek-v3"}, smartRoutingAccountModels(account))
	require.True(t, smartRoutingAccountClaims(account, "gpt-4o"))
	require.False(t, smartRoutingAccountClaims(account, "gpt-5.6"), "wildcard mappings must not turn the catalog into passthrough")
}

func TestSmartRoutingSelectionReleaseIsIdempotent(t *testing.T) {
	released := 0
	var once sync.Once
	selection := &AccountSelectionResult{Account: &Account{ID: 9}, ReleaseFunc: func() {
		once.Do(func() { released++ })
	}}
	ctx := context.WithValue(context.Background(), smartRoutingSelectionContextKey{}, &smartRoutingSelection{
		selection: selection,
		groupID:   1,
		model:     "gpt-4o",
	})

	got := takeSmartRoutingSelection(ctx, func() *int64 { id := int64(1); return &id }(), "gpt-4o")
	require.Same(t, selection, got)
	ReleaseSmartRoutingSelection(ctx)
	ReleaseSmartRoutingSelection(ctx)
	require.Equal(t, 1, released, "middleware and handler cleanup must tolerate double release")
}

type smartRoutingUserRepoForTest struct {
	userRepoStub
	user *User
}

func (s *smartRoutingUserRepoForTest) GetByID(context.Context, int64) (*User, error) {
	return s.user, nil
}

type smartRoutingGroupRepoForTest struct {
	groupRepoStub
	groups map[int64]*Group
}

func (s *smartRoutingGroupRepoForTest) GetByID(_ context.Context, id int64) (*Group, error) {
	group, ok := s.groups[id]
	if !ok {
		return nil, ErrGroupNotFound
	}
	return group, nil
}

func TestSmartRoutingKeysPreservesOrderAndFiltersCurrentPermissions(t *testing.T) {
	first := &Group{ID: 101, Name: "first", Platform: PlatformOpenAI, Status: StatusActive}
	second := &Group{ID: 102, Name: "second", Platform: PlatformGrok, Status: StatusActive}
	disabled := &Group{ID: 103, Name: "disabled", Platform: PlatformOpenAI, Status: "disabled"}
	unsupported := &Group{ID: 104, Name: "unsupported", Platform: PlatformAnthropic, Status: StatusActive}
	forbidden := &Group{ID: 105, Name: "forbidden", Platform: PlatformOpenAI, Status: StatusActive, IsExclusive: true}
	user := &User{
		ID:                   7,
		Status:               StatusActive,
		AllowedGroups:        []int64{first.ID, second.ID},
		RestrictPublicGroups: true,
	}
	svc := &APIKeyService{
		userRepo: &smartRoutingUserRepoForTest{user: user},
		groupRepo: &smartRoutingGroupRepoForTest{groups: map[int64]*Group{
			first.ID: first, second.ID: second, disabled.ID: disabled,
			unsupported.ID: unsupported, forbidden.ID: forbidden,
		}},
	}
	key := &APIKey{UserID: user.ID, RoutingGroupIDs: []int64{
		first.ID, disabled.ID, forbidden.ID, 999, second.ID, unsupported.ID,
	}}

	got, err := svc.SmartRoutingKeys(context.Background(), key)
	require.NoError(t, err)
	require.Len(t, got, 2)
	require.Equal(t, []int64{first.ID, second.ID}, []int64{*got[0].GroupID, *got[1].GroupID})
	require.Same(t, first, got[0].Group)
	require.Same(t, second, got[1].Group)
}

type smartRoutingSchedulerForTest struct {
	selection *AccountSelectionResult
	err       error
	calls     int
	request   OpenAIAccountScheduleRequest
}

func (s *smartRoutingSchedulerForTest) Select(_ context.Context, request OpenAIAccountScheduleRequest) (*AccountSelectionResult, OpenAIAccountScheduleDecision, error) {
	s.calls++
	s.request = request
	if s.err != nil {
		return nil, OpenAIAccountScheduleDecision{}, s.err
	}
	return s.selection, OpenAIAccountScheduleDecision{Layer: "test"}, nil
}

func (*smartRoutingSchedulerForTest) ReportResult(int64, bool, *int) {}
func (*smartRoutingSchedulerForTest) ReportSwitch()                  {}
func (*smartRoutingSchedulerForTest) SnapshotMetrics() OpenAIAccountSchedulerMetricsSnapshot {
	return OpenAIAccountSchedulerMetricsSnapshot{}
}

func enableSmartRoutingSchedulerForTest(t *testing.T) {
	t.Helper()
	resetOpenAIAdvancedSchedulerSettingCacheForTest()
	openAIAdvancedSchedulerSettingCache.Store(&cachedOpenAIAdvancedSchedulerSetting{
		enabled:   true,
		expiresAt: time.Now().Add(time.Minute).UnixNano(),
	})
	t.Cleanup(resetOpenAIAdvancedSchedulerSettingCacheForTest)
}

func TestPrepareSmartRoutingReusesReservationAndReleasesOnce(t *testing.T) {
	enableSmartRoutingSchedulerForTest(t)
	group := &Group{ID: 201, Name: "openai", Platform: PlatformOpenAI, Status: StatusActive, Hydrated: true}
	groupID := group.ID
	released := 0
	scheduler := &smartRoutingSchedulerForTest{selection: &AccountSelectionResult{
		Account:     &Account{ID: 301, Platform: PlatformOpenAI},
		ReleaseFunc: func() { released++ },
	}}
	svc := &OpenAIGatewayService{openaiScheduler: scheduler}
	key := &APIKey{UserID: 7, GroupID: &groupID, Group: group}

	ctx, ok, err := svc.PrepareSmartRouting(context.Background(), key, "gpt-4o", "", OpenAIEndpointCapabilityChatCompletions, false)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, 1, scheduler.calls)
	require.Equal(t, "gpt-4o", scheduler.request.RequestedModel)
	require.Equal(t, OpenAIUpstreamTransportAny, scheduler.request.RequiredTransport)

	selected, decision, err := svc.SelectAccountWithSchedulerForCapability(ctx, &groupID, "", "", "gpt-4o", nil, OpenAIUpstreamTransportAny, OpenAIEndpointCapabilityChatCompletions, false, false, true, PlatformOpenAI)
	require.NoError(t, err)
	require.Same(t, scheduler.selection, selected)
	require.Equal(t, "smart_routing", decision.Layer)
	require.Equal(t, 1, scheduler.calls, "handler must reuse the middleware reservation")

	ReleaseSmartRoutingSelection(ctx)
	ReleaseSmartRoutingSelection(ctx)
	require.Equal(t, 1, released, "middleware and handler cleanup must release the reservation once")
}

func TestPrepareSmartRoutingTreatsNoAvailableAsCleanEarlyReturn(t *testing.T) {
	enableSmartRoutingSchedulerForTest(t)
	group := &Group{ID: 202, Name: "openai", Platform: PlatformOpenAI, Status: StatusActive, Hydrated: true}
	groupID := group.ID
	scheduler := &smartRoutingSchedulerForTest{err: ErrNoAvailableAccounts}
	svc := &OpenAIGatewayService{openaiScheduler: scheduler}
	key := &APIKey{UserID: 7, GroupID: &groupID, Group: group}

	ctx, ok, err := svc.PrepareSmartRouting(context.Background(), key, "gpt-4o", "", OpenAIEndpointCapabilityChatCompletions, false)
	require.NoError(t, err)
	require.False(t, ok)
	require.Equal(t, 1, scheduler.calls)
	ReleaseSmartRoutingSelection(ctx)
}
