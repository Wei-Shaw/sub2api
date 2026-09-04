package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	dbaccount "github.com/Wei-Shaw/sub2api/ent/account"
	dbaccountgroup "github.com/Wei-Shaw/sub2api/ent/accountgroup"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

func (r *accountRepository) SupportsAtomicAccountProvisioning() bool { return true }

func (r *accountRepository) UpdateProvisionedAccount(
	ctx context.Context,
	spec *service.AccountProvisioningSpec,
	probeEnabled *bool,
	rateSyncEnabled *bool,
	rateMultiplier *float64,
) error {
	return r.updateProvisionedAccount(ctx, spec, probeEnabled, rateSyncEnabled, rateMultiplier, true)
}

func (r *accountRepository) updateProvisionedAccount(
	ctx context.Context,
	spec *service.AccountProvisioningSpec,
	probeEnabled *bool,
	rateSyncEnabled *bool,
	rateMultiplier *float64,
	requireActiveProxies bool,
) error {
	if spec == nil || spec.Account == nil || spec.Account.ID <= 0 {
		return service.ErrAccountNilInput
	}
	normalized, err := spec.NormalizeAndValidate()
	if err != nil {
		return err
	}
	account := normalized.Account

	tx, err := r.client.Tx(ctx)
	if err != nil && !errors.Is(err, dbent.ErrTxStarted) {
		return err
	}
	txClient := r.client
	if tx != nil {
		defer func() { _ = tx.Rollback() }()
		txClient = tx.Client()
	}
	if err := verifyCodexIdentityTemplateAssignment(ctx, txClient, account); err != nil {
		return err
	}

	previousGroupIDs, err := loadProvisionedAccountGroupIDs(ctx, txClient, account.ID)
	if err != nil {
		return err
	}
	existingPolicy, previousAccountProxyID, previousPlatform, previousAccountType, err := loadProvisionedCodexPolicy(ctx, txClient, account.ID)
	if err != nil {
		return err
	}
	managedPolicy, identityChanged, err := service.PrepareCodexIdentityPolicyForAccountTransition(
		existingPolicy,
		previousPlatform,
		previousAccountType,
		*normalized.Identity,
		account.Platform,
		account.Type,
	)
	if err != nil {
		return err
	}
	accountProxyChanged := !sameOptionalInt64(previousAccountProxyID, account.ProxyID)
	if accountProxyChanged {
		identityChanged = rotateProfilesInheritingAccountProxy(existingPolicy, &managedPolicy) || identityChanged
	}
	normalized.Identity = &managedPolicy
	if err := validateProvisioningProxyReferences(ctx, txClient, account.ProxyID, managedPolicy, requireActiveProxies); err != nil {
		return err
	}
	if accountProxyChanged {
		if err := snapshotRotatedInheritedAccountProxy(ctx, txClient, account.ID, existingPolicy, managedPolicy, previousAccountProxyID); err != nil {
			return err
		}
	}

	account.Status = normalized.FinalStatus
	account.Schedulable = normalized.Schedulable
	account.ProvisioningState = service.AccountProvisioningActive
	account.CodexIdentityPolicy = managedPolicy
	if _, err := r.updateLockedAccount(ctx, txClient, account, probeEnabled, rateSyncEnabled, rateMultiplier); err != nil {
		return err
	}
	groups, err := replaceProvisionedAccountGroups(ctx, txClient, account.ID, normalized.GroupIDs)
	if err != nil {
		return err
	}
	if identityChanged {
		if err := transitionProvisionedCodexIdentity(ctx, txClient, account.ID, existingPolicy, managedPolicy); err != nil {
			return err
		}
	}
	if err := enqueueSchedulerOutbox(
		ctx,
		txClient,
		service.SchedulerOutboxEventAccountChanged,
		&account.ID,
		nil,
		buildSchedulerGroupPayload(mergeGroupIDs(previousGroupIDs, normalized.GroupIDs)),
	); err != nil {
		return err
	}
	if tx != nil {
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	account.GroupIDs = append([]int64(nil), normalized.GroupIDs...)
	account.AccountGroups = groups
	if tx != nil {
		r.syncSchedulerAccountSnapshot(ctx, account.ID)
	}
	return nil
}

// ProvisionAccount persists a complete account configuration as one database
// transaction. The row starts pending and is activated only after every related
// record and the scheduler outbox event are durable.
func (r *accountRepository) ProvisionAccount(ctx context.Context, spec *service.AccountProvisioningSpec) error {
	if spec == nil || spec.Account == nil {
		return service.ErrAccountNilInput
	}
	normalized, err := spec.NormalizeAndValidate()
	if err != nil {
		return err
	}
	account := normalized.Account
	policy, err := service.PrepareCodexIdentityPolicyForCreate(*normalized.Identity, account.Platform, account.Type)
	if err != nil {
		return err
	}
	normalized.Identity = &policy
	policyMap, err := service.EncodeCodexIdentityPolicy(policy)
	if err != nil {
		return err
	}

	tx, err := r.client.Tx(ctx)
	if err != nil && !errors.Is(err, dbent.ErrTxStarted) {
		return err
	}
	txClient := r.client
	if tx != nil {
		defer func() { _ = tx.Rollback() }()
		txClient = tx.Client()
	}
	if err := verifyCodexIdentityTemplateAssignment(ctx, txClient, account); err != nil {
		return err
	}
	if err := validateProvisioningProxyReferences(ctx, txClient, account.ProxyID, policy, true); err != nil {
		return err
	}

	account.Status = normalized.FinalStatus
	account.Schedulable = false
	account.ProvisioningState = service.AccountProvisioningPending
	account.CodexIdentityPolicy = policy
	if err := createAccountRecord(ctx, txClient, account); err != nil {
		return err
	}

	groups, err := createProvisionedAccountGroups(ctx, txClient, account.ID, normalized.GroupIDs)
	if err != nil {
		return err
	}
	if err := createProvisionedCodexIdentity(ctx, txClient, account.ID, policy); err != nil {
		return err
	}

	finalSchedulable := normalized.Schedulable && normalized.ProvisioningState == service.AccountProvisioningActive
	if _, err := txClient.Account.UpdateOneID(account.ID).
		SetStatus(normalized.FinalStatus).
		SetSchedulable(finalSchedulable).
		SetProvisioningState(string(normalized.ProvisioningState)).
		SetCodexIdentityPolicy(policyMap).
		Save(ctx); err != nil {
		return err
	}
	if normalized.ProvisioningState == service.AccountProvisioningActive {
		if err := enqueueSchedulerOutbox(
			ctx,
			txClient,
			service.SchedulerOutboxEventAccountChanged,
			&account.ID,
			nil,
			buildSchedulerGroupPayload(normalized.GroupIDs),
		); err != nil {
			return err
		}
	}
	if tx != nil {
		if err := tx.Commit(); err != nil {
			return err
		}
	}

	account.Status = normalized.FinalStatus
	account.Schedulable = finalSchedulable
	account.ProvisioningState = normalized.ProvisioningState
	account.CodexIdentityPolicy = policy
	account.GroupIDs = append([]int64(nil), normalized.GroupIDs...)
	account.AccountGroups = groups
	return nil
}

func verifyCodexIdentityTemplateAssignment(ctx context.Context, client sqlExecutor, account *service.Account) error {
	if account == nil || account.CodexIdentityTemplateID == nil {
		return nil
	}
	if account.CodexIdentityTemplateAppliedRevision == nil ||
		*account.CodexIdentityTemplateID <= 0 ||
		*account.CodexIdentityTemplateAppliedRevision <= 0 {
		return infraerrors.BadRequest(
			"INVALID_CODEX_IDENTITY_ASSIGNMENT",
			"a template assignment requires a positive template id and applied revision",
		)
	}
	rows, err := client.QueryContext(ctx, `
		SELECT revision FROM codex_identity_templates WHERE id=$1 FOR SHARE
	`, *account.CodexIdentityTemplateID)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return err
		}
		return service.ErrCodexIdentityTemplateNotFound
	}
	var currentRevision int64
	if err := rows.Scan(&currentRevision); err != nil {
		return err
	}
	if currentRevision != *account.CodexIdentityTemplateAppliedRevision {
		return service.ErrCodexIdentityTemplateRevisionConflict.WithMetadata(map[string]string{
			"expected_revision": fmt.Sprintf("%d", *account.CodexIdentityTemplateAppliedRevision),
			"current_revision":  fmt.Sprintf("%d", currentRevision),
		})
	}
	return nil
}

func createProvisionedAccountGroups(
	ctx context.Context,
	client *dbent.Client,
	accountID int64,
	groupIDs []int64,
) ([]service.AccountGroup, error) {
	if len(groupIDs) == 0 {
		return nil, nil
	}
	builders := make([]*dbent.AccountGroupCreate, 0, len(groupIDs))
	groups := make([]service.AccountGroup, 0, len(groupIDs))
	seen := make(map[int64]struct{}, len(groupIDs))
	for _, groupID := range groupIDs {
		if groupID <= 0 {
			return nil, fmt.Errorf("invalid group id %d", groupID)
		}
		if _, exists := seen[groupID]; exists {
			return nil, fmt.Errorf("duplicate group id %d", groupID)
		}
		seen[groupID] = struct{}{}
		priority := len(groups) + 1
		builders = append(builders, client.AccountGroup.Create().
			SetAccountID(accountID).
			SetGroupID(groupID).
			SetPriority(priority))
		groups = append(groups, service.AccountGroup{AccountID: accountID, GroupID: groupID, Priority: priority})
	}
	if _, err := client.AccountGroup.CreateBulk(builders...).Save(ctx); err != nil {
		return nil, err
	}
	return groups, nil
}

func replaceProvisionedAccountGroups(
	ctx context.Context,
	client *dbent.Client,
	accountID int64,
	groupIDs []int64,
) ([]service.AccountGroup, error) {
	if _, err := client.AccountGroup.Delete().Where(dbaccountgroup.AccountIDEQ(accountID)).Exec(ctx); err != nil {
		return nil, err
	}
	return createProvisionedAccountGroups(ctx, client, accountID, groupIDs)
}

func loadProvisionedAccountGroupIDs(ctx context.Context, client *dbent.Client, accountID int64) ([]int64, error) {
	rows, err := client.AccountGroup.Query().Where(dbaccountgroup.AccountIDEQ(accountID)).All(ctx)
	if err != nil {
		return nil, err
	}
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.GroupID)
	}
	return ids, nil
}

func loadProvisionedCodexPolicy(
	ctx context.Context,
	client *dbent.Client,
	accountID int64,
) (service.CodexIdentityPolicySpec, *int64, string, string, error) {
	row, err := client.Account.Query().
		Where(dbaccount.IDEQ(accountID)).
		Select(dbaccount.FieldCodexIdentityPolicy, dbaccount.FieldProxyID, dbaccount.FieldPlatform, dbaccount.FieldType).
		ForUpdate().
		Only(ctx)
	if err != nil {
		return service.CodexIdentityPolicySpec{}, nil, "", "", err
	}
	policy, err := service.DecodeCodexIdentityPolicy(row.CodexIdentityPolicy, row.Platform, row.Type)
	return policy, row.ProxyID, row.Platform, row.Type, err
}

func rotateProfilesInheritingAccountProxy(
	previous service.CodexIdentityPolicySpec,
	next *service.CodexIdentityPolicySpec,
) bool {
	if next == nil || next.Mode != service.CodexIdentityPolicyOSProfileDevicePool {
		return false
	}
	previousByProfile := make(map[string]service.CodexOSProfilePolicy, len(previous.Profiles))
	for _, profile := range previous.Profiles {
		previousByProfile[codexProvisionedProfileKey(profile)] = profile
	}
	rotated := false
	for i := range next.Profiles {
		profile := &next.Profiles[i]
		if !codexProfileInheritsAccountProxy(*profile) {
			continue
		}
		oldProfile, exists := previousByProfile[codexProvisionedProfileKey(*profile)]
		if !exists {
			continue
		}
		if profile.Epoch <= oldProfile.Epoch {
			profile.Epoch = oldProfile.Epoch + 1
		}
		rotated = true
	}
	if rotated && next.Version <= previous.Version {
		next.Version = previous.Version + 1
	}
	return rotated
}

func codexProfileInheritsAccountProxy(profile service.CodexOSProfilePolicy) bool {
	if profile.ProxyMode != service.CodexProxyInherit {
		return false
	}
	overridden := make(map[int]service.CodexProxyMode, len(profile.Slots))
	for _, slot := range profile.Slots {
		overridden[slot.Index] = slot.ProxyMode
	}
	for index := 0; index < profile.SlotCount; index++ {
		if mode, exists := overridden[index]; !exists || mode == service.CodexProxyInherit {
			return true
		}
	}
	return false
}

func sameOptionalInt64(left, right *int64) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func snapshotRotatedInheritedAccountProxy(
	ctx context.Context,
	client *dbent.Client,
	accountID int64,
	previous service.CodexIdentityPolicySpec,
	next service.CodexIdentityPolicySpec,
	previousAccountProxyID *int64,
) error {
	nextByProfile := make(map[string]service.CodexOSProfilePolicy, len(next.Profiles))
	for _, profile := range next.Profiles {
		nextByProfile[codexProvisionedProfileKey(profile)] = profile
	}
	for _, oldProfile := range previous.Profiles {
		newProfile, exists := nextByProfile[codexProvisionedProfileKey(oldProfile)]
		if !exists || newProfile.Epoch <= oldProfile.Epoch || !codexProfileInheritsAccountProxy(oldProfile) {
			continue
		}
		inheritedIndices := make([]int64, 0, oldProfile.SlotCount)
		overrides := make(map[int]service.CodexProxyMode, len(oldProfile.Slots))
		for _, slot := range oldProfile.Slots {
			overrides[slot.Index] = slot.ProxyMode
		}
		for index := 0; index < oldProfile.SlotCount; index++ {
			if mode, exists := overrides[index]; !exists || mode == service.CodexProxyInherit {
				inheritedIndices = append(inheritedIndices, int64(index))
			}
		}
		if len(inheritedIndices) == 0 {
			continue
		}
		if previousAccountProxyID == nil {
			if _, err := client.ExecContext(ctx, `
				DELETE FROM account_codex_device_bindings AS bindings
				USING account_codex_device_slots AS slots, account_codex_profiles AS profiles
				WHERE bindings.slot_id=slots.id
				  AND slots.profile_id=profiles.id
				  AND bindings.account_id=$1
				  AND profiles.os_class=$2
				  AND profiles.canonical_surface=$3
				  AND profiles.epoch=$4
				  AND slots.slot_index=ANY($5)
			`, accountID, oldProfile.OSClass, oldProfile.CanonicalSurface, oldProfile.Epoch, pq.Array(inheritedIndices)); err != nil {
				return err
			}
			continue
		}
		if _, err := client.ExecContext(ctx, `
			UPDATE account_codex_device_slots AS slots
			SET proxy_mode='proxy', proxy_id=$1, updated_at=NOW()
			FROM account_codex_profiles AS profiles
			WHERE slots.profile_id=profiles.id
			  AND slots.account_id=$2
			  AND profiles.os_class=$3
			  AND profiles.canonical_surface=$4
			  AND profiles.epoch=$5
			  AND slots.slot_index=ANY($6)
			  AND slots.proxy_id IS NULL
		`, *previousAccountProxyID, accountID, oldProfile.OSClass, oldProfile.CanonicalSurface, oldProfile.Epoch, pq.Array(inheritedIndices)); err != nil {
			return err
		}
	}
	return nil
}

func transitionProvisionedCodexIdentity(
	ctx context.Context,
	client sqlExecutor,
	accountID int64,
	previous service.CodexIdentityPolicySpec,
	next service.CodexIdentityPolicySpec,
) error {
	nextByProfile := make(map[string]service.CodexOSProfilePolicy, len(next.Profiles))
	for _, profile := range next.Profiles {
		nextByProfile[codexProvisionedProfileKey(profile)] = profile
	}
	for _, oldProfile := range previous.Profiles {
		newProfile, exists := nextByProfile[codexProvisionedProfileKey(oldProfile)]
		if exists && newProfile.Epoch == oldProfile.Epoch {
			continue
		}
		if !exists {
			if _, err := client.ExecContext(ctx, `
				DELETE FROM account_codex_device_bindings
				WHERE account_id=$1 AND os_class=$2 AND canonical_surface=$3
			`, accountID, oldProfile.OSClass, oldProfile.CanonicalSurface); err != nil {
				return err
			}
		}
		if _, err := client.ExecContext(ctx, `
			UPDATE account_codex_device_slots AS slots
			SET state='draining', updated_at=NOW()
			FROM account_codex_profiles AS profiles
			WHERE slots.profile_id=profiles.id
			  AND slots.account_id=$1
			  AND profiles.os_class=$2
			  AND profiles.canonical_surface=$3
			  AND profiles.epoch=$4
			  AND slots.state='active'
		`, accountID, oldProfile.OSClass, oldProfile.CanonicalSurface, oldProfile.Epoch); err != nil {
			return err
		}
	}
	if err := createProvisionedCodexIdentity(ctx, client, accountID, next); err != nil {
		return err
	}
	if _, err := client.ExecContext(ctx, `
		UPDATE account_codex_device_bindings
		SET policy_version=$1, updated_at=NOW()
		WHERE account_id=$2
	`, next.Version, accountID); err != nil {
		return err
	}
	_, err := finalizeDrainedCodexDeviceSlots(ctx, client, accountID)
	return err
}

func createProvisionedCodexIdentity(
	ctx context.Context,
	client sqlExecutor,
	accountID int64,
	policy service.CodexIdentityPolicySpec,
) error {
	sessionPolicy, err := json.Marshal(policy.SessionPolicy)
	if err != nil {
		return err
	}
	if _, err := client.ExecContext(ctx, `
		INSERT INTO account_codex_identity_policies
			(account_id, mode, binding_scope, session_policy, affinity_ttl_seconds, unsupported_policy, version)
		VALUES ($1, $2, $3, $4::jsonb, $5, $6, $7)
		ON CONFLICT (account_id) DO UPDATE SET
			mode=EXCLUDED.mode,
			binding_scope=EXCLUDED.binding_scope,
			session_policy=EXCLUDED.session_policy,
			affinity_ttl_seconds=EXCLUDED.affinity_ttl_seconds,
			unsupported_policy=EXCLUDED.unsupported_policy,
			version=EXCLUDED.version,
			updated_at=NOW()
	`, accountID, policy.Mode, policy.BindingScope, string(sessionPolicy), policy.AffinityTTLSeconds, policy.UnsupportedPolicy, policy.Version); err != nil {
		return err
	}

	for _, profile := range policy.Profiles {
		profileID, err := insertProvisionedCodexProfile(ctx, client, accountID, profile)
		if err != nil {
			return err
		}
		overrides := make(map[int]service.CodexDeviceSlotPolicy, len(profile.Slots))
		for _, slot := range profile.Slots {
			overrides[slot.Index] = slot
		}
		for slotIndex := 0; slotIndex < profile.SlotCount; slotIndex++ {
			override, exists := overrides[slotIndex]
			proxyMode := service.CodexProxyInherit
			var proxyID *int64
			clientVersionMode := service.CodexClientVersionInherit
			clientVersion := ""
			if exists {
				proxyMode = override.ProxyMode
				proxyID = override.ProxyID
				clientVersionMode = override.ClientVersionMode
				clientVersion = override.ClientVersion
			}
			if _, err := client.ExecContext(ctx, `
				INSERT INTO account_codex_device_slots
					(account_id, profile_id, slot_index, proxy_mode, proxy_id,
					 client_version_mode, client_version, epoch, state)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'active')
				ON CONFLICT (profile_id, slot_index, epoch) DO NOTHING
			`, accountID, profileID, slotIndex, proxyMode, proxyID,
				clientVersionMode, clientVersion, profile.Epoch); err != nil {
				return err
			}
		}
	}
	return nil
}

func insertProvisionedCodexProfile(
	ctx context.Context,
	client sqlExecutor,
	accountID int64,
	profile service.CodexOSProfilePolicy,
) (int64, error) {
	rows, err := client.QueryContext(ctx, `
		INSERT INTO account_codex_profiles
			(account_id, os_class, canonical_surface, architecture, proxy_mode, proxy_id, slot_count, epoch, catalog_version)
		VALUES ($1, $2, $3, NULLIF($4, ''), $5, $6, $7, $8, $9)
		ON CONFLICT (account_id, os_class, canonical_surface, epoch) DO UPDATE SET
			canonical_surface=EXCLUDED.canonical_surface,
			architecture=EXCLUDED.architecture,
			proxy_mode=EXCLUDED.proxy_mode,
			proxy_id=EXCLUDED.proxy_id,
			slot_count=EXCLUDED.slot_count,
			catalog_version=EXCLUDED.catalog_version,
			updated_at=NOW()
		RETURNING id
	`, accountID, profile.OSClass, profile.CanonicalSurface, profile.Architecture, profile.ProxyMode, profile.ProxyID, profile.SlotCount, profile.Epoch, profile.CatalogVersion)
	if err != nil {
		return 0, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return 0, err
		}
		return 0, errors.New("profile insert returned no id")
	}
	var profileID int64
	if err := rows.Scan(&profileID); err != nil {
		return 0, err
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	return profileID, nil
}

func codexProvisionedProfileKey(profile service.CodexOSProfilePolicy) string {
	return string(profile.OSClass) + "/" + string(profile.CanonicalSurface)
}

func validateProvisioningProxyReferences(
	ctx context.Context,
	client *dbent.Client,
	accountProxyID *int64,
	policy service.CodexIdentityPolicySpec,
	requireActive bool,
) error {
	if policy.Mode != service.CodexIdentityPolicyOSProfileDevicePool {
		return nil
	}
	proxyIDs := policy.ReferencedProxyIDs()
	if accountProxyID != nil {
		proxyIDs = append(proxyIDs, *accountProxyID)
	}
	seen := make(map[int64]struct{}, len(proxyIDs))
	uniqueIDs := make([]int64, 0, len(proxyIDs))
	for _, id := range proxyIDs {
		if id <= 0 {
			return infraerrors.BadRequest("ACCOUNT_PROVISIONING_INVALID", "proxy_id must be positive")
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		uniqueIDs = append(uniqueIDs, id)
	}
	if len(uniqueIDs) == 0 {
		return nil
	}
	rows, err := client.QueryContext(ctx, `
		SELECT id, status, expires_at
		FROM proxies
		WHERE id=ANY($1) AND deleted_at IS NULL
		FOR UPDATE
	`, pq.Array(uniqueIDs))
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	valid := make(map[int64]struct{}, len(uniqueIDs))
	now := time.Now()
	for rows.Next() {
		var id int64
		var status string
		var expiresAt sql.NullTime
		if err := rows.Scan(&id, &status, &expiresAt); err != nil {
			return err
		}
		if requireActive && (status != service.StatusActive || (expiresAt.Valid && !expiresAt.Time.After(now))) {
			return infraerrors.BadRequest("ACCOUNT_PROVISIONING_INVALID", fmt.Sprintf("proxy %d is not active", id))
		}
		valid[id] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, id := range uniqueIDs {
		if _, exists := valid[id]; !exists {
			return infraerrors.BadRequest("ACCOUNT_PROVISIONING_INVALID", fmt.Sprintf("proxy %d does not exist", id))
		}
	}
	return nil
}
