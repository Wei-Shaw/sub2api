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
)

// ApplyRevisionMutation performs the complete admin control-plane write in one
// PostgreSQL transaction. The mutable catalog_models row is only the CAS/control
// row; the published revision receives a copy of the resulting operator policy.
func (r *modelCatalogRepository) ApplyRevisionMutation(ctx context.Context, request service.CatalogAdminRevisionMutationRequest) (service.CatalogPublicationRecord, error) {
	if r == nil || r.db == nil {
		return service.CatalogPublicationRecord{}, errors.New("nil catalog repository database")
	}
	if strings.TrimSpace(request.Scope) == "" || request.Stage.Revision <= 0 || request.ModelID <= 0 || request.ExpectedOperatorVersion <= 0 {
		return service.CatalogPublicationRecord{}, service.ErrCatalogInvalidSnapshot
	}
	if err := validateCatalogRevisionStage(request.Stage); err != nil {
		return service.CatalogPublicationRecord{}, err
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return service.CatalogPublicationRecord{}, fmt.Errorf("begin admin catalog mutation: %w", err)
	}
	rollback := func(cause error) (service.CatalogPublicationRecord, error) {
		_ = tx.Rollback()
		return service.CatalogPublicationRecord{}, cause
	}
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext('model-catalog-sync')::bigint)`); err != nil {
		return rollback(fmt.Errorf("lock admin catalog mutation: %w", err))
	}

	var (
		currentRevisionID int64
		currentEpoch      int64
	)
	if err := tx.QueryRowContext(ctx, `
		SELECT active_revision_id, epoch
		FROM catalog_publications
		WHERE scope = $1
		FOR UPDATE
	`, strings.TrimSpace(request.Scope)).Scan(&currentRevisionID, &currentEpoch); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return rollback(service.ErrCatalogNoPublication)
		}
		return rollback(fmt.Errorf("lock catalog publication: %w", err))
	}
	if currentEpoch != request.ExpectedEpoch || currentRevisionID != request.ExpectedRevisionID {
		return rollback(fmt.Errorf("%w: expected=(%d,%d) actual=(%d,%d)", service.ErrCatalogPublicationConflict, request.ExpectedEpoch, request.ExpectedRevisionID, currentEpoch, currentRevisionID))
	}

	before, err := loadAdminModelStateTx(ctx, tx, currentRevisionID, request.ModelID)
	if err != nil {
		return rollback(err)
	}
	var (
		currentState   service.CatalogOperatorState
		currentReason  string
		currentVersion int64
	)
	if err := tx.QueryRowContext(ctx, `
		SELECT operator_state, COALESCE(operator_reason, ''), operator_version
		FROM catalog_models
		WHERE id = $1
		FOR UPDATE
	`, request.ModelID).Scan(&currentState, &currentReason, &currentVersion); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return rollback(service.ErrCatalogModelNotFound)
		}
		return rollback(fmt.Errorf("load catalog model control row: %w", err))
	}
	if currentVersion != request.ExpectedOperatorVersion {
		return rollback(fmt.Errorf("%w: expected version=%d actual=%d", service.ErrCatalogOperatorConflict, request.ExpectedOperatorVersion, currentVersion))
	}

	if request.OperatorState != nil {
		if *request.OperatorState != service.CatalogOperatorStateEnabled && *request.OperatorState != service.CatalogOperatorStateDisabled {
			return rollback(service.ErrCatalogInvalidSnapshot)
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE catalog_models
			SET operator_state = $2,
			    operator_reason = NULLIF($3, ''),
			    operator_version = operator_version + 1,
			    last_operator_change_at = NOW(),
			    updated_at = NOW()
			WHERE id = $1 AND operator_version = $4
		`, request.ModelID, string(*request.OperatorState), strings.TrimSpace(request.Reason), request.ExpectedOperatorVersion); err != nil {
			return rollback(fmt.Errorf("update catalog operator policy: %w", err))
		}
		currentState = *request.OperatorState
		currentReason = strings.TrimSpace(request.Reason)
		currentVersion++
	}

	stage := request.Stage
	stage.Revision, err = nextCatalogRevisionTx(ctx, tx)
	if err != nil {
		return rollback(err)
	}
	revisionID, err := stageAdminRevisionTx(ctx, tx, stage)
	if err != nil {
		return rollback(err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE catalog_revisions
		SET state = 'validated', validated_at = NOW()
		WHERE id = $1 AND state = 'staged'
	`, revisionID); err != nil {
		return rollback(fmt.Errorf("validate admin catalog revision: %w", err))
	}

	nextEpoch := currentEpoch + 1
	var updatedAt time.Time
	if err := tx.QueryRowContext(ctx, `
		UPDATE catalog_publications
		SET active_revision_id = $2, epoch = $3, updated_at = NOW()
		WHERE scope = $1
		RETURNING updated_at
	`, strings.TrimSpace(request.Scope), revisionID, nextEpoch).Scan(&updatedAt); err != nil {
		return rollback(fmt.Errorf("publish admin catalog revision: %w", err))
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE catalog_revisions
		SET state = 'published', published_at = COALESCE(published_at, NOW())
		WHERE id = $1
	`, revisionID); err != nil {
		return rollback(fmt.Errorf("mark admin catalog revision published: %w", err))
	}

	publicationBefore, _ := json.Marshal(map[string]any{
		"scope": strings.TrimSpace(request.Scope), "publication_epoch": currentEpoch, "catalog_revision_id": currentRevisionID,
	})
	publicationAfter, _ := json.Marshal(map[string]any{
		"scope": strings.TrimSpace(request.Scope), "publication_epoch": nextEpoch, "catalog_revision_id": revisionID,
	})
	payload, _ := json.Marshal(map[string]any{
		"scope": strings.TrimSpace(request.Scope), "publication_epoch": nextEpoch, "catalog_revision_id": revisionID,
	})
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO catalog_outbox (event_type, scope, publication_epoch, catalog_revision_id, payload, dedup_key)
		VALUES ('catalog.publication.published', $1, $2, $3, $4::jsonb, $5)
		ON CONFLICT (dedup_key) WHERE dedup_key IS NOT NULL DO NOTHING
	`, strings.TrimSpace(request.Scope), nextEpoch, revisionID, string(payload), fmt.Sprintf("%s:%d:%d", strings.TrimSpace(request.Scope), nextEpoch, revisionID)); err != nil {
		return rollback(fmt.Errorf("write admin catalog outbox: %w", err))
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO catalog_lifecycle_audits
			(catalog_revision_id, action, actor_type, actor_user_id, reason, before_state, after_state, request_id, correlation_id)
		VALUES ($1, 'publish', 'admin', $2, $3, $4::jsonb, $5::jsonb, $6, $7)
	`, revisionID, nullableAdminActor(request.ActorUserID), nullableCatalogString(request.Reason), string(publicationBefore), string(publicationAfter), nullableCatalogString(request.RequestID), nullableCatalogString(request.CorrelationID)); err != nil {
		return rollback(fmt.Errorf("write admin publication audit: %w", err))
	}

	after := map[string]any{
		"model_id": request.ModelID, "operator_state": currentState,
		"operator_reason": currentReason, "operator_version": currentVersion,
		"catalog_revision_id": revisionID, "catalog_epoch": nextEpoch,
	}
	afterJSON, _ := json.Marshal(after)
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO catalog_lifecycle_audits
			(model_id, catalog_revision_id, action, actor_type, actor_user_id, reason, before_state, after_state, request_id, correlation_id)
		VALUES ($1, $2, $3, 'admin', $4, $5, $6::jsonb, $7::jsonb, $8, $9)
	`, request.ModelID, revisionID, nonEmptyAdminAction(request.Action), nullableAdminActor(request.ActorUserID), nullableCatalogString(request.Reason), string(before), string(afterJSON), nullableCatalogString(request.RequestID), nullableCatalogString(request.CorrelationID)); err != nil {
		return rollback(fmt.Errorf("write admin model audit: %w", err))
	}
	if err := tx.Commit(); err != nil {
		return service.CatalogPublicationRecord{}, fmt.Errorf("commit admin catalog mutation: %w", err)
	}
	return service.CatalogPublicationRecord{Scope: strings.TrimSpace(request.Scope), CatalogRevisionID: revisionID, Epoch: nextEpoch, UpdatedAt: updatedAt}, nil
}

func nextCatalogRevisionTx(ctx context.Context, tx *sql.Tx) (int64, error) {
	var revision int64
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(revision), 0) + 1 FROM catalog_revisions`).Scan(&revision); err != nil {
		return 0, fmt.Errorf("allocate admin catalog revision: %w", err)
	}
	return revision, nil
}

func stageAdminRevisionTx(ctx context.Context, tx *sql.Tx, stage service.CatalogRevisionStage) (int64, error) {
	syncRunID, err := createSyncRunTx(ctx, tx, stage.SyncRun)
	if err != nil {
		return 0, fmt.Errorf("create admin catalog sync run: %w", err)
	}
	var revisionID int64
	if err := tx.QueryRowContext(ctx, `
		INSERT INTO catalog_revisions (revision, sync_run_id, normalized_hash, normalizer_version, state, model_count)
		VALUES ($1, $2, $3, $4, 'staged', $5) RETURNING id
	`, stage.Revision, syncRunID, strings.TrimSpace(stage.NormalizedHash), strings.TrimSpace(stage.Normalizer), len(stage.Models)).Scan(&revisionID); err != nil {
		return 0, fmt.Errorf("insert admin catalog revision: %w", err)
	}
	for _, candidate := range stage.Models {
		modelID, policy, err := upsertCatalogModel(ctx, tx, candidate)
		if err != nil {
			return 0, fmt.Errorf("upsert admin catalog model %s: %w", candidate.CanonicalKey, err)
		}
		candidate.OperatorState = policy.State
		candidate.OperatorReason = policy.Reason
		candidate.OperatorVersion = policy.OperatorVersion
		if err := insertCatalogModelRevision(ctx, tx, revisionID, modelID, candidate); err != nil {
			return 0, fmt.Errorf("insert admin catalog model revision %s: %w", candidate.CanonicalKey, err)
		}
		for _, alias := range candidate.Aliases {
			if err := upsertCatalogAlias(ctx, tx, revisionID, modelID, alias, candidate); err != nil {
				return 0, fmt.Errorf("upsert admin catalog alias %s: %w", alias.Alias, err)
			}
		}
	}
	missingResult, err := tx.ExecContext(ctx, `
		INSERT INTO catalog_model_revisions
			(catalog_revision_id, model_id, source_state, provider, platform, mode, capabilities,
			 context_window, max_output_tokens, pricing_schema_version, pricing_json, pricing_valid,
			 operator_state, operator_reason, operator_version, pricing_source, source_metadata, source_hash)
		SELECT $1, cm.id, 'missing', '', '', '', '{}'::jsonb, NULL, NULL, 1, NULL, FALSE,
		       cm.operator_state, cm.operator_reason, cm.operator_version, NULL, '{}'::jsonb,
		       'missing:' || cm.id::text || ':' || $1::text
		FROM catalog_models cm
		WHERE NOT EXISTS (SELECT 1 FROM catalog_model_revisions cmr WHERE cmr.catalog_revision_id = $1 AND cmr.model_id = cm.id)
	`, revisionID)
	if err != nil {
		return 0, fmt.Errorf("stage admin source-missing models: %w", err)
	}
	missingCount, _ := missingResult.RowsAffected()
	if missingCount > 0 {
		if _, err := tx.ExecContext(ctx, `UPDATE catalog_revisions SET model_count = model_count + $2 WHERE id = $1`, revisionID, missingCount); err != nil {
			return 0, fmt.Errorf("update admin model count: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE catalog_sync_runs SET status = 'staged', normalized_hash = $2, completed_at = NOW() WHERE id = $1`, syncRunID, stage.NormalizedHash); err != nil {
		return 0, fmt.Errorf("mark admin sync run staged: %w", err)
	}
	return revisionID, nil
}

func loadAdminModelStateTx(ctx context.Context, tx *sql.Tx, revisionID, modelID int64) ([]byte, error) {
	var (
		state, reason, sourceHash string
		version                   int64
		pricing                   []byte
		pricingValid              bool
	)
	err := tx.QueryRowContext(ctx, `
		SELECT cmr.operator_state, COALESCE(cmr.operator_reason, ''), cmr.operator_version,
		       cmr.pricing_valid, cmr.pricing_json, cmr.source_hash
		FROM catalog_model_revisions cmr
		WHERE cmr.catalog_revision_id = $1 AND cmr.model_id = $2
	`, revisionID, modelID).Scan(&state, &reason, &version, &pricingValid, &pricing, &sourceHash)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrCatalogModelNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("load admin model before state: %w", err)
	}
	before, marshalErr := json.Marshal(map[string]any{
		"model_id": modelID, "operator_state": state, "operator_reason": reason,
		"operator_version": version, "pricing_valid": pricingValid,
		"pricing": json.RawMessage(pricing), "source_hash": sourceHash,
	})
	if marshalErr != nil {
		return nil, fmt.Errorf("marshal admin model before state: %w", marshalErr)
	}
	return before, nil
}

func nullableAdminActor(actor int64) any {
	if actor <= 0 {
		return nil
	}
	return actor
}

func nonEmptyAdminAction(action string) string {
	if strings.TrimSpace(action) == "" {
		return "admin_catalog_update"
	}
	return strings.TrimSpace(action)
}
