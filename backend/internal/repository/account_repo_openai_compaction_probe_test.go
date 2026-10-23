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

func newOpenAICompactionProbeRepoTest(t *testing.T) (*accountRepository, sqlmock.Sqlmock, *dbent.Client, *service.Account, service.OpenAICompactionProbeTarget) {
	t.Helper()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	t.Cleanup(func() { _ = client.Close() })
	repo := newAccountRepositoryWithSQL(client, db, nil)
	account := &service.Account{
		ID: 301, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "sk-old", "base_url": "https://one.example.test"},
		Extra:       map[string]any{"keep": true},
	}
	target := service.OpenAICompactionProbeTarget{
		Kind: service.OpenAICompactionKindV2, RequestedModel: "gpt-5.4", Model: "gpt-5.4",
		Protocol: service.APIProtocolResponses, Endpoint: "https://one.example.test/v1/responses", EndpointHash: "endpoint-one",
		LogicalAccountID: account.ID, CredentialAccountID: account.ID, CredentialSource: "direct", AuthMode: service.AccountTypeAPIKey,
		FingerprintVersion: 1,
	}
	target.CredentialFingerprint = service.OpenAICompactionCredentialFingerprint(account)
	target.ConfigFingerprint = service.OpenAICompactionProbeTargetFingerprint(account, account, target)
	return repo, mock, client, account, target
}

func TestUpdateOpenAICompactionProbeTargetMergesEvidenceAtomically(t *testing.T) {
	repo, mock, _, account, target := newOpenAICompactionProbeRepoTest(t)
	now := time.Date(2026, 10, 18, 10, 0, 0, 0, time.UTC)
	observation := service.OpenAICompactionProbeObservation{Verdict: service.OpenAICompactionProbeVerdictSupported, CheckedAt: now, Status: 200}
	expectedUpdates, err := service.BuildOpenAICompactionProbeExtraUpdates(account.Extra, target, observation)
	require.NoError(t, err)
	expectedPayload, _ := json.Marshal(expectedUpdates)
	credentialsJSON, _ := json.Marshal(account.Credentials)
	extraJSON, _ := json.Marshal(account.Extra)

	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)SELECT platform, type, credentials.*FOR UPDATE`).WithArgs(account.ID).
		WillReturnRows(sqlmock.NewRows([]string{"platform", "type", "credentials", "extra", "proxy_id", "parent_account_id"}).
			AddRow(account.Platform, account.Type, credentialsJSON, extraJSON, nil, nil))
	mock.ExpectExec(`(?s)UPDATE accounts.*extra = COALESCE.*WHERE id = \$2`).
		WithArgs(string(expectedPayload), account.ID).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO scheduler_outbox")).
		WithArgs(service.SchedulerOutboxEventAccountChanged, account.ID, nil, nil, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	require.NoError(t, repo.UpdateOpenAICompactionProbeTarget(context.Background(), account, target, observation))
	require.NoError(t, mock.ExpectationsWereMet())
	require.NotContains(t, string(expectedPayload), "sk-old")
	require.Contains(t, string(expectedPayload), target.ID())
}

func TestUpdateOpenAICompactionProbeTargetRejectsChangedCredentialSource(t *testing.T) {
	repo, mock, _, account, target := newOpenAICompactionProbeRepoTest(t)
	changedCredentials := map[string]any{"api_key": "sk-new", "base_url": "https://one.example.test"}
	credentialsJSON, _ := json.Marshal(changedCredentials)
	extraJSON, _ := json.Marshal(account.Extra)

	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)SELECT platform, type, credentials.*FOR UPDATE`).WithArgs(account.ID).
		WillReturnRows(sqlmock.NewRows([]string{"platform", "type", "credentials", "extra", "proxy_id", "parent_account_id"}).
			AddRow(account.Platform, account.Type, credentialsJSON, extraJSON, nil, nil))
	mock.ExpectRollback()

	err := repo.UpdateOpenAICompactionProbeTarget(context.Background(), account, target, service.OpenAICompactionProbeObservation{
		Verdict: service.OpenAICompactionProbeVerdictSupported, CheckedAt: time.Now(), Status: 200,
	})
	require.ErrorIs(t, err, service.ErrOpenAICompactionProbeIdentityChanged)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUpdateOpenAICompactionProbeTargetMergesTwoTargetsWithoutLoss(t *testing.T) {
	repo, mock, _, account, first := newOpenAICompactionProbeRepoTest(t)
	second := first
	second.Kind = service.OpenAICompactionKindLegacy
	second.Model = "legacy-model"
	second.Endpoint = "https://one.example.test/v1/responses/compact"
	second.EndpointHash = "endpoint-two"
	second.ConfigFingerprint = service.OpenAICompactionProbeTargetFingerprint(account, account, second)
	credentialsJSON, _ := json.Marshal(account.Credentials)

	firstObservation := service.OpenAICompactionProbeObservation{Verdict: service.OpenAICompactionProbeVerdictSupported, CheckedAt: time.Now(), Status: 200}
	firstUpdates, err := service.BuildOpenAICompactionProbeExtraUpdates(account.Extra, first, firstObservation)
	require.NoError(t, err)
	firstPayload, _ := json.Marshal(firstUpdates)
	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)SELECT platform, type, credentials.*FOR UPDATE`).WithArgs(account.ID).
		WillReturnRows(sqlmock.NewRows([]string{"platform", "type", "credentials", "extra", "proxy_id", "parent_account_id"}).
			AddRow(account.Platform, account.Type, credentialsJSON, mustJSON(t, account.Extra), nil, nil))
	mock.ExpectExec(`(?s)UPDATE accounts.*extra = COALESCE.*WHERE id = \$2`).WithArgs(string(firstPayload), account.ID).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO scheduler_outbox")).
		WithArgs(service.SchedulerOutboxEventAccountChanged, account.ID, nil, nil, sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	require.NoError(t, repo.UpdateOpenAICompactionProbeTarget(context.Background(), account, first, firstObservation))

	secondObservation := service.OpenAICompactionProbeObservation{Verdict: service.OpenAICompactionProbeVerdictUnsupported, CheckedAt: time.Now().Add(time.Second), Status: 404}
	secondUpdates, err := service.BuildOpenAICompactionProbeExtraUpdates(firstUpdates, second, secondObservation)
	require.NoError(t, err)
	secondPayload, _ := json.Marshal(secondUpdates)
	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)SELECT platform, type, credentials.*FOR UPDATE`).WithArgs(account.ID).
		WillReturnRows(sqlmock.NewRows([]string{"platform", "type", "credentials", "extra", "proxy_id", "parent_account_id"}).
			AddRow(account.Platform, account.Type, credentialsJSON, mustJSON(t, firstUpdates), nil, nil))
	mock.ExpectExec(`(?s)UPDATE accounts.*extra = COALESCE.*WHERE id = \$2`).WithArgs(string(secondPayload), account.ID).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO scheduler_outbox")).
		WithArgs(service.SchedulerOutboxEventAccountChanged, account.ID, nil, nil, sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	require.NoError(t, repo.UpdateOpenAICompactionProbeTarget(context.Background(), account, second, secondObservation))

	var stored map[string]any
	require.NoError(t, json.Unmarshal(mustJSON(t, secondUpdates), &stored))
	records, ok := stored[service.OpenAICompactionProbeTargetsExtraKey].(map[string]any)
	require.True(t, ok)
	require.Len(t, records, 2)
	require.NoError(t, mock.ExpectationsWereMet())
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	encoded, err := json.Marshal(value)
	require.NoError(t, err)
	return encoded
}
