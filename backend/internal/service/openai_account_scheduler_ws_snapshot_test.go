//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestOpenAIGatewayService_SelectAccountWithScheduler_UsesWSPassthroughSnapshotFlags(t *testing.T) {
	ctx := context.Background()
	groupID := int64(10105)
	account := &Account{
		ID:          35001,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 10,
		Extra: map[string]any{
			"openai_oauth_responses_websockets_v2_mode": OpenAIWSIngressModePassthrough,
	REDACTED,
REDACTED

	snapshotCache := &openAISnapshotCacheStub{
		snapshotAccounts: []*Account{accountREDACTED,
		accountsByID:     map[int64]*Account{account.ID: accountREDACTED,
REDACTED
	cfg := &config.Config{REDACTED
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.OAuthEnabled = true
	cfg.Gateway.OpenAIWS.APIKeyEnabled = true
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
	cfg.Gateway.OpenAIWS.ModeRouterV2Enabled = true
	cfg.Gateway.OpenAIWS.IngressModeDefault = OpenAIWSIngressModeCtxPool

	svc := &OpenAIGatewayService{
		accountRepo:        stubOpenAIAccountRepo{accounts: []Account{*accountREDACTEDREDACTED,
		cache:              &stubGatewayCache{REDACTED,
		cfg:                cfg,
		schedulerSnapshot:  &SchedulerSnapshotService{cache: snapshotCacheREDACTED,
		concurrencyService: NewConcurrencyService(stubConcurrencyCache{REDACTED),
REDACTED

	selection, decision, err := svc.SelectAccountWithScheduler(
		ctx,
		&groupID,
		"",
		"session_hash_ws_passthrough",
		"gpt-5.1",
		nil,
		OpenAIUpstreamTransportResponsesWebsocketV2,
	)
REDACTED
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	require.Equal(t, account.ID, selection.Account.ID)
	require.Equal(t, openAIAccountScheduleLayerLoadBalance, decision.Layer)
REDACTED
