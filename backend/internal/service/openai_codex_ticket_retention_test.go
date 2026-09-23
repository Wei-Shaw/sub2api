package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestCodexAccountTicketRejectionStopsRoundAndRetainsSavedTicket(t *testing.T) {
	for _, code := range []int{401, 403, 429} {
		for _, stage := range []string{"harvest", "fixed"} {
			t.Run(strconv.Itoa(code)+"-"+stage, func(t *testing.T) {
				var calls atomic.Int64
				started, release := make(chan struct{}), make(chan struct{})
				s, repo := ticketJobService(t, &codexTicketFuncUpstream{do: func(req *http.Request) (*http.Response, error) {
					if calls.Add(1) == 1 {
						close(started)
						select {
						case <-release:
						case <-req.Context().Done():
							return nil, req.Context().Err()
						}
					}
					resp := codexTicketResponse()
					if stage == "harvest" || req.Header.Get(openAICodexTurnStateHeader) != "" {
						resp.StatusCode = code
					}
					return resp, nil
				}})
				account, err := repo.GetByID(context.Background(), 41)
				require.NoError(t, err)
				old := verifiedTestTicket(account, 292)
				old.CapturedAt = time.Now().Add(-52 * time.Minute)
				old.ExpiresAt = old.CapturedAt.Add(time.Hour)
				require.NoError(t, repo.UpdateExtra(context.Background(), 41, map[string]any{openAICodexTicketExtraKey(old.Model): old}))
				job := s.startCodexAccountTicketJob(context.Background(), 41, true)
				select {
				case <-started:
				case <-time.After(3 * time.Second):
					t.Fatal("probe did not start")
				}
				status, err := s.GetCodexAccountTicketStatus(context.Background(), 41)
				require.NoError(t, err)
				require.Equal(t, "harvesting", status.State)
				require.True(t, status.TicketUsable)
				require.True(t, status.Refreshing)
				require.True(t, status.CapturedAt.Equal(old.CapturedAt))
				require.True(t, status.ExpiresAt.Equal(old.ExpiresAt))
				close(release)
				waitCodexTicketJob(t, job)
				wantCalls := int64(1)
				if stage == "fixed" {
					wantCalls = 2
				}
				require.Equal(t, wantCalls, calls.Load(), "must not rotate exits after an upstream rejection")
				status, err = s.GetCodexAccountTicketStatus(context.Background(), 41)
				require.NoError(t, err)
				require.Equal(t, "ready", status.State)
				require.True(t, status.TicketUsable)
				require.False(t, status.Refreshing)
				require.Contains(t, status.LastError, strconv.Itoa(code))
				require.NotNil(t, status.RetryAfter)
				require.True(t, status.RetryAfter.After(time.Now().Add(4*time.Minute)))
				require.Nil(t, s.startCodexAccountTicketJob(context.Background(), 41, false))
				encoded, err := json.Marshal(status)
				require.NoError(t, err)
				require.NotContains(t, string(encoded), old.State)
				require.NotContains(t, string(encoded), "secret")

				// Cold service: the database, not the old process cache, supplies the
				// same ticket and expiry. Failure never grants another hour of life.
				restart := ticketTestService(t, config.OpenAICodexTicketConfig{Enabled: true}, nil)
				restart.accountRepo = repo
				live, err := repo.GetByID(context.Background(), 41)
				require.NoError(t, err)
				got := restart.lookupOpenAICodexTicket(live, old.Model)
				require.NotNil(t, got)
				require.Equal(t, old.State, got.State)
				require.True(t, got.ExpiresAt.Equal(old.ExpiresAt))
				headers := http.Header{}
				require.NoError(t, restart.applyOpenAICodexTicket(context.Background(), live, old.Model, headers))
				require.Equal(t, old.State, headers.Get(openAICodexTurnStateHeader))
			})
		}
	}
}

type codexTicketFailSaveRepo struct{ *codexTicketRefreshRepo }

func (r *codexTicketFailSaveRepo) UpdateExtra(context.Context, int64, map[string]any) error {
	return errors.New("simulated database unavailable")
}

func TestCodexAccountTicketSaveFailurePreservesDurableTicket(t *testing.T) {
	s, repo := ticketJobService(t, &codexTicketFuncUpstream{do: func(*http.Request) (*http.Response, error) {
		return codexTicketResponse(), nil
	}})
	account, err := repo.GetByID(context.Background(), 41)
	require.NoError(t, err)
	old := verifiedTestTicket(account, 292)
	old.CapturedAt = time.Now().Add(-52 * time.Minute)
	old.ExpiresAt = old.CapturedAt.Add(time.Hour)
	require.NoError(t, repo.UpdateExtra(context.Background(), 41, map[string]any{openAICodexTicketExtraKey(old.Model): old}))
	s.accountRepo = &codexTicketFailSaveRepo{repo}
	waitCodexTicketJob(t, s.startCodexAccountTicketJob(context.Background(), 41, true))
	status, err := s.GetCodexAccountTicketStatus(context.Background(), 41)
	require.NoError(t, err)
	require.True(t, status.TicketUsable)
	require.Equal(t, "ready", status.State)
	require.Equal(t, "Could not save verified STATE", status.LastError)
	require.True(t, status.ExpiresAt.Equal(old.ExpiresAt))
	live, err := repo.GetByID(context.Background(), 41)
	require.NoError(t, err)
	got := s.lookupOpenAICodexTicket(live, old.Model)
	require.NotNil(t, got)
	require.True(t, got.CapturedAt.Equal(old.CapturedAt))
}

func TestCodexAccountTicketStatusNeverAdvertisesUnusableSavedTicket(t *testing.T) {
	for _, scenario := range []string{"expired", "disabled", "global-disabled", "inactive", "missing"} {
		t.Run(scenario, func(t *testing.T) {
			account := ticketTestAccount(41)
			ticket := verifiedTestTicket(account, 292)
			global := true
			switch scenario {
			case "expired":
				ticket.CapturedAt = time.Now().Add(-2 * time.Hour)
				ticket.ExpiresAt = ticket.CapturedAt.Add(time.Hour)
			case "disabled":
				ac := codexAccountTicketConfigOf(account)
				ac.Enabled = false
				account.Extra[codexAccountTicketConfigKey] = ac
			case "global-disabled":
				global = false
			case "inactive":
				account.Status = "disabled"
			}
			if scenario != "missing" {
				account.Extra[openAICodexTicketExtraKey(ticket.Model)] = ticket
			}
			s := ticketTestService(t, config.OpenAICodexTicketConfig{Enabled: global}, nil)
			s.accountRepo = &codexTicketRefreshRepo{accounts: []Account{*account}}
			status, err := s.GetCodexAccountTicketStatus(context.Background(), 41)
			require.NoError(t, err)
			require.False(t, status.TicketUsable)
			require.False(t, status.Refreshing)
			require.Nil(t, status.CapturedAt)
			require.Nil(t, status.ExpiresAt)
			require.Zero(t, status.RemainingSeconds)
		})
	}
}
