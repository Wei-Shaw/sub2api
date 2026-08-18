//go:build unit

package service

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"testing"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

type accountRepoStubForBulkUpdate struct {
	accountRepoStub
	bulkUpdateErr       error
	bulkUpdateIDs       []int64
	bulkUpdateCalls     int
	lastBulkUpdate      AccountBulkUpdate
	bindGroupErrByID    map[int64]error
	bindGroupsCalls     []int64
	bindGroupsByAccount map[int64][]int64
	createAccount       *Account
	createID            int64
	createErr           error
	updatedAccounts     []*Account
	updateErr           error
	getByIDsAccounts    []*Account
	getByIDsErr         error
	getByIDsCalled      bool
	getByIDsIDs         []int64
	getByIDAccounts     map[int64]*Account
	getByIDErrByID      map[int64]error
	getByIDCalled       []int64
	listByGroupData     map[int64][]Account
	listByGroupErr      map[int64]error
	listData            []Account
	listResult          *pagination.PaginationResult
	listErr             error
	listCalled          bool
	lastListParams      pagination.PaginationParams
	lastListFilters     struct {
		platform    string
		accountType string
		status      string
		search      string
		groupID     int64
		privacyMode string
REDACTED
REDACTED

func (s *accountRepoStubForBulkUpdate) BulkUpdate(_ context.Context, ids []int64, updates AccountBulkUpdate) (int64, error) {
	s.bulkUpdateCalls++
	s.bulkUpdateIDs = append([]int64{REDACTED, ids...)
	s.lastBulkUpdate = updates
	if s.bulkUpdateErr != nil {
		return 0, s.bulkUpdateErr
REDACTED
	return int64(len(ids)), nil
REDACTED

func requireApplicationErrorReason(t *testing.T, err error, reason string) {
REDACTED
	var appErr *infraerrors.ApplicationError
	require.ErrorAs(t, err, &appErr)
	require.Equal(t, reason, appErr.Reason)
REDACTED

func (s *accountRepoStubForBulkUpdate) Create(_ context.Context, account *Account) error {
	s.createAccount = account
	if s.createID > 0 {
		account.ID = s.createID
REDACTED
	return s.createErr
REDACTED

func (s *accountRepoStubForBulkUpdate) Update(_ context.Context, account *Account) error {
	s.updatedAccounts = append(s.updatedAccounts, account)
	return s.updateErr
REDACTED

func (s *accountRepoStubForBulkUpdate) BindGroups(_ context.Context, accountID int64, groupIDs []int64) error {
	s.bindGroupsCalls = append(s.bindGroupsCalls, accountID)
	if s.bindGroupsByAccount == nil {
		s.bindGroupsByAccount = make(map[int64][]int64)
REDACTED
	s.bindGroupsByAccount[accountID] = append([]int64{REDACTED, groupIDs...)
	if err, ok := s.bindGroupErrByID[accountID]; ok {
		return err
REDACTED
	return nil
REDACTED

func (s *accountRepoStubForBulkUpdate) GetByIDs(_ context.Context, ids []int64) ([]*Account, error) {
	s.getByIDsCalled = true
	s.getByIDsIDs = append([]int64{REDACTED, ids...)
	if s.getByIDsErr != nil {
		return nil, s.getByIDsErr
REDACTED
	return s.getByIDsAccounts, nil
REDACTED

func (s *accountRepoStubForBulkUpdate) GetByID(_ context.Context, id int64) (*Account, error) {
	s.getByIDCalled = append(s.getByIDCalled, id)
	if err, ok := s.getByIDErrByID[id]; ok {
		return nil, err
REDACTED
	if account, ok := s.getByIDAccounts[id]; ok {
		return account, nil
REDACTED
	return nil, errors.New("account not found")
REDACTED

func (s *accountRepoStubForBulkUpdate) ListByGroup(_ context.Context, groupID int64) ([]Account, error) {
	if err, ok := s.listByGroupErr[groupID]; ok {
		return nil, err
REDACTED
	if rows, ok := s.listByGroupData[groupID]; ok {
		return rows, nil
REDACTED
	return nil, nil
REDACTED

func (s *accountRepoStubForBulkUpdate) ListAllWithFilters(context.Context, string, string, string, string, int64, string) ([]Account, error) {
	return nil, nil
REDACTED

func (s *accountRepoStubForBulkUpdate) ListWithFilters(_ context.Context, params pagination.PaginationParams, platform, accountType, status, search string, groupID int64, privacyMode string) ([]Account, *pagination.PaginationResult, error) {
	s.listCalled = true
	s.lastListParams = params
	s.lastListFilters.platform = platform
	s.lastListFilters.accountType = accountType
	s.lastListFilters.status = status
	s.lastListFilters.search = search
	s.lastListFilters.groupID = groupID
	s.lastListFilters.privacyMode = privacyMode
	if s.listErr != nil {
		return nil, nil, s.listErr
REDACTED
	if s.listResult != nil {
		return s.listData, s.listResult, nil
REDACTED
	return s.listData, &pagination.PaginationResult{Total: int64(len(s.listData))REDACTED, nil
REDACTED

// TestAdminService_BulkUpdateAccounts_AllSuccessIDs 验证批量更新成功时返回 success_ids/failed_ids。
func TestAdminService_BulkUpdateAccounts_AllSuccessIDs(t *testing.T) {
	repo := &accountRepoStubForBulkUpdate{REDACTED
	svc := &adminServiceImpl{accountRepo: repoREDACTED

	schedulable := true
	input := &BulkUpdateAccountsInput{
		AccountIDs:  []int64{1, 2, 3REDACTED,
		Schedulable: &schedulable,
REDACTED

	result, err := svc.BulkUpdateAccounts(context.Background(), input)
REDACTED
	require.Equal(t, 3, result.Success)
	require.Equal(t, 0, result.Failed)
	require.ElementsMatch(t, []int64{1, 2, 3REDACTED, result.SuccessIDs)
	require.Empty(t, result.FailedIDs)
	require.Len(t, result.Results, 3)
REDACTED

func TestAdminService_BulkUpdateAccounts_RejectsRateChangeForSyncedAccounts(t *testing.T) {
	repo := &accountRepoStubForBulkUpdate{
		getByIDsAccounts: []*Account{
			{
				ID: 1,
				Extra: map[string]any{
					UpstreamBillingProbeEnabledExtraKey:    true,
					UpstreamBillingRateSyncEnabledExtraKey: true,
			REDACTED,
		REDACTED,
			{ID: 2, Extra: map[string]any{REDACTEDREDACTED,
	REDACTED,
REDACTED
	svc := &adminServiceImpl{accountRepo: repoREDACTED
	rateMultiplier := 0.5

	result, err := svc.BulkUpdateAccounts(context.Background(), &BulkUpdateAccountsInput{
		AccountIDs:     []int64{1, 2REDACTED,
		RateMultiplier: &rateMultiplier,
REDACTED)

	require.Nil(t, result)
REDACTED
	var appErr *infraerrors.ApplicationError
	require.ErrorAs(t, err, &appErr)
	require.Equal(t, int32(http.StatusConflict), appErr.Code)
	require.Equal(t, "UPSTREAM_BILLING_RATE_SYNC_BULK_CONFLICT", appErr.Reason)
	require.Equal(t, "1", appErr.Metadata["count"])
	require.True(t, repo.getByIDsCalled)
	require.Empty(t, repo.bulkUpdateIDs, "rate conflict must be rejected before any write")
REDACTED

// TestAdminService_BulkUpdateAccounts_PartialFailureIDs 验证部分失败时 success_ids/failed_ids 正确。
func TestAdminService_BulkUpdateAccounts_PartialFailureIDs(t *testing.T) {
	repo := &accountRepoStubForBulkUpdate{
		bindGroupErrByID: map[int64]error{
			2: errors.New("bind failed"),
	REDACTED,
REDACTED
	svc := &adminServiceImpl{
		accountRepo: repo,
		groupRepo:   &groupRepoStubForAdmin{getByID: &Group{ID: 10, Name: "g10"REDACTEDREDACTED,
REDACTED

	groupIDs := []int64{10REDACTED
	schedulable := false
	input := &BulkUpdateAccountsInput{
		AccountIDs:            []int64{1, 2, 3REDACTED,
		GroupIDs:              &groupIDs,
		Schedulable:           &schedulable,
		SkipMixedChannelCheck: true,
REDACTED

	result, err := svc.BulkUpdateAccounts(context.Background(), input)
REDACTED
	require.Equal(t, 2, result.Success)
	require.Equal(t, 1, result.Failed)
	require.ElementsMatch(t, []int64{1, 3REDACTED, result.SuccessIDs)
	require.ElementsMatch(t, []int64{2REDACTED, result.FailedIDs)
	require.Len(t, result.Results, 3)
REDACTED

func TestAdminService_BulkUpdateAccounts_NilGroupRepoReturnsError(t *testing.T) {
	repo := &accountRepoStubForBulkUpdate{REDACTED
	svc := &adminServiceImpl{accountRepo: repoREDACTED

	groupIDs := []int64{10REDACTED
	input := &BulkUpdateAccountsInput{
		AccountIDs: []int64{1REDACTED,
		GroupIDs:   &groupIDs,
REDACTED

	result, err := svc.BulkUpdateAccounts(context.Background(), input)
	require.Nil(t, result)
REDACTED
	require.Contains(t, err.Error(), "group repository not configured")
REDACTED

// TestAdminService_BulkUpdateAccounts_MixedChannelPreCheckBlocksOnExistingConflict verifies
// that the global pre-check detects a conflict with existing group members and returns an
// error before any DB write is performed.
func TestAdminService_BulkUpdateAccounts_MixedChannelPreCheckBlocksOnExistingConflict(t *testing.T) {
	repo := &accountRepoStubForBulkUpdate{
		getByIDsAccounts: []*Account{
			{ID: 1, Platform: PlatformAntigravityREDACTED,
	REDACTED,
		// Group 10 already contains an Anthropic account.
		listByGroupData: map[int64][]Account{
			10: {{ID: 99, Platform: PlatformAnthropicREDACTEDREDACTED,
	REDACTED,
REDACTED
	svc := &adminServiceImpl{
		accountRepo: repo,
		groupRepo:   &groupRepoStubForAdmin{getByID: &Group{ID: 10, Name: "target-group"REDACTEDREDACTED,
REDACTED

	groupIDs := []int64{10REDACTED
	input := &BulkUpdateAccountsInput{
		AccountIDs: []int64{1REDACTED,
		GroupIDs:   &groupIDs,
REDACTED

	result, err := svc.BulkUpdateAccounts(context.Background(), input)
	require.Nil(t, result)
REDACTED
	require.Contains(t, err.Error(), "mixed channel")
	// No BindGroups should have been called since the check runs before any write.
	require.Empty(t, repo.bindGroupsCalls)
REDACTED

func TestAdminServiceBulkUpdateAccounts_ResolvesIDsFromFilters(t *testing.T) {
	repo := &accountRepoStubForBulkUpdate{
		listData: []Account{
			{ID: 7REDACTED,
			{ID: 11REDACTED,
	REDACTED,
		listResult: &pagination.PaginationResult{Total: 2REDACTED,
REDACTED
	svc := &adminServiceImpl{accountRepo: repoREDACTED

	schedulable := true
	input := &BulkUpdateAccountsInput{
		Schedulable: &schedulable,
REDACTED

	filtersField := reflect.ValueOf(input).Elem().FieldByName("Filters")
	require.True(t, filtersField.IsValid(), "BulkUpdateAccountsInput should expose Filters for filter-target bulk update")
	require.Equal(t, reflect.Ptr, filtersField.Kind(), "BulkUpdateAccountsInput.Filters should be a pointer field")

	filtersValue := reflect.New(filtersField.Type().Elem())
	filtersValue.Elem().FieldByName("Platform").SetString(PlatformOpenAI)
	filtersValue.Elem().FieldByName("Type").SetString(AccountTypeOAuth)
	filtersValue.Elem().FieldByName("Status").SetString(StatusActive)
	filtersValue.Elem().FieldByName("Group").SetString("12")
	filtersValue.Elem().FieldByName("PrivacyMode").SetString(PrivacyModeCFBlocked)
	filtersValue.Elem().FieldByName("Search").SetString("bulk-target")
	filtersField.Set(filtersValue)

	result, err := svc.BulkUpdateAccounts(context.Background(), input)
REDACTED
	require.True(t, repo.listCalled, "expected filter-target bulk update to resolve matching IDs via account list filters")
	require.Equal(t, PlatformOpenAI, repo.lastListFilters.platform)
	require.Equal(t, AccountTypeOAuth, repo.lastListFilters.accountType)
	require.Equal(t, StatusActive, repo.lastListFilters.status)
	require.Equal(t, "bulk-target", repo.lastListFilters.search)
	require.Equal(t, int64(12), repo.lastListFilters.groupID)
	require.Equal(t, PrivacyModeCFBlocked, repo.lastListFilters.privacyMode)
	require.Equal(t, []int64{7, 11REDACTED, repo.bulkUpdateIDs)
	require.Equal(t, 2, result.Success)
	require.Equal(t, 0, result.Failed)
	require.Equal(t, []int64{7, 11REDACTED, result.SuccessIDs)
REDACTED

func TestAdminServiceBulkUpdateAccounts_NormalizesOpenAISettings(t *testing.T) {
	repo := &accountRepoStubForBulkUpdate{getByIDsAccounts: []*Account{
		{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKeyREDACTED,
		{ID: 2, Platform: PlatformOpenAI, Type: AccountTypeAPIKeyREDACTED,
REDACTEDREDACTED
	svc := &adminServiceImpl{accountRepo: repoREDACTED

	result, err := svc.BulkUpdateAccounts(context.Background(), &BulkUpdateAccountsInput{
		AccountIDs: []int64{1, 2REDACTED,
REDACTED
			openAIEndpointCapabilitiesCredentialKey: []any{"chat_completions", "embeddings"REDACTED,
	REDACTED,
		Extra: map[string]any{
			openAILongContextBillingEnabledKey: true,
			"openai_responses_mode":            "auto",
	REDACTED,
REDACTED)

REDACTED
	require.Equal(t, 2, result.Success)
	require.Zero(t, result.LongContextInheritedCount)
	require.Equal(t, 1, repo.bulkUpdateCalls)
	require.Contains(t, repo.lastBulkUpdate.Credentials, openAIEndpointCapabilitiesCredentialKey)
	require.Nil(t, repo.lastBulkUpdate.Credentials[openAIEndpointCapabilitiesCredentialKey])
	require.Equal(t, true, repo.lastBulkUpdate.Extra[openAILongContextBillingEnabledKey])
	require.Contains(t, repo.lastBulkUpdate.Extra, "openai_responses_mode")
	require.Nil(t, repo.lastBulkUpdate.Extra["openai_responses_mode"])
REDACTED

func TestAdminServiceBulkUpdateAccounts_AcceptsLongContextAccountTypes(t *testing.T) {
	for _, accountType := range []string{AccountTypeOAuth, AccountTypeSetupToken, AccountTypeAPIKeyREDACTED {
		t.Run(accountType, func(t *testing.T) {
			repo := &accountRepoStubForBulkUpdate{getByIDsAccounts: []*Account{{
				ID: 1, Platform: PlatformOpenAI, Type: accountType,
		REDACTEDREDACTEDREDACTED
			svc := &adminServiceImpl{accountRepo: repoREDACTED

			result, err := svc.BulkUpdateAccounts(context.Background(), &BulkUpdateAccountsInput{
				AccountIDs: []int64{1REDACTED,
				Extra:      map[string]any{openAILongContextBillingEnabledKey: falseREDACTED,
		REDACTED)

		REDACTED
			require.Equal(t, 1, result.Success)
			require.Equal(t, 1, repo.bulkUpdateCalls)
	REDACTED)
REDACTED
REDACTED

func TestAdminServiceBulkUpdateAccounts_EmbeddingsOnlyResetsResponsesMode(t *testing.T) {
	repo := &accountRepoStubForBulkUpdate{getByIDsAccounts: []*Account{
		{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKeyREDACTED,
REDACTEDREDACTED
	svc := &adminServiceImpl{accountRepo: repoREDACTED

	_, err := svc.BulkUpdateAccounts(context.Background(), &BulkUpdateAccountsInput{
		AccountIDs: []int64{1REDACTED,
REDACTED
			openAIEndpointCapabilitiesCredentialKey: []string{"embeddings"REDACTED,
	REDACTED,
REDACTED)

REDACTED
	require.Equal(t, []string{"embeddings"REDACTED, repo.lastBulkUpdate.Credentials[openAIEndpointCapabilitiesCredentialKey])
	require.Contains(t, repo.lastBulkUpdate.Extra, "openai_responses_mode")
	require.Nil(t, repo.lastBulkUpdate.Extra["openai_responses_mode"])
REDACTED

func TestAdminServiceBulkUpdateAccounts_RejectsInvalidOpenAISettingValuesBeforeWrite(t *testing.T) {
	tests := []struct {
		name        string
		credentials map[string]any
		extra       map[string]any
		reason      string
REDACTED{
		{name: "long context type", extra: map[string]any{openAILongContextBillingEnabledKey: "true"REDACTED, reason: "OPENAI_LONG_CONTEXT_BILLING_INVALID"REDACTED,
		{name: "empty capabilities", credentials: map[string]any{openAIEndpointCapabilitiesCredentialKey: []any{REDACTEDREDACTED, reason: "OPENAI_ENDPOINT_CAPABILITIES_INVALID"REDACTED,
		{name: "unknown capability", credentials: map[string]any{openAIEndpointCapabilitiesCredentialKey: []any{"responses"REDACTEDREDACTED, reason: "OPENAI_ENDPOINT_CAPABILITIES_INVALID"REDACTED,
		{name: "capabilities type", credentials: map[string]any{openAIEndpointCapabilitiesCredentialKey: "chat_completions"REDACTED, reason: "OPENAI_ENDPOINT_CAPABILITIES_INVALID"REDACTED,
		{name: "responses mode", extra: map[string]any{"openai_responses_mode": "sometimes"REDACTED, reason: "OPENAI_RESPONSES_MODE_INVALID"REDACTED,
		{name: "responses type", extra: map[string]any{"openai_responses_mode": trueREDACTED, reason: "OPENAI_RESPONSES_MODE_INVALID"REDACTED,
		{
			name:        "embeddings conflict",
			credentials: map[string]any{openAIEndpointCapabilitiesCredentialKey: []any{"embeddings"REDACTEDREDACTED,
			extra:       map[string]any{"openai_responses_mode": "force_responses"REDACTED,
			reason:      "OPENAI_RESPONSES_MODE_INVALID",
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &accountRepoStubForBulkUpdate{REDACTED
			svc := &adminServiceImpl{accountRepo: repoREDACTED
			result, err := svc.BulkUpdateAccounts(context.Background(), &BulkUpdateAccountsInput{
				AccountIDs:  []int64{1REDACTED,
				Credentials: tt.credentials,
				Extra:       tt.extra,
		REDACTED)
			require.Nil(t, result)
			requireApplicationErrorReason(t, err, tt.reason)
			require.Zero(t, repo.bulkUpdateCalls)
	REDACTED)
REDACTED
REDACTED

func TestAdminServiceBulkUpdateAccounts_RejectsInvalidOpenAITargetsBeforeWrite(t *testing.T) {
	tests := []struct {
		name     string
		accounts []*Account
		input    *BulkUpdateAccountsInput
REDACTED{
		{
			name:     "missing account",
			accounts: []*Account{{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuthREDACTEDREDACTED,
			input: &BulkUpdateAccountsInput{
				AccountIDs: []int64{1, 2REDACTED,
				Extra:      map[string]any{openAILongContextBillingEnabledKey: trueREDACTED,
		REDACTED,
	REDACTED,
		{
			name:     "mixed platform long context",
			accounts: []*Account{{ID: 1, Platform: PlatformAnthropic, Type: AccountTypeOAuthREDACTEDREDACTED,
			input: &BulkUpdateAccountsInput{
				AccountIDs: []int64{1REDACTED,
				Extra:      map[string]any{openAILongContextBillingEnabledKey: trueREDACTED,
		REDACTED,
	REDACTED,
		{
			name:     "oauth endpoint capabilities",
			accounts: []*Account{{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuthREDACTEDREDACTED,
			input: &BulkUpdateAccountsInput{
				AccountIDs:  []int64{1REDACTED,
		REDACTEDopenAIEndpointCapabilitiesCredentialKey: nilREDACTED,
		REDACTED,
	REDACTED,
		{
			name:     "unsupported OpenAI long context account type",
			accounts: []*Account{{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeServiceAccountREDACTEDREDACTED,
			input: &BulkUpdateAccountsInput{
				AccountIDs: []int64{1REDACTED,
				Extra:      map[string]any{openAILongContextBillingEnabledKey: trueREDACTED,
		REDACTED,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &accountRepoStubForBulkUpdate{getByIDsAccounts: tt.accountsREDACTED
			svc := &adminServiceImpl{accountRepo: repoREDACTED
			result, err := svc.BulkUpdateAccounts(context.Background(), tt.input)
			require.Nil(t, result)
			requireApplicationErrorReason(t, err, "OPENAI_BULK_TARGET_INVALID")
			require.Zero(t, repo.bulkUpdateCalls)
	REDACTED)
REDACTED
REDACTED

func TestAdminServiceBulkUpdateAccounts_ForcedResponsesRequiresChatCapability(t *testing.T) {
	repo := &accountRepoStubForBulkUpdate{getByIDsAccounts: []*Account{{
		ID:       1,
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
REDACTED
			openAIEndpointCapabilitiesCredentialKey: []any{"embeddings"REDACTED,
	REDACTED,
REDACTEDREDACTEDREDACTED
	svc := &adminServiceImpl{accountRepo: repoREDACTED

	result, err := svc.BulkUpdateAccounts(context.Background(), &BulkUpdateAccountsInput{
		AccountIDs: []int64{1REDACTED,
		Extra:      map[string]any{"openai_responses_mode": "force_chat_completions"REDACTED,
REDACTED)

	require.Nil(t, result)
	requireApplicationErrorReason(t, err, "OPENAI_BULK_TARGET_INVALID")
	require.Zero(t, repo.bulkUpdateCalls)
REDACTED

func TestAdminServiceBulkUpdateAccounts_ForcedResponsesAcceptsChatCapabilityUpdate(t *testing.T) {
	repo := &accountRepoStubForBulkUpdate{getByIDsAccounts: []*Account{{
		ID:       1,
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
REDACTED
			openAIEndpointCapabilitiesCredentialKey: []any{"embeddings"REDACTED,
	REDACTED,
REDACTEDREDACTEDREDACTED
	svc := &adminServiceImpl{accountRepo: repoREDACTED

	_, err := svc.BulkUpdateAccounts(context.Background(), &BulkUpdateAccountsInput{
		AccountIDs: []int64{1REDACTED,
REDACTED
			openAIEndpointCapabilitiesCredentialKey: []any{"chat_completions"REDACTED,
	REDACTED,
		Extra: map[string]any{"openai_responses_mode": "force_responses"REDACTED,
REDACTED)

REDACTED
	require.Equal(t, 1, repo.bulkUpdateCalls)
REDACTED

func TestAdminServiceBulkUpdateAccounts_ReportsLongContextShadowInheritance(t *testing.T) {
	parentID := int64(1)
	repo := &accountRepoStubForBulkUpdate{getByIDsAccounts: []*Account{
		{ID: parentID, Platform: PlatformOpenAI, Type: AccountTypeOAuthREDACTED,
		{ID: 2, Platform: PlatformOpenAI, Type: AccountTypeOAuth, ParentAccountID: &parentIDREDACTED,
REDACTEDREDACTED
	svc := &adminServiceImpl{accountRepo: repoREDACTED

	result, err := svc.BulkUpdateAccounts(context.Background(), &BulkUpdateAccountsInput{
		AccountIDs: []int64{parentID, 2REDACTED,
		Extra:      map[string]any{openAILongContextBillingEnabledKey: trueREDACTED,
REDACTED)

REDACTED
	require.Equal(t, 1, result.LongContextInheritedCount)
	require.Equal(t, 1, repo.bulkUpdateCalls)
REDACTED

func TestAdminServiceBulkUpdateAccounts_RequiresParentForShadowOnlyLongContextUpdate(t *testing.T) {
	parentID := int64(10)
	repo := &accountRepoStubForBulkUpdate{getByIDsAccounts: []*Account{
		{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth, ParentAccountID: &parentIDREDACTED,
		{ID: 2, Platform: PlatformOpenAI, Type: AccountTypeOAuth, ParentAccountID: &parentIDREDACTED,
REDACTEDREDACTED
	svc := &adminServiceImpl{accountRepo: repoREDACTED

	result, err := svc.BulkUpdateAccounts(context.Background(), &BulkUpdateAccountsInput{
		AccountIDs: []int64{1, 2REDACTED,
		Extra:      map[string]any{openAILongContextBillingEnabledKey: trueREDACTED,
REDACTED)

	require.Nil(t, result)
	requireApplicationErrorReason(t, err, "OPENAI_LONG_CONTEXT_PARENT_REQUIRED")
	require.Zero(t, repo.bulkUpdateCalls)
REDACTED

func TestAdminServiceBulkUpdateAccounts_ShadowLongContextAllowsOtherUpdates(t *testing.T) {
	parentID := int64(10)
	repo := &accountRepoStubForBulkUpdate{getByIDsAccounts: []*Account{{
		ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth, ParentAccountID: &parentID,
REDACTEDREDACTEDREDACTED
	svc := &adminServiceImpl{accountRepo: repoREDACTED
	status := StatusDisabled

	result, err := svc.BulkUpdateAccounts(context.Background(), &BulkUpdateAccountsInput{
		AccountIDs: []int64{1REDACTED,
		Status:     status,
		Extra:      map[string]any{openAILongContextBillingEnabledKey: falseREDACTED,
REDACTED)

REDACTED
	require.Equal(t, 1, result.LongContextInheritedCount)
	require.Equal(t, 1, repo.bulkUpdateCalls)
	require.NotNil(t, repo.lastBulkUpdate.Status)
	require.Equal(t, status, *repo.lastBulkUpdate.Status)
REDACTED

func TestAdminServiceBulkUpdateAccounts_ValidatesFilterResolvedOpenAITargets(t *testing.T) {
	repo := &accountRepoStubForBulkUpdate{
		listData:         []Account{{ID: 7REDACTEDREDACTED,
		listResult:       &pagination.PaginationResult{Total: 1REDACTED,
		getByIDsAccounts: []*Account{{ID: 7, Platform: PlatformAnthropic, Type: AccountTypeOAuthREDACTEDREDACTED,
REDACTED
	svc := &adminServiceImpl{accountRepo: repoREDACTED

	result, err := svc.BulkUpdateAccounts(context.Background(), &BulkUpdateAccountsInput{
		Filters: &BulkUpdateAccountFilters{Platform: PlatformOpenAIREDACTED,
		Extra:   map[string]any{openAILongContextBillingEnabledKey: trueREDACTED,
REDACTED)

	require.Nil(t, result)
	requireApplicationErrorReason(t, err, "OPENAI_BULK_TARGET_INVALID")
	require.Equal(t, []int64{7REDACTED, repo.getByIDsIDs)
	require.Zero(t, repo.bulkUpdateCalls)
REDACTED
