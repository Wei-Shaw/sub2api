//go:build unit

package service

import (
	"context"
	"net/http"
	"testing"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestAdminService_CreateGroup_RejectsEmptyEnabledModelAllowlist(t *testing.T) {
	repo := &groupRepoStubForAdmin{createID: 51}
	svc := &adminServiceImpl{groupRepo: repo}

	_, err := svc.CreateGroup(context.Background(), &CreateGroupInput{
		Name:           "allowlist-empty-group",
		Platform:       PlatformOpenAI,
		RateMultiplier: 1,
		ModelAllowlist: GroupModelAllowlist{Enabled: true},
	})

	require.Error(t, err)
	appErr := infraerrors.FromError(err)
	require.Equal(t, int32(http.StatusBadRequest), appErr.Code)
	require.Equal(t, "INVALID_MODEL_ALLOWLIST", appErr.Reason)
	require.Nil(t, repo.created, "拒绝时不得落库")
}

func TestAdminService_CreateGroup_RejectsInvalidAllowlistWildcard(t *testing.T) {
	repo := &groupRepoStubForAdmin{createID: 51}
	svc := &adminServiceImpl{groupRepo: repo}

	_, err := svc.CreateGroup(context.Background(), &CreateGroupInput{
		Name:           "allowlist-wildcard-group",
		Platform:       PlatformOpenAI,
		RateMultiplier: 1,
		ModelAllowlist: GroupModelAllowlist{Enabled: true, Models: []string{"gpt-*-5.4"}},
	})

	require.Error(t, err)
	appErr := infraerrors.FromError(err)
	require.Equal(t, int32(http.StatusBadRequest), appErr.Code)
	require.Equal(t, "INVALID_MODEL_ALLOWLIST", appErr.Reason)
	require.Nil(t, repo.created, "拒绝时不得落库")
}

func TestAdminService_CreateGroup_NormalizesModelAllowlist(t *testing.T) {
	repo := &groupRepoStubForAdmin{createID: 52}
	svc := &adminServiceImpl{groupRepo: repo}

	_, err := svc.CreateGroup(context.Background(), &CreateGroupInput{
		Name:           "allowlist-normalized-group",
		Platform:       PlatformOpenAI,
		RateMultiplier: 1,
		ModelAllowlist: GroupModelAllowlist{
			Enabled: true,
			Models:  []string{" gpt-5.4 ", "GPT-5.4", "claude-*", "  "},
		},
	})

	require.NoError(t, err)
	require.NotNil(t, repo.created)
	require.True(t, repo.created.ModelAllowlist.Enabled)
	require.Equal(t, []string{"gpt-5.4", "claude-*"}, repo.created.ModelAllowlist.Models)
}

func TestAdminService_UpdateGroup_RejectsEmptyEnabledModelAllowlist(t *testing.T) {
	existing := &Group{ID: 1, Name: "existing", Platform: PlatformOpenAI, Status: StatusActive}
	repo := &groupRepoStubForAdmin{getByID: existing}
	svc := &adminServiceImpl{groupRepo: repo}

	_, err := svc.UpdateGroup(context.Background(), existing.ID, &UpdateGroupInput{
		ModelAllowlist: &GroupModelAllowlist{Enabled: true},
	})

	require.Error(t, err)
	appErr := infraerrors.FromError(err)
	require.Equal(t, int32(http.StatusBadRequest), appErr.Code)
	require.Equal(t, "INVALID_MODEL_ALLOWLIST", appErr.Reason)
	require.Nil(t, repo.updated, "拒绝时不得落库")
}

func TestAdminService_UpdateGroup_RejectsInvalidAllowlistWildcard(t *testing.T) {
	existing := &Group{ID: 1, Name: "existing", Platform: PlatformOpenAI, Status: StatusActive}
	repo := &groupRepoStubForAdmin{getByID: existing}
	svc := &adminServiceImpl{groupRepo: repo}

	_, err := svc.UpdateGroup(context.Background(), existing.ID, &UpdateGroupInput{
		ModelAllowlist: &GroupModelAllowlist{Enabled: true, Models: []string{"foo-*bar"}},
	})

	require.Error(t, err)
	appErr := infraerrors.FromError(err)
	require.Equal(t, int32(http.StatusBadRequest), appErr.Code)
	require.Equal(t, "INVALID_MODEL_ALLOWLIST", appErr.Reason)
	require.Nil(t, repo.updated, "拒绝时不得落库")
}

func TestAdminService_UpdateGroup_NormalizesAndResetsModelAllowlist(t *testing.T) {
	existing := &Group{
		ID: 1, Name: "existing", Platform: PlatformOpenAI, Status: StatusActive,
		ModelAllowlist: GroupModelAllowlist{Enabled: true, Models: []string{"gpt-5.4"}},
	}
	repo := &groupRepoStubForAdmin{getByID: existing}
	svc := &adminServiceImpl{groupRepo: repo}

	// 归一化：按小写去重保序（保留首次出现的原始拼写）。
	updated := GroupModelAllowlist{Enabled: true, Models: []string{" GPT-5.4 ", "claude-*"}}
	_, err := svc.UpdateGroup(context.Background(), existing.ID, &UpdateGroupInput{ModelAllowlist: &updated})
	require.NoError(t, err)
	require.NotNil(t, repo.updated)
	require.Equal(t, []string{"GPT-5.4", "claude-*"}, repo.updated.ModelAllowlist.Models)

	// 关闭且清空条目也应被接受（关闭白名单）。
	repo.updated = nil
	disabled := GroupModelAllowlist{Enabled: false}
	_, err = svc.UpdateGroup(context.Background(), existing.ID, &UpdateGroupInput{ModelAllowlist: &disabled})
	require.NoError(t, err)
	require.NotNil(t, repo.updated)
	require.False(t, repo.updated.ModelAllowlist.Enabled)
	require.Empty(t, repo.updated.ModelAllowlist.Models)
}
