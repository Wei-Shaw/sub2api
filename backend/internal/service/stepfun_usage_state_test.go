//go:build unit

package service

import (
	"context"
	"errors"
	"io"
	"maps"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/stretchr/testify/require"
)

type stepFunStateTestRepo struct {
	AccountRepository
	account    *Account
	updates    map[string]any
	attemptSeq int64
}

func (r *stepFunStateTestRepo) GetByID(context.Context, int64) (*Account, error) {
	return r.account, nil
}

func (r *stepFunStateTestRepo) UpdateExtra(_ context.Context, _ int64, updates map[string]any) error {
	r.updates = updates
	return nil
}

func (r *stepFunStateTestRepo) NextStepPlanUsageAttemptSeq(context.Context) (int64, error) {
	r.attemptSeq++
	return r.attemptSeq, nil
}

func (r *stepFunStateTestRepo) UpdateStepPlanExtraIfIdentityUnchanged(
	_ context.Context,
	_ int64,
	_ string,
	_ string,
	updates map[string]any,
	attemptSeq int64,
) error {
	r.updates = updates
	r.attemptSeq = attemptSeq
	if r.account.Extra == nil {
		r.account.Extra = make(map[string]any)
	}
	for key, value := range updates {
		r.account.Extra[key] = value
	}
	return nil
}

type stepFunStateTestUpstream struct {
	statusCode     int
	body           string
	err            error
	beforeResponse func()
}

func (u *stepFunStateTestUpstream) Do(*http.Request, string, int64, int) (*http.Response, error) {
	if u.beforeResponse != nil {
		u.beforeResponse()
	}
	if u.err != nil {
		return nil, u.err
	}
	status := u.statusCode
	if status == 0 {
		status = http.StatusOK
	}
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(u.body)),
	}, nil
}

func (u *stepFunStateTestUpstream) DoWithTLS(*http.Request, string, int64, int, *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(nil, "", 0, 0)
}

func stepFunStateTestAccount(extra map[string]any) *Account {
	return &Account{
		ID: 21, Platform: PlatformStepfun, Type: AccountTypeAPIKey, Status: StatusActive,
		Credentials: map[string]any{
			"account_mode": AccountModeCoding,
			"api_key":      "sk-stepfun",
			"base_url":     DefaultStepFunCodingBaseURL,
		},
		Extra: extra,
	}
}

type stepFunIdentityRaceRepo struct {
	AccountRepository
	account *Account
	updates map[string]any
}

func (r *stepFunIdentityRaceRepo) GetByID(context.Context, int64) (*Account, error) {
	clone := *r.account
	clone.Credentials = maps.Clone(r.account.Credentials)
	clone.Extra = maps.Clone(r.account.Extra)
	return &clone, nil
}

func (r *stepFunIdentityRaceRepo) UpdateExtra(_ context.Context, _ int64, updates map[string]any) error {
	r.updates = updates
	return nil
}

func (r *stepFunIdentityRaceRepo) NextStepPlanUsageAttemptSeq(context.Context) (int64, error) {
	return 1, nil
}

func (r *stepFunIdentityRaceRepo) UpdateStepPlanExtraIfIdentityUnchanged(
	_ context.Context,
	_ int64,
	expectedAPIKey string,
	expectedBaseURL string,
	updates map[string]any,
	_ int64,
) error {
	if r.account.GetCNAPIKey() != expectedAPIKey || r.account.GetOpenAIBaseURL() != expectedBaseURL {
		return ErrCNQuotaIdentityChanged
	}
	r.updates = updates
	return nil
}

type stepFunIdentitySequenceRepo struct {
	AccountRepository
	accounts []*Account
	getCalls int
	updates  map[string]any
}

func cloneStepFunTestAccount(account *Account) *Account {
	clone := *account
	clone.Credentials = maps.Clone(account.Credentials)
	clone.Extra = maps.Clone(account.Extra)
	return &clone
}

func (r *stepFunIdentitySequenceRepo) GetByID(context.Context, int64) (*Account, error) {
	index := r.getCalls
	if index >= len(r.accounts) {
		index = len(r.accounts) - 1
	}
	r.getCalls++
	return cloneStepFunTestAccount(r.accounts[index]), nil
}

func (r *stepFunIdentitySequenceRepo) UpdateExtra(_ context.Context, _ int64, updates map[string]any) error {
	r.updates = updates
	return nil
}

func (r *stepFunIdentitySequenceRepo) NextStepPlanUsageAttemptSeq(context.Context) (int64, error) {
	return 1, nil
}

func (r *stepFunIdentitySequenceRepo) UpdateStepPlanExtraIfIdentityUnchanged(
	_ context.Context,
	_ int64,
	expectedAPIKey string,
	expectedBaseURL string,
	updates map[string]any,
	_ int64,
) error {
	if len(r.accounts) == 0 {
		return ErrCNQuotaIdentityChanged
	}
	current := r.accounts[len(r.accounts)-1]
	if current.GetCNAPIKey() != expectedAPIKey || current.GetOpenAIBaseURL() != expectedBaseURL {
		return ErrCNQuotaIdentityChanged
	}
	r.updates = updates
	return nil
}

// stepFunCASRaceRepo simulates the TOCTOU window this CAS contract closes:
// GetByID keeps returning the original row, but the identity changes before the
// conditional write executes. The old implementation passed its pre-check and
// then wrote through UpdateExtra; the CAS path must surface the conflict.
type stepFunCASRaceRepo struct {
	AccountRepository
	account         *Account
	identityChanged bool
	legacyUpdates   int
	casCalls        int
	updates         map[string]any
}

type stepFunReloadFailureRepo struct {
	AccountRepository
	account  *Account
	getCalls int
}

func (r *stepFunReloadFailureRepo) GetByID(context.Context, int64) (*Account, error) {
	r.getCalls++
	if r.getCalls > 1 {
		return nil, errors.New("read after write failed")
	}
	return cloneStepFunTestAccount(r.account), nil
}

func (r *stepFunReloadFailureRepo) UpdateStepPlanExtraIfIdentityUnchanged(
	context.Context,
	int64,
	string,
	string,
	map[string]any,
	int64,
) error {
	return nil
}

func (r *stepFunReloadFailureRepo) NextStepPlanUsageAttemptSeq(context.Context) (int64, error) {
	return 1, nil
}

func (r *stepFunCASRaceRepo) GetByID(context.Context, int64) (*Account, error) {
	r.identityChanged = true
	return cloneStepFunTestAccount(r.account), nil
}

func (r *stepFunCASRaceRepo) UpdateExtra(_ context.Context, _ int64, updates map[string]any) error {
	r.legacyUpdates++
	r.updates = updates
	return nil
}

func (r *stepFunCASRaceRepo) NextStepPlanUsageAttemptSeq(context.Context) (int64, error) {
	return 1, nil
}

func (r *stepFunCASRaceRepo) UpdateStepPlanExtraIfIdentityUnchanged(
	_ context.Context,
	_ int64,
	expectedAPIKey string,
	expectedBaseURL string,
	updates map[string]any,
	_ int64,
) error {
	r.casCalls++
	if r.identityChanged || r.account.GetCNAPIKey() != expectedAPIKey || r.account.GetOpenAIBaseURL() != expectedBaseURL {
		return ErrCNQuotaIdentityChanged
	}
	r.updates = updates
	return nil
}

func TestStepFunUsageProbePersistsStructuredSnapshotOnSuccess(t *testing.T) {
	repo := &stepFunStateTestRepo{account: stepFunStateTestAccount(nil)}
	upstream := &stepFunStateTestUpstream{body: `{
		"quota": {
			"state": "known",
			"allowed": true,
			"credit_buckets": [
				{"total": "100", "remaining": "70", "used_percent": "30", "expires_at_ms": 1790000000000}
			],
			"windows": []
		},
		"plan": {"state": "known", "name": "Plus"}
	}`}
	svc := NewCNProviderQuotaService(repo, nil, upstream, nil)

	result, err := svc.QueryUsage(context.Background(), repo.account.ID)
	require.NoError(t, err)
	require.True(t, result.Success)

	snapshot, ok := repo.updates[StepFunUsageSnapshotExtraKey].(StepFunUsageSnapshot)
	require.True(t, ok, "snapshot must be persisted as a structured value")
	require.Equal(t, StepFunUsageStatusOK, snapshot.Status)
	require.True(t, snapshot.CredentialValid)
	require.Equal(t, int64(1), repo.attemptSeq)
	require.Equal(t, int64(1), repo.updates[StepFunUsageAttemptSeqExtraKey])
	require.Equal(t, "Plus", snapshot.PlanLevel)
	require.Len(t, snapshot.Tiers, 1)
	require.Equal(t, "monthly", snapshot.Tiers[0].Window)
	require.InDelta(t, 30, snapshot.Tiers[0].UsedPercent, 1e-9)
	require.NotNil(t, snapshot.FetchedAt)
	require.WithinDuration(t, time.Now().UTC(), snapshot.LastAttemptAt, 5*time.Second)
	require.Empty(t, snapshot.LastError)
}

func TestStepFunUsageProbePersistsStructuredSnapshotOnAuthenticationFailure(t *testing.T) {
	repo := &stepFunStateTestRepo{account: stepFunStateTestAccount(nil)}
	upstream := &stepFunStateTestUpstream{
		statusCode: http.StatusUnauthorized,
		body:       `{"error":"invalid key"}`,
	}
	svc := NewCNProviderQuotaService(repo, nil, upstream, nil)

	result, err := svc.QueryUsage(context.Background(), repo.account.ID)
	require.NoError(t, err)
	require.False(t, result.Success)

	snapshot, ok := repo.updates[StepFunUsageSnapshotExtraKey].(StepFunUsageSnapshot)
	require.True(t, ok, "failed attempts must still persist a structured snapshot")
	require.Equal(t, StepFunUsageStatusFailed, snapshot.Status)
	require.False(t, snapshot.CredentialValid)
	require.Equal(t, http.StatusUnauthorized, snapshot.HTTPStatus)
	require.Contains(t, snapshot.LastError, "Authentication failed")
	require.Nil(t, snapshot.FetchedAt)
}

func TestStepFunUsageProbeKeepsLastUsageTiersOnAuthenticationFailure(t *testing.T) {
	oldAttempt := time.Now().UTC().Add(-time.Hour)
	repo := &stepFunStateTestRepo{account: stepFunStateTestAccount(map[string]any{
		StepFunUsageSnapshotExtraKey: StepFunUsageSnapshot{
			Status:          StepFunUsageStatusOK,
			CredentialValid: true,
			PlanLevel:       "Plus",
			Tiers:           []CNQuotaTier{{Window: "monthly", UsedPercent: 30}},
			FetchedAt:       &oldAttempt,
			LastAttemptAt:   oldAttempt,
		},
	})}
	upstream := &stepFunStateTestUpstream{
		statusCode: http.StatusUnauthorized,
		body:       `{"error":"invalid key"}`,
	}
	svc := NewCNProviderQuotaService(repo, nil, upstream, nil)

	_, err := svc.QueryUsage(context.Background(), repo.account.ID)
	require.NoError(t, err)

	snapshot, ok := repo.updates[StepFunUsageSnapshotExtraKey].(StepFunUsageSnapshot)
	require.True(t, ok)
	require.Equal(t, StepFunUsageStatusFailed, snapshot.Status)
	require.Len(t, snapshot.Tiers, 1)
	require.InDelta(t, 30, snapshot.Tiers[0].UsedPercent, 1e-9)
	require.Equal(t, "Plus", snapshot.PlanLevel)
	require.NotNil(t, snapshot.FetchedAt)
	require.Equal(t, oldAttempt, *snapshot.FetchedAt)
}

func TestStepFunUsageProbePersistsHTTPFailureWithoutMarkingCredentialInvalid(t *testing.T) {
	oldAttempt := time.Now().UTC().Add(-time.Hour)
	repo := &stepFunStateTestRepo{account: stepFunStateTestAccount(map[string]any{
		StepFunUsageSnapshotExtraKey: StepFunUsageSnapshot{
			Status:          StepFunUsageStatusOK,
			CredentialValid: true,
			PlanLevel:       "Plus",
			Tiers:           []CNQuotaTier{{Window: "monthly", UsedPercent: 30}},
			FetchedAt:       &oldAttempt,
			LastAttemptAt:   oldAttempt,
		},
	})}
	upstream := &stepFunStateTestUpstream{
		statusCode: http.StatusInternalServerError,
		body:       `{"error":"upstream unavailable"}`,
	}
	svc := NewCNProviderQuotaService(repo, nil, upstream, nil)

	result, err := svc.QueryUsage(context.Background(), repo.account.ID)
	require.NoError(t, err)
	require.False(t, result.Success)
	require.True(t, result.CredentialValid, "HTTP 500 is not an authentication failure")

	snapshot, ok := repo.updates[StepFunUsageSnapshotExtraKey].(StepFunUsageSnapshot)
	require.True(t, ok)
	require.Equal(t, StepFunUsageStatusFailed, snapshot.Status)
	require.True(t, snapshot.CredentialValid)
	require.Equal(t, http.StatusInternalServerError, snapshot.HTTPStatus)
	require.Len(t, snapshot.Tiers, 1)
	require.Equal(t, oldAttempt, *snapshot.FetchedAt)
}

func TestStepFunUsageProbePersistsTransportFailure(t *testing.T) {
	oldAttempt := time.Now().UTC().Add(-time.Hour)
	repo := &stepFunStateTestRepo{account: stepFunStateTestAccount(map[string]any{
		StepFunUsageSnapshotExtraKey: StepFunUsageSnapshot{
			Status:          StepFunUsageStatusOK,
			CredentialValid: true,
			PlanLevel:       "Plus",
			Tiers:           []CNQuotaTier{{Window: "monthly", UsedPercent: 30}},
			FetchedAt:       &oldAttempt,
			LastAttemptAt:   oldAttempt,
		},
	})}
	upstream := &stepFunStateTestUpstream{err: errors.New("dial timeout")}
	svc := NewCNProviderQuotaService(repo, nil, upstream, nil)

	_, err := svc.QueryUsage(context.Background(), repo.account.ID)
	require.ErrorContains(t, err, "upstream request failed")

	snapshot, ok := repo.updates[StepFunUsageSnapshotExtraKey].(StepFunUsageSnapshot)
	require.True(t, ok, "transport failures must persist a structured snapshot")
	require.Equal(t, StepFunUsageStatusFailed, snapshot.Status)
	require.True(t, snapshot.CredentialValid, "transport failures are not credential failures")
	require.Contains(t, snapshot.LastError, "dial timeout")
	require.Len(t, snapshot.Tiers, 1)
	require.Equal(t, oldAttempt, *snapshot.FetchedAt)
}

func TestStepFunUsageProbeURLRejectionDoesNotMarkCredentialInvalid(t *testing.T) {
	repo := &stepFunStateTestRepo{account: stepFunStateTestAccount(nil)}
	upstream := &stepFunStateTestUpstream{}
	svc := NewCNProviderQuotaService(repo, nil, upstream, cnProbeAllowlistConfig("api.kimi.com"))

	_, err := svc.QueryUsage(context.Background(), repo.account.ID)
	require.ErrorContains(t, err, "CN_QUOTA_URL_REJECTED")

	snapshot, ok := repo.updates[StepFunUsageSnapshotExtraKey].(StepFunUsageSnapshot)
	require.True(t, ok)
	require.Equal(t, StepFunUsageStatusFailed, snapshot.Status)
	require.True(t, snapshot.CredentialValid, "URL policy rejection is not an authentication failure")
}

func TestStepFunUsageProbeDoesNotPersistAfterIdentityChangesMidFlight(t *testing.T) {
	repo := &stepFunIdentityRaceRepo{account: stepFunStateTestAccount(nil)}
	upstream := &stepFunStateTestUpstream{
		body: `{
			"quota": {"state": "known", "allowed": true,
				"credit_buckets": [{"total": 100, "remaining": 70, "used_percent": 30}]
			},
			"plan": {"state": "known", "name": "Plus"}
		}`,
		beforeResponse: func() {
			repo.account.Credentials["api_key"] = "sk-new-identity"
		},
	}
	svc := NewCNProviderQuotaService(repo, nil, upstream, nil)

	result, err := svc.QueryUsage(context.Background(), repo.account.ID)
	require.ErrorContains(t, err, "CN_QUOTA_IDENTITY_CHANGED")
	require.Nil(t, result)
	require.Nil(t, repo.updates, "usage from the old key must not be written onto the new identity")
}

func TestStepFunUsageProbeDoesNotPersistFailureAfterIdentityChangesMidFlight(t *testing.T) {
	repo := &stepFunIdentityRaceRepo{account: stepFunStateTestAccount(nil)}
	upstream := &stepFunStateTestUpstream{
		statusCode: http.StatusUnauthorized,
		body:       `{"error":"invalid key"}`,
		beforeResponse: func() {
			repo.account.Credentials["api_key"] = "sk-new-identity"
		},
	}
	svc := NewCNProviderQuotaService(repo, nil, upstream, nil)

	result, err := svc.QueryUsage(context.Background(), repo.account.ID)
	require.ErrorContains(t, err, "CN_QUOTA_IDENTITY_CHANGED")
	require.Nil(t, result)
	require.Nil(t, repo.updates, "failure from the old key must not be written onto the new identity")
}

func TestStepFunUsageProbeSuccessUsesIdentityCASAtWriteTime(t *testing.T) {
	repo := &stepFunCASRaceRepo{account: stepFunStateTestAccount(nil)}
	upstream := &stepFunStateTestUpstream{body: `{
		"quota": {"state": "known", "allowed": true,
			"credit_buckets": [{"total": 100, "remaining": 70, "used_percent": 30}]
		},
		"plan": {"state": "known", "name": "Plus"}
	}`}
	svc := NewCNProviderQuotaService(repo, nil, upstream, nil)

	result, err := svc.QueryUsage(context.Background(), repo.account.ID)
	require.ErrorContains(t, err, "CN_QUOTA_IDENTITY_CHANGED")
	require.Nil(t, result)
	require.Equal(t, 1, repo.casCalls)
	require.Zero(t, repo.legacyUpdates)
	require.Nil(t, repo.updates)
}

func TestStepFunUsageProbeFailureUsesIdentityCASAtWriteTime(t *testing.T) {
	repo := &stepFunCASRaceRepo{account: stepFunStateTestAccount(nil)}
	upstream := &stepFunStateTestUpstream{
		statusCode: http.StatusUnauthorized,
		body:       `{"error":"invalid key"}`,
	}
	svc := NewCNProviderQuotaService(repo, nil, upstream, nil)

	result, err := svc.QueryUsage(context.Background(), repo.account.ID)
	require.ErrorContains(t, err, "CN_QUOTA_IDENTITY_CHANGED")
	require.Nil(t, result)
	require.Equal(t, 1, repo.casCalls)
	require.Zero(t, repo.legacyUpdates)
	require.Nil(t, repo.updates)
}

func TestSetStepPlanUsageAutoRefreshRejectsIdentityChange(t *testing.T) {
	oldAccount := stepFunStateTestAccount(nil)
	newAccount := stepFunStateTestAccount(nil)
	newAccount.Credentials["api_key"] = "sk-new-identity"
	repo := &stepFunIdentitySequenceRepo{accounts: []*Account{oldAccount, newAccount}}
	svc := &CNProviderQuotaService{accountRepo: repo}

	state, err := svc.SetStepPlanUsageAutoRefresh(context.Background(), oldAccount.ID, false)
	require.ErrorContains(t, err, "CN_QUOTA_IDENTITY_CHANGED")
	require.Nil(t, state)
	require.Nil(t, repo.updates)
}

func TestSetStepPlanUsageAutoRefreshUsesIdentityCASAtWriteTime(t *testing.T) {
	repo := &stepFunCASRaceRepo{account: stepFunStateTestAccount(nil)}
	svc := &CNProviderQuotaService{accountRepo: repo}

	state, err := svc.SetStepPlanUsageAutoRefresh(context.Background(), repo.account.ID, false)
	require.ErrorContains(t, err, "CN_QUOTA_IDENTITY_CHANGED")
	require.Nil(t, state)
	require.Equal(t, 1, repo.casCalls)
	require.Zero(t, repo.legacyUpdates)
	require.Nil(t, repo.updates)
}

func TestStepFunUsageProbeClearsStaleFlatWindowsOnSuccessfulRefresh(t *testing.T) {
	repo := &stepFunStateTestRepo{account: stepFunStateTestAccount(map[string]any{
		"stepfun_5h_used_percent":      80.0,
		"stepfun_5h_reset_at":          "2026-10-21T04:00:00Z",
		"stepfun_weekly_used_percent":  70.0,
		"stepfun_weekly_reset_at":      "2026-10-27T00:00:00Z",
		"stepfun_monthly_used_percent": 95.0,
		"stepfun_monthly_reset_at":     "2026-11-20T00:00:00Z",
	})}
	upstream := &stepFunStateTestUpstream{body: `{
		"quota": {
			"state": "known",
			"allowed": true,
			"windows": [{"name": "weekly", "used_percent": "20"}]
		},
		"plan": {"state": "known", "name": "Plus"}
	}`}
	svc := NewCNProviderQuotaService(repo, nil, upstream, nil)

	result, err := svc.QueryUsage(context.Background(), repo.account.ID)
	require.NoError(t, err)
	require.True(t, result.Success)
	require.Equal(t, 20.0, repo.updates["stepfun_weekly_used_percent"])

	for _, key := range []string{
		"stepfun_5h_used_percent",
		"stepfun_5h_reset_at",
		"stepfun_weekly_reset_at",
		"stepfun_monthly_used_percent",
		"stepfun_monthly_reset_at",
	} {
		require.Contains(t, repo.updates, key, "successful refresh must explicitly clear windows absent from the latest response")
		require.Nil(t, repo.updates[key])
	}
}

func TestStepFunUsageProbePersistsUnavailableSnapshotForInactivePlan(t *testing.T) {
	repo := &stepFunStateTestRepo{account: stepFunStateTestAccount(nil)}
	upstream := &stepFunStateTestUpstream{body: `{
		"quota": {"state": "unknown", "reason": "Step Plan is not activated"},
		"plan": {"state": "unknown"}
	}`}
	svc := NewCNProviderQuotaService(repo, nil, upstream, nil)

	result, err := svc.QueryUsage(context.Background(), repo.account.ID)
	require.NoError(t, err)
	require.False(t, result.Success)

	snapshot, ok := repo.updates[StepFunUsageSnapshotExtraKey].(StepFunUsageSnapshot)
	require.True(t, ok)
	require.Equal(t, StepFunUsageStatusUnavailable, snapshot.Status)
	require.True(t, snapshot.CredentialValid)
	require.Equal(t, "Step Plan is not activated", snapshot.LastError)
	require.Nil(t, snapshot.FetchedAt)
}

func TestStepPlanUsageStateReadsStructuredSnapshotAndDefaultsAutoRefreshOn(t *testing.T) {
	repo := &stepFunStateTestRepo{account: stepFunStateTestAccount(map[string]any{
		StepFunUsageSnapshotExtraKey: map[string]any{
			"status":           StepFunUsageStatusOK,
			"credential_valid": true,
			"plan_level":       "Plus",
			"tiers": []any{
				map[string]any{"window": "monthly", "used_percent": float64(42), "reset_at": "2026-11-20T09:00:00Z"},
			},
			"fetched_at":      "2026-10-21T03:00:00Z",
			"last_attempt_at": "2026-10-21T03:00:00Z",
		},
	})}
	svc := &CNProviderQuotaService{accountRepo: repo}

	state, err := svc.GetStepPlanUsageState(context.Background(), repo.account.ID)
	require.NoError(t, err)
	require.True(t, state.Eligible)
	require.True(t, state.AutoRefreshEnabled)
	require.NotNil(t, state.Snapshot)
	require.Equal(t, StepFunUsageStatusOK, state.Snapshot.Status)
	require.Equal(t, "Plus", state.Snapshot.PlanLevel)
	require.Len(t, state.Snapshot.Tiers, 1)
	require.InDelta(t, 42, state.Snapshot.Tiers[0].UsedPercent, 1e-9)
}

func TestSetStepPlanUsageAutoRefreshPersistsAccountFlag(t *testing.T) {
	repo := &stepFunStateTestRepo{account: stepFunStateTestAccount(nil)}
	svc := &CNProviderQuotaService{accountRepo: repo}

	state, err := svc.SetStepPlanUsageAutoRefresh(context.Background(), repo.account.ID, false)
	require.NoError(t, err)
	require.False(t, state.AutoRefreshEnabled)
	require.Equal(t, false, repo.updates[StepFunUsageAutoRefreshExtraKey])
	// 纯开关写入不得占用 attempt 序号：序号只属于用量探测结果。
	require.Zero(t, repo.attemptSeq)
}

func TestSetStepPlanUsageAutoRefreshReturnsLatestSnapshotAfterWrite(t *testing.T) {
	oldAccount := stepFunStateTestAccount(nil)
	latestAccount := stepFunStateTestAccount(map[string]any{
		StepFunUsageAutoRefreshExtraKey: false,
		StepFunUsageSnapshotExtraKey: map[string]any{
			"status":           StepFunUsageStatusOK,
			"credential_valid": true,
			"plan_level":       "Plus",
			"tiers": []any{
				map[string]any{"window": "monthly", "used_percent": float64(55)},
			},
			"last_attempt_at": "2026-10-21T03:00:00Z",
		},
	})
	repo := &stepFunIdentitySequenceRepo{accounts: []*Account{oldAccount, latestAccount}}
	svc := &CNProviderQuotaService{accountRepo: repo}

	state, err := svc.SetStepPlanUsageAutoRefresh(context.Background(), oldAccount.ID, false)
	require.NoError(t, err)
	require.False(t, state.AutoRefreshEnabled)
	require.NotNil(t, state.Snapshot)
	require.Len(t, state.Snapshot.Tiers, 1)
	require.InDelta(t, 55, state.Snapshot.Tiers[0].UsedPercent, 1e-9)
}

func TestSetStepPlanUsageAutoRefreshKeepsSuccessWhenReloadFails(t *testing.T) {
	repo := &stepFunReloadFailureRepo{account: stepFunStateTestAccount(nil)}
	svc := &CNProviderQuotaService{accountRepo: repo}

	state, err := svc.SetStepPlanUsageAutoRefresh(context.Background(), repo.account.ID, false)
	require.NoError(t, err)
	require.Equal(t, repo.account.ID, state.AccountID)
	require.True(t, state.Eligible)
	require.False(t, state.AutoRefreshEnabled)
	require.Nil(t, state.Snapshot)
}

func TestStepPlanUsageStateFallsBackToLegacyFlatSnapshot(t *testing.T) {
	repo := &stepFunStateTestRepo{account: stepFunStateTestAccount(map[string]any{
		"stepfun_monthly_used_percent": float64(37),
		"stepfun_monthly_reset_at":     "2026-11-20T09:00:00Z",
		"stepfun_usage_updated_at":     "2026-10-21T03:00:00Z",
	})}
	svc := &CNProviderQuotaService{accountRepo: repo}

	state, err := svc.GetStepPlanUsageState(context.Background(), repo.account.ID)
	require.NoError(t, err)
	require.NotNil(t, state.Snapshot)
	require.Equal(t, StepFunUsageStatusOK, state.Snapshot.Status)
	require.Len(t, state.Snapshot.Tiers, 1)
	require.Equal(t, "monthly", state.Snapshot.Tiers[0].Window)
	require.InDelta(t, 37, state.Snapshot.Tiers[0].UsedPercent, 1e-9)
	require.NotNil(t, state.Snapshot.FetchedAt)
}
