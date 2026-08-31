package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

type codexIdentityTemplateRepository struct {
	db             *sql.DB
	accountRepo    service.AccountRepository
	schedulerCache service.SchedulerCache
}

func NewCodexIdentityTemplateRepository(
	db *sql.DB,
	accountRepo service.AccountRepository,
	schedulerCache service.SchedulerCache,
) service.CodexIdentityTemplateRepository {
	return &codexIdentityTemplateRepository{db: db, accountRepo: accountRepo, schedulerCache: schedulerCache}
}

func (r *codexIdentityTemplateRepository) ListCodexIdentityTemplates(ctx context.Context) ([]*service.CodexIdentityTemplate, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT templates.id, templates.name, templates.description, templates.revision,
		       templates.session_policy, templates.affinity_ttl_seconds, templates.unsupported_policy,
		       templates.created_at, templates.updated_at, COUNT(accounts.id)
		FROM codex_identity_templates AS templates
		LEFT JOIN accounts ON accounts.codex_identity_template_id=templates.id AND accounts.deleted_at IS NULL
		GROUP BY templates.id
		ORDER BY LOWER(templates.name), templates.id
	`)
	if err != nil {
		return nil, err
	}
	items := make([]*service.CodexIdentityTemplate, 0)
	for rows.Next() {
		template, err := scanCodexIdentityTemplate(rows)
		if err != nil {
			_ = rows.Close()
			return nil, err
		}
		items = append(items, template)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for _, template := range items {
		if err := loadCodexIdentityTemplateProfiles(ctx, r.db, template); err != nil {
			return nil, err
		}
	}
	return items, nil
}

func (r *codexIdentityTemplateRepository) GetCodexIdentityTemplate(ctx context.Context, id int64) (*service.CodexIdentityTemplate, error) {
	template, err := getCodexIdentityTemplate(ctx, r.db, id)
	if err != nil {
		return nil, err
	}
	return template, nil
}

func (r *accountRepository) GetCodexIdentityTemplate(ctx context.Context, id int64) (*service.CodexIdentityTemplate, error) {
	rows, err := r.sql.QueryContext(ctx, `
		SELECT templates.id, templates.name, templates.description, templates.revision,
		       templates.session_policy, templates.affinity_ttl_seconds, templates.unsupported_policy,
		       templates.created_at, templates.updated_at,
		       (SELECT COUNT(*) FROM accounts
		        WHERE accounts.codex_identity_template_id=templates.id AND accounts.deleted_at IS NULL)
		FROM codex_identity_templates AS templates WHERE templates.id=$1
	`, id)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, service.ErrCodexIdentityTemplateNotFound
	}
	template, err := scanCodexIdentityTemplate(rows)
	if err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := loadCodexIdentityTemplateProfiles(ctx, r.sql, template); err != nil {
		return nil, err
	}
	return template, nil
}

func (r *accountRepository) GetCodexIdentityTemplateByName(ctx context.Context, name string) (*service.CodexIdentityTemplate, error) {
	rows, err := r.sql.QueryContext(ctx, `
		SELECT templates.id, templates.name, templates.description, templates.revision,
		       templates.session_policy, templates.affinity_ttl_seconds, templates.unsupported_policy,
		       templates.created_at, templates.updated_at,
		       (SELECT COUNT(*) FROM accounts
		        WHERE accounts.codex_identity_template_id=templates.id AND accounts.deleted_at IS NULL)
		FROM codex_identity_templates AS templates
		WHERE LOWER(BTRIM(templates.name))=LOWER(BTRIM($1))
	`, name)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, service.ErrCodexIdentityTemplateNotFound
	}
	template, err := scanCodexIdentityTemplate(rows)
	if err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := loadCodexIdentityTemplateProfiles(ctx, r.sql, template); err != nil {
		return nil, err
	}
	return template, nil
}

func (r *codexIdentityTemplateRepository) CreateCodexIdentityTemplate(ctx context.Context, template *service.CodexIdentityTemplate) (*service.CodexIdentityTemplate, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := validateCodexIdentityTemplateProxyReferences(ctx, tx, template); err != nil {
		return nil, err
	}

	sessionPolicy, err := json.Marshal(template.SessionPolicy)
	if err != nil {
		return nil, err
	}
	err = tx.QueryRowContext(ctx, `
		INSERT INTO codex_identity_templates
			(name, description, session_policy, affinity_ttl_seconds, unsupported_policy)
		VALUES ($1, $2, $3::jsonb, $4, $5)
		RETURNING id
	`, template.Name, template.Description, string(sessionPolicy), template.AffinityTTLSeconds, template.UnsupportedPolicy).Scan(&template.ID)
	if err != nil {
		return nil, translateCodexIdentityTemplateWriteError(err)
	}
	if err := replaceCodexIdentityTemplateProfiles(ctx, tx, template.ID, template.Profiles); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, translateCodexIdentityTemplateWriteError(err)
	}
	return r.GetCodexIdentityTemplate(ctx, template.ID)
}

func (r *codexIdentityTemplateRepository) UpdateCodexIdentityTemplate(
	ctx context.Context,
	template *service.CodexIdentityTemplate,
	expectedRevision int64,
	confirmAssignedAccounts bool,
) (*service.CodexIdentityTemplate, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	var currentRevision int64
	if err := tx.QueryRowContext(ctx, `
		SELECT revision FROM codex_identity_templates WHERE id=$1 FOR UPDATE
	`, template.ID).Scan(&currentRevision); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, service.ErrCodexIdentityTemplateNotFound
		}
		return nil, err
	}
	if currentRevision != expectedRevision {
		return nil, service.ErrCodexIdentityTemplateRevisionConflict.WithMetadata(map[string]string{
			"expected_revision": fmt.Sprintf("%d", expectedRevision),
			"current_revision":  fmt.Sprintf("%d", currentRevision),
		})
	}
	if err := validateCodexIdentityTemplateProxyReferences(ctx, tx, template); err != nil {
		return nil, err
	}
	current, err := getCodexIdentityTemplateForUpdate(ctx, tx, template.ID)
	if err != nil {
		return nil, err
	}

	runtimeChanged := !codexIdentityTemplateRuntimeEqual(current, template)
	if runtimeChanged && current.AssignedAccountCount > 0 && !confirmAssignedAccounts {
		return nil, service.ErrCodexIdentityTemplateUpdateConfirmationRequired.WithMetadata(map[string]string{
			"assigned_account_count": fmt.Sprintf("%d", current.AssignedAccountCount),
		})
	}
	nextRevision := currentRevision
	if runtimeChanged {
		nextRevision++
	}
	template.ID = current.ID
	template.Revision = nextRevision

	sessionPolicy, err := json.Marshal(template.SessionPolicy)
	if err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE codex_identity_templates
		SET name=$1, description=$2, session_policy=$3::jsonb,
		    affinity_ttl_seconds=$4, unsupported_policy=$5,
		    revision=$6, updated_at=NOW()
		WHERE id=$7
	`, template.Name, template.Description, string(sessionPolicy), template.AffinityTTLSeconds,
		template.UnsupportedPolicy, nextRevision, template.ID); err != nil {
		return nil, translateCodexIdentityTemplateWriteError(err)
	}
	var affectedAccountIDs []int64
	if runtimeChanged {
		if err := replaceCodexIdentityTemplateProfiles(ctx, tx, template.ID, template.Profiles); err != nil {
			return nil, err
		}
		affectedAccountIDs, err = propagateCodexIdentityTemplate(ctx, tx, template)
		if err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, translateCodexIdentityTemplateWriteError(err)
	}
	r.syncCodexIdentityTemplateAccounts(ctx, affectedAccountIDs)
	return r.GetCodexIdentityTemplate(ctx, template.ID)
}

func getCodexIdentityTemplateForUpdate(
	ctx context.Context,
	tx *sql.Tx,
	id int64,
) (*service.CodexIdentityTemplate, error) {
	row := tx.QueryRowContext(ctx, `
		SELECT templates.id, templates.name, templates.description, templates.revision,
		       templates.session_policy, templates.affinity_ttl_seconds, templates.unsupported_policy,
		       templates.created_at, templates.updated_at,
		       (SELECT COUNT(*) FROM accounts
		        WHERE accounts.codex_identity_template_id=templates.id AND accounts.deleted_at IS NULL)
		FROM codex_identity_templates AS templates
		WHERE templates.id=$1
		FOR UPDATE
	`, id)
	template, err := scanCodexIdentityTemplate(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, service.ErrCodexIdentityTemplateNotFound
		}
		return nil, err
	}
	if err := loadCodexIdentityTemplateProfiles(ctx, tx, template); err != nil {
		return nil, err
	}
	return template, nil
}

func codexIdentityTemplateRuntimeEqual(left, right *service.CodexIdentityTemplate) bool {
	if left == nil || right == nil ||
		left.SessionPolicy != right.SessionPolicy ||
		left.AffinityTTLSeconds != right.AffinityTTLSeconds ||
		left.UnsupportedPolicy != right.UnsupportedPolicy ||
		len(left.Profiles) != len(right.Profiles) {
		return false
	}
	for index := range left.Profiles {
		leftProfile := left.Profiles[index]
		rightProfile := right.Profiles[index]
		leftProfile.ID = 0
		rightProfile.ID = 0
		for slotIndex := range leftProfile.Slots {
			leftProfile.Slots[slotIndex].ID = 0
		}
		for slotIndex := range rightProfile.Slots {
			rightProfile.Slots[slotIndex].ID = 0
		}
		if !reflect.DeepEqual(leftProfile, rightProfile) {
			return false
		}
	}
	return true
}

func validateCodexIdentityTemplateProxyReferences(
	ctx context.Context,
	tx *sql.Tx,
	template *service.CodexIdentityTemplate,
) error {
	if template == nil {
		return nil
	}
	seen := make(map[int64]struct{})
	for _, profile := range template.Profiles {
		if profile.ProxyMode == service.CodexProxyExplicit && profile.ProxyID != nil {
			seen[*profile.ProxyID] = struct{}{}
		}
		for _, slot := range profile.Slots {
			if slot.ProxyMode == service.CodexProxyExplicit && slot.ProxyID != nil {
				seen[*slot.ProxyID] = struct{}{}
			}
		}
	}
	if len(seen) == 0 {
		return nil
	}
	ids := make([]int64, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT id, status, expires_at
		FROM proxies
		WHERE id=ANY($1) AND deleted_at IS NULL
		ORDER BY id
		FOR SHARE
	`, pq.Array(ids))
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	now := time.Now()
	validated := make(map[int64]struct{}, len(ids))
	for rows.Next() {
		var id int64
		var status string
		var expiresAt sql.NullTime
		if err := rows.Scan(&id, &status, &expiresAt); err != nil {
			return err
		}
		if status != service.StatusActive || expiresAt.Valid && !expiresAt.Time.After(now) {
			return infraerrors.BadRequest(
				"INVALID_CODEX_IDENTITY_TEMPLATE_PROXY",
				fmt.Sprintf("proxy %d is unavailable", id),
			)
		}
		validated[id] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, id := range ids {
		if _, ok := validated[id]; !ok {
			return infraerrors.BadRequest(
				"INVALID_CODEX_IDENTITY_TEMPLATE_PROXY",
				fmt.Sprintf("proxy %d does not exist", id),
			)
		}
	}
	return nil
}

type codexTemplateAssignedAccount struct {
	id       int64
	platform string
	typeName string
	policy   []byte
}

func propagateCodexIdentityTemplate(
	ctx context.Context,
	tx *sql.Tx,
	template *service.CodexIdentityTemplate,
) ([]int64, error) {
	requested, err := service.MaterializeCodexIdentityTemplate(template)
	if err != nil {
		return nil, err
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT id, platform, type, codex_identity_policy
		FROM accounts
		WHERE codex_identity_template_id=$1 AND deleted_at IS NULL
		ORDER BY id
		FOR UPDATE
	`, template.ID)
	if err != nil {
		return nil, err
	}
	assigned := make([]codexTemplateAssignedAccount, 0)
	for rows.Next() {
		var account codexTemplateAssignedAccount
		if err := rows.Scan(&account.id, &account.platform, &account.typeName, &account.policy); err != nil {
			_ = rows.Close()
			return nil, err
		}
		assigned = append(assigned, account)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	affected := make([]int64, 0, len(assigned))
	for _, account := range assigned {
		if account.platform != service.PlatformOpenAI || account.typeName != service.AccountTypeOAuth {
			return nil, infraerrors.Conflict(
				"CODEX_IDENTITY_TEMPLATE_ACCOUNT_INVALID",
				"a Codex identity template is assigned to an unsupported account",
			)
		}
		var raw map[string]any
		if err := json.Unmarshal(account.policy, &raw); err != nil {
			return nil, fmt.Errorf("decode account %d Codex identity policy: %w", account.id, err)
		}
		existing, err := service.DecodeCodexIdentityPolicy(raw, account.platform, account.typeName)
		if err != nil {
			return nil, fmt.Errorf("decode account %d Codex identity policy: %w", account.id, err)
		}
		next, changed, err := service.PrepareCodexIdentityPolicyForAccountTransition(
			existing,
			account.platform,
			account.typeName,
			requested,
			account.platform,
			account.typeName,
		)
		if err != nil {
			return nil, fmt.Errorf("apply Codex identity template to account %d: %w", account.id, err)
		}
		if changed {
			if err := transitionProvisionedCodexIdentity(ctx, tx, account.id, existing, next); err != nil {
				return nil, fmt.Errorf("transition account %d Codex identity projection: %w", account.id, err)
			}
		}
		encoded, err := service.EncodeCodexIdentityPolicy(next)
		if err != nil {
			return nil, err
		}
		policyJSON, err := json.Marshal(encoded)
		if err != nil {
			return nil, err
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE accounts
			SET codex_identity_policy=$1::jsonb,
			    codex_identity_template_applied_revision=$2,
			    updated_at=NOW()
			WHERE id=$3 AND codex_identity_template_id=$4
		`, string(policyJSON), template.Revision, account.id, template.ID); err != nil {
			return nil, err
		}
		if changed {
			groupIDs, err := loadCodexIdentityTemplateAccountGroups(ctx, tx, account.id)
			if err != nil {
				return nil, err
			}
			if err := enqueueSchedulerOutbox(
				ctx,
				tx,
				service.SchedulerOutboxEventAccountChanged,
				&account.id,
				nil,
				buildSchedulerGroupPayload(groupIDs),
			); err != nil {
				return nil, err
			}
		}
		affected = append(affected, account.id)
	}
	return affected, nil
}

func loadCodexIdentityTemplateAccountGroups(ctx context.Context, tx *sql.Tx, accountID int64) ([]int64, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT group_id FROM account_groups WHERE account_id=$1 ORDER BY group_id
	`, accountID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	groupIDs := make([]int64, 0)
	for rows.Next() {
		var groupID int64
		if err := rows.Scan(&groupID); err != nil {
			return nil, err
		}
		groupIDs = append(groupIDs, groupID)
	}
	return groupIDs, rows.Err()
}

func (r *codexIdentityTemplateRepository) syncCodexIdentityTemplateAccounts(ctx context.Context, accountIDs []int64) {
	if r == nil || r.accountRepo == nil || r.schedulerCache == nil || len(accountIDs) == 0 {
		return
	}
	base := context.Background()
	if ctx != nil {
		base = context.WithoutCancel(ctx)
	}
	syncCtx, cancel := context.WithTimeout(base, 5*time.Second)
	defer cancel()
	accounts, err := r.accountRepo.GetByIDs(syncCtx, accountIDs)
	if err != nil {
		return
	}
	for _, account := range accounts {
		if account != nil {
			_ = r.schedulerCache.SetAccount(syncCtx, account)
		}
	}
}

func (r *codexIdentityTemplateRepository) DeleteCodexIdentityTemplate(ctx context.Context, id int64) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var assigned int64
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM accounts
		WHERE codex_identity_template_id=$1
	`, id).Scan(&assigned); err != nil {
		return err
	}
	if assigned > 0 {
		return service.ErrCodexIdentityTemplateInUse.WithMetadata(map[string]string{
			"assigned_account_count": fmt.Sprintf("%d", assigned),
		})
	}
	result, err := tx.ExecContext(ctx, `DELETE FROM codex_identity_templates WHERE id=$1`, id)
	if err != nil {
		return translateCodexIdentityTemplateWriteError(err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return service.ErrCodexIdentityTemplateNotFound
	}
	return tx.Commit()
}

type codexIdentityTemplateQuerier interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

type codexIdentityTemplateRowsQuerier interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

type codexIdentityTemplateScanner interface {
	Scan(...any) error
}

func getCodexIdentityTemplate(ctx context.Context, query codexIdentityTemplateQuerier, id int64) (*service.CodexIdentityTemplate, error) {
	row := query.QueryRowContext(ctx, `
		SELECT templates.id, templates.name, templates.description, templates.revision,
		       templates.session_policy, templates.affinity_ttl_seconds, templates.unsupported_policy,
		       templates.created_at, templates.updated_at,
		       (SELECT COUNT(*) FROM accounts
		        WHERE accounts.codex_identity_template_id=templates.id AND accounts.deleted_at IS NULL)
		FROM codex_identity_templates AS templates
		WHERE templates.id=$1
	`, id)
	template, err := scanCodexIdentityTemplate(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, service.ErrCodexIdentityTemplateNotFound
		}
		return nil, err
	}
	if err := loadCodexIdentityTemplateProfiles(ctx, query, template); err != nil {
		return nil, err
	}
	return template, nil
}

func scanCodexIdentityTemplate(scanner codexIdentityTemplateScanner) (*service.CodexIdentityTemplate, error) {
	template := &service.CodexIdentityTemplate{}
	var sessionPolicy []byte
	if err := scanner.Scan(
		&template.ID, &template.Name, &template.Description, &template.Revision,
		&sessionPolicy, &template.AffinityTTLSeconds, &template.UnsupportedPolicy,
		&template.CreatedAt, &template.UpdatedAt, &template.AssignedAccountCount,
	); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(sessionPolicy, &template.SessionPolicy); err != nil {
		return nil, fmt.Errorf("decode Codex identity template session policy: %w", err)
	}
	return template, nil
}

func loadCodexIdentityTemplateProfiles(ctx context.Context, query codexIdentityTemplateRowsQuerier, template *service.CodexIdentityTemplate) error {
	rows, err := query.QueryContext(ctx, `
		SELECT id, os_class, canonical_surface, COALESCE(architecture, ''),
		       proxy_mode, proxy_id, slot_count, catalog_version
		FROM codex_identity_template_profiles
		WHERE template_id=$1
		ORDER BY os_class, canonical_surface, id
	`, template.ID)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	profiles := make([]service.CodexIdentityTemplateProfile, 0)
	for rows.Next() {
		profile := service.CodexIdentityTemplateProfile{}
		var proxyID sql.NullInt64
		if err := rows.Scan(
			&profile.ID, &profile.OSClass, &profile.CanonicalSurface, &profile.Architecture,
			&profile.ProxyMode, &proxyID, &profile.SlotCount, &profile.CatalogVersion,
		); err != nil {
			return err
		}
		if proxyID.Valid {
			id := proxyID.Int64
			profile.ProxyID = &id
		}
		profiles = append(profiles, profile)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for index := range profiles {
		if err := loadCodexIdentityTemplateSlots(ctx, query, template.ID, &profiles[index]); err != nil {
			return err
		}
	}
	template.Profiles = profiles
	return nil
}

func loadCodexIdentityTemplateSlots(
	ctx context.Context,
	query codexIdentityTemplateRowsQuerier,
	templateID int64,
	profile *service.CodexIdentityTemplateProfile,
) error {
	rows, err := query.QueryContext(ctx, `
		SELECT id, slot_index, proxy_mode, proxy_id
		FROM codex_identity_template_slots
		WHERE template_id=$1 AND profile_id=$2
		ORDER BY slot_index
	`, templateID, profile.ID)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	profile.Slots = make([]service.CodexIdentityTemplateSlot, 0)
	for rows.Next() {
		slot := service.CodexIdentityTemplateSlot{}
		var proxyID sql.NullInt64
		if err := rows.Scan(&slot.ID, &slot.Index, &slot.ProxyMode, &proxyID); err != nil {
			return err
		}
		if proxyID.Valid {
			id := proxyID.Int64
			slot.ProxyID = &id
		}
		profile.Slots = append(profile.Slots, slot)
	}
	return rows.Err()
}

func replaceCodexIdentityTemplateProfiles(
	ctx context.Context,
	tx *sql.Tx,
	templateID int64,
	profiles []service.CodexIdentityTemplateProfile,
) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM codex_identity_template_profiles WHERE template_id=$1`, templateID); err != nil {
		return err
	}
	for _, profile := range profiles {
		var profileID int64
		if err := tx.QueryRowContext(ctx, `
			INSERT INTO codex_identity_template_profiles
				(template_id, os_class, canonical_surface, architecture, proxy_mode, proxy_id, slot_count, catalog_version)
			VALUES ($1, $2, $3, NULLIF($4, ''), $5, $6, $7, $8)
			RETURNING id
		`, templateID, profile.OSClass, profile.CanonicalSurface, profile.Architecture,
			profile.ProxyMode, profile.ProxyID, profile.SlotCount, profile.CatalogVersion).Scan(&profileID); err != nil {
			return translateCodexIdentityTemplateWriteError(err)
		}
		for _, slot := range profile.Slots {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO codex_identity_template_slots
					(template_id, profile_id, slot_index, proxy_mode, proxy_id)
				VALUES ($1, $2, $3, $4, $5)
			`, templateID, profileID, slot.Index, slot.ProxyMode, slot.ProxyID); err != nil {
				return translateCodexIdentityTemplateWriteError(err)
			}
		}
	}
	return nil
}

func translateCodexIdentityTemplateWriteError(err error) error {
	if err == nil {
		return nil
	}
	var pqErr *pq.Error
	if !errors.As(err, &pqErr) {
		return err
	}
	switch string(pqErr.Code) {
	case "23505":
		if pqErr.Constraint == "idx_codex_identity_templates_name_ci" {
			return service.ErrCodexIdentityTemplateNameExists.WithCause(err)
		}
		return infraerrors.Conflict("CODEX_IDENTITY_TEMPLATE_CONFLICT", "Codex identity template conflicts with existing data").WithCause(err)
	case "23503":
		if pqErr.Constraint == "accounts_codex_identity_template_fk" {
			return service.ErrCodexIdentityTemplateInUse.WithCause(err)
		}
		return infraerrors.BadRequest("INVALID_CODEX_IDENTITY_TEMPLATE_PROXY", "a referenced proxy does not exist").WithCause(err)
	case "23514":
		return infraerrors.BadRequest("INVALID_CODEX_IDENTITY_TEMPLATE", "Codex identity template violates a database constraint").WithCause(err)
	default:
		return err
	}
}
