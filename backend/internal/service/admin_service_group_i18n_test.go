//go:build unit

package service

import (
	"context"
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestAdminService_CreateGroup_NormalizesI18n(t *testing.T) {
	repo := &groupRepoStubForAdmin{}
	svc := &adminServiceImpl{groupRepo: repo}

	_, err := svc.CreateGroup(context.Background(), &CreateGroupInput{
		Name:           "Claude Code - 标准",
		Platform:       PlatformAnthropic,
		RateMultiplier: 1.0,
		I18n: GroupI18n{
			"en": {Name: " Claude Code - Standard ", Description: "Pro + Max pool"},
			"zh": {},
		},
	})

	require.NoError(t, err)
	require.NotNil(t, repo.created)
	require.Equal(t, GroupI18n{
		"en": {Name: "Claude Code - Standard", Description: "Pro + Max pool"},
	}, repo.created.I18n)
}

func TestAdminService_CreateGroup_RejectsInvalidI18nLocale(t *testing.T) {
	repo := &groupRepoStubForAdmin{}
	svc := &adminServiceImpl{groupRepo: repo}

	_, err := svc.CreateGroup(context.Background(), &CreateGroupInput{
		Name:           "g",
		Platform:       PlatformAnthropic,
		RateMultiplier: 1.0,
		I18n:           GroupI18n{"en_US": {Name: "x"}},
	})

	require.Error(t, err)
	appErr := infraerrors.FromError(err)
	require.Equal(t, int32(http.StatusBadRequest), appErr.Code)
	require.Equal(t, "INVALID_GROUP_I18N", appErr.Reason)
	require.Nil(t, repo.created)
}

func TestAdminService_UpdateGroup_I18n(t *testing.T) {
	newExisting := func() *Group {
		return &Group{
			ID: 1, Name: "标准", Platform: PlatformAnthropic, Status: StatusActive, RateMultiplier: 1,
			I18n: GroupI18n{"en": {Name: "Standard"}},
		}
	}

	t.Run("omitted i18n keeps stored translations", func(t *testing.T) {
		repo := &groupRepoStubForAdmin{getByID: newExisting()}
		svc := &adminServiceImpl{groupRepo: repo}

		_, err := svc.UpdateGroup(context.Background(), 1, &UpdateGroupInput{Name: "标准版"})

		require.NoError(t, err)
		require.Equal(t, "标准版", repo.updated.Name)
		require.Equal(t, GroupI18n{"en": {Name: "Standard"}}, repo.updated.I18n)
	})

	t.Run("provided i18n replaces stored translations as a whole", func(t *testing.T) {
		repo := &groupRepoStubForAdmin{getByID: newExisting()}
		svc := &adminServiceImpl{groupRepo: repo}
		next := GroupI18n{"zh": {Description: " 号池 "}}

		_, err := svc.UpdateGroup(context.Background(), 1, &UpdateGroupInput{I18n: &next})

		require.NoError(t, err)
		require.Equal(t, GroupI18n{"zh": {Description: "号池"}}, repo.updated.I18n)
	})

	t.Run("empty i18n clears stored translations", func(t *testing.T) {
		repo := &groupRepoStubForAdmin{getByID: newExisting()}
		svc := &adminServiceImpl{groupRepo: repo}
		next := GroupI18n{}

		_, err := svc.UpdateGroup(context.Background(), 1, &UpdateGroupInput{I18n: &next})

		require.NoError(t, err)
		require.NotNil(t, repo.updated.I18n)
		require.Empty(t, repo.updated.I18n)
	})

	t.Run("invalid i18n is rejected before persisting", func(t *testing.T) {
		repo := &groupRepoStubForAdmin{getByID: newExisting()}
		svc := &adminServiceImpl{groupRepo: repo}
		next := GroupI18n{"not a locale": {Name: "x"}}

		_, err := svc.UpdateGroup(context.Background(), 1, &UpdateGroupInput{I18n: &next})

		require.Error(t, err)
		require.Equal(t, "INVALID_GROUP_I18N", infraerrors.FromError(err).Reason)
		require.Nil(t, repo.updated)
	})

	// 简易模式只放行名称与描述，译文属于同一类展示文案，同样放行。
	t.Run("simple mode keeps i18n editable", func(t *testing.T) {
		repo := &groupRepoStubForAdmin{getByID: newExisting()}
		svc := &adminServiceImpl{cfg: &config.Config{RunMode: config.RunModeSimple}, groupRepo: repo}
		next := GroupI18n{"en": {Name: "Standard v2"}}
		rate := 9.0

		_, err := svc.UpdateGroup(context.Background(), 1, &UpdateGroupInput{I18n: &next, RateMultiplier: &rate})

		require.NoError(t, err)
		require.Equal(t, GroupI18n{"en": {Name: "Standard v2"}}, repo.updated.I18n)
		require.Equal(t, 1.0, repo.updated.RateMultiplier)
	})
}

func TestCloneGroupForDuplicate_DropsNameTranslations(t *testing.T) {
	source := &Group{
		ID: 7, Name: "标准", Platform: PlatformAnthropic,
		I18n: GroupI18n{"en": {Name: "Standard", Description: "Pro + Max pool"}},
	}

	clone := cloneGroupForDuplicate(source, "op-1")

	require.Equal(t, GroupI18n{"en": {Description: "Pro + Max pool"}}, clone.I18n)
}
