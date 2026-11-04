package admin

import (
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestAccountResponseCodexTicketsUsesConfiguredPolicy(t *testing.T) {
	account := &service.Account{ID: 41, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth}
	h := &AccountHandler{cfg: &config.Config{}}
	require.Empty(t, h.accountResponseFromService(account).CodexTurnTickets)
	require.Empty(t, h.accountListResponseFromService(account).CodexTurnTickets)
	h.cfg.Gateway.OpenAICodexTicket = config.OpenAICodexTicketConfig{Enabled: true, Models: []string{"configured-model"}, FailClosed: false}
	status := h.accountListResponseFromService(account).CodexTurnTickets
	require.Len(t, status, 1)
	require.Equal(t, "configured-model", status[0].Model)
	require.False(t, status[0].Blocked)
	h.cfg.Gateway.OpenAICodexTicket.FailClosed = true
	require.True(t, h.accountResponseFromService(account).CodexTurnTickets[0].Blocked)
}

func TestAccountResponseCodexTicketsReadsLiveSettingsAfterRestart(t *testing.T) {
	cfg := &config.Config{}
	repo := &settingHandlerRepoStub{values: map[string]string{service.SettingKeyOpenAICodexTicketEnabled: "true"}}
	settings := service.NewSettingService(repo, cfg)
	h := &AccountHandler{cfg: cfg}
	h.SetCodexTicketSettings(settings)
	account := &service.Account{ID: 41, Platform: service.PlatformOpenAI, Type: service.AccountTypeSetupToken}
	require.Len(t, h.accountListResponseFromService(account).CodexTurnTickets, 2)
	require.False(t, cfg.Gateway.OpenAICodexTicket.Enabled)
	repo.values[service.SettingKeyOpenAICodexTicketEnabled] = "false"
	settings.InvalidateOpenAICodexTicketEnabledCache()
	require.Empty(t, h.accountResponseFromService(account).CodexTurnTickets)
}
