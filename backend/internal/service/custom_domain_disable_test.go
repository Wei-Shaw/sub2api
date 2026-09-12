package service

import (
	"context"
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestCustomDomainOwnerCannotUndoAdminDisable(t *testing.T) {
	ctx := context.Background()
	repo := newCustomDomainRepoStub()
	svc := newCustomDomainTestService(repo)
	domain, err := svc.CreateForUser(ctx, 42, "api.customer.example")
	require.NoError(t, err)
	svc.SetDNSResolverForTest(&customDomainDNSStub{txt: map[string][]string{domain.VerificationTXTName: {domain.VerificationTXTValue}}})
	_, err = svc.VerifyForUser(ctx, 42, domain.ID)
	require.NoError(t, err)
	_, err = svc.DisableAsAdmin(ctx, domain.ID, "disabled by operator")
	require.NoError(t, err)
	_, err = svc.VerifyForUser(ctx, 42, domain.ID)
	require.ErrorIs(t, err, ErrCustomDomainInactive)
	require.ErrorIs(t, svc.DeleteForUser(ctx, 42, domain.ID), ErrCustomDomainInactive)
	stored, readErr := repo.GetByID(ctx, domain.ID)
	require.NoError(t, readErr)
	require.Equal(t, CustomDomainStatusDisabled, stored.Status, "owner verify must preserve the administrator's disable; verify error=%v", err)
}

type customDomainDNSFunc func(context.Context, string) ([]string, error)

func (f customDomainDNSFunc) LookupTXT(ctx context.Context, name string) ([]string, error) {
	return f(ctx, name)
}

func TestCustomDomainDisableDuringDNSVerificationWins(t *testing.T) {
	for _, validTXT := range []bool{true, false} {
		t.Run(fmt.Sprint(validTXT), func(t *testing.T) {
			ctx := context.Background()
			repo := newCustomDomainRepoStub()
			svc := newCustomDomainTestService(repo)
			domain, err := svc.CreateForUser(ctx, 42, "api.customer.example")
			require.NoError(t, err)
			svc.SetDNSResolverForTest(customDomainDNSFunc(func(context.Context, string) ([]string, error) {
				_, err := svc.DisableAsAdmin(ctx, domain.ID, "operator hold")
				require.NoError(t, err)
				if validTXT {
					return []string{domain.VerificationTXTValue}, nil
				}
				return nil, fmt.Errorf("DNS failure")
			}))
			_, err = svc.VerifyForUser(ctx, 42, domain.ID)
			require.ErrorIs(t, err, ErrCustomDomainInactive)
			stored, err := repo.GetByID(ctx, domain.ID)
			require.NoError(t, err)
			require.Equal(t, CustomDomainStatusDisabled, stored.Status)
			require.Equal(t, "operator hold", *stored.DisabledReason)
		})
	}
}

func TestCustomDomainOwnerCanVerifyAfterExplicitAdminEnable(t *testing.T) {
	ctx := context.Background()
	svc := newCustomDomainTestService(newCustomDomainRepoStub())
	domain, err := svc.CreateForUser(ctx, 42, "api.customer.example")
	require.NoError(t, err)
	_, err = svc.DisableAsAdmin(ctx, domain.ID, "operator hold")
	require.NoError(t, err)
	_, err = svc.EnableAsAdmin(ctx, domain.ID)
	require.NoError(t, err)
	svc.SetDNSResolverForTest(&customDomainDNSStub{txt: map[string][]string{domain.VerificationTXTName: {domain.VerificationTXTValue}}})
	verified, err := svc.VerifyForUser(ctx, 42, domain.ID)
	require.NoError(t, err)
	require.Equal(t, CustomDomainStatusActive, verified.Status)
}
