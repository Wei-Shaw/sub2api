//go:build integration

package repository

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCodexIdentityShadowExclusionSerializesConcurrentWriters(t *testing.T) {
	ctx := context.Background()
	createParent := func(label string) int64 {
		var id int64
		err := integrationDB.QueryRowContext(ctx, `
			INSERT INTO accounts
				(name, platform, type, credentials, extra, status, schedulable, provisioning_state, codex_identity_policy)
			VALUES
				($1, 'openai', 'oauth', '{"access_token":"test-token"}'::jsonb,
				 '{"codex_fingerprint_seed":"11111111-1111-4111-8111-111111111111"}'::jsonb,
				 'active', TRUE, 'active',
				 '{"mode":"off","binding_scope":"api_key_os","session_policy":{"mode":"conversation_isolated"},"affinity_ttl_seconds":3600,"unsupported_policy":"reject","version":1}'::jsonb)
			RETURNING id
		`, fmt.Sprintf("shadow-lock-%s-%d", label, time.Now().UnixNano())).Scan(&id)
		require.NoError(t, err)
		t.Cleanup(func() { _, _ = integrationDB.ExecContext(context.Background(), "DELETE FROM accounts WHERE id=$1", id) })
		return id
	}
	insertShadow := func(tx *sql.Tx, parentID int64) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO accounts
				(name, platform, type, credentials, extra, status, schedulable, provisioning_state,
				 codex_identity_policy, parent_account_id, quota_dimension)
			VALUES
				($1, 'openai', 'oauth', '{}'::jsonb, '{}'::jsonb, 'active', TRUE, 'active',
				 '{"mode":"off"}'::jsonb, $2, 'spark')
		`, fmt.Sprintf("shadow-%d-%d", parentID, time.Now().UnixNano()), parentID)
		return err
	}
	enablePool := func(tx *sql.Tx, parentID int64) error {
		_, err := tx.ExecContext(ctx, `
			UPDATE accounts
			SET codex_identity_policy='{"mode":"os_profile_device_pool","binding_scope":"api_key_os","session_policy":{"mode":"conversation_isolated"},"affinity_ttl_seconds":3600,"unsupported_policy":"reject","version":1,"profiles":[{"os_class":"linux","canonical_surface":"cli","architecture":"x86_64","slot_count":1,"epoch":1,"catalog_version":1}]}'::jsonb
			WHERE id=$1
		`, parentID)
		return err
	}
	assertBlocked := func(t *testing.T, result <-chan error) {
		t.Helper()
		select {
		case err := <-result:
			t.Fatalf("concurrent writer completed before parent lock released: %v", err)
		case <-time.After(150 * time.Millisecond):
		}
	}

	t.Run("shadow commits first", func(t *testing.T) {
		parentID := createParent("shadow-first")
		shadowTx, err := integrationDB.BeginTx(ctx, nil)
		require.NoError(t, err)
		require.NoError(t, insertShadow(shadowTx, parentID))
		parentTx, err := integrationDB.BeginTx(ctx, nil)
		require.NoError(t, err)
		result := make(chan error, 1)
		go func() { result <- enablePool(parentTx, parentID) }()
		assertBlocked(t, result)
		require.NoError(t, shadowTx.Commit())
		require.Error(t, <-result)
		_ = parentTx.Rollback()
	})

	t.Run("parent commits first", func(t *testing.T) {
		parentID := createParent("parent-first")
		parentTx, err := integrationDB.BeginTx(ctx, nil)
		require.NoError(t, err)
		require.NoError(t, enablePool(parentTx, parentID))
		shadowTx, err := integrationDB.BeginTx(ctx, nil)
		require.NoError(t, err)
		result := make(chan error, 1)
		go func() { result <- insertShadow(shadowTx, parentID) }()
		assertBlocked(t, result)
		require.NoError(t, parentTx.Commit())
		require.Error(t, <-result)
		_ = shadowTx.Rollback()
	})

	t.Run("soft delete is allowed but restore under enabled parent is rejected", func(t *testing.T) {
		parentID := createParent("restore")
		shadowTx, err := integrationDB.BeginTx(ctx, nil)
		require.NoError(t, err)
		require.NoError(t, insertShadow(shadowTx, parentID))
		require.NoError(t, shadowTx.Commit())
		var shadowID int64
		require.NoError(t, integrationDB.QueryRowContext(ctx, `
			SELECT id FROM accounts
			WHERE parent_account_id=$1 AND quota_dimension='spark' AND deleted_at IS NULL
		`, parentID).Scan(&shadowID))
		_, err = integrationDB.ExecContext(ctx, "UPDATE accounts SET deleted_at=NOW() WHERE id=$1", shadowID)
		require.NoError(t, err, "soft-deleting a shadow must remain possible")
		parentTx, err := integrationDB.BeginTx(ctx, nil)
		require.NoError(t, err)
		require.NoError(t, enablePool(parentTx, parentID))
		require.NoError(t, parentTx.Commit())
		_, err = integrationDB.ExecContext(ctx, "UPDATE accounts SET deleted_at=NULL WHERE id=$1", shadowID)
		require.Error(t, err, "restoring a shadow under a profile-enabled parent must be rejected")
	})
}
