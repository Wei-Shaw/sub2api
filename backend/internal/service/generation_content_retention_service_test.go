package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

func retentionCfg(days, batch, interval int, dryRun bool) *config.Config {
	return &config.Config{Gateway: config.GatewayConfig{ContentRetention: config.ContentRetentionConfig{
		RetentionDays:   days,
		BatchSize:       batch,
		IntervalSeconds: interval,
		DryRun:          dryRun,
	}}}
}

func retentionStartupCfg(captureEnabled, retentionEnabled bool) *config.Config {
	return &config.Config{Gateway: config.GatewayConfig{
		ContentCapture: config.ContentCaptureConfig{Enabled: captureEnabled},
		ContentRetention: config.ContentRetentionConfig{
			Enabled:         retentionEnabled,
			RetentionDays:   90,
			BatchSize:       1,
			IntervalSeconds: 60,
			DryRun:          true,
		},
	}}
}

type countingRetentionRepo struct {
	fakeGenContentRepo
	purgeCalls chan struct{}
}

func (r *countingRetentionRepo) PurgeExpiredContent(ctx context.Context, cutoff time.Time, batch int, dryRun bool) (int64, error) {
	select {
	case r.purgeCalls <- struct{}{}:
	default:
	}
	return r.fakeGenContentRepo.PurgeExpiredContent(ctx, cutoff, batch, dryRun)
}

func contentRow(ageDays int, hasContent bool) *GenerationContent {
	r := &GenerationContent{CreatedAt: time.Now().AddDate(0, 0, -ageDays)}
	if hasContent {
		r.PromptRedacted = "prompt"
		r.ResponseRedacted = "response"
	}
	return r
}

func TestNewGenerationContentRetentionService_UsesConfig(t *testing.T) {
	svc := NewGenerationContentRetentionService(&fakeGenContentRepo{}, retentionCfg(30, 100, 120, true))
	if svc.retentionDays != 30 || svc.batch != 100 || svc.interval != 120*time.Second || !svc.dryRun {
		t.Fatalf("config not applied: days=%d batch=%d interval=%s dryRun=%v", svc.retentionDays, svc.batch, svc.interval, svc.dryRun)
	}
}

func TestNewGenerationContentRetentionService_Defaults(t *testing.T) {
	svc := NewGenerationContentRetentionService(&fakeGenContentRepo{}, nil)
	if svc.retentionDays != defaultGenerationRetentionDays || svc.batch != defaultGenerationRetentionBatchSize {
		t.Fatalf("defaults not applied: days=%d batch=%d", svc.retentionDays, svc.batch)
	}
	if svc.interval != time.Duration(defaultGenerationRetentionIntervalSeconds)*time.Second {
		t.Fatalf("default interval not applied: %s", svc.interval)
	}
}

func TestNewGenerationContentRetentionService_ClampsRetentionDays(t *testing.T) {
	// 配置写了过小的保留天数（3 天）→ 必须夹紧到 7，护住看板近 7 日窗口。
	svc := NewGenerationContentRetentionService(&fakeGenContentRepo{}, retentionCfg(3, 0, 0, false))
	if svc.retentionDays != minGenerationRetentionDays {
		t.Fatalf("retentionDays not clamped: got %d, want %d", svc.retentionDays, minGenerationRetentionDays)
	}
}

func TestProvideGenerationContentRetentionServiceRequiresCaptureAndRetentionFlags(t *testing.T) {
	cases := []struct {
		name string
		cfg  *config.Config
	}{
		{name: "nil config", cfg: nil},
		{name: "capture off retention on", cfg: retentionStartupCfg(false, true)},
		{name: "capture on retention off", cfg: retentionStartupCfg(true, false)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if svc := ProvideGenerationContentRetentionService(&fakeGenContentRepo{}, tc.cfg); svc != nil {
				defer svc.Stop()
				t.Fatalf("retention daemon must not start for %s", tc.name)
			}
		})
	}
}

func TestProvideGenerationContentRetentionServiceStartsWhenFlagsEnabled(t *testing.T) {
	repo := &countingRetentionRepo{
		fakeGenContentRepo: fakeGenContentRepo{rows: []*GenerationContent{
			contentRow(100, true),
		}},
		purgeCalls: make(chan struct{}, 1),
	}
	svc := ProvideGenerationContentRetentionService(repo, retentionStartupCfg(true, true))
	if svc == nil {
		t.Fatal("expected retention daemon when content capture and retention flags are enabled")
	}
	defer svc.Stop()

	select {
	case <-repo.purgeCalls:
	case <-time.After(2 * time.Second):
		t.Fatal("retention daemon did not schedule its startup cleanup")
	}
}

func TestGenerationContentRetention_RunOnceDryRun_NoSideEffects(t *testing.T) {
	repo := &fakeGenContentRepo{rows: []*GenerationContent{
		contentRow(100, true), // 过期 + 有内容 → 会命中
		contentRow(1, true),   // 未过期 → 不动
	}}
	svc := NewGenerationContentRetentionService(repo, nil) // days=90
	n, err := svc.RunOnce(context.Background(), true)
	if err != nil {
		t.Fatalf("dry-run err: %v", err)
	}
	if n != 1 {
		t.Fatalf("dry-run should count 1 expired row, got %d", n)
	}
	// 关键：dry-run 零副作用——两行内容都还在。
	if repo.rows[0].PromptRedacted == "" || repo.rows[1].PromptRedacted == "" {
		t.Fatalf("dry-run must not blank any content: %+v", repo.rows)
	}
}

func TestGenerationContentRetention_RunOncePurgesOnlyExpired(t *testing.T) {
	expired := contentRow(100, true)
	fresh := contentRow(1, true)
	repo := &fakeGenContentRepo{rows: []*GenerationContent{expired, fresh}}
	svc := NewGenerationContentRetentionService(repo, nil) // days=90

	n, err := svc.RunOnce(context.Background(), false)
	if err != nil {
		t.Fatalf("purge err: %v", err)
	}
	if n != 1 {
		t.Fatalf("should purge exactly 1 expired row, got %d", n)
	}
	if expired.PromptRedacted != "" || expired.ResponseRedacted != "" {
		t.Errorf("expired content not blanked: %+v", expired)
	}
	if fresh.PromptRedacted == "" || fresh.ResponseRedacted == "" {
		t.Errorf("fresh content was wrongly blanked: %+v", fresh)
	}
}

func TestGenerationContentRetention_RunOnceDrainsMultipleBatches(t *testing.T) {
	rows := make([]*GenerationContent, 0, 5)
	for i := 0; i < 5; i++ {
		rows = append(rows, contentRow(100, true)) // 全过期 + 有内容
	}
	repo := &fakeGenContentRepo{rows: rows}
	svc := NewGenerationContentRetentionService(repo, retentionCfg(90, 2, 0, false)) // batch=2

	n, err := svc.RunOnce(context.Background(), false)
	if err != nil {
		t.Fatalf("purge err: %v", err)
	}
	if n != 5 {
		t.Fatalf("should drain all 5 expired across batches, got %d", n)
	}
	for i, r := range repo.rows {
		if r.PromptRedacted != "" || r.ResponseRedacted != "" {
			t.Errorf("row %d not blanked after drain: %+v", i, r)
		}
	}
}

func TestGenerationContentRetention_RunOnceFailOpenOnRepoError(t *testing.T) {
	repo := &fakeGenContentRepo{err: errors.New("db down")}
	svc := NewGenerationContentRetentionService(repo, nil)
	if _, err := svc.RunOnce(context.Background(), false); err == nil {
		t.Fatalf("expected error from repo to propagate to RunOnce")
	}
	// cleanupOnce 必须吞错不 panic（fail-open）。
	svc.cleanupOnce()
}

func TestGenerationContentRetention_NilSafe(t *testing.T) {
	var svc *GenerationContentRetentionService
	if n, err := svc.RunOnce(context.Background(), false); n != 0 || err != nil {
		t.Fatalf("nil service RunOnce should be no-op, got n=%d err=%v", n, err)
	}
	svc.Start() // nil-safe
	svc.Stop()  // nil-safe
	// nil repo 也应安全。
	svc2 := NewGenerationContentRetentionService(nil, nil)
	if n, err := svc2.RunOnce(context.Background(), false); n != 0 || err != nil {
		t.Fatalf("nil repo RunOnce should be no-op, got n=%d err=%v", n, err)
	}
}
