package service

import (
	"context"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type codexTicketFuncUpstream struct {
	HTTPUpstream
	do    func(*http.Request) (*http.Response, error)
	proxy func(*http.Request, string) (*http.Response, error)
}

func (u *codexTicketFuncUpstream) Do(req *http.Request, p string, _ int64, _ int) (*http.Response, error) {
	if u.proxy != nil {
		return u.proxy(req, p)
	}
	return u.do(req)
}
func codexTicketResponse() *http.Response { return codexModelResponse(openAICodexTicketDefaultModel) }
func waitCodexTicketJob(t *testing.T, job *codexAccountTicketJob) {
	t.Helper()
	require.NotNil(t, job)
	select {
	case <-job.done:
	case <-time.After(15 * time.Second):
		t.Fatal("ticket job did not finish")
	}
}
func ticketJobService(t *testing.T, u HTTPUpstream) (*OpenAIGatewayService, *codexTicketRefreshRepo) {
	t.Helper()
	r := &codexTicketRefreshRepo{accounts: []Account{*ticketTestAccount(41)}}
	s := ticketTestService(t, config.OpenAICodexTicketConfig{Enabled: true, HarvestAttemptTimeoutSeconds: 2}, u)
	s.accountRepo = r
	t.Cleanup(s.StopOpenAICodexTicketHarvester)
	return s, r
}
func TestCodexAccountTicketHarvestAndFixedReplay(t *testing.T) {
	var calls atomic.Int64
	var mu sync.Mutex
	var urls []string
	u := &codexTicketFuncUpstream{proxy: func(req *http.Request, p string) (*http.Response, error) {
		n := calls.Add(1)
		mu.Lock()
		urls = append(urls, p)
		mu.Unlock()
		require.True(t, req.Close)
		require.Equal(t, HTTPUpstreamProfileOpenAIHarvest, HTTPUpstreamProfileFromContext(req.Context()))
		if n == 1 {
			return codexModelResponse("gpt-5.6-luna"), nil
		}
		if req.Header.Get(openAICodexTurnStateHeader) == "" {
			require.Contains(t, p, "us.1024proxy.io")
		} else {
			require.Equal(t, "http://fixed.example.com:8080", p)
			require.Len(t, req.Header.Get(openAICodexTurnStateHeader), 292)
		}
		return codexTicketResponse(), nil
	}}
	s, r := ticketJobService(t, u)
	job := s.startCodexAccountTicketJob(context.Background(), 41, true)
	waitCodexTicketJob(t, job)
	require.Equal(t, int64(3), calls.Load())
	require.NotEqual(t, urls[0], urls[1])
	a, _ := r.GetByID(context.Background(), 41)
	ticket := s.lookupOpenAICodexTicket(a, openAICodexTicketDefaultModel)
	require.NotNil(t, ticket)
	require.True(t, ticket.Verified)
	require.Equal(t, 2, ticket.Attempts)
	status, err := s.GetCodexAccountTicketStatus(context.Background(), 41)
	require.NoError(t, err)
	require.Equal(t, "ready", status.State)
	require.NoError(t, s.applyOpenAICodexTicket(context.Background(), a, openAICodexTicketDefaultModel, http.Header{}))
	s.refreshOpenAICodexTickets(context.Background())
	require.Equal(t, int64(3), calls.Load())
	restart := ticketTestService(t, config.OpenAICodexTicketConfig{Enabled: true}, nil)
	restart.accountRepo = r
	require.NotNil(t, restart.lookupOpenAICodexTicket(a, openAICodexTicketDefaultModel))
}
func TestCodexAccountTicketFixedProxyMismatchRejectsAndBoundsAttempts(t *testing.T) {
	var calls atomic.Int64
	u := &codexTicketFuncUpstream{do: func(req *http.Request) (*http.Response, error) {
		calls.Add(1)
		if req.Header.Get(openAICodexTurnStateHeader) != "" {
			return codexModelResponse("gpt-5.6-luna"), nil
		}
		return codexTicketResponse(), nil
	}}
	s, r := ticketJobService(t, u)
	waitCodexTicketJob(t, s.startCodexAccountTicketJob(context.Background(), 41, true))
	require.Equal(t, int64(16), calls.Load())
	a, _ := r.GetByID(context.Background(), 41)
	require.Nil(t, s.lookupOpenAICodexTicket(a, openAICodexTicketDefaultModel))
	status, err := s.GetCodexAccountTicketStatus(context.Background(), 41)
	require.NoError(t, err)
	require.Equal(t, "error", status.State)
	require.Equal(t, 8, status.Attempts)
	s.refreshOpenAICodexTickets(context.Background())
	require.Equal(t, int64(16), calls.Load())
}
func TestCodexAccountTicketDisableDuringJobPreventsLatePublication(t *testing.T) {
	for _, stage := range []string{"harvest", "fixed"} {
		t.Run(stage, func(t *testing.T) {
			started := make(chan struct{})
			release := make(chan struct{})
			var once sync.Once
			s, r := ticketJobService(t, &codexTicketFuncUpstream{do: func(req *http.Request) (*http.Response, error) {
				if (stage == "harvest" && req.Header.Get(openAICodexTurnStateHeader) == "") || (stage == "fixed" && req.Header.Get(openAICodexTurnStateHeader) != "") {
					once.Do(func() { close(started) })
					<-release
				}
				return codexTicketResponse(), nil
			}})
			job := s.startCodexAccountTicketJob(context.Background(), 41, true)
			<-started
			require.Same(t, job, s.startCodexAccountTicketJob(context.Background(), 41, true))
			status, err := s.ConfigureCodexAccountTicket(context.Background(), 41, CodexAccountTicketUpdate{Enabled: false})
			require.NoError(t, err)
			require.Equal(t, "disabled", status.State)
			close(release)
			waitCodexTicketJob(t, job)
			a, _ := r.GetByID(context.Background(), 41)
			require.Nil(t, s.lookupOpenAICodexTicket(a, openAICodexTicketDefaultModel))
			require.False(t, s.openAICodexTicketBlocksAccount(a, openAICodexTicketDefaultModel))
		})
	}
}
func TestCodexTicketHarvesterStopCancelsInFlightWork(t *testing.T) {
	started := make(chan struct{})
	var once sync.Once
	s, _ := ticketJobService(t, &codexTicketFuncUpstream{do: func(req *http.Request) (*http.Response, error) {
		once.Do(func() { close(started) })
		<-req.Context().Done()
		return nil, req.Context().Err()
	}})
	s.StartOpenAICodexTicketHarvester()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("no probe")
	}
	done := make(chan struct{})
	go func() { s.StopOpenAICodexTicketHarvester(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("stop did not cancel probe")
	}
	s.StartOpenAICodexTicketHarvester()
}
func TestCodexTicketProbeBypassesPluginDuringWiring(t *testing.T) {
	manager := &PluginManager{}
	manager.route.Store(&pluginRoute{pluginID: 1, rolloutPercent: 100, unavailable: "must not handle probes"})
	s := ticketTestService(t, config.OpenAICodexTicketConfig{}, &codexTicketFuncUpstream{do: func(req *http.Request) (*http.Response, error) {
		require.True(t, req.Close)
		return codexTicketResponse(), nil
	}})
	s.SetPluginManager(manager)
	_, status, err := s.fireOpenAICodexTicketProbe(context.Background(), ticketTestAccount(41), "token", openAICodexTicketDefaultModel, "http://proxy:8080", time.Second)
	require.NoError(t, err)
	require.Equal(t, 200, status)
}
func TestCodexTicketPolicyExemptsCredentialShadows(t *testing.T) {
	a := ticketTestAccount(42)
	parent := int64(41)
	a.ParentAccountID = &parent
	s := ticketTestService(t, config.OpenAICodexTicketConfig{Enabled: true}, nil)
	require.False(t, s.openAICodexTicketBlocksAccount(a, openAICodexTicketDefaultModel))
	require.NoError(t, s.applyOpenAICodexTicket(context.Background(), a, openAICodexTicketDefaultModel, http.Header{}))
	require.Empty(t, OpenAICodexTicketStatuses(a, config.OpenAICodexTicketConfig{Enabled: true}, time.Now()))
}

// A master-switch update cannot publish a response already in flight.
type codexTicketAtomicSettingRepo struct {
	SettingRepository
	enabled atomic.Bool
}

func (r *codexTicketAtomicSettingRepo) GetValue(_ context.Context, key string) (string, error) {
	if key == SettingKeyOpenAICodexTicketEnabled {
		if r.enabled.Load() {
			return "true", nil
		}
		return "false", nil
	}
	return "", ErrSettingNotFound
}
func TestCodexTicketGlobalDisablePreventsLatePublication(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	s, r := ticketJobService(t, &codexTicketFuncUpstream{do: func(req *http.Request) (*http.Response, error) {
		once.Do(func() { close(started) })
		<-release
		return codexTicketResponse(), nil
	}})
	settings := &codexTicketAtomicSettingRepo{}
	settings.enabled.Store(true)
	s.settingService = NewSettingService(settings, s.cfg)
	job := s.startCodexAccountTicketJob(context.Background(), 41, true)
	<-started
	settings.enabled.Store(false)
	s.settingService.InvalidateOpenAICodexTicketEnabledCache()
	close(release)
	waitCodexTicketJob(t, job)
	account, _ := r.GetByID(context.Background(), 41)
	require.Nil(t, s.lookupOpenAICodexTicket(account, openAICodexTicketDefaultModel))
	require.NoError(t, s.applyOpenAICodexTicket(context.Background(), account, openAICodexTicketDefaultModel, http.Header{}))
}
