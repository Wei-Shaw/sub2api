//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/cursor"
	"github.com/stretchr/testify/require"
)

type cursorModelsListAccountRepo struct {
	accountRepoStub
	byGroup    []Account
	byPlatform []Account
}

func (s *cursorModelsListAccountRepo) ListSchedulableByGroupID(_ context.Context, _ int64) ([]Account, error) {
	return s.byGroup, nil
}

func (s *cursorModelsListAccountRepo) ListSchedulableByPlatform(_ context.Context, platform string) ([]Account, error) {
	if platform != PlatformCursor {
		return nil, nil
	}
	return s.byPlatform, nil
}

func TestGetGroupModelsListCandidates_CursorUsesLivePicker(t *testing.T) {
	accountRepo := &cursorModelsListAccountRepo{
		byGroup: []Account{{
			ID:       7,
			Platform: PlatformCursor,
			Credentials: map[string]any{
				"access_token": "tok",
				"model_mapping": map[string]any{
					"custom-opus": "claude-opus-5",
				},
			},
		}},
	}
	svc := &adminServiceImpl{
		accountRepo: accountRepo,
		groupRepo: &groupRepoStubForAdmin{
			getByIDByID: map[int64]*Group{
				51: {ID: 51, Platform: PlatformCursor},
			},
		},
		cursorAvailableModels: func(context.Context, cursor.Credentials) ([]cursor.AvailableModel, error) {
			return []cursor.AvailableModel{
				{Name: "default"},
				{Name: "claude-opus-5"},
				{Name: "gpt-5.6-sol"},
			}, nil
		},
	}

	candidates, err := svc.GetGroupModelsListCandidates(context.Background(), 51, PlatformCursor)
	require.NoError(t, err)
	require.Equal(t, []string{"default", "claude-opus-5", "gpt-5.6-sol", "custom-opus"}, candidates)
}

func TestGetGroupModelsListCandidates_CursorCreateUsesAnySchedulableAccount(t *testing.T) {
	accountRepo := &cursorModelsListAccountRepo{
		byPlatform: []Account{{
			ID:       8,
			Platform: PlatformCursor,
			Credentials: map[string]any{
				"access_token": "tok",
			},
		}},
	}
	svc := &adminServiceImpl{
		accountRepo: accountRepo,
		cursorAvailableModels: func(context.Context, cursor.Credentials) ([]cursor.AvailableModel, error) {
			return []cursor.AvailableModel{
				{Name: "composer-2.5"},
				{Name: "grok-4.6"},
			}, nil
		},
	}

	candidates, err := svc.GetGroupModelsListCandidates(context.Background(), 0, PlatformCursor)
	require.NoError(t, err)
	require.Equal(t, []string{"composer-2.5", "grok-4.6"}, candidates)
}

func TestGetGroupModelsListCandidates_CursorFallsBackToDefaults(t *testing.T) {
	svc := &adminServiceImpl{}

	candidates, err := svc.GetGroupModelsListCandidates(context.Background(), 0, PlatformCursor)
	require.NoError(t, err)
	require.Equal(t, cursor.DefaultModelIDs(), candidates)
}
