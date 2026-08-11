package service

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func legacyProfitDiagnosticService(accounts []Account) *OpenAIGatewayService {
	return &OpenAIGatewayService{
		accountRepo:        stubOpenAIAccountRepo{accounts: accountsREDACTED,
		cfg:                &config.Config{REDACTED,
		rateLimitService:   newOpenAIAdvancedSchedulerRateLimitService("false"),
		concurrencyService: NewConcurrencyService(stubConcurrencyCache{REDACTED),
REDACTED
REDACTED

func legacyProfitDiagnosticAccount(id int64) *Account {
	now := time.Now()
	a := upstreamCostTestAccount(id, UpstreamBillingProbeStatusOK, 0.9, now.Add(-time.Minute), 30*time.Minute)
	a.Status = StatusActive
	a.Schedulable = true
	a.Concurrency = 1
	return a
REDACTED

func TestSelectAccountWithScheduler_LegacyProfitDiagnostics(t *testing.T) {
	groupID := int64(5313)
	ctx := profitControlTestCtx(profitControlTestGroup(groupID, 0.5, 0))

	t.Run("threshold reports deterministic pool count", func(t *testing.T) {
		account := legacyProfitDiagnosticAccount(53131)
		profitControlTestAccountWithRate(account, 0.9)
		svc := legacyProfitDiagnosticService([]Account{*accountREDACTED)

		selection, _, err := svc.SelectAccountWithScheduler(ctx, &groupID, "", "", "gpt-test", nil, OpenAIUpstreamTransportAny, false)
		require.Nil(t, selection)
		require.ErrorIs(t, err, ErrNoAvailableAccounts)
		require.Contains(t, err.Error(), "pool=1, filtered: profit_threshold=1")
REDACTED)

	t.Run("missing account rate reports invalid rate", func(t *testing.T) {
		account := upstreamCostTestOAuthAccount(53132)
		account.Status = StatusActive
		account.Schedulable = true
		account.Concurrency = 1
		svc := legacyProfitDiagnosticService([]Account{*accountREDACTED)

		selection, _, err := svc.SelectAccountWithScheduler(ctx, &groupID, "", "", "gpt-test", nil, OpenAIUpstreamTransportAny, false)
		require.Nil(t, selection)
		require.ErrorIs(t, err, ErrNoAvailableAccounts)
		require.Contains(t, err.Error(), "profit_invalid_account_rate=1")
REDACTED)

	t.Run("model support gate does not report profit reasons", func(t *testing.T) {
		account := legacyProfitDiagnosticAccount(53133)
		profitControlTestAccountWithRate(account, 0.9)
		account.Credentials = map[string]any{"model_mapping": map[string]any{"other-model": "other-model"REDACTEDREDACTED
		svc := legacyProfitDiagnosticService([]Account{*accountREDACTED)

		selection, _, err := svc.SelectAccountWithScheduler(ctx, &groupID, "", "", "gpt-test", nil, OpenAIUpstreamTransportAny, false)
		require.Nil(t, selection)
		require.ErrorIs(t, err, ErrNoAvailableAccounts)
		require.Contains(t, err.Error(), "ineligible=1")
		require.NotContains(t, err.Error(), openAIProfitFilterReasonThreshold)
		require.NotContains(t, err.Error(), openAIProfitFilterReasonInvalidAccountRate)
REDACTED)

	t.Run("compact preserves compact sentinel", func(t *testing.T) {
		account := legacyProfitDiagnosticAccount(53134)
		account.Extra = map[string]any{"openai_compact_supported": falseREDACTED
		profitControlTestAccountWithRate(account, 0.4)
		svc := legacyProfitDiagnosticService([]Account{*accountREDACTED)

		selection, _, err := svc.SelectAccountWithScheduler(ctx, &groupID, "", "", "gpt-test", nil, OpenAIUpstreamTransportAny, true)
		require.Nil(t, selection)
		require.ErrorIs(t, err, ErrNoAvailableCompactAccounts)
		require.False(t, strings.Contains(err.Error(), "pool="), err)
		require.False(t, errors.Is(err, ErrNoAvailableAccounts))
REDACTED)
REDACTED
