package service

import (
	"context"
	"errors"
	"math"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

type budgetCall struct {
	userID int64
	cost   float64
	taskID int64
}

// mockBudgetGuard records gate/charge calls and can force a fail-closed rejection.
type mockBudgetGuard struct {
	checkErr    error
	checkCalls  []budgetCall
	chargeCalls []budgetCall
}

type recordingVideoTaskCreationRepo struct {
	calls        []VideoTaskCreationInput
	result       *VideoTaskCreationResult
	err          error
	replayCalls  []VideoTaskCreationReplayInput
	replayResult *VideoTaskCreationResult
	replayFound  bool
	replayErr    error
}

func (r *recordingVideoTaskCreationRepo) CreateWithReservation(_ context.Context, input VideoTaskCreationInput) (*VideoTaskCreationResult, error) {
	r.calls = append(r.calls, input)
	if r.err != nil {
		return nil, r.err
	}
	if r.result != nil {
		return r.result, nil
	}
	task := cloneVideoTask(input.Task)
	task.ID = 9001
	reservationID := int64(7001)
	task.ReservationID = &reservationID
	return &VideoTaskCreationResult{
		Task: task,
		Reservation: &BillingReservation{
			ID:                reservationID,
			ReservationKey:    "video_task:create:" + task.CreationKey,
			SourceType:        "video_task",
			SourceID:          task.ID,
			UserID:            task.CreatedBy,
			APIKeyID:          task.APIKeyID,
			ReservedAmountUSD: input.ReservedAmountUSD,
			Status:            BillingReservationStatusActive,
			ExpiresAt:         input.ReservationExpiresAt,
		},
	}, nil
}

func (r *recordingVideoTaskCreationRepo) ReplayExisting(_ context.Context, input VideoTaskCreationReplayInput) (*VideoTaskCreationResult, bool, error) {
	r.replayCalls = append(r.replayCalls, input)
	return r.replayResult, r.replayFound, r.replayErr
}

func (m *mockBudgetGuard) CheckBudget(_ context.Context, userID int64, cost float64) error {
	m.checkCalls = append(m.checkCalls, budgetCall{userID: userID, cost: cost})
	return m.checkErr
}

func (m *mockBudgetGuard) Charge(_ context.Context, userID int64, cost float64, taskID int64) error {
	m.chargeCalls = append(m.chargeCalls, budgetCall{userID: userID, cost: cost, taskID: taskID})
	return nil
}

func approxEqual(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func cfgWithCostPerSecond(rate float64) *config.Config {
	return &config.Config{VideoGateway: config.VideoGatewayConfig{CostPerSecond: rate}}
}

type billingSeedanceAdapter struct {
	result *VideoAdapterResult
}

func (a *billingSeedanceAdapter) Provider() string { return VideoProviderSeedance }

func (a *billingSeedanceAdapter) CreateTask(_ context.Context, _ *VideoProviderAccount, task *VideoTask) (*VideoAdapterResult, error) {
	return &VideoAdapterResult{UpstreamTaskID: "seedance-billing-" + strconv.FormatInt(task.ID, 10), Status: VideoStatusSubmitted}, nil
}

func (a *billingSeedanceAdapter) PollTask(_ context.Context, _ *VideoProviderAccount, _ *VideoTask) (*VideoAdapterResult, error) {
	return cloneVideoAdapterResult(a.result), nil
}

func (a *billingSeedanceAdapter) CancelTask(_ context.Context, _ *VideoProviderAccount, _ *VideoTask) (*VideoAdapterResult, error) {
	return &VideoAdapterResult{Status: VideoStatusCancelled}, nil
}

func (a *billingSeedanceAdapter) NormalizeStatus(upstream string) string {
	return normalizeVideoStatus(upstream)
}

func (a *billingSeedanceAdapter) BuildCreatePayload(_ *VideoProviderAccount, _ *VideoTask) map[string]any {
	return map[string]any{}
}

type videoBillingDeductCall struct {
	userID int64
	amount float64
}

type recordingVideoBillingUserRepo struct {
	UserRepository
	calls []videoBillingDeductCall
	err   error
}

func (r *recordingVideoBillingUserRepo) DeductBalance(_ context.Context, userID int64, amount float64) error {
	r.calls = append(r.calls, videoBillingDeductCall{userID: userID, amount: amount})
	return r.err
}

type videoBillingSettingRepo struct {
	values map[string]string
}

func (r *videoBillingSettingRepo) Get(_ context.Context, key string) (*Setting, error) {
	if v, ok := r.values[key]; ok {
		return &Setting{Key: key, Value: v}, nil
	}
	return nil, ErrSettingNotFound
}

func (r *videoBillingSettingRepo) GetValue(_ context.Context, key string) (string, error) {
	if v, ok := r.values[key]; ok {
		return v, nil
	}
	return "", ErrSettingNotFound
}

func (r *videoBillingSettingRepo) Set(_ context.Context, key, value string) error {
	if r.values == nil {
		r.values = map[string]string{}
	}
	r.values[key] = value
	return nil
}

func (r *videoBillingSettingRepo) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	out := map[string]string{}
	for _, key := range keys {
		if v, ok := r.values[key]; ok {
			out[key] = v
		}
	}
	return out, nil
}

func (r *videoBillingSettingRepo) SetMultiple(_ context.Context, settings map[string]string) error {
	if r.values == nil {
		r.values = map[string]string{}
	}
	for key, value := range settings {
		r.values[key] = value
	}
	return nil
}

func (r *videoBillingSettingRepo) GetAll(_ context.Context) (map[string]string, error) {
	out := map[string]string{}
	for key, value := range r.values {
		out[key] = value
	}
	return out, nil
}

func (r *videoBillingSettingRepo) Delete(_ context.Context, key string) error {
	delete(r.values, key)
	return nil
}

type recordingVideoBillingCache struct {
	billingCacheWorkerStub
	mu    sync.Mutex
	calls []videoBillingDeductCall
}

func (c *recordingVideoBillingCache) DeductUserBalance(_ context.Context, userID int64, amount float64) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls = append(c.calls, videoBillingDeductCall{userID: userID, amount: amount})
	return nil
}

func (c *recordingVideoBillingCache) deductCalls() []videoBillingDeductCall {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]videoBillingDeductCall(nil), c.calls...)
}

func newVideoBalanceBillingDeps() (*recordingVideoBillingUserRepo, *recordingVideoBillingCache, *SettingService, *BillingCacheService) {
	userRepo := &recordingVideoBillingUserRepo{}
	cache := &recordingVideoBillingCache{}
	settingSvc := NewSettingService(&videoBillingSettingRepo{values: map[string]string{
		SettingKeyUSDCNYRate: "7.20",
	}}, &config.Config{})
	cacheSvc := NewBillingCacheService(cache, nil, nil, nil, nil, nil, &config.Config{})
	return userRepo, cache, settingSvc, cacheSvc
}

func TestVideoGatewayPricingAdapterReturnsMoneyAndSnapshot(t *testing.T) {
	_, _, settingSvc, _ := newVideoBalanceBillingDeps()
	svc := NewVideoGatewayService(newMemoryVideoGatewayRepo(), noopVideoKeyEncryptor{}, cfgWithCostPerSecond(1))
	svc.SetBalanceBillingDependencies(nil, settingSvc, nil)

	var pricing VideoTaskPricing = svc
	amountUSD, snapshot, err := pricing.EstimatePrice(context.Background(), &VideoTask{
		Provider:   VideoProviderSeedance,
		Model:      "doubao-seedance-2-0-260128",
		Duration:   5,
		Resolution: "720p",
	})
	if err != nil {
		t.Fatalf("estimate price: %v", err)
	}
	if got, want := amountUSD.String(), "0.6944444444"; got != want {
		t.Fatalf("USD amount = %s, want %s", got, want)
	}
	if got, want := snapshot.AmountOriginal.String(), "5.0000000000"; got != want {
		t.Fatalf("original amount = %s, want %s", got, want)
	}
	if snapshot.AmountOriginal.Currency() != Currency("CNY") {
		t.Fatalf("original currency = %s, want CNY", snapshot.AmountOriginal.Currency())
	}
	if snapshot.ExchangeRate != "7.2000000000" {
		t.Fatalf("exchange rate = %q, want 7.2000000000", snapshot.ExchangeRate)
	}
	if snapshot.PricingVersion != VideoPricingVersionSeedance202603 {
		t.Fatalf("pricing version = %q", snapshot.PricingVersion)
	}
}

func TestVideoGatewayCreateUsesReservationOnlyForFlagOnRealProvider(t *testing.T) {
	ctx := context.Background()
	creationKey := strings.Repeat("a", 64)
	fingerprint := strings.Repeat("b", 64)

	t.Run("flag on real provider uses atomic creation repository", func(t *testing.T) {
		repo := newMemoryVideoGatewayRepo()
		providerID := seedSmokeAuthorizedSeedanceProvider(repo, "fake-only-key", "https://provider.invalid")
		cfg := cfgWithCostPerSecond(1)
		cfg.ReliabilityCore.VideoEnabled = true
		cfg.ReliabilityCore.ReservationTTLHours = 6
		svc := NewVideoGatewayService(repo, noopVideoKeyEncryptor{}, cfg)
		creationRepo := &recordingVideoTaskCreationRepo{}
		svc.SetVideoTaskCreationRepository(creationRepo)

		task, err := svc.CreateTask(ctx, VideoTaskCreateParams{
			ProviderAccountID:   providerID,
			TaskType:            VideoTaskTypeTextToVideo,
			Model:               "doubao-seedance-2-0-260128",
			Prompt:              "reserve first",
			Duration:            5,
			Resolution:          "720p",
			CreatedBy:           8,
			CreationKey:         creationKey,
			CreationFingerprint: fingerprint,
		})
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		if len(creationRepo.calls) != 1 {
			t.Fatalf("atomic creation calls = %d, want 1", len(creationRepo.calls))
		}
		if len(repo.tasks) != 0 {
			t.Fatalf("legacy repository wrote %d tasks", len(repo.tasks))
		}
		if task.ReservationID == nil || *task.ReservationID != 7001 {
			t.Fatalf("reservation id = %#v", task.ReservationID)
		}
		input := creationRepo.calls[0]
		if input.Task.CreationKey != creationKey || input.Task.CreationFingerprint != fingerprint {
			t.Fatalf("creation identity = %q/%q", input.Task.CreationKey, input.Task.CreationFingerprint)
		}
		if input.ReservedAmountUSD.String() != "0.6944444444" {
			t.Fatalf("reserved USD = %s", input.ReservedAmountUSD.String())
		}
	})

	t.Run("flag off keeps legacy creation", func(t *testing.T) {
		repo := newMemoryVideoGatewayRepo()
		providerID := seedSmokeAuthorizedSeedanceProvider(repo, "fake-only-key", "https://provider.invalid")
		cfg := cfgWithCostPerSecond(1)
		svc := NewVideoGatewayService(repo, noopVideoKeyEncryptor{}, cfg)
		creationRepo := &recordingVideoTaskCreationRepo{}
		svc.SetVideoTaskCreationRepository(creationRepo)

		task, err := svc.CreateTask(ctx, VideoTaskCreateParams{
			ProviderAccountID:   providerID,
			TaskType:            VideoTaskTypeTextToVideo,
			Prompt:              "legacy",
			Duration:            5,
			CreatedBy:           8,
			CreationKey:         creationKey,
			CreationFingerprint: fingerprint,
		})
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		if len(creationRepo.calls) != 0 || len(repo.tasks) != 1 {
			t.Fatalf("atomic calls/tasks = %d/%d, want 0/1", len(creationRepo.calls), len(repo.tasks))
		}
		if task.ReservationID != nil {
			t.Fatalf("flag-off task reserved balance: %#v", task.ReservationID)
		}
		if task.APIKeyID != nil {
			t.Fatalf("JWT/admin task API key attribution = %#v, want nil", task.APIKeyID)
		}
	})

	t.Run("mock never reserves even when flag is on", func(t *testing.T) {
		repo := newMemoryVideoGatewayRepo()
		providerID := repo.seedMockProvider()
		cfg := &config.Config{ReliabilityCore: config.ReliabilityCoreConfig{VideoEnabled: true, ReservationTTLHours: 6}}
		svc := NewVideoGatewayService(repo, noopVideoKeyEncryptor{}, cfg)
		creationRepo := &recordingVideoTaskCreationRepo{}
		svc.SetVideoTaskCreationRepository(creationRepo)

		task, err := svc.CreateTask(ctx, VideoTaskCreateParams{
			ProviderAccountID:   providerID,
			TaskType:            VideoTaskTypeTextToVideo,
			Prompt:              "mock",
			CreatedBy:           8,
			CreationKey:         creationKey,
			CreationFingerprint: fingerprint,
		})
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		if len(creationRepo.calls) != 0 || task.ReservationID != nil {
			t.Fatalf("mock atomic calls/reservation = %d/%#v", len(creationRepo.calls), task.ReservationID)
		}
	})

	t.Run("reservation failure leaves no task", func(t *testing.T) {
		repo := newMemoryVideoGatewayRepo()
		providerID := seedSmokeAuthorizedSeedanceProvider(repo, "fake-only-key", "https://provider.invalid")
		cfg := cfgWithCostPerSecond(1)
		cfg.ReliabilityCore.VideoEnabled = true
		cfg.ReliabilityCore.ReservationTTLHours = 6
		svc := NewVideoGatewayService(repo, noopVideoKeyEncryptor{}, cfg)
		creationRepo := &recordingVideoTaskCreationRepo{err: ErrBillingReservationInsufficientBalance}
		svc.SetVideoTaskCreationRepository(creationRepo)

		_, err := svc.CreateTask(ctx, VideoTaskCreateParams{
			ProviderAccountID:   providerID,
			TaskType:            VideoTaskTypeTextToVideo,
			Prompt:              "cannot afford",
			Duration:            5,
			CreatedBy:           8,
			CreationKey:         creationKey,
			CreationFingerprint: fingerprint,
		})
		if !errors.Is(err, ErrBillingReservationInsufficientBalance) {
			t.Fatalf("error = %v", err)
		}
		if len(repo.tasks) != 0 || len(repo.events) != 0 {
			t.Fatalf("partial task/events = %d/%d", len(repo.tasks), len(repo.events))
		}
	})
}

func TestVideoGatewayCreateReplaysBeforeMutableRouteValidationAfterFlagOff(t *testing.T) {
	creationKey := strings.Repeat("c", 64)
	fingerprint := strings.Repeat("d", 64)
	existing := &VideoTask{ID: 321, CreatedBy: 8, CreationKey: creationKey, CreationFingerprint: fingerprint, Status: VideoStatusQueued}
	creationRepo := &recordingVideoTaskCreationRepo{
		replayFound:  true,
		replayResult: &VideoTaskCreationResult{Task: existing, Replayed: true},
	}
	cfg := &config.Config{ReliabilityCore: config.ReliabilityCoreConfig{VideoEnabled: false, ReservationTTLHours: 6}}
	svc := NewVideoGatewayService(newMemoryVideoGatewayRepo(), noopVideoKeyEncryptor{}, cfg)
	svc.SetVideoTaskCreationRepository(creationRepo)

	task, err := svc.CreateTask(context.Background(), VideoTaskCreateParams{
		ProviderAccountID:   9_999_999,
		TaskType:            VideoTaskTypeTextToVideo,
		Prompt:              "same canonical payload",
		CreatedBy:           8,
		CreationKey:         creationKey,
		CreationFingerprint: fingerprint,
	})
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if task.ID != existing.ID {
		t.Fatalf("task id = %d, want %d", task.ID, existing.ID)
	}
	if len(creationRepo.replayCalls) != 1 || len(creationRepo.calls) != 0 {
		t.Fatalf("replay/create calls = %d/%d, want 1/0", len(creationRepo.replayCalls), len(creationRepo.calls))
	}
}

func TestVideoGatewayCreateConcurrentRepositoryReplayDoesNotDuplicateEvents(t *testing.T) {
	repo := newMemoryVideoGatewayRepo()
	providerID := seedSmokeAuthorizedSeedanceProvider(repo, "fake-only-key", "https://provider.invalid")
	creationKey := strings.Repeat("e", 64)
	fingerprint := strings.Repeat("f", 64)
	existing := &VideoTask{ID: 654, ProviderAccountID: providerID, Provider: VideoProviderSeedance, CreatedBy: 8, CreationKey: creationKey, CreationFingerprint: fingerprint, Status: VideoStatusQueued}
	creationRepo := &recordingVideoTaskCreationRepo{result: &VideoTaskCreationResult{Task: existing, Replayed: true}}
	cfg := cfgWithCostPerSecond(1)
	cfg.ReliabilityCore.VideoEnabled = true
	cfg.ReliabilityCore.ReservationTTLHours = 6
	svc := NewVideoGatewayService(repo, noopVideoKeyEncryptor{}, cfg)
	svc.SetVideoTaskCreationRepository(creationRepo)

	_, err := svc.CreateTask(context.Background(), VideoTaskCreateParams{
		ProviderAccountID:   providerID,
		TaskType:            VideoTaskTypeTextToVideo,
		Model:               "doubao-seedance-2-0-260128",
		Prompt:              "concurrent replay",
		Duration:            5,
		CreatedBy:           8,
		CreationKey:         creationKey,
		CreationFingerprint: fingerprint,
	})
	if err != nil {
		t.Fatalf("create replay: %v", err)
	}
	if len(repo.events) != 0 {
		t.Fatalf("replay appended %d duplicate events", len(repo.events))
	}
}

func TestVideoGatewayTrialWrapperConcurrentReplayDoesNotDuplicateGateEvent(t *testing.T) {
	t.Setenv("SUB2API_VIDEO_REAL_SMOKE_ENABLED", "1")
	t.Setenv("SUB2API_VIDEO_REDACTED_EVENT_LOG", t.TempDir()+"/audit.log")
	t.Setenv("SUB2API_MEDIA_URL_ALLOWLIST", "provider.invalid")
	repo := newMemoryVideoGatewayRepo()
	providerID := seedSmokeAuthorizedSeedanceProvider(repo, "fake-only-key", "https://provider.invalid")
	creationKey := strings.Repeat("1", 64)
	fingerprint := strings.Repeat("2", 64)
	existing := &VideoTask{ID: 777, ProviderAccountID: providerID, Provider: VideoProviderSeedance, CreatedBy: 8, CreationKey: creationKey, CreationFingerprint: fingerprint, Status: VideoStatusQueued}
	creationRepo := &recordingVideoTaskCreationRepo{result: &VideoTaskCreationResult{Task: existing, Replayed: true}}
	cfg := cfgWithCostPerSecond(1)
	cfg.ReliabilityCore.VideoEnabled = true
	cfg.ReliabilityCore.ReservationTTLHours = 6
	svc := NewVideoGatewayService(repo, noopVideoKeyEncryptor{}, cfg)
	svc.SetVideoTaskCreationRepository(creationRepo)

	_, err := svc.CreateAPIKeySeedanceTinyTrialTask(context.Background(), VideoProviderSeedance, VideoTaskCreateParams{
		TaskType:            VideoTaskTypeTextToVideo,
		Model:               "doubao-seedance-2-0-260128",
		Prompt:              "trial concurrent replay",
		Duration:            5,
		CreatedBy:           8,
		CreationKey:         creationKey,
		CreationFingerprint: fingerprint,
	})
	if err != nil {
		t.Fatalf("trial replay: %v", err)
	}
	if len(repo.events) != 0 {
		t.Fatalf("trial replay appended %d duplicate events", len(repo.events))
	}
}

func TestSeedanceActualCostUsesProviderUsageTokens(t *testing.T) {
	ctx := context.Background()
	repo := newMemoryVideoGatewayRepo()
	providerID := seedSmokeAuthorizedSeedanceProvider(repo, "seedance-billing-test-key", "https://ark.example.test")
	tokens := int64(102960)
	svc := NewVideoGatewayService(repo, noopVideoKeyEncryptor{}, nil)
	svc.adapters[VideoProviderSeedance] = &billingSeedanceAdapter{result: &VideoAdapterResult{
		Status:           VideoStatusSucceeded,
		ResultURL:        "https://ark-content.cn-beijing.volces.com/v/ok.mp4",
		UsageTotalTokens: &tokens,
		ActualResolution: "720p",
	}}
	guard := &mockBudgetGuard{}
	svc.SetBudgetGuard(guard)

	task, err := svc.CreateTask(ctx, VideoTaskCreateParams{
		ProviderAccountID: providerID,
		TaskType:          VideoTaskTypeTextToVideo,
		Model:             "doubao-seedance-2-0-260128",
		Prompt:            "bill by tokens",
		Duration:          5,
		Resolution:        "720p",
		CreatedBy:         8,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	for range 2 {
		if err := svc.ProcessRunnableTasks(ctx, 10, time.Minute); err != nil {
			t.Fatalf("process: %v", err)
		}
	}
	got, _, err := svc.GetTask(ctx, task.ID, 8, false)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	want := 46.0 * 0.10296
	if !approxEqual(got.CostEstimate, want) {
		t.Fatalf("actual cost = %.8f, want %.8f", got.CostEstimate, want)
	}
	if len(repo.usage) != 1 || !approxEqual(repo.usage[0].CostEstimate, want) {
		t.Fatalf("usage log cost = %#v, want %.8f", repo.usage, want)
	}
	if len(guard.chargeCalls) != 1 || !approxEqual(guard.chargeCalls[0].cost, want) {
		t.Fatalf("charge calls = %#v, want cost %.8f", guard.chargeCalls, want)
	}
}

func TestSeedanceSucceededTaskDeductsUserBalanceInUSDAndQueuesCache(t *testing.T) {
	ctx := context.Background()
	repo := newMemoryVideoGatewayRepo()
	providerID := seedSmokeAuthorizedSeedanceProvider(repo, "seedance-billing-test-key", "https://ark.example.test")
	tokens := int64(102960)
	svc := NewVideoGatewayService(repo, noopVideoKeyEncryptor{}, nil)
	svc.adapters[VideoProviderSeedance] = &billingSeedanceAdapter{result: &VideoAdapterResult{
		Status:           VideoStatusSucceeded,
		ResultURL:        "https://ark-content.cn-beijing.volces.com/v/ok.mp4",
		UsageTotalTokens: &tokens,
		ActualResolution: "720p",
	}}
	userRepo, cache, settingSvc, cacheSvc := newVideoBalanceBillingDeps()
	defer cacheSvc.Stop()
	svc.userRepo = userRepo
	svc.settingService = settingSvc
	svc.billingCacheService = cacheSvc

	task, err := svc.CreateTask(ctx, VideoTaskCreateParams{
		ProviderAccountID: providerID,
		TaskType:          VideoTaskTypeTextToVideo,
		Model:             "doubao-seedance-2-0-260128",
		Prompt:            "deduct balance by tokens",
		Duration:          5,
		Resolution:        "720p",
		CreatedBy:         8,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	for range 2 {
		if err := svc.ProcessRunnableTasks(ctx, 10, time.Minute); err != nil {
			t.Fatalf("process: %v", err)
		}
	}
	cacheSvc.Stop()

	wantUSD := (46.0 * 0.10296) / 7.20
	if len(userRepo.calls) != 1 || userRepo.calls[0].userID != 8 || !approxEqual(userRepo.calls[0].amount, wantUSD) {
		t.Fatalf("DeductBalance calls = %#v, want user=8 amount %.8f", userRepo.calls, wantUSD)
	}
	cacheCalls := cache.deductCalls()
	if len(cacheCalls) != 1 || cacheCalls[0].userID != 8 || !approxEqual(cacheCalls[0].amount, wantUSD) {
		t.Fatalf("QueueDeductBalance cache calls = %#v, want user=8 amount %.8f", cacheCalls, wantUSD)
	}
	if _, ok := repo.balanceClaims[task.ID]; !ok {
		t.Fatalf("expected task %d balance charge to be claimed", task.ID)
	}
}

func TestSeedanceBalanceChargeIsIdempotentOnWorkerRetry(t *testing.T) {
	ctx := context.Background()
	repo := newMemoryVideoGatewayRepo()
	task := &VideoTask{
		ID:               42,
		Provider:         VideoProviderSeedance,
		Model:            "doubao-seedance-2-0-260128",
		Status:           VideoStatusSucceeded,
		UsageTotalTokens: videoInt64Ptr(102960),
		Currency:         BillingCurrencyCNY,
		CreatedBy:        8,
	}
	repo.tasks[task.ID] = cloneVideoTask(task)
	svc := NewVideoGatewayService(repo, noopVideoKeyEncryptor{}, nil)
	userRepo, cache, settingSvc, cacheSvc := newVideoBalanceBillingDeps()
	defer cacheSvc.Stop()
	svc.userRepo = userRepo
	svc.settingService = settingSvc
	svc.billingCacheService = cacheSvc

	svc.chargeForVideo(ctx, task)
	svc.chargeForVideo(ctx, task)
	cacheSvc.Stop()

	if len(userRepo.calls) != 1 {
		t.Fatalf("DeductBalance must be idempotent, calls=%#v", userRepo.calls)
	}
	if cacheCalls := cache.deductCalls(); len(cacheCalls) != 1 {
		t.Fatalf("cache balance deduction must be idempotent, calls=%#v", cacheCalls)
	}
}

func TestSeedanceBalanceChargeReleasesClaimAfterDeductFailure(t *testing.T) {
	ctx := context.Background()
	repo := newMemoryVideoGatewayRepo()
	task := &VideoTask{
		ID:               45,
		Provider:         VideoProviderSeedance,
		Model:            "doubao-seedance-2-0-260128",
		Status:           VideoStatusSucceeded,
		UsageTotalTokens: videoInt64Ptr(102960),
		Currency:         BillingCurrencyCNY,
		CreatedBy:        8,
	}
	repo.tasks[task.ID] = cloneVideoTask(task)
	svc := NewVideoGatewayService(repo, noopVideoKeyEncryptor{}, nil)
	userRepo, cache, settingSvc, cacheSvc := newVideoBalanceBillingDeps()
	defer cacheSvc.Stop()
	userRepo.err = errors.New("deduct temporarily unavailable")
	svc.userRepo = userRepo
	svc.settingService = settingSvc
	svc.billingCacheService = cacheSvc

	svc.chargeForVideo(ctx, task)
	if _, ok := repo.balanceClaims[task.ID]; ok {
		t.Fatalf("deduct failure must release balance claim for task %d", task.ID)
	}

	userRepo.err = nil
	svc.chargeForVideo(ctx, task)
	cacheSvc.Stop()

	if len(userRepo.calls) != 2 {
		t.Fatalf("expected failed attempt plus retry deduction, calls=%#v", userRepo.calls)
	}
	if cacheCalls := cache.deductCalls(); len(cacheCalls) != 1 {
		t.Fatalf("only successful retry should enqueue cache deduction, calls=%#v", cacheCalls)
	}
	if _, ok := repo.balanceClaims[task.ID]; !ok {
		t.Fatalf("successful retry should leave balance charge claimed")
	}
}

func TestProcessRunnableTasksRetriesUnchargedSucceededBilling(t *testing.T) {
	ctx := context.Background()
	repo := newMemoryVideoGatewayRepo()
	task := &VideoTask{
		ID:               46,
		Provider:         VideoProviderSeedance,
		Model:            "doubao-seedance-2-0-260128",
		Status:           VideoStatusSucceeded,
		UsageTotalTokens: videoInt64Ptr(102960),
		Currency:         BillingCurrencyCNY,
		CreatedBy:        8,
	}
	repo.tasks[task.ID] = cloneVideoTask(task)
	svc := NewVideoGatewayService(repo, noopVideoKeyEncryptor{}, nil)
	userRepo, cache, settingSvc, cacheSvc := newVideoBalanceBillingDeps()
	defer cacheSvc.Stop()
	svc.userRepo = userRepo
	svc.settingService = settingSvc
	svc.billingCacheService = cacheSvc

	if err := svc.ProcessRunnableTasks(ctx, 10, time.Minute); err != nil {
		t.Fatalf("process runnable tasks: %v", err)
	}
	cacheSvc.Stop()

	if len(userRepo.calls) != 1 {
		t.Fatalf("succeeded uncharged task should be retried for billing, calls=%#v", userRepo.calls)
	}
	if cacheCalls := cache.deductCalls(); len(cacheCalls) != 1 {
		t.Fatalf("successful retry should enqueue cache deduction, calls=%#v", cacheCalls)
	}
}

func TestSeedanceFailedTaskDoesNotDeductBalance(t *testing.T) {
	ctx := context.Background()
	repo := newMemoryVideoGatewayRepo()
	task := &VideoTask{
		ID:               43,
		Provider:         VideoProviderSeedance,
		Model:            "doubao-seedance-2-0-260128",
		Status:           VideoStatusFailed,
		UsageTotalTokens: videoInt64Ptr(102960),
		Currency:         BillingCurrencyCNY,
		CreatedBy:        8,
	}
	repo.tasks[task.ID] = cloneVideoTask(task)
	svc := NewVideoGatewayService(repo, noopVideoKeyEncryptor{}, nil)
	userRepo, cache, settingSvc, cacheSvc := newVideoBalanceBillingDeps()
	defer cacheSvc.Stop()
	svc.userRepo = userRepo
	svc.settingService = settingSvc
	svc.billingCacheService = cacheSvc

	svc.chargeForVideo(ctx, task)
	cacheSvc.Stop()

	if len(userRepo.calls) != 0 {
		t.Fatalf("failed task must not deduct user balance, calls=%#v", userRepo.calls)
	}
	if cacheCalls := cache.deductCalls(); len(cacheCalls) != 0 {
		t.Fatalf("failed task must not deduct balance cache, calls=%#v", cacheCalls)
	}
	if _, ok := repo.balanceClaims[task.ID]; ok {
		t.Fatalf("failed task must not claim balance charge")
	}
}

func TestMockSucceededTaskHasZeroBillingSideEffects(t *testing.T) {
	ctx := context.Background()
	repo := newMemoryVideoGatewayRepo()
	task := &VideoTask{ID: 144, Provider: VideoProviderMock, Status: VideoStatusSucceeded, CostEstimate: 99, CreatedBy: 8}
	repo.tasks[task.ID] = cloneVideoTask(task)
	svc := NewVideoGatewayService(repo, noopVideoKeyEncryptor{}, nil)
	userRepo, cache, settingSvc, cacheSvc := newVideoBalanceBillingDeps()
	defer cacheSvc.Stop()
	guard := &mockBudgetGuard{}
	svc.userRepo = userRepo
	svc.settingService = settingSvc
	svc.billingCacheService = cacheSvc
	svc.SetBudgetGuard(guard)

	svc.chargeForVideo(ctx, task)
	cacheSvc.Stop()

	if len(userRepo.calls) != 0 {
		t.Fatalf("mock task deducted user balance: %#v", userRepo.calls)
	}
	if calls := cache.deductCalls(); len(calls) != 0 {
		t.Fatalf("mock task queued cache deduction: %#v", calls)
	}
	if len(guard.chargeCalls) != 0 {
		t.Fatalf("mock task invoked budget charge hook: %#v", guard.chargeCalls)
	}
	if _, claimed := repo.balanceClaims[task.ID]; claimed {
		t.Fatalf("mock task claimed balance charge")
	}
}

func TestSeedanceActualCostSelectsVideoInputRate(t *testing.T) {
	tokens := int64(102960)
	svc := NewVideoGatewayService(newMemoryVideoGatewayRepo(), noopVideoKeyEncryptor{}, nil)
	cost := svc.calculateVideoActualCost(&VideoTask{
		Provider:         VideoProviderSeedance,
		Model:            "doubao-seedance-2-0-260128",
		Status:           VideoStatusSucceeded,
		UsageTotalTokens: &tokens,
		HasVideoInput:    true,
	})
	want := 28.0 * 0.10296
	if !approxEqual(cost, want) {
		t.Fatalf("video-input cost = %.8f, want %.8f", cost, want)
	}
}

func videoInt64Ptr(v int64) *int64 {
	return &v
}

func TestSeedanceFailedTaskCostsZeroEvenWithUsageTokens(t *testing.T) {
	ctx := context.Background()
	repo := newMemoryVideoGatewayRepo()
	providerID := seedSmokeAuthorizedSeedanceProvider(repo, "seedance-billing-test-key", "https://ark.example.test")
	tokens := int64(102960)
	svc := NewVideoGatewayService(repo, noopVideoKeyEncryptor{}, nil)
	svc.adapters[VideoProviderSeedance] = &billingSeedanceAdapter{result: &VideoAdapterResult{
		Status:           VideoStatusFailed,
		ErrorMessage:     "provider failed",
		UsageTotalTokens: &tokens,
		CostEstimate:     99,
	}}
	guard := &mockBudgetGuard{}
	svc.SetBudgetGuard(guard)

	task, err := svc.CreateTask(ctx, VideoTaskCreateParams{
		ProviderAccountID: providerID,
		TaskType:          VideoTaskTypeTextToVideo,
		Model:             "doubao-seedance-2-0-260128",
		Prompt:            "failed task is free",
		Duration:          5,
		Resolution:        "720p",
		CreatedBy:         8,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	for range 2 {
		if err := svc.ProcessRunnableTasks(ctx, 10, time.Minute); err != nil {
			t.Fatalf("process: %v", err)
		}
	}
	got, _, err := svc.GetTask(ctx, task.ID, 8, false)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status != VideoStatusFailed || got.CostEstimate != 0 {
		t.Fatalf("failed task status/cost = %s/%.4f, want failed/0", got.Status, got.CostEstimate)
	}
	if len(repo.usage) != 1 || repo.usage[0].CostEstimate != 0 {
		t.Fatalf("usage log cost = %#v, want zero", repo.usage)
	}
	if len(guard.chargeCalls) != 0 {
		t.Fatalf("failed task must not charge, got %#v", guard.chargeCalls)
	}
}

func TestSeedanceBudgetEstimateUsesReferenceTableWhenConfigRateUnset(t *testing.T) {
	svc := NewVideoGatewayService(newMemoryVideoGatewayRepo(), noopVideoKeyEncryptor{}, nil)
	cost := svc.estimateVideoCost(&VideoTask{
		Provider:   VideoProviderSeedance,
		Model:      "doubao-seedance-2-0-260128",
		Duration:   5,
		Resolution: "720p",
	})
	if !approxEqual(cost, 5.0) {
		t.Fatalf("720p 5s estimate = %.4f, want 5.0", cost)
	}
	fastCost := svc.estimateVideoCost(&VideoTask{
		Provider:   VideoProviderSeedance,
		Model:      "doubao-seedance-2-0-fast",
		Duration:   5,
		Resolution: "720p",
	})
	if !approxEqual(fastCost, 4.0) {
		t.Fatalf("fast 720p 5s estimate = %.4f, want 4.0", fastCost)
	}
}

// TestVideoBudgetGateAllowsWhenAffordable: sufficient budget => create proceeds and
// the gate was consulted with the duration-based estimated cost.
func TestVideoBudgetGateAllowsWhenAffordable(t *testing.T) {
	ctx := context.Background()
	repo := newMemoryVideoGatewayRepo()
	providerID := repo.seedMockProvider()
	svc := NewVideoGatewayService(repo, noopVideoKeyEncryptor{}, cfgWithCostPerSecond(0.5))
	guard := &mockBudgetGuard{} // checkErr nil => affordable
	svc.SetBudgetGuard(guard)

	task, err := svc.CreateTask(ctx, VideoTaskCreateParams{
		ProviderAccountID: providerID,
		TaskType:          VideoTaskTypeTextToVideo,
		Prompt:            "affordable render",
		CreatedBy:         3,
		// Duration 0 => default 5s; cost = 0.5 × 5 = 2.5
	})
	if err != nil {
		t.Fatalf("expected create to proceed, got %v", err)
	}
	if task == nil || task.ID == 0 {
		t.Fatal("expected a persisted task")
	}
	if len(repo.tasks) != 1 {
		t.Fatalf("expected one task persisted, got %d", len(repo.tasks))
	}
	if len(guard.checkCalls) != 1 {
		t.Fatalf("expected budget gate consulted once, got %d", len(guard.checkCalls))
	}
	if guard.checkCalls[0].userID != 3 || !approxEqual(guard.checkCalls[0].cost, 2.5) {
		t.Fatalf("expected gate(user=3, cost=2.5), got %+v", guard.checkCalls[0])
	}
}

// TestVideoBudgetGateRejectsFailClosed: insufficient budget => the gate's error is
// propagated, NO task is persisted and NO provider dispatch / charge occurs.
func TestVideoBudgetGateRejectsFailClosed(t *testing.T) {
	ctx := context.Background()
	repo := newMemoryVideoGatewayRepo()
	providerID := repo.seedMockProvider()
	svc := NewVideoGatewayService(repo, noopVideoKeyEncryptor{}, cfgWithCostPerSecond(0.5))
	denied := errors.New("INSUFFICIENT_BUDGET: over per-call cap")
	guard := &mockBudgetGuard{checkErr: denied}
	svc.SetBudgetGuard(guard)

	task, err := svc.CreateTask(ctx, VideoTaskCreateParams{
		ProviderAccountID: providerID,
		TaskType:          VideoTaskTypeTextToVideo,
		Prompt:            "over-budget render",
		CreatedBy:         4,
	})
	if err == nil {
		t.Fatal("expected fail-closed rejection, got nil error")
	}
	if !errors.Is(err, denied) {
		t.Fatalf("expected the gate's denial error to propagate, got %v", err)
	}
	if task != nil {
		t.Fatalf("expected no task on rejection, got %#v", task)
	}
	if len(repo.tasks) != 0 {
		t.Fatalf("fail-closed must NOT persist a task; got %d persisted", len(repo.tasks))
	}
	if len(guard.chargeCalls) != 0 {
		t.Fatalf("rejected create must not charge; got %d charges", len(guard.chargeCalls))
	}
}

// TestVideoBudgetDoesNotChargeMockOnSuccess preserves the create-time budget
// check while proving a delivered mock task never invokes the charge hook.
func TestVideoBudgetDoesNotChargeMockOnSuccess(t *testing.T) {
	ctx := context.Background()
	repo := newMemoryVideoGatewayRepo()
	providerID := repo.seedMockProvider()
	svc := NewVideoGatewayService(repo, noopVideoKeyEncryptor{}, cfgWithCostPerSecond(2.0))
	guard := &mockBudgetGuard{}
	svc.SetBudgetGuard(guard)

	task, err := svc.CreateTask(ctx, VideoTaskCreateParams{
		ProviderAccountID: providerID,
		TaskType:          VideoTaskTypeTextToVideo,
		Prompt:            "render and bill me",
		CreatedBy:         8,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	// mock provider: queued -> submitted -> running -> succeeded across 3 ticks.
	for range 3 {
		if err := svc.ProcessRunnableTasks(ctx, 10, time.Minute); err != nil {
			t.Fatalf("process: %v", err)
		}
	}
	got, _, err := svc.GetTask(ctx, task.ID, 8, false)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status != VideoStatusSucceeded {
		t.Fatalf("expected succeeded, got %s", got.Status)
	}
	if len(guard.checkCalls) != 1 {
		t.Fatalf("expected one create-time budget check, got %d", len(guard.checkCalls))
	}
	if len(guard.chargeCalls) != 0 {
		t.Fatalf("mock success must not invoke charge hook, got %#v", guard.chargeCalls)
	}
}

// TestStaticBudgetGuardCheckBudget exercises the concrete interception primitive
// directly: an unconfigured cap fails closed, a cost over the cap is rejected, and a
// cost at or under the cap passes. This nails the comparison logic itself (the existing
// mockBudgetGuard only returns a canned error and never compares cost vs budget).
func TestStaticBudgetGuardCheckBudget(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name          string
		perCallBudget float64
		cost          float64
		wantErr       bool
	}{
		{"unconfigured cap fails closed", 0, 0, true},
		{"negative cap fails closed", -1, 0, true},
		{"NaN cap fails closed", math.NaN(), 1.0, true},
		{"Inf cap fails closed", math.Inf(1), 1.0, true},
		{"NaN cost fails closed", 3.0, math.NaN(), true},
		{"Inf cost fails closed", 3.0, math.Inf(1), true},
		{"cost over cap rejected", 3.0, 5.0, true},
		{"cost under cap allowed", 3.0, 2.5, false},
		{"cost equal to cap allowed (boundary)", 3.0, 3.0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewStaticBudgetGuard(tc.perCallBudget)
			err := g.CheckBudget(ctx, 1, tc.cost)
			if tc.wantErr && err == nil {
				t.Fatalf("expected rejection for cost=%v cap=%v, got nil", tc.cost, tc.perCallBudget)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("expected pass for cost=%v cap=%v, got %v", tc.cost, tc.perCallBudget, err)
			}
		})
	}
}

// TestVideoBudgetGateInterceptsWhenCostExceedsBudget is the dry-run interception proof:
// with a real StaticBudgetGuard armed at a per-call cap, a create whose duration-based
// estimate exceeds the cap is rejected by the gate at CreateTask — no task is persisted
// (the request never enters the provider call path). No real provider/key/DB is touched
// (memory repo + mock provider).
func TestVideoBudgetGateInterceptsWhenCostExceedsBudget(t *testing.T) {
	ctx := context.Background()
	repo := newMemoryVideoGatewayRepo()
	providerID := repo.seedMockProvider()
	svc := NewVideoGatewayService(repo, noopVideoKeyEncryptor{}, cfgWithCostPerSecond(0.5))
	svc.SetBudgetGuard(NewStaticBudgetGuard(3.0)) // per-call cap 3.0

	task, err := svc.CreateTask(ctx, VideoTaskCreateParams{
		ProviderAccountID: providerID,
		TaskType:          VideoTaskTypeTextToVideo,
		Prompt:            "over-cap render",
		CreatedBy:         5,
		Duration:          10, // cost = 0.5 × 10 = 5.0 > cap 3.0 => intercepted
	})
	if err == nil {
		t.Fatal("expected the budget gate to intercept, got nil error")
	}
	if !strings.Contains(err.Error(), "exceeds per-call cap") {
		t.Fatalf("expected an over-cap interception error, got %v", err)
	}
	if task != nil {
		t.Fatalf("expected no task on interception, got %#v", task)
	}
	if len(repo.tasks) != 0 {
		t.Fatalf("intercepted create must NOT persist a task (no call path); got %d persisted", len(repo.tasks))
	}
}

// TestVideoBudgetGateAllowsWithinBudget is the complementary pass-through proof: the SAME
// guard config (per-call cap 3.0) admits a create whose estimate is within budget, and
// the task is persisted. Paired with the interception test above, this proves the gate
// decides on the actual cost-vs-budget comparison, not on guard presence alone.
func TestVideoBudgetGateAllowsWithinBudget(t *testing.T) {
	ctx := context.Background()
	repo := newMemoryVideoGatewayRepo()
	providerID := repo.seedMockProvider()
	svc := NewVideoGatewayService(repo, noopVideoKeyEncryptor{}, cfgWithCostPerSecond(0.5))
	svc.SetBudgetGuard(NewStaticBudgetGuard(3.0)) // same per-call cap 3.0

	task, err := svc.CreateTask(ctx, VideoTaskCreateParams{
		ProviderAccountID: providerID,
		TaskType:          VideoTaskTypeTextToVideo,
		Prompt:            "within-cap render",
		CreatedBy:         6,
		Duration:          4, // cost = 0.5 × 4 = 2.0 <= cap 3.0 => allowed
	})
	if err != nil {
		t.Fatalf("expected create within budget to proceed, got %v", err)
	}
	if task == nil || task.ID == 0 {
		t.Fatal("expected a persisted task")
	}
	if len(repo.tasks) != 1 {
		t.Fatalf("expected one task persisted, got %d", len(repo.tasks))
	}
}

// cfgWithVideoBudget builds a config with BOTH the VA1 per-second rate and the per-call
// budget cap set, to exercise the production DI wiring (ProvideVideoGatewayService).
func cfgWithVideoBudget(rate, perCallBudget float64) *config.Config {
	return &config.Config{VideoGateway: config.VideoGatewayConfig{CostPerSecond: rate, PerCallBudget: perCallBudget}}
}

// TestProvideVideoGatewayServiceArmsGuardFromConfig proves the PRODUCTION wiring path:
// ProvideVideoGatewayService (the single DI seam, also called by wire_gen.go) reads
// video_gateway.per_call_budget and injects a StaticBudgetGuard armed at that cap. With
// cost_per_second=1.5 and per_call_budget=2 a 5s create estimates 7.5 > 2 and is rejected
// at CreateTask — no task persisted, no provider/key/DB touched. This is the unit-level
// mirror of the B-1 "empty-brake" (空踩刹车) check against the real DI wrapper.
func TestProvideVideoGatewayServiceArmsGuardFromConfig(t *testing.T) {
	ctx := context.Background()
	repo := newMemoryVideoGatewayRepo()
	providerID := repo.seedMockProvider()
	svc := ProvideVideoGatewayService(repo, noopVideoKeyEncryptor{}, cfgWithVideoBudget(1.5, 2.0), nil, nil, nil)

	task, err := svc.CreateTask(ctx, VideoTaskCreateParams{
		ProviderAccountID: providerID,
		TaskType:          VideoTaskTypeTextToVideo,
		Prompt:            "empty-brake render",
		CreatedBy:         7,
		Duration:          5, // cost = 1.5 × 5 = 7.5 > cap 2.0 => intercepted
	})
	if err == nil {
		t.Fatal("expected ProvideVideoGatewayService to arm the guard and intercept, got nil error")
	}
	if !strings.Contains(err.Error(), "exceeds per-call cap") {
		t.Fatalf("expected an over-cap interception error, got %v", err)
	}
	if task != nil {
		t.Fatalf("expected no task on interception, got %#v", task)
	}
	if len(repo.tasks) != 0 {
		t.Fatalf("armed guard must NOT persist an over-cap task; got %d persisted", len(repo.tasks))
	}
}

// TestProvideVideoGatewayServiceUnarmedWhenBudgetZero proves the default is inert: with
// per_call_budget=0 the wiring injects NO guard, so the gateway behaves exactly as before
// (the create proceeds and is persisted). This guarantees installing the field changes no
// production behaviour until a real cap is set.
func TestProvideVideoGatewayServiceUnarmedWhenBudgetZero(t *testing.T) {
	ctx := context.Background()
	repo := newMemoryVideoGatewayRepo()
	providerID := repo.seedMockProvider()
	svc := ProvideVideoGatewayService(repo, noopVideoKeyEncryptor{}, cfgWithVideoBudget(1.5, 0), nil, nil, nil)

	task, err := svc.CreateTask(ctx, VideoTaskCreateParams{
		ProviderAccountID: providerID,
		TaskType:          VideoTaskTypeTextToVideo,
		Prompt:            "unarmed render",
		CreatedBy:         7,
		Duration:          5,
	})
	if err != nil {
		t.Fatalf("expected unarmed gate to allow create, got %v", err)
	}
	if task == nil || task.ID == 0 {
		t.Fatal("expected a persisted task when the gate is unarmed")
	}
	if len(repo.tasks) != 1 {
		t.Fatalf("expected one task persisted, got %d", len(repo.tasks))
	}
}

// TestProvideVideoGatewayServiceAdmitsNormalClipAtRealCap proves the B-2 production config
// (cost_per_second=1.5, per_call_budget=30) admits a normal single 5s clip: estimate 7.5 <=
// cap 30 => allowed and persisted. The brake stops over-budget bursts, not the intended
// single smoke.
func TestProvideVideoGatewayServiceAdmitsNormalClipAtRealCap(t *testing.T) {
	ctx := context.Background()
	repo := newMemoryVideoGatewayRepo()
	providerID := repo.seedMockProvider()
	svc := ProvideVideoGatewayService(repo, noopVideoKeyEncryptor{}, cfgWithVideoBudget(1.5, 30.0), nil, nil, nil)

	task, err := svc.CreateTask(ctx, VideoTaskCreateParams{
		ProviderAccountID: providerID,
		TaskType:          VideoTaskTypeTextToVideo,
		Prompt:            "real-cap single clip",
		CreatedBy:         7,
		Duration:          5, // cost = 1.5 × 5 = 7.5 <= cap 30 => allowed
	})
	if err != nil {
		t.Fatalf("expected the real cap (30) to admit a normal 5s clip, got %v", err)
	}
	if task == nil || task.ID == 0 {
		t.Fatal("expected a persisted task within the real cap")
	}
	if len(repo.tasks) != 1 {
		t.Fatalf("expected one task persisted, got %d", len(repo.tasks))
	}
}
