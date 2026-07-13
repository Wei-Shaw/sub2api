package repository

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"reflect"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

type accountIDsPayloadMatcher struct {
	want []int64
REDACTED

func (m accountIDsPayloadMatcher) Match(value driver.Value) bool {
	raw, ok := value.([]byte)
	if !ok {
		return false
REDACTED
	var payload struct {
		AccountIDs []int64 `json:"account_ids"`
REDACTED
	if err := json.Unmarshal(raw, &payload); err != nil {
		return false
REDACTED
	return reflect.DeepEqual(m.want, payload.AccountIDs)
REDACTED

func TestAutoPauseExpiredAccountsEnqueuesAffectedAccounts(t *testing.T) {
	db, mock, err := sqlmock.New()
REDACTED
	t.Cleanup(func() { _ = db.Close() REDACTED)

	now := time.Now()
	mock.ExpectQuery(`(?s)UPDATE accounts.*RETURNING id`).
		WithArgs(now).
		WillReturnRows(sqlmock.NewRows([]string{"id"REDACTED).AddRow(int64(11)).AddRow(int64(29)))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO scheduler_outbox (event_type, account_id, group_id, payload)")).
		WithArgs(service.SchedulerOutboxEventAccountBulkChanged, nil, nil, accountIDsPayloadMatcher{want: []int64{11, 29REDACTEDREDACTED).
		WillReturnResult(sqlmock.NewResult(1, 1))

	repo := newAccountRepositoryWithSQL(nil, db, nil)
	updated, err := repo.AutoPauseExpiredAccounts(context.Background(), now)

REDACTED
	require.EqualValues(t, 2, updated)
	require.NoError(t, mock.ExpectationsWereMet())
REDACTED

func TestAutoPauseExpiredAccountsSkipsOutboxWithoutChanges(t *testing.T) {
	db, mock, err := sqlmock.New()
REDACTED
	t.Cleanup(func() { _ = db.Close() REDACTED)

	now := time.Now()
	mock.ExpectQuery(`(?s)UPDATE accounts.*RETURNING id`).
		WithArgs(now).
		WillReturnRows(sqlmock.NewRows([]string{"id"REDACTED))

	repo := newAccountRepositoryWithSQL(nil, db, nil)
	updated, err := repo.AutoPauseExpiredAccounts(context.Background(), now)

REDACTED
	require.Zero(t, updated)
	require.NoError(t, mock.ExpectationsWereMet())
REDACTED
