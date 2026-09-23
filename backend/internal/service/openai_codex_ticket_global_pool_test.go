package service

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type codexTicketGlobalPoolRepo struct {
	SettingRepository
	mu   sync.RWMutex
	pool string
}

func (r *codexTicketGlobalPoolRepo) GetValue(_ context.Context, key string) (string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if key == SettingKeyOpenAICodexTicketHarvestProxyURL {
		return r.pool, nil
	}
	return "", ErrSettingNotFound
}

func (r *codexTicketGlobalPoolRepo) set(s *OpenAIGatewayService, value string) {
	r.mu.Lock()
	r.pool = value
	r.mu.Unlock()
	s.settingService.InvalidateOpenAICodexTicketHarvestProxyCache()
}

func TestCodexAccountTicketGlobalPoolOverridesLegacyAccountProxy(t *testing.T) {
	var calls atomic.Int64
	s, repo := ticketJobService(t, &codexTicketFuncUpstream{proxy: func(req *http.Request, proxy string) (*http.Response, error) {
		calls.Add(1)
		if req.Header.Get(openAICodexTurnStateHeader) == "" {
			require.Contains(t, proxy, "global.example.com")
			require.NotContains(t, proxy, "legacy.example.com")
		} else {
			require.Equal(t, "http://fixed.example.com:8080", proxy)
		}
		return codexTicketResponse(), nil
	}})
	pool := &codexTicketGlobalPoolRepo{pool: "socks5h://global:secret@global.example.com:1080"}
	s.settingService = NewSettingService(pool, s.cfg)
	legacy := codexAccountTicketConfigOf(&repo.accounts[0])
	legacy.ProxyURL = "http://legacy:old-secret@legacy.example.com:8080"
	repo.accounts[0].Extra[codexAccountTicketConfigKey] = legacy
	other := ticketTestAccount(42)
	other.Extra = map[string]any{} // The global pool must not opt in a normal account.
	repo.accounts = append(repo.accounts, *other)
	waitCodexTicketJob(t, s.startCodexAccountTicketJob(context.Background(), 41, true))
	require.Equal(t, int64(2), calls.Load())
	status, err := s.GetCodexAccountTicketStatus(context.Background(), 41)
	require.NoError(t, err)
	require.True(t, status.TicketUsable)
	require.Equal(t, "global.example.com:1080", status.ProxyDisplay)
	otherStatus, err := s.GetCodexAccountTicketStatus(context.Background(), 42)
	require.NoError(t, err)
	require.False(t, otherStatus.Enabled)
	require.True(t, otherStatus.ProxyConfigured)
	s.refreshOpenAICodexTickets(context.Background())
	require.Equal(t, int64(2), calls.Load())
	require.Nil(t, s.startCodexAccountTicketJob(context.Background(), 42, true))
	for _, input := range []CodexAccountTicketUpdate{{Enabled: true, ProxyURL: legacy.ProxyURL}, {Enabled: true, ClearProxy: true}} {
		_, err := s.ConfigureCodexAccountTicket(context.Background(), 41, input)
		require.Error(t, err)
		require.NotContains(t, err.Error(), "old-secret")
	}
}

func TestCodexAccountTicketGlobalPoolChangeKeepsValidTicket(t *testing.T) {
	account := ticketTestAccount(41)
	old := verifiedTestTicket(account, 292)
	account.Extra[openAICodexTicketExtraKey(old.Model)] = old
	s := ticketTestService(t, config.OpenAICodexTicketConfig{Enabled: true}, nil)
	s.accountRepo = &codexTicketRefreshRepo{accounts: []Account{*account}}
	pool := &codexTicketGlobalPoolRepo{pool: "http://first.example.com:8080"}
	s.settingService = NewSettingService(pool, s.cfg)
	pool.set(s, "http://second.example.com:8080")
	status, err := s.GetCodexAccountTicketStatus(context.Background(), 41)
	require.NoError(t, err)
	require.Equal(t, "second.example.com:8080", status.ProxyDisplay)
	require.True(t, status.TicketUsable)
	require.True(t, status.ExpiresAt.Equal(old.ExpiresAt))
	s.refreshOpenAICodexTickets(context.Background())
	require.Empty(t, s.openaiCodexAccountJobs)
	headers := http.Header{}
	require.NoError(t, s.applyOpenAICodexTicket(context.Background(), account, old.Model, headers))
	require.Equal(t, old.State, headers.Get(openAICodexTurnStateHeader))
	// Saving unchanged account settings retires its legacy proxy without changing the ticket binding.
	_, err = s.ConfigureCodexAccountTicket(context.Background(), 41, CodexAccountTicketUpdate{Enabled: true})
	require.NoError(t, err)
	live, err := s.accountRepo.GetByID(context.Background(), 41)
	require.NoError(t, err)
	require.Empty(t, codexAccountTicketConfigOf(live).ProxyURL)
	require.Equal(t, old.ConfigRevision, codexAccountTicketConfigOf(live).Revision)
	require.NotNil(t, s.lookupOpenAICodexTicket(live, old.Model))
}

func TestCodexAccountTicketGlobalPoolChangeRejectsOldJobPublication(t *testing.T) {
	for _, stage := range []string{"harvest", "fixed"} {
		t.Run(stage, func(t *testing.T) {
			started, release := make(chan struct{}), make(chan struct{})
			var once sync.Once
			s, repo := ticketJobService(t, &codexTicketFuncUpstream{proxy: func(req *http.Request, proxy string) (*http.Response, error) {
				isFixed := req.Header.Get(openAICodexTurnStateHeader) != ""
				if (stage == "harvest" && strings.Contains(proxy, "old.example.com")) || (stage == "fixed" && isFixed) {
					once.Do(func() {
						close(started)
						select {
						case <-release:
						case <-req.Context().Done():
						}
					})
				}
				return codexTicketResponse(), nil
			}})
			pool := &codexTicketGlobalPoolRepo{pool: "http://old.example.com:8080"}
			s.settingService = NewSettingService(pool, s.cfg)
			oldJob := s.startCodexAccountTicketJob(context.Background(), 41, true)
			select {
			case <-started:
			case <-time.After(3 * time.Second):
				t.Fatal("no probe")
			}
			pool.set(s, "http://new.example.com:8080")
			close(release)
			waitCodexTicketJob(t, oldJob)
			live, err := repo.GetByID(context.Background(), 41)
			require.NoError(t, err)
			require.Nil(t, s.lookupOpenAICodexTicket(live, openAICodexTicketDefaultModel))
			// A cooldown from the old source does not delay the newly configured pool.
			newJob := s.startCodexAccountTicketJob(context.Background(), 41, false)
			require.NotSame(t, oldJob, newJob)
			waitCodexTicketJob(t, newJob)
			status, err := s.GetCodexAccountTicketStatus(context.Background(), 41)
			require.NoError(t, err)
			require.True(t, status.TicketUsable)
			require.Equal(t, "new.example.com:8080", status.ProxyDisplay)
		})
	}
}

func TestCodexAccountTicketMissingGlobalPoolNeverUsesLegacyOverride(t *testing.T) {
	s, _ := ticketJobService(t, nil)
	s.cfg.Gateway.OpenAICodexTicket.HarvestProxyURL = ""
	status, err := s.GetCodexAccountTicketStatus(context.Background(), 41)
	require.NoError(t, err)
	require.True(t, status.Enabled)
	require.False(t, status.ProxyConfigured)
	require.Equal(t, "error", status.State)
	require.Nil(t, s.startCodexAccountTicketJob(context.Background(), 41, true))
	_, err = s.HarvestCodexAccountTicket(context.Background(), 41)
	require.Error(t, err)
	_, err = s.ConfigureCodexAccountTicket(context.Background(), 41, CodexAccountTicketUpdate{Enabled: true})
	require.Error(t, err)
	status, err = s.ConfigureCodexAccountTicket(context.Background(), 41, CodexAccountTicketUpdate{Enabled: false})
	require.NoError(t, err)
	require.Equal(t, "disabled", status.State)
}
