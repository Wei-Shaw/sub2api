package service

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestCollectSelectionFailureStats(t *testing.T) {
	svc := &GatewayService{}
	model := "gpt-5.4"
	resetAt := time.Now().Add(2 * time.Minute).Format(time.RFC3339)

	accounts := []Account{
		// excluded
		{
			ID:          1,
			Platform:    PlatformOpenAI,
			Status:      StatusActive,
			Schedulable: true,
		},
		// unschedulable
		{
			ID:          2,
			Platform:    PlatformOpenAI,
			Status:      StatusActive,
			Schedulable: false,
		},
		// platform filtered
		{
			ID:          3,
			Platform:    PlatformAntigravity,
			Status:      StatusActive,
			Schedulable: true,
		},
		// model unsupported
		{
			ID:          4,
			Platform:    PlatformOpenAI,
			Status:      StatusActive,
			Schedulable: true,
			Credentials: map[string]any{
				"model_mapping": map[string]any{
					"gpt-image": "gpt-image",
				},
			},
		},
		// model rate limited
		{
			ID:          5,
			Platform:    PlatformOpenAI,
			Status:      StatusActive,
			Schedulable: true,
			Extra: map[string]any{
				"model_rate_limits": map[string]any{
					model: map[string]any{
						"rate_limit_reset_at": resetAt,
					},
				},
			},
		},
		// eligible
		{
			ID:          6,
			Platform:    PlatformOpenAI,
			Status:      StatusActive,
			Schedulable: true,
		},
	}

	excluded := map[int64]struct{}{1: {}}
	stats := svc.collectSelectionFailureStats(context.Background(), accounts, model, PlatformOpenAI, excluded, false)

	if stats.Total != 6 {
		t.Fatalf("total=%d want=6", stats.Total)
	}
	if stats.Excluded != 1 {
		t.Fatalf("excluded=%d want=1", stats.Excluded)
	}
	if stats.Unschedulable != 1 {
		t.Fatalf("unschedulable=%d want=1", stats.Unschedulable)
	}
	if stats.PlatformFiltered != 1 {
		t.Fatalf("platform_filtered=%d want=1", stats.PlatformFiltered)
	}
	if stats.ModelUnsupported != 1 {
		t.Fatalf("model_unsupported=%d want=1", stats.ModelUnsupported)
	}
	if stats.ModelRateLimited != 1 {
		t.Fatalf("model_rate_limited=%d want=1", stats.ModelRateLimited)
	}
	if stats.Eligible != 1 {
		t.Fatalf("eligible=%d want=1", stats.Eligible)
	}
}

func TestDiagnoseSelectionFailure_UnschedulableDetail(t *testing.T) {
	svc := &GatewayService{}
	acc := &Account{
		ID:          7,
		Platform:    PlatformOpenAI,
		Status:      StatusActive,
		Schedulable: false,
	}

	diagnosis := svc.diagnoseSelectionFailure(context.Background(), acc, "gpt-5.4", PlatformOpenAI, map[int64]struct{}{}, false)
	if diagnosis.Category != "unschedulable" {
		t.Fatalf("category=%s want=unschedulable", diagnosis.Category)
	}
	if diagnosis.Detail != "generic_unschedulable" {
		t.Fatalf("detail=%s want=generic_unschedulable", diagnosis.Detail)
	}
}

func TestDiagnoseSelectionFailure_ModelRateLimitedDetail(t *testing.T) {
	svc := &GatewayService{}
	model := "gpt-5.4"
	resetAt := time.Now().Add(2 * time.Minute).UTC().Format(time.RFC3339)
	acc := &Account{
		ID:          8,
		Platform:    PlatformOpenAI,
		Status:      StatusActive,
		Schedulable: true,
		Extra: map[string]any{
			"model_rate_limits": map[string]any{
				model: map[string]any{
					"rate_limit_reset_at": resetAt,
				},
			},
		},
	}

	diagnosis := svc.diagnoseSelectionFailure(context.Background(), acc, model, PlatformOpenAI, map[int64]struct{}{}, false)
	if diagnosis.Category != "model_rate_limited" {
		t.Fatalf("category=%s want=model_rate_limited", diagnosis.Category)
	}
	if !strings.Contains(diagnosis.Detail, "remaining=") {
		t.Fatalf("detail=%s want contains remaining=", diagnosis.Detail)
	}
}
