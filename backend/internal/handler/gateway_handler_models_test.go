package handler

import (
	"reflect"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
)

func TestExpandOpenAISysAliases(t *testing.T) {
	models := []openai.Model{
		{ID: "alpha", Object: "model", Created: 1, OwnedBy: "owner-a", Type: "model", DisplayName: "Alpha"},
		{ID: "ALPHA-sYs", Object: "model", Created: 99, OwnedBy: "owner-manual", Type: "model", DisplayName: "Manual Alpha Sys"},
		{ID: "beta-SYS", Object: "model", Created: 2, OwnedBy: "owner-b", Type: "model", DisplayName: "Beta Sys"},
		{ID: "gamma", Object: "model", Created: 3, OwnedBy: "owner-c", Type: "model", DisplayName: "Gamma"},
		{ID: "ALPHA", Object: "model", Created: 4, OwnedBy: "owner-dup", Type: "model", DisplayName: "Alpha Duplicate"},
	}

	got := expandOpenAISysAliases(models)
	want := []openai.Model{
		{ID: "alpha", Object: "model", Created: 1, OwnedBy: "owner-a", Type: "model", DisplayName: "Alpha"},
		{ID: "ALPHA-sYs", Object: "model", Created: 99, OwnedBy: "owner-manual", Type: "model", DisplayName: "Manual Alpha Sys"},
		{ID: "beta-SYS", Object: "model", Created: 2, OwnedBy: "owner-b", Type: "model", DisplayName: "Beta Sys"},
		{ID: "gamma", Object: "model", Created: 3, OwnedBy: "owner-c", Type: "model", DisplayName: "Gamma"},
		{ID: "gamma-Sys", Object: "model", Created: 3, OwnedBy: "owner-c", Type: "model", DisplayName: "Gamma (Sys)"},
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expandOpenAISysAliases() mismatch\nwant: %#v\ngot:  %#v", want, got)
	}
}

func TestExpandClaudeSysAliases(t *testing.T) {
	models := []claude.Model{
		{ID: "delta-sys", Type: "model", DisplayName: "Manual Delta Sys", CreatedAt: "2024-01-01T00:00:00Z"},
		{ID: "Delta", Type: "model", DisplayName: "Delta", CreatedAt: "2024-01-02T00:00:00Z"},
		{ID: "epsilon", Type: "model", DisplayName: "Epsilon", CreatedAt: "2024-01-03T00:00:00Z"},
		{ID: "EPSILON", Type: "model", DisplayName: "Epsilon Duplicate", CreatedAt: "2024-01-04T00:00:00Z"},
	}

	got := expandClaudeSysAliases(models)
	want := []claude.Model{
		{ID: "delta-sys", Type: "model", DisplayName: "Manual Delta Sys", CreatedAt: "2024-01-01T00:00:00Z"},
		{ID: "Delta", Type: "model", DisplayName: "Delta", CreatedAt: "2024-01-02T00:00:00Z"},
		{ID: "epsilon", Type: "model", DisplayName: "Epsilon", CreatedAt: "2024-01-03T00:00:00Z"},
		{ID: "epsilon-Sys", Type: "model", DisplayName: "Epsilon (Sys)", CreatedAt: "2024-01-03T00:00:00Z"},
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expandClaudeSysAliases() mismatch\nwant: %#v\ngot:  %#v", want, got)
	}
}
