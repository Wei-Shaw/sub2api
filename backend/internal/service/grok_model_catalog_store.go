package service

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

var (
	errGrokCatalogSnapshotRepositoryUnavailable = infraerrors.InternalServer(
		"GROK_CATALOG_SNAPSHOT_REPOSITORY_UNAVAILABLE",
		"grok model catalog snapshot repository is unavailable",
	)
	errGrokCatalogAccountMismatch  = errors.New("grok model catalog account mismatch")
	errGrokCatalogConcurrentChange = errors.New("grok model catalog changed concurrently")
)

// GrokCatalogSnapshotRepository is the narrow compare-and-swap capability for one
// observed-model snapshot. It is intentionally not part of AccountRepository.
type GrokCatalogSnapshotRepository interface {
	CompareAndSwapGrokObservedModels(ctx context.Context, id int64, expected, next json.RawMessage) (bool, error)
}

// PersistGrokCatalogObservation merges one observation into the account snapshot.
// The observation is fixed for every attempt; a conflict rereads and merges again,
// at most three times, and never falls back to an unlocked extra update.
func PersistGrokCatalogObservation(ctx context.Context, repo AccountRepository, accountID int64, observation GrokCatalogObservation) (*GrokObservedModelsSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	cas, err := grokCatalogSnapshotRepository(repo)
	if err != nil {
		return nil, err
	}
	if accountID <= 0 || (observation.Attempt.Scope != nil && observation.Attempt.Scope.AccountID != accountID) {
		return nil, errGrokCatalogAccountMismatch
	}

	for attempt := 0; attempt < 3; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		account, err := repo.GetByID(ctx, accountID)
		if err != nil {
			return nil, err
		}
		if account == nil {
			return nil, ErrAccountNotFound
		}
		if account.ID != accountID || account.Platform != PlatformGrok {
			return nil, errGrokCatalogAccountMismatch
		}
		previous := parseGrokObservedModels(account.Extra)
		next, changed, err := MergeGrokCatalogObservation(previous, observation)
		if err != nil {
			return nil, err
		}
		if !changed {
			return next, nil
		}
		var current any
		if account.Extra != nil {
			current = account.Extra[grokObservedModelsExtraKey]
		}
		expectedJSON, err := json.Marshal(current)
		if err != nil {
			return nil, err
		}
		nextJSON, err := json.Marshal(next)
		if err != nil {
			return nil, err
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		applied, err := cas.CompareAndSwapGrokObservedModels(ctx, accountID, expectedJSON, nextJSON)
		if err != nil {
			return nil, err
		}
		if applied {
			return next, nil
		}
	}
	return nil, errGrokCatalogConcurrentChange
}

func grokCatalogSnapshotRepository(repo AccountRepository) (GrokCatalogSnapshotRepository, error) {
	if isNilGrokCatalogValue(repo) {
		return nil, errGrokCatalogSnapshotRepositoryUnavailable
	}
	cas, ok := repo.(GrokCatalogSnapshotRepository)
	if !ok || isNilGrokCatalogValue(cas) {
		return nil, errGrokCatalogSnapshotRepositoryUnavailable
	}
	return cas, nil
}

func isNilGrokCatalogValue(value any) bool {
	if value == nil {
		return true
	}
	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return rv.IsNil()
	default:
		return false
	}
}
