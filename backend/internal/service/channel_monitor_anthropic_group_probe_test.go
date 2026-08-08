package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type fakeAnthropicGroupProbeGateway struct {
	accounts         []*Account
	excludedBySelect []map[int64]struct{}
	forwardCalls     []int64
}

func (f *fakeAnthropicGroupProbeGateway) SelectAccountWithLoadAwareness(_ context.Context, _ *int64, _ string, _ string, excludedIDs map[int64]struct{}, _ string, _ int64) (*AccountSelectionResult, error) {
	copied := make(map[int64]struct{}, len(excludedIDs))
	for id := range excludedIDs {
		copied[id] = struct{}{}
	}
	f.excludedBySelect = append(f.excludedBySelect, copied)
	for _, account := range f.accounts {
		if _, excluded := excludedIDs[account.ID]; excluded {
			continue
		}
		return &AccountSelectionResult{Account: account, Acquired: true}, nil
	}
	return nil, errors.New("no available accounts")
}

func (f *fakeAnthropicGroupProbeGateway) Forward(_ context.Context, c *gin.Context, account *Account, parsed *ParsedRequest) (*ForwardResult, error) {
	f.forwardCalls = append(f.forwardCalls, account.ID)
	if account.ID == 1 {
		return nil, &UpstreamFailoverError{StatusCode: http.StatusBadGateway}
	}
	prompt := gjson.GetBytes(parsed.Body.Bytes(), "messages.0.content").String()
	expected := monitorExpectedFromPrompt(prompt)
	c.JSON(http.StatusOK, gin.H{
		"id":   "msg-monitor",
		"type": "message",
		"content": []gin.H{{
			"type": "text",
			"text": expected,
		}},
	})
	return &ForwardResult{Model: parsed.Model}, nil
}

func TestRunAnthropicGroupAccountProbeForModel_TriesNextAccountUntilSuccess(t *testing.T) {
	gateway := &fakeAnthropicGroupProbeGateway{accounts: []*Account{
		{ID: 1, Name: "bad-kiro", Platform: PlatformAnthropic, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1},
		{ID: 2, Name: "good-kiro", Platform: PlatformAnthropic, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1},
	}}
	svc := NewChannelMonitorService(nil, nil)
	svc.SetGroupAccountProbeDependencies(nil, gateway, nil)

	result := svc.runAnthropicGroupAccountProbeForModel(context.Background(), &ChannelMonitor{
		Provider:     MonitorProviderAnthropic,
		PrimaryModel: "claude-sonnet-4-6",
	}, "claude-sonnet-4-6", nil, &monitorProbeGroup{id: 9})

	require.NotNil(t, result)
	require.Equal(t, MonitorStatusOperational, result.Status)
	require.Contains(t, result.Message, "account 2")
	require.Equal(t, []int64{1, 2}, gateway.forwardCalls)
	require.Len(t, gateway.excludedBySelect, 2)
	require.NotContains(t, gateway.excludedBySelect[0], int64(1))
	require.Contains(t, gateway.excludedBySelect[1], int64(1))
}

func monitorExpectedFromPrompt(prompt string) string {
	re := regexp.MustCompile(`(?s)Q:\s*(-?\d+)\s*([+-])\s*(-?\d+)\s*=\s*\?\s*A:\s*$`)
	matches := re.FindStringSubmatch(prompt)
	if len(matches) != 4 {
		return "0"
	}
	a, errA := strconv.Atoi(matches[1])
	b, errB := strconv.Atoi(matches[3])
	if errA != nil || errB != nil {
		return "0"
	}
	if matches[2] == "+" {
		return fmt.Sprintf("%d", a+b)
	}
	return fmt.Sprintf("%d", a-b)
}
