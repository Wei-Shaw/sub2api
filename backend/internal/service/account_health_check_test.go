package service

import (
	"context"
	"testing"
	"time"
)

type fakeBatchTaskRepo struct {
	nextID  int64
	created []*AccountBatchTestTask
}

func (r *fakeBatchTaskRepo) Create(_ context.Context, task *AccountBatchTestTask) (*AccountBatchTestTask, error) {
	r.nextID++
	out := *task
	out.ID = r.nextID
	out.CreatedAt = time.Now()
	out.UpdatedAt = out.CreatedAt
	r.created = append(r.created, &out)
	return &out, nil
}

func (r *fakeBatchTaskRepo) GetByID(_ context.Context, id int64) (*AccountBatchTestTask, error) {
	for _, task := range r.created {
		if task.ID == id {
			out := *task
			return &out, nil
		}
	}
	return nil, ErrAccountNotFound
}

func (r *fakeBatchTaskRepo) List(_ context.Context, _ int, _ int) ([]*AccountBatchTestTask, int64, error) {
	return r.created, int64(len(r.created)), nil
}

func (r *fakeBatchTaskRepo) MarkRunning(_ context.Context, id int64, startedAt time.Time) error {
	for _, task := range r.created {
		if task.ID == id {
			task.Status = AccountBatchTestStatusRunning
			task.StartedAt = &startedAt
		}
	}
	return nil
}

func (r *fakeBatchTaskRepo) IncrementProgress(_ context.Context, id int64, status string, triggeredDisabled bool) error {
	for _, task := range r.created {
		if task.ID == id {
			task.CompletedCount++
			if status == AccountBatchTestResultSuccess {
				task.SuccessCount++
			} else {
				task.FailedCount++
			}
			if triggeredDisabled {
				task.DeactivatedCount++
			}
		}
	}
	return nil
}

func (r *fakeBatchTaskRepo) Finish(_ context.Context, id int64, status string, errMsg string, finishedAt time.Time) error {
	for _, task := range r.created {
		if task.ID == id {
			task.Status = status
			task.ErrorMessage = errMsg
			task.FinishedAt = &finishedAt
		}
	}
	return nil
}

type fakeBatchResultRepo struct {
	nextID  int64
	results []*AccountBatchTestResult
}

func (r *fakeBatchResultRepo) Create(_ context.Context, result *AccountBatchTestResult) (*AccountBatchTestResult, error) {
	r.nextID++
	out := *result
	out.ID = r.nextID
	out.CreatedAt = time.Now()
	r.results = append(r.results, &out)
	return &out, nil
}

func (r *fakeBatchResultRepo) ListByTaskID(_ context.Context, taskID int64, _ int, _ int) ([]*AccountBatchTestResult, int64, error) {
	var out []*AccountBatchTestResult
	for _, result := range r.results {
		if result.TaskID == taskID {
			out = append(out, result)
		}
	}
	return out, int64(len(out)), nil
}

type fakeHealthAccountRepo struct {
	accounts   map[int64]*Account
	recorded   []recordedHealthResult
	bulkUpdate int64
}

type recordedHealthResult struct {
	accountID int64
	status    string
	streak    int
	disable   bool
}

func (r *fakeHealthAccountRepo) GetAccountsByIDs(_ context.Context, ids []int64) ([]*Account, error) {
	var out []*Account
	for _, id := range ids {
		if account := r.accounts[id]; account != nil {
			copied := *account
			out = append(out, &copied)
		}
	}
	return out, nil
}

func (r *fakeHealthAccountRepo) ListScheduledCandidates(_ context.Context) ([]*Account, error) {
	var out []*Account
	for _, account := range r.accounts {
		if account.Platform == PlatformOpenAI &&
			account.Status == StatusActive &&
			account.Schedulable &&
			account.HealthCheckEnabled &&
			!account.HealthCheckProtected {
			copied := *account
			out = append(out, &copied)
		}
	}
	return out, nil
}

func (r *fakeHealthAccountRepo) UpdateSettings(_ context.Context, id int64, settings AccountHealthCheckSettings) (*Account, error) {
	account := r.accounts[id]
	if account == nil {
		return nil, ErrAccountNotFound
	}
	if settings.Enabled != nil {
		account.HealthCheckEnabled = *settings.Enabled
	}
	if settings.Protected != nil {
		account.HealthCheckProtected = *settings.Protected
	}
	copied := *account
	return &copied, nil
}

func (r *fakeHealthAccountRepo) BulkUpdateSettings(_ context.Context, ids []int64, settings AccountHealthCheckSettings) (int64, error) {
	var updated int64
	for _, id := range ids {
		account := r.accounts[id]
		if account == nil {
			continue
		}
		if settings.Enabled != nil {
			account.HealthCheckEnabled = *settings.Enabled
		}
		if settings.Protected != nil {
			account.HealthCheckProtected = *settings.Protected
		}
		updated++
	}
	r.bulkUpdate = updated
	return updated, nil
}

func (r *fakeHealthAccountRepo) RecordResult(_ context.Context, accountID int64, status string, errorMessage string, failStreak int, disable bool) (*Account, error) {
	account := r.accounts[accountID]
	if account == nil {
		return nil, ErrAccountNotFound
	}
	account.HealthCheckFailStreak = failStreak
	account.LastHealthCheckStatus = &status
	if errorMessage == "" {
		account.LastHealthCheckError = nil
	} else {
		account.LastHealthCheckError = &errorMessage
	}
	now := time.Now()
	account.LastHealthCheckAt = &now
	if disable {
		account.Schedulable = false
	}
	r.recorded = append(r.recorded, recordedHealthResult{accountID: accountID, status: status, streak: failStreak, disable: disable})
	copied := *account
	return &copied, nil
}

type fakeBackgroundTestRunner struct {
	resultByID map[int64]*ScheduledTestResult
}

func (r *fakeBackgroundTestRunner) RunTestBackgroundObserveOnly(_ context.Context, accountID int64, _ string) (*ScheduledTestResult, error) {
	if result := r.resultByID[accountID]; result != nil {
		return result, nil
	}
	now := time.Now()
	return &ScheduledTestResult{
		Status:       AccountBatchTestResultSuccess,
		ResponseText: "ok",
		StartedAt:    now,
		FinishedAt:   now,
	}, nil
}

func TestAccountHealthCheckService_CreateManualBatchTestPersistsResults(t *testing.T) {
	ctx := context.Background()
	taskRepo := &fakeBatchTaskRepo{}
	resultRepo := &fakeBatchResultRepo{}
	accountRepo := &fakeHealthAccountRepo{accounts: map[int64]*Account{
		1: {ID: 1, Name: "a1", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true},
	}}
	runner := &fakeBackgroundTestRunner{}
	svc := NewAccountHealthCheckService(taskRepo, resultRepo, accountRepo, runner)

	task, err := svc.CreateManualBatchTest(ctx, []int64{1}, "", 1, false)
	if err != nil {
		t.Fatalf("CreateManualBatchTest() error = %v", err)
	}
	waitForTaskResults(t, resultRepo, 1)

	if task.ModelID != DefaultAccountHealthCheckModel {
		t.Fatalf("default model = %q, want %q", task.ModelID, DefaultAccountHealthCheckModel)
	}
	if got := len(resultRepo.results); got != 1 {
		t.Fatalf("results persisted = %d, want 1", got)
	}
	if resultRepo.results[0].Status != AccountBatchTestResultSuccess {
		t.Fatalf("result status = %q, want success", resultRepo.results[0].Status)
	}
}

func TestAccountHealthCheckService_ScheduledUsesOnlyOptInUnprotectedPool(t *testing.T) {
	ctx := context.Background()
	taskRepo := &fakeBatchTaskRepo{}
	resultRepo := &fakeBatchResultRepo{}
	accountRepo := &fakeHealthAccountRepo{accounts: map[int64]*Account{
		1: {ID: 1, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, HealthCheckEnabled: true},
		2: {ID: 2, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, HealthCheckEnabled: false},
		3: {ID: 3, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, HealthCheckEnabled: true, HealthCheckProtected: true},
		4: {ID: 4, Platform: PlatformAnthropic, Status: StatusActive, Schedulable: true, HealthCheckEnabled: true},
	}}
	svc := NewAccountHealthCheckService(taskRepo, resultRepo, accountRepo, &fakeBackgroundTestRunner{})

	task, err := svc.RunScheduled(ctx)
	if err != nil {
		t.Fatalf("RunScheduled() error = %v", err)
	}
	if task == nil {
		t.Fatalf("RunScheduled() task = nil")
	}
	if task.TotalCount != 1 {
		t.Fatalf("scheduled total = %d, want 1", task.TotalCount)
	}
	waitForTaskResults(t, resultRepo, 1)
	if got := *resultRepo.results[0].AccountID; got != 1 {
		t.Fatalf("scheduled account = %d, want 1", got)
	}
}

func TestAccountHealthCheckService_FailureStreakAndProtectedDisablePolicy(t *testing.T) {
	now := time.Now()
	failed := &ScheduledTestResult{
		Status:       AccountBatchTestResultFailed,
		ErrorMessage: "temporary upstream failure",
		StartedAt:    now,
		FinishedAt:   now,
	}
	accountRepo := &fakeHealthAccountRepo{accounts: map[int64]*Account{
		1: {ID: 1, Name: "normal", Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, HealthCheckFailStreak: 2},
		2: {ID: 2, Name: "protected", Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, HealthCheckFailStreak: 2, HealthCheckProtected: true},
	}}
	taskRepo := &fakeBatchTaskRepo{}
	resultRepo := &fakeBatchResultRepo{}
	svc := NewAccountHealthCheckService(taskRepo, resultRepo, accountRepo, &fakeBackgroundTestRunner{
		resultByID: map[int64]*ScheduledTestResult{1: failed, 2: failed},
	})

	if _, err := svc.CreateManualBatchTest(context.Background(), []int64{1, 2}, "gpt-5.4-mini", 2, true); err != nil {
		t.Fatalf("CreateManualBatchTest() error = %v", err)
	}
	waitForTaskResults(t, resultRepo, 2)

	if accountRepo.accounts[1].Schedulable {
		t.Fatalf("normal account should be disabled after 3 failures when autoDisable=true")
	}
	if !accountRepo.accounts[2].Schedulable {
		t.Fatalf("protected account must never be disabled")
	}
	if accountRepo.accounts[1].HealthCheckFailStreak != 3 || accountRepo.accounts[2].HealthCheckFailStreak != 3 {
		t.Fatalf("failure streaks = (%d,%d), want (3,3)", accountRepo.accounts[1].HealthCheckFailStreak, accountRepo.accounts[2].HealthCheckFailStreak)
	}
}

func TestAccountHealthCheckService_HealthClassifier401DisablesImmediately(t *testing.T) {
	now := time.Now()
	unauthorized := &ScheduledTestResult{
		Status:       AccountBatchTestResultFailed,
		ErrorMessage: "API returned 401: unauthorized",
		StartedAt:    now,
		FinishedAt:   now,
	}
	accountRepo := &fakeHealthAccountRepo{accounts: map[int64]*Account{
		1: {ID: 1, Name: "normal", Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true},
		2: {ID: 2, Name: "protected", Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, HealthCheckProtected: true},
	}}
	taskRepo := &fakeBatchTaskRepo{}
	resultRepo := &fakeBatchResultRepo{}
	svc := NewAccountHealthCheckService(taskRepo, resultRepo, accountRepo, &fakeBackgroundTestRunner{
		resultByID: map[int64]*ScheduledTestResult{1: unauthorized, 2: unauthorized},
	})

	if _, err := svc.CreateManualBatchTest(context.Background(), []int64{1, 2}, "gpt-5.4-mini", 2, false); err != nil {
		t.Fatalf("CreateManualBatchTest() error = %v", err)
	}
	waitForTaskResults(t, resultRepo, 2)

	if accountRepo.accounts[1].Schedulable || accountRepo.accounts[2].Schedulable {
		t.Fatalf("401 health check should disable scheduling immediately, schedulable=(%v,%v)", accountRepo.accounts[1].Schedulable, accountRepo.accounts[2].Schedulable)
	}
	if taskRepo.created[0].DeactivatedCount != 2 {
		t.Fatalf("deactivated count = %d, want 2", taskRepo.created[0].DeactivatedCount)
	}
}

func TestAccountHealthCheckService_HealthClassifier429CountsAsSuccess(t *testing.T) {
	now := time.Now()
	rateLimited := &ScheduledTestResult{
		Status:       AccountBatchTestResultFailed,
		ErrorMessage: "API returned 429: rate limit exceeded",
		StartedAt:    now,
		FinishedAt:   now,
	}
	accountRepo := &fakeHealthAccountRepo{accounts: map[int64]*Account{
		1: {ID: 1, Name: "a1", Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, HealthCheckFailStreak: 2},
	}}
	taskRepo := &fakeBatchTaskRepo{}
	resultRepo := &fakeBatchResultRepo{}
	svc := NewAccountHealthCheckService(taskRepo, resultRepo, accountRepo, &fakeBackgroundTestRunner{
		resultByID: map[int64]*ScheduledTestResult{1: rateLimited},
	})

	if _, err := svc.CreateManualBatchTest(context.Background(), []int64{1}, "gpt-5.4-mini", 1, true); err != nil {
		t.Fatalf("CreateManualBatchTest() error = %v", err)
	}
	waitForTaskResults(t, resultRepo, 1)

	if resultRepo.results[0].Status != AccountBatchTestResultSuccess {
		t.Fatalf("429 result status = %q, want success", resultRepo.results[0].Status)
	}
	if accountRepo.accounts[1].HealthCheckFailStreak != 0 {
		t.Fatalf("429 should clear failure streak, got %d", accountRepo.accounts[1].HealthCheckFailStreak)
	}
	if !accountRepo.accounts[1].Schedulable {
		t.Fatalf("429 should not disable scheduling")
	}
}

func TestAccountHealthCheckService_SuccessClearsFailureStreakWithoutRestoringSchedulable(t *testing.T) {
	ctx := context.Background()
	accountRepo := &fakeHealthAccountRepo{accounts: map[int64]*Account{
		1: {ID: 1, Name: "a1", Platform: PlatformOpenAI, Status: StatusActive, Schedulable: false, HealthCheckFailStreak: 2},
	}}
	resultRepo := &fakeBatchResultRepo{}
	svc := NewAccountHealthCheckService(&fakeBatchTaskRepo{}, resultRepo, accountRepo, &fakeBackgroundTestRunner{})

	if _, err := svc.CreateManualBatchTest(ctx, []int64{1}, "", 1, true); err != nil {
		t.Fatalf("CreateManualBatchTest() error = %v", err)
	}
	waitForTaskResults(t, resultRepo, 1)

	account := accountRepo.accounts[1]
	if account.HealthCheckFailStreak != 0 {
		t.Fatalf("failure streak = %d, want 0", account.HealthCheckFailStreak)
	}
	if account.Schedulable {
		t.Fatalf("successful health check must not auto-restore schedulable")
	}
}

func waitForTaskResults(t *testing.T, repo *fakeBatchResultRepo, want int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(repo.results) >= want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d results, got %d", want, len(repo.results))
}
