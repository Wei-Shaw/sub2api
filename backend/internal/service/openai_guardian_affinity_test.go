package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type guardianAffinityGroupRepo struct {
	GroupRepository
	group *Group
	err   error
REDACTED

type guardianAffinityAccountRepo struct {
	schedulerGroupAwareOpenAIAccountRepo
	setErrorCalls int
REDACTED

func (r *guardianAffinityAccountRepo) SetError(context.Context, int64, string) error {
	r.setErrorCalls++
	return nil
REDACTED

func (r guardianAffinityGroupRepo) GetByID(context.Context, int64) (*Group, error) {
	if r.err != nil {
		return nil, r.err
REDACTED
	return r.group, nil
REDACTED

func (r guardianAffinityGroupRepo) GetByIDLite(context.Context, int64) (*Group, error) {
	if r.err != nil {
		return nil, r.err
REDACTED
	return r.group, nil
REDACTED

func guardianAffinityTestContext(t *testing.T, model, subagent, parentHeader, metadata string) context.Context {
REDACTED
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)
	c.Request.Header.Set(openAISubagentHeader, subagent)
	if parentHeader != "" {
		c.Request.Header.Set(codexParentThreadIDHeader, parentHeader)
REDACTED
	if metadata != "" {
		c.Request.Header.Set(codexTurnMetadataHeader, metadata)
REDACTED
	return WithOpenAIGuardianParentAffinity(context.Background(), c, nil, model)
REDACTED

func TestWithOpenAIGuardianParentAffinity_RequiresUnambiguousReviewLineage(t *testing.T) {
	parentID := "11111111-1111-4111-8111-111111111111"
	wantHash := DeriveSessionHashFromSeed(parentID)

	for _, subagent := range []string{"guardian", "review", "GUARDIAN"REDACTED {
		t.Run(subagent, func(t *testing.T) {
			ctx := guardianAffinityTestContext(t, codexAutoReviewModel, subagent, parentID, `{"parent_thread_id":"`+parentID+`"REDACTED`)
			affinity, ok := openAIGuardianParentAffinityFromContext(ctx)
			require.True(t, ok)
			require.Equal(t, wantHash, affinity.currentSessionHash)
	REDACTED)
REDACTED

	t.Run("metadata only", func(t *testing.T) {
		ctx := guardianAffinityTestContext(t, codexAutoReviewModel, "guardian", "", `{"parent_thread_id":"`+parentID+`"REDACTED`)
		_, ok := openAIGuardianParentAffinityFromContext(ctx)
		require.True(t, ok)
REDACTED)

	t.Run("websocket envelope metadata", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodGet, "/openai/v1/responses", nil)
		body := []byte(`{"type":"response.create","response":{"model":"codex-auto-review","client_metadata":{"x-codex-turn-metadata":"{\"parent_thread_id\":\"` + parentID + `\",\"subagent_kind\":\"guardian\"REDACTED"REDACTEDREDACTEDREDACTED`)
		ctx := WithOpenAIGuardianParentAffinity(context.Background(), c, body, codexAutoReviewModel)
		affinity, ok := openAIGuardianParentAffinityFromContext(ctx)
		require.True(t, ok)
		require.Equal(t, wantHash, affinity.currentSessionHash)
REDACTED)

	for name, ctx := range map[string]context.Context{
		"ordinary model":       guardianAffinityTestContext(t, "gpt-5.6-sol", "guardian", parentID, ""),
		"ordinary subagent":    guardianAffinityTestContext(t, codexAutoReviewModel, "collab_spawn", parentID, ""),
		"missing parent":       guardianAffinityTestContext(t, codexAutoReviewModel, "guardian", "", ""),
		"conflicting lineage":  guardianAffinityTestContext(t, codexAutoReviewModel, "guardian", parentID, `{"parent_thread_id":"different-parent"REDACTED`),
		"conflicting subagent": guardianAffinityTestContext(t, codexAutoReviewModel, "guardian", parentID, `{"parent_thread_id":"`+parentID+`","subagent_kind":"collab_spawn"REDACTED`),
REDACTED {
		t.Run(name, func(t *testing.T) {
			_, ok := openAIGuardianParentAffinityFromContext(ctx)
			require.False(t, ok)
	REDACTED)
REDACTED
REDACTED

func TestOpenAIGatewayService_GuardianParentAffinitySelectsParentAccountAcrossSchedulers(t *testing.T) {
	parentID := "22222222-2222-4222-8222-222222222222"
	parentHash := DeriveSessionHashFromSeed(parentID)
	groupID := int64(102001)

	for _, mode := range []struct {
		name           string
		advanced       string
		stickyWeighted string
REDACTED{
		{name: "legacy", advanced: "false"REDACTED,
		{name: "advanced", advanced: "true"REDACTED,
		{name: "advanced sticky weighted", advanced: "true", stickyWeighted: "true"REDACTED,
REDACTED {
		t.Run(mode.name, func(t *testing.T) {
			accounts := []Account{
				{
					ID: 39001, Platform: PlatformOpenAI, Type: AccountTypeOAuth,
					Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 10,
					GroupIDs: []int64{groupIDREDACTED, Credentials: map[string]any{"plan_type": "team"REDACTED,
			REDACTED,
				{
					ID: 39002, Platform: PlatformOpenAI, Type: AccountTypeOAuth,
					Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 0,
					GroupIDs: []int64{groupIDREDACTED, Credentials: map[string]any{"plan_type": "team"REDACTED,
			REDACTED,
		REDACTED
			cfg := &config.Config{REDACTED
			cfg.Gateway.OpenAIWS.LBTopK = 2
			cache := &schedulerTestGatewayCache{sessionBindings: map[string]int64{"openai:" + parentHash: 39001REDACTEDREDACTED
			svc := &OpenAIGatewayService{
				accountRepo:        schedulerGroupAwareOpenAIAccountRepo{schedulerTestOpenAIAccountRepo{accounts: accountsREDACTEDREDACTED,
				cache:              cache,
				cfg:                cfg,
				rateLimitService:   newOpenAIAdvancedSchedulerRateLimitService(mode.advanced, mode.stickyWeighted),
				concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{acquireResults: map[int64]bool{39001: true, 39002: trueREDACTEDREDACTED),
		REDACTED

			ctx := guardianAffinityTestContext(t, codexAutoReviewModel, "guardian", parentID, "")
			selection, decision, err := svc.SelectAccountWithScheduler(
				ctx, &groupID, "", "guardian-child-session", codexAutoReviewModel,
				nil, OpenAIUpstreamTransportAny, false,
			)
		REDACTED
			require.NotNil(t, selection)
			require.Equal(t, int64(39001), selection.Account.ID)
			require.Equal(t, openAIAccountScheduleLayerGuardianParent, decision.Layer)
			require.Zero(t, cache.deletedSessions["openai:"+parentHash])
			if selection.ReleaseFunc != nil {
				selection.ReleaseFunc()
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestOpenAIGatewayService_GuardianParentAffinityFallsBackWithoutCrossGroupOrFailoverBypass(t *testing.T) {
	parentID := "33333333-3333-4333-8333-333333333333"
	parentHash := DeriveSessionHashFromSeed(parentID)
	groupID := int64(102011)
	otherGroupID := int64(102012)

	for name, excluded := range map[string]map[int64]struct{REDACTED{
		"parent moved out of group":              nil,
		"parent excluded after upstream failure": {39011: {REDACTEDREDACTED,
REDACTED {
		t.Run(name, func(t *testing.T) {
			parentGroups := []int64{groupIDREDACTED
			if excluded == nil {
				parentGroups = []int64{otherGroupIDREDACTED
		REDACTED
			accounts := []Account{
				{ID: 39011, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 0, GroupIDs: parentGroups, Credentials: map[string]any{"plan_type": "team"REDACTEDREDACTED,
				{ID: 39012, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 5, GroupIDs: []int64{groupIDREDACTED, Credentials: map[string]any{"plan_type": "team"REDACTEDREDACTED,
		REDACTED
			cache := &schedulerTestGatewayCache{sessionBindings: map[string]int64{"openai:" + parentHash: 39011REDACTEDREDACTED
			svc := &OpenAIGatewayService{
				accountRepo:        schedulerGroupAwareOpenAIAccountRepo{schedulerTestOpenAIAccountRepo{accounts: accountsREDACTEDREDACTED,
				cache:              cache,
				cfg:                &config.Config{REDACTED,
				rateLimitService:   newOpenAIAdvancedSchedulerRateLimitService("true"),
				concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{acquireResults: map[int64]bool{39011: true, 39012: trueREDACTEDREDACTED),
		REDACTED

			ctx := guardianAffinityTestContext(t, codexAutoReviewModel, "guardian", parentID, "")
			selection, _, err := svc.SelectAccountWithScheduler(
				ctx, &groupID, "", "guardian-fallback-child", codexAutoReviewModel,
				excluded, OpenAIUpstreamTransportAny, false,
			)
		REDACTED
			require.NotNil(t, selection)
			require.Equal(t, int64(39012), selection.Account.ID)
			require.Zero(t, cache.deletedSessions["openai:"+parentHash], "a child request must never delete its parent's binding")
			if selection.ReleaseFunc != nil {
				selection.ReleaseFunc()
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestOpenAIGatewayService_GuardianParentHashCollisionPreservesParentBinding(t *testing.T) {
	parentID := "44444444-4444-4444-8444-444444444444"
	parentHash := DeriveSessionHashFromSeed(parentID)
	groupID := int64(102021)
	otherGroupID := int64(102022)

	for _, advanced := range []string{"false", "true"REDACTED {
		t.Run(map[string]string{"false": "legacy", "true": "advanced"REDACTED[advanced], func(t *testing.T) {
			accounts := []Account{
				{ID: 39021, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 0, GroupIDs: []int64{otherGroupIDREDACTED, Credentials: map[string]any{"plan_type": "team"REDACTEDREDACTED,
				{ID: 39022, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 5, GroupIDs: []int64{groupIDREDACTED, Credentials: map[string]any{"plan_type": "team"REDACTEDREDACTED,
		REDACTED
			cache := &schedulerTestGatewayCache{sessionBindings: map[string]int64{"openai:" + parentHash: 39021REDACTEDREDACTED
			svc := &OpenAIGatewayService{
				accountRepo:        schedulerGroupAwareOpenAIAccountRepo{schedulerTestOpenAIAccountRepo{accounts: accountsREDACTEDREDACTED,
				cache:              cache,
				cfg:                &config.Config{REDACTED,
				rateLimitService:   newOpenAIAdvancedSchedulerRateLimitService(advanced),
				concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{acquireResults: map[int64]bool{39021: true, 39022: trueREDACTEDREDACTED),
		REDACTED

			ctx := guardianAffinityTestContext(t, codexAutoReviewModel, "guardian", parentID, "")
			selection, _, err := svc.SelectAccountWithScheduler(
				ctx, &groupID, "", parentHash, codexAutoReviewModel,
				nil, OpenAIUpstreamTransportAny, false,
			)
		REDACTED
			require.NotNil(t, selection)
			require.Equal(t, int64(39022), selection.Account.ID)
			require.NoError(t, svc.BindStickySessionAfterProfitAdmission(ctx, &groupID, parentHash, selection.Account.ID))
			require.Equal(t, int64(39021), cache.sessionBindings["openai:"+parentHash])
			require.Zero(t, cache.deletedSessions["openai:"+parentHash])
			if selection.ReleaseFunc != nil {
				selection.ReleaseFunc()
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestOpenAIGatewayService_GuardianParentAffinityHonorsRequiredPrivacy(t *testing.T) {
	parentID := "55555555-5555-4555-8555-555555555555"
	parentHash := DeriveSessionHashFromSeed(parentID)
	groupID := int64(102031)

	for _, advanced := range []string{"false", "true"REDACTED {
		t.Run(map[string]string{"false": "legacy", "true": "advanced"REDACTED[advanced], func(t *testing.T) {
			accounts := []Account{
				{ID: 39031, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 0, GroupIDs: []int64{groupIDREDACTED, Credentials: map[string]any{"plan_type": "team"REDACTEDREDACTED,
				{ID: 39032, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 5, GroupIDs: []int64{groupIDREDACTED, Credentials: map[string]any{"plan_type": "team"REDACTED, Extra: map[string]any{"privacy_mode": PrivacyModeTrainingOffREDACTEDREDACTED,
		REDACTED
			repo := &guardianAffinityAccountRepo{schedulerGroupAwareOpenAIAccountRepo: schedulerGroupAwareOpenAIAccountRepo{schedulerTestOpenAIAccountRepo{accounts: accountsREDACTEDREDACTEDREDACTED
			cache := &schedulerTestGatewayCache{sessionBindings: map[string]int64{"openai:" + parentHash: 39031REDACTEDREDACTED
			svc := &OpenAIGatewayService{
				accountRepo:        repo,
				cache:              cache,
				cfg:                &config.Config{REDACTED,
				rateLimitService:   newOpenAIAdvancedSchedulerRateLimitService(advanced),
				concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{acquireResults: map[int64]bool{39031: true, 39032: trueREDACTEDREDACTED),
				schedulerSnapshot: &SchedulerSnapshotService{
					accountRepo: repo,
					groupRepo:   guardianAffinityGroupRepo{group: &Group{ID: groupID, Name: "privacy", RequirePrivacySet: trueREDACTEDREDACTED,
			REDACTED,
		REDACTED

			ctx := guardianAffinityTestContext(t, codexAutoReviewModel, "guardian", parentID, "")
			selection, _, err := svc.SelectAccountWithScheduler(
				ctx, &groupID, "", "guardian-privacy-child", codexAutoReviewModel,
				nil, OpenAIUpstreamTransportAny, false,
			)
		REDACTED
			require.NotNil(t, selection)
			require.Equal(t, int64(39032), selection.Account.ID)
			require.Zero(t, repo.setErrorCalls, "a group-scoped privacy gate must not globally error a shared account")
			require.False(t, svc.isOpenAIAccountRequestRuntimeBlocked(&accounts[0], codexAutoReviewModel))
			if selection.ReleaseFunc != nil {
				selection.ReleaseFunc()
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestOpenAIGatewayService_PreviousResponseHonorsGroupAndRequiredPrivacy(t *testing.T) {
	groupID := int64(3904)

	tests := []struct {
		name         string
		boundAccount Account
		groupErr     error
REDACTED{
		{
			name: "privacy unset",
			boundAccount: Account{
				ID: 39041, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
				Status: StatusActive, Schedulable: true, Concurrency: 1,
				GroupIDs: []int64{groupIDREDACTED,
				Extra:    map[string]any{"openai_apikey_responses_websockets_v2_enabled": trueREDACTED,
		REDACTED,
	REDACTED,
		{
			name: "privacy policy lookup error fails closed",
			boundAccount: Account{
				ID: 39041, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
				Status: StatusActive, Schedulable: true, Concurrency: 1,
				GroupIDs: []int64{groupIDREDACTED,
				Extra:    map[string]any{"openai_apikey_responses_websockets_v2_enabled": trueREDACTED,
		REDACTED,
			groupErr: errors.New("group repository unavailable"),
	REDACTED,
		{
			name: "different group",
			boundAccount: Account{
				ID: 39041, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
				Status: StatusActive, Schedulable: true, Concurrency: 1,
				GroupIDs: []int64{groupID + 1REDACTED,
				Extra: map[string]any{
					"openai_apikey_responses_websockets_v2_enabled": true,
					"privacy_mode": PrivacyModeTrainingOff,
			REDACTED,
		REDACTED,
	REDACTED,
REDACTED

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fallback := Account{
				ID: 39042, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
				Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 5,
				GroupIDs: []int64{groupIDREDACTED,
				Extra: map[string]any{
					"openai_apikey_responses_websockets_v2_enabled": true,
					"privacy_mode": PrivacyModeTrainingOff,
			REDACTED,
		REDACTED
			accounts := []Account{tc.boundAccount, fallbackREDACTED
			repo := &guardianAffinityAccountRepo{schedulerGroupAwareOpenAIAccountRepo: schedulerGroupAwareOpenAIAccountRepo{schedulerTestOpenAIAccountRepo{accounts: accountsREDACTEDREDACTEDREDACTED
			cache := &schedulerTestGatewayCache{REDACTED
			store := NewOpenAIWSStateStore(cache)
			groupRepo := guardianAffinityGroupRepo{
				group: &Group{
					ID: groupID, Name: "privacy-required", Platform: PlatformOpenAI,
					Status: StatusActive, RequirePrivacySet: true,
			REDACTED,
				err: tc.groupErr,
		REDACTED
			svc := &OpenAIGatewayService{
				accountRepo:        repo,
				cache:              cache,
				cfg:                &config.Config{REDACTED,
				rateLimitService:   newOpenAIAdvancedSchedulerRateLimitService("true"),
				concurrencyService: NewConcurrencyService(&schedulerTestConcurrencyCache{REDACTED),
				openaiWSStateStore: store,
				schedulerSnapshot: &SchedulerSnapshotService{
					accountRepo: repo,
					groupRepo:   groupRepo,
			REDACTED,
		REDACTED
			responseID := "resp_privacy_guard"
			require.NoError(t, store.BindResponseAccount(context.Background(), groupID, responseID, tc.boundAccount.ID, time.Hour))

			directSelection, directErr := svc.SelectAccountByPreviousResponseID(
				context.Background(), &groupID, responseID, codexAutoReviewModel, nil, false,
			)
			require.NoError(t, directErr)
			require.Nil(t, directSelection, "the previous-response helper must enforce fresh group/privacy state")

			selection, decision, err := svc.SelectAccountWithSchedulerForCapability(
				context.Background(), &groupID, responseID, "", codexAutoReviewModel,
				nil, OpenAIUpstreamTransportAny, OpenAIEndpointCapabilityResponses,
				false, false, true,
			)
		REDACTED
			require.NotNil(t, selection)
			require.Equal(t, fallback.ID, selection.Account.ID)
			require.NotEqual(t, openAIAccountScheduleLayerPreviousResponse, decision.Layer)
			require.Zero(t, repo.setErrorCalls)
			require.False(t, svc.isOpenAIAccountRequestRuntimeBlocked(&accounts[0], codexAutoReviewModel))
			boundAccountID, getErr := store.GetResponseAccount(context.Background(), groupID, responseID)
			require.NoError(t, getErr)
			require.Equal(t, tc.boundAccount.ID, boundAccountID, "transient policy misses must preserve the response binding")
			if selection.ReleaseFunc != nil {
				selection.ReleaseFunc()
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestOpenAIGatewayService_PreviousResponseSimpleModeIgnoresGroupMembership(t *testing.T) {
	groupID := int64(3905)
	bound := Account{
		ID: 39051, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
		Status: StatusActive, Schedulable: true, Concurrency: 1,
		GroupIDs: []int64{groupID + 1REDACTED,
		Extra:    map[string]any{"openai_apikey_responses_websockets_v2_enabled": trueREDACTED,
REDACTED
	fallback := Account{
		ID: 39052, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
		Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 10,
		GroupIDs: []int64{groupIDREDACTED,
		Extra:    map[string]any{"openai_apikey_responses_websockets_v2_enabled": trueREDACTED,
REDACTED
	accounts := []Account{bound, fallbackREDACTED
	repo := &guardianAffinityAccountRepo{schedulerGroupAwareOpenAIAccountRepo: schedulerGroupAwareOpenAIAccountRepo{schedulerTestOpenAIAccountRepo{accounts: accountsREDACTEDREDACTEDREDACTED
	cache := &schedulerTestGatewayCache{REDACTED
	store := NewOpenAIWSStateStore(cache)
	cfg := &config.Config{RunMode: config.RunModeSimpleREDACTED
	svc := &OpenAIGatewayService{
		accountRepo:        repo,
		cache:              cache,
		cfg:                cfg,
		rateLimitService:   newOpenAIAdvancedSchedulerRateLimitService("true"),
		concurrencyService: NewConcurrencyService(&schedulerTestConcurrencyCache{REDACTED),
		openaiWSStateStore: store,
		schedulerSnapshot: &SchedulerSnapshotService{
			accountRepo: repo,
			groupRepo: guardianAffinityGroupRepo{group: &Group{
				ID: groupID, Name: "simple-mode", Platform: PlatformOpenAI, Status: StatusActive,
	REDACTED
	REDACTED,
REDACTED
	responseID := "resp_simple_mode_cross_group"
	require.NoError(t, store.BindResponseAccount(context.Background(), groupID, responseID, bound.ID, time.Hour))

	directSelection, err := svc.SelectAccountByPreviousResponseID(
		context.Background(), &groupID, responseID, codexAutoReviewModel, nil, false,
	)
REDACTED
	require.NotNil(t, directSelection)
	require.Equal(t, bound.ID, directSelection.Account.ID)
	if directSelection.ReleaseFunc != nil {
		directSelection.ReleaseFunc()
REDACTED

	selection, decision, err := svc.SelectAccountWithSchedulerForCapability(
		context.Background(), &groupID, responseID, "", codexAutoReviewModel,
		nil, OpenAIUpstreamTransportAny, OpenAIEndpointCapabilityResponses,
		false, false, true,
	)
REDACTED
	require.NotNil(t, selection)
	require.Equal(t, bound.ID, selection.Account.ID)
	require.Equal(t, openAIAccountScheduleLayerPreviousResponse, decision.Layer)
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
REDACTED
REDACTED
