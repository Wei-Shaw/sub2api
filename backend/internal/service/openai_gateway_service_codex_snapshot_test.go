package service

import (
	"context"
	"maps"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
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

// TestBuildCodexUsageExtraUpdates_FreshAccountUsedPercentNotInverted_Issue2994 locks in the
// canonical "used %" semantics for the 5h window. A fresh account reports a tiny
// secondary-used-percent (~1%); the stored codex_5h_used_percent must equal that value
// directly and must NOT be inverted to ~99%. Regression guard for issue #2994 / the reverted
// commit b65dde63 (PR #2918), which applied `100 - used` and made fresh accounts look
// exhausted, tripping auto-pause and excluding them from scheduling.
func TestBuildCodexUsageExtraUpdates_FreshAccountUsedPercentNotInverted_Issue2994(t *testing.T) {
	secondaryUsed := 1.0 // 5h window: barely used
	secondaryWindow := 300
	primaryUsed := 2.0 // 7d window: barely used
	primaryWindow := 10080

	snapshot := &OpenAICodexUsageSnapshot{
		PrimaryUsedPercent:     &primaryUsed,
		PrimaryWindowMinutes:   &primaryWindow,
		SecondaryUsedPercent:   &secondaryUsed,
		SecondaryWindowMinutes: &secondaryWindow,
		UpdatedAt:              "2026-02-16T10:00:00Z",
	}

	updates := buildCodexUsageExtraUpdates(snapshot, time.Date(2026, 2, 16, 10, 0, 0, 0, time.UTC))
	if updates == nil {
		t.Fatal("expected non-nil updates")
	}

	if got := updates["codex_5h_used_percent"]; got != 1.0 {
		t.Fatalf("codex_5h_used_percent = %v, want 1.0 (direct used%%, NOT inverted to 99)", got)
	}
	if got := updates["codex_7d_used_percent"]; got != 2.0 {
		t.Fatalf("codex_7d_used_percent = %v, want 2.0 (direct used%%, NOT inverted to 98)", got)
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

// 非 5h/周级窗口（生图响应的 1440 分钟窗口）只写原始字段，不能落入 codex_5h_* / codex_7d_*（#8117）。
func TestBuildCodexUsageExtraUpdates_NonCanonicalWindowSkipsCanonicalFields(t *testing.T) {
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name      string
		snapshot  *OpenAICodexUsageSnapshot
		want5h    any // nil 表示不应写入
		want7d    any
		wantPrime float64
	}{
		{
			name: "生图单窗口 1440",
			snapshot: &OpenAICodexUsageSnapshot{
				PrimaryUsedPercent: float64Ptr(98), PrimaryWindowMinutes: intPtr(1440), PrimaryResetAfterSeconds: intPtr(3600),
			},
			wantPrime: 98,
		},
		{
			name: "1440 与周窗口同时出现时只保留周窗口",
			snapshot: &OpenAICodexUsageSnapshot{
				PrimaryUsedPercent: float64Ptr(100), PrimaryWindowMinutes: intPtr(1440), PrimaryResetAfterSeconds: intPtr(3600),
				SecondaryUsedPercent: float64Ptr(9), SecondaryWindowMinutes: intPtr(10080), SecondaryResetAfterSeconds: intPtr(432000),
			},
			want7d:    9.0,
			wantPrime: 100,
		},
		{
			name: "1440 与 5h 窗口同时出现时只保留 5h",
			snapshot: &OpenAICodexUsageSnapshot{
				PrimaryUsedPercent: float64Ptr(20), PrimaryWindowMinutes: intPtr(300), PrimaryResetAfterSeconds: intPtr(600),
				SecondaryUsedPercent: float64Ptr(100), SecondaryWindowMinutes: intPtr(1440), SecondaryResetAfterSeconds: intPtr(3600),
			},
			want5h:    20.0,
			wantPrime: 20,
		},
		{
			name: "仅周窗口 10080 仍归为 7d",
			snapshot: &OpenAICodexUsageSnapshot{
				PrimaryUsedPercent: float64Ptr(75), PrimaryWindowMinutes: intPtr(10080), PrimaryResetAfterSeconds: intPtr(162257),
			},
			want7d:    75.0,
			wantPrime: 75,
		},
		{
			name: "超过一天的长窗口仍归为 7d",
			snapshot: &OpenAICodexUsageSnapshot{
				PrimaryUsedPercent: float64Ptr(40), PrimaryWindowMinutes: intPtr(20160), PrimaryResetAfterSeconds: intPtr(86400),
			},
			want7d:    40.0,
			wantPrime: 40,
		},
		{
			name: "缺少窗口长度的历史数据沿用 primary=7d",
			snapshot: &OpenAICodexUsageSnapshot{
				PrimaryUsedPercent: float64Ptr(80), PrimaryResetAfterSeconds: intPtr(50000),
			},
			want7d:    80.0,
			wantPrime: 80,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			updates := buildCodexUsageExtraUpdates(tt.snapshot, now)
			require.Equal(t, tt.wantPrime, updates["codex_primary_used_percent"])
			require.Equal(t, now.Format(time.RFC3339), updates["codex_usage_updated_at"])
			for window, want := range map[string]any{"5h": tt.want5h, "7d": tt.want7d} {
				if want == nil {
					for _, suffix := range []string{"used_percent", "reset_after_seconds", "window_minutes", "reset_at"} {
						_, ok := updates["codex_"+window+"_"+suffix]
						require.False(t, ok, "codex_%s_%s must not be written: %v", window, suffix, updates)
					}
					continue
				}
				require.Equal(t, want, updates["codex_"+window+"_used_percent"])
			}
		})
	}
}

// #8117：已有真实周快照（10080 分钟 9%）时，生图成功响应带回的 1440 分钟 100% 窗口
// 不能覆盖它，账号也不能因此被 7d 自动暂停。
func TestCodexImageDailyWindowDoesNotAutoPauseAccountBy7d(t *testing.T) {
	now := time.Now().UTC()
	weekly := buildCodexUsageExtraUpdates(&OpenAICodexUsageSnapshot{
		PrimaryUsedPercent: float64Ptr(9), PrimaryWindowMinutes: intPtr(10080), PrimaryResetAfterSeconds: intPtr(432000),
	}, now.Add(-time.Hour))
	image := buildCodexUsageExtraUpdates(&OpenAICodexUsageSnapshot{
		PrimaryUsedPercent: float64Ptr(100), PrimaryWindowMinutes: intPtr(1440), PrimaryResetAfterSeconds: intPtr(3600),
	}, now)

	// UpdateExtra 以 JSONB `||` 合并：本次未写的键保留原值。
	extra := map[string]any{"auto_pause_7d_threshold": 0.98}
	maps.Copy(extra, weekly)
	maps.Copy(extra, image)
	require.Equal(t, 9.0, extra["codex_7d_used_percent"])
	require.Equal(t, 10080, extra["codex_7d_window_minutes"])

	account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: extra}
	paused, decision := shouldAutoPauseOpenAIAccountByQuota(context.Background(), account)
	require.False(t, paused, "image daily window must not pause the account by 7d: %+v", decision)

	// 对照：真实周窗口达到阈值时仍按 7d 暂停。
	maps.Copy(extra, buildCodexUsageExtraUpdates(&OpenAICodexUsageSnapshot{
		PrimaryUsedPercent: float64Ptr(98), PrimaryWindowMinutes: intPtr(10080), PrimaryResetAfterSeconds: intPtr(432000),
	}, now))
	paused, decision = shouldAutoPauseOpenAIAccountByQuota(context.Background(), account)
	require.True(t, paused)
	require.Equal(t, "7d", decision.window)

	// 读取端的原始字段回退与写入端同口径：只有生图窗口时不能推出周用量。
	rawOnly := buildCodexUsageExtraUpdates(&OpenAICodexUsageSnapshot{
		PrimaryUsedPercent: float64Ptr(100), PrimaryWindowMinutes: intPtr(1440), PrimaryResetAfterSeconds: intPtr(3600),
	}, now)
	require.Equal(t, openAIQuotaHeadroomNeutralFactor, openAIQuotaHeadroomFactor(&Account{Extra: rawOnly}, now))
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
