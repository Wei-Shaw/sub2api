package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCodexOSProfileDevicePoolMigrationDefinesLifecycleGraph(t *testing.T) {
	payload, err := FS.ReadFile("234_codex_os_profile_device_pool.sql")
	require.NoError(t, err)
	sql := string(payload)
	for _, required := range []string{
		"provisioning_state",
		"DEFAULT 'pending'",
		"account_codex_identity_policies",
		"id                  BIGSERIAL PRIMARY KEY",
		"account_id          BIGINT NOT NULL UNIQUE REFERENCES accounts(id) ON DELETE CASCADE",
		"account_codex_profiles",
		"account_codex_device_slots",
		"account_codex_device_bindings",
		"max_active_conversations_per_slot",
		"disable_cross_key_continuation",
		"catalog_version",
		"state IN ('active', 'draining')",
		"accounts_codex_identity_mode_exclusive_check",
		"accounts_codex_identity_active_credentials_check",
		"agentIdentity",
		"agent_runtime_id",
		"agent_private_key",
		"accounts_codex_identity_active_seed_check",
		"enforce_pending_account_unschedulable",
		"trg_accounts_pending_unschedulable",
		"enforce_codex_identity_shadow_exclusion",
		"trg_accounts_codex_identity_shadow_exclusion",
	} {
		require.Truef(t, strings.Contains(sql, required), "migration must contain %q", required)
	}
}
