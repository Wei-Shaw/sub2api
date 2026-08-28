package service

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"testing"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type expiryTargetAccountRepo struct {
	AccountRepository
	mu      sync.Mutex
	account *Account
}

func (r *expiryTargetAccountRepo) GetByID(context.Context, int64) (*Account, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	copy := *r.account
	copy.Extra = cloneOpenAIAutoResetExtra(r.account.Extra)
	return &copy, nil
}

func (r *expiryTargetAccountRepo) UpdateExtra(_ context.Context, _ int64, updates map[string]any) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.applyExtra(updates)
	return nil
}

func (r *expiryTargetAccountRepo) CompareAndSwapExtra(_ context.Context, _ int64, key string, expected any, updates map[string]any) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var current any
	if r.account.Extra != nil {
		current = r.account.Extra[key]
	}
	currentJSON, _ := json.Marshal(current)
	expectedJSON, _ := json.Marshal(expected)
	if string(currentJSON) != string(expectedJSON) {
		return false, nil
	}
	r.applyExtra(updates)
	return true, nil
}

func (r *expiryTargetAccountRepo) Update(_ context.Context, account *Account) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	copy := *account
	copy.Extra = cloneOpenAIAutoResetExtra(account.Extra)
	r.account = &copy
	return nil
}

func (r *expiryTargetAccountRepo) applyExtra(updates map[string]any) {
	if r.account.Extra == nil {
		r.account.Extra = make(map[string]any)
	}
	for key, value := range updates {
		r.account.Extra[key] = value
	}
}

func TestOpenAIQuotaService_SetAndCancelResetCreditExpiryTarget(t *testing.T) {
	expiresAt := time.Date(2099, time.July, 3, 4, 5, 6, 123456789, time.UTC).Format(time.RFC3339Nano)
	creditID := "upstream-credit-id"
	otherCreditID := "another-upstream-credit-id"
	account := &Account{
		ID: 201, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive,
		Extra: map[string]any{
			openaiQuotaResetCreditsKey: OpenAIRateLimitResetCredits{
				AvailableCount: 2,
				Credits: []OpenAIRateLimitResetCreditDetail{
					{ID: creditID, ExpiresAt: expiresAt},
					{ID: otherCreditID, ExpiresAt: expiresAt},
				},
			},
		},
	}
	repo := &expiryTargetAccountRepo{account: account}
	svc := &OpenAIQuotaService{accountRepo: repo}

	for _, leadTime := range []int{
		OpenAIResetCreditExpiryTargetMinLeadTimeMinutes - 1,
		OpenAIResetCreditExpiryTargetMaxLeadTimeMinutes + 1,
	} {
		_, err := svc.SetResetCreditExpiryTarget(context.Background(), account.ID, creditID, leadTime)
		require.Equal(t, "OPENAI_RESET_CREDIT_LEAD_TIME_INVALID", infraerrors.Reason(err))
	}

	_, err := svc.SetResetCreditExpiryTarget(context.Background(), account.ID, "missing-credit", 30)
	require.Equal(t, "OPENAI_RESET_CREDIT_TARGET_UNAVAILABLE", infraerrors.Reason(err))

	updated, err := svc.SetResetCreditExpiryTarget(context.Background(), account.ID, creditID, 30)
	require.NoError(t, err)
	target := ResolveOpenAIResetCreditExpiryTarget(updated)
	require.NotNil(t, target)
	require.NotEmpty(t, target.PlanID)
	require.NoError(t, uuid.Validate(target.PlanID))
	require.Equal(t, creditID, target.CreditID)
	require.Equal(t, expiresAt, target.ExpiresAt)
	require.Equal(t, 30, target.LeadTimeMinutes)
	encoded, err := json.Marshal(updated.Extra)
	require.NoError(t, err)
	require.Contains(t, string(encoded), creditID)

	firstPlanID := target.PlanID
	updated, err = svc.SetResetCreditExpiryTarget(context.Background(), account.ID, creditID, 60)
	require.NoError(t, err)
	target = ResolveOpenAIResetCreditExpiryTarget(updated)
	require.Equal(t, 60, target.LeadTimeMinutes)
	require.NotEqual(t, firstPlanID, target.PlanID)

	_, err = svc.SetResetCreditExpiryTarget(context.Background(), account.ID, otherCreditID, 30)
	require.Equal(t, http.StatusConflict, infraerrors.Code(err))
	require.Equal(t, "OPENAI_RESET_CREDIT_EXPIRY_TARGET_EXISTS", infraerrors.Reason(err))
	stored, err := repo.GetByID(context.Background(), account.ID)
	require.NoError(t, err)
	require.Equal(t, creditID, ResolveOpenAIResetCreditExpiryTarget(stored).CreditID)

	require.NoError(t, repo.UpdateExtra(context.Background(), account.ID, map[string]any{
		OpenAIAutoResetCreditStateExtraKey: OpenAIAutoResetCreditState{
			Status: OpenAIAutoResetStatusResetting, TriggerReason: OpenAIAutoResetTriggerReasonExpiryTarget,
			TriggerWindow: "5h", AvailableCount: 1, ErrorCode: "pending",
			AttemptCycleHash: "cycle", AttemptCreditHash: "credit",
		},
	}))

	canceled, err := svc.CancelResetCreditExpiryTarget(context.Background(), account.ID)
	require.NoError(t, err)
	require.Nil(t, ResolveOpenAIResetCreditExpiryTarget(canceled))
	state := openAIAutoResetStateFromExtra(canceled.Extra)
	require.NotNil(t, state)
	require.Equal(t, OpenAIAutoResetStatusAvailable, state.Status)
	require.Empty(t, state.TriggerReason)
	require.Empty(t, state.TriggerWindow)
	require.Empty(t, state.ErrorCode)
	require.Empty(t, state.AttemptCycleHash)
	require.Empty(t, state.AttemptCreditHash)
}

func TestOpenAIQuotaAutoResetService_OldPlanCannotClearReplacement(t *testing.T) {
	now := time.Now().UTC()
	oldTarget := &OpenAIResetCreditExpiryTarget{
		PlanID: uuid.NewString(), CreditID: "old-credit",
		ExpiresAt: now.Add(time.Hour).Format(time.RFC3339), LeadTimeMinutes: 30,
	}
	replacement := OpenAIResetCreditExpiryTarget{
		PlanID: uuid.NewString(), CreditID: "replacement-credit",
		ExpiresAt: now.Add(2 * time.Hour).Format(time.RFC3339), LeadTimeMinutes: 30,
	}
	existingState := OpenAIAutoResetCreditState{Status: OpenAIAutoResetStatusAvailable, AvailableCount: 2}
	repo := &expiryTargetAccountRepo{account: &Account{
		ID: 202, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive,
		Extra: map[string]any{
			OpenAIAutoResetCreditExpiryTargetExtraKey: replacement,
			OpenAIAutoResetCreditStateExtraKey:        existingState,
		},
	}}
	svc := &OpenAIQuotaAutoResetService{accountRepo: repo}

	err := svc.finishOpenAIAutoResetExpiryTarget(context.Background(), 202, oldTarget, nil, "STALE_WORKER", now)
	require.NoError(t, err)
	stored, err := repo.GetByID(context.Background(), 202)
	require.NoError(t, err)
	require.Equal(t, replacement.PlanID, ResolveOpenAIResetCreditExpiryTarget(stored).PlanID)
	require.Equal(t, &existingState, openAIAutoResetStateFromExtra(stored.Extra))
}

func TestAdminAccountUpdatesPreserveManagedExpiryTarget(t *testing.T) {
	current := OpenAIResetCreditExpiryTarget{
		PlanID: uuid.NewString(), CreditID: "current-credit",
		ExpiresAt: time.Now().UTC().Add(time.Hour).Format(time.RFC3339), LeadTimeMinutes: 30,
	}
	repo := &expiryTargetAccountRepo{account: &Account{
		ID: 203, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive,
		Extra: map[string]any{OpenAIAutoResetCreditExpiryTargetExtraKey: current, "custom": "old"},
	}}
	svc := &adminServiceImpl{accountRepo: repo}

	updated, err := svc.UpdateAccount(context.Background(), 203, &UpdateAccountInput{Extra: map[string]any{
		OpenAIAutoResetCreditExpiryTargetExtraKey: map[string]any{"credit_id": "forged"},
		"custom": "new",
	}})
	require.NoError(t, err)
	require.Equal(t, "new", updated.Extra["custom"])
	require.Equal(t, &current, ResolveOpenAIResetCreditExpiryTarget(updated))

	require.NoError(t, svc.UpdateAccountExtra(context.Background(), 203, map[string]any{
		OpenAIAutoResetCreditExpiryTargetExtraKey: map[string]any{"credit_id": "forged-again"},
		"another": "value",
	}))
	stored, err := repo.GetByID(context.Background(), 203)
	require.NoError(t, err)
	require.Equal(t, &current, ResolveOpenAIResetCreditExpiryTarget(stored))
	require.Equal(t, "value", stored.Extra["another"])
}
