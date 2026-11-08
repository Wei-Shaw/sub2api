package repository

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func seedUserForAliasTest(t *testing.T, repo *userRepository, email string) {
	t.Helper()
	require.NoError(t, repo.Create(context.Background(), &service.User{
		Email:        email,
		Username:     email,
		PasswordHash: "hash",
		Role:         service.RoleUser,
		Status:       service.StatusActive,
	}))
}

func TestUserRepositoryExistsByEmailAlias(t *testing.T) {
	cases := []struct {
		name   string
		stored string
		probe  string
		want   bool
	}{
		{"same address", "someone@gmail.com", "someone@gmail.com", true},
		{"gmail plus alias", "someone@gmail.com", "someone+bulk294@gmail.com", true},
		{"gmail dot trick", "d.axis.2026@gmail.com", "daxis2026@gmail.com", true},
		{"gmail dot trick both sides", "d.axis.2026@gmail.com", "da.xis.2026@gmail.com", true},
		{"stored plus alias found by canonical form", "someone+tag@gmail.com", "someone@gmail.com", true},
		{"googlemail is a gmail alias", "someone@googlemail.com", "some.one@gmail.com", true},
		{"fqdn root dot on probe", "d.axis.2026@gmail.com", "da.xis.2026@gmail.com.", true},
		{"fqdn root dot on stored row", "d.axis.2026@gmail.com.", "daxis2026@gmail.com", true},
		{"legacy row with spacing and case", "  D.Axis.2026@Gmail.com  ", "daxis2026@gmail.com", true},
		{"non-gmail plus alias", "first.last@qq.com", "first.last+tag@qq.com", true},
		{"different gmail inbox", "someone@gmail.com", "someoneelse@gmail.com", false},
		{"non-gmail dots are significant", "first.last@qq.com", "firstlast@qq.com", false},
		{"different domain", "someone@gmail.com", "someone@qq.com", false},
		{"distinct plus-prefixed locals", "+alice@gmail.com", "+bob@gmail.com", false},
		{"underscore is not a wildcard", "user_x@qq.com", "userax@qq.com", false},
		{"percent is not a wildcard", "a%b@qq.com", "axxb@qq.com", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo, _ := newUserEntRepo(t)
			seedUserForAliasTest(t, repo, tc.stored)

			got, err := repo.ExistsByEmailAlias(context.Background(), tc.probe)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestUserRepositoryExistsByEmailAliasIgnoresMalformedInput(t *testing.T) {
	repo, _ := newUserEntRepo(t)
	seedUserForAliasTest(t, repo, "someone@gmail.com")

	got, err := repo.ExistsByEmailAlias(context.Background(), "not-an-email")
	require.NoError(t, err)
	require.False(t, got)
}

func TestUserRepositoryCreateWithEmailAliasGuard(t *testing.T) {
	repo, _ := newUserEntRepo(t)
	ctx := context.Background()
	seedUserForAliasTest(t, repo, "d.axis.2026@gmail.com")

	// 注册路径：别名变体在唯一性锁内被拒绝。
	err := repo.CreateWithEmailAliasGuard(ctx, &service.User{
		Email:        "da.xis.2026+free@googlemail.com",
		Username:     "alias-variant",
		PasswordHash: "hash",
		Role:         service.RoleUser,
		Status:       service.StatusActive,
	})
	require.ErrorIs(t, err, service.ErrEmailExists)

	// 不同收件箱仍可注册。
	require.NoError(t, repo.CreateWithEmailAliasGuard(ctx, &service.User{
		Email:        "other.person@gmail.com",
		Username:     "other-person",
		PasswordHash: "hash",
		Role:         service.RoleUser,
		Status:       service.StatusActive,
	}))

	// 管理员建号（Create）不受别名限制。
	require.NoError(t, repo.Create(ctx, &service.User{
		Email:        "daxis2026+support@gmail.com",
		Username:     "admin-created",
		PasswordHash: "hash",
		Role:         service.RoleUser,
		Status:       service.StatusActive,
	}))
}

func TestUserRepositoryCountUsersByEmailDomain(t *testing.T) {
	repo, _ := newUserEntRepo(t)
	ctx := context.Background()

	active := &service.User{
		Email:        "first@custom.example",
		Username:     "first",
		PasswordHash: "hash",
		Role:         service.RoleUser,
		Status:       service.StatusActive,
	}
	seedUserForAliasTest(t, repo, active.Email)
	seedUserForAliasTest(t, repo, "other@sub.custom.example")
	deleted := &service.User{
		Email:        "deleted@custom.example",
		Username:     "deleted",
		PasswordHash: "hash",
		Role:         service.RoleUser,
		Status:       service.StatusActive,
	}
	require.NoError(t, repo.Create(ctx, deleted))
	require.NoError(t, repo.Delete(ctx, deleted.ID))

	count, err := repo.CountUsersByEmailDomain(ctx, "custom.example")
	require.NoError(t, err)
	require.Equal(t, 2, count)
}

func TestUserRepositoryCreateWithEmailAliasGuardAndDomainLimit(t *testing.T) {
	repo, _ := newUserEntRepo(t)
	ctx := context.Background()
	seedUserForAliasTest(t, repo, "first@custom.example.")

	err := repo.CreateWithEmailAliasGuardAndDomainLimit(ctx, &service.User{
		Email:        "second@custom.example",
		Username:     "second",
		PasswordHash: "hash",
		Role:         service.RoleUser,
		Status:       service.StatusActive,
	}, "custom.example")
	require.ErrorIs(t, err, service.ErrEmailDomainRegistrationLimit)
}

func TestUserRepositoryCountUsersByEmailDomainEscapesLikeWildcards(t *testing.T) {
	repo, _ := newUserEntRepo(t)
	ctx := context.Background()
	seedUserForAliasTest(t, repo, "first@foo_bar.com")
	seedUserForAliasTest(t, repo, "other@fooxbar.com")

	count, err := repo.CountUsersByEmailDomain(ctx, "foo_bar.com")

	require.NoError(t, err)
	require.Equal(t, 1, count)
}
