//go:build unit

package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

type userRepoStubForListUsers struct {
	userRepoStub
	users []User
	err   error
REDACTED

func (s *userRepoStubForListUsers) ListWithFilters(_ context.Context, params pagination.PaginationParams, _ UserListFilters) ([]User, *pagination.PaginationResult, error) {
	if s.err != nil {
		return nil, nil, s.err
REDACTED
	out := make([]User, len(s.users))
	copy(out, s.users)
	return out, &pagination.PaginationResult{
		Total:    int64(len(out)),
		Page:     params.Page,
		PageSize: params.PageSize,
REDACTED, nil
REDACTED

type userGroupRateRepoStubForListUsers struct {
	batchCalls int
	singleCall []int64

	batchErr  error
	batchData map[int64]map[int64]float64

	singleErr  map[int64]error
	singleData map[int64]map[int64]float64
REDACTED

func (s *userGroupRateRepoStubForListUsers) GetByUserIDs(_ context.Context, _ []int64) (map[int64]map[int64]float64, error) {
	s.batchCalls++
	if s.batchErr != nil {
		return nil, s.batchErr
REDACTED
	return s.batchData, nil
REDACTED

func (s *userGroupRateRepoStubForListUsers) GetByUserID(_ context.Context, userID int64) (map[int64]float64, error) {
	s.singleCall = append(s.singleCall, userID)
	if err, ok := s.singleErr[userID]; ok {
		return nil, err
REDACTED
	if rates, ok := s.singleData[userID]; ok {
		return rates, nil
REDACTED
	return map[int64]float64{REDACTED, nil
REDACTED

func (s *userGroupRateRepoStubForListUsers) GetByUserAndGroup(_ context.Context, userID, groupID int64) (*float64, error) {
	panic("unexpected GetByUserAndGroup call")
REDACTED

func (s *userGroupRateRepoStubForListUsers) SyncUserGroupRates(_ context.Context, userID int64, rates map[int64]*float64) error {
	panic("unexpected SyncUserGroupRates call")
REDACTED

func (s *userGroupRateRepoStubForListUsers) GetByGroupID(_ context.Context, _ int64) ([]UserGroupRateEntry, error) {
	panic("unexpected GetByGroupID call")
REDACTED

func (s *userGroupRateRepoStubForListUsers) SyncGroupRateMultipliers(_ context.Context, _ int64, _ []GroupRateMultiplierInput) error {
	panic("unexpected SyncGroupRateMultipliers call")
REDACTED

func (s *userGroupRateRepoStubForListUsers) DeleteByGroupID(_ context.Context, _ int64) error {
	panic("unexpected DeleteByGroupID call")
REDACTED

func (s *userGroupRateRepoStubForListUsers) DeleteByUserID(_ context.Context, userID int64) error {
	panic("unexpected DeleteByUserID call")
REDACTED

func TestAdminService_ListUsers_BatchRateFallbackToSingle(t *testing.T) {
	userRepo := &userRepoStubForListUsers{
		users: []User{
			{ID: 101, Username: "u1"REDACTED,
			{ID: 202, Username: "u2"REDACTED,
	REDACTED,
REDACTED
	rateRepo := &userGroupRateRepoStubForListUsers{
		batchErr: errors.New("batch unavailable"),
		singleData: map[int64]map[int64]float64{
			101: {11: 1.1REDACTED,
			202: {22: 2.2REDACTED,
	REDACTED,
REDACTED
	svc := &adminServiceImpl{
		userRepo:          userRepo,
		userGroupRateRepo: rateRepo,
REDACTED

	users, total, err := svc.ListUsers(context.Background(), 1, 20, UserListFilters{REDACTED)
REDACTED
	require.Equal(t, int64(2), total)
	require.Len(t, users, 2)
	require.Equal(t, 1, rateRepo.batchCalls)
	require.ElementsMatch(t, []int64{101, 202REDACTED, rateRepo.singleCall)
	require.Equal(t, 1.1, users[0].GroupRates[11])
	require.Equal(t, 2.2, users[1].GroupRates[22])
REDACTED
