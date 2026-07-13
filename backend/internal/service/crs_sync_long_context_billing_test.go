//go:build unit

package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type crsLongContextAccountRepo struct {
	AccountRepository
	accounts map[string]*Account
	nextID   int64
REDACTED

type crsOpenAILongContextSource struct {
	collection  string
	credentials map[string]any
	extra       map[string]any
REDACTED

func newCRSLongContextAccountRepo(existing ...*Account) *crsLongContextAccountRepo {
	repo := &crsLongContextAccountRepo{accounts: make(map[string]*Account)REDACTED
	for _, account := range existing {
		if account == nil {
			continue
	REDACTED
		crsID, _ := account.Extra["crs_account_id"].(string)
		repo.accounts[crsID] = account
		if account.ID > repo.nextID {
			repo.nextID = account.ID
	REDACTED
REDACTED
	return repo
REDACTED

func (r *crsLongContextAccountRepo) Create(_ context.Context, account *Account) error {
	r.nextID++
	account.ID = r.nextID
	crsID, _ := account.Extra["crs_account_id"].(string)
	r.accounts[crsID] = account
	return nil
REDACTED

func (r *crsLongContextAccountRepo) Update(_ context.Context, account *Account) error {
	crsID, _ := account.Extra["crs_account_id"].(string)
	r.accounts[crsID] = account
	return nil
REDACTED

func (r *crsLongContextAccountRepo) GetByCRSAccountID(_ context.Context, crsID string) (*Account, error) {
	return r.accounts[crsID], nil
REDACTED

func (r *crsLongContextAccountRepo) ListShadowsByParent(_ context.Context, _ int64) ([]*Account, error) {
	return nil, nil
REDACTED

func TestCRSSyncOpenAILongContextBilling(t *testing.T) {
	tests := []struct {
		name          string
		collection    string
		credentials   map[string]any
		sourceExtra   map[string]any
		existingExtra map[string]any
		wantAction    string
		wantEnabled   bool
REDACTED{
		{name: "OAuth create defaults missing value disabled", collection: "openaiOAuthAccounts", credentials: map[string]any{"access_token": "oauth-token"REDACTED, wantAction: "created"REDACTED,
		{name: "OAuth create preserves source true", collection: "openaiOAuthAccounts", credentials: map[string]any{"access_token": "oauth-token"REDACTED, sourceExtra: map[string]any{openAILongContextBillingEnabledKey: trueREDACTED, wantAction: "created", wantEnabled: trueREDACTED,
		{name: "OAuth create preserves source false", collection: "openaiOAuthAccounts", credentials: map[string]any{"access_token": "oauth-token"REDACTED, sourceExtra: map[string]any{openAILongContextBillingEnabledKey: falseREDACTED, wantAction: "created"REDACTED,
		{name: "OAuth update defaults missing value disabled", collection: "openaiOAuthAccounts", credentials: map[string]any{"access_token": "oauth-token"REDACTED, existingExtra: map[string]any{"existing": trueREDACTED, wantAction: "updated"REDACTED,
		{name: "OAuth update preserves existing true when source omits value", collection: "openaiOAuthAccounts", credentials: map[string]any{"access_token": "oauth-token"REDACTED, existingExtra: map[string]any{openAILongContextBillingEnabledKey: trueREDACTED, wantAction: "updated", wantEnabled: trueREDACTED,
		{name: "OAuth update preserves existing false when source omits value", collection: "openaiOAuthAccounts", credentials: map[string]any{"access_token": "oauth-token"REDACTED, existingExtra: map[string]any{openAILongContextBillingEnabledKey: falseREDACTED, wantAction: "updated"REDACTED,
		{name: "OAuth update preserves source true over existing false", collection: "openaiOAuthAccounts", credentials: map[string]any{"access_token": "oauth-token"REDACTED, sourceExtra: map[string]any{openAILongContextBillingEnabledKey: trueREDACTED, existingExtra: map[string]any{openAILongContextBillingEnabledKey: falseREDACTED, wantAction: "updated", wantEnabled: trueREDACTED,
		{name: "OAuth update preserves source false over existing true", collection: "openaiOAuthAccounts", credentials: map[string]any{"access_token": "oauth-token"REDACTED, sourceExtra: map[string]any{openAILongContextBillingEnabledKey: falseREDACTED, existingExtra: map[string]any{openAILongContextBillingEnabledKey: trueREDACTED, wantAction: "updated"REDACTED,
		{name: "OAuth rejects malformed source value", collection: "openaiOAuthAccounts", credentials: map[string]any{"access_token": "oauth-token"REDACTED, sourceExtra: map[string]any{openAILongContextBillingEnabledKey: "false"REDACTED, wantAction: "failed"REDACTED,
		{name: "OAuth rejects malformed existing value", collection: "openaiOAuthAccounts", credentials: map[string]any{"access_token": "oauth-token"REDACTED, existingExtra: map[string]any{openAILongContextBillingEnabledKey: "false"REDACTED, wantAction: "failed"REDACTED,
		{name: "OAuth update rejects malformed source value", collection: "openaiOAuthAccounts", credentials: map[string]any{"access_token": "oauth-token"REDACTED, sourceExtra: map[string]any{openAILongContextBillingEnabledKey: "false"REDACTED, existingExtra: map[string]any{openAILongContextBillingEnabledKey: trueREDACTED, wantAction: "failed"REDACTED,
		{name: "API key create defaults missing value disabled", collection: "openaiResponsesAccounts", credentials: map[string]any{"api_key": "sk-test"REDACTED, wantAction: "created"REDACTED,
		{name: "API key create preserves source true", collection: "openaiResponsesAccounts", credentials: map[string]any{"api_key": "sk-test"REDACTED, sourceExtra: map[string]any{openAILongContextBillingEnabledKey: trueREDACTED, wantAction: "created", wantEnabled: trueREDACTED,
		{name: "API key create preserves source false", collection: "openaiResponsesAccounts", credentials: map[string]any{"api_key": "sk-test"REDACTED, sourceExtra: map[string]any{openAILongContextBillingEnabledKey: falseREDACTED, wantAction: "created"REDACTED,
		{name: "API key update defaults missing value disabled", collection: "openaiResponsesAccounts", credentials: map[string]any{"api_key": "sk-test"REDACTED, existingExtra: map[string]any{"existing": trueREDACTED, wantAction: "updated"REDACTED,
		{name: "API key update preserves existing true when source omits value", collection: "openaiResponsesAccounts", credentials: map[string]any{"api_key": "sk-test"REDACTED, existingExtra: map[string]any{openAILongContextBillingEnabledKey: trueREDACTED, wantAction: "updated", wantEnabled: trueREDACTED,
		{name: "API key update preserves existing false when source omits value", collection: "openaiResponsesAccounts", credentials: map[string]any{"api_key": "sk-test"REDACTED, existingExtra: map[string]any{openAILongContextBillingEnabledKey: falseREDACTED, wantAction: "updated"REDACTED,
		{name: "API key update preserves source true over existing false", collection: "openaiResponsesAccounts", credentials: map[string]any{"api_key": "sk-test"REDACTED, sourceExtra: map[string]any{openAILongContextBillingEnabledKey: trueREDACTED, existingExtra: map[string]any{openAILongContextBillingEnabledKey: falseREDACTED, wantAction: "updated", wantEnabled: trueREDACTED,
		{name: "API key update preserves source false over existing true", collection: "openaiResponsesAccounts", credentials: map[string]any{"api_key": "sk-test"REDACTED, sourceExtra: map[string]any{openAILongContextBillingEnabledKey: falseREDACTED, existingExtra: map[string]any{openAILongContextBillingEnabledKey: trueREDACTED, wantAction: "updated"REDACTED,
		{name: "API key rejects malformed source value", collection: "openaiResponsesAccounts", credentials: map[string]any{"api_key": "sk-test"REDACTED, sourceExtra: map[string]any{openAILongContextBillingEnabledKey: "false"REDACTED, wantAction: "failed"REDACTED,
		{name: "API key rejects malformed existing value", collection: "openaiResponsesAccounts", credentials: map[string]any{"api_key": "sk-test"REDACTED, existingExtra: map[string]any{openAILongContextBillingEnabledKey: "false"REDACTED, wantAction: "failed"REDACTED,
		{name: "API key update rejects malformed source value", collection: "openaiResponsesAccounts", credentials: map[string]any{"api_key": "sk-test"REDACTED, sourceExtra: map[string]any{openAILongContextBillingEnabledKey: "false"REDACTED, existingExtra: map[string]any{openAILongContextBillingEnabledKey: trueREDACTED, wantAction: "failed"REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			const crsID = "crs-openai-1"
			var existing *Account
			if tt.existingExtra != nil {
				existingExtra := mergeMap(tt.existingExtra, map[string]any{"crs_account_id": crsIDREDACTED)
				accountType := AccountTypeOAuth
				if tt.collection == "openaiResponsesAccounts" {
					accountType = AccountTypeAPIKey
			REDACTED
				existing = &Account{ID: 41, Platform: PlatformOpenAI, Type: accountType, Extra: existingExtraREDACTED
		REDACTED
			repo := newCRSLongContextAccountRepo(existing)
			result := runCRSOpenAILongContextSync(t, repo, crsOpenAILongContextSource{
				collection:  tt.collection,
				credentials: tt.credentials,
				extra:       tt.sourceExtra,
		REDACTED)

			require.Len(t, result.Items, 1)
			require.Equal(t, tt.wantAction, result.Items[0].Action)
			if tt.wantAction == "failed" {
				require.Contains(t, result.Items[0].Error, "openai_long_context_billing_enabled must be a boolean")
				return
		REDACTED
			stored, ok := repo.accounts[crsID].Extra[openAILongContextBillingEnabledKey]
			require.True(t, ok)
			require.Equal(t, tt.wantEnabled, stored)
	REDACTED)
REDACTED
REDACTED

func runCRSOpenAILongContextSync(t *testing.T, repo AccountRepository, source crsOpenAILongContextSource) *SyncFromCRSResult {
REDACTED
	account := map[string]any{
		"kind":        "openai",
		"id":          "crs-openai-1",
		"name":        "OpenAI CRS",
		"isActive":    true,
		"schedulable": true,
		"credentials": source.credentials,
REDACTED
	if source.extra != nil {
		account["extra"] = source.extra
REDACTED

	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		if request.URL.Path == "/web/auth/login" {
			_, _ = response.Write([]byte(`{"success":true,"token":"admin-token"REDACTED`))
			return
	REDACTED
		require.Equal(t, "/admin/sync/export-accounts", request.URL.Path)
		require.NoError(t, json.NewEncoder(response).Encode(map[string]any{
			"success": true,
			"data":    map[string]any{source.collection: []any{accountREDACTEDREDACTED,
	REDACTED))
REDACTED))
	t.Cleanup(server.Close)

	cfg := &config.Config{REDACTED
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	service := NewCRSSyncService(repo, nil, nil, nil, nil, cfg)
	result, err := service.SyncFromCRS(context.Background(), SyncFromCRSInput{
		BaseURL:  server.URL,
		Username: "admin",
		Password: "password",
REDACTED)
REDACTED
	return result
REDACTED
