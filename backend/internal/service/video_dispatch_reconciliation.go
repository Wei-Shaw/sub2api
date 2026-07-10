package service

import (
	"context"
	"fmt"
)

const (
	VideoDispatchFindingSeverityWarning = "warning"
	VideoDispatchFindingCodeUnknown     = "video_dispatch_unknown"
)

type VideoDispatchFinding struct {
	Severity          string `json:"severity"`
	Code              string `json:"code"`
	TaskID            int64  `json:"task_id"`
	RecommendedAction string `json:"recommended_action"`
}

type VideoDispatchReconciler struct {
	repo VideoDispatchRepository
}

func NewVideoDispatchReconciler(repo VideoDispatchRepository) *VideoDispatchReconciler {
	return &VideoDispatchReconciler{repo: repo}
}

// DryRun is intentionally read-only and returns a fixed safe projection. It
// never accepts an adapter/provider dependency, and therefore cannot contact a
// provider or expose task prompts, URLs, response bodies, or credentials.
func (r *VideoDispatchReconciler) DryRun(ctx context.Context, limit int) ([]VideoDispatchFinding, error) {
	if r == nil || r.repo == nil {
		return nil, fmt.Errorf("video dispatch repository is required")
	}
	if limit <= 0 {
		limit = 100
	}
	tasks, err := r.repo.ListDispatchUnknownTasks(ctx, limit)
	if err != nil {
		return nil, fmt.Errorf("list unknown video dispatches: %w", err)
	}
	findings := make([]VideoDispatchFinding, 0, len(tasks))
	for _, task := range tasks {
		if task == nil || task.DispatchState != VideoDispatchStateUnknown {
			continue
		}
		findings = append(findings, VideoDispatchFinding{
			Severity:          VideoDispatchFindingSeverityWarning,
			Code:              VideoDispatchFindingCodeUnknown,
			TaskID:            task.ID,
			RecommendedAction: "verify provider acceptance, then record the upstream task id or close the task without redispatch",
		})
	}
	return findings, nil
}
