//go:build unit

package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeEmailForAliasDedup(t *testing.T) {
	cases := []struct {
		name  string
		email string
		want  string
	}{
		{"plain", "user@example.com", "user@example.com"},
		{"uppercase and spaces", "  User@Example.COM ", "user@example.com"},
		{"plus alias stripped", "user+tag@example.com", "user@example.com"},
		{"gmail plus alias", "someone+bulk294@gmail.com", "someone@gmail.com"},
		{"gmail dots removed", "some.one@gmail.com", "someone@gmail.com"},
		{"gmail dots and plus", "s.o.m.e+x@gmail.com", "some@gmail.com"},
		{"googlemail folded to gmail", "user@googlemail.com", "user@gmail.com"},
		{"non-gmail keeps dots", "first.last@qq.com", "first.last@qq.com"},
		{"fqdn root dot dropped", "d.axis.2026@gmail.com.", "daxis2026@gmail.com"},
		{"fqdn root dot on other domain", "first.last@qq.com.", "first.last@qq.com"},
		{"leading plus keeps local part", "+alice@gmail.com", "+alice@gmail.com"},
		{"dot-only local part kept", "...@gmail.com", "...@gmail.com"},
		{"invalid keeps lowered raw", "not-an-email", "not-an-email"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, NormalizeEmailForAliasDedup(tc.email))
		})
	}
}

func TestNormalizeEmailForAliasDedupKeepsDistinctInboxes(t *testing.T) {
	// 剥离 "+后缀" 不能把同域下不同用户折叠成同一身份。
	require.NotEqual(t,
		NormalizeEmailForAliasDedup("+alice@gmail.com"),
		NormalizeEmailForAliasDedup("+bob@gmail.com"),
	)
	require.NotEqual(t,
		NormalizeEmailForAliasDedup("alice@gmail.com"),
		NormalizeEmailForAliasDedup("bob@gmail.com"),
	)
}

func TestEmailAliasDedupProbes(t *testing.T) {
	require.ElementsMatch(t,
		[]EmailAliasProbe{{Local: "someone", Domain: "gmailcom"}, {Local: "someone", Domain: "googlemailcom"}},
		EmailAliasDedupProbes("Some.One+tag@gmail.com"),
	)
	require.ElementsMatch(t,
		[]EmailAliasProbe{{Local: "daxis2026", Domain: "gmailcom"}, {Local: "daxis2026", Domain: "googlemailcom"}},
		EmailAliasDedupProbes("d.axis.2026@googlemail.com."),
	)
	require.Equal(t,
		[]EmailAliasProbe{{Local: "firstlast", Domain: "qqcom"}},
		EmailAliasDedupProbes("first.last+tag@qq.com"),
	)
	require.Nil(t, EmailAliasDedupProbes("not-an-email"))
	require.Nil(t, EmailAliasDedupProbes("...@gmail.com"))
}

// aliasDedupRepoStub implements only the methods alias dedup uses; other
// UserRepository methods come from the embedded nil interface (a wrong call
// would panic, failing the test).
type aliasDedupRepoStub struct {
	UserRepository
	exists      bool
	existsErr   error
	stored      []string
	aliasErr    error
	aliasChecks []string
}

func (s *aliasDedupRepoStub) ExistsByEmail(context.Context, string) (bool, error) {
	return s.exists, s.existsErr
}

func (s *aliasDedupRepoStub) ExistsByEmailAlias(_ context.Context, email string) (bool, error) {
	s.aliasChecks = append(s.aliasChecks, email)
	if s.aliasErr != nil {
		return false, s.aliasErr
	}
	identity := NormalizeEmailForAliasDedup(email)
	for _, candidate := range s.stored {
		if NormalizeEmailForAliasDedup(candidate) == identity {
			return true, nil
		}
	}
	return false, nil
}

func TestExistsByEmailOrAlias(t *testing.T) {
	ctx := context.Background()

	t.Run("exact duplicate short-circuits", func(t *testing.T) {
		repo := &aliasDedupRepoStub{exists: true}
		svc := &AuthService{userRepo: repo}
		got, err := svc.existsByEmailOrAlias(ctx, "user@gmail.com")
		require.NoError(t, err)
		require.True(t, got)
		require.Empty(t, repo.aliasChecks, "no alias probe expected after exact hit")
	})

	t.Run("plus alias variant detected", func(t *testing.T) {
		repo := &aliasDedupRepoStub{stored: []string{"someone+bulk294@gmail.com"}}
		svc := &AuthService{userRepo: repo}
		got, err := svc.existsByEmailOrAlias(ctx, "Someone@gmail.com")
		require.NoError(t, err)
		require.True(t, got)
	})

	t.Run("gmail dot variant detected", func(t *testing.T) {
		repo := &aliasDedupRepoStub{stored: []string{"some.one@gmail.com"}}
		svc := &AuthService{userRepo: repo}
		got, err := svc.existsByEmailOrAlias(ctx, "someone@gmail.com")
		require.NoError(t, err)
		require.True(t, got)
	})

	t.Run("fqdn root dot variant detected", func(t *testing.T) {
		repo := &aliasDedupRepoStub{stored: []string{"d.axis.2026@gmail.com"}}
		svc := &AuthService{userRepo: repo}
		got, err := svc.existsByEmailOrAlias(ctx, "da.xis.2026@gmail.com.")
		require.NoError(t, err)
		require.True(t, got)
	})

	t.Run("different inbox allowed", func(t *testing.T) {
		repo := &aliasDedupRepoStub{stored: []string{"other@gmail.com"}}
		svc := &AuthService{userRepo: repo}
		got, err := svc.existsByEmailOrAlias(ctx, "user@gmail.com")
		require.NoError(t, err)
		require.False(t, got)
	})

	t.Run("distinct plus-prefixed locals allowed", func(t *testing.T) {
		repo := &aliasDedupRepoStub{stored: []string{"+alice@gmail.com"}}
		svc := &AuthService{userRepo: repo}
		got, err := svc.existsByEmailOrAlias(ctx, "+bob@gmail.com")
		require.NoError(t, err)
		require.False(t, got)
	})

	t.Run("alias probe error fails closed", func(t *testing.T) {
		repo := &aliasDedupRepoStub{aliasErr: errors.New("db down")}
		svc := &AuthService{userRepo: repo}
		_, err := svc.existsByEmailOrAlias(ctx, "user@gmail.com")
		require.Error(t, err)
	})

	t.Run("exact check error propagates", func(t *testing.T) {
		repo := &aliasDedupRepoStub{existsErr: errors.New("db down")}
		svc := &AuthService{userRepo: repo}
		_, err := svc.existsByEmailOrAlias(ctx, "user@gmail.com")
		require.Error(t, err)
	})
}
