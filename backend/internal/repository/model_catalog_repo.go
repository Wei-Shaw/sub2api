package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

var ErrCatalogAliasConflict = errors.New("catalog alias targets another model")

// modelCatalogRepository is intentionally a background-plane repository. No
// request service receives this type; request code receives only
// service.ModelCatalogReader.
type modelCatalogRepository struct {
	db *sql.DB
}

func NewModelCatalogRepository(db *sql.DB) service.ModelCatalogRepository {
	return &modelCatalogRepository{db: db}
}

func NewModelCatalogAdminRepository(db *sql.DB) service.ModelCatalogAdminRepository {
	return &modelCatalogRepository{db: db}
}

func (r *modelCatalogRepository) NextRevision(ctx context.Context, _ string) (int64, error) {
	if r == nil || r.db == nil {
		return 0, errors.New("catalog repository database is nil")
	}
	var revision int64
	err := r.db.QueryRowContext(ctx, `
		SELECT COALESCE(MAX(revision), 0) + 1
		FROM catalog_revisions
	`).Scan(&revision)
	if err != nil {
		return 0, fmt.Errorf("next catalog revision: %w", err)
	}
	return revision, nil
}

func (r *modelCatalogRepository) ValidateRevision(ctx context.Context, catalogRevisionID int64) error {
	if r == nil || r.db == nil {
		return errors.New("catalog repository database is nil")
	}
	if catalogRevisionID <= 0 {
		return service.ErrCatalogRevisionNotValidated
	}
	result, err := r.db.ExecContext(ctx, `
		UPDATE catalog_revisions
		SET state = 'validated', validated_at = NOW()
		WHERE id = $1 AND state = 'staged'
	`, catalogRevisionID)
	if err != nil {
		return fmt.Errorf("validate catalog revision %d: %w", catalogRevisionID, err)
	}
	rows, err := result.RowsAffected()
	if err == nil && rows == 1 {
		return nil
	}
	var state string
	if queryErr := r.db.QueryRowContext(ctx, `SELECT state FROM catalog_revisions WHERE id = $1`, catalogRevisionID).Scan(&state); queryErr == nil && (state == "validated" || state == "published") {
		return nil
	}
	if err != nil {
		return fmt.Errorf("validate catalog revision %d: rows affected: %w", catalogRevisionID, err)
	}
	if rows != 1 {
		return service.ErrCatalogRevisionNotValidated
	}
	return nil
}

func (r *modelCatalogRepository) StageRevision(ctx context.Context, stage service.CatalogRevisionStage) (revisionID int64, err error) {
	if r == nil || r.db == nil {
		return 0, errors.New("nil catalog repository database")
	}
	if err := validateCatalogRevisionStage(stage); err != nil {
		return 0, err
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin catalog revision stage: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext('model-catalog-sync')::bigint)`); err != nil {
		_ = tx.Rollback()
		return 0, fmt.Errorf("acquire catalog sync lease: %w", err)
	}
	syncRunID, err := createSyncRunTx(ctx, tx, stage.SyncRun)
	if err != nil {
		_ = tx.Rollback()
		return 0, fmt.Errorf("create catalog sync run: %w", err)
	}
	markFailed := func(cause error) {
		_, _ = r.db.ExecContext(ctx, `
			UPDATE catalog_sync_runs
			SET status = 'failed', completed_at = NOW(), validation_errors = $2::jsonb
			WHERE id = $1
		`, syncRunID, mustCatalogErrorJSON(cause))
	}

	rollback := func(cause error) (int64, error) {
		_ = tx.Rollback()
		markFailed(cause)
		return 0, cause
	}

	var stagedID sql.NullInt64
	err = tx.QueryRowContext(ctx, `
		INSERT INTO catalog_revisions
			(revision, sync_run_id, normalized_hash, normalizer_version, state, model_count)
		VALUES ($1, $2, $3, $4, 'staged', $5)
		ON CONFLICT (normalized_hash) DO NOTHING
		RETURNING id
	`, stage.Revision, syncRunID, strings.TrimSpace(stage.NormalizedHash), strings.TrimSpace(stage.Normalizer), len(stage.Models)).Scan(&stagedID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return rollback(fmt.Errorf("insert catalog revision: %w", err))
	}
	if !stagedID.Valid {
		err = tx.QueryRowContext(ctx, `SELECT id FROM catalog_revisions WHERE normalized_hash = $1`, stage.NormalizedHash).Scan(&revisionID)
		if err != nil {
			return rollback(fmt.Errorf("load existing catalog revision: %w", err))
		}
		if err := tx.Commit(); err != nil {
			markFailed(err)
			return 0, fmt.Errorf("commit duplicate catalog revision stage: %w", err)
		}
		if err := r.markSyncRunStaged(ctx, syncRunID, stage); err != nil {
			return 0, err
		}
		return revisionID, nil
	}
	revisionID = stagedID.Int64

	for _, candidate := range stage.Models {
		modelID, policy, err := upsertCatalogModel(ctx, tx, candidate)
		if err != nil {
			return rollback(fmt.Errorf("upsert catalog model %s: %w", candidate.CanonicalKey, err))
		}
		// Operator policy is owned by the control-plane row. Every staged
		// revision gets a copy so its hash and later reload are self-contained.
		candidate.OperatorState = policy.State
		candidate.OperatorReason = policy.Reason
		candidate.OperatorVersion = policy.OperatorVersion
		if err := insertCatalogModelRevision(ctx, tx, revisionID, modelID, candidate); err != nil {
			return rollback(fmt.Errorf("insert catalog model revision %s: %w", candidate.CanonicalKey, err))
		}
		for _, alias := range candidate.Aliases {
			if err := upsertCatalogAlias(ctx, tx, revisionID, modelID, alias, candidate); err != nil {
				return rollback(fmt.Errorf("upsert catalog alias %s: %w", alias.Alias, err))
			}
		}
	}

	// Preserve stable catalog identity across source removals. A source refresh
	// is a complete publication, so models absent from this candidate remain in
	// the revision as source-missing rows instead of disappearing from admin
	// history or being able to reappear as a new enabled identity later.
	missingResult, err := tx.ExecContext(ctx, `
		INSERT INTO catalog_model_revisions
			(catalog_revision_id, model_id, source_state, provider, platform, mode,
			 capabilities, context_window, max_output_tokens, pricing_schema_version,
			 pricing_json, pricing_valid, operator_state, operator_reason, operator_version,
			 pricing_source, source_metadata, source_hash)
		SELECT $1::bigint, cm.id, 'missing', '', '', '', '{}'::jsonb, NULL, NULL, 1,
		       NULL, FALSE, cm.operator_state, cm.operator_reason, cm.operator_version,
		       NULL, '{}'::jsonb,
		       'missing:' || cm.id::text || ':' || $1::bigint::text
		FROM catalog_models cm
		WHERE NOT EXISTS (
			SELECT 1 FROM catalog_model_revisions cmr
			WHERE cmr.catalog_revision_id = $1::bigint AND cmr.model_id = cm.id
		)
	`, revisionID)
	if err != nil {
		return rollback(fmt.Errorf("stage source-missing catalog models: %w", err))
	}
	missingCount, err := missingResult.RowsAffected()
	if err != nil {
		return rollback(fmt.Errorf("count source-missing catalog models: %w", err))
	}
	if missingCount > 0 {
		if _, err := tx.ExecContext(ctx, `
			UPDATE catalog_revisions
			SET model_count = model_count + $2
			WHERE id = $1
		`, revisionID, missingCount); err != nil {
			return rollback(fmt.Errorf("update catalog revision model count: %w", err))
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE catalog_sync_runs
			SET missing_count = missing_count + $2,
			    normalized_count = normalized_count + $2
			WHERE id = $1
		`, syncRunID, missingCount); err != nil {
			return rollback(fmt.Errorf("update catalog sync missing count: %w", err))
		}
	}

	if err := tx.Commit(); err != nil {
		markFailed(err)
		return 0, fmt.Errorf("commit catalog revision stage: %w", err)
	}
	if err := r.markSyncRunStaged(ctx, syncRunID, stage); err != nil {
		return 0, err
	}
	return revisionID, nil
}

func createSyncRunTx(ctx context.Context, tx *sql.Tx, spec service.CatalogSyncRunSpec) (int64, error) {
	var id int64
	err := tx.QueryRowContext(ctx, `
		INSERT INTO catalog_sync_runs
			(source_set, trigger, actor_user_id, upstream_version, upstream_etag, upstream_hash,
			 normalizer_version, status, source_count, normalized_count, added_count, changed_count,
			 missing_count, invalid_count)
		VALUES ($1, $2, $3, $4, $5, $6, $7, 'running', $8, $9, $10, $11, $12, $13)
		RETURNING id
	`,
		spec.SourceSet,
		spec.Trigger,
		spec.ActorUserID,
		nullableCatalogString(spec.UpstreamVersion),
		nullableCatalogString(spec.UpstreamETag),
		nullableCatalogString(spec.UpstreamHash),
		spec.Normalizer,
		spec.SourceCount,
		spec.NormalizedCount,
		spec.AddedCount,
		spec.ChangedCount,
		spec.MissingCount,
		spec.InvalidCount,
	).Scan(&id)
	return id, err
}

func (r *modelCatalogRepository) markSyncRunStaged(ctx context.Context, syncRunID int64, stage service.CatalogRevisionStage) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE catalog_sync_runs
		SET status = 'staged', normalized_hash = $2, completed_at = NOW()
		WHERE id = $1
	`, syncRunID, stage.NormalizedHash)
	if err != nil {
		return fmt.Errorf("mark catalog sync run staged: %w", err)
	}
	return nil
}

type catalogOperatorPolicy struct {
	State           service.CatalogOperatorState
	Reason          string
	OperatorVersion int64
}

func upsertCatalogModel(ctx context.Context, tx *sql.Tx, candidate service.CatalogSnapshotModelSpec) (int64, catalogOperatorPolicy, error) {
	var (
		modelID int64
		policy  catalogOperatorPolicy
	)
	err := tx.QueryRowContext(ctx, `
		INSERT INTO catalog_models (canonical_key, canonical_key_normalized)
		VALUES ($1, $2)
		ON CONFLICT (canonical_key_normalized)
		DO UPDATE SET canonical_key = EXCLUDED.canonical_key, updated_at = NOW()
		RETURNING id, operator_state, COALESCE(operator_reason, ''), operator_version
	`, strings.TrimSpace(candidate.CanonicalKey), normalizeCatalogValue(candidate.CanonicalKey)).Scan(
		&modelID, &policy.State, &policy.Reason, &policy.OperatorVersion,
	)
	policy.State = service.CatalogOperatorState(strings.TrimSpace(string(policy.State)))
	return modelID, policy, err
}

func insertCatalogModelRevision(ctx context.Context, tx *sql.Tx, revisionID, modelID int64, candidate service.CatalogSnapshotModelSpec) error {
	pricingJSON, err := marshalOptionalCatalogJSON(candidate.PricingValid, candidate.Pricing)
	if err != nil {
		return err
	}
	capabilitiesJSON := normalizeCatalogJSON(candidate.Capabilities, []byte(`{}`))
	metadataJSON := normalizeCatalogJSON(candidate.SourceMetadata, []byte(`{}`))
	_, err = tx.ExecContext(ctx, `
		INSERT INTO catalog_model_revisions
			(catalog_revision_id, model_id, source_state, provider, platform, mode, capabilities,
			 context_window, max_output_tokens, pricing_schema_version, pricing_json, pricing_valid,
			 operator_state, operator_reason, operator_version, pricing_source, source_metadata, source_hash)
		VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, $8, $9, $10, $11::jsonb, $12, $13, $14, $15, $16, $17::jsonb, $18)
	`,
		revisionID,
		modelID,
		string(candidate.SourceState),
		strings.TrimSpace(candidate.Provider),
		normalizeCatalogValue(candidate.Platform),
		normalizeCatalogValue(candidate.Mode),
		string(capabilitiesJSON),
		nullableCatalogInt64(candidate.ContextWindow),
		nullableCatalogInt64(candidate.MaxOutputTokens),
		positiveCatalogPricingSchema(candidate.PricingSchemaVersion),
		optionalCatalogJSONArg(pricingJSON),
		candidate.PricingValid,
		candidate.OperatorState,
		nullableCatalogString(candidate.OperatorReason),
		positiveCatalogOperatorVersion(candidate.OperatorVersion),
		nullableCatalogString(candidate.PricingSource),
		string(metadataJSON),
		strings.TrimSpace(candidate.SourceHash),
	)
	return err
}

func upsertCatalogAlias(ctx context.Context, tx *sql.Tx, revisionID, modelID int64, alias service.CatalogAliasSpec, candidate service.CatalogSnapshotModelSpec) error {
	aliasValue := normalizeCatalogValue(alias.Alias)
	platformScope := normalizeCatalogPlatformScope(alias.PlatformScope)
	source := strings.TrimSpace(candidate.PricingSource)
	if source == "" {
		source = "catalog-import"
	}
	_, err := tx.ExecContext(ctx, `
		INSERT INTO catalog_model_aliases
			(alias_normalized, platform_scope, model_id, source, state, introduced_revision_id)
		VALUES ($1, $2, $3, $4, 'active', $5)
		ON CONFLICT (platform_scope, alias_normalized)
		DO UPDATE SET
			state = 'active',
			introduced_revision_id = EXCLUDED.introduced_revision_id,
			retired_revision_id = NULL,
			updated_at = NOW()
		WHERE catalog_model_aliases.model_id = EXCLUDED.model_id
	`, aliasValue, platformScope, modelID, source, revisionID)
	if err != nil {
		return err
	}
	var existingModelID int64
	err = tx.QueryRowContext(ctx, `
		SELECT model_id
		FROM catalog_model_aliases
		WHERE platform_scope = $1 AND alias_normalized = $2
	`, platformScope, aliasValue).Scan(&existingModelID)
	if err != nil {
		return err
	}
	if existingModelID != modelID {
		return fmt.Errorf("%w: %s/%s existing=%d candidate=%d", ErrCatalogAliasConflict, platformScope, aliasValue, existingModelID, modelID)
	}
	return nil
}

func (r *modelCatalogRepository) PublishRevision(ctx context.Context, request service.CatalogPublishRequest) (service.CatalogPublicationRecord, error) {
	if r == nil || r.db == nil {
		return service.CatalogPublicationRecord{}, errors.New("nil catalog repository database")
	}
	if request.CatalogRevisionID <= 0 || strings.TrimSpace(request.Scope) == "" {
		return service.CatalogPublicationRecord{}, service.ErrCatalogInvalidSnapshot
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return service.CatalogPublicationRecord{}, err
	}
	rollback := func(cause error) (service.CatalogPublicationRecord, error) {
		_ = tx.Rollback()
		return service.CatalogPublicationRecord{}, cause
	}

	var revisionState string
	if err := tx.QueryRowContext(ctx, `SELECT state FROM catalog_revisions WHERE id = $1 FOR SHARE`, request.CatalogRevisionID).Scan(&revisionState); err != nil {
		return rollback(fmt.Errorf("load catalog revision for publication: %w", err))
	}
	if revisionState != "validated" && revisionState != "published" {
		return rollback(fmt.Errorf("%w: state=%s", service.ErrCatalogRevisionNotValidated, revisionState))
	}

	var (
		currentRevisionID int64
		currentEpoch      int64
		updatedAt         time.Time
	)
	rowErr := tx.QueryRowContext(ctx, `
		SELECT active_revision_id, epoch
		FROM catalog_publications
		WHERE scope = $1
		FOR UPDATE
	`, strings.TrimSpace(request.Scope)).Scan(&currentRevisionID, &currentEpoch)
	nextEpoch := int64(1)
	if errors.Is(rowErr, sql.ErrNoRows) {
		if request.ExpectedEpoch != 0 || request.ExpectedRevisionID != 0 {
			return rollback(fmt.Errorf("%w: publication does not exist", service.ErrCatalogPublicationConflict))
		}
		if err := tx.QueryRowContext(ctx, `
			INSERT INTO catalog_publications (scope, active_revision_id, epoch)
			VALUES ($1, $2, $3)
			RETURNING updated_at
		`, strings.TrimSpace(request.Scope), request.CatalogRevisionID, nextEpoch).Scan(&updatedAt); err != nil {
			return rollback(fmt.Errorf("create catalog publication: %w", err))
		}
	} else if rowErr != nil {
		return rollback(fmt.Errorf("lock catalog publication: %w", rowErr))
	} else {
		if request.ExpectedEpoch != currentEpoch || request.ExpectedRevisionID != currentRevisionID {
			return rollback(fmt.Errorf("%w: expected=(%d,%d) actual=(%d,%d)", service.ErrCatalogPublicationConflict, request.ExpectedEpoch, request.ExpectedRevisionID, currentEpoch, currentRevisionID))
		}
		nextEpoch = currentEpoch + 1
		if err := tx.QueryRowContext(ctx, `
			UPDATE catalog_publications
			SET active_revision_id = $2, epoch = $3, updated_at = NOW()
			WHERE scope = $1
			RETURNING updated_at
		`, strings.TrimSpace(request.Scope), request.CatalogRevisionID, nextEpoch).Scan(&updatedAt); err != nil {
			return rollback(fmt.Errorf("advance catalog publication: %w", err))
		}
	}

	var beforeState []byte
	if rowErr == nil {
		beforeState, err = json.Marshal(map[string]any{
			"scope":               strings.TrimSpace(request.Scope),
			"publication_epoch":   currentEpoch,
			"catalog_revision_id": currentRevisionID,
		})
		if err != nil {
			return rollback(fmt.Errorf("marshal catalog publication before state: %w", err))
		}
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE catalog_revisions
		SET state = 'published', published_at = COALESCE(published_at, NOW())
		WHERE id = $1
	`, request.CatalogRevisionID); err != nil {
		return rollback(fmt.Errorf("mark catalog revision published: %w", err))
	}

	payload, err := json.Marshal(map[string]any{
		"scope":               strings.TrimSpace(request.Scope),
		"publication_epoch":   nextEpoch,
		"catalog_revision_id": request.CatalogRevisionID,
	})
	if err != nil {
		return rollback(fmt.Errorf("marshal catalog publication payload: %w", err))
	}
	dedupKey := fmt.Sprintf("%s:%d:%d", strings.TrimSpace(request.Scope), nextEpoch, request.CatalogRevisionID)
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO catalog_outbox
			(event_type, scope, publication_epoch, catalog_revision_id, payload, dedup_key)
		VALUES ($1, $2, $3, $4, $5::jsonb, $6)
		ON CONFLICT (dedup_key) WHERE dedup_key IS NOT NULL DO NOTHING
	`, "catalog.publication.published", strings.TrimSpace(request.Scope), nextEpoch, request.CatalogRevisionID, string(payload), dedupKey); err != nil {
		return rollback(fmt.Errorf("write catalog outbox: %w", err))
	}

	afterState, err := json.Marshal(map[string]any{
		"scope":               strings.TrimSpace(request.Scope),
		"publication_epoch":   nextEpoch,
		"catalog_revision_id": request.CatalogRevisionID,
	})
	if err != nil {
		return rollback(fmt.Errorf("marshal catalog audit state: %w", err))
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO catalog_lifecycle_audits
			(catalog_revision_id, action, actor_type, actor_user_id, reason, before_state, after_state, request_id, correlation_id)
		VALUES ($1, 'publish', $2, $3, $4, $5::jsonb, $6::jsonb, $7, $8)
	`, request.CatalogRevisionID, nullableCatalogString(request.ActorType), request.ActorUserID,
		nullableCatalogString(request.Reason), nullableCatalogJSON(beforeState), string(afterState), nullableCatalogString(request.RequestID), nullableCatalogString(request.CorrelationID)); err != nil {
		return rollback(fmt.Errorf("write catalog lifecycle audit: %w", err))
	}
	if err := tx.Commit(); err != nil {
		return service.CatalogPublicationRecord{}, fmt.Errorf("commit catalog publication: %w", err)
	}
	return service.CatalogPublicationRecord{
		Scope:             strings.TrimSpace(request.Scope),
		CatalogRevisionID: request.CatalogRevisionID,
		Epoch:             nextEpoch,
		UpdatedAt:         updatedAt,
	}, nil
}

func (r *modelCatalogRepository) ListOperatorStates(ctx context.Context, modelIDs []int64) (map[int64]service.CatalogOperatorStateRecord, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("nil catalog repository database")
	}
	states := make(map[int64]service.CatalogOperatorStateRecord, len(modelIDs))
	if len(modelIDs) == 0 {
		return states, nil
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, operator_state, COALESCE(operator_reason, ''), operator_version
		FROM catalog_models
		WHERE id = ANY($1)
	`, pq.Array(modelIDs))
	if err != nil {
		return nil, fmt.Errorf("list catalog operator states: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var record service.CatalogOperatorStateRecord
		if err := rows.Scan(&record.ModelID, &record.State, &record.Reason, &record.OperatorVersion); err != nil {
			return nil, fmt.Errorf("scan catalog operator state: %w", err)
		}
		record.State = service.CatalogOperatorState(strings.TrimSpace(string(record.State)))
		states[record.ModelID] = record
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate catalog operator states: %w", err)
	}
	return states, nil
}

func (r *modelCatalogRepository) UpdateOperatorState(ctx context.Context, request service.CatalogOperatorStateUpdateRequest) (service.CatalogOperatorStateRecord, error) {
	if r == nil || r.db == nil {
		return service.CatalogOperatorStateRecord{}, errors.New("nil catalog repository database")
	}
	if request.ModelID <= 0 || request.ExpectedVersion <= 0 {
		return service.CatalogOperatorStateRecord{}, service.ErrCatalogOperatorConflict
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return service.CatalogOperatorStateRecord{}, fmt.Errorf("begin catalog operator update: %w", err)
	}
	rollback := func(cause error) (service.CatalogOperatorStateRecord, error) {
		_ = tx.Rollback()
		return service.CatalogOperatorStateRecord{}, cause
	}

	var before service.CatalogOperatorStateRecord
	err = tx.QueryRowContext(ctx, `
		SELECT id, operator_state, COALESCE(operator_reason, ''), operator_version
		FROM catalog_models
		WHERE id = $1
		FOR UPDATE
	`, request.ModelID).Scan(&before.ModelID, &before.State, &before.Reason, &before.OperatorVersion)
	if errors.Is(err, sql.ErrNoRows) {
		return rollback(fmt.Errorf("%w: id=%d", service.ErrCatalogModelNotFound, request.ModelID))
	}
	if err != nil {
		return rollback(fmt.Errorf("load catalog operator state: %w", err))
	}
	if before.OperatorVersion != request.ExpectedVersion {
		return rollback(fmt.Errorf("%w: expected=%d actual=%d", service.ErrCatalogOperatorConflict, request.ExpectedVersion, before.OperatorVersion))
	}

	reason := strings.TrimSpace(request.Reason)
	var after service.CatalogOperatorStateRecord
	err = tx.QueryRowContext(ctx, `
		UPDATE catalog_models
		SET operator_state = $2,
		    operator_reason = NULLIF($3, ''),
		    operator_version = operator_version + 1,
		    last_operator_change_at = NOW(),
		    retired_at = CASE WHEN $2 = 'retired' THEN COALESCE(retired_at, NOW()) ELSE NULL END,
		    updated_at = NOW()
		WHERE id = $1
	RETURNING id, operator_state, COALESCE(operator_reason, ''), operator_version
	`, request.ModelID, string(request.State), reason).Scan(&after.ModelID, &after.State, &after.Reason, &after.OperatorVersion)
	if err != nil {
		return rollback(fmt.Errorf("update catalog operator state: %w", err))
	}

	beforeJSON, err := json.Marshal(map[string]any{
		"operator_state":   before.State,
		"operator_reason":  before.Reason,
		"operator_version": before.OperatorVersion,
	})
	if err != nil {
		return rollback(fmt.Errorf("marshal catalog operator before state: %w", err))
	}
	afterJSON, err := json.Marshal(map[string]any{
		"operator_state":   after.State,
		"operator_reason":  after.Reason,
		"operator_version": after.OperatorVersion,
	})
	if err != nil {
		return rollback(fmt.Errorf("marshal catalog operator after state: %w", err))
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO catalog_lifecycle_audits
			(model_id, action, actor_type, actor_user_id, reason, before_state, after_state, request_id, correlation_id)
		VALUES ($1, 'operator-state-update', 'admin', $2, $3, $4::jsonb, $5::jsonb, $6, $7)
	`, request.ModelID, nullableCatalogInt64(request.ActorUserID), nullableCatalogString(reason), string(beforeJSON), string(afterJSON), nullableCatalogString(request.RequestID), nullableCatalogString(request.CorrelationID)); err != nil {
		return rollback(fmt.Errorf("write catalog operator audit: %w", err))
	}
	if err := tx.Commit(); err != nil {
		return service.CatalogOperatorStateRecord{}, fmt.Errorf("commit catalog operator update: %w", err)
	}
	after.State = service.CatalogOperatorState(strings.TrimSpace(string(after.State)))
	return after, nil
}

func (r *modelCatalogRepository) LoadActiveSnapshot(ctx context.Context, scope string) (service.CatalogSnapshotSpec, error) {
	if r == nil || r.db == nil {
		return service.CatalogSnapshotSpec{}, errors.New("nil catalog repository database")
	}
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return service.CatalogSnapshotSpec{}, err
	}
	rollback := func(cause error) (service.CatalogSnapshotSpec, error) {
		_ = tx.Rollback()
		return service.CatalogSnapshotSpec{}, cause
	}

	var spec service.CatalogSnapshotSpec
	var publishedAt, verifiedAt time.Time
	err = tx.QueryRowContext(ctx, `
		SELECT p.epoch, r.id, r.revision, r.normalized_hash,
		       COALESCE(r.published_at, r.created_at), COALESCE(r.validated_at, r.created_at)
		FROM catalog_publications p
		JOIN catalog_revisions r ON r.id = p.active_revision_id
		WHERE p.scope = $1
	`, strings.TrimSpace(scope)).Scan(&spec.Epoch, &spec.RevisionID, &spec.Revision, &spec.Checksum, &publishedAt, &verifiedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return rollback(service.ErrCatalogNoPublication)
	}
	if err != nil {
		return rollback(fmt.Errorf("load active catalog publication: %w", err))
	}
	spec.Scope = strings.TrimSpace(scope)
	spec.PublishedAt = publishedAt
	spec.VerifiedAt = verifiedAt

	rows, err := tx.QueryContext(ctx, `
		SELECT cmr.id, cmr.model_id, cm.canonical_key, cmr.operator_state,
		       COALESCE(cmr.operator_reason, ''), cmr.operator_version,
		       cmr.source_state, cmr.provider, cmr.platform, cmr.mode,
		       cmr.capabilities, cmr.context_window, cmr.max_output_tokens,
		       cmr.pricing_schema_version, cmr.pricing_json, cmr.pricing_valid,
		       cmr.pricing_source, cmr.source_metadata, cmr.source_hash
		FROM catalog_model_revisions cmr
		JOIN catalog_models cm ON cm.id = cmr.model_id
		WHERE cmr.catalog_revision_id = $1
		ORDER BY cmr.model_id ASC
	`, spec.RevisionID)
	if err != nil {
		return rollback(fmt.Errorf("load active catalog model revisions: %w", err))
	}
	defer func() { _ = rows.Close() }()
	modelIndex := make(map[int64]int)
	for rows.Next() {
		var (
			candidate                   service.CatalogSnapshotModelSpec
			pricingRaw, capabilitiesRaw []byte
			metadataRaw                 []byte
			contextWindow, maxOutput    sql.NullInt64
			pricingSource               sql.NullString
			pricingValid                bool
		)
		if err := rows.Scan(
			&candidate.RevisionID,
			&candidate.ID,
			&candidate.CanonicalKey,
			&candidate.OperatorState,
			&candidate.OperatorReason,
			&candidate.OperatorVersion,
			&candidate.SourceState,
			&candidate.Provider,
			&candidate.Platform,
			&candidate.Mode,
			&capabilitiesRaw,
			&contextWindow,
			&maxOutput,
			&candidate.PricingSchemaVersion,
			&pricingRaw,
			&pricingValid,
			&pricingSource,
			&metadataRaw,
			&candidate.SourceHash,
		); err != nil {
			return rollback(fmt.Errorf("scan active catalog model revision: %w", err))
		}
		candidate.OperatorState = service.CatalogOperatorState(candidate.OperatorState)
		candidate.SourceState = service.CatalogSourceState(candidate.SourceState)
		candidate.PricingValid = pricingValid
		candidate.PricingSource = pricingSource.String
		candidate.Capabilities = append([]byte(nil), capabilitiesRaw...)
		candidate.SourceMetadata = append([]byte(nil), metadataRaw...)
		if contextWindow.Valid {
			candidate.ContextWindow = contextWindow.Int64
		}
		if maxOutput.Valid {
			candidate.MaxOutputTokens = maxOutput.Int64
		}
		if pricingValid {
			if len(pricingRaw) == 0 {
				return rollback(fmt.Errorf("%w: pricing missing for model %s", service.ErrCatalogInvalidSnapshot, candidate.CanonicalKey))
			}
			var pricing service.LiteLLMModelPricing
			if err := json.Unmarshal(pricingRaw, &pricing); err != nil {
				return rollback(fmt.Errorf("%w: pricing decode for model %s: %v", service.ErrCatalogInvalidSnapshot, candidate.CanonicalKey, err))
			}
			candidate.Pricing = &pricing
		}
		modelIndex[candidate.ID] = len(spec.Models)
		spec.Models = append(spec.Models, candidate)
	}
	if err := rows.Err(); err != nil {
		return rollback(fmt.Errorf("iterate active catalog model revisions: %w", err))
	}
	if len(spec.Models) == 0 {
		return rollback(fmt.Errorf("%w: active revision has no model rows", service.ErrCatalogInvalidSnapshot))
	}

	aliasRows, err := tx.QueryContext(ctx, `
		SELECT alias_normalized, platform_scope, model_id
		FROM catalog_model_aliases
		WHERE state = 'active'
		  AND (introduced_revision_id IS NULL OR introduced_revision_id <= $1)
		  AND (retired_revision_id IS NULL OR retired_revision_id > $1)
		ORDER BY platform_scope, alias_normalized
	`, spec.RevisionID)
	if err != nil {
		return rollback(fmt.Errorf("load active catalog aliases: %w", err))
	}
	defer func() { _ = aliasRows.Close() }()
	for aliasRows.Next() {
		var alias service.CatalogAliasSpec
		var modelID int64
		if err := aliasRows.Scan(&alias.Alias, &alias.PlatformScope, &modelID); err != nil {
			return rollback(fmt.Errorf("scan active catalog alias: %w", err))
		}
		modelPosition, ok := modelIndex[modelID]
		if !ok {
			return rollback(fmt.Errorf("%w: alias %s points to model %d outside revision", service.ErrCatalogInvalidSnapshot, alias.Alias, modelID))
		}
		spec.Models[modelPosition].Aliases = append(spec.Models[modelPosition].Aliases, alias)
	}
	if err := aliasRows.Err(); err != nil {
		return rollback(fmt.Errorf("iterate active catalog aliases: %w", err))
	}
	if _, err := service.NewCatalogRuntimeSnapshot(spec); err != nil {
		return rollback(err)
	}
	if err := tx.Commit(); err != nil {
		return service.CatalogSnapshotSpec{}, fmt.Errorf("commit active catalog snapshot read: %w", err)
	}
	return spec, nil
}

func (r *modelCatalogRepository) ListOutboxAfter(ctx context.Context, scope string, afterID int64, limit int) ([]service.CatalogOutboxEvent, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("nil catalog repository database")
	}
	if limit <= 0 {
		limit = 100
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, event_type, scope, publication_epoch, catalog_revision_id, model_id, payload, created_at
		FROM catalog_outbox
		WHERE scope = $1 AND id > $2
		ORDER BY id ASC
		LIMIT $3
	`, strings.TrimSpace(scope), afterID, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	events := make([]service.CatalogOutboxEvent, 0, limit)
	for rows.Next() {
		var (
			event      service.CatalogOutboxEvent
			modelID    sql.NullInt64
			payloadRaw []byte
		)
		if err := rows.Scan(&event.ID, &event.EventType, &event.Scope, &event.PublicationEpoch, &event.CatalogRevisionID, &modelID, &payloadRaw, &event.CreatedAt); err != nil {
			return nil, err
		}
		if modelID.Valid {
			value := modelID.Int64
			event.ModelID = &value
		}
		event.Payload = append(event.Payload[:0], payloadRaw...)
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return events, nil
}

func validateCatalogRevisionStage(stage service.CatalogRevisionStage) error {
	if stage.Revision <= 0 || strings.TrimSpace(stage.NormalizedHash) == "" || strings.TrimSpace(stage.Normalizer) == "" || len(stage.Models) == 0 {
		return service.ErrCatalogInvalidSnapshot
	}
	_, err := service.NewCatalogRuntimeSnapshot(service.CatalogSnapshotSpec{
		Scope:      service.CatalogScopeGlobal,
		Epoch:      1,
		RevisionID: 1,
		Revision:   1,
		Checksum:   stage.NormalizedHash,
		VerifiedAt: time.Unix(1, 0).UTC(),
		Models:     stage.Models,
	})
	if err != nil {
		return err
	}
	for _, candidate := range stage.Models {
		if strings.TrimSpace(candidate.SourceHash) == "" {
			return fmt.Errorf("%w: missing source hash for %s", service.ErrCatalogInvalidSnapshot, candidate.CanonicalKey)
		}
	}
	return nil
}

func marshalOptionalCatalogJSON(valid bool, value *service.LiteLLMModelPricing) ([]byte, error) {
	if !valid || value == nil {
		return nil, nil
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return encoded, nil
}

func optionalCatalogJSONArg(value []byte) any {
	if len(value) == 0 {
		return nil
	}
	return string(value)
}

func normalizeCatalogJSON(value, fallback []byte) []byte {
	if len(value) == 0 {
		return fallback
	}
	return append([]byte(nil), value...)
}

func mustCatalogErrorJSON(err error) string {
	encoded, marshalErr := json.Marshal([]string{err.Error()})
	if marshalErr != nil {
		return `["catalog stage failed"]`
	}
	return string(encoded)
}

func nullableCatalogString(value string) any {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return value
}

func nullableCatalogInt64(value int64) any {
	if value <= 0 {
		return nil
	}
	return value
}

func nullableCatalogJSON(value []byte) any {
	if len(value) == 0 {
		return nil
	}
	return string(value)
}

func positiveCatalogPricingSchema(value int) int {
	if value <= 0 {
		return 1
	}
	return value
}

func positiveCatalogOperatorVersion(value int64) int64 {
	if value <= 0 {
		return 1
	}
	return value
}

func normalizeCatalogValue(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizeCatalogPlatformScope(value string) string {
	value = normalizeCatalogValue(value)
	if value == "" {
		return service.CatalogPlatformScopeAny
	}
	return value
}
