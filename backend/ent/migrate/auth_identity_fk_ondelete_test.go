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
}

func TestPaymentOrdersOutTradeNoPartialUniqueIndex(t *testing.T) {
	idx := findIndexByName(t, PaymentOrdersTable, "paymentorder_out_trade_no")
	require.True(t, idx.Unique)
	require.Len(t, idx.Columns, 1)
	require.Equal(t, "out_trade_no", idx.Columns[0].Name)
	require.NotNil(t, idx.Annotation)
	require.Equal(t, (&entsql.IndexAnnotation{Where: "out_trade_no <> ''"}).Where, idx.Annotation.Where)
}

func findForeignKeyBySymbol(t *testing.T, table *entschema.Table, symbol string) *entschema.ForeignKey {
	t.Helper()

	for _, fk := range table.ForeignKeys {
		if fk.Symbol == symbol {
			return fk
		}
	}

	require.Failf(t, "missing foreign key", "table %s should include foreign key %s", table.Name, symbol)
	return nil
}

func findIndexByName(t *testing.T, table *entschema.Table, name string) *entschema.Index {
	t.Helper()

	for _, idx := range table.Indexes {
		if idx.Name == name {
			return idx
		}
	}

	require.Failf(t, "missing index", "table %s should include index %s", table.Name, name)
	return nil
}
