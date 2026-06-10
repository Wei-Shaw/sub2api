//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

type accountRepoStubForAdminList struct {
	accountRepoStub

	getByIDAccount          *Account
	getByIDErr              error
	getByIDsAccounts        []*Account
	getByIDsErr             error
	listWithFiltersCalls    int
	listWithFiltersParams   pagination.PaginationParams
	listWithFiltersPlatform string
	listWithFiltersType     string
	listWithFiltersStatus   string
	listWithFiltersSearch   string
	listWithFiltersPrivacy  string
	listWithFiltersAccounts []Account
	listWithFiltersResult   *pagination.PaginationResult
	listWithFiltersErr      error
}

func (s *accountRepoStubForAdminList) GetByID(_ context.Context, _ int64) (*Account, error) {
	if s.getByIDErr != nil {
		return nil, s.getByIDErr
	}
	if s.getByIDAccount != nil {
		return s.getByIDAccount, nil
	}
	return nil, ErrAccountNotFound
}

func (s *accountRepoStubForAdminList) GetByIDs(_ context.Context, _ []int64) ([]*Account, error) {
	if s.getByIDsErr != nil {
		return nil, s.getByIDsErr
	}
	return s.getByIDsAccounts, nil
}

func (s *accountRepoStubForAdminList) ListWithFilters(_ context.Context, params pagination.PaginationParams, platform, accountType, status, search string, groupID int64, privacyMode string) ([]Account, *pagination.PaginationResult, error) {
	s.listWithFiltersCalls++
	s.listWithFiltersParams = params
	s.listWithFiltersPlatform = platform
	s.listWithFiltersType = accountType
	s.listWithFiltersStatus = status
	s.listWithFiltersSearch = search
	s.listWithFiltersPrivacy = privacyMode

	if s.listWithFiltersErr != nil {
		return nil, nil, s.listWithFiltersErr
	}

	result := s.listWithFiltersResult
	if result == nil {
		result = &pagination.PaginationResult{
			Total:    int64(len(s.listWithFiltersAccounts)),
			Page:     params.Page,
			PageSize: params.PageSize,
		}
	}

	return s.listWithFiltersAccounts, result, nil
}

type proxyRepoStubForAdminList struct {
	proxyRepoStub

	listWithFiltersCalls    int
	listWithFiltersParams   pagination.PaginationParams
	listWithFiltersProtocol string
	listWithFiltersStatus   string
	listWithFiltersSearch   string
	listWithFiltersProxies  []Proxy
	listWithFiltersResult   *pagination.PaginationResult
	listWithFiltersErr      error

	listWithFiltersAndAccountCountCalls    int
	listWithFiltersAndAccountCountParams   pagination.PaginationParams
	listWithFiltersAndAccountCountProtocol string
	listWithFiltersAndAccountCountStatus   string
	listWithFiltersAndAccountCountSearch   string
	listWithFiltersAndAccountCountProxies  []ProxyWithAccountCount
	listWithFiltersAndAccountCountResult   *pagination.PaginationResult
	listWithFiltersAndAccountCountErr      error
}

func (s *proxyRepoStubForAdminList) ListWithFilters(_ context.Context, params pagination.PaginationParams, protocol, status, search string) ([]Proxy, *pagination.PaginationResult, error) {
	s.listWithFiltersCalls++
	s.listWithFiltersParams = params
	s.listWithFiltersProtocol = protocol
	s.listWithFiltersStatus = status
	s.listWithFiltersSearch = search

	if s.listWithFiltersErr != nil {
		return nil, nil, s.listWithFiltersErr
	}

	result := s.listWithFiltersResult
	if result == nil {
		result = &pagination.PaginationResult{
			Total:    int64(len(s.listWithFiltersProxies)),
			Page:     params.Page,
			PageSize: params.PageSize,
		}
	}

	return s.listWithFiltersProxies, result, nil
}

func (s *proxyRepoStubForAdminList) ListWithFiltersAndAccountCount(_ context.Context, params pagination.PaginationParams, protocol, status, search string) ([]ProxyWithAccountCount, *pagination.PaginationResult, error) {
	s.listWithFiltersAndAccountCountCalls++
	s.listWithFiltersAndAccountCountParams = params
	s.listWithFiltersAndAccountCountProtocol = protocol
	s.listWithFiltersAndAccountCountStatus = status
	s.listWithFiltersAndAccountCountSearch = search

	if s.listWithFiltersAndAccountCountErr != nil {
		return nil, nil, s.listWithFiltersAndAccountCountErr
	}

	result := s.listWithFiltersAndAccountCountResult
	if result == nil {
		result = &pagination.PaginationResult{
			Total:    int64(len(s.listWithFiltersAndAccountCountProxies)),
			Page:     params.Page,
			PageSize: params.PageSize,
		}
	}

	return s.listWithFiltersAndAccountCountProxies, result, nil
}

type redeemRepoStubForAdminList struct {
	redeemRepoStub

	listWithFiltersCalls  int
	listWithFiltersParams pagination.PaginationParams
	listWithFiltersType   string
	listWithFiltersStatus string
	listWithFiltersSearch string
	listWithFiltersCodes  []RedeemCode
	listWithFiltersResult *pagination.PaginationResult
	listWithFiltersErr    error
}

func (s *redeemRepoStubForAdminList) ListWithFilters(_ context.Context, params pagination.PaginationParams, codeType, status, search string) ([]RedeemCode, *pagination.PaginationResult, error) {
	s.listWithFiltersCalls++
	s.listWithFiltersParams = params
	s.listWithFiltersType = codeType
	s.listWithFiltersStatus = status
	s.listWithFiltersSearch = search

	if s.listWithFiltersErr != nil {
		return nil, nil, s.listWithFiltersErr
	}

	result := s.listWithFiltersResult
	if result == nil {
		result = &pagination.PaginationResult{
			Total:    int64(len(s.listWithFiltersCodes)),
			Page:     params.Page,
			PageSize: params.PageSize,
		}
	}

	return s.listWithFiltersCodes, result, nil
}

func (s *redeemRepoStubForAdminList) ListByUserPaginated(_ context.Context, userID int64, params pagination.PaginationParams, codeType string) ([]RedeemCode, *pagination.PaginationResult, error) {
	panic("unexpected ListByUserPaginated call")
}

func (s *redeemRepoStubForAdminList) SumPositiveBalanceByUser(_ context.Context, userID int64) (float64, error) {
	panic("unexpected SumPositiveBalanceByUser call")
}

func TestAdminService_ListAccounts_WithSearch(t *testing.T) {
	t.Run("search 参数正常传递到 repository 层", func(t *testing.T) {
		repo := &accountRepoStubForAdminList{
			listWithFiltersAccounts: []Account{{ID: 1, Name: "acc"}},
			listWithFiltersResult:   &pagination.PaginationResult{Total: 10},
		}
		svc := &adminServiceImpl{accountRepo: repo}

		accounts, total, err := svc.ListAccounts(context.Background(), 1, 20, PlatformGemini, AccountTypeOAuth, StatusActive, "acc", 0, "", "name", "ASC", 0)
		require.NoError(t, err)
		require.Equal(t, int64(10), total)
		require.Equal(t, []Account{{ID: 1, Name: "acc"}}, accounts)

		require.Equal(t, 1, repo.listWithFiltersCalls)
		require.Equal(t, pagination.PaginationParams{Page: 1, PageSize: 20, SortBy: "name", SortOrder: "ASC"}, repo.listWithFiltersParams)
		require.Equal(t, PlatformGemini, repo.listWithFiltersPlatform)
		require.Equal(t, AccountTypeOAuth, repo.listWithFiltersType)
		require.Equal(t, StatusActive, repo.listWithFiltersStatus)
		require.Equal(t, "acc", repo.listWithFiltersSearch)
	})
}

func TestAdminService_ListAccounts_NormalizesLegacyGeminiAPIKeyAccounts(t *testing.T) {
	repo := &accountRepoStubForAdminList{
		listWithFiltersAccounts: []Account{
			{
				ID:       11,
				Name:     "legacy-gemini-relay",
				Platform: PlatformGemini,
				Type:     AccountTypeOAuth,
				Credentials: map[string]any{
					"api_key":  "sk-test",
					"base_url": "https://iacc.cc",
				},
			},
		},
		listWithFiltersResult: &pagination.PaginationResult{Total: 1},
	}
	svc := &adminServiceImpl{accountRepo: repo}

	accounts, total, err := svc.ListAccounts(context.Background(), 1, 20, PlatformGemini, AccountTypeOAuth, "", "", 0, "", "", "", 0)
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, accounts, 1)
	require.Equal(t, AccountTypeAPIKey, accounts[0].Type)
	require.Equal(t, GeminiUpstreamCompatibleRelay, accounts[0].Credentials["upstream_type"])
	require.Equal(t, GeminiUpstreamCompatibleRelay, accounts[0].Credentials["tier_id"])
	require.Equal(t, AccountTypeOAuth, repo.listWithFiltersType)
}

func TestAdminService_GetAccount_NormalizesLegacyGeminiAPIKeyAccount(t *testing.T) {
	stored := &Account{
		ID:       12,
		Name:     "legacy-gemini-relay",
		Platform: PlatformGemini,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"api_key":  "sk-test",
			"base_url": "https://iacc.cc",
		},
	}
	repo := &accountRepoStubForAdminList{getByIDAccount: stored}
	svc := &adminServiceImpl{accountRepo: repo}

	account, err := svc.GetAccount(context.Background(), stored.ID)
	require.NoError(t, err)
	require.NotNil(t, account)
	require.Equal(t, AccountTypeAPIKey, account.Type)
	require.Equal(t, GeminiUpstreamCompatibleRelay, account.Credentials["upstream_type"])
	require.Equal(t, AccountTypeOAuth, stored.Type)
}

func TestAdminService_GetAccountsByIDs_NormalizesLegacyGeminiAPIKeyAccounts(t *testing.T) {
	repo := &accountRepoStubForAdminList{
		getByIDsAccounts: []*Account{
			{
				ID:       13,
				Name:     "legacy-gemini-relay",
				Platform: PlatformGemini,
				Type:     AccountTypeOAuth,
				Credentials: map[string]any{
					"api_key":  "sk-test",
					"base_url": "https://iacc.cc",
				},
			},
		},
	}
	svc := &adminServiceImpl{accountRepo: repo}

	accounts, err := svc.GetAccountsByIDs(context.Background(), []int64{13})
	require.NoError(t, err)
	require.Len(t, accounts, 1)
	require.Equal(t, AccountTypeAPIKey, accounts[0].Type)
	require.Equal(t, GeminiUpstreamCompatibleRelay, accounts[0].Credentials["upstream_type"])
}

func TestAdminService_ListAccounts_WithPrivacyMode(t *testing.T) {
	t.Run("privacy_mode 参数正常传递到 repository 层", func(t *testing.T) {
		repo := &accountRepoStubForAdminList{
			listWithFiltersAccounts: []Account{{ID: 2, Name: "acc2"}},
			listWithFiltersResult:   &pagination.PaginationResult{Total: 1},
		}
		svc := &adminServiceImpl{accountRepo: repo}

		accounts, total, err := svc.ListAccounts(context.Background(), 1, 20, PlatformOpenAI, AccountTypeOAuth, StatusActive, "acc2", 0, PrivacyModeCFBlocked, "", "", 0)
		require.NoError(t, err)
		require.Equal(t, int64(1), total)
		require.Equal(t, []Account{{ID: 2, Name: "acc2"}}, accounts)
		require.Equal(t, PrivacyModeCFBlocked, repo.listWithFiltersPrivacy)
	})
}

func TestAdminService_ListAccounts_WithBatchFilterPaginatesAfterFiltering(t *testing.T) {
	batchID := int64(7)
	otherBatchID := int64(9)
	repo := &accountRepoStubForAdminList{
		listWithFiltersAccounts: []Account{
			{ID: 1, Name: "skip", BatchID: &otherBatchID},
			{ID: 2, Name: "match-1", BatchID: &batchID},
			{ID: 3, Name: "match-2", BatchID: &batchID},
		},
		listWithFiltersResult: &pagination.PaginationResult{Total: 3},
	}
	svc := &adminServiceImpl{accountRepo: repo}

	accounts, total, err := svc.ListAccounts(context.Background(), 2, 1, "", "", "", "", 0, "", "name", "ASC", batchID)
	require.NoError(t, err)
	require.Equal(t, int64(2), total)
	require.Equal(t, []Account{{ID: 3, Name: "match-2", BatchID: &batchID}}, accounts)
	require.Equal(t, 1, repo.listWithFiltersCalls)
	require.Equal(t, pagination.PaginationParams{Page: 1, PageSize: 1000, SortBy: "name", SortOrder: "ASC"}, repo.listWithFiltersParams)
}

func TestAdminService_ListProxies_WithSearch(t *testing.T) {
	t.Run("search 参数正常传递到 repository 层", func(t *testing.T) {
		repo := &proxyRepoStubForAdminList{
			listWithFiltersProxies: []Proxy{{ID: 2, Name: "p1"}},
			listWithFiltersResult:  &pagination.PaginationResult{Total: 7},
		}
		svc := &adminServiceImpl{proxyRepo: repo}

		proxies, total, err := svc.ListProxies(context.Background(), 3, 50, "http", StatusActive, "p1", "name", "ASC")
		require.NoError(t, err)
		require.Equal(t, int64(7), total)
		require.Equal(t, []Proxy{{ID: 2, Name: "p1"}}, proxies)

		require.Equal(t, 1, repo.listWithFiltersCalls)
		require.Equal(t, pagination.PaginationParams{Page: 3, PageSize: 50, SortBy: "name", SortOrder: "ASC"}, repo.listWithFiltersParams)
		require.Equal(t, "http", repo.listWithFiltersProtocol)
		require.Equal(t, StatusActive, repo.listWithFiltersStatus)
		require.Equal(t, "p1", repo.listWithFiltersSearch)
	})
}

func TestAdminService_ListProxiesWithAccountCount_WithSearch(t *testing.T) {
	t.Run("search 参数正常传递到 repository 层", func(t *testing.T) {
		repo := &proxyRepoStubForAdminList{
			listWithFiltersAndAccountCountProxies: []ProxyWithAccountCount{{Proxy: Proxy{ID: 3, Name: "p2"}, AccountCount: 5}},
			listWithFiltersAndAccountCountResult:  &pagination.PaginationResult{Total: 9},
		}
		svc := &adminServiceImpl{proxyRepo: repo}

		proxies, total, err := svc.ListProxiesWithAccountCount(context.Background(), 2, 10, "socks5", StatusDisabled, "p2", "account_count", "DESC")
		require.NoError(t, err)
		require.Equal(t, int64(9), total)
		require.Equal(t, []ProxyWithAccountCount{{Proxy: Proxy{ID: 3, Name: "p2"}, AccountCount: 5}}, proxies)

		require.Equal(t, 1, repo.listWithFiltersAndAccountCountCalls)
		require.Equal(t, pagination.PaginationParams{Page: 2, PageSize: 10, SortBy: "account_count", SortOrder: "DESC"}, repo.listWithFiltersAndAccountCountParams)
		require.Equal(t, "socks5", repo.listWithFiltersAndAccountCountProtocol)
		require.Equal(t, StatusDisabled, repo.listWithFiltersAndAccountCountStatus)
		require.Equal(t, "p2", repo.listWithFiltersAndAccountCountSearch)
	})
}

func TestAdminService_ListRedeemCodes_WithSearch(t *testing.T) {
	t.Run("search 参数正常传递到 repository 层", func(t *testing.T) {
		repo := &redeemRepoStubForAdminList{
			listWithFiltersCodes:  []RedeemCode{{ID: 4, Code: "ABC"}},
			listWithFiltersResult: &pagination.PaginationResult{Total: 3},
		}
		svc := &adminServiceImpl{redeemCodeRepo: repo}

		codes, total, err := svc.ListRedeemCodes(context.Background(), 1, 20, RedeemTypeBalance, StatusUnused, "ABC", "value", "ASC")
		require.NoError(t, err)
		require.Equal(t, int64(3), total)
		require.Equal(t, []RedeemCode{{ID: 4, Code: "ABC"}}, codes)

		require.Equal(t, 1, repo.listWithFiltersCalls)
		require.Equal(t, pagination.PaginationParams{Page: 1, PageSize: 20, SortBy: "value", SortOrder: "ASC"}, repo.listWithFiltersParams)
		require.Equal(t, RedeemTypeBalance, repo.listWithFiltersType)
		require.Equal(t, StatusUnused, repo.listWithFiltersStatus)
		require.Equal(t, "ABC", repo.listWithFiltersSearch)
	})
}
