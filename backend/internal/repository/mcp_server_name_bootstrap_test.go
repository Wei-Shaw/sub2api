package repository

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/ent/setting"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

func newMCPNamespaceTestClient(t *testing.T, path string) *ent.Client {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	db.SetMaxOpenConns(1)
	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)
	_, err = db.Exec("PRAGMA busy_timeout = 5000")
	require.NoError(t, err)
	client := enttest.NewClient(t, enttest.WithOptions(ent.Driver(entsql.OpenDB(dialect.SQLite, db))))
	t.Cleanup(func() { _ = client.Close() })
	return client
}

type mcpTestEntropy func([]byte) (int, error)

func (f mcpTestEntropy) Read(p []byte) (int, error) { return f(p) }

func TestBootstrapMCPServerName_PersistsAcrossDatabaseReopen(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "settings.db")
	firstClient := newMCPNamespaceTestClient(t, path)
	firstConfig := &config.Config{}
	require.NoError(t, ensureBootstrapMCPServerName(ctx, firstClient, firstConfig))
	first := firstConfig.Gateway.AnthropicMCPServerName
	require.Contains(t, mcpServerNameWords, first)
	require.NoError(t, firstClient.Close())

	restartedClient := newMCPNamespaceTestClient(t, path)
	restartedConfig := &config.Config{}
	require.NoError(t, ensureBootstrapMCPServerName(ctx, restartedClient, restartedConfig))
	require.Equal(t, first, restartedConfig.Gateway.AnthropicMCPServerName)
	// Once initialized, randomness is neither needed nor consulted again.
	name, err := loadOrCreateMCPServerName(ctx, restartedClient, mcpTestEntropy(func([]byte) (int, error) {
		t.Fatal("existing deployment must not regenerate its namespace")
		return 0, io.EOF
	}))
	require.NoError(t, err)
	require.Equal(t, first, name)
}

func TestBootstrapMCPServerName_ConcurrentInitializersUseDatabaseWinner(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	path := filepath.Join(t.TempDir(), "concurrent.db")
	client := newMCPNamespaceTestClient(t, path)
	replica := newMCPNamespaceTestClient(t, path)
	const workers = 8
	ready := make(chan struct{}, workers)
	release := make(chan struct{})
	values := make([]string, workers)
	errs := make([]error, workers)
	var wg sync.WaitGroup
	for i := range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			entropy := mcpTestEntropy(func(p []byte) (int, error) {
				ready <- struct{}{}
				select {
				case <-release:
				case <-ctx.Done():
					return 0, ctx.Err()
				}
				for j := range p {
					p[j] = byte(i)
				}
				return len(p), nil
			})
			dbClient := client
			if i%2 == 1 {
				dbClient = replica
			}
			values[i], errs[i] = loadOrCreateMCPServerName(ctx, dbClient, entropy)
		}()
	}
	// Force every initializer to observe a missing row and choose a distinct
	// candidate before any of them inserts, exercising the conflict path.
	for range workers {
		select {
		case <-ready:
		case <-ctx.Done():
			t.Fatal("initializers did not reach the creation barrier")
		}
	}
	close(release)
	wg.Wait()
	for i := range workers {
		require.NoError(t, errs[i])
		require.Equal(t, values[0], values[i])
	}
	stored, err := client.Setting.Query().Where(setting.KeyEQ(anthropicMCPServerNameSettingKey)).Only(ctx)
	require.NoError(t, err)
	require.Equal(t, values[0], stored.Value)
	count, err := client.Setting.Query().Where(setting.KeyEQ(anthropicMCPServerNameSettingKey)).Count(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, count)
}

func TestBootstrapMCPServerName_IndependentDeployments(t *testing.T) {
	ctx := context.Background()
	first := newMCPNamespaceTestClient(t, filepath.Join(t.TempDir(), "first.db"))
	second := newMCPNamespaceTestClient(t, filepath.Join(t.TempDir(), "second.db"))
	a, err := loadOrCreateMCPServerName(ctx, first, bytes.NewReader([]byte{0}))
	require.NoError(t, err)
	b, err := loadOrCreateMCPServerName(ctx, second, bytes.NewReader([]byte{1}))
	require.NoError(t, err)
	require.NotEqual(t, a, b)
}

func TestBootstrapMCPServerName_InvalidStoredValueNeverRotates(t *testing.T) {
	for _, value := range []string{"", "workspace", "with spaces", "non_ascii_工具", strings.Repeat("x", 33)} {
		t.Run(value, func(t *testing.T) {
			ctx := context.Background()
			client := newMCPNamespaceTestClient(t, filepath.Join(t.TempDir(), "invalid.db"))
			_, err := client.Setting.Create().SetKey(anthropicMCPServerNameSettingKey).SetValue(value).Save(ctx)
			require.NoError(t, err)
			cfg := &config.Config{}
			require.ErrorContains(t, ensureBootstrapMCPServerName(ctx, client, cfg), "invalid persisted")
			require.Empty(t, cfg.Gateway.AnthropicMCPServerName)
			stored, err := client.Setting.Query().Where(setting.KeyEQ(anthropicMCPServerNameSettingKey)).Only(ctx)
			require.NoError(t, err)
			require.Equal(t, value, stored.Value)
		})
	}
}

func TestBootstrapMCPServerName_ErrorsDoNotPublishFallback(t *testing.T) {
	ctx := context.Background()
	client := newMCPNamespaceTestClient(t, filepath.Join(t.TempDir(), "errors.db"))
	cfg := &config.Config{}
	require.Error(t, ensureBootstrapMCPServerName(ctx, nil, cfg))
	require.Error(t, ensureBootstrapMCPServerName(ctx, client, nil))
	_, err := loadOrCreateMCPServerName(ctx, client, mcpTestEntropy(func([]byte) (int, error) {
		return 0, errors.New("entropy unavailable")
	}))
	require.ErrorContains(t, err, "entropy unavailable")
	count, err := client.Setting.Query().Count(ctx)
	require.NoError(t, err)
	require.Zero(t, count)
	require.NoError(t, client.Close())
	require.Error(t, ensureBootstrapMCPServerName(ctx, client, cfg))
	require.Empty(t, cfg.Gateway.AnthropicMCPServerName)
}

func TestBootstrapMCPServerName_WriteErrorDoesNotPublishFallback(t *testing.T) {
	ctx := context.Background()
	client := newMCPNamespaceTestClient(t, filepath.Join(t.TempDir(), "write-error.db"))
	client.Setting.Use(func(ent.Mutator) ent.Mutator {
		return ent.MutateFunc(func(context.Context, ent.Mutation) (ent.Value, error) {
			return nil, errors.New("storage is read-only")
		})
	})
	cfg := &config.Config{}
	require.ErrorContains(t, ensureBootstrapMCPServerName(ctx, client, cfg), "storage is read-only")
	require.Empty(t, cfg.Gateway.AnthropicMCPServerName)
	count, err := client.Setting.Query().Count(ctx)
	require.NoError(t, err)
	require.Zero(t, count)
}

func TestBootstrapMCPServerName_WordPool(t *testing.T) {
	seen := make(map[string]bool)
	for _, word := range mcpServerNameWords {
		require.True(t, validPersistedMCPServerName(word), word)
		require.False(t, seen[word], word)
		seen[word] = true
	}
	// Validation is independent of pool membership: future pool edits must not
	// force an existing deployment to adopt a different name.
	require.True(t, validPersistedMCPServerName("extension"))
}
