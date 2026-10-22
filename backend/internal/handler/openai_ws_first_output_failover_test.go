package handler

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	coderws "github.com/coder/websocket"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestOpenAIWSFirstOutputFailoverBudget(t *testing.T) {
	for _, tc := range []struct {
		name         string
		responses    []string
		clientEvents []string
		wantAccounts []int64
		maxSwitches  int
	}{
		{
			name: "second_timeout_stops_before_third_account", maxSwitches: 3,
			responses: []string{"timeout", "timeout"}, clientEvents: []string{""},
			wantAccounts: []int64{801, 802},
		},
		{
			name: "first_timeout_can_recover", maxSwitches: 3,
			responses: []string{"timeout", "response.completed"}, clientEvents: []string{"response.completed"},
			wantAccounts: []int64{801, 802},
		},
		{
			name: "other_failure_does_not_consume_timeout_budget", maxSwitches: 3,
			responses: []string{"429", "timeout", "response.completed"}, clientEvents: []string{"response.completed"},
			wantAccounts: []int64{801, 802, 803},
		},
		{
			name: "timeout_does_not_block_other_failover", maxSwitches: 3,
			responses: []string{"timeout", "429", "response.completed"}, clientEvents: []string{"response.completed"},
			wantAccounts: []int64{801, 802, 803},
		},
		{
			name: "timeout_still_obeys_total_switch_budget", maxSwitches: 1,
			responses: []string{"429", "timeout"}, clientEvents: []string{""},
			wantAccounts: []int64{801, 802},
		},
		{
			name: "later_passthrough_timeout_still_requires_reconnect", maxSwitches: 3,
			responses: []string{"response.completed", "timeout"}, clientEvents: []string{"response.completed", ""},
			wantAccounts: []int64{801, 801},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			conn, _, attempts := newOpenAIWSTurnBudgetSession(t, service.OpenAIWSIngressModePassthrough, tc.responses, nil, func(cfg *config.Config) {
				cfg.Gateway.MaxAccountSwitches = tc.maxSwitches
				cfg.Gateway.OpenAIFirstOutputTimeoutSeconds = 1
			})
			for turn, eventType := range tc.clientEvents {
				readOpenAIWSFirstOutputBudgetTurn(t, conn, turn+1, eventType)
			}
			accounts, _, _ := attempts()
			require.Equal(t, tc.wantAccounts, accounts)
		})
	}
}

func TestOpenAIWSFirstOutputFailoverBudgetResetsAfterSuccessfulTurn(t *testing.T) {
	for _, mode := range []string{service.OpenAIWSIngressModeCtxPool, service.OpenAIWSIngressModeHTTPBridge} {
		t.Run(mode, func(t *testing.T) {
			conn, repo, attempts := newOpenAIWSTurnBudgetSession(t, service.OpenAIWSIngressModePassthrough,
				[]string{"timeout", "response.completed", "429", "timeout", "response.completed"}, nil,
				func(cfg *config.Config) {
					cfg.Gateway.MaxAccountSwitches = 3
					cfg.Gateway.OpenAIFirstOutputTimeoutSeconds = 1
				})
			repo.mu.Lock()
			repo.accounts[1].Extra["openai_apikey_responses_websockets_v2_mode"] = mode
			repo.mu.Unlock()
			readOpenAIWSFirstOutputBudgetTurn(t, conn, 1, "response.completed")
			readOpenAIWSFirstOutputBudgetTurn(t, conn, 2, "response.completed")
			accounts, turns, _ := attempts()
			require.Equal(t, []int64{801, 802, 802, 801, 803}, accounts)
			require.Equal(t, []string{"turn-1", "turn-1", "turn-2", "turn-2", "turn-2"}, turns)
		})
	}
}

func readOpenAIWSFirstOutputBudgetTurn(t *testing.T, conn *coderws.Conn, turn int, wantEvent string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	payload := fmt.Sprintf(`{"type":"response.create","model":"gpt-5.1","instructions":"turn-%d","input":[{"role":"user","content":"hello"}]}`, turn)
	require.NoError(t, conn.Write(ctx, coderws.MessageText, []byte(payload)))
	_, event, err := conn.Read(ctx)
	if wantEvent == "" {
		var closeErr coderws.CloseError
		require.ErrorAs(t, err, &closeErr)
		return
	}
	require.NoError(t, err)
	require.Equal(t, wantEvent, gjson.GetBytes(event, "type").String())
}
