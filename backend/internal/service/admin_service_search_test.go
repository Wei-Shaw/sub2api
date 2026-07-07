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
REDACTED

func (s *accountRepoStubForAdminList) ListAllWithFilters(context.Context, string, string, string, string, int64, string) ([]Account, error) {
	return nil, nil
REDACTED

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
REDACTED

	result := s.listWithFiltersResult
	if result == nil {
		result = &pagination.PaginationResult{
			Total:    int64(len(s.listWithFiltersAccounts)),
			Page:     params.Page,
			PageSize: params.PageSize,
	REDACTED
REDACTED

	return s.listWithFiltersAccounts, result, nil
REDACTED

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
REDACTED

func (s *proxyRepoStubForAdminList) ListWithFilters(_ context.Context, params pagination.PaginationParams, protocol, status, search string) ([]Proxy, *pagination.PaginationResult, error) {
	s.listWithFiltersCalls++
	s.listWithFiltersParams = params
	s.listWithFiltersProtocol = protocol
	s.listWithFiltersStatus = status
	s.listWithFiltersSearch = search

	if s.listWithFiltersErr != nil {
		return nil, nil, s.listWithFiltersErr
REDACTED

	result := s.listWithFiltersResult
	if result == nil {
		result = &pagination.PaginationResult{
			Total:    int64(len(s.listWithFiltersProxies)),
			Page:     params.Page,
			PageSize: params.PageSize,
	REDACTED
REDACTED

	return s.listWithFiltersProxies, result, nil
REDACTED

func (s *proxyRepoStubForAdminList) ListWithFiltersAndAccountCount(_ context.Context, params pagination.PaginationParams, protocol, status, search string) ([]ProxyWithAccountCount, *pagination.PaginationResult, error) {
	s.listWithFiltersAndAccountCountCalls++
	s.listWithFiltersAndAccountCountParams = params
	s.listWithFiltersAndAccountCountProtocol = protocol
	s.listWithFiltersAndAccountCountStatus = status
	s.listWithFiltersAndAccountCountSearch = search

	if s.listWithFiltersAndAccountCountErr != nil {
		return nil, nil, s.listWithFiltersAndAccountCountErr
REDACTED

	result := s.listWithFiltersAndAccountCountResult
	if result == nil {
		result = &pagination.PaginationResult{
			Total:    int64(len(s.listWithFiltersAndAccountCountProxies)),
			Page:     params.Page,
			PageSize: params.PageSize,
	REDACTED
REDACTED

	return s.listWithFiltersAndAccountCountProxies, result, nil
REDACTED

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
REDACTED

func (s *redeemRepoStubForAdminList) ListWithFilters(_ context.Context, params pagination.PaginationParams, codeType, status, search string) ([]RedeemCode, *pagination.PaginationResult, error) {
	s.listWithFiltersCalls++
	s.listWithFiltersParams = params
	s.listWithFiltersType = codeType
	s.listWithFiltersStatus = status
	s.listWithFiltersSearch = search

	if s.listWithFiltersErr != nil {
		return nil, nil, s.listWithFiltersErr
REDACTED

	result := s.listWithFiltersResult
	if result == nil {
		result = &pagination.PaginationResult{
			Total:    int64(len(s.listWithFiltersCodes)),
			Page:     params.Page,
			PageSize: params.PageSize,
	REDACTED
REDACTED

	return s.listWithFiltersCodes, result, nil
REDACTED

func (s *redeemRepoStubForAdminList) ListByUserPaginated(_ context.Context, userID int64, params pagination.PaginationParams, codeType string) ([]RedeemCode, *pagination.PaginationResult, error) {
	panic("unexpected ListByUserPaginated call")
REDACTED

func (s *redeemRepoStubForAdminList) SumPositiveBalanceByUser(_ context.Context, userID int64) (float64, error) {
	panic("unexpected SumPositiveBalanceByUser call")
REDACTED

func TestAdminService_ListAccounts_WithSearch(t *testing.T) {
	t.Run("search 参数正常传递到 repository 层", func(t *testing.T) {
		repo := &accountRepoStubForAdminList{
			listWithFiltersAccounts: []Account{{ID: 1, Name: "acc"REDACTEDREDACTED,
			listWithFiltersResult:   &pagination.PaginationResult{Total: 10REDACTED,
	REDACTED
		svc := &adminServiceImpl{accountRepo: repoREDACTED

		accounts, total, err := svc.ListAccounts(context.Background(), 1, 20, PlatformGemini, AccountTypeOAuth, StatusActive, "acc", 0, "", "name", "ASC")
	REDACTED
		require.Equal(t, int64(10), total)
		require.Equal(t, []Account{{ID: 1, Name: "acc"REDACTEDREDACTED, accounts)

		require.Equal(t, 1, repo.listWithFiltersCalls)
		require.Equal(t, pagination.PaginationParams{Page: 1, PageSize: 20, SortBy: "name", SortOrder: "ASC"REDACTED, repo.listWithFiltersParams)
		require.Equal(t, PlatformGemini, repo.listWithFiltersPlatform)
		require.Equal(t, AccountTypeOAuth, repo.listWithFiltersType)
		require.Equal(t, StatusActive, repo.listWithFiltersStatus)
		require.Equal(t, "acc", repo.listWithFiltersSearch)
REDACTED)
REDACTED

func TestAdminService_ListAccounts_WithPrivacyMode(t *testing.T) {
	t.Run("privacy_mode 参数正常传递到 repository 层", func(t *testing.T) {
		repo := &accountRepoStubForAdminList{
			listWithFiltersAccounts: []Account{{ID: 2, Name: "acc2"REDACTEDREDACTED,
			listWithFiltersResult:   &pagination.PaginationResult{Total: 1REDACTED,
	REDACTED
		svc := &adminServiceImpl{accountRepo: repoREDACTED

		accounts, total, err := svc.ListAccounts(context.Background(), 1, 20, PlatformOpenAI, AccountTypeOAuth, StatusActive, "acc2", 0, PrivacyModeCFBlocked, "", "")
	REDACTED
		require.Equal(t, int64(1), total)
		require.Equal(t, []Account{{ID: 2, Name: "acc2"REDACTEDREDACTED, accounts)
		require.Equal(t, PrivacyModeCFBlocked, repo.listWithFiltersPrivacy)
REDACTED)
REDACTED

func TestAdminService_ListProxies_WithSearch(t *testing.T) {
	t.Run("search 参数正常传递到 repository 层", func(t *testing.T) {
		repo := &proxyRepoStubForAdminList{
			listWithFiltersProxies: []Proxy{{ID: 2, Name: "p1"REDACTEDREDACTED,
			listWithFiltersResult:  &pagination.PaginationResult{Total: 7REDACTED,
	REDACTED
		svc := &adminServiceImpl{proxyRepo: repoREDACTED

		proxies, total, err := svc.ListProxies(context.Background(), 3, 50, "http", StatusActive, "p1", "name", "ASC")
	REDACTED
		require.Equal(t, int64(7), total)
		require.Equal(t, []Proxy{{ID: 2, Name: "p1"REDACTEDREDACTED, proxies)

		require.Equal(t, 1, repo.listWithFiltersCalls)
		require.Equal(t, pagination.PaginationParams{Page: 3, PageSize: 50, SortBy: "name", SortOrder: "ASC"REDACTED, repo.listWithFiltersParams)
		require.Equal(t, "http", repo.listWithFiltersProtocol)
		require.Equal(t, StatusActive, repo.listWithFiltersStatus)
		require.Equal(t, "p1", repo.listWithFiltersSearch)
REDACTED)
REDACTED

func TestAdminService_ListProxiesWithAccountCount_WithSearch(t *testing.T) {
	t.Run("search 参数正常传递到 repository 层", func(t *testing.T) {
		repo := &proxyRepoStubForAdminList{
			listWithFiltersAndAccountCountProxies: []ProxyWithAccountCount{{Proxy: Proxy{ID: 3, Name: "p2"REDACTED, AccountCount: 5REDACTEDREDACTED,
			listWithFiltersAndAccountCountResult:  &pagination.PaginationResult{Total: 9REDACTED,
	REDACTED
		svc := &adminServiceImpl{proxyRepo: repoREDACTED

		proxies, total, err := svc.ListProxiesWithAccountCount(context.Background(), 2, 10, "socks5", StatusDisabled, "p2", "account_count", "DESC")
	REDACTED
		require.Equal(t, int64(9), total)
		require.Equal(t, []ProxyWithAccountCount{{Proxy: Proxy{ID: 3, Name: "p2"REDACTED, AccountCount: 5REDACTEDREDACTED, proxies)

		require.Equal(t, 1, repo.listWithFiltersAndAccountCountCalls)
		require.Equal(t, pagination.PaginationParams{Page: 2, PageSize: 10, SortBy: "account_count", SortOrder: "DESC"REDACTED, repo.listWithFiltersAndAccountCountParams)
		require.Equal(t, "socks5", repo.listWithFiltersAndAccountCountProtocol)
		require.Equal(t, StatusDisabled, repo.listWithFiltersAndAccountCountStatus)
		require.Equal(t, "p2", repo.listWithFiltersAndAccountCountSearch)
REDACTED)
REDACTED

func TestAdminService_ListRedeemCodes_WithSearch(t *testing.T) {
	t.Run("search 参数正常传递到 repository 层", func(t *testing.T) {
		repo := &redeemRepoStubForAdminList{
			listWithFiltersCodes:  []RedeemCode{{ID: 4, Code: "ABC"REDACTEDREDACTED,
			listWithFiltersResult: &pagination.PaginationResult{Total: 3REDACTED,
	REDACTED
		svc := &adminServiceImpl{redeemCodeRepo: repoREDACTED

		codes, total, err := svc.ListRedeemCodes(context.Background(), 1, 20, RedeemTypeBalance, StatusUnused, "ABC", "value", "ASC")
	REDACTED
		require.Equal(t, int64(3), total)
		require.Equal(t, []RedeemCode{{ID: 4, Code: "ABC"REDACTEDREDACTED, codes)

		require.Equal(t, 1, repo.listWithFiltersCalls)
		require.Equal(t, pagination.PaginationParams{Page: 1, PageSize: 20, SortBy: "value", SortOrder: "ASC"REDACTED, repo.listWithFiltersParams)
		require.Equal(t, RedeemTypeBalance, repo.listWithFiltersType)
		require.Equal(t, StatusUnused, repo.listWithFiltersStatus)
		require.Equal(t, "ABC", repo.listWithFiltersSearch)
REDACTED)
REDACTED
