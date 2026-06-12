package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

type generateRedeemCodesRepoStub struct {
	created []RedeemCode
}

func (s *generateRedeemCodesRepoStub) Create(ctx context.Context, code *RedeemCode) error {
	if code == nil {
		return nil
	}
	clone := *code
	s.created = append(s.created, clone)
	return nil
}
func (s *generateRedeemCodesRepoStub) CreateBatch(ctx context.Context, codes []RedeemCode) error {
	s.created = append(s.created, codes...)
	return nil
}
func (s *generateRedeemCodesRepoStub) GetByID(context.Context, int64) (*RedeemCode, error) {
	panic("unexpected GetByID call")
}
func (s *generateRedeemCodesRepoStub) GetByCode(context.Context, string) (*RedeemCode, error) {
	panic("unexpected GetByCode call")
}
func (s *generateRedeemCodesRepoStub) Update(context.Context, *RedeemCode) error {
	panic("unexpected Update call")
}
func (s *generateRedeemCodesRepoStub) BatchUpdate(context.Context, []int64, RedeemCodeBatchUpdateFields) (int64, error) {
	panic("unexpected BatchUpdate call")
}
func (s *generateRedeemCodesRepoStub) Delete(context.Context, int64) error {
	panic("unexpected Delete call")
}
func (s *generateRedeemCodesRepoStub) Use(context.Context, int64, int64) error {
	panic("unexpected Use call")
}
func (s *generateRedeemCodesRepoStub) List(context.Context, pagination.PaginationParams) ([]RedeemCode, *pagination.PaginationResult, error) {
	panic("unexpected List call")
}
func (s *generateRedeemCodesRepoStub) ListWithFilters(context.Context, pagination.PaginationParams, string, string, string, *int64) ([]RedeemCode, *pagination.PaginationResult, error) {
	panic("unexpected ListWithFilters call")
}
func (s *generateRedeemCodesRepoStub) ListByUser(context.Context, int64, int) ([]RedeemCode, error) {
	panic("unexpected ListByUser call")
}
func (s *generateRedeemCodesRepoStub) ListByUserPaginated(context.Context, int64, pagination.PaginationParams, string) ([]RedeemCode, *pagination.PaginationResult, error) {
	panic("unexpected ListByUserPaginated call")
}
func (s *generateRedeemCodesRepoStub) SumPositiveBalanceByUser(context.Context, int64) (float64, error) {
	panic("unexpected SumPositiveBalanceByUser call")
}

func TestAdminServiceGenerateRedeemCodesPersistsUsagePolicyFields(t *testing.T) {
	maxTotalUses := 50
	maxUsesPerUser := 1
	repo := &generateRedeemCodesRepoStub{}
	svc := &adminServiceImpl{redeemCodeRepo: repo}

	codes, err := svc.GenerateRedeemCodes(context.Background(), &GenerateRedeemCodesInput{
		Count:          2,
		Type:           RedeemTypeBalance,
		Value:          25,
		UsagePolicy:    RedeemUsagePolicyOncePerUser,
		UsageScope:     "campaign-2026",
		MaxTotalUses:   &maxTotalUses,
		MaxUsesPerUser: &maxUsesPerUser,
	})

	require.NoError(t, err)
	require.Len(t, codes, 2)
	require.Len(t, repo.created, 2)
	for _, code := range repo.created {
		require.Equal(t, RedeemUsagePolicyOncePerUser, code.UsagePolicy)
		require.Equal(t, "campaign-2026", code.UsageScope)
		require.Equal(t, &maxTotalUses, code.MaxTotalUses)
		require.Equal(t, &maxUsesPerUser, code.MaxUsesPerUser)
		require.Zero(t, code.UsedCount)
	}
}
