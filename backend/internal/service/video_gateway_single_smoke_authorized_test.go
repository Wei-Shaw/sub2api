package service

import (
	"context"
	"strings"
	"testing"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// authReasonAbsent reports whether the smoke gate's "not authorized" reason is absent
// from the blocked-reasons list for `task`. The other gate conditions are satisfied by
// the caller's env so this isolates the single_smoke_authorized check.
func authReasonAbsent(reasons []string) bool {
	for _, r := range reasons {
		if strings.Contains(r, "single smoke authorization") {
			return false
		}
	}
	return true
}

func authorizedSmokeTask() *VideoTask {
	return &VideoTask{Model: "doubao-seedance-2-0-260128", Prompt: "x", Duration: 5}
}

func smokeDurationReasonPresent(reasons []string) bool {
	for _, r := range reasons {
		if strings.Contains(r, "single smoke duration") {
			return true
		}
	}
	return false
}

// setSmokeGateEnv opens the three env gates so the ONLY remaining gate is the
// single_smoke_authorized metadata under test.
func setSmokeGateEnv(t *testing.T) {
	t.Helper()
	t.Setenv("SUB2API_VIDEO_REAL_SMOKE_ENABLED", "1")
	t.Setenv("SUB2API_VIDEO_REDACTED_EVENT_LOG", t.TempDir()+"/audit.log")
	t.Setenv("SUB2API_VIDEO_URL_ALLOWLIST", "volces.com")
}

// TestSingleSmokeAuthorizedSetViaCreateProvider proves the admin CREATE path
// (POST /api/v1/admin/video/providers with metadata_json) persists single_smoke_authorized
// and that the gate reads it back as authorized.
func TestSingleSmokeAuthorizedSetViaCreateProvider(t *testing.T) {
	setSmokeGateEnv(t)
	ctx := context.Background()
	repo := newMemoryVideoGatewayRepo()
	svc := NewVideoGatewayService(repo, noopVideoKeyEncryptor{}, nil)

	created, err := svc.CreateProviderAccount(ctx, VideoProviderCreateParams{
		Provider:     VideoProviderSeedance,
		DisplayName:  "Seedance Smoke",
		Enabled:      true,
		APIKey:       "seedance-real-key-placeholder-1234",
		DefaultModel: "doubao-seedance-2-0-260128",
		Metadata:     map[string]any{"single_smoke_authorized": true},
	})
	if err != nil {
		t.Fatalf("create provider: %v", err)
	}

	// Read back through the repo (round-trips the metadata exactly as the DB would).
	stored, err := svc.GetProviderAccount(ctx, created.ID)
	if err != nil {
		t.Fatalf("get provider: %v", err)
	}
	if !metadataBool(stored.Metadata, "single_smoke_authorized") {
		t.Fatalf("expected single_smoke_authorized persisted true, got metadata=%#v", stored.Metadata)
	}

	// Gate: with all env gates open AND the flag set, the authorization reason is gone.
	reasons := seedanceSmokeGateBlockedReasons(stored, authorizedSmokeTask())
	if !authReasonAbsent(reasons) {
		t.Fatalf("authorized account should NOT be blocked for authorization, reasons=%v", reasons)
	}
}

// TestSingleSmokeAuthorizedSetViaUpdateProvider proves the admin UPDATE path
// (PATCH /api/v1/admin/video/providers/:id with metadata_json) flips an unauthorized
// account to authorized: before the patch the gate blocks on authorization; after it does not.
func TestSingleSmokeAuthorizedSetViaUpdateProvider(t *testing.T) {
	setSmokeGateEnv(t)
	ctx := context.Background()
	repo := newMemoryVideoGatewayRepo()
	svc := NewVideoGatewayService(repo, noopVideoKeyEncryptor{}, nil)

	created, err := svc.CreateProviderAccount(ctx, VideoProviderCreateParams{
		Provider:     VideoProviderSeedance,
		DisplayName:  "Seedance Smoke",
		Enabled:      true,
		APIKey:       "seedance-real-key-placeholder-1234",
		DefaultModel: "doubao-seedance-2-0-260128",
		// no metadata => unauthorized initially
	})
	if err != nil {
		t.Fatalf("create provider: %v", err)
	}

	// Before the patch: the authorization reason MUST be present (fail-closed default).
	before, err := svc.GetProviderAccount(ctx, created.ID)
	if err != nil {
		t.Fatalf("get provider (before): %v", err)
	}
	if authReasonAbsent(seedanceSmokeGateBlockedReasons(before, authorizedSmokeTask())) {
		t.Fatal("a provider without single_smoke_authorized must be blocked on authorization")
	}

	// PATCH metadata_json => { "single_smoke_authorized": true }
	authorized := map[string]any{"single_smoke_authorized": true}
	updated, err := svc.UpdateProviderAccount(ctx, created.ID, VideoProviderUpdateParams{
		Metadata: &authorized,
	})
	if err != nil {
		t.Fatalf("update provider: %v", err)
	}
	if !metadataBool(updated.Metadata, "single_smoke_authorized") {
		t.Fatalf("expected single_smoke_authorized after patch, got metadata=%#v", updated.Metadata)
	}

	// After the patch (read back through the repo): authorization reason cleared.
	after, err := svc.GetProviderAccount(ctx, created.ID)
	if err != nil {
		t.Fatalf("get provider (after): %v", err)
	}
	if !authReasonAbsent(seedanceSmokeGateBlockedReasons(after, authorizedSmokeTask())) {
		t.Fatalf("after the patch the account should not be blocked for authorization, reasons=%v",
			seedanceSmokeGateBlockedReasons(after, authorizedSmokeTask()))
	}
}

// TestSingleSmokeAuthorizedAcceptsStringForm proves the gate tolerates the metadata value
// arriving as the JSON string "true"/"1" (admin tooling / DB JSON round-trips), not only a
// native bool — so the write script works regardless of how the value is serialized.
func TestSingleSmokeAuthorizedAcceptsStringForm(t *testing.T) {
	for _, v := range []any{true, "true", "1", "yes"} {
		acc := &VideoProviderAccount{Metadata: map[string]any{"single_smoke_authorized": v}}
		if !metadataBool(acc.Metadata, "single_smoke_authorized") {
			t.Fatalf("expected authorized for metadata value %#v", v)
		}
	}
	for _, v := range []any{false, "false", "0", "", nil} {
		acc := &VideoProviderAccount{Metadata: map[string]any{"single_smoke_authorized": v}}
		if metadataBool(acc.Metadata, "single_smoke_authorized") {
			t.Fatalf("expected NOT authorized for metadata value %#v", v)
		}
	}
}

func TestSeedanceProductionAuthorizedSkipsSmokeDurationLimit(t *testing.T) {
	setSmokeGateEnv(t)

	acc := &VideoProviderAccount{Metadata: map[string]any{"production_authorized": true}}
	task := &VideoTask{Model: "doubao-seedance-2-0-260128", Prompt: "x", Duration: 10}

	reasons := seedanceSmokeGateBlockedReasons(acc, task)
	if !authReasonAbsent(reasons) {
		t.Fatalf("production_authorized account should pass authorization gate, reasons=%v", reasons)
	}
	if smokeDurationReasonPresent(reasons) {
		t.Fatalf("production_authorized account should skip 1-5s smoke duration cap, reasons=%v", reasons)
	}
}

func TestSeedanceUnapprovedProviderKeepsTinySmokeDurationLimit(t *testing.T) {
	setSmokeGateEnv(t)

	acc := &VideoProviderAccount{Metadata: map[string]any{"single_smoke_authorized": true}}
	task := &VideoTask{Model: "doubao-seedance-2-0-260128", Prompt: "x", Duration: 10}

	reasons := seedanceSmokeGateBlockedReasons(acc, task)
	if !smokeDurationReasonPresent(reasons) {
		t.Fatalf("single-smoke account must keep 1-5s duration cap, reasons=%v", reasons)
	}
}

func TestSeedanceAdminProductionRequiresProductionAuthorized(t *testing.T) {
	setSmokeGateEnv(t)
	ctx := context.Background()
	repo := newMemoryVideoGatewayRepo()
	providerID := seedSmokeAuthorizedSeedanceProvider(repo, "seedance-admin-prod-test-key", "https://ark.example.test")
	svc := NewVideoGatewayService(repo, noopVideoKeyEncryptor{}, nil)

	task, err := svc.CreateTask(ctx, VideoTaskCreateParams{
		ProviderAccountID:                      providerID,
		TaskType:                               VideoTaskTypeTextToVideo,
		Model:                                  "doubao-seedance-2-0-260128",
		Prompt:                                 "admin production should require authorization",
		Duration:                               10,
		Resolution:                             "720p",
		CreatedBy:                              8,
		EnforceRealProviderTrial:               false,
		RequireSeedanceProductionAuthorization: true,
	})

	if err == nil {
		t.Fatalf("expected production authorization error, got task=%#v", task)
	}
	if reason := infraerrors.Reason(err); reason != "VIDEO_PRODUCTION_NOT_AUTHORIZED" {
		t.Fatalf("Reason(err) = %q, want VIDEO_PRODUCTION_NOT_AUTHORIZED (err=%v)", reason, err)
	}
	if len(repo.tasks) != 0 {
		t.Fatalf("unauthorized production create must not persist a task, got %d", len(repo.tasks))
	}
}

func TestSeedanceTinySmokeTrialStillAllowedWithoutProductionAuthorized(t *testing.T) {
	setSmokeGateEnv(t)
	ctx := context.Background()
	repo := newMemoryVideoGatewayRepo()
	providerID := seedSmokeAuthorizedSeedanceProvider(repo, "seedance-smoke-test-key", "https://ark.example.test")
	svc := NewVideoGatewayService(repo, noopVideoKeyEncryptor{}, nil)

	task, err := svc.CreateTask(ctx, VideoTaskCreateParams{
		ProviderAccountID:        providerID,
		TaskType:                 VideoTaskTypeTextToVideo,
		Model:                    "doubao-seedance-2-0-260128",
		Prompt:                   "tiny real remains allowed",
		Duration:                 5,
		Resolution:               "720p",
		CreatedBy:                8,
		EnforceRealProviderTrial: true,
	})

	if err != nil {
		t.Fatalf("tiny smoke trial should remain allowed: %v", err)
	}
	if task == nil || task.ID == 0 {
		t.Fatal("expected trial task to be persisted")
	}
	if len(repo.tasks) != 1 {
		t.Fatalf("expected one trial task, got %d", len(repo.tasks))
	}
}
