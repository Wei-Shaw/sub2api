package migrate

import (
	"testing"

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

// TestPaymentOrdersOutTradeNoPartialUniqueIndex 已删除：payment 表已迁移到
// plugins/payment/，host 不再持有 PaymentOrdersTable。

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
