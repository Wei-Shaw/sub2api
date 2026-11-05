//go:build integration

package repository

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/traffic"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// TestUsageTrafficPostgresRoundTrip exercises actual JSONB casts in both CTE
// batch paths, the direct INSERT, SQL scanning and generated Ent mapping.
func TestUsageTrafficPostgresRoundTrip(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	repo := newUsageLogRepositoryWithSQL(client, integrationDB)
	user := mustCreateUser(t, client, &service.User{Email: "traffic-" + uuid.NewString() + "@example.com"})
	key := mustCreateApiKey(t, client, &service.APIKey{UserID: user.ID, Key: "sk-traffic-" + uuid.NewString(), Name: "traffic"})
	account := mustCreateAccount(t, client, &service.Account{Name: "traffic-" + uuid.NewString()})

	var nullable, kind string
	var defaultValue sql.NullString
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT is_nullable, data_type, column_default FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'usage_logs' AND column_name = 'traffic'`).Scan(&nullable, &kind, &defaultValue))
	require.Equal(t, "YES", nullable)
	require.Equal(t, "jsonb", kind)
	require.False(t, defaultValue.Valid)

	for _, path := range []string{"single", "batch", "best-effort", "ent"} {
		for _, observation := range []struct {
			name  string
			value *traffic.Usage
		}{
			{"observed", trafficTestUsage()},
			{"unknown", nil},
			{"consumed-response-prefix", trafficConsumedResponseTestUsage(t)},
		} {
			t.Run(path+"/"+observation.name, func(t *testing.T) {
				value := observation.value
				log := &service.UsageLog{UserID: user.ID, APIKeyID: key.ID, AccountID: account.ID, RequestID: uuid.NewString(), Model: "gpt-5", CreatedAt: time.Now().UTC(), Traffic: value}
				switch path {
				case "single":
					inserted, err := repo.createSingle(ctx, integrationDB, log)
					require.NoError(t, err)
					require.True(t, inserted)
				case "batch":
					inserted, err := repo.Create(ctx, log)
					require.NoError(t, err)
					require.True(t, inserted)
				case "best-effort":
					require.NoError(t, repo.CreateBestEffort(ctx, log))
					require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT id FROM usage_logs WHERE request_id=$1 AND api_key_id=$2", log.RequestID, key.ID).Scan(&log.ID))
				case "ent":
					builder := client.UsageLog.Create().SetUserID(user.ID).SetAPIKeyID(key.ID).SetAccountID(account.ID).SetRequestID(log.RequestID).SetModel(log.Model).SetCreatedAt(log.CreatedAt)
					if value != nil {
						builder.SetTraffic(value)
					}
					entity, err := builder.Save(ctx)
					require.NoError(t, err)
					log.ID = entity.ID
				}
				got, err := repo.GetByID(ctx, log.ID)
				require.NoError(t, err)
				require.Equal(t, value, got.Traffic)
				entity, err := client.UsageLog.Get(ctx, log.ID)
				require.NoError(t, err)
				require.Equal(t, value, entity.Traffic)
				var isNull bool
				require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT traffic IS NULL FROM usage_logs WHERE id=$1", log.ID).Scan(&isNull))
				require.Equal(t, value == nil, isNull, "unknown must be SQL NULL, not empty JSON or synthetic zero")
			})
		}
	}
}
