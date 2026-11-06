package repository

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/internal/pkg/reqcompress"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// TestListOpsAccountsForStatsKeepsNegotiatedRequestCompression pins a field
// regression: the ops dashboard reads a column projection that loads neither
// compress_request_enabled nor updated_at. Mirroring that row into the
// compression registry looked like an unordered "disabled" observation, so every
// ops refresh dropped the negotiated coding and deleted the shared Redis value.
// The account list then lost its algorithm badge, and the next request probed br
// again.
func TestListOpsAccountsForStatsKeepsNegotiatedRequestCompression(t *testing.T) {
	ctx := context.Background()
	repo, client := newAccountEntRepo(t)

	created, err := client.Account.Create().
		SetName("compress-request-ops-projection").
		SetPlatform(service.PlatformOpenAI).
		SetType(service.AccountTypeAPIKey).
		SetStatus(service.StatusActive).
		SetCredentials(map[string]any{"api_key": "sk-test"}).
		SetCompressRequestEnabled(true).
		Save(ctx)
	require.NoError(t, err)
	accountID := created.ID

	server := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: server.Addr()})
	// The registry is process-wide: start from a clean slot and leave none behind.
	reqcompress.ResetAccount(ctx, accountID)
	reqcompress.ConfigureRedis(rdb)
	t.Cleanup(func() {
		reqcompress.ResetAccount(context.Background(), accountID)
		reqcompress.ConfigureRedis(nil)
		_ = rdb.Close()
	})

	// A full row read opts the account in, and one request settles zstd.
	_, err = repo.GetByID(ctx, accountID)
	require.NoError(t, err)
	require.True(t, reqcompress.Enabled(accountID))
	require.Equal(t, []string{"br", "zstd"}, sendCompressibleRequest(t, accountID, func(encoding string) int {
		if encoding == "br" {
			return http.StatusUnsupportedMediaType
		}
		return http.StatusOK
	}))
	require.Equal(t, reqcompress.AlgorithmZstd, reqcompress.SettledAlgorithm(ctx, accountID))

	accounts, err := repo.ListOpsAccountsForStats(ctx, "", nil)
	require.NoError(t, err)
	require.Len(t, accounts, 1)
	require.True(t, reqcompress.Enabled(accountID), "a projected read must not disable compression")
	require.Equal(t, reqcompress.AlgorithmZstd, reqcompress.SettledAlgorithm(ctx, accountID),
		"a projected read must not clear the shared coding")

	// The next account-list read still reports zstd, and traffic keeps using it
	// without probing br again.
	_, err = repo.GetByID(ctx, accountID)
	require.NoError(t, err)
	require.Equal(t, reqcompress.AlgorithmZstd, reqcompress.SettledAlgorithm(ctx, accountID))
	require.Equal(t, []string{"zstd"}, sendCompressibleRequest(t, accountID, func(string) int {
		return http.StatusOK
	}))

	// A full row still reports a real switch change.
	require.NoError(t, client.Account.UpdateOneID(accountID).SetCompressRequestEnabled(false).Exec(ctx))
	_, err = repo.GetByID(ctx, accountID)
	require.NoError(t, err)
	require.False(t, reqcompress.Enabled(accountID))
	require.Equal(t, reqcompress.AlgorithmNone, reqcompress.SettledAlgorithm(ctx, accountID))
}

func newAccountEntRepo(t *testing.T) (*accountRepository, *dbent.Client) {
	t.Helper()

	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s?mode=memory&cache=shared&_fk=1", t.Name()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(drv)))
	t.Cleanup(func() { _ = client.Close() })

	return newAccountRepositoryWithSQL(client, db, nil), client
}

// sendCompressibleRequest forwards one eligible request through the default
// compression registry and returns the Content-Encoding of every attempt.
func sendCompressibleRequest(t *testing.T, accountID int64, status func(encoding string) int) []string {
	t.Helper()

	body := bytes.Repeat([]byte(`{"input":"compressible request body"}`), 128)
	req, err := http.NewRequest(http.MethodPost, "https://upstream.example/v1/responses", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	var encodings []string
	resp, err := reqcompress.Execute(accountID, req, func(attempt *http.Request) (*http.Response, error) {
		encoding := attempt.Header.Get("Content-Encoding")
		encodings = append(encodings, encoding)
		return &http.Response{
			StatusCode: status(encoding),
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("ok")),
			Request:    attempt,
		}, nil
	})
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	return encodings
}
