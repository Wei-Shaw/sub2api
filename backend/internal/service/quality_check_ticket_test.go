//go:build unit

package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type qualityTicketRepo struct {
	AccountRepository
	account *Account
}

func (r *qualityTicketRepo) GetByID(context.Context, int64) (*Account, error)         { return r.account, nil }
func (r *qualityTicketRepo) UpdateExtra(context.Context, int64, map[string]any) error { return nil }

func qualityTicketResponse() *http.Response {
	return newJSONResponse(200, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"答案：21颗\"}\n\ndata: {\"type\":\"response.completed\"}\n\n")
}

func TestQualityTicketRequestUsesMappedModelAndFreshTicket(t *testing.T) {
	ctx := context.Background()
	account := ticketTestAccount(41)
	account.Credentials["model_mapping"] = map[string]any{"quality-alias": "gpt-6-astra"}
	gateway := ticketTestService(t, config.OpenAICodexTicketConfig{Enabled: true, FailClosed: true}, nil)
	upstream := &queuedHTTPUpstream{responses: []*http.Response{qualityTicketResponse(), qualityTicketResponse(), qualityTicketResponse()}}
	svc := &AccountTestService{accountRepo: &qualityTicketRepo{account: account}, openaiGatewayService: gateway, httpUpstream: upstream}
	var first *OpenAICodexTicketReceipt
	for i, letter := range []string{"A", "B", "C"} {
		state := openAICodexTicketStatePrefix + strings.Repeat(letter, 286)
		ticket := &openAICodexTicket{Model: "gpt-6-astra", State: state, Length: 292, CapturedAt: time.Now().Add(time.Duration(i) * time.Second), ExpiresAt: time.Now().Add(time.Hour)}
		if i == 2 {
			// An external refresh persists a newer ticket in the account row.
			account.Extra = map[string]any{openAICodexTicketExtraKey(ticket.Model): ticket}
		} else {
			gateway.storeOpenAICodexTicket(ctx, account, ticket)
		}
		result := svc.runQualityPrompt(ctx, account.ID, "quality-alias", "candy", QualityCandyPrompt)
		require.Equal(t, "pass", result.Status, result.Note)
		require.Len(t, upstream.requests, i+1)
		req := upstream.requests[i]
		require.Equal(t, state, req.Header.Get(openAICodexTurnStateHeader))
		body, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		require.Equal(t, "gpt-6-astra", gjson.GetBytes(body, "model").String())
		require.Equal(t, QualityCandyPrompt, gjson.GetBytes(body, "input.0.content.0.text").String())
		require.Equal(t, "injected", result.Ticket.Status)
		require.Equal(t, int64(41), result.Ticket.AccountID)
		require.Equal(t, "gpt-6-astra", result.Ticket.Model)
		require.Equal(t, 292, result.Ticket.Length)
		require.True(t, result.Ticket.CapturedAt.Equal(ticket.CapturedAt))
		serialized, err := json.Marshal(result)
		require.NoError(t, err)
		require.NotContains(t, string(serialized), state)
		require.NotContains(t, string(serialized), "\"state\"")
		if first == nil {
			first = result.Ticket
		} else {
			require.True(t, first.CapturedAt.Before(*result.Ticket.CapturedAt))
		}
	}
}

func TestQualityTicketMissingExpiredAndWrongScopeNeverDispatch(t *testing.T) {
	for _, scenario := range []string{"missing", "expired", "other_account", "other_model", "invalid_length"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			account := ticketTestAccount(41)
			// Quality checks still block missing tickets under fail-open traffic policy.
			gateway := ticketTestService(t, config.OpenAICodexTicketConfig{Enabled: true, FailClosed: false}, nil)
			ticket := &openAICodexTicket{Model: "gpt-6-astra", State: fakeCodexTicketState(292), Length: 292, CapturedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour)}
			owner := account
			expected := "missing"
			switch scenario {
			case "expired":
				ticket.ExpiresAt = time.Now().Add(-time.Minute)
				account.Extra = map[string]any{openAICodexTicketExtraKey(ticket.Model): ticket}
				expected = "expired"
			case "other_account":
				owner = ticketTestAccount(42)
			case "other_model":
				ticket.Model = "gpt-5.6-sol"
			case "invalid_length":
				ticket.State = fakeCodexTicketState(312)
				ticket.Length = 312
			}
			if scenario != "missing" {
				gateway.storeOpenAICodexTicket(ctx, owner, ticket)
			}
			upstream := &queuedHTTPUpstream{}
			svc := &AccountTestService{accountRepo: &qualityTicketRepo{account: account}, openaiGatewayService: gateway, httpUpstream: upstream}
			result := svc.runQualityPrompt(ctx, account.ID, "gpt-6-astra", "candy", QualityCandyPrompt)
			require.Equal(t, "waiting_ticket", result.Status)
			require.Equal(t, expected, result.Ticket.Status)
			require.Empty(t, upstream.requests)
			require.Empty(t, result.Text)
		})
	}
}

func TestQualityTicketDisabledAndUngatedModelAreExplicit(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		model, expected := "gpt-6-astra", "disabled"
		if enabled {
			model, expected = "gpt-5.5", "not_required"
		}
		account := ticketTestAccount(41)
		gateway := ticketTestService(t, config.OpenAICodexTicketConfig{Enabled: enabled, FailClosed: true}, nil)
		upstream := &queuedHTTPUpstream{responses: []*http.Response{qualityTicketResponse()}}
		svc := &AccountTestService{accountRepo: &qualityTicketRepo{account: account}, openaiGatewayService: gateway, httpUpstream: upstream}
		result := svc.runQualityPrompt(context.Background(), account.ID, model, "candy", QualityCandyPrompt)
		require.Equal(t, "pass", result.Status)
		require.Equal(t, expected, result.Ticket.Status)
		require.Len(t, upstream.requests, 1)
		require.Empty(t, upstream.requests[0].Header.Get(openAICodexTurnStateHeader))
	}
}
