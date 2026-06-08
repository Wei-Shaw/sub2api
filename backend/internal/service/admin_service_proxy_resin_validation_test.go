package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

type proxyRepoStubForResinValidation struct {
	existing *Proxy
	created  []*Proxy
	updated  []*Proxy
}

func (s *proxyRepoStubForResinValidation) Create(_ context.Context, proxy *Proxy) error {
	s.created = append(s.created, proxy)
	return nil
}

func (s *proxyRepoStubForResinValidation) GetByID(_ context.Context, id int64) (*Proxy, error) {
	if s.existing == nil {
		return nil, ErrProxyNotFound
	}
	return s.existing, nil
}

func (s *proxyRepoStubForResinValidation) ListByIDs(context.Context, []int64) ([]Proxy, error) {
	panic("unexpected ListByIDs call")
}

func (s *proxyRepoStubForResinValidation) Update(_ context.Context, proxy *Proxy) error {
	s.updated = append(s.updated, proxy)
	s.existing = proxy
	return nil
}

func (s *proxyRepoStubForResinValidation) Delete(context.Context, int64) error {
	panic("unexpected Delete call")
}

func (s *proxyRepoStubForResinValidation) List(context.Context, pagination.PaginationParams) ([]Proxy, *pagination.PaginationResult, error) {
	panic("unexpected List call")
}

func (s *proxyRepoStubForResinValidation) ListWithFilters(context.Context, pagination.PaginationParams, string, string, string) ([]Proxy, *pagination.PaginationResult, error) {
	panic("unexpected ListWithFilters call")
}

func (s *proxyRepoStubForResinValidation) ListWithFiltersAndAccountCount(context.Context, pagination.PaginationParams, string, string, string) ([]ProxyWithAccountCount, *pagination.PaginationResult, error) {
	panic("unexpected ListWithFiltersAndAccountCount call")
}

func (s *proxyRepoStubForResinValidation) ListActive(context.Context) ([]Proxy, error) {
	panic("unexpected ListActive call")
}

func (s *proxyRepoStubForResinValidation) ListActiveWithAccountCount(context.Context) ([]ProxyWithAccountCount, error) {
	panic("unexpected ListActiveWithAccountCount call")
}

func (s *proxyRepoStubForResinValidation) ExistsByHostPortAuth(context.Context, string, int, string, string, string) (bool, error) {
	panic("unexpected ExistsByHostPortAuth call")
}

func (s *proxyRepoStubForResinValidation) CountAccountsByProxyID(context.Context, int64) (int64, error) {
	panic("unexpected CountAccountsByProxyID call")
}

func (s *proxyRepoStubForResinValidation) ListAccountSummariesByProxyID(context.Context, int64) ([]ProxyAccountSummary, error) {
	panic("unexpected ListAccountSummariesByProxyID call")
}

func (s *proxyRepoStubForResinValidation) SweepExpiredProxies(context.Context, time.Time) (int64, error) {
	panic("unexpected SweepExpiredProxies call")
}

func (s *proxyRepoStubForResinValidation) ListAllForFallback(context.Context) ([]Proxy, error) {
	panic("unexpected ListAllForFallback call")
}

func (s *proxyRepoStubForResinValidation) CountExpired(context.Context) (int64, error) {
	panic("unexpected CountExpired call")
}

func (s *proxyRepoStubForResinValidation) CountExpiringSoon(context.Context, time.Time) (int64, error) {
	panic("unexpected CountExpiringSoon call")
}

func TestAdminService_CreateProxy_AllowsResinWithoutToken(t *testing.T) {
	t.Parallel()

	repo := &proxyRepoStubForResinValidation{}
	svc := &adminServiceImpl{proxyRepo: repo}

	created, err := svc.CreateProxy(context.Background(), &CreateProxyInput{
		Name:     "resin",
		Protocol: ProxyProtocolResinHTTP,
		Host:     "resin.example.com",
		Port:     2260,
		Username: "openai",
	})
	require.NoError(t, err)
	require.NotNil(t, created)
	require.Len(t, repo.created, 1)
}

func TestAdminService_UpdateProxy_AllowsExistingResinWithoutToken(t *testing.T) {
	t.Parallel()

	repo := &proxyRepoStubForResinValidation{
		existing: &Proxy{
			ID:       1,
			Name:     "resin",
			Protocol: ProxyProtocolResinHTTP,
			Host:     "resin.example.com",
			Port:     2260,
			Username: "openai",
			Password: "",
			Status:   StatusActive,
		},
	}
	svc := &adminServiceImpl{proxyRepo: repo}

	updated, err := svc.UpdateProxy(context.Background(), 1, &UpdateProxyInput{
		Name: "resin-updated",
	})
	require.NoError(t, err)
	require.NotNil(t, updated)
	require.Len(t, repo.updated, 1)
}
