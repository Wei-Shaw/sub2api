package service

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

type recordingVideoDispatchReconciliationRepo struct {
	tasks  []*VideoTask
	writes int
}

func (r *recordingVideoDispatchReconciliationRepo) MarkDispatchingCAS(context.Context, *VideoTask, *VideoTaskEvent) (bool, error) {
	r.writes++
	return true, nil
}

func (r *recordingVideoDispatchReconciliationRepo) MarkDispatchAcceptedCAS(context.Context, *VideoTask, *VideoTaskEvent) (bool, error) {
	r.writes++
	return true, nil
}

func (r *recordingVideoDispatchReconciliationRepo) MarkDispatchUnknownCAS(context.Context, *VideoTask, *VideoTaskEvent) (bool, error) {
	r.writes++
	return true, nil
}

func (r *recordingVideoDispatchReconciliationRepo) ListDispatchUnknownTasks(context.Context, int) ([]*VideoTask, error) {
	return r.tasks, nil
}

func TestVideoDispatchReconcilerDryRunReportsUnknownWithoutWritesOrSecrets(t *testing.T) {
	repo := &recordingVideoDispatchReconciliationRepo{tasks: []*VideoTask{
		{
			ID:                71,
			Status:            VideoStatusSubmitted,
			DispatchState:     VideoDispatchStateUnknown,
			Prompt:            "credential=must-not-leak",
			ReferenceImageURL: "https://secret.example/path?token=must-not-leak",
			ErrorMessage:      "Bearer must-not-leak",
		},
	}}
	reconciler := NewVideoDispatchReconciler(repo)

	findings, err := reconciler.DryRun(context.Background(), 10)
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if repo.writes != 0 {
		t.Fatalf("dry run performed %d writes", repo.writes)
	}
	if len(findings) != 1 {
		t.Fatalf("findings = %#v", findings)
	}
	if findings[0].Severity != "warning" || findings[0].Code != "video_dispatch_unknown" || findings[0].TaskID != 71 || findings[0].RecommendedAction == "" {
		t.Fatalf("unexpected finding: %#v", findings[0])
	}
	encoded, err := json.Marshal(findings)
	if err != nil {
		t.Fatalf("marshal findings: %v", err)
	}
	for _, forbidden := range []string{"must-not-leak", "secret.example", "Bearer", "credential", "http://", "https://"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("dry run leaked %q in %s", forbidden, encoded)
		}
	}
}
