package service

import (
	"testing"
	"time"
)

func TestCodexSnapshotBaseTime(t *testing.T) {
	fallback := time.Date(2026, 2, 20, 9, 0, 0, 0, time.UTC)

	t.Run("nil snapshot uses fallback", func(t *testing.T) {
		got := codexSnapshotBaseTime(nil, fallback)
		if !got.Equal(fallback) {
			t.Fatalf("got %v, want fallback %v", got, fallback)
		}
	})

	t.Run("empty updatedAt uses fallback", func(t *testing.T) {
		got := codexSnapshotBaseTime(&OpenAICodexUsageSnapshot{}, fallback)
		if !got.Equal(fallback) {
			t.Fatalf("got %v, want fallback %v", got, fallback)
		}
	})

	t.Run("valid updatedAt wins", func(t *testing.T) {
		got := codexSnapshotBaseTime(&OpenAICodexUsageSnapshot{UpdatedAt: "2026-02-16T10:00:00Z"}, fallback)
		want := time.Date(2026, 2, 16, 10, 0, 0, 0, time.UTC)
		if !got.Equal(want) {
			t.Fatalf("got %v, want %v", got, want)
		}
	})

	t.Run("invalid updatedAt uses fallback", func(t *testing.T) {
		got := codexSnapshotBaseTime(&OpenAICodexUsageSnapshot{UpdatedAt: "invalid"}, fallback)
		if !got.Equal(fallback) {
			t.Fatalf("got %v, want fallback %v", got, fallback)
		}
	})
}

func TestCodexResetAtRFC3339(t *testing.T) {
	base := time.Date(2026, 2, 16, 10, 0, 0, 0, time.UTC)

	t.Run("nil reset returns nil", func(t *testing.T) {
		if got := codexResetAtRFC3339(base, nil); got != nil {
			t.Fatalf("expected nil, got %v", *got)
		}
	})

	t.Run("positive seconds", func(t *testing.T) {
		sec := 90
		got := codexResetAtRFC3339(base, &sec)
		if got == nil {
			t.Fatal("expected non-nil")
		}
		if *got != "2026-02-16T10:01:30Z" {
			t.Fatalf("got %s, want %s", *got, "2026-02-16T10:01:30Z")
		}
	})

	t.Run("negative seconds clamp to base", func(t *testing.T) {
		sec := -3
		got := codexResetAtRFC3339(base, &sec)
		if got == nil {
			t.Fatal("expected non-nil")
		}
		if *got != "2026-02-16T10:00:00Z" {
			t.Fatalf("got %s, want %s", *got, "2026-02-16T10:00:00Z")
		}
	})
}

func TestBuildCodexUsageExtraUpdates_UsesSnapshotUpdatedAt(t *testing.T) {
	primaryUsed := 88.0
	primaryReset := 86400
	primaryWindow := 10080
	secondaryUsed := 12.0
	secondaryReset := 3600
	secondaryWindow := 300

	snapshot := &OpenAICodexUsageSnapshot{
		PrimaryUsedPercent:         &primaryUsed,
		PrimaryResetAfterSeconds:   &primaryReset,
		PrimaryWindowMinutes:       &primaryWindow,
		SecondaryUsedPercent:       &secondaryUsed,
		SecondaryResetAfterSeconds: &secondaryReset,
		SecondaryWindowMinutes:     &secondaryWindow,
		UpdatedAt:                  "2026-02-16T10:00:00Z",
	}

	updates := buildCodexUsageExtraUpdates(snapshot, time.Date(2026, 2, 20, 8, 0, 0, 0, time.UTC))
	if updates == nil {
		t.Fatal("expected non-nil updates")
	}

	if got := updates["codex_usage_updated_at"]; got != "2026-02-16T10:00:00Z" {
		t.Fatalf("codex_usage_updated_at = %v, want %s", got, "2026-02-16T10:00:00Z")
	}
	if got := updates["codex_5h_reset_at"]; got != "2026-02-16T11:00:00Z" {
		t.Fatalf("codex_5h_reset_at = %v, want %s", got, "2026-02-16T11:00:00Z")
	}
	if got := updates["codex_7d_reset_at"]; got != "2026-02-17T10:00:00Z" {
		t.Fatalf("codex_7d_reset_at = %v, want %s", got, "2026-02-17T10:00:00Z")
	}
}

func TestBuildCodexUsageExtraUpdates_FallbackToNowWhenUpdatedAtInvalid(t *testing.T) {
	primaryUsed := 15.0
	primaryReset := 30
	primaryWindow := 300

	fallbackNow := time.Date(2026, 2, 20, 8, 30, 0, 0, time.UTC)
	snapshot := &OpenAICodexUsageSnapshot{
		PrimaryUsedPercent:       &primaryUsed,
		PrimaryResetAfterSeconds: &primaryReset,
		PrimaryWindowMinutes:     &primaryWindow,
		UpdatedAt:                "invalid-time",
	}

	updates := buildCodexUsageExtraUpdates(snapshot, fallbackNow)
	if updates == nil {
		t.Fatal("expected non-nil updates")
	}

	if got := updates["codex_usage_updated_at"]; got != "2026-02-20T08:30:00Z" {
		t.Fatalf("codex_usage_updated_at = %v, want %s", got, "2026-02-20T08:30:00Z")
	}
	if got := updates["codex_5h_reset_at"]; got != "2026-02-20T08:30:30Z" {
		t.Fatalf("codex_5h_reset_at = %v, want %s", got, "2026-02-20T08:30:30Z")
	}
}

func TestBuildCodexUsageExtraUpdates_ClampNegativeResetSeconds(t *testing.T) {
	primaryUsed := 90.0
	primaryReset := 7200
	primaryWindow := 10080
	secondaryUsed := 100.0
	secondaryReset := -15
	secondaryWindow := 300

	snapshot := &OpenAICodexUsageSnapshot{
		PrimaryUsedPercent:         &primaryUsed,
		PrimaryResetAfterSeconds:   &primaryReset,
		PrimaryWindowMinutes:       &primaryWindow,
		SecondaryUsedPercent:       &secondaryUsed,
		SecondaryResetAfterSeconds: &secondaryReset,
		SecondaryWindowMinutes:     &secondaryWindow,
		UpdatedAt:                  "2026-02-16T10:00:00Z",
	}

	updates := buildCodexUsageExtraUpdates(snapshot, time.Time{})
	if updates == nil {
		t.Fatal("expected non-nil updates")
	}

	if got := updates["codex_5h_reset_after_seconds"]; got != -15 {
		t.Fatalf("codex_5h_reset_after_seconds = %v, want %d", got, -15)
	}
	if got := updates["codex_5h_reset_at"]; got != "2026-02-16T10:00:00Z" {
		t.Fatalf("codex_5h_reset_at = %v, want %s", got, "2026-02-16T10:00:00Z")
	}
}

func TestBuildCodexUsageExtraUpdates_NilSnapshot(t *testing.T) {
	if got := buildCodexUsageExtraUpdates(nil, time.Now()); got != nil {
		t.Fatalf("expected nil updates, got %v", got)
	}
}

func TestBuildCodexUsageExtraUpdates_WithoutNormalizedWindowFields(t *testing.T) {
	primaryUsed := 42.0
	fallbackNow := time.Date(2026, 2, 20, 9, 15, 0, 0, time.UTC)
	snapshot := &OpenAICodexUsageSnapshot{
		PrimaryUsedPercent: &primaryUsed,
		UpdatedAt:          "",
	}

	updates := buildCodexUsageExtraUpdates(snapshot, fallbackNow)
	if updates == nil {
		t.Fatal("expected non-nil updates")
	}

	if got := updates["codex_usage_updated_at"]; got != "2026-02-20T09:15:00Z" {
		t.Fatalf("codex_usage_updated_at = %v, want %s", got, "2026-02-20T09:15:00Z")
	}
	if _, ok := updates["codex_5h_reset_at"]; ok {
		t.Fatalf("did not expect codex_5h_reset_at in updates: %v", updates["codex_5h_reset_at"])
	}
	if _, ok := updates["codex_7d_reset_at"]; ok {
		t.Fatalf("did not expect codex_7d_reset_at in updates: %v", updates["codex_7d_reset_at"])
	}
}
