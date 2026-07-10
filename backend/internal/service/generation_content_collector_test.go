package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

type fakeGenContentRepo struct {
	rows []*GenerationContent
	err  error
}

func (f *fakeGenContentRepo) Create(_ context.Context, c *GenerationContent) error {
	if f.err != nil {
		return f.err
	}
	f.rows = append(f.rows, c)
	return nil
}

func (f *fakeGenContentRepo) CreateVideoTaskContent(_ context.Context, c *GenerationContent) error {
	if f.err != nil {
		return f.err
	}
	f.rows = append(f.rows, c)
	return nil
}

// 只读看板方法在采集器写入测试中用不到，提供满足接口的空实现即可。
func (f *fakeGenContentRepo) GetCaptureStats(context.Context) (*GenerationContentStats, error) {
	return &GenerationContentStats{}, nil
}

func (f *fakeGenContentRepo) GetRecent(context.Context, int) ([]GenerationContentSample, error) {
	return nil, nil
}

// PurgeExpiredContent 内存实现：按 created_at < cutoff 且仍有内容过滤，单批封顶 batch；
// dryRun 只计数不改。供保留期清理服务测试真证谓词与批处理排空。
func (f *fakeGenContentRepo) UpdateVideoTaskAdoption(_ context.Context, input GenerationContentAdoptionInput) (*GenerationContentAdoption, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &GenerationContentAdoption{
		TaskID:         input.TaskID,
		AdoptionStatus: input.AdoptionStatus,
		QualityScore:   input.QualityScore,
		Notes:          input.Notes,
		Saved:          true,
	}, nil
}

func (f *fakeGenContentRepo) GetWeeklyReport(context.Context, time.Time, time.Time) (*GenerationContentWeeklyReport, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &GenerationContentWeeklyReport{}, nil
}

func (f *fakeGenContentRepo) PurgeExpiredContent(_ context.Context, cutoff time.Time, batch int, dryRun bool) (int64, error) {
	if f.err != nil {
		return 0, f.err
	}
	if batch <= 0 {
		batch = 500
	}
	var n int64
	for _, r := range f.rows {
		if n >= int64(batch) {
			break
		}
		if r.CreatedAt.Before(cutoff) && (r.PromptRedacted != "" || r.ResponseRedacted != "") {
			n++
			if !dryRun {
				r.PromptRedacted = ""
				r.ResponseRedacted = ""
			}
		}
	}
	return n, nil
}

func enabledContentCaptureCfg() *config.Config {
	return &config.Config{Gateway: config.GatewayConfig{ContentCapture: config.ContentCaptureConfig{Enabled: true}}}
}

func TestCollectorStoresRedactedPrompt(t *testing.T) {
	repo := &fakeGenContentRepo{}
	cfg := enabledContentCaptureCfg()
	collector := NewGenerationContentCollector(repo, cfg)
	gid := int64(7)
	collector.Collect(context.Background(), GenerationContentCaptureArgs{
		RequestID:  "req-1",
		UserID:     11,
		APIKeyID:   22,
		GroupID:    &gid,
		AccountID:  33,
		Model:      "claude-opus-4-8",
		PromptBody: []byte(`{"messages":[{"role":"user","content":"hi a@b.com"}]}`),
		Result:     &ForwardResult{},
	})
	if len(repo.rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(repo.rows))
	}
	row := repo.rows[0]
	if row.RequestID != "req-1" || row.UserID == nil || *row.UserID != 11 || row.APIKeyID == nil || *row.APIKeyID != 22 || row.GroupID == nil || *row.GroupID != 7 || row.AccountID == nil || *row.AccountID != 33 {
		t.Fatalf("attribution not stored correctly: %+v", row)
	}
	if strings.Contains(row.PromptRedacted, "a@b.com") {
		t.Errorf("prompt not redacted: %s", row.PromptRedacted)
	}
	if row.RedactionVersion != generationRedactionVersion {
		t.Errorf("redaction_version = %d, want %d", row.RedactionVersion, generationRedactionVersion)
	}
	if row.PromptBytes == 0 {
		t.Errorf("expected PromptBytes > 0")
	}
}

func TestCollectorFailOpenOnRepoError(t *testing.T) {
	repo := &fakeGenContentRepo{err: errors.New("db down")}
	collector := NewGenerationContentCollector(repo, enabledContentCaptureCfg())
	// must not panic / must return normally despite repo error
	collector.Collect(context.Background(), GenerationContentCaptureArgs{RequestID: "req-2", PromptBody: []byte(`{}`)})
}

func TestCollectorStoresVideoTaskMetadataWithoutAccountAttribution(t *testing.T) {
	repo := &fakeGenContentRepo{}
	collector := NewGenerationContentCollector(repo, enabledContentCaptureCfg())

	if err := collector.CollectVideoTask(context.Background(), VideoGenerationContentCaptureArgs{
		TaskID:         99,
		UserID:         7,
		Model:          "mock-video-v1",
		Prompt:         "render launch storyboard for render@example.test",
		NegativePrompt: "no brand token 13800138000",
		ResultURL:      "/api/v1/video/mock-assets/task-99.mp4",
		Resolution:     "720p",
		Duration:       5,
		AspectRatio:    "16:9",
	}); err != nil {
		t.Fatalf("CollectVideoTask() error = %v", err)
	}

	if len(repo.rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(repo.rows))
	}
	row := repo.rows[0]
	if row.TaskID == nil || *row.TaskID != 99 {
		t.Fatalf("expected task_id 99, got %+v", row.TaskID)
	}
	if row.UserID == nil || *row.UserID != 7 {
		t.Fatalf("expected user_id 7, got %+v", row.UserID)
	}
	if row.APIKeyID != nil || row.GroupID != nil || row.AccountID != nil {
		t.Fatalf("video capture must leave api_key_id/group_id/account_id empty: %+v", row)
	}
	if row.RequestID != "" {
		t.Fatalf("video capture must not invent request_id, got %q", row.RequestID)
	}
	if strings.Contains(row.PromptRedacted, "render@example.test") || strings.Contains(row.PromptRedacted, "13800138000") {
		t.Fatalf("video prompt was not redacted: %s", row.PromptRedacted)
	}
	if !strings.Contains(row.ResponseRedacted, "metadata_summary") || !strings.Contains(row.ResponseRedacted, "task-99.mp4") {
		t.Fatalf("video response summary missing expected metadata: %s", row.ResponseRedacted)
	}
	if row.ResponseBytes == 0 || row.PromptBytes == 0 {
		t.Fatalf("expected byte counts, got prompt=%d response=%d", row.PromptBytes, row.ResponseBytes)
	}
}

func TestCollectorNilSafe(t *testing.T) {
	var c *GenerationContentCollector
	c.Collect(context.Background(), GenerationContentCaptureArgs{RequestID: "x"}) // nil collector
	c2 := NewGenerationContentCollector(nil, enabledContentCaptureCfg())          // nil repo
	c2.Collect(context.Background(), GenerationContentCaptureArgs{RequestID: "y"})
}

func TestGatewayCollectGenerationContentGating(t *testing.T) {
	repo := &fakeGenContentRepo{}

	// disabled config → no-op even with collector wired
	disabled := &GatewayService{cfg: &config.Config{}, generationCollector: NewGenerationContentCollector(repo, &config.Config{})}
	disabled.CollectGenerationContent(context.Background(), GenerationContentCaptureArgs{RequestID: "d", PromptBody: []byte(`{}`)})
	if len(repo.rows) != 0 {
		t.Fatalf("disabled capture must not store, got %d rows", len(repo.rows))
	}

	// nil collector → no-op, no panic
	noCollector := &GatewayService{cfg: enabledContentCaptureCfg()}
	noCollector.CollectGenerationContent(context.Background(), GenerationContentCaptureArgs{RequestID: "n", PromptBody: []byte(`{}`)})

	// enabled + wired → stores
	cfg := enabledContentCaptureCfg()
	on := &GatewayService{cfg: cfg, generationCollector: NewGenerationContentCollector(repo, cfg)}
	on.CollectGenerationContent(context.Background(), GenerationContentCaptureArgs{RequestID: "e", PromptBody: []byte(`{}`)})
	if len(repo.rows) != 1 {
		t.Fatalf("enabled capture must store 1 row, got %d", len(repo.rows))
	}
}
