//go:build integration

package repository

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestListDueOllamaCloudUsageAccountsOrderingLimitAndProxyHydration(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	repo := newAccountRepositoryWithSQL(tx.Client(), tx, nil)
	now := time.Date(2026, time.July, 22, 12, 0, 0, 0, time.UTC)
	proxy := mustCreateProxy(t, tx.Client(), &service.Proxy{
		Name: "ollama-due-proxy", Protocol: "http", Host: "127.0.0.1", Port: 3128,
		Username: "user", Password: "pass", Status: service.StatusActive,
REDACTED)

	createAccount := func(name, baseURL string, proxyID *int64, nextRefreshAt *time.Time) *service.Account {
	REDACTED
		extra := map[string]any{
			service.OllamaCloudUsageSessionExtraKey:     "cipher:wos-session=fixture",
			service.OllamaCloudUsageAutoRefreshExtraKey: true,
	REDACTED
		if nextRefreshAt != nil {
			extra[service.OllamaCloudUsageSnapshotExtraKey] = map[string]any{
				"status": service.OllamaCloudUsageStatusOK, "next_refresh_at": nextRefreshAt.UTC().Format(time.RFC3339Nano),
		REDACTED
	REDACTED
		return mustCreateAccount(t, tx.Client(), &service.Account{
			Name: name, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey,
	REDACTED"api_key": name, "base_url": baseURLREDACTED,
			Extra:       extra, ProxyID: proxyID,
	REDACTED)
REDACTED

	uppercasePath := createAccount("ollama-uppercase-path", "https://ollama.com/V1", nil, nil)
	missingSnapshot := createAccount("ollama-due-missing", "HTTPS://WWW.OLLAMA.COM:443/v1", &proxy.ID, nil)
	oldest := now.Add(-2 * time.Hour)
	due := createAccount("ollama-due-oldest", "https://ollama.com", nil, &oldest)
	future := now.Add(time.Minute)
	_ = createAccount("ollama-not-due", "https://ollama.com", nil, &future)
	_ = createAccount("ollama-ineligible", "https://ollama.com.evil.test", nil, nil)

	accounts, err := repo.ListDueOllamaCloudUsageAccounts(ctx, now, 2)

REDACTED
	require.Len(t, accounts, 2)
	require.Equal(t, missingSnapshot.ID, accounts[0].ID)
	require.Equal(t, due.ID, accounts[1].ID)
	require.NotContains(t, accountIDs(accounts), uppercasePath.ID)
	require.NotNil(t, accounts[0].Proxy)
	require.Equal(t, proxy.ID, accounts[0].Proxy.ID)
	require.Equal(t, proxy.URL(), accounts[0].Proxy.URL())
REDACTED

func TestListDueOllamaCloudUsageAccountsParsesRFC3339NanoAndFailsOpen(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	repo := newAccountRepositoryWithSQL(tx.Client(), tx, nil)
	now := time.Date(2026, time.July, 22, 14, 0, 0, 0, time.UTC)

	create := func(name, nextRefreshAt string) *service.Account {
	REDACTED
		return mustCreateAccount(t, tx.Client(), &service.Account{
			Name: name, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey,
	REDACTED"api_key": name, "base_url": "https://ollama.com"REDACTED,
			Extra: map[string]any{
				service.OllamaCloudUsageSessionExtraKey:     "cipher:wos-session=fixture",
				service.OllamaCloudUsageAutoRefreshExtraKey: true,
				service.OllamaCloudUsageSnapshotExtraKey: map[string]any{
					"status": service.OllamaCloudUsageStatusOK, "next_refresh_at": nextRefreshAt,
			REDACTED,
		REDACTED,
	REDACTED)
REDACTED

	sevenDigitsOffset := create("ollama-nano-seven", "2026-07-22T11:00:00.1234567-02:00")
	eightDigitsOffset := create("ollama-nano-eight", "2026-07-22T11:00:00.12345678+01:00")
	nineDigitsZ := create("ollama-nano-nine", "2026-07-22T09:00:00.123456789Z")
	invalidCalendar := create("ollama-nano-invalid", "2026-02-30T09:00:00.123456789Z")
	future := create("ollama-nano-future", "2026-07-22T15:00:00.123456789Z")

	accounts, err := repo.ListDueOllamaCloudUsageAccounts(ctx, now, 10)

	require.NoError(t, err, "invalid stored values must not abort the query")
	require.Equal(t, []int64{
		invalidCalendar.ID,
		nineDigitsZ.ID,
		eightDigitsOffset.ID,
		sevenDigitsOffset.ID,
REDACTED, accountIDs(accounts))
	require.NotContains(t, accountIDs(accounts), future.ID)
REDACTED

func TestLockAndMergeAccountProbeExtraCoalescesNullableOllamaGroupIdentity(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	account := mustCreateAccount(t, tx.Client(), &service.Account{
		Name: "ordinary-openai-without-base-url", Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey,
REDACTED"api_key": "sk-no-base-url"REDACTED,
		Extra:       map[string]any{service.UpstreamBillingProbeEnabledExtraKey: trueREDACTED,
REDACTED)
	loaded, err := newAccountRepositoryWithSQL(tx.Client(), tx, nil).GetByID(ctx, account.ID)
REDACTED

	merged, err := lockAndMergeAccountProbeExtra(ctx, tx.Client(), loaded, nil)

	require.NoError(t, err, "a NULL Ollama eligibility expression must scan as false")
	require.NotContains(t, merged, service.OllamaCloudUsageSessionExtraKey)
	require.Equal(t, true, merged[service.UpstreamBillingProbeEnabledExtraKey])
REDACTED

func TestOllamaCloudUsageGroupWritesAreAtomicAcrossPlatformsAndURLVariants(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	repo := newAccountRepositoryWithSQL(tx.Client(), tx, nil)
	create := func(name, platform, apiKey, baseURL string) *service.Account {
	REDACTED
		return mustCreateAccount(t, tx.Client(), &service.Account{
			Name: name, Platform: platform, Type: service.AccountTypeAPIKey,
	REDACTED"api_key": apiKey, "base_url": baseURLREDACTED,
			Extra:       map[string]any{REDACTED,
	REDACTED)
REDACTED
	first := create("ollama-group-openai", service.PlatformOpenAI, "shared-key", "https://ollama.com")
	second := create("ollama-group-anthropic", service.PlatformAnthropic, "shared-key", "HTTPS://WWW.OLLAMA.COM:443/v1")
	different := create("ollama-group-different", service.PlatformOpenAI, "different-key", "https://ollama.com")

	require.NoError(t, repo.SaveOllamaCloudUsageSession(ctx, first, "cipher:shared", false))
	for _, id := range []int64{first.ID, second.IDREDACTED {
		account, err := repo.GetByID(ctx, id)
	REDACTED
		require.Equal(t, "cipher:shared", account.Extra[service.OllamaCloudUsageSessionExtraKey])
		require.Equal(t, false, account.Extra[service.OllamaCloudUsageAutoRefreshExtraKey])
REDACTED
	differentLoaded, err := repo.GetByID(ctx, different.ID)
REDACTED
	require.NotContains(t, differentLoaded.Extra, service.OllamaCloudUsageSessionExtraKey)

	secondLoaded, err := repo.GetByID(ctx, second.ID)
REDACTED
	require.NoError(t, repo.SetOllamaCloudUsageAutoRefresh(ctx, secondLoaded, true))
	firstLoaded, err := repo.GetByID(ctx, first.ID)
REDACTED
	secondLoaded, err = repo.GetByID(ctx, second.ID)
REDACTED
	require.Equal(t, true, firstLoaded.Extra[service.OllamaCloudUsageAutoRefreshExtraKey])
	require.Equal(t, true, secondLoaded.Extra[service.OllamaCloudUsageAutoRefreshExtraKey])

	now := time.Now().UTC()
	snapshot := &service.OllamaCloudUsageSnapshot{
		Status: service.OllamaCloudUsageStatusOK, LastAttemptAt: now, NextRefreshAt: now.Add(time.Hour),
REDACTED
	require.NoError(t, repo.UpdateOllamaCloudUsageSnapshot(ctx, firstLoaded, snapshot))
	secondLoaded, err = repo.GetByID(ctx, second.ID)
REDACTED
	require.Equal(t, service.OllamaCloudUsageStatusOK,
		secondLoaded.Extra[service.OllamaCloudUsageSnapshotExtraKey].(map[string]any)["status"])

	staleSecond := secondLoaded
	require.NoError(t, repo.UpdateCredentials(ctx, second.ID, map[string]any{
		"api_key": "rotated-key", "base_url": "https://ollama.com",
REDACTED))
	require.ErrorIs(t, repo.DisableOllamaCloudUsageAutoRefresh(ctx, staleSecond), service.ErrOllamaCloudUsageIdentityChanged)
	firstLoaded, err = repo.GetByID(ctx, first.ID)
REDACTED
	secondLoaded, err = repo.GetByID(ctx, second.ID)
REDACTED
	require.Equal(t, "cipher:shared", firstLoaded.Extra[service.OllamaCloudUsageSessionExtraKey])
	require.Equal(t, true, firstLoaded.Extra[service.OllamaCloudUsageAutoRefreshExtraKey])
	require.NotContains(t, secondLoaded.Extra, service.OllamaCloudUsageSessionExtraKey)
	require.NotContains(t, secondLoaded.Extra, service.OllamaCloudUsageAutoRefreshExtraKey)

	require.NoError(t, repo.DeleteOllamaCloudUsageSession(ctx, firstLoaded))
	firstLoaded, err = repo.GetByID(ctx, first.ID)
REDACTED
	require.NotContains(t, firstLoaded.Extra, service.OllamaCloudUsageSessionExtraKey)
REDACTED

func TestConcurrentOllamaCloudUsageSaveAndDeleteSerializeGroupState(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client := testEntClient(t)
	repo := newAccountRepositoryWithSQL(client, integrationDB, nil)
	suffix := time.Now().UnixNano()
	apiKey := fmt.Sprintf("ollama-concurrent-%d", suffix)
	create := func(platform string) *service.Account {
	REDACTED
		return mustCreateAccount(t, client, &service.Account{
			Name: fmt.Sprintf("%s-%s", apiKey, platform), Platform: platform, Type: service.AccountTypeAPIKey,
	REDACTED"api_key": apiKey, "base_url": "https://ollama.com"REDACTED,
			Extra: map[string]any{
				service.OllamaCloudUsageSessionExtraKey:     "cipher:initial",
				service.OllamaCloudUsageAutoRefreshExtraKey: true,
		REDACTED,
	REDACTED)
REDACTED
	first := create(service.PlatformOpenAI)
	second := create(service.PlatformAnthropic)
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(context.Background(), "DELETE FROM accounts WHERE id IN ($1, $2)", first.ID, second.ID)
REDACTED)
	anchor, err := repo.GetByID(ctx, first.ID)
REDACTED

	start := make(chan struct{REDACTED)
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		errs <- repo.SaveOllamaCloudUsageSession(ctx, anchor, "cipher:replacement", true)
REDACTED()
	go func() {
		defer wg.Done()
		<-start
		errs <- repo.DeleteOllamaCloudUsageSession(ctx, anchor)
REDACTED()
	close(start)
	wg.Wait()
	close(errs)
	for writeErr := range errs {
		require.NoError(t, writeErr)
REDACTED

	firstLoaded, err := repo.GetByID(ctx, first.ID)
REDACTED
	secondLoaded, err := repo.GetByID(ctx, second.ID)
REDACTED
	managedState := func(account *service.Account) map[string]any {
		state := make(map[string]any)
		for _, key := range []string{
			service.OllamaCloudUsageSessionExtraKey,
			service.OllamaCloudUsageAutoRefreshExtraKey,
			service.OllamaCloudUsageSnapshotExtraKey,
	REDACTED {
			if value, ok := account.Extra[key]; ok {
				state[key] = value
		REDACTED
	REDACTED
		return state
REDACTED
	firstState := managedState(firstLoaded)
	require.Equal(t, firstState, managedState(secondLoaded), "a serialized last commit must own the whole group")
	if len(firstState) > 0 {
		require.Equal(t, "cipher:replacement", firstState[service.OllamaCloudUsageSessionExtraKey])
		require.Equal(t, true, firstState[service.OllamaCloudUsageAutoRefreshExtraKey])
		require.NotContains(t, firstState, service.OllamaCloudUsageSnapshotExtraKey)
REDACTED
REDACTED

func accountIDs(accounts []service.Account) []int64 {
	ids := make([]int64, len(accounts))
	for index := range accounts {
		ids[index] = accounts[index].ID
REDACTED
	return ids
REDACTED

func TestOllamaCloudUsageCredentialAndBulkUpdatesPreserveManagedStateOnlyWhenSafe(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	repo := newAccountRepositoryWithSQL(tx.Client(), tx, nil)
	now := time.Now().UTC()
	newAccount := func(name string) *service.Account {
		return mustCreateAccount(t, tx.Client(), &service.Account{
			Name: name, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey,
	REDACTED"api_key": "old-key", "base_url": "https://ollama.com"REDACTED,
			Extra: map[string]any{
				service.OllamaCloudUsageSessionExtraKey:     "cipher:wos-session=fixture",
				service.OllamaCloudUsageAutoRefreshExtraKey: true,
				service.OllamaCloudUsageSnapshotExtraKey: map[string]any{
					"status": service.OllamaCloudUsageStatusOK, "last_attempt_at": now, "next_refresh_at": now.Add(time.Hour),
			REDACTED,
		REDACTED,
	REDACTED)
REDACTED

	rawAccount := newAccount("ollama-raw-credentials")
	require.NoError(t, repo.UpdateCredentials(ctx, rawAccount.ID, map[string]any{
		"api_key": "old-key", "base_url": "https://ollama.com/V1",
REDACTED))
	rawUpdated, err := repo.GetByID(ctx, rawAccount.ID)
REDACTED
	require.NotContains(t, rawUpdated.Extra, service.OllamaCloudUsageSessionExtraKey)
	require.NotContains(t, rawUpdated.Extra, service.OllamaCloudUsageAutoRefreshExtraKey)
	require.NotContains(t, rawUpdated.Extra, service.OllamaCloudUsageSnapshotExtraKey)

	bulkAccount := newAccount("ollama-bulk-credentials")
	rows, err := repo.BulkUpdate(ctx, []int64{bulkAccount.IDREDACTED, service.AccountBulkUpdate{
REDACTED"base_url": "HTTPS://WWW.OLLAMA.COM:443/v1"REDACTED,
REDACTED)
REDACTED
	require.Equal(t, int64(1), rows)
	bulkUnchanged, err := repo.GetByID(ctx, bulkAccount.ID)
REDACTED
	require.Contains(t, bulkUnchanged.Extra, service.OllamaCloudUsageSnapshotExtraKey)

	rows, err = repo.BulkUpdate(ctx, []int64{bulkAccount.IDREDACTED, service.AccountBulkUpdate{
REDACTED"base_url": "https://ollama.com/V1"REDACTED,
REDACTED)
REDACTED
	require.Equal(t, int64(1), rows)
	bulkIneligible, err := repo.GetByID(ctx, bulkAccount.ID)
REDACTED
	require.NotContains(t, bulkIneligible.Extra, service.OllamaCloudUsageSessionExtraKey)
	require.NotContains(t, bulkIneligible.Extra, service.OllamaCloudUsageAutoRefreshExtraKey)
	require.NotContains(t, bulkIneligible.Extra, service.OllamaCloudUsageSnapshotExtraKey)
REDACTED

func TestProxyIdentityUpdateInvalidatesOllamaSnapshotAndRejectsInFlightCAS(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	accountRepo := newAccountRepositoryWithSQL(tx.Client(), tx, nil)
	proxyRepo := newProxyRepositoryWithSQL(tx.Client(), tx)
	proxy := mustCreateProxy(t, tx.Client(), &service.Proxy{
		Name: "ollama-identity-proxy", Protocol: "http", Host: "old.example", Port: 8080,
		Username: "old-user", Password: "old-pass", Status: service.StatusActive,
REDACTED)
	now := time.Now().UTC()
	account := mustCreateAccount(t, tx.Client(), &service.Account{
		Name: "ollama-proxy-account", Platform: service.PlatformAnthropic, Type: service.AccountTypeAPIKey,
REDACTED"api_key": "key", "base_url": "https://ollama.com"REDACTED,
		ProxyID:     &proxy.ID,
		Extra: map[string]any{
			service.OllamaCloudUsageSessionExtraKey:     "cipher:wos-session=fixture",
			service.OllamaCloudUsageAutoRefreshExtraKey: true,
			service.OllamaCloudUsageSnapshotExtraKey: map[string]any{
				"status": service.OllamaCloudUsageStatusOK, "last_attempt_at": now, "next_refresh_at": now.Add(time.Hour),
		REDACTED,
	REDACTED,
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
	require.NotContains(t, got.Extra, service.OllamaCloudUsageSnapshotExtraKey)
	require.Equal(t, "cipher:wos-session=fixture", got.Extra[service.OllamaCloudUsageSessionExtraKey])
	require.Equal(t, true, got.Extra[service.OllamaCloudUsageAutoRefreshExtraKey])

	err = accountRepo.UpdateOllamaCloudUsageSnapshot(ctx, inFlight, &service.OllamaCloudUsageSnapshot{
		Status: service.OllamaCloudUsageStatusOK, LastAttemptAt: now, NextRefreshAt: now.Add(time.Hour),
REDACTED)
	require.ErrorIs(t, err, service.ErrOllamaCloudUsageIdentityChanged)
REDACTED
