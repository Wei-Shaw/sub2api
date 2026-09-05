package repository

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

func (r *accountRepository) RefreshCodexConversationBinding(ctx context.Context, bindingID int64) error {
	result, err := r.client.ExecContext(ctx, "UPDATE account_codex_device_bindings SET updated_at=NOW() WHERE id=$1", bindingID)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return fmt.Errorf("Codex conversation binding is no longer available")
	}
	return nil
}

func validCodexConversationHash(hash string) bool {
	decoded, err := hex.DecodeString(hash)
	return err == nil && len(decoded) == 32 && hex.EncodeToString(decoded) == hash
}

func (r *accountRepository) FindCodexConversationBinding(ctx context.Context, accountID, apiKeyID int64, os service.CodexOSClass, surface service.CodexClientSurface, hash string) (*service.CodexResolvedDeviceSlot, error) {
	if !validCodexConversationHash(hash) {
		return nil, fmt.Errorf("invalid Codex conversation hash")
	}
	// Refresh affinity before returning the slot. Only a digest is persisted.
	_, err := r.client.ExecContext(ctx, `
        UPDATE account_codex_device_bindings SET updated_at=NOW()
        WHERE account_id=$1 AND api_key_id=$2 AND os_class=$3 AND canonical_surface=$4 AND conversation_hash=$5
    `, accountID, apiKeyID, os, surface, hash)
	if err != nil {
		return nil, err
	}
	slot, err := loadCodexConversationBinding(ctx, r.client, accountID, apiKeyID, os, surface, hash)
	if err != nil {
		return nil, err
	}
	return r.hydrateCodexResolvedDeviceSlotProxy(ctx, slot)
}

func (r *accountRepository) BindCodexConversationSlot(ctx context.Context, accountID, apiKeyID int64, os service.CodexOSClass, surface service.CodexClientSurface, hash string, slotID int64) (*service.CodexResolvedDeviceSlot, error) {
	if !validCodexConversationHash(hash) {
		return nil, fmt.Errorf("invalid Codex conversation hash")
	}
	tx, err := r.client.Tx(ctx)
	if err != nil && !errors.Is(err, dbent.ErrTxStarted) {
		return nil, err
	}
	client := r.client
	if tx != nil {
		defer func() { _ = tx.Rollback() }()
		client = tx.Client()
	}
	rows, err := client.QueryContext(ctx, `
        SELECT slots.id FROM account_codex_device_slots AS slots
        JOIN account_codex_profiles AS profiles ON profiles.id=slots.profile_id
        JOIN accounts ON accounts.id=slots.account_id
        JOIN api_keys ON api_keys.id=$2 AND api_keys.deleted_at IS NULL
        WHERE slots.account_id=$1 AND slots.id=$3
          AND profiles.os_class=$4 AND profiles.canonical_surface=$5
          AND slots.state IN ('active','draining')
          AND accounts.deleted_at IS NULL AND accounts.status='active'
          AND accounts.provisioning_state='active' AND accounts.schedulable=TRUE
        FOR SHARE OF slots, accounts, api_keys
    `, accountID, apiKeyID, slotID, os, surface)
	if err != nil {
		return nil, err
	}
	found := rows.Next()
	rowErr := rows.Err()
	_ = rows.Close()
	if rowErr != nil {
		return nil, rowErr
	}
	if !found {
		return nil, service.ErrDeviceProfileUnsupported
	}
	_, err = client.ExecContext(ctx, `
        INSERT INTO account_codex_device_bindings
            (account_id,api_key_id,os_class,canonical_surface,conversation_hash,slot_id,policy_version)
        SELECT $1,$2,$3,$4,$5,$6,version FROM account_codex_identity_policies WHERE account_id=$1
        ON CONFLICT (account_id,api_key_id,os_class,canonical_surface,conversation_hash)
        DO UPDATE SET updated_at=NOW()
    `, accountID, apiKeyID, os, surface, hash, slotID)
	if err != nil {
		return nil, err
	}
	// First writer wins: never move a conversation already bound by another request.
	slot, err := loadCodexConversationBinding(ctx, client, accountID, apiKeyID, os, surface, hash)
	if err != nil {
		return nil, err
	}
	if slot == nil {
		return nil, fmt.Errorf("Codex conversation binding disappeared")
	}
	if tx != nil {
		if err := tx.Commit(); err != nil {
			return nil, err
		}
	}
	return r.hydrateCodexResolvedDeviceSlotProxy(ctx, slot)
}
