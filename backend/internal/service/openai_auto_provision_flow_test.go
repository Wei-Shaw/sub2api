//go:build unit

package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"github.com/stretchr/testify/require"
)

type openAIAutoProvisionFlowSettingRepo struct {
	mu     sync.Mutex
	values map[string]string
}

func (r *openAIAutoProvisionFlowSettingRepo) Get(context.Context, string) (*Setting, error) {
	return nil, ErrSettingNotFound
}

func (r *openAIAutoProvisionFlowSettingRepo) GetValue(_ context.Context, key string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	value, ok := r.values[key]
	if !ok {
		return "", ErrSettingNotFound
	}
	return value, nil
}

func (r *openAIAutoProvisionFlowSettingRepo) Set(_ context.Context, key, value string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.values[key] = value
	return nil
}

func (r *openAIAutoProvisionFlowSettingRepo) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := r.values[key]; ok {
			result[key] = value
		}
	}
	return result, nil
}

func (r *openAIAutoProvisionFlowSettingRepo) SetMultiple(_ context.Context, values map[string]string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for key, value := range values {
		r.values[key] = value
	}
	return nil
}

func (r *openAIAutoProvisionFlowSettingRepo) GetAll(_ context.Context) (map[string]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := make(map[string]string, len(r.values))
	for key, value := range r.values {
		result[key] = value
	}
	return result, nil
}

func (r *openAIAutoProvisionFlowSettingRepo) Delete(_ context.Context, key string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.values, key)
	return nil
}

type openAIAutoProvisionFlowAccounts struct {
	mu       sync.Mutex
	accounts map[int64]Account
}

func (r *openAIAutoProvisionFlowAccounts) GetByID(_ context.Context, id int64) (*Account, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	account, ok := r.accounts[id]
	if !ok {
		return nil, errors.New("account not found")
	}
	return cloneOpenAIAutoProvisionFlowAccount(account), nil
}

func (r *openAIAutoProvisionFlowAccounts) ListSchedulableByPlatform(_ context.Context, platform string) ([]Account, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var result []Account
	for _, account := range r.accounts {
		if account.Platform == platform && account.Schedulable {
			result = append(result, *cloneOpenAIAutoProvisionFlowAccount(account))
		}
	}
	return result, nil
}

func (r *openAIAutoProvisionFlowAccounts) ListAllWithFilters(_ context.Context, platform, accountType, status, _ string, _ int64, _ string) ([]Account, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var result []Account
	for _, account := range r.accounts {
		if account.Platform == platform && account.Type == accountType && account.Status == status {
			result = append(result, *cloneOpenAIAutoProvisionFlowAccount(account))
		}
	}
	return result, nil
}

func (r *openAIAutoProvisionFlowAccounts) UpdateAccount(_ context.Context, id int64, input *UpdateAccountInput) (*Account, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	account, ok := r.accounts[id]
	if !ok {
		return nil, errors.New("account not found")
	}
	if input != nil && input.Credentials != nil {
		account.Credentials = cloneOpenAIAutoProvisionFlowCredentials(input.Credentials)
	}
	r.accounts[id] = account
	return cloneOpenAIAutoProvisionFlowAccount(account), nil
}

func (r *openAIAutoProvisionFlowAccounts) ClearAccountError(_ context.Context, id int64) (*Account, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	account, ok := r.accounts[id]
	if !ok {
		return nil, errors.New("account not found")
	}
	account.Status = StatusActive
	account.ErrorMessage = ""
	account.Schedulable = true
	r.accounts[id] = account
	return cloneOpenAIAutoProvisionFlowAccount(account), nil
}

func (r *openAIAutoProvisionFlowAccounts) addHealthy(id int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.accounts[id] = Account{
		ID:          id,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Credentials: map[string]any{"email": fmt.Sprintf("new-%d@example.com", id)},
	}
}

func (r *openAIAutoProvisionFlowAccounts) account(id int64) Account {
	r.mu.Lock()
	defer r.mu.Unlock()
	return *cloneOpenAIAutoProvisionFlowAccount(r.accounts[id])
}

func cloneOpenAIAutoProvisionFlowCredentials(values map[string]any) map[string]any {
	result := make(map[string]any, len(values))
	for key, value := range values {
		result[key] = value
	}
	return result
}

func cloneOpenAIAutoProvisionFlowAccount(account Account) *Account {
	account.Credentials = cloneOpenAIAutoProvisionFlowCredentials(account.Credentials)
	return &account
}

type openAIAutoProvisionFlowClient struct {
	provision        []openAIProvisionCommand
	reauthorizations []openAIReauthorizeCommand
}

func (c *openAIAutoProvisionFlowClient) Provision(_ context.Context, command openAIProvisionCommand, _, _ string) error {
	c.provision = append(c.provision, command)
	return nil
}

func (c *openAIAutoProvisionFlowClient) Reauthorize(_ context.Context, command openAIReauthorizeCommand, _, _ string) error {
	c.reauthorizations = append(c.reauthorizations, command)
	return nil
}

type openAIAutoProvisionFlowOAuthClient struct {
	exchangeCalls []openAIExchangeCall
	token         openai.TokenResponse
}

type openAIExchangeCall struct {
	code         string
	codeVerifier string
	redirectURI  string
	clientID     string
}

func (c *openAIAutoProvisionFlowOAuthClient) ExchangeCode(_ context.Context, code, codeVerifier, redirectURI, _, clientID string) (*openai.TokenResponse, error) {
	c.exchangeCalls = append(c.exchangeCalls, openAIExchangeCall{
		code: code, codeVerifier: codeVerifier, redirectURI: redirectURI, clientID: clientID,
	})
	return &c.token, nil
}

func (c *openAIAutoProvisionFlowOAuthClient) RefreshToken(context.Context, string, string) (*openai.TokenResponse, error) {
	return nil, errors.New("unexpected refresh token call")
}

func (c *openAIAutoProvisionFlowOAuthClient) RefreshTokenWithClientID(context.Context, string, string, string) (*openai.TokenResponse, error) {
	return nil, errors.New("unexpected refresh token call")
}

func TestOpenAIAutoProvisionFullReplenishmentAndReauthorizationFlow(t *testing.T) {
	originalXAIModelMapping := xai.RuntimeModelMappingOptions()
	t.Cleanup(func() { xai.SetRuntimeModelMappingOptions(originalXAIModelMapping) })

	const (
		errorAccountID = int64(99)
		errorEmail     = "broken@example.com"
	)

	settingsRepo := &openAIAutoProvisionFlowSettingRepo{values: map[string]string{
		SettingKeyOpenAIAutoProvisionEnabled:         "true",
		SettingKeyOpenAIAutoProvisionTarget:          "3",
		SettingKeyOpenAIAutoProvisionIntervalSeconds: "15",
		SettingKeyOpenAIAutoProvisionTurbURL:         "http://turb.test",
		SettingKeyOpenAIAutoProvisionTurbAuthCode:    "fake-auth-code",
		SettingKeyOpenAIAutoProvisionCallbackURL:     "http://sub2api.test/api/v1/integrations/openai/auto-provision/callback",
		SettingKeyOpenAIAutoProvisionCallbackSecret:  "fake-callback-secret",
		SettingKeyOpenAIAutoProvisionEmailSource:     "outlook",
		SettingKeyOpenAIAutoProvisionWorkers:         "2",
		SettingKeyOpenAIReauthorizationEnabled:       "true",
	}}
	accounts := &openAIAutoProvisionFlowAccounts{accounts: map[int64]Account{
		1: {
			ID:          1,
			Platform:    PlatformOpenAI,
			Type:        AccountTypeOAuth,
			Status:      StatusActive,
			Schedulable: true,
			Credentials: map[string]any{"email": "healthy@example.com"},
		},
		errorAccountID: {
			ID:           errorAccountID,
			Platform:     PlatformOpenAI,
			Type:         AccountTypeOAuth,
			Status:       StatusError,
			ErrorMessage: "invalid oauth token",
			Credentials: map[string]any{
				"email":              errorEmail,
				"chatgpt_account_id": "acct-99",
				"access_token":       "old-access-token",
			},
		},
	}}
	client := &openAIAutoProvisionFlowClient{}
	oauthClient := &openAIAutoProvisionFlowOAuthClient{
		token: openai.TokenResponse{
			AccessToken:  "new-access-token",
			RefreshToken: "new-refresh-token",
			ExpiresIn:    3600,
			IDToken:      openAIAutoProvisionFlowIDToken(errorEmail, "acct-99"),
		},
	}
	oauth := NewOpenAIOAuthService(nil, oauthClient)
	defer oauth.Stop()
	service := NewOpenAIAutoProvisionService(
		accounts,
		accounts,
		NewSettingService(settingsRepo, &config.Config{}),
		settingsRepo,
		oauth,
		client,
		nil,
		nil,
	)
	ctx := context.Background()

	// 1) One healthy account against target three dispatches two registrations.
	require.NoError(t, service.RunOnce(ctx))
	require.Len(t, client.provision, 1)
	require.Equal(t, 2, client.provision[0].Count)
	require.Empty(t, client.reauthorizations)

	// Reauthorization is driven by the existing OAuth refresh service path,
	// rather than the provision scheduler.
	require.NoError(t, service.ReauthorizeErroredOpenAIOAuthAccounts(ctx))
	require.Len(t, client.reauthorizations, 1)
	require.Equal(t, errorAccountID, client.reauthorizations[0].AccountID)
	require.Equal(t, errorEmail, client.reauthorizations[0].Email)

	// 2) A second refresh/provision cycle does not duplicate either pending request.
	require.NoError(t, service.RunOnce(ctx))
	require.NoError(t, service.ReauthorizeErroredOpenAIOAuthAccounts(ctx))
	require.Len(t, client.provision, 1)
	require.Len(t, client.reauthorizations, 1)

	// 3) Turb finishes two jobs but only uploads one account. The callback clears
	// the old batch, so the next cycle computes and dispatches the remaining one.
	accounts.addHealthy(2)
	registrationCallback := OpenAIAutoProvisionCallback{
		RequestID:      client.provision[0].RequestID,
		EventID:        client.provision[0].RequestID + ":registration:completed",
		Kind:           "registration",
		Status:         "completed",
		RequestedCount: 2,
		SucceededCount: 1,
		FailedCount:    1,
		PendingCount:   0,
	}
	replay, err := service.HandleProvisionCallback(ctx, registrationCallback)
	require.NoError(t, err)
	require.False(t, replay)
	replay, err = service.HandleProvisionCallback(ctx, registrationCallback)
	require.NoError(t, err)
	require.True(t, replay)

	require.NoError(t, service.RunOnce(ctx))
	require.Len(t, client.provision, 2)
	require.Equal(t, 1, client.provision[1].Count)

	// 4) The final uploaded account reaches the target; the following cycle does
	// not call Turb again even though the reauthorization is still pending.
	accounts.addHealthy(3)
	finalRegistrationCallback := registrationCallback
	finalRegistrationCallback.RequestID = client.provision[1].RequestID
	finalRegistrationCallback.EventID = client.provision[1].RequestID + ":registration:completed"
	finalRegistrationCallback.RequestedCount = 1
	finalRegistrationCallback.SucceededCount = 1
	finalRegistrationCallback.FailedCount = 0
	replay, err = service.HandleProvisionCallback(ctx, finalRegistrationCallback)
	require.NoError(t, err)
	require.False(t, replay)
	require.NoError(t, service.RunOnce(ctx))
	require.Len(t, client.provision, 2)

	// 5) Turb's Codex retry returns the complete Authorization callback URL.
	// Sub2API parses code/state from that URL, exchanges them, validates identity,
	// persists fresh credentials, and clears the account error before accepting
	// the terminal job callback.
	authURL, err := oauth.GenerateAuthURL(ctx, nil, "", PlatformOpenAI)
	require.NoError(t, err)
	parsedAuthURL, err := url.Parse(authURL.AuthURL)
	require.NoError(t, err)
	oauthState := parsedAuthURL.Query().Get("state")
	require.NotEmpty(t, oauthState)
	oauthCallbackURL := "http://localhost:1455/auth/callback?code=fake-authorization-code&state=" + url.QueryEscape(oauthState)
	oauthCallback := OpenAIAutoReauthorizationCallback{
		RequestID:   client.reauthorizations[0].RequestID,
		EventID:     client.reauthorizations[0].RequestID + ":reauthorization:oauth-callback",
		AccountID:   errorAccountID,
		Email:       errorEmail,
		SessionID:   authURL.SessionID,
		CallbackURL: oauthCallbackURL,
		RedirectURI: "http://localhost:1455/auth/callback",
	}
	replay, err = service.HandleReauthorizationCallback(ctx, oauthCallback)
	require.NoError(t, err)
	require.False(t, replay)
	require.Len(t, oauthClient.exchangeCalls, 1)
	require.Equal(t, "fake-authorization-code", oauthClient.exchangeCalls[0].code)
	require.Equal(t, oauthCallback.RedirectURI, oauthClient.exchangeCalls[0].redirectURI)
	updatedAccount := accounts.account(errorAccountID)
	require.Equal(t, StatusActive, updatedAccount.Status)
	require.True(t, updatedAccount.Schedulable)
	require.Empty(t, updatedAccount.ErrorMessage)
	require.Equal(t, "new-access-token", updatedAccount.GetCredential("access_token"))
	require.Equal(t, "new-refresh-token", updatedAccount.GetCredential("refresh_token"))
	require.Equal(t, errorEmail, updatedAccount.GetCredential("email"))
	require.Equal(t, "acct-99", updatedAccount.GetCredential("chatgpt_account_id"))

	completionCallback := OpenAIAutoProvisionCallback{
		RequestID: client.reauthorizations[0].RequestID,
		EventID:   client.reauthorizations[0].RequestID + ":reauthorization:completed",
		Kind:      "reauthorization",
		Status:    "succeeded",
		AccountID: errorAccountID,
		Email:     errorEmail,
	}
	replay, err = service.HandleReauthorizationCompletion(ctx, completionCallback)
	require.NoError(t, err)
	require.False(t, replay)
	replay, err = service.HandleReauthorizationCallback(ctx, oauthCallback)
	require.NoError(t, err)
	require.True(t, replay)
	replay, err = service.HandleReauthorizationCompletion(ctx, completionCallback)
	require.NoError(t, err)
	require.True(t, replay)

	// 6) Reauthorization is no longer scheduled after the error was cleared.
	require.NoError(t, service.RunOnce(ctx))
	require.NoError(t, service.ReauthorizeErroredOpenAIOAuthAccounts(ctx))
	require.Len(t, client.reauthorizations, 1)

	// 7) The explicit reauthorization setting is authoritative even for an
	// already errored account.
	accounts.mu.Lock()
	account := accounts.accounts[errorAccountID]
	account.Status = StatusError
	account.Schedulable = false
	account.ErrorMessage = "failed again"
	accounts.accounts[errorAccountID] = account
	accounts.mu.Unlock()
	settingsRepo.values[SettingKeyOpenAIReauthorizationEnabled] = "false"
	require.NoError(t, service.ReauthorizeErroredOpenAIOAuthAccounts(ctx))
	require.Len(t, client.reauthorizations, 1)
}

type openAIAutoProvisionEmptyCandidatePager struct{}

func (openAIAutoProvisionEmptyCandidatePager) ListOAuthRefreshCandidatePage(context.Context, OAuthRefreshPageOptions) (*OAuthRefreshCandidatePage, error) {
	return &OAuthRefreshCandidatePage{Accounts: []Account{}}, nil
}

type openAIAutoProvisionDispatcherSpy struct {
	calls int
}

func (s *openAIAutoProvisionDispatcherSpy) ReauthorizeErroredOpenAIOAuthAccounts(context.Context) error {
	s.calls++
	return nil
}

func TestTokenRefreshCycleInvokesOpenAIReauthorizationDispatcher(t *testing.T) {
	cfg := &config.Config{TokenRefresh: config.TokenRefreshConfig{
		Enabled:                  true,
		RefreshBeforeExpiryHours: 0.5,
		CycleTimeoutSeconds:      5,
	}}
	dispatcher := &openAIAutoProvisionDispatcherSpy{}
	refreshService := &TokenRefreshService{
		candidatePager: openAIAutoProvisionEmptyCandidatePager{},
		registrations: []tokenRefreshRegistration{{
			platform:  PlatformOpenAI,
			refresher: &tokenRefresherStub{},
			executor:  &tokenRefresherStub{},
		}},
		cfg: &cfg.TokenRefresh,
	}
	refreshService.SetOpenAIReauthorizationDispatcher(dispatcher)

	refreshService.processRefreshContext(context.Background())
	require.Equal(t, 1, dispatcher.calls)
}

func openAIAutoProvisionFlowIDToken(email, accountID string) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	payload, _ := json.Marshal(map[string]any{
		"email": email,
		"exp":   time.Now().Add(time.Hour).Unix(),
		"https://api.openai.com/auth": map[string]string{
			"chatgpt_account_id": accountID,
		},
	})
	return fmt.Sprintf(
		"%s.%s.signature",
		header,
		base64.RawURLEncoding.EncodeToString(payload),
	)
}
