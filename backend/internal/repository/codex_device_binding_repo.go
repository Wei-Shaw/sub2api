package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

func (r *accountRepository) ResolveCodexDeviceBinding(
	ctx context.Context,
	accountID int64,
	apiKeyID int64,
	osClass service.CodexOSClass,
) (*service.CodexResolvedDeviceSlot, error) {
	if accountID <= 0 || apiKeyID <= 0 {
		return nil, service.ErrDeviceProfileUnsupported
	}
	if _, err := (service.CodexIdentityPolicySpec{
		Mode: service.CodexIdentityPolicyOSProfileDevicePool,
		Profiles: []service.CodexOSProfilePolicy{{
			OSClass: osClass, CanonicalSurface: canonicalSurfaceForBindingValidation(osClass),
			Architecture: architectureForBindingValidation(osClass), SlotCount: 1,
		}},
	}).NormalizeAndValidate(service.PlatformOpenAI, service.AccountTypeOAuth); err != nil {
		return nil, service.ErrDeviceProfileUnsupported
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
	resolved, err := resolveCodexDeviceBinding(ctx, client, accountID, apiKeyID, osClass)
	if err != nil {
		return nil, err
	}
	if tx != nil {
		if err := tx.Commit(); err != nil {
			return nil, err
		}
	}
	return r.hydrateCodexResolvedDeviceSlotProxy(ctx, resolved)
}

func (r *accountRepository) RebindCodexDeviceBinding(
	ctx context.Context,
	oldAccountID int64,
	newAccountID int64,
	apiKeyID int64,
	osClass service.CodexOSClass,
) (*service.CodexResolvedDeviceSlot, error) {
	if oldAccountID <= 0 || newAccountID <= 0 || apiKeyID <= 0 {
		return nil, service.ErrDeviceProfileUnsupported
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
	if _, err := client.ExecContext(ctx, `
		DELETE FROM account_codex_device_bindings
		WHERE account_id=$1 AND api_key_id=$2 AND os_class=$3
	`, oldAccountID, apiKeyID, osClass); err != nil {
		return nil, err
	}
	if _, err := finalizeDrainedCodexDeviceSlots(ctx, client, oldAccountID); err != nil {
		return nil, err
	}
	resolved, err := resolveCodexDeviceBinding(ctx, client, newAccountID, apiKeyID, osClass)
	if err != nil {
		return nil, err
	}
	if tx != nil {
		if err := tx.Commit(); err != nil {
			return nil, err
		}
	}
	return r.hydrateCodexResolvedDeviceSlotProxy(ctx, resolved)
}

func (r *accountRepository) hydrateCodexResolvedDeviceSlotProxy(
	ctx context.Context,
	resolved *service.CodexResolvedDeviceSlot,
) (*service.CodexResolvedDeviceSlot, error) {
	if resolved == nil || resolved.ProxyID == nil {
		return resolved, nil
	}
	proxyEntity, err := r.client.Proxy.Get(ctx, *resolved.ProxyID)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, service.ErrProxyNotFound
		}
		return nil, err
	}
	resolved.Proxy = proxyEntityToService(proxyEntity)
	return resolved, nil
}

func (r *accountRepository) DeleteCodexDeviceBinding(
	ctx context.Context,
	accountID int64,
	apiKeyID int64,
	osClass service.CodexOSClass,
) error {
	tx, err := r.client.Tx(ctx)
	if err != nil && !errors.Is(err, dbent.ErrTxStarted) {
		return err
	}
	client := r.client
	if tx != nil {
		defer func() { _ = tx.Rollback() }()
		client = tx.Client()
	}
	if _, err := client.ExecContext(ctx, `
		DELETE FROM account_codex_device_bindings
		WHERE account_id=$1 AND api_key_id=$2 AND os_class=$3
	`, accountID, apiKeyID, osClass); err != nil {
		return err
	}
	if _, err := finalizeDrainedCodexDeviceSlots(ctx, client, accountID); err != nil {
		return err
	}
	if tx != nil {
		return tx.Commit()
	}
	return nil
}

func (r *accountRepository) ListCodexDeviceSlots(
	ctx context.Context,
	accountID int64,
	osClass service.CodexOSClass,
	includeDraining bool,
) ([]service.CodexResolvedDeviceSlot, error) {
	stateClause := "AND slots.state='active'"
	if includeDraining {
		stateClause = "AND slots.state IN ('active','draining')"
	}
	osClause := ""
	args := []any{accountID}
	if osClass != "" {
		osClause = "AND profiles.os_class=$2"
		args = append(args, osClass)
	}
	rows, err := r.client.QueryContext(ctx, `
		SELECT profiles.id, slots.id, profiles.os_class, profiles.canonical_surface,
		       COALESCE(profiles.architecture, ''), profiles.catalog_version, slots.slot_index, slots.epoch,
		       slots.state, policies.version,
		       (SELECT COUNT(*) FROM account_codex_device_bindings AS bindings WHERE bindings.slot_id=slots.id),
		       COALESCE(slots.proxy_id, profiles.proxy_id, accounts.proxy_id)
		FROM account_codex_device_slots AS slots
		JOIN account_codex_profiles AS profiles ON profiles.id=slots.profile_id AND profiles.account_id=slots.account_id
		JOIN account_codex_identity_policies AS policies ON policies.account_id=slots.account_id
		JOIN accounts ON accounts.id=slots.account_id
		WHERE slots.account_id=$1 `+osClause+`
		  `+stateClause+`
		ORDER BY profiles.epoch DESC, slots.slot_index ASC
	`, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	result := make([]service.CodexResolvedDeviceSlot, 0)
	for rows.Next() {
		resolved := service.CodexResolvedDeviceSlot{AccountID: accountID}
		var proxyID sql.NullInt64
		if err := rows.Scan(
			&resolved.ProfileID, &resolved.SlotID, &resolved.OSClass, &resolved.CanonicalSurface,
			&resolved.Architecture, &resolved.CatalogVersion, &resolved.SlotIndex, &resolved.Epoch, &resolved.State,
			&resolved.PolicyVersion, &resolved.BindingCount, &proxyID,
		); err != nil {
			return nil, err
		}
		if proxyID.Valid {
			resolved.ProxyID = &proxyID.Int64
		}
		result = append(result, resolved)
	}
	return result, rows.Err()
}

func (r *accountRepository) FinalizeDrainedCodexDeviceSlots(ctx context.Context, accountID int64) (int64, error) {
	tx, err := r.client.Tx(ctx)
	if err != nil && !errors.Is(err, dbent.ErrTxStarted) {
		return 0, err
	}
	client := r.client
	if tx != nil {
		defer func() { _ = tx.Rollback() }()
		client = tx.Client()
	}
	deleted, err := finalizeDrainedCodexDeviceSlots(ctx, client, accountID)
	if err != nil {
		return 0, err
	}
	if tx != nil {
		if err := tx.Commit(); err != nil {
			return 0, err
		}
	}
	return deleted, nil
}

func finalizeDrainedCodexDeviceSlots(ctx context.Context, client *dbent.Client, accountID int64) (int64, error) {
	if _, err := client.ExecContext(ctx, `
		DELETE FROM account_codex_device_bindings AS bindings
		USING account_codex_device_slots AS slots, account_codex_identity_policies AS policies
		WHERE bindings.slot_id=slots.id
		  AND bindings.account_id=policies.account_id
		  AND bindings.account_id=$1
		  AND slots.state='draining'
		  AND bindings.updated_at + make_interval(secs => policies.affinity_ttl_seconds) <= NOW()
	`, accountID); err != nil {
		return 0, err
	}
	result, err := client.ExecContext(ctx, `
		DELETE FROM account_codex_device_slots AS slots
		WHERE slots.account_id=$1
		  AND slots.state='draining'
		  AND NOT EXISTS (
		      SELECT 1 FROM account_codex_device_bindings AS bindings
		      WHERE bindings.slot_id=slots.id
		  )
	`, accountID)
	if err != nil {
		return 0, err
	}
	deleted, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	if _, err := client.ExecContext(ctx, `
		DELETE FROM account_codex_profiles AS profiles
		WHERE profiles.account_id=$1
		  AND NOT EXISTS (
		      SELECT 1 FROM account_codex_device_slots AS slots
		      WHERE slots.profile_id=profiles.id
		  )
	`, accountID); err != nil {
		return 0, err
	}
	return deleted, nil
}

func resolveCodexDeviceBinding(
	ctx context.Context,
	client *dbent.Client,
	accountID int64,
	apiKeyID int64,
	osClass service.CodexOSClass,
) (*service.CodexResolvedDeviceSlot, error) {
	if existing, err := loadCodexDeviceBinding(ctx, client, accountID, apiKeyID, osClass); err != nil {
		return nil, err
	} else if existing != nil {
		now := time.Now()
		drainingExpired := existing.State == "draining" &&
			existing.AffinityTTLSeconds > 0 &&
			!now.Before(existing.LastSeenAt.Add(time.Duration(existing.AffinityTTLSeconds)*time.Second))
		if !drainingExpired {
			if _, err := client.ExecContext(ctx, `
				UPDATE account_codex_device_bindings SET updated_at=NOW() WHERE id=$1
			`, existing.BindingID); err != nil {
				return nil, err
			}
			existing.LastSeenAt = now
			return existing, nil
		}
		if _, err := client.ExecContext(ctx, "DELETE FROM account_codex_device_bindings WHERE id=$1", existing.BindingID); err != nil {
			return nil, err
		}
		if _, err := finalizeDrainedCodexDeviceSlots(ctx, client, accountID); err != nil {
			return nil, err
		}
	}

	rows, err := client.QueryContext(ctx, `
		SELECT slots.id, profiles.id, policies.version
		FROM account_codex_device_slots AS slots
		JOIN account_codex_profiles AS profiles ON profiles.id=slots.profile_id AND profiles.account_id=slots.account_id
		JOIN account_codex_identity_policies AS policies ON policies.account_id=slots.account_id
		JOIN accounts ON accounts.id=slots.account_id
		WHERE slots.account_id=$1
		  AND profiles.os_class=$2
		  AND slots.state='active'
		  AND policies.mode='os_profile_device_pool'
		  AND accounts.provisioning_state='active'
		  AND accounts.status='active'
		  AND accounts.schedulable=TRUE
		ORDER BY md5($1::text || ':' || $3::text || ':' || slots.id::text)
		LIMIT 1
		FOR SHARE OF slots
	`, accountID, osClass, apiKeyID)
	if err != nil {
		return nil, err
	}
	var slotID, profileID, policyVersion int64
	if rows.Next() {
		err = rows.Scan(&slotID, &profileID, &policyVersion)
	} else if rows.Err() != nil {
		err = rows.Err()
	} else {
		err = service.ErrDeviceProfileUnsupported
	}
	_ = rows.Close()
	if err != nil {
		return nil, err
	}
	if _, err := client.ExecContext(ctx, `
		INSERT INTO account_codex_device_bindings
			(account_id, api_key_id, os_class, slot_id, policy_version)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (account_id, api_key_id, os_class) DO NOTHING
	`, accountID, apiKeyID, osClass, slotID, policyVersion); err != nil {
		return nil, err
	}
	resolved, err := loadCodexDeviceBinding(ctx, client, accountID, apiKeyID, osClass)
	if err != nil {
		return nil, err
	}
	if resolved == nil {
		return nil, fmt.Errorf("binding insert did not produce a readable row for profile %d", profileID)
	}
	return resolved, nil
}

func loadCodexDeviceBinding(
	ctx context.Context,
	client *dbent.Client,
	accountID int64,
	apiKeyID int64,
	osClass service.CodexOSClass,
) (*service.CodexResolvedDeviceSlot, error) {
	rows, err := client.QueryContext(ctx, `
		SELECT bindings.id, bindings.account_id, bindings.api_key_id,
		       profiles.id, slots.id, profiles.os_class, profiles.canonical_surface,
		       COALESCE(profiles.architecture, ''), profiles.catalog_version, slots.slot_index, slots.epoch,
		       slots.state, bindings.policy_version, bindings.updated_at, policies.affinity_ttl_seconds,
		       COALESCE(slots.proxy_id, profiles.proxy_id, accounts.proxy_id)
		FROM account_codex_device_bindings AS bindings
		JOIN account_codex_device_slots AS slots ON slots.id=bindings.slot_id AND slots.account_id=bindings.account_id
		JOIN account_codex_profiles AS profiles ON profiles.id=slots.profile_id AND profiles.account_id=slots.account_id
		JOIN account_codex_identity_policies AS policies ON policies.account_id=bindings.account_id
		JOIN accounts ON accounts.id=bindings.account_id
		WHERE bindings.account_id=$1 AND bindings.api_key_id=$2 AND bindings.os_class=$3
		LIMIT 1
		FOR UPDATE OF bindings
	`, accountID, apiKeyID, osClass)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, nil
	}
	resolved := &service.CodexResolvedDeviceSlot{}
	var proxyID sql.NullInt64
	if err := rows.Scan(
		&resolved.BindingID, &resolved.AccountID, &resolved.APIKeyID,
		&resolved.ProfileID, &resolved.SlotID, &resolved.OSClass, &resolved.CanonicalSurface,
		&resolved.Architecture, &resolved.CatalogVersion, &resolved.SlotIndex, &resolved.Epoch,
		&resolved.State, &resolved.PolicyVersion, &resolved.LastSeenAt, &resolved.AffinityTTLSeconds, &proxyID,
	); err != nil {
		return nil, err
	}
	if proxyID.Valid {
		resolved.ProxyID = &proxyID.Int64
	}
	return resolved, rows.Err()
}

func canonicalSurfaceForBindingValidation(osClass service.CodexOSClass) service.CodexClientSurface {
	if osClass == service.CodexOSGeneric {
		return service.CodexSurfaceSDK
	}
	return service.CodexSurfaceCLI
}

func architectureForBindingValidation(osClass service.CodexOSClass) service.CodexArchitecture {
	if osClass == service.CodexOSGeneric {
		return ""
	}
	return service.CodexArchX8664
}
