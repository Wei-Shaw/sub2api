package schema

import (
	"testing"

	"entgo.io/ent/entc/load"
	"github.com/stretchr/testify/require"
)

func TestChatSessionSchemas(t *testing.T) {
	spec, err := (&load.Config{Path: "."}).Load()
	require.NoError(t, err)

	schemas := map[string]*load.Schema{}
	for _, schema := range spec.Schemas {
		schemas[schema.Name] = schema
	}

	session := requireSchema(t, schemas, "ChatSession")
	requireSchemaFields(t, session,
		"user_id",
		"api_key_id",
		"title",
		"model",
		"status",
		"expires_at",
		"deleted_at",
	)
	requireHasIndex(t, session, "user_id", "updated_at")
	requireHasIndex(t, session, "user_id", "expires_at")

	message := requireSchema(t, schemas, "ChatMessage")
	requireSchemaFields(t, message,
		"session_id",
		"user_id",
		"role",
		"content",
		"status",
		"model",
		"duration_ms",
		"usage_log_id",
		"actual_cost",
		"error_message",
	)
	requireHasIndex(t, message, "session_id", "created_at")
	requireHasIndex(t, message, "user_id", "created_at")
	requireHasIndex(t, message, "usage_log_id")
}

func requireHasIndex(t *testing.T, schema *load.Schema, fields ...string) {
	t.Helper()

	for _, index := range schema.Indexes {
		if len(index.Fields) != len(fields) {
			continue
		}
		match := true
		for i := range fields {
			if index.Fields[i] != fields[i] {
				match = false
				break
			}
		}
		if match {
			return
		}
	}

	require.Failf(t, "missing index", "schema %s should include index on %v", schema.Name, fields)
}
