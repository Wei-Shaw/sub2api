package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func modelAutoSyncAccount(mapping map[string]string) *Account {
	return &Account{
		ID: 17, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive,
		Credentials: map[string]any{"api_key": "sk-test", "base_url": "https://models.example/v1", "model_mapping": mapping},
		Extra:       map[string]any{OpenAIModelAutoSyncEnabledExtraKey: true},
	}
}

func TestOpenAIModelAutoSyncReconcilePreservesManualMappings(t *testing.T) {
	now := time.Now()
	account := modelAutoSyncAccount(map[string]string{"manual": "manual", "alias": "upstream", "gpt-*": "routed"})
	before := stringMappingFromRaw(account.Credentials["model_mapping"])
	mapping, state := reconcileOpenAIModels(account, []string{"gpt-new", "new-model", "alias"}, OpenAIModelAutoSyncState{}, now)
	require.Equal(t, map[string]string{"manual": "manual", "alias": "upstream", "gpt-*": "routed", "new-model": "new-model"}, mapping)
	require.Equal(t, []string{"new-model"}, state.ManagedModels)
	require.Equal(t, before, stringMappingFromRaw(account.Credentials["model_mapping"]), "shared account maps must not be mutated")
	require.Equal(t, now.Add(time.Hour).Unix(), state.NextSyncUnix)
}

func TestOpenAIModelAutoSyncRetiresOnlyAfterTwoSuccessfulMisses(t *testing.T) {
	now := time.Now()
	account := modelAutoSyncAccount(map[string]string{"old": "old", "manual": "manual"})
	previous := OpenAIModelAutoSyncState{ManagedModels: []string{"old"}}
	mapping, first := reconcileOpenAIModels(account, []string{"new"}, previous, now)
	require.Equal(t, "old", mapping["old"])
	require.Equal(t, []string{"old"}, first.MissingModels)
	account.Credentials["model_mapping"] = mapping
	mapping, second := reconcileOpenAIModels(account, []string{"new"}, first, now.Add(time.Hour))
	require.NotContains(t, mapping, "old")
	require.Equal(t, "manual", mapping["manual"])
	require.Equal(t, []string{"new"}, second.ManagedModels)

	// Errors between successes reset the removal counter.
	failed := failedOpenAIModelSync(first, now, "upstream_error")
	_, afterError := reconcileOpenAIModels(account, []string{"new"}, failed, now.Add(time.Hour))
	require.Contains(t, afterError.ManagedModels, "old")
	require.Equal(t, []string{"old"}, afterError.MissingModels)
}

func TestOpenAIModelAutoSyncRelinquishesManualEdits(t *testing.T) {
	account := modelAutoSyncAccount(map[string]string{"rewritten": "replacement"})
	previous := OpenAIModelAutoSyncState{ManagedModels: []string{"deleted", "rewritten"}}
	mapping, state := reconcileOpenAIModels(account, []string{"deleted", "rewritten", "new"}, previous, time.Now())
	require.NotContains(t, mapping, "deleted")
	require.Equal(t, "replacement", mapping["rewritten"])
	require.Equal(t, []string{"new"}, state.ManagedModels)
	require.Equal(t, []string{"deleted", "rewritten"}, state.ExcludedModels)
	account.Credentials["model_mapping"] = mapping
	mapping, _ = reconcileOpenAIModels(account, []string{"deleted", "rewritten", "new"}, state, time.Now())
	require.NotContains(t, mapping, "deleted")
}

func TestOpenAIModelAutoSyncNeverEmptiesMapping(t *testing.T) {
	previous := OpenAIModelAutoSyncState{ManagedModels: []string{"old"}, MissingModels: []string{"old"}, ExcludedModels: []string{"new"}}
	account := modelAutoSyncAccount(map[string]string{"old": "old"})
	mapping, state := reconcileOpenAIModels(account, []string{"new"}, previous, time.Now())
	require.Nil(t, mapping)
	require.Equal(t, "empty_mapping", state.ErrorCode)
	require.Equal(t, previous.ManagedModels, state.ManagedModels)
	for _, models := range [][]string{nil, {"*"}, {"valid", "bad\nmodel"}} {
		mapping, state = reconcileOpenAIModels(account, models, previous, time.Now())
		require.Nil(t, mapping)
		require.Equal(t, "error", state.Status)
	}
}

func TestOpenAIModelAutoSyncConfiguration(t *testing.T) {
	for _, accountType := range []string{AccountTypeOAuth, AccountTypeAPIKey} {
		extra, err := normalizeOpenAIModelAutoSyncExtra(PlatformOpenAI, accountType, false, map[string]any{
			OpenAIModelAutoSyncEnabledExtraKey: true, OpenAIModelAutoSyncStateExtraKey: "injected", "note": "keep",
		})
		require.NoError(t, err)
		require.Equal(t, true, extra[OpenAIModelAutoSyncEnabledExtraKey])
		require.NotContains(t, extra, OpenAIModelAutoSyncStateExtraKey)
		require.Equal(t, "keep", extra["note"])
	}
	_, err := normalizeOpenAIModelAutoSyncExtra(PlatformOpenAI, AccountTypeOAuth, true, map[string]any{OpenAIModelAutoSyncEnabledExtraKey: true})
	require.Error(t, err)
	_, err = normalizeOpenAIModelAutoSyncExtra(PlatformAnthropic, AccountTypeAPIKey, false, map[string]any{OpenAIModelAutoSyncEnabledExtraKey: true})
	require.Error(t, err)
	_, err = normalizeOpenAIModelAutoSyncExtra(PlatformOpenAI, AccountTypeOAuth, false, map[string]any{OpenAIModelAutoSyncEnabledExtraKey: "true"})
	require.Error(t, err)
	require.False(t, openAIModelAutoSyncEnabled(&Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}))
}

type modelAutoSyncRepo struct {
	account *Account
	commits int
	commit  func(*Account, map[string]string, *OpenAIModelAutoSyncState) error
}

func (r *modelAutoSyncRepo) GetByID(context.Context, int64) (*Account, error) {
	copy := *r.account
	return &copy, nil
}

func (r *modelAutoSyncRepo) ListDueOpenAIModelSyncAccounts(context.Context, time.Time, int) ([]Account, error) {
	return []Account{*r.account}, nil
}

func (r *modelAutoSyncRepo) CommitOpenAIModelSync(_ context.Context, expected *Account, mapping map[string]string, state *OpenAIModelAutoSyncState) error {
	if r.commit != nil {
		return r.commit(expected, mapping, state)
	}
	if !reflect.DeepEqual(expected.Credentials, r.account.Credentials) || !openAIModelAutoSyncEnabled(r.account) {
		return ErrOpenAIModelAutoSyncChanged
	}
	r.commits++
	if mapping != nil {
		r.account.Credentials["model_mapping"] = mapping
	}
	r.account.Extra[OpenAIModelAutoSyncStateExtraKey] = *state
	return nil
}

func TestOpenAIModelAutoSyncWorkerCadenceAndFailure(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	repo := &modelAutoSyncRepo{account: modelAutoSyncAccount(nil)}
	svc := NewOpenAIModelAutoSyncService(repo, nil, nil)
	defer svc.Stop()
	svc.now = func() time.Time { return now }
	fetches := 0
	svc.fetchModels = func(context.Context, *Account) ([]string, error) {
		fetches++
		if fetches == 1 {
			return []string{"new-model"}, nil
		}
		return nil, errors.New("private upstream response token=secret")
	}
	require.NoError(t, svc.RunDue(context.Background()))
	require.NoError(t, svc.RunDue(context.Background()))
	require.Equal(t, 1, fetches)
	require.Equal(t, 1, repo.commits)
	now = now.Add(time.Hour)
	require.NoError(t, svc.RunDue(context.Background()))
	require.Equal(t, map[string]string{"new-model": "new-model"}, repo.account.Credentials["model_mapping"])
	state := decodeOpenAIModelAutoSyncState(repo.account.Extra)
	require.Equal(t, "upstream_error", state.ErrorCode)
	require.Equal(t, now.Add(-time.Hour).UTC().Format(time.RFC3339), state.LastSuccessAt)
	require.Equal(t, 2, repo.commits)
}

func TestOpenAIModelAutoSyncRejectsInFlightConfigurationChange(t *testing.T) {
	repo := &modelAutoSyncRepo{account: modelAutoSyncAccount(nil)}
	svc := NewOpenAIModelAutoSyncService(repo, nil, nil)
	defer svc.Stop()
	svc.fetchModels = func(context.Context, *Account) ([]string, error) {
		repo.account.Extra[OpenAIModelAutoSyncEnabledExtraKey] = false
		return []string{"new"}, nil
	}
	require.ErrorIs(t, svc.syncAccount(context.Background(), 17), ErrOpenAIModelAutoSyncChanged)
	require.Zero(t, repo.commits)
}

func TestOpenAIModelAutoSyncStopCancelsDiscovery(t *testing.T) {
	repo := &modelAutoSyncRepo{account: modelAutoSyncAccount(nil)}
	svc := NewOpenAIModelAutoSyncService(repo, nil, nil)
	started := make(chan struct{})
	svc.fetchModels = func(ctx context.Context, _ *Account) ([]string, error) {
		close(started)
		<-ctx.Done()
		return nil, ctx.Err()
	}
	svc.Start()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("runner did not start")
	}
	svc.Stop()
	svc.Stop()
	require.Zero(t, repo.commits)
}

func TestOpenAIModelAutoSyncOAuthRefreshesBeforeDiscovery(t *testing.T) {
	repo := &modelAutoSyncRepo{account: modelAutoSyncAccount(nil)}
	repo.account.Type = AccountTypeOAuth
	repo.account.Credentials = map[string]any{"access_token": "old"}
	svc := NewOpenAIModelAutoSyncService(repo, nil, nil)
	defer svc.Stop()
	svc.refreshToken = func(context.Context, *Account) (string, error) {
		repo.account.Credentials = map[string]any{"access_token": "new"}
		return "new", nil
	}
	svc.fetchModels = func(_ context.Context, account *Account) ([]string, error) {
		require.Equal(t, "new", account.GetOpenAIAccessToken())
		return []string{"new-model"}, nil
	}
	require.NoError(t, svc.syncAccount(context.Background(), 17))
	require.Equal(t, "new", repo.account.Credentials["access_token"])
	require.Equal(t, 1, repo.commits)
}

func TestOpenAIModelAutoSyncLiveDiscovery(t *testing.T) {
	for _, accountType := range []string{AccountTypeOAuth, AccountTypeAPIKey} {
		t.Run(accountType, func(t *testing.T) {
			body := `{"data":[{"id":"new-model"}]}`
			if accountType == AccountTypeOAuth {
				body = `{"models":[{"slug":"new-model"},{"slug":"hidden","visibility":"hide"},{"slug":"subscription-only","supported_in_api":false},{"slug":"gpt-5.5"}]}`
			}
			upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}}
			svc := &AccountTestService{httpUpstream: upstream, cfg: upstreamModelSyncTestConfig()}
			account := modelAutoSyncAccount(nil)
			account.Type = accountType
			account.Credentials["access_token"] = "test-token"
			models, err := svc.fetchOpenAIModelsForAutoSync(context.Background(), account)
			require.NoError(t, err)
			if accountType == AccountTypeOAuth {
				require.Equal(t, []string{"new-model", "subscription-only"}, models)
				require.Equal(t, "chatgpt.com", upstream.lastReq.URL.Host)
				require.Equal(t, "Bearer test-token", upstream.lastReq.Header.Get("Authorization"))
			} else {
				require.Equal(t, []string{"new-model"}, models)
				require.Equal(t, "https://models.example/v1/models", upstream.lastReq.URL.String())
			}
		})
	}
}

func TestOpenAIModelAutoSyncRejectsPartialOrFailedDiscovery(t *testing.T) {
	for _, body := range []string{
		`{"data":[{"id":"new"}],"has_more":true}`,
		`{"data":[{"id":"new"}],"next_page":"cursor"}`,
		`{"data":[{"id":"new"}],"nextPageToken":"cursor"}`,
	} {
		_, err := filterOpenAIModelAutoSyncResponse(modelAutoSyncAccount(nil), []string{"new"}, []byte(body))
		require.Error(t, err)
	}
	upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: http.StatusNotFound, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"private":"do not persist"}`))}}
	svc := &AccountTestService{httpUpstream: upstream, cfg: upstreamModelSyncTestConfig()}
	models, err := svc.fetchOpenAIModelsForAutoSync(context.Background(), modelAutoSyncAccount(map[string]string{"manual": "manual"}))
	require.Error(t, err)
	require.Nil(t, models, "a configured fallback cannot be evidence of upstream availability")
}
