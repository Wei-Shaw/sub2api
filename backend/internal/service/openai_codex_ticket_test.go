package service

import (
	"context"
	"encoding/json"
	"io"
	"maps"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func fakeCodexTicketState(n int) string {
	return openAICodexTicketStatePrefix + strings.Repeat("B", n-len(openAICodexTicketStatePrefix))
}
func ticketTestAccount(id int64) *Account {
	proxyID := int64(7)
	return &Account{ID: id, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, ProxyID: &proxyID, Proxy: &Proxy{ID: 7, Protocol: "http", Host: "fixed.example.com", Port: 8080}, Credentials: map[string]any{"access_token": "tok", "chatgpt_account_id": "acc-1"}, Extra: map[string]any{codexAccountTicketConfigKey: codexAccountTicketConfig{Enabled: true, Model: openAICodexTicketDefaultModel, ProxyURL: "socks5h://user-sid-{sid}-t-5:secret@us.1024proxy.io:3000", Revision: "revision-1"}}}
}
func ticketTestService(t *testing.T, cfg config.OpenAICodexTicketConfig, upstream HTTPUpstream) *OpenAIGatewayService {
	t.Helper()
	if cfg.HarvestProxyURL == "" {
		cfg.HarvestProxyURL = "socks5h://global-sid-{sid}-t-5:secret@us.1024proxy.io:3000"
	}
	return &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{OpenAICodexTicket: cfg}}, httpUpstream: upstream}
}
func verifiedTestTicket(account *Account, n int) *openAICodexTicket {
	ac := codexAccountTicketConfigOf(account)
	now := time.Now()
	return &openAICodexTicket{AccountID: account.ID, Model: ac.Model, State: fakeCodexTicketState(n), Length: n, CapturedAt: now, ExpiresAt: now.Add(time.Hour), Verified: true, ConfigRevision: ac.Revision, FixedProxyFingerprint: codexTicketFixedProxyFingerprint(account)}
}
func codexModelResponse(model string) *http.Response {
	header := http.Header{}
	header.Set(openAICodexTurnStateHeader, fakeCodexTicketState(292))
	return &http.Response{StatusCode: 200, Header: header, Body: io.NopCloser(strings.NewReader("event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"model\":\"" + model + "\",\"status\":\"completed\"}}\n\n"))}
}

type codexTicketRefreshRepo struct {
	AccountRepository
	mu       sync.Mutex
	accounts []Account
	updates  map[string]any
}

func (r *codexTicketRefreshRepo) GetByID(_ context.Context, id int64) (*Account, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, account := range r.accounts {
		if account.ID == id {
			account.Extra = maps.Clone(account.Extra)
			account.Credentials = maps.Clone(account.Credentials)
			return &account, nil
		}
	}
	return nil, ErrAccountNotFound
}
func (r *codexTicketRefreshRepo) ListByPlatform(context.Context, string) ([]Account, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := append([]Account(nil), r.accounts...)
	for i := range result {
		result[i].Extra = maps.Clone(result[i].Extra)
	}
	return result, nil
}
func (r *codexTicketRefreshRepo) UpdateExtra(_ context.Context, id int64, updates map[string]any) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.updates == nil {
		r.updates = map[string]any{}
	}
	for k, v := range updates {
		r.updates[k] = v
	}
	for i := range r.accounts {
		if r.accounts[i].ID == id {
			r.accounts[i].Extra = maps.Clone(r.accounts[i].Extra)
			if r.accounts[i].Extra == nil {
				r.accounts[i].Extra = map[string]any{}
			}
			for k, v := range updates {
				r.accounts[i].Extra[k] = v
			}
		}
	}
	return nil
}

func TestCodexAccountTicketSwitchIsolation(t *testing.T) {
	for _, tc := range []struct {
		name                         string
		global, enabled, wantBlocked bool
	}{{"all off", false, false, false}, {"master off", false, true, false}, {"account off", true, false, false}, {"both on", true, true, true}} {
		t.Run(tc.name, func(t *testing.T) {
			account := ticketTestAccount(41)
			ac := codexAccountTicketConfigOf(account)
			ac.Enabled = tc.enabled
			account.Extra[codexAccountTicketConfigKey] = ac
			svc := ticketTestService(t, config.OpenAICodexTicketConfig{Enabled: tc.global, FailClosed: true, HarvestProxyURL: "http://legacy:8080"}, nil)
			headers := http.Header{}
			headers.Set(openAICodexTurnStateHeader, "client-state")
			err := svc.applyOpenAICodexTicket(context.Background(), account, ac.Model, headers)
			require.Equal(t, tc.wantBlocked, err == ErrOpenAICodexTicketUnavailable)
			require.Equal(t, tc.wantBlocked, svc.openAICodexTicketBlocksAccount(account, ac.Model))
			require.False(t, svc.openAICodexTicketBlocksAccount(account, "gpt-5.5"))
			require.Equal(t, "client-state", headers.Get(openAICodexTurnStateHeader))
			repo := &codexTicketRefreshRepo{accounts: []Account{*account}}
			svc.accountRepo = repo
			if !tc.wantBlocked {
				svc.refreshOpenAICodexTickets(context.Background())
				require.Empty(t, svc.openaiCodexAccountJobs)
			}
		})
	}
}
func TestCodexAccountTicketVerifiedBindingAndLength(t *testing.T) {
	for _, length := range []int{292, 332} {
		account := ticketTestAccount(41)
		if length == 332 {
			ac := codexAccountTicketConfigOf(account)
			ac.TicketPlan = codexTicketPlanTeam
			account.Extra[codexAccountTicketConfigKey] = ac
		}
		ticket := verifiedTestTicket(account, length)
		svc := ticketTestService(t, config.OpenAICodexTicketConfig{Enabled: true}, nil)
		svc.storeOpenAICodexTicket(context.Background(), account, ticket)
		headers := http.Header{}
		headers.Set(openAICodexTurnStateHeader, "stale")
		require.NoError(t, svc.applyOpenAICodexTicket(context.Background(), account, ticket.Model, headers))
		require.Equal(t, ticket.State, headers.Get(openAICodexTurnStateHeader))
		require.ErrorIs(t, svc.applyOpenAICodexTicket(context.Background(), ticketTestAccount(42), ticket.Model, http.Header{}), ErrOpenAICodexTicketUnavailable)
		require.NoError(t, svc.applyOpenAICodexTicket(context.Background(), account, "gpt-5.6-sol", http.Header{}))
	}
}
func TestCodexAccountTicketRejectsLegacyAndChangedBinding(t *testing.T) {
	for _, change := range []string{"unverified", "revision", "proxy", "account", "model", "expired", "invalid state"} {
		t.Run(change, func(t *testing.T) {
			account := ticketTestAccount(41)
			ticket := verifiedTestTicket(account, 292)
			switch change {
			case "unverified":
				ticket.Verified = false
			case "revision":
				ticket.ConfigRevision = "old"
			case "proxy":
				ticket.FixedProxyFingerprint = "old"
			case "account":
				ticket.AccountID = 42
			case "model":
				ticket.Model = "gpt-5.6-sol"
			case "expired":
				ticket.ExpiresAt = time.Now().Add(-time.Minute)
			case "invalid state":
				ticket.State = "header\ninjection"
			}
			account.Extra[openAICodexTicketExtraKey(openAICodexTicketDefaultModel)] = ticket
			svc := ticketTestService(t, config.OpenAICodexTicketConfig{Enabled: true}, nil)
			require.Nil(t, svc.lookupOpenAICodexTicket(account, openAICodexTicketDefaultModel))
			require.True(t, svc.openAICodexTicketBlocksAccount(account, openAICodexTicketDefaultModel))
		})
	}
}
func TestCodexAccountTicketLiveConfigOverridesStaleScheduler(t *testing.T) {
	stale := ticketTestAccount(41)
	live := *stale
	live.Extra = map[string]any{}
	svc := ticketTestService(t, config.OpenAICodexTicketConfig{Enabled: true}, nil)
	svc.accountRepo = &codexTicketRefreshRepo{accounts: []Account{live}}
	require.False(t, svc.openAICodexTicketBlocksAccount(stale, openAICodexTicketDefaultModel))
	require.NoError(t, svc.applyOpenAICodexTicket(context.Background(), stale, openAICodexTicketDefaultModel, http.Header{}))
}
func TestCodexAccountTicketPrivateConfigPreservationAndRedaction(t *testing.T) {
	account := ticketTestAccount(41)
	repo := &codexTicketRefreshRepo{accounts: []Account{*account}}
	svc := ticketTestService(t, config.OpenAICodexTicketConfig{}, nil)
	svc.accountRepo = repo
	status, err := svc.ConfigureCodexAccountTicket(context.Background(), 41, CodexAccountTicketUpdate{Enabled: true})
	require.NoError(t, err)
	require.True(t, status.ProxyConfigured)
	require.Equal(t, "us.1024proxy.io:3000", status.ProxyDisplay)
	encoded, _ := json.Marshal(status)
	require.NotContains(t, string(encoded), "secret")
	require.NotContains(t, string(encoded), "user-sid")
	next, err := repo.GetByID(context.Background(), 41)
	require.NoError(t, err)
	require.Empty(t, codexAccountTicketConfigOf(next).ProxyURL)
	require.Equal(t, codexAccountTicketConfigOf(account).Revision, codexAccountTicketConfigOf(next).Revision)
	preserved := MergeOpenAICodexTicketExtra(map[string]any{codexAccountTicketConfigKey: "forged", "custom": true}, next.Extra)
	require.Equal(t, next.Extra[codexAccountTicketConfigKey], preserved[codexAccountTicketConfigKey])
	require.Equal(t, true, preserved["custom"])
	require.NotContains(t, MergeOpenAICodexTicketExtra(next.Extra, nil), codexAccountTicketConfigKey)
	require.NotContains(t, RedactOpenAICodexTicketExtra(next.Extra), codexAccountTicketConfigKey)
	_, err = svc.ConfigureCodexAccountTicket(context.Background(), 41, CodexAccountTicketUpdate{Enabled: true, ClearProxy: true})
	require.Error(t, err)
}
func TestCodexTicketFreshSIDAndURLValidation(t *testing.T) {
	for _, raw := range []string{"socks5h://user-sid-{sid}-t-5:secret@us.1024proxy.io:3000", "socks5h://user-sid-old123-t-5:secret@us.1024proxy.io:3000"} {
		require.NoError(t, ValidateOpenAICodexTicketHarvestProxyURL(raw))
		a, b := freshCodexTicketProxyURL(raw), freshCodexTicketProxyURL(raw)
		require.NotEqual(t, a, b)
		parsed, err := url.Parse(a)
		require.NoError(t, err)
		require.NotContains(t, parsed.User.Username(), "{sid}")
		require.NotContains(t, parsed.User.Username(), "old123")
		pass, _ := parsed.User.Password()
		require.Equal(t, "secret", pass)
	}
	raw := "http://fixed-user:secret@proxy.example.com:8080"
	require.Equal(t, raw, freshCodexTicketProxyURL(raw))
}
func TestCodexTicketCompletionChecksActualModel(t *testing.T) {
	for _, raw := range []string{
		`data: {"type":"response.created","response":{"model":"gpt-6-astra"}}` + "\n\n",
		`data: {"type":"response.completed","response":{"model":"gpt-5.6-luna","status":"completed"}}` + "\n\n",
		`data: {"type":"response.failed","response":{"model":"gpt-6-astra"}}` + "\n\n",
		`data: {"type":"response.completed","response":{"model":"gpt-6-astra","status":"incomplete"}}` + "\n\n",
	} {
		require.Error(t, validateCodexTicketCompletedModel(strings.NewReader(raw), openAICodexTicketDefaultModel))
	}
	response := codexModelResponse(openAICodexTicketDefaultModel)
	require.NoError(t, validateCodexTicketCompletedModel(response.Body, openAICodexTicketDefaultModel))
}
func TestOpenAICodexTicketStatusesOptInOnly(t *testing.T) {
	account := ticketTestAccount(41)
	ticket := verifiedTestTicket(account, 292)
	account.Extra[openAICodexTicketExtraKey(ticket.Model)] = ticket
	statuses := OpenAICodexTicketStatuses(account, config.OpenAICodexTicketConfig{Enabled: true}, time.Now())
	require.Len(t, statuses, 1)
	require.True(t, statuses[0].Ready)
	require.Greater(t, statuses[0].RemainingSeconds, int64(3500))
	account.Extra = map[string]any{}
	require.Empty(t, OpenAICodexTicketStatuses(account, config.OpenAICodexTicketConfig{Enabled: true}, time.Now()))
}
func TestExtractOpenAICodexTicketModel(t *testing.T) {
	require.Equal(t, openAICodexTicketDefaultModel, extractOpenAICodexTicketModel([]byte(`{"model":"gpt-6-astra"}`)))
}
func TestOpenAICodexTicketGateCompactRequestUsesForwardOutboundModel(t *testing.T) {
	svc := ticketTestService(t, config.OpenAICodexTicketConfig{Enabled: true}, nil)
	svc.cfg.Gateway.OpenAICompactModel = "gpt-5.5"
	account := ticketTestAccount(41)
	require.True(t, svc.isOpenAIAccountRequestRuntimeBlocked(account, "gpt-6-astra", false))
	require.False(t, svc.isOpenAIAccountRequestRuntimeBlocked(account, "gpt-6-astra", true))
}
