//go:build integration

package repository

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestAccountUpdatePreservesConcurrentProbeSnapshot(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	repo := newAccountRepositoryWithSQL(tx.Client(), tx, nil)
	account := mustCreateAccount(t, tx.Client(), &service.Account{
		Name:        "probe-update-preserve",
		Platform:    service.PlatformOpenAI,
		Type:        service.AccountTypeAPIKey,
REDACTED"api_key": "sk-old"REDACTED,
		Extra:       map[string]any{service.UpstreamBillingProbeEnabledExtraKey: trueREDACTED,
REDACTED)

	stale, err := repo.GetByID(ctx, account.ID)
REDACTED
	require.NotContains(t, stale.Extra, service.UpstreamBillingProbeExtraKey)
	require.NoError(t, repo.UpdateUpstreamBillingProbeSnapshot(ctx, stale, &service.UpstreamBillingProbeSnapshot{
		Status:        service.UpstreamBillingProbeStatusOK,
		LastAttemptAt: time.Now().UTC(),
REDACTED))

	stale.Name = "ordinary-edit"
	require.NoError(t, repo.Update(ctx, stale))
	got, err := repo.GetByID(ctx, account.ID)
REDACTED
	snapshot, ok := got.Extra[service.UpstreamBillingProbeExtraKey].(map[string]any)
	require.True(t, ok)
	require.Equal(t, service.UpstreamBillingProbeStatusOK, snapshot["status"])

	require.NoError(t, repo.UpdateExtra(ctx, got.ID, map[string]any{service.UpstreamBillingProbeEnabledExtraKey: falseREDACTED))
	disabled, err := repo.GetByID(ctx, account.ID)
REDACTED
	require.NotContains(t, disabled.Extra, service.UpstreamBillingProbeExtraKey)
REDACTED

func TestAccountUpdatePreservesConcurrentProbeEnableFlag(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	repo := newAccountRepositoryWithSQL(tx.Client(), tx, nil)
	account := mustCreateAccount(t, tx.Client(), &service.Account{
		Name:        "probe-update-enable",
		Platform:    service.PlatformOpenAI,
		Type:        service.AccountTypeAPIKey,
REDACTED"api_key": "sk-test"REDACTED,
		Extra: map[string]any{
			service.UpstreamBillingProbeEnabledExtraKey: true,
			service.UpstreamBillingProbeExtraKey:        map[string]any{"status": service.UpstreamBillingProbeStatusOKREDACTED,
	REDACTED,
REDACTED)

	stale, err := repo.GetByID(ctx, account.ID)
REDACTED
	require.NoError(t, repo.UpdateExtra(ctx, account.ID, map[string]any{service.UpstreamBillingProbeEnabledExtraKey: falseREDACTED))
	stale.Name = "ordinary-edit"
	require.NoError(t, repo.Update(ctx, stale))

	got, err := repo.GetByID(ctx, account.ID)
REDACTED
	require.Equal(t, false, got.Extra[service.UpstreamBillingProbeEnabledExtraKey])
	require.NotContains(t, got.Extra, service.UpstreamBillingProbeExtraKey)
REDACTED

func TestAccountUpdateClearsProbeSnapshotWhenIdentityChanges(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	repo := newAccountRepositoryWithSQL(tx.Client(), tx, nil)
	account := mustCreateAccount(t, tx.Client(), &service.Account{
		Name:        "probe-update-identity",
		Platform:    service.PlatformOpenAI,
		Type:        service.AccountTypeAPIKey,
REDACTED"api_key": "sk-old"REDACTED,
		Extra: map[string]any{
			service.UpstreamBillingProbeEnabledExtraKey: true,
			service.UpstreamBillingProbeExtraKey:        map[string]any{"status": service.UpstreamBillingProbeStatusOKREDACTED,
	REDACTED,
REDACTED)

	loaded, err := repo.GetByID(ctx, account.ID)
REDACTED
	loaded.Credentials["api_key"] = "sk-new"
	require.NoError(t, repo.Update(ctx, loaded))

	got, err := repo.GetByID(ctx, account.ID)
REDACTED
	require.NotContains(t, got.Extra, service.UpstreamBillingProbeExtraKey)
REDACTED

func TestBulkUpdateAndCredentialUpdateDeleteProbeKey(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	repo := newAccountRepositoryWithSQL(tx.Client(), tx, nil)
	newAccount := func(name string) *service.Account {
		return mustCreateAccount(t, tx.Client(), &service.Account{
			Name:        name,
			Platform:    service.PlatformOpenAI,
			Type:        service.AccountTypeAPIKey,
	REDACTED"api_key": "sk-old"REDACTED,
			Extra: map[string]any{
				service.UpstreamBillingProbeEnabledExtraKey: true,
				service.UpstreamBillingProbeExtraKey:        map[string]any{"status": service.UpstreamBillingProbeStatusOKREDACTED,
		REDACTED,
	REDACTED)
REDACTED

	bulkAccount := newAccount("probe-bulk-clear")
	_, err := repo.BulkUpdate(ctx, []int64{bulkAccount.IDREDACTED, service.AccountBulkUpdate{
		Extra: map[string]any{service.UpstreamBillingProbeExtraKey: nilREDACTED,
REDACTED)
REDACTED
	got, err := repo.GetByID(ctx, bulkAccount.ID)
REDACTED
	require.NotContains(t, got.Extra, service.UpstreamBillingProbeExtraKey)

	credentialAccount := newAccount("probe-credentials-clear")
	require.NoError(t, repo.UpdateCredentials(ctx, credentialAccount.ID, map[string]any{"api_key": "sk-new"REDACTED))
	got, err = repo.GetByID(ctx, credentialAccount.ID)
REDACTED
	require.NotContains(t, got.Extra, service.UpstreamBillingProbeExtraKey)
REDACTED

func TestProbeSnapshotCASIncludesLoadedEnabledState(t *testing.T) {
	tests := []struct {
		name           string
		loadedEnabled  bool
		concurrentFlip *bool
		wantConflict   bool
REDACTED{
		{name: "manual_false_stays_false", loadedEnabled: falseREDACTED,
		{name: "periodic_true_disabled_in_flight", loadedEnabled: true, concurrentFlip: boolPtr(false), wantConflict: trueREDACTED,
		{name: "manual_false_enabled_in_flight", loadedEnabled: false, concurrentFlip: boolPtr(true), wantConflict: trueREDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			tx := testEntTx(t)
			repo := newAccountRepositoryWithSQL(tx.Client(), tx, nil)
			account := mustCreateAccount(t, tx.Client(), &service.Account{
				Name:        "probe-enabled-cas-" + tt.name,
				Platform:    service.PlatformOpenAI,
				Type:        service.AccountTypeAPIKey,
		REDACTED"api_key": "sk-test"REDACTED,
				Extra:       map[string]any{service.UpstreamBillingProbeEnabledExtraKey: tt.loadedEnabledREDACTED,
		REDACTED)
			inFlight, err := repo.GetByID(ctx, account.ID)
		REDACTED
			if tt.concurrentFlip != nil {
				require.NoError(t, repo.UpdateExtra(ctx, account.ID, map[string]any{
					service.UpstreamBillingProbeEnabledExtraKey: *tt.concurrentFlip,
			REDACTED))
		REDACTED

			err = repo.UpdateUpstreamBillingProbeSnapshot(ctx, inFlight, &service.UpstreamBillingProbeSnapshot{
				Status:        service.UpstreamBillingProbeStatusOK,
				LastAttemptAt: time.Now().UTC(),
		REDACTED)
			if tt.wantConflict {
				require.ErrorIs(t, err, service.ErrUpstreamBillingProbeIdentityChanged)
		REDACTED else {
			REDACTED
		REDACTED
			got, err := repo.GetByID(ctx, account.ID)
		REDACTED
			if tt.wantConflict {
				require.NotContains(t, got.Extra, service.UpstreamBillingProbeExtraKey)
		REDACTED else {
				require.Contains(t, got.Extra, service.UpstreamBillingProbeExtraKey)
		REDACTED
	REDACTED)
REDACTED
REDACTED

func boolPtr(value bool) *bool {
	return &value
REDACTED

func TestProxyIdentityUpdateInvalidatesProbeAndRejectsInFlightSnapshot(t *testing.T) {
	tests := []struct {
		name             string
		includeProbeKey  bool
		probeValue       any
		wantInvalidation bool
REDACTED{
		{name: "missing_snapshot"REDACTED,
		{name: "json_null_snapshot", includeProbeKey: trueREDACTED,
		{name: "existing_snapshot", includeProbeKey: true, probeValue: map[string]any{"status": service.UpstreamBillingProbeStatusOKREDACTED, wantInvalidation: trueREDACTED,
REDACTED
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			tx := testEntTx(t)
			accountRepo := newAccountRepositoryWithSQL(tx.Client(), tx, nil)
			proxyRepo := newProxyRepositoryWithSQL(tx.Client(), tx)
			proxy := mustCreateProxy(t, tx.Client(), &service.Proxy{
				Name:     "probe-proxy",
				Protocol: "http",
				Host:     "old.example",
				Port:     8080,
				Username: "old-user",
				Password: "old-pass",
				Status:   service.StatusActive,
		REDACTED)
			extra := map[string]any{service.UpstreamBillingProbeEnabledExtraKey: trueREDACTED
			if tt.includeProbeKey {
				extra[service.UpstreamBillingProbeExtraKey] = tt.probeValue
		REDACTED
			account := mustCreateAccount(t, tx.Client(), &service.Account{
				Name:        "proxy-probe-account",
				Platform:    service.PlatformOpenAI,
				Type:        service.AccountTypeAPIKey,
		REDACTED"api_key": "sk-test"REDACTED,
				Extra:       extra,
				ProxyID:     &proxy.ID,
		REDACTED)
			inFlight, err := accountRepo.GetByID(ctx, account.ID)
		REDACTED
			require.NotNil(t, inFlight.Proxy)
			require.Equal(t, "old.example", inFlight.Proxy.Host)

			proxyToUpdate, err := proxyRepo.GetByID(ctx, proxy.ID)
		REDACTED
			proxyToUpdate.Host = "new.example"
			require.NoError(t, proxyRepo.Update(ctx, proxyToUpdate))

			got, err := accountRepo.GetByID(ctx, account.ID)
		REDACTED
			if tt.wantInvalidation || !tt.includeProbeKey {
				require.NotContains(t, got.Extra, service.UpstreamBillingProbeExtraKey)
		REDACTED else {
				require.Contains(t, got.Extra, service.UpstreamBillingProbeExtraKey)
				require.Nil(t, got.Extra[service.UpstreamBillingProbeExtraKey])
		REDACTED
			if !tt.wantInvalidation {
				require.Equal(t, inFlight.UpdatedAt, got.UpdatedAt, "missing/null snapshots must not cause an account row write")
		REDACTED
			err = accountRepo.UpdateUpstreamBillingProbeSnapshot(ctx, inFlight, &service.UpstreamBillingProbeSnapshot{
				Status:        service.UpstreamBillingProbeStatusOK,
				LastAttemptAt: time.Now().UTC(),
		REDACTED)
			require.ErrorIs(t, err, service.ErrUpstreamBillingProbeIdentityChanged)

			rows, err := tx.QueryContext(ctx, `
				SELECT COUNT(*), COALESCE(MAX(payload::text), '')
				FROM scheduler_outbox
				WHERE event_type = $1
			`, service.SchedulerOutboxEventAccountBulkChanged)
		REDACTED
			require.True(t, rows.Next())
			var (
				outboxCount int
				payloadJSON string
			)
			require.NoError(t, rows.Scan(&outboxCount, &payloadJSON))
			require.NoError(t, rows.Close())
			if tt.wantInvalidation {
				require.Equal(t, 1, outboxCount)
				var payload struct {
					AccountIDs []int64 `json:"account_ids"`
			REDACTED
				require.NoError(t, json.Unmarshal([]byte(payloadJSON), &payload))
				require.Equal(t, []int64{account.IDREDACTED, payload.AccountIDs)
		REDACTED else {
				require.Zero(t, outboxCount, "no snapshot change means no PR2 cache invalidation event")
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestSweepExpiredProxyWithoutFallbackInvalidatesOnlyExistingProbeSnapshot(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	proxyRepo := newProxyRepositoryWithSQL(tx.Client(), tx)
	accountRepo := newAccountRepositoryWithSQL(tx.Client(), tx, nil)
	past := time.Now().Add(-time.Hour)
	proxy := &service.Proxy{
		Name:           "expired-probe-proxy-none",
		Protocol:       "http",
		Host:           "127.0.0.1",
		Port:           8080,
		Status:         service.StatusActive,
		ExpiresAt:      &past,
		FallbackMode:   service.FallbackModeNone,
		ExpiryWarnDays: 7,
REDACTED
	require.NoError(t, proxyRepo.Create(ctx, proxy))
	newAccount := func(name string, probe any, includeProbe bool) *service.Account {
		extra := map[string]any{service.UpstreamBillingProbeEnabledExtraKey: trueREDACTED
		if includeProbe {
			extra[service.UpstreamBillingProbeExtraKey] = probe
	REDACTED
		return mustCreateAccount(t, tx.Client(), &service.Account{
			Name:        name,
			Platform:    service.PlatformOpenAI,
			Type:        service.AccountTypeAPIKey,
	REDACTED"api_key": "sk-test"REDACTED,
			Extra:       extra,
			ProxyID:     &proxy.ID,
	REDACTED)
REDACTED
	withSnapshot := newAccount("expired-proxy-with-snapshot", map[string]any{"status": service.UpstreamBillingProbeStatusOKREDACTED, true)
	withoutSnapshot := newAccount("expired-proxy-without-snapshot", nil, false)
	withJSONNull := newAccount("expired-proxy-null-snapshot", nil, true)
	untouchedUpdatedAt := make(map[int64]time.Time, 2)
	for _, untouched := range []*service.Account{withoutSnapshot, withJSONNullREDACTED {
		loaded, err := accountRepo.GetByID(ctx, untouched.ID)
	REDACTED
		untouchedUpdatedAt[untouched.ID] = loaded.UpdatedAt
REDACTED

	changed, err := proxyRepo.SweepExpiredProxies(ctx, time.Now())
REDACTED
	require.Zero(t, changed, "probe invalidation must not inflate the rerouted account count")

	got, err := accountRepo.GetByID(ctx, withSnapshot.ID)
REDACTED
	require.NotContains(t, got.Extra, service.UpstreamBillingProbeExtraKey)
	for _, untouched := range []*service.Account{withoutSnapshot, withJSONNullREDACTED {
		got, err = accountRepo.GetByID(ctx, untouched.ID)
	REDACTED
		require.Equal(t, untouchedUpdatedAt[untouched.ID], got.UpdatedAt)
REDACTED

	payload := latestBulkAccountOutboxPayload(t, ctx, tx)
	require.Equal(t, []int64{withSnapshot.IDREDACTED, payload)
REDACTED

func TestSweepExpiredProxyFallbackRerouteDeletesProbeSnapshot(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	proxyRepo := newProxyRepositoryWithSQL(tx.Client(), tx)
	accountRepo := newAccountRepositoryWithSQL(tx.Client(), tx, nil)
	past := time.Now().Add(-time.Hour)
	proxy := &service.Proxy{
		Name:           "expired-probe-proxy-direct",
		Protocol:       "http",
		Host:           "127.0.0.1",
		Port:           8080,
		Status:         service.StatusActive,
		ExpiresAt:      &past,
		FallbackMode:   service.FallbackModeDirect,
		ExpiryWarnDays: 7,
REDACTED
	require.NoError(t, proxyRepo.Create(ctx, proxy))
	account := mustCreateAccount(t, tx.Client(), &service.Account{
		Name:        "expired-proxy-rerouted-snapshot",
		Platform:    service.PlatformOpenAI,
		Type:        service.AccountTypeAPIKey,
REDACTED"api_key": "sk-test"REDACTED,
		Extra: map[string]any{
			service.UpstreamBillingProbeEnabledExtraKey: true,
			service.UpstreamBillingProbeExtraKey:        map[string]any{"status": service.UpstreamBillingProbeStatusOKREDACTED,
	REDACTED,
		ProxyID: &proxy.ID,
REDACTED)

	changed, err := proxyRepo.SweepExpiredProxies(ctx, time.Now())
REDACTED
	require.EqualValues(t, 1, changed)

	got, err := accountRepo.GetByID(ctx, account.ID)
REDACTED
	require.Nil(t, got.ProxyID)
	require.NotContains(t, got.Extra, service.UpstreamBillingProbeExtraKey)
	require.Equal(t, []int64{account.IDREDACTED, latestBulkAccountOutboxPayload(t, ctx, tx))
REDACTED

func latestBulkAccountOutboxPayload(t *testing.T, ctx context.Context, tx sqlQueryer) []int64 {
REDACTED
	var payloadJSON []byte
	require.NoError(t, scanSingleRow(ctx, tx, `
		SELECT payload
		FROM scheduler_outbox
		WHERE event_type = $1
		ORDER BY id DESC
		LIMIT 1
	`, []any{service.SchedulerOutboxEventAccountBulkChangedREDACTED, &payloadJSON))
	var payload struct {
		AccountIDs []int64 `json:"account_ids"`
REDACTED
	require.NoError(t, json.Unmarshal(payloadJSON, &payload))
	return payload.AccountIDs
REDACTED
