package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type openAISnapshotCacheStub struct {
	SchedulerCache
	snapshotAccounts []*Account
	accountsByID     map[int64]*Account
	openAIState      *OpenAISchedulerBucketState
	openAIStateMiss  bool
	getAccountCalls  int
}

func (s *openAISnapshotCacheStub) GetSnapshot(ctx context.Context, bucket SchedulerBucket) ([]*Account, bool, error) {
	if len(s.snapshotAccounts) == 0 {
		if s.openAIState == nil {
			return nil, false, nil
		}
		out := make([]*Account, 0, len(s.openAIState.Accounts))
		for _, account := range s.openAIState.Accounts {
			if account == nil {
				continue
			}
			cloned := *account
			out = append(out, &cloned)
		}
		if len(out) == 0 {
			return nil, false, nil
		}
		return out, true, nil
	}
	out := make([]*Account, 0, len(s.snapshotAccounts))
	for _, account := range s.snapshotAccounts {
		if account == nil {
			continue
		}
		cloned := *account
		out = append(out, &cloned)
	}
	return out, true, nil
}

func (s *openAISnapshotCacheStub) GetOpenAIBucketState(ctx context.Context, bucket SchedulerBucket) (*OpenAISchedulerBucketState, bool, error) {
	if s.openAIStateMiss || s.openAIState == nil {
		return nil, false, nil
	}
	return s.openAIState, true, nil
}

func (s *openAISnapshotCacheStub) GetAccount(ctx context.Context, accountID int64) (*Account, error) {
	s.getAccountCalls++
	if s.accountsByID == nil {
		if s.openAIState == nil {
			return nil, nil
		}
		for _, account := range s.openAIState.Accounts {
			if account != nil && account.ID == accountID {
				cloned := *account
				return &cloned, nil
			}
		}
		return nil, nil
	}
	account := s.accountsByID[accountID]
	if account == nil {
		return nil, nil
	}
	cloned := *account
	return &cloned, nil
}

func requireStructFieldByName(t *testing.T, typ reflect.Type, fieldName string) reflect.StructField {
	t.Helper()
	field, ok := typ.FieldByName(fieldName)
	if !ok {
		t.Fatalf("missing field %s on %s", fieldName, typ.Name())
	}
	return field
}

func requireSetStringField(t *testing.T, target any, fieldName string, value string) {
	t.Helper()
	v := reflect.ValueOf(target)
	if v.Kind() != reflect.Ptr || v.IsNil() {
		t.Fatalf("target for %s must be non-nil pointer", fieldName)
	}
	field := v.Elem().FieldByName(fieldName)
	if !field.IsValid() {
		t.Fatalf("missing field %s", fieldName)
	}
	if !field.CanSet() || field.Kind() != reflect.String {
		t.Fatalf("field %s is not a settable string", fieldName)
	}
	field.SetString(value)
}

func requireSetBoolField(t *testing.T, target any, fieldName string, value bool) {
	t.Helper()
	v := reflect.ValueOf(target)
	if v.Kind() != reflect.Ptr || v.IsNil() {
		t.Fatalf("target for %s must be non-nil pointer", fieldName)
	}
	field := v.Elem().FieldByName(fieldName)
	if !field.IsValid() {
		t.Fatalf("missing field %s", fieldName)
	}
	if !field.CanSet() || field.Kind() != reflect.Bool {
		t.Fatalf("field %s is not a settable bool", fieldName)
	}
	field.SetBool(value)
}

func requireStickyFieldValue(t *testing.T, decision OpenAIAccountScheduleDecision, fieldName string) reflect.Value {
	t.Helper()
	decisionValue := reflect.ValueOf(decision)
	stickyField := decisionValue.FieldByName("Sticky")
	if !stickyField.IsValid() {
		t.Fatalf("missing Sticky field on OpenAIAccountScheduleDecision")
	}
	if stickyField.IsNil() {
		t.Fatalf("decision.Sticky is nil")
	}
	value := stickyField.Elem().FieldByName(fieldName)
	if !value.IsValid() {
		t.Fatalf("missing sticky field %s", fieldName)
	}
	return value
}

func requireStickyStringField(t *testing.T, decision OpenAIAccountScheduleDecision, fieldName string) string {
	t.Helper()
	value := requireStickyFieldValue(t, decision, fieldName)
	if value.Kind() != reflect.String {
		t.Fatalf("sticky field %s is not string", fieldName)
	}
	return value.String()
}

func requireStickyBoolField(t *testing.T, decision OpenAIAccountScheduleDecision, fieldName string) bool {
	t.Helper()
	value := requireStickyFieldValue(t, decision, fieldName)
	if value.Kind() != reflect.Bool {
		t.Fatalf("sticky field %s is not bool", fieldName)
	}
	return value.Bool()
}

func requireStickyBoundAccountID(t *testing.T, decision OpenAIAccountScheduleDecision) *int64 {
	t.Helper()
	value := requireStickyFieldValue(t, decision, "BoundAccountID")
	if value.Kind() != reflect.Ptr {
		t.Fatalf("sticky field BoundAccountID is not pointer")
	}
	if value.IsNil() {
		return nil
	}
	if value.Elem().Kind() != reflect.Int64 {
		t.Fatalf("sticky field BoundAccountID does not point to int64")
	}
	v := value.Elem().Int()
	bound := int64(v)
	return &bound
}

func requireStickyAffinityBindingValue(t *testing.T, decision OpenAIAccountScheduleDecision) reflect.Value {
	t.Helper()
	value := requireStickyFieldValue(t, decision, "AffinityBinding")
	if value.Kind() != reflect.Ptr {
		t.Fatalf("sticky field AffinityBinding is not pointer")
	}
	if value.IsNil() {
		t.Fatalf("sticky field AffinityBinding is nil")
	}
	return value.Elem()
}

func requireStickyAffinityBindingStringField(t *testing.T, decision OpenAIAccountScheduleDecision, fieldName string) string {
	t.Helper()
	value := requireStickyAffinityBindingValue(t, decision).FieldByName(fieldName)
	if !value.IsValid() {
		t.Fatalf("missing affinity binding field %s", fieldName)
	}
	if value.Kind() != reflect.String {
		t.Fatalf("affinity binding field %s is not string", fieldName)
	}
	return value.String()
}

func requireStickyAffinityBindingAccountID(t *testing.T, decision OpenAIAccountScheduleDecision) int64 {
	t.Helper()
	value := requireStickyAffinityBindingValue(t, decision).FieldByName("BoundAccountID")
	if !value.IsValid() {
		t.Fatalf("missing affinity binding field BoundAccountID")
	}
	if value.Kind() != reflect.Int64 {
		t.Fatalf("affinity binding field BoundAccountID is not int64")
	}
	return value.Int()
}

func requireDecisionStringField(t *testing.T, decision OpenAIAccountScheduleDecision, fieldName string) string {
	t.Helper()
	value := reflect.ValueOf(decision).FieldByName(fieldName)
	if !value.IsValid() {
		t.Fatalf("missing decision field %s", fieldName)
	}
	if value.Kind() != reflect.String {
		t.Fatalf("decision field %s is not string", fieldName)
	}
	return value.String()
}

func TestOpenAIAccountScheduleContracts_StickyObservability(t *testing.T) {
	requestType := reflect.TypeOf(OpenAIAccountScheduleRequest{})
	requireStructFieldByName(t, requestType, "SessionSource")
	requireStructFieldByName(t, requestType, "ParentSessionPresent")
	requireStructFieldByName(t, requestType, "ParentSessionKey")

	decisionType := reflect.TypeOf(OpenAIAccountScheduleDecision{})
	stickyField := requireStructFieldByName(t, decisionType, "Sticky")
	requireStructFieldByName(t, decisionType, "SelectedGroup")
	require.Equal(t, reflect.Ptr, stickyField.Type.Kind())
	stickyType := stickyField.Type.Elem()
	requireStructFieldByName(t, stickyType, "SessionSource")
	requireStructFieldByName(t, stickyType, "SessionHashPresent")
	requireStructFieldByName(t, stickyType, "EvalResult")
	requireStructFieldByName(t, stickyType, "SelectedAccountChanged")
	requireStructFieldByName(t, stickyType, "ParentSessionPresent")
	requireStructFieldByName(t, stickyType, "ParentSessionKey")
	requireStructFieldByName(t, stickyType, "BoundAccountID")
	affinityBindingField := requireStructFieldByName(t, stickyType, "AffinityBinding")
	require.Equal(t, reflect.Ptr, affinityBindingField.Type.Kind())
	affinityBindingType := affinityBindingField.Type.Elem()
	requireStructFieldByName(t, affinityBindingType, "BoundAccountID")
	requireStructFieldByName(t, affinityBindingType, "AffinityDomain")
	requireStructFieldByName(t, affinityBindingType, "SelectedGroup")
}

func TestDefaultOpenAIAccountScheduler_Select_StickyHitObservability(t *testing.T) {
	ctx := context.Background()
	groupID := int64(401)
	account := Account{
		ID:          54001,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
	}
	service := &OpenAIGatewayService{
		accountRepo:        stubOpenAIAccountRepo{accounts: []Account{account}},
		cache:              &stubGatewayCache{sessionBindings: map[string]int64{"openai:sticky_hit_observed": account.ID}},
		cfg:                &config.Config{},
		concurrencyService: NewConcurrencyService(stubConcurrencyCache{}),
	}
	schedulerAny := newDefaultOpenAIAccountScheduler(service, nil)
	scheduler, ok := schedulerAny.(*defaultOpenAIAccountScheduler)
	require.True(t, ok)

	req := OpenAIAccountScheduleRequest{
		GroupID:        &groupID,
		SessionHash:    "sticky_hit_observed",
		RequestedModel: "gpt-5.1",
	}
	requireSetStringField(t, &req, "SessionSource", "header_x_session_affinity")

	selection, decision, err := scheduler.Select(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	require.Equal(t, account.ID, selection.Account.ID)
	require.Equal(t, openAIAccountScheduleLayerSessionSticky, decision.Layer)
	require.True(t, decision.StickySessionHit)
	require.Equal(t, "header_x_session_affinity", requireStickyStringField(t, decision, "SessionSource"))
	require.True(t, requireStickyBoolField(t, decision, "SessionHashPresent"))
	require.Equal(t, "hit", requireStickyStringField(t, decision, "EvalResult"))
	require.False(t, requireStickyBoolField(t, decision, "SelectedAccountChanged"))
	require.False(t, requireStickyBoolField(t, decision, "ParentSessionPresent"))
	require.Empty(t, requireStickyStringField(t, decision, "ParentSessionKey"))
	boundAccountID := requireStickyBoundAccountID(t, decision)
	require.NotNil(t, boundAccountID)
	require.Equal(t, account.ID, *boundAccountID)
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}

func TestDefaultOpenAIAccountScheduler_Select_StickyNoBindingObservability(t *testing.T) {
	ctx := context.Background()
	groupID := int64(402)
	account := Account{
		ID:          54002,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
	}
	service := &OpenAIGatewayService{
		accountRepo:        stubOpenAIAccountRepo{accounts: []Account{account}},
		cache:              &stubGatewayCache{sessionBindings: map[string]int64{}},
		cfg:                &config.Config{},
		concurrencyService: NewConcurrencyService(stubConcurrencyCache{}),
	}
	schedulerAny := newDefaultOpenAIAccountScheduler(service, nil)
	scheduler, ok := schedulerAny.(*defaultOpenAIAccountScheduler)
	require.True(t, ok)

	req := OpenAIAccountScheduleRequest{
		GroupID:        &groupID,
		SessionHash:    "sticky_no_binding_observed",
		RequestedModel: "gpt-5.1",
	}
	requireSetStringField(t, &req, "SessionSource", "prompt_cache_key")

	selection, decision, err := scheduler.Select(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	require.Equal(t, account.ID, selection.Account.ID)
	require.Equal(t, openAIAccountScheduleLayerLoadBalance, decision.Layer)
	require.Equal(t, "prompt_cache_key", requireStickyStringField(t, decision, "SessionSource"))
	require.True(t, requireStickyBoolField(t, decision, "SessionHashPresent"))
	require.Equal(t, "miss_no_binding", requireStickyStringField(t, decision, "EvalResult"))
	require.False(t, requireStickyBoolField(t, decision, "SelectedAccountChanged"))
	require.Nil(t, requireStickyBoundAccountID(t, decision))
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}

func TestDefaultOpenAIAccountScheduler_Select_StickyInvalidBindingObservability(t *testing.T) {
	ctx := context.Background()
	groupID := int64(404)
	fallback := Account{
		ID:          54006,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
	}
	service := &OpenAIGatewayService{
		accountRepo:        stubOpenAIAccountRepo{accounts: []Account{fallback}},
		cache:              &stubGatewayCache{sessionBindings: map[string]int64{"openai:sticky_invalid_observed": 54005}},
		cfg:                &config.Config{},
		concurrencyService: NewConcurrencyService(stubConcurrencyCache{}),
	}
	schedulerAny := newDefaultOpenAIAccountScheduler(service, nil)
	scheduler, ok := schedulerAny.(*defaultOpenAIAccountScheduler)
	require.True(t, ok)

	req := OpenAIAccountScheduleRequest{
		GroupID:        &groupID,
		SessionHash:    "sticky_invalid_observed",
		SessionSource:  "header_x_session_affinity",
		RequestedModel: "gpt-5.1",
	}

	selection, decision, err := scheduler.Select(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	require.Equal(t, fallback.ID, selection.Account.ID)
	require.Equal(t, openAIAccountScheduleLayerLoadBalance, decision.Layer)
	require.Equal(t, openAIStickyEvalResultMissBindingInvalid, requireStickyStringField(t, decision, "EvalResult"))
	require.True(t, requireStickyBoolField(t, decision, "SelectedAccountChanged"))
	boundAccountID := requireStickyBoundAccountID(t, decision)
	require.NotNil(t, boundAccountID)
	require.Equal(t, int64(54005), *boundAccountID)
	require.Equal(t, 1, service.cache.(*stubGatewayCache).deletedSessions["openai:sticky_invalid_observed"])
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}

func TestDefaultOpenAIAccountScheduler_Select_StickyRestrictedBindingObservability(t *testing.T) {
	ctx := context.Background()
	groupID := int64(405)
	stickyAccount := Account{
		ID:          54007,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Extra: map[string]any{
			"quota_limit": float64(100),
			"quota_used":  float64(10),
		},
	}
	fallback := Account{
		ID:          54008,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Extra: map[string]any{
			"quota_limit": float64(100),
			"quota_used":  float64(100),
		},
	}
	cache := &stubGatewayCache{sessionBindings: map[string]int64{"openai:sticky_restricted_observed": stickyAccount.ID}}
	service := &OpenAIGatewayService{
		accountRepo:        stubOpenAIAccountRepo{accounts: []Account{stickyAccount, fallback}},
		cache:              cache,
		cfg:                &config.Config{},
		concurrencyService: NewConcurrencyService(stubConcurrencyCache{}),
	}
	schedulerAny := newDefaultOpenAIAccountScheduler(service, nil)
	scheduler, ok := schedulerAny.(*defaultOpenAIAccountScheduler)
	require.True(t, ok)

	req := OpenAIAccountScheduleRequest{
		GroupID:        &groupID,
		SessionHash:    "sticky_restricted_observed",
		SessionSource:  "header_session_id",
		RequestedModel: "gpt-5.1",
		TargetGroup:    TargetGroupExhausted,
	}

	selection, decision, err := scheduler.Select(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	require.Equal(t, fallback.ID, selection.Account.ID)
	require.Equal(t, openAIAccountScheduleLayerLoadBalance, decision.Layer)
	require.Equal(t, openAIStickyEvalResultMissBindingRestricted, requireStickyStringField(t, decision, "EvalResult"))
	require.True(t, requireStickyBoolField(t, decision, "SelectedAccountChanged"))
	boundAccountID := requireStickyBoundAccountID(t, decision)
	require.NotNil(t, boundAccountID)
	require.Equal(t, stickyAccount.ID, *boundAccountID)
	require.Zero(t, cache.deletedSessions["openai:sticky_restricted_observed"])
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}

func TestDefaultOpenAIAccountScheduler_Select_StickyExcludedBindingObservability(t *testing.T) {
	ctx := context.Background()
	groupID := int64(406)
	stickyAccount := Account{
		ID:          54009,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
	}
	fallback := Account{
		ID:          54010,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
	}
	service := &OpenAIGatewayService{
		accountRepo:        stubOpenAIAccountRepo{accounts: []Account{stickyAccount, fallback}},
		cache:              &stubGatewayCache{sessionBindings: map[string]int64{"openai:sticky_excluded_observed": stickyAccount.ID}},
		cfg:                &config.Config{},
		concurrencyService: NewConcurrencyService(stubConcurrencyCache{}),
	}
	schedulerAny := newDefaultOpenAIAccountScheduler(service, nil)
	scheduler, ok := schedulerAny.(*defaultOpenAIAccountScheduler)
	require.True(t, ok)

	req := OpenAIAccountScheduleRequest{
		GroupID:        &groupID,
		SessionHash:    "sticky_excluded_observed",
		SessionSource:  "prompt_cache_key",
		RequestedModel: "gpt-5.1",
		ExcludedIDs:    map[int64]struct{}{stickyAccount.ID: {}},
	}

	selection, decision, err := scheduler.Select(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	require.Equal(t, fallback.ID, selection.Account.ID)
	require.Equal(t, openAIAccountScheduleLayerLoadBalance, decision.Layer)
	require.Equal(t, openAIStickyEvalResultMissBindingExcluded, requireStickyStringField(t, decision, "EvalResult"))
	require.True(t, requireStickyBoolField(t, decision, "SelectedAccountChanged"))
	boundAccountID := requireStickyBoundAccountID(t, decision)
	require.NotNil(t, boundAccountID)
	require.Equal(t, stickyAccount.ID, *boundAccountID)
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}

func TestDefaultOpenAIAccountScheduler_Select_StickyNoSessionSignalObservability(t *testing.T) {
	ctx := context.Background()
	groupID := int64(407)
	account := Account{
		ID:          54011,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
	}
	service := &OpenAIGatewayService{
		accountRepo:        stubOpenAIAccountRepo{accounts: []Account{account}},
		cache:              &stubGatewayCache{sessionBindings: map[string]int64{}},
		cfg:                &config.Config{},
		concurrencyService: NewConcurrencyService(stubConcurrencyCache{}),
	}
	schedulerAny := newDefaultOpenAIAccountScheduler(service, nil)
	scheduler, ok := schedulerAny.(*defaultOpenAIAccountScheduler)
	require.True(t, ok)

	req := OpenAIAccountScheduleRequest{
		GroupID:        &groupID,
		SessionHash:    "pool_mode_internal_hash",
		RequestedModel: "gpt-5.1",
	}

	selection, decision, err := scheduler.Select(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	require.Equal(t, account.ID, selection.Account.ID)
	require.Equal(t, openAIAccountScheduleLayerLoadBalance, decision.Layer)
	require.Equal(t, openAIStickySessionSourceNone, requireStickyStringField(t, decision, "SessionSource"))
	require.True(t, requireStickyBoolField(t, decision, "SessionHashPresent"))
	require.Equal(t, openAIStickyEvalResultNoSessionSignal, requireStickyStringField(t, decision, "EvalResult"))
	require.False(t, requireStickyBoolField(t, decision, "SelectedAccountChanged"))
	require.Nil(t, requireStickyBoundAccountID(t, decision))
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}

func TestDefaultOpenAIAccountScheduler_Select_StickyHitOverridesNoSessionSignal(t *testing.T) {
	ctx := context.Background()
	groupID := int64(408)
	account := Account{
		ID:          54012,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
	}
	service := &OpenAIGatewayService{
		accountRepo:        stubOpenAIAccountRepo{accounts: []Account{account}},
		cache:              &stubGatewayCache{sessionBindings: map[string]int64{"openai:sticky_no_signal_hit": account.ID}},
		cfg:                &config.Config{},
		concurrencyService: NewConcurrencyService(stubConcurrencyCache{}),
	}
	schedulerAny := newDefaultOpenAIAccountScheduler(service, nil)
	scheduler, ok := schedulerAny.(*defaultOpenAIAccountScheduler)
	require.True(t, ok)

	req := OpenAIAccountScheduleRequest{
		GroupID:        &groupID,
		SessionHash:    "sticky_no_signal_hit",
		RequestedModel: "gpt-5.1",
	}

	selection, decision, err := scheduler.Select(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	require.Equal(t, account.ID, selection.Account.ID)
	require.Equal(t, openAIAccountScheduleLayerSessionSticky, decision.Layer)
	require.True(t, decision.StickySessionHit)
	require.Equal(t, openAIStickySessionSourceNone, requireStickyStringField(t, decision, "SessionSource"))
	require.True(t, requireStickyBoolField(t, decision, "SessionHashPresent"))
	require.Equal(t, openAIStickyEvalResultHit, requireStickyStringField(t, decision, "EvalResult"))
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}

func TestDefaultOpenAIAccountScheduler_Select_PreviousResponseOverridesNoSessionSignal(t *testing.T) {
	ctx := context.Background()
	groupID := int64(409)
	account := Account{
		ID:          54013,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Extra: map[string]any{
			"openai_apikey_responses_websockets_v2_enabled": true,
		},
	}
	service := &OpenAIGatewayService{
		accountRepo:        stubOpenAIAccountRepo{accounts: []Account{account}},
		cache:              &stubGatewayCache{sessionBindings: map[string]int64{}},
		cfg:                newOpenAIWSV2TestConfig(),
		concurrencyService: NewConcurrencyService(stubConcurrencyCache{}),
	}
	store := service.getOpenAIWSStateStore()
	require.NoError(t, store.BindResponseAccount(ctx, groupID, "resp_no_signal_prev", account.ID, time.Hour))
	schedulerAny := newDefaultOpenAIAccountScheduler(service, nil)
	scheduler, ok := schedulerAny.(*defaultOpenAIAccountScheduler)
	require.True(t, ok)

	req := OpenAIAccountScheduleRequest{
		GroupID:            &groupID,
		SessionHash:        "pool_mode_internal_hash",
		PreviousResponseID: "resp_no_signal_prev",
		RequestedModel:     "gpt-5.1",
	}
	requireSetBoolField(t, &req, "ParentSessionPresent", true)
	requireSetStringField(t, &req, "ParentSessionKey", "resp_no_signal_prev")

	selection, decision, err := scheduler.Select(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	require.Equal(t, account.ID, selection.Account.ID)
	require.Equal(t, openAIAccountScheduleLayerPreviousResponse, decision.Layer)
	require.True(t, decision.StickyPreviousHit)
	require.Equal(t, openAIStickySessionSourceNone, requireStickyStringField(t, decision, "SessionSource"))
	require.True(t, requireStickyBoolField(t, decision, "SessionHashPresent"))
	require.Equal(t, openAIStickyEvalResultBypassedPreviousResponse, requireStickyStringField(t, decision, "EvalResult"))
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}

func TestDefaultOpenAIAccountScheduler_Select_PreviousResponseBypassesStickyObservability(t *testing.T) {
	ctx := context.Background()
	groupID := int64(403)
	previousAccount := Account{
		ID:          54003,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Extra: map[string]any{
			"openai_apikey_responses_websockets_v2_enabled": true,
		},
	}
	stickyAccount := Account{
		ID:          54004,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Extra: map[string]any{
			"openai_apikey_responses_websockets_v2_enabled": true,
		},
	}
	cache := &stubGatewayCache{sessionBindings: map[string]int64{"openai:sticky_prev_bypass_observed": stickyAccount.ID}}
	cfg := newOpenAIWSV2TestConfig()
	service := &OpenAIGatewayService{
		accountRepo:        stubOpenAIAccountRepo{accounts: []Account{previousAccount, stickyAccount}},
		cache:              cache,
		cfg:                cfg,
		concurrencyService: NewConcurrencyService(stubConcurrencyCache{}),
	}
	store := service.getOpenAIWSStateStore()
	require.NoError(t, store.BindResponseAccount(ctx, groupID, "resp_sticky_observed", previousAccount.ID, time.Hour))
	schedulerAny := newDefaultOpenAIAccountScheduler(service, nil)
	scheduler, ok := schedulerAny.(*defaultOpenAIAccountScheduler)
	require.True(t, ok)

	req := OpenAIAccountScheduleRequest{
		GroupID:            &groupID,
		SessionHash:        "sticky_prev_bypass_observed",
		PreviousResponseID: "resp_sticky_observed",
		RequestedModel:     "gpt-5.1",
	}
	requireSetStringField(t, &req, "SessionSource", "header_session_id")
	requireSetBoolField(t, &req, "ParentSessionPresent", true)
	requireSetStringField(t, &req, "ParentSessionKey", "resp_sticky_observed")

	selection, decision, err := scheduler.Select(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	require.Equal(t, previousAccount.ID, selection.Account.ID)
	require.Equal(t, openAIAccountScheduleLayerPreviousResponse, decision.Layer)
	require.True(t, decision.StickyPreviousHit)
	require.Equal(t, "header_session_id", requireStickyStringField(t, decision, "SessionSource"))
	require.True(t, requireStickyBoolField(t, decision, "SessionHashPresent"))
	require.Equal(t, "bypassed_previous_response_id", requireStickyStringField(t, decision, "EvalResult"))
	require.True(t, requireStickyBoolField(t, decision, "SelectedAccountChanged"))
	require.True(t, requireStickyBoolField(t, decision, "ParentSessionPresent"))
	require.Equal(t, "resp_sticky_observed", requireStickyStringField(t, decision, "ParentSessionKey"))
	boundAccountID := requireStickyBoundAccountID(t, decision)
	require.NotNil(t, boundAccountID)
	require.Equal(t, stickyAccount.ID, *boundAccountID)
	require.Equal(t, previousAccount.ID, cache.sessionBindings["openai:sticky_prev_bypass_observed"])
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}

func TestOpenAIGatewayService_SelectAccountWithScheduler_SessionStickyRateLimitedAccountFallsBackToFreshCandidate(t *testing.T) {
	ctx := context.Background()
	groupID := int64(10101)
	rateLimitedUntil := time.Now().Add(30 * time.Minute)
	staleSticky := &Account{ID: 31001, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 0}
	staleBackup := &Account{ID: 31002, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 5}
	freshSticky := &Account{ID: 31001, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 0, RateLimitResetAt: &rateLimitedUntil}
	freshBackup := &Account{ID: 31002, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 5}
	cache := &stubGatewayCache{sessionBindings: map[string]int64{"openai:session_hash_rate_limited": 31001}}
	snapshotCache := &openAISnapshotCacheStub{snapshotAccounts: []*Account{staleSticky, staleBackup}, accountsByID: map[int64]*Account{31001: freshSticky, 31002: freshBackup}}
	snapshotService := &SchedulerSnapshotService{cache: snapshotCache}
	svc := &OpenAIGatewayService{accountRepo: stubOpenAIAccountRepo{accounts: []Account{*freshSticky, *freshBackup}}, cache: cache, cfg: &config.Config{}, schedulerSnapshot: snapshotService, concurrencyService: NewConcurrencyService(stubConcurrencyCache{})}

	selection, decision, err := svc.SelectAccountWithScheduler(ctx, &groupID, "", "session_hash_rate_limited", "gpt-5.1", TargetGroupAny, nil, OpenAIUpstreamTransportAny)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	require.Equal(t, int64(31002), selection.Account.ID)
	require.Equal(t, openAIAccountScheduleLayerLoadBalance, decision.Layer)
}

func TestOpenAIGatewayService_SelectAccountForModelWithExclusions_SkipsFreshlyRateLimitedSnapshotCandidate(t *testing.T) {
	ctx := context.Background()
	groupID := int64(10102)
	rateLimitedUntil := time.Now().Add(30 * time.Minute)
	stalePrimary := &Account{ID: 32001, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 0}
	staleSecondary := &Account{ID: 32002, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 5}
	freshPrimary := &Account{ID: 32001, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 0, RateLimitResetAt: &rateLimitedUntil}
	freshSecondary := &Account{ID: 32002, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 5}
	snapshotCache := &openAISnapshotCacheStub{snapshotAccounts: []*Account{stalePrimary, staleSecondary}, accountsByID: map[int64]*Account{32001: freshPrimary, 32002: freshSecondary}}
	snapshotService := &SchedulerSnapshotService{cache: snapshotCache}
	svc := &OpenAIGatewayService{accountRepo: stubOpenAIAccountRepo{accounts: []Account{*freshPrimary, *freshSecondary}}, cfg: &config.Config{}, schedulerSnapshot: snapshotService}

	account, err := svc.SelectAccountForModelWithExclusions(ctx, &groupID, "", "gpt-5.1", nil)
	require.NoError(t, err)
	require.NotNil(t, account)
	require.Equal(t, int64(32002), account.ID)
}

func TestOpenAIGatewayService_SelectAccountWithScheduler_SessionStickyDBRuntimeRecheckSkipsStaleCachedAccount(t *testing.T) {
	ctx := context.Background()
	groupID := int64(10103)
	rateLimitedUntil := time.Now().Add(30 * time.Minute)
	staleSticky := &Account{ID: 33001, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 0}
	staleBackup := &Account{ID: 33002, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 5}
	dbSticky := Account{ID: 33001, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 0, RateLimitResetAt: &rateLimitedUntil}
	dbBackup := Account{ID: 33002, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 5}
	cache := &stubGatewayCache{sessionBindings: map[string]int64{"openai:session_hash_db_runtime_recheck": 33001}}
	snapshotCache := &openAISnapshotCacheStub{
		snapshotAccounts: []*Account{staleSticky, staleBackup},
		accountsByID:     map[int64]*Account{33001: staleSticky, 33002: staleBackup},
	}
	snapshotService := &SchedulerSnapshotService{cache: snapshotCache}
	svc := &OpenAIGatewayService{
		accountRepo:        stubOpenAIAccountRepo{accounts: []Account{dbSticky, dbBackup}},
		cache:              cache,
		cfg:                &config.Config{},
		schedulerSnapshot:  snapshotService,
		concurrencyService: NewConcurrencyService(stubConcurrencyCache{}),
	}

	selection, decision, err := svc.SelectAccountWithScheduler(ctx, &groupID, "", "session_hash_db_runtime_recheck", "gpt-5.1", TargetGroupAny, nil, OpenAIUpstreamTransportAny)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	require.Equal(t, int64(33002), selection.Account.ID)
	require.Equal(t, openAIAccountScheduleLayerLoadBalance, decision.Layer)
}

func TestOpenAIGatewayService_SelectAccountWithScheduler_SessionStickyTargetGroupMismatchKeepsBinding(t *testing.T) {
	ctx := context.Background()
	groupID := int64(20201)
	stickyAccount := Account{
		ID:          41001,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Extra: map[string]any{
			"quota_limit": float64(100),
			"quota_used":  float64(10),
		},
	}
	cache := &stubGatewayCache{sessionBindings: map[string]int64{"openai:session_hash_target_group_mismatch": stickyAccount.ID}}
	svc := &OpenAIGatewayService{
		accountRepo:        stubOpenAIAccountRepo{accounts: []Account{stickyAccount}},
		cache:              cache,
		cfg:                &config.Config{},
		concurrencyService: NewConcurrencyService(stubConcurrencyCache{}),
	}

	selection, _, err := svc.SelectAccountWithScheduler(
		ctx,
		&groupID,
		"",
		"session_hash_target_group_mismatch",
		"gpt-5.1",
		TargetGroupExhausted,
		nil,
		OpenAIUpstreamTransportAny,
	)
	require.Error(t, err)
	require.Nil(t, selection)
	require.Equal(t, stickyAccount.ID, cache.sessionBindings["openai:session_hash_target_group_mismatch"])
	require.Zero(t, cache.deletedSessions["openai:session_hash_target_group_mismatch"])
}

func TestOpenAIGatewayService_SelectAccountWithScheduler_ExhaustedRateLimitedStickyKeepsBinding(t *testing.T) {
	ctx := context.Background()
	groupID := int64(20202)
	rateLimitedUntil := time.Now().Add(30 * time.Minute)
	stickyExhausted := Account{
		ID:               42001,
		Platform:         PlatformOpenAI,
		Type:             AccountTypeAPIKey,
		Status:           StatusActive,
		Schedulable:      true,
		Concurrency:      1,
		RateLimitResetAt: &rateLimitedUntil,
		Extra: map[string]any{
			"quota_limit": float64(100),
			"quota_used":  float64(100),
		},
	}
	cache := &stubGatewayCache{sessionBindings: map[string]int64{"openai:session_hash_exhausted_sticky": stickyExhausted.ID}}
	svc := &OpenAIGatewayService{
		accountRepo:        stubOpenAIAccountRepo{accounts: []Account{stickyExhausted}},
		cache:              cache,
		cfg:                &config.Config{},
		concurrencyService: NewConcurrencyService(stubConcurrencyCache{}),
	}

	selection, decision, err := svc.SelectAccountWithScheduler(
		ctx,
		&groupID,
		"",
		"session_hash_exhausted_sticky",
		"gpt-5.1",
		TargetGroupExhausted,
		nil,
		OpenAIUpstreamTransportAny,
	)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	require.Equal(t, stickyExhausted.ID, selection.Account.ID)
	require.Equal(t, openAIAccountScheduleLayerSessionSticky, decision.Layer)
	require.Equal(t, stickyExhausted.ID, cache.sessionBindings["openai:session_hash_exhausted_sticky"])
	require.Zero(t, cache.deletedSessions["openai:session_hash_exhausted_sticky"])
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}

func TestOpenAIGatewayService_SelectAccountWithScheduler_SessionStickyUnschedulableClearsBinding(t *testing.T) {
	ctx := context.Background()
	groupID := int64(20203)
	unschedSticky := Account{ID: 43001, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusDisabled, Schedulable: true, Concurrency: 1}
	fallback := Account{ID: 43002, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1}
	cache := &stubGatewayCache{sessionBindings: map[string]int64{"openai:session_hash_unsched_sticky": unschedSticky.ID}}
	svc := &OpenAIGatewayService{
		accountRepo:        stubOpenAIAccountRepo{accounts: []Account{unschedSticky, fallback}},
		cache:              cache,
		cfg:                &config.Config{},
		concurrencyService: NewConcurrencyService(stubConcurrencyCache{}),
	}

	selection, decision, err := svc.SelectAccountWithScheduler(
		ctx,
		&groupID,
		"",
		"session_hash_unsched_sticky",
		"gpt-5.1",
		TargetGroupAny,
		nil,
		OpenAIUpstreamTransportAny,
	)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	require.Equal(t, fallback.ID, selection.Account.ID)
	require.Equal(t, openAIAccountScheduleLayerLoadBalance, decision.Layer)
	require.Equal(t, 1, cache.deletedSessions["openai:session_hash_unsched_sticky"])
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}

func TestOpenAIGatewayService_SelectAccountWithScheduler_LoadBalanceAllowsRateLimitedExhaustedAccount(t *testing.T) {
	ctx := context.Background()
	groupID := int64(20204)
	rateLimitedUntil := time.Now().Add(30 * time.Minute)
	exhausted := Account{
		ID:               44001,
		Platform:         PlatformOpenAI,
		Type:             AccountTypeAPIKey,
		Status:           StatusActive,
		Schedulable:      true,
		Concurrency:      1,
		Priority:         0,
		RateLimitResetAt: &rateLimitedUntil,
		Extra: map[string]any{
			"quota_limit": float64(100),
			"quota_used":  float64(100),
		},
	}
	active := Account{
		ID:          44002,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Priority:    10,
		Extra: map[string]any{
			"quota_limit": float64(100),
			"quota_used":  float64(10),
		},
	}
	svc := &OpenAIGatewayService{
		accountRepo:        stubOpenAIAccountRepo{accounts: []Account{exhausted, active}},
		cache:              &stubGatewayCache{sessionBindings: map[string]int64{}},
		cfg:                &config.Config{},
		concurrencyService: NewConcurrencyService(stubConcurrencyCache{}),
	}

	selection, decision, err := svc.SelectAccountWithScheduler(
		ctx,
		&groupID,
		"",
		"",
		"gpt-5.1",
		TargetGroupExhausted,
		nil,
		OpenAIUpstreamTransportAny,
	)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	require.Equal(t, exhausted.ID, selection.Account.ID)
	require.Equal(t, openAIAccountScheduleLayerLoadBalance, decision.Layer)
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}

func TestOpenAIGatewayService_SelectAccountWithScheduler_LoadBalanceWaitPlanSkipsDBRecheckedStaleCandidate(t *testing.T) {
	ctx := context.Background()
	groupID := int64(20205)
	rateLimitedUntil := time.Now().Add(30 * time.Minute)
	stalePrimary := &Account{ID: 45001, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 0}
	staleSecondary := &Account{ID: 45002, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 9}
	dbPrimary := Account{ID: 45001, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 0, RateLimitResetAt: &rateLimitedUntil}
	dbSecondary := Account{ID: 45002, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 9}

	snapshotCache := &openAISnapshotCacheStub{
		snapshotAccounts: []*Account{stalePrimary, staleSecondary},
		accountsByID: map[int64]*Account{
			45001: stalePrimary,
			45002: staleSecondary,
		},
	}
	svc := &OpenAIGatewayService{
		accountRepo:        stubOpenAIAccountRepo{accounts: []Account{dbPrimary, dbSecondary}},
		cache:              &stubGatewayCache{sessionBindings: map[string]int64{}},
		cfg:                &config.Config{},
		schedulerSnapshot:  &SchedulerSnapshotService{cache: snapshotCache},
		concurrencyService: NewConcurrencyService(stubConcurrencyCache{acquireResults: map[int64]bool{45001: false, 45002: false}}),
	}

	selection, decision, err := svc.SelectAccountWithScheduler(
		ctx,
		&groupID,
		"",
		"session_hash_waitplan_db_recheck",
		"gpt-5.1",
		TargetGroupAny,
		nil,
		OpenAIUpstreamTransportAny,
	)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	require.Equal(t, int64(45002), selection.Account.ID)
	require.NotNil(t, selection.WaitPlan)
	require.Equal(t, int64(45002), selection.WaitPlan.AccountID)
	require.Equal(t, openAIAccountScheduleLayerLoadBalance, decision.Layer)
}

func TestOpenAIGatewayService_SelectAccountForModelWithExclusions_DBRuntimeRecheckSkipsStaleCachedCandidate(t *testing.T) {
	ctx := context.Background()
	groupID := int64(10104)
	rateLimitedUntil := time.Now().Add(30 * time.Minute)
	stalePrimary := &Account{ID: 34001, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 0}
	staleSecondary := &Account{ID: 34002, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 5}
	dbPrimary := Account{ID: 34001, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 0, RateLimitResetAt: &rateLimitedUntil}
	dbSecondary := Account{ID: 34002, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 5}
	snapshotCache := &openAISnapshotCacheStub{
		snapshotAccounts: []*Account{stalePrimary, staleSecondary},
		accountsByID:     map[int64]*Account{34001: stalePrimary, 34002: staleSecondary},
	}
	snapshotService := &SchedulerSnapshotService{cache: snapshotCache}
	svc := &OpenAIGatewayService{
		accountRepo:       stubOpenAIAccountRepo{accounts: []Account{dbPrimary, dbSecondary}},
		cfg:               &config.Config{},
		schedulerSnapshot: snapshotService,
	}

	account, err := svc.SelectAccountForModelWithExclusions(ctx, &groupID, "", "gpt-5.1", nil)
	require.NoError(t, err)
	require.NotNil(t, account)
	require.Equal(t, int64(34002), account.ID)
}

func TestOpenAIGatewayService_SelectAccountWithScheduler_PreviousResponseSticky(t *testing.T) {
	ctx := context.Background()
	groupID := int64(9)
	account := Account{
		ID:          1001,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 2,
		Extra: map[string]any{
			"openai_apikey_responses_websockets_v2_enabled": true,
		},
	}
	cache := &stubGatewayCache{}
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.OAuthEnabled = true
	cfg.Gateway.OpenAIWS.APIKeyEnabled = true
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
	cfg.Gateway.OpenAIWS.StickySessionTTLSeconds = 1800
	cfg.Gateway.OpenAIWS.StickyResponseIDTTLSeconds = 3600

	svc := &OpenAIGatewayService{
		accountRepo:        stubOpenAIAccountRepo{accounts: []Account{account}},
		cache:              cache,
		cfg:                cfg,
		concurrencyService: NewConcurrencyService(stubConcurrencyCache{}),
	}

	store := svc.getOpenAIWSStateStore()
	require.NoError(t, store.BindResponseAccount(ctx, groupID, "resp_prev_001", account.ID, time.Hour))

	selection, decision, err := svc.SelectAccountWithScheduler(
		ctx,
		&groupID,
		"resp_prev_001",
		"session_hash_001",
		"gpt-5.1",
		TargetGroupAny,
		nil,
		OpenAIUpstreamTransportAny,
	)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	require.Equal(t, account.ID, selection.Account.ID)
	require.Equal(t, openAIAccountScheduleLayerPreviousResponse, decision.Layer)
	require.True(t, decision.StickyPreviousHit)
	require.Equal(t, account.ID, cache.sessionBindings["openai:session_hash_001"])
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}

func TestOpenAIGatewayService_SelectAccountWithScheduler_SessionSticky(t *testing.T) {
	ctx := context.Background()
	groupID := int64(10)
	account := Account{
		ID:          2001,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
	}
	cache := &stubGatewayCache{
		sessionBindings: map[string]int64{
			"openai:session_hash_abc": account.ID,
		},
	}

	svc := &OpenAIGatewayService{
		accountRepo:        stubOpenAIAccountRepo{accounts: []Account{account}},
		cache:              cache,
		cfg:                &config.Config{},
		concurrencyService: NewConcurrencyService(stubConcurrencyCache{}),
	}

	selection, decision, err := svc.SelectAccountWithScheduler(
		ctx,
		&groupID,
		"",
		"session_hash_abc",
		"gpt-5.1",
		TargetGroupAny,
		nil,
		OpenAIUpstreamTransportAny,
	)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	require.Equal(t, account.ID, selection.Account.ID)
	require.Equal(t, openAIAccountScheduleLayerSessionSticky, decision.Layer)
	require.True(t, decision.StickySessionHit)
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}

func TestOpenAIGatewayService_SelectAccountWithScheduler_SessionStickyBusyKeepsSticky(t *testing.T) {
	ctx := context.Background()
	groupID := int64(10100)
	accounts := []Account{
		{
			ID:          21001,
			Platform:    PlatformOpenAI,
			Type:        AccountTypeAPIKey,
			Status:      StatusActive,
			Schedulable: true,
			Concurrency: 1,
			Priority:    0,
		},
		{
			ID:          21002,
			Platform:    PlatformOpenAI,
			Type:        AccountTypeAPIKey,
			Status:      StatusActive,
			Schedulable: true,
			Concurrency: 1,
			Priority:    9,
		},
	}
	cache := &stubGatewayCache{
		sessionBindings: map[string]int64{
			"openai:session_hash_sticky_busy": 21001,
		},
	}
	cfg := &config.Config{}
	cfg.Gateway.Scheduling.StickySessionMaxWaiting = 2
	cfg.Gateway.Scheduling.StickySessionWaitTimeout = 45 * time.Second
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.APIKeyEnabled = true
	cfg.Gateway.OpenAIWS.OAuthEnabled = true
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true

	concurrencyCache := stubConcurrencyCache{
		acquireResults: map[int64]bool{
			21001: false, // sticky 账号已满
			21002: true,  // 若回退负载均衡会命中该账号（本测试要求不能切换）
		},
		waitCounts: map[int64]int{
			21001: 999,
		},
		loadMap: map[int64]*AccountLoadInfo{
			21001: {AccountID: 21001, LoadRate: 90, WaitingCount: 9},
			21002: {AccountID: 21002, LoadRate: 1, WaitingCount: 0},
		},
	}

	svc := &OpenAIGatewayService{
		accountRepo:        stubOpenAIAccountRepo{accounts: accounts},
		cache:              cache,
		cfg:                cfg,
		concurrencyService: NewConcurrencyService(concurrencyCache),
	}

	selection, decision, err := svc.SelectAccountWithScheduler(
		ctx,
		&groupID,
		"",
		"session_hash_sticky_busy",
		"gpt-5.1",
		TargetGroupAny,
		nil,
		OpenAIUpstreamTransportAny,
	)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	require.Equal(t, int64(21001), selection.Account.ID, "busy sticky account should remain selected")
	require.False(t, selection.Acquired)
	require.NotNil(t, selection.WaitPlan)
	require.Equal(t, int64(21001), selection.WaitPlan.AccountID)
	require.Equal(t, openAIAccountScheduleLayerSessionSticky, decision.Layer)
	require.True(t, decision.StickySessionHit)
}

func TestOpenAIGatewayService_SelectAccountWithScheduler_SessionSticky_ForceHTTP(t *testing.T) {
	ctx := context.Background()
	groupID := int64(1010)
	account := Account{
		ID:          2101,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Extra: map[string]any{
			"openai_ws_force_http": true,
		},
	}
	cache := &stubGatewayCache{
		sessionBindings: map[string]int64{
			"openai:session_hash_force_http": account.ID,
		},
	}

	svc := &OpenAIGatewayService{
		accountRepo:        stubOpenAIAccountRepo{accounts: []Account{account}},
		cache:              cache,
		cfg:                &config.Config{},
		concurrencyService: NewConcurrencyService(stubConcurrencyCache{}),
	}

	selection, decision, err := svc.SelectAccountWithScheduler(
		ctx,
		&groupID,
		"",
		"session_hash_force_http",
		"gpt-5.1",
		TargetGroupAny,
		nil,
		OpenAIUpstreamTransportAny,
	)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	require.Equal(t, account.ID, selection.Account.ID)
	require.Equal(t, openAIAccountScheduleLayerSessionSticky, decision.Layer)
	require.True(t, decision.StickySessionHit)
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}

func TestOpenAIGatewayService_SelectAccountWithScheduler_RequiredWSV2_SkipsStickyHTTPAccount(t *testing.T) {
	ctx := context.Background()
	groupID := int64(1011)
	accounts := []Account{
		{
			ID:          2201,
			Platform:    PlatformOpenAI,
			Type:        AccountTypeAPIKey,
			Status:      StatusActive,
			Schedulable: true,
			Concurrency: 1,
			Priority:    0,
		},
		{
			ID:          2202,
			Platform:    PlatformOpenAI,
			Type:        AccountTypeAPIKey,
			Status:      StatusActive,
			Schedulable: true,
			Concurrency: 1,
			Priority:    5,
			Extra: map[string]any{
				"openai_apikey_responses_websockets_v2_enabled": true,
			},
		},
	}
	cache := &stubGatewayCache{
		sessionBindings: map[string]int64{
			"openai:session_hash_ws_only": 2201,
		},
	}
	cfg := newOpenAIWSV2TestConfig()

	// 构造“HTTP-only 账号负载更低”的场景，验证 required transport 会强制过滤。
	concurrencyCache := stubConcurrencyCache{
		loadMap: map[int64]*AccountLoadInfo{
			2201: {AccountID: 2201, LoadRate: 0, WaitingCount: 0},
			2202: {AccountID: 2202, LoadRate: 90, WaitingCount: 5},
		},
	}

	svc := &OpenAIGatewayService{
		accountRepo:        stubOpenAIAccountRepo{accounts: accounts},
		cache:              cache,
		cfg:                cfg,
		concurrencyService: NewConcurrencyService(concurrencyCache),
	}

	selection, decision, err := svc.SelectAccountWithScheduler(
		ctx,
		&groupID,
		"",
		"session_hash_ws_only",
		"gpt-5.1",
		TargetGroupAny,
		nil,
		OpenAIUpstreamTransportResponsesWebsocketV2,
	)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	require.Equal(t, int64(2202), selection.Account.ID)
	require.Equal(t, openAIAccountScheduleLayerLoadBalance, decision.Layer)
	require.False(t, decision.StickySessionHit)
	require.Equal(t, 1, decision.CandidateCount)
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}

func TestOpenAIGatewayService_SelectAccountWithScheduler_RequiredWSV2_NoAvailableAccount(t *testing.T) {
	ctx := context.Background()
	groupID := int64(1012)
	accounts := []Account{
		{
			ID:          2301,
			Platform:    PlatformOpenAI,
			Type:        AccountTypeOAuth,
			Status:      StatusActive,
			Schedulable: true,
			Concurrency: 1,
		},
	}

	svc := &OpenAIGatewayService{
		accountRepo:        stubOpenAIAccountRepo{accounts: accounts},
		cache:              &stubGatewayCache{},
		cfg:                newOpenAIWSV2TestConfig(),
		concurrencyService: NewConcurrencyService(stubConcurrencyCache{}),
	}

	selection, decision, err := svc.SelectAccountWithScheduler(
		ctx,
		&groupID,
		"",
		"",
		"gpt-5.1",
		TargetGroupAny,
		nil,
		OpenAIUpstreamTransportResponsesWebsocketV2,
	)
	require.Error(t, err)
	require.Nil(t, selection)
	require.Equal(t, openAIAccountScheduleLayerLoadBalance, decision.Layer)
	require.Equal(t, 0, decision.CandidateCount)
}

func TestOpenAIGatewayService_SelectAccountWithScheduler_LoadBalanceTopKFallback(t *testing.T) {
	ctx := context.Background()
	groupID := int64(11)
	accounts := []Account{
		{
			ID:          3001,
			Platform:    PlatformOpenAI,
			Type:        AccountTypeAPIKey,
			Status:      StatusActive,
			Schedulable: true,
			Concurrency: 1,
			Priority:    0,
		},
		{
			ID:          3002,
			Platform:    PlatformOpenAI,
			Type:        AccountTypeAPIKey,
			Status:      StatusActive,
			Schedulable: true,
			Concurrency: 1,
			Priority:    0,
		},
		{
			ID:          3003,
			Platform:    PlatformOpenAI,
			Type:        AccountTypeAPIKey,
			Status:      StatusActive,
			Schedulable: true,
			Concurrency: 1,
			Priority:    0,
		},
	}

	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.LBTopK = 2
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.Priority = 0.4
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.Load = 1.0
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.Queue = 1.0
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.ErrorRate = 0.2
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.TTFT = 0.1

	concurrencyCache := stubConcurrencyCache{
		loadMap: map[int64]*AccountLoadInfo{
			3001: {AccountID: 3001, LoadRate: 95, WaitingCount: 8},
			3002: {AccountID: 3002, LoadRate: 20, WaitingCount: 1},
			3003: {AccountID: 3003, LoadRate: 10, WaitingCount: 0},
		},
		acquireResults: map[int64]bool{
			3003: false, // top1 失败，必须回退到 top-K 的下一候选
			3002: true,
		},
	}

	svc := &OpenAIGatewayService{
		accountRepo:        stubOpenAIAccountRepo{accounts: accounts},
		cache:              &stubGatewayCache{},
		cfg:                cfg,
		concurrencyService: NewConcurrencyService(concurrencyCache),
	}

	selection, decision, err := svc.SelectAccountWithScheduler(
		ctx,
		&groupID,
		"",
		"",
		"gpt-5.1",
		TargetGroupAny,
		nil,
		OpenAIUpstreamTransportAny,
	)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	require.Equal(t, int64(3002), selection.Account.ID)
	require.Equal(t, openAIAccountScheduleLayerLoadBalance, decision.Layer)
	require.Equal(t, 3, decision.CandidateCount)
	require.Equal(t, 2, decision.TopK)
	require.Greater(t, decision.LoadSkew, 0.0)
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}

func TestSelectByLoadBalance_ExhaustedOverflowUsesReserve(t *testing.T) {
	ctx := context.Background()
	groupID := int64(1301)
	accounts := []Account{
		newOpenAIExhaustedAccountForTest(3101, 10),
		newOpenAIReserveCandidateAccountForTest(3102, 10, 20),
	}
	loadMap := map[int64]*AccountLoadInfo{
		3101: {AccountID: 3101, CurrentConcurrency: 7, LoadRate: 70},
		3102: {AccountID: 3102, CurrentConcurrency: 0, LoadRate: 80},
	}
	svc := newOpenAIReserveSelectionServiceForTest(accounts, loadMap)

	selection, decision, err := svc.SelectAccountWithScheduler(
		ctx,
		&groupID,
		"",
		"session_hash_exhausted_overflow_reserve",
		"gpt-5.1",
		TargetGroupExhausted,
		nil,
		OpenAIUpstreamTransportAny,
	)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	require.Equal(t, int64(3102), selection.Account.ID)
	require.Equal(t, openAIAccountScheduleLayerLoadBalance, decision.Layer)
	require.Equal(t, "reserve", requireDecisionStringField(t, decision, "SelectedGroup"))
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}

func TestSelectByLoadBalance_ExhaustedEmptyTreatsUsageAsFullAndUsesReserve(t *testing.T) {
	ctx := context.Background()
	groupID := int64(13015)
	accounts := []Account{
		newOpenAIReserveCandidateAccountForTest(3115, 10, 20),
	}
	loadMap := map[int64]*AccountLoadInfo{
		3115: {AccountID: 3115, CurrentConcurrency: 0, LoadRate: 0},
	}
	svc := newOpenAIReserveSelectionServiceForTest(accounts, loadMap)

	selection, decision, err := svc.SelectAccountWithScheduler(
		ctx,
		&groupID,
		"",
		"session_hash_exhausted_empty_uses_reserve",
		"gpt-5.1",
		TargetGroupExhausted,
		nil,
		OpenAIUpstreamTransportAny,
	)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	require.Equal(t, int64(3115), selection.Account.ID)
	require.Equal(t, openAIAccountScheduleLayerLoadBalance, decision.Layer)
	require.Equal(t, "reserve", requireDecisionStringField(t, decision, "SelectedGroup"))
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}

func TestSelectByLoadBalance_ExhaustedEmptyUsesPaidTierModelSubsetReserve(t *testing.T) {
	ctx := context.Background()
	groupID := int64(13016)
	activeTeam := newOpenAIProjectionPaidTierAccount(3116, 10, "team", []string{"gpt-5.5"})
	projection := BuildOpenAIModelSubsetProjection(&OpenAIProjectionInputs{
		Bucket:           SchedulerBucket{GroupID: groupID, Platform: PlatformOpenAI, Mode: SchedulerModeSingle},
		CanonicalCatalog: []string{"gpt-5.5"},
		AccountsAll:      []Account{activeTeam},
	})
	loadMap := map[int64]*AccountLoadInfo{
		activeTeam.ID: {AccountID: activeTeam.ID, CurrentConcurrency: 0, LoadRate: 0},
	}
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.LBTopK = 1
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.Load = 1
	svc := &OpenAIGatewayService{
		accountRepo:        stubOpenAIAccountRepo{accounts: []Account{activeTeam}},
		cache:              &stubGatewayCache{sessionBindings: map[string]int64{}},
		cfg:                cfg,
		schedulerSnapshot:  &SchedulerSnapshotService{cache: &openAISnapshotCacheStub{openAIState: newOpenAIBucketStateForTest([]Account{activeTeam}, 11, projection.Models)}},
		concurrencyService: NewConcurrencyService(stubConcurrencyCache{loadMap: loadMap}),
	}

	selection, decision, err := svc.SelectAccountWithScheduler(
		ctx,
		&groupID,
		"",
		"session_hash_exhausted_empty_paid_tier_reserve",
		"gpt-5.5",
		TargetGroupExhausted,
		nil,
		OpenAIUpstreamTransportAny,
	)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	require.Equal(t, activeTeam.ID, selection.Account.ID)
	require.Equal(t, "reserve", requireDecisionStringField(t, decision, "SelectedGroup"))
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}

func TestSelectByLoadBalance_GPT55ProjectionTreatsCaseAndSysAsEquivalent(t *testing.T) {
	ctx := context.Background()
	groupID := int64(13017)
	activeTeam := newOpenAIProjectionPaidTierAccount(3117, 10, "team", []string{"gpt-5.5"})
	activeTeam.Credentials["model_mapping"] = map[string]any{"gpt-5.5": "gpt-5.5"}
	projection := BuildOpenAIModelSubsetProjection(&OpenAIProjectionInputs{
		Bucket:           SchedulerBucket{GroupID: groupID, Platform: PlatformOpenAI, Mode: SchedulerModeSingle},
		CanonicalCatalog: []string{"gpt-5.5"},
		AccountsAll:      []Account{activeTeam},
	})
	loadMap := map[int64]*AccountLoadInfo{
		activeTeam.ID: {AccountID: activeTeam.ID, CurrentConcurrency: 0, LoadRate: 0},
	}
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.LBTopK = 1
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.Load = 1
	svc := &OpenAIGatewayService{
		accountRepo:        stubOpenAIAccountRepo{accounts: []Account{activeTeam}},
		cache:              &stubGatewayCache{sessionBindings: map[string]int64{}},
		cfg:                cfg,
		schedulerSnapshot:  &SchedulerSnapshotService{cache: &openAISnapshotCacheStub{openAIState: newOpenAIBucketStateForTest([]Account{activeTeam}, 12, projection.Models)}},
		concurrencyService: NewConcurrencyService(stubConcurrencyCache{loadMap: loadMap}),
	}

	tests := []struct {
		name      string
		model     string
		target    AccountTargetGroup
		wantGroup string
	}{
		{name: "active uppercase", model: "GPT-5.5", target: TargetGroupActive, wantGroup: string(TargetGroupActive)},
		{name: "exhausted uppercase sys", model: "GPT-5.5-Sys", target: TargetGroupExhausted, wantGroup: "reserve"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			selection, decision, err := svc.SelectAccountWithScheduler(
				ctx,
				&groupID,
				"",
				"session_hash_gpt55_case_equivalent_"+tt.name,
				tt.model,
				tt.target,
				nil,
				OpenAIUpstreamTransportAny,
			)
			require.NoError(t, err)
			require.NotNil(t, selection)
			require.NotNil(t, selection.Account)
			require.Equal(t, activeTeam.ID, selection.Account.ID)
			require.Equal(t, tt.wantGroup, requireDecisionStringField(t, decision, "SelectedGroup"))
			if selection.ReleaseFunc != nil {
				selection.ReleaseFunc()
			}
		})
	}
}

func TestSelectByLoadBalance_ExhaustedBelowThresholdDoesNotUseReserve(t *testing.T) {
	ctx := context.Background()
	groupID := int64(1302)
	accounts := []Account{
		newOpenAIExhaustedAccountForTest(3201, 10),
		newOpenAIReserveCandidateAccountForTest(3202, 10, 20),
	}
	loadMap := map[int64]*AccountLoadInfo{
		3201: {AccountID: 3201, CurrentConcurrency: 6, LoadRate: 99},
		3202: {AccountID: 3202, CurrentConcurrency: 0, LoadRate: 0},
	}
	svc := newOpenAIReserveSelectionServiceForTest(accounts, loadMap)

	selection, decision, err := svc.SelectAccountWithScheduler(
		ctx,
		&groupID,
		"",
		"session_hash_exhausted_below_threshold",
		"gpt-5.1",
		TargetGroupExhausted,
		nil,
		OpenAIUpstreamTransportAny,
	)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	require.Equal(t, int64(3201), selection.Account.ID)
	require.Equal(t, "exhausted", requireDecisionStringField(t, decision, "SelectedGroup"))
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}

func TestSelectByLoadBalance_ReserveBelowThresholdAbsorbsOverflowFirst(t *testing.T) {
	ctx := context.Background()
	groupID := int64(1303)
	accounts := []Account{
		newOpenAIExhaustedAccountForTest(3301, 10),
		newOpenAIReserveCandidateAccountForTest(3302, 10, 20),
	}
	loadMap := map[int64]*AccountLoadInfo{
		3301: {AccountID: 3301, CurrentConcurrency: 7, LoadRate: 0},
		3302: {AccountID: 3302, CurrentConcurrency: 5, LoadRate: 99},
	}
	svc := newOpenAIReserveSelectionServiceForTest(accounts, loadMap)

	selection, decision, err := svc.SelectAccountWithScheduler(
		ctx,
		&groupID,
		"",
		"session_hash_reserve_absorbs_first",
		"gpt-5.1",
		TargetGroupExhausted,
		nil,
		OpenAIUpstreamTransportAny,
	)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	require.Equal(t, int64(3302), selection.Account.ID)
	require.Equal(t, 1, decision.CandidateCount)
	require.Equal(t, "reserve", requireDecisionStringField(t, decision, "SelectedGroup"))
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}

func TestSelectByLoadBalance_ReserveAtThresholdThenCoSchedulesWithExhausted(t *testing.T) {
	ctx := context.Background()
	groupID := int64(1304)
	accounts := []Account{
		newOpenAIExhaustedAccountForTest(3401, 10),
		newOpenAIReserveCandidateAccountForTest(3402, 10, 20),
	}
	loadMap := map[int64]*AccountLoadInfo{
		3401: {AccountID: 3401, CurrentConcurrency: 7, LoadRate: 0},
		3402: {AccountID: 3402, CurrentConcurrency: 6, LoadRate: 99},
	}
	svc := newOpenAIReserveSelectionServiceForTest(accounts, loadMap)

	selection, decision, err := svc.SelectAccountWithScheduler(
		ctx,
		&groupID,
		"",
		"session_hash_reserve_at_threshold",
		"gpt-5.1",
		TargetGroupExhausted,
		nil,
		OpenAIUpstreamTransportAny,
	)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	require.Equal(t, int64(3401), selection.Account.ID)
	require.Equal(t, 2, decision.CandidateCount)
	require.Equal(t, "exhausted", requireDecisionStringField(t, decision, "SelectedGroup"))
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}

func TestSelectByLoadBalance_ReserveOnlyOverflowFailureStillMarksSelectedGroupReserve(t *testing.T) {
	ctx := context.Background()
	groupID := int64(13045)
	accounts := []Account{
		newOpenAIExhaustedAccountForTest(3451, 10),
		newOpenAIReserveCandidateAccountForTest(3452, 10, 20),
	}
	loadMap := map[int64]*AccountLoadInfo{
		3451: {AccountID: 3451, CurrentConcurrency: 7, LoadRate: 0},
		3452: {AccountID: 3452, CurrentConcurrency: 5, LoadRate: 99},
	}
	svc := newOpenAIReserveSelectionServiceForTest(accounts, loadMap)
	svc.concurrencyService = NewConcurrencyService(stubConcurrencyCache{loadMap: loadMap, acquireErrors: map[int64]error{3452: errors.New("reserve acquire failed")}})

	selection, decision, err := svc.SelectAccountWithScheduler(
		ctx,
		&groupID,
		"",
		"session_hash_reserve_failure_group",
		"gpt-5.1",
		TargetGroupExhausted,
		nil,
		OpenAIUpstreamTransportAny,
	)
	require.Nil(t, selection)
	require.Error(t, err)
	require.Equal(t, openAIAccountScheduleLayerLoadBalance, decision.Layer)
	require.Equal(t, "reserve", requireDecisionStringField(t, decision, "SelectedGroup"))
}

func TestReservePoolReleasesHighestRemainingQuotaBackToActive(t *testing.T) {
	ctx := context.Background()
	groupID := int64(1305)
	accounts := []Account{
		newOpenAIExhaustedAccountForTest(3501, 1),
		newOpenAIReserveCandidateAccountForTest(3502, 1, 10),
		newOpenAIReserveCandidateAccountForTest(3503, 1, 80),
	}
	loadMap := map[int64]*AccountLoadInfo{
		3501: {AccountID: 3501, CurrentConcurrency: 1, LoadRate: 50},
		3502: {AccountID: 3502, CurrentConcurrency: 0, LoadRate: 0},
		3503: {AccountID: 3503, CurrentConcurrency: 0, LoadRate: 99},
	}
	svc := newOpenAIReserveSelectionServiceForTest(accounts, loadMap)

	selection, decision, err := svc.SelectAccountWithScheduler(
		ctx,
		&groupID,
		"",
		"session_hash_reserve_release_high_quota",
		"gpt-5.1",
		TargetGroupExhausted,
		nil,
		OpenAIUpstreamTransportAny,
	)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	require.Equal(t, int64(3503), selection.Account.ID)
	require.Equal(t, "reserve", requireDecisionStringField(t, decision, "SelectedGroup"))
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}

func TestSelectByLoadBalance_TargetGroupAnyCanSelectReserve(t *testing.T) {
	ctx := context.Background()
	groupID := int64(1306)
	accounts := []Account{
		newOpenAIExhaustedAccountForTest(3601, 4),
		newOpenAIReserveCandidateAccountForTest(3602, 4, 20),
		{
			ID:          3603,
			Platform:    PlatformOpenAI,
			Type:        AccountTypeAPIKey,
			Status:      StatusActive,
			Schedulable: true,
			Concurrency: 4,
		},
	}
	loadMap := map[int64]*AccountLoadInfo{
		3601: {AccountID: 3601, CurrentConcurrency: 3, LoadRate: 90},
		3602: {AccountID: 3602, CurrentConcurrency: 0, LoadRate: 0},
		3603: {AccountID: 3603, CurrentConcurrency: 0, LoadRate: 10},
	}
	svc := newOpenAIReserveSelectionServiceForTest(accounts, loadMap)

	selection, decision, err := svc.SelectAccountWithScheduler(
		ctx,
		&groupID,
		"",
		"session_hash_any_can_reserve",
		"gpt-5.1",
		TargetGroupAny,
		nil,
		OpenAIUpstreamTransportAny,
	)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	require.Equal(t, int64(3602), selection.Account.ID)
	require.Equal(t, openAISelectedGroupReserve, requireDecisionStringField(t, decision, "SelectedGroup"))
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}

func TestSelectByLoadBalance_TargetGroupActiveCanSelectReserve(t *testing.T) {
	ctx := context.Background()
	groupID := int64(1307)
	accounts := []Account{
		newOpenAIExhaustedAccountForTest(3701, 4),
		newOpenAIReserveCandidateAccountForTest(3702, 4, 20),
		{
			ID:          3703,
			Platform:    PlatformOpenAI,
			Type:        AccountTypeAPIKey,
			Status:      StatusActive,
			Schedulable: true,
			Concurrency: 4,
		},
	}
	loadMap := map[int64]*AccountLoadInfo{
		3701: {AccountID: 3701, CurrentConcurrency: 3, LoadRate: 90},
		3702: {AccountID: 3702, CurrentConcurrency: 0, LoadRate: 0},
		3703: {AccountID: 3703, CurrentConcurrency: 0, LoadRate: 10},
	}
	svc := newOpenAIReserveSelectionServiceForTest(accounts, loadMap)

	selection, decision, err := svc.SelectAccountWithScheduler(
		ctx,
		&groupID,
		"",
		"session_hash_active_can_reserve",
		"gpt-5.1",
		TargetGroupActive,
		nil,
		OpenAIUpstreamTransportAny,
	)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	require.Equal(t, int64(3702), selection.Account.ID)
	require.Equal(t, openAISelectedGroupReserve, requireDecisionStringField(t, decision, "SelectedGroup"))
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}

func TestSelectByLoadBalance_TargetGroupAnyCanSelectReserveFromProjection(t *testing.T) {
	ctx := context.Background()
	groupID := int64(13070)
	overlayReserve := Account{
		ID:          37041,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 4,
		Credentials: map[string]any{"model_mapping": map[string]any{"gpt-5.6": "gpt-5.6"}},
	}
	activeAccount := Account{
		ID:          37042,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 4,
		Credentials: map[string]any{"model_mapping": map[string]any{"gpt-5.6": "gpt-5.6"}},
	}
	snapshotCache := &openAISnapshotCacheStub{
		snapshotAccounts: []*Account{&overlayReserve, &activeAccount},
		openAIState: newOpenAIBucketStateForTest([]Account{overlayReserve, activeAccount}, 7, map[string]OpenAIModelRoleView{
			"gpt-5.6": {
				CanonicalModel:     "gpt-5.6",
				ReserveOverflowIDs: []int64{overlayReserve.ID},
			},
		}),
	}
	loadMap := map[int64]*AccountLoadInfo{
		overlayReserve.ID: {AccountID: overlayReserve.ID, CurrentConcurrency: 0, LoadRate: 0},
		activeAccount.ID:  {AccountID: activeAccount.ID, CurrentConcurrency: 0, LoadRate: 10},
	}
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.LBTopK = 1
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.Load = 1
	svc := &OpenAIGatewayService{
		accountRepo:        stubOpenAIAccountRepo{accounts: []Account{overlayReserve, activeAccount}},
		cache:              &stubGatewayCache{sessionBindings: map[string]int64{}},
		cfg:                cfg,
		schedulerSnapshot:  &SchedulerSnapshotService{cache: snapshotCache},
		concurrencyService: NewConcurrencyService(stubConcurrencyCache{loadMap: loadMap}),
	}

	selection, decision, err := svc.SelectAccountWithScheduler(
		ctx,
		&groupID,
		"",
		"session_hash_projection_overlay_accept",
		"gpt-5.6",
		TargetGroupAny,
		nil,
		OpenAIUpstreamTransportAny,
	)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	require.Equal(t, overlayReserve.ID, selection.Account.ID)
	require.Equal(t, openAISelectedGroupReserve, requireDecisionStringField(t, decision, "SelectedGroup"))
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}

func TestSelectByLoadBalance_ReserveSelectedGroupWritesReserveForActiveAny(t *testing.T) {
	ctx := context.Background()
	accounts := []Account{
		newOpenAIReserveCandidateAccountForTest(37051, 4, 20),
	}
	loadMap := map[int64]*AccountLoadInfo{
		37051: {AccountID: 37051, CurrentConcurrency: 0, LoadRate: 0},
	}

	for _, tc := range []struct {
		name        string
		groupID     int64
		targetGroup AccountTargetGroup
	}{
		{name: "any_reserve_only", groupID: 13071, targetGroup: TargetGroupAny},
		{name: "active_reserve_only", groupID: 13072, targetGroup: TargetGroupActive},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := newOpenAIReserveSelectionServiceForTest(accounts, loadMap)

			selection, decision, err := svc.SelectAccountWithScheduler(
				ctx,
				&tc.groupID,
				"",
				"session_hash_"+tc.name,
				"gpt-5.1",
				tc.targetGroup,
				nil,
				OpenAIUpstreamTransportAny,
			)
			require.NoError(t, err)
			require.NotNil(t, selection)
			require.NotNil(t, selection.Account)
			require.Equal(t, int64(37051), selection.Account.ID)
			require.Equal(t, openAISelectedGroupReserve, requireDecisionStringField(t, decision, "SelectedGroup"))
			if selection.ReleaseFunc != nil {
				selection.ReleaseFunc()
			}
		})
	}
}

func TestSelectByLoadBalance_ReserveSelectedGroupWritesSharedAndAffinitySticky(t *testing.T) {
	ctx := context.Background()
	groupID := int64(1308)
	sessionHash := "session_hash_reserve_no_shared_sticky"
	accounts := []Account{
		newOpenAIExhaustedAccountForTest(3801, 10),
		newOpenAIReserveCandidateAccountForTest(3802, 10, 20),
	}
	loadMap := map[int64]*AccountLoadInfo{
		3801: {AccountID: 3801, CurrentConcurrency: 7, LoadRate: 70},
		3802: {AccountID: 3802, CurrentConcurrency: 0, LoadRate: 80},
	}
	cache := newOpenAIAffinityGatewayCacheStub()
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.LBTopK = 1
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.Load = 1
	svc := &OpenAIGatewayService{
		accountRepo:        stubOpenAIAccountRepo{accounts: accounts},
		cache:              cache,
		cfg:                cfg,
		concurrencyService: NewConcurrencyService(stubConcurrencyCache{loadMap: loadMap}),
	}

	selection, decision, err := svc.SelectAccountWithScheduler(
		ctx,
		&groupID,
		"",
		sessionHash,
		"gpt-5.1",
		TargetGroupExhausted,
		nil,
		OpenAIUpstreamTransportAny,
	)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	require.Equal(t, int64(3802), selection.Account.ID)
	require.Equal(t, "reserve", requireDecisionStringField(t, decision, "SelectedGroup"))
	require.Equal(t, int64(3802), cache.sessionBindings["openai:"+sessionHash])
	binding := cache.mustGetAffinityBinding(t, openAIStickyAffinityBindingNamespace, groupID, "openai:"+sessionHash)
	require.Equal(t, int64(3802), binding.BoundAccountID)
	require.Equal(t, string(TargetGroupExhausted), binding.AffinityDomain)
	require.Equal(t, openAISelectedGroupReserve, binding.SelectedGroup)
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}

func TestStickyReserveBindingStillMatchesExhaustedClass(t *testing.T) {
	ctx := context.Background()
	groupID := int64(1317)
	sessionHash := "session_hash_reserve_affinity_exhausted"
	accounts := []Account{
		newOpenAIExhaustedAccountForTest(3931, 10),
		newOpenAIReserveCandidateAccountForTest(3932, 10, 20),
	}
	sharedCache := newOpenAIAffinityGatewayCacheStub()
	writerConcurrencyCache := &stubConcurrencyCache{loadMap: map[int64]*AccountLoadInfo{
		3931: {AccountID: 3931, CurrentConcurrency: 7, LoadRate: 70},
		3932: {AccountID: 3932, CurrentConcurrency: 0, LoadRate: 0},
	}}
	readerConcurrencyCache := &stubConcurrencyCache{loadMap: map[int64]*AccountLoadInfo{
		3931: {AccountID: 3931, CurrentConcurrency: 1, LoadRate: 10},
		3932: {AccountID: 3932, CurrentConcurrency: 9, LoadRate: 90},
	}}
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.LBTopK = 1
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.Load = 1
	writerSvc := &OpenAIGatewayService{
		accountRepo:        stubOpenAIAccountRepo{accounts: accounts},
		cache:              sharedCache,
		cfg:                cfg,
		concurrencyService: NewConcurrencyService(writerConcurrencyCache),
	}
	readerSvc := &OpenAIGatewayService{
		accountRepo:        stubOpenAIAccountRepo{accounts: accounts},
		cache:              sharedCache,
		cfg:                cfg,
		concurrencyService: NewConcurrencyService(readerConcurrencyCache),
	}

	selection, decision, err := writerSvc.SelectAccountWithScheduler(
		ctx,
		&groupID,
		"",
		sessionHash,
		"gpt-5.1",
		TargetGroupExhausted,
		nil,
		OpenAIUpstreamTransportAny,
	)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	require.Equal(t, int64(3932), selection.Account.ID)
	require.Equal(t, openAISelectedGroupReserve, requireDecisionStringField(t, decision, "SelectedGroup"))
	require.Equal(t, int64(3932), sharedCache.sessionBindings["openai:"+sessionHash])
	binding := sharedCache.mustGetAffinityBinding(t, openAIStickyAffinityBindingNamespace, groupID, "openai:"+sessionHash)
	require.Equal(t, int64(3932), binding.BoundAccountID)
	require.Equal(t, string(TargetGroupExhausted), binding.AffinityDomain)
	require.Equal(t, openAISelectedGroupReserve, binding.SelectedGroup)
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}

	selection, decision, err = readerSvc.SelectAccountWithScheduler(
		ctx,
		&groupID,
		"",
		sessionHash,
		"gpt-5.1",
		TargetGroupExhausted,
		nil,
		OpenAIUpstreamTransportAny,
	)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	require.Equal(t, int64(3932), selection.Account.ID)
	require.Equal(t, openAIAccountScheduleLayerSessionSticky, decision.Layer)
	require.True(t, decision.StickySessionHit)
	require.Equal(t, openAIStickyEvalResultHit, requireStickyStringField(t, decision, "EvalResult"))
	require.NotEqual(t, openAIStickyEvalResultMissBindingRestricted, requireStickyStringField(t, decision, "EvalResult"))
	require.Equal(t, openAISelectedGroupReserve, requireDecisionStringField(t, decision, "SelectedGroup"))
	require.Equal(t, int64(3932), requireStickyAffinityBindingAccountID(t, decision))
	require.Equal(t, string(TargetGroupExhausted), requireStickyAffinityBindingStringField(t, decision, "AffinityDomain"))
	require.Equal(t, openAISelectedGroupReserve, requireStickyAffinityBindingStringField(t, decision, "SelectedGroup"))
	require.Equal(t, int64(3932), sharedCache.sessionBindings["openai:"+sessionHash])
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}

func TestStickyReserveBindingStillMatchesExhaustedClass_OnProjectionMiss(t *testing.T) {
	ctx := context.Background()
	groupID := int64(13171)
	sessionHash := "session_hash_reserve_affinity_exhausted_projection_miss"
	accounts := []Account{newOpenAIExhaustedAccountForTest(3937, 10), newOpenAIReserveCandidateAccountForTest(3938, 10, 20)}
	sharedCache := newOpenAIAffinityGatewayCacheStub()
	writerLoadMap := map[int64]*AccountLoadInfo{3937: {AccountID: 3937, CurrentConcurrency: 7, LoadRate: 70}, 3938: {AccountID: 3938, CurrentConcurrency: 0, LoadRate: 0}}
	readerLoadMap := map[int64]*AccountLoadInfo{3937: {AccountID: 3937, CurrentConcurrency: 1, LoadRate: 10}, 3938: {AccountID: 3938, CurrentConcurrency: 9, LoadRate: 90}}
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.LBTopK = 1
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.Load = 1
	snapshotCache := &openAISnapshotCacheStub{snapshotAccounts: []*Account{&accounts[0], &accounts[1]}, accountsByID: map[int64]*Account{3937: &accounts[0], 3938: &accounts[1]}, openAIStateMiss: true}
	writerSvc := &OpenAIGatewayService{accountRepo: stubOpenAIAccountRepo{accounts: accounts}, cache: sharedCache, cfg: cfg, schedulerSnapshot: &SchedulerSnapshotService{cache: snapshotCache}, concurrencyService: NewConcurrencyService(stubConcurrencyCache{loadMap: writerLoadMap})}
	readerSvc := &OpenAIGatewayService{accountRepo: stubOpenAIAccountRepo{accounts: accounts}, cache: sharedCache, cfg: cfg, schedulerSnapshot: &SchedulerSnapshotService{cache: snapshotCache}, concurrencyService: NewConcurrencyService(stubConcurrencyCache{loadMap: readerLoadMap})}

	selection, decision, err := writerSvc.SelectAccountWithScheduler(ctx, &groupID, "", sessionHash, "gpt-5.1", TargetGroupExhausted, nil, OpenAIUpstreamTransportAny)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	require.Equal(t, int64(3938), selection.Account.ID)
	require.Equal(t, openAISelectedGroupReserve, requireDecisionStringField(t, decision, "SelectedGroup"))
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}

	selection, decision, err = readerSvc.SelectAccountWithScheduler(ctx, &groupID, "", sessionHash, "gpt-5.1", TargetGroupExhausted, nil, OpenAIUpstreamTransportAny)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	require.Equal(t, int64(3938), selection.Account.ID)
	require.Equal(t, openAIAccountScheduleLayerSessionSticky, decision.Layer)
	require.True(t, decision.StickySessionHit)
	require.Equal(t, openAISelectedGroupReserve, requireDecisionStringField(t, decision, "SelectedGroup"))
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}

func TestStickyReserveBinding_ProjectionVersionMismatchRebinds(t *testing.T) {
	ctx := context.Background()
	groupID := int64(13170)
	sessionHash := "session_hash_projection_version_mismatch"
	exhaustedAccount := newOpenAIExhaustedAccountForTest(39341, 10)
	exhaustedAccount.Credentials["model_mapping"] = map[string]any{"gpt-5.4": "gpt-5.4"}
	reserveAccount := newOpenAIReserveCandidateAccountForTest(39342, 10, 20)
	reserveAccount.Credentials["model_mapping"] = map[string]any{"gpt-5.4": "gpt-5.4"}
	snapshotCache := &openAISnapshotCacheStub{
		snapshotAccounts: []*Account{&reserveAccount},
		accountsByID: map[int64]*Account{
			exhaustedAccount.ID: &exhaustedAccount,
			reserveAccount.ID:   &reserveAccount,
		},
		openAIState: newOpenAIBucketStateForTest([]Account{exhaustedAccount, reserveAccount}, 2, map[string]OpenAIModelRoleView{
			"gpt-5.4": {
				CanonicalModel:   "gpt-5.4",
				ExhaustedBaseIDs: []int64{exhaustedAccount.ID},
			},
		}),
	}
	sharedCache := newOpenAIAffinityGatewayCacheStub()
	sharedCache.sessionBindings["openai:"+sessionHash] = reserveAccount.ID
	sharedCache.setAffinityBinding(t, openAIStickyAffinityBindingNamespace, groupID, "openai:"+sessionHash, &openAIAffinityBinding{
		BoundAccountID:     reserveAccount.ID,
		AffinityDomain:     string(TargetGroupExhausted),
		SelectedGroup:      openAISelectedGroupReserve,
		ProjectionVersion:  1,
		ProjectionModelKey: "gpt-5.4",
	}, time.Hour)
	loadMap := map[int64]*AccountLoadInfo{
		exhaustedAccount.ID: {AccountID: exhaustedAccount.ID, CurrentConcurrency: 1, LoadRate: 10},
		reserveAccount.ID:   {AccountID: reserveAccount.ID, CurrentConcurrency: 0, LoadRate: 0},
	}
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.LBTopK = 1
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.Load = 1
	svc := &OpenAIGatewayService{
		accountRepo:        stubOpenAIAccountRepo{accounts: []Account{exhaustedAccount, reserveAccount}},
		cache:              sharedCache,
		cfg:                cfg,
		schedulerSnapshot:  &SchedulerSnapshotService{cache: snapshotCache},
		concurrencyService: NewConcurrencyService(stubConcurrencyCache{loadMap: loadMap}),
	}

	selection, decision, err := svc.SelectAccountWithScheduler(
		ctx,
		&groupID,
		"",
		sessionHash,
		"gpt-5.4",
		TargetGroupExhausted,
		nil,
		OpenAIUpstreamTransportAny,
	)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	require.Equal(t, exhaustedAccount.ID, selection.Account.ID)
	require.Equal(t, openAIAccountScheduleLayerLoadBalance, decision.Layer)
	require.Equal(t, string(TargetGroupExhausted), requireDecisionStringField(t, decision, "SelectedGroup"))
	require.Equal(t, exhaustedAccount.ID, sharedCache.sessionBindings["openai:"+sessionHash])
	require.False(t, sharedCache.hasAffinityBinding(openAIStickyAffinityBindingNamespace, groupID, "openai:"+sessionHash))
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}

func TestSelectByLoadBalance_ExhaustedProjectionMissFallsBackToLegacyReserve(t *testing.T) {
	ctx := context.Background()
	groupID := int64(13173)
	exhaustedBase := newOpenAIExhaustedAccountForTest(39371, 10)
	reserveAccount := newOpenAIReserveCandidateAccountForTest(39372, 10, 20)
	accounts := []Account{exhaustedBase, reserveAccount}
	loadMap := map[int64]*AccountLoadInfo{39371: {AccountID: 39371, CurrentConcurrency: 7, LoadRate: 70}, 39372: {AccountID: 39372, CurrentConcurrency: 0, LoadRate: 0}}
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.LBTopK = 1
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.Load = 1
	snapshotCache := &openAISnapshotCacheStub{snapshotAccounts: []*Account{&exhaustedBase, &reserveAccount}, accountsByID: map[int64]*Account{39371: &exhaustedBase, 39372: &reserveAccount}, openAIStateMiss: true}
	svc := &OpenAIGatewayService{accountRepo: stubOpenAIAccountRepo{accounts: accounts}, cache: &stubGatewayCache{sessionBindings: map[string]int64{}}, cfg: cfg, schedulerSnapshot: &SchedulerSnapshotService{cache: snapshotCache}, concurrencyService: NewConcurrencyService(stubConcurrencyCache{loadMap: loadMap})}

	selection, decision, err := svc.SelectAccountWithScheduler(ctx, &groupID, "", "session_hash_exhausted_projection_miss", "gpt-5.1", TargetGroupExhausted, nil, OpenAIUpstreamTransportAny)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	require.Equal(t, reserveAccount.ID, selection.Account.ID)
	require.Equal(t, openAISelectedGroupReserve, requireDecisionStringField(t, decision, "SelectedGroup"))
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}

func TestOpenAIGatewayService_SelectAccountWithScheduler_StickyReserveBindingUnknownModelProjectionMissFailsClosed(t *testing.T) {
	ctx := context.Background()
	groupID := int64(13174)
	sessionHash := "session_hash_reserve_affinity_unknown_projection_miss"
	exhaustedAccount := newOpenAIExhaustedAccountForTest(39381, 10)
	reserveAccount := newOpenAIReserveCandidateAccountForTest(39382, 10, 20)
	accounts := []Account{exhaustedAccount, reserveAccount}
	sharedCache := newOpenAIAffinityGatewayCacheStub()
	sharedCache.sessionBindings["openai:"+sessionHash] = reserveAccount.ID
	sharedCache.setAffinityBinding(t, openAIStickyAffinityBindingNamespace, groupID, "openai:"+sessionHash, &openAIAffinityBinding{BoundAccountID: reserveAccount.ID, AffinityDomain: string(TargetGroupExhausted), SelectedGroup: openAISelectedGroupReserve}, time.Hour)
	loadMap := map[int64]*AccountLoadInfo{39381: {AccountID: 39381, CurrentConcurrency: 7, LoadRate: 70}, 39382: {AccountID: 39382, CurrentConcurrency: 0, LoadRate: 0}}
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.LBTopK = 1
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.Load = 1
	snapshotCache := &openAISnapshotCacheStub{snapshotAccounts: []*Account{&exhaustedAccount, &reserveAccount}, accountsByID: map[int64]*Account{39381: &exhaustedAccount, 39382: &reserveAccount}, openAIStateMiss: true}
	svc := &OpenAIGatewayService{accountRepo: stubOpenAIAccountRepo{accounts: accounts}, cache: sharedCache, cfg: cfg, schedulerSnapshot: &SchedulerSnapshotService{cache: snapshotCache}, concurrencyService: NewConcurrencyService(stubConcurrencyCache{loadMap: loadMap})}

	selection, _, err := svc.SelectAccountWithScheduler(ctx, &groupID, "", sessionHash, "gpt-5.unknown", TargetGroupExhausted, nil, OpenAIUpstreamTransportAny)
	require.ErrorIs(t, err, ErrSchedulerCacheNotReady)
	require.Nil(t, selection)
}

func TestSelectByLoadBalance_ExhaustedUnknownModelProjectionMissFailsClosed(t *testing.T) {
	ctx := context.Background()
	groupID := int64(13175)
	exhaustedBase := newOpenAIExhaustedAccountForTest(39391, 10)
	reserveAccount := newOpenAIReserveCandidateAccountForTest(39392, 10, 20)
	accounts := []Account{exhaustedBase, reserveAccount}
	loadMap := map[int64]*AccountLoadInfo{39391: {AccountID: 39391, CurrentConcurrency: 7, LoadRate: 70}, 39392: {AccountID: 39392, CurrentConcurrency: 0, LoadRate: 0}}
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.LBTopK = 1
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.Load = 1
	snapshotCache := &openAISnapshotCacheStub{snapshotAccounts: []*Account{&exhaustedBase, &reserveAccount}, accountsByID: map[int64]*Account{39391: &exhaustedBase, 39392: &reserveAccount}, openAIStateMiss: true}
	svc := &OpenAIGatewayService{accountRepo: stubOpenAIAccountRepo{accounts: accounts}, cache: &stubGatewayCache{sessionBindings: map[string]int64{}}, cfg: cfg, schedulerSnapshot: &SchedulerSnapshotService{cache: snapshotCache}, concurrencyService: NewConcurrencyService(stubConcurrencyCache{loadMap: loadMap})}

	selection, _, err := svc.SelectAccountWithScheduler(ctx, &groupID, "", "session_hash_unknown_model_projection_miss", "gpt-5.unknown", TargetGroupExhausted, nil, OpenAIUpstreamTransportAny)
	require.ErrorIs(t, err, ErrSchedulerCacheNotReady)
	require.Nil(t, selection)
}

func TestStickyAffinityBindingPersistsAcrossServiceInstances(t *testing.T) {
	TestStickyReserveBindingStillMatchesExhaustedClass(t)
}

func TestPreviousResponseReserveBindingDoesNotMissBindingRestricted(t *testing.T) {
	ctx := context.Background()
	groupID := int64(1318)
	sessionHash := "session_hash_previous_response_reserve_affinity"
	reserveAccount := newOpenAIReserveCandidateAccountForTest(3933, 1, 20)
	cache := newOpenAIAffinityGatewayCacheStub()
	cfg := newOpenAIWSV2TestConfig()
	svc := &OpenAIGatewayService{
		accountRepo:        stubOpenAIAccountRepo{accounts: []Account{reserveAccount}},
		cache:              cache,
		cfg:                cfg,
		concurrencyService: NewConcurrencyService(stubConcurrencyCache{}),
	}
	store := svc.getOpenAIWSStateStore()
	binding := &openAIAffinityBinding{
		BoundAccountID: reserveAccount.ID,
		AffinityDomain: string(TargetGroupExhausted),
		SelectedGroup:  openAISelectedGroupReserve,
	}
	require.NoError(t, store.BindResponseAccount(ctx, groupID, "resp_prev_reserve_affinity", reserveAccount.ID, time.Hour))
	cache.setAffinityBinding(t, openAIResponseAffinityBindingNamespace, groupID, "resp_prev_reserve_affinity", binding, time.Hour)

	selection, decision, err := svc.SelectAccountWithScheduler(
		ctx,
		&groupID,
		"resp_prev_reserve_affinity",
		sessionHash,
		"gpt-5.1",
		TargetGroupExhausted,
		nil,
		OpenAIUpstreamTransportAny,
	)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	require.Equal(t, reserveAccount.ID, selection.Account.ID)
	require.Equal(t, openAIAccountScheduleLayerPreviousResponse, decision.Layer)
	require.True(t, decision.StickyPreviousHit)
	require.Equal(t, openAIStickyEvalResultBypassedPreviousResponse, requireStickyStringField(t, decision, "EvalResult"))
	require.NotEqual(t, openAIStickyEvalResultMissBindingRestricted, requireStickyStringField(t, decision, "EvalResult"))
	require.Equal(t, openAISelectedGroupReserve, requireDecisionStringField(t, decision, "SelectedGroup"))
	require.Equal(t, int64(3933), requireStickyAffinityBindingAccountID(t, decision))
	require.Equal(t, string(TargetGroupExhausted), requireStickyAffinityBindingStringField(t, decision, "AffinityDomain"))
	require.Equal(t, openAISelectedGroupReserve, requireStickyAffinityBindingStringField(t, decision, "SelectedGroup"))
	require.Equal(t, reserveAccount.ID, cache.sessionBindings["openai:"+sessionHash])
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}

func TestOpenAIGatewayService_SelectAccountWithScheduler_PreviousResponseReserveDoesNotBypassGuardrail(t *testing.T) {
	ctx := context.Background()
	groupID := int64(1309)
	sessionHash := "session_hash_previous_response_reserve"
	reserveAccount := newOpenAIReserveCandidateAccountForTest(3901, 1, 20)
	cache := &stubGatewayCache{sessionBindings: map[string]int64{}}
	cfg := newOpenAIWSV2TestConfig()
	svc := &OpenAIGatewayService{
		accountRepo:        stubOpenAIAccountRepo{accounts: []Account{reserveAccount}},
		cache:              cache,
		cfg:                cfg,
		concurrencyService: NewConcurrencyService(stubConcurrencyCache{}),
	}
	store := svc.getOpenAIWSStateStore()
	require.NoError(t, store.BindResponseAccount(ctx, groupID, "resp_prev_reserve", reserveAccount.ID, time.Hour))

	selection, decision, err := svc.SelectAccountWithScheduler(
		ctx,
		&groupID,
		"resp_prev_reserve",
		sessionHash,
		"gpt-5.1",
		TargetGroupExhausted,
		nil,
		OpenAIUpstreamTransportAny,
	)
	require.Error(t, err)
	require.Nil(t, selection)
	require.Equal(t, openAIAccountScheduleLayerLoadBalance, decision.Layer)
	require.False(t, decision.StickyPreviousHit)
	_, bound := cache.sessionBindings["openai:"+sessionHash]
	require.False(t, bound)
}

func TestOpenAIGatewayService_SelectAccountWithScheduler_PreviousResponseReserveAcceptedForAny(t *testing.T) {
	ctx := context.Background()
	groupID := int64(1313)
	sessionHash := "session_hash_previous_response_any_reserve"
	exhaustedBase := Account{
		ID:          3913,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Extra: map[string]any{
			"quota_limit": float64(100),
			"quota_used":  float64(100),
			"openai_apikey_responses_websockets_v2_enabled": true,
		},
	}
	overlayReserve := newOpenAIReserveCandidateAccountForTest(3914, 1, 80)
	activeReserve := newOpenAIReserveCandidateAccountForTest(3915, 1, 20)
	activeAccount := Account{ID: 3916, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1}
	cache := &stubGatewayCache{sessionBindings: map[string]int64{}}
	cfg := newOpenAIWSV2TestConfig()
	cfg.Gateway.OpenAIWS.LBTopK = 1
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.Load = 1
	snapshotCache := &openAISnapshotCacheStub{
		snapshotAccounts: []*Account{&exhaustedBase, &overlayReserve, &activeReserve, &activeAccount},
		accountsByID:     map[int64]*Account{3913: &exhaustedBase, 3914: &overlayReserve, 3915: &activeReserve, 3916: &activeAccount},
		openAIState: newOpenAIBucketStateForTest([]Account{exhaustedBase, overlayReserve, activeReserve, activeAccount}, 10, map[string]OpenAIModelRoleView{
			"gpt-5.1": {
				CanonicalModel:     "gpt-5.1",
				ExhaustedBaseIDs:   []int64{exhaustedBase.ID},
				ReserveOverflowIDs: []int64{overlayReserve.ID},
			},
		}),
	}
	svc := &OpenAIGatewayService{
		accountRepo:        stubOpenAIAccountRepo{accounts: []Account{exhaustedBase, overlayReserve, activeReserve, activeAccount}},
		cache:              cache,
		cfg:                cfg,
		schedulerSnapshot:  &SchedulerSnapshotService{cache: snapshotCache},
		concurrencyService: NewConcurrencyService(stubConcurrencyCache{loadMap: map[int64]*AccountLoadInfo{3913: {AccountID: 3913, LoadRate: 90}, 3914: {AccountID: 3914, LoadRate: 0}, 3915: {AccountID: 3915, LoadRate: 40}, 3916: {AccountID: 3916, LoadRate: 10}}}),
	}
	store := svc.getOpenAIWSStateStore()
	require.NoError(t, store.BindResponseAccount(ctx, groupID, "resp_prev_any_reserve", overlayReserve.ID, time.Hour))

	selection, decision, err := svc.SelectAccountWithScheduler(
		ctx,
		&groupID,
		"resp_prev_any_reserve",
		sessionHash,
		"gpt-5.1",
		TargetGroupAny,
		nil,
		OpenAIUpstreamTransportAny,
	)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	require.Equal(t, overlayReserve.ID, selection.Account.ID)
	require.Equal(t, openAIAccountScheduleLayerPreviousResponse, decision.Layer)
	require.True(t, decision.StickyPreviousHit)
	require.Equal(t, openAISelectedGroupReserve, requireDecisionStringField(t, decision, "SelectedGroup"))
	require.Equal(t, overlayReserve.ID, cache.sessionBindings["openai:"+sessionHash])
	accountID, getErr := store.GetResponseAccount(ctx, groupID, "resp_prev_any_reserve")
	require.NoError(t, getErr)
	require.Equal(t, overlayReserve.ID, accountID)
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}

func TestOpenAIGatewayService_SelectAccountWithScheduler_PreviousResponseNonOverlayReserveCandidateAcceptedForAny(t *testing.T) {
	ctx := context.Background()
	groupID := int64(1315)
	sessionHash := "session_hash_previous_response_any_non_overlay"
	exhaustedBase := Account{
		ID:          3920,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Extra: map[string]any{
			"quota_limit": float64(100),
			"quota_used":  float64(100),
			"openai_apikey_responses_websockets_v2_enabled": true,
		},
	}
	overlayReserve := newOpenAIReserveCandidateAccountForTest(3921, 1, 80)
	activeReserve := newOpenAIReserveCandidateAccountForTest(3922, 1, 20)
	activeAccount := Account{ID: 3923, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1}
	cache := &stubGatewayCache{sessionBindings: map[string]int64{}}
	cfg := newOpenAIWSV2TestConfig()
	cfg.Gateway.OpenAIWS.LBTopK = 1
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.Load = 1
	svc := &OpenAIGatewayService{
		accountRepo:        stubOpenAIAccountRepo{accounts: []Account{exhaustedBase, overlayReserve, activeReserve, activeAccount}},
		cache:              cache,
		cfg:                cfg,
		concurrencyService: NewConcurrencyService(stubConcurrencyCache{loadMap: map[int64]*AccountLoadInfo{3920: {AccountID: 3920, LoadRate: 90}, 3921: {AccountID: 3921, LoadRate: 0}, 3922: {AccountID: 3922, LoadRate: 40}, 3923: {AccountID: 3923, LoadRate: 10}}}),
	}
	store := svc.getOpenAIWSStateStore()
	require.NoError(t, store.BindResponseAccount(ctx, groupID, "resp_prev_any_non_overlay", activeReserve.ID, time.Hour))

	selection, decision, err := svc.SelectAccountWithScheduler(
		ctx,
		&groupID,
		"resp_prev_any_non_overlay",
		sessionHash,
		"gpt-5.1",
		TargetGroupAny,
		nil,
		OpenAIUpstreamTransportAny,
	)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	require.Equal(t, activeReserve.ID, selection.Account.ID)
	require.Equal(t, openAIAccountScheduleLayerPreviousResponse, decision.Layer)
	require.True(t, decision.StickyPreviousHit)
	require.Equal(t, "active", requireDecisionStringField(t, decision, "SelectedGroup"))
	require.Equal(t, activeReserve.ID, cache.sessionBindings["openai:"+sessionHash])
	accountID, getErr := store.GetResponseAccount(ctx, groupID, "resp_prev_any_non_overlay")
	require.NoError(t, getErr)
	require.Equal(t, activeReserve.ID, accountID)
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}

func TestPreviousResponseNonOverlayReserveCandidateStillAcceptedForAny(t *testing.T) {
	ctx := context.Background()
	groupID := int64(13171)
	sessionHash := "session_hash_prev_projection_any_non_overlay"
	responseID := "resp_prev_projection_any_non_overlay"
	exhaustedBase := newOpenAIExhaustedAccountForTest(39350, 1)
	exhaustedBase.Credentials["model_mapping"] = map[string]any{"gpt-4.1": "gpt-4.1"}
	activeReserve := newOpenAIReserveCandidateAccountForTest(39351, 1, 20)
	activeReserve.Credentials["model_mapping"] = map[string]any{"gpt-5.1": "gpt-5.1"}
	activeAccount := Account{ID: 39352, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1, Credentials: map[string]any{"model_mapping": map[string]any{"gpt-5.1": "gpt-5.1"}}}
	snapshotCache := &openAISnapshotCacheStub{
		snapshotAccounts: []*Account{&exhaustedBase, &activeReserve, &activeAccount},
		accountsByID: map[int64]*Account{
			exhaustedBase.ID: &exhaustedBase,
			activeReserve.ID: &activeReserve,
			activeAccount.ID: &activeAccount,
		},
		openAIState: newOpenAIBucketStateForTest([]Account{exhaustedBase, activeReserve, activeAccount}, 8, map[string]OpenAIModelRoleView{
			"gpt-5.1": {
				CanonicalModel: "gpt-5.1",
			},
		}),
	}
	cache := &stubGatewayCache{sessionBindings: map[string]int64{}}
	cfg := newOpenAIWSV2TestConfig()
	cfg.Gateway.OpenAIWS.LBTopK = 1
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.Load = 1
	svc := &OpenAIGatewayService{
		accountRepo:        stubOpenAIAccountRepo{accounts: []Account{exhaustedBase, activeReserve, activeAccount}},
		cache:              cache,
		cfg:                cfg,
		schedulerSnapshot:  &SchedulerSnapshotService{cache: snapshotCache},
		concurrencyService: NewConcurrencyService(stubConcurrencyCache{loadMap: map[int64]*AccountLoadInfo{39350: {AccountID: 39350, LoadRate: 90}, 39351: {AccountID: 39351, LoadRate: 40}, 39352: {AccountID: 39352, LoadRate: 10}}}),
	}
	store := svc.getOpenAIWSStateStore()
	require.NoError(t, store.BindResponseAccount(ctx, groupID, responseID, activeReserve.ID, time.Hour))

	selection, decision, err := svc.SelectAccountWithScheduler(
		ctx,
		&groupID,
		responseID,
		sessionHash,
		"gpt-5.1",
		TargetGroupAny,
		nil,
		OpenAIUpstreamTransportAny,
	)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	require.Equal(t, activeReserve.ID, selection.Account.ID)
	require.Equal(t, openAIAccountScheduleLayerPreviousResponse, decision.Layer)
	require.True(t, decision.StickyPreviousHit)
	require.Equal(t, string(TargetGroupActive), requireDecisionStringField(t, decision, "SelectedGroup"))
	require.Equal(t, activeReserve.ID, cache.sessionBindings["openai:"+sessionHash])
	accountID, getErr := store.GetResponseAccount(ctx, groupID, responseID)
	require.NoError(t, getErr)
	require.Equal(t, activeReserve.ID, accountID)
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}

func TestOpenAIGatewayService_SelectAccountWithScheduler_PreviousResponseReserveAcceptedForActive(t *testing.T) {
	ctx := context.Background()
	groupID := int64(1314)
	sessionHash := "session_hash_previous_response_active_reserve"
	exhaustedBase := Account{
		ID:          3917,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Extra: map[string]any{
			"quota_limit": float64(100),
			"quota_used":  float64(100),
			"openai_apikey_responses_websockets_v2_enabled": true,
		},
	}
	overlayReserve := newOpenAIReserveCandidateAccountForTest(3915, 1, 80)
	activeReserve := newOpenAIReserveCandidateAccountForTest(3916, 1, 20)
	activeAccount := Account{ID: 3918, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1}
	cache := &stubGatewayCache{sessionBindings: map[string]int64{}}
	cfg := newOpenAIWSV2TestConfig()
	cfg.Gateway.OpenAIWS.LBTopK = 1
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.Load = 1
	snapshotCache := &openAISnapshotCacheStub{
		snapshotAccounts: []*Account{&exhaustedBase, &overlayReserve, &activeReserve, &activeAccount},
		accountsByID:     map[int64]*Account{3917: &exhaustedBase, 3915: &overlayReserve, 3916: &activeReserve, 3918: &activeAccount},
		openAIState: newOpenAIBucketStateForTest([]Account{exhaustedBase, overlayReserve, activeReserve, activeAccount}, 11, map[string]OpenAIModelRoleView{
			"gpt-5.1": {
				CanonicalModel:     "gpt-5.1",
				ExhaustedBaseIDs:   []int64{exhaustedBase.ID},
				ReserveOverflowIDs: []int64{overlayReserve.ID},
			},
		}),
	}
	svc := &OpenAIGatewayService{
		accountRepo:        stubOpenAIAccountRepo{accounts: []Account{exhaustedBase, overlayReserve, activeReserve, activeAccount}},
		cache:              cache,
		cfg:                cfg,
		schedulerSnapshot:  &SchedulerSnapshotService{cache: snapshotCache},
		concurrencyService: NewConcurrencyService(stubConcurrencyCache{loadMap: map[int64]*AccountLoadInfo{3917: {AccountID: 3917, LoadRate: 90}, 3915: {AccountID: 3915, LoadRate: 0}, 3916: {AccountID: 3916, LoadRate: 40}, 3918: {AccountID: 3918, LoadRate: 10}}}),
	}
	store := svc.getOpenAIWSStateStore()
	require.NoError(t, store.BindResponseAccount(ctx, groupID, "resp_prev_active_reserve", overlayReserve.ID, time.Hour))

	selection, decision, err := svc.SelectAccountWithScheduler(
		ctx,
		&groupID,
		"resp_prev_active_reserve",
		sessionHash,
		"gpt-5.1",
		TargetGroupActive,
		nil,
		OpenAIUpstreamTransportAny,
	)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	require.Equal(t, overlayReserve.ID, selection.Account.ID)
	require.Equal(t, openAIAccountScheduleLayerPreviousResponse, decision.Layer)
	require.True(t, decision.StickyPreviousHit)
	require.Equal(t, openAISelectedGroupReserve, requireDecisionStringField(t, decision, "SelectedGroup"))
	require.Equal(t, overlayReserve.ID, cache.sessionBindings["openai:"+sessionHash])
	accountID, getErr := store.GetResponseAccount(ctx, groupID, "resp_prev_active_reserve")
	require.NoError(t, getErr)
	require.Equal(t, overlayReserve.ID, accountID)
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}

func TestOpenAIGatewayService_SelectAccountWithScheduler_PreviousResponseNonOverlayReserveCandidateAcceptedForActive(t *testing.T) {
	ctx := context.Background()
	groupID := int64(1316)
	sessionHash := "session_hash_previous_response_active_non_overlay"
	exhaustedBase := Account{
		ID:          3924,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Extra: map[string]any{
			"quota_limit": float64(100),
			"quota_used":  float64(100),
			"openai_apikey_responses_websockets_v2_enabled": true,
		},
	}
	overlayReserve := newOpenAIReserveCandidateAccountForTest(3925, 1, 80)
	activeReserve := newOpenAIReserveCandidateAccountForTest(3926, 1, 20)
	activeAccount := Account{ID: 3927, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1}
	cache := &stubGatewayCache{sessionBindings: map[string]int64{}}
	cfg := newOpenAIWSV2TestConfig()
	cfg.Gateway.OpenAIWS.LBTopK = 1
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.Load = 1
	svc := &OpenAIGatewayService{
		accountRepo:        stubOpenAIAccountRepo{accounts: []Account{exhaustedBase, overlayReserve, activeReserve, activeAccount}},
		cache:              cache,
		cfg:                cfg,
		concurrencyService: NewConcurrencyService(stubConcurrencyCache{loadMap: map[int64]*AccountLoadInfo{3924: {AccountID: 3924, LoadRate: 90}, 3925: {AccountID: 3925, LoadRate: 0}, 3926: {AccountID: 3926, LoadRate: 40}, 3927: {AccountID: 3927, LoadRate: 10}}}),
	}
	store := svc.getOpenAIWSStateStore()
	require.NoError(t, store.BindResponseAccount(ctx, groupID, "resp_prev_active_non_overlay", activeReserve.ID, time.Hour))

	selection, decision, err := svc.SelectAccountWithScheduler(
		ctx,
		&groupID,
		"resp_prev_active_non_overlay",
		sessionHash,
		"gpt-5.1",
		TargetGroupActive,
		nil,
		OpenAIUpstreamTransportAny,
	)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	require.Equal(t, activeReserve.ID, selection.Account.ID)
	require.Equal(t, openAIAccountScheduleLayerPreviousResponse, decision.Layer)
	require.True(t, decision.StickyPreviousHit)
	require.Equal(t, "active", requireDecisionStringField(t, decision, "SelectedGroup"))
	require.Equal(t, activeReserve.ID, cache.sessionBindings["openai:"+sessionHash])
	accountID, getErr := store.GetResponseAccount(ctx, groupID, "resp_prev_active_non_overlay")
	require.NoError(t, getErr)
	require.Equal(t, activeReserve.ID, accountID)
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}

func TestOpenAIGatewayService_SelectAccountWithScheduler_PreviousResponseReserveProjectionMissFallsBackForAny(t *testing.T) {
	ctx := context.Background()
	groupID := int64(13131)
	sessionHash := "session_hash_previous_response_any_reserve_projection_miss"
	exhaustedBase := Account{
		ID:          3941,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Extra: map[string]any{
			"quota_limit": float64(100),
			"quota_used":  float64(100),
			"openai_apikey_responses_websockets_v2_enabled": true,
		},
	}
	overlayReserve := newOpenAIReserveCandidateAccountForTest(3942, 1, 80)
	activeReserve := newOpenAIReserveCandidateAccountForTest(3943, 1, 20)
	activeAccount := Account{ID: 3944, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1}
	cache := &stubGatewayCache{sessionBindings: map[string]int64{}}
	cfg := newOpenAIWSV2TestConfig()
	cfg.Gateway.OpenAIWS.LBTopK = 1
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.Load = 1
	snapshotCache := &openAISnapshotCacheStub{
		snapshotAccounts: []*Account{&exhaustedBase, &overlayReserve, &activeReserve, &activeAccount},
		accountsByID:     map[int64]*Account{3941: &exhaustedBase, 3942: &overlayReserve, 3943: &activeReserve, 3944: &activeAccount},
		openAIStateMiss:  true,
	}
	svc := &OpenAIGatewayService{
		accountRepo:        stubOpenAIAccountRepo{accounts: []Account{exhaustedBase, overlayReserve, activeReserve, activeAccount}},
		cache:              cache,
		cfg:                cfg,
		schedulerSnapshot:  &SchedulerSnapshotService{cache: snapshotCache},
		concurrencyService: NewConcurrencyService(stubConcurrencyCache{loadMap: map[int64]*AccountLoadInfo{3941: {AccountID: 3941, LoadRate: 90}, 3942: {AccountID: 3942, LoadRate: 0}, 3943: {AccountID: 3943, LoadRate: 40}, 3944: {AccountID: 3944, LoadRate: 10}}}),
	}
	store := svc.getOpenAIWSStateStore()
	require.NoError(t, store.BindResponseAccount(ctx, groupID, "resp_prev_any_reserve_projection_miss", overlayReserve.ID, time.Hour))

	selection, decision, err := svc.SelectAccountWithScheduler(ctx, &groupID, "resp_prev_any_reserve_projection_miss", sessionHash, "gpt-5.1", TargetGroupAny, nil, OpenAIUpstreamTransportAny)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	require.Equal(t, activeAccount.ID, selection.Account.ID)
	require.Equal(t, openAIAccountScheduleLayerLoadBalance, decision.Layer)
	require.False(t, decision.StickyPreviousHit)
	require.Equal(t, string(TargetGroupActive), requireDecisionStringField(t, decision, "SelectedGroup"))
	require.Equal(t, activeAccount.ID, cache.sessionBindings["openai:"+sessionHash])
	accountID, getErr := store.GetResponseAccount(ctx, groupID, "resp_prev_any_reserve_projection_miss")
	require.NoError(t, getErr)
	require.Zero(t, accountID)
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}

func TestOpenAIGatewayService_SelectAccountWithScheduler_PreviousResponseNonOverlayReserveCandidateAcceptedForAny_ProjectionMissFallsBackToLegacyOverlay(t *testing.T) {
	ctx := context.Background()
	groupID := int64(13151)
	sessionHash := "session_hash_previous_response_any_non_overlay_projection_miss"
	exhaustedBase := Account{
		ID:          3951,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Extra: map[string]any{
			"quota_limit": float64(100),
			"quota_used":  float64(100),
			"openai_apikey_responses_websockets_v2_enabled": true,
		},
	}
	overlayReserve := newOpenAIReserveCandidateAccountForTest(3952, 1, 80)
	activeReserve := newOpenAIReserveCandidateAccountForTest(3953, 1, 20)
	activeAccount := Account{ID: 3954, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1}
	cache := &stubGatewayCache{sessionBindings: map[string]int64{}}
	cfg := newOpenAIWSV2TestConfig()
	cfg.Gateway.OpenAIWS.LBTopK = 1
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.Load = 1
	snapshotCache := &openAISnapshotCacheStub{
		snapshotAccounts: []*Account{&exhaustedBase, &overlayReserve, &activeReserve, &activeAccount},
		accountsByID:     map[int64]*Account{3951: &exhaustedBase, 3952: &overlayReserve, 3953: &activeReserve, 3954: &activeAccount},
		openAIStateMiss:  true,
	}
	svc := &OpenAIGatewayService{
		accountRepo:        stubOpenAIAccountRepo{accounts: []Account{exhaustedBase, overlayReserve, activeReserve, activeAccount}},
		cache:              cache,
		cfg:                cfg,
		schedulerSnapshot:  &SchedulerSnapshotService{cache: snapshotCache},
		concurrencyService: NewConcurrencyService(stubConcurrencyCache{loadMap: map[int64]*AccountLoadInfo{3951: {AccountID: 3951, LoadRate: 90}, 3952: {AccountID: 3952, LoadRate: 0}, 3953: {AccountID: 3953, LoadRate: 40}, 3954: {AccountID: 3954, LoadRate: 10}}}),
	}
	store := svc.getOpenAIWSStateStore()
	require.NoError(t, store.BindResponseAccount(ctx, groupID, "resp_prev_any_non_overlay_projection_miss", activeReserve.ID, time.Hour))

	selection, decision, err := svc.SelectAccountWithScheduler(ctx, &groupID, "resp_prev_any_non_overlay_projection_miss", sessionHash, "gpt-5.1", TargetGroupAny, nil, OpenAIUpstreamTransportAny)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	require.Equal(t, activeReserve.ID, selection.Account.ID)
	require.Equal(t, openAIAccountScheduleLayerPreviousResponse, decision.Layer)
	require.True(t, decision.StickyPreviousHit)
	require.Equal(t, string(TargetGroupActive), requireDecisionStringField(t, decision, "SelectedGroup"))
	require.Equal(t, activeReserve.ID, cache.sessionBindings["openai:"+sessionHash])
	accountID, getErr := store.GetResponseAccount(ctx, groupID, "resp_prev_any_non_overlay_projection_miss")
	require.NoError(t, getErr)
	require.Equal(t, activeReserve.ID, accountID)
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}

func TestOpenAIGatewayService_SelectAccountWithScheduler_PreviousResponseReserveProjectionMissFallsBackForActive(t *testing.T) {
	ctx := context.Background()
	groupID := int64(13141)
	sessionHash := "session_hash_previous_response_active_reserve_projection_miss"
	exhaustedBase := Account{
		ID:          3961,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Extra: map[string]any{
			"quota_limit": float64(100),
			"quota_used":  float64(100),
			"openai_apikey_responses_websockets_v2_enabled": true,
		},
	}
	overlayReserve := newOpenAIReserveCandidateAccountForTest(3962, 1, 80)
	activeReserve := newOpenAIReserveCandidateAccountForTest(3963, 1, 20)
	activeAccount := Account{ID: 3964, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1}
	cache := &stubGatewayCache{sessionBindings: map[string]int64{}}
	cfg := newOpenAIWSV2TestConfig()
	cfg.Gateway.OpenAIWS.LBTopK = 1
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.Load = 1
	snapshotCache := &openAISnapshotCacheStub{
		snapshotAccounts: []*Account{&exhaustedBase, &overlayReserve, &activeReserve, &activeAccount},
		accountsByID:     map[int64]*Account{3961: &exhaustedBase, 3962: &overlayReserve, 3963: &activeReserve, 3964: &activeAccount},
		openAIStateMiss:  true,
	}
	svc := &OpenAIGatewayService{
		accountRepo:        stubOpenAIAccountRepo{accounts: []Account{exhaustedBase, overlayReserve, activeReserve, activeAccount}},
		cache:              cache,
		cfg:                cfg,
		schedulerSnapshot:  &SchedulerSnapshotService{cache: snapshotCache},
		concurrencyService: NewConcurrencyService(stubConcurrencyCache{loadMap: map[int64]*AccountLoadInfo{3961: {AccountID: 3961, LoadRate: 90}, 3962: {AccountID: 3962, LoadRate: 0}, 3963: {AccountID: 3963, LoadRate: 40}, 3964: {AccountID: 3964, LoadRate: 10}}}),
	}
	store := svc.getOpenAIWSStateStore()
	require.NoError(t, store.BindResponseAccount(ctx, groupID, "resp_prev_active_reserve_projection_miss", overlayReserve.ID, time.Hour))

	selection, decision, err := svc.SelectAccountWithScheduler(ctx, &groupID, "resp_prev_active_reserve_projection_miss", sessionHash, "gpt-5.1", TargetGroupActive, nil, OpenAIUpstreamTransportAny)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	require.Equal(t, activeAccount.ID, selection.Account.ID)
	require.Equal(t, openAIAccountScheduleLayerLoadBalance, decision.Layer)
	require.False(t, decision.StickyPreviousHit)
	require.Equal(t, string(TargetGroupActive), requireDecisionStringField(t, decision, "SelectedGroup"))
	require.Equal(t, activeAccount.ID, cache.sessionBindings["openai:"+sessionHash])
	accountID, getErr := store.GetResponseAccount(ctx, groupID, "resp_prev_active_reserve_projection_miss")
	require.NoError(t, getErr)
	require.Zero(t, accountID)
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}

func TestOpenAIGatewayService_SelectAccountWithScheduler_PreviousResponseNonOverlayReserveCandidateAcceptedForActive_ProjectionMissFallsBackToLegacyOverlay(t *testing.T) {
	ctx := context.Background()
	groupID := int64(13161)
	sessionHash := "session_hash_previous_response_active_non_overlay_projection_miss"
	exhaustedBase := Account{
		ID:          3971,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Extra: map[string]any{
			"quota_limit": float64(100),
			"quota_used":  float64(100),
			"openai_apikey_responses_websockets_v2_enabled": true,
		},
	}
	overlayReserve := newOpenAIReserveCandidateAccountForTest(3972, 1, 80)
	activeReserve := newOpenAIReserveCandidateAccountForTest(3973, 1, 20)
	activeAccount := Account{ID: 3974, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1}
	cache := &stubGatewayCache{sessionBindings: map[string]int64{}}
	cfg := newOpenAIWSV2TestConfig()
	cfg.Gateway.OpenAIWS.LBTopK = 1
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.Load = 1
	snapshotCache := &openAISnapshotCacheStub{
		snapshotAccounts: []*Account{&exhaustedBase, &overlayReserve, &activeReserve, &activeAccount},
		accountsByID:     map[int64]*Account{3971: &exhaustedBase, 3972: &overlayReserve, 3973: &activeReserve, 3974: &activeAccount},
		openAIStateMiss:  true,
	}
	svc := &OpenAIGatewayService{
		accountRepo:        stubOpenAIAccountRepo{accounts: []Account{exhaustedBase, overlayReserve, activeReserve, activeAccount}},
		cache:              cache,
		cfg:                cfg,
		schedulerSnapshot:  &SchedulerSnapshotService{cache: snapshotCache},
		concurrencyService: NewConcurrencyService(stubConcurrencyCache{loadMap: map[int64]*AccountLoadInfo{3971: {AccountID: 3971, LoadRate: 90}, 3972: {AccountID: 3972, LoadRate: 0}, 3973: {AccountID: 3973, LoadRate: 40}, 3974: {AccountID: 3974, LoadRate: 10}}}),
	}
	store := svc.getOpenAIWSStateStore()
	require.NoError(t, store.BindResponseAccount(ctx, groupID, "resp_prev_active_non_overlay_projection_miss", activeReserve.ID, time.Hour))

	selection, decision, err := svc.SelectAccountWithScheduler(ctx, &groupID, "resp_prev_active_non_overlay_projection_miss", sessionHash, "gpt-5.1", TargetGroupActive, nil, OpenAIUpstreamTransportAny)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	require.Equal(t, activeReserve.ID, selection.Account.ID)
	require.Equal(t, openAIAccountScheduleLayerPreviousResponse, decision.Layer)
	require.True(t, decision.StickyPreviousHit)
	require.Equal(t, string(TargetGroupActive), requireDecisionStringField(t, decision, "SelectedGroup"))
	require.Equal(t, activeReserve.ID, cache.sessionBindings["openai:"+sessionHash])
	accountID, getErr := store.GetResponseAccount(ctx, groupID, "resp_prev_active_non_overlay_projection_miss")
	require.NoError(t, getErr)
	require.Equal(t, activeReserve.ID, accountID)
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}

func TestOpenAIGatewayService_SelectAccountWithScheduler_ReserveSharedStickyBindingRejectedForAny_ProjectionMissFallsBackToLegacyOverlay(t *testing.T) {
	ctx := context.Background()
	groupID := int64(13123)
	sessionHash := "session_hash_any_reserve_binding_projection_miss"
	accounts := []Account{
		newOpenAIExhaustedAccountForTest(3981, 4),
		newOpenAIReserveCandidateAccountForTest(3982, 4, 20),
		{ID: 3983, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 4},
	}
	loadMap := map[int64]*AccountLoadInfo{3981: {AccountID: 3981, CurrentConcurrency: 3, LoadRate: 90}, 3982: {AccountID: 3982, CurrentConcurrency: 0, LoadRate: 0}, 3983: {AccountID: 3983, CurrentConcurrency: 0, LoadRate: 10}}
	cache := &stubGatewayCache{sessionBindings: map[string]int64{"openai:" + sessionHash: 3982}}
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.LBTopK = 1
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.Load = 1
	snapshotCache := &openAISnapshotCacheStub{snapshotAccounts: []*Account{&accounts[0], &accounts[1], &accounts[2]}, accountsByID: map[int64]*Account{3981: &accounts[0], 3982: &accounts[1], 3983: &accounts[2]}, openAIStateMiss: true}
	svc := &OpenAIGatewayService{accountRepo: stubOpenAIAccountRepo{accounts: accounts}, cache: cache, cfg: cfg, schedulerSnapshot: &SchedulerSnapshotService{cache: snapshotCache}, concurrencyService: NewConcurrencyService(stubConcurrencyCache{loadMap: loadMap})}

	selection, decision, err := svc.SelectAccountWithScheduler(ctx, &groupID, "", sessionHash, "gpt-5.1", TargetGroupAny, nil, OpenAIUpstreamTransportAny)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	require.Equal(t, int64(3983), selection.Account.ID)
	require.Equal(t, openAIAccountScheduleLayerLoadBalance, decision.Layer)
	require.False(t, decision.StickySessionHit)
	require.Equal(t, 1, cache.deletedSessions["openai:"+sessionHash])
	require.Equal(t, int64(3983), cache.sessionBindings["openai:"+sessionHash])
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}

func TestOpenAIGatewayService_SelectAccountWithScheduler_ReserveSharedStickyBindingRejectedForActive_ProjectionMissFallsBackToLegacyOverlay(t *testing.T) {
	ctx := context.Background()
	groupID := int64(13124)
	sessionHash := "session_hash_active_reserve_binding_projection_miss"
	accounts := []Account{
		newOpenAIExhaustedAccountForTest(3991, 4),
		newOpenAIReserveCandidateAccountForTest(3992, 4, 20),
		{ID: 3993, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 4},
	}
	loadMap := map[int64]*AccountLoadInfo{3991: {AccountID: 3991, CurrentConcurrency: 3, LoadRate: 90}, 3992: {AccountID: 3992, CurrentConcurrency: 0, LoadRate: 0}, 3993: {AccountID: 3993, CurrentConcurrency: 0, LoadRate: 10}}
	cache := &stubGatewayCache{sessionBindings: map[string]int64{"openai:" + sessionHash: 3992}}
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.LBTopK = 1
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.Load = 1
	snapshotCache := &openAISnapshotCacheStub{snapshotAccounts: []*Account{&accounts[0], &accounts[1], &accounts[2]}, accountsByID: map[int64]*Account{3991: &accounts[0], 3992: &accounts[1], 3993: &accounts[2]}, openAIStateMiss: true}
	svc := &OpenAIGatewayService{accountRepo: stubOpenAIAccountRepo{accounts: accounts}, cache: cache, cfg: cfg, schedulerSnapshot: &SchedulerSnapshotService{cache: snapshotCache}, concurrencyService: NewConcurrencyService(stubConcurrencyCache{loadMap: loadMap})}

	selection, decision, err := svc.SelectAccountWithScheduler(ctx, &groupID, "", sessionHash, "gpt-5.1", TargetGroupActive, nil, OpenAIUpstreamTransportAny)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	require.Equal(t, int64(3993), selection.Account.ID)
	require.Equal(t, openAIAccountScheduleLayerLoadBalance, decision.Layer)
	require.False(t, decision.StickySessionHit)
	require.Equal(t, 1, cache.deletedSessions["openai:"+sessionHash])
	require.Equal(t, int64(3993), cache.sessionBindings["openai:"+sessionHash])
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}

func TestPreviousResponseNonOverlayReserveCandidateStillAcceptedForActive(t *testing.T) {
	ctx := context.Background()
	groupID := int64(13172)
	sessionHash := "session_hash_prev_projection_active_non_overlay"
	responseID := "resp_prev_projection_active_non_overlay"
	exhaustedBase := newOpenAIExhaustedAccountForTest(39360, 1)
	exhaustedBase.Credentials["model_mapping"] = map[string]any{"gpt-4.1": "gpt-4.1"}
	activeReserve := newOpenAIReserveCandidateAccountForTest(39361, 1, 20)
	activeReserve.Credentials["model_mapping"] = map[string]any{"gpt-5.1": "gpt-5.1"}
	activeAccount := Account{ID: 39362, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1, Credentials: map[string]any{"model_mapping": map[string]any{"gpt-5.1": "gpt-5.1"}}}
	snapshotCache := &openAISnapshotCacheStub{
		snapshotAccounts: []*Account{&exhaustedBase, &activeReserve, &activeAccount},
		accountsByID: map[int64]*Account{
			exhaustedBase.ID: &exhaustedBase,
			activeReserve.ID: &activeReserve,
			activeAccount.ID: &activeAccount,
		},
		openAIState: newOpenAIBucketStateForTest([]Account{exhaustedBase, activeReserve, activeAccount}, 9, map[string]OpenAIModelRoleView{
			"gpt-5.1": {
				CanonicalModel: "gpt-5.1",
			},
		}),
	}
	cache := &stubGatewayCache{sessionBindings: map[string]int64{}}
	cfg := newOpenAIWSV2TestConfig()
	cfg.Gateway.OpenAIWS.LBTopK = 1
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.Load = 1
	svc := &OpenAIGatewayService{
		accountRepo:        stubOpenAIAccountRepo{accounts: []Account{exhaustedBase, activeReserve, activeAccount}},
		cache:              cache,
		cfg:                cfg,
		schedulerSnapshot:  &SchedulerSnapshotService{cache: snapshotCache},
		concurrencyService: NewConcurrencyService(stubConcurrencyCache{loadMap: map[int64]*AccountLoadInfo{39360: {AccountID: 39360, LoadRate: 90}, 39361: {AccountID: 39361, LoadRate: 40}, 39362: {AccountID: 39362, LoadRate: 10}}}),
	}
	store := svc.getOpenAIWSStateStore()
	require.NoError(t, store.BindResponseAccount(ctx, groupID, responseID, activeReserve.ID, time.Hour))

	selection, decision, err := svc.SelectAccountWithScheduler(
		ctx,
		&groupID,
		responseID,
		sessionHash,
		"gpt-5.1",
		TargetGroupActive,
		nil,
		OpenAIUpstreamTransportAny,
	)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	require.Equal(t, activeReserve.ID, selection.Account.ID)
	require.Equal(t, openAIAccountScheduleLayerPreviousResponse, decision.Layer)
	require.True(t, decision.StickyPreviousHit)
	require.Equal(t, string(TargetGroupActive), requireDecisionStringField(t, decision, "SelectedGroup"))
	require.Equal(t, activeReserve.ID, cache.sessionBindings["openai:"+sessionHash])
	accountID, getErr := store.GetResponseAccount(ctx, groupID, responseID)
	require.NoError(t, getErr)
	require.Equal(t, activeReserve.ID, accountID)
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}

func TestOpenAIGatewayService_SelectAccountWithScheduler_PreviousResponseNonReserveExhaustedKeepsSharedSticky(t *testing.T) {
	ctx := context.Background()
	groupID := int64(1310)
	sessionHash := "session_hash_previous_response_non_reserve"
	exhaustedAccount := Account{
		ID:          3902,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Extra: map[string]any{
			"quota_limit": float64(100),
			"quota_used":  float64(100),
			"openai_apikey_responses_websockets_v2_enabled": true,
		},
	}
	cache := &stubGatewayCache{sessionBindings: map[string]int64{}}
	cfg := newOpenAIWSV2TestConfig()
	svc := &OpenAIGatewayService{
		accountRepo:        stubOpenAIAccountRepo{accounts: []Account{exhaustedAccount}},
		cache:              cache,
		cfg:                cfg,
		concurrencyService: NewConcurrencyService(stubConcurrencyCache{}),
	}
	store := svc.getOpenAIWSStateStore()
	require.NoError(t, store.BindResponseAccount(ctx, groupID, "resp_prev_non_reserve", exhaustedAccount.ID, time.Hour))

	selection, decision, err := svc.SelectAccountWithScheduler(
		ctx,
		&groupID,
		"resp_prev_non_reserve",
		sessionHash,
		"gpt-5.1",
		TargetGroupExhausted,
		nil,
		OpenAIUpstreamTransportAny,
	)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	require.Equal(t, exhaustedAccount.ID, selection.Account.ID)
	require.Equal(t, openAIAccountScheduleLayerPreviousResponse, decision.Layer)
	require.True(t, decision.StickyPreviousHit)
	require.Equal(t, "exhausted", requireDecisionStringField(t, decision, "SelectedGroup"))
	require.Equal(t, exhaustedAccount.ID, cache.sessionBindings["openai:"+sessionHash])
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}

func TestOpenAIGatewayService_SelectAccountWithScheduler_ReserveSharedStickyBindingRejectedForAny(t *testing.T) {
	ctx := context.Background()
	groupID := int64(1311)
	sessionHash := "session_hash_any_reserve_binding"
	accounts := []Account{
		newOpenAIExhaustedAccountForTest(3911, 4),
		newOpenAIReserveCandidateAccountForTest(3912, 4, 20),
		{
			ID:          3913,
			Platform:    PlatformOpenAI,
			Type:        AccountTypeAPIKey,
			Status:      StatusActive,
			Schedulable: true,
			Concurrency: 4,
		},
	}
	loadMap := map[int64]*AccountLoadInfo{
		3911: {AccountID: 3911, CurrentConcurrency: 3, LoadRate: 90},
		3912: {AccountID: 3912, CurrentConcurrency: 0, LoadRate: 0},
		3913: {AccountID: 3913, CurrentConcurrency: 0, LoadRate: 10},
	}
	cache := &stubGatewayCache{sessionBindings: map[string]int64{"openai:" + sessionHash: 3912}}
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.LBTopK = 1
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.Load = 1
	svc := &OpenAIGatewayService{
		accountRepo:        stubOpenAIAccountRepo{accounts: accounts},
		cache:              cache,
		cfg:                cfg,
		concurrencyService: NewConcurrencyService(stubConcurrencyCache{loadMap: loadMap}),
	}

	selection, decision, err := svc.SelectAccountWithScheduler(
		ctx,
		&groupID,
		"",
		sessionHash,
		"gpt-5.1",
		TargetGroupAny,
		nil,
		OpenAIUpstreamTransportAny,
	)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	require.Equal(t, int64(3913), selection.Account.ID)
	require.Equal(t, openAIAccountScheduleLayerLoadBalance, decision.Layer)
	require.False(t, decision.StickySessionHit)
	require.Equal(t, 1, cache.deletedSessions["openai:"+sessionHash])
	require.Equal(t, int64(3913), cache.sessionBindings["openai:"+sessionHash])
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}

func TestOpenAIGatewayService_SelectAccountWithScheduler_ReserveSharedStickyBindingRejectedForActive(t *testing.T) {
	ctx := context.Background()
	groupID := int64(1312)
	sessionHash := "session_hash_active_reserve_binding"
	accounts := []Account{
		newOpenAIExhaustedAccountForTest(3921, 4),
		newOpenAIReserveCandidateAccountForTest(3922, 4, 20),
		{
			ID:          3923,
			Platform:    PlatformOpenAI,
			Type:        AccountTypeAPIKey,
			Status:      StatusActive,
			Schedulable: true,
			Concurrency: 4,
		},
	}
	loadMap := map[int64]*AccountLoadInfo{
		3921: {AccountID: 3921, CurrentConcurrency: 3, LoadRate: 90},
		3922: {AccountID: 3922, CurrentConcurrency: 0, LoadRate: 0},
		3923: {AccountID: 3923, CurrentConcurrency: 0, LoadRate: 10},
	}
	cache := &stubGatewayCache{sessionBindings: map[string]int64{"openai:" + sessionHash: 3922}}
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.LBTopK = 1
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.Load = 1
	svc := &OpenAIGatewayService{
		accountRepo:        stubOpenAIAccountRepo{accounts: accounts},
		cache:              cache,
		cfg:                cfg,
		concurrencyService: NewConcurrencyService(stubConcurrencyCache{loadMap: loadMap}),
	}

	selection, decision, err := svc.SelectAccountWithScheduler(
		ctx,
		&groupID,
		"",
		sessionHash,
		"gpt-5.1",
		TargetGroupActive,
		nil,
		OpenAIUpstreamTransportAny,
	)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	require.Equal(t, int64(3923), selection.Account.ID)
	require.Equal(t, openAIAccountScheduleLayerLoadBalance, decision.Layer)
	require.False(t, decision.StickySessionHit)
	require.Equal(t, 1, cache.deletedSessions["openai:"+sessionHash])
	require.Equal(t, int64(3923), cache.sessionBindings["openai:"+sessionHash])
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}

func TestOpenAIGatewayService_OpenAIAccountSchedulerMetrics(t *testing.T) {
	ctx := context.Background()
	groupID := int64(12)
	account := Account{
		ID:          4001,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
	}
	cache := &stubGatewayCache{
		sessionBindings: map[string]int64{
			"openai:session_hash_metrics": account.ID,
		},
	}
	svc := &OpenAIGatewayService{
		accountRepo:        stubOpenAIAccountRepo{accounts: []Account{account}},
		cache:              cache,
		cfg:                &config.Config{},
		concurrencyService: NewConcurrencyService(stubConcurrencyCache{}),
	}

	selection, _, err := svc.SelectAccountWithScheduler(ctx, &groupID, "", "session_hash_metrics", "gpt-5.1", TargetGroupAny, nil, OpenAIUpstreamTransportAny)
	require.NoError(t, err)
	require.NotNil(t, selection)
	svc.ReportOpenAIAccountScheduleResult(account.ID, true, intPtrForTest(120))
	svc.RecordOpenAIAccountSwitch()

	snapshot := svc.SnapshotOpenAIAccountSchedulerMetrics()
	require.GreaterOrEqual(t, snapshot.SelectTotal, int64(1))
	require.GreaterOrEqual(t, snapshot.StickySessionHitTotal, int64(1))
	require.GreaterOrEqual(t, snapshot.AccountSwitchTotal, int64(1))
	require.GreaterOrEqual(t, snapshot.SchedulerLatencyMsAvg, float64(0))
	require.GreaterOrEqual(t, snapshot.StickyHitRatio, 0.0)
	require.GreaterOrEqual(t, snapshot.RuntimeStatsAccountCount, 1)
}

func intPtrForTest(v int) *int {
	return &v
}

func TestOpenAIAccountRuntimeStats_ReportAndSnapshot(t *testing.T) {
	stats := newOpenAIAccountRuntimeStats()
	stats.report(1001, true, nil)
	firstTTFT := 100
	stats.report(1001, false, &firstTTFT)
	secondTTFT := 200
	stats.report(1001, false, &secondTTFT)

	errorRate, ttft, hasTTFT := stats.snapshot(1001)
	require.True(t, hasTTFT)
	require.InDelta(t, 0.36, errorRate, 1e-9)
	require.InDelta(t, 120.0, ttft, 1e-9)
	require.Equal(t, 1, stats.size())
}

func TestOpenAIAccountRuntimeStats_ReportConcurrent(t *testing.T) {
	stats := newOpenAIAccountRuntimeStats()

	const (
		accountCount = 4
		workers      = 16
		iterations   = 800
	)
	var wg sync.WaitGroup
	wg.Add(workers)
	for worker := 0; worker < workers; worker++ {
		worker := worker
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				accountID := int64(i%accountCount + 1)
				success := (i+worker)%3 != 0
				ttft := 80 + (i+worker)%40
				stats.report(accountID, success, &ttft)
			}
		}()
	}
	wg.Wait()

	require.Equal(t, accountCount, stats.size())
	for accountID := int64(1); accountID <= accountCount; accountID++ {
		errorRate, ttft, hasTTFT := stats.snapshot(accountID)
		require.GreaterOrEqual(t, errorRate, 0.0)
		require.LessOrEqual(t, errorRate, 1.0)
		require.True(t, hasTTFT)
		require.Greater(t, ttft, 0.0)
	}
}

func TestSelectTopKOpenAICandidates(t *testing.T) {
	candidates := []openAIAccountCandidateScore{
		{
			account:  &Account{ID: 11, Priority: 2},
			loadInfo: &AccountLoadInfo{LoadRate: 10, WaitingCount: 1},
			score:    10.0,
		},
		{
			account:  &Account{ID: 12, Priority: 1},
			loadInfo: &AccountLoadInfo{LoadRate: 20, WaitingCount: 1},
			score:    9.5,
		},
		{
			account:  &Account{ID: 13, Priority: 1},
			loadInfo: &AccountLoadInfo{LoadRate: 30, WaitingCount: 0},
			score:    10.0,
		},
		{
			account:  &Account{ID: 14, Priority: 0},
			loadInfo: &AccountLoadInfo{LoadRate: 40, WaitingCount: 0},
			score:    8.0,
		},
	}

	top2 := selectTopKOpenAICandidates(candidates, 2)
	require.Len(t, top2, 2)
	require.Equal(t, int64(13), top2[0].account.ID)
	require.Equal(t, int64(11), top2[1].account.ID)

	topAll := selectTopKOpenAICandidates(candidates, 8)
	require.Len(t, topAll, len(candidates))
	require.Equal(t, int64(13), topAll[0].account.ID)
	require.Equal(t, int64(11), topAll[1].account.ID)
	require.Equal(t, int64(12), topAll[2].account.ID)
	require.Equal(t, int64(14), topAll[3].account.ID)
}

func TestBuildOpenAIWeightedSelectionOrder_DeterministicBySessionSeed(t *testing.T) {
	candidates := []openAIAccountCandidateScore{
		{
			account:  &Account{ID: 101},
			loadInfo: &AccountLoadInfo{LoadRate: 10, WaitingCount: 0},
			score:    4.2,
		},
		{
			account:  &Account{ID: 102},
			loadInfo: &AccountLoadInfo{LoadRate: 30, WaitingCount: 1},
			score:    3.5,
		},
		{
			account:  &Account{ID: 103},
			loadInfo: &AccountLoadInfo{LoadRate: 50, WaitingCount: 2},
			score:    2.1,
		},
	}
	req := OpenAIAccountScheduleRequest{
		GroupID:        int64PtrForTest(99),
		SessionHash:    "session_seed_fixed",
		RequestedModel: "gpt-5.1",
	}

	first := buildOpenAIWeightedSelectionOrder(candidates, req)
	second := buildOpenAIWeightedSelectionOrder(candidates, req)
	require.Len(t, first, len(candidates))
	require.Len(t, second, len(candidates))
	for i := range first {
		require.Equal(t, first[i].account.ID, second[i].account.ID)
	}
}

func TestOpenAIGatewayService_SelectAccountWithScheduler_LoadBalanceDistributesAcrossSessions(t *testing.T) {
	ctx := context.Background()
	groupID := int64(15)
	accounts := []Account{
		{
			ID:          5101,
			Platform:    PlatformOpenAI,
			Type:        AccountTypeAPIKey,
			Status:      StatusActive,
			Schedulable: true,
			Concurrency: 3,
			Priority:    0,
		},
		{
			ID:          5102,
			Platform:    PlatformOpenAI,
			Type:        AccountTypeAPIKey,
			Status:      StatusActive,
			Schedulable: true,
			Concurrency: 3,
			Priority:    0,
		},
		{
			ID:          5103,
			Platform:    PlatformOpenAI,
			Type:        AccountTypeAPIKey,
			Status:      StatusActive,
			Schedulable: true,
			Concurrency: 3,
			Priority:    0,
		},
	}
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.LBTopK = 3
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.Priority = 1
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.Load = 1
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.Queue = 1
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.ErrorRate = 1
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.TTFT = 1

	concurrencyCache := stubConcurrencyCache{
		loadMap: map[int64]*AccountLoadInfo{
			5101: {AccountID: 5101, LoadRate: 20, WaitingCount: 1},
			5102: {AccountID: 5102, LoadRate: 20, WaitingCount: 1},
			5103: {AccountID: 5103, LoadRate: 20, WaitingCount: 1},
		},
	}
	svc := &OpenAIGatewayService{
		accountRepo:        stubOpenAIAccountRepo{accounts: accounts},
		cache:              &stubGatewayCache{sessionBindings: map[string]int64{}},
		cfg:                cfg,
		concurrencyService: NewConcurrencyService(concurrencyCache),
	}

	selected := make(map[int64]int, len(accounts))
	for i := 0; i < 60; i++ {
		sessionHash := fmt.Sprintf("session_hash_lb_%d", i)
		selection, decision, err := svc.SelectAccountWithScheduler(
			ctx,
			&groupID,
			"",
			sessionHash,
			"gpt-5.1",
			TargetGroupAny,
			nil,
			OpenAIUpstreamTransportAny,
		)
		require.NoError(t, err)
		require.NotNil(t, selection)
		require.NotNil(t, selection.Account)
		require.Equal(t, openAIAccountScheduleLayerLoadBalance, decision.Layer)
		selected[selection.Account.ID]++
		if selection.ReleaseFunc != nil {
			selection.ReleaseFunc()
		}
	}

	// 多 session 应该能打散到多个账号，避免“恒定单账号命中”。
	require.GreaterOrEqual(t, len(selected), 2)
}

func TestDeriveOpenAISelectionSeed_NoAffinityAddsEntropy(t *testing.T) {
	req := OpenAIAccountScheduleRequest{
		RequestedModel: "gpt-5.1",
	}
	seed1 := deriveOpenAISelectionSeed(req)
	time.Sleep(1 * time.Millisecond)
	seed2 := deriveOpenAISelectionSeed(req)
	require.NotZero(t, seed1)
	require.NotZero(t, seed2)
	require.NotEqual(t, seed1, seed2)
}

func TestBuildOpenAIWeightedSelectionOrder_HandlesInvalidScores(t *testing.T) {
	candidates := []openAIAccountCandidateScore{
		{
			account:  &Account{ID: 901},
			loadInfo: &AccountLoadInfo{LoadRate: 5, WaitingCount: 0},
			score:    math.NaN(),
		},
		{
			account:  &Account{ID: 902},
			loadInfo: &AccountLoadInfo{LoadRate: 5, WaitingCount: 0},
			score:    math.Inf(1),
		},
		{
			account:  &Account{ID: 903},
			loadInfo: &AccountLoadInfo{LoadRate: 5, WaitingCount: 0},
			score:    -1,
		},
	}
	req := OpenAIAccountScheduleRequest{
		SessionHash: "seed_invalid_scores",
	}

	order := buildOpenAIWeightedSelectionOrder(candidates, req)
	require.Len(t, order, len(candidates))
	seen := map[int64]struct{}{}
	for _, item := range order {
		seen[item.account.ID] = struct{}{}
	}
	require.Len(t, seen, len(candidates))
}

func TestOpenAISelectionRNG_SeedZeroStillWorks(t *testing.T) {
	rng := newOpenAISelectionRNG(0)
	v1 := rng.nextUint64()
	v2 := rng.nextUint64()
	require.NotEqual(t, v1, v2)
	require.GreaterOrEqual(t, rng.nextFloat64(), 0.0)
	require.Less(t, rng.nextFloat64(), 1.0)
}

func TestOpenAIAccountCandidateHeap_PushPopAndInvalidType(t *testing.T) {
	h := openAIAccountCandidateHeap{}
	h.Push(openAIAccountCandidateScore{
		account:  &Account{ID: 7001},
		loadInfo: &AccountLoadInfo{LoadRate: 0, WaitingCount: 0},
		score:    1.0,
	})
	require.Equal(t, 1, h.Len())
	popped, ok := h.Pop().(openAIAccountCandidateScore)
	require.True(t, ok)
	require.Equal(t, int64(7001), popped.account.ID)
	require.Equal(t, 0, h.Len())

	require.Panics(t, func() {
		h.Push("bad_element_type")
	})
}

func TestClamp01_AllBranches(t *testing.T) {
	require.Equal(t, 0.0, clamp01(-0.2))
	require.Equal(t, 1.0, clamp01(1.3))
	require.Equal(t, 0.5, clamp01(0.5))
}

func TestCalcLoadSkewByMoments_Branches(t *testing.T) {
	require.Equal(t, 0.0, calcLoadSkewByMoments(1, 1, 1))
	// variance < 0 分支：sumSquares/count - mean^2 为负值时应钳制为 0。
	require.Equal(t, 0.0, calcLoadSkewByMoments(1, 0, 2))
	require.GreaterOrEqual(t, calcLoadSkewByMoments(6, 20, 3), 0.0)
}

func TestDefaultOpenAIAccountScheduler_ReportSwitchAndSnapshot(t *testing.T) {
	schedulerAny := newDefaultOpenAIAccountScheduler(&OpenAIGatewayService{}, nil)
	scheduler, ok := schedulerAny.(*defaultOpenAIAccountScheduler)
	require.True(t, ok)

	ttft := 100
	scheduler.ReportResult(1001, true, &ttft)
	scheduler.ReportSwitch()
	scheduler.metrics.recordSelect(OpenAIAccountScheduleDecision{
		Layer:             openAIAccountScheduleLayerLoadBalance,
		LatencyMs:         8,
		LoadSkew:          0.5,
		StickyPreviousHit: true,
	})
	scheduler.metrics.recordSelect(OpenAIAccountScheduleDecision{
		Layer:            openAIAccountScheduleLayerSessionSticky,
		LatencyMs:        6,
		LoadSkew:         0.2,
		StickySessionHit: true,
	})

	snapshot := scheduler.SnapshotMetrics()
	require.Equal(t, int64(2), snapshot.SelectTotal)
	require.Equal(t, int64(1), snapshot.StickyPreviousHitTotal)
	require.Equal(t, int64(1), snapshot.StickySessionHitTotal)
	require.Equal(t, int64(1), snapshot.LoadBalanceSelectTotal)
	require.Equal(t, int64(1), snapshot.AccountSwitchTotal)
	require.Greater(t, snapshot.SchedulerLatencyMsAvg, 0.0)
	require.Greater(t, snapshot.StickyHitRatio, 0.0)
	require.Greater(t, snapshot.LoadSkewAvg, 0.0)
}

func TestOpenAIGatewayService_SchedulerWrappersAndDefaults(t *testing.T) {
	svc := &OpenAIGatewayService{}
	ttft := 120
	svc.ReportOpenAIAccountScheduleResult(10, true, &ttft)
	svc.RecordOpenAIAccountSwitch()
	snapshot := svc.SnapshotOpenAIAccountSchedulerMetrics()
	require.GreaterOrEqual(t, snapshot.AccountSwitchTotal, int64(1))
	require.Equal(t, 7, svc.openAIWSLBTopK())
	require.Equal(t, openaiStickySessionTTL, svc.openAIWSSessionStickyTTL())

	defaultWeights := svc.openAIWSSchedulerWeights()
	require.Equal(t, 1.0, defaultWeights.Priority)
	require.Equal(t, 1.0, defaultWeights.Load)
	require.Equal(t, 0.7, defaultWeights.Queue)
	require.Equal(t, 0.8, defaultWeights.ErrorRate)
	require.Equal(t, 0.5, defaultWeights.TTFT)

	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.LBTopK = 9
	cfg.Gateway.OpenAIWS.StickySessionTTLSeconds = 180
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.Priority = 0.2
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.Load = 0.3
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.Queue = 0.4
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.ErrorRate = 0.5
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.TTFT = 0.6
	svcWithCfg := &OpenAIGatewayService{cfg: cfg}

	require.Equal(t, 9, svcWithCfg.openAIWSLBTopK())
	require.Equal(t, 180*time.Second, svcWithCfg.openAIWSSessionStickyTTL())
	customWeights := svcWithCfg.openAIWSSchedulerWeights()
	require.Equal(t, 0.2, customWeights.Priority)
	require.Equal(t, 0.3, customWeights.Load)
	require.Equal(t, 0.4, customWeights.Queue)
	require.Equal(t, 0.5, customWeights.ErrorRate)
	require.Equal(t, 0.6, customWeights.TTFT)
}

func TestDefaultOpenAIAccountScheduler_IsAccountTransportCompatible_Branches(t *testing.T) {
	scheduler := &defaultOpenAIAccountScheduler{}
	require.True(t, scheduler.isAccountTransportCompatible(nil, OpenAIUpstreamTransportAny))
	require.True(t, scheduler.isAccountTransportCompatible(nil, OpenAIUpstreamTransportHTTPSSE))
	require.False(t, scheduler.isAccountTransportCompatible(nil, OpenAIUpstreamTransportResponsesWebsocketV2))

	cfg := newOpenAIWSV2TestConfig()
	scheduler.service = &OpenAIGatewayService{cfg: cfg}
	account := &Account{
		ID:          8801,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Extra: map[string]any{
			"openai_apikey_responses_websockets_v2_enabled": true,
		},
	}
	require.True(t, scheduler.isAccountTransportCompatible(account, OpenAIUpstreamTransportResponsesWebsocketV2))
}

func int64PtrForTest(v int64) *int64 {
	return &v
}

func newOpenAIReserveSelectionServiceForTest(accounts []Account, loadMap map[int64]*AccountLoadInfo) *OpenAIGatewayService {
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.LBTopK = 1
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.Load = 1
	exhaustedAccounts := make([]Account, 0, len(accounts))
	activeAccounts := make([]Account, 0, len(accounts))
	accountPtrs := make([]*Account, 0, len(accounts))
	accountsByID := make(map[int64]*Account, len(accounts))
	for i := range accounts {
		cloned := accounts[i]
		accountPtrs = append(accountPtrs, &cloned)
		accountsByID[cloned.ID] = &cloned
		if cloned.MatchesTargetGroup(TargetGroupExhausted) {
			exhaustedAccounts = append(exhaustedAccounts, cloned)
			continue
		}
		activeAccounts = append(activeAccounts, cloned)
	}
	projectionState := newOpenAIBucketStateForTest(accounts, 1, map[string]OpenAIModelRoleView{
		"gpt-5.1": {
			CanonicalModel:     "gpt-5.1",
			ExhaustedBaseIDs:   sortedOpenAIProjectionIDs(exhaustedAccounts),
			ReserveOverflowIDs: sortedOpenAIProjectionIDs(buildOpenAIReserveOverflowPool(activeAccounts, exhaustedAccounts)),
		},
	})
	return &OpenAIGatewayService{
		accountRepo:        stubOpenAIAccountRepo{accounts: accounts},
		cache:              &stubGatewayCache{sessionBindings: map[string]int64{}},
		cfg:                cfg,
		schedulerSnapshot:  &SchedulerSnapshotService{cache: &openAISnapshotCacheStub{snapshotAccounts: accountPtrs, accountsByID: accountsByID, openAIState: projectionState}},
		concurrencyService: NewConcurrencyService(stubConcurrencyCache{loadMap: loadMap}),
	}
}

func newOpenAIBucketStateForTest(accounts []Account, projectionVersion int64, models map[string]OpenAIModelRoleView) *OpenAISchedulerBucketState {
	accountPtrs := make([]*Account, 0, len(accounts))
	for i := range accounts {
		cloned := accounts[i]
		accountPtrs = append(accountPtrs, &cloned)
	}
	projectionModels := make(map[string]OpenAIModelRoleView, len(models))
	accountReserveIDs := make(map[int64]struct{})
	for model, view := range models {
		canonical := NormalizeOpenAIProjectionModelKey(model)
		if canonical == "" {
			continue
		}
		view.CanonicalModel = canonical
		projectionModels[canonical] = view
		for _, accountID := range view.ReserveOverflowIDs {
			accountReserveIDs[accountID] = struct{}{}
		}
	}
	return &OpenAISchedulerBucketState{
		Accounts:           accountPtrs,
		ProjectionAccounts: append([]*Account(nil), accountPtrs...),
		Projection: &OpenAIModelSubsetProjection{
			Bucket:            SchedulerBucket{Platform: PlatformOpenAI, Mode: SchedulerModeSingle},
			AccountReserveIDs: accountReserveIDs,
			Models:            projectionModels,
		},
		ProjectionVersion: projectionVersion,
		BuiltAt:           time.Unix(1_716_000_000, 0).UTC(),
	}
}

func newOpenAIExhaustedAccountForTest(id int64, concurrency int) Account {
	return Account{
		ID:          id,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: concurrency,
		Credentials: map[string]any{"plan_type": "free"},
		Extra: map[string]any{
			"codex_7d_used_percent": 100.0,
		},
	}
}

func newOpenAIReserveCandidateAccountForTest(id int64, concurrency int, usedPercent float64) Account {
	return Account{
		ID:          id,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: concurrency,
		Credentials: map[string]any{"plan_type": "free"},
		Extra: map[string]any{
			"codex_7d_used_percent":                        usedPercent,
			"openai_oauth_responses_websockets_v2_enabled": true,
		},
	}
}

type openAIAffinityGatewayCacheRecord struct {
	value     string
	expiresAt time.Time
}

type openAIAffinityGatewayCacheStub struct {
	*stubGatewayCache
	mu                sync.Mutex
	sessionExpiries   map[string]time.Time
	sessionRefreshes  map[string]int
	affinityBindings  map[string]openAIAffinityGatewayCacheRecord
	affinityRefreshes map[string]int
	affinityDeletes   map[string]int
}

func newOpenAIAffinityGatewayCacheStub() *openAIAffinityGatewayCacheStub {
	return &openAIAffinityGatewayCacheStub{
		stubGatewayCache:  &stubGatewayCache{sessionBindings: map[string]int64{}, deletedSessions: map[string]int{}},
		sessionExpiries:   make(map[string]time.Time),
		sessionRefreshes:  make(map[string]int),
		affinityBindings:  make(map[string]openAIAffinityGatewayCacheRecord),
		affinityRefreshes: make(map[string]int),
		affinityDeletes:   make(map[string]int),
	}
}

func (c *openAIAffinityGatewayCacheStub) SetSessionAccountID(ctx context.Context, groupID int64, sessionHash string, accountID int64, ttl time.Duration) error {
	if err := c.stubGatewayCache.SetSessionAccountID(ctx, groupID, sessionHash, accountID, ttl); err != nil {
		return err
	}
	c.mu.Lock()
	c.sessionExpiries[sessionHash] = time.Now().Add(ttl)
	c.mu.Unlock()
	return nil
}

func (c *openAIAffinityGatewayCacheStub) RefreshSessionTTL(ctx context.Context, groupID int64, sessionHash string, ttl time.Duration) error {
	if err := c.stubGatewayCache.RefreshSessionTTL(ctx, groupID, sessionHash, ttl); err != nil {
		return err
	}
	c.mu.Lock()
	c.sessionRefreshes[sessionHash]++
	c.sessionExpiries[sessionHash] = time.Now().Add(ttl)
	c.mu.Unlock()
	return nil
}

func (c *openAIAffinityGatewayCacheStub) DeleteSessionAccountID(ctx context.Context, groupID int64, sessionHash string) error {
	if err := c.stubGatewayCache.DeleteSessionAccountID(ctx, groupID, sessionHash); err != nil {
		return err
	}
	c.mu.Lock()
	delete(c.sessionExpiries, sessionHash)
	c.mu.Unlock()
	return nil
}

func (c *openAIAffinityGatewayCacheStub) GetOpenAICompanionBinding(ctx context.Context, groupID int64, namespace string, bindingKey string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	record, ok := c.affinityBindings[c.affinityKey(groupID, namespace, bindingKey)]
	if !ok || time.Now().After(record.expiresAt) {
		return "", fmt.Errorf("not found")
	}
	return record.value, nil
}

func (c *openAIAffinityGatewayCacheStub) SetOpenAICompanionBinding(ctx context.Context, groupID int64, namespace string, bindingKey string, value string, ttl time.Duration) error {
	c.mu.Lock()
	c.affinityBindings[c.affinityKey(groupID, namespace, bindingKey)] = openAIAffinityGatewayCacheRecord{
		value:     value,
		expiresAt: time.Now().Add(ttl),
	}
	c.mu.Unlock()
	return nil
}

func (c *openAIAffinityGatewayCacheStub) RefreshOpenAICompanionBindingTTL(ctx context.Context, groupID int64, namespace string, bindingKey string, ttl time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	key := c.affinityKey(groupID, namespace, bindingKey)
	record, ok := c.affinityBindings[key]
	if !ok {
		return fmt.Errorf("not found")
	}
	record.expiresAt = time.Now().Add(ttl)
	c.affinityBindings[key] = record
	c.affinityRefreshes[key]++
	return nil
}

func (c *openAIAffinityGatewayCacheStub) DeleteOpenAICompanionBinding(ctx context.Context, groupID int64, namespace string, bindingKey string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	key := c.affinityKey(groupID, namespace, bindingKey)
	delete(c.affinityBindings, key)
	c.affinityDeletes[key]++
	return nil
}

func (c *openAIAffinityGatewayCacheStub) affinityKey(groupID int64, namespace string, bindingKey string) string {
	return fmt.Sprintf("%s:%d:%s", namespace, groupID, bindingKey)
}

func (c *openAIAffinityGatewayCacheStub) setAffinityBinding(t *testing.T, namespace string, groupID int64, bindingKey string, binding *openAIAffinityBinding, ttl time.Duration) {
	t.Helper()
	payload, err := json.Marshal(binding)
	require.NoError(t, err)
	require.NoError(t, c.SetOpenAICompanionBinding(context.Background(), groupID, namespace, bindingKey, string(payload), ttl))
}

func (c *openAIAffinityGatewayCacheStub) mustGetAffinityBinding(t *testing.T, namespace string, groupID int64, bindingKey string) *openAIAffinityBinding {
	t.Helper()
	payload, err := c.GetOpenAICompanionBinding(context.Background(), groupID, namespace, bindingKey)
	require.NoError(t, err)
	var binding openAIAffinityBinding
	require.NoError(t, json.Unmarshal([]byte(payload), &binding))
	return &binding
}

func (c *openAIAffinityGatewayCacheStub) hasAffinityBinding(namespace string, groupID int64, bindingKey string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	key := c.affinityKey(groupID, namespace, bindingKey)
	record, ok := c.affinityBindings[key]
	return ok && !time.Now().After(record.expiresAt)
}

func (c *openAIAffinityGatewayCacheStub) sessionExpiry(bindingKey string) time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.sessionExpiries[bindingKey]
}

func (c *openAIAffinityGatewayCacheStub) affinityExpiry(namespace string, groupID int64, bindingKey string) time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.affinityBindings[c.affinityKey(groupID, namespace, bindingKey)].expiresAt
}
