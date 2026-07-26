package schema

import (
	"testing"

	"entgo.io/ent/entc/load"
	"github.com/stretchr/testify/require"
)

func TestCustomDomainSchemas(t *testing.T) {
	spec, err := (&load.Config{Path: "."}).Load()
	require.NoError(t, err)

	schemas := map[string]*load.Schema{}
	for _, loadedSchema := range spec.Schemas {
		schemas[loadedSchema.Name] = loadedSchema
	}

	customDomain := requireSchema(t, schemas, "CustomDomain")
	requireSchemaFields(t, customDomain,
		"user_id",
		"domain",
		"status",
		"all_users",
		"verification_token",
		"verification_txt_name",
		"verification_txt_value",
		"cname_target",
		"verified_at",
		"last_checked_at",
		"last_error",
		"disabled_at",
		"disabled_reason",
		"created_at",
		"updated_at",
		"deleted_at",
	)
	requireSchemaEdges(t, customDomain, "user", "authorized_users")
	requireSchemaIndexes(t, customDomain,
		[]string{"user_id"},
		[]string{"status"},
		[]string{"all_users"},
	)

	customDomainUser := requireSchema(t, schemas, "CustomDomainUser")
	requireSchemaFields(t, customDomainUser, "custom_domain_id", "user_id", "created_at")
	requireSchemaEdges(t, customDomainUser, "custom_domain", "user")
	requireSchemaIndexes(t, customDomainUser, []string{"user_id"})

	userSchema := requireSchema(t, schemas, "User")
	requireSchemaEdges(t, userSchema, "custom_domains", "authorized_custom_domains")

	usageLog := requireSchema(t, schemas, "UsageLog")
	requireSchemaFields(t, usageLog, "custom_domain_id", "custom_domain")
	requireSchemaIndexes(t, usageLog,
		[]string{"custom_domain_id", "created_at"},
		[]string{"custom_domain", "created_at"},
	)
}

func requireSchemaEdges(t *testing.T, schema *load.Schema, names ...string) {
	t.Helper()

	edges := map[string]struct{}{}
	for _, edge := range schema.Edges {
		edges[edge.Name] = struct{}{}
	}

	for _, name := range names {
		_, ok := edges[name]
		require.True(t, ok, "schema %s should include edge %s", schema.Name, name)
	}
}

func requireSchemaIndexes(t *testing.T, schema *load.Schema, expected ...[]string) {
	t.Helper()

	for _, fields := range expected {
		found := false
		for _, schemaIndex := range schema.Indexes {
			if len(schemaIndex.Fields) != len(fields) {
				continue
			}
			match := true
			for i := range fields {
				if schemaIndex.Fields[i] != fields[i] {
					match = false
					break
				}
			}
			if match {
				found = true
				break
			}
		}
		require.True(t, found, "schema %s should include index on %v", schema.Name, fields)
	}
}
