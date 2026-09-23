package service

import (
	"context"
	"net/http"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestCodexAccountTicketManualPlanDefaultAndValidation(t *testing.T) {
	account := ticketTestAccount(41)
	// Deliberately conflicting upstream metadata must not auto-select Team.
	account.Credentials["plan_type"] = "team"
	repo := &codexTicketRefreshRepo{accounts: []Account{*account}}
	s := ticketTestService(t, config.OpenAICodexTicketConfig{}, nil)
	s.accountRepo = repo
	status, err := s.GetCodexAccountTicketStatus(context.Background(), 41)
	require.NoError(t, err)
	require.Equal(t, "pro", status.TicketPlan)
	require.Equal(t, 292, status.TargetLength)
	status, err = s.ConfigureCodexAccountTicket(context.Background(), 41, CodexAccountTicketUpdate{Enabled: true, TicketPlan: "team"})
	require.NoError(t, err)
	require.Equal(t, "team", status.TicketPlan)
	require.Equal(t, 332, status.TargetLength)
	// Older clients that omit the field must preserve a manual Team selection.
	status, err = s.ConfigureCodexAccountTicket(context.Background(), 41, CodexAccountTicketUpdate{Enabled: true})
	require.NoError(t, err)
	require.Equal(t, "team", status.TicketPlan)
	for _, invalid := range []string{"plus", "auto", "292", " "} {
		_, err = s.ConfigureCodexAccountTicket(context.Background(), 41, CodexAccountTicketUpdate{Enabled: true, TicketPlan: invalid})
		require.Error(t, err)
	}
	status, err = s.GetCodexAccountTicketStatus(context.Background(), 41)
	require.NoError(t, err)
	require.Equal(t, "team", status.TicketPlan)
}

func TestCodexAccountTicketPlanChangeInvalidatesTicketAndJob(t *testing.T) {
	account := ticketTestAccount(41)
	oldTicket := verifiedTestTicket(account, 292)
	account.Extra[openAICodexTicketExtraKey(oldTicket.Model)] = oldTicket
	repo := &codexTicketRefreshRepo{accounts: []Account{*account}}
	s := ticketTestService(t, config.OpenAICodexTicketConfig{}, nil)
	s.accountRepo = repo
	s.openaiCodexTickets.Store(openAICodexTicketKey(41, oldTicket.Model), oldTicket)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.openaiCodexAccountJobs = map[int64]*codexAccountTicketJob{41: {revision: oldTicket.ConfigRevision, cancel: cancel, running: true}}
	_, err := s.ConfigureCodexAccountTicket(context.Background(), 41, CodexAccountTicketUpdate{Enabled: true, TicketPlan: "team"})
	require.NoError(t, err)
	require.ErrorIs(t, ctx.Err(), context.Canceled)
	require.Empty(t, s.openaiCodexAccountJobs)
	live, err := repo.GetByID(context.Background(), 41)
	require.NoError(t, err)
	ac := codexAccountTicketConfigOf(live)
	require.NotEqual(t, oldTicket.ConfigRevision, ac.Revision)
	require.Nil(t, s.lookupOpenAICodexTicket(live, ac.Model))
	require.Nil(t, live.Extra[openAICodexTicketExtraKey(ac.Model)])
	require.Equal(t, account.ProxyID, live.ProxyID)
	require.Empty(t, ac.ProxyURL)
	// A ticket still cannot cross the plan boundary if its revision was forged.
	oldTicket.ConfigRevision = ac.Revision
	require.False(t, oldTicket.validFor(live, ac, time.Now()))
	teamTicket := verifiedTestTicket(live, 332)
	require.True(t, teamTicket.validFor(live, ac, time.Now()))
	_, err = s.ConfigureCodexAccountTicket(context.Background(), 41, CodexAccountTicketUpdate{Enabled: true, TicketPlan: "pro"})
	require.NoError(t, err)
	live, err = repo.GetByID(context.Background(), 41)
	require.NoError(t, err)
	ac = codexAccountTicketConfigOf(live)
	teamTicket.ConfigRevision = ac.Revision
	require.False(t, teamTicket.validFor(live, ac, time.Now()))
}

func TestCodexAccountTicketHarvestEnforcesManualPlanBeforeFixedReplay(t *testing.T) {
	for _, tc := range []struct {
		plan        string
		wrong, want int
	}{{"pro", 332, 292}, {"team", 292, 332}, {"pro", 312, 292}} {
		t.Run(tc.plan+"-reject-"+strconv.Itoa(tc.wrong), func(t *testing.T) {
			var calls atomic.Int64
			u := &codexTicketFuncUpstream{proxy: func(req *http.Request, proxy string) (*http.Response, error) {
				n := calls.Add(1)
				response := codexTicketResponse()
				length := tc.want
				if n == 1 {
					length = tc.wrong
				}
				response.Header.Set(openAICodexTurnStateHeader, fakeCodexTicketState(length))
				if n < 3 {
					require.Empty(t, req.Header.Get(openAICodexTurnStateHeader))
					require.Contains(t, proxy, "us.1024proxy.io")
				} else {
					require.Equal(t, int64(3), n)
					require.Len(t, req.Header.Get(openAICodexTurnStateHeader), tc.want)
					require.Equal(t, "http://fixed.example.com:8080", proxy)
				}
				return response, nil
			}}
			s, repo := ticketJobService(t, u)
			ac := codexAccountTicketConfigOf(&repo.accounts[0])
			ac.TicketPlan = tc.plan
			repo.accounts[0].Extra[codexAccountTicketConfigKey] = ac
			waitCodexTicketJob(t, s.startCodexAccountTicketJob(context.Background(), 41, true))
			require.Equal(t, int64(3), calls.Load())
			live, err := repo.GetByID(context.Background(), 41)
			require.NoError(t, err)
			ticket := s.lookupOpenAICodexTicket(live, ac.Model)
			require.NotNil(t, ticket)
			require.Equal(t, tc.want, ticket.Length)
			require.Equal(t, 2, ticket.Attempts)
			status, err := s.GetCodexAccountTicketStatus(context.Background(), 41)
			require.NoError(t, err)
			require.Equal(t, tc.plan, status.TicketPlan)
			require.Equal(t, "ready", status.State)
		})
	}
}
