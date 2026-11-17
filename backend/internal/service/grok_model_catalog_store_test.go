//go:build unit

package service

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sync"
	"testing"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

var errGrokCatalogMemoryInvalidJSON = errors.New("grok model catalog: invalid json")

func TestGrokCatalogPersistMergesAfterConflictWithoutUpdateExtra(t *testing.T) {
	t.Parallel()

	scope := grokCatalogPersistScope(t, 25)
	repo := newGrokCatalogMemoryRepo(&Account{
		ID:          25,
		Platform:    PlatformGrok,
		Credentials: map[string]any{"access_token": "token-keep"},
		Extra: map[string]any{
			"unrelated": "keep",
			grokObservedModelsExtraKey: map[string]any{
				"models":             []any{"base"},
				"fetched_at":         "2026-10-25T01:00:01Z",
				"success_started_at": "2026-10-25T01:00:00Z",
				"last_attempt": map[string]any{
					"started_at":  "2026-10-25T01:00:00Z",
					"finished_at": "2026-10-25T01:00:01Z",
					"status":      "success",
				},
			},
		},
	})
	repo.conflictsRemaining = 1
	repo.onConflict = func(account *Account) {
		account.Extra[grokObservedModelsExtraKey] = map[string]any{
			"models":             []any{"concurrent"},
			"fetched_at":         "2026-10-25T02:00:01Z",
			"success_started_at": "2026-10-25T02:00:00Z",
			"last_attempt": map[string]any{
				"started_at":  "2026-10-25T02:00:00Z",
				"finished_at": "2026-10-25T02:00:01Z",
				"status":      "success",
			},
		}
	}
	observation := GrokCatalogObservation{Attempt: GrokCatalogAttempt{
		StartedAt:  "2026-10-25T04:00:00Z",
		FinishedAt: "2026-10-25T04:00:01Z",
		Status:     "failed",
		ErrorCode:  "transport",
		Scope:      scope,
	}}

	got, err := PersistGrokCatalogObservation(context.Background(), repo, 25, observation)

	require.NoError(t, err)
	require.Equal(t, []string{"concurrent"}, got.Models)
	require.Equal(t, "2026-10-25T02:00:01Z", got.FetchedAt)
	require.Equal(t, "2026-10-25T02:00:00Z", got.SuccessStartedAt)
	require.Equal(t, "2026-10-25T04:00:00Z", got.LastAttempt.StartedAt)
	require.Equal(t, "failed", got.LastAttempt.Status)
	require.Equal(t, "2026-10-25T04:00:00Z", observation.Attempt.StartedAt)
	require.Equal(t, 2, repo.casCalls)
	require.Equal(t, 2, repo.getCalls)
	require.Zero(t, repo.updateExtraCalls)
	stored, err := repo.GetByID(context.Background(), 25)
	require.NoError(t, err)
	require.Equal(t, "keep", stored.Extra["unrelated"])
	require.Equal(t, "token-keep", stored.Credentials["access_token"])
	snap := parseGrokObservedModels(stored.Extra)
	require.Equal(t, []string{"concurrent"}, snap.Models)
	require.Equal(t, "2026-10-25T04:00:01Z", snap.LastAttempt.FinishedAt)
	got.Models[0] = "mutated"
	require.Equal(t, []string{"concurrent"}, parseGrokObservedModels(stored.Extra).Models)
}

func TestGrokCatalogPersistStopsAfterThreeConflicts(t *testing.T) {
	t.Parallel()

	scope := grokCatalogPersistScope(t, 25)
	repo := newGrokCatalogMemoryRepo(&Account{
		ID:       25,
		Platform: PlatformGrok,
		Extra: map[string]any{
			"unrelated": "keep",
			grokObservedModelsExtraKey: map[string]any{
				"models":             []any{"kept"},
				"fetched_at":         "2026-10-25T01:00:01Z",
				"success_started_at": "2026-10-25T01:00:00Z",
			},
		},
	})
	repo.conflictsRemaining = 3

	got, err := PersistGrokCatalogObservation(context.Background(), repo, 25, grokCatalogPersistSuccess(scope, "2026-10-25T03:00:00Z", "2026-10-25T03:00:01Z", "replacement"))

	require.Nil(t, got)
	require.ErrorIs(t, err, errGrokCatalogConcurrentChange)
	require.Equal(t, 3, repo.getCalls)
	require.Equal(t, 3, repo.casCalls)
	require.Zero(t, repo.updateExtraCalls)
	stored, err := repo.GetByID(context.Background(), 25)
	require.NoError(t, err)
	require.Equal(t, "keep", stored.Extra["unrelated"])
	require.Equal(t, []string{"kept"}, parseGrokObservedModels(stored.Extra).Models)
}

func TestGrokCatalogPersistRejectsDeletedMismatchAndMissingCapability(t *testing.T) {
	t.Parallel()

	t.Run("nil repository", func(t *testing.T) {
		t.Parallel()
		got, err := PersistGrokCatalogObservation(context.Background(), nil, 25, grokCatalogPersistFailure(t, 25))
		require.Nil(t, got)
		require.ErrorIs(t, err, errGrokCatalogSnapshotRepositoryUnavailable)
		require.True(t, infraerrors.IsInternalServer(err))
	})

	t.Run("typed nil repository", func(t *testing.T) {
		t.Parallel()
		var repo *grokCatalogMemoryRepo
		got, err := PersistGrokCatalogObservation(context.Background(), repo, 25, grokCatalogPersistFailure(t, 25))
		require.Nil(t, got)
		require.ErrorIs(t, err, errGrokCatalogSnapshotRepositoryUnavailable)
	})

	t.Run("capability missing does not update extra", func(t *testing.T) {
		t.Parallel()
		repo := &grokCatalogRepoWithoutCAS{}
		got, err := PersistGrokCatalogObservation(context.Background(), repo, 25, grokCatalogPersistFailure(t, 25))
		require.Nil(t, got)
		require.ErrorIs(t, err, errGrokCatalogSnapshotRepositoryUnavailable)
		require.Zero(t, repo.getCalls)
		require.Zero(t, repo.updateCalls)
	})

	t.Run("deleted account", func(t *testing.T) {
		t.Parallel()
		repo := newGrokCatalogMemoryRepo(grokCatalogPersistAccount())
		repo.deleted = true
		got, err := PersistGrokCatalogObservation(context.Background(), repo, 25, grokCatalogPersistSuccess(grokCatalogPersistScope(t, 25), "2026-10-25T03:00:00Z", "2026-10-25T03:00:01Z", "grok-4.7"))
		require.Nil(t, got)
		require.ErrorIs(t, err, ErrAccountNotFound)
		require.Zero(t, repo.casCalls)
	})

	t.Run("scope account id mismatches", func(t *testing.T) {
		t.Parallel()
		repo := newGrokCatalogMemoryRepo(grokCatalogPersistAccount())
		got, err := PersistGrokCatalogObservation(context.Background(), repo, 25, grokCatalogPersistSuccess(grokCatalogPersistScope(t, 99), "2026-10-25T03:00:00Z", "2026-10-25T03:00:01Z", "grok-4.7"))
		require.Nil(t, got)
		require.ErrorIs(t, err, errGrokCatalogAccountMismatch)
		require.Zero(t, repo.getCalls)
		require.Zero(t, repo.casCalls)
		stored, err := repo.GetByID(context.Background(), 25)
		require.NoError(t, err)
		require.Equal(t, []string{"kept"}, parseGrokObservedModels(stored.Extra).Models)
		require.Equal(t, "keep", stored.Extra["unrelated"])
	})

	t.Run("loaded account id mismatches", func(t *testing.T) {
		t.Parallel()
		repo := &grokCatalogMismatchedIDRepo{grokCatalogMemoryRepo: newGrokCatalogMemoryRepo(grokCatalogPersistAccount())}
		got, err := PersistGrokCatalogObservation(context.Background(), repo, 25, grokCatalogPersistFailure(t, 25))
		require.Nil(t, got)
		require.ErrorIs(t, err, errGrokCatalogAccountMismatch)
		require.Zero(t, repo.casCalls)
		require.Equal(t, []string{"kept"}, parseGrokObservedModels(repo.account.Extra).Models)
	})

	t.Run("platform mismatches", func(t *testing.T) {
		t.Parallel()
		account := grokCatalogPersistAccount()
		account.Platform = PlatformOpenAI
		repo := newGrokCatalogMemoryRepo(account)
		got, err := PersistGrokCatalogObservation(context.Background(), repo, 25, grokCatalogPersistFailure(t, 25))
		require.Nil(t, got)
		require.ErrorIs(t, err, errGrokCatalogAccountMismatch)
		require.Zero(t, repo.casCalls)
		require.Equal(t, "keep", repo.account.Extra["unrelated"])
	})

	t.Run("non positive account id", func(t *testing.T) {
		t.Parallel()
		repo := newGrokCatalogMemoryRepo(grokCatalogPersistAccount())
		got, err := PersistGrokCatalogObservation(context.Background(), repo, 0, grokCatalogPersistFailure(t, 25))
		require.Nil(t, got)
		require.ErrorIs(t, err, errGrokCatalogAccountMismatch)
		require.Zero(t, repo.getCalls)
	})
}

func TestGrokCatalogPersistKeepsNewerSnapshotWhenObservationIsOlder(t *testing.T) {
	t.Parallel()

	scope := grokCatalogPersistScope(t, 25)
	repo := newGrokCatalogMemoryRepo(&Account{
		ID:       25,
		Platform: PlatformGrok,
		Extra: map[string]any{
			"unrelated": "keep",
			grokObservedModelsExtraKey: map[string]any{
				"models":             []any{"newer"},
				"fetched_at":         "2026-10-25T05:00:01Z",
				"success_started_at": "2026-10-25T05:00:00Z",
				"last_attempt": map[string]any{
					"started_at":  "2026-10-25T05:00:00Z",
					"finished_at": "2026-10-25T05:00:01Z",
					"status":      "success",
				},
			},
		},
	})

	got, err := PersistGrokCatalogObservation(context.Background(), repo, 25, grokCatalogPersistSuccess(scope, "2026-10-25T03:00:00Z", "2026-10-25T03:00:01Z", "older"))

	require.NoError(t, err)
	require.Equal(t, []string{"newer"}, got.Models)
	require.Equal(t, "2026-10-25T05:00:01Z", got.FetchedAt)
	require.Zero(t, repo.casCalls)
	require.Zero(t, repo.updateExtraCalls)
	stored, err := repo.GetByID(context.Background(), 25)
	require.NoError(t, err)
	require.Equal(t, []string{"newer"}, parseGrokObservedModels(stored.Extra).Models)
	require.Equal(t, "keep", stored.Extra["unrelated"])
}

func TestGrokCatalogPersistCancelStopsImmediately(t *testing.T) {
	t.Parallel()

	t.Run("already canceled", func(t *testing.T) {
		t.Parallel()
		repo := newGrokCatalogMemoryRepo(grokCatalogPersistAccount())
		repo.writesWithoutContextCheck = true
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		got, err := PersistGrokCatalogObservation(ctx, repo, 25, grokCatalogPersistSuccess(grokCatalogPersistScope(t, 25), "2026-10-25T03:00:00Z", "2026-10-25T03:00:01Z", "grok-4.7"))

		require.Nil(t, got)
		require.ErrorIs(t, err, context.Canceled)
		require.Zero(t, repo.getCalls)
		require.Zero(t, repo.casCalls)
		require.Equal(t, []string{"kept"}, parseGrokObservedModels(repo.account.Extra).Models)
	})

	t.Run("canceled between attempts", func(t *testing.T) {
		t.Parallel()
		repo := newGrokCatalogMemoryRepo(grokCatalogPersistAccount())
		repo.writesWithoutContextCheck = true
		repo.conflictsRemaining = 3
		ctx, cancel := context.WithCancel(context.Background())
		repo.afterCAS = func() { cancel() }

		got, err := PersistGrokCatalogObservation(ctx, repo, 25, grokCatalogPersistSuccess(grokCatalogPersistScope(t, 25), "2026-10-25T03:00:00Z", "2026-10-25T03:00:01Z", "grok-4.7"))

		require.Nil(t, got)
		require.ErrorIs(t, err, context.Canceled)
		require.Equal(t, 1, repo.casCalls)
		require.Equal(t, []string{"kept"}, parseGrokObservedModels(repo.account.Extra).Models)
	})
}

func TestGrokCatalogPersistMemoryRepoDeepCopiesAndMergesExtra(t *testing.T) {
	t.Parallel()

	extra := map[string]any{
		"unrelated": "keep",
		grokObservedModelsExtraKey: map[string]any{
			"models": []any{"kept"},
		},
	}
	account := &Account{ID: 25, Platform: PlatformGrok, Extra: extra, Credentials: map[string]any{"access_token": "token-keep"}}
	repo := newGrokCatalogMemoryRepo(account)
	extra["unrelated"] = "changed-input"
	account.Credentials["access_token"] = "changed-input"

	got, err := repo.GetByID(context.Background(), 25)
	require.NoError(t, err)
	require.Equal(t, "keep", got.Extra["unrelated"])
	got.Extra["unrelated"] = "changed-read"
	got.Extra[grokObservedModelsExtraKey].(map[string]any)["models"] = []any{"changed-read"}
	got.Credentials["access_token"] = "changed-read"

	again, err := repo.GetByID(context.Background(), 25)
	require.NoError(t, err)
	require.Equal(t, "keep", again.Extra["unrelated"])
	require.Equal(t, []any{"kept"}, again.Extra[grokObservedModelsExtraKey].(map[string]any)["models"])
	require.Equal(t, "token-keep", again.Credentials["access_token"])

	metadata := map[string]any{"source": "manual"}
	require.NoError(t, repo.UpdateExtra(context.Background(), 25, map[string]any{"metadata": metadata}))
	metadata["source"] = "mutated"
	stored, err := repo.GetByID(context.Background(), 25)
	require.NoError(t, err)
	require.Equal(t, "keep", stored.Extra["unrelated"])
	require.Equal(t, "manual", stored.Extra["metadata"].(map[string]any)["source"])
	require.Equal(t, []any{"kept"}, stored.Extra[grokObservedModelsExtraKey].(map[string]any)["models"])

	require.NoError(t, repo.UpdateExtra(context.Background(), 25, map[string]any{
		grokObservedModelsExtraKey: map[string]any{"models": []any{"replaced"}},
	}))
	stored, err = repo.GetByID(context.Background(), 25)
	require.NoError(t, err)
	require.Equal(t, "keep", stored.Extra["unrelated"])
	require.Equal(t, "manual", stored.Extra["metadata"].(map[string]any)["source"])
	require.Equal(t, []any{"replaced"}, stored.Extra[grokObservedModelsExtraKey].(map[string]any)["models"])
	require.ErrorIs(t, repo.UpdateExtra(context.Background(), 99, map[string]any{"metadata": "x"}), ErrAccountNotFound)
}

func TestGrokCatalogPersistConcurrentWritersAreRaceSafe(t *testing.T) {
	repo := newGrokCatalogMemoryRepo(&Account{
		ID:          25,
		Platform:    PlatformGrok,
		Credentials: map[string]any{"access_token": "token-keep"},
		Extra:       map[string]any{"unrelated": "keep"},
	})
	scope := grokCatalogPersistScope(t, 25)
	var wg sync.WaitGroup
	errCh := make(chan error, 3)
	wg.Add(3)
	go func() {
		defer wg.Done()
		_, err := PersistGrokCatalogObservation(context.Background(), repo, 25, grokCatalogPersistSuccess(scope, "2026-10-25T02:00:00Z", "2026-10-25T02:00:01Z", "older"))
		errCh <- err
	}()
	go func() {
		defer wg.Done()
		_, err := PersistGrokCatalogObservation(context.Background(), repo, 25, grokCatalogPersistSuccess(scope, "2026-10-25T03:00:00Z", "2026-10-25T03:00:01Z", "newer"))
		errCh <- err
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			got, err := repo.GetByID(context.Background(), 25)
			if err != nil {
				errCh <- err
				return
			}
			got.Extra["unrelated"] = "mutated"
			if raw, ok := got.Extra[grokObservedModelsExtraKey].(map[string]any); ok {
				raw["models"] = []any{"mutated"}
			}
		}
	}()
	wg.Wait()
	close(errCh)
	for err := range errCh {
		require.NoError(t, err)
	}

	stored, err := repo.GetByID(context.Background(), 25)
	require.NoError(t, err)
	require.Equal(t, "keep", stored.Extra["unrelated"])
	require.Equal(t, "token-keep", stored.Credentials["access_token"])
	require.Equal(t, []string{"newer"}, parseGrokObservedModels(stored.Extra).Models)
	require.Zero(t, repo.updateExtraCalls)
}

func grokCatalogPersistScope(t *testing.T, accountID int64) *GrokCatalogScope {
	t.Helper()
	scope, err := NewGrokCatalogScope(accountID, AccountTypeOAuth, "https://cli-chat-proxy.grok.com/v1/models")
	require.NoError(t, err)
	return scope
}

func grokCatalogPersistSuccess(scope *GrokCatalogScope, started, finished, model string) GrokCatalogObservation {
	return GrokCatalogObservation{
		Models: []string{model},
		Attempt: GrokCatalogAttempt{
			StartedAt:  started,
			FinishedAt: finished,
			Status:     "success",
			Scope:      scope,
		},
	}
}

func grokCatalogPersistFailure(t *testing.T, accountID int64) GrokCatalogObservation {
	t.Helper()
	return GrokCatalogObservation{Attempt: GrokCatalogAttempt{
		StartedAt:  "2026-10-25T03:00:00Z",
		FinishedAt: "2026-10-25T03:00:01Z",
		Status:     "failed",
		ErrorCode:  "transport",
		Scope:      grokCatalogPersistScope(t, accountID),
	}}
}

func grokCatalogPersistAccount() *Account {
	return &Account{
		ID:          25,
		Platform:    PlatformGrok,
		Credentials: map[string]any{"access_token": "token-keep"},
		Extra: map[string]any{
			"unrelated": "keep",
			grokObservedModelsExtraKey: map[string]any{
				"models":             []any{"kept"},
				"fetched_at":         "2026-10-25T01:00:01Z",
				"success_started_at": "2026-10-25T01:00:00Z",
				"last_attempt": map[string]any{
					"started_at":  "2026-10-25T01:00:00Z",
					"finished_at": "2026-10-25T01:00:01Z",
					"status":      "failed",
					"error_code":  "transport",
				},
			},
		},
	}
}

type grokCatalogRepoWithoutCAS struct {
	AccountRepository
	getCalls    int
	updateCalls int
}

func (r *grokCatalogRepoWithoutCAS) GetByID(context.Context, int64) (*Account, error) {
	r.getCalls++
	return nil, ErrAccountNotFound
}

func (r *grokCatalogRepoWithoutCAS) UpdateExtra(context.Context, int64, map[string]any) error {
	r.updateCalls++
	return nil
}

type grokCatalogMismatchedIDRepo struct {
	*grokCatalogMemoryRepo
}

func (r *grokCatalogMismatchedIDRepo) GetByID(ctx context.Context, id int64) (*Account, error) {
	account, err := r.grokCatalogMemoryRepo.GetByID(ctx, id)
	if account != nil {
		account.ID = id + 1
	}
	return account, err
}

// grokCatalogMemoryRepo is the thread-safe account fixture shared with later catalog persistence tests.
type grokCatalogMemoryRepo struct {
	AccountRepository
	mu                        sync.Mutex
	account                   *Account
	deleted                   bool
	getCalls                  int
	casCalls                  int
	updateExtraCalls          int
	conflictsRemaining        int
	writesWithoutContextCheck bool
	onConflict                func(*Account)
	afterCAS                  func()
}

func newGrokCatalogMemoryRepo(account *Account) *grokCatalogMemoryRepo {
	return &grokCatalogMemoryRepo{account: cloneGrokCatalogMemoryAccount(account)}
}

func (r *grokCatalogMemoryRepo) GetByID(ctx context.Context, id int64) (*Account, error) {
	if !r.writesWithoutContextCheck {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.getCalls++
	if r.deleted || r.account == nil || r.account.ID != id {
		return nil, ErrAccountNotFound
	}
	return cloneGrokCatalogMemoryAccount(r.account), nil
}

func (r *grokCatalogMemoryRepo) UpdateExtra(ctx context.Context, id int64, updates map[string]any) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.updateExtraCalls++
	if r.deleted || r.account == nil || r.account.ID != id {
		return ErrAccountNotFound
	}
	if r.account.Extra == nil {
		r.account.Extra = map[string]any{}
	}
	for key, value := range updates {
		r.account.Extra[key] = cloneGrokCatalogMemoryValue(value)
	}
	return nil
}

func (r *grokCatalogMemoryRepo) CompareAndSwapGrokObservedModels(ctx context.Context, id int64, expected, next json.RawMessage) (bool, error) {
	if !r.writesWithoutContextCheck {
		if err := ctx.Err(); err != nil {
			return false, err
		}
	}
	expectedJSON, err := normalizeGrokCatalogMemoryJSON(expected)
	if err != nil {
		return false, err
	}
	nextJSON, err := normalizeGrokCatalogMemoryJSON(next)
	if err != nil {
		return false, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.casCalls++
	if r.conflictsRemaining > 0 {
		r.conflictsRemaining--
		if r.onConflict != nil {
			r.onConflict(r.account)
		}
		if r.afterCAS != nil {
			r.afterCAS()
		}
		return false, nil
	}
	if r.deleted || r.account == nil || r.account.ID != id || r.account.Platform != PlatformGrok {
		if r.afterCAS != nil {
			r.afterCAS()
		}
		return false, nil
	}
	current, err := grokCatalogMemoryCurrentJSON(r.account)
	if err != nil {
		return false, err
	}
	if !grokCatalogMemoryJSONEqual(current, expectedJSON) {
		if r.afterCAS != nil {
			r.afterCAS()
		}
		return false, nil
	}
	var decoded any
	if err := json.Unmarshal(nextJSON, &decoded); err != nil {
		return false, err
	}
	if r.account.Extra == nil {
		r.account.Extra = map[string]any{}
	}
	r.account.Extra[grokObservedModelsExtraKey] = cloneGrokCatalogMemoryValue(decoded)
	if r.afterCAS != nil {
		r.afterCAS()
	}
	return true, nil
}

func grokCatalogMemoryCurrentJSON(account *Account) (json.RawMessage, error) {
	var current any
	if account != nil && account.Extra != nil {
		current = account.Extra[grokObservedModelsExtraKey]
	}
	raw, err := json.Marshal(current)
	if err != nil {
		return nil, err
	}
	return raw, nil
}

func normalizeGrokCatalogMemoryJSON(raw json.RawMessage) (json.RawMessage, error) {
	if len(bytesTrimSpace(raw)) == 0 {
		return json.RawMessage("null"), nil
	}
	if !json.Valid(raw) {
		return nil, errGrokCatalogMemoryInvalidJSON
	}
	return raw, nil
}

func grokCatalogMemoryJSONEqual(left, right json.RawMessage) bool {
	var lhs, rhs any
	if err := json.Unmarshal(left, &lhs); err != nil {
		return false
	}
	if err := json.Unmarshal(right, &rhs); err != nil {
		return false
	}
	return reflect.DeepEqual(lhs, rhs)
}

func cloneGrokCatalogMemoryAccount(account *Account) *Account {
	if account == nil {
		return nil
	}
	cloned := *account
	cloned.Extra = cloneGrokCatalogMemoryMap(account.Extra)
	cloned.Credentials = cloneGrokCatalogMemoryMap(account.Credentials)
	if account.GroupIDs != nil {
		cloned.GroupIDs = append([]int64(nil), account.GroupIDs...)
	}
	if account.RateMultiplier != nil {
		value := *account.RateMultiplier
		cloned.RateMultiplier = &value
	}
	return &cloned
}

func cloneGrokCatalogMemoryMap(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	cloned, ok := cloneGrokCatalogMemoryValue(in).(map[string]any)
	if !ok {
		return map[string]any{}
	}
	return cloned
}

func cloneGrokCatalogMemoryValue(value any) any {
	raw, err := json.Marshal(value)
	if err != nil {
		panic("grok catalog memory value must be json: " + err.Error())
	}
	var cloned any
	if err := json.Unmarshal(raw, &cloned); err != nil {
		panic("grok catalog memory value must be json: " + err.Error())
	}
	return cloned
}

func bytesTrimSpace(raw []byte) []byte {
	start, end := 0, len(raw)
	for start < end && isASCIISpace(raw[start]) {
		start++
	}
	for end > start && isASCIISpace(raw[end-1]) {
		end--
	}
	return raw[start:end]
}

func isASCIISpace(b byte) bool {
	return b == ' ' || b == '\n' || b == '\t' || b == '\r'
}
