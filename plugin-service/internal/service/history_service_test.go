package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Wei-Shaw/sub2api/plugin-service/internal/model"
	"github.com/Wei-Shaw/sub2api/plugin-service/internal/repository"
)

func TestHistoryServiceDeleteRemovesOwnRecord(t *testing.T) {
	ctx := context.Background()
	repo := repository.NewHistoryRepository()
	svc := NewHistoryService(repo)
	principal := model.CurrentPrincipal{UserID: 7, Role: model.RoleUser, Email: "user@example.com", Plugin: "image-generation"}

	record, err := svc.Create(ctx, principal, "cat", map[string]any{"prompt": "cat"})
	if err != nil {
		t.Fatal(err)
	}

	if err := svc.Delete(ctx, principal, record.ID); err != nil {
		t.Fatal(err)
	}

	if _, err := svc.Get(ctx, principal, record.ID); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("Get() err = %v, want ErrNotFound", err)
	}
}

func TestHistoryServiceDeleteRejectsOtherUsersRecord(t *testing.T) {
	ctx := context.Background()
	repo := repository.NewHistoryRepository()
	svc := NewHistoryService(repo)
	owner := model.CurrentPrincipal{UserID: 7, Role: model.RoleUser, Email: "owner@example.com", Plugin: "image-generation"}
	other := model.CurrentPrincipal{UserID: 8, Role: model.RoleUser, Email: "other@example.com", Plugin: "image-generation"}

	record, err := svc.Create(ctx, owner, "cat", map[string]any{"prompt": "cat"})
	if err != nil {
		t.Fatal(err)
	}

	if err := svc.Delete(ctx, other, record.ID); !errors.Is(err, ErrHistoryForbidden) {
		t.Fatalf("Delete() err = %v, want ErrHistoryForbidden", err)
	}
}
