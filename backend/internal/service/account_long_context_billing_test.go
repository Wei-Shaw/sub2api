//go:build unit

package service

import (
	"context"
	"net/http"
	"testing"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestAccountIsOpenAILongContextBillingEnabled(t *testing.T) {
	tests := []struct {
		name    string
		account *Account
		want    bool
REDACTED{
		{name: "nil account is disabled", account: nil, want: falseREDACTED,
		{name: "non OpenAI account is disabled", account: &Account{Platform: PlatformGrokREDACTED, want: falseREDACTED,
		{name: "missing extra defaults disabled", account: &Account{Platform: PlatformOpenAIREDACTED, want: falseREDACTED,
		{name: "missing key defaults disabled", account: &Account{Platform: PlatformOpenAI, Extra: map[string]any{REDACTEDREDACTED, want: falseREDACTED,
		{name: "explicit true is enabled", account: &Account{Platform: PlatformOpenAI, Extra: map[string]any{"openai_long_context_billing_enabled": trueREDACTEDREDACTED, want: trueREDACTED,
		{name: "explicit false is disabled", account: &Account{Platform: PlatformOpenAI, Extra: map[string]any{"openai_long_context_billing_enabled": falseREDACTEDREDACTED, want: falseREDACTED,
		{name: "malformed value is disabled", account: &Account{Platform: PlatformOpenAI, Extra: map[string]any{"openai_long_context_billing_enabled": "false"REDACTEDREDACTED, want: falseREDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, tt.account.IsOpenAILongContextBillingEnabled())
	REDACTED)
REDACTED
REDACTED

func TestNormalizeOpenAILongContextBillingExtra(t *testing.T) {
	t.Run("OpenAI missing key persists disabled default", func(t *testing.T) {
		extra, err := normalizeOpenAILongContextBillingExtra(PlatformOpenAI, nil)

	REDACTED
		require.Equal(t, false, extra["openai_long_context_billing_enabled"])
REDACTED)

	t.Run("OpenAI explicit false is preserved", func(t *testing.T) {
		extra, err := normalizeOpenAILongContextBillingExtra(PlatformOpenAI, map[string]any{"openai_long_context_billing_enabled": falseREDACTED)

	REDACTED
		require.Equal(t, false, extra["openai_long_context_billing_enabled"])
REDACTED)

	t.Run("OpenAI malformed value is rejected", func(t *testing.T) {
		_, err := normalizeOpenAILongContextBillingExtra(PlatformOpenAI, map[string]any{"openai_long_context_billing_enabled": "false"REDACTED)

	REDACTED
		require.Equal(t, http.StatusBadRequest, infraerrors.Code(err))
REDACTED)

	t.Run("non OpenAI extra is unchanged", func(t *testing.T) {
		extra, err := normalizeOpenAILongContextBillingExtra(PlatformGrok, nil)

	REDACTED
		require.Nil(t, extra)
REDACTED)

	t.Run("non OpenAI malformed value is ignored", func(t *testing.T) {
		extra := map[string]any{openAILongContextBillingEnabledKey: "provider-owned"REDACTED
		normalized, err := normalizeOpenAILongContextBillingExtra(PlatformAnthropic, extra)

	REDACTED
		require.Equal(t, extra, normalized)
REDACTED)
REDACTED

type longContextBillingRepoStub struct {
	accountRepoStub
	account          *Account
	accounts         []*Account
	createdAccount   *Account
	updateExtraCalls int
	bulkUpdateCalls  int
REDACTED

func (r *longContextBillingRepoStub) Create(_ context.Context, account *Account) error {
	account.ID = 1
	r.account = account
	r.createdAccount = account
	return nil
REDACTED

func (r *longContextBillingRepoStub) GetByID(_ context.Context, _ int64) (*Account, error) {
	return r.account, nil
REDACTED

func (r *longContextBillingRepoStub) GetByIDs(_ context.Context, _ []int64) ([]*Account, error) {
	if r.accounts != nil {
		return r.accounts, nil
REDACTED
	if r.account == nil {
		return nil, nil
REDACTED
	return []*Account{r.accountREDACTED, nil
REDACTED

func (r *longContextBillingRepoStub) Update(_ context.Context, account *Account) error {
	r.account = account
	return nil
REDACTED

func (r *longContextBillingRepoStub) UpdateExtra(_ context.Context, _ int64, _ map[string]any) error {
	r.updateExtraCalls++
	return nil
REDACTED

func (r *longContextBillingRepoStub) BulkUpdate(_ context.Context, _ []int64, _ AccountBulkUpdate) (int64, error) {
	r.bulkUpdateCalls++
	return 1, nil
REDACTED

func TestAdminServiceCreateAccountDefaultsOpenAILongContextBillingDisabled(t *testing.T) {
	repo := &longContextBillingRepoStub{REDACTED
	svc := &adminServiceImpl{accountRepo: repoREDACTED

	account, err := svc.CreateAccount(context.Background(), &CreateAccountInput{
		Name:                 "openai-account",
		Platform:             PlatformOpenAI,
		Type:                 AccountTypeAPIKey,
		Credentials:          map[string]any{"api_key": "test"REDACTED,
		SkipDefaultGroupBind: true,
REDACTED)

REDACTED
	require.Same(t, account, repo.createdAccount)
	require.Equal(t, false, account.Extra[openAILongContextBillingEnabledKey])
REDACTED

func TestAdminServiceCreateAccountRejectsMalformedOpenAILongContextBillingValue(t *testing.T) {
	repo := &longContextBillingRepoStub{REDACTED
	svc := &adminServiceImpl{accountRepo: repoREDACTED

	account, err := svc.CreateAccount(context.Background(), &CreateAccountInput{
		Platform: PlatformOpenAI,
		Extra:    map[string]any{openAILongContextBillingEnabledKey: "false"REDACTED,
REDACTED)

	require.Nil(t, account)
	require.Equal(t, http.StatusBadRequest, infraerrors.Code(err))
	require.Nil(t, repo.createdAccount)
REDACTED

func TestAdminServiceUpdateAccountPreservesOpenAILongContextBillingOptOutWhenOmitted(t *testing.T) {
	repo := &longContextBillingRepoStub{account: &Account{
		ID:       1,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Extra:    map[string]any{openAILongContextBillingEnabledKey: falseREDACTED,
REDACTEDREDACTED
	svc := &adminServiceImpl{accountRepo: repoREDACTED

	account, err := svc.UpdateAccount(context.Background(), 1, &UpdateAccountInput{Extra: map[string]any{REDACTEDREDACTED)

REDACTED
	require.Equal(t, false, account.Extra[openAILongContextBillingEnabledKey])
REDACTED

func TestAdminServiceUpdateAccountAllowsExplicitCodexImportOptIn(t *testing.T) {
	repo := &longContextBillingRepoStub{account: &Account{
		ID:          1,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
REDACTED"access_token": "old-token"REDACTED,
		Extra: map[string]any{
			openAILongContextBillingEnabledKey: false,
			"import_source":                    "codex_session",
	REDACTED,
REDACTEDREDACTED
	svc := &adminServiceImpl{accountRepo: repoREDACTED

	account, err := svc.UpdateAccount(context.Background(), 1, &UpdateAccountInput{
REDACTED"access_token": "new-token"REDACTED,
		Extra: map[string]any{
			openAILongContextBillingEnabledKey: true,
			"import_source":                    "codex_session",
	REDACTED,
REDACTED)

REDACTED
	require.Equal(t, true, account.Extra[openAILongContextBillingEnabledKey])
REDACTED

func TestAdminServiceUpdateAccountAllowsExplicitOptInOutsideCodexImport(t *testing.T) {
	repo := &longContextBillingRepoStub{account: &Account{
		ID:       1,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Extra: map[string]any{
			openAILongContextBillingEnabledKey: false,
			"import_source":                    "codex_session",
	REDACTED,
REDACTEDREDACTED
	svc := &adminServiceImpl{accountRepo: repoREDACTED

	account, err := svc.UpdateAccount(context.Background(), 1, &UpdateAccountInput{Extra: map[string]any{
		openAILongContextBillingEnabledKey: true,
		"import_source":                    "codex_session",
REDACTEDREDACTED)

REDACTED
	require.Equal(t, true, account.Extra[openAILongContextBillingEnabledKey])
REDACTED

func TestAdminServiceUpdateAccountRejectsMalformedOpenAILongContextBillingValue(t *testing.T) {
	repo := &longContextBillingRepoStub{account: &Account{ID: 1, Platform: PlatformOpenAIREDACTEDREDACTED
	svc := &adminServiceImpl{accountRepo: repoREDACTED

	account, err := svc.UpdateAccount(context.Background(), 1, &UpdateAccountInput{Extra: map[string]any{
		openAILongContextBillingEnabledKey: 1,
REDACTEDREDACTED)

	require.Nil(t, account)
	require.Equal(t, http.StatusBadRequest, infraerrors.Code(err))
REDACTED

func TestAdminServiceUpdateAccountExtraRejectsMalformedOpenAILongContextBillingValue(t *testing.T) {
	repo := &longContextBillingRepoStub{account: &Account{ID: 1, Platform: PlatformOpenAIREDACTEDREDACTED
	svc := &adminServiceImpl{accountRepo: repoREDACTED

	err := svc.UpdateAccountExtra(context.Background(), 1, map[string]any{
		openAILongContextBillingEnabledKey: "true",
REDACTED)

	require.Equal(t, http.StatusBadRequest, infraerrors.Code(err))
	require.Zero(t, repo.updateExtraCalls)
REDACTED

func TestAdminServiceUpdateAccountExtraAllowsProviderOwnedValueForNonOpenAIAccount(t *testing.T) {
	repo := &longContextBillingRepoStub{account: &Account{ID: 1, Platform: PlatformAnthropicREDACTEDREDACTED
	svc := &adminServiceImpl{accountRepo: repoREDACTED

	err := svc.UpdateAccountExtra(context.Background(), 1, map[string]any{
		openAILongContextBillingEnabledKey: "provider-owned",
REDACTED)

REDACTED
	require.Equal(t, 1, repo.updateExtraCalls)
REDACTED

func TestAdminServiceBulkUpdateAccountsRejectsMalformedOpenAILongContextBillingValue(t *testing.T) {
	repo := &longContextBillingRepoStub{account: &Account{ID: 1, Platform: PlatformOpenAIREDACTEDREDACTED
	svc := &adminServiceImpl{accountRepo: repoREDACTED

	result, err := svc.BulkUpdateAccounts(context.Background(), &BulkUpdateAccountsInput{
		AccountIDs: []int64{1REDACTED,
		Extra:      map[string]any{openAILongContextBillingEnabledKey: []bool{trueREDACTEDREDACTED,
REDACTED)

	require.Nil(t, result)
	require.Equal(t, http.StatusBadRequest, infraerrors.Code(err))
	require.Zero(t, repo.bulkUpdateCalls)
REDACTED

func TestAdminServiceBulkUpdateAccountsAllowsProviderOwnedValueForNonOpenAIAccounts(t *testing.T) {
	repo := &longContextBillingRepoStub{account: &Account{ID: 1, Platform: PlatformGrokREDACTEDREDACTED
	svc := &adminServiceImpl{accountRepo: repoREDACTED

	result, err := svc.BulkUpdateAccounts(context.Background(), &BulkUpdateAccountsInput{
		AccountIDs: []int64{1REDACTED,
		Extra:      map[string]any{openAILongContextBillingEnabledKey: []string{"provider-owned"REDACTEDREDACTED,
REDACTED)

REDACTED
	require.NotNil(t, result)
	require.Equal(t, 1, repo.bulkUpdateCalls)
REDACTED

func TestAdminServiceBulkUpdateAccountsRejectsMalformedValueForMixedTargetsIncludingOpenAI(t *testing.T) {
	repo := &longContextBillingRepoStub{accounts: []*Account{
		{ID: 1, Platform: PlatformGrokREDACTED,
		{ID: 2, Platform: PlatformOpenAIREDACTED,
REDACTEDREDACTED
	svc := &adminServiceImpl{accountRepo: repoREDACTED

	result, err := svc.BulkUpdateAccounts(context.Background(), &BulkUpdateAccountsInput{
		AccountIDs: []int64{1, 2REDACTED,
		Extra:      map[string]any{openAILongContextBillingEnabledKey: "malformed"REDACTED,
REDACTED)

	require.Nil(t, result)
	require.Equal(t, http.StatusBadRequest, infraerrors.Code(err))
	require.Zero(t, repo.bulkUpdateCalls)
REDACTED
