package repository

import (
	"context"
	"encoding/json"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
)

func newOllamaCloudUsageRepositoryTestClient(t *testing.T) (*dbent.Client, sqlmock.Sqlmock) {
REDACTED
	db, mock, err := sqlmock.New()
REDACTED
	t.Cleanup(func() { _ = db.Close() REDACTED)
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	t.Cleanup(func() { _ = client.Close() REDACTED)
	return client, mock
REDACTED

func ollamaCloudUsageRepositoryAccount() *service.Account {
	return &service.Account{
		ID: 17, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey,
REDACTED"api_key": "key", "base_url": "https://ollama.com"REDACTED,
		Extra: map[string]any{
			service.OllamaCloudUsageSessionExtraKey:     "cipher:wos-session=secret",
			service.OllamaCloudUsageAutoRefreshExtraKey: true,
	REDACTED,
REDACTED
REDACTED

func TestUpdateOllamaCloudUsageSnapshotRowsAffectedZeroIsIdentityConflict(t *testing.T) {
	client, mock := newOllamaCloudUsageRepositoryTestClient(t)
	mock.ExpectBegin()
	expectOllamaCloudUsageGroupLock(mock, ollamaCloudUsageRepositoryAccount(), true,
		`"cipher:wos-session=secret"`, `true`, `null`)
	mock.ExpectExec(`(?s)`+regexp.QuoteMeta("UPDATE accounts")).
		WithArgs(sqlmock.AnyArg(), "key", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectRollback()
	repo := newAccountRepositoryWithSQL(client, nil, nil)

	err := repo.UpdateOllamaCloudUsageSnapshot(context.Background(), ollamaCloudUsageRepositoryAccount(), &service.OllamaCloudUsageSnapshot{
		Status:        service.OllamaCloudUsageStatusOK,
		LastAttemptAt: time.Now(),
		NextRefreshAt: time.Now().Add(time.Hour),
REDACTED)

	require.ErrorIs(t, err, service.ErrOllamaCloudUsageIdentityChanged)
	require.NoError(t, mock.ExpectationsWereMet())
REDACTED

func expectOllamaCloudUsageGroupLock(
	mock sqlmock.Sqlmock,
	account *service.Account,
	anchorMatches bool,
	sessionJSON, autoJSON, snapshotJSON string,
) {
	apiKey, _ := account.Credentials["api_key"].(string)
	credentials, _ := json.Marshal(normalizeJSONMap(account.Credentials))
	var proxyID any
	if account.ProxyID != nil {
		proxyID = *account.ProxyID
REDACTED
	mock.ExpectQuery(`(?s)`+regexp.QuoteMeta("SELECT")+`.*`+regexp.QuoteMeta("FOR NO KEY UPDATE")).
		WithArgs(apiKey, account.ID, account.Platform, account.Type, string(credentials), proxyID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "anchor_matches", "session", "auto_refresh", "snapshot"REDACTED).
			AddRow(account.ID, anchorMatches, sessionJSON, autoJSON, snapshotJSON))
REDACTED

func TestOllamaCloudUsageManagedWriteRejectsChangedProxyIdentity(t *testing.T) {
	client, mock := newOllamaCloudUsageRepositoryTestClient(t)
	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)` + regexp.QuoteMeta("SELECT protocol, host, port") + `.*` + regexp.QuoteMeta("FOR SHARE")).
		WithArgs(int64(9)).
		WillReturnRows(sqlmock.NewRows([]string{"protocol", "host", "port", "username", "password", "status"REDACTED).
			AddRow("http", "new.example", 3128, "user", "pass", service.StatusActive))
	mock.ExpectRollback()

	account := ollamaCloudUsageRepositoryAccount()
	proxyID := int64(9)
	account.ProxyID = &proxyID
	account.Proxy = &service.Proxy{
		ID: proxyID, Protocol: "http", Host: "old.example", Port: 3128,
		Username: "user", Password: "pass", Status: service.StatusActive,
REDACTED
	repo := newAccountRepositoryWithSQL(client, nil, nil)

	err := repo.SaveOllamaCloudUsageSession(context.Background(), account, "cipher:wos-session=replacement", true)

	require.ErrorIs(t, err, service.ErrOllamaCloudUsageIdentityChanged)
	require.NoError(t, mock.ExpectationsWereMet())
REDACTED

func TestSaveAndDeleteOllamaCloudUsageSessionKeepCiphertextOutOfSQL(t *testing.T) {
	var capturedSQL []string
	matcher := sqlmock.QueryMatcherFunc(func(expectedSQL, actualSQL string) error {
		capturedSQL = append(capturedSQL, actualSQL)
		return sqlmock.QueryMatcherRegexp.Match(expectedSQL, actualSQL)
REDACTED)
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(matcher))
REDACTED
	t.Cleanup(func() { _ = db.Close() REDACTED)
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	t.Cleanup(func() { _ = client.Close() REDACTED)
	repo := newAccountRepositoryWithSQL(client, db, nil)
	account := ollamaCloudUsageRepositoryAccount()
	const replacement = "cipher:wos-session=browser-cookie-secret"

	mock.ExpectBegin()
	expectOllamaCloudUsageGroupLock(mock, account, true, `"cipher:wos-session=secret"`, `true`, `null`)
	mock.ExpectExec(`(?s)UPDATE accounts.*ollama_cloud_usage_session.*ollama_cloud_usage_auto_refresh.*ollama_cloud_usage_snapshot`).
		WithArgs(`{"ollama_cloud_usage_auto_refresh":true,"ollama_cloud_usage_session":"cipher:wos-session=browser-cookie-secret"REDACTED`, "key", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	require.NoError(t, repo.SaveOllamaCloudUsageSession(context.Background(), account, replacement, true))

	account.Extra[service.OllamaCloudUsageSessionExtraKey] = replacement
	mock.ExpectBegin()
	expectOllamaCloudUsageGroupLock(mock, account, true, `"cipher:wos-session=browser-cookie-secret"`, `true`, `null`)
	mock.ExpectExec(`(?s)UPDATE accounts.*ollama_cloud_usage_session.*ollama_cloud_usage_auto_refresh.*ollama_cloud_usage_snapshot`).
		WithArgs(`{REDACTED`, "key", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	require.NoError(t, repo.DeleteOllamaCloudUsageSession(context.Background(), account))

	require.NotEmpty(t, capturedSQL)
	for _, query := range capturedSQL {
		require.NotContains(t, query, "browser-cookie-secret")
REDACTED
	require.NoError(t, mock.ExpectationsWereMet())
REDACTED

func TestOllamaCloudBaseURLSQLRegexMatchesServiceSemantics(t *testing.T) {
	for _, baseURL := range []string{
		"https://ollama.com",
		"HTTPS://WWW.OLLAMA.COM:443/v1",
		"https://ollama.com/V1",
		"https://ollama.com/v1/",
		"https://ollama.com.evil.test/v1",
REDACTED {
		t.Run(baseURL, func(t *testing.T) {
			matched, err := regexp.MatchString(ollamaCloudBaseURLRegexSQL, baseURL)
		REDACTED
			account := ollamaCloudUsageRepositoryAccount()
			account.Credentials["base_url"] = baseURL
			require.Equal(t, service.IsOllamaCloudUsageAccount(account), matched)
	REDACTED)
REDACTED
REDACTED

func TestListOllamaCloudUsageGroupAccountsUsesOneStrictBatchQuery(t *testing.T) {
	db, mock, err := sqlmock.New()
REDACTED
	t.Cleanup(func() { _ = db.Close() REDACTED)
	var capturedSQL string
	mock.ExpectQuery("SELECT id").
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id"REDACTED))
	repo := newAccountRepositoryWithSQL(nil, captureQuerySQL{db: db, captured: &capturedSQLREDACTED, nil)
	first := ollamaCloudUsageRepositoryAccount()
	second := ollamaCloudUsageRepositoryAccount()
	second.ID = 18
	second.Platform = service.PlatformAnthropic
	second.Credentials = map[string]any{"api_key": "key", "base_url": "https://www.ollama.com:443/v1"REDACTED

	accounts, err := repo.ListOllamaCloudUsageGroupAccounts(context.Background(), []*service.Account{first, secondREDACTED)

REDACTED
	require.Empty(t, accounts)
	query := normalizeSQLWhitespace(capturedSQL)
	require.Contains(t, query, "credentials ->> 'api_key' = ANY($1)")
	require.Contains(t, query, "platform IN ('openai', 'anthropic')")
	require.Contains(t, query, "jsonb_typeof(credentials -> 'api_key') = 'string'")
	require.Contains(t, query, ollamaCloudBaseURLMatchesSQL("credentials ->> 'base_url'"))
	require.NotContains(t, query, "~*")
	require.NoError(t, mock.ExpectationsWereMet())
REDACTED

func TestListDueOllamaCloudUsageAccountsFiltersOrdersAndLimits(t *testing.T) {
	db, mock, err := sqlmock.New()
REDACTED
	t.Cleanup(func() { _ = db.Close() REDACTED)
	now := time.Date(2026, time.July, 22, 12, 0, 0, 0, time.UTC)
	var capturedSQL string
	mock.ExpectQuery("WITH eligible AS").
		WithArgs(20).
		WillReturnRows(sqlmock.NewRows([]string{"id", "group_last_used_at"REDACTED))
	repo := newAccountRepositoryWithSQL(nil, captureQuerySQL{db: db, captured: &capturedSQLREDACTED, nil)

	accounts, err := repo.ListDueOllamaCloudUsageAccounts(context.Background(), now, 20)

REDACTED
	require.Empty(t, accounts)
	normalized := normalizeSQLWhitespace(capturedSQL)
	for _, clause := range []string{
		"deleted_at IS NULL",
		"status = 'active'",
		"platform IN ('openai', 'anthropic')",
		"type = 'apikey'",
		ollamaCloudBaseURLMatchesSQL("credentials ->> 'base_url'"),
		"jsonb_typeof(extra -> 'ollama_cloud_usage_session') = 'string'",
		`extra @> '{"ollama_cloud_usage_auto_refresh": trueREDACTED'::jsonb`,
		"MAX(last_used_at) AS group_last_used_at",
		"PARTITION BY api_key",
		"WHERE group_rank = 1",
		"LIMIT $1",
		"group_last_used_at > parsed_fetched_at::timestamptz",
		"group_last_used_at > parsed_last_attempt_at::timestamptz",
REDACTED {
		require.Contains(t, normalized, clause)
REDACTED
	require.NotContains(t, normalized, "~*")
	require.NoError(t, mock.ExpectationsWereMet())
REDACTED

func TestBulkUpdateOllamaIdentityCleanupIsValueConditional(t *testing.T) {
	exec := &recordingSQLExecutor{result: rowsAffectedResult(1)REDACTED
	repo := newAccountRepositoryWithSQL(nil, exec, nil)

	_, err := repo.BulkUpdate(context.Background(), []int64{17REDACTED, service.AccountBulkUpdate{
REDACTED"base_url": "https://www.ollama.com:443/v1"REDACTED,
REDACTED)

REDACTED
	require.NotEmpty(t, exec.execQueries)
	query := normalizeSQLWhitespace(exec.execQueries[0])
	require.Contains(t, query, "NOT ("+ollamaCloudBaseURLMatchesSQL("credentials ->> 'base_url'"))
	require.Contains(t, query, ollamaCloudBaseURLMatchesSQL("$1::jsonb ->> 'base_url'"))
	require.NotContains(t, query, "~*")
	require.Contains(t, query, "platform IN ('openai', 'anthropic') AND type = 'apikey'")
	require.Contains(t, query, "- 'ollama_cloud_usage_session' - 'ollama_cloud_usage_auto_refresh' - 'ollama_cloud_usage_snapshot'")
	payload, ok := exec.execArgs[0][0].([]byte)
	require.True(t, ok)
	require.NotContains(t, string(payload), service.OllamaCloudUsageSnapshotExtraKey)
REDACTED

func TestUpdateCredentialsIdentityChangeClearsAllOllamaManagedExtra(t *testing.T) {
	client, mock := newOllamaCloudUsageRepositoryTestClient(t)
	mock.ExpectBegin()
	mock.ExpectExec(`(?s)UPDATE accounts.*credentials -> 'api_key' IS DISTINCT FROM.*ollama_cloud_usage_session.*ollama_cloud_usage_auto_refresh.*ollama_cloud_usage_snapshot`).
		WithArgs(`{"api_key":"new-key","base_url":"https://ollama.com"REDACTED`, int64(17)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO scheduler_outbox")).
		WithArgs(service.SchedulerOutboxEventAccountChanged, int64(17), nil, nil, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	repo := newAccountRepositoryWithSQL(client, nil, nil)

	err := repo.UpdateCredentials(context.Background(), 17, map[string]any{
		"api_key": "new-key", "base_url": "https://ollama.com",
REDACTED)

REDACTED
	require.NoError(t, mock.ExpectationsWereMet())
REDACTED

func TestDisableOllamaCloudUsageAutoRefreshUsesGroupIdentityCAS(t *testing.T) {
	client, mock := newOllamaCloudUsageRepositoryTestClient(t)
	account := ollamaCloudUsageRepositoryAccount()
	mock.ExpectBegin()
	expectOllamaCloudUsageGroupLock(mock, account, true, `"cipher:wos-session=secret"`, `true`, `null`)
	mock.ExpectExec(`(?s)UPDATE accounts.*ollama_cloud_usage_auto_refresh`).
		WithArgs(`{"ollama_cloud_usage_auto_refresh":false,"ollama_cloud_usage_session":"cipher:wos-session=secret"REDACTED`, "key", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	repo := newAccountRepositoryWithSQL(client, nil, nil)

	err := repo.DisableOllamaCloudUsageAutoRefresh(context.Background(), account)

REDACTED
	require.NoError(t, mock.ExpectationsWereMet())
REDACTED

// Ollama 清理分支必须带顶层 credentials DISTINCT 守卫：没有它，非 Ollama 的
// openai/anthropic apikey 账号在凭证未变化的持久化上也会误清探测快照。
func TestUpdateCredentialsCleanupBranchRequiresChangedCredentials(t *testing.T) {
	client, mock := newOllamaCloudUsageRepositoryTestClient(t)
	mock.ExpectBegin()
	mock.ExpectExec(`(?s)UPDATE accounts.*CASE.*AND credentials IS DISTINCT FROM \$1::jsonb\s+AND \(\s+credentials -> 'api_key' IS DISTINCT FROM`).
		WithArgs(`{"api_key":"same-key","base_url":"https://relay.example.com/v1"REDACTED`, int64(17)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO scheduler_outbox")).
		WithArgs(service.SchedulerOutboxEventAccountChanged, int64(17), nil, nil, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	repo := newAccountRepositoryWithSQL(client, nil, nil)

	err := repo.UpdateCredentials(context.Background(), 17, map[string]any{
		"api_key": "same-key", "base_url": "https://relay.example.com/v1",
REDACTED)

REDACTED
	require.NoError(t, mock.ExpectationsWereMet())
REDACTED
