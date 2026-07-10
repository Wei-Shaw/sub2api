package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

// formAChainOpts configures a single drive of the FULL Form A product chain. The repo,
// service (gate armed) and provider account — including the base URL and key — are built
// by the caller (newFormAHarnessSvc) and passed to the driver; these knobs only tune the
// task and the poll cadence.
type formAChainOpts struct {
	maxTicks    int
	duration    int
	model       string
	aspectRatio string        // optional; empty => CreateTask default 16:9
	tickDelay   time.Duration // sleep between worker ticks (0 for mock; ~poll interval for real)
}

// formAChainResult captures every observable channel of the Form A chain.
type formAChainResult struct {
	created   *VideoTask
	createErr error
	task      *VideoTask
	events    []*VideoTaskEvent
	auditPath string
}

// driveFormASeedanceChain runs the REAL Form A product chain end to end:
//
//	svc.CreateTask  → VA1 budget gate → DB persist (queued)
//	  worker ProcessRunnableTasks → submitTask → seedanceVideoAdapter.CreateTask (HTTP)
//	  worker ProcessRunnableTasks → pollTask   → seedanceVideoAdapter.PollTask   (HTTP)
//	  → terminal status persisted + (on success) VA1 charge
//
// It is the single driver shared by the in-memory mock proof (this file) and the
// build-tagged real Form A smoke (the B-2 switch). The ONLY difference between mock and
// real is opts.baseURL/apiKey — everything between CreateTask and the persisted result is
// the production code path (budget gate is in the chain, NOT bypassed; key is decrypted
// from the DB account exactly as in production). cfg is supplied by the caller via the
// package config builder; here we always wire ProvideVideoGatewayService so the gate is
// armed from config.
func driveFormASeedanceChain(t *testing.T, repo *memoryVideoGatewayRepo, svc *VideoGatewayService, providerID int64, o formAChainOpts) *formAChainResult {
	t.Helper()
	ctx := context.Background()

	model := o.model
	if model == "" {
		model = "doubao-seedance-2-0-260128"
	}
	duration := o.duration
	if duration == 0 {
		duration = 5
	}

	created, err := svc.CreateTask(ctx, VideoTaskCreateParams{
		ProviderAccountID: providerID,
		TaskType:          VideoTaskTypeTextToVideo,
		Model:             model,
		Prompt:            "form A single-shot harness clip",
		CreatedBy:         21,
		Duration:          duration,
		AspectRatio:       strings.TrimSpace(o.aspectRatio),
	})
	res := &formAChainResult{created: created, createErr: err}
	res.auditPath = os.Getenv("SUB2API_VIDEO_REDACTED_EVENT_LOG")
	if err != nil {
		// Budget gate (or routing) rejected before any provider dispatch.
		return res
	}

	ticks := o.maxTicks
	if ticks == 0 {
		ticks = 80
	}
	for i := 0; i < ticks; i++ {
		if i > 0 && o.tickDelay > 0 {
			time.Sleep(o.tickDelay)
		}
		if perr := svc.ProcessRunnableTasks(ctx, 10, 15*time.Minute); perr != nil {
			t.Fatalf("process tick %d: %v", i, perr)
		}
		cur, _, gerr := svc.GetTask(ctx, created.ID, 21, true)
		if gerr == nil && IsTerminalVideoStatus(cur.Status) {
			break
		}
	}

	task, events, gerr := svc.GetTask(ctx, created.ID, 21, true)
	if gerr != nil {
		t.Fatalf("get final task: %v", gerr)
	}
	res.task = task
	res.events = events
	return res
}

// newFormAHarnessSvc builds the in-memory repo + a smoke-authorized seedance account +
// the PRODUCTION DI service (gate armed from config) for a Form A chain drive.
func newFormAHarnessSvc(t *testing.T, baseURL, apiKey string, costPerSecond, perCallBudget float64) (*memoryVideoGatewayRepo, *VideoGatewayService, int64) {
	t.Helper()
	repo := newMemoryVideoGatewayRepo()
	providerID := seedSmokeAuthorizedSeedanceProvider(repo, apiKey, baseURL)
	svc := ProvideVideoGatewayService(repo, noopVideoKeyEncryptor{}, cfgWithVideoBudget(costPerSecond, perCallBudget), nil, nil, nil)
	return repo, svc, providerID
}

// TestFormASeedanceHappyPathInMemory is the harness proof: the full Form A chain reaches
// SUCCEEDED through the real seedanceVideoAdapter HTTP path against a LOCAL mock-Ark server
// (no real network/cost). It proves: (a) the armed budget gate ADMITS a normal 5s clip
// (7.5 <= cap 30) — i.e. the chain runs through the gate, not around it; (b) exactly ONE
// upstream create is issued (no double-submit); (c) the SSRF-validated result_url is stored;
// (d) the redacted audit log is written and never contains the key.
func TestFormASeedanceHappyPathInMemory(t *testing.T) {
	const fakeKey = "forma-mock-key-1234567890" // credible length, clearly fake, never real
	var creates, polls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			creates++
			w.WriteHeader(http.StatusOK)
			// status normalizes to Submitted so the worker advances to the poll branch.
			_, _ = w.Write([]byte(`{"id":"ark-task-1","status":"submitted"}`))
			return
		}
		polls++
		w.WriteHeader(http.StatusOK)
		if polls >= 2 {
			_, _ = w.Write([]byte(`{"id":"ark-task-1","status":"succeeded","content":{"video_url":"https://ark-content.cn-beijing.volces.com/v/ok.mp4"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"id":"ark-task-1","status":"running"}`))
	}))
	t.Cleanup(srv.Close)

	t.Setenv("SUB2API_VIDEO_REAL_SMOKE_ENABLED", "1")
	t.Setenv("SUB2API_VIDEO_REDACTED_EVENT_LOG", t.TempDir()+"/audit.log")
	t.Setenv("SUB2API_VIDEO_URL_ALLOWLIST", "volces.com")

	repo, svc, providerID := newFormAHarnessSvc(t, srv.URL, fakeKey, 1.5, 30.0)
	res := driveFormASeedanceChain(t, repo, svc, providerID, formAChainOpts{})

	if res.createErr != nil {
		t.Fatalf("armed gate should ADMIT a normal 5s clip (7.5 <= 30), got %v", res.createErr)
	}
	if res.task.Status != VideoStatusSucceeded {
		t.Fatalf("expected the full chain to reach succeeded, got %s (err=%q)", res.task.Status, res.task.ErrorMessage)
	}
	if res.task.ResultURL != "https://ark-content.cn-beijing.volces.com/v/ok.mp4" {
		t.Fatalf("expected the SSRF-validated result_url stored, got %q", res.task.ResultURL)
	}
	if creates != 1 {
		t.Fatalf("expected exactly ONE upstream create (no double-submit), got %d", creates)
	}
	if res.task.PollCount < 1 {
		t.Fatalf("expected the worker to have polled at least once, got poll_count=%d", res.task.PollCount)
	}
	raw, rerr := os.ReadFile(res.auditPath)
	if rerr != nil || len(raw) == 0 {
		t.Fatalf("expected redacted audit log written, err=%v len=%d", rerr, len(raw))
	}
	if strings.Contains(string(raw), fakeKey) {
		t.Fatalf("audit log leaked the key: %q", string(raw))
	}
	// The full event timeline must exist (routed/queued/submitted/.../succeeded).
	if len(res.events) < 4 {
		t.Fatalf("expected the full Form A event timeline, got %d events", len(res.events))
	}
}

// TestFormASeedanceBudgetGateInterceptsInChain proves the gate is genuinely IN the Form A
// chain: at cap 2 a 5s clip estimates 7.5 > 2 and is rejected at CreateTask — no task is
// persisted and the upstream is NEVER reached. (Complements the admit case above.)
func TestFormASeedanceBudgetGateInterceptsInChain(t *testing.T) {
	const fakeKey = "forma-mock-key-1234567890"
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"x","status":"submitted"}`))
	}))
	t.Cleanup(srv.Close)

	t.Setenv("SUB2API_VIDEO_REAL_SMOKE_ENABLED", "1")
	t.Setenv("SUB2API_VIDEO_REDACTED_EVENT_LOG", t.TempDir()+"/audit.log")
	t.Setenv("SUB2API_VIDEO_URL_ALLOWLIST", "volces.com")

	repo, svc, providerID := newFormAHarnessSvc(t, srv.URL, fakeKey, 1.5, 2.0) // cap 2 => intercept
	res := driveFormASeedanceChain(t, repo, svc, providerID, formAChainOpts{})

	if res.createErr == nil {
		t.Fatal("expected the armed gate to intercept the over-cap create")
	}
	if !strings.Contains(res.createErr.Error(), "exceeds per-call cap") {
		t.Fatalf("expected an over-cap interception error, got %v", res.createErr)
	}
	if called {
		t.Fatal("an intercepted create must NEVER reach the provider")
	}
	if len(repo.tasks) != 0 {
		t.Fatalf("intercepted create must not persist a task, got %d", len(repo.tasks))
	}
}

// TestFormASeedanceNoDoubleSubmitOnQueuedCreate guards the double-billing risk: if the
// upstream CREATE response status normalizes to "queued" (an Ark token the adapter
// anticipates but whose real value is UNVERIFIED), the worker must NOT re-issue a second
// billed create on the next tick. The worker advances a successfully-created task out of
// "queued" to "submitted" so the next tick POLLS. Asserts exactly ONE upstream create.
func TestFormASeedanceNoDoubleSubmitOnQueuedCreate(t *testing.T) {
	const fakeKey = "forma-mock-key-1234567890"
	var creates, polls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			creates++
			w.WriteHeader(http.StatusOK)
			// create-response status that NormalizeStatus maps to VideoStatusQueued.
			_, _ = w.Write([]byte(`{"id":"ark-q","status":"queued"}`))
			return
		}
		polls++
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"ark-q","status":"succeeded","content":{"video_url":"https://ark-content.cn-beijing.volces.com/v/ok.mp4"}}`))
	}))
	t.Cleanup(srv.Close)

	t.Setenv("SUB2API_VIDEO_REAL_SMOKE_ENABLED", "1")
	t.Setenv("SUB2API_VIDEO_REDACTED_EVENT_LOG", t.TempDir()+"/audit.log")
	t.Setenv("SUB2API_VIDEO_URL_ALLOWLIST", "volces.com")

	repo, svc, providerID := newFormAHarnessSvc(t, srv.URL, fakeKey, 1.5, 30.0)
	res := driveFormASeedanceChain(t, repo, svc, providerID, formAChainOpts{})

	if res.createErr != nil {
		t.Fatalf("create should be admitted: %v", res.createErr)
	}
	if creates != 1 {
		t.Fatalf("a 'queued' create-status must NOT trigger a re-submit; got %d upstream creates (double-billing!)", creates)
	}
	if res.task.Status != VideoStatusSucceeded {
		t.Fatalf("expected the chain to reach succeeded after a queued-create, got %s (err=%q)", res.task.Status, res.task.ErrorMessage)
	}
	if polls < 1 {
		t.Fatal("expected at least one poll after the create advanced to submitted")
	}
}
