package migrate

import (
	"testing"

	"entgo.io/ent/dialect/entsql"
	entschema "entgo.io/ent/dialect/sql/schema"
	"github.com/stretchr/testify/require"
)

func TestAuthIdentityFoundationForeignKeyOnDeleteActions(t *testing.T) {
	require.Equal(
		t,
		entschema.Cascade,
		findForeignKeyBySymbol(t, AuthIdentitiesTable, "auth_identities_users_auth_identities").OnDelete,
	)
	require.Equal(
		t,
		entschema.Cascade,
		findForeignKeyBySymbol(t, AuthIdentityChannelsTable, "auth_identity_channels_auth_identities_channels").OnDelete,
	)
	require.Equal(
		t,
		entschema.Cascade,
		findForeignKeyBySymbol(t, IdentityAdoptionDecisionsTable, "identity_adoption_decisions_pending_auth_sessions_adoption_decision").OnDelete,
	)

	require.Equal(
		t,
		entschema.SetNull,
		findForeignKeyBySymbol(t, PendingAuthSessionsTable, "pending_auth_sessions_users_pending_auth_sessions").OnDelete,
	)
	require.Equal(
		t,
		entschema.SetNull,
		findForeignKeyBySymbol(t, IdentityAdoptionDecisionsTable, "identity_adoption_decisions_auth_identities_adoption_decisions").OnDelete,
	)
REDACTED

func TestPaymentOrdersOutTradeNoPartialUniqueIndex(t *testing.T) {
	idx := findIndexByName(t, PaymentOrdersTable, "paymentorder_out_trade_no")
	require.True(t, idx.Unique)
	require.Len(t, idx.Columns, 1)
	require.Equal(t, "out_trade_no", idx.Columns[0].Name)
	require.NotNil(t, idx.Annotation)
	require.Equal(t, (&entsql.IndexAnnotation{Where: "out_trade_no <> ''"REDACTED).Where, idx.Annotation.Where)
REDACTED

func TestAccountsParentAccountForeignKey(t *testing.T) {
	fk := findForeignKeyByColumn(t, AccountsTable, "parent_account_id")
	require.Len(t, fk.Columns, 1)
	require.Equal(t, "parent_account_id", fk.Columns[0].Name)
	require.False(t, fk.Columns[0].Unique, "active-shadow uniqueness is enforced by the partial uq_accounts_spark_shadow_per_parent index")
	require.Len(t, fk.RefColumns, 1)
	require.Equal(t, "id", fk.RefColumns[0].Name)
	require.Equal(t, entschema.Restrict, fk.OnDelete)
REDACTED

func findForeignKeyBySymbol(t *testing.T, table *entschema.Table, symbol string) *entschema.ForeignKey {
REDACTED

	for _, fk := range table.ForeignKeys {
		if fk.Symbol == symbol {
			return fk
	REDACTED
REDACTED

	require.Failf(t, "missing foreign key", "table %s should include foreign key %s", table.Name, symbol)
	return nil
REDACTED

func findIndexByName(t *testing.T, table *entschema.Table, name string) *entschema.Index {
REDACTED

	for _, idx := range table.Indexes {
		if idx.Name == name {
			return idx
	REDACTED
REDACTED

	require.Failf(t, "missing index", "table %s should include index %s", table.Name, name)
	return nil
REDACTED

func findForeignKeyByColumn(t *testing.T, table *entschema.Table, column string) *entschema.ForeignKey {
REDACTED

	for _, fk := range table.ForeignKeys {
		for _, col := range fk.Columns {
			if col.Name == column {
				return fk
		REDACTED
	REDACTED
REDACTED

	require.Failf(t, "missing foreign key", "table %s should include foreign key for column %s", table.Name, column)
	return nil
REDACTED
