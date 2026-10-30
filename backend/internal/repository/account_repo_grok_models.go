package repository

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

var errGrokCatalogInvalidJSON = errors.New("grok model catalog: invalid json")

var _ service.GrokCatalogSnapshotRepository = (*accountRepository)(nil)

const compareAndSwapGrokObservedModelsSQL = `UPDATE accounts
SET extra = jsonb_set(COALESCE(extra, '{}'::jsonb), '{grok_observed_models}', $3::jsonb, true),
    updated_at = NOW()
WHERE id = $1 AND deleted_at IS NULL AND platform = 'grok'
  AND COALESCE(extra -> 'grok_observed_models', 'null'::jsonb) = $2::jsonb`

func (r *accountRepository) CompareAndSwapGrokObservedModels(ctx context.Context, id int64, expected, next json.RawMessage) (bool, error) {
	if scoped := r.accountTransactionScope(ctx); scoped != r {
		return scoped.CompareAndSwapGrokObservedModels(ctx, id, expected, next)
	}
	expectedArg, err := grokCatalogJSONArg(expected)
	if err != nil {
		return false, err
	}
	nextArg, err := grokCatalogJSONArg(next)
	if err != nil {
		return false, err
	}
	result, err := clientFromContext(ctx, r.client).ExecContext(ctx, compareAndSwapGrokObservedModelsSQL, id, expectedArg, nextArg)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if affected == 0 {
		return false, nil
	}
	if dbent.TxFromContext(ctx) == nil {
		r.syncSchedulerAccountSnapshot(ctx, id)
	}
	return true, nil
}

func grokCatalogJSONArg(raw json.RawMessage) (string, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return "null", nil
	}
	if !json.Valid(trimmed) {
		return "", errGrokCatalogInvalidJSON
	}
	return string(trimmed), nil
}
