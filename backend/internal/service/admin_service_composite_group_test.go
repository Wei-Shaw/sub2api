//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/stretchr/testify/require"
)

type accountRepoStubForCompositeModelsList struct {
	accountRepoStub
	accounts []Account
REDACTED

func (s *accountRepoStubForCompositeModelsList) ListSchedulableByGroupID(_ context.Context, _ int64) ([]Account, error) {
	return s.accounts, nil
REDACTED

func TestAdminService_CreateCompositeGroupCopiesAccountsFromConcreteGroups(t *testing.T) {
	var copiedFrom []int64
	var boundGroupID int64
	var boundAccountIDs []int64
	groupRepo := &groupRepoStubForAdmin{
		createID: 99,
		getByIDByID: map[int64]*Group{
			10: {ID: 10, Platform: PlatformOpenAIREDACTED,
			20: {ID: 20, Platform: PlatformGeminiREDACTED,
	REDACTED,
		getAccountIDsByGroupIDsFn: func(groupIDs []int64) ([]int64, error) {
			copiedFrom = append([]int64{REDACTED, groupIDs...)
			return []int64{101, 202REDACTED, nil
	REDACTED,
		bindAccountsToGroupFn: func(groupID int64, accountIDs []int64) error {
			boundGroupID = groupID
			boundAccountIDs = append([]int64{REDACTED, accountIDs...)
			return nil
	REDACTED,
REDACTED
	svc := &adminServiceImpl{groupRepo: groupRepoREDACTED

	group, err := svc.CreateGroup(context.Background(), &CreateGroupInput{
		Name:               "Composite",
		Platform:           PlatformComposite,
		RateMultiplier:     1,
		MaxReasoningEffort: "medium",
		ReasoningEffortMappings: []ReasoningEffortMapping{
			{From: "max", To: "xhigh"REDACTED,
	REDACTED,
		CopyAccountsFromGroupIDs: []int64{10, 20, 10REDACTED,
REDACTED)

REDACTED
	require.Equal(t, PlatformComposite, groupRepo.created.Platform)
	require.Equal(t, "medium", groupRepo.created.MaxReasoningEffort)
	require.Equal(t, []ReasoningEffortMapping{{From: "max", To: "xhigh"REDACTEDREDACTED, groupRepo.created.ReasoningEffortMappings)
	require.Equal(t, int64(99), group.ID)
	require.Equal(t, int64(2), group.AccountCount)
	require.ElementsMatch(t, []int64{10, 20REDACTED, copiedFrom)
	require.Equal(t, int64(99), boundGroupID)
	require.ElementsMatch(t, []int64{101, 202REDACTED, boundAccountIDs)
REDACTED

func TestAdminService_UpdateCompositeGroupCopiesAccountsFromConcreteGroups(t *testing.T) {
	var clearedGroupID int64
	var copiedFrom []int64
	var boundGroupID int64
	var boundAccountIDs []int64
	groupRepo := &groupRepoStubForAdmin{
		getByIDByID: map[int64]*Group{
			10: {ID: 10, Platform: PlatformOpenAIREDACTED,
			20: {ID: 20, Platform: PlatformGrokREDACTED,
			99: {ID: 99, Platform: PlatformComposite, RateMultiplier: 1, SubscriptionType: SubscriptionTypeStandardREDACTED,
	REDACTED,
		deleteAccountGroupsByGroupIDFn: func(groupID int64) (int64, error) {
			clearedGroupID = groupID
			return 2, nil
	REDACTED,
		getAccountIDsByGroupIDsFn: func(groupIDs []int64) ([]int64, error) {
			copiedFrom = append([]int64{REDACTED, groupIDs...)
			return []int64{301, 302REDACTED, nil
	REDACTED,
		bindAccountsToGroupFn: func(groupID int64, accountIDs []int64) error {
			boundGroupID = groupID
			boundAccountIDs = append([]int64{REDACTED, accountIDs...)
			return nil
	REDACTED,
REDACTED
	svc := &adminServiceImpl{groupRepo: groupRepoREDACTED
	maxReasoningEffort := "low"
	reasoningEffortMappings := []ReasoningEffortMapping{{From: "max", To: "high"REDACTEDREDACTED

	group, err := svc.UpdateGroup(context.Background(), 99, &UpdateGroupInput{
		MaxReasoningEffort:       &maxReasoningEffort,
		ReasoningEffortMappings:  &reasoningEffortMappings,
		CopyAccountsFromGroupIDs: []int64{10, 20REDACTED,
REDACTED)

REDACTED
	require.Equal(t, PlatformComposite, group.Platform)
	require.Equal(t, "low", group.MaxReasoningEffort)
	require.Equal(t, reasoningEffortMappings, group.ReasoningEffortMappings)
	require.Equal(t, int64(99), clearedGroupID)
	require.ElementsMatch(t, []int64{10, 20REDACTED, copiedFrom)
	require.Equal(t, int64(99), boundGroupID)
	require.ElementsMatch(t, []int64{301, 302REDACTED, boundAccountIDs)
REDACTED

func TestAdminService_CreateAccountAllowsCompositeGroupAssignment(t *testing.T) {
	accountRepo := &accountRepoStubForBulkUpdate{createID: 7REDACTED
	groupRepo := &groupRepoStubForAdmin{
		getByIDByID: map[int64]*Group{
			99: {ID: 99, Platform: PlatformCompositeREDACTED,
	REDACTED,
REDACTED
	svc := &adminServiceImpl{accountRepo: accountRepo, groupRepo: groupRepoREDACTED

	account, err := svc.CreateAccount(context.Background(), &CreateAccountInput{
		Name:                  "OpenAI account",
		Platform:              PlatformOpenAI,
		Type:                  AccountTypeAPIKey,
		Concurrency:           1,
		GroupIDs:              []int64{99REDACTED,
		SkipDefaultGroupBind:  true,
		SkipMixedChannelCheck: true,
REDACTED)

REDACTED
	require.Equal(t, int64(7), account.ID)
	require.Equal(t, PlatformOpenAI, accountRepo.createAccount.Platform)
	require.ElementsMatch(t, []int64{99REDACTED, accountRepo.bindGroupsByAccount[7])
REDACTED

func TestAdminService_UpdateAccountAllowsCompositeGroupAssignment(t *testing.T) {
	accountRepo := &accountRepoStubForBulkUpdate{
		getByIDAccounts: map[int64]*Account{
			7: {ID: 7, Platform: PlatformGemini, Type: AccountTypeAPIKey, Status: StatusActive, Extra: map[string]any{REDACTEDREDACTED,
	REDACTED,
REDACTED
	groupRepo := &groupRepoStubForAdmin{
		getByIDByID: map[int64]*Group{
			99: {ID: 99, Platform: PlatformCompositeREDACTED,
	REDACTED,
REDACTED
	svc := &adminServiceImpl{accountRepo: accountRepo, groupRepo: groupRepoREDACTED
	groupIDs := []int64{99REDACTED

	account, err := svc.UpdateAccount(context.Background(), 7, &UpdateAccountInput{
		GroupIDs:              &groupIDs,
		SkipMixedChannelCheck: true,
REDACTED)

REDACTED
	require.Equal(t, int64(7), account.ID)
	require.Len(t, accountRepo.updatedAccounts, 1)
	require.ElementsMatch(t, []int64{99REDACTED, accountRepo.bindGroupsByAccount[7])
REDACTED

func TestAdminService_CompositeModelsListCandidatesIncludeConcreteAccountMappings(t *testing.T) {
	accountRepo := &accountRepoStubForCompositeModelsList{
		accounts: []Account{
			{
				ID:       1,
				Platform: PlatformOpenAI,
		REDACTED
					"model_mapping": map[string]any{"gpt-custom": "gpt-5"REDACTED,
			REDACTED,
		REDACTED,
			{
				ID:       2,
		REDACTED
		REDACTED
					"model_mapping": map[string]any{"gemini-custom": "gemini-2.5-flash"REDACTED,
			REDACTED,
		REDACTED,
			{
				ID:       3,
				Platform: PlatformKimi,
		REDACTED
					"model_mapping": map[string]any{"kimi-custom": "kimi-k2"REDACTED,
			REDACTED,
		REDACTED,
	REDACTED,
REDACTED
	groupRepo := &groupRepoStubForAdmin{
		getByIDByID: map[int64]*Group{
			99: {ID: 99, Platform: PlatformCompositeREDACTED,
	REDACTED,
REDACTED
	svc := &adminServiceImpl{accountRepo: accountRepo, groupRepo: groupRepoREDACTED

	candidates, err := svc.GetGroupModelsListCandidates(context.Background(), 99, PlatformComposite)

REDACTED
	require.Contains(t, candidates, "gpt-custom")
	require.Contains(t, candidates, "gemini-custom")
	require.Contains(t, candidates, "kimi-custom")
	require.Contains(t, candidates, "gpt-5.5")
	require.Contains(t, candidates, "gemini-2.5-flash")
REDACTED

// 独立 CN 分组的模型列表候选沿用 default 分支的 Claude 默认列表；
// composite 支持不得改变独立分组的候选语义。
func TestAdminService_CNProviderModelsListCandidatesKeepClaudeDefaults(t *testing.T) {
	want := make([]string, 0, len(claude.DefaultModels))
	for _, model := range claude.DefaultModels {
		want = append(want, model.ID)
REDACTED
	for _, platform := range []string{PlatformKimi, PlatformZhipu, PlatformDeepseekREDACTED {
		require.Equal(t, want, defaultModelsListCandidateIDs(platform), "platform=%s", platform)
REDACTED
REDACTED
