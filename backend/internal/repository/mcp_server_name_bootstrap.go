package repository

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"math/big"
	"time"

	"github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/setting"
	"github.com/Wei-Shaw/sub2api/internal/config"
)

const anthropicMCPServerNameSettingKey = "anthropic_mcp_server_name"

// These ordinary ASCII words are candidates only for a deployment's first
// initialization. The persisted winner, not this pool or a process-local draw,
// is authoritative on every subsequent startup and across replicas.
var mcpServerNameWords = []string{
	"agent", "assist", "bridge", "builder", "code", "command", "console", "context",
	"data", "debug", "design", "dev", "editor", "engine", "files", "format",
	"gateway", "helper", "index", "input", "kit", "memory", "model", "notes",
	"output", "parser", "patch", "project", "query", "reader", "render", "request",
	"response", "runner", "schema", "script", "search", "service", "shell", "store",
	"stream", "system", "task", "text", "toolbox", "tools", "transform", "utility",
	"view", "worker", "workflow", "writer",
}

func ensureBootstrapMCPServerName(ctx context.Context, client *ent.Client, cfg *config.Config) error {
	if client == nil || cfg == nil {
		return fmt.Errorf("MCP namespace bootstrap requires a database client and config")
	}
	name, err := loadOrCreateMCPServerName(ctx, client, rand.Reader)
	if err != nil {
		return fmt.Errorf("initialize persistent MCP namespace: %w", err)
	}
	// Published before services are constructed; request paths only read it.
	cfg.Gateway.AnthropicMCPServerName = name
	return nil
}

func loadOrCreateMCPServerName(ctx context.Context, client *ent.Client, entropy io.Reader) (string, error) {
	stored, err := client.Setting.Query().Where(setting.KeyEQ(anthropicMCPServerNameSettingKey)).Only(ctx)
	if err != nil {
		if !ent.IsNotFound(err) {
			return "", err
		}
		index, err := rand.Int(entropy, big.NewInt(int64(len(mcpServerNameWords))))
		if err != nil {
			return "", fmt.Errorf("choose MCP namespace: %w", err)
		}
		candidate := mcpServerNameWords[index.Int64()]
		// Concurrent initializers must not overwrite each other. Never use our
		// candidate until the committed winner has been read from the database.
		err = client.Setting.Create().SetKey(anthropicMCPServerNameSettingKey).
			SetValue(candidate).SetUpdatedAt(time.Now()).
			OnConflictColumns(setting.FieldKey).DoNothing().Exec(ctx)
		if err != nil && !isSQLNoRowsError(err) {
			return "", err
		}
		stored, err = client.Setting.Query().Where(setting.KeyEQ(anthropicMCPServerNameSettingKey)).Only(ctx)
		if err != nil {
			return "", err
		}
	}
	if !validPersistedMCPServerName(stored.Value) {
		// Do not silently rotate an existing deployment's identity or run with
		// an ephemeral fallback when persistence is broken.
		return "", fmt.Errorf("invalid persisted %s; expected a lowercase ASCII word of 1-32 letters other than workspace", anthropicMCPServerNameSettingKey)
	}
	return stored.Value, nil
}

func validPersistedMCPServerName(name string) bool {
	if len(name) == 0 || len(name) > 32 || name == "workspace" {
		return false
	}
	for _, c := range name {
		if c < 'a' || c > 'z' {
			return false
		}
	}
	return true
}
