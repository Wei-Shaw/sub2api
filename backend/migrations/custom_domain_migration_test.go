package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCustomDomainMigrationContract(t *testing.T) {
	data, err := FS.ReadFile("182_custom_domains.sql")
	require.NoError(t, err)

	sql := string(data)
	for _, fragment := range []string{
		"CREATE TABLE IF NOT EXISTS custom_domains",
		"verification_token",
		"verification_txt_name",
		"verification_txt_value",
		"CONSTRAINT custom_domains_status_check",
		"CREATE UNIQUE INDEX IF NOT EXISTS custom_domains_domain_unique_active",
		"ON custom_domains (lower(domain))",
		"WHERE deleted_at IS NULL",
		"CREATE TABLE IF NOT EXISTS custom_domain_users",
		"PRIMARY KEY (custom_domain_id, user_id)",
		"custom_domain_id BIGINT NOT NULL REFERENCES custom_domains(id) ON DELETE CASCADE",
		"user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE",
		"ADD COLUMN IF NOT EXISTS custom_domain_id BIGINT NULL",
		"ADD COLUMN IF NOT EXISTS custom_domain VARCHAR(253) NULL",
		"CREATE INDEX IF NOT EXISTS usage_logs_custom_domain_id_created_at_idx",
		"CREATE INDEX IF NOT EXISTS usage_logs_custom_domain_created_at_idx",
		"VALUES ('custom_domains_enabled', 'false')",
	} {
		require.True(t, strings.Contains(sql, fragment), "migration should contain %q", fragment)
	}
}
