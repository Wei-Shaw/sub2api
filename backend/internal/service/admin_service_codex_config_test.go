//go:build unit

package service

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestAdminService_CreateGroup_TrimsCodexConfigDefaultModel(t *testing.T) {
	repo := &groupRepoStubForAdmin{}
	svc := &adminServiceImpl{groupRepo: repo}

	group, err := svc.CreateGroup(context.Background(), &CreateGroupInput{
		Name:                    "codex-default-model",
		Platform:                PlatformOpenAI,
		RateMultiplier:          1,
		CodexConfigDefaultModel: "  custom-model  ",
		CodexConfigReviewModel:  "  custom-review-model  ",
	})

	require.NoError(t, err)
	require.Equal(t, "custom-model", group.CodexConfigDefaultModel)
	require.Equal(t, "custom-model", repo.created.CodexConfigDefaultModel)
	require.Equal(t, "custom-review-model", group.CodexConfigReviewModel)
	require.Equal(t, "custom-review-model", repo.created.CodexConfigReviewModel)
}

func TestAdminService_UpdateGroup_CanClearCodexConfigDefaultModel(t *testing.T) {
	existing := &Group{
		ID:                      1,
		Name:                    "codex-default-model",
		Platform:                PlatformOpenAI,
		Status:                  StatusActive,
		CodexConfigDefaultModel: "custom-model",
		CodexConfigReviewModel:  "custom-review-model",
	}
	repo := &groupRepoStubForAdmin{getByID: existing}
	svc := &adminServiceImpl{groupRepo: repo}
	empty := ""

	group, err := svc.UpdateGroup(context.Background(), existing.ID, &UpdateGroupInput{
		CodexConfigDefaultModel: &empty,
		CodexConfigReviewModel:  &empty,
	})

	require.NoError(t, err)
	require.Empty(t, group.CodexConfigDefaultModel)
	require.Empty(t, repo.updated.CodexConfigDefaultModel)
	require.Empty(t, group.CodexConfigReviewModel)
	require.Empty(t, repo.updated.CodexConfigReviewModel)
}

func TestAdminService_UpdateGroup_OmittedCodexConfigDefaultModelPreservesValue(t *testing.T) {
	existing := &Group{
		ID:                      1,
		Name:                    "codex-default-model",
		Platform:                PlatformOpenAI,
		Status:                  StatusActive,
		CodexConfigDefaultModel: "custom-model",
		CodexConfigReviewModel:  "custom-review-model",
	}
	repo := &groupRepoStubForAdmin{getByID: existing}
	svc := &adminServiceImpl{groupRepo: repo}

	group, err := svc.UpdateGroup(context.Background(), existing.ID, &UpdateGroupInput{})

	require.NoError(t, err)
	require.Equal(t, "custom-model", group.CodexConfigDefaultModel)
	require.Equal(t, "custom-model", repo.updated.CodexConfigDefaultModel)
	require.Equal(t, "custom-review-model", group.CodexConfigReviewModel)
	require.Equal(t, "custom-review-model", repo.updated.CodexConfigReviewModel)
}

func TestAdminService_SimpleModePreservesCodexConfigDefaultModel(t *testing.T) {
	repo := &groupRepoStubForAdmin{}
	svc := &adminServiceImpl{
		cfg:       &config.Config{RunMode: config.RunModeSimple},
		groupRepo: repo,
	}

	group, err := svc.CreateGroup(context.Background(), &CreateGroupInput{
		Name:                    "codex-default-model-simple",
		Platform:                PlatformOpenAI,
		CodexConfigDefaultModel: "custom-model",
		CodexConfigReviewModel:  "custom-review-model",
	})

	require.NoError(t, err)
	require.Equal(t, "custom-model", group.CodexConfigDefaultModel)
	require.Equal(t, "custom-model", repo.created.CodexConfigDefaultModel)
	require.Equal(t, "custom-review-model", group.CodexConfigReviewModel)
	require.Equal(t, "custom-review-model", repo.created.CodexConfigReviewModel)
}

func TestAdminService_RejectsInvalidCodexConfigModels(t *testing.T) {
	tests := []struct {
		name  string
		input CreateGroupInput
	}{
		{
			name: "default model too long",
			input: CreateGroupInput{
				Name:                    "codex-invalid-default",
				Platform:                PlatformOpenAI,
				RateMultiplier:          1,
				CodexConfigDefaultModel: strings.Repeat("m", maxCodexConfigModelLength+1),
			},
		},
		{
			name: "review model contains control character",
			input: CreateGroupInput{
				Name:                   "codex-invalid-review",
				Platform:               PlatformOpenAI,
				RateMultiplier:         1,
				CodexConfigReviewModel: "review\nmodel",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &groupRepoStubForAdmin{}
			svc := &adminServiceImpl{groupRepo: repo}

			group, err := svc.CreateGroup(context.Background(), &tt.input)

			require.Nil(t, group)
			require.Error(t, err)
			require.Equal(t, http.StatusBadRequest, infraerrors.Code(err))
			require.Equal(t, "INVALID_CODEX_CONFIG_MODEL", infraerrors.Reason(err))
			require.Nil(t, repo.created)
		})
	}
}

func TestAdminService_UpdateGroup_RejectsInvalidCodexConfigReviewModel(t *testing.T) {
	existing := &Group{
		ID:                     1,
		Name:                   "codex-invalid-review-update",
		Platform:               PlatformOpenAI,
		Status:                 StatusActive,
		CodexConfigReviewModel: "existing-review-model",
	}
	repo := &groupRepoStubForAdmin{getByID: existing}
	svc := &adminServiceImpl{groupRepo: repo}
	invalid := strings.Repeat("r", maxCodexConfigModelLength+1)

	group, err := svc.UpdateGroup(context.Background(), existing.ID, &UpdateGroupInput{
		CodexConfigReviewModel: &invalid,
	})

	require.Nil(t, group)
	require.Error(t, err)
	require.Equal(t, http.StatusBadRequest, infraerrors.Code(err))
	require.Equal(t, "INVALID_CODEX_CONFIG_MODEL", infraerrors.Reason(err))
	require.Nil(t, repo.updated)
}
